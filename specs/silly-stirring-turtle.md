# SRapi 商用化跃迁工程计划（P1 + Pareto 路由）

> 状态：起草中（Phase 1 探索 → Phase 2 设计 → Phase 4 最终方案）

## Context（待 Phase 4 补完）

- 起因：SRapi 当前架构骨架完整（22 模块、157 OpenAPI operation、15k 行 spec），但业务逻辑落地率仅 20-30%，呈"豪华骨架 + 空心肌肉"形态。
- 用户决策：并行推进 P1 三条主线，押 Cost-Quality-Latency 三维 Pareto 路由 + 在线 eval 作为差异化武器，**聚焦业务功能**而非商业模式 / 客户分层 / 合规话题。
- 目标产出：把演示骨架填实，到"会跑、能用、不丢数据、有差异化护城河"的真正商用级网关。

## P1 工程线路（并行三条主线）

### 线 A：网关稳定性
- A1. Auth & Session 持久化（memory → Postgres + Redis refresh）
- A2. 限流中间件落地（RPM/TPM/并发 Redis 令牌桶）
- A3. 账号健康 Worker（自动探测 + 状态机自动转移 + 熔断）
- A4. 跨供应商故障转移（同模型异厂商 fallback 链）
- A5. 多供应商扩到 ≥10 家（DeepSeek/Kimi/通义/智谱/Grok/Mistral/Groq/Together）

### 线 B：支付计费闭环
- B1. 实际计费引擎（usage_log → 价格表精确计算 → ledger 扣减 → 余额拦截）
- B2. 支付落地（Stripe + 支付宝 + 微信）
- B3. Affiliate 持久化（memory → entstore，分销/邀请/返佣账本闭环）
- B4. 订阅激活流程（payment.paid → subscription.activate → entitlement.refresh）

### 线 C：可观测性 + 多租户基础
- C1. OTel + 结构化 trace（Langfuse 数据模型 trace/observation/score）
- C2. 三层多租户雏形（User 上加 Workspace）
- C3. PII / Prompt Injection 基础（LLM-Guard 起步）

### 杀手锏 K1：Cost-Quality-Latency 三维 Pareto 自动路由 + 在线 eval

详细任务拆解见下方 Phase 4 最终方案。

---

# Phase 1 探索记录

## 代码现状

### 线 A — 网关稳定性

| 子任务 | 现状关键证据 | 我们预留的资产 | 还缺什么 |
|---|---|---|---|
| **A1 Session 持久化** | 已落地 `auth_sessions` Ent/PostgreSQL 表、`entstore/auth`、app/httpserver 注入；session ID 与 CSRF token 均只存 SHA-256 hash；HTTP 回归已证明 runtime rebuild 后旧 cookie + CSRF 仍可用 | Password 登录、CSRF 防护、credential_version 框架完整；`000002_auth_sessions` up/down 迁移和 `make migration-check` 已覆盖 | 后续可补 session 清理 worker / Redis refresh rotation；核心 AuthSession 持久化已完成 |
| **A2 限流中间件** | 已落地 `platform/ratelimit` Redis Lua 原子多维计数与 Redis ZSet 并发租约；Gateway enforce API key RPM/TPM/concurrency 与 User RPM，超限返回 429 + `Retry-After`；账号成功用量会从最近窗口写回 `rpm_used` / `tpm_used` metadata，scheduler 已基于 `rpm_limit` / `tpm_limit` / `max_concurrency` 记录 `rpm_limit_exceeded` / `tpm_limit_exceeded` / `concurrency_full` reject evidence | API Key/User 限额字段、API key `concurrency_limit` 字段、Gateway usage evidence、30s account cooldown、Account metadata 运行时限额字段、Scheduler structured reject reasons | 还缺账号级 counter 的 Redis 原子化/跨实例一致性、p99 benchmark；核心 API key/user RPM/TPM enforcement、API key concurrency enforcement 与 scheduler/account-level reject evidence 已完成 |
| **A3 账号健康 Worker** | 已落地 `accounts.ProbeAccount()`、provider adapter `/models` 探测、`health_probe` worker、`ACCOUNT_HEALTH_PROBE_*` 配置和 `app.go` 生命周期装配；worker 每 5min 默认遍历活跃 API-key account，写入 `AccountHealthSnapshot`，并通过 `cooldown_active` / `circuit_open` metadata 让 scheduler 避开异常账号 | `AccountHealthSnapshot` schema、`accounts.LatestHealthSnapshotByAccount()`、scheduler `circuit_state` / cooldown 读取路径、状态机单测和 worker RunOnce 单测 | 后续可补 provider-specific 深度探测、反代 runtime 探测、探测指标和 dashboard 展示；A3 核心主动探测 + 状态机 + worker 已完成 |
| **A4 跨厂商故障转移** | `gatewayCandidates()` 已遍历所有 provider 的 account，天然支持跨厂商候选；`ScheduleResult.Candidates` 已返回排序候选列表；`SchedulerDecision.fallback_from_decision_id` 已落库并暴露到管理 API；lease update 已按 `(request_id, attempt_no)` 定位，避免 fallback attempt 互相覆盖；Gateway text / Responses / Messages / Embeddings / Gemini GenerateContent handler 已按 retryable provider error 自动切换候选；`usage_logs` 已按 attempt 记录，`srapi_gateway_failover_total` 已暴露 | Filter reason 已枚举到 9 类；sticky session、account group 隔离、attempt_no、fallback_excluded reject evidence、failover usage evidence、failover metric 已实现 | 后续可补 streaming provider 直通场景的首字节前 retry 边界测试和更多 failover smoke；A4 核心调度 + HTTP fallback 已完成 |
| **A5 多供应商扩展** | Adapter 已实现 6 种：openai/anthropic/gemini 兼容 + generic-reverse-proxy + codex/antigravity 反代；`generic-reverse-proxy` 已读取 provider/account metadata 的 `base_url`、自定义 auth header、chat/embeddings path、body mapping、response path，支持 API-key HTTP client 与 custom reverse-proxy runtime，Gateway 回归已证明配置型上游端到端可用；Provider 数据库驱动；`bootstrapGatewayCatalog()` 仅 seed 一个 openai-compatible | `capabilities map` + `adapter_type` + `protocol` 三维度可组合；credential 加密+版本框架完整；generic adapter 已可承载无专用 adapter 的 OpenAI-compatible 上游 | **新增 `providers/preset/`**（DeepSeek/Kimi/通义/智谱/Grok/Mistral/Groq/Together 配置）；完善 `/admin/providers/{id}/test` 对代表 preset 的覆盖 |

### 线 C — 可观测性 + 多租户 + PII

