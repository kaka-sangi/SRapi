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

func TestOpenAICompatibleAdapterInvokesEmbeddingsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer embeddings-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var payload struct {
			Model          string   `json:"model"`
			Input          []string `json:"input"`
			EncodingFormat string   `json:"encoding_format"`
			Dimensions     *int     `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "embedding-upstream" || len(payload.Input) != 2 || payload.Input[0] != "first" || payload.Input[1] != "second" {
			t.Fatalf("unexpected upstream payload: %+v", payload)
		}
		if payload.EncodingFormat != "float" || payload.Dimensions == nil || *payload.Dimensions != 3 {
			t.Fatalf("expected encoding/dimensions, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2,0.3],"index":0},{"object":"embedding","embedding":[0.4,0.5,0.6],"index":1}],"model":"embedding-upstream","usage":{"prompt_tokens":7,"total_tokens":7}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeEmbeddings(context.Background(), contract.EmbeddingRequest{
		RequestID:      "req_embeddings",
		Model:          "embedding-local",
		Input:          []string{"first", "second"},
		EncodingFormat: "float",
		Dimensions:     ptrInt(3),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:       1,
			Metadata: map[string]any{"base_url": upstream.URL + "/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "embedding-upstream"},
		Credential: map[string]any{"api_key": "embeddings-secret"},
	})
	if err != nil {
		t.Fatalf("invoke embeddings upstream: %v", err)
	}
	if resp.Model != "embedding-upstream" || len(resp.Data) != 2 || len(resp.Data[0].Vector) != 3 || resp.Data[1].Index != 1 {
		t.Fatalf("unexpected embeddings response: %+v", resp)
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 0 {
		t.Fatalf("unexpected embedding usage: %+v", resp.Usage)
	}
}

func TestOpenAICompatibleAdapterInvokesImageGenerationsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer images-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var payload struct {
			Model          string `json:"model"`
			Prompt         string `json:"prompt"`
			N              int    `json:"n"`
			Size           string `json:"size"`
			Quality        string `json:"quality"`
			Style          string `json:"style"`
			ResponseFormat string `json:"response_format"`
			User           string `json:"user"`
			Background     string `json:"background"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "image-upstream" || payload.Prompt != "draw a precise test image" || payload.N != 2 || payload.Size != "1024x1024" {
			t.Fatalf("unexpected image payload: %+v", payload)
		}
		if payload.Quality != "high" || payload.Style != "vivid" || payload.ResponseFormat != "url" || payload.User != "user-123" || payload.Background != "transparent" {
			t.Fatalf("expected image conversion fields, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000000,"data":[{"url":"https://example.test/image-1.png","revised_prompt":"draw a precise test image, revised"},{"b64_json":"aW1hZ2UtMg=="}],"model":"image-upstream","usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageGeneration(context.Background(), contract.ImageGenerationRequest{
		RequestID:      "req_images",
		Model:          "image-local",
		Prompt:         "draw a precise test image",
		Count:          2,
		Size:           "1024x1024",
		Quality:        "high",
		Style:          "vivid",
		ResponseFormat: "url",
		User:           "user-123",
		Extra:          map[string]any{"background": "transparent"},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "image-upstream"},
		Credential: map[string]any{"api_key": "images-secret"},
	})
	if err != nil {
		t.Fatalf("invoke image generation upstream: %v", err)
	}
	if resp.Model != "image-upstream" || resp.Created != 1710000000 || len(resp.Data) != 2 || resp.Data[0].URL == "" || resp.Data[1].Base64JSON == "" {
		t.Fatalf("unexpected image generation response: %+v", resp)
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected image usage: %+v", resp.Usage)
	}
}

