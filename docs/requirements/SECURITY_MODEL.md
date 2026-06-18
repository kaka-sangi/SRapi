# SRapi 安全模型

## 1. 目标

本文档定义 SRapi 生产环境必须遵守的安全模型与边界，覆盖控制台登录、API Key、Provider 凭证、Cookie、CSRF、日志脱敏、审计、密钥轮换和 AI Gateway 特有风险。所列要求均已在代码中落地，本文档是平台的权威安全模型，而非待办清单。

安全原则：

```txt
默认最小权限、敏感数据不落明文、日志默认脱敏、所有高风险操作可审计、外部输入不可信。
```

## 2. 信任边界

```txt
Browser Console
  -> Console API (/api/v1/*)
  -> Application Service
  -> PostgreSQL / Redis

API Client
  -> Gateway API (/v1/*)
  -> API Key Auth
  -> Scheduler
  -> Provider Adapter
  -> Upstream Provider
```

边界规则：

- Browser Console 使用 Cookie 登录态，不使用 Gateway API Key。
- API Client 使用 Bearer API Key，不使用控制台 Cookie。
- Provider Adapter 不得接触用户登录 Session。
- Scheduler 不得解密 Provider 凭证。
- Provider 凭证只在 Adapter 发起请求前由受控服务解密。

## 3. 控制台认证

控制台 API 使用：

```txt
HttpOnly Cookie + CSRF Token
```

Cookie 必须设置：

```txt
HttpOnly = true
Secure = true in non-local environments
SameSite = Lax by default
Path = /
```

Session 要求：

- Access session 必须有过期时间。
- Refresh session 如果引入，必须支持 rotation。
- 登出必须使当前 session 失效。
- Session ID 持久化时只能保存 hash，不得保存 Cookie 原文。
- 高风险操作可以要求二次确认或重新认证。

CSRF 要求：

- 所有控制台写操作必须校验 `X-CSRF-Token`。
- 只读 GET 不应产生状态变更。
- CORS 默认只允许控制台来源。

CSRF Token 实现采用：

```txt
Synchronizer Token Pattern
```

流程：

- 登录后服务端为 session 生成 CSRF token。
- 前端通过 `/api/v1/auth/csrf` 或 session bootstrap 响应获取 token。
- 所有控制台写操作通过 `X-CSRF-Token` 发送 token。
- 服务端必须校验 token 与当前 session 绑定。
- CSRF token 持久化时只能保存 hash，登录响应只返回本次新建 token 的明文值。

可选方案：

```txt
Signed Double-Submit Cookie
```

要求：

- CSRF cookie 可以被前端读取，但必须签名。
- Session cookie 必须保持 HttpOnly。
- CSRF token 必须绑定 session 或 session id 派生值。
- 禁止使用未签名的 naive double-submit cookie。

Defense in depth：

- 写操作校验 `Origin`。
- `Origin` 缺失时校验 `Referer`。
- 可启用 Fetch Metadata headers 拦截跨站写请求。
- SameSite 只是纵深防御，不得替代 CSRF token。

### 3.1 控制台密码安全

控制台用户密码不得明文或可逆加密存储。

当前哈希方案：

```txt
bcrypt cost 12
```

实现细节：

- 密码哈希由 `apps/api/internal/modules/users/service/service.go` 的 `defaultBcryptCost = 12`
  控制，使用 `golang.org/x/crypto/bcrypt`。
- bcrypt 自带每密码独立 salt，并把 cost 与 salt 编码进 hash 字符串本身，
  无需额外存储参数。

要求：

- 每个密码必须使用独立 salt（bcrypt 默认满足）。
- 密码哈希参数（cost）必须随 hash 一起保存（bcrypt 默认满足）。
- 登录失败必须有速率限制。
- 管理员密码重置必须写 audit log。

> Roadmap / 尚未实现：迁移到 Argon2id 仍是可选演进方向（建议参数起点
> memory = 19456 KiB、iterations = 2、parallelism = 1）。当前代码未使用 Argon2id；
> 任何切换都必须保留 cost 12 bcrypt 的现有 hash 校验路径以兼容已有用户。

Public registration:

- `POST /api/v1/auth/register` 只在
  `admin_settings.security.registration_enabled=true` 时开放。
- `admin_settings.security.registration_email_suffix_allowlist` 非空时，注册邮箱域名
  必须精确匹配归一化后缀；空列表允许所有合法邮箱域名。
- 注册接口只创建普通用户，使用系统默认余额和 RPM 限制，并在成功后创建
  HttpOnly console session cookie 与一次性 CSRF token。
- 重复邮箱、邮箱后缀策略拒绝、非法邮箱、弱密码和非法姓名必须返回同一类
  通用注册错误，避免通过注册结果枚举现有账号或配置的注册域名。
- 注册成功写入安全 audit，但 audit 不得包含明文密码、session cookie 或 CSRF
  token。