| 子任务 | 现状关键证据 | 我们预留的资产 | 还缺什么 |
|---|---|---|---|
| **C1 OTel + 结构化 trace** | `runtime_metrics.go:15-113` 手写 Prometheus 文本，20+ 指标；slog 已集成（`runtime_state.go:6`）；`requestIDMiddleware` 已注入 `X-Request-ID`；`ObsAlertEvent/ObsSloDefinition` 表完整但无逻辑 | Prometheus 指标可扩展；context 传 request_id；OBS 表设计完整 | **加 `go.opentelemetry.io/otel`** 依赖；slog handler 装饰器自动注入 request_id/user_id；metrics 加 histogram bucket（0.1/0.5/1/5s）；SLO breach 检测 worker |
| **C2 多租户三层** | User → APIKey 一层；`AccountGroup` 是 provider 分组**不是多租户**；`Role` 表仅 name+description；无 Workspace/Org/Team 表；无 Entitlement 表 | UserRole 多对多已留；APIKey 已有 scopes_json/allowed_models_json | **新增 `Workspace` ent schema**（先不上 Organization）；User 加 `workspace_id` FK；APIKey 加可选 `workspace_id`；Role 加 `permissions` JSON；新增 `Entitlement` 表 |
| **C3 PII / Injection** | 已落地 `modules/content_safety/` 起步模块；Gateway admission 在 CanonicalRequest 后统一扫描 prompt/messages/embedding/image/audio/speech/moderation/rerank 文本字段，邮箱、手机号、SSN、身份证/国民 ID、信用卡会在上游 dispatch 前脱敏，prompt-injection 关键词会产生 warning；命中后写 `gateway.content_safety` audit，仅保存 finding 类型/数量/严重性，不保存原始 PII；usage evidence 会持久化 `content_safety_*` compatibility warnings | Gateway pipeline hook 已接入所有 `prepareGatewayAdmission` 调用点；Canonical IR 仍保持 provider-neutral；audit/usage 证据链可用于后续策略 | 后续可补配置化阈值、block/mask/warn 策略、LLM-Guard/结构化 detector、按 workspace/API key 的可配置安全策略 |

### Pareto 路由扩展点（杀手锏 K1）

**当前完成度高于预期**——`StrategyRegistry` 框架已成型，scoreBreakdown 已有 11 维度（Final/Health/Quota/Latency/Quality/Sticky/Cache/Cost/Fairness/RiskPenalty/SaturationPenalty），SchedulerStrategy 表已支持持久化策略配置；`docs/SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 已设计 9 个策略和灰度机制。

| 扩展点 | 现状 | 改动 |
|---|---|---|
| **costScore 子分数** | `scheduler/service/service.go:479` 权重已挂但**算法未实现** | 实现 `costScore(candidate)`：从 pricing + 历史 actual_cost 算 |
| **cacheScore 子分数** | 硬编码 `0.0` | 实现 `cacheScore(candidate)`：从 cache hit history + prompt prefix hash 算 |
| **运行时策略加载** | `StrategyRegistry` 仅编译时注册 | 新增 `strategy_loader.go`：从 `SchedulerStrategy` 表读 config_json → unmarshal 为运行时 strategy |
| **缺失的 5 个内置策略** | spec 设计了 9 个但只实现 2 个（balanced / cost_saver） | 实现 latency_first / quota_protect / sticky_first / cache_affinity_first / premium_quality |
| **质量评估闭环（杀手锏核心）** | **完全缺失** | 新增 `modules/quality_eval/`、`QualityEvaluation` ent schema、`quality_feedback_worker`：抽样过去的 Decision/Feedback，调用 LLM-as-judge，反馈到 RuntimeState |
| **Pareto 前沿优化** | 已落地 `scheduler/service/pareto.go`：Cost / Latency / Quality Pareto 前沿筛选先于加权排序执行，`Decision.Scores["pareto"].frontier_account_ids` 记录前沿证据；缺失明确输入的目标不参与支配判断，避免默认分数误剪候选 | 后续 K1.4 的 QualityEval 需要把在线质量分写入 `quality_score` / `quality_tier` 信号 |
| **策略灰度/dry-run** | 已落地 `scheduler/service/simulator.go`、`POST /api/v1/admin/scheduler/simulate`、snapshot-backed `POST /api/v1/admin/scheduler/replay` 和真实 Gateway 流量稳定百分比分流；Admin Settings 可按 model / API key prefix hash scope 启用 shadow strategy，Decision/Snapshot 只落 hash/bucket/selection 证据 | 后续可补 user_group / provider 独立 scope、完整灰度效果指标和一键回滚 UI |
| **可解释性** | scoreBreakdown 已记录但无自然语言解释 | Decision 加 `selection_rationale` 字段（"为什么选 A 不选 B" 一句话） |

### 通用基础设施

- **Workers 框架已有**：`outbox` + `retention`（`apps/api/internal/workers/`，`app.go:74-77` 注册）→ 我们可以照搬这个模式添加 `health_check`、`quality_eval`、`pricing_sync`、`subscription_activator` 等
- **配置**：纯 env vars（`config/config.go`），无 feature flag 系统
- **数据库迁移**：`apps/api/atlas.hcl` + `make migration-diff` / `make migration-hash` 已建立 Ent → Atlas → PostgreSQL 16 的增量迁移工作流；`make migration-check` 已约束 `postgres/up` 与 `postgres/down` SQL 文件同名配对、编号连续，后续新增 schema 不应继续膨胀 initial migration。
- **Atlas 配置**：`apps/api/atlas.hcl` 以 `ent://ent/schema` 为目标 schema，以 `docker://postgres/16/dev?search_path=public` 为 dev database，迁移目录指向 `migrations/postgres/up`。

### 线 B — 支付计费

| 子任务 | 现状关键证据 | 我们预留的资产 | 还缺什么 |
|---|---|---|---|
| **B1 计费引擎** | `UsageLog.cost` schema 字段在但**默认 `"0.00000000"` 从不回写**；`gatewayPricingEvidence.PricingSource` 已支持 `pricing_rule / mapping_override / default_zero` 三档（`runtime_gateway_core.go:60-79,158-177`）；`BillingLedger` schema + entstore + service.Record/List 完整 | 预估计价已经算出，只是不写回 UsageLog；BillingLedger 支持 6 种 type（usage_charge / payment_credit / refund / adjustment / affiliate_transfer） | **在 `recordGatewayUsage` 处回写 cost**；新增 **余额扣费 worker**（聚合 UsageLog → 写 BillingLedger）；`GET /me/balance` API；PricingRule 导入 API |
| **B2 支付落地** | `payments/service/service.go` **908 行**，CreateOrder / HandleWebhook / RequestRefund / markPaidAndFulfill / fulfill / enqueuePaid / enqueueRefunded / selectProviderInstance 全部实现；HMAC-SHA256 签名验证 + 幂等 (`paymentauditlog.idempotency_key`)；AES-GCM 配置加密（config_ciphertext）；Webhook 路由 `POST /api/v1/webhooks/payments/{provider}` 已注册（`runtime_user_handlers.go:284-308`） | 订单状态机（pending → paid → fulfilled，含退款分支 8 状态）；provider_snapshot_json 防漂移；多商户实例选择 | **go.mod 无任何支付 SDK**——需添 `stripe/stripe-go/v78`、评估 `smartwalle/alipay`、`wechatpay-apiv3/wechatpay-go`；**新增 `payments/providers/{stripe,alipay,wechat,easypay}/provider.go`** 实现 PaymentProvider 接口；支付配置管理员加密保存 API；订单过期清理 worker |
| **B3 Affiliate** | `affiliate/service/service.go` **651 行**已实现 BindInvite / AccrueRebate / TransferToBalance（框架）；entstore 550 行；InviteCode / InviteRelationship / AffiliateLedger / AffiliateRule schema 完整；返利补偿 type=refund_compensation 已留 | 自邀请校验、过期码、激活状态检查；按时间/规则计算返利 | **payments.fulfill 后没调 `affiliate.AccrueRebate`**（联动缺失）；**payments.RequestRefund 没调 `affiliate.CompensateRefund`**；**`TransferToBalance` 没调 `billing.Record`**；用户侧 `GET /me/affiliate/ledger` API；返利结算 worker |
| **B4 订阅激活** | `subscriptions/service/service.go` 已实现 CreatePlan/CreateUserSubscription/CreatePricingRule/CheckEntitlement/EstimatePrice 接口（具体实现度待核）；entitlements_snapshot_json 防漂移；**DomainEventsOutbox/Inbox schema + entstore + `events.Service.Enqueue/DispatchPending` 完整**；`PaymentOrderPaid` 事件已通过 `enqueuePaid` 写入 Outbox（`payments/service/service.go:450`） | Outbox 模式已就位，事件幂等已支持（`idempotency_key UNIQUE per module`） | **🔴 Outbox dispatcher worker 缺失**（事件入队无人消费）；**Payment 事件 subscriber 缺失**（payment.paid → activate subscription / accrue affiliate）；订阅过期清理 worker；`FindPricingRuleByModelAndProvider` |

