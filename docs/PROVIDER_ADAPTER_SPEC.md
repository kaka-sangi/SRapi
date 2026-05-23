# SRapi Provider Adapter 规范

## 1. 目标

Provider Adapter 是 SRapi 连接不同 AI 服务商和兼容协议的扩展点。

目标：

- 新增 Provider 不修改 Gateway 主流程。
- 新增 Provider 不修改 Scheduler 核心评分逻辑。
- Provider-specific 逻辑封装在 Adapter 内。
- 统一请求、响应、错误、usage 和流式处理。
- 为调度内核提供能力声明、错误分类和成本信息。
- 支持 Chat Completions、Responses、Messages、GenerateContent 等主流端点通过 Canonical AI Request 相互转换。

## 2. 适配器边界

Provider Adapter 负责：

- Provider 能力声明。
- 账号凭证解析和认证注入。
- 请求转换。
- 响应转换。
- 流式响应解析。
- 错误分类。
- Usage 解析。
- Cache usage 解析。
- 健康检查。
- OAuth refresh，可选。

Provider Adapter 不负责：

- 用户鉴权。
- API Key 鉴权。
- 账号选择。
- 用户计费。
- 订阅权限判断。
- 调度策略评分。
- 数据库存储细节。

## 3. Provider 类型

第一阶段支持类型：

```txt
openai-compatible
anthropic-compatible
gemini-compatible
native-openai
native-anthropic
native-gemini
native-grok
openrouter
reverse-proxy-chatgpt-web
reverse-proxy-codex-cli
reverse-proxy-claude-web
reverse-proxy-claude-code-cli
reverse-proxy-gemini-cli
reverse-proxy-grok-web
reverse-proxy-cursor
reverse-proxy-augment
reverse-proxy-copilot
reverse-proxy-antigravity
```

`reverse-proxy-*` 类 Adapter 必须经由 `REVERSE_PROXY_SPEC.md` 定义的 Reverse Proxy Runtime 发起上游请求。
`reverse-proxy-*` 是 2api / official-client simulation 路径，必须使用 OAuth、session、desktop、CLI 或 IDE token 等非 API-key 运行时身份；`runtime_class = api_key` 必须使用对应官方 API-key Adapter，不得作为 2api 反代账号进入 Reverse Proxy Runtime。
这里的 2api 定义已经由 `2API_REVERSE_PROXY_DEFINITION.md` 锁定为 `sub2api` / `CLIProxyAPI` / `chatgpt2api` 风格：Adapter 模拟目标官方客户端请求真实上游。不得把 `reverse-proxy-*` 实现成 Gateway-local DTO、本地 Codex/Claude/Antigravity 进程入口、或通用兼容 API 透传。

每个 Adapter 必须声明 runtime_class：

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

命名约束：

```txt
provider.name         业务服务商实体名称，例如 openai、anthropic、openrouter、自定义上游名称。
provider.adapter_type 代码适配器类型，必须使用上方枚举之一。
provider.protocol     协议风格，例如 openai-compatible、anthropic-compatible。
```

统一使用 `anthropic-compatible`，不使用 `claude-compatible`。

其中 MVP 优先：

```txt
openai-compatible
anthropic-compatible endpoint rendering
native-openai 或 openrouter
```

已实现的文本上游 dispatch：

```txt
openai-compatible       -> /chat/completions
anthropic-compatible    -> /messages
gemini-compatible       -> /models/{model}:generateContent 或 :streamGenerateContent
native-gemini           -> /models/{model}:generateContent 或 :streamGenerateContent
reverse-proxy-chatgpt-web -> Reverse Proxy Runtime + ChatGPT Web /backend-api/conversation payload
reverse-proxy-gemini-cli -> Reverse Proxy Runtime + Gemini GenerateContent payload
reverse-proxy-antigravity -> Reverse Proxy Runtime + Antigravity / Google Cloud Code v1internal payload
```

WP-430 ChatGPT Web 2api boundary:

