# SRapi 能力分类与版本化规范

## 1. 目标

本文档定义 SRapi 的模型能力、Provider 能力、请求能力和端点能力的统一分类、命名、版本化和兼容规则。

目标：

- 适应 AI Provider 和模型能力快速变化。
- 防止能力字段在各模块中重复定义、命名漂移。
- 让 Gateway、Scheduler、Provider Adapter、Model Registry 使用同一能力语义。
- 支持 capability downgrade、experimental capability 和 provider-specific extension。

## 2. 能力对象类型

SRapi 至少存在四类能力对象：

```txt
RequestCapability      一次请求需要的能力
ModelCapability        内部 canonical model 声明的能力
ProviderCapability     Provider / Adapter 声明的能力
EndpointCapability     客户端端点协议可表达的能力
```

关系：

```txt
EndpointCapability -> RequestCapability
RequestCapability  -> Scheduler hard filter
ModelCapability    -> CandidateBuilder
ProviderCapability -> Adapter selection and runtime constraints
```

## 3. 命名规则

能力 key 使用 lower_snake_case：

```txt
text_generation
streaming
tool_calling
parallel_tool_calls
vision_input
json_mode
structured_output
reasoning
prompt_cache
context_cache
usage_in_stream
```

禁止：

```txt
supportsXXX
hasXXX
claudeCompatibleFeature
openAIJsonMode
```

布尔字段可以在 DTO 中使用 `supports_` 前缀，但 registry 中的 canonical key 必须不带 `supports_`。

示例：

```txt
canonical key: tool_calling
DTO field: supports_tools
```

## 4. 能力分类

### 4.1 输入能力

```txt
text_input
image_input
audio_input
video_input
file_input
pdf_input
browser_state_input
code_context_input
```

### 4.2 输出能力

```txt
text_output
image_output
audio_output
video_output
embedding_output
rerank_output
moderation_output
structured_output
```

### 4.2.1 端点族能力

Endpoint capability 用于调度路由族，不替代输入/输出能力：

```txt
chat_completions
responses
messages
embeddings
images
moderations
rerank
```

`moderations` 表示 Provider Account 能处理 `/v1/moderations` 兼容端点；`moderation_output` 表示模型/Provider 能产出审核分类结果。Gateway moderation 请求必须要求 `moderations.v1`，避免只具备文本生成能力的候选账号被误选。

`rerank` 表示 Provider Account 能处理 `/v1/rerank` 兼容端点；`rerank_output` 表示模型/Provider 能产出排序评分。Gateway rerank 请求必须要求 `rerank.v1`，避免 generation-only 或 embedding-only provider 被误选。

### 4.3 交互能力

```txt
streaming
tool_calling
parallel_tool_calls
function_calling
computer_use
web_search
code_execution
mcp_tool_use
stateful_session
realtime_websocket
```

### 4.4 控制能力

```txt
json_mode
response_schema
reasoning_control
thinking_budget
temperature_control
top_p_control
logprobs
seed_control
batch_request
```

### 4.5 缓存能力

```txt
prompt_cache
context_cache
kv_cache_affinity
cache_read_usage
cache_write_usage
```

### 4.6 Usage 能力

```txt
input_token_usage
output_token_usage
cached_token_usage
reasoning_token_usage
audio_token_usage
usage_in_stream
provider_cost_hint
```

### 4.7 安全和运行时能力

```txt
oauth_refresh
web_session_cookie
device_fingerprint_required
http2_required
egress_profile_required
region_pinning
rate_limit_headers
quota_headers
```

## 5. Capability Descriptor

每个能力应使用 descriptor 表示：

```txt
key
version
status
level
source
first_seen_at
last_verified_at
metadata_json
```

字段：

- `key`：canonical capability key。
- `version`：能力语义版本，例如 `v1`。
- `status`：`stable`、`experimental`、`deprecated`、`disabled`。
- `level`：`native`、`emulated`、`partial`、`unsupported`。
- `source`：`manual`、`adapter_declared`、`provider_preset`、`runtime_probe`。
- `metadata_json`：能力参数，例如 max images、schema dialect、reasoning budget range。

## 6. RequestCapability

Gateway 根据客户端端点和请求 body 生成 RequestCapability：

```txt
required_capabilities
optional_capabilities
forbidden_capabilities
quality_preferences
compatibility_warnings
```

示例：

```txt
required_capabilities:
  - text_input.v1
  - text_output.v1
  - streaming.v1
  - tool_calling.v1
optional_capabilities:
  - usage_in_stream.v1
quality_preferences:
  - low_latency
  - low_cost
```

Scheduler hard filter 只使用 `required_capabilities`，不得因 optional capability 不满足而直接拒绝候选。

## 7. ModelCapability

Model Registry 中的能力用于表达 canonical model 的业务能力。

建议字段：

```txt
capabilities_json
capability_version
capability_source
capability_verified_at
```

能力示例：

