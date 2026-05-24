package healthprobe

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
)

const (
	defaultInterval        = 5 * time.Minute
	defaultTimeout         = 10 * time.Second
	defaultMaxConcurrent   = 8
	shutdownPollInterval   = 10 * time.Millisecond
	defaultHistoryLimit    = 5
	defaultCooldown        = 5 * time.Minute
	defaultFailureLimit    = 3
	defaultMinErrorSamples = 3
	defaultErrorThreshold  = 0.5
)

// Config controls health probe worker scheduling, concurrency, and state thresholds.
type Config struct {
	Interval               time.Duration
	Timeout                time.Duration
	MaxConcurrent          int
	MasterKey              string
	Clock                  accountservice.Clock
	FailureThreshold       int
	ErrorRateThreshold     float32
	MinSamplesForErrorRate int
	Cooldown               time.Duration
	ProbePolicy            accountcontract.AccountProbePolicy
	Adapter                provideradaptercontract.ProbeAdapter
}

// Result summarizes one health probe worker pass.
type Result struct {
	Selected  int
	Probed    int
	Skipped   int
	Failed    int
	Unhealthy int
}

// Worker periodically probes active API-key provider accounts and records health snapshots.
type Worker struct {
	accounts      *accountservice.Service
	providers     *providerservice.Service
	adapter       provideradaptercontract.ProbeAdapter
	logger        *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	maxConcurrent int
	policy        accountcontract.AccountProbePolicy

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New builds a health probe worker from account and provider stores.
func New(accounts accountcontract.Store, providers providercontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if accounts == nil || providers == nil {
		return nil, accountservice.ErrInvalidInput
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
	adapter := cfg.Adapter
	if adapter == nil {
		client := &http.Client{Timeout: durationOrDefault(cfg.Timeout, defaultTimeout)}
		adapter, err = provideradapterservice.New(client)
		if err != nil {
			return nil, err
		}
	}
	return &Worker{
		accounts:      accountsSvc,
		providers:     providersSvc,
		adapter:       adapter,
		logger:        logger,
		interval:      durationOrDefault(cfg.Interval, defaultInterval),
		timeout:       durationOrDefault(cfg.Timeout, defaultTimeout),
		maxConcurrent: positiveOrDefault(cfg.MaxConcurrent, defaultMaxConcurrent),
		policy:        normalizePolicy(cfg),
	}, nil
}

// Start begins the background probe loop.
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

// Shutdown stops the background probe loop and waits for it to exit.
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

// RunOnce executes one probe pass.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	accounts, err := w.accounts.List(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{Selected: len(accounts)}
	sem := make(chan struct{}, w.maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, account := range accounts {
		account := account
		if !probeEligible(account) {
			mu.Lock()
			result.Skipped++
			mu.Unlock()
			continue
		}
		provider, err := w.providers.FindByID(ctx, account.ProviderID)
		if err != nil {
			mu.Lock()
			result.Failed++
			firstErr = errors.Join(firstErr, err)
			mu.Unlock()
			continue
		}
		select {
		case <-ctx.Done():
			mu.Lock()
			firstErr = errors.Join(firstErr, ctx.Err())
			mu.Unlock()
			continue
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.probeOne(ctx, account, provider, &mu, &result, &firstErr)
		}()
	}
	wg.Wait()
	return result, firstErr
}

func (w *Worker) run(ctx context.Context) {
	w.probeAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.probeAndLog(ctx)
		}
	}
}

func (w *Worker) probeAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("account health probe failed", "error", err)
	}
	if result.Probed > 0 || result.Failed > 0 {
		w.logger.Debug("account health probes completed", "selected", result.Selected, "probed", result.Probed, "skipped", result.Skipped, "failed", result.Failed, "unhealthy", result.Unhealthy)
	}
}

func (w *Worker) probeOne(parent context.Context, account accountcontract.ProviderAccount, provider providercontract.Provider, mu *sync.Mutex, result *Result, firstErr *error) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	snapshot, _, err := w.accounts.ProbeAccount(ctx, account.ID, adapterProber{adapter: w.adapter, provider: provider}, w.policy)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		return
	}
	result.Probed++
	if snapshot.CircuitState == "open" {
		result.Unhealthy++
	}
}

type adapterProber struct {
	adapter  provideradaptercontract.ProbeAdapter
	provider providercontract.Provider
}

func (p adapterProber) ProbeAccount(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (accountcontract.AccountProbeResult, error) {
	resp, err := p.adapter.ProbeAccount(ctx, provideradaptercontract.ProbeRequest{
		Provider:   p.provider,
		Account:    account,
		Credential: credential,
	})
	if err != nil {
		return accountcontract.AccountProbeResult{}, err
	}
	return accountcontract.AccountProbeResult{
		OK:         resp.OK,
		ErrorClass: resp.ErrorClass,
		StatusCode: resp.StatusCode,
		LatencyMS:  resp.LatencyMS,
		Metadata:   resp.Metadata,
	}, nil
}

func probeEligible(account accountcontract.ProviderAccount) bool {
	return account.Status == accountcontract.StatusActive && account.RuntimeClass == accountcontract.RuntimeClassAPIKey
}

func normalizePolicy(cfg Config) accountcontract.AccountProbePolicy {
	policy := cfg.ProbePolicy
	if policy.HistoryLimit <= 0 {
		policy.HistoryLimit = defaultHistoryLimit
	}
	if cfg.FailureThreshold > 0 {
		policy.FailureThreshold = cfg.FailureThreshold
	}
	if policy.FailureThreshold <= 0 {
		policy.FailureThreshold = defaultFailureLimit
	}
	if cfg.ErrorRateThreshold > 0 {
		policy.ErrorRateThreshold = cfg.ErrorRateThreshold
	}
	if policy.ErrorRateThreshold <= 0 {
		policy.ErrorRateThreshold = defaultErrorThreshold
	}
	if cfg.MinSamplesForErrorRate > 0 {
		policy.MinSamplesForErrorRate = cfg.MinSamplesForErrorRate
	}
	if policy.MinSamplesForErrorRate <= 0 {
		policy.MinSamplesForErrorRate = defaultMinErrorSamples
	}
	if cfg.Cooldown > 0 {
		policy.Cooldown = cfg.Cooldown
	}
	if policy.Cooldown <= 0 {
		policy.Cooldown = defaultCooldown
	}
	return policy
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
