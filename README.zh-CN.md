# cpa-plugin-opencode-session

CLIProxyAPI 的 **OpenCode Go** 插件：一键接入、限额面板，以及补上 Codex/Claude 不会带的 `x-opencode-session`。

## 能力

- ID：`opencode-session`
- `request_interceptor` / `management_api` / `quota_provider` / `usage_plugin`

## 做什么

1. **快捷接入**：在插件页粘贴 Go API Key，会请求官方 ` /models` 和 `/usage`，然后写入名为 `opencode-go` 的 openai-compatibility 渠道（模型列表、`disable-cooling`、session 头、`proxy-url: direct`）。可选同时写 Claude 兼容渠道。
2. **限额管理**：读取官方 `GET https://opencode.ai/zen/go/v1/usage` 的 5 小时 / 周 / 月 used%，在插件页展示，并作为 CPA QuotaProvider。
3. **Session**：把 Codex `Session-Id` 等映射成 `x-opencode-session`，并改写 `xhigh` / `json_schema` / `namespace` tools。

管理页：

```text
/v0/resource/plugins/opencode-session/status
```

从同域 `management.html` 打开，才能自动带上管理密钥。

## 安装

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    opencode-session:
      enabled: true
      priority: 10
```

## License

MIT