- 注册成功后，邮箱所有权通过独立 email verification flow 证明；注册接口本身
  不把邮件验证 token 或投递细节返回给调用方。

Email verification:

- `POST /api/v1/auth/email-verification/request` 是公开接口；对于格式合法的邮箱，
  必须返回统一 accepted 响应，不能泄露邮箱是否存在、用户是否激活、邮箱是否已验证
  或邮件是否实际投递。
- 存储层只能保存 verification token 的 keyed hash、过期时间和使用时间，不能保存明文 token。
- outbox 投递事件只能保存加密 token、邮箱 hash、用户 id、模板和过期时间；不得保存
  明文邮箱、明文 token、密码、session cookie 或 CSRF token。
- `POST /api/v1/auth/email-verification/confirm` 必须单次消费未过期 token 并设置
  `users.email_verified_at`；重复使用、过期或未知 token 必须失败。
- 邮箱验证只证明邮箱所有权，不创建 console session，不刷新 session cookie，
  也不授予额外权限。

Password reset:

- `POST /api/v1/auth/password-reset/request` 是公开接口；对于格式合法的邮箱，
  必须返回统一 accepted 响应，不能泄露邮箱是否存在、用户是否激活或邮件是否实际投递。
- 存储层只能保存 reset token 的 keyed hash、过期时间和使用时间，不能保存明文 token。
- outbox 投递事件只能保存加密 token、邮箱 hash、用户 id、模板和过期时间；不得保存
  明文邮箱、明文 token、密码、session cookie 或 CSRF token。
- `POST /api/v1/auth/password-reset/confirm` 必须单次消费未过期 token，重置 password hash
  后撤销该用户活跃 console session；重复使用、过期或未知 token 必须失败。

Transactional auth email delivery:

- Outbox worker 只能在 `EMAIL_PUBLIC_BASE_URL`、`EMAIL_SMTP_HOST` 和
  `EMAIL_SMTP_FROM` 配齐后投递 password reset / email verification 邮件；缺少配置时事件
  保持 retry/fail 状态，不得静默标记成功。
- worker 只能在内存中解密 token payload，持久 outbox 仍只能保存加密 token 和邮箱 hash。
- action URL 必须从 `EMAIL_PUBLIC_BASE_URL` 和事件里的相对 path 构造，不能使用请求
  `Host` header；base URL 必须是绝对 `http` / `https` URL 且不带 query/fragment。
- 发送前必须重新读取当前用户，并校验用户仍为 active 且当前邮箱 hash 与事件 hash 一致；
  邮箱已变更或用户停用时跳过发送。
- 邮件 header 字段必须过滤 CR/LF，防止 SMTP header injection。
- `EMAIL_SMTP_PASSWORD` 只允许通过部署环境注入；Admin Settings、OpenAPI/SDK、audit
  snapshot 和 settings JSON 不得接收或保存 SMTP password。

Optional notification unsubscribe:

- 只有可选通知事件可以退订，例如 `balance.low`、
  `subscription.expiry_reminder` 和 `account.quota_alert`；password reset、
  email verification、notification contact verification 等 transactional mail
  不得被偏好状态压制。
- 退订 token 必须签名且有过期时间；payload 只能包含 event、邮箱 hash 和 expiry，
  不得包含明文邮箱、用户 id、session cookie、CSRF token 或 SMTP secret。
- 一键退订邮件头只允许 `List-Unsubscribe` 与
  `List-Unsubscribe-Post: List-Unsubscribe=One-Click`，且所有 header value 必须过滤
  CR/LF。
- `GET /api/v1/notifications/unsubscribe` 只能预览 token；实际状态变更使用
  `POST /api/v1/notifications/unsubscribe`。写入的偏好状态只保存 event、邮箱 hash、
  status、source 和更新时间。
- `GET /api/v1/me/notification-preferences` 返回当前用户主邮箱对应的可选通知订阅状态；
  `PUT /api/v1/me/notification-preferences` 必须使用当前 console session 和 CSRF。
- 当前用户偏好更新只能修改 allowlisted optional events，不能修改 transactional auth mail。
  存储层仍只保存 event、邮箱 hash、status、source 和更新时间，不得保存明文邮箱。
- 额外通知邮箱使用独立的验证型 contact flow，不得通过偏好 API 直接写入未验证邮箱。
  `POST /api/v1/me/notification-contacts` 和 contact update/delete 必须使用当前
  console session 与 CSRF。Contact verification outbox payload 只能保存 contact id、
  recipient email hash、加密后的 contact email、加密后的 verification token、过期时间和
  action path；不得保存明文 contact email、session cookie、CSRF token、SMTP secret、
  API key、provider credential 或 prompt。只有 verified 且未 disabled 的 contact 会参与
  optional notification delivery，并且仍按该 contact email 的 event-scoped unsubscribe
  状态和 one-click header 处理。
