# Changelog

All notable changes to SRapi are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). The detailed development
ledger (per work package) lives in [`specs/plans/STATUS.md`](specs/plans/STATUS.md).

## [Unreleased]

- Documentation governance: rewritten README (English + 简体中文), added `LICENSE` (AGPL-3.0),
  `CONTRIBUTING.md`, `SECURITY.md`, and this changelog; refreshed `docs/` and `specs/` indexes.

## [0.1.0] - 2026-06-06

Initial public release candidate. SRapi is a self-hosted AI gateway and control plane.

### Added

- **Gateway** — OpenAI-compatible (`/v1/chat/completions`, `/v1/responses` + compact/ws/input-items,
  `/v1/embeddings`, `/v1/moderations`, `/v1/rerank`, `/v1/images/{generations,edits,variations}`,
  `/v1/audio/{speech,transcriptions}`, `/v1/realtime`), Anthropic Messages (`/v1/messages`,
  `/v1/messages/count_tokens`), and native Gemini (`/v1beta/models`, `:generateContent`,
  `:streamGenerateContent`, `:countTokens`) endpoints, with lossless conversion through a canonical
  AI representation. SSE streaming and WebSocket/Realtime relay.
- **Scheduler kernel** — pluggable strategy registry with capability hard-filters, scoring (health,
  quota, latency, cache affinity, session stickiness, priority, live concurrency, cost), leases,
  decision/feedback evidence, dry-run, and shadow decisions.
- **Provider adapters & presets** — OpenAI-compatible, Anthropic-compatible, and Gemini adapters;
  presets for OpenAI, Anthropic, Gemini, Antigravity, DeepSeek, Moonshot/Kimi, Qwen, Zhipu/GLM,
  Grok, Groq, Mistral, Together, and OpenRouter, plus custom upstreams.
- **Reverse-proxy ("2api") runtime** — official-client / web-session dispatch for Codex CLI,
  Claude Code CLI, Claude Web, ChatGPT Web, Antigravity, and Gemini CLI, with per-account TLS/HTTP
  fingerprinting, isolated cookie jars, OAuth refresh lifecycle, and an outbound SSRF egress guard.
- **Rate & capacity limits** — per-key, per-user, per-account, per-model, and per-group RPM / TPM /
  concurrency; cross-provider failover; request idempotency keys.
- **Accounts & operations** — account groups, priority tiers, proxy binding, per-account model
  mapping, model discovery, background health probes, availability rollups, scheduled connectivity
  tests, and quota refresh.
- **Commerce** — subscription plans, decimal-safe pricing, balance billing with pay-as-you-go
  overage, payments via Stripe / Alipay / WeChat Pay (signed, idempotent webhooks), affiliate/invite
  rebates, redeem codes, promo codes, and announcements.
- **Auth** — console sessions (cookie + CSRF), TOTP 2FA, password reset, email verification, public
  registration with policy gates, OAuth/OIDC sign-in (with OIDC id_token validation), CAPTCHA, RBAC
  roles, workspaces, and per-user custom attributes.
- **Observability & ops** — Prometheus `/metrics`, OpenTelemetry traces, SLO definitions and
  burn-rate alerts, durable system and audit logs, channel/account health monitoring, data-retention
  workers, PostgreSQL backup/restore, transactional email, and release smoke tests.
- **Control plane** — Next.js admin console and self-service workspace, an admin AI Copilot, and a
  gateway-billed Playground, all generated against the OpenAPI contract (332 operations).
- **Tooling** — OpenAPI-first codegen for Go server types and the TypeScript SDK, Ent + Atlas
  versioned migrations, an architecture/code-quality harness, and the `make check` quality gate run
  in CI.

[Unreleased]: https://example.com/your-repo/compare/v0.1.0...HEAD
[0.1.0]: https://example.com/your-repo/releases/tag/v0.1.0
