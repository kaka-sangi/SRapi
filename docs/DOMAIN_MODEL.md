# SRapi 领域模型

## 1. 目的

本文档定义 SRapi 的核心领域概念，避免在后续开发中出现概念混用、职责重叠和模块边界不清的问题。

SRapi 的核心领域可以概括为：

```txt
用户通过 API Key 请求某个模型，Gateway 将请求标准化后交给 Scheduler。
Scheduler 根据用户权益、模型能力、账号状态、缓存亲和、成本策略和可用性选择一个 Provider Account。
Provider Adapter 使用该账号调用上游 AI Provider。
请求结果回流到 Usage、Billing、Scheduler Feedback 和 Observability。
```

## 2. 核心概念关系

```txt
User
  ├── Workspace
  ├── API Key
  ├── Subscription
  ├── Balance / Ledger
  └── Usage Log

Provider
  ├── Provider Adapter
  ├── Provider Account
  └── Provider Model Mapping

Model Registry
  ├── Model Alias
  ├── Model Capability
  └── Pricing Rule

Gateway Request
  ├── Request Profile
  ├── Scheduler Decision
  ├── Lease
  ├── Provider Dispatch
  └── Scheduler Feedback
```

## 3. User

用户是 SRapi 的资源消费者和控制台主体。

用户可以：

- 登录控制台。
- 创建 API Key。
- 查看用量。
- 购买套餐。
- 管理自己的配置。

关键属性：

```txt
id
email
name
role
status
workspace_id
balance
created_at
updated_at
```

用户状态：

```txt
active
suspended
deleted
```

新用户默认归属一个个人 Workspace。Workspace 是内部多租户边界，按设计不对外暴露独立的 Workspace 管理 API；持久化层会在创建用户时为未指定 `workspace_id` 的用户创建 `personal-<user_id>` Workspace 并写回用户。

## 4. Workspace

Workspace 是多租户边界，用于把用户、API Key 和权限/安全策略聚合到同一租户作用域。当前作用域只支持个人 Workspace，不建 Organization/Team；Organization/Team 多人协作仍属 Roadmap，尚未实现。

关键属性：

```txt
id
name
slug
owner_user_id
type
status
metadata
created_at
updated_at
```

规则：

- `type=personal` 表示个人工作区。
- `users.workspace_id` 和 `api_keys.workspace_id` 为 nullable，便于增量迁移和导入数据；新建用户默认会有个人 Workspace。
- API Key 创建时如果未指定 Workspace，会继承 owner user 的 Workspace。

## 5. Role

角色用于控制后台权限。内置角色负责平台级管理入口，自定义角色用于细粒度 Admin API 授权。

内置角色（不可修改、不可删除）：

```txt
owner
admin
operator
user
```

角色字段：

```txt
name
description
permissions
```

规则：

- `permissions` 是 `["resource:action"]` 字符串数组，例如 `payment_order:read`。
- `owner` 和 `admin` 保留全量管理权限；内置角色（`owner`/`admin`/`operator`/`user`）不可被修改或删除。自定义角色通过 Admin Roles API 创建/编辑/删除（`/api/v1/admin/roles`），再分配给用户。
- 运行时从用户角色合并 permissions 到 session user DTO，Admin API 可以选择 `owner/admin` 或特定 permission 放行。
- 角色只控制控制台和管理 API 权限，不直接控制模型访问。模型访问由 API Key、Subscription、Entitlement 和 User Group 共同决定。

## 6. User Group

用户组用于绑定策略、价格、权限和调度偏好。

用途：

- 设置可访问模型。
- 设置默认调度策略。
- 设置价格倍率。
- 设置限流。
- 设置账号池范围。

关键属性：

```txt
id
name
default_strategy
rate_multiplier
allowed_models
account_group_scope
status
```

## 7. API Key

API Key 是用户调用 Gateway 的凭证。

规则：

