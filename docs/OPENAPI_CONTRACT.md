# SRapi OpenAPI 契约规范

## 1. 目标

SRapi 使用 OpenAPI-first 工作流。OpenAPI 契约是前端、后端、SDK、文档和测试之间的唯一 HTTP 接口来源。

目标：

- 避免前后端接口漂移。
- 为前端生成类型安全 client。
- 为后端生成请求/响应类型和 server interface。
- 为第三方调用者生成文档。
- 支持 CI 检查接口破坏性变更。

## 2. 文件组织

推荐结构：

```txt
packages/openapi/
├── openapi.yaml
├── paths/
│   ├── auth.yaml
│   ├── user.yaml
│   ├── admin-users.yaml
│   ├── api-keys.yaml
│   ├── providers.yaml
│   ├── models.yaml
│   ├── accounts.yaml
│   ├── gateway.yaml
│   ├── gateway-routes.yaml
│   ├── scheduler.yaml
│   ├── billing.yaml
│   ├── subscriptions.yaml
│   ├── payments.yaml
│   ├── affiliate.yaml
│   ├── capabilities.yaml
│   ├── operations.yaml
│   └── observability.yaml
├── schemas/
│   ├── common.yaml
│   ├── errors.yaml
│   ├── auth.yaml
│   ├── users.yaml
│   ├── providers.yaml
│   ├── models.yaml
│   ├── accounts.yaml
│   ├── scheduler.yaml
│   ├── capabilities.yaml
│   ├── events.yaml
│   ├── billing.yaml
│   └── payments.yaml
└── examples/
```

## 3. API 分区

### 3.1 控制台用户 API

```txt
/api/v1/me
/api/v1/api-keys
/api/v1/usage
/api/v1/billing
/api/v1/subscriptions
/api/v1/payment
/api/v1/affiliate
```

用于普通用户控制台。

### 3.2 管理员 API

```txt
/api/v1/admin/users
/api/v1/admin/providers
/api/v1/admin/models
/api/v1/admin/accounts
/api/v1/admin/scheduler
/api/v1/admin/subscription-plans
/api/v1/admin/user-subscriptions
/api/v1/admin/pricing-rules
/api/v1/admin/payments
/api/v1/admin/affiliate
/api/v1/admin/ops
/api/v1/admin/settings
```

需要管理员权限。

### 3.3 网关兼容 API

```txt
/v1/models
/v1/chat/completions
/v1/responses
/v1/messages
/v1/embeddings
/v1/images/generations
/v1/moderations
```

这些接口面向 API 客户端，必须兼容 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 等主流 AI 端点风格。

Gateway 所有 AI 端点必须先转换为 `AI_ENDPOINT_COMPATIBILITY.md` 定义的 Canonical AI Request，再进入 Scheduler。

Gateway 路由族、Provider alias、passthrough 和 WebSocket 阶段边界以 `GATEWAY_ROUTE_MATRIX.md` 为准。

### 3.4 Webhook API

```txt
/api/v1/webhooks/payments/{provider}
```

Webhook 必须支持幂等处理。

### 3.5 运维 API

```txt
/livez
/readyz
/metrics
```

运维端点的认证、探针语义和生产限制以 `OPERATIONS.md` 为准。

## 4. 鉴权规范

### 4.1 控制台 API

控制台 API 推荐：

```txt
HttpOnly Cookie + CSRF Token
```

请求头：

```txt
X-CSRF-Token: <token>
```

### 4.2 Gateway API

Gateway API 使用：

```txt
Authorization: Bearer sk-xxxx
```

### 4.3 管理员 API

管理员 API 使用控制台登录态，并要求 RBAC 权限。

### 4.4 Security Schemes

OpenAPI 契约必须显式声明安全方案：

```yaml
components:
  securitySchemes:
    cookieAuth:
      type: apiKey
      in: cookie
      name: srapi_session
    csrfHeader:
      type: apiKey
      in: header
      name: X-CSRF-Token
    gatewayBearerAuth:
      type: http
      scheme: bearer
```

规则：

- 控制台读接口使用 `cookieAuth`。
- 控制台写接口使用 `cookieAuth` + `csrfHeader`。
- Gateway `/v1/*` 接口使用 `gatewayBearerAuth`。

