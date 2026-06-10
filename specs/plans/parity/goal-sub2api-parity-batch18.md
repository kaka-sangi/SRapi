# Goal：热路径去全表扫 —— 网关请求主链不再每请求扫 usage_logs / accounts（第十八批 · NFR 性能批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：16-agent NFR 对抗性核查后，结论是「正确性地基扎实，但热路径与多副本就绪度有确定性炸弹」。本批针对其中**最危险的 critical 性能炸弹**：每个成功网关请求都在请求 goroutine 内**同步全表扫 `usage_logs`**——`recordGatewayUsage→recordGatewayAccountSnapshots` 调 `rt.usage.List(ctx)`（`Query().Order(ByID()).All`，无 WHERE/无 LIMIT）把整张表拉进内存（`runtime_gateway_usage.go:662`），再 `usageLogsForAccount` 过滤、`updateAccountRuntimeQuotaMetadata` 再遍历两次。这是 **O(请求数 × 表行数)** 的正反馈炸弹：表越大每请求越慢、请求越多表越大。核查给出确定性劣化曲线——usage_logs 在 ~10 万行内、连接池 25、中低 QPS（几十）可正常跑；到 ~50–100 万行时每请求的全表 SELECT+反序列化进入数百 ms~秒级、连接池被记账尾工作长期占据、p99 飙升；到几百万行（日均百万请求约数天即达）时单请求秒级、连接池瞬时打满、网关雪崩。同一链上还串联：调度候选每请求全表 `accounts.List`（仅 `DeletedAtIsNil`，无 provider/status/limit 谓词）+ 逐候选 2DB+2Redis 的 N+1（`runtime_gateway_core.go:1163`），且 failover 每 attempt 重跑（`failover.go` per-attempt 全量重调度）；`apiKeyByID` 走 `ListByUser` 全量映射（每 key 一次 group 子查询 N+1）再线性扫，单请求调 2 次（`runtime_gateway_core.go:591`）；`EstimatePrice` 每请求 2–3 次全量 `ListPricingRules`；`SummarizeUserWindow` 逐行 `big.Rat` 求和而非 SQL SUM。**结论：单副本容量天花板 = usage_logs 行数，且随时间单调逼近——这是确定性而非概率性劣化。本批拆除这条「每请求 O(表行数)」的正反馈炸弹。**
> 前置：建议 batch15（架构清理）已落地——本批要改的 `runtime_gateway_core.go` 实测已 **2198 行**（距 `maxRuntimeFileLines=2200` 仅余 2 行），**任何增行改动必触线**，故本批增行前**必须先按 batch15 rank59 拆该文件到 <1800**；batch15 rank66 已规划 `EstimatePrice` 谓词查询 + usage 快照谓词层，本批**承接并合并**该层、再做更深的「异步剥离」改造（rank66 只到「谓词查询」，本批把整个 `recordGatewayAccountSnapshots`/记账尾工作移出请求 goroutine）。
> 关联：本批是 NFR program 四批（batch16 并发安全 / batch17 部署就绪 / **batch18 热路径去全表扫** / batch19 容量治理）的性能主攻批。与 batch16 关联——记账异步化的有界队列**可复用 batch16 引入的 worker 框架**；与 batch19 关联——读/抓取路径的全表扫（`/v1/usage`、管理列表、`/metrics`、slo_evaluator）由 batch19 用**同款 DB 聚合手法**处理，本批只攻**请求主链**；rank2 的「请求路径快照剥离」与 batch19 rank22（availability rollup worker / 请求路径快照去抖，承接 batch15 rank67）协同。复用的现成原语：DB 聚合可对照 `usage/store.go:96` 的 `SummarizeUserWindow` 谓词写法（本批把它从「Select+Go 求和」升级为 SQL SUM）、批量删/批处理可对照 `usage/store.go:137` `CleanupLogs` 的批模式、异步队列可对照 batch16 的 worker leader-gate 框架。

---

