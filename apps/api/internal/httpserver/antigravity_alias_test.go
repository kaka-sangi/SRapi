package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestGatewayAntigravityProviderAliasTargetsOpenAIReverseProxy(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		UserAgent     string
		Project       string
		RequestID     string
		Model         string
		Prompt        string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Project   string `json:"project"`
			RequestID string `json:"requestId"`
			UserAgent string `json:"userAgent"`
			Model     string `json:"model"`
			Request   struct {
				Contents []struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"contents"`
			} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.UserAgent != "antigravity" {
			t.Fatalf("expected antigravity v1internal userAgent, got %+v", payload)
		}
		call := upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
			Project:       payload.Project,
			RequestID:     payload.RequestID,
			Model:         payload.Model,
		}
		if len(payload.Request.Contents) > 0 && len(payload.Request.Contents[0].Parts) > 0 {
			call.Prompt = payload.Request.Contents[0].Parts[0].Text
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity alias ok"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":9,"totalTokenCount":17}}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity","display_name":"Antigravity","adapter_type":"reverse-proxy-antigravity","protocol":"openai-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-fallback","display_name":"Antigravity Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-alias-model","display_name":"Antigravity Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"antigravity-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-alias-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"access_token":"desktop-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"project-1"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/antigravity/v1/chat/completions", `{"model":"antigravity-alias-model","messages":[{"role":"user","content":"alias antigravity"}]}`)
	var resp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(resp.Choices) != 1 || decodeChatMessageText(t, resp.Choices[0].Message.Content) != "antigravity alias ok" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1internal:generateContent" ||
		call.Authorization != "Bearer desktop-token" ||
		call.UserAgent != "Antigravity/1.0" ||
		call.Project != "project-1" ||
		!strings.HasPrefix(call.RequestID, "agent-") ||
		call.Model != "antigravity-upstream" ||
		call.Prompt != "alias antigravity" {
		t.Fatalf("unexpected Antigravity upstream call: %+v", call)
	}

	assertAntigravityAliasEvidence(t, handler, sessionCookie, "antigravity-alias-model", string(providerResp.Data.Id), string(accountResp.Data.Id), "/api/provider/antigravity/v1/chat/completions", "openai-compatible", 17)
}

func TestGatewayAntigravityProviderAliasTargetsAnthropicReverseProxy(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		UserAgent     string
		Project       string
		Model         string
		System        string
		MaxTokens     int
		Message       string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Project string `json:"project"`
			Model   string `json:"model"`
			Request struct {
				SystemInstruction *struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"systemInstruction"`
				GenerationConfig struct {
					MaxOutputTokens int `json:"maxOutputTokens"`
				} `json:"generationConfig"`
				Contents []struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"contents"`
			} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		call := upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
			Project:       payload.Project,
			Model:         payload.Model,
			System:        geminiSystemInstructionText(payload.Request.SystemInstruction),
			MaxTokens:     payload.Request.GenerationConfig.MaxOutputTokens,
		}
		if len(payload.Request.Contents) > 0 && len(payload.Request.Contents[0].Parts) > 0 {
			call.Message = payload.Request.Contents[0].Parts[0].Text
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity messages ok"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":6,"totalTokenCount":11}}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity","display_name":"Antigravity","adapter_type":"reverse-proxy-antigravity","protocol":"anthropic-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-fallback-messages","display_name":"Antigravity Fallback Messages","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-messages-model","display_name":"Antigravity Messages Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-claude","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"antigravity-messages-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-claude","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-messages-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"access_token":"desktop-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"project-1"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/antigravity/v1/messages", `{"model":"antigravity-messages-model","system":"be direct","max_tokens":48,"messages":[{"role":"user","content":"alias messages"}]}`)
	var resp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text == nil || *resp.Content[0].Text != "antigravity messages ok" {
		t.Fatalf("unexpected messages response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1internal:generateContent" ||
		call.Authorization != "Bearer desktop-token" ||
		call.UserAgent != "Antigravity/1.0" ||
		call.Project != "project-1" ||
		call.Model != "antigravity-claude" ||
		call.System != "be direct" ||
		call.MaxTokens != 48 ||
		call.Message != "alias messages" {
		t.Fatalf("unexpected Antigravity Messages upstream call: %+v", call)
	}

	assertAntigravityAliasEvidence(t, handler, sessionCookie, "antigravity-messages-model", string(providerResp.Data.Id), string(accountResp.Data.Id), "/antigravity/v1/messages", "anthropic-compatible", 11)
}

func TestGatewayAntigravityGeminiAliasTargetsReverseProxy(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []upstreamNativeGeminiCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Request struct {
				Contents []struct {
					Role  string `json:"role"`
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"contents"`
				SystemInstruction *struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"systemInstruction"`
				GenerationConfig struct {
					MaxOutputTokens int `json:"maxOutputTokens"`
				} `json:"generationConfig"`
			} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamNativeGeminiCall{
			Path:       r.URL.Path,
			APIKey:     r.URL.Query().Get("key"),
			Contents:   payload.Request.Contents,
			SystemText: geminiSystemInstructionText(payload.Request.SystemInstruction),
			MaxTokens:  payload.Request.GenerationConfig.MaxOutputTokens,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity gemini alias ok"}]}}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":7,"totalTokenCount":13}}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity","display_name":"Antigravity","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-gemini-fallback","display_name":"Antigravity Gemini Fallback","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-gemini-model","display_name":"Antigravity Gemini Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-gemini","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"antigravity-gemini-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-gemini-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-gemini-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"access_token":"desktop-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"project-1"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	body := `{"systemInstruction":{"parts":[{"text":"stay concise"}]},"contents":[{"role":"user","parts":[{"text":"alias gemini"}]}],"generationConfig":{"maxOutputTokens":24}}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/antigravity/v1beta/models/antigravity-gemini-model:generateContent", body)
	var resp apiopenapi.GeminiGenerateContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gemini response: %v", err)
	}
	if len(resp.Candidates) != 1 || len(resp.Candidates[0].Content.Parts) != 1 || resp.Candidates[0].Content.Parts[0].Text == nil || *resp.Candidates[0].Content.Parts[0].Text != "antigravity gemini alias ok" {
		t.Fatalf("unexpected gemini alias response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]upstreamNativeGeminiCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1internal:generateContent" || call.APIKey != "" || call.MaxTokens != 0 || call.SystemText != "stay concise" {
		t.Fatalf("unexpected Antigravity Gemini upstream call: %+v", call)
	}
	if len(call.Contents) != 1 || call.Contents[0].Role != "user" || call.Contents[0].Parts[0].Text != "alias gemini" {
		t.Fatalf("unexpected Antigravity Gemini payload: %+v", call)
	}

	assertAntigravityAliasEvidence(t, handler, sessionCookie, "antigravity-gemini-model", string(providerResp.Data.Id), string(accountResp.Data.Id), "/api/provider/antigravity/v1beta/models/antigravity-gemini-model:generateContent", "gemini-compatible", 13)
}

func TestGatewayAntigravityGeminiStreamAliasTargetsReverseProxy(t *testing.T) {
	type streamCall struct {
		Path     string
		Alt      string
		APIKey   string
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}
	}
	var (
		mu    sync.Mutex
		calls []streamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Request struct {
				Contents []struct {
					Role  string `json:"role"`
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"contents"`
			} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream stream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, streamCall{
			Path:     r.URL.Path,
			Alt:      r.URL.Query().Get("alt"),
			APIKey:   r.URL.Query().Get("key"),
			Contents: payload.Request.Contents,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"antigravity stream\"}]}}]}}\n\n"))
		_, _ = w.Write([]byte("data: {\"response\":{\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":5,\"totalTokenCount\":9}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity","display_name":"Antigravity","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-gemini-stream-model","display_name":"Antigravity Gemini Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-gemini-stream-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-gemini-stream-account","runtime_class":"oauth_refresh","upstream_client":"antigravity_desktop","credential":{"access_token":"desktop-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"project-1"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	path := "/antigravity/v1beta/models/antigravity-gemini-stream-model:streamGenerateContent"
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, path, `{"contents":[{"role":"user","parts":[{"text":"stream alias"}]}]}`)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected event stream content type, got %q", got)
	}
	body := rec.Body.String()
	for _, expected := range []string{"data:", "antigravity stream", "usageMetadata", "data: [DONE]"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected SSE body to contain %q, got %s", expected, body)
		}
	}

	mu.Lock()
	gotCalls := append([]streamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one stream upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1internal:streamGenerateContent" || call.Alt != "sse" || call.APIKey != "" {
		t.Fatalf("unexpected Antigravity Gemini stream upstream call: %+v", call)
	}
	if len(call.Contents) != 1 || call.Contents[0].Role != "user" || call.Contents[0].Parts[0].Text != "stream alias" {
		t.Fatalf("unexpected Antigravity Gemini stream payload: %+v", call)
	}

	assertAntigravityAliasEvidence(t, handler, sessionCookie, "antigravity-gemini-stream-model", string(providerResp.Data.Id), string(accountResp.Data.Id), path, "gemini-compatible", 9)
}

func assertAntigravityAliasEvidence(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, modelName, providerID, accountID, endpoint, targetProtocol string, totalTokens int) {
	t.Helper()

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model="+modelName, nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one Antigravity alias decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != providerID || decision.CandidateCount != 1 || decision.TargetProtocol != targetProtocol || decision.SourceEndpoint != endpoint {
		t.Fatalf("expected Antigravity alias scheduler evidence, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model="+modelName, nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one Antigravity usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.ProviderId == nil || *usage.ProviderId != providerID || usage.AccountId == nil || *usage.AccountId != accountID || usage.SourceEndpoint != endpoint || usage.TargetProtocol == nil || *usage.TargetProtocol != targetProtocol || usage.TotalTokens != totalTokens {
		t.Fatalf("expected Antigravity usage evidence, got %+v", usage)
	}
}
