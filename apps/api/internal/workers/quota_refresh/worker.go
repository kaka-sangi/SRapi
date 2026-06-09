package quotarefresh

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
	defaultInterval      = 30 * time.Minute
	defaultTimeout       = 15 * time.Second
	defaultMaxConcurrent = 4
	shutdownPollInterval = 10 * time.Millisecond
)

// Config controls the scheduled per-account quota/subscription refresh worker.
type Config struct {
	Interval      time.Duration
	Timeout       time.Duration
	MaxConcurrent int
	MasterKey     string
	Clock         accountservice.Clock
	Adapter       provideradaptercontract.AccountQuotaFetcher
}

// Result summarizes one refresh pass.
type Result struct {
	Selected  int
	Refreshed int
	Skipped   int
	Failed    int
	Signals   int
}

// Worker periodically refreshes provider quota/subscription standing for accounts
// that expose a quota endpoint, persisting the resulting quota snapshots.
type Worker struct {
	accounts      *accountservice.Service
	providers     *providerservice.Service
	adapter       provideradaptercontract.AccountQuotaFetcher
	logger        *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	maxConcurrent int

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New builds a quota refresh worker from account and provider stores.
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
		svc, err := provideradapterservice.New(client)
		if err != nil {
			return nil, err
		}
		adapter = svc
	}
	return &Worker{
		accounts:      accountsSvc,
		providers:     providersSvc,
		adapter:       adapter,
		logger:        logger,
		interval:      durationOrDefault(cfg.Interval, defaultInterval),
		timeout:       durationOrDefault(cfg.Timeout, defaultTimeout),
		maxConcurrent: positiveOrDefault(cfg.MaxConcurrent, defaultMaxConcurrent),
	}, nil
}

// Start begins the background refresh loop.
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

// Shutdown stops the background refresh loop and waits for it to exit.
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

// RunOnce executes one refresh pass immediately.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	return w.refreshPass(ctx)
}

func (w *Worker) run(ctx context.Context) {
	w.refreshAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.refreshAndLog(ctx)
		}
	}
}

func (w *Worker) refreshAndLog(ctx context.Context) {
	result, err := w.refreshPass(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("account quota refresh failed", "error", err)
	}
	if result.Refreshed > 0 || result.Failed > 0 {
		w.logger.Debug("account quota refresh completed", "selected", result.Selected, "refreshed", result.Refreshed, "skipped", result.Skipped, "failed", result.Failed, "signals", result.Signals)
	}
}

func (w *Worker) refreshPass(ctx context.Context) (Result, error) {
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
		if account.Status != accountcontract.StatusActive {
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
		if !w.adapter.QuotaConfigured(provideradaptercontract.ProbeRequest{Provider: provider, Account: account}) {
			mu.Lock()
			result.Skipped++
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				mu.Lock()
				firstErr = errors.Join(firstErr, ctx.Err())
				mu.Unlock()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			w.refreshOne(ctx, account, provider, &mu, &result, &firstErr)
		}()
	}
	wg.Wait()
	return result, firstErr
}

func (w *Worker) refreshOne(parent context.Context, account accountcontract.ProviderAccount, provider providercontract.Provider, mu *sync.Mutex, result *Result, firstErr *error) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	credential, err := w.accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		mu.Lock()
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
		return
	}
	report, err := w.adapter.FetchAccountQuota(ctx, provideradaptercontract.ProbeRequest{
		Provider:   provider,
		Account:    account,
		Credential: credential,
	})
	if err != nil {
		w.persistQuotaProviderError(ctx, account, err)
		mu.Lock()
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
		return
	}
	signals := 0
	for _, signal := range report.QuotaSignals {
		snapshotAt := signal.SnapshotAt
		if snapshotAt.IsZero() {
			snapshotAt = time.Now().UTC()
		}
		if _, err := w.accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
			AccountID:      account.ID,
			ProviderID:     account.ProviderID,
			QuotaType:      signal.QuotaType,
			Remaining:      signal.Remaining,
			Used:           signal.Used,
			QuotaLimit:     signal.QuotaLimit,
			RemainingRatio: signal.RemainingRatio,
			ResetAt:        signal.ResetAt,
			SnapshotAt:     snapshotAt,
		}); err != nil {
			mu.Lock()
			*firstErr = errors.Join(*firstErr, err)
			mu.Unlock()
			break
		}
		signals++
	}
	mu.Lock()
	result.Refreshed++
	result.Signals += signals
	mu.Unlock()
}

func (w *Worker) persistQuotaProviderError(ctx context.Context, account accountcontract.ProviderAccount, err error) {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return
	}
	if providerErr.StatusCode != http.StatusForbidden {
		return
	}
	metadata := provideradaptercontract.QuotaErrorMetadata(account.Metadata, providerErr, time.Now().UTC())
	status := account.Status
	if provideradaptercontract.QuotaErrorClassRequiresOperatorAction(providerErr.Class) {
		status = accountcontract.StatusSuspended
	}
	if _, updateErr := w.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata, Status: &status}); updateErr != nil {
		w.logger.Warn("failed to persist account quota error metadata", "account_id", account.ID, "error", updateErr)
	}
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
