package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	"nhooyr.io/websocket"
)

func TestRuntimeSanitizesHeadersAndInjectsAccountContext(t *testing.T) {
	var gotHeader http.Header
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{
			AccountID:      1,
			RuntimeClass:   "oauth_refresh",
			UpstreamClient: ptrString("codex_cli"),
			Credential:     map[string]any{"access_token": "access-token", "user_agent": "Codex/1.0"},
		},
		Method: http.MethodPost,
		URL:    upstream.URL,
		Headers: http.Header{
			"X-Request-ID":    {"req_leak"},
			"X-Forwarded-For": {"203.0.113.1"},
			"Via":             {"SRapi"},
			"X-SRapi-Test":    {"leak"},
			"X-Gateway-Test":  {"leak"},
			"User-Agent":      {"SRapi/test"},
		},
		Body: []byte(`{"model":"upstream-model","messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	for _, key := range []string{"X-Request-ID", "X-Forwarded-For", "Via", "X-SRapi-Test", "X-Gateway-Test"} {
		if gotHeader.Get(key) != "" {
			t.Fatalf("expected %s to be sanitized, got headers %+v", key, gotHeader)
		}
	}
	if gotHeader.Get("Authorization") != "Bearer access-token" {
		t.Fatalf("expected injected bearer token, got %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("User-Agent") != "Codex/1.0" {
		t.Fatalf("expected runtime user agent, got %q", gotHeader.Get("User-Agent"))
	}
	if strings.Contains(gotBody, "request_id") || !strings.Contains(gotBody, "upstream-model") {
		t.Fatalf("unexpected upstream body %q", gotBody)
	}
}

func TestRuntimeInjectsCliClientTokenAndDefaultClientUserAgent(t *testing.T) {
	var gotHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{
			AccountID:      7,
			RuntimeClass:   "cli_client_token",
			UpstreamClient: ptrString("claude_code_cli"),
			Credential: map[string]any{
				"access_token":     "wrong-token",
				"cli_client_token": "cli-token",
			},
		},
		Method: http.MethodPost,
		URL:    upstream.URL,
		Body:   []byte(`{"model":"cli-model"}`),
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if gotHeader.Get("Authorization") != "Bearer cli-token" {
		t.Fatalf("expected cli client token auth, got %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("User-Agent") != "Claude-Code/1.0" {
		t.Fatalf("expected claude code user agent, got %q", gotHeader.Get("User-Agent"))
	}
}

func TestRuntimeInjectsAPIKeyRuntimeBearerToken(t *testing.T) {
	var gotHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{
			AccountID:      8,
			RuntimeClass:   "api_key",
			UpstreamClient: ptrString("codex_cli"),
			Credential:     map[string]any{"api_key": "api-key-token"},
		},
		Method: http.MethodPost,
		URL:    upstream.URL,
		Headers: http.Header{
			"Authorization": {"Bearer leaked"},
		},
		Body: []byte(`{"model":"codex-model"}`),
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if gotHeader.Get("Authorization") != "Bearer api-key-token" {
		t.Fatalf("expected api key bearer auth, got %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("User-Agent") != "Codex/1.0" {
		t.Fatalf("expected codex user agent, got %q", gotHeader.Get("User-Agent"))
	}
}

func TestRuntimeInjectsAntigravityDesktopTokenAndDefaultUserAgent(t *testing.T) {
	var gotHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{
			AccountID:      11,
			RuntimeClass:   "desktop_client_token",
			UpstreamClient: ptrString("antigravity_desktop"),
			Credential: map[string]any{
				"access_token": "desktop-token",
			},
		},
		Method: http.MethodPost,
		URL:    upstream.URL,
		Body:   []byte(`{"model":"antigravity-model"}`),
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if gotHeader.Get("Authorization") != "Bearer desktop-token" {
		t.Fatalf("expected desktop bearer token auth, got %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("User-Agent") != "Antigravity/1.0" {
		t.Fatalf("expected antigravity user agent, got %q", gotHeader.Get("User-Agent"))
	}
}

func TestRuntimeRelaysWebSocketWithAccountContextAndHeaderHygiene(t *testing.T) {
	var (
		mu         sync.Mutex
		gotHeader  http.Header
		gotPayload []byte
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHeader = r.Header.Clone()
		mu.Unlock()
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:    []string{"srapi.realtime.v1"},
			CompressionMode: websocket.CompressionDisabled,
		})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		msgType, payload, err := conn.Read(r.Context())
		if err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		if msgType != websocket.MessageText {
			t.Errorf("expected text websocket payload, got %v", msgType)
			return
		}
		mu.Lock()
		gotPayload = append([]byte(nil), payload...)
		mu.Unlock()
		if err := conn.Write(r.Context(), websocket.MessageText, append([]byte("echo:"), payload...)); err != nil {
			t.Errorf("write websocket text response: %v", err)
			return
		}
		if err := conn.Write(r.Context(), websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
			t.Errorf("write websocket binary response: %v", err)
			return
		}
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	clientToUpstream := make(chan contract.WebSocketMessage, 1)
	upstreamToClient := make(chan contract.WebSocketMessage, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	resultCh := make(chan webSocketRelayTestResult, 1)
	go func() {
		result, err := svc.RelayWebSocket(ctx, contract.WebSocketRelayRequest{
			Account: contract.AccountRuntime{
				AccountID:      21,
				RuntimeClass:   "cli_client_token",
				UpstreamClient: ptrString("codex_cli"),
				Credential:     map[string]any{"cli_client_token": "cli-token"},
			},
			URL: httpToWebSocketURL(upstream.URL),
			Headers: http.Header{
				"Authorization":          {"Bearer leaked"},
				"Cookie":                 {"leaked=session"},
				"Sec-Websocket-Protocol": {"leaked-protocol"},
				"X-Forwarded-For":        {"203.0.113.10"},
				"X-SRapi-Test":           {"leak"},
				"User-Agent":             {"SRapi/test"},
			},
			Subprotocols:     []string{"srapi.realtime.v1"},
			ClientToUpstream: clientToUpstream,
			UpstreamToClient: upstreamToClient,
		})
		resultCh <- webSocketRelayTestResult{result: result, err: err}
	}()
	clientToUpstream <- contract.WebSocketMessage{Type: contract.WebSocketMessageText, Data: []byte("hello websocket")}
	close(clientToUpstream)

	first := readRelayMessage(t, upstreamToClient)
	second := readRelayMessage(t, upstreamToClient)
	result := readRelayResult(t, resultCh)
	if result.err != nil {
		t.Fatalf("relay websocket: %v", result.err)
	}

	if first.Type != contract.WebSocketMessageText || string(first.Data) != "echo:hello websocket" {
		t.Fatalf("unexpected first websocket relay message: %+v", first)
	}
	if second.Type != contract.WebSocketMessageBinary || !bytes.Equal(second.Data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected second websocket relay message: %+v", second)
	}
	if result.result.UpstreamStatusCode != http.StatusSwitchingProtocols ||
		result.result.Subprotocol != "srapi.realtime.v1" ||
		result.result.MessagesUpstream != 1 ||
		result.result.MessagesDownstream != 2 ||
		result.result.BytesUpstream != len("hello websocket") ||
		result.result.BytesDownstream != len("echo:hello websocket")+3 {
		t.Fatalf("unexpected websocket relay result: %+v", result.result)
	}
	metrics := svc.Metrics()
	if metrics.RequestTotal != 1 || metrics.RequestSuccessTotal != 1 {
		t.Fatalf("expected websocket relay success metrics, got %+v", metrics)
	}

	mu.Lock()
	headers := gotHeader.Clone()
	payload := append([]byte(nil), gotPayload...)
	mu.Unlock()
	if string(payload) != "hello websocket" {
		t.Fatalf("expected upstream websocket payload, got %q", payload)
	}
	if headers.Get("Authorization") != "Bearer cli-token" || headers.Get("Cookie") != "" || headers.Get("User-Agent") != "Codex/1.0" {
		t.Fatalf("unexpected websocket account headers: %+v", headers)
	}
	if headers.Get("X-Forwarded-For") != "" || headers.Get("X-SRapi-Test") != "" || strings.Contains(headers.Get("Sec-Websocket-Protocol"), "leaked-protocol") {
		t.Fatalf("expected websocket relay headers to be sanitized, got %+v", headers)
	}
}

func TestRuntimeRelaysWebSocketWebSessionCookieFromCredential(t *testing.T) {
	var gotHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		if _, _, err := conn.Read(r.Context()); err != nil {
			t.Errorf("read websocket payload: %v", err)
			return
		}
		if err := conn.Write(r.Context(), websocket.MessageText, []byte("ok")); err != nil {
			t.Errorf("write websocket response: %v", err)
		}
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	clientToUpstream := make(chan contract.WebSocketMessage, 1)
	upstreamToClient := make(chan contract.WebSocketMessage, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	resultCh := make(chan webSocketRelayTestResult, 1)
	go func() {
		result, err := svc.RelayWebSocket(ctx, contract.WebSocketRelayRequest{
			Account: contract.AccountRuntime{
				AccountID:    22,
				RuntimeClass: "web_session_cookie",
				Credential:   map[string]any{"cookie": "session=credential-cookie"},
			},
			URL:              httpToWebSocketURL(upstream.URL),
			Headers:          http.Header{"Authorization": {"Bearer leaked"}, "Cookie": {"session=leaked"}},
			ClientToUpstream: clientToUpstream,
			UpstreamToClient: upstreamToClient,
		})
		resultCh <- webSocketRelayTestResult{result: result, err: err}
	}()
	clientToUpstream <- contract.WebSocketMessage{Type: contract.WebSocketMessageText, Data: []byte("hello")}
	close(clientToUpstream)
	if msg := readRelayMessage(t, upstreamToClient); msg.Type != contract.WebSocketMessageText || string(msg.Data) != "ok" {
		t.Fatalf("unexpected websocket response: %+v", msg)
	}
	if result := readRelayResult(t, resultCh); result.err != nil {
		t.Fatalf("relay websocket: %v", result.err)
	}
	if gotHeader.Get("Authorization") != "" || gotHeader.Get("Cookie") != "session=credential-cookie" {
		t.Fatalf("expected credential cookie and no leaked auth, got %+v", gotHeader)
	}
}

func TestRuntimeRejectsSRapiInternalBodyFields(t *testing.T) {
	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{AccountID: 1, RuntimeClass: "custom_reverse_proxy", Credential: map[string]any{"access_token": "token"}},
		Method:  http.MethodPost,
		URL:     "http://127.0.0.1/",
		Body:    []byte(`{"metadata":{"srapi_trace":"must-not-leak"}}`),
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("expected RuntimeError, got %T", err)
	}
	if runtimeErr.Class != "invalid_request" {
		t.Fatalf("expected invalid_request, got %+v", runtimeErr)
	}
}

type webSocketRelayTestResult struct {
	result contract.WebSocketRelayResult
	err    error
}

func readRelayMessage(t *testing.T, ch <-chan contract.WebSocketMessage) contract.WebSocketMessage {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("websocket relay channel closed before message")
		}
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for websocket relay message")
		return contract.WebSocketMessage{}
	}
}

func readRelayResult(t *testing.T, ch <-chan webSocketRelayTestResult) webSocketRelayTestResult {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for websocket relay result")
		return webSocketRelayTestResult{}
	}
}

func httpToWebSocketURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") {
		return "wss://" + strings.TrimPrefix(rawURL, "https://")
	}
	if strings.HasPrefix(rawURL, "http://") {
		return "ws://" + strings.TrimPrefix(rawURL, "http://")
	}
	return rawURL
}

func TestRuntimeKeepsPerAccountCookieJarsIsolated(t *testing.T) {
	var mu sync.Mutex
	cookiesByAccount := map[string]string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get("account")
		mu.Lock()
		cookiesByAccount[accountID] = r.Header.Get("Cookie")
		mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "session", Value: accountID})
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	for _, accountID := range []int{1, 2, 1, 2} {
		_, err := svc.Do(context.Background(), contract.Request{
			Account: contract.AccountRuntime{AccountID: accountID, RuntimeClass: "web_session_cookie", Credential: map[string]any{}},
			Method:  http.MethodGet,
			URL:     upstream.URL + "?account=" + strconv.Itoa(accountID),
		})
		if err != nil {
			t.Fatalf("runtime request for account %d: %v", accountID, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if cookiesByAccount["1"] != "session=1" {
		t.Fatalf("expected account 1 cookie jar only, got %q", cookiesByAccount["1"])
	}
	if cookiesByAccount["2"] != "session=2" {
		t.Fatalf("expected account 2 cookie jar only, got %q", cookiesByAccount["2"])
	}
}

func TestRuntimeClassifiesReverseProxyErrorsAndMetrics(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"account_locked","message":"locked"}}`))
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{AccountID: 1, RuntimeClass: "custom_reverse_proxy", Credential: map[string]any{"access_token": "token"}},
		Method:  http.MethodGet,
		URL:     upstream.URL,
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("expected RuntimeError, got %T", err)
	}
	if runtimeErr.Class != "account_locked" {
		t.Fatalf("expected account_locked, got %+v", runtimeErr)
	}
	metrics := svc.Metrics()
	if metrics.RequestTotal != 1 || metrics.AccountLockedTotal != 1 || metrics.RequestErrorTotal["account_locked"] != 1 {
		t.Fatalf("unexpected runtime metrics: %+v", metrics)
	}
}

func TestRuntimeRefreshUsesPerAccountLockAndDoesNotOverwriteOnFailure(t *testing.T) {
	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Refresh(context.Background(), contract.RefreshRequest{
		Account: contract.AccountRuntime{AccountID: 1, RuntimeClass: "oauth_refresh", Credential: map[string]any{}},
	})
	if err == nil {
		t.Fatal("expected missing refresh token error")
	}
	resp, err := svc.Refresh(context.Background(), contract.RefreshRequest{
		Account: contract.AccountRuntime{AccountID: 1, RuntimeClass: "oauth_refresh", Credential: map[string]any{"access_token": "old-token", "refresh_token": "refresh-token"}},
	})
	if err != nil {
		t.Fatalf("refresh with token: %v", err)
	}
	if resp.Credential["access_token"] != "refresh-token" || resp.RefreshedAt == "" {
		t.Fatalf("unexpected refresh response: %+v", resp)
	}
	metrics := svc.Metrics()
	if metrics.OAuthRefreshTotal["credential_missing"] != 1 || metrics.OAuthRefreshTotal["success"] != 1 {
		t.Fatalf("unexpected refresh metrics: %+v", metrics)
	}
}

func ptrString(value string) *string {
	return &value
}
