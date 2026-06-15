package accountquotaalert

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval       = time.Minute
	defaultHistoryLimit   = 20
	defaultThresholdRatio = 0.2
	shutdownPollInterval  = 10 * time.Millisecond
)

type Config struct {
	Interval     time.Duration
	HistoryLimit int
	MasterKey    string
	Clock        accountservice.Clock
	Events       eventscontract.Store
	AdminControl admincontrolcontract.Store
	RunGuard     runonceguard.Guard
}

type Result struct {
	Selected int
	Checked  int
	Enqueued int
	Skipped  int
}

type Worker struct {
	store        accountcontract.Store
	accounts     *accountservice.Service
	events       *eventsservice.Service
	adminControl *admincontrolservice.Service
	logger       *slog.Logger
	clock        accountservice.Clock
	interval     time.Duration
	historyLimit int
	guard        runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(accounts accountcontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if accounts == nil {
		return nil, accountservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	accountsSvc, err := accountservice.New(accounts, cfg.MasterKey, cfg.Clock)
	if err != nil {
		return nil, err
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
	historyLimit := cfg.HistoryLimit
	if historyLimit <= 1 {
		historyLimit = defaultHistoryLimit
	}
	return &Worker{
		store:        accounts,
		accounts:     accountsSvc,
		events:       eventsSvc,
		adminControl: adminControlSvc,
		logger:       logger,
		clock:        clockOrDefault(cfg.Clock),
		interval:     interval,
		historyLimit: historyLimit,
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
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("worker panicked; goroutine stopped", "worker", "account_quota_alert", "panic", r, "stack", string(debug.Stack()))
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

func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, "account_quota_alert", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.alertPass(runCtx)
		return runErr
	})
	return result, err
}

func (w *Worker) alertPass(ctx context.Context) (Result, error) {
	if w.events == nil {
		return Result{}, nil
	}
	settings := w.accountQuotaNotificationSettings(ctx)
	if !settings.enabled || settings.thresholdRatio <= 0 {
		return Result{}, nil
	}
	accounts, err := w.accounts.List(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{Selected: len(accounts)}
	for _, account := range accounts {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if account.Status != accountcontract.StatusActive {
			result.Skipped++
			continue
		}
		snapshots, err := w.store.ListQuotaSnapshotsByAccount(ctx, account.ID, w.historyLimit)
		if err != nil {
			return result, err
		}
		enqueued, checked, skipped, err := w.enqueueAccountQuotaAlerts(ctx, account, snapshots, settings)
		result.Enqueued += enqueued
		result.Checked += checked
		result.Skipped += skipped
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (w *Worker) run(ctx context.Context) {
	w.alertAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.alertAndLog(ctx)
		}
	}
}

func (w *Worker) alertAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("account quota alert scan failed", "error", err)
		return
	}
	if result.Enqueued > 0 {
		w.logger.Info("account quota alerts enqueued", "selected", result.Selected, "checked", result.Checked, "enqueued", result.Enqueued)
	}
}

func (w *Worker) enqueueAccountQuotaAlerts(ctx context.Context, account accountcontract.ProviderAccount, snapshots []accountcontract.AccountQuotaSnapshot, settings accountQuotaNotificationSettings) (int, int, int, error) {
	latestByType := map[string]accountcontract.AccountQuotaSnapshot{}
	previousByType := map[string]accountcontract.AccountQuotaSnapshot{}
	for _, snapshot := range snapshots {
		if accountcontract.IsSyntheticQuotaSnapshot(snapshot) {
			continue
		}
		quotaType := strings.TrimSpace(snapshot.QuotaType)
		if quotaType == "" {
			continue
		}
		if _, ok := latestByType[quotaType]; !ok {
			latestByType[quotaType] = snapshot
			continue
		}
		if _, ok := previousByType[quotaType]; !ok {
			previousByType[quotaType] = snapshot
		}
	}

	enqueued := 0
	checked := 0
	skipped := 0
	for quotaType, latest := range latestByType {
		previous, ok := previousByType[quotaType]
		if !ok {
			skipped++
			continue
		}
		if !crossedRemainingRatioThreshold(previous.RemainingRatio, latest.RemainingRatio, settings.thresholdRatio) {
			checked++
			continue
		}
		_, err := w.events.Enqueue(ctx, eventscontract.EnqueueRequest{
			EventType:      notificationscontract.EventAccountQuotaAlertTriggered,
			ProducerModule: "accounts",
			AggregateType:  "provider_account",
			AggregateID:    strconv.Itoa(account.ID),
			IdempotencyKey: accountQuotaAlertIdempotencyKey(account.ID, latest, settings.thresholdRatio),
			Payload: map[string]any{
				"account_id":               account.ID,
				"account_name":             account.Name,
				"provider_id":              account.ProviderID,
				"runtime_class":            string(account.RuntimeClass),
				"quota_snapshot_id":        latest.ID,
				"quota_type":               quotaType,
				"quota_used":               latest.Used,
				"quota_limit":              latest.QuotaLimit,
				"quota_remaining":          latest.Remaining,
				"quota_remaining_ratio":    ratioString(latest.RemainingRatio),
				"quota_threshold":          ratioString(settings.thresholdRatio),
				"previous_remaining_ratio": ratioString(previous.RemainingRatio),
				"reset_at":                 formatOptionalTime(latest.ResetAt),
				"snapshot_at":              latest.SnapshotAt.UTC().Format(time.RFC3339Nano),
				"triggered_at":             settings.now.UTC().Format(time.RFC3339Nano),
				"account_url":              "/admin/accounts/" + strconv.Itoa(account.ID),
			},
		})
		if err != nil {
			return enqueued, checked, skipped, err
		}
		enqueued++
		checked++
	}
	return enqueued, checked, skipped, nil
}

type accountQuotaNotificationSettings struct {
	enabled        bool
	thresholdRatio float32
	now            time.Time
}

func (w *Worker) accountQuotaNotificationSettings(ctx context.Context) accountQuotaNotificationSettings {
	settings := accountQuotaNotificationSettings{
		enabled:        true,
		thresholdRatio: defaultThresholdRatio,
		now:            w.clock.Now(),
	}
	if w.adminControl == nil {
		return settings
	}
	adminSettings, err := w.adminControl.GetAdminSettings(ctx)
	if err != nil {
		w.logger.Warn("failed to read admin settings for account quota notification", "error", err)
		return accountQuotaNotificationSettings{enabled: false}
	}
	if adminSettings.Email.AccountQuotaNotifyEnabled != nil {
		settings.enabled = *adminSettings.Email.AccountQuotaNotifyEnabled
	}
	if ratio, ok := parseThresholdRatio(adminSettings.Email.AccountQuotaNotifyRemainingRatio); ok {
		settings.thresholdRatio = ratio
	}
	return settings
}

func clockOrDefault(clock accountservice.Clock) accountservice.Clock {
	if clock == nil {
		return accountservice.SystemClock{}
	}
	return clock
}

func crossedRemainingRatioThreshold(before float32, after float32, threshold float32) bool {
	if threshold <= 0 {
		return false
	}
	return before > threshold && after <= threshold
}

func accountQuotaAlertIdempotencyKey(accountID int, snapshot accountcontract.AccountQuotaSnapshot, threshold float32) string {
	bucket := snapshot.SnapshotAt.UTC().Format("2006-01-02")
	if snapshot.ResetAt != nil {
		bucket = snapshot.ResetAt.UTC().Format(time.RFC3339)
	}
	return "account_quota_alert:" + strconv.Itoa(accountID) + ":" + strings.TrimSpace(snapshot.QuotaType) + ":" + ratioString(threshold) + ":" + bucket
}

func ratioString(value float32) string {
	return strconv.FormatFloat(float64(value), 'f', 8, 32)
}

func parseThresholdRatio(value string) (float32, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil || parsed <= 0 || parsed > 1 {
		return 0, false
	}
	return float32(parsed), true
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
