package service_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
)

func TestScheduleRejectsRuntimeLimitAndCapabilityFailures(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.RequestCapabilities = []capabilitiescontract.Descriptor{
		{Key: capabilitiescontract.KeyToolCalling, Level: capabilitiescontract.DescriptorLevelRequired, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
	}
	req.Candidates = []contract.Candidate{
		candidate(1, withMaxConcurrency(1), withConcurrency(1), withCapabilities(capabilitiescontract.KeyToolCalling)),
		candidate(2, withRPMLimit(10), withRPMUsed(10), withCapabilities(capabilitiescontract.KeyToolCalling)),
		candidate(3, withTPMLimit(100), withTPMUsed(100), withCapabilities(capabilitiescontract.KeyToolCalling)),
		candidate(4, noOptions(), noRuntime(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(5, noOptions(), noRuntime(), withCapabilities(capabilitiescontract.KeyToolCalling)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 5 {
		t.Fatalf("expected account 5 selected, got %d", result.Candidate.Account.ID)
	}
	if result.Lease.Status != contract.LeaseStatusPending || result.Lease.AccountID != 5 {
		t.Fatalf("expected pending lease for account 5, got %+v", result.Lease)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "concurrency_full")
	assertRejectReason(t, result.Decision.RejectReasons, 2, "rpm_limit_exceeded")
	assertRejectReason(t, result.Decision.RejectReasons, 3, "tpm_limit_exceeded")
	assertRejectReason(t, result.Decision.RejectReasons, 4, "capability_mismatch")
	if result.Decision.RejectedCount != 4 || result.Decision.CandidateCount != 5 {
		t.Fatalf("unexpected decision counts: %+v", result.Decision)
	}
}

func TestScheduleReturnsNoAvailableAccountWithStructuredReasons(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, noOptions(), withQuotaExhausted(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, noOptions(), withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(3, noOptions(), withCooldownActive(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(4, noOptions(), noRuntime(), withAccountStatus(accountcontract.StatusNeedsReauth), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(5, noOptions(), noRuntime(), withCredential(""), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err == nil {
		t.Fatal("expected no available account")
	}
	if err != service.ErrNoAvailableAccount {
		t.Fatalf("expected ErrNoAvailableAccount, got %v", err)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "quota_exhausted")
	assertRejectReason(t, result.Decision.RejectReasons, 2, "circuit_open")
	assertRejectReason(t, result.Decision.RejectReasons, 3, "cooldown_active")
	assertRejectReason(t, result.Decision.RejectReasons, 4, "needs_reauth")
	assertRejectReason(t, result.Decision.RejectReasons, 5, "credential_invalid")
	if result.Decision.SelectedAccountID != nil {
		t.Fatalf("expected no selected account, got %+v", result.Decision.SelectedAccountID)
	}
}

func TestBalancedStrategyPrefersHealthyCandidate(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withQuotaRemaining(0.50), withRelativeCost("0.5"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.60), withQuotaRemaining(0.90), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected healthier account 1 selected, got %d", result.Candidate.Account.ID)
	}
	if len(result.Decision.RejectReasons) != 0 {
		t.Fatalf("expected no hard filter rejects, got %+v", result.Decision.RejectReasons)
	}
	account1 := decisionScore(t, result.Decision.Scores, 1)
	account2 := decisionScore(t, result.Decision.Scores, 2)
	if account1["health_score"].(float64) <= account2["health_score"].(float64) {
		t.Fatalf("expected account 1 health score to dominate, got a1=%+v a2=%+v", account1, account2)
	}
}

func TestScheduleReturnsRankedAvailableCandidates(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.40), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(3, withHealth(0.95), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(4, withHealth(0.75), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 3 {
		t.Fatalf("expected selected candidate 3, got %+v", result.Candidate.Account)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("expected three ranked candidates after filtering, got %+v", result.Candidates)
	}
	got := []int{result.Candidates[0].Account.ID, result.Candidates[1].Account.ID, result.Candidates[2].Account.ID}
	want := []int{3, 4, 1}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("expected ranked candidate order %v, got %v", want, got)
		}
	}
	assertRejectReason(t, result.Decision.RejectReasons, 2, "circuit_open")
}

func TestParetoFrontierExcludesDominatedCandidateBeforeWeightedSelection(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.70), withQuotaRemaining(0.50), withLatencyP95MS(2000), withRelativeCost("0.1"), withQualityScore("0.90"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.70), withQuotaRemaining(0.50), withLatencyP95MS(1000), withRelativeCost("0.9"), withQualityScore("0.60"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(3, withHealth(1.00), withQuotaRemaining(1.00), withLatencyP95MS(2000), withRelativeCost("0.2"), withQualityScore("0.40"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected Pareto frontier candidate 1 selected, got %d", result.Candidate.Account.ID)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("expected all available candidates returned by rank, got %+v", result.Candidates)
	}
	gotRank := []int{result.Candidates[0].Account.ID, result.Candidates[1].Account.ID, result.Candidates[2].Account.ID}
	wantRank := []int{1, 2, 3}
	for idx := range wantRank {
		if gotRank[idx] != wantRank[idx] {
			t.Fatalf("expected Pareto frontier candidates before dominated candidate %v, got %v", wantRank, gotRank)
		}
	}
	frontierIDs := paretoFrontierIDs(t, result.Decision.Scores)
	if len(frontierIDs) != 2 || frontierIDs[0] != 1 || frontierIDs[1] != 2 {
		t.Fatalf("expected accounts 1 and 2 in Pareto frontier, got %v", frontierIDs)
	}
	account1 := decisionScore(t, result.Decision.Scores, 1)
	account3 := decisionScore(t, result.Decision.Scores, 3)
	if account3["final_score"].(float64) <= account1["final_score"].(float64) {
		t.Fatalf("test setup expected dominated account 3 to have higher weighted score than selected account 1: a1=%+v a3=%+v", account1, account3)
	}
}

func TestFreeTierRejectsProtectedLowQuotaAccount(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.UserTier = contract.UserTierFree
	req.Strategy = contract.StrategyCostSaver
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withQuotaRemaining(0.08), withProtectedAccount(), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.90), withQuotaRemaining(0.70), withRelativeCost("0.2"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected normal quota account 2 selected, got %d", result.Candidate.Account.ID)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "quota_protected")
}

func TestSoftStickyInfluencesCloseCandidates(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	stickyAccountID := 1
	req.StickyAccountID = &stickyAccountID
	req.StickyStrength = contract.StickyStrengthSoft
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.90), withQuotaRemaining(0.60), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.92), withQuotaRemaining(0.60), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected soft sticky account 1 selected, got %d", result.Candidate.Account.ID)
	}
	if !result.Decision.StickyHit {
		t.Fatalf("expected sticky hit in decision, got %+v", result.Decision)
	}
	if sticky := decisionScore(t, result.Decision.Scores, 1)["sticky_score"].(float64); sticky <= 0 {
		t.Fatalf("expected sticky score, got %+v", result.Decision.Scores)
	}
}

func TestSoftStickyDoesNotBypassHardFilters(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	stickyAccountID := 1
	req.StickyAccountID = &stickyAccountID
	req.StickyStrength = contract.StickyStrengthSoft
	req.Candidates = []contract.Candidate{
		candidate(1, withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.88), withQuotaRemaining(0.70), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected fallback account 2 selected, got %d", result.Candidate.Account.ID)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "circuit_open")
	if result.Decision.RejectReasons["sticky_broken_reason"] != "circuit_open" {
		t.Fatalf("expected sticky broken reason, got %+v", result.Decision.RejectReasons)
	}
}

func TestHardStickyOnlyAllowsBoundAccount(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	stickyAccountID := 2
	req.StickyAccountID = &stickyAccountID
	req.StickyStrength = contract.StickyStrengthHard
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.99), withQuotaRemaining(1), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.20), withQuotaRemaining(0.30), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected hard sticky account 2 selected, got %d", result.Candidate.Account.ID)
	}
	if !result.Decision.StickyHit {
		t.Fatalf("expected hard sticky hit, got %+v", result.Decision)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "hard_sticky_mismatch")
}

func TestRoutingHintsAreRecordedWithoutLeakingAffinityKey(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	stickyAccountID := 1
	req.ModelAlias = "claude-sonnet"
	req.FallbackModels = []string{"claude-haiku"}
	req.SessionAffinityKey = "conversation-secret"
	req.SessionAffinitySource = "header:x-srapi-session-affinity-key"
	req.StickyAccountID = &stickyAccountID
	req.StickyStrength = contract.StickyStrengthSoft
	req.Candidates = []contract.Candidate{
		candidate(1, withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	hints, ok := result.Decision.Scores["routing_hints"].(map[string]any)
	if !ok {
		t.Fatalf("expected routing hints in decision scores, got %+v", result.Decision.Scores)
	}
	if hints["model_alias"] != "claude-sonnet" || hints["sticky_strength"] != "soft" || hints["sticky_account_id"].(float64) != 1 {
		t.Fatalf("unexpected routing hints: %+v", hints)
	}
	if hints["session_affinity_key_hash"] == "conversation-secret" || !strings.HasPrefix(hints["session_affinity_key_hash"].(string), "sha256:") {
		t.Fatalf("expected hashed affinity key, got %+v", hints)
	}
	fallbacks, ok := hints["fallback_models"].([]any)
	if !ok || len(fallbacks) != 1 || fallbacks[0] != "claude-haiku" {
		t.Fatalf("expected fallback model hint, got %+v", hints["fallback_models"])
	}
}

func TestSchedulePersistsSanitizedRequestSnapshotForReplay(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.SessionAffinityKey = "conversation-secret"
	req.SessionAffinitySource = "header:x-srapi-session-affinity-key"
	req.EstimatedInputTokens = 1000
	req.EstimatedOutputTokens = 200
	req.Candidates = []contract.Candidate{
		candidate(1, withCapabilities(capabilitiescontract.KeyStreaming), withAccountMetadata(map[string]any{
			"quality_score":          "0.93",
			"access_token":           "leak",
			"cached_token_estimate":  125,
			"nested":                 map[string]any{"cookie": "leak", "cache_score": 0.7},
			"device_fingerprint_key": "not-sensitive",
		}), withProviderConfig(map[string]any{
			"base_url": "https://provider.example",
			"api_key":  "leak",
		})),
		candidate(2, withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	snapshots, err := svc.ListRequestSnapshots(context.Background())
	if err != nil {
		t.Fatalf("list request snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected one request snapshot, got %+v", snapshots)
	}
	snapshot := snapshots[0]
	if snapshot.DecisionID != result.Decision.ID || snapshot.RequestID != result.Decision.RequestID || snapshot.AttemptNo != result.Decision.AttemptNo {
		t.Fatalf("expected snapshot to reference persisted decision, snapshot=%+v decision=%+v", snapshot, result.Decision)
	}
	if snapshot.RequestProfile["session_affinity_key"] != nil || snapshot.RequestProfile["session_affinity_key_hash"] == "conversation-secret" {
		t.Fatalf("expected hashed affinity key only, got %+v", snapshot.RequestProfile)
	}
	hash, ok := snapshot.RequestProfile["session_affinity_key_hash"].(string)
	if !ok || !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("expected hashed affinity key, got %+v", snapshot.RequestProfile)
	}
	if len(snapshot.CandidateSnapshot) != 2 {
		t.Fatalf("expected all request candidates in snapshot, got %+v", snapshot.CandidateSnapshot)
	}
	first := snapshot.CandidateSnapshot[0]
	if first.AccountMetadata["access_token"] != nil {
		t.Fatalf("expected access token stripped from account metadata, got %+v", first.AccountMetadata)
	}
	if first.AccountMetadata["quality_score"] != "0.93" || first.AccountMetadata["cached_token_estimate"].(float64) != 125 {
		t.Fatalf("expected scoring metadata retained, got %+v", first.AccountMetadata)
	}
	nested, ok := first.AccountMetadata["nested"].(map[string]any)
	if !ok || nested["cookie"] != nil || nested["cache_score"].(float64) != 0.7 {
		t.Fatalf("expected nested sensitive metadata stripped only, got %+v", first.AccountMetadata)
	}
	if first.ProviderConfig["api_key"] != nil || first.ProviderConfig["base_url"] != "https://provider.example" {
		t.Fatalf("expected provider config sanitized, got %+v", first.ProviderConfig)
	}
	if got := snapshot.RankedAccountIDs; len(got) != 1 || got[0] != result.Candidate.Account.ID {
		t.Fatalf("expected ranked available account IDs, got %+v", got)
	}
	if first.AccountHasCredential == nil || !*first.AccountHasCredential {
		t.Fatalf("expected credential presence marker without credential value, got %+v", first)
	}
}

func TestReplayStrategiesReplaysRequestSnapshotsWithoutSideEffects(t *testing.T) {
	store := schedulermemory.New()
	svc, err := service.New(store, nil)
	if err != nil {
		t.Fatalf("create scheduler service: %v", err)
	}
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.60), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected balanced request to select account 1, got %+v", result.Candidate)
	}

	percent := 100.0
	replay, err := svc.ReplayStrategies(context.Background(), contract.StrategyReplayRequest{
		ShadowStrategy:       contract.StrategyCostSaver,
		ShadowRolloutPercent: &percent,
		Limit:                10,
	})
	if err != nil {
		t.Fatalf("replay strategies: %v", err)
	}
	if !replay.DryRun || replay.Requested != 1 || replay.Replayed != 1 || replay.Skipped != 0 {
		t.Fatalf("unexpected replay summary: %+v", replay)
	}
	if replay.WinnerChanged != 1 || replay.CurrentWinCounts["1"] != 1 || replay.ShadowWinCounts["2"] != 1 {
		t.Fatalf("expected winner change summary, got %+v", replay)
	}
	item := replay.Items[0]
	if item.SnapshotID == 0 || item.DecisionID != result.Decision.ID || item.RequestID != req.RequestID {
		t.Fatalf("expected replay item to reference persisted snapshot, got %+v", item)
	}
	if item.Current.Decision.SelectedAccountID == nil || *item.Current.Decision.SelectedAccountID != 1 {
		t.Fatalf("expected current strategy to replay account 1, got %+v", item.Current)
	}
	if item.Shadow.Decision.SelectedAccountID == nil || *item.Shadow.Decision.SelectedAccountID != 2 {
		t.Fatalf("expected shadow strategy to replay account 2, got %+v", item.Shadow)
	}
	if !item.Rollout.Enabled || !item.Rollout.ShadowSelected || item.Rollout.KeyHash == "" || item.Rollout.KeyHash == req.RequestID {
		t.Fatalf("expected hashed rollout preview, got %+v", item.Rollout)
	}

	decisions, err := store.ListDecisions(context.Background())
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected replay not to create decisions, got %+v", decisions)
	}
	leases, err := store.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("expected replay not to acquire leases, got %+v", leases)
	}
}

func TestReplayStrategiesPreservesCredentialPresenceSemantics(t *testing.T) {
	store := schedulermemory.New()
	svc, err := service.New(store, nil)
	if err != nil {
		t.Fatalf("create scheduler service: %v", err)
	}
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withCredential(""), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	if _, err := svc.Schedule(context.Background(), req); !errors.Is(err, service.ErrNoAvailableAccount) {
		t.Fatalf("expected schedule to persist failed no-account decision, got %v", err)
	}

	replay, err := svc.ReplayStrategies(context.Background(), contract.StrategyReplayRequest{
		ShadowStrategy: contract.StrategyCostSaver,
	})
	if err != nil {
		t.Fatalf("replay strategies: %v", err)
	}
	if replay.Replayed != 1 {
		t.Fatalf("expected replayed failed snapshot, got %+v", replay)
	}
	item := replay.Items[0]
	if item.Current.Error != "no_available_account" || item.Shadow.Error != "no_available_account" {
		t.Fatalf("expected replay to preserve credential rejection, got current=%+v shadow=%+v", item.Current, item.Shadow)
	}
	assertRejectReason(t, item.Current.Decision.RejectReasons, 1, "credential_invalid")
	assertRejectReason(t, item.Shadow.Decision.RejectReasons, 1, "credential_invalid")
}

func TestCostSaverPrefersLowerRelativeCost(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Strategy = contract.StrategyCostSaver
	req.Candidates = []contract.Candidate{
		candidate(1, noOptions(), noRuntime(), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, noOptions(), noRuntime(), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected lower-cost account 2 selected, got %d", result.Candidate.Account.ID)
	}
	if result.Decision.Strategy != contract.StrategyCostSaver || result.Decision.StrategyVersion == "" || len(result.Decision.StrategyWeights) == 0 {
		t.Fatalf("expected strategy snapshot in decision, got %+v", result.Decision)
	}
	if !strings.HasPrefix(result.Decision.StrategyConfigHash, "sha256:") {
		t.Fatalf("expected strategy config hash snapshot, got %+v", result.Decision)
	}
}

func TestLatencyFirstPrefersLowerP95Latency(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Strategy = contract.StrategyLatencyFirst
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withQuotaRemaining(0.90), withLatencyP95MS(8000), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.85), withQuotaRemaining(0.90), withLatencyP95MS(1000), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected lower-latency account 2 selected, got %d", result.Candidate.Account.ID)
	}
	if result.Decision.Strategy != contract.StrategyLatencyFirst {
		t.Fatalf("expected latency_first strategy snapshot, got %+v", result.Decision)
	}
}

func TestQuotaProtectPrefersMoreRemainingQuota(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Strategy = contract.StrategyQuotaProtect
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withQuotaRemaining(0.10), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.85), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected quota-rich account 2 selected, got %d", result.Candidate.Account.ID)
	}
}