<role_and_context>
你是 SRapi 仓库的资深后端 / SRE 工程师。SRapi 是一个对标 sub2api 的 AI 网关 / 计费平台，本批是 NFR 治理 program 的**性能批**，主攻**网关请求热路径的全表扫与同步记账**。
技术栈与布局（OpenAPI-first）：后端 Go + ent ORM，主服务在 `apps/api`——HTTP 层 `apps/api/internal/httpserver/`（网关主链 `runtime_gateway_*.go`），业务模块 `apps/api/internal/modules/<域>/{contract,service,store}`，14 个周期 worker 在 `apps/api/internal/workers/`，app 启动装配在 `apps/api/internal/app/`，配置在 `apps/api/internal/config/`，基础设施在 `apps/api/internal/platform/{db,redis,otel,ratelimit}`，共享纯函数在 `apps/api/internal/pkg/`。持久化双实现：Postgres 经 `apps/api/internal/persistence/entstore/<域>/store.go`，Redis 经 `redisstore`。前端 Next.js + TypeScript（`apps/web`，本批基本不动前端）。部署 `deploy/`（docker-compose + nginx + prometheus/tempo/alertmanager）。本地开发：`make dev-up` + `npm run dev`；登录 admin@srapi.local / Admin1234。
架构红线（由两个守门测试 + gofmt 强制）：
- `apps/api/internal/architecture/architecture_test.go`：跨模块只允许 import 目标模块的 `contract` 层；`contract` 禁止 import Ent / 生成的 OpenAPI DTO / HTTP server 包；worker 只能 import 模块 contract/service；Ent/Redis store 只能 import 模块 contract；**单文件硬上限 `maxRuntimeFileLines=2200`**（runtime_* 文件）。
- `apps/api/internal/codequality/code_quality_test.go`：`maxProductionFuncLines=210`（单函数行数上限）。
- gofmt 必须全过；Go 1.26.3。
**本批维度是「性能（去全表扫 + 异步剥离 + 缓存 / DB 聚合）」**，强调点全部压在：(1) 网关请求 goroutine 内绝不再有「全表 List 后内存过滤」；(2) 每请求的 DB 往返数从 O(账号数 / key 数 / 表行数) 降到 O(1)~O(候选数)；(3) **优化前后行为逐位 / 逐字节等价**（定价金额、usage 聚合、调度决策、SSE 流帧）。本批除「异步剥离」是行为时序变更（须按 open_decision 确认最终一致可接受、用持久队列保正确性）外，其余皆为**纯机械等价重构**。凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
</role_and_context>

<objective>
目标（可衡量最终态）：网关请求主链（鉴权 → 准入 → 调度 → 转发 → 记账）**不再有任何「全表 List 后内存过滤」**，每请求的 DB 往返数从「随账号数 / key 数 / usage_logs 行数线性增长」降到「常数 ~ 候选数」级；`recordGatewayAccountSnapshots` 与记账尾工作（计费记账 / 反馈 / 快照 / 风控落库）移出请求 goroutine（交有界持久队列 / 周期 worker），请求路径只做必须项 + 一次 `usage_log` INSERT；定价 / apiKey / 窗口聚合改缓存或 DB 聚合。完成后：(a) `grep` 证明热路径无 `usage.List()` / `accounts.List()` 全表调用；(b) 调用计数 / 基准证明每请求 DB 往返从 O(N) 降 O(1)~O(候选数)；(c) **等价性 / 快照测试证明优化前后定价金额、usage 聚合、调度决策、SSE 流帧逐位 / 逐字节一致**；(d) 两个守门测试 + gofmt + `go vet` + `go test ./...` 全绿，`runtime_gateway_core.go` 拆分后 <1800 行留余量。
动机（这些 NFR 风险在规模下的确定性后果）：每成功请求全表扫 `usage_logs` 是 **O(请求 × 行数) 的炸弹**——到百万行级单请求秒级、连接池（`MaxOpenConns=25`）瞬时打满、网关雪崩、`/metrics` 抓取同步超时；这是核查给出的「容量天花板 = usage_logs 行数且单调逼近」的根因，不修则系统**注定**在表增长到阈值时倒下（不是会不会，是何时）。调度候选每请求全表 `accounts.List` + 逐候选 N+1 + failover 每 attempt 重跑，把单请求的 DB / Redis 往返放大到「账号数 × attempt」量级；`apiKeyByID` 的 `ListByUser` + N+1 group 子查询在企业用户几十上百把 key 时再放大。把这些剥离后，单副本容量天花板从「usage_logs 行数」抬升到「真实 QPS × 连接池」，系统才能在单副本撑到中等规模、并为 batch16/17 的多副本扩容奠定热路径基础。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元，每项带真实文件锚点 + 等价性证明）：

