# SRapi 2api 反代定义

## 1. 为什么单独定义

SRapi 中的“反代”按 AI 2api 语境使用，不等同于普通 API Gateway，也不等同于“本地 Codex / Claude Code / Antigravity 客户端接入 SRapi”。

本定义是后续 `reverse-proxy-*` Provider Adapter、Reverse Proxy Runtime、Scheduler evidence 和测试 harness 的约束来源。实现时如果与本文冲突，以本文为准。

本文件不是开放讨论项。SRapi 的“反代 / 2api”已经由本地参考实现锁定为：

```txt
SRapi 模拟目标官方客户端或目标上游客户端请求形态，
使用选中的 OAuth / session / desktop / CLI / IDE credential 请求真实上游，
再把结果渲染成下游兼容 API。
```

硬规则：

- 在 SRapi 项目内，“反代 / 2api”只按 `/home/senran/Desktop/sub2api`、`/home/senran/Desktop/CLIProxyAPI`、`/home/senran/Desktop/chatgpt2api` 的做法解释。
- 不再用通用网络 reverse proxy 定义替代本定义。
- 不再把“接入本地 Codex / Claude Code / Antigravity 客户端”当作 SRapi 反代目标。
- 不在 Gateway service 增加 Codex / Claude Code / Antigravity 本地 DTO；官方客户端请求模拟属于 Provider Adapter 和 Reverse Proxy Runtime。

## 2. SRapi 反代定义来源

SRapi 的“反代 / 2api”定义以本地参考项目为准：

- `/home/senran/Desktop/sub2api`
- `/home/senran/Desktop/CLIProxyAPI`
- `/home/senran/Desktop/chatgpt2api`

这些项目就是 SRapi 的 2api 语义来源，不需要再从通用代理术语重新推导：

- `sub2api`：账号池、OAuth/API-key 区分、Claude Code / Codex / Antigravity 等上游形态、兼容 API 输出、粘性会话和额度/冷却调度。
- `CLIProxyAPI`：Codex、Claude Code、Gemini、Antigravity 等 OAuth/device/token 登录，按官方 CLI/桌面/IDE 客户端请求形态转发，并对外提供 OpenAI / Claude / Gemini 兼容 API。
- `chatgpt2api`：使用 ChatGPT Web access token/session、browser/OAI/Sentinel headers 和 `/backend-api/*` 上游路径，对外提供 OpenAI-compatible 图片、对话和相关能力。

这些项目的共同做法就是本项目采用的 2api 反代定义：下游暴露 OpenAI / Anthropic / Gemini 兼容 API，上游直接模拟官方 Web、桌面、CLI、IDE 客户端请求形态，并使用 OAuth、session、desktop token、CLI token 或 IDE token 请求真实上游。

通用网络术语里的 reverse proxy 只说明“在客户端和服务器之间转发”。在 SRapi 中，决策依据不是这个宽泛定义，而是上述本地 2api 项目的 official-client upstream simulation 做法。

也就是说，SRapi 的反代判断标准不是“有没有一个代理服务器”，而是：

- 下游是否仍使用兼容 API。
- SRapi 是否选择账号池里的非 API-key 账号身份。
- Adapter 是否构造目标官方客户端会发出的 upstream endpoint、headers、body、stream/WSS 形态。
- Reverse Proxy Runtime 是否使用该账号的 OAuth/session/desktop/CLI/IDE credential 和传输上下文发给真实上游。

如果一条路径只是把下游请求按 OpenAI-compatible / Anthropic-compatible / Gemini-compatible 官方 API-key 方式发给普通上游，它不是 SRapi 2api 反代；它只是官方 API-key Provider Adapter。

## 3. SRapi 里的 2api 反代定义

SRapi 的 2api 反代是：

```txt
Downstream SDK / App Request
  -> SRapi compatible Gateway endpoint
  -> Canonical AI Request
  -> Scheduler selects Provider Account
  -> Provider Adapter builds upstream official-client request
  -> Reverse Proxy Runtime sends request with selected account identity
  -> Upstream official client endpoint
  -> SRapi renders response back to downstream protocol
```

核心含义：

