# Goal：计费维度深化（service_tier / cache 分档 / 长上下文 / 图片 token / BillingModelSource）+ daily/weekly 配额 + usage 聚合分解（第十三批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查把曾被推迟的「二期计费维度」全部编入本批，并补两处计费侧摆设/半摆设——(1) **daily/weekly 配额键缺失**（`daily_usage_usd`/`weekly_usage_usd` 完整累加+按窗口重置、UI 还画了进度条，但全仓无 daily/weekly 配额键，`CheckEntitlement`/`CostAllowance` 只读 monthly，daily/weekly 进度条永不传 limit → percent 恒 0、宽度恒 0%，是典型「画了进度条但永远空」的摆设）；(2) **usage 聚合丢成本分解**（per-log 已记录 input/output/cache_read/cache_write 成本并 UI 展示，但 `UsageAggregate`/`APIKeyUsageSummary`/`usageAccumulator` 聚合层只汇总 `TotalCost`，运营无法在汇总层看 cache 成本占比，半摆设）；(3) **模型注册表 `quality_tier` 字段选路/计费零消费**（`Candidate` 结构体只灌 `ModelFamily`，但 UI 帮助文案称「调度器在 quality 策略时使用」与代码矛盾，摆设文案）；其余 rank43-48 是 sub2api 有而 SRapi 计费引擎缺的真实定价维度（service_tier 倍率价 / cache 5m/1h 分档 / 长上下文整次倍率 / 图片输出 token 独立费率 / BillingModelSource / LiteLLM 同步）。这是「业务面完整」收口计费域的最后一批深化。
> 前置：建议 batch2（money 公共包 + 定价归位到 billing 域，PricingRule/EstimatePrice 已在 `modules/billing`）已落地——本批所有定价改动落在 `modules/billing/service/pricing.go` 与 `contract.go`，依赖该归位结果。与 batch12（支付上游真实性）同属 commerce/billing 域，建议接 batch12 后做。LiteLLM（rank48）可放在本批最后做。
> 关联：与 batch8（promo/affiliate）无冲突；与 batch15（架构清理）有隐性依赖——本批会给 `pricing.go`/`runtime_gateway_core.go` 增行，而 `runtime_gateway_core.go` 仅余 44 行到 2200 红线，故凡触该文件务必先按 batch15 rank59 的拆分思路腾余量或把新逻辑落到下沉后的小文件，不得把巨文件推过红线（见 `<constraints>`）。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个对标 sub2api 的 AI 网关/计费平台。
技术栈与约定：
- 后端 Go + ent ORM，OpenAPI-first：schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`。改 API 须**先改 `packages/openapi/openapi.yaml` 再生成**，再落 handler/service。
- 前端 Next.js + TS：`apps/web`，路由在 `app/`，组件在 `components/`，导航在 `components/layout/nav-items.ts`。
- 本地开发：`make dev-up` + `npm run dev`；登录 admin@srapi.local / Admin1234。
- 所有改动须过 `cd apps/api && go build ./... && go vet ./...`；前端须过 `tsc` / `lint`。
- 架构红线（`apps/api/internal/architecture` + `internal/codequality` 两个守门测试）：模块生产码只能 import 别模块的 **contract** 层（白名单）；contract 层禁止 import Ent / 生成的 OpenAPI DTO / HTTP server 包；单文件 ≤ 2200 行（`runtime_*` 文件）、单函数 ≤ 210 行；全部 `gofmt`。
- 凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
本批是**计费引擎深化**：金额一律 `string` + `big.Rat` 8 位定点，绝不引入 `float64`；每加一个计费维度都要有等价/差异对照测试证明既有计费结果不被破坏。强调点：「摆设」类目标（daily/weekly 配额、usage 聚合分解、quality_tier 文案）必须**真接通到消费点**或**诚实下架文案/字段**，不得保留「字段在但永远不生效」的假象。
</role_and_context>

<objective>
目标（可衡量最终态）：SRapi 计费引擎覆盖 sub2api 全部强制计费维度，且本批涉及的三处计费摆设全部被消除——daily/weekly 配额从「画了空进度条」变为「真做 hard_cap/allowance 判定且 UI 进度条有宽度」、usage 汇总层从「只有总成本」变为「按 input/output/cache 分解可见」、模型注册表 `quality_tier` 从「文案承诺但代码不用」变为「真灌入选路或文案诚实降级为纯展示」；同时补齐 service_tier 倍率价、cache 5m/1h 分档、长上下文整次倍率、图片输出 token 独立费率、BillingModelSource、LiteLLM 价表同步六个二期维度。完成后所有维度都有针对性测试，且原有计费金额在未触发新维度时逐位不变。
动机：计费维度缺失=少收/错收钱（service_tier priority 应 2x 却按 1x、cache 1h 应贵却按 5m、长上下文应乘倍率却平价），直接是资金漏洞；daily/weekly 配额摆设=运营以为设了日限额其实从不强制（额度形同虚设的安全/资金假象）；usage 聚合丢分解=运营看不清 cache 成本占比无法优化定价。这些是计费域「业务面完整」的最后缺口。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个维度；每个维度的 schema/契约改动先改 OpenAPI/ent 再生成；每项带测试、单独 commit）：

1. **rank12 daily/weekly 配额键（最高强制价值，先做）**：entitlement 结构增 `daily_cost_quota`/`weekly_cost_quota`（及可选 token 版）；`CheckEntitlement`/`CostAllowance` 用 `MaterializedUsage.DailyUsageUSD`/`WeeklyUsageUSD` 做与 monthly **同构**的 `hard_cap`/`allowance` 判定（参见 `apps/api/internal/modules/subscriptions/domain/usage.go:28-36` 的 `ApplyUsageDelta` 已累加 Daily/Weekly/Monthly 三窗、`ResetExpiredUsage` 已按窗重置——数据源现成，只缺配额侧消费）；前端 `subscription-usage-bars.tsx:24-25` 的 daily/weekly `UsageBar` 传入对应 `limit`（现仅 monthly 传 limit）。**接通的消费点**：`CheckEntitlement` 的 daily/weekly 分支真返回 deny/allowance，UsageBar 真有宽度。

2. **rank30 usage 聚合成本分解（半摆设）**：`usageAccumulator`（`apps/api/internal/modules/usage/service/service.go:359-387`，现仅 `totalCost *big.Rat`）增 `inputCost/outputCost/cacheReadCost/cacheWriteCost` 四个 `*big.Rat` 累加；`UsageAggregate`（`contract.go:161-174`，现仅 `TotalCost`）与 `APIKeyUsageSummary` 增对应字段；`aggregate()` 一并 sum 并 `money.FormatRatFixed(...,8)`；OpenAPI usage 汇总响应增字段；前端 usage 汇总卡展示成本分解。per-log 侧已暴露（`runtime_api_mapping.go:554-557`），只补聚合层。

3. **rank14 模型注册表 quality_tier（摆设文案，二选一落定）**：(a) `scheduler/contract` 的 `Candidate`（`apps/api/internal/modules/scheduler/contract/contract.go:63`，现有 `ModelFamily` 无 `QualityTier`）增 `QualityTier`，在候选组装处（`runtime_gateway_core.go:217/1166`，现只灌 `ModelFamily`）用 `resolution.Model.QualityTier` 灌入，`firstQualityTier` 兜底回退注册表值，使 quality 策略真消费注册表字段；**或** (b) 删 UI「调度器在 quality 策略时使用」误导文案（`apps/web/src/locales/zh.ts:802` 与 `en.ts:807`），降为纯展示并在 help 文案说明「当前 quality 评分来自账号 metadata / pricing_override / 在线评分」。默认 (a) 真接通；若 (a) 工作量超预算则 (b) 诚实降级——二选一无中间态。

4. **rank43 service_tier 倍率价（priority 2x / flex 0.5x）**：`PricingRequest`（`apps/api/internal/modules/billing/contract/contract.go:110-122`）增 `ServiceTier`；`PricingRule` 增 priority 档价或倍率；`tokenPriceFromRule` 按 tier 选价 / 乘倍率；`gatewayPricingRequest` 从 canonical 透传（网关请求体已 normalize service_tier，见 `codex_responses_payload.go:290`，只缺计费侧读取）。

5. **rank44 cache 创建 5m/1h 分档计费**：Anthropic usage 解析（`apps/api/internal/modules/provider_adapters/service/conversation_anthropic_usage.go:26-27`，现仅 `CacheCreationInputTokens`/`CacheReadInputTokens` 两字段）增 `ephemeral_5m`/`ephemeral_1h` 拆分；`Usage`/`UsageLog` 增 5m/1h token 与对应价；pricing 增两档 cache-write 价；**无明细时回退全部 5m**（防漏收）。

6. **rank45 长上下文整次会话倍率**：`PricingRule` 增 `long_context_threshold` + `multiplier`，**仅当无 token 区间时**对 input/output/cacheRead/cacheCreation 整次乘倍率（对齐既有 `selectTokenPricingInterval` 阶梯换价：区间存在则走阶梯，不存在且超阈值则走整次倍率，二者互斥）。

7. **rank47 图片输出 token 独立费率 / 尺寸倍率表**：token 模式下从 output token 拆出 `image_output_tokens` 给独立价（对齐 sub2api `ImageOutputPricePerToken`）；可选加图片尺寸倍率表。per-image 按尺寸价已端到端可用（`selectImagePricingInterval`），本项补 token 子集独立费率。

8. **rank46 BillingModelSource（requested / upstream / channel_mapped）**：mapping/channel 加 `billing_model_source` 开关；`EstimatePrice` 据此决定用 requested 还是 upstream 模型名解析定价规则（usagelog 已记 `requested_model`/`upstream_model` 两者，只缺「用哪个定价」的选择器）。

9. **rank48 LiteLLM 远程价表定时同步 worker（本批最后做）**：新增 worker 定时拉 LiteLLM/上游价表 upsert 进 `pricing_rule`（family 级，优先级**低于**强制维度规则，作兜底）；用 httptest mock 价表源，不打真实远端。

明确不做（out-of-scope，均由其它批次涵盖，非永久搁置）：
- 不做计费引擎的金额公共包 / 定价归位（已在 **batch2** 完成，本批直接复用 `internal/pkg/money` 与 `modules/billing` 的 PricingRule）。
- 不做支付上游退款 / 对账 / 手续费 / 通道负载均衡 / Airwallex / webhook 取证（**batch12** 涵盖）。
- 不做 daily/weekly 之外的 RPD/per-day **上游账号**配额追踪（属 accounts 域 quota，**batch11** rank38/39 涵盖；本批 daily/weekly 是**用户订阅**侧成本配额，勿混）。
- 不顺手拆 `runtime_gateway_core.go`/`pricing.go` 等巨文件做纯机械重构（**batch15** 涵盖；本批只在触线时做最小腾挪，见 constraints）。
- 不改 scheduler 策略 CRUD / scope / 权重键改名（**batch10** 涵盖）；本批 rank14 只动 `Candidate.QualityTier` 灌入或文案，不碰策略加载。
</scope>

<constraints>
途中不得改变：
- 金额一律 `string` + `big.Rat` 8 位定点，绝不引入 `float64`；保留既有 0 价回退防漏收语义（如 `cacheWriteRateOrInput`）。
- 不破坏 batch1-12 已落地能力：batch2 的 money 公共包 / billing 定价归位、batch8 的 promo/affiliate 资金链、batch12 的支付上游闭环、既有 monthly 配额与 per-image/阶梯换价计费——未触发新维度时计费金额必须**逐位不变**（用对照/快照测试证明）。
- 凭证 AES-GCM 不得改明文。
- OpenAPI 兼容：所有新增字段一律 optional / 带默认值，不得改既有响应结构的必填性或类型；若不得不调整既有结构须先问。
- 不得修改与本任务无关的 `*_test.go`；新维度配新测试，迁移涉及的旧测试只调 import 不删断言。
- ent schema 改动（PricingRule 增 long_context_threshold/multiplier/priority 价、entitlement 增 daily/weekly 配额键、UsageLog 增 5m/1h token 与成本分解列等）须 `go generate ./ent/...` 并同步 **store + mock**（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑：mock 方法签名需与接口一致）。
- 删枚举值 / 不可逆数据迁移 / 把存量 pricing_override 旧 key 规整——先问我（pricing_override 双 key 收敛属 batch15 rank63，本批不碰）。
- 触及 `runtime_gateway_core.go`（仅余约 44 行到 2200 行红线）时：优先把新增逻辑落到新的小文件 / 下沉到 `billing.Service` 方法，**绝不把该文件推过红线**；若无法避免增行，先按 batch15 rank59 思路做最小拆分腾余量，并在 commit message 注明。
- LiteLLM worker 与任何上游价表拉取一律用 httptest mock，不打真实远端。
</constraints>

<success_criteria>
完成需同时满足（每条给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）；前端 `npm run -w apps/web lint` 与 `tsc --noEmit` 通过（贴尾部）。
2. **daily/weekly 配额接通可证（rank12）**：新增测试证明 daily/weekly `hard_cap` 超额被拒、`allowance` 模式 overage 生效（数据源为 `MaterializedUsage.Daily/WeeklyUsageUSD`），贴出测试名 + `go test` 输出；chrome-devtools 截图证明订阅页 daily/weekly `UsageBar` 在有配额时**宽度非 0**（对比修复前恒 0%）。
3. **usage 聚合分解可证（rank30）**：测试断言 `UsageAggregate`/`APIKeyUsageSummary` 的 `inputCost/outputCost/cacheReadCost/cacheWriteCost` 之和等于 `TotalCost`（逐位），贴 `go test` 输出；chrome-devtools 截图证明 usage 汇总卡展示成本分解。
4. **quality_tier 处置可证（rank14）**：若选 (a) 接通——`grep -n QualityTier apps/api/internal/modules/scheduler/contract/contract.go` 命中，测试证明 quality 策略下 `Candidate.QualityTier` 取自 `resolution.Model.QualityTier`（非空且影响 score）；若选 (b) 下架——`grep` 证明 `zh.ts`/`en.ts` 的「调度器使用」文案已改为纯展示说明，贴改后片段。无「字段在但永不消费」残留。
5. **service_tier 倍率价可证（rank43）**：表驱动测试证明同一 token 量下 `service_tier=priority` 计费≈2x、`flex`≈0.5x、缺省=1x，贴输出。
6. **cache 5m/1h 分档可证（rank44）**：测试证明带 `ephemeral_1h` 明细按 1h 价、`ephemeral_5m` 按 5m 价、无明细回退全 5m，贴输出。
7. **长上下文倍率可证（rank45）**：测试证明超 `long_context_threshold` 且**无区间**时整次乘 multiplier、有区间时走阶梯不乘倍率（互斥），贴输出。
8. **图片 token 独立费率可证（rank47）**：测试证明 token 模式下 `image_output_tokens` 用独立价计、与普通 output token 价区分，贴输出。
9. **BillingModelSource 可证（rank46）**：测试证明 `billing_model_source=upstream` 时用 upstream 模型名解析定价规则、`requested` 时用请求模型名，命中不同 PricingRule，贴输出。
10. **LiteLLM 同步可证（rank48）**：httptest mock 价表源，测试证明 worker upsert family 级 PricingRule 且其优先级低于强制维度规则（强制规则仍优先命中），贴输出。
11. **回归可证（核心）**：等价性测试用一组覆盖 normal/cache/0 价/多币种且**不触发任一新维度**的样例，断言 `EstimatePrice` 结果与基准逐位相等；贴 `go test` 输出。
12. `go test ./...` 全绿（贴尾部）；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 **38 turns** 后停止并汇总：已完成维度、各维度测试红/绿、剩余阻塞项及原因。
</success_criteria>

<sequencing>
按依赖顺序、每维度自带测试并单独 commit；凡 schema/OpenAPI 改动先行（先改 `packages/openapi/openapi.yaml` + `apps/api/ent/schema/*` → `go generate` → 同步 store/mock → 再写 service/handler/前端）：
1. **阶段一（daily/weekly 配额，强制价值最高）**：entitlement 增 daily/weekly 配额键（OpenAPI + schema）→ `CheckEntitlement`/`CostAllowance` 同构判定 → 前端 UsageBar 传 limit → 配额测试 + UI 截图。
2. **阶段二（usage 聚合分解）**：`usageAccumulator` + `UsageAggregate`/`APIKeyUsageSummary` 增成本分解字段（OpenAPI）→ `aggregate()` sum → 前端汇总卡 → 分解=总和测试 + 截图。
3. **阶段三（quality_tier 二选一落定）**：(a) `Candidate.QualityTier` 灌入选路 + 兜底；或 (b) 改 `zh.ts`/`en.ts` 文案 → 对应测试 / grep。
4. **阶段四（service_tier 倍率价）**：`PricingRequest.ServiceTier` + PricingRule priority 档/倍率（OpenAPI + schema）→ `tokenPriceFromRule` 选价 → `gatewayPricingRequest` 透传 → 倍率测试。
5. **阶段五（cache 5m/1h 分档）**：anthropic usage 拆 ephemeral_5m/1h → Usage/UsageLog 增字段（schema）→ pricing 两档 cache-write 价 → 分档测试（含回退 5m）。
6. **阶段六（长上下文整次倍率）**：PricingRule 增 threshold+multiplier（schema）→ calc 仅无区间时乘 → 互斥测试。
7. **阶段七（图片输出 token 独立费率）**：image_output_tokens 拆分 + 独立价 → 测试。
8. **阶段八（BillingModelSource）**：mapping/channel 加开关 → `EstimatePrice` 选模型名 → 测试。
9. **阶段九（LiteLLM worker，最后）**：新增同步 worker + httptest mock + family 兜底优先级测试。
10. **阶段十（全量回归）**：跑等价性测试 + `go test ./...` + 前端 lint/tsc。
每阶段一个 commit，message 末尾加：
`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/billing-dimensions-batch13`）若干语义化 commit + 各计费维度的 schema/契约/计算/前端改动 + 每维度的针对性测试（service_tier/cache 分档/长上下文/图片 token/BillingModelSource/daily-weekly 配额/usage 聚合分解/LiteLLM 同步，全 passing）+ 一组「不触发新维度则逐位不变」的等价性回归测试 + `progress.txt` 收尾（列每个维度的最终处置、quality_tier 选 (a) 还是 (b)、各测试名与证据位置、未做的 out-of-scope 项及其归属批次）。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改成 `float64`；退回明文凭证；**伪造未实现能力**——「摆设」的两种合法处置只有「真接通到消费点（daily/weekly 配额真判定、usage 分解真汇总、quality_tier 真灌入）」或「诚实下架（删字段/改文案为纯展示）」，绝不假装接通；在测试里打真实上游 / 真实 LiteLLM 价表源（一律 httptest mock）。
先问我：破坏现有 OpenAPI 响应结构（改既有字段必填性/类型）；删现有顶层路由文件；删 ent 枚举值的存量迁移 / 任何不可逆数据迁移（含 pricing_override 旧 key 规整——那是 batch15）；向真实上游 / 真实 LiteLLM 端点发起需真凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 `progress.txt`（做了什么 / 证据在哪 / quality_tier 的二选一决定 / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 `progress.txt`，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 `success_criteria` 即停；若 38 turns 仍未达成，停下汇总：已完成维度、各维度测试红/绿、daily-weekly/usage-分解/quality_tier 三处摆设的接通 or 下架状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；已抽查标注「✓ 核实」）：
- daily/weekly 配额数据源：`apps/api/internal/modules/subscriptions/domain/usage.go:28-36`（✓ 核实 `ApplyUsageDelta` 累加 Daily/Weekly/MonthlyUsageUSD，`ResetExpiredUsage` 按窗重置）；`apps/api/internal/modules/subscriptions/service/service.go:361-417`（CheckEntitlement 现仅 monthly 分支）。
- daily/weekly 配额 UI 摆设点：`apps/web/src/components/features/subscription-usage-bars.tsx:24-26`（✓ 核实：daily/weekly `UsageBar` 不传 `limit`，仅 monthly 传 `limit={quota}`）。
- usage 聚合分解：`apps/api/internal/modules/usage/contract/contract.go:161-174`（✓ 核实 `UsageAggregate` 现仅 `TotalCost`，无成本分解字段）；`apps/api/internal/modules/usage/service/service.go:359-387`（✓ 核实 `usageAccumulator` 现仅 `totalCost *big.Rat`，`aggregate()` 仅 `money.FormatRatFixed(totalCost,8)`）；per-log 已暴露 `apps/api/internal/httpserver/runtime_api_mapping.go:554-557`。
- quality_tier：`apps/api/internal/modules/scheduler/contract/contract.go:63,67`（✓ 核实 `Candidate` 有 `ModelFamily` 无 `QualityTier`）；灌入点 `apps/api/internal/httpserver/runtime_gateway_core.go:217,1166`（只灌 ModelFamily）；误导文案 `apps/web/src/locales/zh.ts:802` / `en.ts:807`。
- 定价契约/计算：`apps/api/internal/modules/billing/contract/contract.go:110-122`（✓ 核实 `PricingRequest` 现有 InputTokens/OutputTokens/CacheRead/CacheWrite/ImageCount/ImageSize/PricingOverride，无 ServiceTier）；`apps/api/internal/modules/billing/service/pricing.go:340-437`（tokenPriceFromRule/calc）、`:452-456`（✓ 核实 `priceFromPayload` 双 key 别名——本批勿收敛，batch15 rank63）。
- cache 分档：`apps/api/internal/modules/provider_adapters/service/conversation_anthropic_usage.go:26-27`（✓ 核实仅 `CacheCreationInputTokens`/`CacheReadInputTokens` 两字段）。
- service_tier 透传：网关请求体 normalize 在 `apps/api/internal/modules/provider_adapters/service/codex_responses_payload.go:290`（仅 normalize 不计费）；`gatewayPricingRequest` 在 httpserver 编排路径。
- LiteLLM：现状纯本地 family 兜底 `selectFamilyPricingRule`（pricing.go）；新增 worker 落 `apps/api/internal/workers/`（镜像既有 retention/quota_refresh worker 结构）。
- OpenAPI 源：`packages/openapi/openapi.yaml`（所有新字段先改此处再生成）。
- 守门测试（触线参考）：`apps/api/internal/architecture/architecture_test.go`（maxRuntimeFileLines=2200，contract-only import）；`apps/api/internal/codequality/code_quality_test.go`（maxProductionFuncLines=210）。
sub2api 仅作能力对照不抄代码。本批相关 sub2api 参照点（仅借鉴能力/算法，不拷源码）：service_tier priority 2x / flex 0.5x 倍率；cache_creation 的 ephemeral_5m / ephemeral_1h 两档价；`applyLongCtx = len(intervals)==0` 的长上下文整次倍率与阶梯换价互斥规则；`ImageOutputPricePerToken` 把图片输出当 output token 子集独立计价；BillingModelSource 选「按 requested 还是 upstream 模型名定价」；LiteLLM 远程价表定时同步并以 family 级作兜底（优先级低于强制维度规则）。
</references>
