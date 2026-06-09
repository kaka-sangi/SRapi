# Goal：SRapi 计费深度 + 配额真实性 + 信息架构内聚（第一批）

> 用途：把本文件整段（或下方 `<role_and_context>`…`<stop_clause>` 部分）粘贴给编码 agent（Codex / Claude Code）作为目标提示词。
> 背景：基于 12-agent 对比 SRapi vs sub2api 的核查结论，闭合经确认的关键缺口第一批（rank 1-5）。其余缺口已列为 out-of-scope，防止 scope creep。
> 生成日期：2026-06-08。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first，schema 在 apps/api/ent/schema/，业务在 apps/api/internal/modules/<域>/{contract,service,store}，HTTP 在 apps/api/internal/httpserver/）；前端 Next.js + TypeScript（apps/web，路由在 apps/web/src/app/，组件在 apps/web/src/components/，导航在 apps/web/src/components/layout/nav-items.ts）。
本地开发：make dev-up + npm run dev；不要设置 NEXT_PUBLIC_SRAPI_BASE_URL（CSP 会拦跨域）；登录 admin@srapi.local / Admin1234。
所有改动须通过 go build/vet + 前端 tsc/lint，遵循 OpenAPI-first（先改 spec 再生成）。绝不抄 sub2api 源码，只借鉴其能力与算法思路。
</role_and_context>

<objective>
目标（可衡量最终态）：闭合 SRapi 相对 sub2api 在「计费引擎深度」「配额真实性」「信息架构内聚」三条主线上经核查确认的关键缺口，直到：(a) 未配价模型不再静默按 0 元放行；(b) 配额调度不再被合成快照淹没；(c) 网关配置从 6+ 散页收敛为以渠道为中心的聚合视图；每项都有可观察证据（编译通过 + 针对性测试绿 + 浏览器验证）。
动机：这些是收入安全（漏收/0 价放行）、运营可观测（盲调）、配置可用性（运营找不到入口）三类直接影响平台能否真实运营的问题；业务面完整（把后端已有/应有能力在 UI 闭环暴露）是本仓库的 TOP 优先级，优先于工艺打磨。
</objective>

<scope>
本次包含（按 sequencing 分批，一次只推进一个单元）：
1. 配额正确性：分离合成配额快照与真实上游快照，使调度器/告警不再读到 ratio 恒=1 的合成快照。
2. 定价兜底：未命中 PricingRule 时按 model.family 模糊匹配兜底，杜绝 default_zero 静默 0 元。
3. 计费倍率：AccountGroup 加 rate_multiplier，计费产出 actual_cost 并在 UsageLog 快照倍率。
4. 信息架构：建立 /admin/channels/[id] 聚合详情页（tab 收纳 定价/模型映射/模型限制/payload规则/错误透传/TLS指纹），把 models/payload-rules/error-passthrough/tls-profiles 从 Gateway 顶层降为详情 tab；channelsPricing 从 Commerce 段移回 Gateway。
5. 用户面闭环：dashboard 露出余额卡（复用 useBalance）+ 新增 /me/platform-quotas 与平台配额卡。

明确不做（out-of-scope，本批次刻意排除，避免 scope creep）：
- 不做 LiteLLM 远程价表定时同步（仅做本地 family 兜底）。
- 不做 Anthropic OAuth 真实额度拉取、Gemini RPD/RPM 视图、CRS 整库迁移、per-platform 一键 OAuth Preset（这些是后续批次）。
- 不做 token 区间分层/billing_mode/cache 5m-1h 分档（留待 schema 二期，本批 UsageLog 改动只加 rate_multiplier/actual_cost）。
- 不重构 scheduler 调度算法本体、不动 entstore 的 Serializable 事务机制、不顺手清理无关代码。
- 不退回明文存储凭证、不删除 BillingLedger、不把单页多 tab 的 settings/billing/account 拆回多页。
</scope>