- SRapi 对下游暴露 OpenAI-compatible、Anthropic-compatible、Gemini-compatible 或其他统一 API。
- SRapi 对上游不是调用普通 API SDK 形态，而是模拟目标官方客户端或非 API Key 客户端形态。
- 这里的“反代 / 2api”重点在上游请求模拟：SRapi 直接构造 ChatGPT Web、Codex、Claude Code、Gemini CLI、Antigravity 等官方客户端会发出的 upstream request，并用选中账号的 OAuth、session、desktop、CLI 或 IDE token 发给它们的真实上游。
- 选中账号的 `runtime_class`、`upstream_client`、credential、proxy、cookie jar、User-Agent、header template 决定上游看到的请求身份。
- OAuth / session / desktop / CLI / IDE credential 是 2api 路径的默认身份来源；`api_key` 账号不是 2api 身份来源。
- Gateway service 不新增 Codex / Claude Code / Antigravity 本地 DTO；协议模拟属于 Provider Adapter / Runtime Adapter。
- Reverse Proxy Runtime 负责出站连接、凭证注入、header hygiene、cookie jar、proxy、HTTP/WSS relay 和风险分类。

## 4. 不是这些东西

SRapi 2api 反代不是：

- 仅把下游请求原样转发给 OpenAI / Anthropic / Gemini 兼容 API。
- 让本地 Codex / Claude Code / Antigravity 作为唯一客户端接入 SRapi。
- 让本地 Codex / Claude Code / Antigravity 进程替 SRapi 做业务请求。
- 在 Gateway service 层为 Codex / Claude Code / Antigravity 增加一组三方本地 DTO。
- 让调用方传入的 `Authorization`、`Cookie`、`Sec-WebSocket-*` 或 SRapi 内部 header 进入上游。
- 绕过 Scheduler、API key policy、subscription entitlement、usage、billing、audit 或 feedback。

## 5. 目标上游形态

不同 `reverse-proxy-*` adapter 必须构造对应上游客户端形态：

| Adapter | 上游身份 | 典型上游形态 |
| --- | --- | --- |
| `reverse-proxy-chatgpt-web` | ChatGPT Web 客户端 | ChatGPT `/backend-api/conversation`，browser / OAI device-session / Sentinel requirements headers，ChatGPT Web Conversation body，ChatGPT OAuth / Web session credential。 |
| `reverse-proxy-codex-cli` | Codex CLI / ChatGPT Codex 客户端 | Codex `/backend-api/codex/responses` 或 Responses WebSocket，Codex headers、session/cache headers、Codex OAuth / device / CLI client token。 |
| `reverse-proxy-claude-code-cli` | Claude Code 客户端 | Anthropic Messages 端点上的 Claude Code OAuth / setup-token credential header、Claude Code beta/version/cache/signing/body conventions。 |
| `reverse-proxy-gemini-cli` | Gemini CLI / Code Assist 客户端 | Gemini Code Assist / Cloud Code endpoints、project/user context、Google OAuth credential behavior。 |
| `reverse-proxy-antigravity` | Antigravity Desktop / IDE 客户端 | Google Cloud Code / Antigravity internal endpoints、desktop/IDE token, user-agent, HTTP behavior, protocol-specific payload. |
| `custom_reverse_proxy` | Operator-defined upstream client | Explicit endpoint and egress profile defined by operator metadata. |

Implementation status:

