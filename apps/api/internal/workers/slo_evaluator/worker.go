package sloevaluator

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/service"
)

const (
	defaultInterval      = time.Minute
	defaultTimeout       = 30 * time.Second
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval time.Duration
	Timeout  time.Duration
	Clock    service.Clock
}

type Worker struct {
	operations *service.Service
	logger     *slog.Logger
	interval   time.Duration
	timeout    time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store contract.ObservabilityStore, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, service.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	operations, err := service.NewWithStores(nil, store, cfg.Clock)
	if err != nil {
		return nil, err
	}
	return &Worker{
		operations: operations,
		logger:     logger,
		interval:   durationOrDefault(cfg.Interval, defaultInterval),
		timeout:    durationOrDefault(cfg.Timeout, defaultTimeout),
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

func (w *Worker) RunOnce(ctx context.Context) (contract.AlertEvaluationResult, error) {
	if w == nil {
		return contract.AlertEvaluationResult{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	evalCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	return w.operations.EvaluateSLOAlerts(evalCtx)
}

func (w *Worker) run(ctx context.Context) {
	w.evaluateAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.evaluateAndLog(ctx)
		}
	}
}

func (w *Worker) evaluateAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("SLO alert evaluation failed", "error", err)
		return
	}
	if result.Created > 0 || result.Updated > 0 || result.Resolved > 0 {
		w.logger.Info(
			"SLO alert evaluation completed",
			"evaluated", result.Evaluated,
			"breached", result.Breached,
			"created", result.Created,
			"updated", result.Updated,
			"resolved", result.Resolved,
		)
	}
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
