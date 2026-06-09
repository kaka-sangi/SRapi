# Goal：信息架构收尾 + 用户自助 + 去重（第六批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：覆盖 ranked backlog rank 10 + 14 + 18。收口「功能散乱 / 信息重复」：新增用户端可用渠道自助页、把 ops 监控收进 tab 并让 scheduler-decisions 归位、统一 usage 聚合消除前后端重复。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first，用量在 modules/usage，网关在 httpserver）；前端 Next.js + TypeScript（apps/web，路由在 apps/web/src/app/，导航在 apps/web/src/components/layout/nav-items.ts）。
本地开发：make dev-up + npm run dev；登录 admin@srapi.local / Admin1234。所有改动须过 go build/vet + 前端 tsc/lint，遵循 OpenAPI-first。绝不抄 sub2api 源码。
</role_and_context>

<objective>
目标（可衡量最终态）：(a) 用户端新增「可用渠道/模型 + 单价 + 渠道状态」自助页，并提供 GET /api/v1/me/available-models 作为模型清单的单一权威来源；(b) ops 监控的 4-5 个分散入口收进 /admin/ops 下的 tab，scheduler-decisions 从用户顶层路由移到 admin/ops 下，provider-accounts 只读视图并入 admin/accounts 筛选；(c) 后端用单一 UsageAggregate 承载所有用量维度、前端抽 useUsageTotals 共享 hook，消除四个同构 struct + 三套累加 + dashboard/usage 各算一遍。完成后有浏览器验证与编译佐证。
动机：网关/运维入口散乱让运营找不到东西、scheduler-decisions「用户路由却仅 admin 显示」入口分裂、用户发请求前无法预估成本判断模型可用性；usage 聚合在前后端重复实现导致口径漂移与维护成本。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元）：
1. 单一权威模型清单：后端 GET /api/v1/me/available-models（或 /me/channels）返回当前用户分组可见模型 + 单价 + 所属渠道 + 可用性（先改 OpenAPI spec 再生成）；前端新增 app/available-channels/page.tsx 放进 WORKSPACE_SECTION，作为模型清单单一权威来源，api-keys 编辑器与 usage 筛选都引用它（消除三处模型清单语义各异）。
2. ops 收口：把 channels/monitor、scheduled-tests、ops/strategy、scheduler-decisions 收进 /admin/ops 下 tab（总览/渠道监控/定时探测/调度策略/调度决策）；scheduler-decisions 从用户顶层 /scheduler-decisions 移入 admin/ops；删除 navSectionsForRole 中单独追加给 admin 的 GATEWAY_SECTION。
3. 双入口合并：确认 provider-accounts（163 行只读）是 admin/accounts 的只读子集后，并为 accounts 的一个只读/健康筛选 tab，删除重复顶层入口。
4. usage 去重：后端用单一 UsageAggregate{Key,Type,...} 承载 window/model/daily 维度、前端按 Type 区分，删并行 struct 与累加函数；前端抽 useUsageTotals(logs) 共享 hook 供 dashboard（概览）与 usage（明细+筛选）复用。

明确不做（out-of-scope）：
- 不重组语义化 NavSection 骨架本身（这是 SRapi 优于 sub2api 扁平列表之处，仅做组内合并）。
- 不把 settings/billing/account 等单页多 tab 拆回多页。
- 不动计费逻辑、不改 schema（本批以前端 IA + 只读聚合端点为主；available-models 端点只读聚合现有数据）。
- 第四批若已加 dashboard 余额卡，本批不重复做（仅在 available-channels 引用定价）。
</scope>

<constraints>
途中不得改变：
- 现有页面的功能不能丢：合并/移动入口时，被合并页的能力必须在新位置完整可达。
- /v1/models（网关 key 专用）行为不变；新 /me/available-models 是控制台会话鉴权的另一端点。
- 现有 OpenAPI 响应结构兼容：新增端点可以，破坏现有结构需先说明。
- 路由迁移用重定向或导航更新，不要留下死链；删除顶层路由文件前先确认无外部引用（删除前先问）。
- 不得修改无关 *_test.go；新增端点配新测试。
其他约束：前端遵循 warm-paper 视觉与既有 PageHeader/CardTitle/StatCard 组件。
</constraints>

