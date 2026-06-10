# Goal：架构与技术债清理 —— 拆巨文件 / 消重 / 删死代码 / 补测 / 性能与能力收尾（第十五批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查后，batch8-14 已把全部「摆设/缺失」业务能力清完，本批是 program 的**收尾架构清理批**。核查发现的技术债集中在五处：(1) 五个核心文件逼近 `maxRuntimeFileLines=2200` 红线，余量极小——`runtime_gateway_core.go`=2156（仅余 44 行，最高危）、`conversation_protocols.go`=2129、`provider_adapters/service.go`=1982、`runtime_api_mapping.go`=1896、`scheduler/service.go`=1885；任何后续功能落到这些文件即触线。(2) 两个巨函数逼近 `maxProductionFuncLines=210`（`parseAnthropicCompatibleStream`=206、`parseOpenAICompatibleStream`=201）。(3) 跨包重复实现（metadata 强制转换、quota 持久化、pricing 双 key 别名）——多路径口径漂移风险。(4) 死代码（subscriptions 四个无调用方函数）、命名误导（`codex_quota.go` 托管通用 helper）、跨持久化耦合。(5) 金额相关纯函数零测试 + 两项功能收尾（SOCKS5+uTLS 拨号器、availability rollup worker、通用导入 update-existing）。这是「架构完美」验收口径的最后一公里。
> 前置：建议 batch8-14 已落地（本批要拆/消重的文件多被前序功能批改过；尤其 rank64 的 promo 名额回滚若 batch8 已做，则此处只清耦合/命名，勿重复实现）。
> 关联：rank59 拆文件是其余功能批次的**隐性前提**（core 仅余 44 行，任何增行功能都会触线，理想做法是穿插——每个改 `runtime_gateway_core.go`/`service.go` 的功能批之前先拆）；本批是 batch2（money 公共包）、batch13（计费维度）地基工作的延续与收口；与 batch5-7（配额/认证真实性）的成果须全程回归保护，不得破坏。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个对标 sub2api 的 AI 网关/计费平台。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first：先改 `packages/openapi/openapi.yaml` 再生成；schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`，worker 在 `apps/api/internal/workers/`，共享纯函数在 `apps/api/internal/pkg/`）。前端 Next.js + TypeScript（apps/web，路由 `app/`，组件 `components/`，导航 `components/layout/nav-items.ts`）。
本地开发：`make dev-up` + `npm run dev`；登录 admin@srapi.local / Admin1234。所有改动须通过 `cd apps/api && go build ./... && go vet ./...`（本批前端改动极少，如有则 `cd apps/web && npm run lint && npx tsc --noEmit`）。
架构红线（由两个守门测试 + gofmt 强制，本批就是要让它们更绿、留更多余量）：
- `apps/api/internal/architecture/architecture_test.go`：跨模块只允许 import 目标模块的 `contract` 层（service/store/handler 间禁止直接耦合）；`contract` 层禁止 import Ent / 生成的 OpenAPI DTO / HTTP server 包；worker 只能 import 模块 contract/service；Ent/Redis store 只能 import 模块 contract；单文件硬上限 `maxRuntimeFileLines=2200`（runtime_* 文件），`runtime_http.go` 兼容层上限 120 行。
- `apps/api/internal/codequality/code_quality_test.go`：`maxProductionFuncLines=210`（单函数行数上限）。
- gofmt 必须全过。
凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
**本批主基调是「纯机械重构、零行为变更」**：拆文件/抽 helper/删死代码这部分，对外行为（API 响应、计费金额、调度决策、SSE 流帧）必须逐位/逐字节不变；只有 rank37（SOCKS5+uTLS）和 rank67/68（收尾）是新增能力，须配新测试。
</role_and_context>

<objective>
目标（可衡量最终态）：所有核心文件降到 <1800 行（给 2200 留 ≥400 余量）、所有生产函数 <150 行（给 210 留余量）、消除全部跨包重复 helper（抽到 `internal/pkg` 共享单实现）、删尽死代码、修正命名误导、补齐金额相关纯函数与 `SummarizeUserWindow`/allowance 的测试、修热路径性能债、补 SOCKS5+uTLS 拨号器与两项收尾；两个守门测试 + gofmt + `go vet` + `go test ./...` 全绿。完成后 `architecture_test` 与 `code_quality_test` 不仅通过，且核心文件/函数离红线有舒适余量，program 达成「架构完美」验收口径。
动机：技术债不是「摆设」（不涉及伪造业务能力），但它是 program 完整性的最后短板——核心文件仅余 44 行意味着下一个功能批必触线、跨包重复 helper 意味着两条计费/调度路径会静默漂移、死代码与命名误导会误导后续 agent、金额纯函数无测试意味着「日志 cost 之和」与「账本 amount 之和」对不上的回归无人拦截。本批把地基理顺、把红线腾出余量、把回归网补全，使前序所有功能成果可长期维护。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元，每项带真实文件锚点）：

**A. 拆巨文件腾余量（纯机械，保行为）**
1. **rank59 拆五个核心巨文件到 <1800**：按职责把文件拆成同包多文件（同 package、仅移动函数，不改签名/调用方）——
   - `apps/api/internal/httpserver/runtime_gateway_core.go`(2156→<1800)：拆出 admission（准入）/ quota-reserve（配额预留）/ rate-limit / pricing-请求构造 等子文件（如 `runtime_gateway_admission.go`、`runtime_gateway_pricing_request.go`）。
   - `apps/api/internal/modules/provider_adapters/service/conversation_protocols.go`(2129→<1800)：按协议族拆 `_openai` / `_anthropic` / `_gemini`。
   - `apps/api/internal/modules/provider_adapters/service/service.go`(1982→<1800)：按 `invoke{协议}Compatible` / `Normalize` 等职责拆。
   - `apps/api/internal/httpserver/runtime_api_mapping.go`(1896→<1800)：按映射域（账号/usage/capability 回显等）拆。
   - `apps/api/internal/modules/scheduler/service/service.go`(1885→<1800)：拆 `strategy_seeds` / `scoring` / `feedback` / `snapshot` / `metadata_coerce` 等子文件。
   - 拆出的文件须保持同 package（不引入新跨模块 import），且 diff 为纯移动（无逻辑改动）。
2. **rank60 两个巨函数抽小函数到 <150**：`apps/api/internal/modules/provider_adapters/service/conversation_protocols.go` 的 `parseAnthropicCompatibleStream`(206) 与 `parseOpenAICompatibleStream`(201)——把 SSE 帧分发（event 类型 switch 各分支）抽成小函数（如 `handleAnthropicStreamEvent`），降到 <150 留余量；SSE 输出字节流不得变。

**B. 消重与命名整理（纯机械，保行为）**
3. **rank61 metadata 强制转换 helper 抽共享**：`apps/api/internal/modules/scheduler/service/service.go:1409-1680`（`metadataBool:1409` / `metadataValue:1634` / `floatValue:1642` / `intValue:1663`）与 `apps/api/internal/httpserver/runtime_gateway_resolution.go:185-347`（`metadataBool:185` / `metadataValue:317`）两套近乎相同实现——抽到 `apps/api/internal/pkg/metacoerce`（或同类共享包），两处都引用，统一 json.Number / string-float 边界语义。注意：scheduler service 是模块、httpserver 是 HTTP 层，共享包须放 `internal/pkg`（不违反「跨模块只 import contract」红线，pkg 是公共基础设施）。
4. **rank62 quota 持久化 worker/handler 抽共享 helper**：`apps/api/internal/workers/quota_refresh/worker.go`（`RecordQuotaSnapshot` 调用 ~:270、`persistQuotaProviderError:294`）与 `apps/api/internal/httpserver/runtime_admin_quota_fetch_handlers.go` 的同名近乎相同实现——抽 contract/service helper（如 `QuotaSnapshotFromSignal` 与 `ApplyQuotaProviderError`），两调用方复用。helper 应落在两者都能合法 import 的层（accounts 或 provider_adapters 模块的 contract/service）。
5. **rank63 pricing_override 双 key 迁移 + 删旧别名**：`apps/api/internal/modules/billing/service/pricing.go:452-455` 每个价格字段双 key 兜底（`*_per_million_tokens` 与历史 `*_per_million`）——二选一并落定：(a) 写一次性迁移把存量 `mapping.pricing_override` 旧短 key 规整为 `_per_million_tokens` 后，删 `payloadMoney` 的第二别名参数；或 (b) 写入校验拒绝旧 key 并在文档说明。默认 (a)，但**删旧别名前必须确认无存量数据仍用旧 key**（先查 DB 或写迁移）；不可逆数据迁移先问我。
6. **rank64 死代码删除 + codex_quota 命名整理 + 跨持久化耦合（promo 回滚视 batch8 状态）**：
   - 删 `apps/api/internal/modules/subscriptions/service/service.go` 的四个无调用方函数 `mergeEntitlements:498` / `payloadMoney:685` / `payloadString:699` / `cloneTime:786`（删前 grep 确认零生产调用方；注意 billing 的 `payloadMoney` 是另一个同名函数，勿误删）。
   - `apps/api/internal/modules/provider_adapters/service/codex_quota.go` 实际托管 anthropic/quota_fetch 共享的通用 helper（`clampFloat:116` / `parseQuotaHeaderFields:137` / `parseHeaderFloat:92` / `parseHeaderInt:104` / `formatPercentQuotaValue` 等）——把这些**通用** helper 移到 `quota_common.go`（同包），`codex_quota.go` 只留 codex 专属（`codexQuotaSignalsFromHeaders` 等）。
   - 跨持久化耦合：`apps/api/internal/persistence/entstore/payments/store.go` 直调 admincontrol 包级 promo 函数——评估经 contract Store 接口注入降耦合（若改动面过大可文档化为已知脆点并加注释，优先低风险）。
   - promo `UsedCount` 名额回滚（下单即 +1 但过期/取消从不回滚）：**若 batch8 已做则此处略**（先 grep 确认 markPaidAndFulfill 占用 / Cancel·Expire 释放是否已存在）；未做则在 `Cancel/Expire` 时释放 `UsedCount`。

**C. 补测（回归网）**
7. **rank65 金额相关纯函数 + SummarizeUserWindow + allowance 补测**：
   - 为 `apps/api/internal/modules/api_keys/domain/cost_usage.go`（`ApplyCostUsage` / `ResetExpiredCostWindows`）与 `apps/api/internal/modules/subscriptions/domain/usage.go`（`ApplyUsageDelta` / `ResetExpiredUsage`）补表驱动纯函数测试（窗口跨界重置 / nil start / 负 cost / 多币种边界）——这两个 domain 目录现确认**无 `*_test.go`**。
   - 为 `apps/api/internal/persistence/entstore/usage` 的 `SummarizeUserWindow`（allowance 判定数据源）补 store 集成测试（SuccessOnly / 时间边界 / ProviderID 过滤）。
   - 补 allowance 模式 e2e（`used_cost` 读物化 monthly 值）。

**D. 性能债（保行为，可观测改善）**
8. **rank66 EstimatePrice / usage 全表扫性能优化**：`apps/api/internal/modules/billing/service/pricing.go:140-153`（`EstimatePrice` 每请求全量 `ListPricingRules` 后内存线性筛选）+ `:262-306`（select）；`apps/api/internal/httpserver/runtime_gateway_usage.go`（`recordGatewayAccountSnapshots` 每次 `usage.List()` 全表拉日志、`gatewayAccountRateMultiplier` 逐 group 串行 `FindGroupByID`）——pricing 改 store 侧 `(model_id,provider_id)` 谓词查询或进程内缓存（规则变更失效）；usage 快照按 `account_id`+时间窗谓词；rate multiplier 批量取 group。结果须与优化前逐位一致（对比测试佐证）。

**E. 能力收尾（新增能力，配新测试）**
9. **rank37 SOCKS5 + uTLS 组合拨号器**：`apps/api/internal/modules/reverse_proxy/service/egress_profile.go:129` 现对「非 http 代理 + TLSTemplate」硬拒（`unsupportedEgressProfile`）——引入 `golang.org/x/net/proxy`（新增依赖须说明理由），新增 `dialUTLSHTTP1ViaSOCKS5`（SOCKS5 Dialer 建 conn → `performUTLSHTTP1Handshake`），放宽代理 scheme 白名单到 `http`/`socks5`/`socks5h`。用 httptest/本地 mock SOCKS5 验证，不打真实上游。
10. **rank49 自定义 ClientHello / HTTP2 指纹边界文档化**：当前仅支持 utls 预设 HelloID，逐字段 `ClientHelloSpec`（cipher/curve/extension 顺序/GREASE）与 HTTP/2(Akamai) 指纹未做（`require_h2`/`http2_template` 硬拒）。**本批不实现完整自定义指纹**（工作量大、预设已覆盖主流反爬），而是在 docs 矩阵中**明确标注「当前只支持预设 ClientHello，HTTP/2 指纹与逐字段 ClientHelloSpec 为已知能力边界」**，使其成为有据可查的边界而非隐藏摆设。
11. **rank50 网关级 web search 设计取舍文档化或抽共享**：网关对 `web_search` 仅透传 provider 原生 hosted 能力（`apps/api/internal/modules/gateway/hosted_web_search.go`），Tavily/Brave 只服务 admin Copilot——这是有意 passthrough 设计取舍。**本批默认文档化为产品决策**（在 docs 写明「无 hosted-search 的上游不自履约 web_search，属设计取舍」）；若评估后决定接通，则把 `copilot/web_search.go` 的 `SearchFunc` 提到共享 pkg 供 gateway 兜底（属可选 stretch，须先问产品意图）。
12. **rank42 账号调度状态字段索引化（文档化推迟到规模阈值）**：`rate_limited_at`/`overload_until`/`schedulable`/`expires_at` 现塞 `metadata_json` 非类型化列，调度热路径 `List()` 全表后 Go 过滤——当前规模可接受。**本批不做 schema 迁移**，而是在 docs 写明「当账号数超过 N（建议 5k）时应把热调度状态键提升为类型化索引列」，并加 TODO 注释指向 `provideraccount.go`，使其成为有据可查的扩展性决策而非永久搁置（program 零 deferral 的合法形态：明确规模触发条件 + 落点）。
13. **rank67 availability rollup worker**：`apps/api/internal/httpserver/runtime_admin_availability_handlers.go:56-66` 现仅在 admin 打开 availability 视图时惰性计算 `AccountAvailabilityRollup`，从未被查看的账号永不物化——加后台 rollup/retention worker（镜像现有 retention worker 模式）使 rollup 表权威。
14. **rank68 通用账号导入 update-existing**：`apps/api/internal/httpserver/runtime_admin_account_import.go` 通用 import 路径 `updatedIDs`/`UpdatedCount` 恒 0（只创建从不更新已存在）——二选一并落定：(a) 实现 update-existing（对齐 codex importer 的 `UpdateExisting`）；或 (b) 移除该端点的 `UpdatedCount`/`UpdatedIds` 惰性响应字段（避免假装支持）。默认 (a) 若改动可控，否则 (b) 诚实下架字段。

明确不做（out-of-scope，均由其它批次涵盖或本批刻意以「文档化边界」处置，非永久搁置）：
- 不重复实现 batch8 已做的 promo 名额回滚（rank64 该子项视 batch8 状态决定）。
- 不实现完整自定义 ClientHello/HTTP2 指纹（rank49）——本批以「文档化能力边界」处置；若产品后续要做属独立深度批次。
- 不实现网关级 web search 自履约（rank50）——本批以「文档化设计取舍」处置；接通属产品决策。
- 不做账号调度状态字段的 schema 迁移（rank42）——本批以「文档化规模触发条件 + TODO 落点」处置；真迁移待账号规模到阈值。
- 不新增任何业务能力或计费维度（全在 batch8-14 已覆盖）；不改 scheduler/计费的对外决策结果。

> 注：rank49/50/42 三项的「文档化处置」是 program 零 deferral 的合法形态——它们不是被偷偷放弃，而是被**显式记录为有据可查的能力边界/设计取舍/规模触发条件**，验收时可在 docs 矩阵看到明确标注，下一手 agent 不会误以为是摆设或遗漏。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64；保留 0 价回退防漏收语义。**拆文件/抽 helper/性能优化前后，定价金额结果、usage 聚合结果、调度决策、SSE 流帧必须逐位/逐字节一致**（用对比/快照测试证明）。
- 不破坏已落地的 batch1-14 能力：支付下单→回调→入账→退款扣回主链、promo/redeem/订阅权益强制、payload 转换、error-passthrough、TLS 指纹 uTLS 出站、会话粘度、故障转移+冷却+调度反馈、分组费率倍率、Anthropic/Codex 真实配额+告警、账号认证矩阵真实性（batch7 CI 守门）、batch8 affiliate 闭环、batch9 RBAC+风控+内容安全、batch10 scheduler CRUD、batch11 账号域+Antigravity+channel-monitor worker、batch12 支付上游真实性、batch13 计费维度、batch14 端用户登录+平台自助——这些的现有测试必须仍全绿。
- 凭证 AES-GCM 加密不得改明文。
- OpenAPI 兼容：本批基本不动对外契约（如 rank68 选「下架字段」需删响应字段，属破坏响应结构，先问我）。
- 不得修改与本任务无关的 `*_test.go`；迁移后原测试应仍能覆盖（必要时只调 import 路径，不得删断言）。
- ent schema 若改（rank42 本批不改；其它项原则上不改 schema），如确需改：`go generate ./ent/...` 后同步 store/mock（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑）；删枚举值 / 不可逆迁移先问我。
- 抽共享 helper 必须遵守架构红线：共享纯函数放 `internal/pkg`（不引入「跨模块 import 非 contract」违规）；不得让 httpserver/worker 直接 import 别模块的 service/store。
其他约束：新增依赖（rank37 的 `golang.org/x/net/proxy`）须先说明理由。
</constraints>

<success_criteria>
完成需同时满足（每条在输出里给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）。
2. **架构红线腾余量可证（核心）**：`go test ./internal/architecture/... ./internal/codequality/...` 全绿（贴测试名 + 输出）；并贴出 `wc -l` 证明五个核心文件（runtime_gateway_core.go / conversation_protocols.go / provider_adapters/service.go / runtime_api_mapping.go / scheduler/service.go）均 <1800；贴出证据证明 `parseAnthropicCompatibleStream` / `parseOpenAICompatibleStream` 现 <150 行。
3. **消重单实现可证**：`grep -rn` 证明 metadata 强制转换 helper（`floatValue`/`intValue`/`metadataValue`）的「定义」只剩共享 `internal/pkg` 一处（scheduler service 与 runtime_gateway_resolution.go 不再各有定义）；quota 持久化 helper 同理收敛为单实现；贴出 grep 结果（定义点计数）。
4. **死代码/命名/旧别名可证**：`grep -rn` 证明 `mergeEntitlements`/`payloadString`/`cloneTime` 及 subscriptions 的 `payloadMoney` 已删（生产码零定义/零调用）；`codex_quota.go` 不再托管通用 helper（已移 `quota_common.go`，贴文件清单）；pricing 旧 key 别名已删或写入校验生效（贴 `pricing.go:452-455` 改后片段 + 迁移/校验说明）。
5. **行为不变可证（纯机械部分）**：贴出对比/快照测试证明拆文件 + 抽 helper + 性能优化前后，`EstimatePrice`（覆盖正常/缓存/0价/多币种）、usage 聚合、SSE 流帧逐位/逐字节一致；贴 `go test` 通过输出。
6. **补测可证**：新增 `cost_usage`/`usage` domain 纯函数测试（窗口跨界/nil start/负 cost）+ `SummarizeUserWindow` store 测试 + allowance e2e 全通过；贴出 `go test ./internal/modules/api_keys/domain/... ./internal/modules/subscriptions/domain/... ./internal/persistence/entstore/usage/...` 输出（不再 `[no test files]`）。
7. **性能优化可证**：贴出 `EstimatePrice` 改为谓词/缓存查询、usage 快照按 account_id+时间窗谓词、rate multiplier 批量取 group 的 diff 片段 + 一条说明（如基准测试或调用计数对比）；结果等价测试见第 5 条。
8. **SOCKS5+uTLS 可证（rank37）**：贴出 `dialUTLSHTTP1ViaSOCKS5` 新代码 + 用本地 mock SOCKS5 的测试证明 uTLS ClientHello 经 SOCKS5 隧道发出成功；贴 `egress_profile.go:129` 放宽白名单后片段；`golang.org/x/net/proxy` 新依赖理由。
9. **收尾可证**：rank67 availability rollup worker 新代码 + 测试证明从未被查看的账号也被物化；rank68 update-existing 实现（`UpdatedCount`>0 测试）或字段诚实下架（grep 证明字段已删）；贴证据。
10. **边界文档可证（rank49/50/42）**：`docs/` 下产出/更新能力边界矩阵，明确标注「ClientHello 仅预设 / HTTP2 指纹未做（rank49）」「网关 web_search 仅 passthrough 设计取舍（rank50）」「账号调度状态字段索引化的规模触发条件 N + 落点（rank42）」；贴 docs 路径 + 相关行；代码侧 rank42 的 TODO 注释指向 `provideraccount.go`。
11. `go test ./...` 全绿（贴尾部）；gofmt 无 diff（`gofmt -l` 空）；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 40 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试（或等价性证明）并单独 commit；message 末尾加 Co-Authored-By 行：
1. 阶段一（拆文件腾余量，纯机械）：rank59 逐文件拆（先拆 core，余量最紧）→ rank60 两个巨函数抽小函数。每拆一个文件跑一次 `go build ./...` + 相关现有测试确认零回归；diff 应为纯移动。本阶段每个核心文件一个 commit。
2. 阶段二（消重命名，纯机械）：rank61 metadata helper 抽 `internal/pkg` → rank62 quota helper 抽共享 → rank63 pricing 旧 key 迁移/删别名（迁移先确认存量数据）→ rank64 删死代码 + codex_quota 命名整理 + 跨持久化耦合（promo 回滚视 batch8 状态）。每子项独立 commit + 等价性测试。
3. 阶段三（补测）：rank65 cost_usage/usage domain 纯函数测试 + SummarizeUserWindow store 测试 + allowance e2e。一个 commit（或按包拆）。
4. 阶段四（性能债）：rank66 EstimatePrice/usage/rate-multiplier 谓词/缓存优化 + 等价性测试。一个 commit。
5. 阶段五（能力收尾）：rank37 SOCKS5+uTLS 拨号器（新依赖 + 测试）→ rank67 availability rollup worker → rank68 update-existing。每项独立 commit。
6. 阶段六（边界文档）：rank49/50/42 文档化能力边界/设计取舍/规模触发条件 + rank42 代码 TODO 注释 → 收尾矩阵文档。一个 commit。
拆文件（阶段一）须在任何会增行的改动之前完成（core 仅余 44 行）；若与其它功能批穿插，则每批改 core/service.go 前先拆。
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch15`）若干语义化 commit + 拆分后的同包多文件（diff 纯机械）+ `internal/pkg` 共享 helper（metadata 强制转换 / quota 持久化）+ `quota_common.go` + 删死代码后的 subscriptions service + pricing 旧 key 迁移 + 新增 domain/store/allowance 测试（全 passing）+ 性能优化后的 pricing/usage 查询 + `dialUTLSHTTP1ViaSOCKS5` + availability rollup worker + update-existing（或下架字段）+ docs 能力边界矩阵 + progress.txt 收尾（列每个 rank 的最终处置、证据位置、文档化边界三项的明确决策、未做的 stretch）。
</artifact>

