package idempotencycleanup

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = time.Hour
	shutdownPollInterval = 10 * time.Millisecond
)

var errNilStore = errors.New("idempotency cleanup worker requires a store")

type Clock interface {
	Now() time.Time
}

// Config controls the expired idempotency record cleanup worker.
type Config struct {
	Interval time.Duration
	Clock    Clock
	RunGuard runonceguard.Guard
}

// Worker periodically deletes idempotency records whose expiry time has passed so
// the table does not grow without bound.
type Worker struct {
	store    contract.Store
	logger   *slog.Logger
	interval time.Duration
	clock    Clock
	guard    runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store contract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, errNilStore
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Worker{store: store, logger: logger, interval: interval, clock: clock, guard: cfg.RunGuard}, nil
}

// Start runs the cleanup loop until Shutdown is called or parent is canceled.
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
		w.run(ctx)
	}()
}

// Shutdown stops the cleanup loop and waits for it to exit.
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

// RunOnce deletes records expired as of the worker's current clock time.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w == nil {
		return 0, nil
	}
	var deleted int
	_, err := runonceguard.Run(ctx, w.guard, "idempotency_cleanup", func(runCtx context.Context) error {
		var runErr error
		deleted, runErr = w.store.DeleteExpired(runCtx, w.clock.Now())
		return runErr
	})
	return deleted, err
}

func (w *Worker) run(ctx context.Context) {
	w.cleanupAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanupAndLog(ctx)
		}
	}
}

func (w *Worker) cleanupAndLog(ctx context.Context) {
	deleted, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("idempotency cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		w.logger.Info("expired idempotency records deleted", "deleted", deleted)
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}
