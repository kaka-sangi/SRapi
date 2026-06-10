# Goal：端用户登录摆设清除 + 平台自助/配置补全（第十四批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查发现，SRapi 的「端用户登录」与「平台自助/配置」两片仍有成片摆设与缺口——wechat/dingtalk 登录只走通用 Bearer-userinfo 通路驱动不了真协议（按钮在却登不进）、OAuth 老用户每次仍要重输密码（identity 只 bind 去重不认证返回）、无邮箱验证码 passwordless 闭环；约 12 个 settings 功能开关（PaymentsEnabled/SubscriptionPlansEnabled/InvitationRebateEnabled/ChannelMonitoringEnabled/EnabledChannels/DefaultGroup/UserSelfDeleteEnabled/Backup.*）存而不读（开关拨了无任何强制点）；站点品牌/协议（SiteName/Logo/Version/CustomMenus/UserAgreement/PrivacyPolicy）只在 admin 内存读、无公开端点、前台不渲染；用户自定义属性 Required/Enabled 在用户生命周期从不强制（只有 admin 能填）；console 写操作（下单/建 Key/兑换码）无幂等可重复提交；captcha 仅读 env 无 admin UI；auditlog 无保留/清理 worker 无限增长。这些都是「业务能力假象」：UI 给了入口/开关，后端不兑现。
> 前置依赖：rank54 微信支付 OAuth 取 openid 依赖 rank51 wechat 登录能力（本批内部排序：先 wechat 再微信支付）；rank8 self-delete 与 batch9 RBAC 解耦可独立；整体建议接 batch12（支付上游真实性）/batch13（计费维度）之后落地，因 PaymentsEnabled 强制点与支付 handler 相邻。
> 关联：本批是 batch7 文末「附录：端用户登录认证摆设（候选 batch8，本批不做）」的兑现——上游认证矩阵（batch7）与端用户登录（本批）是两张正交的表，勿混；feature-flag 强制点会触及 batch8（affiliate/promo 的 InvitationRebateEnabled）、batch11（ChannelMonitoringEnabled 喂 channel-monitor worker）、batch12（PaymentsEnabled 喂支付 handler）、batch13（SubscriptionPlansEnabled 喂订阅端点）已落地的能力，须只「加门控、不破坏」；架构清理（batch15）在本批之后，本批落到大文件须留意行数红线（见 constraints）。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个对标 sub2api 的 AI 网关/计费平台。
技术栈与约定：
- 后端 Go + ent ORM，OpenAPI-first：spec 在 `packages/openapi/openapi.yaml`（先改 spec 再 `go generate` 生成 DTO/SDK，绝不手改生成物）；ent schema 在 `apps/api/ent/schema/`；业务在 `apps/api/internal/modules/<域>/{contract,service,store}`；HTTP 在 `apps/api/internal/httpserver/`；持久化在 `apps/api/internal/persistence/entstore/<域>/`；后台 worker 在 `apps/api/internal/workers/<名>/`；运行时配置在 `apps/api/internal/config/config.go`。
- 前端 Next.js + TypeScript（`apps/web`）：路由在 `app/`，组件在 `components/`，导航在 `components/layout/nav-items.ts`，API 客户端在 `lib/`。
- 本地开发：`make dev-up` 起后端 + `npm run dev` 起前端；登录 admin@srapi.local / Admin1234（端用户测试需自助注册一个普通用户）。
- 改动须过：后端 `cd apps/api && go build ./... && go vet ./...`；前端 `cd apps/web && npx tsc --noEmit && npm run lint`。
- 架构红线（守门测试 `apps/api/internal/architecture/architecture_test.go` + `apps/api/internal/codequality/code_quality_test.go`）：模块生产码只能 import 别模块的 contract 层（白名单）；contract 层禁止 import Ent/生成的 OpenAPI DTO/HTTP server 包；worker 只能 import 模块 contract/service；单文件硬上限 2200 行（runtime_* 文件），单函数硬上限 210 行；全部 gofmt 必须过。本批多处落点（runtime_user_handlers.go、runtime_oauth_handlers.go、server.go、admin_control service）已较大，新增逻辑优先开新文件 / 抽小函数，不得把任一文件推过 2200 或任一函数过 210。
- 凭证 AES-GCM 加密不得改明文（端用户登录 OAuth refresh token、wechat/dingtalk 凭据沿用现加密路径）。
- 绝不抄 sub2api 源码，只借鉴能力与协议思路（wechat sns/oauth2、dingtalk userAccessToken 等的字段/端点形态可参照，代码自写）。
本批主题侧重「摆设清除」：每一项要么真接通（写明接通的消费点在哪），要么诚实下架（写明下架范围与残留清理），不留「UI 有但后端不兑现」的中间态。
</role_and_context>