- 原文只展示一次。
- 数据库只保存哈希。
- 使用 key prefix 做快速定位。
- 可以绑定权限、模型范围、限流和过期时间。
- 默认继承 owner user 的 Workspace，后续 Workspace 级策略可按该字段过滤。

关键属性：

```txt
id
user_id
workspace_id
name
prefix
hash
status
scopes
allowed_models
rpm_limit
tpm_limit
concurrency_limit
expires_at
last_used_at
created_at
```

状态：

```txt
active
disabled
expired
revoked
```

## 8. Provider

Provider 表示一个 AI 服务商或协议类型。

示例：

```txt
openai
anthropic
gemini
grok
openrouter
openai-compatible
```

Provider 不是具体账号，也不是具体模型。它代表服务商和协议能力集合。

命名规则：

```txt
provider.name      表示业务服务商实体，例如 openai、anthropic、openrouter、自定义 openai-compatible 上游。
provider.adapter_type 表示代码适配器类型，例如 native-openai、native-anthropic、native-gemini、native-grok、openrouter、openai-compatible、anthropic-compatible。
provider.protocol  表示协议风格，例如 openai-compatible、anthropic-compatible。
```

`anthropic-compatible` 是统一名称，不使用 `claude-compatible`。

关键属性：

```txt
id
name
display_name
adapter_type
status
capabilities
created_at
updated_at
```

## 9. Provider Adapter

Provider Adapter 是对某个 Provider 协议的代码实现。

它负责：

- 请求转换。
- 响应转换。
- 流式响应处理。
- 错误分类。
- usage 解析。
- 认证注入。
- 健康检查。

Provider Adapter 不保存业务数据。

## 10. Provider Account

Provider Account 是可被调度使用的上游账号或上游凭证。

账号 runtime_class（与 `PROVIDER_ADAPTER_SPEC.md` / `REVERSE_PROXY_SPEC.md` 一致）：

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

`runtime_class != api_key` 的账号必须通过 Reverse Proxy Runtime 调用上游，详见 `REVERSE_PROXY_SPEC.md`。

关键属性：

```txt
id
provider_id
name
runtime_class
upstream_client
credential_ciphertext
cookie_jar_id
device_fingerprint_id
egress_profile_id
proxy_id
status
priority
weight
risk_level
created_at
updated_at
```

状态：

```txt
active
disabled
cooling_down
rate_limited
needs_reauth
suspended
dead
```

Provider Account 是 Scheduler 的主要候选对象。

## 11. Account Group

Account Group 是账号池分组。

用途：

- 将账号分配给不同用户组。
- 区分高质量账号、低成本账号、测试账号。
- 控制模型或 Provider 的路由范围。

关键属性：

```txt
id
name
description
provider_scope
model_scope
strategy_hint
status
```

## 12. Model Registry

Model Registry 是 SRapi 内部的模型目录。

它解决：

- 外部模型名和内部模型名映射。
- 模型能力描述。
- 模型价格。
- fallback 关系。
- 不同 Provider 对同一模型族的实现差异。

关键属性：

```txt
id
canonical_name
display_name
family
context_window
max_output_tokens
quality_tier
status
```

## 13. Model Alias

Model Alias 用于兼容不同客户端或商业命名。

示例：

```txt
gpt-4o -> openai:gpt-4o
claude-sonnet -> anthropic:claude-3-7-sonnet
fast-chat -> internal selected low-latency model
```

别名可以指向：

- 单一 canonical model。
- 一个 fallback model list。
- 一个 strategy-driven virtual model。

## 14. Model Capability

描述模型能力。

字段：

```txt
supports_stream
supports_tools
supports_vision
supports_json
supports_audio
supports_image_generation
supports_embeddings
supports_moderations
supports_rerank
supports_prompt_cache
supports_context_cache
supports_batch
```

能力是 Gateway 和 Scheduler 判断候选 Provider 的基础。

