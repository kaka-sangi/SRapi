package operations

import (
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestAlertRuleAndSilencePersistence(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/operations-alert-rules.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	providerID := 9

	rule, err := store.CreateAlertRule(ctx, contract.AlertRule{
		Name:            "Chat error rate",
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.1,
		Severity:        contract.AlertSeverityCritical,
		Enabled:         true,
		WindowSeconds:   1800,
		CooldownSeconds: 300,
		MinRequestCount: 5,
		Scope:           contract.AlertRuleScope{SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", ProviderID: &providerID},
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	if rule.ID == 0 || rule.Scope.ProviderID == nil || *rule.Scope.ProviderID != providerID {
		t.Fatalf("unexpected persisted rule: %+v", rule)
	}

	rule.Enabled = false
	rule.Threshold = 0.2
	updated, err := store.UpdateAlertRule(ctx, rule)
	if err != nil {
		t.Fatalf("update alert rule: %v", err)
	}
	if updated.Enabled || updated.Threshold != 0.2 {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	found, err := store.FindAlertRuleByID(ctx, rule.ID)
	if err != nil {
		t.Fatalf("find alert rule: %v", err)
	}
	if found.MetricType != contract.AlertMetricErrorRate || found.MinRequestCount != 5 {
		t.Fatalf("unexpected found rule: %+v", found)
	}

	rules, err := store.ListAlertRules(ctx)
	if err != nil {
		t.Fatalf("list alert rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %+v", rules)
	}

	createdBy := 3
	silence, err := store.CreateAlertSilence(ctx, contract.AlertSilence{
		Comment:   "deploy window",
		Matcher:   contract.AlertSilenceMatcher{RuleID: "rule.1", Severity: contract.AlertSeverityCritical, ProviderID: &providerID},
		StartsAt:  now,
		EndsAt:    now.Add(time.Hour),
		CreatedBy: &createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create silence: %v", err)
	}
	if silence.ID == 0 || silence.Matcher.RuleID != "rule.1" || silence.Matcher.ProviderID == nil || silence.CreatedBy == nil {
		t.Fatalf("unexpected persisted silence: %+v", silence)
	}

	silences, err := store.ListAlertSilences(ctx)
	if err != nil {
		t.Fatalf("list silences: %v", err)
	}
	if len(silences) != 1 {
		t.Fatalf("expected one silence, got %+v", silences)
	}

	if err := store.DeleteAlertSilence(ctx, silence.ID); err != nil {
		t.Fatalf("delete silence: %v", err)
	}
	silences, err = store.ListAlertSilences(ctx)
	if err != nil {
		t.Fatalf("list silences after delete: %v", err)
	}
	if len(silences) != 0 {
		t.Fatalf("expected zero silences after delete, got %+v", silences)
	}

	if err := store.DeleteAlertRule(ctx, rule.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if _, err := store.FindAlertRuleByID(ctx, rule.ID); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	if err := store.DeleteAlertRule(ctx, rule.ID); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected not found deleting missing rule, got %v", err)
	}
}
