# SRapi AI 端点兼容与转换规范

## 1. 目标

SRapi 不只是 OpenAI-compatible 代理，而是多 AI 协议端点兼容与相互转换层。

核心目标：

```txt
任意主流客户端端点 -> SRapi Canonical AI IR -> 任意可用上游 Provider 端点 -> 原客户端协议响应
```

Gateway 核心能力（已实现）：

- 同时暴露主流客户端端点。
- 将不同端点请求转换为统一内部表示。
- Scheduler 基于统一内部表示做 Provider-neutral 调度。
- Provider Adapter 可选择任意兼容上游端点发起调用。
- 返回结果必须转换回客户端原始端点风格。
- 无法无损转换的字段必须显式标记或拒绝，而不是静默丢失。

## 2. 支持端点族

### 2.1 OpenAI-compatible

已暴露的核心端点：

```txt
GET  /v1/models
GET  /v1/usage
POST /v1/chat/completions
POST /v1/responses
POST /v1/responses/compact
GET  /v1/responses/{response_id}/input_items
GET  /v1/responses/ws        (WebSocket upgrade)
```

已暴露的扩展端点：

```txt
POST /v1/embeddings
POST /v1/images/generations
POST /v1/images/edits
POST /v1/images/variations
POST /v1/videos
GET  /v1/videos/{video_id}
GET  /v1/videos/{video_id}/content
POST /v1/audio/transcriptions
POST /v1/audio/speech
POST /v1/moderations
POST /v1/rerank
GET  /v1/realtime            (WebSocket upgrade)
```

Roadmap（尚未实现）：

```txt
POST /v1/batches
```

`/v1/videos` 是 OpenAI-style video generation Gateway route。Gateway 最小解析
JSON body，调度前要求 `videos.v1` capability，Provider Adapter 当前调用
xAI/OpenAI-compatible `/videos/generations` 上游。成功 create 会把返回的 video ID
按 API Key 绑定到选中的 Provider Account；`GET /v1/videos/{video_id}` 和
`GET /v1/videos/{video_id}/content` 必须先找到该绑定，再 hard sticky 调度回同一账号。
缺少或过期绑定返回 `video_binding_not_found`，避免不同上游账号之间读取视频状态或内容。

### 2.2 Anthropic-compatible

已暴露的端点：

```txt
POST /v1/messages
POST /v1/messages/count_tokens
GET  /v1/models
```

统一命名为 `anthropic-compatible`，不使用 `claude-compatible`。

`POST /v1/messages/count_tokens` 接受 Anthropic Messages-style count body，包括 `model`、`messages`、`system`、`tools`、`tool_choice`、`thinking` 和兼容扩展字段。Gateway normalization 只用于 policy / entitlement / Scheduler / evidence，不在 Gateway service 构造 Provider-local DTO；Scheduler 要求 `anthropic_count_tokens.v1` 与 `token_counting.v1`，避免 Gemini countTokens 账号被误选。Provider Adapter 保留 Anthropic count_tokens body shape，只把 `model` 替换成调度后 mapping 的 upstream model，再调用选中上游 `/messages/count_tokens`。API-key Anthropic 账号按 Anthropic auth mode 注入凭证；`runtime_class != api_key` 的 Claude Code / Anthropic 反代账号通过 Reverse Proxy Runtime 使用选中账号 OAuth/session/CLI credential，并构造 Claude Code count_tokens official-client path/header/body。显式声明 `anthropic_count_tokens` + `token_counting` 的 OpenAI-compatible 账号可使用本地估算兜底服务 Claude Code compaction。成功 count_tokens 请求记录 Scheduler decision/feedback 和 request evidence，但不进入生成用量，usage tokens 与 cost 记 0。

`adapter_type=bedrock` 复用 Anthropic Messages 入口和 Canonical AI Request，但上游不是 Anthropic `/messages`。Provider Adapter 会把请求转换为 Amazon Bedrock Runtime InvokeModel / InvokeModelWithResponseStream：

