# SRapi Gateway 路由矩阵规范

## 1. 目标

本文档定义 SRapi 对外 Gateway API 的路由族、Provider-prefixed alias、passthrough 边界、运行时归属和实现状态。

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
| WebSocket / Realtime | `/v1/responses/ws`、`/v1/realtime` | 长连接代理，需特殊 slot 和粘性策略。 |

## 3. 核心入口

以下核心兼容入口均已实现并注册：

```txt
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
POST /v1/responses/compact
POST /v1/messages
```

入口约束：

- API Key 鉴权。
- 模型可见性过滤。
- Chat Completions / Responses / Messages 转 Canonical AI Request。
- 非流式和 SSE 流式。
- Scheduler decision / feedback / usage log。
- OpenAI-compatible 错误渲染。
- 已实现的 Provider alias 必须复用标准 Gateway runtime，只改变 provider context，并把实际 alias path 写入 usage log 与 scheduler decision；如果标准入口支持 `Idempotency-Key`，对应 alias 必须继承同一幂等语义。

当前 Provider alias 由 Compatible Provider preset registry 动态注册，不为每个 Provider 复制 handler。已实现规则：

```txt
OpenAI-compatible preset:
  POST {alias}/v1/chat/completions
  POST {alias}/v1/responses
  GET  {alias}/v1/responses/{response_id}/input_items
  POST {alias}/v1/responses/compact
  POST {alias}/v1/messages
  POST {alias}/v1/embeddings
  POST {alias}/v1/images/generations
  POST {alias}/v1/images/edits
  POST {alias}/v1/images/variations
  POST {alias}/v1/videos
  GET  {alias}/v1/videos/{video_id}
  GET  {alias}/v1/videos/{video_id}/content
  POST {alias}/v1/audio/transcriptions
  POST {alias}/v1/audio/speech
  POST {alias}/v1/moderations

Anthropic-compatible preset:
  POST {alias}/v1/messages

Antigravity preset:
  POST {alias}/v1/chat/completions
  POST {alias}/v1/messages
  GET  {gemini_alias}/models
  GET  {gemini_alias}/models/{model}
  POST {gemini_alias}/models/{model}:generateContent
  POST {gemini_alias}/models/{model}:streamGenerateContent
  POST {gemini_alias}/models/{model}:countTokens
```

`{alias}` 来自 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` 的 `route_aliases`；`{gemini_alias}` 来自同一 preset 的 Gemini model-action aliases。例如 `/api/provider/deepseek/v1/chat/completions` 强制 `provider_key=deepseek`，但仍复用标准 Gateway runtime、API Key policy、model visibility、Scheduler、usage 和 decision 记录。

Provider alias 只通过 `/api/provider/{provider_key}` 路由族暴露。所有 alias 只强制对应 `provider_key`，并把原始 alias path 写入 usage log 与 scheduler decision 的 `source_endpoint` 证据。

## 4. Provider alias 路由

### 4.1 OpenAI / OpenAI-compatible

```txt
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

Gemini 原生错误必须渲染为 Google-compatible 形状；通过 `/v1/messages` 或 `/v1/chat/completions` 进入的请求则按客户端源协议渲染。Gemini-shaped 路由的 Gateway API key 可来自 `x-goog-api-key`、`Authorization: Bearer`、`x-api-key` 或 `key` query 参数；其他查询参数不参与 Gateway 鉴权。

已实现的 Gemini-native 路由：

```txt
GET  /v1beta/models
GET  /v1beta/models/{model}
POST /v1beta/models/{model}:generateContent
POST /v1beta/models/{model}:streamGenerateContent
POST /v1beta/models/{model}:countTokens
```

`generateContent` 和 `streamGenerateContent` 路由复用标准 Gateway runtime：API Key 鉴权、模型可见性、entitlement、Scheduler、Provider Adapter、usage log、scheduler decision / feedback 均与 OpenAI/Anthropic 兼容入口一致，并要求候选账号具备 `gemini_generate_content.v1` effective capability。WP-240 起，当 Scheduler 选择 `gemini-compatible`、`native-gemini`、`reverse-proxy-gemini-cli` 或 Antigravity Gemini alias Provider Account 时，Provider Adapter 会调用 Gemini `models/{model}:generateContent`、`models/{model}:streamGenerateContent` 或 Antigravity `v1internal` 上游。WP-540 起，`GET /v1beta/models` 返回 Gemini `models.list` 兼容响应，只基于 SRapi 模型 registry 和 API Key 可见性渲染 Google-shaped model list，不调度 Provider Account，也不读取上游凭证。WP-640 起，`GET /v1beta/models/{model}` 返回 Gemini `models.get` 兼容响应，复用相同 registry-backed model metadata 和 API Key 可见性边界。WP-550 起，`POST /v1beta/models/{model}:countTokens` 复用 Gateway policy、entitlement、Scheduler、Provider Adapter 和 Reverse Proxy Runtime 边界，要求 `gemini_count_tokens.v1` + `token_counting.v1`，调用选中上游 Gemini `models/{mapped_model}:countTokens`，但成功请求不进入生成用量，usage tokens 和 cost 记 0。

