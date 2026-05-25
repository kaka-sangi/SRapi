# SRapi 数据模型设计

## 1. 目标

本文档定义 SRapi 第一阶段核心数据模型，为后续 Ent schema、数据库迁移和 API 契约提供依据。

数据库建议：

```txt
PostgreSQL 16+
```

ORM / Schema：

```txt
Ent + Atlas migrations
```

## 2. 通用字段规范

大多数业务表建议包含：

```txt
id                 bigint / uuid
created_at         timestamptz
updated_at         timestamptz
deleted_at         timestamptz nullable
```

是否使用软删除：

- 用户、账号、API Key、配置类资源建议软删除。
- Ledger、Usage、Audit 不建议软删除。
- Payment order 不建议软删除。

## 3. ID 策略

第一阶段建议使用：

```txt
bigint snowflake-like id 或 PostgreSQL bigserial
```

如果希望更方便分布式和外部暴露，可以使用：

```txt
uuid / ulid
```

建议外部 API 不依赖连续自增 ID，可后续引入 public id。

## 4. 金额字段

禁止使用 float 存储真实账务金额。

推荐二选一：

```txt
numeric(20, 8)
```

或：

```txt
amount_minor bigint + currency
```

若需要支持多币种和高精度模型成本，建议使用 `numeric(20, 8)`。

## 5. JSON 字段使用原则

可以使用 `jsonb` 存储：

- Provider capabilities。
- Model capabilities。
- Capability descriptors。
- Scheduler score breakdown。
- Scheduler strategy config。
- Reject reasons。
- Domain event payload snapshot。
- Webhook payload snapshot。
- Metadata。
- Admin Control Plane v1 的低频 typed settings collections，例如 announcements、redeem codes、promo codes、risk-control config、ops settings 和 system settings。

不应把核心查询字段只放 JSON。

Admin Control Plane v1 可以先用 `settings.value_json` 承载低频管理资源集合，避免在控制台首版中引入过多表。以下能力一旦进入用户高频路径或需要独立约束，应提升为一等 Ent schema 和迁移：

- 用户侧兑换码核销。
- 优惠码订单应用和使用记录。
- 公告用户投递与阅读回执。
- 高吞吐风控事件。
- 高吞吐系统日志。

能力类 JSON 必须遵守 `CAPABILITY_TAXONOMY_SPEC.md` 的 key、version、status、level 和 metadata schema 规则。

## 6. 用户与权限

### 6.1 users

```txt
id
email
email_verified_at
name
password_hash
status
balance
currency
created_at
updated_at
deleted_at
```

索引：

```txt
unique(email)
index(status)
```

### 6.2 auth_sessions

控制台登录 Session 持久化表。

```txt
id
session_id_hash
csrf_token_hash
user_id
expires_at
last_active_at
ip
user_agent
status
created_at
updated_at
deleted_at
```

索引：

```txt
unique(session_id_hash)
index(user_id, status)
index(expires_at)
```

安全要求：

- 不存 Session ID 原文。
- 不存 CSRF token 原文。
- `session_id_hash` 和 `csrf_token_hash` 使用 SHA-256 派生值；登录响应只返回本次新建 Session 的明文 CSRF token。

### 6.3 roles

```txt
id
name
description
created_at
updated_at
```

索引：

```txt
unique(name)
```

### 6.4 user_roles

```txt
id
user_id
role_id
created_at
```

索引：

```txt
unique(user_id, role_id)
index(role_id)
```

### 6.5 user_groups

用户权益组用于订阅、价格倍率和用户侧权限分组，MVP 阶段不单独落库。
MVP 中 API Key 的 `group_ids` 指向 9.2 的 `account_groups`，用于选择可用账号池和调度偏好。

### 6.6 user_group_members

用户权益组成员关系随 `user_groups` 在订阅阶段引入，MVP 阶段不单独落库。

```txt
MVP no-op
```

### 6.7 reserved user entitlement group fields

```txt
id
user_id
user_group_id
created_at
```

索引：

```txt
unique(user_id, user_group_id)
```

## 7. API Key

### 7.1 api_keys