<objective>
目标（可衡量最终态）：端用户登录的每个可见入口都名实相符（wechat/dingtalk 要么真登进、要么按钮已下架；OAuth 老用户免密直发 session；邮箱验证码注册/登录闭环可走通），且所有「存而不读」的平台配置都接上真实强制/消费点（约 12 个 feature flag 各有门控、公开 site-config 驱动前台品牌/协议、/me/attributes 让用户自助填写且注册强制必填、console 写操作幂等去重、captcha 可 UI 配置、auditlog 按保留天数清理）。完成后，运营在 settings 拨任一开关都有可观察后果，端用户看到的每个登录按钮都能用。
动机：摆设 = 业务能力假象与信任风险。登录页摆着 wechat/dingtalk 按钮却登不进，是直接的用户信任损伤；PaymentsEnabled 等开关存而不读，意味着运营以为「关了支付」实际仍在收单（资金面风险）；console 写操作无幂等，下单/兑换可被重复提交（资金面风险）；用户属性 Required 从不强制、无 /me 入口，是合规/资料完整性的空壳。这些都必须要么真接通、要么诚实下架。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元；凡「二选一」必须落定到无中间态——接通或下架，不留摆设）：

1. **rank51 wechat/dingtalk 登录处置（摆设，二选一必落定）**：现状只走通用 Bearer-userinfo 通路（`apps/api/internal/modules/auth/service/oauth_start.go` + `pending_oauth.go` 的通用 OAuth2/OIDC 流水线），驱动不了真微信（`sns/oauth2/access_token` + query userinfo + code2Session + unionid）/真钉钉（`/v1.0/oauth2/userAccessToken` → `/v1.0/contact/users/me`）的非标准协议。
   - (a) **实接专用 handler**：为 wechat/dingtalk 写专用认证路径（自写协议适配，httptest mock 上游），完成 code→token→userinfo→upsert 用户/identity；或
   - (b) **诚实下架**：从登录 provider 白名单 + 前端 `apps/web/src/app/login/page.tsx` 的动态按钮源移除 wechat/dingtalk，并在 docs 端用户登录矩阵标注 deliberate non-goal。
   默认按产品意愿选 (b) 止血（工作量 L，真协议复杂）；若选 (a) 则须 mock 上游测试证明登进。**无论哪选，验收点是登录页不再出现「点了登不进」的按钮。**

2. **rank53 OAuth 返回用户免密快速通道（摆设，必接通）**：现状 OAuth 老用户靠 FindByEmail 匹配，provider-subject 绑定只用于 bind 去重从不认证返回用户（逻辑在 `apps/api/internal/modules/auth/service/pending_oauth.go`，audit 旧引 `auth/service.go:683` 已失效，真实落点见 references），导致老 OAuth 用户每次仍要重输密码。
   - 加「已绑定 identity → 凭 provider_subject(hash) 直发 session」快速路径：OAuth 回调时先按 ProviderSubjectHash 查已绑定 identity，命中则直发 session（跳过密码），未命中再回落现有 FindByEmail/bind 流程。**接通消费点：`runtime_oauth_handlers.go` 回调处。**

