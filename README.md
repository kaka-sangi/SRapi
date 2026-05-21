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
| [后端架构设计](docs/ARCHITECTURE.md) | 后端模块边界、依赖方向和调用链 | 是 |
| [领域模型](docs/DOMAIN_MODEL.md) | 核心业务概念和术语边界 | 是 |
| [OpenAPI 契约规范](docs/OPENAPI_CONTRACT.md) | HTTP 契约、错误、鉴权、分页和 codegen 规则 | 是 |
| [AI 端点兼容与转换规范](docs/AI_ENDPOINT_COMPATIBILITY.md) | Chat Completions、Responses、Messages、Gemini 等端点互转与 Canonical AI IR | 是 |
| [数据模型设计](docs/DATA_MODEL.md) | PostgreSQL 表、索引、一致性和加密字段 | 是 |
| [安全模型](docs/SECURITY_MODEL.md) | API Key、Cookie、CSRF、Provider 凭证、日志和审计要求 | 是 |
| [调度内核专项设计](docs/SCHEDULING_KERNEL_DESIGN.md) | 调度内核总体设计和长期演进模型 | 是 |
| [Scheduler v1 实现级规格](docs/SCHEDULER_V1_SPEC.md) | MVP 调度过滤、打分、Lease、Decision 和 Feedback 规则 | 是 |
| [调度场景矩阵](docs/SCHEDULING_SCENARIOS.md) | Scheduler 单元测试、集成测试和模拟器场景 | 是 |
| [Provider Adapter 规范](docs/PROVIDER_ADAPTER_SPEC.md) | Provider 扩展、错误分类、usage 和流式解析规范 | 是 |
| [反代运行时与去特征规范](docs/REVERSE_PROXY_SPEC.md) | 2api 反代、TLS / HTTP/2 / Header 指纹、cookie / OAuth 凭证、反封号策略 | 是 |
| [Gateway 路由矩阵](docs/GATEWAY_ROUTE_MATRIX.md) | Gateway 路由族、Provider alias、passthrough、WebSocket 和阶段规划 | 是 |
| [兼容 Provider 注册表规范](docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md) | OpenAI-compatible / Anthropic-compatible preset、base URL、auth mode、模型目录和 route alias | 是 |
| [配置与环境变量规范](docs/CONFIGURATION_SPEC.md) | 配置优先级、环境变量、默认值和生产安全约束 | 是 |
| [运维与生产治理规范](docs/OPERATIONS.md) | 迁移、备份、健康检查、发布、数据生命周期和事故处理 | 生产部署必读 |
| [Observability 与告警规格](docs/OBSERVABILITY_SPEC.md) | AI-native 指标、Ops Dashboard、SLO、Burn-rate 告警和 Provider 健康矩阵 | 运维开发必读 |
| [支付系统规格](docs/PAYMENT_SPEC.md) | 支付渠道、多实例、订单状态机、Webhook、退款和幂等 | 商业化开发必读 |
| [邀请返利规格](docs/AFFILIATE_REBATE_SPEC.md) | 邀请关系、返利账本、退款补偿、转余额和风控 | 商业化开发必读 |
| [前端设计系统与视觉规范](docs/FRONTEND_DESIGN_SYSTEM.md) | 控制台视觉、组件、动效和响应式约束 | 前端开发必读 |

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
