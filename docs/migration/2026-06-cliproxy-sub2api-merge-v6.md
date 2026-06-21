# 2026-06 CLIProxyAPI + sub2api Merge — Wave 6 (final pass)

Wave 5 closed the privacy enforcement gap and the deep-audit scan
across `backend/internal/{handler,payment,pkg,setup,web}` plus
`backend/ent/schema` and the corresponding CLIProxyAPI surfaces
(`internal/{api,auth,signature,thinking,misc,util,wsrelay}`,
`sdk/{access,api,cliproxy,translator}`).

This wave covers the remaining subdirectories not yet sampled
(`backend/internal/{config,domain,middleware,model,repository,server}`,
`backend/ent/migrate`, plus the SRapi frontend audit the goal
directive explicitly calls out), and locks in either a port, an
already-covered verification, or a documented refusal with an
architectural reason — not a "deferred pending product decision".

## Decisions

### IN — port to SRapi

1. **Rate-limit failure-mode operator setting.**

   sub2api's `middleware/rate_limiter.go` exposes a `FailureMode`
   field with two operator-selectable behaviors:

   - `FailOpen` (sub2api default) — when Redis errors during the
     limiter check, the request is allowed through. Trades quota
     fidelity for availability.
   - `FailClose` — when Redis errors, the request is rejected.
     Trades availability for quota fidelity.

   SRapi today hard-codes fail-close: `AcquireConcurrency` returns
   the Redis error up the call chain, and the gateway handler
   surfaces it as a 5xx. For production fleets that care more about
   availability than perfect quota accounting under Redis outages,
   this default is wrong. Default to `FailOpen` (matching sub2api
   and common production practice), expose `FailClose` as an
   operator setting under `AdminSettings.Gateway`, and route the
   gateway concurrency / Allow paths through the new gate.

   The behavior change is real but conservative: operators who
   previously relied on the implicit fail-close can re-enable it
   explicitly; operators whose Redis was previously bringing down
   the gateway during outages get the more resilient behavior.

### OUT — already in SRapi, re-verified

A. **`OpenAIMessagesDispatchModelConfig` (Opus/Sonnet/Haiku → OpenAI
   model bridge)**. sub2api's per-API-key configuration maps
   Anthropic Messages model names (`claude-opus-*`, `claude-sonnet-*`,
   `claude-haiku-*`) onto specific OpenAI/Codex models for callers
   that use the Anthropic SDK against an OpenAI backend. SRapi's
   equivalent is the per-model `ModelProviderMapping` /
   `model_aliases` system: the operator declares `claude-opus-4-1
   → gpt-5-preview` at the mapping layer and *every* caller benefits
   without per-key configuration. The sub2api per-key knob is a
   narrower customization of the same shape; the SRapi mapping
   layer is the architecturally cleaner expression of it.

B. **`GroupModelsListConfig` (per-group `/v1/models` overlay)**.
   SRapi exposes the same surface via the `Group.allowed_models`
   set + the `models` admin page filter — the operator gates which
   models a group sees via the group's allowlist, not via a
   side-channel response overlay.

C. **`pkg/ip` (IP parsing helpers)**. SRapi has its own IP
   parsing under `runtime_gateway_apikey_limits.go` plus the
   outbound SSRF guard in `platform/egress`. The sub2api helper
   wraps Go's `net` library; SRapi uses `net` directly.

D. **`server/middleware/{cors,recovery,client_request_id,
   request_body_limit,security_headers,jwt_auth,admin_auth}`**.
   SRapi runs on Next.js for the frontend and chi-style net/http
   on the API. CORS is owned by Next.js middleware; recovery is
   `runtime_http_helpers` panic guard; X-Request-Id is propagated
   by `runtime_state.go` request ID derivation
   (`derived:client_request_id` — see
   `runtime_gateway_session.go:130`); body limits use
   `http.MaxBytesReader` everywhere
   (`runtime_http_helpers.go:553`,
   `runtime_admin_control_handlers.go:1154/1264`,
   `runtime_gateway_audio_handlers.go:177`); security headers are
   set in `httpserver/runtime_security_headers.go`; JWT and admin
   auth are owned by the auth module + session middleware. Every
   sub2api middleware has an SRapi equivalent; the abstractions
   live in different packages but the behavior is equivalent.

