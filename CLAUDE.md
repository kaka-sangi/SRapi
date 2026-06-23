# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

SRapi is a self-hosted AI gateway. One OpenAI-compatible endpoint routes requests through provider adapters to upstream AI services (OpenAI, Anthropic, Gemini, etc.) with scheduling, rate limiting, billing, and full admin control. The backend is Go, the frontend is Next.js (App Router), and the contract is a single OpenAPI spec.

## Build and test commands

```bash
# Backend (run from apps/api/)
cd apps/api
go build ./internal/...          # compile check
go build ./cmd/srapi/            # build the binary
go test ./internal/...           # all tests (~125 packages)
go test ./internal/modules/scheduler/service/... -run TestName -v  # single test
go vet ./internal/...            # static analysis

# Frontend (run from apps/web/)
cd apps/web
npx tsc --noEmit                 # type check
npm run dev                      # dev server

# From repo root (via Makefile)
make api-test                    # all Go tests
make api-run                     # run API with go run (needs .env)
make dev-up / dev-down           # Docker Compose (API + Postgres + Redis)
make openapi-lint                # validate OpenAPI spec
make openapi-bundle              # bundle multi-file spec into single file
make openapi-codegen             # regenerate Go types from spec
make openapi-ts-codegen          # regenerate TypeScript SDK from spec
make ent-generate                # regenerate Ent ORM client from schema
make smoke-health                # curl /api/v1/health
make smoke-gateway               # end-to-end gateway smoke test
STORAGE_BACKEND=memory go run ./cmd/srapi/  # run with in-memory stores (no DB)
```

## Architecture

### Monorepo layout

```
apps/api/           Go backend (single binary)
  cmd/srapi/        Entrypoint
  ent/schema/       78 Ent entity schemas (DB models)
  internal/
    app/            App lifecycle (startup, shutdown, worker orchestration)
    config/         123 env-var config with YAML file override support
    httpserver/     HTTP handlers, middleware, gateway core (~236 files)
    modules/        46 domain modules (contract/ service/ store/)
    platform/       Shared infra (DB, Redis, crypto, rate limiter, circuit breaker, task queue)
    workers/        24 background workers (health probes, token refresh, billing, etc.)
    openapi/        Generated Go types from OpenAPI spec
apps/web/           Next.js 16 frontend (App Router)
  src/app/          Pages (admin/, auth/, account/, playground/, etc.)
  src/components/   Shared UI components
  src/hooks/        React Query hooks for all API endpoints
  src/i18n/         en + zh translations (~3200 keys each)
  src/lib/          API clients, form helpers, formatting
  src/context/      React contexts (language, toast, copilot session)
  src/providers/    Query client, theme, top-level providers
packages/openapi/   Source OpenAPI spec (multi-file with $ref to schemas/)
packages/sdk/       Generated TypeScript SDK (types + client)
deploy/             Docker Compose, Tempo config
tools/              CI validation scripts
```

### Request flow (gateway hot path)

1. `httpserver/server.go` — middleware chain: recover → CORS → security headers → request ID → tracing → concurrency → maintenance gate → RBAC
2. `runtime_gateway_handlers.go` — parse canonical request, run admission (auth, rate limits, quota, balance reservation)
3. `modules/scheduler/` — score candidates on health, quota, latency, quality; pick account
4. `modules/provider_adapters/` — convert canonical → provider wire format, dispatch upstream
5. `modules/reverse_proxy/` — TLS/HTTP fingerprinted proxy for CLI/web-session accounts
6. `runtime_gateway_streaming.go` — SSE passthrough with Anthropic usage accumulator
7. `runtime_gateway_failover.go` — retry on transient errors, blacklist on permanent, classify error
8. `runtime_gateway_usage.go` — record usage log, compute cost, debit balance, emit events

### Key design patterns

- **Contract/Service/Store layering**: each module has `contract/` (interfaces), `service/` (logic), `store/` (persistence). The httpserver wires them via `runtimeState`.
- **OpenAPI-first**: the spec at `packages/openapi/openapi.yaml` is the source of truth. `make openapi-codegen` generates Go types; `make openapi-ts-codegen` generates the TS SDK. Both must stay in sync.
- **Ent ORM**: 78 entity schemas in `ent/schema/`. Run `make ent-generate` after schema changes. Generated code is in `ent/` (do not edit).
- **Memory stores**: every store has an in-memory implementation for tests and `STORAGE_BACKEND=memory` mode.
- **i18n**: all user-visible strings in `src/i18n/messages/{en,zh}.ts`. Use `t("section.key")` via `useLanguage()`. en and zh must have identical key sets.
- **Design tokens**: use `text-srapi-*`, `bg-srapi-*`, `border-srapi-*` colors, never raw Tailwind colors (red-500, etc.). The token palette supports light/dark themes.
- **Admin copilot (小r)**: AI operator in `modules/copilot/` with tool-calling loop over the admin API. System prompt in `copilot/tools.go`.

### Configuration

All config via environment variables (123 total). Optional YAML file override via `SRAPI_CONFIG_FILE=/path/to/config.yaml` (env vars always win). Key vars: `DATABASE_*`, `REDIS_*`, `JWT_SECRET`, `SRAPI_MASTER_KEY`, `TOTP_ENCRYPTION_KEY`. Release mode enforces strong secrets.

### Workers

24 background workers in `internal/workers/`. All implement `Start(ctx)` + `Shutdown(ctx)`. Shutdown is parallel via WaitGroup. Key workers: `accounts_token_refresh` (OAuth + keepalive), `health_probe`, `balance_charger`, `scheduled_test`, `channel_monitor`.
