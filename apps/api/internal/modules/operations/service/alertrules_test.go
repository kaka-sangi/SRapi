package service

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func TestAlertRuleCRUDValidatesAndDefaults(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{Name: "  "}); err != ErrInvalidInput {
		t.Fatalf("expected invalid input for blank name, got %v", err)
	}

	providerID := 7
	created, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:      "Chat error rate",
		Threshold: 0.1,
		Scope:     contract.AlertRuleScope{SourceEndpoint: "/v1/chat/completions", ProviderID: &providerID},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if created.ID == 0 ||
		created.MetricType != contract.AlertMetricErrorRate ||
		created.Operator != contract.AlertOperatorGT ||
		created.Severity != contract.AlertSeverityWarning ||
		!created.Enabled ||
		created.WindowSeconds != 3600 {
		t.Fatalf("unexpected rule defaults: %+v", created)
	}

	disabled := false
	updated, err := svc.UpdateAlertRule(t.Context(), created.ID, contract.UpdateAlertRuleRequest{
		Enabled:    &disabled,
		MetricType: metricPtr(contract.AlertMetricLatencyP95),
		Operator:   operatorPtr(contract.AlertOperatorGTE),
		Threshold:  floatPtr(2500),
		Severity:   severityPtr(contract.AlertSeverityCritical),
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if updated.Enabled || updated.MetricType != contract.AlertMetricLatencyP95 || updated.Severity != contract.AlertSeverityCritical {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	rules, err := svc.ListAlertRules(t.Context())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %+v", rules)
	}

	if err := svc.DeleteAlertRule(t.Context(), created.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	rules, err = svc.ListAlertRules(t.Context())
	if err != nil {
		t.Fatalf("list rules after delete: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected zero rules after delete, got %+v", rules)
	}
}

func TestEvaluateAlertRulesFiresUpdatesAndResolvesEvents(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Chat error rate",
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.25,
		Severity:        contract.AlertSeverityCritical,
		WindowSeconds:   3600,
		MinRequestCount: 2,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: "/v1/chat/completions",
			Model:          "gpt-ops",
			ProviderID:     intPtr(7),
		},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Model: "gpt-ops", ProviderID: intPtr(7), Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_1", SourceEndpoint: "/v1/chat/completions", Model: "gpt-ops", ProviderID: intPtr(7), Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-3 * time.Minute)},
		{RequestID: "req_bad_2", SourceEndpoint: "/v1/chat/completions", Model: "gpt-ops", ProviderID: intPtr(7), Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-4 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 || result.Suppressed != 0 {
		t.Fatalf("unexpected first evaluation result: %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %+v", alerts)
	}
	if alerts[0].RuleID != "rule.1" || alerts[0].Status != contract.AlertStatusFiring || alerts[0].Severity != contract.AlertSeverityCritical {
		t.Fatalf("unexpected fired alert: %+v", alerts[0])
	}
	if alerts[0].Details["source_endpoint"] != "/v1/chat/completions" || alerts[0].Details["model"] != "gpt-ops" || alerts[0].Details["provider_id"] != 7 {
		t.Fatalf("expected alert rule scope in details, got %+v", alerts[0].Details)
	}

	result, err = svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("reevaluate rules: %v", err)
	}
	if result.Created != 0 || result.Updated != 1 {
		t.Fatalf("expected existing alert update, got %+v", result)
	}

	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok_1", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_ok_2", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-3 * time.Minute)},
	}
	result, err = svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate recovery: %v", err)
	}
	if result.Breached != 0 || result.Resolved != 1 {
		t.Fatalf("expected resolution, got %+v", result)
	}
}

func TestEvaluateAlertRulesSuppressesWithMatchingSilence(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Chat error rate",
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.25,
		Severity:        contract.AlertSeverityCritical,
		MinRequestCount: 2,
		Scope:           contract.AlertRuleScope{SourceEndpoint: "/v1/chat/completions"},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := svc.CreateAlertSilence(t.Context(), contract.CreateAlertSilenceRequest{
		Comment:  "maintenance",
		Matcher:  contract.AlertSilenceMatcher{RuleID: "rule.1"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create silence: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_1", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-3 * time.Minute)},
		{RequestID: "req_bad_2", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-4 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Breached != 1 || result.Created != 1 || result.Suppressed != 1 {
		t.Fatalf("expected suppressed creation, got %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %+v", alerts)
	}
	if alerts[0].Status != contract.AlertStatusSuppressed || alerts[0].SuppressedBy == nil || *alerts[0].SuppressedBy != "silence.1" {
		t.Fatalf("expected suppressed alert with silence attribution, got %+v", alerts[0])
	}
}

func TestCreateAlertSilenceRejectsInvalidWindow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertSilence(t.Context(), contract.CreateAlertSilenceRequest{
		StartsAt: now,
		EndsAt:   now.Add(-time.Minute),
	}); err != ErrInvalidInput {
		t.Fatalf("expected invalid input for inverted window, got %v", err)
	}
}

func metricPtr(v contract.AlertMetricType) *contract.AlertMetricType { return &v }
func operatorPtr(v contract.AlertOperator) *contract.AlertOperator   { return &v }
func severityPtr(v contract.AlertSeverity) *contract.AlertSeverity   { return &v }
func floatPtr(v float64) *float64                                    { return &v }
func intPtr(v int) *int                                              { return &v }
