// Package usageaggregationreconciler drives the cross-table billing aggregation
// for usage_log rows whose eager (live-path) aggregation was dropped — e.g. when
// the process crashed between recording a usage_log row and applying its
// subscription/api-key increments. It periodically sweeps unaggregated rows and
// applies them idempotently via the aggregation coordinator, so the
// materialized-usage and cost-usage caches converge to the usage_log truth.
package usageaggregationreconciler

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = time.Minute
	shutdownPollInterval = 10 * time.Millisecond
	defaultBatchLimit    = 500
	defaultMaxBatches    = 20
	// defaultSettleMargin keeps the sweep off rows young enough that the live
	// eager apply may still be in flight; it must exceed the live async-write
	// timeout. ApplyAggregation is idempotent regardless, so this only avoids
	// redundant claims.
	defaultSettleMargin = 5 * time.Minute
	// defaultMaxAge floors how far back the sweep looks. It must stay shorter than
	// the migration's backfill window (45d) so the sweep never reaches pre-feature
	// rows (aggregated by the old path but unmarked) and double-counts them. It
	// also covers the longest billing window (monthly), beyond which drift is moot
	// because the window has already reset.
	defaultMaxAge = 35 * 24 * time.Hour
)

// ErrInvalidInput is returned when the worker is constructed without an aggregator.
var ErrInvalidInput = errors.New("usage aggregation reconciler: nil aggregator")

// Aggregator sweeps unaggregated, settled usage_log rows created in [after,
// before) and applies their billing aggregation, returning how many it aggregated.
type Aggregator interface {
	SweepPending(ctx context.Context, after, before time.Time, limit int) (int, error)
}

type Config struct {
	Interval     time.Duration
	BatchLimit   int
	MaxBatches   int
	SettleMargin time.Duration
	MaxAge       time.Duration
	Clock        func() time.Time
	RunGuard     runonceguard.Guard
}

type Worker struct {
	agg          Aggregator
	logger       *slog.Logger
	interval     time.Duration
	limit        int
	maxBatch     int
	settleMargin time.Duration
	maxAge       time.Duration
	now          func() time.Time
	guard        runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(agg Aggregator, logger *slog.Logger, cfg Config) (*Worker, error) {
	if agg == nil {
		return nil, ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	limit := cfg.BatchLimit
	if limit <= 0 {
		limit = defaultBatchLimit
	}
	maxBatch := cfg.MaxBatches
	if maxBatch <= 0 {
		maxBatch = defaultMaxBatches
	}
	settle := cfg.SettleMargin
	if settle <= 0 {
		settle = defaultSettleMargin
	}
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	now := cfg.Clock
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Worker{
		agg:          agg,
		logger:       logger,
		interval:     interval,
		limit:        limit,
		maxBatch:     maxBatch,
		settleMargin: settle,
		maxAge:       maxAge,
		now:          now,
		guard:        cfg.RunGuard,
	}, nil
}

func (w *Worker) Start(parent context.Context) {
	if w == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("worker panicked; goroutine stopped", "worker", "usage_aggregation_reconciler", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		w.run(ctx)
	}()
}

func (w *Worker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}

	cancel()
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			w.mu.Lock()
			if w.done == done {
				w.cancel = nil
				w.done = nil
			}
			w.mu.Unlock()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) run(ctx context.Context) {
	w.sweepAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sweepAndLog(ctx)
		}
	}
}

func (w *Worker) sweepAndLog(ctx context.Context) {
	applied, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("usage aggregation reconcile failed", "error", err)
		return
	}
	if applied > 0 {
		w.logger.Info("reconciled dropped usage billing aggregations", "applied", applied)
	}
}

// RunOnce performs a single reconcile pass under the run-once guard (so only one
// instance sweeps in a multi-replica deployment). Returns the number of rows
// aggregated.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w == nil {
		return 0, nil
	}
	var total int
	_, err := runonceguard.Run(ctx, w.guard, "usage_aggregation_reconciler", func(runCtx context.Context) error {
		var runErr error
		total, runErr = w.sweep(runCtx)
		return runErr
	})
	return total, err
}

func (w *Worker) sweep(ctx context.Context) (int, error) {
	now := w.now()
	before := now.Add(-w.settleMargin)
	after := now.Add(-w.maxAge)
	total := 0
	for batch := 0; batch < w.maxBatch; batch++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		applied, err := w.agg.SweepPending(ctx, after, before, w.limit)
		total += applied
		if err != nil {
			return total, err
		}
		// No progress this batch: either drained, or the remaining rows were
		// claimed concurrently. Either way the next tick will catch any stragglers.
		if applied == 0 {
			break
		}
	}
	return total, nil
}
