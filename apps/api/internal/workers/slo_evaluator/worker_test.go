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

type captureStore struct {
	nextAlertID int
	slos        []contract.SLODefinition
	alerts      []contract.AlertEvent
	usageLogs   []usagecontract.UsageLog
}

func newCaptureStore() *captureStore {
	return &captureStore{nextAlertID: 1}
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

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func ptrString(value string) *string { return &value }
