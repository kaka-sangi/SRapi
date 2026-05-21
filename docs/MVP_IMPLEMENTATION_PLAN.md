# SRapi MVP 实施计划

## 1. 目标

本文档把 SRapi 方案拆解为可执行的 MVP 里程碑。

MVP 目标：

```txt
建立一个可本地运行的前后端分离 AI Gateway，支持 API Key、Provider Account、Model Registry、OpenAI-compatible Chat Completions、OpenAI Responses、Anthropic Messages、端点互转、Scheduler v1、Usage Log 和基础管理 API。
```

MVP 不追求功能大而全，重点是把架构骨架、契约、数据模型和调度内核闭环打稳。

实现级需求、非功能需求、验收条件和测试映射以 `MVP_SPEC.md` 为准；本文档负责里程碑拆分和执行顺序。

## 2. MVP 范围

### 必须包含

- Monorepo 结构。
- Go API 服务。
- PostgreSQL。
- Redis。
- OpenAPI 契约。
- API Key 管理。
- Provider 管理。
- Model Registry。
- Provider Account 管理。
- OpenAI-compatible `/v1/models`。
- OpenAI-compatible `/v1/chat/completions`。
- OpenAI-compatible `/v1/responses`。
- Anthropic-compatible `/v1/messages`。
- Chat Completions / Responses / Messages 通过 Canonical AI Request 相互转换。
- Scheduler v1。
- Usage Log。
- Scheduler Decision。
- 基础管理员接口。

### 暂缓

- 支付。
- 订阅购买。
- 多支付渠道。
- 复杂 RBAC。
- 高级策略 DSL。
- 机器学习调度。
- Realtime API。
- Batch API。
- Fine-tuning API。
- Kubernetes 部署。

## 3. 里程碑总览

```txt
M0 文档与仓库骨架
M1 OpenAPI 契约基础
M2 后端基础设施
M3 数据模型与迁移
M4 Auth 与 API Key
M5 Provider / Model / Account
M6 Gateway 最小闭环
M7 Scheduler v1
M8 Usage / Observability
M9 前端接入准备接口
M10 本地部署与质量门禁
```

## 4. M0：文档与仓库骨架

### 目标

建立清晰项目结构。

### 交付

```txt
SRapi/
├── apps/api
├── apps/web
├── packages/openapi
├── docs
├── deploy
└── tools
```

### 完成标准

- 目录存在。
- README 指向核心文档。
- docs 包含架构、领域、OpenAPI、数据模型、安全模型、调度、Scheduler v1、Provider、MVP 实现级规格文档。

## 5. M1：OpenAPI 契约基础

### 目标

建立契约驱动开发流程。

### 交付

```txt
packages/openapi/openapi.yaml
packages/openapi/paths/*.yaml
packages/openapi/schemas/*.yaml
```

第一批接口：

```txt
GET  /api/v1/health
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/me
GET  /api/v1/api-keys
POST /api/v1/api-keys
GET  /api/v1/admin/providers
POST /api/v1/admin/providers
GET  /api/v1/admin/models
POST /api/v1/admin/models
GET  /api/v1/admin/accounts
POST /api/v1/admin/accounts
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
POST /v1/messages
```

### 测试

- OpenAPI lint。
- OpenAPI bundle。
- 生成 TypeScript client。
- 生成 Go server types。

### 完成标准

- 契约可生成代码。
- 生成代码不需要手改。

## 6. M2：后端基础设施

### 目标

Go API 服务可以启动，并连接基础设施。

### 交付

```txt
apps/api/cmd/srapi/main.go
apps/api/internal/app
apps/api/internal/config
apps/api/internal/http
apps/api/internal/platform/db
apps/api/internal/platform/redis
apps/api/internal/platform/logger
apps/api/internal/platform/crypto
```

### 能力

- 配置加载。
- Logger。
- HTTP server。
- Health check。
- Graceful shutdown。
- PostgreSQL connection。
- Redis connection。

