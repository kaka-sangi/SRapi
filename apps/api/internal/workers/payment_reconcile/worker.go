package paymentreconcile

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 2 * time.Minute
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval      time.Duration
	Clock         service.Clock
	Audit         auditcontract.Store
	Billing       billingcontract.Store
	Events        eventscontract.Store
	Users         userscontract.Store
	Subscriptions subscriptioncontract.Store
	RunGuard      runonceguard.Guard
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
	if deps.Billing == nil && cfg.Billing != nil {
		billingSvc, err := billingservice.New(cfg.Billing, nil)
		if err != nil {
			return nil, err
		}
		deps.Billing = billingSvc
	}
	if deps.Events == nil && cfg.Events != nil {
		eventsSvc, err := eventsservice.New(cfg.Events, nil)
		if err != nil {
			return nil, err
		}
		deps.Events = eventsSvc
	}
	if deps.Subscriptions == nil && cfg.Subscriptions != nil {
		subscriptionSvc, err := subscriptionservice.New(cfg.Subscriptions, nil)
		if err != nil {
			return nil, err
		}
		deps.Subscriptions = subscriptionSvc
	}
	if deps.Balance == nil && cfg.Users != nil {
		usersSvc, err := usersservice.New(cfg.Users, nil)
		if err != nil {
			return nil, err
		}
		deps.Balance = balanceAdapter{users: usersSvc}
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

func (w *Worker) RunOnce(ctx context.Context) (contract.ReconcileOrdersResult, error) {
	if w == nil {
		return contract.ReconcileOrdersResult{}, nil
	}
	var result contract.ReconcileOrdersResult
	_, err := runonceguard.Run(ctx, w.guard, "payment_reconcile", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.payments.ReconcilePendingOrders(runCtx, time.Time{})
		return runErr
	})
	return result, err
}

func (w *Worker) run(ctx context.Context) {
	w.reconcileAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.reconcileAndLog(ctx)
		}
	}
}

func (w *Worker) reconcileAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("payment order reconciliation failed", "error", err)
		return
	}
	if result.Paid > 0 || result.Failed > 0 {
		w.logger.Info("payment orders reconciled", "selected", result.Selected, "paid", result.Paid, "failed", result.Failed)
	}
}

type balanceAdapter struct {
	users *usersservice.Service
}

func (a balanceAdapter) CreditBalance(ctx context.Context, userID int, amount, currency string) error {
	_, err := a.users.UpdateBalance(ctx, userID, usersservice.BalanceUpdateRequest{
		Operation: userscontract.BalanceOperationIncrement,
		Amount:    amount,
		Currency:  currency,
	})
	return err
}

func (a balanceAdapter) DebitBalance(ctx context.Context, userID int, amount, currency string) error {
	_, err := a.users.UpdateBalance(ctx, userID, usersservice.BalanceUpdateRequest{
		Operation: userscontract.BalanceOperationDecrement,
		Amount:    amount,
		Currency:  currency,
	})
	return err
}
