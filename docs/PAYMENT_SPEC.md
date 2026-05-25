# SRapi 支付系统规格

## 1. 目标

本文档定义 SRapi 支付系统的产品能力、数据边界、订单状态机、支付渠道、Webhook、退款、幂等和安全要求。

支付系统服务于：

- 用户自助充值余额。
- 用户购买订阅套餐。
- 管理员配置支付渠道。
- 外部支付/发卡/充值系统接入。
- 后续邀请返利与对账。

MVP 明确暂缓完整支付实现，但数据模型、OpenAPI 和模块边界必须按本文档预留。

## 2. 模块边界

```txt
Payments
  ├─ Payment Settings
  ├─ Payment Provider Instance
  ├─ Order Service
  ├─ Webhook Verifier
  ├─ Refund Service
  ├─ Fulfillment Service
  └─ Reconciliation Job
```

Payments 只负责订单、渠道、回调、退款和履约触发，不直接修改模型调度策略。

履约动作通过 Billing / Subscriptions 服务完成：

```txt
payment paid
  ↓
Payment Fulfillment
  ↓
Billing ledger append 或 Subscription activation
  ↓
Audit log
```

## 3. 支付对象

### 3.1 Payment Provider Instance

同一支付渠道可以创建多个实例，用于风控、限额和负载均衡。

字段以 `DATA_MODEL.md` 为准，并至少包含：

```txt
id
provider
name
status
config_ciphertext
config_version
supported_methods_json
limits_json
sort_order
metadata_json
created_at
updated_at
deleted_at
```

### 3.2 Payment Order

订单必须使用平台内部唯一 `order_no`，不得依赖第三方交易号作为主键。

核心字段：

```txt
order_no
user_id
provider_instance_id
amount
currency
status
product_type
product_id
provider_transaction_id
provider_snapshot_json
expires_at
paid_at
closed_at
metadata_json
```

`provider_snapshot_json` 保存下单时非敏感展示信息，避免渠道配置变更后历史订单显示漂移。

## 4. 支持渠道规划

| 渠道 | 阶段 | 说明 |
| --- | --- | --- |
| EasyPay | Phase 2 | 兼容易支付协议，可承接支付宝/微信聚合支付。 |
| Alipay Official | Phase 2+ | 支付宝官方开放平台。 |
| WeChat Pay Official | Phase 2+ | 微信支付 APIv3。 |
| Stripe | Phase 2+ | 国际银行卡和多币种支付。 |
| LDCPay | Phase 3 | Linux DO Credit 或类似积分支付。 |
| Custom Webhook | Phase 3 | 外部支付系统通过受控 API 入账。 |

当前实现已超过最初 MVP 抽象：`payments/providers/checkout` 定义统一下单接口，`payments/providers/stripe` 使用 `stripe-go/v78` 创建 Stripe Checkout Session 并由 Stripe webhook SDK 验签，`payments/providers/easypay` 生成带签名的 EasyPay 跳转 URL。Alipay Official 与 WeChat Pay Official 仍是待接入渠道，计划分别使用 `smartwalle/alipay/v3` 和 `wechatpay-apiv3/wechatpay-go`。

Stripe provider config 至少包含：

```json
{
  "secret_key": "sk_test_...",
  "webhook_secret": "whsec_...",
  "success_url": "https://app.example/pay/success",
  "cancel_url": "https://app.example/pay/cancel"
}
```

EasyPay provider config 至少包含：

```json
{
  "gateway_url": "https://pay.example/submit",
  "merchant_id": "1000",
  "webhook_secret": "provider-signing-secret",
  "notify_url": "https://api.example/api/v1/webhooks/payments/easypay",
  "return_url": "https://app.example/pay/return"
}
```

这些配置通过 payment provider instance 的 `config_ciphertext` 加密保存；订单 metadata 只保存 checkout URL、session id、签名摘要等非密钥信息。

## 5. 前台可见支付方式

管理员可以创建多个 provider instance，但用户侧展示应按“支付方式”聚合：

```txt
alipay
wechat
card
crypto_or_credit
custom
```

同一可见支付方式可以路由到不同底层实例，但同一时刻必须有确定策略。

## 6. 实例负载均衡

多实例选择策略：

