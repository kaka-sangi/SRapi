# Goal：上游认证矩阵真实性 —— 消灭摆设认证 + 刷新闭环（第七批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：12-agent 逐格 trace 网关签发路径后核查发现，SRapi 的 `runtime_class` / `Provider.auth_methods` 体系里存在**摆设认证**——allowlist/UI 里能选，但 `injectAuth` 不消费或没有刷新端点，选了必失败或过期即死。这直接污染「配额真实性」：系统显示「有可用账号/配额」，实际请求发不出（假配额）。本批把「账户认证」从「能配/能登」升级为「配了就真能签发、过期能自刷、没有摆设项」。
> 根因（一句话）：`Provider.auth_methods`(=preset 的 RuntimeClassAllowlist) 是「承诺」，`injectAuth` + `oauthRefreshSettings` 才是「兑现」，两者脱节就成摆设。
> 关联：与 batch5（配额真实性）强相关，可视为 batch5 的认证子目标；本批先于「健康检查联动配额」落地。

---

<role_and_context>
你是 SRapi 仓库的资深后端工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first）。上游认证的关键结构：
- 账号实体 `provideraccount.runtime_class`（apps/api/ent/schema/provideraccount.go:22）锁死一种认证；`account_type`/`upstream_client`/`credential_ciphertext`(AES-GCM) 配套。
- 分流规则 `isReverseProxyRuntime`（modules/provider_adapters service.go:1330）：runtime_class!=api_key ⇒ 走 reverse_proxy；api_key ⇒ 各 adapter 直连拼头。
- 单一签发函数 `injectAuth`（modules/reverse_proxy service.go:701）按 runtime_class case 构造上游 Authorization/Cookie。
- 刷新设置 `oauthRefreshSettings`（modules/reverse_proxy service.go:967-987）按 upstream_client 内建 token 端点。
- provider→认证白名单 `RuntimeClassAllowlist`（modules/account_provisioning registry.go 各 preset），经 catalog_handlers.go:432/490-509 暴露为 `auth_methods`。
本地开发：make dev-up；登录 admin@srapi.local / Admin1234。所有改动须过 go build/vet。绝不抄 sub2api 源码，只借鉴能力与算法思路。凭证 AES-GCM 加密不得改明文。
</role_and_context>

<objective>
目标（可衡量最终态）：管理员能选的每一个上游认证方式都「名实相符」——要么端到端真签发（含过期自刷），要么干脆不出现在 allowlist/UI；并用 CI 守门防止「承诺」与「兑现」再次脱节。完成后：(a) `service_account_json` 不再是必失败的摆设；(b) 凡 allowlist 含 oauth_refresh/oauth_device_code 的 provider 都有可验证的刷新闭环或被明确标 not_supported；(c) `desktop_client_token`/`ide_plugin_token`/`web_session_cookie` 不再是「配了不工作/够不着」的伪选项；(d) 一条 CI 断言保证 allowlist ⊆ 真实可签发集合。
动机：摆设认证 = 假配额。选了 service_account_json 的 anthropic 账号、过期不自刷的 openai/gemini oauth 账号，会被系统当「可用」纳入调度与配额统计，实际 401/发不出，运营盲目依赖不存在的容量。这是「配额真实性」的认证侧根因。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元；每项二选一并落地，不留中间态）：
1. **service_account_json（P0，必做）**：二选一——
   (a) **止血下架**：从 anthropic + bedrock 的 RuntimeClassAllowlist 移除 service_account_json，删 UI 选项；或
   (b) **实接 Vertex/GCP SA**：新增 SA token provider（解析 credential 的 SA JSON client_email/private_key → RS256 签 JWT → POST oauth2.googleapis.com/token 换 access_token → 按 exp 缓存刷新 → injectAuth 走 Bearer），并把 gemini/anthropic 的 URL 构造指向 Vertex 端点。
   默认先做 (a) 止血；(b) 作为 stretch 阶段（见 sequencing 阶段五，可选）。
2. **openai/gemini × oauth_*（P0，必做）**：二选一——
   (a) 给 oauthRefreshSettings 增内建 TokenEndpoint/ClientID/Scope case（gemini 含 client_secret），实现刷新闭环；或
   (b) 若产品不支持这两家用户态 OAuth 直连，改判 not_supported 并从 preset allowlist 移除。
3. **desktop_client_token / ide_plugin_token（P1，必做）**：二选一——
   (a) 并入 oauth_refresh（antigravity 已有真 oauth 路径，删这两个冗余 class + 从 allowlist 移除）；或
   (b) 若确有专属令牌交换流程，实现专属字段读取 + 刷新。默认 (a)。
4. **web_session_cookie（P1，必做）**：补 chatgpt-web/claude-web preset——让 catalog 的 preset allowlist 含 web_session_cookie 且 presetAdapterType 产出 reverse-proxy-chatgpt-web/claude-web，使 Quick Setup/admin create 可直接选（现 injectAuth Cookie 注入逻辑已真实，只是无预设可达）；若产品不要 web 反代，则连同 injectAuth 的 cookie case 一起明确标注为「仅手建」并从默认 UI 隐藏。
5. **CI 守门 + 矩阵文档（必做）**：加一个测试断言「每个 preset RuntimeClassAllowlist 里的 class ∈ (injectAuth 支持的 class ∪ 直连 adapter 支持的 class ∪ 有 oauthRefreshSettings 的 class)」；产出 docs 里的「provider × auth_method × {✅/🟡/⬜/—}」矩阵作为防回归基线。