### 测试

- `go test ./...`
- health endpoint test。

### 完成标准

- 本地能启动 API。
- `/api/v1/health` 返回 ok。

## 7. M3：数据模型与迁移

### 目标

建立 MVP 必需 schema。

### MVP 表

```txt
users
api_keys
providers
model_registry
model_aliases
model_provider_mappings
provider_accounts
account_groups
account_group_members
usage_logs
scheduler_decisions
scheduler_feedbacks
billing_ledger
settings
audit_logs
```

### 交付

```txt
apps/api/ent/schema/*.go
apps/api/migrations
```

### 测试

- Ent schema compile。
- Migration apply test。
- Repository integration test。

### 完成标准

- 数据库迁移可运行。
- Ent client 可生成。

## 8. M4：Auth 与 API Key

### 目标

支持基本用户与 API Key 调用凭证。

### 交付模块

```txt
modules/auth
modules/users
modules/api_keys
```

### 能力

- 创建初始管理员。
- 登录。
- 获取当前用户。
- 创建 API Key。
- API Key 哈希存储。
- API Key 鉴权 middleware。

### 测试

- 登录成功/失败。
- API Key 创建后只展示一次。
- API Key hash 校验。
- 禁用 key 后不可调用 Gateway。

### 完成标准

- 用户能创建 API Key。
- Gateway 能识别 `Authorization: Bearer sk-xxx`。

## 9. M5：Provider / Model / Account

### 目标

建立上游账号池和模型目录。

### 交付模块

```txt
modules/providers
modules/models
modules/accounts
```

### 能力

- 创建 Provider。
- 创建 Model。
- 创建 Model Alias。
- 创建 Provider Model Mapping。
- 创建 Provider Account。
- 加密保存 Provider Account credential。
- 测试账号连通性。

### MVP 默认数据

可提供 seed：

```txt
provider: openai-compatible
model: gpt-4o-mini 或 configurable
mapping: openai-compatible model mapping
```

### 测试

- Provider CRUD。
- Model CRUD。
- Account credential encryption。
- Account validation mock。

### 完成标准

- 管理员可配置一个 OpenAI-compatible 上游账号。

## 10. M6：Gateway 最小闭环

### 目标

客户端能通过 SRapi 调用上游模型。

### 交付模块

```txt
modules/gateway
modules/providers/adapters/openai_compatible
```

