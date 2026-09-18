package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	pluginID          = "opencode-session"
	pluginName        = "OpenCode Go"
	pluginAuthor      = "tianheil3"
	pluginRepoURL     = "https://github.com/tianheil3/cpa-plugin-opencode-session"
	quotaProviderID   = "opencode-go"
	defaultZenBaseURL = "https://opencode.ai/zen/go/v1"
	defaultClaudeURL  = "https://opencode.ai/zen/go"
)

type sessionPlugin struct {
	cfg pluginConfig

	mu              sync.Mutex
	lastFailure     *quotaEvent
	quotaByAuthID   map[string]cachedQuota
	quotaFetchedAt  time.Time
	quotaRefreshing bool
	lastPickAuthID  string
	tokens          tokenStore
	tokensLoaded    bool
	tokensDirty     bool
	persistTimer    *time.Timer
}

type cachedQuota struct {
	Usage          zenUsage
	FetchedAt      time.Time
	ExhaustedUntil time.Time
	MaskedKey      string
}

type quotaEvent struct {
	At      time.Time `json:"at"`
	Model   string    `json:"model,omitempty"`
	Status  int       `json:"status,omitempty"`
	Message string    `json:"message,omitempty"`
}

var _ pluginapi.RequestInterceptor = (*sessionPlugin)(nil)

func (p *sessionPlugin) Identifier() string {
	return pluginID
}

func (p *sessionPlugin) InterceptRequestBeforeAuth(ctx context.Context, req pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error) {
	return p.intercept(req)
}

func (p *sessionPlugin) InterceptRequestAfterAuth(ctx context.Context, req pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error) {
	return p.intercept(req)
}

func (p *sessionPlugin) intercept(req pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error) {
	resp := pluginapi.RequestInterceptResponse{}
	if !p.shouldInjectSession(req) {
		if strings.TrimSpace(req.ToFormat) != "" {
			models, _ := p.openCodeInterceptTargets()
			if modelInOpenCodeList(req.Model, req.RequestedModel, models) {
				resp.ClearHeaders = []string{sessionHeader, "X-Opencode-Session"}
			}
		}
		return resp, nil
	}
	sid := sessionIDFrom(req.Headers, req.Body, req.Metadata)
	resp.Headers = make(http.Header, 1)
	resp.Headers.Set(sessionHeader, sid)
	model := req.Model
	if model == "" {
		model = req.RequestedModel
	}
	if p.cfg.shouldRewrite(model, req.RequestedModel) {
		if body, ok := rewriteBody(model, req.Body, p.cfg); ok {
			resp.Body = body
		}
	}
	return resp, nil
}
