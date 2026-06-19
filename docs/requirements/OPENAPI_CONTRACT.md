# SRapi OpenAPI 契约规范

## 1. 目标

SRapi 使用 OpenAPI-first 工作流。OpenAPI 契约是前端、后端、SDK、文档和测试之间的唯一 HTTP 接口来源。

目标：

- 避免前后端接口漂移。
- 为前端生成类型安全 client。
- 为后端生成请求/响应类型和 server interface。
- 为第三方调用者生成文档。
- 支持 CI 检查接口破坏性变更。

## 2. 文件组织

契约是单一权威文件：

```txt
packages/openapi/
├── openapi.yaml                # 唯一契约源，~700KB，约 361 个 operationId
└── oapi-codegen.server.yaml    # 后端 server 代码生成配置
```

`packages/openapi/openapi.yaml` 内联了全部 `paths`、`components/schemas`、`securitySchemes` 和示例，无需 `$ref` 到分文件目录。早期设计曾考虑拆分为 `paths/`、`schemas/`、`examples/` 多文件结构，但实际落地采用单文件维护，以保证 lint / bundle / breaking-change 检查和代码生成都对一个确定来源运行。新增或修改接口直接编辑这一个文件，再跑生成与 CI 校验即可。

## 3. API 分区

### 3.1 控制台用户 API

```txt
/api/v1/me
/api/v1/api-keys
/api/v1/usage
/api/v1/billing
/api/v1/subscriptions
/api/v1/payment
/api/v1/affiliate
```

用于普通用户控制台。

### 3.2 管理员 API

```txt
/api/v1/admin/users
/api/v1/admin/roles
/api/v1/admin/providers
/api/v1/admin/models
/api/v1/admin/accounts
/api/v1/admin/scheduler
/api/v1/admin/subscription-plans
/api/v1/admin/user-subscriptions
/api/v1/admin/pricing-rules
/api/v1/admin/payments
/api/v1/admin/affiliates
/api/v1/admin/ops
/api/v1/admin/settings
/api/v1/admin/notifications
/api/v1/admin/dashboard/snapshot
/api/v1/admin/announcements
/api/v1/admin/redeem-codes
/api/v1/admin/promo-codes
/api/v1/admin/risk-control
```

需要管理员权限。

### 3.3 网关兼容 API

```txt
/v1/models
/v1/chat/completions
/v1/responses
/v1/responses/compact
/v1/messages
/v1/messages/count_tokens
/v1/embeddings
/v1/images/generations
/v1/images/edits
/v1/images/variations
/v1/audio/transcriptions
/v1/audio/speech
/v1/moderations
/v1/rerank
/v1/responses/ws
/v1/realtime
/v1beta/models
/v1beta/models/{model}
/v1beta/models/{model}:countTokens
```

这些接口面向 API 客户端，必须兼容 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 等主流 AI 端点风格。

Gateway 所有 AI 端点必须先转换为 `AI_ENDPOINT_COMPATIBILITY.md` 定义的 Canonical AI Request，再进入 Scheduler。

Gateway 路由族、Provider alias、passthrough 和 WebSocket 阶段边界以 `GATEWAY_ROUTE_MATRIX.md` 为准。`/v1/images/edits` 的 OpenAPI contract 同时描述 multipart form-data 和 application/json image reference bodies；JSON references 只允许本地 data URL / base64 payload，remote URL 与 `file_id` references 由运行时明确拒绝，直到 Files API / remote-fetch 安全边界实现。`stream=true` 返回 `text/event-stream`，当前 v1 只输出最终 `image.generation.result` chunk 和 `[DONE]`。`/v1/responses/compact` 复用 `ResponsesRequest` 和标准 Gateway 证据链，调度前要求 `responses_compact` effective capability；同协议 Codex/OpenAI compact 响应按原始 `response.compaction` JSON 回放，不在跨协议路径伪造压缩语义。`/v1/responses/ws` 在 OpenAPI 中以 WebSocket upgrade route 表达；运行时 JSON frame schema 复用 `ResponsesRequest`，不新增 provider-specific Gateway DTO。`/v1/realtime` 是 OpenAI-compatible Realtime WebSocket upgrade route，模型从 query `model` 解析；API-key accounts 走官方 API-key upstream relay，OAuth/session/client-token accounts 走 Reverse Proxy Runtime 双向 relay；不是 `POST /v1/realtime`。`GET /v1beta/models` 与 `GET /v1beta/models/{model}` 是 Gemini-compatible models.list / models.get 形状，只做 Gateway API Key 鉴权、模型可见性过滤和 Google-shaped rendering，不获取 Scheduler lease，也不读取 Provider Account 凭证。`POST /v1beta/models/{model}:countTokens` 是 Gemini-compatible countTokens 形状，走 Gateway API Key、模型可见性、entitlement、Scheduler、Provider Adapter 和 Reverse Proxy Runtime 边界；成功请求只记录 Scheduler/request evidence，generation usage tokens 和 cost 保持 0。`POST /v1/messages/count_tokens` 是 Anthropic-compatible count_tokens 形状；Gateway 只用归一化结果做 policy / entitlement / Scheduler / evidence，Provider Adapter 保留 Anthropic body 形状并把 `model` 映射成上游模型后调用 `/messages/count_tokens`，成功请求同样不进入生成用量和成本。

### 3.4 Webhook API

```txt
/api/v1/webhooks/payments/{provider}
```

Webhook 必须支持幂等处理。

### 3.5 运维 API

```txt
/livez
/readyz
/metrics
```

运维端点的认证、探针语义和生产限制以 `OPERATIONS.md` 为准。

## 4. 鉴权规范

### 4.1 控制台 API

控制台 API 推荐：

```txt
HttpOnly Cookie + CSRF Token
```

请求头：

```txt
X-CSRF-Token: <token>
```

### 4.2 Gateway API

Gateway API 使用：

```txt
Authorization: Bearer sk-xxxx
```

### 4.3 管理员 API

管理员 API 使用控制台登录态，并要求 RBAC 权限。

Admin route 的 `x-srapi-rbac` 可以声明内置角色权限，也可以声明细粒度 permission：

```yaml
x-srapi-rbac:
  owner: read
  admin: read
  permission: payment_order:read
```

运行时保留 `owner/admin` 超级管理权限；普通用户必须通过自定义角色获得对应 `resource:action` permission 才能访问该 Admin API。Role 名称是可扩展 string，不再限制为固定 enum。

### 4.4 Security Schemes

OpenAPI 契约必须显式声明安全方案：

```yaml
components:
  securitySchemes:
    cookieAuth:
      type: apiKey
      in: cookie
      name: srapi_session
    csrfHeader:
      type: apiKey
      in: header
      name: X-CSRF-Token
    gatewayBearerAuth:
      type: http
      scheme: bearer
```