- `provider.protocol` 仍可保持 `anthropic-compatible`，下游客户端继续走 `/v1/messages`。
- 账号凭证使用 `aws_access_key_id` / `aws_secret_access_key` / 可选 `aws_session_token` 与 `aws_region`，由 SRapi 凭证加密边界 materialize 后在 Adapter 内签名。
- 上游 URL 形态为 `/model/{modelId}/invoke` 或 `/model/{modelId}/invoke-with-response-stream`，请求由 AWS SigV4 `bedrock` service 签名。
- Bedrock 请求体强制使用 `anthropic_version: bedrock-2023-05-31`，并移除 Bedrock 不接收的 `model`、`stream` 等字段。
- 流式 Bedrock event stream 会在 Adapter 内解码为 Anthropic-compatible stream events，再进入统一响应和 usage 解析。

### 2.3 Gemini-compatible

Provider Adapter 层支持 Gemini 原生请求模型：

```txt
models/{model}:generateContent
models/{model}:streamGenerateContent
models/{model}:countTokens
models/{model}:embedContent
```

下游已公开 Gemini-native 文本生成路由：

```txt
GET  /v1beta/models
POST /v1beta/models/{model}:generateContent
POST /v1beta/models/{model}:streamGenerateContent
POST /v1beta/models/{model}:countTokens
```

这些路由完成客户端侧 Gemini GenerateContent 与 Canonical AI Request / Response 的转换，并复用 Gateway API Key、模型策略、Scheduler、Provider Adapter、usage 和 decision 记录。Gemini-shaped 下游入口接受 Google SDK / REST 常用的 `x-goog-api-key`、`Authorization: Bearer`、`x-api-key` 和 `key` query 参数；其他查询参数不参与 Gateway 鉴权。该兼容只作用于 `/v1beta/models*` 及其 Gemini alias 路由，不改变 OpenAI / Anthropic 入口只用 Bearer API key 的语义。WP-240 起，`generateContent` / `streamGenerateContent` 请求要求 `gemini_generate_content.v1`，目标 Provider 为 `gemini-compatible` / `native-gemini` / `reverse-proxy-gemini-cli` 或 Antigravity Gemini alias 时，Provider Adapter 会调用 Gemini `generateContent` / `streamGenerateContent` 或 Antigravity `v1internal` 上游；OpenAI-compatible Chat Completions 能力不能替代 Gemini 原生入口能力。

WP-540 起，`GET /v1beta/models` 返回 Gemini `models.list` 兼容的 `{models,nextPageToken}`，基于 SRapi model registry、Gateway API Key 可见性、active Provider mapping，以及 active account metadata 中的 `excluded_models` / `supported_models` 可路由性渲染 active models；`GET /v1beta/models/{model}` 使用同一可见性边界。响应 model name 使用 `models/{canonical_name}`，并包含 `baseModelId`、`version`、`displayName`、`inputTokenLimit`、`outputTokenLimit` 和 `supportedGenerationMethods`；`supportedGenerationMethods` 至少包含 `generateContent`，并按模型能力追加 `streamGenerateContent` 与 `countTokens`。`pageSize` / `pageToken` 非法时返回 Google-style `INVALID_ARGUMENT`。该目录路由不进入 Scheduler、不读取 Provider Account 凭证、也不做上游模型发现。

WP-550 起，`POST /v1beta/models/{model}:countTokens` 接受 Gemini `countTokens` body：可直接提供 `contents`、`systemInstruction`、`generationConfig`、`tools` 等字段，也可提供 `generateContentRequest`。该路由只把请求归一化用于 Gateway policy / entitlement / Scheduler，不构造本地 Provider DTO；Scheduler 要求 `gemini_count_tokens.v1` 与 `token_counting.v1`，避免 Anthropic count_tokens 账号被误选。Provider Adapter 将原始 Gemini countTokens body 发送到选中上游 `models/{mapped_model}:countTokens`。API-key Gemini 账号按 Gemini auth mode 注入凭证，`runtime_class != api_key` 的 Gemini 反代账号仍通过 Reverse Proxy Runtime 使用选中账号 OAuth/session/client-token 凭证。成功 countTokens 请求记录 Scheduler decision/feedback 和 request evidence，但不进入生成用量，usage tokens 与 cost 记 0。

### 2.4 OpenRouter 与其他聚合协议

OpenRouter、xAI/Grok、Cloudflare Workers AI、LiteLLM 风格上游通常提供 OpenAI-compatible 或近似协议。

SRapi 必须允许通过 `provider.adapter_type` 和 `provider.protocol` 区分：

```txt
provider.name         = openrouter / grok / custom-upstream
provider.adapter_type = openrouter / openai-compatible / native-grok / reverse-proxy-antigravity
provider.protocol     = openai-compatible / anthropic-compatible / gemini-compatible
```