**A. 请求主链全表扫剥离（critical，本批核心）**
1. **rank2 `recordGatewayAccountSnapshots` 去全表扫 + 异步剥离**：现状 `recordGatewayUsage`（`runtime_gateway_usage.go:23`）在请求 goroutine 内同步调 `recordGatewayAccountSnapshots`（`:653`），其中 `rt.usage.List(ctx)`（`:662`）拉整张 `usage_logs` 表（`usage/store.go:75` 的 `Query().Order(ByID()).All`，无谓词），再 `usageLogsForAccount` 过滤、`updateAccountRuntimeQuotaMetadata` 遍历两次。双管齐下：
   - (1) **谓词查询层**（承接 batch15 rank66）：在 `usage/store.go` 新增 `ListByAccountWindow(account_id + 时间窗谓词)`（或复用 `SummarizeUserWindow` 式 SQL 聚合），只取该账号近窗口行，替代全表 `List`；
   - (2) **异步剥离**：把整个 `recordGatewayAccountSnapshots` 移出请求 goroutine——交给已有 `health_probe`/`quota_refresh` worker 周期重算，或异步有界队列（持久队列保正确性，见 open_decisions）。请求路径只写一行 `usage_log`。
2. **rank6 `recordGatewayUsage` 整体异步化 + 消二次全窗扫**：除全表扫外，`recordGatewayUsage`（`:23-104` 同步链）还顺序串联 `gatewayUsageCost`（二次 `PriceGatewayCost` + 条件 allowance 全窗扫）、`usage.Record` 写、`RecordFeedback` 写、`recordGatewayMaterializedCosts` 写、cooldown、`recordGatewayRiskFailure`、`recordGatewayAccountSnapshots`，全部在客户端等响应期间顺序执行、占用 25 连接池。把计费记账 / 反馈 / 快照 / 风控落库改异步（**有界 worker 队列 / outbox 持久队列保正确性**，非 fire-and-forget），消除 record 路径的**二次 `gatewayUserPeriodUsage` 全窗扫**（复用准入期已算好的值）。请求路径只做必须项 + 一次 `usage_log` INSERT。可对照 sub2api outbox/deferred_service/flusher 思路。

**B. 调度候选去全表 List + N+1（critical）**
3. **rank5 `gatewayCandidates` DB 谓词过滤 + 批量 IN + 缓存 + failover 复用候选**：现状 `gatewayCandidates`（`runtime_gateway_core.go:1163`）先 `accounts.List`（仅 `DeletedAtIsNil`，无 provider/status/limit 谓词），再 `for mapping{providers.FindByID}` 嵌套 `for account{apiKeyAllowsAccount(1DB) + health 快照(1DB) + quota 快照(1DB,取该账号全部) + AccountConcurrency(1Redis) + LastUsed(1Redis)}`；failover 对每次 attempt（默认上限 3）整套重跑（`runtime_gateway_failover.go` per-attempt 重调度）。改：
   - 按 provider / 分组从 DB 直接过滤账号（`WHERE provider_id IN AND status=active`）替代全表 List；
   - health/quota 最新快照用**单条 `IN(account_ids)` 批量查询**；
   - Concurrency/LastUsed 用 **Redis pipeline/MGET 批量取**；
   - 对 providers/models/groups/pricing 引入**带失效的进程内缓存**（TTL + 写时失效，放 `internal/pkg` 守红线）；
   - failover **复用首次构建的候选集合**（`result.Candidates`）而非每 attempt 全量重调度。
   可对照 sub2api `SchedulerCache`（按 bucket 缓存快照）思路。