- `reverse-proxy-chatgpt-web` dispatches OpenAI-compatible text requests through Reverse Proxy Runtime, not through direct API-key HTTP.
- The upstream endpoint is `{chatgpt_origin}/backend-api/conversation`; account metadata should store `base_url` at the ChatGPT Web origin, not at `/v1/chat/completions`.
- Adapter-owned official-client shape includes browser `Origin` / `Referer` / `Sec-*`, `OAI-*`, `X-OpenAI-Target-*`, Sentinel requirements headers, and ChatGPT Web Conversation body fields such as `action`, `parent_message_id`, `conversation_mode`, `force_use_sse`, timezone, and `client_contextual_info`.
- If no static Sentinel requirements token is configured, the adapter bootstraps ChatGPT Web and requests `/backend-api/sentinel/chat-requirements` through Reverse Proxy Runtime before the conversation request; `chatgpt_requirements_auto=false` disables this behavior.
- Arkose and Turnstile requirements are surfaced as reverse-proxy challenge classes unless an operator-provided challenge token is already available.
- Runtime-owned transport still injects the selected account credential as OAuth/Web-session bearer or cookie and strips caller/SRapi headers; adapter must not forward caller `Authorization`.
- `runtime_class = api_key` is rejected for this adapter because ChatGPT Web 2api means official Web OAuth/session simulation, not OpenAI API-key mode.

WP-420 Claude Code 2api boundary:

- `reverse-proxy-claude-code-cli` dispatches Anthropic Messages-shaped text requests through Reverse Proxy Runtime, not through direct API-key HTTP.
- The upstream endpoint is `{base_url}/messages?beta=true`; account metadata should store `base_url` at the `/v1` prefix.
- Adapter-owned official-client shape includes Claude Code `Anthropic-Beta`, `Anthropic-Version`, `X-App`, `X-Stainless-*`, `X-Claude-Code-Session-Id`, `x-client-request-id`, stream/non-stream `Accept`, and Claude Code system/billing blocks.
- Runtime-owned transport still injects the selected account credential as OAuth/CLI bearer and strips caller/SRapi headers; adapter must not forward caller `Authorization` or add `x-api-key`.
- `runtime_class = api_key` is rejected for this adapter because Claude Code 2api means official-client OAuth/session/client-token simulation, not Anthropic API-key mode.

WP-450 Antigravity 2api boundary:

- `reverse-proxy-antigravity` dispatches text requests through Reverse Proxy Runtime to Antigravity / Google Cloud Code `v1internal` endpoints, not to generic `/chat/completions`, `/messages`, or public Gemini `models/{model}:generateContent`.
- `provider.protocol` still describes the downstream/client source protocol and Gateway normalization path, but Antigravity upstream text dispatch always uses the official-client envelope with nested Gemini request fields.
- The upstream endpoint is `{base_url}/v1internal:generateContent` or `{base_url}/v1internal:streamGenerateContent?alt=sse`; account metadata should store `base_url` at the Cloud Code origin such as `https://cloudcode-pa.googleapis.com`.
- Adapter-owned official-client shape includes `project`, `requestId`, `userAgent: antigravity`, `requestType`, mapped upstream `model`, nested `request.contents`, optional `systemInstruction`, `generationConfig`, `tools`, `toolConfig`, safety settings, and `sessionId`.
- Runtime-owned transport injects the selected account credential as desktop/IDE/OAuth bearer and strips caller/SRapi headers; adapter must not forward caller `Authorization`.
- `runtime_class = api_key` is rejected for this adapter because Antigravity 2api means official desktop/IDE/OAuth simulation, not Google/Gemini API-key mode.
- WP-500 adds Antigravity model discovery for existing configured accounts: admin `discover-models` posts `{base_url}/v1internal:fetchAvailableModels` through Reverse Proxy Runtime, includes `project_id` when configured, parses upstream `models`, and persists `supported_models` only when requested.
- WP-530 adds Antigravity project bootstrap for discovery: when `project_id` / `antigravity_project_id` / `cloudaicompanion_project` is missing, discovery uses the selected account credential through Reverse Proxy Runtime to call `/v1internal:loadCodeAssist` and, if needed, `/v1internal:onboardUser` before fetching models. This is selected-account 2api bootstrap, not local Antigravity process execution.
- Full Antigravity OAuth onboarding UI/API, credit overage retry policy, full schema cleaning, and persistent realtime session lifecycle remain follow-up packages.

