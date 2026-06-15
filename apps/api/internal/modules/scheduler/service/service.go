package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/metacoerce"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// feedbackSignalWindow bounds how far back the scheduler scans recent feedback
// signals when scoring candidates. This scan runs on every routing decision, so
// the window is a direct hot-path cost. 14 days is ample for the EWMA-style
// recency signals we derive (older data is already heavily discounted) while
// halving the rows scanned per request versus the original 30-day window.
const feedbackSignalWindow = 14 * 24 * time.Hour
const strategyRefreshInterval = 30 * time.Second

type Service struct {
	store            contract.Store
	clock            Clock
	registry         *StrategyRegistry
	strategyCacheMu  sync.Mutex
	strategyLoadedAt time.Time
	strategyDirty    bool
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock, registry: NewStrategyRegistry(), strategyDirty: true}, nil
}

func (s *Service) Schedule(ctx context.Context, req contract.ScheduleRequest) (result contract.ScheduleResult, err error) {
	ctx, span := platformotel.StartSpan(ctx, "scheduler.Schedule",
		attribute.String("srapi.request_id", strings.TrimSpace(req.RequestID)),
		attribute.Int("srapi.scheduler.attempt_no", scheduleAttemptNo(req.AttemptNo)),
		attribute.String("srapi.scheduler.strategy", string(req.Strategy)),
		attribute.String("srapi.gateway.model", strings.TrimSpace(req.Model)),
		attribute.String("srapi.gateway.source_endpoint", strings.TrimSpace(req.SourceEndpoint)),
		attribute.Int("srapi.scheduler.candidate_count", len(req.Candidates)),
	)
	defer func() {
		platformotel.EndSpan(span, err, schedulerTraceErrorType(err), schedulerTraceAttrs(result, err)...)
	}()

	if err := validateScheduleRequest(req); err != nil {
		return contract.ScheduleResult{}, ErrInvalidInput
	}
	if err := normalizeScheduleCapabilities(&req); err != nil {
		return contract.ScheduleResult{}, ErrInvalidInput
	}
	if err := s.enrichFeedbackSignals(ctx, &req); err != nil {
		return contract.ScheduleResult{}, err
	}
	if err := s.ensureStrategiesFresh(ctx); err != nil {
		return contract.ScheduleResult{}, err
	}
	if err := s.applyStrategyRollout(&req); err != nil {
		return contract.ScheduleResult{}, err
	}

	strategy, err := s.registry.Resolve(req.Strategy, strategyScopeKeys(req)...)
	if err != nil {
		return contract.ScheduleResult{}, err
	}
	attemptNo := scheduleAttemptNo(req.AttemptNo)
	evaluation := s.evaluateSchedule(req, strategy)
	if evaluation.err != nil {
		decision, _, err := s.createDecisionWithSnapshot(ctx, req, strategy, evaluation.decision, evaluation.candidatesByRank)
		if err != nil {
			return contract.ScheduleResult{}, err
		}
		return contract.ScheduleResult{Decision: decision}, evaluation.err
	}
	lease, err := s.acquireLease(ctx, req, attemptNo, evaluation.selected)
	if err != nil {
		evaluation.rejectReasons[accountKey(evaluation.selected.Account.ID)] = "concurrency_full"
		decisionInput := s.buildDecision(req, strategy, nil, len(req.Candidates), len(evaluation.rejectReasons), evaluation.scorePayload, evaluation.rejectReasons)
		decision, _, err := s.createDecisionWithSnapshot(ctx, req, strategy, decisionInput, evaluation.candidatesByRank)
		if err != nil {
			return contract.ScheduleResult{}, err
		}
		return contract.ScheduleResult{Decision: decision}, ErrNoAvailableAccount
	}
	decision, _, err := s.createDecisionWithSnapshot(ctx, req, strategy, evaluation.decision, evaluation.candidatesByRank)
	if err != nil {
		if _, releaseErr := s.store.UpdateLeaseStatus(ctx, strings.TrimSpace(req.RequestID), attemptNo, contract.LeaseStatusReleased); releaseErr != nil {
			slog.Warn("scheduler: failed to release lease after decision creation error",
				"request_id", strings.TrimSpace(req.RequestID),
				"attempt_no", attemptNo,
				"account_id", evaluation.selected.Account.ID,
				"error", releaseErr)
		}
		return contract.ScheduleResult{}, err
	}
	return contract.ScheduleResult{Decision: decision, Candidate: evaluation.selected, Candidates: evaluation.candidatesByRank, Lease: lease}, nil
}

func schedulerTraceErrorType(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrNoAvailableAccount):
		return "no_available_account"
	case errors.Is(err, ErrUserBalanceInsufficient):
		return "user_balance_insufficient"
	default:
		return "scheduler_error"
	}
}

func schedulerTraceAttrs(result contract.ScheduleResult, err error) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String("srapi.scheduler.outcome", "error")}
	if err == nil {
		attrs[0] = attribute.String("srapi.scheduler.outcome", "selected")
	} else if errors.Is(err, ErrNoAvailableAccount) || errors.Is(err, ErrUserBalanceInsufficient) {
		attrs[0] = attribute.String("srapi.scheduler.outcome", "rejected")
	}
	if result.Decision.ID > 0 {
		attrs = append(attrs, attribute.Int("srapi.scheduler.decision_id", result.Decision.ID))
	}
	if result.Decision.CandidateCount > 0 {
		attrs = append(attrs, attribute.Int("srapi.scheduler.candidate_count", result.Decision.CandidateCount))
	}
	attrs = append(attrs, attribute.Int("srapi.scheduler.rejected_count", result.Decision.RejectedCount))
	if result.Decision.SelectedProviderID != nil {
		attrs = append(attrs, attribute.Int("srapi.scheduler.selected_provider_id", *result.Decision.SelectedProviderID))
	}
	if result.Decision.SelectedAccountID != nil {
		attrs = append(attrs, attribute.Int("srapi.scheduler.selected_account_id", *result.Decision.SelectedAccountID))
	}
	if result.Decision.TargetProtocol != "" {
		attrs = append(attrs, attribute.String("srapi.provider.protocol", result.Decision.TargetProtocol))
	}
	return attrs
}

func validateScheduleRequest(req contract.ScheduleRequest) error {
	requestID := strings.TrimSpace(req.RequestID)
	model := strings.TrimSpace(req.Model)
	sourceEndpoint := strings.TrimSpace(req.SourceEndpoint)
	if requestID == "" || req.UserID <= 0 || req.APIKeyID <= 0 || model == "" || sourceEndpoint == "" {
		return ErrInvalidInput
	}
	return nil
}

func normalizeScheduleCapabilities(req *contract.ScheduleRequest) error {
	requestCapabilities, err := capabilitiescontract.NormalizeDescriptors(req.RequestCapabilities)
	if err != nil {
		return err
	}
	req.RequestCapabilities = requestCapabilities
	for idx := range req.Candidates {
		effectiveCapabilities, err := capabilitiescontract.NormalizeDescriptors(req.Candidates[idx].EffectiveCapabilities)
		if err != nil {
			return err
		}
		req.Candidates[idx].EffectiveCapabilities = effectiveCapabilities
	}
	return nil
}

type scheduleEvaluation struct {
	decision         contract.Decision
	selected         contract.Candidate
	candidatesByRank []contract.Candidate
	scorePayload     map[string]any
	rejectReasons    map[string]any
	err              error
}

