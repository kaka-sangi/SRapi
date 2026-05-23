# SRapi 反代运行时与去特征规范

## 1. 目标

术语边界以 `2API_REVERSE_PROXY_DEFINITION.md` 为准：SRapi 的“反代 / 2api”只按本地 `/home/senran/Desktop/sub2api`、`/home/senran/Desktop/CLIProxyAPI`、`/home/senran/Desktop/chatgpt2api` 的 2api 做法解释，即 SRapi 使用选中 Provider Account 模拟目标官方客户端请求上游，而不是把本地 Codex / Claude Code / Antigravity 客户端作为下游入口，也不是按通用网络 reverse proxy 定义重新解释。

实现时不要再把“反代”解释成 Gateway service 本地 DTO、本地 CLI 进程代理、或普通 API-key upstream fallback。SRapi 2api 的判定点是：Provider Adapter 构造目标官方客户端 upstream endpoint/header/body/stream/WSS shape，Reverse Proxy Runtime 使用选中账号的 OAuth/session/desktop/CLI/IDE credential 发给真实上游。

SRapi 的核心差异化能力之一，是像 `sub2api`、`claude2api`、`chatgpt2api`、`gemini2api`、`grok2api`、`cursor2api`、`augment2api`、`copilot2api`、`antigravity2api` 这类 2api 项目一样，把上游 AI 厂商的 **Web 会话、桌面客户端 OAuth、CLI/IDE 设备码 token** 反向暴露为统一的 OpenAI / Anthropic / Gemini 兼容 API。

这与传统 API Key Gateway 的关键差别：

- 上游期望的请求方应为 **官方 Web / 桌面 / CLI 客户端**，不是 API SDK。
- 上游会通过 **TLS / HTTP/2 / Header / 行为指纹** 检测非官方客户端。
- 凭证不是 `sk-...`，而是 **cookie / session token / OAuth refresh token / device code**。
- 这类凭证一旦被识别为机器化使用，**账号会被锁定、限流或封禁**。

因此 SRapi 必须实现一个独立的 **Reverse Proxy Runtime（反代运行时）**，专门负责让上游看到的请求“尽可能像官方客户端发出”，而不是“来自一个 Gateway”。

## 2. 反代不是“透明转发”

反代运行时的真实含义：

```txt
用户客户端 -> SRapi 兼容端点 -> Canonical AI Request -> Provider Adapter 构造官方客户端形态 -> Reverse Proxy Runtime -> 上游官方客户端端点
```

特点：

- 用户客户端可以是任意 OpenAI / Anthropic 兼容 SDK。
- SRapi 在内部把请求翻译为 **目标上游官方客户端会发出的请求**，例如 Codex CLI、Claude Code CLI、Gemini CLI、Antigravity Desktop / IDE 的请求形态。
- Provider Adapter 负责上游官方客户端 payload / endpoint / protocol shape；Reverse Proxy Runtime 负责把这个请求“包装”成与官方客户端传输特征一致或接近一致的样子。
- 流式响应必须按上游官方客户端的格式向客户端原样输出，再转换回客户端协议。

