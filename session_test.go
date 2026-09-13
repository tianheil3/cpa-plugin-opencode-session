package main

import (
	"testing"

	"github.com/google/uuid"
)

func TestSessionIDFromPrefersOpencodeHeader(t *testing.T) {
	t.Parallel()
	got := sessionIDFrom(map[string][]string{
		"Session-Id":         {"codex-session"},
		"x-opencode-session": {"zen-session"},
	}, nil, nil)
	if got != "zen-session" {
		t.Fatalf("sessionIDFrom() = %q, want zen-session", got)
	}
}

func TestSessionIDFromCodexSessionID(t *testing.T) {
	t.Parallel()
	got := sessionIDFrom(map[string][]string{
		"Session-Id": {"019f-mac-codex"},
	}, []byte(`{"prompt_cache_key":"pck"}`), nil)
	if got != "019f-mac-codex" {
		t.Fatalf("sessionIDFrom() = %q, want Codex Session-Id", got)
	}
}

func TestSessionIDFromPromptCacheKey(t *testing.T) {
	t.Parallel()
	got := sessionIDFrom(nil, []byte(`{"prompt_cache_key":"pck-1"}`), nil)
	if got != "pck-1" {
		t.Fatalf("sessionIDFrom() = %q, want pck-1", got)
	}
}

func TestSessionIDFromMetadataFallback(t *testing.T) {
	t.Parallel()
	got := sessionIDFrom(nil, nil, map[string]any{
		"canonical_session_id": "codex:abc",
	})
	if got != "codex:abc" {
		t.Fatalf("sessionIDFrom() = %q, want canonical_session_id", got)
	}
}

func TestSessionIDFromGeneratesUUID(t *testing.T) {
	t.Parallel()
	got := sessionIDFrom(nil, []byte(`{`), nil)
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("sessionIDFrom() = %q, want UUID: %v", got, err)
	}
}