- Low-balance notification triggers are emitted as `BalanceLowTriggered`
  outbox events only after a successful usage charge crosses the configured
  threshold downward. Payloads store recipient user id, recipient email hash,
  balance numbers, threshold, ledger reference, usage log ids, and optional
  recharge URL; they must not store plaintext email, unsubscribe token, session
  cookie, CSRF token, SMTP secret, API key, provider credential, or prompt.
- Notification dispatch must re-read the current user, verify the recipient
  email hash, honor `balance.low` unsubscribe state, add one-click unsubscribe
  headers when a public base URL is configured, and leave SMTP failures
  retryable in outbox instead of blocking or rolling back balance charging.
- Subscription expiry reminder triggers are emitted as
  `SubscriptionExpiryReminderTriggered` outbox events when active subscriptions
  are 7, 3, or 1 days from expiry. Payloads store subscription id, user id, plan
  id/name, reminder key, expiry timestamp, triggered timestamp, and console
  path only; they must not store plaintext email, unsubscribe token, session
  cookie, CSRF token, SMTP secret, API key, provider credential, or prompt.
- Subscription reminder dispatch must re-read the current user, skip inactive
  users, honor `subscription.expiry_reminder` unsubscribe state, add one-click
  unsubscribe headers when a public base URL is configured, and leave SMTP
  failures retryable in outbox instead of changing subscription state.
- Account quota notification triggers are emitted as
  `AccountQuotaAlertTriggered` outbox events only when persisted quota
  snapshots cross the configured remaining-ratio threshold downward. Payloads
  store account id/name, provider id, runtime class, quota type, quota numbers,
  threshold, snapshot timestamps, and console path only; they must not store
  plaintext recipient emails, unsubscribe tokens, session cookies, CSRF tokens,
  SMTP secrets, API keys, provider credentials, or prompts.
- Account quota alert dispatch must select active owner/admin users at send
  time, honor each user's `account.quota_alert` unsubscribe state, add
  one-click unsubscribe headers when a public base URL is configured, and leave
  SMTP/template failures retryable in outbox instead of changing account quota
  state.

Notification email templates:

- Admin template reads require an admin session. Template update、restore 和 preview 使用
  CSRF；preview 不保存状态。
- 模板只能使用事件允许的 placeholder；未知事件、未知 placeholder、畸形 `{{...}}`
  标记、空模板和超长模板必须拒绝。
- 预览和投递渲染必须对变量做 HTML escaping；subject 必须过滤 CR/LF。
- URL placeholder 只允许 `http`、`https`、`mailto` 或安全相对路径；不安全值
  必须渲染为空字符串，不能进入 HTML href。
- Template API、audit 和 settings snapshot 不得包含 SMTP password、plaintext recipient
  email、unsubscribe token、provider credential、API key 或 session/CSRF token。

Current-user profile updates:

- `PATCH /api/v1/me` 必须使用当前 console session，并要求 CSRF header。
- 请求体必须是字段白名单；当前只允许更新 `name`，不得通过该接口修改 email、
  roles、status、balance、RPM limit、password、avatar 或其他管理员字段。
- Profile update audit 只能记录非敏感用户元数据，不得记录 session cookie、
  CSRF token 或 credential-like 字段。

Current-user avatar storage:

- `PUT /api/v1/me/avatar` 和 `DELETE /api/v1/me/avatar` 必须使用当前 console
  session，并要求 CSRF header。
- 上传只接受 `multipart/form-data` 字段 `avatar`；服务端必须执行大小限制、
  图片解码、格式白名单和尺寸限制，不能信任浏览器提供的文件名或
  `Content-Type`。
- v1 只接受 PNG/JPEG 输入，并统一重编码为 PNG 后存储和服务；SVG、远程 URL
  和任意 data URL 不进入存储，避免脚本、SSRF 和外链追踪风险。
- 头像服务响应和 audit 只能记录 content type、byte size、sha256、尺寸和
 更新时间；不得记录 session cookie、CSRF token、原始文件名、API key、
  provider credential 或 prompt。
- 头像读取必须走受控 API，返回 `image/png`、`ETag` 和
  `X-Content-Type-Options: nosniff`。

Current-user auth identity directory:

- `GET /api/v1/me/auth-identities` 必须使用当前 console session。
- `DELETE /api/v1/me/auth-identities/{id}` 必须使用当前 console session 和
  `csrfHeader`，并且只能删除当前用户拥有的外部身份。不得按 provider 批量解绑，
  以免同类 provider 多实例时误删其它登录入口。
- 响应只能包含本地 email 派生身份和已绑定外部身份的安全摘要，不得返回原始
  upstream subject、authorization code、access token、refresh token、session cookie、
  CSRF token、provider secret 或完整 credential payload。
- 外部身份持久化只能保存 provider、provider instance key、
  `provider_subject_hash`、display-safe `subject_hint`、验证时间和非敏感资料摘要。
- 本地 email 身份由 `users.email` 派生，不是可删除的外部身份；当删除会导致用户
  没有任何登录方式时，服务层必须拒绝解绑。