### 4.4 Grok

```txt
/api/provider/grok/*
/api/provider/grok/v1/*
```

Grok API-key/upstream 与 Grok Web session 都归属 `grok` provider，但 runtime_class 不同。

### 4.5 Antigravity

```txt
/api/provider/antigravity/*
/api/provider/antigravity/v1/*
/api/provider/antigravity/v1beta/*
```

Antigravity 可承载 Claude-shaped 和 Gemini-shaped 端点，必须在 route metadata 中标明目标子协议。
一阶后端反代身份已实现：管理员可配置 `adapter_type = reverse-proxy-antigravity`、
`runtime_class = oauth_refresh`、`upstream_client = antigravity_desktop`，
并通过现有 `/v1/chat/completions`、`/v1/messages` 或 Gemini-native Gateway 路径进入
Scheduler / Provider Adapter / Reverse Proxy Runtime。WP-450 起，上游请求由 Provider Adapter
转换为 Antigravity / Google Cloud Code `v1internal` official-client shape；`provider.protocol`
只决定下游协议归一化和响应渲染。
Antigravity 文本 alias 已实现：`/api/provider/antigravity/v1/chat/completions` 和
`/api/provider/antigravity/v1/messages` 强制 `provider_key=antigravity`，仍复用标准
Gateway runtime。Antigravity Gemini model alias 已实现：
`/api/provider/antigravity/v1beta/models` 和
`/api/provider/antigravity/v1beta/models/{model}` 只返回可见且映射到
`antigravity` provider 的 registry-backed model metadata；
`/api/provider/antigravity/v1beta/models/{model}:generateContent`、
`/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent` 和
`/api/provider/antigravity/v1beta/models/{model}:countTokens` 强制
`provider_key=antigravity`，并复用标准 Gemini-native Gateway handler。
Antigravity adapter 会把生成请求转为 `v1internal:generateContent` /
`v1internal:streamGenerateContent`，把计数请求转为 `v1internal:countTokens`；
成功 `countTokens` 请求 usage tokens 和 cost 记 0。

## 5. Route Matrix

> 状态列：**Shipped** = 已实现并注册的对外路由；**Roadmap** = 尚未实现的规划项。括号内的历史 WP 编号仅作溯源参考。

