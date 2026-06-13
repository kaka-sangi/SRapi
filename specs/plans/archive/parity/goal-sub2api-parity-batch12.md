# Goal：支付上游真实性补全 —— 退款/对账/手续费/负载均衡 + webhook 取证可见（第十二批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查后，支付主链「下单→回调验签→入账→退款扣回 balance」已端到端闭环且资金安全（stripe/wechat/alipay/easypay 真 SDK+验签+幂等），但支付与**上游网关的真实同步**仍是一片摆设/缺失：`RequestRefund` 只改本地账务从不调任何提供商退款 API（本地已退、上游未退，对不上账）；完全依赖被动 webhook，无主动查单/对账，webhook 丢失则用户已付款而订单卡 Pending 直到过期；无手续费率、无通道负载均衡（多实例永远只用排序最前者）；`OrderStatusFailed` 枚举从不被设置、退款无 `refunding/refund_failed` 中间态；每次 webhook 写的 `PaymentAuditLog`（含验签结果/payload/idempotency_key）只写不读、排障关键证据写入即埋葬；wechat jsapi/h5 强制的 `payer_openid/client_ip` 只能藏 `Metadata` 自由 map，标准 UI 跑不通 jsapi 通道。这些既是**资金真实性漏洞**（本地退款假成功、丢单无兜底），也是**取证盲区**（验签失败/重复回调无从复盘）。
> 前置依赖：无强外部依赖；与 batch8（affiliate/promo 资金链）无冲突，可并行；checkout.Provider 接口扩展是本批所有 provider 实现的前提，须先行。注意 runtime_gateway_core.go 仅余 ~44 行触线余量，但本批落点在 payments 模块与 httpserver 支付 handler，不碰 core；若新增支付 handler 使既有文件超 2200 行，参考 batch15 拆分策略或先问。
> 关联：本批是 commerce 域真实性补全的第二块，承接 batch8（分销/促销资金链）；与 batch13（计费维度深化）同属 commerce/billing 谱系，建议 batch13 接其后。退款扣回 balance 的本地资金安全（batch1-7 已落地）必须保持不回归。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个对标 sub2api 的 AI 网关/计费平台，管理面比 sub2api 更宽。
技术栈与约定：
- 后端 Go + ent ORM（apps/api），**OpenAPI-first**：先改 `packages/openapi/openapi.yaml` 再生成，不手写对外 DTO。ent schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`。支付域在 `apps/api/internal/modules/payments/{contract,service,store}` + `providers/{checkout,stripe,wechat,alipay,easypay}`，后台 worker 在 `apps/api/internal/workers/<名>`。
- 前端 Next.js + TypeScript（apps/web）：路由在 `app/`，组件在 `components/`，导航在 `components/layout/nav-items.ts`，admin SDK 在 `apps/web/src/lib/`。
- 本地开发：`make dev-up` 起后端依赖，`apps/web` 下 `npm run dev`；不要设 `NEXT_PUBLIC_SRAPI_BASE_URL`（CSP 会拦跨源）；登录 admin@srapi.local / Admin1234。
- 所有改动须过 `cd apps/api && go build ./... && go vet ./...`，前端过 `tsc --noEmit` 与 lint。
- **架构红线**（`apps/api/internal/architecture/architecture_test.go` + `internal/codequality/code_quality_test.go` 守门）：模块生产码只能 import 别模块的 `contract` 层（白名单，service/store/handler 间禁止直接耦合；contract 禁止 import ent/生成 DTO/httpserver）；worker 只能 import 模块 contract/service；单文件 ≤2200 行（runtime_* 文件，runtime_http.go 兼容层 ≤120 行）；单函数 ≤210 行；全部 `gofmt` 必过。
- 凭证/密钥 AES-GCM 加密不得改明文；上游退款/查单凭证沿用现加密路径读取。
- 绝不抄 sub2api 源码，只借鉴能力与算法思路。
本批主题强调点：支付提供商接口扩展（Refund/QueryOrder）须是**接口先行、各 provider 逐一实现、单测 httptest mock 上游**；任何「本地状态推进」都必须**以上游真实成功为前提**，绝不本地假成功。
</role_and_context>

<objective>
目标（可衡量最终态）：管理员/系统对支付订单的每一个生命周期操作都**与上游真实同步且可取证**——退款先调上游成功才落 `Refunded`、上游失败则回滚不扣回 balance；丢单有主动对账 worker 兜底（已支付的 Pending 订单自动 fulfill）；手续费按通道叠加；多实例按策略分流；`failed` 态真落地；每笔订单的 webhook 验签/payload 时间线在 admin 可查；jsapi/h5 的 `payer_openid/client_ip` 是一等类型化入参。完成后用 httptest mock 上游证明：真退款链、退款中间态、对账兜底、手续费、负载均衡、failed 态、审计可见、jsapi 入参八条均可验证。
动机：当前「本地退款假成功」是直接的**资金真实性漏洞**（账面已退、上游未退，用户重复申诉时无对账依据）；「完全依赖 webhook 无兜底」让丢单订单永久卡死（用户已付款拿不到货）；`PaymentAuditLog` 只写不读让「丢单/重复回调/验签失败」三类最常见支付事故的取证证据写入即埋葬。这些摆设/缺失让系统在「支付正常」的表象下隐藏真实的钱与信任风险。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元；每个子目标带真实文件锚点；摆设类必须接到真实消费点）：

1. **rank31 上游真实退款**：`checkout.Provider`（`apps/api/internal/modules/payments/providers/checkout/checkout.go:32`，现仅 `CreateSession`）增 `Refund(req RefundRequest) (RefundResult, error)`（或新增独立 `Refunder` 接口，按能力探测）；stripe/wechat/alipay/easypay 各 `provider.go` 实现真实退款 SDK 调用（含验签/幂等）；`RequestRefund`（`payments/service/service.go:744`）改为**先调上游 Refund 成功 → 再落 `Refunded` + 扣回 balance + 记账**，上游失败则回滚（不动 balance、订单不进退款态）。**消费点**：RequestRefund 退款链 + admin 退款端点。

2. **rank35 退款中间态 + 退款 webhook 终态**：`OrderStatus`（`payments/contract/contract.go:29-33`）增 `refunding`/`refund_failed`；`validateTransition`（`service.go:1101`）补合法转移（paid/partially_refunded → refunding → refunded/refund_failed）；上游异步退款场景下 RequestRefund 先落 `refunding`，退款 webhook 回调推进 `refunded`/`refund_failed` 终态。**消费点**：退款 webhook 分支真实更新订单状态。

3. **rank32 主动对账（QueryOrder）**：`checkout.Provider` 增 `QueryOrder(req QueryRequest) (QueryResult, error)`，各 provider 实现；新增 `apps/api/internal/workers/payment_reconcile/worker.go`（镜像 `apps/api/internal/workers/order_expirer/worker.go` 模式），周期对 Pending 订单调上游 QueryOrder，已支付则走与 webhook 同一条 `markPaidAndFulfill` 兜底路径并写审计。**消费点**：对账 worker 调 QueryOrder 后兜底 fulfill。

4. **rank33 支付手续费率**：`PaymentProviderInstance`（contract + ent schema）增 `fee_rate`；`CreateOrder`（`service.go:377`）按选中通道 `fee_rate` 计算应付额（区分「充值额/到账额」与「应付额」，金额一律 string+big.Rat 8 位定点）；admin 通道实例表单暴露 `fee_rate`，前端下单展示手续费明细。**消费点**：CreateOrder 计算 + 通道表单 + 下单明细。

5. **rank34 通道负载均衡**：`selectProviderInstance`（`service.go:957`，现 `SliceStable` 后恒 `return candidates[0]`）支持策略选择（至少 round-robin；或 `PaymentProviderInstance` 加 `weight` 做加权选择）；admin 暴露策略/权重配置。**消费点**：selectProviderInstance 真按策略分流。

6. **rank23 OrderStatus failed 真落地**：`HandleWebhook`（`service.go:548`，现 `status!='paid'` 仅返回 `Handled:false`，见 :621 附近分支）在 `normalized.Status=='failed'` 时对 Pending 订单执行 `validateTransition → OrderStatusFailed` 落库 + 审计。**消费点**：failed webhook 真置 Failed 态。

7. **rank16 PaymentAuditLog 可见**：先改 OpenAPI 加 `PaymentAuditLog` schema + `GET /api/v1/admin/payment-orders/{id}/audit-logs`，handler 调既有 `ListAuditLogsByOrder`（`sanitizePayload` 已 redact），admin 订单详情抽屉展示回调时间线（事件类型/验签结果/idempotency_key/时间）。**消费点**：新建 admin 端点 + 订单详情时间线 UI（`apps/web/src/app/admin/orders/page.tsx`）。

8. **rank24 WeChat payer_openid/client_ip 类型化入参**：`CreateOrderRequest`（`contract.go:144`，现仅有 `Metadata` map）增显式 `PayerOpenID`/`PayerClientIP`（OpenAPI 先行）；handler 从 `X-Forwarded-For` 自动填 client_ip；wechat provider 改优先读类型化字段再回退 metadata；前端 jsapi 选项引导授权拿 openid 并展示该字段。**消费点**：handler 透传 + wechat provider 消费 + 前端 jsapi 表单字段。

9. **rank36 Airwallex provider（条件做）**：若目标市场需要，新增 `apps/api/internal/modules/payments/providers/airwallex/` 实现 `checkout.Provider`（含本批扩展的 Refund/QueryOrder）。**注**：rank36 confidence=med；若产品确认不进入相应市场，则在 progress.txt + docs 明确「本批暂不实现 Airwallex，归入后续支付提供商扩展」——**这是排期归属而非永久搁置**，不得写成 deferral。优先做完 1-8。

明确不做（out-of-scope，仅「由其它批次涵盖」或「本批刻意不碰」，非永久搁置）：
- 计费维度深化（service_tier 倍率价 / cache 5m·1h 分档 / 长上下文倍率 / 图片 token 独立费率 / BillingModelSource）—— 由 **batch13** 涵盖。
- 微信支付 OAuth 取 openid 的 start+callback 路由/页面（端用户登录子系统，依赖 wechat 登录能力 rank54/rank51）—— 由 **batch14** 涵盖（本批的 rank24 只做「下单时类型化传 openid/client_ip」，不做获取 openid 的 OAuth 流程）。
- console 写操作幂等保护（rank55，含 POST /payment/orders 幂等）—— 由 **batch14** 涵盖。
- promo per_user_limit/min_order_amount 与 UsedCount 回滚（rank17/rank64 部分）—— 由 **batch8** 涵盖。
- 不动 scheduler、affiliate、subscriptions 业务本体；不顺手清理无关代码或重构无关文件。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64；手续费/退款/应付额计算全部走既有 money 口径（沿用 batch2 落地的 `internal/pkg/money`，不引第二套格式化）。
- 不破坏 batch1-7 已落地能力：下单→webhook 验签→入账主链、退款扣回 balance 的资金安全、幂等（`withGatewayIdempotency`）、订单过期（order_expirer worker）、promo/redeem/订阅权益强制、outbox 事件（PaymentOrderPaid 等）必须仍有测试通过。
- 凭证 AES-GCM 加密不得改明文；上游退款/查单凭证沿用现加密读取路径。
- OpenAPI 兼容：新增 `payer_openid/payer_client_ip`、`fee_rate`、`PaymentAuditLog` schema、`refunding/refund_failed` 枚举值均为**新增**（向后兼容）；不得破坏现有响应结构（如改 `OrderStatus` 已有取值、删字段需先问）。
- 不得修改与本任务无关的 `*_test.go`；迁移/扩展接口后原 provider 测试应仍覆盖（必要时调 mock 实现新接口方法，不得删断言）。
- ent schema 改动（`PaymentProviderInstance` 加 `fee_rate`/`weight`、`OrderStatus` 枚举加值）后须 `go generate ./ent/...` 并同步 store/mock：注意 **memory store 的 DeletedAt 过滤** 与 **Store-mock codegen 坑**（生成后核对 mock 方法签名）。新增枚举值是兼容操作；**删枚举值或不可逆数据迁移先问我**。
- 退款/查单/对账测试一律 httptest mock 上游，**绝不向真实支付上游发起需真实凭证的调用**。
其他约束：新增依赖（如某 provider 的退款 SDK 方法）须先说明理由；若退款/查单可复用 provider 已有 SDK client 则不引新库。
</constraints>

<success_criteria>
完成需同时满足（每条给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部输出）；前端 `npm run lint` 与 `tsc --noEmit` 通过（贴尾部）。
2. **真实退款可证（核心）**：新增测试以 httptest mock 上游退款 API，证明 `RequestRefund` **先调上游 Refund 成功才落 `refunded` 并扣回 balance**；并有一条「上游退款失败→订单不进退款态、balance 不变、返回错误」的回滚断言。贴出测试名 + `go test` 输出。
3. **退款中间态可证**：测试证明同步退款走 paid→refunding→refunded、异步退款由退款 webhook 把 `refunding` 推进 `refunded`/`refund_failed`；贴出 `validateTransition` 新增合法转移的 grep + 测试输出。
4. **对账兜底可证**：测试以 mock QueryOrder 返回「已支付」，证明 `payment_reconcile` worker 对一笔 Pending 订单调上游后走 `markPaidAndFulfill` 兜底（订单转 paid、产物履约、写审计）；贴出 worker 测试名 + 输出 + worker 文件路径。
5. **手续费可证**：测试证明给通道设 `fee_rate` 后 `CreateOrder` 的应付额按费率叠加且为 8 位定点 string（贴一组 amount×fee_rate→应付额断言）；前端通道表单 + 下单明细展示手续费的 chrome-devtools 截图。
6. **负载均衡可证**：测试证明多个同 method 实例在 round-robin（或加权）下被分流到不同实例，而非恒取 `candidates[0]`（贴分流断言输出）。
7. **failed 态可证**：测试证明 `HandleWebhook` 收 `status=='failed'` 对 Pending 订单落 `OrderStatusFailed` + 写审计（贴测试输出 + grep 证明存在 Failed setter，非仅枚举定义）。
8. **审计可见可证**：`GET /api/v1/admin/payment-orders/{id}/audit-logs` 返回回调时间线（事件类型/验签结果/时间，payload 已 redact）；贴 handler 测试输出 + admin 订单详情时间线抽屉的 chrome-devtools 截图。
9. **jsapi 入参可证**：grep 证明 `CreateOrderRequest` 含 `PayerOpenID/PayerClientIP` 且 OpenAPI 已暴露；测试证明 wechat jsapi 通道用类型化 openid 下单成功（不再依赖 Metadata 自由 map）；前端 jsapi 选项字段截图。
10. **Airwallex 处置可证**：要么 provider 已实现并有 `checkout.Provider` 接口符合性测试通过；要么 progress.txt + docs 明确「本批暂不做 Airwallex，归入后续支付提供商扩展批次」并说明理由（非永久搁置）。
11. 回归可证：现有支付主链测试（下单/webhook 验签/入账/退款扣回 balance/订单过期/幂等）全绿（贴尾部）；`go test ./...` 全绿；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 35 turns 后停止并汇总剩余阻塞项与各 rank 的处置状态。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit；schema/OpenAPI 改动先行；每阶段 commit message 末尾加 Co-Authored-By 行：
1. **阶段一（接口 + schema/OpenAPI 先行）**：先改 `packages/openapi/openapi.yaml`（`payer_openid/payer_client_ip`、`fee_rate`、`PaymentAuditLog` schema + audit-logs 端点、`refunding/refund_failed` 枚举）并生成；ent schema 加 `fee_rate`/`weight` + `OrderStatus` 枚举值后 `go generate ./ent/...` 同步 store/mock；`checkout.Provider` 扩 `Refund` + `QueryOrder`（或拆 `Refunder`/`OrderQuerier` 接口），各 provider 先加**空实现/未支持错误**让 `go build` 绿。commit。
2. **阶段二（真实退款 + 中间态）**：stripe/wechat/alipay/easypay 实现真实 `Refund`；`RequestRefund` 接上游（先成功后落态、失败回滚）+ `refunding/refund_failed` 中间态 + `validateTransition` 转移；退款 webhook 分支推进终态。配 httptest mock 退款上游的测试。commit。
3. **阶段三（对账 worker）**：各 provider 实现 `QueryOrder`；新建 `payment_reconcile` worker（镜像 order_expirer）周期对账 Pending 订单兜底 fulfill。配 worker 测试。commit。
4. **阶段四（手续费 + 负载均衡 + failed 态）**：`CreateOrder` 接 `fee_rate` 应付额；`selectProviderInstance` round-robin/weight；`HandleWebhook` failed 态落地 + 审计。配测试。commit。
5. **阶段五（审计可见 + jsapi 入参）**：admin audit-logs 端点 handler + 订单详情时间线 UI；`CreateOrderRequest` 类型化 `PayerOpenID/PayerClientIP` + handler 自动填 client_ip + wechat provider 消费 + 前端 jsapi 字段。配测试 + 截图。commit。
6. **阶段六（条件：Airwallex）**：若产品确认需要，新建 airwallex provider 实现全接口；否则在 progress.txt/docs 记排期归属。commit（若做）。
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/payments-upstream-truth-batch12`）若干语义化 commit + 扩展后的 `checkout.Provider`（Refund/QueryOrder）+ 各 provider 真实实现 + `payment_reconcile` 对账 worker + `fee_rate`/负载均衡/failed 态/审计端点/类型化 jsapi 入参 + 全部新测试（httptest mock 上游，全 passing）+ 前端订单详情时间线/通道手续费/jsapi 字段 UI + progress.txt 收尾（列每个 rank 的最终处置、证据位置、Airwallex 排期决定、未做的 out-of-scope 项及其归属批次）。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证；在测试里打真实支付上游；**伪造未实现能力**——退款/对账若某 provider 真做不了，合法处置只有「真接通该 provider 的退款/查单」或「诚实标注该 provider 不支持并返回明确错误（且不在 UI 假装可退）」，绝不本地假成功冒充上游已退；Airwallex 不做就别假装已接。
先问我：破坏现有 OpenAPI 响应结构（改已有 `OrderStatus` 取值/删字段）；删除现有顶层路由文件；删 ent 枚举值的存量迁移；任何不可逆数据迁移；向真实支付上游发起需真实凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 每个 rank 处置与证据在哪 / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：已完成阶段、每个 rank（31/35/32/33/34/23/16/24/36）的当前处置状态、编译/测试状态、Airwallex 决定、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；已抽查核实，标注修正处）：
- 提供商接口：`apps/api/internal/modules/payments/providers/checkout/checkout.go:32-33`（`type Provider interface { CreateSession }`，待扩 Refund/QueryOrder）。
- 各 provider 实现：`apps/api/internal/modules/payments/providers/{stripe,wechat,alipay,easypay}/provider.go`（现有 5 个目录：alipay/checkout/easypay/stripe/wechat，**无 airwallex** — 已核实）；wechat `provider.go:275-276` 现从 `Metadata`/`Config` 读 `payer_client_ip`/`payer_openid`，:284-287 缺则 `ErrInvalidConfig`。
- 支付 service：`apps/api/internal/modules/payments/service/service.go`：`CreateOrder:377`（直接用 amount 下单，待接 fee_rate）、`HandleWebhook:548`（status!='paid' 仅 Handled:false 分支约在 :621）、`RequestRefund:744`（纯本地账务，无 provider 调用）、`selectProviderInstance:957`（SliceStable 后恒 return candidates[0]）、`validateTransition:1101`（转移表，failed 合法但无 setter）。
- 支付 contract：`apps/api/internal/modules/payments/contract/contract.go:29-33`（`OrderStatusPartiallyRefunded/Refunded/Failed`，缺 refunding/refund_failed）、`:144`（`CreateOrderRequest` 仅 UserID/Method/Amount/Currency/ProductType/ProductID/PromoCode/ExpiresAt/Metadata — **无 PayerOpenID/PayerClientIP**，已核实）。
- 审计：`service.go` 写入约 :526/:593；`ListAuditLogsByOrder`（接口+store+测试已有，无上层调用方，`sanitizePayload` 已 redact）；OpenAPI 现无 `PaymentAuditLog` schema。
- admin 支付 handler：审计核查写 `runtime_admin_*payment*.go`（glob 占位）；**实际**支付订单 admin handler 在 `apps/api/internal/httpserver/runtime_admin_control_handlers.go`（含 payment-orders 相关）+ `runtime_admin_control_plane_handlers.go`，新增 audit-logs 端点落于此文件族（按行数余量选；勿超 2200 行红线）。
- worker 镜像模式：`apps/api/internal/workers/order_expirer/worker.go`（+ worker_test.go）作为新 `payment_reconcile` worker 的结构样板（已核实 order_expirer 存在）；同级有 outbox/balance_charger/quota_refresh 等可参考注册方式。
- 路由注册：`apps/api/internal/httpserver/server.go`（payment-orders 路由注册处；新增 `GET /api/v1/admin/payment-orders/{id}/audit-logs`）。
- 前端：`apps/web/src/app/admin/orders/page.tsx`（订单管理页，加审计时间线抽屉）；admin SDK `apps/web/src/lib/`（加 audit-logs 拉取 + fee_rate/jsapi 字段类型）；下单/通道表单组件按 payments 现有前端结构定位。
- 金额口径：`apps/api/internal/pkg/money`（batch2 落地的统一定点运算，手续费/退款/应付额一律走它）。
- OpenAPI 源：`packages/openapi/openapi.yaml`（先改后生成：payer 字段 / fee_rate / PaymentAuditLog / audit-logs 端点 / refunding·refund_failed 枚举）。
- 守门：`apps/api/internal/architecture/architecture_test.go`（≤2200 行 / contract-only import）、`apps/api/internal/codequality/code_quality_test.go`（≤210 行/函数）。

sub2api 仅作能力对照不抄代码。本批相关参照点（只借鉴能力与算法思路）：
- sub2api 的 `PaymentService.Refund` 调上游退款 API 后再落本地态（先上游后本地的顺序与失败回滚语义）。
- sub2api 的对账/查单（QueryOrder/Reconcile）兜底已支付的 Pending 订单的轮询 worker 思路。
- sub2api 的通道 `fee_rate`/区分充值额与到账额的应付额计算，以及多通道按 round-robin/权重选择实例。
- sub2api 的 webhook 失败态落地（failed）与退款 in-flight（refunding/refund_failed）中间态机。
- sub2api 的支付审计/回调时间线在 admin 的可见化。
</references>
