# SRapi MVP 实现级规格

## 1. 元数据

| 字段 | 值 |
| --- | --- |
| 状态 | Draft |
| 适用阶段 | MVP / Phase 1 |
| 关联文档 | `MVP_IMPLEMENTATION_PLAN.md`, `ARCHITECTURE.md`, `OPENAPI_CONTRACT.md`, `AI_ENDPOINT_COMPATIBILITY.md`, `GATEWAY_ROUTE_MATRIX.md`, `DATA_MODEL.md`, `SCHEDULING_KERNEL_DESIGN.md`, `PROVIDER_ADAPTER_SPEC.md`, `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`, `REVERSE_PROXY_SPEC.md`, `SECURITY_MODEL.md`, `CONFIGURATION_SPEC.md`, `OPERATIONS.md`, `OBSERVABILITY_SPEC.md`, `PAYMENT_SPEC.md`, `AFFILIATE_REBATE_SPEC.md` |
| 目标读者 | 后端、前端、测试、文档、AI 编码代理 |

## 2. 目标

MVP 必须交付一个可本地运行、可通过 Chat Completions、Responses、Messages 等主流 AI 端点调用上游模型、可进行端点互转、并能记录调度决策与用量的 AI Gateway 骨架。

MVP 的重点不是一次性完成商业化系统，而是把以下闭环打通：

```txt
API Key -> Client Endpoint Adapter -> Canonical AI Request -> Scheduler v1 -> Provider Adapter -> Usage Log -> Admin Observability
```

## 3. 范围

### 3.1 必须包含

