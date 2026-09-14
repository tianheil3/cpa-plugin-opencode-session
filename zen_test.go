package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseZenModelIDs(t *testing.T) {
	t.Parallel()
	ids, err := parseZenModelIDs([]byte(`{"data":[{"id":"minimax-m3"},{"id":"opencode-go/deepseek-v4.1-flash"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "minimax-m3" || ids[1] != "deepseek-v4.1-flash" {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestRemainingFraction(t *testing.T) {
	t.Parallel()
	if remainingFraction(0) != 1 || remainingFraction(100) != 0 {
		t.Fatal("bounds")
	}
	got := remainingFraction(25)
	if got < 0.74 || got > 0.76 {
		t.Fatalf("got %v", got)
	}
}

func TestUpsertOpenCodeProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	src := []byte("host: \"\"\nport: 18317\nopenai-compatibility:\n  - name: other\n    base-url: https://example.com\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := upsertOpenCodeProvider(path, "opencode-go", defaultZenBaseURL, "sk-test-key", []string{"deepseek-v4.1-flash", "kimi-k3"}, false); err != nil {
		t.Fatal(err)
	}
	keys, models, err := listConfiguredKeys(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "sk-test-key" {
		t.Fatalf("keys = %#v", keys)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "x-opencode-session") {
		t.Fatalf("missing session header: %s", raw)
	}
	if !strings.Contains(string(raw), "name: other") {
		t.Fatal("clobbered unrelated provider")
	}
}

func TestUpsertReenablesDisabledProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	src := []byte("openai-compatibility:\n  - name: opencode-go\n    base-url: https://opencode.ai/zen/go/v1\n    disabled: true\n    api-key-entries:\n      - api-key: sk-old\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := upsertOpenCodeProvider(path, "opencode-go", defaultZenBaseURL, "sk-old", nil, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	if !strings.Contains(text, "disabled: false") {
		t.Fatalf("connect must re-enable the channel:\n%s", text)
	}
	if strings.Contains(text, "disabled: true") {
		t.Fatalf("stale disabled: true left in place:\n%s", text)
	}
}

func TestUpsertKeepsExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	src := []byte("openai-compatibility:\n  - name: opencode-go\n    base-url: https://opencode.ai/zen/go/v1\n    api-key-entries:\n      - api-key: sk-one\n        proxy-url: direct\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := upsertOpenCodeProvider(path, "opencode-go", defaultZenBaseURL, "sk-two", nil, false); err != nil {
		t.Fatal(err)
	}
	keys, _, err := listConfiguredKeys(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %#v", keys)
	}
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key] = true
	}
	if !seen["sk-one"] || !seen["sk-two"] {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestUsageToQuota(t *testing.T) {
	t.Parallel()
	q := usageToQuota(zenUsage{
		Rolling: zenUsageWindow{Status: "ok", Percent: 10, ResetsAt: "t1"},
		Weekly:  zenUsageWindow{Status: "ok", Percent: 20, ResetsAt: "t2"},
		Monthly: zenUsageWindow{Status: "ok", Percent: 30, ResetsAt: "t3"},
	})
	if len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 3 {
		t.Fatalf("%#v", q)
	}
	if q.Groups[0].Buckets[0].RemainingFraction < 0.89 {
		t.Fatalf("remaining = %v", q.Groups[0].Buckets[0].RemainingFraction)
	}
}
