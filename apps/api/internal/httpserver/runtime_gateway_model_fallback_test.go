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

func TestGatewayChatCompletionFallsBackAcrossAliasModels(t *testing.T) {
	var (
		mu             sync.Mutex
		primaryCalls   int
		fallbackCalls  int
		receivedModels []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		receivedModels = append(receivedModels, payload.Model)
		if payload.Model == "alias-fallback-primary-upstream" {
			primaryCalls++
		}
		if payload.Model == "alias-fallback-secondary-upstream" {
			fallbackCalls++
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if payload.Model == "alias-fallback-primary-upstream" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"primary quota exhausted","type":"rate_limit"}}`))
			return
		}
		if payload.Model != "alias-fallback-secondary-upstream" {
			t.Fatalf("unexpected upstream model %q", payload.Model)
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl_alias_fallback","object":"chat.completion","model":"alias-fallback-secondary-model","choices":[{"message":{"role":"assistant","content":"model fallback ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"alias-model-fallback-provider","display_name":"Alias Model Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	primaryModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"alias-fallback-primary-model","display_name":"Alias Fallback Primary","status":"active"}`)
	fallbackModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"alias-fallback-secondary-model","display_name":"Alias Fallback Secondary","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(primaryModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"alias-fallback-primary-upstream","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(fallbackModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"alias-fallback-secondary-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"alias-model-fallback-account","runtime_class":"api_key","credential":{"api_key":"alias-model-fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	aliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(primaryModel.Data.Id)+"/aliases", strings.NewReader(`{"alias":"public-alias-fallback","fallback_models":["alias-fallback-secondary-model"],"status":"active"}`))
	aliasReq.Header.Set("Content-Type", "application/json")
	aliasReq.AddCookie(sessionCookie)
	aliasReq.Header.Set("X-CSRF-Token", csrf)
	aliasRec := httptest.NewRecorder()
	handler.ServeHTTP(aliasRec, aliasReq)
	if aliasRec.Code != http.StatusCreated {
		t.Fatalf("expected alias create 201, got %d body=%s", aliasRec.Code, aliasRec.Body.String())
	}

	keyResp := mustCreateAPIKey(t, handler, sessionCookie, csrf, `{"name":"alias-fallback-key","scopes":["gateway:invoke"],"allowed_models":["public-alias-fallback"]}`)
	rec := mustGatewayRequest(t, handler, keyResp.Data.PlaintextKey, http.MethodPost, "/v1/chat/completions", `{"model":"public-alias-fallback","messages":[{"role":"user","content":"use fallback"}]}`)
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(chatResp.Choices) != 1 || decodeChatMessageText(t, chatResp.Choices[0].Message.Content) != "model fallback ok" {
		t.Fatalf("expected fallback response, got %+v", chatResp)
	}
	if chatResp.Model != "alias-fallback-secondary-model" {
		t.Fatalf("expected response model to show fallback model, got %q", chatResp.Model)
	}

	mu.Lock()
	gotPrimaryCalls := primaryCalls
	gotFallbackCalls := fallbackCalls
	gotModels := append([]string(nil), receivedModels...)
	mu.Unlock()
	if gotPrimaryCalls != 1 || gotFallbackCalls != 1 {
		t.Fatalf("expected one primary and one fallback call, got primary=%d fallback=%d models=%v", gotPrimaryCalls, gotFallbackCalls, gotModels)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=alias-fallback-secondary-model", nil)
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
		t.Fatalf("expected one successful fallback usage row, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.Model != "alias-fallback-secondary-model" || usage.RequestedModel == nil || *usage.RequestedModel != "public-alias-fallback" || usage.UpstreamModel == nil || *usage.UpstreamModel != "alias-fallback-secondary-upstream" {
		t.Fatalf("unexpected fallback usage evidence: %+v", usage)
	}
}