## 4. Adapter 生命周期

```txt
Register
  ↓
Initialize
  ↓
Declare Capabilities
  ↓
Validate Account
  ↓
Dispatch Request
  ↓
Parse Response / Stream
  ↓
Classify Error
  ↓
Report Usage
  ↓
Shutdown
```

## 5. 核心接口草案

```go
type ProviderAdapter interface {
    Name() string
    Capabilities(ctx context.Context) ProviderCapabilities
    ValidateAccount(ctx context.Context, account ProviderAccount) error
    HealthCheck(ctx context.Context, account ProviderAccount) (*ProviderHealth, error)
    BuildRequest(ctx context.Context, req GatewayRequest, account ProviderAccount) (*ProviderRequest, error)
    Send(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error)
    Stream(ctx context.Context, req *ProviderRequest) (ProviderStream, error)
    ParseUsage(ctx context.Context, resp *ProviderResponse) (*Usage, error)
    ClassifyError(err error) ProviderError
}
```

流式接口草案：

```go
type ProviderStream interface {
    Recv() (*StreamChunk, error)
    Close() error
    Usage() *Usage
}
```

## 6. ProviderCapabilities

Adapter 必须声明能力。能力命名、版本、状态、降级和 metadata schema 以 `CAPABILITY_TAXONOMY_SPEC.md` 为准。

```txt
provider_name
protocol
supports_chat_completions
supports_responses
supports_messages
supports_generate_content
supports_embeddings
supports_images
supports_audio_speech
supports_moderations
supports_rerank
supports_stream
supports_tools
supports_parallel_tool_calls
supports_vision
supports_json_mode
supports_structured_output
supports_reasoning
supports_stateful_responses
supports_prompt_cache
supports_context_cache
supports_usage_in_stream
supports_oauth_refresh
rate_limit_model
quota_model
```

WP-290 起，`supports_images` 映射到 canonical `images` endpoint capability。Gateway image generation 请求必须带 `images` request capability；OpenAI-compatible API-key 和 reverse-proxy accounts 使用 `/images/generations` 上游路径，并解析 `url` / `b64_json` image outputs。WP-480 起，Gateway image edit 请求也使用 `images` request capability；OpenAI-compatible API-key 和 reverse-proxy accounts 使用 multipart `/images/edits` 上游路径，转发 `image` / `image[]`、可选 `mask` 和输出选项，并解析同一 OpenAI-compatible image response。WP-510 起，下游 JSON image references 会在 HTTP/Gateway 层解码成同一个 canonical image edit request，Provider Adapter 仍只接收已归一化的 image bytes 并以上游 multipart `/images/edits` 发出；remote URL 和 `file_id` references 仍在 Gateway 层拒绝。WP-520 起，`stream=true` 的 image edit 请求仍经过同一 Provider Adapter 和 usage 证据链，但当前 v1 只把最终 image response 重新包装成 `image.generation.result` SSE；上游 progressive image SSE relay 留待后续包。WP-490 起，Gateway image variation 请求也使用 `images` request capability；OpenAI-compatible API-key 和 reverse-proxy accounts 使用 multipart `/images/variations` 上游路径，转发单个 source `image`、`n`、`size`、`response_format` 和 `user`，并解析同一 OpenAI-compatible image response。OpenAI 官方 upstream 当前仅支持 `dall-e-2`，SRapi 通过 model mapping 决定上游模型名。

WP-310 起，`supports_moderations` 映射到 canonical `moderations` endpoint capability。Gateway moderation 请求必须带 `moderations` request capability；OpenAI-compatible API-key 和 reverse-proxy accounts 使用 `/moderations` 上游路径，并解析 `flagged`、`categories`、`category_scores` 和 `category_applied_input_types`。

