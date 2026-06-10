package service

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/store/memory"
)

func newService(t *testing.T, now time.Time) *Service {
	t.Helper()
	svc, err := New(memory.New(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestCreatePlanValidatesScope(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, time.Now().UTC())

	if _, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "x", ScopeType: contract.ScopeGroup}); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for group scope without id, got %v", err)
	}
	if _, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "", ScopeType: contract.ScopeAll}); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for empty name, got %v", err)
	}
	plan, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "all", ScopeType: contract.ScopeAll, IntervalSeconds: 10})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if plan.IntervalSeconds != minIntervalSeconds {
		t.Fatalf("expected interval clamped to %d, got %d", minIntervalSeconds, plan.IntervalSeconds)
	}
}

func TestDuePlansSelectsPastNextRun(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	svc := newService(t, now)

	// never-run plan is due immediately.
	neverRun, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "fresh", Enabled: true, ScopeType: contract.ScopeAll, IntervalSeconds: 3600})
	if err != nil {
		t.Fatalf("create never-run: %v", err)
	}
	// due plan: last run 2h ago, interval 1h => next run 1h ago.
	due, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "due", Enabled: true, ScopeType: contract.ScopeAll, IntervalSeconds: 3600})
	if err != nil {
		t.Fatalf("create due: %v", err)
	}
	if _, err := svc.RecordOutcome(ctx, due.ID, contract.RunOutcome{
		Status:     contract.RunStatusOK,
		StartedAt:  now.Add(-2 * time.Hour),
		FinishedAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("record due outcome: %v", err)
	}
	// not-due plan: last run 10m ago, interval 1h => next run in 50m.
	notDue, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "recent", Enabled: true, ScopeType: contract.ScopeAll, IntervalSeconds: 3600})
	if err != nil {
		t.Fatalf("create not-due: %v", err)
	}
	if _, err := svc.RecordOutcome(ctx, notDue.ID, contract.RunOutcome{
		Status:     contract.RunStatusOK,
		StartedAt:  now.Add(-10 * time.Minute),
		FinishedAt: now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("record not-due outcome: %v", err)
	}
	// disabled plan that would otherwise be due.
	disabled, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "off", Enabled: false, ScopeType: contract.ScopeAll, IntervalSeconds: 3600})
	if err != nil {
		t.Fatalf("create disabled: %v", err)
	}

	duePlans, err := svc.DuePlans(ctx, now)
	if err != nil {
		t.Fatalf("due plans: %v", err)
	}
	got := map[int]bool{}
	for _, p := range duePlans {
		got[p.ID] = true
	}
	if !got[neverRun.ID] {
		t.Fatalf("expected never-run plan %d to be due", neverRun.ID)
	}
	if !got[due.ID] {
		t.Fatalf("expected past-next-run plan %d to be due", due.ID)
	}
	if got[notDue.ID] {
		t.Fatalf("expected recent plan %d NOT to be due", notDue.ID)
	}
	if got[disabled.ID] {
		t.Fatalf("expected disabled plan %d NOT to be due", disabled.ID)
	}
}

func TestNextRunAtUsesCronExpressionWhenValid(t *testing.T) {
	lastRun := time.Date(2026, 6, 5, 12, 30, 0, 0, time.UTC)
	next := NextRunAt(contract.Plan{
		LastRunAt:       &lastRun,
		IntervalSeconds: 3600,
		CronExpression:  "0 */6 * * *",
	})
	if next == nil {
		t.Fatal("expected next cron run")
	}
	want := time.Date(2026, 6, 5, 18, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected cron next run %s, got %s", want, *next)
	}
}

func TestNextRunAtUsesCronDayOfMonthOrWeekdaySemantics(t *testing.T) {
	lastRun := time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC)
	next := NextRunAt(contract.Plan{
		LastRunAt:       &lastRun,
		IntervalSeconds: 3600,
		CronExpression:  "0 9 15 * 5",
	})
	if next == nil {
		t.Fatal("expected next cron run")
	}
	want := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected weekday cron run %s, got %s", want, *next)
	}
}

func TestNextRunAtFallsBackToIntervalForInvalidCron(t *testing.T) {
	lastRun := time.Date(2026, 6, 5, 12, 30, 0, 0, time.UTC)
	next := NextRunAt(contract.Plan{
		LastRunAt:       &lastRun,
		IntervalSeconds: 3600,
		CronExpression:  "bad cron",
	})
	if next == nil {
		t.Fatal("expected fallback next run")
	}
	want := time.Date(2026, 6, 5, 13, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected interval fallback %s, got %s", want, *next)
	}
}

func TestRecordOutcomeUpdatesPlanSummary(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	svc := newService(t, now)
	plan, err := svc.CreatePlan(ctx, contract.CreatePlan{Name: "p", Enabled: true, ScopeType: contract.ScopeAll, IntervalSeconds: 600})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	run, err := svc.RecordOutcome(ctx, plan.ID, contract.RunOutcome{
		Trigger:    contract.TriggerManual,
		Status:     contract.RunStatusPartial,
		Probed:     2,
		Unhealthy:  1,
		Summary:    "probed=2 unhealthy=1",
		StartedAt:  now,
		FinishedAt: now,
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	if run.Trigger != contract.TriggerManual {
		t.Fatalf("unexpected run trigger: %s", run.Trigger)
	}
	reloaded, err := svc.FindPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("find plan: %v", err)
	}
	if reloaded.LastStatus != contract.RunStatusPartial || reloaded.LastSummary != "probed=2 unhealthy=1" {
		t.Fatalf("plan summary not updated: %+v", reloaded)
	}
	if reloaded.LastRunAt == nil || !reloaded.LastRunAt.Equal(now) {
		t.Fatalf("plan last run not updated: %+v", reloaded)
	}
}
