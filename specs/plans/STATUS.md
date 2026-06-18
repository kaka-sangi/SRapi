# SRapi Goal Status

> Internal development progress ledger (one entry per completed work package). This tracks
> engineering execution, not the product. For the product overview see the root
> [`README.md`](../../README.md); for the specs map see [`README.md`](../README.md).

## Current Snapshot

status_version: 2
updated_at: 2026-06-10

last_completed:

| WP | Summary |
| --- | --- |
| WP-000 | Codex execution specs added: final state, roadmap, work packages, gates. |
| WP-010 | Architecture baseline harness verified via `make architecture-check`. |
| WP-020 | OpenAPI lint, bundle, Go/TS codegen drift, SDK typecheck verified. |
| WP-030 | Ent schema, initial migration, data model, API-key persistence aligned. |
| WP-040 | Console session/CSRF, HMAC-only key storage, secret-free responses tested. |
| WP-050 | Gateway extraction owns Canonical AI normalization with golden tests. |
| WP-060 | Capability taxonomy aligned to canonical keys; feeds Scheduler hard filters. |
| WP-070 | OpenAI-compatible Adapter v1: streaming/non-streaming, usage, error classification. |
| WP-080 | Responses/Messages compatibility proven via golden tests and mock upstream. |
| WP-090 | Scheduler v1 hardening: scenario matrix, lease-state, Redis concurrency coverage. |
| WP-100 | Gateway evidence closure: usage, billing, audit, outbox, feedback tests. |
| WP-110 | Provider preset registry expansion; dynamic alias routes preserve policy. |
| WP-120 | Reverse Proxy Runtime foundation: header strip, cookie isolation, risk mapping. |
| WP-130 | OAuth refresh/token lifecycle: re-encryption, audit, ban/session signals. |
| WP-140 | CLI runtime classes; model alias and session affinity feed Scheduler. |
| WP-150 | Admin diagnostics expose Scheduler evidence and account health summaries. |
| WP-170 | Account operations parity: groups, inspect/export/import, proxy, recovery, SDK parity. |
| WP-180 | Subscription/pricing foundations: plans, decimal pricing, entitlement admission, SDK parity. |
| WP-190 | Payment order foundations: encrypted providers, signed webhooks, fulfillment, SDK parity. |
| WP-200 | Affiliate rebate phase 2: invites, rules, accrual, transfers, SDK parity. |
| WP-210 | Production ops: Prometheus metrics, secret rejection, retention, backup, smoke. |
| C2.1 | `/metrics` via client_golang scrape collector; latency/probe buckets from rollups. |
| WP-220 | Anthropic upstream adapter: Messages dispatch, SSE usage, error classification. |
| WP-230 | Gemini-native Gateway routes: GenerateContent/StreamGenerateContent, canonical conversion, evidence. |
| WP-240 | Gemini-compatible upstream adapter dispatches GenerateContent with usage and errors. |
| WP-250 | Provider account model discovery persists `supported_models`, feeds candidate filtering. |
| WP-260 | Ops SLO/alert control plane v1: definitions, burn-rate evidence, SDK parity. |
| WP-270 | Embeddings passthrough runtime v1: `/v1/embeddings`, alias routing, feedback evidence. |
| WP-280 | HTTP runtime partitioned into route-family files with size-cap harness. |
| WP-290 | Images generations runtime v1 with capability filtering and SDK parity. |
| WP-300 | Go code-quality harness: gofmt, vet, file/function-size thresholds enforced. |
| WP-310 | Moderations runtime v1: `/v1/moderations`, alias routing, feedback, SDK parity. |
| WP-320 | Rerank runtime v1: `/v1/rerank`, alias routing, feedback, SDK parity. |
| WP-330 | Audio transcriptions runtime v1: multipart upstream, capability filtering, SDK parity. |
| WP-340 | Audio speech runtime v1: binary rendering, capability filtering, SDK parity. |
| WP-350 | Antigravity reverse-proxy runtime identity added with protocol dispatch tests. |
| WP-360 | Antigravity text provider aliases seeded; route through standard pipeline. |
| WP-370 | Antigravity Gemini model-action aliases route via standard Gemini handler. |
| WP-380 | Responses WebSocket runtime: `/v1/responses/ws` streams events as frames. |
| WP-390 | Reverse Proxy Runtime gains `RelayWebSocket` primitive for upstream WSS relay. |
| WP-400 | Codex CLI 2api HTTP text path via Reverse Proxy Runtime. |
| WP-410 | Codex CLI 2api Responses WebSocket relay with Scheduler/usage evidence. |
| WP-420 | Claude Code CLI 2api Messages path with beta/session headers. |
| WP-430 | ChatGPT Web 2api Conversation path with browser/Sentinel headers. |
| WP-440 | ChatGPT Web 2api auto-fetches Sentinel requirements, PoW, challenge classification. |
| WP-450 | Antigravity 2api text path to Cloud Code generateContent envelope. |
| WP-460 | Realtime slot lifecycle v1: acquire/release slots, per-key limits, metrics. |
| WP-470 | OpenAI-compatible Realtime WebSocket relay v1 via Reverse Proxy Runtime. |
| WP-480 | Images edits runtime v1: multipart upstream, capability filtering, SDK parity. |
| WP-490 | Images variations runtime v1: multipart upstream, capability filtering, SDK parity. |
| WP-500 | Antigravity 2api model discovery persists `supported_models` credential-free. |
| WP-510 | Images edits JSON references accept data-URL/base64; reject remote URLs. |
| WP-520 | Images edits streaming returns SSE result chunk when `stream=true`. |
| WP-530 | Antigravity project bootstrap via loadCodeAssist/onboardUser before discovery. |
| WP-540 | Gemini native models list `GET /v1beta/models` with key visibility. |
| WP-550 | Gemini native countTokens `:countTokens`; schedules token-counting accounts only. |
| WP-560 | Anthropic Messages count_tokens with alias and Claude Code dispatch. |
| WP-570 | AdminOps realtime active slot API returns node summaries, no secrets. |
| WP-580 | SDK examples and 2api migration guide v1 plus `make examples-check`. |
| WP-590 | Distributed realtime slot store v1: in-memory and Redis implementations. |
| WP-600 | Codex refresh-token-only import and OAuth exchange/dispatch lifecycle. |
| WP-610 | Claude Code refresh-token-only import and OAuth exchange/dispatch lifecycle. |
| WP-620 | Antigravity refresh-token-only import and Google OAuth exchange lifecycle. |
| WP-630 | OpenAI-compatible API-key Realtime relay; non-api-key stays on proxy. |
| WP-700 | Admin Control Plane v1: dashboard, ops, settings, codes, risk APIs. |
| WP-710 | Incremental migration workflow: Atlas diff/hash targets, pairing checks. |
| WP-760 | AdminOps durable system logs owned by operations, with health evidence, filters, cleanup. |
| WP-770 | Console TOTP 2FA: encrypted secrets, HMAC recovery codes, challenges. |
| WP-780 | Current-user announcement inbox with per-user read receipts. |
| WP-790 | Current-user redeem-code redemption with idempotent per-user receipts. |
| WP-800 | Current-user promo-code application during order creation, atomic use-counts. |
| WP-810 | Public console registration v1 with allowlist and enumeration-safe errors. |
| WP-820 | Current-user password change revokes sessions, clears cookie, audit-safe. |
| WP-830 | Current-user profile update: allowlisted name only, mass-assignment rejected. |
| WP-840 | Public password reset v1: hashed tokens, single-use, session revocation. |
| WP-850 | Public email verification v1: hashed tokens, single-use, marks verified. |
| WP-860 | Notification email dispatch foundation: outbox events via configured SMTP. |
| WP-870 | Notification preferences and one-click unsubscribe with List-Unsubscribe headers. |
| WP-880 | Notification email template management: admin CRUD, preview, placeholder validation. |
| WP-890 | Current-user notification preferences; transactional auth events non-optional. |
| WP-900 | Low-balance notification trigger on threshold crossing, preference-aware email. |
| WP-910 | Subscription expiry reminders at 7/3/1 days, idempotent, preference-aware. |
| WP-930 | Verified notification contacts v1: request/verify/manage, encrypted outbox mail. |
| WP-940 | Current-user avatar storage v1: normalized PNG upload, controlled serving. |
| WP-950 | User auth identity directory v1: list/unbind external identities. |
| WP-960 | Pending OAuth session foundation: hash-only, single-use, local redirects. |
| WP-970 | OAuth/OIDC authorization start v1: state, PKCE, nonce, encrypted cookie. |
| WP-980 | OAuth/OIDC callback v1: code exchange, UserInfo, pending session cookie. |
| WP-990 | Pending OAuth decision preview v1: read-only safe summary, next_step. |
| WP-1000 | Pending OAuth current-user bind v1; rejects cross-user ownership transfer. |
| WP-1010 | Pending OAuth existing-account bind-login v1 with 2FA challenge support. |
| WP-1020 | Pending OAuth create-account v1 from verified email with rollback. |
| WP-1030 | Pending OAuth email-completion v1: verify missing provider email via outbox. |
| WP-1040 | Pending OAuth profile-adoption v1: opt-in display name, no avatar fetch. |
| WP-1050 | Upstream OAuth re-auth parking: permanent refresh failure parks `needs_reauth`. |
| WP-1060 | Outbound SSRF egress guard v1: post-DNS IP screening on dials. |
| WP-1070 | Gateway request idempotency v1: opt-in `Idempotency-Key` replays captured response. |
| WP-1080 | Gateway idempotency hardening: cleanup worker, messages/embeddings coverage. |
| WP-1090 | Health-probe desync jitter v1: bounded random delay before probes. |
| WP-1100 | API-key egress consistency v1: native traffic uses managed per-account egress. |
| WP-1310 | Frontend completeness/correctness pass: nav rebuild, P0 fixes, i18n, verification. |
| WP-1300 | OpenAPI formalization of 24 new admin endpoints across 9 surfaces (partial — spec only, handlers not migrated). |
| WP-1290 | OIDC id_token validation v1: go-oidc RS256/JWKS, iss/aud/exp/nonce checks. |
| WP-1280 | ID-referencing config import v1: natural-key remap for rate limits. |
| WP-1270 | Confidential-client console OAuth v1: env client_secret in token exchange. |
| WP-1260 | Per-model/per-group TPM v1 completes rate/capacity limiter matrix. |
| WP-1250 | Config snapshot import v1: upsert natural-keyed sections, dry-run. |
| WP-1240 | Config snapshot export v1: versioned portable operator-config JSON document. |
| WP-1230 | Scheduled connectivity test runner v1: opt-in billable generative probe. |
| WP-1220 | Per-model max concurrency v1 via threaded modelID dispatch lease. |
| WP-1210 | Per-account-group max concurrency v1 via bundled rollback leases. |
| WP-1200 | Per-account-group RPM capacity v1 enforced after account selection. |
| WP-1190 | Per-model RPM rate limit v1 enforced at gateway admission. |
| WP-1180 | Subscription-allowance-first billing with pay-as-you-go cost-based overage. |
| WP-1170 | Scheduled account quota refresh worker v1, gated and bounded. |
| WP-1110 | User custom attributes (EAV) v1: typed definitions and per-user values. |
| WP-1120 | Error-passthrough DB rules v1: priority expose/mask gateway resolver. |
| WP-1130 | TLS fingerprint DB profiles v1: named egress profiles, account-wins expander. |
| WP-1140 | Auth CAPTCHA v1: config-driven Turnstile/hCaptcha/reCAPTCHA, disabled by default. |
| WP-1150 | Health-probe availability rollups v1 with daily bucketing and uptime API. |
| WP-1160 | Per-provider quota/subscription fetch scaffold v1, fully config-driven. |
| A1.1 | AuthSession persistence: hashed `auth_sessions` storage survives restart. |
| A2.1 | Gateway API-key/user RPM and key TPM via Redis atomic counters. |
| A2.1.1 | Redis rate-limit p99 guard: `TestLimiterP99Budget` plus bench target. |
| A2.2 | Scheduler account-level quota evidence: rpm/tpm/concurrency reject reasons. |
| A2.3 | Provider-account RPM/TPM Redis counters after selection across paths. |
| A2.4 | Provider-account max-concurrency Redis ZSet leases with rollback on dispatch. |
| A4.1 | Scheduler failover foundations: ranked candidates, fallback decision linkage. |
| A4.2 | Gateway handlers consume ranked candidates with retry and failover evidence. |
| A4.2.2 | Direct-dispatch media/token-count handlers reuse the same retry/failover path. |
| A4.2.1 | Local schema repair drops obsolete single-column usage-log request_id index. |
| A2/A4 smoke gates | `smoke-rate-limit` and `smoke-failover` prove throttle and fallback evidence. |
| A5.1 | Generic reverse-proxy adapter accepts `generic-reverse-proxy` configured upstreams. |
| A5.2 | Provider preset install covers DeepSeek/Kimi/Qwen/Zhipu/Grok/Mistral/Groq/Together. |
| C3.3 | Gateway content safety redacts PII before dispatch, safe audit summaries. |
| K1.2 | Scheduler strategy loading refreshes persisted strategies into runtime descriptors. |
| K1.5 | Scheduler ranking applies Cost/Latency/Quality Pareto frontier before selection. |
| K1.6 | Scheduler strategy simulation: dry-run current-vs-shadow comparison, no persistence. |
| K1.6.1 | Scheduler simulation adds rollout-percent/key with deterministic hashed bucket. |
| K1.6.2 | Real Scheduler attempts persist sanitized `scheduler_request_snapshots` atomically. |
| K1.6.3 | Scheduler historical replay compares strategies over sanitized snapshots. |
| K1.6.4 | Scoped real-traffic Scheduler strategy rollout with sanitized hash evidence. |
| K1.7 | Admin strategy comparison UI `/admin/ops/strategy` with replay charts. |
| K1.4 | QualityEval: encrypted samples, hourly judge worker, quality_score evidence. |
| K1.8 | Scheduler explainability persists `selection_rationale` exposed through SDK. |
| C3.1 | Workspace persistence: workspaces plus user/api-key workspace linkage migrations. |
| C3.2 | Role permission persistence: roles permissions_json, entitlements cache, RBAC. |
| B1.2.1 | Usage charging composite index; pending charges scan oldest-first. |
| B1.2.2 | Balance charger throughput configurable; drains 20x500 backlog default. |
| B4.1.1 | Opt-in Stripe test-mode payment smoke with unit-covered reconciliation. |
| B4.2 | Alipay Official payment support: page/wap pay, signed notification verification. |
| B4.2.1 | Opt-in Alipay Page Pay smoke; webhook returns plain-text `success`. |
| B4.3 | WeChat Pay Official support: Native/H5/JSAPI, APIv3 notification decrypt. |
| B4.3.1 | Opt-in WeChat APIv3 smoke with signed/encrypted notification verification. |
| C1.1 | Structured trace spans over scheduler/payments/probe with OTLP export smoke. |
| C1.2 | SLO burn-rate evaluator worker creates/resolves burn-rate alert events. |
| B1.2.3 | Opt-in PostgreSQL balance_charger pressure gate seeds 10k pending logs. |
| C1.1.1 | Opt-in OTel HTTP tracing overhead gate with 5ms p99 budget. |
| C1.1.2 | Opt-in Jaeger visualization smoke verifies trace via Query API. |
| C1.1.3 | Opt-in Tempo visualization smoke verifies trace via Query API. |
| C1.1.4 | Ops alert posture metric plus AdminOps summary and Prometheus rules. |
| C1.1.5 | Local alert notification routing via Alertmanager to webhook receiver. |
| C1.1.6 | Local bootstrap generates strong secrets without touching existing env. |
| C1.1.7 | Env hygiene gate rejects weak/missing secrets and permissive permissions. |
| C1.1.8 | Compose deploy preflight verifies env, files, targets, and wiring. |
| A4.3 | Chat Completions streaming failover regression: primary 503 retries secondary. |
| Quality gate | Fixed web-check Vitest localStorage sharing; `make check` now repeatable. |
| Spec governance | Reconciled stale A5.2 guidance with implemented preset registry evidence. |
| Reference parity | OpenAI Responses/Codex compatibility: input_items, compact capability, request normalization. |
| Gemini compatibility | `GET /v1beta/models/{model}` returns Google-shaped metadata, no credentials/lease. |
| Reference parity | Codex upstream quota headers parsed into safe persisted quota snapshots. |
| Reference parity | Upstream 429 reset evidence feeds native account cooldown metadata. |
| Reference parity | Upstream 529/overloaded maps to `overloaded` class, 503, account cooldown. |
| Reference parity | API-key 401/403 applies conservative auth_failed cooldown, account stays active. |
| Reference parity | Account metadata declares native error_cooldown_rules by status/class/keyword. |
| Reference parity | Account `handled_error_status_codes` limit which statuses mutate account state. |
| Reference parity | Gateway endpoints gain bounded same-candidate transient retry before failover. |
| Reference parity | Direct-dispatch media/token-count endpoints share the same retry/failover invoker. |
| Reference parity | Provider error message passthrough is native and metadata-gated, sanitized. |
| Reference parity | Account health probes support metadata-defined synthetic probe profiles. |
| Reference parity | Gemini routes accept Google-friendly key forms; OpenAI/Anthropic stay Bearer-only. |
| Reference parity | Gateway exposes `GET /v1/usage` self-service key-scoped usage snapshot. |
| Reference parity | Reverse Proxy Runtime supports metadata-defined egress_profile subsets, uTLS templates. |
| Reference parity | Amazon Bedrock Anthropic Messages support: SigV4, event-stream decoding. |
| Reference parity | Payment provider updates protect in-progress orders from unsafe edits. |

