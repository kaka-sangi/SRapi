# SRapi 邀请返利规格

## 1. 目标

本文档定义 SRapi 邀请返利系统的领域模型、账本、结算、退款补偿和安全边界。

返利系统服务于商业化增长，但不得破坏账务一致性。

核心原则：

```txt
邀请关系可追溯
返利账本只追加
退款通过反向补偿
提现或转余额必须审计
```

## 1.1 当前实现状态

返利系统已落地（模块 `apps/api/internal/modules/affiliate`，ent schema `invitecode` / `inviterelationship` / `affiliaterule` / `affiliateledger`，持久化 `internal/persistence/entstore/affiliate`）。下表区分已交付能力与仍在路线图上的能力。

| 状态 | 能力 |
| --- | --- |
| 已实现 | 邀请码与邀请关系（`CreateInviteCode` / `BindInvite`，自邀请与重复绑定拒绝）。 |
| 已实现 | 返利规则（`CreateRule`，`GetEffectiveRule` 选取生效规则；当前 `trigger_type` 仅 `payment_paid`）。 |
| 已实现 | 支付履约后按规则生成 `accrue` 账本（`AccrueRebate`，按订单号幂等）。 |
| 已实现 | 退款反向补偿（`CompensateRefund`，追加 `refund_compensation`，支持部分/全额）。 |
| 已实现 | 转余额（`TransferToBalance`，`Idempotency-Key` 幂等，写 billing ledger 与 user balance）。 |
| 已实现 | 用户侧汇总/账本接口与管理侧只读列表（见 §9）。 |
| 路线图（未实现） | 提现 / withdraw（`withdraw` 账本类型已预留为常量，但无服务方法与对外接口）。 |
| 路线图（未实现） | 分层返利、活动规则、外部结算、风险评分。 |

## 2. 阶段边界（历史设计 → 当前状态）

下表为最初的分阶段规划，状态列反映落地情况。

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| MVP | 用户关系与账本扩展点。 | 已实现（不再是"暂缓"）。 |
| Phase 2 | 支付订单完成后按规则生成返利 ledger。 | 已实现（见 §4 触发流程）。 |
| Phase 3 | 返利提现、分层返利、活动规则、外部结算。 | 路线图 / 未实现。 |

## 3. 领域对象

### 3.1 Invite Code

```txt
id
user_id
code
status
created_at
expires_at
```

一个用户可以有多个邀请码，但同一时刻默认展示一个主邀请码。

### 3.2 Invite Relationship

```txt
id
inviter_user_id
invitee_user_id
invite_code_id
created_at
first_paid_at
status
```

规则：

- 一个 invitee 只能绑定一个 inviter。
- 自己不能邀请自己。
- 管理员手动调整必须写 audit log。
- 是否允许解绑或改绑必须由策略明确，默认不允许。

### 3.3 Affiliate Rule

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
```

`trigger_type` 示例：

```txt
payment_paid
subscription_purchased
manual_adjustment
```

### 3.4 Affiliate Ledger

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

`type`：

```txt
accrue
settle
transfer_to_balance
withdraw
refund_compensation
manual_adjustment
```

## 4. 返利触发流程

```txt
Payment order fulfilled
  ↓
Find invite relationship
  ↓
Load active affiliate rule
  ↓
Calculate rebate
  ↓
Append affiliate_ledger(accrue)
  ↓
Audit log
```

返利计算必须基于已确认入账金额，不得基于 pending order。

## 5. 结算状态

```txt
pending
settled
canceled
compensated
```

可配置结算等待期，避免支付后立即退款导致返利被套取（退款发生在等待期内时由 `refund_compensation` 反向补偿，见 §6）。

## 6. 退款补偿

如果支付订单已产生返利，退款时不得删除原 `accrue` 记录。

必须追加：

```txt
affiliate_ledger(type=refund_compensation, amount=-original_amount)
```

规则：

- 部分退款按比例补偿。
- 全额退款补偿全部返利。
- 已提现返利不得直接回滚历史提现，需要产生负余额或待追偿记录。
- 补偿动作必须与 payment refund 在同一业务事务或可靠 outbox 中保持最终一致。

## 7. 转余额与提现

返利可以支持两种出账方式：

| 方式 | 说明 |
| --- | --- |
| `transfer_to_balance` | 转入 SRapi 用户余额，可用于消费。已实现。 |
| `withdraw` | 提现到外部渠道。路线图 / 未实现（账本类型常量已预留，尚无服务方法与对外接口）。 |

转余额必须写入 Billing Ledger：

```txt
affiliate_ledger transfer_to_balance
  ↓
billing_ledger credit
  ↓
users.balance update
```

## 8. 风控规则

必须防止：

- 自邀请。
- 循环邀请。
- 同设备/同支付账户批量套利。
- 退款套返利。
- 管理员绕过审计调整返利。

当前已落地基础风控规则（自邀请拒绝、重复绑定拒绝、管理员调整写审计）。风险评分为路线图能力，尚未实现。

## 9. OpenAPI

用户侧：

```txt
GET  /api/v1/me/affiliate
GET  /api/v1/me/affiliate/ledger
POST /api/v1/me/affiliate/transfer-to-balance
```

`POST /api/v1/me/affiliate/transfer-to-balance` 必须使用控制台 session、CSRF header 和 `Idempotency-Key` header。服务端按幂等 key 生成转余额 reference，重复请求返回同一 affiliate ledger 结果且不会重复写 billing ledger 或重复增加 user balance。

管理侧（均为只读列表）：

```txt
GET   /api/v1/admin/affiliates/invites
GET   /api/v1/admin/affiliates/rebates
GET   /api/v1/admin/affiliates/transfers
```

以上路由的契约以 `packages/openapi/openapi.yaml` 为准（`/api/v1/me/affiliate*` 与 `/api/v1/admin/affiliates/*`）。

返利流程通过 outbox 发布事件，当前实现的事件类型为 `AffiliateRebateAccrued` 与 `AffiliateRebateCompensated`（事件名以 `internal/modules/affiliate/service` 为准）。

## 10. 数据一致性

强一致：

- 转余额时 affiliate ledger、billing ledger、user balance。
- 手动调整和 audit log。

最终一致：

- 支付履约后返利 accrual。
- 退款后的返利补偿。
- 报表聚合。

如果使用 outbox，必须支持幂等消费。

## 11. 安全与审计

必须审计：

- 返利规则创建/修改/禁用。
- 邀请关系手动调整。
- 手动返利调整。
- 转余额。
- 提现审批（伴随 withdraw 落地，路线图能力）。
- 退款补偿失败。

不得在公开接口暴露其他用户敏感信息，邀请列表默认只展示脱敏邮箱或匿名 ID。

## 12. 测试要求

必须覆盖：

- 自邀请拒绝。
- 重复绑定拒绝。
- 支付成功生成返利。
- 重复 Webhook 不重复返利。
- 部分退款补偿。
- 全额退款补偿。
- 转余额账务一致。
- 管理员调整写 audit。