func TestOpenAICompatibleAdapterInvokesModerationsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/moderations" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer moderations-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var payload struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
			User  string   `json:"user"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "moderation-upstream" || len(payload.Input) != 2 || payload.Input[0] != "first safe input" || payload.Input[1] != "second safe input" || payload.User != "user-123" {
			t.Fatalf("unexpected moderation payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"modr_test","model":"moderation-upstream","results":[{"flagged":false,"categories":{"violence":false,"self-harm":false},"category_scores":{"violence":0.01,"self-harm":0.02},"category_applied_input_types":{"violence":["text"]}}],"usage":{"prompt_tokens":8,"total_tokens":8}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeModerations(context.Background(), contract.ModerationRequest{
		RequestID: "req_moderations",
		Model:     "moderation-local",
		Input:     []string{"first safe input", "second safe input"},
		User:      "user-123",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "moderation-upstream"},
		Credential: map[string]any{"api_key": "moderations-secret"},
	})
	if err != nil {
		t.Fatalf("invoke moderation upstream: %v", err)
	}
	if resp.ID != "modr_test" || resp.Model != "moderation-upstream" || len(resp.Results) != 1 || resp.Results[0].Flagged || resp.Results[0].Categories["violence"] {
		t.Fatalf("unexpected moderation response: %+v", resp)
	}
	if resp.Results[0].CategoryScores["self-harm"] <= 0 || len(resp.Results[0].CategoryAppliedInputTypes["violence"]) != 1 {
		t.Fatalf("expected moderation category details, got %+v", resp.Results[0])
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 8 || resp.Usage.OutputTokens != 0 {
		t.Fatalf("unexpected moderation usage: %+v", resp.Usage)
	}
}

func TestRerankCompatibleAdapterInvokesRerankUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer rerank-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var payload struct {
			Model           string `json:"model"`
			Query           string `json:"query"`
			Documents       []any  `json:"documents"`
			TopN            *int   `json:"top_n"`
			ReturnDocuments bool   `json:"return_documents"`
			User            string `json:"user"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "rerank-upstream" || payload.Query != "what is srapi" || len(payload.Documents) != 2 || payload.TopN == nil || *payload.TopN != 1 || !payload.ReturnDocuments || payload.User != "user-123" {
			t.Fatalf("unexpected rerank payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"rerank_test","model":"rerank-upstream","results":[{"index":1,"relevance_score":0.92,"document":{"text":"SRapi routes requests through Scheduler.","source":"docs"}}],"usage":{"prompt_tokens":9,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeRerank(context.Background(), contract.RerankRequest{
		RequestID:       "req_rerank",
		Model:           "rerank-local",
		Query:           "what is srapi",
		Documents:       []contract.RerankDocument{{Text: "Payments settle orders."}, {Text: "SRapi routes requests through Scheduler.", Fields: map[string]any{"text": "SRapi routes requests through Scheduler.", "source": "docs"}}},
		TopN:            ptrInt(1),
		ReturnDocuments: true,
		User:            "user-123",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "rerank-compatible",
			Protocol:    "rerank-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "rerank-upstream"},
		Credential: map[string]any{"api_key": "rerank-secret"},
	})
	if err != nil {
		t.Fatalf("invoke rerank upstream: %v", err)
	}
	if resp.ID != "rerank_test" || resp.Model != "rerank-upstream" || len(resp.Results) != 1 || resp.Results[0].Index != 1 || resp.Results[0].RelevanceScore <= 0.9 || resp.Results[0].Document == nil {
		t.Fatalf("unexpected rerank response: %+v", resp)
	}
	if resp.Results[0].Document.Fields["source"] != "docs" {
		t.Fatalf("expected returned document fields, got %+v", resp.Results[0].Document)
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 0 {
		t.Fatalf("unexpected rerank usage: %+v", resp.Usage)
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

func TestGeminiCompatibleAdapterInvokesGenerateContentUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-pro:generateContent" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "gemini-secret" {
			t.Fatalf("unexpected api key query %q", got)
		}
		if r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		var payload struct {
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
			GenerationConfig *struct {
				MaxOutputTokens int      `json:"maxOutputTokens"`
				Temperature     float32  `json:"temperature"`
				TopP            float32  `json:"topP"`
				StopSequences   []string `json:"stopSequences"`
			} `json:"generationConfig"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Contents) != 2 || payload.Contents[0].Role != "user" || payload.Contents[0].Parts[0].Text != "hello gemini" || payload.Contents[1].Role != "model" || payload.Contents[1].Parts[0].Text != "prior answer" {
			t.Fatalf("unexpected Gemini contents: %+v", payload.Contents)
		}
		if payload.SystemInstruction == nil || len(payload.SystemInstruction.Parts) != 1 || payload.SystemInstruction.Parts[0].Text != "be concise" {
			t.Fatalf("expected system instruction, got %+v", payload.SystemInstruction)
		}
		if payload.GenerationConfig == nil || payload.GenerationConfig.MaxOutputTokens != 64 || payload.GenerationConfig.Temperature != 0.3 || payload.GenerationConfig.TopP != 0.7 || len(payload.GenerationConfig.StopSequences) != 1 || payload.GenerationConfig.StopSequences[0] != "stop" {
			t.Fatalf("unexpected generation config: %+v", payload.GenerationConfig)
		}
		if len(payload.Tools) != 1 {
			t.Fatalf("expected one Gemini tool wrapper, got %+v", payload.Tools)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"gemini says hi"}]}}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4,"totalTokenCount":14,"cachedContentTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:       "req_gemini_adapter",
		Model:           "gemini-local",
		Instructions:    "be concise",
		MaxOutputTokens: ptrInt(64),
		Temperature:     ptrFloat32(0.3),
		TopP:            ptrFloat32(0.7),
		Stop:            []string{"stop"},
		Messages: []contract.TextMessage{
			{Role: "user", Content: "hello gemini"},
			{Role: "assistant", Content: "prior answer"},
		},
		Tools: []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":       "lookup",
				"parameters": map[string]any{"type": "object"},
			},
		}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "gemini-compatible",
			Protocol:    "gemini-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if resp.Text != "gemini says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 4 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected gemini response: %+v", resp)
	}
}

func TestGeminiCompatibleAdapterStreamsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-pro:streamGenerateContent" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var payload struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode stream payload: %v", err)
		}
		if len(payload.Contents) != 1 || payload.Contents[0].Parts[0].Text != "stream gemini" {
			t.Fatalf("unexpected stream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" stream\"}]}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":6,\"totalTokenCount\":11}}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_gemini_stream",
		Model:     "gemini-local",
		Prompt:    "stream gemini",
		Stream:    true,
		Provider: providercontract.Provider{
			AdapterType: "native-gemini",
			Protocol:    "gemini-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini stream: %v", err)
	}
	if resp.Text != "hello stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 {
		t.Fatalf("unexpected gemini stream response: %+v", resp)
	}
}

func TestGeminiCompatibleAdapterAcceptsModelsBaseURL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-pro:generateContent" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_gemini_models_base",
		Model:      "gemini-local",
		Prompt:     "hello",
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta/models"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if resp.Text != "ok" {
		t.Fatalf("unexpected gemini response: %+v", resp)
	}
}

func TestGeminiCompatibleAdapterClassifiesGoogleError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota exhausted","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:  "req_gemini_error",
		Model:      "gemini-local",
		Prompt:     "hello",
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	assertProviderError(t, err, "rate_limit", http.StatusTooManyRequests)
}

func TestReverseProxyGeminiAdapterDispatchesThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"candidates":[{"content":{"parts":[{"text":"gemini runtime response"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_gemini",
		Model:     "gemini-local",
		Prompt:    "hello",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-gemini-cli",
			Protocol:    "gemini-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("gemini_cli"),
			Metadata:       map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta", "user_agent": "GeminiCLI/1.0"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke reverse gemini adapter: %v", err)
	}
	if resp.Text != "gemini runtime response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected reverse gemini response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent" || runtime.request.ExpectStream {
		t.Fatalf("unexpected reverse gemini request: %+v", runtime.request)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) || runtime.request.Account.UpstreamClient == nil || *runtime.request.Account.UpstreamClient != "gemini_cli" {
		t.Fatalf("expected gemini runtime context, got %+v", runtime.request.Account)
	}
	var payload struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode reverse gemini payload: %v", err)
	}
	if len(payload.Contents) != 1 || payload.Contents[0].Parts[0].Text != "hello" {
		t.Fatalf("unexpected reverse gemini payload: %+v", payload)
	}
}

func TestAnthropicCompatibleAdapterInvokesMessagesUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "anthropic-secret" {
			t.Fatalf("unexpected x-api-key %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("unexpected anthropic-version %q", got)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi/auth header leakage: %+v", r.Header)
		}
		var payload struct {
			Model     string `json:"model"`
			System    string `json:"system"`
			Stream    bool   `json:"stream"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools      []map[string]any `json:"tools"`
			ToolChoice map[string]any   `json:"tool_choice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "claude-upstream" || payload.System != "be concise\nsystem from chat" || payload.MaxTokens != 128 {
			t.Fatalf("unexpected upstream payload: %+v", payload)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" || payload.Messages[0].Content != "hello anthropic" {
			t.Fatalf("unexpected upstream messages: %+v", payload.Messages)
		}
		if len(payload.Tools) != 1 || payload.Tools[0]["name"] != "lookup" || payload.Tools[0]["input_schema"] == nil {
			t.Fatalf("expected Anthropic tool schema, got %+v", payload.Tools)
		}
		if payload.ToolChoice["type"] != "tool" || payload.ToolChoice["name"] != "lookup" {
			t.Fatalf("expected Anthropic tool_choice, got %+v", payload.ToolChoice)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"anthropic says hi"}],"usage":{"input_tokens":6,"output_tokens":7,"cache_read_input_tokens":2}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:       "req_anthropic",
		Model:           "claude-local",
		Prompt:          "hello anthropic",
		Instructions:    "be concise",
		MaxOutputTokens: ptrInt(128),
		Messages: []contract.TextMessage{
			{Role: "system", Content: "system from chat"},
			{Role: "user", Content: "hello anthropic"},
		},
		Tools: []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name": "lookup",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		}},
		ToolChoice: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "lookup",
			},
		},
		Provider: providercontract.Provider{
			ID:          2,
			AdapterType: "anthropic-compatible",
			Protocol:    "anthropic-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 2, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
	if resp.Text != "anthropic says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 6 || resp.Usage.OutputTokens != 7 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected anthropic adapter response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterStreamsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var payload struct {
			Model     string `json:"model"`
			Stream    bool   `json:"stream"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "claude-upstream" || !payload.Stream || payload.MaxTokens != 1024 {
			t.Fatalf("unexpected stream payload: %+v", payload)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Content != "hello stream" {
			t.Fatalf("unexpected stream messages: %+v", payload.Messages)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" stream\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":6,\"cache_creation_input_tokens\":1}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_anthropic_stream",
		Model:     "claude-local",
		Prompt:    "hello stream",
		Stream:    true,
		Provider: providercontract.Provider{
			ID:          2,
			AdapterType: "anthropic-compatible",
			Protocol:    "anthropic-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 2, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic stream upstream: %v", err)
	}
	if resp.Text != "hello stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected anthropic stream response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterClassifiesRateLimitErrorObject(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_anthropic_rate",
		Model:     "claude-local",
		Prompt:    "hello",
		Provider: providercontract.Provider{
			AdapterType: "anthropic-compatible",
			Protocol:    "anthropic-compatible",
		},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	assertProviderError(t, err, "rate_limit", http.StatusTooManyRequests)
}

func TestReverseProxyAnthropicCompatibleAdapterUsesMessagesEndpoint(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"content":[{"type":"text","text":"reverse anthropic response"}],"usage":{"input_tokens":2,"output_tokens":3}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_anthropic",
		Model:     "claude-local",
		Prompt:    "hello",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-claude-code-cli",
			Protocol:    "anthropic-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             12,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("claude_code_cli"),
			Metadata:       map[string]any{"base_url": "https://upstream.example/v1", "user_agent": "Claude-Code/1.0"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke reverse anthropic adapter: %v", err)
	}
	if resp.Text != "reverse anthropic response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected reverse anthropic response: %+v", resp)
	}
	if runtime.request.URL != "https://upstream.example/v1/messages" {
		t.Fatalf("expected Anthropic messages endpoint, got %s", runtime.request.URL)
	}
	if runtime.request.Headers.Get("anthropic-version") == "" || runtime.request.Headers.Get("x-api-key") != "" || runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("unexpected reverse anthropic headers: %+v", runtime.request.Headers)
	}
	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode runtime payload: %v", err)
	}
	if payload.Model != "claude-upstream" || len(payload.Messages) != 1 || payload.Messages[0].Content != "hello" {
		t.Fatalf("unexpected reverse anthropic payload: %+v", payload)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "claude_code_cli" ||
		runtime.request.Account.Credential["cli_client_token"] != "cli-token" {
		t.Fatalf("expected claude runtime context, got %+v", runtime.request.Account)
	}
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

func TestReverseProxyAdapterPassesCliRuntimeContext(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"choices":[{"message":{"role":"assistant","content":"cli response"}}],"usage":{"input_tokens":1,"output_tokens":2}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_cli_runtime",
		Model:     "codex-local",
		Prompt:    "hello",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://upstream.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke cli reverse proxy adapter: %v", err)
	}
	if resp.Text != "cli response" {
		t.Fatalf("unexpected cli response: %+v", resp)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "codex_cli" ||
		runtime.request.Account.Credential["cli_client_token"] != "cli-token" {
		t.Fatalf("expected cli runtime context, got %+v", runtime.request.Account)
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

func ptrFloat32(value float32) *float32 {
	return &value
}
