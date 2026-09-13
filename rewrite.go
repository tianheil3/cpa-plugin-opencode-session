package main

import (
	"encoding/json"
	"strings"
)

var unsupportedEfforts = map[string]struct{}{
	"xhigh": {},
	"max":   {},
	"ultra": {},
}

func allowsXHigh(slug string) bool {
	s := strings.ToLower(slug)
	return strings.Contains(s, "grok") || strings.Contains(s, "gpt-")
}

func supportsJSONSchema(slug string) bool {
	s := strings.ToLower(slug)
	return strings.Contains(s, "gpt-") || strings.Contains(s, "codex")
}

func nativeToolsOK(slug string) bool {
	s := strings.ToLower(slug)
	if strings.Contains(s, "gpt-") || strings.Contains(s, "codex") {
		return true
	}
	if strings.Contains(s, "claude") || strings.Contains(s, "gemini") {
		return true
	}
	return false
}

func rewriteBody(model string, body []byte, cfg pluginConfig) ([]byte, bool) {
	if len(body) == 0 {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	slug, _ := payload["model"].(string)
	if slug == "" {
		slug = model
	}
	changed := false
	if cfg.ClampReasoning && !allowsXHigh(slug) {
		if clampReasoning(payload) {
			changed = true
		}
	}
	if cfg.DropJSONSchema && !supportsJSONSchema(slug) {
		if dropJSONSchema(payload) {
			changed = true
		}
	}
	if cfg.FunctionTools && !nativeToolsOK(slug) {
		if flattenFunctionTools(payload) {
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return out, true
}

func clampReasoning(payload map[string]any) bool {
	changed := false
	if reasoning, ok := payload["reasoning"].(map[string]any); ok {
		effort, _ := reasoning["effort"].(string)
		if effort == "" {
			reasoning["effort"] = "high"
			payload["reasoning"] = reasoning
			changed = true
		} else if _, bad := unsupportedEfforts[strings.ToLower(effort)]; bad {
			reasoning["effort"] = "high"
			payload["reasoning"] = reasoning
			changed = true
		}
	} else if _, exists := payload["reasoning"]; !exists {
		payload["reasoning"] = map[string]any{"effort": "high"}
		changed = true
	}
	if effort, ok := payload["reasoning_effort"].(string); ok {
		if _, bad := unsupportedEfforts[strings.ToLower(effort)]; bad {
			payload["reasoning_effort"] = "high"
			changed = true
		}
	}
	return changed
}

func dropJSONSchema(payload map[string]any) bool {
	changed := false
	if text, ok := payload["text"].(map[string]any); ok {
		if format, ok := text["format"].(map[string]any); ok {
			if t, _ := format["type"].(string); t == "json_schema" {
				delete(text, "format")
				payload["text"] = text
				changed = true
			}
		}
	}
	if include, ok := payload["include"].([]any); ok {
		kept := make([]any, 0, len(include))
		for _, item := range include {
			s, _ := item.(string)
			if s == "reasoning.encrypted_content" {
				changed = true
				continue
			}
			kept = append(kept, item)
		}
		if changed {
			if len(kept) == 0 {
				delete(payload, "include")
			} else {
				payload["include"] = kept
			}
		}
	}
	return changed
}

func flattenFunctionTools(payload map[string]any) bool {
	tools, ok := payload["tools"].([]any)
	if !ok {
		return false
	}
	out := make([]any, 0, len(tools))
	changed := false
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := tool["type"].(string)
		if t == "namespace" {
			changed = true
			inner, _ := tool["tools"].([]any)
			for _, child := range inner {
				ct, ok := child.(map[string]any)
				if !ok {
					continue
				}
				kind, _ := ct["type"].(string)
				if kind == "" || kind == "function" {
					out = append(out, ct)
				}
			}
			continue
		}
		if t == "" || t == "function" {
			out = append(out, tool)
			continue
		}
		changed = true
	}
	if !changed {
		return false
	}
	payload["tools"] = out
	return true
}