明确不做（out-of-scope）：
- 不做端用户登录认证（wechat/dingtalk/微信支付/邮箱验证码/OAuth 免密快速通道）——这是独立子系统，见文末「附录」，留作后续批次。
- 不做 `upstream`（base_url+api_key 透传）一等建模（P2，custom_reverse_proxy 暂可覆盖，留后续）。
- 不动 codex/anthropic/antigravity 的 oauth 刷新（已 wired_effective，沿用，仅作回归保护）。
- 不改 api_key 直连签发（已 wired_effective）。
- 不动配额/调度算法本体（健康检查联动配额在 batch5 做，本批只产出可被其消费的「认证可签发性」判定）。
</scope>

<constraints>
途中不得改变：
- 凭证 AES-GCM 加密不得改明文；OAuth refresh token 存储沿用现加密路径。
- 已 wired_effective 的认证不得回归：api_key（OpenAI Bearer / Anthropic x-api-key / Gemini ?key= / Bedrock SigV4）、codex/anthropic/antigravity 的 oauth_refresh+刷新、anthropic 的 cli_client_token，必须仍有测试通过。
- 「二选一」的每项必须落定到无中间态：不允许留下「allowlist 有但仍不工作」的项——CI 断言就是用来保证这一点的。
- 现有 OpenAPI 响应结构兼容：从 allowlist 移除 class 属配置层变更；若改 auth_methods 暴露结构需先说明。
- ent schema 若改（如删 runtime_class 枚举值）go generate ./ent/...，同步 store/mock（memory store DeletedAt 过滤、Store-mock codegen 已知坑）；注意已存量账号的 runtime_class 迁移（删值前先问迁移策略）。
- 不得修改无关 *_test.go；新增能力配新测试；拉取上游/换 token 用 httptest mock，不打真实上游。
其他约束：新增依赖（如 JWT 签名库）须先说明理由。
</constraints>

<success_criteria>
完成需同时满足（每条在输出里给出可观察证据，评估器不自己跑命令）：
1. cd apps/api && go build ./... && go vet ./...，退出码 0（贴尾部）。
2. **零摆设可证（核心）**：新增 CI 测试断言「所有 preset allowlist 的 class ⊆ 可签发集合」通过；贴出测试名 + go test 输出。并贴出该断言在「修复前会失败、修复后通过」的说明（如临时插入 service_account_json 让它红、移除后转绿）。
3. service_account_json 处置可证：
   - 若选下架：grep 证明 anthropic/bedrock preset allowlist 不再含 service_account_json，UI 选项移除（贴 registry.go 片段）；
   - 若选实接：httptest mock oauth2.googleapis.com/token，测试证明 SA JSON → RS256 JWT → 换得 access_token → injectAuth Bearer；贴出测试输出。
4. openai/gemini oauth 处置可证：
   - 若选补刷新：测试证明 openai/gemini 账号 token 过期后下一请求自动刷新成功（httptest mock token 端点）；
   - 若选 not_supported：grep 证明已从 allowlist 移除且 adapter/injectAuth 对其返回明确不支持错误；贴出证据。
