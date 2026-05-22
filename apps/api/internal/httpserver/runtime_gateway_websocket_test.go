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