func (s *Service) evaluateSchedule(req contract.ScheduleRequest, strategy contract.StrategyDescriptor) scheduleEvaluation {
	if req.UserBalanceInsufficient {
		rejectReasons := rejectAllCandidates(req.Candidates, "user_balance_insufficient")
		return scheduleEvaluation{
			decision:      s.buildDecision(req, strategy, nil, len(req.Candidates), len(rejectReasons), nil, rejectReasons),
			rejectReasons: rejectReasons,
			err:           ErrUserBalanceInsufficient,
		}
	}

	scores := make([]candidateScore, 0, len(req.Candidates))
	rejectReasons := map[string]any{}
	eligible := make([]contract.Candidate, 0, len(req.Candidates))
	for _, candidate := range req.Candidates {
		reason := rejectReason(candidate, req)
		if reason != "" {
			rejectReasons[accountKey(candidate.Account.ID)] = reason
			continue
		}
		eligible = append(eligible, candidate)
	}
	selectedTier, deferredTier := filterByPriorityTier(eligible, req.StickyAccountID)
	for _, candidate := range deferredTier {
		rejectReasons[accountKey(candidate.Account.ID)] = "lower_priority_tier"
	}
	for _, candidate := range selectedTier {
		scores = append(scores, candidateScore{
			Candidate: candidate,
			Score:     scoreCandidate(candidate, req, strategy),
		})
	}
	addStickyBrokenReason(rejectReasons, req)
	if len(scores) == 0 {
		return scheduleEvaluation{
			decision:      s.buildDecision(req, strategy, nil, len(req.Candidates), len(rejectReasons), nil, rejectReasons),
			rejectReasons: rejectReasons,
			err:           ErrNoAvailableAccount,
		}
	}

	scorePayload := map[string]any{}
	for _, score := range scores {
		scorePayload[accountKey(score.Candidate.Account.ID)] = score.Score
	}
	frontier := paretoFrontier(scores)
	sortCandidateScores(frontier, req.RequestID)
	sortCandidateScores(scores, req.RequestID)
	scorePayload["pareto"] = map[string]any{
		"objective":            "cost_latency_quality",
		"frontier_account_ids": paretoFrontierAccountIDs(frontier),
	}
	selected := frontier[0].Candidate
	// Rank the selected priority tier, then append the deferred lower tiers so the
	// candidate list still reflects every eligible account. The winner stays in
	// the top tier, but the gateway failover loop can see (and, after excluding a
	// failed top-tier account, re-schedule onto) the lower tiers.
	candidatesByRank := append(rankedCandidates(frontier, scores), deferredTier...)
	return scheduleEvaluation{
		decision:         s.buildDecision(req, strategy, &selected, len(req.Candidates), len(rejectReasons), scorePayload, rejectReasons),
		selected:         selected,
		candidatesByRank: candidatesByRank,
		scorePayload:     scorePayload,
		rejectReasons:    rejectReasons,
	}
}

func (s *Service) ListDecisions(ctx context.Context) ([]contract.Decision, error) {
	return s.store.ListDecisions(ctx)
}

func (s *Service) createDecisionWithSnapshot(ctx context.Context, req contract.ScheduleRequest, strategy contract.StrategyDescriptor, decision contract.Decision, ranked []contract.Candidate) (contract.Decision, contract.RequestSnapshot, error) {
	snapshot := s.buildRequestSnapshot(req, strategy, decision, ranked)
	return s.store.CreateDecisionWithSnapshot(ctx, decision, snapshot)
}

func (s *Service) ListRequestSnapshots(ctx context.Context) ([]contract.RequestSnapshot, error) {
	return s.store.ListRequestSnapshots(ctx)
}

func (s *Service) applyStrategyRollout(req *contract.ScheduleRequest) error {
	if req == nil || !req.StrategyRollout.Enabled {
		return nil
	}
	rollout := req.StrategyRollout
	if math.IsNaN(rollout.Percent) || math.IsInf(rollout.Percent, 0) || rollout.Percent < 0 || rollout.Percent > 100 {
		return ErrInvalidInput
	}
	if rollout.ShadowStrategy == "" {
		return ErrInvalidInput
	}
	if _, err := s.registry.Resolve(rollout.ShadowStrategy, strategyScopeKeys(*req)...); err != nil {
		return err
	}
	key := strings.TrimSpace(rollout.Key)
	if key == "" {
		key = strings.TrimSpace(req.RequestID)
	}
	if key == "" {
		return ErrInvalidInput
	}
	bucket := rolloutBucket(key)
	rollout.Key = ""
	rollout.Bucket = bucket
	rollout.KeyHash = affinityKeyHash(key)
	rollout.ShadowSelected = bucket < rollout.Percent || rollout.Percent >= 100
	req.StrategyRollout = rollout
	if rollout.ShadowSelected {
		req.Strategy = rollout.ShadowStrategy
		req.Warnings = appendUniqueString(req.Warnings, "strategy_rollout_shadow_selected")
		return nil
	}
	req.Warnings = appendUniqueString(req.Warnings, "strategy_rollout_current_selected")
	return nil
}

func (s *Service) RecordFeedback(ctx context.Context, req contract.RecordFeedbackRequest) (contract.Feedback, error) {
	if strings.TrimSpace(req.RequestID) == "" || req.DecisionID <= 0 || req.AttemptNo <= 0 || req.AccountID <= 0 || req.ProviderID <= 0 {
		return contract.Feedback{}, ErrInvalidInput
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "USD"
	}
	actualCost := strings.TrimSpace(req.ActualCost)
	if actualCost == "" {
		actualCost = "0.00000000"
	}
	feedback, err := s.store.CreateFeedback(ctx, contract.Feedback{
		RequestID:    strings.TrimSpace(req.RequestID),
		DecisionID:   req.DecisionID,
		AttemptNo:    req.AttemptNo,
		AccountID:    req.AccountID,
		ProviderID:   req.ProviderID,
		Model:        strings.TrimSpace(req.Model),
		Success:      req.Success,
		ErrorClass:   req.ErrorClass,
		StatusCode:   req.StatusCode,
		LatencyMS:    req.LatencyMS,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		CachedTokens: req.CachedTokens,
		ActualCost:   actualCost,
		Currency:     currency,
		CreatedAt:    s.clock.Now(),
	})
	if err != nil {
		return contract.Feedback{}, err
	}
	status := contract.LeaseStatusCommitted
	if !req.Success {
		status = contract.LeaseStatusFailed
	}
	if _, err := s.store.UpdateLeaseStatus(ctx, strings.TrimSpace(req.RequestID), req.AttemptNo, status); err != nil {
		slog.Warn("scheduler: failed to update lease status on feedback",
			"request_id", strings.TrimSpace(req.RequestID),
			"attempt_no", req.AttemptNo,
			"account_id", req.AccountID,
			"lease_status", string(status),
			"error", err)
		return feedback, nil
	}
	return feedback, nil
}

func (s *Service) ListFeedbacks(ctx context.Context) ([]contract.Feedback, error) {
	return s.store.ListFeedbacks(ctx)
}

func (s *Service) ListStrategies(ctx context.Context) ([]contract.StrategyDescriptor, error) {
	if err := s.RefreshStrategies(ctx); err != nil {
		return nil, err
	}
	return s.registry.List(), nil
}

func (s *Service) ListLeases(ctx context.Context) ([]contract.Lease, error) {
	return s.store.ListLeases(ctx)
}

