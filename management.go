package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (p *sessionPlugin) RegisterManagement(_ context.Context, _ pluginapi.ManagementRegistrationRequest) (pluginapi.ManagementRegistrationResponse, error) {
	return pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{Method: http.MethodGet, Path: "/plugins/opencode-session/status", Handler: p},
			{Method: http.MethodGet, Path: "/plugins/opencode-session/auth-files", Handler: p},
			{Method: http.MethodPatch, Path: "/plugins/opencode-session/auth-files/status", Handler: p},
			{Method: http.MethodDelete, Path: "/plugins/opencode-session/auth-files", Handler: p},
			{Method: http.MethodPost, Path: "/plugins/opencode-session/connect", Handler: p},
			{Method: http.MethodPost, Path: "/plugins/opencode-session/sync-models", Handler: p},
			{Method: http.MethodPost, Path: "/plugins/opencode-session/refresh", Handler: p},
		},
		Resources: []pluginapi.ResourceRoute{
			{
				Path:        "/status",
				Menu:        "OpenCode Go",
				Description: "Connect multiple OpenCode Go subscriptions; requests pick the key with the most remaining quota.",
				Handler:     p,
			},
		},
	}, nil
}

func (p *sessionPlugin) HandleManagement(ctx context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	path := strings.ToLower(req.Path)
	switch {
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/status") && strings.Contains(path, "/resource/"):
		return htmlResponse(p.dashboardPage(ctx)), nil
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/plugins/opencode-session/status"):
		return p.handleStatus(ctx)
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/plugins/opencode-session/auth-files"):
		return p.handleAuthFiles(ctx)
	case (req.Method == http.MethodPatch || req.Method == http.MethodPost) && strings.HasSuffix(path, "/plugins/opencode-session/auth-files/status"):
		return p.handleAuthFilesStatus(ctx, req.Body)
	case req.Method == http.MethodDelete && strings.HasSuffix(path, "/plugins/opencode-session/auth-files"):
		return p.handleAuthFilesDelete(ctx, req)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/plugins/opencode-session/connect"):
		return p.handleConnect(ctx, req.Body)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/plugins/opencode-session/sync-models"):
		return p.handleSyncModels(ctx, req.Body)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/plugins/opencode-session/refresh"):
		return p.handleStatus(ctx)
	default:
		if req.Method == http.MethodGet && (strings.HasSuffix(path, "/status") || strings.HasSuffix(path, "/")) {
			return htmlResponse(p.dashboardPage(ctx)), nil
		}
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "unknown route"}), nil
	}
}

func (p *sessionPlugin) dashboardPage(ctx context.Context) string {
	payload, err := p.statusPayload(ctx)
	if err != nil {
		payload = map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"version": pluginVersion,
		}
	}
	raw, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		raw = []byte("null")
	}
	return strings.Replace(dashboardHTML, "window.__OPENCODE_STATUS__ = null;", "window.__OPENCODE_STATUS__ = "+string(raw)+";", 1)
}

type connectRequest struct {
	APIKey        string `json:"api_key"`
	IncludeClaude bool   `json:"include_claude"`
}

func (p *sessionPlugin) handleConnect(ctx context.Context, body []byte) (pluginapi.ManagementResponse, error) {
	var req connectRequest
	_ = json.Unmarshal(body, &req)
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "api_key is required"}), nil
	}
	includeClaude := req.IncludeClaude || p.cfg.IncludeClaude
	models, err := fetchZenModels(ctx, p.cfg.zenBaseURL(), apiKey)
	if err != nil {
		return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()}), nil
	}
	usage, errUsage := fetchZenUsage(ctx, p.cfg.zenBaseURL(), apiKey)
	if errUsage != nil {
		return jsonResponse(http.StatusBadGateway, map[string]any{"error": errUsage.Error()}), nil
	}
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	if err := upsertOpenCodeProvider(path, p.cfg.providerName(), p.cfg.zenBaseURL(), apiKey, models, includeClaude); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	entries, _, _ := listConfiguredEntries(path, p.cfg.providerName())
	authID := stableCompatAuthID(p.cfg.providerName(), apiKey, p.cfg.zenBaseURL(), "direct")
	for _, entry := range entries {
		if entry.APIKey == apiKey {
			authID = entry.AuthID(p.cfg.providerName())
			break
		}
	}
	p.rememberUsage(authID, maskKey(apiKey), usage)
	return jsonResponse(http.StatusOK, map[string]any{
		"ok":            true,
		"provider":      p.cfg.providerName(),
		"models":        len(models),
		"key":           maskKey(apiKey),
		"pool_size":     len(entries),
		"includeClaude": includeClaude,
	}), nil
}