---

# Phase 2 + 4 最终工程方案

## A. Context（更新）

SRapi 当前看似"演示项目"的根因不是骨架弱，而是 **(1) 装配层默认走 memory store**、**(2) 关键 worker（Outbox dispatcher / 健康探测 / 余额扣费 / 订单订阅过期）缺位**、**(3) 适配器层缺位**（支付 SDK、通用反代、多供应商 preset）、**(4) 一处计费回写断链**（UsageLog.cost 永远 0.00000000）、**(5) 缺中间件**（限流 / OTel）。

经直接验证，业务 service 完成度远高于初判：`payments/service.go` 908 行、`affiliate/service.go` 651 行、`scheduler/service.go` 含 lease + scoreBreakdown 10 维度、`subscriptions` 含 CheckEntitlement + EstimatePrice、entstore for affiliate/payments/billing/subscriptions 全部就绪、DomainEventsOutbox 表 + Enqueue/DispatchPending 全部就绪——**真正缺的是"接线"和"装配"**。

目标产出：把演示骨架填实到"会跑、能用、不丢数据、有差异化护城河"，落地 P1 12 项 + K1 Pareto 杀手锏。

## B. 总体策略：4 大执行原则

1. **装配优先（最高 ROI）**：很多 entstore / 业务 service 已就位，先在 `runtime_state.go` 把 `*memory.New()` 默认值改成 `entstore.New(client)`，并把 affiliate/subscriptions 接入 payments service 的 `Dependencies`，**这一步几乎不写新代码就能从"演示" → "持久化"**。
2. **契约稳定，只填实现**：`*/contract/*.go` 接口层不动（否则会触发 OpenAPI / 前端 SDK 大面积变更）；新增能力以"新加方法 / 新加字段 / 新建模块"形式落地，不改既有签名语义。
3. **增量迁移工作流先行**：第一件事建立 `migrations/postgres/up/000002_xxx.sql` ~ `00000N` 的命名约定 + `make migration-check` 配套；后续每个新 ent schema 都伴随一个迁移文件 + down 文件，避免 866 行 initial schema 膨胀。
4. **Worker 化是黏合剂**：照搬 `workers/outbox/`、`workers/retention/` 模板（184~217 行/个）新增 6 个 worker：`events_dispatcher`、`health_probe`、`balance_charger`、`order_expirer`、`subscription_expirer`、`quality_eval`。每个 worker 独立 Start/Stop，由 `app.go` 注册。

## C. 关键风险与缓解

| 风险 | 缓解 |
|---|---|
| 新增 ent schema 字段触发 OpenAPI codegen drift | 所有新 schema 字段先内部使用，仅在确定对外暴露时才加 OpenAPI；`make check` 中 `ent-generate-check` 守门 |
| 增量迁移与 Ent 自动 schema 漂移 | 建立 atlas.hcl 或 ent migrate diff 流程，**每次 `make ent-generate` 后立即 `atlas migrate diff` 生成 SQL** |
| 限流中间件引入 p99 抖动 | Redis pipeline + Lua 脚本原子操作；性能基准必须 ≤ 2ms p99 才合并 |
| Outbox dispatcher 重复消费 | Inbox 表已有 `(event_id, consumer_name)` UNIQUE 约束，消费侧靠 `MarkInboxProcessed` 幂等 |
| 支付 SDK 引入安全风险（私钥/Webhook 验签） | 所有 secret 走现有 `platform/crypto` AES-GCM，Webhook 验签强制 + 重放检测（idempotency_key 已有） |
| 多供应商 preset 错误导致流量打错 | preset 仅是数据模板，新 provider 需经 `/admin/providers/{id}/test` 验证才能 status=active |

## D. 任务依赖图（关键路径）

```
基础设施层（先做，约 3-5 天）
├── I1 增量迁移工作流 ──────────────┐
└── I2 装配层切换 entstore  ────┐   │
                                ▼   ▼
线 A（网关稳定性，~3 周）  ⇄  线 B（支付计费，~3 周）  ⇄  线 C（可观测+多租户，~2 周）
├── A1 AuthSession 持久化   ├── B1 cost 回写 + 余额扣费 worker   ├── C1 OTel + slog 装饰
├── A2 限流中间件          ├── B2 Outbox dispatcher worker     ├── C2 Prom client + histogram
├── A3 健康 Probe + worker  ├── B3 Payment ↔ Affiliate 联动     ├── C3 SLO breach worker
├── A4 跨厂商故障转移      ├── B4 订单/订阅过期 worker         ├── C4 Workspace schema
└── A5 通用反代 + 8 preset  ├── B5 Stripe SDK                  ├── C5 Role.permissions + Entitlement
                            ├── B6 Alipay SDK                  └── C6 content_safety 模块
                            └── B7 WechatPay SDK
                                      │
                                      ▼
                            K1 Pareto 杀手锏（依赖 A3+B1+C1，~4 周）
                            ├── K1.1 costScore + cacheScore 实算
                            ├── K1.2 strategy_loader（DB→runtime）
                            ├── K1.3 补齐 5 个缺失策略
                            ├── K1.4 QualityEval 模块 + worker
                            ├── K1.5 Pareto optimizer
                            ├── K1.6 strategy_simulator（dry-run）
                            └── K1.7 /admin/ops/strategy 前端
```

**关键路径**：I1 → I2 → B1（cost 回写）→ B2（dispatcher）→ B3（联动） →  K1.4 QualityEval。这条线缺一不可。

## E. 详细任务卡

