package service

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func TestCleanupRetentionBuildsConfiguredCutoffs(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	store := &captureRetentionStore{}
	svc, err := New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.CleanupRetention(t.Context(), contract.RetentionPolicy{
		UsageLogs:              90 * 24 * time.Hour,
		SchedulerDecisions:     30 * 24 * time.Hour,
		SchedulerFeedbacks:     45 * 24 * time.Hour,
		AuditLogs:              365 * 24 * time.Hour,
		AccountHealthSnapshots: 15 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("cleanup retention: %v", err)
	}
	if result.UsageLogs != 1 {
		t.Fatalf("expected store result, got %+v", result)
	}

	assertCutoff(t, store.cutoffs.UsageLogs, now.Add(-90*24*time.Hour))
	assertCutoff(t, store.cutoffs.SchedulerDecisions, now.Add(-30*24*time.Hour))
	assertCutoff(t, store.cutoffs.SchedulerFeedbacks, now.Add(-45*24*time.Hour))
	assertCutoff(t, store.cutoffs.AuditLogs, now.Add(-365*24*time.Hour))
	assertCutoff(t, store.cutoffs.AccountHealthSnapshots, now.Add(-15*24*time.Hour))
}

func TestCleanupRetentionSkipsDisabledPolicies(t *testing.T) {
	store := &captureRetentionStore{}
	svc, err := New(store, fixedClock{now: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.CleanupRetention(t.Context(), contract.RetentionPolicy{}); err != nil {
		t.Fatalf("cleanup retention: %v", err)
	}
	if store.cutoffs.UsageLogs != nil ||
		store.cutoffs.SchedulerDecisions != nil ||
		store.cutoffs.SchedulerFeedbacks != nil ||
		store.cutoffs.AuditLogs != nil ||
		store.cutoffs.AccountHealthSnapshots != nil {
		t.Fatalf("expected nil cutoffs for disabled retention, got %+v", store.cutoffs)
	}
}

type captureRetentionStore struct {
	cutoffs contract.RetentionCutoffs
}

func (s *captureRetentionStore) Cleanup(_ context.Context, cutoffs contract.RetentionCutoffs) (contract.CleanupResult, error) {
	s.cutoffs = cutoffs
	return contract.CleanupResult{UsageLogs: 1}, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func assertCutoff(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil || !got.Equal(want) {
		t.Fatalf("expected cutoff %s, got %v", want, got)
	}
}

func TestCreateAndListSLOEvaluatesAvailabilityBurnRate(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_good", SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", Success: true, CreatedAt: now.Add(-time.Hour)},
		{RequestID: "req_provider_bad", SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-time.Hour)},
		{RequestID: "req_client_bad", SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", Success: false, ErrorClass: ptrString("invalid_request"), CreatedAt: now.Add(-time.Hour)},
		{RequestID: "req_other_endpoint", SourceEndpoint: "/v1/messages", Model: "gpt-4o-mini", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-time.Hour)},
	}
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.CreateSLO(t.Context(), contract.CreateSLORequest{
		Name:       "Chat availability",
		SLIType:    contract.SLITypeAvailability,
		Objective:  0.99,
		WindowDays: 28,
		Filter: contract.SLOFilter{
			SourceEndpoint:    "/v1/chat/completions",
			ErrorOwnerExclude: []string{"client", "business"},
		},
	})
	if err != nil {
		t.Fatalf("create slo: %v", err)
	}
	if created.Status != contract.SLOStatusActive || created.AlertPolicy.Name != "multi_window_burn_rate" {
		t.Fatalf("unexpected created slo defaults: %+v", created)
	}

	items, err := svc.ListSLOs(t.Context())
	if err != nil {
		t.Fatalf("list slos: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one slo, got %+v", items)
	}
	evaluation := items[0].Evaluation
	if evaluation.TotalRequests != 2 || evaluation.GoodRequests != 1 || evaluation.BadRequests != 1 {
		t.Fatalf("unexpected evaluation counts: %+v", evaluation)
	}
	if evaluation.BurnRate < 49.9 || evaluation.BurnRate > 50.1 {
		t.Fatalf("expected burn rate near 50, got %+v", evaluation)
	}
}

func TestAcknowledgeAlertMarksActorAndTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	alert, err := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "slo_chat_availability",
		Severity:    contract.AlertSeverityCritical,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "sha256:test",
		Summary:     "burn rate high",
		Details:     map[string]any{"burn_rate": 14.4},
		StartedAt:   now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	updated, err := svc.AcknowledgeAlert(t.Context(), alert.ID, contract.AckAlertRequest{ActorUserID: 42})
	if err != nil {
		t.Fatalf("ack alert: %v", err)
	}
	if updated.Status != contract.AlertStatusAcknowledged || updated.AcknowledgedAt == nil || !updated.AcknowledgedAt.Equal(now) || updated.AcknowledgedBy == nil || *updated.AcknowledgedBy != 42 {
		t.Fatalf("unexpected acknowledged alert: %+v", updated)
	}
}

type captureObservabilityStore struct {
	nextSLOID   int
	nextAlertID int
	slos        map[int]contract.SLODefinition
	alerts      map[int]contract.AlertEvent
	usageLogs   []usagecontract.UsageLog
}

func newCaptureObservabilityStore() *captureObservabilityStore {
	return &captureObservabilityStore{
		nextSLOID:   1,
		nextAlertID: 1,
		slos:        map[int]contract.SLODefinition{},
		alerts:      map[int]contract.AlertEvent{},
	}
}

func (s *captureObservabilityStore) CreateSLO(_ context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	input.ID = s.nextSLOID
	s.nextSLOID++
	s.slos[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) UpdateSLO(_ context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	if _, ok := s.slos[input.ID]; !ok {
		return contract.SLODefinition{}, ErrNotFound
	}
	s.slos[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) FindSLOByID(_ context.Context, id int) (contract.SLODefinition, error) {
	value, ok := s.slos[id]
	if !ok {
		return contract.SLODefinition{}, ErrNotFound
	}
	return value, nil
}

func (s *captureObservabilityStore) ListSLOs(_ context.Context) ([]contract.SLODefinition, error) {
	out := make([]contract.SLODefinition, 0, len(s.slos))
	for _, value := range s.slos {
		out = append(out, value)
	}
	return out, nil
}

func (s *captureObservabilityStore) CreateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	input.ID = s.nextAlertID
	s.nextAlertID++
	s.alerts[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) UpdateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	if _, ok := s.alerts[input.ID]; !ok {
		return contract.AlertEvent{}, ErrNotFound
	}
	s.alerts[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) FindAlertByID(_ context.Context, id int) (contract.AlertEvent, error) {
	value, ok := s.alerts[id]
	if !ok {
		return contract.AlertEvent{}, ErrNotFound
	}
	return value, nil
}

func (s *captureObservabilityStore) ListAlerts(_ context.Context) ([]contract.AlertEvent, error) {
	out := make([]contract.AlertEvent, 0, len(s.alerts))
	for _, value := range s.alerts {
		out = append(out, value)
	}
	return out, nil
}

func (s *captureObservabilityStore) ListUsageLogs(_ context.Context) ([]usagecontract.UsageLog, error) {
	return append([]usagecontract.UsageLog(nil), s.usageLogs...), nil
}

func ptrString(value string) *string { return &value }
