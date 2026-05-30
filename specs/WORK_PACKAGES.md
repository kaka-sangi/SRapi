# SRapi Work Packages

## Package Format

Each work package contains:

- Objective
- Read first
- Owns
- Definition of Done
- Required gates

Codex should execute one package at a time.

## WP-000: Codex Execution Specs

Objective: create the execution layer that lets Codex continue SRapi through goals.

Read first:

- `docs/README.md`
- `README.md`

Owns:

- `specs/*`
- README/docs links to `specs/`

Definition of Done:

- `specs/README.md`, `GOAL_EXECUTION_PROTOCOL.md`, `FINAL_STATE.md`, `ROADMAP.md`, `WORK_PACKAGES.md`, `QUALITY_GATES.md`, `REFERENCE_PROJECT_DECISIONS.md`, and `STATUS.md` exist.
- README and docs index mention `specs/`.
- `STATUS.md` points to the next implementation package.

Required gates:

- Markdown files are readable and internally linked.
- No code generation required.

## WP-010: Architecture Baseline Audit

Objective: audit the current implementation against the modular monolith rules and create small fixes where boundaries already drift.

Read first:

- `docs/ARCHITECTURE.md`
- `docs/ARCHITECTURE_REQUIREMENTS.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `specs/QUALITY_GATES.md`

Owns:

- `apps/api/internal/architecture`
- `apps/api/internal/app`
- module import boundary tests
- docs updates if boundary rules change

Definition of Done:

- Architecture tests describe allowed and forbidden dependencies.
- HTTP layer does not directly depend on Ent.
- Workers do not depend on handlers.
- Persistence implementations remain under `internal/persistence`.

Required gates:

- `make architecture-check`
- `cd apps/api && go test ./internal/architecture ./internal/app ./internal/httpserver`

## WP-020: OpenAPI Contract Split And Drift Discipline

Objective: make OpenAPI maintainable as the route surface grows.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `packages/openapi/openapi.yaml`

Owns:

- `packages/openapi`
- `apps/api/internal/openapi` generated code only through generation
- `packages/sdk/typescript` generated SDK only through generation

Definition of Done:

- OpenAPI contract is split or structured enough for long-term maintenance.
- Gateway, admin, user, scheduler, usage, audit, and ops operations have stable tags and operation IDs.
- Generated Go and TypeScript artifacts are in sync.
- Security schemes match cookie/CSRF and gateway bearer rules.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`

## WP-030: Data Model And Migration Parity

Objective: align Ent schemas, PostgreSQL migrations, and data model docs.

Read first:

- `docs/DATA_MODEL.md`
- `docs/DOMAIN_MODEL.md`
- `docs/SECURITY_MODEL.md`
- `docs/DOMAIN_EVENTS_SPEC.md`

Owns:

- `apps/api/ent/schema`
- `apps/api/migrations`
- `apps/api/internal/persistence/entstore`
- `docs/DATA_MODEL.md` when behavior changes

Definition of Done:

- MVP tables match docs.
- Sensitive fields use hash/encryption/sensitive markers as applicable.
- Initial migration applies to an empty database.
- Down migration covers all created objects.
- Repository integration tests cover key lookup paths.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `cd apps/api && go test ./internal/persistence/entstore/... ./internal/platform/db`

## WP-040: Auth, Session, CSRF, API Key Hardening

Objective: make console auth and Gateway API key auth production-safe.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/MVP_SPEC.md`

Owns:

- `apps/api/internal/modules/auth`
- `apps/api/internal/modules/users`
- `apps/api/internal/modules/api_keys`
- HTTP middleware for cookie, CSRF, and bearer auth
- related OpenAPI schemas

Definition of Done:

- Login uses HttpOnly session cookie.
- Write APIs require CSRF.
- API keys are returned once, stored only as HMAC hash plus prefix.
- Key disabled/expired/model-scope failures are tested.
- Logs and audit never include plaintext API keys.

Required gates:

- `cd apps/api && go test ./internal/modules/auth/... ./internal/modules/users/... ./internal/modules/api_keys/... ./internal/httpserver`
- OpenAPI codegen checks if contract changes.

## WP-050: Gateway Module Extraction And Canonical AI IR

Objective: extract Gateway logic from HTTP glue and introduce Canonical AI Request/Response as the only internal AI request format.

Read first:

- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`

Owns:

- `apps/api/internal/modules/gateway` if created
- Gateway endpoint adapters
- Canonical AI IR DTOs
- compatibility warning model
- HTTP gateway handlers

Definition of Done:

- Chat Completions, Responses, and Messages parse into Canonical AI Request.
- Response renderer can render OpenAI Chat, OpenAI Responses, and Anthropic Messages shapes.
- Source protocol and source endpoint are preserved for audit only.
- No endpoint handler directly selects Provider Account.

Required gates:

- `cd apps/api && go test ./internal/httpserver ./internal/modules/...`
- Golden tests for endpoint conversion.
- OpenAPI gates if route schemas change.

## WP-060: Capability Taxonomy Alignment

Objective: replace ad hoc capability names with canonical descriptors.

Read first:

- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/SCHEDULER_V1_SPEC.md`

Owns:

- capability seed data
- model capability DTOs
- provider capability DTOs
- scheduler capability matching

Definition of Done:

- Canonical keys use names like `streaming`, `tool_calling`, `json_mode`, `structured_output`.
- DTO convenience fields map to descriptors but do not become source-of-truth keys.
- Scheduler hard filters use `RequestCapability` versus `EffectiveCapability`.
- Tests reject unknown or misspelled capability keys.

Required gates:

- `cd apps/api && go test ./internal/modules/models/... ./internal/modules/providers/... ./internal/modules/scheduler/...`

## WP-070: OpenAI-Compatible Adapter v1

Objective: make OpenAI-compatible upstream dispatch robust enough for MVP.

Read first:

- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/SECURITY_MODEL.md`

Owns:

- `apps/api/internal/modules/provider_adapters`
- OpenAI-compatible request builder
- stream parser
- usage parser
- error classifier
- provider preset integration

Definition of Done:

- Non-streaming and SSE streaming work through adapter interface.
- Upstream errors are mapped to internal provider error classes.
- Usage is parsed when present and marked estimated when absent.
- Provider credentials are injected without leaking to logs.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver`
- Gateway smoke with mock upstream.

## WP-080: Responses And Messages Compatibility

Objective: finish MVP endpoint conversion between Chat Completions, Responses, and Anthropic Messages.

Read first:

- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`

Owns:

- endpoint adapters and renderers
- conversion golden tests
- compatibility warning behavior

Definition of Done:

- `/v1/chat/completions`, `/v1/responses`, and `/v1/messages` can target the same OpenAI-compatible upstream.
- Tools, structured output, max tokens, instructions/system, and stream flags are converted or rejected with explicit warnings/errors.
- Stream events render in the caller's source protocol.

Required gates:

- Golden conversion tests.
- `cd apps/api && go test ./internal/httpserver ./internal/modules/...`

## WP-090: Scheduler v1 Hardening

Objective: make Scheduler v1 auditable and safe under concurrency.

Read first:

- `docs/SCHEDULING_KERNEL_DESIGN.md`
- `docs/SCHEDULER_V1_SPEC.md`
- `docs/SCHEDULER_STRATEGY_EXTENSION_SPEC.md`
- `docs/SCHEDULING_SCENARIOS.md`

Owns:

- `apps/api/internal/modules/scheduler`
- `apps/api/internal/persistence/redisstore/scheduler`
- `apps/api/internal/persistence/entstore/scheduler`

Definition of Done:

- `balanced` and `cost_saver` strategies exist with version/hash/weights snapshot.
- Hard filters emit structured reject reasons.
- Lease acquisition is atomic and prevents concurrency overflow.
- Feedback updates are recorded.
- Scheduler decisions preserve candidate count, selected account, scores, and warnings.

Required gates:

- `cd apps/api && go test ./internal/modules/scheduler/... ./internal/persistence/redisstore/scheduler/... ./internal/persistence/entstore/scheduler/...`
- Scenario tests from `docs/SCHEDULING_SCENARIOS.md`.

## WP-100: Usage, Billing, Audit, Outbox Closure

Objective: make every Gateway request produce durable operational evidence without blocking the main path unnecessarily.

Read first:

- `docs/DOMAIN_EVENTS_SPEC.md`
- `docs/OBSERVABILITY_SPEC.md`
- `docs/DATA_MODEL.md`
- `docs/SECURITY_MODEL.md`

Owns:

- `apps/api/internal/modules/usage`
- `apps/api/internal/modules/billing`
- `apps/api/internal/modules/audit`
- `apps/api/internal/modules/events`
- `apps/api/internal/workers/outbox`

Definition of Done:

- Success and failure requests record usage.
- Scheduler feedback is recorded.
- High-risk admin writes record audit.
- Domain outbox has idempotent dispatch tests.
- Billing ledger uses decimal strings or numeric-safe representation, not floats.

Required gates:

- `cd apps/api && go test ./internal/modules/usage/... ./internal/modules/billing/... ./internal/modules/audit/... ./internal/modules/events/... ./internal/workers/...`

## WP-110: Provider Preset Registry Expansion

Objective: support compatible provider presets without creating adapter forks for every provider.

Read first:

- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`

Owns:

- `apps/api/internal/modules/providers/preset`
- provider alias route registration
- preset capability declarations

Definition of Done:

- OpenAI-compatible and Anthropic-compatible generic presets exist.
- Common provider keys can be registered by metadata.
- Provider alias paths force provider context but reuse Gateway runtime.
- Model visibility and API key group permissions still apply.

Required gates:

- `cd apps/api && go test ./internal/modules/providers/preset/... ./internal/httpserver`

## WP-120: Reverse Proxy Runtime Foundation

Objective: implement the safe base runtime for non-API-key accounts.

Read first:

- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/OBSERVABILITY_SPEC.md`

Owns:

- `apps/api/internal/modules/reverse_proxy`
- runtime errors and metrics
- outgoing header hygiene
- account-isolated clients

Definition of Done:

- `runtime_class != api_key` must route through Reverse Proxy Runtime.
- Account clients are isolated.
- Forbidden SRapi headers cannot reach upstream.
- Cookie jar and credential handling are designed for encryption.
- Risk classes map to account state changes or feedback.

Required gates:

- `cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/...`
- Header hygiene tests.
- Cross-account isolation tests.

## WP-130: OAuth Refresh And Token Lifecycle

Objective: support refreshable non-API-key credentials safely.

Read first:

- `docs/REVERSE_PROXY_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/OPERATIONS.md`

Owns:

- refresh lock implementation
- credential update flow
- audit records
- account status transitions

Definition of Done:

- Refresh uses per-account lock.
- Refresh failure never overwrites old credentials.
- Refresh success re-encrypts credentials.
- Session invalid, account locked, account banned, and abuse signals stop scheduling.

Required gates:

- `cd apps/api && go test ./internal/modules/accounts/... ./internal/modules/reverse_proxy/...`

## WP-140: CLI Runtime Lessons From CLIProxyAPI

Objective: add Codex/Claude/Gemini CLI-style runtime concepts without importing CLIProxyAPI architecture wholesale.

Read first:

- `specs/REFERENCE_PROJECT_DECISIONS.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`

Owns:

- runtime classes for `cli_client_token` and `oauth_refresh`
- model alias behavior
- session affinity hooks
- websocket route planning

Definition of Done:

- SRapi has explicit abstractions for CLI account runtime, not file-watcher state as source of truth.
- Model alias and session affinity feed Scheduler and model registry.
- Codex/Claude/Gemini CLI-specific behavior is confined to adapter/runtime layers.

Required gates:

- Adapter/runtime unit tests.
- Scheduler capability and sticky tests.

## WP-150: Admin Ops And Scheduler Diagnostics

Objective: expose the minimum operator view required to debug Gateway behavior.

Read first:

- `docs/OBSERVABILITY_SPEC.md`
- `docs/SCHEDULER_V1_SPEC.md`
- `docs/OPERATIONS.md`
- `docs/OPENAPI_CONTRACT.md`

Owns:

- admin scheduler overview
- decisions list/detail
- provider/account health summaries
- error owner classification
- metrics foundations

Definition of Done:

- Admin can query recent decisions and inspect reject reasons and score breakdown.
- Provider/account health summary includes cooldown, error class, latency, quota, and runtime class.
- Metrics names follow operations spec.

Required gates:

- OpenAPI gates.
- `cd apps/api && go test ./internal/httpserver ./internal/modules/scheduler/... ./internal/modules/usage/...`

## WP-160: Frontend Foundation

Objective: create the actual console shell and first usable workflows.

Read first:

- `docs/FRONTEND_DESIGN_SYSTEM.md`
- `docs/OPENAPI_CONTRACT.md`
- `specs/FINAL_STATE.md`

Owns:

- `apps/web`
- shared API client usage
- layout, theme tokens, auth shell
- API key, provider account, usage, scheduler decision views

Definition of Done:

- Next.js app starts locally.
- Uses generated TypeScript SDK.
- First screen is console experience, not marketing landing page.
- Responsive layout follows design system and avoids generic admin theme.
- API key create/list and scheduler decision view work against backend.

Required gates:

- frontend typecheck/lint if configured.
- browser smoke screenshot for desktop and mobile once app exists.

## WP-170: Account Operations Parity From sub2api

Objective: implement operator-grade account pool management inspired by sub2api.

Read first:

- `specs/REFERENCE_PROJECT_DECISIONS.md`
- `docs/DOMAIN_MODEL.md`
- `docs/OBSERVABILITY_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`

Owns:

- account groups
- health/quota snapshots
- account test and recovery APIs
- proxy binding
- import/export where safe

Definition of Done:

- Admin can list, test, enable, disable, and inspect accounts.
- Account health and quota are visible and feed Scheduler.
- Sensitive import/export behavior is documented and safe.

Required gates:

- Account service and persistence tests.
- OpenAPI gates.

## WP-180: Subscription And Pricing

Objective: implement user entitlement, pricing rules, and subscription state before payments.

Read first:

- `docs/DOMAIN_MODEL.md`
- `docs/DATA_MODEL.md`
- `docs/PAYMENT_SPEC.md`
- `docs/MVP_SPEC.md`

Owns:

- subscription plans
- user subscriptions
- pricing rule resolution
- entitlement contract used by Gateway/Scheduler

Definition of Done:

- User/model entitlement can reject Gateway before Scheduler consumes accounts.
- Pricing rules use decimal-safe values.
- Usage charge estimates can be linked to billing ledger.

Required gates:

- Ent/migration checks.
- Billing/subscription tests.

## WP-190: Payment System Phase 2

Objective: implement payment orders and at least one provider integration path.

Read first:

- `docs/PAYMENT_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/DOMAIN_EVENTS_SPEC.md`
- `docs/OPERATIONS.md`

Owns:

- payment provider instances
- order state machine
- webhook verification
- fulfillment
- refund hooks

Definition of Done:

- Payment config is encrypted.
- Order state transitions are legal and tested.
- Webhooks are signed, idempotent, and fail closed.
- Fulfillment writes billing/subscription state and audit.

Required gates:

- Payment unit/integration tests.
- Secret scan.
- Ent/migration checks.

## WP-200: Affiliate Rebate Phase 2

Objective: implement invitation and rebate ledger after payment correctness exists.

Read first:

- `docs/AFFILIATE_REBATE_SPEC.md`
- `docs/PAYMENT_SPEC.md`
- `docs/DOMAIN_EVENTS_SPEC.md`

Owns:

- invite code
- invite relationship
- affiliate rules
- affiliate ledger
- refund compensation
- transfer to balance

Definition of Done:

- Self-invite and duplicate invite are rejected.
- Paid orders generate idempotent rebate accrual.
- Refunds append compensation entries.
- Transfer to balance writes billing ledger and audit.

Required gates:

- Affiliate tests.
- Payment regression tests.

## WP-210: Production Operations

Objective: make self-hosted production operation safe.

Read first:

- `docs/OPERATIONS.md`
- `docs/CONFIGURATION_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/OBSERVABILITY_SPEC.md`

Owns:

- backup/restore
- data retention
- `/metrics`
- release smoke
- config validation
- deployment docs

Definition of Done:

- Release mode rejects weak secrets.
- Backup and restore flow is documented and tested where practical.
- Metrics endpoint emits baseline metrics.
- Data cleanup jobs exist for configured retention tables.

Required gates:

- `make check`
- Docker Compose smoke.
- Secret scan.

## WP-220: Anthropic-Compatible Upstream Adapter

Objective: complete Anthropic-compatible upstream dispatch for the existing `/v1/messages` Gateway runtime and provider aliases.

Read first:

- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`

Owns:

- `apps/api/internal/modules/provider_adapters`
- Anthropic-compatible request/response and SSE parsing
- reverse-proxy adapter dispatch for Anthropic-compatible accounts
- focused Gateway regressions for `/v1/messages` and Anthropic provider aliases
- route/provider docs when behavior changes

Definition of Done:

- `anthropic-compatible` Provider targets use Anthropic Messages `/messages` upstream payloads, not OpenAI Chat Completions payloads.
- API-key accounts inject Anthropic-compatible credentials without leaking SRapi request headers or credentials.
- Non-streaming and SSE streaming Anthropic Messages responses parse text and usage.
- Upstream Anthropic error objects map to internal provider error classes.
- Reverse-proxy runtime accounts preserve Anthropic protocol dispatch.
- Gateway `/v1/messages` and Anthropic provider aliases can schedule an Anthropic-compatible account and record usage/decision evidence.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver`
- `make architecture-check`

## WP-230: Gemini Native Gateway Route Foundation

Objective: add the first Gemini-native Gateway route family while preserving the existing Canonical AI Request, Scheduler, Provider Adapter, usage, and decision loop.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`

Owns:

- Gemini native OpenAPI request/response schemas and generated SDK/server types
- `/v1beta/models/{model}:generateContent` and `/v1beta/models/{model}:streamGenerateContent` Gateway routes
- Gemini request normalization to Canonical AI Request
- Gemini response and SSE rendering from Canonical AI Response
- Gateway regressions proving auth, model policy, Scheduler decision, usage, and request ID evidence
- docs/status updates for the Gemini route foundation

Definition of Done:

- Gemini GenerateContent requests convert `contents`, `systemInstruction`, and `generationConfig` into Canonical AI Request fields.
- The route uses Gateway API Key auth, model visibility, entitlement admission, Scheduler, Provider Adapter invocation, usage logs, and scheduler decision/feedback evidence.
- Responses render Google/Gemini-shaped JSON and stream events for Gemini clients.
- Provider errors render Google-style gateway errors for Gemini native callers.
- The implementation does not add frontend visuals.
- OpenAPI Go and TypeScript generated artifacts are in sync.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/httpserver`
- `make architecture-check`

## WP-240: Gemini Native Upstream Adapter

Objective: allow scheduled Gemini-compatible and native-gemini Provider Accounts to invoke Gemini `generateContent` / `streamGenerateContent` upstream endpoints from the Canonical AI Request path.

Read first:

- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`

Owns:

- `apps/api/internal/modules/provider_adapters`
- Gemini GenerateContent request payload construction from `contract.TextRequest`
- Gemini non-streaming response parsing, usage normalization, and error classification
- Gemini SSE stream aggregation and usage normalization
- API-key auth injection for Gemini-compatible accounts
- reverse-proxy Gemini dispatch through Reverse Proxy Runtime
- focused Gateway regression proving the WP-230 Gemini native route can schedule a Gemini-compatible upstream account
- provider adapter and route/provider docs when behavior changes

Definition of Done:

- `provider.protocol` or `provider.adapter_type` of `gemini-compatible` / `native-gemini` targets Gemini `models/{model}:generateContent` or `models/{model}:streamGenerateContent`, not OpenAI Chat Completions.
- API-key accounts inject Gemini-compatible credentials without leaking SRapi request headers or credentials.
- Non-streaming and SSE streaming Gemini responses parse text plus `usageMetadata` prompt/candidates/cache tokens.
- Gemini error objects map to internal provider error classes.
- Reverse-proxy runtime accounts preserve Gemini protocol dispatch.
- Gateway `/v1beta/models/{model}:generateContent` can schedule a Gemini-compatible account and record usage/decision evidence.
- No frontend visuals.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver`
- `make architecture-check`

## WP-250: Provider Model Discovery v1

Objective: let operators safely discover upstream model catalogs for Provider Accounts and optionally persist the discovered IDs as account `supported_models` metadata without bypassing existing Gateway/Scheduler boundaries.

Read first:

- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/ARCHITECTURE.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`

Owns:

- `packages/openapi/openapi.yaml`
- generated Go OpenAPI types and generated TypeScript SDK
- `apps/api/internal/httpserver`
- account discovery HTTP client logic for OpenAI-compatible, Anthropic-compatible, and Gemini-compatible model-list shapes
- account metadata persistence for discovered `supported_models`
- admin audit/test evidence for discovery attempts
- provider registry docs/status updates for the live discovery behavior

Definition of Done:

- Admin route `POST /api/v1/admin/accounts/{id}/discover-models` is OpenAPI-described, generated, CSRF-protected, and available without frontend changes.
- API-key Provider Accounts can discover model IDs from OpenAI-compatible `/models`, Anthropic-compatible `/models`, and Gemini-compatible `/models` upstream responses.
- The route supports preview-only discovery by default and `persist=true` to update account metadata with `supported_models`, `model_discovery_source`, and `model_discovery_last_seen_at`.
- Persisted `supported_models` participates in later provider-neutral candidate filtering so discovery changes routing only through existing Gateway/Scheduler boundaries.
- Discovery injects provider credentials according to existing auth-mode metadata without returning or logging credential values.
- Unsupported runtime/provider combinations fail with explicit control-plane errors rather than leaking upstream internals.
- Focused HTTP regressions prove success, persistence, secret hygiene, and unsupported runtime/provider handling.
- No Scheduler provider-specific logic and no frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver ./internal/modules/accounts/...`
- `make architecture-check`

## WP-260: Ops SLO And Alert Control Plane v1

Objective: add the first durable operations control plane for SLO definitions, SLO evaluation snapshots, and alert acknowledgement so SRapi can move from basic metrics to actionable production governance without adding frontend visuals.

Read first:

- `docs/OBSERVABILITY_SPEC.md`
- `docs/OPERATIONS.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `docs/SECURITY_MODEL.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`

Owns:

- `packages/openapi/openapi.yaml`
- generated Go OpenAPI types and generated TypeScript SDK
- `apps/api/ent/schema`
- `apps/api/migrations/postgres`
- `apps/api/internal/modules/operations`
- `apps/api/internal/persistence/entstore/operations`
- `apps/api/internal/httpserver`
- operations/observability docs and status updates

Definition of Done:

- Admin Ops exposes OpenAPI-described, generated, cookie-authenticated routes for:
  - `GET /api/v1/admin/ops/slo`
  - `POST /api/v1/admin/ops/slo`
  - `PATCH /api/v1/admin/ops/slo/{id}`
  - `GET /api/v1/admin/ops/alerts`
  - `POST /api/v1/admin/ops/alerts/{id}/ack`
- SLO definitions persist fields from `OBSERVABILITY_SPEC.md`: name, SLI type, objective, window, status, filter, alert policy, and burn-rate thresholds.
- Alert events persist severity, status, fingerprint, summary, details, started/resolved/acknowledged metadata, and never contain credentials, prompts, API keys, cookies, or Authorization values.
- SLO list responses include computed availability/burn-rate evidence from existing usage logs for gateway availability SLOs.
- SLO create/update and alert ack are CSRF-protected admin writes and emit safe audit records.
- Persistence is implemented through Ent and PostgreSQL migrations with data model docs aligned.
- Focused service/store/HTTP regressions cover SLO creation/update/list, computed burn-rate evidence, alert ack, CSRF protection, and secret hygiene.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `make ent-generate-check`
- `make migration-check`
- `cd apps/api && go test ./internal/modules/operations/... ./internal/persistence/entstore/operations ./internal/httpserver`
- `make architecture-check`
- `git diff --check`

## WP-270: Embeddings Passthrough Runtime v1

Objective: add the first `/v1/embeddings` Gateway runtime so OpenAI-compatible embedding clients go through SRapi auth, model policy, entitlement, Scheduler, Provider Adapter dispatch, usage, billing, and operational evidence instead of bypassing the platform.

Read first:

- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`

Owns:

- `packages/openapi/openapi.yaml`
- generated Go OpenAPI types and generated TypeScript SDK
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`
- provider alias route registration for OpenAI-compatible presets
- gateway/provider compatibility docs and status updates

Definition of Done:

- `POST /v1/embeddings` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/embeddings`, reuse the same runtime while forcing provider context.
- Requests minimally validate `model` plus string or string-array `input`; token-array input is explicitly rejected until a later compatibility package supports it.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to `/embeddings`, pass the mapped upstream model, parse usage, and return OpenAI-shaped embedding objects.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, model/API-key policy rejection, upstream error classification, usage/decision evidence, and adapter request/response parsing.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make secret-scan`
- `git diff --check`

## WP-280: HTTP Runtime Partition And Size Harness

Objective: split the oversized HTTP runtime implementation into route-family files and add architecture harness coverage so `runtime_http.go` stops growing as a catch-all protocol layer.

Read first:

- `docs/ARCHITECTURE.md`
- `docs/ARCHITECTURE_REQUIREMENTS.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `apps/api/internal/httpserver`
- `apps/api/internal/architecture`