WP-400 起，`reverse-proxy-codex-cli` 的 HTTP text path 已按上述定义实现：Adapter 构造 Codex Responses body/headers，Reverse Proxy Runtime 负责把选中账号身份发往配置的 Codex base URL `/responses`。WP-410 起，显式请求的 `/v1/responses/ws` 也可调度启用 WebSocket 的 Codex 反代账号，并以 Codex Responses WebSocket 官方客户端形态连接上游 `/responses`。WP-420 起，`reverse-proxy-claude-code-cli` 的 HTTP Messages path 构造 Claude Code 官方客户端形态：`/messages?beta=true`、Claude Code beta/version/stainless/session headers、Claude Code system/billing blocks，并由 Reverse Proxy Runtime 注入选中账号的 OAuth/CLI token。WP-560 起，Anthropic `/v1/messages/count_tokens` 和 provider alias 也进入同一边界：API-key Anthropic 账号调用 `/messages/count_tokens`，Claude Code 2api 账号调用 `/messages/count_tokens?beta=true`，Adapter 保留 Anthropic count_tokens body shape、映射 upstream model，并添加 Claude Code token-counting beta/headers/system-billing blocks。WP-430 起，`reverse-proxy-chatgpt-web` 的 HTTP text path 构造 ChatGPT Web 官方客户端形态：`/backend-api/conversation`、browser/OAI device-session/Sentinel headers、ChatGPT Web Conversation body，并由 Reverse Proxy Runtime 注入选中账号的 OAuth/Web session token。WP-440 起，ChatGPT Web Sentinel requirements 的 bootstrap 和 `/backend-api/sentinel/chat-requirements` 也通过 Reverse Proxy Runtime 使用同一选中账号上下文完成。WP-450 起，`reverse-proxy-antigravity` 的 HTTP text path 构造 Antigravity / Google Cloud Code 官方客户端形态：`/v1internal:generateContent` 或 `/v1internal:streamGenerateContent?alt=sse`、Antigravity `project`/`requestId`/`userAgent`/`requestType` envelope、嵌套 Gemini request body，并由 Reverse Proxy Runtime 注入选中桌面/IDE/OAuth token。WP-500 起，Antigravity model discovery 也通过 Reverse Proxy Runtime 使用同一选中账号身份 POST 到 `/v1internal:fetchAvailableModels`，而不是用 API key 或本地客户端进程取模型列表。WP-530 起，缺少 project metadata 的 Antigravity discovery 会先通过 Reverse Proxy Runtime 使用选中账号请求 `/v1internal:loadCodeAssist`，必要时再请求 `/v1internal:onboardUser` 取得 `cloudaicompanionProject`，然后再调用 model discovery；只有 `persist=true` 时才把解析到的 project 写回账号 metadata。这不是把本地 Codex/Claude/Antigravity 客户端接入 SRapi，也不是把下游请求原样转发到 OpenAI/Anthropic/Gemini-compatible 普通 API。

反代运行时服务于 `runtime_class != api_key` 的账号。`runtime_class = api_key` 的账号必须走官方 API-key Adapter 路径，不属于 SRapi 2api 反代；`reverse-proxy-*` Adapter 不得依赖 Reverse Proxy Runtime 从 API key 凭证注入上游身份。

## 3. 账号运行时分类

SRapi 必须支持以下 runtime class，每一类有不同的反代要求。

```txt
api_key
oauth_refresh
oauth_device_code
web_session_cookie
desktop_client_token
cli_client_token
ide_plugin_token
service_account_json
custom_reverse_proxy
```

字段建议：

```txt
account.runtime_class
account.upstream_client     例如 chatgpt_web、claude_web、claude_code_cli、codex_cli、gemini_cli、cursor_ide、augment_ide、copilot_ide、antigravity_desktop、grok_web
account.egress_profile_id   反代指纹模板 ID
account.proxy_id            出口代理
account.cookie_jar_id       独立 cookie jar
account.device_fingerprint  仅 web/desktop/ide 必填
```

不同 runtime class 必须使用不同的 Provider Adapter。

## 4. 反代调用链路

```txt
Selected Provider Adapter
  ↓
Reverse Proxy Runtime
  ├─ Egress Profile Resolver
  ├─ Credential Materializer
  ├─ Cookie Jar Loader
  ├─ Proxy Selector
  ├─ TLS / HTTP/2 Impersonator
  ├─ Header Hygiene Filter
  ├─ Body Passthrough Guard
  ├─ Stream Relay
  ├─ Challenge Handler  optional
  └─ Behavior Pacer
  ↓
Upstream Official Endpoint
```

每一步都必须可关闭、可替换、可观测。

## 5. Egress Profile（指纹模板）

Egress Profile 描述“模拟哪一个官方客户端”。

字段：

```txt
id
name
upstream_client       chatgpt_web / claude_web / codex_cli / claude_code_cli / gemini_cli / cursor / augment / copilot / antigravity / grok_web / chrome_browser_lts / firefox_browser_lts
tls_template          utls 模板枚举，例如 chrome_120、firefox_120、safari_17、ios_17、android_chrome_120、openai_python_sdk、anthropic_python_sdk
http2_template        SETTINGS frame + window size + pseudo header order
http_version_policy   prefer_h2 / require_h2 / require_h1
user_agent
header_order_template
header_set_template
accept_language
accept_encoding
sec_ch_ua_template
extra_static_headers
forbidden_headers
body_encoding         identity / gzip / br
stream_format         sse / wss / chunked
behavior_pacer        official_client_realistic / minimal_throttle / aggressive
challenge_strategy    none / passthrough / external_solver
client_version_pin    例如 "ChatGPT/1.2025.5"
last_validated_at
version
```

约束：

