package service_test

import (
	"context"
	"testing"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func withCostWindow(used, limit float64) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.RuntimeState.CostWindowUsed = used
		l := limit
		candidate.Limits.CostWindowLimit = &l
	}
}

// TestCostWindowLimitRejectsOverspentAccount proves an account that has reached
// its rolling cost-window cap is skipped, and one under its cap is selected.
func TestCostWindowLimitRejectsOverspentAccount(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withCostWindow(10.0, 5.0), withHealth(0.99), withQuotaRemaining(0.99), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withCostWindow(1.0, 5.0), withHealth(0.50), withQuotaRemaining(0.50), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected under-cap account 2 selected, got %d", result.Candidate.Account.ID)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "cost_window_exceeded")
}

// TestCostWindowNoLimitIsNoRestriction proves the cap is opt-in: with no limit
// set, spend does not block selection.
func TestCostWindowNoLimitIsNoRestriction(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	c := candidate(1, withHealth(0.9), withQuotaRemaining(0.9), withCapabilities(capabilitiescontract.KeyStreaming))
	c.RuntimeState.CostWindowUsed = 9999 // huge spend, but no limit configured
	req.Candidates = []contract.Candidate{c}
	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected account 1 selected when no cost limit set, got %d", result.Candidate.Account.ID)
	}
}
