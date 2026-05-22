package retention

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

func TestRunOnceCleansConfiguredRetention(t *testing.T) {
	store := &captureRetentionStore{}
	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		UsageLogsDays: 90,
		Clock:         fixedClock{now: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.UsageLogs != 2 {
		t.Fatalf("expected cleanup result, got %+v", result)
	}
	if store.cutoffs.UsageLogs == nil {
		t.Fatal("expected usage log retention cutoff")
	}
}

type captureRetentionStore struct {
	cutoffs contract.RetentionCutoffs
}

func (s *captureRetentionStore) Cleanup(_ context.Context, cutoffs contract.RetentionCutoffs) (contract.CleanupResult, error) {
	s.cutoffs = cutoffs
	return contract.CleanupResult{UsageLogs: 2}, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
