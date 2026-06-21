package httpserver

import (
	"context"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	concurrencyslotsservice "github.com/srapi/srapi/apps/api/internal/modules/concurrency_slots/service"
)

// runtime_gateway_slot_cooldown.go wires the ported concurrency_slots and
// rate_limit_cooldown modules into the gateway hot path. Both are in-process
// complements to the existing distributed Redis-backed gates (the failover
// loop already calls rateLimiter.AcquireConcurrency for the
// account/group/model rate-limit windows). They sit at the candidate-loop
// boundary: AFTER the scheduler picks a candidate but BEFORE the upstream
// HTTP call.
//
// Both modules are gated by per-account metadata flags so existing operators
// see no behaviour change until they opt in:
//   - `concurrency_slot_enabled` (bool)        — gate concurrency_slot acquisition.
//   - `rate_limit_cooldown_enabled` (bool)     — gate the in-process cooldown
//                                                 record on transient failures.
// Both default to false. The cooldown check (skipping accounts in cooldown)
// is always on — it's harmless when no account has ever been recorded.

// defaultConcurrencySlotCapacity is used when an account opted into the gate
// but did not set `max_concurrency` in its metadata. Matches sub2api's
// AccountWaitPlan default of 16 (its dispatchConcurrencyDefault when
// account-level field is unset).
const defaultConcurrencySlotCapacity = 16

// gatewayConcurrencySlotWaitBudget bounds how long AcquireSlot will wait
// in-call before surfacing as a transient gateway-side failure (caller can
// then failover to a different account). Tighter than
// gatewayConcurrencyWaitBudget because the existing wait-budget already
// covered the scheduler-side gate.
const gatewayConcurrencySlotWaitBudget = 1500 * time.Millisecond

// acquireGatewayAccountConcurrencySlot acquires an in-process concurrency
// slot for the picked account when its metadata opts in. Returns a release
// closure (callable any number of times) and a bool indicating whether a
// slot was actually acquired — callers use that to decide whether to defer
// the release. When the feature is disabled, returns a no-op closure.
//
// On wait-budget timeout or ctx cancellation the closure is nil and err is
// non-nil; callers should treat it as a transient failover hint.
func (rt *runtimeState) acquireGatewayAccountConcurrencySlot(ctx context.Context, account accountcontract.ProviderAccount) (release func(), acquired bool, err error) {
	if rt.concurrencySlots == nil {
		return func() {}, false, nil
	}
	if !accountConcurrencySlotEnabled(account.Metadata) {
		return func() {}, false, nil
	}
	capacity := accountConcurrencySlotCapacity(account.Metadata)
	release, err = rt.concurrencySlots.AcquireSlot(ctx, int64(account.ID), capacity, gatewayConcurrencySlotWaitBudget)
	if err != nil {
		return nil, false, err
	}
	return release, true, nil
}

// recordGatewayAccountRateLimitCooldown records an upstream 429 hit
// against the (accountID, model) pair so a 429 on one model never blocks
// a different model on the same credential. retryAfter ≤ 0 is treated as
// "use module default" (the module clamps to >= 1s anyway).
func (rt *runtimeState) recordGatewayAccountRateLimitCooldown(account accountcontract.ProviderAccount, model string, retryAfter time.Duration) {
	if rt.rateLimitCooldown == nil {
		return
	}
	if !accountRateLimitCooldownEnabled(account.Metadata) {
		return
	}
	if account.ID <= 0 {
		return
	}
	rt.rateLimitCooldown.RecordRateLimitHit(int64(account.ID), model, retryAfter)
}

// gatewayCooldownedAccountIDs returns every account currently in
// cooldown for the supplied model — caller appends them to
// ExcludedAccountIDs so the scheduler skips them on this attempt. The
// set is the union of (accountID, model) and account-wide entries: a
// 429 on any model still excludes its account, while a 429 on a
// sibling model never does.
func (rt *runtimeState) gatewayCooldownedAccountIDs(model string) []int {
	if rt.rateLimitCooldown == nil {
		return nil
	}
	ids64 := rt.rateLimitCooldown.CooldownedIDs(model)
	if len(ids64) == 0 {
		return nil
	}
	out := make([]int, 0, len(ids64))
	for _, id := range ids64 {
		if id <= 0 {
			continue
		}
		out = append(out, int(id))
	}
	return out
}

// accountConcurrencySlotEnabled reports whether the per-account metadata
// flag opts into the in-process concurrency-slot gate. Recognized synonyms
// mirror metacoerce conventions used elsewhere in the runtime.
func accountConcurrencySlotEnabled(metadata map[string]any) bool {
	for _, key := range []string{"concurrency_slot_enabled", "concurrency_slot.enabled"} {
		if metadataBool(metadata, key) {
			return true
		}
	}
	return false
}

func accountRateLimitCooldownEnabled(metadata map[string]any) bool {
	for _, key := range []string{"rate_limit_cooldown_enabled", "rate_limit_cooldown.enabled"} {
		if metadataBool(metadata, key) {
			return true
		}
	}
	return false
}

func accountConcurrencySlotCapacity(metadata map[string]any) int {
	for _, key := range []string{"concurrency_slot_capacity", "max_concurrency"} {
		if cap := metadataInt(metadata, key); cap > 0 {
			return cap
		}
	}
	return defaultConcurrencySlotCapacity
}

// errIsConcurrencySlotTransient reports whether the AcquireSlot error came
// from the wait-budget timer. Caller treats true as "try another candidate"
// rather than a hard gateway error.
func errIsConcurrencySlotTransient(err error) bool {
	if err == nil {
		return false
	}
	// Direct type match without unwrap to keep the dependency tight.
	if err == concurrencyslotsservice.ErrSlotAcquireTimeout {
		return true
	}
	// Defensive substring match in case the caller wraps.
	return strings.Contains(err.Error(), "acquire timeout")
}
