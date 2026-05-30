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
original_amount
discount_amount
promo_code_id nullable
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
`amount` 是实际提交给支付渠道的最终应付金额；`original_amount` 保存用户提交的
原始金额；`discount_amount` 保存服务端计算后的优惠金额；`promo_code_id` 只保存
settings-backed promo code 的稳定 id，不保存优惠码明文。

## 4. 支持渠道规划

| 渠道 | 阶段 | 说明 |
| --- | --- | --- |
| EasyPay | Phase 2 | 兼容易支付协议，可承接支付宝/微信聚合支付。 |
| Alipay Official | Phase 2+ | 支付宝官方开放平台，已接入 PC Page Pay / H5 Wap Pay 下单和异步通知验签。 |
| WeChat Pay Official | Phase 2+ | 微信支付 APIv3，已接入 Native / H5 / JSAPI 下单和 APIv3 通知验签解密。 |
| Stripe | Phase 2+ | 国际银行卡和多币种支付。 |
| LDCPay | Phase 3 | Linux DO Credit 或类似积分支付。 |
| Custom Webhook | Phase 3 | 外部支付系统通过受控 API 入账。 |

当前实现已超过最初 MVP 抽象：`payments/providers/checkout` 定义统一下单接口，`payments/providers/stripe` 使用 `stripe-go/v78` 创建 Stripe Checkout Session 并由 Stripe webhook SDK 验签，`payments/providers/easypay` 生成带签名的 EasyPay 跳转 URL，`payments/providers/alipay` 使用 `smartwalle/alipay/v3` 生成支付宝 Page/Wap Pay 支付 URL，并由 `payments/service` 使用支付宝公钥验签异步通知。`payments/providers/wechat` 使用 `wechatpay-apiv3/wechatpay-go` 创建微信 Native / H5 / JSAPI 预支付订单，并由 `payments/service` 使用微信 APIv3 通知签名验证和 AES-GCM 解密后复用现有幂等、金额校验和履约链路。管理员侧已支持 provider instance 的创建、更新和本地配置测试；测试接口只解密并校验必需配置，不发起外部扣款或网络请求。Stripe test-mode 充值闭环已有 `make smoke-payment-stripe` 入口；Alipay Page Pay checkout smoke 已有 `make smoke-payment-alipay` 入口，并可选用本地签名通知验证 SRapi webhook 链路；WeChat Pay APIv3 prepay smoke 已有 `make smoke-payment-wechat` 入口，并可选用本地签名加密通知验证 SRapi webhook 链路；真实沙箱/商户外部回调仍需对应渠道凭证和平台通知演练。

Stripe provider config 至少包含：

```json
{
  "secret_key": "sk_test_...",
  "webhook_secret": "whsec_...",
  "success_url": "https://app.example/pay/success",
  "cancel_url": "https://app.example/pay/cancel"
}
```

Stripe test-mode smoke 运行方式：

```bash
STRIPE_SMOKE_SECRET_KEY=<stripe-test-secret-key> \
STRIPE_SMOKE_WEBHOOK_SECRET=<stripe-webhook-signing-secret> \
make smoke-payment-stripe
```

该 smoke 要求 API 已启动，并使用 `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` 登录。脚本会创建或更新一个 `stripe-smoke` provider instance，发起一笔 `balance_credit` Checkout Session，校验 checkout URL/session id，向 `/api/v1/webhooks/payments/stripe` 提交本地签名的 `checkout.session.completed` 事件，确认订单 fulfilled、重复 webhook 幂等、余额增加，最后禁用临时 provider。它只验证 SRapi 到 Stripe Checkout 创建 API 的真实 test-mode 出站调用；webhook 入站用本地签名事件复用同一个 SRapi 验签和履约路径，生产环境仍应配置 Stripe Dashboard webhook endpoint 并执行真实回调演练。

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

Alipay Official provider config 至少包含：

```json
{
  "app_id": "2021000000000000",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----...",
  "alipay_public_key": "-----BEGIN PUBLIC KEY-----...",
  "notify_url": "https://api.example/api/v1/webhooks/payments/alipay",
  "return_url": "https://app.example/pay/return"
}
```

