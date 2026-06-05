package service

import (
	"context"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/store/memory"
)

func newService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(memory.New())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestCreateDefinitionNormalizesAndValidates(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "  ", Scope: contract.ScopeAccount}); err == nil {
		t.Fatal("expected blank name to be rejected")
	}
	if _, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "x", Scope: "bogus"}); err == nil {
		t.Fatal("expected invalid scope to be rejected")
	}
	if _, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "x", Scope: contract.ScopeAccount, Request: contract.CustomRequest{Body: "{not json"}}); err == nil {
		t.Fatal("expected invalid json body to be rejected")
	}

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{
		Name:     "monitor",
		Scope:    contract.ScopeAccount,
		Interval: 5, // below minimum, clamps to 30
		Request: contract.CustomRequest{
			Method:              "post",
			ExpectedStatusCodes: []int{200, 200, 99, 401},
			Headers:             map[string]string{" X ": " v ", "empty": ""},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.Interval != 30 {
		t.Fatalf("expected interval clamp to 30, got %d", def.Interval)
	}
	if def.Request.Method != "POST" {
		t.Fatalf("expected method uppercased, got %q", def.Request.Method)
	}
	if len(def.Request.ExpectedStatusCodes) != 2 {
		t.Fatalf("expected deduped/filtered status codes, got %v", def.Request.ExpectedStatusCodes)
	}
	if def.Request.Headers["X"] != "v" || len(def.Request.Headers) != 1 {
		t.Fatalf("expected trimmed headers, got %v", def.Request.Headers)
	}
}

func TestApplyTemplateUpdatesExistingSkipsMissing(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m1", Scope: contract.ScopeProvider, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create def: %v", err)
	}
	tpl, err := svc.CreateTemplate(ctx, contract.CreateTemplate{
		Name:    "tpl",
		Request: contract.CustomRequest{Method: "GET", ResponseContains: "ok"},
	})
	if err != nil {
		t.Fatalf("create tpl: %v", err)
	}

	applied, err := svc.ApplyTemplate(ctx, tpl.ID, []int{def.ID, 9999})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected one applied (missing skipped), got %d", len(applied))
	}
	if applied[0].Request.ResponseContains != "ok" || applied[0].Request.Method != "GET" {
		t.Fatalf("template request not applied: %+v", applied[0].Request)
	}
}

func TestRunHistoryRecordedAndOrdered(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m", Scope: contract.ScopeAccount, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.RecordRun(ctx, contract.RecordRun{
			MonitorID:    def.ID,
			RunID:        "run",
			CheckedCount: 1,
			OKCount:      1,
			Results:      []contract.CheckResult{{AccountID: 1, Model: "gpt-4o-mini", OK: true}},
		}); err != nil {
			t.Fatalf("record run %d: %v", i, err)
		}
	}
	runs, err := svc.ListRuns(ctx, def.ID, 50)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	// Newest first.
	if runs[0].ID < runs[1].ID {
		t.Fatalf("expected runs newest-first, got %d before %d", runs[0].ID, runs[1].ID)
	}
}

func TestDeleteDefinitionRemovesRuns(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m", Scope: contract.ScopeAccount, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.RecordRun(ctx, contract.RecordRun{MonitorID: def.ID, RunID: "r", CheckedCount: 1}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := svc.DeleteDefinition(ctx, def.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetDefinition(ctx, def.ID); err != contract.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
