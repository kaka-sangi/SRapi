# Goal：额度护栏与计费性能（订阅行物化用量 + API Key USD 配额）（第四批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：覆盖 ranked backlog rank 9 + rank 15。消除「每请求全表重扫日志现算周期用量」的性能劣化，并补齐下游分发 key 最常见的「按金额预算」护栏。两项共用滑动窗口边界工具。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first，schema 在 apps/api/ent/schema/，网关在 apps/api/internal/httpserver/，订阅在 modules/subscriptions，apikey 准入在网关 admission 路径）；前端 Next.js + TypeScript（apps/web）。
本地开发：make dev-up + npm run dev；登录 admin@srapi.local / Admin1234。所有改动须过 go build/vet + 前端 tsc/lint，遵循 OpenAPI-first。绝不抄 sub2api 源码。
</role_and_context>

<objective>
目标（可衡量最终态）：(a) UserSubscription 行上物化 daily/weekly/monthly 已用额（USD）+ window_start，计费时增量累加并惰性/定时重置，请求热路径只读一个数而非全表重扫日志；(b) APIKey 支持 USD 成本配额（cost_quota/cost_used）与 5h/1d/7d 滑窗花费上限，准入/扣费时累加并超限拒绝。两项统一额度判定口径（建议 billable_cost）。完成后有测试证明命中限额会拒绝、且热路径不再做全表扫描。
动机：现在 gatewayUserPeriodUsage + gatewayUserPlatformSpend + gatewayBillableCost 每请求最多 3 次对同一用户全量 ListByUser 扫描、仅月维度，I/O 随日志量线性放大；API Key 只有次数/速率限额，下游分发 key 时「最多花 $X / 每5h最多 $Y」的预算护栏缺失。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元）：
1. schema：UserSubscription 增 daily_usage_usd/weekly_usage_usd/monthly_usage_usd + window_start（或各周期独立 window_start）。APIKey 增 cost_quota/cost_used + cost_used_5h/1d/7d + window_start。
2. 滑窗工具：抽一个 window 边界工具（UTC 日/周/月起点 + 滑动 5h/1d/7d 边界计算 + 重置判定），供订阅与 apikey 共用，消除时间边界算法重复。
3. 累加与重置：计费成功后增量累加到订阅与 key 的窗口用量；读取时按 window_start 惰性重置过期窗口（或 worker 定时重置）。
4. 准入判定：admission/扣费路径用物化字段判定订阅 allowance 与 key cost 上限，超限拒绝；统一 allowance/quota 判定口径为 billable_cost。
5. 前端：订阅页用物化字段画 daily/weekly/monthly 进度条；API Key 编辑器暴露 cost_quota 与滑窗上限并展示已用。

明确不做（out-of-scope）：
- 不改既有的次数/速率限额（rpm/tpm/concurrency/request_limit_5h/1d/7d）语义，只新增 USD 维度。
- 不做 per-user-platform 配额的用户自助（属第六批/配额批）。
- 不把账号快照查询改造为事件驱动聚合（可选优化，本批至少给 usage store 加带 account_id+时间窗+Limit 的查询方法把快照查询移出全表扫描即可）。
- 不动 scheduler、不改 OpenAPI 既有响应结构。
</scope>

<constraints>
途中不得改变：
- 金额 string + big.Rat 8 位定点（用 money 公共包），绝不改回 float。
- 物化用量与「按日志重算」在迁移期必须一致：提供一个校验测试或脚本证明物化值 = 日志聚合值（至少单测层面）。
- 现有限额拒绝行为（次数/速率）不得回归。
- ent schema 改动后 go generate ./ent/...，同步 store/mock。
- 不得修改无关 *_test.go；新增能力必须配新测试。
其他约束：前端遵循 warm-paper 视觉与既有组件。
</constraints>

<success_criteria>
完成需同时满足（每条给出可观察证据）：
1. cd apps/api && go build ./... && go vet ./...，退出码 0（贴尾部）。
2. 物化正确性可证：测试证明连续 N 次计费后 monthly_usage_usd = 各次 billable_cost 之和，且跨窗口边界会重置；贴出测试通过输出。
3. 热路径可证：测试或代码证明请求路径周期用量改为读物化字段，不再对用户日志做全量 ListByUser 扫描（贴出改后的读取点 + 说明）。
4. key 成本护栏可证：测试证明 cost_quota 用尽 / 5h 滑窗花费超上限时准入被拒（返回限额错误）；贴出测试通过输出。
5. 前端：cd apps/web && npm run lint && npx tsc --noEmit 退出码 0（贴尾部）。
6. 浏览器验证（chrome-devtools）：截图证明订阅页 daily/weekly/monthly 进度条、API Key 编辑器 cost 配额字段与已用展示。
7. go test ./... 全绿 + git status 干净，git diff --stat 贴出。
或 30 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
每单元自带测试并单独 commit：
1. 阶段一：滑窗 window 工具 + 单测（UTC 月初/周初、滑动 5h/1d/7d 边界、重置判定）。
2. 阶段二：schema（UserSubscription 三周期用量+window_start；APIKey cost 字段+滑窗）→ go generate → 同步 store/mock。
3. 阶段三：计费成功后累加 + 惰性/定时重置；统一 billable_cost 口径；配单测（含物化=日志聚合校验）。
4. 阶段四：admission/扣费用物化字段判定并拒绝超限；usage store 加带窗口+Limit 查询移出全表扫描。
5. 阶段五：前端进度条 + key cost 配额 UI。
6. 阶段六：全量 go test + 前端构建 + 浏览器验证。
每阶段一个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + 滑窗工具 + schema/累加/准入/前端改动 + 新增测试（全 passing）+ progress.txt 收尾。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 过测试；--no-verify 或 git push --force；把金额改回 float；让物化用量与真实计费口径不一致还假装一致。
先问我：破坏现有 OpenAPI 响应结构、删除现有顶层路由文件、不可逆数据迁移（尤其物化字段的回填迁移策略需先说明）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt + git commit。
新 context 开始：先 pwd、git log --oneline -10、读 progress.txt，再 go build ./... 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 30 turns 仍未达成，停下汇总：已完成阶段、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
- 周期用量热路径（待消除全表扫描）：apps/api/internal/httpserver/runtime_gateway_core.go（gatewayUserPeriodUsage:588 / gatewayUserPlatformSpend:611 / effectivePlatformLimits:645）。
- 账号快照全表拉取：apps/api/internal/persistence/entstore/usage/store.go:64（List 无 Where/Limit，需加带 account_id+时间窗+Limit 的查询）。
- 订阅 schema：apps/api/ent/schema/usersubscription.go（无 usage/window 列）。
- API Key schema：apps/api/ent/schema/apikey.go:27-32（仅 rpm/tpm/concurrency/request_limit_5h/1d/7d，需加 cost 维度）。
- 订阅页：apps/web/src/app/admin/subscriptions/page.tsx 及用户订阅展示；API Key 编辑器：apps/web/src/app/api-keys/page.tsx 与 admin/api-keys。
sub2api 仅作能力对照不抄代码：滑动窗口花费上限思路、账号用量按 source 区分。
</references>
