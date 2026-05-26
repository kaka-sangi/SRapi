package service_test

import (
	"context"
	"encoding/json"
	"io"
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

func textParts(text string) []contract.ContentPart {
	return []contract.ContentPart{{Kind: contract.ContentPartText, Text: text}}
}

func conversationResponseText(resp contract.ConversationResponse) string {
	parts := make([]string, 0, len(resp.Parts))
	for _, part := range resp.Parts {
		switch part.Kind {
		case "", contract.ContentPartText, contract.ContentPartThinking, contract.ContentPartRefusal, contract.ContentPartToolResult:
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func assertToolUsePart(t *testing.T, part contract.ContentPart, id string, name string, arguments string) {
	t.Helper()
	if part.Kind != contract.ContentPartToolUse || part.ToolCallID != id || part.ToolName != name || part.ToolArgumentsJSON != arguments {
		t.Fatalf("unexpected tool use part: %+v", part)
	}
}

func conversationStreamEventsByType(events []contract.ConversationStreamEvent, eventType contract.ConversationStreamEventType) []contract.ConversationStreamEvent {
	out := make([]contract.ConversationStreamEvent, 0)
	for _, event := range events {
		if event.Type == eventType {
			out = append(out, event)
		}
	}
	return out
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func imageURLPart(url string) contract.ContentPart {
	return contract.ContentPart{Kind: contract.ContentPartImage, MediaURL: url, MIMEType: "image/png"}
}

func toolUsePart(id string, name string, arguments string) contract.ContentPart {
	return contract.ContentPart{
		Kind:              contract.ContentPartToolUse,
		ToolCallID:        id,
		ToolName:          name,
		ToolArgumentsJSON: arguments,
	}
}

func toolResultPart(id string, text string) contract.ContentPart {
	return contract.ContentPart{
		Kind:            contract.ContentPartToolResult,
		ToolResultForID: id,
		Text:            text,
	}
}

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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_adapter",
		Model:      "gpt-local",
		InputParts: textParts("hello upstream"),
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
	if conversationResponseText(resp) != "upstream says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected adapter response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterPreservesToolCallResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"query\":\"weather\"}"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_tool_call",
		Model:      "gpt-local",
		InputParts: textParts("call lookup"),
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
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse || resp.Usage.OutputTokens != 1 {
		t.Fatalf("unexpected tool call response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "call_1", "lookup", `{"query":"weather"}`)
}

func TestOpenAICompatibleAdapterPreservesSameProtocolRawBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload["model"] != "gpt-upstream" || payload["parallel_tool_calls"] != true || payload["user"] != "user-raw" {
			t.Fatalf("expected raw OpenAI fields to be preserved with mapped model, got %+v", payload)
		}
		if streamOptions, _ := payload["stream_options"].(map[string]any); streamOptions["include_usage"] != false {
			t.Fatalf("expected raw stream usage option to be preserved, got %+v", payload["stream_options"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"raw ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_openai_raw",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-local",
		InputParts:     textParts("canonical fallback"),
		RawBody:        []byte(`{"model":"gpt-local","messages":[{"role":"user","content":"raw"}],"parallel_tool_calls":true,"stream_options":{"include_usage":false},"user":"user-raw"}`),
		Provider:       providercontract.Provider{AdapterType: "openai-compatible", Protocol: "openai-compatible"},
		Account:        accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential:     map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke raw OpenAI upstream: %v", err)
	}
	if conversationResponseText(resp) != "raw ok" {
		t.Fatalf("unexpected raw OpenAI response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterDoesNotUseResponsesRawBodyForChatUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if _, hasInput := payload["input"]; hasInput {
			t.Fatalf("responses raw input must not be forwarded to chat upstream: %+v", payload)
		}
		if payload["model"] != "gpt-upstream" || payload["messages"] == nil {
			t.Fatalf("expected canonical chat payload, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fallback ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_openai_no_raw_responses",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-local",
		InputParts:     textParts("canonical fallback"),
		RawBody:        []byte(`{"model":"gpt-local","input":"raw responses input","parallel_tool_calls":true}`),
		Provider:       providercontract.Provider{AdapterType: "openai-compatible", Protocol: "openai-compatible"},
		Account:        accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential:     map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke OpenAI fallback upstream: %v", err)
	}
}

func TestOpenAICompatibleAdapterRendersContentPartsToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role       string           `json:"role"`
				Content    json.RawMessage  `json:"content"`
				ToolCallID string           `json:"tool_call_id"`
				ToolCalls  []map[string]any `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Messages) != 3 {
			t.Fatalf("expected user, assistant tool call, and tool result messages, got %+v", payload.Messages)
		}
		var userContent []map[string]any
		if err := json.Unmarshal(payload.Messages[0].Content, &userContent); err != nil {
			t.Fatalf("decode user content blocks: %v", err)
		}
		if payload.Messages[0].Role != "user" || len(userContent) != 2 || userContent[0]["text"] != "look at this" {
			t.Fatalf("unexpected OpenAI user content: role=%s content=%+v", payload.Messages[0].Role, userContent)
		}
		imageURL, _ := userContent[1]["image_url"].(map[string]any)
		if userContent[1]["type"] != "image_url" || imageURL["url"] != "https://example.test/image.png" {
			t.Fatalf("expected image_url block, got %+v", userContent[1])
		}
		if payload.Messages[1].Role != "assistant" || len(payload.Messages[1].ToolCalls) != 1 {
			t.Fatalf("expected assistant tool call message, got %+v", payload.Messages[1])
		}
		function, _ := payload.Messages[1].ToolCalls[0]["function"].(map[string]any)
		if payload.Messages[1].ToolCalls[0]["id"] != "call_1" || function["name"] != "lookup" || function["arguments"] != `{"query":"weather"}` {
			t.Fatalf("unexpected OpenAI tool call: %+v", payload.Messages[1].ToolCalls[0])
		}
		var toolContent string
		if err := json.Unmarshal(payload.Messages[2].Content, &toolContent); err != nil {
			t.Fatalf("decode tool content: %v", err)
		}
		if payload.Messages[2].Role != "tool" || payload.Messages[2].ToolCallID != "call_1" || toolContent != "sunny" {
			t.Fatalf("unexpected OpenAI tool result message: %+v content=%q", payload.Messages[2], toolContent)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_openai_parts",
		Model:     "gpt-local",
		Messages: []contract.ConversationMessage{
			{Role: "user", Parts: []contract.ContentPart{
				{Kind: contract.ContentPartText, Text: "look at this"},
				imageURLPart("https://example.test/image.png"),
			}},
			{Role: "assistant", Parts: []contract.ContentPart{toolUsePart("call_1", "lookup", `{"query":"weather"}`)}},
			{Role: "tool", Parts: []contract.ContentPart{toolResultPart("call_1", "sunny")}},
		},
		Provider: providercontract.Provider{AdapterType: "openai-compatible", Protocol: "openai-compatible"},
		Account:  accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:  modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{
			"api_key": "upstream-secret",
		},
	})
	if err != nil {
		t.Fatalf("invoke upstream: %v", err)
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

func TestOpenAICompatibleAdapterInvokesImageEditsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-edit-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		imageFile, imageHeader, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("expected upstream image: %v", err)
		}
		defer imageFile.Close()
		imageBytes, err := io.ReadAll(imageFile)
		if err != nil {
			t.Fatalf("read upstream image: %v", err)
		}
		maskFile, maskHeader, err := r.FormFile("mask")
		if err != nil {
			t.Fatalf("expected upstream mask: %v", err)
		}
		defer maskFile.Close()
		maskBytes, err := io.ReadAll(maskFile)
		if err != nil {
			t.Fatalf("read upstream mask: %v", err)
		}
		if imageHeader.Filename != "source.png" || imageHeader.Header.Get("Content-Type") != "image/png" || string(imageBytes) != "PNG-source" {
			t.Fatalf("unexpected upstream image file filename=%q content_type=%q data=%q", imageHeader.Filename, imageHeader.Header.Get("Content-Type"), string(imageBytes))
		}
		if maskHeader.Filename != "mask.png" || maskHeader.Header.Get("Content-Type") != "image/png" || string(maskBytes) != "PNG-mask" {
			t.Fatalf("unexpected upstream mask file filename=%q content_type=%q data=%q", maskHeader.Filename, maskHeader.Header.Get("Content-Type"), string(maskBytes))
		}
		if r.FormValue("model") != "image-edit-upstream" || r.FormValue("prompt") != "replace the background" || r.FormValue("n") != "1" || r.FormValue("size") != "1024x1024" || r.FormValue("quality") != "high" || r.FormValue("response_format") != "b64_json" || r.FormValue("user") != "user-123" || r.FormValue("background") != "transparent" {
			t.Fatalf("unexpected upstream image edit fields: model=%q prompt=%q n=%q size=%q quality=%q response_format=%q user=%q background=%q", r.FormValue("model"), r.FormValue("prompt"), r.FormValue("n"), r.FormValue("size"), r.FormValue("quality"), r.FormValue("response_format"), r.FormValue("user"), r.FormValue("background"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000100,"data":[{"b64_json":"aW1hZ2UtZWRpdA==","revised_prompt":"replace the background, revised"}],"model":"image-edit-upstream","usage":{"input_tokens":22,"output_tokens":3,"total_tokens":25}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageEdit(context.Background(), contract.ImageEditRequest{
		RequestID:      "req_image_edit",
		Model:          "image-edit-local",
		Prompt:         "replace the background",
		Images:         []contract.ImageInput{{FileName: "source.png", ContentType: "image/png", Bytes: []byte("PNG-source")}},
		Mask:           &contract.ImageInput{FileName: "mask.png", ContentType: "image/png", Bytes: []byte("PNG-mask")},
		Count:          1,
		Size:           "1024x1024",
		Quality:        "high",
		ResponseFormat: "b64_json",
		User:           "user-123",
		Extra:          map[string]any{"background": "transparent"},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "image-edit-upstream"},
		Credential: map[string]any{"api_key": "image-edit-secret"},
	})
	if err != nil {
		t.Fatalf("invoke image edit upstream: %v", err)
	}
	if resp.Model != "image-edit-upstream" || resp.Created != 1710000100 || len(resp.Data) != 1 || resp.Data[0].Base64JSON == "" {
		t.Fatalf("unexpected image edit response: %+v", resp)
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 22 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected image edit usage: %+v", resp.Usage)
	}
}

func TestOpenAICompatibleAdapterInvokesImageVariationsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/variations" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-variation-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		imageFile, imageHeader, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("expected upstream image: %v", err)
		}
		defer imageFile.Close()
		imageBytes, err := io.ReadAll(imageFile)
		if err != nil {
			t.Fatalf("read upstream image: %v", err)
		}
		if imageHeader.Filename != "source.png" || imageHeader.Header.Get("Content-Type") != "image/png" || string(imageBytes) != "PNG-source" {
			t.Fatalf("unexpected upstream image file filename=%q content_type=%q data=%q", imageHeader.Filename, imageHeader.Header.Get("Content-Type"), string(imageBytes))
		}
		if r.FormValue("model") != "image-variation-upstream" || r.FormValue("n") != "2" || r.FormValue("size") != "1024x1024" || r.FormValue("response_format") != "url" || r.FormValue("user") != "user-123" || r.FormValue("style_hint") != "studio" {
			t.Fatalf("unexpected upstream image variation fields: model=%q n=%q size=%q response_format=%q user=%q style_hint=%q", r.FormValue("model"), r.FormValue("n"), r.FormValue("size"), r.FormValue("response_format"), r.FormValue("user"), r.FormValue("style_hint"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000300,"data":[{"url":"https://example.test/wp490-variation.png"}],"model":"image-variation-upstream","usage":{"input_tokens":15,"output_tokens":2,"total_tokens":17}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageVariation(context.Background(), contract.ImageVariationRequest{
		RequestID:      "req_image_variation",
		Model:          "image-variation-local",
		Image:          contract.ImageInput{FileName: "source.png", ContentType: "image/png", Bytes: []byte("PNG-source")},
		Count:          2,
		Size:           "1024x1024",
		ResponseFormat: "url",
		User:           "user-123",
		Extra:          map[string]any{"style_hint": "studio"},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "image-variation-upstream"},
		Credential: map[string]any{"api_key": "image-variation-secret"},
	})
	if err != nil {
		t.Fatalf("invoke image variation upstream: %v", err)
	}
	if resp.Model != "image-variation-upstream" || resp.Created != 1710000300 || len(resp.Data) != 1 || resp.Data[0].URL == "" {
		t.Fatalf("unexpected image variation response: %+v", resp)
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 15 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected image variation usage: %+v", resp.Usage)
	}
}

func TestOpenAICompatibleAdapterInvokesAudioTranscriptionsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer audio-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected upstream file: %v", err)
		}
		defer file.Close()
		audio, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upstream file: %v", err)
		}
		if header.Filename != "sample.wav" || header.Header.Get("Content-Type") != "audio/wav" || string(audio) != "RIFF-test-audio" {
			t.Fatalf("unexpected upstream audio file filename=%q content_type=%q data=%q", header.Filename, header.Header.Get("Content-Type"), string(audio))
		}
		if r.FormValue("model") != "audio-upstream" || r.FormValue("language") != "en" || r.FormValue("prompt") != "meeting notes" || r.FormValue("response_format") != "verbose_json" || r.FormValue("temperature") != "0.2" || r.FormValue("user") != "user-123" {
			t.Fatalf("unexpected upstream transcription fields: model=%q language=%q prompt=%q response_format=%q temperature=%q user=%q", r.FormValue("model"), r.FormValue("language"), r.FormValue("prompt"), r.FormValue("response_format"), r.FormValue("temperature"), r.FormValue("user"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"transcribed audio","task":"transcribe","language":"en","duration":1.5,"segments":[{"id":0,"start":0,"end":1.5,"text":"transcribed audio","tokens":[1,2]}],"usage":{"prompt_tokens":9,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeAudioTranscription(context.Background(), contract.AudioTranscriptionRequest{
		RequestID:      "req_audio",
		Model:          "audio-local",
		FileName:       "sample.wav",
		ContentType:    "audio/wav",
		Audio:          []byte("RIFF-test-audio"),
		Language:       "en",
		Prompt:         "meeting notes",
		ResponseFormat: "verbose_json",
		Temperature:    ptrFloat32(0.2),
		User:           "user-123",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "audio-upstream"},
		Credential: map[string]any{"api_key": "audio-secret"},
	})
	if err != nil {
		t.Fatalf("invoke audio transcription upstream: %v", err)
	}
	if resp.Model != "audio-upstream" || resp.Text != "transcribed audio" || resp.Language != "en" || resp.Duration == nil || *resp.Duration != 1.5 || len(resp.Segments) != 1 {
		t.Fatalf("unexpected audio transcription response: %+v", resp)
	}
	if resp.Usage.Estimated || resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 0 {
		t.Fatalf("unexpected audio transcription usage: %+v", resp.Usage)
	}
}

func TestOpenAICompatibleAdapterInvokesAudioSpeechUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer speech-secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var payload struct {
			Model          string   `json:"model"`
			Input          string   `json:"input"`
			Voice          string   `json:"voice"`
			ResponseFormat string   `json:"response_format"`
			Speed          *float32 `json:"speed"`
			Instructions   string   `json:"instructions"`
			User           string   `json:"user"`
			Accent         string   `json:"accent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "speech-upstream" || payload.Input != "say this aloud" || payload.Voice != "alloy" || payload.ResponseFormat != "wav" {
			t.Fatalf("unexpected speech payload: %+v", payload)
		}
		if payload.Speed == nil || *payload.Speed < 1.19 || *payload.Speed > 1.21 || payload.Instructions != "warm" || payload.User != "user-123" || payload.Accent != "neutral" {
			t.Fatalf("expected speech conversion fields, got %+v", payload)
		}
		w.Header().Set("Content-Type", "audio/wav; charset=binary")
		_, _ = w.Write([]byte("RIFF-speech-audio"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeAudioSpeech(context.Background(), contract.AudioSpeechRequest{
		RequestID:      "req_speech",
		Model:          "speech-local",
		Input:          "say this aloud",
		Voice:          "alloy",
		ResponseFormat: "wav",
		Speed:          ptrFloat32(1.2),
		Instructions:   "warm",
		User:           "user-123",
		Extra:          map[string]any{"accent": "neutral"},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "speech-upstream"},
		Credential: map[string]any{"api_key": "speech-secret"},
	})
	if err != nil {
		t.Fatalf("invoke audio speech upstream: %v", err)
	}
	if resp.Model != "speech-upstream" || resp.ContentType != "audio/wav" || string(resp.Audio) != "RIFF-speech-audio" {
		t.Fatalf("unexpected audio speech response: %+v", resp)
	}
	if !resp.Usage.Estimated || resp.Usage.InputTokens <= 0 || resp.Usage.OutputTokens <= 0 {
		t.Fatalf("unexpected audio speech usage: %+v", resp.Usage)
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:       "req_conversion_fields",
		Model:           "gpt-local",
		InputParts:      textParts("run lookup"),
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
	if conversationResponseText(resp) != "lookup done" || resp.Usage.InputTokens != 8 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected adapter response: %+v", resp)
	}
}

func TestOpenAICompatibleAdapterStreamsUpstream(t *testing.T) {
	rawSSE := "event: chat.completion.chunk\n" +
		"data: {\"choices\":[{\"delta\":\n" +
		"data: {\"content\":\"hello\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" stream\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":6,\"total_tokens\":11,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\n" +
		"data: [DONE]\n\n"
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
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_stream",
		Model:      "gpt-local",
		InputParts: textParts("hello stream"),
		Stream:     true,
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
	if conversationResponseText(resp) != "hello stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw OpenAI stream to be preserved\nexpected:\n%s\nactual:\n%s", rawSSE, string(resp.Raw))
	}
	if len(resp.StreamEvents) < 3 {
		t.Fatalf("expected OpenAI stream events, got %+v", resp.StreamEvents)
	}
	if resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[0].Delta.Text != "hello" {
		t.Fatalf("expected first OpenAI content delta, got %+v", resp.StreamEvents[0])
	}
	if resp.StreamEvents[0].ContentIndex != 0 {
		t.Fatalf("expected default OpenAI choice index 0, got %+v", resp.StreamEvents[0])
	}
	if want := "{\"choices\":[{\"delta\":\n{\"content\":\"hello\"}}]}"; string(resp.StreamEvents[0].Raw) != want {
		t.Fatalf("expected first OpenAI raw event payload %q, got %q", want, string(resp.StreamEvents[0].Raw))
	}
	if resp.StreamEvents[1].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[1].Delta.Text != " stream" {
		t.Fatalf("expected second OpenAI content delta preserving leading space, got %+v", resp.StreamEvents[1])
	}
	if resp.StreamEvents[2].Type != contract.ConversationStreamEventUsage || resp.StreamEvents[2].Usage.InputTokens != 5 {
		t.Fatalf("expected OpenAI usage stream event, got %+v", resp.StreamEvents[2])
	}
	if resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected OpenAI terminal stop stream event, got %+v", resp.StreamEvents)
	}
}

func TestOpenAICompatibleAdapterPreservesStreamChoiceIndex(t *testing.T) {
	rawSSE := "data: {\"choices\":[{\"index\":1,\"delta\":{\"content\":\"second choice\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":1,\"finish_reason\":\"stop\",\"delta\":{}}]}\n\n" +
		"data: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_stream_choice_index",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
		Stream:     true,
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
		t.Fatalf("invoke stream upstream: %v", err)
	}
	if len(resp.StreamEvents) < 2 {
		t.Fatalf("expected indexed OpenAI stream events, got %+v", resp.StreamEvents)
	}
	if resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[0].ContentIndex != 1 || resp.StreamEvents[0].Delta.Text != "second choice" {
		t.Fatalf("expected indexed content delta, got %+v", resp.StreamEvents[0])
	}
	if resp.StreamEvents[1].Type != contract.ConversationStreamEventStop || resp.StreamEvents[1].ContentIndex != 1 {
		t.Fatalf("expected indexed stop event, got %+v", resp.StreamEvents[1])
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_stream_estimated",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	if err != nil {
		t.Fatalf("invoke stream upstream: %v", err)
	}
	if conversationResponseText(resp) != "estimated usage" || !resp.Usage.Estimated {
		t.Fatalf("expected estimated stream usage, got %+v", resp)
	}
}

func TestGenericReverseProxyAdapterInvokesConfiguredChatUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat" {
			t.Fatalf("unexpected generic upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "generic-secret" {
			t.Fatalf("unexpected generic auth header %q", got)
		}
		var payload struct {
			UpstreamModel string `json:"upstream_model"`
			PromptItems   []struct {
				Content string `json:"content"`
			} `json:"prompt_items"`
			Route string `json:"route"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode generic chat request: %v", err)
		}
		if payload.UpstreamModel != "generic-upstream" || len(payload.PromptItems) != 1 || payload.PromptItems[0].Content != "hello generic" || payload.Route != "test" {
			t.Fatalf("unexpected generic chat payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"text":"generic says hi"},"metering":{"input_tokens":6,"output_tokens":7,"cached_tokens":2}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_generic_chat",
		Model:      "generic-local",
		InputParts: textParts("hello generic"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "generic-reverse-proxy",
			Protocol:    "openai-compatible",
			ConfigSchema: map[string]any{
				"base_url":             upstream.URL,
				"chat_path":            "/v2/chat",
				"auth_header_template": "X-Api-Key: {{api_key}}",
				"body_mapping_rules": map[string]any{
					"model_field":    "upstream_model",
					"messages_field": "prompt_items",
					"extra":          map[string]any{"route": "test"},
				},
				"response_path_rules": map[string]any{
					"text_path":  "output.text",
					"usage_path": "metering",
				},
			},
		},
		Account:    accountcontract.ProviderAccount{ID: 1, RuntimeClass: accountcontract.RuntimeClassAPIKey},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "generic-upstream"},
		Credential: map[string]any{"api_key": "generic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke generic chat upstream: %v", err)
	}
	if conversationResponseText(resp) != "generic says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 6 || resp.Usage.OutputTokens != 7 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected generic chat response: %+v", resp)
	}
}

func TestGenericReverseProxyAdapterStreamsConfiguredChatUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected generic stream path %s", r.URL.Path)
		}
		var payload struct {
			Model         string `json:"model"`
			Stream        bool   `json:"stream"`
			StreamOptions *struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode generic stream request: %v", err)
		}
		if payload.Model != "stream-upstream" || !payload.Stream || payload.StreamOptions == nil || !payload.StreamOptions.IncludeUsage {
			t.Fatalf("unexpected generic stream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"generic\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" stream\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_generic_stream",
		Model:      "generic-local",
		InputParts: textParts("hello stream"),
		Stream:     true,
		Provider: providercontract.Provider{
			AdapterType:  "generic-reverse-proxy",
			Protocol:     "openai-compatible",
			ConfigSchema: map[string]any{"base_url": upstream.URL + "/v1"},
		},
		Account:    accountcontract.ProviderAccount{ID: 1, RuntimeClass: accountcontract.RuntimeClassAPIKey},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "stream-upstream"},
		Credential: map[string]any{"api_key": "generic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke generic stream upstream: %v", err)
	}
	if conversationResponseText(resp) != "generic stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected generic stream response: %+v", resp)
	}
}

func TestGenericReverseProxyAdapterInvokesEmbeddingsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/embeddings" {
			t.Fatalf("unexpected generic embeddings path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer embedding-secret" {
			t.Fatalf("unexpected generic embeddings auth %q", got)
		}
		var payload struct {
			Model string   `json:"model"`
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode generic embeddings request: %v", err)
		}
		if payload.Model != "embedding-upstream" || len(payload.Texts) != 2 || payload.Texts[0] != "first" || payload.Texts[1] != "second" {
			t.Fatalf("unexpected generic embeddings payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2],"index":0},{"embedding":[0.3,0.4],"index":1}],"model":"embedding-upstream","usage":{"prompt_tokens":8,"total_tokens":8}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeEmbeddings(context.Background(), contract.EmbeddingRequest{
		RequestID: "req_generic_embeddings",
		Model:     "embedding-local",
		Input:     []string{"first", "second"},
		Provider: providercontract.Provider{
			AdapterType: "generic-reverse-proxy",
			Protocol:    "openai-compatible",
			ConfigSchema: map[string]any{
				"base_url":             upstream.URL,
				"embeddings_path":      "/v2/embeddings",
				"auth_header_template": "Authorization: Bearer {{api_key}}",
				"embeddings_body_mapping_rules": map[string]any{
					"input_field": "texts",
				},
			},
		},
		Account:    accountcontract.ProviderAccount{ID: 1, RuntimeClass: accountcontract.RuntimeClassAPIKey},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "embedding-upstream"},
		Credential: map[string]any{"api_key": "embedding-secret"},
	})
	if err != nil {
		t.Fatalf("invoke generic embeddings upstream: %v", err)
	}
	if resp.Model != "embedding-upstream" || len(resp.Data) != 2 || len(resp.Data[0].Vector) != 2 || resp.Usage.InputTokens != 8 || resp.Usage.Estimated {
		t.Fatalf("unexpected generic embeddings response: %+v", resp)
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_stream_interrupted",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	assertProviderError(t, err, "stream_interrupted", http.StatusBadGateway)
}

func TestOpenAICompatibleAdapterClassifiesStreamErrorFrame(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: error\ndata: {\"error\":{\"type\":\"rate_limit_error\",\"message\":\"slow down\",\"code\":429}}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_stream_error",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"api_key": "upstream-secret"},
	})
	providerErr := assertProviderError(t, err, "rate_limit", http.StatusTooManyRequests)
	if providerErr.Message != "slow down" {
		t.Fatalf("expected stream error message to be preserved, got %+v", providerErr)
	}
}

func TestAdapterFallsBackToLocalResponseWithoutBaseURL(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_local",
		Model:      "gpt-local",
		InputParts: textParts("hello local"),
		Mapping: modelcontract.ModelProviderMapping{
			UpstreamModelName: "gpt-local",
		},
	})
	if err != nil {
		t.Fatalf("invoke local fallback: %v", err)
	}
	if !strings.Contains(conversationResponseText(resp), "hello local") || !resp.Usage.Estimated {
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_auth",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_rate_limit",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_5xx",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_invalid",
		Model:      "gpt-local",
		InputParts: textParts("hello"),
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:       "req_gemini_adapter",
		Model:           "gemini-local",
		Instructions:    "be concise",
		MaxOutputTokens: ptrInt(64),
		Temperature:     ptrFloat32(0.3),
		TopP:            ptrFloat32(0.7),
		Stop:            []string{"stop"},
		Messages: []contract.ConversationMessage{
			{Role: "user", Parts: textParts("hello gemini")},
			{Role: "assistant", Parts: textParts("prior answer")},
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
	if conversationResponseText(resp) != "gemini says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 4 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected gemini response: %+v", resp)
	}
}

func TestGeminiCompatibleAdapterPreservesSameProtocolRawBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload["model"] != nil || payload["stream"] != nil || payload["cachedContent"] != "cachedContents/raw" {
			t.Fatalf("expected raw Gemini fields without body model/stream injection, got %+v", payload)
		}
		if _, ok := payload["generationConfig"].(map[string]any); !ok {
			t.Fatalf("expected raw generationConfig, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"gemini raw ok"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_gemini_raw",
		SourceProtocol: "gemini-compatible",
		SourceEndpoint: "/v1beta/models/gemini-local:generateContent",
		TargetProtocol: "gemini-compatible",
		Model:          "gemini-local",
		InputParts:     textParts("canonical fallback"),
		RawBody:        []byte(`{"contents":[{"role":"user","parts":[{"text":"raw"}]}],"generationConfig":{"responseMimeType":"application/json"},"cachedContent":"cachedContents/raw"}`),
		Provider:       providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:        accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential:     map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke raw Gemini upstream: %v", err)
	}
	if conversationResponseText(resp) != "gemini raw ok" {
		t.Fatalf("unexpected raw Gemini response: %+v", resp)
	}
}

func TestGeminiCompatibleAdapterRendersFunctionPartsToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					FunctionCall     map[string]any `json:"functionCall"`
					FunctionResponse map[string]any `json:"functionResponse"`
					ThoughtSignature string         `json:"thoughtSignature"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Contents) != 2 {
			t.Fatalf("expected function call and response contents, got %+v", payload.Contents)
		}
		if payload.Contents[0].Role != "model" || payload.Contents[0].Parts[0].FunctionCall["name"] != "lookup" {
			t.Fatalf("unexpected Gemini function call content: %+v", payload.Contents[0])
		}
		args, _ := payload.Contents[0].Parts[0].FunctionCall["args"].(map[string]any)
		if args["query"] != "weather" {
			t.Fatalf("unexpected Gemini function args: %+v", args)
		}
		if payload.Contents[0].Parts[0].ThoughtSignature != "sig_tool_1" {
			t.Fatalf("expected Gemini thoughtSignature to be preserved, got %+v", payload.Contents[0].Parts[0])
		}
		if payload.Contents[1].Role != "user" || payload.Contents[1].Parts[0].FunctionResponse["response"] == nil {
			t.Fatalf("unexpected Gemini function response content: %+v", payload.Contents[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_gemini_functions",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{
			{Role: "assistant", Parts: []contract.ContentPart{{
				Kind:              contract.ContentPartToolUse,
				ToolCallID:        "call_1",
				ToolName:          "lookup",
				ToolArgumentsJSON: `{"query":"weather"}`,
				Metadata:          map[string]any{"signature": "sig_tool_1"},
			}}},
			{Role: "tool", Parts: []contract.ContentPart{{
				Kind:            contract.ContentPartToolResult,
				ToolName:        "lookup",
				ToolResultForID: "call_1",
				Text:            `{"forecast":"sunny"}`,
			}}},
		},
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
}

func TestGeminiCompatibleAdapterAddsDummyThoughtSignatureForFunctionCallHistory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Parts []struct {
					FunctionCall     map[string]any `json:"functionCall"`
					ThoughtSignature string         `json:"thoughtSignature"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Contents) != 1 ||
			len(payload.Contents[0].Parts) != 1 ||
			payload.Contents[0].Parts[0].FunctionCall["name"] != "lookup" ||
			payload.Contents[0].Parts[0].ThoughtSignature != "skip_thought_signature_validator" {
			t.Fatalf("expected dummy thoughtSignature for function call history, got %+v", payload.Contents)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_gemini_dummy_signature",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{{
			Role:  "assistant",
			Parts: []contract.ContentPart{toolUsePart("call_1", "lookup", `{"query":"weather"}`)},
		}},
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
}

func TestGeminiCompatibleAdapterPreservesTextThoughtSignature(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Parts []struct {
					Text             string `json:"text"`
					ThoughtSignature string `json:"thoughtSignature"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Contents) != 1 ||
			len(payload.Contents[0].Parts) != 1 ||
			payload.Contents[0].Parts[0].Text != "visible model thought" ||
			payload.Contents[0].Parts[0].ThoughtSignature != "sig_gemini_text_1" {
			t.Fatalf("expected Gemini text thoughtSignature to be preserved, got %+v", payload.Contents)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_gemini_text_signature",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{{
			Role: "assistant",
			Parts: []contract.ContentPart{{
				Kind:           contract.ContentPartThinking,
				Text:           "visible model thought",
				OriginProtocol: "gemini",
				Metadata:       map[string]any{"thoughtSignature": "sig_gemini_text_1"},
			}},
		}},
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
}

func TestGeminiCompatibleAdapterDoesNotReuseAnthropicSignatureAsThoughtSignature(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Parts []struct {
					Text             string `json:"text"`
					ThoughtSignature string `json:"thoughtSignature"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Contents) != 1 ||
			len(payload.Contents[0].Parts) != 1 ||
			payload.Contents[0].Parts[0].Text != "anthropic thought" ||
			payload.Contents[0].Parts[0].ThoughtSignature != "" {
			t.Fatalf("expected Anthropic signature to stay out of Gemini thoughtSignature, got %+v", payload.Contents)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_gemini_anthropic_signature",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{{
			Role: "assistant",
			Parts: []contract.ContentPart{{
				Kind:           contract.ContentPartThinking,
				Text:           "anthropic thought",
				OriginProtocol: "anthropic",
				Metadata:       map[string]any{"signature": "sig_anthropic_1"},
			}},
		}},
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
}

func TestGeminiCompatibleAdapterRetriesSignatureErrorWithDowngradedThinking(t *testing.T) {
	var requests []struct {
		Contents []struct {
			Parts []struct {
				Text             string         `json:"text"`
				ThoughtSignature string         `json:"thoughtSignature"`
				FunctionCall     map[string]any `json:"functionCall"`
			} `json:"parts"`
		} `json:"contents"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Parts []struct {
					Text             string         `json:"text"`
					ThoughtSignature string         `json:"thoughtSignature"`
					FunctionCall     map[string]any `json:"functionCall"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		requests = append(requests, payload)
		if len(requests) == 1 {
			http.Error(w, `{"error":{"status":"INVALID_ARGUMENT","message":"Corrupted thought signature."}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"recovered"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_gemini_signature_retry",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{{
			Role: "assistant",
			Parts: []contract.ContentPart{
				{Kind: contract.ContentPartThinking, Text: "visible thought", OriginProtocol: "gemini", Metadata: map[string]any{"thoughtSignature": "stale_sig"}},
				{Kind: contract.ContentPartThinking, Metadata: map[string]any{"type": "redacted_thinking", "data": "opaque"}},
				{Kind: contract.ContentPartText, Text: "answer"},
			},
		}},
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if conversationResponseText(resp) != "recovered" || !stringSliceContains(resp.Warnings, "gemini_signature_sensitive_history_downgraded") {
		t.Fatalf("expected recovered response with downgrade warning, got %+v", resp)
	}
	if len(requests) != 2 {
		t.Fatalf("expected one retry, got %d requests", len(requests))
	}
	if requests[0].Contents[0].Parts[0].ThoughtSignature != "stale_sig" {
		t.Fatalf("expected first request to preserve signature, got %+v", requests[0])
	}
	if len(requests[1].Contents[0].Parts) != 2 ||
		requests[1].Contents[0].Parts[0].Text != "visible thought" ||
		requests[1].Contents[0].Parts[0].ThoughtSignature != "" ||
		requests[1].Contents[0].Parts[1].Text != "answer" {
		t.Fatalf("expected retry to remove signatures and opaque thinking only, got %+v", requests[1])
	}
}

func TestGeminiCompatibleAdapterRetriesSignatureErrorWithDowngradedTools(t *testing.T) {
	requestBodies := make([][]byte, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		requestBodies = append(requestBodies, body)
		if len(requestBodies) == 1 {
			http.Error(w, `{"error":{"status":"INVALID_ARGUMENT","message":"Expected thinking or function response with valid signature."}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"tool recovered"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_gemini_tool_signature_retry",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{{
			Role: "assistant",
			Parts: []contract.ContentPart{{
				Kind:              contract.ContentPartToolUse,
				ToolCallID:        "call_1",
				ToolName:          "lookup",
				ToolArgumentsJSON: `{"query":"weather"}`,
				OriginProtocol:    "gemini",
				Metadata:          map[string]any{"thoughtSignature": "bad_tool_sig"},
			}},
		}},
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if conversationResponseText(resp) != "tool recovered" || !stringSliceContains(resp.Warnings, "gemini_signature_sensitive_history_downgraded") {
		t.Fatalf("expected recovered response with downgrade warning, got %+v", resp)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected one retry, got %d requests", len(requestBodies))
	}
	var firstPayload, secondPayload struct {
		Contents []struct {
			Parts []struct {
				Text             string         `json:"text"`
				ThoughtSignature string         `json:"thoughtSignature"`
				FunctionCall     map[string]any `json:"functionCall"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(requestBodies[0], &firstPayload); err != nil {
		t.Fatalf("decode first request: %v", err)
	}
	if err := json.Unmarshal(requestBodies[1], &secondPayload); err != nil {
		t.Fatalf("decode second request: %v", err)
	}
	if len(firstPayload.Contents) != 1 ||
		len(firstPayload.Contents[0].Parts) != 1 ||
		firstPayload.Contents[0].Parts[0].FunctionCall["name"] != "lookup" ||
		firstPayload.Contents[0].Parts[0].ThoughtSignature != "bad_tool_sig" {
		t.Fatalf("expected first request to preserve tool signature, got %+v", firstPayload)
	}
	if len(secondPayload.Contents) != 1 ||
		len(secondPayload.Contents[0].Parts) != 1 ||
		secondPayload.Contents[0].Parts[0].FunctionCall != nil ||
		!strings.Contains(secondPayload.Contents[0].Parts[0].Text, "[tool_call name=lookup id=call_1 arguments={\"query\":\"weather\"}]") {
		t.Fatalf("expected retry to downgrade tool call to text, got %+v", secondPayload)
	}
}

func TestGeminiCompatibleAdapterPreservesFunctionCallResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"thoughtSignature":"sig_gemini_1","functionCall":{"name":"lookup","args":{"query":"weather"}}}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_tool_call",
		Model:      "gemini-local",
		InputParts: textParts("call lookup"),
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse || resp.Usage.OutputTokens != 1 {
		t.Fatalf("unexpected gemini tool call response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "", "lookup", `{"query":"weather"}`)
	if resp.Parts[0].Metadata["signature"] != "sig_gemini_1" ||
		resp.Parts[0].Metadata["thoughtSignature"] != "sig_gemini_1" {
		t.Fatalf("expected Gemini thoughtSignature metadata, got %+v", resp.Parts[0])
	}
}

func TestGeminiCompatibleAdapterPreservesFinishReason(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"role":"model","parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_max_tokens",
		Model:      "gemini-local",
		InputParts: textParts("short"),
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "models/gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if resp.StopReason != contract.StopReasonMaxTokens || conversationResponseText(resp) != "partial" {
		t.Fatalf("expected Gemini MAX_TOKENS stop reason, got %+v", resp)
	}
}

func TestGeminiCompatibleAdapterStreamsUpstream(t *testing.T) {
	rawSSE := ": keep-alive\n" +
		"data: {\"candidates\":[{\"content\":\n" +
		"data: {\"parts\":[{\"text\":\"hello\"}]}}]}\n\n" +
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" stream\"}]}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":6,\"totalTokenCount\":11}}\n\n"
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
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_stream",
		Model:      "gemini-local",
		InputParts: textParts("stream gemini"),
		Stream:     true,
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
	if conversationResponseText(resp) != "hello stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 {
		t.Fatalf("unexpected gemini stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw Gemini stream to be preserved\nexpected:\n%s\nactual:\n%s", rawSSE, string(resp.Raw))
	}
	if len(resp.StreamEvents) < 3 {
		t.Fatalf("expected Gemini stream events, got %+v", resp.StreamEvents)
	}
	if resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[0].Delta.Text != "hello" {
		t.Fatalf("expected first Gemini content delta, got %+v", resp.StreamEvents[0])
	}
	if want := "{\"candidates\":[{\"content\":\n{\"parts\":[{\"text\":\"hello\"}]}}]}"; string(resp.StreamEvents[0].Raw) != want {
		t.Fatalf("expected first Gemini raw event payload %q, got %q", want, string(resp.StreamEvents[0].Raw))
	}
	if resp.StreamEvents[1].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[1].Delta.Text != " stream" {
		t.Fatalf("expected second Gemini content delta preserving leading space, got %+v", resp.StreamEvents[1])
	}
	if resp.StreamEvents[2].Type != contract.ConversationStreamEventUsage || resp.StreamEvents[2].Usage.InputTokens != 5 {
		t.Fatalf("expected Gemini usage stream event, got %+v", resp.StreamEvents[2])
	}
}

func TestGeminiCompatibleAdapterStreamsPartIndexes(t *testing.T) {
	rawSSE := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"first\"},{\"text\":\"second\"}]}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":2}}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_stream_parts",
		Model:      "gemini-local",
		InputParts: textParts("stream gemini parts"),
		Stream:     true,
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
	textEvents := conversationStreamEventsByType(resp.StreamEvents, contract.ConversationStreamEventContentDelta)
	if len(textEvents) != 2 {
		t.Fatalf("expected two Gemini text stream events, got %+v", resp.StreamEvents)
	}
	if textEvents[0].ContentIndex != 0 || textEvents[1].ContentIndex != 1 {
		t.Fatalf("expected Gemini stream part indexes, got %+v", textEvents)
	}
}

func TestGeminiCompatibleAdapterStreamsFunctionCall(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"lookup\",\"args\":{\"query\":\"weather\"}}}]}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":1}}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_stream_tool",
		Model:      "gemini-local",
		InputParts: textParts("call lookup"),
		Stream:     true,
		Provider:   providercontract.Provider{AdapterType: "native-gemini", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini stream: %v", err)
	}
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse {
		t.Fatalf("unexpected gemini stream tool response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "", "lookup", `{"query":"weather"}`)
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_models_base",
		Model:      "gemini-local",
		InputParts: textParts("hello"),
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta/models"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("invoke gemini upstream: %v", err)
	}
	if conversationResponseText(resp) != "ok" {
		t.Fatalf("unexpected gemini response: %+v", resp)
	}
}

func TestGeminiCompatibleAdapterCountsTokensUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-pro:countTokens" || r.URL.Query().Get("key") != "gemini-secret" {
			t.Fatalf("unexpected token count upstream request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		var payload struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode token count payload: %v", err)
		}
		if len(payload.Contents) != 1 || payload.Contents[0].Parts[0].Text != "count these tokens" {
			t.Fatalf("unexpected token count payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalTokens":13,"cachedContentTokenCount":2,"promptTokensDetails":[{"modality":"TEXT","tokenCount":11}]}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeTokenCount(context.Background(), contract.TokenCountRequest{
		RequestID:  "req_gemini_count",
		Model:      "gemini-local",
		RawBody:    []byte(`{"contents":[{"role":"user","parts":[{"text":"count these tokens"}]}]}`),
		Provider:   providercontract.Provider{AdapterType: "native-gemini", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	if err != nil {
		t.Fatalf("count gemini tokens: %v", err)
	}
	if resp.TotalTokens != 13 || resp.CachedContentTokenCount == nil || *resp.CachedContentTokenCount != 2 || len(resp.PromptTokensDetails) != 1 || resp.PromptTokensDetails[0].Modality != "TEXT" {
		t.Fatalf("unexpected token count response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterCountsTokensUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Fatalf("unexpected token count upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "anthropic-secret" {
			t.Fatalf("unexpected x-api-key %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("unexpected anthropic-version %q", got)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode Anthropic count payload: %v", err)
		}
		if payload.Model != "claude-upstream" || len(payload.Messages) != 1 || payload.Messages[0].Content != "count anthropic tokens" {
			t.Fatalf("unexpected Anthropic count payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":17,"cache_creation_input_tokens":1}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeTokenCount(context.Background(), contract.TokenCountRequest{
		RequestID:  "req_anthropic_count",
		Model:      "claude-local",
		RawBody:    []byte(`{"model":"claude-local","messages":[{"role":"user","content":"count anthropic tokens"}]}`),
		Provider:   providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 2, Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("count Anthropic tokens: %v", err)
	}
	if resp.TotalTokens != 17 || resp.Metadata["cache_creation_input_tokens"] == nil {
		t.Fatalf("unexpected Anthropic count response: %+v", resp)
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_error",
		Model:      "gemini-local",
		InputParts: textParts("hello"),
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	assertProviderError(t, err, "rate_limit", http.StatusTooManyRequests)
}

func TestGeminiCompatibleAdapterClassifiesStreamErrorFrame(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: error\ndata: {\"error\":{\"status\":\"RESOURCE_EXHAUSTED\",\"message\":\"quota exhausted\",\"code\":429}}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_gemini_stream_error",
		Model:      "gemini-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Provider:   providercontract.Provider{AdapterType: "gemini-compatible", Protocol: "gemini-compatible"},
		Account:    accountcontract.ProviderAccount{ID: 1, Metadata: map[string]any{"base_url": upstream.URL + "/v1beta"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"api_key": "gemini-secret"},
	})
	providerErr := assertProviderError(t, err, "rate_limit", http.StatusTooManyRequests)
	if providerErr.Message != "quota exhausted" {
		t.Fatalf("expected Gemini stream error message to be preserved, got %+v", providerErr)
	}
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_reverse_gemini",
		Model:      "gemini-local",
		InputParts: textParts("hello"),
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
	if conversationResponseText(resp) != "gemini runtime response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
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

func TestReverseProxyGeminiAdapterPreservesSameProtocolRawBody(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"candidates":[{"content":{"parts":[{"text":"gemini raw runtime"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_reverse_gemini_raw",
		SourceProtocol: "gemini-compatible",
		SourceEndpoint: "/v1beta/models/gemini-local:generateContent",
		TargetProtocol: "gemini-compatible",
		Model:          "gemini-local",
		InputParts:     textParts("canonical fallback"),
		RawBody:        []byte(`{"contents":[{"role":"user","parts":[{"text":"raw"}]}],"cachedContent":"cachedContents/raw"}`),
		Provider:       providercontract.Provider{AdapterType: "reverse-proxy-gemini-cli", Protocol: "gemini-compatible"},
		Account: accountcontract.ProviderAccount{
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("gemini_cli"),
			Metadata:       map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke reverse raw gemini: %v", err)
	}
	if conversationResponseText(resp) != "gemini raw runtime" {
		t.Fatalf("unexpected reverse raw gemini response: %+v", resp)
	}
	var payload map[string]any
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode reverse raw gemini payload: %v", err)
	}
	if payload["cachedContent"] != "cachedContents/raw" || payload["model"] != nil {
		t.Fatalf("expected reverse Gemini raw body to be preserved, got %+v", payload)
	}
}

func TestReverseProxyGeminiAdapterRetriesSignatureErrorWithDowngradedThinking(t *testing.T) {
	runtime := sequenceRuntime{
		responses: []reverseproxycontract.Response{
			{
				StatusCode: http.StatusBadRequest,
				Body:       []byte(`{"error":{"status":"INVALID_ARGUMENT","message":"Corrupted thought signature."}}`),
			},
			{
				StatusCode: http.StatusOK,
				Body:       []byte(`{"candidates":[{"content":{"parts":[{"text":"gemini runtime recovered"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1}}`),
			},
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_reverse_gemini_signature_retry",
		Model:     "gemini-local",
		Messages: []contract.ConversationMessage{{
			Role: "assistant",
			Parts: []contract.ContentPart{{
				Kind:           contract.ContentPartThinking,
				Text:           "runtime thought",
				OriginProtocol: "gemini",
				Metadata:       map[string]any{"thoughtSignature": "stale_runtime_sig"},
			}},
		}},
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-gemini-cli",
			Protocol:    "gemini-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("gemini_cli"),
			Metadata:       map[string]any{"base_url": "https://generativelanguage.googleapis.com/v1beta"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke reverse gemini adapter: %v", err)
	}
	if conversationResponseText(resp) != "gemini runtime recovered" || !stringSliceContains(resp.Warnings, "gemini_signature_sensitive_history_downgraded") {
		t.Fatalf("expected recovered response with downgrade warning, got %+v", resp)
	}
	if len(runtime.requests) != 2 {
		t.Fatalf("expected one reverse proxy retry, got %d requests", len(runtime.requests))
	}
	var firstPayload, secondPayload struct {
		Contents []struct {
			Parts []struct {
				Text             string `json:"text"`
				ThoughtSignature string `json:"thoughtSignature"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(runtime.requests[0].Body, &firstPayload); err != nil {
		t.Fatalf("decode first request: %v", err)
	}
	if err := json.Unmarshal(runtime.requests[1].Body, &secondPayload); err != nil {
		t.Fatalf("decode second request: %v", err)
	}
	if firstPayload.Contents[0].Parts[0].ThoughtSignature != "stale_runtime_sig" {
		t.Fatalf("expected first reverse request to preserve signature, got %+v", firstPayload)
	}
	if secondPayload.Contents[0].Parts[0].Text != "runtime thought" ||
		secondPayload.Contents[0].Parts[0].ThoughtSignature != "" {
		t.Fatalf("expected reverse retry to downgrade thinking signature, got %+v", secondPayload)
	}
}

func TestReverseProxyGeminiAdapterCountsTokensThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"totalTokens":21}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeTokenCount(context.Background(), contract.TokenCountRequest{
		RequestID: "req_reverse_gemini_count",
		Model:     "gemini-local",
		RawBody:   []byte(`{"contents":[{"parts":[{"text":"hello"}]}]}`),
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
		t.Fatalf("invoke reverse gemini token count: %v", err)
	}
	if resp.TotalTokens != 21 {
		t.Fatalf("unexpected reverse gemini count response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:countTokens" {
		t.Fatalf("unexpected reverse gemini count request: %+v", runtime.request)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) || runtime.request.Account.Credential["cli_client_token"] != "cli-token" {
		t.Fatalf("expected selected-account runtime context, got %+v", runtime.request.Account)
	}
}

func TestReverseProxyClaudeCodeAdapterCountsTokensThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"input_tokens":29}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeTokenCount(context.Background(), contract.TokenCountRequest{
		RequestID: "req_reverse_claude_count",
		Model:     "claude-local",
		RawBody:   []byte(`{"model":"claude-local","messages":[{"role":"user","content":"hello"}]}`),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-claude-code-cli",
			Protocol:    "anthropic-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             12,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("claude_code_cli"),
			Metadata: map[string]any{
				"base_url":                 "https://upstream.example/v1",
				"user_agent":               "Claude-Code/1.0",
				"claude_code_session_id":   "session-123",
				"claude_client_request_id": "client-req-123",
				"claude_code_version":      "2.1.63",
				"claude_code_build":        "abc123",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke reverse Claude Code token count: %v", err)
	}
	if resp.TotalTokens != 29 {
		t.Fatalf("unexpected Claude Code count response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://upstream.example/v1/messages/count_tokens?beta=true" {
		t.Fatalf("unexpected Claude Code count request: %+v", runtime.request)
	}
	if headerValue(runtime.request.Headers, "Anthropic-Version") != "2023-06-01" ||
		!strings.Contains(headerValue(runtime.request.Headers, "Anthropic-Beta"), "claude-code-20250219") ||
		!strings.Contains(headerValue(runtime.request.Headers, "Anthropic-Beta"), "token-counting-2024-11-01") ||
		headerValue(runtime.request.Headers, "X-Claude-Code-Session-Id") != "session-123" ||
		headerValue(runtime.request.Headers, "x-client-request-id") != "client-req-123" ||
		headerValue(runtime.request.Headers, "Accept") != "application/json" {
		t.Fatalf("unexpected Claude Code count headers: %+v", runtime.request.Headers)
	}
	if runtime.request.Headers.Get("x-api-key") != "" || runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("adapter must leave auth injection to runtime, got %+v", runtime.request.Headers)
	}
	var payload struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode Claude Code count payload: %v", err)
	}
	if payload.Model != "claude-upstream" || len(payload.Messages) != 1 || payload.Messages[0].Content != "hello" || len(payload.System) < 2 ||
		!strings.HasPrefix(payload.System[0].Text, "x-anthropic-billing-header: cc_version=2.1.63.abc123;") ||
		payload.System[1].Text != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("unexpected Claude Code count payload: %+v", payload)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) || runtime.request.Account.Credential["cli_client_token"] != "cli-token" {
		t.Fatalf("expected selected-account runtime context, got %+v", runtime.request.Account)
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:       "req_anthropic",
		Model:           "claude-local",
		InputParts:      textParts("hello anthropic"),
		Instructions:    "be concise",
		MaxOutputTokens: ptrInt(128),
		Messages: []contract.ConversationMessage{
			{Role: "system", Parts: textParts("system from chat")},
			{Role: "user", Parts: textParts("hello anthropic")},
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
	if conversationResponseText(resp) != "anthropic says hi" || resp.Usage.Estimated || resp.Usage.InputTokens != 6 || resp.Usage.OutputTokens != 7 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected anthropic adapter response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterRendersBlockContentToUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string           `json:"role"`
				Content []map[string]any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Messages) != 2 {
			t.Fatalf("expected user blocks and tool result messages, got %+v", payload.Messages)
		}
		if payload.Messages[0].Role != "user" || len(payload.Messages[0].Content) != 2 || payload.Messages[0].Content[0]["text"] != "inspect" {
			t.Fatalf("unexpected Anthropic user content: %+v", payload.Messages[0])
		}
		source, _ := payload.Messages[0].Content[1]["source"].(map[string]any)
		if payload.Messages[0].Content[1]["type"] != "image" || source["url"] != "https://example.test/image.png" {
			t.Fatalf("expected Anthropic image block, got %+v", payload.Messages[0].Content[1])
		}
		if payload.Messages[1].Role != "user" || payload.Messages[1].Content[0]["type"] != "tool_result" || payload.Messages[1].Content[0]["tool_use_id"] != "call_1" {
			t.Fatalf("unexpected Anthropic tool result content: %+v", payload.Messages[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}],"usage":{"input_tokens":5,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_anthropic_blocks",
		Model:     "claude-local",
		Messages: []contract.ConversationMessage{
			{Role: "user", Parts: []contract.ContentPart{
				{Kind: contract.ContentPartText, Text: "inspect"},
				imageURLPart("https://example.test/image.png"),
			}},
			{Role: "tool", Parts: []contract.ContentPart{toolResultPart("call_1", "sunny")}},
		},
		Provider:   providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
}

func TestAnthropicCompatibleAdapterPreservesNestedToolResultContent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string           `json:"role"`
				Content []map[string]any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Messages) != 1 || len(payload.Messages[0].Content) != 1 {
			t.Fatalf("expected one tool result message, got %+v", payload.Messages)
		}
		result := payload.Messages[0].Content[0]
		nested, _ := result["content"].([]any)
		if result["type"] != "tool_result" || result["tool_use_id"] != "call_1" || len(nested) != 2 {
			t.Fatalf("expected nested tool_result content, got %+v", result)
		}
		text, _ := nested[0].(map[string]any)
		image, _ := nested[1].(map[string]any)
		source, _ := image["source"].(map[string]any)
		if text["type"] != "text" || text["text"] != "File metadata: 800x600 PNG" {
			t.Fatalf("expected non-empty nested text, got %+v", nested)
		}
		if image["type"] != "image" || source["media_type"] != "image/png" || source["data"] != "iVBOR" {
			t.Fatalf("expected nested image source, got %+v", nested)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}],"usage":{"input_tokens":5,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_anthropic_nested_tool_result",
		Model:     "claude-local",
		Messages: []contract.ConversationMessage{{
			Role: "tool",
			Parts: []contract.ContentPart{{
				Kind:            contract.ContentPartToolResult,
				ToolResultForID: "call_1",
				Text:            "File metadata: 800x600 PNG",
				Metadata: map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "File metadata: 800x600 PNG"},
						map[string]any{"type": "text", "text": ""},
						map[string]any{
							"type": "image",
							"source": map[string]any{
								"type":       "base64",
								"media_type": "image/png",
								"data":       "iVBOR",
							},
						},
					},
				},
			}},
		}},
		Provider:   providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
}

func TestAnthropicCompatibleAdapterPreservesBlockCacheControl(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string           `json:"role"`
				Content []map[string]any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if len(payload.Messages) != 2 {
			t.Fatalf("expected user and tool result messages, got %+v", payload.Messages)
		}
		textCache, _ := payload.Messages[0].Content[0]["cache_control"].(map[string]any)
		if payload.Messages[0].Role != "user" ||
			payload.Messages[0].Content[0]["type"] != "text" ||
			textCache["type"] != "ephemeral" ||
			textCache["ttl"] != "1h" {
			t.Fatalf("expected text cache_control to be preserved, got %+v", payload.Messages[0])
		}
		resultCache, _ := payload.Messages[1].Content[0]["cache_control"].(map[string]any)
		if payload.Messages[1].Role != "user" ||
			payload.Messages[1].Content[0]["type"] != "tool_result" ||
			resultCache["type"] != "ephemeral" {
			t.Fatalf("expected tool result cache_control to be preserved, got %+v", payload.Messages[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}],"usage":{"input_tokens":5,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_anthropic_cache_control",
		Model:     "claude-local",
		Messages: []contract.ConversationMessage{
			{Role: "user", Parts: []contract.ContentPart{{
				Kind:     contract.ContentPartText,
				Text:     "cached context",
				Metadata: map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}, "internal_note": "drop"},
			}}},
			{Role: "tool", Parts: []contract.ContentPart{{
				Kind:            contract.ContentPartToolResult,
				ToolResultForID: "call_1",
				Text:            "sunny",
				Metadata:        map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
			}}},
		},
		Provider:   providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
}

func TestAnthropicCompatibleAdapterPreservesSameProtocolRawBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload["model"] != "claude-upstream" || payload["service_tier"] != "auto" || payload["container"] == nil {
			t.Fatalf("expected raw Anthropic fields with mapped model, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"anthropic raw ok"}],"usage":{"input_tokens":2,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_anthropic_raw",
		SourceProtocol: "anthropic-compatible",
		SourceEndpoint: "/v1/messages",
		TargetProtocol: "anthropic-compatible",
		Model:          "claude-local",
		InputParts:     textParts("canonical fallback"),
		RawBody:        []byte(`{"model":"claude-local","messages":[{"role":"user","content":"raw"}],"service_tier":"auto","container":{"id":"container-1"}}`),
		Provider:       providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:        accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential:     map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke raw Anthropic upstream: %v", err)
	}
	if conversationResponseText(resp) != "anthropic raw ok" {
		t.Fatalf("unexpected raw Anthropic response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterPreservesThinkingConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			MaxTokens int            `json:"max_tokens"`
			Thinking  map[string]any `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.MaxTokens != 4096 ||
			payload.Thinking["type"] != "enabled" ||
			payload.Thinking["budget_tokens"].(float64) != 2048 {
			t.Fatalf("expected Anthropic thinking config to be preserved, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"thinking config ok"}],"usage":{"input_tokens":2,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	maxTokens := 4096
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:       "req_anthropic_thinking_config",
		Model:           "claude-local",
		InputParts:      textParts("think"),
		MaxOutputTokens: &maxTokens,
		Reasoning:       map[string]any{"type": "enabled", "budget_tokens": 2048},
		Provider:        providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:         accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:         modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential:      map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
	if conversationResponseText(resp) != "thinking config ok" {
		t.Fatalf("unexpected anthropic response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterRectifiesThinkingBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			MaxTokens int            `json:"max_tokens"`
			Thinking  map[string]any `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.MaxTokens != 2048 ||
			payload.Thinking["type"] != "enabled" ||
			payload.Thinking["budget_tokens"].(float64) != 2047 {
			t.Fatalf("expected Anthropic thinking budget to be rectified below max_tokens, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"budget ok"}],"usage":{"input_tokens":2,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	maxTokens := 2048
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:       "req_anthropic_thinking_budget",
		Model:           "claude-local",
		InputParts:      textParts("think"),
		MaxOutputTokens: &maxTokens,
		Reasoning:       map[string]any{"type": "enabled", "budget_tokens": 32000},
		Provider:        providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:         accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:         modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential:      map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
}

func TestAnthropicCompatibleAdapterDoesNotSendOpenAIReasoningAsThinking(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if _, ok := payload["thinking"]; ok {
			t.Fatalf("did not expect OpenAI reasoning shape to be sent as Anthropic thinking, got %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"reasoning ignored"}],"usage":{"input_tokens":2,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_openai_reasoning",
		Model:      "claude-local",
		InputParts: textParts("think"),
		Reasoning:  map[string]any{"effort": "high"},
		Provider:   providercontract.Provider{AdapterType: "anthropic-compatible", Protocol: "anthropic-compatible"},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	if err != nil {
		t.Fatalf("invoke anthropic upstream: %v", err)
	}
}

func TestAnthropicCompatibleAdapterPreservesToolUseResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"query":"weather"}}],"usage":{"input_tokens":6,"output_tokens":2}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_tool_use",
		Model:      "claude-local",
		InputParts: textParts("call lookup"),
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
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected anthropic tool use response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "toolu_1", "lookup", `{"query":"weather"}`)
}

func TestAnthropicCompatibleAdapterPreservesToolUseSignatureResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"query":"weather"},"signature":"sig_tool_abc"}],"usage":{"input_tokens":6,"output_tokens":2}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_tool_use_signature",
		Model:      "claude-local",
		InputParts: textParts("call lookup"),
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
	assertToolUsePart(t, resp.Parts[0], "toolu_1", "lookup", `{"query":"weather"}`)
	if resp.Parts[0].Metadata["signature"] != "sig_tool_abc" {
		t.Fatalf("expected Anthropic tool signature metadata, got %+v", resp.Parts[0])
	}
}

func TestAnthropicCompatibleAdapterPreservesThinkingBlocks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stop_reason":"end_turn","content":[{"type":"thinking","thinking":"private chain","signature":"sig_think_1"},{"type":"redacted_thinking","data":"enc_think_1"},{"type":"text","text":"final"}],"usage":{"input_tokens":6,"output_tokens":3}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_thinking",
		Model:      "claude-local",
		InputParts: textParts("think"),
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
	if len(resp.Parts) != 3 || resp.Parts[0].Kind != contract.ContentPartThinking || resp.Parts[0].Text != "private chain" {
		t.Fatalf("expected thinking, redacted_thinking, and text parts, got %+v", resp.Parts)
	}
	if resp.Parts[0].Metadata["signature"] != "sig_think_1" {
		t.Fatalf("expected thinking signature metadata, got %+v", resp.Parts[0])
	}
	if resp.Parts[1].Kind != contract.ContentPartThinking ||
		resp.Parts[1].Metadata["type"] != "redacted_thinking" ||
		resp.Parts[1].Metadata["data"] != "enc_think_1" ||
		string(resp.Parts[1].Raw) == "" {
		t.Fatalf("expected redacted_thinking data and raw payload, got %+v", resp.Parts[1])
	}
}

func TestAnthropicCompatibleAdapterStreamsUpstream(t *testing.T) {
	rawSSE := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\n" +
		"data: \"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" stream\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":6,\"cache_creation_input_tokens\":1}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
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
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_stream",
		Model:      "claude-local",
		InputParts: textParts("hello stream"),
		Stream:     true,
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
	if conversationResponseText(resp) != "hello stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected anthropic stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw Anthropic stream to be preserved\nexpected:\n%s\nactual:\n%s", rawSSE, string(resp.Raw))
	}
	if len(resp.StreamEvents) < 4 {
		t.Fatalf("expected Anthropic stream events, got %+v", resp.StreamEvents)
	}
	if resp.StreamEvents[1].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[1].Delta.Text != "hello" {
		t.Fatalf("expected first Anthropic content delta, got %+v", resp.StreamEvents[1])
	}
	if want := "{\"type\":\"content_block_delta\",\n\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}"; string(resp.StreamEvents[1].Raw) != want {
		t.Fatalf("expected first Anthropic raw event payload %q, got %q", want, string(resp.StreamEvents[1].Raw))
	}
	if resp.StreamEvents[2].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[2].Delta.Text != " stream" {
		t.Fatalf("expected second Anthropic content delta preserving leading space, got %+v", resp.StreamEvents[2])
	}
	if resp.StreamEvents[3].Type != contract.ConversationStreamEventUsage || resp.StreamEvents[3].Usage.OutputTokens != 6 {
		t.Fatalf("expected Anthropic usage stream event, got %+v", resp.StreamEvents[3])
	}
	if resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected Anthropic terminal stop stream event, got %+v", resp.StreamEvents)
	}
}

func TestAnthropicCompatibleAdapterUsesNamedSSEEventType(t *testing.T) {
	rawSSE := "event: message_start\n" +
		"data: {\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"named\"}}\n\n" +
		"event: message_delta\n" +
		"data: {\"usage\":{\"output_tokens\":6,\"cache_read_input_tokens\":1}}\n\n" +
		"event: message_stop\n" +
		"data: {}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_named_stream",
		Model:      "claude-local",
		InputParts: textParts("hello stream"),
		Stream:     true,
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
		t.Fatalf("invoke anthropic named stream upstream: %v", err)
	}
	if conversationResponseText(resp) != "named" || resp.Usage.Estimated || resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 6 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected anthropic named stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw Anthropic named stream to be preserved\nexpected:\n%s\nactual:\n%s", rawSSE, string(resp.Raw))
	}
	if len(resp.StreamEvents) < 4 ||
		resp.StreamEvents[0].Type != contract.ConversationStreamEventUsage ||
		resp.StreamEvents[0].RawEventType != "message_start" ||
		resp.StreamEvents[1].Type != contract.ConversationStreamEventContentDelta ||
		resp.StreamEvents[1].RawEventType != "content_block_delta" ||
		string(resp.StreamEvents[1].Raw) != `{"delta":{"type":"text_delta","text":"named"}}` ||
		resp.StreamEvents[2].Type != contract.ConversationStreamEventUsage ||
		resp.StreamEvents[2].RawEventType != "message_delta" ||
		resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected Anthropic named stream events, got %+v", resp.StreamEvents)
	}
}

func TestAnthropicCompatibleAdapterPreservesContextManagement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var payload struct {
			Model             string         `json:"model"`
			Thinking          map[string]any `json:"thinking"`
			ContextManagement map[string]any `json:"context_management"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "claude-upstream" || payload.Thinking["type"] != "enabled" || payload.Thinking["budget_tokens"] != float64(2048) {
			t.Fatalf("unexpected thinking payload: %+v", payload)
		}
		edits, ok := payload.ContextManagement["edits"].([]any)
		if !ok || len(edits) != 1 {
			t.Fatalf("expected context_management edits, got %+v", payload.ContextManagement)
		}
		edit, ok := edits[0].(map[string]any)
		if !ok || edit["type"] != "clear_thinking_20251015" || edit["trigger"] != "input_tokens" || edit["value"] != float64(20000) {
			t.Fatalf("unexpected context_management edit: %+v", edits[0])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"kept context"}],"usage":{"input_tokens":3,"output_tokens":2}}`))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:         "req_anthropic_context_management",
		Model:             "claude-local",
		InputParts:        textParts("hello"),
		MaxOutputTokens:   ptrInt(4096),
		Reasoning:         map[string]any{"type": "enabled", "budget_tokens": 2048},
		ContextManagement: map[string]any{"edits": []any{map[string]any{"type": "clear_thinking_20251015", "trigger": "input_tokens", "value": 20000}}},
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
	if conversationResponseText(resp) != "kept context" {
		t.Fatalf("unexpected anthropic response: %+v", resp)
	}
}

func TestAnthropicCompatibleAdapterStreamsThinkingSignature(t *testing.T) {
	rawSSE := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"private \"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"chain\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig_\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"think\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":6}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_stream_thinking",
		Model:      "claude-local",
		InputParts: textParts("think"),
		Stream:     true,
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
	if len(resp.Parts) != 1 || resp.Parts[0].Kind != contract.ContentPartThinking || resp.Parts[0].Text != "private chain" {
		t.Fatalf("expected streamed thinking part, got %+v", resp.Parts)
	}
	if resp.Parts[0].Metadata["signature"] != "sig_think" {
		t.Fatalf("expected aggregated thinking signature, got %+v", resp.Parts[0])
	}
	reasoningEvents := conversationStreamEventsByType(resp.StreamEvents, contract.ConversationStreamEventReasoning)
	if len(reasoningEvents) != 4 ||
		reasoningEvents[0].Delta.Text != "private " ||
		reasoningEvents[1].Delta.Text != "chain" ||
		reasoningEvents[2].Delta.Metadata["signature_delta"] != "sig_" ||
		reasoningEvents[3].Delta.Metadata["signature_delta"] != "think" {
		t.Fatalf("expected thinking text and signature deltas, got %+v", reasoningEvents)
	}
}

func TestAnthropicCompatibleAdapterStreamsToolUseEvents(t *testing.T) {
	rawSSE := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup\",\"input\":{}}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"weather\\\"}\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":2}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_tool_stream",
		Model:      "claude-local",
		InputParts: textParts("call lookup"),
		Stream:     true,
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
		t.Fatalf("invoke anthropic tool stream upstream: %v", err)
	}
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected anthropic tool stream response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "toolu_1", "lookup", `{"query":"weather"}`)
	if len(resp.StreamEvents) < 5 {
		t.Fatalf("expected Anthropic tool stream events, got %+v", resp.StreamEvents)
	}
	start := resp.StreamEvents[1]
	if start.Type != contract.ConversationStreamEventToolCallDelta || start.Delta.ToolCallID != "toolu_1" || start.Delta.ToolName != "lookup" || start.Delta.ToolArgumentsJSON != "" {
		t.Fatalf("expected Anthropic tool start event, got %+v", start)
	}
	firstDelta := resp.StreamEvents[2]
	secondDelta := resp.StreamEvents[3]
	if firstDelta.Type != contract.ConversationStreamEventToolCallDelta || firstDelta.Delta.ToolArgumentsJSON != `{"query":` {
		t.Fatalf("expected first Anthropic tool delta, got %+v", firstDelta)
	}
	if secondDelta.Type != contract.ConversationStreamEventToolCallDelta || secondDelta.Delta.ToolArgumentsJSON != `"weather"}` {
		t.Fatalf("expected second Anthropic tool delta, got %+v", secondDelta)
	}
	if resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected Anthropic terminal stop stream event, got %+v", resp.StreamEvents)
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_rate",
		Model:      "claude-local",
		InputParts: textParts("hello"),
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

func TestAnthropicCompatibleAdapterClassifiesStreamErrorFrame(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"message\":\"slow anthropic\"}}\n\n"))
	}))
	defer upstream.Close()

	svc, err := service.New(upstream.Client())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_anthropic_stream_error",
		Model:      "claude-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Provider: providercontract.Provider{
			AdapterType: "anthropic-compatible",
			Protocol:    "anthropic-compatible",
		},
		Account:    accountcontract.ProviderAccount{Metadata: map[string]any{"base_url": upstream.URL + "/v1"}},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	providerErr := assertProviderError(t, err, "rate_limit", http.StatusTooManyRequests)
	if providerErr.Message != "slow anthropic" {
		t.Fatalf("expected Anthropic stream error message to be preserved, got %+v", providerErr)
	}
}

func TestReverseProxyClaudeCodeCLIAdapterUsesOfficialClientMessagesShape(t *testing.T) {
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_reverse_anthropic",
		Model:      "claude-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-claude-code-cli",
			Protocol:    "anthropic-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             12,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("claude_code_cli"),
			Metadata: map[string]any{
				"base_url":                 "https://upstream.example/v1",
				"user_agent":               "Claude-Code/1.0",
				"claude_code_session_id":   "session-123",
				"claude_client_request_id": "client-req-123",
				"claude_code_version":      "2.1.63",
				"claude_code_build":        "abc123",
				"claude_code_entrypoint":   "cli",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke reverse anthropic adapter: %v", err)
	}
	if conversationResponseText(resp) != "reverse anthropic response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected reverse anthropic response: %+v", resp)
	}
	if runtime.request.URL != "https://upstream.example/v1/messages?beta=true" {
		t.Fatalf("expected Claude Code messages endpoint, got %s", runtime.request.URL)
	}
	if headerValue(runtime.request.Headers, "Anthropic-Version") != "2023-06-01" ||
		!strings.Contains(headerValue(runtime.request.Headers, "Anthropic-Beta"), "claude-code-20250219") ||
		!strings.Contains(headerValue(runtime.request.Headers, "Anthropic-Beta"), "oauth-2025-04-20") ||
		headerValue(runtime.request.Headers, "X-App") != "cli" ||
		headerValue(runtime.request.Headers, "X-Stainless-Retry-Count") != "0" ||
		headerValue(runtime.request.Headers, "X-Stainless-Runtime") != "node" ||
		headerValue(runtime.request.Headers, "X-Stainless-Lang") != "js" ||
		headerValue(runtime.request.Headers, "X-Stainless-Timeout") != "600" ||
		headerValue(runtime.request.Headers, "X-Claude-Code-Session-Id") != "session-123" ||
		headerValue(runtime.request.Headers, "x-client-request-id") != "client-req-123" ||
		headerValue(runtime.request.Headers, "Accept") != "application/json" {
		t.Fatalf("unexpected Claude Code headers: %+v", runtime.request.Headers)
	}
	if runtime.request.Headers.Get("x-api-key") != "" || runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("adapter must leave auth injection to runtime, got %+v", runtime.request.Headers)
	}
	var payload struct {
		Model  string `json:"model"`
		System []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"system"`
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
	if len(payload.System) < 2 ||
		!strings.HasPrefix(payload.System[0].Text, "x-anthropic-billing-header: cc_version=2.1.63.abc123; cc_entrypoint=cli; cch=") ||
		payload.System[1].Text != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("expected Claude Code system blocks, got %+v", payload.System)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "claude_code_cli" ||
		runtime.request.Account.Credential["cli_client_token"] != "cli-token" {
		t.Fatalf("expected claude runtime context, got %+v", runtime.request.Account)
	}
}

func TestReverseProxyClaudeCodeCLIRejectsAPIKeyRuntime(t *testing.T) {
	runtime := capturingRuntime{}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_reverse_claude_api_key",
		Model:      "claude-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-claude-code-cli",
			Protocol:    "anthropic-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             12,
			RuntimeClass:   accountcontract.RuntimeClassAPIKey,
			UpstreamClient: ptrString("claude_code_cli"),
			Metadata:       map[string]any{"base_url": "https://upstream.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"api_key": "anthropic-secret"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
	if runtime.request.URL != "" {
		t.Fatalf("runtime should not be called for api_key Claude Code reverse proxy, got %s", runtime.request.URL)
	}
}

func TestReverseProxyAnthropicCompatibleAdapterPreservesSameProtocolRawBody(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"content":[{"type":"text","text":"anthropic raw runtime"}],"usage":{"input_tokens":2,"output_tokens":1}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_reverse_anthropic_raw",
		SourceProtocol: "anthropic-compatible",
		SourceEndpoint: "/v1/messages",
		TargetProtocol: "anthropic-compatible",
		Model:          "claude-local",
		InputParts:     textParts("canonical fallback"),
		RawBody:        []byte(`{"model":"claude-local","messages":[{"role":"user","content":"raw"}],"service_tier":"auto","container":{"id":"container-1"},"stream":true}`),
		Provider: providercontract.Provider{
			ID:          2,
			AdapterType: "anthropic-compatible",
			Protocol:    "anthropic-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             12,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("claude_code_cli"),
			Metadata:       map[string]any{"base_url": "https://anthropic.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"access_token": "oauth-access"},
	})
	if err != nil {
		t.Fatalf("invoke reverse raw anthropic adapter: %v", err)
	}
	if conversationResponseText(resp) != "anthropic raw runtime" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 1 {
		t.Fatalf("unexpected reverse raw anthropic response: %+v", resp)
	}
	if runtime.request.URL != "https://anthropic.example/v1/messages" || runtime.request.ExpectStream {
		t.Fatalf("unexpected reverse raw anthropic runtime request: %+v", runtime.request)
	}
	var payload map[string]any
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode reverse raw anthropic payload: %v", err)
	}
	container, _ := payload["container"].(map[string]any)
	if payload["model"] != "claude-upstream" ||
		payload["service_tier"] != "auto" ||
		payload["stream"] != false ||
		container["id"] != "container-1" {
		t.Fatalf("expected reverse Anthropic raw body to be preserved with mapped stream fields, got %+v", payload)
	}
}

func TestReverseProxyOpenAICompatibleAdapterUsesRuntimeForNonAPIKeyAccount(t *testing.T) {
	var upstreamHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_reverse_proxy",
		Model:      "rp-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-openai-compatible",
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
	if conversationResponseText(resp) != "reverse proxy response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected reverse proxy adapter response: %+v", resp)
	}
	if upstreamHeaders.Get("User-Agent") != "Codex/1.0" {
		t.Fatalf("expected reverse proxy user agent, got %q", upstreamHeaders.Get("User-Agent"))
	}
	if metrics := runtime.Metrics(); metrics.RequestTotal != 1 || metrics.RequestSuccessTotal != 1 {
		t.Fatalf("expected reverse proxy runtime metrics, got %+v", metrics)
	}
}

func TestReverseProxyOpenAICompatibleAdapterPreservesSameProtocolRawBody(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"choices\":[{\"delta\":{\"content\":\"reverse raw\"}}]}\n\n" +
					"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_reverse_openai_raw",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "gpt-local",
		InputParts:     textParts("canonical fallback"),
		Stream:         true,
		RawBody:        []byte(`{"model":"gpt-local","messages":[{"role":"user","content":"raw"}],"parallel_tool_calls":true,"stream":false,"stream_options":{"include_usage":false},"user":"user-raw"}`),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             7,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://openai.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-upstream"},
		Credential: map[string]any{"access_token": "oauth-access"},
	})
	if err != nil {
		t.Fatalf("invoke reverse raw openai adapter: %v", err)
	}
	if conversationResponseText(resp) != "reverse raw" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 1 {
		t.Fatalf("unexpected reverse raw openai response: %+v", resp)
	}
	if runtime.request.URL != "https://openai.example/v1/chat/completions" || !runtime.request.ExpectStream {
		t.Fatalf("unexpected reverse raw openai runtime request: %+v", runtime.request)
	}
	var payload map[string]any
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode reverse raw openai payload: %v", err)
	}
	streamOptions, _ := payload["stream_options"].(map[string]any)
	if payload["model"] != "gpt-upstream" ||
		payload["parallel_tool_calls"] != true ||
		payload["user"] != "user-raw" ||
		payload["stream"] != true ||
		streamOptions["include_usage"] != true {
		t.Fatalf("expected reverse OpenAI raw body to be preserved with mapped stream fields, got %+v", payload)
	}
}

func TestReverseProxyChatGPTWebAdapterUsesConversationOfficialClientShape(t *testing.T) {
	const chatGPTUserAgent = "Mozilla/5.0 ChatGPTWeb/1.0"
	var upstreamHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		if r.URL.Path != "/backend-api/conversation" {
			t.Fatalf("unexpected chatgpt web upstream path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer chatgpt-web-token" {
			t.Fatalf("expected chatgpt bearer token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("User-Agent") != chatGPTUserAgent ||
			r.Header.Get("Accept") != "text/event-stream" ||
			r.Header.Get("Content-Type") != "application/json" ||
			r.Header.Get("X-OpenAI-Target-Path") != "/backend-api/conversation" ||
			r.Header.Get("X-OpenAI-Target-Route") != "/backend-api/conversation" ||
			r.Header.Get("OAI-Device-Id") != "device-123" ||
			r.Header.Get("OAI-Session-Id") != "session-123" ||
			r.Header.Get("OAI-Client-Version") != "client-version-123" ||
			r.Header.Get("OAI-Client-Build-Number") != "build-123" ||
			r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token") != "requirements-token" {
			t.Fatalf("unexpected chatgpt web headers: %+v", r.Header)
		}
		if r.Header.Get("X-Request-ID") != "" || r.Header.Get("X-SRapi-Test") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		var payload struct {
			Action             string `json:"action"`
			Model              string `json:"model"`
			ForceUseSSE        bool   `json:"force_use_sse"`
			Timezone           string `json:"timezone"`
			TimezoneOffsetMin  int    `json:"timezone_offset_min"`
			ParentMessageID    string `json:"parent_message_id"`
			WebsocketRequestID string `json:"websocket_request_id"`
			ConversationMode   struct {
				Kind string `json:"kind"`
			} `json:"conversation_mode"`
			ClientContextualInfo struct {
				PageWidth int `json:"page_width"`
			} `json:"client_contextual_info"`
			Messages []struct {
				Author struct {
					Role string `json:"role"`
				} `json:"author"`
				Content struct {
					ContentType string   `json:"content_type"`
					Parts       []string `json:"parts"`
				} `json:"content"`
			} `json:"messages"`
			StreamOptions any `json:"stream_options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode chatgpt web payload: %v", err)
		}
		if payload.Action != "next" ||
			payload.Model != "gpt-5-chat-web" ||
			!payload.ForceUseSSE ||
			payload.Timezone != "Asia/Shanghai" ||
			payload.TimezoneOffsetMin != -480 ||
			payload.ConversationMode.Kind != "primary_assistant" ||
			payload.ClientContextualInfo.PageWidth != 1400 ||
			payload.ParentMessageID == "" ||
			payload.WebsocketRequestID == "" ||
			payload.StreamOptions != nil ||
			len(payload.Messages) != 1 ||
			payload.Messages[0].Author.Role != "user" ||
			payload.Messages[0].Content.ContentType != "text" ||
			len(payload.Messages[0].Content.Parts) != 1 ||
			payload.Messages[0].Content.Parts[0] != "hello chatgpt web" {
			t.Fatalf("unexpected chatgpt web payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"chatgpt web response\"]}}}\n\n" +
				"data: [DONE]\n\n",
		))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_chatgpt_web_proxy",
		Model:      "chatgpt-local",
		InputParts: textParts("hello chatgpt web"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             11,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("chatgpt_web"),
			Metadata: map[string]any{
				"base_url":                    upstream.URL,
				"user_agent":                  chatGPTUserAgent,
				"chatgpt_requirements_token":  "requirements-token",
				"oai_device_id":               "device-123",
				"oai_session_id":              "session-123",
				"chatgpt_client_version":      "client-version-123",
				"chatgpt_client_build_number": "build-123",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat-web"},
		Credential: map[string]any{"access_token": "chatgpt-web-token"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "chatgpt web response" || !resp.Usage.Estimated {
		t.Fatalf("unexpected chatgpt web response: %+v", resp)
	}
	if upstreamHeaders.Get("Authorization") != "Bearer chatgpt-web-token" {
		t.Fatalf("expected runtime to inject chatgpt auth, got %+v", upstreamHeaders)
	}
	if metrics := runtime.Metrics(); metrics.RequestTotal != 1 || metrics.RequestSuccessTotal != 1 {
		t.Fatalf("expected reverse proxy runtime metrics, got %+v", metrics)
	}
}

func TestReverseProxyChatGPTWebAdapterAutoFetchesRequirements(t *testing.T) {
	const chatGPTUserAgent = "Mozilla/5.0 ChatGPTWeb/1.0"
	var paths []string
	var conversationHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer chatgpt-web-token" {
			t.Fatalf("expected runtime bearer token on %s, got %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/":
			if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Accept"), "text/html") {
				t.Fatalf("unexpected bootstrap request method=%s headers=%+v", r.Method, r.Header)
			}
			_, _ = w.Write([]byte(`<html data-build="build-123"><script src="/assets/c/test/_build.js"></script></html>`))
		case "/backend-api/sentinel/chat-requirements":
			if r.Method != http.MethodPost ||
				r.Header.Get("Content-Type") != "application/json" ||
				r.Header.Get("X-OpenAI-Target-Path") != "/backend-api/sentinel/chat-requirements" {
				t.Fatalf("unexpected requirements request method=%s headers=%+v", r.Method, r.Header)
			}
			var body struct {
				P string `json:"p"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode requirements request: %v", err)
			}
			if !strings.HasPrefix(body.P, "gAAAAAC") {
				t.Fatalf("expected generated legacy requirements token, got %q", body.P)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"requirements-token-auto","so_token":"so-token","proofofwork":{"required":true,"seed":"seed","difficulty":"ff"}}`))
		case "/backend-api/conversation":
			conversationHeaders = r.Header.Clone()
			if r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token") != "requirements-token-auto" ||
				!strings.HasPrefix(r.Header.Get("OpenAI-Sentinel-Proof-Token"), "gAAAAAB") ||
				r.Header.Get("OpenAI-Sentinel-SO-Token") != "so-token" {
				t.Fatalf("unexpected conversation sentinel headers: %+v", r.Header)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"conversation.delta\",\"delta\":\"auto requirements ok\"}\n\ndata: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_chatgpt_web_auto_requirements",
		Model:      "chatgpt-local",
		InputParts: textParts("hello auto requirements"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             11,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("chatgpt_web"),
			Metadata: map[string]any{
				"base_url":       upstream.URL,
				"user_agent":     chatGPTUserAgent,
				"oai_device_id":  "device-123",
				"oai_session_id": "session-123",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat-web"},
		Credential: map[string]any{"access_token": "chatgpt-web-token"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "auto requirements ok" {
		t.Fatalf("unexpected chatgpt web response: %+v", resp)
	}
	if strings.Join(paths, ",") != "/,/backend-api/sentinel/chat-requirements,/backend-api/conversation" {
		t.Fatalf("unexpected auto requirements request sequence: %+v", paths)
	}
	if conversationHeaders.Get("User-Agent") != chatGPTUserAgent {
		t.Fatalf("expected conversation user agent, got %+v", conversationHeaders)
	}
	if metrics := runtime.Metrics(); metrics.RequestTotal != 3 || metrics.RequestSuccessTotal != 3 {
		t.Fatalf("expected three reverse proxy runtime successes, got %+v", metrics)
	}
}

func TestReverseProxyChatGPTWebMissingRequirementsCanDisableAutoFetch(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"message":{"content":{"parts":["should not be called"]}}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_chatgpt_web_manual_requirements",
		Model:      "chatgpt-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             11,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("chatgpt_web"),
			Metadata:       map[string]any{"base_url": "https://chatgpt.example", "chatgpt_requirements_auto": false},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat-web"},
		Credential: map[string]any{"access_token": "chatgpt-web-token"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
	if runtime.request.URL != "" {
		t.Fatalf("reverse proxy runtime should not be called when requirements are missing and auto fetch is disabled, got %+v", runtime.request)
	}
}

func TestReverseProxyChatGPTWebRejectsAPIKeyRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"message":{"content":{"parts":["should not be called"]}}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_chatgpt_web_api_key_runtime",
		Model:      "chatgpt-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             11,
			RuntimeClass:   accountcontract.RuntimeClassAPIKey,
			UpstreamClient: ptrString("chatgpt_web"),
			Metadata:       map[string]any{"base_url": "https://chatgpt.example", "chatgpt_requirements_token": "requirements-token"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat-web"},
		Credential: map[string]any{"api_key": "sk-secret"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
	if runtime.request.URL != "" {
		t.Fatalf("reverse proxy runtime should not be called, got %+v", runtime.request)
	}
}

func TestReverseProxyCodexCLIAdapterUsesResponsesOfficialClientShape(t *testing.T) {
	const codexUserAgent = "codex_cli_rs/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9"
	var upstreamHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer codex-token" {
			t.Fatalf("expected codex bearer token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Originator") != "codex_cli_rs" ||
			r.Header.Get("User-Agent") != codexUserAgent ||
			r.Header.Get("Accept") != "text/event-stream" ||
			r.Header.Get("OpenAI-Beta") != "responses=experimental" ||
			r.Header.Get("Chatgpt-Account-Id") != "chatgpt-account" ||
			r.Header.Get("Session_id") != "session-123" ||
			r.Header.Get("X-Client-Request-Id") != "req_codex_proxy" ||
			r.Header.Get("X-Codex-Beta-Features") != "feature-a" ||
			r.Header.Get("Version") != "0.118.0" {
			t.Fatalf("unexpected codex headers: %+v", r.Header)
		}
		var payload struct {
			Model        string `json:"model"`
			Instructions string `json:"instructions"`
			Stream       bool   `json:"stream"`
			Store        *bool  `json:"store"`
			Input        []struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"input"`
			StreamOptions any `json:"stream_options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex payload: %v", err)
		}
		if payload.Model != "codex-upstream" ||
			payload.Instructions != "be concise\nsystem guardrail" ||
			!payload.Stream ||
			payload.Store == nil ||
			*payload.Store ||
			payload.StreamOptions != nil ||
			len(payload.Input) != 1 ||
			payload.Input[0].Type != "message" ||
			payload.Input[0].Role != "user" ||
			len(payload.Input[0].Content) != 1 ||
			payload.Input[0].Content[0].Type != "input_text" ||
			payload.Input[0].Content[0].Text != "hello codex" {
			t.Fatalf("unexpected codex payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ignored \"}\n\n" +
				"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"codex response\"}]}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n" +
				"data: [DONE]\n\n",
		))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:    "req_codex_proxy",
		Model:        "codex-local",
		Instructions: "be concise",
		Messages: []contract.ConversationMessage{
			{Role: "system", Parts: textParts("system guardrail")},
			{Role: "user", Parts: textParts("hello codex")},
		},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata: map[string]any{
				"base_url":            upstream.URL + "/backend-api/codex",
				"user_agent":          codexUserAgent,
				"chatgpt_account_id":  "chatgpt-account",
				"codex_session_id":    "session-123",
				"codex_beta_features": "feature-a",
				"codex_version":       "0.118.0",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "codex response" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected codex response: %+v", resp)
	}
	if string(resp.Raw) != "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ignored \"}\n\n"+
		"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"codex response\"}]}}\n\n"+
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"+
		"data: [DONE]\n\n" {
		t.Fatalf("expected raw Codex stream to be preserved, got %q", string(resp.Raw))
	}
	if len(resp.StreamEvents) < 3 {
		t.Fatalf("expected Codex stream events, got %+v", resp.StreamEvents)
	}
	if resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[0].Delta.Text != "ignored " {
		t.Fatalf("expected Codex text delta event, got %+v", resp.StreamEvents[0])
	}
	if resp.StreamEvents[1].Type != contract.ConversationStreamEventUsage || resp.StreamEvents[1].Usage.InputTokens != 4 || resp.StreamEvents[1].Usage.OutputTokens != 5 || resp.StreamEvents[1].Usage.CachedTokens != 1 {
		t.Fatalf("expected Codex usage event, got %+v", resp.StreamEvents[1])
	}
	if resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected Codex terminal stop event, got %+v", resp.StreamEvents)
	}
	if upstreamHeaders.Get("Authorization") != "Bearer codex-token" {
		t.Fatalf("expected runtime to inject codex auth, got %+v", upstreamHeaders)
	}
	if metrics := runtime.Metrics(); metrics.RequestTotal != 1 || metrics.RequestSuccessTotal != 1 {
		t.Fatalf("expected reverse proxy runtime metrics, got %+v", metrics)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesResponsesToolResultImageInputs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		var payload struct {
			Input []struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				CallID  string `json:"call_id"`
				Output  string `json:"output"`
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL string `json:"image_url"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex payload: %v", err)
		}
		if len(payload.Input) != 2 {
			t.Fatalf("expected image message and function output items, got %+v", payload.Input)
		}
		if payload.Input[0].Type != "function_call_output" ||
			payload.Input[0].CallID != "call_1" ||
			payload.Input[0].Output != "File metadata: 800x600 PNG" {
			t.Fatalf("expected tool result text to become function_call_output, got %+v", payload.Input[0])
		}
		if payload.Input[1].Type != "message" ||
			payload.Input[1].Role != "user" ||
			len(payload.Input[1].Content) != 1 ||
			payload.Input[1].Content[0].Type != "input_image" ||
			payload.Input[1].Content[0].ImageURL != "data:image/png;base64,iVBOR" {
			t.Fatalf("expected nested tool result image to become input_image, got %+v", payload.Input[1])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: [DONE]\n\n"))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_codex_tool_result_image",
		Model:          "codex-local",
		SourceProtocol: "anthropic-compatible",
		SourceEndpoint: "/v1/messages",
		Messages: []contract.ConversationMessage{{
			Role: "user",
			Parts: []contract.ContentPart{
				toolResultPart("call_1", "File metadata: 800x600 PNG"),
				{Kind: contract.ContentPartImage, MediaBase64: "iVBOR", MIMEType: "image/png"},
			},
		}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex upstream: %v", err)
	}
	if conversationResponseText(resp) != "ok" {
		t.Fatalf("unexpected codex response: %+v", resp)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesResponsesFunctionCallInputs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		var payload struct {
			Input []struct {
				Type      string `json:"type"`
				Role      string `json:"role"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
				Output    string `json:"output"`
				Content   []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex payload: %v", err)
		}
		if len(payload.Input) != 3 {
			t.Fatalf("expected assistant message, function call, and function output items, got %+v", payload.Input)
		}
		if payload.Input[0].Type != "message" ||
			payload.Input[0].Role != "assistant" ||
			len(payload.Input[0].Content) != 1 ||
			payload.Input[0].Content[0].Type != "output_text" ||
			payload.Input[0].Content[0].Text != "I will check." {
			t.Fatalf("expected assistant message item, got %+v", payload.Input[0])
		}
		if payload.Input[1].Type != "function_call" ||
			payload.Input[1].CallID != "call_1" ||
			payload.Input[1].Name != "lookup_weather" ||
			payload.Input[1].Arguments != `{"city":"Boston"}` {
			t.Fatalf("expected function_call item, got %+v", payload.Input[1])
		}
		if payload.Input[2].Type != "function_call_output" ||
			payload.Input[2].CallID != "call_1" ||
			payload.Input[2].Output != `{"forecast":"sunny"}` {
			t.Fatalf("expected function_call_output item, got %+v", payload.Input[2])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: [DONE]\n\n"))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_codex_function_call_input",
		Model:          "codex-local",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		Messages: []contract.ConversationMessage{
			{Role: "assistant", Parts: []contract.ContentPart{
				{Kind: contract.ContentPartText, Text: "I will check."},
				toolUsePart("call_1", "lookup_weather", `{"city":"Boston"}`),
			}},
			{Role: "tool", Parts: []contract.ContentPart{
				toolResultPart("call_1", `{"forecast":"sunny"}`),
			}},
		},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex upstream: %v", err)
	}
	if conversationResponseText(resp) != "ok" {
		t.Fatalf("unexpected codex response: %+v", resp)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesResponsesContextInputItems(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		var payload struct {
			Input []map[string]any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex payload: %v", err)
		}
		if len(payload.Input) != 3 {
			t.Fatalf("expected reasoning, item_reference, and message items, got %+v", payload.Input)
		}
		if payload.Input[0]["type"] != "reasoning" ||
			payload.Input[0]["id"] != "rs_1" ||
			payload.Input[0]["encrypted_content"] != "gAAA" {
			t.Fatalf("expected raw reasoning item, got %+v", payload.Input[0])
		}
		if payload.Input[1]["type"] != "item_reference" ||
			payload.Input[1]["id"] != "fc_1" {
			t.Fatalf("expected raw item_reference item, got %+v", payload.Input[1])
		}
		content, _ := payload.Input[2]["content"].([]any)
		part, _ := content[0].(map[string]any)
		if payload.Input[2]["type"] != "message" ||
			payload.Input[2]["role"] != "user" ||
			part["type"] != "input_text" ||
			part["text"] != "continue" {
			t.Fatalf("expected user message item, got %+v", payload.Input[2])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: [DONE]\n\n"))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_codex_context_input_items",
		Model:          "codex-local",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		Messages: []contract.ConversationMessage{{
			Role: "assistant",
			Parts: []contract.ContentPart{
				{
					Kind:           contract.ContentPartMetadata,
					Metadata:       map[string]any{"responses_item_type": "reasoning"},
					Raw:            json.RawMessage(`{"type":"reasoning","id":"rs_1","encrypted_content":"gAAA"}`),
					OriginProtocol: "openai-compatible",
				},
				{
					Kind:           contract.ContentPartMetadata,
					Metadata:       map[string]any{"responses_item_type": "item_reference"},
					Raw:            json.RawMessage(`{"type":"item_reference","id":"fc_1"}`),
					OriginProtocol: "openai-compatible",
				},
			},
		}, {
			Role:  "user",
			Parts: textParts("continue"),
		}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex upstream: %v", err)
	}
	if conversationResponseText(resp) != "ok" {
		t.Fatalf("unexpected codex response: %+v", resp)
	}
}

func TestReverseProxyCodexCLIAdapterStreamsMultilineSSEData(t *testing.T) {
	rawSSE := "data: {\"type\":\"response.output_text.delta\",\n" +
		"data: \"delta\":\"codex\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":2}}}}\n\n" +
		"data: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_proxy_multiline_sse",
		Model:      "codex-local",
		InputParts: textParts("hello codex"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "codex" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected codex multiline stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw Codex multiline stream to be preserved, got %q", string(resp.Raw))
	}
	if len(resp.StreamEvents) < 2 || resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta || resp.StreamEvents[0].Delta.Text != "codex" {
		t.Fatalf("expected Codex multiline content delta event, got %+v", resp.StreamEvents)
	}
	if want := "{\"type\":\"response.output_text.delta\",\n\"delta\":\"codex\"}"; string(resp.StreamEvents[0].Raw) != want {
		t.Fatalf("expected first Codex raw event payload %q, got %q", want, string(resp.StreamEvents[0].Raw))
	}
}

func TestReverseProxyCodexCLIAdapterUsesNamedSSEEventType(t *testing.T) {
	rawSSE := "event: response.output_text.delta\n" +
		"data: {\"delta\":\"named\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":2}}}}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_proxy_named_sse",
		Model:      "codex-local",
		InputParts: textParts("hello codex"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "named" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 || resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected codex named stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw Codex named stream to be preserved, got %q", string(resp.Raw))
	}
	if len(resp.StreamEvents) != 3 ||
		resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta ||
		resp.StreamEvents[0].RawEventType != "response.output_text.delta" ||
		string(resp.StreamEvents[0].Raw) != `{"delta":"named"}` ||
		resp.StreamEvents[2].Type != contract.ConversationStreamEventStop ||
		resp.StreamEvents[2].RawEventType != "response.completed" {
		t.Fatalf("expected Codex named stream events, got %+v", resp.StreamEvents)
	}
}

func TestReverseProxyCodexCLIAdapterStreamsIncompleteTerminalUsage(t *testing.T) {
	rawSSE := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
		"data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":2}}}}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(rawSSE))
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_proxy_incomplete_sse",
		Model:      "codex-local",
		InputParts: textParts("hello codex"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": upstream.URL + "/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "partial" ||
		resp.StopReason != contract.StopReasonMaxTokens ||
		resp.Usage.Estimated ||
		resp.Usage.InputTokens != 4 ||
		resp.Usage.OutputTokens != 5 ||
		resp.Usage.CachedTokens != 2 {
		t.Fatalf("unexpected codex incomplete stream response: %+v", resp)
	}
	if string(resp.Raw) != rawSSE {
		t.Fatalf("expected raw Codex incomplete stream to be preserved, got %q", string(resp.Raw))
	}
	if len(resp.StreamEvents) != 3 ||
		resp.StreamEvents[0].Type != contract.ConversationStreamEventContentDelta ||
		resp.StreamEvents[1].Type != contract.ConversationStreamEventUsage ||
		resp.StreamEvents[2].Type != contract.ConversationStreamEventStop ||
		resp.StreamEvents[2].StopReason != contract.StopReasonMaxTokens ||
		resp.StreamEvents[2].RawEventType != "response.incomplete" {
		t.Fatalf("expected Codex incomplete terminal events, got %+v", resp.StreamEvents)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesFunctionCallResponse(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"lookup\",\"arguments\":\"{\\\"query\\\":\\\"weather\\\"}\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_tool_call",
		Model:      "codex-local",
		InputParts: textParts("call lookup"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://codex.example.test/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected codex function call response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "call_1", "lookup", `{"query":"weather"}`)
	if len(resp.StreamEvents) < 3 {
		t.Fatalf("expected Codex function call stream events, got %+v", resp.StreamEvents)
	}
	if resp.StreamEvents[0].Type != contract.ConversationStreamEventToolCallDelta ||
		resp.StreamEvents[0].ContentIndex != 0 ||
		resp.StreamEvents[0].Delta.ToolCallID != "call_1" ||
		resp.StreamEvents[0].Delta.ToolName != "lookup" ||
		resp.StreamEvents[0].Delta.ToolArgumentsJSON != `{"query":"weather"}` {
		t.Fatalf("expected Codex function call stream event, got %+v", resp.StreamEvents[0])
	}
	if resp.StreamEvents[1].Type != contract.ConversationStreamEventUsage || resp.StreamEvents[1].Usage.OutputTokens != 2 {
		t.Fatalf("expected Codex function call usage event, got %+v", resp.StreamEvents[1])
	}
	if resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected Codex function call terminal stop event, got %+v", resp.StreamEvents)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesFunctionCallArgumentDeltas(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"lookup\"}}\n\n" +
					"data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":0,\"delta\":\"{\\\"query\\\":\"}\n\n" +
					"data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"output_index\":0,\"delta\":\"\\\"weather\\\"}\"}\n\n" +
					"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"lookup\",\"arguments\":\"{\\\"query\\\":\\\"weather\\\"}\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_tool_delta",
		Model:      "codex-local",
		InputParts: textParts("call lookup"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://codex.example.test/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if len(resp.Parts) != 1 || resp.StopReason != contract.StopReasonToolUse {
		t.Fatalf("unexpected codex function call delta response: %+v", resp)
	}
	assertToolUsePart(t, resp.Parts[0], "call_1", "lookup", `{"query":"weather"}`)
	toolEvents := conversationStreamEventsByType(resp.StreamEvents, contract.ConversationStreamEventToolCallDelta)
	if len(toolEvents) != 2 {
		t.Fatalf("expected two Codex function argument delta events, got %+v", resp.StreamEvents)
	}
	if toolEvents[0].Delta.ToolCallID != "call_1" || toolEvents[0].Delta.ToolName != "lookup" || toolEvents[0].Delta.ToolArgumentsJSON != `{"query":` {
		t.Fatalf("expected first Codex argument delta, got %+v", toolEvents[0])
	}
	if toolEvents[1].Delta.ToolCallID != "call_1" || toolEvents[1].Delta.ToolName != "lookup" || toolEvents[1].Delta.ToolArgumentsJSON != `"weather"}` {
		t.Fatalf("expected second Codex argument delta, got %+v", toolEvents[1])
	}
	if resp.StreamEvents[len(resp.StreamEvents)-1].Type != contract.ConversationStreamEventStop {
		t.Fatalf("expected Codex terminal stop stream event, got %+v", resp.StreamEvents)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesReasoningTextDeltas(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"type\":\"response.reasoning_text.delta\",\"item_id\":\"rs_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"think \"}\n\n" +
					"data: {\"type\":\"response.reasoning_text.delta\",\"item_id\":\"rs_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"first\"}\n\n" +
					"data: {\"type\":\"response.reasoning_text.done\",\"item_id\":\"rs_1\",\"output_index\":0,\"content_index\":0,\"text\":\"think first\"}\n\n" +
					"data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":1,\"content_index\":0,\"delta\":\"answer\"}\n\n" +
					"data: {\"type\":\"response.output_text.done\",\"item_id\":\"msg_1\",\"output_index\":1,\"content_index\":0,\"text\":\"answer\"}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_reasoning_delta",
		Model:      "codex-local",
		InputParts: textParts("reason then answer"),
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://codex.example.test/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "codex-token"},
	})
	if err != nil {
		t.Fatalf("invoke codex reverse proxy adapter: %v", err)
	}
	if len(resp.Parts) != 2 || resp.Parts[0].Kind != contract.ContentPartThinking || resp.Parts[0].Text != "think first" || resp.Parts[1].Text != "answer" {
		t.Fatalf("expected Codex reasoning and text parts, got %+v", resp.Parts)
	}
	reasoningEvents := conversationStreamEventsByType(resp.StreamEvents, contract.ConversationStreamEventReasoning)
	if len(reasoningEvents) != 2 || reasoningEvents[0].Delta.Text != "think " || reasoningEvents[1].Delta.Text != "first" {
		t.Fatalf("expected Codex reasoning delta events, got %+v", resp.StreamEvents)
	}
	textEvents := conversationStreamEventsByType(resp.StreamEvents, contract.ConversationStreamEventContentDelta)
	if len(textEvents) != 1 || textEvents[0].Delta.Text != "answer" {
		t.Fatalf("expected Codex output text delta event, got %+v", resp.StreamEvents)
	}
	if textEvents[0].ContentIndex != 1 {
		t.Fatalf("expected Codex output text delta to preserve output index, got %+v", textEvents[0])
	}
}

func TestReverseProxyCodexCLIAdapterPassesCliRuntimeContext(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"cli \"}\n\n" +
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"response\"}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_cli_runtime",
		Model:      "codex-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://upstream.example/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("invoke cli reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "cli response" {
		t.Fatalf("unexpected cli response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://upstream.example/backend-api/codex/responses" || !runtime.request.ExpectStream {
		t.Fatalf("unexpected codex runtime request: %+v", runtime.request)
	}
	if runtime.request.Headers.Get("Accept") != "text/event-stream" ||
		runtime.request.Headers.Get("OpenAI-Beta") != "responses=experimental" ||
		runtime.request.Headers.Get("Originator") != "codex_cli_rs" ||
		runtime.request.Headers.Get("User-Agent") != "codex_cli_rs/0.125.0" ||
		runtime.request.Headers.Get("Version") != "0.125.0" ||
		runtime.request.Headers.Get("Session_id") != "srapi-codex-account-9" ||
		runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("unexpected codex runtime headers before auth injection: %+v", runtime.request.Headers)
	}
	var payload struct {
		Model         string `json:"model"`
		Stream        bool   `json:"stream"`
		StreamOptions any    `json:"stream_options"`
		Input         []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode codex runtime payload: %v", err)
	}
	if payload.Model != "codex-upstream" || !payload.Stream || payload.StreamOptions != nil || len(payload.Input) != 1 || payload.Input[0].Role != "user" || payload.Input[0].Content[0].Text != "hello" {
		t.Fatalf("unexpected codex runtime payload: %+v", payload)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCliClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "codex_cli" ||
		runtime.request.Account.Credential["cli_client_token"] != "cli-token" {
		t.Fatalf("expected cli runtime context, got %+v", runtime.request.Account)
	}
}

func TestReverseProxyCodexCLIAdapterPreservesRawResponsesPayload(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body: []byte(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"raw response\"}\n\n" +
					"data: [DONE]\n\n",
			),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	rawPayload := []byte(`{
		"model":"codex-local",
		"input":[
			{"role":"system","content":"raw system"},
			{"role":"user","content":"raw user"}
		],
		"stream":false,
		"store":true,
		"service_tier":"fast",
		"reasoning":{"effort":"high"},
		"text":{"format":{"type":"text"},"verbosity":"low"},
		"previous_response_id":"resp_prev",
		"parallel_tool_calls":true,
		"prompt_cache_key":"cache-123",
		"temperature":0.2,
		"max_output_tokens":64,
		"metadata":{"tenant":"downstream"},
		"stream_options":{"include_usage":true},
		"user":"downstream-user"
	}`)
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:      "req_raw_codex",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		Model:          "codex-local",
		RawBody:        rawPayload,
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             16,
			RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://upstream.example/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"access_token": "oauth-token"},
	})
	if err != nil {
		t.Fatalf("invoke raw codex reverse proxy adapter: %v", err)
	}
	if conversationResponseText(resp) != "raw response" {
		t.Fatalf("unexpected raw codex response: %+v", resp)
	}
	if runtime.request.Headers.Get("OpenAI-Beta") != "responses=experimental" ||
		runtime.request.Headers.Get("User-Agent") != "codex_cli_rs/0.125.0" ||
		runtime.request.Headers.Get("Session_id") != "srapi-codex-account-16" {
		t.Fatalf("unexpected raw codex headers: %+v", runtime.request.Headers)
	}
	var payload map[string]any
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode raw codex payload: %v", err)
	}
	if payload["model"] != "codex-upstream" || payload["stream"] != true || payload["store"] != false || payload["service_tier"] != "priority" {
		t.Fatalf("unexpected raw codex payload defaults: %+v", payload)
	}
	if payload["instructions"] != "raw system" || payload["previous_response_id"] != "resp_prev" || payload["parallel_tool_calls"] != true || payload["prompt_cache_key"] != "cache-123" {
		t.Fatalf("expected raw codex metadata to be preserved, got %+v", payload)
	}
	for _, removed := range []string{"temperature", "max_output_tokens", "metadata", "stream_options", "user"} {
		if _, ok := payload[removed]; ok {
			t.Fatalf("expected unsupported field %q to be removed from %+v", removed, payload)
		}
	}
	reasoning, _ := payload["reasoning"].(map[string]any)
	text, _ := payload["text"].(map[string]any)
	if reasoning["effort"] != "high" || text["verbosity"] != "low" {
		t.Fatalf("expected reasoning/text to be preserved, got reasoning=%+v text=%+v", reasoning, text)
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected one normalized user input, got %+v", payload["input"])
	}
	item, _ := input[0].(map[string]any)
	content, _ := item["content"].([]any)
	part, _ := content[0].(map[string]any)
	if item["role"] != "user" || item["type"] != "message" || part["type"] != "input_text" || part["text"] != "raw user" {
		t.Fatalf("unexpected normalized raw input: %+v", item)
	}
}

func TestReverseProxyCodexCLIPrepareRealtimeBuildsResponsesWebSocketSession(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	session, err := svc.PrepareRealtime(context.Background(), contract.RealtimeRequest{
		RequestID:      "req_codex_ws",
		Model:          "codex-local",
		RequestPayload: []byte(`{"model":"codex-local","input":"hello codex ws","stream":true}`),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassCliClientToken,
			UpstreamClient: ptrString("codex_cli"),
			Metadata: map[string]any{
				"base_url":                                     "https://chatgpt.example/backend-api/codex",
				"user_agent":                                   "codex-cli/0.118.0 (Mac OS)",
				"chatgpt_account_id":                           "chatgpt-account",
				"codex_session_id":                             "session-123",
				"codex_beta_features":                          "feature-a",
				"codex_version":                                "0.118.0",
				"codex_turn_metadata":                          `{"cwd":"/repo"}`,
				"codex_client_request_id":                      "client-req-123",
				"x_responsesapi_include_timing_metrics":        "true",
				"openai_oauth_responses_websockets_v2_enabled": true,
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"cli_client_token": "cli-token"},
	})
	if err != nil {
		t.Fatalf("prepare codex realtime: %v", err)
	}
	if session.URL != "wss://chatgpt.example/backend-api/codex/responses" {
		t.Fatalf("unexpected codex websocket URL %q", session.URL)
	}
	if headerValue(session.Headers, "OpenAI-Beta") != "responses_websockets=2026-02-06" ||
		headerValue(session.Headers, "Originator") != "codex_cli_rs" ||
		headerValue(session.Headers, "ChatGPT-Account-ID") != "chatgpt-account" ||
		headerValue(session.Headers, "X-Codex-Beta-Features") != "feature-a" ||
		headerValue(session.Headers, "Version") != "0.118.0" ||
		headerValue(session.Headers, "X-Codex-Turn-Metadata") != `{"cwd":"/repo"}` ||
		headerValue(session.Headers, "X-Client-Request-Id") != "client-req-123" ||
		headerValue(session.Headers, "X-ResponsesAPI-Include-Timing-Metrics") != "true" {
		t.Fatalf("unexpected codex websocket headers: %+v", session.Headers)
	}
	if headerValue(session.Headers, "session_id") != "session-123" {
		t.Fatalf("unexpected codex websocket session_id header: %+v", session.Headers)
	}
	if session.Headers.Get("Authorization") != "" {
		t.Fatalf("adapter should leave auth injection to reverse proxy runtime, got %+v", session.Headers)
	}
	var frame map[string]any
	if err := json.Unmarshal(session.InitialFrame, &frame); err != nil {
		t.Fatalf("decode initial frame: %v", err)
	}
	if frame["type"] != "response.create" || frame["model"] != "codex-upstream" || frame["input"] != "hello codex ws" || frame["stream"] != true {
		t.Fatalf("unexpected codex websocket initial frame: %+v", frame)
	}
}

func TestReverseProxyCodexCLIRejectsAPIKeyRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"output_text":"should not be called"}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_codex_api_key_runtime",
		Model:      "codex-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassAPIKey,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://upstream.example/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"api_key": "sk-secret"},
	})
	if err == nil {
		t.Fatalf("expected api_key runtime rejection")
	}
	providerErr, ok := err.(contract.ProviderError)
	if !ok {
		t.Fatalf("expected provider error, got %T %v", err, err)
	}
	if providerErr.Class != "invalid_request" || providerErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected provider error: %+v", providerErr)
	}
	if runtime.request.URL != "" {
		t.Fatalf("reverse proxy runtime should not be called, got %+v", runtime.request)
	}
}

func TestReverseProxyCodexCLIPrepareRealtimeRejectsAPIKeyRuntime(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.PrepareRealtime(context.Background(), contract.RealtimeRequest{
		RequestID:      "req_codex_ws_api_key_runtime",
		Model:          "codex-local",
		RequestPayload: []byte(`{"model":"codex-local","input":"hello"}`),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-codex-cli",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             9,
			RuntimeClass:   accountcontract.RuntimeClassAPIKey,
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"base_url": "https://chatgpt.example/backend-api/codex"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
		Credential: map[string]any{"api_key": "sk-secret"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
}

func TestOpenAICompatiblePrepareRealtimeBuildsRealtimeWebSocketSession(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	headers := http.Header{}
	headers.Set("OpenAI-Safety-Identifier", "safe-user-hash")
	headers.Set("Authorization", "Bearer leaked")
	session, err := svc.PrepareRealtime(context.Background(), contract.RealtimeRequest{
		RequestID: "req_openai_realtime_ws",
		Model:     "local-realtime",
		Headers:   headers,
		Provider: providercontract.Provider{
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           10,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url": "https://api.openai.example/v1",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-realtime-2"},
		Credential: map[string]any{"access_token": "oauth-token"},
	})
	if err != nil {
		t.Fatalf("prepare openai realtime: %v", err)
	}
	if session.URL != "wss://api.openai.example/v1/realtime?model=gpt-realtime-2" {
		t.Fatalf("unexpected realtime websocket URL %q", session.URL)
	}
	if session.Headers.Get("OpenAI-Safety-Identifier") != "safe-user-hash" {
		t.Fatalf("expected safety identifier header, got %+v", session.Headers)
	}
	if session.Headers.Get("Authorization") != "" || session.InitialFrame != nil {
		t.Fatalf("expected adapter to leave auth and initial frame empty, got headers=%+v frame=%s", session.Headers, session.InitialFrame)
	}
}

func TestOpenAICompatiblePrepareRealtimeAllowsAPIKeyRuntime(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	session, err := svc.PrepareRealtime(context.Background(), contract.RealtimeRequest{
		RequestID: "req_openai_realtime_api_key",
		Model:     "local-realtime",
		Provider: providercontract.Provider{
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           10,
			RuntimeClass: accountcontract.RuntimeClassAPIKey,
			Metadata:     map[string]any{"base_url": "https://api.openai.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-realtime-2"},
		Credential: map[string]any{"api_key": "sk-secret"},
	})
	if err != nil {
		t.Fatalf("prepare api-key openai realtime: %v", err)
	}
	if session.URL != "wss://api.openai.example/v1/realtime?model=gpt-realtime-2" {
		t.Fatalf("unexpected realtime websocket URL %q", session.URL)
	}
	if session.Headers.Get("Authorization") != "" || session.InitialFrame != nil {
		t.Fatalf("expected adapter to leave official api-key auth to gateway relay, got headers=%+v frame=%s", session.Headers, session.InitialFrame)
	}
}

func TestReverseProxyOpenAICompatiblePrepareRealtimeRejectsAPIKeyRuntime(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.PrepareRealtime(context.Background(), contract.RealtimeRequest{
		RequestID: "req_openai_realtime_reverse_proxy_api_key",
		Model:     "local-realtime",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-openai-compatible",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           10,
			RuntimeClass: accountcontract.RuntimeClassAPIKey,
			Metadata:     map[string]any{"base_url": "https://api.openai.example/v1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-realtime-2"},
		Credential: map[string]any{"api_key": "sk-secret"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
}

func TestReverseProxyAntigravityOpenAIAdapterDispatchesThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity openai response"}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}},"traceId":"trace-1"}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_openai",
		Model:      "antigravity-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             15,
			RuntimeClass:   accountcontract.RuntimeClassDesktopClientToken,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "antigravity-openai-upstream"},
		Credential: map[string]any{"access_token": "desktop-token"},
	})
	if err != nil {
		t.Fatalf("invoke antigravity openai adapter: %v", err)
	}
	if conversationResponseText(resp) != "antigravity openai response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected antigravity openai response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://antigravity.example/v1internal:generateContent" {
		t.Fatalf("unexpected antigravity openai request: %+v", runtime.request)
	}
	if runtime.request.Headers.Get("Content-Type") != "application/json" || runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("adapter should leave antigravity auth injection to runtime, got %+v", runtime.request.Headers)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassDesktopClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "antigravity_desktop" ||
		runtime.request.Account.Credential["access_token"] != "desktop-token" {
		t.Fatalf("expected antigravity desktop runtime context, got %+v", runtime.request.Account)
	}
	var payload struct {
		Project     string `json:"project"`
		RequestID   string `json:"requestId"`
		UserAgent   string `json:"userAgent"`
		RequestType string `json:"requestType"`
		Model       string `json:"model"`
		Request     struct {
			SessionID        string `json:"sessionId"`
			GenerationConfig struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			SafetySettings []struct {
				Threshold string `json:"threshold"`
			} `json:"safetySettings"`
		} `json:"request"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode antigravity openai payload: %v", err)
	}
	if payload.Project != "project-1" ||
		!strings.HasPrefix(payload.RequestID, "agent-") ||
		payload.UserAgent != "antigravity" ||
		payload.RequestType != "agent" ||
		payload.Model != "antigravity-openai-upstream" ||
		payload.Request.SessionID == "" ||
		payload.Request.GenerationConfig.MaxOutputTokens != 0 ||
		len(payload.Request.Contents) != 1 ||
		payload.Request.Contents[0].Role != "user" ||
		len(payload.Request.Contents[0].Parts) != 1 ||
		payload.Request.Contents[0].Parts[0].Text != "hello" ||
		len(payload.Request.SafetySettings) == 0 ||
		payload.Request.SafetySettings[0].Threshold != "OFF" {
		t.Fatalf("unexpected antigravity openai payload: %+v", payload)
	}
}

func TestReverseProxyAntigravityAnthropicAdapterDispatchesThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity anthropic response"}]}}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":4}}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_anthropic",
		Model:      "antigravity-claude-local",
		InputParts: textParts("hello anthropic"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "anthropic-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             17,
			RuntimeClass:   accountcontract.RuntimeClassDesktopClientToken,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "claude-upstream"},
		Credential: map[string]any{"access_token": "desktop-token"},
	})
	if err != nil {
		t.Fatalf("invoke antigravity anthropic adapter: %v", err)
	}
	if conversationResponseText(resp) != "antigravity anthropic response" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected antigravity anthropic response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://antigravity.example/v1internal:generateContent" {
		t.Fatalf("unexpected antigravity anthropic request: %+v", runtime.request)
	}
	if runtime.request.Headers.Get("anthropic-version") != "" || runtime.request.Headers.Get("x-api-key") != "" || runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("unexpected antigravity anthropic headers: %+v", runtime.request.Headers)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassDesktopClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "antigravity_desktop" ||
		runtime.request.Account.Credential["access_token"] != "desktop-token" {
		t.Fatalf("expected antigravity desktop runtime context, got %+v", runtime.request.Account)
	}
	var payload struct {
		Model   string `json:"model"`
		Request struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			ToolConfig *struct {
				FunctionCallingConfig map[string]string `json:"functionCallingConfig"`
			} `json:"toolConfig"`
		} `json:"request"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode antigravity anthropic payload: %v", err)
	}
	if payload.Model != "claude-upstream" ||
		len(payload.Request.Contents) != 1 ||
		len(payload.Request.Contents[0].Parts) != 1 ||
		payload.Request.Contents[0].Parts[0].Text != "hello anthropic" ||
		payload.Request.ToolConfig == nil ||
		payload.Request.ToolConfig.FunctionCallingConfig["mode"] != "VALIDATED" {
		t.Fatalf("unexpected antigravity anthropic payload: %+v", payload)
	}
}

func TestReverseProxyAntigravityGeminiAdapterDispatchesThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity gemini response"}]}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":5}}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_gemini",
		Model:      "antigravity-gemini-local",
		InputParts: textParts("hello gemini"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             16,
			RuntimeClass:   accountcontract.RuntimeClassDesktopClientToken,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"access_token": "desktop-token"},
	})
	if err != nil {
		t.Fatalf("invoke antigravity gemini adapter: %v", err)
	}
	if conversationResponseText(resp) != "antigravity gemini response" || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected antigravity gemini response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://antigravity.example/v1internal:generateContent" {
		t.Fatalf("unexpected antigravity gemini request: %+v", runtime.request)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassDesktopClientToken) ||
		runtime.request.Account.UpstreamClient == nil ||
		*runtime.request.Account.UpstreamClient != "antigravity_desktop" ||
		runtime.request.Account.Credential["access_token"] != "desktop-token" {
		t.Fatalf("expected antigravity desktop runtime context, got %+v", runtime.request.Account)
	}
	var payload struct {
		Model   string `json:"model"`
		Request struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		} `json:"request"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode antigravity gemini payload: %v", err)
	}
	if payload.Model != "gemini-pro" || len(payload.Request.Contents) != 1 || len(payload.Request.Contents[0].Parts) != 1 || payload.Request.Contents[0].Parts[0].Text != "hello gemini" {
		t.Fatalf("unexpected antigravity gemini payload: %+v", payload)
	}
}

func TestReverseProxyAntigravityAdapterStreamsMultilineSSEData(t *testing.T) {
	rawSSE := "data: {\"response\":{\"candidates\":[{\"content\":\n" +
		"data: {\"parts\":[{\"text\":\"antigravity\"}]}}],\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":5}}}\n\n"
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(rawSSE),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_multiline_sse",
		Model:      "antigravity-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             16,
			RuntimeClass:   accountcontract.RuntimeClassDesktopClientToken,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"access_token": "desktop-token"},
	})
	if err != nil {
		t.Fatalf("invoke antigravity stream adapter: %v", err)
	}
	if conversationResponseText(resp) != "antigravity" || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected antigravity multiline stream response: %+v", resp)
	}
	if runtime.request.URL != "https://antigravity.example/v1internal:streamGenerateContent?alt=sse" || !runtime.request.ExpectStream {
		t.Fatalf("expected antigravity stream runtime request, got %+v", runtime.request)
	}
	if runtime.request.Headers.Get("Accept") != "text/event-stream" || runtime.request.Headers.Get("Accept-Encoding") != "identity" {
		t.Fatalf("expected antigravity stream headers, got %+v", runtime.request.Headers)
	}
}

func TestReverseProxyAntigravityAdapterClassifiesStreamErrorFrame(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte("event: error\ndata: {\"error\":{\"status\":\"UNAVAILABLE\",\"message\":\"antigravity unavailable\",\"code\":503}}\n\n"),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_stream_error",
		Model:      "antigravity-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "gemini-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             16,
			RuntimeClass:   accountcontract.RuntimeClassDesktopClientToken,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gemini-pro"},
		Credential: map[string]any{"access_token": "desktop-token"},
	})
	providerErr := assertProviderError(t, err, "provider_5xx", http.StatusServiceUnavailable)
	if providerErr.Message != "antigravity unavailable" {
		t.Fatalf("expected Antigravity stream error message to be preserved, got %+v", providerErr)
	}
}

func TestReverseProxyAntigravityCleansToolSchemas(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"schema response"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_schema",
		Model:      "antigravity-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             15,
			RuntimeClass:   accountcontract.RuntimeClassDesktopClientToken,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "antigravity-openai-upstream"},
		Credential: map[string]any{"access_token": "desktop-token"},
		Tools: []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "lookup",
					"description": "lookup data",
					"parameters": map[string]any{
						"$schema":    "https://json-schema.org/draft/2020-12/schema",
						"type":       "object",
						"nullable":   true,
						"enumTitles": []any{"unused"},
						"properties": map[string]any{
							"query": map[string]any{
								"type":       "string",
								"nullable":   true,
								"deprecated": true,
								"prefill":    "x",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("invoke antigravity schema adapter: %v", err)
	}
	var payload struct {
		Request struct {
			Tools []struct {
				FunctionDeclarations []struct {
					Parameters map[string]any `json:"parameters"`
				} `json:"functionDeclarations"`
			} `json:"tools"`
		} `json:"request"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode antigravity schema payload: %v", err)
	}
	if len(payload.Request.Tools) != 1 || len(payload.Request.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("unexpected antigravity tools payload: %+v", payload)
	}
	params := payload.Request.Tools[0].FunctionDeclarations[0].Parameters
	if _, ok := params["$schema"]; ok {
		t.Fatalf("schema key should be removed: %+v", params)
	}
	if _, ok := params["enumTitles"]; ok {
		t.Fatalf("enumTitles should be removed: %+v", params)
	}
	if got, ok := params["type"].([]any); !ok || len(got) != 2 || got[0] != "object" || got[1] != "null" {
		t.Fatalf("nullable object type should be normalized, got %+v", params["type"])
	}
	props := params["properties"].(map[string]any)
	query := props["query"].(map[string]any)
	if _, ok := query["deprecated"]; ok {
		t.Fatalf("nested deprecated should be removed: %+v", query)
	}
	if _, ok := query["prefill"]; ok {
		t.Fatalf("nested prefill should be removed: %+v", query)
	}
	if got, ok := query["type"].([]any); !ok || len(got) != 2 || got[0] != "string" || got[1] != "null" {
		t.Fatalf("nullable string type should be normalized, got %+v", query["type"])
	}
}

func TestReverseProxyAntigravityRejectsAPIKeyRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"should not call"}]}}]}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_antigravity_api_key_runtime",
		Model:      "antigravity-local",
		InputParts: textParts("hello"),
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-antigravity",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:             15,
			RuntimeClass:   accountcontract.RuntimeClassAPIKey,
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"base_url": "https://antigravity.example", "project_id": "project-1"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "antigravity-openai-upstream"},
		Credential: map[string]any{"api_key": "sk-secret"},
	})
	assertProviderError(t, err, "invalid_request", http.StatusBadRequest)
	if runtime.request.URL != "" {
		t.Fatalf("reverse proxy runtime should not be called for api_key runtime, got %+v", runtime.request)
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
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_reverse_stream",
		Model:      "rp-local",
		InputParts: textParts("hello"),
		Stream:     true,
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-openai-compatible",
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
	if conversationResponseText(resp) != "runtime stream" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected reverse proxy stream response: %+v", resp)
	}
}

func TestGenericReverseProxyAdapterDispatchesCustomRuntimeThroughRuntime(t *testing.T) {
	runtime := capturingRuntime{
		response: reverseproxycontract.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":{"message":"runtime generic response"},"usage":{"prompt_tokens":2,"completion_tokens":3}}`),
		},
	}
	svc, err := service.NewWithReverseProxy(nil, &runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_generic_runtime",
		Model:      "generic-local",
		InputParts: textParts("hello runtime"),
		Provider: providercontract.Provider{
			AdapterType: "generic-reverse-proxy",
			Protocol:    "openai-compatible",
			ConfigSchema: map[string]any{
				"base_url":             "https://generic.example/api",
				"chat_path":            "/chat",
				"auth_header_template": "X-Generic-Token: {{access_token}}",
				"response_path_rules":  map[string]any{"text_path": "result.message"},
			},
		},
		Account: accountcontract.ProviderAccount{
			ID:           9,
			RuntimeClass: accountcontract.RuntimeClassCustomReverseProxy,
			ProxyID:      ptrString("http://proxy.example:8080"),
			Metadata:     map[string]any{"user_agent": "GenericClient/1.0"},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "runtime-upstream"},
		Credential: map[string]any{"access_token": "runtime-token"},
	})
	if err != nil {
		t.Fatalf("invoke generic runtime adapter: %v", err)
	}
	if conversationResponseText(resp) != "runtime generic response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("unexpected generic runtime response: %+v", resp)
	}
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://generic.example/api/chat" {
		t.Fatalf("unexpected generic runtime request: %+v", runtime.request)
	}
	if runtime.request.Headers.Get("X-Generic-Token") != "runtime-token" || runtime.request.Headers.Get("Authorization") != "" {
		t.Fatalf("unexpected generic runtime headers: %+v", runtime.request.Headers)
	}
	if runtime.request.Account.RuntimeClass != string(accountcontract.RuntimeClassCustomReverseProxy) ||
		runtime.request.Account.ProxyID == nil ||
		*runtime.request.Account.ProxyID != "http://proxy.example:8080" ||
		runtime.request.Account.UserAgent != "GenericClient/1.0" ||
		runtime.request.Account.Credential["access_token"] != "runtime-token" {
		t.Fatalf("expected custom reverse proxy runtime context, got %+v", runtime.request.Account)
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(runtime.request.Body, &payload); err != nil {
		t.Fatalf("decode generic runtime payload: %v", err)
	}
	if payload.Model != "runtime-upstream" {
		t.Fatalf("unexpected generic runtime payload: %+v", payload)
	}
}

func TestReverseProxyAdapterMapsRuntimeErrors(t *testing.T) {
	runtime := failingRuntime{}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_reverse_error",
		Model:     "rp-local",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-openai-compatible",
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_reverse_legacy",
		Model:     "rp-local",
		Provider: providercontract.Provider{
			AdapterType: "reverse-proxy-openai-compatible",
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

type sequenceRuntime struct {
	requests  []reverseproxycontract.Request
	responses []reverseproxycontract.Response
	errs      []error
}

func (r *sequenceRuntime) Do(_ context.Context, req reverseproxycontract.Request) (reverseproxycontract.Response, error) {
	r.requests = append(r.requests, req)
	idx := len(r.requests) - 1
	if idx < len(r.errs) && r.errs[idx] != nil {
		return reverseproxycontract.Response{}, r.errs[idx]
	}
	if idx < len(r.responses) {
		return r.responses[idx], nil
	}
	return reverseproxycontract.Response{}, nil
}

func assertProviderError(t *testing.T, err error, class string, statusCode int) contract.ProviderError {
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
	return providerErr
}

func headerValue(headers http.Header, key string) string {
	for existingKey, values := range headers {
		if !strings.EqualFold(existingKey, key) {
			continue
		}
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	if value := strings.TrimSpace(headers.Get(key)); value != "" {
		return value
	}
	return ""
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