// ActiveLeaseCount returns the live number of pending scheduler leases without
// requiring metrics collection to scan historical lease rows.
func (s *Service) ActiveLeaseCount(ctx context.Context) int {
	if s == nil {
		return 0
	}
	if counter, ok := s.store.(contract.ActiveLeaseCounter); ok {
		count, err := counter.CountActiveLeases(ctx)
		if err == nil && count >= 0 {
			return count
		}
		return 0
	}
	leases, err := s.store.ListLeases(ctx)
	if err != nil {
		return 0
	}
	count := 0
	for _, lease := range leases {
		if lease.Status == contract.LeaseStatusPending {
			count++
		}
	}
	return count
}

// AccountConcurrency returns the live number of in-flight scheduler leases for
// an account, or 0 when the backing store cannot report it (e.g. the in-memory
// store or an unavailable cache). The gateway uses it to populate
// Candidate.RuntimeState.CurrentConcurrency so load-aware scoring reflects real
// traffic. Best-effort: errors degrade to 0 rather than failing scheduling.
func (s *Service) AccountConcurrency(ctx context.Context, accountID int) int {
	if s == nil || accountID <= 0 {
		return 0
	}
	counter, ok := s.store.(contract.AccountConcurrencyCounter)
	if !ok {
		return 0
	}
	count, err := counter.CountAccountConcurrency(ctx, accountID)
	if err != nil || count < 0 {
		return 0
	}
	return count
}

// AccountLastUsed returns when an account was last selected (epoch ms), or 0
// when the backing store cannot report it. Used as a least-recently-used
// scheduling tie-breaker so equally-scored accounts share load.
func (s *Service) AccountLastUsed(ctx context.Context, accountID int) int64 {
	if s == nil || accountID <= 0 {
		return 0
	}
	reporter, ok := s.store.(contract.AccountLastUsedReporter)
	if !ok {
		return 0
	}
	last, err := reporter.AccountLastUsed(ctx, accountID)
	if err != nil || last < 0 {
		return 0
	}
	return last
}

// AccountConcurrencyBatch resolves live in-flight lease counts for many
// accounts at once, preferring the store's batched reader (one round trip)
// over per-account reads. Best-effort like AccountConcurrency: store errors
// degrade to missing entries rather than failing scheduling.
func (s *Service) AccountConcurrencyBatch(ctx context.Context, accountIDs []int) map[int]int {
	if s == nil || len(accountIDs) == 0 {
		return map[int]int{}
	}
	if reader, ok := s.store.(contract.AccountRuntimeBatchReader); ok {
		if counts, err := reader.CountAccountConcurrencyBatch(ctx, accountIDs); err == nil {
			return counts
		}
	}
	out := make(map[int]int, len(accountIDs))
	if _, ok := s.store.(contract.AccountConcurrencyCounter); !ok {
		return out
	}
	for _, accountID := range accountIDs {
		if count := s.AccountConcurrency(ctx, accountID); count > 0 {
			out[accountID] = count
		}
	}
	return out
}

// AccountLastUsedBatch resolves last-used markers (epoch ms) for many accounts
// at once, preferring the store's batched reader over per-account reads.
// Best-effort: errors degrade to missing entries.
func (s *Service) AccountLastUsedBatch(ctx context.Context, accountIDs []int) map[int]int64 {
	if s == nil || len(accountIDs) == 0 {
		return map[int]int64{}
	}
	if reader, ok := s.store.(contract.AccountRuntimeBatchReader); ok {
		if marks, err := reader.AccountLastUsedBatch(ctx, accountIDs); err == nil {
			return marks
		}
	}
	out := make(map[int]int64, len(accountIDs))
	if _, ok := s.store.(contract.AccountLastUsedReporter); !ok {
		return out
	}
	for _, accountID := range accountIDs {
		if last := s.AccountLastUsed(ctx, accountID); last > 0 {
			out[accountID] = last
		}
	}
	return out
}

func (s *Service) acquireLease(ctx context.Context, req contract.ScheduleRequest, attemptNo int, selected contract.Candidate) (contract.Lease, error) {
	ttl := req.LeaseTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := s.clock.Now()
	return s.store.AcquireLease(ctx, contract.Lease{
		ID:        fmt.Sprintf("lease_%s_%d_%d", strings.TrimSpace(req.RequestID), attemptNo, selected.Account.ID),
		RequestID: strings.TrimSpace(req.RequestID),
		AttemptNo: attemptNo,
		AccountID: selected.Account.ID,
		Status:    contract.LeaseStatusPending,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
		UpdatedAt: now,
	}, selected.Limits.MaxConcurrency)
}

func (s *Service) buildDecision(req contract.ScheduleRequest, strategy contract.StrategyDescriptor, selected *contract.Candidate, candidateCount int, rejectedCount int, scores map[string]any, rejectReasons map[string]any) contract.Decision {
	sourceProtocol := strings.TrimSpace(req.SourceProtocol)
	if sourceProtocol == "" {
		sourceProtocol = "openai-compatible"
	}
	targetProtocol := strings.TrimSpace(req.TargetProtocol)
	if targetProtocol == "" && selected != nil {
		targetProtocol = selected.Provider.Protocol
	}
	estimatedCost := strings.TrimSpace(req.EstimatedCost)
	if estimatedCost == "" {
		estimatedCost = "0.00000000"
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "USD"
	}
	decision := contract.Decision{
		RequestID:              strings.TrimSpace(req.RequestID),
		AttemptNo:              scheduleAttemptNo(req.AttemptNo),
		UserID:                 req.UserID,
		APIKeyID:               req.APIKeyID,
		SourceProtocol:         sourceProtocol,
		SourceEndpoint:         strings.TrimSpace(req.SourceEndpoint),
		TargetProtocol:         targetProtocol,
		Model:                  strings.TrimSpace(req.Model),
		Strategy:               strategy.Name,
		StrategyVersion:        strategy.Version,
		StrategyConfigHash:     strategy.ConfigHash,
		FallbackFromDecisionID: cloneIntPtr(req.FallbackFromDecisionID),
		CandidateCount:         candidateCount,
		RejectedCount:          rejectedCount,
		Scores:                 scores,
		RejectReasons:          rejectReasons,
		StrategyWeights:        strategyWeightsPayload(strategy),
		CompatibilityWarnings:  cloneStrings(req.Warnings),
		SelectionRationale:     selectionRationale(selected, candidateCount, rejectedCount, scores, rejectReasons),
		EstimatedCost:          estimatedCost,
		Currency:               currency,
		CreatedAt:              s.clock.Now(),
	}
	attachRoutingHints(&decision, req)
	attachPricingEvidence(&decision, req)
	if selected != nil {
		providerID := selected.Provider.ID
		accountID := selected.Account.ID
		decision.SelectedProviderID = &providerID
		decision.SelectedAccountID = &accountID
		decision.StickyHit = stickyScore(*selected, req) > 0
		decision.CacheAffinityHit = cacheScore(*selected, req, healthScore(*selected)) > 0
	}
	return decision
}

func (s *Service) buildRequestSnapshot(req contract.ScheduleRequest, strategy contract.StrategyDescriptor, decision contract.Decision, ranked []contract.Candidate) contract.RequestSnapshot {
	return contract.RequestSnapshot{
		RequestID:             decision.RequestID,
		AttemptNo:             decision.AttemptNo,
		DecisionID:            decision.ID,
		RequestProfile:        sanitizedRequestProfile(req),
		CandidateSnapshot:     sanitizedCandidateSnapshots(req.Candidates),
		RejectedSnapshot:      cloneMapAny(decision.RejectReasons),
		RankedAccountIDs:      rankedAccountIDs(ranked),
		SelectedAccountID:     cloneIntPtr(decision.SelectedAccountID),
		SelectedProviderID:    cloneIntPtr(decision.SelectedProviderID),
		Strategy:              strategy.Name,
		StrategyVersion:       strategy.Version,
		StrategyConfigHash:    strategy.ConfigHash,
		StrategyWeights:       cloneMapAny(decision.StrategyWeights),
		CompatibilityWarnings: cloneStrings(decision.CompatibilityWarnings),
		CreatedAt:             decision.CreatedAt,
	}
}

