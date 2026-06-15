package qualityeval

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	qualityservice "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/service"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = time.Hour
	defaultTimeout       = 30 * time.Second
	defaultBatchLimit    = 100
	defaultSamplePercent = 1.0
	shutdownPollInterval = 10 * time.Millisecond
)

type Config struct {
	Interval      time.Duration
	Timeout       time.Duration
	BatchLimit    int
	SamplePercent float64
	MasterKey     string
	Clock         qualityservice.Clock
	Judge         qualitycontract.Judge
	OpenAIAPIKey  string
	OpenAIBaseURL string
	JudgeModel    string
	JudgeTimeout  time.Duration
	RunGuard      runonceguard.Guard
}

type Result struct {
	Selected  int
	Evaluated int
	Skipped   int
	Failed    int
}

type Worker struct {
	quality       *qualityservice.Service
	judge         qualitycontract.Judge
	logger        *slog.Logger
	interval      time.Duration
	timeout       time.Duration
	batchLimit    int
	samplePercent float64
	guard         runonceguard.Guard

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(store qualitycontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	if store == nil {
		return nil, qualityservice.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	qualitySvc, err := qualityservice.New(store, cfg.MasterKey, cfg.Clock)
	if err != nil {
		return nil, err
	}
	judge := cfg.Judge
	if judge == nil && cfg.OpenAIAPIKey != "" {
		judge, err = qualityservice.NewOpenAIJudge(qualityservice.OpenAIJudgeConfig{
			APIKey:  cfg.OpenAIAPIKey,
			BaseURL: cfg.OpenAIBaseURL,
			Model:   cfg.JudgeModel,
			Timeout: cfg.JudgeTimeout,
		})
		if err != nil {
			return nil, err
		}
	}
	if judge == nil {
		return nil, qualityservice.ErrUnavailable
	}
	return &Worker{
		quality:       qualitySvc,
		judge:         judge,
		logger:        logger,
		interval:      durationOrDefault(cfg.Interval, defaultInterval),
		timeout:       durationOrDefault(cfg.Timeout, defaultTimeout),
		batchLimit:    positiveOrDefault(cfg.BatchLimit, defaultBatchLimit),
		samplePercent: samplePercentOrDefault(cfg.SamplePercent),
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
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("worker panicked; goroutine stopped", "worker", "quality_eval", "panic", r, "stack", string(debug.Stack()))
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
	_, err := runonceguard.Run(ctx, w.guard, "quality_eval", func(runCtx context.Context) error {
		var runErr error
		result, runErr = w.evaluatePending(runCtx)
		return runErr
	})
	return result, err
}

func (w *Worker) evaluatePending(ctx context.Context) (Result, error) {
	samples, err := w.quality.ListPendingSamples(ctx, qualitycontract.PendingSampleFilter{
		SamplePercent: w.samplePercent,
		Limit:         w.batchLimit,
		Now:           time.Now().UTC(),
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{Selected: len(samples)}
	var firstErr error
	for _, sample := range samples {
		if err := w.evaluateOne(ctx, sample); err != nil {
			result.Failed++
			firstErr = errors.Join(firstErr, err)
			continue
		}
		result.Evaluated++
	}
	return result, firstErr
}

func (w *Worker) run(ctx context.Context) {
	w.evaluateAndLog(ctx)
	ticker := time.NewTicker(w.interval)
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
	result, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("quality evaluation failed", "error", err)
	}
	if result.Selected > 0 || result.Failed > 0 {
		w.logger.Info("quality evaluations completed", "selected", result.Selected, "evaluated", result.Evaluated, "failed", result.Failed)
	}
}

func (w *Worker) evaluateOne(parent context.Context, sample qualitycontract.Sample) error {
	ctx, cancel := context.WithTimeout(parent, w.timeout)
	defer cancel()
	evalSample, err := w.quality.EvaluationSample(sample)
	if err != nil {
		return err
	}
	result, err := w.judge.Evaluate(ctx, evalSample)
	if err != nil {
		return err
	}
	_, _, err = w.quality.RecordEvaluation(ctx, sample, result)
	return err
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

func samplePercentOrDefault(value float64) float64 {
	if value <= 0 {
		return defaultSamplePercent
	}
	if value > 100 {
		return 100
	}
	return value
}