- Egress Profile 必须可在管理后台版本化更新。
- Egress Profile 不写死在代码，必须可热加载。
- 每个 `upstream_client` 至少要有一个默认 Profile。
- 升级 Profile 不得改写旧 decision 记录。

当前代码内置的最小默认 User-Agent 覆盖 CLI/桌面身份，包括 `codex_cli`、`claude_code_cli`、
`gemini_cli` 和 `antigravity_desktop`。完整 TLS/HTTP2 指纹、header 顺序和版本化 Profile
仍属于 Egress Profile 管理面；账号 metadata 中的 `user_agent` 可覆盖默认值。

## 6. TLS / JA3 / JA4 指纹

Go 标准 `crypto/tls` 输出的 JA3/JA4 是公开已知特征，必须替换。

要求：

- 使用 `utls`（`refraction-networking/utls`）或同等能力库。
- 每个 Egress Profile 绑定一个 `ClientHelloID` 或自定义 ClientHello 规范。
- 必须实现：
  - Cipher suites 顺序匹配
  - Extensions 顺序匹配
  - Supported versions
  - Supported groups
  - Signature algorithms
  - ALPN
  - GREASE 行为
  - 会话票据策略
  - Key share 顺序

测试要求：

- 必须有 JA3/JA4 验证测试，对比目标 Profile 的真实指纹。
- 任何 Profile 升级必须重新跑指纹快照测试。

## 7. HTTP/2 与 HTTP/3 指纹

要求：

- 必须能控制 SETTINGS frame 顺序与值，至少：
  - HEADER_TABLE_SIZE
  - ENABLE_PUSH
  - MAX_CONCURRENT_STREAMS
  - INITIAL_WINDOW_SIZE
  - MAX_FRAME_SIZE
  - MAX_HEADER_LIST_SIZE
- 必须能控制 WINDOW_UPDATE 大小。
- 必须能控制伪头顺序：
  - `:method`
  - `:authority`
  - `:scheme`
  - `:path`
- 必须能控制 PRIORITY frame 行为。
- HTTP/3 暂不在 MVP，但 Egress Profile 必须预留 `http3_template` 字段。

## 8. Header 去特征

### 8.1 强制要求

- 严格按 Egress Profile 的 `header_order_template` 输出。
- 严格使用 Profile 中的大小写。
- 必须按 Profile 输出 `User-Agent`。
- `sec-ch-ua`、`sec-ch-ua-platform`、`sec-ch-ua-mobile` 等浏览器特征头必须与 Profile 一致。
- `x-stainless-*`、`anthropic-version`、`openai-beta`、`x-request-source` 等 SDK 特征头必须按 Profile 选择性添加。

### 8.2 禁止行为

反代请求向上游发出的 HTTP 头中，**禁止出现** SRapi 自身可识别的痕迹：

```txt
X-Request-ID            禁止透传 SRapi 内部 trace id
X-Forwarded-For         默认禁止
X-Forwarded-Host        默认禁止
X-Forwarded-Proto       默认禁止
Forwarded               默认禁止
Via                     默认禁止
X-SRapi-*               永久禁止
X-Gateway-*             永久禁止
Server                  禁止泄漏 SRapi 自身名称
User-Agent: SRapi/*     永久禁止
```

可选透传必须由 Egress Profile 显式开启，并模拟成上游官方客户端会发出的字段。

### 8.3 Cookie

- 每账号独立 cookie jar。
- cookie jar 在数据库中加密保存。
- cookie 不得跨账号共享。
- 必须保留上游 `Set-Cookie` 的属性（HttpOnly、Secure、SameSite、Domain、Path、Expires）。
- 必须支持 `cf_clearance`、`__cf_bm`、`__Secure-next-auth.session-token`、`sessionKey`、`SAPISID`、`SAPISIDHASH` 等典型 cookie。

### 8.4 Authorization / API Key

- Web session：不发送 `Authorization`，只发送 cookie。
- OAuth refresh：按上游官方客户端使用方式发送，例如 `Authorization: Bearer <access_token>`。
- Device code：按官方客户端协议发送 token。
- 不得把上游 API Key 错误地用作 OAuth bearer。

## 9. Body 透传与不改写

反代请求 body 必须严格按目标上游官方客户端的格式构造，且**不得包含 SRapi 自身的扩展字段**。

要求：