Antigravity 反代账号使用 `reverse-proxy-antigravity` 表示客户端身份。WP-450 起，
它的上游 text dispatch 使用 Antigravity / Google Cloud Code `v1internal`
official-client shape；`provider.protocol` 仍决定下游请求如何进入 Canonical AI Request
以及如何渲染回客户端协议。它不得绕过 Canonical AI Request、Scheduler、Provider Adapter
或 Reverse Proxy Runtime，也不得新增 Gateway-local DTO。
WP-500 起，Antigravity model discovery 也使用同一 2api 边界：管理员触发
`discover-models` 时，SRapi 通过 Reverse Proxy Runtime 用选中账号的 OAuth/desktop/IDE
credential POST 到 `{base_url}/v1internal:fetchAvailableModels`，解析上游 `models`，并可把
结果持久化为账号 `supported_models` 供 Provider-neutral Scheduler 过滤。
WP-530 起，Antigravity discovery 在账号缺少 `project_id`、`antigravity_project_id` 或
`cloudaicompanion_project` 时，会先通过同一 Reverse Proxy Runtime / selected account
credential 请求 `{base_url}/v1internal:loadCodeAssist`，必要时请求
`{base_url}/v1internal:onboardUser`，取得 project 后再请求
`{base_url}/v1internal:fetchAvailableModels`。预览 discovery 不写回账号 metadata；
`persist=true` 才持久化解析到的 project 与 `supported_models`。
WP-360 起，Antigravity 文本 provider alias（例如
`/api/provider/antigravity/v1/chat/completions` 和
`/api/provider/antigravity/v1/messages`）只强制 `provider_key=antigravity`；
OpenAI/Anthropic/Gemini 的下游协议仍由 `provider.protocol` 决定，Antigravity 上游
统一由 adapter 转为 `v1internal` envelope。
WP-370 起，Gemini-native Antigravity provider alias（例如
`/api/provider/antigravity/v1beta/models`、
`/api/provider/antigravity/v1beta/models/{model}`、
`/api/provider/antigravity/v1beta/models/{model}:generateContent`、
`/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent` 和
`/api/provider/antigravity/v1beta/models/{model}:countTokens`）复用同一个
Gemini Gateway handler，只改写 provider context 和保留原始 alias source endpoint。
list/get 只返回可见且映射到 `antigravity` provider 的 registry-backed model metadata；
`countTokens` 走 Antigravity official-client `v1internal:countTokens`，成功请求 usage
tokens 和 cost 记 0。

## 3. 端点转换架构

### 3.1 总流程

```txt
Client Endpoint Request
  ↓
Client Endpoint Adapter
  ↓
Canonical AI Request
  ↓
Gateway Policy + API Key + Entitlement
  ↓
Scheduler v1
  ↓
Provider Endpoint Adapter
  ↓
Upstream Provider Endpoint
  ↓
Canonical AI Response / Stream Event
  ↓
Client Response Renderer
```

### 3.2 职责边界

Client Endpoint Adapter 负责：

- 解析客户端端点格式。
- 校验端点级请求结构。
- 转换为 Canonical AI Request。
- 记录源协议和源端点。
- 将 Canonical AI Response 转回客户端协议格式。

Provider Endpoint Adapter 负责：

- 将 Canonical AI Request 转换为上游 Provider 端点格式。
- 注入 Provider 凭证和协议头。
- 解析上游响应、流式事件、usage 和错误。
- 转换为 Canonical AI Response 或 Canonical Stream Event。

Scheduler 只允许依赖 Canonical AI Request、模型能力、用户权益、账号状态和价格信息，不得依赖客户端端点格式。

## 4. Canonical AI Request

所有客户端端点进入 Gateway 后必须转换为统一内部请求。

字段：

```txt
request_id
source_protocol
source_endpoint
response_protocol
user_id
api_key_id
model
canonical_model
input_items
messages
instructions
modalities
stream
tools
tool_choice
response_format
json_schema
reasoning
temperature
top_p
max_output_tokens
stop
metadata
provider_options
cache_control
conversation_hash
session_hash
compatibility_warnings
```

### 4.1 Content Block

`input_items` 和 `messages` 中的内容必须统一为 Content Block。

类型：

```txt
text
image
audio
file
tool_call
tool_result
reasoning
refusal
metadata
```

