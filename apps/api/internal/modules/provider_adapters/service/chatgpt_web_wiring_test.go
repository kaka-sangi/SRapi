package service_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
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