能力字段映射到 `CAPABILITY_TAXONOMY_SPEC.md` 的 canonical capability key。上面的 `supports_*` 字段只是 DTO 表达形式；能力注册、匹配、降级和版本化由 capability descriptor 承载（实现见 `apps/api/internal/modules/capabilities`）：

```txt
key
version
status
level
metadata_json
```

Gateway 负责从客户端请求提取 RequestCapability；Scheduler 只能使用 RequestCapability 与 EffectiveCapability 做候选过滤；Provider Adapter 负责声明 ProviderCapability。

## 15. Provider Model Mapping

Provider Model Mapping 表示某个 Provider 如何调用某个内部模型。

关键属性：

```txt
id
provider_id
model_id
upstream_model_name
capability_override
pricing_override
status
```

用途：

- 同一个 canonical model 可以由不同 Provider 提供。
- 不同 Provider 的上游模型名可能不同。
- 某个 Provider 可覆盖价格和能力。
- `capability_override` 必须遵守 `CAPABILITY_TAXONOMY_SPEC.md`，并参与 EffectiveCapability 计算。

## 16. Pricing Rule

Pricing Rule 描述模型或 Provider 的价格。

建议金额使用整数最小单位或 decimal，不能使用 float 存储真实账务金额。

字段：

```txt
model_id
provider_id
input_price_per_million_tokens
output_price_per_million_tokens
cache_read_price_per_million_tokens
cache_write_price_per_million_tokens
currency
effective_from
effective_to
```

## 17. Subscription Plan

Subscription Plan 是可售卖套餐。

它定义用户购买后获得什么权益。

字段：

```txt
id
name
description
price
currency
validity_days
entitlements
for_sale
sort_order
status
```

## 18. User Subscription

User Subscription 是用户实际拥有的订阅。

字段：

```txt
id
user_id
plan_id
status
starts_at
expires_at
entitlements_snapshot
created_at
updated_at
```

`entitlements_snapshot` 是订阅激活时复制的套餐权益证据，不作为 Gateway admission 的热查询源。active subscription 创建时会同步 materialize `Entitlement` 行，后续套餐变更不会回写既有订阅或 entitlement 缓存。

状态：

```txt
active
expired
cancelled
suspended
```

## 19. Entitlement

Entitlement 是用户权益的可查询执行层。落库为 `entitlements`，每行表示一个 feature，来源是 active user subscription 的 `entitlements_snapshot`。

示例：

```txt
allowed_models
monthly_token_quota
monthly_cost_quota
rpm_limit
tpm_limit
priority_level
scheduler_strategy
account_group_scope
```

Entitlement 可以来自：

- Subscription Plan。
- User Group。
- Admin override。
- API Key override。

解析优先级：

```txt
API Key override > User override > Subscription > User Group > System default
```

执行契约：

- Subscription 来源的 `entitlements` 查询层为热查询源，字段包括 `user_id`、`scope_type`、`scope_id`、`feature_key`、`value_json`、`quota_limit`、`expires_at`、`source_subscription_id`。
- `CheckEntitlement()` 从 active entitlement rows 合并 `allowed_models`、`account_group_scope`、`scheduler_strategy`、`monthly_token_quota`、`monthly_cost_quota` 后再进入 Scheduler。
- `entitlements_snapshot` 作为审计/防漂移证据保留；Gateway admission 读取 `entitlements`，并校验来源 subscription 的 active window。

## 20. Quota

Quota 表示额度或限制。

类型：

```txt
user_daily_cost
user_monthly_cost
api_key_rpm
api_key_tpm
account_daily_quota
account_monthly_quota
provider_rate_limit
subscription_token_quota
```

Quota 有两类：

- 用户侧 quota：决定用户是否能发起请求。
- 账号侧 quota：决定某个账号是否能被调度。

## 21. Gateway Request

Gateway Request 是客户端发来的模型调用请求。

它在进入 Scheduler 前会被标准化为统一结构。

字段：

```txt
request_id
user_id
api_key_id
model
messages
input_modalities
stream
tools
metadata
idempotency_key
```

## 22. Request Profile