> 工作量单位：人天（PD）。每张卡都列「目标 / 涉及文件 / 实现要点 / 验收 / 依赖」。

### 基础设施层

**[I1] 建立增量迁移工作流（1.5 PD）**
- 状态：已落地（`apps/api/atlas.hcl`、`make migration-diff`、`make migration-hash`、up/down 配对与连续编号测试、`atlas.sum`）。
- 目标：每次新增 ent schema 自动产生 000002+ 迁移文件并能 `migration-check` 校验
- 文件：新增 `apps/api/atlas.hcl`；扩展 `Makefile` 增加 `migration-diff` 目标
- 要点：用 Atlas `migrate diff` 从 `ent://ent/schema` 生成 SQL；约定 split 目录文件名 `postgres/up/0000NN_short_subject.sql` + `postgres/down/0000NN_short_subject.sql`；CI 中跑 `migration-check`
- 验收：`make migration-diff MIGRATION_NAME=0000NN_subject` 调用 pin 的 Atlas CLI；`make migration-hash` 刷新 `postgres/up/atlas.sum`；`make migration-check` 校验 Ent/initial drift/down 覆盖和 up/down 配对连续
- 依赖：无

**[I2] 装配层从 memory 切换到 entstore（1 PD）**
- 目标：affiliate / billing / subscriptions / payments / auth 默认走 PostgreSQL，重启不丢数据
- 文件：`apps/api/internal/httpserver/runtime_state.go:160-300`（替换 5 处 `*memory.New()` 为 `entstore*.New(client)`）；`apps/api/internal/app/app.go` 注入 ent client
- 要点：保留 `opts.*` 注入路径（测试用 memory）；启动日志打印每个模块的 store 类型；e2e 测试切到 entstore
- 验收：重启 API 后 affiliate/payment/subscription 数据保留；`runtime_stores_test.go` 全过；新加一个 smoke 测：创建邀请码 → 重启 → 仍能查到
- 依赖：I1

### 线 A — 网关稳定性

**[A1.1] AuthSession ent schema + entstore/auth（2 PD）**
- 状态：已落地（`auth_sessions` Ent schema、`entstore/auth`、`000002_auth_sessions` up/down migration、runtime/app 注入、重启后旧 session cookie 可复用回归）。
- 目标：登录会话持久化，重启 / 多副本部署都不会丢
- 文件：新增 `apps/api/ent/schema/authsession.go`、`apps/api/internal/persistence/entstore/auth/store.go`、迁移 `postgres/up/000002_auth_sessions.sql` / `postgres/down/000002_auth_sessions.sql`；改 `runtime_state.go` / `app.go` 注入 AuthSession store
- 要点：字段 `(id, session_id_hash, csrf_token_hash, user_id, expires_at, last_active_at, ip, user_agent, status, deleted_at)`；索引 `(session_id_hash UNIQUE)、(user_id, status)`；session_id 与 csrf 都存 hash 而非明文
- 验收：登录 → 重启 → 仍能复用 session；session 过期自动失效；entstore_test 覆盖 CRUD
- 依赖：I1, I2

**[A2.1] Redis 限流原语 + 中间件（3 PD）**
- 状态：部分落地（API key/user RPM + API key TPM 已通过 Redis Lua 原子 enforcement；API key `concurrency_limit` 已通过 Redis ZSet 租约 enforcement；p99 benchmark 待后续补齐）。
- 目标：API Key 的 RPM/TPM/并发限制在请求入口被真正 enforce
- 文件：新增 `apps/api/internal/platform/ratelimit/{limiter.go,scripts.go}`；改 `runtime_gateway_core.go` 在 admission 后、Scheduler 前检查 RPM/TPM 限额；改 `runtime_http_helpers.go` / `server.go` 在 Gateway API key 鉴权后获取并释放并发租约；改 `app.go` / `server.go` 注入 Redis limiter
- 要点：Lua 脚本原子化"判定 + 扣减 + 写回"；Key 模板 `srapi:rl:{kind}:{owner_id}:{window}`；超限返回 429 + `Retry-After`；并发用 Redis ZSet + 过期清理
- 验收：`TestGatewayEnforcesAPIKeyRPMLimit` 证明第二个 Gateway 请求被 429 拒绝；`TestGatewayEnforcesAPIKeyConcurrencyLimit` 证明并发请求被 429 拒绝且释放后可恢复；`ratelimit` 单测证明 TPM 超限时 RPM 不发生部分扣减，并覆盖 ZSet 租约释放与过期；p99 benchmark 仍待补齐
- 依赖：I2

**[A2.2] 限流维度装配到 scheduler reject reason（0.5 PD）**
- 状态：已落地（scheduler 已读取 account metadata 的 `rpm_used` / `tpm_used` / `current_concurrency` 与 `rpm_limit` / `tpm_limit` / `max_concurrency`；Gateway 成功用量后会按最近窗口写回 account runtime quota metadata；HTTP 回归证明第二次请求因账号 RPM 被 scheduler 拒绝并记录 `rpm_limit_exceeded`）。
- 目标：scheduler 在候选过滤阶段也拒绝超限的账户，避免重复计费
- 文件：`apps/api/internal/modules/scheduler/service/service.go:190` 附近 reject reason 列表加 `rpm/tpm_limit_exceeded`、`concurrency_full`
- 要点：从 candidate metadata 读 rpm_used/rpm_limit；reject 计入 SchedulerDecision.rejected_count
- 验收：`scheduler/service` 单测覆盖 runtime limit 和 capability reject；`TestGatewayUpdatesAccountRuntimeQuotaMetadataForScheduler` 覆盖真实 Gateway usage → account metadata → scheduler `rpm_limit_exceeded` 路径
- 依赖：A2.1

**[A3.1] accounts.ProbeAccount() 探测方法（2 PD）**
- 状态：已落地（`accounts/service.ProbeAccount` 会解密凭证、调用 provider adapter probe、聚合历史快照、写入 `AccountHealthSnapshot`，并维护 cooldown/circuit metadata）。
- 目标：主动向上游发轻量请求（如 `/v1/models`）测健康
- 文件：`apps/api/internal/modules/accounts/service/service.go` 加 `ProbeAccount(ctx, account) (HealthSnapshot, error)`
- 要点：复用 `provider_adapters` 已有的 client；探测策略：success_rate / latency_p50 / latency_p95 滚动平均；2xx=健康，4xx/5xx=错误，网络异常=timeout
- 验收：mock 一个 fake upstream，验证 probe 能正确分类
- 依赖：无（与 A4 并行）

**[A3.2] health_probe worker（2 PD）**
- 状态：已落地（`internal/workers/health_probe` 按配置周期运行、支持并发上限/超时/优雅关闭，`app.go` 在持久化 account/provider store 可用时启动；测试覆盖只探测 active API-key account 并写回 snapshot/metadata）。
- 目标：每 5 分钟遍历 status=active 的 account 调 Probe，更新 AccountHealthSnapshot
- 文件：新增 `apps/api/internal/workers/health_probe/worker.go + worker_test.go`（照搬 retention 模板）；`app.go` 注册启动
- 要点：worker 用 sync.WaitGroup + ctx.Done() 优雅退出；并发受 `ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT` 限制；3 连续错误 OR error_rate > 50% → 写入 unhealthy/open-circuit 快照和 cooldown metadata
- 验收：fake 一批账户，跑 worker 一次，验证 snapshot 写入；状态机用单测覆盖
- 依赖：A3.1

