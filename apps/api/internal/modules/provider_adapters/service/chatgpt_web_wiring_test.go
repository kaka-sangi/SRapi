package service_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
	"github.com/srapi/srapi/apps/api/internal/pkg/httputil"
)

// stubClearanceProvider records Resolve calls and returns a canned bundle.
type stubClearanceProvider struct {
	mu     sync.Mutex
	calls  []httputil.ResolveRequest
	bundle *httputil.ClearanceBundle
	err    error
}

func (s *stubClearanceProvider) Resolve(r httputil.ResolveRequest) (*httputil.ClearanceBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r)
	if s.err != nil {
		return nil, s.err
	}
	return s.bundle, nil
}

func (s *stubClearanceProvider) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// TestChatGPTWebWiringCFClearanceRetriesAndInjectsCookies proves that when
// the upstream returns a CF challenge on the first attempt, the wiring
// resolves a clearance bundle, retries, and the retry carries the
// cf_clearance cookie.
func TestChatGPTWebWiringCFClearanceRetriesAndInjectsCookies(t *testing.T) {
	var hits int32
	var retryHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			// CF challenge response.
			w.Header().Set("cf-mitigated", "challenge")
			w.Header().Set("cf-ray", "abc-1")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><body>Just a moment...</body></html>"))
			return
		}
		retryHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"hello after clearance\"]}}}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	stub := &stubClearanceProvider{
		bundle: &httputil.ClearanceBundle{
			Cookies:   map[string]string{"cf_clearance": "PORTED-OK"},
			UserAgent: "Mozilla/5.0 (ClearedTest)",
		},
	}
	service.SetChatGPTWebClearanceProvider(stub)
	defer service.SetChatGPTWebClearanceProvider(nil)

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create reverse proxy runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_cf_wired",
		Model:      "chatgpt-cf",
		InputParts: []contract.ContentPart{{Kind: contract.ContentPartText, Text: "hi"}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           21,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-1",
				"user_agent":                 "Mozilla/5.0 OriginalAgent",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
		Credential: map[string]any{"access_token": "tok-cf"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web with CF retry: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 upstream hits (1 challenge + 1 retry), got %d", got)
	}
	if stub.Calls() != 1 {
		t.Fatalf("expected 1 Resolve call to clearance provider, got %d", stub.Calls())
	}
	if cookie := retryHeaders.Get("Cookie"); !strings.Contains(cookie, "cf_clearance=PORTED-OK") {
		t.Fatalf("retry did not inject cf_clearance cookie; Cookie=%q", cookie)
	}
}

// TestChatGPTWebWiringCFClearanceUnconfiguredProducesClearError proves that
// when the clearance provider is unconfigured and an upstream challenge is
// detected, the error surfaced to callers names the env var fix.
func TestChatGPTWebWiringCFClearanceUnconfiguredProducesClearError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("cf-mitigated", "challenge")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html><body>Just a moment...</body></html>"))
	}))
	defer upstream.Close()

	// Empty provider mirrors FlareSolverr without FLARESOLVERR_URL set.
	service.SetChatGPTWebClearanceProvider(httputil.NewFlareSolverrProvider(httputil.FlareSolverrConfig{}))
	defer service.SetChatGPTWebClearanceProvider(nil)

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create reverse proxy runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_cf_uncfg",
		Model:      "chatgpt-cf",
		InputParts: []contract.ContentPart{{Kind: contract.ContentPartText, Text: "hi"}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           22,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-1",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
		Credential: map[string]any{"access_token": "tok-cf"},
	})
	if err == nil {
		t.Fatal("expected error when provider unconfigured")
	}
	var perr contract.ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("expected ProviderError, got %T %v", err, err)
	}
	if perr.Class != "challenge_required" {
		t.Errorf("expected class challenge_required, got %q", perr.Class)
	}
	if !strings.Contains(perr.Message, "FLARESOLVERR_URL") {
		t.Errorf("error should name FLARESOLVERR_URL, got %q", perr.Message)
	}
}