规则：

- 控制台读接口使用 `cookieAuth`。
- 控制台写接口使用 `cookieAuth` + `csrfHeader`。
- Gateway `/v1/*` 接口使用 `gatewayBearerAuth`。

### 4.5 Public Registration

Public console registration is an unauthenticated auth route:

```txt
POST /api/v1/auth/register
```

The route is enabled only when admin settings
`security.registration_enabled=true`. If
`security.registration_email_suffix_allowlist` is non-empty, the request email
domain must exactly match one normalized suffix such as `@example.com`; an empty
list allows all valid email domains. The route creates a regular user account,
applies the configured default balance and default RPM limit, sets the HttpOnly
console session cookie, and returns the standard `LoginResponse` with the
one-time CSRF token. Duplicate email, suffix-policy rejection, and malformed
input responses must use the same generic `400 INVALID_REQUEST` shape so callers
cannot use registration errors to enumerate existing accounts or configured
registration domains.

Email verification and password reset are handled by the dedicated
unauthenticated contracts below.

### 4.5.1 Email Verification

Public console email verification uses two unauthenticated auth routes:

```txt
POST /api/v1/auth/email-verification/request
POST /api/v1/auth/email-verification/confirm
```

`request` accepts an email and returns the same `202` accepted response for
syntactically valid input whether the email is missing, inactive, already
verified, or linked to an active unverified user. When an active unverified user
exists, SRapi stores only a keyed hash of the single-use verification token in
`email_verification_tokens` and enqueues `AuthEmailVerificationRequested` with
an encrypted verification token payload for later mail delivery. The event
payload must not contain the plaintext email, plaintext token, password,
session cookie, or CSRF token.

`confirm` accepts the verification token, consumes it exactly once, and marks
the user's primary email as verified by setting `users.email_verified_at`.
Invalid, expired, or already consumed tokens return `401`. Confirming email
ownership does not create a login session.

### 4.5.2 Password Reset

Public console password recovery uses two unauthenticated auth routes:

```txt
POST /api/v1/auth/password-reset/request
POST /api/v1/auth/password-reset/confirm
```

`request` accepts an email and returns the same `202` accepted response for
syntactically valid input whether or not an active user exists. When an active
user exists, SRapi stores only a keyed hash of the single-use reset token in
`password_reset_tokens` and enqueues `AuthPasswordResetRequested` with an
encrypted reset token payload for later mail delivery. The event payload must
not contain the plaintext email, plaintext token, password, session cookie, or
CSRF token.

`confirm` accepts the reset token and new password, consumes the token exactly
once, replaces the user's password hash, and revokes active console sessions
for that user. Invalid, expired, or already consumed tokens return `401`.

### 4.5.3 OAuth/OIDC Browser Flow

Public OAuth/OIDC authorization starts through:

```txt
GET /api/v1/auth/oauth/{provider}/start
GET /api/v1/auth/oauth/{provider}/callback
```

The start route is unauthenticated but only works when Admin Settings enable OAuth
and provide a matching non-secret provider config. It returns `302` to the
provider authorization endpoint and sets a short-lived encrypted HttpOnly
`srapi_oauth_flow` cookie scoped to `/api/v1/auth/oauth`.

The authorization request always uses authorization code + PKCE S256, includes
`state`, and includes `nonce` for OpenID scopes. The encrypted flow cookie binds
state, PKCE verifier, nonce, provider, provider key, intent, local redirect
path, creation time, and expiry. The cookie must not expose provider client
secret, authorization code, token material, raw subject, profile claims, session
cookie, CSRF token, or API key material.

The callback route validates the encrypted flow cookie and returned `state`,
uses the stored PKCE verifier to exchange the authorization code at the
configured token endpoint, fetches UserInfo with a bearer access token, and
creates a short-lived pending OAuth session. The pending session stores only a
keyed hash of the upstream subject plus a safe profile summary, then sets an
HttpOnly `srapi_oauth_pending` cookie scoped to `/api/v1/auth/oauth/pending`
and redirects to the bound local console path. The callback supports public
clients with `token_auth_method=none` and confidential clients: when a provider
client secret is configured (env `OAUTH_CLIENT_SECRETS_JSON`, keyed by
`provider_key`, never on the Admin Settings / OpenAPI / SDK surface), the token
exchange sends `client_secret` (client_secret_post) alongside PKCE. When an OIDC
issuer is configured for the provider (env `OAUTH_ISSUERS_JSON`), the callback
also verifies the returned `id_token` via OIDC discovery + JWKS (RS256 signature,
`iss` / `aud` / `exp`, and the flow-bound `nonce`); a verification failure
rejects login with `PROVIDER_AUTH_FAILED`. Final account adoption (creating or
binding to a local account, with opt-in provider display-name adoption) is
implemented through the pending-decision routes below.

The console can inspect the pending decision through:

```txt
GET  /api/v1/auth/oauth/pending
POST /api/v1/auth/oauth/pending/bind-current-user
POST /api/v1/auth/oauth/pending/send-verify-code
POST /api/v1/auth/oauth/pending/email-completion/confirm
POST /api/v1/auth/oauth/pending/create-account
POST /api/v1/auth/oauth/pending/bind-login
POST /api/v1/auth/oauth/pending/bind-login/2fa
```

The `GET` route reads only the HttpOnly `srapi_oauth_pending` cookie and returns a
safe pending-session preview: provider, provider key, display-safe subject
hint, normalized profile summary, local redirect, expiry, and a `next_step`
decision such as `email_completion_required`, `bind_existing_login_required`,
or `create_account_required`. The route is read-only and must not consume the
pending session; it never returns the pending token, raw upstream subject,
provider-subject hash, authorization code, provider access/refresh token,
session cookie, CSRF token, or full upstream claims. When `next_step` is
`create_account_required`, the preview also returns a short-lived
`create_account_action` token. That token is signed, bound to the current
pending OAuth cookie, and usable only for the create-account mutation.

The `POST /bind-current-user` route requires `cookieAuth` plus `csrfHeader`.
It binds the pending external identity to the authenticated console user, rejects
identities already owned by another user, consumes the pending token after a
successful bind, clears the pending cookie, and returns the refreshed current
user auth identity list. It does not create accounts, validate passwords, or
issue login sessions.

The `POST /send-verify-code` route is for pending OAuth sessions whose provider
profile did not contain a usable email. It requires only the HttpOnly pending
cookie and a submitted email address, then enqueues a high-entropy encrypted
email-completion link through the transactional outbox. The response is uniform
`202` and does not reveal whether the email already belongs to a local account.
The follow-up `POST /email-completion/confirm` route requires the same pending
cookie plus the emailed token. It writes the verified email into the pending
session and returns a fresh safe pending preview; it does not consume the pending
session or issue a console session.

