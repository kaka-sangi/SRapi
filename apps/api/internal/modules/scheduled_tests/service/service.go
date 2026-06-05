package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
)

// ErrInvalidInput is returned for malformed plans.
var ErrInvalidInput = errors.New("invalid scheduled test plan")

const (
	minIntervalSeconds = 60
	defaultRunHistory  = 50
	maxRunHistory      = 200
)

// Clock provides the current time; defaults to time.Now.
type Clock func() time.Time

// Service exposes scheduled-test-plan CRUD, due-plan selection, and run history.
type Service struct {
	store contract.Store
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) now() time.Time { return s.clock().UTC() }

// ListPlans returns all plans ordered by id.
func (s *Service) ListPlans(ctx context.Context) ([]contract.Plan, error) {
	return s.store.ListPlans(ctx)
}

// FindPlan returns a single plan.
func (s *Service) FindPlan(ctx context.Context, id int) (contract.Plan, error) {
	if id <= 0 {
		return contract.Plan{}, ErrInvalidInput
	}
	return s.store.FindPlanByID(ctx, id)
}

// CreatePlan validates and persists a new plan.
func (s *Service) CreatePlan(ctx context.Context, input contract.CreatePlan) (contract.Plan, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Plan{}, ErrInvalidInput
	}
	scope := normalizeScope(input.ScopeType)
	if scope == "" {
		return contract.Plan{}, ErrInvalidInput
	}
	input.ScopeType = scope
	if scope == contract.ScopeAll {
		input.ScopeID = nil
	} else if input.ScopeID == nil || *input.ScopeID <= 0 {
		return contract.Plan{}, ErrInvalidInput
	}
	input.CronExpression = strings.TrimSpace(input.CronExpression)
	input.IntervalSeconds = normalizeInterval(input.IntervalSeconds)
	if input.MaxResults < 0 {
		input.MaxResults = 0
	}
	return s.store.CreatePlan(ctx, input)
}

// UpdatePlan validates and applies a partial update.
func (s *Service) UpdatePlan(ctx context.Context, id int, input contract.UpdatePlan) (contract.Plan, error) {
	if id <= 0 {
		return contract.Plan{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Plan{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.ScopeType != nil {
		scope := normalizeScope(*input.ScopeType)
		if scope == "" {
			return contract.Plan{}, ErrInvalidInput
		}
		input.ScopeType = &scope
		if scope == contract.ScopeAll {
			input.ScopeID = nil
			input.ClearScopeID = true
		}
	}
	if input.ScopeID != nil && *input.ScopeID <= 0 {
		return contract.Plan{}, ErrInvalidInput
	}
	if input.CronExpression != nil {
		cron := strings.TrimSpace(*input.CronExpression)
		input.CronExpression = &cron
	}
	if input.IntervalSeconds != nil {
		interval := normalizeInterval(*input.IntervalSeconds)
		input.IntervalSeconds = &interval
	}
	if input.MaxResults != nil && *input.MaxResults < 0 {
		zero := 0
		input.MaxResults = &zero
	}
	return s.store.UpdatePlan(ctx, id, input)
}

// DeletePlan removes a plan.
func (s *Service) DeletePlan(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeletePlan(ctx, id)
}

// ListRuns returns recent runs for a plan.
func (s *Service) ListRuns(ctx context.Context, planID int, limit int) ([]contract.Run, error) {
	if planID <= 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = defaultRunHistory
	}
	if limit > maxRunHistory {
		limit = maxRunHistory
	}
	return s.store.ListRunsByPlan(ctx, planID, limit)
}

// DuePlans returns enabled plans whose next scheduled run has passed at the
// given moment. A plan that has never run is due immediately. The next run is
// last_run_at + interval (cron is treated as an interval hint for now).
func (s *Service) DuePlans(ctx context.Context, at time.Time) ([]contract.Plan, error) {
	plans, err := s.store.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	at = at.UTC()
	due := make([]contract.Plan, 0, len(plans))
	for _, plan := range plans {
		if !plan.Enabled {
			continue
		}
		if PlanDue(plan, at) {
			due = append(due, plan)
		}
	}
	return due, nil
}

// PlanDue reports whether a plan should run at the given moment.
func PlanDue(plan contract.Plan, at time.Time) bool {
	if !plan.Enabled {
		return false
	}
	next := NextRunAt(plan)
	if next == nil {
		return true
	}
	return !at.Before(*next)
}

// NextRunAt returns the next scheduled run time, or nil when the plan has never
// run (and is therefore due immediately).
func NextRunAt(plan contract.Plan) *time.Time {
	if plan.LastRunAt == nil {
		return nil
	}
	interval := time.Duration(normalizeInterval(plan.IntervalSeconds)) * time.Second
	next := plan.LastRunAt.UTC().Add(interval)
	return &next
}

// RecordOutcome persists a run and updates the plan's last-run summary.
func (s *Service) RecordOutcome(ctx context.Context, planID int, outcome contract.RunOutcome) (contract.Run, error) {
	if planID <= 0 {
		return contract.Run{}, ErrInvalidInput
	}
	if outcome.Trigger == "" {
		outcome.Trigger = contract.TriggerSchedule
	}
	if outcome.Status == "" {
		outcome.Status = contract.RunStatusOK
	}
	if outcome.StartedAt.IsZero() {
		outcome.StartedAt = s.now()
	}
	if outcome.FinishedAt.IsZero() {
		outcome.FinishedAt = s.now()
	}
	run, err := s.store.RecordRun(ctx, planID, outcome)
	if err != nil {
		return contract.Run{}, err
	}
	if _, err := s.store.MarkPlanRun(ctx, planID, outcome.FinishedAt, outcome.Status, outcome.Summary); err != nil {
		return contract.Run{}, err
	}
	return run, nil
}

func normalizeScope(scope contract.ScopeType) contract.ScopeType {
	switch contract.ScopeType(strings.ToLower(strings.TrimSpace(string(scope)))) {
	case contract.ScopeAll:
		return contract.ScopeAll
	case contract.ScopeAccount:
		return contract.ScopeAccount
	case contract.ScopeGroup:
		return contract.ScopeGroup
	default:
		return ""
	}
}

func normalizeInterval(seconds int) int {
	if seconds < minIntervalSeconds {
		return minIntervalSeconds
	}
	return seconds
}
