# Goal：RBAC 真实化 + 风控/内容安全接强制点（第九批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：8 域对抗性核查发现三大**安全摆设**集中暴露——(1) RBAC 细粒度权限系统是整片摆设：roles 页能给角色分配任意权限字符串，但后端只定义唯一权限常量 `payment_order:read`，215 处 `requireAdminSession`/`requireAdminWriteSession` 只认 owner/admin 二档，`requireAdminPermission` 仅 2 处调用且对 owner/admin 短路放行，导致 `RoleOperator` 角色进不了任何 admin 端点、前端权限框是永不被后端校验的自由文本；(2) 风控规则整页摆设：`enabled/mode/maxFailedRequestsPerMinute/maxCostPerDay/cooldownSeconds/blockedCountries/blockedIPs` 全套表单存在，但 `GetRiskConfig` 从不在任何网关/认证路径被读取或强制，黑名单不拦截、阈值不限速、`enforce` 无效果，且无代码写 `RiskControlLog` 故 `RiskStatus` 恒 0；(3) 内容安全半摆设：硬编码 always-on（`New()` 无 Config 入参）只脱敏不拦截，prompt-injection 命中只记 finding 从不 block，信用卡正则 `(?:[0-9][ -]?){13,19}` 无 Luhn 校验，把任意 13-19 位长数字当卡号静默替换并清空 RawBody，运营无从感知或关闭。三者本质相同：后端表单/字段建好但无强制点，造成「已配置安全策略」的业务能力假象——这是直接的安全漏洞，必须要么真接通、要么诚实下架。
> 前置依赖：无强外部依赖；与 batch8（分销/促销资金链）相互独立，可并行。rank25（RoleOperator 绑权限组）依赖本批 rank2 的权限目录先落地。
> 关联：是 batch1-7 已落地能力之上的安全侧补强；与 batch8 互不冲突，建议接 batch8 之后开工；与 batch10/14 的 admin 端点无重叠。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api），管理面比 sub2api 更宽。
技术栈：
- 后端 Go + ent ORM（apps/api，OpenAPI-first）。schema 在 `apps/api/ent/schema/`，业务在 `apps/api/internal/modules/<域>/{contract,service,store}`，HTTP 在 `apps/api/internal/httpserver/`。
- 前端 Next.js + TypeScript（apps/web，路由在 `app/`，组件在 `components/`，导航在 `components/layout/nav-items.ts`）。
本地开发：`make dev-up` + `npm run dev`；登录 admin@srapi.local / Admin1234。
所有改动须通过 `cd apps/api && go build ./... && go vet ./...` 与前端 `npm run -w apps/web tsc`（或等价 typecheck）+ lint。OpenAPI-first：先改 `packages/openapi/openapi.yaml` 再生成。
架构红线（由 `apps/api/internal/architecture/architecture_test.go` 与 `apps/api/internal/codequality/code_quality_test.go` 守门）：模块生产码只能 import 别模块的 contract（白名单），contract 层禁止 import Ent/生成的 OpenAPI DTO/HTTP server；单文件 ≤2200 行（runtime_* 文件，core 仅余约 44 行余量——本批改动须警惕触线，必要时把新逻辑放进新文件而非堆进 `runtime_gateway_core.go`）；函数 ≤210 行；gofmt 必须通过。
凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
本批强调点：权限改造是「机械但量大」的批量替换（215 处），务必分资源批次替换并回归；风控/内容安全是「把已建好的字段接到真实强制点」，强制点必须落在网关准入热路径 `prepareGatewayAdmissionWithOptions` 与认证路径上，且不破坏 batch1-7 已落地的网关行为。
</role_and_context>

<objective>
目标（可衡量最终态）：三大安全摆设全部从「字段/表单假象」升级为「名实相符」——(a) RBAC 走真实细粒度权限目录：约 215 处二档鉴权改为按「资源:动作」权限校验，`RoleOperator` 拥有真实可达的权限组（或显式声明二档并诚实下架权限分配 UI + 删 RoleOperator）；(b) 风控配置在网关准入与认证路径被真实读取强制：IP/国家黑名单拦截、失败次数/日成本滑窗判定，`enforce` 拒绝并写 `RiskControlLog`（`RiskStatus` 不再恒 0），`monitor` 只记日志；(c) 内容安全从硬编码 always-on 升级为可配置（开关/模式/自定义关键词/按模型范围），prompt-injection 支持 `block` 动作，信用卡命中加 Luhn 校验避免误伤，并产出可查的审核日志。
动机：摆设安全 = 安全漏洞 + 信任崩塌。运营在 roles 页给 operator 配了一堆权限以为限权了，实际后端任意 admin 即全放行；在风控页拉黑了 IP/设了日成本上限以为防住了滥用，实际网关从不读这些配置；内容安全把客户的正常 13-19 位订单号/序列号当信用卡静默改写还清空请求体，运营却无法关闭也无从感知。这三者都让系统对外宣称「有安全策略」却实际敞口，是最该优先清除的业务能力假象。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个子目标；每个「摆设」子目标必须落定到「真接通」或「诚实下架」，不留中间态）：

