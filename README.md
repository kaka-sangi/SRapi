# SRapi

SRapi 是一个面向 AI 快速迭代时代的自托管 AI API Gateway 与管理平台。

项目目标不是简单代理多个 AI API，而是构建一个具备长期演进能力的统一平台：

- 多服务商、多模型统一接入
- 高度模块化、可插拔、可扩展的后端架构
- OpenAPI-first 的前后端协作流程
- OpenAI Responses、Chat Completions、Anthropic Messages 等主流 AI 端点兼容与相互转换
- 兼容 sub2api / claude2api / chatgpt2api / gemini2api / grok2api / cursor2api / antigravity2api 等 2api 反代运行时
- 通过完整 TLS / HTTP/2 / Header / 行为指纹模拟，最大化降低账号封禁风险
- Claude / ChatGPT 风格的现代卡片式控制台
- 面向账号额度、可用性、会话粘度、缓存亲和、成本控制的自适应调度内核

## 当前状态

当前目录已初始化为 SRapi 新项目规划目录，优先落地架构与开发方案。

当前可运行工程件：

- `packages/openapi/openapi.yaml`：M1 首批接口契约，可 lint / bundle。
- `packages/openapi/oapi-codegen.server.yaml`：Go OpenAPI types/server interface 生成配置。
- `apps/api`：Go API 最小骨架，提供 `/livez`、`/readyz`、`/api/v1/health`。
- `apps/api/internal/app`：启动编排与 HTTP server 组装层。
- `apps/api/internal/platform/db` / `redis` / `logger` / `crypto`：启动期平台连接、日志与密钥派生基础抽象。
- `apps/api/ent/schema`：MVP 最小数据表集合的 Ent schema，可生成 Ent client。
- `apps/api/internal/platform/db`：PostgreSQL 启动 client 与空库迁移应用门禁。
- `apps/api/internal/platform/redis`：Redis 启动 client 与健康探测基础抽象。
- `apps/api/internal/persistence/entstore`：users / API keys / providers / models / accounts 的 Ent-backed repository，可通过 contract 注入运行时。
- `apps/api/internal/modules/providers/preset`：兼容 Provider preset 注册表骨架。
- `apps/api/migrations`：保留 Atlas / Ent 版本化迁移目录。
- `deploy/docker-compose.yml`：本地 PostgreSQL、Redis、API 启动编排。
- `.env.example` 与 `Makefile`：本地启动和质量检查入口。

后续阶段将按文档逐步生成：

```txt
SRapi/
├── apps/
│   ├── web/                  # Next.js 前端
│   └── api/                  # Go 后端
├── packages/
│   ├── openapi/              # OpenAPI 契约
│   └── sdk/                  # 生成的 TypeScript SDK，可选
├── docs/                     # 架构与开发文档
├── deploy/                   # Docker Compose / Nginx / Caddy 部署配置
└── tools/                    # 代码生成、检查、开发工具脚本
```

## 文档

