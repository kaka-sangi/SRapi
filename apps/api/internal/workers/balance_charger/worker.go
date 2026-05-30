package balancecharger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
)

const (
	defaultInterval      = time.Minute
	shutdownPollInterval = 10 * time.Millisecond
	defaultBatchLimit    = 500
	defaultMaxBatches    = 20
)

type Config struct {
	Interval         time.Duration
	BatchLimit       int
	MaxBatchesPerRun int
	Clock            billingservice.Clock
	Audit            auditcontract.Store
	Users            userscontract.Store
	Events           eventscontract.Store
	AdminControl     admincontrolcontract.Store
}

type Worker struct {
	billing      *billingservice.Service
	users        *usersservice.Service
	audit        *auditservice.Service
	events       *eventsservice.Service
	adminControl *admincontrolservice.Service
	logger       *slog.Logger
	interval     time.Duration
	limit        int
	maxBatch     int

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
	var eventsSvc *eventsservice.Service
	if cfg.Events != nil {
		eventsSvc, err = eventsservice.New(cfg.Events, cfg.Clock)
		if err != nil {
			return nil, err
		}
	}
	var adminControlSvc *admincontrolservice.Service
	if cfg.AdminControl != nil {
		adminControlSvc, err = admincontrolservice.New(cfg.AdminControl, nil)
		if err != nil {
			return nil, err
		}
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	limit := cfg.BatchLimit
	if limit <= 0 {
		limit = defaultBatchLimit
	}
	maxBatch := cfg.MaxBatchesPerRun
	if maxBatch <= 0 {
		maxBatch = defaultMaxBatches
	}
	return &Worker{
		billing:      billingSvc,
		users:        usersSvc,
		audit:        auditSvc,
		events:       eventsSvc,
		adminControl: adminControlSvc,
		logger:       logger,
		interval:     interval,
		limit:        limit,
		maxBatch:     maxBatch,
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
	aggregate := billingcontract.ChargePendingUsageResult{}
	for batch := 0; batch < w.maxBatch; batch++ {
		if err := ctx.Err(); err != nil {
			return aggregate, err
		}
		result, err := w.billing.ChargePendingUsage(ctx, billingcontract.ChargePendingUsageRequest{Limit: w.limit})
		if err != nil {
			return aggregate, err
		}
		aggregate.Selected += result.Selected
		aggregate.Charged += result.Charged
		aggregate.Batches = append(aggregate.Batches, result.Batches...)
		if result.Selected == 0 || result.Selected < w.limit || result.Charged == 0 {
			break
		}
	}
	if err := w.handleDisabledUsers(ctx, aggregate.Batches); err != nil && !errors.Is(err, context.Canceled) {
		return aggregate, err
	}
	if err := w.enqueueBalanceLowNotifications(ctx, aggregate.Batches); err != nil && !errors.Is(err, context.Canceled) {
		return aggregate, err
	}
	return aggregate, nil
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

func (w *Worker) enqueueBalanceLowNotifications(ctx context.Context, batches []billingcontract.ChargeUsageResult) error {
	if w == nil || w.events == nil || w.users == nil {
		return nil
	}
	settings := w.balanceLowNotificationSettings(ctx)
	if !settings.enabled || settings.threshold.Sign() <= 0 {
		return nil
	}
	for _, batch := range batches {
		if !crossedBalanceThreshold(batch.BalanceBefore, batch.BalanceAfter, settings.threshold) {
			continue
		}
		user, err := w.users.FindByID(ctx, batch.UserID)
		if err != nil {
			if errors.Is(err, usersservice.ErrUserNotFound) {
				continue
			}
			return err
		}
		if user.Status != userscontract.StatusActive {
			continue
		}
		_, err = w.events.Enqueue(ctx, eventscontract.EnqueueRequest{
			EventType:      notificationscontract.EventBalanceLowTriggered,
			ProducerModule: "billing",
			AggregateType:  "user",
			AggregateID:    strconv.Itoa(batch.UserID),
			IdempotencyKey: "balance_low:" + strconv.Itoa(batch.UserID) + ":" + batch.LedgerEntry.ReferenceID,
			Payload: map[string]any{
				"recipient_user_id":    batch.UserID,
				"recipient_email_hash": notificationEmailHash(user.Email),
				"balance_before":       batch.BalanceBefore,
				"balance_after":        batch.BalanceAfter,
				"threshold":            settings.threshold.FloatString(8),
				"currency":             user.Currency,
				"ledger_entry_id":      batch.LedgerEntry.ID,
				"usage_log_ids":        batch.ChargedUsageLogIDs,
				"charged_at":           batch.LedgerEntry.CreatedAt.UTC().Format(time.RFC3339Nano),
				"recharge_url":         settings.rechargeURL,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type balanceLowNotificationSettings struct {
	enabled     bool
	threshold   *big.Rat
	rechargeURL string
}

func (w *Worker) balanceLowNotificationSettings(ctx context.Context) balanceLowNotificationSettings {
	settings := balanceLowNotificationSettings{
		enabled:   true,
		threshold: new(big.Rat).SetInt64(5),
	}
	if w.adminControl == nil {
		return settings
	}
	adminSettings, err := w.adminControl.GetAdminSettings(ctx)
	if err != nil {
		w.logger.Warn("failed to read admin settings for balance notification", "error", err)
		return balanceLowNotificationSettings{enabled: false, threshold: new(big.Rat)}
	}
	if adminSettings.Email.BalanceLowNotifyEnabled != nil {
		settings.enabled = *adminSettings.Email.BalanceLowNotifyEnabled
	}
	if threshold, ok := parseMoneyRat(adminSettings.Email.BalanceLowNotifyThreshold); ok {
		settings.threshold = threshold
	}
	settings.rechargeURL = adminSettings.Email.BalanceLowNotifyRechargeURL
	return settings
}

func crossedBalanceThreshold(before, after string, threshold *big.Rat) bool {
	if threshold == nil || threshold.Sign() <= 0 {
		return false
	}
	beforeRat, ok := parseMoneyRat(before)
	if !ok {
		return false
	}
	afterRat, ok := parseMoneyRat(after)
	if !ok {
		return false
	}
	return beforeRat.Cmp(threshold) >= 0 && afterRat.Cmp(threshold) < 0
}

func parseMoneyRat(value string) (*big.Rat, bool) {
	rat, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, false
	}
	return rat, true
}

func notificationEmailHash(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:])
}
