# Goal：账号认证/配额/TLS 绑定摆设清除 + 上游配额补全（第十一批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）即可执行。
> 背景：8 域对抗性核查后，accounts-auth-quota + gateway-adapters 域仍残留一片「摆设」与「假配额」：后端 expander 已真接 uTLS 出站但账号表单无 TLS profile 绑定入口（唯一套用途径是 Advanced 里手敲 `metadata.tls_profile`）；`risk_level` 字段被 scheduler 评分消费却缺席 Create/Update 入参（high/medium penalty 是死代码）；`service_account_json` 是无签名器的必失败枚举（选了即 401）；`desktop_client_token`/`ide_plugin_token` 是与 `oauth_refresh` 完全同处理的冗余别名；`QuotaReport` 解析出 plan/credits 却 toast 弹出即丢从不持久化；antigravity 账号无 `QuotaConfig` 导致 `FetchAccountQuota` 恒 `Supported=false`；channel-monitor 的 `interval_seconds`/`enabled`/`trigger` 三字段因无调度 worker 全惰性（唯一执行路径是手动 `/run`，禁用项照样能跑、`scheduled` 枚举值不可达）。这些「摆设 = 假配额 / 抽象泄漏 / 假可用容量」，会让系统把发不出请求或没配额的账号当「可用」纳入调度统计，运营盲目依赖不存在的容量。本批把账号域的认证/配额/出站绑定从「能配/能选」升级为「配了就真生效、没有摆设项」。
> 前置依赖：无强外部依赖。本批与 batch8（分销资金链）、batch9（RBAC/风控/内容安全）、batch10（调度策略 CRUD）相互独立，可并行；channel-monitor 调度 worker（rank6）镜像 batch5/既有 `health_probe`/`quota_refresh` worker 模式。注意 `runtime_gateway_core.go` 仅余 ~44 行触线余量，本批若需改动该文件，先按 batch15(rank59) 思路就地拆出受影响段落再加行（详见 `<constraints>`）。
> 关联：与 batch7（上游认证矩阵真实性）同源——batch7 已处置 allowlist⊆可签发集合的 CI 守门，本批落定其中 `service_account_json`/`desktop`/`ide` 三个 class 的最终代码处置（batch7 若已下架则本批仅做回归确认与 schema 收尾）；与 batch5（配额真实性）强相关——本批补 antigravity 配额与 credits 持久化，喂给已落地的告警/调度下游消费。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个对标 sub2api 的 AI 网关/计费平台，管理面比 sub2api 更宽。
技术栈与约定：
- 后端 Go + ent ORM，OpenAPI-first（apps/api）：实体 schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 装配在 `apps/api/internal/httpserver/`，后台任务在 `apps/api/internal/workers/<worker>/`。任何对外契约改动先改 `packages/openapi/openapi.yaml` 再生成，绝不手改生成物。
- 前端 Next.js + TypeScript（apps/web）：路由在 `app/`，组件在 `components/`（admin 表单组件在 `components/admin/`），导航在 `components/layout/nav-items.ts`，admin SDK 类型/调用在 `src/lib/`。
- 本地开发：`make dev-up` + `npm run dev`；登录 admin@srapi.local / Admin1234。不要设 `NEXT_PUBLIC_SRAPI_BASE_URL`（CSP 会拦跨域）。
- 所有后端改动须过 `cd apps/api && go build ./... && go vet ./...`；前端须过 `tsc` 与 `lint`。
- 架构红线（由 `apps/api/internal/architecture/architecture_test.go` 与 `apps/api/internal/codequality/code_quality_test.go` 守门）：模块生产码跨模块只能 import 目标模块的 `contract` 层（白名单）；contract 层禁止 import ent / 生成的 OpenAPI DTO / httpserver 包；worker 只能 import 模块 contract/service；ent/Redis store 只能 import 模块 contract；单文件 ≤ 2200 行（runtime_* 文件），单函数 ≤ 210 行；`gofmt` 必须通过。
- 凭证一律 AES-GCM 加密存储，不得改回明文；金额一律 string + big.Rat 8 位定点（本批不碰计费，但若顺带触及别动）。
- 绝不抄 sub2api 源码，只借鉴能力与算法思路。
本批主题强调点：账号域的「认证签发 / 配额获取 / 出站 TLS 绑定」三条链必须名实相符——要么真接通到下游消费点，要么诚实下架（删字段/删枚举值/隐藏 UI），不留「配了不工作」的伪选项。
</role_and_context>