**[A4.1] ScheduleResult 改为返回候选排序列表 + Decision 加 fallback 字段（1.5 PD）**
- 状态：已落地（`ScheduleResult.Candidates`、`fallback_from_decision_id` Ent/PostgreSQL/OpenAPI/SDK 字段、attempt-aware memory/Redis lease update、fallback_excluded reject evidence 与单测）。
- 目标：Schedule 一次返回 N 个排序好的候选，gateway handler 失败可换下一个
- 文件：`apps/api/internal/modules/scheduler/contract/contract.go` 的 `ScheduleResult` 加 `Candidates []Candidate`；ent schema `schedulerdecision.go` 加 `fallback_from_decision_id` 字段 + `000004_scheduler_decision_fallback_field` 迁移
- 要点：`Selected` 仍指 candidates[0]，向后兼容；评分排序保留；attempt_no 沿用
- 验收：单测验证 ranking；OpenAPI 不破坏（新字段加在响应里，可选）
- 依赖：I1

**[A4.2] gateway handler retry loop（1.5 PD）**
- 状态：已落地（text / Responses / Messages / Embeddings / Gemini GenerateContent Gateway handler 复用 ranked candidates，遇到 retryable upstream 429/5xx/timeout/network/auth/runtime risk error 会排除失败账号后递增 `attempt_no` 重新调度；失败和成功 attempt 都写入 `usage_logs`；fallback decision 通过 `fallback_from_decision_id` 串联；`/metrics` 暴露 `srapi_gateway_failover_total`；HTTP 回归覆盖 503 → 第二供应商成功）。
- 目标：上游 5xx / 网络错误 / 凭证失效自动切到下一个候选
- 文件：`apps/api/internal/httpserver/runtime_gateway_handlers.go`（多处）把单次调用改为 `for attempt := 1; attempt <= MAX && !ok; attempt++` 循环
- 要点：retry 条件白名单（429 / 5xx / 网络超时 / `needs_reauth`）；流式请求只在第一个 byte 之前可 retry；attempt_no 写入 Decision 与 metric
- 验收：模拟一家供应商返回 503，自动切到第二家；指标 `srapi_gateway_failover_total` 可见
- 依赖：A4.1

**[A5.1] generic-reverse-proxy adapter（2 PD）**
- 状态：已落地（`provider_adapters/service/generic_reverse_proxy.go` 已支持 text/chat、streaming、embeddings、custom path/header/body/response mapping；API-key runtime 走普通 HTTP client，非 API-key runtime 走 Reverse Proxy Runtime；OpenAPI enum 已包含 `generic-reverse-proxy`；adapter 单测覆盖 chat/stream/embeddings/custom runtime，HTTP 回归覆盖 admin 创建 generic provider 后经 Gateway/Scheduler/credential materialization 调用配置型上游并记录 usage evidence）。
- 目标：任意 OpenAI 兼容上游只需配置就能接入，无需写代码
- 文件：`apps/api/internal/modules/provider_adapters/service/generic_reverse_proxy.go`；`service.go:36-73` adapter 路由表加一项
- 要点：从 `provider.metadata` 读 `{base_url, auth_header_template, body_mapping_rules, response_path_rules}`；用 `http.Client` + utls（如已引入）；流式响应直 pipe
- 验收：用一个测试上游（mock OpenAI 兼容服务）跑通 chat / embeddings / streaming
- 依赖：I2

**[A5.2] 8 家供应商 preset 库（1.5 PD）**
- 目标：一键种入 DeepSeek/Kimi/通义/智谱/Grok/Mistral/Groq/Together 8 个 provider preset
- 文件：新增 `apps/api/internal/modules/providers/preset/presets.go`（Go 常量结构体数组）+ 管理 API `POST /api/v1/admin/providers/preset/install`
- 要点：每个 preset = {provider 元数据, capabilities map, default adapter_type=`openai-compatible` 或 `generic-reverse-proxy`, base_url, suggested model mappings}；安装后默认 status=`disabled`，需管理员手动 enable
- 验收：调用 install API 后 8 个 provider 出现在数据库；用 `/admin/providers/{id}/test` 验证至少 3 个能连通
- 依赖：A5.1

### 线 B — 支付计费闭环

**[B1.1] UsageLog.cost 回写（0.5 PD）**
- 目标：每条用量真正记录成本，下游聚合才能扣费
- 文件：`apps/api/internal/httpserver/runtime_gateway_core.go` `recordGatewayUsage()`（含 fallback 路径）
- 要点：把 `gatewayPricingEvidence.Amount` 直接赋给 `UsageLog.cost`；若 `PricingSource == default_zero` 时打 warn 日志而非默认 0；保留 `pricing_evidence_json` 元数据
- 验收：跑一次 chat completion，DB 中 usage_logs.cost 非 0
- 依赖：I2

**[B1.2] balance_charger worker（聚合 UsageLog → BillingLedger，2 PD）**
- 目标：周期性把"已经发生但未扣费"的 usage 转成 BillingLedger.usage_charge 并扣减 user.balance
- 文件：新增 `apps/api/internal/workers/balance_charger/worker.go`；`billing/service/service.go` 加 `ChargeUsage(req) (ledgerID, balanceAfter)` 方法
- 要点：每 1 分钟 SELECT charged_at IS NULL 的 usage_logs；按 user_id 聚合金额 → 调 `billing.ChargeUsage`（事务内 update balance + insert ledger + update usage_log.charged_at）；余额不足 → 触发 `users.suspend` + 写 audit
- 验收：跑一个 chat 调用 → 等 1 分钟 → balance 减少、ledger 出现一条 usage_charge
- 依赖：B1.1

**[B1.3] `GET /me/balance` API + PricingRule 导入 API（1 PD）**
- 目标：用户能查余额；管理员能批量导入定价
- 文件：`apps/api/internal/httpserver/runtime_user_handlers.go` 加 `handleMeBalance`；`runtime_admin_*` 加 `POST /api/v1/admin/pricing/rules:bulk`
- 要点：余额查询直读 User.balance；定价导入接受 CSV 或 JSON 数组 + dry-run 模式
- 验收：调用 GET /me/balance 返回正确余额；导入 50 条规则成功
- 依赖：B1.2

**[B2.1] Outbox dispatcher worker（2 PD）**
- 目标：让已经入队的 `PaymentOrderPaid` 等域事件真正被消费
- 文件：新增 `apps/api/internal/workers/events_dispatcher/worker.go`；`events/service/service.go` 已有 DispatchPending，只需周期调用 + 注册 handler
- 要点：在 worker 启动时注册各 module 的 handler（payment.paid → subscription.activate + affiliate.AccrueRebate；refund → affiliate.CompensateRefund）；DispatchPending 内部已支持 attempt_count、next_retry_at 指数退避
- 验收：手动写一条 PaymentOrderPaid 到 outbox，worker 跑一遍 → inbox 出现处理记录 → subscription 被激活
- 依赖：I2