**C. 可缓存 / 可聚合查询收敛（high）**
4. **rank11 `EstimatePrice` 谓词查询 / 进程内缓存**（承接 batch15 rank66）：`gatewayPricing` 每请求至少 2 次（准入 + 响应），流式再加 1。每次 `EstimatePrice`（`billing/service/pricing.go:140`）→ `ListPricingRules` 全量加载（`billing/store.go:378-398` 的 `Query().Order(ByID()).All` + `pricingRuleModelFamilies` IN 子查询 + `pricingIntervals` 子查询），再 `selectPricingRule` Go 线性筛。改：按 `(model_id,provider_id,at)` 直接 DB 查命中规则（带索引），或把定价规则集做进程内缓存（TTL + 写时失效），`selectPricingRule`/family fallback 在缓存上跑。
5. **rank10 `SummarizeUserWindow` 改 SQL SUM 标量聚合 + 单请求复用 + 平台配额三窗合并**：现状 `SummarizeUserWindow`（`usage/store.go:96`）用 `Select(total_tokens,billable_cost).All` 取窗口内所有行再 Go `big.Rat` 累加（非 SQL SUM）。热路径多次调用：准入 `gatewayUserPeriodUsage` 一次、响应 allowance 路径再一次（仅 allowance plan）、平台配额 daily/weekly/monthly 三次（仅配置了平台配额）。重用户当月 / 7 日窗可数万~数十万行。改：DB 端 `SUM(total_tokens),SUM(billable_cost::numeric)` 一条查询返回标量；同一请求内 period usage 算一次、admission→响应间复用；平台配额三窗合并为一条带 CASE 聚合或维护 Redis 滚动计数器。可对照 sub2api 预聚合表 `ops_repo_preagg`。
6. **rank12 `apiKeyByID` 改 `FindByID` 直查 + 透传 `authed.Key` 复用**：现状 `apiKeyByID(userID,keyID)`（`runtime_gateway_core.go:591`）实现为 `ListByUser(userID)`（`apikeys/store.go:192`，对该用户每把 key 调 `toAPIKey→groupIDs` 一次 APIKeyGroup 子查询，N+1）后 for 线性找 ID；单请求调 2 次（`prepareGatewayAdmission` cost limit + `checkGatewayRateLimit` 再 + `users.FindByID`），鉴权 `Authenticate` 的 `FindByPrefix` 另走一次 group 子查询。改：用**已存在但未用的 `apikeys.FindByID`（`store.go:179`）直查**目标 key；单请求内把鉴权已取得的 `authed.Key` 透传复用，避免 `prepareGatewayAdmission`/`checkGatewayRateLimit` 重复回查；`toAPIKey` 的 group 子查询按需取或批量 IN。

明确不做（out-of-scope，均由其它 NFR / 功能批涵盖，非永久搁置）：
- **不**改读 / 抓取路径的全表扫——`/v1/usage`（`SummarizeAPIKey`）、管理用量/账单/审计列表（`ListFiltered`/`Aggregate`/`Export`/`billing.List`/`audit.List`/raw admin-control）、`/metrics` 即时全表重算、`slo_evaluator` 每 60s 全表读，**全部归 batch19**（rank3/rank4/rank13，同款 DB 聚合手法，本批只攻请求主链）。
- **不**做 retention / 分批删除 / 复合索引 / 一线告警（batch19 rank7/8/21/19）。
- **不**做 worker leader-gate 与并发竞态收口（batch16 rank1/14/15/23）——本批的异步队列**复用** batch16 的 worker 框架，不重复实现。
- **不**做部署护栏 / Redis 池调优 / readiness（batch17）。
- **不**改计费扣费的多副本安全正确性语义（见 constraints）；**不**改任何对外 OpenAPI 契约 / 计费金额 / 调度对外决策结果。
- rank21 `usage_logs` 的 `(provider_id,created_at)` 复合索引归 batch19——核查明确「索引缺口次于全表 List，全表 List 根本绕过索引，故索引白加须排在去全表扫之后」。