- 后续 OAuth/OIDC callback 写入该目录时必须使用 state/nonce/PKCE 或等价机制防止
  CSRF、authorization-code injection 和 mix-up；绑定现有 console user 的写操作仍需
  当前会话和 CSRF 保护。

Pending OAuth/OIDC decision sessions:

- `pending_oauth_sessions` 只能保存短期决策状态；不得保存 authorization code、
  access token、refresh token、raw upstream subject、state、nonce、PKCE verifier、
  provider secret 或完整 claim payload。
- `session_token_hash` 必须使用服务端密钥 HMAC 派生；明文 pending token 只返回给
  本次浏览器流程，并且消费后通过 `consumed_at` 标记单次使用。
- `provider_subject_hash` 必须是上游 subject 的 hash；API/日志/审计只能使用
  `subject_hint` 这类 display-safe 摘要。
- `redirect_to` 必须限制为站内路径，跨站或协议相对路径归一为 `/`。
- 后续公开 start/callback 路由必须使用 state、nonce、PKCE 或等价机制绑定浏览器
  流程，并继续避免把 provider claim payload 或 token material 暴露给 OpenAPI。

OAuth/OIDC authorization start:

- `GET /api/v1/auth/oauth/{provider}/start` 只创建浏览器授权流程，不创建或绑定
  用户身份；`bind_current_user` 仍留给后续 CSRF-protected 写路径。
- 授权请求必须使用 authorization code + PKCE S256，并使用 `state` 绑定浏览器
  流程；OpenID scopes 必须附带 `nonce`，供 callback 校验 ID Token replay。
- 浏览器流程状态保存在短期 encrypted HttpOnly `srapi_oauth_flow` cookie 中，
  cookie path 限定为 `/api/v1/auth/oauth`，内容包括 provider、provider key、
  intent、local redirect、state、PKCE verifier、nonce 和 expiry。
- flow cookie 不得包含 provider client secret、authorization code、access token、
  refresh token、raw upstream subject、完整 claim payload、session cookie、CSRF token
  或 API key material。
- `redirect` 只能保留站内路径；空值、跨站 URL 或 protocol-relative URL 必须归一为 `/`。
- Admin Settings 只暴露非密钥 provider authorization config，例如 client id、
  authorize URL、token URL、userinfo URL、redirect URI、token auth method 和
  scopes；token exchange 所需 client secret 不得进入 Admin Settings API、
  OpenAPI/SDK 响应或 audit snapshot。
- `GET /api/v1/auth/oauth/{provider}/callback` 必须校验 flow cookie、provider、
  provider key、state、client id 和 redirect URI；state 不匹配或 provider 返回
  error 时必须清理 flow cookie。
- callback v1 只支持 `token_auth_method=none` 的 public-client PKCE 交换：
  token request 使用 `grant_type=authorization_code`、authorization `code`、
  原始 `redirect_uri`、`client_id` 和 flow cookie 中的 `code_verifier`。
- callback 只能用 access token 通过 Bearer 请求 UserInfo，并只保留安全 profile
  摘要。raw upstream subject 必须先用 server secret 做 keyed hash 后才能写入
  pending OAuth session；access token、refresh token、authorization code、raw
  subject 和完整 claims 不得落库、入 cookie、写日志或出现在 redirect URL。
- callback 成功后必须设置短期 HttpOnly `srapi_oauth_pending` cookie，并把 cookie
  path 限定为 `/api/v1/auth/oauth/pending`。pending token 不得放进 query string。
- `GET /api/v1/auth/oauth/pending` 只能只读查看 pending decision，不得 consume
  pending token 或执行创建账号、绑定账号、签发登录 session 等状态变更。响应只能包含
  provider、provider key、display-safe subject hint、安全 profile 摘要、站内
  redirect、expiry 和 next-step 结论；不得返回 pending token、raw upstream
  subject、provider-subject hash、authorization code、provider access/refresh
  token、session cookie、CSRF token 或完整 claims。
- `POST /api/v1/auth/oauth/pending/bind-current-user` 必须使用当前 console
  session、CSRF header 和 HttpOnly pending cookie。它只能把 pending 外部身份绑定到
  当前登录用户；如果相同 provider/provider_key/provider_subject_hash 已归属其它用户，
  服务层和持久层都必须拒绝，不能静默转移身份归属。成功后必须消费 pending session、
  清理 pending cookie，并且响应仍只能返回 current-user auth identity 目录的安全摘要。
- 当前用户绑定 pending OAuth 身份不创建账号、不校验密码、不签发新的登录 session，
  这些动作必须通过后续独立的 create-account / bind-existing-login 流程处理。
- `POST /api/v1/auth/oauth/pending/send-verify-code` 只能用于 provider 没有返回可用邮箱的
  pending OAuth 登录流程。它必须要求 HttpOnly pending cookie，统一返回 accepted，不得泄露
  目标邮箱是否已存在。发送内容必须通过 outbox 投递高熵一次性 email-completion 链接，outbox
  payload 只能保存加密邮箱、加密 token、邮箱 hash 和过期时间，不得保存明文邮箱、pending token
  或 provider subject。
