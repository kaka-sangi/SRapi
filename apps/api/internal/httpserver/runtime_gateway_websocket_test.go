package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"nhooyr.io/websocket"
)

func TestGatewayResponsesWebSocketTargetsResponsesRuntime(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		Model         string
		Messages      []upstreamMessage
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string            `json:"model"`
			Messages []upstreamMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Messages:      payload.Messages,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ws response ok"}}],"usage":{"prompt_tokens":6,"completion_tokens":7,"total_tokens":13}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp380-ws-provider","display_name":"WP380 WS Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp380-ws-model","display_name":"WP380 WS Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"wp380-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp380-ws-account","runtime_class":"api_key","credential":{"api_key":"wp380-upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","session_affinity_key":"wp380-session"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	conn := mustDialResponsesWebSocket(t, server.URL+"/v1/responses/ws?model=wp380-ws-model&session_affinity_key=wp380-session&sticky_strength=hard", apiKey)
	defer conn.Close(websocket.StatusNormalClosure, "")

	writeWebSocketJSON(t, conn, map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"input": "hello over websocket",
		},
	})
	event := readWebSocketEvent(t, conn)
	if event["type"] != "response.completed" {
		t.Fatalf("expected response.completed event, got %+v", event)
	}
	response, ok := event["response"].(map[string]any)
	if !ok || response["model"] != "wp380-ws-model" {
		t.Fatalf("expected completed response payload, got %+v", event)
	}
	rawOutput, ok := response["output"].([]any)
	if !ok || len(rawOutput) != 1 {
		t.Fatalf("expected response output, got %+v", response)
	}
	if !strings.Contains(mustMarshalString(t, rawOutput[0]), "ws response ok") {
		t.Fatalf("expected upstream text in websocket response, got %+v", rawOutput[0])
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/chat/completions" || call.Authorization != "Bearer wp380-upstream-secret" || call.Model != "wp380-upstream" {
		t.Fatalf("unexpected upstream call: %+v", call)
	}
	if len(call.Messages) != 1 || !strings.Contains(call.Messages[0].Content, "hello over websocket") {
		t.Fatalf("expected responses prompt forwarded upstream, got %+v", call.Messages)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp380-ws-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one scheduler decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SourceEndpoint != responsesWebSocketSourceEndpoint || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(accountResp.Data.Id) || !decision.StickyHit {
		t.Fatalf("expected websocket source endpoint and sticky account evidence, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp380-ws-model", nil)
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
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].SourceEndpoint != responsesWebSocketSourceEndpoint || usageResp.Data[0].TotalTokens != 13 {
		t.Fatalf("unexpected websocket usage record: %+v", usageResp.Data)
	}
}

func TestGatewayResponsesWebSocketForwardsStreamingEvents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream stream request: %v", err)
		}
		if !payload.Stream {
			t.Fatalf("expected upstream stream request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ws\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" stream\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp380-stream-provider","display_name":"WP380 Stream Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp380-stream-model","display_name":"WP380 Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"wp380-stream-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp380-stream-account","runtime_class":"api_key","credential":{"api_key":"wp380-stream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	conn := mustDialResponsesWebSocket(t, server.URL+"/v1/responses/ws", apiKey)
	defer conn.Close(websocket.StatusNormalClosure, "")

	writeWebSocketJSON(t, conn, map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"model":  "wp380-stream-model",
			"input":  "stream over websocket",
			"stream": true,
		},
	})

	created := readWebSocketEvent(t, conn)
	delta := readWebSocketEvent(t, conn)
	doneText := readWebSocketEvent(t, conn)
	completed := readWebSocketEvent(t, conn)
	if created["type"] != "response.created" || delta["type"] != "response.output_text.delta" || doneText["type"] != "response.output_text.done" || completed["type"] != "response.completed" {
		t.Fatalf("unexpected stream events: created=%+v delta=%+v done=%+v completed=%+v", created, delta, doneText, completed)
	}
	if delta["delta"] != "ws stream" {
		t.Fatalf("expected aggregated stream delta, got %+v", delta)
	}
	if !strings.Contains(mustMarshalString(t, completed), "ws stream") {
		t.Fatalf("expected completed stream payload, got %+v", completed)
	}
}

