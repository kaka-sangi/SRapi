# Reference Project Decisions

## 1. Purpose

SRapi uses the sub2api, CLIProxyAPI, and chatgpt2api reference projects as references, not templates. These are private upstream references, not shipped with SRapi.

This file records what to adopt, what to improve, and what to avoid.

## 2. Lessons From sub2api

Adopt:

- Operator-grade account pool management.
- User and API key distribution.
- Group-based access and pricing policies.
- Usage logs and dashboards.
- Subscription, payment, redeem code, promo code, and affiliate concepts.
- Ops dashboards, error logs, alerting, backup, restore, and update workflows.
- Proxy binding and TLS/header fingerprint ideas.
- Account testing, recovery, cooldown, quota, and model availability workflows.

Improve:

- Keep Gateway, Scheduler, Billing, Provider Adapter, and Reverse Proxy Runtime as separate module boundaries.
- Make Scheduler decisions first-class durable evidence.
- Use Canonical AI IR rather than direct endpoint-to-endpoint special cases.
- Use Capability Taxonomy rather than scattered feature booleans.
- Use OpenAPI as the only HTTP source of truth.
- Use module contracts instead of cross-module repository access.

Avoid:

- Provider-specific account selection in handlers.
- Business rules hidden in admin handlers.
- Float values for real billing amounts.
- Sensitive credentials in generic JSON without encryption boundaries.
- Letting commercial features block the correctness of the core Gateway.

## 3. Lessons From CLIProxyAPI

Adopt:

- Strong protocol compatibility for OpenAI, Responses, Anthropic Messages, Gemini, Codex, Claude Code, and related CLI clients.
- OAuth/device/login flows as account onboarding patterns.
- Runtime auth state and refresh lifecycle concepts.
- Session affinity and model aliasing.
- Streaming and WebSocket edge-case handling.
- Executor/adapter registry style.
- Request/response translator tests.
- Embeddable runtime thinking where useful.

Improve:

- PostgreSQL is the durable account source of truth, not local files.
- File watchers are optional import/runtime helpers, not control-plane state.
- Management APIs are OpenAPI-first and RBAC-protected.
- Usage and decisions are durable and queryable.
- Runtime auth state is represented through Provider Account and Reverse Proxy Runtime contracts.
- Provider behavior feeds Scheduler feedback and Ops signals.

Avoid:

- Letting config YAML become the main database.
- Global mutable runtime state without durable projection.
- Management routes that bypass the SRapi auth/RBAC model.
- Per-provider translators that cannot converge through Canonical AI IR.

## 4. Lessons From chatgpt2api

Adopt:

- ChatGPT Web upstream simulation through access tokens, browser-style headers, OAI device/session IDs, Sentinel requirements, and backend API routes.
- OpenAI-compatible downstream rendering over ChatGPT Web conversation and SSE upstream behavior.
- Account-scoped fingerprint, proxy, and requirements-token handling ideas.

Improve:

- Keep ChatGPT Web behavior inside Provider Adapter and Reverse Proxy Runtime boundaries.
- Make account state durable through Provider Account contracts instead of ad hoc service globals.
- Preserve Scheduler decision, usage, and account feedback evidence for every call.

Avoid:

- Treating ChatGPT Web 2api as normal OpenAI API-key dispatch.
- Letting Web-specific request state leak into Gateway-local DTOs.
- Bypassing SRapi auth, entitlement, billing, audit, or Scheduler evidence.

## 5. SRapi Decision Table

| Concern | sub2api reference | CLIProxyAPI reference | SRapi decision |
| --- | --- | --- | --- |
| Users/API keys | Strong platform feature | Minimal/local API keys | Implement platform model with hashed keys and group scopes. |
| Provider accounts | Rich database-backed accounts | File-backed OAuth/runtime auth | Database-backed Provider Account plus importers/runtime materializers. |
| Routing | Group/platform routing | Provider/model selector | Scheduler v1 owns all account selection. |
| Endpoint compatibility | Many handlers and bridges | Strong translators | Canonical AI IR plus client renderers and provider adapters. |
| Reverse proxy | TLS profiles and anti-risk ideas | CLI OAuth/runtime executors | Dedicated Reverse Proxy Runtime with account isolation. |
| Observability | Admin/Ops dashboard | request logs and management API | AI-native Ops Plane with decisions, feedback, SLO, risk signals. |
| Payments | Built-in commercial system | Out of scope | Delivered a dedicated commercial plane (payments, billing, subscriptions, affiliate) layered on top of a correct Gateway. |
| Frontend | Full admin dashboard | optional management UI/TUI | Delivered a modern console (apps/web) built on the generated SDK and the SRapi design system. |

For ChatGPT Web specific reverse-proxy behavior, the `chatgpt2api` reference project is the source reference for upstream browser/OAI/Sentinel request simulation and compatible rendering.

## 6. Rule For Future References

When copying an idea from a reference project, Codex must answer:

1. Which SRapi module owns this behavior?
2. Which contract exposes it?
3. Which docs define its rules?
4. Which tests prove it?
5. Does it preserve OpenAPI-first, Provider-neutral Scheduler, and credential safety?

If any answer is unclear, implement the abstraction first, not the feature shortcut.
