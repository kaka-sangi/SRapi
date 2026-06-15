package litellmpricing

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = 12 * time.Hour
	defaultTimeout       = 15 * time.Second
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval  time.Duration
	Timeout   time.Duration
	SourceURL string
	Client    *http.Client
	RunGuard  runonceguard.Guard
}

type Result struct {
	Fetched  int
	Upserted int
	Skipped  int
}

type Worker struct {
	pricing   *billingservice.Service
	store     billingcontract.PricingStore
	client    *http.Client
	logger    *slog.Logger
	sourceURL string
	interval  time.Duration
	guard     runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store billingcontract.PricingStore, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil || strings.TrimSpace(cfg.SourceURL) == "" {
		return nil, billingservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	pricing, err := billingservice.NewPricing(store, nil)
	if err != nil {
		return nil, err
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: durationOrDefault(cfg.Timeout, defaultTimeout)}
	}
	return &Worker{
		pricing:   pricing,
		store:     store,
		client:    client,
		logger:    logger,
		sourceURL: strings.TrimSpace(cfg.SourceURL),
		interval:  durationOrDefault(cfg.Interval, defaultInterval),
		guard:     cfg.RunGuard,
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
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("worker panicked; goroutine stopped", "worker", "litellm_pricing", "panic", r, "stack", string(debug.Stack()))
			}
		}()
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

func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	if w == nil {
		return Result{}, nil
	}
	var result Result
	_, err := runonceguard.Run(ctx, w.guard, "litellm_pricing", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.sync(runCtx)
		return runErr
	})
	return result, err
}

func (w *Worker) run(ctx context.Context) {
	w.syncAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.syncAndLog(ctx)
		}
	}
}

func (w *Worker) syncAndLog(ctx context.Context) {
	result, err := w.RunOnce(ctx)
	if err != nil {
		w.logger.Warn("litellm pricing sync failed", "error", err)
		return
	}
	w.logger.Info("litellm pricing sync completed", "fetched", result.Fetched, "upserted", result.Upserted, "skipped", result.Skipped)
}

func (w *Worker) sync(ctx context.Context) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.sourceURL, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, errors.New("litellm pricing source returned status " + resp.Status)
	}
	var payload map[string]litellmModelPrice
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Result{}, err
	}
	return w.upsert(ctx, payload)
}

type litellmModelPrice struct {
	InputCostPerToken      litellmDecimal `json:"input_cost_per_token"`
	OutputCostPerToken     litellmDecimal `json:"output_cost_per_token"`
	CacheReadCostPerToken  litellmDecimal `json:"cache_read_input_token_cost"`
	CacheWriteCostPerToken litellmDecimal `json:"cache_creation_input_token_cost"`
}

func (w *Worker) upsert(ctx context.Context, payload map[string]litellmModelPrice) (Result, error) {
	result := Result{Fetched: len(payload)}
	existing, err := w.pricing.ListPricingRules(ctx)
	if err != nil {
		return result, err
	}
	byFamily := map[string]billingcontract.PricingRule{}
	for _, rule := range existing {
		if rule.ModelID == 0 && strings.TrimSpace(rule.ModelFamily) != "" {
			byFamily[normalizeFamily(rule.ModelFamily)] = rule
		}
	}
	for family, item := range payload {
		rule, ok := pricingRuleFromLiteLLM(family, item)
		if !ok {
			result.Skipped++
			continue
		}
		if current, found := byFamily[normalizeFamily(rule.ModelFamily)]; found {
			updated, err := w.pricing.UpdatePricingRule(ctx, current.ID, billingcontract.UpdatePricingRuleRequest{
				InputPricePerMillionTokens:      &rule.InputPricePerMillionTokens,
				OutputPricePerMillionTokens:     &rule.OutputPricePerMillionTokens,
				CacheReadPricePerMillionTokens:  &rule.CacheReadPricePerMillionTokens,
				CacheWritePricePerMillionTokens: &rule.CacheWritePricePerMillionTokens,
				Currency:                        &rule.Currency,
			})
			if err != nil {
				return result, err
			}
			byFamily[normalizeFamily(rule.ModelFamily)] = updated
		} else {
			created, err := w.store.CreatePricingRule(ctx, rule)
			if err != nil {
				return result, err
			}
			byFamily[normalizeFamily(rule.ModelFamily)] = created
		}
		result.Upserted++
	}
	return result, nil
}

func pricingRuleFromLiteLLM(family string, item litellmModelPrice) (billingcontract.PricingRule, bool) {
	family = normalizeFamily(family)
	input := perTokenToPerMillion(item.InputCostPerToken.String())
	output := perTokenToPerMillion(item.OutputCostPerToken.String())
	if family == "" || (input == "0.00000000" && output == "0.00000000") {
		return billingcontract.PricingRule{}, false
	}
	return billingcontract.PricingRule{
		ModelID:                         0,
		ModelFamily:                     family,
		ProviderID:                      0,
		BillingMode:                     billingcontract.BillingModeToken,
		InputPricePerMillionTokens:      input,
		OutputPricePerMillionTokens:     output,
		CacheReadPricePerMillionTokens:  perTokenToPerMillion(item.CacheReadCostPerToken.String()),
		CacheWritePricePerMillionTokens: perTokenToPerMillion(item.CacheWriteCostPerToken.String()),
		Currency:                        "USD",
	}, true
}

type litellmDecimal string

func (v *litellmDecimal) UnmarshalJSON(raw []byte) error {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		*v = ""
		return nil
	}
	var quoted string
	if strings.HasPrefix(value, "\"") {
		if err := json.Unmarshal(raw, &quoted); err != nil {
			return err
		}
		value = quoted
	}
	*v = litellmDecimal(strings.TrimSpace(value))
	return nil
}

func (v litellmDecimal) String() string {
	return string(v)
}

func perTokenToPerMillion(value string) string {
	amount, ok := money.DecimalRat(value)
	if !ok {
		return money.ZeroAmount
	}
	amount.Mul(amount, big.NewRat(1_000_000, 1))
	return money.FormatRatFixed(amount, 8)
}

func normalizeFamily(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func durationOrDefault(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
