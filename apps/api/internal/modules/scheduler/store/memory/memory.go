package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

type Store struct {
	mu             sync.Mutex
	nextDecisionID int
	nextSnapshotID int
	nextFeedbackID int
	decisions      map[int]contract.Decision
	snapshots      map[int]contract.RequestSnapshot
	feedbacks      map[int]contract.Feedback
	leases         map[string]contract.Lease
	leaseByRequest map[string]string
}

func New() *Store {
	return &Store{
		nextDecisionID: 1,
		nextSnapshotID: 1,
		nextFeedbackID: 1,
		decisions:      map[int]contract.Decision{},
		snapshots:      map[int]contract.RequestSnapshot{},
		feedbacks:      map[int]contract.Feedback{},
		leases:         map[string]contract.Lease{},
		leaseByRequest: map[string]string{},
	}
}

func (s *Store) CreateDecision(_ context.Context, input contract.Decision) (contract.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decision := s.createDecisionLocked(input)
	return cloneDecision(decision), nil
}

func (s *Store) CreateDecisionWithSnapshot(_ context.Context, input contract.Decision, snapshot contract.RequestSnapshot) (contract.Decision, contract.RequestSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decision := s.createDecisionLocked(input)
	createdSnapshot := s.createSnapshotLocked(decision, snapshot)
	return cloneDecision(decision), cloneSnapshot(createdSnapshot), nil
}

func (s *Store) createDecisionLocked(input contract.Decision) contract.Decision {
	decision := cloneDecision(input)
	decision.ID = s.nextDecisionID
	if decision.AttemptNo == 0 {
		decision.AttemptNo = 1
	}
	if decision.CreatedAt.IsZero() {
		decision.CreatedAt = time.Now().UTC()
	}
	s.decisions[decision.ID] = decision
	s.nextDecisionID++
	return decision
}

func (s *Store) createSnapshotLocked(decision contract.Decision, input contract.RequestSnapshot) contract.RequestSnapshot {
	snapshot := cloneSnapshot(input)
	snapshot.ID = s.nextSnapshotID
	snapshot.DecisionID = decision.ID
	snapshot.RequestID = decision.RequestID
	snapshot.AttemptNo = decision.AttemptNo
	snapshot.SelectedAccountID = cloneInt(decision.SelectedAccountID)
	snapshot.SelectedProviderID = cloneInt(decision.SelectedProviderID)
	snapshot.Strategy = decision.Strategy
	snapshot.StrategyVersion = decision.StrategyVersion
	snapshot.StrategyConfigHash = decision.StrategyConfigHash
	snapshot.StrategyWeights = cloneMap(decision.StrategyWeights)
	snapshot.CompatibilityWarnings = cloneStrings(decision.CompatibilityWarnings)
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = decision.CreatedAt
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	s.snapshots[snapshot.ID] = snapshot
	s.nextSnapshotID++
	return snapshot
}

func (s *Store) ListDecisions(_ context.Context) ([]contract.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Decision, 0, len(s.decisions))
	for _, decision := range s.decisions {
		out = append(out, cloneDecision(decision))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListRequestSnapshots(_ context.Context) ([]contract.RequestSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.RequestSnapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		out = append(out, cloneSnapshot(snapshot))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) CreateFeedback(_ context.Context, input contract.Feedback) (contract.Feedback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	feedback := input
	feedback.ID = s.nextFeedbackID
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = time.Now().UTC()
	}
	s.feedbacks[feedback.ID] = feedback
	s.nextFeedbackID++
	return feedback, nil
}

