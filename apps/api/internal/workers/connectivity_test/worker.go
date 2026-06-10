package connectivitytest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = time.Hour
	defaultTimeout       = 30 * time.Second
	defaultMaxConcurrent = 2
	defaultHistoryLimit  = 5
	shutdownPollInterval = 10 * time.Millisecond
	probeInput           = "Respond with OK."
)

// probeModelKeys are the account-metadata / provider-config keys that select the
// model used for the connectivity probe; a configured model is the opt-in signal.
var probeModelKeys = []string{"responses_compact_probe_model", "compact_probe_model", "test_model"}

// Config controls the scheduled connectivity test worker. Disabled by default;
// it issues a real (billable) generative probe, so only accounts that configure
// a probe model are tested.
type Config struct {
	Interval      time.Duration
	Timeout       time.Duration
	MaxConcurrent int
	MasterKey     string
	Clock         accountservice.Clock
	Adapter       provideradaptercontract.ConversationAdapter
	RunGuard      runonceguard.Guard
}

// Result summarizes one connectivity test pass.
type Result struct {
	Selected  int
	Probed    int
	Skipped   int
	Failed    int
	Unhealthy int
}

// Worker periodically issues a real generative connectivity probe against
// opt-in accounts (any runtime class, including OAuth — complementing the cheap
// api_key-only health probe) and records the outcome as a health snapshot.
type Worker struct {
	accounts      *accountservice.Service
	providers     *providerservice.Service
	adapter       provideradaptercontract.ConversationAdapter
	logger        *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	maxConcurrent int
	policy        accountcontract.AccountProbePolicy
	guard         runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

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
		policy:        accountcontract.AccountProbePolicy{HistoryLimit: defaultHistoryLimit},
		guard:         cfg.RunGuard,
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

// RunOnce executes one connectivity test pass immediately.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, "connectivity_test", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.testPass(runCtx)
		return runErr
	})
	return result, err
}

func (w *Worker) run(ctx context.Context) {
	w.testAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.testAndLog(ctx)
		}
	}
}

func (w *Worker) testAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("scheduled connectivity test failed", "error", err)
	}
	if result.Probed > 0 || result.Failed > 0 {
		w.logger.Debug("scheduled connectivity test completed", "selected", result.Selected, "probed", result.Probed, "skipped", result.Skipped, "failed", result.Failed, "unhealthy", result.Unhealthy)
	}
}

func (w *Worker) testPass(ctx context.Context) (Result, error) {
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
		model := probeModel(account, provider)
		if model == "" {
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
			w.testOne(ctx, account, provider, model, &mu, &result, &firstErr)
		}()
	}
	wg.Wait()
	return result, firstErr
}

func (w *Worker) testOne(parent context.Context, account accountcontract.ProviderAccount, provider providercontract.Provider, model string, mu *sync.Mutex, result *Result, firstErr *error) {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	prober := conversationProber{adapter: w.adapter, provider: provider, model: model}
	snapshot, _, err := w.accounts.ProbeAccount(ctx, account.ID, prober, w.policy)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		result.Failed++
		*firstErr = errors.Join(*firstErr, err)
		return
	}
	result.Probed++
	if snapshot.CircuitState == "open" || !strings.EqualFold(snapshot.Status, "healthy") {
		result.Unhealthy++
	}
}

// conversationProber issues a real generative probe and reports the outcome.
// Upstream failures are returned as a not-OK result (not an error) so they are
// folded into an unhealthy snapshot rather than dropped.
type conversationProber struct {
	adapter  provideradaptercontract.ConversationAdapter
	provider providercontract.Provider
	model    string
}

func (p conversationProber) ProbeAccount(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (accountcontract.AccountProbeResult, error) {
	startedAt := time.Now()
	raw, err := json.Marshal(map[string]any{"model": p.model, "input": probeInput})
	if err != nil {
		return accountcontract.AccountProbeResult{OK: false, ErrorClass: "probe_payload_failed", StatusCode: http.StatusInternalServerError, CheckedAt: time.Now().UTC()}, nil
	}
	resp, err := p.adapter.InvokeConversation(ctx, provideradaptercontract.ConversationRequest{
		RequestID:      fmt.Sprintf("scheduled_connectivity_%d", account.ID),
		SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SourceEndpoint: string(gatewaycontract.EndpointResponsesCompact),
		TargetProtocol: p.provider.Protocol,
		Model:          p.model,
		InputParts:     []provideradaptercontract.ContentPart{{Kind: provideradaptercontract.ContentPartText, Text: probeInput}},
		RawBody:        raw,
		Provider:       p.provider,
		Account:        account,
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: p.model},
		Credential:     credential,
	})
	latency := int(time.Since(startedAt).Milliseconds())
	if err == nil {
		status := resp.StatusCode
		if status <= 0 {
			status = http.StatusOK
		}
		return accountcontract.AccountProbeResult{OK: true, StatusCode: status, LatencyMS: latency, CheckedAt: time.Now().UTC()}, nil
	}
	var providerErr provideradaptercontract.ProviderError
	status := http.StatusBadGateway
	errorClass := "provider_probe_failed"
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode > 0 {
			status = providerErr.StatusCode
		}
		if strings.TrimSpace(providerErr.Class) != "" {
			errorClass = strings.TrimSpace(providerErr.Class)
		}
	}
	return accountcontract.AccountProbeResult{OK: false, ErrorClass: errorClass, StatusCode: status, LatencyMS: latency, CheckedAt: time.Now().UTC()}, nil
}

func probeModel(account accountcontract.ProviderAccount, provider providercontract.Provider) string {
	for _, values := range []map[string]any{account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		for _, key := range probeModelKeys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if raw, ok := values[key]; ok {
		if str, ok := raw.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
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
