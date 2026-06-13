# Goal：并发安全地基 —— worker leader-gate + balance_charger/outbox/idempotency 竞态收口（第十六批 · NFR）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：16-agent NFR 审计判定「SRapi 正确性地基扎实，但多副本就绪度有确定性炸弹」。本批清掉**多副本第一个硬阻塞**：`startWorkers()`（app.go:613）在每个 App 进程无条件 `.Start()` 全部 14 个周期 worker，全仓零 leader-election（`grep workers/` 命中 0 个 advisory-lock/Redis SetNX/leader）。一旦 `replicas>1`：(1) `quota_refresh`/`health_probe`/`connectivity_test`/`scheduled_test` 各自 `accounts.List()` 后对**同一批上游账号**发探测/拉额度 —— N 副本 = N 倍上游 QPS，极易触发上游 429/风控封号，直接砸掉系统赖以生存的账号池（这是与系统核心目标「保护账号健康」直接相悖的最高优先级阻塞）；(2) `retention`/`slo_evaluator` 各副本重复全表扫 usage_logs，DB 负载 ×N；(3) `balance_charger` 多副本选到同一批待扣日志、同时按 user 开 Serializable 扣同一余额 → 40001 序列化风暴、CPU/锁等待放大；(4) `outbox` 双副本各发一份邮件类通知（密码重置/验证/告警重复送达）。注意：**计费扣费本身在存储层已多副本安全** —— `ChargeUsage` 用 Serializable 隔离 + `charged_at IS NULL` 在 select 与 `UpdateMany` 两端做条件 claim + 行数校验（billing/store.go:133 开事务、:220 条件 `Update().Where(IDIn, ChargedAtIsNil)`），即使多副本同时扣同一批日志也不会重复扣费。本批**不动其正确性语义**，只去掉效率退化（序列化风暴/重复全表扫）与副作用重复（重复探测/重复发邮件）。
> 前置依赖：无前置批次依赖。本批是 program 中四个 NFR 批的第一批，可独立落地。复用现成原语：`migrator.go:67` 的 `pg_advisory_lock` 范式（迁移已用它串行化、可重入、失败拒启动）与 `platform/redis` 客户端。
> 关联：本批是 **batch17 部署就绪**（解锁 `replicas>1`）的**硬前提** —— 没有 leader-gate 就解锁多副本 = 立即触发 N 倍上游探测，故 batch17 的「解锁多副本」必须确认本批已就位（见该批 rank16 部署文档护栏）。与 batch18（热路径去全表扫）协同：本批建立的有界 worker/leader-gate 框架将被 batch18 rank6 的异步记账队列复用。与 batch19（容量治理）协同：batch19 的 retention 分批删除（rank8）、slo_evaluator DB 聚合（rank13）依赖本批的 leader-gate 保证「单副本删除/单副本评估」。

---