- 不得在 body 中追加 `metadata.srapi_*`。
- 不得在 body 中追加 `request_id`（除非上游官方客户端本身会发）。
- 不得在 body 中保留 `compatibility_warnings` 等 SRapi 内部字段。
- JSON 序列化必须按上游客户端的字段顺序与缩进风格输出。
- 不得静默对 body 做 gzip / br 重新压缩或解压，除非 Egress Profile 显式声明。
- 多模态二进制（图片、音频）必须按上游官方客户端的方式上传，不得使用 SRapi 自有上传通道。

## 10. SSE / WSS 流式透传

反代流式响应链路必须做到**对上游官方客户端字节级一致或接近一致**：

- SSE chunk 必须按上游格式原样回写，不得改 `data:` 行号或合并 chunk。
- 不得修改 SSE event name。
- 不得对响应体做二次 gzip / br 编码。
- 不得在 chunk 之间插入 SRapi 心跳。
- WSS：必须以独立连接、独立握手、独立 ping/pong 节奏处理，禁止 SRapi 共用连接。
- 流式中断必须按上游官方客户端的方式标记，并通过 feedback 报告 partial failure。

Gateway 在将上游 chunk 渲染回客户端协议时（例如把 Claude Web 的 SSE 渲染为 OpenAI Chat Completions SSE），转换必须在 **Client Response Renderer** 层进行，反代运行时本身只保证“原样接收上游”。

WP-390 起，Reverse Proxy Runtime 提供直接 WebSocket relay primitive：

- 调用方通过 `WebSocketRuntime.RelayWebSocket` 传入已选中的账号 runtime context、目标 WebSocket URL、可选子协议和双向 message channel。
- runtime 使用与 HTTP 反代相同的 per-account client/proxy/cookie jar、Credential auth injection、User-Agent 选择和 forbidden-header 过滤。
- `Authorization`、`Cookie`、`Sec-WebSocket-*`、`X-Request-ID`、`X-Forwarded-*`、`Via`、`X-SRapi-*`、`X-Gateway-*` 等 caller/gateway header 不得透传；认证材料只能来自选中账号的 credential。
- relay 支持 text/binary message 透传并返回基础 message/byte accounting。provider-native realtime event schema、slot lifecycle 和 Gateway binding 仍由后续 adapter/runtime package 实现。
- WP-410 将该 primitive 绑定到 Codex Responses WebSocket 2api 路径：Gateway 只在 `upstream_ws` / `codex_responses_websocket` 明确启用时尝试，Provider Adapter 生成 Codex WebSocket URL/headers/首帧，Runtime 负责使用选中账号凭证拨号和 relay。更复杂的 session reuse、slot lifecycle、Claude Code 和 Antigravity WebSocket 协议仍是后续包。
- WP-420 将 Claude Code HTTP Messages 2api 绑定到 Reverse Proxy Runtime：Provider Adapter 生成 `/messages?beta=true`、Claude Code 官方客户端 headers/body；Runtime 负责使用选中账号凭证发出请求。Claude Code WebSocket/session slot 生命周期仍是后续包。
- WP-460 将 provider-neutral realtime slot lifecycle 绑定到 `/v1/responses/ws`：Gateway 在 WebSocket upgrade 前获取 slot、在关闭/错误时释放，并通过 deploy-level global/per-API-key 限额保护长连接资源。Reverse Proxy Runtime 仍只负责上游 WSS relay；slot manager 不包含 provider-specific DTO。
- WP-470 将 OpenAI-compatible Realtime `GET /v1/realtime` 绑定到 Reverse Proxy Runtime：Gateway 解析 query `model`，调度具备 `realtime_websocket` 能力的账号，Provider Adapter 构造上游 `/realtime?model=<mapped_upstream_model>` WebSocket session，Runtime 使用选中账号 OAuth/session/client-token credential 注入上游身份并双向 relay text/binary frames。该路径仍不把 caller 的 `Authorization`、`Cookie` 或 SRapi headers 透传给上游，也不是 `POST /v1/realtime`。
- WP-570 将当前节点 active realtime slot 安全摘要暴露到 `GET /api/v1/admin/ops/realtime/slots`，用于运维诊断。该 AdminOps 接口只读取 slot lifecycle metadata 和 hash 后的 affinity key，不读取 Reverse Proxy Runtime credential/cookie jar，也不返回上游 realtime frames。

## 11. 出口 IP 与代理绑定

要求：