3. **rank52 邮箱验证码注册/登录闭环（功能缺失，必补）**：现状仅有 verification + reset（`apps/api/internal/modules/auth/service/email_verification.go`），无 passwordless 验证码闭环。
   - 补 SendVerifyCode 发送/校验端点 + 注册/登录流程消费 verify_code 实现 passwordless 注册/登录（复用现有邮件发送通道与 email_verification 模式，先改 OpenAPI 再生成）。

4. **rank54 微信支付 OAuth 取 openid（功能缺失，依赖 rank51；按产品需要）**：无微信内 H5 支付取 openid 的 start+callback 路由/页面。
   - 仅当做微信内 H5 支付且 rank51 选了 (a) 实接 wechat 时补 start+callback+前端，复用 wechat 能力产出 openid 喂 batch12 的 wechat jsapi payer_openid 入参；若 rank51 选 (b) 下架则本项**诚实下架并在 docs 标注「依赖 wechat 登录，wechat 未实接故本项不做」**（非永久搁置——随 wechat 实接一并解锁，由本批 docs 记录解锁条件）。

5. **rank8 ~12 个 feature flag 接强制点（摆设，逐项必接或删）**：以下开关均存而不读，逐项接强制点或删字段（删需先问）：
   - `PaymentsEnabled`：支付 handler（`handleListPaymentMethods`/`handleCreatePaymentOrder`，`runtime_user_handlers.go`）入口校验，关闭则 403/明确拒绝（与 batch12 支付链协同，只加门控）。
   - `SubscriptionPlansEnabled`：订阅相关端点校验。
   - `InvitationRebateEnabled`：affiliate 入账前校验（与 batch8 的 AccrueRebate 协同，关闭则不入账）。
   - `ChannelMonitoringEnabled`：channel-monitor worker（batch11 已建）消费前校验，关闭则 worker 不跑。
   - `EnabledChannels`：渠道过滤消费点（账号/provider 列表或调度候选按它过滤）。
   - `Users.DefaultGroup`：注册流程按它绑定新用户分组。
   - `Users.UserSelfDeleteEnabled`：新增 `DELETE /api/v1/me` 受此开关门控（自助注销）。
   - `Backup.Enabled/RetentionDays/LastBackupAt`：实现备份 worker（读 Enabled / 定时 / 写 LastBackupAt / 按 RetentionDays 清理旧备份），镜像 `apps/api/internal/workers/retention` 模式。
   每项要么真接强制点、要么删该字段（删字段属破坏性，先问）。

6. **rank9 公开 site-config 端点（摆设，必接通或删字段）**：SiteName/LogoURL/VersionLabel/CustomMenus/UserAgreement/PrivacyPolicy 只在 admin 内存读，前台不渲染。
   - 新增公开 `GET /api/v1/site-config`（无需鉴权，返回 site_name/logo/version/custom_menus/agreement/privacy，先改 OpenAPI），前端 shell（masthead/footer）+ 登录注册页消费（渲染品牌 + 展示用户协议/隐私政策链接）。**接通消费点：`apps/web` shell + 登录注册页。**

7. **rank10 /me/attributes 用户自助 + 注册强制（摆设，必接通）**：Definition.Required/Enabled/DisplayOrder 在用户生命周期从不强制，只有 admin PUT 能写值（`apps/api/internal/modules/userattributes/service/service.go` 的 `validateValueForType`/Required 仅 admin set-value 触发）。
   - 新增 `GET/PUT /api/v1/me/attributes` 让用户自填（先改 OpenAPI）；注册后对 Required 且 Enabled 的属性强制（注册校验或登录后 onboarding 拦截）；OAuth 登录同步时 upsert 属性值（与 rank53 协同）。前端用户中心加属性填写区。**接通消费点：/me 端点 + 注册校验 + 用户中心 UI。**

