# 2026-06 CLIProxyAPI + sub2api Merge — Wave 3

Wave 1 (commit `1ceb1de4`) absorbed Vertex AI, per-(account, model) cooldown,
GCP project rotation, Codex reasoning replay, OpenAI Moderation, maintenance
mode. Wave 2 (see `2026-06-cliproxy-sub2api-merge-v2.md`) extended cooldown
to 5xx/408, gated thinking-strip by model family, added per-account
temp-unschedulable, and folded scheduler-snapshot cleanup into the retention
worker.

Wave 2 deferred two items with rationale:

- **Antigravity reasoning replay** — scope too large to land safely inside
  the wave-2 commit budget. This wave owns it.
- **OpenAI rate-limit reset credit** — needs a live OAuth-token harness to
  verify; remains deferred for the same reason as wave 2.

This document is the binding scope for the work done in this wave.

## Decisions

### IN — port to SRapi

1. **Antigravity reasoning replay** (deferred wave-2 item 1).

   The upstream Antigravity reverse-proxy path re-generates `thoughtSignature`
   blocks every turn for Gemini-shaped models. When the signature drifts the
   upstream returns `400` with a "thoughtSignature"-bearing body and SRapi's
   `gemini_signature_retry.go` silently downgrades the thinking blocks to
   plain text. The CLIProxyAPI reference pre-injects the prior turn's
   `thoughtSignature` + `function_call_part` items into the next request so
   the upstream accepts the continuation byte-identically.

   The wire shape differs from the Codex replay we already ship: items are
   keyed by `(contentIndex, partIndex)` against `request.contents[*].parts[*]`
   instead of the OpenAI Responses `input` array. The normalizer therefore
   needs separate item types (`thought_signature`, `function_call_part`),
   and the cache instance must be independent from the existing
   `CodexReasoningReplayCache` (the session-key namespace differs).

   Port plan (mirrors the Codex port architecturally):

   - `antigravity_reasoning_replay_cache.go` — bounded LRU + sliding TTL
     cache, in-memory only (no `homekv` dep). Same defaults as the
     reference (10k entries, 1h TTL, 128-batch evict). Item normalizers
     for the two shapes; the on-the-wire byte stream stays equivalent.
   - `antigravity_reasoning_replay.go` — scope derivation
     (`sessionId` from payload → `metadata[execution_session]` →
     hash of message text), payload pre-merge (insert cached items into
     `request.contents` honoring `contentIndex`/`partIndex` and skipping
     items already present in the live payload), and SSE / non-stream
     response capture into the cache.
   - Wire-up in `provider_adapters/service/antigravity.go`:
     - Before each request: call the prepare hook to inject cached items.
     - After a 2xx response: capture `thoughtSignature` + `functionCall`
       items by walking `response.candidates.0.content.parts`. Streaming
       path observes each SSE frame.
     - On a 4xx/5xx response: if the body matches the signature-failure
       fingerprint, clear the scope's cache entry (so the next turn falls
       back to a fresh signature instead of replaying a poisoned one).
   - Gate by upstream model family: `gemini-*` / `flash-*` / `agent-*`
     (matches CLIProxyAPI `antigravityUsesReasoningReplayCache`). Claude
     family is excluded — Antigravity-on-Claude has its own thinking
     replay path through the existing signature wiring.

2. **`reasoning_effort: "max"` normalization to `"xhigh"`.**

   sub2api commit `142d8c36` documents that DeepSeek and a handful of
   Chinese-LLM upstreams accept `xhigh` but reject `max` as the highest
   reasoning bucket. SRapi already routes `"xhigh"` through the gateway
   correctly (see `service.go` `reasoningEffortForBudget`) but accepts
   `"max"` from callers without normalizing. Adapter-side mapping is too
   late — quota gating happens before the adapter runs and uses the raw
   value. Normalize at gateway admission so callers asking for `"max"`
   land in the same `xhigh` bucket as callers asking for `"xhigh"`.

### OUT — already in SRapi, not a real gap

- DeepSeek / Kimi / GLM / MiniMax fallback pricing (sub2api commit
  `a4ce7339` family). SRapi's billing module already supports
  per-family fallback pricing rules via `pricing_equivalence` (see
  `modules/billing/service/pricing_equivalence_test.go`); seeding
  vendor-specific rates is a deploy-time operator concern, not a code
  change.

- MiniMax M-series `thinking.type=enabled → adaptive` rewrite
  (sub2api `56c6325d`). SRapi's `thinking_protocol_family.go` already
  classifies `minimax-m*` as a passback-required family and
  `conversation_protocols.go` already emits `adaptive` for that family.

- Anthropic-compatible thinking-block protocol-aware filtering
  (sub2api `6baf00d7`). SRapi's wave-2 model-family gate covers the
  same behavior (the wave-2 fix landed in `claude_signature_wiring.go`).

- Account ID display in admin list (sub2api `eba9bea9`). SRapi's
  admin accounts grid already shows the account `id` column.

- IP ACL denial message includes client IP (sub2api `56c62c59`).
  Out of scope — SRapi's gateway `writeGeminiGatewayError` and
  `runtime_gateway_apikey_limits.go` deny path explicitly does not
  echo the caller IP in the response body to avoid information
  leakage to scrapers. Operators see the IP via audit log entries.

### OUT — deferred for the next wave with the same wave-2 rationale

- OpenAI rate-limit reset credit (Wave 2 item 3). Same blocker — needs
  a live ChatGPT/Codex OAuth token harness.

## Order of work

1. **Antigravity reasoning replay cache** — new file
   `antigravity_reasoning_replay_cache.go` (LRU + TTL + normalizers),
   plus unit tests for cache eviction, sliding TTL, and item
   normalization. One commit + push.

2. **Antigravity reasoning replay wiring** — new file
   `antigravity_reasoning_replay.go` (scope, prepare, capture,
   clear-on-failure), wired into `antigravity.go` for non-stream,
   stream, and image-generation pre/post points. Unit tests for the
   merge / filter / SSE accumulator logic. One commit + push.

3. **Reasoning-effort `max → xhigh` normalization** at gateway
   admission. One small commit + push.

`make check` at the end of the wave gates the push of the final commit.

## Non-goals

- No new architectural layers. Cache and wiring live next to the
  existing Codex replay files inside `modules/provider_adapters/service`.
- No frontend surface — the admin web layer needs nothing new for
  this wave. (Replay state is implementation-internal; surfacing it
  would invite operators to inspect / clear it from the UI, which we
  do not currently want.)
- No KV-backed cache backend. The CLIProxyAPI reference supports a
  `homekv`-backed mode for multi-process deployments; SRapi runs the
  reasoning replay caches in-process today and the existing Codex
  cache made the same call. A future package can add a shared backend
  to both caches behind the same interface if scale demands it.