Owns:

- `apps/api/internal/httpserver`
- `apps/api/internal/architecture`
- architecture docs and status updates

Definition of Done:

- HTTP runtime code is partitioned into coherent files such as gateway handlers, admin/control-plane handlers, operations handlers, runtime bootstrap/state, response/error helpers, and metrics helpers.
- Route handlers continue to call service/contract boundaries only; no Ent or persistence implementation imports are introduced into `internal/httpserver`.
- `runtime_http.go` is reduced to a small compatibility or shared-runtime file instead of a 7000+ line catch-all.
- Architecture harness adds a file-size or ownership check that prevents `runtime_http.go` from regressing into the catch-all layer again.
- Existing Gateway, admin, operations, payment, subscription, and observability HTTP tests continue to pass without frontend visual work.
- No user-facing API behavior changes are introduced.

Required gates:

- `cd apps/api && go test ./internal/httpserver ./internal/architecture`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `git diff --check`

## WP-290: Images Generations Runtime v1

Objective: add OpenAI-compatible image generation runtime through the standard Gateway auth, model policy, entitlement, Scheduler, Provider Adapter, usage evidence, and generated contract path.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/images/generations` and OpenAI-compatible provider alias contract
- Gateway image request normalization/rendering
- Provider Adapter image generation dispatch for OpenAI-compatible API-key and reverse-proxy accounts
- HTTP runtime handler/tests and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/images/generations` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/images/generations`, reuse the same runtime while forcing provider context.
- Requests minimally validate `model` and `prompt`; OpenAI-compatible `n`, `size`, `quality`, `style`, `response_format`, and `user` fields are preserved where supported.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to `/images/generations`, pass the mapped upstream model, parse `url` and `b64_json` image outputs, and return OpenAI-shaped image generation responses.
- The request capability taxonomy includes an explicit image generation endpoint capability so Scheduler can reject text-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream request/response parsing, and usage/decision evidence.
- Image edits, variations, audio, rerank, moderation, and realtime are left to later packages.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make secret-scan`
- `git diff --check`

## WP-300: Go Code Quality Harness

Objective: add an explicit code-quality harness so formatting, static checks, and size guardrails are executable gates rather than informal review expectations.

Read first:

- `docs/ARCHITECTURE.md`
- `docs/ARCHITECTURE_REQUIREMENTS.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `specs/QUALITY_GATES.md`
- `Makefile`
- `apps/api/internal/architecture`

Owns:

- `Makefile`
- `apps/api/internal/codequality`
- `specs/QUALITY_GATES.md`
- architecture requirements/status updates

Definition of Done:

- `make code-quality-check` exists and is documented in `make help`.
- Code-quality check runs as part of `make check`.
- Harness verifies Go formatting drift, `go vet ./...`, production Go file size, and production function size while excluding generated Go output from size thresholds.
- Thresholds are calibrated to current code so the harness prevents further uncontrolled growth without forcing unrelated refactors in this package.
- Documentation explains what code-quality-check covers and how it differs from architecture-check.
- No frontend visuals are added.

Required gates:

- `make code-quality-check`
- `make architecture-check`
- `cd apps/api && go test ./...`
- `make check`
- `git diff --check`

## WP-310: Moderations Runtime v1

Objective: add the first OpenAI-compatible moderation runtime so safety classification requests use SRapi auth, model policy, entitlement, Scheduler, Provider Adapter dispatch, usage, billing, and operational evidence instead of bypassing the platform.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/moderations` and OpenAI-compatible provider alias contract
- Gateway moderation request normalization/rendering
- Provider Adapter moderation dispatch for OpenAI-compatible API-key and reverse-proxy accounts
- HTTP runtime handler/tests, capability taxonomy, and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/moderations` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/moderations`, reuse the same runtime while forcing provider context.
- Requests minimally validate `model` and text/string-array `input`; image multimodal moderation input remains a later compatibility package.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to `/moderations`, pass the mapped upstream model, parse `flagged`, `categories`, `category_scores`, and `category_applied_input_types`, and return OpenAI-shaped moderation responses.
- The request capability taxonomy includes an explicit moderation endpoint capability so Scheduler can reject generation-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream request/response parsing, and usage/decision evidence.
- Audio, rerank, image moderation input, realtime, and SDK examples are left to later packages.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-320: Rerank Runtime v1

Objective: add a provider-neutral rerank runtime so search/retrieval ranking requests use SRapi auth, model policy, entitlement, Scheduler, Provider Adapter dispatch, usage, billing, and operational evidence instead of bypassing the platform.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/modules/providers/preset`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/rerank` and rerank-compatible provider alias contract
- Gateway rerank request normalization/rendering
- Provider Adapter rerank dispatch for rerank-compatible API-key and reverse-proxy accounts
- Provider preset registry support for `rerank-compatible`
- HTTP runtime handler/tests, capability taxonomy, and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/rerank` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- Rerank-compatible provider alias routes, including `/api/provider/rerank-compatible/v1/rerank`, reuse the same runtime while forcing provider context.
- Requests minimally validate `model`, non-empty `query`, non-empty `documents`, optional `top_n`, and optional `return_documents`; document objects are preserved as structured metadata.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- Rerank-compatible API-key and reverse-proxy accounts dispatch upstream to `/rerank`, pass the mapped upstream model, parse `index`, `relevance_score`, optional `document`, optional usage, and return a stable rerank response.
- The request capability taxonomy includes an explicit `rerank` endpoint capability so Scheduler can reject generation-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream request/response parsing, and usage/decision evidence.
- Audio, realtime/websocket, image edits/variations, SDK examples, and migration guides are left to later packages.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/modules/providers/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-330: Audio Transcriptions Runtime v1

Objective: add OpenAI-compatible audio transcription runtime so speech-to-text requests use SRapi auth, model policy, entitlement, Scheduler, Provider Adapter dispatch, usage, billing, and operational evidence instead of bypassing the platform.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/audio/transcriptions` and OpenAI-compatible provider alias contract
- Gateway audio transcription request normalization/rendering
- Provider Adapter multipart dispatch for OpenAI-compatible API-key and reverse-proxy accounts
- HTTP runtime handler/tests, capability taxonomy, and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/audio/transcriptions` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/audio/transcriptions`, reuse the same runtime while forcing provider context.
- Requests minimally validate multipart `file`, `model`, optional `language`, `prompt`, `response_format`, `temperature`, and `user`.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to `/audio/transcriptions`, pass the mapped upstream model, preserve audio file metadata/content type, parse transcription `text`, optional verbose metadata, and usage, and return OpenAI-shaped responses.
- The request capability taxonomy includes an explicit `audio_transcriptions` endpoint capability so Scheduler can reject text-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream multipart request/response parsing, and usage/decision evidence.
- Speech synthesis, streaming audio transcription, realtime/websocket, SDK examples, and migration guides are left to later packages.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/modules/providers/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-340: Audio Speech Runtime v1

Objective: add OpenAI-compatible audio speech synthesis runtime so text-to-speech requests use SRapi auth, model policy, entitlement, Scheduler, Provider Adapter dispatch, usage, billing, and operational evidence instead of bypassing the platform.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/audio/speech` and OpenAI-compatible provider alias contract
- Gateway audio speech request normalization and binary response rendering
- Provider Adapter JSON dispatch for OpenAI-compatible API-key and reverse-proxy accounts
- HTTP runtime handler/tests, capability taxonomy, and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/audio/speech` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/audio/speech`, reuse the same runtime while forcing provider context.
- Requests minimally validate JSON `model`, non-empty `input`, `voice`, optional `response_format`, optional `speed`, optional `instructions`, and optional `user`.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to `/audio/speech`, pass the mapped upstream model, preserve voice/format/speed/instructions/user fields, parse binary audio bytes and content type, and return audio bytes to the client.
- The request capability taxonomy includes an explicit `audio_speech` endpoint capability so Scheduler can reject transcription-only or text-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream request/response parsing, binary content type, and usage/decision evidence.
- Speech streaming, realtime/websocket, SDK examples, migration guides, and frontend visuals are left to later packages.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/modules/providers/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-350: Antigravity Reverse Proxy Runtime Identity v1

Objective: make Antigravity a first-class backend reverse-proxy identity so operators can configure `reverse-proxy-antigravity` Provider Accounts with `antigravity_desktop` runtime context and route existing Gateway text requests through Scheduler, Provider Adapter, and Reverse Proxy Runtime without adding Gateway-local DTOs or provider-specific Scheduler branches.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/modules/reverse_proxy`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `ProviderAdapterType` enum support for `reverse-proxy-antigravity`
- Reverse Proxy Runtime default client identity for `account.upstream_client = antigravity_desktop`
- Provider Adapter reverse-proxy dispatch for Antigravity accounts using the existing `provider.protocol` target sub-protocol
- HTTP Gateway regression proving a configured Antigravity desktop account can be selected by Scheduler and invoked through Reverse Proxy Runtime
- route matrix/provider adapter/reverse proxy docs and status updates

Definition of Done:

- Admin Provider creation accepts `adapter_type = reverse-proxy-antigravity` through the OpenAPI-generated enum.
- `desktop_client_token` or `ide_plugin_token` accounts with `upstream_client = antigravity_desktop` use `access_token` bearer auth and the Antigravity default User-Agent unless account metadata overrides it.
- `reverse-proxy-antigravity` does not create a new Gateway-local DTO; existing OpenAI-compatible, Anthropic-compatible, and Gemini-compatible request contracts continue to select the target upstream protocol through `provider.protocol`.
- OpenAI-compatible text dispatch targets `/chat/completions`; Gemini-compatible text dispatch targets `models/{model}:generateContent` or `:streamGenerateContent` while preserving mapped upstream model names.
- Gateway `/v1/chat/completions` can schedule an Antigravity reverse-proxy account and record normal usage/decision evidence through existing runtime paths.
- Antigravity-specific route aliases such as `/api/provider/antigravity/*` remain a separate route-registry package unless this package explicitly adds them.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-360: Antigravity Text Provider Alias Routes v1

Objective: promote Antigravity text provider aliases from the route matrix planning state to implemented backend behavior so operators can expose Antigravity desktop/IDE reverse-proxy accounts through provider-prefixed OpenAI Chat Completions and Anthropic Messages routes without adding Gateway-local DTOs or provider-specific Scheduler branches.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/providers/preset`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/modules/reverse_proxy`
- `apps/api/internal/httpserver`

Owns:

- Provider preset registry entry for `antigravity`
- capability-driven provider alias route registration
- Antigravity text aliases for `/antigravity/v1/*`, `/api/provider/antigravity/*`, and `/api/provider/antigravity/v1/*`
- OpenAPI representative contracts for `/api/provider/antigravity/v1/chat/completions` and `/api/provider/antigravity/v1/messages`
- Gateway regressions proving Antigravity aliases force the `antigravity` provider key and still route through Scheduler, Provider Adapter, and Reverse Proxy Runtime
- route matrix/provider registry/compatibility docs and status updates

Definition of Done:

- `provider_key = antigravity` is seeded with text route aliases, `reverse_proxy_antigravity` platform metadata, and desktop/IDE reverse-proxy account type allowlist.
- `/api/provider/antigravity/v1/chat/completions` and `/antigravity/v1/chat/completions` schedule only Provider records named `antigravity` and dispatch OpenAI-compatible payloads through `reverse-proxy-antigravity` when the selected provider uses `protocol = openai-compatible`.
- `/api/provider/antigravity/v1/messages` schedules only Provider records named `antigravity` and dispatches Anthropic Messages payloads through `reverse-proxy-antigravity` when the selected provider uses `protocol = anthropic-compatible`.
- Gateway usage logs and Scheduler decisions preserve the alias source endpoint and selected Provider/Account evidence.
- No Gateway-local Antigravity request/response DTO is introduced; `provider.protocol` continues to select the target upstream protocol.
- Gemini `/antigravity/v1beta/models/{model}:generateContent` aliases remain a follow-up route-parser package unless this package explicitly adds Gemini model-action alias parsing.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/providers/preset ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-370: Antigravity Gemini Model-Action Alias Routes v1

Objective: complete the Antigravity provider-prefixed Gateway alias surface for Gemini-native GenerateContent and StreamGenerateContent routes by routing Antigravity `/v1beta/models/{model}:...` aliases through the existing Gemini Gateway handler, Scheduler, Provider Adapter, and Reverse Proxy Runtime without adding Gateway-local DTOs.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/providers/preset`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/modules/reverse_proxy`
- `apps/api/internal/httpserver`

Owns:

- Antigravity Gemini model-action alias metadata in the provider preset registry
- HTTP route registration for `/antigravity/v1beta/models/{model}:generateContent`, `/antigravity/v1beta/models/{model}:streamGenerateContent`, `/api/provider/antigravity/v1beta/models/{model}:generateContent`, and `/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent`
- OpenAPI representative contracts for `/api/provider/antigravity/v1beta/models/{model}:generateContent` and `/api/provider/antigravity/v1beta/models/{model}:streamGenerateContent`
- Gateway regressions proving Antigravity Gemini aliases force `provider_key=antigravity`, preserve alias source endpoints, and still dispatch through `reverse-proxy-antigravity` using `provider.protocol = gemini-compatible`
- route matrix/provider registry/compatibility docs and status updates

Definition of Done:

- Antigravity Gemini model-action aliases are registered from preset metadata instead of a provider-specific branch in Scheduler or Provider Adapter code.
- Non-streaming Antigravity Gemini alias requests normalize the Gemini request, schedule only Provider records named `antigravity`, dispatch to upstream `models/{mapped_model}:generateContent`, render Gemini-shaped responses, and record usage/decision evidence with the alias source endpoint.
- Streaming Antigravity Gemini alias requests dispatch to upstream `models/{mapped_model}:streamGenerateContent`, render Gemini SSE chunks, and preserve Scheduler/usage evidence.
- Standard `/v1beta/models/{model}:generateContent` and `/v1beta/models/{model}:streamGenerateContent` behavior remains unchanged.
- No Gateway-local Antigravity request/response DTO is introduced; `provider.protocol` continues to select the target upstream protocol.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/providers/preset ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-380: Responses WebSocket Runtime Foundation v1

Objective: expose the first backend WebSocket transport for Responses-compatible clients at `/v1/responses/ws`, while preserving the existing Gateway -> Scheduler -> Provider Adapter -> usage/decision evidence path and avoiding Gateway-local provider DTOs.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/SCHEDULING_KERNEL_DESIGN.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/httpserver`
- `apps/api/internal/modules/gateway`

Owns:

- OpenAPI contract for `GET /v1/responses/ws` WebSocket upgrade
- HTTP Gateway WebSocket transport adapter for Responses `response.create` events
- Scheduler session-affinity propagation from WebSocket query/header hints
- WebSocket regressions proving non-streaming and streaming Responses requests reuse existing runtime behavior
- route matrix/compatibility/OpenAPI docs and status updates

Definition of Done:

- `GET /v1/responses/ws` authenticates with Gateway bearer keys before upgrading.
- WebSocket text frames accept either raw `ResponsesRequest` JSON or `response.create` events with the request under `response`; query `model` may fill a missing request model.
- Each accepted request is executed by the existing `/v1/responses` Gateway runtime so model policy, entitlement, Scheduler, Provider Adapter, usage logs, billing, and Scheduler feedback remain unchanged.
- Query/header sticky hints such as `session_affinity_key`, `sticky_strength`, and `sticky_account_id` feed the existing Scheduler affinity logic; the WebSocket layer does not choose accounts directly.
- Non-streaming Responses results return a `response.completed` WebSocket frame, and streaming Responses requests forward the rendered Responses stream events as WebSocket JSON frames.
- Gateway usage logs and Scheduler decisions preserve `/v1/responses/ws` as the source endpoint.
- Direct upstream WSS relay, provider-native realtime semantics, and richer slot lifecycle remain follow-up packages.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-390: Reverse Proxy WSS Relay Foundation v1

Objective: add the first Reverse Proxy Runtime primitive for direct upstream WebSocket/WSS relay so future provider-native realtime adapters can use the same per-account client, credential injection, header hygiene, proxy binding, and metrics path as HTTP reverse-proxy requests.

Read first:

- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `apps/api/internal/modules/reverse_proxy/contract`
- `apps/api/internal/modules/reverse_proxy/service`

Owns:

- Reverse Proxy Runtime contract for WebSocket relay
- runtime implementation that dials upstream WebSocket endpoints with per-account HTTP client/proxy/cookie context
- header/auth/User-Agent hygiene for WebSocket handshakes
- bidirectional text/binary message relay primitives and relay accounting
- focused tests proving relay behavior, credential-source precedence, and no leaked SRapi/client auth headers
- reverse proxy docs and status updates

Definition of Done:

- `reverse_proxy/contract` exposes a WebSocket relay interface without changing existing HTTP `Runtime.Do` call sites or fakes.
- `reverse_proxy/service.Service` implements both existing HTTP runtime and WebSocket relay runtime.
- WebSocket handshakes reuse sanitized headers, account credential auth injection, default upstream-client User-Agent, per-account client/proxy/cookie context, and compression-disabled behavior.
- Caller-provided `Authorization`, `Cookie`, `Sec-WebSocket-*`, `X-Request-ID`, `X-Forwarded-*`, `Via`, `X-SRapi-*`, and `X-Gateway-*` headers do not leak upstream; credentials are injected only from the selected account runtime.
- Relay supports text and binary messages from upstream and client directions, returns basic message/byte accounting, and records reverse-proxy request/success/error metrics.
- This package does not implement provider-native realtime protocol adapters, Gateway route binding, or rich slot lifecycle; those remain follow-up packages.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/reverse_proxy/...`
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-400: Codex CLI 2api Responses Upstream Shape v1

Objective: make `reverse-proxy-codex-cli` construct the Codex official-client HTTP Responses shape and send it through Reverse Proxy Runtime, instead of treating Codex 2api as generic OpenAI-compatible `/chat/completions`.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `apps/api/internal/modules/provider_adapters/contract`
- `apps/api/internal/modules/provider_adapters/service`
- `apps/api/internal/modules/reverse_proxy/contract`
- local reference: `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/codex_executor.go`

Owns:

- Codex CLI reverse-proxy adapter dispatch for text requests.
- Codex `/backend-api/codex/responses` request body construction from Canonical AI Request.
- Codex official-client headers such as `Accept: text/event-stream`, `Originator`, `X-Client-Request-Id`, `Session_id`, `Version`, `X-Codex-Beta-Features`, and `Chatgpt-Account-Id` when account metadata provides them.
- Reverse Proxy Runtime credential injection for CLI/OAuth/session token runtime classes; `api_key` remains official API-key adapter behavior and is not a 2api reverse-proxy credential source.
- Codex Responses SSE/JSON parsing into `TextResponse`, including `response.output_text.delta`, `response.output_item.done`, `response.completed`, and usage.
- Focused tests proving Codex uses `/responses`, generic reverse-proxy OpenAI-compatible behavior still uses `/chat/completions`, and selected account credentials/UA reach upstream.

Definition of Done:

- `reverse-proxy-codex-cli` text requests call `base_url + "/responses"` with `stream: true`; they must not call `/chat/completions`.
- Codex request body includes mapped upstream `model`, Responses-style `input`, non-null `instructions`, supported sampling/tool/output fields, and no OpenAI Chat Completions `stream_options`.
- Runtime receives selected account context and injects account credentials; caller auth must not be forwarded.
- `reverse-proxy-codex-cli` rejects `runtime_class = api_key` and requires OAuth/session/CLI-token style account credentials.
- Parser accepts Codex SSE and JSON response shapes and returns text/usage without Gateway-local Codex DTOs.
- This package does not implement Codex Responses WebSocket upstream relay or local Codex CLI client ingress; those remain follow-up packages.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/...`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-410: Codex CLI 2api Responses WebSocket Upstream Relay v1

Objective: bind `/v1/responses/ws` to the Reverse Proxy Runtime WebSocket primitive for explicitly requested Codex CLI 2api accounts, so SRapi simulates Codex official-client Responses WebSocket requests upstream using selected OAuth/session/CLI credentials.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `apps/api/internal/httpserver/runtime_gateway_websocket.go`
- `apps/api/internal/modules/provider_adapters/contract`
- `apps/api/internal/modules/provider_adapters/service`
- `apps/api/internal/modules/reverse_proxy/contract`
- local reference: `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/codex_websockets_executor.go`

Owns:

- Provider Adapter realtime contract and Codex CLI `PrepareRealtime` behavior.
- Codex Responses WebSocket URL derivation from configured Codex base URL.
- Codex WebSocket official-client headers including `OpenAI-Beta: responses_websockets=2026-02-06`, `Originator`, `X-Client-Request-Id`, `Version`, `session_id`, timing and account headers when account metadata provides them.
- Initial upstream text frame construction by injecting `type: response.create` and replacing SRapi's local model with the mapped upstream model.
- `/v1/responses/ws` explicit opt-in via `upstream_ws` / `codex_responses_websocket` query or SRapi headers, followed by normal API key auth, model policy, entitlement, Scheduler, and selected-account checks.
- Usage and Scheduler evidence for the WebSocket source endpoint.
- Focused tests proving selected account credentials reach upstream, caller/SRapi headers do not define upstream auth, and usage evidence is recorded from upstream Responses WebSocket frames.

Definition of Done:

- Codex upstream WebSocket relay is only attempted when explicitly requested and a scheduled `reverse-proxy-codex-cli` account has websocket support metadata enabled.
- The upstream WebSocket request uses the selected account's OAuth/session/CLI credential through Reverse Proxy Runtime; `runtime_class = api_key` remains rejected for Codex 2api.
- The upstream URL is `base_url + "/responses"` with `http -> ws` and `https -> wss` scheme mapping.
- The initial upstream message is Codex Responses WebSocket shape, not a Gateway-local DTO and not an OpenAI Chat Completions payload.
- The local SRapi model name is not leaked upstream; the frame uses the selected mapping's `upstream_model_name`.
- Success and failure paths preserve `/v1/responses/ws` as source endpoint and record Scheduler/usage evidence.
- This package does not implement local Codex CLI client ingress, persistent multi-turn WebSocket session reuse, or Claude/Antigravity WebSocket adapters.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-470: OpenAI-compatible Realtime WebSocket Relay v1

Objective: expose `GET /v1/realtime` as an OpenAI-compatible Realtime WebSocket gateway route that schedules realtime-capable accounts and relays frames through Reverse Proxy Runtime using selected OAuth/session/client-token credentials.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/httpserver/runtime_gateway_websocket.go`
- `apps/api/internal/modules/provider_adapters/contract`
- `apps/api/internal/modules/provider_adapters/service`
- local reference: `/home/senran/Desktop/sub2api/backend/internal/service/openai_ws_protocol_resolver_test.go`
- local reference: `/home/senran/Desktop/CLIProxyAPI/sdk/api/handlers/openai/openai_responses_websocket_test.go`

Owns:

- `GET /v1/realtime` OpenAPI contract and docs alignment; this route is WebSocket upgrade, not `POST /v1/realtime`.
- Canonical realtime request normalization with required `realtime_websocket` and `streaming` capabilities.
- Provider Adapter `PrepareRealtime` for OpenAI-compatible OAuth/session/client-token accounts, deriving upstream `ws/wss` `/realtime?model=<mapped_upstream_model>`.
- Reverse Proxy Runtime relay binding for bidirectional text/binary frames.
- Realtime slot acquisition/release before/after upgrade using provider-neutral slot manager.
- Scheduler/usage evidence preserving `/v1/realtime` source endpoint.
- Tests proving selected account credentials define upstream auth and caller/SRapi headers do not leak upstream.

Definition of Done:

- `/v1/realtime?model=...` requires Gateway API Key auth, model policy, entitlement, `realtime_websocket` capability, and realtime slot capacity before upgrade.
- The scheduled Provider Account supplies the upstream OAuth/session/client-token credential through Reverse Proxy Runtime; `runtime_class = api_key` remains rejected for this 2api route.
- The upstream URL uses the selected mapping's `upstream_model_name`, not the local SRapi model name.
- Caller `Authorization`, `Cookie`, `Sec-WebSocket-*`, `X-SRapi-*`, and gateway headers do not define upstream identity.
- Success and failure paths preserve `/v1/realtime` in Scheduler decisions and usage logs.
- This package does not implement official API-key Realtime, persistent upstream session pools, local client ingress, or Claude Code / Antigravity provider-native realtime adapters.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-420: Claude Code CLI 2api Messages Upstream Shape v1