8. **rank55 console 写操作幂等（功能缺失，必补）**：现幂等仅包裹 4 个网关端点（`server.go` 的 `withGatewayIdempotency`），POST `/payment/orders`、POST `/api-keys`、兑换码等 console 写操作无幂等保护可重复提交。
   - 提供 `withConsoleIdempotency`（基于 Idempotency-Key header 或 user+action+body-hash），包裹支付下单/兑换码兑换/Key 创建等不可重复写操作（复用现有 idempotency store + `idempotency_cleanup` worker）。

9. **rank56 captcha admin UI 配置（功能缺失，必补）**：captcha 仅从 env 读取（`apps/api/internal/config/config.go:130` CaptchaConfig，`getEnv("CAPTCHA_*")`），admin settings 页无 captcha 段。
   - captcha 配置纳入 admin_control settings（security tab），runtime 从 settings 读（settings 覆盖 env），提供 test 验证按钮。

10. **rank57 auditlog 保留 + 清理 worker（功能缺失，必补）**：无 auditlog 保留配置或清理 worker，审计表无限增长（已有 usage_log/system_log 清理但缺 auditlog）。
    - 新增 auditlog 保留天数设置 + 清理逻辑（复用 `apps/api/internal/workers/retention` 模式覆盖 auditlog 表）。

11. **rank58 SecuritySecret 集中密钥保险库（功能缺失，低优先，必给出结论）**：无 security_secret 集中加密存取表，敏感值散落 `setting.value_ciphertext` 与账号 metadata。
    - 评估是否需集中 SecuritySecret 表：`Setting.value_ciphertext` 已覆盖大部分场景。**本批必须给出明确结论**——要么实现最小可用的 SecuritySecret 表 + CRUD，要么在 docs 写明「现 value_ciphertext 已满足，刻意不引入独立保险库」并附理由（非搁置，是明确的产品决策记录）。

明确不做（out-of-scope，均为「由其它批次涵盖」或「本批刻意不碰」，非永久搁置）：
- 上游认证矩阵（service_account_json / oauth_refresh / web_session_cookie 等 provider 侧认证）——由 batch7 + batch11 涵盖，本批只做端用户登录，两表勿混。
- channel-monitor worker 本体、Antigravity 配额、TLS profile 绑定——由 batch11 涵盖；本批仅给 `ChannelMonitoringEnabled` 门控（消费 batch11 已建的 worker）。
- 支付上游真实退款/对账/手续费/wechat jsapi payer_openid 入参本体——由 batch12 涵盖；本批仅给 `PaymentsEnabled` 门控并产出 rank54 openid 喂给 batch12 入参。
- 计费维度（daily/weekly 配额、service_tier、cache 分档等）——由 batch13 涵盖；本批仅给 `SubscriptionPlansEnabled` 门控。
- 架构拆文件/消重/补测——由 batch15 涵盖；本批只须不推任何文件过 2200 行、不新增跨包重复 helper。
- RBAC 细粒度权限——由 batch9 涵盖；本批 self-delete 端点用现有 `requireUserSession` 即可，不依赖权限目录。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64（rank8 PaymentsEnabled 门控只拦请求，不碰金额运算）。
- 不得破坏 batch1-13 已落地能力：feature-flag 门控只「加判断、关则拒绝」，不改既有支付/订阅/affiliate/channel-monitor/计费的正常路径行为；OAuth 免密快速通道只在「identity 命中」时短路，未命中须完整回落现有 FindByEmail/bind 流程。
- 凭证 AES-GCM 加密不得改明文：端用户 OAuth refresh token、wechat/dingtalk 凭据、captcha secret、SecuritySecret（若建）均走现加密路径。
- OpenAPI 兼容：新增端点（/site-config、/me/attributes、/me DELETE、邮箱验证码、wechat/微信支付 OAuth）走新增路径，不破坏现有响应结构；改任一现有响应结构须先问。
- 不得修改与本任务无关的 `*_test.go`；新增能力配新测试；拉取上游（wechat/dingtalk/邮件/captcha verify）一律 httptest mock，绝不打真实上游。
- ent schema 若改（如新增 Backup.LastBackupAt 持久列、SecuritySecret 表、auditlog 保留字段）须 `go generate ./ent/...` 并同步 store/mock（含 memory store 的 DeletedAt 过滤、Store-mock codegen 已知坑——生成后核对 mock 方法齐全）。
- 删枚举值 / 不可逆数据迁移 / 删既有字段（如 rank8 选「删 flag」而非「接强制点」）须先问。
- 架构红线：本批落点（runtime_user_handlers.go、runtime_oauth_handlers.go、server.go、admin_control/service、auth/service 各文件）新增逻辑优先开新文件或抽小函数，不得把任一文件推过 2200 行、任一函数过 210 行；跨模块只 import contract。
其他约束：新增依赖（如 captcha 校验客户端、wechat/dingtalk SDK——优先自写 http 调用避免 SDK）须先说明理由。
</constraints>