1. **rank2 — RBAC 细粒度权限目录（核心，L）**：
   - 在 `apps/api/internal/modules/users/contract/contract.go`（现仅 `PermissionPaymentOrderRead = "payment_order:read"`，:31-34）定义完整的权限目录常量集（`资源:动作`，如 `account:read`/`account:write`/`pricing_rule:write`/`scheduler_strategy:write`/`risk_control:write`/`role:write` 等，覆盖现有 admin 端点的资源面）。
   - 把 `apps/api/internal/httpserver/` 中约 215 处 `requireAdminSession`/`requireAdminWriteSession`（统计已核实 = 215）按所属资源批量替换为 `requireAdminPermission(r, <对应权限常量>)`；分资源分批替换并各自回归。
   - `requireAdminPermission`（`runtime_http_helpers.go:281-297`）当前对 owner/admin **短路放行**（:286-289）——去掉短路或改为可配置（owner 始终全权是合理设计，但 admin 短路使权限目录形同虚设，须改为 admin 也走权限集合，或保留 owner 短路、admin 不短路）。
   - 前端 `apps/web/src/app/admin/roles/page.tsx:67-72` 的 permissions 从自由文本 tags 改为受控多选（数据源 = 权限目录，建议后端暴露一个 `GET /api/v1/admin/permission-catalog` 或在角色相关响应里内联目录）。
   - 给内置角色绑定真实权限组（owner=全权、admin=除 owner-only 外全部、operator=只读+受限写）。built-in 角色受保护规则沿用既有（不可删 owner/admin）。
   - **二选一并落定**：若评估后认定产品只需 owner/admin 二档，则**诚实下架**——移除 roles 页的权限分配 UI、删 `RoleOperator`（走 ent 枚举删除流程，先问迁移策略）、并在 docs 标注「仅 owner/admin 二档」。默认走「真接通」（接通的消费点 = 约 215 处 `requireAdminPermission` 调用 + operator 可达其权限组端点）。

2. **rank25 — RoleOperator 真实可达能力（S，依赖 rank2）**：
   - operator 角色（`users/contract/contract.go:27`）当前进不了任何 admin 端点。给它绑定真实权限组（如只读类 `*:read` + 少量受限写），使其凭 `requireAdminPermission` 可达对应端点。
   - 接通的消费点 = operator 登录后能访问其权限组覆盖的 admin 端点、且不能访问未授权端点（403）。若 rank2 选了「二档下架」，则此项随 RoleOperator 一并删除。

3. **rank4 — 风控接真实强制点（M）**：
   - 在网关准入 `prepareGatewayAdmissionWithOptions`（`apps/api/internal/httpserver/runtime_gateway_core.go:207`）与认证路径接入 `RiskControlConfig`（`GetRiskConfig` 现仅 `runtime_admin_control_plane_handlers.go:457/476` 与 `admin_control/service/service.go:532` 自身读，无网关消费者）：
     - IP 黑名单（`blockedIPs`）、国家黑名单（`blockedCountries`，来源 IP→国家映射或请求头）命中拦截；
     - 按 key/用户维度的失败次数滑窗（`maxFailedRequestsPerMinute`）与日成本阈值（`maxCostPerDay`，滑窗或自然日）判定；
     - `mode == enforce` 时拒绝请求并 `append RiskControlLog(action=block)`；`mode == monitor` 时只记日志放行；命中冷却 `cooldownSeconds`。
   - 接通的消费点 = 网关准入读取并强制 `RiskControlConfig`；`RiskControlLog` 被真实写入（`RiskStatus` 不再恒 0）。
   - **二选一并落定**：若认定本批不接全部维度，则只接「IP/国家黑名单 + 日成本阈值」并明确：未接维度从风控页**下架**（删对应表单字段），不得保留「表单有但不强制」的字段。默认尽量真接通全部已暴露维度。