<constraints>
途中不得改变：
- 金额一律用 string + big.Rat 8 位定点（保留 decimalRat/formatRatFixed/cacheWriteRateOrInput 防漏收语义），绝不改回 float64。
- PricingRule 的 effective_from/effective_to 时间窗与 provider_id=0 通配优先级规则不得破坏。
- 凭证 AES-GCM 加密（credential_ciphertext）不得改为明文。
- 公共 API 签名与现有 OpenAPI 端点的兼容性：新增端点可以，破坏现有响应结构需先说明。
- 不得修改与本任务无关的 *_test.go；新增能力必须配新测试。
- ent schema 改动后必须重新生成代码（go generate ./ent/...）并确认 store/mock 同步（memory store 的 DeletedAt 过滤、Store-mock codegen 是已知坑）。
其他约束：新增依赖须先说明理由；前端遵循 warm-paper 视觉与既有 PageHeader/CardTitle/StatCard 组件，不引入新设计语言。
</constraints>

<success_criteria>
完成需同时满足（每条都要在你的输出里给出可观察证据，评估器不会自己跑命令）：
1. 后端编译与静态检查：cd apps/api && go build ./... && go vet ./...，退出码 0（贴出输出尾部）。
2. 配额修复可证：新增/修改测试证明——写入一条真实 quota 信号 + 一条合成快照后，调度器读取的 QuotaRemainingRatio 反映真实信号而非恒=1；贴出该测试名与 go test 通过输出。
3. 定价兜底可证：新增测试证明——对未配 PricingRule 但 model.family=opus 的请求，EstimatePrice 返回非零兜底价（不再 default_zero）；贴出测试通过输出。
4. 计费倍率可证：新增测试证明——AccountGroup.rate_multiplier=0.8 时 actual_cost = cost × 0.8 且 UsageLog 快照了倍率；贴出测试通过输出。
5. 前端编译：cd apps/web && npm run lint && npx tsc --noEmit，退出码 0（贴出尾部）。
6. 信息架构可证：nav-items.ts 中 ADMIN_GATEWAY_SECTION 项数从 9 降到 ≤5，channelsPricing 不再在 ADMIN_COMMERCE_SECTION；/admin/channels/[id] 页存在且含列出的 tab。贴出改后的 nav-items.ts 相关片段。
7. 浏览器验证（用 chrome-devtools）：登录后截图证明——(a) /admin/channels/[id] 聚合页 tab 可切换；(b) 用户 dashboard 顶部显示余额卡。
8. git status 干净（除预期改动），git diff --stat 贴出。
或 30 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、一次推进一个单元，每个单元自带测试并单独 commit：
1. 阶段一（地基/数据层）：ent schema 改动（AccountGroup.rate_multiplier、UsageLog.actual_cost+rate_multiplier），go generate，同步 store/mock。
2. 阶段二（计费逻辑）：定价 family 兜底（subscriptions/service service.go selectPricingRule 未命中分支）+ rate_multiplier 计入 actual_cost；配单测。
3. 阶段三（配额修复）：分离合成 vs 真实快照（runtime_filters.go:249 / runtime_gateway_usage.go:554-562 / store ListQuotaSnapshotsByAccount 按 quota_type 分桶 / runtime_gateway_core.go:1187 读取过滤）；配单测。
4. 阶段四（后端用户端点）：新增 GET /api/v1/me/platform-quotas（先改 OpenAPI spec 再生成）。
5. 阶段五（前端 IA）：nav-items.ts 重组 + /admin/channels/[id] 聚合页 + dashboard 余额/配额卡。
6. 阶段六：跑全量 go test ./... 与前端构建 + 浏览器验证。
每完成一个阶段提交一次 commit，commit message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：一个 feat 分支上的若干语义化 commit（每阶段一个）+ 改后的 nav-items.ts + 新 /admin/channels/[id]/page.tsx + 新增测试文件（全部 passing）+ 一段 progress.txt 收尾，列出已完成阶段、各成功标准的证据位置、未做的 out-of-scope 项。
</artifact>

<guardrails>
绝不：删除或改写既有测试让其通过；hardcode 让测试通过；为过测试 --no-verify 跳过校验或 git push --force；把金额改回 float；退回明文凭证；伪造未实际接入的能力（如 5m/1h 分档、billing_mode——本批不做就不要假装做）。
先问我：任何破坏现有 OpenAPI 响应结构、删除现有顶层路由文件、或不可逆的数据迁移操作。
正常措辞执行即可，无需过度强调。
</guardrails>