<objective>
目标（可衡量最终态）：账号域里每一个「可配/可选/已解析」的认证、配额、出站绑定能力都名实相符——(a) 管理员能在账号表单一等公民地选 TLS 指纹 profile 并真生效 uTLS 出站；(b) `risk_level` 可设且 scheduler 的 high/medium penalty 分支真被命中（不再死代码）；(c) `service_account_json` 不再是必失败摆设（删枚举值或实接 SA 签名器）、`desktop_client_token`/`ide_plugin_token` 不再是悬空冗余别名；(d) `QuotaReport` 的 plan/credits 被持久化并可被告警/调度/历史消费（不再 toast 即丢）；(e) antigravity 账号 `FetchAccountQuota` 返回 `Supported=true` 并写 snapshot；(f) channel-monitor 的 `interval_seconds`/`enabled`/`trigger` 三字段被一个调度 worker 真正消费（按 interval 周期跑、禁用项被跳过、自动运行写 `Trigger=scheduled`）。
动机：摆设认证/配额 = 假可用容量。选了 `service_account_json` 的账号、解析出 credits 却不存的账号、antigravity 永远 `Supported=false` 的账号，会被系统当「可用/有配额」纳入调度与统计，实际 401 / 无配额数据 / 永不触发告警，运营依赖不存在的容量；TLS profile 配了套不上账号则反爬伪装形同虚设；risk_level/penalty 分支死代码让运维的风险标注毫无效果；channel-monitor 三惰性字段让运营以为「定时监控已开」实则只有手动一次性探测。
</objective>

<scope>
本次包含（按 `<sequencing>` 一次推进一个单元；每个「二选一」必须落定到无中间态，不留「字段在但不消费」的残留）：

1. **rank7 — TLS 指纹 profile 账号绑定 UI（M）**：
   - `account-form-dialog.tsx` + `admin-account-form.ts` 增加一等公民「TLS 指纹 profile」字段：一个 `enable` 开关 + 一个 profile 下拉（数据源 `useTlsProfiles` 仅取 `enabled` 的），选中后写入 `metadata.tls_profile=<profile 名>`（与后端 `SetNamedProfileExpander`/`expandEgressProfileMetadata` 消费的键一致）。
   - 渠道详情页 `apps/web/src/app/admin/channels/[id]/page.tsx:542-565` 的 `TlsPanel` 从「渲染全量 `profiles.map`」改为「本渠道下账号实际绑定 `metadata.tls_profile` 的聚合视图」（哪些 profile 被本渠道哪些账号引用）。
   - 过渡期给账号表单 `metadata_hints` 补 `tls_profile` 提示（避免老用户找不到键）。
   - 消费点证明：选了 profile 的账号出站请求走对应 uTLS ClientHello（`runtime_state.go:483` 已 wire `SetNamedProfileExpander`，本批只接 UI 入口）。

2. **rank11 — ProviderAccount `risk_level` 可设（S）**：
   - `risk_level` 加入 `accounts` 的 `UpdateRequest`（可选加入 `CreateRequest`），accounts service + update handler 做字段映射（先改 `openapi.yaml` 再生成）。
   - `account-form-dialog.tsx` 加 `select(normal/medium/high)`。
   - 消费点已存在：scheduler riskPenalty（high=0.15 / medium=0.05）即生效，high/medium 分支不再死代码。

3. **rank27 — `service_account_json` 必失败摆设处置（S，二选一落定）**：
   - 默认 (a) **删枚举值**：从 `provideraccount.runtime_class` 枚举删 `service_account_json`（已从所有 preset allowlist/UI 隐藏，`registry_test` 已强制排除，删除最干净），留迁移注释；删值前先确认存量账号无该 runtime_class（见 `<guardrails>`）。
   - 或 (b) **实接 Google SA 签名器**：解析 credential 的 SA JSON（client_email/private_key）→ RS256 签 JWT → POST `oauth2.googleapis.com/token` 换 access_token → 按 exp 缓存刷新 → `injectAuth`（`reverse_proxy/service/service.go:701`）增 case 走 Bearer。换 token 用 httptest mock，不打真实上游。
   - 注：若 batch7 已对此 class 做过处置，本批只做代码层落定 + 回归确认，不重复决策。

