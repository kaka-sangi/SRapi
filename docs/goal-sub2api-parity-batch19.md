# Goal：容量治理 —— 可观测/管理读路径去全表扫 + 全表 retention 覆盖 + 分批删除 + 索引收尾（第十九批 · NFR 容量/数据增长/存储）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。本文档是 NFR 治理 program 第四批，主题维度=**容量 / 数据增长 / 存储**。
> 背景：8 域对抗性 NFR 核查后，SRapi 正确性地基扎实（计费 ChargeUsage 在存储层已多副本安全：Serializable + `charged_at IS NULL` 在 select 与 UpdateMany 两端条件 claim + 行数校验，`billing/store.go:133/220`；限流/并发槽/调度租约全用 Redis 原子 Lua），但**容量曲线有确定性炸弹**：(1) 可观测与管理读路径仍全表扫——`/metrics` 每 15s 抓取新建 collector，`Collect()` 全表 `usage.List()`(runtime_metrics.go:286) + 全表 `scheduler.ListDecisions()`(:317) + 遍历所有 account 各做 `LatestHealthSnapshotByAccount`(:424/:432 的 N+1)，且所有计数器用 `NewConstMetric` 即时重算(:912)——**retention 删老行后 counter 倒退、违反 counter 单调性、`rate()`/`sum` 失真**；`/v1/usage`(SummarizeAPIKey)、管理用量/账单/审计列表同样全表 `List()` 后 Go 过滤(usage/service.go:174/137/316、billing/store.go:59、audit/store.go:47)；slo_evaluator 每 60s 把 SLO 窗口内（默认 28 天、上限 365 天）全部 usage_logs 读进内存逐行评估(slo_evaluator/worker.go)。(2) 多张高频写表无任何 retention——retention worker 只清 5 张表(retention/worker.go policyFromConfig)，`scheduler_request_snapshots`/`account_quota_snapshots`/`domain_events_outbox+inbox`/`ops_system_logs`/`monitor_run_results` append-only 从不删——**磁盘必然耗尽**。(3) retention 的 `Cleanup` 对每表单条无界 `Delete().Where(CreatedAtLT)`，无 LIMIT/批次(operations/store.go:36-84)——首次启用或长期未清的大表一次性删海量行→长事务、行锁、WAL 暴涨、autovacuum 风暴，期间网关 usage 写入被阻塞。
> 在 N 副本 / 高 QPS / 大数据量下会怎样：`/metrics` 每副本各扫一次 `usage_logs`，多副本=DB 负载×N 且 `rate()` 语义跨副本错乱；`/v1/usage` 与管理列表的响应时间随 `usage_logs` 行数线性膨胀（百万行时单次列表请求秒级 + 把整表序列化进内存吃光连接池）；无 retention 的写表随时间单调逼近磁盘上限，到顶即写失败、网关全挂；首次启用 retention 对积压大表的单条无界 DELETE 会锁表数十秒到分钟、期间所有 usage 写入被阻塞。**容量天花板 = `usage_logs` 行数**，随时间单调逼近，是确定性而非概率性的劣化。本批把「表无限增长」与「读/抓取路径随表膨胀」两条容量曲线**都压平**。
> 前置依赖：rank13(slo_evaluator 单副本评估)与 rank8(分批删除单副本)依赖 **batch16 的 leader-gate** 已就位（否则多副本各跑一遍评估/同时删除互锁）；rank19(一线黄金信号告警)依赖**本批 rank4** 的进程内真 counter 先落地（告警表达式引用真实单调指标）；rank21 索引与 rank7 的 `account_quota_snapshot` 可清理字段属 ent schema 改动，须 `go generate ./ent/...` + 同步 store/mock。
> 关联：本批是 NFR program 第四批，与 batch16(并发安全：worker leader-gate + 竞态收口)/batch17(部署就绪：Redis 护栏 + readiness + HA + standalone)/batch18(热路径去全表扫：网关请求主链)同属一条治理线。本批 rank22(availability rollup worker)**承接 batch15 rank67** 并与 **batch18 rank2**（请求路径快照剥离）协同；rank4/rank3/rank13 的 DB 聚合手法与 batch18 同款。复用的现成原语：`migrator.go:67` 的 `pg_advisory_lock` 范式（batch16 已据此抽出 leader-gate）、`platform/redis` 客户端、`usage.CleanupLogs` 的按 ID 批删模式（团队已知正确做法，retention 没用上）、`dependencyPinger` 接口、`AccountAvailabilityRollup` 汇总表 schema（已存在但无 worker 填充）。

---