func TestGatewayResponsesWebSocketRelaysCodexUpstreamWebSocket(t *testing.T) {
	type upstreamObservation struct {
		Path          string
		Authorization string
		Originator    string
		Beta          string
		AccountID     string
		RequestID     string
		Version       string
		UserAgent     string
		SessionID     string
		InitialFrame  []byte
	}
	observations := make(chan upstreamObservation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			t.Errorf("accept codex upstream websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		msgType, payload, err := conn.Read(r.Context())
		if err != nil {
			t.Errorf("read codex upstream frame: %v", err)
			return
		}
		if msgType != websocket.MessageText {
			t.Errorf("expected codex upstream text frame, got %v", msgType)
			return
		}
		observations <- upstreamObservation{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Originator:    r.Header.Get("Originator"),
			Beta:          r.Header.Get("OpenAI-Beta"),
			AccountID:     r.Header.Get("ChatGPT-Account-ID"),
			RequestID:     r.Header.Get("X-Client-Request-Id"),
			Version:       r.Header.Get("Version"),
			UserAgent:     r.Header.Get("User-Agent"),
			SessionID:     r.Header.Get("session_id"),
			InitialFrame:  append([]byte(nil), payload...),
		}
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.created","response":{"id":"resp_ws","model":"codex-upstream"}}`)); err != nil {
			t.Errorf("write codex created frame: %v", err)
			return
		}
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_ws","model":"codex-upstream","output":[{"type":"message","content":[{"type":"output_text","text":"codex websocket ok"}]}],"usage":{"input_tokens":8,"output_tokens":9,"cached_tokens":1}}}`)); err != nil {
			t.Errorf("write codex completed frame: %v", err)
			return
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp410-codex-ws-provider","display_name":"WP410 Codex WS Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp410-codex-ws-model","display_name":"WP410 Codex WS Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp410-codex-ws-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-ws-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","codex_responses_websocket":true,"user_agent":"codex-cli/0.118.0 (Mac OS)","chatgpt_account_id":"chatgpt-ws-account","codex_client_request_id":"codex-ws-client-req","codex_session_id":"codex-ws-session","codex_version":"0.118.0"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	conn := mustDialResponsesWebSocket(t, server.URL+"/v1/responses/ws?model=wp410-codex-ws-model&upstream_ws=true", apiKey)
	defer conn.Close(websocket.StatusNormalClosure, "")

	writeWebSocketJSON(t, conn, map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"input":  "hello codex websocket",
			"stream": true,
		},
	})
	created := readWebSocketEvent(t, conn)
	completed := readWebSocketEvent(t, conn)
	if created["type"] != "response.created" || completed["type"] != "response.completed" {
		t.Fatalf("unexpected codex websocket events: created=%+v completed=%+v", created, completed)
	}
	if !strings.Contains(mustMarshalString(t, completed), "codex websocket ok") {
		t.Fatalf("expected codex upstream completion payload, got %+v", completed)
	}

	var obs upstreamObservation
	select {
	case obs = <-observations:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for codex upstream observation")
	}
	if obs.Path != "/backend-api/codex/responses" ||
		obs.Authorization != "Bearer codex-ws-token" ||
		obs.Originator != "codex_cli_rs" ||
		obs.Beta != "responses_websockets=2026-02-06" ||
		obs.AccountID != "chatgpt-ws-account" ||
		obs.RequestID != "codex-ws-client-req" ||
		obs.Version != "0.118.0" ||
		obs.UserAgent != "codex-cli/0.118.0 (Mac OS)" ||
		obs.SessionID != "codex-ws-session" {
		t.Fatalf("unexpected codex upstream request: %+v", obs)
	}
	var initialFrame map[string]any
	if err := json.Unmarshal(obs.InitialFrame, &initialFrame); err != nil {
		t.Fatalf("decode codex initial frame: %v", err)
	}
	if initialFrame["type"] != "response.create" ||
		initialFrame["model"] != "codex-upstream" ||
		initialFrame["input"] != "hello codex websocket" ||
		initialFrame["stream"] != true {
		t.Fatalf("unexpected codex initial frame: %+v", initialFrame)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp410-codex-ws-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one scheduler decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SourceEndpoint != responsesWebSocketSourceEndpoint || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(accountResp.Data.Id) {
		t.Fatalf("expected codex websocket scheduler evidence, got %+v", decision)
	}

	usageResp := waitForRealtimeUsageLog(t, handler, sessionCookie, "wp410-codex-ws-model")
	if len(usageResp.Data) != 1 ||
		!usageResp.Data[0].Success ||
		usageResp.Data[0].SourceEndpoint != responsesWebSocketSourceEndpoint ||
		usageResp.Data[0].TotalTokens != 18 ||
		usageResp.Data[0].UsageEstimated {
		t.Fatalf("unexpected codex websocket usage record: %+v", usageResp.Data)
	}
}

