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
- v1 favors clear live/read-model aggregation over premature rollup tables. High
  volume rollups and external log backends are Phase 2 work.

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
GET /api/v1/admin/ops/alert-events
PUT /api/v1/admin/ops/settings
```

Ops v1 is read-model based:

- Request, token, latency, and error evidence comes from `usage_logs`.
- Realtime concurrency comes from the realtime slot module.
- Scheduler and account evidence comes from existing scheduler/accounts
  contracts where available.
- Alert events reuse the existing operations alert control plane.
- System logs are a structured event list. v1 may expose an empty list until a
  durable system-log sink lands; it must not read local stdout/stderr files.

The API must preserve low-cardinality labels and must not return raw API keys,
session affinity keys, credentials, prompts, cookies, or provider-native frames.

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

v1 supports console management only. User-facing announcement delivery and read
receipts are separate product work.

### 3.4 Redeem Codes

Routes:

```txt
GET  /api/v1/admin/redeem-codes
POST /api/v1/admin/redeem-codes
POST /api/v1/admin/redeem-codes/batch-generate
POST /api/v1/admin/redeem-codes/batch-disable
GET  /api/v1/admin/redeem-codes/stats
```

Redeem code fields:

- `id`
- `code`
- `type`: `balance`, `subscription`, `custom`
- `amount`
- `currency`
- `status`: `active`, `disabled`, `redeemed`, `expired`
- `max_redemptions`
- `redeemed_count`
- `expires_at`
- `metadata`
- timestamps

v1 is an administrator management surface. Applying a redeem code to user
balance or subscription state must be introduced as a separate user-facing,
idempotent flow.

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
- `discount_amount`
- `currency`
- `status`: `active`, `disabled`, `expired`
- `max_uses`
- `used_count`
- `expires_at`
- `metadata`
- timestamps

Percent discounts are decimal strings and are interpreted as ratios in the
range `0` to `1`. Amount discounts require `currency`.

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
must use encrypted storage before production use; v1 responses expose only
configured flags for secrets.

PUT is a partial update. Omitted tabs retain previous values. Each write creates
an audit record containing the changed tab names and redacted before/after
snapshots.

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

New v1 data can initially use the `settings` table for low-frequency
administrator-managed collections:

- `admin_control.announcements`
- `admin_control.redeem_codes`
- `admin_control.promo_codes`
- `admin_control.risk_config`
- `admin_control.ops_settings`
- `admin_control.system_settings`
- `admin_control.risk_logs`
- `admin_control.system_logs`

This keeps the first implementation reviewable. When user-facing redemption,
promo-code application, announcement read receipts, or high-volume risk/system
logs are added, promote those collections to first-class Ent schemas with
migrations.

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
- OpenAPI/SDK parity.
- Focused backend tests.

Phase 2 includes:

- Rollup tables for dashboard and ops trends.
- Durable high-volume system logs and risk event tables.
- User-facing announcement delivery and read receipts.
- User-facing redeem and promo application flows.
- Backup restore workflow with async job state and re-authentication.
- External notification channels and alert-routing rules.
