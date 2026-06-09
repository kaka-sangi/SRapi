# Provider Authentication Matrix

This matrix is the regression baseline for provider preset `auth_methods`.

Legend:
- ✅ wired and preset-reachable
- 🟡 supported only by manual or legacy account configuration
- ⬜ runtime class exists, but no default preset may expose it
- — not supported for that provider family

| Provider preset / family | api_key | oauth_refresh / oauth_device_code | cli_client_token | desktop_client_token / ide_plugin_token | web_session_cookie | service_account_json | custom_reverse_proxy |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `openai` | ✅ Bearer API key | — not supported by preset | — | — | — | ⬜ hidden | ✅ Bearer passthrough |
| Third-party OpenAI-compatible presets | ✅ Bearer API key | — | — | — | — | ⬜ hidden | ✅ Bearer passthrough |
| `chatgpt-web` | — | — | — | — | ✅ Cookie via `reverse-proxy-chatgpt-web` | ⬜ hidden | ✅ Bearer passthrough |
| `codex-cli` | — | ✅ OAuth refresh/device with built-in Codex endpoint | 🟡 legacy/manual CLI token | — | — | ⬜ hidden | ✅ Bearer passthrough |
| `anthropic` | ✅ `x-api-key` | ✅ OAuth refresh/device with built-in Claude Code endpoint | ✅ CLI client token | — | — | ⬜ hidden | ✅ Bearer passthrough |
| Third-party Anthropic-compatible presets | ✅ `x-api-key` | — | — | — | — | ⬜ hidden | ✅ Bearer passthrough |
| `gemini` | ✅ `?key=` / configured API-key mode | — not supported by preset | — | — | — | ⬜ hidden | ✅ Bearer passthrough |
| `antigravity` | — | ✅ OAuth refresh with `upstream_client=antigravity_desktop` and client secret | — | ⬜ merged into `oauth_refresh` in presets | — | ⬜ hidden | ✅ Bearer passthrough |
| `bedrock` | ✅ AWS SigV4 credential shape | — | — | — | — | ⬜ hidden | — |
| `rerank-compatible` | ✅ API key | — | — | — | — | ⬜ hidden | ✅ Bearer passthrough |

Guardrail:
- `apps/api/internal/modules/providers/preset/registry_test.go` contains `TestPresetRuntimeAllowlistsOnlyExposeSignableAuthMethods`. It fails if any preset exposes a runtime class outside the signable set.
- `service_account_json`, `desktop_client_token`, and `ide_plugin_token` remain runtime enum values for stored legacy accounts, but default provider presets and the account-create fallback UI do not expose them.
- OpenAI and Gemini OAuth account runtimes are explicitly rejected with `not_supported` in the provider adapter when configured manually against the first-party presets.
