package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

type Store struct {
	mu             sync.Mutex
	nextDecisionID int
	nextFeedbackID int
	decisions      map[int]contract.Decision
	feedbacks      map[int]contract.Feedback
	leases         map[string]contract.Lease
	leaseByRequest map[string]string
}

func New() *Store {
	return &Store{
		nextDecisionID: 1,
		nextFeedbackID: 1,
		decisions:      map[int]contract.Decision{},
		feedbacks:      map[int]contract.Feedback{},
		leases:         map[string]contract.Lease{},
		leaseByRequest: map[string]string{},
	}
}

func (s *Store) CreateDecision(_ context.Context, input contract.Decision) (contract.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return cloneDecision(decision), nil
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
