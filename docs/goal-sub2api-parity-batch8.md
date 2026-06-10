# Goal：分销与促销资金链闭环 —— 消灭最大整片摆设（第八批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查发现，SRapi 整片最严重的摆设是 **affiliate 邀请闭环**——`CreateInviteCode` / `BindInvite` / `CreateRule` 三个写入服务方法只有测试调用方，没有任何 HTTP 端点、worker 或前端入口。用户永远拿不到邀请码、无法绑定推荐关系；无 relationship 则 `AccrueRebate` 恒命中 `no_invite_relationship`；`AffiliateRule` 全字段（rate/fixed/max/有效期）在计算逻辑被消费但无落库入口，`GetEffectiveRule` 恒 `no_effective_rule`。返佣引擎虽已通过 outbox 接通（`PaymentOrderPaid`→`AccrueRebate`）却被从源头饿死。同域还有两个伴生摆设：affiliate ledger 的 `settle`/`withdraw`/`manual_adjustment` 三类型在余额汇总/admin transfer 过滤/DTO 被消费却无任何生产者（恒 0），以及 promo_code 只有全局 `MaxUses` 而缺 `per_user_limit`/`min_order_amount`、且 `UsedCount` 下单即 +1 但订单过期/取消从不回滚（名额泄漏）。这是「业务能力假象」型摆设：返佣/促销在 UI 与 DTO 上俨然成型，实际从源头跑不通，是资金链上最高价值的可见性与正确性漏洞。
> 前置：无强依赖；与 batch9（RBAC）独立，本批可先行。
> 关联：是 batch8-15 program 的首批；返佣/促销资金链与 batch12（支付上游真实性）同属 commerce 域但解耦，promo `UsedCount` 回滚在本批落地后 batch15（架构债 rank64）只需清理跨持久化耦合与命名，不再重做回滚。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api，管理面比 sub2api 更宽）。
技术栈与约定：
- 后端 Go + ent ORM，**OpenAPI-first**：先改 `packages/openapi/openapi.yaml` 再生成。schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`，worker 在 `apps/api/internal/workers/`，持久化实现在 `apps/api/internal/persistence/entstore/<域>/`。
- 前端 Next.js + TypeScript（`apps/web`）：路由在 `src/app/`，组件在 `src/components/`，导航在 `src/components/layout/nav-items.ts`，admin/用户 API 客户端在 `src/lib/`。
- 本地开发：`make dev-up` + 前端 `npm run dev`（不要设 `NEXT_PUBLIC_SRAPI_BASE_URL`，CSP 会拦跨源）；登录 admin@srapi.local / Admin1234。
- 所有改动须过 `cd apps/api && go build ./... && go vet ./...` + 前端 `npm run -w apps/web tsc` 与 lint。
- 架构红线（`apps/api/internal/architecture/architecture_test.go` + `internal/codequality/code_quality_test.go`）：模块生产码只能 import 别模块的 **contract** 层（白名单），contract 层禁 import ent/生成 DTO/httpserver；单文件 ≤ 2200 行（注意 `runtime_gateway_core.go` 仅余 ~44 行余量，本批新增 HTTP handler 请落到 affiliate/promo 专属 runtime_* 文件而非塞核心文件）、单函数 ≤ 210 行；gofmt 必过。
- 凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
本批主题是「资金链闭环」：每一处摆设的合法处置只有两种——**真接通**（接上消费点，端到端可验）或**诚实下架**（删字段/类型/DTO，不留假象）——绝不假装接通。金额一律 string + big.Rat 8 位定点，不得引入 float。
</role_and_context>

<objective>
目标（可衡量最终态，零 deferral）：用户能在前端生成并复制邀请码/邀请链接；新用户注册可携带 `invite_code` 并真实建立推荐关系；管理员能在 admin 创建/列出/编辑 `affiliate_rule`（rate/fixed/max/有效期/触发类型/币种）；一笔被推荐用户的付款经 outbox 触发 `AccrueRebate` 后返回 `Applied:true` 并写入一条 `accrue` ledger（端到端 e2e 可证，不再恒命中 `no_invite_relationship`/`no_effective_rule`）。同时：affiliate ledger 的 `withdraw` 有真实写入路径（用户提现申请 + admin 审批写 `withdraw` 账目），未实现的 `settle`/`manual_adjustment` 二选一——本批至少接通其一或诚实从 contract/summary/DTO 删除；promo_code 支持 `per_user_limit`/`min_order_amount` 并在 preview/finalize 按 user 维度计数与最低额校验；promo `UsedCount` 改为「付款占用、过期/取消释放」，不再泄漏名额。
动机：这是 SRapi 最大的「业务能力假象」摆设——返佣/促销在 DTO 与 UI 上看似可用，实际从源头无法启动，运营以为有分销获客能力却一个邀请关系都建不出，且 promo 名额可被未支付订单永久占走（资金/促销正确性漏洞）。修这块对业务面完整度的边际价值最高。
</objective>

<scope>
本次包含（按 sequencing 推进，每个子目标带真实文件锚点；摆设类目标必须写明「接通的消费点」或「下架范围」）：

1. **rank1 — affiliate 邀请闭环（接通）**：
   - 新增 `POST /api/v1/me/affiliate/invite-codes`（当前用户生成邀请码，调 `affiliate/service/service.go:56` `CreateInviteCode`）+ `GET /api/v1/me/affiliate/invite-codes`（列出本人邀请码）。
   - `GET /api/v1/me/affiliate`（`server.go:698` 已有）扩展为返回邀请码/可复制邀请链接/已邀请人数（消费点：前端 `apps/web/src/app/affiliate/page.tsx` 新增邀请区，展示码+链接+复制按钮+邀请计数）。
   - 注册流程接受可选 `invite_code` 参数，注册成功后调 `BindInvite`（`service.go:72`）建立推荐关系（消费点：用户注册 handler；新用户绑定后 outbox 链路 `apps/api/internal/workers/outbox/domain_handler.go`（AccrueRebate/CompensateRefund 已接）即可在其付款时返佣）。
2. **rank3 的 affiliate 规则 CRUD 部分（接通；scheduler 策略 CRUD 本身属 batch10，本批只做 affiliate-rules）**：
   - 先改 OpenAPI 再生成：新增 admin `POST /api/v1/admin/affiliate-rules`、`GET /api/v1/admin/affiliate-rules`、`PATCH /api/v1/admin/affiliate-rules/{id}`，service 侧调 `CreateRule`（`service.go:99`）并补 List/Update；规则字段（rate/fixed/max/有效期/trigger_type/currency）必须可落库，使 `GetEffectiveRule`（`service.go:176`）能命中 active 规则。
   - 消费点：前端新增 `apps/web/src/app/admin/affiliates/rules/`（与现有 `invites`/`rebates`/`transfers` 同级）规则管理页 + admin API 客户端方法。
3. **rank15 — affiliate ledger settle/withdraw/manual_adjustment（接通其一 + 诚实下架其余）**：
   - **withdraw（必做接通）**：新增用户提现申请端点 + admin 审批端点，审批通过时 `AppendLedger`（memory store `apps/api/internal/modules/affiliate/store/memory/memory.go:197`，及 entstore 对应实现）写入 `LedgerTypeWithdraw`（`contract/contract.go:50`），使 `WithdrawnAmount` 不再恒 0；余额汇总（`service.go:578` 起的 ledger type switch）正确扣减。
   - **settle / manual_adjustment**：二选一——若本批接通 admin manual-adjustment 端点（写 `LedgerTypeManualAdjustment`，`contract/contract.go`）则接通；确不接通的类型从 `contract`/summary 计算/OpenAPI DTO 中删除（不得保留恒 0 的假类型）。在 progress.txt 写明每个类型的最终处置。
4. **rank17 — promo per_user_limit / min_order_amount（接通）**：
   - `admin_control/contract.go` 的 `PromoCode` 增 `per_user_limit`/`min_order_amount`（当前仅 `MaxUses`，`contract.go:397/410`）；OpenAPI 先改后生成；ent schema 若需新增列则 `go generate` 并同步 store/mock。
   - `previewPromoCode`（`promo.go:200`）与 finalize 路径按 `user_id` 查 `UserPromoCodeApplication`（`promo.go:53/76/94/124` 已有该实体查询）计当前用户已用次数与最低订单额校验；消费点：前端 `apps/web/src/app/admin/promo-codes/page.tsx` 表单暴露两个新字段。
5. **rank64（部分）— promo UsedCount 名额回滚（接通正确性）**：
   - 当前 `promo.go:102` 下单即 `UsedCount++`，过期/取消从不回滚。改为：占用时机移到 `markPaidAndFulfill`（`payments/service/service.go:809`），或保留下单占用但在 `CancelOrder`（`service.go:478`）/`ExpirePendingOrders`（`service.go:502`）释放 `UsedCount` 与对应 `UserPromoCodeApplication`。二选一并在 progress.txt 说明，但最终必须满足「未支付订单过期/取消后名额已释放」。

明确不做（out-of-scope，均由其它批次涵盖，非永久搁置）：
- scheduler 策略 CRUD（rank3 的 scheduler 部分）、scope/quality 抽象泄漏、cron/probe-model、simulate/overview 孤儿端点——**由 batch10 涵盖**。
- RBAC 细粒度权限（rank2）、affiliate-rules 端点的细粒度权限收口——本批用现有 `requireAdminWriteSession`/`requireAdminSession` 即可，权限目录化**由 batch9 涵盖**。
- `InvitationRebateEnabled` 等 feature flag 强制点（rank8）——**由 batch14 涵盖**；本批返佣不额外加开关门控（沿用现状）。
- 支付上游真实退款/对账/手续费（rank31-36）——**由 batch12 涵盖**；本批退款补偿沿用现有 `CompensateRefund` outbox 链路。
- promo 跨持久化耦合清理与 codex_quota 命名整理（rank64 其余部分）——**由 batch15 涵盖**（本批只落 UsedCount 回滚正确性）。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，**绝不改回 float64**；返佣 rate/fixed/max 与 promo 最低额运算沿用现有 money 口径（`internal/pkg/money`）。
- 不破坏已落地的 batch1-7 能力：支付下单→回调→验签→入账主链、退款扣回 balance、promo/redeem/订阅权益强制、outbox `AccrueRebate`/`CompensateRefund` 接线、上游认证矩阵 CI 守门，均须仍有测试通过。
- 凭证 AES-GCM 加密不得改明文。
- OpenAPI 兼容：新增端点/字段属增量；不得破坏现有 `PromoCode`/`AffiliateLedger`/`/me/affiliate` 响应结构（删恒 0 的 ledger 类型若影响 DTO 枚举，先在 progress 说明并按「先问我」处置）。
- 不得修改与本任务无关的 `*_test.go`；新增能力配新测试；现有 affiliate/promo/payments 测试断言不得删改让其通过。
- ent schema 改动（promo 新列 / 提现实体若新增）后须 `go generate ./ent/...` 并**同步 store 与 mock**：注意 memory store 的 `DeletedAt` 过滤约定与 Store-mock codegen 已知坑（生成的 mock 缺新方法会编译失败，需补齐）。
- 删枚举值 / 不可逆数据迁移先问我（如确要删 `settle`/`manual_adjustment` 涉及存量 ledger 行或 DTO 枚举）。
</constraints>

<success_criteria>
完成需同时满足（每条都是 agent 自己能贴出的可观察证据，评估器不自跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）；前端 `npm run -w apps/web tsc` 与 lint 通过（贴尾部）。
2. **返佣 e2e 闭环可证（核心，对应 rank1+rank3-affiliate）**：新增 e2e/集成测试证明完整链路——用户生成邀请码 → 新用户注册携带 `invite_code` 成功 `BindInvite` 建立 relationship → admin 创建一条 active `affiliate_rule` → 被推荐用户付款触发 `AccrueRebate` 返回 `Applied:true` 且写入一条 `accrue` ledger（断言 `Reason` 不再是 `no_invite_relationship`/`no_effective_rule`）；贴出测试名 + `go test` 输出。
3. **邀请入口可证（接通点）**：grep 证明 `server.go` 新增 `POST .../me/affiliate/invite-codes` 与 admin `affiliate-rules` 路由（贴 grep）；chrome-devtools 截图前端 `apps/web/src/app/affiliate/page.tsx` 邀请区（邀请码+可复制链接+已邀请人数）与 admin 规则管理页可用。
4. **ledger withdraw 可证（对应 rank15）**：测试证明提现审批通过后写入 `LedgerTypeWithdraw` 账目、`WithdrawnAmount` > 0、余额汇总正确扣减（贴测试输出）；grep 证明 `settle`/`manual_adjustment` 的最终处置——要么有生产者写入（贴写入点），要么已从 contract/summary/DTO 删除（贴 grep 证明不再有恒 0 的孤儿类型）。
5. **promo per-user/最低额可证（对应 rank17）**：测试证明同一用户超 `per_user_limit` 被拒、订单额低于 `min_order_amount` 被拒、不同用户互不影响（贴测试输出）；chrome-devtools 截图 promo 表单暴露两字段。
6. **promo 名额回滚可证（对应 rank64 部分）**：测试证明未支付订单经过期/取消后 `UsedCount`（及 `UserPromoCodeApplication`）已释放，同一码可被再次领用（贴测试输出）。
7. `go test ./...` 全绿（贴尾部）；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 35 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、每阶段自带测试并单独 commit（schema/OpenAPI 改动先行）：
1. **阶段一（契约先行）**：改 `packages/openapi/openapi.yaml`——新增 `/me/affiliate/invite-codes`（GET/POST）、`/admin/affiliate-rules`（GET/POST/{id} PATCH）、提现申请/审批端点、`PromoCode` 增 `per_user_limit`/`min_order_amount`；ent schema 若需 promo 新列/提现实体则一并改并 `go generate ./ent/...` 同步 store/mock。跑 `go build ./...` 确认生成态。commit。
2. **阶段二（affiliate 邀请后端）**：me invite-codes handler（调 `CreateInviteCode`）+ `/me/affiliate` 扩展返回邀请码/链接/计数；注册流程接 `invite_code` 调 `BindInvite`；handler 落到 affiliate 专属 runtime_* 文件（勿塞核心巨文件）。带 handler 测试。commit。
3. **阶段三（affiliate 规则 CRUD 后端）**：admin affiliate-rules handler + service Create/List/Update（`CreateRule` 已存在，补 List/Update），保证 `GetEffectiveRule` 能命中。带测试。commit。
4. **阶段四（ledger withdraw + settle/manual_adjustment 处置）**：提现申请/审批端点写 `LedgerTypeWithdraw`；接通或删除 `settle`/`manual_adjustment`。带余额汇总测试。commit。
5. **阶段五（promo per-user/最低额 + UsedCount 回滚）**：preview/finalize 按 user 计数+最低额；UsedCount 占用/释放改造（payments Cancel/Expire 或 markPaidAndFulfill）。带 promo 测试。commit。
6. **阶段六（前端 + e2e）**：affiliate 页邀请区、admin 规则页、promo 表单两字段；返佣 e2e 测试（造邀请→绑定→建规则→付款→AccrueRebate Applied:true）；chrome-devtools 截图。commit。
每阶段 commit message 末尾加：
`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + 邀请码/规则/提现的 OpenAPI 契约 + 后端 service/handler/store（含 entstore 与 mock 同步）+ 注册流程 invite_code 接线 + promo per-user/最低额/回滚改造 + 前端 affiliate 邀请区/admin 规则页/promo 表单 + 全部新测试（passing，含返佣 e2e）+ progress.txt 收尾（列每个子目标的最终处置：接通点在哪 / 或下架范围、证据位置、未做的 out-of-scope 项归属批次）。
</artifact>