Objective: make `reverse-proxy-claude-code-cli` construct the Claude Code official-client HTTP Messages shape and send it through Reverse Proxy Runtime with selected OAuth/CLI credentials, instead of treating Claude Code 2api as generic Anthropic-compatible API-key dispatch.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `apps/api/internal/modules/provider_adapters/contract`
- `apps/api/internal/modules/provider_adapters/service`
- `apps/api/internal/modules/reverse_proxy/contract`
- local reference: `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/claude_executor.go`
- local reference: `/home/senran/Desktop/sub2api/backend/internal/service/gateway_service.go`

Owns:

- Claude Code reverse-proxy adapter dispatch for Anthropic Messages text requests.
- Claude Code upstream endpoint derivation: `{base_url}/messages?beta=true`.
- Claude Code official-client headers: `Anthropic-Beta`, `Anthropic-Version`, `X-App`, `X-Stainless-*`, `X-Claude-Code-Session-Id`, `x-client-request-id`, and stream/non-stream `Accept`.
- Claude Code system/billing blocks in the upstream Messages body.
- Reverse Proxy Runtime credential injection for OAuth/session/CLI-token runtime classes; `api_key` remains official API-key adapter behavior and is not a Claude Code 2api credential source.
- Focused adapter and Gateway regressions proving selected account credentials/UA reach upstream while caller/SRapi auth does not.

Definition of Done:

- `reverse-proxy-claude-code-cli` text requests call `base_url + "/messages?beta=true"` and must not send generic direct Anthropic API-key headers.
- Runtime receives selected account context and injects account credentials; caller auth must not be forwarded.
- `reverse-proxy-claude-code-cli` rejects `runtime_class = api_key` and requires OAuth/session/CLI-token style account credentials.
- Upstream body includes mapped upstream model, Anthropic Messages payload, Claude Code system/billing blocks, and no Gateway-local Claude DTOs.
- Gateway regression proves `/v1/messages` can schedule a Claude Code reverse-proxy account, send OAuth/CLI bearer upstream, and record Scheduler/usage evidence.
- This package does not implement Claude Code WebSocket/session slot lifecycle, full tool-name signing/remapping, or local Claude CLI client ingress.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-430: ChatGPT Web 2api Conversation Upstream Shape v1

Objective: make `reverse-proxy-chatgpt-web` construct the ChatGPT Web official-client Conversation shape and send it through Reverse Proxy Runtime with selected OAuth/session credentials, instead of treating ChatGPT Web 2api as generic OpenAI-compatible `/chat/completions`.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `apps/api/internal/modules/provider_adapters/contract`
- `apps/api/internal/modules/provider_adapters/service`
- `apps/api/internal/modules/reverse_proxy/contract`
- local reference: `/home/senran/Desktop/chatgpt2api/services/openai_backend_api.py`
- local reference: `/home/senran/Desktop/chatgpt2api/services/protocol/conversation.py`

Owns:

- ChatGPT Web reverse-proxy adapter dispatch for OpenAI-compatible text requests.
- ChatGPT Web upstream endpoint derivation: `{chatgpt_origin}/backend-api/conversation`.
- ChatGPT Web official-client headers: browser `Origin` / `Referer` / `Sec-*`, `OAI-Device-Id`, `OAI-Session-Id`, `OAI-Language`, `OAI-Client-Version`, `OAI-Client-Build-Number`, `X-OpenAI-Target-Path`, `X-OpenAI-Target-Route`, and Sentinel chat requirements headers.
- ChatGPT Web Conversation body shape: `action: next`, Web conversation `messages`, mapped upstream model, `parent_message_id`, `conversation_mode`, `force_use_sse`, timezone, `websocket_request_id`, and client contextual info.
- Reverse Proxy Runtime credential injection for OAuth/session/token runtime classes; `api_key` remains official API-key adapter behavior and is not a ChatGPT Web 2api credential source.
- ChatGPT Web SSE/JSON parsing from conversation payloads into `TextResponse`.
- Focused adapter and Gateway regressions proving selected account credentials/UA reach upstream while caller/SRapi auth does not.

Definition of Done:

- `reverse-proxy-chatgpt-web` text requests call `/backend-api/conversation` and must not call generic OpenAI-compatible `/chat/completions`.
- Runtime receives selected account context and injects account credentials; caller auth must not be forwarded.
- `reverse-proxy-chatgpt-web` rejects `runtime_class = api_key` and requires OAuth/session/client-token style account credentials.
- Upstream body includes mapped upstream model and ChatGPT Web Conversation payload with no Gateway-local ChatGPT DTOs.
- Adapter requires a Sentinel chat requirements token from credential/account metadata for v1; automatic bootstrap, PoW, Turnstile, and requirements fetching remain follow-up work.
- Gateway regression proves `/v1/chat/completions` can schedule a ChatGPT Web reverse-proxy account, send OAuth bearer upstream, and record Scheduler/usage evidence.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-440: ChatGPT Web Sentinel Requirements Auto Fetch v1

Objective: make `reverse-proxy-chatgpt-web` obtain ChatGPT Web Sentinel chat requirements through the same selected-account Reverse Proxy Runtime path when a static requirements token is not configured, so ChatGPT Web 2api does not require operators to pre-inject short-lived request tokens for every text call.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `apps/api/internal/modules/provider_adapters/service/chatgpt_web.go`
- `apps/api/internal/modules/reverse_proxy/contract`
- local reference: `/home/senran/Desktop/chatgpt2api/services/openai_backend_api.py`
- local reference: `/home/senran/Desktop/chatgpt2api/utils/pow.py`

Owns:

- ChatGPT Web homepage bootstrap through Reverse Proxy Runtime to collect PoW script/build context.
- ChatGPT Web legacy requirements `p` body generation and `/backend-api/sentinel/chat-requirements` request shape.
- Optional PoW proof token generation for requirements responses that require `proofofwork`.
- Challenge classification for Arkose and Turnstile requirements when an external token is not already configured.
- Conversation request header merge using auto-fetched requirements token, proof token, turnstile token, and SO token.
- Focused adapter and Gateway regressions proving bootstrap, requirements, and conversation all use the selected account runtime identity.

Definition of Done:

- Missing `chatgpt_requirements_token` no longer fails by default; the adapter first sends official-client bootstrap and requirements requests through Reverse Proxy Runtime.
- Operators can still disable auto fetch with `chatgpt_requirements_auto=false`, in which case missing requirements remains a clear `invalid_request`.
- Automatic requirements fetch must not add Gateway-local DTOs or bypass Reverse Proxy Runtime.
- Arkose requirements map to `challenge_required`; missing Turnstile token maps to `captcha_required`.
- Gateway regression proves `/v1/chat/completions` can schedule a ChatGPT Web account without a static requirements token and still record Scheduler/usage evidence.
- This package does not implement external Turnstile/Arkose solving, challenge token persistence, or browser TLS impersonation.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-450: Antigravity Official-Client Upstream Shape v1

Objective: make `reverse-proxy-antigravity` construct the Antigravity / Google Cloud Code official-client `v1internal` upstream shape through Reverse Proxy Runtime, instead of treating Antigravity 2api as generic OpenAI/Anthropic/Gemini compatible upstream HTTP.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `apps/api/internal/modules/provider_adapters/contract`
- `apps/api/internal/modules/provider_adapters/service`
- `apps/api/internal/modules/reverse_proxy/contract`
- local reference: `/home/senran/Desktop/CLIProxyAPI/internal/runtime/executor/antigravity_executor.go`
- local reference: `/home/senran/Desktop/sub2api/backend/internal/pkg/antigravity/request_transformer.go`
- local reference: `/home/senran/Desktop/sub2api/backend/internal/service/antigravity_gateway_service.go`

Owns:

- Antigravity reverse-proxy adapter dispatch for text requests.
- Antigravity upstream endpoint derivation: `{base_url}/v1internal:generateContent` and `{base_url}/v1internal:streamGenerateContent?alt=sse`.
- Antigravity official-client request envelope: `project`, `requestId`, `userAgent`, `requestType`, `model`, and nested Gemini `request`.
- OpenAI-compatible, Anthropic-compatible, and Gemini-compatible canonical text inputs mapped into the nested Gemini request without Gateway-local Antigravity DTOs.
- Reverse Proxy Runtime credential injection for desktop/IDE/OAuth/client-token runtime classes; `api_key` remains official API-key adapter behavior and is not an Antigravity 2api credential source.
- v1internal response unwrapping and parsing into the caller's existing OpenAI/Anthropic/Gemini downstream response rendering path.
- Focused adapter and Gateway regressions proving selected account credentials reach upstream while caller/SRapi auth does not.

Definition of Done:

- `reverse-proxy-antigravity` text requests call Cloud Code `v1internal` endpoints and must not call generic `/chat/completions`, `/messages`, or public Gemini `models/{model}:generateContent` upstream paths.
- Runtime receives selected account context and injects account credentials; caller auth must not be forwarded.
- `reverse-proxy-antigravity` rejects `runtime_class = api_key` and requires OAuth/session/desktop/IDE/client-token style account credentials.
- Upstream body includes mapped upstream model, configured `project_id`, generated Antigravity request/session IDs, and nested Gemini-compatible request payload with no SRapi internal fields.
- Gateway regression proves `/v1/chat/completions` can schedule an Antigravity reverse-proxy account, send desktop bearer upstream to `/v1internal:generateContent`, and record Scheduler/usage evidence.
- This package does not implement Antigravity OAuth onboarding, project discovery, credit overage retry policy, full tool-schema cleaning, or persistent realtime session lifecycle.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-460: Realtime Slot Lifecycle v1

Objective: make Responses WebSocket and future provider-native realtime adapters use an explicit backend realtime slot lifecycle instead of relying on an implicit handler loop, while preserving the Gateway -> Scheduler -> Provider Adapter -> Reverse Proxy Runtime evidence path.

Read first:

- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `apps/api/internal/httpserver/runtime_gateway_websocket.go`
- `apps/api/internal/modules/reverse_proxy/contract`
- `apps/api/internal/modules/provider_adapters/contract`

Owns:

- Backend realtime slot manager contract/service with acquire/release lifecycle.
- Responses WebSocket runtime integration with slot acquisition before WebSocket accept and release on close/error.
- Deploy-level realtime WebSocket slot limits, including global and per-API-key limits.
- Realtime lifecycle metrics for active/acquired/released/rejected slots.
- Focused unit and HTTP regressions proving limit enforcement and lifecycle release.

Definition of Done:

- `/v1/responses/ws` acquires a realtime slot after Gateway API key auth and before accepting the WebSocket.
- Slots record request id, user/API key ids, source endpoint, sticky account hint, sticky strength, and hashed session affinity key; raw affinity keys must not be stored.
- Slots are released on normal close, client disconnect, upstream relay completion, or handler error.
- Global and per-API-key slot limits reject excess WebSocket handshakes with a gateway 429 response before the upgrade.
- `/metrics` exposes active, acquired, released, and rejected realtime slot counters/gauges.
- Slot management remains provider-neutral and does not add Gateway-local provider DTOs.
- This package does not implement Claude Code or Antigravity provider-native realtime adapters, persistent upstream session reuse, or distributed Redis-backed slot storage.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/realtime/... ./internal/httpserver -run 'TestGatewayResponsesWebSocket|TestGatewayResponsesWebSocketEnforcesRealtimeSlotLimit'`
- `cd apps/api && go test ./internal/httpserver ./internal/modules/realtime/... ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/...`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-480: Images Edits Runtime v1

Objective: add OpenAI-compatible image edits through the standard Gateway auth, model policy, entitlement, Scheduler, Provider Adapter, usage evidence, and generated contract path.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `packages/openapi/openapi.yaml`
- OpenAI official image edits API docs
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/images/edits` and OpenAI-compatible provider alias contract
- Gateway multipart image edit normalization/rendering
- Provider Adapter multipart image edit dispatch for OpenAI-compatible API-key and reverse-proxy accounts
- HTTP runtime handler/tests and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/images/edits` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/images/edits`, reuse the same runtime while forcing provider context.
- Requests minimally validate `model`, `prompt`, and at least one image file; OpenAI SDK-style `image[]`, single `image`, optional `mask`, `n`, `size`, `quality`, `response_format`, `output_format`, `output_compression`, `background`, `moderation`, `input_fidelity`, and `user` fields are preserved where supported.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to multipart `/images/edits`, pass the mapped upstream model, parse `url` and `b64_json` image outputs, and return OpenAI-shaped image responses.
- The endpoint reuses the explicit `images` endpoint capability so Scheduler can reject text-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream multipart request/response parsing, and usage/decision evidence.
- JSON image references, streaming image edit events, image variations, and frontend visuals are left to later packages.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-490: Images Variations Runtime v1

Objective: add OpenAI-compatible image variations through the standard Gateway auth, model policy, entitlement, Scheduler, Provider Adapter, usage evidence, and generated contract path.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `packages/openapi/openapi.yaml`
- OpenAI official image variations OpenAPI spec
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/httpserver`

Owns:

- OpenAPI `POST /v1/images/variations` and OpenAI-compatible provider alias contract
- Gateway multipart image variation normalization/rendering
- Provider Adapter multipart image variation dispatch for OpenAI-compatible API-key and reverse-proxy accounts
- HTTP runtime handler/tests and Gateway route matrix/docs/status updates

Definition of Done:

- `POST /v1/images/variations` is OpenAPI-described, generated, and secured with `gatewayBearerAuth`.
- OpenAI-compatible provider alias routes, including `/api/provider/openai-compatible/v1/images/variations`, reuse the same runtime while forcing provider context.
- Requests minimally validate `model` and a single image file; `n`, `size`, `response_format`, `user`, and extra multipart form fields are preserved where supported.
- Runtime follows the standard Gateway path: API key auth, model visibility, entitlement admission, Scheduler candidate selection, provider credential materialization, Provider Adapter invocation, usage log, billing metadata, Scheduler feedback, and outbox event.
- OpenAI-compatible API-key and reverse-proxy accounts dispatch upstream to multipart `/images/variations`, pass the mapped upstream model, parse `url` and `b64_json` image outputs, and return OpenAI-shaped image responses.
- The endpoint reuses the explicit `images` endpoint capability so Scheduler can reject text-only candidates.
- Provider and validation errors use the existing OpenAI-compatible Gateway error envelope and preserve request IDs.
- Focused regressions prove standard route success, provider alias forced context, upstream multipart request/response parsing, and usage/decision evidence.
- Multi-image variations, JSON image references, streaming image variation events, and frontend visuals are left to later packages.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-500: Antigravity 2api Model Discovery v1

Objective: let operators discover Antigravity official-client upstream model catalogs for `reverse-proxy-antigravity` Provider Accounts through the Reverse Proxy Runtime, preserving the 2api boundary and reusing existing account `supported_models` routing metadata.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `/home/senran/Desktop/CLIProxyAPI/cmd/fetch_antigravity_models/main.go`
- `apps/api/internal/httpserver/model_discovery.go`
- `apps/api/internal/modules/reverse_proxy`

Owns:

- OpenAPI model discovery source enum extension for `reverse-proxy-antigravity`
- Admin account model discovery support for `reverse-proxy-antigravity` non-API-key accounts
- Antigravity upstream discovery endpoint derivation: `{base_url}/v1internal:fetchAvailableModels`
- Reverse Proxy Runtime dispatch using selected account OAuth/desktop/IDE credentials and default Antigravity upstream-client identity
- Antigravity model response parsing and persistence to account `supported_models`
- Focused HTTP regressions proving credential/header hygiene, preview/persist behavior, and API-key rejection for the 2api path

Definition of Done:

- `POST /api/v1/admin/accounts/{id}/discover-models` supports `reverse-proxy-antigravity` accounts with `runtime_class != api_key` and rejects API-key Antigravity 2api discovery.
- Discovery calls Antigravity / Google Cloud Code `{base_url}/v1internal:fetchAvailableModels` with `POST` and a JSON body containing configured `project_id` when present.
- Discovery uses Reverse Proxy Runtime so selected account credentials are injected by runtime; caller authorization, cookies, SRapi request ids, and gateway headers are not forwarded upstream.
- Antigravity discovery parses model IDs from the upstream `models` object, filters known internal preview IDs copied from the local CLIProxyAPI reference, normalizes/deduplicates/sorts IDs, and obeys the existing `limit`.
- `persist=true` writes `supported_models`, `model_discovery_source=reverse-proxy-antigravity`, `model_discovery_endpoint`, and `model_discovery_last_seen_at` to account metadata, so Scheduler filtering continues through existing Provider-neutral rules.
- Existing OpenAI/Anthropic/Gemini API-key discovery behavior remains unchanged.
- This package does not implement Antigravity OAuth onboarding, project discovery/onboardUser, credit overage retry policy, full tool-schema cleaning, or persistent realtime session lifecycle.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver -run 'TestAdminAccountModelDiscovery' -count=1`
- `cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-510: Images Edits JSON References v1

Objective: extend the existing OpenAI-compatible `/v1/images/edits` runtime so JSON callers can provide local image references without bypassing Gateway auth, model policy, Scheduler, Provider Adapter, usage evidence, or the OpenAPI-first contract.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `packages/openapi/openapi.yaml`
- `/home/senran/Desktop/chatgpt2api/test/test_v1_images_edits_json.py`
- `/home/senran/Desktop/chatgpt2api/test/test_v1_images_edits_api.py`
- `apps/api/internal/httpserver/runtime_gateway_media_handlers.go`
- `apps/api/internal/modules/gateway`
- `apps/api/internal/modules/provider_adapters`

Owns:

- OpenAPI schema support for JSON image edit references alongside existing multipart image edits
- HTTP image edit JSON decoder for `image`, `images`, `image_url`, and `b64_json` local references
- Runtime rejection for remote `image_url` and `file_id` references until an explicit Files/remote-fetch package exists
- Focused HTTP regressions proving JSON references become upstream multipart image edit calls and existing multipart behavior remains unchanged
- Docs/status updates for the compatibility boundary

Definition of Done:

- `POST /v1/images/edits` and OpenAI-compatible provider alias routes accept `application/json` bodies in addition to multipart form-data.
- JSON callers can send a single `image` or multiple `images` using data URLs, `{ "image_url": "data:..." }`, `{ "image_url": { "url": "data:..." } }`, or `{ "b64_json": "...", "mime_type": "...", "filename": "..." }`.
- JSON image references are decoded into the existing canonical image edit request and then forwarded upstream as multipart `/images/edits`; no new Gateway-local DTO path is added.
- Remote HTTP(S) image URLs and `file_id` references fail with explicit 400 errors because SRapi does not yet have the Files API / remote-fetch security boundary for this endpoint.
- `model`, `prompt`, `n`, `size`, `quality`, `response_format`, output options, `stream`, `partial_images`, `user`, and extra JSON properties preserve current behavior where applicable.
- Existing multipart `image`, `image[]`, and `mask` image edit requests continue to pass unchanged.
- This package does not implement image edit SSE streaming, remote URL fetching, Files API lookup, persistent image storage, or frontend visuals.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver -run 'TestGatewayImageEdit' -count=1`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-520: Images Edits Streaming Events v1

Objective: add OpenAI-compatible image edit streaming events to the existing `/v1/images/edits` runtime without introducing a new provider-specific shortcut or bypassing Gateway auth, Scheduler, Provider Adapter, usage evidence, or the current multipart/JSON compatibility behavior.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `packages/openapi/openapi.yaml`
- `/home/senran/Desktop/chatgpt2api/test/test_v1_images_edits.py`
- `/home/senran/Desktop/chatgpt2api/services/protocol/conversation.py`
- `apps/api/internal/httpserver/runtime_gateway_media_handlers.go`
- `apps/api/internal/modules/provider_adapters/service/image_edits.go`
- `apps/api/internal/modules/provider_adapters/service/service.go`

Owns:

- OpenAPI description of image edit SSE/streaming behavior and request fields already reserved for streaming
- HTTP runtime support for `stream=true` and `partial_images` on `/v1/images/edits`
- Provider adapter support for streaming image edit outputs and SSE forwarding where upstream supports it
- Focused regressions proving streaming and non-streaming image edit paths both preserve Gateway evidence and return OpenAI-shaped outputs
- Docs/status updates for the streaming compatibility boundary

Definition of Done:

- `POST /v1/images/edits` accepts `stream=true` with multipart and JSON bodies and returns `text/event-stream` for streaming calls.
- `partial_images` is honored or preserved as supported by the existing upstream path, with explicit rejection if an upstream/provider cannot handle it.
- Streaming image edit events are relayed or synthesized through the same Gateway auth, model policy, entitlement, Scheduler, Provider Adapter, usage, billing, and feedback path as non-streaming edits.
- Existing non-streaming image edit behavior remains unchanged, including JSON local references from WP-510.
- Remote URL and `file_id` rejection behavior from WP-510 remains unchanged.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver -run 'TestGatewayImageEdit' -count=1`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-530: Antigravity Project Bootstrap v1

Objective: let configured Antigravity 2api accounts discover their Cloud Code project through the same selected-account OAuth/desktop/IDE credential path used by the local 2api reference implementations, so operators do not have to hand-fill `project_id` before model discovery.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `/home/senran/Desktop/CLIProxyAPI/internal/auth/antigravity/auth.go`
- `/home/senran/Desktop/sub2api/backend/internal/pkg/antigravity/client.go`
- `apps/api/internal/httpserver/model_discovery.go`
- `apps/api/internal/modules/reverse_proxy`
- `apps/api/internal/modules/accounts`

Owns:

- Antigravity model discovery bootstrap behavior for accounts missing `project_id`.
- Reverse Proxy Runtime calls to Antigravity `/v1internal:loadCodeAssist` and `/v1internal:onboardUser`.
- Account metadata persistence of discovered `project_id` only when the admin discovery request asks to persist results.
- Focused HTTP regressions proving selected account credentials are used upstream and caller credentials/SRapi headers do not leak.
- 2api/reverse-proxy docs and status updates.

Definition of Done:

- `reverse-proxy-antigravity` discovery first uses existing `project_id`, `antigravity_project_id`, or `cloudaicompanion_project` metadata when present.
- If no project metadata exists, discovery posts `loadCodeAssist` through Reverse Proxy Runtime using the selected account credential and Antigravity official-client metadata.
- If `loadCodeAssist` has no project, discovery posts `onboardUser` through Reverse Proxy Runtime with a tier derived from `allowedTiers`, `currentTier`, or `free-tier`, then uses the returned project.
- `persist=false` discovery does not mutate account metadata; `persist=true` persists the bootstrapped project plus model discovery metadata.
- API-key runtime accounts remain rejected for Antigravity 2api discovery.
- No local Antigravity process is invoked, no Gateway-local Antigravity DTO is introduced, and no frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/httpserver -run 'TestAdminAccountModelDiscovery.*Antigravity|TestGatewayAntigravity' -count=1`
- `cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-540: Gemini Native Models List v1

Objective: implement Gemini-compatible `GET /v1beta/models` model listing so Gemini SDK-style clients can inspect visible SRapi models through a Google-shaped response without bypassing Gateway API key policy or introducing provider-specific account selection in the handler.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `packages/openapi/openapi.yaml`
- Gemini official `models.list` REST contract
- `apps/api/internal/httpserver/runtime_gateway_handlers.go`
- `apps/api/internal/httpserver/runtime_api_mapping.go`
- `apps/api/internal/modules/models`

Owns:

- OpenAPI `GET /v1beta/models` contract and generated Go/TypeScript SDK drift.
- Gemini-native list handler with Gateway API key auth and allowed-model filtering.
- Google-shaped model list renderer using SRapi model registry metadata.
- Route matrix / endpoint compatibility docs and focused HTTP regressions.

Definition of Done:

- `GET /v1beta/models` is OpenAPI-described, secured with `gatewayBearerAuth`, and returns `{ "models": [...], "nextPageToken": "..." }`.
- The handler authenticates Gateway API keys and renders Google-style error objects for invalid, disabled, or expired keys.
- The response only includes active models visible to the API key and presents names as `models/{canonical_name}`.
- `pageSize` and `pageToken` query parameters provide deterministic pagination; invalid pagination returns Gemini `INVALID_ARGUMENT`.
- Gemini model objects include stable SRapi-derived fields: `name`, `baseModelId`, `version`, `displayName`, `inputTokenLimit`, `outputTokenLimit`, and `supportedGenerationMethods`.
- `supportedGenerationMethods` is derived from SRapi model capabilities and includes at least `generateContent` for text-capable models, plus `streamGenerateContent` for streaming-capable models and `countTokens` for token-counting-capable models.
- No Scheduler lease is acquired, no Provider Account credential is touched, and no frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver -run 'TestGatewayGeminiListModels' -count=1`
- `cd apps/api && go test ./internal/httpserver ./internal/modules/gateway/...`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-550: Gemini Native Count Tokens v1

