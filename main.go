package main

import (
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var pluginVersion = "0.1.0"

func buildPlugin(configYAML []byte, _ string) (pluginapi.Plugin, error) {
	cfg, err := parseConfig(configYAML)
	if err != nil {
		return pluginapi.Plugin{}, err
	}
	p := &sessionPlugin{cfg: cfg}
	return pluginapi.Plugin{
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           pluginAuthor,
			GitHubRepository: pluginRepoURL,
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "rewrite_body",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Rewrite request JSON (tools, reasoning, json_schema) for OpenCode-compatible models.",
				},
				{
					Name:        "clamp_reasoning",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Clamp xhigh/max/ultra reasoning to high for models that reject those levels.",
				},
				{
					Name:        "drop_json_schema",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Drop text.format json_schema for models that do not support it.",
				},
				{
					Name:        "function_tools",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "Keep only type=function tools (flatten namespace, drop web_search).",
				},
				{
					Name:        "match_models",
					Type:        pluginapi.ConfigFieldTypeArray,
					Description: "If set, only rewrite bodies for these exact model names. Session header is always injected.",
				},
				{
					Name:        "match_prefixes",
					Type:        pluginapi.ConfigFieldTypeArray,
					Description: "If set, only rewrite bodies for models with these prefixes.",
				},
				{
					Name:        "skip_models",
					Type:        pluginapi.ConfigFieldTypeArray,
					Description: "Model names that skip body rewrite.",
				},
			},
		},
		Capabilities: pluginapi.Capabilities{
			RequestInterceptor: p,
		},
	}, nil
}