func TestStickyFirstPrioritizesStickyCandidate(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	stickyAccountID := 1
	req.Strategy = contract.StrategyStickyFirst
	req.StickyAccountID = &stickyAccountID
	req.StickyStrength = contract.StickyStrengthSoft
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.80), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.92), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected sticky account 1 selected, got %d", result.Candidate.Account.ID)
	}
	if !result.Decision.StickyHit {
		t.Fatalf("expected sticky hit in decision, got %+v", result.Decision)
	}
}

func TestPremiumQualityPrefersHealthOverCost(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Strategy = contract.StrategyPremiumQuality
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withQuotaRemaining(0.90), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.60), withQuotaRemaining(0.90), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected premium quality to select healthier account 1, got %d", result.Candidate.Account.ID)
	}
}

func TestCacheAffinityFirstPrefersHealthyCachedCandidate(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Strategy = contract.StrategyCacheAffinityFirst
	req.EstimatedInputTokens = 80000
	req.EstimatedOutputTokens = 1000
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.88), withQuotaRemaining(0.90), withCacheScore("0.90"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.95), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 1 {
		t.Fatalf("expected cache-affinity account 1 selected, got %d", result.Candidate.Account.ID)
	}
	if !result.Decision.CacheAffinityHit {
		t.Fatalf("expected cache affinity hit in decision, got %+v", result.Decision)
	}
	if cache := decisionScore(t, result.Decision.Scores, 1)["cache_score"].(float64); cache <= 0 {
		t.Fatalf("expected positive cache score, got %+v", result.Decision.Scores)
	}
}