<success_criteria>
完成需同时满足（每条在输出里给出 agent 自己能贴出的可观察证据，评估器不自跑命令）：
1. `cd apps/api && go build ./... && go vet ./...` 退出码 0（贴出尾部）；`cd apps/web && npx tsc --noEmit && npm run lint` 通过（贴出尾部）。
2. **rank51 wechat/dingtalk 处置可证**：若实接——httptest mock wechat `sns/oauth2`+userinfo 与 dingtalk `userAccessToken`+`contact/users/me`，测试证明 code→token→userinfo→upsert 登进（贴测试名+输出）；若下架——grep 证明登录 provider 白名单 + `apps/web/src/app/login/page.tsx` 按钮源已无 wechat/dingtalk，chrome-devtools 截图登录页不再有这两个按钮。
3. **rank53 OAuth 免密可证**：测试证明已绑定 identity 的 OAuth 老用户回调凭 ProviderSubjectHash 直发 session（不经密码），未绑定时仍回落原流程（贴测试名+输出）；chrome-devtools 截图老用户 OAuth 一键登入。
4. **rank52 邮箱验证码可证**：测试证明 SendVerifyCode → 校验 verify_code → 注册/登录成功的 passwordless 闭环（mock 邮件发送，贴测试输出）。
5. **rank8 feature flag 可证（核心）**：测试证明关闭 `PaymentsEnabled` 后 `handleCreatePaymentOrder` 返回 403/拒绝、开启则正常；新用户注册被绑定到 `Users.DefaultGroup`；`DELETE /api/v1/me` 在 `UserSelfDeleteEnabled=false` 时被拒、true 时生效；备份 worker 测试证明读 Enabled、写 LastBackupAt、按 RetentionDays 清理（贴各自测试名+输出）；其余 flag（SubscriptionPlansEnabled/InvitationRebateEnabled/ChannelMonitoringEnabled/EnabledChannels）grep 证明已有消费点（贴 grep 结果，每个 flag 在生产码除 contract/store/openapi.gen 外 ≥1 消费者）。
6. **rank9 site-config 可证**：`curl /api/v1/site-config`（或测试）返回 site_name/logo/agreement 等字段；chrome-devtools 截图前台 shell 渲染品牌 + 登录页展示协议链接。
7. **rank10 /me/attributes 可证**：测试证明 `GET/PUT /api/v1/me/attributes` 可读写、注册时 Required+Enabled 属性缺失被拒（贴测试输出）；chrome-devtools 截图用户中心属性填写区。
8. **rank55 幂等可证**：测试证明同一 Idempotency-Key（或同 user+action+body-hash）重复 POST 下单/建 Key 只生效一次（贴测试输出）。
9. **rank56 captcha UI 可证**：chrome-devtools 截图 admin settings security tab 的 captcha 配置段 + test 按钮；测试/grep 证明 runtime 从 settings 读取 captcha 配置。
10. **rank57 auditlog 保留可证**：测试证明清理逻辑按保留天数删除过期 auditlog（贴测试输出）。
11. **rank58 SecuritySecret 结论可证**：要么贴出新表 + CRUD 测试，要么贴出 docs 中「刻意不做」的明确决策记录（含理由）。
12. **零摆设回归可证**：docs 下产出/更新「端用户登录矩阵」+「feature-flag 强制点对照表」，每项标注 ✅接通 / ⬜下架 / — 不适用，与代码现状一致（贴文档片段）。
13. `cd apps/api && go test ./...` 全绿（贴尾部）；`git status` 干净（除预期改动），`git diff --stat` 贴出。
或 40 turns 后停止并汇总剩余阻塞项（本批跨域子项多，给较宽预算）。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit；schema/OpenAPI 改动先行：
1. 阶段一（OpenAPI + schema 先行）：在 `packages/openapi/openapi.yaml` 一次性加入新端点契约（/site-config、/me/attributes、DELETE /me、邮箱验证码、wechat/微信支付 OAuth 若实接）+ 必要 ent schema 改动（Backup.LastBackupAt 持久列、auditlog 保留、SecuritySecret 若建）；`go generate`（OpenAPI + `./ent/...`）+ 同步 store/mock。commit。
2. 阶段二（端用户登录）：rank51 wechat/dingtalk 处置（实接或下架，落定无中间态）→ rank53 OAuth 免密快速通道 → rank52 邮箱验证码闭环 → rank54 微信支付 OAuth（依赖 rank51 实接，否则随之下架）。每子项独立 commit + 测试。
3. 阶段三（feature flags 接强制）：逐个 flag 接消费点（PaymentsEnabled → SubscriptionPlansEnabled → InvitationRebateEnabled → ChannelMonitoringEnabled → EnabledChannels → DefaultGroup → self-delete 端点 → 备份 worker）。每个 flag 一个 commit + 门控测试。
4. 阶段四（平台自助/配置）：rank9 site-config 公开端点 + 前端消费 → rank10 /me/attributes + 注册强制 + 用户中心 UI → rank55 console 幂等 → rank56 captcha UI → rank57 auditlog 保留 → rank58 SecuritySecret 结论。每子项独立 commit + 测试/截图。
5. 阶段五（文档收尾）：产出端用户登录矩阵 + feature-flag 对照表，更新 progress.txt。commit。
每阶段每 commit message 末尾加：
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch14`）若干语义化 commit + 各端用户登录项的处置（实接/下架）+ 各 feature flag 的强制点 + 公开 site-config 端点 + /me/attributes + DELETE /me + 邮箱验证码闭环 + console 幂等中间件 + captcha admin UI + 备份/auditlog 清理 worker + SecuritySecret 结论 + 全部新测试（passing）+ docs 端用户登录矩阵与 feature-flag 对照表 + progress.txt 收尾（列每子项最终处置「接通/下架」、接通的消费点位置、证据位置、未做的 out-of-scope 项及其归属批次）。
</artifact>

<guardrails>
绝不：删除/改写既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；退回明文凭证（端用户 token/captcha secret/wechat 凭据须加密）；伪造未实现能力——摆设的两种合法处置只有「真接通」（写明消费点）或「诚实下架」（写明下架范围 + 残留清理），绝不假装接通（例：rank51 不实接就别留 wechat 按钮假装能登；rank54 wechat 未接就别假装微信支付可用；任一 feature flag 别只加字段不接消费点）。
先问我：破坏现有 OpenAPI 响应结构；删除现有顶层路由文件；删 ent 枚举值的存量迁移；任何不可逆数据迁移（如 auditlog 清理首次上线的历史数据策略）；rank8 选「删 flag 字段」而非接强制点；向真实上游（wechat/dingtalk/邮件/captcha verify）发起需真凭证的调用（一律 httptest mock）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（含每个子项的最终处置决定「接通+消费点 / 下架+范围」+ 证据位置 + 下一步）+ git commit。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 40 turns 仍未达成，停下汇总：每个子项的当前处置状态（接通/下架/未动）、各 feature-flag 强制点红/绿、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；标注 * 者为本次抽查/修正过的真实路径）：
- 端用户登录：
  - OAuth 通用流水线与 provider-subject 绑定 *`apps/api/internal/modules/auth/service/pending_oauth.go`（PendingOAuth* 流程，ProviderSubjectHash 在 :44/:106；audit 旧引 `auth/service.go:683` 已失效——`auth/service.go` 仅 327 行不含该方法，identity 绑定逻辑在 pending_oauth.go）；OAuth start 在 *`apps/api/internal/modules/auth/service/oauth_start.go`。
  - OAuth 回调 handler *`apps/api/internal/httpserver/runtime_oauth_handlers.go`（rank53 免密快速通道落点；wechat/dingtalk 引用在此文件）。
  - 邮箱验证 *`apps/api/internal/modules/auth/service/email_verification.go`（156 行）+ `password_reset.go`（rank52 复用模式）。
  - 登录页 *`apps/web/src/app/login/page.tsx`、注册页 *`apps/web/src/app/auth/register/page.tsx`（动态登录按钮源 + 协议展示）。
- feature flags：
  - 契约 *`apps/api/internal/modules/admin_control/contract/contract.go`：Features 在 :85-90（EnabledChannels/ChannelMonitoringEnabled/InvitationRebateEnabled/PaymentsEnabled）、SubscriptionPlansEnabled :138、Users.DefaultGroup :114 / UserSelfDeleteEnabled :115、Backup :46/:157（LastBackupAt :159）；General(site-config) :75-79、Agreement :81-84。
  - 支付 handler `apps/api/internal/httpserver/runtime_user_handlers.go`（handleListPaymentMethods/handleCreatePaymentOrder 门控；audit 引 :131-132,949,971,996）。
  - 路由注册 *`apps/api/internal/httpserver/server.go`：网关幂等 `withGatewayIdempotency` 在 :595-603 区段（rank55 新增 withConsoleIdempotency 包裹支付下单/Key/兑换码）；user-attribute 路由现全为 `/admin/*`（registerCapabilityAdminRoutes，rank10 新增 /me/attributes）。
  - 备份/清理 worker 参照 *`apps/api/internal/workers/retention`（已有；rank8 备份 worker、rank57 auditlog 清理镜像此模式）；现有 worker 清单含 idempotency_cleanup（rank55 复用其 store）。
- 平台自助/配置：
  - 用户属性 service *`apps/api/internal/modules/userattributes/service/service.go`（validateValueForType :115、Required 校验 :96，仅 admin set-value 触发；rank10 接 /me + 注册强制）。
  - captcha 配置 *`apps/api/internal/config/config.go`：CaptchaConfig 结构 :130，env 读取在 :324-330（Enabled/Provider/SecretKey/SiteKey/VerifyURL）；rank56 纳入 admin_control settings。
  - admin settings 前端 *`apps/web/src/app/admin/settings/page.tsx`（rank56 captcha 段 + rank8 各 flag UI 已有则只接强制不改 UI）。
  - OpenAPI spec `packages/openapi/openapi.yaml`（新端点先改此处再生成）。
- sub2api 仅作能力对照不抄代码：
  - wechat 登录（`sns/oauth2/access_token` + query userinfo + code2Session + unionid）、dingtalk 登录（`/v1.0/oauth2/userAccessToken` → `/v1.0/contact/users/me` + 邮箱补全）的协议形态；SendVerifyCode passwordless 闭环；OAuth subject 路由（已绑定 subject 直发 session）；微信支付 OAuth 取 openid 的 start+callback 形态；feature-flag 在各业务入口的门控位置。以上均仅参照能力与端点字段，代码自写。
</references>