4. **rank28 — `desktop_client_token` / `ide_plugin_token` 冗余别名处置（S，二选一落定）**：
   - 二者在 `injectAuth`（service.go:709 与 `oauth_refresh`/`oauth_device_code` 同 case 共用 Bearer access_token）无独立行为、无 preset/UI 暴露。
   - 默认 (a) **文档化为 `oauth_refresh` 纯别名**（保留行为正确的现状，在 schema/代码加注释说明是 legacy 别名）；可选 (b) 读时折叠为 `oauth_refresh` 让 schema 整洁。低优先（行为已正确，重在消除「悬空枚举」的误导）。

5. **rank13 — `QuotaReport` plan/credits 持久化（M）**：
   - `quota_fetch.go:77-80` 已解析 `Plan`/`CreditsRemaining`/`CreditsUsed`/`CreditsLimit`/`Currency`，admin fetch handler 返回即丢。
   - 把 plan/credits 持久化为账号 `metadata`（`last_quota_plan` / `last_quota_credits_remaining` / `_used` / `_limit` / `_currency`）或合成一条 credits 维度的 `QuotaSnapshot`，使其存活并能喂告警/调度/历史查看（与现有 `persistQuotaSignals` 同路径）。
   - 补 codex `account_plan.*` 路径的 fixture 测试（现 `quota_fetch_test.go` 只测 anthropic）。
   - 消费点证明：持久化后告警 worker（`account_quota_alert`）/admin 历史视图能读到 credits。

6. **rank38 — Antigravity 账号配额获取（M）**：
   - `antigravityPreset()`（`providers/preset/registry.go:258`）无 `QuotaConfig`（preset 字段在 `registry.go:73`），导致 `quota_fetch.go:114 QuotaConfigured=false` → 每个 antigravity 账号 `FetchAccountQuota Supported=false`。
   - 给 `antigravityPreset` 加 `QuotaConfig`（quota_url + 解析 paths，OAuth access_token + project_id 鉴权）打 Antigravity models/quota 端点，解析 forbidden/validation/violation 类信号（既有 forbidden 持久化机制会自动点亮）。
   - 探针/换 token 用 httptest mock，不打真实上游。
   - 消费点证明：`FetchAccountQuota` 对 antigravity 返回 `Supported=true` 并写 `AccountQuotaSnapshot`。

7. **rank6 — channel-monitor 调度 worker（M，同时点亮三摆设字段）**：
   - `internal/workers/` 下无任何消费 `channel_monitors`/`MonitorDefinition` 的 worker（唯一执行路径是手动 `runtime_admin_channel_monitor_run.go`，其 `:99` 硬编码 `Trigger: "manual"`，且 `/run` 不检查 `def.Enabled`）。
   - 新增 `internal/workers/channel_monitor` 调度 worker，镜像 `health_probe`/`quota_refresh` 模式：选 `enabled` 且 `last_run` 早于 `interval_seconds` 的 def，跑 `runChannelMonitorProbe`，`RecordRun` 时 `Trigger="scheduled"`。
   - `scheduled` 自动路径与手动 `/run` 都 gate `enabled`（禁用项被跳过）。
   - 探针执行逻辑从 http 层（`runtime_admin_channel_monitor_run.go`）抽进 `channel_monitors` 模块 service，供 worker 与 handler 复用（修正「探针逻辑写在 http 层无法被 worker 复用」的错位）。
   - 消费点证明：`interval_seconds` 真驱动周期、`enabled=false` 被跳过、自动运行写 `Trigger=scheduled`（三摆设字段同时点亮）。

