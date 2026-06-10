package authsessioncleanup

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 24 * time.Hour
	shutdownPollInterval = 10 * time.Millisecond
)

type Clock interface {
	Now() time.Time
}

// Config controls the expired auth session cleanup worker.
type Config struct {
	Interval time.Duration
	Clock    Clock
	RunGuard runonceguard.Guard
}

// Worker periodically expires active console sessions whose expiry time has passed.
type Worker struct {
	store    contract.CleanupStore
	logger   *slog.Logger
	interval time.Duration
	clock    Clock
	guard    runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New creates an auth session cleanup worker from a store that supports cleanup.
func New(store contract.CleanupStore, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, service.ErrInvalidInput
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

// NewFromStore returns nil when store does not support expired session cleanup.
func NewFromStore(store any, logger *slog.Logger, cfg Config) (*Worker, error) {
	cleanupStore, ok := store.(contract.CleanupStore)
	if !ok {
		return nil, nil
	}
	return New(cleanupStore, logger, cfg)
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

// RunOnce expires sessions for the worker's current clock time.
func (w *Worker) RunOnce(ctx context.Context) (contract.CleanupExpiredSessionsResult, error) {
	if w == nil {
		return contract.CleanupExpiredSessionsResult{}, nil
	}
	var result contract.CleanupExpiredSessionsResult
	_, err := runonceguard.Run(ctx, w.guard, "auth_session_cleanup", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.store.CleanupExpiredSessions(runCtx, w.clock.Now())
		return runErr
	})
	return result, err
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
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("auth session cleanup failed", "error", err)
		return
	}
	if result.Expired > 0 {
		w.logger.Info("auth sessions expired", "selected", result.Selected, "expired", result.Expired)
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}
