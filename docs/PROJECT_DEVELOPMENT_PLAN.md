# SRapi 完整项目开发方案

## 1. 项目定位

SRapi 是一个面向 AI API 聚合、账号池管理、智能调度、计费订阅和企业级控制台的自托管平台。

它的核心价值不是简单转发请求，而是在多个 AI 服务商、多个账号、多个模型、不同用户等级和不同成本结构之间，提供稳定、可观测、可计费、可扩展的统一入口。

## 2. 产品目标

### 2.1 核心目标

- 端点兼容与转换优先，所有 AI 调用先进入 Canonical AI Request，再由 Provider Adapter 转换到目标上游协议。
- 统一接入 OpenAI、Anthropic、Gemini、Grok、OpenRouter、OpenAI-compatible、Anthropic-compatible 等服务商。
- 为用户提供兼容 OpenAI API 风格的统一调用入口。
- 为用户提供 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 等主流 AI 端点，并支持这些端点之间的相互转换。
- 为管理员提供现代化 Web 控制台管理账号、分组、模型、密钥、订阅、支付、用量和系统策略。
- 构建独立的自适应 AI 调度内核，在账号额度、可用性、会话粘度、缓存亲和和成本之间动态平衡。
- 使用 OpenAPI-first 开发方式，让前后端类型、接口、SDK 和文档保持一致。
- 保持高度模块化，使未来 AI 服务商、模型能力、缓存机制、支付渠道和调度策略都能独立扩展。

### 2.2 非目标

第一阶段不追求：

- 复杂微服务拆分。
- Kubernetes 原生 Operator。
- 机器学习级别的自动调度训练。
- 多租户 SaaS 商业化全套能力。
- 极复杂策略 DSL。

第一阶段优先做成稳定、清晰、可扩展的模块化单体。

## 3. 整体技术栈

### 3.1 前端

- Next.js
- React
- TypeScript
- Tailwind CSS
- shadcn/ui
- Framer Motion
- TanStack Query
- OpenAPI generated client
- lucide-react
- next-intl，可选
- zod，可选，用于前端额外表单校验

### 3.2 后端

推荐主线：

- Go
- chi 或 Gin
- oapi-codegen
- Ent
- Atlas migrations
- PostgreSQL
- Redis
- Zap 或 slog
- OpenTelemetry
- Prometheus
- Docker Compose

路由建议：

- 如果追求标准库生态与 OpenAPI 契合：优先 `chi`。
- 如果希望参考 sub2api 已有实践：可以选择 `gin`。

最终推荐：

```txt
Go + chi + oapi-codegen + Ent + PostgreSQL + Redis
```

## 4. 仓库结构

建议使用前后端分离的 monorepo：

```txt
SRapi/
├── apps/
│   ├── web/
│   │   ├── app/
│   │   ├── components/
│   │   ├── features/
│   │   ├── lib/
│   │   ├── hooks/
│   │   ├── styles/
│   │   └── package.json
│   └── api/
│       ├── cmd/
│       │   └── srapi/
│       ├── internal/
│       │   ├── app/
│       │   ├── config/
│       │   ├── http/
│       │   ├── modules/
│       │   ├── platform/
│       │   ├── openapi/
│       │   └── worker/
│       ├── ent/
│       ├── migrations/
│       ├── openapi.yaml
│       ├── go.mod
│       └── Dockerfile
├── packages/
│   ├── openapi/
│   └── sdk/
├── docs/
├── deploy/
│   ├── docker-compose.yml
│   ├── nginx/
│   └── caddy/
└── tools/
```

## 5. 前端设计方案

### 5.1 视觉风格

前端采用 Claude 与 ChatGPT 结合的卡片式产品风格。

关键词：

- 温暖中性色
- 克制高级感
- 大留白
- 卡片式信息组织
- 轻量阴影
- 柔和边框
- 动效克制
- 避免传统后台管理系统的密集压迫感

### 5.2 色彩建议

```txt
背景色：#FAF9F5 / #F7F5EF
卡片色：#FFFFFF / #FDFCF8
主文本：#1F1F1D
次文本：#6F6A60
边框色：#E7E1D6
强调色：#D97706 / #B45309
成功色：#16A34A
警告色：#D97706
错误色：#DC2626
```

