# SRapi Roadmap

## Phase 0: Execution Foundation

Goal: make SRapi governable by specs and quality gates.

Exit criteria:

- `specs/` exists and is linked from README.
- `specs/STATUS.md` identifies the next work package.
- Quality gates are documented.
- Current docs remain the source of architecture truth.

Primary packages:

- WP-000
- WP-010

## Phase 1: MVP Gateway Closure

Goal: complete the first real Gateway loop.

Required loop:

```txt
API Key -> Canonical AI Request -> Scheduler v1 -> OpenAI-compatible Adapter -> Usage Log -> Scheduler Decision -> Feedback
```

Exit criteria:

- `/v1/models`, `/v1/chat/completions`, `/v1/responses`, `/v1/messages` work against mock and OpenAI-compatible upstreams.
- Streaming and non-streaming paths are tested.
- Every Gateway request records usage, decision, feedback, and request id.
- OpenAPI, Ent, migrations, and generated SDKs are in sync.

Primary packages:

- WP-020 through WP-090

## Phase 2: Reverse Proxy Runtime v1

Goal: support the first safe non-API-key account runtime.

Exit criteria:

- `runtime_class != api_key` routes through Reverse Proxy Runtime.
- Per-account client, cookie jar, proxy, header hygiene, refresh lock, and risk classes exist.
- At least one reverse-proxy adapter is validated through mocks.
- Outgoing headers and body hygiene have automated tests.

Primary packages:

- WP-100 through WP-130

## Phase 3: Control Console And Ops v1

Goal: make SRapi operable by humans.

Exit criteria:

- Next.js console exists.
- Admin can manage providers, models, accounts, API keys, usage, decisions, and audit.
- Ops pages explain health, traffic, errors, providers, and scheduler decisions.
- Frontend uses generated TypeScript SDK.

Primary packages:

- WP-140 through WP-170

## Phase 4: Commercial Platform

Goal: add self-service monetization without weakening ledger integrity.

Exit criteria:

- Billing ledger is reliable and append-only.
- Subscriptions and plans are implemented.
- Payment provider instances, orders, webhook verification, fulfillment, refunds, and audit exist.
- Affiliate ledger and compensation are implemented after payment correctness.

Primary packages:

- WP-180 through WP-210

## Phase 5: Provider Expansion

Goal: broaden model and provider coverage through presets and adapters.

Exit criteria:

- OpenAI-compatible presets cover common providers.
- Anthropic-compatible presets are implemented.
- Gemini native routes and adapter are implemented.
- Grok/OpenRouter/aggregator variants use presets where possible.
- Model discovery and capability probes are available.

Primary packages:

- WP-220 through WP-250

## Phase 6: Production Hardening

Goal: make SRapi safe to operate long term.

Exit criteria:

- Backup and restore workflows exist.
- `/metrics`, SLOs, alerts, and notification channels exist.
- Data retention and cleanup jobs exist.
- Release smoke checks and rollback docs exist.
- Security review gates cover secrets, SSRF, prompt logging, CSRF, and credential encryption.

Primary packages:

- WP-260 through WP-290

## Phase 7: Advanced AI Surface

Goal: expose advanced endpoint families without compromising the core architecture.

Exit criteria:

- Images, embeddings, audio, rerank, moderation, websocket/realtime, and passthrough endpoints follow Gateway route matrix.
- Each endpoint still goes through auth, entitlement, scheduler, adapter, usage, and observability.
- Compatibility warnings and capability matching remain explicit.

Primary packages:

- WP-300 through WP-330

## Phase 8: Ecosystem And Polish

Goal: make SRapi extensible and pleasant to integrate.

Exit criteria:

- Public SDKs and examples exist.
- Plugin/provider extension docs exist.
- Migration guides from sub2api/CLIProxyAPI-style deployments exist.
- UI is complete, responsive, accessible, and consistent with the design system.

Primary packages:

- WP-340 through WP-360

