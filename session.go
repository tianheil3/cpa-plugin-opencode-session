package main

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

const sessionHeader = "x-opencode-session"

func headerGet(headers map[string][]string, names ...string) string {
	for _, name := range names {
		for key, values := range headers {
			if strings.EqualFold(key, name) {
				for _, value := range values {
					if trimmed := strings.TrimSpace(value); trimmed != "" {
						return trimmed
					}
				}
			}
		}
	}
	return ""
}

func sessionIDFrom(headers map[string][]string, body []byte, metadata map[string]any) string {
	if sid := headerGet(headers,
		sessionHeader,
		"X-Opencode-Session",
		"Session-Id",
		"Session_id",
		"Thread-Id",
		"Thread_id",
		"X-Claude-Code-Session-Id",
		"X-DeepSeek-Harness-Session-Id",
		"X-Session-Affinity",
		"X-Session-Id",
		"X-Client-Request-Id",
		"X-Codex-Window-Id",
	); sid != "" {
		return sid
	}
	if sid := sessionIDFromBody(body); sid != "" {
		return sid
	}
	if sid := sessionIDFromMetadata(metadata); sid != "" {
		return sid
	}
	return uuid.NewString()
}

func sessionIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if meta, ok := payload["client_metadata"].(map[string]any); ok {
		for _, key := range []string{"session_id", "thread_id"} {
			if value, ok := meta[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	if value, ok := payload["prompt_cache_key"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func sessionIDFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range []string{"canonical_session_id", "session_id", "thread_id"} {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