func TestGatewayRealtimeWebSocketRelaysOpenAIUpstreamWebSocket(t *testing.T) {
	type upstreamObservation struct {
		Path             string
		QueryModel       string
		Authorization    string
		SafetyIdentifier string
		UserAgent        string
		LeakedHeader     string
		ClientFrame      []byte
	}
	observations := make(chan upstreamObservation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			t.Errorf("accept realtime upstream websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		msgType, payload, err := conn.Read(r.Context())
		if err != nil {
			t.Errorf("read realtime upstream frame: %v", err)
			return
		}
		if msgType != websocket.MessageText {
			t.Errorf("expected realtime upstream text frame, got %v", msgType)
			return
		}
		observations <- upstreamObservation{
			Path:             r.URL.Path,
			QueryModel:       r.URL.Query().Get("model"),
			Authorization:    r.Header.Get("Authorization"),
			SafetyIdentifier: r.Header.Get("OpenAI-Safety-Identifier"),
			UserAgent:        r.Header.Get("User-Agent"),
			LeakedHeader:     r.Header.Get("X-SRapi-Leak"),
			ClientFrame:      append([]byte(nil), payload...),
		}
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"session.created","session":{"id":"sess_realtime"}}`)); err != nil {
			t.Errorf("write realtime upstream frame: %v", err)
			return
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp470-realtime-provider","display_name":"WP470 Realtime Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"realtime_websocket":true,"streaming":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp470-realtime-model","display_name":"WP470 Realtime Model","status":"active","capabilities":[{"key":"realtime_websocket","level":"required","status":"experimental","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-realtime-2","status":"active","capability_override":[{"key":"realtime_websocket","level":"required","status":"experimental","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp470-realtime-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"realtime-access-token","refresh_token":"refresh-token"},"metadata":{"base_url":"`+upstream.URL+`/v1","user_agent":"OpenAI-Realtime-Test/1.0","capability_realtime_websocket":true},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	conn := mustDialRealtimeWebSocket(t, server.URL+"/v1/realtime?model=wp470-realtime-model&session_affinity_key=wp470-realtime-session", apiKey, http.Header{
		"OpenAI-Safety-Identifier": {"safe-user-hash"},
		"X-SRapi-Leak":             {"leaked"},
	})
	defer conn.Close(websocket.StatusNormalClosure, "")

	writeWebSocketJSON(t, conn, map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"type":         "realtime",
			"instructions": "be concise",
		},
	})
	event := readWebSocketEvent(t, conn)
	if event["type"] != "session.created" {
		t.Fatalf("expected session.created event, got %+v", event)
	}

	var obs upstreamObservation
	select {
	case obs = <-observations:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for realtime upstream observation")
	}
	if obs.Path != "/v1/realtime" ||
		obs.QueryModel != "gpt-realtime-2" ||
		obs.Authorization != "Bearer realtime-access-token" ||
		obs.SafetyIdentifier != "safe-user-hash" ||
		obs.UserAgent != "OpenAI-Realtime-Test/1.0" ||
		obs.LeakedHeader != "" {
		t.Fatalf("unexpected realtime upstream request: %+v", obs)
	}
	var clientFrame map[string]any
	if err := json.Unmarshal(obs.ClientFrame, &clientFrame); err != nil {
		t.Fatalf("decode realtime upstream frame: %v", err)
	}
	if clientFrame["type"] != "session.update" || !strings.Contains(mustMarshalString(t, clientFrame), "be concise") {
		t.Fatalf("unexpected realtime upstream frame: %+v", clientFrame)
	}
	conn.Close(websocket.StatusNormalClosure, "")

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp470-realtime-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one scheduler decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SourceEndpoint != realtimeWebSocketSourceEndpoint || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(accountResp.Data.Id) {
		t.Fatalf("expected realtime scheduler evidence, got %+v", decision)
	}

	usageResp := waitForRealtimeUsageLog(t, handler, sessionCookie, "wp470-realtime-model")
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].SourceEndpoint != realtimeWebSocketSourceEndpoint {
		t.Fatalf("unexpected realtime usage record: %+v", usageResp.Data)
	}
}

func TestGatewayRealtimeWebSocketRelaysOpenAIAPIKeyUpstreamWebSocket(t *testing.T) {
	type upstreamObservation struct {
		Path             string
		QueryModel       string
		Authorization    string
		SafetyIdentifier string
		Cookie           string
		LeakedHeader     string
		ClientFrame      []byte
	}
	observations := make(chan upstreamObservation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			t.Errorf("accept api-key realtime upstream websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		msgType, payload, err := conn.Read(r.Context())
		if err != nil {
			t.Errorf("read api-key realtime upstream frame: %v", err)
			return
		}
		if msgType != websocket.MessageText {
			t.Errorf("expected api-key realtime upstream text frame, got %v", msgType)
			return
		}
		observations <- upstreamObservation{
			Path:             r.URL.Path,
			QueryModel:       r.URL.Query().Get("model"),
			Authorization:    r.Header.Get("Authorization"),
			SafetyIdentifier: r.Header.Get("OpenAI-Safety-Identifier"),
			Cookie:           r.Header.Get("Cookie"),
			LeakedHeader:     r.Header.Get("X-SRapi-Leak"),
			ClientFrame:      append([]byte(nil), payload...),
		}
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"session.created","session":{"id":"sess_api_key_realtime"}}`)); err != nil {
			t.Errorf("write api-key realtime upstream frame: %v", err)
			return
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp630-realtime-api-key-provider","display_name":"WP630 Realtime API Key Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"realtime_websocket":true,"streaming":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp630-realtime-api-key-model","display_name":"WP630 Realtime API Key Model","status":"active","capabilities":[{"key":"realtime_websocket","level":"required","status":"experimental","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-realtime-2","status":"active","capability_override":[{"key":"realtime_websocket","level":"required","status":"experimental","version":"v1"},{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp630-realtime-api-key-account","runtime_class":"api_key","credential":{"api_key":"official-realtime-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","capability_realtime_websocket":true},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	conn := mustDialRealtimeWebSocket(t, server.URL+"/v1/realtime?model=wp630-realtime-api-key-model", apiKey, http.Header{
		"OpenAI-Safety-Identifier": {"api-key-safe-user-hash"},
		"Cookie":                   {"caller-cookie=leaked"},
		"X-SRapi-Leak":             {"leaked"},
	})
	defer conn.Close(websocket.StatusNormalClosure, "")

	writeWebSocketJSON(t, conn, map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"type":         "realtime",
			"instructions": "api key path",
		},
	})
	event := readWebSocketEvent(t, conn)
	if event["type"] != "session.created" {
		t.Fatalf("expected session.created event, got %+v", event)
	}

	var obs upstreamObservation
	select {
	case obs = <-observations:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for api-key realtime upstream observation")
	}
	if obs.Path != "/v1/realtime" ||
		obs.QueryModel != "gpt-realtime-2" ||
		obs.Authorization != "Bearer official-realtime-secret" ||
		obs.SafetyIdentifier != "api-key-safe-user-hash" ||
		obs.Cookie != "" ||
		obs.LeakedHeader != "" {
		t.Fatalf("unexpected api-key realtime upstream request: %+v", obs)
	}
	if !strings.Contains(string(obs.ClientFrame), "api key path") {
		t.Fatalf("expected downstream realtime frame to reach upstream, got %s", obs.ClientFrame)
	}
	conn.Close(websocket.StatusNormalClosure, "")

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp630-realtime-api-key-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one scheduler decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SourceEndpoint != realtimeWebSocketSourceEndpoint || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(accountResp.Data.Id) {
		t.Fatalf("expected api-key realtime scheduler evidence, got %+v", decision)
	}

	usageResp := waitForRealtimeUsageLog(t, handler, sessionCookie, "wp630-realtime-api-key-model")
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].SourceEndpoint != realtimeWebSocketSourceEndpoint {
		t.Fatalf("unexpected api-key realtime usage record: %+v", usageResp.Data)
	}
}

