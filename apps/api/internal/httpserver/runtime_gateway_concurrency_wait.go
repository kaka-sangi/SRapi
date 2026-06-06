package httpserver

import (
	"context"
	"strconv"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

const (
	// gatewayConcurrencyWaitBudget bounds how long a request will wait for a
	// per-account concurrency slot to free before failing, when every eligible
	// account is at its MaxConcurrency cap. Mirrors sub2api's AccountWaitPlan: a
	// short wait smooths transient capacity spikes instead of an instant 429/503.
	gatewayConcurrencyWaitBudget = 3 * time.Second
	// gatewayConcurrencyWaitInterval is the poll interval while waiting; each
	// retry re-derives live concurrency, so a finished in-flight request frees a
	// slot for the next poll.
	gatewayConcurrencyWaitInterval = 150 * time.Millisecond
)

// gatewayMaxConcurrencyWaitReschedules caps how many times the wait loop will
// re-run scheduling (and thus persist a decision) so a busy pool with rapidly
// churning slots cannot cause unbounded decision writes.
const gatewayMaxConcurrencyWaitReschedules = 3

// scheduleGatewayRequestWaitingForSlot schedules a request and, when every
// eligible account is blocked by LIVE in-flight concurrency, briefly waits for a
// slot to free within gatewayConcurrencyWaitBudget before giving up. It polls the
// cheap per-account concurrency counter (no scheduler decision is persisted per
// poll) and only re-runs scheduling when a slot actually frees. Any other
// failure — including metadata-only saturation that can never free — returns
// immediately since a wait cannot clear it.
func (rt *runtimeState) scheduleGatewayRequestWaitingForSlot(ctx context.Context, req schedulercontract.ScheduleRequest, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey) (schedulercontract.ScheduleResult, error) {
	result, err := rt.scheduleGatewayRequest(ctx, req, modelID, forcedProviderKey, apiKey)
	if err == nil || !gatewayScheduleConcurrencySaturated(result, err) {
		return result, err
	}
	saturated := concurrencyFullAccountIDs(result.Decision.RejectReasons)
	baseline := make(map[int]int, len(saturated))
	waitable := false
	for _, id := range saturated {
		current := rt.scheduler.AccountConcurrency(ctx, id)
		baseline[id] = current
		if current > 0 {
			waitable = true // only live in-flight leases can free a slot
		}
	}
	if !waitable {
		return result, err
	}
	deadline := time.Now().Add(gatewayConcurrencyWaitBudget)
	for reschedules := 0; reschedules < gatewayMaxConcurrencyWaitReschedules && time.Now().Before(deadline); {
		if waitErr := sleepGatewayRetryDelay(ctx, gatewayConcurrencyWaitInterval); waitErr != nil {
			return result, err
		}
		freed := false
		for _, id := range saturated {
			if rt.scheduler.AccountConcurrency(ctx, id) < baseline[id] {
				freed = true
				break
			}
		}
		if !freed {
			continue
		}
		retried, retryErr := rt.scheduleGatewayRequest(ctx, req, modelID, forcedProviderKey, apiKey)
		reschedules++
		if retryErr == nil || !gatewayScheduleConcurrencySaturated(retried, retryErr) {
			return retried, retryErr
		}
		// The freed slot was taken by another request; refresh baselines and keep waiting.
		result, err = retried, retryErr
		for _, id := range saturated {
			baseline[id] = rt.scheduler.AccountConcurrency(ctx, id)
		}
	}
	return result, err
}

// gatewayScheduleConcurrencySaturated reports whether a failed schedule was
// caused (at least in part) by an account being at its concurrency cap — the
// only reject class a short wait can plausibly clear (a finishing in-flight
// request frees the slot).
func gatewayScheduleConcurrencySaturated(result schedulercontract.ScheduleResult, err error) bool {
	if err == nil {
		return false
	}
	for _, reason := range result.Decision.RejectReasons {
		if str, ok := reason.(string); ok && str == "concurrency_full" {
			return true
		}
	}
	return false
}

// concurrencyFullAccountIDs extracts the account IDs rejected for concurrency
// saturation from a decision's reject reasons (keyed "account_<id>").
func concurrencyFullAccountIDs(rejectReasons map[string]any) []int {
	var ids []int
	for key, reason := range rejectReasons {
		if str, ok := reason.(string); !ok || str != "concurrency_full" {
			continue
		}
		if !strings.HasPrefix(key, "account_") {
			continue
		}
		if id, convErr := strconv.Atoi(strings.TrimPrefix(key, "account_")); convErr == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}
