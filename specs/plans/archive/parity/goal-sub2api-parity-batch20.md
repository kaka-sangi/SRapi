# Goal：落地收口与真实性对账 —— 分批提交 24k 行在途成果 + 修正 batch18/19 虚报项 + 清除残余摆设 + 浏览器验证（第二十批 · 收口）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。本批不是新功能批，是 **batch8-19 的落地收口批**。
> 背景：2026-06-10 对抗性核查（3-agent + 人工复验）确认：batch8-19 的工作已全部存在于工作树并通过全量门禁（`go test ./...` / vet / build / architecture / codequality / migration-check / openapi 双端 codegen-check / web typecheck+lint 全绿，已复验），**但 ~246 文件 / ~24k 行全部未提交**、全部批次的浏览器验证被跳过，且 progress.txt 有 **3 项 batch19 声称与代码不符（虚报）**、batch14 摆设清零声称下仍有 **5-6 个 settings 开关存而不读**。
> 核查发现（全部带代码证据，已逐条人工复验关键项）：
> 1. **虚报①**：「/metrics 改 process-local 单调 counter」**未实现**——`runtime_metrics.go:286`（`usage.List` 全表）、`:317`（`ListDecisions` 全表）、`:360`（`ListLeases` 全表）仍每次抓取全表重算 + `NewConstMetric`；retention 删行后 counter 倒退、多副本 `rate()` 错乱依旧（NFR rank4 未完成）。
> 2. **虚报②**：「retention 分批删除」**未实现**——`entstore/operations/store.go:36`(`Cleanup`) 仍单条无界 DELETE，contract `RetentionCutoffs`/`CleanupResult` 无任何 limit/batch 参数（NFR rank8 未完成）；`usage_logs (provider_id,created_at)` 复合索引也未加（rank21，000040/000041 均非此内容）。
> 3. **部分完成**：调度候选已改 `ListActiveByProviderIDs`（非全表），但 `runtime_gateway_core.go:580` 的 `apiKeyAllowsAccount` 仍每候选触发 `ListGroupIDsByAccount`（:653-662）N+1（NFR rank5 残留）。
> 4. **残余摆设**（违反「真接通或诚实下架」铁律）：`admin_control/contract/contract.go` 定义但**零消费点**的 settings 开关：`invitation_rebate_enabled`（讽刺：batch8 刚建好 affiliate 闭环，总开关却不被 AccrueRebate 消费）、`request_shaper_enabled`、`balance_low_notify_enabled`、`subscription_expiry_notify_enabled`、`account_quota_notify_enabled`。另：RoleOperator 仅有 admin-surface 准入检查（`runtime_http_helpers.go:303`），无成套可达能力。
> 5. **部署面矛盾**：`deploy/k8s/api-deployment.yaml:9` 写 `replicas: 3`，但 /metrics 在多副本下计数语义仍错乱（见虚报①）且 README 护栏写明 replicas=1；compose 无 scale 护栏注释。Next standalone 切换后 `next.config` 的 `output:'standalone'` 与 runtime `SRAPI_API_PROXY_TARGET` rewrites 生效路径未经核实。
> 6. **工程债**：batch10 期间 Ent 生成码（`scheduled_test_plans.probe_model`）因当时工具链 1.26.2 无法重跑而**手工编写**——现在本地工具链已是 1.26.3，可以也必须重跑 `go generate` 对账。文档漂移：parity README 批次表 batch8/9/10 未标 ✅、`STATUS.md` 停在 2026-06-06、`ROADMAP.md` 自称 all complete 但仅覆盖到 WP-360。
> 7. **已核实为真、勿重做**：leader-gate 全 worker 覆盖（`app.go:223-302` newWorkerLeaderGuard + 19 处 optionalWorkerGuard）、网关快照走 outbox 异步（`runtime_gateway_usage.go:105` enqueue → `workers/outbox/gateway_snapshot.go`）、风控 enforce 真拦截（`runtime_gateway_risk_control.go:27-50`）、内容安全 Luhn+block（`content_safety/service.go:264-295`）、affiliate 全链含前端（`/affiliate` + `/admin/affiliates/rules` + outbox AccrueRebate 生产者）、调度策略 CRUD+scope 真加载、channel-monitor worker、/me/attributes+Required 强制、站点公开配置端点、RBAC permission-catalog 受约束选择器。
> 关联：本批承接 batch16-19（NFR）与 batch8-15（摆设清除）的收尾，完成后 parity program 才算真正可宣称「零摆设、零虚报、已落 main」。

---

