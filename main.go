package main

import (
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var pluginVersion = "0.2.10"

func buildPlugin(configYAML []byte, _ string) (pluginapi.Plugin, error) {
	cfg, err := parseConfig(configYAML)
	if err != nil {
		return pluginapi.Plugin{}, err
	}
	p := &sessionPlugin{cfg: cfg}
	q := &quotaAdapter{p: p}
	return pluginapi.Plugin{
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           pluginAuthor,
			GitHubRepository: pluginRepoURL,
			ConfigFields: []pluginapi.ConfigField{
				{Name: "rewrite_body", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Rewrite Codex-style JSON for OpenCode Go models."},
				{Name: "clamp_reasoning", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Clamp xhigh/max/ultra reasoning to high."},
				{Name: "drop_json_schema", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Drop json_schema / encrypted include."},
				{Name: "function_tools", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Keep only type=function tools."},
				{Name: "cpa_config_path", Type: pluginapi.ConfigFieldTypeString, Description: "Path to CLIProxyAPI config.yaml for one-click connect."},
				{Name: "provider_name", Type: pluginapi.ConfigFieldTypeString, Description: "openai-compatibility provider name. Default opencode-go."},
				{Name: "base_url", Type: pluginapi.ConfigFieldTypeString, Description: "OpenCode Go base URL. Default https://opencode.ai/zen/go/v1."},
				{Name: "include_claude", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Also write the same key as a Claude-compatible OpenCode Go channel."},
				{Name: "proxy_url", Type: pluginapi.ConfigFieldTypeString, Description: "Per-key proxy-url. Default direct. Use http://127.0.0.1:7890 to go through a proxy."},
				{Name: "match_models", Type: pluginapi.ConfigFieldTypeArray, Description: "If set, only rewrite bodies for these exact model names."},
				{Name: "match_prefixes", Type: pluginapi.ConfigFieldTypeArray, Description: "If set, only rewrite bodies for models with these prefixes."},
				{Name: "skip_models", Type: pluginapi.ConfigFieldTypeArray, Description: "Model names that skip body rewrite."},
			},
		},
		Capabilities: pluginapi.Capabilities{
			RequestInterceptor: p,
			ManagementAPI:      p,
			QuotaProvider:      q,
			UsagePlugin:        p,
			Scheduler:          p,
		},
	}, nil
}