```txt
id
user_id
name
prefix
hash
status
scopes_json
allowed_models_json
rpm_limit
tpm_limit
concurrency_limit
expires_at
last_used_at
created_at
updated_at
deleted_at
```

索引：

```txt
unique(prefix)
index(user_id, status)
index(expires_at)
```

安全要求：

- 不存 API Key 原文。
- `hash` 使用安全哈希。
- `prefix` 用于定位，不用于认证。

### 7.2 api_key_groups

用于支持一个 API Key 绑定多个账号组，Gateway 运行时按请求模型和 platform 解析最终 account group。

```txt
id
api_key_id
account_group_id
created_at
```

索引：

```txt
unique(api_key_id, account_group_id)
index(account_group_id)
```

规则：

- 旧版 `api_keys.group_id` 如果存在，只作为单组兼容字段。
- 新接口应优先使用 `group_ids`。
- `/v1/models` 对多组 API Key 返回可见模型并集。
- 请求执行时的 group 解析规则以 `GATEWAY_ROUTE_MATRIX.md` 为准。

## 8. Provider 与模型

### 8.1 providers

```txt
id
name
display_name
adapter_type
status
capabilities_json
config_schema_json
created_at
updated_at
deleted_at
```

索引：

```txt
unique(name)
index(status)
```

### 8.2 model_registry

```txt
id
canonical_name
display_name
family
context_window
max_output_tokens
quality_tier
status
capabilities_json
created_at
updated_at
deleted_at
```

索引：

```txt
unique(canonical_name)
index(family)
index(status)
```

### 8.3 model_aliases

```txt
id
alias
model_id
strategy_hint
fallback_models_json
status
created_at
updated_at
```

索引：

```txt
unique(alias)
index(model_id)
```

### 8.4 model_provider_mappings

```txt
id
model_id
provider_id
upstream_model_name
status
capability_override_json
pricing_override_json
created_at
updated_at
```

索引：

```txt
unique(model_id, provider_id, upstream_model_name)
index(provider_id, status)
```

### 8.5 pricing_rules

```txt
id
model_id
provider_id
input_price_per_million
output_price_per_million
cache_read_price_per_million
cache_write_price_per_million
currency
effective_from
effective_to
created_at
updated_at
```

索引：

```txt
index(model_id, provider_id)
index(effective_from, effective_to)
```

### 8.6 capability_definitions

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

规则：

- 能力 key、分类、版本和废弃策略以 `CAPABILITY_TAXONOMY_SPEC.md` 为准。
- MVP 可以先以 seed 数据或代码常量实现，但数据库模型必须预留。

## 9. 账号池

### 9.1 provider_accounts

```txt
id
provider_id
name
account_type
credential_ciphertext
credential_version
proxy_id
status
priority
weight
risk_level
metadata_json
created_at
updated_at
deleted_at
```

索引：

```txt
index(provider_id, status)
index(status, priority)
```

### 9.2 account_groups

```txt
id
name
description
provider_scope_json
model_scope_json
strategy_hint
status
created_at
updated_at
```

索引：

```txt
unique(name)
index(status)
```

### 9.3 account_group_members

```txt
id
account_id
account_group_id
created_at
```

索引：

```txt
unique(account_id, account_group_id)
index(account_group_id)
```

### 9.4 proxies

```txt
id
name
type
url_ciphertext
url_version
status
metadata_json
created_at
updated_at
deleted_at
```

类型：

```txt
http
https
socks5
```

索引：

```txt
unique(name)
index(status)
index(type, status)
```

规则：

- `url_ciphertext` 存储加密后的代理 URL，API 响应只暴露 `url_configured`。
- `url_version` 记录代理 URL 密文字段的 key/aad 版本，便于后续密钥轮换。

## 10. 调度相关

### 10.1 scheduler_decisions

```txt
id
request_id
attempt_no
user_id
api_key_id
source_protocol
source_endpoint
target_protocol
model
strategy
strategy_version
fallback_from_decision_id
selected_provider_id
selected_account_id
candidate_count
rejected_count
scores_json
reject_reasons_json
strategy_weights_json
compatibility_warnings_json
sticky_hit
cache_affinity_hit
estimated_cost
currency
created_at
```

索引：