- `POST /api/v1/auth/oauth/pending/email-completion/confirm` 必须要求同一个 HttpOnly pending
  cookie 和邮件 token。token 解密后必须匹配当前 pending session id、未过期且邮箱语法安全；
  成功只把该邮箱写回 pending session 并标记 provider-independent email ownership verified，
  不得消费 pending session、绑定身份、创建账号或签发 console session。确认后由新的
  pending preview 决定进入 create-account 或 bind-login。
- `POST /api/v1/auth/oauth/pending/create-account` 只能用于未登录浏览器把 verified
  provider email 创建为新的 console 账号。它必须同时要求 HttpOnly pending cookie 和
  `GET /pending` 返回的短期 create-account action token；该 token 必须绑定 pending
  token hash，不能与其它 pending OAuth session 互换。成功前必须校验注册开关、邮箱后缀
  策略、pending email 一致性和身份归属冲突；成功后才能绑定身份、消费 pending session、
  清理 pending cookie 并签发新的 console session。绑定或 pending 消费失败且尚未签发
  session 时，必须回滚新建用户，避免半完成 OAuth 流程占用邮箱。
- `POST /api/v1/auth/oauth/pending/bind-login` 只能用于未登录浏览器把 pending 外部身份
  绑定到一个已有 console 账号。它必须要求 HttpOnly pending cookie 和本地
  email/password；密码错误、禁用用户、pending 目标用户不一致、身份已归属其它用户都必须
  拒绝。未启用 2FA 的账号成功后才能绑定身份、消费 pending session、清理 pending
  cookie 并签发新的 console session。
- 启用 2FA 的账号在 `/bind-login` 密码通过后只能返回 pending-OAuth 专用的
  two-factor challenge，不得绑定身份、消费 pending session 或签发 session cookie。
  `POST /api/v1/auth/oauth/pending/bind-login/2fa` 必须重新要求同一个 HttpOnly pending
  cookie；challenge 必须绑定 pending token hash，不能与普通登录 2FA challenge 或其它
  pending OAuth session 互换。
- pending OAuth email-completion / create-account / bind-login 响应和审计仍不得返回/记录 pending token、raw upstream
  subject、provider-subject hash、authorization code、provider access/refresh token、
  password、TOTP secret、TOTP code、session cookie、CSRF token 或完整 claims。

Current-user password changes:

- `POST /api/v1/me/password` 必须使用当前 console session，并要求 CSRF header。
- 服务端必须校验当前密码后才替换 password hash；请求不得携带或指定目标 user id。
- 改密成功后必须撤销该用户活跃 console session，并清除当前 session cookie。
- Audit 只能记录非敏感用户元数据，不得记录当前密码、新密码、password hash、
  session cookie 或 CSRF token。

## 4. API Key 安全

API Key 用于 Gateway API。

### 4.1 格式

当前格式（见 `apps/api/internal/modules/api_keys/service/service.go` `GeneratePlaintextKey`）：

```txt
sk_<prefix-hex>_<secret-hex>
```

- `sk` 固定前缀（`keyPrefix`）。
- `<prefix-hex>` 是 6 字节随机值的 hex（12 个十六进制字符），`prefix = "sk_" + hex`
  整体作为查找键持久化。
- `<secret-hex>` 是 32 字节 CSPRNG 随机值的 hex（64 个十六进制字符）。

要求：

- 原文只展示一次。
- `prefix`（即 `sk_<prefix-hex>`）用于快速定位，不用于认证。
- `secret` 必须使用 CSPRNG 生成。
- API Key 必须支持禁用、过期、模型范围、RPM/TPM 限制。

### 4.2 存储

数据库字段：

```txt
prefix
hash
status
scopes_json
allowed_models_json
rpm_limit
tpm_limit
concurrency_limit
expires_at
last_used_at
```

哈希方案（见 `api_keys/service/service.go` `HashPlaintext`）：

```txt
hash = "hmac-sha256:" + hex(HMAC-SHA256(server_pepper, full_api_key))
```

原因：

- API Key 是高熵随机值，不需要像用户密码一样使用慢哈希。
- HMAC pepper 可以防止数据库泄漏后离线验证 key。
- `server_pepper`（`API_KEY_PEPPER`）必须来自环境变量或密钥管理系统；service 构造时
  强制 pepper 至少 32 字节，否则返回 `ErrPepperUnavailable` 拒绝启动。
- key 校验使用 `hmac.Equal` 常量时间比较，避免计时旁路。

禁止：

- 持久化完整 API Key。
- 在日志、错误、audit before/after 中输出完整 API Key。
- 使用 prefix 作为认证依据。

删除行为：

