# SRapi 后端架构设计

## 1. 架构目标

SRapi 后端采用模块化单体架构，目标是在第一阶段避免微服务复杂度，同时通过清晰的模块边界、接口抽象和依赖方向，为后续扩展 Provider、模型能力、调度策略、计费方式和部署形态预留空间。

核心目标：

- 保持业务模块高内聚、低耦合。
- 调度内核保持 Provider-neutral。
- Gateway 不直接依赖具体 Provider 实现。
- Gateway 负责多 AI 端点兼容，并把 Chat Completions、Responses、Messages 等端点统一转换为 Canonical AI Request。
- Provider Adapter 下必须存在独立的反代运行时（Reverse Proxy Runtime），承载 sub2api / claude2api / chatgpt2api / gemini2api / grok2api / cursor2api / antigravity2api 等 2api 场景。
- Billing 不侵入 Gateway 和 Scheduler 主流程。
- OpenAPI 作为 HTTP 契约唯一来源。
- PostgreSQL 作为真实数据源。
- Redis 只承载可重建的运行时状态。
- 所有敏感凭证通过平台层加密能力访问。

MVP 级可执行架构要求与 harness 见 `ARCHITECTURE_REQUIREMENTS.md`。

## 2. 架构风格

第一阶段采用：

```txt
Modular Monolith + Hexagonal Boundaries + OpenAPI-first
```

含义：

- 单进程部署，降低初期复杂度。
- 内部按领域模块拆分。
- 模块之间通过接口交互。
- 具体基础设施实现放在 platform 层或模块内 adapter 中。
- HTTP 层、数据库层、Provider 协议层不能污染核心业务逻辑。
- 跨模块同步调用必须依赖 `MODULE_INTERFACE_CONTRACTS.md` 定义的 contract。
- 跨模块最终一致协作必须通过 `DOMAIN_EVENTS_SPEC.md` 定义的领域事件和 Outbox。

## 3. 代码结构

推荐后端目录：

```txt
apps/api/
├── cmd/
│   └── srapi/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── bootstrap.go
│   │   ├── dependencies.go
│   │   └── lifecycle.go
│   ├── config/
│   ├── http/
│   │   ├── server.go
│   │   ├── router.go
│   │   ├── middleware/
│   │   └── response/
│   ├── modules/
│   │   ├── auth/
│   │   ├── users/
│   │   ├── api_keys/
│   │   ├── providers/
│   │   ├── models/
│   │   ├── accounts/
│   │   ├── gateway/
│   │   ├── scheduler/
│   │   ├── billing/
│   │   ├── subscriptions/
│   │   ├── payments/
│   │   ├── affiliate/
│   │   ├── audit/
│   │   └── observability/
│   ├── openapi/
│   ├── platform/
│   │   ├── db/
│   │   ├── redis/
│   │   ├── logger/
│   │   ├── crypto/
│   │   ├── httpclient/
│   │   ├── clock/
│   │   └── objectstore/
│   └── workers/
├── ent/
├── migrations/
└── go.mod
```

## 4. 分层职责

### 4.1 `cmd`

只负责进程入口：

- 加载配置。
- 初始化 logger。
- 初始化 `internal/app`。
- 启动 HTTP server。
- 处理 graceful shutdown。

不得包含业务逻辑。

### 4.2 `internal/app`

负责应用组装和生命周期：

- 创建 HTTP server。
- 管理启动与停止顺序。
- 创建数据库、Redis、persistence store 和其他平台资源。
- 通过模块 contract 把 persistence 实现注入 HTTP runtime。

### 4.3 `internal/httpserver`

负责 HTTP 协议层：

- 路由注册。
- Middleware。
- OpenAPI handler adapter。
- 统一响应。
- 统一错误渲染。

不得直接访问 Ent query；需要状态时只能使用已注入的 service / contract。

### 4.4 `internal/modules`

业务模块所在地。

每个模块拥有自己的：