The `POST /create-account` route is for unauthenticated browser completion when
the provider returned a verified email that does not belong to an existing local
account. It requires the HttpOnly pending cookie plus the `create_account_action`
token from the preview response. SRapi validates the pending session, action
token, registration settings, email suffix policy, and email match before
creating the account. On success it binds the external identity, consumes and
clears the pending cookie, sets a normal console session cookie, and returns
`LoginResponse`. If binding or pending consumption fails before a session is
issued, SRapi rolls back the newly created local user so the email is not left
reserved by a half-completed OAuth flow.

The `POST /bind-login` route is for unauthenticated browser completion when the
pending provider profile should be attached to an existing console account. It
requires the pending cookie plus local email/password credentials. If password
authentication succeeds and the account has no second factor enabled, SRapi binds
the external identity to that account, consumes the pending token, clears the
pending cookie, sets a normal console session cookie, and returns `LoginResponse`.
If the account requires TOTP, it returns `202 LoginTwoFactorRequiredResponse`
without binding or consuming the pending token. The follow-up
`POST /bind-login/2fa` requires the same pending cookie and a pending-OAuth
specific challenge id; the challenge is bound to the pending token and cannot be
reused as a normal login challenge or with another pending session.

Only `intent=login` is accepted by this v1 start/callback flow. The explicit
CSRF-protected bind route may adopt either a login pending session or a future
`bind_current_user` pending session for the currently authenticated user.

### 4.5.4 Transactional Auth Email Delivery

Password reset and email verification delivery is handled by the domain-event
outbox worker, not by the public auth request handlers. When
`EMAIL_PUBLIC_BASE_URL`, `EMAIL_SMTP_HOST`, and `EMAIL_SMTP_FROM` are configured,
the worker consumes `AuthPasswordResetRequested` and
`AuthEmailVerificationRequested`, decrypts the token payload with the
deployment master key, and renders the configured action path with a `token`
query parameter.

The dispatcher must validate the current user record before sending: inactive
users are skipped, and the current `users.email` hash must match the event's
recipient email hash. This makes stale queued events harmless after an email
change. Missing email configuration leaves delivery retryable/failed in the
outbox rather than silently marking the event as published. SMTP password
material is deployment env only and is not part of the Admin Settings API,
generated SDK request types, responses, or audit snapshots.

### 4.5.5 Optional Notification Unsubscribe

Optional notification email preferences use public notification routes:

```txt
GET  /api/v1/notifications/unsubscribe?token=...
POST /api/v1/notifications/unsubscribe
```

`GET` validates a signed unsubscribe token without changing state. `POST`
applies the preference and accepts the token from a query parameter, JSON body,
or form field so one-click unsubscribe clients can submit the generated link.
Tokens contain the event name, an email hash, and expiry; they do not contain
the plaintext email address.

Only optional notification events such as `balance.low`,
`subscription.expiry_reminder`, and `account.quota_alert` can be unsubscribed.
Transactional auth and contact-verification mail such as password reset, email
verification, and `notification.contact_verification` is not suppressible
through this route and does not include unsubscribe headers.

Current users can also manage the same optional event preferences directly:

```txt
GET /api/v1/me/notification-preferences
PUT /api/v1/me/notification-preferences
```

`GET` returns the optional notification event catalog and the current user's
subscription state. `PUT` requires `cookieAuth` plus `csrfHeader` and updates
only allowlisted optional events for the current user's primary email hash. The
stored preference state is shared with one-click unsubscribe and does not store
plaintext recipient emails.

Current users manage secondary notification recipients through the separate
verified-contact API:

```txt
GET    /api/v1/me/notification-contacts
POST   /api/v1/me/notification-contacts
POST   /api/v1/me/notification-contacts/verify
PATCH  /api/v1/me/notification-contacts/{id}
DELETE /api/v1/me/notification-contacts/{id}
```

`POST` requires `cookieAuth` plus `csrfHeader`, creates or reuses a pending
contact, and enqueues `NotificationContactVerificationRequested` with encrypted
recipient email and encrypted verification token payloads. The primary account
email cannot be added as a secondary contact. `verify` consumes the signed,
time-limited token from the current session and marks the contact verified.
Optional notification delivery includes only verified, enabled contacts and
still applies each recipient email's event-scoped unsubscribe preference and
one-click unsubscribe headers.

`AdminSettings.email` also carries the low-balance trigger defaults:
`balance_low_notify_enabled`, `balance_low_notify_threshold`, and
`balance_low_notify_recharge_url`. These settings control when the
`balance.low` optional notification is enqueued after balance charging. The
actual email remains outbox-dispatched and preference-aware; the charge worker
does not call SMTP directly.

`subscription_expiry_notify_enabled` controls the optional
`subscription.expiry_reminder` trigger. The subscription expiry worker scans
active subscriptions and enqueues 7-day, 3-day, and 1-day reminder events with
subscription metadata only. Email rendering, one-click unsubscribe handling,
and SMTP retry stay inside the outbox notification dispatcher.

`account_quota_notify_enabled` and
`account_quota_notify_remaining_ratio` control the optional
`account.quota_alert` trigger. The account quota alert worker scans persisted
provider-account quota snapshots and enqueues an alert only when the latest
remaining ratio crosses downward through the configured threshold. Dispatch is
still outbox-backed, preference-aware, and SMTP-retryable.

### 4.6 Current-User Profile

Current users can update the small set of profile fields they own:

```txt
PATCH /api/v1/me
```

The route requires `cookieAuth` plus `csrfHeader` and currently accepts only
`name`. The request schema is an explicit allowlist: callers cannot update
email, roles, status, balance, RPM limits, password, avatar, or other
admin-managed fields through this route. Email changes, notification emails,
and identity bindings remain separate flows because they need verification or
provider-specific OAuth handling.

Current-user avatar storage is a dedicated flow:

```txt
PUT    /api/v1/me/avatar
DELETE /api/v1/me/avatar
GET    /api/v1/users/{id}/avatar
```

Avatar writes require `cookieAuth` plus `csrfHeader` and accept only
`multipart/form-data` field `avatar`. SRapi decodes PNG/JPEG uploads, rejects
files over 1 MiB or images above 1024x1024, re-encodes accepted images as PNG,
stores only SRapi-owned image bytes and metadata, and serves the normalized
image through `GET /api/v1/users/{id}/avatar` with `image/png` and `nosniff`.
`GET /api/v1/me` includes avatar metadata and a relative `avatar_url` when the
current user has configured an avatar.

Current-user auth identity directory:

```txt
GET    /api/v1/me/auth-identities
DELETE /api/v1/me/auth-identities/{id}
```

