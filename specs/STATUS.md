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
- WP-210: Production operations now includes baseline Prometheus `/metrics`, release-mode weak secret/default admin password rejection, data retention cleanup worker, PostgreSQL backup/restore targets, release smoke script coverage, and deployment/config docs.
- WP-220: Anthropic-compatible upstream adapter now dispatches Messages payloads to `/messages`, parses non-streaming and SSE usage, classifies Anthropic error objects, preserves reverse-proxy runtime dispatch, and proves provider aliases record Scheduler/usage evidence.
- WP-230: Gemini-native Gateway route foundation now exposes GenerateContent and StreamGenerateContent routes, converts Gemini requests to Canonical AI Request, renders Gemini JSON/SSE responses, returns Google-style errors, and proves Gateway auth/model policy/Scheduler/usage evidence.
- WP-240: Gemini-compatible/native-gemini upstream adapter now dispatches GenerateContent and StreamGenerateContent payloads to Gemini APIs, parses usage metadata, classifies Google errors, preserves reverse-proxy Gemini runtime dispatch, and proves Gateway Gemini routes schedule Gemini-compatible upstream accounts.
- WP-250: Provider Account upstream model discovery now exposes `POST /api/v1/admin/accounts/{id}/discover-models`, persists discovered `supported_models` metadata, and feeds provider-neutral candidate filtering without leaking credentials.
- WP-260: Ops SLO and alert control plane v1 now includes durable SLO definitions, alert events, computed availability/burn-rate evidence from usage logs, CSRF-protected admin APIs, safe audit records, Ent/Postgres persistence, and generated OpenAPI/SDK parity.
- WP-270: Embeddings passthrough runtime v1 now exposes OpenAI-compatible `/v1/embeddings`, provider alias routing, canonical embeddings normalization/rendering, OpenAI-compatible upstream `/embeddings` adapter dispatch, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-280: HTTP runtime partitioning split the 7750-line catch-all runtime into route-family files and added an architecture harness that keeps `runtime_http.go` thin and caps `runtime_*.go` file size.
- WP-290: Images generations runtime v1 now exposes OpenAI-compatible `/v1/images/generations`, provider alias routing, canonical image normalization/rendering, OpenAI-compatible upstream `/images/generations` adapter dispatch, explicit `images` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-300: Go code-quality harness now adds `make code-quality-check`, runs it from `make check`, and enforces gofmt drift, `go vet ./...`, production file-size, and production function-size thresholds.
- WP-310: Moderations runtime v1 now exposes OpenAI-compatible `/v1/moderations`, provider alias routing, canonical moderation normalization/rendering, OpenAI-compatible upstream `/moderations` adapter dispatch, explicit `moderations` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-320: Rerank runtime v1 now exposes `/v1/rerank`, rerank-compatible provider alias routing, canonical rerank normalization/rendering, rerank-compatible upstream `/rerank` adapter dispatch, explicit `rerank` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-330: Audio transcriptions runtime v1 now exposes OpenAI-compatible `/v1/audio/transcriptions`, provider alias routing, canonical audio transcription normalization/rendering, OpenAI-compatible upstream multipart `/audio/transcriptions` adapter dispatch, explicit `audio_transcriptions` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-340: Audio speech runtime v1 now exposes OpenAI-compatible `/v1/audio/speech`, provider alias routing, canonical audio speech normalization, binary audio rendering, OpenAI-compatible upstream JSON `/audio/speech` adapter dispatch, explicit `audio_speech` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-350: Antigravity reverse proxy runtime identity now adds `reverse-proxy-antigravity` to the OpenAPI/SDK adapter enum, default `antigravity_desktop` Reverse Proxy Runtime identity, OpenAI/Gemini protocol dispatch tests, and a Gateway regression proving desktop-token Antigravity accounts route through Scheduler and Reverse Proxy Runtime.
- WP-360: Antigravity text provider aliases now seed an `antigravity` provider preset, register text aliases from capability metadata, expose representative OpenAPI/SDK alias contracts, and prove OpenAI Chat/Anthropic Messages alias requests still route through Scheduler, Provider Adapter, and Reverse Proxy Runtime without Gateway-local DTOs.
- WP-370: Antigravity Gemini model-action aliases now route `/antigravity/v1beta` and `/api/provider/antigravity/v1beta` GenerateContent/StreamGenerateContent requests through the standard Gemini Gateway handler while forcing `provider_key=antigravity`, preserving alias evidence, and avoiding Gateway-local DTOs.
- WP-380: Responses WebSocket runtime foundation now exposes `GET /v1/responses/ws`, accepts raw `ResponsesRequest` or `response.create` frames, executes each request through the existing Responses Gateway runtime, forwards streaming Responses events as WebSocket JSON frames, and preserves Scheduler/usage source endpoint evidence.
- WP-390: Reverse Proxy Runtime now exposes a `WebSocketRuntime.RelayWebSocket` primitive for direct upstream WSS relay with per-account client/proxy/cookie context, credential-driven auth injection, forbidden-header hygiene, text/binary message relay, and relay accounting.
- WP-400: Codex CLI 2api HTTP text path now sends `reverse-proxy-codex-cli` requests through Reverse Proxy Runtime to Codex `/responses` official-client shape, parses Codex Responses SSE/JSON output, keeps generic OpenAI-compatible reverse proxy on `/chat/completions`, and enforces the OAuth/session/client-token credential boundary instead of treating API keys as 2api identity.