current:

- package: Phase 1 production smoke and observability hardening
- status: API key/user rate limits, API key concurrency, scheduler account quota evidence, provider-account RPM/TPM Redis counters, provider-account ordinary HTTP concurrency Redis leases, local schema repair for multi-attempt usage evidence, non-streaming and streaming local failover attempt evidence, protocol-level OTLP trace export smoke, local Jaeger query visibility smoke, local Tempo query visibility smoke, the OTel HTTP p99 overhead guard, Ops alert posture metric/AdminOps summary/deployable Prometheus rules/local Alertmanager notification routing/default hygiene gate/local Prometheus+Alertmanager profile, strong local `.env` bootstrap generation, env hygiene checks, deploy preflight, the balance_charger local 10k pending-usage drain guard, an opt-in PostgreSQL balance_charger pressure harness, opt-in Stripe/Alipay/WeChat payment smoke entries, local payment webhook regressions, transactional auth email dispatch foundation, optional notification unsubscribe foundation, admin notification template management foundation, current-user notification preference management foundation, low-balance notification trigger foundation, subscription expiry reminder trigger foundation, account-quota notification trigger foundation, verified extra notification contact foundation, current-user avatar storage foundation, OAuth start/callback/pending decision preview/current-user bind/existing-account bind-login/create-account/email-completion, and the OpenAI Responses/Codex/Gemini compatibility plus Codex quota-snapshot, reset-aware cooldown, overload cooldown, and auth-failure cooldown increments above are implemented and locally verified where credentials are not required; live external provider/payment/email smoke still depends on valid upstream, merchant, or SMTP credentials.
- objective: continue closing production smoke, sandbox, collector-visualization, pressure-test gaps, and remaining reference-parity gaps without letting docs/specs drift.

next_recommended: WP-1110..WP-1170 + WP-1180 closed the verified sub2api gap batch, automated the quota fetch, and layered subscription-allowance-first billing over pay-as-you-go (user EAV, error-passthrough DB rules, TLS fingerprint DB profiles, auth CAPTCHA, health-probe availability rollups, per-provider quota/subscription fetch + scheduled refresh worker) — all behind `make check`. WP-1190/1200 added per-model and per-account-group RPM ceilings (model:<id>:rpm at admission, group:<id>:rpm after selection). The rate/capacity limiter matrix is complete and WP-1230 added the scheduled connectivity test runner. WP-1240/1250 closed the config backup/restore loop (export + natural-key import). The major sub2api capability gap surface is now broadly closed (OAuth depth, egress/SSRF/TLS, idempotency, EAV, error-passthrough, CAPTCHA, health rollups + connectivity runner, subscription-allowance billing, the full rate/capacity limiter matrix, config backup/restore). Remaining items are narrower / externally-gated: wire real Antigravity/Gemini-CLI/Codex subscription endpoints into the WP-1160/1170 config once their API shapes are confirmed (external dependency); credential-gated SMTP smoke (needs real SMTP credentials). The previously-listed in-house gaps are now closed: WP-1290 added OIDC id_token RS256/JWKS validation (vetted `go-oidc` dependency). WP-1300 is only partial — it formalized the OpenAPI schemas for all 24 new admin surfaces, but the handlers were never migrated and remain local-DTO at the wire (the formal types are reference/SDK material until an optional future rewire routes them through the generated structs). (Done this batch: WP-1270 confidential-client OAuth; WP-1280 ID-referencing config import via natural-key remap; WP-1290 OIDC id_token validation. Partial this batch: WP-1300 OpenAPI formalization of the new admin surfaces — spec only, handlers not migrated. Blocked on external inputs: real provider subscription endpoints, SMTP smoke.) External smoke also still depends on credentials for `make smoke-payment-stripe`, `make smoke-payment-alipay`, `make smoke-payment-wechat`, deployed collector trace visualization, `make balance-charger-pressure BALANCE_CHARGER_PRESSURE_DSN=...`, `make otel-overhead-bench` after OTel changes, or the remaining Phase 1 production pressure-test tasks.

last_gates:

- `cd apps/api && go test ./internal/modules/auth/... ./internal/persistence/entstore/auth -run 'Test(PendingOAuth|StorePersists|Cleanup|DeleteByUser|PasswordReset|EmailVerification|Login|Authenticate|Logout)' -count=1`: pass
- `make ent-generate-check`: pass
- `make migration-check`: pass
- `make architecture-check`: pass
- `make code-quality-check`: pass
- `git diff --check`: pass
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/workers/account_quota_alert ./internal/workers/outbox ./internal/modules/admin_control/... ./internal/app -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api/internal/modules/notifications apps/api/internal/modules/admin_control apps/api/internal/workers/account_quota_alert apps/api/internal/workers/outbox apps/api/internal/app apps/api/internal/architecture apps/api/internal/httpserver packages/openapi packages/sdk docs specs apps/web/src/components/admin/admin-resource-pages.tsx apps/web/tests/unit/admin-settings-form.test.ts`: pass
- `make code-quality-check`: pass
- `cd apps/web && npx vitest run tests/unit/admin-settings-form.test.ts`: pass
- `cd apps/web && npm run typecheck`: pass
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/modules/subscriptions/... ./internal/workers/subscription_expirer ./internal/workers/outbox ./internal/modules/admin_control/... -count=1`: pass
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/modules/subscriptions/... ./internal/workers/subscription_expirer ./internal/workers/outbox ./internal/modules/admin_control/... ./internal/app ./internal/httpserver -run 'Test(AuthPasswordResetEventSendsRenderedEmail|AuthEmailEventSkipsStaleRecipientHash|AuthEmailEventRequiresConfiguredSMTPAndBaseURL|BalanceLowEvent|SubscriptionExpiryEvent|PreferenceService|EnqueueSubscriptionExpiryReminders|RunOnce|CurrentUserNotificationPreferences|NotificationUnsubscribe|AdminSettings)' -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api/internal/modules/notifications apps/api/internal/modules/subscriptions apps/api/internal/workers/subscription_expirer apps/api/internal/workers/outbox apps/api/internal/app apps/api/internal/persistence/entstore/subscriptions apps/api/internal/modules/admin_control apps/api/internal/httpserver packages/openapi packages/sdk docs specs`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web file `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this WP did not edit that file.
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/workers/balance_charger ./internal/workers/outbox ./internal/modules/admin_control/... -count=1`: pass
- `cd apps/api && go test ./internal/workers/outbox ./internal/app -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api/internal/modules/notifications apps/api/internal/modules/admin_control apps/api/internal/workers/balance_charger apps/api/internal/workers/outbox apps/api/internal/app packages/openapi packages/sdk docs specs`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web file `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this WP did not edit that file.
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/notifications/... -count=1`: pass
- `cd apps/api && go test ./internal/httpserver -run 'Test(CurrentUserNotificationPreferences|NotificationUnsubscribeEndpoint)' -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api/internal/modules/notifications apps/api/internal/httpserver packages/openapi packages/sdk/typescript/src docs specs`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web file `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this WP did not edit that file.
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/modules/admin_control/... ./internal/config ./internal/workers/outbox ./internal/app ./internal/httpserver -run 'Test(AuthPasswordResetEventSendsRenderedEmail|AuthEmailEventSkipsStaleRecipientHash|AuthEmailEventRequiresConfiguredSMTPAndBaseURL|UpdateAdminSettingsNormalizesEmailConfigWithoutSMTPSecret|UpdateAdminSettingsRejectsInvalidEmailPublicBaseURL|EmailConfigDefaultsOverridesAndValidation|WorkerRetriesAuthEmailWhenEmailDeliveryNotConfigured|Register|UpdateAdminSettings|EmailVerification|PasswordReset|UpdateAdminSettingsEmailDoesNotAcceptSMTPPassword|AdminSettingsEmailPasswordConfiguredComesFromRuntimeConfig)' -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- .env.example apps/api/internal/config apps/api/internal/modules/admin_control apps/api/internal/modules/notifications apps/api/internal/workers/outbox apps/api/internal/app apps/api/internal/httpserver packages/openapi packages/sdk docs specs`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web files `apps/web/src/app/page.tsx:97` and `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this WP did not edit those files.
- `make ent-generate-check`: pass
- `make migration-check`: pass
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/auth/... ./internal/modules/users/... ./internal/persistence/entstore/auth ./internal/httpserver -run 'Test(RequestEmailVerification|ConfirmEmailVerification|VerifyEmail|EmailVerification|UpdateEmail|Register)' -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api/internal/modules/auth apps/api/internal/modules/users apps/api/internal/httpserver apps/api/internal/persistence/entstore/auth apps/api/internal/persistence/entstore/users apps/api/internal/platform/db apps/api/ent apps/api/migrations packages/openapi packages/sdk docs specs`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web files `apps/web/src/app/page.tsx:97` and `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this turn did not edit those files.
- `cd apps/api && go test ./internal/modules/users/... ./internal/modules/auth/... ./internal/persistence/entstore/users ./internal/httpserver -run 'Test(Register|CreateHashesPasswordAndDefaultsRole|AuthenticatePassword|CreateRole|UpdateBalance|LoginCreatesSession|CurrentUser)' -count=1`: pass
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api/internal/modules/users apps/api/internal/modules/auth apps/api/internal/persistence/entstore/users apps/api/internal/httpserver packages/openapi packages/sdk docs specs`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web files `apps/web/src/app/page.tsx:97` and `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this turn did not edit those files.
- `make migration-hash`: pass
- `make ent-generate-check`: pass
- `make migration-check`: pass
- `make openapi-lint`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/admin_control/... ./internal/persistence/entstore/admincontrol ./internal/platform/db ./internal/httpserver -run 'TestSystemLogsRecordListAndCleanup|TestAdminOpsSystemLogsListAndCleanup|TestConsoleWriteRoutesRequireCSRF|Test(PostgresVersionedUpMigrationsMatchEntSchema|PostgresDownMigrationsCoverCreatedTables|PostgresIncrementalMigrationsArePairedAndContiguous|EntSchemaAppliesToEmptyDatabase)' -count=1`: pass
- `make architecture-check`: pass
- `git diff --check -- apps/api docs specs packages/openapi packages/sdk`: pass
- `make code-quality-check`: blocked by pre-existing trailing whitespace in unrelated web files `apps/web/src/app/page.tsx:97` and `apps/web/src/components/admin/admin-resource-pages.tsx:1652-1653`; this turn did not edit those files.
- `git diff --check`: blocked by the same unrelated web trailing whitespace.
- `cd apps/api && go test ./internal/modules/reverse_proxy/service -count=1`: pass
- `cd apps/api && go test ./internal/modules/reverse_proxy/service -run 'TestRuntime(SupportsTLSEgressProfileThroughHTTPConnectProxy|RejectsTLSEgressProfileWithProxy|RejectsTLSEgressProfileWithHTTPSProxy|BuildsUTLSTransportForSupportedEgressProfile)' -count=1 -v`: pass
- `cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/modules/accounts/... -count=1`: pass
- `cd apps/api && go test ./...`: pass
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver -run 'Test.*Codex|TestRecordGatewayUsagePersistsProviderQuotaSignals|TestGatewayResponsesInputItemsAliasReplaysRawUpstreamJSON' -count=1`: pass
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver -run 'TestOpenAICompatibleAdapterClassifiesRateLimit|TestGatewayRateLimitFeedbackAppliesAccountCooldown|TestReverseProxyCodexCLIAdapterUsesResponsesOfficialClientShape' -count=1`: pass
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/gateway/... ./internal/httpserver`: pass
- `make architecture-check`: pass
- `make code-quality-check`: pass
- `git diff --check`: pass
- `make secret-scan`: pass
- `make smoke-payment-stripe`: credential-gated as expected on this workstation; exits before running because `STRIPE_SMOKE_SECRET_KEY` / webhook signing secret are not configured
- `make smoke-payment-alipay`: credential-gated as expected on this workstation; exits before running because real Alipay sandbox/test merchant app ID, merchant private key, and Alipay public key are not configured; optional local signed-notification mode also requires an explicit notification signing key
- `make smoke-payment-wechat`: credential-gated as expected on this workstation; exits before running because real WeChat Pay merchant app ID, merchant ID, APIv3 key, certificate serial number, and merchant private key are not configured; optional local signed-notification mode also requires an explicit platform notification signing key
- `cd apps/api && go test ./internal/httpserver -run 'TestTracingMiddleware' -count=1 -v`: pass; overhead guard skips unless `SRAPI_OTEL_P99_GUARD=1`
- `make otel-overhead-bench`: pass; local p99 overhead 0s against 5ms budget with 2,000 samples / 200 warmup
- `cd apps/api && go test ./internal/platform/otel -count=1 -v`: pass; Jaeger smoke skips unless `SRAPI_OTEL_JAEGER_SMOKE=1`
- `make smoke-jaeger-trace`: pass; local Jaeger all-in-one accepted OTLP/gRPC span and Query API returned the trace
- `make smoke-tempo-trace`: pass; local Tempo accepted OTLP/gRPC span and Query API returned the trace
- `cd apps/api && go test ./internal/modules/operations/...`: pass
- `cd apps/web && npm run test -- admin-ops-alerts.test.ts`: pass
- `cd apps/web && npm run typecheck`: pass
- `cd apps/web && npx eslint src/lib/admin-ops-alerts.ts src/components/admin/admin-resource-pages.tsx tests/unit/admin-ops-alerts.test.ts`: pass
- `cd apps/web && npx prettier --check src/lib/admin-ops-alerts.ts tests/unit/admin-ops-alerts.test.ts`: pass
- `make observability-rules-check`: pass
- `node --test tools/bootstrap-env.test.mjs`: pass
- `node --check tools/bootstrap-env.mjs && node --check tools/bootstrap-env.test.mjs`: pass
- `npx prettier --check tools/bootstrap-env.mjs tools/bootstrap-env.test.mjs`: pass
- `SRAPI_BOOTSTRAP_ENV_FILE=$(mktemp -d)/.env make bootstrap-env`: pass; generated a private `.env` with no weak local placeholders
- `node --test tools/env-check.test.mjs`: pass
- `node --check tools/env-check.mjs && node --check tools/env-check.test.mjs`: pass
- `npx prettier --check tools/env-check.mjs tools/env-check.test.mjs`: pass
- `SRAPI_ENV_CHECK_FILE=$(mktemp -d)/.env make env-check`: pass against a bootstrap-generated env file
- `SRAPI_ENV_CHECK_FILE=$(mktemp -d)/.env make env-check`: fails as expected against copied `.env.example` with weak placeholders and permissive permissions
- `node --test tools/deploy-preflight.test.mjs`: pass
- `node --check tools/deploy-preflight.mjs && node --check tools/deploy-preflight.test.mjs`: pass
- `npx prettier --check tools/deploy-preflight.mjs tools/deploy-preflight.test.mjs`: pass
- `SRAPI_DEPLOY_PREFLIGHT_ENV_FILE=$(mktemp -d)/.env make deploy-preflight`: pass against a bootstrap-generated env file; warns on this workstation because Docker Compose is unavailable
- `node --test tools/observability-rules-check.test.mjs`: pass
- `node --check tools/observability-rules-check.mjs && node --check tools/observability-rules-check.test.mjs`: pass
- `npx prettier --check tools/observability-rules-check.mjs tools/observability-rules-check.test.mjs`: pass
- `cd apps/api && go test ./internal/codequality -run 'Test(NodeScriptsParse|NodeUnitTestsPass|ContainerFilesUsePinnedNonRootDefaults|MakeCheckRunsMandatoryQualityGates)' -count=1`: pass
- `cd apps/api && go test ./internal/codequality -count=1`: blocked by concurrent protocol/adapter-side growth in `apps/api/internal/modules/gateway/service/service.go` (2757 lines > 2180) and `apps/api/internal/modules/provider_adapters/service/conversation_protocols.go:1844` (`parseAnthropicCompatibleStream` 213 lines > 210); this turn did not edit those protected files.
- `cd apps/api && go test ./internal/httpserver -run 'TestGatewayChatCompletion(FailoverRecordsAttemptEvidence|StreamFailoverBeforeDownstreamWrite)' -count=1 -v`: pass
- `make check`: pass; includes OpenAPI lint/bundle/codegen checks, SDK typecheck, migration check, architecture/code-quality/API tests, examples check, secret scan, web typecheck/lint/unit/build, and bundle budget
- `cd apps/api && go test ./internal/modules/providers/preset -run TestDefaultRegistrySeedsCompatiblePresets -count=1 -v`: pass
- `cd apps/api && go test ./internal/httpserver -run TestAdminInstallProviderPresetsIsIdempotent -count=1 -v`: pass
- `cd apps/api && go test ./...`: pass
- `cd apps/api && go test ./internal/httpserver -run TestGatewayRealtimeWebSocketRelaysOpenAIUpstreamWebSocket -count=1 -v`: pass after one concurrent `architecture-check` run exposed a transient existing realtime WebSocket timing failure
- `make architecture-check`: pass on rerun
- `cd apps/api && go test ./internal/workers/balance_charger -count=1 -v`: pass, pressure test skipped because `SRAPI_BALANCE_CHARGER_PRESSURE_DSN` was not configured
- `make balance-charger-pressure BALANCE_CHARGER_PRESSURE_DSN=postgres://...`: pass against local `srapi-dev-postgres`, charged 10,000 usage logs in 1.55s inside the test run
- Official OpenAI docs review: pass; Responses `input_items` query and compact opaque compaction semantics checked against current official API docs before finalizing this compatibility increment.
- `make openapi-lint`: pass
- `make openapi-bundle`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/gateway/... ./internal/httpserver -run 'Test.*(Responses|ResponseInputItems|Compact|Codex|WebSocket|Realtime)' -count=1`: pass
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/gateway/... ./internal/httpserver`: pass
- `make architecture-check`: pass
- `make code-quality-check`: pass
- `git diff --check`: pass

notes:

- Existing `docs/` remains the architecture and domain source of truth.
- Stripe, Alipay Page Pay, and WeChat Pay APIv3 smoke checks now have first-class Make targets and script tests, but actual external runs still require the corresponding merchant/test credentials; Alipay and WeChat optional local signed-notification modes are SRapi webhook-path smokes and do not replace externally delivered platform callbacks.
- Local Jaeger trace visibility is covered by `make smoke-jaeger-trace`; local Tempo trace visibility is covered by `make smoke-tempo-trace`; deployed collector/query backends still require topology-specific smoke.
- The balance_charger PostgreSQL pressure gate passed against the local dev Postgres container; rerun it against production-adjacent storage before claiming deployed database throughput under real IO.
- The rate-limit p99 guard is now available, but this workstation did not produce a valid 2ms Redis baseline; rerun it against local/native or production-adjacent Redis before claiming the limiter p99 budget is met.
- Historical strategy replay can only be claimed for decisions that have `scheduler_request_snapshots`; older decision-only rows remain report-only because they lack the full request profile and candidate set.
- Future goal runs must read `specs/README.md` first, then continue from `next_recommended`.
- Future goal runs must preserve unrelated user worktree changes if present.
- A5.2 now has code/test/docs evidence in the provider preset registry, admin provider install API, admin provider test diagnostics, and `../design/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`; no real upstream credential is required for the representative test coverage.
- Frontend visual implementation deferral was lifted by WP-160a: tone, component library, data layer, harness, e2e, bundle budget all landed under `apps/web` and `tools/`. `make web-check` is part of `make check`. New tone source: `../../docs/requirements/PRODUCT_TONE.md`. New architecture source: `../../docs/requirements/FRONTEND_ARCHITECTURE.md`. Future frontend goal runs must respect both, plus `../../docs/requirements/FRONTEND_DESIGN_SYSTEM.md`.
- WP-500 keeps discovery responses credential-free while allowing reverse-proxy Antigravity accounts to use selected credentials upstream.
- WP-080 added `TestGatewayCompatibilityEndpointsTargetSameOpenAICompatibleUpstream`, which records three upstream `/v1/chat/completions` calls using one OpenAI-compatible account and verifies provider/account usage evidence.
- `make smoke-gateway` passed against a temporary local API on `127.0.0.1:18080`; the temporary process was stopped after the run.
- WP-090 added `TestSchedulingScenarioMatrixMVP`, `TestRecordFailedFeedbackMarksLeaseFailed`, and `TestRedisLeaseStoreAllowsOnlyOneConcurrentAcquire`; no production Scheduler logic changed.
- WP-100 added service-level evidence tests for usage, billing decimal-string ledger entries, and audit records, plus an HTTP regression proving Gateway requests record Scheduler feedback through the runtime store.
- WP-100 smoke-gateway revalidated the local Gateway endpoints on `127.0.0.1:18080` and the temporary process was stopped after the run.
- WP-110 added `TestGatewayProviderAliasUsesPresetProviderKey` to prove non-generic preset aliases such as `/api/provider/deepseek/v1/chat/completions` force provider context while reusing Gateway runtime and Scheduler evidence.
- WP-120 reused the existing runtime foundation and verified it with module gates plus focused HTTP regressions for session-invalid auto protection and OAuth refresh persistence/failure handling.
- WP-130 added `TestGatewayReverseProxyBanSignalDisablesAccountAndStopsScheduling` to prove `account_banned` disables the account and prevents subsequent upstream dispatch.
- WP-140 added `TestRuntimeInjectsCliClientTokenAndDefaultClientUserAgent`, `TestReverseProxyAdapterPassesCliRuntimeContext`, `TestRoutingHintsAreRecordedWithoutLeakingAffinityKey`, and `TestGatewayModelAliasAndSessionAffinityFeedScheduler` to prove CLI runtime context, model alias strategy, and session affinity are explicit Scheduler/runtime inputs.
- WP-150 expanded `AccountHealthSnapshot` in OpenAPI and generated SDKs, added health summary derivation from account metadata/usage logs, and extended the rate-limit cooldown HTTP regression to prove operator-visible runtime/error/quota/cooldown diagnostics.
- WP-160 (frontend visual implementation) was deferred at the time, then delivered in full later: the frontend was rebuilt and completed under WP-1310. This note is retained for provenance.
- WP-1310 is the frontend follow-through: after the `apps/web` rewrite landed, an audit found the console exposed only a fraction of the backend it sits on. WP-1310 slice 1 fixed the P0 inverted account toggle, made every admin page reachable via grouped navigation, wired the account-domain dead capabilities, rebuilt the admin dashboard on the admin snapshot, added the payment-providers page + redeem stats + usage aggregates, fixed silent-failure feedback gaps, localized the error/404 pages, and refreshed `../../docs/requirements/FRONTEND_ARCHITECTURE.md` — all under `make web-check` (typecheck/lint/i18n parity). Slices 1+2 then closed the rest, including group member management — the one item that required a backend change: a new `GET /api/v1/admin/account-groups/{id}/accounts` endpoint (OpenAPI-first + regenerated Go/TS SDKs + `TestAdminListAccountGroupMembers` regression, reusing the existing `accounts.ListGroupMembers` service/store) plus a frontend Manage-members dialog. A follow-on graphical-config increment (mirroring sub2api's easy-config UX — operators pick, they don't hand-write JSON) replaced the model `capabilities` JSON-array textarea with a multi-select chip selector: a reusable `multiselect` field type on `ResourceFormDialog`, `lib/capabilities.ts` (canonical capability options + key↔descriptor mapping), and a `capabilities: string[]` model-form selection mapped to standard `v1/stable/required` descriptors on save (no backend change — the API already accepts the descriptor array). On the user side, the API-key create dialog's allowed-models and account-groups comma-separated text fields became a reusable `TagInput` chip control (`components/ui/tag-input.tsx` — type-to-add, click-to-remove, dedupe), and the dialog now catches create-mutation rejections (previously a silent throw); the dead `parseGroupIdsCsv` helper was removed. A follow-on key-value editor then replaced the remaining freeform-JSON-object textareas: a reusable `KeyValueEditor` component + a `keyvalue` `ResourceFormDialog` field type (rows of key + value; each value parsed JSON-if-valid-else-string, so scalars are natural and nested values stay expressible) now drive account `metadata`, subscription plan `entitlements`, payment-provider `config`/`limits`/`metadata`, and provider `capabilities`/`config_schema` — no hand-written JSON anywhere in the admin/user create-edit forms (capabilities use chips, the rest use key-value grids; the per-account OAuth/service-account credential stays a structured JSON box by design). All `*Json` form-state strings + their `parseJsonObject`/`prettyJson` helpers were removed. Browser verification then closed the WP against the live stack and, in doing so, caught two out-of-original-scope defects on the regular-`user` surface, both fixed: the user dashboard (`GatewayOverview`) was hitting admin-only endpoints (403 for users) and the user nav linked to admin-gated gateway pages — so the dashboard was slimmed to `/me/usage`-only KPIs + recent activity, the "Gateway" nav group is now admin-only, and the dead admin-overview hook/method/key/import chain was removed; separately, `ResourceFormDialog` now always renders a `DialogDescription` (sr-only fallback) to clear a Radix a11y warning. WP-1310 is fully closed (web typecheck/lint/i18n parity all green after the fixes).
- WP-170 added account group operations, account inspect/export/import, proxy bind, recover, persisted test/gateway health and quota snapshots, recursive export metadata sanitization, expanded CSRF regression coverage, and generated SDK methods for account operations.
- WP-180 added `GET /api/v1/me/subscriptions`, admin subscription plan/user subscription/pricing rule APIs, entitlement rejection before Scheduler lease consumption, decimal-normalized pricing rule responses, pricing metadata on billing ledger entries, generated SDK methods, Ent/migration parity, and CSRF coverage for new console writes.
- WP-190 added current-user and admin payment APIs, encrypted payment provider config, legal order state transitions, signed/idempotent webhook handling, fulfillment-side billing/subscription/audit/outbox effects, refund hooks, Ent/Postgres persistence, migration drift coverage, and generated SDK/OpenAPI parity.
- WP-200 added the affiliate module, Ent schemas and PostgreSQL tables for invite/affiliate ledgers, payment outbox dispatch into affiliate accrual/compensation, refund compensation capping, and transfer-to-balance tests proving affiliate ledger, billing ledger, user balance, and audit evidence stay aligned.
- WP-200 added current-user affiliate APIs for summary, ledger, and transfer-to-balance with CSRF plus `Idempotency-Key` protection, generated Go/TypeScript OpenAPI artifacts, and HTTP regressions proving current-user ledger isolation and duplicate-transfer idempotency. Frontend visuals remain deferred per explicit user instruction.
- WP-210 `/metrics` samples now come from process-local gateway/scheduler metric state, realtime/reverse-proxy runtime state, ops alert state, and availability rollups; release smoke checks health/readiness/metrics plus mock Gateway flow.
- WP-210 retention cleanup is bounded by `DATA_RETENTION_BATCH_LIMIT` for usage logs, scheduler decisions/feedbacks, audit logs, and account health snapshots; financial ledgers, payment records, affiliate ledgers, credentials, and user state remain excluded from automatic cleanup.
- WP-210 added `make backup-postgres` and `make restore-postgres` with checksum support; secret material such as `SRAPI_MASTER_KEY` remains outside database backups and must be backed up through the deployment secret store.
- WP-210 Docker Compose smoke could not be executed in this environment because `docker compose` and `docker-compose` are unavailable.
- WP-220 added Anthropic-compatible Adapter dispatch for API-key and reverse-proxy accounts, including `x-api-key`/`anthropic-version` API-key headers, `/messages` endpoint derivation, Anthropic usage/cache token parsing, SSE aggregation, and Anthropic error classification.
- WP-220 added `TestGatewayAnthropicProviderAliasTargetsMessagesUpstream` to prove `/api/provider/anthropic-compatible/v1/messages` forces the Anthropic-compatible provider while reusing Gateway auth, model policy, Scheduler decisions, and usage evidence.
- WP-230 added `/v1beta/models/{model}:generateContent` and `/v1beta/models/{model}:streamGenerateContent` as Gateway routes, including OpenAPI/SDK schemas, generated Go/TypeScript artifacts, Canonical conversion, Gemini response/SSE rendering, Google RPC-style error rendering, usage/decision evidence, and docs alignment.
- WP-230 intentionally stops short of a Gemini-native upstream `generateContent` provider adapter; that belongs to WP-240+ Provider Expansion.
- WP-240 added Gemini-compatible/native-gemini adapter dispatch to `/models/{model}:generateContent` and `/models/{model}:streamGenerateContent`, API-key query/header/bearer auth handling, Gemini usageMetadata parsing, Google error classification, and reverse-proxy-gemini-cli runtime dispatch.
- WP-240 added `TestGatewayGeminiGenerateContentSchedulesGeminiCompatibleUpstream` to prove Gemini Gateway routes can schedule Gemini-compatible upstream accounts while preserving usage and Scheduler decision evidence.
- WP-260 added `obs_slo_definitions` and `obs_alert_events`, `GET/POST/PATCH /api/v1/admin/ops/slo`, `GET /api/v1/admin/ops/alerts`, and `POST /api/v1/admin/ops/alerts/{id}/ack`; SLO objective accepts ratio or percent input and persists ratios.
- WP-260 alert ack audit intentionally records only safe alert summaries, not `details_json`.
- WP-270 added `POST /v1/embeddings` and OpenAI-compatible provider alias embeddings routes; token-array input is intentionally rejected until a later compatibility package.
- WP-270 also identified `apps/api/internal/httpserver/runtime_http.go` as oversized at 7750 lines; WP-280 is dedicated to partitioning this file and adding a harness check so this does not remain accepted architecture.
- WP-280 reduced `apps/api/internal/httpserver/runtime_http.go` from 7750 lines to a package shell and split implementation across `runtime_state.go`, `runtime_user_handlers.go`, `runtime_admin_*`, `runtime_gateway_*`, `runtime_metrics.go`, helper/mapping/filter files.
- WP-280 added `TestHTTPRuntimeFilesStayPartitioned`, enforcing `runtime_http.go` <= 120 lines and each `runtime_*.go` <= 2200 lines.
- WP-290 added `images` to the canonical capability registry and provider convenience key mapping, so Scheduler requires explicit image endpoint support before routing image generation requests.
- WP-290 added `TestOpenAICompatibleAdapterInvokesImageGenerationsUpstream`, `TestGatewayImageGenerationRouteTargetsOpenAICompatibleUpstream`, and `TestGatewayImageGenerationAliasForcesProviderContext`.
- WP-300 calibrated code-quality thresholds to current production code: non-generated production Go files must stay at or below 2180 lines, and non-generated production functions must stay at or below 210 lines. The harness also enforces `git diff --check`, mandatory `make check` gate composition, generated-contract and lockfile secret-scan coverage, repository text hygiene, script syntax, baseline container hygiene, no speculative production markers, and restricted panic/recover usage.
- WP-310 added `moderations` to the canonical capability registry and provider convenience key mapping, so Scheduler requires explicit moderation endpoint support before routing moderation requests.
- WP-310 added `TestOpenAICompatibleAdapterInvokesModerationsUpstream`, `TestGatewayModerationRouteTargetsOpenAICompatibleUpstream`, and `TestGatewayModerationAliasForcesProviderContext`.
- WP-320 added `rerank` to the canonical capability registry and provider convenience key mapping, so Scheduler requires explicit rerank endpoint support before routing rerank requests.
- WP-320 added `TestRerankCompatibleAdapterInvokesRerankUpstream`, `TestGatewayRerankRouteTargetsRerankCompatibleUpstream`, and `TestGatewayRerankAliasForcesProviderContext`.
- WP-330 added `audio_transcriptions` to the canonical capability registry and provider convenience key mapping, so Scheduler requires explicit audio transcription endpoint support before routing speech-to-text requests.
- WP-330 added `TestOpenAICompatibleAdapterInvokesAudioTranscriptionsUpstream`, `TestGatewayAudioTranscriptionRouteTargetsOpenAICompatibleUpstream`, and `TestGatewayAudioTranscriptionAliasForcesProviderContext`.
- WP-340 added `audio_speech` to the canonical capability registry and provider convenience key mapping, so Scheduler requires explicit speech synthesis endpoint support before routing text-to-speech requests.
- WP-340 added `TestOpenAICompatibleAdapterInvokesAudioSpeechUpstream`, `TestGatewayAudioSpeechRouteTargetsOpenAICompatibleUpstream`, and `TestGatewayAudioSpeechAliasForcesProviderContext`.
- WP-350 verified local client shape without reading credentials: Codex CLI is installed as `codex-cli 0.133.0`, Claude Code is installed as `2.1.144 (Claude Code)`, and Antigravity is installed as desktop app metadata `Antigravity 1.107.0`.
- WP-350 added `TestRuntimeInjectsAntigravityDesktopTokenAndDefaultUserAgent`, `TestReverseProxyAntigravityOpenAIAdapterDispatchesThroughRuntime`, `TestReverseProxyAntigravityAnthropicAdapterDispatchesThroughRuntime`, `TestReverseProxyAntigravityGeminiAdapterDispatchesThroughRuntime`, and `TestGatewayAntigravityReverseProxyUsesDesktopRuntimeIdentity`.
- WP-360 added the `antigravity` preset with `/api/provider/antigravity` and `/api/provider/antigravity/v1` text aliases plus desktop/IDE reverse-proxy account allowlist.
- WP-360 changed provider alias registration to read endpoint capabilities from preset metadata; the registry test now guards OpenAI image/audio capabilities so dynamic alias coverage does not regress.
- WP-360 added `TestGatewayAntigravityProviderAliasTargetsOpenAIReverseProxy` and `TestGatewayAntigravityProviderAliasTargetsAnthropicReverseProxy`, proving Antigravity aliases force `provider_key=antigravity`, preserve alias source endpoints, and dispatch through Reverse Proxy Runtime using `provider.protocol`.
- WP-370 added Antigravity Gemini model-action alias metadata to the provider preset registry and HTTP registration for `/api/provider/antigravity/v1beta/models/{model}:generateContent` and `/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent`.
- WP-370 added `TestGatewayAntigravityGeminiAliasTargetsReverseProxy` and `TestGatewayAntigravityGeminiStreamAliasTargetsReverseProxy`, proving alias source endpoints, forced provider context, mapped upstream model dispatch, Gemini JSON/SSE rendering, and usage/Scheduler evidence.
- WP-380 added `TestGatewayResponsesWebSocketTargetsResponsesRuntime` and `TestGatewayResponsesWebSocketForwardsStreamingEvents`, proving WebSocket non-streaming and streaming requests reuse `/v1/responses` auth/model policy/Scheduler/Provider Adapter/usage paths and preserve `/v1/responses/ws` as the source endpoint.
- WP-380 added `nhooyr.io/websocket` only for transport handshake/frame handling; business protocol stays in the existing Responses runtime and direct upstream WSS relay remains a follow-up package.
- Local client availability is confirmed in PATH for `codex`, `claude`, and `antigravity`. Codex CLI and Claude Code CLI reverse-proxy runtime identities already exist (`reverse-proxy-codex-cli`, `reverse-proxy-claude-code-cli`) and Antigravity alias routing now covers OpenAI-compatible, Anthropic-compatible, and Gemini-compatible routes; actual reverse-proxy use still requires configured Provider/Account/base_url/token records.
- WP-390 added `TestRuntimeRelaysWebSocketWithAccountContextAndHeaderHygiene` and `TestRuntimeRelaysWebSocketWebSessionCookieFromCredential`, proving direct upstream WebSocket relay uses account credentials, default upstream-client User-Agent, subprotocol negotiation, text/binary message relay, metrics, and sanitized handshake headers.
- WP-390 intentionally does not add Gateway route binding or provider-native realtime schema adapters; those are next-stage realtime packages that can now depend on the runtime primitive.
- WP-400 added `TestReverseProxyCodexCLIAdapterUsesResponsesOfficialClientShape`, proving Codex 2api sends `base_url + "/responses"` with selected account token, Codex official-client headers, Responses-style body, and Codex SSE parsing, without adding Gateway-local Codex DTOs.
- WP-400 also added `TestRuntimeDoesNotInjectAPIKeyRuntimeCredentials` and `TestReverseProxyCodexCLIRejectsAPIKeyRuntime`, so SRapi's 2api reverse-proxy boundary stays aligned with OAuth/session/client-token accounts rather than official API-key adapters.
- WP-410 added `TestReverseProxyCodexCLIPrepareRealtimeBuildsResponsesWebSocketSession`, `TestReverseProxyCodexCLIPrepareRealtimeRejectsAPIKeyRuntime`, and `TestGatewayResponsesWebSocketRelaysCodexUpstreamWebSocket`, proving Codex Responses WebSocket 2api uses selected account OAuth/CLI token credentials, Codex official-client headers, mapped upstream model frames, and `/v1/responses/ws` Scheduler/usage evidence.
- WP-410 intentionally keeps WebSocket relay opt-in via `upstream_ws` / `codex_responses_websocket` flags and account metadata; persistent Codex session reuse, local Codex CLI ingress, and Claude/Antigravity WebSocket adapters remain follow-up work.
- WP-420 added `TestReverseProxyClaudeCodeCLIAdapterUsesOfficialClientMessagesShape`, `TestReverseProxyClaudeCodeCLIRejectsAPIKeyRuntime`, and `TestGatewayClaudeCodeReverseProxyUsesOfficialClientMessagesShape`, proving Claude Code 2api uses selected account OAuth/CLI token credentials, Claude Code official-client headers/body, `/messages?beta=true`, and Scheduler/usage evidence without Gateway-local DTOs.
- WP-420 refreshed `../../docs/constraints/2API_REVERSE_PROXY_DEFINITION.md` to define SRapi 2api/反代 from the local reference projects `sub2api`, `CLIProxyAPI`, and `chatgpt2api`: upstream official-client request simulation with OAuth/session/client-token identity, not local Codex/Claude/Antigravity client ingress.
- WP-430 added `TestReverseProxyChatGPTWebAdapterUsesConversationOfficialClientShape`, `TestReverseProxyChatGPTWebRejectsAPIKeyRuntime`, and `TestGatewayChatGPTWebReverseProxyUsesConversationOfficialClientShape`, proving ChatGPT Web 2api uses selected account OAuth/session credentials, browser/OAI/Sentinel headers, `/backend-api/conversation`, ChatGPT Web Conversation body, and Scheduler/usage evidence without Gateway-local DTOs.
- WP-440 added `TestReverseProxyChatGPTWebAdapterAutoFetchesRequirements`, `TestReverseProxyChatGPTWebMissingRequirementsCanDisableAutoFetch`, and updated `TestGatewayChatGPTWebReverseProxyUsesConversationOfficialClientShape`, proving missing static requirements tokens trigger selected-account bootstrap and `/backend-api/sentinel/chat-requirements` before conversation without Gateway-local DTOs.
- WP-440 intentionally does not implement external Arkose/Turnstile solving, challenge token persistence, or browser TLS impersonation.
- WP-450 added Antigravity official-client upstream shape coverage across OpenAI-compatible, Anthropic-compatible, Gemini-compatible adapter inputs plus `TestReverseProxyAntigravityRejectsAPIKeyRuntime` and an updated Gateway regression proving `/v1/chat/completions` schedules an Antigravity desktop account and sends `/v1internal:generateContent` with selected account bearer credentials.
- WP-450 intentionally does not implement Antigravity OAuth onboarding, project discovery, credit overage retry policy, full tool-schema cleaning, or persistent realtime session lifecycle.
- WP-460 added `TestAcquireReleaseTracksRealtimeSlotLifecycle`, global/per-API-key slot-limit tests, and `TestGatewayResponsesWebSocketEnforcesRealtimeSlotLimit`, proving raw session affinity keys are hashed and excess WebSocket handshakes fail with 429 before upgrade.
- WP-460 intentionally does not add Claude Code or Antigravity provider-native realtime adapters, persistent upstream session reuse, or distributed Redis-backed slot storage.
- WP-470 added `TestNormalizeRealtimeWebSocketRequiresRealtimeCapability`, `TestOpenAICompatiblePrepareRealtimeBuildsRealtimeWebSocketSession`, and `TestGatewayRealtimeWebSocketRelaysOpenAIUpstreamWebSocket`, proving `/v1/realtime` uses selected account OAuth identity, mapped upstream model query, allowed Realtime headers, realtime slot lifecycle, Scheduler decisions, and usage evidence.
- WP-630 added `TestOpenAICompatiblePrepareRealtimeAllowsAPIKeyRuntime` and `TestGatewayRealtimeWebSocketRelaysOpenAIAPIKeyUpstreamWebSocket`, proving official API-key Realtime uses selected account API-key credentials, preserves `OpenAI-Safety-Identifier`, strips caller auth/cookie/SRapi headers, and keeps `/v1/realtime` Scheduler/usage evidence.
- WP-470/WP-630 intentionally do not add persistent upstream session pools, local client ingress, browser ephemeral Realtime token minting, or Claude Code / Antigravity provider-native realtime adapters.
- WP-480 added `TestOpenAICompatibleAdapterInvokesImageEditsUpstream`, `TestGatewayImageEditRouteTargetsOpenAICompatibleUpstream`, and `TestGatewayImageEditAliasForcesProviderContext`, proving multipart `image` / optional `mask`, mapped upstream model, provider alias context, usage logs, and Scheduler decisions for `/v1/images/edits`.
- WP-480 intentionally does not add JSON image references, streaming image edit events, image variations, or frontend visuals.
- WP-490 verified the current OpenAI OpenAPI spec for `/images/variations`: multipart `POST`, required single `image`, optional `n`, `size`, `response_format`, and `user`, with upstream support currently limited to `dall-e-2`.
- WP-490 added `TestOpenAICompatibleAdapterInvokesImageVariationsUpstream`, `TestGatewayImageVariationRouteTargetsOpenAICompatibleUpstream`, and `TestGatewayImageVariationAliasForcesProviderContext`, proving multipart `image`, mapped upstream model, provider alias context, usage logs, and Scheduler decisions for `/v1/images/variations`.
- WP-490 intentionally does not add multi-image variations, JSON image references, streaming image variation events, or frontend visuals.
- WP-530 added `TestAdminAccountModelDiscoveryBootstrapsAntigravityProjectFromLoadCodeAssist` and `TestAdminAccountModelDiscoveryPersistsAntigravityOnboardedProject`, proving Antigravity discovery uses selected-account OAuth/desktop/IDE credentials through Reverse Proxy Runtime for `/v1internal:loadCodeAssist`, `/v1internal:onboardUser`, and `/v1internal:fetchAvailableModels`.
- WP-530 intentionally does not invoke local Antigravity, add Gateway-local DTOs, or implement full Antigravity OAuth onboarding UI/API, credit overage retry policy, or provider-native realtime.
- WP-540 added `TestGatewayGeminiListModels` and `TestGatewayGeminiListModelsRejectsInvalidPaginationAndDisabledKey`, proving Gemini models.list shape, API-key visibility filtering, pagination, supported generation method derivation, and Google-style errors.
- WP-540 intentionally does not schedule Provider Accounts, call upstream Gemini model discovery, create usage records for catalog listing, or add frontend visuals.
- WP-550 added `TestGeminiCompatibleAdapterCountsTokensUpstream`, `TestReverseProxyGeminiAdapterCountsTokensThroughRuntime`, and `TestGatewayGeminiCountTokensSchedulesGeminiCompatibleUpstream`, proving Gemini countTokens API-key dispatch, selected-account reverse-proxy dispatch, Scheduler capability filtering, Google-shaped response rendering, and zero generation usage/cost evidence. The countTokens service logic is split into ownership-specific `gemini_count_tokens.go` files so `code-quality-check` file-size gates stay meaningful.
- WP-560 added `TestAnthropicCompatibleAdapterCountsTokensUpstream`, `TestReverseProxyClaudeCodeAdapterCountsTokensThroughRuntime`, `TestGatewayAnthropicCountTokensSchedulesAnthropicCompatibleUpstream`, and `TestGatewayAnthropicCountTokensRequiresProviderScopedCapability`, proving Anthropic count_tokens API-key dispatch, Claude Code 2api Reverse Proxy Runtime dispatch, upstream model mapping inside the preserved count_tokens body shape, Scheduler capability filtering, Anthropic-shaped response rendering, and zero generation usage/cost evidence.
- WP-560 intentionally does not implement OpenAI tokenizer estimation, provider-native realtime adapters, SDK examples, or frontend visuals.
- WP-570 added `TestAdminOpsRealtimeSlotsListsActiveSlotsSafely` and extended realtime service lifecycle tests, proving `GET /api/v1/admin/ops/realtime/slots` lists active current-node slots and counters while returning only hashed affinity metadata and no API key, credential, prompt, or provider-specific frame content.
- WP-570 intentionally does not implement distributed Redis-backed slot storage, persistent upstream session pools, provider-native Claude Code / Antigravity realtime protocols, local client ingress, or frontend visuals.
- WP-580 added `examples/README.md`, curl / TypeScript / Python examples, `../../docs/insights/MIGRATION_GUIDE_2API.md`, and `tools/examples-check.mjs`; the harness checks required routes, env vars, TypeScript SDK compile compatibility, 2api boundary phrases, README/docs links, and secret-like placeholders.
- WP-580 intentionally does not add frontend visuals, local Codex / Claude Code / Antigravity ingress, Gateway-local provider DTOs, or runnable upstream credential import automation.
- WP-590 added `TestRedisRealtimeStoreEnforcesDistributedSlotLimits`, `TestRedisRealtimeStoreReleaseFromAnotherInstanceFreesCapacity`, `TestRedisRealtimeStoreExpiresSlotsWithoutLeakingSensitiveData`, and app wiring tests proving Redis-backed realtime slots are used when available, required in release mode, and safe to inspect from AdminOps.
- WP-590 intentionally does not add provider-native Claude Code / Antigravity realtime adapters, persistent upstream session pools, local Codex / Claude Code / Antigravity ingress, or Codex refresh-token-only import automation.
- WP-600 added `TestRuntimeRefreshUsesPerAccountLockAndDoesNotOverwriteOnFailure`, `TestRuntimeRefreshClassifiesInvalidGrantWithoutOverwritingCredential`, `TestGatewayCodexRefreshTokenOnlyCreateCanRequestResponses`, `TestAdminAccountImportCodexRefreshTokenOnlyExchangesTokenWithoutLeakingCredential`, and `TestGatewayCodexRefreshTokenOnlyUpdateCanRequestResponses`, proving Codex refresh-token-only create/import/update exchanges and persists usable OAuth state before Gateway dispatch.
- WP-600 intentionally does not add local Codex CLI ingress or Gateway-local Codex DTOs; Claude Code and Antigravity refresh-token-only import landed in WP-610 and WP-620.
- WP-610 added `TestClaudeRefreshUsesJSONTokenRequest`, `TestGatewayClaudeRefreshTokenOnlyCreateCanRequestMessages`, `TestAdminAccountImportClaudeRefreshTokenOnlyExchangesTokenWithoutLeakingCredential`, and `TestGatewayClaudeRefreshTokenOnlyUpdateCanRequestMessages`, proving Claude Code refresh-token-only create/import/update exchanges and persists usable OAuth state before Gateway `/v1/messages` dispatch.
- WP-620 added `TestAntigravityRefreshUsesClientSecretFormTokenRequest`, `TestAntigravityRefreshRequiresClientSecret`, `TestGatewayAntigravityRefreshTokenOnlyCreateCanRequestChat`, `TestAdminAccountImportAntigravityRefreshTokenOnlyRequiresClientSecret`, and `TestGatewayAntigravityRefreshTokenOnlyUpdateCanRequestChat`, proving Antigravity refresh-token-only create/update requires configured client secret, exchanges via form OAuth, and persists usable OAuth state before Gateway text dispatch.
- Antigravity refresh-token-only import requires encrypted credential `oauth_client_secret` / `client_secret`; SRapi intentionally does not hard-code the Google OAuth client secret or expose it through account metadata.
- WP-700 added `../design/ADMIN_CONTROL_PLANE_SPEC.md`, settings-backed admin-control persistence through the existing `settings` table, global API-key inventory listing, dashboard/ops read models from existing usage/account/user/realtime evidence, and `TestAdminControlPlaneV1EndpointsAndAudit` proving new admin writes require CSRF and record audit evidence.
- WP-700 intentionally stores low-frequency console-managed state as typed settings-backed collections first; high-volume transactional histories such as redeem redemption events can be promoted to dedicated Ent schemas in a later package when the product needs per-redemption concurrency semantics.
- WP-760 added `ops_system_logs` Ent/PostgreSQL persistence, operations-owned `RecordSystemLog`/filtered list/bounded cleanup/health service APIs, memory and Ent store coverage, `GET /api/v1/admin/ops/system-logs/health`, `POST /api/v1/admin/ops/system-logs/cleanup`, OpenAPI/SDK generation, and `TestSystemLogsRecordListAndCleanup` plus `TestAdminOpsSystemLogsListAndCleanup` proving dry-run cleanup, CSRF, request/trace fields, store health evidence, and audit omission of raw search strings. The old admin-control system-log contract/store duplicate has been removed so logs remain operational evidence rather than settings state.
- WP-770 added `TOTP_ENCRYPTION_KEY`, `user_totp_secrets`, TOTP memory/Ent stores, password-login 202 second-factor challenges, `/api/v1/auth/login/2fa`, `/api/v1/me/totp/*` APIs, and service/HTTP regressions proving no session cookie is issued before second-factor verification.
- WP-780 added `user_announcement_reads`, `/api/v1/me/announcements`, `/api/v1/me/announcements/{id}/read`, OpenAPI/SDK generation, and `TestUserAnnouncementsFilterVisibleAndTrackReadState` plus `TestCurrentUserAnnouncementsListAndReadState` proving audience/time-window filtering, CSRF, invisible-announcement 404, idempotent receipts, and unread reset after announcement edits.
- WP-790 added `user_redeem_code_redemptions`, `/api/v1/me/redeem-codes/redeem`, `redeem_code_credit` billing ledger evidence, balance/subscription code fulfillment, OpenAPI/SDK generation, and `TestRedeemCodeCreditsBalanceOnce`, `TestRedeemCodeCreditsBalanceOncePersistently`, and `TestCurrentUserRedeemCodeCreditsBalanceOnce` proving current-user CSRF, persistent receipt uniqueness, and no duplicate side effects on repeated redemption.
- WP-800 added optional `promo_code` to current-user payment order creation, `payment_orders` original/discount/promo fields, `user_promo_code_applications`, atomic persistent order+receipt+`used_count` updates, OpenAPI/SDK generation, and `TestCreateOrderAppliesPromoCodeBeforeCheckout` plus `TestStoreCreatesDiscountedOrderAndPromoApplicationAtomically` proving discount calculation is applied before checkout and exhausted promo retries roll back without half-created orders.
- WP-810 added `POST /api/v1/auth/register`, normalized `registration_email_suffix_allowlist`, OpenAPI/SDK generation, and `TestRegisterCreatesSessionWhenEnabled`, `TestRegisterRejectsDuplicateWithoutLeakingUserState`, `TestRegisterRespectsRegistrationSetting`, `TestRegisterRespectsEmailSuffixAllowlist`, and `TestUpdateAdminSettingsRejectsInvalidRegistrationEmailSuffixAllowlist`, proving settings-gated public registration creates a regular user session while duplicate/invalid/suffix-policy rejection input does not leak existing-user state or registration-domain policy.
- WP-820 added `ChangePassword`, session-store `DeleteByUserID`, `POST /api/v1/me/password`, OpenAPI/SDK generation, and `TestChangeCurrentUserPasswordRevokesSessionAndAllowsNewPassword`, `TestChangeCurrentUserPasswordRejectsWrongCurrentPassword`, `TestLogoutUserRevokesAllUserSessions`, and `TestDeleteByUserIDRevokesOnlyActiveUserSessions`, proving current-user password changes require the current password and revoke active console sessions without exposing secrets.
- WP-830 added `UpdateProfile`, `PATCH /api/v1/me`, OpenAPI/SDK generation, and `TestUpdateProfileOnlyChangesDisplayName` plus `TestUpdateCurrentUserProfileRequiresCSRFAndAllowlistsFields`, proving current users can update their display name while mass-assignment attempts against email, roles, status, or other protected fields are rejected.
- WP-840 added `password_reset_tokens`, OpenAPI/SDK generation, `RequestPasswordReset` / `ConfirmPasswordReset`, and `TestRequestPasswordResetStoresHashAndOutboxWithoutEnumeration`, `TestConfirmPasswordResetConsumesTokenAndRevokesSessions`, `TestPasswordResetTokensAreHashOnlyAndSingleUse`, and `TestPasswordResetRequestAndConfirmAreSingleUseAndNonEnumerating`, proving password reset uses uniform request responses, hash-only durable tokens, encrypted outbox delivery metadata, single-use consumption, password replacement, and session revocation.
- WP-850 added `email_verification_tokens`, OpenAPI/SDK generation, `RequestEmailVerification` / `ConfirmEmailVerification`, and `TestRequestEmailVerificationStoresHashAndOutboxWithoutEnumeration`, `TestConfirmEmailVerificationConsumesTokenAndMarksEmailVerified`, `TestEmailVerificationTokensAreHashOnlyAndSingleUse`, `TestVerifyEmailSetsVerifiedAt`, `TestUpdateEmailClearsVerifiedAt`, and `TestEmailVerificationRequestAndConfirmAreSingleUseAndNonEnumerating`, proving email verification uses uniform request responses, hash-only durable tokens, encrypted outbox delivery metadata, single-use consumption, `email_verified_at` marking, email-change reverification, and no implicit session creation.
- WP-860 added `internal/modules/notifications`, env-backed `EmailConfig`, SMTP delivery through the outbox worker, app wiring, `.env.example` entries, non-secret AdminSettings email metadata, and regressions for rendered auth email delivery, stale recipient hash skips, missing-config retry behavior, runtime-derived SMTP password configured state, and rejection of `smtp_password` in Admin Settings requests.
- WP-870 added `PreferenceService`, `EmailMessage.Headers`, strict SMTP optional header allowlisting, `GET`/`POST /api/v1/notifications/unsubscribe`, OpenAPI/SDK generation, and regressions proving optional unsubscribe tokens are hash-only, event-scoped, one-click compatible, and never apply to transactional auth mail.
- WP-880 added notification template catalog/rendering APIs in the notifications module, AdminNotifications OpenAPI/SDK routes for list/detail/update/restore/preview, settings-backed `<event>.subject` / `<event>.html` overrides, audit records for template writes, and service/HTTP regressions proving placeholder allowlists, preview escaping, unsafe URL blanking, CSRF, and restore behavior.
- WP-890 added current-user notification preference APIs, OpenAPI/SDK generation, `PreferenceService.ListPreferences` / `SetPreference`, and regressions proving default subscribed state, CSRF-protected updates, shared one-click unsubscribe suppression state, transactional-event rejection, and plaintext-email-free preference storage.
- WP-900 added Admin Settings low-balance notification controls, `BalanceLowTriggered` domain-event docs, balance charger threshold-crossing detection with idempotent outbox enqueue, dynamic admin notification template lookup in the outbox worker, and regressions proving hash-only recipient identity, duplicate enqueue suppression, optional unsubscribe suppression, and one-click headers.
- WP-910 added Admin Settings subscription-expiry reminder controls, `SubscriptionExpiryReminderTriggered` domain-event docs, subscription expirer reminder scanning for 7/3/1-day windows with idempotent outbox enqueue, `subscription.expiry_reminder` template/preference support, and regressions proving duplicate enqueue suppression, disabled global switch behavior, optional unsubscribe suppression, and one-click headers.
- WP-930 added settings-backed verified notification contacts, current-user contact APIs, `NotificationContactVerificationRequested`, transactional `notification.contact_verification` templates, encrypted contact verification outbox payloads, optional delivery to verified enabled contacts, and regressions proving no plaintext contact email in outbox payloads plus per-contact unsubscribe/disabled suppression.
- WP-940 added settings-backed current-user avatar storage, `PUT`/`DELETE /api/v1/me/avatar`, authenticated `GET /api/v1/users/{id}/avatar`, OpenAPI/SDK generation, and regressions proving CSRF enforcement, PNG/JPEG decode-and-reencode behavior, controlled `image/png` serving with `nosniff`, current-user avatar metadata decoration, delete-to-404 behavior, invalid file rejection, and oversized upload rejection.
- WP-950 added `user_auth_identities`, `GET /api/v1/me/auth-identities`, CSRF-protected `DELETE /api/v1/me/auth-identities/{id}`, OpenAPI/SDK generation, and users service/HTTP regressions proving current sessions can list the derived email sign-in identity, unauthenticated callers get 401, missing-CSRF unbinds get 403, external identities can be unbound by exact id, and external identities are represented without raw upstream subjects or tokens.
- WP-960 added `pending_oauth_sessions`, `PendingOAuthStore`, auth service create/consume APIs, memory and Ent persistence, incremental migration, and regressions proving pending OAuth session tokens are not stored raw, require the server secret, consume exactly once, reject expired sessions, normalize redirect paths, and retain only hashed provider subjects plus safe profile summaries.
- WP-990 added read-only `GET /api/v1/auth/oauth/pending`, non-consuming `FindPendingOAuthSession` support in auth service/memory/Ent stores, OpenAPI/SDK generation, docs/spec governance, and regressions proving pending previews do not consume tokens, hide consumed/expired sessions, reject missing cookies, return safe profile/next-step decisions, and do not expose pending tokens, raw upstream subjects, or provider-subject hashes.
- WP-1000 added CSRF-protected `POST /api/v1/auth/oauth/pending/bind-current-user`, users service/store ownership checks for external identity binding, OpenAPI/SDK generation, docs/spec governance, and regressions proving current-user pending OAuth binding rejects missing CSRF, avoids cross-user identity transfer, consumes and clears pending cookies, and returns only safe auth identity summaries.
- WP-1010 added `POST /api/v1/auth/oauth/pending/bind-login` and `/bind-login/2fa`, auth service pending-OAuth-bound 2FA challenges, OpenAPI/SDK generation, docs/spec governance, and regressions proving existing-account bind-login authenticates local credentials, keeps pending sessions unconsumed before 2FA, rejects challenges paired with another pending cookie, consumes/clears pending cookies after success, and returns normal console sessions without leaking provider subjects or pending tokens.
- A1.2 added `auth_session_cleanup` worker, `AUTH_SESSION_CLEANUP_INTERVAL_SECONDS`, Ent and memory cleanup store support, app lifecycle wiring, and docs/spec governance so expired active console sessions are marked `expired` and soft-deleted without overwriting revoked logout records.
- K1.4 quality gate hardening added `make smoke-quality-eval`, proving local QualityEval sample capture, worker evaluation, and Scheduler quality evidence in one command. Scheduler decision score evidence now preserves `quality_eval_score`, `quality_eval_samples`, and `quality_tier`; explicit zero quality scores remain valid evidence instead of falling back to the default quality score.

