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
			{Method: http.MethodPost, Path: "/plugins/opencode-session/connect", Handler: p},
			{Method: http.MethodPost, Path: "/plugins/opencode-session/sync-models", Handler: p},
			{Method: http.MethodPost, Path: "/plugins/opencode-session/refresh", Handler: p},
		},
		Resources: []pluginapi.ResourceRoute{
			{
				Path:        "/status",
				Menu:        "OpenCode Go",
				Description: "Connect an OpenCode Go key, sync models, and watch 5h/weekly/monthly quota.",
				Handler:     p,
			},
		},
	}, nil
}

func (p *sessionPlugin) HandleManagement(ctx context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	path := strings.ToLower(req.Path)
	switch {
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/status") && strings.Contains(path, "/resource/"):
		return htmlResponse(dashboardHTML), nil
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/plugins/opencode-session/status"):
		return p.handleStatus(ctx)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/plugins/opencode-session/connect"):
		return p.handleConnect(ctx, req.Body)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/plugins/opencode-session/sync-models"):
		return p.handleSyncModels(ctx, req.Body)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/plugins/opencode-session/refresh"):
		return p.handleStatus(ctx)
	default:
		if req.Method == http.MethodGet && (strings.HasSuffix(path, "/status") || strings.HasSuffix(path, "/")) {
			return htmlResponse(dashboardHTML), nil
		}
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "unknown route"}), nil
	}
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
	if _, err := fetchZenUsage(ctx, p.cfg.zenBaseURL(), apiKey); err != nil {
		return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()}), nil
	}
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	if err := upsertOpenCodeProvider(path, p.cfg.providerName(), p.cfg.zenBaseURL(), apiKey, models, includeClaude); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"ok":            true,
		"provider":      p.cfg.providerName(),
		"models":        len(models),
		"key":           maskKey(apiKey),
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
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	keys, models, err := listConfiguredKeys(path, p.cfg.providerName())
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	accounts := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		item := map[string]any{"key": maskKey(key)}
		usage, errUsage := fetchZenUsage(ctx, p.cfg.zenBaseURL(), key)
		if errUsage != nil {
			item["error"] = errUsage.Error()
		} else {
			item["usage"] = usage
			item["quota"] = usageToQuota(usage)
		}
		accounts = append(accounts, item)
	}
	p.mu.Lock()
	failure := p.lastFailure
	p.mu.Unlock()
	return jsonResponse(http.StatusOK, map[string]any{
		"ok":         true,
		"provider":   p.cfg.providerName(),
		"base_url":   p.cfg.zenBaseURL(),
		"models":     models,
		"modelCount": len(models),
		"accounts":   accounts,
		"session":    true,
		"version":    pluginVersion,
		"failure":    failure,
		"fetchedAt":  time.Now().UTC().Format(time.RFC3339),
	}), nil
}

func htmlResponse(body string) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"text/html; charset=utf-8"},
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