<progress_and_resume>
每完成一个阶段更新 progress.txt（自由文本：做了什么、证据在哪、下一步）；用 git commit 记录每个检查点。
新 context 开始时：先 pwd、git log --oneline -10、读 progress.txt，再 cd apps/api && go build ./... 确认当前编译态，然后从 progress.txt 的下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 30 turns 仍未达成，停下并汇总：已完成阶段、当前编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（绝对路径以仓库根 /home/senran/Desktop/SRapi 为基准）：
- 定价/计价：apps/api/internal/modules/subscriptions/service/service.go（EstimatePrice:515, selectPricingRule:654, priceFromRule:695, defaultMoney:776）
- 定价 schema：apps/api/ent/schema/pricingrule.go（单价四字段 + effective 时间窗）
- 用量 schema：apps/api/ent/schema/usagelog.go（仅 cost+billable_cost，需加 actual_cost/rate_multiplier）
- 分组 schema：apps/api/ent/schema/accountgroup.go（加 rate_multiplier）
- 0价放行点：apps/api/internal/httpserver/runtime_gateway_core.go（default_zero:558-560；调度读快照:1187；effectivePlatformLimits:645）
- 合成快照：apps/api/internal/httpserver/runtime_filters.go:249；runtime_gateway_usage.go:554-562
- 配额快照 store：apps/api/internal/persistence/entstore ListQuotaSnapshotsByAccount（按 SnapshotAt 倒序、需按 quota_type 分桶）
- 扣费/账本：apps/api/internal/persistence/entstore/billing/store.go（ChargeUsage:103）
- 导航：apps/web/src/components/layout/nav-items.ts（ADMIN_GATEWAY_SECTION:102-115 共9项；channelsPricing 错置 Commerce:123；navSectionsForRole:184）
- 定价页/模型页：apps/web/src/app/admin/channels/pricing/page.tsx；apps/web/src/app/admin/models/page.tsx（pricingOverride keyvalue:175-181）
- 用户 dashboard：apps/web/src/components/features/gateway-overview.tsx（无 useBalance，需加余额卡）；billing 页 useBalance 参考：apps/web/src/app/billing/page.tsx:80
sub2api 仅作能力对照，不抄代码：group.rate_multiplier、usage_log 四项 cost+快照、ModelPricingResolver family 模糊匹配、AccountUsageService 的 Source=passive/active 区分。
</references>

完成后开始下一批次。后续批次已写成独立 goal 文档，见总索引 [goal-sub2api-parity-README.md](./goal-sub2api-parity-README.md)：
- 第二批 [batch2](./goal-sub2api-parity-batch2.md)：计费域地基重构（money 公共包 + 定价归位，rank 16/8）
- 第三批 [batch3](./goal-sub2api-parity-batch3.md)：计费维度深化（billing_mode/区间 + usage_log 分解，rank 5/12）
- 第四批 [batch4](./goal-sub2api-parity-batch4.md)：额度护栏与性能（订阅行物化 + Key USD 配额，rank 9/15）
- 第五批 [batch5](./goal-sub2api-parity-batch5.md)：配额真实性 + 账户导入认证（rank 6/7/13/17）
- 第六批 [batch6](./goal-sub2api-parity-batch6.md)：信息架构收尾 + 用户自助 + 去重（rank 10/14/18）

下表为后续批次涵盖项一览（详见各批文档）：

| # | 行动 | 类别 |
|---|---|---|
| 5 | 按次/图片 billing_mode + token 区间分层定价 | 计费深度 |
| 6 | Anthropic OAuth 真实额度拉取（5h/7d/7d-Sonnet） | 配额获取 |
| 7 | Preset 增 OAuth 配置 → per-platform 一键授权 | 账户认证 |
| 8 | 把定价从 subscriptions 抽到独立 billing 域 | 模块边界 |
| 9 | 订阅行物化 daily/weekly/monthly 用量（消除全表重扫） | 计费性能 |
| 12 | usage_log 成本四项分解 + requested/upstream 快照 | 对账审计 |
| 13 | 统一导入入口 + 共用指纹去重 | 账户导入 |
| 14 | ops 监控收进 /admin/ops tab + scheduler-decisions 归位 | 信息架构 |
| 15 | API Key 加 USD 成本配额 + 滑窗花费上限 | 成本护栏 |
| 16 | 抽 money 公共包 + 统一 USD/0 常量 | 去重 |
| 17 | 账号封禁/验证状态结构化识别 + 用户端配额自助/reset | 配额健康 |
| 18 | 统一 usage 聚合 struct + dashboard/usage 共享 hook | 去重 |