字段：

```txt
type
role
text
media_url
media_base64
mime_type
tool_call_id
tool_name
tool_arguments_json
provider_metadata_json
```

## 5. Canonical AI Response

字段：

```txt
id
request_id
model
canonical_model
provider_id
account_id
output_items
message
choices
finish_reason
usage
cache_usage
provider_error
raw_provider_metadata
compatibility_warnings
```

流式事件字段：

```txt
request_id
event_index
event_type
delta
content_block_delta
tool_call_delta
usage_delta
finish_reason
raw_event_type
provider_metadata_json
```

## 6. 转换规则

### 6.1 Chat Completions -> Canonical

```txt
messages              -> messages / input_items
temperature           -> temperature
top_p                 -> top_p
max_tokens            -> max_output_tokens
tools                 -> tools
tool_choice           -> tool_choice
response_format       -> response_format / json_schema
stream                -> stream
```

### 6.2 Responses -> Canonical

```txt
input                 -> input_items
instructions          -> instructions
tools                 -> tools
reasoning             -> reasoning
text.format           -> response_format / json_schema
previous_response_id  -> conversation reference or compatibility warning
store                 -> provider option or SRapi state policy
stream                -> stream
```

`previous_response_id` 必须引用 Responses response id。Gateway 为兼容第三方
Responses 实现保留 opaque custom id，但会拒绝明显属于 message、item、
tool call、reasoning、compaction 或 Chat Completions 的对象 id，避免把
`msg_*` / `chatcmpl-*` / `call_*` 等误用继续传入上游。

### 6.3 Anthropic Messages -> Canonical

```txt
system                -> instructions
messages              -> messages / content blocks
max_tokens            -> max_output_tokens
tools                 -> tools
tool_choice           -> tool_choice
thinking              -> reasoning
stream                -> stream
```

### 6.4 Gemini GenerateContent -> Canonical

```txt
contents              -> input_items / messages
systemInstruction     -> instructions
generationConfig      -> temperature / top_p / max_output_tokens / stop
safetySettings        -> compatibility_warnings until provider_options is persisted
tools                 -> tools
streamGenerateContent -> stream
```

## 7. 反向渲染规则

默认情况下，SRapi 必须把响应渲染回客户端调用的源端点格式。

示例：

```txt
客户端调用 /v1/messages
上游实际调用 /v1/chat/completions
SRapi 返回 Anthropic Messages 响应格式
```

```txt
客户端调用 /v1/responses
上游实际调用 Anthropic /v1/messages
SRapi 返回 OpenAI Responses 响应格式
```

```txt
客户端调用 /v1/chat/completions
上游实际调用 Gemini generateContent
SRapi 返回 OpenAI Chat Completions 响应格式
```

```txt
客户端调用 /v1beta/models/{model}:generateContent
上游实际调用 /v1/chat/completions、Anthropic /v1/messages、Gemini generateContent 或其他可调度 Provider Adapter
SRapi 返回 Gemini GenerateContent 响应格式
```

Responses 渲染必须保留 Canonical 终态语义：`stopReason=max_tokens` 输出
`status=incomplete` 和 `incomplete_details.reason=max_output_tokens`；流式 Responses
终态输出 `response.incomplete`，而不是伪装成 `response.completed`。

## 8. 无损与有损转换策略

SRapi 必须优先做无损转换。

无法无损转换时必须遵守：

- 不得静默丢弃关键语义。
- 如果可安全降级，必须写入 `compatibility_warnings`。
- 如果降级会导致行为错误，必须返回 `422 VALIDATION_FAILED` 或 OpenAI/Anthropic-compatible 等价错误。
- 原始 Provider metadata 可以进入 `raw_provider_metadata`，但不得泄漏 secret。

常见有损场景：

```txt
Responses stateful previous_response_id -> stateless Chat Completions
OpenAI built-in tools -> Anthropic/Gemini 无等价工具
Provider-specific reasoning blocks -> 目标协议无 reasoning 字段
并行 tool calls -> 目标协议只支持串行 tool call
Provider 原生 safety settings -> OpenAI-compatible 无直接字段
```