Objective: implement Gemini-compatible `POST /v1beta/models/{model}:countTokens` so Gemini SDK-style clients can count prompt tokens through SRapi without treating token counting as generation usage or bypassing Provider Account scheduling.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `packages/openapi/openapi.yaml`
- Gemini official `models.countTokens` REST contract
- `apps/api/internal/httpserver/runtime_gateway_handlers.go`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/modules/gateway`

Owns:

- OpenAPI `POST /v1beta/models/{model}:countTokens` contract and generated Go/TypeScript SDK drift.
- Gemini countTokens Gateway handler with API key auth, model visibility, entitlement, Scheduler, and Google-style errors.
- Provider Adapter dispatch to Gemini `models/{mapped_model}:countTokens` for API-key and reverse-proxy Gemini accounts.
- `token_counting` capability taxonomy and Scheduler filtering for countTokens requests.
- Route matrix / endpoint compatibility docs and focused adapter + HTTP regressions.

Definition of Done:

- `POST /v1beta/models/{model}:countTokens` is OpenAPI-described, secured with `gatewayBearerAuth`, and returns Google-shaped `GeminiCountTokensResponse`.
- Requests accept top-level Gemini countTokens fields or `generateContentRequest`.
- Gateway normalization is only used for policy, entitlement, Scheduler, and evidence; Provider Adapter forwards the original countTokens body to upstream and does not create Gateway-local provider DTOs.
- Gemini API-key accounts inject credentials according to Gemini auth mode.
- `runtime_class != api_key` Gemini accounts use Reverse Proxy Runtime with the selected account credentials.
- Scheduler requires `token_counting.v1`, and models/accounts without that capability are rejected before upstream dispatch.
- Successful countTokens requests record Scheduler decision/feedback and request evidence, but generation usage tokens and cost remain 0.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver -run 'TestGatewayGeminiCountTokens' -count=1`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-560: Anthropic Messages Count Tokens v1

Objective: implement Anthropic-compatible `POST /v1/messages/count_tokens` so Anthropic SDK-style clients can count prompt tokens through SRapi without treating token counting as generation usage or bypassing Provider Account scheduling.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/CAPABILITY_TAXONOMY_SPEC.md`
- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `packages/openapi/openapi.yaml`
- Anthropic official Messages Count Tokens API contract
- `apps/api/internal/httpserver/runtime_gateway_handlers.go`
- `apps/api/internal/modules/provider_adapters`
- `apps/api/internal/modules/gateway`

Owns:

- OpenAPI `POST /v1/messages/count_tokens` contract and generated Go/TypeScript SDK drift.
- Anthropic count_tokens Gateway handler with API key auth, model visibility, entitlement, Scheduler, and Anthropic-style errors.
- Provider Adapter dispatch to Anthropic `/messages/count_tokens` for API-key and reverse-proxy Anthropic accounts.
- `token_counting` capability taxonomy usage and Scheduler filtering for Anthropic count_tokens requests.
- Route matrix / endpoint compatibility docs and focused adapter + HTTP regressions.

Definition of Done:

- `POST /v1/messages/count_tokens` is OpenAPI-described, secured with `gatewayBearerAuth`, and returns Anthropic-shaped `{ "input_tokens": N }`.
- Requests accept Anthropic Messages-style count body with `model`, `messages`, `system`, `tools`, `tool_choice`, `thinking`, and compatible additional properties.
- Gateway normalization is only used for policy, entitlement, Scheduler, and evidence; Provider Adapter preserves the Anthropic count_tokens body shape, replaces only `model` with the mapped upstream model, and does not create Gateway-local provider DTOs.
- Anthropic API-key accounts inject credentials according to Anthropic auth mode.
- `runtime_class != api_key` Anthropic accounts use Reverse Proxy Runtime with the selected account credentials.
- Scheduler requires `token_counting.v1`, and models/accounts without that capability are rejected before upstream dispatch.
- Successful count_tokens requests record Scheduler decision/feedback and request evidence, but generation usage tokens and cost remain 0.
- No frontend visuals are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/httpserver -run 'TestGatewayAnthropicCountTokens' -count=1`
- `cd apps/api && go test ./internal/modules/gateway/... ./internal/modules/provider_adapters/... ./internal/httpserver`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-570: Realtime Active Slot Admin API v1

Objective: expose a safe AdminOps read API for current realtime WebSocket slots so operators can inspect active `/v1/responses/ws` and `/v1/realtime` lifecycle state without introducing provider DTOs, local client ingress, or credential-bearing output.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/OBSERVABILITY_SPEC.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/realtime/contract`
- `apps/api/internal/modules/realtime/service`
- `apps/api/internal/httpserver/runtime_gateway_websocket.go`
- `apps/api/internal/httpserver/runtime_admin_control_handlers.go`

Owns:

- OpenAPI `GET /api/v1/admin/ops/realtime/slots` contract and generated Go/TypeScript SDK drift.
- Realtime module read contract for current in-process active slots and aggregate counters.
- AdminOps HTTP handler that returns safe slot summaries under console cookie auth.
- Docs that define the endpoint as current-node operational state, not a distributed persistent session pool.
- Focused service and HTTP regressions.

Definition of Done:

- Admins can list active realtime slots with slot id, kind, request id, user id, API key id, source endpoint, acquisition time, sanitized session-affinity metadata, sticky account id, and sticky strength.
- Response includes aggregate active/acquired/released/rejected counts and active counts by endpoint, slot kind, and API key id.
- Raw session affinity keys, caller authorization, cookies, upstream credentials, prompt payloads, and provider-specific realtime frames are never returned.
- The endpoint is read-only, uses `cookieAuth`, and requires no CSRF token.
- The implementation keeps realtime lifecycle logic inside the realtime module and HTTP rendering inside `internal/httpserver`.
- No frontend visuals and no local Codex / Claude Code / Antigravity ingress are added.

Required gates:

- `make openapi-lint`
- `make openapi-bundle`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/realtime/... ./internal/httpserver -run 'TestRealtime|TestGateway.*Realtime|TestAdminOpsRealtime' -count=1`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-580: SDK Examples And 2api Migration Guides v1

Objective: add the first public integration examples and migration guides so operators and client developers can use SRapi's generated SDK, OpenAI/Anthropic/Gemini-compatible Gateway routes, and 2api account concepts without relying on frontend visuals or reverse-engineering tests.

Read first:

- `README.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `specs/FINAL_STATE.md`
- `specs/QUALITY_GATES.md`
- `packages/sdk/typescript/src`
- `tools/smoke-local.mjs`

Owns:

- `examples/` developer-facing examples for curl, TypeScript SDK, and Python requests.
- `docs/MIGRATION_GUIDE_2API.md` for sub2api / CLIProxyAPI / chatgpt2api style deployment migration.
- README and docs index links to the examples and migration guide.
- A lightweight examples quality harness that checks examples for route drift, required environment variables, and forbidden real-secret placeholders.
- Specs status and work package ledger updates.

Definition of Done:

- Examples show safe local usage with `SRAPI_BASE_URL`, `SRAPI_API_KEY`, and optional admin cookie/CSRF variables, never real tokens.
- Examples cover at least `/v1/models`, `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, Gemini `models.list` / `countTokens`, Anthropic `count_tokens`, and AdminOps realtime slot listing.
- Migration guide explicitly preserves SRapi's 2api definition: selected Provider Account OAuth/session/desktop/CLI/IDE credential to real upstream, not local Codex / Claude Code / Antigravity ingress and not Gateway-local provider DTOs.
- The examples-check harness is runnable with `make examples-check` and is included in package gates.
- No frontend visuals are added.

Required gates:

- `make examples-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-590: Distributed Realtime Slot Store v1

Objective: make realtime WebSocket slot lifecycle enforceable across API nodes by adding an optional Redis-backed realtime slot manager while preserving the Gateway -> realtime contract boundary and keeping provider-native protocol details out of slot state.

Read first:

- `docs/ARCHITECTURE.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `docs/OBSERVABILITY_SPEC.md`
- `docs/CONFIGURATION_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `apps/api/internal/modules/realtime/contract`
- `apps/api/internal/modules/realtime/service`
- `apps/api/internal/httpserver/runtime_gateway_websocket.go`
- `apps/api/internal/httpserver/runtime_metrics.go`
- `apps/api/internal/app/app.go`
- `apps/api/internal/persistence/redisstore/scheduler`

Owns:

- Realtime module store abstraction so the service can use in-memory state or Redis-backed state without HTTP knowing which one is active.
- Redis realtime slot store under `apps/api/internal/persistence/redisstore/realtime`.
- App/httpserver wiring that uses Redis for realtime slots when Redis is reachable, requires it in release mode, and falls back to in-memory state in local mode.
- Metrics/AdminOps slot listing behavior over the selected realtime manager.
- Docs/specs status for distributed active slot semantics and remaining non-goals.

Definition of Done:

- Two realtime service instances sharing the Redis store enforce global and per-API-key slot limits together.
- Releasing a slot from another instance frees capacity and preserves released counters.
- Expired slots no longer count as active and are marked released/expired without exposing provider-specific frames or credentials.
- `GET /api/v1/admin/ops/realtime/slots` can read distributed active slots through the same realtime manager contract.
- Release mode fails startup if Redis-backed realtime slots cannot be initialized; local mode logs fallback and keeps using in-memory slots.
- No local Codex / Claude Code / Antigravity ingress, no provider-native realtime protocol adapter, and no persistent upstream session pool are added.

Required gates:

- `cd apps/api && go test ./internal/modules/realtime/... ./internal/persistence/redisstore/realtime/... ./internal/app ./internal/httpserver -run 'TestRealtime|TestRedisRealtime|TestRealtimeSlot|TestAdminOpsRealtime' -count=1`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-500+: Ecosystem And Remaining Advanced Surface

Use `ROADMAP.md` Phase 7 through Phase 8 to split future packages for:

- provider-native realtime protocol adapters and richer slot lifecycle
- Codex / Claude Code / Antigravity OAuth and refresh-token-only account import flows

Each new package must be added here before implementation starts.

## WP-600: Codex Refresh Token Import And OAuth Lifecycle v1

Objective: make Codex 2api account onboarding match the sub2api-style operator flow where an operator can import a Codex OAuth refresh token, SRapi derives and persists the encrypted access-token state needed for official-client upstream requests, and Gateway requests can immediately schedule that Provider Account without local Codex CLI ingress or Gateway-local DTOs.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/MIGRATION_GUIDE_2API.md`
- `apps/api/internal/modules/accounts/service`
- `apps/api/internal/modules/reverse_proxy/service`
- `apps/api/internal/modules/provider_adapters/service/codex.go`
- `apps/api/internal/httpserver/runtime_admin_catalog_handlers.go`
- `/home/senran/Desktop/sub2api`
- `/home/senran/Desktop/CLIProxyAPI`
- `/home/senran/Desktop/chatgpt2api`

Owns:

- Codex OAuth credential normalization for `runtime_class=oauth_refresh` Provider Accounts.
- Real refresh-token-to-access-token exchange path through Reverse Proxy Runtime HTTP client boundaries.
- Import/update validation that accepts refresh-token-only Codex credentials, encrypts the resulting credential, and never returns tokens in API responses/audit.
- Refresh persistence, failure handling, and Scheduler account health evidence for Codex refresh failures.
- Tests that prove a refresh-token-only Codex import can request Codex `/responses` through the selected Provider Account official-client shape.

Definition of Done:

- Admin import/create/update accepts a Codex `refresh_token` without an initial `access_token` and obtains the first access token before the account is marked usable.
- Gateway `/v1/responses` using `reverse-proxy-codex-cli` dispatches to Codex `/responses` with selected-account OAuth credentials derived from the imported refresh token.
- Expired Codex access tokens refresh with a per-account distributed lock or an equivalent single-writer guard; failures do not overwrite the previous credential.
- Credential responses, audit records, Scheduler decisions, usage logs, and metrics never expose refresh/access tokens.
- No local Codex CLI client ingress, no Gateway-local Codex DTO, and no caller header/cookie passthrough are added.

Required gates:

- `cd apps/api && go test ./internal/modules/accounts/... ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/httpserver -run 'Test.*Codex.*Refresh|Test.*Codex.*Import|TestGateway.*Codex' -count=1`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-610: Claude Code Refresh Token Import And OAuth Lifecycle v1

Objective: extend the refresh-token-only onboarding pattern from Codex to Claude Code 2api accounts, preserving the same selected Provider Account -> Provider Adapter -> Reverse Proxy Runtime boundary and avoiding local Claude Code client ingress.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/MIGRATION_GUIDE_2API.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `apps/api/internal/modules/reverse_proxy/service`
- `apps/api/internal/modules/provider_adapters/service/claude_code.go`
- `apps/api/internal/httpserver/runtime_admin_catalog_handlers.go`
- `/home/senran/Desktop/CLIProxyAPI/internal/auth/claude`
- `/home/senran/Desktop/sub2api/backend/internal/repository/claude_oauth_service.go`

Owns:

- Claude Code OAuth credential normalization for `runtime_class=oauth_refresh` Provider Accounts.
- Claude JSON token refresh request support through Reverse Proxy Runtime without leaking provider-specific DTOs into Gateway.
- Admin create/import/update validation for refresh-token-only Claude Code credentials.
- Gateway regression proving `/v1/messages` uses selected-account OAuth credentials derived from the imported refresh token.

Definition of Done:

- Admin create/import/update accepts Claude Code `refresh_token` without initial `access_token` and obtains the first access token before the account is usable.
- Gateway `/v1/messages` using `reverse-proxy-claude-code-cli` dispatches with selected-account OAuth credentials derived from the imported refresh token.
- Refresh failures do not overwrite previous credentials and do not leak access/refresh tokens in responses, audit, logs, metrics, usage, or Scheduler evidence.
- No local Claude Code client ingress and no Gateway-local Claude DTO are added.

Required gates:

- `cd apps/api && go test ./internal/modules/accounts/... ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/httpserver -run 'Test.*Claude.*Refresh|Test.*Claude.*Import|TestGateway.*Claude' -count=1`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-620: Antigravity Refresh Token Import And OAuth Lifecycle v1

Objective: extend the refresh-token-only onboarding pattern to Antigravity 2api accounts, preserving the selected Provider Account -> Provider Adapter -> Reverse Proxy Runtime boundary and avoiding local Antigravity client ingress.

Read first:

- `docs/2API_REVERSE_PROXY_DEFINITION.md`
- `docs/MIGRATION_GUIDE_2API.md`
- `docs/REVERSE_PROXY_SPEC.md`
- `apps/api/internal/modules/reverse_proxy/service`
- `apps/api/internal/modules/provider_adapters/service/antigravity.go`
- `apps/api/internal/httpserver/runtime_admin_catalog_handlers.go`
- `/home/senran/Desktop/CLIProxyAPI/internal/auth/antigravity`
- `/home/senran/Desktop/sub2api/backend/internal/pkg/antigravity`

Owns:

- Antigravity OAuth credential normalization for `runtime_class=oauth_refresh` Provider Accounts.
- Google OAuth form token refresh request support through Reverse Proxy Runtime, using encrypted credential `oauth_client_secret` / `client_secret` rather than hard-coded client secrets.
- Admin create/import/update validation for refresh-token-only Antigravity credentials.
- Gateway regression proving `/v1/chat/completions` can use selected-account OAuth credentials derived from the imported refresh token.

Definition of Done:

- Admin create/import/update accepts Antigravity `refresh_token` without initial `access_token` and obtains the first access token before the account is usable.
- Gateway text requests using `reverse-proxy-antigravity` dispatch with selected-account OAuth credentials derived from the imported refresh token.
- Refresh failures do not overwrite previous credentials and do not leak access/refresh tokens/client secrets in responses, audit, logs, metrics, usage, or Scheduler evidence.
- No local Antigravity client ingress, onboarding UI/API, or Gateway-local Antigravity DTO is added.

Required gates:

- `cd apps/api && go test ./internal/modules/accounts/... ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/httpserver -run 'Test.*Antigravity.*Refresh|Test.*Antigravity.*Import|TestGateway.*Antigravity' -count=1`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-630: OpenAI-compatible API-key Realtime WebSocket Relay v1

Objective: close the official API-key Realtime gap for trusted server-side `/v1/realtime` WebSocket clients while preserving SRapi's 2api boundary: `runtime_class = api_key` accounts use selected-account API-key credentials directly, and `reverse-proxy-*` / non-API-key accounts continue to use Reverse Proxy Runtime.

Read first:

- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/GATEWAY_ROUTE_MATRIX.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`
- official OpenAI Realtime WebSocket docs: `https://developers.openai.com/api/docs/guides/realtime-websocket`
- `apps/api/internal/httpserver/runtime_gateway_websocket.go`
- `apps/api/internal/modules/provider_adapters/service/realtime.go`

Owns:

- Provider Adapter `PrepareRealtime` support for OpenAI-compatible `runtime_class = api_key` accounts.
- Gateway direct upstream WebSocket relay for official API-key Realtime sessions.
- Header hygiene proving caller `Authorization`, `Cookie`, and SRapi headers do not define upstream identity.
- Scheduler/usage evidence preserving `/v1/realtime` source endpoint.
- Docs/spec governance clarifying API-key Realtime versus 2api Reverse Proxy Runtime.

Definition of Done:

- `/v1/realtime?model=...` can schedule an OpenAI-compatible API-key account with `realtime_websocket` capability and connect upstream to `/realtime?model=<mapped_upstream_model>`.
- The upstream `Authorization` header uses only selected account `api_key` / `openai_api_key`.
- `OpenAI-Safety-Identifier` is preserved, while caller `Authorization`, `Cookie`, `Sec-WebSocket-*`, `X-SRapi-*`, and Gateway headers do not reach upstream.
- Non-API-key OpenAI-compatible Realtime still uses Reverse Proxy Runtime, and `reverse-proxy-*` 2api adapters still reject `runtime_class = api_key`.
- Success paths preserve Scheduler decisions and usage logs with `/v1/realtime` source endpoint.
- Persistent upstream session pools, browser ephemeral token minting, local client ingress, and Claude Code / Antigravity provider-native realtime adapters remain out of scope.
- No frontend visuals are added.

Required gates:

- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/httpserver -run 'TestOpenAICompatiblePrepareRealtime|TestGatewayRealtimeWebSocketRelaysOpenAI' -count=1`
- `cd apps/api && go test ./internal/modules/provider_adapters/... ./internal/modules/reverse_proxy/... ./internal/httpserver -run 'Test.*Realtime|Test.*WebSocket' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `git diff --check`

## WP-700: Admin Control Plane v1

Objective: deliver SRapi's first complete management control-plane backend for
the console without copying sub2api implementation structure. The package adds
typed dashboard, ops monitoring, settings, announcement, redeem-code,
promo-code, and risk-control APIs that follow SRapi module contracts,
OpenAPI-first code generation, decimal-safe money rules, and safe audit
logging.

Read first:

- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/OBSERVABILITY_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/DATA_MODEL.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `packages/openapi/openapi.yaml`
- `/home/senran/Desktop/sub2api` for capability analysis only; do not copy code
  or reproduce its service shape.

Owns:

- OpenAPI contracts for Admin Control Plane v1 route families.
- Admin dashboard snapshot read model.
- Ops read-model endpoints for overview, throughput trend, error trend,
  error distribution, latency histogram, concurrency, system logs, alert
  events, and ops settings.
- Settings-backed admin-control module for low-frequency announcements,
  redeem codes, promo codes, risk-control config/logs, ops settings, and
  typed system settings.
- HTTP handlers under existing `runtime_admin_*.go` route families.
- Safe audit records for all admin writes.
- Generated Go OpenAPI types and TypeScript SDK.
- Focused service and HTTP tests.

Definition of Done:

- All requested Admin Control Plane v1 APIs are defined in OpenAPI and have
  stable operation IDs, tags, security schemes, error envelopes, and generated
  SDK types.
- Dashboard snapshot includes API key, account, request, user, token, cost,
  RPM/TPM, latency, active user, model distribution, token trend, and user usage
  trend sections.
- Ops monitoring endpoints are backed by existing usage/account/realtime/alert
  evidence and do not expose credentials, prompts, API keys, cookies, or raw
  provider payloads.
- Announcements, redeem codes, promo codes, risk-control config/logs, ops
  settings, and typed system settings are managed through module contracts.
- All write routes require CSRF and record safe audit logs.
- Financial fields use decimal strings and currency, not float.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-codegen`
- `make openapi-ts-codegen`
- `cd apps/api && go test ./...`
- `make architecture-check`
- `make code-quality-check`
- `make secret-scan`
- `make check`
- `git diff --check`

## WP-760: AdminOps Durable System Logs v1

Objective: finish the AdminOps system-log follow-up from the control-plane
phase by moving sanitized system logs out of settings-backed placeholder state
and into a durable, queryable, bounded-cleanup store.

Read first:

- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `docs/SECURITY_MODEL.md`
- `docs/MODULE_INTERFACE_CONTRACTS.md`
- `packages/openapi/openapi.yaml`

Owns:

- `ops_system_logs` Ent schema and incremental PostgreSQL migration.
- Admin-control service/store contract for recording, listing, and cleaning
  sanitized system-log events.
- In-memory and Ent-backed store implementations.
- `GET /api/v1/admin/ops/system-logs` filters for level, source, text query,
  and time range.
- `POST /api/v1/admin/ops/system-logs/cleanup` with CSRF, dry-run,
  `max_delete` caps, and safe audit summaries.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, HTTP, migration, and contract drift tests.

Definition of Done:

- System logs persist to `ops_system_logs` with indexed level/source/time and
  request/trace correlation fields.
- List responses include request and trace IDs and never include credentials,
  prompts, cookies, raw API keys, or provider-native frames.
- Cleanup rejects unbounded requests, supports dry-run, caps deletion volume,
  and records audit evidence without raw search strings or log bodies.
- Admin Control Plane and data-model docs no longer describe durable system
  logs as pending.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/admin_control/... ./internal/persistence/entstore/admincontrol ./internal/platform/db ./internal/httpserver -run 'TestSystemLogsRecordListAndCleanup|TestAdminOpsSystemLogsListAndCleanup|TestConsoleWriteRoutesRequireCSRF|Test(PostgresVersionedUpMigrationsMatchEntSchema|PostgresDownMigrationsCoverCreatedTables|PostgresIncrementalMigrationsArePairedAndContiguous|EntSchemaAppliesToEmptyDatabase)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-830: Current-User Profile Update

Objective: close the current-user profile management gap found during the
docs/sub2api comparison by adding a SRapi-native self-service profile update
flow. This intentionally uses SRapi's current User model and does not copy
sub2api's username/avatar/notify-email shape.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/users/contract/contract.go`
- `apps/api/internal/httpserver/runtime_user_handlers.go`

Owns:

- Current-user API:
  - `PATCH /api/v1/me`
- Users service profile update method that only edits user-owned profile
  fields.
- Explicit OpenAPI request schema allowlisting `name`.
- Audit-safe profile-update evidence.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused users service and HTTP tests for CSRF and mass-assignment protection.

Definition of Done:

- The route requires a valid console session and CSRF header.
- The request body can update `name` only.
- Attempts to include `email`, `roles`, `status`, balance, RPM limit, password,
  or other admin-managed fields are rejected and do not change those fields.
- Empty names are rejected; accepted names are trimmed and capped at 120
  characters.
- Email change, avatar URL, notification email, and auth identity binding remain
  explicit follow-ups because they need verification, dedicated schema, or OAuth
  provider flows.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/users/... ./internal/httpserver -run 'Test(UpdateProfile|UpdateCurrentUserProfile|Register)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-820: Current-User Password Change

