# Goal：配额真实性 + 账户导入认证（第五批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：覆盖 ranked backlog rank 6 + 7 + 13 + 17。让上游账号有真实剩余额度信号（不再盲调）、把 per-platform 一键 OAuth 与导入入口做成运营可用、识别封禁/验证状态。四项围绕「账号生命周期」，共用 QuotaReport / provider preset 框架。
> 注意（已核查）：SRapi 已有 config-driven QuotaReport 框架（preset 配 quota_url+JSON 路径提取 Plan/Credits，仅 Codex 接了）与通用 OAuth 状态机、bind-current-user/bind-login 端点——本批是「补齐与接线」，不是从零造，复用现有框架。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first，账号在 modules/accounts、账号导入授权在 modules/account_provisioning，网关在 httpserver，配额刷新在 workers/quota_refresh）；前端 Next.js + TypeScript（apps/web）。
本地开发：make dev-up + npm run dev；登录 admin@srapi.local / Admin1234。所有改动须过 go build/vet + 前端 tsc/lint，遵循 OpenAPI-first。绝不抄 sub2api 源码。
凭证一律 AES-GCM 加密（credential_ciphertext），绝不改明文。
</role_and_context>

<objective>
目标（可衡量最终态）：(a) Anthropic（最大上游）账号能拉到真实剩余额度信号（5h/7d/7d-Sonnet），写入与 codex 同构的 QuotaSignal，调度器据此避让接近耗尽的账号；(b) provider Preset 内置 OAuth 配置，管理员对 anthropic/codex/gemini/antigravity 点「授权」即走完整流程，无需手填 client_id/endpoint；(c) 三处导入入口统一为一个「导入账号」对话框，共用指纹去重；(d) 403 被结构化识别为 validation/violation/forbidden（含 validation_url），封禁账号不再被反复调度。完成后有测试与浏览器验证佐证。
动机：Anthropic 账号当前唯一「额度」是无条件写入的合成快照（ratio 恒 1），调度避让与告警全部失真，运营盲调；per-platform 一键 OAuth 缺失（要手填 endpoint）是与 sub2api 体验差距最大的一点；三处导入入口让管理员不知用哪个；封账号被持续调度直到反复失败才降级。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元）：
1. Anthropic 真实额度：新增 Anthropic OAuth 专用 quota fetcher（用 access_token 请求 /api/oauth/usage），把 five_hour/seven_day/seven_day_sonnet 映射为 QuotaSignal（quota_type=anthropic_5h/7d/7d_sonnet，RemainingRatio=1-utilization，ResetAt=resets_at）写快照；网关 Anthropic 响应路径解析 anthropic-ratelimit-unified-5h/7d 头做被动采样（Source=passive）。
2. Preset OAuth：account_provisioning 的 Preset struct 增可选 OAuthConfig（ClientID/AuthorizeURL/TokenURL/DeviceAuthorizeURL/Scopes/UsePKCE），为 anthropic/codex/gemini/antigravity 填已知常量；新增 GET /admin/providers/{id}/oauth-config 让前端按 provider 自动预填，管理员只需点「授权」（复用现有状态机，不重写）。
3. 统一导入：合并 file-import / codex-session-import / quick-setup 三条后端路径为一个「导入账号」对话框（内部按内容类型分 tab：标准 JSON / codex session / 单账号 OAuth）；把 codex 的 buildCodexIdentityKeys 抽成 provider-agnostic 的 import dedup helper 共用；结果模型统一为 created/updated/skipped/failed + warnings；把「哪些 runtime_class 需 OAuth 刷新」收敛到 provider preset 元数据，导入期与运行期共用一套判定。
4. 封禁/验证识别：QuotaReport / classifyProviderHTTPError 增 403 分类（validation/violation/forbidden）+ validation_url 提取，落到 account.Metadata 与快照；前端账号列表展示状态 badge；新增 POST /admin/accounts/{id}/reset-quota；给 FetchAccountQuota 加短期缓存 + 负缓存 + singleflight + 抖动防风控。

明确不做（out-of-scope）：
- 不做 Gemini tier RPD/RPM 完整视图（可留二期；本批 Anthropic 优先）。
- 不做 CRS 整库迁移、不做明文凭证往返备份。
- 不做 per-user-platform 配额的用户自助（第六批）。
- 不改既有 bind-current-user/bind-login 行为（已完整，沿用）。
</scope>