func TestCacheAffinityDoesNotOverridePoorHealth(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Strategy = contract.StrategyCacheAffinityFirst
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.30), withQuotaRemaining(0.90), withCacheScore("1.00"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.95), withQuotaRemaining(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected healthy account 2 selected, got %d", result.Candidate.Account.ID)
	}
	if result.Decision.CacheAffinityHit {
		t.Fatalf("did not expect cache affinity hit for selected uncached account, got %+v", result.Decision)
	}
	if cache := decisionScore(t, result.Decision.Scores, 1)["cache_score"].(float64); cache != 0 {
		t.Fatalf("expected poor-health cache score to be suppressed, got %+v", result.Decision.Scores)
	}
}

func TestSchedulingScenarioMatrixMVP(t *testing.T) {
	cases := []struct {
		name          string
		req           contract.ScheduleRequest
		wantAccountID int
		wantErr       error
		wantRejects   map[int]string
		wantDecision  map[string]any
		wantLease     bool
		assertScores  func(t *testing.T, decision contract.Decision)
	}{
		{
			name: "A health priority",
			req: withCandidates(baseRequest(),
				candidate(1, withHealth(0.95), withQuotaRemaining(0.50), withRelativeCost("0.5"), withCapabilities(capabilitiescontract.KeyStreaming)),
				candidate(2, withHealth(0.60), withQuotaRemaining(0.90), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
			),
			wantAccountID: 1,
			wantLease:     true,
			assertScores: func(t *testing.T, decision contract.Decision) {
				t.Helper()
				if decisionScore(t, decision.Scores, 1)["health_score"].(float64) <= decisionScore(t, decision.Scores, 2)["health_score"].(float64) {
					t.Fatalf("scenario A expected account 1 health score to dominate: %+v", decision.Scores)
				}
			},
		},
		{
			name: "B quota protection",
			req: func() contract.ScheduleRequest {
				req := baseRequest()
				req.UserTier = contract.UserTierFree
				req.Strategy = contract.StrategyCostSaver
				return withCandidates(req,
					candidate(1, withHealth(0.95), withQuotaRemaining(0.08), withProtectedAccount(), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
					candidate(2, withHealth(0.90), withQuotaRemaining(0.70), withRelativeCost("0.2"), withCapabilities(capabilitiescontract.KeyStreaming)),
				)
			}(),
			wantAccountID: 2,
			wantRejects:   map[int]string{1: "quota_protected"},
			wantLease:     true,
		},
		{
			name: "D soft sticky hit",
			req: func() contract.ScheduleRequest {
				stickyAccountID := 1
				req := baseRequest()
				req.StickyAccountID = &stickyAccountID
				req.StickyStrength = contract.StickyStrengthSoft
				return withCandidates(req,
					candidate(1, withHealth(0.90), withQuotaRemaining(0.60), withCapabilities(capabilitiescontract.KeyStreaming)),
					candidate(2, withHealth(0.92), withQuotaRemaining(0.60), withCapabilities(capabilitiescontract.KeyStreaming)),
				)
			}(),
			wantAccountID: 1,
			wantDecision:  map[string]any{"sticky_hit": true},
			wantLease:     true,
		},
		{
			name: "E sticky failure fallback",
			req: func() contract.ScheduleRequest {
				stickyAccountID := 1
				req := baseRequest()
				req.StickyAccountID = &stickyAccountID
				req.StickyStrength = contract.StickyStrengthSoft
				return withCandidates(req,
					candidate(1, withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
					candidate(2, withHealth(0.88), withQuotaRemaining(0.70), withCapabilities(capabilitiescontract.KeyStreaming)),
				)
			}(),
			wantAccountID: 2,
			wantRejects:   map[int]string{1: "circuit_open"},
			wantDecision:  map[string]any{"sticky_broken_reason": "circuit_open"},
			wantLease:     true,
		},
		{
			name: "J cost first",
			req: func() contract.ScheduleRequest {
				req := baseRequest()
				req.Strategy = contract.StrategyCostSaver
				return withCandidates(req,
					candidate(1, withHealth(0.95), withQuotaRemaining(0.90), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
					candidate(2, withHealth(0.85), withQuotaRemaining(0.90), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
				)
			}(),
			wantAccountID: 2,
			wantLease:     true,
			assertScores: func(t *testing.T, decision contract.Decision) {
				t.Helper()
				if decisionScore(t, decision.Scores, 2)["cost_score"].(float64) <= decisionScore(t, decision.Scores, 1)["cost_score"].(float64) {
					t.Fatalf("scenario J expected account 2 cost score to dominate: %+v", decision.Scores)
				}
			},
		},
		{
			name: "L lease concurrency limit",
			req: withCandidates(baseRequest(),
				candidate(1, withMaxConcurrency(10), withConcurrency(10), withCapabilities(capabilitiescontract.KeyStreaming)),
				candidate(2, withMaxConcurrency(10), withConcurrency(5), withCapabilities(capabilitiescontract.KeyStreaming)),
			),
			wantAccountID: 2,
			wantRejects:   map[int]string{1: "concurrency_full"},
			wantLease:     true,
		},
		{
			name: "M RPM limit",
			req: withCandidates(baseRequest(),
				candidate(1, withRPMLimit(10), withRPMUsed(10), withCapabilities(capabilitiescontract.KeyStreaming)),
				candidate(2, withRPMLimit(10), withRPMUsed(5), withCapabilities(capabilitiescontract.KeyStreaming)),
			),
			wantAccountID: 2,
			wantRejects:   map[int]string{1: "rpm_limit_exceeded"},
			wantLease:     true,
		},
		{
			name: "N no available account",
			req: withCandidates(baseRequest(),
				candidate(1, withAccountStatus(accountcontract.StatusDisabled), withCapabilities(capabilitiescontract.KeyStreaming)),
				candidate(2, withQuotaExhausted(), withCapabilities(capabilitiescontract.KeyStreaming)),
				candidate(3, withCircuitOpen(), withCapabilities(capabilitiescontract.KeyStreaming)),
			),
			wantErr:     service.ErrNoAvailableAccount,
			wantRejects: map[int]string{1: "account_disabled", 2: "quota_exhausted", 3: "circuit_open"},
		},
		{
			name: "Q user balance insufficient",
			req: func() contract.ScheduleRequest {
				req := baseRequest()
				req.UserBalanceInsufficient = true
				return withCandidates(req,
					candidate(1, withCapabilities(capabilitiescontract.KeyStreaming)),
					candidate(2, withCapabilities(capabilitiescontract.KeyStreaming)),
				)
			}(),
			wantErr:     service.ErrUserBalanceInsufficient,
			wantRejects: map[int]string{1: "user_balance_insufficient", 2: "user_balance_insufficient"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService(t)
			result, err := svc.Schedule(context.Background(), tc.req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got result=%+v err=%v", tc.wantErr, result, err)
				}
				if result.Decision.SelectedAccountID != nil {
					t.Fatalf("expected no selected account, got %+v", result.Decision)
				}
			} else if err != nil {
				t.Fatalf("schedule: %v", err)
			}
			if tc.wantAccountID > 0 && result.Candidate.Account.ID != tc.wantAccountID {
				t.Fatalf("expected account %d selected, got %+v", tc.wantAccountID, result.Candidate.Account)
			}
			for accountID, reason := range tc.wantRejects {
				assertRejectReason(t, result.Decision.RejectReasons, accountID, reason)
			}
			for key, value := range tc.wantDecision {
				switch key {
				case "sticky_hit":
					if result.Decision.StickyHit != value.(bool) {
						t.Fatalf("expected sticky_hit=%v, got %+v", value, result.Decision)
					}
				default:
					if result.Decision.RejectReasons[key] != value {
						t.Fatalf("expected decision marker %s=%v, got %+v", key, value, result.Decision.RejectReasons)
					}
				}
			}
			if tc.wantLease && (result.Lease.Status != contract.LeaseStatusPending || result.Lease.AccountID != tc.wantAccountID) {
				t.Fatalf("expected pending lease on account %d, got %+v", tc.wantAccountID, result.Lease)
			}
			if !tc.wantLease && result.Lease.ID != "" {
				t.Fatalf("expected no lease, got %+v", result.Lease)
			}
			if tc.assertScores != nil {
				tc.assertScores(t, result.Decision)
			}
		})
	}
}

func TestUserBalanceInsufficientCreatesDecisionWithoutLease(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.UserBalanceInsufficient = true
	req.Candidates = []contract.Candidate{
		candidate(1, withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.Schedule(context.Background(), req)
	if !errors.Is(err, service.ErrUserBalanceInsufficient) {
		t.Fatalf("expected ErrUserBalanceInsufficient, got result=%+v err=%v", result, err)
	}
	if result.Decision.SelectedAccountID != nil {
		t.Fatalf("expected no selected account, got %+v", result.Decision)
	}
	assertRejectReason(t, result.Decision.RejectReasons, 1, "user_balance_insufficient")
	assertRejectReason(t, result.Decision.RejectReasons, 2, "user_balance_insufficient")
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected no lease for user-side balance rejection, got %+v", leases)
	}
}

func TestStrategyRegistryListsSeededStrategies(t *testing.T) {
	svc := newService(t)
	strategies, err := svc.ListStrategies(context.Background())
	if err != nil {
		t.Fatalf("list strategies: %v", err)
	}
	if len(strategies) != 7 {
		t.Fatalf("expected 7 seeded strategies, got %d", len(strategies))
	}
	seen := map[contract.StrategyName]bool{}
	for _, strategy := range strategies {
		if strategy.Version == "" || strategy.Status != "active" || !strings.HasPrefix(strategy.ConfigHash, "sha256:") || len(strategy.Weights) == 0 || len(strategy.Config) == 0 {
			t.Fatalf("unexpected strategy descriptor: %+v", strategy)
		}
		seen[strategy.Name] = true
	}
	for _, name := range []contract.StrategyName{
		contract.StrategyBalanced,
		contract.StrategyCostSaver,
		contract.StrategyLatencyFirst,
		contract.StrategyQuotaProtect,
		contract.StrategyStickyFirst,
		contract.StrategyCacheAffinityFirst,
		contract.StrategyPremiumQuality,
	} {
		if !seen[name] {
			t.Fatalf("expected seeded strategy %s in %+v", name, strategies)
		}
	}
}

func TestServiceRefreshesActiveStrategyBeforeSchedule(t *testing.T) {
	store := &dynamicStrategyStore{Store: schedulermemory.New()}
	svc, err := service.New(store, nil)
	if err != nil {
		t.Fatalf("create scheduler service: %v", err)
	}

	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.60), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	first, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule with seeded strategy: %v", err)
	}
	if first.Candidate.Account.ID != 1 {
		t.Fatalf("expected seeded balanced strategy to select healthier account 1, got %d", first.Candidate.Account.ID)
	}

	store.strategies = []contract.StrategyDescriptor{
		{
			ID:      42,
			Name:    contract.StrategyBalanced,
			Version: "v2",
			Status:  "active",
			Config: map[string]any{
				"weights": map[string]any{
					"cost_weight": 1.0,
				},
			},
			Description: "DB-loaded balanced override",
		},
	}
	req.RequestID = "req_scheduler_strategy_refresh"
	second, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule with DB-loaded strategy: %v", err)
	}
	if second.Candidate.Account.ID != 2 {
		t.Fatalf("expected DB-loaded balanced strategy to select lower-cost account 2, got %d", second.Candidate.Account.ID)
	}
	if second.Decision.StrategyVersion != "v2" || !strings.HasPrefix(second.Decision.StrategyConfigHash, "sha256:") {
		t.Fatalf("expected DB-loaded strategy snapshot, got %+v", second.Decision)
	}
	if second.Decision.StrategyWeights["cost"] != 1.0 {
		t.Fatalf("expected cost-only strategy weights, got %+v", second.Decision.StrategyWeights)
	}
}

func TestSimulateStrategyDoesNotPersistDecisionOrLease(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.60), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.SimulateStrategy(context.Background(), contract.StrategySimulationRequest{
		Request:         req,
		CurrentStrategy: contract.StrategyBalanced,
		ShadowStrategy:  contract.StrategyCostSaver,
	})
	if err != nil {
		t.Fatalf("simulate strategy: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("expected dry-run result, got %+v", result)
	}
	if selectedAccountID(result.Current.Decision) != 1 || selectedAccountID(result.Shadow.Decision) != 2 {
		t.Fatalf("expected balanced account 1 and cost_saver account 2, got current=%+v shadow=%+v", result.Current.Decision, result.Shadow.Decision)
	}
	if !result.Diff.WinnerChanged || selectedAccountIDPtr(result.Diff.CurrentSelectedAccountID) != 1 || selectedAccountIDPtr(result.Diff.ShadowSelectedAccountID) != 2 {
		t.Fatalf("expected winner diff, got %+v", result.Diff)
	}
	if result.Diff.CostScoreDelta <= 0 {
		t.Fatalf("expected shadow winner to improve cost score, got %+v", result.Diff)
	}
	decisions, err := svc.ListDecisions(context.Background())
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected simulation to avoid persisted decisions, got %+v", decisions)
	}
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected simulation to avoid leases, got %+v", leases)
	}
	snapshots, err := svc.ListRequestSnapshots(context.Background())
	if err != nil {
		t.Fatalf("list request snapshots: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected simulation to avoid request snapshots, got %+v", snapshots)
	}
}