```txt
text_input.v1: native
text_output.v1: native
tool_calling.v1: native
vision_input.v1: partial
prompt_cache.v1: unsupported
```

ModelCapability 不应包含具体 Provider 的认证、Header、URL 或 runtime 细节。

## 8. ProviderCapability

ProviderCapability 由 Provider Adapter、Provider Preset 和运行时探测共同产生。

字段：

```txt
provider_id
adapter_type
protocol
runtime_class
capabilities_json
capability_version
verified_at
```

ProviderCapability 可以包含：

- 协议能力。
- runtime 约束。
- usage 解析能力。
- rate limit / quota 能力。
- stream 能力。

不得包含：

- 明文凭证。
- 用户权益。
- Scheduler strategy。

## 9. Provider Model Mapping Override

`model_provider_mappings.capability_override` 可以覆盖 Provider 对某个模型的能力。

覆盖规则：

1. ModelCapability 提供 canonical 默认能力。
2. ProviderCapability 提供 provider 级能力上限。
3. Provider Model Mapping override 提供具体模型在该 Provider 下的实际能力。
4. Account runtime state 可以临时禁用能力，例如 streaming 临时不可用。

最终能力：

```txt
EffectiveCapability = ModelCapability ∩ ProviderCapability ∩ MappingOverride ∩ AccountRuntimeState
```

## 10. 能力匹配规则

候选账号必须满足：

```txt
required_capabilities ⊆ effective_capabilities
```

对 `partial` 能力：

- 如果请求明确要求完整能力，则不匹配。
- 如果请求允许降级，则可匹配但必须产生 warning。

对 `emulated` 能力：

- 可以匹配，但 Scheduler 可增加 risk_penalty 或 latency_penalty。
- Gateway 必须在 compatibility warning 中标记。

## 11. 能力降级

Gateway 可以在用户或 API Key 策略允许时执行降级。

示例：

```txt
structured_output -> json_mode
usage_in_stream -> final_usage_only
parallel_tool_calls -> sequential_tool_calls
prompt_cache -> no_cache
```

降级必须记录：

```txt
request_id
original_capability
downgraded_capability
reason
client_visible_warning
```

禁止静默降级影响语义的能力，例如：

- vision input 被忽略。
- tool call 被删除。
- JSON schema 被丢弃。

## 12. Experimental Capability

实验能力必须标记：

```txt
status=experimental
stability=unstable
requires_feature_flag=true
```

实验能力不得默认向所有用户开放。

需要：

- feature flag。
- 明确文档。
- golden test。
- fallback 或明确错误。

## 13. Deprecated Capability

能力废弃流程：

1. 标记 `deprecated`。
2. 提供替代 capability key。
3. 保留至少一个 minor 版本周期。
4. 在 Admin Ops 中提示受影响 Provider / Model / API Key。
5. 移除前必须通过兼容性检查。

## 14. Capability Registry

建议在数据库或配置中维护 capability registry：

```txt
capability_definitions
```

字段：

```txt
id
key
version
category
status
description
schema_json
replacement_key
created_at
updated_at
```

索引：

```txt
unique(key, version)
index(category, status)
```

MVP 可以先以代码常量或 seed 数据实现，但语义必须与本文档一致。

## 15. Capability Schema

复杂能力必须定义 schema。

示例：

```txt
structured_output.v1:
  schema_dialects: [json_schema_draft_07, json_schema_2020_12]
  strict_mode: boolean
  max_schema_bytes: integer
```

```txt
vision_input.v1:
  max_images: integer
  supported_mime_types: string[]
  max_image_bytes: integer
```

```txt
reasoning_control.v1:
  levels: [low, medium, high]
  supports_budget_tokens: boolean
```

## 16. 与现有文档关系

- `DOMAIN_MODEL.md` 的 Model Capability 使用本文档的 canonical key。
- `DATA_MODEL.md` 的 capabilities JSON 字段使用本文档的 descriptor 结构。
- `PROVIDER_ADAPTER_SPEC.md` 的 ProviderCapabilities 必须映射到本文档。
- `SCHEDULER_V1_SPEC.md` 的 CandidateBuilder 必须使用 EffectiveCapability。
- `AI_ENDPOINT_COMPATIBILITY.md` 的 Canonical AI Request 必须能表达 RequestCapability。

## 17. 测试要求

能力相关测试必须覆盖：

- 请求能力提取。
- 模型能力匹配。
- Provider 能力匹配。
- Mapping override。
- partial / emulated 能力处理。
- 降级 warning。
- experimental feature flag。
- deprecated capability 兼容。
- 新旧 capability descriptor schema 兼容。

## 18. 新能力引入清单

新增能力前必须回答：

- capability key 是什么。
- 属于哪个 category。
- 语义版本是多少。
- 是否 stable。
- 是否需要 metadata schema。
- Gateway 如何从请求中提取。
- Scheduler 是否参与 hard filter。
- Provider Adapter 如何声明。
- 是否允许降级。
- 是否需要 OpenAPI 暴露。
- 是否需要 Admin UI 配置。
