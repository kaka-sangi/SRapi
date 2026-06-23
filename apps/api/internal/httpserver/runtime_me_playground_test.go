package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestMePlaygroundChatBillsUser proves the 交界地 playground runs a real,
// session-authenticated gateway chat that streams AND records a usage log for
// the user (the billing path) — without any API key in the request.
func TestMePlaygroundChatBillsUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOpenAISSE(w,
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"playground-ok"},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"pg-provider","display_name":"PG","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"pg-model","display_name":"PG Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"pg-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pg-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	// Models endpoint lists the active model.
	mReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/playground/models", nil)
	mReq.AddCookie(sessionCookie)
	mRec := httptest.NewRecorder()
	handler.ServeHTTP(mRec, mReq)
	if mRec.Code != http.StatusOK {
		t.Fatalf("playground models: expected 200, got %d", mRec.Code)
	}
	var models apiopenapi.PlaygroundModelsResponse
	_ = json.NewDecoder(mRec.Body).Decode(&models)
	if len(models.Data) == 0 {
		t.Fatalf("expected at least one playground model")
	}

	// Chat: session + CSRF, no API key in the request.
	chat := httptest.NewRequest(http.MethodPost, "/api/v1/me/playground/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"model":"pg-model"}`))
	chat.AddCookie(sessionCookie)
	chat.Header.Set("X-CSRF-Token", csrf)
	chat.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, chat)
	if rec.Code != http.StatusOK {
		t.Fatalf("playground chat: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "playground-ok") {
		t.Fatalf("expected streamed content in SSE.\nbody: %s", rec.Body.String())
	}

	// Billing path: a usage log must have been recorded for the user.
	uReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=pg-model", nil)
	uReq.AddCookie(sessionCookie)
	uRec := httptest.NewRecorder()
	handler.ServeHTTP(uRec, uReq)
	if uRec.Code != http.StatusOK {
		t.Fatalf("usage-logs: expected 200, got %d", uRec.Code)
	}
	var usage apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(uRec.Body).Decode(&usage); err != nil {
		t.Fatalf("decode usage-logs: %v", err)
	}
	if usage.Pagination.Total < 1 {
		t.Fatalf("expected a usage log recorded for the playground chat, got total=%d", usage.Pagination.Total)
	}
}

func TestCurrentUserAvailableModelsReturnsChannelStatusAndPricing(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"available-provider","display_name":"Available Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"available-model","display_name":"Available Model","family":"available-family","status":"active","context_window":128000,"max_output_tokens":4096}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"available-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"available-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)
	mustCreatePricingRule(t, handler, sessionCookie, csrf, `{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"1.25","output_price_per_million_tokens":"2.50","cache_read_price_per_million_tokens":"0.10","cache_write_price_per_million_tokens":"0.20","currency":"usd"}`)

	disabledModelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"hidden-disabled-model","display_name":"Hidden Disabled","status":"disabled"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(disabledModelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"hidden-upstream","status":"active"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/available-models", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("available models: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AvailableModelListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode available models: %v", err)
	}
	var found *apiopenapi.AvailableModel
	for i := range resp.Data {
		if resp.Data[i].Id == "hidden-disabled-model" {
			t.Fatalf("disabled model should not be listed: %+v", resp.Data[i])
		}
		if resp.Data[i].Id == "available-model" {
			found = &resp.Data[i]
		}
	}
	if found == nil {
		t.Fatalf("expected available-model in response, got %+v", resp.Data)
	}
	if found.Status != apiopenapi.AvailableModelStatusAvailable || len(found.Channels) != 1 {
		t.Fatalf("unexpected model availability: %+v", *found)
	}
	channel := found.Channels[0]
	if channel.Status != apiopenapi.AvailableModelStatusAvailable {
		t.Fatalf("unexpected channel status: %+v", channel)
	}
	if channel.ActiveAccountCount != 0 || channel.TotalAccountCount != 0 {
		t.Fatalf("internal account counts must be redacted, got active=%d total=%d", channel.ActiveAccountCount, channel.TotalAccountCount)
	}
	if channel.ProviderName != "" || channel.UpstreamModel != "" {
		t.Fatalf("internal provider identity must be redacted, got name=%q upstream=%q", channel.ProviderName, channel.UpstreamModel)
	}
	if channel.Pricing.Source != apiopenapi.AvailableModelPricingSourcePricingRule ||
		channel.Pricing.Currency != "USD" ||
		channel.Pricing.InputPricePerMillionTokens != "1.25000000" ||
		channel.Pricing.OutputPricePerMillionTokens != "2.50000000" ||
		channel.Pricing.CacheReadPricePerMillionTokens != "0.10000000" ||
		channel.Pricing.CacheWritePricePerMillionTokens != "0.20000000" {
		t.Fatalf("unexpected pricing: %+v", channel.Pricing)
	}
}

// TestBuildPlaygroundChatBodyParams proves the conversation-level parameters
// (system prompt, temperature, max_tokens) reach the gateway request body, and
// that blank/zero values stay omitted.
func TestBuildPlaygroundChatBodyParams(t *testing.T) {
	system := "  be terse  "
	temperature := 0.3
	maxTokens := 256
	content := "hi"
	req := apiopenapi.PlaygroundChatRequest{
		Model:       "pg-model",
		Messages:    []apiopenapi.PlaygroundMessage{{Role: "user", Content: &content}},
		System:      &system,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	}
	rawBody, body, err := buildPlaygroundChatBody(req)
	if err != nil {
		t.Fatalf("buildPlaygroundChatBody: %v", err)
	}
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
		Temperature *float64 `json:"temperature"`
		MaxTokens   *int     `json:"max_tokens"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("unmarshal raw body: %v", err)
	}
	if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[0].Content != "be terse" {
		t.Fatalf("expected trimmed system message first, got %+v", payload.Messages)
	}
	if payload.Temperature == nil || *payload.Temperature != 0.3 {
		t.Fatalf("expected temperature 0.3, got %+v", payload.Temperature)
	}
	if payload.MaxTokens == nil || *payload.MaxTokens != 256 {
		t.Fatalf("expected max_tokens 256, got %+v", payload.MaxTokens)
	}
	if body.Model != "pg-model" {
		t.Fatalf("decoded request lost the model: %+v", body)
	}

	// Blank system and out-of-range values must stay omitted.
	blank := "   "
	badTemp := 9.9
	zeroTokens := 0
	req.System = &blank
	req.Temperature = &badTemp
	req.MaxTokens = &zeroTokens
	rawBody, _, err = buildPlaygroundChatBody(req)
	if err != nil {
		t.Fatalf("buildPlaygroundChatBody (blank): %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(rawBody, &generic); err != nil {
		t.Fatalf("unmarshal blank-case body: %v", err)
	}
	if _, ok := generic["temperature"]; ok {
		t.Fatalf("out-of-range temperature must be omitted: %s", rawBody)
	}
	if _, ok := generic["max_tokens"]; ok {
		t.Fatalf("zero max_tokens must be omitted: %s", rawBody)
	}
	messages, _ := generic["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("blank system must not add a message: %s", rawBody)
	}
}

// TestMePlaygroundChatRequiresAuth confirms the endpoint is session-gated.
func TestMePlaygroundChatRequiresAuth(t *testing.T) {
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/playground/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session: expected 401, got %d", rec.Code)
	}
}