Objective: close the next auth account-lifecycle gap found during the
docs/sub2api comparison by adding a SRapi-native current-user password change
flow. This does not copy sub2api's handler shape; it uses SRapi's cookie
session, CSRF, users service, hashed session store, and audit boundaries.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/users/contract/contract.go`
- `apps/api/internal/modules/auth/contract/contract.go`
- `apps/api/internal/httpserver/runtime_user_handlers.go`

Owns:

- Current-user API:
  - `POST /api/v1/me/password`
- Users service password replacement only after verifying the current password.
- Auth session store support for revoking active sessions by user id.
- Cookie clearing after successful password change.
- Audit-safe password-change evidence without passwords, password hashes,
  session cookies, or CSRF tokens.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused users/auth/HTTP/persistence tests.

Definition of Done:

- The route requires a valid console session and CSRF header.
- The request body includes only `current_password` and `new_password`; callers
  cannot specify a target user id.
- Wrong current password returns `401 UNAUTHORIZED` and leaves the session
  active.
- Successful password change updates the password hash, rejects the old
  password on subsequent login, accepts the new password, revokes active console
  sessions for the user, and clears the current cookie.
- Persistent auth sessions are revoked by `user_id` using existing indexed
  `auth_sessions` fields; no new migration is required.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/users/... ./internal/modules/auth/... ./internal/persistence/entstore/auth ./internal/httpserver -run 'Test(ChangeCurrentUserPassword|ChangePassword|LogoutUser|DeleteByUserID|Register|LoginCreatesSession|AuthenticatePassword)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-780: Current-User Announcement Inbox v1

Objective: close the announcement follow-up found during sub2api comparison by
adding SRapi-native current-user announcement delivery and read receipts without
copying sub2api frontend or storage internals.

Read first:

- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `packages/openapi/openapi.yaml`

Owns:

- `user_announcement_reads` Ent schema and incremental PostgreSQL migration.
- Admin Control service/store methods for visible current-user announcements
  and idempotent read receipts.
- Current-user APIs:
  - `GET /api/v1/me/announcements`
  - `POST /api/v1/me/announcements/{id}/read`
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, HTTP, migration, and contract drift tests.

Definition of Done:

- Users only see published announcements matching their role audience and time
  window.
- Read receipts are unique on `(user_id, announcement_id)` and do not store
  announcement bodies, emails, role snapshots, or delivery payloads.
- Updating an announcement after `read_at` makes it unread again for that user.
- Mark-read requires CSRF and returns 404 for invisible announcements.
- Data-model and control-plane docs no longer describe announcement read
  receipts as pending.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/admin_control/... ./internal/persistence/entstore/admincontrol ./internal/platform/db ./internal/httpserver -run 'Test(UserAnnouncementsFilterVisibleAndTrackReadState|CurrentUserAnnouncementsListAndReadState|ConsoleWriteRoutesRequireCSRF|PostgresVersionedUpMigrationsMatchEntSchema|PostgresDownMigrationsCoverCreatedTables|PostgresIncrementalMigrationsArePairedAndContiguous|EntSchemaAppliesToEmptyDatabase)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-790: Current-User Redeem Code Redemption v1

Objective: close the redeem-code follow-up found during sub2api comparison by
adding SRapi-native user-side redemption without copying sub2api storage or
route internals.

Read first:

- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `docs/PAYMENT_SPEC.md`
- `packages/openapi/openapi.yaml`

Owns:

- `user_redeem_code_redemptions` Ent schema and incremental PostgreSQL
  migration.
- Admin Control service/store methods for idempotent current-user redemption.
- Current-user API:
  - `POST /api/v1/me/redeem-codes/redeem`
- Balance redemption fulfillment into `users.balance` and
  `billing_ledger(type=redeem_code_credit)`.
- Subscription redemption fulfillment into `user_subscriptions` and materialized
  `entitlements`.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, HTTP, migration, and contract drift tests.

Definition of Done:

- Redemption requires a console session and CSRF token.
- Codes are normalized case-insensitively, honor disabled/expired/max
  redemption limits, and update `redeemed_count`.
- Repeating the same code by the same user returns the original receipt without
  duplicating balance credits, subscriptions, or ledger rows.
- Balance credits update user balance and billing ledger atomically in the
  persistent store.
- Subscription codes create a subscription and entitlement cache rows from the
  referenced active plan.
- Redemption receipts are unique on `(user_id, redeem_code_id)` and do not store
  the plaintext code.
- Data-model, OpenAPI, and control-plane docs no longer describe user redeem
  flow as pending.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/admin_control/... ./internal/persistence/entstore/admincontrol ./internal/platform/db ./internal/httpserver -run 'Test(RedeemCodeCreditsBalanceOnce|CurrentUserRedeemCodeCreditsBalanceOnce|PostgresVersionedUpMigrationsMatchEntSchema|PostgresDownMigrationsCoverCreatedTables|PostgresIncrementalMigrationsArePairedAndContiguous|EntSchemaAppliesToEmptyDatabase)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-800: Current-User Promo Code Application v1

Objective: close the promo-code follow-up found during the docs/sub2api
comparison by adding SRapi-native user-side promo application to payment order
creation without copying sub2api order or coupon internals.

Read first:

- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `docs/PAYMENT_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/payments/contract/contract.go`
- `apps/api/internal/modules/admin_control/contract/contract.go`

Owns:

- A durable promo-code application receipt table if order creation needs
  per-user/order idempotency beyond the settings-backed promo-code collection.
- Current-user payment-order request contract support for an optional promo
  code.
- Admin Control or Payment service logic that validates active promo codes,
  expiry, max uses, currency, and amount/percent discount rules before payment
  provider checkout creation.
- Atomic persistent updates for promo `used_count`, payment order discounted
  amount, and receipt evidence.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, HTTP, migration, and contract drift tests.

Definition of Done:

- Promo application requires a current-user payment order flow and the existing
  payment-order CSRF/idempotency expectations are preserved.
- Amount discounts cannot make an order negative and must match order currency.
- Percent discounts use decimal ratios and produce deterministic decimal-string
  order amounts.
- Disabled, expired, exhausted, wrong-currency, and malformed promo codes return
  explicit client errors before provider checkout creation.
- Reusing the same promo for the same committed order cannot increment
  `used_count` twice.
- Data-model, OpenAPI, payment, and control-plane docs no longer describe promo
  application as pending.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check` when a receipt table is added
- `make migration-check` when a receipt table is added
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/admin_control/... ./internal/modules/payments/... ./internal/persistence/entstore/admincontrol ./internal/platform/db ./internal/httpserver -run 'Test(PromoCode|PaymentOrder|PostgresVersionedUpMigrationsMatchEntSchema|PostgresDownMigrationsCoverCreatedTables|PostgresIncrementalMigrationsArePairedAndContiguous|EntSchemaAppliesToEmptyDatabase)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-770: Console TOTP 2FA v1

Objective: finish the Auth follow-up by adding SRapi-native current-user TOTP
enrollment and login second-factor verification without copying sub2api route
or storage internals.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/CONFIGURATION_SPEC.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `packages/openapi/openapi.yaml`

Owns:

- `user_totp_secrets` Ent schema and incremental PostgreSQL migration.
- TOTP service/store contract with memory and Ent-backed persistence.
- AES-GCM encrypted TOTP secret storage using `TOTP_ENCRYPTION_KEY`.
- Recovery code generation, HMAC-only storage, and one-time consumption.
- Login 2FA challenge flow: `POST /api/v1/auth/login` returns `202` when
  TOTP is enabled, and `POST /api/v1/auth/login/2fa` creates the session after
  TOTP or recovery-code verification.
- Current-user TOTP APIs: status, setup, enable, disable.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, HTTP, migration, and contract drift tests.

Definition of Done:

- Password-only login remains backward-compatible for users without TOTP.
- TOTP-enabled users do not receive a session cookie until second factor
  verification succeeds.
- Setup/enable/disable routes require CSRF and never log or persist plaintext
  recovery codes beyond the enable response.
- `TOTP_ENCRYPTION_KEY` is documented and release validation rejects weak
  values.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/... ./internal/modules/totp/... ./internal/persistence/entstore/totp ./internal/httpserver -run 'Test(LoginCreatesSessionAndTouchesUser|LoginRequiresSecondFactorWhenEnabled|CompleteSecondFactorLoginCreatesSession|SetupEnableAndVerifyTOTP|VerifyLoginConsumesRecoveryCode|CurrentUserTOTPSetupEnableAndTwoFactorLogin|ConsoleWriteRoutesRequireCSRF)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-810: Public Console Registration v1

Objective: close the auth registration gap found during the docs/sub2api
comparison by adding a small SRapi-native public registration flow without
copying sub2api auth routes or storage internals.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/users/contract/contract.go`
- `apps/api/internal/modules/auth/contract/contract.go`

Owns:

- Public auth API:
  - `POST /api/v1/auth/register`
- Admin settings gate using `security.registration_enabled`.
- Optional registration email suffix policy using
  `security.registration_email_suffix_allowlist`; values are normalized to exact
  `@domain.tld` suffixes and an empty list allows all valid email domains.
- Regular user creation through the existing users service with configured
  default balance and default RPM limit.
- Immediate console session creation using the existing session cookie and CSRF
  response shape.
- Generic duplicate/invalid registration errors to avoid account enumeration.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused HTTP, auth, user service, and contract drift tests.

Definition of Done:

- Registration is disabled with `403 FORBIDDEN` when admin settings disable it.
- Non-empty registration email suffix allowlists are normalized, reject invalid
  domains at settings update time, and block unmatched registration emails with
  the same generic registration error.
- Successful registration creates a regular `user` role account, sets the
  HttpOnly session cookie, and returns a CSRF token in `LoginResponse`.
- Duplicate email, suffix-policy rejection, and invalid input return the same
  generic `400 INVALID_REQUEST` response.
- Registration audit evidence never records plaintext password, session cookie,
  or CSRF token.
- Email verification and password reset remain explicit follow-ups until SRapi
  has mail/outbox delivery and hash-stored one-time token infrastructure.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/users/... ./internal/modules/auth/... ./internal/persistence/entstore/users ./internal/httpserver -run 'Test(Register|CreateHashesPasswordAndDefaultsRole|AuthenticatePassword|CreateRole|UpdateBalance|LoginCreatesSession|CurrentUser)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-860: Notification Email Dispatch Foundation v1

Objective: close the delivery gap left by WP-840/WP-850 by adding an
SRapi-native transactional email dispatcher for auth lifecycle events. This does
not copy sub2api's broad notification preference/template system; SRapi consumes
the existing domain-event outbox, keeps tokens encrypted until worker dispatch,
and keeps SMTP secrets in deployment env until encrypted settings secret storage
exists.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `docs/CONFIGURATION_SPEC.md`
- `apps/api/internal/workers/outbox`
- `apps/api/internal/modules/auth/service/password_reset.go`
- `apps/api/internal/modules/auth/service/email_verification.go`

Owns:

- `internal/modules/notifications` service and contract for auth transactional
  email rendering and sending.
- Outbox worker dispatch for:
  - `AuthPasswordResetRequested`
  - `AuthEmailVerificationRequested`
- SMTP sender adapter with TLS/STARTTLS support and no-auth local SMTP support.
- Deployment env config:
  - `EMAIL_PUBLIC_BASE_URL`
  - `EMAIL_SMTP_HOST`
  - `EMAIL_SMTP_PORT`
  - `EMAIL_SMTP_USERNAME`
  - `EMAIL_SMTP_PASSWORD`
  - `EMAIL_SMTP_FROM`
  - `EMAIL_SMTP_FROM_NAME`
  - `EMAIL_SMTP_USE_TLS`
- Admin Settings non-secret email metadata only; `smtp_password` is not accepted
  or persisted in Admin Settings/OpenAPI/SDK/audit, and
  `smtp_password_configured` is derived from runtime config in responses.
- Focused notification service, config, outbox, app wiring, HTTP contract, and
  OpenAPI/SDK drift tests.

Definition of Done:

- Password reset and email verification outbox events send rendered HTML mail
  only when public base URL, SMTP host, and sender are configured.
- Outbox payload remains secret-safe: no plaintext email, plaintext token,
  password, session cookie, CSRF token, or SMTP password.
- Worker decrypts the token only in memory and builds links from
  `EMAIL_PUBLIC_BASE_URL` plus the event's relative action path; it never uses
  request Host headers.
- Worker re-reads the current user before sending and skips inactive users or
  stale events whose recipient email hash no longer matches the current email.
- Missing email config leaves auth mail delivery retryable/failed instead of
  silently marking the event as published.
- SMTP header fields are CR/LF sanitized and local unauthenticated SMTP remains
  supported.
- OpenAPI and TypeScript SDK expose no `smtp_password` request/response field.
- Admin Settings update requests cannot set `smtp_password_configured`; the
  response flag comes from `EMAIL_SMTP_PASSWORD`.
- Broader user notification preferences, unsubscribe links, balance/subscription
  notifications, template preview/restore APIs, avatar storage, and OAuth
  identity onboarding remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/modules/admin_control/... ./internal/config ./internal/workers/outbox ./internal/app ./internal/httpserver -run 'Test(AuthPasswordResetEventSendsRenderedEmail|AuthEmailEventSkipsStaleRecipientHash|AuthEmailEventRequiresConfiguredSMTPAndBaseURL|UpdateAdminSettingsNormalizesEmailConfigWithoutSMTPSecret|UpdateAdminSettingsRejectsInvalidEmailPublicBaseURL|EmailConfigDefaultsOverridesAndValidation|WorkerRetriesAuthEmailWhenEmailDeliveryNotConfigured|Register|UpdateAdminSettings|EmailVerification|PasswordReset|UpdateAdminSettingsEmailDoesNotAcceptSMTPPassword|AdminSettingsEmailPasswordConfiguredComesFromRuntimeConfig)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-870: Notification Preferences and One-Click Unsubscribe v1

Objective: close the first notification-management gap after WP-860 by adding
SRapi-native optional notification unsubscribe primitives. This does not copy
sub2api's user-column JSON email list; SRapi stores event-scoped preference
state by recipient email hash and keeps transactional auth mail non-suppressible.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/notifications/contract/contract.go`
- `apps/api/internal/modules/notifications/service`

Owns:

- Public notification preference APIs:
  - `GET /api/v1/notifications/unsubscribe`
  - `POST /api/v1/notifications/unsubscribe`
- Signed unsubscribe token generation and validation with event, email hash,
  and expiry only.
- Event-scoped settings-backed preference storage while this remains low-volume.
- Optional email `List-Unsubscribe` and `List-Unsubscribe-Post` header support
  with a strict SMTP header allowlist.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service and HTTP tests.

Definition of Done:

- `GET` validates a token without mutating state; `POST` applies the preference.
- `POST` accepts token from query, JSON body, or form body to support one-click
  unsubscribe POSTs.
- Tokens and stored preference keys never include plaintext email, user id,
  session cookie, CSRF token, SMTP secret, or provider credential.
- Only optional notification events such as `balance.low` and
  `account.quota_alert` can be unsubscribed.
- Transactional auth templates such as `auth.password_reset` and
  `auth.email_verification` cannot generate unsubscribe tokens and are never
  suppressed by optional preferences.
- SMTP custom headers are limited to `List-Unsubscribe` and
  `List-Unsubscribe-Post` after CR/LF sanitization.
- Balance/subscription/account-quota triggers, template preview/restore APIs,
  avatar storage, OAuth identity binding/onboarding, and credential-gated SMTP
  smoke remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/httpserver -run 'TestNotificationUnsubscribeEndpoint|TestPreferenceService' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-880: Notification Email Template Management v1

Objective: close the notification-template control-plane gap found during the
docs/sub2api comparison by adding SRapi-native admin template list, detail,
preview, update, and restore APIs. This does not copy sub2api's locale-specific
settings shape; SRapi v1 stores event-level overrides in the existing typed
Admin Settings email template map and keeps the template catalog in the
notifications module so locale support can be added later without changing the
current key contract.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/ADMIN_CONTROL_PLANE_SPEC.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/notifications/contract/contract.go`
- `apps/api/internal/modules/notifications/service`
- `apps/api/internal/httpserver/runtime_admin_control_plane_handlers.go`

Owns:

- Admin notification template APIs:
  - `GET /api/v1/admin/notifications/email-templates`
  - `GET /api/v1/admin/notifications/email-templates/{event}`
  - `PUT /api/v1/admin/notifications/email-templates/{event}`
  - `POST /api/v1/admin/notifications/email-templates/{event}/restore`
  - `POST /api/v1/admin/notifications/email-template-preview`
- Template event catalog for `auth.password_reset`,
  `auth.email_verification`, `balance.low`, and `account.quota_alert`.
- Placeholder allowlists, subject/HTML size validation, safe preview rendering,
  and URL placeholder scheme checks.
- Admin Settings-backed override storage using `<event>.subject` and
  `<event>.html` keys.
- Safe audit records for template update and restore actions.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused notification service and admin HTTP tests.

Definition of Done:

- Admin reads require a console admin session; update, restore, and preview use
  `cookieAuth` plus `csrfHeader`.
- List returns event metadata, template details, and a placeholder union without
  SMTP secrets, recipient data, provider credentials, or unsubscribe tokens.
- Update rejects unknown events, empty templates, oversized templates, malformed
  placeholders, and placeholders outside the event allowlist.
- Preview renders without saving state, escapes variable values for HTML output,
  sanitizes subjects against header injection, and blanks unsafe URL placeholders
  such as `javascript:` URLs.
- Restore removes only the selected event's override keys and returns the
  built-in template.
- Transactional auth email delivery uses the same renderer while remaining
  non-suppressible by unsubscribe preferences.
- Balance/subscription/account-quota trigger scheduling, current-user
  preference management, avatar storage, OAuth identity binding/onboarding, and
  credential-gated SMTP smoke remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestAdminNotificationEmailTemplate|TestNotificationUnsubscribeEndpoint' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-910: Subscription Expiry Reminder Trigger v1

Objective: close the next notification-trigger gap found during the
docs/sub2api comparison by adding an SRapi-native subscription expiry reminder
trigger. This does not copy sub2api's service-local email sender; SRapi scans
active subscriptions, enqueues safe reminder domain events, and lets the
existing outbox notification dispatcher render, suppress, retry, and deliver
optional email with one-click unsubscribe support.

Read first:

- `docs/DOMAIN_EVENTS_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `apps/api/internal/workers/subscription_expirer/worker.go`
- `apps/api/internal/modules/subscriptions/service/service.go`
- `apps/api/internal/modules/notifications/service/service.go`

Owns:

- `SubscriptionExpiryReminderTriggered` domain event.
- Active subscription expiry reminder windows: 7 days, 3 days, and 1 day.
- Admin Settings email default:
  - `subscription_expiry_notify_enabled`
- Outbox notification handling for `subscription.expiry_reminder` with template
  rendering, one-click unsubscribe headers, and current-user preference
  suppression.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused subscription, notification, worker, HTTP preference, and admin
  settings tests.

Definition of Done:

- The subscription expirer worker keeps expiring overdue subscriptions and also
  enqueues reminder events for active subscriptions in the configured reminder
  windows when the global switch is enabled.
- Reminder events are idempotent per subscription and reminder key.
- Event payloads contain only safe operational fields: subscription id, user id,
  plan id/name, days remaining, reminder key, expiry timestamp, triggered
  timestamp, and console path.
- Payloads and idempotency keys do not include plaintext email, unsubscribe
  token, session cookie, CSRF token, SMTP secret, API key, provider credential,
  or prompt.
- Notification dispatch re-reads the current user, skips inactive users, honors
  `subscription.expiry_reminder` unsubscribe state, and attaches one-click
  unsubscribe headers when possible.
- SMTP/template failures remain outbox-retryable and never change subscription
  state.
- Avatar storage, OAuth identity binding/onboarding, and credential-gated SMTP
  smoke remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/modules/subscriptions/... ./internal/workers/subscription_expirer ./internal/workers/outbox ./internal/modules/admin_control/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Test(CurrentUserNotificationPreferences|NotificationUnsubscribeEndpoint)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-920: Account Quota Notification Trigger v1

Objective: close the account-quota notification-trigger gap found during the
docs/sub2api comparison by adding an SRapi-native quota alert trigger. This
does not copy sub2api's gateway-local goroutine mail sender or per-account
extra-field email list; SRapi scans persisted quota snapshots, enqueues safe
domain events, and lets the outbox notification dispatcher render, suppress,
retry, and deliver optional admin email.

Read first:

- `docs/DOMAIN_EVENTS_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `apps/api/internal/workers/account_quota_alert/worker.go`
- `apps/api/internal/modules/notifications/service/service.go`
- `apps/api/internal/modules/accounts/contract/contract.go`

Owns:

- `AccountQuotaAlertTriggered` domain event.
- Account quota snapshot threshold-crossing detection:
  `previous_remaining_ratio > threshold && latest_remaining_ratio <= threshold`.
- Admin Settings email defaults:
  - `account_quota_notify_enabled`
  - `account_quota_notify_remaining_ratio`
- Outbox notification handling for `account.quota_alert` with active owner/admin
  recipient selection, template rendering, one-click unsubscribe headers, and
  current-user preference suppression.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused notification, worker, admin settings, and outbox routing tests.

Definition of Done:

- The account quota alert worker scans active provider accounts and recent quota
  snapshots, then enqueues an idempotent event only on downward threshold
  crossing.
- Event idempotency is scoped by account, quota type, threshold, and reset/date
  bucket.
- Event payloads contain only safe operational fields: account id/name,
  provider id, runtime class, quota snapshot id, quota type, quota numbers,
  threshold, previous ratio, reset/snapshot/trigger timestamps, and console
  path.
- Payloads and idempotency keys do not include plaintext recipient email,
  unsubscribe token, session cookie, CSRF token, SMTP secret, API key, provider
  credential, or prompt.
- Notification dispatch selects active owner/admin users at send time, honors
  each user's `account.quota_alert` unsubscribe state, and attaches one-click
  unsubscribe headers when possible.
- SMTP/template failures remain outbox-retryable and never change account quota
  state or gateway request outcomes.
- Avatar storage, OAuth identity binding/onboarding, and credential-gated SMTP
  smoke remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/workers/account_quota_alert ./internal/workers/outbox ./internal/modules/admin_control/... ./internal/app -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-930: Verified Notification Contacts v1

Objective: close the verified-extra-notification-contact gap found during the
docs/sub2api comparison by adding an SRapi-native contact flow. This does not
copy sub2api's user JSON extra-email field or code-cache flow; SRapi stores
settings-backed contact state, verifies ownership with signed time-limited
tokens, routes verification mail through outbox, and includes only verified,
enabled contacts in optional notification delivery.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/SECURITY_MODEL.md`
- `docs/DOMAIN_EVENTS_SPEC.md`
- `apps/api/internal/modules/notifications/service/contacts.go`
- `apps/api/internal/modules/notifications/service/service.go`
- `apps/api/internal/httpserver/runtime_notification_handlers.go`

Owns:

- Current-user notification contact APIs:
  - `GET /api/v1/me/notification-contacts`
  - `POST /api/v1/me/notification-contacts`
  - `POST /api/v1/me/notification-contacts/verify`
  - `PATCH /api/v1/me/notification-contacts/{id}`
  - `DELETE /api/v1/me/notification-contacts/{id}`
- `NotificationContactVerificationRequested` domain event.
- Transactional `notification.contact_verification` email template.
- Optional notification dispatch expansion for verified, enabled contacts.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused notification service, HTTP lifecycle, and outbox routing tests.

Definition of Done:

- Users can add up to three secondary notification contacts, excluding their
  primary account email.
- Contact writes require current console session plus CSRF.
- Verification events contain only contact id, recipient email hash, encrypted
  contact email, encrypted verification token, action path, and expiry.
- Verification tokens are signed, time-limited, and checked against the stored
  contact token hash before marking a contact verified.
- Optional notification delivery includes the primary email plus verified,
  enabled secondary contacts, deduplicates addresses, and honors each
  recipient email's event-scoped unsubscribe state and one-click headers.
- Contact verification email is transactional and not suppressible by optional
  unsubscribe preferences.
- Payloads, idempotency keys, audit snapshots, and preference storage do not
  include plaintext unsubscribe token, session cookie, CSRF token, SMTP secret,
  API key, provider credential, or prompt.
- Avatar storage, OAuth identity binding/onboarding, and credential-gated SMTP
  smoke remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... ./internal/httpserver ./internal/workers/outbox -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-940: Current-User Avatar Storage v1

Objective: close the avatar-storage gap found during the docs/sub2api
comparison with an SRapi-native flow. This does not copy sub2api's profile
`avatar_url` setter, remote URL adoption, or data URL storage. SRapi keeps
profile updates field-allowlisted, accepts only authenticated current-user
avatar uploads, validates image bytes server-side, stores a normalized
SRapi-owned PNG, and serves it through a controlled API.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/SECURITY_MODEL.md`
- `docs/DATA_MODEL.md`
- `apps/api/internal/modules/users/service/avatar.go`
- `apps/api/internal/httpserver/runtime_user_handlers.go`

Owns:

- Current-user avatar APIs:
  - `PUT /api/v1/me/avatar`
  - `DELETE /api/v1/me/avatar`
  - `GET /api/v1/users/{id}/avatar`
- `GET /api/v1/me` avatar metadata (`avatar_url`, MIME, byte size, SHA-256,
  updated-at).
- Settings-backed avatar v1 storage under `users.avatar:v1:user:{user_id}`.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused users service and HTTP lifecycle tests.

Definition of Done:

- Avatar writes require current console session plus CSRF.
- Uploads accept only `multipart/form-data` field `avatar`.
- PNG/JPEG inputs are decoded, size-limited to 1 MiB, dimension-limited to
  1024x1024, re-encoded as PNG, and stored with sha256/size/dimension metadata.
- Remote URLs, SVG, arbitrary data URLs, browser filenames, and caller
  `Content-Type` are not trusted or stored.
- Avatar reads require a console session and return controlled `image/png`
  bytes with `ETag` and `X-Content-Type-Options: nosniff`.
- Audit snapshots and API responses do not include session cookie, CSRF token,
  original filename, API key, provider credential, prompt, or raw upload body.
- Objectstore/CDN-backed avatar storage remains a future promotion path if
  avatar traffic or storage volume exceeds the low-frequency settings-backed
  v1 design.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/users/service ./internal/httpserver -run 'TestAvatarService|TestCurrentUserAvatar|TestUpdateCurrentUserProfileRequiresCSRFAndAllowlistsFields' -count=1`