- WP-400 implements the HTTP Codex CLI 2api path for text requests: `reverse-proxy-codex-cli` builds a Codex Responses request and sends `base_url + "/responses"` through Reverse Proxy Runtime. The same runtime boundary handles `/v1/responses/compact` by sending `base_url + "/responses/compact"` and replaying the raw `response.compaction` JSON.
- WP-410 implements the Codex CLI 2api Responses WebSocket upstream relay for explicitly requested `/v1/responses/ws` calls: SRapi schedules an eligible Codex reverse-proxy account, derives Codex `ws/wss` `/responses`, sends Codex official-client headers plus a `response.create` frame with the mapped upstream model, and uses the selected account OAuth/session/CLI credential through Reverse Proxy Runtime.
- WP-600 implements Codex refresh-token-only onboarding for `reverse-proxy-codex-cli`: admin create/import/update may receive only an OAuth `refresh_token`, Reverse Proxy Runtime exchanges it at the Codex OAuth token endpoint using the Codex CLI client ID/scope, persists the resulting encrypted access-token state, and Gateway requests can immediately call Codex `/responses` with selected-account OAuth identity.
- WP-420 implements the Claude Code CLI 2api Messages HTTP path: `reverse-proxy-claude-code-cli` builds `/messages?beta=true`, Claude Code OAuth/beta/version/stainless/session headers, and Claude Code system/billing blocks, while Reverse Proxy Runtime injects the selected OAuth/CLI token.
- WP-560 extends Anthropic count_tokens into the same boundary: API-key Anthropic accounts call `/messages/count_tokens` with the mapped upstream model, while `reverse-proxy-claude-code-cli` accounts call `/messages/count_tokens?beta=true` with Claude Code token-counting beta, headers, and system/billing blocks through Reverse Proxy Runtime.
- WP-610 implements Claude Code refresh-token-only onboarding for `reverse-proxy-claude-code-cli`: admin create/import/update may receive only a Claude Code OAuth `refresh_token`, Reverse Proxy Runtime exchanges it at the Anthropic OAuth token endpoint using the Claude Code client ID and JSON refresh request shape, persists encrypted access-token state, and Gateway `/v1/messages` can immediately use the selected account OAuth identity.
- WP-430 implements the ChatGPT Web 2api Conversation HTTP path: `reverse-proxy-chatgpt-web` builds `/backend-api/conversation`, browser/OAI/Sentinel headers, and ChatGPT Web Conversation body, while Reverse Proxy Runtime injects the selected OAuth/Web-session token.
- WP-440 adds ChatGPT Web Sentinel requirements auto fetch: if the selected account has no static requirements token, the adapter bootstraps ChatGPT Web and posts `/backend-api/sentinel/chat-requirements` through Reverse Proxy Runtime before the conversation request.
- WP-450 implements the Antigravity Desktop/IDE 2api HTTP text path: `reverse-proxy-antigravity` builds Google Cloud Code `/v1internal:generateContent` or `/v1internal:streamGenerateContent?alt=sse` requests with Antigravity `project`/`requestId`/`userAgent`/`requestType` envelope, while Reverse Proxy Runtime injects the selected desktop/IDE/OAuth token.
- WP-500 implements Antigravity 2api model discovery for already configured accounts: admin discovery posts `{base_url}/v1internal:fetchAvailableModels` through Reverse Proxy Runtime with selected OAuth/desktop/IDE credentials, parses the upstream `models` object, and can persist `supported_models` for Provider-neutral Scheduler filtering.
- WP-530 extends Antigravity discovery with selected-account project bootstrap: if no project metadata is configured, SRapi posts `/v1internal:loadCodeAssist` and, when needed, `/v1internal:onboardUser` through Reverse Proxy Runtime using the same account credential before fetching available models. Persisted discovery writes the resolved project metadata; preview discovery remains side-effect free.
- WP-620 implements Antigravity refresh-token-only onboarding for `reverse-proxy-antigravity`: admin create/import/update may receive only an Antigravity OAuth `refresh_token`, Reverse Proxy Runtime exchanges it at the Google OAuth token endpoint using the Antigravity client ID and configured client secret, persists encrypted access-token state, and Gateway text requests can immediately use the selected account OAuth identity.
- WP-470 implements OpenAI-compatible Realtime WebSocket relay for `GET /v1/realtime`: SRapi parses downstream query `model`, schedules a realtime-capable account, builds upstream `/realtime?model=<mapped_upstream_model>`, and relays non-API-key accounts through Reverse Proxy Runtime using the selected account OAuth/session/client-token credential. WP-630 adds the official API-key Realtime path for `runtime_class = api_key` accounts, which deliberately bypasses 2api Reverse Proxy Runtime and uses only the selected account API key. Neither path is local Codex / Claude Code / Antigravity client ingress.
- Persistent Codex WebSocket session reuse, richer prompt-cache policy, local Codex CLI client ingress, Claude Code WebSocket adapters, Antigravity onboarding UI/API, Antigravity WebSocket adapters, and Antigravity credit overage policy are still follow-up work.

## 6. Local Reference Interpretation Rules

When a future implementation package touches 2api behavior, the local references mean the following:

- `sub2api`: account-pool driven 2api service behavior, Claude Code mimicry, Antigravity forwarding, model/account fallback, quota/cooldown handling, and compatible API rendering.
- `CLIProxyAPI`: OAuth/device/runtime executors for Codex, Claude Code, Gemini, Antigravity, request translators, session affinity, streaming/WebSocket behavior, and account-scoped proxy handling.
- `chatgpt2api`: ChatGPT Web upstream simulation through access token, browser headers, OAI device/session IDs, Sentinel requirements, backend API routes, SSE parsing, and compatible OpenAI-style rendering.

