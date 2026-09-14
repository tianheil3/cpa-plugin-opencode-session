package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestStableCompatAuthIDIsDeterministic(t *testing.T) {
	t.Parallel()
	a := stableCompatAuthID("opencode-go", "sk-one", "https://opencode.ai/zen/go/v1", "direct")
	b := stableCompatAuthID("opencode-go", "sk-one", "https://opencode.ai/zen/go/v1", "direct")
	c := stableCompatAuthID("opencode-go", "sk-two", "https://opencode.ai/zen/go/v1", "direct")
	if a == "" || a != b {
		t.Fatalf("a=%q b=%q", a, b)
	}
	if a == c {
		t.Fatal("different keys must not share auth id")
	}
	if len(a) < len("openai-compatibility:opencode-go:")+12 {
		t.Fatalf("id too short: %q", a)
	}
}

func TestHandleRegisterAdvertisesScheduler(t *testing.T) {
	raw, err := handleRegister([]byte(`{"config_yaml":null}`))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"scheduler":true`) {
		t.Fatalf("register payload missing scheduler: %s", text)
	}
}

func TestPickFallsThroughOnMixedPool(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.Pick(context.Background(), pluginapi.SchedulerPickRequest{
		Provider: "openai-compatibility",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "openai-compatibility:opencode-go:aaaaaaaaaaaa", Provider: "openai-compatible-opencode-go", Attributes: map[string]string{"compat_name": "opencode-go"}},
			{ID: "openai-compatibility:openrouter:bbbbbbbbbbbb", Provider: "openai-compatible-openrouter", Attributes: map[string]string{"compat_name": "openrouter"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Handled {
		t.Fatalf("mixed pool must fall through: %#v", resp)
	}
}

func TestPickIgnoresOtherProviders(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.Pick(context.Background(), pluginapi.SchedulerPickRequest{
		Provider: "codex",
		Candidates: []pluginapi.SchedulerAuthCandidate{{
			ID:       "codex-1",
			Provider: "codex",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Handled {
		t.Fatalf("codex must fall through: %#v", resp)
	}
}

func TestPickPrefersHigherRollingQuota(t *testing.T) {
	t.Parallel()
	low := "openai-compatibility:opencode-go:lowlowlowlow"
	high := "openai-compatibility:opencode-go:highhighhigh"
	p := &sessionPlugin{cfg: defaultConfig()}
	p.rememberUsage(low, "sk-LOW…low1", zenUsage{Rolling: zenUsageWindow{Percent: 90}})
	p.rememberUsage(high, "sk-HI…high", zenUsage{Rolling: zenUsageWindow{Percent: 10}})
	resp, err := p.Pick(context.Background(), pluginapi.SchedulerPickRequest{
		Provider: "openai-compatible-opencode-go",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: low, Provider: "openai-compatible-opencode-go", Attributes: map[string]string{"compat_name": "opencode-go"}},
			{ID: high, Provider: "openai-compatible-opencode-go", Attributes: map[string]string{"compat_name": "opencode-go"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Handled || resp.AuthID != high {
		t.Fatalf("resp = %#v want %s", resp, high)
	}
}

func TestPickSkipsRecentlyExhaustedKey(t *testing.T) {
	t.Parallel()
	dead := "openai-compatibility:opencode-go:deaddeaddead"
	live := "openai-compatibility:opencode-go:livelivelive"
	p := &sessionPlugin{cfg: defaultConfig()}
	p.rememberUsage(dead, "sk-DEAD…dead", zenUsage{Rolling: zenUsageWindow{Percent: 5}})
	p.rememberUsage(live, "sk-LIVE…live", zenUsage{Rolling: zenUsageWindow{Percent: 40}})
	p.markExhausted(dead, time.Now().Add(time.Hour))
	resp, err := p.Pick(context.Background(), pluginapi.SchedulerPickRequest{
		Provider: "openai-compatible-opencode-go",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: dead, Provider: "openai-compatible-opencode-go", Attributes: map[string]string{"compat_name": "opencode-go", "base_url": "https://opencode.ai/zen/go/v1"}},
			{ID: live, Provider: "openai-compatible-opencode-go", Attributes: map[string]string{"compat_name": "opencode-go", "base_url": "https://opencode.ai/zen/go/v1"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Handled || resp.AuthID != live {
		t.Fatalf("resp = %#v want %s", resp, live)
	}
}

func TestHandleUsageMarksQuotaExhausted(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	id := "openai-compatibility:opencode-go:abcabcabcabc"
	p.rememberUsage(id, "sk-ABC…abc1", zenUsage{Rolling: zenUsageWindow{Percent: 10}})
	p.HandleUsage(context.Background(), pluginapi.UsageRecord{
		Provider: "openai-compatible-opencode-go",
		BaseURL:  "https://opencode.ai/zen/go/v1",
		AuthID:   id,
		Failed:   true,
		Failure:  pluginapi.UsageFailure{StatusCode: 429, Body: "rate limit"},
	})
	p.mu.Lock()
	until := p.quotaByAuthID[id].ExhaustedUntil
	p.mu.Unlock()
	if until.IsZero() || until.Before(time.Now()) {
		t.Fatalf("exhausted until = %v", until)
	}
}
