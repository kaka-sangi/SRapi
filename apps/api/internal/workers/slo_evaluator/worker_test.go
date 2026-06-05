package sloevaluator

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func TestRunOnceCreatesBurnRateAlert(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	store := newCaptureStore()
	store.slos = []contract.SLODefinition{{
		ID:         1,
		Name:       "Chat availability",
		SLIType:    contract.SLITypeAvailability,
		Objective:  0.99,
		WindowDays: 1,
		Status:     contract.SLOStatusActive,
		Filter: contract.SLOFilter{
			SourceEndpoint: "/v1/chat/completions",
		},
		AlertPolicy: contract.AlertPolicy{
			Thresholds: []contract.BurnRateThreshold{{
				Severity:        contract.AlertSeverityCritical,
				LongWindow:      time.Hour,
				ShortWindow:     5 * time.Minute,
				BurnRate:        2,
				MinRequestCount: 2,
			}},
		},
	}}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-2 * time.Minute)},
	}
	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 {
		t.Fatalf("unexpected evaluation result: %+v", result)
	}
	if len(store.alerts) != 1 || store.alerts[0].RuleID != "slo.burn_rate.critical" {
		t.Fatalf("expected one burn-rate alert, got %+v", store.alerts)
	}
}

func TestRunRulesOnceFiresGenericMetricAlert(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureStore()
	store.rules = []contract.AlertRule{{
		ID:              1,
		Name:            "Chat error rate",
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.25,
		Severity:        contract.AlertSeverityCritical,
		Enabled:         true,
		WindowSeconds:   3600,
		MinRequestCount: 2,
		Scope:           contract.AlertRuleScope{SourceEndpoint: "/v1/chat/completions"},
	}}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_1", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_2", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-3 * time.Minute)},
	}
	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunRulesOnce(t.Context())
	if err != nil {
		t.Fatalf("run rules once: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 {
		t.Fatalf("unexpected rule evaluation result: %+v", result)
	}
	if len(store.alerts) != 1 || store.alerts[0].RuleID != "rule.1" {
		t.Fatalf("expected one generic rule alert, got %+v", store.alerts)
	}
}

type captureStore struct {
	nextAlertID   int
	nextRuleID    int
	nextSilenceID int
	slos          []contract.SLODefinition
	alerts        []contract.AlertEvent
	rules         []contract.AlertRule
	silences      []contract.AlertSilence
	usageLogs     []usagecontract.UsageLog
}

func newCaptureStore() *captureStore {
	return &captureStore{nextAlertID: 1, nextRuleID: 1, nextSilenceID: 1}
}

func (s *captureStore) CreateSLO(context.Context, contract.SLODefinition) (contract.SLODefinition, error) {
	return contract.SLODefinition{}, contract.ErrNotFound
}

func (s *captureStore) UpdateSLO(context.Context, contract.SLODefinition) (contract.SLODefinition, error) {
	return contract.SLODefinition{}, contract.ErrNotFound
}

func (s *captureStore) FindSLOByID(context.Context, int) (contract.SLODefinition, error) {
	return contract.SLODefinition{}, contract.ErrNotFound
}

func (s *captureStore) ListSLOs(context.Context) ([]contract.SLODefinition, error) {
	return append([]contract.SLODefinition(nil), s.slos...), nil
}

func (s *captureStore) CreateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	input.ID = s.nextAlertID
	s.nextAlertID++
	s.alerts = append(s.alerts, input)
	return input, nil
}

func (s *captureStore) UpdateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	for idx := range s.alerts {
		if s.alerts[idx].ID == input.ID {
			s.alerts[idx] = input
			return input, nil
		}
	}
	return contract.AlertEvent{}, contract.ErrNotFound
}

func (s *captureStore) FindAlertByID(_ context.Context, id int) (contract.AlertEvent, error) {
	for _, alert := range s.alerts {
		if alert.ID == id {
			return alert, nil
		}
	}
	return contract.AlertEvent{}, contract.ErrNotFound
}

func (s *captureStore) ListAlerts(context.Context) ([]contract.AlertEvent, error) {
	return append([]contract.AlertEvent(nil), s.alerts...), nil
}

func (s *captureStore) ListUsageLogs(context.Context) ([]usagecontract.UsageLog, error) {
	return append([]usagecontract.UsageLog(nil), s.usageLogs...), nil
}

func (s *captureStore) ListUsageLogsSince(_ context.Context, since time.Time) ([]usagecontract.UsageLog, error) {
	out := make([]usagecontract.UsageLog, 0, len(s.usageLogs))
	for _, log := range s.usageLogs {
		if since.IsZero() || !log.CreatedAt.Before(since) {
			out = append(out, log)
		}
	}
	return out, nil
}

func (s *captureStore) CreateAlertRule(_ context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	input.ID = s.nextRuleID
	s.nextRuleID++
	s.rules = append(s.rules, input)
	return input, nil
}

func (s *captureStore) UpdateAlertRule(_ context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	for idx := range s.rules {
		if s.rules[idx].ID == input.ID {
			s.rules[idx] = input
			return input, nil
		}
	}
	return contract.AlertRule{}, contract.ErrNotFound
}

func (s *captureStore) FindAlertRuleByID(_ context.Context, id int) (contract.AlertRule, error) {
	for _, rule := range s.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return contract.AlertRule{}, contract.ErrNotFound
}

func (s *captureStore) ListAlertRules(context.Context) ([]contract.AlertRule, error) {
	return append([]contract.AlertRule(nil), s.rules...), nil
}

func (s *captureStore) DeleteAlertRule(_ context.Context, id int) error {
	for idx := range s.rules {
		if s.rules[idx].ID == id {
			s.rules = append(s.rules[:idx], s.rules[idx+1:]...)
			return nil
		}
	}
	return contract.ErrNotFound
}

func (s *captureStore) CreateAlertSilence(_ context.Context, input contract.AlertSilence) (contract.AlertSilence, error) {
	input.ID = s.nextSilenceID
	s.nextSilenceID++
	s.silences = append(s.silences, input)
	return input, nil
}

func (s *captureStore) ListAlertSilences(context.Context) ([]contract.AlertSilence, error) {
	return append([]contract.AlertSilence(nil), s.silences...), nil
}

func (s *captureStore) DeleteAlertSilence(_ context.Context, id int) error {
	for idx := range s.silences {
		if s.silences[idx].ID == id {
			s.silences = append(s.silences[:idx], s.silences[idx+1:]...)
			return nil
		}
	}
	return contract.ErrNotFound
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func ptrString(value string) *string { return &value }
