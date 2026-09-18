# cpa-plugin-opencode-session

CLIProxyAPI plugin for **OpenCode Go**: one-click connect, quota windows, and the `x-opencode-session` header Codex/Claude clients forget to send.

[中文说明](README.zh-CN.md)

## Capability

- ID: `opencode-session`
- Capabilities: `request_interceptor`, `management_api`, `quota_provider`, `usage_plugin`
- Author: tianheil3

## What you get

1. **快捷接入** — paste a Go API key on the plugin page. It probes `GET /zen/go/v1/models` and `GET /zen/go/v1/usage`, then writes an `openai-compatibility` provider named `opencode-go` (models, `disable-cooling`, `request-retry`, `x-opencode-session: $x-opencode-session`, `proxy-url: direct`). Optional Claude-compatible channel at `https://opencode.ai/zen/go`.
2. **限额管理** — rolling 5h / weekly / monthly remaining quota from the official usage API. Shown as the same card style as Codex/Claude. `QuotaProvider` identifier is `opencode-go`; CPA runtime credentials are `openai-compatible-opencode-go`. On CPA 7.2.x the bundled Management Center quota page is hardcoded to five OAuth providers. To show OpenCode Go on **API Key 与限额**:

```yaml
remote-management:
  disable-auto-update-panel: true
```

Then:

```bash
python3 scripts/patch-cpa-quota-page.py /opt/cliproxy-api/static/management.html
```

Set `disable-auto-update-panel` first. CPA otherwise re-downloads `management.html` on boot and wipes the patch. Re-run the script after a CPA upgrade.
3. **Session 注入** — maps Codex `Session-Id` / Claude / DeepSeek Harness headers onto `x-opencode-session`, and rewrites Codex-style JSON (`xhigh`, `json_schema`, `namespace` tools). Other providers are left unchanged.
4. **Local token totals** — successful OpenCode Go requests accumulate input / output / cache-read / cache-write. The plugin page and quota page show counts (including 亿) and cache hit rate. Stored next to CPA config as `opencode-session-tokens.json`; this is not the official bill.

Management UI (after enable):

```text
/v0/resource/plugins/opencode-session/status
```

Authenticated APIs (management key):

| Route | Purpose |
|---|---|
| `GET /v0/management/plugins/opencode-session/status` | Masked keys, quota windows, model list, local token totals |
| `GET /v0/management/plugins/opencode-session/auth-files` | Synthetic Auth Files cards for openai-compat keys |
| `PATCH /v0/management/plugins/opencode-session/auth-files/status` | `{ "name": "opencode-go-…", "disabled": true }` |
| `DELETE /v0/management/plugins/opencode-session/auth-files` | `{ "names": ["opencode-go-…"] }` |
| `POST /v0/management/plugins/opencode-session/connect` | `{ "api_key": "sk-...", "include_claude": false }` |
| `POST /v0/management/plugins/opencode-session/sync-models` | Refresh the model list from Zen |
| `POST /v0/management/plugins/opencode-session/refresh` | Same as status |
| `POST /v0/management/plugins/opencode-session/tokens/reset` | Clear local token totals |

Open the page from the Management Center plugin iframe. Connect writes CPA config, so the page must send the Management Key. It reads, in order: the typed field, `window.parent.__CPA_MGMT_KEY` (set by `scripts/patch-cpa-quota-page.py`), encrypted `localStorage['cli-proxy-auth']` when **记住密码** is on, then the parent React store. A Go API Key in the top box is not a Management Key.

## Install

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    opencode-session:
      enabled: true
      priority: 10
```

Restart CLIProxyAPI. One-click connect writes the provider for you. If you already have it, keep:

```yaml
headers:
  x-opencode-session: "$x-opencode-session"
```

## Session rewrite

On `request.intercept_before` / `request.intercept_after`, and only for OpenCode Go credentials/models:

1. Resolve a session id: existing `x-opencode-session` → Codex `Session-Id`/`Thread-Id` → Claude / DeepSeek Harness headers → `prompt_cache_key` → host `canonical_session_id` → UUID.
2. Set execution header `x-opencode-session`.
3. Optionally rewrite JSON (clamp `xhigh`, drop `json_schema`, flatten function tools).

Other providers are left unchanged.

## Plugin config

| Key | Default | Meaning |
|---|---|---|
| `rewrite_body` | `true` | Rewrite request JSON |
| `clamp_reasoning` | `true` | Clamp unsupported reasoning levels |
| `drop_json_schema` | `true` | Drop json_schema / encrypted include |
| `function_tools` | `true` | Keep only `type=function` tools |
| `cpa_config_path` | `config.yaml` | Config file for one-click connect |
| `provider_name` | `opencode-go` | openai-compatibility provider name |
| `base_url` | `https://opencode.ai/zen/go/v1` | Go API base |
| `include_claude` | `false` | Also write Claude-compatible channel |
| `match_models` / `match_prefixes` / `skip_models` | empty | Limit body rewrite |

Session injection always runs.

## Build

Requires Go 1.26+ and CGO.

```bash
make test
make build VERSION=0.2.9
```

## License

MIT
