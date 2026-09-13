package main

import (
	"encoding/json"
	"testing"
)

func TestRewriteBodyClampsXHighForDeepSeek(t *testing.T) {
	t.Parallel()
	in := []byte(`{"model":"deepseek-v4.1-flash","reasoning":{"effort":"xhigh"}}`)
	out, ok := rewriteBody("deepseek-v4.1-flash", in, defaultConfig())
	if !ok {
		t.Fatal("rewriteBody() returned false, want rewrite")
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	reasoning := payload["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("effort = %#v, want high", reasoning["effort"])
	}
}

func TestRewriteBodyKeepsXHighForGPT(t *testing.T) {
	t.Parallel()
	in := []byte(`{"model":"gpt-5.6-luna","reasoning":{"effort":"xhigh"}}`)
	if _, ok := rewriteBody("gpt-5.6-luna", in, defaultConfig()); ok {
		t.Fatal("rewriteBody() rewrote GPT payload, want unchanged")
	}
}

func TestRewriteBodyDropsJSONSchema(t *testing.T) {
	t.Parallel()
	in := []byte(`{"model":"deepseek-v4.1-flash","text":{"format":{"type":"json_schema"}},"include":["reasoning.encrypted_content","foo"]}`)
	out, ok := rewriteBody("deepseek-v4.1-flash", in, defaultConfig())
	if !ok {
		t.Fatal("rewriteBody() returned false, want rewrite")
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	text := payload["text"].(map[string]any)
	if _, exists := text["format"]; exists {
		t.Fatalf("text.format still present: %#v", text["format"])
	}
	include := payload["include"].([]any)
	if len(include) != 1 || include[0] != "foo" {
		t.Fatalf("include = %#v, want [foo]", include)
	}
}

func TestRewriteBodyFlattensNamespaceTools(t *testing.T) {
	t.Parallel()
	in := []byte(`{"model":"deepseek-v4.1-flash","tools":[{"type":"namespace","tools":[{"type":"function","name":"read"},{"type":"web_search"}]},{"type":"web_search"}]}`)
	out, ok := rewriteBody("deepseek-v4.1-flash", in, defaultConfig())
	if !ok {
		t.Fatal("rewriteBody() returned false, want rewrite")
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	tools := payload["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "read" {
		t.Fatalf("kept tool = %#v, want function read", tool)
	}
}
