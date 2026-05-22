# SRapi Gateway 路由矩阵规范

## 1. 目标

本文档定义 SRapi 对外 Gateway API 的路由族、Provider-prefixed alias、passthrough 边界、运行时归属和阶段规划。

原则：

```txt
路由别名不是新运行时
协议解析在 Handler
账号选择在 Scheduler
Provider 转发在 Adapter / Runtime
```

## 2. 路由分层

| 层级 | 示例 | 说明 |
| --- | --- | --- |
| 标准兼容入口 | `/v1/chat/completions` | 面向 OpenAI / Anthropic / Gemini SDK 的主入口。 |
| Provider-prefixed alias | `/api/provider/openai/v1/chat/completions` | 强制绑定 provider/platform，不新增 runtime。 |
| Native provider endpoint | `/v1beta/models/*:generateContent` | 保留原生协议形状。 |
| Passthrough | `/v1/embeddings`、`/v1/images/*` | 最小改写，仍需鉴权、调度、用量。 |
| WebSocket / Realtime | `/v1/responses/ws` | 长连接代理，需特殊 slot 和粘性策略。 |

## 3. MVP 必须支持

```txt
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
POST /v1/messages
```

MVP 要求：

- API Key 鉴权。
- 模型可见性过滤。
- Chat Completions / Responses / Messages 转 Canonical AI Request。
- 非流式和 SSE 流式。
- Scheduler decision / feedback / usage log。
- OpenAI-compatible 错误渲染。
- 已实现的 Provider alias 必须复用标准 Gateway runtime，只改变 provider context，并把实际 alias path 写入 usage log 与 scheduler decision。

当前 Provider alias 由 Compatible Provider preset registry 动态注册，不为每个 Provider 复制 handler。已实现规则：

```txt
OpenAI-compatible preset:
  POST {alias}/v1/chat/completions
  POST {alias}/v1/responses
  POST {alias}/v1/messages
  POST {alias}/v1/embeddings
  POST {alias}/v1/images/generations
  POST {alias}/v1/audio/transcriptions
  POST {alias}/v1/audio/speech
  POST {alias}/v1/moderations

Anthropic-compatible preset:
  POST {alias}/v1/messages

Antigravity preset:
  POST {alias}/v1/chat/completions
  POST {alias}/v1/messages
  POST {gemini_alias}/models/{model}:generateContent
  POST {gemini_alias}/models/{model}:streamGenerateContent
```

