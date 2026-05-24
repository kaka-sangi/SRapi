package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entschedulerdecision "github.com/srapi/srapi/apps/api/ent/schedulerdecision"
	entschedulerfeedback "github.com/srapi/srapi/apps/api/ent/schedulerfeedback"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

var ErrInvalidStore = errors.New("invalid scheduler ent store")

type Store struct {
	client         *ent.Client
	mu             sync.Mutex
	leases         map[string]contract.Lease
	leaseByRequest map[string]string
	leaseStore     LeaseStore
}

func New(client *ent.Client) (*Store, error) {
	return NewWithLeaseStore(client, nil)
}

type LeaseStore interface {
	AcquireLease(ctx context.Context, input contract.Lease, maxConcurrency *int) (contract.Lease, error)
	UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status contract.LeaseStatus) (contract.Lease, error)
	ListLeases(ctx context.Context) ([]contract.Lease, error)
}

func NewWithLeaseStore(client *ent.Client, leaseStore LeaseStore) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{
		client:         client,
		leases:         map[string]contract.Lease{},
		leaseByRequest: map[string]string{},
		leaseStore:     leaseStore,
	}, nil
}

func (s *Store) CreateDecision(ctx context.Context, input contract.Decision) (contract.Decision, error) {
	create := s.client.SchedulerDecision.Create().
		SetRequestID(input.RequestID).
		SetAttemptNo(input.AttemptNo).
		SetUserID(input.UserID).
		SetAPIKeyID(input.APIKeyID).
		SetSourceProtocol(input.SourceProtocol).
		SetSourceEndpoint(input.SourceEndpoint).
		SetTargetProtocol(input.TargetProtocol).
		SetModel(input.Model).
		SetStrategy(string(input.Strategy)).
		SetStrategyVersion(input.StrategyVersion).
		SetStrategyConfigHash(input.StrategyConfigHash).
		SetNillableFallbackFromDecisionID(input.FallbackFromDecisionID).
		SetNillableSelectedProviderID(input.SelectedProviderID).
		SetNillableSelectedAccountID(input.SelectedAccountID).
		SetCandidateCount(input.CandidateCount).
		SetRejectedCount(input.RejectedCount).
		SetScoresJSON(cloneMap(input.Scores)).
		SetRejectReasonsJSON(cloneMap(input.RejectReasons)).
		SetStrategyWeightsJSON(cloneMap(input.StrategyWeights)).
		SetCompatibilityWarningsJSON(cloneStrings(input.CompatibilityWarnings)).
		SetStickyHit(input.StickyHit).
		SetCacheAffinityHit(input.CacheAffinityHit).
		SetEstimatedCost(input.EstimatedCost).
		SetCurrency(input.Currency)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Decision{}, err
	}
	return toDecision(created), nil
}

func (s *Store) ListDecisions(ctx context.Context) ([]contract.Decision, error) {
	rows, err := s.client.SchedulerDecision.Query().
		Order(entschedulerdecision.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Decision, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDecision(row))
	}
	return out, nil
}

func (s *Store) CreateFeedback(ctx context.Context, input contract.Feedback) (contract.Feedback, error) {
	create := s.client.SchedulerFeedback.Create().
		SetRequestID(input.RequestID).
		SetDecisionID(input.DecisionID).
		SetAttemptNo(input.AttemptNo).
		SetAccountID(input.AccountID).
		SetProviderID(input.ProviderID).
		SetModel(input.Model).
		SetSuccess(input.Success).
		SetNillableErrorClass(input.ErrorClass).
		SetNillableStatusCode(input.StatusCode).
		SetLatencyMs(input.LatencyMS).
		SetInputTokens(input.InputTokens).
		SetOutputTokens(input.OutputTokens).
		SetCachedTokens(input.CachedTokens).
		SetActualCost(input.ActualCost).
		SetCurrency(input.Currency)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Feedback{}, err
	}
	return toFeedback(created), nil
}

func (s *Store) ListFeedbacks(ctx context.Context) ([]contract.Feedback, error) {
	rows, err := s.client.SchedulerFeedback.Query().
		Order(entschedulerfeedback.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Feedback, 0, len(rows))
	for _, row := range rows {
		out = append(out, toFeedback(row))
	}
	return out, nil
}

func (s *Store) AcquireLease(ctx context.Context, input contract.Lease, maxConcurrency *int) (contract.Lease, error) {
	if s.leaseStore != nil {
		return s.leaseStore.AcquireLease(ctx, input, maxConcurrency)
	}
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
	return lease, nil
}

func (s *Store) UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status contract.LeaseStatus) (contract.Lease, error) {
	if s.leaseStore != nil {
		return s.leaseStore.UpdateLeaseStatus(ctx, requestID, attemptNo, status)
	}
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
	return lease, nil
}

func (s *Store) ListLeases(ctx context.Context) ([]contract.Lease, error) {
	if s.leaseStore != nil {
		return s.leaseStore.ListLeases(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLeases(time.Now().UTC())
	out := make([]contract.Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		out = append(out, lease)
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

func toDecision(row *ent.SchedulerDecision) contract.Decision {
	return contract.Decision{
		ID:                     row.ID,
		RequestID:              row.RequestID,
		AttemptNo:              row.AttemptNo,
		UserID:                 row.UserID,
		APIKeyID:               row.APIKeyID,
		SourceProtocol:         row.SourceProtocol,
		SourceEndpoint:         row.SourceEndpoint,
		TargetProtocol:         row.TargetProtocol,
		Model:                  row.Model,
		Strategy:               contract.StrategyName(row.Strategy),
		StrategyVersion:        row.StrategyVersion,
		StrategyConfigHash:     row.StrategyConfigHash,
		FallbackFromDecisionID: cloneInt(row.FallbackFromDecisionID),
		SelectedProviderID:     cloneInt(row.SelectedProviderID),
		SelectedAccountID:      cloneInt(row.SelectedAccountID),
		CandidateCount:         row.CandidateCount,
		RejectedCount:          row.RejectedCount,
		Scores:                 cloneMap(row.ScoresJSON),
		RejectReasons:          cloneMap(row.RejectReasonsJSON),
		StrategyWeights:        cloneMap(row.StrategyWeightsJSON),
		CompatibilityWarnings:  cloneStrings(row.CompatibilityWarningsJSON),
		StickyHit:              row.StickyHit,
		CacheAffinityHit:       row.CacheAffinityHit,
		EstimatedCost:          row.EstimatedCost,
		Currency:               row.Currency,
		CreatedAt:              row.CreatedAt,
	}
}

func toFeedback(row *ent.SchedulerFeedback) contract.Feedback {
	return contract.Feedback{
		ID:           row.ID,
		RequestID:    row.RequestID,
		DecisionID:   row.DecisionID,
		AttemptNo:    row.AttemptNo,
		AccountID:    row.AccountID,
		ProviderID:   row.ProviderID,
		Model:        row.Model,
		Success:      row.Success,
		ErrorClass:   cloneString(row.ErrorClass),
		StatusCode:   cloneInt(row.StatusCode),
		LatencyMS:    row.LatencyMs,
		InputTokens:  row.InputTokens,
		OutputTokens: row.OutputTokens,
		CachedTokens: row.CachedTokens,
		ActualCost:   row.ActualCost,
		Currency:     row.Currency,
		CreatedAt:    row.CreatedAt,
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}
