package sloevaluator

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
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
	RunGuard runonceguard.Guard
}

type Worker struct {
	operations *service.Service
	logger     *slog.Logger
	interval   time.Duration
	timeout    time.Duration
	guard      runonceguard.Guard

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
		guard:      cfg.RunGuard,
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
				w.logger.Error("worker panicked; goroutine stopped", "worker", "slo_evaluator", "panic", r, "stack", string(debug.Stack()))
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

func (w *Worker) RunOnce(ctx context.Context) (contract.AlertEvaluationResult, error) {
	if w == nil {
		return contract.AlertEvaluationResult{}, nil
	}
	result, _, err := w.runGuarded(ctx, func(runCtx context.Context) (contract.AlertEvaluationResult, contract.AlertRuleEvaluationResult, error) {
		result, err := w.evaluateSLOAlerts(runCtx)
		return result, contract.AlertRuleEvaluationResult{}, err
	})
	return result, err
}

// RunRulesOnce evaluates the configurable generic metric alert rules and applies
// any active silences. It is invoked on the same cadence as the SLO pass.
func (w *Worker) RunRulesOnce(ctx context.Context) (contract.AlertRuleEvaluationResult, error) {
	if w == nil {
		return contract.AlertRuleEvaluationResult{}, nil
	}
	_, result, err := w.runGuarded(ctx, func(runCtx context.Context) (contract.AlertEvaluationResult, contract.AlertRuleEvaluationResult, error) {
		result, runErr := w.evaluateAlertRules(runCtx)
		return contract.AlertEvaluationResult{}, result, runErr
	})
	return result, err
}

func (w *Worker) evaluateSLOAlerts(ctx context.Context) (contract.AlertEvaluationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	evalCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	return w.operations.EvaluateSLOAlerts(evalCtx)
}

func (w *Worker) evaluateAlertRules(ctx context.Context) (contract.AlertRuleEvaluationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	evalCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	return w.operations.EvaluateAlertRules(evalCtx)
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
	result, ruleResult, err := w.runGuarded(ctx, func(runCtx context.Context) (contract.AlertEvaluationResult, contract.AlertRuleEvaluationResult, error) {
		result, err := w.evaluateSLOAlerts(runCtx)
		if err != nil {
			return result, contract.AlertRuleEvaluationResult{}, err
		}
		ruleResult, err := w.evaluateAlertRules(runCtx)
		return result, ruleResult, err
	})
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

	if ruleResult.Created > 0 || ruleResult.Updated > 0 || ruleResult.Resolved > 0 {
		w.logger.Info(
			"alert rule evaluation completed",
			"evaluated", ruleResult.Evaluated,
			"breached", ruleResult.Breached,
			"created", ruleResult.Created,
			"updated", ruleResult.Updated,
			"resolved", ruleResult.Resolved,
			"suppressed", ruleResult.Suppressed,
		)
	}
}

func (w *Worker) runGuarded(ctx context.Context, fn func(context.Context) (contract.AlertEvaluationResult, contract.AlertRuleEvaluationResult, error)) (contract.AlertEvaluationResult, contract.AlertRuleEvaluationResult, error) {
	var sloResult contract.AlertEvaluationResult
	var ruleResult contract.AlertRuleEvaluationResult
	_, err := runonceguard.Run(ctx, w.guard, "slo_evaluator", func(runCtx context.Context) error {
		var runErr error
		sloResult, ruleResult, runErr = fn(runCtx)
		return runErr
	})
	return sloResult, ruleResult, err
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
