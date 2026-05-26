# SRapi Quality Gates

## 1. Universal Gates

Code quality means the requested behavior is correct and the repository remains easy to understand, modify, test, debug, and extend. A change should improve or preserve code health over time; passing tests is necessary but not sufficient.

Review every implementation change against these standards:

- Correctness: normal inputs, invalid inputs, edge cases, and error paths behave predictably.
- Readability: names and structure explain intent without requiring hidden context.
- Maintainability: ownership boundaries are clear, coupling stays low, and business logic is not scattered.
- Simplicity: use the smallest boring design that solves the current need; no speculative abstractions.
- Testability: core logic, side effects, and error paths can be verified in focused tests.
- Locality: inputs, rules, state changes, errors, and outputs are discoverable without excessive file hopping.
- Stable evolution: changes are small, reversible, and protected by behavior checks.
- Consistency: error handling, dependency injection, API shapes, tests, and layout follow existing project patterns.

Run for every implementation package unless explicitly skipped with a reason in `specs/STATUS.md`:

```bash
git status --short
```

Before finalizing a package:

```bash
make architecture-check
make code-quality-check
```

For broad or cross-cutting changes:

```bash
make check
```

## 2. OpenAPI Gates

Required when changing HTTP routes, request/response schemas, error envelopes, auth behavior, generated Go types, or generated TypeScript SDK:

```bash
make openapi-lint
make openapi-bundle
make openapi-codegen-check
make openapi-ts-codegen-check
make sdk-ts-typecheck
```

Rules:

- Edit `packages/openapi/openapi.yaml` first.
- Do not manually edit `apps/api/internal/openapi/openapi.gen.go`.
- Do not manually edit generated files under `packages/sdk/typescript/src`.
- Gateway-compatible endpoints must preserve source protocol error shapes.

## 3. Ent And Migration Gates

Required when changing Ent schemas, migrations, persistent repository code, indexes, encrypted fields, or data model docs:

```bash
make ent-generate-check
make migration-check
cd apps/api && go test ./internal/persistence/entstore/... ./internal/platform/db
```

Rules:

- Ent schema, migrations, and `docs/DATA_MODEL.md` must agree.
- PostgreSQL is source of truth.
- Redis must remain rebuildable.
- Secrets are encrypted or hashed.

## 4. Go Module Gates

Required for backend behavior changes:

```bash
cd apps/api && go test ./...
```

For narrow packages, start with focused tests:

```bash
cd apps/api && go test ./internal/modules/<module>/...
```

Rules:

- Services depend on store interfaces and contracts.
- Ent access remains in `internal/persistence/entstore`.
- Workers use contracts/services, not handlers.
- Provider-specific behavior must not leak into Scheduler core.

## 4.1 Go Code Quality Harness

Required for backend implementation packages and always run by `make check`:

```bash
make code-quality-check
```

It covers:

- `gofmt -l` drift across Go files.
- `go vet ./...`.
- `git diff --check` whitespace drift.
- `make check` must include architecture, code-quality, API test, generated drift, migration, and secret-scan gates.
- `make check` must include observability rule hygiene so deployable alert rules cannot drift outside low-cardinality/sensitive-data guardrails.
- Secret scanning must cover generated OpenAPI/SDK artifacts and lockfiles.
- `make bootstrap-env` must generate strong local secrets instead of copying weak placeholders, and `make env-check` must reject existing weak or over-permissive env files.
- `make deploy-preflight` must reuse env and observability hygiene checks before Compose deployment and warn or fail on missing host deployment tools depending on strict mode.
- Production Go file-size thresholds, excluding generated Ent/OpenAPI/SDK code.
- Production Go function-size thresholds, excluding tests and generated Ent/OpenAPI/SDK code.
- Repository text hygiene for tracked source/docs/config files.
- Node and shell script syntax checks.
- Dockerfile and Compose baseline container hygiene.
- Production Go code must not add speculative markers such as `TODO`, `FIXME`, `HACK`, or `XXX`.
- Production Go `panic`/`recover` usage is restricted to documented bootstrap escape hatches.

Rules:

- Treat failures as harness findings, not as optional style advice.
- If a threshold is hit by new code, split by ownership, extract cohesive private functions, or move behavior behind an existing boundary before merging.
- Threshold changes must be documented in `docs/ARCHITECTURE_REQUIREMENTS.md` and justified in `specs/STATUS.md`.
- Do not add a new abstraction, framework, helper, or global registry unless at least two real call sites need it now.
- New exported APIs must be documented; complex private logic needs a short comment explaining why, not what.
- Errors must be explicit and actionable; do not silently swallow failures.

## 5. Gateway Gates

Required when changing Gateway routes, endpoint adapters, Canonical AI IR, Provider Adapter dispatch, streaming, or usage recording:

```bash
cd apps/api && go test ./internal/httpserver ./internal/modules/...
make smoke-gateway
```