func selectionRationale(selected *contract.Candidate, candidateCount int, rejectedCount int, scores map[string]any, rejectReasons map[string]any) string {
	if selected == nil {
		if rejectedCount == 0 {
			return "No account selected because no schedulable candidates were available."
		}
		reason, count := mostCommonRejectReason(rejectReasons)
		if reason == "" {
			return fmt.Sprintf("No account selected because %d of %d candidates were rejected.", rejectedCount, candidateCount)
		}
		return fmt.Sprintf("No account selected because %d of %d candidates were rejected; the most common reason was %s (%d).", rejectedCount, candidateCount, reason, count)
	}

	selectedScore, ok := scoreForAccount(scores, selected.Account.ID)
	if !ok {
		return fmt.Sprintf("Selected account %d on provider %d after %d eligible candidates were evaluated.", selected.Account.ID, selected.Provider.ID, candidateCount-rejectedCount)
	}

	factors := topScoreFactors(selectedScore, 2)
	rationale := fmt.Sprintf("Selected account %d on provider %d with final score %.3f", selected.Account.ID, selected.Provider.ID, selectedScore.Final)
	if len(factors) > 0 {
		rationale += " driven by " + strings.Join(factors, " and ")
	}
	if nextScore, nextOK := runnerUpScore(scores, selected.Account.ID); nextOK {
		rationale += fmt.Sprintf("; next best account %d scored %.3f", nextScore.AccountID, nextScore.Final)
	}
	if rejectedCount > 0 {
		rationale += fmt.Sprintf("; %d of %d candidates were rejected", rejectedCount, candidateCount)
	}
	return rationale + "."
}

func scoreForAccount(scores map[string]any, accountID int) (scoreBreakdown, bool) {
	value, ok := scores[accountKey(accountID)]
	if !ok {
		return scoreBreakdown{}, false
	}
	return scoreBreakdownValue(value)
}

func runnerUpScore(scores map[string]any, selectedAccountID int) (scoreBreakdown, bool) {
	var runnerUp scoreBreakdown
	found := false
	for key, value := range scores {
		if key == "pareto" {
			continue
		}
		score, ok := scoreBreakdownValue(value)
		if !ok || score.AccountID == selectedAccountID {
			continue
		}
		if !found || score.Final > runnerUp.Final || (score.Final == runnerUp.Final && score.AccountID < runnerUp.AccountID) {
			runnerUp = score
			found = true
		}
	}
	return runnerUp, found
}

func scoreBreakdownValue(value any) (scoreBreakdown, bool) {
	switch typed := value.(type) {
	case scoreBreakdown:
		return typed, true
	case map[string]any:
		raw, err := json.Marshal(typed)
		if err != nil {
			return scoreBreakdown{}, false
		}
		var score scoreBreakdown
		if err := json.Unmarshal(raw, &score); err != nil {
			return scoreBreakdown{}, false
		}
		return score, score.AccountID > 0
	default:
		return scoreBreakdown{}, false
	}
}

func topScoreFactors(score scoreBreakdown, limit int) []string {
	type factor struct {
		name  string
		value float64
	}
	factors := []factor{
		{name: "health", value: score.Health},
		{name: "quota", value: score.Quota},
		{name: "latency", value: score.Latency},
		{name: "quality", value: score.Quality},
		{name: "sticky", value: score.Sticky},
		{name: "cache", value: score.Cache},
		{name: "cost", value: score.Cost},
		{name: "fairness", value: score.Fairness},
	}
	sort.SliceStable(factors, func(i, j int) bool {
		if factors[i].value == factors[j].value {
			return factors[i].name < factors[j].name
		}
		return factors[i].value > factors[j].value
	})
	out := make([]string, 0, limit)
	for _, factor := range factors {
		if factor.value <= 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s %.2f", factor.name, factor.value))
		if len(out) == limit {
			break
		}
	}
	return out
}

func mostCommonRejectReason(rejectReasons map[string]any) (string, int) {
	counts := map[string]int{}
	for _, value := range rejectReasons {
		reason := strings.TrimSpace(fmt.Sprint(value))
		if reason == "" {
			continue
		}
		counts[reason]++
	}
	bestReason := ""
	bestCount := 0
	for reason, count := range counts {
		if count > bestCount || (count == bestCount && reason < bestReason) {
			bestReason = reason
			bestCount = count
		}
	}
	return bestReason, bestCount
}

func sanitizedRequestProfile(req contract.ScheduleRequest) map[string]any {
	profile := map[string]any{
		"request_id":                strings.TrimSpace(req.RequestID),
		"attempt_no":                scheduleAttemptNo(req.AttemptNo),
		"fallback_from_decision_id": cloneIntPtr(req.FallbackFromDecisionID),
		"user_id":                   req.UserID,
		"api_key_id":                req.APIKeyID,
		"source_protocol":           defaultString(req.SourceProtocol, "openai-compatible"),
		"source_endpoint":           strings.TrimSpace(req.SourceEndpoint),
		"target_protocol":           strings.TrimSpace(req.TargetProtocol),
		"model":                     strings.TrimSpace(req.Model),
		"model_alias":               strings.TrimSpace(req.ModelAlias),
		"fallback_models":           cloneStrings(req.FallbackModels),
		"account_group_scope":       cloneInts(req.AccountGroupScope),
		"user_tier":                 string(req.UserTier),
		"user_balance_insufficient": req.UserBalanceInsufficient,
		"estimated_input_tokens":    req.EstimatedInputTokens,
		"estimated_output_tokens":   req.EstimatedOutputTokens,
		"estimated_cost":            strings.TrimSpace(req.EstimatedCost),
		"currency":                  strings.TrimSpace(req.Currency),
		"pricing_rule_id":           cloneIntPtr(req.PricingRuleID),
		"pricing_source":            strings.TrimSpace(req.PricingSource),
		"pricing_estimated":         req.PricingEstimated,
		"is_stream":                 req.IsStream,
		"sticky_account_id":         cloneIntPtr(req.StickyAccountID),
		"sticky_strength":           string(req.StickyStrength),
		"strategy":                  string(req.Strategy),
		"warnings":                  cloneStrings(req.Warnings),
		"request_capabilities":      req.RequestCapabilities,
		"excluded_account_ids":      cloneInts(req.ExcludedAccountIDs),
	}
	if source := strings.TrimSpace(req.SessionAffinitySource); source != "" {
		profile["session_affinity_source"] = source
	}
	if hash := affinityKeyHash(req.SessionAffinityKey); hash != "" {
		profile["session_affinity_key_hash"] = hash
	}
	if hints := routingHints(req); len(hints) > 0 {
		profile["routing_hints"] = hints
	}
	return removeNilValues(profile)
}