These are references for SRapi's 2api semantics, not architecture templates. SRapi must still preserve its own Gateway -> Canonical AI Request -> Scheduler -> Provider Adapter -> Reverse Proxy Runtime boundaries.

## 6.1 Implementation Decision Rules

When implementing a `reverse-proxy-*` package:

1. First identify the target upstream official-client shape in `sub2api`, `CLIProxyAPI`, or `chatgpt2api`.
2. Keep the downstream route compatible with OpenAI / Anthropic / Gemini style APIs unless the route matrix explicitly defines another compatible protocol.
3. Put target-specific upstream endpoint/header/body construction in Provider Adapter code, not Gateway service DTOs.
4. Put credential injection, cookie jar, proxy, user-agent, forbidden-header stripping, HTTP/WSS dialing, and runtime error classification in Reverse Proxy Runtime.
5. Use the selected Provider Account credential as upstream identity. Caller `Authorization`, caller cookies, and local machine login state do not define upstream identity.
6. Reject or avoid scheduling `runtime_class = api_key` accounts for `reverse-proxy-*` 2api adapters.
7. Add a focused test that proves the upstream path/header/body is the official-client shape from the reference project, not a generic compatible API fallback.

## 7. Boundary Rules

1. `runtime_class = api_key` is official API-key adapter behavior, not SRapi 2api reverse proxy behavior.
2. `reverse-proxy-*` 2api adapters require OAuth/session/client-token style account credentials (`runtime_class != api_key`) and must reject or avoid scheduling `api_key` runtime accounts.
3. `runtime_class != api_key` under a `reverse-proxy-*` adapter must use Reverse Proxy Runtime for all upstream HTTP/WSS calls.
4. Provider Adapter owns upstream official-client payload shape. Reverse Proxy Runtime must not invent business DTOs.
5. Reverse Proxy Runtime owns transport behavior: account credential injection, forbidden header stripping, user-agent selection, proxy binding, cookie jar, timeout, relay accounting, and runtime error classes.
6. Client Response Renderer owns downstream response shape. Upstream official-client SSE/WSS frames may be transformed only after the runtime has received them.
7. Every successful or failed 2api call must still produce Scheduler decision/feedback and usage evidence.

## 8. Implementation Test Implications

A valid 2api reverse-proxy test should prove at least:

- The selected account identity, not the caller credential, is used upstream.
- The upstream path/header/body matches the target official-client shape, not merely OpenAI-compatible defaults.
- SRapi and caller-only headers are stripped before the upstream request.
- Scheduler decision and usage evidence preserve the original downstream source endpoint.
- Failure classes such as `session_invalid`, `account_locked`, `account_banned`, `device_unrecognized`, `challenge_required`, `geo_blocked`, `timeout`, and `network_error` are mapped into account protection and feedback paths.

## 9. Source References

- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/codex_executor.go`: Codex official-client/OAuth upstream request shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/codex_websockets_executor.go`: Codex Responses WebSocket upstream shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/codex_openai_images.go`: Codex/OpenAI image upstream shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/claude_executor.go`: Claude Code official-client/OAuth upstream request shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/antigravity_executor.go`: Antigravity official-client/OAuth upstream request shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/auth/codex`: Codex OAuth/device token acquisition and storage behavior.
- `/home/senran/Desktop/CLIProxyAPI/internal/auth/claude`: Claude OAuth/device token acquisition and storage behavior.
- `/home/senran/Desktop/CLIProxyAPI/internal/auth/antigravity`: Antigravity OAuth/token acquisition and storage behavior.
- `/home/senran/Desktop/sub2api/backend/internal/service/gateway_service.go`: account-pool Gateway behavior and compatible API rendering.
- `/home/senran/Desktop/sub2api/backend/internal/service/openai_gateway_service.go`: OpenAI/Codex OAuth account dispatch, passthrough, compatible rendering, quota and sticky-session behavior.
- `/home/senran/Desktop/sub2api/backend/internal/pkg/antigravity/request_transformer.go`: Antigravity request transformation into upstream internal protocol shape.
- `/home/senran/Desktop/sub2api/backend/internal/service/antigravity_gateway_service.go`: Antigravity upstream forwarding and protocol conversion.
- `/home/senran/Desktop/chatgpt2api/services/openai_backend_api.py`: ChatGPT Web upstream request shape using access token, browser-style headers, device/session IDs, Sentinel requirements, and backend API paths.
