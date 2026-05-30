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

- 高吞吐风控事件。

AdminOps 系统日志已经提升为 `ops_system_logs` 一等表，不能再放回
`settings.value_json`。
公告内容仍由 `settings.value_json` 的 `admin_control.announcements` 承载；
用户侧阅读回执已经提升为 `user_announcement_reads` 一等表。
兑换码配置仍由 `settings.value_json` 的 `admin_control.redeem_codes` 承载；
用户侧核销回执已经提升为 `user_redeem_code_redemptions` 一等表。
优惠码配置仍由 `settings.value_json` 的 `admin_control.promo_codes` 承载；
用户侧下单应用回执已经提升为 `user_promo_code_applications` 一等表。
用户头像 v1 使用 per-user `settings.value_json` key
`users.avatar:v1:user:{user_id}` 承载低频、小体积、SRapi 重编码后的 PNG
对象和元数据（content type、byte size、sha256、width、height、updated_at）。
若头像成为高频查询、需要对象存储生命周期管理、CDN、公网匿名读取或独立约束，
必须提升为 objectstore-backed 一等 avatar 存储，而不是继续扩大 settings JSON。

能力类 JSON 必须遵守 `CAPABILITY_TAXONOMY_SPEC.md` 的 key、version、status、level 和 metadata schema 规则。

## 6. 用户与权限

### 6.1 workspaces

`workspaces` 是当前多租户边界的一等实体。第一阶段只建 Workspace，不引入 Organization/Team。

```txt
id
name
slug
owner_user_id nullable
type
status
metadata_json
created_at
updated_at
deleted_at
```

索引：

```txt
unique(slug)
index(owner_user_id, status)
index(status)
```

规则：

- 新用户通过持久化 User store 创建时，如果没有显式 `workspace_id`，会在同一事务中创建一个 `personal-<user_id>` slug 的个人 Workspace 并写回 `users.workspace_id`。
- `owner_user_id`、`users.workspace_id` 和 `api_keys.workspace_id` 当前为 nullable scalar id，保持迁移和导入数据的安全落地；公开 API 暂不暴露 Workspace 管理面。
- `type` 当前使用 `personal`，后续 Organization/Team 进入产品范围时再扩展。

### 6.2 users

```txt
id
email
email_verified_at
name
password_hash
status
workspace_id nullable
balance
currency
created_at
updated_at
deleted_at
```

索引：

```txt
unique(email)
index(workspace_id)
index(status)
```

规则：

- `email_verified_at` 只由 email verification flow 或可信外部身份绑定流程设置；
  普通 profile update、注册请求和密码找回不得直接写入该字段。

### 6.3 auth_sessions

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
- 过期 session 由后台 worker 从 `active` 标记为 `expired` 并设置 `deleted_at`；用户登出产生的 `revoked` 状态不被过期清理覆盖。

### 6.4 user_auth_identities

`user_auth_identities` 是当前用户外部登录身份目录。它承载未来 OAuth/OIDC
callback 完成后的绑定结果；本地 email 登录身份由 `users.email` 和
`users.email_verified_at` 派生，不在此表重复存储。
外部身份解绑按 `id + user_id` 精确删除一条记录，而不是按 provider 批量删除，
以便未来同一 provider 类型存在多个 issuer / tenant / app 实例时仍能安全管理。

```txt
id
user_id
provider
provider_key
provider_subject_hash
subject_hint
display_name
email
email_verified
avatar_url
verified_at
last_used_at
created_at
updated_at
deleted_at
```

索引：

```txt
unique(provider, provider_key, provider_subject_hash)
index(user_id, provider)
index(user_id)
index(last_used_at)
```

规则：

- `provider` 使用小而稳定的枚举：`oidc`、`github`、`google`、`linuxdo`、
  `wechat`、`dingtalk`。`email` 是派生身份，不写入本表。
- `provider_key` 是 provider instance key，例如 OIDC issuer 或内置 provider key。
- `provider_subject_hash` 保存归一化外部 subject 的不可逆 hash，不保存原始
  upstream subject、access token、refresh token、session cookie 或 provider secret。
- `subject_hint` 只用于 UI 显示，必须是不可用于登录或 API 调用的安全摘要。
- `avatar_url` 只表示可信外部身份建议的资料 URL；SRapi-hosted 当前用户头像仍由
  avatar storage flow 管理。