## Work Package Ledger

| Package | Status | Notes |
| --- | --- | --- |
| WP-000 | completed | Execution specs created. |
| WP-010 | completed | `make architecture-check` passes; architecture harness covers current boundary rules. |
| WP-020 | completed | OpenAPI lint/bundle/codegen drift checks and SDK typecheck pass. |
| WP-030 | completed | `api_key_groups.account_group_id` aligns schema, migration, Ent store, and docs; Ent/migration/full checks pass. |
| WP-040 | completed | Cookie/CSRF/API Key hardening tests pass; memory and Ent API Key stores preserve security-sensitive state consistently. |
| WP-050 | completed | Gateway Canonical AI IR and golden endpoint conversion tests pass. |
| WP-060 | completed | Canonical capability registry, descriptor validation, and Scheduler capability matching tests pass. |
| WP-070 | completed | OpenAI-compatible Adapter v1: non-streaming, SSE streaming aggregation, usage parsing, and provider error classification covered. |
| WP-080 | completed | Chat Completions, Responses, and Messages conversion works through the same OpenAI-compatible upstream account; golden and HTTP regression tests pass. |
| WP-090 | completed | MVP scheduler scenarios A/B/D/E/J/L/M/N/Q, feedback lease failure, decision evidence, and Redis concurrent lease atomics are covered. |
| WP-100 | completed | Usage, billing, audit, outbox, and Scheduler feedback evidence covered by focused service and HTTP runtime tests. |
| WP-110 | completed | Common compatible provider presets, dynamic alias route registration, and alias policy/evidence regressions pass. |
| WP-120 | completed | Runtime routing, header hygiene, account isolation, risk metrics/protection, and reverse-proxy adapter gates pass. |
| WP-130 | completed | Refresh lifecycle, audit, re-encryption path, and ban/session scheduling stop regressions pass. |
| WP-140 | completed | CLI runtime classes, CLI token materialization, model alias strategy, and session affinity Scheduler evidence are covered. |
| WP-150 | completed | Scheduler diagnostics and account health summaries include score/reject evidence, runtime class, recent error, quota, cooldown, latency, and circuit state. |
| WP-160 | superseded | Frontend visual implementation was deferred here, then delivered in full by WP-1310 (frontend rebuild + business-completeness pass). |
| WP-170 | completed | Account groups, account test/recovery, proxy binding, safe import/export, and persisted health/quota snapshots are covered. |
| WP-180 | completed | Subscription plans, user subscriptions, entitlement admission, decimal pricing rules, billing metadata linkage, admin/current-user APIs, and generated SDK/OpenAPI parity are covered. |
| WP-190 | completed | Encrypted payment providers, payment orders, signed/idempotent webhooks, fulfillment, refunds, persistence, and generated API/SDK parity are covered. |
| WP-200 | completed | Invite/rebate persistence, idempotent payment accrual, refund compensation, user-facing affiliate APIs, transfer-to-balance accounting, audit/outbox evidence, generated API/SDK parity, and migration/data-model parity are covered. |
| WP-210 | completed | Metrics baseline, release config validation, retention cleanup, backup/restore targets, release smoke script, and ops docs are covered. |
| WP-220 | completed | Anthropic-compatible upstream adapter dispatch for Messages runtime and provider aliases. |
| WP-230 | completed | Gemini native Gateway route foundation, including GenerateContent JSON/SSE routes and Google-style error rendering. |
| WP-240 | completed | Gemini native upstream adapter dispatch for API-key and reverse-proxy accounts. |
| WP-250 | completed | Provider Account upstream model discovery and supported-model candidate filtering. |
| WP-260 | completed | Durable SLO definitions, alert events, computed burn-rate evidence, admin APIs, and safe ack audit are covered. |
| WP-270 | completed | Embeddings passthrough runtime v1. |
| WP-280 | completed | HTTP runtime partition and size harness. |
| WP-290 | completed | Images generations runtime v1. |
| WP-300 | completed | Go code-quality harness. |
| WP-310 | completed | Moderations runtime v1. |
| WP-320 | completed | Rerank runtime v1. |
| WP-330 | completed | Audio transcriptions runtime v1. |
| WP-340 | completed | Audio speech runtime v1. |
| WP-350 | completed | Antigravity reverse proxy runtime identity v1. |
| WP-360 | completed | Antigravity text provider alias routes v1. |
| WP-370 | completed | Antigravity Gemini model-action alias routes v1. |
| WP-380 | completed | Responses WebSocket runtime foundation v1. |
| WP-390 | completed | Reverse Proxy WSS relay foundation v1. |
| WP-400 | completed | Codex CLI 2api Responses upstream shape v1 with OAuth/session/client-token boundary. |
| WP-410 | completed | Codex CLI 2api Responses WebSocket upstream relay v1 with OAuth/session/client-token boundary. |
| WP-420 | completed | Claude Code CLI 2api Messages upstream shape v1 with OAuth/session/client-token boundary. |
| WP-430 | completed | ChatGPT Web 2api Conversation upstream shape v1 with OAuth/session/client-token boundary. |
| WP-440 | completed | ChatGPT Web Sentinel requirements auto-fetch v1 through Reverse Proxy Runtime. |
| WP-450 | completed | Antigravity official-client v1internal upstream shape v1 through Reverse Proxy Runtime. |
| WP-460 | completed | Realtime slot lifecycle v1. |
| WP-470 | completed | OpenAI-compatible Realtime WebSocket relay v1 with OAuth/session/client-token Reverse Proxy Runtime boundary. |
| WP-480 | completed | Images edits runtime v1. |
| WP-490 | completed | Images variations runtime v1. |
| WP-500 | completed | Antigravity 2api model discovery v1. |
| WP-510 | completed | Images edits JSON references v1. |
| WP-520 | completed | Images edits streaming events v1. |
| WP-530 | completed | Antigravity project bootstrap for 2api model discovery v1. |
| WP-540 | completed | Gemini native models list v1. |
| WP-550 | completed | Gemini native countTokens v1. |
| WP-560 | completed | Anthropic Messages count_tokens v1 with Claude Code 2api runtime dispatch. |
| WP-570 | completed | AdminOps realtime active slot API v1. |
| WP-580 | completed | SDK examples and 2api migration guide v1 with examples-check harness. |
| WP-590 | completed | Distributed Redis-backed realtime slot store v1. |
| WP-600 | completed | Codex refresh-token-only import and OAuth lifecycle v1. |
| WP-610 | completed | Claude Code refresh-token-only import and OAuth lifecycle v1. |
| WP-620 | completed | Antigravity refresh-token-only import and OAuth lifecycle v1. |
| WP-630 | completed | OpenAI-compatible official API-key Realtime WebSocket relay v1. |
| WP-700 | completed | Admin Control Plane v1 docs, OpenAPI contracts, module-backed APIs, audit coverage, generated SDKs, and full gates pass. |
| WP-720 | completed | K1.4 QualityEval module, encrypted samples, worker, judge client, migration, Scheduler quality aggregation, and docs/spec governance. |
| WP-730 | completed | C3.1 Workspace schema, nullable User/APIKey workspace scope, personal workspace defaults, API Key inheritance, incremental migration, and docs/spec governance. |
| WP-740 | completed | C3.2 Role permissions, admin roles API, entitlement cache table/materialization, payment_order:read RBAC, incremental migration, and docs/spec governance. |
| WP-750 | completed | B1.2.1 usage charging pending-scan index, oldest-first batch ordering, migration, and data-model/spec governance. |
| WP-760 | completed | AdminOps durable system logs table owned by operations, health evidence, filters, bounded cleanup, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-770 | completed | Console TOTP 2FA v1 with encrypted user TOTP secrets, recovery-code hashes, login second-factor challenge, current-user TOTP APIs, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-780 | completed | Current-user announcement inbox with role/time-window visibility, per-user read receipts, CSRF mark-read API, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-790 | completed | Current-user redeem-code redemption with durable receipts, balance/subscription fulfillment, CSRF, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-800 | completed | Current-user promo-code application for payment orders with discount validation, receipt/idempotency evidence, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-810 | completed | Public console registration v1 with settings gate, normalized email suffix allowlist, generic duplicate/invalid/policy rejection errors, immediate console session, OpenAPI/SDK, and docs/spec governance. |
| WP-820 | completed | Current-user password change with current-password verification, CSRF, active session revocation, cookie clearing, audit-safe metadata, OpenAPI/SDK, and docs/spec governance. |
| WP-830 | completed | Current-user profile update with CSRF, field allowlisting, display-name update, mass-assignment guard, OpenAPI/SDK, and docs/spec governance. |
| WP-840 | completed | Public password reset with uniform request response, hash-only single-use token persistence, encrypted outbox delivery metadata, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-850 | completed | Public email verification with uniform request response, hash-only single-use token persistence, encrypted outbox delivery metadata, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-860 | completed | Notification email dispatch foundation for auth outbox events with env-backed SMTP, stale-email skip, missing-config retry, no SMTP password persistence, OpenAPI/SDK, and docs/spec governance. |
| WP-870 | completed | Notification preferences and one-click unsubscribe foundation with hash-only event-scoped tokens, public GET/POST unsubscribe APIs, strict optional mail headers, OpenAPI/SDK, and docs/spec governance. |
| WP-880 | completed | Admin notification email template management with list/detail/update/restore/preview APIs, placeholder allowlists, safe rendering, settings-backed overrides, audit, OpenAPI/SDK, and docs/spec governance. |
| WP-890 | completed | Current-user notification preferences with primary-email hash storage, CSRF updates, transactional-event rejection, shared unsubscribe state, OpenAPI/SDK, and docs/spec governance. |
| WP-900 | completed | Low-balance notification trigger with threshold-crossing outbox events, hash-only recipient identity, admin controls, dynamic templates, one-click optional email dispatch, OpenAPI/SDK, and docs/spec governance. |
| WP-910 | completed | Subscription expiry reminder trigger with 7/3/1-day idempotent outbox events, admin controls, safe subscription payloads, dynamic templates, one-click optional email dispatch, OpenAPI/SDK, and docs/spec governance. |
| WP-930 | completed | Verified notification contacts with CSRF-protected current-user APIs, encrypted verification outbox payloads, transactional contact verification templates, optional delivery fan-out to verified enabled contacts, and docs/spec governance. |
| WP-940 | completed | Current-user avatar storage with CSRF-protected upload/delete APIs, authenticated controlled image serving, SRapi-normalized PNG storage, OpenAPI/SDK, and docs/spec governance. |
| WP-950 | completed | User auth identity directory with derived local email identity, durable hash-only external identity records, exact-id external identity unbind, OpenAPI/SDK, migration, and docs/spec governance. |
| WP-960 | completed | Pending OAuth session foundation with HMAC hash-only pending tokens, hashed provider subjects, local redirect normalization, single-use consume semantics, Ent/PostgreSQL migration, and docs/spec governance. |
| WP-990 | completed | Pending OAuth decision preview with read-only pending cookie inspection, non-consuming service/store lookup, safe next-step/profile response, OpenAPI/SDK, and docs/spec governance. |
| WP-1000 | completed | Pending OAuth current-user bind with session+CSRF+pending-cookie enforcement, cross-user identity transfer rejection, pending consume/clear semantics, OpenAPI/SDK, and docs/spec governance. |
| WP-1010 | completed | Pending OAuth existing-account bind-login with email/password auth, pending-token-bound 2FA challenge, normal session issuance, pending consume/clear semantics, OpenAPI/SDK, and docs/spec governance. |
| WP-1020 | completed | Pending OAuth create-account with pending-cookie-bound action token, registration policy checks, verified-email account creation, external identity binding, pending consume/clear semantics, session issuance, OpenAPI/SDK, and docs/spec governance. |
| WP-1030 | completed | Pending OAuth email-completion with uniform send response, encrypted high-entropy outbox link, same-pending-cookie confirmation, verified email writeback, safe next-step preview, OpenAPI/SDK, and docs/spec governance. |
| WP-1040 | completed | Pending OAuth profile adoption: opt-in `adopt_display_name` on existing-account bind flows, `oauth-bind-v2` pending-cookie-bound two-factor opt-in, self-service-equivalent name validation, avatar non-adoption (SSRF/privacy) guarantee, `display_name_adopted` audit, OpenAPI/SDK, and docs/spec governance. |
| WP-1050 | completed | Upstream-account OAuth re-auth parking: shared `protectProviderAccountForClass` applied to serve-time refresh failures so permanent `session_invalid` parks `needs_reauth` (ending dead-refresh-token replay) while transient classes stay active; reuses the existing refresh runtime, no new refresh path, gateway regression, and docs/spec governance. |
| WP-1060 | completed | Outbound SSRF egress guard: dial-time `net.Dialer.Control` IP screen (loopback/RFC1918/ULA/link-local+metadata/CGNAT/multicast) on direct reverse-proxy upstream/refresh dials, mode-gated (`Server.Mode != "local"`) like cookie-Secure, DNS-rebinding-safe, proxied egress operator-trusted, no internal-address leak; reverse_proxy unit + client regressions and docs/spec governance. |
| WP-1070 | completed | Gateway request idempotency: opt-in `Idempotency-Key` on `/v1/chat/completions` + `/v1/responses` wiring the existing `IdempotencyRecord` entity (new `idempotency` module + entstore, atomic begin), `withGatewayIdempotency` wrapper with non-streaming response replay / 409 in-flight / 422 key-reuse / per-bearer scoping / streaming+key-less passthrough, OpenAPI header+409, service+gateway regressions, and docs/spec governance. |
| WP-1080 | completed | Gateway idempotency hardening: `Store.DeleteExpired` + `idempotency_cleanup` worker (app lifecycle, memory-storage-skipped) reaping `expires_at`-passed records, idempotency extended to `/v1/messages` + `/v1/embeddings` (OpenAPI header+409), worker + embeddings-replay regressions, and docs/spec governance. |
| WP-1090 | completed | Health-probe desync jitter: bounded per-account random `[0, jitter)` (default = interval) in the background probe loop so multi-account probes never burst synchronously; `RunOnce` stays jitter-free; cheap non-generative `GET /models`, api_key-only, OAuth passive preserved; egress finding documented (probe already same-client as real api_key traffic); jitter regressions and docs/spec governance. |
| WP-1100 | completed | API-key egress consistency: `reverse_proxy.ManagedEgressClient` + adapter gated `egressHTTPClient` route native api_key gateway traffic AND its health probe through the account's configured proxy/uTLS/SSRF egress (managed accounts only; plain accounts byte-identical on the shared client); reverse_proxy + adapter gating regressions and docs/spec governance. |
| WP-1110 | completed | User custom attributes (EAV): `UserAttributeDefinition`/`UserAttributeValue` Ent entities, `userattributes` module (typed validation), `entstore/userattributes`, admin CRUD for definitions + per-user value get/set; migration 000021 + full gates. |
| WP-1170 | completed | Scheduled account quota refresh: `quota_refresh` worker promotes WP-1160 from manual to automated (lists active accounts, gates on `QuotaConfigured`, decrypts + `FetchAccountQuota` + persists snapshots); `ACCOUNT_QUOTA_REFRESH_*` config (off by default); app/worker wiring + bootstrap allowlist; full gates. |
| WP-1120 | completed | Error-passthrough DB rules: `ErrorPassthroughRule` entity, `error_passthrough` module (priority-ordered expose/mask by status+class+keyword), entstore, admin CRUD, gateway `gatewayPublicMessage` override (falls back to per-account metadata when no rule matches). |
| WP-1130 | completed | TLS fingerprint DB profiles: `TLSFingerprintProfile` entity, `tls_profiles` module (validates template + http-version policy vs egress resolver set), entstore, admin CRUD, `reverse_proxy.SetNamedProfileExpander` fills only unset egress keys (account wins). |
| WP-1140 | completed | Auth CAPTCHA: config-driven `captcha` module (Turnstile/hCaptcha/reCAPTCHA siteverify), `CaptchaConfig`, login+register `verifyCaptcha` via `X-Captcha-Token`/`Cf-Turnstile-Response`; disabled by default → no-op. |
| WP-1150 | completed | Health-probe availability rollups: `AccountAvailabilityRollup` entity, `health_rollups` module (per-day UTC bucketing → availability ratio + avg success rate), entstore, `GET /admin/accounts/{id}/availability?days=N` computes/persists/returns trailing window + overall uptime. |
| WP-1160 | completed | Per-provider quota/subscription fetch scaffold: `provider_adapters.FetchAccountQuota` reuses probe plumbing, config-driven endpoint + JSON-path mappings (Codex/Antigravity/Gemini-CLI by config, Codex header signals folded in), `POST /admin/accounts/{id}/quota-fetch` decrypts credential, fetches, persists signals, returns normalized report. |
| WP-1310 | completed (slices 1+2) | Frontend business-completeness & correctness pass (`apps/web`): fixed P0 inverted account enable/disable; rebuilt the admin sidebar into 6 grouped sections so all ~22 admin pages are reachable (were 11); wired the dead per-account capabilities (clear-error/recover/discover-models/bind-proxy/export + `allSettled`+confirm bulk + a health/quota/rpm/proxy-quality detail drawer) and test→list invalidation; admin dashboard now uses `useAdminDashboard`; new `/admin/payment-providers`; redeem stats strip + active-only confirmed bulk disable; admin usage aggregate breakdowns; ops alerts `status==="firing"` + derived SLO health + SLO create/edit dialog; models pagination/filter/search; partial-refund remaining cap; strategy-replay extra inputs + richer result; pricing bulk-import dialog; api-key/order-cancel confirm+toast; error/404 i18n; `PageQueryState` disabled-query fix; `statusLabel` helper + `status` namespace applied to all badges (no raw enum tokens in the UI); added the missing data hooks; refreshed `../../docs/requirements/FRONTEND_ARCHITECTURE.md`. Group member management closed end-to-end with a new backend `GET /api/v1/admin/account-groups/{id}/accounts` endpoint (OpenAPI+handler+regenerated SDKs+HTTP regression) + a frontend member-management dialog. web typecheck + `eslint .` (0/0) + i18n-parity + `next build`, and backend `go test`/codegen-drift/sdk-typecheck all pass. Browser-verified live (login/nav/graphical-config/wired-capabilities/group-members/mobile/i18n) — which also caught + fixed two out-of-scope `user`-surface defects (user dashboard was calling admin-only 403 endpoints → slimmed to `/me/usage`-only + "Gateway" nav group made admin-only + dead admin-overview chain removed; `ResourceFormDialog` Radix a11y description warning). Fully closed. |
| WP-1300 | partial — spec only, handlers not migrated | OpenAPI formalization of the new admin surfaces: `packages/openapi/openapi.yaml` now describes all 24 admin endpoints the WP-1110..WP-1280 batch shipped as local DTOs (model-rate-limits, group-rate-limits, TLS profiles, error-passthrough rules, user-attribute defs + per-user values, account availability, account quota-fetch, config snapshot export + import), with schemas hand-verified field-for-field against the Go handler payloads. Spec-only (no handler rewire); `ConfigImportRequest` reuses `Create*Request` for natural-keyed sections + dedicated `Import{Model,Group}RateLimit`. `make openapi-lint`, Go+TS codegen + both drift checks, and `sdk-ts-typecheck` all pass — generated Go/TS types now cover these surfaces. Handlers stay local-DTO at the wire; the formal types are reference/SDK material until an optional future rewire. |
| WP-1290 | completed | OIDC id_token validation: the console-login token exchange now also returns the `id_token`, and when an issuer is configured (env `OAUTH_ISSUERS_JSON` keyed by provider_key) the callback verifies it via `go-oidc` — discovery → JWKS → RS256 signature + iss/aud(==client_id)/exp — plus a `nonce` match against the flow (replay/CSRF defense); failure rejects login. Adds the vetted `github.com/coreos/go-oidc/v3` dependency. Verified end-to-end with a fake OIDC provider test (valid / nonce-mismatch / wrong-aud / expired / empty / forged-key). When no issuer is configured, behavior is unchanged (userinfo-over-TLS auth). |
| WP-1280 | completed | ID-referencing config import (natural-key remap): config snapshot export now denormalizes the model/group natural key onto rate-limit sections (`model_name` / `account_group_name`), and `config-snapshot/import` gained `model_rate_limits` + `group_rate_limits` sections that resolve the name to the target environment's id and upsert — rows whose referenced model/group is absent in the target are `skipped` (counted, not an error). Makes rate-limit config portable across environments despite non-portable integer IDs; `dry_run` reports created/updated/skipped. |
| WP-1270 | completed | Confidential-client console OAuth: token exchange (`exchangeOAuthAuthorizationCode`) previously sent only `client_id` + `code_verifier` (PKCE/public client), so providers requiring a `client_secret` could not be used. Added `OAuthConfig.ClientSecrets` (env `OAUTH_CLIENT_SECRETS_JSON`, keyed by provider_key — secrets stay in deployment env, never AdminSettings) and `runtimeState.oauthClientSecret`; the exchange now sends `client_secret` (client_secret_post) when configured, coexisting with PKCE. id_token signature validation deferred (no JWT/JOSE lib in go.mod; hand-rolling RS256/JWKS is unsafe — needs a vetted dependency). |
| WP-1260 | completed | Per-model / per-group TPM: `tpm_limit` added to `ModelRateLimit` + `AccountGroupRateLimit` (migration 000027, default 0=unlimited), `TPMForModel`/`TPMForGroup`, admin upsert accepts it. Enforced at the same seams as the RPM checks (model at admission via `model:<id>:tpm`, group after selection via `group:<id>:tpm`) with Cost = request token count — filling the last cells of the rate/capacity matrix (model/group now have rpm + tpm + concurrency, matching account). |
| WP-1250 | completed | Config snapshot import: `POST /api/v1/admin/config-snapshot/import?dry_run=` applies the natural-keyed, portable sections of a snapshot by upsert — TLS profiles (by name), user-attribute definitions (by key), error-passthrough rules (by name): list existing, match by natural key, create-or-update; `dry_run=true` reports created/updated counts without writing. ID-referencing entities (rate limits, providers, models) stay export-only (IDs don't port). Completes the config backup/restore loop. |
| WP-1240 | completed | Config snapshot export: read-only `GET /api/v1/admin/config-snapshot` assembles one versioned JSON of operator config (providers, models, account groups, subscription plans, pricing rules, model/group rate limits, error-passthrough rules, TLS profiles, user-attribute definitions, admin settings) via a generic `snapshotSection` helper reusing the per-resource converters. Excludes account credentials + operational data (usage/audit/snapshots). Re-import deferred (dependency ordering/upsert risk). |
| WP-1230 | completed | Scheduled connectivity test runner: new `connectivity_test` worker issues a real (billable) generative probe ("Respond with OK.") via `adapter.InvokeConversation` to opt-in accounts (any runtime class, incl. OAuth — complements the api_key-only health probe which can't probe OAuth) and folds the outcome into a health snapshot through `accounts.ProbeAccount(prober, policy)`. Opt-in = a configured probe model (`test_model`/`compact_probe_model`/... in account metadata or provider config); off by default; 1h interval, bounded concurrency. `ACCOUNT_CONNECTIVITY_TEST_*` config; app/worker wiring + bootstrap allowlist; pure helpers + prober outcome unit-tested. |
| WP-1220 | completed | Per-model max concurrency: `max_concurrency` added to `ModelRateLimit` (migration 000026, default 0=unlimited), `model_rate_limits.ConcurrencyForModel`, admin upsert accepts it. `prepareProviderDispatch` now takes `modelID` (threaded from `req.Mapping.ModelID` at all 11 dispatch entry points) and acquires a `model:<id>:concurrency` lease into the dispatch lease slice (same acquire-with-rollback). Completes the limiter matrix: per-key/user/account/model/group × rpm + concurrency. |
| WP-1210 | completed | Per-account-group max concurrency: `max_concurrency` added to `AccountGroupRateLimit` (migration 000025, default 0=unlimited), `group_rate_limits.ConcurrencyForGroup`, admin upsert accepts it. Enforced in `prepareProviderDispatch`: the account concurrency lease + a `group:<id>:concurrency` lease per limited group are bundled into `providerDispatchState.concurrencyLeases` (acquire-with-rollback so none leak), released together at all dispatch sites. Concurrency helpers extracted to `runtime_gateway_group_concurrency.go`. Per-model concurrency deferred (needs modelID through the dispatch layer). |
| WP-1200 | completed | Per-account-group RPM capacity: `AccountGroupRateLimit` entity (unique account_group_id), `group_rate_limits` module + entstore, admin list/upsert/delete (`/api/v1/admin/group-rate-limits`), enforced after account selection in `reserveGatewayAccountQuota` — for the selected account's groups, a `group:<id>:rpm` check via the existing Redis limiter; exceeding triggers the same 429-class failover as per-account limits (migration 000024). Per-group concurrency deferred (reuse the existing account concurrency-lease mechanism later). |
| WP-1190 | completed | Per-model RPM rate limit: `ModelRateLimit` entity (unique model_id), `model_rate_limits` module + entstore, admin list/upsert/delete (`/api/v1/admin/model-rate-limits`), enforced at gateway admission in `checkGatewayRateLimit` via the existing Redis limiter keyed `model:<id>:rpm` (global per-model ceiling on top of per-key/user RPM); migration 000023; off unless a rule is set. Per-group scope deferred (needs post-selection enforcement). |
| WP-1180 | completed | Subscription-allowance-first billing with pay-as-you-go overage (cost-based $): opt-in per-plan `cost_quota_mode: allowance` makes `CheckEntitlement` allow-with-overage instead of denying on `monthly_cost_quota`; `UsageLog.billable_cost` (migration 000022, backfilled = cost) carries the post-allowance overage computed once in `recordGatewayUsage` via `CostAllowance` + pure `BillableOverage`; `balance_charger`/billing entstore bill `billable_cost`. Default `hard_cap` = unchanged. Split unit-tested; full `make check` green. |
| WP-500+ | completed | Ecosystem and advanced endpoint packages — enumerated individually above as completed WP-500..WP-1310 entries. Genuinely remaining/roadmap items (e.g. Batch / Fine-tuning APIs, full JA3/JA4 + HTTP/2 fingerprinting, affiliate withdrawal) are tracked in `ROADMAP.md` and the relevant `docs/`. |