<role_and_context>
你是 SRapi 仓库的资深后端工程师 / SRE。SRapi 是一个对标 sub2api 的 AI 网关/计费平台。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first：先改 `packages/openapi/openapi.yaml` 再生成；schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`）。本批的核心面是**进程内编排与持久化层并发**：
- 应用引导 `apps/api/internal/app/`（`App` 持有全部 14 个 worker，`startWorkers()`/`stopWorkers()` 生命周期编排，app.go:613/123）。
- 14 个周期 worker 在 `apps/api/internal/workers/`：`outbox` / `retention` / `auth_session_cleanup` / `idempotency_cleanup` / `order_expirer` / `subscription_expirer` / `quota_refresh` / `balance_charger` / `health_probe` / `quality_eval` / `slo_evaluator` / `connectivity_test` / `scheduled_test` / `account_quota_alert`。
- 平台基建 `apps/api/internal/platform/{db,redis,otel,ratelimit}`：`db.migrator` 用 `pg_advisory_lock` 串行化迁移（migrator.go:67，**本批可复用的 advisory-lock 范式**）；`redis` 客户端承载 rate-limit/并发槽/scheduler lease/session affinity 这些热路径原语；`ratelimit` 已用 Redis 原子 Lua 脚本（跨副本天然一致，本批不动）。
- 持久化 `entstore`（Postgres）+ `redisstore`。本批触及 `entstore/{billing,events,idempotency}/store.go`。
- 前端 Next.js + TypeScript（apps/web）—— 本批基本不涉及前端。
- 部署 `deploy/`（docker-compose + nginx + prometheus/tempo/alertmanager）。本批不改部署文件，但产物（leader-gate）是 batch17 解锁多副本的前提。
本地开发：`make dev-up`。登录 admin@srapi.local / Admin1234。
架构红线（由两个守门测试 + gofmt 强制）：跨模块只允许 import 目标模块的 `contract` 层；`contract` 禁止 import Ent/生成 DTO/HTTP server；worker 只能 import 模块 contract/service；Ent/Redis store 只能 import 模块 contract；单文件 `maxRuntimeFileLines=2200`、单函数 `maxProductionFuncLines=210`；gofmt 必须全过；Go 1.26.3。
**本批维度强调点（并发/多副本正确性）**：leader-gate 是「进程内编排 + 分布式协调」工作，不是业务模块 —— 通用 leader-gate 包须放 `apps/api/internal/platform/`（与 db/redis 同级的基础设施层）或 `internal/pkg`，**不得**让 worker 直接 import 别模块的 service/store；store 层的原子化只改 `entstore/*/store.go`（合法 import contract）。本批主基调：**计费正确性已多副本安全，本批只修效率退化与副作用重复，绝不触碰扣费的正确性语义。**
</role_and_context>

<objective>
目标（可衡量最终态）：在 `replicas>1` 下，所有「调上游 / 有副作用 / 重复全表扫」的周期 worker 都受分布式单例保护 —— 任一时刻只有一个副本真正执行该 worker 的循环体，其余副本跳过；并且 `balance_charger` 选取、`outbox` 投递、`idempotency` 接管这三处 read-then-act 竞态在存储层原子化，做到「即使 leader-gate 失效也不重复扣费/不重复发邮件/不重复执行被保护请求」的纵深防御。完成后：(a) 两实例同时启动，`pg_advisory_lock` 下只有一个实例的 `quota_refresh`/`balance_charger` 等周期 worker 真正跑循环（日志/计数器可证）；(b) 并发投递同一 outbox 事件只触发一次下游副作用；(c) 并发 Reacquire 同一过期幂等锁只有一方 `OutcomeProceed`。

动机（这些 NFR 风险在规模下的确定性后果）：
- **worker N 倍打上游 → 触发风控封号**：`quota_refresh`（worker.go:190 `accounts.List` 后 :270 `RecordQuotaSnapshot` 逐账号上游探测）等 4 个上游探测类 worker 在 N 副本下对同一批账号发 N 倍探测请求，上游 429/封号是确定性后果 —— 这砸掉的是系统赖以生存的账号池，是「与核心目标直接相悖」的最高优先级阻塞。
- **balance_charger 序列化风暴**：多副本选到同一批待扣日志（`ListPendingUsageCharges` 仅按 `created_at` + `ChargedAtIsNil` 查，无 `FOR UPDATE SKIP LOCKED`，billing/store.go:73）、同时按 user 开 Serializable 扣同一用户余额 → 大量 40001 回滚重试，CPU/锁等待放大；扣费结果正确（条件 claim 保证），但效率退化为序列化风暴。
- **outbox 重复发邮件**：`ListDispatchableOutbox` 无锁（events/store.go:81）、`MarkOutboxPublished` 无 status 守卫（:110），两副本都通过判断都调下游 handler；返佣/退款按 ReferenceID 账本幂等不会重复入账，但邮件层无账本/唯一约束 → 同封密码重置/验证/告警通知被两副本各发一次。
- **idempotency 双接管**：`Reacquire` 对过期锁无条件 UPDATE（idempotency/store.go:53，仅匹配 key/method/path，无 `locked_until<now` 条件），两副本同时检测过期并各自 Reacquire 都 `affected=1` 都返回 `OutcomeProceed` → 重复执行被保护请求。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元，每项带真实文件锚点）：

**A. leader-gate 通用机制 + 全 worker 接入（rank1，核心，必做）**
1. **抽 `leaderGate` 通用包**：复用 `apps/api/internal/platform/db/migrator.go:67` 同款 `pg_advisory_lock` 范式 —— 对每个 worker 名取一个稳定的 64 位锁键（如 `hash(worker_name)`），在 worker 循环开始时 `pg_try_advisory_lock($key)`（**非阻塞** try 版本，拿不到的副本直接跳过本轮该 worker 循环体而非阻塞等待）；持有连接的生命周期与「在 leader」状态对齐，副本退出/崩溃时会话级锁自动释放，下一副本接管。备选实现：Redis `SetNX` + TTL 续租 leader key（复用 `platform/redis`）。两种均可，**默认 `pg_advisory_lock`**（与 migrator 一致、零额外依赖、崩溃自动释放）。
   - 落点：通用 leader-gate 放 `apps/api/internal/platform/leadergate/`（基础设施层，与 db/redis 同级，不违反架构红线）。
   - 形态：一个包裹器（如 `leadergate.Gate.Do(ctx, name, fn)` 或 `leadergate.Guard(name)` 返回「是否在 leader」），让各 worker 的 `RunOnce`/循环体在执行前先 `TryAcquire`，非 leader 直接返回。
2. **全部 14 个周期 worker 接入 leader-gate**（按副作用分级，全部 gate）：
   - 上游探测类（最高危，N 倍打风控）：`quota_refresh`、`health_probe`、`connectivity_test`、`scheduled_test`。
   - 计费类（序列化风暴）：`balance_charger`。
   - 全表扫类（DB 负载 ×N）：`retention`、`slo_evaluator`、`quality_eval`、`account_quota_alert`。
   - 幂等清理/到期类（重复全表扫无害但浪费）：`auth_session_cleanup`、`idempotency_cleanup`、`order_expirer`、`subscription_expirer`。
   - 投递类（重复副作用）：`outbox`。
   - 接入点：`apps/api/internal/app/app.go:613` `startWorkers()` 处统一为各 worker 注入 gate，或在各 `worker.go` 的 `RunOnce` 入口处 gate（二选一，倾向在 worker 内 gate 以便单元测试，`startWorkers` 仅注入 gate 依赖）。

**B. 存储层原子化（纵深防御 —— 即使 leader-gate 失效也安全）**
3. **rank14：balance_charger 多副本安全**（二选一并落定）：
   - 默认 (a)：在 leader-gate 下只一个副本跑 `balance_charger`（A 项已覆盖）；本子项额外修 `RunOnce` 遇 err 直接 `return` 会中断本轮其余批次的问题 —— 改为单批失败记录后继续下批（有上限退避），不让一批 40001 拖垮整 pass。
   - 备选 (b)：若产品要求多副本并行扣费，`ListPendingUsageCharges`（billing/store.go:73）改 `SELECT ... FOR UPDATE SKIP LOCKED`（按 user 分片 claim）或 worker 分片键（`user_id % N`），并对 40001 做有上限退避。
   - **无论 a/b，绝不改 `ChargeUsage` 的 Serializable + 条件 claim 正确性语义**（billing/store.go:133/220）。
4. **rank15：outbox/inbox 去重原子化**：
   - `inbox` 去重 read-then-act 改原子：`RecordInbox` 改 `INSERT ... ON CONFLICT (event_id, consumer) DO NOTHING`，用 `affected==1` 判抢到处理权（已有 `(event_id,consumer)` 唯一索引，本子项把「先读后判」换成「插冲突即让权」）。
   - `MarkOutboxPublished`（events/store.go:110）加 `WHERE status='pending'` 条件更新（乐观守卫）。
   - `ListDispatchableOutbox`（events/store.go:81）可选加 `FOR UPDATE SKIP LOCKED`（若保留多副本投递）。
   - 邮件层加 `(event_id)` 去重表（最后一道防线，防 handler 重入）。
5. **rank23：idempotency Reacquire 乐观条件更新**：`Reacquire`（idempotency/store.go:53）改 `UPDATE SET locked_until=? WHERE key/method/path AND status='in_progress' AND locked_until<now`，用 `affected==1` 判抢到接管权；抢不到按 `OutcomeInFlight` 处理（而非 `OutcomeProceed`）。

明确不做（out-of-scope，均由其它 NFR 批/功能批涵盖，非永久搁置）：
- **不动计费扣费正确性语义**（Serializable + 条件 claim 已多副本安全，本批只去序列化风暴与中断问题）。
- **不做热路径性能优化**（全表扫去除、N+1 收敛、异步记账 → batch18）。
- **不做容量治理**（retention 新表接入、分批删除、索引、availability rollup → batch19；本批只让 retention/slo_evaluator 受 leader-gate 保护，不改其删除/聚合逻辑本体）。
- **不做部署护栏**（Redis 池/超时、真实 readiness、compose limits、解锁多副本文档、k8s manifest → batch17；本批是其前提，但本批不改任何 `deploy/` 文件）。
- 不改 ratelimit/scheduler lease 的 Redis Lua 原子脚本（已跨副本一致）。

> **决策注记（受 open_decisions 影响的默认假设）**：本 program 的默认目标是**支持多副本水平扩展**（见 open_decisions「是否要多副本/水平扩展」），因此 worker leader-gate 是开 `replicas>1` 的**硬前提**而非可选优化；即使运维短期选择长期单副本，leader-gate 也无害（单副本下永远拿得到锁），且为「随时可安全扩容」留好地基。本批默认 `pg_advisory_lock` 实现（与 migrator 一致、崩溃自动释放）。`balance_charger` 默认走 (a) leader-gate 单副本路径（最简、零并行风暴），(b) `SKIP LOCKED` 分片仅在产品明确要求多副本并行扣费时才做。
</scope>

<constraints>
途中**绝不破坏已验证的多副本安全正确性**：
- **计费 `ChargeUsage` 已是多副本安全**（Serializable 隔离 billing/store.go:133 + `charged_at IS NULL` 在 select 与 `UpdateMany` 两端条件 claim billing/store.go:220 + 行数校验），本批**只去掉效率退化（序列化风暴）与 `RunOnce` 中断问题，不得改其扣费正确性语义**。优化前后「待扣日志被扣且仅扣一次、金额一致、账本一致」必须不变。
- 限流/并发槽/scheduler lease 已用 Redis 原子 Lua 脚本（跨副本一致），本批不动。
- 金额一律 string + big.Rat 8 位定点，**绝不改回 float64**；保留 0 价回退防漏收语义。
- 凭证 AES-GCM 加密不得改明文。
- 本批是**并发正确性修复**，不是性能重构，但任何「行为可见」的改动（如 `RunOnce` 失败后继续下批、Reacquire 抢不到改判 `OutcomeInFlight`）必须**行为等价于「单副本下的正确语义」** —— 用并发 store 测试证明「N 副本并发只有一方 `affected=1` / 只发一次邮件 / 只一方 `OutcomeProceed`」，且单副本路径行为不变。
- 不破坏现有 OpenAPI 响应结构（本批基本不动对外契约）。
- ent schema 若改（如 inbox 唯一约束、邮件去重表）：`go generate ./ent/...` 后同步 store/mock（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑）；删枚举值/不可逆迁移先问我。
- 不得修改与本任务无关的 `*_test.go`；新增能力配新测试；并发测试用真实 Postgres（`pg_advisory_lock` 需真库）或 testcontainers，不打真实上游。
其他约束：leader-gate 通用包放 `internal/platform/`（基础设施层），不让 worker 直接 import 别模块 service/store。
</constraints>

<success_criteria>
完成需同时满足（每条给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）。
2. **leader-gate 机制可证（核心，对应 rank1）**：贴出 `leadergate` 包代码 + 一个**两实例并发抢锁测试** —— 两个 gate 实例对同一 worker 名同时 `TryAcquire`，证明只有一个返回「在 leader」、另一个跳过；leader 退出后另一个能接管（advisory 锁会话级自动释放）。贴 `go test ./internal/platform/leadergate/...` 输出。
3. **全 worker 接入可证（对应 rank1）**：贴 `grep -rn` 证明全部 14 个 worker 的循环体/`RunOnce` 入口都过 leader-gate（或 `startWorkers` 处统一注入 gate）；贴一段两实例并发跑 `quota_refresh`/`balance_charger` 的测试或日志，证明上游探测/扣费循环只在 leader 副本执行（计数器/日志：非 leader 副本 `accounts.List` 调用数为 0 或循环体未进入）。
4. **balance_charger 可证（对应 rank14）**：贴并发 store 测试证明两副本并发选取同一批待扣日志、并发 `ChargeUsage` 后该批每条日志 `charged_at` 只被写一次（条件 claim：只有一方 `UpdateMany affected==N`，另一方 `affected==0`），扣费金额/账本一致（**正确性不变**）；并证明 `RunOnce` 单批失败后继续处理其余批次（不中断整 pass）。
5. **outbox/inbox 可证（对应 rank15）**：贴并发测试证明同一 `(event_id, consumer)` 并发 `RecordInbox` 只有一方 `affected==1` 抢到处理权；`MarkOutboxPublished` 在 `status!='pending'` 时 `affected==0` 不重复发布；下游邮件类副作用对同一事件只触发一次（贴 mock 下游调用计数==1）。
6. **idempotency 可证（对应 rank23）**：贴并发测试证明两副本对同一过期锁并发 `Reacquire`，只有一方 `affected==1` 返回 `OutcomeProceed`，另一方 `OutcomeInFlight`（不再双 `OutcomeProceed` 双执行）。
7. **正确性回归不破坏**：`go test ./internal/workers/... ./internal/persistence/entstore/...` 全绿（贴尾部）；尤其 billing/events/idempotency store 的现有测试仍通过（证明只加并发护栏、未改扣费/投递正确性语义）。
8. （如适用）Prometheus counter 不倒退：若为 leader 状态加了可观测指标（如 `srapi_worker_leader{worker=...}`），证明其语义正确（leader 副本=1、非 leader=0），不破坏现有指标。
9. `go test ./...` 全绿（贴尾部）；`gofmt -l` 空；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 **35 turns** 后停止并汇总剩余阻塞项（leader-gate 机制就绪否、14 worker 接入进度、三处原子化各自状态、剩余阻塞及原因）。

> success_hint 细化：两个 App 实例同时启动，`pg_advisory_lock` 下只有一个实例的 `quota_refresh`/`balance_charger` 等周期 worker 真正执行循环（日志/计数器可证）；并发投递同一 outbox 事件只发一次邮件；并发 Reacquire 同一过期锁只有一方 `OutcomeProceed`。`go test ./internal/workers/... ./internal/persistence/entstore/...` 全绿；`go build && go vet` 退出码 0。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit；message 末尾加 Co-Authored-By 行：
1. **阶段一（leader-gate 通用机制先行，核心）**：写 `apps/api/internal/platform/leadergate/` 包（`pg_advisory_lock` 非阻塞 try 版本，复用 migrator.go:67 范式）+ 两实例并发抢锁测试。**其余三项的最简解都依赖它**，故必须先就位。一个 commit。
2. **阶段二（全 worker 接入）**：在 `startWorkers`（app.go:613）注入 gate 依赖 + 各 worker 循环体/`RunOnce` 入口接 gate；先接最高危的上游探测类（quota_refresh/health_probe/connectivity_test/scheduled_test）→ 计费类（balance_charger）→ 全表扫/清理类。每接一组跑 `go build` + 相关测试。一个或按组多个 commit。
   - 注意：本阶段在 worker 文件**增行前**先确认其行数离 `maxRuntimeFileLines=2200` 红线有余量（worker 文件普遍较小，但 `startWorkers` 所在 app.go 若逼近红线，按 batch15 rank59 同款手法先拆同包文件 —— 不过 app.go 与 core 不同，预计无须拆；若需拆则 core 仅余约 44 行的教训提醒：增行前必查行数）。
3. **阶段三（存储层原子化，纵深防御）**：rank14 balance_charger（`RunOnce` 失败继续 + 可选 `SKIP LOCKED`）→ rank15 outbox/inbox 原子化（`INSERT ON CONFLICT DO NOTHING` + `MarkOutboxPublished WHERE status=pending` + 邮件去重表）→ rank23 idempotency Reacquire 乐观条件更新。每项独立 commit + 并发 store 测试（证明只一方 `affected=1`）。
   - schema/机制先行原则：leaderGate（阶段一）先于一切接入；若 rank15 邮件去重表需 ent schema，则该 schema 改在该子项 commit 内先 `go generate` 同步 store/mock。
每阶段一个或多个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch16`）若干语义化 commit +
- `apps/api/internal/platform/leadergate/` 通用 leader-gate 包（`pg_advisory_lock` try 版本 + 两实例并发抢锁测试）；
- `apps/api/internal/app/app.go` `startWorkers` 注入 gate + 全部 14 个 worker 循环体接 gate 的改动；
- `entstore/billing/store.go` `RunOnce` 失败继续（+ 可选 `SKIP LOCKED`，不改扣费正确性）、`entstore/events/store.go` `RecordInbox` `ON CONFLICT DO NOTHING` + `MarkOutboxPublished WHERE status=pending`、`entstore/idempotency/store.go` `Reacquire` 乐观条件更新；
- （如选）邮件 `(event_id)` 去重表 ent schema + 同步 store/mock；
- 新测试：leader-gate 并发抢锁测试 + balance_charger/outbox/inbox/idempotency 的并发 store 等价性测试（证明只一方 `affected=1` / 只发一次 / 只一方 `OutcomeProceed`）；
- progress.txt 收尾（列每个 rank 的最终处置、证据位置、14 worker 接入清单、balance_charger 走 a 还是 b、未做的 out-of-scope 及其归属批次）。
</artifact>

<guardrails>
绝不：删除/改写既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证；**破坏计费多副本安全语义**（`ChargeUsage` 的 Serializable + 条件 claim 是已验证的正确性地基，本批只加 leader-gate 与去序列化风暴，绝不改其扣费正确性）；为并发护栏牺牲扣费/投递正确性还假装等价；伪造未实测的并发安全改善（每条 success_criteria 必须有真实运行的并发测试佐证，不得仅凭推理声称「现在多副本安全」）；在测试里打真实上游（并发测试用真实 Postgres / testcontainers，不向上游发探测）。
先问我：破坏现有 OpenAPI 响应结构；删除现有顶层路由文件；任何不可逆数据迁移；**解锁多副本（`replicas>1`）前确认 leader-gate 已全 worker 就位**（本批就是建立这个前提，本批内不解锁多副本，那是 batch17）；向真实上游发起需真实凭证的调用；删 ent 枚举值的存量迁移。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 证据在哪 / 14 worker 接入清单与勾选 / balance_charger 走 a 还是 b / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，`grep -rn "leadergate\|TryAcquire" internal/workers/ internal/app/` 看接入进度，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：leader-gate 通用机制就绪否（并发抢锁测试红/绿）、14 worker 接入进度（清单逐项勾选）、rank14/15/23 三处原子化各自状态（并发测试红/绿）、计费正确性回归是否仍全绿、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；行数为本批审计时实测，改后会变）：
- **leader-gate 接入点 / 复用范式**：`apps/api/internal/app/app.go:613`（`startWorkers()` 无条件全启 14 worker）/:123（`a.startWorkers()` 调用）；`apps/api/internal/platform/db/migrator.go:67`（`SELECT pg_advisory_lock($1)`，**可复用的 advisory-lock 范式** —— 本批用 `pg_try_advisory_lock` 非阻塞版本）/:72（`pg_advisory_unlock`）。
- **14 个周期 worker**：`apps/api/internal/workers/{outbox,retention,auth_session_cleanup,idempotency_cleanup,order_expirer,subscription_expirer,quota_refresh,balance_charger,health_probe,quality_eval,slo_evaluator,connectivity_test,scheduled_test,account_quota_alert}/worker.go`（各 `Start`/`RunOnce`）。上游探测证据：`apps/api/internal/workers/quota_refresh/worker.go:190`（`accounts.List`）/:270（`RecordQuotaSnapshot` 逐账号上游探测）。
- **rank14 balance_charger**：`apps/api/internal/persistence/entstore/billing/store.go:73`（`ListPendingUsageCharges` 仅 `created_at`+`ChargedAtIsNil`，无 `FOR UPDATE SKIP LOCKED`）/:107（`ChargeUsage`）/:133（`BeginTx Serializable` —— **已多副本安全，勿改语义**）/:220（`Update().Where(IDIn, ChargedAtIsNil).SetChargedAt` 条件 claim + 行数校验 —— **正确性地基**）；`apps/api/internal/modules/billing/service/service.go`（按 user/currency 分组逐批 `ChargeUsage`、`RunOnce` 遇 err 中断本轮其余批次）。
- **rank15 outbox/inbox**：`apps/api/internal/persistence/entstore/events/store.go:81`（`ListDispatchableOutbox` 无锁）/:110（`MarkOutboxPublished` 无 status guard）；`apps/api/internal/workers/outbox/worker.go`（`RecordInbox` 后判 `status!=Processed` 的 read-then-act）；下游邮件副作用见 outbox 的 domain handler；幂等护栏参照点 `apps/api/internal/modules/affiliate/service`（按 ReferenceID 账本幂等的现成范式）。
- **rank23 idempotency**：`apps/api/internal/persistence/entstore/idempotency/store.go:53`（`Reacquire` Update().Where 仅匹配 key/method/path，**无 `LockedUntil<now` 条件**）/:62（`SetLockedUntil`）/:84（`ClearLockedUntil`）；`apps/api/internal/modules/idempotency/service/service.go`（`Begin` 主路径 `InsertOrGet` 唯一约束原子安全，Reacquire 分支是竞态点）。
- **leader-gate 落点**：`apps/api/internal/platform/leadergate/`（新增，基础设施层，与 db/redis/otel/ratelimit 同级，不违反「跨模块只 import contract」红线）。

复用的现成原语（仅借鉴，不抄实现）：
- `migrator.go:67` 的 `pg_advisory_lock` 范式（迁移已用它做分布式串行化、可重入、失败拒启动）—— 本批 leader-gate 用 `pg_try_advisory_lock` 非阻塞变体。
- `platform/redis` 客户端（leader-gate 备选实现 Redis `SetNX`+TTL 续租）。
- `usage.CleanupLogs` 的按 ID 批删模式（batch19 retention 分批删除会复用，本批仅让 retention 受 leader-gate 保护）。
- `dependencyPinger` 接口（batch17 真实 readiness 会用，本批不涉及但同属 platform 层）。
- `health_rollups` 汇总思路（batch19 rank4/22 会用，本批不涉及）。
- events 的 `(event_id, consumer)` 唯一索引（rank15 `INSERT ON CONFLICT DO NOTHING` 依赖它）。

sub2api 仅作能力对照不抄代码：
- `SchedulerCache`（scheduler_cache.go，按 bucket 缓存快照）—— batch18 调度候选缓存会对照，本批不涉及。
- `ops_repo_preagg.go` 预聚合 —— batch19 可观测去全表扫会对照，本批不涉及。
- `outbox`/`deferred_service`/`flusher` 的有界持久队列 —— batch18 异步记账会对照，本批仅修 outbox 投递竞态。
- `Dockerfile.goreleaser` 单二进制 —— batch17 镜像瘦身会对照，本批不涉及。
- 分布式单例/leader-election 的「按任务名取分布式锁、非持有者跳过」思路 —— 本批 leader-gate 的能力对照，仅借鉴架构思路，实现走 SRapi 自有 `pg_advisory_lock` 范式。
</references>