Provider-hosted web search 是 built-in tool 的特例：Responses `web_search` / legacy `web_search_preview` 与 Anthropic web search server tool 在 Canonical AI Request 中保留原始 tool `type`，并要求 `web_search.v1`。Gateway 不把这类 hosted tool 重写成普通 function；当 Provider 返回 Responses-style `web_search_call` 时，Responses renderer 保留 `web_search_call.action`，而不是输出 `function_call.arguments`。SRapi 不用 Tavily/Brave/Copilot search 为无 hosted-search 能力的上游自履约 gateway web search；该产品边界见 `CAPABILITY_BOUNDARIES.md`。

## 9. Endpoint Compatibility Matrix

| Source endpoint | Target OpenAI Chat | Target OpenAI Responses | Target Anthropic Messages | Target Gemini GenerateContent |
| --- | --- | --- | --- | --- |
| `/v1/chat/completions` | native | convert | convert | convert |
| `/v1/responses` | convert with warnings by default; native `/responses` passthrough for `native-openai`, provider.name=openai, or explicit native Responses opt-in | native | convert | convert |
| `/v1/responses/compact` | unsupported | native compact only | unsupported | unsupported |
| `/v1/messages` | convert | convert | native | convert |
| Gemini `generateContent` | convert | convert | convert | native |

转换测试覆盖文本、流式、基础 tool calls、JSON mode / structured output。

Vision、audio、file、built-in tools（含 web search）、reasoning、realtime 均已实现，并复用 Canonical AI Request 预留的统一字段。Batch 仍在 Roadmap（见第 13 节）。

## 10. 能力协商

ProviderCapabilities 必须声明端点能力：

```txt
supported_client_protocols
supported_upstream_protocols
supports_chat_completions
supports_responses
supports_responses_compact
supports_messages
supports_generate_content
supports_embeddings
supports_image_generations
supports_image_edits
supports_image_variations
supports_audio
supports_stream
supports_tools
supports_parallel_tool_calls
supports_structured_output
supports_reasoning
supports_stateful_responses
supports_prompt_cache
supports_usage_in_stream
```

Scheduler 候选构建必须基于 Canonical AI Request 的能力需求，而不是源端点名称。

## 11. 测试要求

必须建立端点转换黄金测试集：

```txt
chat_completions_to_responses
chat_completions_to_messages
responses_to_chat_completions
responses_to_messages
messages_to_chat_completions
messages_to_responses
messages_to_gemini_generate_content
gemini_generate_content_to_chat_completions
stream_event_translation
tool_call_translation
usage_translation
error_translation
lossy_conversion_warning
unsupported_conversion_rejection
```

每个测试必须断言：

- 请求语义保持一致。
- 角色、系统指令、工具调用、JSON 输出约束不丢失。
- usage 和 cache usage 可归一化。
- 错误格式按源端点风格返回。
- 有损转换必须出现 warning 或被拒绝。

## 12. 安全要求

端点转换层必须遵守：

- 不记录完整 prompt、messages、input_items、tool arguments。
- 不把用户 prompt 当作系统指令执行。
- Tool call 参数必须按目标工具 schema 校验。
- Provider 原始错误必须脱敏后再渲染到客户端协议。
- `raw_provider_metadata` 不得包含 Authorization、Cookie、API Key、OAuth token 或 Provider credential。
- Stateful Responses 或 conversation 存储如果启用，必须有保留时间、用户隔离和删除策略。

## 13. 当前能力与 Roadmap

> 下文 `WP-xxx` 标签为历史实现记录，仅用于追溯；以 `internal/httpserver/server.go` 的路由注册为权威清单。

核心文本端点（已实现）：

```txt
/v1/models
/v1/chat/completions
/v1/responses
/v1/responses/{response_id}/input_items
/v1/responses/compact
/v1/messages
```

核心协议转换（已实现）：

```txt
OpenAI Chat Completions <-> Canonical AI Request
OpenAI Responses <-> Canonical AI Request
Anthropic Messages <-> Canonical AI Request
Gemini GenerateContent <-> Canonical AI Request
Canonical AI Request -> OpenAI-compatible upstream
Canonical AI Response -> OpenAI Chat / Responses / Anthropic Messages / Gemini response
```

`GET /v1/responses/{response_id}/input_items` is an OpenAI Responses
subresource rather than a new conversation runtime. SRapi requires query
`model` so API key model visibility, entitlement, Scheduler, Provider Adapter,
usage evidence, and scheduler decision records still apply. The request requires
`responses_input_items.v1`; ordinary `responses.v1` generation support is not
enough because cross-protocol generation adapters cannot read a native OpenAI
Responses stateful input item list. The selected adapter calls upstream
`/responses/{response_id}/input_items` with supported pagination query
parameters (`after`, repeated `include`, `limit`, `order`) and replays the raw
JSON list. The SRapi-only `model` query parameter is not forwarded upstream, and
successful requests record zero generation tokens/cost.