func (s *Store) ListFeedbacks(_ context.Context) ([]contract.Feedback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Feedback, 0, len(s.feedbacks))
	for _, feedback := range s.feedbacks {
		out = append(out, feedback)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListFeedbackSignals(_ context.Context, query contract.FeedbackSignalQuery) ([]contract.FeedbackSignal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(query.AccountIDs) == 0 {
		return []contract.FeedbackSignal{}, nil
	}
	accountIDs := intSet(query.AccountIDs)
	accumulators := map[int]*feedbackSignalAccumulator{}
	for _, feedback := range s.feedbacks {
		if !feedback.Success {
			continue
		}
		if len(accountIDs) > 0 && !accountIDs[feedback.AccountID] {
			continue
		}
		if query.Model != "" && feedback.Model != query.Model {
			continue
		}
		if !query.Since.IsZero() && feedback.CreatedAt.Before(query.Since) {
			continue
		}
		totalTokens := positiveInt(feedback.InputTokens) + positiveInt(feedback.OutputTokens) + positiveInt(feedback.CachedTokens)
		if totalTokens <= 0 {
			continue
		}
		accumulator := accumulators[feedback.AccountID]
		if accumulator == nil {
			accumulator = &feedbackSignalAccumulator{}
			accumulators[feedback.AccountID] = accumulator
		}
		accumulator.sampleCount++
		accumulator.inputTokens += positiveInt(feedback.InputTokens)
		accumulator.outputTokens += positiveInt(feedback.OutputTokens)
		accumulator.cachedTokens += positiveInt(feedback.CachedTokens)
		if cost, ok := positiveCost(feedback.ActualCost); ok {
			accumulator.totalCost += cost
			accumulator.hasCost = true
		}
	}
	out := make([]contract.FeedbackSignal, 0, len(accumulators))
	for accountID, accumulator := range accumulators {
		totalTokens := accumulator.inputTokens + accumulator.outputTokens + accumulator.cachedTokens
		if totalTokens <= 0 {
			continue
		}
		signal := contract.FeedbackSignal{
			AccountID:    accountID,
			SampleCount:  accumulator.sampleCount,
			InputTokens:  accumulator.inputTokens,
			OutputTokens: accumulator.outputTokens,
			CachedTokens: accumulator.cachedTokens,
		}
		if accumulator.hasCost {
			signal.CostPer1KTokens = accumulator.totalCost / float64(totalTokens) * 1000
			signal.HasCost = true
		}
		cacheBasis := accumulator.inputTokens + accumulator.cachedTokens
		if cacheBasis <= 0 {
			cacheBasis = totalTokens
		}
		if cacheBasis > 0 {
			signal.CacheHitRate = clamp01(float64(accumulator.cachedTokens) / float64(cacheBasis))
			signal.HasCache = true
		}
		out = append(out, signal)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}

func (s *Store) ListActiveStrategies(_ context.Context) ([]contract.StrategyDescriptor, error) {
	return nil, nil
}

func (s *Store) AcquireLease(_ context.Context, input contract.Lease, maxConcurrency *int) (contract.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.expireLeases(now)
	if input.ID == "" || input.RequestID == "" || input.AccountID <= 0 {
		return contract.Lease{}, errors.New("invalid lease")
	}
	if input.AttemptNo <= 0 {
		input.AttemptNo = 1
	}
	if maxConcurrency != nil && *maxConcurrency >= 0 && s.pendingConcurrency(input.AccountID) >= *maxConcurrency {
		return contract.Lease{}, errors.New("concurrency full")
	}
	lease := input
	lease.Status = contract.LeaseStatusPending
	if lease.CreatedAt.IsZero() {
		lease.CreatedAt = now
	}
	if lease.UpdatedAt.IsZero() {
		lease.UpdatedAt = lease.CreatedAt
	}
	if lease.ExpiresAt.IsZero() {
		lease.ExpiresAt = now.Add(30 * time.Second)
	}
	s.leases[lease.ID] = lease
	s.leaseByRequest[leaseRequestKey(lease.RequestID, lease.AttemptNo)] = lease.ID
	return cloneLease(lease), nil
}

func (s *Store) UpdateLeaseStatus(_ context.Context, requestID string, attemptNo int, status contract.LeaseStatus) (contract.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLeases(time.Now().UTC())
	leaseID, ok := s.leaseByRequest[leaseRequestKey(requestID, attemptNo)]
	if !ok {
		return contract.Lease{}, errors.New("lease not found")
	}
	lease, ok := s.leases[leaseID]
	if !ok {
		return contract.Lease{}, errors.New("lease not found")
	}
	if lease.Status == contract.LeaseStatusPending {
		lease.Status = status
		lease.UpdatedAt = time.Now().UTC()
		s.leases[lease.ID] = lease
	}
	return cloneLease(lease), nil
}

func (s *Store) ListLeases(_ context.Context) ([]contract.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLeases(time.Now().UTC())
	out := make([]contract.Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		out = append(out, cloneLease(lease))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) pendingConcurrency(accountID int) int {
	count := 0
	for _, lease := range s.leases {
		if lease.AccountID == accountID && lease.Status == contract.LeaseStatusPending {
			count++
		}
	}
	return count
}

func (s *Store) expireLeases(now time.Time) {
	for id, lease := range s.leases {
		if lease.Status == contract.LeaseStatusPending && !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(now) {
			lease.Status = contract.LeaseStatusExpired
			lease.UpdatedAt = now
			s.leases[id] = lease
		}
	}
}

func leaseRequestKey(requestID string, attemptNo int) string {
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return requestID + ":" + strconv.Itoa(attemptNo)
}

func cloneDecision(value contract.Decision) contract.Decision {
	value.Scores = cloneMap(value.Scores)
	value.RejectReasons = cloneMap(value.RejectReasons)
	value.StrategyWeights = cloneMap(value.StrategyWeights)
	value.CompatibilityWarnings = cloneStrings(value.CompatibilityWarnings)
	return value
}

func cloneSnapshot(value contract.RequestSnapshot) contract.RequestSnapshot {
	raw, err := json.Marshal(value)
	if err != nil {
		return contract.RequestSnapshot{}
	}
	var cloned contract.RequestSnapshot
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return contract.RequestSnapshot{}
	}
	return cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneLease(value contract.Lease) contract.Lease {
	return value
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func intSet(values []int) map[int]bool {
	out := make(map[int]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

type feedbackSignalAccumulator struct {
	sampleCount  int
	inputTokens  int
	outputTokens int
	cachedTokens int
	totalCost    float64
	hasCost      bool
}

func positiveInt(value int) int {
	if value <= 0 {
		return 0
	}
	return value
}

func positiveCost(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
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