**[B2.2] payment ↔ affiliate ↔ subscription 联动 handler（1.5 PD）**
- 目标：payment.paid 一发，affiliate accrue / subscription activate 都自动触发
- 文件：新增 `apps/api/internal/modules/payments/service/event_handlers.go`（或就近）；接入 `events_dispatcher` 启动注册
- 要点：handler 用 inbox idempotency 防重；失败走 outbox 重试；refund 路径同样要触发 `affiliate.CompensateRefund` 与 `billing.Record(type=refund)`
- 验收：完整跑一次 mock 充值流程，验证 ledger / subscription / affiliate ledger 三者都被写
- 依赖：B2.1

**[B3.1] order_expirer worker（0.5 PD）**
- 目标：清理 expires_at 之前未支付的订单
- 文件：新增 `apps/api/internal/workers/order_expirer/worker.go`
- 要点：每 5 分钟扫 status=pending AND expires_at<now → 状态机转 `closed`；写 PaymentAuditLog
- 验收：mock 一个 1 分钟过期的订单，等 5 分钟，状态自动变 closed
- 依赖：I2

**[B3.2] subscription_expirer worker（0.5 PD）**
- 目标：到期订阅自动转 expired
- 文件：新增 `apps/api/internal/workers/subscription_expirer/worker.go`
- 要点：每 1 小时扫 status=active AND expires_at<now → expired；触发 entitlement 重算事件
- 验收：mock 过期订阅，跑一遍，状态正确
- 依赖：I2

**[B4.1] Stripe SDK 接入（3 PD）**
- 目标：能真正发起 Stripe Checkout 并接收 webhook
- 文件：`go.mod` 加 `stripe/stripe-go/v78`；新增 `apps/api/internal/modules/payments/providers/stripe/provider.go` 实现 PaymentProvider 接口（CreateCheckout / VerifyWebhook / QueryStatus）
- 要点：复用现有 HandleWebhook 流程，只把"签名验证 + 状态映射"接到 stripe SDK；secret 走 `decryptConfig` 注入
- 验收：用 Stripe test mode 完整跑一次充值 → webhook → balance 增加
- 依赖：B2.1 + B2.2

**[B4.2] Alipay SDK 接入（4 PD）**
- 目标：能真正发起支付宝当面付/H5/PC 网页支付
- 文件：`go.mod` 评估加 `smartwalle/alipay/v3`（最成熟开源库）；新增 `payments/providers/alipay/provider.go`
- 要点：支付宝 RSA2 签名走 SDK；config 含 app_id / private_key / alipay_public_key（加密存）；优先支持"扫码支付"+"H5"两个常用场景
- 验收：用支付宝沙箱完整跑一次充值；webhook 验签 + 金额校验
- 依赖：B4.1（沿用 PaymentProvider 抽象）

**[B4.3] WeChat Pay SDK 接入（4 PD）**
- 目标：能真正发起微信扫码 / Native / JSAPI 支付
- 文件：`go.mod` 加 `wechatpay-apiv3/wechatpay-go`；新增 `payments/providers/wechat/provider.go`
- 要点：微信 V3 用 APIv3 + 平台证书自动轮换；config 含 mch_id / api_v3_key / serial_no / private_key
- 验收：用微信沙箱完整跑一次扫码支付；platform_cert 自动 refresh
- 依赖：B4.2

### 线 C — 可观测性 + 多租户

**[C1.1] OpenTelemetry SDK 接入（2 PD）**
- 目标：分布式链路追踪，请求经过的所有 service / store 都有 span
- 文件：`go.mod` 加 `go.opentelemetry.io/otel`、`otel/sdk`、`otel/exporters/otlp/otlptrace/otlptracegrpc`；新增 `apps/api/internal/platform/otel/tracer.go`；`app.go` 启动时初始化全局 TracerProvider
- 要点：env 控制 OTLP endpoint + sampling rate；scheduler.Schedule / payment.HandleWebhook / accounts.Probe 等关键路径手动 span；HTTP 中间件自动包 root span
- 验收：跑一个 chat completion → 在本地 Jaeger / Tempo 看到完整 trace
- 依赖：I2

**[C1.2] slog 装饰器自动注入字段（1 PD）**
- 目标：所有日志自动带 request_id / user_id / api_key_id / trace_id
- 文件：新增 `apps/api/internal/platform/logger/context_handler.go`（slog.Handler 装饰器）；`runtime_state.go` 替换默认 logger
- 要点：从 context 提取字段；保留 child logger 模式；零分配（用 LogValuer）
- 验收：所有日志看到完整 trace 字段
- 依赖：C1.1

**[C2.1] Prometheus client 替换手写文本 + histogram（1.5 PD）**
- 目标：标准 Prometheus 数据类型 + p50/p95/p99 分位数
- 文件：`go.mod` 加 `prometheus/client_golang`；`apps/api/internal/httpserver/runtime_metrics.go:15-113` 重写：counter / histogram / gauge 用官方 SDK；histogram bucket `[0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]` 秒
- 要点：现有 20 个指标全保留语义；新增 `srapi_gateway_request_duration_seconds_bucket`、`srapi_provider_probe_latency_seconds`
- 验收：`/metrics` 输出标准 Prom 文本；Grafana 能渲染 p99
- 依赖：无

**[C2.2] SLO breach 检测 worker（2 PD）**
- 目标：让 `ObsAlertEvent` / `ObsSloDefinition` 表真正用起来，breach 时写告警事件
- 文件：新增 `apps/api/internal/workers/slo_evaluator/worker.go`；新增 `modules/operations/service` 中 `EvaluateSLO()`
- 要点：每 1 分钟跑一次；query Prometheus 或直接读 UsageLog 聚合；breach 时写 ObsAlertEvent + 可选 webhook 通知
- 验收：mock 一个高错误率，1 分钟内出现 alert event
- 依赖：C2.1

**[C3.1] Workspace ent schema + User.workspace_id（2 PD）**
- 目标：建立多租户的"组织/工作区"一级实体（先不上 Org）
- 文件：新增 `apps/api/ent/schema/workspace.go`；改 `user.go` 加 `workspace_id` FK；改 `apikey.go` 加可选 `workspace_id`；迁移 `000003_workspaces.up.sql`
- 要点：默认每个 user 自动 attach 一个"个人 workspace"避免破坏现有数据；APIKey 不指定 workspace_id 则继承 user 的
- 验收：现有 e2e 测试不破；新 schema 字段全 nullable；迁移幂等
- 依赖：I1, I2

**[C3.2] Role.permissions JSON + Entitlement 表（1.5 PD）**
- 目标：建立可扩展权限系统 + 把 subscriptions 的 entitlement_snapshot 升级为独立表
- 文件：改 `apps/api/ent/schema/role.go` 加 `permissions_json`；新增 `apps/api/ent/schema/entitlement.go`（user_id, scope_type, scope_id, feature_key, quota_limit, expires_at, source_subscription_id）
- 要点：permissions 是 `["resource:action"]` 字符串数组；Entitlement 是查询缓存层，由 subscription_activator 写入
- 验收：用 admin API 创建一个角色带 `["payment_order:read"]` 权限并赋给用户，能限制 API 访问
- 依赖：C3.1