<constraints>
途中不得改变：
- 凭证 AES-GCM 加密不得改明文；OAuth refresh token 存储沿用现加密路径。
- 合成快照与真实快照的分离（第一批已做）不得破坏；新真实信号须走真实快照通道，不要再污染合成空间。
- 通用 OAuth 状态机与 PKCE 实现复用，不重写第二套。
- 现有 OpenAPI 响应结构兼容：新增端点可以，破坏现有结构需先说明。
- ent schema 若改 go generate ./ent/...，同步 store/mock。
- 不得修改无关 *_test.go；新增能力必须配新测试。拉取上游有外部依赖，测试用 mock/httptest，不打真实上游。
其他约束：抓取上游必须带防风控（缓存/负缓存/singleflight/抖动），新增依赖须先说明。
</constraints>

<success_criteria>
完成需同时满足（每条给出可观察证据）：
1. cd apps/api && go build ./... && go vet ./...，退出码 0（贴尾部）。
2. Anthropic 额度可证：用 httptest mock /api/oauth/usage，测试证明 fetcher 把 5h/7d/7d_sonnet 映射成 RemainingRatio<1 的真实 QuotaSignal 并写快照、调度读到的是真实值；贴出测试通过输出。
3. Preset OAuth 可证：GET /admin/providers/{id}/oauth-config 对 anthropic 返回预填的 authorize/token URL 等；测试或截图证明前端授权对话框无需手填即可发起。
4. 统一导入可证：测试证明同一账号重复导入被 dedup helper 跳过（skipped），三类内容走同一结果模型；贴出输出。
5. 封禁识别可证：测试证明 403 violation 响应被分类并写入 account.Metadata + 提取 validation_url；FetchAccountQuota 重复调用命中缓存/singleflight（不重复打上游）；贴出输出。
6. 前端：cd apps/web && npm run lint && npx tsc --noEmit 退出码 0（贴尾部）。
7. 浏览器验证（chrome-devtools）：截图证明（a）账号列表展示真实额度/封禁 badge；（b）一键授权对话框预填；（c）统一导入对话框 tab 切换。
8. go test ./... 全绿 + git status 干净，git diff --stat 贴出。
或 35 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
每单元自带测试并单独 commit：
1. 阶段一：FetchAccountQuota 防风控（缓存/负缓存/singleflight/抖动）+ 403 结构化分类 + validation_url 提取（先做共用底座）。
2. 阶段二：Anthropic OAuth quota fetcher（httptest mock）+ 写真实快照 + 被动响应头采样。
3. 阶段三：Preset OAuthConfig + GET /admin/providers/{id}/oauth-config + 前端授权对话框预填。
4. 阶段四：统一导入对话框 + provider-agnostic dedup helper + runtime_class 刷新判定收敛到 preset。
5. 阶段五：前端 badge + reset-quota 端点 + 浏览器验证。
每阶段一个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + Anthropic fetcher + Preset OAuthConfig + 统一导入对话框 + 封禁识别 + 新增测试（全 passing，外部调用用 mock）+ progress.txt 收尾。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 过测试；--no-verify 或 git push --force；退回明文凭证；在测试里打真实上游 API；伪造未接入的能力（Gemini RPD/RPM 本批不做就不要假装做）；重写已有的 OAuth 状态机/bind 端点。
先问我：破坏现有 OpenAPI 响应结构、删除现有顶层路由文件、不可逆数据迁移、向真实上游发起需要真实凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt + git commit。
新 context 开始：先 pwd、git log --oneline -10、读 progress.txt，再 go build ./... 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 35 turns 仍未达成，停下汇总：已完成阶段、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
- 合成快照（第一批应已分离真实/合成）：apps/api/internal/httpserver/runtime_filters.go:249；runtime_gateway_usage.go:554-562；调度读取 runtime_gateway_core.go:1187。
- 配额框架（复用）：config-driven QuotaReport（preset quota_url + JSON 路径提取 Plan/Credits，目前仅 Codex preset 配了）；codex window 魔数：apps/api/internal/.../codex_quota.go:83-86（300=5h/10080=7d，应改命名常量）。
- 账号导入授权：apps/api/internal/modules/account_provisioning（Preset struct registry.go:41-58，无 OAuth 字段）；通用 OAuth 状态机；catalog_handlers.go:1443（refresh-only 凭证换 access 白名单）；运行期 RuntimeClass 判定 gateway_core.go:1648。
- 导入三入口：file-import handleImportAdminAccounts；codex-session-import（buildCodexIdentityKeys 指纹去重 + JWT 富化）；quick-setup（runtime-class 推断）。
- 平台配额端点（admin-only）：server.go:659-661（三条 platform-quotas 路由）。
- 错误分类：classifyProviderHTTPError（403 现笼统归 auth_failed）。
- 前端：apps/web/src/app/admin/accounts/page.tsx；account-oauth-authorize-dialog.tsx（现要手填 clientId/authorizeUrl/tokenUrl/redirectUri/scopes）。
sub2api 仅作能力对照不抄代码：Anthropic OAuth /oauth/usage 5h/7d/7d-sonnet、AccountUsageService Source=passive/active、identity adoption。
</references>