明确不做（out-of-scope，均由其它批次涵盖，非永久搁置）：
- 端用户登录 wechat/dingtalk 摆设、邮箱验证码、OAuth 免密、~12 个 feature flag、备份 worker —— 由 **batch14** 涵盖。
- 上游真实退款/对账/手续费/通道负载均衡/Airwallex —— 由 **batch12** 涵盖。
- 计费维度深化（service_tier/cache 分档/长上下文/图片 token/BillingModelSource/daily-weekly 配额/usage 聚合分解）—— 由 **batch13** 涵盖。
- RBAC 细粒度、风控强制点、内容安全可配置 —— 由 **batch9** 涵盖。
- scheduler 策略 CRUD / scope 加载 / cron 解析 / 探测模型选择器 / simulate-overview 孤儿端点 —— 由 **batch10** 涵盖。
- 拆五个核心巨文件 / 消重 / 死代码 / SOCKS5+uTLS / 完整自定义 ClientHello —— 由 **batch15** 涵盖（本批改 `runtime_gateway_core.go` 时仅就地小幅拆段腾余量，不做系统性重构）。
- Gemini OAuth/RPD（rank39）—— 由 batch15 或单独排期处置，本批不碰（仅 antigravity 配额）。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64（本批不碰计费，若顺带触及别动）。
- 不破坏已落地的 batch1-7 能力：支付下单→回调→入账主链、退款扣回 balance、promo/redeem/订阅权益强制、payload 转换引擎、error-passthrough、TLS 指纹 uTLS 出站本体、会话粘度、故障转移+冷却、分组费率倍率、anthropic/codex 真实配额获取+告警下游消费、账号导入认证矩阵 + batch7 的 allowlist⊆可签发集合 CI 守门——这些的现有/新增测试必须仍全绿。
- 凭证 AES-GCM 加密不得改明文；OAuth refresh token 存储沿用现加密路径（rank27 实接 SA 时新 token 缓存也走加密路径）。
- 现有 OpenAPI 响应结构兼容：`risk_level` 进 Create/Update 属新增可选字段（向后兼容）；删 `service_account_json` 枚举值属配置/schema 层变更——若涉及改 `auth_methods` 暴露结构需先说明。
- 不修改与本批无关的 `*_test.go`；新增能力配新测试；拉取上游/换 token/探配额一律用 httptest mock，绝不打真实上游。
- ent schema 改后（如删 `runtime_class` 枚举值、`risk_level` 已是 schema 字段无需改）须 `cd apps/api && go generate ./ent/...`，并同步 store/mock：注意 memory store 的 DeletedAt 过滤、Store-mock codegen 已知坑（生成的 mock 方法签名需与接口一致）。
- 删 `runtime_class` 枚举值是不可逆迁移：删值前先 grep + 查存量账号是否有该值，先问迁移策略（见 `<guardrails>`）。
- channel-monitor 探针逻辑下沉到模块时，跨模块只能 import 目标模块 contract（架构红线），别让 worker 直接 import http 层或其它模块 service。
- `runtime_gateway_core.go` 仅余 ~44 行触线余量：本批若必须改它（如 rank13 持久化挂在请求路径上），优先把改动落到模块 service 或新文件，避免把该文件推过 2200 行红线。
其他约束：新增依赖（如 RS256 JWT 库供 rank27(b)）须先说明理由。
</constraints>

<success_criteria>
完成需同时满足（每条在输出里给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：

1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）；前端 `npm run -w apps/web tsc`（或等价）+ `lint` 通过（贴出尾部）。

2. **rank7 TLS 绑定可证**：`grep -rn "tls_profile" apps/web/src/components/admin/account-form-dialog.tsx apps/web/src/lib/admin-account-form.ts` 命中新字段（贴出，证明不再是 0）；chrome-devtools 截图账号表单的「TLS 指纹 profile」开关+下拉、以及渠道详情 `TlsPanel` 的本渠道聚合视图；测试或 trace 证明选了 profile 的账号出站走对应 uTLS ClientHello（消费点：`SetNamedProfileExpander`）。

3. **rank11 risk_level 可证**：`grep -rn "RiskLevel" apps/api/internal/modules/accounts/` 证明已进 Update/CreateRequest + service 映射（不再仅在实体 struct）；测试证明设 `risk_level=high` 的账号在 scheduler 评分时命中 0.15 penalty 分支（high/medium 不再死代码）；chrome-devtools 截图账号表单的 risk_level select。

4. **rank27 service_account_json 处置可证**：
   - 若删枚举值：`grep -rn "service_account_json" apps/api/internal/` 证明枚举/injectAuth 不再含该值（仅留迁移注释），`go generate ./ent/...` 后 store/mock 同步、build 绿；并贴出确认存量账号无该值的查询。
   - 若实接：httptest mock `oauth2.googleapis.com/token`，测试证明 SA JSON → RS256 JWT → 换得 access_token → `injectAuth` Bearer；贴出测试名 + `go test` 输出。

5. **rank28 desktop/ide token 处置可证**：`grep -rn "desktop_client_token\|ide_plugin_token" apps/api/internal/modules/reverse_proxy/` 证明其最终状态（注释为 oauth_refresh 别名，或读时折叠），无「枚举存在但语义悬空无说明」的残留。

6. **rank13 credits 持久化可证**：测试证明 fetch 后 `Plan`/`CreditsRemaining` 等被写入账号 metadata（或合成 QuotaSnapshot）而非丢弃；贴出新增 codex `account_plan.*` fixture 测试名 + `go test` 输出；说明告警/历史下游如何读到（消费点）。