### 5.3 页面结构

```txt
apps/web/app/
├── page.tsx                         # Landing
├── login/page.tsx
├── dashboard/page.tsx
├── dashboard/api-keys/page.tsx
├── dashboard/usage/page.tsx
├── dashboard/billing/page.tsx
├── dashboard/subscriptions/page.tsx
├── admin/page.tsx
├── admin/users/page.tsx
├── admin/accounts/page.tsx
├── admin/models/page.tsx
├── admin/providers/page.tsx
├── admin/scheduler/page.tsx
├── admin/payments/page.tsx
├── admin/settings/page.tsx
└── gateway/docs/page.tsx
```

### 5.4 前端功能模块

```txt
features/
├── auth/
├── dashboard/
├── api-keys/
├── usage/
├── billing/
├── subscriptions/
├── admin-users/
├── admin-accounts/
├── admin-models/
├── admin-providers/
├── admin-scheduler/
├── admin-payments/
└── settings/
```

### 5.5 核心组件

```txt
components/
├── layout/
│   ├── app-shell.tsx
│   ├── sidebar.tsx
│   ├── topbar.tsx
│   └── command-menu.tsx
├── cards/
│   ├── metric-card.tsx
│   ├── provider-card.tsx
│   ├── model-card.tsx
│   ├── account-health-card.tsx
│   └── plan-card.tsx
├── charts/
├── tables/
├── empty-states/
└── motion/
```

### 5.6 状态与请求

- Server Components 用于静态和首屏数据。
- TanStack Query 用于客户端可变数据、轮询和乐观更新。
- OpenAPI generated client 作为唯一 API 调用入口。
- 所有 API 请求错误统一进入 toast、dialog 或 inline error。

## 6. 后端架构方案

后端采用模块化单体，内部使用领域模块拆分。

```txt
internal/
├── app/
│   ├── bootstrap.go
│   ├── lifecycle.go
│   └── dependencies.go
├── config/
├── http/
│   ├── server.go
│   ├── router.go
│   ├── middleware/
│   └── response/
├── modules/
│   ├── auth/
│   ├── users/
│   ├── api_keys/
│   ├── providers/
│   ├── models/
│   ├── accounts/
│   ├── gateway/
│   ├── scheduler/
│   ├── billing/
│   ├── subscriptions/
│   ├── payments/
│   ├── admin/
│   ├── audit/
│   └── observability/
├── platform/
│   ├── db/
│   ├── redis/
│   ├── logger/
│   ├── crypto/
│   ├── mailer/
│   ├── httpclient/
│   └── objectstore/
├── openapi/
└── worker/
```

### 6.1 模块内标准结构

每个模块尽量保持相似结构：

```txt
modules/example/
├── handler.go
├── service.go
├── repository.go
├── ent_repository.go
├── model.go
├── dto.go
├── errors.go
├── policy.go
└── service_test.go
```

职责：

- `handler.go`：HTTP 协议、鉴权、参数、响应。
- `service.go`：业务逻辑。
- `repository.go`：数据访问接口。
- `ent_repository.go`：Ent 实现。
- `model.go`：领域模型。
- `dto.go`：模块内部 DTO，优先复用 OpenAPI 生成类型。
- `errors.go`：模块错误码。
- `policy.go`：业务策略。

## 7. OpenAPI-first 工作流

SRapi 必须从第一天开始使用 OpenAPI 契约驱动开发。

流程：

```txt
设计 openapi.yaml
  ↓
生成 Go server interfaces/types
  ↓
后端实现接口
  ↓
生成 TypeScript client
  ↓
前端用 TanStack Query 调用
  ↓
CI 校验契约无破坏性变更
```

建议工具：

- 后端：`oapi-codegen`
- 前端：`@hey-api/openapi-ts` 或 `orval`
- 文档：Scalar 或 Swagger UI

OpenAPI 文件组织：

```txt
packages/openapi/
├── openapi.yaml
├── paths/
│   ├── auth.yaml
│   ├── users.yaml
│   ├── api-keys.yaml
│   ├── providers.yaml
│   ├── models.yaml
│   ├── accounts.yaml
│   ├── gateway.yaml
│   ├── scheduler.yaml
│   ├── billing.yaml
│   ├── subscriptions.yaml
│   └── payments.yaml
└── schemas/
```