<role_and_context>
你是 SRapi 仓库的资深后端 / SRE / 交付工程师。SRapi 是一个对标 sub2api 的 AI 网关 / 计费平台。
技术栈与布局：后端 Go + ent ORM，OpenAPI-first（先改 `packages/openapi/openapi.yaml` 再生成；schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`，周期 worker 在 `apps/api/internal/workers/`，应用装配在 `apps/api/internal/app/`）。持久化双实现：`entstore`(Postgres) + 内存 store（测试）。前端 Next.js + TypeScript（apps/web）。部署在 `deploy/`。
本地开发：`make dev-up`；登录 admin@srapi.local / Admin1234。浏览器验证用 `~/.claude/cdp-chrome.sh` + chrome-devtools MCP。
架构红线：跨模块只 import 目标模块 `contract`；单文件 ≤2200 行、函数 ≤210 行；gofmt；Go 1.26.3；由 `internal/architecture` + `internal/codequality` 测试强制。
**本批特殊性：工作树已有 ~24k 行未提交的 batch8-19 成果（已通过全量门禁）。本批的产出 = 修正 + 收口 + 一个干净的、按域分批的 commit 序列落到 main。在动任何新代码前先理解：这些在途改动是资产不是草稿，不得回退或重写已验证为真的部分。**
</role_and_context>

<objective>
目标（可衡量最终态）：
1. progress.txt / parity README 中**每一条声称都与代码一致**：3 项虚报（/metrics 真 counter、retention 分批删除、复合索引）补齐实现，或在文档中诚实降级为未完成并列入 backlog——优先补齐实现。
2. **残余摆设清零**：5 个存而不读 settings 开关逐个「真接通或诚实下架」；RoleOperator 拥有一组真实、文档化的默认可达能力（或诚实从 UI 移除该角色档位）。
3. **调度 N+1 收口**：候选过滤的 group 查询改批量预取（一次 `ListGroupIDsByAccounts` / join），百候选下调度路径查询数 O(1)~O(批)。
4. **生成码对账**：用 1.26.3 工具链重跑 `go generate ./ent/...` 与全部 codegen check，确认手改的 probe_model 生成面与生成器输出逐字节一致（不一致则以生成器为准修正）。
5. **部署面一致**：k8s `replicas: 1` + 注释说明解锁条件（/metrics 真 counter 落地 + 验证）；compose 加 scale 护栏注释；核实 `next.config` standalone 输出与 runtime 代理目标真实生效（容器内改 env 能改变代理目标，无需重建镜像）。
6. **文档归真**：parity README 批次表、STATUS.md、ROADMAP.md 与实际状态同步。
7. **分批落 main**：24k 行按域拆成有序 commit 序列（建议 8-12 个：affiliate/promo → RBAC+风控+内容安全 → scheduler+scheduled-test → 账号认证/配额/TLS → 支付 → 计费维度 → 端用户auth+settings → 架构拆分 → NFR并发 → NFR部署 → NFR性能/容量 → 本批修正+文档），**每个 commit 单独通过 build+vet+focused tests**，最后全量门禁一次。
8. **浏览器验证**：对 batch8-19 新增的关键用户可见面跑一遍真实浏览器走查并截图留证：affiliate 邀请→注册→规则→accrual→提现申请→admin 审批全链、调度策略 CRUD+simulate、风控 enforce 真拒绝、支付下单→（沙箱）退款状态机、settings 开关真生效、/me/attributes Required 强制、scheduled-test probe_model。
</objective>

<scope>
**A. 虚报修正（最高优先，全是 NFR 已立项未兑现）**
1. `/metrics` 真 counter：`runtime_metrics.go` 移除抓取期 `usage.List`/`ListDecisions`/`ListLeases` 全表重算，改进程内 `promauto.Counter/Histogram` 在网关/调度路径递增 + `/metrics` 直接 gather；账号健康类指标读 rollup 表。验收：抓取期零全表查询；retention 删行后 counter 不倒退。
2. retention 分批删除：`operations/contract` 增加批大小（默认 1000-5000）+ `entstore/operations/store.go` 改 LIMIT+游标循环删（复用 `usage.CleanupLogs` 批删模式），`CleanupResult` 回报每表删除行数与是否截断。
3. 迁移新增 `usage_logs (provider_id, created_at)` 复合索引（new migration + atlas.sum）。
4. 调度候选 group N+1 → 批量预取。

**B. 摆设对账（铁律：真接通或诚实下架）**
5. `invitation_rebate_enabled`：AccrueRebate 入口（outbox domain_handler）读取该开关，关闭时不产生 accrual 并记审计原因。
6. `balance_low_notify_enabled` / `subscription_expiry_notify_enabled` / `account_quota_notify_enabled`：接到对应通知生产者的 enable 检查；若对应通知生产者本身不存在，则连开关一起诚实下架（OpenAPI/Go/TS/前端四面同步移除）。
7. `request_shaper_enabled`：若 request shaper（payload transform 引擎）应受其控制则接上；否则下架。
8. RoleOperator：定义一组默认权限（只读运营面 + 探测触发），或从前端角色选项移除。

**C. 工程收口**
9. `go generate ./ent/...` 对账 probe_model 手改码；全部 codegen check 重跑。
10. k8s/compose 副本护栏 + 注释；`next.config` standalone 与 runtime 代理核实（起容器验证）。
11. 文档同步：parity README 批次表（batch8-10 补 ✅、本批新增行）、STATUS.md、ROADMAP.md、PROVIDER_AUTH_MATRIX 反向 grep 复核。

**D. 落地与验证**
12. 按域分批 commit（见 objective 7 的切分建议），每 commit 过 build+vet+focused tests；commit message 对应批次主题。
13. `make dev-up` + 浏览器走查（见 objective 8 清单）+ 截图存 `specs/plans/parity/evidence/batch20/`。

本次不包含（明确非范围）：新功能、新 provider、Airwallex（维持「不做假表面」决策）、HA 拓扑/PITR（等 RPO 拍板）、web experience polish B/C/D/E（独立 initiative）、多副本实测（解锁条件未齐）。
</scope>

<constraints>
- 在途 24k 行是已验证资产：除虚报修正与摆设对账涉及的文件外，不得重写/回退既有在途改动。
- OpenAPI-first：任何契约变化先改 `packages/openapi/openapi.yaml`，再生成 Go/TS。
- 摆设处置只有两种：真接通 或 四面同步诚实下架（OpenAPI/Go/TS/前端）。禁止留"开关在 UI 存在但后端忽略"的中间态。
- 每个 commit 必须独立可构建可测试；禁止一个巨型 commit 落 main。
- 浏览器验证发现的任何不一致按「先修后提交」处理，不得带病落 main。
- 全量门禁清单（最终一次全过）：`go test ./... -count=1`、`go vet ./...`、`go build ./...`、architecture+codequality、`make migration-check diff-check openapi-codegen-check openapi-ts-codegen-check`、`cd apps/web && npm run typecheck && npm run lint`。
</constraints>

<success_criteria>
1. `grep` 验证：5 个开关 key 要么有真实消费点（file:line 可指）要么全表面无残留。
2. `/metrics` 抓取路径零 `List(ctx)` 全表调用（代码评审 + 测试断言）。
3. retention 对预置 1 万行测试数据分批删除且单批 ≤ 配置上界。
4. `git log` 显示 ≥8 个按域 commit，每个信息完整；`git status` 干净。
5. `specs/plans/parity/evidence/batch20/` 有走查截图；走查清单逐项打钩记录在 progress.txt。
6. parity README / STATUS / ROADMAP 与代码状态一致，README 本批行标 ✅。
</success_criteria>

<sequencing>
1. 先 C9（ent 对账）——它可能改生成文件，先做避免污染后续 commit 切分。
2. A1-A4 虚报修正（代码改动，进对应域的 commit）。
3. B5-B8 摆设对账。
4. C10-C11 部署面与文档。
5. D12 分批 commit（此时所有代码改动已完成，按域整理提交）。
6. D13 浏览器走查（dev-up 跑在已提交代码上），发现问题 → 修复 → 追加 commit。
</sequencing>

<prohibited>
- 伪造未实现能力；把"声称完成"写进文档而代码不符。
- 用一个 squash commit 落全部 24k 行。
- 跳过浏览器走查（本批的存在意义之一就是补上 batch8-19 全部跳过的浏览器验证）。
- 为通过门禁而放宽 architecture/codequality 阈值。
</prohibited>

<progress_protocol>
每完成一个 scope 项在 progress.txt 追加：做了什么、证据命令+退出码、未做什么及原因。续作时先读 progress.txt 最后一节。
</progress_protocol>

<stop_clause>
全部 success_criteria 满足且最终全量门禁绿、`git status` 干净、浏览器走查证据齐备时停止。若 ent 对账发现生成器输出与手改码语义冲突且无法本地解决，停下并报告差异。
</stop_clause>

> 参考：specs/plans/parity/goal-sub2api-parity-README.md（程序总索引）、progress.txt（batch8-19 全部 evidence）、docs/requirements/QUALITY_GATES.md、docs/constraints/PROVIDER_AUTH_MATRIX.md。
