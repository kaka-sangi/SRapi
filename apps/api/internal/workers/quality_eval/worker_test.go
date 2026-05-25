package qualityeval

import (
	"context"
	"log/slog"
	"testing"
	"time"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	qualityservice "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/service"
	qualitymemory "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/store/memory"
)

func TestWorkerEvaluatesPendingSamples(t *testing.T) {
	store := qualitymemory.New()
	qualitySvc, err := qualityservice.New(store, "quality_eval_master_key_32_bytes_min", nil)
	if err != nil {
		t.Fatalf("new quality service: %v", err)
	}
	if _, _, err := qualitySvc.CaptureSample(t.Context(), qualitycontract.CaptureSampleRequest{
		FeedbackID:      1,
		RequestID:       "req_worker_quality",
		DecisionID:      2,
		AttemptNo:       1,
		AccountID:       3,
		ProviderID:      4,
		Model:           "quality-model",
		SourceEndpoint:  "/v1/chat/completions",
		SanitizedPrompt: "question",
		SanitizedOutput: "answer",
		CapturedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("capture sample: %v", err)
	}

	worker, err := New(store, slog.Default(), Config{
		MasterKey:     "quality_eval_master_key_32_bytes_min",
		SamplePercent: 100,
		Judge:         fakeJudge{},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker: %v", err)
	}
	if result.Selected != 1 || result.Evaluated != 1 || result.Failed != 0 {
		t.Fatalf("unexpected worker result: %+v", result)
	}
	evaluations, err := store.ListEvaluations(t.Context())
	if err != nil {
		t.Fatalf("list evaluations: %v", err)
	}
	if len(evaluations) != 1 || evaluations[0].Score != 0.8 || evaluations[0].JudgeModel != "fake-judge" {
		t.Fatalf("expected persisted fake evaluation, got %+v", evaluations)
	}
}

type fakeJudge struct{}

func (fakeJudge) Evaluate(context.Context, qualitycontract.EvaluationSample) (qualitycontract.JudgeResult, error) {
	return qualitycontract.JudgeResult{
		JudgeModel:  "fake-judge",
		Score:       0.8,
		Correctness: 4,
		Coherence:   4,
		Safety:      4,
	}, nil
}