4. **rank5 — 内容安全可配置 + block + Luhn（L）**：
   - 给 `content_safety/service` 引入 Config（`New()` 现无入参，`service/service.go:77`；`Service struct{}` 无字段）：开关（启用/禁用整能力）、模式（`monitor` 只脱敏记录 / `block` 拦截 / `off`）、自定义关键词、按模型范围生效。Config 数据源接 `admin_control` settings（security tab），runtime 从 settings 读。
   - prompt-injection（`promptInjectionPatterns`，:56-64）支持 `block` 动作：命中且模式=block 时拒绝请求（不再只记 finding）。
   - 信用卡正则（`service/service.go:52` `(?:[0-9][ -]?){13,19}`）加 Luhn 校验：仅 Luhn 通过才视为卡号脱敏，避免把订单号/序列号等长数字误脱敏并清空 RawBody（清空发生在 `runtime_gateway_core.go` 内容安全应用路径，须确认仅在真命中时清空）。
   - 产出审核日志（flagged 记录可查），可选 flagged-hash 缓存与 ban/unban 端点（对照 sub2api ContentModerationService 能力，借鉴不抄码）。
   - 接通的消费点 = admin 可在 settings 关闭/调模式/加关键词，且改动经网关准入路径生效；block 模式真拦截；Luhn 失败的长数字不再被改写；审核日志端点/视图可查。
   - **二选一并落定**：审核日志/ban-unban 若本批不全做，须明确哪些做了（开关/模式/关键词/Luhn/block 为本批硬要求），未做的（如 flagged-hash 缓存）标注由 batch14 或后续覆盖，不得伪造。

明确不做（out-of-scope，均为「由其它批次涵盖」或「本批刻意不碰」，非永久搁置）：
- scheduler 策略 CRUD / scope / cron / 探测模型 / simulate-overview（rank3/21/22/19/20/18）——由 **batch10** 涵盖。
- TLS profile 绑定 / risk_level 入参 / SA-signer / channel-monitor worker / 配额补全（rank7/11/27/28/13/38/6）——由 **batch11** 涵盖。注意：本批的 rank4「风控」与 rank11「ProviderAccount.risk_level」是两个独立概念（前者是平台准入风控配置，后者是账号调度评分字段），勿混。
- 端用户登录摆设、~12 个 feature flag 强制、site-config、/me/attributes、console 幂等、captcha UI、备份/auditlog 保留（rank51/53/52/54/8/9/10/55/56/57/58）——由 **batch14** 涵盖。其中 captcha（rank56）与 auditlog 保留（rank57）虽属安全域，本批刻意不碰，留 batch14。
- 架构清理（拆巨文件/消重/死代码，rank59-68）——由 **batch15** 涵盖；但本批改动须遵守红线，必要时把新逻辑放进新文件。
</scope>

<constraints>
途中不得改变：
- 金额一律 string + big.Rat 8 位定点，绝不改回 float64（风控 `maxCostPerDay` 日成本判定须用既有 money 口径比较，不得引入 float 比较）。
- 不破坏 batch1-7 已落地能力：网关准入热路径现有行为（配额预留/限速/定价/会话粘度/故障转移/分组倍率/payload 转换）不得回归——风控/内容安全 hook 只新增准入判定，不改既有判定结果；现有内容安全脱敏在「未配置时」的默认行为需保持兼容（默认开启脱敏 monitor，避免升级后静默关闭已依赖的脱敏）。
- 凭证 AES-GCM 加密不得改明文。
- OpenAPI 兼容：新增权限目录端点/审核日志端点属新增（允许）；若需改动现有 admin 端点的响应结构（如 role/permission 字段）须先说明。从 `requireAdminSession` 改为 `requireAdminPermission` 是鉴权层变更，不改响应体结构。
- 不得修改与本任务无关的 `*_test.go`；新增能力配新测试；权限改造后原有针对 admin 端点的测试若因鉴权改变而需调整，只调整鉴权前置（给测试用户授予对应权限），不得删断言。
- ent schema 若改（如内容安全 Config 落 setting、风控日志结构、删 RoleOperator 枚举值）须 `go generate ./ent/...` 并同步 store/mock（含 memory store 的 `DeletedAt` 过滤、Store-mock codegen 已知坑）。
- 删枚举值（如 RoleOperator）/不可逆迁移先问我。
其他约束：新增依赖（如国家归属库、滑窗限速库）须先说明理由，能用既有 redis/内存窗口实现则不引新库。
</constraints>

