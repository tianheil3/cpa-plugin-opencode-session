package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const tokensFileName = "opencode-session-tokens.json"

type tokenCounters struct {
	Requests         int64 `json:"requests"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type tokenAuthRow struct {
	tokenCounters
	Key    string `json:"key,omitempty"`
	AuthID string `json:"auth_id,omitempty"`
}

type tokenStore struct {
	Schema    int                      `json:"schema"`
	StartedAt string                   `json:"started_at"`
	UpdatedAt string                   `json:"updated_at"`
	Total     tokenCounters            `json:"total"`
	ByAuth    map[string]tokenAuthRow  `json:"by_auth"`
	ByModel   map[string]tokenCounters `json:"by_model"`
}

func newTokenStore() tokenStore {
	now := time.Now().UTC().Format(time.RFC3339)
	return tokenStore{
		Schema:    1,
		StartedAt: now,
		UpdatedAt: now,
		ByAuth:    map[string]tokenAuthRow{},
		ByModel:   map[string]tokenCounters{},
	}
}

func (c *tokenCounters) add(delta tokenCounters) {
	if c == nil {
		return
	}
	c.Requests += delta.Requests
	c.InputTokens += delta.InputTokens
	c.OutputTokens += delta.OutputTokens
	c.ReasoningTokens += delta.ReasoningTokens
	c.CacheReadTokens += delta.CacheReadTokens
	c.CacheWriteTokens += delta.CacheWriteTokens
	c.TotalTokens += delta.TotalTokens
}

func cacheHitDenom(c tokenCounters) int64 {
	if c.InputTokens <= 0 {
		return 0
	}
	if c.CacheReadTokens > c.InputTokens {
		sum := c.InputTokens + c.CacheReadTokens + c.CacheWriteTokens
		if sum > 0 {
			return sum
		}
	}
	return c.InputTokens
}

func cacheHitRate(c tokenCounters) *float64 {
	denom := cacheHitDenom(c)
	if denom <= 0 {
		return nil
	}
	rate := float64(c.CacheReadTokens) / float64(denom)
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &rate
}

func formatTokenCount(n int64) string {
	if n < 0 {
		n = 0
	}
	switch {
	case n >= 100000000:
		return fmt.Sprintf("%.2f 亿", float64(n)/1e8)
	case n >= 10000:
		return fmt.Sprintf("%.2f 万", float64(n)/1e4)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatHitRate(c tokenCounters) string {
	rate := cacheHitRate(c)
	if rate == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", *rate*100)
}

func usageDelta(detail pluginapi.UsageDetail) tokenCounters {
	cacheRead := detail.CacheReadTokens
	if cacheRead == 0 && detail.CachedTokens > 0 {
		cacheRead = detail.CachedTokens
	}
	cacheWrite := detail.CacheCreationTokens
	input := detail.InputTokens
	output := detail.OutputTokens
	total := detail.TotalTokens
	if total == 0 {
		total = input + output
		if cacheRead > input {
			total = input + cacheRead + cacheWrite + output
		}
	}
	return tokenCounters{
		Requests:         1,
		InputTokens:      input,
		OutputTokens:     output,
		ReasoningTokens:  detail.ReasoningTokens,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		TotalTokens:      total,
	}
}

func countersMap(c tokenCounters) map[string]any {
	out := map[string]any{
		"requests":            c.Requests,
		"input_tokens":        c.InputTokens,
		"output_tokens":       c.OutputTokens,
		"reasoning_tokens":    c.ReasoningTokens,
		"cache_read_tokens":   c.CacheReadTokens,
		"cache_write_tokens":  c.CacheWriteTokens,
		"total_tokens":        c.TotalTokens,
		"total_display":       formatTokenCount(c.TotalTokens),
		"input_display":       formatTokenCount(c.InputTokens),
		"output_display":      formatTokenCount(c.OutputTokens),
		"cache_read_display":  formatTokenCount(c.CacheReadTokens),
		"cache_write_display": formatTokenCount(c.CacheWriteTokens),
		"cache_hit_display":   formatHitRate(c),
	}
	if rate := cacheHitRate(c); rate != nil {
		out["cache_hit_rate"] = *rate
	}
	return out
}

func (s tokenStore) toMap() map[string]any {
	auths := make([]tokenAuthRow, 0, len(s.ByAuth))
	for id, row := range s.ByAuth {
		row.AuthID = id
		auths = append(auths, row)
	}
	sort.Slice(auths, func(i, j int) bool {
		if auths[i].TotalTokens == auths[j].TotalTokens {
			return auths[i].AuthID < auths[j].AuthID
		}
		return auths[i].TotalTokens > auths[j].TotalTokens
	})
	authMaps := make([]map[string]any, 0, len(auths))
	for _, row := range auths {
		item := countersMap(row.tokenCounters)
		item["id"] = row.AuthID
		item["key"] = row.Key
		authMaps = append(authMaps, item)
	}
	models := make([]string, 0, len(s.ByModel))
	for name := range s.ByModel {
		models = append(models, name)
	}
	sort.Slice(models, func(i, j int) bool {
		a, b := s.ByModel[models[i]], s.ByModel[models[j]]
		if a.TotalTokens == b.TotalTokens {
			return models[i] < models[j]
		}
		return a.TotalTokens > b.TotalTokens
	})
	modelMaps := make([]map[string]any, 0, len(models))
	for _, name := range models {
		item := countersMap(s.ByModel[name])
		item["model"] = name
		modelMaps = append(modelMaps, item)
	}
	out := countersMap(s.Total)
	out["schema"] = s.Schema
	out["started_at"] = s.StartedAt
	out["updated_at"] = s.UpdatedAt
	out["by_auth"] = authMaps
	out["by_model"] = modelMaps
	return out
}

func (p *sessionPlugin) tokensPath() string {
	return filepath.Join(filepath.Dir(resolveConfigPath(p.cfg.CPAConfigPath)), tokensFileName)
}

func (p *sessionPlugin) ensureTokensLocked() {
	if p.tokensLoaded {
		return
	}
	p.tokensLoaded = true
	raw, err := os.ReadFile(p.tokensPath())
	if err != nil {
		p.tokens = newTokenStore()
		return
	}
	var store tokenStore
	if json.Unmarshal(raw, &store) != nil {
		p.tokens = newTokenStore()
		return
	}
	if store.ByAuth == nil {
		store.ByAuth = map[string]tokenAuthRow{}
	}
	if store.ByModel == nil {
		store.ByModel = map[string]tokenCounters{}
	}
	if strings.TrimSpace(store.StartedAt) == "" {
		store.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if store.Schema == 0 {
		store.Schema = 1
	}
	p.tokens = store
}

func (p *sessionPlugin) recordTokens(authID, model, maskedKey string, detail pluginapi.UsageDetail) {
	delta := usageDelta(detail)
	if delta.TotalTokens == 0 && delta.InputTokens == 0 && delta.OutputTokens == 0 && delta.CacheReadTokens == 0 {
		delta.Requests = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.mu.Lock()
	p.ensureTokensLocked()
	if p.tokens.ByAuth == nil {
		p.tokens.ByAuth = map[string]tokenAuthRow{}
	}
	if p.tokens.ByModel == nil {
		p.tokens.ByModel = map[string]tokenCounters{}
	}
	p.tokens.Total.add(delta)
	p.tokens.UpdatedAt = now
	if p.tokens.StartedAt == "" {
		p.tokens.StartedAt = now
	}
	authID = strings.TrimSpace(authID)
	if authID != "" {
		row := p.tokens.ByAuth[authID]
		row.add(delta)
		if masked := strings.TrimSpace(maskedKey); masked != "" {
			row.Key = masked
		}
		p.tokens.ByAuth[authID] = row
	}
	model = strings.TrimSpace(model)
	if model != "" {
		row := p.tokens.ByModel[model]
		row.add(delta)
		p.tokens.ByModel[model] = row
	}
	p.schedulePersistLocked()
	p.mu.Unlock()
}

func (p *sessionPlugin) schedulePersistLocked() {
	p.tokensDirty = true
	if p.persistTimer != nil {
		p.persistTimer.Reset(2 * time.Second)
		return
	}
	p.persistTimer = time.AfterFunc(2*time.Second, p.flushTokens)
}

func (p *sessionPlugin) flushTokens() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.persistTimer != nil {
		p.persistTimer.Stop()
		p.persistTimer = nil
	}
	if !p.tokensDirty {
		return
	}
	p.ensureTokensLocked()
	raw, err := json.MarshalIndent(p.tokens, "", "  ")
	path := p.tokensPath()
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		return
	}
	p.tokensDirty = false
}

func (p *sessionPlugin) resetTokens() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.persistTimer != nil {
		p.persistTimer.Stop()
		p.persistTimer = nil
	}
	path := p.tokensPath()
	p.tokens = newTokenStore()
	p.tokensLoaded = true
	p.tokensDirty = false
	_ = os.Remove(path)
	_ = os.Remove(path + ".tmp")
}

func (p *sessionPlugin) tokensPayload() map[string]any {
	p.mu.Lock()
	p.ensureTokensLocked()
	out := p.tokens.toMap()
	p.mu.Unlock()
	return out
}

func (p *sessionPlugin) tokensForAuth(authID string) map[string]any {
	authID = strings.TrimSpace(authID)
	p.mu.Lock()
	p.ensureTokensLocked()
	row, ok := p.tokens.ByAuth[authID]
	p.mu.Unlock()
	if !ok {
		return countersMap(tokenCounters{})
	}
	return countersMap(row.tokenCounters)
}

func (p *sessionPlugin) maskedKeyForAuth(authID string) string {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if row, ok := p.quotaByAuthID[authID]; ok {
		return row.MaskedKey
	}
	if p.tokensLoaded {
		if row, ok := p.tokens.ByAuth[authID]; ok {
			return row.Key
		}
	}
	return ""
}

func (p *sessionPlugin) isOpenCodeUsage(record pluginapi.UsageRecord) bool {
	if isOpenCodeQuotaRequest(pluginapi.QuotaFetchRequest{
		Provider: record.Provider,
		AuthID:   record.AuthID,
		Attributes: map[string]string{
			"base_url": record.BaseURL,
		},
	}, p.cfg.providerName()) {
		return true
	}
	if looksLikeOpenCode(record.Provider, record.BaseURL, record.Model) {
		return true
	}
	authID := strings.TrimSpace(record.AuthID)
	if authID == "" {
		return false
	}
	_, ids := p.openCodeInterceptTargets()
	_, ok := ids[authID]
	return ok
}