可选字段包括 `gateway_url`、`production`、`mode`、`subject`、`body`、`qr_pay_mode` 和 `qr_code_width`。`mode=page` 走 `alipay.trade.page.pay`，`mode=wap` / `mode=h5` 走 `alipay.trade.wap.pay`；回调归属按本地 `out_trade_no` 对应订单绑定的 provider instance 验签，避免多商户实例互相抢签。按 [Alipay 异步通知说明](https://global.alipay.com/developer/helpcenter/detail?_route=sg&categoryId=67617&knowId=201602452303&sceneCode=AC_DEV)，HTTP webhook 成功处理支付宝异步通知后返回纯文本 `success`，否则支付宝会按渠道规则重试通知；验签时保留支付宝返回的全部参数，排除 `sign` 和 `sign_type`。

WeChat Pay Official provider config 至少包含：

```json
{
  "app_id": "wx0000000000000000",
  "mch_id": "1900000000",
  "api_v3_key": "32-byte-api-v3-key",
  "serial_no": "merchant_certificate_serial_no",
  "private_key": "-----BEGIN PRIVATE KEY-----...",
  "notify_url": "https://api.example/api/v1/webhooks/payments/wechat"
}
```

可选字段包括 `mode`、`description`、`payer_client_ip`、`payer_openid`、`h5_type`、`h5_app_name`、`h5_app_url`、`h5_bundle_id`、`h5_package_name`、`wechatpay_public_key` 和 `wechatpay_public_key_id`。默认 `mode=native` 返回二维码 `code_url`；`mode=h5` 需要 `payer_client_ip` 并返回 H5 跳转链接；`mode=jsapi` 需要 `payer_openid` 并返回调起支付参数。若配置 `wechatpay_public_key` / `wechatpay_public_key_id`，通知验签使用微信支付公钥；否则使用 SDK 自动下载并轮换平台证书。

WeChat Pay APIv3 smoke 运行方式：

```bash
WECHAT_SMOKE_APP_ID=<wechat-app-id> \
WECHAT_SMOKE_MCH_ID=<wechat-merchant-id> \
WECHAT_SMOKE_API_V3_KEY=<32-byte-api-v3-key> \
WECHAT_SMOKE_SERIAL_NO=<merchant-certificate-serial> \
WECHAT_SMOKE_PRIVATE_KEY=<merchant-private-key-pem> \
make smoke-payment-wechat
```

该 smoke 要求 API 已启动，并使用 `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` 登录。脚本会创建或更新一个 `wechat-smoke` provider instance，发起一笔 `balance_credit` 微信预支付订单，校验 Native/H5/JSAPI checkout metadata，最后禁用临时 provider。`WECHAT_SMOKE_LOCAL_WEBHOOK=1` 时，脚本会从 `WECHAT_SMOKE_PLATFORM_PRIVATE_KEY` 派生临时微信支付公钥写入 provider config，再向 `/api/v1/webhooks/payments/wechat` 提交本地签名且 AES-GCM 加密的 `TRANSACTION.SUCCESS` 事件，确认订单 fulfilled、重复 webhook 幂等、余额增加。该本地通知只验证 SRapi 的 APIv3 验签、解密和履约路径；生产环境仍应配置微信支付平台通知并执行真实回调演练。

这些配置通过 payment provider instance 的 `config_ciphertext` 加密保存；订单 metadata 只保存 checkout URL、session id、签名摘要等非密钥信息。

管理员更新 provider instance 时，`provider` 类型保持不可变，`name/status/supported_methods/limits/sort_order/metadata/config` 可更新。若重命名或替换配置，服务端会用新的 AAD 重新加密 config；响应和 audit 只暴露 `config_configured` / `config_version`，不回显密钥字段。有未关闭订单（`pending` 或 `paid`）绑定到该实例时，管理员仍可更新展示名、排序和非敏感 metadata，但不得禁用/归档实例、移除已有 `supported_methods`，或替换加密 config；这些变更必须等订单进入 `fulfilled`、`refunded`、`canceled`、`expired` 或 `failed` 后再执行，避免 checkout、webhook 和退款归属漂移。

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

`POST /api/v1/payment/orders` 可接受可选 `promo_code`。服务端必须在创建第三方
checkout 前校验优惠码 active 状态、`starts_at`、`expires_at`、`max_uses`、币种、
金额折扣或比例折扣规则，并把折扣后的最终 `amount` 提交给支付渠道。持久化实现
必须在同一事务里写 `payment_orders` 折扣字段、`user_promo_code_applications`
回执和 `admin_control.promo_codes` 的 `used_count` 更新；失败时不得留下半创建订单。

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
- 优惠码校验、折扣金额、用量耗尽、错误币种和事务回滚。
- 退款反向 ledger。
- 支付配置脱敏。
- 多实例限额和负载均衡。

外部 smoke：

- Stripe：`make smoke-payment-stripe`，需要 Stripe test mode secret key 和 webhook signing secret，覆盖 Checkout Session 创建、SRapi webhook 验签/幂等/履约、余额入账和临时 provider 清理。
- Alipay：`make smoke-payment-alipay`，需要支付宝沙箱或测试商户 `app_id`、应用私钥和支付宝公钥。默认覆盖 Page Pay RSA2 checkout URL 生成和临时 provider 清理；`ALIPAY_SMOKE_LOCAL_WEBHOOK=1` 可额外用本地签名通知覆盖 SRapi webhook 验签/`success` 应答/履约/余额入账/重复通知幂等。该本地签名模式不得替代支付宝沙箱真实回调结论。
- WeChat：`make smoke-payment-wechat`，需要微信支付商户 `app_id`、`mch_id`、APIv3 key、商户证书序列号和商户私钥。默认覆盖真实预支付下单、Native/H5/JSAPI checkout metadata 和临时 provider 清理；`WECHAT_SMOKE_LOCAL_WEBHOOK=1` 可额外用本地 APIv3 签名加密通知覆盖 SRapi webhook 验签/解密/履约/余额入账/重复通知幂等。该本地签名模式不得替代微信支付平台真实通知结论。

## 14. 阶段规划

| 阶段 | 内容 |
| --- | --- |
| MVP | 数据模型和接口占位，支付暂缓。 |
| Phase 2 | EasyPay、支付订单、Webhook、余额充值、管理后台。 |
| Phase 2+ | 支付宝/微信官方、Stripe、退款、对账。 |
| Phase 3 | 外部支付集成、LDCPay、自定义渠道、邀请返利联动。 |
