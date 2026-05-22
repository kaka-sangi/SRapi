# SRapi 模块接口契约规范

## 1. 目标

本文档定义 SRapi 模块之间的内部接口契约，防止模块化单体在实现阶段退化为跨模块互相调用 repository、handler 或具体实现。

目标：

- 跨模块调用只能依赖稳定 contract。
- 模块内部实现可以重构，外部调用方不受影响。
- HTTP、数据库、Provider SDK、Ent schema 不泄漏进领域服务边界。
- 同步调用和异步事件有明确选择标准。
- 为未来拆分服务保留接口形态。

## 2. Contract 分层

模块内部建议分为：

```txt
modules/{module}/
├── domain/        领域对象、值对象、领域规则
├── service/       应用服务实现
├── contract/      允许其他模块依赖的接口与 DTO
├── repository/    模块私有 repository interface
├── handler/       HTTP handler adapter
├── policy/        模块私有策略
└── errors/        typed errors
```

`contract` 是跨模块依赖的唯一入口。

## 3. 依赖规则

允许：

```txt
module A service -> module B contract interface
module handler -> same module service
module service -> same module repository interface
module service -> platform service interface
```

禁止：

```txt
module A -> module B repository
module A -> module B handler
module A -> module B concrete service implementation
module A -> module B Ent schema query
module A -> module B private policy
module A -> module B database transaction internals
```

跨模块调用不得传递：

- Ent entity。
- HTTP request / response。
- SQL row / transaction。
- Provider SDK 原始对象。
- 未脱敏凭证。

## 4. Contract 类型

### 4.1 Query Contract

用于读取其他模块状态，不产生业务状态变更。

示例：

```txt
ApiKeyReader.ValidateGatewayKey
ModelCatalog.ResolveVisibleModel
EntitlementReader.GetUserEntitlement
AccountSnapshotReader.ListSchedulableAccounts
```

要求：

- 返回只读 DTO。
- 不暴露 repository 查询条件细节。
- 可以被缓存，但必须声明一致性要求。

### 4.2 Command Contract

用于请求其他模块执行状态变更。

示例：

```txt
BillingCommand.CommitUsageCharge
SubscriptionCommand.ActivatePlan
PaymentCommand.MarkOrderPaid
AffiliateCommand.CompensateRefundedRebate
```

要求：

- 必须声明幂等键。
- 必须返回业务结果或 typed error。
- 不得要求调用方持有被调用模块的数据库事务。

### 4.3 Event Contract

用于最终一致的跨模块协作。

示例：

```txt
PaymentOrderPaid
UsageCommitted
ProviderAccountHealthChanged
AffiliateRebateAccrued
```

事件规范以 `DOMAIN_EVENTS_SPEC.md` 为准。

### 4.4 Policy Contract

用于跨模块查询可执行权限或策略结论。

示例：

```txt
ModelAccessPolicy.CanUseModel
AccountRoutingPolicy.CanRouteToGroup
SecurityPolicy.CanAccessAdminResource
```

Policy Contract 只能返回判断结果和原因，不得执行写操作。

## 5. 同步调用选择标准

可以同步调用：

- 请求路径上必须立即得到结论的鉴权、权限、模型解析和账号候选构建。
- 单一业务一致性边界内的状态变更。
- 需要立即返回给用户的命令结果。

不应同步调用：

- 报表聚合。
- 长期观测数据写入。
- 支付后返利计算。
- 告警生成。
- 邮件、Webhook、通知。
- 可重试补偿任务。

这些应使用 `DOMAIN_EVENTS_SPEC.md` 的事件机制。

## 6. 核心模块 Contract 清单

### 6.1 Auth

对外 contract：

```txt
SessionReader.GetCurrentSession
AdminAuthPolicy.RequireAdminRole
```

不得对外暴露 password hash、refresh token 原文或 session 存储细节。

### 6.2 API Keys

对外 contract：

```txt
GatewayKeyAuthenticator.Authenticate
ApiKeyPolicyReader.GetGatewayPolicy
ApiKeyGroupReader.ListBoundGroups
```

Gateway 只能通过这些 contract 鉴权，不得直接查询 `api_keys` 表。

### 6.3 Models

对外 contract：

```txt
ModelCatalog.ResolveModel
ModelCatalog.ListVisibleModels
ModelCapabilityReader.GetModelCapabilities
ModelMappingReader.ResolveProviderMapping
```

能力字段命名和版本以 `CAPABILITY_TAXONOMY_SPEC.md` 为准。

### 6.4 Providers

对外 contract：

```txt
ProviderRegistry.GetProvider
ProviderAdapterRegistry.GetAdapter
ProviderPresetReader.GetPreset
ProviderCapabilityReader.GetProviderCapabilities
```

Provider Adapter 只能通过 registry 获取，不得在 Gateway 或 Scheduler 中硬编码具体实现。

### 6.5 Accounts

对外 contract：

