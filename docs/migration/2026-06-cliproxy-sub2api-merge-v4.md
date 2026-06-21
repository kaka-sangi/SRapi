# 2026-06 CLIProxyAPI + sub2api Merge — Wave 4

Wave 3 closed the deferred Antigravity reasoning replay and the
DeepSeek-style `max → xhigh` clamp (see
`2026-06-cliproxy-sub2api-merge-v3.md`). A deeper audit of both
upstreams surfaced two further gaps that are clear architectural wins
rather than UI / cosmetic deltas. This document scopes Wave 4.

## Decisions

### IN — port to SRapi

1. **Honor upstream-parsed `RetryAfter` in failover cooldown.**

   `provider_adapters/service/error_classification.go` already parses
   Anthropic `anthropic-ratelimit-unified-5h-reset` /
   `anthropic-ratelimit-unified-7d-reset` headers (via
   `anthropicRetryAfterFromHeaders`), Codex quota-window resets, and
   Gemini `retryDelay` into `ProviderError.RetryAfter` — a real
   `*time.Time`. But `runtime_gateway_failover.go:741-748` then calls
   `ClassifyUpstreamError(upstreamStatus, nil, err)` — without
   headers — and uses `decision.RetryAfterMs` for the cooldown. That
   classifier only reads the generic `Retry-After` header, which
   Anthropic does not send for the 5h/7d windows. So the cooldown
   falls back to the rate-limit module's local default (minutes) even
   though the real window is hours, and the account gets re-picked
   long before it can actually serve traffic.

   Fix: add a `providerGatewayRetryAfter(err)` accessor that pulls
   `ProviderError.RetryAfter` out, and use it at the cooldown call
   site. Falls back to the existing path when no parsed value is
   present, so behavior for Codex `Retry-After` 429s is unchanged.

   This is the architecturally cheapest fix that delivers the value
   sub2api's PR `de38d623` (Anthropic window preservation) and the
   CLIProxyAPI executor cooldown logic both signal as important.

2. **Antigravity 429 decision tree.**

   Both CLIProxyAPI (`antigravity_executor.go:decideAntigravity429`)
   and sub2api (`classifyAntigravity429`) distinguish four 429
   shapes from the upstream body:

   - `instant_retry_same_auth` — `RATE_LIMIT_EXCEEDED` with very
     short `retryDelay` (< 3s). Stay on the same account.
   - `short_cooldown_switch_auth` — `RATE_LIMIT_EXCEEDED` with a
     bounded `retryDelay` (< 5m). Cool the account down for the
     retry window and rotate.
   - `full_quota_exhausted` — `QUOTA_EXHAUSTED` reason OR a
     `retryDelay` longer than the short cooldown threshold. Park the
     account for a long cooldown (the real reset window).
   - `soft_retry` — `RESOURCE_EXHAUSTED` without a structured
     reason. Generic short retry.

   SRapi currently treats every Antigravity 429 as the same generic
   transient cooldown, so a quota-exhausted account is re-picked
   after a short delay and fails again. We add a classifier next to
   the Antigravity adapter, populate the parsed `retryDelay` into
   `ProviderError.RetryAfter`, and let the wave-4 cooldown plumbing
   (item 1) do the rest. No new module — the wiring point is the
   existing `classifyGeminiProviderHTTPErrorWithHeaders` path that
   the Antigravity adapter already routes through.

### OUT — not real gaps after verification

- **OAuth signup auto-apply promo code** (sub2api `e4ccb75d`). The
  sub2api code reads a promo code from a query-string parameter the
  OAuth start step embeds in `state`. SRapi's OAuth surface uses a
  hash-only pending session and `srapi_oauth_pending` cookie; adding
  promo-code support requires a product decision on where the input
  comes from (query param, redirect URL, signup form after bind).
  Not a gap to close blindly; flag for product.

- **SSE error event body capture in ops logs** (sub2api `6c7203d8`).
  SRapi's `gatewayUpstreamErrorEvent` already records
  `ProviderErrorBodyExcerpt` from the provider error (the stream-side
  classifier surfaces `event: error` frames as terminal errors with
  the JSON body intact — see `service_test.go:2973` etc.). Captured
  via the standard failover error event, not the truncated path.

- **Anthropic 429 *window preservation* across rotations** (sub2api
  `de38d623`). Functionally covered by item 1: once cooldown duration
  matches the real reset, account-rotation behavior naturally
  preserves the multi-hour pause — the cooldown service already
  enforces it per-account. The sub2api code also added a hard
  "this rule takes precedence over user temp-unsched rules" gate;
  SRapi's temp-unsched lives in a separate operator-controlled module
  and the cooldown service is independent, so the conflict doesn't
  arise.

- **CLIProxyAPI plugin host system** (16K lines). SRapi's
  architecture explicitly uses first-class modules instead of
  out-of-process plugins (`FINAL_STATE` and `MODULE_INTERFACE_CONTRACTS`
  invariants). Porting would require a parallel extension surface
  that contradicts the existing extensibility model. Reject.

- **CLIProxyAPI dedicated WebSocket relay system**. Already covered
  by SRapi's `RelayWebSocket` primitive (WP-390 / WP-410 / WP-470 /
  WP-630 — see `STATUS.md`).

- **CLIProxyAPI file watcher + auth sync**. Already covered by
  `platform/configwatch` and the `accounts_token_refresh` worker.

- **CLIProxyAPI redisqueue usage tracking**. SRapi's usage module +
  domain-events outbox already pipelines usage with the same
  guarantees and with persistent durability across restarts.

- **grok-composer session isolation**. SRapi's `realtime.go` already
  sets `x-grok-conv-id` from request settings; the gap CLIProxyAPI
  closes is only the auto-generated UUID fallback when the caller
  doesn't supply one. The realtime path already has a fallback via
  `conversation_id` / `prompt_cache_key` lookup which is the
  better-shaped contract (caller-controlled). Reject.

## Order of work

1. **`providerGatewayRetryAfter` accessor + failover plumbing**
   (item 1 — accessor in `runtime_gateway_core.go`, one call-site
   edit in `runtime_gateway_failover.go`, regression test for the
   Anthropic 5h-reset case driving a 5-hour cooldown). One commit +
   push.

2. **Antigravity 429 decision tree** (item 2 — new file
   `antigravity_429_classifier.go`, wire into
   `classifyGeminiProviderHTTPErrorWithHeaders` so the existing
   Antigravity adapter automatically inherits it, tests for each
   decision shape). One commit + push.

`make check` at the end of the wave gates the push of the final
commit.

## Non-goals

- No new modules. Both items fit inside existing module boundaries.
- No frontend surface — the changes are internal to the failover and
  cooldown paths.
- No deprecation of the existing fallback paths — items keep the
  generic `Retry-After` / generic 429 behavior as the default when
  no upstream signal is available.