`GET /v1/usage` is a client integration endpoint, not an admin usage export.
It authenticates the Gateway Bearer key and returns only that key's usage
window, today summary, model distribution, recent request evidence, key limits,
allowed models, expiry metadata, and owner wallet balance. It intentionally does
not reveal other API keys for the same user.

WP-270 已实现：

```txt
Embeddings endpoint
```

边界：

- `POST /v1/embeddings` 接受 OpenAI-compatible 的 `model` 和 string / string-array `input`。
- token-array input 暂不支持，返回 OpenAI-compatible Gateway error。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/embeddings`）强制 provider context。
- OpenAI-compatible API-key 和 reverse-proxy accounts 上游调用 `/embeddings` 并解析 usage。

WP-290 已实现：

```txt
Images generations endpoint
```

边界：

- `POST /v1/images/generations` 接受 OpenAI-compatible 的 `model`、`prompt`、`n`、`size`、`quality`、`style`、`response_format` 和 `user`。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `image_generations` endpoint capability；Provider 或 account/mapping 必须显式声明 image generation 能力，text-only provider 不会被误选。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/images/generations`）强制 provider context。
- OpenAI-compatible API-key 和 reverse-proxy accounts 上游调用 `/images/generations` 并解析 `url`、`b64_json` 和 `revised_prompt`。
- Image edits 和 variations 不在 WP-290 范围内。

WP-480 已实现：

```txt
Images edits endpoint
```

边界：

- `POST /v1/images/edits` 接受 OpenAI-compatible multipart form-data：`model`、`prompt`、一个或多个 `image` / `image[]`、可选 `mask`、`n`、`size`、`quality`、`response_format`、`output_format`、`output_compression`、`background`、`moderation`、`input_fidelity` 和 `user`。
- WP-510 起，同一路由也接受 JSON image references：单个 `image`、多个 `images` 或可选 `mask` 可使用 data URL、`{"image_url":"data:..."}`、`{"image_url":{"url":"data:..."}}` 或 `{"b64_json":"...","mime_type":"...","filename":"..."}`。JSON references 会解码进同一个 canonical image edit request；OpenAI-compatible API-key / 普通 reverse-proxy accounts 以上游 multipart `/images/edits` 发出，`reverse-proxy-codex-cli` accounts 转换为 Codex `/responses` 的 `image_generation` tool edit action，源图像使用 `input_image` data URL，mask 使用 `input_image_mask.image_url`。
- WP-520 起，`stream=true` 的 image edit 请求会在同一 Gateway auth / Scheduler / Provider Adapter / usage path 上返回 `text/event-stream`；当前 v1 只渲染最终 `image.generation.result` chunk 和 `[DONE]`，不会伪造 upstream 增量。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `image_edits` endpoint capability；Provider 或 account/mapping 必须显式声明 image edit 能力，generation-only provider 不会被误选。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/images/edits`）强制 provider context。
- OpenAI-compatible API-key 和普通 reverse-proxy accounts 上游调用 multipart `/images/edits`；`reverse-proxy-codex-cli` accounts 上游调用 `/responses`，使用 `image_generation` tool 的 `action=edit`。两条路径都解析 `url`、`b64_json` 和 `revised_prompt`。
- Remote `image_url` 和 `file_id` references 仍明确拒绝，直到后续 Files API / remote-fetch 安全边界实现；streaming image edit 的 upstream progressive relay 仍留给后续兼容包。

WP-490 已实现：

```txt
Images variations endpoint
```

边界：

- `POST /v1/images/variations` 接受 OpenAI-compatible multipart form-data 和 JSON 本地图像引用：单个 `image`、`model`、`n`、`size`、`response_format` 和 `user`。JSON `image` 或单元素 `images` 可使用 data URL、`{"image_url":"data:..."}`、`{"image_url":{"url":"data:..."}}` 或 `{"b64_json":"...","mime_type":"...","filename":"..."}`，并会解码进同一个 canonical image variation request。
- OpenAI 官方 upstream 当前说明该 endpoint 仅支持 `dall-e-2`；SRapi 不在 Gateway 层硬编码模型名，而是通过模型映射把本地 canonical model 映射到上游模型。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `image_variations` endpoint capability；Provider 或 account/mapping 必须显式声明 image variation 能力，generation-only provider 不会被误选。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/images/variations`）强制 provider context。
- OpenAI-compatible API-key 和 reverse-proxy accounts 上游调用 multipart `/images/variations`，并解析 `url`、`b64_json` 和 `revised_prompt`。
- Remote `image_url`、`file_id` references、多图 variation、streaming image variation events 和 frontend visuals 留给后续 Files API / media compatibility 包。