- Domain model。
- Service。
- Contract。
- Repository interface。
- Handler。
- Policy。
- Errors。
- Tests。

### 4.5 `internal/platform`

基础设施能力：

- 数据库连接。
- Redis。
- 加密。
- 日志。
- HTTP client。
- 对象存储。
- 时间源。

当前已落地的基础设施抽象至少包括 `internal/platform/logger` 与 `internal/platform/crypto`。
当前已落地的启动期平台抽象还包括 `internal/platform/db` 与 `internal/platform/redis`。

平台层不得反向依赖业务模块。

### 4.6 `internal/openapi`

存放 OpenAPI 生成代码。

规则：

- 生成代码不得手改。
- 业务代码可以引用生成的请求/响应类型。
- 生成接口由 handler 或 adapter 实现。

### 4.7 `internal/workers`

承载后台轮询和异步投递任务。

规则：

- 只能通过模块 contract / service 访问业务能力。
- 不得直接访问 Ent query、HTTP handler 或 persistence 实现。
- 生命周期由 `internal/app` 统一启动和关闭。

## 5. 模块依赖方向

推荐依赖方向：

```txt
http → modules → platform
       modules → openapi types
       modules → other module interfaces
       modules → domain event publisher
```

禁止：

```txt
platform → modules
repository → handler
service → handler
provider adapter → http handler
scheduler → concrete provider implementation
billing → concrete gateway handler
module A → module B repository
module A → module B handler
```

## 6. 核心调用链

### 6.1 控制台 API 调用链

```txt
HTTP Request
  ↓
OpenAPI Handler Adapter
  ↓
Module Handler
  ↓
Module Service
  ↓
Repository Interface
  ↓
Ent Repository Implementation
  ↓
PostgreSQL
```

### 6.2 网关请求调用链

```txt
Gateway HTTP Request
  ↓
Gateway Handler
  ↓
API Key Auth
  ↓
Client Endpoint Adapter
  ↓
Canonical AI Request Normalizer
  ↓
Scheduler Kernel
  ↓
Provider Adapter Registry
  ↓
Selected Provider Adapter
  ↓
Reverse Proxy Runtime  仅对 runtime_class != api_key 的账号
  ↓
Upstream AI Provider 或 Upstream Official Client Endpoint
  ↓
Response Normalizer / Stream Relay
  ↓
Usage + Billing + Scheduler Feedback
```

### 6.3 支付回调调用链

```txt
Payment Webhook
  ↓
Payment Handler
  ↓
Payment Provider Verifier
  ↓
Payment Service
  ↓
Idempotency Check
  ↓
Order State Transition
  ↓
Billing / Subscription Update
  ↓
Audit Log
```

支付完成、退款、返利、账务和观测的后续处理应优先通过 `DOMAIN_EVENTS_SPEC.md` 的事件机制完成，避免 Payments 同步耦合 Billing、Affiliate、Observability 和通知模块。

### 6.4 异步任务调用链

```txt
Worker Scheduler
  ↓
Job Handler
  ↓
Module Service
  ↓
Repository / Provider Client
  ↓
Feedback / Audit / Metrics
```

## 7. 模块边界

### 7.1 Auth

负责控制台用户认证，不负责 API Key 网关认证。

拥有：

- 登录。
- 登出。
- Refresh token。
- Session。
- TOTP。

依赖：

- Users。
- Platform crypto。
- Redis。

### 7.2 API Keys

负责 API Key 生命周期和网关鉴权。

拥有：

- Key 创建。
- Key 哈希。
- Key 范围。
- Key 状态。
- Key 限额。

被 Gateway 使用。

### 7.3 Providers

负责 Provider 定义、能力声明和 Provider Adapter 注册。

不负责具体账号额度。

