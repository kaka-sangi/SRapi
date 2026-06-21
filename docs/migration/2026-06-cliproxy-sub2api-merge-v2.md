# 2026-06 CLIProxyAPI + sub2api Merge — Wave 2

The first merge wave (see commit `1ceb1de4`) absorbed Vertex AI, per-(account, model)
cooldown, GCP project rotation, Codex reasoning replay, OpenAI Moderation pass, and
the maintenance-mode gate. This document defines the second wave triggered by the
2026-06-21 upstream pulls (`CLIProxyAPI bbef8da4..369e560f`,
`sub2api e34ad2b1..945b9b20`).

The waved analysis lives in the parallel `claude/agent_*` debug ledger; this file
is the binding scope: what we adopt, what we reject, and the order of work.

## Decisions

### IN — port to SRapi

1. **Antigravity reasoning replay.** SRapi already ports `CodexReasoningReplayCache`
   (Codex CLI → OpenAI Responses). The Antigravity reverse-proxy path has no
   equivalent, so multi-turn Gemini-via-Antigravity re-generates thought blocks
   every turn. CLIProxyAPI added a sibling implementation keyed by
   `(model, sessionKey)`. We port it alongside the Codex cache, wired into
   `provider_adapters/service/antigravity.go`.

2. **Transient-error cooldown for 408 / 5xx.** Current call site at
   `runtime_gateway_failover.go:731-737` only records account cooldown when
   `decision.Class == "transient" && RetryAfterMs > 0` (i.e. 429 with header) or
   raw 429. 5xx / 408 without `Retry-After` keep retrying the same throttled
   upstream until cross-candidate failover kicks in. We extend the recorder
   to also fire on `server_bad` (5xx) and on transient class without Retry-After,
   using the bounded default cooldown the rate-limit service already enforces.

3. **OpenAI rate-limit reset credit.** sub2api added a `POST /admin/.../reset`
   path that consumes one OpenAI `/wham/rate-limit-reset-credits/consume`
   ticket against a ChatGPT/Codex OAuth account, freeing a stuck 5h window
   mid-cycle. SRapi has no equivalent. Implement as a new admin action
   `POST /api/v1/admin/accounts/{id}/openai/reset-quota-window` wired through
   `provider_adapters.ResetOpenAIQuotaWindow(...)`. Surface in the admin
   account drawer; only render when account is OAuth + OpenAI/Codex.

4. **Thinking signature strip — gate by model family.** Today
   `claudeThinkingSanitizeRawPayload` strips invalid Claude signatures on every
   payload bound for a Claude-shaped endpoint. That breaks "passback required"
   models (DeepSeek, Kimi, Qwen-thinking, MiniMax) which require thinking blocks
   to round-trip byte-identically. Add a model-family classifier and skip the
   strip for passback-required families. Anthropic-strict family keeps the
   current behavior; unknown family stays conservative (no strip, no cache push).

5. **Images failover — per-account model remap.** Images-edits / images-variations
   route through the generic failover invoker with a frozen canonical request;
   when failover swaps accounts, the second account's per-credential model
   mapping is ignored. We re-resolve the upstream model from the chosen
   account's mapping at attempt-time, parallel to the existing chat-completions
   path. (Multipart bodies stay non-cached — only the JSON envelope picks up
   per-account remapping.)

6. **Scheduler request snapshot cleanup worker.** `scheduler_request_snapshots`
   ships under WP-1310 but has no TTL enforcement. sub2api uses a watermark +
   advisory-lock cleanup pattern. We add a `workers/scheduler_snapshot_cleanup`
   worker (bounded batch delete by `created_at < now - retention`), modelled
   on the existing `workers/retention` worker.

7. **Account temp-unschedulable.** sub2api lets operators say "skip this
   account for N minutes without disabling it"; expiry is automatic. SRapi
   currently only has health-probe-driven cooldown — no operator-initiated
   pause. We add `temp_unschedulable_until` + `temp_unschedulable_reason`
   on `ProviderAccount`, filter in the Scheduler hard-filter pass, expose
   `POST /api/v1/admin/accounts/{id}/temp-unschedule` and
   `DELETE /api/v1/admin/accounts/{id}/temp-unschedule`, and add a small
   action control in the account drawer.