func TestSimulateStrategyReportsRejectedShadowWithoutLease(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withAccountStatus(accountcontract.StatusDisabled), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	result, err := svc.SimulateStrategy(context.Background(), contract.StrategySimulationRequest{
		Request:        req,
		ShadowStrategy: contract.StrategyCostSaver,
	})
	if err != nil {
		t.Fatalf("simulate strategy: %v", err)
	}
	if result.Current.Error != "no_available_account" || result.Shadow.Error != "no_available_account" {
		t.Fatalf("expected no available account errors, got %+v", result)
	}
	assertRejectReason(t, result.Shadow.Decision.RejectReasons, 1, "account_disabled")
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected no lease from rejected simulation, got %+v", leases)
	}
}

func TestSimulateStrategyReturnsStableRolloutPreview(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withHealth(0.95), withRelativeCost("0.9"), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withHealth(0.60), withRelativeCost("0.1"), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	percent := 100.0

	first, err := svc.SimulateStrategy(context.Background(), contract.StrategySimulationRequest{
		Request:              req,
		ShadowStrategy:       contract.StrategyCostSaver,
		ShadowRolloutPercent: &percent,
		RolloutKey:           "api-key:1:model:sim-model",
	})
	if err != nil {
		t.Fatalf("simulate strategy with rollout: %v", err)
	}
	second, err := svc.SimulateStrategy(context.Background(), contract.StrategySimulationRequest{
		Request:              req,
		ShadowStrategy:       contract.StrategyCostSaver,
		ShadowRolloutPercent: &percent,
		RolloutKey:           "api-key:1:model:sim-model",
	})
	if err != nil {
		t.Fatalf("repeat simulate strategy with rollout: %v", err)
	}
	if !first.Rollout.Enabled || !first.Rollout.ShadowSelected || first.Rollout.Percent != 100 {
		t.Fatalf("expected enabled 100 percent rollout to select shadow, got %+v", first.Rollout)
	}
	if first.Rollout.Bucket != second.Rollout.Bucket || first.Rollout.KeyHash != second.Rollout.KeyHash {
		t.Fatalf("expected stable rollout bucket/hash, first=%+v second=%+v", first.Rollout, second.Rollout)
	}
	if first.Rollout.KeyHash == "" || first.Rollout.KeyHash == "api-key:1:model:sim-model" || !strings.HasPrefix(first.Rollout.KeyHash, "sha256:") {
		t.Fatalf("expected hashed rollout key only, got %+v", first.Rollout)
	}
}

