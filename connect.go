package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func upsertOpenCodeProvider(configPath, providerName, baseURL, apiKey string, models []string, includeClaude bool) error {
	if strings.TrimSpace(configPath) == "" {
		configPath = "config.yaml"
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return err
	}
	doc := mappingNode(&root)
	if doc == nil {
		return fmt.Errorf("config.yaml root is not a mapping")
	}
	if err := upsertCompatProvider(doc, providerName, baseURL, apiKey, models); err != nil {
		return err
	}
	if includeClaude {
		if err := upsertClaudeKey(doc, apiKey); err != nil {
			return err
		}
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	backup := configPath + ".opencode-session.bak"
	if _, err := os.Stat(backup); err != nil {
		_ = os.WriteFile(backup, raw, 0o600)
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func mappingNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return mappingNode(n.Content[0])
	}
	if n.Kind == yaml.MappingNode {
		return n
	}
	return nil
}

func mappingGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mappingSet(m *yaml.Node, key string, value *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, scalarNode(key), value)
}

func scalarNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func boolNode(v bool) *yaml.Node {
	if v {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
}

func intNode(v int) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int"}
	n.Value = fmt.Sprintf("%d", v)
	return n
}

func upsertCompatProvider(doc *yaml.Node, providerName, baseURL, apiKey string, models []string) error {
	seq := mappingGet(doc, "openai-compatibility")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mappingSet(doc, "openai-compatibility", seq)
	}
	var target *yaml.Node
	for _, item := range seq.Content {
		if mappingGetString(item, "name") == providerName {
			target = item
			break
		}
	}
	if target == nil {
		target = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		seq.Content = append(seq.Content, target)
	}
	mappingSet(target, "name", scalarNode(providerName))
	mappingSet(target, "base-url", scalarNode(strings.TrimRight(baseURL, "/")))
	mappingSet(target, "disable-cooling", boolNode(true))
	mappingSet(target, "request-retry", intNode(3))
	headers := mappingGet(target, "headers")
	if headers == nil || headers.Kind != yaml.MappingNode {
		headers = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mappingSet(target, "headers", headers)
	}
	mappingSet(headers, "x-opencode-session", scalarNode("$x-opencode-session"))
	upsertAPIKeyEntry(target, apiKey)
	if len(models) > 0 {
		mappingSet(target, "models", modelsSeq(models))
	}
	return nil
}

func upsertAPIKeyEntry(provider *yaml.Node, apiKey string) {
	seq := mappingGet(provider, "api-key-entries")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mappingSet(provider, "api-key-entries", seq)
	}
	for _, item := range seq.Content {
		if mappingGetString(item, "api-key") == apiKey {
			mappingSet(item, "proxy-url", scalarNode("direct"))
			return
		}
	}
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mappingSet(entry, "api-key", scalarNode(apiKey))
	mappingSet(entry, "proxy-url", scalarNode("direct"))
	seq.Content = append(seq.Content, entry)
}

func upsertClaudeKey(doc *yaml.Node, apiKey string) error {
	seq := mappingGet(doc, "claude-api-key")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mappingSet(doc, "claude-api-key", seq)
	}
	for _, item := range seq.Content {
		if mappingGetString(item, "api-key") == apiKey {
			mappingSet(item, "base-url", scalarNode(defaultClaudeURL))
			mappingSet(item, "proxy-url", scalarNode("direct"))
			headers := mappingGet(item, "headers")
			if headers == nil || headers.Kind != yaml.MappingNode {
				headers = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				mappingSet(item, "headers", headers)
			}
			mappingSet(headers, "x-opencode-session", scalarNode("$x-opencode-session"))
			return nil
		}
	}
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mappingSet(entry, "api-key", scalarNode(apiKey))
	mappingSet(entry, "base-url", scalarNode(defaultClaudeURL))
	mappingSet(entry, "proxy-url", scalarNode("direct"))
	headers := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mappingSet(headers, "x-opencode-session", scalarNode("$x-opencode-session"))
	mappingSet(entry, "headers", headers)
	seq.Content = append(seq.Content, entry)
	return nil
}

func modelsSeq(models []string) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, name := range models {
		item := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mappingSet(item, "name", scalarNode(name))
		mappingSet(item, "alias", scalarNode(""))
		seq.Content = append(seq.Content, item)
	}
	return seq
}

func mappingGetString(m *yaml.Node, key string) string {
	n := mappingGet(m, key)
	if n == nil {
		return ""
	}
	return n.Value
}

func listConfiguredKeys(configPath, providerName string) ([]string, []string, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, nil, err
	}
	doc := mappingNode(&root)
	seq := mappingGet(doc, "openai-compatibility")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil, nil, nil
	}
	var keys, models []string
	for _, item := range seq.Content {
		if mappingGetString(item, "name") != providerName {
			continue
		}
		if entries := mappingGet(item, "api-key-entries"); entries != nil && entries.Kind == yaml.SequenceNode {
			for _, entry := range entries.Content {
				if key := mappingGetString(entry, "api-key"); key != "" {
					keys = append(keys, key)
				}
			}
		}
		if modelSeq := mappingGet(item, "models"); modelSeq != nil && modelSeq.Kind == yaml.SequenceNode {
			for _, model := range modelSeq.Content {
				if name := mappingGetString(model, "name"); name != "" {
					models = append(models, name)
				}
			}
		}
	}
	return keys, models, nil
}

func resolveConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "config.yaml"
	}
	if filepath.IsAbs(path) {
		return path
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, path)
	}
	return path
}
