package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (p *sessionPlugin) HandleUsage(_ context.Context, record pluginapi.UsageRecord) {
	if !record.Failed {
		return
	}
	if !looksLikeOpenCode(record.Provider, record.BaseURL, record.Model) {
		return
	}
	quotaHit := record.Failure.StatusCode == http.StatusTooManyRequests || looksLikeQuotaBody(record.Failure.Body)
	p.mu.Lock()
	p.lastFailure = &quotaEvent{
		At:      record.RequestedAt,
		Model:   record.Model,
		Status:  record.Failure.StatusCode,
		Message: summarizeBody([]byte(record.Failure.Body)),
	}
	p.mu.Unlock()
	if quotaHit {
		authID := strings.TrimSpace(record.AuthID)
		if authID == "" {
			p.mu.Lock()
			authID = p.lastPickAuthID
			p.mu.Unlock()
		}
		p.markExhausted(authID, time.Time{})
	}
}

func looksLikeQuotaBody(body string) bool {
	s := strings.ToLower(body)
	return strings.Contains(s, "quota") || strings.Contains(s, "rate limit") || strings.Contains(s, "rate_limit") || strings.Contains(s, "usage limit")
}

func looksLikeOpenCode(provider, baseURL, model string) bool {
	blob := strings.ToLower(provider + " " + baseURL + " " + model)
	return strings.Contains(blob, "opencode") || strings.Contains(blob, "zen/go")
}