- 每账号可绑定一个 `proxy_id`。
- 同一账号在长时间内必须使用同一出口 IP，不得在每次请求中跨 IP 切换。
- 不同账号必须可以走不同出口 IP。
- 代理类型支持：`http`、`https`、`socks5`、`socks5h`。
- 代理凭证加密存储。
- 必须支持按 `Egress Profile` 限制可用国家/区域，避免 cookie 与 IP 国别明显不匹配。

可选能力：

- 同账号支持 IP 漂移窗口（例如 24 小时内最多 1 次切换）。
- 出口 IP 健康检测（黑名单、速度、稳定性）。

## 12. 账号上下文隔离

每个 `runtime_class != api_key` 的账号必须拥有：

```txt
独立 TLS 会话上下文（含 TLS session cache）
独立 HTTP/2 连接池
独立 cookie jar
独立 User-Agent 与 Egress Profile 绑定
独立 proxy_id
独立 device_fingerprint（如适用）
独立 challenge token 缓存
独立 OAuth refresh 锁
```

禁止：

- 多账号共用同一 HTTP client。
- 多账号共用 cookie jar。
- 多账号共用 cf_clearance。
- 在多账号之间复用 device fingerprint。

## 13. OAuth / Refresh Token

要求：

- 必须支持 access token 自动刷新。
- 刷新过程必须加分布式锁，避免并发刷新。
- 刷新失败不得覆盖旧凭证。
- 刷新成功后必须重新加密存储。
- 必须记录刷新事件审计。
- 必须支持 Device Code 流程的轮询与最终凭证落库。

支持的典型上游：

```txt
chatgpt_web
codex_cli
claude_web
claude_code_cli
gemini_cli
cursor_ide
augment_ide
copilot_ide
antigravity_desktop
grok_web
```

## 14. 反爬挑战处理

反代运行时必须为 Cloudflare、Arkose、Turnstile、JS challenge 等保留集成点。

策略：

```txt
challenge_strategy = none
challenge_strategy = passthrough
challenge_strategy = external_solver
```

要求：

- `none`：上游不需要挑战，例如 OAuth API 路径。
- `passthrough`：把挑战内容透传给账号管理员手工解决，缓存结果。
- `external_solver`：调用外部解题服务，必须支持禁用、超时、成本上限。

约束：

- 不得在代码中硬编码解题密钥。
- 挑战 token 必须按 cookie jar 绑定。
- 解题失败必须降权账号，不得无限重试。

## 15. 行为节奏与反指纹

上游普遍会基于行为指纹判断是否机器化。

要求：

- 必须支持 `behavior_pacer = official_client_realistic`：
  - 单账号 RPM 不得超过官方客户端常见上限。
  - 同账号连续请求间最小 jitter 必须可配置。
  - 不允许同账号在毫秒级并发多请求，除非 Egress Profile 显式声明。
- 必须支持 `behavior_pacer = minimal_throttle`：
  - 仅遵守上游硬限，不额外抖动。
- 调度器必须把节奏限制作为硬过滤的一部分，不能让评分覆盖节奏。
- 在长时间空闲后突然爆发，必须先进入 ramp-up 模式，避免“沉默后猛打”这种典型 bot 特征。

## 16. 错误识别与封号信号

反代运行时必须把上游的封号或风控信号识别出来，避免继续打：

错误分类增加（与 `PROVIDER_ADAPTER_SPEC.md` 对齐）：

```txt
challenge_required
captcha_required
session_invalid
account_locked
account_banned
abuse_detected
geo_blocked
device_unrecognized
upstream_client_outdated
```

策略：

- `session_invalid`、`account_locked`、`account_banned`、`abuse_detected`：账号立即进入 `needs_reauth` 或 `disabled`，不得继续调度。
- `challenge_required`、`captcha_required`：进入冷却，触发挑战处理。
- `geo_blocked`：标记账号与出口 IP 不兼容。
- `device_unrecognized`：标记账号与当前 device fingerprint 不兼容，强制重新配对。
- `upstream_client_outdated`：标记 Egress Profile 过期，触发管理员告警。

所有信号必须写入 audit 和 feedback，并显式提示用户。

## 17. 可观测性

必须暴露 metrics：

```txt
reverse_proxy_request_total{upstream_client, egress_profile}
reverse_proxy_request_success_total{upstream_client}
reverse_proxy_request_error_total{upstream_client, error_class}
reverse_proxy_challenge_total{upstream_client, strategy}
reverse_proxy_account_locked_total{upstream_client}
reverse_proxy_account_banned_total{upstream_client}
reverse_proxy_oauth_refresh_total{upstream_client, status}
reverse_proxy_websocket_relay_total{upstream_client, status}
reverse_proxy_proxy_failure_total{proxy_type}
reverse_proxy_ja3_mismatch_total
reverse_proxy_h2_mismatch_total
reverse_proxy_header_leak_total
```

