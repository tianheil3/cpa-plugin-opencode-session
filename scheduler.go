package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	quotaCacheTTL    = 45 * time.Second
	exhaustedHold    = 5 * time.Minute
	exhaustedRemain  = 0.01
	quotaRefreshWait = 12 * time.Second
)

func (p *sessionPlugin) Pick(ctx context.Context, req pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
	ours := p.openCodeCandidates(req.Candidates)
	if len(ours) == 0 || len(ours) != len(req.Candidates) {
		return pluginapi.SchedulerPickResponse{}, nil
	}
	p.kickQuotaRefresh()
	id := p.pickOpenCodeAuthID(ours)
	if id == "" {
		if enabled, known := p.enabledAuthIDs(); known && len(enabled) == 0 {
			return pluginapi.SchedulerPickResponse{Handled: true}, nil
		}
		return pluginapi.SchedulerPickResponse{}, nil
	}
	p.mu.Lock()
	p.lastPickAuthID = id
	p.mu.Unlock()
	return pluginapi.SchedulerPickResponse{Handled: true, AuthID: id}, nil
}

func (p *sessionPlugin) openCodeCandidates(candidates []pluginapi.SchedulerAuthCandidate) []pluginapi.SchedulerAuthCandidate {
	out := make([]pluginapi.SchedulerAuthCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if isOpenCodeSchedulerCandidate(candidate, p.cfg.providerName()) {
			out = append(out, candidate)
		}
	}
	return out
}

func isOpenCodeSchedulerCandidate(candidate pluginapi.SchedulerAuthCandidate, providerName string) bool {
	req := pluginapi.QuotaFetchRequest{
		Provider:   candidate.Provider,
		Attributes: candidate.Attributes,
	}
	return isOpenCodeQuotaRequest(req, providerName)
}

func (p *sessionPlugin) pickOpenCodeAuthID(candidates []pluginapi.SchedulerAuthCandidate) string {
	enabled, known := p.enabledAuthIDs()
	filtered := make([]pluginapi.SchedulerAuthCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			continue
		}
		if known {
			if _, ok := enabled[id]; !ok {
				continue
			}
		}
		filtered = append(filtered, candidate)
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) == 1 {
		return strings.TrimSpace(filtered[0].ID)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	bestID := strings.TrimSpace(filtered[0].ID)
	bestScore := p.quotaScoreLocked(bestID)
	last := strings.TrimSpace(p.lastPickAuthID)
	for _, candidate := range filtered[1:] {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			continue
		}
		score := p.quotaScoreLocked(id)
		if score > bestScore || (score == bestScore && id == last) {
			bestScore = score
			bestID = id
		}
	}
	return bestID
}

func (p *sessionPlugin) enabledAuthIDs() (map[string]struct{}, bool) {
	entries, _, err := listConfiguredEntries(resolveConfigPath(p.cfg.CPAConfigPath), p.cfg.providerName())
	if err != nil {
		return nil, false
	}
	out := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Disabled {
			continue
		}
		out[entry.AuthID(p.cfg.providerName())] = struct{}{}
	}
	return out, true
}

func (p *sessionPlugin) quotaScoreLocked(authID string) float64 {
	item, ok := p.quotaByAuthID[authID]
	if !ok {
		return 0
	}
	now := time.Now()
	if !item.ExhaustedUntil.IsZero() && now.Before(item.ExhaustedUntil) {
		return -1e9
	}
	rolling := remainingFraction(item.Usage.Rolling.Percent)
	weekly := remainingFraction(item.Usage.Weekly.Percent)
	monthly := remainingFraction(item.Usage.Monthly.Percent)
	if rolling < exhaustedRemain {
		resetScore := resetSoonScore(item.Usage.Rolling.ResetsAt, now)
		return -1e6 + resetScore + weekly
	}
	return rolling*1e6 + weekly*1e3 + monthly
}

func resetSoonScore(resetsAt string, now time.Time) float64 {
	when, ok := parseResetTime(resetsAt)
	if !ok {
		return 0
	}
	until := when.Sub(now).Seconds()
	if until < 0 {
		until = 0
	}
	return 1e5 / (1 + until)
}

func parseResetTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05Z07:00"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func (p *sessionPlugin) rememberUsage(authID, masked string, usage zenUsage) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.quotaByAuthID == nil {
		p.quotaByAuthID = map[string]cachedQuota{}
	}
	item := p.quotaByAuthID[authID]
	item.Usage = usage
	item.FetchedAt = time.Now()
	if masked != "" {
		item.MaskedKey = masked
	}
	if remainingFraction(usage.Rolling.Percent) > exhaustedRemain {
		item.ExhaustedUntil = time.Time{}
	}
	p.quotaByAuthID[authID] = item
	p.quotaFetchedAt = time.Now()
}

func (p *sessionPlugin) markExhausted(authID string, until time.Time) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	if until.IsZero() {
		until = time.Now().Add(exhaustedHold)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.quotaByAuthID == nil {
		p.quotaByAuthID = map[string]cachedQuota{}
	}
	item := p.quotaByAuthID[authID]
	item.ExhaustedUntil = until
	p.quotaByAuthID[authID] = item
}

func (p *sessionPlugin) kickQuotaRefresh() {
	p.mu.Lock()
	stale := p.quotaFetchedAt.IsZero() || time.Since(p.quotaFetchedAt) > quotaCacheTTL
	if p.quotaRefreshing || !stale {
		p.mu.Unlock()
		return
	}
	p.quotaRefreshing = true
	p.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), quotaRefreshWait)
		defer cancel()
		p.refreshQuotaCache(ctx)
		p.mu.Lock()
		p.quotaRefreshing = false
		p.mu.Unlock()
	}()
}

func (p *sessionPlugin) refreshQuotaCache(ctx context.Context) {
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	entries, _, err := listConfiguredEntries(path, p.cfg.providerName())
	if err != nil || len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		if entry.Disabled {
			continue
		}
		usage, errUsage := fetchZenUsage(ctx, entry.BaseURL, entry.APIKey)
		if errUsage != nil {
			continue
		}
		p.rememberUsage(entry.AuthID(p.cfg.providerName()), maskKey(entry.APIKey), usage)
	}
}

func (p *sessionPlugin) preferredMaskedKey(entries []configuredEntry) string {
	if len(entries) == 0 {
		return ""
	}
	candidates := make([]pluginapi.SchedulerAuthCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Disabled {
			continue
		}
		candidates = append(candidates, pluginapi.SchedulerAuthCandidate{
			ID:       entry.AuthID(p.cfg.providerName()),
			Provider: "openai-compatible-" + p.cfg.providerName(),
			Attributes: map[string]string{
				"compat_name":  p.cfg.providerName(),
				"provider_key": "openai-compatible-" + p.cfg.providerName(),
				"base_url":     entry.BaseURL,
			},
		})
	}
	id := p.pickOpenCodeAuthID(candidates)
	p.mu.Lock()
	defer p.mu.Unlock()
	if item, ok := p.quotaByAuthID[id]; ok && item.MaskedKey != "" {
		return item.MaskedKey
	}
	for _, entry := range entries {
		if entry.AuthID(p.cfg.providerName()) == id {
			return maskKey(entry.APIKey)
		}
	}
	return maskKey(entries[0].APIKey)
}

func stableCompatAuthID(providerName, apiKey, baseURL, proxyURL string) string {
	kind := fmt.Sprintf("openai-compatibility:%s", strings.ToLower(strings.TrimSpace(providerName)))
	if strings.TrimSpace(providerName) == "" {
		kind = "openai-compatibility:openai-compatibility"
	}
	hasher := sha256.New()
	hasher.Write([]byte(kind))
	for _, part := range []string{apiKey, baseURL, proxyURL} {
		hasher.Write([]byte{0})
		hasher.Write([]byte(strings.TrimSpace(part)))
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if len(digest) < 12 {
		digest = fmt.Sprintf("%012s", digest)
	}
	return kind + ":" + digest[:12]
}
