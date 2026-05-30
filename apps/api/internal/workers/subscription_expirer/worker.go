package subscriptionexpirer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
)

const (
	defaultInterval      = time.Hour
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval     time.Duration
	Clock        service.Clock
	Events       eventscontract.Store
	AdminControl admincontrolcontract.Store
}

type Dependencies = service.Dependencies

type Worker struct {
	subscriptions *service.Service
	adminControl  *admincontrolservice.Service
	logger        *slog.Logger
	interval      time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store contract.Store, deps Dependencies, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, service.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	if deps.Events == nil && cfg.Events != nil {
		events, err := eventsservice.New(cfg.Events, nil)
		if err != nil {
			return nil, err
		}
		deps.Events = events
	}
	var adminControl *admincontrolservice.Service
	if cfg.AdminControl != nil {
		var err error
		adminControl, err = admincontrolservice.New(cfg.AdminControl, nil)
		if err != nil {
			return nil, err
		}
	}
	subscriptions, err := service.NewWithDependencies(store, deps, cfg.Clock)
	if err != nil {
		return nil, err
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Worker{subscriptions: subscriptions, adminControl: adminControl, logger: logger, interval: interval}, nil
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

func (w *Worker) RunOnce(ctx context.Context) (contract.ExpireSubscriptionsResult, error) {
	if w == nil {
		return contract.ExpireSubscriptionsResult{}, nil
	}
	expiration, err := w.subscriptions.ExpireActiveUserSubscriptions(ctx, time.Time{})
	if err != nil {
		return expiration, err
	}
	if !w.subscriptionExpiryReminderEnabled(ctx) {
		return expiration, nil
	}
	_, err = w.subscriptions.EnqueueSubscriptionExpiryReminders(ctx, time.Time{})
	return expiration, err
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
		w.logger.Warn("subscription expiration failed", "error", err)
		return
	}
	if result.Expired > 0 {
		w.logger.Info("subscriptions expired", "selected", result.Selected, "expired", result.Expired)
	}
}

func (w *Worker) subscriptionExpiryReminderEnabled(ctx context.Context) bool {
	if w.adminControl == nil {
		return true
	}
	settings, err := w.adminControl.GetAdminSettings(ctx)
	if err != nil {
		w.logger.Warn("failed to read admin settings for subscription expiry reminder", "error", err)
		return false
	}
	if settings.Email.SubscriptionExpiryNotifyEnabled == nil {
		return true
	}
	return *settings.Email.SubscriptionExpiryNotifyEnabled
}