```txt
unique(request_id, attempt_no)
index(user_id, created_at)
index(api_key_id, created_at)
index(selected_account_id, created_at)
index(strategy, created_at)
```

规则：

- 同一 Gateway 请求如果发生 fallback，必须使用同一个 `request_id` 和递增 `attempt_no`。
- fallback attempt 必须通过 `fallback_from_decision_id` 指向上一条 `scheduler_decisions.id`，形成可审计链路。
- 每个 attempt 必须保留当时的 `strategy_version` 与 `strategy_weights_json`。
- 历史 decision 不得因策略权重变更而被重写。

### 10.2 scheduler_feedbacks

```txt
id
request_id
decision_id
attempt_no
account_id
provider_id
model
success
error_class
status_code
latency_ms
input_tokens
output_tokens
cached_tokens
actual_cost
currency
created_at
```

索引：

```txt
index(decision_id)
index(request_id, attempt_no)
index(account_id, created_at)
index(provider_id, created_at)
index(error_class, created_at)
```

### 10.3 scheduler_strategies

```txt
id
name
version
status
scope_type
scope_id
config_json
config_hash
description
created_by
created_at
updated_at
activated_at
deprecated_at
```

索引：

```txt
unique(name, version, scope_type, scope_id)
index(status, scope_type, scope_id)
index(name, status)
```

规则：