WP-320 起，`supports_rerank` 映射到 canonical `rerank` endpoint capability。Gateway rerank 请求必须带 `rerank` request capability；rerank-compatible API-key 和 reverse-proxy accounts 使用 `/rerank` 上游路径，并解析 `index`、`relevance_score`、可选 `document` 和可选 usage。

WP-330 起，`supports_audio_transcriptions` 映射到 canonical `audio_transcriptions` endpoint capability。Gateway audio transcription 请求必须带 `audio_transcriptions` request capability；OpenAI-compatible API-key 和 reverse-proxy accounts 使用 `/audio/transcriptions` 上游路径，并解析 transcription JSON/verbose JSON 或 plain text fallback。

WP-340 起，`supports_audio_speech` / `supports_speech` 映射到 canonical `audio_speech` endpoint capability。Gateway audio speech 请求必须带 `audio_speech` request capability；OpenAI-compatible API-key 和 reverse-proxy accounts 使用 `/audio/speech` 上游路径，并返回 binary audio bytes 与 content type。

WP-320 rerank adapter boundary:

- Rerank-compatible API-key accounts dispatch rerank requests to `{base_url}/rerank`.
- Reverse-proxy rerank-compatible accounts use the same route through Reverse Proxy Runtime with account runtime context.
- Adapter input includes mapped upstream model, query, string/object documents, optional `top_n`, optional `return_documents`, and optional user.
- Adapter output preserves upstream result order, index, relevance score, optional returned document, and usage when present.
- Providers that do not advertise `rerank` capability are not eligible for rerank Gateway scheduling.

WP-270 embeddings adapter boundary:

- OpenAI-compatible API-key accounts dispatch embedding requests to `{base_url}/embeddings`.
- Reverse-proxy OpenAI-compatible accounts use the same route through Reverse Proxy Runtime with account runtime context.
- Adapter input includes mapped upstream model, string-array input, encoding format, optional dimensions, and optional user.
- Adapter output preserves OpenAI-shaped embedding order/index and parses prompt/total token usage as input-token usage.
- Providers that do not advertise `embeddings` capability are not eligible for embeddings Gateway scheduling.

上述 `supports_*` 字段是实现 DTO 的便利表达，必须映射为 canonical capability descriptor：

```txt
key
version
status
level
source
metadata_json
```

Provider Adapter 不得发明未登记的 capability key。新增能力必须先更新 `CAPABILITY_TAXONOMY_SPEC.md` 和对应测试。

## 7. Model Capability 映射

Adapter 必须能处理内部模型到上游模型的映射。

输入：

```txt
canonical_model
provider
account
request_capabilities
```

输出：

```txt
upstream_model_name
capability_override
pricing_override
```

规则：

- 模型映射优先来自数据库 `model_provider_mappings`。
- Adapter 可以提供默认映射。
- 数据库配置优先于 Adapter 默认映射。
- `capability_override` 必须使用 `CAPABILITY_TAXONOMY_SPEC.md` 的 descriptor 结构。
- 最终能力由 ModelCapability、ProviderCapability、MappingOverride 和 AccountRuntimeState 共同计算，Scheduler 只消费 EffectiveCapability。

## 8. 账号凭证模型

Provider Account 的 credential 解密后交给 Adapter。

凭证类型：

```txt
api_key
oauth_access_token
oauth_refresh_token
oauth_device_code
web_session_cookie
desktop_session_token
cli_device_token
ide_plugin_token
service_account_json
custom_headers
custom_reverse_proxy_payload
```

Adapter 负责把凭证注入请求。

要求：

- `runtime_class != api_key` 的凭证必须通过 Reverse Proxy Runtime 注入，不得绕过。
- 反代凭证（cookie、OAuth token、device code）必须按 `SECURITY_MODEL.md` 加密存储。
- Adapter 不得在请求或响应中向客户端泄漏反代凭证。