<role_and_context>
你是 SRapi 仓库的资深后端 / SRE 工程师。SRapi 是一个对标 sub2api 的 AI 网关 / 计费平台。
技术栈与布局：后端 Go + ent ORM，OpenAPI-first（先改 `packages/openapi/openapi.yaml` 再生成；schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`，14 个周期 worker 在 `apps/api/internal/workers/`，应用装配在 `apps/api/internal/app/`，配置在 `apps/api/internal/config/`，基础设施在 `apps/api/internal/platform/{db,redis,otel,ratelimit}`，共享纯函数在 `apps/api/internal/pkg/`）。持久化双实现：`apps/api/internal/persistence/entstore`(Postgres) + `redisstore`。前端 Next.js + TypeScript（apps/web）。部署在 `deploy/`（docker-compose + nginx + prometheus + tempo + alertmanager）。
本地开发：`make dev-up`；登录 admin@srapi.local / Admin1234。所有改动须通过 `cd apps/api && go build ./... && go vet ./...`。
架构红线（由 `apps/api/internal/architecture/architecture_test.go` + `apps/api/internal/codequality/code_quality_test.go` + gofmt 强制）：跨模块只允许 import 目标模块的 `contract` 层；`contract` 禁止 import Ent / 生成的 OpenAPI DTO / HTTP server 包；worker 只能 import 模块 contract/service；Ent/Redis store 只能 import 模块 contract；共享纯函数（如进程内 counter 注册表、分批删除游标）放 `internal/pkg`；单文件 ≤2200 行（`maxRuntimeFileLines`，runtime_* 文件）、单生产函数 ≤210 行（`maxProductionFuncLines`）；gofmt 必须全过；Go 版本 1.26.3。
本批维度强调点（容量 / 存储）：
- **读 / 抓取路径**：凡热路径或周期抓取仍走「整表 `List()` 进内存再 Go 过滤」的，必须改 **DB 端聚合（SUM/COUNT/COUNT FILTER）或带 WHERE+LIMIT+keyset 分页**——验收口径是「百万行 `usage_logs` 下仍 O(窗口/常数)响应」。
- **写表增长**：凡 append-only 高频写表必须有 retention 上界；删除必须**分批**（LIMIT + 游标 + 批间 sleep），不得单条无界 DELETE 锁表。
- **可观测正确性**：`/metrics` counter 必须**进程内单调**（promauto Counter/Histogram 在热路径递增，`/metrics` 直接 gather），retention 删行后不得倒退，多副本聚合交给 Prometheus 侧按 job sum / per-pod 解决。
- 本批以**纯性能 / 容量重构为主**：除新增 availability rollup worker(rank22)、新增 retention 接入(rank7)、新增告警规则(rank19)、新增索引(rank21) 外，对外行为（API 响应内容、计费金额、usage 聚合数值、SSE 流帧）必须**逐位 / 逐字节不变**。
</role_and_context>

<objective>
目标（可衡量最终态）：
1. `/v1/usage`(SummarizeAPIKey)、管理用量 / 账单 / 审计列表、`/metrics` 抓取、slo_evaluator 评估——**全部不再整表 `List()` 进内存**，改为 DB 端聚合或带谓词 + 强制分页的查询；在百万行 `usage_logs` 下仍 O(窗口 / 常数)响应。
2. `/metrics` 改为进程内真 `prometheus.Counter/Histogram`（promauto，在网关 / 调度路径递增），`/metrics` 直接 gather 已注册指标（O(1)）；账号探针指标读 `health_rollups` 汇总而非 N+1；**retention 删行后 counter 不倒退、`rate()` 单调可用**。
3. 全部无 retention 的高频写表（`scheduler_request_snapshots` / `account_quota_snapshots` / `domain_events_outbox+inbox` / `ops_system_logs` / `monitor_run_results`）接入 retention；`billing_ledger` 按月分区 + 冷归档策略（财务不可删，须先与运维 / 财务确认保留期，见决策注记）。
4. retention `Cleanup` 改**分批循环删除**（LIMIT 1000~5000 + id/created_at 游标 + 批间 sleep，复用 `usage.CleanupLogs` 批删模式），大表清理不再长事务锁表。
5. 新增 availability rollup worker 把 health snapshot 压日桶（承接 batch15 rank67），使从未被查看的账号也被物化；`usage_logs` 增加 `(provider_id,created_at)` 复合索引；补一线 SLO 黄金信号告警；流式 meter 增量解析 / `RawBody` 免全复制（资源充裕时）。
6. `go build ./... && go vet ./...` 退出码 0、两个守门测试 + gofmt + `go test ./...` 全绿。

动机（这些 NFR 风险在规模下的确定性后果）：
- `/metrics` 用 `NewConstMetric` 即时重算 + retention 删行 = **counter 倒退**，`rate(srapi_*_total[5m])` 出现负斜率被 Prometheus 当 counter reset 处理，监控彻底失真；多副本下每副本各报一份全表聚合，`sum` 重复计数、`rate()` 语义错乱——这是 production_readiness_verdict 列出的多副本硬阻塞 #2 的容量侧。
- 每个管理列表 / `/v1/usage` 把整张 `usage_logs` 序列化进单个 JSON 响应 = O(表行数) 的内存与延迟炸弹；`usage_logs` 是增长最快的表，行数随时间单调逼近，到百万行时单请求秒级、连接池被记账尾工作 + 列表读长期占据、p99 飙升、`/metrics` 抓取超时。
- 无 retention 的写表（每决策一行带 JSON blob 的 `scheduler_request_snapshots`、每 30min×账号 + 每成功请求再写的 `account_quota_snapshots`、发布后从不删的 outbox/inbox）**磁盘必然耗尽**——这不是 if 而是 when。
- retention 单条无界 DELETE 对积压大表 = 长事务 + 大量行锁 + WAL 暴涨 + autovacuum 风暴，**清理动作本身**会把网关 usage 写入阻塞数十秒到分钟。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元，每项带真实文件锚点）：

**A. 读 / 抓取路径去全表扫（DB 聚合 + 强制分页，保行为）**
1. **rank3（critical）`/v1/usage` 与管理用量 / 账单 / 审计列表去全表扫**：
   - `apps/api/internal/modules/usage/service/service.go:174`(`SummarizeAPIKey`) 改 `WHERE api_key_id + created_at` 的 DB 端 `SUM/COUNT` 聚合（复用 `SummarizeUserWindow` 的谓词写法但改 DB 聚合，不再 `store.List(ctx)` 全表后 `filterLogs:316` 内存过滤）。
   - `:137`(`ListFiltered`)/`Aggregate`/`Export` 把过滤 + 聚合下推 SQL + **强制分页**（LIMIT/keyset）。
   - `apps/api/internal/persistence/entstore/billing/store.go:59`(`List` 全表 `Query().Order(ByID()).All`) 与 `apps/api/internal/persistence/entstore/audit/store.go:47`(同) 改带 `WHERE+LIMIT+keyset` 分页。
   - raw admin-control 端点（`runtime_admin_control_handlers.go:32/66/89` 把整张表序列化进单个 JSON）强制分页上界。
   - 废弃读路径用 `Store.List()` 全表（保留必要的内部全量调用点须显式标注且非热路径）。
2. **rank4（high）`/metrics` 改进程内真 counter + 读汇总**：
   - `apps/api/internal/httpserver/runtime_metrics.go:18`(`handleMetrics` 每抓取新建 collector) + `:274`(`Collect` 全表) + `:286`(`usage.List`)/`:317`(`scheduler.ListDecisions`)/`:424-432`(per-account `LatestHealthSnapshotByAccount` N+1)/`:912`(`NewConstMetric` 即时重算)——改为**进程内真 `prometheus.Counter/Histogram`（promauto）**，在网关 / 调度路径递增，`/metrics` 直接 gather 已注册指标（O(1)）；账号探针指标由 health worker 写 `health_rollups` 聚合表，抓取读汇总而非 N+1。多副本聚合按 job 维度 sum / per-pod 在 Prometheus 侧解决（不在应用内合并）。
3. **rank13（high）slo_evaluator DB 端 COUNT FILTER 聚合**：
   - `apps/api/internal/workers/slo_evaluator/worker.go`（每 60s）调 `EvaluateSLOAlerts→ListUsageLogsSince`（`modules/operations/service/service.go`、`entstore operations/store.go:257-262` 仅 `CreatedAtGTE` 无 LIMIT，把窗口内可达全表的 usage_logs 全量进内存逐条循环）——SLO 可用率分子 / 分母改 `SQL COUNT(*) FILTER` 在 DB 聚合只回传计数，评估窗口按 SLO 维度下推 `WHERE`；`ListSLOs`（管理页）同路径。配合 batch16 leader-gate 保证单副本评估。

**B. 写表增长封堵（retention 覆盖 + 分批删除）**
4. **rank7（high）全表 retention 覆盖**：retention worker 现仅清 5 张表（`apps/api/internal/workers/retention/worker.go` 的 `policyFromConfig`：usage / scheduler_decisions / scheduler_feedbacks / audit / account_health_snapshots）。接入：
   - `scheduler_request_snapshots`（同 scheduler_decisions 天数）；
   - `account_quota_snapshots`（`ent/schema/accountquotasnapshot.go:23` 的 `snapshot_at` 非 TimeMixin，需补可清理字段 / 改按 `snapshot_at` 谓词）；
   - `domain_events_outbox`/`inbox`（按 `published_at`/`processed_at` 删或归档）；
   - `ops_system_logs`（后台 worker 复用已有 `CleanupSystemLogs`）；
   - `monitor_run_results`（按 `created_at`）；
   - `billing_ledger` 按月分区 + 冷归档（**财务不可直接删**，保留期 / 是否需冷归档须先确认——见决策注记，本批默认只落「分区 + 归档骨架」不真删）。
   每新接入表的 retention 配置走 `internal/config`（天数可配），并补 store 测试。
5. **rank8（med）retention 分批循环删除**：`apps/api/internal/persistence/entstore/operations/store.go:36-84` 的 `Cleanup` 对每表单条 `Delete().Where(CreatedAtLT).Exec` 无 LIMIT——改分批循环（每批 LIMIT 1000~5000、按 id/created_at 游标、批间 sleep），复用 `apps/api/internal/persistence/entstore/usage/store.go` 的 `CleanupLogs`（按 MaxDelete 批删）模式；配合 batch16 leader-gate 保证单副本删。
6. **rank22（med）availability rollup worker + 快照去抖 + quota snapshot retention**：
   - 新增 availability rollup worker 把 health snapshot 周期压成日桶（承接 batch15 rank67），使 `ent/schema/accountavailabilityrollup.go` 的汇总表权威化——从未被 admin 打开 availability 视图的账号也被物化（现仅 HTTP handler 惰性算，见 batch18 references 的 `runtime_admin_availability_handlers.go`）。
   - 请求路径快照去抖 / 采样（`runtime_gateway_usage.go:674/678/692` 每成功请求各写 health+quota+逐 signal）——与 **batch18 rank2** 协同剥离（本批不重复做剥离，只确保 rollup worker 是物化权威 + quota snapshot 纳入 rank7 retention）。
   - `account_quota_snapshot` 纳入 retention（与 rank7 同处落地）。

**C. 索引 / 告警 / 流式收尾**
7. **rank21（med）`usage_logs` 复合索引**：`apps/api/ent/schema/usagelog.go` 的 `Indexes()` 无 `(provider_id,created_at)` 复合，`SummarizeUserWindow` 用 `ProviderIDEQ` 过滤却走 `user_id+created_at` 后回表——加 `(provider_id,created_at)` 复合索引。**排在 rank3/rank10 之后**（先把全表 List 改成带谓词 SQL，否则索引白加）。
8. **rank19（low）一线 SLO 黄金信号告警**：`deploy/prometheus-srapi-alerts.yaml`（仅两条规则均基于 `srapi_ops_alert_events` 二阶派生信号）补一线告警：基于 `srapi_gateway_request_duration_seconds` 的 P99 burn-rate、错误率、`up{job="srapi-api"}==0`、（rank4 真 counter 落地后）连接池 / Redis 健康。**依赖本批 rank4 的真 counter 先就位**。保留 ops_alert_events 作业务层补充。
9. **rank20（low）流式 meter 增量解析 + RawBody 免全复制**：`runtime_gateway_streaming.go`（`maxStreamMeterBytes=16MB` 整段 meter `bytes.Buffer`）改边读边增量解析（只保留解析所需尾部 / 计数）；`runtime_http_helpers.go` 的 same-protocol 直透避免 `RawBody` 全复制。**资源充裕时再做，优先级最低**。

明确不做（out-of-scope —— 由其它 NFR / 功能批涵盖，非永久搁置）：
- 不做 worker leader-gate 通用机制本体（**batch16** 已落地；本批 rank13/rank8 直接复用其 gate）。
- 不做网关请求**主链**（每请求）的全表扫剥离与异步记账（**batch18** rank2/rank5/rank6/rank10/rank11/rank12 覆盖；本批只处理可观测 / 管理 / 周期评估读路径，与主链 DB 聚合手法同款但范围不重叠）。
- 不做 Redis 连接护栏 / readiness / 资源 limits / standalone 镜像（**batch17** 覆盖）。
- 不实现 `billing_ledger` 的真实删除（财务监管约束，本批只落分区 + 冷归档骨架；保留期待财务 / 运维拍板）。
- 不改 ChargeUsage / 限流 / 调度租约的正确性语义（见 constraints）。

> **决策注记（受 open_decisions 影响的默认假设）**：本批受三条 open_decision 影响，按以下默认假设推进，验收时在 progress.txt 标注待确认项：
> - **各高频写表保留期**（open_decisions「各高频写表的合规 / 业务保留期」）：retention 天数全部经 `internal/config` **可配**，默认值待运维 / 财务确认前用保守占位（`scheduler_request_snapshots`=14d、`account_quota_snapshots`=30d、`domain_events_outbox/inbox` 已发布 / 已处理=7d、`ops_system_logs`=30d、`monitor_run_results`=14d）；`billing_ledger`（财务监管）**默认不删**，只落按月分区 + 冷归档骨架，真删 / 归档窗口待财务拍板。
> - **可观测延迟容忍度**（open_decisions「可观测延迟容忍度」）：默认假设可接受**分钟级**新鲜度——故 `/metrics` 进程内 counter + health_rollups 汇总、availability rollup 日桶均为后台 / 增量更新；若产品要求强一致实时，缓存方案需配 pub/sub 失效（本批不做，标注为待确认）。
> - **多副本目标**（open_decisions「是否要多副本 / 水平扩展」）：默认目标是支持多副本，故 rank13/rank8 必须在 batch16 leader-gate 下单副本评估 / 删除；rank4 的 counter 多副本聚合交 Prometheus 侧（per-pod + job sum），不在应用内合并。
</scope>

<constraints>
途中**绝不破坏已验证的多副本安全正确性**：
- 计费扣费 `ChargeUsage` 已是 **Serializable + `charged_at IS NULL` 条件 claim 多副本安全**（`apps/api/internal/persistence/entstore/billing/store.go:133/220`）；限流 / 并发槽 / 调度租约已 Redis 原子 Lua；到期 / 状态流转条件更新幂等；outbox 消费侧 `(event_id,consumer)` 唯一索引去重。**本批只去掉效率退化与读路径退化，不得改其正确性语义**——retention 接入 / 分批删除不得触碰 `billing_ledger` 的写入与扣费 claim 逻辑；rollup / 聚合只读，不参与扣费判定。
- 金额一律 **string + big.Rat 8 位定点**，绝不改回 float64；DB 端 `SUM(billable_cost::numeric)` 聚合结果回传后仍以 string/big.Rat 承载，保留 0 价回退防漏收语义；聚合数值必须与原「逐行 big.Rat 求和」**逐位一致**（用等价性测试证明）。
- **性能 / 容量优化必须行为等价**：优化前后定价金额、usage 聚合数值（SummarizeAPIKey/Window 的 token/cost 标量）、调度决策、SSE 流帧**逐位 / 逐字节一致**；counter 改造后同一组请求产出的指标值与原 NewConstMetric 路径一致（用等价性 / 快照测试证明）。本批的「DB 聚合替代 Go 内存求和」「分批 DELETE 替代单条 DELETE」「进程内 counter 替代即时重算」都属**纯机械重构、零行为变更**——这是验收硬口径。
- 不破坏 OpenAPI 对外响应：`/v1/usage`、管理列表的**响应字段与数值**不变；若 rank3 给列表加强制分页参数（page/limit/cursor），属向后兼容的新增可选入参，须先说明（不删 / 不改既有字段语义）。
- ent schema 改动（rank21 复合索引、rank7 的 `account_quota_snapshot` 可清理字段）须 `go generate ./ent/...` 后同步 `store/mock`（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑）；新增索引属向前兼容迁移；不删枚举值。
- 不得修改与本任务无关的 `*_test.go`；新增能力（rollup worker / 新 retention 接入 / 索引）配新测试；迁移后原测试若只需调 import 路径不得删断言。
- 新增依赖须先说明理由（本批原则上无新外部依赖；promauto 已在 `prometheus/client_golang` 内）。
</constraints>

<success_criteria>
完成需同时满足（每条给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴尾部）。
2. **读路径去全表扫可证（rank3，核心）**：`grep -rn "\.List(ctx)" apps/api/internal/modules/usage/service apps/api/internal/persistence/entstore/billing apps/api/internal/persistence/entstore/audit` 证明 `SummarizeAPIKey`/`ListFiltered`/`Aggregate`/`Export`/`billing.List`/`audit.List` 热路径不再整表 List；贴出改后片段证明已是 DB 端 `SUM/COUNT` 聚合或带 `WHERE+LIMIT+keyset` 分页；贴一条「百万行下仍 O(窗口)」的说明（基准或调用计数）。raw admin-control 端点贴分页上界片段。
3. **/metrics 真 counter 可证（rank4）**：贴出 promauto `Counter/Histogram` 在网关 / 调度路径递增的 diff + `handleMetrics` 改为直接 gather 已注册指标（不再每抓取新建全表 collector）的片段；贴测试 / 说明证明 **counter 在 retention 删行后不倒退**（单调）、账号探针指标读 `health_rollups` 汇总而非 N+1（`runtime_metrics.go:424-432` 的 per-account 循环已消除）；Prometheus counter 不倒退可由「连续两次抓取 + 中间删行后值不下降」的测试佐证。
4. **slo_evaluator DB 聚合可证（rank13）**：贴出 `ListUsageLogsSince` 全量进内存改为 `COUNT(*) FILTER` DB 聚合的 diff（`operations/store.go:257-262` 不再无 LIMIT 全量 All）；贴等价性测试证明可用率分子 / 分母与原逐行评估逐位一致。
5. **retention 覆盖 + 分批删除可证（rank7+rank8）**：贴 `policyFromConfig` / `totalDeleted` 扩展后片段证明 `scheduler_request_snapshots`/`account_quota_snapshots`/`outbox`/`inbox`/`ops_system_logs`/`monitor_run_results` 已接入；贴 `operations/store.go` 的 `Cleanup` 改分批循环（LIMIT + 游标 + 批间 sleep）的 diff；贴 store 测试证明「插入 N 行 + 分批删除 + 每批 ≤LIMIT + 跨批游标推进」；`billing_ledger` 贴分区 + 冷归档骨架 + 决策注记（不真删）。
6. **availability rollup worker 可证（rank22）**：贴新 worker 代码 + 测试证明**从未被查看的账号也被物化进 `account_availability_rollup`**（断言 rollup 表有该账号行）；贴 quota snapshot 已纳入 rank7 retention 的证据。
7. **索引可证（rank21）**：贴 `usagelog.go` 的 `Indexes()` 新增 `(provider_id,created_at)` + `go generate ./ent/...` 后生成产物片段 + 迁移文件。
8. **告警可证（rank19）**：贴 `prometheus-srapi-alerts.yaml` 新增的 P99 burn-rate / 错误率 / `up==0` / 连接池 / Redis 健康规则（引用 rank4 落地的真 counter 名）；用 `promtool check rules` 或等价语法校验说明（不打真实集群）。
9. **流式优化可证（rank20，若做）**：贴增量解析 / RawBody 免全复制 diff + 等价性测试证明 usage 解析结果与整段 meter 一致；若资源 / turns 不足而跳过，在 progress.txt 标注为本批刻意延后的 low 项。
10. **等价性可证（核心口径）**：贴对比 / 快照测试证明 rank3/rank4/rank13 的聚合数值（token/cost/可用率）、counter 值、SSE 流帧逐位 / 逐字节一致——**「DB 聚合 / 进程内 counter / 分批删除」=纯机械重构零行为变更**。
11. `go test ./...` 全绿（尤其 `./internal/modules/usage/... ./internal/persistence/entstore/operations/... ./internal/persistence/entstore/usage/... ./internal/workers/... ./internal/architecture/... ./internal/codequality/...`，贴尾部）；`gofmt -l` 空；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 **40 turns** 后停止并汇总剩余阻塞项（已完成 rank、读 / 抓取路径去全表扫覆盖度、retention 接入表清单、分批删除是否就位、rollup worker 状态、counter 单调性、rank19/20/21 收尾状态、剩余阻塞项及原因）。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试（或等价性证明）并单独 commit；message 末尾加 Co-Authored-By 行：
1. **阶段一（读 / 抓取路径去全表扫，与 batch18 同款 DB 聚合手法）**：
   - rank3：`SummarizeAPIKey` → `ListFiltered`/`Aggregate`/`Export` → `billing.List`/`audit.List` → raw control 端点强制分页。每子项独立 commit + 等价性测试（聚合数值逐位一致）。
   - rank4：先在 `internal/pkg`（或 httpserver 内合规位置）建进程内 counter 注册表 → 在网关 / 调度路径接入 promauto 递增 → `handleMetrics` 改直接 gather → 账号探针指标改读 `health_rollups`。一个或多个 commit + counter 单调性测试。
   - rank13：slo_evaluator `ListUsageLogsSince` → `COUNT(*) FILTER` DB 聚合。一个 commit + 等价性测试。
   - 注：若 `runtime_metrics.go`(990 行) / `usage/service.go` 等文件增行逼近红线，**先按 batch15 rank59 拆文件**（core 仅余 ~44 行的教训：任何增行前先腾余量）。
2. **阶段二（写表增长封堵）**：
   - rank8 先行：`Cleanup` 改分批循环（机制先就位，避免阶段二接新表后首次全删锁表）+ store 测试。
   - rank7：逐表接入 retention（schema 改动如 `account_quota_snapshot` 可清理字段先 `go generate ./ent/...` + 同步 store/mock）→ 配置项 → store 测试。`billing_ledger` 分区 + 归档骨架单独 commit。
   - rank22：availability rollup worker（镜像现有 retention worker 模式，在 leader-gate 下）+ quota snapshot 纳入 retention + worker 测试。
3. **阶段三（索引 / 告警 / 流式收尾）**：
   - rank21：`usagelog.go` 加复合索引 → `go generate ./ent/...` → 迁移。
   - rank19：告警规则（**必须在 rank4 真 counter 落地后**，表达式引用真实指标名）+ `promtool check rules` 校验。
   - rank20（可选，资源 / turns 充裕时）：流式增量解析 + RawBody 免全复制 + 等价性测试。
每阶段更新 progress.txt + `git commit`；schema / 机制（counter 注册表、分批删除游标）先行；增行前先拆文件。
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch19`）若干语义化 commit + 具体产物：
- 进程内 counter 注册表（promauto Counter/Histogram，落 `internal/pkg` 或 httpserver 合规位置）+ 网关 / 调度路径递增点 + `handleMetrics` 直接 gather；
- DB 端聚合查询（`SummarizeAPIKey` 的 SUM/COUNT、`ListFiltered`/`Aggregate`/`Export` 下推 + 分页、`billing.List`/`audit.List` keyset 分页、slo_evaluator 的 COUNT FILTER）；
- retention 接入（6+ 张写表配置 + store 实现）+ 分批循环删除 Cleanup + `billing_ledger` 分区 / 归档骨架；
- availability rollup worker（含 worker 注册、leader-gate 接入、retention 接入）；
- `usage_logs` 的 `(provider_id,created_at)` 复合索引 + `go generate` 产物 + 迁移；
- `prometheus-srapi-alerts.yaml` 一线黄金信号告警规则；
- 新测试：等价性 / 快照测试（聚合数值、counter 值、SSE 帧逐位一致）+ 并发 / 单调性测试（counter 删行不倒退、分批删除游标推进、rollup 物化未查看账号）+ retention store 测试；
- （可选）流式增量解析 / RawBody 免全复制；
- progress.txt 收尾（列每个 rank 的最终处置、证据位置、retention 天数默认值与待确认项、`billing_ledger` 归档决策、未做的 low 项及原因）。
</artifact>