- 策略 descriptor、配置 schema、版本和灰度规则以 `SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 为准。
- `scheduler_decisions` 必须记录当次使用的策略版本、配置 hash 和权重快照。

### 10.4 scheduler_request_snapshots

用于保存每次真实 Scheduler attempt 的可回放证据。它和 `scheduler_decisions` 同事务写入，供后续历史 replay 使用。

```txt
id
request_id
attempt_no
decision_id
request_profile_json
candidate_snapshot_json
rejected_snapshot_json
ranked_account_ids_json
selected_account_id
selected_provider_id
strategy
strategy_version
strategy_config_hash
strategy_weights_json
compatibility_warnings_json
created_at
updated_at
```

索引：

```txt
unique(request_id, attempt_no)
unique(decision_id)
index(strategy, created_at)
index(selected_account_id, created_at)
```

规则：

- 每条 `scheduler_decisions` 写入必须同步写入一条 snapshot，避免出现不可回放的新决策。
- `request_profile_json` 只能保存调度必要字段、策略提示、模型/能力、价格估算和不可逆 affinity hash，不得保存原始 prompt、原始 session affinity key 或 rollout key。
- `candidate_snapshot_json` 只能保存调度重算需要的 provider/account/mapping/runtime/capability 字段，不得保存账号凭证、cookie、OAuth token、API key、密码或 credential ciphertext。
- 历史 replay 只能声称覆盖有 snapshot 的 decision；旧的 decision-only 行只能用于报表统计。

### 10.5 quality_eval_samples

用于在 `QUALITY_EVAL_ENABLED=true` 时保存可供 LLM-as-judge 评估的加密样本。样本来自已完成且成功写入 `scheduler_feedbacks` 的 Gateway 文本请求，保存的是经过 Gateway content-safety 处理后的规范化 prompt/output 摘要，不是原始请求体。

```txt
id
feedback_id
request_id
decision_id
attempt_no
account_id
provider_id
model
source_endpoint
sample_request_hash
sample_payload_ciphertext
payload_version
captured_at
created_at
updated_at
```

索引：

```txt
unique(feedback_id)
index(decision_id)
index(request_id, attempt_no)
index(account_id, model, captured_at)
index(sample_request_hash)
```

规则：

- `sample_payload_ciphertext` 必须使用 `SRAPI_MASTER_KEY` 派生密钥进行 AES-GCM 加密，当前 `payload_version = v1`。
- `sample_request_hash` 是基于请求侧脱敏文本、request_id、attempt_no 和 model 的不可逆 SHA-256，用于稳定 1% 抽样；它不得包含明文 prompt 或输出。
- 每条 feedback 最多创建一条 sample，重复捕获必须幂等返回既有记录。
- 当前捕获范围是文本型 Gateway 成功响应：OpenAI Chat Completions、OpenAI Responses、Anthropic Messages、Gemini GenerateContent。二进制/音频/图片/流式内部事件不进入本表。

### 10.6 quality_evaluations

保存 QualityEval worker 对样本的 LLM-as-judge 结果，并作为 Scheduler account+model 质量聚合的输入。

```txt
id
feedback_id
request_id
decision_id
attempt_no
account_id
provider_id
model
source_endpoint
sample_request_hash
judge_model
score
rubric_json
judged_at
created_at
updated_at
```

索引：

```txt
unique(feedback_id)
index(decision_id)
index(account_id, model, judged_at)
index(judge_model, judged_at)
index(sample_request_hash)
```

规则：

- `score` 归一化到 `0.0 - 1.0`；`rubric_json` 当前包含 `correctness`、`coherence`、`safety` 三项 0-5 分和短 rationale。
- worker 默认每小时从未评估 sample 中按 `sample_request_hash` 稳定抽样 1%，调用 `gpt-4o-mini` 兼容的 Chat Completions JSON mode 评判模型。
- Gateway 调度时按最近 30 天的 `(account_id, model)` 平均 `score` 注入候选 `quality_score`、`quality_eval_score`、`quality_tier` 和样本数，供 Scheduler score/Pareto 使用。
- 本表不保存明文 prompt、输出、账号凭证、API key、cookie 或 OAuth token。

### 10.7 sticky_sessions

```txt
id
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
created_at
updated_at
```

索引：

```txt
unique(binding_key)
index(account_id)
index(expires_at)
```

### 10.8 cache_affinity_records

```txt
id
provider_id
model
account_id
prompt_prefix_hash
cached_token_estimate
cache_write_time
last_hit_time
ttl_seconds
created_at
updated_at
```

索引：

```txt
unique(provider_id, model, account_id, prompt_prefix_hash)
index(prompt_prefix_hash)
index(last_hit_time)
```

### 10.9 account_health_snapshots

```txt
id
account_id
provider_id
status
success_rate
error_rate
latency_p50_ms
latency_p95_ms
rate_limit_count
timeout_count
cooldown_until
circuit_state
snapshot_at
```

索引：

```txt
index(account_id, snapshot_at)
index(provider_id, snapshot_at)
index(status)
```

### 10.10 account_quota_snapshots

```txt
id
account_id
provider_id
quota_type
remaining
used
quota_limit
remaining_ratio
reset_at
snapshot_at
```

索引：

```txt
index(account_id, quota_type, snapshot_at)
index(reset_at)
```

## 11. 用量与计费

### 11.1 usage_logs

```txt
id
request_id
attempt_no
user_id
api_key_id
provider_id
account_id
source_protocol
source_endpoint
target_protocol
model
input_tokens
output_tokens
cached_tokens
total_tokens
usage_estimated
latency_ms
success
error_class
cost
currency
compatibility_warnings_json
created_at
```

索引：

```txt
unique(request_id, attempt_no)
index(user_id, created_at)
index(api_key_id, created_at)
index(account_id, created_at)
index(source_endpoint, created_at)
index(model, created_at)
```

规则：

- 同一 Gateway 请求如果发生 fallback，必须为每次 provider attempt 记录一条 `usage_logs`。
- 所有 attempt 共用同一个 `request_id`，并递增 `attempt_no`，便于和 `scheduler_decisions` / `scheduler_feedbacks` 串联。

### 11.2 billing_ledger

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
metadata_json
created_at
```

索引：

```txt
index(user_id, created_at)
index(reference_type, reference_id)
```

规则：

- 账务流水只追加。
- 退款或修正使用反向记录。
- 用户余额更新和 ledger 写入必须在同一事务内。

## 12. 订阅

### 12.1 subscription_plans

```txt
id
name
description
price
currency
validity_days
entitlements_json
for_sale
sort_order
status
created_at
updated_at
deleted_at
```

索引：

```txt
index(for_sale, sort_order)
index(status)
```

### 12.2 user_subscriptions

```txt
id
user_id
plan_id
status
starts_at
expires_at
entitlements_snapshot_json
source_type
source_id
created_at
updated_at
```

索引：

```txt
index(user_id, status)
index(expires_at)
index(plan_id)
```