禁止：

- Adapter 自行持久化明文凭证。
- Adapter 在日志中输出凭证。
- Adapter 将凭证泄漏到错误 details。
- Adapter 自行决定 TLS / HTTP/2 / Header / 出口 IP 等去特征参数，这些必须由 Reverse Proxy Runtime 按 Egress Profile 控制。

## 9. 请求标准化

Gateway 内部标准请求必须以 `AI_ENDPOINT_COMPATIBILITY.md` 的 Canonical AI Request 为准。

基础字段：

```txt
request_id
source_protocol
source_endpoint
response_protocol
model
canonical_model
input_items
messages
instructions
stream
temperature
top_p
max_output_tokens
tools
tool_choice
response_format
json_schema
reasoning
metadata
provider_options
compatibility_warnings
```

Adapter 根据 Provider 协议转换。

示例：

- OpenAI-compatible：大部分字段透传。
- OpenAI Responses：input/instructions/tools/reasoning/text.format 需要转换。
- Anthropic Messages：messages/system/max_tokens/tools/thinking 需要转换。
- Gemini：contents/generationConfig/tools 需要转换。

## 10. 响应标准化

Adapter 输出统一响应必须以 `AI_ENDPOINT_COMPATIBILITY.md` 的 Canonical AI Response 为准。

基础字段：

```txt
id
request_id
model
canonical_model
provider
account_id
output_items
message
choices
usage
cache_usage
raw_provider_metadata
compatibility_warnings
```

流式响应输出统一 chunk：

```txt
request_id
index
delta
finish_reason
usage_optional
raw_event_type
```

Gateway 再转换为客户端需要的 OpenAI-compatible 格式。
如果客户端调用的是 `/v1/messages` 或 `/v1/responses`，Gateway 必须渲染回对应源端点格式。

## 11. 错误分类

Adapter 必须把上游错误转换为内部错误分类。

分类：

```txt
rate_limit
quota_exceeded
auth_failed
permission_denied
model_unavailable
invalid_request
content_policy
provider_5xx
network_error
timeout
challenge_required
captcha_required
session_invalid
account_locked
account_banned
abuse_detected
geo_blocked
device_unrecognized
upstream_client_outdated
unknown
```

反代相关错误（`challenge_required`、`captcha_required`、`session_invalid`、`account_locked`、`account_banned`、`abuse_detected`、`geo_blocked`、`device_unrecognized`、`upstream_client_outdated`）只允许由 `runtime_class != api_key` 的 Adapter 上报，处理规则以 `REVERSE_PROXY_SPEC.md` 为准。

错误结构：

```txt
class
provider_code
http_status
message
retryable
should_cooldown
should_disable_account
provider_level
account_level
```

处理规则：

- `invalid_request` 不惩罚账号。
- `content_policy` 不惩罚账号。
- `auth_failed` 通常标记账号需要处理。
- `rate_limit` 通常进入短冷却。
- `provider_5xx` 同时影响 Provider 健康。

## 12. Usage 解析

Adapter 必须尽力解析：

```txt
input_tokens
output_tokens
cached_tokens
total_tokens
provider_usage_raw
```

如果 Provider 不返回 usage：

- 使用 tokenizer 估算。
- 标记 `usage_estimated = true`。
- 后续允许异步修正。

## 13. Cache Usage 解析

如果 Provider 支持 prompt cache 或 context cache，Adapter 应解析：

```txt
cache_read_tokens
cache_write_tokens
cache_hit
cache_key_hint
cache_ttl_hint
```

这些信息用于更新 CacheAffinityManager。

## 14. 健康检查

Adapter 应支持轻量健康检查。

健康检查类型：

```txt
credential_validity
model_list
minimal_completion
quota_probe
```

第一阶段建议实现：

- API Key 是否有效。
- `/models` 或轻量模型列表。
- 可选最小 completion 测试。

## 15. OAuth 刷新

支持 OAuth 的 Provider Adapter 需要提供：