> **决策注记（受 open_decisions 影响，写明默认假设）**：
> - rank6/rank2 的「记账 / 快照异步落库」前提是 open_decision「计费记账是否允许异步落库（短暂最终一致）」——**默认假设：接受秒级最终一致，但用持久队列（outbox / 有界 worker 队列）而非 fire-and-forget goroutine 保正确性**，使「失败可重试、不丢账」。若运维 / 财务要求严格实时一致，则改造仅做「去全表扫 + DB 聚合」部分、保留同步落库（异步剥离降级为可选），须在 progress.txt 标注。
> - rank5/rank11 的「进程内缓存（TTL）」前提是 open_decision「可观测延迟容忍度」——**默认假设：providers/models/groups/pricing 这类近静态配置可接受秒级 TTL 缓存 + 写时失效**；若要求强一致实时，则缓存须配 pub/sub 失效而非纯 TTL（影响选型，先问）。
> - 本批 critical 项的优先级取决于 open_decision「目标规模与峰值 QPS」——**默认假设：目标是支持中等规模（usage_logs 可达百万行级）和 / 或多副本**，故 batch18 视为上线硬阻塞；若明确长期单副本 + 中低 QPS + 短 retention 控住表体积，则本批可下调优先级（但 rank2 的全表扫仍是确定性劣化，建议至少做谓词查询层）。
</scope>

<constraints>
途中**绝不破坏已验证的多副本安全正确性与计费正确性**：
- **计费扣费已是多副本安全，本批不动其正确性语义**：`ChargeUsage` 在存储层用 Serializable 隔离 + `charged_at IS NULL` 在 select 与 UpdateMany 两端做条件 claim + 行数校验（`apps/api/internal/persistence/entstore/billing/store.go:133`/`:220`），即使多副本同时扣同一批日志也不会重复扣费；限流 / 并发槽 / 调度租约已是 Redis 原子 Lua。**本批只去掉效率退化与同步阻塞副作用重复，不得改其正确性语义**——异步化记账只改「何时落库」（用持久队列保不丢、可重试），不改「扣多少、按什么条件 claim」。
- **金额一律 string + `big.Rat` 8 位定点，绝不改回 float64**；保留 0 价回退防漏收语义。
- **性能优化必须行为等价**：拆文件 / 抽 helper / 谓词查询 / 缓存 / DB 聚合 / 异步剥离前后，**定价金额、usage 聚合结果、调度决策（候选顺序 / 选中账号）、SSE 流帧必须逐位 / 逐字节一致**——用等价性 / 快照测试证明（对同一输入，优化前后输出 byte-for-byte 相同）。`SummarizeUserWindow` 改 SQL SUM 后，`big.Rat` 8 位定点的求和结果必须与逐行 Go 累加**完全相同**（注意 numeric 精度 / 四舍五入边界）。
- **不破坏对外 OpenAPI 响应**：网关响应体、`/v1/usage` 响应结构、计费金额回显不变（本批不动 `/v1/usage` 读路径，归 batch19）。
- **ent schema 若改**（本批原则上不改 schema，新增 `ListByAccountWindow` 是 store 方法非 schema 变更；若 rank5 需新增账号过滤索引）：`go generate ./ent/...` 后同步 store/mock（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑）；删枚举值 / 不可逆迁移先问我。
- **抽共享缓存 / helper 必须遵守架构红线**：进程内缓存 / 共享纯函数放 `internal/pkg`（不引入「跨模块 import 非 contract」违规）；不得让 httpserver 直接 import 别模块的 service/store。
- **不得修改与本任务无关的 `*_test.go`**；拆文件后原测试应仍能覆盖（必要时只调 import 路径，不得删断言）；新增能力 / 异步路径配新测试。
- 不在测试里打真实上游；异步队列 / 缓存失效用本地 mock / 内存实现验证。
其他约束：本批基本不引入新依赖（异步队列复用 batch16 worker 框架 / 现有 outbox）；如确需新依赖须先说明理由。
</constraints>