func sanitizedCandidateSnapshots(candidates []contract.Candidate) []contract.CandidateSnapshot {
	out := make([]contract.CandidateSnapshot, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, contract.CandidateSnapshot{
			AccountID:             candidate.Account.ID,
			ProviderID:            candidate.Provider.ID,
			MappingID:             candidate.Mapping.ID,
			ModelID:               candidate.Mapping.ModelID,
			RuntimeClass:          string(candidate.Account.RuntimeClass),
			AccountHasCredential:  boolPtr(strings.TrimSpace(candidate.Account.CredentialCiphertext) != ""),
			AccountStatus:         string(candidate.Account.Status),
			AccountWeight:         candidate.Account.Weight,
			AccountRiskLevel:      cloneStringPtr(candidate.Account.RiskLevel),
			AccountMetadata:       sanitizeSnapshotMap(candidate.Account.Metadata),
			ProviderProtocol:      strings.TrimSpace(candidate.Provider.Protocol),
			ProviderStatus:        string(candidate.Provider.Status),
			ProviderCapabilities:  sanitizeSnapshotMap(candidate.Provider.Capabilities),
			ProviderConfig:        sanitizeSnapshotMap(candidate.Provider.ConfigSchema),
			MappingStatus:         string(candidate.Mapping.Status),
			UpstreamModelName:     strings.TrimSpace(candidate.Mapping.UpstreamModelName),
			PricingOverride:       sanitizeSnapshotMap(candidate.Mapping.PricingOverride),
			EffectiveCapabilities: cloneCapabilityDescriptors(candidate.EffectiveCapabilities),
			RuntimeState:          candidate.RuntimeState,
			Limits:                cloneRuntimeLimits(candidate.Limits),
		})
	}
	return out
}

func rankedAccountIDs(candidates []contract.Candidate) []int {
	out := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.Account.ID)
	}
	return out
}

func attachPricingEvidence(decision *contract.Decision, req contract.ScheduleRequest) {
	evidence := map[string]any{}
	if req.EstimatedInputTokens > 0 {
		evidence["estimated_input_tokens"] = req.EstimatedInputTokens
	}
	if req.EstimatedOutputTokens > 0 {
		evidence["estimated_output_tokens"] = req.EstimatedOutputTokens
	}
	if cost := strings.TrimSpace(req.EstimatedCost); cost != "" {
		evidence["estimated_cost"] = cost
	}
	if currency := strings.TrimSpace(req.Currency); currency != "" {
		evidence["currency"] = currency
	}
	if req.PricingRuleID != nil {
		evidence["pricing_rule_id"] = *req.PricingRuleID
	}
	if source := strings.TrimSpace(req.PricingSource); source != "" {
		evidence["pricing_source"] = source
	}
	evidence["pricing_estimated"] = req.PricingEstimated
	if len(evidence) == 1 && !req.PricingEstimated {
		return
	}
	if decision.Scores == nil {
		decision.Scores = map[string]any{}
	}
	decision.Scores["pricing"] = evidence
}

func attachRoutingHints(decision *contract.Decision, req contract.ScheduleRequest) {
	hints := routingHints(req)
	if len(hints) == 0 {
		return
	}
	if decision.Scores == nil {
		decision.Scores = map[string]any{}
	}
	decision.Scores["routing_hints"] = hints
}

func routingHints(req contract.ScheduleRequest) map[string]any {
	hints := map[string]any{}
	if alias := strings.TrimSpace(req.ModelAlias); alias != "" {
		hints["model_alias"] = alias
	}
	if len(req.FallbackModels) > 0 {
		hints["fallback_models"] = cloneStrings(req.FallbackModels)
	}
	if source := strings.TrimSpace(req.SessionAffinitySource); source != "" {
		hints["session_affinity_source"] = source
	}
	if keyHash := affinityKeyHash(req.SessionAffinityKey); keyHash != "" {
		hints["session_affinity_key_hash"] = keyHash
	}
	if req.StickyAccountID != nil {
		hints["sticky_account_id"] = *req.StickyAccountID
	}
	if req.StickyStrength != "" {
		hints["sticky_strength"] = string(req.StickyStrength)
	}
	if rollout := strategyRolloutHint(req.StrategyRollout); len(rollout) > 0 {
		hints["strategy_rollout"] = rollout
	}
	return hints
}

func affinityKeyHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func strategyRolloutHint(rollout contract.StrategyRollout) map[string]any {
	if !rollout.Enabled {
		return nil
	}
	return map[string]any{
		"shadow_strategy":  string(rollout.ShadowStrategy),
		"percent":          rollout.Percent,
		"bucket":           rollout.Bucket,
		"shadow_selected":  rollout.ShadowSelected,
		"rollout_key_hash": strings.TrimSpace(rollout.KeyHash),
	}
}

func rolloutBucket(key string) float64 {
	sum := sha256Sum(key)
	value := binary.BigEndian.Uint64(sum[:8])
	return float64(value%10000) / 100
}

func sha256Sum(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

type candidateScore struct {
	Candidate contract.Candidate
	Score     scoreBreakdown
}

func sortCandidateScores(scores []candidateScore, requestID string) {
	sort.SliceStable(scores, func(i, j int) bool {
		a, b := scores[i], scores[j]
		if a.Score.Final != b.Score.Final {
			return a.Score.Final > b.Score.Final
		}
		// Equal score: rotate load instead of always picking the same account.
		// 1) least-recently-used first (0 = never used within the window = oldest).
		la, lb := a.Candidate.RuntimeState.LastUsedUnixMs, b.Candidate.RuntimeState.LastUsedUnixMs
		if la != lb {
			return la < lb
		}
		// 2) request-seeded hash so a cold pool (all last-used 0) still spreads
		//    uniformly across requests while staying deterministic per request
		//    (replay-safe: requestID is part of the persisted snapshot).
		ha, hb := tieBreakHash(requestID, a.Candidate.Account.ID), tieBreakHash(requestID, b.Candidate.Account.ID)
		if ha != hb {
			return ha < hb
		}
		return a.Candidate.Account.ID < b.Candidate.Account.ID
	})
}

// tieBreakHash deterministically maps (requestID, accountID) to a value used to
// rotate selection among otherwise-tied candidates.
func tieBreakHash(requestID string, accountID int) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(requestID))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(strconv.Itoa(accountID)))
	return h.Sum64()
}

func rankedCandidates(frontier []candidateScore, scores []candidateScore) []contract.Candidate {
	out := make([]contract.Candidate, 0, len(scores))
	seen := map[int]bool{}
	for _, score := range frontier {
		out = append(out, score.Candidate)
		seen[score.Candidate.Account.ID] = true
	}
	for _, score := range scores {
		if seen[score.Candidate.Account.ID] {
			continue
		}
		out = append(out, score.Candidate)
	}
	return out
}

type scoreBreakdown struct {
	AccountID          int      `json:"account_id"`
	Final              float64  `json:"final_score"`
	Health             float64  `json:"health_score"`
	Quota              float64  `json:"quota_score"`
	Latency            float64  `json:"latency_score"`
	Quality            float64  `json:"quality_score"`
	QualityEval        *float64 `json:"quality_eval_score,omitempty"`
	QualityEvalSamples int      `json:"quality_eval_samples,omitempty"`
	QualityTier        string   `json:"quality_tier,omitempty"`
	Sticky             float64  `json:"sticky_score"`
	Cache              float64  `json:"cache_score"`
	Cost               float64  `json:"cost_score"`
	Fairness           float64  `json:"fairness_score"`
	RiskPenalty        float64  `json:"risk_penalty"`
	SaturationPenalty  float64  `json:"saturation_penalty"`
}

