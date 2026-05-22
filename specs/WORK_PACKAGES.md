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

## WP-490+: Ecosystem And Remaining Advanced Surface

Use `ROADMAP.md` Phase 7 through Phase 8 to split future packages for:

- provider-native realtime protocol adapters and richer slot lifecycle
- image variations and image edit streaming / JSON reference compatibility
- SDK examples
- migration guides

Each new package must be added here before implementation starts.
