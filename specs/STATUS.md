# SRapi Goal Status

## Current Snapshot

status_version: 1
updated_at: 2026-05-23

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
- WP-410: Codex CLI 2api Responses WebSocket relay now lets explicitly requested `/v1/responses/ws` calls schedule an eligible `reverse-proxy-codex-cli` account, derive Codex `wss://.../responses`, send Codex official-client headers and a `response.create` upstream-model frame through Reverse Proxy Runtime, and record Scheduler/usage evidence from upstream WebSocket frames.
- WP-420: Claude Code CLI 2api HTTP Messages path now sends `reverse-proxy-claude-code-cli` requests through Reverse Proxy Runtime to `/messages?beta=true`, builds Claude Code beta/version/stainless/session headers plus system/billing blocks, and enforces the OAuth/session/client-token credential boundary instead of treating API keys as 2api identity.
- WP-430: ChatGPT Web 2api HTTP Conversation path now sends `reverse-proxy-chatgpt-web` requests through Reverse Proxy Runtime to `/backend-api/conversation`, builds browser/OAI/Sentinel headers plus ChatGPT Web Conversation body, and enforces the OAuth/session/client-token credential boundary instead of treating API keys as 2api identity.
- WP-440: ChatGPT Web 2api now auto-fetches Sentinel chat requirements through Reverse Proxy Runtime when a static requirements token is absent, including homepage bootstrap, legacy requirements `p` generation, optional PoW proof token generation, and Arkose/Turnstile challenge classification.
- WP-450: Antigravity 2api HTTP text path now sends `reverse-proxy-antigravity` requests through Reverse Proxy Runtime to Google Cloud Code `/v1internal:generateContent` or `/v1internal:streamGenerateContent?alt=sse`, builds the Antigravity `project`/`requestId`/`userAgent`/`requestType` envelope with nested Gemini request payload, and enforces the desktop/IDE/OAuth credential boundary instead of treating API keys as 2api identity.
- WP-460: Realtime slot lifecycle v1 now adds a provider-neutral realtime module, acquires `/v1/responses/ws` slots after Gateway auth and before WebSocket upgrade, releases slots on close/error, enforces deploy-level global/per-API-key slot limits, and exposes realtime slot metrics without storing raw affinity keys or provider DTOs.
- WP-470: OpenAI-compatible Realtime WebSocket relay v1 now exposes `GET /v1/realtime`, schedules accounts with `realtime_websocket` capability, derives upstream `/realtime?model=<mapped_upstream_model>`, and relays frames through Reverse Proxy Runtime using selected OAuth/session/client-token credentials rather than caller headers or Gateway-local DTOs.
- WP-480: Images edits runtime v1 now exposes OpenAI-compatible multipart `POST /v1/images/edits`, provider alias routing, canonical image edit normalization/rendering, OpenAI-compatible upstream `/images/edits` multipart adapter dispatch, explicit `images` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-490: Images variations runtime v1 now exposes OpenAI-compatible multipart `POST /v1/images/variations`, provider alias routing, canonical image variation normalization/rendering, OpenAI-compatible upstream `/images/variations` multipart adapter dispatch, explicit `images` Scheduler capability filtering, usage/billing/Scheduler feedback evidence, and generated OpenAPI/SDK parity.
- WP-500: Antigravity 2api model discovery now lets `reverse-proxy-antigravity` accounts POST `{base_url}/v1internal:fetchAvailableModels` through Reverse Proxy Runtime, parses model catalogs, persists `supported_models` when requested, and keeps the discovery endpoint credential-free in responses.
- WP-510: Images edits JSON references now let `/v1/images/edits` and OpenAI-compatible image edit aliases accept application/json local data URL / base64 image references, decode them into the existing canonical image edit path, forward upstream multipart `/images/edits`, and explicitly reject remote URLs and `file_id` references.
- WP-520: Images edits streaming events now let `/v1/images/edits` and OpenAI-compatible image edit aliases return `text/event-stream` when `stream=true`, synthesize a final `image.generation.result` SSE chunk through the existing Gateway/auth/Scheduler/Provider Adapter/usage path, and keep remote URL / `file_id` rejection unchanged.
- WP-530: Antigravity project bootstrap now lets `reverse-proxy-antigravity` discovery use selected-account credentials through Reverse Proxy Runtime to call `/v1internal:loadCodeAssist` and, when needed, `/v1internal:onboardUser` before model discovery; preview remains side-effect free and persisted discovery writes resolved project metadata.
- WP-540: Gemini native models list now exposes `GET /v1beta/models`, authenticates Gateway API keys, filters active SRapi model registry entries by API-key visibility, renders Google-shaped `models.list` responses with pagination and supported generation methods, and does not acquire Scheduler leases or touch Provider Account credentials.
- WP-550: Gemini native countTokens now exposes `POST /v1beta/models/{model}:countTokens`, advertises `countTokens` in Gemini `models.list` when the SRapi model has `token_counting`, accepts Gemini countTokens body or `generateContentRequest`, schedules only `token_counting` capable Gemini accounts, dispatches upstream `models/{mapped_model}:countTokens` through API-key or Reverse Proxy Runtime credentials, and records Scheduler/request evidence without generation usage or cost.
- WP-560: Anthropic Messages count_tokens now exposes `POST /v1/messages/count_tokens` plus anthropic-compatible provider alias, accepts Anthropic count_tokens body shape, schedules only `token_counting` capable accounts, dispatches upstream `/messages/count_tokens` with mapped upstream model for API-key accounts, dispatches Claude Code 2api `/messages/count_tokens?beta=true` through Reverse Proxy Runtime with selected OAuth/CLI credentials, and records Scheduler/request evidence without generation usage or cost.
- WP-570: AdminOps realtime active slot API now exposes `GET /api/v1/admin/ops/realtime/slots`, returning current-node active slot summaries and endpoint/kind/API-key aggregate counters without raw affinity keys, credentials, prompts, provider frames, local client ingress, or Gateway-local provider DTOs.
- WP-580: SDK examples and 2api migration guide v1 now provide curl, TypeScript SDK, and Python requests examples for OpenAI/Anthropic/Gemini-compatible Gateway routes plus AdminOps realtime slot listing, document migration from sub2api / CLIProxyAPI / chatgpt2api style deployments into SRapi Provider Account / Scheduler / Reverse Proxy Runtime boundaries, and add `make examples-check`.
- WP-590: Distributed realtime slot store v1 now adds a realtime Store contract, in-memory and Redis-backed implementations, app/httpserver wiring that uses Redis in release-capable deployments, distributed slot limit/listing tests, and docs for cross-node AdminOps semantics without provider-native DTOs or local client ingress.
- WP-600: Codex refresh-token-only import and OAuth lifecycle now lets admin create/import/update accept a Codex OAuth `refresh_token` without an initial `access_token`, exchange it through Reverse Proxy Runtime using the Codex CLI OAuth token shape, persist encrypted access-token state, and immediately dispatch Gateway `/v1/responses` to Codex `/responses` with selected-account OAuth identity and no token leakage in control-plane responses.

