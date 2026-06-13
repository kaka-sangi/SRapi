# Batch 14 End-User Login And Platform Settings Parity

This batch removes visible login/settings placeholders by either wiring a real
consumer or explicitly downranking the feature.

## End-User Login Matrix

| Capability | Status | Consumer / decision |
| --- | --- | --- |
| Email + password login | Connected | Existing `/api/v1/auth/login`. |
| Email passwordless login/register | Connected | `/api/v1/auth/passwordless/request` creates or finds the user, sends an encrypted outbox token, and `/api/v1/auth/passwordless/login` consumes it into a session. |
| Email verification | Connected | Existing `/api/v1/auth/email-verification/*`, reused token storage for passwordless. |
| OIDC/GitHub/Google/LinuxDo OAuth | Connected | `/api/v1/auth/oauth/{provider}/callback` fast-paths bound identities by provider subject hash before falling back to pending bind/create flows. |
| WeChat login | Downranked | Removed from OAuth provider whitelist and hidden from the login button source. No visible button remains until a dedicated WeChat protocol handler is implemented. |
| DingTalk login | Downranked | Removed from OAuth provider whitelist and hidden from the login button source. No visible button remains until a dedicated DingTalk protocol handler is implemented. |
| WeChat Pay JSAPI openid OAuth | Not applicable | Deliberately not implemented because WeChat login is downranked in this batch; it should be unlocked together with a real WeChat auth handler. |

## Feature Flag Consumption

| Setting | Status | Consumer |
| --- | --- | --- |
| `features.payments_enabled` | Connected | `handleListPaymentMethods` and `handleCreatePaymentOrder` reject when disabled. |
| `payment.subscription_plans_enabled` | Connected | User subscription endpoint and admin subscription plan/user-subscription mutations reject when disabled. |
| `features.invitation_rebate_enabled` | Connected | Outbox `PaymentOrderPaid` handler skips affiliate `AccrueRebate` when disabled. |
| `features.channel_monitoring_enabled` | Connected | Channel-monitor worker receives an `Enabled(ctx)` gate backed by admin settings. |
| `features.enabled_channels` | Connected | Gateway scheduler candidates are filtered by provider protocol before quality scoring and scheduling. Empty list means unrestricted. |
| `users.default_group` | Connected | API key creation uses the named account group as default group scope when the caller omits group IDs. The project does not yet have a separate user-group table; this is intentionally applied to the gateway group scope that exists today. |
| `users.user_self_delete_enabled` | Connected | `DELETE /api/v1/me` rejects when disabled, deletes the current user and clears the session when enabled. |
| `backup.enabled` | Connected | Backup worker no-ops when false. |
| `backup.retention_days` | Connected | Backup worker removes old `backups/srapi-*.dump` and checksum files past retention. |
| `backup.last_backup_at` | Connected | Backup worker writes the timestamp after successful `pg_dump`. |

## Platform Self-Service

| Capability | Status | Consumer |
| --- | --- | --- |
| Public site config | Connected | `GET /api/v1/site-config` exposes site name, logo URL, version label, custom menus, user agreement, and privacy policy. Register page consumes the brand and agreement/privacy links. |
| User attributes | Connected | `GET/PUT /api/v1/me/attributes` lets users read/write enabled attributes. Registration and self-service updates enforce enabled required definitions. |
| Console write idempotency | Connected | `withConsoleIdempotency` wraps payment order creation, API key creation, and redeem-code redemption using the existing idempotency store. |
| Captcha admin runtime config | Connected | `GET/PUT /api/v1/admin/settings/captcha` stores admin-managed provider/site key/secret config; public captcha config and auth verification read admin settings when managed. |
| Audit log retention | Connected | Retention worker already includes `AuditLogsDays` and operations cleanup deletes audit logs by cutoff. |

## SecuritySecret Decision

SRapi should not introduce a separate `security_secret` table in this batch.
The existing encrypted settings and domain-specific encrypted ciphertext fields
already cover the active secret classes:

- Settings secrets: copilot keys, web-search keys, captcha secret now use
  AES-GCM ciphertext under the server master key.
- Provider/payment/account secrets: account credentials, proxy URLs, payment
  provider configs, TOTP secrets, notification contact secrets, and auth email
  tokens already have domain-specific encrypted storage and audit redaction.

Adding a generic vault table now would duplicate ownership, make rotation
semantics less clear, and create a new cross-module dependency before there are
multiple real call sites requiring shared secret lifecycle operations. The
boring structure is to keep secrets next to the domain that owns their
validation and rotation, and add a dedicated vault only when at least two
domains need shared CRUD/rotation/history semantics.
