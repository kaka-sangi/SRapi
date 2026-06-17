// Package accountstokenrefresh runs a periodic OAuth-refresh sweep over
// provider accounts so a slipping access token is refreshed BEFORE it
// expires (rather than after the gateway sees its first 401). The worker
// only touches accounts whose runtime_class is oauth_refresh or
// oauth_device_code, whose status is active, and whose token_expires_at
// falls inside the refresh window (default: now + 5 minutes).
//
// Failure policy lives on the accounts service (RefreshAccessToken):
//   - Any error whose message matches the permanent-error regex
//     (invalid_grant / invalid_client / unauthorized_client / invalid_token /
//     access_denied / consent_required / login_required) flips the account
//     into needs_reauth_at immediately.
//   - Otherwise the failure counter (refresh_attempts) increments; at 5
//     consecutive failures the account also flips into needs_reauth_at so
//     the worker stops hammering the upstream.
//   - A successful refresh zeros refresh_attempts, clears needs_reauth_at,
//     snapshots token_expires_at from the new credential, and stamps
//     last_refreshed_at.
//
// The worker reuses the reverse_proxy.Refresher injected at construction
// (the same one the gateway uses to perform an inline refresh on a 401),
// adapted to the local AccountRefresher interface so the accounts module
// keeps its one-way dependency on the reverse-proxy contract.
package accountstokenrefresh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval         = 5 * time.Minute
	defaultRefreshThreshold = 5 * time.Minute
	defaultMaxConcurrent    = 4
	defaultTimeout          = 30 * time.Second
	shutdownPollInterval    = 10 * time.Millisecond
	workerName              = "accounts_token_refresh"
)

// Config controls the proactive token-refresh worker.
type Config struct {
	Interval         time.Duration
	RefreshThreshold time.Duration
	MaxConcurrent    int
	Timeout          time.Duration
	MasterKey        string
	Refresher        reverseproxycontract.Refresher
	// BlockPrivateEgress mirrors the runtime reverse-proxy SSRF policy so the
	// adapter the worker builds (when Refresher is nil) refuses to call back
	// into the local network.
	BlockPrivateEgress bool
	RunGuard           runonceguard.Guard
}

// Result summarises one refresh pass.
type Result struct {
	Selected  int
	Refreshed int
	Skipped   int
	Failed    int
}

// Worker is the background refresh loop.
type Worker struct {
	accounts         *accountservice.Service
	refresher        reverseproxycontract.Refresher
	logger           *slog.Logger
	interval         time.Duration
	refreshThreshold time.Duration
	maxConcurrent    int
	timeout          time.Duration
	guard            runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs the worker. accounts is required; refresher defaults to a
// fresh reverse_proxy service when nil.
func New(accounts accountcontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if accounts == nil {
		return nil, accountservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	accountsSvc, err := accountservice.New(accounts, cfg.MasterKey, nil)
	if err != nil {
		return nil, err
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
		accounts:         accountsSvc,
		refresher:        refresher,
		logger:           logger,
		interval:         durationOrDefault(cfg.Interval, defaultInterval),
		refreshThreshold: durationOrDefault(cfg.RefreshThreshold, defaultRefreshThreshold),
		maxConcurrent:    positiveOrDefault(cfg.MaxConcurrent, defaultMaxConcurrent),
		timeout:          durationOrDefault(cfg.Timeout, defaultTimeout),
		guard:            cfg.RunGuard,
	}, nil
}

// Start begins the background refresh loop. Idempotent.
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
				w.logger.Error("worker panicked; goroutine stopped", "worker", workerName, "panic", r, "stack", string(debug.Stack()))
			}
		}()
		w.run(ctx)
	}()
}

// Shutdown stops the background loop and waits for it to exit.
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

