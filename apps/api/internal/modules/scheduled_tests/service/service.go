package service

import (
	"context"
	"errors"
	"strconv"
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
	input.ProbeModel = strings.TrimSpace(input.ProbeModel)
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
	if input.ProbeModel != nil {
		model := strings.TrimSpace(*input.ProbeModel)
		input.ProbeModel = &model
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
// given moment. A plan that has never run is due immediately.
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
	if next, ok := cronNextAfter(strings.TrimSpace(plan.CronExpression), plan.LastRunAt.UTC()); ok {
		return &next
	}
	interval := time.Duration(normalizeInterval(plan.IntervalSeconds)) * time.Second
	next := plan.LastRunAt.UTC().Add(interval)
	return &next
}

func cronNextAfter(expression string, after time.Time) (time.Time, bool) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return time.Time{}, false
	}
	minutes, _, ok := parseCronField(fields[0], 0, 59)
	if !ok {
		return time.Time{}, false
	}
	hours, _, ok := parseCronField(fields[1], 0, 23)
	if !ok {
		return time.Time{}, false
	}
	days, dayWildcard, ok := parseCronField(fields[2], 1, 31)
	if !ok {
		return time.Time{}, false
	}
	months, _, ok := parseCronField(fields[3], 1, 12)
	if !ok {
		return time.Time{}, false
	}
	weekdays, _, ok := parseCronField(fields[4], 0, 7)
	if !ok {
		return time.Time{}, false
	}
	weekdays[0] = weekdays[0] || weekdays[7]
	weekdayWildcard := cronFieldCoversRange(weekdays, 0, 6)
	cursor := after.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := cursor.Add(366 * 24 * time.Hour)
	for !cursor.After(limit) {
		if cronTimeMatches(cursor, minutes, hours, days, months, weekdays, dayWildcard, weekdayWildcard) {
			return cursor, true
		}
		cursor = cursor.Add(time.Minute)
	}
	return time.Time{}, false
}

func cronTimeMatches(t time.Time, minutes, hours, days, months, weekdays []bool, dayWildcard, weekdayWildcard bool) bool {
	dayMatches := days[t.Day()]
	weekdayMatches := weekdays[int(t.Weekday())]
	if !dayWildcard && !weekdayWildcard {
		dayMatches = dayMatches || weekdayMatches
	} else {
		dayMatches = dayMatches && weekdayMatches
	}
	return minutes[t.Minute()] &&
		hours[t.Hour()] &&
		months[int(t.Month())] &&
		dayMatches
}

func parseCronField(field string, min int, max int) ([]bool, bool, bool) {
	out := make([]bool, max+1)
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, false
		}
		step := 1
		if strings.Contains(part, "/") {
			pieces := strings.Split(part, "/")
			if len(pieces) != 2 {
				return nil, false, false
			}
			part = pieces[0]
			parsedStep, err := strconv.Atoi(pieces[1])
			if err != nil || parsedStep <= 0 {
				return nil, false, false
			}
			step = parsedStep
		}
		start, end, ok := cronRange(part, min, max)
		if !ok {
			return nil, false, false
		}
		for value := start; value <= end; value += step {
			out[value] = true
		}
	}
	for value := min; value <= max; value++ {
		if out[value] {
			return out, cronFieldCoversRange(out, min, max), true
		}
	}
	return nil, false, false
}

func cronFieldCoversRange(values []bool, min int, max int) bool {
	for value := min; value <= max; value++ {
		if value >= len(values) || !values[value] {
			return false
		}
	}
	return true
}

func cronRange(part string, min int, max int) (int, int, bool) {
	if part == "*" {
		return min, max, true
	}
	if strings.Contains(part, "-") {
		pieces := strings.Split(part, "-")
		if len(pieces) != 2 {
			return 0, 0, false
		}
		start, err := strconv.Atoi(pieces[0])
		if err != nil {
			return 0, 0, false
		}
		end, err := strconv.Atoi(pieces[1])
		if err != nil {
			return 0, 0, false
		}
		return start, end, start >= min && end <= max && start <= end
	}
	value, err := strconv.Atoi(part)
	if err != nil || value < min || value > max {
		return 0, 0, false
	}
	return value, value, true
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
