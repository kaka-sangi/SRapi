package scheduledtests

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entplan "github.com/srapi/srapi/apps/api/ent/scheduledtestplan"
	entrun "github.com/srapi/srapi/apps/api/ent/scheduledtestplanrun"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
)

// ErrInvalidStore is returned when the ent client is nil.
var ErrInvalidStore = errors.New("invalid scheduled tests ent store")

// Store is the Ent-backed implementation of the scheduled-test-plan store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreatePlan(ctx context.Context, input contract.CreatePlan) (contract.Plan, error) {
	now := time.Now().UTC()
	create := s.client.ScheduledTestPlan.Create().
		SetName(input.Name).
		SetEnabled(input.Enabled).
		SetScopeType(string(input.ScopeType)).
		SetIntervalSeconds(input.IntervalSeconds).
		SetCronExpression(input.CronExpression).
		SetMaxResults(input.MaxResults).
		SetAutoRecover(input.AutoRecover).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if input.ScopeID != nil {
		create.SetScopeID(*input.ScopeID)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.Plan{}, err
	}
	return toPlan(row), nil
}

func (s *Store) UpdatePlan(ctx context.Context, id int, input contract.UpdatePlan) (contract.Plan, error) {
	if id <= 0 {
		return contract.Plan{}, ErrInvalidStore
	}
	update := s.client.ScheduledTestPlan.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if input.Name != nil {
		update.SetName(*input.Name)
	}
	if input.Enabled != nil {
		update.SetEnabled(*input.Enabled)
	}
	if input.ScopeType != nil {
		update.SetScopeType(string(*input.ScopeType))
	}
	if input.ClearScopeID {
		update.ClearScopeID()
	} else if input.ScopeID != nil {
		update.SetScopeID(*input.ScopeID)
	}
	if input.IntervalSeconds != nil {
		update.SetIntervalSeconds(*input.IntervalSeconds)
	}
	if input.CronExpression != nil {
		update.SetCronExpression(*input.CronExpression)
	}
	if input.MaxResults != nil {
		update.SetMaxResults(*input.MaxResults)
	}
	if input.AutoRecover != nil {
		update.SetAutoRecover(*input.AutoRecover)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.Plan{}, mapNotFound(err)
	}
	return toPlan(row), nil
}

func (s *Store) DeletePlan(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if _, err := s.client.ScheduledTestPlanRun.Delete().Where(entrun.PlanID(id)).Exec(ctx); err != nil {
		return err
	}
	if err := s.client.ScheduledTestPlan.DeleteOneID(id).Exec(ctx); err != nil {
		return mapNotFound(err)
	}
	return nil
}

func (s *Store) FindPlanByID(ctx context.Context, id int) (contract.Plan, error) {
	if id <= 0 {
		return contract.Plan{}, ErrInvalidStore
	}
	row, err := s.client.ScheduledTestPlan.Get(ctx, id)
	if err != nil {
		return contract.Plan{}, mapNotFound(err)
	}
	return toPlan(row), nil
}

func (s *Store) ListPlans(ctx context.Context) ([]contract.Plan, error) {
	rows, err := s.client.ScheduledTestPlan.Query().
		Order(ent.Asc(entplan.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Plan, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPlan(row))
	}
	return out, nil
}

func (s *Store) MarkPlanRun(ctx context.Context, id int, ranAt time.Time, status string, summary string) (contract.Plan, error) {
	if id <= 0 {
		return contract.Plan{}, ErrInvalidStore
	}
	row, err := s.client.ScheduledTestPlan.UpdateOneID(id).
		SetLastRunAt(ranAt.UTC()).
		SetLastStatus(status).
		SetLastSummary(summary).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return contract.Plan{}, mapNotFound(err)
	}
	return toPlan(row), nil
}

func (s *Store) RecordRun(ctx context.Context, planID int, outcome contract.RunOutcome) (contract.Run, error) {
	if planID <= 0 {
		return contract.Run{}, ErrInvalidStore
	}
	row, err := s.client.ScheduledTestPlanRun.Create().
		SetPlanID(planID).
		SetTrigger(outcome.Trigger).
		SetStatus(outcome.Status).
		SetSelected(outcome.Selected).
		SetProbed(outcome.Probed).
		SetSkipped(outcome.Skipped).
		SetFailed(outcome.Failed).
		SetUnhealthy(outcome.Unhealthy).
		SetRecovered(outcome.Recovered).
		SetSummary(outcome.Summary).
		SetStartedAt(outcome.StartedAt.UTC()).
		SetFinishedAt(outcome.FinishedAt.UTC()).
		Save(ctx)
	if err != nil {
		return contract.Run{}, err
	}
	return toRun(row), nil
}

func (s *Store) ListRunsByPlan(ctx context.Context, planID int, limit int) ([]contract.Run, error) {
	if planID <= 0 {
		return nil, ErrInvalidStore
	}
	query := s.client.ScheduledTestPlanRun.Query().
		Where(entrun.PlanID(planID)).
		Order(ent.Desc(entrun.FieldStartedAt), ent.Desc(entrun.FieldID))
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRun(row))
	}
	return out, nil
}

func toPlan(row *ent.ScheduledTestPlan) contract.Plan {
	plan := contract.Plan{
		ID:              row.ID,
		Name:            row.Name,
		Enabled:         row.Enabled,
		ScopeType:       contract.ScopeType(row.ScopeType),
		IntervalSeconds: row.IntervalSeconds,
		CronExpression:  row.CronExpression,
		MaxResults:      row.MaxResults,
		AutoRecover:     row.AutoRecover,
		LastStatus:      row.LastStatus,
		LastSummary:     row.LastSummary,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if row.ScopeID != nil {
		scopeID := *row.ScopeID
		plan.ScopeID = &scopeID
	}
	if row.LastRunAt != nil {
		lastRun := *row.LastRunAt
		plan.LastRunAt = &lastRun
	}
	return plan
}

func toRun(row *ent.ScheduledTestPlanRun) contract.Run {
	return contract.Run{
		ID:         row.ID,
		PlanID:     row.PlanID,
		Trigger:    row.Trigger,
		Status:     row.Status,
		Selected:   row.Selected,
		Probed:     row.Probed,
		Skipped:    row.Skipped,
		Failed:     row.Failed,
		Unhealthy:  row.Unhealthy,
		Recovered:  row.Recovered,
		Summary:    row.Summary,
		StartedAt:  row.StartedAt,
		FinishedAt: row.FinishedAt,
	}
}

func mapNotFound(err error) error {
	if ent.IsNotFound(err) {
		return contract.ErrNotFound
	}
	return err
}