func TestSimulateStrategyRejectsInvalidRolloutPercent(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	percent := 101.0

	_, err := svc.SimulateStrategy(context.Background(), contract.StrategySimulationRequest{
		Request:              req,
		ShadowStrategy:       contract.StrategyCostSaver,
		ShadowRolloutPercent: &percent,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("expected invalid input for rollout percent, got %v", err)
	}
}

func TestLeasePreventsConcurrentSchedulingAndFeedbackReleases(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withMaxConcurrency(1), withCapabilities(capabilitiescontract.KeyStreaming)),
	}

	first, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("first schedule: %v", err)
	}
	if first.Lease.Status != contract.LeaseStatusPending {
		t.Fatalf("expected pending lease, got %+v", first.Lease)
	}

	secondReq := req
	secondReq.RequestID = "req_scheduler_2"
	second, err := svc.Schedule(context.Background(), secondReq)
	if err != service.ErrNoAvailableAccount {
		t.Fatalf("expected no available account from lease saturation, got result=%+v err=%v", second, err)
	}
	assertRejectReason(t, second.Decision.RejectReasons, 1, "concurrency_full")

	_, err = svc.RecordFeedback(context.Background(), contract.RecordFeedbackRequest{
		RequestID:  first.Decision.RequestID,
		DecisionID: first.Decision.ID,
		AttemptNo:  first.Decision.AttemptNo,
		AccountID:  first.Candidate.Account.ID,
		ProviderID: first.Candidate.Provider.ID,
		Model:      first.Decision.Model,
		Success:    true,
	})
	if err != nil {
		t.Fatalf("record feedback: %v", err)
	}
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) == 0 || leases[0].Status != contract.LeaseStatusCommitted {
		t.Fatalf("expected committed lease after feedback, got %+v", leases)
	}

	thirdReq := req
	thirdReq.RequestID = "req_scheduler_3"
	third, err := svc.Schedule(context.Background(), thirdReq)
	if err != nil {
		t.Fatalf("expected scheduling after release, got result=%+v err=%v", third, err)
	}
}