### 4.5 Provider Account Import / Export

账号池导入导出接口必须保持凭证安全边界：

- `GET /api/v1/admin/accounts/export` 只导出账号元数据、分组、状态、权重、代理绑定等可操作字段，不得返回 `credential`、`credential_ciphertext`、OAuth token、Cookie、API Key 或 refresh token。
- 导出响应必须包含 `credential_exported: false`，用于提醒调用方该 payload 不能作为完整备份凭证源。
- 导出 metadata 必须递归移除敏感键，例如 `api_key`、`access_token`、`refresh_token`、`authorization`、`cookie`、`secret`、`password`、`token`。
- `POST /api/v1/admin/accounts/import` 的凭证字段是 write-only 输入；服务端必须通过 Provider Account 凭证加密边界持久化，不得在响应、audit before/after、错误 details 或日志中回显。
- import/export 写语义以 OpenAPI schema 为准：export 是读接口使用 `cookieAuth`，import 是写接口必须使用 `cookieAuth` + `csrfHeader`。
- `POST /api/v1/admin/accounts/{id}/discover-models` 用于发现 upstream model catalog；默认只返回预览结果，`persist=true` 才写入 `supported_models`、`model_discovery_source` 和 `model_discovery_last_seen_at`。
- 该 discovery 结果必须用于后续 Provider Account model 选择，保持 `supported_models` 与现有 Scheduler/Gateway 边界一致。

### 4.6 RBAC Matrix

管理员接口必须在 OpenAPI 描述中标注权限需求。

| 能力 | owner | admin | operator | user |
| --- | --- | --- | --- | --- |
| `/api/v1/admin/users` | yes | yes | no | no |
| `/api/v1/admin/providers` | yes | yes | read/test only | no |
| `/api/v1/admin/models` | yes | yes | read only | no |
| `/api/v1/admin/accounts` | yes | yes | read/test only | no |
| `/api/v1/admin/scheduler` | yes | yes | read/simulate only | no |
| `/api/v1/admin/ops/slo` | yes | yes | read only | no |
| `/api/v1/admin/ops/alerts` | yes | yes | read only | no |
| `/api/v1/admin/settings` | yes | yes | no | no |
| `/api/v1/admin/audit-logs` | yes | yes | no | no |

### 4.7 Ops SLO / Alert APIs

`/api/v1/admin/ops/slo` 和 `/api/v1/admin/ops/alerts` 属于 AdminOps 控制面：

- `GET /api/v1/admin/ops/slo` 返回 SLO definition 以及基于 `usage_logs` 计算的 availability/burn-rate 证据。
- `POST /api/v1/admin/ops/slo`、`PATCH /api/v1/admin/ops/slo/{id}` 必须使用 `cookieAuth` + `csrfHeader`，并记录安全 audit before/after。
- SLO `objective` 请求可接受 `0.995` 或 `99.5`；响应统一返回比例值。
- `GET /api/v1/admin/ops/alerts` 支持 `status`、`severity` 过滤。
- `POST /api/v1/admin/ops/alerts/{id}/ack` 必须使用 CSRF，并且 audit 只记录 ack 摘要，不复制 alert `details`。

## 5. 统一响应格式

控制台和管理 API 使用统一响应格式。

成功响应：

```json
{
  "data": {},
  "request_id": "req_xxx"
}
```

列表响应：

```json
{
  "data": [],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100,
    "has_next": true
  },
  "request_id": "req_xxx"
}
```

