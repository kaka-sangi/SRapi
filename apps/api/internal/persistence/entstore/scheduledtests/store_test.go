package scheduledtests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"

	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:"+filepath.Join(t.TempDir(), "scheduled.db")+"?_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestStorePlanCRUDAndRuns(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	scopeID := 7

	created, err := store.CreatePlan(ctx, contract.CreatePlan{
		Name:            "nightly all",
		Enabled:         true,
		ScopeType:       contract.ScopeGroup,
		ScopeID:         &scopeID,
		IntervalSeconds: 3600,
		MaxResults:      5,
		AutoRecover:     true,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if created.ID == 0 || created.ScopeID == nil || *created.ScopeID != scopeID {
		t.Fatalf("unexpected created plan: %+v", created)
	}

	found, err := store.FindPlanByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find plan: %v", err)
	}
	if found.Name != "nightly all" || !found.AutoRecover {
		t.Fatalf("unexpected found plan: %+v", found)
	}

	disabled := false
	newScope := contract.ScopeAll
	updated, err := store.UpdatePlan(ctx, created.ID, contract.UpdatePlan{
		Enabled:      &disabled,
		ScopeType:    &newScope,
		ClearScopeID: true,
	})
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}
	if updated.Enabled || updated.ScopeType != contract.ScopeAll || updated.ScopeID != nil {
		t.Fatalf("unexpected updated plan: %+v", updated)
	}

	ranAt := time.Date(2026, 6, 5, 1, 0, 0, 0, time.UTC)
	if _, err := store.MarkPlanRun(ctx, created.ID, ranAt, contract.RunStatusOK, "probed=3"); err != nil {
		t.Fatalf("mark run: %v", err)
	}
	reloaded, err := store.FindPlanByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloaded.LastRunAt == nil || !reloaded.LastRunAt.Equal(ranAt) || reloaded.LastStatus != contract.RunStatusOK {
		t.Fatalf("unexpected last run state: %+v", reloaded)
	}

	for i := 0; i < 3; i++ {
		_, err := store.RecordRun(ctx, created.ID, contract.RunOutcome{
			Trigger:    contract.TriggerSchedule,
			Status:     contract.RunStatusOK,
			Probed:     i + 1,
			StartedAt:  ranAt.Add(time.Duration(i) * time.Minute),
			FinishedAt: ranAt.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("record run %d: %v", i, err)
		}
	}
	runs, err := store.ListRunsByPlan(ctx, created.ID, 2)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].Probed != 3 {
		t.Fatalf("expected newest run first (probed=3), got %d", runs[0].Probed)
	}
}

func TestStoreDeleteRemovesPlanAndRuns(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	plan, err := store.CreatePlan(ctx, contract.CreatePlan{Name: "temp", Enabled: true, ScopeType: contract.ScopeAll, IntervalSeconds: 600})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.RecordRun(ctx, plan.ID, contract.RunOutcome{Status: contract.RunStatusOK, StartedAt: now, FinishedAt: now}); err != nil {
		t.Fatalf("record run: %v", err)
	}
	if err := store.DeletePlan(ctx, plan.ID); err != nil {
		t.Fatalf("delete plan: %v", err)
	}
	if _, err := store.FindPlanByID(ctx, plan.ID); err != contract.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	runs, err := store.ListRunsByPlan(ctx, plan.ID, 0)
	if err != nil {
		t.Fatalf("list runs after delete: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no runs after delete, got %d", len(runs))
	}
}