WP-310 已实现：

```txt
Moderations endpoint
```

边界：

- `POST /v1/moderations` 接受 OpenAI-compatible 的 `model` 和 string / string-array `input`。
- image/multimodal moderation input 暂不支持，留给后续兼容包。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `moderations` endpoint capability；Provider 或 account/mapping 必须显式声明 moderation 能力，generation-only provider 不会被误选。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/moderations`）强制 provider context。
- OpenAI-compatible API-key 和 reverse-proxy accounts 上游调用 `/moderations` 并解析 `flagged`、`categories`、`category_scores` 和 `category_applied_input_types`。

WP-320 已实现：

```txt
Rerank endpoint
```

边界：

- `POST /v1/rerank` 接受 `model`、非空 `query`、string/object `documents`、可选 `top_n`、可选 `return_documents` 和可选 `user`。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `rerank` endpoint capability；Provider 或 account/mapping 必须显式声明 rerank 能力，generation-only provider 不会被误选。
- Rerank-compatible provider alias（例如 `/api/provider/rerank-compatible/v1/rerank`）强制 provider context。
- Rerank-compatible API-key 和 reverse-proxy accounts 上游调用 `/rerank` 并解析 `index`、`relevance_score`、可选 `document` 和 usage。

WP-330 已实现：

```txt
Audio transcriptions endpoint
```

边界：

- `POST /v1/audio/transcriptions` 接受 OpenAI-compatible multipart `file`、`model`、可选 `language`、`prompt`、`response_format`、`temperature` 和 `user`。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `audio_transcriptions` endpoint capability；Provider 或 account/mapping 必须显式声明 audio transcription 能力，text-only provider 不会被误选。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/audio/transcriptions`）强制 provider context。
- OpenAI-compatible API-key 和 reverse-proxy accounts 上游调用 `/audio/transcriptions` 并解析 JSON/verbose JSON transcription response；plain text 上游响应会被包装为稳定 transcription response。
- Streaming transcription、speaker diarization 深度语义、audio moderation 和 realtime 不在 WP-330 范围内。

WP-340 已实现：

```txt
Audio speech endpoint
```

边界：

- `POST /v1/audio/speech` 接受 OpenAI-compatible JSON `model`、`input`、`voice`、可选 `response_format`、`speed`、`instructions` 和 `user`。
- 请求仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- Scheduler 使用 `audio_speech` endpoint capability；Provider 或 account/mapping 必须显式声明 speech synthesis 能力，transcription-only 或 text-only provider 不会被误选。
- OpenAI-compatible provider alias（例如 `/api/provider/openai-compatible/v1/audio/speech`）强制 provider context。
- OpenAI-compatible API-key 和 reverse-proxy accounts 上游调用 `/audio/speech`，保留 voice/format/speed/instructions/user 与 passthrough extension fields，并把上游 binary audio bytes 与 content type 直接返回给客户端。
- Speech response usage 目前按输入文本和音频字节长度估算；如上游后续暴露稳定 token/audio usage header，应在 adapter 层补解析。

WP-380 已实现：

```txt
Responses WebSocket transport
```

边界：

