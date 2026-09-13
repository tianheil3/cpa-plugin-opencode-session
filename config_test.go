package main

import "testing"

func TestParseConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RewriteBody || !cfg.ClampReasoning || !cfg.DropJSONSchema || !cfg.FunctionTools {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestShouldRewriteMatchAndSkip(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	cfg.MatchPrefixes = []string{"deepseek"}
	cfg.SkipModels = []string{"deepseek-flash"}
	if !cfg.shouldRewrite("deepseek-v4.1-flash", "") {
		t.Fatal("expected prefix match")
	}
	if cfg.shouldRewrite("deepseek-flash", "") {
		t.Fatal("expected skip_models to win")
	}
	if cfg.shouldRewrite("gpt-5.6-luna", "") {
		t.Fatal("expected unmatched model to skip rewrite")
	}
}

func TestShouldRewriteDisabled(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	cfg.RewriteBody = false
	if cfg.shouldRewrite("deepseek-v4.1-flash", "") {
		t.Fatal("rewrite_body=false should skip")
	}
}