- Monorepo 基础目录。
- Go API 服务。
- PostgreSQL 与 Redis 本地开发环境。
- OpenAPI-first 契约生成流程。
- 控制台登录与当前用户接口。
- API Key 创建、展示一次、哈希存储、鉴权。
- Provider / Model / Provider Account 基础管理。
- OpenAI-compatible `/v1/models`。
- OpenAI-compatible `/v1/chat/completions`，包含非流式和流式。
- OpenAI-compatible `/v1/responses`，包含非流式和流式基础转换。
- Anthropic-compatible `/v1/messages`，包含非流式和流式基础转换。
- Gateway 路由矩阵最小集合与 Provider alias 阶段边界，详见 `GATEWAY_ROUTE_MATRIX.md`。
- Chat Completions、Responses、Messages 之间通过 Canonical AI Request / Response 相互转换。
- OpenAI-compatible Provider Adapter v1。
- OpenAI-compatible / Anthropic-compatible Provider preset 注册表骨架，详见 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`。
- 反代运行时（Reverse Proxy Runtime）骨架：账号 runtime_class、独立 HTTP client、独立 cookie jar、独立出口代理、Header Hygiene、SSE 字节透传、OAuth refresh 接口、反代错误分类。
- 至少 1 个反代 Provider Adapter（推荐 `reverse-proxy-claude-code-cli` 或 `reverse-proxy-codex-cli`，使用 OAuth refresh token），用于验证反代闭环。
- Scheduler v1 的账号选择、过滤、打分、Lease、Decision、Feedback。
- Usage Log 与基础管理查询接口。
- 基础 Audit Log。
- 本地质量门禁。

### 3.2 明确暂缓

- 支付和外部支付 Webhook，详见 `PAYMENT_SPEC.md` 的 Phase 2 规划。
- 邀请返利、提现和退款返利补偿，详见 `AFFILIATE_REBATE_SPEC.md`。
- 完整订阅购买流程。
- 复杂 RBAC。
- 高级策略 DSL。
- 机器学习调度。
- Realtime、Batch、Fine-tuning、Assistants 全量兼容。
- Responses stateful store、内置工具、远程工具全量兼容。
- Gemini-native 公开端点。
- 完整 utls TLS 指纹 / HTTP/2 SETTINGS 模拟、Egress Profile 管理后台、Behavior Pacer 高级模式、外部 challenge solver 集成、HTTP/3、WSS 反代等高级反代能力，详见 `REVERSE_PROXY_SPEC.md` 阶段表。
- Kubernetes 生产部署。
- 完整 Ops Dashboard、SLO / burn-rate 告警、Provider 健康矩阵，详见 `OBSERVABILITY_SPEC.md` Phase 2。

## 4. 功能需求

| 编号 | 需求 |
| --- | --- |
| FR-001 | 系统必须提供 `/api/v1/health`，用于本地和部署健康检查。 |
| FR-002 | 系统必须支持管理员引导或种子数据创建第一个管理员用户。 |
| FR-003 | 控制台 API 必须使用 HttpOnly Cookie 登录态，并对写操作要求 CSRF Token。 |
| FR-004 | Gateway API 必须使用 `Authorization: Bearer sk-...` API Key 鉴权。 |
| FR-005 | API Key 原文必须只展示一次，数据库只能保存安全哈希和 prefix。 |
| FR-006 | 管理员必须能创建、查询、更新、禁用 Provider。 |
| FR-007 | 管理员必须能创建、查询、更新 Model Registry、Model Alias、Provider Model Mapping。 |
| FR-008 | 管理员必须能创建、查询、更新、测试、禁用、启用 Provider Account。 |
| FR-009 | Provider Account 凭证必须加密保存，不得明文持久化。 |
| FR-010 | `/v1/models` 必须返回当前 API Key 可访问模型列表。 |
| FR-011 | `/v1/chat/completions` 必须支持 OpenAI-compatible 基础请求字段。 |
| FR-012 | `/v1/chat/completions` 必须支持 `stream: true` 的 SSE 流式响应。 |
| FR-012A | `/v1/responses` 必须支持基础 `input`、`instructions`、`tools`、`stream` 和结构化输出字段，并转换为 Canonical AI Request。 |
| FR-012B | `/v1/messages` 必须支持 Anthropic-compatible 基础 `system`、`messages`、`max_tokens`、`tools`、`stream` 字段，并转换为 Canonical AI Request。 |
| FR-012C | Gateway 必须能把 Canonical AI Response 渲染回调用方源端点格式。 |
| FR-013 | Gateway 必须把请求标准化后交给 Scheduler，不得绕过 Scheduler 直接选择账号。 |
| FR-014 | Scheduler v1 必须至少支持 `balanced` 与 `cost_saver` 策略。 |
| FR-015 | Scheduler v1 必须执行账号硬过滤，包括禁用、额度耗尽、并发满、RPM/TPM 满、熔断打开。 |
| FR-016 | Scheduler v1 必须生成可审计的 `scheduler_decisions` 记录。 |
| FR-017 | Scheduler v1 必须使用 Lease 防止账号并发超限。 |
| FR-018 | Provider Adapter 必须支持请求转换、响应转换、流式解析、错误分类、usage 解析。 |
| FR-018A | 反代 Provider Adapter 必须通过 Reverse Proxy Runtime 发起上游请求，账号之间不得共享 HTTP client、cookie jar、UA、出口 IP。 |
| FR-018B | 反代请求向上游发出的 Header 与 Body 中不得包含任何 SRapi 自有标识（`X-Request-ID`、`X-Forwarded-*`、`Via`、`X-SRapi-*`、`User-Agent: SRapi/*` 等）。 |
| FR-018C | 反代账号识别到 `session_invalid` / `account_locked` / `account_banned` / `abuse_detected` 时必须自动进入 `needs_reauth` 或 `disabled`，且不得继续被调度。 |
| FR-018D | 反代账号的 OAuth refresh 必须加分布式锁，刷新失败不得覆盖旧凭证，刷新成功必须重新加密存储并写 audit。 |
| FR-019 | 每次 Gateway 请求必须生成 `usage_logs` 记录，成功和失败都要记录。 |
| FR-020 | 每次 Provider 调用结果必须生成 `scheduler_feedbacks` 或等价反馈记录。 |
| FR-021 | 管理 API 必须能查询 usage logs、scheduler decisions、账号健康和额度基础信息。 |
| FR-022 | 所有管理端高风险写操作必须写入 `audit_logs`。 |
| FR-023 | OpenAPI 契约必须能 lint、bundle，并生成 Go server types 与 TypeScript client。 |
| FR-024 | 本地开发必须能通过一条命令或清晰步骤启动 PostgreSQL、Redis、API。 |
| FR-025 | MVP 必须提供配置样例、配置校验和 release 模式弱 secret 拒绝启动规则，详见 `CONFIGURATION_SPEC.md`。 |
| FR-026 | MVP 必须提供基础运维端点和本地部署门禁，生产治理扩展以 `OPERATIONS.md` 为准。 |

## 5. 非功能需求

| 编号 | 需求 |
| --- | --- |
| NFR-001 | 所有 HTTP 响应必须携带 `X-Request-ID`，响应体也应包含 `request_id` 或 OpenAI-compatible 等价追踪信息。 |
| NFR-002 | API Key 原文、Provider 凭证、OAuth Token、Cookie、完整用户 prompt 默认不得写入日志。 |
| NFR-003 | Redis 只能保存可重建运行时状态，PostgreSQL 是业务真实来源。 |
| NFR-004 | MVP 中 Gateway 额外引入的非流式代理延迟 p95 目标应小于 100ms，不包含上游模型耗时。 |
| NFR-005 | Scheduler v1 单次决策 p95 目标应小于 20ms，候选账号数量大于 100 时允许后续优化。 |
| NFR-006 | Provider 调用超时必须可配置，并有默认值。 |
| NFR-007 | 生成代码不得手改，所有变更必须回到 OpenAPI 契约。 |
| NFR-008 | 金额和成本不得用 float 存储或传输真实账务数值。 |
| NFR-009 | 所有数据库迁移必须可重复执行、可在空库应用、可在测试中验证。 |
| NFR-010 | MVP 必须能在 Windows 开发环境下运行基础脚本。 |
| NFR-011 | 端点转换不得静默丢失关键语义；无法无损转换时必须返回 compatibility warning 或明确错误。 |
| NFR-012 | 反代请求 outgoing header set 必须在自动化测试中按白名单校验，禁止出现 SRapi 自有头部。 |
| NFR-013 | 反代账号 cookie jar、device fingerprint、HTTP client 必须按账号隔离；自动化测试必须覆盖跨账号污染场景。 |
| NFR-014 | SRapi 不内置任何具体上游 ToS 绕过手段；任何反代行为的合规风险由部署者承担，并在 README 与管理后台明示。 |
| NFR-015 | Gateway 路由新增或 Provider alias 新增必须同步 `GATEWAY_ROUTE_MATRIX.md`，不得在 handler 中复制 provider-specific 账号选择逻辑。 |
| NFR-016 | Compatible Provider preset 新增必须同步 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`，不得硬编码 secret 或绕过模型可见性。 |

## 6. 验收条件

| 编号 | 场景 |
| --- | --- |
| AC-001 | Given 本地 PostgreSQL、Redis 已启动，When 启动 API，Then `/api/v1/health` 返回 200 且包含 `request_id`。 |
| AC-002 | Given 首次启动环境，When 执行管理员引导，Then 系统存在一个可登录管理员用户。 |
| AC-003 | Given 用户登录控制台，When 创建 API Key，Then 响应只返回一次原文 key，数据库不包含原文 key。 |
| AC-004 | Given API Key 被禁用，When 调用 `/v1/models`，Then 返回 401 或 403，并记录 request id。 |
| AC-005 | Given 已配置一个 OpenAI-compatible Provider Account，When 调用 `/v1/models`，Then 返回该 API Key 可访问模型。 |
| AC-006 | Given 已配置模型映射和 Provider Account，When 使用 OpenAI SDK 指向 SRapi 调用 `/v1/chat/completions`，Then 能收到非流式响应。 |
| AC-007 | Given 请求包含 `stream: true`，When 调用 `/v1/chat/completions`，Then SRapi 返回 OpenAI-compatible SSE chunk。 |
| AC-007A | Given 调用 `/v1/responses`，When 上游实际使用 OpenAI-compatible Chat Completions，Then SRapi 返回 Responses-compatible 响应。 |
| AC-007B | Given 调用 `/v1/messages`，When 上游实际使用 OpenAI-compatible Chat Completions，Then SRapi 返回 Anthropic Messages-compatible 响应。 |
| AC-007C | Given 请求包含 tools 或 structured output，When 端点转换无法无损表达，Then 响应包含 compatibility warning 或返回明确校验错误。 |
| AC-008 | Given 所有账号都不可用，When 调用 chat completions，Then 返回明确的 `NO_AVAILABLE_ACCOUNT` 或 OpenAI-compatible service unavailable 错误。 |
| AC-009 | Given 两个账号一个并发已满，When 触发调度，Then Scheduler 不选择并发已满账号，并记录 reject reason。 |
| AC-010 | Given 一次 Gateway 请求完成，When 查询 scheduler decisions，Then 能看到候选数、拒绝原因、选中账号和 score breakdown。 |
| AC-011 | Given Provider 返回 rate limit，When Adapter 分类错误，Then feedback 标记 `rate_limit`，账号进入短冷却或降权。 |
| AC-012 | Given Provider 不返回 usage，When 请求完成，Then usage 标记为估算，后续允许异步修正。 |
| AC-012A | Given 配置一个反代 Provider Account（OAuth refresh token 或 web session cookie），When 通过 Gateway 调用对应模型，Then 上游能成功响应，且 outgoing header 不含任何 SRapi 自有标识。 |
| AC-012B | Given 同一 Provider 配置 2 个反代账号，When 同时发起请求，Then 各账号使用独立 cookie jar 与 HTTP client，互不污染。 |
| AC-012C | Given 反代账号上游返回 `session_invalid` 或 `account_locked`，When 反代运行时识别后，Then 账号自动 `needs_reauth` 或 `disabled` 并停止被调度。 |
| AC-012D | Given 反代账号的 OAuth access token 过期，When 触发 refresh，Then 刷新过程加锁、刷新失败不覆盖旧凭证、刷新成功后凭证重新加密保存并写 audit。 |
| AC-013 | Given 管理员禁用账号，When 查询 audit logs，Then 可以看到操作者、资源、前后状态摘要和 trace id。 |
| AC-014 | Given OpenAPI 契约变更，When 运行质量门禁，Then lint、bundle、codegen check 都通过。 |
| AC-015 | Given 新开发者 clone 项目，When 按 README 执行本地启动步骤，Then 能完成管理员登录和一次 mock Gateway 调用。 |

## 7. 边界场景

| 编号 | 场景 | 期望 |
| --- | --- | --- |
| EC-001 | API Key prefix 存在但 hash 不匹配 | 拒绝请求，不泄漏 key 是否部分正确。 |
| EC-002 | Provider Account 凭证解密失败 | 账号不可用，记录内部错误，不返回敏感细节给客户端。 |
| EC-003 | 上游流式响应中途断开 | 记录 partial failure，不默认重试已经开始输出 token 的请求。 |
| EC-004 | Redis 中 Lease 过期但请求仍在运行 | Feedback 提交时必须处理过期租约，避免并发计数永久泄漏。 |
| EC-005 | Usage 写入失败 | 请求结果仍返回客户端，但必须记录错误并允许补偿任务修复。 |
| EC-006 | OpenAPI 生成代码与契约不一致 | 质量门禁失败。 |
| EC-007 | 管理员重复提交创建 Provider Account | 如果有幂等 key，返回同一结果；否则按唯一约束返回 conflict。 |
| EC-008 | 模型别名指向不存在模型 | 管理 API 校验失败。 |
| EC-009 | Provider 返回非标准错误体 | Adapter 必须映射到 `unknown` 或更接近的内部错误分类。 |
| EC-010 | 用户余额不足但账号可用 | 用户侧 quota 先拒绝，Scheduler 不应消耗上游账号。 |
| EC-011 | Responses stateful 字段无法映射到无状态上游 | 返回 compatibility warning，或在必须保持语义时拒绝请求。 |
| EC-012 | Anthropic thinking / OpenAI reasoning 无等价目标字段 | 记录 compatibility warning，不得伪造能力。 |

## 8. MVP 最小数据集合

MVP 必须实现的数据表以 `DATA_MODEL.md` 为准，但至少需要覆盖：

```txt
users
roles
user_roles
api_keys
api_key_groups
providers
model_registry
model_aliases
model_provider_mappings
pricing_rules
provider_accounts
account_groups
account_group_members
usage_logs
scheduler_decisions
scheduler_feedbacks
billing_ledger
account_health_snapshots
account_quota_snapshots
settings
audit_logs
idempotency_records
```

`sticky_sessions` 与 `cache_affinity_records` 可在 MVP 中 Redis-only，但必须在 `SCHEDULER_V1_SPEC.md` 中明确 key、TTL、重建策略和后续落库路径。

## 9. 需求到测试映射

| 需求 | 测试类型 |
| --- | --- |
| FR-001, NFR-001 | health endpoint integration test |
| FR-003, FR-004, FR-005 | auth/API key unit + integration tests |
| FR-006 到 FR-009 | admin CRUD + credential encryption tests |
| FR-010 到 FR-012C | AI endpoint compatibility contract tests + `AI_ENDPOINT_COMPATIBILITY.md` golden tests |
| FR-013 到 FR-017 | Scheduler unit tests + `SCHEDULING_SCENARIOS.md` |
| FR-018 | Provider Adapter mock tests |
| FR-019 到 FR-021 | usage/decision repository integration tests |
| FR-022 | audit log tests |
| FR-023 | OpenAPI CI checks |
| FR-024 | Docker Compose smoke test |
| FR-025, FR-026 | config validation + health/readiness smoke test |

## 10. 交付门禁

MVP 不满足以下任一项，不得标记完成：

- OpenAPI lint / bundle / codegen check 通过。
- `go test ./...` 通过。
- Ent schema generate 与 migration apply test 通过。
- API Key 不存原文的测试通过。
- Provider Account 凭证加密测试通过。
- Gateway 请求必须产生 decision、feedback、usage log。
- `/v1/chat/completions`、`/v1/responses`、`/v1/messages` 的最小互转测试通过。
- Scheduler 场景矩阵最小集合通过。
- 本地启动流程在 README 中可复现。