### 6.5 user_totp_secrets

当前用户 TOTP 二次验证密钥表。

```txt
id
user_id
secret_ciphertext
secret_version
enabled
recovery_code_hashes_json
last_used_at
created_at
updated_at
```

索引：

```txt
unique(user_id)
index(enabled, user_id)
```

规则：

- `secret_ciphertext` 使用 `TOTP_ENCRYPTION_KEY` 派生 AES-GCM key 加密。
- `recovery_code_hashes_json` 只保存恢复码 HMAC hash，不保存明文。
- `enabled=false` 表示 setup 尚未完成；disable 删除该行，避免保留可重新启用的旧 secret。

### 6.6 pending_oauth_sessions

`pending_oauth_sessions` 是 OAuth/OIDC callback 完成后、用户最终选择登录/
绑定/建号动作之前的短期决策会话。它只保存服务端 HMAC hash 和安全摘要，
不保存 authorization code、access token、refresh token、raw upstream subject、
state、nonce、PKCE verifier 或 provider secret。

```txt
id
session_token_hash
intent
provider
provider_key
provider_subject_hash
subject_hint
target_user_id nullable
redirect_to
resolved_email
display_name
email_verified
avatar_url
expires_at
consumed_at nullable
created_at
updated_at
```

索引：

```txt
unique(session_token_hash)
index(target_user_id)
index(expires_at)
index(consumed_at)
index(provider, provider_key, provider_subject_hash)
```

规则：

- `session_token_hash` 必须由服务端密钥 HMAC 派生；明文 pending token 只返回给
  本次浏览器流程。
- `provider_subject_hash` 是上游 subject 的服务端派生 hash，不能保存 raw subject。
- `consumed_at` 一旦写入表示 pending session 已完成；消费必须通过
  `consumed_at IS NULL` 和 `expires_at > now` 原子更新保证单次使用。
- `redirect_to` 只保存站内路径；空值或跨站路径归一为 `/`。

### 6.7 password_reset_tokens

公开密码找回 token 的持久 receipt。

```txt
id
user_id
token_hash
token_version
expires_at
used_at nullable
created_at
updated_at
```

索引：

```txt
unique(token_hash)
index(user_id, created_at)
index(expires_at)
index(used_at)
```

### 6.8 email_verification_tokens

公开邮箱验证 token 的持久 receipt。

```txt
id
user_id
token_hash
token_version
expires_at
used_at nullable
created_at
updated_at
```

索引：

```txt
unique(token_hash)
index(user_id, created_at)
index(expires_at)
index(used_at)
```

规则：

- `token_hash` 是由服务端密钥计算的 HMAC / keyed hash，不保存明文 verification token。
- `used_at` 一旦写入表示 token 已消费；confirm 必须通过 `used_at IS NULL` 和 `expires_at > now`
  原子更新实现单次消费。
- 邮件投递使用 `domain_events_outbox`，payload 只能包含加密 token 和邮箱 hash。

### 6.4B user_announcement_reads

当前用户公告阅读回执表。公告内容仍来自 Admin Control Plane 的
`admin_control.announcements` typed collection；该表只保存 per-user receipt，
避免把用户高频状态写回全局 settings JSON。

```txt
id
user_id
announcement_id
read_at
created_at
updated_at
```

索引：

```txt
unique(user_id, announcement_id)
index(announcement_id)
index(user_id, read_at)
```

规则：

- 同一用户同一公告最多一条回执。
- 如果公告内容被管理员更新且 `updated_at > read_at`，用户侧列表重新视为未读。
- 该表不保存公告正文、用户邮箱、角色快照或投递 payload。

### 6.4C user_redeem_code_redemptions

当前用户兑换码核销回执表。兑换码配置仍来自 Admin Control Plane 的
`admin_control.redeem_codes` typed collection；该表保存每次成功核销的
per-user receipt、财务/订阅引用和幂等证据，避免把用户高频状态只写回全局
settings JSON。

```txt
id
user_id
redeem_code_id
code_digest
type
amount
currency
balance_before
balance_after
billing_ledger_id
user_subscription_id
redeemed_at
metadata_json
created_at
updated_at
```

索引：

