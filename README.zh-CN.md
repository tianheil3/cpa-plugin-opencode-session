# cpa-plugin-opencode-session

CLIProxyAPI 请求拦截插件：给 OpenCode Zen / OpenCode Go 补上 `x-opencode-session`，并可选改写 Codex 风格的 JSON，避免模型拒请求。

Codex CLI 发的是 `Session-Id`，OpenCode Zen 要的是 `x-opencode-session`。CLIProxyAPI 的 OpenAI 兼容执行器会重建上游请求头，所以必须由插件写入执行头，再用配置里的 `$x-opencode-session` 拷到真正发出去的请求上。

## 能力

- ID：`opencode-session`
- 能力：`request_interceptor`
- 作者：tianheil3

## 行为

在 `request.intercept_before` 和 `request.intercept_after` 中：

1. 按优先级解析 session id：已有 `x-opencode-session` → Codex `Session-Id`/`Thread-Id` → Claude / DeepSeek Harness 头 → 请求体 `prompt_cache_key` → 宿主 `canonical_session_id` → 生成 UUID。
2. 写入执行头 `x-opencode-session`。
3. 可选改写 JSON：把 `xhigh`/`max`/`ultra` 压成 `high`；丢掉 `json_schema` 与 `reasoning.encrypted_content`；把 `namespace` 工具展平并只保留 `function`。

## 商店安装

官方 registry 收录后：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    opencode-session:
      enabled: true
      priority: 10
```

OpenAI 兼容渠道还需要：

```yaml
headers:
  x-opencode-session: "$x-opencode-session"
```

## 构建

需要 Go 1.26+ 和 CGO。

```bash
make test
make build
```

## License

MIT
