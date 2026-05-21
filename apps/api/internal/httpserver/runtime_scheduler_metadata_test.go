package httpserver

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
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