```txt
unique(user_id, redeem_code_id)
index(redeem_code_id)
index(user_id, redeemed_at)
index(code_digest)
```

规则：

- 同一用户同一兑换码最多一条核销回执；重复提交返回既有结果，不重复入账。
- `balance` 兑换必须同事务更新 `users.balance` 并写
  `billing_ledger(type=redeem_code_credit)`。
- `subscription` 兑换必须同事务创建 `user_subscriptions` 和对应
  `entitlements` 缓存行，来源为 `source_type=redeem_code`。
- 该表不保存兑换码明文；只保存规范化 code 的 digest、redeem code id 和低敏
  metadata。

### 6.4D user_promo_code_applications

当前用户支付订单优惠码应用回执表。优惠码配置仍来自 Admin Control Plane 的
`admin_control.promo_codes` typed collection；该表保存每笔成功下单优惠的
per-order receipt、折扣金额和幂等证据，避免把用户侧使用历史只写回全局
settings JSON。

```txt
id
user_id
promo_code_id
code_digest
payment_order_id
order_no
original_amount
discount_amount
final_amount
currency
discount_type
applied_at
metadata_json
created_at
updated_at
```

索引：

```txt
unique(payment_order_id)
unique(order_no)
index(promo_code_id)
index(user_id, applied_at)
index(code_digest)
```

规则：

- 同一支付订单最多一条优惠码应用回执；已提交订单的重复 finalize 不重复递增
  `used_count`。
- 优惠码在创建支付订单前按 active/starts_at/expires_at/max_uses、币种、金额折扣
  或比例折扣规则校验。
- 持久化支付订单时，同一事务写 `payment_orders` 折扣字段、创建应用回执并递增
  `admin_control.promo_codes` 中对应 `used_count`。
- 该表不保存优惠码明文；只保存规范化 code 的 digest、promo code id 和低敏
  metadata。

### 6.4 roles

```txt
id
name
description
permissions_json
created_at
updated_at
```

索引：

```txt
unique(name)
```

规则：

- `permissions_json` 是 `["resource:action"]` 字符串数组，例如 `["payment_order:read"]`。
- `owner` 和 `admin` 是控制台超级管理角色；自定义角色通过 Admin Roles API 创建后才能分配给用户。
- 运行时会把用户所有角色的 `permissions_json` 去重合并到登录会话用户 DTO，用于细粒度 Admin API 门禁。

### 6.5 user_roles

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

### 6.6 user_groups

用户权益组用于订阅、价格倍率和用户侧权限分组，MVP 阶段不单独落库。
MVP 中 API Key 的 `group_ids` 指向 9.2 的 `account_groups`，用于选择可用账号池和调度偏好。

### 6.7 user_group_members

用户权益组成员关系随 `user_groups` 在订阅阶段引入，MVP 阶段不单独落库。

```txt
MVP no-op
```

### 6.8 reserved user entitlement group fields

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
workspace_id nullable
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
index(workspace_id, status)
index(expires_at)
```

安全要求：

- 不存 API Key 原文。
- `hash` 使用安全哈希。
- `prefix` 用于定位，不用于认证。
- 持久化创建 API Key 时如果未指定 `workspace_id`，会继承 owner user 的 `workspace_id`；显式指定时用于后续多租户管理面。

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
selection_rationale
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
- `selection_rationale` 只保存非敏感解释文本，可引用账号 / Provider ID、分数和拒绝原因，不得保存 prompt、凭证、cookie、原始 affinity key 或上游响应正文。
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
charged_at nullable
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
index(charged_at, success, created_at)
```

规则：

- 同一 Gateway 请求如果发生 fallback，必须为每次 provider attempt 记录一条 `usage_logs`。
- 所有 attempt 共用同一个 `request_id`，并递增 `attempt_no`，便于和 `scheduler_decisions` / `scheduler_feedbacks` 串联。
- `cost` 和 `currency` 是 balance charger 的扣费输入；成功且 `charged_at IS NULL` 的记录会被后台 worker 按 `created_at` 顺序批量转成 `billing_ledger.usage_charge`。
- `index(charged_at, success, created_at)` 服务未扣费 usage 扫描，避免在高吞吐 Gateway 日志中对已扣费记录做全表过滤。

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