| 策略 | 说明 |
| --- | --- |
| `round_robin` | 按顺序轮询可用实例。 |
| `least_amount_today` | 优先选择今日累计金额最低的实例。 |
| `priority_weighted` | 按优先级和权重选择。 |
| `manual_pin` | 管理员指定唯一实例。 |

选择实例前必须执行硬过滤：

- 实例禁用。
- 支付方式不支持。
- 单笔金额超限。
- 每日限额超限。
- 风控冷却中。
- 缺少必要配置。

## 7. 订单状态机

```txt
pending
  ├─ paid
  │   ├─ fulfilled
  │   ├─ partially_refunded
  │   └─ refunded
  ├─ expired
  ├─ canceled
  └─ failed
```

规则：

- `paid` 只能由可信 Webhook、主动查询确认或管理员受控确认触发。
- `fulfilled` 必须在 Billing/Subscription 写入成功后设置。
- 退款不得删除原订单或原 ledger。
- 超时订单由后台 worker 关闭。
- 重复 Webhook 必须幂等返回成功或已处理状态。

## 8. Webhook

Webhook 路径建议：

```txt
POST /api/v1/payment/webhook/easypay
POST /api/v1/payment/webhook/alipay
POST /api/v1/payment/webhook/wxpay
POST /api/v1/payment/webhook/stripe
POST /api/v1/payment/webhook/ldcpay
POST /api/v1/payment/webhook/custom/{instance_id}
```

Webhook 必须：

- 校验签名。
- 校验订单金额和币种。
- 校验订单状态转移合法。
- 记录原始 payload snapshot，敏感字段脱敏。
- 使用幂等键防止重复履约。
- fail closed：签名、金额、订单号不匹配时不得入账。

## 9. 退款

退款能力按 provider instance 独立配置。

退款必须满足：

- 只有已支付订单可退款。
- 不允许超过可退金额。
- 退款成功后写反向 billing ledger。
- 如果订单触发过邀请返利，必须调用 `AFFILIATE_REBATE_SPEC.md` 的补偿流程。
- 退款失败不得修改用户余额。

## 10. 金额和币种

真实账务金额不得使用 float。

推荐：

```txt
numeric(20, 8)
```

或：

```txt
amount_minor bigint + currency
```

所有支付服务商返回金额都必须转换为平台内部金额对象后再进入业务逻辑。

## 11. OpenAPI 草案

用户侧：

```txt
GET  /api/v1/payment/methods
POST /api/v1/payment/orders
GET  /api/v1/payment/orders
GET  /api/v1/payment/orders/{id}
POST /api/v1/payment/orders/{id}/cancel
```

管理侧：

```txt
GET   /api/v1/admin/payments/settings
PATCH /api/v1/admin/payments/settings
GET   /api/v1/admin/payments/providers
POST  /api/v1/admin/payments/providers
PATCH /api/v1/admin/payments/providers/{id}
POST  /api/v1/admin/payments/providers/{id}/test
GET   /api/v1/admin/payments/orders
GET   /api/v1/admin/payments/orders/{id}
POST  /api/v1/admin/payments/orders/{id}/refund
GET   /api/v1/admin/payments/stats
```

外部集成：

```txt
POST /api/v1/admin/payment-integrations/credit
POST /api/v1/admin/payment-integrations/orders/{order_no}/confirm
```

外部集成必须使用独立权限和审计日志，不得复用普通用户 API Key。

## 12. 安全要求

支付配置密钥必须加密保存：

```txt
payment_provider_instances.config_ciphertext
```

禁止：

- 在日志中输出商户私钥、Webhook secret、Stripe secret key。
- 客户端提交 paid 状态直接入账。
- Webhook fail open。
- 金额校验失败时继续履约。
- 退款时直接改写历史 ledger。

## 13. 测试要求

必须覆盖：

- 订单状态机合法/非法迁移。
- Webhook 签名验证。
- Webhook 幂等。
- 金额和币种不匹配拒绝。
- 退款反向 ledger。
- 支付配置脱敏。
- 多实例限额和负载均衡。

## 14. 阶段规划

| 阶段 | 内容 |
| --- | --- |
| MVP | 数据模型和接口占位，支付暂缓。 |
| Phase 2 | EasyPay、支付订单、Webhook、余额充值、管理后台。 |
| Phase 2+ | 支付宝/微信官方、Stripe、退款、对账。 |
| Phase 3 | 外部支付集成、LDCPay、自定义渠道、邀请返利联动。 |