7. **rank38 Antigravity 配额可证**：测试（httptest mock antigravity quota 端点）证明 antigravity 账号 `FetchAccountQuota` 返回 `Supported=true` 并写 `AccountQuotaSnapshot`（修复前 `Supported=false`）；贴出 `grep` 证明 `antigravityPreset` 现含 `QuotaConfig` + 测试输出。

8. **rank6 channel-monitor worker 可证**：`ls apps/api/internal/workers/channel_monitor/` + 测试证明：(a) worker 选 `enabled` 且过期的 def 并跑、`RecordRun` 写 `Trigger="scheduled"`；(b) `enabled=false` 的 def 被跳过（自动与手动 `/run` 都 gate）；(c) 探针逻辑已抽进 `channel_monitors` 模块（`grep` 证明 worker 不再 import http 层）；贴出测试名 + 输出。

9. `cd apps/api && go test ./...` 全绿（贴出尾部）；`git status` 干净（除预期改动），`git diff --stat` 贴出。

或 35 turns 后停止并汇总剩余阻塞项（已完成项、各 class/字段最终处置、编译/测试状态、剩余阻塞项及原因）。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit（schema/OpenAPI 改动先行）：

1. **阶段一 — OpenAPI/schema 先行**：把 `risk_level` 加入 accounts Create/Update 请求体 schema（`packages/openapi/openapi.yaml`），`go generate`；评估 rank27 是否删 `runtime_class` 枚举值（先 grep 存量+问迁移）；rank38 的 `QuotaConfig` 是 preset 内配置无需改 schema。此阶段产出生成物 + build 绿。commit。

2. **阶段二 — rank7 TLS 绑定 UI**：account-form 加 enable 开关 + profile 下拉（写 `metadata.tls_profile`）+ metadata_hints 提示；渠道 `TlsPanel` 改本渠道聚合视图。前端 tsc/lint + 截图。commit。

3. **阶段三 — rank11 risk_level 入参**：accounts service + update handler 映射；account-form 加 select；补 scheduler penalty 命中测试。commit。

4. **阶段四 — rank27 / rank28 class 处置**：service_account_json 删枚举值（`go generate` + store/mock 同步）或实接 SA 签名器；desktop/ide token 注释/折叠为 oauth_refresh 别名；跑 batch7 的 allowlist⊆可签发集合 CI 断言确认仍绿。commit。

5. **阶段五 — rank13 credits 持久化**：把 plan/credits 持久化为 metadata 或合成 QuotaSnapshot；补 codex `account_plan.*` fixture 测试。commit。

6. **阶段六 — rank38 Antigravity QuotaConfig**：antigravityPreset 加 QuotaConfig；httptest mock 测试证明 Supported=true 写 snapshot。commit。

7. **阶段七 — rank6 channel-monitor worker**：探针逻辑从 http 层抽进 `channel_monitors` 模块 service；新增 `workers/channel_monitor` 调度 worker（镜像 health_probe）；自动+手动路径都 gate enabled、自动写 Trigger=scheduled；补 worker 测试。跑全量 `go test`。commit。

