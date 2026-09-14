package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	return persistYAML(configPath, raw, &root)
}

func persistYAML(configPath string, original []byte, root *yaml.Node) error {
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	backup := configPath + ".opencode-session.bak"
	if _, err := os.Stat(backup); err != nil {
		_ = os.WriteFile(backup, original, 0o600)
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

func mappingDelete(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
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
	mappingSet(target, "disabled", boolNode(false))
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
			mappingSet(item, "disabled", boolNode(false))
			mappingDelete(item, "weight")
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

type configuredEntry struct {
	APIKey   string
	ProxyURL string
	BaseURL  string
	Disabled bool
}

func (e configuredEntry) AuthID(providerName string) string {
	return stableCompatAuthID(providerName, e.APIKey, e.BaseURL, e.ProxyURL)
}

func listConfiguredKeys(configPath, providerName string) ([]string, []string, error) {
	entries, models, err := listConfiguredEntries(configPath, providerName)
	if err != nil {
		return nil, nil, err
	}
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.APIKey)
	}
	return keys, models, nil
}

func listConfiguredEntries(configPath, providerName string) ([]configuredEntry, []string, error) {
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
	var entries []configuredEntry
	var models []string
	for _, item := range seq.Content {
		if mappingGetString(item, "name") != providerName {
			continue
		}
		baseURL := mappingGetString(item, "base-url")
		providerDisabled := yamlTruthy(mappingGetString(item, "disabled"))
		if entriesNode := mappingGet(item, "api-key-entries"); entriesNode != nil && entriesNode.Kind == yaml.SequenceNode {
			for _, entry := range entriesNode.Content {
				key := mappingGetString(entry, "api-key")
				if key == "" {
					continue
				}
				entries = append(entries, configuredEntry{
					APIKey:   key,
					ProxyURL: mappingGetString(entry, "proxy-url"),
					BaseURL:  baseURL,
					Disabled: providerDisabled || keyEntryDisabled(entry),
				})
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
	return entries, models, nil
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

func yamlTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "1", "on":
		return true
	default:
		return false
	}
}

func keyEntryDisabled(entry *yaml.Node) bool {
	if yamlTruthy(mappingGetString(entry, "disabled")) {
		return true
	}
	weight := strings.TrimSpace(mappingGetString(entry, "weight"))
	if weight == "" {
		return false
	}
	n, err := strconv.Atoi(weight)
	return err == nil && n <= 0
}

func opencodeAuthFileName(entry configuredEntry, providerName string) string {
	id := entry.AuthID(providerName)
	parts := strings.Split(id, ":")
	digest := parts[len(parts)-1]
	if digest == "" {
		digest = "key"
	}
	return "opencode-go-" + digest
}

func lookupConfiguredEntry(entries []configuredEntry, providerName, name string) (configuredEntry, int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return configuredEntry{}, -1, false
	}
	for i, entry := range entries {
		if opencodeAuthFileName(entry, providerName) == name || entry.AuthID(providerName) == name {
			return entry, i, true
		}
	}
	if strings.HasPrefix(name, "opencode-go-") {
		if idx, err := strconv.Atoi(strings.TrimPrefix(name, "opencode-go-")); err == nil && idx >= 1 && idx <= len(entries) {
			return entries[idx-1], idx - 1, true
		}
	}
	return configuredEntry{}, -1, false
}

func loadCompatDoc(configPath string) (raw []byte, root yaml.Node, doc *yaml.Node, err error) {
	raw, err = os.ReadFile(configPath)
	if err != nil {
		return nil, yaml.Node{}, nil, err
	}
	if err = yaml.Unmarshal(raw, &root); err != nil {
		return nil, yaml.Node{}, nil, err
	}
	doc = mappingNode(&root)
	if doc == nil {
		return nil, yaml.Node{}, nil, fmt.Errorf("config.yaml root is not a mapping")
	}
	return raw, root, doc, nil
}

func findCompatProvider(doc *yaml.Node, providerName string) *yaml.Node {
	seq := mappingGet(doc, "openai-compatibility")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range seq.Content {
		if mappingGetString(item, "name") == providerName {
			return item
		}
	}
	return nil
}

func providerKeyNodes(provider *yaml.Node) []*yaml.Node {
	seq := mappingGet(provider, "api-key-entries")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]*yaml.Node, 0, len(seq.Content))
	for _, item := range seq.Content {
		if mappingGetString(item, "api-key") != "" {
			out = append(out, item)
		}
	}
	return out
}

