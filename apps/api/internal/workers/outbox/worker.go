package outbox

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/events/service"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultConsumerName  = "outbox-dispatcher"
	defaultInterval      = 2 * time.Second
	defaultLimit         = 100
	defaultRetryBackoff  = 30 * time.Second
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval              time.Duration
	Limit                 int
	RetryBackoff          time.Duration
	ConsumerName          string
	EventHandler          service.OutboxHandler
	DispatchClock         service.Clock
	AccountStore          accountcontract.Store
	AffiliateStore        affiliatecontract.Store
	AdminControlStore     admincontrolcontract.Store
	AuditStore            auditcontract.Store
	UserStore             userscontract.Store
	EmailConfig           notificationscontract.EmailConfig
	EmailPublicBaseURL    string
	EmailSMTPHost         string
	EmailSMTPPort         int
	EmailSMTPUsername     string
	EmailSMTPPassword     string
	EmailSMTPFrom         string
	EmailSMTPFromName     string
	EmailSMTPUseTLS       bool
	EmailSender           notificationscontract.EmailSender
	EmailTemplates        map[string]string
	EmailTemplateProvider notificationscontract.EmailTemplateProvider
	MasterKey             string
	SubscriptionStore     subscriptioncontract.Store
	UsageStore            usagecontract.Store
	RunGuard              runonceguard.Guard
}

type Worker struct {
	events       *service.Service
	handler      service.OutboxHandler
	logger       *slog.Logger
	interval     time.Duration
	limit        int
	retryBackoff time.Duration
	guard        runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store contract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, service.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	events, err := service.New(store, cfg.DispatchClock)
	if err != nil {
		return nil, err
	}
	consumerName := cfg.ConsumerName
	if consumerName == "" {
		consumerName = defaultConsumerName
	}
	handler := cfg.EventHandler
	if handler == nil {
		handler, err = defaultEventHandler(events, cfg)
		if err != nil {
			return nil, err
		}
	}
	handler = inboxHandler{
		events:       events,
		consumerName: consumerName,
		next:         handler,
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	limit := cfg.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	retryBackoff := cfg.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = defaultRetryBackoff
	}
	return &Worker{
		events:       events,
		handler:      handler,
		logger:       logger,
		interval:     interval,
		limit:        limit,
		retryBackoff: retryBackoff,
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

func (w *Worker) RunOnce(ctx context.Context) (service.DispatchResult, error) {
	if w == nil {
		return service.DispatchResult{}, nil
	}
	var result service.DispatchResult
	_, err := runonceguard.Run(ctx, w.guard, "outbox", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.events.DispatchPending(runCtx, w.handler, service.DispatchOptions{
			Limit:        w.limit,
			RetryBackoff: w.retryBackoff,
		})
		return runErr
	})
	return result, err
}

func (w *Worker) run(ctx context.Context) {
	w.dispatchAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.dispatchAndLog(ctx)
		}
	}
}

func (w *Worker) dispatchAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("domain event outbox dispatch failed", "error", err)
		return
	}
	if result.Selected > 0 {
		w.logger.Debug("domain event outbox dispatched", "selected", result.Selected, "published", result.Published, "failed", result.Failed)
	}
}

type inboxHandler struct {
	events       *service.Service
	consumerName string
	next         service.OutboxHandler
}

func (h inboxHandler) HandleOutboxEvent(ctx context.Context, event contract.OutboxEvent) error {
	record, created, err := h.events.RecordInbox(ctx, contract.RecordInboxRequest{
		EventID:      event.EventID,
		ConsumerName: h.consumerName,
		EventType:    event.EventType,
	})
	if err != nil {
		return err
	}
	if !created {
		if record.Status == contract.InboxStatusProcessed {
			return nil
		}
		if record.Status == contract.InboxStatusFailed {
			return h.handleAndMark(ctx, record, event)
		}
		return contract.ErrInboxClaimed
	}
	return h.handleAndMark(ctx, record, event)
}

func (h inboxHandler) handleAndMark(ctx context.Context, record contract.InboxRecord, event contract.OutboxEvent) error {
	if record.Status == contract.InboxStatusProcessed {
		return nil
	}
	if err := h.next.HandleOutboxEvent(ctx, event); err != nil {
		if _, markErr := h.events.MarkInboxFailed(ctx, record, err); markErr != nil {
			return markErr
		}
		return err
	}
	_, err := h.events.MarkInboxProcessed(ctx, record.ID)
	return err
}