## 13. 支付

### 13.1 payment_provider_instances

```txt
id
provider
name
status
config_ciphertext
config_version
metadata_json
created_at
updated_at
deleted_at
```

### 13.2 payment_orders

```txt
id
user_id
order_no
provider_instance_id
amount
currency
status
product_type
product_id
provider_transaction_id
metadata_json
created_at
paid_at
closed_at
updated_at
```

索引：

```txt
unique(order_no)
index(user_id, created_at)
index(status, created_at)
index(provider_transaction_id)
```

### 13.3 payment_audit_logs

```txt
id
order_id
provider_instance_id
event_type
payload_json
signature_valid
created_at
```

索引：

```txt
index(order_id, created_at)
index(event_type, created_at)
```

### 13.4 invite_codes

邀请码表，受 `AFFILIATE_REBATE_SPEC.md` 约束。

```txt
id
user_id
code
status
expires_at
created_at
updated_at
```

索引：

```txt
unique(code)
index(user_id, status)
index(expires_at)
```

规则：

- 一个用户可以有多个邀请码。
- 公开入口只能接受 active 且未过期的邀请码。

### 13.5 invite_relationships

邀请绑定关系表。

```txt
id
inviter_user_id
invitee_user_id
invite_code_id
status
first_paid_at
created_at
updated_at
```

索引：

```txt
unique(invitee_user_id)
index(inviter_user_id, created_at)
index(invite_code_id)
index(status, created_at)
```

规则：

- 一个 invitee 只能绑定一个 inviter。
- 自邀请必须拒绝。
- 关系默认不可改绑，管理员调整必须写审计。

### 13.6 affiliate_rules

返利规则表。

```txt
id
name
status
trigger_type
rate
fixed_amount
currency
max_rebate_amount
valid_from
valid_to
metadata_json
created_at
updated_at
```

索引：

```txt
index(trigger_type, currency, status)
index(valid_from, valid_to)
```

规则：

- 支付成功返利使用 `trigger_type=payment_paid` 的 active 规则。
- 金额字段使用 fixed decimal string，不使用 float。

### 13.7 affiliate_ledgers

邀请返利账本，受 `AFFILIATE_REBATE_SPEC.md` 约束。

```txt
id
user_id
related_user_id
payment_order_id
subscription_id
type
amount
currency
status
reference_id
metadata_json
created_at
settled_at
```

索引：

```txt
unique(reference_id)
index(user_id, created_at)
index(related_user_id, created_at)
index(payment_order_id)
index(type, created_at)
```

规则：

- 只追加，不改写历史 accrual。
- 退款补偿必须使用反向 ledger。
- 转余额必须与 `billing_ledger` 和用户余额保持事务一致或通过可靠 outbox 保证最终一致。

## 14. 幂等

### 14.1 idempotency_records

```txt
id
idempotency_key
method
path
request_hash
status
response_snapshot_json
locked_until
expires_at
created_at
updated_at
```

索引：

```txt
unique(idempotency_key, method, path)
index(expires_at)
```

## 15. 审计

### 15.1 audit_logs

```txt
id
actor_user_id
action
resource_type
resource_id
before_json
after_json
ip
user_agent
trace_id
created_at
```

索引：

```txt
index(actor_user_id, created_at)
index(resource_type, resource_id)
index(action, created_at)
```

## 15A. 领域事件

### 15A.1 domain_events_outbox

```txt
id
event_id
event_type
event_version
producer_module
aggregate_type
aggregate_id
correlation_id
causation_id
idempotency_key
payload_json
metadata_json
status
attempt_count
next_retry_at
last_error
created_at
published_at
```

索引：

```txt
unique(event_id)
unique(producer_module, idempotency_key)
index(status, next_retry_at)
index(event_type, created_at)
index(aggregate_type, aggregate_id, created_at)
index(correlation_id)
```

### 15A.2 domain_events_inbox

```txt
id
event_id
consumer_name
event_type
status
attempt_count
last_error
processed_at
created_at
```

索引：

```txt
unique(event_id, consumer_name)
index(consumer_name, status, created_at)
```

规则：

