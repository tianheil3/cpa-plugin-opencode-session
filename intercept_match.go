package main

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (p *sessionPlugin) shouldInjectSession(req pluginapi.RequestInterceptRequest) bool {
	models, authIDs := p.openCodeInterceptTargets()
	return shouldInjectSession(req, p.cfg.providerName(), models, authIDs)
}

func (p *sessionPlugin) openCodeInterceptTargets() ([]string, map[string]struct{}) {
	entries, models, err := listConfiguredEntries(resolveConfigPath(p.cfg.CPAConfigPath), p.cfg.providerName())
	if err != nil {
		return nil, nil
	}
	authIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if id := strings.TrimSpace(entry.AuthID(p.cfg.providerName())); id != "" {
			authIDs[id] = struct{}{}
		}
	}
	return models, authIDs
}

func shouldInjectSession(req pluginapi.RequestInterceptRequest, providerName string, models []string, authIDs map[string]struct{}) bool {
	afterAuth := strings.TrimSpace(req.ToFormat) != ""
	if afterAuth {
		if id := metadataString(req.Metadata, "selected_auth_id", "pinned_auth_id", "auth_id"); id != "" {
			if _, ok := authIDs[id]; ok {
				return true
			}
			if len(authIDs) > 0 {
				return false
			}
		}
		provider := metadataString(req.Metadata, "session_affinity_provider", "provider", "compat_name", "provider_key")
		if provider != "" {
			return isOpenCodeQuotaRequest(pluginapi.QuotaFetchRequest{
				Provider:   provider,
				Attributes: metadataAttributes(req.Metadata),
				AuthID:     metadataString(req.Metadata, "selected_auth_id", "pinned_auth_id", "auth_id"),
			}, providerName)
		}
	}
	return modelInOpenCodeList(req.Model, req.RequestedModel, models)
}

func modelInOpenCodeList(model, requested string, models []string) bool {
	if len(models) == 0 {
		return false
	}
	for _, name := range []string{model, requested} {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, have := range models {
			if strings.EqualFold(name, have) {
				return true
			}
		}
	}
	return false
}

func metadataString(meta map[string]any, keys ...string) string {
	if len(meta) == 0 {
		return ""
	}
	for _, key := range keys {
		raw, ok := meta[key]
		if !ok || raw == nil {
			continue
		}
		if s, ok := raw.(string); ok {
			if v := strings.TrimSpace(s); v != "" {
				return v
			}
		}
	}
	return ""
}

func metadataAttributes(meta map[string]any) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, key := range []string{"compat_name", "provider_key", "base_url", "base-url", "provider"} {
		if v := metadataString(meta, key); v != "" {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
