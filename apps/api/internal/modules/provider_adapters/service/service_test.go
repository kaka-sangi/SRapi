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
	if resp.Text != "reverse anthropic response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_claude_api_key",
		Model:     "claude-local",
		Prompt:    "hello",
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_proxy",
		Model:     "rp-local",
		Prompt:    "hello",
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_chatgpt_web_proxy",
		Model:     "chatgpt-local",
		Prompt:    "hello chatgpt web",
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
	if resp.Text != "chatgpt web response" || !resp.Usage.Estimated {
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_chatgpt_web_auto_requirements",
		Model:     "chatgpt-local",
		Prompt:    "hello auto requirements",
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
	if resp.Text != "auto requirements ok" {
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_chatgpt_web_manual_requirements",
		Model:     "chatgpt-local",
		Prompt:    "hello",
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_chatgpt_web_api_key_runtime",
		Model:     "chatgpt-local",
		Prompt:    "hello",
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
				"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":5,\"cached_tokens\":1}}}\n\n" +
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID:    "req_codex_proxy",
		Model:        "codex-local",
		Instructions: "be concise",
		Messages: []contract.TextMessage{
			{Role: "system", Content: "system guardrail"},
			{Role: "user", Content: "hello codex"},
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
	if resp.Text != "codex response" || resp.Usage.Estimated || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 || resp.Usage.CachedTokens != 1 {
		t.Fatalf("unexpected codex response: %+v", resp)
	}
	if upstreamHeaders.Get("Authorization") != "Bearer codex-token" {
		t.Fatalf("expected runtime to inject codex auth, got %+v", upstreamHeaders)
	}
	if metrics := runtime.Metrics(); metrics.RequestTotal != 1 || metrics.RequestSuccessTotal != 1 {
		t.Fatalf("expected reverse proxy runtime metrics, got %+v", metrics)
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
			Metadata:       map[string]any{"base_url": "https://upstream.example/backend-api/codex"},
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
	if runtime.request.Method != http.MethodPost || runtime.request.URL != "https://upstream.example/backend-api/codex/responses" || !runtime.request.ExpectStream {
		t.Fatalf("unexpected codex runtime request: %+v", runtime.request)
	}
	if runtime.request.Headers.Get("Accept") != "text/event-stream" ||
		runtime.request.Headers.Get("Originator") != "codex_cli_rs" ||
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_codex_api_key_runtime",
		Model:     "codex-local",
		Prompt:    "hello",
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

func TestOpenAICompatiblePrepareRealtimeRejectsAPIKeyRuntime(t *testing.T) {
	svc, err := service.New(nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.PrepareRealtime(context.Background(), contract.RealtimeRequest{
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_antigravity_openai",
		Model:     "antigravity-local",
		Prompt:    "hello",
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
	if resp.Text != "antigravity openai response" || resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_antigravity_anthropic",
		Model:     "antigravity-claude-local",
		Prompt:    "hello anthropic",
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
	if resp.Text != "antigravity anthropic response" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 4 {
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_antigravity_gemini",
		Model:     "antigravity-gemini-local",
		Prompt:    "hello gemini",
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
	if resp.Text != "antigravity gemini response" || resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 5 {
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_antigravity_schema",
		Model:     "antigravity-local",
		Prompt:    "hello",
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_antigravity_api_key_runtime",
		Model:     "antigravity-local",
		Prompt:    "hello",
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
	resp, err := svc.InvokeText(context.Background(), contract.TextRequest{
		RequestID: "req_reverse_stream",
		Model:     "rp-local",
		Prompt:    "hello",
		Stream:    true,
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
	_, err = svc.InvokeText(context.Background(), contract.TextRequest{
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