// TestChatGPTWebWiringFileUploadProducesAssetPointer proves the multimodal
// payload builder uploaded a binary InputPart and put a file-service:// URI
// into the outbound conversation payload.
func TestChatGPTWebWiringFileUploadProducesAssetPointer(t *testing.T) {
	// Build a 4x3 PNG so the dimension extraction has something real.
	pngBytes := buildTestPNG(t, 4, 3)
	pngB64 := base64.StdEncoding.EncodeToString(pngBytes)

	var registerHit, putHit, finaliseHit int32
	var conversationBody []byte
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/backend-api/files" && r.Method == http.MethodPost:
			atomic.AddInt32(&registerHit, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_id":"file_abc","upload_url":"` + uploadBlobURL(r.Host, "/blob") + `"}`))
		case r.URL.Path == "/blob" && r.Method == http.MethodPut:
			atomic.AddInt32(&putHit, 1)
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/backend-api/files/file_abc/uploaded" && r.Method == http.MethodPost:
			atomic.AddInt32(&finaliseHit, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/backend-api/conversation" && r.Method == http.MethodPost:
			conversationBody, _ = readBody(r)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"ok\"]}}}\n\ndata: [DONE]\n\n"))
		default:
			t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer uploadServer.Close()

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create reverse proxy runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_upload_wired",
		Model:     "chatgpt-upload",
		InputParts: []contract.ContentPart{
			{Kind: contract.ContentPartText, Text: "describe this image"},
			{Kind: contract.ContentPartImage, MediaBase64: pngB64, MIMEType: "image/png"},
		},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           33,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   uploadServer.URL,
				"chatgpt_requirements_token": "tok-up",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
		Credential: map[string]any{"access_token": "tok-up"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web with upload: %v", err)
	}
	if atomic.LoadInt32(&registerHit) == 0 || atomic.LoadInt32(&putHit) == 0 || atomic.LoadInt32(&finaliseHit) == 0 {
		t.Fatalf("expected register/put/finalise to all be called; got register=%d put=%d finalise=%d",
			registerHit, putHit, finaliseHit)
	}
	if !bytes.Contains(conversationBody, []byte("file-service://file_abc")) {
		t.Fatalf("expected outbound conversation body to reference asset_pointer; got: %s", string(conversationBody))
	}
	if !bytes.Contains(conversationBody, []byte("multimodal_text")) {
		t.Fatalf("expected multimodal_text content; got: %s", string(conversationBody))
	}
}