func TestRecordFailedFeedbackMarksLeaseFailed(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withMaxConcurrency(1), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	result, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	errorClass := "stream_interrupted"
	statusCode := 502
	feedback, err := svc.RecordFeedback(context.Background(), contract.RecordFeedbackRequest{
		RequestID:    result.Decision.RequestID,
		DecisionID:   result.Decision.ID,
		AttemptNo:    result.Decision.AttemptNo,
		AccountID:    result.Candidate.Account.ID,
		ProviderID:   result.Candidate.Provider.ID,
		Model:        result.Decision.Model,
		Success:      false,
		ErrorClass:   &errorClass,
		StatusCode:   &statusCode,
		LatencyMS:    123,
		InputTokens:  10,
		OutputTokens: 3,
	})
	if err != nil {
		t.Fatalf("record failed feedback: %v", err)
	}
	if feedback.ErrorClass == nil || *feedback.ErrorClass != errorClass || feedback.StatusCode == nil || *feedback.StatusCode != statusCode {
		t.Fatalf("expected failed feedback details, got %+v", feedback)
	}
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != contract.LeaseStatusFailed {
		t.Fatalf("expected failed lease after failed feedback, got %+v", leases)
	}
}

func TestLeaseExpiresAndFreesConcurrency(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.LeaseTTL = time.Nanosecond
	req.Candidates = []contract.Candidate{
		candidate(1, withMaxConcurrency(1), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	first, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("first schedule: %v", err)
	}
	if first.Lease.Status != contract.LeaseStatusPending {
		t.Fatalf("expected pending lease, got %+v", first.Lease)
	}
	time.Sleep(time.Millisecond)

	secondReq := req
	secondReq.RequestID = "req_scheduler_expired_2"
	second, err := svc.Schedule(context.Background(), secondReq)
	if err != nil {
		t.Fatalf("expected lease expiry to free concurrency, got result=%+v err=%v", second, err)
	}
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	foundExpired := false
	for _, lease := range leases {
		if lease.RequestID == first.Lease.RequestID && lease.Status == contract.LeaseStatusExpired {
			foundExpired = true
		}
	}
	if !foundExpired {
		t.Fatalf("expected first lease expired, got %+v", leases)
	}
}

func TestFallbackAttemptRecordsDecisionChainAndAttemptScopedLease(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.Candidates = []contract.Candidate{
		candidate(1, withMaxConcurrency(2), withHealth(0.95), withCapabilities(capabilitiescontract.KeyStreaming)),
		candidate(2, withMaxConcurrency(2), withHealth(0.90), withCapabilities(capabilitiescontract.KeyStreaming)),
	}
	first, err := svc.Schedule(context.Background(), req)
	if err != nil {
		t.Fatalf("first schedule: %v", err)
	}
	if first.Decision.AttemptNo != 1 || first.Lease.AttemptNo != 1 {
		t.Fatalf("expected first attempt=1, got decision=%+v lease=%+v", first.Decision, first.Lease)
	}

	secondReq := req
	secondReq.AttemptNo = 2
	secondReq.FallbackFromDecisionID = &first.Decision.ID
	secondReq.ExcludedAccountIDs = []int{first.Candidate.Account.ID}
	second, err := svc.Schedule(context.Background(), secondReq)
	if err != nil {
		t.Fatalf("fallback schedule: %v", err)
	}
	if second.Decision.AttemptNo != 2 || second.Lease.AttemptNo != 2 {
		t.Fatalf("expected fallback attempt=2, got decision=%+v lease=%+v", second.Decision, second.Lease)
	}
	if second.Decision.FallbackFromDecisionID == nil || *second.Decision.FallbackFromDecisionID != first.Decision.ID {
		t.Fatalf("expected fallback_from_decision_id=%d, got %+v", first.Decision.ID, second.Decision)
	}
	if second.Candidate.Account.ID == first.Candidate.Account.ID {
		t.Fatalf("expected excluded first account to be skipped, got %+v", second.Candidate.Account)
	}
	assertRejectReason(t, second.Decision.RejectReasons, first.Candidate.Account.ID, "fallback_excluded")

	_, err = svc.RecordFeedback(context.Background(), contract.RecordFeedbackRequest{
		RequestID:  first.Decision.RequestID,
		DecisionID: first.Decision.ID,
		AttemptNo:  first.Decision.AttemptNo,
		AccountID:  first.Candidate.Account.ID,
		ProviderID: first.Candidate.Provider.ID,
		Model:      first.Decision.Model,
		Success:    false,
	})
	if err != nil {
		t.Fatalf("record first feedback: %v", err)
	}
	leases, err := svc.ListLeases(context.Background())
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	statusByAttempt := map[int]contract.LeaseStatus{}
	for _, lease := range leases {
		statusByAttempt[lease.AttemptNo] = lease.Status
	}
	if statusByAttempt[1] != contract.LeaseStatusFailed || statusByAttempt[2] != contract.LeaseStatusPending {
		t.Fatalf("expected feedback to update only attempt 1, got %+v", leases)
	}
}

func TestScheduleRejectsUnknownCapabilityKeys(t *testing.T) {
	svc := newService(t)
	req := baseRequest()
	req.RequestCapabilities = []capabilitiescontract.Descriptor{
		{Key: "tool_callng", Level: capabilitiescontract.DescriptorLevelRequired, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
	}
	req.Candidates = []contract.Candidate{
		candidate(1, withCapabilities(capabilitiescontract.KeyToolCalling)),
	}
	if _, err := svc.Schedule(context.Background(), req); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for misspelled request capability, got %v", err)
	}

	req = baseRequest()
	req.RequestCapabilities = []capabilitiescontract.Descriptor{
		{Key: capabilitiescontract.KeyToolCalling, Level: capabilitiescontract.DescriptorLevelRequired, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
	}
	req.Candidates = []contract.Candidate{
		candidate(1, withCapabilities("tool_callng")),
	}
	if _, err := svc.Schedule(context.Background(), req); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for misspelled effective capability, got %v", err)
	}
}

func newService(t *testing.T) *service.Service {
	t.Helper()
	svc, err := service.New(schedulermemory.New(), nil)
	if err != nil {
		t.Fatalf("create scheduler service: %v", err)
	}
	return svc
}

type dynamicStrategyStore struct {
	*schedulermemory.Store
	strategies []contract.StrategyDescriptor
}

func (s *dynamicStrategyStore) ListActiveStrategies(_ context.Context) ([]contract.StrategyDescriptor, error) {
	return append([]contract.StrategyDescriptor(nil), s.strategies...), nil
}

func baseRequest() contract.ScheduleRequest {
	return contract.ScheduleRequest{
		RequestID:      "req_scheduler",
		UserID:         1,
		APIKeyID:       1,
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "gpt-test",
		Strategy:       contract.StrategyBalanced,
	}
}

func withCandidates(req contract.ScheduleRequest, candidates ...contract.Candidate) contract.ScheduleRequest {
	req.Candidates = candidates
	return req
}

type candidateOption func(*contract.Candidate)

func candidate(id int, opts ...candidateOption) contract.Candidate {
	out := contract.Candidate{
		Account: accountcontract.ProviderAccount{
			ID:                   id,
			ProviderID:           1,
			Name:                 "account",
			RuntimeClass:         accountcontract.RuntimeClassAPIKey,
			CredentialCiphertext: "encrypted",
			Status:               accountcontract.StatusActive,
			Weight:               1,
		},
		Provider: providercontract.Provider{
			ID:          1,
			Name:        "provider",
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
			Status:      providercontract.StatusActive,
		},
		Mapping: modelcontract.ModelProviderMapping{
			ID:                id,
			ModelID:           1,
			ProviderID:        1,
			UpstreamModelName: "gpt-test",
			Status:            modelcontract.StatusActive,
			PricingOverride:   map[string]any{},
		},
		EffectiveCapabilities: []capabilitiescontract.Descriptor{
			{Key: capabilitiescontract.KeyStreaming, Level: capabilitiescontract.DescriptorLevelRequired, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
		},
	}
	for _, opt := range opts {
		opt(&out)
	}
	return out
}

func noOptions() candidateOption {
	return func(*contract.Candidate) {}
}

func noRuntime() candidateOption {
	return func(*contract.Candidate) {}
}

func withMaxConcurrency(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.Limits.MaxConcurrency = &value }
}

func withConcurrency(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.CurrentConcurrency = value }
}