<guardrails>
绝不：删除/改写既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证；在测试里打真实上游（SOCKS5/uTLS 用本地 mock）；**伪造未实现的能力**——本批的 rank49/50/42 三项只能以「诚实文档化能力边界/设计取舍/规模触发条件」处置，绝不假装已实现完整指纹/web-search/索引化；rank68 若不实现 update-existing 必须诚实下架 `UpdatedCount`/`UpdatedIds` 字段，绝不留恒 0 的假字段；拆文件/抽 helper 绝不顺手改行为（必须有等价性证明）。
先问我：破坏现有 OpenAPI 响应结构（如 rank68 删响应字段）；删除现有顶层路由文件；删 ent 枚举值的存量迁移；任何不可逆数据迁移（如 rank63 pricing 旧 key 一次性迁移前确认存量）；向真实上游发起需真凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 证据在哪 / 行数与红线余量 / 下一步 / 文档化三项的决策）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态、`wc -l` 五个核心文件看当前余量，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 40 turns 仍未达成，停下汇总：已完成阶段、五个核心文件当前行数 vs 1800、两个守门测试红/绿、补测覆盖情况、rank37/67/68 能力收尾状态、rank49/50/42 文档化决策、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；行数为本批审计时实测，拆改后会变）：
- 守门测试：apps/api/internal/architecture/architecture_test.go:15（maxRuntimeFileLines=2200）、:85-141（跨模块/contract import 规则）；apps/api/internal/codequality/code_quality_test.go:21（maxProductionFuncLines=210）/:268。
- 拆文件（rank59，实测行数）：apps/api/internal/httpserver/runtime_gateway_core.go(2156)；apps/api/internal/modules/provider_adapters/service/conversation_protocols.go(2129)；apps/api/internal/modules/provider_adapters/service/service.go(1982)；apps/api/internal/httpserver/runtime_api_mapping.go(1896)；apps/api/internal/modules/scheduler/service/service.go(1885)。（旁证逼近红线：admin_control/service.go=1833、gateway/service.go=1792，本批不强制拆但勿增行。）
- 巨函数（rank60）：apps/api/internal/modules/provider_adapters/service/conversation_protocols.go 的 parseAnthropicCompatibleStream(206)、parseOpenAICompatibleStream(201)。
- metadata helper 重复（rank61）：apps/api/internal/modules/scheduler/service/service.go:1409（metadataBool）/:1634（metadataValue）/:1642（floatValue）/:1663（intValue）；apps/api/internal/httpserver/runtime_gateway_resolution.go:185（metadataBool）/:317（metadataValue）。
- quota 持久化重复（rank62）：apps/api/internal/workers/quota_refresh/worker.go（RecordQuotaSnapshot ~:270、persistQuotaProviderError:294）；apps/api/internal/httpserver/runtime_admin_quota_fetch_handlers.go（同名 snapshot 构建/provider-error 持久化）。
- pricing 双 key（rank63）：apps/api/internal/modules/billing/service/pricing.go:452-455（input/output/cache_read/cache_write 各 `*_per_million_tokens` 与 `*_per_million` 双 key）。
- 死代码 + 命名（rank64）：apps/api/internal/modules/subscriptions/service/service.go:498(mergeEntitlements)/:685(payloadMoney)/:699(payloadString)/:786(cloneTime)；apps/api/internal/modules/provider_adapters/service/codex_quota.go:92(parseHeaderFloat)/:104(parseHeaderInt)/:116(clampFloat)/:126(formatPercentQuotaValue)/:137(parseQuotaHeaderFields)（通用 helper 待移 quota_common.go）；apps/api/internal/persistence/entstore/payments/store.go（直调 admincontrol 包级 promo 函数）；apps/api/internal/persistence/entstore/admincontrol/promo.go:102（UsedCount 占用无释放，回滚视 batch8 状态）。
- 补测（rank65）：apps/api/internal/modules/api_keys/domain/cost_usage.go（确认无 *_test.go）；apps/api/internal/modules/subscriptions/domain/usage.go（确认无 *_test.go）；apps/api/internal/persistence/entstore/usage/store.go（SummarizeUserWindow）。
- 性能债（rank66）：apps/api/internal/modules/billing/service/pricing.go:140-153（EstimatePrice 全量 List）/:262-306（select）；apps/api/internal/httpserver/runtime_gateway_usage.go（recordGatewayAccountSnapshots usage.List() 全表、gatewayAccountRateMultiplier 串行 FindGroupByID）。
- SOCKS5+uTLS（rank37）：apps/api/internal/modules/reverse_proxy/service/egress_profile.go:129（非 http 代理+TLSTemplate→unsupportedEgressProfile）；新增 golang.org/x/net/proxy。
- 指纹边界（rank49）：apps/api/internal/modules/reverse_proxy/service/egress_profile.go（clientHelloIDForTLSTemplate 仅枚举预设、rejectUnsupportedEgressFields、validateHTTPVersionPolicy require_h2 拒）。
- web search（rank50）：apps/api/internal/modules/gateway/hosted_web_search.go（仅 passthrough）；apps/api/internal/modules/copilot/web_search.go（仅 copilot 消费）。
- 调度状态索引（rank42）：apps/api/ent/schema/provideraccount.go（无调度状态列）；apps/api/internal/httpserver/runtime_gateway_core.go（List() 全表过滤）。
- 收尾：apps/api/internal/httpserver/runtime_admin_availability_handlers.go:56-66（rollup 惰性，rank67）；apps/api/internal/httpserver/runtime_admin_account_import.go（updatedIDs 恒 0，rank68）。
- 共享 helper 落点：apps/api/internal/pkg/（metadata 强制转换 / quota helper 须放此处以不违反架构红线；参照 batch2 的 internal/pkg/money 先例）。
sub2api 仅作能力对照不抄代码：本批以技术债清理为主，无直接 sub2api 功能参照；rank37 SOCKS5+uTLS 组合拨号可对照 sub2api 的代理+指纹正交拨号思路（仅借鉴「SOCKS5 Dialer 建 conn 后在其上跑自定义 ClientHello」的架构，不抄实现）；rank49/50 的能力边界对照 sub2api 的完整 ClientHelloSpec / 服务端 web search 执行器，用于在 docs 矩阵中标注差距而非实现。
</references>
