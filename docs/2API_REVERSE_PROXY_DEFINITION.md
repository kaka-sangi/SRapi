# SRapi 2api 反代定义

## 1. 为什么单独定义

SRapi 中的“反代”按 AI 2api 语境使用，不等同于普通 API Gateway，也不等同于“本地 Codex / Claude Code / Antigravity 客户端接入 SRapi”。

本定义是后续 `reverse-proxy-*` Provider Adapter、Reverse Proxy Runtime、Scheduler evidence 和测试 harness 的约束来源。实现时如果与本文冲突，以本文为准。

## 2. 通用 reverse proxy 定义

通用网络语境里，reverse proxy 是位于客户端和后端服务器之间的服务。客户端连接 reverse proxy，reverse proxy 代表后端服务器接收请求、选择或转发到后端，再把后端响应返回客户端。

参考资料：

- MDN: Proxy servers and tunneling, reverse proxies section.
- Cloudflare: What is a reverse proxy?

这一定义只说明“谁在客户端和服务器之间转发”，不自动说明 AI 2api 场景里的协议模拟、凭证材料、官方客户端指纹或响应转换。

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
| `reverse-proxy-codex-cli` | Codex CLI / ChatGPT Codex 客户端 | Codex `/backend-api/codex/responses` 或 Responses WebSocket，Codex headers、session/cache headers、Codex OAuth/API token。 |
| `reverse-proxy-claude-code-cli` | Claude Code 客户端 | Anthropic Messages 端点上的 Claude Code OAuth/API-key header、Claude Code beta/version/cache/signing/body conventions。 |
| `reverse-proxy-gemini-cli` | Gemini CLI / Code Assist 客户端 | Gemini Code Assist / Cloud Code endpoints、project/user context、Google OAuth credential behavior。 |
| `reverse-proxy-antigravity` | Antigravity Desktop / IDE 客户端 | Google Cloud Code / Antigravity internal endpoints、desktop/IDE token, user-agent, HTTP behavior, protocol-specific payload. |
| `custom_reverse_proxy` | Operator-defined upstream client | Explicit endpoint and egress profile defined by operator metadata. |

Implementation status:

- WP-400 implements the HTTP Codex CLI 2api path for text requests: `reverse-proxy-codex-cli` builds a Codex Responses request and sends `base_url + "/responses"` through Reverse Proxy Runtime.
- Codex Responses WebSocket upstream relay and richer prompt-cache/session policy are still follow-up work.

## 6. Boundary Rules

1. `runtime_class = api_key` defaults to official API-key adapter behavior unless an explicit adapter says otherwise.
2. `runtime_class != api_key` under a `reverse-proxy-*` adapter must use Reverse Proxy Runtime for all upstream HTTP/WSS calls.
3. Provider Adapter owns upstream official-client payload shape. Reverse Proxy Runtime must not invent business DTOs.
4. Reverse Proxy Runtime owns transport behavior: account credential injection, forbidden header stripping, user-agent selection, proxy binding, cookie jar, timeout, relay accounting, and runtime error classes.
5. Client Response Renderer owns downstream response shape. Upstream official-client SSE/WSS frames may be transformed only after the runtime has received them.
6. Every successful or failed 2api call must still produce Scheduler decision/feedback and usage evidence.

## 7. Implementation Test Implications

A valid 2api reverse-proxy test should prove at least:

- The selected account identity, not the caller credential, is used upstream.
- The upstream path/header/body matches the target official-client shape, not merely OpenAI-compatible defaults.
- SRapi and caller-only headers are stripped before the upstream request.
- Scheduler decision and usage evidence preserve the original downstream source endpoint.
- Failure classes such as `session_invalid`, `account_locked`, `account_banned`, `device_unrecognized`, `challenge_required`, `geo_blocked`, `timeout`, and `network_error` are mapped into account protection and feedback paths.

## 8. Source References

- MDN: https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Proxy_servers_and_tunneling
- Cloudflare: https://www.cloudflare.com/learning/cdn/glossary/reverse-proxy/
- 2api.ai public positioning for unified AI API proxy / control plane: https://2api.ai/
- Sub2API public description as a proxy consolidating Claude, OpenAI, Gemini, and Antigravity API access: https://shyft.ai/skills/sub2api
