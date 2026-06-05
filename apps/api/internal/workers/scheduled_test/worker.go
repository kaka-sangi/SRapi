package scheduledtest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
)

const (
	defaultTick          = time.Minute
	defaultTimeout       = 30 * time.Second
	shutdownPollInterval = 10 * time.Millisecond
)

// Config controls the scheduled-test-plan worker.
type Config struct {
	Tick      time.Duration
	Timeout   time.Duration
	MasterKey string
	Clock     accountservice.Clock
	Adapter   provideradaptercontract.ConversationAdapter
}

// Worker evaluates due scheduled-test plans on a fixed tick and runs each via
// the shared Runner (reusing accounts.ProbeAccount).
type Worker struct {
	plans   *scheduledservice.Service
	runner  *Runner
	logger  *slog.Logger
	tick    time.Duration
	timeout time.Duration
	clock   func() time.Time

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New wires the worker from the raw stores.
func New(accounts accountcontract.Store, providers providercontract.Store, plansStore scheduledcontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if accounts == nil || providers == nil || plansStore == nil {
		return nil, scheduledservice.ErrInvalidInput
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
	var clockFn scheduledservice.Clock
	if cfg.Clock != nil {
		clockFn = func() time.Time { return cfg.Clock.Now() }
	}
	plansSvc, err := scheduledservice.New(plansStore, clockFn)
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
	runner, err := NewRunner(accountsSvc, providersSvc, plansSvc, RealProber(adapter))
	if err != nil {
		return nil, err
	}
	clock := time.Now
	if cfg.Clock != nil {
		clock = func() time.Time { return cfg.Clock.Now() }
	}
	return &Worker{
		plans:   plansSvc,
		runner:  runner,
		logger:  logger,
		tick:    durationOrDefault(cfg.Tick, defaultTick),
		timeout: durationOrDefault(cfg.Timeout, defaultTimeout),
		clock:   clock,
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

func (w *Worker) run(ctx context.Context) {
	w.evaluate(ctx)
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.evaluate(ctx)
		}
	}
}

// RunOnce evaluates due plans immediately and returns the number of plans run.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	if w == nil {
		return 0, nil
	}
	return w.evaluate(ctx)
}

func (w *Worker) evaluate(ctx context.Context) (int, error) {
	due, err := w.plans.DuePlans(ctx, w.clock())
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			w.logger.Warn("scheduled test due-plan selection failed", "error", err)
		}
		return 0, err
	}
	ran := 0
	for _, plan := range due {
		select {
		case <-ctx.Done():
			return ran, ctx.Err()
		default:
		}
		runCtx, cancel := context.WithTimeout(ctx, w.timeout)
		_, runErr := w.runner.RunPlan(runCtx, plan, scheduledcontract.TriggerSchedule)
		cancel()
		ran++
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			w.logger.Warn("scheduled test plan run failed", "plan_id", plan.ID, "error", runErr)
		}
	}
	return ran, nil
}

func durationOrDefault(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