// RunOnce performs a single refresh pass; useful for tests and the optional
// leader-guard handoff.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, workerName, func(runCtx context.Context) error {
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
		w.logger.Warn("token refresh pass failed", "worker", workerName, "error", err)
	}
	if result.Refreshed > 0 || result.Failed > 0 {
		w.logger.Info("token refresh pass completed",
			"worker", workerName,
			"selected", result.Selected,
			"refreshed", result.Refreshed,
			"skipped", result.Skipped,
			"failed", result.Failed,
		)
	}
}

func (w *Worker) refreshPass(ctx context.Context) (Result, error) {
	accounts, err := w.accounts.List(ctx)
	if err != nil {
		return Result{}, err
	}
	now := time.Now().UTC()
	deadline := now.Add(w.refreshThreshold)
	var due []accountcontract.ProviderAccount
	for _, account := range accounts {
		if !w.eligibleForRefresh(account, deadline) {
			continue
		}
		due = append(due, account)
	}
	result := Result{Selected: len(due), Skipped: len(accounts) - len(due)}
	if len(due) == 0 {
		return result, nil
	}
	sem := make(chan struct{}, w.maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	adapter := refresherAdapter{refresher: w.refresher}
	for _, account := range due {
		account := account
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					result.Failed++
					firstErr = errors.Join(firstErr, fmt.Errorf("token refresh panicked: %v", r))
					mu.Unlock()
					w.logger.Error("worker panicked; goroutine stopped", "worker", workerName, "account_id", account.ID, "panic", r, "stack", string(debug.Stack()))
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
			refreshCtx, cancel := context.WithTimeout(ctx, w.timeout)
			defer cancel()
			if _, err := w.accounts.RefreshAccessToken(refreshCtx, account.ID, adapter); err != nil {
				mu.Lock()
				result.Failed++
				firstErr = errors.Join(firstErr, fmt.Errorf("account %d: %w", account.ID, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			result.Refreshed++
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result, firstErr
}

// eligibleForRefresh decides whether an account should be touched on this pass.
// We refresh active oauth-style accounts that are NOT already flagged as
// needs_reauth (manual intervention required), AND whose token expires within
// the refresh window. Accounts with no token_expires_at are skipped — they
// haven't been refreshed yet (so we don't know when to act) and the gateway's
// on-demand refresh path remains responsible for those.
func (w *Worker) eligibleForRefresh(account accountcontract.ProviderAccount, deadline time.Time) bool {
	if account.Status != accountcontract.StatusActive {
		return false
	}
	if account.RuntimeClass != accountcontract.RuntimeClassOauthRefresh && account.RuntimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		return false
	}
	if account.NeedsReauthAt != nil {
		return false
	}
	if account.TokenExpiresAt == nil {
		return false
	}
	return !account.TokenExpiresAt.After(deadline)
}

// refresherAdapter bridges the reverse_proxy refresher into the accounts
// service's AccountRefresher interface so the cross-module dependency stays
// one-way (accounts has no idea reverse_proxy exists).
type refresherAdapter struct {
	refresher reverseproxycontract.Refresher
}

func (a refresherAdapter) RefreshAccount(ctx context.Context, req accountservice.RefreshRequest) (accountservice.RefreshResult, error) {
	resp, err := a.refresher.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.AccountID,
			RuntimeClass:   string(req.RuntimeClass),
			UpstreamClient: req.UpstreamClient,
			ProxyID:        req.ProxyID,
			Metadata:       req.Metadata,
			Credential:     req.Credential,
		},
	})
	if err != nil {
		return accountservice.RefreshResult{}, err
	}
	return accountservice.RefreshResult{Credential: resp.Credential}, nil
}

// NewRefresherAdapter is exported so non-worker callers (HTTP handler, app
// wiring) can adapt the reverse_proxy refresher when invoking
// accountservice.RefreshAccessToken directly.
func NewRefresherAdapter(refresher reverseproxycontract.Refresher) accountservice.AccountRefresher {
	return refresherAdapter{refresher: refresher}
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