func scoreCandidate(candidate contract.Candidate, req contract.ScheduleRequest, strategy contract.StrategyDescriptor) scoreBreakdown {
	weights := strategy.Weights
	health := healthScore(candidate)
	quota := quotaScore(candidate)
	latency := latencyScore(candidate)
	quality := qualityScore(candidate)
	qualityEval := qualityEvalScore(candidate)
	qualitySamples := qualityEvalSamples(candidate)
	qualityTier := qualityTierValue(candidate)
	sticky := stickyScore(candidate, req)
	cache := cacheScore(candidate, req, health)
	cost := costScore(candidate)
	fairness := normalizeWeight(candidate.Account.Weight)
	riskPenalty := riskPenalty(candidate)
	saturationPenalty := saturationPenalty(candidate)
	final := health*weights["health"] + quota*weights["quota"] + latency*weights["latency"] + sticky*weights["sticky"] + cache*weights["cache"] + cost*weights["cost"] + fairness*weights["fairness"] + quality*weights["quality"] - riskPenalty - saturationPenalty
	return scoreBreakdown{
		AccountID:          candidate.Account.ID,
		Final:              final,
		Health:             health,
		Quota:              quota,
		Latency:            latency,
		Quality:            quality,
		QualityEval:        qualityEval,
		QualityEvalSamples: qualitySamples,
		QualityTier:        qualityTier,
		Sticky:             sticky,
		Cache:              cache,
		Cost:               cost,
		Fairness:           fairness,
		RiskPenalty:        riskPenalty,
		SaturationPenalty:  saturationPenalty,
	}
}

// filterByPriorityTier keeps only the highest-priority (lowest Priority number)
// tier of eligible accounts so a primary pool is used exclusively until it is
// exhausted/unhealthy before any backup tier receives traffic. This makes
// Account.Priority a hard scheduling tier (sub2api filterByMinPriority parity)
// rather than a soft scoring nudge, which is what operators expect when they set
// a primary account to priority 0 and a backup to priority 100.
//
// A sticky/affinity account is always retained even when it sits in a lower
// tier, so an established session is not torn off its account just because a
// higher-priority account also exists; the soft sticky score still lets it win.
func filterByPriorityTier(candidates []contract.Candidate, stickyAccountID *int) (selected []contract.Candidate, deferred []contract.Candidate) {
	if len(candidates) <= 1 {
		return candidates, nil
	}
	minPriority := candidates[0].Account.Priority
	for _, candidate := range candidates[1:] {
		if candidate.Account.Priority < minPriority {
			minPriority = candidate.Account.Priority
		}
	}
	for _, candidate := range candidates {
		if candidate.Account.Priority == minPriority || (stickyAccountID != nil && candidate.Account.ID == *stickyAccountID) {
			selected = append(selected, candidate)
			continue
		}
		deferred = append(deferred, candidate)
	}
	return selected, deferred
}

func rejectReason(candidate contract.Candidate, req contract.ScheduleRequest) string {
	if excludedAccount(candidate.Account.ID, req.ExcludedAccountIDs) {
		return "fallback_excluded"
	}
	if req.StickyStrength == contract.StickyStrengthHard {
		if req.StickyAccountID == nil {
			return "hard_sticky_missing"
		}
		if candidate.Account.ID != *req.StickyAccountID {
			return "hard_sticky_mismatch"
		}
	}
	if candidate.Provider.Status != "active" {
		return "provider_disabled"
	}
	switch candidate.Account.Status {
	case accountcontract.StatusActive:
	case accountcontract.StatusNeedsReauth:
		return "needs_reauth"
	default:
		return "account_disabled"
	}
	if candidate.Mapping.Status != "active" {
		return "model_not_supported"
	}
	if candidate.Account.ProviderID != candidate.Provider.ID || candidate.Mapping.ProviderID != candidate.Provider.ID {
		return "model_not_supported"
	}
	if requestCapabilityMismatch(req.RequestCapabilities, candidate.EffectiveCapabilities) {
		return "capability_mismatch"
	}
	if strings.TrimSpace(candidate.Account.CredentialCiphertext) == "" {
		return "credential_invalid"
	}
	if candidate.RuntimeState.QuotaExhausted || candidate.RuntimeState.QuotaAutoPaused {
		return "quota_exhausted"
	}
	if quotaProtected(candidate, req) {
		return "quota_protected"
	}
	if candidate.RuntimeState.CircuitOpen {
		return "circuit_open"
	}
	if candidate.RuntimeState.CooldownActive {
		return "cooldown_active"
	}
	if limitReached(candidate.Limits.MaxConcurrency, candidate.RuntimeState.CurrentConcurrency) {
		return "concurrency_full"
	}
	if limitReached(candidate.Limits.RPMLimit, candidate.RuntimeState.RPMUsed) {
		return "rpm_limit_exceeded"
	}
	if limitReached(candidate.Limits.TPMLimit, candidate.RuntimeState.TPMUsed) {
		return "tpm_limit_exceeded"
	}
	if costWindowExceeded(candidate.Limits.CostWindowLimit, candidate.RuntimeState.CostWindowUsed) {
		return "cost_window_exceeded"
	}
	return ""
}

// costWindowExceeded reports whether an account has reached its rolling
// cost-window cap. A nil/non-positive limit means no cap.
func costWindowExceeded(limit *float64, used float64) bool {
	return limit != nil && *limit > 0 && used >= *limit
}

func requestCapabilityMismatch(requested []capabilitiescontract.Descriptor, effective []capabilitiescontract.Descriptor) bool {
	if len(requested) == 0 {
		return false
	}
	effectiveByKey := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range effective {
		if descriptor.Status == capabilitiescontract.DescriptorStatusDeprecated {
			continue
		}
		key := strings.TrimSpace(descriptor.Key)
		if key == "" {
			continue
		}
		effectiveByKey[key] = descriptor
	}
	for _, descriptor := range requested {
		if descriptor.Level != capabilitiescontract.DescriptorLevelRequired {
			continue
		}
		effectiveDescriptor, ok := effectiveByKey[strings.TrimSpace(descriptor.Key)]
		if !ok {
			return true
		}
		if effectiveDescriptor.Level == capabilitiescontract.DescriptorLevelUnsupported {
			return true
		}
	}
	return false
}

func rejectAllCandidates(candidates []contract.Candidate, reason string) map[string]any {
	rejectReasons := make(map[string]any, len(candidates))
	for _, candidate := range candidates {
		rejectReasons[accountKey(candidate.Account.ID)] = reason
	}
	return rejectReasons
}

func addStickyBrokenReason(rejectReasons map[string]any, req contract.ScheduleRequest) {
	if req.StickyAccountID == nil {
		return
	}
	reason, ok := rejectReasons[accountKey(*req.StickyAccountID)]
	if !ok && req.StickyStrength == contract.StickyStrengthHard {
		reason = "sticky_account_not_found"
		ok = true
	}
	if !ok {
		return
	}
	switch req.StickyStrength {
	case contract.StickyStrengthHard:
		rejectReasons["hard_sticky_unavailable"] = reason
	case contract.StickyStrengthSoft:
		rejectReasons["sticky_broken_reason"] = reason
	}
}

func healthScore(candidate contract.Candidate) float64 {
	if candidate.RuntimeState.CircuitOpen || candidate.RuntimeState.CooldownActive {
		return 0
	}
	if candidate.RuntimeState.HealthScore != nil {
		return clamp01(*candidate.RuntimeState.HealthScore)
	}
	return 0.70
}

