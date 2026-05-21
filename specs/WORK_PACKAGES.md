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

## WP-220+: Advanced Endpoint And Provider Expansion

Use `ROADMAP.md` Phase 5 through Phase 8 to split future packages for:

- Gemini native routes
- images and media runtime
- embeddings and rerank
- realtime/websocket
- provider live discovery
- SLO and alert center
- SDK examples
- migration guides

Each new package must be added here before implementation starts.

