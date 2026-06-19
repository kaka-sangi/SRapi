package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// TestGatewayCountTokensFallsBackToEstimateOnOpenAIUpstream proves that a
// count_tokens call routed to an upstream with no token-count surface
// (openai-compatible) returns a local estimate instead of a hard failure —
// Claude Code calls count_tokens for context compaction and breaks on errors.
func TestGatewayCountTokensFallsBackToEstimateOnOpenAIUpstream(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"openai-count-provider","display_name":"OpenAI Count Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"anthropic_count_tokens":true,"token_counting":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"openai-count-model","display_name":"OpenAI Count Model","status":"active","capabilities":[{"key":"token_counting","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-count-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"openai-count-account","runtime_class":"api_key","credential":{"api_key":"openai-count-secret"},"metadata":{"base_url":"https://api.openai.com/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	body := `{"model":"openai-count-model","system":"count only","messages":[{"role":"user","content":"count this prompt with several words in it"}]}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages/count_tokens", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected estimated count_tokens 200 on openai-compatible upstream, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode count_tokens response: %v", err)
	}
	if resp.InputTokens <= 0 {
		t.Fatalf("expected positive estimated input_tokens, got %d", resp.InputTokens)
	}
}
