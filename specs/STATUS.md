# SRapi Goal Status

## Current Snapshot

status_version: 1
updated_at: 2026-05-22

last_completed:

- WP-000: Codex execution specs added under `specs/`, with final state, roadmap, work packages, gates, and reference-project decisions.
- WP-010: architecture baseline harness verified with `make architecture-check`.
- WP-020: OpenAPI lint, bundle, Go codegen drift check, TypeScript SDK drift check, and SDK typecheck verified.
- WP-030: Ent schema, PostgreSQL initial migration, data model docs, and API Key group persistence aligned around `account_group_id`; Ent, migration, persistence, and full checks passed.
- WP-040: console session cookie/CSRF behavior, API Key HMAC-only storage, disabled/expired key rejection, and secret-free API key responses/audit covered by tests.
- WP-050: Gateway module extraction now owns Canonical AI request/response normalization and has golden conversion tests for Chat Completions, Responses, and Anthropic Messages.
- WP-060: capability taxonomy alignment uses canonical descriptor keys, validates unknown/misspelled capability keys, and feeds Scheduler hard filters with RequestCapability versus EffectiveCapability.
- WP-070: OpenAI-compatible Adapter v1 dispatches non-streaming and SSE requests, parses usage, classifies upstream errors, and protects credentials.
- WP-080: Responses and Messages compatibility is covered by golden conversion tests plus a real mock upstream regression proving Chat Completions, Responses, and Messages target the same OpenAI-compatible Provider Account.
- WP-090: Scheduler v1 hardening now has an explicit MVP scenario matrix harness, failed feedback lease-state coverage, Redis atomic concurrent lease coverage, and passing Scheduler gates.
- WP-100: Gateway evidence closure now has focused usage, billing, audit, outbox, and scheduler feedback tests proving successful and failed requests produce durable operational evidence.
- WP-110: Provider preset registry expansion now seeds common OpenAI-compatible and Anthropic-compatible presets, dynamically registers provider alias routes from preset metadata, and verifies alias routes preserve model/API key policy and scheduler evidence.
- WP-120: Reverse Proxy Runtime foundation routes non-API-key accounts through the runtime, strips SRapi/gateway headers, isolates account cookie jars, maps runtime risk classes to metrics/account protection, and validates OAuth refresh persistence through HTTP regressions.
- WP-130: OAuth refresh and token lifecycle now verifies per-account refresh behavior, refresh failure audit without credential overwrite, refresh success credential re-encryption/audit, and ban/session signals stopping future scheduling.
- WP-140: CLI runtime concepts are represented as durable account runtime classes and adapter/runtime inputs; model alias strategy and session affinity now feed Scheduler decisions with hashed affinity evidence.
- WP-150: Admin diagnostics now expose Scheduler overview/decision evidence and account health summaries with runtime class, recent error class, quota, cooldown, latency, and circuit state.
- WP-170: Account operations parity now covers account groups, safe inspect/export/import, proxy binding, recovery, persisted health/quota snapshots, CSRF coverage, and generated SDK/OpenAPI parity.
- WP-180: Subscription and pricing foundations now include subscription plans, user subscriptions, decimal-safe pricing rules, Gateway entitlement/pricing admission, billing metadata linkage, admin/current-user control-plane APIs, and generated SDK/OpenAPI parity.
- WP-190: Payment order foundations now include encrypted provider instances, current-user order APIs, signed/idempotent webhooks, refund hooks, fulfillment into billing/subscription state, audit/outbox evidence, Ent/Postgres persistence, and generated SDK/OpenAPI parity.
- WP-200: Affiliate rebate Phase 2 now includes invite codes, invite relationships, affiliate rules, idempotent payment-paid accrual, refund compensation ledgers, transfer-to-balance accounting, audit/outbox evidence, Ent/Postgres persistence, and migration/data-model parity.

current:

- package: WP-210
- status: pending
- objective: production operations.

next_recommended: WP-210

last_gates:

- `cd apps/api && go test ./internal/modules/payments/... ./internal/modules/affiliate/... ./internal/persistence/entstore/affiliate ./internal/workers/outbox`: pass
- `make ent-generate-check`: pass
- `make migration-check`: pass
- `cd apps/api && go test ./...`: pass
- `make architecture-check`: pass
- `make secret-scan`: pass
- `git diff --check`: pass

notes:

- Existing `docs/` remains the architecture and domain source of truth.
- Future goal runs must read `specs/README.md` first, then continue from `next_recommended`.
- The repository currently has unrelated dirty worktree entries; Codex must preserve them.
- Frontend visual implementation is intentionally deferred per user instruction.
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
- WP-160 is deferred by explicit user instruction: frontend visual implementation will be handled later by Gemini. Continue backend work at WP-170.
- WP-170 added account group operations, account inspect/export/import, proxy bind, recover, persisted test/gateway health and quota snapshots, recursive export metadata sanitization, expanded CSRF regression coverage, and generated SDK methods for account operations.
- WP-180 added `GET /api/v1/me/subscriptions`, admin subscription plan/user subscription/pricing rule APIs, entitlement rejection before Scheduler lease consumption, decimal-normalized pricing rule responses, pricing metadata on billing ledger entries, generated SDK methods, Ent/migration parity, and CSRF coverage for new console writes.
- WP-190 added current-user and admin payment APIs, encrypted payment provider config, legal order state transitions, signed/idempotent webhook handling, fulfillment-side billing/subscription/audit/outbox effects, refund hooks, Ent/Postgres persistence, migration drift coverage, and generated SDK/OpenAPI parity.
- WP-200 added the affiliate module, Ent schemas and PostgreSQL tables for invite/affiliate ledgers, payment outbox dispatch into affiliate accrual/compensation, refund compensation capping, and transfer-to-balance tests proving affiliate ledger, billing ledger, user balance, and audit evidence stay aligned.
- WP-200 did not add frontend visuals per explicit user instruction. OpenAPI user/admin affiliate routes remain a later control-plane surface because this package closed the backend accounting and event path first.

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
| WP-160 | deferred | Frontend visual implementation intentionally deferred per user instruction; backend work continues. |
| WP-170 | completed | Account groups, account test/recovery, proxy binding, safe import/export, and persisted health/quota snapshots are covered. |
| WP-180 | completed | Subscription plans, user subscriptions, entitlement admission, decimal pricing rules, billing metadata linkage, admin/current-user APIs, and generated SDK/OpenAPI parity are covered. |
| WP-190 | completed | Encrypted payment providers, payment orders, signed/idempotent webhooks, fulfillment, refunds, persistence, and generated API/SDK parity are covered. |
| WP-200 | completed | Invite/rebate persistence, idempotent payment accrual, refund compensation, transfer-to-balance accounting, audit/outbox evidence, and migration/data-model parity are covered. |
| WP-210 | pending | Production operations. |
| WP-220+ | pending | Advanced endpoint and provider expansion. |
