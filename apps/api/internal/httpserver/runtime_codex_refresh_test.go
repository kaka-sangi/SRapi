package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const codexOAuthClientIDForTest = "app_EMoamEEZ73f0CkXaXp7hrann"

type codexUpstreamObservation struct {
	Path          string
	Authorization string
	Originator    string
	Beta          string
	UserAgent     string
	SessionID     string
	Version       string
	RequestID     string
	Payload       map[string]any
}

func TestGatewayCodexRefreshTokenOnlyCreateCanRequestResponses(t *testing.T) {
	var tokenCalls int
	var responseCalls int
	var tokenForm url.Values
	var responseAuthorization string
	var responsePath string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenForm = cloneURLValues(r.PostForm)
			if r.Method != http.MethodPost ||
				r.PostForm.Get("grant_type") != "refresh_token" ||
				r.PostForm.Get("refresh_token") != "create-refresh" ||
				r.PostForm.Get("client_id") != codexOAuthClientIDForTest ||
				r.PostForm.Get("scope") != "openid profile email" {
				t.Fatalf("unexpected codex refresh request: method=%s form=%v", r.Method, r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"create-access","refresh_token":"create-refresh-rotated","id_token":"id-token","token_type":"Bearer","expires_in":3600}`)
		case "/backend-api/codex/responses":
			responseCalls++
			responseAuthorization = r.Header.Get("Authorization")
			responsePath = r.URL.Path
			if r.Header.Get("Originator") != "codex_cli_rs" || r.Header.Get("User-Agent") != "codex-cli/test" {
				t.Fatalf("unexpected codex headers: %+v", r.Header)
			}
			var payload struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode codex responses payload: %v", err)
			}
			if payload.Model != "codex-upstream" || !payload.Stream {
				t.Fatalf("unexpected codex responses payload: %+v", payload)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex create ok\"}\n\ndata: [DONE]\n\n")
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-refresh-create-provider","display_name":"Codex Refresh Create","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-refresh-create-model","display_name":"Codex Refresh Create Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"codex-refresh-create-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"refresh_token":"create-refresh"},"metadata":{"base_url":"` + upstream.URL + `/backend-api/codex","oauth_token_url":"` + upstream.URL + `/oauth/token","user_agent":"codex-cli/test"},"status":"active"}`
	_, rawAccount := mustCreateAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	if strings.Contains(rawAccount, "create-refresh") || strings.Contains(rawAccount, "create-access") {
		t.Fatalf("account create response leaked credential: %s", rawAccount)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-refresh-create-model","input":"hello codex"}`)
	if !strings.Contains(rec.Body.String(), "codex create ok") {
		t.Fatalf("expected codex response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 {
		t.Fatalf("expected one token refresh call, got %d form=%v", tokenCalls, tokenForm)
	}
	if responseCalls != 1 || responseAuthorization != "Bearer create-access" || responsePath != "/backend-api/codex/responses" {
		t.Fatalf("unexpected codex upstream call count=%d auth=%q path=%q", responseCalls, responseAuthorization, responsePath)
	}
}

func TestGatewayCodexResponsesStreamReplaysRawSSE(t *testing.T) {
	rawSSE := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_raw\",\"raw_marker\":\"created\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"raw codex\",\"raw_marker\":\"delta\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"raw codex\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":3}},\"raw_marker\":\"completed\"}\n\n" +
		"data: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected codex upstream path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer codex-stream-token" {
			t.Fatalf("expected codex stream auth header, got %q", r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex stream payload: %v", err)
		}
		if payload["stream"] != true {
			t.Fatalf("expected codex stream payload, got %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, rawSSE)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-raw-stream-provider","display_name":"Codex Raw Stream","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-raw-stream-model","display_name":"Codex Raw Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-raw-stream-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-stream-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-raw-stream-model","input":"raw stream","stream":true}`)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}
	if got := rec.Body.String(); got != rawSSE {
		t.Fatalf("expected raw Codex Responses SSE replay\nexpected:\n%s\nactual:\n%s", rawSSE, got)
	}
}

func TestGatewayCodexResponsesCompactReplaysRawJSON(t *testing.T) {
	var upstreamPath string
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&upstreamPayload); err != nil {
			t.Fatalf("decode compact payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"cmp_gateway","object":"response.compaction","input_tokens":12,"output_tokens":3,"raw_only_marker":"compact-upstream"}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-compact-provider","display_name":"Codex Compact","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-compact-model","display_name":"Codex Compact Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-compact-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-compact-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-compact-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","capability_responses_compact":true},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses/compact", `{"model":"codex-compact-model","input":"compact this","previous_response_id":"resp_prev","stream":false}`)
	if upstreamPath != "/backend-api/codex/responses/compact" {
		t.Fatalf("expected compact upstream path, got %q", upstreamPath)
	}
	if upstreamPayload["model"] != "codex-compact-upstream" ||
		upstreamPayload["previous_response_id"] != "resp_prev" ||
		upstreamPayload["stream"] != false {
		t.Fatalf("expected mapped compact payload, got %+v", upstreamPayload)
	}
	var response map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode compact response: %v", err)
	}
	if response["object"] != "response.compaction" || response["raw_only_marker"] != "compact-upstream" {
		t.Fatalf("expected raw compact JSON response, got %+v", response)
	}
}

func TestGatewayCodexConvertsTextEndpointsToResponsesUpstream(t *testing.T) {
	var calls []codexUpstreamObservation
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex payload: %v", err)
		}
		calls = append(calls, codexUpstreamObservation{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Originator:    r.Header.Get("Originator"),
			Beta:          r.Header.Get("OpenAI-Beta"),
			UserAgent:     r.Header.Get("User-Agent"),
			SessionID:     r.Header.Get("Session_id"),
			Version:       r.Header.Get("Version"),
			RequestID:     r.Header.Get("X-Client-Request-Id"),
			Payload:       payload,
		})
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex converted ok\"}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-convert-provider","display_name":"Codex Convert","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-convert-model","display_name":"Codex Convert Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"structured_output","level":"optional","status":"stable","version":"v1"},{"key":"reasoning_control","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-convert-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-convert-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	responsesBody := `{"model":"codex-convert-model","instructions":"raw instructions","input":[{"role":"system","content":"raw system"},{"role":"user","content":"raw responses"}],"stream":false,"store":true,"service_tier":"fast","reasoning":{"effort":"high"},"text":{"format":{"type":"text"},"verbosity":"low"},"previous_response_id":"resp_prev","parallel_tool_calls":true,"metadata":{"downstream":"removed"},"temperature":0.2,"max_output_tokens":32}`
	if rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", responsesBody); !strings.Contains(rec.Body.String(), "codex converted ok") {
		t.Fatalf("expected responses conversion output, got %s", rec.Body.String())
	}
	if rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"codex-convert-model","messages":[{"role":"system","content":"chat system"},{"role":"user","content":"chat prompt"}],"response_format":{"type":"json_object"},"stream":false}`); !strings.Contains(rec.Body.String(), "codex converted ok") {
		t.Fatalf("expected chat conversion output, got %s", rec.Body.String())
	}
	if rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"codex-convert-model","system":"messages system","max_tokens":32,"messages":[{"role":"user","content":"messages prompt"}]}`); !strings.Contains(rec.Body.String(), "codex converted ok") {
		t.Fatalf("expected messages conversion output, got %s", rec.Body.String())
	}

	if len(calls) != 3 {
		t.Fatalf("expected three codex upstream calls, got %d: %+v", len(calls), calls)
	}
	for idx, call := range calls {
		if call.Path != "/backend-api/codex/responses" ||
			call.Authorization != "Bearer codex-convert-token" ||
			call.Originator != "codex_cli_rs" ||
			call.Beta != "responses=experimental" ||
			call.UserAgent != "codex_cli_rs/0.125.0" ||
			call.Version != "0.125.0" ||
			!strings.HasPrefix(call.SessionID, "srapi-codex-account-") ||
			strings.TrimSpace(call.RequestID) == "" {
			t.Fatalf("unexpected codex headers for call %d: %+v", idx, call)
		}
		if call.Payload["model"] != "codex-upstream" || call.Payload["stream"] != true || call.Payload["store"] != false {
			t.Fatalf("unexpected codex payload defaults for call %d: %+v", idx, call.Payload)
		}
	}
	first := calls[0].Payload
	if first["instructions"] != "raw instructions\nraw system" ||
		first["service_tier"] != "priority" ||
		first["previous_response_id"] != "resp_prev" ||
		first["parallel_tool_calls"] != true {
		t.Fatalf("unexpected raw responses conversion: %+v", first)
	}
	if _, ok := first["metadata"]; ok {
		t.Fatalf("metadata should be stripped for Codex internal upstream: %+v", first)
	}
	if _, ok := first["max_output_tokens"]; ok {
		t.Fatalf("max_output_tokens should be stripped for Codex internal upstream: %+v", first)
	}
	if got := codexObservationUserText(first); got != "raw responses" {
		t.Fatalf("expected raw responses user text, got %q from %+v", got, first)
	}
	if got := codexObservationUserText(calls[1].Payload); got != "chat prompt" {
		t.Fatalf("expected chat user text, got %q from %+v", got, calls[1].Payload)
	}
	if calls[1].Payload["instructions"] != "chat system" {
		t.Fatalf("expected chat system instructions, got %+v", calls[1].Payload)
	}
	if got := codexObservationUserText(calls[2].Payload); got != "messages prompt" {
		t.Fatalf("expected messages user text, got %q from %+v", got, calls[2].Payload)
	}
	if calls[2].Payload["instructions"] != "messages system" {
		t.Fatalf("expected messages system instructions, got %+v", calls[2].Payload)
	}
}

func TestGatewayCodexResponsesRawBodyClearedAfterContentSafetyRedaction(t *testing.T) {
	var payload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex redacted ok\"}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-redact-provider","display_name":"Codex Redact","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-redact-model","display_name":"Codex Redact Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-redact-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-redact-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rawEmail := "ada@example.com"
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-redact-model","input":"Email ada@example.com before replying"}`)
	if !strings.Contains(rec.Body.String(), "codex redacted ok") {
		t.Fatalf("expected redacted output, got %s", rec.Body.String())
	}
	if payload == nil {
		t.Fatal("expected codex upstream payload")
	}
	rendered, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal codex payload: %v", err)
	}
	if strings.Contains(string(rendered), rawEmail) {
		t.Fatalf("raw responses body bypassed content safety: %s", rendered)
	}
	if !strings.Contains(string(rendered), "[REDACTED_EMAIL]") {
		t.Fatalf("expected redacted email in codex payload, got %s", rendered)
	}
}

func codexObservationUserText(payload map[string]any) string {
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return ""
	}
	item, ok := input[0].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := item["content"].([]any)
	if !ok || len(content) == 0 {
		return ""
	}
	part, ok := content[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := part["text"].(string)
	return strings.TrimSpace(text)
}

func TestAdminAccountImportCodexRefreshTokenOnlyExchangesTokenWithoutLeakingCredential(t *testing.T) {
	var tokenCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		tokenCalls++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.PostForm.Get("refresh_token") != "import-refresh" || r.PostForm.Get("client_id") != codexOAuthClientIDForTest {
			t.Fatalf("unexpected import refresh form: %v", r.PostForm)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"import-access","refresh_token":"import-refresh-rotated","expires_in":3600}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-refresh-import-provider","display_name":"Codex Refresh Import","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	body := `{"accounts":[{"provider_id":"` + string(providerResp.Data.Id) + `","name":"codex-refresh-import-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"refresh_token":"import-refresh"},"metadata":{"base_url":"https://codex.invalid/backend-api/codex","oauth_token_url":"` + upstream.URL + `/oauth/token"},"status":"active"}]}`

	importResp, rawBody := mustImportAdminAccountsRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, body)
	if importResp.Data.CreatedCount != 1 || importResp.Data.SkippedCount != 0 || len(importResp.Data.Errors) != 0 || tokenCalls != 1 {
		t.Fatalf("unexpected import response: %+v token_calls=%d", importResp.Data, tokenCalls)
	}
	if strings.Contains(rawBody, "import-refresh") || strings.Contains(rawBody, "import-access") {
		t.Fatalf("import response leaked credential: %s", rawBody)
	}
}

