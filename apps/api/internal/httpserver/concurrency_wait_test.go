package httpserver

import (
	"errors"
	"testing"

	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func TestGatewayScheduleConcurrencySaturated(t *testing.T) {
	noErr := schedulercontract.ScheduleResult{}
	if gatewayScheduleConcurrencySaturated(noErr, nil) {
		t.Fatal("a successful schedule is never saturated")
	}

	satur := schedulercontract.ScheduleResult{Decision: schedulercontract.Decision{
		RejectReasons: map[string]any{"account_5": "concurrency_full", "account_6": "concurrency_full"},
	}}
	if !gatewayScheduleConcurrencySaturated(satur, errors.New("no available account")) {
		t.Fatal("expected concurrency_full reject reasons to count as saturation (waitable)")
	}

	// Non-waitable failures (auth/quota/etc.) must not trigger a wait.
	other := schedulercontract.ScheduleResult{Decision: schedulercontract.Decision{
		RejectReasons: map[string]any{"account_5": "needs_reauth", "account_6": "quota_exhausted"},
	}}
	if gatewayScheduleConcurrencySaturated(other, errors.New("no available account")) {
		t.Fatal("non-concurrency reject reasons must not be treated as waitable")
	}

	// Mixed: at least one account is just busy, so a wait may still help.
	mixed := schedulercontract.ScheduleResult{Decision: schedulercontract.Decision{
		RejectReasons: map[string]any{"account_5": "needs_reauth", "account_6": "concurrency_full"},
	}}
	if !gatewayScheduleConcurrencySaturated(mixed, errors.New("no available account")) {
		t.Fatal("a busy account among the rejects should still be waitable")
	}
}

func TestConcurrencyFullAccountIDs(t *testing.T) {
	ids := concurrencyFullAccountIDs(map[string]any{
		"account_5":  "concurrency_full",
		"account_6":  "needs_reauth",
		"account_42": "concurrency_full",
		"garbage":    "concurrency_full",
	})
	got := map[int]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 2 || !got[5] || !got[42] {
		t.Fatalf("expected [5 42], got %v", ids)
	}
}
