package usageaggregationreconciler

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

type fakeAggregator struct {
	seq     []int       // applied count returned per successive call
	calls   int         // number of SweepPending invocations
	afters  []time.Time // the `after` floor passed to each call
	befores []time.Time // the `before` cutoff passed to each call
	limit   int         // the limit passed (last call)
}

func (f *fakeAggregator) SweepPending(_ context.Context, after, before time.Time, limit int) (int, error) {
	f.afters = append(f.afters, after)
	f.befores = append(f.befores, before)
	f.limit = limit
	i := f.calls
	f.calls++
	if i < len(f.seq) {
		return f.seq[i], nil
	}
	return 0, nil
}

func newTestWorker(t *testing.T, agg Aggregator, cfg Config) *Worker {
	t.Helper()
	w, err := New(agg, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), cfg)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	return w
}

func TestSweepStopsWhenDrained(t *testing.T) {
	agg := &fakeAggregator{seq: []int{500, 500, 7}}
	w := newTestWorker(t, agg, Config{BatchLimit: 500, MaxBatches: 10})

	total, err := w.sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	// 500 + 500 + 7, then a 0-batch breaks the loop.
	if total != 1007 {
		t.Fatalf("total applied = %d, want 1007", total)
	}
	if agg.calls != 4 {
		t.Fatalf("SweepPending calls = %d, want 4 (3 productive + 1 empty)", agg.calls)
	}
	if agg.limit != 500 {
		t.Fatalf("limit = %d, want 500", agg.limit)
	}
}

func TestSweepRespectsMaxBatches(t *testing.T) {
	// Always-full batches must stop at MaxBatches rather than loop forever.
	agg := &fakeAggregator{seq: []int{500, 500, 500, 500, 500}}
	w := newTestWorker(t, agg, Config{BatchLimit: 500, MaxBatches: 3})

	total, err := w.sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if total != 1500 || agg.calls != 3 {
		t.Fatalf("total=%d calls=%d, want total=1500 calls=3", total, agg.calls)
	}
}

func TestSweepUsesSettleMarginCutoff(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	agg := &fakeAggregator{seq: []int{0}}
	w := newTestWorker(t, agg, Config{
		SettleMargin: 10 * time.Minute,
		MaxAge:       48 * time.Hour,
		Clock:        func() time.Time { return now },
	})

	if _, err := w.sweep(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(agg.befores) == 0 || len(agg.afters) == 0 {
		t.Fatal("expected at least one SweepPending call")
	}
	wantBefore := now.Add(-10 * time.Minute)
	if !agg.befores[0].Equal(wantBefore) {
		t.Fatalf("before cutoff = %s, want %s (now - settle margin)", agg.befores[0], wantBefore)
	}
	wantAfter := now.Add(-48 * time.Hour)
	if !agg.afters[0].Equal(wantAfter) {
		t.Fatalf("after floor = %s, want %s (now - max age)", agg.afters[0], wantAfter)
	}
}