func (p *sessionPlugin) handleSyncModels(ctx context.Context, body []byte) (pluginapi.ManagementResponse, error) {
	var req connectRequest
	_ = json.Unmarshal(body, &req)
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	keys, _, err := listConfiguredKeys(path, p.cfg.providerName())
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" && len(keys) > 0 {
		apiKey = keys[0]
	}
	if apiKey == "" {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "no OpenCode Go API key configured"}), nil
	}
	models, err := fetchZenModels(ctx, p.cfg.zenBaseURL(), apiKey)
	if err != nil {
		return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()}), nil
	}
	if err := upsertOpenCodeProvider(path, p.cfg.providerName(), p.cfg.zenBaseURL(), apiKey, models, false); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{"ok": true, "models": models}), nil
}

func (p *sessionPlugin) handleStatus(ctx context.Context) (pluginapi.ManagementResponse, error) {
	payload, err := p.statusPayload(ctx)
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, payload), nil
}

func (p *sessionPlugin) statusPayload(ctx context.Context) (map[string]any, error) {
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	entries, models, err := listConfiguredEntries(path, p.cfg.providerName())
	if err != nil {
		return nil, err
	}
	accounts := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		masked := maskKey(entry.APIKey)
		item := map[string]any{
			"key":  masked,
			"id":   entry.AuthID(p.cfg.providerName()),
			"name": opencodeAuthFileName(entry, p.cfg.providerName()),
		}
		base := entry.BaseURL
		if base == "" {
			base = p.cfg.zenBaseURL()
		}
		usage, errUsage := fetchZenUsage(ctx, base, entry.APIKey)
		if errUsage != nil {
			item["error"] = errUsage.Error()
		} else {
			item["usage"] = usage
			item["quota"] = usageToQuota(usage)
			p.rememberUsage(entry.AuthID(p.cfg.providerName()), masked, usage)
		}
		accounts = append(accounts, item)
	}
	preferred := p.preferredMaskedKey(entries)
	for _, item := range accounts {
		item["preferred"] = item["key"] == preferred
	}
	p.mu.Lock()
	failure := p.lastFailure
	p.mu.Unlock()
	return map[string]any{
		"ok":         true,
		"provider":   p.cfg.providerName(),
		"base_url":   p.cfg.zenBaseURL(),
		"models":     models,
		"modelCount": len(models),
		"accounts":   accounts,
		"pool": map[string]any{
			"size":      len(entries),
			"preferred": preferred,
			"strategy":  "quota",
		},
		"session":   true,
		"version":   pluginVersion,
		"failure":   failure,
		"fetchedAt": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func htmlResponse(body string) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type":              {"text/html; charset=utf-8"},
			"Cache-Control":             {"no-store, no-cache, must-revalidate"},
			"Pragma":                    {"no-cache"},
			"X-Opencode-Session-Plugin": {pluginVersion},
		},
		Body: []byte(body),
	}
}

type abiManagementRoute struct {
	Method      string `json:"Method,omitempty"`
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type abiResourceRoute struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu,omitempty"`
	Description string `json:"Description,omitempty"`
}

type abiManagementRegistrationResponse struct {
	Routes    []abiManagementRoute `json:"routes,omitempty"`
	Resources []abiResourceRoute   `json:"resources,omitempty"`
}

func toABIManagementRegistration(resp pluginapi.ManagementRegistrationResponse) abiManagementRegistrationResponse {
	out := abiManagementRegistrationResponse{
		Routes:    make([]abiManagementRoute, 0, len(resp.Routes)),
		Resources: make([]abiResourceRoute, 0, len(resp.Resources)),
	}
	for _, route := range resp.Routes {
		out.Routes = append(out.Routes, abiManagementRoute{
			Method:      route.Method,
			Path:        route.Path,
			Menu:        route.Menu,
			Description: route.Description,
		})
	}
	for _, route := range resp.Resources {
		out.Resources = append(out.Resources, abiResourceRoute{
			Path:        route.Path,
			Menu:        route.Menu,
			Description: route.Description,
		})
	}
	return out
}

func jsonResponse(status int, v any) pluginapi.ManagementResponse {
	raw, _ := json.Marshal(v)
	return pluginapi.ManagementResponse{
		StatusCode: status,
		Headers: http.Header{
			"Content-Type": {"application/json; charset=utf-8"},
		},
		Body: raw,
	}
}