- 删除 API Key 必须退出认证路径，后续请求返回 `invalid_api_key`。
- 删除后的软删除行可以继续保存 `prefix`、HMAC `hash`、owner user id 和 key name，
  用作认证失败排障墓碑。
- 墓碑归因必须同时满足合法 SRapi key 格式、prefix 命中软删除行、full API key
  HMAC 常量时间匹配；只命中 prefix 不得归因。
- `gateway.auth` 系统日志可以保存 `attempted_key_prefix`、`deleted_key_id`、
  `deleted_key_owner_user_id` 和 `deleted_key_name`；不得保存完整 API Key、secret
  段、Authorization header 或 HMAC。

## 5. Provider 凭证安全

Provider Account 的凭证包括：

```txt
api_key
oauth_access_token
oauth_refresh_token
oauth_device_code
web_session_cookie
desktop_client_token
cli_device_token
ide_plugin_token
service_account_json
custom_headers
custom_reverse_proxy_payload
```

必须加密字段：

```txt
provider_accounts.credential_ciphertext
provider_accounts.cookie_jar_ciphertext
provider_accounts.device_fingerprint_ciphertext
proxies.url_ciphertext
payment_provider_instances.config_ciphertext
settings.value_ciphertext when is_secret = true
```

### 5.1 加密方案

当前方案：

```txt
AES-256-GCM
```

每条密文必须包含或可关联：

```txt
key_version
nonce
ciphertext
auth_tag
aad_version
created_at
```

主密钥要求：

- 不得写入仓库。
- 本地开发通过 `.env` 提供。
- 生产环境应来自 KMS 或密钥管理系统。
- 支持 key version，为后续轮换做准备。

AES-GCM 要求：

- 同一 `key_version` 下 nonce 不得重复。
- 推荐使用 96-bit CSPRNG nonce。
- 必须使用认证附加数据 AAD 绑定资源上下文。
- AAD 至少包含 `resource_type`、`resource_id`、`field_name`、`key_version`。
- 解密失败必须返回内部错误，不得回退到明文或忽略认证失败。
- 生产环境优先使用 KMS 或 envelope encryption 管理主密钥。

### 5.2 解密边界

- 只有 Provider 调用前的受控路径可以解密。
- 解密后的凭证只存在内存中。
- Adapter 不得自行持久化明文凭证。
- 错误 details 不得包含凭证。

### 5.3 反代凭证额外要求

`runtime_class != api_key` 的反代凭证（cookie、OAuth token、device code、desktop / CLI / IDE token、device fingerprint 等）必须遵守：

- 必须经由 Reverse Proxy Runtime 注入，不得通过日志、metrics、scheduler scores_json、usage metadata 或 audit before/after 泄漏。
- 必须每账号独立 cookie jar、device fingerprint 与连接池，不得跨账号共享。
- 必须支持 OAuth 自动 refresh 与失败保护，详见 `REVERSE_PROXY_SPEC.md`。
- 备份导出必须二次加密或屏蔽。
- 反代请求向上游发出时，不得包含任何 SRapi 自身标识（`X-Request-ID`、`X-Forwarded-*`、`Via`、`X-SRapi-*`、`User-Agent: SRapi/*` 等）。
- 解题 / 验证码 / Cloudflare challenge 的中间 token 必须按 cookie jar 绑定，且不得跨账号复用。

### 5.4 账号导入导出安全边界

Provider Account 的 import/export 是运维便利接口，不是明文凭证备份通道。

- 导出接口不得返回 `credential`、`credential_ciphertext`、API Key、OAuth token、refresh token、Cookie、Authorization header、password、secret 或其他可复用凭证。
- 导出 metadata 必须递归移除敏感键，只保留非敏感操作字段，例如 `base_url`、策略标签、分组和代理绑定。
- 导出响应必须显式标记 `credential_exported: false`。
- 导入接口可以接收 write-only `credential` payload，但服务端必须立即进入 Provider Account 加密写入路径。
- 导入失败的错误消息、audit、日志和响应不得回显导入凭证内容。
- import 属于控制台写操作，必须要求登录态和 CSRF；export 属于管理读操作，仍必须要求登录态和 RBAC。

## 6. RBAC 与权限

内置角色（见 `apps/api/internal/modules/users/contract/contract.go`，角色名不可变）：

```txt
owner
admin
operator
user
```

权限边界：

| 能力 | owner | admin | operator | user |
| --- | --- | --- | --- | --- |
| 用户管理 | yes | yes | no | self only |
| Provider 管理 | yes | yes | read/test only | no |
| Provider Account 凭证写入 | yes | yes | no | no |
| Scheduler 策略修改 | yes | yes | no | no |
| Scheduler 查看 | yes | yes | yes | no |
| API Key 管理 | yes | yes | no | self only |
| Usage 查看 | yes | yes | yes | self only |
| Billing 调整 | yes | no by default | no | no |
| Audit 查看 | yes | yes | no | no |

所有管理端写操作必须写 audit log。

## 7. Prompt 与日志安全