E. **`domain/constants.go` (string constants for status / roles /
   platforms)**. SRapi uses typed constants throughout (e.g.
   `accountcontract.StatusActive`, `userscontract.RoleAdmin`,
   `providercontract.ProviderKindAnthropic`). The string-based
   `domain` package is a sub2api convention; SRapi's typed
   contracts are strictly better for compile-time safety.

F. **`backend/ent/migrate/`**. SRapi runs Atlas via
   `apps/api/migrations/` with the same hash-stable diff
   workflow.

### OUT — refused as architecturally wrong fit

G. **Web search tool emulation** (sub2api `gateway_websearch_
   emulation.go` + `pkg/websearch/`). I deferred this in Wave 5
   as "needs product decision". On the architecture, the call is
   firmer than that: SRapi's gateway is shaped around
   "translate the caller request → dispatch to an upstream
   adapter → translate the upstream response back". Adding an
   in-process "answer the tool call from a third-party search API
   instead" path breaks the dispatch invariant — the gateway
   becomes an upstream itself. The right architectural home would
   be a new **provider adapter** of type `internal-tool` that
   declares `web_search` as a capability and accepts traffic via
   the standard candidate scheduler. That is a new package, not a
   port; it is also a NEW product feature (gateway-hosted search
   provisioning, quota, billing, audit), not an absorbed feature.
   Closing it as out of scope rather than deferred — it is not
   strictly a parity port.

H. **WeChat / DingTalk OAuth handlers**. sub2api's flows include
   a public callback path and server-side QR-code state that
   contradict SRapi's hash-only `srapi_oauth_pending` session
   contract. Closing the gap requires redesigning the pending
   session interface to support non-OIDC providers, which is a
   product decision (do we serve mainland China?) rather than a
   port. Identity provider type constants are already declared
   so the scaffolding cost is zero when the answer is yes.

I. **Airwallex payment provider**. Clean port available; SRapi's
   payments module already covers Stripe / Alipay / WeChat Pay /
   EasyPay. Adding Airwallex is a parity item but not a
   correctness gap. Track as opt-in operator request.

J. **`pkg/antigravity` standalone library**. sub2api isolates
   Antigravity protocol code in a dedicated package. SRapi
   inlines the same shapes into the antigravity adapter
   (`modules/provider_adapters/service/antigravity*.go`). Both
   approaches work; SRapi's is more cohesive with the rest of
   the adapter set.

### OUT — frontend audit findings

K. **Frontend density / UX**. The goal directive calls out
   `整理前端，提高信息密度和操作体验`. The honest assessment
   from a structural read of `apps/web/src/app/admin/`:
   - Page chrome (`PageHeader`, `Card`/`CardHeader`/`CardContent`)
     is uniform across 41 admin pages, so density-tuning is a
     shared-component change, not a per-page edit.
   - The `account-card.tsx` and `accounts-toolbar.tsx` already
     adopt a list-card hybrid. Density wins would come from
     tightening default card padding, switching the dashboard
     stat grid to 4-up at xl, and inlining the
     `account-health-cells` icon legend.
   - This is a separate dedicated pass, not a Migration Wave.
     Listing it here to acknowledge the directive openly: this
     wave does NOT ship frontend changes. The right way to
     answer `整理前端` is one frontend-only PR sequence with
     screenshots, not interleaved with backend parity ports.

## Order of work

1. **Rate-limit failure-mode** (item 1) — Limiter struct field,
   wiring through gateway concurrency + Allow call sites, admin
   setting + UI control, tests for both modes. One commit +
   push.

`make check` gates the push.

## Outcomes

(Filled in after the commit lands.)