func quotaScore(candidate contract.Candidate) float64 {
	if candidate.RuntimeState.QuotaExhausted || candidate.RuntimeState.QuotaAutoPaused {
		return 0
	}
	if candidate.RuntimeState.QuotaRemainingRatio == nil {
		return 1
	}
	ratio := clamp01(*candidate.RuntimeState.QuotaRemainingRatio)
	switch {
	case ratio >= 0.70:
		return 1.00
	case ratio >= 0.30:
		return 0.70
	case ratio >= 0.10:
		return 0.35
	case ratio > 0:
		return 0.10
	default:
		return 0
	}
}

func latencyScore(candidate contract.Candidate) float64 {
	if candidate.RuntimeState.LatencyP95MS == nil {
		return 0.60
	}
	return clamp01(1 - float64(*candidate.RuntimeState.LatencyP95MS)/10000)
}

func qualityScore(candidate contract.Candidate) float64 {
	valueMaps := qualityValueMaps(candidate)
	if score, ok := firstScoreValue(valueMaps, "quality_score", "quality_eval_score", "online_eval_score", "judge_score"); ok {
		return score
	}
	if tier, ok := firstQualityTier(valueMaps); ok {
		switch tier {
		case "premium", "high", "gold":
			return 0.90
		case "standard", "medium", "silver":
			return 0.60
		case "basic", "low", "bronze":
			return 0.35
		default:
			return 0.50
		}
	}
	return 0.50
}

func qualityEvalScore(candidate contract.Candidate) *float64 {
	valueMaps := qualityValueMaps(candidate)
	if score, ok := firstScoreValue(valueMaps, "quality_eval_score", "online_eval_score", "judge_score"); ok {
		return &score
	}
	return nil
}

func qualityEvalSamples(candidate contract.Candidate) int {
	valueMaps := qualityValueMaps(candidate)
	if count, ok := firstPositiveInt(valueMaps, "quality_eval_samples", "online_eval_samples", "judge_samples"); ok {
		return count
	}
	return 0
}

func qualityTierValue(candidate contract.Candidate) string {
	valueMaps := qualityValueMaps(candidate)
	if tier, ok := firstQualityTier(valueMaps); ok {
		return tier
	}
	return ""
}

func qualityValueMaps(candidate contract.Candidate) []map[string]any {
	modelRegistryTier := map[string]any{}
	if strings.TrimSpace(candidate.QualityTier) != "" {
		modelRegistryTier["quality_tier"] = candidate.QualityTier
	}
	return []map[string]any{candidate.Mapping.PricingOverride, candidate.Account.Metadata, candidate.Provider.Capabilities, candidate.Provider.ConfigSchema, modelRegistryTier}
}

func firstQualityTier(values []map[string]any) (string, bool) {
	for _, metadata := range values {
		for _, key := range []string{"quality_tier", "quality"} {
			value, ok := metacoerce.Value(metadata, key)
			if !ok {
				continue
			}
			parsed := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
			if parsed != "" {
				return parsed, true
			}
		}
	}
	return "", false
}

func stickyScore(candidate contract.Candidate, req contract.ScheduleRequest) float64 {
	if req.StickyAccountID == nil || candidate.Account.ID != *req.StickyAccountID {
		return 0
	}
	switch req.StickyStrength {
	case contract.StickyStrengthHard:
		return 1.0
	case contract.StickyStrengthSoft:
		return 0.7
	default:
		return 0
	}
}

func cacheScore(candidate contract.Candidate, req contract.ScheduleRequest, health float64) float64 {
	if health < 0.40 {
		return 0
	}
	valueMaps := []map[string]any{candidate.Mapping.PricingOverride, candidate.Account.Metadata, candidate.Provider.Capabilities, candidate.Provider.ConfigSchema}
	if score, ok := firstScoreValue(valueMaps, "cache_score", "cache_affinity_score", "cache_hit_rate", "prompt_cache_hit_rate", "cache_saving_ratio"); ok {
		return score
	}
	if saving, savingOK := firstPositiveFloat(valueMaps, "estimated_cache_saving", "cache_saving_estimate"); savingOK {
		if total, totalOK := firstPositiveFloat(valueMaps, "estimated_total_cost", "estimated_cost", "total_cost"); totalOK {
			return clamp01(saving / total)
		}
	}
	if cachedTokens, ok := firstPositiveFloat([]map[string]any{candidate.Mapping.PricingOverride, candidate.Account.Metadata}, "cached_token_estimate", "estimated_cached_tokens"); ok {
		totalTokens := float64(req.EstimatedInputTokens + req.EstimatedOutputTokens)
		if totalTokens <= 0 {
			totalTokens = cachedTokens
		}
		return clamp01(cachedTokens / (totalTokens + cachedTokens))
	}
	return 0
}

func quotaProtected(candidate contract.Candidate, req contract.ScheduleRequest) bool {
	if !freeTier(req.UserTier) || !protectedAccount(candidate) || candidate.RuntimeState.QuotaRemainingRatio == nil {
		return false
	}
	return *candidate.RuntimeState.QuotaRemainingRatio > 0 && *candidate.RuntimeState.QuotaRemainingRatio < 0.10
}

func freeTier(tier contract.UserTier) bool {
	return tier == "" || tier == contract.UserTierFree
}

func protectedAccount(candidate contract.Candidate) bool {
	if metacoerce.Bool(candidate.Account.Metadata, "quota_protected") || metacoerce.Bool(candidate.Account.Metadata, "protected") {
		return true
	}
	value, ok := candidate.Account.Metadata["quality_tier"].(string)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "premium", "protected":
		return true
	default:
		return false
	}
}

func limitReached(limit *int, used int) bool {
	return limit != nil && *limit >= 0 && used >= *limit
}

func saturationPenalty(candidate contract.Candidate) float64 {
	limit := candidate.Limits.MaxConcurrency
	if limit == nil || *limit <= 0 {
		return 0
	}
	ratio := float64(candidate.RuntimeState.CurrentConcurrency) / float64(*limit)
	if ratio <= 0.75 {
		return 0
	}
	if ratio >= 1 {
		return 0.25
	}
	return (ratio - 0.75)
}

func costScore(candidate contract.Candidate) float64 {
	if value, ok := candidate.Mapping.PricingOverride["relative_cost"]; ok {
		if parsed, ok := metacoerce.Float(value); ok {
			return clamp01(1 - parsed)
		}
	}
	return 0.6
}

func (s *Service) enrichFeedbackSignals(ctx context.Context, req *contract.ScheduleRequest) error {
	if req == nil || len(req.Candidates) == 0 {
		return nil
	}
	accountIDs := candidateAccountIDs(req.Candidates)
	if len(accountIDs) == 0 {
		return nil
	}
	signalRows, err := s.store.ListFeedbackSignals(ctx, contract.FeedbackSignalQuery{
		AccountIDs: accountIDs,
		Model:      strings.TrimSpace(req.Model),
		Since:      s.clock.Now().Add(-feedbackSignalWindow),
	})
	if err != nil {
		return err
	}
	signals := feedbackSignalMap(signalRows)
	if len(signals) == 0 {
		return nil
	}
	minCost, maxCost, hasCostRange := feedbackCostRange(signals)
	for idx := range req.Candidates {
		signal, ok := signals[req.Candidates[idx].Account.ID]
		if !ok {
			continue
		}
		applyFeedbackSignal(&req.Candidates[idx], signal, minCost, maxCost, hasCostRange)
	}
	return nil
}