Request Profile 是 Gateway 请求进入 Scheduler 前的标准化画像。

它来自 `AI_ENDPOINT_COMPATIBILITY.md` 定义的 Canonical AI Request，而不是某个具体客户端端点。

字段：

```txt
request_id
source_protocol
source_endpoint
model
estimated_input_tokens
estimated_output_tokens
is_stream
is_long_context
idempotency_key
requires_tools
requires_vision
conversation_hash
session_hash
priority
strategy_hint
```

## 22.1 Client Endpoint Adapter

Client Endpoint Adapter 负责把不同客户端协议转换为 Canonical AI Request，并把 Canonical AI Response 渲染回源端点格式。

已支持的源端点（均已注册路由，见 `apps/api/internal/httpserver/server.go` 与 `apps/api/internal/modules/gateway/contract` 的 `SourceEndpoint`）：

```txt
openai_chat_completions       /v1/chat/completions
openai_responses              /v1/responses, /v1/responses/compact
anthropic_messages            /v1/messages, /v1/messages/count_tokens
gemini_generate_content       /v1beta/models/{model}:generateContent, :streamGenerateContent, :countTokens
embeddings                    /v1/embeddings
images                        /v1/images/generations, /edits, /variations
audio                         /v1/audio/transcriptions, /audio/speech
moderations                   /v1/moderations
rerank                        /v1/rerank
realtime                      /v1/realtime (WebSocket)
```

Roadmap / 尚未实现：

```txt
batch    # 上游 Batch API（如 OpenAI /v1/batches）暂未提供源端点
```

## 22.2 Canonical AI Request

Canonical AI Request 是 SRapi 内部所有 AI 调用的统一请求模型。

用途：

- 让 Scheduler 不依赖具体端点协议。
- 让 Provider Adapter 可以自由选择可用上游协议。
- 让 Chat Completions、Responses、Messages、GenerateContent 等端点可以相互转换。

关键属性以 `AI_ENDPOINT_COMPATIBILITY.md` 为准。

## 22.3 Canonical AI Response

Canonical AI Response 是 Provider Adapter 输出到 Gateway 的统一响应模型。

Gateway 必须根据调用方源端点将其渲染为：

```txt
OpenAI Chat Completions response
OpenAI Responses response
Anthropic Messages response
Gemini GenerateContent response
```

无法无损转换时必须携带 compatibility warning 或返回明确错误。

## 23. Scheduler Decision

Scheduler Decision 是一次调度选择的结果和解释。

字段：

```txt
request_id
attempt_no
strategy
strategy_version
candidate_count
rejected_count
selected_account_id
selected_provider_id
score_breakdown
reject_reasons
strategy_weights_json
sticky_hit
cache_affinity_hit
estimated_cost
```

它必须可审计、可观察、可用于调试。

## 24. Lease

Lease 是调度选中账号后的短期资源预占。

用于防止并发请求突破账号限制。

字段：

```txt
lease_id
request_id
account_id
estimated_tokens
estimated_cost
expires_at
status
```

状态：

```txt
pending
committed
released
expired
failed
```

## 25. Sticky Session

Sticky Session 是会话粘度绑定。

用途：

- 提升连续对话体验。
- 利用上游 session。
- 提升缓存命中率。

字段：

```txt
binding_key
binding_type
user_id
api_key_id
model
provider_id
account_id
strength
expires_at
last_seen_at
```

强度：

```txt
hard
soft
cache_only
none
```

## 26. Cache Affinity Record

Cache Affinity Record 表示某个 prompt prefix 或上下文在某个 Provider Account 上可能有缓存收益。

字段：

```txt
provider_id
model
account_id
prompt_prefix_hash
cached_token_estimate
cache_write_time
last_hit_time
ttl_seconds
```

## 27. Usage Log

Usage Log 是请求级用量记录。

字段：

```txt
request_id
user_id
api_key_id
provider_id
account_id
model
input_tokens
output_tokens
cached_tokens
latency_ms
success
error_class
created_at
```