5. desktop/ide token、web_session_cookie 处置可证：grep + 测试证明其最终状态（合并/删除，或 preset 可达），无「allowlist 有但 injectAuth 不消费」的残留。
6. 回归可证：已 wired_effective 的 5 类认证（见 constraints）的现有/新增测试全绿；贴出输出。
7. 矩阵文档：docs/ 下产出/更新 provider × auth_method 矩阵（✅/🟡/⬜/—），与代码现状一致。
8. go test ./... 全绿 + git status 干净，git diff --stat 贴出。
或 30 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit：
1. 阶段一（守门先行）：写 CI 断言「allowlist class ⊆ 可签发集合」+ 列举三个集合的小工具；此时断言应因 service_account_json 等而**红**，作为基线。
2. 阶段二（P0 止血）：service_account_json 下架（或排期实接）；openai/gemini oauth 二选一落定。跑阶段一断言转**绿**。
3. 阶段三（P1 清理）：desktop/ide token 合并删除；web_session_cookie 补 preset 或明确隐藏。
4. 阶段四（回归 + 文档）：补/跑已 wired_effective 五类认证的回归测试；产出矩阵文档。
5. 阶段五（可选 stretch）：实接 Vertex/GCP SA（若阶段二选了实接而非下架，则此阶段并入阶段二）。
每阶段一个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + CI 守门测试 + 各认证项的处置（下架/实接/合并/补 preset）+ 已 wired 认证的回归测试 + docs 认证矩阵 + progress.txt 收尾（列每个 class 的最终处置、证据位置、未做的 out-of-scope/附录项）。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 过测试；--no-verify 或 git push --force；退回明文凭证；在测试里打真实上游；保留任何「allowlist 有但不工作」的摆设项还假装修好；伪造未实现的能力（如阶段五不做就别假装 Vertex 已接）。
先问我：删除 runtime_class 枚举值涉及的存量账号数据迁移；破坏现有 OpenAPI 响应结构；向真实上游发起需真实凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（含每个 class 的最终处置决定 + 证据）+ git commit。
新 context 开始：先 pwd、git log --oneline -10、读 progress.txt，再 cd apps/api && go build ./... 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 30 turns 仍未达成，停下汇总：每个 class 的当前处置状态、CI 断言红/绿、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line）：
- 分流：apps/api/internal/modules/provider_adapters/service.go:1330（isReverseProxyRuntime，api_key⇒false 直连）。
- 直连签发：同上 invokeOpenAICompatible（:510 取 api_key /:529 Bearer）、invokeAnthropicCompatible（:459，默认 x-api-key）、invokeGeminiCompatible（:1362/:1389 ?key= query）、Bedrock（AWS SigV4）。
- 反代签发：apps/api/internal/modules/reverse_proxy/service.go:701（injectAuth case 分发）/:705-708（cli_client_token firstCredentialString）/:709（oauth/desktop/ide 共用 Bearer access_token）/ web_session_cookie case（删 Authorization+设 Cookie）。
- 刷新：apps/api/internal/modules/reverse_proxy/service.go:967-987（oauthRefreshSettings：仅 codex_cli/claude_code_cli/antigravity 有内建端点）；shouldRefreshCredential / retryAfterAuthRefresh / supportsOAuthRefresh（仅 oauth_*）。
- 预设白名单：apps/api/internal/modules/account_provisioning/registry.go（anthropic allowlist 含 api_key/oauth_refresh/oauth_device_code/cli_client_token/service_account_json ~:209-214；antigravity 含 oauth_refresh/desktop_client_token/ide_plugin_token；codex :173-176 不含 api_key；openai :134-139 含 oauth_*）。
- auth_methods 暴露：apps/api/internal/httpserver/runtime_admin_catalog_handlers.go:432/490-509（AuthMethodStrings）；supportsRefreshTokenOnlyImport :1629。
- 业务 adapter 头：codex.go:122/:126（拒 api_key）、claude_code.go:20（isClaudeCodeReverseProxy）、chatgpt_web adapter（浏览器指纹头）。
- account schema：apps/api/ent/schema/provideraccount.go:21-24（account_type/runtime_class/upstream_client/credential_ciphertext）。
- provisioning OAuth：apps/api/internal/modules/account_provisioning/{contract/contract.go:15 device_code,:45 TokenAuthMethod; service/service.go PKCE/device/token 交换 clientAuthMethod:602}。
sub2api 仅作能力对照不抄代码：Account.platform+type+credentials 正交建模；service_account(Vertex RS256 JWT 换 Google token)、buildVertexGeminiURL；openai/oauth.go、geminicli/oauth.go 的内建刷新端点；AccountTypeUpstream(base_url+api_key 透传)。
</references>

---

## 附录：端用户登录认证摆设（独立子系统，候选 batch8，本批不做）

逐项已核查，列为后续批次 mini-backlog（与上游认证矩阵是两张表，勿混）：

| 项 | 类别 | sub2api | SRapi 现状 | 补法 | 工作量 |
|---|---|---|---|---|---|
| wechat 登录 | 摆设 | ✅ 专用非标准实现 | ⬜ 仅通用 Bearer-userinfo 通路，驱动不了真微信(`sns/oauth2/access_token`+query userinfo+code2Session+unionid) | 专用 handler 或从白名单/前端下架 | L |
| dingtalk 登录 | 摆设 | ✅ 专用 + 邮箱补全 | ⬜ 同上(真钉钉需 `/v1.0/oauth2/userAccessToken`→`/v1.0/contact/users/me`) | 专用 handler 或下架 | L |
| 微信支付 OAuth(取 openid) | 缺失 | ✅ start+callback+前端 | ⬜ 无任何路由/页面 | 仅做微信内 H5 支付时补，依赖 wechat 能力 | M |
| 邮箱验证码注册/登录 | 缺失 | ✅ SendVerifyCode | ⬜ 仅 verification+reset，无 passwordless 验证码闭环 | 补发送/校验 + 注册消费 verify_code | M |
| OAuth 返回用户免密快速通道 | 体验缺口 | ✅ subject 路由 | ⬜ 用 FindByEmail 匹配；`FindAuthIdentityByProviderSubject`(auth/service.go:683) 仅 bind 去重，从不认证返回用户 → 老 OAuth 用户每次仍要重输密码 | 加「已绑定 identity→凭 provider_subject 直发 session」快速路径 | M |
| microsoft / discord | 缺失 | ⬜ 无 | ⬜ 无(理论上可走通用 oidc 配置) | 按需用通用 OIDC provider_key 配置即可，无需专用 | S |

**真有效的端用户登录（勿动）**：password、totp、email(验证+找回)、oidc、google、github、linuxdo —— 一条通用 OAuth2/OIDC 流水线 + `/auth/oauth/providers` 动态按钮。
