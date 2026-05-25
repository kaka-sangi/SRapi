package service

import (
	"strings"
	"testing"
	"time"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	qualitymemory "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/store/memory"
)

func TestCaptureSampleEncryptsPayloadAndAggregatesEvaluation(t *testing.T) {
	store := qualitymemory.New()
	svc, err := New(store, "quality_eval_master_key_32_bytes_min", fixedClock{now: time.Date(2026, 5, 25, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new quality service: %v", err)
	}

	sample, created, err := svc.CaptureSample(t.Context(), qualitycontract.CaptureSampleRequest{
		FeedbackID:      10,
		RequestID:       "req_quality_sample",
		DecisionID:      20,
		AttemptNo:       1,
		AccountID:       30,
		ProviderID:      40,
		Model:           "quality-model",
		SourceEndpoint:  "/v1/chat/completions",
		SanitizedPrompt: "Explain retries",
		SanitizedOutput: "Retries repeat transient failures safely.",
	})
	if err != nil || !created {
		t.Fatalf("capture sample: sample=%+v created=%v err=%v", sample, created, err)
	}
	if sample.SampleRequestHash == "" || sample.SamplePayloadCiphertext == "" || strings.Contains(sample.SamplePayloadCiphertext, "Explain retries") {
		t.Fatalf("expected hashed encrypted sample, got %+v", sample)
	}
	evalSample, err := svc.EvaluationSample(sample)
	if err != nil {
		t.Fatalf("decrypt evaluation sample: %v", err)
	}
	if evalSample.SanitizedPrompt != "Explain retries" || evalSample.SanitizedOutput == "" {
		t.Fatalf("expected decrypted sanitized sample, got %+v", evalSample)
	}

	evaluation, created, err := svc.RecordEvaluation(t.Context(), sample, qualitycontract.JudgeResult{
		JudgeModel:  "judge-model",
		Score:       0.80,
		Correctness: 5,
		Coherence:   4,
		Safety:      3,
		Rationale:   "clear answer",
	})
	if err != nil || !created {
		t.Fatalf("record evaluation: evaluation=%+v created=%v err=%v", evaluation, created, err)
	}
	aggregate, err := svc.AggregateScore(t.Context(), 30, "quality-model")
	if err != nil {
		t.Fatalf("aggregate score: %v", err)
	}
	if aggregate.SampleCount != 1 || aggregate.Score != 0.80 {
		t.Fatalf("unexpected aggregate: %+v", aggregate)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
