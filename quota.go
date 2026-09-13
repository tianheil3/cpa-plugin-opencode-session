package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type quotaAdapter struct {
	p *sessionPlugin
}

func (q *quotaAdapter) Identifier() string {
	return quotaProviderID
}

func (q *quotaAdapter) DescribeQuota(context.Context, pluginapi.QuotaDescribeRequest) (pluginapi.QuotaDescribeResponse, error) {
	return pluginapi.QuotaDescribeResponse{
		SupportedProviders: quotaProviderAliases(q.p.cfg.providerName()),
		DisplayName:        "OpenCode Go",
		SupportsReset:      false,
	}, nil
}

func (q *quotaAdapter) FetchQuota(ctx context.Context, req pluginapi.QuotaFetchRequest) (pluginapi.QuotaFetchResponse, error) {
	if !isOpenCodeQuotaRequest(req, q.p.cfg.providerName()) {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("credential is not OpenCode Go")
	}
	apiKey := apiKeyFromQuotaRequest(req)
	if apiKey == "" {
		keys, _, err := listConfiguredKeys(resolveConfigPath(q.p.cfg.CPAConfigPath), q.p.cfg.providerName())
		if err == nil && len(keys) > 0 {
			apiKey = keys[0]
		}
	}
	if apiKey == "" {
		return pluginapi.QuotaFetchResponse{}, fmt.Errorf("no OpenCode Go API key on this credential")
	}
	usage, err := fetchZenUsage(ctx, q.p.cfg.zenBaseURL(), apiKey)
	if err != nil {
		return pluginapi.QuotaFetchResponse{}, err
	}
	return usageToQuota(usage), nil
}

func (q *quotaAdapter) ResetQuota(context.Context, pluginapi.QuotaResetRequest) (pluginapi.QuotaResetResponse, error) {
	return pluginapi.QuotaResetResponse{
		Success: false,
		Message: "OpenCode Go quotas reset on the official rolling/weekly/monthly windows; they cannot be cleared locally.",
	}, nil
}

func usageToQuota(usage zenUsage) pluginapi.QuotaFetchResponse {
	bucket := func(window string, w zenUsageWindow) pluginapi.QuotaBucket {
		remain := remainingFraction(w.Percent) * 100
		desc := fmt.Sprintf("%s · remaining %.0f%%", strings.TrimSpace(w.Status), remain)
		return pluginapi.QuotaBucket{
			Window:            window,
			RemainingFraction: remainingFraction(w.Percent),
			ResetTime:         w.ResetsAt,
			Description:       desc,
		}
	}
	return pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{Plan: "OpenCode Go", TierName: "Go"},
		Groups: []pluginapi.QuotaGroup{{
			DisplayName: "OpenCode Go",
			Buckets: []pluginapi.QuotaBucket{
				bucket("5h", usage.Rolling),
				bucket("7d", usage.Weekly),
				bucket("30d", usage.Monthly),
			},
		}},
	}
}

func quotaProviderAliases(providerName string) []string {
	name := strings.ToLower(strings.TrimSpace(providerName))
	if name == "" {
		name = quotaProviderID
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(name)
	add(quotaProviderID)
	add("openai-compatibility:" + name)
	add("openai-compatible-" + name)
	add("openai-compatibility:" + quotaProviderID)
	add("openai-compatible-" + quotaProviderID)
	return out
}

func isOpenCodeQuotaRequest(req pluginapi.QuotaFetchRequest, providerName string) bool {
	aliases := quotaProviderAliases(providerName)
	in := func(s string) bool {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return false
		}
		for _, alias := range aliases {
			if s == alias {
				return true
			}
		}
		return false
	}
	if in(req.Provider) {
		return true
	}
	if req.Attributes != nil {
		if in(req.Attributes["compat_name"]) || in(req.Attributes["provider_key"]) {
			return true
		}
		base := strings.ToLower(req.Attributes["base_url"] + " " + req.Attributes["base-url"])
		if strings.Contains(base, "opencode.ai/zen/go") {
			return true
		}
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	return strings.Contains(provider, "opencode-go")
}

func apiKeyFromQuotaRequest(req pluginapi.QuotaFetchRequest) string {
	if req.Attributes != nil {
		for _, key := range []string{"api_key", "api-key", "token"} {
			if v := strings.TrimSpace(req.Attributes[key]); v != "" {
				return v
			}
		}
	}
	if req.Metadata != nil {
		for _, key := range []string{"api_key", "api-key", "token"} {
			if v, ok := req.Metadata[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if len(req.StorageJSON) > 0 {
		var payload map[string]any
		if json.Unmarshal(req.StorageJSON, &payload) == nil {
			for _, key := range []string{"api_key", "api-key", "token"} {
				if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}