func candidateAccountIDs(candidates []contract.Candidate) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		id := candidate.Account.ID
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func feedbackSignalMap(signals []contract.FeedbackSignal) map[int]contract.FeedbackSignal {
	out := make(map[int]contract.FeedbackSignal, len(signals))
	for _, signal := range signals {
		totalTokens := signal.InputTokens + signal.OutputTokens + signal.CachedTokens
		if signal.AccountID <= 0 || signal.SampleCount <= 0 || totalTokens <= 0 {
			continue
		}
		if signal.HasCache {
			signal.CacheHitRate = clamp01(signal.CacheHitRate)
		}
		if signal.HasCost {
			signal.CostPer1KTokens = math.Max(0, signal.CostPer1KTokens)
		}
		out[signal.AccountID] = signal
	}
	return out
}

func feedbackCostRange(signals map[int]contract.FeedbackSignal) (float64, float64, bool) {
	minCost := math.MaxFloat64
	maxCost := 0.0
	found := false
	for _, signal := range signals {
		if !signal.HasCost {
			continue
		}
		if signal.CostPer1KTokens < minCost {
			minCost = signal.CostPer1KTokens
		}
		if signal.CostPer1KTokens > maxCost {
			maxCost = signal.CostPer1KTokens
		}
		found = true
	}
	return minCost, maxCost, found
}

func applyFeedbackSignal(candidate *contract.Candidate, signal contract.FeedbackSignal, minCost float64, maxCost float64, hasCostRange bool) {
	if candidate.Mapping.PricingOverride == nil {
		candidate.Mapping.PricingOverride = map[string]any{}
	}
	metadata := candidate.Mapping.PricingOverride
	metadata["feedback_sample_count"] = signal.SampleCount
	metadata["feedback_input_tokens"] = signal.InputTokens
	metadata["feedback_output_tokens"] = signal.OutputTokens
	metadata["feedback_cached_tokens"] = signal.CachedTokens
	if signal.HasCost {
		metadata["feedback_cost_per_1k_tokens"] = signal.CostPer1KTokens
	}
	if signal.HasCache {
		metadata["feedback_cache_hit_rate"] = signal.CacheHitRate
	}
	if signal.HasCost && hasCostRange && !hasAnyMetadataKey(metadata, "relative_cost") {
		metadata["relative_cost"] = normalizedFeedbackCost(signal.CostPer1KTokens, minCost, maxCost)
	}
	if signal.HasCache && !hasAnyMetadataKey(metadata, "cache_score", "cache_affinity_score", "cache_hit_rate", "prompt_cache_hit_rate", "cache_saving_ratio") {
		metadata["cache_hit_rate"] = signal.CacheHitRate
	}
}

func normalizedFeedbackCost(cost float64, minCost float64, maxCost float64) float64 {
	if maxCost <= minCost {
		return 0.5
	}
	return clamp01((cost - minCost) / (maxCost - minCost))
}

func hasAnyMetadataKey(metadata map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := metacoerce.Value(metadata, key); ok {
			return true
		}
	}
	return false
}

func firstScoreValue(values []map[string]any, keys ...string) (float64, bool) {
	value, ok := firstFloat(values, keys...)
	if !ok {
		return 0, false
	}
	return clamp01(value), true
}

func firstFloat(values []map[string]any, keys ...string) (float64, bool) {
	for _, metadata := range values {
		for _, key := range keys {
			value, ok := metacoerce.Value(metadata, key)
			if !ok {
				continue
			}
			parsed, ok := metacoerce.Float(value)
			if ok {
				return parsed, true
			}
		}
	}
	return 0, false
}

func firstPositiveFloat(values []map[string]any, keys ...string) (float64, bool) {
	for _, metadata := range values {
		for _, key := range keys {
			value, ok := metacoerce.Value(metadata, key)
			if !ok {
				continue
			}
			parsed, ok := metacoerce.Float(value)
			if ok && parsed > 0 {
				return parsed, true
			}
		}
	}
	return 0, false
}

func firstPositiveInt(values []map[string]any, keys ...string) (int, bool) {
	for _, metadata := range values {
		for _, key := range keys {
			value, ok := metacoerce.Value(metadata, key)
			if !ok {
				continue
			}
			parsed, ok := metacoerce.Int(value)
			if ok && parsed > 0 {
				return parsed, true
			}
		}
	}
	return 0, false
}

func riskPenalty(candidate contract.Candidate) float64 {
	if candidate.Account.RiskLevel == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(*candidate.Account.RiskLevel)) {
	case "high":
		return 0.15
	case "medium":
		return 0.05
	default:
		return 0
	}
}

func excludedAccount(accountID int, excluded []int) bool {
	for _, excludedID := range excluded {
		if accountID == excludedID {
			return true
		}
	}
	return false
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func strategyWeightsPayload(strategy contract.StrategyDescriptor) map[string]any {
	weights := strategy.Weights
	out := make(map[string]any, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}

func normalizeWeight(weight float32) float64 {
	if weight <= 0 {
		return 0
	}
	if weight >= 1 {
		return 1
	}
	return float64(weight)
}

func accountKey(id int) string {
	return strings.TrimSpace("account_" + strconv.Itoa(id))
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	cloned := make([]int, len(values))
	copy(cloned, values)
	return cloned
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCapabilityDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	cloned := make([]capabilitiescontract.Descriptor, len(values))
	copy(cloned, values)
	return cloned
}

func cloneRuntimeLimits(value contract.RuntimeLimits) contract.RuntimeLimits {
	return contract.RuntimeLimits{
		MaxConcurrency:  cloneIntPtr(value.MaxConcurrency),
		RPMLimit:        cloneIntPtr(value.RPMLimit),
		TPMLimit:        cloneIntPtr(value.TPMLimit),
		CostWindowLimit: cloneFloatPtr(value.CostWindowLimit),
	}
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func removeNilValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		if value == nil {
			continue
		}
		out[key] = value
	}
	return out
}

func sanitizeSnapshotMap(values map[string]any) map[string]any {
	return sanitizeSnapshotValue(cloneMapAny(values)).(map[string]any)
}

func sanitizeSnapshotValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if sensitiveSnapshotKey(key) {
				continue
			}
			out[key] = sanitizeSnapshotValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, sanitizeSnapshotValue(child))
		}
		return out
	default:
		return value
	}
}

func sensitiveSnapshotKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(key)))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "cached_token_estimate", "estimated_cached_tokens", "cached_tokens", "input_tokens", "output_tokens", "total_tokens", "token_count", "token_budget", "estimated_input_tokens", "estimated_output_tokens":
		return false
	}
	switch normalized {
	case "api_key", "apikey", "access_token", "refresh_token", "authorization", "cookie", "secret", "password", "token", "credential", "credential_ciphertext", "cookie_jar_ciphertext", "device_fingerprint_ciphertext", "oauth_access_token", "oauth_refresh_token", "oauth_device_code", "web_session_cookie", "cli_client_token", "custom_headers", "custom_reverse_proxy_payload":
		return true
	}
	sensitiveFragments := []string{"api_key", "access_token", "refresh_token", "authorization", "cookie", "secret", "password", "credential", "ciphertext", "device_fingerprint"}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return strings.HasSuffix(normalized, "_secret") ||
		strings.HasSuffix(normalized, "_password") ||
		strings.HasSuffix(normalized, "_credential") ||
		strings.HasSuffix(normalized, "_ciphertext") ||
		strings.HasSuffix(normalized, "_cookie") ||
		strings.HasSuffix(normalized, "_token") ||
		strings.HasSuffix(normalized, "_access_token") ||
		strings.HasSuffix(normalized, "_refresh_token")
}

func scheduleAttemptNo(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func boolPtr(value bool) *bool {
	return &value
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