错误响应：

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "invalid request",
    "details": {},
    "trace_id": "trace_xxx"
  },
  "request_id": "req_xxx"
}
```

## 6. Gateway 错误格式

OpenAI-compatible API 应尽量返回 OpenAI 风格错误：

```json
{
  "error": {
    "message": "The request is invalid.",
    "type": "invalid_request_error",
    "param": null,
    "code": "invalid_request"
  }
}
```

内部仍需要记录 SRapi 自己的错误分类：

```txt
provider_error_class
scheduler_reject_reason
billing_error_code
account_error_state
```

## 7. HTTP 状态码规范

```txt
200 OK                    成功
201 Created               创建成功
202 Accepted              异步任务已接受
204 No Content            删除成功且无返回
400 Bad Request           请求格式错误
401 Unauthorized          未认证
403 Forbidden             无权限或套餐限制
404 Not Found             资源不存在
409 Conflict              状态冲突或唯一约束冲突
422 Unprocessable Entity  业务校验失败
429 Too Many Requests     限流或额度超限
500 Internal Server Error 内部错误
502 Bad Gateway           上游错误
503 Service Unavailable   服务不可用或无可用账号
504 Gateway Timeout       上游超时
```

## 8. 错误码规范

错误码使用大写蛇形命名。

示例：

```txt
INVALID_REQUEST
UNAUTHORIZED
FORBIDDEN
RESOURCE_NOT_FOUND
RESOURCE_CONFLICT
VALIDATION_FAILED
RATE_LIMIT_EXCEEDED
USER_BALANCE_INSUFFICIENT
SUBSCRIPTION_EXPIRED
API_KEY_DISABLED
MODEL_NOT_ALLOWED
MODEL_NOT_FOUND
NO_AVAILABLE_ACCOUNT
SCHEDULER_REJECTED
PROVIDER_RATE_LIMITED
PROVIDER_AUTH_FAILED
PROVIDER_QUOTA_EXCEEDED
PAYMENT_ORDER_NOT_FOUND
PAYMENT_WEBHOOK_INVALID
INTERNAL_ERROR
```

## 9. 分页规范

请求参数：

```txt
page
page_size
sort
order
```

默认：

```txt
page = 1
page_size = 20
max_page_size = 100
```

响应：

```json
{
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100,
    "has_next": true
  }
}
```

## 10. 过滤和搜索规范

列表接口可使用：

```txt
q
status
provider
model
user_id
created_from
created_to
```

复杂过滤第一阶段不做 DSL，优先使用明确 query 参数。

## 11. 幂等规范

以下接口必须支持幂等：

- 创建支付订单。
- 支付 Webhook。
- Gateway 非流式请求，可选。
- 账务调整。
- 订阅激活。

请求头：

```txt
Idempotency-Key: <key>
```

幂等记录建议保存：

```txt
key
method
path
request_hash
response_snapshot
status
expires_at
```

## 12. Trace 与 Request ID

所有响应都应包含：

```txt
X-Request-ID
```

如果启用 OpenTelemetry，还应关联 trace id。

前端错误展示和后台日志查询都应使用 request id。

## 13. 时间格式

所有时间使用 ISO 8601 UTC 字符串。

示例：

```txt
2026-05-21T00:00:00Z
```

## 14. 金额格式

API 响应中金额建议使用字符串或整数最小单位，避免 float 精度问题。

推荐：

```json
{
  "amount": "12.34",
  "currency": "USD"
}
```

数据库内部可使用 decimal 或 int64 minor units。

## 15. Token 用量格式

统一 usage：

```json
{
  "input_tokens": 1000,
  "output_tokens": 500,
  "cached_tokens": 800,
  "total_tokens": 1500
}
```

Provider 特有字段放入：

```json
{
  "provider_usage": {}
}
```

## 16. 主要接口草案

### 16.1 Auth

```txt
POST /api/v1/auth/login
POST /api/v1/auth/logout
POST /api/v1/auth/refresh
GET  /api/v1/auth/session
```

### 16.2 Current User

```txt
GET /api/v1/me
GET /api/v1/me/usage
GET /api/v1/me/billing
GET /api/v1/me/subscriptions
```

### 16.3 API Keys

```txt
GET    /api/v1/api-keys
POST   /api/v1/api-keys
GET    /api/v1/api-keys/{id}
PATCH  /api/v1/api-keys/{id}
DELETE /api/v1/api-keys/{id}
```

API Key 创建和更新必须支持 `group_ids`，用于多组绑定；运行时解析规则以 `GATEWAY_ROUTE_MATRIX.md` 为准。

### 16.4 Admin Providers

```txt
GET   /api/v1/admin/providers
POST  /api/v1/admin/providers
GET   /api/v1/admin/providers/{id}
PATCH /api/v1/admin/providers/{id}
POST  /api/v1/admin/providers/{id}/test
```

### 16.5 Admin Models

```txt
GET   /api/v1/admin/models
POST  /api/v1/admin/models
GET   /api/v1/admin/models/{id}
PATCH /api/v1/admin/models/{id}
POST  /api/v1/admin/models/{id}/aliases
POST  /api/v1/admin/models/{id}/mappings
GET   /api/v1/admin/capabilities
GET   /api/v1/admin/capabilities/{key}
```

Capability descriptor、版本、状态和降级规则以 `CAPABILITY_TAXONOMY_SPEC.md` 为准。

### 16.6 Admin Accounts

```txt
GET   /api/v1/admin/accounts
POST  /api/v1/admin/accounts
GET   /api/v1/admin/accounts/{id}
PATCH /api/v1/admin/accounts/{id}
POST  /api/v1/admin/accounts/{id}/test
POST  /api/v1/admin/accounts/{id}/discover-models
POST  /api/v1/admin/accounts/{id}/disable
POST  /api/v1/admin/accounts/{id}/enable
GET   /api/v1/admin/accounts/{id}/health
GET   /api/v1/admin/accounts/{id}/quota
```

`GET /api/v1/admin/accounts/{id}/health` 必须返回运维排障所需的低基数字段：

- 账号和 Provider 标识：`account_id`、`provider_id`、`runtime_class`、`status`。
- 最近错误与健康：`error_class`、`success_rate`、`error_rate`、`latency_p50_ms`、`latency_p95_ms`。
- 额度与限流：`quota_remaining_ratio`、`quota_exhausted`、`rate_limit_count`、`timeout_count`。
- 保护状态：`cooldown_until`、`cooldown_reason`、`circuit_state`、`snapshot_at`。

该响应不得包含账号名称、上游凭证、Cookie、OAuth token、API Key 或 prompt 内容。

### 16.7 Admin Scheduler

```txt
GET  /api/v1/admin/scheduler/overview
GET  /api/v1/admin/scheduler/decisions
GET  /api/v1/admin/scheduler/decisions/{id}
POST /api/v1/admin/scheduler/simulate
GET  /api/v1/admin/scheduler/strategies
POST /api/v1/admin/scheduler/strategies
GET  /api/v1/admin/scheduler/strategies/{id}
PATCH /api/v1/admin/scheduler/strategies/{id}
POST /api/v1/admin/scheduler/strategies/{id}/activate
POST /api/v1/admin/scheduler/strategies/{id}/simulate
GET  /api/v1/admin/scheduler/strategies/{id}/versions
```

策略 descriptor、配置 schema、版本、dry-run、shadow decision 和回滚规则以 `SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 为准。

