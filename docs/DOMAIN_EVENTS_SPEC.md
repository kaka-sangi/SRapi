# SRapi 领域事件与最终一致性规范

## 1. 目标

本文档定义 SRapi 内部领域事件、Outbox、事件消费、幂等和补偿规则，用于降低模块之间的同步耦合。

目标：

- 支付、计费、返利、观测、通知等跨模块流程最终一致。
- 请求主链路保持短路径和可解释。
- 事件可以重试、回放和审计。
- 未来可从进程内事件演进到消息队列或独立服务。

## 2. 使用原则

应使用事件的场景：

- 一个模块状态变更后，多个模块需要后续响应。
- 后续动作可重试、可补偿、允许最终一致。
- 后续动作失败不应回滚主业务状态。
- 需要审计、报表、告警或通知。

不应使用事件的场景：

- 鉴权和权限判断。
- Gateway 请求实时调度。
- Lease 获取和释放。
- 同一事务内必须立即一致的账务核心写入。

## 3. 事件命名

事件命名使用过去式业务事实：

```txt
PaymentOrderPaid
PaymentOrderRefunded
UsageCommitted
BillingLedgerCreated
SubscriptionActivated
AffiliateRebateAccrued
AffiliateRebateCompensated
ProviderAccountHealthChanged
ProviderAccountQuotaChanged
AccountQuotaAlertTriggered
NotificationContactVerificationRequested
SchedulerDecisionRecorded
GatewayRequestCompleted
AlertTriggered
```

禁止使用命令式名称：

```txt
PayOrder
CreateRebate
SendEmail
UpdateDashboard
```

## 4. 事件 Envelope

所有事件必须包装为统一 envelope：

```txt
event_id
event_type
event_version
occurred_at
producer_module
aggregate_type
aggregate_id
correlation_id
causation_id
idempotency_key
trace_id
actor_type
actor_id
payload_json
metadata_json
```

字段说明：

- `event_id` 全局唯一。
- `event_type` 使用稳定业务事实名称。
- `event_version` 从 `v1` 开始。
- `correlation_id` 贯穿一次业务流程。
- `causation_id` 指向触发当前事件的命令或事件。
- `idempotency_key` 用于消费者去重。
- `payload_json` 不得包含敏感明文。

## 5. Outbox 表

建议新增 `domain_events_outbox`：

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

状态：

```txt
pending
published
failed
```

MVP 状态机：

```txt
pending --dispatch ok--> published
pending --dispatch failed--> failed(next_retry_at, last_error, attempt_count + 1)
failed(next_retry_at <= now) --dispatch ok--> published
failed(next_retry_at <= now) --dispatch failed--> failed(next_retry_at, last_error, attempt_count + 1)
```

多 worker 或外部消息中间件阶段可增加：

```txt
publishing
dead_lettered
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

## 6. Inbox / Consumer 去重

建议新增 `domain_events_inbox`：

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

每个消费者必须先写 inbox 再处理；处理成功标记 `processed` 和 `processed_at`，处理失败标记 `failed`、递增 `attempt_count` 并写入 `last_error`。重复事件如果已 `processed` 必须直接跳过。

## 7. 发布模式

MVP 可采用数据库 Outbox + Worker 轮询：

```txt
module local transaction
  ↓
write business state
  ↓
write domain_events_outbox
  ↓
commit
  ↓
worker publishes / dispatches event
  ↓