current:

- package: WP-610
- status: pending
- objective: extend refresh-token-only import and OAuth lifecycle support to Claude Code 2api Provider Accounts.

next_recommended: WP-610

last_gates:

- `cd apps/api && go test ./internal/modules/accounts/... ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/httpserver -run 'Test.*Codex.*Refresh|Test.*Codex.*Import|TestGateway.*Codex' -count=1`: pass
- `cd apps/api && go test ./...`: pass
- `make architecture-check`: pass
- `make code-quality-check`: pass
- `make examples-check`: pass
- `make secret-scan`: pass
- `git diff --check`: pass

notes:

- Existing `docs/` remains the architecture and domain source of truth.
- Future goal runs must read `specs/README.md` first, then continue from `next_recommended`.
- Future goal runs must preserve unrelated user worktree changes if present.
- Frontend visual implementation is intentionally deferred per user instruction.
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
- WP-410 added `TestReverseProxyCodexCLIPrepareRealtimeBuildsResponsesWebSocketSession`, `TestReverseProxyCodexCLIPrepareRealtimeRejectsAPIKeyRuntime`, and `TestGatewayResponsesWebSocketRelaysCodexUpstreamWebSocket`, proving Codex Responses WebSocket 2api uses selected account OAuth/CLI token credentials, Codex official-client headers, mapped upstream model frames, and `/v1/responses/ws` Scheduler/usage evidence.
- WP-410 intentionally keeps WebSocket relay opt-in via `upstream_ws` / `codex_responses_websocket` flags and account metadata; persistent Codex session reuse, local Codex CLI ingress, and Claude/Antigravity WebSocket adapters remain follow-up work.
- WP-420 added `TestReverseProxyClaudeCodeCLIAdapterUsesOfficialClientMessagesShape`, `TestReverseProxyClaudeCodeCLIRejectsAPIKeyRuntime`, and `TestGatewayClaudeCodeReverseProxyUsesOfficialClientMessagesShape`, proving Claude Code 2api uses selected account OAuth/CLI token credentials, Claude Code official-client headers/body, `/messages?beta=true`, and Scheduler/usage evidence without Gateway-local DTOs.
- WP-420 refreshed `docs/2API_REVERSE_PROXY_DEFINITION.md` to define SRapi 2api/反代 from the local reference projects `sub2api`, `CLIProxyAPI`, and `chatgpt2api`: upstream official-client request simulation with OAuth/session/client-token identity, not local Codex/Claude/Antigravity client ingress.
- WP-430 added `TestReverseProxyChatGPTWebAdapterUsesConversationOfficialClientShape`, `TestReverseProxyChatGPTWebRejectsAPIKeyRuntime`, and `TestGatewayChatGPTWebReverseProxyUsesConversationOfficialClientShape`, proving ChatGPT Web 2api uses selected account OAuth/session credentials, browser/OAI/Sentinel headers, `/backend-api/conversation`, ChatGPT Web Conversation body, and Scheduler/usage evidence without Gateway-local DTOs.
- WP-440 added `TestReverseProxyChatGPTWebAdapterAutoFetchesRequirements`, `TestReverseProxyChatGPTWebMissingRequirementsCanDisableAutoFetch`, and updated `TestGatewayChatGPTWebReverseProxyUsesConversationOfficialClientShape`, proving missing static requirements tokens trigger selected-account bootstrap and `/backend-api/sentinel/chat-requirements` before conversation without Gateway-local DTOs.
- WP-440 intentionally does not implement external Arkose/Turnstile solving, challenge token persistence, or browser TLS impersonation.
- WP-450 added Antigravity official-client upstream shape coverage across OpenAI-compatible, Anthropic-compatible, Gemini-compatible adapter inputs plus `TestReverseProxyAntigravityRejectsAPIKeyRuntime` and an updated Gateway regression proving `/v1/chat/completions` schedules an Antigravity desktop account and sends `/v1internal:generateContent` with selected account bearer credentials.
- WP-450 intentionally does not implement Antigravity OAuth onboarding, project discovery, credit overage retry policy, full tool-schema cleaning, or persistent realtime session lifecycle.
- WP-460 added `TestAcquireReleaseTracksRealtimeSlotLifecycle`, global/per-API-key slot-limit tests, and `TestGatewayResponsesWebSocketEnforcesRealtimeSlotLimit`, proving raw session affinity keys are hashed and excess WebSocket handshakes fail with 429 before upgrade.
- WP-460 intentionally does not add Claude Code or Antigravity provider-native realtime adapters, persistent upstream session reuse, or distributed Redis-backed slot storage.
- WP-470 added `TestNormalizeRealtimeWebSocketRequiresRealtimeCapability`, `TestOpenAICompatiblePrepareRealtimeBuildsRealtimeWebSocketSession`, `TestOpenAICompatiblePrepareRealtimeRejectsAPIKeyRuntime`, and `TestGatewayRealtimeWebSocketRelaysOpenAIUpstreamWebSocket`, proving `/v1/realtime` uses selected account OAuth identity, mapped upstream model query, allowed Realtime headers, realtime slot lifecycle, Scheduler decisions, and usage evidence.
- WP-470 intentionally does not add official API-key Realtime, persistent upstream session pools, local client ingress, or Claude Code / Antigravity provider-native realtime adapters.
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
- WP-580 added `examples/README.md`, curl / TypeScript / Python examples, `docs/MIGRATION_GUIDE_2API.md`, and `tools/examples-check.mjs`; the harness checks required routes, env vars, TypeScript SDK compile compatibility, 2api boundary phrases, README/docs links, and secret-like placeholders.
- WP-580 intentionally does not add frontend visuals, local Codex / Claude Code / Antigravity ingress, Gateway-local provider DTOs, or runnable upstream credential import automation.
- WP-590 added `TestRedisRealtimeStoreEnforcesDistributedSlotLimits`, `TestRedisRealtimeStoreReleaseFromAnotherInstanceFreesCapacity`, `TestRedisRealtimeStoreExpiresSlotsWithoutLeakingSensitiveData`, and app wiring tests proving Redis-backed realtime slots are used when available, required in release mode, and safe to inspect from AdminOps.
- WP-590 intentionally does not add provider-native Claude Code / Antigravity realtime adapters, persistent upstream session pools, local Codex / Claude Code / Antigravity ingress, or Codex refresh-token-only import automation.
- WP-600 added `TestRuntimeRefreshUsesPerAccountLockAndDoesNotOverwriteOnFailure`, `TestRuntimeRefreshClassifiesInvalidGrantWithoutOverwritingCredential`, `TestGatewayCodexRefreshTokenOnlyCreateCanRequestResponses`, `TestAdminAccountImportCodexRefreshTokenOnlyExchangesTokenWithoutLeakingCredential`, and `TestGatewayCodexRefreshTokenOnlyUpdateCanRequestResponses`, proving Codex refresh-token-only create/import/update exchanges and persists usable OAuth state before Gateway dispatch.
- WP-600 intentionally does not add local Codex CLI ingress, Gateway-local Codex DTOs, Claude Code refresh-token-only import, or Antigravity refresh-token-only import.

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
| WP-610 | pending | Claude Code refresh-token-only import and OAuth lifecycle v1. |
| WP-500+ | pending | Remaining ecosystem and advanced endpoint packages. |
