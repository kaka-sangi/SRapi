package channelmonitor

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	channelmonitorsservice "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultTick          = 30 * time.Second
	defaultTimeout       = 15 * time.Second
	shutdownPollInterval = 10 * time.Millisecond
)

// Config controls the channel-monitor scheduler worker.
type Config struct {
	Tick      time.Duration
	Timeout   time.Duration
	MasterKey string
	Clock     accountservice.Clock
	Adapter   channelmonitorscontract.ProbeAdapter
	RunGuard  runonceguard.Guard
	Enabled   func(context.Context) bool

	AdminControlStore admincontrolcontract.Store
}

// Worker periodically evaluates due channel monitor definitions and records
// synthetic probe runs with Trigger=scheduled.
type Worker struct {
	monitors *channelmonitorsservice.Service
	deps     channelmonitorscontract.RunnerDependencies
	logger   *slog.Logger
	tick     time.Duration
	timeout  time.Duration
	clock    func() time.Time
	guard    runonceguard.Guard
	enabled  func(context.Context) bool

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New wires the worker from raw stores.
func New(accounts accountcontract.Store, providers providercontract.Store, models modelcontract.Store, monitors channelmonitorscontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if accounts == nil || providers == nil || models == nil || monitors == nil {
		return nil, channelmonitorsservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	accountsSvc, err := accountservice.New(accounts, cfg.MasterKey, cfg.Clock)
	if err != nil {
		return nil, err
	}
	providersSvc, err := providerservice.New(providers, nil)
	if err != nil {
		return nil, err
	}
	modelsSvc, err := modelservice.New(models, nil)
	if err != nil {
		return nil, err
	}
	monitorsSvc, err := channelmonitorsservice.New(monitors)
	if err != nil {
		return nil, err
	}
	adapter := cfg.Adapter
	if adapter == nil {
		client := &http.Client{Timeout: durationOrDefault(cfg.Timeout, defaultTimeout)}
		svc, err := provideradapterservice.New(client)
		if err != nil {
			return nil, err
		}
		adapter = svc
	}
	clock := time.Now
	if cfg.Clock != nil {
		clock = func() time.Time { return cfg.Clock.Now() }
	}
	enabled := cfg.Enabled
	if enabled == nil && cfg.AdminControlStore != nil {
		adminControl, err := admincontrolservice.New(cfg.AdminControlStore, nil)
		if err != nil {
			return nil, err
		}
		enabled = func(ctx context.Context) bool {
			settings, err := adminControl.GetAdminSettings(ctx)
			if err != nil {
				return false
			}
			return settings.Features.ChannelMonitoringEnabled
		}
	}
	return &Worker{
		monitors: monitorsSvc,
		deps: channelmonitorscontract.RunnerDependencies{
			Accounts:  accountsSvc,
			Providers: providersSvc,
			Models:    modelsSvc,
			Adapter:   adapter,
		},
		logger:  logger,
		tick:    durationOrDefault(cfg.Tick, defaultTick),
		timeout: durationOrDefault(cfg.Timeout, defaultTimeout),
		clock:   clock,
		guard:   cfg.RunGuard,
		enabled: enabled,
	}, nil
}

// Start begins the background monitor evaluation loop.
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
				w.logger.Error("worker panicked; goroutine stopped", "worker", "channel_monitor", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		w.run(ctx)
	}()
}

// Shutdown stops the background monitor evaluation loop.
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

// RunOnce evaluates due monitors immediately.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w == nil {
		return 0, nil
	}
	var ran int
	_, err := runonceguard.Run(ctx, w.guard, "channel_monitor", func(runCtx context.Context) error {
		var runErr error
		ran, runErr = w.evaluate(runCtx)
		return runErr
	})
	return ran, err
}

func (w *Worker) run(ctx context.Context) {
	w.evaluateAndLog(ctx)
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.evaluateAndLog(ctx)
		}
	}
}

func (w *Worker) evaluateAndLog(ctx context.Context) {
	ran, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("channel monitor evaluation failed", "error", err)
	}
	if ran > 0 {
		w.logger.Debug("channel monitor evaluation completed", "ran", ran)
	}
}

func (w *Worker) evaluate(ctx context.Context) (int, error) {
	if w.enabled != nil && !w.enabled(ctx) {
		return 0, nil
	}
	runCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	runs, err := w.monitors.RunDue(runCtx, w.deps, w.clock().UTC(), 0)
	return len(runs), err
}

func durationOrDefault(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