OpenAI-compatible / Anthropic-compatible 等兼容 Provider preset 的 route alias、默认 base URL、auth mode、模型目录和账号类型边界由 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` 约束。

### 7.4 Models

负责 Model Registry、模型别名、能力、价格和 fallback 关系。

Scheduler 和 Gateway 都依赖它。

### 7.5 Accounts

负责上游账号池：

- 账号凭证。
- 账号分组。
- 账号状态。
- 账号额度快照。
- 代理配置。

账号凭证必须通过 crypto 服务加密存储。

### 7.6 Gateway

负责协议兼容、端点互转、请求标准化、响应标准化和流式转发。

必须支持的客户端端点族：

```txt
OpenAI Chat Completions
OpenAI Responses
Anthropic Messages
Gemini GenerateContent compatible IR
```

Gateway 内部必须把所有客户端端点转换为 Canonical AI Request，再交给 Scheduler。

Gateway 不应该做复杂账号选择，只调用 Scheduler。

对于 2api / 反代账号，Gateway 必须经由 Provider Adapter 下的 Reverse Proxy Runtime 发起上游请求，详见 `REVERSE_PROXY_SPEC.md`。

Gateway 对外路由族、Provider-prefixed alias、passthrough、WebSocket 和阶段边界以 `GATEWAY_ROUTE_MATRIX.md` 为准。

### 7.7 Scheduler

负责候选账号构建、策略过滤、评分、Lease、反馈。

Scheduler 只能依赖 Provider capability 和 account runtime state，不直接调用上游。

Scheduler 不得依赖客户端源端点名称，只能基于 Canonical AI Request 的能力需求、模型能力、价格、权益和账号运行时状态做决策。

### 7.8 Billing

负责计费、余额、账本、扣费和成本统计。

Billing 不应该参与账号选择，但可以向 Scheduler 提供用户余额、成本策略和套餐等级。

### 7.9 Subscriptions

负责套餐与用户权益。

向 API Keys、Billing、Scheduler 提供 entitlement 查询。

### 7.10 Payments

负责订单、支付渠道、Webhook 和退款。

支付完成后通过服务接口更新 Billing 或 Subscription。

支付渠道、多实例、订单状态机、Webhook、退款和幂等以 `PAYMENT_SPEC.md` 为准。

### 7.10A Affiliate

负责邀请关系、返利规则、返利账本、退款补偿和转余额。

Affiliate 不直接修改支付订单状态；支付退款发生后由 Payments 或 Billing 触发返利补偿流程。

返利规则以 `AFFILIATE_REBATE_SPEC.md` 为准。

### 7.11 Audit

负责记录高风险操作和关键状态变更。

所有管理端写操作都应该可接入 Audit。

### 7.12 Observability

负责 metrics、trace、调度可视化数据和运行诊断。

不承载核心业务状态。

AI-native 指标、Ops Dashboard、SLO、Burn-rate 告警和 Provider 健康矩阵以 `OBSERVABILITY_SPEC.md` 为准。

## 8. 同步与异步边界

### 8.1 同步路径

必须同步完成：

- 用户鉴权。
- API Key 鉴权。
- 请求调度。
- Lease 获取。
- 上游请求。
- 流式转发。
- 请求结果基础记录。

### 8.2 可异步完成

可以异步完成：

- 用量聚合。
- 成本报表。
- 账号健康快照持久化。
- 调度决策长期归档。
- 邮件发送。
- Webhook 补偿。
- 备份。

### 8.3 必须保证最终一致

- Billing ledger。
- Usage logs。
- Payment order state。
- Subscription activation。
- Account quota snapshot。

## 9. 状态存储边界

### 9.1 PostgreSQL

作为真实来源：

- 用户。
- API Key 哈希。
- Provider 配置。
- Account 配置。
- Model Registry。
- Payment order。
- Billing ledger。
- Subscription。
- Audit log。
- Usage log。
- Scheduler decision。
- Scheduler feedback。
- Domain events outbox / inbox。

Domain Events Outbox 在 MVP 阶段使用数据库轮询 dispatcher：`pending` 事件成功后进入 `published`，失败后记录 `attempt_count`、`last_error` 和 `next_retry_at`，到期后可重试。未来替换为外部消息中间件时，不得改变事件 envelope 和 inbox 幂等语义。

### 9.2 Redis

作为运行时缓存：

- API Key auth cache。
- Rate limit counters。
- Account concurrency。
- Lease。
- Circuit breaker state。
- Sticky binding cache。
- Cache affinity cache。
- Short-term health window。

Redis 数据必须可从 PostgreSQL 或运行时反馈重建。
release 模式下 Scheduler Lease 使用 Redis-backed store；本地开发在 Redis 不可用时可降级为内存 lease，但 `/readyz` 仍应标记 Redis 不可用。

## 10. 错误处理边界

所有模块错误应映射为统一错误结构：

```txt
code
message
http_status
details
trace_id
```

模块内部使用 typed error，HTTP 层统一渲染。

Gateway 上游错误需要转换为：

- 对客户端兼容的 OpenAI-style error。
- 对内部可观测的 ProviderErrorClass。

## 11. 事务边界

建议事务只包裹单一业务一致性边界。

示例：

- 创建订单。
- 支付回调更新订单和账本。
- 创建 API Key。
- 提交 usage 和 billing ledger。

不建议把上游 AI 请求放进数据库事务。

## 12. 扩展原则

新增 Provider：

- 增加 Provider Adapter。
- 增加模型映射。
- 增加错误分类器。
- 增加 usage parser。
- 通过 `CAPABILITY_TAXONOMY_SPEC.md` 声明 Provider / Model / Endpoint capability。
- 不修改 Scheduler 核心流程。

新增客户端端点协议：

- 增加 Client Endpoint Adapter。
- 增加 Canonical AI Request / Response 转换测试。
- 不修改 Scheduler 核心流程。
- 不修改 Provider Adapter 的账号选择逻辑。

新增调度策略：

- 增加 Strategy 实现或配置。
- 增加 strategy descriptor、配置 schema、版本和 dry-run 测试，详见 `SCHEDULER_STRATEGY_EXTENSION_SPEC.md`。
- 不修改 Gateway。
- 不修改 Provider Adapter。

新增支付渠道：

- 增加 Payment Provider。
- 不修改订单状态机核心。

新增模型能力：

- 扩展 Model Capability。
- 扩展 Provider Capability。
- 使用 `CAPABILITY_TAXONOMY_SPEC.md` 的 capability key、version、status 和 downgrade 规则。
- 保持旧字段兼容。

新增跨模块协作：

- 优先增加 contract 或 domain event。
- 不允许直接依赖其他模块 repository、handler 或 Ent query。
- contract 规则以 `MODULE_INTERFACE_CONTRACTS.md` 为准。
- 事件、重试、死信和补偿以 `DOMAIN_EVENTS_SPEC.md` 为准。

## 13. 架构约束

- Provider-specific 逻辑不得进入 Scheduler score core。
- Client endpoint-specific 逻辑不得进入 Scheduler score core。
- Gateway 不得直接选择账号。
- Billing 不得直接调用 Provider。
- 跨模块调用不得绕过 module contract。
- 可最终一致的跨模块副作用不得强行放进同步主链路。
- Payment webhook 必须幂等。
- API Key 原文不得持久化。
- 上游凭证、cookie、OAuth token 不得明文存储。
- OpenAPI 生成代码不得手改。
- Redis 不得作为唯一真实来源。
- 所有后台高风险操作必须进入 Audit。
- 反代请求向上游发出时不得包含任何 SRapi 自有标识，详见 `REVERSE_PROXY_SPEC.md`。

## 14. 第一阶段架构完成标准

- 应用可以本地启动。
- HTTP server、config、logger、db、redis 初始化完成。
- OpenAPI 生成链路可运行。
- Auth、API Keys、Providers、Models、Accounts、Gateway、Scheduler 有基础模块目录。
- Gateway 能调用 Scheduler。
- Scheduler 能返回可解释 decision。
- Provider Adapter registry 可注册至少一个 OpenAI-compatible adapter。
- Usage log 和 scheduler decision 可持久化。