每阶段一个 commit，message 末尾加：
```
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
```
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch11`）若干语义化 commit + 具体产物：
- 前端：account-form-dialog TLS profile 字段 + risk_level select；渠道 TlsPanel 本渠道聚合视图。
- 后端：accounts Create/Update 的 risk_level 映射；rank27/28 class 的最终处置（删枚举值+迁移注释 / 实接签名器 / 别名注释）；quota plan/credits 持久化；antigravityPreset QuotaConfig；新 `workers/channel_monitor` 调度 worker + 探针逻辑下沉到 `channel_monitors` 模块。
- 新测试全 passing：scheduler risk penalty 命中、codex account_plan fixture、antigravity 配额 Supported=true、channel-monitor worker（enabled gate + scheduled trigger）、（若实接）SA JWT→token→Bearer。
- progress.txt 收尾（列每个 rank 的最终处置、证据位置、各 success_criteria 对应状态、未做的 out-of-scope 项归属哪批）。
</artifact>

<guardrails>
绝不：删除/改写既有测试让其通过；hardcode 让测试/探针通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证；在测试里打真实上游（一律 httptest mock）；保留任何「字段/枚举在但不消费」的摆设残留还假装修好；**伪造未实现能力——摆设的两种合法处置只有「真接通到下游消费点」或「诚实下架（删字段/删枚举值/隐藏 UI）」，绝不假装接通**（如 rank27 不实接就别假装 Vertex 已接、rank38 mock 不通就别假装 antigravity 配额已点亮）。
先问我：删除 `runtime_class` 枚举值涉及的存量账号数据迁移（不可逆）；破坏现有 OpenAPI 响应结构；删除现有顶层路由文件；删 ent 枚举值的存量迁移；向真实上游发起需真实凭证的调用（探配额/换 token）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 证据在哪 / 各 rank 最终处置决定 / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：已完成阶段、各 rank（7/11/27/28/13/38/6）的当前处置状态、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根 `/home/senran/Desktop/SRapi` 为基准；已抽查核实，行号来自审计多数可信）：
- TLS 绑定（rank7）：`apps/web/src/components/admin/account-form-dialog.tsx`、`apps/web/src/lib/admin-account-form.ts`（现 grep `tls_profile`=0）；`apps/web/src/app/admin/channels/[id]/page.tsx:542-565`（TlsPanel 渲染全量 `profiles.map`，需改本渠道聚合）；消费点 `apps/api/internal/httpserver/runtime_state.go:483`（`SetNamedProfileExpander` 已 wire）+ `apps/api/internal/modules/reverse_proxy/service/egress_profile.go:39`（`SetNamedProfileExpander` 定义）。
- risk_level（rank11）：`apps/api/internal/modules/accounts/contract/contract.go:71`（`RiskLevel *string` 现仅在实体 struct）、`:197-220`（Create/Update 请求体附近）；scheduler penalty 消费点 `apps/api/internal/modules/scheduler/service/service.go`（riskPenalty high=0.15/medium=0.05）；回显点 `apps/api/internal/httpserver/runtime_api_mapping.go:414`。
- service_account_json（rank27）：`apps/api/internal/modules/reverse_proxy/service/service.go:701-723`（`injectAuth`，无 SA case，落 default 塞 Bearer）；`apps/api/ent/schema/provideraccount.go`（`runtime_class` 枚举）；preset/`registry_test` 已强制排除。
- desktop/ide token（rank28）：`apps/api/internal/modules/reverse_proxy/service/service.go:709`（与 `oauth_refresh`/`oauth_device_code` 同 case 共用 Bearer）；preset 不列出（registry_test 守门）。
- credits 持久化（rank13）：`apps/api/internal/modules/provider_adapters/service/quota_fetch.go:77-80`（解析 Plan/CreditsRemaining/Used/Limit）、`:114`（`QuotaConfigured`）；持久化路径 `apps/api/internal/workers/quota_refresh/worker.go:264-291`（现只写 QuotaSignals）；handler 返回点 `apps/api/internal/httpserver/runtime_admin_quota_fetch_handlers.go`（返回即丢）；测试 `quota_fetch_test.go`（只测 anthropic，需补 codex account_plan fixture）。
- Antigravity 配额（rank38）：`apps/api/internal/modules/providers/preset/registry.go:73`（`QuotaConfig map[string]string` preset 字段）、`:258`（`antigravityPreset()` 无 QuotaConfig）、`:234`（`geminiPreset()` 参考）；`quota_fetch.go:114`（QuotaConfigured=false → Supported=false）。
- channel-monitor worker（rank6）：`apps/api/internal/httpserver/runtime_admin_channel_monitor_run.go:99`（硬编码 `Trigger: "manual"`，无 `if !def.Enabled`）；`apps/api/internal/modules/channel_monitors/service/service.go`（探针逻辑下沉目标）；镜像模式参考 `apps/api/internal/workers/health_probe/` 与 `apps/api/internal/workers/quota_refresh/worker.go`；新 worker 落 `apps/api/internal/workers/channel_monitor/`。
- 守门测试：`apps/api/internal/architecture/architecture_test.go`（跨模块 contract-only import、单文件 ≤2200）、`apps/api/internal/codequality/code_quality_test.go`（单函数 ≤210）。
- OpenAPI：`packages/openapi/openapi.yaml`（risk_level 入参先改这里再生成）。

sub2api 仅作能力对照不抄代码。本批相关参照点：sub2api 的 Account.platform+type+credentials 正交建模（account 上独立的 risk/proxy/tls 绑定字段）；service_account（Vertex RS256 JWT 换 Google token）；antigravity/各 provider 的 quota 端点与 forbidden/validation/violation 信号分类；channel/health monitor 的后台周期调度 worker 模式。
</references>
