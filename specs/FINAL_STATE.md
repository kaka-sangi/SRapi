# SRapi Final State

## 1. Product Definition

SRapi is a self-hosted AI API Gateway and management platform for the fast-changing AI model ecosystem.

The final product is not a simple multi-provider proxy. It is a platform that gives operators one stable control plane for:

- AI endpoint compatibility
- account-pool management
- adaptive scheduling
- provider and model capability governance
- reverse-proxy runtime isolation
- usage accounting and billing
- operations, alerts, and audit
- modern user and admin consoles

## 2. Platform Planes

### 2.1 Gateway Plane

The Gateway Plane exposes client-facing AI endpoints:

- OpenAI-compatible `/v1/models`
- OpenAI-compatible `/v1/chat/completions`
- OpenAI Responses `/v1/responses`
- Anthropic-compatible `/v1/messages`
- Provider-prefixed route aliases
- Phase 2+ passthrough/media endpoints: embeddings, images, audio, rerank
- Phase 3+ realtime and websocket endpoints

Every AI request must enter:

```txt
Client Endpoint Adapter -> Canonical AI Request -> Scheduler -> Provider Adapter -> Client Response Renderer
```

Gateway handlers must not pick accounts directly.

### 2.2 Canonical AI Core

Canonical AI IR is the internal contract for endpoint conversion.

It must preserve:

- source protocol
- source endpoint
- response protocol
- model and canonical model
- messages and content blocks
- tool calls and tool results
- reasoning controls
- structured output intent
- streaming intent
- compatibility warnings
- request and session hashes

Lossy conversion must be explicit through warnings or validation errors.

### 2.3 Scheduler Plane

The Scheduler is provider-neutral and capability-aware.

It owns:

- RequestProfile
- CapabilityResolver input
- CandidateBuilder
- hard filters
- score engine
- lease acquisition
- decision records
- feedback ingestion
- sticky and cache affinity hooks
- strategy registry

It must produce auditable `scheduler_decisions` and `scheduler_feedbacks` for every Gateway attempt.

### 2.4 Provider Runtime Plane

Provider Adapter is the extension point for upstream protocols.

Supported families:

- `openai-compatible`
- `anthropic-compatible`
- `gemini-compatible`
- native OpenAI
- native Anthropic
- native Gemini
- native Grok
- OpenRouter and similar aggregator presets
- reverse-proxy adapters for web, desktop, CLI, IDE, and custom runtimes

Adapter code must stay out of Scheduler scoring logic.

### 2.5 Reverse Proxy Runtime Plane

All `runtime_class != api_key` accounts must use Reverse Proxy Runtime.

The final runtime supports:

- per-account HTTP client
- per-account TLS session context
- per-account HTTP/2 connection pool
- per-account encrypted cookie jar
- per-account egress profile
- per-account proxy binding
- OAuth/device-code refresh with distributed locks
- header hygiene
- body hygiene
- SSE/WSS stream relay
- risk and ban signal classification
- challenge integration points
- behavior pacing

SRapi must not include ToS bypass logic, CAPTCHA cracking, credential scraping, or hardcoded third-party secrets.

### 2.6 Control Plane

The Control Plane manages:

- users and roles
- sessions and CSRF
- API keys
- providers
- provider presets
- model registry
- model aliases
- provider model mappings
- pricing rules
- provider accounts
- account groups
- settings
- audit logs

All public HTTP contracts are OpenAPI-first.

### 2.7 Commercial Plane

The Commercial Plane manages:

- usage logs
- billing ledger
- balances
- subscriptions
- payment provider instances
- payment orders
- webhooks
- refunds
- affiliate relationships
- affiliate ledger

Ledger-like data is append-only. Refunds and affiliate compensation use reverse entries, not destructive updates.

### 2.8 Operations Plane

The Operations Plane explains system behavior.

It must answer:

- Which account was selected and why?
- Which accounts were rejected and why?
- Which provider/model/group is failing?
- Is the failure client, business, scheduler, provider, reverse-proxy, or internal owned?
- What is the TTFT, stream completion rate, token throughput, cost, and margin?
- Are reverse-proxy accounts showing session, challenge, device, or geo risk?

Minimum final surfaces:

- `/livez`
- `/readyz`
- `/metrics`
- `/api/v1/health`
- admin ops overview
- provider health matrix
- scheduler decision explorer
- error fingerprints
- SLO and burn-rate alerts

### 2.9 Web Console Plane

The final web app is a production console, not a landing page.

It must include:

- user dashboard
- API key management
- usage and billing views
- provider/account/model administration
- scheduler simulator and decision stream
- ops dashboard
- payment administration
- settings and audit

Visual direction follows `docs/FRONTEND_DESIGN_SYSTEM.md`.

## 3. Non-Negotiable Invariants

- OpenAPI is the HTTP source of truth.
- PostgreSQL is the durable source of truth.
- Redis state must be rebuildable.
- API keys are shown once and stored only as HMAC hashes.
- Provider credentials, cookies, proxy secrets, payment config, and device fingerprints are encrypted.
- Scheduler must not decrypt credentials.
- Provider Adapter must not perform user auth or billing decisions.
- Gateway must not directly query Ent or another module's repository.
- Module-to-module calls use `contract` packages.
- Generated code is not manually edited.
- Logs, metrics, audit, and scheduler records must not contain full prompts, API keys, OAuth tokens, cookies, or provider credentials.

## 4. Reference Baseline

SRapi should absorb the useful lessons from `sub2api` and `CLIProxyAPI`, but not clone their internal architecture.

`sub2api` informs the operator and commercial platform.

`CLIProxyAPI` informs the protocol, OAuth, executor, and CLI runtime behavior.

SRapi's final architecture is stricter:

- Canonical AI IR instead of pairwise endpoint sprawl.
- Capability taxonomy instead of ad hoc support flags.
- Scheduler decisions as durable evidence.
- Reverse Proxy Runtime as an isolated runtime plane.
- Domain events and module contracts instead of cross-module coupling.