func TestAdminOpsRealtimeSlotsListsActiveSlotsSafely(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	conn := mustDialResponsesWebSocket(t, server.URL+"/v1/responses/ws?session_affinity_key=secret-session&sticky_account_id=42&sticky_strength=hard", apiKey)
	defer conn.Close(websocket.StatusNormalClosure, "")

	var slotsResp apiopenapi.RealtimeActiveSlotListResponse
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/realtime/slots", nil)
		req.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected realtime slot list 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&slotsResp); err != nil {
			t.Fatalf("decode realtime slot list: %v", err)
		}
		if len(slotsResp.Data) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(slotsResp.Data) != 1 {
		t.Fatalf("expected one active realtime slot, got %+v", slotsResp)
	}
	slot := slotsResp.Data[0]
	if slot.Kind != apiopenapi.RealtimeSlotKind(realtimecontract.SlotKindResponsesWebSocket) ||
		slot.SourceEndpoint != apiopenapi.RealtimeActiveSlotSourceEndpoint(responsesWebSocketSourceEndpoint) ||
		slot.SessionAffinitySource != "query:sticky_account_id" ||
		slot.SessionAffinityKeyHash == "" ||
		strings.Contains(slot.SessionAffinityKeyHash, "secret-session") ||
		slot.StickyAccountId == nil ||
		*slot.StickyAccountId != "42" ||
		slot.StickyStrength != "hard" {
		t.Fatalf("unexpected sanitized realtime slot: %+v", slot)
	}
	if slotsResp.Counters.ActiveSlots != 1 ||
		slotsResp.Counters.AcquiredTotal != 1 ||
		slotsResp.Counters.ActiveByEndpoint[responsesWebSocketSourceEndpoint] != 1 ||
		slotsResp.Counters.ActiveByKind[string(realtimecontract.SlotKindResponsesWebSocket)] != 1 ||
		len(slotsResp.Counters.ActiveByApiKeyId) != 1 {
		t.Fatalf("unexpected realtime slot counters: %+v", slotsResp.Counters)
	}
	if strings.Contains(mustMarshalString(t, slotsResp), "secret-session") ||
		strings.Contains(mustMarshalString(t, slotsResp), apiKey) {
		t.Fatalf("realtime slot response leaked sensitive input: %+v", slotsResp)
	}
}

