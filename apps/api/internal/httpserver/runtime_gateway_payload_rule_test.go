package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	payloadrulesmemory "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestGatewayAppliesPayloadOverrideRule proves the end-to-end path: an operator
// "override" rule mutates the marshaled upstream request body before dispatch.
func TestGatewayAppliesPayloadOverrideRule(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	// Seed an override rule that forces reasoning.effort=high on every
	// openai-compatible upstream request.
	payloadStore := payloadrulesmemory.New()
	if _, err := payloadStore.CreateRule(context.Background(), payloadrulescontract.CreateRule{
		Name:          "force-high-effort",
		Enabled:       true,
		Action:        payloadrulescontract.ActionOverride,
		MatchModel:    "*",
		MatchProtocol: "openai-compatible",
		Params:        map[string]any{"reasoning.effort": "high"},
	}); err != nil {
		t.Fatalf("seed payload rule: %v", err)
	}

	handler := New(config.Load(), nil, WithPayloadRulesStore(payloadStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"transform-provider","display_name":"Transform Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"transform-model","display_name":"Transform Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"transform-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"transform-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"transform-model","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	reasoning, _ := doc["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("expected payload override reasoning.effort=high in upstream body, got %s", sent)
	}
}

func TestGatewaySkipsPayloadRulesWhenRequestShaperDisabled(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	payloadStore := payloadrulesmemory.New()
	if _, err := payloadStore.CreateRule(context.Background(), payloadrulescontract.CreateRule{
		Name:          "force-high-effort",
		Enabled:       true,
		Action:        payloadrulescontract.ActionOverride,
		MatchModel:    "*",
		MatchProtocol: "openai-compatible",
		Params:        map[string]any{"reasoning.effort": "high"},
	}); err != nil {
		t.Fatalf("seed payload rule: %v", err)
	}
	adminStore := admincontrolmemory.New()
	adminSvc, err := admincontrolservice.New(adminStore, nil)
	if err != nil {
		t.Fatalf("new admin control service: %v", err)
	}
	settings, err := adminSvc.GetAdminSettings(t.Context())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Gateway.RequestShaperEnabled = false
	if _, err := adminSvc.UpdateAdminSettings(t.Context(), settings, 1); err != nil {
		t.Fatalf("update admin settings: %v", err)
	}

	handler := New(config.Load(), nil, WithPayloadRulesStore(payloadStore), WithAdminControlStore(adminStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"shape-disabled-provider","display_name":"Shape Disabled Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"shape-disabled-model","display_name":"Shape Disabled Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"shape-disabled-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"shape-disabled-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"shape-disabled-model","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	if _, ok := doc["reasoning"]; ok {
		t.Fatalf("request shaper disabled should skip operator payload rules, got %s", sent)
	}
}

func TestGatewayChatReasoningEffortReachesGeminiThinkingConfig(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"gemini thinking ok"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"chat-to-gemini-thinking-provider","display_name":"Chat To Gemini Thinking","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"chat-to-gemini-thinking-model","display_name":"Chat To Gemini Thinking Model","status":"active","capabilities":[{"key":"reasoning_control","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"models/gemini-pro","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"chat-to-gemini-thinking-account","runtime_class":"api_key","credential":{"api_key":"gemini-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"chat-to-gemini-thinking-model","messages":[{"role":"user","content":"think"}],"reasoning_effort":"medium"}`)
	var resp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode chat completion response: %v", err)
	}
	if len(resp.Choices) != 1 || decodeChatMessageText(t, resp.Choices[0].Message.Content) != "gemini thinking ok" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	generationConfig, _ := doc["generationConfig"].(map[string]any)
	thinkingConfig, _ := generationConfig["thinkingConfig"].(map[string]any)
	if thinkingConfig["thinkingBudget"] != float64(8192) || thinkingConfig["includeThoughts"] != true {
		t.Fatalf("expected OpenAI reasoning_effort to reach Gemini thinkingConfig, got %s", sent)
	}
}

func TestGatewayGeminiThinkingConfigReachesOpenAIReasoningEffort(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"openai reasoning ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gemini-to-chat-thinking-provider","display_name":"Gemini To Chat Thinking","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-to-chat-thinking-model","display_name":"Gemini To Chat Thinking Model","status":"active","capabilities":[{"key":"reasoning_control","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-chat-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"gemini-to-chat-thinking-account","runtime_class":"api_key","credential":{"api_key":"openai-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"contents":[{"role":"user","parts":[{"text":"think"}]}],"generationConfig":{"thinkingConfig":{"thinkingBudget":8192,"includeThoughts":true}}}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/gemini-to-chat-thinking-model:generateContent", body)
	var resp apiopenapi.GeminiGenerateContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode Gemini response: %v", err)
	}
	if len(resp.Candidates) != 1 || len(resp.Candidates[0].Content.Parts) != 1 || resp.Candidates[0].Content.Parts[0].Text == nil || *resp.Candidates[0].Content.Parts[0].Text != "openai reasoning ok" {
		t.Fatalf("unexpected Gemini response: %+v", resp)
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	if doc["reasoning_effort"] != "medium" {
		t.Fatalf("expected Gemini thinkingConfig to reach OpenAI reasoning_effort=medium, got %s", sent)
	}
	if _, ok := doc["response_format"]; ok {
		t.Fatalf("did not expect Gemini thinkingConfig to leak into OpenAI response_format, got %s", sent)
	}
}