func withRPMLimit(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.Limits.RPMLimit = &value }
}

func withRPMUsed(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.RPMUsed = value }
}

func withTPMLimit(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.Limits.TPMLimit = &value }
}

func withTPMUsed(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.TPMUsed = value }
}

func withQuotaExhausted() candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.QuotaExhausted = true }
}

func withHealth(value float64) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.HealthScore = &value }
}

func withQuotaRemaining(value float64) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.QuotaRemainingRatio = &value }
}

func withLatencyP95MS(value int) candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.LatencyP95MS = &value }
}

func withProtectedAccount() candidateOption {
	return func(candidate *contract.Candidate) {
		if candidate.Account.Metadata == nil {
			candidate.Account.Metadata = map[string]any{}
		}
		candidate.Account.Metadata["quota_protected"] = true
	}
}

func withAccountMetadata(metadata map[string]any) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.Account.Metadata = metadata
	}
}

func withProviderConfig(config map[string]any) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.Provider.ConfigSchema = config
	}
}

func withCircuitOpen() candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.CircuitOpen = true }
}

func withCooldownActive() candidateOption {
	return func(candidate *contract.Candidate) { candidate.RuntimeState.CooldownActive = true }
}

func withAccountStatus(status accountcontract.Status) candidateOption {
	return func(candidate *contract.Candidate) { candidate.Account.Status = status }
}