current:

- package: WP-410+
- status: pending
- objective: split the next ecosystem or remaining advanced endpoint package from the roadmap.

next_recommended: WP-410+

last_gates:

- `make openapi-lint`: pass
- `make openapi-bundle`: pass
- `make openapi-codegen-check`: pass
- `make openapi-ts-codegen-check`: pass
- `make sdk-ts-typecheck`: pass
- `make ent-generate-check`: pass
- `make migration-check`: pass
- `cd apps/api && go test ./internal/httpserver ./internal/architecture`: pass
- `cd apps/api && go test ./...`: pass
- `make architecture-check`: pass
- `make code-quality-check`: pass
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
- WP-210 added `/metrics` baseline samples from usage logs, scheduler decisions/leases, and reverse-proxy runtime signals; release smoke checks health/readiness/metrics plus mock Gateway flow.
- WP-210 added retention cleanup for usage logs, scheduler decisions/feedbacks, audit logs, and account health snapshots; financial ledgers, payment records, affiliate ledgers, credentials, and user state remain excluded from automatic cleanup.
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
- WP-300 calibrated code-quality thresholds to current production code: non-generated production Go files must stay at or below 2200 lines, and non-generated production functions must stay at or below 220 lines.
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
- WP-360 added the `antigravity` preset with `/antigravity/v1`, `/api/provider/antigravity`, and `/api/provider/antigravity/v1` text aliases plus desktop/IDE reverse-proxy account allowlist.
- WP-360 changed provider alias registration to read endpoint capabilities from preset metadata; the registry test now guards OpenAI image/audio capabilities so dynamic alias coverage does not regress.
- WP-360 added `TestGatewayAntigravityProviderAliasTargetsOpenAIReverseProxy` and `TestGatewayAntigravityProviderAliasTargetsAnthropicReverseProxy`, proving Antigravity aliases force `provider_key=antigravity`, preserve alias source endpoints, and dispatch through Reverse Proxy Runtime using `provider.protocol`.
- WP-370 added Antigravity Gemini model-action alias metadata to the provider preset registry and HTTP registration for `/antigravity/v1beta/models/{model}:generateContent`, `/antigravity/v1beta/models/{model}:streamGenerateContent`, `/api/provider/antigravity/v1beta/models/{model}:generateContent`, and `/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent`.
- WP-370 added `TestGatewayAntigravityGeminiAliasTargetsReverseProxy` and `TestGatewayAntigravityGeminiStreamAliasTargetsReverseProxy`, proving alias source endpoints, forced provider context, mapped upstream model dispatch, Gemini JSON/SSE rendering, and usage/Scheduler evidence.
- WP-380 added `TestGatewayResponsesWebSocketTargetsResponsesRuntime` and `TestGatewayResponsesWebSocketForwardsStreamingEvents`, proving WebSocket non-streaming and streaming requests reuse `/v1/responses` auth/model policy/Scheduler/Provider Adapter/usage paths and preserve `/v1/responses/ws` as the source endpoint.
- WP-380 added `nhooyr.io/websocket` only for transport handshake/frame handling; business protocol stays in the existing Responses runtime and direct upstream WSS relay remains a follow-up package.
- Local client availability is confirmed in PATH for `codex`, `claude`, and `antigravity`. Codex CLI and Claude Code CLI reverse-proxy runtime identities already exist (`reverse-proxy-codex-cli`, `reverse-proxy-claude-code-cli`) and Antigravity alias routing now covers OpenAI-compatible, Anthropic-compatible, and Gemini-compatible routes; actual reverse-proxy use still requires configured Provider/Account/base_url/token records.
- WP-390 added `TestRuntimeRelaysWebSocketWithAccountContextAndHeaderHygiene` and `TestRuntimeRelaysWebSocketWebSessionCookieFromCredential`, proving direct upstream WebSocket relay uses account credentials, default upstream-client User-Agent, subprotocol negotiation, text/binary message relay, metrics, and sanitized handshake headers.
- WP-390 intentionally does not add Gateway route binding or provider-native realtime schema adapters; those are next-stage realtime packages that can now depend on the runtime primitive.
- WP-400 added `TestReverseProxyCodexCLIAdapterUsesResponsesOfficialClientShape`, proving Codex 2api sends `base_url + "/responses"` with selected account token, Codex official-client headers, Responses-style body, and Codex SSE parsing, without adding Gateway-local Codex DTOs.
- WP-400 also added `TestRuntimeDoesNotInjectAPIKeyRuntimeCredentials` and `TestReverseProxyCodexCLIRejectsAPIKeyRuntime`, so SRapi's 2api reverse-proxy boundary stays aligned with OAuth/session/client-token accounts rather than official API-key adapters.

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
| WP-410+ | pending | Remaining ecosystem and advanced endpoint packages. |
