package main

import (
	"strings"

	"gopkg.in/yaml.v3"
)

type pluginConfig struct {
	RewriteBody     bool     `yaml:"rewrite_body"`
	ClampReasoning  bool     `yaml:"clamp_reasoning"`
	DropJSONSchema  bool     `yaml:"drop_json_schema"`
	FunctionTools   bool     `yaml:"function_tools"`
	MatchModels     []string `yaml:"match_models"`
	MatchPrefixes   []string `yaml:"match_prefixes"`
	SkipModels      []string `yaml:"skip_models"`
}

func defaultConfig() pluginConfig {
	return pluginConfig{
		RewriteBody:    true,
		ClampReasoning: true,
		DropJSONSchema: true,
		FunctionTools:  true,
	}
}

func parseConfig(raw []byte) (pluginConfig, error) {
	cfg := defaultConfig()
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return pluginConfig{}, err
	}
	return cfg, nil
}

func (c pluginConfig) shouldRewrite(model, requested string) bool {
	if !c.RewriteBody {
		return false
	}
	slug := strings.TrimSpace(model)
	if slug == "" {
		slug = strings.TrimSpace(requested)
	}
	lower := strings.ToLower(slug)
	for _, skip := range c.SkipModels {
		if strings.EqualFold(slug, skip) {
			return false
		}
	}
	if len(c.MatchModels) == 0 && len(c.MatchPrefixes) == 0 {
		return true
	}
	for _, name := range c.MatchModels {
		if strings.EqualFold(slug, name) {
			return true
		}
	}
	for _, prefix := range c.MatchPrefixes {
		if prefix != "" && strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}
