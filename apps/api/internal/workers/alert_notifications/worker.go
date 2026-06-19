package alertnotifications

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 30 * time.Second
	defaultTimeout       = 30 * time.Second
	defaultBatchLimit    = 20
	shutdownPollInterval = 10 * time.Millisecond
)

type SMTPConfig struct {
	PublicBaseURL string
	Host          string
	Port          int
	Username      string
	Password      string
	From          string
	FromName      string
	UseTLS        bool
}

type Config struct {
	Interval      time.Duration
	Timeout       time.Duration
	BatchLimit    int
	PublicBaseURL string
	SMTP          SMTPConfig
	EmailSender   notificationscontract.EmailSender
	Clock         service.Clock
	RunGuard      runonceguard.Guard
}

type Worker struct {
	operations  *service.Service
	logger      *slog.Logger
	interval    time.Duration
	timeout     time.Duration
	batchLimit  int
	baseURL     string
	emailSender notificationscontract.EmailSender
	guard       runonceguard.Guard

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
	batchLimit := cfg.BatchLimit
	if batchLimit <= 0 {
		batchLimit = defaultBatchLimit
	}
	baseURL := strings.TrimSpace(cfg.PublicBaseURL)
	if baseURL == "" {
		baseURL = cfg.SMTP.PublicBaseURL
	}
	emailSender := cfg.EmailSender
	if emailSender == nil {
		emailSender = notificationsservice.NewSMTPSender(notificationscontract.EmailConfig{
			PublicBaseURL: cfg.SMTP.PublicBaseURL,
			SMTPHost:      cfg.SMTP.Host,
			SMTPPort:      cfg.SMTP.Port,
			SMTPUsername:  cfg.SMTP.Username,
			SMTPPassword:  cfg.SMTP.Password,
			SMTPFrom:      cfg.SMTP.From,
			SMTPFromName:  cfg.SMTP.FromName,
			SMTPUseTLS:    cfg.SMTP.UseTLS,
		})
	}
	return &Worker{
		operations:  operations,
		logger:      logger,
		interval:    durationOrDefault(cfg.Interval, defaultInterval),
		timeout:     durationOrDefault(cfg.Timeout, defaultTimeout),
		batchLimit:  batchLimit,
		baseURL:     strings.TrimRight(baseURL, "/"),
		emailSender: emailSender,
		guard:       cfg.RunGuard,
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
				w.logger.Error("worker panicked; goroutine stopped", "worker", "alert_notifications", "panic", r, "stack", string(debug.Stack()))
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

func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w == nil {
		return 0, nil
	}
	var delivered int
	_, err := runonceguard.Run(ctx, w.guard, "alert_notifications", func(runCtx context.Context) error {
		var runErr error
		delivered, runErr = w.dispatch(runCtx)
		return runErr
	})
	return delivered, err
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
	delivered, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("alert notification dispatch failed", "error", err)
		return
	}
	if delivered > 0 {
		w.logger.Info("alert notification dispatch completed", "delivered", delivered)
	}
}

func (w *Worker) dispatch(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	items, err := w.operations.ListDueNotificationDeliveries(runCtx, w.batchLimit)
	if err != nil {
		return 0, err
	}
	var delivered int
	for _, item := range items {
		if item.Channel.Type != contract.NotificationChannelTypeEmail {
			continue
		}
		err := w.emailSender.Send(runCtx, notificationscontract.EmailMessage{
			To:      item.Delivery.Target,
			Subject: alertEmailSubject(item),
			HTML:    w.alertEmailHTML(item),
		})
		if err != nil {
			if _, markErr := w.operations.MarkNotificationDeliveryFailed(runCtx, item.Delivery, err.Error()); markErr != nil {
				return delivered, markErr
			}
			continue
		}
		if _, err := w.operations.MarkNotificationDeliveryDelivered(runCtx, item.Delivery); err != nil {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

func alertEmailSubject(item contract.DueDelivery) string {
	status := strings.ToUpper(string(item.Alert.Status))
	severity := strings.ToUpper(string(item.Alert.Severity))
	return fmt.Sprintf("[SRapi %s %s] %s", severity, status, item.Alert.Summary)
}

func (w *Worker) alertEmailHTML(item contract.DueDelivery) string {
	link := w.alertLink(item.Alert.ID)
	rows := []string{
		fmt.Sprintf("<p><strong>%s</strong></p>", html.EscapeString(item.Alert.Summary)),
		"<table>",
		fmt.Sprintf("<tr><td>Severity</td><td>%s</td></tr>", html.EscapeString(string(item.Alert.Severity))),
		fmt.Sprintf("<tr><td>Status</td><td>%s</td></tr>", html.EscapeString(string(item.Alert.Status))),
		fmt.Sprintf("<tr><td>Rule</td><td>%s</td></tr>", html.EscapeString(item.Alert.RuleID)),
		fmt.Sprintf("<tr><td>Fingerprint</td><td>%s</td></tr>", html.EscapeString(item.Alert.Fingerprint)),
		fmt.Sprintf("<tr><td>Started</td><td>%s</td></tr>", html.EscapeString(item.Alert.StartedAt.Format(time.RFC3339))),
		"</table>",
	}
	if link != "" {
		rows = append(rows, fmt.Sprintf(`<p><a href="%s">Open alert in AdminOps</a></p>`, html.EscapeString(link)))
	}
	return strings.Join(rows, "")
}

func (w *Worker) alertLink(alertID int) string {
	if w.baseURL == "" || alertID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/admin/ops/alert-events?alert_id=%d", w.baseURL, alertID)
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
