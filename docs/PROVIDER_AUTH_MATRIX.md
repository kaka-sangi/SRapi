# Provider Authentication Matrix

This matrix is the regression baseline for provider preset `auth_methods`.

Legend:
- ✅ wired and preset-reachable
- 🟡 supported only by manual account configuration
- — not supported or removed

| Provider preset / family | api_key | oauth_refresh / oauth_device_code | cli_client_token | web_session_cookie | custom_reverse_proxy |
| --- | --- | --- | --- | --- | --- |
| `openai` | ✅ Bearer API key | — not supported by preset | — | — | ✅ Bearer passthrough |
| Third-party OpenAI-compatible presets | ✅ Bearer API key | — | — | — | ✅ Bearer passthrough |
| `chatgpt-web` | — | — | — | ✅ Cookie via `reverse-proxy-chatgpt-web` | ✅ Bearer passthrough |
| `codex-cli` | — | ✅ OAuth refresh/device with built-in Codex endpoint | 🟡 legacy/manual CLI token | — | ✅ Bearer passthrough |
| `anthropic` | ✅ `x-api-key` | ✅ OAuth refresh/device with built-in Claude Code endpoint | ✅ CLI client token | — | ✅ Bearer passthrough |
| Third-party Anthropic-compatible presets | ✅ `x-api-key` | — | — | — | ✅ Bearer passthrough |
| `gemini` | ✅ `?key=` / configured API-key mode | — not supported by preset | — | — | ✅ Bearer passthrough |
| `antigravity` | — | ✅ OAuth refresh with `upstream_client=antigravity_desktop` and client secret | — | — | ✅ Bearer passthrough |
| `bedrock` | ✅ AWS SigV4 credential shape | — | — | — | — |
| `rerank-compatible` | ✅ API key | — | — | — | ✅ Bearer passthrough |

Guardrail:
- `apps/api/internal/modules/providers/preset/registry_test.go` contains `TestPresetRuntimeAllowlistsOnlyExposeSignableAuthMethods`. It fails if any preset exposes a runtime class outside the signable set.
- `service_account_json`, `desktop_client_token`, and `ide_plugin_token` have been removed from the runtime enum instead of being left as selectable-but-unsupported aliases. Existing deployments should audit stored `provider_accounts.runtime_class` values before upgrading.
- OpenAI and Gemini OAuth account runtimes are explicitly rejected with `not_supported` in the provider adapter when configured manually against the first-party presets.
