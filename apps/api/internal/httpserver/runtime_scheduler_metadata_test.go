package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	qualitymemory "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	qualityevalworker "github.com/srapi/srapi/apps/api/internal/workers/quality_eval"
)

func TestSchedulerRuntimeMetadataParsesHealthQuotaLatency(t *testing.T) {
	metadata := map[string]any{
		"health_score":        json.Number("0.87"),
		"remaining_ratio":     "0.42",
		"latency_p95_ms":      1250.9,
		"current_concurrency": json.Number("2"),
		"rpm_used":            "7.5",
		"tpm_used":            uint16(64),
		"max_concurrency":     "4",
		"rpm_limit":           json.Number("30"),
		"tpm_limit":           float64(1000),
	}

	state := schedulerRuntimeState(metadata)
	assertFloatPtrNear(t, state.HealthScore, 0.87)
	assertFloatPtrNear(t, state.QuotaRemainingRatio, 0.42)
	assertIntPtr(t, state.LatencyP95MS, 1250)
	if state.CurrentConcurrency != 2 || state.RPMUsed != 7 || state.TPMUsed != 64 {
		t.Fatalf("unexpected runtime counters: %+v", state)
	}

	limits := schedulerRuntimeLimits(metadata)
	assertIntPtr(t, limits.MaxConcurrency, 4)
	assertIntPtr(t, limits.RPMLimit, 30)
	assertIntPtr(t, limits.TPMLimit, 1000)

	aliasState := schedulerRuntimeState(map[string]any{
		"quota_remaining_ratio": 0.75,
		"p95_latency_ms":        "2400",
	})
	assertFloatPtrNear(t, aliasState.QuotaRemainingRatio, 0.75)
	assertIntPtr(t, aliasState.LatencyP95MS, 2400)

	exhaustedState := schedulerRuntimeState(map[string]any{"remaining_ratio": 0})
	if !exhaustedState.QuotaExhausted {
		t.Fatalf("expected zero remaining_ratio to mark quota exhausted, got %+v", exhaustedState)
	}
}

func TestGatewaySchedulerScoresUseAccountRuntimeMetadata(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"metadata-provider","display_name":"Metadata Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"metadata-model","display_name":"Metadata Model","status":"active"}`)
	mappingBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"metadata-upstream","status":"active"}`
	_ = mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), mappingBody)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"metadata-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active","metadata":{"health_score":0.9,"remaining_ratio":0.4,"latency_p95_ms":2500}}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"metadata-model","messages":[{"role":"user","content":"metadata score"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=metadata-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one metadata-model decision, got %d", len(decisionsResp.Data))
	}

	accountID, err := strconv.Atoi(string(accountResp.Data.Id))
	if err != nil {
		t.Fatalf("parse account id: %v", err)
	}
	score := schedulerDecisionScore(t, decisionsResp.Data[0].Scores, accountID)
	assertNumberNear(t, score["health_score"], 0.9)
	assertNumberNear(t, score["quota_score"], 0.7)
	assertNumberNear(t, score["latency_score"], 0.75)
}

func TestGatewaySchedulerScoresUseQualityEvaluationAggregate(t *testing.T) {
	qualityStore := qualitymemory.New()
	upstream := openAIChatCompletionTestServer(t)
	handler := New(config.Load(), nil, WithQualityEvalStore(qualityStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerA := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"quality-a-provider","display_name":"Quality A","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	providerB := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"quality-b-provider","display_name":"Quality B","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"quality-route-model","display_name":"Quality Route Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerA.Data.Id)+`","upstream_model_name":"quality-a-upstream","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerB.Data.Id)+`","upstream_model_name":"quality-b-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerA.Data.Id)+`","name":"quality-a-account","runtime_class":"api_key","credential":{"api_key":"a-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.6},"status":"active"}`)
	accountB := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerB.Data.Id)+`","name":"quality-b-account","runtime_class":"api_key","credential":{"api_key":"b-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.6},"status":"active"}`)

	accountID, err := strconv.Atoi(string(accountB.Data.Id))
	if err != nil {
		t.Fatalf("parse account id: %v", err)
	}
	providerID, err := strconv.Atoi(string(providerB.Data.Id))
	if err != nil {
		t.Fatalf("parse provider id: %v", err)
	}
	if _, _, err := qualityStore.CreateEvaluation(t.Context(), qualitycontract.Evaluation{
		FeedbackID:        100,
		RequestID:         "req_quality_seed",
		DecisionID:        200,
		AttemptNo:         1,
		AccountID:         accountID,
		ProviderID:        providerID,
		Model:             "quality-route-model",
		SourceEndpoint:    "/v1/chat/completions",
		SampleRequestHash: "sha256:0001",
		JudgeModel:        "fake-judge",
		Score:             0.95,
		Rubric:            map[string]any{"correctness": 5},
		JudgedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed quality evaluation: %v", err)
	}

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"quality-route-model","messages":[{"role":"user","content":"quality route"}]}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=quality-route-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one quality-route-model decision, got %d", len(decisionsResp.Data))
	}
	score := schedulerDecisionScore(t, decisionsResp.Data[0].Scores, accountID)
	assertNumberNear(t, score["quality_score"], 0.95)
}

func TestGatewayCapturesQualitySampleWhenEnabled(t *testing.T) {
	qualityStore := qualitymemory.New()
	upstream := openAIChatCompletionTestServer(t)
	cfg := config.Load()
	cfg.QualityEval.Enabled = true
	cfg.QualityEval.OpenAIAPIKey = "not-used-by-http-runtime"
	handler := New(cfg, nil, WithQualityEvalStore(qualityStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"quality-capture-provider","display_name":"Quality Capture","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"quality-capture-model","display_name":"Quality Capture Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"quality-capture-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"quality-capture-account","runtime_class":"api_key","credential":{"api_key":"capture-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"quality-capture-model","messages":[{"role":"user","content":"quality capture prompt"}]}`)

	samples, err := qualityStore.ListPendingSamples(t.Context(), qualitycontract.PendingSampleFilter{SamplePercent: 100, Limit: 10, Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("list pending quality samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected one pending quality sample, got %+v", samples)
	}
	if samples[0].SamplePayloadCiphertext == "" || strings.Contains(samples[0].SamplePayloadCiphertext, "quality capture prompt") {
		t.Fatalf("expected encrypted quality sample payload, got %+v", samples[0])
	}
}