The route requires `cookieAuth` and returns the local email sign-in identity
plus verified external OAuth/OIDC identities already bound to the current user.
External identities expose only `provider`, `provider_key`, display-safe
`subject_hint`, non-sensitive profile summary fields, and timestamps. Raw
upstream subjects, OAuth codes, tokens, cookies, provider secrets, and
credential payloads are never part of this contract.
The delete route requires `cookieAuth` plus `csrfHeader`, removes one external
identity by its persistent id, returns the refreshed identity list, and never
accepts provider-wide unbinds that could remove another provider instance
accidentally. The derived local email identity has no persistent id and cannot
be unbound through this endpoint.
paths.

### 4.7 Current-User Password

Current users can change their own console password:

```txt
POST /api/v1/me/password
```

The route requires `cookieAuth` plus `csrfHeader`, verifies the current
password before replacing the stored password hash, revokes active console
sessions for that user, clears the current session cookie, and returns `204`.
Audit evidence records only non-secret user metadata; it must not include either
password, session cookie, CSRF token, or password hash.

### 4.8 Current-User Announcements

Current-user announcements are read from the console control plane:

```txt
GET  /api/v1/me/announcements
POST /api/v1/me/announcements/{id}/read
```

`GET /api/v1/me/announcements` returns only published announcements visible to
the current user's role and active time window. Each item includes read state
from `user_announcement_reads`; if an announcement is updated after `read_at`,
it is treated as unread again. `POST /api/v1/me/announcements/{id}/read` is a
console write and requires `cookieAuth` plus `csrfHeader`.

### 4.9 Console TOTP / 2FA

Console TOTP routes are part of the current-user control plane:

```txt
POST /api/v1/auth/login/2fa
GET  /api/v1/me/totp/status
POST /api/v1/me/totp/setup
POST /api/v1/me/totp/enable
POST /api/v1/me/totp/disable
```

`POST /api/v1/auth/login` keeps the existing `200 LoginResponse` shape for
users without TOTP. When TOTP is enabled it returns `202
LoginTwoFactorRequiredResponse`, does not set the session cookie, and provides
a short-lived `challenge_id`. `POST /api/v1/auth/login/2fa` accepts that
challenge plus a TOTP or recovery code, sets the session cookie on success, and
returns the standard `LoginResponse`.

Current-user setup, enable, and disable routes are console writes and must
require `cookieAuth` plus `csrfHeader`. Setup returns the enrollment secret and
`otpauth://` URI once; enable verifies a TOTP code and returns one-time
recovery codes. Recovery codes are accepted for login/disable but must be
stored only as keyed hashes and consumed on first use.

### 4.10 Current-User Redeem Codes

Current-user redeem-code application is a console write:

```txt
POST /api/v1/me/redeem-codes/redeem
```

The route requires `cookieAuth` plus `csrfHeader`. It accepts a code, normalizes
it case-insensitively, and returns a `RedeemCodeRedemptionResult` containing
the redemption receipt, the updated redeem-code summary, and an
`already_redeemed` flag. Balance codes credit the current user's balance and
produce a `billing_ledger` entry with `type=redeem_code_credit`; subscription
codes create a user subscription from the referenced plan and materialize
entitlements. Repeating the same code for the same user returns the original
receipt without a second balance/subscription side effect.

### 4.11 Current-User Promo Codes

Current-user promo-code application is part of payment order creation:

```txt
POST /api/v1/payment/orders
```

The route already requires `cookieAuth` plus `csrfHeader`.
`CreatePaymentOrderRequest` accepts optional `promo_code`; `PaymentOrder`
returns `original_amount`, `discount_amount`, and nullable `promo_code_id`.
The server validates and applies the promo before provider checkout creation,
so the payment provider only sees the final payable `amount`. Persistent stores
write the payment order, `user_promo_code_applications` receipt, and promo-code
`used_count` increment atomically.

### 4.12 AdminOps System Logs

AdminOps system-log routes are part of the console control plane:

```txt
GET  /api/v1/admin/ops/system-logs
GET  /api/v1/admin/ops/system-logs/health
POST /api/v1/admin/ops/system-logs/cleanup
```

`GET /api/v1/admin/ops/system-logs` returns sanitized structured events from
`ops_system_logs`. Query filters may include `level`, `source`, `q`, `start`,
and `end`; pagination uses the existing admin list defaults. System-log
record/list/cleanup/health behavior is owned by the operations module, not by
admin-control settings state. Gateway provider-attempt failures and gateway
usage-log write failures must be visible here even when the richer
`ops_error_logs` writer is unavailable, so operators can diagnose observability
breakage from the console.

`GET /api/v1/admin/ops/system-logs/health` returns store health evidence:
storage mode, writable/degraded/stale flags, total rows, level counts, last log
time, and last error summary. It also returns `error_evidence_recorder`
runtime health for the asynchronous `ops_error_logs` writer: enabled/started/
draining flags, queue depth/capacity, enqueued/processed/recorded counters, and
dropped/write-failure counters. Store facts must derive from store statistics,
not from a sampled list page; recorder facts must derive from the active runtime
snapshot so operators can see when detailed error evidence is being dropped.

`POST /api/v1/admin/ops/system-logs/cleanup` is a write route and must require
`cookieAuth` plus `csrfHeader`. Cleanup requires at least one bounded filter,
supports `dry_run`, caps `max_delete`, and writes a safe audit record with
matched/deleted counts and normalized filter metadata. Audit payloads must not
copy raw log messages, raw search strings, credentials, prompts, cookies, API
keys, or provider-native frames.

### 4.13 Admin Request Log Files

Request-log-file routes are the admin surface for optional gateway HTTP
envelope captures:

```txt
GET    /api/v1/admin/request-log-files
GET    /api/v1/admin/request-log-files/{name}
GET    /api/v1/admin/request-log-files/{name}/download
DELETE /api/v1/admin/request-log-files/{name}
```

`GET /api/v1/admin/request-log-files` returns generated
`RequestLogFileDescriptor` rows sorted newest-first. Query filters include
`request_id` as a prefix match, `error_only`, `from`, `to`, and `limit`. This
is the supported correlation path from `ops_error_logs.request_id` to the
captured raw request/response envelope.

`GET /api/v1/admin/request-log-files/{name}/download` returns the raw dump as
`text/plain`; it must not be modeled as binary in OpenAPI because the browser
SDK parses it as text for preview.

`DELETE /api/v1/admin/request-log-files/{name}` removes a captured file and is
a write route. It must require `cookieAuth` plus `csrfHeader`.

### 4.14 AdminOps Error Logs

AdminOps error-log routes are the durable operator-facing upstream failure
surface:

```txt
GET   /api/v1/admin/ops/error-logs
GET   /api/v1/admin/ops/error-logs/fingerprints
GET   /api/v1/admin/ops/error-logs/{id}
PATCH /api/v1/admin/ops/error-logs/{id}
```

`GET /api/v1/admin/ops/error-logs` returns persisted `ops_error_logs` rows,
not usage-derived guesses. Query filters may include `user_id`, `account_id`,
`provider_id`, `model`, `source_endpoint`, `error_class`, `platform`,
`resolution`, `status_min`, `status_max`, `start`, `end`, and `q`. Pagination
uses the shared `Pagination` schema.

`GET /api/v1/admin/ops/error-logs/fingerprints` returns a bounded real-time
aggregation over `ops_error_logs` for incident triage. It supports the same
safe filters as the list route plus `limit` (default 20, max 100), defaults to a
24 hour window when `start` is omitted, and returns `{data, meta, request_id}`.
`meta` must expose `total`, `scanned`, `truncated`, `window_start`, and
`window_end` so operators know whether the live scan covered the requested
window. When `truncated=true`, `total` is the number of groups discovered in
the scanned sample before the response limit, not a full-window cardinality.
Fingerprint keys may use only low-sensitivity dimensions such as
endpoint, target protocol, model, status class/code, error class/phase/owner/
source, and normalized message pattern. They must not include request ids,
API key ids, user/account/provider raw identifiers, request or response bodies,
prompts, credentials, cookies, or other high-cardinality secrets.

`GET /api/v1/admin/ops/error-logs/{id}` returns the same sanitized row in a
`{data, request_id}` envelope for detail dialogs and incident links. The row
includes structured operational evidence such as source/target protocol,
attempt number, latency, estimated token counts, upstream request id,
error owner/source, and bounded `upstream_errors` attempt history.

`PATCH /api/v1/admin/ops/error-logs/{id}` is a write route and must require
`cookieAuth` plus `csrfHeader`. It accepts `resolution` (`open`,
`investigating`, `resolved`, `muted`) and optional `note`, sets `resolved_at`
only for `resolved`, and returns the updated row. Response IDs follow the
OpenAPI `Id` type as strings.

These routes intentionally do not store or expose raw request bodies, headers,
prompts, credentials, or replay payloads. Error logs carry enough structured
evidence to diagnose provider/account/model failures while keeping replay
semantics in the separate idempotency and scheduler snapshot surfaces.

### 4.15 Admin Live Error Stream

`GET /api/v1/admin/error-stream` returns a `text/event-stream` feed for live
gateway provider-attempt failures. The stream emits `event: gateway_error`
frames; each `data` line is a sanitized JSON payload with `request_id`,
`trace_id`, provider/account ids and names, model mapping, endpoint/protocol,
attempt number, upstream status/request id, error classification, message, and
bounded redacted body excerpt.

The `trace_id` must come from the request context, not from the client payload
or provider body, and it must match the trace id written to `ops_system_logs`
and `ops_error_logs` for the same gateway request. Admin UI deep links may use
`request_id` plus `trace_id` to pivot from the live feed to durable system logs
and error logs. The stream must never include raw prompt, raw request body,
Authorization header, cookie, credential material, or provider secret.

### 4.16 Provider Account Import / Export

账号池导入导出接口必须保持凭证安全边界：

- `GET /api/v1/admin/accounts/export` 只导出账号元数据、分组、状态、权重、代理绑定等可操作字段，不得返回 `credential`、`credential_ciphertext`、OAuth token、Cookie、API Key 或 refresh token。
- 导出响应必须包含 `credential_exported: false`，用于提醒调用方该 payload 不能作为完整备份凭证源。
- 导出 metadata 必须递归移除敏感键，例如 `api_key`、`access_token`、`refresh_token`、`authorization`、`cookie`、`secret`、`password`、`token`。
- `POST /api/v1/admin/accounts/import` 的凭证字段是 write-only 输入；服务端必须通过 Provider Account 凭证加密边界持久化，不得在响应、audit before/after、错误 details 或日志中回显。
- import/export 写语义以 OpenAPI schema 为准：export 是读接口使用 `cookieAuth`，import 是写接口必须使用 `cookieAuth` + `csrfHeader`。
- `POST /api/v1/admin/accounts/{id}/discover-models` 用于发现 upstream model catalog；默认只返回预览结果，`persist=true` 才写入 `supported_models`、`model_discovery_source` 和 `model_discovery_last_seen_at`。
- discovery 响应的 `endpoint` 必须是脱敏后的 upstream endpoint；如果实际请求使用 query API key 等凭证形式，凭证不得出现在响应、audit 或日志中。
- WP-500 起，`reverse-proxy-antigravity` 非 API-key 账号也支持 discovery，source 为 `reverse-proxy-antigravity`，通过 Reverse Proxy Runtime 使用选中账号凭证 POST 到 `{base_url}/v1internal:fetchAvailableModels`。
- WP-530 起，Antigravity discovery 在账号缺少 project metadata 时会先通过同一 Reverse Proxy Runtime / selected account credential 调用 `{base_url}/v1internal:loadCodeAssist`，必要时调用 `{base_url}/v1internal:onboardUser`，再进行模型发现；`persist=true` 时写回解析到的 project metadata。
- 该 discovery 结果必须用于后续 Provider Account model 选择，保持 `supported_models` 与现有 Scheduler/Gateway 边界一致。

### 4.17 RBAC Matrix

管理员接口必须在 OpenAPI 描述中标注权限需求。

| 能力 | owner | admin | operator | user |
| --- | --- | --- | --- | --- |
| `/api/v1/admin/users` | yes | yes | no | no |
| `/api/v1/admin/providers` | yes | yes | read/test only | no |
| `/api/v1/admin/models` | yes | yes | read only | no |
| `/api/v1/admin/accounts` | yes | yes | read/test only | no |
| `/api/v1/admin/scheduler` | yes | yes | read/simulate only | no |
| `/api/v1/admin/ops/realtime/slots` | yes | yes | read only | no |
| `/api/v1/admin/ops/slo` | yes | yes | read only | no |
| `/api/v1/admin/ops/alerts` | yes | yes | read only | no |
| `/api/v1/admin/settings` | yes | yes | no | no |
| `/api/v1/admin/notifications/email-templates` | yes | yes | no | no |
| `/api/v1/admin/dashboard/snapshot` | yes | yes | read only | no |
| `/api/v1/admin/announcements` | yes | yes | no | no |
| `/api/v1/admin/redeem-codes` | yes | yes | no | no |
| `/api/v1/admin/promo-codes` | yes | yes | no | no |
| `/api/v1/admin/risk-control` | yes | yes | read only | no |
| `/api/v1/admin/audit-logs` | yes | yes | no | no |

### 4.18 Ops SLO / Alert APIs