**[C3.3] content_safety 起步模块（2 PD）**
- 状态：已落地（`internal/modules/content_safety` 正则检测 + Gateway admission hook + audit/usage evidence；HTTP 回归证明包含 PII 和 prompt injection 的 Chat Completions 请求在上游 dispatch 前被脱敏，audit 记录不泄漏原始值）。
- 目标：在 gateway 链路上挂 PII regex + 简单 prompt-injection 黑名单
- 文件：新增 `apps/api/internal/modules/content_safety/{contract,service}`；在 HTTP runtime 的 Gateway admission 阶段对 CanonicalRequest 做无状态扫描
- 要点：PII 起步用 regex（邮箱、手机号、SSN、身份证/国民 ID、信用卡）mask；injection 起步用关键词 blacklist（"ignore previous instructions" 等通用规则）warn；hit 后写入 AuditLog，后续再补配置化阈值和 `block / mask / warn` 策略
- 验收：发一个包含 PII 和 prompt-injection 关键词的 prompt，上游只收到 redaction marker；usage/audit 只保存 warning/finding evidence，不泄漏原始 PII
- 依赖：I2

### 杀手锏 K1 — Cost-Quality-Latency Pareto 路由

**[K1.1] costScore + cacheScore 实算（2 PD）**
- 目标：scheduler 评分模型 10 维度里这俩从硬编码 0 / 未实现 → 真实算法
- 文件：`apps/api/internal/modules/scheduler/service/service.go:472-682`（新增 `func costScore(c Candidate) float64` 与 `cacheScore`）
- 要点：costScore = 1 - normalize(prevailing_cost_per_1k_tokens, 历史窗口 min/max)；cacheScore = 历史 prompt prefix 命中率（按 account+model 维度的滚动窗口）
- 验收：单测覆盖边界（无历史、全命中、全 miss）；指标 `srapi_scheduler_cost_score_avg` 出现
- 依赖：B1.1（要有真实 cost 数据）

**[K1.2] strategy_loader（DB → runtime，2 PD）**
- 目标：策略从代码 hardcode → 从 `SchedulerStrategy` 表加载，可热更新
- 文件：新增 `apps/api/internal/modules/scheduler/service/strategy_loader.go`；改 `StrategyRegistry` 加 `Refresh(ctx)`
- 要点：解析 `config_json` 为权重 map + filter list；version 字段支持 A/B；activated_at 排序
- 验收：在 DB 改一条策略 config，1 分钟内 scheduler 决策权重变化
- 依赖：无

**[K1.3] 补齐 5 个缺失内置策略（1.5 PD）**
- 目标：spec 设计的 9 策略全实现：latency_first / quota_protect / sticky_first / cache_affinity_first / premium_quality
- 文件：`scheduler/service/service.go` 注册表加 5 个 StrategyDescriptor
- 要点：每个策略只调整权重 map，不写新代码；premium_quality 依赖 K1.4 quality score
- 验收：`ListStrategies()` 返回 7 个；用每个跑 schedule 单测
- 依赖：K1.1, K1.2

**[K1.4] QualityEval 模块 + worker（4 PD，杀手锏核心）**
- 目标：用 LLM-as-judge 对历史决策的输出做质量评分，反馈回 scheduler
- 文件：新增 `apps/api/ent/schema/qualityevaluation.go`（decision_id, sample_request_hash, judge_model, score, rubric_json, judged_at）+ 迁移；新增 `apps/api/internal/modules/quality_eval/{contract, service}`；新增 `apps/api/internal/workers/quality_eval/worker.go`
- 要点：每小时随机抽样 1% 已完成 SchedulerFeedback；调一个评判模型（先用 GPT-4o-mini 简单 rubric：correctness / coherence / safety 三档 0-5 分）；写 QualityEvaluation；scheduler 在 score 时按 account+model 聚合平均
- 验收：跑 100 个真实请求，1 小时后 ~1% 出现 QualityEvaluation 记录；scheduler decision 中 quality 维度非零
- 依赖：K1.1, B1.1

**[K1.5] Pareto optimizer（2 PD）**
- 状态：已落地（`apps/api/internal/modules/scheduler/service/pareto.go`；Scheduler 先筛 Cost/Latency/Quality Pareto 前沿，再在前沿内按策略加权分排序；前沿账号写入 `Decision.Scores["pareto"].frontier_account_ids`；测试覆盖被支配候选即使加权分更高也不会胜出，且所有可用候选仍保留在 failover rank 中）。
- 目标：从简单加权求和升级为多目标 Pareto 支配筛选
- 文件：新增 `apps/api/internal/modules/scheduler/service/pareto.go`
- 要点：输入 candidates + (cost, latency, quality) 三维；输出 Pareto 前沿子集（任一维度上没有被其他候选支配）；只有双方都有明确输入的目标才参与支配判断；然后再用 strategy 权重在前沿内排序
- 验收：mock 3 个候选（A 最便宜慢、B 最快贵、C 中等），Pareto 前沿应包含 A 和 B、排除被支配的
- 依赖：K1.1, K1.4

**[K1.6] strategy_simulator（dry-run + shadow，2 PD）**
- 状态：已落地（单请求 dry-run + shadow strategy、稳定 rollout 预览、真实调度的脱敏 request/candidate snapshot 持久化、`POST /api/v1/admin/scheduler/replay` 批量历史回放 API、以及 Admin Settings 驱动的真实 Gateway 流量稳定百分比分流已完成；真实分流当前支持 model / API key prefix hash 作用域，user_group / provider 独立 scope 属于后续扩展）。
- 目标：能用请求 profile 假装用新策略跑一遍，对比 current vs shadow 的选择效果
- 文件：新增 `apps/api/internal/modules/scheduler/service/simulator.go`；API `POST /api/v1/admin/scheduler/simulate` 和 `POST /api/v1/admin/scheduler/replay`；新增 `scheduler_request_snapshots` 作为历史 replay 证据源
- 要点：使用同一 RequestProfile 和候选集重算两个策略的评分，不真调用上游、不写 SchedulerDecision、不获取 Lease；输出 current/shadow winner 与 final/cost/latency/quality/risk delta；历史 replay 仅声称覆盖有 `scheduler_request_snapshots` 的决策
- 验收：服务层和 HTTP 回归证明 shadow decision 与 historical replay 不创建 decision/lease，且 cost_saver 可在同一候选集下相对 balanced 改变 winner 并返回 diff / 汇总
- 依赖：K1.2