<guardrails>
绝不：删除/改写既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证；**伪造未实现能力**——摆设的合法处置只有「真接通」（接上消费点端到端可验）或「诚实下架」（删字段/类型/DTO），绝不假装接通（如 ledger 类型保留恒 0 还宣称已实现、邀请码 handler 返回假数据不真写库）。
先问我：破坏现有 OpenAPI 响应结构（如改 `PromoCode`/`AffiliateLedger` 既有字段语义）；删除现有顶层路由文件；删 ent 枚举值涉及的存量迁移（如下架 `settle`/`manual_adjustment` 触及存量 ledger 行）；任何不可逆数据迁移；向真实上游发起需真凭证的调用（本批返佣/promo 不涉上游，测试用内存/httptest）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 每个子目标的接通点或下架范围 / 证据在哪 / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：已完成阶段、每个子目标的接通/下架状态、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；多数来自审计 evidence，已抽查核实存在）：
- affiliate service：`apps/api/internal/modules/affiliate/service/service.go`（`CreateInviteCode:56` / `BindInvite:72` / `CreateRule:99` / `AccrueRebate:155`，`no_invite_relationship:169` / `GetEffectiveRule:176` / `no_effective_rule:179`；ledger type switch `:240`,`:578-588`）。
- affiliate contract：`apps/api/internal/modules/affiliate/contract/contract.go`（`LedgerTypeAccrue:47` / `LedgerTypeSettle:48` / `LedgerTypeWithdraw:50` / `LedgerTypeManualAdjustment`）。
- affiliate store：`apps/api/internal/modules/affiliate/store/memory/memory.go:197`（`AppendLedger`）；entstore 对应实现 `apps/api/internal/persistence/entstore/affiliate/`。
- 路由：`apps/api/internal/httpserver/server.go`（admin GET `:480-482` invites/rebates/transfers；me GET/POST `:698-700` affiliate/ledger/transfer-to-balance；本批在此新增 me invite-codes + admin affiliate-rules + 提现端点）。
- affiliate handler：`apps/api/internal/httpserver/runtime_admin_affiliate_handlers.go`（新增 me/规则/提现 handler，勿塞 `runtime_gateway_core.go` 核心巨文件）。
- 返佣触发：`apps/api/internal/workers/outbox/domain_handler.go`（`PaymentOrderPaid`→`AccrueRebate`、`CompensateRefund` 已接线，约 :124-141）。
- promo 持久化：`apps/api/internal/persistence/entstore/admincontrol/promo.go`（`UserPromoCodeApplication` 查询 `:53/:76/:94/:124`；`UsedCount++ :102`；`previewPromoCode:200`；release 候选点 `:258`）。
- promo 契约：`apps/api/internal/modules/admin_control/contract/contract.go`（`PromoCode.MaxUses:397/:410`，本批增 `per_user_limit`/`min_order_amount`）。
- 支付占用/释放点：`apps/api/internal/modules/payments/service/service.go`（CreateOrder promo 占用 `:390` 附近；`CancelOrder:478`；`ExpirePendingOrders:502`；`markPaidAndFulfill:809`）。
- 前端：`apps/web/src/app/affiliate/page.tsx`（邀请区）；`apps/web/src/app/admin/affiliates/{invites,rebates,transfers}/`（新增同级 `rules/`）；`apps/web/src/app/admin/promo-codes/page.tsx`（promo 表单两字段）；admin/用户 API 客户端在 `apps/web/src/lib/`。
- OpenAPI：`packages/openapi/openapi.yaml`（先改后生成）。
- 架构守门：`apps/api/internal/architecture/architecture_test.go`、`internal/codequality/code_quality_test.go`。

sub2api 仅作能力对照不抄代码：本批参照点——affiliate/invitation 邀请码→绑定→返佣规则的资金链建模（邀请关系一次性绑定、规则按 trigger/有效期/币种生效、ledger 区分 accrue/settle/withdraw/manual_adjustment）；promo_code 的 per-user 限领 + 最低订单额 + first-order-only 维度，以及「占用随订单生命周期释放」的名额管理思路。仅借鉴能力与算法，不复制任何源码。
</references>
