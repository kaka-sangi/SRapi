package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
)

// Store is an in-memory implementation of the scheduled-test-plan store.
type Store struct {
	mu      sync.Mutex
	plans   map[int]contract.Plan
	runs    map[int][]contract.Run
	planSeq int
	runSeq  int
	clock   func() time.Time
}

func New() *Store {
	return &Store{
		plans: make(map[int]contract.Plan),
		runs:  make(map[int][]contract.Run),
		clock: time.Now,
	}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) CreatePlan(ctx context.Context, input contract.CreatePlan) (contract.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planSeq++
	now := s.now()
	plan := contract.Plan{
		ID:              s.planSeq,
		Name:            input.Name,
		Enabled:         input.Enabled,
		ScopeType:       input.ScopeType,
		ScopeID:         cloneIntPtr(input.ScopeID),
		IntervalSeconds: input.IntervalSeconds,
		CronExpression:  input.CronExpression,
		MaxResults:      input.MaxResults,
		AutoRecover:     input.AutoRecover,
		LastStatus:      "",
		LastSummary:     "",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.plans[plan.ID] = plan
	return plan, nil
}

func (s *Store) UpdatePlan(ctx context.Context, id int, input contract.UpdatePlan) (contract.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.plans[id]
	if !ok {
		return contract.Plan{}, contract.ErrNotFound
	}
	if input.Name != nil {
		plan.Name = *input.Name
	}
	if input.Enabled != nil {
		plan.Enabled = *input.Enabled
	}
	if input.ScopeType != nil {
		plan.ScopeType = *input.ScopeType
	}
	if input.ClearScopeID {
		plan.ScopeID = nil
	} else if input.ScopeID != nil {
		plan.ScopeID = cloneIntPtr(input.ScopeID)
	}
	if input.IntervalSeconds != nil {
		plan.IntervalSeconds = *input.IntervalSeconds
	}
	if input.CronExpression != nil {
		plan.CronExpression = *input.CronExpression
	}
	if input.MaxResults != nil {
		plan.MaxResults = *input.MaxResults
	}
	if input.AutoRecover != nil {
		plan.AutoRecover = *input.AutoRecover
	}
	plan.UpdatedAt = s.now()
	s.plans[id] = plan
	return plan, nil
}

func (s *Store) DeletePlan(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.plans, id)
	delete(s.runs, id)
	return nil
}

func (s *Store) FindPlanByID(ctx context.Context, id int) (contract.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.plans[id]
	if !ok {
		return contract.Plan{}, contract.ErrNotFound
	}
	return plan, nil
}

func (s *Store) ListPlans(ctx context.Context) ([]contract.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Plan, 0, len(s.plans))
	for _, plan := range s.plans {
		out = append(out, plan)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) MarkPlanRun(ctx context.Context, id int, ranAt time.Time, status string, summary string) (contract.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.plans[id]
	if !ok {
		return contract.Plan{}, contract.ErrNotFound
	}
	at := ranAt.UTC()
	plan.LastRunAt = &at
	plan.LastStatus = status
	plan.LastSummary = summary
	plan.UpdatedAt = s.now()
	s.plans[id] = plan
	return plan, nil
}

func (s *Store) RecordRun(ctx context.Context, planID int, outcome contract.RunOutcome) (contract.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[planID]; !ok {
		return contract.Run{}, contract.ErrNotFound
	}
	s.runSeq++
	run := contract.Run{
		ID:         s.runSeq,
		PlanID:     planID,
		Trigger:    outcome.Trigger,
		Status:     outcome.Status,
		Selected:   outcome.Selected,
		Probed:     outcome.Probed,
		Skipped:    outcome.Skipped,
		Failed:     outcome.Failed,
		Unhealthy:  outcome.Unhealthy,
		Recovered:  outcome.Recovered,
		Summary:    outcome.Summary,
		StartedAt:  outcome.StartedAt.UTC(),
		FinishedAt: outcome.FinishedAt.UTC(),
	}
	s.runs[planID] = append(s.runs[planID], run)
	return run, nil
}

func (s *Store) ListRunsByPlan(ctx context.Context, planID int, limit int) ([]contract.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := s.runs[planID]
	out := make([]contract.Run, len(runs))
	copy(out, runs)
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.After(out[j].StartedAt)
		}
		return out[i].ID > out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}