```txt
AccountSnapshotReader.ListSchedulableAccounts
AccountCredentialMaterializer.MaterializeForAdapter
AccountHealthWriter.ReportHealthChange
AccountQuotaReader.GetQuotaSnapshot
```

Scheduler 只读取 account runtime snapshot，不解密凭证。

### 6.6 Gateway

对外 contract：

```txt
GatewayRequestNormalizer.Normalize
GatewayResponseRenderer.Render
GatewayUsageReporter.ReportUsageObserved
```

Gateway 不暴露 handler，不对其他模块提供路由级 API。

### 6.7 Scheduler

对外 contract：

```txt
Scheduler.Schedule
Scheduler.RecordFeedback
SchedulerSimulator.Simulate
StrategyRegistry.GetStrategy
```

策略扩展以 `SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 为准。

### 6.8 Realtime

对外 contract：

```txt
RealtimeSlotManager.Acquire
RealtimeSlotManager.Release
RealtimeSlotManager.Snapshot
```

Realtime 模块只管理长连接 slot 生命周期、限额和匿名化 sticky/session 证据。它不得选择 Provider Account，不得解密 Provider credential，不得包含 Codex / Claude Code / Antigravity 等 provider-specific Gateway DTO。

### 6.9 Billing

对外 contract：

```txt
EntitlementReader.GetUserEntitlement
BalanceReader.GetSpendableBalance
BillingCommand.CommitUsageCharge
BillingCommand.ApplyPaymentCredit
BillingCommand.ApplyRefundDebit
```

Billing 可以提供用户余额、套餐权益和成本策略给 Scheduler，但不得参与账号选择。

### 6.10 Payments

对外 contract：

```txt
PaymentOrderCommand.CreateOrder
PaymentOrderCommand.CancelOrder
PaymentRefundCommand.RequestRefund
PaymentWebhookCommand.HandleWebhook
```

支付完成、退款完成等跨模块后续动作应通过领域事件驱动。

### 6.11 Affiliate

对外 contract：

```txt
AffiliateRuleReader.GetEffectiveRule
AffiliateCommand.AccrueRebate
AffiliateCommand.CompensateRefund
AffiliateBalanceCommand.TransferToBalance
```

Affiliate 不得直接修改 Payment Order 状态。

### 6.12 Observability

对外 contract：

```txt
MetricRecorder.RecordGatewayMetric
TraceAnnotator.AnnotateDecision
OpsQueryReader.GetProviderHealthMatrix
AlertCommand.AcknowledgeAlert
```

Observability 不承载业务真实状态，不得成为 Gateway、Scheduler、Billing 的强一致依赖。

## 7. DTO 规则

跨模块 DTO 必须：

- 使用稳定字段名。
- 字段只增不破坏。
- 明确 nullable / optional 语义。
- 金额使用 decimal 或 minor unit，不使用 float。
- 时间统一 `timestamptz` / RFC3339。
- 敏感字段只传递引用 ID 或密文句柄。

DTO 禁止包含：

- 明文 API Key。
- 明文 Provider credential。
- cookie / OAuth token 原文。
- 原始 prompt 内容。
- 上游完整错误 body 中可能包含的敏感内容。

## 8. 事务边界

跨模块 contract 不得要求调用方共享数据库事务。

如果必须保持一致性：

1. 在拥有状态的模块内完成本地事务。
2. 写入 outbox event。
3. 由事件 handler 执行后续模块动作。
4. 使用幂等键和补偿任务保证最终一致。

## 9. 错误契约

Contract 返回 typed error，必须包含：

```txt
code
message
retryable
reason
safe_details
```

禁止返回：

- 数据库原始错误。
- Provider SDK 原始错误。
- 包含凭证或 prompt 的 details。

HTTP 层负责把 typed error 渲染为 `OPENAPI_CONTRACT.md` 的错误格式。

## 10. 版本与兼容

Contract 变更规则：

- 新增字段：允许。
- 新增 error code：允许，但必须同步文档和测试。
- 删除字段：禁止，必须先 deprecate。
- 修改字段语义：禁止，必须新增字段。
- 修改方法签名：需要新 contract 版本。

版本命名：

```txt
GatewaySchedulerContractV1
BillingEntitlementContractV1
ProviderCapabilityContractV1
```

## 11. Contract 测试

每个 contract 必须覆盖：

- 成功路径。
- 权限失败。
- 状态不存在。
- 幂等重复调用。
- typed error 映射。
- 敏感字段不泄漏。
- 兼容旧 DTO。

高风险 contract 必须有 consumer-driven contract test。

## 12. 实现检查清单

新增跨模块调用前必须确认：

- 是否已有 contract 可复用。
- 是否应该用 domain event 而不是同步调用。
- 是否跨越了 repository 或 handler 边界。
- DTO 是否包含 Ent、HTTP、SQL、SDK 对象。
- 错误是否为 typed error。
- 是否需要幂等键。
- 是否需要审计。
- 是否需要 OpenAPI 暴露。
