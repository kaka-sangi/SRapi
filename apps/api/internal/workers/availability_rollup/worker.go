package availabilityrollup

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	healthrollupscontract "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
	healthrollupsservice "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 24 * time.Hour
	defaultWindowDays    = 7
	defaultSnapshotLimit = 2000
	shutdownPollInterval = 10 * time.Millisecond
)

// Config controls periodic account availability rollup materialization.
type Config struct {
	Interval      time.Duration
	WindowDays    int
	SnapshotLimit int
	MasterKey     string
	Clock         accountservice.Clock
	RunGuard      runonceguard.Guard
}

// Result summarizes one availability rollup pass.
type Result struct {
	Selected  int
	Skipped   int
	Refreshed int
	Rollups   int
}

// Worker periodically materializes account availability rollups from health snapshots.
type Worker struct {
	accounts      *accountservice.Service
	rollups       *healthrollupsservice.Service
	logger        *slog.Logger
	clock         accountservice.Clock
	interval      time.Duration
	windowDays    int
	snapshotLimit int
	guard         runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New builds an availability rollup worker from account and rollup stores.
func New(accounts accountcontract.Store, rollups healthrollupscontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if accounts == nil || rollups == nil {
		return nil, accountservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	accountsSvc, err := accountservice.New(accounts, cfg.MasterKey, cfg.Clock)
	if err != nil {
		return nil, err
	}
	rollupsSvc, err := healthrollupsservice.New(rollups)
	if err != nil {
		return nil, err
	}
	return &Worker{
		accounts:      accountsSvc,
		rollups:       rollupsSvc,
		logger:        logger,
		clock:         clockOrDefault(cfg.Clock),
		interval:      durationOrDefault(cfg.Interval, defaultInterval),
		windowDays:    positiveOrDefault(cfg.WindowDays, defaultWindowDays),
		snapshotLimit: positiveOrDefault(cfg.SnapshotLimit, defaultSnapshotLimit),
		guard:         cfg.RunGuard,
	}, nil
}

// Start begins the background rollup loop.
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

// Shutdown stops the background rollup loop and waits for it to exit.
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

// RunOnce materializes rollups for every active provider account.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, "availability_rollup", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.rollupPass(runCtx)
		return runErr
	})
	return result, err
}

func (w *Worker) rollupPass(ctx context.Context) (Result, error) {
	accounts, err := w.accounts.List(ctx)
	if err != nil {
		return Result{}, err
	}
	now := w.clock.Now().UTC()
	since := startOfUTCDay(now.AddDate(0, 0, -(w.windowDays - 1)))
	result := Result{Selected: len(accounts)}
	for _, account := range accounts {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if account.Status != accountcontract.StatusActive {
			result.Skipped++
			continue
		}
		snapshots, err := w.accounts.ListHealthSnapshotsByAccount(ctx, account.ID, w.snapshotLimit)
		if err != nil {
			return result, err
		}
		samples := healthSnapshotsToSamples(snapshots, since)
		if len(samples) == 0 {
			result.Skipped++
			continue
		}
		rollups, err := w.rollups.RefreshAccount(ctx, account.ID, samples, now)
		if err != nil {
			return result, err
		}
		result.Refreshed++
		result.Rollups += len(rollups)
	}
	return result, nil
}

func (w *Worker) run(ctx context.Context) {
	w.rollupAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.rollupAndLog(ctx)
		}
	}
}

func (w *Worker) rollupAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("availability rollup failed", "error", err)
		return
	}
	if result.Refreshed > 0 {
		w.logger.Info("availability rollup completed", "selected", result.Selected, "refreshed", result.Refreshed, "rollups", result.Rollups)
	}
}

func healthSnapshotsToSamples(snapshots []accountcontract.AccountHealthSnapshot, since time.Time) []healthrollupscontract.Sample {
	samples := make([]healthrollupscontract.Sample, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.SnapshotAt.Before(since) {
			continue
		}
		samples = append(samples, healthrollupscontract.Sample{
			ProviderID:  snapshot.ProviderID,
			Healthy:     strings.EqualFold(snapshot.Status, "healthy"),
			SuccessRate: snapshot.SuccessRate,
			At:          snapshot.SnapshotAt,
		})
	}
	return samples
}

func startOfUTCDay(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func clockOrDefault(clock accountservice.Clock) accountservice.Clock {
	if clock == nil {
		return accountservice.SystemClock{}
	}
	return clock
}

func durationOrDefault(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func positiveOrDefault(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
