# SRapi Admin Control Plane v1

## 1. Purpose

Admin Control Plane v1 is SRapi's first complete management backend for the
console. It does not copy the reference implementation shape from sub2api.
The reference project is used only to identify operator workflows and missing
surfaces. SRapi keeps its own modular-monolith boundaries, OpenAPI-first
contract, decimal-safe money model, and audit requirements.

The control plane must answer three operator questions:

- What is happening now across users, keys, accounts, models, requests, tokens,
  cost, and latency?
- Which control-plane resources can an administrator manage without touching
  the gateway data path?
- Which settings and risk policies are active, who changed them, and what did
  they affect?

## 2. Design Principles

- OpenAPI is the HTTP source of truth. Every route is defined in
  `packages/openapi/openapi.yaml` before Go handlers or SDK types change.
- HTTP handlers call module services/contracts only. They must not query Ent or
  Redis directly.
- Console write operations use `cookieAuth` plus `csrfHeader` and write safe
  audit log records.
- Money, balance, discounts, costs, and code values are decimal strings with a
  currency. Float is not allowed for financial state.
- Secrets are write-only or redacted. Responses expose `configured: true/false`
  rather than secret values.
- Ops monitoring follows SRE golden signals: latency, traffic, errors, and
  saturation. SRapi adds AI-native signals such as tokens, TTFT-ready fields,
  account health, scheduler decisions, and reverse-proxy risk classes.
- The control plane favors clear live/read-model aggregation over premature
  rollup tables. High-volume rollup tables and external log backends remain on
  the roadmap (see §7) and are not yet implemented.

## 3. Route Families

### 3.1 Dashboard Snapshot

```txt
GET /api/v1/admin/dashboard/snapshot
```

The snapshot is a single console bootstrap payload for overview cards and
charts. It is generated from existing module contracts:

- users
- api_keys
- accounts
- usage
- scheduler
- operations

Required response sections:

- `generated_at`
- `window`
- `inventory`
- `traffic`
- `users`
- `tokens`
- `costs`
- `performance`
- `health`
- `model_distribution`
- `token_trend`
- `user_usage_trend`

Cost fields:

- `actual_cost`
- `account_cost`
- `standard_cost`

In v1, if only one cost source exists, all three fields may use the same
decimal total and set `cost_basis` to `usage_log_cost`. Future pricing and
account-cost modules can split these fields without changing the response
shape.

### 3.2 Ops Monitoring

Routes:

```txt
GET /api/v1/admin/ops/overview
GET /api/v1/admin/ops/throughput-trend
GET /api/v1/admin/ops/error-trend
GET /api/v1/admin/ops/error-distribution
GET /api/v1/admin/ops/latency-histogram
GET /api/v1/admin/ops/concurrency
GET /api/v1/admin/ops/system-logs
POST /api/v1/admin/ops/system-logs/cleanup
GET /api/v1/admin/ops/alert-events
PUT /api/v1/admin/ops/settings
```

Ops v1 is read-model based:

- Request, token, latency, and error evidence comes from `usage_logs`.
- Realtime concurrency comes from the realtime slot module.
- Scheduler and account evidence comes from existing scheduler/accounts
  contracts where available.
- Alert events reuse the existing operations alert control plane.
- System logs are structured, sanitized events stored in `ops_system_logs`.
  Listing supports bounded filters by level, source, text query, and time
  range. Cleanup is a CSRF-protected admin write with dry-run support,
  `max_delete` caps, and safe audit summaries that do not copy raw query text.
  The endpoint must not read local stdout/stderr files.

The API must preserve low-cardinality labels and must not return raw API keys,
session affinity keys, credentials, prompts, cookies, or provider-native frames.

These routes are the console ops dashboard surface and match
`packages/openapi/openapi.yaml` (`/api/v1/admin/ops/*`). They are complementary
to the SLO / scheduler-decision / realtime-slot ops routes documented in
`OBSERVABILITY_SPEC.md` §11 (`overview`, `slo`, `scheduler/decisions`,
`realtime/slots`, `alerts/{id}/ack`); see OpenAPI for the authoritative,
combined ops route list.

### 3.3 Announcements

Routes:

```txt
GET    /api/v1/admin/announcements
POST   /api/v1/admin/announcements
PUT    /api/v1/admin/announcements/{id}
DELETE /api/v1/admin/announcements/{id}
```

Announcement fields:

- `id`
- `title`
- `content`
- `status`: `draft`, `active`, `archived`
- `severity`: `info`, `success`, `warning`, `critical`
- `audience`: `all`, `admins`, `operators`, `users`
- `starts_at`
- `ends_at`
- `created_at`
- `updated_at`

