package orderexpirer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 5 * time.Minute
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval time.Duration
	Clock    service.Clock
	Audit    auditcontract.Store
	RunGuard runonceguard.Guard
}

type Dependencies = service.Dependencies

type Worker struct {
	payments *service.Service
	logger   *slog.Logger
	interval time.Duration
	guard    runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store contract.Store, masterKey string, deps service.Dependencies, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, service.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	if deps.Audit == nil && cfg.Audit != nil {
		auditSvc, err := auditservice.New(cfg.Audit, nil)
		if err != nil {
			return nil, err
		}
		deps.Audit = auditSvc
	}
	payments, err := service.New(store, masterKey, deps, cfg.Clock)
	if err != nil {
		return nil, err
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Worker{payments: payments, logger: logger, interval: interval, guard: cfg.RunGuard}, nil
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

func (w *Worker) RunOnce(ctx context.Context) (contract.ExpireOrdersResult, error) {
	if w == nil {
		return contract.ExpireOrdersResult{}, nil
	}
	var result contract.ExpireOrdersResult
	_, err := runonceguard.Run(ctx, w.guard, "order_expirer", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.payments.ExpirePendingOrders(runCtx, time.Time{})
		return runErr
	})
	return result, err
}

func (w *Worker) run(ctx context.Context) {
	w.expireAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.expireAndLog(ctx)
		}
	}
}

func (w *Worker) expireAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("payment order expiration failed", "error", err)
		return
	}
	if result.Expired > 0 {
		w.logger.Info("payment orders expired", "selected", result.Selected, "expired", result.Expired)
	}
}