consumer handles with inbox idempotency
```

后续可替换为：

```txt
PostgreSQL outbox -> Redis Stream / NATS / Kafka
```

替换消息中间件不得改变事件 envelope 和 consumer 幂等语义。

## 8. 核心事件目录

### 8.1 GatewayRequestCompleted

生产者：Gateway。

触发：上游请求完成或最终失败。

关键 payload：

```txt
request_id
user_id
api_key_id
source_protocol
model
provider_id
account_id
success
error_class
status_code
latency_ms
input_tokens
output_tokens
cached_tokens
estimated_cost
actual_cost
currency
```

消费者：

- Usage。
- Billing。
- Scheduler Feedback。
- Observability。

主请求不得等待所有消费者完成。

### 8.2 UsageCommitted

生产者：Usage / Billing。

触发：用量事实写入完成。

关键 payload：

```txt
usage_log_id
request_id
user_id
api_key_id
provider_id
account_id
model
input_tokens
output_tokens
cached_tokens
cost
currency
```

消费者：

- Billing ledger。
- Observability。
- Cost report。

### 8.3 BillingLedgerCreated

生产者：Billing。

触发：账务流水提交。

关键 payload：

```txt
ledger_id
user_id
type
amount
currency
balance_after
source_type
source_id
```

消费者：

- Observability。
- Audit。
- Notification，可选。

### 8.4 BalanceLowTriggered

生产者：Billing balance charger。

触发：成功扣费后，用户余额从大于等于低余额提醒阈值降到阈值以下。

关键 payload：

```txt
recipient_user_id
recipient_email_hash
balance_before
balance_after
threshold
currency
ledger_entry_id
usage_log_ids
charged_at
recharge_url
```

安全规则：

- payload 不得包含明文邮箱、unsubscribe token、session cookie、CSRF token、SMTP secret、API key、provider credential 或 prompt。
- 事件通过 `billing:balance_low:<user_id>:<ledger_reference>` 幂等键去重。
- Outbox 通知消费者必须重新读取当前用户并校验邮箱 hash；邮箱变更、用户停用或用户已退订 `balance.low` 时跳过投递。
- 邮件投递失败保持 outbox retry，而不回滚扣费结果。

消费者：

- Notification。

### 8.5 SubscriptionExpiryReminderTriggered

生产者：Subscriptions expiry worker。

触发：活跃订阅距离过期还有 7、3 或 1 天时，由订阅过期 worker 扫描并入队。

关键 payload：

```txt
subscription_id
recipient_user_id
plan_id
subscription_name
days_remaining
reminder_key
expires_at
triggered_at
subscription_url
```

安全规则：

- payload 不得包含明文邮箱、recipient email hash、unsubscribe token、session cookie、CSRF token、SMTP secret、API key、provider credential 或 prompt。
- 事件通过 `subscriptions:subscription_expiry_reminder:<subscription_id>:<reminder_key>` 幂等键去重。
- Outbox 通知消费者必须重新读取当前用户；用户停用或用户已退订 `subscription.expiry_reminder` 时跳过投递。
- 邮件投递失败保持 outbox retry，而不改变订阅状态。

消费者：

- Notification。

### 8.6 NotificationContactVerificationRequested

生产者：Notifications current-user contact API。

触发：当前用户添加或重新请求验证额外通知邮箱。

关键 payload：

```txt
recipient_user_id
recipient_email_hash
contact_id
contact_email_ciphertext
verification_token_ciphertext
verification_token_version
verification_url_path
expires_at
```

安全规则：

- payload 不得包含明文 contact email、unsubscribe token、session cookie、CSRF token、SMTP secret、API key、provider credential 或 prompt。
- contact email 和 verification token 只能以 deployment master key 派生密钥加密后进入 outbox。
- 事件通过 `notifications.contact_verification:<user_id>:<contact_id>:<token_hash_prefix>` 幂等键去重。
- Outbox 通知消费者必须重新读取当前用户，跳过 inactive user，并校验解密后的 contact email hash 与 payload hash 一致。
- contact verification mail 是 transactional mail，不受 optional unsubscribe preference 压制，也不携带 one-click unsubscribe headers。

消费者：

- Notification。

### 8.7 PaymentOrderPaid

生产者：Payments。

触发：支付订单从待支付进入已支付。

关键 payload：

```txt
order_id
order_no
user_id
provider
provider_instance_id
amount
currency
paid_at
provider_transaction_id
```

消费者：

- Billing credit。
- Subscription activation。
- Affiliate rebate accrual。
- Audit。

支付模块只负责支付事实，不直接写返利账本。

### 8.8 PaymentOrderRefunded

生产者：Payments。

触发：退款成功。

关键 payload：

```txt
order_id
refund_id
user_id
amount
currency
refund_reason
refunded_at
```

消费者：

- Billing refund debit。
- Affiliate rebate compensation。
- Audit。

### 8.9 SubscriptionActivated

生产者：Subscriptions。

触发：套餐权益激活。

关键 payload：

```txt
subscription_id
user_id
plan_id
source_order_id
started_at
expires_at
entitlement_snapshot
```

消费者：

- Audit。
- Observability。

### 8.10 AffiliateRebateAccrued

生产者：Affiliate。

触发：返利入账。

关键 payload：

```txt
affiliate_ledger_id
inviter_user_id
invitee_user_id
source_order_id
rebate_amount
currency
rule_id
status
```

消费者：

- Billing，可选转余额。
- Audit。
- Observability。

### 8.11 AffiliateRebateCompensated

生产者：Affiliate。

触发：因退款或风控发生返利冲正。

关键 payload：

```txt
compensation_ledger_id
original_ledger_id
source_refund_id
amount
currency
reason
```

消费者：

- Billing。
- Audit。

### 8.12 ProviderAccountHealthChanged

生产者：Scheduler / Provider Adapter / Account Health Worker。

触发：账号健康状态发生显著变化。

关键 payload：

```txt
provider_id
account_id
old_health_state
new_health_state
error_class
cooldown_until
reason
```

消费者：

- Observability。
- Alerting。
- Audit，高风险变更。

### 8.13 SchedulerDecisionRecorded

生产者：Scheduler。

触发：调度决策持久化。

关键 payload：

```txt
decision_id
request_id
attempt_no
strategy
strategy_version
selected_provider_id
selected_account_id
candidate_count
rejected_count
```

消费者：

- Observability。
- Ops Dashboard。
- Offline analysis。

## 9. 事件版本规则

事件版本使用：

```txt
PaymentOrderPaid.v1
PaymentOrderPaid.v2
```

兼容规则：

- 新增 optional 字段：不升主版本。
- 修改字段语义：必须新版本。
- 删除字段：必须新版本。
- payload 中字段重命名：必须新版本。

消费者必须声明支持的事件版本。

## 10. 幂等规则

生产者：

- 同一业务事实只能产生一个 `idempotency_key`。
- 支付 webhook 产生事件时必须绑定 webhook idempotency record。
- 账务事件必须绑定 ledger id。

消费者：

- 每个 event + consumer 只处理一次。
- 重试不得重复扣款、重复返利或重复通知。
- 外部副作用必须使用外部幂等键。

## 11. 重试与死信

重试策略：

```txt
1m
5m
15m
1h
6h
24h
```

进入死信条件：

- 超过最大重试次数。
- payload schema 无法解析。
- 业务对象永久不存在。
- 消费者明确返回 non_retryable error。

死信必须可在 Admin Ops 中查看和人工重放。

## 12. 补偿规则

跨模块流程失败不得篡改原事件。

补偿应产生新事件，例如：

```txt
PaymentOrderRefunded -> AffiliateRebateCompensated
BillingLedgerCreated(type=compensation)
```

禁止：

- 删除旧 ledger。
- 修改已发布事件 payload。
- 通过手动 SQL 绕过审计。

## 13. 安全与隐私

事件 payload 禁止包含：

- 明文 API Key。
- Provider credential。
- cookie / OAuth token。
- 原始 prompt 和 completion。
- 支付渠道完整回调原文中的敏感字段。

可存储：

- 哈希。
- prefix。
- provider_transaction_id。
- 脱敏错误摘要。
- 聚合 token 和 cost 数据。

## 14. 测试要求

每个事件必须覆盖：

- schema 校验。
- producer outbox 写入。
- consumer 幂等。
- retryable error 重试。
- non_retryable error 死信。
- 敏感字段扫描。
- 版本兼容测试。

关键商业事件必须有端到端测试：

```txt
PaymentOrderPaid -> Billing credit -> Affiliate rebate -> Audit
PaymentOrderRefunded -> Billing debit -> Affiliate compensation -> Audit
GatewayRequestCompleted -> Usage -> Billing -> Scheduler Feedback
```