```txt
CanRefresh(account) bool
Refresh(ctx, account) (*UpdatedCredential, error)
```

刷新规则：

- 刷新过程必须加锁。
- 刷新失败不能覆盖旧凭证。
- 刷新结果必须重新加密存储。
- 刷新失败应记录账号状态。

## 16. 代理支持

Provider Account 可以绑定 Proxy。

Adapter 不直接管理代理池，但必须使用平台层 HTTP client factory 创建客户端。

```txt
account.proxy_id -> platform/httpclient -> provider request
```

## 17. 重试边界

Adapter 内部只允许做协议级安全重试。

业务重试和 fallback 由 Gateway / Scheduler 控制。

Adapter 不应自行切换账号。

## 18. Metrics

Adapter 应暴露或上报：

```txt
provider_request_total
provider_request_success_total
provider_request_error_total
provider_latency_ms
provider_stream_chunks_total
provider_usage_tokens_total
provider_rate_limit_total
provider_auth_failed_total
```

## 19. 日志规范

日志必须包含：

```txt
request_id
provider
account_id
model
upstream_model
error_class
latency_ms
```

日志不得包含：

- API Key 原文。
- OAuth token。
- Cookie。
- 用户完整 prompt，除非管理员显式开启调试且做脱敏。

## 20. 新增 Provider 流程

1. 增加 Adapter 实现。
2. 注册到 Adapter Registry。
3. 如果是 OpenAI-compatible / Anthropic-compatible preset，先在 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` 定义 provider_key、route alias、auth mode、默认 base URL、模型目录和 account type allowlist。
4. 增加默认 ProviderCapabilities。
5. 增加错误分类器。
6. 增加 usage parser。
7. 增加模型映射默认值。
8. 增加 account validation。
9. 增加 mock 测试。
10. 增加 stream 测试。
11. 增加文档和示例配置。

## 21. MVP Adapter 要求

第一版 OpenAI-compatible Adapter 必须支持：

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses` 的 Canonical AI Request 转换输入
- `/v1/messages` 的 Canonical AI Request 转换输入
- stream
- API Key 认证
- 基础错误分类
- usage 解析
- account health check
- request timeout
- proxy 可选

暂缓：

- OAuth。
- prompt cache 深度解析。
- batch。
- realtime。
- fine-tuning。

## 22. 端点转换要求

Provider Adapter 只负责 Canonical AI Request 与上游 Provider 端点之间的转换。

客户端端点之间的互转规则以 `AI_ENDPOINT_COMPATIBILITY.md` 为准。

MVP 必须至少支持：

```txt
OpenAI Chat Completions -> Canonical AI Request -> OpenAI-compatible upstream
OpenAI Responses -> Canonical AI Request -> OpenAI-compatible upstream
Anthropic Messages -> Canonical AI Request -> OpenAI-compatible upstream
OpenAI-compatible upstream response -> Chat Completions response
OpenAI-compatible upstream response -> Responses response
OpenAI-compatible upstream response -> Anthropic Messages response
```

Advanced endpoint adapter coverage now also includes OpenAI-compatible passthrough families:

```txt
Embeddings -> OpenAI-compatible /embeddings
Images generations -> OpenAI-compatible /images/generations
Images edits -> OpenAI-compatible /images/edits
Images variations -> OpenAI-compatible /images/variations
Moderations -> OpenAI-compatible /moderations
Audio transcriptions -> OpenAI-compatible /audio/transcriptions
Rerank -> rerank-compatible /rerank
Audio speech -> OpenAI-compatible /audio/speech
```

Audio transcription dispatch must send multipart form-data with `file`, mapped upstream `model`, and supported optional OpenAI-compatible fields. Adapter logs and errors must not include audio bytes, prompts, API keys, cookies, or provider credentials.

Audio speech dispatch must send JSON with mapped upstream `model`, `input`, `voice`, optional `response_format`, `speed`, `instructions`, `user`, and safe passthrough extension fields. Adapter responses are binary; logs and errors must not include speech input text, audio bytes, API keys, cookies, or provider credentials.