`entitlements_snapshot_json` 是订阅创建时从套餐复制出的不可变权益快照，用于审计和防漂移。Gateway 的实时权益判定读取 `entitlements` 查询缓存，不直接扫描订阅 snapshot。

### 12.3 entitlements

`entitlements` 是从 active user subscription materialize 出来的查询缓存层。它保留一行一个 feature，服务层按 active subscription 状态、开始时间和过期时间过滤后合并。

```txt
id
user_id
scope_type
scope_id
feature_key
value_json
quota_limit nullable
expires_at
source_subscription_id
created_at
updated_at
```

索引：

```txt
index(user_id, feature_key, expires_at)
unique(source_subscription_id, feature_key)
index(scope_type, scope_id, feature_key)
```

规则：

- 当前 `scope_type=user`，`scope_id=user_id`；后续 Workspace/Organization entitlement 可沿用同一表扩展。
- `value_json` 以 `{"value": <snapshot value>}` 保存原始 feature 值，保证列表、字符串、数值都能无损参与 Gateway entitlement 判定。
- `quota_limit` 对 `monthly_token_quota`、`monthly_cost_quota` 保存字符串化查询列；其他 feature 可为空。
- 创建 active user subscription 时在同一持久化事务内写入 entitlement rows；过期判定依赖 `expires_at` 和来源 subscription 的 active window。

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
original_amount
discount_amount
promo_code_id nullable
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
index(provider_instance_id, created_at)
index(promo_code_id)
index(expires_at)
```

规则：

- `amount` 是实际提交给支付渠道的最终应付金额。
- `original_amount` 是用户提交的原始订单金额；未使用优惠码时等于 `amount`。
- `discount_amount` 是服务端计算后的优惠金额；未使用优惠码时为 `0.00000000`。
- `promo_code_id` 指向 settings-backed promo code 的稳定 id，不保存优惠码明文。

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
- Auth transactional email 事件只能携带加密 reset/verification token、recipient user id、
  recipient email hash、模板名、相对 action path 和 expiry。Outbox worker 投递前必须用当前
  用户邮箱 hash 校验事件未过期于邮箱变更；SMTP password 只来自部署环境，不进入 outbox、
  settings JSON 或 audit snapshot。
- Optional notification unsubscribe preferences may be settings-backed while
  they are low-volume and event-scoped. Keys and values must use event names and
  email hashes only; plaintext recipient emails and SMTP secrets are forbidden.
  If preferences become a user-facing high-volume collection, promote them to a
  dedicated Ent schema with unique `(event, email_hash)` constraints.

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

### 15B.3 ops_system_logs

AdminOps 结构化系统日志表，只保存 SRapi 已脱敏的运维事件，不读取或镜像
本机 stdout/stderr 文件。

```txt
id
level
source
message
request_id
trace_id
metadata_json
created_at
updated_at
```

索引：

```txt
index(created_at)
index(level, created_at)
index(source, created_at)
index(request_id)
index(trace_id)
```

规则：

- `level` 使用 `debug`、`info`、`warn`、`error`。
- `source` 是稳定低基数来源，例如 `ops.dashboard` 或 `ops.worker`。
- `message` 必须是安全摘要，不得保存 prompt、请求体、Authorization header、Cookie、API Key、OAuth token 或 Provider 凭证。
- `metadata_json` 只保存低敏诊断字段；高基数明细应通过 `request_id` / `trace_id` 跳转到对应证据链。
- Admin cleanup 必须要求至少一个过滤条件，支持 dry-run，限制 `max_delete`，并写入不包含原始搜索字符串或日志正文的 audit 摘要。

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
ops_system_logs
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
user_totp_secrets.secret_ciphertext
```

加密建议：

- AES-GCM。
- 主密钥来自环境变量或密钥管理系统。
- 记录 credential_version 以支持轮换。

## 20. 第一阶段最小表集合

MVP 最小可先实现：

```txt
users
workspaces
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
ops_system_logs
user_announcement_reads
user_promo_code_applications
user_redeem_code_redemptions
user_totp_secrets
settings
audit_logs
idempotency_records
```

`sticky_sessions` 和 `cache_affinity_records` 在 MVP 中可以先使用 Redis-only 实现，但必须遵守 `SCHEDULER_V1_SPEC.md` 中的 TTL、重建和后续落库约束。

支付和订阅可在 Phase 3 引入。