**[K1.7] /admin/ops/strategy 前端策略对比仪表盘（3 PD）**
- 状态：已落地（`/admin/ops/strategy` 复用生成 TypeScript SDK 调用 `POST /api/v1/admin/scheduler/replay`，展示 replay 过滤、汇总指标、Recharts current-vs-shadow score 曲线、winner 分布和逐 snapshot 证据；未新增后端 handler，后端能力由 K1.6.3 historical replay API 承担）。
- 目标：管理员可视化对比两个策略在同一时间窗口的成本/延迟/质量
- 文件：`apps/web/src/app/admin/ops/strategy/page.tsx` + `apps/web/src/components/admin/admin-strategy-page.tsx`；`apps/web/src/lib/admin-api.ts` 和 `apps/web/src/hooks/admin-queries.ts` 接入 replay SDK；`apps/web/src/components/layout/AdminSidebar.tsx` 暴露入口
- 要点：复用 K1.6.3 脱敏 snapshot replay 数据源；图表用 Recharts；过滤维度当前覆盖 strategy / time range / limit / rollout percent / model / request_id；provider 维度待 snapshot replay contract 暴露 provider filter 后补齐
- 验收：浏览器打开页面并执行 replay 后，能看到 current/shadow 两条策略分数曲线、winner change 汇总和 replay evidence 表；无 mock 数据
- 依赖：K1.6

## F. 数据库迁移序列（依次落库）

```
000001 initial_schema                       （已有）
000002 auth_sessions                        （A1.1）
000003 api_key_concurrency_limit            （A2.1）
000004 scheduler_decision_fallback_field    （A4.1）
000005 usage_log_attempts                   （A4.1 fallback evidence）
000006 scheduler_request_snapshots          （K1.6 replay evidence）
next   workspaces_and_user_workspace_id     （C3.1，未落地）
next   role_permissions_and_entitlements    （C3.2，未落地）
next   quality_evaluations                  （K1.4，未落地）
next   usage_logs_charged_at_index          （B1.2 性能索引，未落地）
```

每个迁移配套同名 `down` 脚本；每次合并前 `make migration-check` 必须通过。当前仓库使用 split 目录命名：`postgres/up/000002_auth_sessions.sql` 与 `postgres/down/000002_auth_sessions.sql`。

## G. 关键文件路径清单（新增 / 修改）

### 新增（按模块）

- `apps/api/atlas.hcl`（I1）
- `apps/api/ent/schema/{authsession,workspace,entitlement,qualityevaluation}.go`（A1, C3.1, C3.2, K1.4）
- `apps/api/internal/persistence/entstore/auth/store.go`（A1.1）
- `apps/api/internal/platform/{ratelimit,otel}/`（A2.1, C1.1）
- `apps/api/internal/platform/logger/context_handler.go`（C1.2）
- `apps/api/internal/modules/payments/providers/{stripe,alipay,wechat,easypay}/provider.go`（B4.1-3）
- `apps/api/internal/modules/payments/service/event_handlers.go`（B2.2）
- `apps/api/internal/modules/provider_adapters/service/generic_reverse_proxy.go`（A5.1）
- `apps/api/internal/modules/providers/preset/presets.go`（A5.2）
- `apps/api/internal/modules/content_safety/{contract,service,store/memory}/`（C3.3）
- `apps/api/internal/modules/quality_eval/{contract,service}/`（K1.4）
- `apps/api/internal/modules/scheduler/service/{pareto,simulator,strategy_loader}.go`（K1.2/5/6）
- `apps/api/internal/workers/{health_probe,events_dispatcher,balance_charger,order_expirer,subscription_expirer,slo_evaluator,quality_eval}/worker.go`（A3.2, B2.1, B1.2, B3.1, B3.2, C2.2, K1.4）
- `apps/api/internal/httpserver/middleware_ratelimit.go`（A2.1）
- `apps/web/src/app/admin/ops/strategy/page.tsx` + 子组件（K1.7）

### 修改（按文件）

- `apps/api/go.mod`（新依赖：stripe-go, alipay-sdk, wechatpay-go, otel suite, prom client）
- `apps/api/Makefile` / 根 `Makefile`（migration-diff 目标）
- `apps/api/internal/httpserver/runtime_state.go:100, 136, 160-300`（装配切换）
- `apps/api/internal/httpserver/server.go:352`（中间件链）
- `apps/api/internal/httpserver/runtime_metrics.go:15-113`（Prom client 重写）
- `apps/api/internal/httpserver/runtime_gateway_core.go`（recordGatewayUsage cost 回写、handler retry loop）
- `apps/api/internal/httpserver/runtime_gateway_handlers.go`（retry loop）
- `apps/api/internal/app/app.go`（注册 6+ worker）
- `apps/api/internal/modules/scheduler/{contract,service}/`（K1.1-3, A4.1）
- `apps/api/ent/schema/{user,apikey,role,schedulerdecision}.go`（加字段）

## H. 验收策略

### 单元测试
- 每个新增 service / worker 至少 80% 行覆盖率
- 关键算法（costScore / cacheScore / Pareto / 状态机）边界值用 table-driven

### 集成测试
- 复用现有 `make check`（含 lint / codegen drift / typecheck / api-test / examples-check / secret-scan / web-check）
- 新增 4 个 smoke：
  1. `smoke-payment-stripe`：Stripe test mode 完整充值闭环
  2. `smoke-failover`：模拟一家上游 503，请求自动切到第二家
  3. `smoke-rate-limit`：超 RPM 返回 429
  4. `smoke-quality-eval`：100 个请求后 QualityEvaluation 出现记录

### 性能基准
- 限流中间件 p99 ≤ 2ms（Redis pipeline + Lua）
- OTel 启用后整体 p99 增加 ≤ 5ms
- balance_charger worker 在 10k usage_log/min 下处理无积压

### 手工验证
- `make dev-up` 启动 → 前端 admin 走一遍：登录、创建 API Key、充值（Stripe test）、调一次 chat、看用量、看余额、看决策审计、看健康仪表盘

## I. 工作量估算

| 主线 | PD 合计（不含测试） | 含测试 + 文档 |
|---|---|---|
| 基础设施 I1+I2 | 2.5 | 4 |
| 线 A | 16 | 24 |
| 线 B | 19 | 28 |
| 线 C | 12 | 18 |
| 杀手锏 K1 | 16.5 | 24 |
| **合计** | **66** | **98 PD** |

- **1 人全职 (5 PD/周)**：≈ 20 周（5 个月）
- **2 人并行（按主线分工）**：≈ 10-12 周（2.5-3 个月），关键路径在 B1→B2→K1.4
- **里程碑**：
  - 第 4 周末：I1+I2 + 线 A（A1-A3）+ B1（cost 回写 + 余额扣费）完成 → 可邀请 Beta 用户用免费额度
  - 第 8 周末：线 B 全完 + 线 C OTel/Prom/Workspace → 可开始收费
  - 第 12 周末：K1 杀手锏完整落地 → 真正"超越 sub2api"

## J. 退出 Plan Mode 后第一刀

建议合并第一刀 PR：**I1 + I2 + B1.1**（增量迁移工作流 + 装配切换 + UsageLog.cost 回写）——3 项加起来 ~4 PD，能在 1 周内把"持久化 + 计费数据真实"两个最大的演示感来源同时解决，且不引入任何新外部依赖。