func syncProviderDisabled(provider *yaml.Node) {
	nodes := providerKeyNodes(provider)
	allDisabled := len(nodes) > 0
	for _, node := range nodes {
		if !keyEntryDisabled(node) {
			allDisabled = false
			break
		}
	}
	mappingSet(provider, "disabled", boolNode(allDisabled))
}

func setConfiguredEntryDisabled(configPath, providerName, name string, disabled bool) error {
	raw, root, doc, err := loadCompatDoc(configPath)
	if err != nil {
		return err
	}
	provider := findCompatProvider(doc, providerName)
	if provider == nil {
		return fmt.Errorf("auth file not found")
	}
	nodes := providerKeyNodes(provider)
	entries, _, err := listConfiguredEntries(configPath, providerName)
	if err != nil {
		return err
	}
	_, idx, ok := lookupConfiguredEntry(entries, providerName, name)
	if !ok || idx < 0 || idx >= len(nodes) {
		return fmt.Errorf("auth file not found")
	}
	node := nodes[idx]
	if disabled {
		mappingSet(node, "disabled", boolNode(true))
		mappingSet(node, "weight", intNode(0))
	} else {
		mappingSet(node, "disabled", boolNode(false))
		mappingDelete(node, "weight")
	}
	syncProviderDisabled(provider)
	return persistYAML(configPath, raw, &root)
}

func deleteConfiguredEntries(configPath, providerName string, names []string) (deleted []string, failed []map[string]string, err error) {
	raw, root, doc, err := loadCompatDoc(configPath)
	if err != nil {
		return nil, nil, err
	}
	provider := findCompatProvider(doc, providerName)
	if provider == nil {
		for _, name := range names {
			failed = append(failed, map[string]string{"name": name, "error": "auth file not found"})
		}
		return nil, failed, nil
	}
	entries, _, err := listConfiguredEntries(configPath, providerName)
	if err != nil {
		return nil, nil, err
	}
	seq := mappingGet(provider, "api-key-entries")
	remove := map[int]string{}
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		_, idx, ok := lookupConfiguredEntry(entries, providerName, name)
		if !ok {
			failed = append(failed, map[string]string{"name": name, "error": "auth file not found"})
			continue
		}
		remove[idx] = name
	}
	if seq != nil && seq.Kind == yaml.SequenceNode && len(remove) > 0 {
		nodes := providerKeyNodes(provider)
		dropKeys := map[string]struct{}{}
		for idx, name := range remove {
			if idx >= 0 && idx < len(nodes) {
				dropKeys[mappingGetString(nodes[idx], "api-key")] = struct{}{}
				deleted = append(deleted, name)
			}
		}
		kept := make([]*yaml.Node, 0, len(seq.Content))
		for _, item := range seq.Content {
			key := mappingGetString(item, "api-key")
			if key == "" {
				kept = append(kept, item)
				continue
			}
			if _, drop := dropKeys[key]; drop {
				continue
			}
			kept = append(kept, item)
		}
		seq.Content = kept
	}
	if len(providerKeyNodes(provider)) == 0 {
		compat := mappingGet(doc, "openai-compatibility")
		if compat != nil && compat.Kind == yaml.SequenceNode {
			kept := make([]*yaml.Node, 0, len(compat.Content))
			for _, item := range compat.Content {
				if item != provider {
					kept = append(kept, item)
				}
			}
			compat.Content = kept
		}
	} else {
		syncProviderDisabled(provider)
	}
	if err := persistYAML(configPath, raw, &root); err != nil {
		return nil, nil, err
	}
	return deleted, failed, nil
}
