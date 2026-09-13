package main

import (
	"context"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	pluginID      = "opencode-session"
	pluginName    = "OpenCode Session"
	pluginAuthor  = "tianheil3"
	pluginRepoURL = "https://github.com/tianheil3/cpa-plugin-opencode-session"
)

type sessionPlugin struct {
	cfg pluginConfig
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
	sid := sessionIDFrom(req.Headers, req.Body, req.Metadata)
	resp.Headers = make(http.Header, 2)
	resp.Headers.Set(sessionHeader, sid)
	resp.Headers.Set("Session-Id", sid)
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
