# cpa-plugin-opencode-session

CLIProxyAPI request interceptor that injects `x-opencode-session` for OpenCode Zen / OpenCode Go, and optionally rewrites request JSON so Codex-style payloads work on those models.

Codex CLI sends `Session-Id`. OpenCode Zen requires `x-opencode-session`. CLIProxyAPI's OpenAI-compat executor rebuilds upstream headers, so the client header never reaches Zen unless a plugin writes it onto the execution request **and** the provider config copies it with `$x-opencode-session`.

[中文说明](README.zh-CN.md)

## Capability

- ID: `opencode-session`
- Capability: `request_interceptor`
- Author: tianheil3

## What it does

On `request.intercept_before` and `request.intercept_after`:

1. Resolve a session id (first non-empty wins):
   - `x-opencode-session`
   - Codex `Session-Id` / `Thread-Id`
   - Claude `X-Claude-Code-Session-Id`
   - DeepSeek Harness `X-DeepSeek-Harness-Session-Id` / `X-Session-Affinity`
   - `X-Session-Id`, `X-Client-Request-Id`
   - body `prompt_cache_key` / `client_metadata.session_id`
   - host `Metadata.canonical_session_id`
   - generated UUID (last resort; unblocks `MissingSessionID`)
2. Set execution header `x-opencode-session` (and `Session-Id`) to that value.
3. Optionally rewrite JSON:
   - clamp `xhigh` / `max` / `ultra` reasoning to `high` for models that reject those levels
   - drop `text.format=json_schema` and `include: reasoning.encrypted_content`
   - flatten `namespace` tools and drop non-`function` tools (including `web_search`)

## Install from the plugin store

After the official registry lists this plugin:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    opencode-session:
      enabled: true
      priority: 10
```

## Required provider header

OpenAI-compat executors do not forward interceptor headers by themselves. Copy the injected value onto the wire:

```yaml
openai-compatibility:
  - name: opencode-go
    base-url: https://opencode.ai/zen/go/v1
    headers:
      x-opencode-session: "$x-opencode-session"
```

`$Name` is resolved from the plugin-augmented execution headers. If the value is missing, CLIProxyAPI omits the header.

## Manual install

Place the shared library in the host plugin directory:

```text
plugins/linux/amd64/opencode-session.so
plugins/darwin/arm64/opencode-session.dylib
plugins/windows/amd64/opencode-session.dll
```

Enable `plugins.configs.opencode-session.enabled` and restart CLIProxyAPI.

## Configuration

Host-owned fields (`enabled`, `priority`) stay in CLIProxyAPI. The rest is passed to the plugin:

| Key | Default | Meaning |
|---|---|---|
| `rewrite_body` | `true` | Rewrite request JSON |
| `clamp_reasoning` | `true` | Clamp unsupported reasoning levels |
| `drop_json_schema` | `true` | Drop json_schema / encrypted include |
| `function_tools` | `true` | Keep only `type=function` tools |
| `match_models` | empty | If set, only rewrite these exact names |
| `match_prefixes` | empty | If set, only rewrite matching prefixes |
| `skip_models` | empty | Skip body rewrite for these names |

Session header injection always runs, even when body rewrite is skipped.

## Build

Requires Go 1.26+ and CGO.

```bash
make test
make build
```

## Release layout

GitHub Release tag `vX.Y.Z` must contain:

```text
opencode-session_<version>_<goos>_<goarch>.zip
checksums.txt
```

Each zip has the dynamic library at the zip root (`opencode-session.so` / `.dylib` / `.dll`).

## License

MIT
