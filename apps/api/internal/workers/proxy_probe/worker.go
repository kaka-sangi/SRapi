// Package proxyprobe periodically dials each active proxy through to a known
// probe URL and folds the outcome into the proxy's rolling
// success/failure counters. The counters reset every ~7 days inside
// accountservice.RecordProxyProbe, giving the admin Availability column a
// rolling-window percentage without a separate snapshot table.
//
// The worker is intentionally tiny — it just iterates proxies, runs a probe
// for each, and calls RecordProxyProbe. The dialing logic and the rolling
// window are owned by the accounts service so the operator-initiated
// "probe now" handler can share the exact same code path.
package proxyprobe

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 6 * time.Hour
	defaultTimeout       = 8 * time.Second
	defaultMaxConcurrent = 4
	defaultProbeURL      = "https://www.cloudflare.com/cdn-cgi/trace"
	shutdownPollInterval = 10 * time.Millisecond
)

// Config controls the proxy probe worker. Disabled by default; producers must
// opt-in via Enabled=true so unattended deployments do not start hitting
// outbound URLs without explicit consent.
type Config struct {
	Enabled       bool
	Interval      time.Duration
	Timeout       time.Duration
	MaxConcurrent int
	ProbeURL      string
	MasterKey     string
	Clock         accountservice.Clock
	RunGuard      runonceguard.Guard
}

// Result summarizes one probe pass.
type Result struct {
	Selected int
	Probed   int
	Skipped  int
	OK       int
	Failed   int
}

// Worker periodically probes each active proxy and folds the outcome into the
// proxy's rolling counters via accounts.RecordProxyProbe.
type Worker struct {
	accounts      *accountservice.Service
	logger        *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	maxConcurrent int
	probeURL      string
	guard         runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New wires the worker from a raw accounts store. Returns nil, nil when
// Enabled=false so the wire-up in app.go can stay one-line.
func New(accounts accountcontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if accounts == nil {
		return nil, accountservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	svc, err := accountservice.New(accounts, cfg.MasterKey, cfg.Clock)
	if err != nil {
		return nil, err
	}
	probeURL := cfg.ProbeURL
	if probeURL == "" {
		probeURL = defaultProbeURL
	}
	return &Worker{
		accounts:      svc,
		logger:        logger,
		interval:      durationOrDefault(cfg.Interval, defaultInterval),
		timeout:       durationOrDefault(cfg.Timeout, defaultTimeout),
		maxConcurrent: positiveOrDefault(cfg.MaxConcurrent, defaultMaxConcurrent),
		probeURL:      probeURL,
		guard:         cfg.RunGuard,
	}, nil
}

// Start begins the periodic probe loop in a background goroutine.
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
				w.logger.Error("worker panicked; goroutine stopped", "worker", "proxy_probe", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		w.run(ctx)
	}()
}

// Shutdown stops the probe loop and waits for the in-flight pass to drain or
// for ctx to expire, whichever comes first.
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

// RunOnce executes one probe pass immediately and reports the outcome counts.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, "proxy_probe", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.probePass(runCtx)
		return runErr
	})
	return result, err
}

func (w *Worker) run(ctx context.Context) {
	w.runAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runAndLog(ctx)
		}
	}
}

func (w *Worker) runAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("scheduled proxy probe failed", "error", err)
	}
	if result.Probed > 0 || result.Failed > 0 {
		w.logger.Debug(
			"scheduled proxy probe completed",
			"selected", result.Selected,
			"probed", result.Probed,
			"skipped", result.Skipped,
			"ok", result.OK,
			"failed", result.Failed,
		)
	}
}

func (w *Worker) probePass(ctx context.Context) (Result, error) {
	proxies, err := w.accounts.ListProxies(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{Selected: len(proxies)}
	sem := make(chan struct{}, w.maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, proxy := range proxies {
		proxy := proxy
		if proxy.Status != accountcontract.ProxyStatusActive {
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
					w.logger.Error("worker panicked; goroutine stopped", "worker", "proxy_probe", "proxy_id", proxy.ID, "panic", r, "stack", string(debug.Stack()))
				}
			}()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			ok, latency := w.probeOne(ctx, proxy)
			if _, err := w.accounts.RecordProxyProbe(ctx, proxy.ID, ok, latency); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Debug("record proxy probe failed", "proxy_id", proxy.ID, "error", err)
			}
			mu.Lock()
			defer mu.Unlock()
			result.Probed++
			if ok {
				result.OK++
			} else {
				result.Failed++
			}
		}()
	}
	wg.Wait()
	return result, nil
}

// probeOne dials probeURL through the given proxy with a hard timeout and
// reports (ok, latencyMs). It deliberately re-implements only the dialing
// half of accountservice.TestProxy — the service's TestProxy also persists a
// _last_test metadata snapshot, which would race with RecordProxyProbe's
// counter updates if we called it from inside the worker.
func (w *Worker) probeOne(parent context.Context, proxy accountcontract.ProxyDefinition) (bool, int) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	urls, err := w.accounts.ResolveProxyURL(ctx, proxyIDPtr(proxy.ID))
	if err != nil || urls == nil {
		return false, 0
	}
	parsedProxy, err := url.Parse(*urls)
	if err != nil || parsedProxy.Scheme == "" || parsedProxy.Host == "" {
		return false, 0
	}
	transport := &http.Transport{Proxy: http.ProxyURL(parsedProxy)}
	client := &http.Client{Transport: transport, Timeout: w.timeout}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.probeURL, nil)
	if err != nil {
		return false, 0
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(start) / time.Millisecond)
	if err != nil {
		return false, latency
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300, latency
}

func proxyIDPtr(id int) *string {
	s := intToString(id)
	return &s
}

func intToString(id int) string {
	// Avoid pulling in strconv just for one Itoa; the proxy IDs are small ints.
	if id == 0 {
		return "0"
	}
	negative := false
	if id < 0 {
		negative = true
		id = -id
	}
	var buf [20]byte
	pos := len(buf)
	for id > 0 {
		pos--
		buf[pos] = byte('0' + id%10)
		id /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
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