- `cd apps/api && go test ./internal/modules/users/... ./internal/httpserver -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-950: User Auth Identity Directory v1

Objective: close the first OAuth identity binding/onboarding gap found during
the docs/sub2api comparison by adding an SRapi-native current-user sign-in
identity directory. This does not copy sub2api's pending OAuth flow or raw
provider subject storage. SRapi first creates a durable, hash-only external
identity foundation that future OAuth/OIDC callbacks can write to safely.

Read first:

- `docs/OPENAPI_CONTRACT.md`
- `docs/SECURITY_MODEL.md`
- `docs/DATA_MODEL.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/users/contract/contract.go`
- `apps/api/internal/persistence/entstore/users/store.go`

Owns:

- Current-user API:
  - `GET /api/v1/me/auth-identities`
  - `DELETE /api/v1/me/auth-identities/{id}`
- `user_auth_identities` Ent schema and incremental PostgreSQL migration.
- Users contract/store/service support for derived local email identity plus
  external OAuth/OIDC identity records.
- Memory and Ent persistence support for future provider callback upserts.
- Generated OpenAPI Go types and TypeScript SDK.
- CSRF-protected external identity unbind by persistent identity id.
- Focused users service, Ent store, HTTP, migration, and contract drift tests.

Definition of Done:

- `GET /api/v1/me/auth-identities` requires current console session and returns
  a derived local email sign-in identity.
- `DELETE /api/v1/me/auth-identities/{id}` requires current console session plus
  CSRF, deletes only the current user's external identity with that exact id, and
  returns the refreshed identity list.
- The derived local email identity is not addressable for unbind; unbind refuses
  to remove the user's last available sign-in method.
- External identities are stored with provider, provider key, subject hash,
  display-safe subject hint, verified/profile metadata, and timestamps.
- Raw upstream subject, authorization code, access token, refresh token,
  session cookie, CSRF token, provider secret, API key, and credential payloads
  are never returned or persisted in the identity directory.
- OpenAPI and SDK expose stable `AuthIdentityProvider`,
  `CurrentUserAuthIdentity`, and list response schemas.
- OAuth/OIDC start/callback, pending decision sessions, profile adoption, and
  external identity bind mutation APIs remain follow-up packages built on this
  directory.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/users/... ./internal/persistence/entstore/users ./internal/httpserver -run 'Test(ListAuthIdentities|UnbindAuthIdentity|CurrentUserAuthIdentit)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-960: Pending OAuth Session Foundation v1

Objective: close the next OAuth onboarding gap found during the docs/sub2api
comparison by adding an SRapi-native pending decision session foundation. This
does not copy sub2api's browser-cookie/session flow or raw provider subject
storage. SRapi stores only a short-lived hash-only decision receipt that future
OAuth/OIDC start/callback, bind, create-account, and profile-adoption routes can
consume safely.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/DATA_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `apps/api/internal/modules/auth/contract/contract.go`
- `apps/api/internal/persistence/entstore/auth/store.go`
- `apps/api/ent/schema/userauthidentity.go`

Owns:

- `pending_oauth_sessions` Ent schema and incremental PostgreSQL migration.
- Auth contract/store/service support for creating and consuming pending OAuth
  sessions.
- Memory and Ent persistence implementations.
- HMAC hash-only pending session tokens using the existing auth server secret.
- Hashed provider subject storage and display-safe profile summaries.
- Single-use consume semantics with `consumed_at` and expiry checks.
- Data model, security model, OpenAPI boundary docs, and focused service/store
  regressions.

Definition of Done:

- Pending session creation rejects missing provider, provider key, intent,
  provider subject hash, invalid target user ids, and missing server secret.
- The plaintext pending token is returned only to the current browser flow; the
  store receives only `session_token_hash`.
- Raw upstream subject, authorization code, access token, refresh token,
  provider secret, state, nonce, PKCE verifier, session cookie, CSRF token, and
  full claim payloads are never persisted in `pending_oauth_sessions`.
- `provider_subject_hash` is the only provider subject identity stored; UI
  summary fields are limited to `subject_hint`, email/display/avatar metadata,
  and email verification state.
- `redirect_to` accepts only local paths and normalizes empty, cross-site, or
  protocol-relative redirects to `/`.
- Consume operations are single-use and expiry-aware.
- No public OAuth/OIDC OpenAPI route is exposed in this package; provider
  start/callback, profile adoption, bind-current-user, and create-account routes
  remain follow-up packages built on this foundation.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `cd apps/api && go test ./internal/modules/auth/... ./internal/persistence/entstore/auth -run 'Test(PendingOAuth|StorePersists|Cleanup|DeleteByUser|PasswordReset|EmailVerification|Login|Authenticate|Logout)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-970: OAuth Authorization Start v1

Objective: close the next OAuth onboarding gap found during the docs/sub2api
comparison by adding an SRapi-native authorization start route. This does not
copy sub2api's multiple plaintext OAuth cookies or callback/session adoption
behavior. SRapi issues one encrypted short-lived browser flow cookie, sends
state + PKCE S256 + OIDC nonce to the provider, and leaves callback/token
exchange/profile adoption for the next package.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/CONFIGURATION_SPEC.md`
- `apps/api/internal/modules/auth/service/pending_oauth.go`
- `apps/api/internal/modules/users/contract/contract.go`
- `packages/openapi/openapi.yaml`

Owns:

- Public auth API:
  - `GET /api/v1/auth/oauth/{provider}/start`
- Admin Settings `oauth_provider_configs` for non-secret provider authorization
  config: provider, provider key, display name, client id, authorize URL,
  redirect URI, and scopes.
- Auth service authorization URL generation with state, PKCE S256, and OIDC
  nonce.
- Encrypted HttpOnly `srapi_oauth_flow` cookie scoped to `/api/v1/auth/oauth`.
- Local redirect normalization and provider allowlist/config validation.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, admin settings, HTTP, and contract drift tests.

Definition of Done:

- OAuth start is disabled unless Admin Settings has `oauth_enabled=true` and a
  matching enabled provider config.
- The provider redirect includes `response_type=code`, `client_id`,
  `redirect_uri`, `state`, `code_challenge_method=S256`, `code_challenge`,
  scopes, and `nonce` for OpenID scopes.
- The encrypted flow cookie binds provider, provider key, intent, local redirect,
  state, PKCE verifier, nonce, creation time, and expiry.
- The flow cookie and Admin Settings responses do not expose provider secret,
  authorization code, access token, refresh token, raw upstream subject, full
  claim payload, session cookie, CSRF token, or API key material.
- `intent=login` is the only public start intent in this package; binding an
  external identity to a current user remains a CSRF-protected follow-up route.
- Callback token exchange, ID token validation, profile normalization, pending
  decision session creation, bind-current-user, create-account, and
  bind-existing-login flows remain follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service ./internal/modules/admin_control/service -run 'Test(StartOAuthAuthorization|PendingOAuth|UpdateAdminSettings.*OAuth)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestOAuthStart|TestUpdateAdminSettingsRejectsInvalidRegistrationEmailSuffixAllowlist|TestAdminControlPlaneV1EndpointsAndAudit' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-980: OAuth Callback Pending Session v1

Objective: close the next OAuth onboarding gap found during the docs/sub2api
comparison by adding an SRapi-native authorization callback route on top of the
encrypted flow cookie and hash-only pending OAuth session foundation. This does
not copy sub2api's plaintext browser/session cookies or raw claim storage.
SRapi validates state + provider config, uses PKCE public-client token exchange,
normalizes UserInfo into a safe profile summary, and stores only HMAC-scoped
provider subject hashes.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/CONFIGURATION_SPEC.md`
- `apps/api/internal/httpserver/runtime_oauth_handlers.go`
- `apps/api/internal/modules/auth/service/pending_oauth.go`
- `packages/openapi/openapi.yaml`

Owns:

- Public auth API:
  - `GET /api/v1/auth/oauth/{provider}/callback`
- Admin Settings callback config fields: `token_url`, `userinfo_url`, and
  `token_auth_method=none`.
- OAuth callback validation for flow cookie, provider, provider key, client id,
  redirect URI, and returned state.
- PKCE token exchange with `grant_type=authorization_code`, code, redirect URI,
  client id, and code verifier.
- Bearer UserInfo fetch and safe profile extraction.
- Auth service HMAC helper for provider subject hashes scoped by provider and
  provider key.
- Short-lived HttpOnly `srapi_oauth_pending` cookie scoped to
  `/api/v1/auth/oauth/pending`.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, admin settings, HTTP callback, and contract drift tests.

Definition of Done:

- Callback rejects missing/mismatched flow state and clears the flow cookie.
- Callback rejects disabled/missing provider configs and callback configs that
  are not complete for v1 public-client exchange.
- Successful callback exchanges the authorization code with the original PKCE
  verifier and fetches UserInfo with `Authorization: Bearer <access_token>`.
- Pending OAuth sessions store only a keyed hash of the upstream subject plus
  safe display/email/avatar summary fields.
- Flow cookie is cleared after callback completion; pending token is set only in
  an HttpOnly cookie and is not placed in redirect URLs.
- Client secrets, refresh tokens, access tokens, authorization codes, raw
  upstream subjects, full claims, session cookies, CSRF tokens, and API key
  material are not stored in Admin Settings, pending sessions, logs, or OpenAPI
  responses.
- Confidential-client secret handling, ID token signature/nonce validation,
  pending decision exchange, create-account, bind-existing-login, and
  bind-current-user mutation APIs remain follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service ./internal/modules/admin_control/service -run 'Test(StartOAuthAuthorization|PendingOAuth|HashOAuthProviderSubject|UpdateAdminSettings.*OAuth)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestOAuth(Start|Callback)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-990: Pending OAuth Decision Preview v1

Objective: close the next OAuth onboarding gap found during the docs/sub2api
comparison by adding an SRapi-native pending decision preview. This does not
copy sub2api's mutation-heavy pending exchange. SRapi exposes a read-only
inspection route that lets the console decide whether to ask for email
completion, bind an existing login, create a new account, or continue a later
authenticated bind flow without consuming the pending token.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `apps/api/internal/httpserver/runtime_oauth_handlers.go`
- `apps/api/internal/modules/auth/service/pending_oauth.go`
- `apps/api/internal/persistence/entstore/auth/store.go`
- `packages/openapi/openapi.yaml`

Owns:

- Public auth API:
  - `GET /api/v1/auth/oauth/pending`
- Non-consuming pending OAuth lookup in auth contract/service/store.
- Safe pending decision response with provider, provider key, display-safe
  subject hint, local redirect, profile summary, expiry, and `next_step`.
- Next-step decisions for email completion, bind-existing-login,
  create-account, ready-for-login, and bind-current-user continuation.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, Ent persistence, HTTP pending-preview, and contract drift
  tests.

Definition of Done:

- Missing, invalid, expired, or consumed pending cookies return 401 and clear
  the pending cookie where applicable.
- Preview is read-only and does not consume the pending OAuth session.
- Responses do not include pending token, raw upstream subject,
  provider-subject hash, authorization code, provider access/refresh token,
  session cookie, CSRF token, API key material, or full upstream claims.
- Existing active-account detection is limited to a next-step decision and a
  boolean continuation hint; final account creation/binding remains a follow-up
  mutation package.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service ./internal/persistence/entstore/auth -run 'Test(PendingOAuth|HashOAuthProviderSubject)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestOAuth(Start|Callback)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1000: Pending OAuth Current-User Bind v1

Objective: close the next pending OAuth mutation gap found during the
docs/sub2api comparison by adding an SRapi-native current-user bind path. This
does not copy sub2api's combined pending exchange/create/bind-login handlers.
SRapi keeps callback read-only, then requires an authenticated console session
and CSRF header before attaching the verified external identity to the current
user.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `apps/api/internal/modules/users/contract/contract.go`
- `apps/api/internal/modules/users/service/service.go`
- `apps/api/internal/modules/auth/service/pending_oauth.go`
- `apps/api/internal/httpserver/runtime_oauth_handlers.go`
- `packages/openapi/openapi.yaml`

Owns:

- Public auth API:
  - `POST /api/v1/auth/oauth/pending/bind-current-user`
- Users service command for binding a verified external identity to one user.
- Store lookup by provider/provider key/provider-subject hash so identity
  ownership can be checked before writes.
- Store-level rejection of attempts to transfer an existing external identity
  to a different user.
- Pending OAuth bind handler that requires session + CSRF + pending cookie,
  consumes the pending session only after successful binding, clears the pending
  cookie, records audit-safe identity snapshots, and returns the refreshed
  current-user auth identity list.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused users service, Ent persistence, HTTP pending-bind, and contract drift
  tests.

Definition of Done:

- Missing console session returns 401; missing or invalid CSRF returns 403;
  missing, invalid, expired, or consumed pending cookie returns 401 and clears
  the pending cookie where applicable.
- Binding succeeds only for the authenticated console user. A pending session
  targeting another user or an external identity already owned by another user
  returns conflict and does not transfer identity ownership.
- Successful bind persists only provider, provider key, hashed subject,
  display-safe subject hint, verification timestamp, last-used timestamp, and
  non-sensitive profile summary.
- Successful bind consumes the pending OAuth token, clears the pending cookie,
  and returns the current user's auth identity list without pending token, raw
  upstream subject, provider-subject hash, authorization code, provider tokens,
  session cookie, CSRF token, API key material, or full upstream claims.
- Create-account, bind-existing-login with password/2FA, email completion, and
  verification-code pending flows remain separate follow-up mutation packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/users/service ./internal/persistence/entstore/users -run 'Test(BindAuthIdentity|StoreFindsAuthIdentity)' -count=1`
- `cd apps/api && go test ./internal/modules/auth/service ./internal/persistence/entstore/auth -run 'Test(PendingOAuth|HashOAuthProviderSubject)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestOAuth(Start|Callback)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1300: OpenAPI Formalization Of The New Admin Surfaces v1

Objective: close the consistency/SDK debt accrued across WP-1110..WP-1280, which shipped their admin handlers as local DTOs (served via `writeJSONAny`, with no route↔spec conformance test). Describe every one of those endpoints in the OpenAPI contract so the generated Go and TypeScript types cover them, without changing any handler behavior.

What changed (spec-only — `packages/openapi/openapi.yaml`):
- Added 24 admin operations across 9 surfaces: model-rate-limits (list/upsert/delete), account-group-rate-limits (list/upsert/delete), TLS fingerprint profiles (list/create/update/delete), error-passthrough rules (list/create/update/delete), user-attribute definitions (list/create/update/delete) + per-user attribute values (list/set), `GET /admin/accounts/{id}/availability`, `POST /admin/accounts/{id}/quota-fetch`, and config snapshot `GET /admin/config-snapshot` + `POST /admin/config-snapshot/import`.
- Schemas hand-verified field-for-field against the Go handler payloads: single responses use `data`/`request_id`, list responses use `data`/`pagination`/`request_id`; write ops carry `csrfHeader: []` + the `x-srapi-rbac` write grant; ID path params reuse `#/components/schemas/Id`; deletes reuse `DeleteResponse`.
- `ConfigSnapshotResponse` references the existing `Provider`/`Model`/`AccountGroup`/`SubscriptionPlan`/`PricingRule`/`AdminSettings` plus the new `ErrorPassthroughRule`/`TLSProfile`/`UserAttributeDefinition` and denormalized `Snapshot{Model,Group}RateLimit` (carrying the natural-key name). `ConfigImportRequest` reuses the `Create*Request` schemas for the natural-keyed sections (TLS / user-attr / error rules) and dedicated `Import{Model,Group}RateLimit` for the ID-remapped ones; `ConfigImportResponse` mirrors the handler's per-section `ImportSectionResult` (created/updated) and `ImportRemapResult` (created/updated/skipped).

Design note: this is reference/SDK material, not a rewire. The handlers still emit local DTOs at the wire; the formal types make the surfaces discoverable and SDK-generable, and leave a clean path for a future WP to optionally route the handlers through the generated structs (and add a route↔spec conformance test) if desired.

Tests/gates: `make openapi-lint` (valid), `make openapi-codegen` + `make openapi-codegen-check` (Go types, no drift), `make openapi-ts-codegen` + `make openapi-ts-codegen-check` (TS types, no drift), `make sdk-ts-typecheck`, `go build ./...`, and full `go test ./...` — all pass. No new tables, no migrations, no handler changes.

## WP-1290: OIDC id_token Validation v1

Objective: complete confidential-client console OAuth (WP-1270 deferred this) with full OIDC id_token verification — signature, standard claims, and nonce — using a vetted library (user-approved dependency).

What changed:
- Dependency: `github.com/coreos/go-oidc/v3` (+ `go-jose/v4`), the standard Go OIDC verifier. `go get` + `go mod tidy`; httpserver-only import (no architecture-rule impact).
- `exchangeOAuthAuthorizationCode` now returns the `id_token` alongside the access token (the token response's `id_token` field, previously ignored).
- `config.OAuthConfig.Issuers` — a `map[provider_key]issuer` from env `OAUTH_ISSUERS_JSON`. The issuer is a public URL, kept in env alongside the client_secret (no AdminSettings/OpenAPI surface). `runtimeState.oauthIssuer` resolves it.
- `verifyOIDCIDToken(ctx, issuer, clientID, rawIDToken, expectedNonce)`: `oidc.NewProvider(issuer)` performs discovery (→ JWKS), `provider.Verifier(&oidc.Config{ClientID})` verifies RS256 signature + iss + aud(==clientID) + exp, then the code checks `nonce` == the flow's nonce. The callback calls it only when an issuer is configured; verification failure rejects login (`PROVIDER_AUTH_FAILED`). No issuer → unchanged behavior.

Tests/gates: end-to-end `verifyOIDCIDToken` test against a fake OIDC provider (RSA keygen + discovery + JWKS + RS256 signing) covering valid / nonce-mismatch / wrong-aud / expired / empty / forged-key-signature; exchange test updated for the new return; full `make check`. No new tables.

## WP-1280: ID-Referencing Config Import (Natural-Key Remap) v1

Objective: make rate-limit config (which references model/group integer IDs) importable across environments — the piece WP-1250 deferred because integer IDs do not port. Solution: denormalize the natural key into the export and remap it on import.

What changed:
- Export: `model_rate_limits` and `group_rate_limits` snapshot sections now carry the referenced entity's natural key — `model_name` (the model's canonical name) and `account_group_name` — resolved via id→name maps built from the listed models/groups (`snapshotModelRateLimits` / `snapshotGroupRateLimits`). Additive (the existing `model_id`/`account_group_id` fields remain).
- Import: `config-snapshot/import` gained `model_rate_limits` + `group_rate_limits` sections. For each row it resolves the name against the target environment's models/groups (canonical-name / group-name → id), then upserts the limit with the **target** id. A row whose referenced model/group is absent in the target is `skipped` (counted via `importRemapResult{created,updated,skipped}`), not an error — so a partial environment imports what it can. `dry_run` reports the counts without writing.
- Why this is safe where a raw ID import is not: integer PKs differ across environments; matching on the stable natural key (model canonical name, group name) is the correct portable join. Natural-keyed entities (TLS profiles, user attributes, error rules) were already importable in WP-1250; this extends the same principle to the ID-referencing rate limits.

Tests/gates: build + full `make check`. No new tables (orchestration over existing services).

## WP-1270: Confidential-Client Console OAuth v1

Problem (verified): the console-login OAuth token exchange (`exchangeOAuthAuthorizationCode`) sent `grant_type`/`code`/`redirect_uri`/`client_id`/`code_verifier` only — a public client with PKCE. Providers configured as confidential clients (web-application type) require a `client_secret` at the token endpoint and could not be used. `OAuthProviderConfig` had a `TokenAuthMethod` field but no secret to send.

What changed:
- `config.OAuthConfig.ClientSecrets` — a `map[provider_key]client_secret` parsed from env `OAUTH_CLIENT_SECRETS_JSON` via `parseStringMapEnv`. Secrets stay in deployment env, matching SRapi's posture (JWT/MASTER/SMTP secrets are env-only, never AdminSettings) — so no settings/OpenAPI/redaction surface is touched.
- `runtimeState.oauthClientSecret(providerKey)` resolves the secret; `exchangeOAuthAuthorizationCode` gained a `clientSecret` param and sets `client_secret` (client_secret_post) when non-empty. PKCE is preserved (the two coexist); public clients (no secret) are byte-identical to before.

Deferred: id_token validation. The flow authenticates the profile via the userinfo endpoint over TLS (access token from a PKCE-protected exchange), which is already secure; full OIDC id_token validation needs JWT signature verification (RS256 + JWKS), and there is no JWT/JOSE library in go.mod. Hand-rolling JWT verification is a security anti-pattern, so id_token validation is deferred pending a vetted dependency (e.g. golang-jwt + a JWKS fetcher) — a separate decision.

Tests/gates: `parseStringMapEnv` unit test; `exchangeOAuthAuthorizationCode` httptest verifying `client_secret` is sent for confidential clients, PKCE preserved, and omitted for public clients. Full `make check`. No new tables.

## WP-1260: Per-Model / Per-Group TPM v1

Objective: add tokens-per-minute ceilings for models and account groups — the last missing cells of the rate/capacity matrix (accounts/keys already had TPM; model/group had only RPM + concurrency). TPM bounds aggregate token throughput more precisely than RPM (a few large requests can overload an upstream even at low request rates).

What changed:
- `tpm_limit` column on `ModelRateLimit` and `AccountGroupRateLimit` (migration `000027`, default 0 = unlimited; existing rows correct without backfill). `TPMForModel`/`TPMForGroup` (fail-open); admin upsert accepts/returns it on both `/admin/model-rate-limits` and `/admin/group-rate-limits`.
- Enforcement reuses the existing limiter seams alongside the RPM checks: model TPM in `checkGatewayRateLimit` (`model:<id>:tpm`, evaluated at admission where the model id + token estimate are known) and group TPM in `reserveGatewayAccountQuota` (`group:<id>:tpm`, after the scheduler selects an account). Each check's Cost is `max(1, input+output+cached tokens)` over a 1-minute window — identical to the existing per-account / per-key TPM. Exceeding returns the established 429-class error → failover.
- Matrix now complete: per-API-key (rpm/tpm), per-user (rpm), per-account (rpm/tpm/concurrency), per-model (rpm/tpm/concurrency), per-group (rpm/tpm/concurrency).

Tests/gates: `model_rate_limits` + `group_rate_limits` service tests extended (`TPMForModel`/`TPMForGroup` + independence from rpm/concurrency), migration `000027`, full `make check`.

## WP-1250: Config Snapshot Import v1

Objective: complete the WP-1240 backup/restore loop by safely applying a config snapshot back, using natural-key upsert + a dry-run validation pass — without the cross-environment ID remapping that makes ID-referencing entities unsafe to import.

What changed:
- `POST /api/v1/admin/config-snapshot/import?dry_run=true|false`. Body is the importable subset: `{ tls_profiles, user_attribute_definitions, error_passthrough_rules }`.
- For each section: list existing rows, index by natural key (TLS profile `name`, user-attribute `key`, error-rule `name`), and for each incoming item create when the key is new or update the matched row — via the modules' existing Create/Update methods. Returns per-section `{ created, updated }`.
- `dry_run=true` computes the create/update decision per item but performs no writes (a validation/preview pass). Real imports are audited.
- Scope rationale: only natural-keyed, self-contained config is importable. Rate limits reference model/group integer IDs; providers/models carry IDs and cross-references — these do not port across environments, so they stay export-only. Sections apply independently (no cross-section transaction); imports are idempotent on re-run, and dry-run lets operators validate first.

Tests/gates: build + full `make check`. No new tables (orchestration over existing module services).

## WP-1240: Config Snapshot Export v1

Problem (verified): backup options were infra-level (`make backup-postgres` / `restore-postgres` pg_dump) or per-resource (accounts export/import, usage export) — there was no single portable snapshot of operator-managed configuration for review, migration, or version control.

What changed:
- Read-only `GET /api/v1/admin/config-snapshot` returns one versioned JSON: `{ snapshot_version, generated_at, data: { providers, models, account_groups, subscription_plans, pricing_rules, model_rate_limits, group_rate_limits, error_passthrough_rules, tls_profiles, user_attribute_definitions, settings } }`.
- A generic `snapshotSection[T,R](ctx, list, conv)` lists each collection and converts via the SAME `toAPI*` / local payload converters the per-resource admin list endpoints use, so the snapshot is snake_case-consistent and never drifts from them. Any list error fails the whole snapshot (no misleading partials).
- Scope is deliberately the config plane: it excludes account credentials (accounts have their own redacted export) and operational data (usage/audit/health snapshots).