## 8. 核心业务模块

### 8.1 Auth 模块

能力：

- 用户注册
- 登录
- 登出
- Refresh Token
- 邮箱验证
- TOTP，可选
- 管理员登录
- CSRF 防护
- Session 管理

推荐策略：

- 控制台使用 HttpOnly Cookie。
- API Key 网关使用 `Authorization: Bearer sk-xxx`。
- 管理操作要求 RBAC。

### 8.2 Users 模块

能力：

- 用户资料
- 状态管理
- 余额
- 角色
- 权限
- 所属分组
- 用量概览

### 8.3 API Keys 模块

能力：

- 创建 API Key
- 删除 API Key
- 启用/禁用
- Key 哈希存储
- Key 权限
- Key 模型范围
- Key RPM/TPM 限制
- Key 用量统计

### 8.4 Provider 模块

能力：

- 服务商注册
- 服务商配置
- 服务商健康状态
- 服务商级别限流
- 错误分类规则
- Provider Adapter 管理

### 8.5 Model Registry 模块

能力：

- 模型注册
- 模型能力描述
- 模型别名映射
- 模型替代关系
- 上下文长度
- 支持能力
- 价格配置
- 质量等级

模型能力字段：

```txt
model_name
provider
context_window
max_output_tokens
supports_stream
supports_tools
supports_vision
supports_json
supports_cache
input_price
output_price
cache_read_price
cache_write_price
quality_tier
fallback_models
```

上述 `supports_*` 字段只是管理端 DTO 形态；真实能力注册、版本、降级和匹配规则必须以 `CAPABILITY_TAXONOMY_SPEC.md` 的 capability descriptor 为准。

### 8.6 Accounts 模块

能力：

- 上游账号管理
- API Key 型账号
- OAuth 型账号
- Cookie / Session 型账号
- 代理配置
- 分组绑定
- 权重配置
- 健康状态
- 额度快照
- 并发状态
- 冷却状态

### 8.7 Gateway 模块

能力：

- OpenAI-compatible `/v1/chat/completions`
- OpenAI-compatible `/v1/responses`
- Anthropic-compatible `/v1/messages`
- `/v1/models`
- Embeddings，可选
- Images，可选
- Anthropic-compatible，可选
- 多端点互转：Chat Completions / Responses / Messages
- 流式响应
- 请求标准化
- 响应标准化
- 错误标准化
- 用量提取

### 8.8 Scheduler 模块

能力：

- 候选账号构建
- 账号硬过滤
- 多目标评分
- Lease 管理
- 会话粘度
- 缓存亲和
- 成本估算
- 健康反馈
- 熔断降级
- 策略注册

该模块是 SRapi 的核心，详见 `SCHEDULING_KERNEL_DESIGN.md`。

### 8.9 Billing 模块

能力：

- 余额管理
- 用量扣费
- 价格计算
- 账本流水
- 欠费处理
- 用量聚合
- 用户组费率

建议使用 ledger 账本模型，避免只维护余额字段。

### 8.10 Subscriptions 模块

能力：

- 套餐管理
- 用户订阅
- 到期时间
- 日/周/月限额
- 权益解析
- 订阅状态
- 订阅续期

### 8.11 Payments 模块

能力：

- 支付配置
- 支付渠道
- 订单创建
- Webhook 回调
- 退款
- 审计日志

支付 Provider 接口：

```txt
Stripe
Alipay
Wechat Pay
Manual
```

### 8.12 Observability 模块

能力：

- 请求日志
- Trace ID
- Prometheus metrics
- 调度决策日志
- 账号健康面板
- 错误分类统计
- 成本统计
- 延迟统计

## 9. 调度内核总览

调度内核的目标是：

```txt
在账号额度、账号可用性、会话粘度、缓存亲和、成本控制、用户体验之间寻找动态最优解。
```

核心流程：

```txt
客户端端点请求
  ↓
端点适配为 Canonical AI Request
  ↓
请求分类
  ↓
能力解析
  ↓
候选账号构建
  ↓
硬过滤
  ↓
多维打分
  ↓
账号选择
  ↓
Lease 预占
  ↓
上游请求
  ↓
成功/失败反馈
  ↓
更新健康、额度、缓存和账单
```

