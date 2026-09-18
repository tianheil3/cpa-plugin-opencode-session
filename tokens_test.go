package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestFormatTokenCount(t *testing.T) {
	t.Parallel()
	if got := formatTokenCount(1234); got != "1234" {
		t.Fatalf("got %q", got)
	}
	if got := formatTokenCount(25600); got != "2.56 万" {
		t.Fatalf("got %q", got)
	}
	if got := formatTokenCount(123456789); got != "1.23 亿" {
		t.Fatalf("got %q", got)
	}
}

func TestCacheHitRateSubset(t *testing.T) {
	t.Parallel()
	rate := cacheHitRate(tokenCounters{InputTokens: 100, CacheReadTokens: 40})
	if rate == nil || *rate < 0.399 || *rate > 0.401 {
		t.Fatalf("rate = %v", rate)
	}
}

func TestCacheHitRateIndependentWhenCacheExceedsInput(t *testing.T) {
	t.Parallel()
	rate := cacheHitRate(tokenCounters{InputTokens: 20, CacheReadTokens: 80, CacheWriteTokens: 0})
	if rate == nil || *rate < 0.799 || *rate > 0.801 {
		t.Fatalf("rate = %v", rate)
	}
}

func TestHandleUsageCountsOpenCodeSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	p.HandleUsage(context.Background(), pluginapi.UsageRecord{
		Provider: "openai-compatible-opencode-go",
		BaseURL:  "https://opencode.ai/zen/go/v1",
		Model:    "minimax-m2.5",
		AuthID:   "auth-1",
		Detail: pluginapi.UsageDetail{
			InputTokens:         100,
			OutputTokens:        30,
			ReasoningTokens:     5,
			CacheReadTokens:     40,
			CacheCreationTokens: 10,
			TotalTokens:         130,
		},
	})
	got := p.tokensPayload()
	if got["requests"] != int64(1) {
		t.Fatalf("requests = %#v", got["requests"])
	}
	if got["total_tokens"] != int64(130) {
		t.Fatalf("total = %#v", got["total_tokens"])
	}
	if got["cache_read_tokens"] != int64(40) {
		t.Fatalf("cache_read = %#v", got["cache_read_tokens"])
	}
	if got["cache_hit_display"] != "40.0%" {
		t.Fatalf("hit = %#v", got["cache_hit_display"])
	}
	if got["total_display"] != "130" {
		t.Fatalf("display = %#v", got["total_display"])
	}
	models, _ := got["by_model"].([]map[string]any)
	if len(models) != 1 || models[0]["model"] != "minimax-m2.5" {
		t.Fatalf("by_model = %#v", got["by_model"])
	}
	p.flushTokens()
	raw, err := os.ReadFile(filepath.Join(dir, tokensFileName))
	if err != nil {
		t.Fatal(err)
	}
	var stored tokenStore
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Total.TotalTokens != 130 {
		t.Fatalf("persisted = %+v", stored.Total)
	}

	reloaded := &sessionPlugin{cfg: defaultConfig()}
	reloaded.cfg.CPAConfigPath = path
	again := reloaded.tokensPayload()
	if again["total_tokens"] != int64(130) {
		t.Fatalf("reload = %#v", again)
	}
}

func TestHandleUsageIgnoresOtherProviders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	p.HandleUsage(context.Background(), pluginapi.UsageRecord{
		Provider: "deepseek",
		BaseURL:  "https://api.deepseek.com",
		Model:    "deepseek-chat",
		Detail:   pluginapi.UsageDetail{InputTokens: 999, OutputTokens: 999, TotalTokens: 1998},
	})
	got := p.tokensPayload()
	if got["requests"] != int64(0) || got["total_tokens"] != int64(0) {
		t.Fatalf("counted other provider: %#v", got)
	}
}

func TestHandleUsageFailedDoesNotCountTokens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	p.HandleUsage(context.Background(), pluginapi.UsageRecord{
		Provider: "opencode-go",
		BaseURL:  "https://opencode.ai/zen/go/v1",
		Model:    "minimax-m2.5",
		Failed:   true,
		Failure:  pluginapi.UsageFailure{StatusCode: 429, Body: "quota exceeded"},
		Detail:   pluginapi.UsageDetail{InputTokens: 10, TotalTokens: 10},
	})
	got := p.tokensPayload()
	if got["requests"] != int64(0) {
		t.Fatalf("failed request counted: %#v", got)
	}
	if p.lastFailure == nil || p.lastFailure.Status != 429 {
		t.Fatalf("failure = %+v", p.lastFailure)
	}
}

func TestTokensReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	p.HandleUsage(context.Background(), pluginapi.UsageRecord{
		Provider: "openai-compatible-opencode-go",
		BaseURL:  "https://opencode.ai/zen/go/v1",
		Model:    "kimi-k2.5",
		AuthID:   "auth-2",
		Detail:   pluginapi.UsageDetail{InputTokens: 50, OutputTokens: 10, TotalTokens: 60},
	})
	p.flushTokens()
	resp, err := p.handleTokensReset()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := p.tokensPayload()
	if got["requests"] != int64(0) || got["total_tokens"] != int64(0) {
		t.Fatalf("after reset %#v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, tokensFileName)); !os.IsNotExist(err) {
		t.Fatalf("token file still present: %v", err)
	}
}

func TestCachedTokensFallbackToCacheRead(t *testing.T) {
	t.Parallel()
	delta := usageDelta(pluginapi.UsageDetail{InputTokens: 80, CachedTokens: 20, OutputTokens: 5, TotalTokens: 85})
	if delta.CacheReadTokens != 20 {
		t.Fatalf("delta = %+v", delta)
	}
}
