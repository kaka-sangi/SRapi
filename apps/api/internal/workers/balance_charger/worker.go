package balancecharger

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
)

const (
	defaultInterval      = time.Minute
	shutdownPollInterval = 10 * time.Millisecond
	defaultBatchLimit    = 500
)

type Config struct {
	Interval time.Duration
	Clock    billingservice.Clock
	Audit    auditcontract.Store
	Users    userscontract.Store
}

type Worker struct {
	billing  *billingservice.Service
	users    *usersservice.Service
	audit    *auditservice.Service
	logger   *slog.Logger
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store billingcontract.UsageChargeStore, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, billingservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	billingSvc, err := billingservice.NewUsageCharger(store, cfg.Clock)
	if err != nil {
		return nil, err
	}
	var usersSvc *usersservice.Service
	if cfg.Users != nil {
		usersSvc, err = usersservice.New(cfg.Users, nil)
		if err != nil {
			return nil, err
		}
	}
	var auditSvc *auditservice.Service
	if cfg.Audit != nil {
		auditSvc, err = auditservice.New(cfg.Audit, nil)
		if err != nil {
			return nil, err
		}
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Worker{
		billing:  billingSvc,
		users:    usersSvc,
		audit:    auditSvc,
		logger:   logger,
		interval: interval,
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

func (w *Worker) RunOnce(ctx context.Context) (billingcontract.ChargePendingUsageResult, error) {
	if w == nil {
		return billingcontract.ChargePendingUsageResult{}, nil
	}
	result, err := w.billing.ChargePendingUsage(ctx, billingcontract.ChargePendingUsageRequest{Limit: defaultBatchLimit})
	if err != nil {
		return result, err
	}
	if err := w.handleDisabledUsers(ctx, result.Batches); err != nil && !errors.Is(err, context.Canceled) {
		return result, err
	}
	return result, nil
}

func (w *Worker) run(ctx context.Context) {
	w.chargeAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.chargeAndLog(ctx)
		}
	}
}

func (w *Worker) chargeAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("balance charging failed", "error", err)
		return
	}
	if result.Charged > 0 {
		w.logger.Info("usage charges applied", "selected", result.Selected, "charged", result.Charged)
	}
}

func (w *Worker) handleDisabledUsers(ctx context.Context, batches []billingcontract.ChargeUsageResult) error {
	if w == nil || (w.users == nil && w.audit == nil) {
		return nil
	}
	for _, batch := range batches {
		if !batch.UserDisabled {
			continue
		}
		if w.users != nil {
			if _, err := w.users.SetStatus(ctx, batch.UserID, userscontract.StatusDisabled); err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				w.logger.Warn("failed to suspend user after low balance", "error", err, "user_id", batch.UserID)
			}
		}
		if w.audit != nil {
			if _, err := w.audit.Record(ctx, auditcontract.RecordRequest{
				Action:       "user.suspend",
				ResourceType: "user",
				ResourceID:   strconv.Itoa(batch.UserID),
				After: map[string]any{
					"status":            "disabled",
					"reason":            "insufficient_balance",
					"balance_after":     batch.BalanceAfter,
					"ledger_entry_id":   batch.LedgerEntry.ID,
					"usage_log_ids":     batch.ChargedUsageLogIDs,
					"reference_type":    batch.LedgerEntry.ReferenceType,
					"reference_id":      batch.LedgerEntry.ReferenceID,
					"charged_at":        batch.LedgerEntry.CreatedAt.UTC().Format(time.RFC3339Nano),
					"balance_before":    batch.BalanceBefore,
					"usage_charge_type": string(batch.LedgerEntry.Type),
				},
			}); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Warn("failed to audit user suspension after low balance", "error", err, "user_id", batch.UserID)
			}
		}
	}
	return nil
}