`/api/v1/admin/ops/slo` 和 `/api/v1/admin/ops/alerts` 属于 AdminOps 控制面：

- `GET /api/v1/admin/ops/slo` 返回 SLO definition 以及基于 `usage_logs` 计算的 availability/burn-rate 证据。
- `POST /api/v1/admin/ops/slo`、`PATCH /api/v1/admin/ops/slo/{id}` 必须使用 `cookieAuth` + `csrfHeader`，并记录安全 audit before/after。
- SLO `objective` 请求可接受 `0.995` 或 `99.5`；响应统一返回比例值。
- `GET /api/v1/admin/ops/alerts` 支持 `status`、`severity` 过滤。
- `GET/POST/PATCH/DELETE /api/v1/admin/ops/alert-rules{,/{id}}` 管理内置 metric 告警规则；规则 scope 可用低基数 `source_endpoint`、`model`、`provider_id` 和 `error_class` 限定评估范围，不得引入 API key、用户邮箱、prompt 或 credential 等高基数/敏感维度。
- `GET/POST/DELETE /api/v1/admin/ops/alert-silences{,/{id}}` 管理静默窗口；matcher 允许同样的低基数 scope 字段和 `rule_id` / `severity`，用于维护期压制匹配告警。
- `POST /api/v1/admin/ops/alerts/{id}/ack` 必须使用 CSRF，并且 audit 只记录 ack 摘要，不复制 alert `details`。
- `GET/POST/PATCH/DELETE /api/v1/admin/ops/notification-channels{,/{id}}` 管理内置 Ops alert email 通道；响应只能返回通道元数据和收件目标，不得返回 SMTP secret。
- `GET /api/v1/admin/ops/notification-deliveries` 返回 alert notification 投递证据，支持 `channel_id` 和 `status` 过滤；delivery 可水合 alert summary 和 channel name，但不得复制 alert details 或邮件正文。
- `GET /api/v1/admin/scheduler/decisions` 返回调度决策证据，支持 `request_id`、`account_id`、`provider_id`、`model`、`source_endpoint` 以及 `start`/`end` RFC3339 时间窗口过滤；`start` 为包含边界，`end` 为排除边界，用于从 alert event details 精确回放事故窗口。
- `GET /api/v1/admin/ops/realtime/slots` 返回 active realtime slot 摘要和聚合计数；Redis 可用时该视图覆盖同一 Redis 后端上的 API 节点，本地降级模式只覆盖当前节点内存。它不是持久 upstream session pool 查询，且不得返回原始 affinity key、API key、credential、prompt 或 provider-specific frame。

## 5. 统一响应格式

控制台和管理 API 使用统一响应格式。

成功响应：

```json
{
  "data": {},
  "request_id": "req_xxx"
}
```

列表响应：

```json
{
  "data": [],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100,
    "has_next": true
  },
  "request_id": "req_xxx"
}
```