默认不得记录完整用户 prompt、messages、tool arguments。

允许记录：

```txt
request_id
user_id
api_key_id
model
provider
account_id
input_token_estimate
output_tokens
latency_ms
error_class
status_code
```

禁止默认记录：

```txt
full prompt
full messages
API Key
Provider credential
OAuth token
Cookie
Authorization header
PII raw value
```

调试模式要求：

- 必须显式开启。
- 必须限制管理员权限。
- 必须脱敏 Authorization、Cookie、email、phone、token-like 字段。
- 必须有保留时间。
- 必须写入 audit log。

QualityEval 是默认关闭的特殊在线评估路径：

- 只有 `QUALITY_EVAL_ENABLED=true` 且配置了 judge API key 时才捕获样本并启动 worker。
- 样本只能来自成功完成并写入 `scheduler_feedbacks` 的文本 Gateway 请求，且捕获发生在 Gateway content-safety 脱敏之后。
- `quality_eval_samples.sample_payload_ciphertext` 只保存截断后的脱敏 prompt/output 摘要，必须用 `SRAPI_MASTER_KEY` 派生密钥 AES-GCM 加密；`quality_evaluations` 只保存 score、rubric、judge model 和不可逆 hash。
- LLM judge 调用会把脱敏样本发送给配置的 OpenAI-compatible Chat Completions endpoint；生产启用前必须按数据处理要求确认该外部评估端点。
- Usage、audit、scheduler decision、scheduler snapshot 仍不得保存原始 prompt/messages/tool arguments。

Console TOTP / 2FA:

- TOTP secret 只保存在 `user_totp_secrets.secret_ciphertext`，使用 `TOTP_ENCRYPTION_KEY` 派生 AES-GCM key 加密。
- 登录密码校验通过但用户启用 TOTP 时，只返回短期 `challenge_id`，不设置 console session cookie。
- `/api/v1/auth/login/2fa` 校验 challenge 与 TOTP/恢复码后才创建 session 并触发 last-login 更新。
- 恢复码只返回一次，数据库只保存 HMAC-SHA256 hash；成功使用后必须移除该 hash。
- setup/enable/disable 都是当前用户写接口，必须要求 HttpOnly session cookie 和 CSRF header。

## 8. Gateway 安全

Gateway 必须执行：

- API Key 鉴权。
- API Key 状态、过期时间、模型范围检查。
- RPM/TPM 限制。
- 用户余额或 entitlement 检查。
- 请求体大小限制。
- CanonicalRequest 文本字段的基础 PII 脱敏与 prompt-injection 命中记录。
- 上游超时限制。
- Provider 错误脱敏。

OpenAI-compatible 错误响应不得泄漏：

- 上游完整错误体中的 secret。
- 内部账号 id 以外的敏感配置。
- 数据库错误细节。
- 堆栈信息。

### 8.1 Content Safety 起步边界

Gateway admission 在 CanonicalRequest 生成后、Scheduler 和 Provider Adapter 之前执行 `content_safety` 扫描。当前策略是轻量级输入清洗与证据记录：

- 邮箱、手机号、SSN、身份证/国民 ID、信用卡文本会被替换为固定 redaction marker。
- `ignore previous instructions`、`developer mode`、`reveal/print/show your system prompt` 等 prompt-injection 关键词会产生 warning。
- 命中后写入 `gateway.content_safety` audit log，只记录 finding kind、severity、count、redacted，不记录原始 prompt 或 PII。
- usage log 的 `compatibility_warnings` 会保留 `content_safety_pii_redacted` / `content_safety_prompt_injection_detected`，用于后续运营分析和策略升级。

限制：

- 这是起步层，不等价于完整 LLM guardrail。
- 当前不会阻断请求；后续 block/mask/warn 策略必须显式配置并写入 audit。
- 图片、音频二进制内容、文件内部隐藏文本和间接 prompt injection 仍需专用 detector 或上游安全能力处理。

### 8.2 出站 SSRF 防护

反代 / Reverse Proxy Runtime 向上游发起的直连必须经过 SSRF egress 防护
（见 `apps/api/internal/modules/reverse_proxy/service/ssrf.go`），由
`runtime_state.go` 在 `SERVER_MODE != "local"` 时启用
（`WithBlockedPrivateEgress`）：

- 防护点在 `net.Dialer.Control`，即 DNS 解析之后拿到真实 `ip:port` 时执行，
  因此能拦截 URL 字符串检查无法识别的 DNS rebinding。
- 被拒绝的目标地址包括 loopback、unspecified、link-local（含云元数据
  `169.254.169.254` 与 `fe80::/10`）、各类 multicast、RFC1918 私网与 ULA
  `fc00::/7`、以及 RFC 6598 运营级 NAT `100.64.0.0/10`。
- 命中时返回 `egress_blocked`（502），错误消息不回显被拦截的内部地址。
- 同一加密 isolated client / transport 同时服务上游业务请求与 OAuth refresh 拨号，
  二者都受此防护覆盖。
