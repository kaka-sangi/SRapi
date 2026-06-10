# Contributing to SRapi

Thanks for your interest in SRapi. This guide covers how to set up the project, the rules every
change follows, and how to get a change merged.

Please also read [`AGENTS.md`](AGENTS.md) — it is the short, binding set of engineering rules for
this repository (write simple, boring, maintainable code; keep changes focused; don't add
speculative abstraction). Everything below builds on it.

## Before you start

- For anything beyond a small fix, open an issue first to align on the approach.
- One change, one goal. Do not mix feature work, refactors, formatting, and dependency upgrades in
  the same pull request.
- Security-sensitive changes (auth, credentials, the reverse-proxy runtime, billing) must be called
  out explicitly in the PR description.

## Development setup

Requirements: **Go 1.26**, **Node 22**, **PostgreSQL 16**, **Redis 7**, and Docker (for the local
stack). See the [Quick start](README.md#quick-start) for the fastest path.

```bash
make bootstrap-env   # create a local .env with generated secrets
make dev-up          # start PostgreSQL, Redis, and the API
make web-install     # install console dependencies
make web-dev         # run the Next.js console
```

Run `make help` to see every target.

## Project layout

- `apps/api` — Go backend. Domain logic lives in `internal/modules/*`, each behind an explicit
  contract; cross-module calls go through contracts, never direct package reach-in.
- `apps/web` — Next.js console (admin + self-service workspace).
- `packages/openapi/openapi.yaml` — the OpenAPI contract; it is the **source of truth** for the HTTP
  surface and generates both the Go server types and the TypeScript SDK.
- `apps/api/ent/schema` + `apps/api/migrations` — the data model and its Atlas-versioned migrations.
- `docs/` — architecture and operations docs. `specs/` — the long-running development execution specs.

## OpenAPI-first workflow

The HTTP contract drives the generated code, so the order matters:

1. Edit `packages/openapi/openapi.yaml` first.
2. Regenerate: `make openapi-codegen` (Go) and `make openapi-ts-codegen` (TypeScript SDK).
3. Implement the handler/module against the generated types.
4. `make openapi-codegen-check` and `make openapi-ts-codegen-check` must show no drift.

For data-model changes: edit the Ent schema, run `make ent-generate`, then `make migration-diff` to
create the next numbered migration. Never edit a released migration; add a new one.
`make migration-check` enforces numbering and up/down pairing.

## Quality gates

Run the full gate before pushing:

```bash
make check
```

It runs: diff-check, OpenAPI lint + bundle, Go and TypeScript SDK codegen drift checks, TypeScript
typecheck, Ent generate check, migration check, Go architecture + code-quality + tests, the examples
check, the web lint/typecheck/build/test, and a secret scan. CI
([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs the same `make check`.

Useful focused gates: `make architecture-check`, `make code-quality-check`, `make migration-check`,
`make api-test`, `make web-check`, `make secret-scan`.

## Keep docs in sync

When you change behavior, update the matching doc in the same PR. The mapping is listed in
[`docs/README.md`](docs/README.md) (§ "维护规则" / maintenance rules) — for example, changing the HTTP
contract requires updating `docs/requirements/OPENAPI_CONTRACT.md`, changing data tables requires
`docs/requirements/DATA_MODEL.md`, and any new user-visible copy must follow `docs/requirements/PRODUCT_TONE.md`.

## Commits and pull requests

- This repo uses **Conventional Commits** with a scope, e.g.
  `feat(gateway): ...`, `fix(quality): ...`, `docs(specs): ...`.
- Keep the diff minimal and easy to review; no unrelated changes.
- In the PR description: summarize what changed, what you tested, and any remaining risk. If you
  added user-visible copy, link the relevant entries in `docs/requirements/PRODUCT_TONE.md`.
- Make sure `make check` passes.

## License

By contributing, you agree that your contributions are licensed under the project's
[AGPL-3.0](LICENSE) license.
