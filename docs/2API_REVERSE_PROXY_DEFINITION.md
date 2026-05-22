# SRapi 2api 反代定义

## 1. 为什么单独定义

SRapi 中的“反代”按 AI 2api 语境使用，不等同于普通 API Gateway，也不等同于“本地 Codex / Claude Code / Antigravity 客户端接入 SRapi”。

本定义是后续 `reverse-proxy-*` Provider Adapter、Reverse Proxy Runtime、Scheduler evidence 和测试 harness 的约束来源。实现时如果与本文冲突，以本文为准。

## 2. SRapi 反代定义来源

SRapi 的“反代 / 2api”定义以本地参考项目为准：

- `/home/senran/Desktop/sub2api`
- `/home/senran/Desktop/CLIProxyAPI`
- `/home/senran/Desktop/chatgpt2api`

这些项目的共同做法就是本项目采用的 2api 反代定义：下游暴露 OpenAI / Anthropic / Gemini 兼容 API，上游直接模拟官方 Web、桌面、CLI、IDE 客户端请求形态，并使用 OAuth、session、desktop token、CLI token 或 IDE token 请求真实上游。

通用网络术语里的 reverse proxy 只说明“在客户端和服务器之间转发”。在 SRapi 中，决策依据不是这个宽泛定义，而是上述本地 2api 项目的 official-client upstream simulation 做法。

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
- Gateway service 不新增 Codex / Claude Code / Antigravity 本地 DTO；协议模拟属于 Provider Adapter / Runtime Adapter。
- Reverse Proxy Runtime 负责出站连接、凭证注入、header hygiene、cookie jar、proxy、HTTP/WSS relay 和风险分类。

## 4. 不是这些东西

SRapi 2api 反代不是：

- 仅把下游请求原样转发给 OpenAI / Anthropic / Gemini 兼容 API。
- 让本地 Codex / Claude Code / Antigravity 作为唯一客户端接入 SRapi。
- 在 Gateway service 层为 Codex / Claude Code / Antigravity 增加一组三方本地 DTO。
- 让调用方传入的 `Authorization`、`Cookie`、`Sec-WebSocket-*` 或 SRapi 内部 header 进入上游。
- 绕过 Scheduler、API key policy、subscription entitlement、usage、billing、audit 或 feedback。

## 5. 目标上游形态

不同 `reverse-proxy-*` adapter 必须构造对应上游客户端形态：

| Adapter | 上游身份 | 典型上游形态 |
| --- | --- | --- |
| `reverse-proxy-codex-cli` | Codex CLI / ChatGPT Codex 客户端 | Codex `/backend-api/codex/responses` 或 Responses WebSocket，Codex headers、session/cache headers、Codex OAuth / device / CLI client token。 |
| `reverse-proxy-claude-code-cli` | Claude Code 客户端 | Anthropic Messages 端点上的 Claude Code OAuth / setup-token credential header、Claude Code beta/version/cache/signing/body conventions。 |
| `reverse-proxy-gemini-cli` | Gemini CLI / Code Assist 客户端 | Gemini Code Assist / Cloud Code endpoints、project/user context、Google OAuth credential behavior。 |
| `reverse-proxy-antigravity` | Antigravity Desktop / IDE 客户端 | Google Cloud Code / Antigravity internal endpoints、desktop/IDE token, user-agent, HTTP behavior, protocol-specific payload. |
| `custom_reverse_proxy` | Operator-defined upstream client | Explicit endpoint and egress profile defined by operator metadata. |

Implementation status:

- WP-400 implements the HTTP Codex CLI 2api path for text requests: `reverse-proxy-codex-cli` builds a Codex Responses request and sends `base_url + "/responses"` through Reverse Proxy Runtime.
- WP-410 implements the Codex CLI 2api Responses WebSocket upstream relay for explicitly requested `/v1/responses/ws` calls: SRapi schedules an eligible Codex reverse-proxy account, derives Codex `ws/wss` `/responses`, sends Codex official-client headers plus a `response.create` frame with the mapped upstream model, and uses the selected account OAuth/session/CLI credential through Reverse Proxy Runtime.
- WP-420 implements the Claude Code CLI 2api Messages HTTP path: `reverse-proxy-claude-code-cli` builds `/messages?beta=true`, Claude Code OAuth/beta/version/stainless/session headers, and Claude Code system/billing blocks, while Reverse Proxy Runtime injects the selected OAuth/CLI token.
- Persistent Codex WebSocket session reuse, richer prompt-cache policy, local Codex CLI client ingress, Claude Code WebSocket adapters, and Antigravity WebSocket adapters are still follow-up work.

## 6. Boundary Rules

1. `runtime_class = api_key` is official API-key adapter behavior, not SRapi 2api reverse proxy behavior.
2. `reverse-proxy-*` 2api adapters require OAuth/session/client-token style account credentials (`runtime_class != api_key`) and must reject or avoid scheduling `api_key` runtime accounts.
3. `runtime_class != api_key` under a `reverse-proxy-*` adapter must use Reverse Proxy Runtime for all upstream HTTP/WSS calls.
4. Provider Adapter owns upstream official-client payload shape. Reverse Proxy Runtime must not invent business DTOs.
5. Reverse Proxy Runtime owns transport behavior: account credential injection, forbidden header stripping, user-agent selection, proxy binding, cookie jar, timeout, relay accounting, and runtime error classes.
6. Client Response Renderer owns downstream response shape. Upstream official-client SSE/WSS frames may be transformed only after the runtime has received them.
7. Every successful or failed 2api call must still produce Scheduler decision/feedback and usage evidence.

## 7. Implementation Test Implications

A valid 2api reverse-proxy test should prove at least:

- The selected account identity, not the caller credential, is used upstream.
- The upstream path/header/body matches the target official-client shape, not merely OpenAI-compatible defaults.
- SRapi and caller-only headers are stripped before the upstream request.
- Scheduler decision and usage evidence preserve the original downstream source endpoint.
- Failure classes such as `session_invalid`, `account_locked`, `account_banned`, `device_unrecognized`, `challenge_required`, `geo_blocked`, `timeout`, and `network_error` are mapped into account protection and feedback paths.

## 8. Source References

- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/codex_executor.go`: Codex official-client/OAuth upstream request shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/claude_executor.go`: Claude Code official-client/OAuth upstream request shape.
- `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/antigravity_executor.go`: Antigravity official-client/OAuth upstream request shape.
- `/home/senran/Desktop/sub2api/backend/internal/service/gateway_service.go`: Claude Code OAuth mimicry, billing/system blocks, and compatible API rendering.
- `/home/senran/Desktop/sub2api/backend/internal/service/antigravity_gateway_service.go`: Antigravity upstream forwarding and protocol conversion.
- `/home/senran/Desktop/chatgpt2api/services/openai_backend_api.py`: ChatGPT Web upstream request shape using access token, browser-style headers, device/session IDs, and backend API paths.