8. **Account `expires_at` index for autopause sweep.** sub2api migration 151
   adds a partial index `WHERE deleted_at IS NULL AND schedulable=TRUE AND
   auto_pause_on_expired=TRUE AND expires_at IS NOT NULL`. SRapi's
   `token_expires_at` sweep currently scans the active account set. Add an
   equivalent partial index for the upcoming temp-unschedule sweep
   (`temp_unschedulable_until IS NOT NULL`).

9. **Codex CLI preset capability fix.** `codexCLICapabilities()` only declares
   `KeyImageGenerations`. The reverse-proxy adapter actually implements
   `invokeReverseProxyCodexImageEdit` and `invokeReverseProxyCodexImageVariation`.
   Add `KeyImageEdits` and `KeyImageVariations` so capability-aware Scheduler
   admission isn't artificially blocking traffic that the adapter can handle.

### OUT — rejected as architecture regression or wrong fit

A. **Anthropic API-key passthrough.** Sub2api lets an operator flag an account
   as "forward the inbound `x-api-key` straight upstream, skip scheduler /
   billing". This violates the SRapi `FINAL_STATE` §3 invariants
   ("Gateway handlers must not pick accounts directly", "Provider Adapter must
   not perform user auth or billing decisions"). If we ever want BYOK we should
   model it as a first-class account runtime class with the scheduler still
   selecting it — not as a bypass path.

B. **OpenAI "cyber" policy / session block / compliance ledger.** Region-specific
   regulatory tooling for CN Cyberspace Administration. SRapi positions as
   region-neutral self-hosted (FINAL_STATE §1). The legitimate generic
   capability (PII redaction + moderation thresholds + audit) already lives
   under `modules/content_safety/` and `modules/ops_error_logs/`. CN compliance
   should ship as a deployable add-on, not as core.

C. **Sub2api `scheduler_outbox` dedup table.** The dedup table is sub2api's
   way of suppressing redundant cache-invalidation events under crash retry.
   SRapi's invalidation path is the `domain_events_outbox` driven
   `GatewayAccountSnapshotRefreshRequested` handler, which is idempotent on
   the consumer side (snapshot refresh is naturally last-writer-wins). Adopting
   sub2api's separate scheduler-outbox table would duplicate infrastructure
   for a problem the SRapi shape doesn't have.

## Order of work

Each numbered group below is one commit + one push. Items are ordered smallest
blast radius first; bigger items get their own commit so review stays focused.

1. **Codex preset images capability fix** (item 9 — one-line registry change +
   regression test).
2. **Thinking signature strip — model-family gate** (item 4 — pure local change
   in `provider_adapters/service`).
3. **Transient-error cooldown** (item 2 — extend `recordGatewayAccountRateLimitCooldown`
   call sites; classifier already returns the right class).
4. **Account temp-unschedulable** (item 7 + item 8 — Ent schema field, store
   helper, scheduler hard-filter, admin API + handler + SDK, web action,
   partial index).
5. **Images failover per-account model remap** (item 5 — extend the failover
   invoker hook used by images edits / variations).
6. **OpenAI quota-window reset credit** (item 3 — provider-adapter call,
   admin endpoint, SDK, web button).
7. **Scheduler snapshot cleanup worker** (item 6 — new worker package +
   wiring + retention config).
8. **Antigravity reasoning replay** (item 1 — port cache module + wire into
   `antigravity.go` translator pre/post hooks).

Each commit runs the targeted module tests; `make check` runs at the end of
the wave.

## Non-goals

- No new architectural layers; everything reuses the existing
  `modules/<area>` ↔ `httpserver/runtime_*` ↔ `apps/web` channels.
- No deprecation churn for already-shipped sub2api/CLIProxyAPI parity items;
  STATUS.md already records those.
- No frontend rebuild — the admin web layer absorbs the new actions inside
  existing drawers/tables to keep information density high (per goal directive:
  no random new buttons).