### 16.8 Gateway

```txt
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
POST /v1/messages
POST /api/provider/openai-compatible/v1/chat/completions
POST /api/provider/openai-compatible/v1/responses
POST /api/provider/openai-compatible/v1/messages
POST /api/provider/openai-compatible/v1/embeddings
POST /api/provider/openai-compatible/v1/images/generations
POST /api/provider/openai-compatible/v1/moderations
POST /api/provider/anthropic-compatible/v1/messages
```

第一阶段必须优先实现标准四个 Gateway 入口；已暴露的 Provider alias 必须复用同一 Gateway runtime，只改变 provider context。

后续更多 Provider alias、passthrough、Gemini native、WebSocket、audio、rerank 等路由以 `GATEWAY_ROUTE_MATRIX.md` 为准。

### 16.9 Admin Subscriptions

```txt
GET  /api/v1/admin/subscription-plans
POST /api/v1/admin/subscription-plans
GET  /api/v1/admin/user-subscriptions
POST /api/v1/admin/user-subscriptions
GET  /api/v1/admin/pricing-rules
POST /api/v1/admin/pricing-rules
```

订阅与定价控制面必须满足：

- 金额和每百万 tokens 单价使用 decimal string，不使用 float 表示真实账务金额。
- `GET /api/v1/me/subscriptions` 只能返回当前用户订阅。
- 管理员创建用户订阅时必须复制套餐权益快照，后续套餐变更不得回写既有订阅权益。
- Pricing Rule 的 `provider_id=0` 表示模型级通用价格，具体 Provider 规则优先。
- Gateway admission 必须在 Scheduler 获取账号 lease 前执行用户/模型 entitlement 检查。

