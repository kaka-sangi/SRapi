package service

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
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