Usage Log 是报表和计费的数据基础，但真实扣费应以 Billing Ledger 为准。

## 28. Billing Ledger

Billing Ledger 是账务流水。

规则：

- 不允许直接只改余额不记流水。
- 每次充值、扣费、退款、补偿都应产生 ledger 记录。
- ledger 记录不可随意修改，只能追加冲正记录。

字段：

```txt
id
user_id
type
amount
currency
balance_before
balance_after
reference_type
reference_id
created_at
```

类型：

```txt
recharge
usage_charge
refund
adjustment
compensation
```

## 29. Payment Order

Payment Order 是支付订单。

字段：

```txt
id
user_id
order_no
provider
amount
currency
status
provider_transaction_id
metadata
created_at
paid_at
closed_at
```

状态：

```txt
pending
paid
failed
closed
refunded
partially_refunded
```

## 30. Audit Log

Audit Log 是管理员和系统关键动作日志。

必须记录：

- 操作人。
- 操作类型。
- 目标资源。
- 变更前后摘要。
- IP。
- User-Agent。
- Trace ID。

## 31. Reverse Proxy Runtime

Reverse Proxy Runtime 是 Provider Adapter 之下的反代执行层。

职责：

- 加载 Egress Profile 决定 TLS、HTTP/2、Header、UA、cookie 注入策略。
- 在 `runtime_class != api_key` 的账号上构建独立 HTTP client。
- 注入凭证、cookie、device fingerprint。
- 防止 SRapi 内部标识泄漏到上游。
- 识别上游封号 / 风控信号并反馈给账号生命周期。

详见 `REVERSE_PROXY_SPEC.md`。

## 32. Egress Profile

Egress Profile 是反代请求的指纹模板。

关键属性：

```txt
id
name
upstream_client
tls_template
http2_template
http_version_policy
user_agent
header_order_template
header_set_template
accept_language
accept_encoding
sec_ch_ua_template
forbidden_headers
body_encoding
stream_format
behavior_pacer
challenge_strategy
client_version_pin
version
last_validated_at
```

Egress Profile 必须可在管理后台版本化更新。升级 Profile 不得改写旧的 decision 与 usage 记录。

## 33. Cookie Jar

Cookie Jar 是每账号独立的 cookie 集合。

关键属性：

```txt
id
account_id
cookies_ciphertext
last_set_at
last_used_at
expires_hint_at
```

要求：

- 加密存储。
- 不得跨账号共享。
- 必须遵守上游 `Set-Cookie` 的 Domain、Path、Secure、HttpOnly、SameSite、Expires。

## 34. Device Fingerprint

Device Fingerprint 描述某账号在反代请求中持续呈现的设备特征。

关键属性：

```txt
id
account_id
client_id
installation_id
device_id
machine_id
locale
timezone
platform
client_version
metadata_ciphertext
created_at
last_used_at
```

要求：

- 仅 `runtime_class` 为 `web_session_cookie` / `desktop_client_token` / `ide_plugin_token` / `cli_client_token` 时必填。
- 同账号同请求必须使用同一 Device Fingerprint，不得在请求间随机切换。

## 35. 概念边界总结

- Provider 是服务商或协议类型。
- Provider Adapter 是 Provider 的代码实现。
- Reverse Proxy Runtime 是 Provider Adapter 下的反代执行层。
- Egress Profile 是反代请求的指纹模板。
- Cookie Jar / Device Fingerprint 是反代账号上下文。
- Provider Account 是具体可调度账号。
- Model Registry 是内部模型目录。
- Provider Model Mapping 是 Provider 对内部模型的实现映射。
- API Key 是用户调用凭证。
- Subscription 是用户权益来源。
- Entitlement 是最终可执行权限。
- Quota 是限制和额度。
- Scheduler Decision 是调度解释。
- Lease 是运行时资源预占。
- Usage Log 是请求用量事实。
- Billing Ledger 是账务事实。
