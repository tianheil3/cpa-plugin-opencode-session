package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (p *sessionPlugin) HandleUsage(_ context.Context, record pluginapi.UsageRecord) {
	if !record.Failed || record.Failure.StatusCode != http.StatusTooManyRequests {
		return
	}
	if !looksLikeOpenCode(record.Provider, record.BaseURL, record.Model) {
		return
	}
	p.mu.Lock()
	p.lastFailure = &quotaEvent{
		At:      record.RequestedAt,
		Model:   record.Model,
		Status:  record.Failure.StatusCode,
		Message: summarizeBody([]byte(record.Failure.Body)),
	}
	p.mu.Unlock()
}

func looksLikeOpenCode(provider, baseURL, model string) bool {
	blob := strings.ToLower(provider + " " + baseURL + " " + model)
	return strings.Contains(blob, "opencode") || strings.Contains(blob, "zen/go")
}