| 文档 | 作用 | 实现前是否必须读 |
| --- | --- | --- |
| [完整项目开发方案](docs/PROJECT_DEVELOPMENT_PLAN.md) | 项目全局路线图与阶段规划 | 是 |
| [MVP 实现级规格](docs/MVP_SPEC.md) | MVP 功能需求、非功能需求、验收条件和测试映射 | 是 |
| [Codex 执行规格](specs/README.md) | 长期 Codex goal 的任务切片、状态记录、质量门禁和最终形态约束 | Codex 开发必读 |
| [后端架构设计](docs/ARCHITECTURE.md) | 后端模块边界、依赖方向和调用链 | 是 |
| [模块接口契约规范](docs/MODULE_INTERFACE_CONTRACTS.md) | 跨模块 contract、DTO、同步调用、事件边界和测试规则 | 是 |
| [领域事件规范](docs/DOMAIN_EVENTS_SPEC.md) | 领域事件、Outbox、Inbox、幂等、重试、死信和补偿 | 是 |
| [领域模型](docs/DOMAIN_MODEL.md) | 核心业务概念和术语边界 | 是 |
| [OpenAPI 契约规范](docs/OPENAPI_CONTRACT.md) | HTTP 契约、错误、鉴权、分页和 codegen 规则 | 是 |
| [AI 端点兼容与转换规范](docs/AI_ENDPOINT_COMPATIBILITY.md) | Chat Completions、Responses、Messages、Gemini 等端点互转与 Canonical AI IR | 是 |
| [数据模型设计](docs/DATA_MODEL.md) | PostgreSQL 表、索引、一致性和加密字段 | 是 |
| [安全模型](docs/SECURITY_MODEL.md) | API Key、Cookie、CSRF、Provider 凭证、日志和审计要求 | 是 |
| [SDK 与 HTTP 示例](examples/README.md) | curl、TypeScript SDK 和 Python requests 的安全本地调用示例 | 集成开发必读 |
| [2api 迁移指南](docs/MIGRATION_GUIDE_2API.md) | sub2api / CLIProxyAPI / chatgpt2api 风格部署迁移到 SRapi 的账号、模型和反代边界映射 | 反代迁移必读 |
| [调度内核专项设计](docs/SCHEDULING_KERNEL_DESIGN.md) | 调度内核总体设计和长期演进模型 | 是 |
| [Scheduler v1 实现级规格](docs/SCHEDULER_V1_SPEC.md) | MVP 调度过滤、打分、Lease、Decision 和 Feedback 规则 | 是 |
| [Scheduler 策略扩展规范](docs/SCHEDULER_STRATEGY_EXTENSION_SPEC.md) | 策略注册、版本、灰度、dry-run、shadow decision 和回滚规则 | 是 |
| [调度场景矩阵](docs/SCHEDULING_SCENARIOS.md) | Scheduler 单元测试、集成测试和模拟器场景 | 是 |
| [Provider Adapter 规范](docs/PROVIDER_ADAPTER_SPEC.md) | Provider 扩展、错误分类、usage 和流式解析规范 | 是 |
| [能力分类与版本化规范](docs/CAPABILITY_TAXONOMY_SPEC.md) | Request / Model / Provider / Endpoint capability 命名、匹配、降级和版本规则 | 是 |
| [反代运行时与去特征规范](docs/REVERSE_PROXY_SPEC.md) | 2api 反代、TLS / HTTP/2 / Header 指纹、cookie / OAuth 凭证、反封号策略 | 是 |
| [Gateway 路由矩阵](docs/GATEWAY_ROUTE_MATRIX.md) | Gateway 路由族、Provider alias、passthrough、WebSocket 和阶段规划 | 是 |
| [兼容 Provider 注册表规范](docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md) | OpenAI-compatible / Anthropic-compatible preset、base URL、auth mode、模型目录和 route alias | 是 |
| [配置与环境变量规范](docs/CONFIGURATION_SPEC.md) | 配置优先级、环境变量、默认值和生产安全约束 | 是 |
| [运维与生产治理规范](docs/OPERATIONS.md) | 迁移、备份、健康检查、发布、数据生命周期和事故处理 | 生产部署必读 |
| [Observability 与告警规格](docs/OBSERVABILITY_SPEC.md) | AI-native 指标、Ops Dashboard、SLO、Burn-rate 告警和 Provider 健康矩阵 | 运维开发必读 |
| [支付系统规格](docs/PAYMENT_SPEC.md) | 支付渠道、多实例、订单状态机、Webhook、退款和幂等 | 商业化开发必读 |
| [邀请返利规格](docs/AFFILIATE_REBATE_SPEC.md) | 邀请关系、返利账本、退款补偿、转余额和风控 | 商业化开发必读 |
| [前端设计系统与视觉规范](docs/FRONTEND_DESIGN_SYSTEM.md) | 控制台视觉、组件、动效和响应式约束 | 前端开发必读 |
| [产品基调与文案规范](docs/PRODUCT_TONE.md) | 产品定位、中英文语气规则、术语替换表（v0.1.0 起统一基调） | 任何 UI / 文档撰写必读 |

## 推荐技术方向

### 前端

- Next.js
- React
- TypeScript
- shadcn/ui
- Tailwind CSS
- Framer Motion
- TanStack Query
- OpenAPI generated client

### 后端

- Go
- chi 或 Gin
- oapi-codegen
- Ent + Atlas
- PostgreSQL
- Redis
- OpenTelemetry
- Prometheus

## 本地启动

准备环境：

```bash
make bootstrap-env
```

运行当前质量检查：

```bash
make check
```

当前 `make check` 覆盖 OpenAPI lint / bundle、Go OpenAPI codegen drift check、TypeScript SDK codegen drift check、TypeScript typecheck、Ent generate check、migration check、Go code quality、examples check、Go tests 和 secret scan。

架构门禁可单独运行：

```bash
make architecture-check
```

版本化迁移门禁可单独运行：

```bash
make migration-check
```

重新生成 OpenAPI Go 类型和 server interface：

```bash
make openapi-codegen
```

重新生成 OpenAPI TypeScript SDK：

```bash
make openapi-ts-codegen
```

重新生成 Ent client：

```bash
make ent-generate
```

仅启动 Go API：

```bash
make api-run
```

启动前端开发服务器：

```bash
make web-dev
```

前端质量门（typecheck / lint / 单测 + a11y / build）：

