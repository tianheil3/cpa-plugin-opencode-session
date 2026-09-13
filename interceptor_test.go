package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestInterceptInjectsSessionFromCodexHeader(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.InterceptRequestAfterAuth(context.Background(), pluginapi.RequestInterceptRequest{
		Model: "deepseek-v4.1-flash",
		Headers: map[string][]string{
			"Session-Id": {"mac-codex-1"},
		},
		Body: []byte(`{"model":"deepseek-v4.1-flash","input":"hi"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Headers.Get(sessionHeader); got != "mac-codex-1" {
		t.Fatalf("x-opencode-session = %q, want mac-codex-1", got)
	}
}

func TestInterceptRewritesBody(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.InterceptRequestBeforeAuth(context.Background(), pluginapi.RequestInterceptRequest{
		Model: "deepseek-v4.1-flash",
		Headers: map[string][]string{
			"Session-Id": {"s1"},
		},
		Body: []byte(`{"model":"deepseek-v4.1-flash","reasoning":{"effort":"xhigh"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) == 0 {
		t.Fatal("expected rewritten body")
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["reasoning"].(map[string]any)["effort"] != "high" {
		t.Fatalf("body = %s", resp.Body)
	}
}

func TestBuildPluginMetadata(t *testing.T) {
	t.Parallel()
	plugin, err := buildPlugin(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.Metadata.Name != pluginName {
		t.Fatalf("name = %q", plugin.Metadata.Name)
	}
	if plugin.Capabilities.ManagementAPI == nil || plugin.Capabilities.QuotaProvider == nil {
		t.Fatal("expected management and quota capabilities")
	}
	interceptor, ok := plugin.Capabilities.RequestInterceptor.(*sessionPlugin)
	if !ok || interceptor.Identifier() != pluginID {
		t.Fatalf("unexpected interceptor: %#v", plugin.Capabilities.RequestInterceptor)
	}
}