### 16.10 Admin Ops

```txt
GET  /api/v1/admin/ops/overview
GET  /api/v1/admin/ops/traffic
GET  /api/v1/admin/ops/errors
GET  /api/v1/admin/ops/providers
GET  /api/v1/admin/ops/scheduler/decisions
GET  /api/v1/admin/ops/alerts
POST /api/v1/admin/ops/alerts/{id}/ack
GET  /api/v1/admin/ops/slo
POST /api/v1/admin/ops/slo
PATCH /api/v1/admin/ops/slo/{id}
GET  /api/v1/admin/ops/events/outbox
GET  /api/v1/admin/ops/events/dead-letter
POST /api/v1/admin/ops/events/{event_id}/replay
```

领域事件 Outbox、Inbox、重试、死信和补偿以 `DOMAIN_EVENTS_SPEC.md` 为准。

### 16.11 Payments

```txt
GET  /api/v1/payment/methods
POST /api/v1/payment/orders
GET  /api/v1/payment/orders
GET  /api/v1/payment/orders/{id}
POST /api/v1/payment/orders/{id}/cancel
GET  /api/v1/admin/payments/orders
POST /api/v1/admin/payments/orders/{id}/refund
```

### 16.12 Affiliate

```txt
GET  /api/v1/me/affiliate
GET  /api/v1/me/affiliate/ledger
POST /api/v1/me/affiliate/transfer-to-balance
GET  /api/v1/admin/affiliate/rules
POST /api/v1/admin/affiliate/rules
PATCH /api/v1/admin/affiliate/rules/{id}
GET  /api/v1/admin/affiliate/ledger
```

## 17. AI 端点兼容边界

第一阶段必须兼容：

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses`
- `/v1/messages`
- `stream: true`
- `messages`
- `input`
- `model`
- `temperature`
- `top_p`
- `max_tokens`
- `max_output_tokens`
- `instructions`
- 基础 tool calls，可选
- JSON mode / structured output 基础字段

端点转换规则以 `AI_ENDPOINT_COMPATIBILITY.md` 为准。

第一阶段可以暂缓：

- Assistants API。
- Responses API stateful store 和内置工具全量兼容。
- Batch API。
- Fine-tuning API。
- Realtime API。

## 18. 版本策略

管理 API 版本：

```txt
/api/v1
```

Gateway 兼容 API 保持行业路径：

```txt
/v1
```

破坏性变更必须进入新版本。

## 19. 生成工具

后端：

```txt
oapi-codegen
```

前端：

```txt
@hey-api/openapi-ts
orval
```

文档：

```txt
Scalar
Swagger UI
```

## 20. OperationId 规范

所有 operationId 必须稳定、可读、可用于生成前后端代码。

命名规则：

```txt
listApiKeys
createApiKey
getApiKey
updateApiKey
deleteApiKey
listAdminProviders
createAdminProvider
testAdminProvider
listAdminAccounts
testAdminAccount
simulateScheduler
listSchedulerDecisions
createChatCompletion
listModels
```

禁止：

- 使用自动生成的 `getApiV1AdminProviders` 作为最终 operationId。
- 同名 operationId。
- 在不改 endpoint 的情况下随意重命名 operationId。

## 21. Schema 复用规范

OpenAPI 必须复用以下公共 schema：

```txt
RequestId
ErrorResponse
Pagination
Money
TokenUsage
AuditActor
ProviderErrorClass
SchedulerRejectReason
```

错误响应必须同时满足：

- 控制台和管理 API 使用 SRapi 标准错误结构。
- Gateway `/v1/*` 对客户端返回 OpenAI-compatible 错误结构。
- 内部日志保留 SRapi 错误码、provider_error_class、scheduler_reject_reason。

## 22. CI 校验

CI 至少包含：

- OpenAPI lint。
- OpenAPI bundle。
- 生成代码是否最新。
- Breaking change 检查。
- 后端编译。
- 前端 typecheck。