<success_criteria>
完成需同时满足（每条在输出里给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴出尾部）；前端 `npm run -w apps/web tsc` / lint 通过（贴出尾部）。
2. **RBAC 真实化可证（rank2）**：测试证明——非授权权限的会话访问受保护 admin 端点返回 403（贴出测试名 + `go test` 输出）；`grep -rn "requireAdminSession\|requireAdminWriteSession" apps/api/internal/httpserver/ | wc -l` 显著下降（从 215 → 接近 0，贴出 grep 数）；`requireAdminPermission` 调用数显著上升（贴出 grep 数）；`requireAdminPermission` 不再对 admin 无条件短路（贴出 `runtime_http_helpers.go` 改后片段）。若选「二档下架」：grep 证明 RoleOperator 已删、权限分配 UI 已移除、docs 已标注。
3. **operator 可达可证（rank25）**：测试证明 operator 会话可访问其权限组覆盖的端点（如 `*:read`），且访问未授权端点返回 403（贴出测试输出）。或随 RoleOperator 删除（贴 grep 证明）。
4. **风控强制可证（rank4）**：测试证明 `enforce` 模式下黑名单 IP / 超 `maxCostPerDay` 日成本阈值 / 超 `maxFailedRequestsPerMinute` 的请求被拒，并写入 `RiskControlLog`（断言日志条数 +1 且 `RiskStatus` 不再恒 0）；`monitor` 模式只记日志放行（贴出测试名 + 输出）。`grep -rn "GetRiskConfig" apps/api/internal/httpserver/runtime_gateway_core.go`（或风控 hook 所在文件）证明网关准入路径已消费（贴出 grep）。
5. **内容安全可证（rank5）**：测试证明——能通过 Config 关闭内容安全（关闭后不脱敏）；prompt-injection 在 block 模式下被拒绝（贴测试输出）；Luhn 校验生效——一个 Luhn 失败的 16 位数字**不**被脱敏、一个真实有效卡号被脱敏（表驱动测试，贴输出）；审核日志可查（端点/视图返回 flagged 记录）。`grep` 证明 `New()` 现接受 Config（贴片段）。
6. 针对性 `go test`（受影响包：httpserver / users / admin_control / content_safety）全绿（贴出尾部）；`go test ./...` 全绿（贴出尾部）。
7. **前端强制点截图（chrome-devtools）**：roles 页权限改为受控多选并能保存生效；risk-control 页配置 enforce + 黑名单后，对命中请求生效（或在 admin 风控日志页能看到 block 记录）；内容安全 settings 段可关闭/调模式。各贴 1 张截图说明。
8. `git status` 干净（除预期改动），`git diff --stat` 贴出。
或 35 turns 后停止并汇总剩余阻塞项（每个摆设子目标的最终处置：真接通 / 诚实下架 / 阻塞原因）。
</success_criteria>

