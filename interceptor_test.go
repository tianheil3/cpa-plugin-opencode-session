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
		Model:    "minimax-m3",
		ToFormat: "openai",
		Headers: map[string][]string{
			"Session-Id": {"mac-codex-1"},
		},
		Body: []byte(`{"model":"minimax-m3","input":"hi"}`),
		Metadata: map[string]any{
			"session_affinity_provider": "openai-compatible-opencode-go",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Headers.Get(sessionHeader); got != "mac-codex-1" {
		t.Fatalf("x-opencode-session = %q, want mac-codex-1", got)
	}
}

func TestInterceptSkipsNonOpenCodeModels(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.InterceptRequestBeforeAuth(context.Background(), pluginapi.RequestInterceptRequest{
		Model: "deepseek-v4.1-flash",
		Headers: map[string][]string{
			"Session-Id": {"mac-codex-1"},
		},
		Body: []byte(`{"model":"deepseek-v4.1-flash","input":"hi"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Headers.Get(sessionHeader); got != "" {
		t.Fatalf("non-OpenCode request got x-opencode-session = %q", got)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("non-OpenCode request should not rewrite body")
	}

	after, err := p.InterceptRequestAfterAuth(context.Background(), pluginapi.RequestInterceptRequest{
		Model:    "gpt-5.4",
		ToFormat: "openai",
		Headers: map[string][]string{
			"Session-Id": {"codex-session"},
		},
		Metadata: map[string]any{
			"session_affinity_provider": "codex",
			"selected_auth_id":          "codex-auth-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Headers.Get(sessionHeader); got != "" {
		t.Fatalf("codex request got x-opencode-session = %q", got)
	}
}

func TestShouldInjectSessionUsesProviderAndAuthID(t *testing.T) {
	t.Parallel()
	authIDs := map[string]struct{}{"openai-compatibility:opencode-go:abc123abc123": {}}
	models := []string{"minimax-m3"}
	if shouldInjectSession(pluginapi.RequestInterceptRequest{
		Model: "deepseek-v4.1-flash",
	}, "opencode-go", models, authIDs) {
		t.Fatal("unrelated model must not inject before auth")
	}
	if !shouldInjectSession(pluginapi.RequestInterceptRequest{
		Model:    "minimax-m3",
		ToFormat: "openai",
		Metadata: map[string]any{"session_affinity_provider": "openai-compatible-opencode-go"},
	}, "opencode-go", models, authIDs) {
		t.Fatal("OpenCode Go provider must inject")
	}
	if shouldInjectSession(pluginapi.RequestInterceptRequest{
		Model:    "minimax-m3",
		ToFormat: "openai",
		Metadata: map[string]any{
			"session_affinity_provider": "openai-compatible-deepseek官方",
			"selected_auth_id":          "other-auth",
		},
	}, "opencode-go", models, authIDs) {
		t.Fatal("same model name on another provider must not inject")
	}
}

func TestInterceptRewritesBody(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.InterceptRequestAfterAuth(context.Background(), pluginapi.RequestInterceptRequest{
		Model:    "minimax-m3",
		ToFormat: "openai",
		Headers: map[string][]string{
			"Session-Id": {"s1"},
		},
		Body: []byte(`{"model":"minimax-m3","reasoning":{"effort":"xhigh"}}`),
		Metadata: map[string]any{
			"session_affinity_provider": "openai-compatible-opencode-go",
		},
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
	if plugin.Capabilities.ManagementAPI == nil || plugin.Capabilities.QuotaProvider == nil || plugin.Capabilities.Scheduler == nil {
		t.Fatal("expected management, quota, and scheduler capabilities")
	}
	interceptor, ok := plugin.Capabilities.RequestInterceptor.(*sessionPlugin)
	if !ok || interceptor.Identifier() != pluginID {
		t.Fatalf("unexpected interceptor: %#v", plugin.Capabilities.RequestInterceptor)
	}
}