错误响应：

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "invalid request",
    "details": {},
    "trace_id": "trace_xxx"
  },
  "request_id": "req_xxx"
}
```

## 6. Gateway 错误格式

OpenAI-compatible API 应尽量返回 OpenAI 风格错误：

```json
{
  "error": {
    "message": "The request is invalid.",
    "type": "invalid_request_error",
    "param": null,
    "code": "invalid_request"
  }
}
```

内部仍需要记录 SRapi 自己的错误分类：

```txt
provider_error_class
scheduler_reject_reason
billing_error_code
account_error_state
```

## 7. HTTP 状态码规范

```txt
200 OK                    成功
201 Created               创建成功
202 Accepted              异步任务已接受
204 No Content            删除成功且无返回
400 Bad Request           请求格式错误
401 Unauthorized          未认证
403 Forbidden             无权限或套餐限制
404 Not Found             资源不存在
409 Conflict              状态冲突或唯一约束冲突
422 Unprocessable Entity  业务校验失败
429 Too Many Requests     限流或额度超限
500 Internal Server Error 内部错误
502 Bad Gateway           上游错误
503 Service Unavailable   服务不可用或无可用账号
504 Gateway Timeout       上游超时
```

## 8. 错误码规范

错误码使用大写蛇形命名。

示例：

```txt
INVALID_REQUEST
UNAUTHORIZED
FORBIDDEN
RESOURCE_NOT_FOUND
RESOURCE_CONFLICT
VALIDATION_FAILED
RATE_LIMIT_EXCEEDED
USER_BALANCE_INSUFFICIENT
SUBSCRIPTION_EXPIRED
API_KEY_DISABLED
MODEL_NOT_ALLOWED
MODEL_NOT_FOUND
NO_AVAILABLE_ACCOUNT
SCHEDULER_REJECTED
PROVIDER_RATE_LIMITED
PROVIDER_AUTH_FAILED
PROVIDER_QUOTA_EXCEEDED
PAYMENT_ORDER_NOT_FOUND
PAYMENT_WEBHOOK_INVALID
INTERNAL_ERROR
```

## 9. 分页规范

请求参数：

```txt
page
page_size
sort
order
```

默认：

```txt
page = 1
page_size = 20
max_page_size = 100
```

响应：

```json
{
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100,
    "has_next": true
  }
}
```

## 10. 过滤和搜索规范

列表接口可使用：

```txt
q
status
provider
model
user_id
created_from
created_to
```

列表接口使用明确的 query 参数，不引入复杂过滤 DSL。

## 11. 幂等规范

以下接口必须支持幂等：

- 创建支付订单。
- 支付 Webhook。
- Gateway 非流式请求，可选。
- 账务调整。
- 订阅激活。

请求头：

```txt
Idempotency-Key: <key>
```

幂等记录建议保存：

```txt
key
method
path
request_hash
response_snapshot
status
expires_at
```

## 12. Trace 与 Request ID

所有响应都应包含：

```txt
X-Request-ID
```

如果启用 OpenTelemetry，还应关联 trace id。

前端错误展示和后台日志查询都应使用 request id。

## 13. 时间格式

所有时间使用 ISO 8601 UTC 字符串。

示例：

```txt
2026-05-21T00:00:00Z
```

## 14. 金额格式

API 响应中金额建议使用字符串或整数最小单位，避免 float 精度问题。

推荐：

```json
{
  "amount": "12.34",
  "currency": "USD"
}
```

数据库内部可使用 decimal 或 int64 minor units。

## 15. Token 用量格式

统一 usage：

```json
{
  "input_tokens": 1000,
  "output_tokens": 500,
  "cached_tokens": 800,
  "total_tokens": 1500
}
```

Provider 特有字段放入：

```json
{
  "provider_usage": {}
}
```

## 16. 主要接口一览

以下是各分区的代表性接口；权威的完整接口清单（约 361 个 operationId）以 `packages/openapi/openapi.yaml` 为准。

### 16.1 Auth

```txt
POST /api/v1/auth/login
POST /api/v1/auth/register
POST /api/v1/auth/logout
POST /api/v1/auth/refresh
GET  /api/v1/auth/session
```

### 16.2 Current User

```txt
GET /api/v1/me
GET /api/v1/me/usage
GET /api/v1/me/billing
GET /api/v1/me/subscriptions
```

### 16.3 API Keys

```txt
GET    /api/v1/api-keys
POST   /api/v1/api-keys
GET    /api/v1/api-keys/{id}
PATCH  /api/v1/api-keys/{id}
DELETE /api/v1/api-keys/{id}
```

API Key 创建和更新必须支持 `group_ids`，用于多组绑定；运行时解析规则以 `GATEWAY_ROUTE_MATRIX.md` 为准。

### 16.4 Admin Providers

```txt
GET   /api/v1/admin/providers
POST  /api/v1/admin/providers
GET   /api/v1/admin/providers/{id}
PATCH /api/v1/admin/providers/{id}
POST  /api/v1/admin/providers/{id}/test
```

### 16.5 Admin Models

```txt
GET   /api/v1/admin/models
POST  /api/v1/admin/models
GET   /api/v1/admin/models/{id}
PATCH /api/v1/admin/models/{id}
POST  /api/v1/admin/models/{id}/aliases
POST  /api/v1/admin/models/{id}/mappings
GET   /api/v1/admin/capabilities
GET   /api/v1/admin/capabilities/{key}
```

Capability descriptor、版本、状态和降级规则以 `CAPABILITY_TAXONOMY_SPEC.md` 为准。

### 16.6 Admin Accounts

```txt
GET   /api/v1/admin/accounts
POST  /api/v1/admin/accounts
GET   /api/v1/admin/accounts/{id}
PATCH /api/v1/admin/accounts/{id}
POST  /api/v1/admin/accounts/{id}/test
POST  /api/v1/admin/accounts/{id}/discover-models
POST  /api/v1/admin/accounts/{id}/disable
POST  /api/v1/admin/accounts/{id}/enable
GET   /api/v1/admin/accounts/{id}/health
GET   /api/v1/admin/accounts/{id}/quota
```

`GET /api/v1/admin/accounts/{id}/health` 必须返回运维排障所需的低基数字段：

- 账号和 Provider 标识：`account_id`、`provider_id`、`runtime_class`、`status`。
- 最近错误与健康：`error_class`、`success_rate`、`error_rate`、`latency_p50_ms`、`latency_p95_ms`。
- 额度与限流：`quota_remaining_ratio`、`quota_exhausted`、`rate_limit_count`、`timeout_count`。
- 保护状态：`cooldown_until`、`cooldown_reason`、`circuit_state`、`snapshot_at`。

该响应不得包含账号名称、上游凭证、Cookie、OAuth token、API Key 或 prompt 内容。

### 16.7 Admin Scheduler

```txt
GET  /api/v1/admin/scheduler/overview
GET  /api/v1/admin/scheduler/decisions
GET  /api/v1/admin/scheduler/decisions/{id}
POST /api/v1/admin/scheduler/simulate
GET  /api/v1/admin/scheduler/strategies
POST /api/v1/admin/scheduler/strategies
GET  /api/v1/admin/scheduler/strategies/{id}
PATCH /api/v1/admin/scheduler/strategies/{id}
POST /api/v1/admin/scheduler/strategies/{id}/activate
GET  /api/v1/admin/scheduler/strategies/{id}/versions
```

当前 dry-run / shadow comparison 使用 `POST /api/v1/admin/scheduler/simulate`，以同一请求 profile 显式传入 current 与 shadow strategy；可选 `shadow_rollout_percent` + `rollout_key` 返回稳定灰度 bucket 与 shadow 命中预览。K1.6.4 起 `AdminSettingsGateway` 还包含真实 Gateway 流量的 Scheduler shadow strategy rollout 控制字段：`scheduler_strategy_rollout_enabled`、`scheduler_strategy_shadow_strategy`、`scheduler_strategy_rollout_percent`、`scheduler_strategy_rollout_models`、`scheduler_strategy_rollout_api_key_hashes`。这些字段是可选字段；旧 settings payload 不带这些字段时默认不启用真实灰度。策略 descriptor、配置 schema、版本、批量历史回放和回滚规则以 `SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 为准。

### 16.8 Gateway

```txt
GET  /v1/models
GET  /v1beta/models
POST /v1/chat/completions
POST /v1/responses
GET  /v1/responses/{response_id}/input_items
POST /v1/responses/compact
POST /v1/messages
POST /api/provider/openai-compatible/v1/chat/completions
POST /api/provider/openai-compatible/v1/responses
GET  /api/provider/openai-compatible/v1/responses/{response_id}/input_items
POST /api/provider/openai-compatible/v1/responses/compact
POST /api/provider/openai-compatible/v1/messages
POST /api/provider/openai-compatible/v1/embeddings
POST /api/provider/openai-compatible/v1/images/generations
POST /api/provider/openai-compatible/v1/images/edits
POST /api/provider/openai-compatible/v1/images/variations
POST /api/provider/openai-compatible/v1/moderations
POST /api/provider/rerank-compatible/v1/rerank
POST /api/provider/anthropic-compatible/v1/messages
POST /api/provider/antigravity/v1/chat/completions
POST /api/provider/antigravity/v1/messages
POST /api/provider/antigravity/v1beta/models/{model}:generateContent
POST /api/provider/antigravity/v1beta/models/{model}:streamGenerateContent
```

标准 Gateway 入口（chat completions、responses、messages、models）与所有已暴露的 Provider alias 复用同一 Gateway runtime，只改变 provider context。Provider alias 仅通过 `/api/provider/{provider_key}` 路由族暴露，并保留原始 alias path 作为 usage / scheduler evidence。

embeddings、images、audio、moderations、rerank、countTokens、Gemini native (`/v1beta/...`)、Responses WebSocket 和 Realtime WebSocket 路由均已上线。完整的 Provider alias、passthrough 与路由族边界以 `GATEWAY_ROUTE_MATRIX.md` 为准。

### 16.9 Admin Subscriptions

```txt
GET  /api/v1/admin/subscription-plans
POST /api/v1/admin/subscription-plans
GET  /api/v1/admin/user-subscriptions
POST /api/v1/admin/user-subscriptions
GET  /api/v1/admin/pricing-rules
POST /api/v1/admin/pricing-rules
POST /api/v1/admin/pricing-rules:bulk
```

订阅与定价控制面必须满足：

