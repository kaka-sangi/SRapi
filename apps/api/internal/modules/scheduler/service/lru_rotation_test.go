package service_test

import (
	"context"
	"fmt"
	"testing"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func withLastUsed(ms int64) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.LastUsedUnixMs = ms }
}

// TestLRUPrefersLeastRecentlyUsedAmongEqualScores proves equal-scored accounts
// are tie-broken by least-recently-used (the older last-used wins).
func TestLRUPrefersLeastRecentlyUsedAmongEqualScores(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.9), withQuotaRemaining(0.9), withLastUsed(2000), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.9), withQuotaRemaining(0.9), withLastUsed(1000), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected least-recently-used account 2 selected, got %d", result.Candidate.Account.ID)
	}
}

// TestEqualCandidatesRotateAcrossRequests proves a cold pool (all last-used 0,
// identical scores) spreads load across requests instead of deterministically
// hammering one account — while staying deterministic for a given request id.
func TestEqualCandidatesRotateAcrossRequests(t *testing.T) {
	svc := newService(t)
	winners := map[int]int{}
	for i := 0; i < 40; i++ {
		req := baseRequest()
		req.RequestID = fmt.Sprintf("req_rotate_%d", i)
		req.Candidates = []contract.Candidate{
			candidate(1, withHealth(0.9), withQuotaRemaining(0.9), withCapabilities(capabilitiescontract.KeyStreaming)),
			candidate(2, withHealth(0.9), withQuotaRemaining(0.9), withCapabilities(capabilitiescontract.KeyStreaming)),
			candidate(3, withHealth(0.9), withQuotaRemaining(0.9), withCapabilities(capabilitiescontract.KeyStreaming)),
		}
		result, err := svc.Schedule(context.Background(), req)
		if err != nil {
			t.Fatalf("schedule %d: %v", i, err)
		}
		winners[result.Candidate.Account.ID]++
	}
	if len(winners) < 2 {
		t.Fatalf("expected load to spread across equal accounts, all went to %v", winners)
	}
	// Same request id is deterministic (replay-safe).
	req := baseRequest()
	req.RequestID = "req_fixed"
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.9), withQuotaRemaining(0.9), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.9), withQuotaRemaining(0.9), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	first, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule fixed: %v", err)
	}
	again, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule fixed again: %v", err)
	}
	if first.Candidate.Account.ID != again.Candidate.Account.ID {
		t.Fatalf("same request id must be deterministic, got %d then %d", first.Candidate.Account.ID, again.Candidate.Account.ID)
	}
}
