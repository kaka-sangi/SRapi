# Goal：调度策略 CRUD + 探测真实性 + 孤儿端点接线（第十批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查发现 scheduler 域是一整片摆设——7 个调度策略硬编码 Go seed，`scheduler_strategies` 表恒空，运营无法创建策略或调任何 weight/version/description；`scope_type`/`scope_id` 作用域字段建模但 `ListActiveStrategies` 写死 global+nil-scope 永不加载；`premium_quality` 的权重键名为 `priority` 实际乘 qualityScore（抽象泄漏，加权重编辑器后会误导）；scheduled-test 的 `cron_expression` 是可编辑表单字段但全模块无 cron 解析（`NextRunAt` 自认 "cron is treated as an interval hint for now"）；探测无模型选择器导致计划可静默全空转（probed=0 的 "干净" run）；`POST /scheduler/simulate`(what-if) 与 `GET /scheduler/overview` 后端+SDK 完整且已注册路由但 apps/web 零调用。本批把 scheduler 域从「能看不能调」升级为「运营能真正调度权重、探测名实相符、孤儿端点有入口」。
> 前置依赖：无强外部依赖；与 batch8（分销资金链）、batch9（RBAC/风控/内容安全）独立，可并行或后行。注意 `runtime_gateway_core.go` 仅余 44 行触线、`scheduler/service/service.go`=1885 行——本批若需在这两个文件增行须谨慎，大幅增行的拆分由 batch15（架构清理）兜底，本批以最小侵入为先（新逻辑尽量落新文件/新方法）。
> 关联：rank22（权重键改名）建议在 rank3（权重编辑器）落地前完成，避免把误导键名固化进 UI；rank3 策略 CRUD 是本批前提（rank18/21/22 的前端与加载逻辑复用其表单与契约）。与 batch7（认证矩阵 CI 守门思路）同源——本批同样要求「建模即可达，否则下架」，不留摆设中间态。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个对标 sub2api 的 AI 网关/计费平台。
技术栈：
- 后端 Go + ent ORM（apps/api），OpenAPI-first：先改 `packages/openapi/openapi.yaml` 再生成。schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`，后台任务在 `apps/api/internal/workers/`。
- 前端 Next.js + TS（apps/web），路由在 `app/`，组件在 `components/`，导航在 `components/layout/nav-items.ts`，admin API 客户端在 `src/lib/admin-api.ts`（封装生成的 SDK）。
本地开发：`make dev-up` + 前端 `npm run dev`；不要设 `NEXT_PUBLIC_SRAPI_BASE_URL`（CSP 会拦跨域）；登录 admin@srapi.local / Admin1234。
所有改动须过：后端 `cd apps/api && go build ./... && go vet ./...`；前端 `npm run lint` + tsc（`npm run typecheck` 或 build）。
架构红线（由 `apps/api/internal/architecture/architecture_test.go` 与 `apps/api/internal/codequality/code_quality_test.go` 守门，必须全绿）：模块生产码只能 import 别模块的 `contract` 层（白名单）；contract 层禁 import Ent/生成 DTO/HTTP server；worker 只能 import 模块 contract/service；Ent/Redis store 只能 import 模块 contract；单文件 ≤2200 行（runtime_* 文件，`runtime_http.go` 兼容层 ≤120）；单函数 ≤210 行；gofmt 必须通过。
凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
本批主题强调点：scheduler 域的「建模 vs 加载/消费」脱节是核心病灶——本批每项都要么真接通到 Schedule 决策路径/前端入口（有可观察证据），要么连同 plumbing 一起诚实下架。
</role_and_context>

<objective>
目标（可衡量最终态）：scheduler 域不再有摆设——(a) 运营能通过 admin API + 权重编辑器 UI 创建/更新/激活/弃用调度策略，调整后的 weight 被 `Schedule` 真实采用（非回落 seed map），有生命周期（config_hash 重算 + active/deprecated）；(b) `scope_type`/`scope_id` 要么被 `ListActiveStrategies` 加载并按作用域应用，要么连同 plumbing 一起删除；(c) `premium_quality` 权重键端到端改名 `quality`，与代码语义一致；(d) scheduled-test 的 `cron_expression` 要么被真实解析驱动 due-time，要么连字段+误导注释一起删除；(e) 探测计划可显式指定 probe model，缺模型时 run 归类为 warning 而非静默 ok；(f) `simulate`/`overview` 两个孤儿端点在前端有可达入口（simulate tab + overview header strip），或被删除。
动机：摆设 = 业务能力假象。运营以为能「调度策略权重」「按作用域定制」「cron 定时探测」，实际全是固定枚举/惰性字段/静默空转——这让 SRapi 在功能宣传上比 sub2api 宽，但调度可运营性是假的。scheduler 决策直接影响请求落到哪个上游账号（容量/成本/质量），一个调不动的调度器是产品核心能力的硬伤。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个子目标，每项带真实文件锚点）：

1. **rank3 scheduler 策略 CRUD（本批前提，必做）**：
   - 后端：先在 `packages/openapi/openapi.yaml` 增 `POST /api/v1/admin/scheduler/strategies`（创建）、`PUT /api/v1/admin/scheduler/strategies/{id}`（更新 weight/description/version）、`DELETE`（或 deprecate）端点，再 `go generate`。
   - service 增 `CreateStrategy`/`UpdateStrategy`/`ActivateStrategy`/`DeprecateStrategy`：写 `scheduler_strategies` 表，重算 `config_hash`（基于 scores_json/config_json/weights 的规范化哈希），管理 `created_by`/`activated_at`/`deprecated_at` 生命周期（active/deprecated 状态机），同名同 version 唯一冲突要返回明确错误（schema 唯一索引在 `apps/api/ent/schema/schedulerstrategy.go:35`）。
   - store：补 entstore 的 Create/Update + memory store 对应方法（注意 memory store 的 `DeletedAt` 过滤与 Store-mock codegen 坑）；让 `ListActiveStrategies`（`apps/api/internal/persistence/entstore/scheduler/store.go` 的 `ListActiveStrategies`）能返回 DB 中的 active 策略，使 `Schedule` 优先用 DB 策略、表空时才回落 `seededStrategyDescriptors()`（`apps/api/internal/modules/scheduler/service/service.go:59`）。
   - 前端：`apps/web/src/app/admin/ops/strategy/page.tsx` 把写死的 `STRATEGIES` 数组（`:24`）替换为从 API 拉取的策略列表 + 权重编辑器（编辑 weights map、description、version、activate/deprecate），在 `src/lib/admin-api.ts`（现仅 `replaySchedulerStrategy`，`:181`/`:538`）补对应客户端方法。
   - 接通的消费点：编辑后的 weight 必须流入 `Schedule` 的打分（`apps/api/internal/modules/scheduler/service/service.go:1071` 的 `final := ... + quality*weights[...]`），用测试证明改 weight 后 winner 变化。

2. **rank22 premium_quality 'priority' 权重键改名 quality（建议在 rank3 权重编辑器前做，必做）**：
   - 把权重键端到端从 `priority` 改名为 `quality`：seed（`apps/api/internal/modules/scheduler/service/service.go:59` 起的 seededStrategyDescriptors 里 premium_quality 的权重）、打分点 `service.go:1071`（`quality*weights["priority"]` → `quality*weights["quality"]`）、loader 归一化（`apps/api/internal/modules/scheduler/service/strategy_loader.go:134` 的 `normalizeStrategyWeightKey`，`:150-151` 现把 priority/quality/quality_weight 折叠成 `"priority"` 返回 → 应折叠成 `"quality"`）、scoreBreakdown JSON 键。
   - `priority` 这个键名释放出来，保留语义给未来「显式软优先级偏置」（区别于 `Account.Priority` 硬层 `filterByPriorityTier`）；本批不实现软优先级，仅注释说明键名归属。
   - 接通的消费点：改名后 premium_quality 策略仍正确乘 qualityScore，权重编辑器里显示的键名 = 实际乘的因子。

3. **rank21 scope_type/scope_id 作用域加载（二选一并落定，必做）**：
   - schema 现支持 per-apikey/group/user 作用域行（`apps/api/ent/schema/schedulerstrategy.go:22-23` 的 `scope_type`/`scope_id`，`:35-36` 唯一索引），但 `ListActiveStrategies`（entstore store.go）写死 `ScopeTypeEQ("global")` + `ScopeIDIsNil()`，registry 仅按 name 索引；store_test 故意造 api_key 行并断言被排除。
   - **方案 (a) 加载全作用域**：`ListActiveStrategies` 去掉 global-only 谓词加载全作用域；scheduler 解析请求 scope（apikey/group/user，从 req 派生）后按 `(scope_type, scope_id)` 命中，未命中回落 global；registry 按 `(scope_type, scope_id, name)` 键控；改写 store_test 断言 api_key 行被加载而非排除。
   - **方案 (b) 删字段**：若产品刻意只承认 global 策略，删 `scope_type`/`scope_id` 字段（`go generate` + 同步 store/mock，**删 schema 字段属不可逆迁移，删前先问我迁移策略**），改写 store_test，并在策略文档注明「调度策略仅 global」。
   - 默认倾向 (a)（能力更完整、对标 sub2api 的 per-scope 路由），但若 (a) 工作量超预算可落 (b)；无论哪个都不留「字段在但永不加载」的摆设。

4. **rank19 scheduled-test cron_expression 真解析（二选一并落定，必做）**：
   - `cron_expression` 是可编辑表单字段（`apps/web/src/lib/admin-scheduled-test-form.ts:15`/`:27`/`:59`），schema 注释承诺可解析时覆盖 interval，但 `NextRunAt`（`apps/api/internal/modules/scheduled_tests/service/service.go:177`，`:144` 注释自认 "cron is treated as an interval hint for now"）完全忽略 `CronExpression` 仅按 last_run_at+interval。
   - **方案 (a) 真解析**：引入 `github.com/robfig/cron/v3`（新增依赖须在 commit message/progress 说明理由），`NextRunAt` 当 `CronExpression` 非空时用 cron 解析驱动 due-time，无效表达式回退 interval 并记日志；表单加 cron 校验提示。
   - **方案 (b) 删字段**：移除 `cron_expression` 字段 + 误导 schema 注释 + 表单字段，保留 interval-only 语义（删 schema 字段属不可逆迁移，**删前先问我**）。
   - 默认倾向 (a)。

5. **rank20 探测模型选择器（必做）**：
   - `probeModel`（`apps/api/internal/workers/scheduled_test/prober.go:77`）只从 3 个账号/provider metadata key 解析（`prober.go:26` 的 `probeModelKeys`），全无则返回空串、runner 计为 Skipped（`apps/api/internal/workers/scheduled_test/runner.go:75-76` model=='' → Skipped），计划照常跑记 probed=0 的「干净」run 却什么都没测。
   - 给 plan 增 `probe_model` 字段（覆盖 metadata key 优先级最高）：OpenAPI + ent schema + 表单（`admin-scheduled-test-form.ts:11-17` 现无 model 字段）；`probeModel` 先读 plan.probe_model 再回落 metadata keys。
   - 把 probed==0 的 run 从「ok」归类为「warning」（runner outcome 或 RecordRun 状态），并在前端 run 列表标注；scope 内账号缺探测模型时给可见告警。
   - 接通的消费点：plan 设 probe_model 后 runner 不再全 Skipped（测试证明 probed>0）；probed=0 的 run 显示 warning。

6. **rank18 simulate/overview 孤儿端点接线（二选一并落定，必做）**：
   - `POST /scheduler/simulate`（`apps/api/internal/modules/scheduler/service/simulator.go:21` 完整实现，含 winner change/per-factor score delta/rollout preview 的 StrategySimulationResult，`server.go:529` 已注册）与 `GET /scheduler/overview`（`server.go:526`）后端+SDK 完整但 apps/web 零调用，ops 页只接了 replay。
   - **方案 (a) 接线（默认）**：`ops/strategy` 页加 single-request **simulate tab**（粘贴/编排请求 profile → 展示 current-vs-shadow winner + per-factor score delta，复用 SDK 类型）；把 overview 做成 scheduler-decisions header strip（选择率/拒因直方图/top 账号）。在 `admin-api.ts` 补 `simulateScheduler`/`schedulerOverview` 客户端方法。
   - **方案 (b) 删端点**：若产品不要 what-if/overview，删两端点 + service + SDK + OpenAPI 定义。
   - 默认 (a)；接通后这两个端点不再是孤儿（前端有可达入口，截图证明）。

明确不做（out-of-scope，均由其它批次涵盖，非永久搁置）：
- **加权随机选路（rank40）/EffectiveLoadFactor 并发缩放（rank41）/账号调度状态字段索引化（rank42）**：由 batch15（架构与技术债清理 + 能力补全）涵盖。
- **scheduler `service.go`=1885 行的拆分（rank59 部分）/metadata 强制转换 helper 跨包消重（rank61）**：由 batch15 涵盖。本批在 scheduler 文件内增行须最小化；若新增 CRUD 逻辑会显著增行，落到新文件（如 `strategy_crud.go`）而非堆进 `service.go`。
- **账号域配额/TLS 摆设、channel-monitor 调度 worker（rank6/7/11/13/38）**：由 batch11 涵盖。
- 本批刻意不碰：scheduler 的评分算法本体（health/quota/latency/sticky/cache/cost/fairness 公式不改，只改 quality 键名与权重来源）；故障转移/冷却/调度反馈闭环（batch1-5 已落地，仅作回归保护）。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64（本批基本不碰计费，若 cost score 涉及金额沿用现路径）。
- 不得破坏 batch1-7 已落地的能力：会话粘度、Account.Priority 硬层 `filterByPriorityTier`、live-concurrency 评分、故障转移+自动冷却+调度反馈、分组费率倍率、配额真实性与认证矩阵；这些的现有测试必须仍绿。
- 凭证 AES-GCM 加密不得改明文。
- OpenAPI 兼容：新增端点/字段属增量，允许；**破坏现有 `simulate`/`overview`/`replay`/`strategies` 响应结构须先问我**。
- 不得修改与本任务无关的 `*_test.go`；scheduler/scheduled_tests 相关测试若因行为变更需调整，只改受影响断言、不删无关断言；新增能力配新测试。
- ent schema 改后须 `go generate ./ent/...` 并同步 store/mock（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑）；新增 `probe_model` 等字段属可加列。
- **删枚举值/删 schema 字段/不可逆迁移先问我**：rank21 方案(b) 删 `scope_type`/`scope_id`、rank19 方案(b) 删 `cron_expression` 都属此类，落 (b) 前先确认迁移策略。
- 新增依赖（如 `robfig/cron`）须在 commit message 与 progress.txt 说明理由。
- scheduler `Schedule` 是网关热路径：DB 策略加载须有进程内缓存或低频查询（避免每请求全表 List 退化）；缓存失效在策略写入时触发。
</constraints>

<success_criteria>
完成需同时满足（每条给出 agent 自己能贴出的可观察证据，评估器不自跑命令）：
1. `cd apps/api && go build ./... && go vet ./...` 退出码 0（贴出尾部输出）。
2. 前端 `npm run lint` 通过 + tsc/build 无类型错误（贴出尾部）。
3. **策略 CRUD 真接通（rank3，核心）**：新增测试证明——admin 经 `CreateStrategy` 写入策略并设定一组与 seed 不同的 weights → `ListActiveStrategies` 返回该策略（非 nil/非 seed） → `Schedule` 用 DB 策略的 weights 打分，winner 与 seed weights 下不同（贴出测试名 + `go test` 输出）；并贴 grep 证明 service 有 `CreateStrategy`/`UpdateStrategy`/`ActivateStrategy`（或等价）方法定义。activate/deprecate 生命周期：deprecated 策略不被 `ListActiveStrategies` 返回（测试断言）。
4. **权重键改名可证（rank22）**：grep 证明 `service.go:1071` 附近不再有 `weights["priority"]` 乘 quality（改为 `weights["quality"]`），`normalizeStrategyWeightKey` 折叠目标为 `"quality"`；测试证明 premium_quality 策略仍正确乘 qualityScore。
5. **scope 处置可证（rank21）**：
   - 若落 (a)：测试证明造一条 group/apikey 作用域的 active 策略后 `ListActiveStrategies` 加载它，且 scheduler 对匹配 scope 的请求应用该策略、不匹配回落 global（贴测试 + 改写后的 store_test 输出）。
   - 若落 (b)：grep 证明 schema 已无 `scope_type`/`scope_id`，store_test 已改写，文档注明仅 global（贴片段）。
6. **cron 处置可证（rank19）**：
   - 若落 (a)：测试证明 `NextRunAt` 对一个具体 cron 表达式（如 `0 */6 * * *`）解析出正确 due-time（与 interval 计算不同），无效表达式回退 interval（贴测试输出）。
   - 若落 (b)：grep 证明 `cron_expression` 字段 + 误导注释 + 表单字段已删（贴片段）。
7. **探测模型可证（rank20）**：测试证明 plan 设 `probe_model` 后 `probeModel` 返回该值、runner 不再计 Skipped（probed>0）；缺模型的 plan run 被归为 warning（断言 outcome 状态）；前端 run 列表 warning 标注截图（chrome-devtools）。
8. **孤儿端点处置可证（rank18）**：
   - 若落 (a)：chrome-devtools 截图证明 `ops/strategy` 页 simulate tab 能跑 what-if 并展示 winner/score-delta，overview header strip 渲染选择率/拒因；`admin-api.ts` grep 证明有 `simulateScheduler`/`schedulerOverview` 方法。
   - 若落 (b)：grep 证明端点 + service + SDK + OpenAPI 定义已删。
9. `cd apps/api && go test ./...` 全绿（贴尾部）；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 35 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit，schema/OpenAPI 改动先行：
1. **阶段一（权重键正名，先于 UI）**：rank22 把 `priority` 权重键端到端改名 `quality`（seed/打分点/loader/scoreBreakdown JSON）+ 回归测试。先做以免误导键名固化进后续编辑器。commit。
2. **阶段二（策略 CRUD 后端）**：rank3——改 `openapi.yaml` 增 strategies POST/PUT/DELETE → `go generate` → service `CreateStrategy`/`UpdateStrategy`/`ActivateStrategy`/`DeprecateStrategy`（config_hash + 生命周期，落新文件如 `strategy_crud.go`）→ entstore + memory store CRUD + `ListActiveStrategies` 返回 DB active 策略（含进程内缓存与失效）→ 单测证明 DB weights 流入 Schedule。commit。
3. **阶段三（scope 加载二选一）**：rank21——(a) `ListActiveStrategies` 加载全作用域 + scheduler scope 解析 + registry 按 (scope,name) 键控 + 改写 store_test；或 (b) 删字段（先问迁移）+ 文档。commit。
4. **阶段四（cron 解析二选一）**：rank19——(a) 引入 robfig/cron 让 `NextRunAt` 真解析；或 (b) 删字段+注释（先问）。commit。
5. **阶段五（探测模型）**：rank20——OpenAPI + ent schema 加 `probe_model`（`go generate`）→ `probeModel` 优先读 plan 字段 → probed==0 归 warning → 表单 + run 列表 UI。commit。
6. **阶段六（前端策略 CRUD + 孤儿端点）**：rank3 前端权重编辑器（替换 `STRATEGIES` 写死数组）+ rank18 simulate tab + overview header strip（或删端点）+ `admin-api.ts` 客户端方法。commit。
每阶段一个 commit，message 末尾加：
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch10`）若干语义化 commit + 具体产物：
- 后端：scheduler 策略 CRUD 端点 + service 方法（新文件 `strategy_crud.go`）+ entstore/memory store CRUD + `ListActiveStrategies` 加载 DB 策略 + 进程内缓存；权重键改名；scope 加载或删除；cron 解析或删除；`probe_model` 字段 + 探测优先级 + warning 归类。
- 前端：`ops/strategy` 权重编辑器 + simulate tab + overview header strip；scheduled-test 表单 probe_model 字段；run 列表 warning 标注；`admin-api.ts` 新方法。
- 测试：每阶段新测试全 passing（CRUD→Schedule、改名回归、scope 加载、cron 解析、探测模型、warning 归类）。
- progress.txt 收尾：列每阶段做了什么 / 每个二选一项的最终处置决定 / 证据位置 / out-of-scope（batch11/15）未做项。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证；**伪造未实现能力**——摆设的两种合法处置只有「真接通」（流入 Schedule/前端有可达入口，有证据）或「诚实下架」（连 plumbing 一起删 + 文档化），绝不假装接通（如 cron 字段留着但仍不解析、scope 字段留着但仍不加载、simulate tab 调不通端点却留个空 tab）。
先问我：破坏现有 `simulate`/`overview`/`replay`/`strategies` 端点的 OpenAPI 响应结构；删现有顶层路由文件；删 `scope_type`/`scope_id` / `cron_expression` 等 schema 字段的存量迁移（不可逆）；任何不可逆数据迁移；向真实上游发起需真凭证的调用（simulate/probe 用 mock，不打真实上游）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 每个二选一项的处置决定 / 证据在哪 / 下一步）+ git commit。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：已完成阶段、每个二选一项的当前处置状态、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准）：
- 策略 seed（DB 空时回落）：`apps/api/internal/modules/scheduler/service/service.go:51`（`seededStrategyDescriptorMap`）/`:59`（`seededStrategyDescriptors`，硬编码 7 策略）。
- 打分点（权重消费）：`apps/api/internal/modules/scheduler/service/service.go:1071`（`final := ... + quality*weights["priority"] - ...`，rank22 改名目标）。
- 权重归一化：`apps/api/internal/modules/scheduler/service/strategy_loader.go:83`/`:134`（`normalizeStrategyWeightKey`）/`:150-151`（priority/quality/quality_weight 折叠成 `"priority"` → 改折叠成 `"quality"`）。
- DB 加载（scope 写死）：`apps/api/internal/persistence/entstore/scheduler/store.go` 的 `ListActiveStrategies`（写死 `StatusEQ("active")` + `ScopeTypeEQ("global")` + `ScopeIDIsNil()`，按 name+id 排序）。
- schema（scope/唯一索引）：`apps/api/ent/schema/schedulerstrategy.go:22`（`scope_type` Default "global"）/`:23`（`scope_id` Optional Nillable）/`:35`（`name,version,scope_type,scope_id` 唯一索引）/`:36`（`status,scope_type,scope_id` 索引）。
- 路由注册（无 CRUD）：`apps/api/internal/httpserver/server.go:526`（GET overview）/`:528`（GET strategies）/`:529`（POST simulate）/`:530`（POST replay）——无 POST/PUT/DELETE strategies。
- simulate 实现：`apps/api/internal/modules/scheduler/service/simulator.go:21`（StrategySimulationResult，winner change/score delta/rollout preview）。
- scheduled-test cron（永不解析）：`apps/api/internal/modules/scheduled_tests/service/service.go:144`（注释 "cron is treated as an interval hint for now"）/`:168`（`NextRunAt(plan)` 调用）/`:177`（`NextRunAt` 实现，忽略 CronExpression）。
- 探测模型解析：`apps/api/internal/workers/scheduled_test/prober.go:26`（`probeModelKeys` 三 key）/`:77`（`probeModel`）；`apps/api/internal/workers/scheduled_test/runner.go:75-76`（`model == ""` → `Skipped++`）/`:65`（另一 Skipped 路径）。
- 前端策略页（写死数组）：`apps/web/src/app/admin/ops/strategy/page.tsx:24`（`const STRATEGIES`）/`:109`/`:125`（map 渲染）——replay-only，无权重编辑器/simulate/overview。
- 前端 admin SDK：`apps/web/src/lib/admin-api.ts:181`/`:538`（仅 `replaySchedulerStrategy`）——需补 strategies CRUD + simulate + overview 方法。
- scheduled-test 表单（无 model 字段）：`apps/web/src/lib/admin-scheduled-test-form.ts:15`/`:27`/`:40`/`:59`（`cronExpression` 可编辑）——需加 `probeModel` 字段。
- OpenAPI 源：`packages/openapi/openapi.yaml`（先改再 `go generate`）。
- 守门测试：`apps/api/internal/architecture/architecture_test.go`（contract-only import + 单文件 ≤2200）/`apps/api/internal/codequality/code_quality_test.go`（单函数 ≤210）——本批改动须保持全绿，注意 `scheduler/service/service.go`=1885 行，新逻辑落新文件。

sub2api 仅作能力对照不抄代码。本批相关 sub2api 参照点：策略的 DB 持久化 + 版本化生命周期（active/deprecated）+ per-scope（global/group/user/key）路由解析；what-if 模拟与调度概览面板的运营可视化；加权随机选路（本批不做，留 batch15）。
</references>