| Inbound family | Runtime owner | Handler 职责 | Status |
| --- | --- | --- | --- |
| `/v1/models` | Gateway model service | 聚合 API Key 可见模型，按 group/provider 过滤。 | Shipped |
| `/v1/usage` | Gateway usage snapshot runtime | Gateway API Key 自查用量与额度；返回当前 Bearer key 的窗口聚合、今日用量、模型分布、最近请求、key 限流/模型策略和 owner wallet balance，不暴露同一用户的其他 key。 | Shipped |
| `/v1/chat/completions` | Compatible text runtime | 校验 OpenAI body，转 Canonical AI Request，渲染 OpenAI 响应。 | Shipped |
| `/v1/responses` | Compatible text runtime | 校验 Responses body，处理 stream/tools/structured output 兼容边界。默认 OpenAI-compatible provider 继续降级到 `/chat/completions`；`native-openai`、provider.name=openai 或显式 native Responses opt-in 时发送上游 `/responses` 并允许同协议 raw JSON/SSE 回放。`gpt-image-*` 顶层 image-only model 仅在配置 Responses main model 后迁移为 `image_generation` tool，否则拒绝。 | Shipped |
| `/v1/responses/{response_id}/input_items` | Responses subresource runtime | OpenAI Responses input_items 子资源；Gateway 通过 query `model` 做 API Key 模型策略、entitlement、Scheduler 和 Provider Adapter 选择，并要求 `responses_input_items` effective capability；普通 `responses` 生成能力不能替代该子资源能力。选中账号调用上游 `/responses/{response_id}/input_items` 并原样回放 JSON list。`model` 只用于 SRapi 调度，不转发上游；成功请求 usage tokens 和 cost 记 0。 | Shipped |
| `/v1/responses/compact` | Responses compact runtime | OpenAI Responses compact 子资源；复用 Gateway auth/model policy/Scheduler/Provider Adapter/usage evidence 链路，并要求 `responses_compact` effective capability。该路由按非流式契约处理，即使请求体带 `stream=true` 也返回单个 JSON body，并支持可选 `Idempotency-Key` 重放保护。OpenAI-compatible API-key / OpenAI reverse-proxy / `reverse-proxy-codex-cli` 同协议请求发送到上游 `/responses/compact` 并原样回放 `response.compaction` JSON。跨协议不做伪转换。 | Shipped |
| `/v1/messages` | Anthropic-compatible runtime | 校验 Messages body，转 Canonical AI Request，渲染 Anthropic 响应。 | Shipped |
| `/v1/messages/count_tokens` | Anthropic-compatible native runtime | Anthropic-shaped count_tokens request/response/error；Gateway 只用 Canonical request 做 policy / entitlement / Scheduler，并要求 `anthropic_count_tokens` + `token_counting`；Provider Adapter 保留 Anthropic body shape、映射上游 model 后调用 `/messages/count_tokens`，Claude Code 2api 账号走 Reverse Proxy Runtime official-client shape，显式 opt-in 的 OpenAI-compatible 账号可返回本地估算，成功请求不进入生成用量。 | Shipped (WP-560) |
| `/v1/embeddings` | Passthrough runtime | 最小解析 model/input，调度后调用 OpenAI-compatible `/embeddings`，记录用量和调度证据。 | Shipped (WP-270) |
| `/v1/images/generations` | Media runtime | 最小解析 OpenAI image generation body，调度前要求 `image_generations` effective capability，调度后调用 OpenAI-compatible `/images/generations`，记录 media/usage 和调度证据。 | Shipped (WP-290) |
| `/v1/images/edits` | Media runtime | 最小解析 OpenAI-compatible multipart image edit body 和 JSON 本地图像引用，支持 `image` / `image[]` / `images`、可选 `mask`、`prompt/model` 与输出选项；JSON `image`/`images`/`mask` 只接受本地 data URL / base64 payload。调度前要求 `image_edits` effective capability；OpenAI-compatible API-key / 普通 reverse-proxy accounts 调用 multipart `/images/edits`，`reverse-proxy-codex-cli` accounts 转换为 Codex `/responses` `image_generation` tool `action=edit`，记录 media/usage 和调度证据。`stream=true` 返回 SSE 的最终 `image.generation.result` chunk；Remote URL 和 `file_id` references 仍需后续安全边界。 | Shipped (WP-480, WP-510, WP-520) |
| `/v1/images/variations` | Media runtime | 最小解析 OpenAI-compatible multipart image variation body 和 JSON 本地图像引用，支持单个 `image`、`model`、`n`、`size`、`response_format` 和 `user`，调度前要求 `image_variations` effective capability，调度后调用 OpenAI-compatible multipart `/images/variations`，记录 media/usage 和调度证据。Remote URL、`file_id` 和多图 variation 仍需后续 Files API / secure fetch 边界；OpenAI 官方上游当前仅支持 `dall-e-2`。 | Shipped (WP-490) |
| `/v1/videos` | Video runtime | 最小解析 OpenAI-style video generation body，调度前要求 `videos` capability，调度后调用 xAI/OpenAI-compatible `/videos/generations`，记录 usage、quota signals 和调度证据；成功创建的视频 ID 绑定到选中 Provider Account，后续读取必须命中该绑定。 | Shipped |
| `/v1/videos/{video_id}` | Video runtime | 读取 OpenAI-style video metadata；Gateway 先按 API Key + video ID 查找创建时账号绑定，再使用 hard sticky 调度到同一账号并调用上游 `/videos/{video_id}`。缺少或过期绑定返回 `video_binding_not_found`，避免跨账号读取。 | Shipped |
| `/v1/videos/{video_id}/content` | Video runtime | 读取 video content binary stream；复用 video ID 账号绑定和 hard sticky 调度，Provider Adapter 先读取上游 video metadata 中的 content URL，再用选中账号凭证下载并流式返回。 | Shipped |
| `/v1/audio/transcriptions` | Audio runtime | 最小解析 multipart audio transcription body，调度后调用 OpenAI-compatible `/audio/transcriptions`，记录 audio/usage 和调度证据。 | Shipped (WP-330) |
| `/v1/audio/speech` | Audio runtime | 最小解析 OpenAI speech JSON body，调度后调用 OpenAI-compatible `/audio/speech`，返回 binary audio 并记录 usage 和调度证据。 | Shipped (WP-340) |
| `/v1/moderations` | Moderation runtime | 最小解析 OpenAI moderation body，调度后调用 OpenAI-compatible `/moderations`，记录用量和调度证据。 | Shipped (WP-310) |
| `/v1/rerank` | Passthrough runtime | 最小解析 query/documents/top_n，调度后调用 rerank-compatible `/rerank`，记录用量和调度证据。 | Shipped (WP-320) |
| `/v1/responses/ws` | Realtime/WS runtime | WebSocket transport for Responses-compatible `response.create` events; by default each frame still enters the standard Responses Gateway runtime, Scheduler, Provider Adapter, usage log, and scheduler decision chain. Query/header sticky hints feed Scheduler session affinity. With explicit `upstream_ws` / `codex_responses_websocket` opt-in and an eligible `reverse-proxy-codex-cli` account, SRapi relays to Codex Responses WebSocket upstream using selected account OAuth/session/CLI credentials. Realtime slot lifecycle and deploy-level slot limits run before WebSocket upgrade; active slot summaries are exposed at `GET /api/v1/admin/ops/realtime/slots`; those limits and summaries are Redis-backed across API nodes when Redis is available, with local in-memory fallback only outside release mode. Additional provider-native realtime adapters remain Roadmap. | Shipped (WP-380, WP-410, WP-460, WP-570, WP-590) |
| `/v1/realtime` | Realtime/WS runtime | OpenAI-compatible Realtime WebSocket upgrade. Gateway authenticates the SRapi API key, resolves query `model`, requires `realtime_websocket` capability, acquires a realtime slot before upgrade, schedules a Provider Account, and asks Provider Adapter for the upstream Realtime WebSocket session. API-key accounts use direct official API-key upstream relay with selected-account credentials only; OAuth/session/client-token accounts use Reverse Proxy Runtime for bidirectional frame relay. Caller auth/cookie/SRapi headers do not define upstream identity. This is `GET /v1/realtime`, not `POST /v1/realtime`. | Shipped (WP-470, WP-630) |
| Gemini `/v1beta/models` | Gemini model list runtime | Google-shaped `models.list` response from active SRapi model registry entries visible to the Gateway API key and backed by active provider/account availability (`excluded_models` / `supported_models`). Supports `pageSize` / `pageToken`; does not acquire Scheduler lease or touch Provider Account credentials. | Shipped (WP-540) |
| Gemini `/v1beta/models/{model}` | Gemini model metadata runtime | Google-shaped `models.get` response for a single active SRapi model registry entry visible to the Gateway API key and backed by active provider/account availability. Accepts canonical names with or without a leading `models/`; does not acquire Scheduler lease or touch Provider Account credentials. | Shipped (WP-640) |
| Gemini `/v1beta/models/*:generateContent` | Compatible text runtime | Google-shaped request/response/error，复用 Canonical/Scheduler/Provider Adapter，并要求 `gemini_generate_content` endpoint capability。 | Shipped (WP-230) |
| Gemini `/v1beta/models/*:streamGenerateContent` | Compatible text runtime | Google-shaped SSE response/error，复用 Canonical/Scheduler/Provider Adapter，并要求 `gemini_generate_content` endpoint capability。 | Shipped (WP-230) |
| Gemini upstream `generateContent` | Provider Adapter | Canonical text request 转 Gemini payload，解析 Gemini response/SSE/error/usage。 | Shipped (WP-240) |
| Gemini `/v1beta/models/*:countTokens` | Provider native runtime | Google-shaped countTokens request/response/error，Gateway 只用 Canonical request 做 policy / entitlement / Scheduler，并要求 `gemini_count_tokens` + `token_counting`；Provider Adapter 发送原始 Gemini countTokens body 到选中上游，成功请求不进入生成用量。 | Shipped (WP-550) |
| 其他协议 native `count_tokens` | Provider native runtime | Anthropic 和 Gemini 的计数已实现（见上两行）；其余协议的原生计数请求尚未实现。 | Roadmap |

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