- 显式配置了 egress 代理 URL 的账号被视为运维可信，按代理路由且当前版本不做该筛查。

## 9. Provider Adapter 安全

Adapter 禁止：

- 打印请求 header 中的 Authorization。
- 打印 Provider 凭证。
- 自行选择或切换账号。
- 自行持久化凭证。
- 对业务错误进行无限重试。

Adapter 必须：

- 使用平台层 HTTP client。
- 尊重请求 timeout。
- 对上游错误分类。
- 对日志字段脱敏。
- 将 usage 和 error feedback 回传。

## 10. AI / LLM 特有安全

至少遵守：

- 用户 prompt 不作为系统指令执行。
- 管理后台展示 prompt 片段时默认隐藏或脱敏。
- Tool call 参数必须验证 schema。
- Provider 返回内容不得直接注入管理后台 HTML。
- 前端不得使用 `dangerouslySetInnerHTML` 渲染模型内容，除非经过可信 sanitizer。

MCP、工具调用与文件处理已在网关支持；新增此类能力或 detector 时需同步扩展专项威胁模型。

## 11. 审计日志

必须审计：

- 登录失败次数异常。
- API Key 创建、禁用、删除。
- Provider 创建、更新、删除。
- Provider Account 创建、更新、禁用、凭证轮换。
- Scheduler 策略修改。
- Billing 手动调整。
- Settings secret 更新。
- 调试日志开关变更。

审计字段：

```txt
actor_user_id
action
resource_type
resource_id
before_json
after_json
ip
user_agent
trace_id
created_at
```

审计日志不得包含完整 secret。

## 12. 密钥轮换

加密层保留以下版本字段以支持轮换：

```txt
credential_version
settings key version
provider account credential version
```

轮换流程：

1. 新增新版本主密钥。
2. 新写入使用新版本。
3. 后台任务逐步重加密旧密文。
4. 验证所有旧版本迁移完成。
5. 禁用旧密钥。

轮换失败不能覆盖原密文。

## 13. 本地开发安全

`.env.example` 可以包含占位符，不得包含真实 secret。

必须忽略：

```txt
.env
.env.local
*.key
*.pem
```

本地默认管理员密码如果存在，必须只用于开发环境，并在 README 中标注不可用于生产。

### 13.1 生产启动拒绝弱默认值

为防止开发占位值被带进生产，配置校验在 `SERVER_MODE=release` 时拒绝启动
（见 `apps/api/internal/config/config.go` `Validate`），具体拒绝项：

- 弱或短（< 32 字节）或含 `local_dev` / `change_me` 等开发标记的
  `JWT_SECRET`、`SRAPI_MASTER_KEY`、`TOTP_ENCRYPTION_KEY`、`API_KEY_PEPPER`。
- 空或仍为开发值（如 `postgres` / `srapi` / `..._change_me`）的 `DATABASE_PASSWORD`。
- 仍为开发默认值或过短（< 12 字节）的 `BOOTSTRAP_ADMIN_PASSWORD`
  （如 `admin` / `admin123` / `..._change_me`）。
- release 模式下还拒绝 `STORAGE_BACKEND=memory`。

`SERVER_MODE=local` 下保留可用的开发默认值；这些默认值含显式 `change_me` /
`local_dev` 标记，从而被上述 release 校验捕获。

## 14. 安全测试清单

测试套件覆盖：

- API Key 原文不落库测试。
- API Key hash 校验测试。
- 禁用 API Key 后不可调用 Gateway。
- Provider 凭证加密和解密失败处理测试。
- Cookie 安全属性测试。
- CSRF 写操作拦截测试。
- RBAC 管理接口权限测试。
- 反代请求不向上游泄漏 SRapi 自有 Header 的回归测试。
- 反代凭证（cookie / OAuth token / device fingerprint）跨账号隔离测试。
- 反代账号封号信号识别与 needs_reauth / disabled 自动切换测试。
- 反代出站 SSRF 防护（loopback / RFC1918 / link-local 元数据 / CGNAT 等）拦截测试。
- 生产模式弱默认 secret / 默认管理员密码启动拒绝测试。
- 日志不包含 Authorization / Cookie / Provider credential 测试。
- Provider 错误脱敏测试。
- Audit log 高风险操作测试。

## 15. Ship Gate 安全门禁

上线前必须确认：

- 没有真实 secret 提交到仓库。
- 没有 API Key 原文持久化。
- 没有 Provider 凭证明文持久化。
- 管理写操作有 RBAC 和 Audit。
- Gateway 有 rate limit。
- Cookie 使用安全属性。
- CSRF 已启用。
- 日志脱敏策略已验证。
- 生产模式启动拒绝弱默认 secret 与默认管理员密码（`SERVER_MODE=release`）。
- 反代出站 SSRF 防护在非 local 模式启用。
- OpenAPI securitySchemes 与实际 middleware 一致。