Also add or update tests for:

- auth failure
- model not visible
- no available account
- provider 429/5xx mapping
- streaming success
- streaming upstream interruption
- request_id propagation
- usage log creation
- scheduler decision creation
- scheduler feedback creation

QualityEval capture, judge worker, or Scheduler quality evidence changes must also run:

```bash
make smoke-quality-eval
```

Prometheus alert rules, Alertmanager routes, Grafana alerting guidance, or deployable observability rule files must also run:

```bash
make observability-rules-check
```

It covers:

- Deployable SRapi alert rule files exist.
- Alert expressions use approved low-cardinality metric labels.
- Rule labels stay limited to fixed routing dimensions.
- Alertmanager notification grouping stays limited to fixed low-cardinality routing dimensions.
- API key, account id, user id, request id, fingerprint, rule id, prompt, credential, cookie, and similar sensitive or high-cardinality fields do not enter rule files.

## 6. Scheduler Gates

Required when changing scheduling, strategies, leases, feedback, account runtime state, or Redis scheduler persistence:

```bash
cd apps/api && go test ./internal/modules/scheduler/... ./internal/persistence/redisstore/scheduler/... ./internal/persistence/entstore/scheduler/...
```

Tests must cover:

- hard filter reject reasons
- balanced strategy
- cost_saver strategy
- score breakdown
- lease acquire and release
- concurrency overflow prevention
- cooldown and circuit open
- feedback recording
- deterministic test seed where randomness exists

## 7. Reverse Proxy Runtime Gates

Required when changing non-API-key account runtime, header handling, proxy handling, cookie jar, OAuth refresh, or egress profile behavior:

```bash
cd apps/api && go test ./internal/modules/reverse_proxy/... ./internal/modules/provider_adapters/... ./internal/modules/accounts/...
```

Tests must cover:

- forbidden outgoing headers are stripped
- SRapi identifiers are not sent upstream
- per-account clients do not share cookie jar or runtime state
- refresh uses lock
- refresh failure does not overwrite credentials
- risk classes map to account state or feedback
- logs do not include credentials

## 8. Security Gates

Required when touching auth, API keys, credentials, cookies, payment config, proxy config, logging, audit, or debug captures:

```bash
make secret-scan
cd apps/api && go test ./internal/platform/crypto ./internal/modules/auth/... ./internal/modules/api_keys/... ./internal/modules/accounts/...
```

Rules:

- No plaintext API key persistence.
- No plaintext provider credentials.
- No Authorization/Cookie/token values in logs.
- CSRF applies to console writes.
- SSRF defenses apply to custom upstream URLs and external fetches.

## 9. Frontend Gates

Required when `apps/web` exists and frontend code changes:

```bash
npm run typecheck --workspace apps/web
npm run lint --workspace apps/web
```

If scripts differ, use the repository's configured equivalents.

Browser verification is required for substantial UI work:

- desktop screenshot
- mobile screenshot
- no text overlap
- no horizontal page overflow except contained data tables
- generated SDK is used for API calls

## 10. Examples And Migration Guide Gates

Required when changing `examples/`, public SDK usage docs, or 2api migration guidance:

```bash
make examples-check
```

It covers:

- Public examples mention required Gateway and AdminOps routes.
- Public examples use `SRAPI_BASE_URL`, `SRAPI_API_KEY`, and optional admin session/CSRF environment variables.
- The TypeScript example typechecks against the generated SDK.
- The 2api migration guide preserves the selected Provider Account OAuth/session/desktop/CLI/IDE credential boundary and rejects local Codex / Claude Code / Antigravity ingress plus Gateway-local DTOs.
- Examples do not contain real-secret placeholders.

## 11. Documentation Gates

Required whenever implementation changes behavior:

- Gateway route change -> update `docs/GATEWAY_ROUTE_MATRIX.md`.
- OpenAPI contract change -> update `docs/OPENAPI_CONTRACT.md`.
- Data model change -> update `docs/DATA_MODEL.md`.
- Module dependency change -> update `docs/MODULE_INTERFACE_CONTRACTS.md`.
- Domain event change -> update `docs/DOMAIN_EVENTS_SPEC.md`.
- Capability change -> update `docs/CAPABILITY_TAXONOMY_SPEC.md`.
- Reverse proxy behavior change -> update `docs/REVERSE_PROXY_SPEC.md`.
- Scheduler strategy change -> update scheduler docs.
- Payment change -> update `docs/PAYMENT_SPEC.md`.
- Observability change -> update `docs/OBSERVABILITY_SPEC.md`.

## 12. Definition Of Done

A package is done when:

- Implementation satisfies `WORK_PACKAGES.md`.
- Required tests pass.
- Generated artifacts are current.
- Security-sensitive changes are covered.
- Docs are updated.
- `specs/STATUS.md` is updated.
- Any skipped gate has a clear reason.