- 金额和每百万 tokens 单价使用 decimal string，不使用 float 表示真实账务金额。
- `GET /api/v1/me/subscriptions` 只能返回当前用户订阅。
- 管理员创建用户订阅时必须复制套餐权益快照，后续套餐变更不得回写既有订阅权益。
- Pricing Rule 的 `provider_id=0` 表示模型级通用价格，具体 Provider 规则优先。
- Gateway admission 必须在 Scheduler 获取账号 lease 前执行用户/模型 entitlement 检查。
- 创建 active user subscription 时必须 materialize `entitlements` 查询缓存；`entitlements_snapshot` 保留为审计/防漂移证据。

### 16.10 Admin Roles

```txt
GET  /api/v1/admin/roles
POST /api/v1/admin/roles
```

Role 控制面必须满足：

- `name` 使用可扩展 role string，内置值为 `owner`、`admin`、`operator`、`user`。
- `permissions` 是去重后的 `resource:action` 字符串数组，例如 `payment_order:read`。
- 创建 role 必须要求控制台登录态和 CSRF。
- User create/update/batch update 可分配已存在的自定义 role。
- `GET /api/v1/admin/payments/orders` 接受 `owner/admin` 或 `payment_order:read` permission。

### 16.11 Admin Ops

```txt
GET  /api/v1/admin/ops/overview
GET  /api/v1/admin/ops/traffic
GET  /api/v1/admin/ops/errors
GET  /api/v1/admin/ops/providers
GET  /api/v1/admin/scheduler/decisions
GET  /api/v1/admin/ops/realtime/slots
GET  /api/v1/admin/ops/alerts
POST /api/v1/admin/ops/alerts/{id}/ack
GET  /api/v1/admin/ops/slo
POST /api/v1/admin/ops/slo
PATCH /api/v1/admin/ops/slo/{id}
GET  /api/v1/admin/ops/notification-channels
POST /api/v1/admin/ops/notification-channels
PATCH /api/v1/admin/ops/notification-channels/{id}
DELETE /api/v1/admin/ops/notification-channels/{id}
GET  /api/v1/admin/ops/notification-deliveries
GET  /api/v1/admin/ops/events/outbox
GET  /api/v1/admin/ops/events/dead-letter
POST /api/v1/admin/ops/events/{event_id}/replay
```

领域事件 Outbox、Inbox、重试、死信和补偿以 `DOMAIN_EVENTS_SPEC.md` 为准。

### 16.11 Payments

```txt
GET  /api/v1/payment/methods
POST /api/v1/payment/orders
GET  /api/v1/payment/orders
GET  /api/v1/payment/orders/{id}
POST /api/v1/payment/orders/{id}/cancel
GET  /api/v1/admin/payments/providers
POST /api/v1/admin/payments/providers
PATCH /api/v1/admin/payments/providers/{id}
POST /api/v1/admin/payments/providers/{id}/test
GET  /api/v1/admin/payments/orders
POST /api/v1/admin/payments/orders/{id}/refund
```

### 16.12 Affiliate

```txt
GET  /api/v1/me/affiliate
GET  /api/v1/me/affiliate/ledger
POST /api/v1/me/affiliate/transfer-to-balance
GET  /api/v1/admin/affiliates/invites
GET  /api/v1/admin/affiliates/rebates
GET  /api/v1/admin/affiliates/transfers
```

用户侧 affiliate 转余额接口是账务写接口，必须同时使用 `cookieAuth`、`csrfHeader` 和 `Idempotency-Key` header。响应返回本次 affiliate ledger、billing ledger id、余额变更前后值和是否实际应用，重复 idempotency key 必须保持无副作用。

## 17. AI 端点兼容边界

当前契约兼容的核心端点与字段：

- `/v1/models`、`/v1beta/models`
- `/v1/chat/completions`
- `/v1/responses`、`/v1/responses/compact`、`/v1/responses/ws`、`GET /v1/responses/{response_id}/input_items`
- `/v1/messages`、`/v1/messages/count_tokens`
- `/v1/embeddings`、`/v1/images/*`、`/v1/audio/*`、`/v1/moderations`、`/v1/rerank`
- `/v1/realtime`（OpenAI-compatible Realtime WebSocket，见 §3.3）
- `stream: true`
- `messages`、`input`、`model`、`temperature`、`top_p`、`max_tokens`、`max_output_tokens`、`instructions`
- tool calls、`tools`、`tool_choice`
- JSON mode / structured output 字段

端点转换规则以 `AI_ENDPOINT_COMPATIBILITY.md` 为准；路由族与协议边界以 `GATEWAY_ROUTE_MATRIX.md` 为准。

明确不在当前契约范围内（Roadmap / 尚未实现）：

- Assistants API。
- Responses API 服务端 stateful store（跨请求会话持久化）和全部内置工具的完整兼容；当前 Responses 支持非持久调用、`compact`、`input_items` 读取，并对同协议 Codex/OpenAI compact 响应原样回放，不在跨协议路径伪造压缩语义。
- Batch API。
- Fine-tuning API。

WebRTC 形态的 Realtime 也不在当前范围内（已上线的 `/v1/realtime` 是 WebSocket upgrade，不是 WebRTC）。

## 18. 版本策略

管理 API 版本：

```txt
/api/v1
```

Gateway 兼容 API 保持行业路径：

```txt
/v1
```

破坏性变更必须进入新版本。

## 19. 生成工具

后端：

```txt
oapi-codegen
```

前端：

```txt
@hey-api/openapi-ts
orval
```

文档：

```txt
Scalar
Swagger UI
```

## 20. OperationId 规范

所有 operationId 必须稳定、可读、可用于生成前后端代码。

命名规则：

```txt
listApiKeys
createApiKey
getApiKey
updateApiKey
deleteApiKey
listAdminProviders
createAdminProvider
testAdminProvider
listAdminAccounts
testAdminAccount
simulateSchedulerStrategy
listSchedulerDecisions
createChatCompletion
listModels
```

禁止：

- 使用自动生成的 `getApiV1AdminProviders` 作为最终 operationId。
- 同名 operationId。
- 在不改 endpoint 的情况下随意重命名 operationId。

## 21. Schema 复用规范

OpenAPI 必须复用以下公共 schema：

```txt
RequestId
ErrorResponse
Pagination
Money
TokenUsage
AuditActor
ProviderErrorClass
SchedulerRejectReason
```

错误响应必须同时满足：

- 控制台和管理 API 使用 SRapi 标准错误结构。
- Gateway `/v1/*` 对客户端返回 OpenAI-compatible 错误结构。
- 内部日志保留 SRapi 错误码、provider_error_class、scheduler_reject_reason。

## 22. CI 校验

CI 至少包含：

- OpenAPI lint。
- OpenAPI bundle。
- 生成代码是否最新。
- Breaking change 检查。
- 后端编译。
- 前端 typecheck。
