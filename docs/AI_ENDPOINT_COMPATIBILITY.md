# SRapi AI 端点兼容与转换规范

## 1. 目标

SRapi 不只是 OpenAI-compatible 代理，而是多 AI 协议端点兼容与相互转换层。

核心目标：

```txt
任意主流客户端端点 -> SRapi Canonical AI IR -> 任意可用上游 Provider 端点 -> 原客户端协议响应
```

第一阶段必须把以下能力作为 Gateway 核心能力设计：

- 同时暴露主流客户端端点。
- 将不同端点请求转换为统一内部表示。
- Scheduler 基于统一内部表示做 Provider-neutral 调度。
- Provider Adapter 可选择任意兼容上游端点发起调用。
- 返回结果必须转换回客户端原始端点风格。
- 无法无损转换的字段必须显式标记或拒绝，而不是静默丢失。

## 2. 支持端点族

### 2.1 OpenAI-compatible

SRapi 必须优先支持：

```txt
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
```

后续扩展：

```txt
POST /v1/embeddings
POST /v1/images/generations
POST /v1/audio/transcriptions
POST /v1/audio/speech
POST /v1/batches
POST /v1/realtime
```

### 2.2 Anthropic-compatible

SRapi 必须优先支持：

```txt
POST /v1/messages
```

后续扩展：

```txt
POST /v1/messages/count_tokens
GET  /v1/models
```

统一命名为 `anthropic-compatible`，不使用 `claude-compatible`。

### 2.3 Gemini-compatible

SRapi 必须在 Provider Adapter 层支持 Gemini 原生请求模型：

```txt
models/{model}:generateContent
models/{model}:streamGenerateContent
models/{model}:embedContent
```

WP-230 起公开 Gemini-native 文本生成路由：

```txt
POST /v1beta/models/{model}:generateContent
POST /v1beta/models/{model}:streamGenerateContent
```

这些路由完成客户端侧 Gemini GenerateContent 与 Canonical AI Request / Response 的转换，并复用 Gateway API Key、模型策略、Scheduler、Provider Adapter、usage 和 decision 记录。WP-240 起，目标 Provider 为 `gemini-compatible` / `native-gemini` / `reverse-proxy-gemini-cli` 时，Provider Adapter 会调用 Gemini `generateContent` 或 `streamGenerateContent` 上游。

### 2.4 OpenRouter 与其他聚合协议

OpenRouter、xAI/Grok、Cloudflare Workers AI、LiteLLM 风格上游通常提供 OpenAI-compatible 或近似协议。

SRapi 必须允许通过 `provider.adapter_type` 和 `provider.protocol` 区分：

```txt
provider.name         = openrouter / grok / custom-upstream
provider.adapter_type = openrouter / openai-compatible / native-grok
provider.protocol     = openai-compatible / anthropic-compatible / gemini-compatible
```

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

## 9. Endpoint Compatibility Matrix

| Source endpoint | Target OpenAI Chat | Target OpenAI Responses | Target Anthropic Messages | Target Gemini GenerateContent |
| --- | --- | --- | --- | --- |
| `/v1/chat/completions` | native | convert | convert | convert |
| `/v1/responses` | convert with warnings | native | convert | convert |
| `/v1/messages` | convert | convert | native | convert |
| Gemini `generateContent` | convert | convert | convert | native |

MVP 必须覆盖文本、流式、基础 tool calls、JSON mode / structured output 的转换测试。

Vision、audio、file、built-in tools、reasoning、batch、realtime 可以分阶段实现，但 Canonical AI Request 必须从第一阶段预留字段。

## 10. 能力协商

ProviderCapabilities 必须声明端点能力：

```txt
supported_client_protocols
supported_upstream_protocols
supports_chat_completions
supports_responses
supports_messages
supports_generate_content
supports_embeddings
supports_images
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

## 13. 阶段要求

MVP 必须实现：

```txt
/v1/models
/v1/chat/completions
/v1/responses
/v1/messages
```

MVP 必须完成：

```txt
OpenAI Chat Completions <-> Canonical AI Request
OpenAI Responses <-> Canonical AI Request
Anthropic Messages <-> Canonical AI Request
Canonical AI Request -> OpenAI-compatible upstream
Canonical AI Response -> OpenAI Chat / Responses / Anthropic Messages response
```

Phase 2 继续实现：

```txt
Gemini native models/list endpoint
Embeddings endpoint
Images endpoint
Token counting endpoint
```

Phase 3+ 继续实现：

```txt
Audio
Batch
Realtime
Fine-tuning
Provider-native built-in tools
Advanced stateful responses
```
