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
	name := q.p.cfg.providerName()
	supported := []string{name, quotaProviderID, "openai-compatibility:" + name}
	if name != quotaProviderID {
		supported = append(supported, "openai-compatibility:"+quotaProviderID)
	}
	return pluginapi.QuotaDescribeResponse{
		SupportedProviders: supported,
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
		desc := fmt.Sprintf("%s %g%% used", w.Status, w.Percent)
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

func isOpenCodeQuotaRequest(req pluginapi.QuotaFetchRequest, providerName string) bool {
	name := strings.ToLower(strings.TrimSpace(providerName))
	if name == "" {
		name = quotaProviderID
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" || provider == name || provider == quotaProviderID ||
		provider == "openai-compatibility:"+name || provider == "openai-compatibility:"+quotaProviderID {
		return true
	}
	compat := ""
	base := ""
	if req.Attributes != nil {
		compat = strings.ToLower(strings.TrimSpace(req.Attributes["compat_name"]))
		base = strings.ToLower(req.Attributes["base_url"] + " " + req.Attributes["base-url"])
	}
	if compat == name || compat == quotaProviderID {
		return true
	}
	if strings.Contains(base, "opencode.ai/zen/go") || strings.Contains(provider, "opencode") {
		return true
	}
	return false
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