func TestGatewayResponsesWebSocketEnforcesRealtimeSlotLimit(t *testing.T) {
	cfg := config.Load()
	cfg.Gateway.RealtimeMaxOpenSlots = 1
	cfg.Gateway.RealtimeMaxOpenSlotsPerKey = 1
	handler := New(cfg, nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	server := httptest.NewServer(handler)
	defer server.Close()
	first := mustDialResponsesWebSocket(t, server.URL+"/v1/responses/ws", apiKey)
	defer first.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	second, resp, err := websocket.Dial(ctx, httpToWebSocketURL(server.URL+"/v1/responses/ws"), &websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		second.Close(websocket.StatusNormalClosure, "")
		t.Fatal("expected second websocket dial to be rejected")
	}
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected realtime slot limit 429, got resp=%v err=%v", resp, err)
	}

	first.Close(websocket.StatusNormalClosure, "")
	waitForMetricValue(t, handler, "srapi_realtime_active_slots 0")
	metrics := metricsBody(t, handler)
	if !strings.Contains(metrics, `srapi_realtime_slots_total{event="acquired"} 1`) ||
		!strings.Contains(metrics, `srapi_realtime_slots_total{event="released"} 1`) ||
		!strings.Contains(metrics, `srapi_realtime_slots_total{event="rejected"} 1`) {
		t.Fatalf("expected realtime slot lifecycle metrics, got:\n%s", metrics)
	}
}

func mustDialRealtimeWebSocket(t *testing.T, rawURL, apiKey string, extra http.Header) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	for key, values := range extra {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	conn, _, err := websocket.Dial(ctx, httpToWebSocketURL(rawURL), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial realtime websocket: %v", err)
	}
	return conn
}

func waitForRealtimeUsageLog(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, model string) apiopenapi.UsageLogListResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last apiopenapi.UsageLogListResponse
	for time.Now().Before(deadline) {
		usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model="+model, nil)
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
		if len(usageResp.Data) > 0 {
			return usageResp
		}
		last = usageResp
		time.Sleep(20 * time.Millisecond)
	}
	return last
}

func mustDialResponsesWebSocket(t *testing.T, rawURL, apiKey string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	conn, _, err := websocket.Dial(ctx, httpToWebSocketURL(rawURL), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial responses websocket: %v", err)
	}
	return conn
}

func writeWebSocketJSON(t *testing.T, conn *websocket.Conn, payload any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal websocket payload: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, encoded); err != nil {
		t.Fatalf("write websocket payload: %v", err)
	}
}

func readWebSocketEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket event: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("expected websocket text event, got %v", messageType)
	}
	var event map[string]any
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode websocket event %s: %v", payload, err)
	}
	return event
}

func httpToWebSocketURL(rawURL string) string {
	switch {
	case strings.HasPrefix(rawURL, "https://"):
		return "wss://" + strings.TrimPrefix(rawURL, "https://")
	case strings.HasPrefix(rawURL, "http://"):
		return "ws://" + strings.TrimPrefix(rawURL, "http://")
	default:
		return rawURL
	}
}

func mustMarshalString(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return strconv.Quote(string(encoded))
}

func metricsBody(t *testing.T, handler http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func waitForMetricValue(t *testing.T, handler http.Handler, expected string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(metricsBody(t, handler), expected) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for metric %q, got:\n%s", expected, metricsBody(t, handler))
}
