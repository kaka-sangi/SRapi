# Goal：计费域地基重构（money 公共包 + 定价归位）（第二批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：第一批之后的纯重构批次，为后续计费维度深化（第三批）打地基。覆盖 ranked backlog rank 16 + rank 8。无新业务能力，风险低，先把金额运算与定价归属理顺。
> 前置：建议第一批已落地（rate_multiplier/actual_cost 已进 schema）。

---

<role_and_context>
你是 SRapi 仓库的资深后端工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first，schema 在 apps/api/ent/schema/，业务在 apps/api/internal/modules/<域>/{contract,service,store}，HTTP 在 apps/api/internal/httpserver/）。
本地开发：make dev-up；登录 admin@srapi.local / Admin1234。所有改动须通过 cd apps/api && go build ./... && go vet ./...。
本批是纯重构：对外行为（计费金额结果、API 响应）不得改变，只改内部结构与归属。绝不抄 sub2api 源码。
</role_and_context>

<objective>
目标（可衡量最终态）：(a) 把散落在 5 处的金额定点运算收敛为单一 internal/pkg/money 公共包，所有计费代码引用它，消除口径漂移隐患；(b) 把 PricingRule 的定价契约/存储/选择/计算从 subscriptions 模块抽到 billing 域，使「定价→billable 拆分→扣费→账本」在领域上连续。完成后定价金额结果与重构前逐位一致（有对比测试佐证）。
动机：定价逻辑现在夹在 subscriptions 模块、金额函数 5 处复制，读懂一次请求计费要跨 5 个包，且多份格式化可能让「日志 cost 之和」与「账本 amount 之和」对不上。这是后续所有计费深化的地基，先理顺再加维度。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元）：
1. money 公共包：新建 internal/pkg/money，提供 decimalRat / addMoney / formatRatFixed(8位) / normalizeCurrency / parseMoneyRat + 常量 DefaultCurrency("USD") / ZeroAmount("0.00000000")；统一四舍五入规则（确认 FloatString 的 round-half-away 语义并文档化）。
2. 替换私有副本：删除 usage / subscriptions / billing-store / httpserver(addDecimalMoney) / balance_charger 五处的本地 decimalRat/formatRatFixed/addMoney/normalizeCurrency/cloneMap，改引用 money 包。
3. 定价归位：把 PricingRule 的 contract + store + select + compute（CreatePricingRule/ListPricingRules/selectPricingRule/priceFromRule/EstimatePrice）从 subscriptions 抽到 billing 模块（或 billing 下新建 pricing 子包）；subscriptions 仅保留额度/权益语义并消费 EstimatePrice。
4. 编排下沉：把 recordGatewayUsage 里 gatewayBillableCost / gatewayPricing 的编排下沉为 billing.Service 的方法（如 billing.PriceAndSplit），httpserver 改调一个入口。

明确不做（out-of-scope）：
- 不新增任何计费维度（billing_mode/区间/service_tier/cache 分档全留第三批）。
- 不改变 PricingRule 字段、effective 时间窗与 provider_id=0 通配优先级。
- 不改 OpenAPI 对外响应结构（定价管理端点路径可保持不变，仅内部实现迁移）。
- 不动 scheduler、不动 entstore 事务机制、不顺手清理无关代码。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64；保留 cacheWriteRateOrInput 的 0 价回退防漏收语义。
- 重构前后定价金额结果必须逐位一致（用对比/快照测试证明）。
- 不得修改与本任务无关的 *_test.go；迁移后原测试应仍能覆盖（必要时调整 import 路径，不得删断言）。
- ent schema 若有 Default('USD')/Default('0.00000000') 改动，须 go generate ./ent/... 并同步 store/mock。
其他约束：新增依赖须先说明理由。
</constraints>

<success_criteria>
完成需同时满足（每条在输出里给出可观察证据）：
1. cd apps/api && go build ./... && go vet ./...，退出码 0（贴出尾部）。
2. grep 证明：仓库内 decimalRat/formatRatFixed/addMoney 的「定义」只剩 internal/pkg/money 一处（贴出 grep -rn 结果，定义点 = 1）。
3. 等价性测试：新增测试用一组覆盖正常/缓存/0价/多币种的样例，断言迁移后 EstimatePrice 结果与基准值逐位相等；贴出 go test 通过输出。
4. 定价归位可证：PricingRule 的 EstimatePrice/selectPricingRule/priceFromRule 现位于 billing 模块；subscriptions 不再 import 定价计算实现（贴出新文件路径 + grep 证明 subscriptions 无 priceFromRule 定义）。
5. go test ./... 全绿（贴出尾部）。
6. git status 干净（除预期改动），git diff --stat 贴出。
或 25 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit：
1. 阶段一：新建 internal/pkg/money + 单测（含 round-half 边界、负数、8 位格式化）。
2. 阶段二：逐包替换私有副本（usage→subscriptions→billing-store→httpserver→balance_charger），每替换一包跑一次 go build 确认无回归。
3. 阶段三：把定价 contract/store/select/compute 迁到 billing；调整 import；保证旧测试通过。
4. 阶段四：httpserver 编排改调单一 billing 入口；跑全量 go test。
每阶段一个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + 新 internal/pkg/money 包 + 迁移后的 billing 定价代码 + 等价性测试 + progress.txt 收尾（列已完成阶段、证据位置、未做项）。
</artifact>

<guardrails>
绝不：删除/改写既有测试让其通过；hardcode 让测试通过；--no-verify 跳校验或 git push --force；把金额改回 float；为重构方便改变对外金额结果。
先问我：任何破坏现有 OpenAPI 响应结构、删除现有顶层路由文件、或不可逆的数据迁移。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么/证据在哪/下一步）+ git commit。
新 context 开始：先 pwd、git log --oneline -10、读 progress.txt，再 go build ./... 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 25 turns 仍未达成，停下汇总：已完成阶段、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
- 金额函数 5 处副本：apps/api/internal/modules/subscriptions/service/service.go:776-822（defaultMoney/normalizeCurrency/decimalRat/formatRatFixed）；apps/api/internal/modules/billing/service/service.go:161-166；apps/api/internal/persistence/entstore/billing/store.go；apps/api/internal/httpserver（addDecimalMoney）；apps/api/internal/workers/balance_charger。
- 定价待迁移：apps/api/internal/modules/subscriptions/service/service.go:295-730（CreatePricingRule / ListPricingRules / selectPricingRule:654 / priceFromRule:695 / EstimatePrice:515）。
- billing 目标域：apps/api/internal/modules/billing；扣费 apps/api/internal/persistence/entstore/billing/store.go（ChargeUsage:103）。
- httpserver 编排：apps/api/internal/httpserver/runtime_gateway_core.go（gatewayPricing:554）；runtime_gateway_usage.go（gatewayBillableCost / recordGatewayUsage）。
- schema 默认值：apps/api/ent/schema/pricingrule.go:25；usagelog.go:43（Default 'USD'）。
sub2api 仅作能力对照不抄代码：BillingService 单一计费域的内聚结构。
</references>
