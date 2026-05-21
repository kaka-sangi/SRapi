package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
)

func TestOpenAICompatibleAdapterInvokesUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		if r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "gpt-upstream" || len(payload.Messages) != 1 || payload.Messages[0].Content != "hello upstream" {
			t.Fatalf("unexpected upstream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"upstream says hi"}}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_adapter",
		Model:     "gpt-local",
		Prompt:    "hello upstream",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:       1,
			Metadata: map[string]any{"base_url": upstream.URL + "/v1"},
		},
		Mapping: modelcontract.ModelProviderMapping{
			UpstreamModelName: "gpt-upstream",
		},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke upstream: %v", err)
	}
	if resp.Text != "upstream says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected adapter response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterForwardsConversionFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens      *int             `json:"max_tokens"`
			Tools          []map[string]any `json:"tools"`
			ToolChoice     any              `json:"tool_choice"`
			ResponseFormat map[string]any   `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "gpt-upstream" {
			t.Fatalf("unexpected upstream model: %+v", payload)
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[0].Content != "be precise" || payload.Messages[1].Content != "run lookup" {
			t.Fatalf("expected system instructions and user prompt in upstream messages, got %+v", payload.Messages)
		}
		if payload.MaxTokens == nil || *payload.MaxTokens != 128 {
			t.Fatalf("expected max_tokens 128, got %+v", payload.MaxTokens)
		}
		if len(payload.Tools) != 1 {
			t.Fatalf("expected one tool, got %+v", payload.Tools)
		}
		function, ok := payload.Tools[0]["function"].(map[string]any)
		if !ok || function["name"] != "lookup" {
			t.Fatalf("expected lookup tool function, got %+v", payload.Tools)
		}
		if payload.ToolChoice != "auto" {
			t.Fatalf("expected tool_choice auto, got %+v", payload.ToolChoice)
		}
		if payload.ResponseFormat["type"] != "json_object" {
			t.Fatalf("expected response_format json_object, got %+v", payload.ResponseFormat)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"lookup done"}}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:       "req_conversion_fields",
		Model:           "gpt-local",
		Prompt:          "run lookup",
		Instructions:    "be precise",
		MaxOutputTokens: ptrInt(128),
		Tools: []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name": "lookup",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		}},
		ToolChoice:     "auto",
		ResponseFormat: map[string]any{"type": "json_object"},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke upstream: %v", err)
	}
	if resp.Text != "lookup done" || resp.Usage.InputTokens != 8 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected adapter response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterStreamsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var payload struct {
			Model         string `json:"model"`
			Stream        bool   `json:"stream"`
			StreamOptions *struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "gpt-upstream" || !payload.Stream || payload.StreamOptions == nil || !payload.StreamOptions.IncludeUsage {
			t.Fatalf("unexpected stream payload: %+v", payload)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Content != "hello stream" {
			t.Fatalf("unexpected stream messages: %+v", payload.Messages)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" stream\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":6,\"total_tokens\":11,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_stream",
		Model:     "gpt-local",
		Prompt:    "hello stream",
		Stream:    true,
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:       1,
			Metadata: map[string]any{"base_url": upstream.URL + "/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke stream upstream: %v", err)
	}
	if resp.Text != "hello stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected stream response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterEstimatesStreamUsageWhenMissing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"estimated usage\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_stream_estimated",
		Model:      "gpt-local",
		Prompt:     "hello",
		Stream:     true,
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke stream upstream: %v", err)
	}
	if resp.Text != "estimated usage" || !resp.Usage.Estimated {
		t.Fatalf("expected estimated stream usage, got %+v", resp)
	}
}

func TestOpenAICompatibleAdapterClassifiesInterruptedStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_stream_interrupted",
		Model:      "gpt-local",
		Prompt:     "hello",
		Stream:     true,
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	assertProviderError(t, err, "stream_interrupted", http.StatusBadGateway)
}

func TestAdapterFallsBackToLocalResponseWithoutBaseURL(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_local",
		Model:     "gpt-local",
		Prompt:    "hello local",
		Mapping: modelcontract.ModelProviderMapping{
			UpstreamModelName: "gpt-local",
		},
	})
	if err != nil {
		t.Fatalf("invoke local fallback: %v", err)
	}
	if !strings.Contains(resp.Text, "hello local") || !resp.Usage.Estimated {
		t.Fatalf("unexpected local fallback response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterClassifiesAuthFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_auth",
		Model:      "gpt-local",
		Prompt:     "hello",
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	assertProviderError(t, err, "auth_failed", http.StatusUnauthorized)
}

func TestOpenAICompatibleAdapterClassifiesRateLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_rate_limit",
		Model:     "gpt-local",
		Prompt:    "hello",
		Account: accountcontract.ProviderAccount{
			Metadata: map[string]any{"base_url": upstream.URL + "/v1"},
		},
		Mapping: modelcontract.ModelProviderMapping{
			UpstreamModelName: "gpt-upstream",
		},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	providerErr, ok := err.(contract.ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Class != "rate_limit" || providerErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected provider error: %+v", providerErr)
	}
}

func TestOpenAICompatibleAdapterClassifiesProvider5xx(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider failed", http.StatusBadGateway)
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_5xx",
		Model:      "gpt-local",
		Prompt:     "hello",
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	assertProviderError(t, err, "provider_5xx", http.StatusBadGateway)
}

func TestOpenAICompatibleAdapterClassifiesInvalidRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid request", http.StatusBadRequest)
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_invalid",
		Model:      "gpt-local",
		Prompt:     "hello",
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
}

func TestReverseProxyAdapterUsesRuntimeForNonAPIKeyAccount(t *testing.T) {
	var upstreamHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		if r.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Fatalf("expected oauth bearer token, got %q", r.Header.Get("Authorization"))
		}
		if strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi user agent leakage: %+v", r.Header)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"reverse proxy response"}}],"usage":{"input_tokens":2,"output_tokens":3}}`))
	}))
	defer upstream.Close()

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create reverse proxy runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_proxy",
		Model:     "rp-local",
		Prompt:    "hello",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             7,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/v1", "user_agent": "Codex/1.0"},
		},
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "rp-upstream"},
		Credential: map[string]any{
			"access_token": "oauth-access",
		},
	})
	if err != nil {
		t.Fatalf("invoke reverse proxy adapter: %v", err)
	}
	if resp.Text != "reverse proxy response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected reverse proxy adapter response: %+v", resp)
	}
	if upstreamHeaders.Get("User-Agent") != "Codex/1.0" {
		t.Fatalf("expected reverse proxy user agent, got %q", upstreamHeaders.Get("User-Agent"))
	}
	if metrics := runtime.Metrics(); metrics.RequestTotal != 1 || metrics.RequestSuccessTotal != 1 {
		t.Fatalf("expected reverse proxy runtime metrics, got %+v", metrics)
	}
}

func TestReverseProxyAdapterStreamsThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"choices\":[{\"delta\":{\"content\":\"runtime\"}}]}\n\n" +
					"data: {\"choices\":[{\"delta\":{\"content\":\" stream\"}}]}\n\n" +
					"data: {\"choices\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5}}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_stream",
		Model:     "rp-local",
		Prompt:    "hello",
		Stream:    true,
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           7,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata:     map[string]any{"base_url": "https://upstream.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "rp-upstream"},
		Credential: map[string]any{"access_token": "oauth-access"},
	})
	if err != nil {
		t.Fatalf("invoke reverse proxy stream adapter: %v", err)
	}
	if !runtime.request.ExpectStream {
		t.Fatalf("expected reverse proxy runtime stream flag, got %+v", runtime.request)
	}
	var payload struct {
		Stream        bool `json:"stream"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode runtime payload: %v", err)
	}
	if !payload.Stream || payload.StreamOptions == nil || !payload.StreamOptions.IncludeUsage {
		t.Fatalf("expected streaming runtime payload, got %+v", payload)
	}
	if resp.Text != "runtime stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected reverse proxy stream response: %+v", resp)
	}
}

func TestReverseProxyAdapterMapsRuntimeErrors(t *testing.T) {
	runtime := failingRuntime{}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_error",
		Model:     "rp-local",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           7,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata:     map[string]any{"base_url": "http://upstream.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "rp-upstream"},
		Credential: map[string]any{"access_token": "oauth-access"},
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	providerErr, ok := err.(contract.ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Class != "session_invalid" || providerErr.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected provider error: %+v", providerErr)
	}
}

func TestReverseProxyAdapterNormalizesLegacyUpstreamError(t *testing.T) {
	runtime := legacyUpstreamErrorRuntime{}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_legacy",
		Model:     "rp-local",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           7,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata:     map[string]any{"base_url": "http://upstream.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "rp-upstream"},
		Credential: map[string]any{"access_token": "oauth-access"},
	})
	assertProviderError(t, err, "provider_5xx", http.StatusBadGateway)
}

type failingRuntime struct{}

func (failingRuntime) Do(context.Context, reverseproxycontract.Request) (reverseproxycontract.Response, error) {
	return reverseproxycontract.Response{}, reverseproxycontract.RuntimeError{Class: "session_invalid", StatusCode: http.StatusForbidden, Message: "session invalid"}
}

type legacyUpstreamErrorRuntime struct{}

func (legacyUpstreamErrorRuntime) Do(context.Context, reverseproxycontract.Request) (reverseproxycontract.Response, error) {
	return reverseproxycontract.Response{}, reverseproxycontract.RuntimeError{Class: "upstream_error", StatusCode: http.StatusBadGateway, Message: "upstream failed"}
}

type capturingRuntime struct {
	request  reverseproxycontract.Request
	response reverseproxycontract.Response
	err      error
}

func (r *capturingRuntime) Do(_ context.Context, req reverseproxycontract.Request) (reverseproxycontract.Response, error) {
	r.request = req
	if r.err != nil {
		return reverseproxycontract.Response{}, r.err
	}
	return r.response, nil
}

func assertProviderError(t *testing.T, err error, class string, statusCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected provider error")
	}
	providerErr, ok := err.(contract.ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Class != class || providerErr.StatusCode != statusCode {
		t.Fatalf("expected provider error %s/%d, got %+v", class, statusCode, providerErr)
	}
}

func ptrString(value string) *string {
	return &value
}

func ptrInt(value int) *int {
	return &value
}