- `GET /v1/responses/ws` 建立 WebSocket 连接，客户端发送 JSON text frame。
- 支持 raw `ResponsesRequest`，也支持 `{"type":"response.create","response":{...}}` / `{"type":"response.create","request":{...}}` event envelope。为兼容 Codex / OpenAI Responses WS Mode 客户端，也接受 flat `{"type":"response.create","model":...,"input":...}` frame；Gateway 会剥离 `type` / `event_id` 后把剩余字段作为 Responses request 进入同一验证链。`model` query 可作为 payload 未携带 model 时的 fallback。
- 每个 `response.create` payload 都转交给现有 `/v1/responses` Gateway runtime，因此仍进入 API Key auth、模型可见性、entitlement、Scheduler、Provider Adapter、usage、billing 和 feedback 证据链。
- `stream:true` 的 Responses SSE 事件会转成同名 JSON WebSocket frame；非流式响应返回 `response.completed` frame。
- `session_affinity_key`、`sticky_strength`、`sticky_account_id` query/header 继续作为 Scheduler sticky routing hint；Gateway 不直接选择账号。
- WP-410 进一步实现 Codex CLI 2api Responses WebSocket upstream relay：当请求显式带 `upstream_ws` 或 `codex_responses_websocket`，且调度出的 `reverse-proxy-codex-cli` 账号 metadata 启用 Codex Responses WebSocket 时，SRapi 通过 Reverse Proxy Runtime 连接 Codex `ws/wss` `/responses`，使用选中账号 OAuth/session/CLI token 凭证、Codex official-client headers，以及经过 Codex Responses normalizer 的 `response.create` 首帧。该首帧会强制 mapped upstream model、`stream=true`、默认 `store=false`，补齐 instructions，规范化 input/tools/service_tier/image_generation，并移除 `background`。
- Codex upstream relay 按 turn 处理：一个客户端 `/v1/responses/ws` 连接可以连续发送多个 `response.create` frame。每个 turn 独立进入 Gateway admission、Scheduler、Provider Adapter、Reverse Proxy Runtime 和 usage 记录；上游 Codex WebSocket relay 收到 terminal event 后释放该次上游连接，但不会关闭客户端 WebSocket。
- WP-460 起，`/v1/responses/ws` 在 WebSocket upgrade 前获得 provider-neutral realtime slot，并在连接关闭、上游 relay 完成、客户端断开或 handler error 时释放；slot 只保存 request/user/API key/source endpoint/sticky metadata 和 session affinity hash，不保存原始 affinity key。
- WP-570 起，`GET /api/v1/admin/ops/realtime/slots` 可查询 active realtime slot 摘要和聚合计数；WP-590 起 Redis 可用时该视图和 slot 限额跨 API 节点生效，本地降级模式只覆盖当前节点内存。这是运维诊断面，不是持久 upstream session pool，也不引入 provider-specific realtime DTO。
- 复杂 persistent upstream session reuse、local Codex CLI client ingress，以及 Claude Code / Antigravity provider-native realtime 协议仍是后续包。

WP-470 已实现：

```txt
OpenAI-compatible Realtime WebSocket relay
```

边界：

- `GET /v1/realtime?model=<model>` 建立 OpenAI-compatible Realtime WebSocket；不是 `POST /v1/realtime`。
- Gateway 先执行 API Key auth、模型可见性、entitlement、Scheduler、`realtime_websocket` capability 过滤和 realtime slot acquisition，再 upgrade WebSocket。
- Provider Adapter 从选中账号和模型映射构造上游 `ws/wss` `/realtime?model=<mapped_upstream_model>` session。
- `runtime_class = api_key` 的 OpenAI-compatible / native-openai 账号走官方 API-key Realtime 路径：Gateway 只用选中账号的 `api_key`/`openai_api_key` 连接上游，不把 caller `Authorization`、cookie 或 SRapi headers 透传给上游，也不进入 2api Reverse Proxy Runtime。
- `runtime_class != api_key` 的 OpenAI-compatible Realtime 仍通过 Reverse Proxy Runtime 使用选中账号 OAuth/session/client-token credential 双向 relay text/binary frames；`reverse-proxy-*` 2api Realtime 路径继续拒绝 `runtime_class = api_key`。
- 只允许 `OpenAI-Safety-Identifier` 等显式白名单握手 header 进入上游；caller `Authorization`、`Cookie`、`Sec-WebSocket-*`、`X-SRapi-*` 和 Gateway headers 不得定义上游身份。

已实现的扩展端点（边界见上文各 WP 块）：Embeddings、Images（generations / edits / variations）、Moderations、Rerank、Audio（transcriptions / speech）、Token counting（Anthropic `count_tokens` 与 Gemini `countTokens`）、Responses WebSocket transport、OpenAI-compatible Realtime WebSocket relay。

### 13.1 Roadmap（尚未实现）

```txt
Batch
Fine-tuning
Provider-native Claude Code / Antigravity realtime protocol adapters
Provider-native built-in tools
Advanced stateful responses
```