Current-user announcement delivery and read receipts are exposed through
`GET /api/v1/me/announcements` and
`POST /api/v1/me/announcements/{id}/read`; read receipts are stored in
`user_announcement_reads`.

### 3.4 Redeem Codes

Routes:

```txt
GET  /api/v1/admin/redeem-codes
POST /api/v1/admin/redeem-codes
POST /api/v1/admin/redeem-codes/batch-generate
POST /api/v1/admin/redeem-codes/batch-disable
GET  /api/v1/admin/redeem-codes/stats
POST /api/v1/me/redeem-codes/redeem
```

Redeem code fields:

- `id`
- `code`
- `type`: `balance`, `subscription`
- `value`: balance amount for balance codes, or subscription plan id for
  subscription codes
- `currency`
- `status`: `active`, `disabled`, `redeemed`, `expired`
- `max_redemptions`
- `redeemed_count`
- `expires_at`
- timestamps

Admin routes remain the management surface for issuing and disabling codes.
Current-user redemption is a separate CSRF-protected transactional flow:
`POST /api/v1/me/redeem-codes/redeem` accepts a code, normalizes it
case-insensitively, and either credits the user's balance or creates a
subscription from the referenced plan. Each `(user_id, redeem_code_id)` can be
redeemed once; repeated submissions by the same user return the original
redemption result without issuing a second ledger entry or subscription.
Successful redemption increments `redeemed_count`; codes move to `redeemed`
after `max_redemptions` is reached, and expired or disabled codes return a
conflict.

### 3.5 Promo Codes

Routes:

```txt
GET    /api/v1/admin/promo-codes
POST   /api/v1/admin/promo-codes
PUT    /api/v1/admin/promo-codes/{id}
DELETE /api/v1/admin/promo-codes/{id}
```

Promo code fields:

- `id`
- `code`
- `discount_type`: `amount`, `percent`
- `discount_value`
- `currency`
- `status`: `active`, `disabled`, `expired`
- `max_uses`
- `used_count`
- `starts_at`
- `expires_at`
- `metadata`
- timestamps

Percent discounts are decimal strings and are interpreted as ratios in the
range greater than `0` and less than or equal to `1`. Amount discounts require
`currency`.

User-facing application happens through `POST /api/v1/payment/orders` with an
optional `promo_code`. The server validates active status, `starts_at`,
`expires_at`, max uses, currency, and deterministic amount/percent discount
math before checkout creation. Persistent stores write the discounted
`payment_orders` row, one `user_promo_code_applications` receipt, and the
promo-code `used_count` increment in one transaction. Receipts store only the
normalized code digest and promo-code id, not the promo code plaintext.

### 3.6 Risk Control

Routes:

```txt
GET /api/v1/admin/risk-control/config
PUT /api/v1/admin/risk-control/config
GET /api/v1/admin/risk-control/status
GET /api/v1/admin/risk-control/logs
```

Risk control v1 is policy and evidence oriented:

- `mode`: `monitor` or `enforce`
- reverse-proxy risk class actions
- suspicious user/API-key/account thresholds
- mixed account group policy
- denylist and allowlist summaries

Risk logs must be sanitized event records. They may be derived from account
metadata and usage error classes in v1. Provider credentials, prompts, API keys,
cookies, and raw upstream payloads are forbidden.

### 3.7 System Settings

Routes:

```txt
GET /api/v1/admin/settings
PUT /api/v1/admin/settings
```

The response is a typed settings snapshot with these tabs:

- `general`
- `agreement`
- `features`
- `security`
- `users`
- `gateway`
- `payment`
- `email`
- `backup`

The settings service stores typed JSON under stable keys. Secret-bearing fields
are encrypted at rest with the AES key derived from `Security.MasterKey` (see
`internal/platform/crypto`) and follow a write-only / `configured` pattern:
responses expose only configured flags for secrets, never the secret values. For
example, the copilot dedicated API key is persisted as encrypted ciphertext that
never crosses the API boundary, and the runtime SMTP password comes only from
`EMAIL_SMTP_PASSWORD`.

Email settings expose non-secret delivery metadata for the console UI:
`smtp_configured`, `smtp_host`, `smtp_port`, `smtp_username`,
`smtp_password_configured`, `smtp_from`, `smtp_from_name`, `smtp_use_tls`,
`public_base_url`, and template overrides. In the current transactional-email
foundation, the runtime SMTP password comes only from `EMAIL_SMTP_PASSWORD`;
`PUT /api/v1/admin/settings` does not accept or persist `smtp_password`, and
responses derive `smtp_password_configured` from runtime config rather than
stored settings JSON.

Notification unsubscribe preferences may use settings-backed state while the
feature remains low-volume and event-scoped. Preference keys store event names
and email hashes only; they must not store plaintext recipient emails or SMTP
secrets. If notification preferences become user-facing high-volume state, move
them to a dedicated Ent schema and migration before adding bulk listing or
per-user management APIs.