- 事件 envelope、Outbox 状态、Inbox 幂等和死信处理以 `DOMAIN_EVENTS_SPEC.md` 为准。
- 事件 payload 不得包含明文 API Key、Provider 凭证、cookie、OAuth token 或原始 prompt。

## 15B. 运维观测

### 15B.1 obs_slo_definitions

```txt
id
name
sli_type
objective
window_days
status
filter_json
alert_policy_json
created_at
updated_at
```

索引：

```txt
unique(name)
index(status, sli_type)
```

规则：

- `objective` 按比例持久化，例如 `0.995` 表示 `99.5%`；管理 API 可接受 `99.5` 百分比输入并归一化。
- `filter_json` 只保存低基数字段，例如 `source_endpoint`、`model`、`provider_id` 和 `error_owner_exclude`。
- `alert_policy_json` 保存多窗口 burn-rate 阈值，不得包含通知凭证或 webhook secret。

### 15B.2 obs_alert_events

```txt
id
slo_id
rule_id
severity
status
fingerprint
summary
details_json
started_at
resolved_at
acknowledged_at
acknowledged_by
suppressed_by
created_at
updated_at
```

索引：

```txt
index(fingerprint, status)
index(rule_id, started_at)
index(severity, status)
index(slo_id, started_at)
```

规则：

- `details_json` 只能保存计算证据、低基数标签和聚合数值，不得保存 prompt、请求体、Authorization header、Cookie、API Key、OAuth token 或 Provider 凭证。
- ack 操作只更新 `status`、`acknowledged_at` 和 `acknowledged_by`；audit 中不得复制 `details_json`。

## 16. 系统配置

### 16.1 settings

```txt
id
key
value_json
value_ciphertext
is_secret
description
updated_by
created_at
updated_at
```

索引：

```txt
unique(key)
```

## 17. 数据冷热分层

高增长表：

```txt
usage_logs
scheduler_decisions
scheduler_request_snapshots
scheduler_feedbacks
quality_eval_samples
quality_evaluations
audit_logs
account_health_snapshots
account_quota_snapshots
domain_events_outbox
domain_events_inbox
obs_slo_definitions
obs_alert_events
```

建议：

- 第一阶段保留普通表。
- 后续按月分区。
- 报表使用聚合表。
- 原始日志可归档到对象存储。

## 18. 一致性边界

必须强一致：

- 用户余额和 billing ledger。
- 支付订单状态和订阅激活。
- API Key 创建和哈希保存。
- Provider Account 凭证保存。

最终一致：

- Usage 聚合。
- 调度反馈快照。
- 账号健康统计。
- 报表数据。
- 领域事件消费和跨模块补偿。

最终一致流程必须通过 `DOMAIN_EVENTS_SPEC.md` 的 Outbox / Inbox / 幂等机制实现。

## 19. 加密字段

必须加密：

```txt
provider_accounts.credential_ciphertext
provider_accounts.cookie_jar_ciphertext
provider_accounts.device_fingerprint_ciphertext
proxies.url_ciphertext
payment_provider_instances.config_ciphertext
settings.value_ciphertext when is_secret = true
quality_eval_samples.sample_payload_ciphertext
```

加密建议：

- AES-GCM。
- 主密钥来自环境变量或密钥管理系统。
- 记录 credential_version 以支持轮换。

## 20. 第一阶段最小表集合

MVP 最小可先实现：

```txt
users
api_keys
api_key_groups
providers
model_registry
capability_definitions
model_aliases
model_provider_mappings
pricing_rules
provider_accounts
account_groups
account_group_members
usage_logs
scheduler_decisions
scheduler_feedbacks
scheduler_request_snapshots
scheduler_strategies
quality_eval_samples
quality_evaluations
billing_ledger
account_health_snapshots
account_quota_snapshots
domain_events_outbox
domain_events_inbox
obs_slo_definitions
obs_alert_events
settings
audit_logs
idempotency_records
```

`sticky_sessions` 和 `cache_affinity_records` 在 MVP 中可以先使用 Redis-only 实现，但必须遵守 `SCHEDULER_V1_SPEC.md` 中的 TTL、重建和后续落库约束。

支付和订阅可在 Phase 3 引入。
