package quotarefresh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
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
	Refresher     reverseproxycontract.Refresher
	// BlockPrivateEgress should match the runtime reverse-proxy SSRF policy.
	BlockPrivateEgress bool
	RunGuard           runonceguard.Guard
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
	refresher     reverseproxycontract.Refresher
	logger        *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	maxConcurrent int
	guard         runonceguard.Guard

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
	refresher := cfg.Refresher
	if refresher == nil {
		svc, err := reverseproxyservice.New(nil, reverseproxyservice.WithBlockedPrivateEgress(cfg.BlockPrivateEgress))
		if err != nil {
			return nil, err
		}
		refresher = svc
	}
	return &Worker{
		accounts:      accountsSvc,
		providers:     providersSvc,
		adapter:       adapter,
		refresher:     refresher,
		logger:        logger,
		interval:      durationOrDefault(cfg.Interval, defaultInterval),
		timeout:       durationOrDefault(cfg.Timeout, defaultTimeout),
		maxConcurrent: positiveOrDefault(cfg.MaxConcurrent, defaultMaxConcurrent),
		guard:         cfg.RunGuard,
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
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("worker panicked; goroutine stopped", "worker", "quota_refresh", "panic", r, "stack", string(debug.Stack()))
			}
		}()
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
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, "quota_refresh", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.refreshPass(runCtx)
		return runErr
	})
	return result, err
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
	result, err := w.RunOnce(ctx)
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
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					result.Failed++
					firstErr = errors.Join(firstErr, fmt.Errorf("account quota refresh panicked: %v", r))
					mu.Unlock()
					w.logger.Error("worker panicked; goroutine stopped", "worker", "quota_refresh", "account_id", account.ID, "panic", r, "stack", string(debug.Stack()))
				}
			}()
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
	runtimeAccount, err := w.accountWithRuntimeProxy(ctx, account)
	if err != nil {
		w.persistQuotaProviderError(ctx, account, err)
		mu.Lock()
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
		return
	}
	credential, err := w.accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		mu.Lock()
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
		return
	}
	if refreshed, ok, err := w.refreshCredential(ctx, runtimeAccount, credential); err != nil {
		w.persistQuotaProviderError(ctx, account, err)
		mu.Lock()
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
		return
	} else if ok {
		credential = refreshed
	}
	report, err := w.adapter.FetchAccountQuota(ctx, provideradaptercontract.ProbeRequest{
		Provider:   provider,
		Account:    runtimeAccount,
		Credential: credential,
	})
	if err != nil {
		if refreshed, retried := w.retryAfterAuthRefresh(ctx, runtimeAccount, credential, err); retried {
			credential = refreshed
			report, err = w.adapter.FetchAccountQuota(ctx, provideradaptercontract.ProbeRequest{
				Provider:   provider,
				Account:    runtimeAccount,
				Credential: credential,
			})
		}
	}
	if err != nil {
		w.persistQuotaProviderError(ctx, account, err)
		mu.Lock()
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
		return
	}
	signals := 0
	signals, err = w.accounts.ApplyQuotaReport(ctx, account, report)
	if err != nil {
		mu.Lock()
		*firstErr = errors.Join(*firstErr, err)
		mu.Unlock()
	}
	mu.Lock()
	result.Refreshed++
	result.Signals += signals
	mu.Unlock()
}

func (w *Worker) refreshCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	if !accountcontract.ShouldRefreshOAuthCredential(account, credential, time.Now().UTC()) {
		return credential, false, nil
	}
	return w.forceRefreshCredential(ctx, account, credential)
}

func (w *Worker) retryAfterAuthRefresh(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, upstreamErr error) (map[string]any, bool) {
	if account.RuntimeClass != accountcontract.RuntimeClassOauthRefresh && account.RuntimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		return nil, false
	}
	class := errorClassName(upstreamErr)
	if class != "session_invalid" && class != "auth_failed" && class != "auth_error" {
		return nil, false
	}
	if mapString(credential, "refresh_token") == "" {
		return nil, false
	}
	refreshed, ok, err := w.forceRefreshCredential(ctx, account, credential)
	if err != nil || !ok {
		return nil, false
	}
	return refreshed, true
}

func (w *Worker) forceRefreshCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	if w.refresher == nil {
		return credential, false, nil
	}
	response, err := w.refresher.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseProxyAccountRuntime(account, credential),
	})
	if err != nil {
		return credential, false, err
	}
	refreshed := response.Credential
	if _, err := w.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Credential: &refreshed}); err != nil {
		return credential, false, err
	}
	return refreshed, true, nil
}

func (w *Worker) accountWithRuntimeProxy(ctx context.Context, account accountcontract.ProviderAccount) (accountcontract.ProviderAccount, error) {
	if account.ProxyID == nil || strings.TrimSpace(*account.ProxyID) == "" {
		return account, nil
	}
	runtimeProxyURL, err := w.accounts.ResolveProxyURL(ctx, account.ProxyID)
	if err != nil {
		return accountcontract.ProviderAccount{}, provideradaptercontract.ProviderError{Class: "proxy_unavailable", StatusCode: http.StatusBadGateway, Message: "provider account proxy unavailable"}
	}
	account.ProxyID = runtimeProxyURL
	return account, nil
}

func (w *Worker) persistQuotaProviderError(ctx context.Context, account accountcontract.ProviderAccount, err error) {
	if updateErr := w.accounts.ApplyQuotaProviderError(ctx, account, err); updateErr != nil {
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

func errorClassName(err error) string {
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) && providerErr.Class != "" {
		return providerErr.Class
	}
	var runtimeErr reverseproxycontract.RuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr.Class != "" {
		return runtimeErr.Class
	}
	return "unknown"
}

func reverseProxyAccountRuntime(account accountcontract.ProviderAccount, credential map[string]any) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      account.ID,
		RuntimeClass:   string(account.RuntimeClass),
		UpstreamClient: account.UpstreamClient,
		ProxyID:        account.ProxyID,
		UserAgent:      mapString(account.Metadata, "user_agent"),
		Metadata:       account.Metadata,
		Credential:     credential,
	}
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