内置策略：

- Balanced
- Quality First
- Cost Saver
- Cache Affinity
- Low Latency

## 10. 数据库设计总览

核心表：

```txt
users
roles
user_roles
api_keys
providers
provider_accounts
account_groups
account_group_members
model_registry
model_provider_mappings
account_health_snapshots
account_quota_snapshots
scheduler_decisions
scheduler_feedbacks
sticky_sessions
cache_affinity_records
usage_logs
billing_ledger
subscription_plans
user_subscriptions
payment_orders
payment_provider_instances
audit_logs
settings
announcements
```

第一阶段不需要一次性全部实现，但 schema 设计要预留扩展空间。

## 11. Redis 设计总览

Redis 用于高频运行时状态。

```txt
gateway:api_key:{hash}:auth
gateway:user:{id}:quota
scheduler:account:{id}:health
scheduler:account:{id}:quota
scheduler:account:{id}:rpm
scheduler:account:{id}:tpm
scheduler:account:{id}:concurrency
scheduler:account:{id}:circuit
scheduler:sticky:{conversation_hash}
scheduler:cache:{prompt_hash}
scheduler:lease:{request_id}
scheduler:provider:{name}:health
```

Redis 数据应该可以重建，不能成为唯一真实来源。

## 12. 安全设计

### 12.1 API Key 安全

- 原文只展示一次。
- 数据库只保存哈希。
- 支持 key 前缀用于快速定位。
- 支持禁用、过期、权限范围。

### 12.2 上游账号安全

- OAuth Token、Cookie、API Key 必须加密存储。
- 加密密钥从环境变量或密钥管理系统读取。
- 管理后台默认隐藏敏感字段。
- 导出备份时敏感字段需二次保护。

### 12.3 管理后台安全

- HttpOnly Cookie。
- CSRF 防护。
- RBAC。
- 审计日志。
- 高风险操作二次确认。

## 13. 可观测性设计

必须记录：

- 每次网关请求的 request id。
- 用户 id / API key id。
- 模型。
- Provider。
- Account。
- 调度策略。
- 调度分数。
- 延迟。
- token 使用量。
- 费用。
- 错误分类。
- 是否命中会话粘度。
- 是否命中缓存亲和。

核心指标：

```txt
requests_total
requests_success_total
requests_error_total
request_latency_ms
provider_error_total
account_circuit_open_total
scheduler_decision_total
scheduler_fallback_total
usage_input_tokens_total
usage_output_tokens_total
billing_cost_total
cache_affinity_hit_total
sticky_session_hit_total
```

## 14. 测试策略

### 14.1 后端测试

- Service 单元测试。
- Repository 集成测试。
- Scheduler 算法测试。
- Gateway 流式响应测试。
- Payment webhook 测试。
- OpenAPI 合约测试。

### 14.2 前端测试

- 关键组件测试。
- 表单交互测试。
- API mock 测试。
- Dashboard smoke test。
- 管理后台关键流程 E2E。

### 14.3 调度内核专项测试

需要构造场景：

- 高额度账号优先但不能被耗尽。
- 低健康账号被降权。
- 粘性账号故障时自动切换。
- 长上下文请求优先缓存亲和。
- 低价用户优先成本策略。
- 高级用户优先成功率。
- 并发租约不超限。

## 15. 开发阶段路线

### Phase 0：项目骨架与契约

目标：建立可运行的前后端分离骨架。

交付：

- Monorepo 目录结构
- Next.js 项目
- Go API 项目
- OpenAPI 初始契约
- Docker Compose
- PostgreSQL + Redis
- 基础 CI 脚本

### Phase 1：账号、模型、API Key、基础网关

目标：实现最小可用 API Gateway。

交付：

- 用户登录
- API Key 管理
- Provider 管理
- Model Registry
- Account Pool
- OpenAI-compatible `/v1/chat/completions`
- 基础转发
- 基础用量记录

### Phase 2：调度内核 v1

目标：实现可配置的账号选择和反馈闭环。

交付：