`{alias}` 来自 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` 的 `route_aliases`；`{gemini_alias}` 来自同一 preset 的 Gemini model-action aliases。例如 `/api/provider/deepseek/v1/chat/completions` 强制 `provider_key=deepseek`，但仍复用标准 Gateway runtime、API Key policy、model visibility、Scheduler、usage 和 decision 记录。

## 4. Provider alias 规划

### 4.1 OpenAI / OpenAI-compatible

```txt
/openai/v1/*
/api/provider/openai/*
/api/provider/openai/v1/*
/api/provider/openai-compatible/*
/api/provider/openai-compatible/v1/*
/api/provider/groq/*
/api/provider/cerebras/*
/api/provider/deepseek/*
/api/provider/moonshot/*
/api/provider/kimi/*
/api/provider/openrouter/*
/api/provider/anyrouter/*
/api/provider/zhipu/*
/api/provider/zai/*
```

这些路径必须进入同一 OpenAI-compatible text / passthrough runtime，区别只在 platform context 和 provider preset。

### 4.2 Anthropic / Anthropic-compatible

```txt
/anthropic/v1/*
/api/provider/anthropic/*
/api/provider/anthropic/v1/*
/api/provider/claude-compatible/*
/api/provider/claude-compatible/v1/*
/api/provider/anthropic-compatible/*
/api/provider/anthropic-compatible/v1/*
```

Anthropic-compatible preset 的 auth、base_url、model catalog 由 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` 管理。进入 `/v1/messages` 或 Anthropic provider alias 后，Gateway 仍复用统一鉴权、模型策略、Scheduler、usage 和 decision 记录；Provider Adapter 必须把目标协议为 `anthropic-compatible` 的候选账号转为 Anthropic Messages `/messages` 上游请求，并默认使用 `x-api-key` 与 `anthropic-version` 协议头。`/api/provider/claude-compatible/*` 只是兼容路由别名，不是新的 adapter 类型。

### 4.3 Gemini / Google

```txt
/v1beta/models/*:generateContent
/v1beta/models/*:streamGenerateContent
/v1beta/models
/api/provider/google/*
/api/provider/google/v1beta/*
/api/provider/google/v1beta1/*
/api/provider/gemini/*
/api/provider/gemini/v1beta/*
/api/provider/gemini/v1beta1/*
```

Gemini 原生错误必须渲染为 Google-compatible 形状；通过 `/v1/messages` 或 `/v1/chat/completions` 进入的请求则按客户端源协议渲染。

已实现的 Gemini-native 路由：

```txt
POST /v1beta/models/{model}:generateContent
POST /v1beta/models/{model}:streamGenerateContent
```

这两个路由复用标准 Gateway runtime：API Key 鉴权、模型可见性、entitlement、Scheduler、Provider Adapter、usage log、scheduler decision / feedback 均与 OpenAI/Anthropic 兼容入口一致。WP-240 起，当 Scheduler 选择 `gemini-compatible`、`native-gemini` 或 `reverse-proxy-gemini-cli` Provider Account 时，Provider Adapter 会调用 Gemini `models/{model}:generateContent` 或 `models/{model}:streamGenerateContent` 上游。

### 4.4 Grok

```txt
/grok/v1/*
/api/provider/grok/*
/api/provider/grok/v1/*
```

Grok API-key/upstream 与 Grok Web session 都归属 `grok` provider，但 runtime_class 不同。

### 4.5 Antigravity

```txt
/antigravity/v1/*
/antigravity/v1beta/*
/api/provider/antigravity/*
/api/provider/antigravity/v1/*
/api/provider/antigravity/v1beta/*
```

Antigravity 可承载 Claude-shaped 和 Gemini-shaped 端点，必须在 route metadata 中标明目标子协议。
一阶后端反代身份已实现：管理员可配置 `adapter_type = reverse-proxy-antigravity`、
`runtime_class = desktop_client_token` 或 `ide_plugin_token`、`upstream_client = antigravity_desktop`，
并通过现有 `/v1/chat/completions`、`/v1/messages` 或 Gemini-native Gateway 路径进入
Scheduler / Provider Adapter / Reverse Proxy Runtime。上游子协议由 `provider.protocol` 决定。
WP-360 起，Antigravity 文本 alias 已实现：`/antigravity/v1/chat/completions`、
`/api/provider/antigravity/v1/chat/completions`、`/antigravity/v1/messages` 和
`/api/provider/antigravity/v1/messages` 强制 `provider_key=antigravity`，仍复用标准
Gateway runtime。WP-370 起，Antigravity Gemini model-action alias 已实现：
`/antigravity/v1beta/models/{model}:generateContent`、
`/antigravity/v1beta/models/{model}:streamGenerateContent`、
`/api/provider/antigravity/v1beta/models/{model}:generateContent` 和
`/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent` 强制
`provider_key=antigravity`，并复用标准 Gemini-native Gateway handler。

## 5. Route Matrix

| Inbound family | Runtime owner | Handler 职责 | 阶段 |
| --- | --- | --- | --- |
| `/v1/models` | Gateway model service | 聚合 API Key 可见模型，按 group/provider 过滤。 | MVP |
| `/v1/chat/completions` | Compatible text runtime | 校验 OpenAI body，转 Canonical AI Request，渲染 OpenAI 响应。 | MVP |
| `/v1/responses` | Compatible text runtime | 校验 Responses body，处理 stream/tools/structured output 兼容边界。 | MVP |
| `/v1/messages` | Anthropic-compatible runtime | 校验 Messages body，转 Canonical AI Request，渲染 Anthropic 响应。 | MVP |
| `/v1/embeddings` | Passthrough runtime | 最小解析 model/input，调度后调用 OpenAI-compatible `/embeddings`，记录用量和调度证据。 | WP-270 |
| `/v1/images/generations` | Media runtime | 最小解析 OpenAI image generation body，调度后调用 OpenAI-compatible `/images/generations`，记录 media/usage 和调度证据。 | WP-290 |
| `/v1/images/edits`, `/v1/images/variations` | Media runtime | 图片编辑和 variation。 | Phase 3 |
| `/v1/audio/transcriptions` | Audio runtime | 最小解析 multipart audio transcription body，调度后调用 OpenAI-compatible `/audio/transcriptions`，记录 audio/usage 和调度证据。 | WP-330 |
| `/v1/audio/speech` | Audio runtime | 最小解析 OpenAI speech JSON body，调度后调用 OpenAI-compatible `/audio/speech`，返回 binary audio 并记录 usage 和调度证据。 | WP-340 |
| `/v1/moderations` | Moderation runtime | 最小解析 OpenAI moderation body，调度后调用 OpenAI-compatible `/moderations`，记录用量和调度证据。 | WP-310 |
| `/v1/rerank` | Passthrough runtime | 最小解析 query/documents/top_n，调度后调用 rerank-compatible `/rerank`，记录用量和调度证据。 | WP-320 |
| `/v1/responses/ws` | Realtime/WS runtime | 长连接、粘性账号、slot 生命周期。 | Phase 3 |
| Gemini `/v1beta/models/*:generateContent` | Compatible text runtime | Google-shaped request/response/error，复用 Canonical/Scheduler/Provider Adapter。 | WP-230 |
| Gemini `/v1beta/models/*:streamGenerateContent` | Compatible text runtime | Google-shaped SSE response/error，复用 Canonical/Scheduler/Provider Adapter。 | WP-230 |
| Gemini upstream `generateContent` | Provider Adapter | Canonical text request 转 Gemini payload，解析 Gemini response/SSE/error/usage。 | WP-240 |
| Native `count_tokens` | Provider native runtime | 计数请求，不进入生成用量。 | Phase 2 |

## 6. Passthrough 规则

Passthrough 不代表裸转发。仍必须执行：

- API Key 鉴权。
- 用户余额 / entitlement 检查。
- 模型可见性检查。
- Scheduler 选择账号。
- Provider Account credential materialization。
- 用量或成本记录。
- 错误分类。
- 日志脱敏。

## 7. 模型 ID 规则

模型 ID 可能包含 `/`，路由参数必须支持 wildcard-backed model id。

示例：

```txt
/v1/models/deepseek/deepseek-chat
/api/provider/openrouter/v1/models/google/gemini-pro
```

Handler 不得简单按单段 path param 截断模型名。

## 8. 多 group API Key 解析

如果 API Key 绑定多个 group：

1. 从 request body 或 path 中解析 model。
2. 通过模型前缀和 provider alias 推断 platform。
3. 过滤该 API Key 可用 group。
4. 如果多个 group 命中，按 `rate_multiplier ASC`、`sort_order ASC`、`id ASC` 确定运行时 group。
5. `/v1/models` 无单一模型时返回所有可见模型并集。

数据模型需在 `api_key_groups` 中支持多对多关系。

## 9. 错误渲染

内部错误必须保留 SRapi typed error，但对外按源协议渲染：

| 源协议 | 错误形状 |
| --- | --- |
| OpenAI-compatible | OpenAI error object。 |
| Anthropic Messages | Anthropic error object。 |
| Gemini native | Google RPC-style error。 |
| 管理 API | SRapi standard error。 |

## 10. 测试要求

每个路由族必须覆盖：

- 鉴权失败。
- 模型不可见。
- 无可用账号。
- Provider 429/5xx 映射。
- 流式中断。
- request_id 贯穿。
- usage / decision 记录。
- provider alias 与标准路径行为一致。

## 11. 与其他文档关系

- 端点互转语义：`AI_ENDPOINT_COMPATIBILITY.md`。
- Provider preset：`COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`。
- Provider Adapter：`PROVIDER_ADAPTER_SPEC.md`。
- 反代账号运行时：`REVERSE_PROXY_SPEC.md`。
- Scheduler 选择：`SCHEDULER_V1_SPEC.md`。