<sequencing>
按依赖顺序、每子项自带测试并单独 commit；schema/OpenAPI 改动先行：
1. 阶段一（OpenAPI/schema 先行）：若新增权限目录端点 / 审核日志端点 / 内容安全 settings 字段 / 风控日志结构需要 OpenAPI 或 ent schema 改动，先一次性改 `packages/openapi/openapi.yaml` + ent schema 并生成（`go generate ./ent/...`），同步 store/mock。
2. 阶段二（RBAC 权限目录 — rank2）：在 `users/contract` 定义权限目录常量集；改 `requireAdminPermission` 去 admin 短路；**分资源分批**把约 215 处 `requireAdminSession`/`requireAdminWriteSession` 替换为对应 `requireAdminPermission`，每批替换后跑该资源相关测试回归；给内置角色绑权限组。一个 commit（或按资源面拆几个 commit）。
3. 阶段三（operator — rank25）：给 operator 绑真实权限组 + 测试其可达/越权 403。一个 commit。
4. 阶段四（风控强制 — rank4）：在网关准入 hook 接 `RiskControlConfig`（黑名单/滑窗/日成本/enforce-monitor/写 RiskControlLog）+ 认证路径接入 + 测试。一个 commit。
5. 阶段五（内容安全 — rank5）：`New(Config)` 化 + settings 接线 + prompt-injection block + 信用卡 Luhn + 审核日志 + 测试。一个 commit。
6. 阶段六（前端 + 回归）：roles 受控多选、risk-control 生效路径、内容安全 settings 段；`go test ./...` + 前端 typecheck/lint + chrome-devtools 截图。一个 commit。
每阶段一个 commit，message 末尾加：
Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch9`）若干语义化 commit + 权限目录常量集与约 215 处鉴权改造 + operator 权限组 + 网关准入风控强制 hook + 内容安全 Config 化（含 block/Luhn/审核日志）+ 前端 roles 受控多选/risk-control/内容安全 settings + 全部新测试（passing）+ progress.txt 收尾（列每个摆设子目标的最终处置「真接通/诚实下架」、证据位置、未做的 out-of-scope 项及归属批次）。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 让测试通过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float（日成本判定用 money 口径）；退回明文凭证；**伪造未实现能力**——摆设的两种合法处置只有「真接通」（指明接通的消费点：约 215 处 requireAdminPermission / 网关准入读 RiskControlConfig / 内容安全 settings 经网关生效）或「诚实下架」（指明下架范围：删字段/删 UI/删枚举 + docs 标注），绝不假装接通（如风控 hook 留 TODO 却宣称已强制、内容安全 Config 加了字段却不读）。
先问我：破坏现有 OpenAPI 响应结构；删现有顶层路由文件；删 ent 枚举值（如 RoleOperator）涉及的存量数据迁移；不可逆数据迁移；向真实上游发起需真实凭证的调用（本批应无此需要，国家归属/IP 判定用本地库或请求头，不打真实第三方）。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（含每个摆设子目标的最终处置决定 + 证据位置 + 下一步）+ git commit。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：每个摆设子目标的当前处置状态（真接通/诚实下架/阻塞）、`go build`/`go test` 状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；已抽查核实，与审计一致）：
- RBAC 鉴权 helper：`apps/api/internal/httpserver/runtime_http_helpers.go:268-279`（requireAdminSession，仅 owner/admin 二档）、`:281-297`（requireAdminPermission，:286-289 对 owner/admin 短路放行）。
- 权限常量与角色：`apps/api/internal/modules/users/contract/contract.go:24-29`（RoleOwner/Admin/Operator/User）、`:31-34`（唯一权限常量 PermissionPaymentOrderRead）、`:103-104`（User.Roles/Permissions）、`:118-126`（RoleDefinition.Permissions）、`:135-139`（UpdateStoredRole.Permissions）。
- 约 215 处鉴权调用点：`apps/api/internal/httpserver/`（`grep -rn "requireAdminSession\|requireAdminWriteSession"` = 215；server.go 路由注册 + 各 runtime_admin_*.go handler）。
- RoleOperator seed：`apps/api/internal/modules/users/store/memory.go:384`（仅种子，生产码除定义/seed 外无使用）。
- 前端 roles：`apps/web/src/app/admin/roles/page.tsx:67-72`（permissions 自由文本 tags，待改受控多选）。
- 风控配置读取点：`apps/api/internal/modules/admin_control/service/service.go:532`（GetRiskConfig）、`:552`（写前读）；消费者仅 `apps/api/internal/httpserver/runtime_admin_control_plane_handlers.go:457/476`（即除 admin_control/control_plane 外无网关消费者）。
- 网关准入热路径（风控 hook 落点）：`apps/api/internal/httpserver/runtime_gateway_core.go:199-207`（prepareGatewayAdmission / prepareGatewayAdmissionWithOptions）、`:275-302`（内容安全应用 + 清空 RawBody 路径）。注意 core 文件约 2156 行余量极小，新风控逻辑建议放新文件（如 `runtime_gateway_risk.go`）由准入处调用。
- 前端 risk-control：`apps/web/src/app/admin/risk-control/page.tsx:45-79`（全套表单，待接生效/下架未接维度）。
- 内容安全服务：`apps/api/internal/modules/content_safety/service/service.go:67`（Service struct{} 无字段）、`:77-79`（New() 无 Config）、`:48-53`（信用卡正则 `(?:[0-9][ -]?){13,19}` 无 Luhn）、`:56-64`（promptInjectionPatterns）、`:82-100`（Apply 仅脱敏不拦截）。
- 内容安全 Config 数据源：`apps/api/internal/modules/admin_control`（settings security tab）、`apps/web/src/app/admin/settings/page.tsx`（新增内容安全段）。
- OpenAPI：`packages/openapi/openapi.yaml`（新增权限目录端点 / 审核日志端点 / 内容安全 settings 字段先改此处再生成）。
- 守门测试：`apps/api/internal/architecture/architecture_test.go`（跨模块 contract-only import + maxRuntimeFileLines=2200）、`apps/api/internal/codequality/code_quality_test.go`（maxProductionFuncLines=210）。
sub2api 仅作能力对照不抄代码。本批相关参照点：sub2api 的 RBAC 权限目录（resource:action 粒度 + 角色→权限组绑定）；RiskControl 的 IP/国家黑名单 + 失败/成本滑窗 + monitor/enforce 双模式 + 风控日志；ContentModerationService 的可配置审核（pre-block + 审核日志 + flagged-hash 缓存 + ban/unban + Luhn 校验信用卡）。仅借鉴能力与判定维度，实现自写。
</references>