func TestQualityEvalSmokeCapturesEvaluatesAndFeedsScheduler(t *testing.T) {
	qualityStore := qualitymemory.New()
	upstream := openAIChatCompletionTestServer(t)
	cfg := config.Load()
	cfg.QualityEval.Enabled = true
	cfg.QualityEval.OpenAIAPIKey = "not-used-by-smoke"
	handler := New(cfg, nil, WithQualityEvalStore(qualityStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"quality-smoke-provider","display_name":"Quality Smoke","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"quality-smoke-model","display_name":"Quality Smoke Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"quality-smoke-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"quality-smoke-account","runtime_class":"api_key","credential":{"api_key":"quality-smoke-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"quality-smoke-model","messages":[{"role":"user","content":"quality smoke capture"}]}`)

	samples, err := qualityStore.ListPendingSamples(t.Context(), qualitycontract.PendingSampleFilter{SamplePercent: 100, Limit: 10, Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("list pending quality samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected one pending quality sample, got %+v", samples)
	}

	worker, err := qualityevalworker.New(qualityStore, slog.New(slog.NewTextHandler(io.Discard, nil)), qualityevalworker.Config{
		MasterKey:     cfg.Security.MasterKey,
		SamplePercent: 100,
		Judge: qualitySmokeJudge{
			model: "quality-smoke-judge",
			score: 0.93,
		},
	})
	if err != nil {
		t.Fatalf("create quality eval worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run quality eval worker: %v", err)
	}
	if result.Selected != 1 || result.Evaluated != 1 || result.Failed != 0 {
		t.Fatalf("unexpected quality eval worker result: %+v", result)
	}

	evaluations, err := qualityStore.ListEvaluations(t.Context())
	if err != nil {
		t.Fatalf("list quality evaluations: %v", err)
	}
	if len(evaluations) != 1 || evaluations[0].Score != 0.93 || evaluations[0].JudgeModel != "quality-smoke-judge" {
		t.Fatalf("expected one quality smoke evaluation, got %+v", evaluations)
	}

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"quality-smoke-model","messages":[{"role":"user","content":"quality smoke scheduler evidence"}]}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=quality-smoke-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 {
		t.Fatalf("expected two quality-smoke decisions, got %d", len(decisionsResp.Data))
	}
	accountID, err := strconv.Atoi(string(accountResp.Data.Id))
	if err != nil {
		t.Fatalf("parse account id: %v", err)
	}
	score := schedulerDecisionScore(t, decisionsResp.Data[len(decisionsResp.Data)-1].Scores, accountID)
	assertNumberNear(t, score["quality_score"], 0.93)
	assertNumberNear(t, score["quality_eval_score"], 0.93)
	if score["quality_tier"] != "premium" || intFromScoreValue(score["quality_eval_samples"]) != 1 {
		t.Fatalf("unexpected quality scheduler evidence: %+v", score)
	}
}

type qualitySmokeJudge struct {
	model string
	score float64
}

func (j qualitySmokeJudge) Evaluate(_ context.Context, sample qualitycontract.EvaluationSample) (qualitycontract.JudgeResult, error) {
	if strings.TrimSpace(sample.SanitizedPrompt) == "" || strings.TrimSpace(sample.SanitizedOutput) == "" {
		return qualitycontract.JudgeResult{}, nil
	}
	return qualitycontract.JudgeResult{
		JudgeModel:  j.model,
		Score:       j.score,
		Correctness: 5,
		Coherence:   5,
		Safety:      4,
		Rationale:   "local quality smoke",
		JudgedAt:    time.Now().UTC(),
	}, nil
}

func openAIChatCompletionTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"quality-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"quality ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func schedulerDecisionScore(t *testing.T, scores apiopenapi.JsonObject, accountID int) map[string]any {
	t.Helper()
	raw, ok := scores["account_"+strconv.Itoa(accountID)]
	if !ok {
		t.Fatalf("expected score for account %d, got %+v", accountID, scores)
	}
	score, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected object score for account %d, got %T %+v", accountID, raw, raw)
	}
	return score
}

func assertFloatPtrNear(t *testing.T, value *float64, expected float64) {
	t.Helper()
	if value == nil {
		t.Fatalf("expected float pointer %.2f, got nil", expected)
	}
	if math.Abs(*value-expected) > 0.0001 {
		t.Fatalf("expected %.4f, got %.4f", expected, *value)
	}
}

func assertIntPtr(t *testing.T, value *int, expected int) {
	t.Helper()
	if value == nil || *value != expected {
		t.Fatalf("expected int pointer %d, got %v", expected, value)
	}
}

func assertNumberNear(t *testing.T, value any, expected float64) {
	t.Helper()
	got, ok := value.(float64)
	if !ok {
		t.Fatalf("expected numeric score %.2f, got %T %+v", expected, value, value)
	}
	if math.Abs(got-expected) > 0.0001 {
		t.Fatalf("expected %.4f, got %.4f", expected, got)
	}
}

func intFromScoreValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}