### 能力

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages`
- 非流式调用。
- 流式调用。
- Client Endpoint Adapter。
- Canonical AI Request / Response 转换。
- 源端点格式响应渲染。
- 请求超时。
- OpenAI-compatible 错误格式。
- Anthropic-compatible 错误格式。
- Provider usage 解析。

### 测试

- `/v1/models` 返回模型列表。
- 非流式 chat completion mock。
- 流式 chat completion mock。
- `/v1/responses` 转 Canonical AI Request，并渲染 Responses-compatible 响应。
- `/v1/messages` 转 Canonical AI Request，并渲染 Anthropic Messages-compatible 响应。
- Chat Completions / Responses / Messages golden conversion tests。
- 无法无损转换时返回 compatibility warning 或明确错误。
- 上游错误转换。

### 完成标准

- 使用 OpenAI SDK 指向 SRapi base URL 可以发起请求。

## 11. M7：Scheduler v1

### 目标

实现账号选择的最小可解释闭环。

### 交付模块

```txt
modules/scheduler
```

### 组件

- RequestClassifier。
- CandidateBuilder。
- PolicyFilter。
- ScoreEngine。
- LeaseManager。
- FeedbackCollector。
- StrategyRegistry。

### 策略

MVP 先实现：

```txt
balanced
cost_saver
```

### 能力

- 账号硬过滤。
- 健康分基础计算。
- 额度分基础计算。
- 成本分基础计算。
- Soft sticky。
- Lease 并发控制。
- Scheduler Decision 持久化。
- Scheduler Feedback 持久化。

### 测试

至少覆盖 `SCHEDULING_SCENARIOS.md` 最小集合。

### 完成标准

- 每次 Gateway 请求都有 decision。
- 无可用账号时返回明确错误。
- 并发不会突破账号限制。

## 12. M8：Usage / Observability

### 目标

记录请求用量和可观测基础数据。

### 交付

```txt
modules/observability
modules/billing minimal usage charge hook
```

### 能力

- Usage Log。
- Request ID。
- Provider error class。
- Latency。
- Token usage。
- Basic metrics。

### 暂缓

- 完整账单系统。
- 支付。
- 订阅。

### 完成标准

- 管理 API 可以查询 usage logs。
- 每次请求有 request_id。

## 13. M9：前端接入准备接口

### 目标

不写视觉设计，但保证前端可接入。

### 交付 API

```txt
GET /api/v1/admin/overview
GET /api/v1/admin/accounts
GET /api/v1/admin/accounts/{id}/health
GET /api/v1/admin/scheduler/overview
GET /api/v1/admin/scheduler/decisions
GET /api/v1/me/usage
```

### 完成标准

- 前端可通过 OpenAPI client 获取控制台基础数据。
- 接口响应结构稳定。

## 14. M10：本地部署与质量门禁

### 目标

开发者一条命令启动完整环境。

### 交付

```txt
deploy/docker-compose.yml
.env.example
Makefile 或 taskfile
tools/dev.ps1
tools/generate-openapi.ps1
```

### 本地服务

```txt
api
postgres
redis
web，可选
```

### 质量门禁

- Go test。
- OpenAPI lint。
- OpenAPI codegen check。
- Ent generate check。
- Docker Compose smoke test。

### 完成标准

- 新开发者可以根据 README 启动项目。

## 15. MVP 端到端验收流程

1. 启动 PostgreSQL、Redis、API。
2. 创建管理员。
3. 登录。
4. 创建 Provider。
5. 创建 Model。
6. 创建 Provider Account。
7. 创建 API Key。
8. 使用 OpenAI SDK 调用 `/v1/chat/completions`。
9. 使用 Responses-compatible 请求调用 `/v1/responses`。
10. 使用 Anthropic Messages-compatible 请求调用 `/v1/messages`。
11. Scheduler 选择账号并生成 decision。
12. Provider 返回响应。
13. Usage log 记录用量。
14. Admin API 查询 decision 和 usage。

## 16. MVP 技术债允许项

第一版允许：

- 简单 RBAC。
- 简单 admin bootstrap。
- 单 Provider Adapter。
- 简单策略权重配置。
- 简单 usage 估算。
- 简单 billing hook，不做完整支付。

不允许：

- 明文存储 Provider 凭证。
- API Key 原文持久化。
- Gateway 绕过 Scheduler 直接选账号。
- 无 request_id。
- 无 OpenAPI 契约。
- 无调度 decision 记录。

## 17. MVP 后的下一阶段

MVP 完成后进入：

```txt
Phase 3 Billing / Subscription / Payment
Phase 4 Cache Affinity / Cost Optimization
Phase 5 Multi-provider Expansion / Advanced Governance
```

## 18. 开工建议顺序

实际开发建议顺序：

1. 创建 monorepo skeleton。
2. 初始化 Go module。
3. 初始化 OpenAPI skeleton。
4. 初始化 PostgreSQL / Redis compose。
5. 写 health endpoint。
6. 写 Ent schema MVP。
7. 写 API Key 模块。
8. 写 Provider / Model / Account 模块。
9. 写 OpenAI-compatible Adapter。
10. 写 Client Endpoint Adapter 和 Canonical AI Request / Response。
11. 写 Gateway 多端点兼容。
12. 写 Scheduler v1。
13. 接入 usage 和 decision 查询。
