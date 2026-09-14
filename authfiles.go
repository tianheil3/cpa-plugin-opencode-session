package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func authFileModelObjects(models []string) []map[string]any {
	out := make([]map[string]any, 0, len(models))
	for _, model := range models {
		if model == "" {
			continue
		}
		out = append(out, map[string]any{
			"id":           model,
			"display_name": model,
			"type":         "openai-compatibility",
			"owned_by":     "opencode-go",
		})
	}
	return out
}

func (p *sessionPlugin) handleAuthFiles(_ context.Context) (pluginapi.ManagementResponse, error) {
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	entries, models, err := listConfiguredEntries(path, p.cfg.providerName())
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	modelObjs := authFileModelObjects(models)
	preferred := p.preferredMaskedKey(entries)
	files := make([]map[string]any, 0, len(entries))
	now := time.Now()
	provider := p.cfg.providerName()
	for _, entry := range entries {
		authID := entry.AuthID(provider)
		masked := maskKey(entry.APIKey)
		status, message := "active", ""
		if entry.Disabled {
			status = "disabled"
		}
		p.mu.Lock()
		item, ok := p.quotaByAuthID[authID]
		p.mu.Unlock()
		if !entry.Disabled && ok && !item.ExhaustedUntil.IsZero() && now.Before(item.ExhaustedUntil) {
			message = "quota exhausted"
		}
		file := map[string]any{
			"id":             authID,
			"name":           opencodeAuthFileName(entry, provider),
			"type":           "opencode-go",
			"provider":       "opencode-go",
			"label":          "OpenCode Go",
			"email":          masked,
			"status":         status,
			"status_message": message,
			"disabled":       entry.Disabled,
			"unavailable":    false,
			"runtime_only":   true,
			"source":         "memory",
			"size":           int64(0),
			"success":        0,
			"failed":         0,
			"models":         modelObjs,
			"supports_quota": true,
			"quota_provider": "opencode-go",
		}
		if masked == preferred && len(entries) > 1 && !entry.Disabled {
			file["note"] = "当前选用"
		}
		files = append(files, file)
	}
	return jsonResponse(http.StatusOK, map[string]any{"ok": true, "files": files}), nil
}

type authFileStatusRequest struct {
	Name     string `json:"name"`
	Disabled *bool  `json:"disabled"`
}

func (p *sessionPlugin) handleAuthFilesStatus(_ context.Context, body []byte) (pluginapi.ManagementResponse, error) {
	var req authFileStatusRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid request body"}), nil
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "name is required"}), nil
	}
	if req.Disabled == nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "disabled is required"}), nil
	}
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	if err := setConfiguredEntryDisabled(path, p.cfg.providerName(), name, *req.Disabled); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		return jsonResponse(status, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{"status": "ok", "disabled": *req.Disabled}), nil
}

type authFileDeleteRequest struct {
	Name  string   `json:"name"`
	Names []string `json:"names"`
}

func (p *sessionPlugin) handleAuthFilesDelete(_ context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	names := make([]string, 0, 4)
	var payload authFileDeleteRequest
	if len(req.Body) > 0 {
		_ = json.Unmarshal(req.Body, &payload)
	}
	if strings.TrimSpace(payload.Name) != "" {
		names = append(names, payload.Name)
	}
	names = append(names, payload.Names...)
	if req.Query != nil {
		names = append(names, req.Query["name"]...)
		names = append(names, req.Query["names"]...)
	}
	cleaned := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, name)
	}
	if len(cleaned) == 0 {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "name is required"}), nil
	}
	path := resolveConfigPath(p.cfg.CPAConfigPath)
	deleted, failed, err := deleteConfiguredEntries(path, p.cfg.providerName(), cleaned)
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	status := "ok"
	code := http.StatusOK
	if len(deleted) == 0 && len(failed) > 0 {
		status = "error"
		code = http.StatusNotFound
	} else if len(failed) > 0 {
		status = "partial"
	}
	return jsonResponse(code, map[string]any{
		"status":  status,
		"deleted": len(deleted),
		"files":   deleted,
		"failed":  failed,
	}), nil
}