Deferred: re-import. Applying a snapshot back is materially riskier than reading it — FK/dependency ordering (providers before mappings before rate limits), per-entity upsert vs create semantics, and conflict handling. It warrants its own WP with validation + dry-run.

Tests/gates: build + full `make check`. No new tables/migrations (read-only assembly over existing services).

## WP-1230: Scheduled Connectivity Test Runner v1

Problem (verified): the `health_probe` worker is scheduled but api_key-only (`doProbe` rejects non-api_key runtime classes), and the real generative connectivity test (`runtimeState.testAccount` responses-compact mode) only runs on admin demand. So OAuth / non-api_key accounts had no automated connectivity verification.

What changed:
- New `internal/workers/connectivity_test` worker (modeled on `health_probe`). Per pass: list accounts, skip inactive, resolve a probe model from account metadata / provider config (`responses_compact_probe_model` / `compact_probe_model` / `test_model`) — a configured model is the **opt-in** signal — and for eligible accounts run a `conversationProber` through `accounts.ProbeAccount(ctx, id, prober, policy)`, which decrypts the credential, folds the result into a health snapshot (status, cooldown, circuit state), and records it.
- `conversationProber.ProbeAccount` issues a minimal real generative call via `provider_adapters.InvokeConversation` ("Respond with OK.", `Mapping{UpstreamModelName: model}`). Success → OK result with upstream status; a `ProviderError` → a **not-OK result returned with nil error** so it folds into an unhealthy snapshot rather than being dropped.
- Because the probe is billable, it is **off by default** and only touches accounts that configure a probe model. `ConnectivityTestConfig` (`ACCOUNT_CONNECTIVITY_TEST_ENABLED` / `_INTERVAL_SECONDS` default 3600 / `_TIMEOUT_SECONDS` / `_MAX_CONCURRENT` default 2). Wired through `app.newHandler` (return tuple), `startWorkers`/`stopWorkers`, and the `internal/architecture` bootstrap-import allowlist.

Tests/gates: internal worker test (`probeModel` opt-in resolution + `conversationProber` success/failure outcome mapping with a stub adapter), `internal/app` bootstrap test, full `make check`. No new tables/migrations.

## WP-1220: Per-Model Max Concurrency v1