func TestChatGPTWebWiringFileUploadFailureStopsConversation(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString(buildTestPNG(t, 2, 2))
	var registerHit, conversationHit int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/backend-api/files" && r.Method == http.MethodPost:
			atomic.AddInt32(&registerHit, 1)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"upload unavailable"}`))
		case r.URL.Path == "/backend-api/conversation" && r.Method == http.MethodPost:
			atomic.AddInt32(&conversationHit, 1)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"should-not-send\"]}}}\n\ndata: [DONE]\n\n"))
		default:
			t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID: "req_upload_fail",
		Model:     "chatgpt-upload",
		InputParts: []contract.ContentPart{
			{Kind: contract.ContentPartText, Text: "describe this image"},
			{Kind: contract.ContentPartImage, MediaBase64: pngB64, MIMEType: "image/png"},
		},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           34,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-up",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
		Credential: map[string]any{"access_token": "tok-up"},
	})
	if err == nil {
		t.Fatal("expected upload failure to abort the ChatGPT web request")
	}
	if atomic.LoadInt32(&registerHit) != 1 {
		t.Fatalf("expected one upload registration attempt, got %d", registerHit)
	}
	if got := atomic.LoadInt32(&conversationHit); got != 0 {
		t.Fatalf("conversation request should not be sent after upload failure, got %d hits", got)
	}
}

func TestChatGPTWebWiringSendsChatGPTAccountHeaders(t *testing.T) {
	var capturedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"ok\"]}}}\n\ndata: [DONE]\n\n"))
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
	_, err = svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_header_wired",
		Model:      "chatgpt-header",
		InputParts: []contract.ContentPart{{Kind: contract.ContentPartText, Text: "hello"}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           35,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-head",
				"chatgpt_account_id":         "metadata-account",
				"originator":                 "metadata-originator",
				"version":                    "0.124.0",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
		Credential: map[string]any{"access_token": "tok-head", "chatgpt_account_id": "credential-account"},
		RequestSettings: map[string]any{
			"chatgpt_account_id": "chatgpt-account-123",
			"originator":         "codex_cli_rs",
			"version":            "0.125.0",
		},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web: %v", err)
	}
	if got := capturedHeaders.Get("chatgpt-account-id"); got != "chatgpt-account-123" {
		t.Fatalf("chatgpt-account-id = %q, want %q", got, "chatgpt-account-123")
	}
	if got := capturedHeaders.Get("originator"); got != "codex_cli_rs" {
		t.Fatalf("originator = %q, want %q", got, "codex_cli_rs")
	}
	if got := capturedHeaders.Get("version"); got != "0.125.0" {
		t.Fatalf("version = %q, want %q", got, "0.125.0")
	}
}

// TestChatGPTWebWiringImageSlotLimiterReleasedOnReturn proves that the per-
// account image-slot is acquired AND released across an invocation (so the
// next request gets the slot).
func TestChatGPTWebWiringImageSlotLimiterReleasedOnReturn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"ok\"]}}}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	build := func() contract.ConversationRequest {
		return contract.ConversationRequest{
			RequestID:  "req_slot_wired",
			Model:      "chatgpt-img",
			InputParts: []contract.ContentPart{{Kind: contract.ContentPartText, Text: "hi"}},
			Provider: providercontract.Provider{
				ID:          1,
				AdapterType: "reverse-proxy-chatgpt-web",
				Protocol:    "openai-compatible",
			},
			Account: accountcontract.ProviderAccount{
				ID:           44,
				RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
				Metadata: map[string]any{
					"base_url":                          upstream.URL,
					"chatgpt_requirements_token":        "tok-slot",
					"chatgpt_image_generation":          "true",
					"chatgpt_image_account_concurrency": "1",
				},
			},
			Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
			Credential: map[string]any{"access_token": "tok-slot"},
		}
	}
	// Two sequential calls must succeed (proving the slot was released).
	for i := 0; i < 2; i++ {
		if _, err := svc.InvokeConversation(context.Background(), build()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
}

func TestChatGPTWebImageGenerationUsesOfficialConversationFlow(t *testing.T) {
	imageBytes := buildTestPNG(t, 2, 2)
	var paths []string
	var imageDownloadURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/backend-api/f/conversation/prepare":
			if got := r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token"); got != "tok-image" {
				t.Fatalf("expected prepare sentinel token, got %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode prepare payload: %v", err)
			}
			if payload["model"] != "gpt-image-web" {
				t.Fatalf("expected prepare model to be mapped, got %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			if got := r.Header.Get("X-Conduit-Token"); got != "conduit-test" {
				t.Fatalf("expected conduit token, got %q", got)
			}
			if got := r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token"); got != "tok-image" {
				t.Fatalf("expected conversation sentinel token, got %q", got)
			}
			if got := r.Header.Get("X-Oai-Turn-Trace-Id"); got == "" {
				t.Fatalf("expected turn trace header")
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode image conversation payload: %v", err)
			}
			if payload["model"] != "gpt-image-web" {
				t.Fatalf("expected mapped image model, got %+v", payload)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`data: {"conversation_id":"conv_img","message":{"author":{"role":"tool"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://file_00000000abcdefabcdefabcdef","width":2,"height":2,"size_bytes":77}]}}}` + "\n\ndata: [DONE]\n\n"))
		case "/backend-api/files/file_00000000abcdefabcdefabcdef/download":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":` + strconv.Quote(imageDownloadURL) + `}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer upstream.Close()
	defer downloadServer.Close()
	imageDownloadURL = downloadServer.URL + "/image.png"

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(downloadServer.Client(), runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageGeneration(context.Background(), contract.ImageGenerationRequest{
		RequestID:      "req_chatgpt_web_image",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/images/generations",
		Model:          "chatgpt-image-local",
		Prompt:         "draw a small red square",
		Count:          1,
		Size:           "1024x1024",
		ResponseFormat: "b64_json",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           66,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-image",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-image-web"},
		Credential: map[string]any{"access_token": "tok-image"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web image generation: %v", err)
	}
	if resp.Model != "gpt-image-web" || len(resp.Data) != 1 {
		t.Fatalf("unexpected image response: %+v", resp)
	}
	if got := resp.Data[0].Base64JSON; got != base64.StdEncoding.EncodeToString(imageBytes) {
		t.Fatalf("unexpected image base64 %q", got)
	}
	if resp.Data[0].Metadata["conversation_id"] != "conv_img" || resp.Data[0].Metadata["source"] != "file-service" {
		t.Fatalf("expected image metadata, got %+v", resp.Data[0].Metadata)
	}
	for _, path := range paths {
		if path == "/v1/images/generations" {
			t.Fatalf("chatgpt web image generation must not use OpenAI-compatible image endpoint; paths=%v", paths)
		}
	}
}

func TestChatGPTWebImageGenerationURLFormatDoesNotDownloadImageBytes(t *testing.T) {
	var conversationRuns int
	var downloadMetadataHits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/f/conversation/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			conversationRuns++
			w.Header().Set("Content-Type", "text/event-stream")
			fileID := "file_00000000abcdefabcdefabcde" + strconv.Itoa(conversationRuns)
			_, _ = w.Write([]byte(`data: {"conversation_id":"conv_img_` + strconv.Itoa(conversationRuns) + `","message":{"author":{"role":"tool"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://` + fileID + `","width":2,"height":2,"size_bytes":77}]}}}` + "\n\ndata: [DONE]\n\n"))
		case "/backend-api/files/file_00000000abcdefabcdefabcde1/download":
			downloadMetadataHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":"https://cdn.example.test/image-1.png"}`))
		case "/backend-api/files/file_00000000abcdefabcdefabcde2/download":
			downloadMetadataHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":"https://cdn.example.test/image-2.png"}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageGeneration(context.Background(), contract.ImageGenerationRequest{
		RequestID:      "req_chatgpt_web_image_url",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/images/generations",
		Model:          "chatgpt-image-local",
		Prompt:         "draw two small squares",
		Count:          2,
		ResponseFormat: "url",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           67,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-image",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-image-web"},
		Credential: map[string]any{"access_token": "tok-image"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web image generation: %v", err)
	}
	if conversationRuns != 2 || downloadMetadataHits != 2 {
		t.Fatalf("expected two official image runs and metadata hits, runs=%d metadata=%d", conversationRuns, downloadMetadataHits)
	}
	if len(resp.Data) != 2 || resp.Data[0].URL == "" || resp.Data[1].URL == "" {
		t.Fatalf("expected two image URLs, got %+v", resp.Data)
	}
	if resp.Data[0].Base64JSON != "" || resp.Data[1].Base64JSON != "" {
		t.Fatalf("url response_format must not download/encode image bytes, got %+v", resp.Data)
	}
}

func TestChatGPTWebImageEditUsesOfficialConversationFlow(t *testing.T) {
	inputImage := buildTestPNG(t, 3, 2)
	maskImage := buildSolidAlphaPNG(t, 3, 2, 0)
	generatedImage := buildTestPNG(t, 2, 2)
	var conversationBody []byte
	var paths []string
	var imageDownloadURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case r.URL.Path == "/backend-api/files" && r.Method == http.MethodPost:
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode upload register: %v", err)
			}
			if payload["use_case"] != "multimodal" || payload["file_name"] != "input.png" {
				t.Fatalf("unexpected upload register payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_id":"file_uploaded_edit","upload_url":"` + uploadBlobURL(r.Host, "/blob-edit") + `"}`))
		case r.URL.Path == "/blob-edit" && r.Method == http.MethodPut:
			if got := r.Header.Get("x-ms-blob-type"); got != "BlockBlob" {
				t.Fatalf("expected blob upload header, got %q", got)
			}
			uploaded, err := readBody(r)
			if err != nil {
				t.Fatalf("read uploaded edit image: %v", err)
			}
			decoded, err := png.Decode(bytes.NewReader(uploaded))
			if err != nil {
				t.Fatalf("edit upload should be alpha-composited PNG: %v", err)
			}
			if _, _, _, a := decoded.At(0, 0).RGBA(); a != 0 {
				t.Fatalf("expected mask alpha to be applied to edit upload, got alpha=%d", a)
			}
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/backend-api/files/file_uploaded_edit/uploaded" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/backend-api/f/conversation/prepare" && r.Method == http.MethodPost:
			if got := r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token"); got != "tok-edit" {
				t.Fatalf("expected prepare sentinel token, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-edit"}`))
		case r.URL.Path == "/backend-api/f/conversation" && r.Method == http.MethodPost:
			if got := r.Header.Get("X-Conduit-Token"); got != "conduit-edit" {
				t.Fatalf("expected conduit token, got %q", got)
			}
			conversationBody, _ = readBody(r)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(
				`data: {"conversation_id":"conv_edit","message":{"author":{"role":"user"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://file_uploaded_edit","width":3,"height":2,"size_bytes":77}]}}}` + "\n\n" +
					`data: {"conversation_id":"conv_edit","message":{"author":{"role":"tool"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://file_generated_edit","width":2,"height":2,"size_bytes":77}]}}}` + "\n\ndata: [DONE]\n\n",
			))
		case r.URL.Path == "/backend-api/files/file_uploaded_edit/download" && r.Method == http.MethodGet:
			t.Fatalf("input image reference must not be treated as generated output")
		case r.URL.Path == "/backend-api/files/file_generated_edit/download" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":` + strconv.Quote(imageDownloadURL) + `}`))
		default:
			t.Fatalf("unexpected upstream call %s %s", r.Method, r.URL.Path)
		}
	}))
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(generatedImage)
	}))
	defer upstream.Close()
	defer downloadServer.Close()
	imageDownloadURL = downloadServer.URL + "/generated.png"

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(downloadServer.Client(), runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageEdit(context.Background(), contract.ImageEditRequest{
		RequestID:      "req_chatgpt_web_edit",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/images/edits",
		Model:          "chatgpt-image-local",
		Prompt:         "make the object blue",
		Images: []contract.ImageInput{{
			FileName:    "input.png",
			ContentType: "image/png",
			Bytes:       inputImage,
		}},
		Mask: &contract.ImageInput{
			FileName:    "mask.png",
			ContentType: "image/png",
			Bytes:       maskImage,
		},
		Count:          1,
		ResponseFormat: "b64_json",
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           68,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-edit",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-image-web"},
		Credential: map[string]any{"access_token": "tok-edit"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web image edit: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Base64JSON != base64.StdEncoding.EncodeToString(generatedImage) {
		t.Fatalf("unexpected edit response: %+v", resp)
	}
	if !bytes.Contains(conversationBody, []byte("multimodal_text")) ||
		!bytes.Contains(conversationBody, []byte("file-service://file_uploaded_edit")) ||
		!bytes.Contains(conversationBody, []byte(`"attachments"`)) {
		t.Fatalf("expected uploaded image reference in official conversation payload: %s", string(conversationBody))
	}
	for _, path := range paths {
		if path == "/v1/images/edits" {
			t.Fatalf("chatgpt web image edit must not use OpenAI-compatible edit endpoint; paths=%v", paths)
		}
	}
}

func TestChatGPTWebImageVariationUsesOfficialConversationFlow(t *testing.T) {
	inputImage := buildTestPNG(t, 2, 2)
	var conversationBody []byte
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case r.URL.Path == "/backend-api/files" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_id":"file_variation_input","upload_url":"` + uploadBlobURL(r.Host, "/blob-variation") + `"}`))
		case r.URL.Path == "/blob-variation" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/backend-api/files/file_variation_input/uploaded" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/backend-api/f/conversation/prepare" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-var"}`))
		case r.URL.Path == "/backend-api/f/conversation" && r.Method == http.MethodPost:
			conversationBody, _ = readBody(r)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(
				`data: {"conversation_id":"conv_var","message":{"author":{"role":"user"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://file_variation_input","width":2,"height":2,"size_bytes":77}]}}}` + "\n\n" +
					`data: {"conversation_id":"conv_var","message":{"author":{"role":"tool"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"file-service://file_generated_var","width":2,"height":2,"size_bytes":77}]}}}` + "\n\ndata: [DONE]\n\n",
			))
		case r.URL.Path == "/backend-api/files/file_variation_input/download" && r.Method == http.MethodGet:
			t.Fatalf("variation input reference must not be treated as generated output")
		case r.URL.Path == "/backend-api/files/file_generated_var/download" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":"https://cdn.example.test/variation.png"}`))
		default:
			t.Fatalf("unexpected upstream call %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeImageVariation(context.Background(), contract.ImageVariationRequest{
		RequestID:      "req_chatgpt_web_variation",
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/images/variations",
		Model:          "chatgpt-image-local",
		Image: contract.ImageInput{
			FileName:    "source.png",
			ContentType: "image/png",
			Bytes:       inputImage,
		},
		Count:          1,
		ResponseFormat: "url",
		Extra:          map[string]any{"prompt": "keep the composition but change the mood"},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           69,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-var",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-image-web"},
		Credential: map[string]any{"access_token": "tok-var"},
	})
	if err != nil {
		t.Fatalf("invoke chatgpt web image variation: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.test/variation.png" || resp.Data[0].Base64JSON != "" {
		t.Fatalf("unexpected variation response: %+v", resp.Data)
	}
	if !bytes.Contains(conversationBody, []byte("file-service://file_variation_input")) ||
		!bytes.Contains(conversationBody, []byte("keep the composition but change the mood")) {
		t.Fatalf("expected variation prompt and uploaded reference in payload: %s", string(conversationBody))
	}
	for _, path := range paths {
		if path == "/v1/images/variations" {
			t.Fatalf("chatgpt web image variation must not use OpenAI-compatible variation endpoint; paths=%v", paths)
		}
	}
}

// TestChatGPTWebWiringWSFallbackHeaderSurfacedWhenUpstreamOffersWS proves the
// WS-fallback inspector sees an upstream wss_url and surfaces the header on
// the gateway response.
func TestChatGPTWebWiringWSFallbackHeaderSurfacedWhenUpstreamOffersWS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Include a wss_url so the fallback inspector triggers.
		_, _ = w.Write([]byte(`data: {"wss_url":"wss://example.com/conv","message":{"author":{"role":"assistant"},"content":{"parts":["fallback-ok"]}}}` + "\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	runtime, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	svc, err := service.NewWithReverseProxy(nil, runtime)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	resp, err := svc.InvokeConversation(context.Background(), contract.ConversationRequest{
		RequestID:  "req_ws_wired",
		Model:      "chatgpt-ws",
		InputParts: []contract.ContentPart{{Kind: contract.ContentPartText, Text: "hi"}},
		Provider: providercontract.Provider{
			ID:          1,
			AdapterType: "reverse-proxy-chatgpt-web",
			Protocol:    "openai-compatible",
		},
		Account: accountcontract.ProviderAccount{
			ID:           55,
			RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
			Metadata: map[string]any{
				"base_url":                   upstream.URL,
				"chatgpt_requirements_token": "tok-ws",
			},
		},
		Mapping:    modelcontract.ModelProviderMapping{UpstreamModelName: "gpt-5-chat"},
		Credential: map[string]any{"access_token": "tok-ws"},
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := resp.Headers.Get(service.ChatGPTWebWSFallbackResponseHeader); got != "1" {
		t.Fatalf("expected WS-fallback header to be set; headers=%v", resp.Headers)
	}
}

// Helpers shared across the wiring tests.

func buildTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 60), uint8(y * 80), 100, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func buildSolidAlphaPNG(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{255, 255, 255, alpha}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode mask png: %v", err)
	}
	return buf.Bytes()
}

func uploadBlobURL(host string, path string) string {
	return "http://" + host + path
}

func readBody(r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return buf.Bytes(), nil
}

// _ keeps json import live in case future assertions need it.
var _ = json.Marshal