func TestGatewayCodexRefreshTokenOnlyUpdateCanRequestResponses(t *testing.T) {
	var tokenCalls int
	var responseAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.PostForm.Get("refresh_token") != "updated-refresh" || r.PostForm.Get("scope") != "openid profile email" {
				t.Fatalf("unexpected update refresh form: %v", r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"updated-access","refresh_token":"updated-refresh-rotated","expires_in":3600}`)
		case "/backend-api/codex/responses":
			responseAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"codex update ok\"}\n\ndata: [DONE]\n\n")
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-refresh-update-provider","display_name":"Codex Refresh Update","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-refresh-update-model","display_name":"Codex Refresh Update Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-refresh-update-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"old-access","refresh_token":"old-refresh"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","oauth_token_url":"`+upstream.URL+`/oauth/token"},"status":"active"}`)

	updateBody := `{"credential":{"refresh_token":"updated-refresh"}}`
	rawUpdate := mustPatchAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(accountResp.Data.Id), updateBody)
	if strings.Contains(rawUpdate, "updated-refresh") || strings.Contains(rawUpdate, "updated-access") {
		t.Fatalf("account update response leaked credential: %s", rawUpdate)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-refresh-update-model","input":"hello codex update"}`)
	if !strings.Contains(rec.Body.String(), "codex update ok") {
		t.Fatalf("expected codex update response text, got %s", rec.Body.String())
	}
	if tokenCalls != 1 || responseAuthorization != "Bearer updated-access" {
		t.Fatalf("unexpected updated credential use: token_calls=%d auth=%q", tokenCalls, responseAuthorization)
	}
}