端点转换测试必须覆盖：

- 非流式文本。
- SSE 流式事件。
- system / instructions 互转。
- tools / tool_choice 互转。
- JSON mode / structured output 互转。
- usage 互转。
- provider error 到源端点错误格式的渲染。
- 无法无损转换时的 compatibility warning 或拒绝。

## 23. 反代运行时引用

`reverse-proxy-*` 类 Adapter 必须遵守 `REVERSE_PROXY_SPEC.md`：

- “反代 / 2api”的权威定义见 `2API_REVERSE_PROXY_DEFINITION.md`：Adapter 必须按本地 `sub2api` / `CLIProxyAPI` / `chatgpt2api` 风格构造目标官方客户端形态的上游请求，例如 ChatGPT Web、Codex CLI、Claude Code CLI、Gemini CLI 或 Antigravity Desktop / IDE，而不是把下游请求简单透传到兼容 API，也不是启动或接入本地客户端进程。
- `reverse-proxy-chatgpt-web` 文本请求必须构造 ChatGPT Web Conversation / official-client 形态，POST 到 ChatGPT Web origin 下的 `/backend-api/conversation`，并通过 Reverse Proxy Runtime 注入选中账号 OAuth/session token 身份；不得退化为 OpenAI-compatible `/chat/completions`，也不得接受 `runtime_class = api_key` 作为 2api 身份。
- `reverse-proxy-codex-cli` 文本请求必须构造 Codex Responses / official-client 形态，POST 到配置的 Codex base URL 下的 `/responses`，并通过 Reverse Proxy Runtime 注入选中账号 OAuth/session/CLI token 身份；不得退化为 OpenAI-compatible `/chat/completions`，也不得接受 `runtime_class = api_key` 作为 2api 身份。
- `reverse-proxy-codex-cli` realtime 请求必须通过 `PrepareRealtime` 构造 Codex Responses WebSocket session：从 Codex base URL 派生 `ws/wss` `/responses`，设置 Codex official-client headers，生成带 `type: response.create` 和 mapped upstream model 的首帧，并继续拒绝 `runtime_class = api_key`。
- OpenAI-compatible Realtime 请求必须通过 `PrepareRealtime` 构造上游 Realtime WebSocket session：从 OpenAI-compatible base URL 派生 `ws/wss` `/realtime?model=<mapped_upstream_model>`，只转发显式允许的 Realtime handshake headers（当前为 `OpenAI-Safety-Identifier`），并通过 Reverse Proxy Runtime 使用选中账号的 OAuth/session/client-token credential 注入上游身份。SRapi 2api Realtime 路径继续拒绝 `runtime_class = api_key`，API-key Realtime 属于后续官方 API-key Adapter 能力。
- 上游请求必须通过 Reverse Proxy Runtime 发起，不得使用裸 `net/http` 默认客户端。
- TLS / HTTP/2 / Header / cookie / User-Agent / 出口 IP 必须由 Egress Profile 决定，不得在 Adapter 内硬编码。
- 不得向上游泄漏 SRapi 内部标识（`X-Request-ID`、`X-Forwarded-*`、`Via`、`X-SRapi-*` 等）。
- 必须处理 `challenge_required`、`session_invalid`、`account_locked`、`account_banned`、`abuse_detected`、`geo_blocked`、`device_unrecognized`、`upstream_client_outdated` 等反代特有错误，并按规范触发账号状态变更。
- 必须每账号独立 cookie jar、HTTP client、proxy、UA，不得跨账号共享。
- SSE / WSS 必须字节级透传，禁止 SRapi 二次合并或重压缩。WP-390 起，provider-native realtime adapter 必须使用 Reverse Proxy Runtime 的 `WebSocketRuntime.RelayWebSocket` primitive，而不是在 Adapter 内直接拨号。
- OAuth refresh / Device Code 流程必须通过反代运行时的统一接口完成，禁止在 Adapter 内单独实现凭证轮换。