<success_criteria>
完成需同时满足（每条都是 agent 自己能贴出的可观察证据，评估器不自己跑命令；逐条对应 items）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）。
2. **拆文件腾余量可证（增行前置）**：`wc -l apps/api/internal/httpserver/runtime_gateway_core.go` <1800（实测起点 2198，距红线仅 2 行，必须先拆）；`go test ./internal/architecture/... ./internal/codequality/...` 全绿（贴测试名 + 输出）。
3. **rank2 去全表扫 + 异步剥离可证（核心）**：`grep -rn "usage.List\|usage\.List(ctx)" apps/api/internal/httpserver/` 证明网关请求热路径**不再调** `usage.List()` 全表（贴 grep 结果——`recordGatewayAccountSnapshots` 已改 `ListByAccountWindow` 或已移出请求 goroutine）；贴出 `ListByAccountWindow` 新代码（带 `account_id` + 时间窗谓词 + LIMIT）；贴证据证明快照生成不再在请求 goroutine 内同步执行（异步队列 / 周期 worker，含失败可重试说明）。
4. **rank6 异步化 + 消二次全窗扫可证**：贴出 `recordGatewayUsage` 改后片段——记账 / 反馈 / 快照 / 风控落库走有界持久队列（非 fire-and-forget），请求路径只做必须项 + 一次 `usage_log` INSERT；`grep` / 说明证明 `gatewayUserPeriodUsage` 二次全窗扫已消除（复用准入期值）；贴持久队列保正确性（失败重试 / 不丢账）的说明 + 测试。
5. **rank5 调度候选去全表 List + N+1 可证**：贴出 `gatewayCandidates` 改后片段——账号按 `WHERE provider_id IN AND status=active` DB 谓词过滤（不再 `accounts.List` 全表）、health/quota 快照单条 `IN(account_ids)` 批量、Concurrency/LastUsed Redis pipeline/MGET、providers/models/groups 进程内缓存（TTL+写时失效）；贴 failover 复用首次候选集合而非每 attempt 重跑的 diff；**调用计数对比**证明每请求 DB/Redis 往返从 O(账号数×attempt) 降到 O(候选数)。
6. **rank11/10/12 收敛可证**：`EstimatePrice` 改谓词查询 / 进程内缓存（贴片段 + 调用计数从每请求 2-3 次全量 List 降到缓存命中 O(1)）；`SummarizeUserWindow` 改 SQL `SUM` 标量聚合（贴片段，证明不再 `Select.All` 逐行 `big.Rat` 累加）+ 单请求复用 + 平台配额三窗合并；`apiKeyByID` 改 `FindByID` 直查 + `authed.Key` 透传（贴片段 + grep 证明不再 `ListByUser` 线性扫）。
7. **等价性可证（纯机械重构口径 · 核心验收）**：贴出对比 / 快照测试证明**优化前后行为逐位 / 逐字节一致**——`EstimatePrice`（覆盖正常 / 缓存命中 / 0 价 / 多币种 / family fallback）金额逐位相同；`SummarizeUserWindow` SQL SUM 与逐行 Go 累加结果 `big.Rat` 8 位定点完全相同（含精度边界）；调度决策（候选顺序 / 选中账号）对同一输入相同；SSE 流帧逐字节相同。贴 `go test` 通过输出。**本批除「异步剥离」是经 open_decision 确认的时序变更外，其余皆零行为变更——等价性测试是验收硬口径。**
8. **容量曲线压平可证**：用调用计数 / 基准（benchmark 或 mock 计数器）证明：网关单请求的 `usage_logs` 读从「全表 O(行数)」变为「O(窗口)或不读（异步）」；调度候选构建 DB 往返从 O(账号数) 降 O(候选数)；定价 / apikey 从每请求多次全量 List 降到缓存 O(1)。可附「百万行 usage_logs 下请求路径不再触发全表 SELECT」的说明 / 测试。
9. `go test ./...` 全绿（贴尾部）；`gofmt -l` 空；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 **40 turns** 后停止并汇总剩余阻塞项（性能批给 40：拆文件 + 异步剥离 + 等价性测试工作量大）。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带等价性测试并单独 commit；message 末尾加 Co-Authored-By 行。**拆文件（增行前置）必须先行**：
0. **阶段零（增行前置 · 拆文件）**：`runtime_gateway_core.go` 实测 2198 行（距 2200 仅 2 行），任何本批增行改动必触线——先按 batch15 rank59 把它拆成同包多文件（如 `runtime_gateway_admission.go` / `runtime_gateway_candidates.go` / `runtime_gateway_pricing_request.go`，纯移动、不改签名 / 调用方）降到 <1800 留余量；跑 `go build ./...` + 现有网关测试确认零回归。一个 commit。
1. **阶段一（最热的请求路径全表扫，影响最大）**：rank2 先做 `usage.List()→ListByAccountWindow` 谓词查询层（承接 batch15 rank66）+ 把 `recordGatewayAccountSnapshots` 移出请求 goroutine（异步队列 / 周期 worker）。等价性测试：账号快照内容与原全表扫路径一致。一个 commit。
2. **阶段二（同链异步剥离，与 rank2 同文件）**：rank6 把记账 / 反馈 / 快照 / 风控落库整体异步化（持久队列保正确性）+ 消二次全窗扫。测试证明异步落库不丢账、失败可重试、请求路径只一次 `usage_log` INSERT。一个 commit。
3. **阶段三（调度候选 N+1，独立子系统）**：rank5 `gatewayCandidates` DB 谓词过滤 + 批量 IN + Redis pipeline + 进程内缓存（放 `internal/pkg`）+ failover 复用候选集合。等价性测试：调度决策（候选顺序 / 选中账号）对同一输入逐位一致。一个 commit。
4. **阶段四（可缓存 / 可聚合查询收敛）**：rank11 `EstimatePrice` 谓词 / 缓存（承接 batch15 rank66）→ rank10 `SummarizeUserWindow` SQL SUM + 单请求复用 + 三窗合并 → rank12 `apiKeyByID` `FindByID` 直查 + `authed.Key` 透传。每子项独立 commit + 等价性测试（定价金额 / 窗口聚合逐位一致）。
进程内缓存机制（rank5/rank11 共用）应先在 `internal/pkg` 落地（TTL + 写时失效原语），再被两处引用。每阶段一个 commit。
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch18`）若干语义化 commit + 具体产物：
- 拆分后的同包多文件（`runtime_gateway_core.go` <1800，diff 纯机械移动）；
- `usage/store.go` 新增 `ListByAccountWindow`（account_id + 时间窗谓词 + LIMIT）；`SummarizeUserWindow` 改 SQL `SUM` 标量聚合；
- `recordGatewayAccountSnapshots` / 记账尾工作的**异步剥离**（有界 worker 队列 / outbox 持久队列，复用 batch16 框架，失败可重试）；
- `gatewayCandidates` 的 DB 谓词过滤 + 批量 `IN` 快照查询 + Redis pipeline/MGET + failover 复用候选集合；
- `internal/pkg` 进程内缓存原语（TTL + 写时失效）+ providers/models/groups/pricing 缓存接入；
- `EstimatePrice` 谓词 / 缓存查询；`apiKeyByID` `FindByID` 直查 + `authed.Key` 透传；
- 新增测试：**等价性 / 快照测试**（定价金额 / usage 聚合 / 调度决策 / SSE 流帧逐位一致）+ 异步落库不丢账测试 + 调用计数 / 基准对比；
- progress.txt 收尾（列每个 rank 的最终处置、证据位置、异步落库的最终一致决策、缓存失效策略、拆文件后各文件行数与红线余量、未做的 out-of-scope 归属批次）。
</artifact>

<guardrails>
绝不：删除 / 改写既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float64；退回明文凭证；**破坏计费扣费的多副本安全正确性语义**（`ChargeUsage` 的 Serializable + `charged_at IS NULL` 条件 claim 不得改）；**为性能牺牲计费 / 调度正确性还假装等价**（异步化只改落库时序、用持久队列保不丢，绝不 fire-and-forget 丢账还说「等价」）；**伪造未实测的性能改善**（调用计数 / 基准必须真跑真贴，不得编造「降到 O(1)」而无证据）；在测试里打真实上游。
先问我：破坏现有 OpenAPI 响应结构；删除现有顶层路由文件；任何不可逆数据迁移；记账异步化是否被允许（open_decision「计费记账是否允许异步落库」——若运维 / 财务未确认接受秒级最终一致，则异步剥离降级为可选，先确认）；缓存改强一致需 pub/sub 失效（open_decision「可观测延迟容忍度」）；向真实上游发起需真凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 证据在哪 / 每请求 DB 往返数前后对比 / 等价性测试位置 / 异步落库决策 / 拆文件后行数与红线余量 / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态、`wc -l apps/api/internal/httpserver/runtime_gateway_core.go` 看当前余量，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 40 turns 仍未达成，停下汇总：已完成阶段、`runtime_gateway_core.go` 当前行数 vs 1800、两个守门测试红 / 绿、rank2/6（全表扫 + 异步剥离）状态、rank5（调度 N+1）状态、rank11/10/12（缓存 / 聚合 / 直查）状态、等价性测试覆盖情况、异步落库的最终一致决策、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；行数为本批审计 + 起草时实测，拆改后会变）：
- 请求主链全表扫（rank2/rank6）：apps/api/internal/httpserver/runtime_gateway_usage.go:23（recordGatewayUsage 同步链入口）/:653（recordGatewayAccountSnapshots）/:662（rt.usage.List 全表扫）；该文件起草时实测 920 行。
- usage store（rank2/rank10）：apps/api/internal/persistence/entstore/usage/store.go:75（List 无谓词 `Query().Order(ByID()).All`）/:85（ListByUser）/:96（SummarizeUserWindow，现 `Select(...).All` + Go `big.Rat` 求和，待改 SQL SUM）/:137（CleanupLogs，批删 MaxDelete 模式可对照）；需新增 ListByAccountWindow。
- 调度候选（rank5）：apps/api/internal/httpserver/runtime_gateway_core.go:1163（gatewayCandidates，全表 accounts.List + 逐候选 N+1）；apps/api/internal/httpserver/runtime_gateway_failover.go（per-attempt 重调度，审计锚点 :432-446）；entstore accounts/store.go（账号谓词查询落点）。
- apiKeyByID（rank12）：apps/api/internal/httpserver/runtime_gateway_core.go:591（apiKeyByID，现 ListByUser + 线性扫）；apps/api/internal/persistence/entstore/apikeys/store.go:179（FindByID，存在但未用，直查目标）/:192（ListByUser，N+1 group 子查询）/:211（List 全表）。
- 定价（rank11）：apps/api/internal/modules/billing/service/pricing.go:140（EstimatePrice / ListPricingRules 全量）；apps/api/internal/persistence/entstore/billing/store.go:378-398（全量 + 两子查询）；该 pricing.go 起草时实测 679 行。
- 守门测试（增行前置 / 拆文件）：apps/api/internal/architecture/architecture_test.go（maxRuntimeFileLines=2200）；apps/api/internal/codequality/code_quality_test.go（maxProductionFuncLines=210）；runtime_gateway_core.go 起草时实测 **2198 行（距红线 2 行，本批增行前必拆）**。
- 计费多副本安全（constraints 红线，勿改正确性）：apps/api/internal/persistence/entstore/billing/store.go:133（Serializable）/:220（charged_at IS NULL 条件 claim + 行数校验）。
- 共享缓存落点（rank5/rank11）：apps/api/internal/pkg/（进程内缓存 TTL + 写时失效原语须放此处以不违反架构红线；参照 batch2 的 internal/pkg/money、batch15 的 internal/pkg/metacoerce 先例）。

复用的现成原语（仅借鉴，不抄实现）：
- apps/api/internal/platform/db/migrator.go:67（`SELECT pg_advisory_lock($1)` 范式——batch16 leaderGate 已复用；本批异步队列若需单副本周期重算可经 batch16 的 leaderGate 接入）。
- apps/api/internal/platform/redis（Redis 客户端——rank5 的 Concurrency/LastUsed pipeline/MGET 批量取经此）。
- apps/api/internal/persistence/entstore/usage/store.go:137（CleanupLogs 的 MaxDelete 批处理模式——`ListByAccountWindow` 的 LIMIT/窗口写法可对照）。
- apps/api/internal/persistence/entstore/usage/store.go:96（SummarizeUserWindow 现有谓词写法——rank10 在其上升级为 SQL SUM）。
- batch16 的 worker 框架 / leaderGate（rank2/rank6 异步队列复用）；batch19 的 health_rollups 汇总（rank2 快照剥离可交周期 worker，与 batch19 rank22 协同）。

sub2api 仅作能力对照不抄代码：
- SchedulerCache（scheduler_cache.go）——按 bucket 缓存账号 / 快照，对照 rank5 的进程内缓存 + 批量快照思路。
- ops_repo_preagg.go（预聚合表）——对照 rank10 的窗口聚合 / 平台配额三窗合并思路。
- outbox / deferred_service / flusher——对照 rank6 的记账异步化「持久队列保正确性」思路。
- Dockerfile.goreleaser 单二进制——非本批，部署形态对照归 batch17。
</references>
