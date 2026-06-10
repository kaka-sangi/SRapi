# SRapi Roadmap

> **Status: all phases below are complete.** The program ran well past this roadmap — through ~WP-1310 (≈150 work packages beyond the WP-360 cap enumerated here), adding the 700-series Admin Control Plane, 900-series notifications, 1000-series OAuth/OIDC + TOTP 2FA, and the 1100–1300-series rate-limit matrix and frontend business-completeness work. See `STATUS.md` for the authoritative ledger and `WORK_PACKAGES.md` for the full package definitions.
>
> The phase narrative below is preserved as the historical plan that the project executed. Each phase records the exit criteria as **delivered**.

## Phase 0: Execution Foundation — done

Goal: make SRapi governable by specs and quality gates.

Exit criteria:

- `specs/` exists and is linked from README.
- `STATUS.md` identifies the next work package.
- Quality gates are documented.
- Current docs remain the source of architecture truth.

Primary packages:

- WP-000
- WP-010

## Phase 1: MVP Gateway Closure — done

Goal: completed the first real Gateway loop (shipped long ago).

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

## Phase 2: Reverse Proxy Runtime v1 — done

Goal: supported the first safe non-API-key account runtime (shipped long ago).

Exit criteria:

- `runtime_class != api_key` routes through Reverse Proxy Runtime.
- Per-account client, cookie jar, proxy, header hygiene, refresh lock, and risk classes exist.
- At least one reverse-proxy adapter is validated through mocks.
- Outgoing headers and body hygiene have automated tests.

Primary packages:

- WP-100 through WP-130

## Phase 3: Control Console And Ops v1 — done

Goal: made SRapi operable by humans.

Exit criteria (delivered; the `apps/web` Next.js console was later rebuilt and extended under WP-1310):

- Next.js console exists.
- Admin can manage providers, models, accounts, API keys, usage, decisions, and audit.
- Ops pages explain health, traffic, errors, providers, and scheduler decisions.
- Frontend uses generated TypeScript SDK.

Primary packages:

- WP-140 through WP-170

## Phase 4: Commercial Platform — done

Goal: added self-service monetization without weakening ledger integrity.

Exit criteria:

- Billing ledger is reliable and append-only.
- Subscriptions and plans are implemented.
- Payment provider instances, orders, webhook verification, fulfillment, refunds, and audit exist.
- Affiliate ledger and compensation are implemented after payment correctness.

Primary packages:

- WP-180 through WP-210

## Phase 5: Provider Expansion — done

Goal: broadened model and provider coverage through presets and adapters.

Exit criteria:

- OpenAI-compatible presets cover common providers.
- Anthropic-compatible presets are implemented.
- Gemini native routes and adapter are implemented.
- Grok/OpenRouter/aggregator variants use presets where possible.
- Model discovery and capability probes are available.

Primary packages:

- WP-220 through WP-250

## Phase 6: Production Hardening — done

Goal: made SRapi safe to operate long term.

Exit criteria:

- Backup and restore workflows exist.
- `/metrics`, SLOs, alerts, and notification channels exist.
- Data retention and cleanup jobs exist.
- Release smoke checks and rollback docs exist.
- Security review gates cover secrets, SSRF, prompt logging, CSRF, and credential encryption.

Primary packages:

- WP-260 through WP-290

## Phase 7: Advanced AI Surface — done

Goal: exposed advanced endpoint families without compromising the core architecture.

Exit criteria:

- Images, embeddings, audio, rerank, moderation, websocket/realtime, and passthrough endpoints follow Gateway route matrix.
- Each endpoint still goes through auth, entitlement, scheduler, adapter, usage, and observability.
- Compatibility warnings and capability matching remain explicit.

Primary packages:

- WP-300 through WP-330

## Phase 8: Ecosystem And Polish — done

Goal: made SRapi extensible and pleasant to integrate.

Exit criteria:

- Public SDKs and examples exist.
- Plugin/provider extension docs exist.
- Migration guides from sub2api/CLIProxyAPI-style deployments exist.
- UI is complete, responsive, accessible, and consistent with the design system.

Primary packages:

- WP-340 through WP-360

## Beyond the roadmap (WP-500 → WP-1310)

This roadmap's package enumeration stops at WP-360. The program kept going well past it; the following series shipped and are recorded individually in `STATUS.md` / `WORK_PACKAGES.md`:

- **700-series — Admin Control Plane:** dashboard snapshot, ops monitoring, typed settings, announcements, redeem codes, promo codes, risk-control APIs (WP-700), and console TOTP 2FA (WP-770).
- **900-series — Notifications:** low-balance notification triggers, outbox/email dispatch, admin-managed templates (WP-900).
- **1000-series — OAuth/OIDC identity:** external-identity sign-in and current-user pending-OAuth binding (WP-1000).
- **1100–1300-series — Egress, rate-limit matrix, and frontend completeness:** API-key egress consistency (WP-1100), TLS-fingerprint DB profiles (WP-1130), and the frontend business-completeness & correctness pass that rebuilt and fully wired the `apps/web` console (WP-1310).

### Roadmap / not yet implemented

Genuinely deferred items (tracked in `STATUS.md` and the relevant `docs/`) include Batch / Fine-tuning API families, full JA3/JA4 + HTTP/2 fingerprinting, and affiliate withdrawal. Everything else enumerated in the phases above is delivered.