```bash
make web-check
```

前端 e2e（Playwright + Chromium + axe-core 全页 a11y）：

```bash
make web-check-e2e
```

启动 PostgreSQL、Redis 和 API：

```bash
make dev-up
```

健康检查：

```bash
make smoke-health
```

管理员登录、API Key 创建和本地 mock Gateway 端点 smoke test：

```bash
make smoke-gateway
```

发布前本地 smoke test：

```bash
make smoke-release
```

`make smoke-release` 会额外检查 `/livez`、`/readyz` 和 `/metrics` 基线指标，适合在 Docker Compose 或单机 release 配置启动后执行。

Stripe test mode 支付闭环 smoke：

```bash
STRIPE_SMOKE_SECRET_KEY=<stripe-test-secret-key> \
STRIPE_SMOKE_WEBHOOK_SECRET=<stripe-webhook-signing-secret> \
make smoke-payment-stripe
```

该 smoke 需要已启动的 API 和可用 Stripe test mode 凭证。它会创建或更新一个临时 Stripe provider instance，发起 Checkout Session，向 SRapi webhook 入口提交本地签名的 `checkout.session.completed` 事件，验证幂等和余额入账，最后禁用临时 provider。

Alipay Page Pay 支付 smoke：

```bash
ALIPAY_SMOKE_APP_ID=<alipay-app-id> \
ALIPAY_SMOKE_PRIVATE_KEY=<merchant-private-key-pem> \
ALIPAY_SMOKE_ALIPAY_PUBLIC_KEY=<alipay-public-key-pem> \
make smoke-payment-alipay
```

该 smoke 需要已启动的 API 和可用支付宝沙箱或测试商户凭证。默认路径会创建临时 Alipay provider instance 并验证 Page Pay RSA2 checkout URL；`ALIPAY_SMOKE_LOCAL_WEBHOOK=1` 可额外用本地签名通知验证 SRapi webhook、`success` 应答、履约、幂等和余额入账，但不能替代支付宝沙箱真实回调。

WeChat Pay APIv3 支付 smoke：

```bash
WECHAT_SMOKE_APP_ID=<wechat-app-id> \
WECHAT_SMOKE_MCH_ID=<wechat-merchant-id> \
WECHAT_SMOKE_API_V3_KEY=<32-byte-api-v3-key> \
WECHAT_SMOKE_SERIAL_NO=<merchant-certificate-serial> \
WECHAT_SMOKE_PRIVATE_KEY=<merchant-private-key-pem> \
make smoke-payment-wechat
```

该 smoke 需要已启动的 API、可用微信支付商户凭证和微信预支付网络连通性。默认 Native 路径会创建临时 WeChat provider instance，调用用户下单 API 发起真实预支付并验证返回的支付 metadata；`WECHAT_SMOKE_LOCAL_WEBHOOK=1` 可额外用本地签名的 APIv3 加密通知验证 SRapi webhook、履约、幂等和余额入账，但不能替代微信支付平台真实通知演练。

PostgreSQL 手动备份和恢复：

```bash
make backup-postgres BACKUP_FILE=backups/srapi.dump
make restore-postgres BACKUP_FILE=backups/srapi.dump
```

本地默认管理员由 `.env.example` 中的 `BOOTSTRAP_ADMIN_EMAIL` 和 `BOOTSTRAP_ADMIN_PASSWORD` 初始化。当前开发骨架会 seed 一个 OpenAI-compatible Provider、`gpt-4o-mini` 模型和本地 mock Provider Account，因此 `make smoke-gateway` 不需要真实上游 API Key。该 smoke 会覆盖 `/v1/models`、`/v1/chat/completions`、`/v1/responses` 和 `/v1/messages` 的最小本地闭环。

Windows PowerShell 基础入口：

```powershell
tools/dev.ps1 check
tools/dev.ps1 architecture-check
tools/dev.ps1 openapi
tools/dev.ps1 api
tools/dev.ps1 up
tools/dev.ps1 smoke-gateway
```

## 反代合规边界

SRapi 的 Reverse Proxy Runtime 只提供自托管运行时能力和安全隔离机制，不内置任何上游 ToS 绕过、验证码破解、cookie 抓取或 token 获取逻辑。部署者需要自行确认账号、地区、网络出口和自动化调用方式符合目标上游服务条款，并承担相应的合规和封号风险。

## 核心设计原则

- 模块化优先
- OpenAPI 契约优先
- 多 AI 端点兼容和协议互转优先
- 2api 反代运行时与去特征优先
- Provider 插件化
- 调度内核独立化
- 策略可配置
- 运行时可观测
- 默认安全
- 面向长期演进