Notification email template management is exposed separately from the full
settings snapshot:

```txt
GET /api/v1/admin/notifications/email-templates
GET /api/v1/admin/notifications/email-templates/{event}
PUT /api/v1/admin/notifications/email-templates/{event}
POST /api/v1/admin/notifications/email-templates/{event}/restore
POST /api/v1/admin/notifications/email-template-preview
```

Template overrides are stored in `email.templates` using `<event>.subject` and
`<event>.html` keys. The notifications module owns the event catalog, built-in
defaults, placeholder allowlists, and rendering rules. Admin writes require
CSRF and create audit records scoped to the edited template. Preview renders
without saving state, escapes variables, and blanks unsafe URL placeholders.

PUT currently accepts the full typed settings snapshot defined by
`AdminSettings`. Each write creates an audit record containing redacted
before/after snapshots.

Gateway settings include the K1.6.4 Scheduler real-traffic rollout controls:

- `scheduler_strategy_rollout_enabled`
- `scheduler_strategy_shadow_strategy`
- `scheduler_strategy_rollout_percent`
- `scheduler_strategy_rollout_models`
- `scheduler_strategy_rollout_api_key_hashes`

The model and API key prefix hash arrays are optional scopes. Empty arrays mean
all models or all API keys. Runtime evidence must store only SHA-256 rollout
key hashes, not raw API key prefixes, API keys, prompts, cookies, or provider
credentials.

## 4. Persistence Strategy

Admin Control Plane v1 uses two persistence categories.

Existing durable tables:

- `users`
- `api_keys`
- `provider_accounts`
- `usage_logs`
- `scheduler_decisions`
- `audit_logs`
- `settings`
- `obs_slo_definitions`
- `obs_alert_events`
- `user_announcement_reads`
- `user_promo_code_applications`

New v1 data can initially use the `settings` table for low-frequency
administrator-managed collections:

- `admin_control.announcements`
- `admin_control.redeem_codes`
- `admin_control.promo_codes`
- `admin_control.risk_config`
- `admin_control.ops_settings`
- `admin_control.system_settings`
- `admin_control.risk_logs`

This keeps the first implementation reviewable. When high-volume risk events
are added, promote those collections to first-class Ent schemas with migrations.

User-facing announcement read receipts are now first-class persistence in
`user_announcement_reads`. Admin-authored announcement content remains a
low-frequency typed settings collection under `admin_control.announcements`;
per-user read state is separate and unique on `(user_id, announcement_id)`.
Current-user promo-code application receipts are first-class persistence in
`user_promo_code_applications`. Admin-authored promo-code definitions remain a
low-frequency typed settings collection under `admin_control.promo_codes`.

## 5. Security And Audit

Every write route must:

- require admin session
- validate CSRF token
- validate request body against typed OpenAPI schemas
- call a module service
- record audit logs with safe before/after snapshots

Audit records must not include:

- API key plaintext or hash
- OAuth tokens
- cookies
- provider credentials
- SMTP password
- admin API key material
- raw prompts or upstream payloads

## 6. Quality Gates

Required for implementation work:

```txt
make openapi-codegen
make openapi-ts-codegen
cd apps/api && go test ./...
make architecture-check
make code-quality-check
make secret-scan
make check
```

If Ent schema changes are introduced:

```txt
make ent-generate
make migration-check
```

## 7. Phase Boundaries

Admin Control Plane v1 includes:

- Dashboard snapshot.
- Ops read-model endpoints.
- Settings-backed announcements, redeem codes, promo codes, risk-control config,
  ops settings, system settings, and sanitized event lists.
- Current-user redeem-code redemption backed by first-class redemption receipts,
  user balance/subscription state, and billing ledger evidence.
- Current-user promo-code application for payment orders backed by first-class
  order receipts and atomic `used_count` updates.
- OpenAPI/SDK parity.
- Focused backend tests.

Since v1, several items originally listed as future work have shipped and are
no longer roadmap items:

- Admin notification email-template management (catalog, per-event subject/HTML
  overrides, restore-to-default, and preview) — see §3.7.
- Operational PostgreSQL backup/restore via the `make backup-postgres` and
  `make restore-postgres` targets (see `../../docs/requirements/OPERATIONS.md`).

Roadmap / not yet implemented:

- Rollup tables for dashboard and ops trends.
- Risk event tables and higher-level security/risk analytics.
- External system-log export, retention policy automation, and archived-log
  retrieval.
- An in-console backup/restore workflow with async job state and
  re-authentication (operational backup/restore today is the make-target CLI
  flow above).
- External notification channels (e.g. Slack/webhook fan-out) and alert-routing
  rules beyond the shipped email notifications.