func TestGatewayCodexServeTimeRefreshFailureProtectsAccount(t *testing.T) {
	cases := []struct {
		name        string
		serveStatus int
		serveBody   string
		wantStatus  string
	}{
		{"permanent invalid_grant parks needs_reauth", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"refresh token revoked"}`, "needs_reauth"},
		{"transient upstream error keeps active", http.StatusInternalServerError, `{"error":"server_error"}`, "active"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var tokenCalls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					t.Fatalf("unexpected upstream path %s", r.URL.Path)
				}
				tokenCalls++
				w.Header().Set("Content-Type", "application/json")
				if tokenCalls == 1 {
					// Import-time mint: succeed but expire within the refresh skew so the
					// next gateway request performs a serve-time refresh.
					_, _ = io.WriteString(w, `{"access_token":"mint-access","refresh_token":"mint-rotated","expires_in":1}`)
					return
				}
				// Serve-time refresh outcome under test.
				w.WriteHeader(tc.serveStatus)
				_, _ = io.WriteString(w, tc.serveBody)
			}))
			defer upstream.Close()

			handler := New(config.Load(), nil)
			loginResp, sessionCookie := mustLoginAdmin(t, handler)
			providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-serve-refresh-provider","display_name":"Codex Serve Refresh","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
			modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-serve-refresh-model","display_name":"Codex Serve Refresh Model","status":"active"}`)
			mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active"}`)
			accountResp, _ := mustCreateAdminAccountRaw(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-serve-refresh-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"refresh_token":"mint-refresh"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","oauth_token_url":"`+upstream.URL+`/oauth/token","user_agent":"codex-cli/test"},"status":"active"}`)
			if string(accountResp.Data.Status) != "active" {
				t.Fatalf("expected account active after import mint, got %q", accountResp.Data.Status)
			}

			_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
			gwReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"codex-serve-refresh-model","input":"hello"}`))
			gwReq.Header.Set("Content-Type", "application/json")
			gwReq.Header.Set("Authorization", "Bearer "+apiKey)
			gwRec := httptest.NewRecorder()
			handler.ServeHTTP(gwRec, gwReq)
			if gwRec.Code == http.StatusOK {
				t.Fatalf("expected gateway failure when serve-time refresh fails, got 200 body=%s", gwRec.Body.String())
			}
			if tokenCalls < 2 {
				t.Fatalf("expected a serve-time refresh attempt after import, token calls=%d", tokenCalls)
			}
			if strings.Contains(gwRec.Body.String(), "mint-refresh") || strings.Contains(gwRec.Body.String(), "mint-access") {
				t.Fatalf("gateway error leaked credential material: %s", gwRec.Body.String())
			}

			getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
			getReq.AddCookie(sessionCookie)
			getRec := httptest.NewRecorder()
			handler.ServeHTTP(getRec, getReq)
			if getRec.Code != http.StatusOK {
				t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
			}
			var getResp apiopenapi.ProviderAccountResponse
			if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
				t.Fatalf("decode account inspect: %v", err)
			}
			if string(getResp.Data.Status) != tc.wantStatus {
				t.Fatalf("expected account status %q after serve-time refresh failure, got %q", tc.wantStatus, getResp.Data.Status)
			}
		})
	}
}

func mustCreateAdminAccountRaw(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) (apiopenapi.ProviderAccountResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	var resp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	return resp, raw
}

func mustImportAdminAccountsRaw(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) (apiopenapi.ProviderAccountImportResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account import 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	var resp apiopenapi.ProviderAccountImportResponse
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&resp); err != nil {
		t.Fatalf("decode account import response: %v", err)
	}
	return resp, raw
}

func mustPatchAdminAccountRaw(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, accountID, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+accountID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func cloneURLValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, vals := range values {
		out[key] = append([]string(nil), vals...)
	}
	return out
}
