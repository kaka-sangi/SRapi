# 2026-06 CLIProxyAPI + sub2api Merge — Wave 5

The previous waves shipped 15 cross-cutting parity items and rejected
13 more with documented rationale. A deeper subdirectory-by-subdirectory
audit surfaced one further architecturally clean port and confirmed
that the remaining surface area is either already-covered, out of
scope by SRapi invariants, or large enough to need a product decision
before code.

This document is the binding scope for Wave 5 and the closeout
record for the rest of the audit.

## Decisions

### IN — port to SRapi

1. **Antigravity privacy enforcement.**

   `setUserSettings` + `fetchUserInfo` on every successful Antigravity
   OAuth refresh disables upstream telemetry (Google's "share
   usage data with Google Cloud" toggle) for the account. Without
   this, every request through a managed Antigravity account
   contributes to the upstream's training/telemetry pipeline — a
   real correctness problem for operators serving sensitive
   workloads.

   sub2api's two-step flow (see
   `backend/internal/service/antigravity_privacy_service.go`):
   - POST `…/v1internal/users/me:setUserSettings` with an empty body;
     expect a `{"userSettings":{}}` response.
   - POST `…/v1internal:fetchUserInfo` with the project_id; expect
     the response's `userSettings` to lack `telemetryEnabled`.

   Port plan:
   - Helper `EnforceAntigravityPrivacy(ctx, accessToken, projectID,
     proxyURL) (status, error)` inside the reverse-proxy package
     since it already owns Antigravity HTTP plumbing (uTLS, proxy
     binding, headers).
   - Hook the helper into the `accounts_token_refresh` worker
     post-refresh path, gated on
     `provider.AdapterType == "reverse-proxy-antigravity"`.
   - Persist `privacy_mode` into `account.Metadata` so the next
     refresh skips when the state is already `privacy_set`.
   - Unit tests for the helper (HTTP stub for both endpoints) and
     for the worker integration (skip-when-set, retry-on-failure,
     no-op for non-antigravity).

### OUT — deferred with rationale

The deep audit found the following candidates. Each is genuinely
useful in the upstream's context but does not pass SRapi's
architectural gate without further product or scope decisions.

A. **Airwallex payment provider** (~640 lines). sub2api ships it
   under `backend/internal/payment/provider/airwallex.go`. SRapi's
   payments module already supports Stripe (international),
   Alipay, WeChat Pay, EasyPay — operationally covers global and
   CN. Adding Airwallex broadens the choice but does not fill a
   correctness gap. The clean port would fit the existing
   `modules/payments/providers/<vendor>` pattern; the work is
   straightforward when an operator with Airwallex revenue asks
   for it.

B. **WeChat / DingTalk / LinuxDo custom OAuth handlers** (~2 800
   lines). sub2api implements each provider end-to-end including
   WeChat's non-standard QR-code flow, DingTalk's H5 cookies, and
   the LinuxDo OAuth 2 dialect. SRapi already declares the
   identity provider type constants and routes OIDC-compliant
   providers through the generic `runtime_oauth_handlers.go`. The
   WeChat flow specifically requires server-side QR-code state
   storage and a public callback path that conflicts with SRapi's
   hash-only pending-session contract. Closing this gap requires
   a deliberate redesign of `pending_oauth.go` to support the
   non-OIDC flows; the right time is when a customer needs it,
   not preemptively.

C. **Web search tool emulation** (~1 K lines: sub2api's
   `gateway_websearch_emulation.go` + `pkg/websearch/` with
   Brave / Tavily providers + Redis-backed quota). sub2api
   intercepts `web_search` / `google_search` tool calls and
   answers them in-process so the caller can use web search
   across all upstreams. This is a NEW dispatch path: today
   SRapi's gateway shape is "translate → upstream → response".
   Adding "translate → in-process web search → synthesize
   tool_result frames" is a new kind of admission decision. The
   correct architectural fit is unclear: it could live as a
   pre-dispatch interceptor inside the gateway, as a new
   provider-adapter pseudo-runtime, or as part of the content
   safety pipeline. We will not pick one without a product
   decision on whether SRapi should ship its own search
   capability vs. delegating to upstream.

D. **OAuth signup auto-apply promo code** (sub2api `e4ccb75d`).
   Same reasoning as wave 4: sub2api passes the promo through
   OAuth `state`; SRapi's hash-only pending-session cookie
   contract needs a separate channel. Needs product.

E. **Content moderation Cyber compliance** (sub2api
   `content_moderation_cyber_test.go`). The Cyberspace
   Administration ledger schema. Already rejected as region-
   neutral invariant in Wave 2.

F. **CLIProxyAPI homekv, pluginhost, pluginstore, wsrelay,
   safemode, htmlsanitize, browser, managementasset** — every
   one of these is either an architecturally-distinct
   extensibility surface (plugin contracts), a CLI-only utility
   (TUI / browser-driven OAuth bootstrap), or covered by an
   SRapi equivalent already (RelayWebSocket, configwatch).

### Already-covered (re-verified during this audit)

- 499 client-closed-request — `runtime_gateway_websocket.go:30`.
- Content safety integration in gateway hot path —
  `runtime_gateway_content_safety_test.go`.
- TLS fingerprint profiles — `modules/tls_profiles/`.
- DingTalk / WeChat / LinuxDo / GitHub / Google identity types —
  `modules/users/contract/contract.go` (types only; provider-
  specific handlers still deferred per item B).
- Error passthrough rules — `ent/schema/errorpassthroughrule.go`.
- Payment audit log — `ent/schema/paymentauditlog.go`.

## Order of work

1. **Antigravity privacy enforcement** (single commit + push):
   helper + worker hook + persistence + tests.

`make check` gates the push.

## Outcomes

Shipped:

- Antigravity privacy enforcement runs after every successful OAuth
  refresh. Telemetry stays off for managed accounts; failure persists
  `privacy_mode=privacy_set_failed` so the next refresh pass retries.
- Existing antigravity refresh + chat integration tests extended to
  exercise the new endpoints alongside the refresh flow.

Refused as out of scope for this wave (see Decisions section): the
audit's four remaining candidates (Airwallex, WeChat/DingTalk OAuth,
web search emulation, OAuth signup promo) remain documented for
future product-driven scope decisions.
