package qualityeval_test

import (
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	qualityservice "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/service"
	qualitystore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/qualityeval"

	_ "github.com/mattn/go-sqlite3"
)

func TestStorePersistsSamplesAndAggregatesEvaluations(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/quality-eval.db?_fk=1")
	defer client.Close()

	store, err := qualitystore.New(client)
	if err != nil {
		t.Fatalf("new quality eval store: %v", err)
	}
	svc, err := qualityservice.New(store, "quality_eval_master_key_32_bytes_min", nil)
	if err != nil {
		t.Fatalf("new quality service: %v", err)
	}
	now := time.Date(2026, 5, 25, 2, 0, 0, 0, time.UTC)
	sample, created, err := svc.CaptureSample(t.Context(), qualitycontract.CaptureSampleRequest{
		FeedbackID:      1,
		RequestID:       "req_quality_store",
		DecisionID:      2,
		AttemptNo:       1,
		AccountID:       3,
		ProviderID:      4,
		Model:           "quality-model",
		SourceEndpoint:  "/v1/chat/completions",
		SanitizedPrompt: "question",
		SanitizedOutput: "answer",
		CapturedAt:      now,
	})
	if err != nil || !created {
		t.Fatalf("capture sample: sample=%+v created=%v err=%v", sample, created, err)
	}
	duplicate, created, err := svc.CaptureSample(t.Context(), qualitycontract.CaptureSampleRequest{
		FeedbackID:      1,
		RequestID:       "req_quality_store",
		DecisionID:      2,
		AttemptNo:       1,
		AccountID:       3,
		ProviderID:      4,
		Model:           "quality-model",
		SourceEndpoint:  "/v1/chat/completions",
		SanitizedPrompt: "question",
		SanitizedOutput: "answer",
		CapturedAt:      now,
	})
	if err != nil || created || duplicate.ID != sample.ID {
		t.Fatalf("expected idempotent sample, duplicate=%+v created=%v err=%v", duplicate, created, err)
	}

	pending, err := svc.ListPendingSamples(t.Context(), qualitycontract.PendingSampleFilter{SamplePercent: 100, Limit: 10})
	if err != nil {
		t.Fatalf("list pending samples: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != sample.ID {
		t.Fatalf("expected pending sample, got %+v", pending)
	}
	if _, created, err := svc.RecordEvaluation(t.Context(), sample, qualitycontract.JudgeResult{
		JudgeModel:  "fake-judge",
		Score:       0.7,
		Correctness: 4,
		Coherence:   4,
		Safety:      3,
		JudgedAt:    now.Add(time.Minute),
	}); err != nil || !created {
		t.Fatalf("record evaluation: created=%v err=%v", created, err)
	}
	aggregate, err := svc.AggregateScore(t.Context(), 3, "quality-model")
	if err != nil {
		t.Fatalf("aggregate score: %v", err)
	}
	if aggregate.SampleCount != 1 || aggregate.Score != 0.7 {
		t.Fatalf("unexpected aggregate: %+v", aggregate)
	}
}