必须有专门面板：

- 每 `upstream_client` 当前可用账号数。
- 最近 24h 封号 / 锁定事件。
- 挑战命中率与平均耗时。
- Egress Profile 版本与生效账号数。
- 出口 IP 健康与封禁情况。

## 18. 安全与凭证保护

- cookie、OAuth token、device code、refresh token 必须按 `SECURITY_MODEL.md` 的 AES-256-GCM 加密存储。
- 解密只能发生在反代运行时实际发起请求的路径上。
- 日志中默认禁止打印 cookie、token、device fingerprint。
- 调试模式必须脱敏。
- 不得把反代凭证或 cookie 写入 `usage_logs.metadata` 或 `scheduler_decisions.scores_json`。
- 备份导出必须二次加密或屏蔽。

## 19. ToS 与合规边界

- SRapi 仅提供能力，不内置违反任何上游 ToS 的硬编码绕过。
- 解题、Token 获取、cookie 抓取等敏感行为必须由部署者手动触发或外接服务。
- README 与管理后台必须显式提示：反代运行时可能与上游 ToS 冲突，使用者承担合规和封号风险。
- 不得默认开启对未配置账号的任意上游探测。

## 20. MVP 范围

MVP 必须实现：

```txt
account.runtime_class enum
account.upstream_client 字段
account.cookie_jar / oauth_refresh / device_code 凭证类型与加密存储
独立 HTTP client：每账号独立 cookie jar、UA、proxy
反代请求 Header Hygiene Filter（禁止泄漏 SRapi 头）
反代请求 Body Passthrough Guard（不追加 SRapi 字段）
SSE 字节透传，无中间合并
OAuth refresh token 自动刷新接口
challenge_required / session_invalid / account_locked / account_banned 错误分类
错误命中后账号自动 needs_reauth 或 disabled
反代专用 metrics
```

MVP 暂缓：

```txt
utls 完整 TLS impersonation
HTTP/2 SETTINGS / WINDOW_UPDATE 精细模拟
Egress Profile 库（多客户端版本）
Behavior Pacer 高级模式
外部 challenge solver 集成
HTTP/3 模拟
WSS / Realtime 反代
```

## 21. Phase 2

```txt
utls + HTTP/2 完整指纹模拟
Egress Profile 管理后台与版本化
浏览器与桌面客户端模板：chrome_lts、chatgpt_desktop、claude_desktop、codex_cli、claude_code_cli、gemini_cli、cursor、augment、copilot、antigravity、grok_web
Behavior Pacer official_client_realistic
出口 IP 健康检测
挑战 token 缓存与外部 solver 接入
```

## 22. Phase 3+

```txt
WSS / Realtime 反代
HTTP/3 模拟
自动指纹库 OTA 更新
风控反馈驱动的自适应节奏
多 device fingerprint 池
账号生命周期自动迁移：抓取 -> 验证 -> 投产 -> 风控降级 -> 重新换绑
```

## 23. 测试要求

- TLS JA3/JA4 快照测试，匹配 Egress Profile 目标值。
- HTTP/2 SETTINGS 快照测试。
- Header 顺序与大小写黄金测试。
- 禁止头泄漏单元测试。
- SSE 字节级 diff 测试：相同上游响应，反代输出与原始官方客户端抓包字节一致。
- OAuth refresh 并发锁测试。
- cookie jar 隔离测试。
- 每账号独立 HTTP client 测试。
- 封号信号识别与账号自动 disabled 测试。
- Behavior Pacer 节奏单元测试。
- 出口 IP 漂移限制测试。

## 24. 与其他文档的关系

- 通用 Provider Adapter 规则见 `PROVIDER_ADAPTER_SPEC.md`，反代运行时是其下的一个特殊运行时类。
- 端点协议互转规则见 `AI_ENDPOINT_COMPATIBILITY.md`，反代运行时只保证上游侧字节级模拟，端点互转在 Client Endpoint Adapter / Client Response Renderer 层完成。
- 凭证加密、日志脱敏、密钥轮换以 `SECURITY_MODEL.md` 为准。
- 调度器只通过 `runtime_class` 与 capability 做选择，不依赖具体 cookie 或 token 内容。