Objective: add a global max concurrent in-flight request ceiling per model (the concurrency complement to WP-1190's per-model RPM), completing the rate/capacity matrix.

What changed:
- `max_concurrency` column on `ModelRateLimit` (migration `000026`, default 0 = unlimited; existing rows correct without backfill). `model_rate_limits.ConcurrencyForModel` (fail-open); admin upsert accepts/returns it.
- `prepareProviderDispatch(ctx, account)` → `prepareProviderDispatch(ctx, account, modelID)`. All 11 dispatch entry points pass `req.Mapping.ModelID` (every provider invoke request — Conversation/TokenCount/ResponseInputItems/Embedding/Image/... — carries `Mapping modelcontract.ModelProviderMapping` with `ModelID`). The function acquires a `model:<id>:concurrency` lease via `acquireModelConcurrency` into `concurrencyLeases` with the existing acquire-with-rollback; exceeding returns the 429-class error → failover.
- New helper in `runtime_gateway_group_concurrency.go`.
- Limiter matrix now complete: per-API-key (rpm/tpm), per-user (rpm), per-account (rpm/tpm/concurrency), per-model (rpm/concurrency), per-group (rpm/concurrency).

Tests/gates: `model_rate_limits` service test extended (`ConcurrencyForModel` + RPM/concurrency independence), migration `000026`, full `make check`.

## WP-1210: Per-Account-Group Max Concurrency v1

Objective: add a max concurrent in-flight request ceiling per account group (the "concurrency capacity" complement to WP-1200's RPM), reusing the existing account concurrency-lease mechanism.

What changed:
- `max_concurrency` column on `AccountGroupRateLimit` (migration `000025`, default 0 = unlimited; existing rows correct without backfill). `group_rate_limits.ConcurrencyForGroup` (fail-open); admin upsert accepts/returns it.
- `providerDispatchState.concurrencyLease` (single) became `concurrencyLeases []ratelimit.ConcurrencyLease`. `prepareProviderDispatch` acquires the account lease, then `acquireAccountGroupConcurrency` takes a `group:<id>:concurrency` lease for each of the selected account's groups that has a ceiling — **acquire-with-rollback**: any acquisition failure (error or capacity-exceeded) releases the leases already taken so none leak. Exceeding returns the existing 429-class `ProviderError` → failover.
- All ~11 dispatch sites release via `releaseGatewayConcurrency(dispatch.concurrencyLeases)` (loops the existing empty-safe single-release). New helpers live in `runtime_gateway_group_concurrency.go` (extracted to keep `runtime_gateway_core.go` under the 2180-line cap).
- Per-model concurrency deferred: it needs the model id threaded through the 11 dispatch entry points; per-group needs only the account (looked up inside dispatch), so it was the clean seam.

Tests/gates: `group_rate_limits` service test extended (`ConcurrencyForGroup` + RPM/concurrency independence), migration `000025`, full `make check`.

## WP-1200: Per-Account-Group RPM Capacity v1

Problem (verified): account groups (`provider_scope` / `model_scope` / `strategy_hint` / `status`) had no capacity control — no per-group rate or concurrency ceiling — so a group's aggregate load could not be bounded. (Also the per-group rate limit deferred from WP-1190.)

What changed:
- `AccountGroupRateLimit` Ent entity (`account_group_id` unique, `rpm_limit`, `enabled`), migration `000024`.
- `group_rate_limits` module (contract/service/memory) + `entstore/groupratelimits`; `RPMForGroup(groupID)` returns the active ceiling (0 = unlimited/disabled/errors, fail-open).
- Enforcement at `reserveGatewayAccountQuota` (post-selection seam, where per-account RPM/TPM already reserve): the selected account's group IDs are fetched via `accounts.ListGroupIDsByAccount`, and each group with a positive limit adds a `group:<id>:rpm` check to the same Redis limiter batch. Exceeding any returns the existing 429-class `ProviderError`, which drives failover to another candidate.
- Admin list/upsert/delete (`/api/v1/admin/group-rate-limits`); upsert validates the group exists via `accounts.FindGroupByID`.
- Scope is RPM in v1. Per-group *concurrency* can reuse the existing `acquireProviderAccountConcurrency` lease mechanism (`group:<id>:concurrency`) in a follow-up; deferred to avoid multi-lease lifecycle risk.

Tests/gates: `group_rate_limits` service tests (upsert / RPM gating / validation / delete), migration `000024` + table-list update, full `make check`.

## WP-1190: Per-Model RPM Rate Limit v1

Problem (verified): `checkGatewayRateLimit` enforced only per-API-key RPM, per-user RPM, and per-API-key TPM; there was no per-model ceiling and the model registry had no rate-limit field, so an expensive/fragile upstream model could not be globally throttled.

What changed:
- `ModelRateLimit` Ent entity (`model_id` unique, `rpm_limit`, `enabled`), migration `000023`.
- `model_rate_limits` module (contract/service/memory) + `entstore/modelratelimits`. `RPMForModel(modelID)` returns the active ceiling (0 = unlimited / disabled / errors — fail-open so lookups never block traffic).
- Enforcement: `checkGatewayRateLimit` now takes `modelID` and, when a positive limit exists, appends a `model:<id>:rpm` check to the existing Redis-backed limiter batch — a global per-model ceiling across all users, on top of the per-key/user limits. Evaluated at admission (modelID already resolved there).
- Admin list/upsert/delete (`/api/v1/admin/model-rate-limits`); upsert validates the model exists.
- Scope is model-only in v1 (the entity is keyed by model). Per-group rate limiting is deferred: a group is known only after account selection, so it needs a post-scheduling seam.

Tests/gates: `model_rate_limits` service tests (upsert / RPM gating / validation / delete), migration `000023` + table-list update, full `make check`.

## WP-1180: Subscription-Allowance-First Billing With Pay-As-You-Go Overage v1

Decision: allowance is **cost-based ($)** — `monthly_cost_quota` is the included $ allowance; overage falls back to balance. Opt-in per plan (`cost_quota_mode`), default `hard_cap` (unchanged).

What changed:
- Ent `UsageLog.billable_cost` (migration `000022`, backfilled `= cost` for existing rows). It is the portion charged to balance after allowance coverage.
- `subscriptions/service`: `EntitlementDecision.CostQuotaMode`; `CheckEntitlement` reads `cost_quota_mode` and, in `allowance` mode, stops denying on `monthly_cost_quota` (allow-with-overage). New `CostAllowance(userID, now)` lookup and pure, unit-tested `BillableOverage(cost, usedBefore, allowance)` = `clamp(0, cost, (usedBefore+cost) - allowance)`.
- `runtime_gateway_usage.recordGatewayUsage` computes `billable_cost` once (gated on success + non-zero cost + an active allowance-mode subscription) via `CostAllowance` + the existing success-only monthly `gatewayUserPeriodUsage`, and stores it on the usage log. Threaded through one method only — no change to the dozens of usage-record call sites.
- usage entstore writes `billable_cost` with an empty→cost fallback; billing entstore `sumUsageCosts` bills `billable_cost` (falling back to `cost`). Net: normal usage bills full cost (unchanged); allowance-covered usage bills only the overage; a fully-covered request bills 0.
- Token allowance (`monthly_token_quota`) stays a hard cap in v1.

Tests/gates: `BillableOverage` table test (covered / boundary / partial / exhausted / zero-cost / unparseable / fractional), billing+usage entstore fixtures updated for `billable_cost`, migration `000022` + backfill, full `make check`.

---
Original charter (problem statement, verified in code): subscription and balance were orthogonal, not layered.
- `subscriptions/service.CheckEntitlement` treats `monthly_cost_quota` / `monthly_token_quota` as HARD CAPS: when `CostUsedInPeriod + EstimatedCost > monthly_cost_quota` it returns `Allowed=false` (`monthly_cost_quota_exceeded`) and the request is rejected — it does NOT fall back to pay-as-you-go.
- `gatewayPricing`/`EstimatePrice` derive per-request cost purely from pricing rules (mapping override > pricing_rule > default_zero); the subscription never alters cost.
- `runtime_gateway_usage.recordGatewayUsage` records `Cost = pricing.Amount` unconditionally; `balance_charger` charges every usage log's full cost with zero subscription awareness.
- Net: a subscribed user is both capped by the monthly quota AND charged balance per request; there is no "spend the subscription allowance first, then bill the overage to balance".

Objective: make subscription a consumable allowance that is spent first, with the overage falling back to pay-as-you-go balance — opt-in per plan, default unchanged.

Proposed approach:
- Per-plan entitlement flag, e.g. `cost_quota_mode: "hard_cap" | "allowance"` (default `hard_cap` = current behavior, zero change).
- In `allowance` mode, `CheckEntitlement` stops denying on cost-quota; instead it returns the remaining allowance so downstream can split each request's cost into covered vs billable.
- New usage field `billable_cost` (Ent + migration): at record time, `billable_cost = clamp(0, cost, (CostUsedInPeriod + cost) - allowance)` — the portion beyond the allowance. `cost - billable_cost` is subscription-covered (free).
- `balance_charger` charges `billable_cost` instead of full `cost`; ledger/audit distinguish covered vs billed.
- Token allowance (`monthly_token_quota`) stays a hard cap in v1 (tokens don't map to $ balance); revisit later.

Open decisions for sign-off: (1) allowance unit = cost-based `monthly_cost_quota` (recommended) vs token-based; (2) opt-in per-plan flag vs global; (3) whether to surface covered-vs-billed in the billing ledger and admin usage views.

Tests/gates (when built): subscriptions service entitlement-split unit tests, billing charge split tests, migration for `billable_cost`, full `make check`.

## WP-1170: Scheduled Account Quota Refresh v1

Objective: promote the WP-1160 quota fetch from admin-triggered to automated, so accounts with a configured quota endpoint are refreshed on a schedule.

What changed:
- `provider_adapters`: `AccountQuotaFetcher` gains `QuotaConfigured(req) bool` (credential-free endpoint check) so callers skip credential decryption for accounts without quota.
- New `internal/workers/quota_refresh` worker (modeled on `health_probe`): lists active accounts, resolves the provider, skips when not `QuotaConfigured`, else decrypts the credential, calls `FetchAccountQuota`, and persists each returned `QuotaSignal` as an `AccountQuotaSnapshot`. Bounded concurrency, per-account timeout, graceful Start/Shutdown, `RunOnce` for deterministic single passes.
- `config.QuotaRefreshConfig` (`ACCOUNT_QUOTA_REFRESH_ENABLED`/`_INTERVAL_SECONDS`/`_TIMEOUT_SECONDS`/`_MAX_CONCURRENT`); disabled by default. Wired through `app.newHandler` (return tuple), `startWorkers`/`stopWorkers`, and the `internal/architecture` bootstrap-import allowlist.

Tests/gates: `internal/app` bootstrap test, `internal/architecture` conformance, `make code-quality-check`, full `go test ./...`. No new tables/OpenAPI changes.

## WP-1160: Per-Provider Quota/Subscription Fetch Scaffold v1

Objective: add an active, out-of-band per-account quota/subscription fetch for OAuth providers (Codex/Antigravity/Gemini-CLI), complementing the existing passive in-band `QuotaSignal` header parsing. Design is SRapi-native and avoids inventing provider API shapes.

What changed:
- `provider_adapters/contract`: `QuotaReport` (provider, supported, source, plan, credits remaining/used/limit, currency, quota signals, status, fetched-at) + `AccountQuotaFetcher` interface.
- `provider_adapters/service/quota_fetch.go`: `FetchAccountQuota` reuses the probe HTTP plumbing (`probeHeaders`, egress client) and is fully config-driven — quota endpoint (`quota_url`/`subscription_url`/`credits_url`) and JSON-path field mappings come from provider config / account metadata, so each provider is supported by configuration; Codex response headers are folded in via the existing `codexQuotaSignalsFromHeaders`. Returns `Supported:false` when no endpoint is configured.
- `POST /api/v1/admin/accounts/{id}/quota-fetch`: decrypts the credential (`accounts.DecryptCredential`), fetches, persists quota signals (`RecordQuotaSnapshot`), audits, and returns the normalized report.

Tests/gates: `make architecture-check`, `make code-quality-check`, full `go test ./...`, `git diff --check`. Follow-up: promote to a scheduled runner with confirmed provider subscription endpoints.

## WP-1150: Health-Probe Availability Rollups v1

Objective: aggregate fine-grained health snapshots into per-day availability rollups over a rolling window.

What changed: `AccountAvailabilityRollup` Ent entity; `health_rollups` module (pure UTC per-day bucketing → availability ratio + avg success rate, provider-agnostic `Sample` input); `entstore/healthrollups` (upsert keyed on account+date); `GET /api/v1/admin/accounts/{id}/availability?days=N` computes from recent snapshots, persists (upsert), and returns the trailing window plus overall uptime. No worker added — computed on admin read + persisted.

Tests/gates: migration `000021`, `make architecture-check`, `make code-quality-check`, full `go test ./...`, `git diff --check`.

## WP-1140: Auth CAPTCHA v1

Objective: add human verification on auth endpoints, off by default.

What changed: stateless `captcha` module (`Verifier` + HTTP siteverify for Turnstile/hCaptcha/reCAPTCHA — shared form-POST + `success` JSON shape); `config.CaptchaConfig` (`CAPTCHA_ENABLED`/`PROVIDER`/`SECRET_KEY`/`VERIFY_URL`); `verifyCaptcha` reads `X-Captcha-Token`/`Cf-Turnstile-Response` and gates `handleLogin`+`handleRegister`. Disabled → no-op (behavior unchanged).

Tests/gates: `internal/config` test, `make architecture-check`, `make code-quality-check`, full `go test ./...`, `git diff --check`.

## WP-1130: TLS Fingerprint DB Profiles v1

Objective: let operators manage named egress fingerprint profiles centrally instead of repeating `egress_profile` metadata per account.

What changed: `TLSFingerprintProfile` Ent entity; `tls_profiles` module (validates uTLS template + HTTP-version policy against the egress resolver's supported set; `Snapshot` for resolution); `entstore/tlsprofiles`; admin CRUD (`/api/v1/admin/tls-profiles*`); `reverse_proxy.SetNamedProfileExpander` wired from `runtimeState.expandEgressProfileMetadata` so a `tls_profile`-referenced account gets the named profile's fields filled into `egress_profile` — only for keys it left unset (account values always win, behavior unchanged when the expander is unset/no ref).

Tests/gates: migration `000021`, `make architecture-check`, `make code-quality-check`, full `go test ./...`, `git diff --check`.

## WP-1120: Error-Passthrough DB Rules v1

Objective: centralize upstream error expose/mask decisions that were previously only configurable via per-account/provider metadata.

What changed: `ErrorPassthroughRule` Ent entity; `error_passthrough` module (priority-ordered rules matched on status code + error class + keyword; `Resolve` returns expose/mask); `entstore/errorpassthrough`; admin CRUD (`/api/v1/admin/error-passthrough-rules*`); the gateway's failover writers became `*Server` methods consulting `gatewayPublicMessage`, where a matching global rule overrides (expose raw / mask) and otherwise falls back to the existing `gatewayProviderPublicMessage` per-account metadata behavior. Existing per-account tests on `gatewayProviderPublicMessage` are unaffected.

Tests/gates: migration `000021`, `make architecture-check`, `make code-quality-check`, full `go test ./...`, `git diff --check`.

## WP-1110: User Custom Attributes (EAV) v1

Objective: operator-defined custom user profile fields.

What changed: `UserAttributeDefinition` + `UserAttributeValue` Ent entities (EAV); `userattributes` module (typed validation — string/number/boolean/select with required + select-option enforcement); `entstore/userattributes`; admin CRUD for definitions (`/api/v1/admin/user-attributes*`) and per-user value get/set (`/api/v1/admin/users/{id}/attributes*`, joining enabled definitions with stored values).

Tests/gates: migration `000021`, `make architecture-check`, `make code-quality-check`, full `go test ./...`, `git diff --check`.

## WP-1100: API-Key Account Egress Consistency v1

Objective: close the egress gap discovered during WP-1090. SRapi routes
non-`api_key` upstream traffic through the per-account `reverse_proxy` egress
(proxy + uTLS fingerprint + SSRF guard) but routes native `api_key` traffic through
the adapter's shared `s.client`, so an `api_key` account configured with a proxy or
TLS-fingerprint profile silently does NOT use it. This makes native `api_key`
gateway traffic AND its health probe honor the account's configured egress.

Owns:

- `reverse_proxy` exposes `ManagedEgressClient(account) (*http.Client, bool, error)`
  on the `Runtime` interface + Service: returns the per-account egress client and
  `true` only when the account actually has managed egress (a proxy or an
  `egress_profile`/`tls_template` etc. in metadata); otherwise `(nil, false, nil)`.
- The provider adapter selects its HTTP client via a gated `egressHTTPClient`
  helper: the managed egress client when the account is configured, else the
  unchanged shared `s.client`. Applied to all native `api_key` invoke paths
  (chat/responses/messages/embeddings/images/input-items/compact) and the health
  probe, so a managed `api_key` account's probe and real traffic share one egress.
- Test mocks of `Runtime` implement the new method as a no-op `(nil, false, nil)`.
- Focused reverse_proxy and adapter regressions.

Out of scope (explicit follow-ups):

- Making ALL `api_key` traffic SSRF-guarded (v1 only changes accounts that already
  opted into managed egress, keeping the default path byte-identical); per-account
  egress for WebSocket/realtime native paths.

Definition of Done:

- An `api_key` account with a proxy or TLS profile sends its native gateway traffic
  and its health probe through that egress; an account with no egress config is
  byte-for-byte unchanged (still uses `s.client`).
- The probe and real traffic of a managed `api_key` account use the same client.
- No credential or internal detail is logged; existing adapter behavior for
  non-managed accounts is unchanged.

Required gates:

- `cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Gateway|Codex|OAuthRefresh' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1090: Health Probe Desync Jitter v1

Objective: harden SRapi's active account health probing against upstream
anti-abuse/correlation detection. Analysis: SRapi already does the safe thing —
the probe is a cheap non-generative `GET /models` (not a generative "hi"), gated to
`api_key` accounts only (OAuth/subscription accounts are never actively probed,
kept purely passive). The remaining risk is that one probe pass fires probes for
all eligible accounts as a near-synchronized burst, which — combined with a shared
egress pool — is itself a correlation signal. This package desynchronizes probes.
It does NOT add generative probes or probe OAuth accounts.

Egress finding (no change required): the request says "probe through the account
egress". For the only probed class (`api_key`), real traffic already uses the same
shared adapter client as the probe (native api_key requests use `s.client`, not the
per-account `reverseProxy.Do` egress), so the probe is already indistinguishable
from real traffic. Routing the probe through `reverseProxy.Do` would give it a
proxy/TLS profile the real traffic lacks — the opposite of the goal — so it is
deliberately not done. The deeper gap (api_key native traffic ignoring per-account
proxy/TLS entirely) is a separate adapter concern, out of scope here.

Owns:

- `health_probe` worker `Config.Jitter` (default = probe interval, capped at the
  interval) and an injectable random source; each eligible account sleeps a
  ctx-aware random `[0, jitter)` before probing, bounded by the existing
  concurrency semaphore, so probes spread across the interval instead of bursting.
- `api_key`-only eligibility and the cheap non-generative `GET /models` probe are
  unchanged; OAuth/subscription accounts stay passive.
- Focused worker regressions.

Out of scope (explicit follow-ups):

- Probe history + 7-day availability rollups (sub2api parity); making api_key native
  traffic honor per-account proxy/TLS egress; any OAuth active probing.

Definition of Done:

- Probes for multiple accounts no longer start simultaneously; the per-account
  start offset is a bounded random value and respects context cancellation.
- A zero/disabled jitter leaves behavior unchanged; default behavior spreads probes
  across the interval.
- No generative probe is introduced and no OAuth/subscription account is probed.

Required gates:

- `cd apps/api && go test ./internal/workers/health_probe/... -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1080: Gateway Idempotency Hardening v1

Objective: harden the WP-1070 gateway idempotency by closing the two loose ends it
left: stored records grow unbounded (the entity carries `expires_at` but nothing
reaps it) and only two of the four mutating gateway endpoints are covered. This
reuses SRapi's existing cleanup-worker fleet and the `withGatewayIdempotency`
wrapper — no new mechanism.

Owns:

- `idempotency.Store.DeleteExpired(ctx, before)` (contract + memory + entstore) and
  a `workers/idempotency_cleanup` worker modeled on `auth_session_cleanup` that
  periodically deletes records whose `expires_at` has passed; wired into the app
  worker lifecycle (start/shutdown) and skipped under memory storage.
- `withGatewayIdempotency` extended to `POST /v1/messages` and `POST /v1/embeddings`
  (embeddings is always non-streaming; messages streams pass through), with the
  optional `Idempotency-Key` header + 409 response added to both OpenAPI operations
  and regenerated Go types + TS SDK.
- Focused worker and gateway regressions.

Out of scope (explicit follow-ups):

- Streaming response replay; images/audio idempotency; multi-instance reap
  coordination (single periodic reaper is sufficient for v1).

Definition of Done:

- Expired idempotency records are reaped on the worker interval; non-expired and
  in-flight records are retained.
- A retried non-streaming `/v1/messages` or `/v1/embeddings` request with the same
  key+body replays the first response without re-executing; streaming/key-less
  requests are unchanged.
- `api_key`/existing behavior is otherwise unchanged; no secret is logged.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/idempotency/... ./internal/workers/idempotency_cleanup/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Idempotenc|Gateway' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1070: Gateway Request Idempotency v1

Objective: close the gateway-idempotency gap from the docs/sub2api comparison.
SRapi already defines an `IdempotencyRecord` Ent entity (key+method+path+request
hash, status, response snapshot, `locked_until`, `expires_at`) but never wires it,
so a client that retries a mutating gateway call after a timeout can double-execute
(double-bill). This does not copy sub2api's IdempotencyCoordinator; SRapi wires its
own entity behind an opt-in `Idempotency-Key` header on the OpenAI-compatible
gateway, reusing the existing isolated-store + HTTP-wrapper patterns.

Scope (v1, deliberately narrow):

- An `idempotency` module (contract + service + memory store) and an
  `entstore/idempotency` over the existing entity; atomic begin via the unique
  `(idempotency_key, method, path)` index with constraint-race handling.
- A `withGatewayIdempotency` HTTP wrapper applied to `POST /v1/chat/completions`
  and `POST /v1/responses` (and their source-endpoint/provider-alias variants):
  - No `Idempotency-Key` header → passthrough unchanged.
  - Streaming requests (`stream:true`) → passthrough (replay deferred to v2).
  - First non-streaming request with a key records in-progress (locked), runs the
    handler through a capturing writer, and stores the response snapshot on
    success; retry with the same key + same request replays the snapshot.
  - Same key, request still in flight → 409; same key, different request body →
    422; the key is scoped per bearer token so tenants never collide.
- Records carry a TTL (`expires_at`); a stale in-progress lock (process crash)
  becomes re-acquirable after `locked_until`.
- OpenAPI: a reusable `Idempotency-Key` header parameter + conflict response on the
  two canonical gateway operations; regenerated Go types + TS SDK.
- Focused service/store and HTTP regressions.

Out of scope (explicit follow-ups):

- Streaming response replay; idempotency on messages/embeddings/images/audio and
  the provider-prefixed alias docs; a TTL cleanup worker (entity carries
  `expires_at`; reaping is a follow-up).

Definition of Done:

- A duplicate non-streaming request with the same `Idempotency-Key` and body
  replays the first response and never re-executes (no second upstream call / bill).
- A concurrent duplicate returns 409; the same key with a different body returns
  422; a missing key or a streaming request is unaffected.
- The key is bound to the bearer token; snapshots and the conflict errors never leak
  another tenant's data, the raw token, or upstream credentials.
- `api_key`/existing gateway behavior is otherwise unchanged.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/idempotency/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Idempotenc|Gateway' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1060: Outbound SSRF Egress Guard v1

Objective: close the one confirmed security gap from the docs/sub2api comparison.
SRapi's reverse-proxy egress (`reverse_proxy/service/egress_profile.go`
`validateEgressTargetURL`) only validates URL scheme; outbound upstream-forward
and OAuth-refresh dials have no IP/CIDR screen, so an admin-configured (or
future user-influenced) target that resolves into the private network — loopback,
RFC1918, link-local, or the `169.254.169.254` cloud-metadata endpoint — is
reachable. This does not copy sub2api's `channel_monitor_ssrf` host/CIDR blocklist
service; SRapi adds a dial-time IP screen on its existing isolated-client transport,
gated by run mode exactly like the existing cookie-`Secure` convention
(`Server.Mode != "local"`).

Owns:

- `reverse_proxy/service/ssrf.go`: `blockedEgressIP` (loopback, unspecified,
  link-local unicast/multicast incl. `169.254.0.0/16` metadata, RFC1918 + ULA via
  `net.IP.IsPrivate`, RFC6598 CGNAT `100.64.0.0/10`, multicast) and a
  `net.Dialer.Control` that screens the POST-DNS-resolution remote IP, defeating
  DNS-rebinding that a URL-string check cannot.
- A `WithBlockedPrivateEgress(bool)` option on `reverse_proxy.New` threaded into the
  isolated-client transport (`newIsolatedClient`) and the uTLS direct dial
  (`dialUTLSHTTP1`); the screen applies to DIRECT dials only — an explicitly
  configured egress proxy remains operator-trusted and unscreened.
- Runtime wiring enables the guard when `cfg.Server.Mode != "local"` so production
  blocks private egress while local/dev/test (loopback httptest servers) keep
  working; the existing `New(nil)` default stays non-blocking and back-compatible.
- Focused unit regressions for the IP classifier and for client-level block/allow.

Out of scope (explicit follow-ups):

- Account health-probe and console-OAuth backchannel egress paths (separate clients)
  and a DB/settings-backed host/CIDR allowlist for intentional private targets.
- Screening proxied (CONNECT) egress and per-account egress policy overrides.

Definition of Done:

- With the guard on, a direct dial whose resolved IP is loopback/private/link-local/
  metadata/CGNAT/multicast fails before connecting; public IPs are unaffected.
- The screen runs at `net.Dialer.Control` (resolved IP), not on the URL string, so a
  hostname that resolves to a private IP is still blocked.
- `Server.Mode == "local"` (default, tests) does not block; `New(nil)` is unchanged.
- `api_key` and all existing reverse-proxy behavior is otherwise unchanged; no secret
  or internal IP is leaked in the blocked-dial error.

Required gates:

- `cd apps/api && go test ./internal/modules/reverse_proxy/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'OAuthRefresh|ReverseProxy|Gateway|Egress|SSRF' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1050: Upstream Account OAuth Re-auth Parking v1

Objective: harden the existing upstream-account OAuth runtime found during the
docs/sub2api comparison. Analysis showed SRapi already refreshes `oauth_refresh`
/ `oauth_device_code` access tokens on the serving path
(`runtime_gateway_core.refreshReverseProxyCredential` + the provider-aware
`reverse_proxy.Refresh`, with import-time minting via `refreshImportCredential`),
so this package does NOT rebuild that runtime. The single remaining correctness
gap: when a serve-time refresh fails *permanently* (a dead/rotated refresh token →
`session_invalid`), the account was only audited and left `active`, so the
scheduler kept selecting it and every request replayed the dead refresh token
against the provider token endpoint. This package parks such accounts for re-auth,
reusing SRapi's existing failure-class→status protection rather than copying
sub2api's per-provider refresher fleet.

Owns:

- A shared `runtimeState.protectProviderAccountForClass` extracted from
  `applyProviderAccountProtection`, so adapter-invoke failures and refresh failures
  use one class→status transition with one `auto_protect` audit.
- `refreshReverseProxyCredential` applies that protection to the refresh error
  class: a permanent `session_invalid` (provider `invalid_grant` /
  `refresh_token_reused` / invalid refresh) transitions the account to
  `needs_reauth`; transient classes (`rate_limit`, `timeout`, `upstream_error`,
  `auth_failed`, `invalid_response`) map to no status and leave it untouched for
  the next attempt.
- Focused gateway regression proving permanent refresh failure parks the account
  and transient failure does not.

Out of scope (explicit follow-ups):

- Interactive authorization-code/device-code re-auth to mint a fresh refresh token
  after parking (per-provider, large) — operators re-import for now.
- Per-provider quota/subscription/credits accounting and proactive background
  refresh; multi-instance refresh coordination beyond the existing per-account
  in-process lock.

Definition of Done:

- A permanently rejected serve-time refresh flips the account to `needs_reauth`
  (idempotently) and records an `auto_protect` audit; the scheduler then stops
  selecting it, ending the dead-token replay loop.
- Transient refresh failures never change account status.
- `api_key` accounts and the existing refresh-on-expiry/rotate/persist behavior are
  unchanged; no new refresh implementation is introduced.
- No secret material (refresh token, access token, client secret, ciphertext) is
  logged, audited, or returned.

Required gates:

- `cd apps/api && go test ./internal/httpserver -run 'OAuthRefresh|ReverseProxy|ProviderAccount' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1040: Pending OAuth Profile Adoption v1

Objective: close the profile-adoption follow-up explicitly deferred by WP-1030 by
letting a pending OAuth session that binds to an existing account opt in to
adopting the provider-returned display name onto the SRapi profile. This does not
copy sub2api's implicit provider-profile overwrite on every sign-in. SRapi keeps
adoption opt-in, validates the adopted name exactly like a self-service profile
edit, and never auto-adopts provider avatar URLs: SRapi avatars use a controlled
upload/storage model, so fetching a remote avatar URL would add SSRF and privacy
exposure that the account owner never authorized.

Owns:

- `PendingOAuthBindLoginRequest` gains an optional `adopt_display_name` flag and a
  new `PendingOAuthBindCurrentUserRequest` carries the same optional flag; both
  default to false and omitting the bind-current-user body keeps every preference
  disabled.
- `POST /api/v1/auth/oauth/pending/bind-current-user`,
  `POST /api/v1/auth/oauth/pending/bind-login`, and
  `POST /api/v1/auth/oauth/pending/bind-login/2fa` honour the opt-in only when the
  bind attaches the identity to an existing account.
- The bind-login two-factor path carries the opt-in inside the existing
  HMAC-signed, pending-cookie-bound challenge (version bumped to `oauth-bind-v2`)
  so the preference is tamper-evident and the two-factor request body is
  unchanged.
- Adoption reuses `users.UpdateProfile` validation (trimmed, non-empty,
  <= 120 runes) and is skipped silently when the provider supplied no name, the
  name exceeds the profile limit, the name is unchanged, or the profile update
  fails, so a successful identity bind is never undone by an adoption preference.
- Audit "after" snapshots for both existing-account bind paths record
  `display_name_adopted`.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service and HTTP regressions for opt-in adoption, default-off binding,
  the signed two-factor opt-in, and the avatar non-adoption guarantee.

Definition of Done:

- Adoption is opt-in only; default and omitted request bodies never change the
  profile display name or avatar.
- Only existing-account bind flows (bind-current-user and bind-login, including
  its two-factor completion) adopt; create-account naming is unchanged.
- Provider avatar URLs are never adopted, fetched, or written to the profile.
- The adopted display name is validated exactly like a self-service profile edit,
  and an unusable provider name leaves the profile unchanged without failing the
  bind.
- The two-factor opt-in is bound to the current pending cookie and cannot be
  replayed against another pending OAuth session.
- Responses and audit never expose pending token, raw upstream subject,
  provider-subject hash, authorization code, provider tokens, password, session
  cookie, CSRF token, or API key material.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service -run 'TestPreparePendingOAuthBindLogin|TestPendingOAuth(Session|ActionToken|EmailCompletion)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestPendingOAuthBind(Login|CurrentUser)|TestOAuthCallback' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1030: Pending OAuth Email Completion v1

Objective: close the next pending OAuth mutation gap found during the
docs/sub2api comparison by adding an SRapi-native email-completion flow for
OAuth/OIDC providers that do not return a usable email. This does not copy
sub2api's front-end pending token or low-entropy code exchange. SRapi keeps the
HttpOnly pending cookie as the browser binding and sends an encrypted,
high-entropy, short-lived email-completion link through the existing
transactional outbox.

Owns:

- `POST /api/v1/auth/oauth/pending/send-verify-code` accepts a submitted email
  only when the pending session is a login flow with no retained email.
- The send route enqueues `PendingOAuthEmailCompletionRequested` with encrypted
  recipient email, encrypted confirmation token, email hash, expiry, and no raw
  pending token/provider subject material.
- Notification templates include `auth.oauth_pending_email_completion`, and the
  worker sends a link containing the encrypted confirmation token.
- `POST /api/v1/auth/oauth/pending/email-completion/confirm` requires the same
  pending cookie plus the emailed token, writes the verified email back to the
  pending session, and returns a fresh safe pending preview.
- Confirmation does not consume pending OAuth, bind identities, create accounts,
  or issue console sessions; the next step remains explicit create-account or
  bind-login.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, notification, Ent persistence, and HTTP regressions.

Definition of Done:

- Missing/invalid/expired/consumed pending cookie returns 401 and clears the
  pending cookie where applicable.
- Send and confirm reject sessions that already have email, target a user, or do
  not represent a login flow.
- Send response is uniform and does not reveal whether the email belongs to an
  existing account.
- Email-completion token is high entropy, encrypted at rest in outbox, tied to
  the current pending session id, expires quickly, and cannot be used with
  another pending OAuth cookie.
- Confirmed email is normalized, marked verified for the pending session, and
  causes the next preview to choose create-account or bind-existing-login using
  existing SRapi decision logic.
- Responses, outbox payload, and audit never expose pending token, raw upstream
  subject, provider-subject hash, authorization code, provider tokens, password,
  session cookie, CSRF token, API key material, or full upstream claims.
- Profile adoption remains a separate follow-up package.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service -run 'TestPendingOAuth(EmailCompletion|ActionToken|Session|PreparePendingOAuthBindLogin|HashOAuthProviderSubject)' -count=1`
- `cd apps/api && go test ./internal/modules/notifications/service -run 'Test(PendingOAuthEmailCompletionEvent|AuthEmailEvent|AuthPasswordResetEvent)' -count=1`
- `cd apps/api && go test ./internal/persistence/entstore/auth -run 'TestPendingOAuth(EmailCompletion|SessionsAreHashOnly)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'TestPendingOAuth(EmailCompletion|CreateAccount|BindLogin)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1020: Pending OAuth Create Account v1

Objective: close the next pending OAuth mutation gap found during the
docs/sub2api comparison by adding an SRapi-native create-account flow for
provider-verified emails. This does not copy sub2api's browser-session pair or
combined exchange response. SRapi keeps `GET /pending` as the decision surface
and issues a short-lived action token bound to the HttpOnly pending cookie before
accepting a state-changing unauthenticated create-account request.

Owns:

- `GET /api/v1/auth/oauth/pending` includes `create_account_action` only when
  `next_step=create_account_required`.
- `POST /api/v1/auth/oauth/pending/create-account` validates the pending cookie,
  action token, registration settings, email suffix policy, provider-retained
  email match, and identity ownership before creating a user.
- New users created from verified provider emails are marked email-verified,
  bound to the external identity, signed in with the normal console session
  cookie, and have the pending cookie consumed/cleared.
- If identity binding or pending consumption fails before session issuance, the
  newly created local user is deleted so the email is not reserved by a partial
  OAuth flow.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service and HTTP regressions for action-token binding and successful
  create-account completion.

Definition of Done:

- Missing/invalid/expired/consumed pending cookie returns 401 and clears the
  pending cookie where applicable.
- Missing or wrong action token returns 403; action tokens cannot be reused with
  another pending OAuth cookie.
- Email mismatch, targeted existing-user sessions, disabled registration, suffix
  policy rejection, and already-used emails fail without consuming pending.
- Success creates the user, marks provider-verified email ownership, binds the
  identity, consumes/clears pending, sets a normal session cookie, and returns
  `LoginResponse`.
- Responses and audit never expose pending token, raw upstream subject,
  provider-subject hash, authorization code, provider tokens, password, session
  cookie, CSRF token, API key material, or full upstream claims.
- Send-verify-code, email completion, and profile adoption remain separate
  follow-up mutation packages.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service -run 'TestPendingOAuth(ActionToken|Session|PreparePendingOAuthBindLogin|HashOAuthProviderSubject)' -count=1`
- `cd apps/api && go test ./internal/modules/users/service ./internal/persistence/entstore/users -run 'Test(DeleteRemovesUser|BindAuthIdentity|ListAuthIdentities|StoreFindsAuthIdentity)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Test(PendingOAuthCreateAccount|PendingOAuthBindLogin|OAuth(Start|Callback))' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-1010: Pending OAuth Existing-Account Bind Login v1

Objective: close the next pending OAuth mutation gap found during the
docs/sub2api comparison by adding an SRapi-native existing-account bind-login
flow. This does not copy sub2api's combined exchange handler or token-pair
response shape. SRapi keeps pending preview read-only, then requires local
password authentication and, when enabled, TOTP before binding the external
identity and issuing a normal console session cookie.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `apps/api/internal/modules/auth/service/pending_oauth.go`
- `apps/api/internal/modules/users/service/service.go`
- `apps/api/internal/httpserver/runtime_oauth_handlers.go`
- `packages/openapi/openapi.yaml`

Owns:

- Public auth APIs:
  - `POST /api/v1/auth/oauth/pending/bind-login`
  - `POST /api/v1/auth/oauth/pending/bind-login/2fa`
- Auth service prepare/complete commands that validate pending OAuth session,
  authenticate local credentials, issue pending-OAuth-specific 2FA challenges
  when needed, and bind those challenges to the pending token hash.
- HTTP bind-login handlers that attach the verified external identity to the
  existing account, reject target-user mismatches and identity ownership
  conflicts, consume/clear pending cookies only after successful binding, and
  issue normal console session cookies.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused auth service and HTTP regressions for no-2FA and 2FA flows.

Definition of Done:

- Missing/invalid/expired/consumed pending cookie returns 401 and clears the
  pending cookie where applicable.
- Invalid email/password returns 401; disabled target users return 403; pending
  sessions targeting a different user or identities already owned by another
  user return conflict.
- Accounts without 2FA bind the identity, consume the pending session, clear the
  pending cookie, set a normal console session cookie, and return `LoginResponse`.
- Accounts with 2FA return `202 LoginTwoFactorRequiredResponse` after password
  validation without binding, consuming pending session, or issuing a session
  cookie. The 2FA completion route requires the same pending cookie and rejects
  challenges paired with a different pending token.
- Responses and audit never expose pending token, raw upstream subject,
  provider-subject hash, authorization code, provider tokens, password, TOTP
  secret/code, session cookie, CSRF token, API key material, or full upstream
  claims.
- Create-account, email completion, send-verify-code, and profile adoption
  remain separate follow-up mutation packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/service -run 'TestPreparePendingOAuthBindLogin|TestPendingOAuthSession|TestHashOAuthProviderSubject' -count=1`
- `cd apps/api && go test ./internal/modules/users/service ./internal/persistence/entstore/users -run 'Test(BindAuthIdentity|StoreFindsAuthIdentity)' -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Test(PendingOAuthBindLogin|OAuth(Start|Callback))' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-900: Low-Balance Notification Trigger v1

Objective: close the first notification-trigger gap found during the
docs/sub2api comparison by adding an SRapi-native balance-low trigger. This
does not copy sub2api's synchronous goroutine email sender or user-column extra
email list; SRapi enqueues a safe domain event after successful balance charging
and lets the existing outbox notification dispatcher render, suppress, retry,
and deliver the optional email.

Read first:

- `docs/DOMAIN_EVENTS_SPEC.md`
- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `apps/api/internal/workers/balance_charger/worker.go`
- `apps/api/internal/modules/notifications/service/service.go`
- `apps/api/internal/workers/outbox/domain_handler.go`

Owns:

- `BalanceLowTriggered` domain event.
- Balance charger threshold-crossing detection:
  `balance_before >= threshold && balance_after < threshold`.
- Admin Settings email defaults:
  - `balance_low_notify_enabled`
  - `balance_low_notify_threshold`
  - `balance_low_notify_recharge_url`
- Outbox notification handling for `balance.low` with template rendering,
  one-click unsubscribe headers, and current-user preference suppression.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused notification, balance charger, outbox, and admin settings tests.

Definition of Done:

- Successful usage charging can enqueue exactly one idempotent
  `BalanceLowTriggered` event when the user crosses below the configured
  threshold.
- The event payload contains only safe operational fields: user id, recipient
  email hash, balances, threshold, currency, ledger reference, usage log ids,
  charged timestamp, and recharge URL.
- Payloads and idempotency keys do not include plaintext email, unsubscribe
  token, session cookie, CSRF token, SMTP secret, API key, provider credential,
  or prompt.
- Notification dispatch re-reads the current user, rejects stale recipient
  hashes, skips inactive users, honors `balance.low` unsubscribe state, and
  attaches one-click unsubscribe headers when possible.
- SMTP/template failures remain outbox-retryable and never roll back balance
  charging.
- Avatar storage, OAuth identity binding/onboarding, and credential-gated SMTP
  smoke remain separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... -count=1`
- `cd apps/api && go test ./internal/workers/balance_charger ./internal/workers/outbox ./internal/modules/admin_control/... -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-890: Current-User Notification Preferences v1

Objective: close the current-user notification preference gap found during the
docs/sub2api comparison by adding SRapi-native primary-email optional
notification preference APIs. This does not copy sub2api's user-column extra
email JSON model; SRapi keeps hash-only event-scoped preference state shared
with one-click unsubscribe, and leaves extra notification contacts for a
separate verified-contact flow.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/notifications/service/preferences.go`
- `apps/api/internal/httpserver/runtime_notification_handlers.go`

Owns:

- Current-user APIs:
  - `GET /api/v1/me/notification-preferences`
  - `PUT /api/v1/me/notification-preferences`
- Preference service list/update methods for optional event subscriptions.
- Shared storage with one-click unsubscribe using event plus primary-email hash.
- Audit-safe current-user preference update records.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused notification service and HTTP tests.

Definition of Done:

- `GET` requires a console session and returns all optional notification events
  with default `subscribed=true` when no stored preference exists.
- `PUT` requires console session plus CSRF and updates only allowlisted optional
  events such as `balance.low` and `account.quota_alert`.
- Transactional auth mail events such as `auth.password_reset` and
  `auth.email_verification` cannot be modified through the preference API.
- Stored preference keys and values do not include plaintext recipient email,
  unsubscribe token, session cookie, CSRF token, SMTP secret, API key, provider
  credential, or prompt.
- Current-user preference updates and one-click unsubscribe share the same
  suppression state used by notification senders.
- Extra notification email addresses, notification triggers, avatar storage,
  OAuth identity binding/onboarding, and credential-gated SMTP smoke remain
  separate follow-up packages.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/notifications/... -count=1`
- `cd apps/api && go test ./internal/httpserver -run 'Test(CurrentUserNotificationPreferences|NotificationUnsubscribeEndpoint)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-850: Public Email Verification v1

Objective: close the email-verification gap found during the docs/sub2api
comparison by adding SRapi-native public email verification with hash-only
durable tokens and outbox-backed delivery metadata. This does not copy
sub2api's short verification-code cache; SRapi persists a single-use receipt,
keeps delivery behind the existing domain-event outbox boundary, and treats
verification as proof of email ownership rather than login.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/auth/contract/contract.go`
- `apps/api/internal/modules/users/contract/contract.go`

Owns:

- Public auth APIs:
  - `POST /api/v1/auth/email-verification/request`
  - `POST /api/v1/auth/email-verification/confirm`
- `email_verification_tokens` Ent schema and incremental PostgreSQL migration.
- Auth store support for hash-only verification token creation and atomic
  single-use consumption.
- Auth service request/confirm flow with uniform request responses and
  `users.email_verified_at` marking after confirmation.
- Outbox event `AuthEmailVerificationRequested` carrying only encrypted token
  delivery metadata and email hash.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, Ent persistence, HTTP, migration, and contract drift tests.

Definition of Done:

- Request route is unauthenticated and returns the same accepted response for
  syntactically valid existing, missing, inactive, already verified, or
  otherwise unavailable emails.
- No plaintext verification token is persisted; durable storage contains only
  keyed token hash, expiry, and used-at receipt data.
- Outbox payload does not contain plaintext email, password, session cookie,
  CSRF token, or plaintext verification token.
- Confirm route consumes exactly one unexpired token, sets
  `users.email_verified_at`, exposes the verified timestamp through current-user
  and admin user responses, and rejects token reuse.
- Confirming email ownership does not create or revoke console sessions.
- Notification email management, avatar storage, and OAuth identity binding
  remain separate follow-up flows.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/... ./internal/modules/users/... ./internal/persistence/entstore/auth ./internal/httpserver -run 'Test(RequestEmailVerification|ConfirmEmailVerification|VerifyEmail|EmailVerification|Register)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`

## WP-840: Public Password Reset v1

Objective: close the next auth lifecycle gap found during the docs/sub2api
comparison by adding SRapi-native public password reset with hash-only durable
tokens and outbox-backed delivery metadata. This does not copy sub2api's Redis
code cache; SRapi persists a single-use receipt and keeps mail delivery behind
the existing domain-event outbox boundary.

Read first:

- `docs/SECURITY_MODEL.md`
- `docs/OPENAPI_CONTRACT.md`
- `docs/DATA_MODEL.md`
- `packages/openapi/openapi.yaml`
- `apps/api/internal/modules/auth/contract/contract.go`
- `apps/api/internal/modules/users/contract/contract.go`

Owns:

- Public auth APIs:
  - `POST /api/v1/auth/password-reset/request`
  - `POST /api/v1/auth/password-reset/confirm`
- `password_reset_tokens` Ent schema and incremental PostgreSQL migration.
- Auth store support for hash-only reset token creation and atomic single-use
  consumption.
- Auth service request/confirm flow with uniform request responses and active
  session revocation after reset.
- Outbox event `AuthPasswordResetRequested` carrying only encrypted token
  delivery metadata and email hash.
- Generated OpenAPI Go types and TypeScript SDK.
- Focused service, Ent persistence, HTTP, migration, and contract drift tests.

Definition of Done:

- Request route is unauthenticated and returns the same accepted response for
  syntactically valid existing, missing, inactive, or otherwise unavailable
  emails.
- No plaintext reset token is persisted; durable storage contains only keyed
  token hash, expiry, and used-at receipt data.
- Outbox payload does not contain plaintext email, password, session cookie,
  CSRF token, or plaintext reset token.
- Confirm route consumes exactly one unexpired token, updates the password hash,
  revokes active console sessions for the user, rejects old password login, and
  rejects token reuse.
- Email verification, notification email management, avatar storage, and OAuth
  identity binding remain separate follow-up flows.
- `specs/STATUS.md` records completion and gates.

Required gates:

- `make ent-generate-check`
- `make migration-check`
- `make openapi-lint`
- `make openapi-codegen-check`
- `make openapi-ts-codegen-check`
- `make sdk-ts-typecheck`
- `cd apps/api && go test ./internal/modules/auth/... ./internal/modules/users/... ./internal/persistence/entstore/auth ./internal/httpserver -run 'Test(RequestPasswordReset|ConfirmPasswordReset|ResetPassword|PasswordReset|Register|LoginCreatesSession)' -count=1`
- `make architecture-check`
- `make code-quality-check`
- `git diff --check`
