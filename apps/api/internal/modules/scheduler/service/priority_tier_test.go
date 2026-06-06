package service_test

import (
	"context"
	"testing"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func withPriority(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.Account.Priority = value }
}

// TestPriorityTierUsesHighestPriorityExclusively proves Account.Priority is a
// hard tier: a higher-priority (lower number) account is used even when a
// lower-priority account would score better, and the lower tier is deferred.
func TestPriorityTierUsesHighestPriorityExclusively(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withPriority(0), withHealth(0.50), withQuotaRemaining(0.50), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withPriority(100), withHealth(0.99), withQuotaRemaining(0.99), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected primary-tier account 1 selected, got %d", result.Candidate.Account.ID)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 2, "lower_priority_tier")
}

// TestPriorityTierFallsBackWhenTopTierUnavailable proves the next tier receives
// traffic once the primary tier has no eligible account.
func TestPriorityTierFallsBackWhenTopTierUnavailable(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withPriority(0), withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withPriority(100), withHealth(0.90), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected fallback-tier account 2 selected, got %d", result.Candidate.Account.ID)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "circuit_open")
}

// TestPriorityTierRetainsStickyAccountAcrossTiers proves an established session
// is not torn off its (lower-priority) account by the priority filter: the soft
// sticky account is retained and wins despite a higher-priority alternative.
func TestPriorityTierRetainsStickyAccountAcrossTiers(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	stickyAccountID := 2
	req.StickyAccountID = &stickyAccountID
	req.StickyStrength = contract.StickyStrengthSoft
	req.Candidates = []contract.Candidate{
		candidate(1, withPriority(0), withHealth(0.50), withQuotaRemaining(0.50), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withPriority(100), withHealth(0.99), withQuotaRemaining(0.99), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected retained sticky account 2 selected, got %d", result.Candidate.Account.ID)
	}
	if !result.Decision.StickyHit {
		t.Fatalf("expected sticky hit in decision, got %+v", result.Decision)
	}
}