- CandidateBuilder
- PolicyFilter
- ScoreEngine
- LeaseManager
- Health feedback
- Quota feedback
- Sticky session soft binding
- Scheduler decision log
- 管理后台调度面板

### Phase 3：计费、订阅、支付

目标：形成商业闭环。

交付：

- 用户余额
- Billing ledger
- Subscription plans
- User subscriptions
- Payment orders
- Stripe / Manual payment 起步
- 支付回调
- 订单审计

### Phase 4：缓存亲和与成本优化

目标：调度内核开始体现 SRapi 差异化价值。

交付：

- CacheAffinityManager
- Prompt prefix hash
- Provider cache cost model
- 长上下文成本估算
- Cost Saver 策略
- Cache Affinity 策略
- 调度收益报表

### Phase 5：高级治理与生态扩展

目标：支持更多服务商和企业级运营能力。

交付：

- 多 Provider Adapter
- Provider 级熔断
- 代理池
- 账号风险评分
- 策略模板
- 审计中心
- 备份恢复
- 管理员操作日志

## 16. MVP 范围建议

第一版 MVP 不要贪大，建议只做：

- 一个后端 API 服务
- 一个 Next.js 控制台
- PostgreSQL
- Redis
- OpenAI-compatible Provider
- Anthropic Provider，可选
- OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 端点兼容与互转
- API Key
- Account Pool
- Model Registry
- Scheduler v1
- Usage logs
- Dashboard 基础数据卡片

MVP 最重要的不是功能多，而是把架构骨架、OpenAPI 流程和调度内核接口打稳。

## 17. 风险与对策

### 17.1 Provider 变化快

对策：Provider Adapter 插件化，模型能力动态配置。

### 17.2 调度复杂度失控

对策：先硬过滤 + 评分公式，不急于做复杂机器学习。

### 17.3 成本计算不准

对策：先预估，后用真实 usage 反馈修正。

### 17.4 流式响应异常处理复杂

对策：网关层统一 stream abstraction，记录 partial failure。

### 17.5 前后端接口漂移

对策：OpenAPI-first，CI 校验生成产物。

## 18. 开发原则

- 先契约，后实现。
- 先模块边界，后功能堆叠。
- 先可观测，后优化。
- 先可配置，后自动化。
- 先单体模块化，后必要时拆服务。
- 任何 Provider 特性不能污染核心领域模型。
- 调度内核必须保持 Provider-neutral。
- 高频状态进 Redis，真实来源进 PostgreSQL。
- 敏感信息默认加密。
- 所有高风险管理操作必须审计。

## 19. 下一步落地顺序

建议接下来按以下顺序继续：

1. 以 `MVP_SPEC.md` 确认 MVP 需求、验收条件和测试映射。
2. 以 `SECURITY_MODEL.md` 确认 API Key、Provider 凭证、Cookie、CSRF、日志和审计边界。
3. 以 `SCHEDULER_V1_SPEC.md` 确认 Scheduler v1 过滤、打分、Lease、Decision 和 Feedback 规则。
4. 以 `AI_ENDPOINT_COMPATIBILITY.md` 确认 Chat Completions、Responses、Messages 与 Canonical AI Request 转换规则。
5. 创建 monorepo 真实代码骨架。
6. 初始化 `apps/web` Next.js 项目。
7. 初始化 `apps/api` Go 项目。
8. 编写第一版 `packages/openapi/openapi.yaml`。
9. 实现 Auth、API Key、Provider、Model、Account 基础接口。
10. 实现 Gateway 多端点兼容和转换最小闭环。
11. 实现 Scheduler v1 的接口与测试。
12. 接入前端 Dashboard 卡片式页面。

## 20. 项目成功标准

第一阶段成功标准：

- 本地一条命令启动前端、后端、PostgreSQL、Redis。
- 用户可以登录控制台。
- 管理员可以添加 Provider Account。
- 用户可以创建 API Key。
- 客户端可以通过 OpenAI-compatible API 调用模型。
- 客户端可以通过 Chat Completions、Responses 或 Messages API 调用模型，并由 SRapi 自动转换到可用上游协议。
- 调度内核会记录每次选择原因。
- 管理后台能看到账号健康、额度、水位、请求量和成本。
- OpenAPI 可以生成前端调用代码。