<success_criteria>
完成需同时满足（每条给出可观察证据）：
1. cd apps/api && go build ./... && go vet ./... 退出码 0（贴尾部）；新 /me/available-models 端点有单测（返回用户可见模型+单价），贴出测试通过输出。
2. 单一权威可证：grep 证明 api-keys 编辑器与 usage 筛选的模型清单来源指向同一 hook/端点（贴出引用点）。
3. usage 去重可证：后端只剩单一 UsageAggregate（贴出 grep，旧的 UsageWindowSummary/UsageModelSummary/UsageDailySummary 已移除或合并）；前端 dashboard 与 usage 都用 useUsageTotals（贴出引用点）。
4. nav 可证：nav-items.ts 中 ops 顶层入口 ≤5、scheduler-decisions 不再在用户顶层路由、navSectionsForRole 不再单独追加 GATEWAY_SECTION（贴出相关片段）。
5. 前端：cd apps/web && npm run lint && npx tsc --noEmit 退出码 0（贴尾部）。
6. 浏览器验证（chrome-devtools）：截图证明（a）用户端 available-channels 页显示模型+单价+状态；（b）/admin/ops 下 tab 含渠道监控/定时探测/调度策略/调度决策且可切换；（c）原 provider-accounts、scheduler-decisions 顶层入口已不在侧边栏、能力在新位置可达。
7. go test ./... 全绿 + git status 干净，git diff --stat 贴出。
或 30 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
每单元自带测试/验证并单独 commit：
1. 阶段一：后端 /me/available-models（改 spec→生成→实现→单测）。
2. 阶段二：前端 available-channels 页 + 接入 WORKSPACE_SECTION + api-keys/usage 引用统一来源。
3. 阶段三：usage 聚合后端合并为 UsageAggregate + 前端 useUsageTotals 共享 hook。
4. 阶段四：/admin/ops tab 收口 + scheduler-decisions 归位 + provider-accounts 合并 + nav-items.ts 调整。
5. 阶段五：前端构建 + 浏览器验证全部入口可达。
每阶段一个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + 新 /me/available-models 端点 + available-channels 页 + 合并后的 ops/accounts 导航 + 统一 UsageAggregate/useUsageTotals + 新增测试（全 passing）+ progress.txt 收尾（含被移动/合并入口的新位置清单）。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 过测试；--no-verify 或 git push --force；在合并入口时丢失被合并页的能力；留下死链。
先问我：删除任何现有顶层路由文件、破坏现有 OpenAPI 响应结构。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（含入口迁移映射表）+ git commit。
新 context 开始：先 pwd、git log --oneline -10、读 progress.txt，再 go build ./... 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 30 turns 仍未达成，停下汇总：已完成阶段、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
- 导航：apps/web/src/components/layout/nav-items.ts（ADMIN_GATEWAY_SECTION:102-115；channelsPricing:123；GATEWAY_SECTION 追加 navSectionsForRole:184-186）。
- ops 分散入口：apps/web/src/app/admin/ops/page.tsx；admin/channels/monitor；admin/scheduled-tests；admin/ops/strategy；用户顶层 apps/web/src/app/scheduler-decisions/page.tsx（allowedRole=admin）。
- 双入口：apps/web/src/app/provider-accounts/page.tsx（163 行只读）vs apps/web/src/app/admin/accounts/page.tsx（完整 CRUD）。
- 模型清单单一权威：网关 /v1/models（gatewayBearerAuth 专用）；api-key allowed_models；playground models；usage 去重。
- usage 聚合重复：apps/api/internal/modules/usage/service.go:402-494（UsageWindowSummary/UsageModelSummary/UsageDailySummary/UsageAggregate 字段全同 + 三套累加）；前端 apps/web/src/components/features/gateway-overview.tsx:73-78 与 apps/web/src/app/usage/page.tsx:104-117（各从 useUsageLogs /me/usage 各写一遍聚合）。
sub2api 仅作能力对照不抄代码：AvailableChannelsView / ChannelStatusView 用户自助、ChannelsView per-channel 聚合。
</references>
