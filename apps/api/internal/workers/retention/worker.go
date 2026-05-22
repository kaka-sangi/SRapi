package retention

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
	defaultInterval      = 24 * time.Hour
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval                   time.Duration
	UsageLogsDays              int
	SchedulerDecisionsDays     int
	SchedulerFeedbacksDays     int
	AuditLogsDays              int
	AccountHealthSnapshotsDays int
	Clock                      service.Clock
}

type Worker struct {
	operations *service.Service
	logger     *slog.Logger
	interval   time.Duration
	policy     contract.RetentionPolicy

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store contract.RetentionStore, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, service.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	operations, err := service.New(store, cfg.Clock)
	if err != nil {
		return nil, err
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Worker{
		operations: operations,
		logger:     logger,
		interval:   interval,
		policy:     policyFromConfig(cfg),
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

func (w *Worker) RunOnce(ctx context.Context) (contract.CleanupResult, error) {
	if w == nil {
		return contract.CleanupResult{}, nil
	}
	return w.operations.CleanupRetention(ctx, w.policy)
}

func (w *Worker) run(ctx context.Context) {
	w.cleanupAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanupAndLog(ctx)
		}
	}
}

func (w *Worker) cleanupAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("retention cleanup failed", "error", err)
		return
	}
	if totalDeleted(result) > 0 {
		w.logger.Info(
			"retention cleanup completed",
			"usage_logs", result.UsageLogs,
			"scheduler_decisions", result.SchedulerDecisions,
			"scheduler_feedbacks", result.SchedulerFeedbacks,
			"audit_logs", result.AuditLogs,
			"account_health_snapshots", result.AccountHealthSnapshots,
		)
	}
}

func totalDeleted(result contract.CleanupResult) int {
	return result.UsageLogs +
		result.SchedulerDecisions +
		result.SchedulerFeedbacks +
		result.AuditLogs +
		result.AccountHealthSnapshots
}

func policyFromConfig(cfg Config) contract.RetentionPolicy {
	return contract.RetentionPolicy{
		UsageLogs:              days(cfg.UsageLogsDays),
		SchedulerDecisions:     days(cfg.SchedulerDecisionsDays),
		SchedulerFeedbacks:     days(cfg.SchedulerFeedbacksDays),
		AuditLogs:              days(cfg.AuditLogsDays),
		AccountHealthSnapshots: days(cfg.AccountHealthSnapshotsDays),
	}
}

func days(value int) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value) * 24 * time.Hour
}