<guardrails>
绝不：删 / 改既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；**破坏计费多副本安全语义**（retention / 分区 / 聚合不得触碰 `ChargeUsage` 的 Serializable + 条件 claim、不得删 `billing_ledger`）；**为性能 / 容量牺牲计费 / 调度 / 评估正确性还假装等价**（DB 聚合数值 / counter 值必须有等价性测试证明逐位一致，不得「看起来对」就过）；**伪造未实测的性能 / 容量改善**（「百万行下 O(窗口)」「counter 不倒退」「分批不锁表」必须有基准 / 调用计数 / 测试佐证，不得口头声称）；保留任何「声称分页但仍全表 List」的假分页。
先问我：破坏 OpenAPI 对外响应结构（如改既有响应字段语义 / 删字段）；删现有顶层路由；任何不可逆数据迁移——**尤其 retention 删除策略（首次对积压大表启用前确认保留期）、`billing_ledger` 分区 / 归档策略、删 ent 枚举值**；在解锁多副本前确认 batch16 leader-gate 已就位（rank13/rank8 依赖它）；向真实上游 / 真实 Prometheus 集群发请求（告警用 `promtool check rules` 本地校验，不打真集群）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 证据在哪 / 哪些表已接入 retention 及天数 / counter 是否单调可证 / 下一步 / `billing_ledger` 与各保留期的待确认项）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，`grep -rn "\.List(ctx)"` 看读路径去全表扫剩余覆盖度，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 40 turns 仍未达成，停下汇总：已完成阶段与 rank、读 / 抓取路径去全表扫覆盖度（哪些仍全表 List）、retention 接入表清单与天数、分批删除是否就位、availability rollup worker 状态、counter 是否进程内单调（retention 删行不倒退）、rank21 索引 / rank19 告警 / rank20 流式收尾状态、两个守门测试红 / 绿、剩余阻塞项及原因（含 open_decisions 待确认项）。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；行数为本批审计 / 抽查时实测，改后会变）：
- **rank3 读路径全表扫**：`apps/api/internal/modules/usage/service/service.go:174`(`SummarizeAPIKey`)/`:137`(`ListFiltered`)/`:316`(`filterLogs` 内存过滤)；`apps/api/internal/persistence/entstore/billing/store.go:59`(`List` 全表)；`apps/api/internal/persistence/entstore/audit/store.go:47`(`List` 全表)；`apps/api/internal/httpserver/runtime_admin_control_handlers.go:32/66/89`(raw 端点整表序列化)。
- **rank4 /metrics 全表重算**：`apps/api/internal/httpserver/runtime_metrics.go:18`(`handleMetrics` 每抓取新建 collector，抽查实测 `:17` 起)/`:274`(`Collect`)/`:286`(`usage.List`)/`:317`(`scheduler.ListDecisions`)/`:424`(`accounts.List`)+`:432`(`LatestHealthSnapshotByAccount` per-account N+1)/`:912`(`NewConstMetric` 即时重算——**抽查修正**：synthesis 记为 `:547`，实测真实位置在 `:912`，per-account 循环在 `:424-432`)；`deploy/prometheus.yml` scrape_interval=15s。
- **rank13 slo_evaluator**：`apps/api/internal/workers/slo_evaluator/worker.go`（60s 触发，实测 193 行）；`apps/api/internal/modules/operations/service/service.go:165/127`(`ListUsageLogsSince`)；`apps/api/internal/persistence/entstore/operations/store.go:257-262`(仅 `CreatedAtGTE` 无 LIMIT)；`alertrules.go:573`(lookback)。
- **rank7 retention 覆盖**：`apps/api/internal/workers/retention/worker.go`（实测 184 行，`policyFromConfig`/`totalDeleted` 仅 5 项——**抽查确认**只清 usage/scheduler_decisions/scheduler_feedbacks/audit/account_health_snapshots）；`apps/api/internal/persistence/entstore/operations/store.go:36-84`(`Cleanup` 仅 5 表)；`apps/api/ent/schema/accountquotasnapshot.go:23`(`snapshot_at` 非 TimeMixin)；outbox/inbox/scheduler_request_snapshot/billing_ledger 零 Delete 路径。
- **rank8 分批删除**：`apps/api/internal/persistence/entstore/operations/store.go:36-41`(**抽查确认**单条 `Delete().Where(CreatedAtLT(...))` 无 LIMIT)；复用 `apps/api/internal/persistence/entstore/usage/store.go` 的 `CleanupLogs`（按 MaxDelete 批删）；`retention/worker.go` 的 `RunOnce` 单次全删。
- **rank22 availability rollup**：`apps/api/internal/workers/health_probe/worker.go`、`quota_refresh/worker.go`；`apps/api/internal/httpserver/runtime_gateway_usage.go:674/678/692`(每请求写快照)；`apps/api/ent/schema/accountavailabilityrollup.go`(汇总表无 worker 写入)。
- **rank21 索引**：`apps/api/ent/schema/usagelog.go` 的 `Indexes()`（无 `provider_id` 复合）；`apps/api/internal/persistence/entstore/usage/store.go` 的 `SummarizeUserWindow`（带 `ProviderIDEQ`）。
- **rank19 告警**：`deploy/prometheus-srapi-alerts.yaml`（实测 35 行，仅 `SRapiCriticalOpsAlertsFiring`/`Warning` 两条基于 `srapi_ops_alert_events`）；`runtime_metrics.go` 已暴露 gateway/scheduler/provider 指标但无对应告警。
- **rank20 流式**：`apps/api/internal/httpserver/runtime_gateway_streaming.go:111/:131-137`(`maxStreamMeterBytes=16MB`)；`runtime_http_helpers.go:444-449`(raw body 复制)。
- **复用的现成原语（仅参照，不抄）**：`apps/api/internal/platform/db/migrator.go:67`(**抽查确认** `SELECT pg_advisory_lock($1)` 范式——batch16 leader-gate 已据此抽出，rank13/rank8 单副本经此 gate)；`apps/api/internal/platform/redis`(连接客户端)；`apps/api/internal/persistence/entstore/usage/store.go` 的 `CleanupLogs`(按 ID 批删模式，rank8 复用)；`dependencyPinger` 接口(batch17 readiness 用)；`AccountAvailabilityRollup` schema(rank22 物化目标)；`health_rollups` 汇总(rank4 账号探针指标数据源)。
- **sub2api 对照点（仅对照差距，不抄代码）**：`SchedulerCache`(scheduler_cache.go 按 bucket 缓存快照)、`ops_repo_preagg.go`(daily_rollup 预聚合——rank4/rank13 的汇总思路对照)、`outbox`/`deferred_service`(异步落库——本批 rank7 outbox retention 与之相关但不重复 batch18 异步剥离)、`Dockerfile.goreleaser`(单二进制——batch17 standalone 对照)。
</references>
