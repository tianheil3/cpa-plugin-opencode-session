package main

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestQuotaProviderAliasesIncludeCompatRuntimeKey(t *testing.T) {
	t.Parallel()
	got := quotaProviderAliases("opencode-go")
	want := []string{
		"opencode-go",
		"openai-compatibility:opencode-go",
		"openai-compatible-opencode-go",
	}
	for _, item := range want {
		found := false
		for _, have := range got {
			if have == item {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("aliases %v missing %q", got, item)
		}
	}
	for _, have := range got {
		if have == "openai-compatibility" {
			t.Fatal("bare openai-compatibility must not be claimed")
		}
	}
}

func TestIsOpenCodeQuotaRequestMatchesRuntimeProvider(t *testing.T) {
	t.Parallel()
	req := pluginapi.QuotaFetchRequest{
		Provider: "openai-compatible-opencode-go",
		Attributes: map[string]string{
			"compat_name": "opencode-go",
			"base_url":    "https://opencode.ai/zen/go/v1",
			"api_key":     "sk-test",
		},
	}
	if !isOpenCodeQuotaRequest(req, "opencode-go") {
		t.Fatal("expected runtime openai-compatible-opencode-go credential to match")
	}
	other := pluginapi.QuotaFetchRequest{
		Provider: "openai-compatible-openrouter",
		Attributes: map[string]string{
			"compat_name": "openrouter",
			"base_url":    "https://openrouter.ai/api/v1",
		},
	}
	if isOpenCodeQuotaRequest(other, "opencode-go") {
		t.Fatal("other openai-compat providers must not match")
	}
}

func TestUsageToQuotaUsesRemainingFraction(t *testing.T) {
	t.Parallel()
	got := usageToQuota(zenUsage{
		Rolling: zenUsageWindow{Status: "ok", Percent: 0, ResetsAt: "2026-09-13T12:00:00Z"},
		Weekly:  zenUsageWindow{Status: "ok", Percent: 25, ResetsAt: "2026-09-20T00:00:00Z"},
		Monthly: zenUsageWindow{Status: "ok", Percent: 100, ResetsAt: "2026-10-01T00:00:00Z"},
	})
	if got.Subscription == nil || got.Subscription.Plan != "OpenCode Go" {
		t.Fatalf("subscription = %+v", got.Subscription)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Buckets) != 3 {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if got.Groups[0].Buckets[0].RemainingFraction != 1 {
		t.Fatalf("rolling remaining = %v", got.Groups[0].Buckets[0].RemainingFraction)
	}
	if got.Groups[0].Buckets[1].RemainingFraction != 0.75 {
		t.Fatalf("weekly remaining = %v", got.Groups[0].Buckets[1].RemainingFraction)
	}
	if got.Groups[0].Buckets[2].RemainingFraction != 0 {
		t.Fatalf("monthly remaining = %v", got.Groups[0].Buckets[2].RemainingFraction)
	}
	windows := []string{
		got.Groups[0].Buckets[0].Window,
		got.Groups[0].Buckets[1].Window,
		got.Groups[0].Buckets[2].Window,
	}
	if windows[0] != "5h" || windows[1] != "7d" || windows[2] != "30d" {
		t.Fatalf("window order = %v, want 5h, 7d, 30d", windows)
	}
}