func withCredential(ciphertext string) candidateOption {
	return func(candidate *contract.Candidate) { candidate.Account.CredentialCiphertext = ciphertext }
}

func withCapabilities(keys ...string) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.EffectiveCapabilities = make([]capabilitiescontract.Descriptor, 0, len(keys))
		for _, key := range keys {
			candidate.EffectiveCapabilities = append(candidate.EffectiveCapabilities, capabilitiescontract.Descriptor{
				Key:     key,
				Level:   capabilitiescontract.DescriptorLevelRequired,
				Status:  capabilitiescontract.DescriptorStatusStable,
				Version: "v1",
			})
		}
	}
}

func withRelativeCost(value string) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.Mapping.PricingOverride["relative_cost"] = value
	}
}

func withCacheScore(value string) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.Mapping.PricingOverride["cache_score"] = value
	}
}

func withQualityScore(value string) candidateOption {
	return func(candidate *contract.Candidate) {
		candidate.Mapping.PricingOverride["quality_score"] = value
	}
}

func assertRejectReason(t *testing.T, reasons map[string]any, accountID int, expected string) {
	t.Helper()
	got, ok := reasons["account_"+strconv.Itoa(accountID)]
	if !ok {
		t.Fatalf("missing reject reason for account %d in %+v", accountID, reasons)
	}
	if got != expected {
		t.Fatalf("expected account %d reject reason %q, got %v", accountID, expected, got)
	}
}

func selectedAccountID(decision contract.Decision) int {
	return selectedAccountIDPtr(decision.SelectedAccountID)
}

func selectedAccountIDPtr(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func decisionScore(t *testing.T, scores map[string]any, accountID int) map[string]any {
	t.Helper()
	raw, ok := scores["account_"+strconv.Itoa(accountID)]
	if !ok {
		t.Fatalf("missing score for account %d in %+v", accountID, scores)
	}
	score, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected score map for account %d, got %T %+v", accountID, raw, raw)
	}
	return score
}

func paretoFrontierIDs(t *testing.T, scores map[string]any) []int {
	t.Helper()
	rawPareto, ok := scores["pareto"]
	if !ok {
		t.Fatalf("missing Pareto evidence in %+v", scores)
	}
	pareto, ok := rawPareto.(map[string]any)
	if !ok {
		t.Fatalf("expected Pareto evidence map, got %T %+v", rawPareto, rawPareto)
	}
	rawIDs, ok := pareto["frontier_account_ids"].([]any)
	if !ok {
		t.Fatalf("expected Pareto frontier account IDs, got %+v", pareto)
	}
	out := make([]int, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		switch typed := rawID.(type) {
		case float64:
			out = append(out, int(typed))
		case int:
			out = append(out, typed)
		default:
			t.Fatalf("unexpected Pareto frontier account ID type %T in %+v", rawID, rawIDs)
		}
	}
	return out
}
