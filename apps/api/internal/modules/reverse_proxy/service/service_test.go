package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestRuntimeDoesNotInjectAPIKeyRuntimeCredentials(t *testing.T) {
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
	if gotHeader.Get("Authorization") != "" {
		t.Fatalf("expected no api key auth injection, got %q", gotHeader.Get("Authorization"))
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

func TestRuntimeAppliesMetadataEgressProfileHeadersAndUserAgent(t *testing.T) {
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
			AccountID:    12,
			RuntimeClass: "oauth_refresh",
			Metadata: map[string]any{
				"egress_profile": map[string]any{
					"user_agent":           "ProfileClient/2.0",
					"accept_language":      "en-US,en;q=0.9",
					"forbidden_headers":    []any{"X-Caller-Leak"},
					"extra_static_headers": map[string]any{"X-Egress-Client": "profile-1"},
				},
			},
			Credential: map[string]any{"access_token": "profile-token"},
		},
		Method: http.MethodPost,
		URL:    upstream.URL,
		Headers: http.Header{
			"Authorization":  {"Bearer leaked"},
			"Cookie":         {"session=leaked"},
			"User-Agent":     {"SRapi/test"},
			"X-Caller-Leak":  {"leaked"},
			"X-Egress-Input": {"kept"},
		},
		Body: []byte(`{"model":"profile-model"}`),
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if gotHeader.Get("Authorization") != "Bearer profile-token" || gotHeader.Get("Cookie") != "" {
		t.Fatalf("expected runtime auth only, got %+v", gotHeader)
	}
	if gotHeader.Get("User-Agent") != "ProfileClient/2.0" {
		t.Fatalf("expected profile user agent, got %q", gotHeader.Get("User-Agent"))
	}
	if gotHeader.Get("Accept-Language") != "en-US,en;q=0.9" || gotHeader.Get("X-Egress-Client") != "profile-1" {
		t.Fatalf("expected egress profile headers, got %+v", gotHeader)
	}
	if gotHeader.Get("X-Caller-Leak") != "" || gotHeader.Get("X-Egress-Input") != "kept" {
		t.Fatalf("expected configured caller header filtering, got %+v", gotHeader)
	}
}

func TestRuntimeRejectsUnsafeEgressProfileStaticHeader(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{
			AccountID:    13,
			RuntimeClass: "oauth_refresh",
			Metadata: map[string]any{
				"egress_profile": map[string]any{
					"extra_static_headers": map[string]any{"Authorization": "Bearer static"},
				},
			},
			Credential: map[string]any{"access_token": "runtime-token"},
		},
		Method: http.MethodGet,
		URL:    upstream.URL,
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Class != "unsupported_egress_profile" {
		t.Fatalf("expected unsupported egress profile error, got %T %v", err, err)
	}
	if upstreamCalled {
		t.Fatal("unsafe egress profile should not call upstream")
	}
	if metrics := svc.Metrics(); metrics.RequestErrorTotal["unsupported_egress_profile"] != 1 {
		t.Fatalf("expected unsupported egress profile metric, got %+v", metrics)
	}
}

func TestRuntimeRejectsUnsupportedEgressProfileFeature(t *testing.T) {
	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Do(context.Background(), contract.Request{
		Account: contract.AccountRuntime{
			AccountID:    14,
			RuntimeClass: "oauth_refresh",
			Metadata: map[string]any{
				"egress_profile": map[string]any{"tls_template": "openai_python_sdk"},
			},
			Credential: map[string]any{"access_token": "runtime-token"},
		},
		Method: http.MethodGet,
		URL:    "https://upstream.example.test/v1/models",
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Class != "unsupported_egress_profile" {
		t.Fatalf("expected unsupported egress profile error, got %T %v", err, err)
	}
}

func TestRuntimeBuildsUTLSTransportForSupportedEgressProfile(t *testing.T) {
	client, err := newIsolatedClient(contract.AccountRuntime{
		AccountID:    15,
		RuntimeClass: "oauth_refresh",
		Metadata: map[string]any{
			"egress_profile": map[string]any{"tls_template": "chrome_120"},
		},
	})
	if err != nil {
		t.Fatalf("create isolated client: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected http transport, got %T", client.Transport)
	}
	if transport.DialTLSContext == nil {
		t.Fatal("expected custom uTLS DialTLSContext")
	}
	if transport.TLSNextProto == nil || transport.ForceAttemptHTTP2 {
		t.Fatalf("expected HTTP/2 disabled for uTLS transport")
	}
	if transport.Proxy != nil {
		t.Fatal("expected direct uTLS transport to avoid standard proxy TLS fallback")
	}
	key, err := clientCacheKey(contract.AccountRuntime{
		AccountID: 15,
		Metadata: map[string]any{"egress_profile": map[string]any{
			"tls_template":        "chrome_120",
			"http_version_policy": "require_h1",
		}},
	})
	if err != nil {
		t.Fatalf("build client cache key: %v", err)
	}
	if !strings.Contains(key, "tls=chrome_120") || !strings.Contains(key, "http=require_h1") {
		t.Fatalf("expected transport settings in client cache key, got %q", key)
	}
}

func TestRuntimeRejectsTLSEgressProfileWithProxy(t *testing.T) {
	_, err := newIsolatedClient(contract.AccountRuntime{
		AccountID:    16,
		RuntimeClass: "oauth_refresh",
		ProxyID:      ptrString("socks5://127.0.0.1:1080"),
		Metadata: map[string]any{
			"egress_profile": map[string]any{"tls_template": "chrome_120"},
		},
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Class != "unsupported_egress_profile" {
		t.Fatalf("expected unsupported egress profile error, got %T %v", err, err)
	}
}

func TestRuntimeSupportsTLSEgressProfileThroughHTTPConnectProxy(t *testing.T) {
	var (
		upstreamHeader http.Header
		connectHost    string
		proxyAuth      string
	)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeader = r.Header.Clone()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT, got %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		connectHost = r.Host
		proxyAuth = r.Header.Get("Proxy-Authorization")
		target, err := net.Dial("tcp", r.Host)
		if err != nil {
			t.Errorf("dial target: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer target.Close()
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("proxy response writer does not support hijack")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack client: %v", err)
			return
		}
		defer clientConn.Close()
		if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			t.Errorf("write CONNECT response: %v", err)
			return
		}
		done := make(chan struct{}, 2)
		go func() {
			_, _ = io.Copy(target, clientConn)
			done <- struct{}{}
		}()
		go func() {
			_, _ = io.Copy(clientConn, target)
			done <- struct{}{}
		}()
		<-done
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	proxyURL.User = url.UserPassword("proxy-user", "proxy-pass")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: upstream.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs}
	if err := configureTransportForEgress(transport, contract.AccountRuntime{
		AccountID: 17,
		ProxyID:   ptrString(proxyURL.String()),
		Metadata:  map[string]any{"egress_profile": map[string]any{"tls_template": "chrome_120"}},
	}, egressProfile{TLSTemplate: "chrome_120"}); err != nil {
		t.Fatalf("configure egress transport: %v", err)
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "ProfileProxyTest/1.0")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("proxy uTLS request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected upstream 200, got %d", resp.StatusCode)
	}
	if connectHost != upstreamURL.Host {
		t.Fatalf("expected CONNECT to %s, got %q", upstreamURL.Host, connectHost)
	}
	if proxyAuth != "Basic cHJveHktdXNlcjpwcm94eS1wYXNz" {
		t.Fatalf("expected proxy auth header, got %q", proxyAuth)
	}
	if upstreamHeader.Get("User-Agent") != "ProfileProxyTest/1.0" {
		t.Fatalf("expected upstream request through proxy, got %+v", upstreamHeader)
	}
}

func TestRuntimeRejectsTLSEgressProfileWithHTTPSProxy(t *testing.T) {
	_, err := newIsolatedClient(contract.AccountRuntime{
		AccountID:    18,
		RuntimeClass: "oauth_refresh",
		ProxyID:      ptrString("https://127.0.0.1:8443"),
		Metadata: map[string]any{
			"egress_profile": map[string]any{"tls_template": "chrome_120"},
		},
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Class != "unsupported_egress_profile" {
		t.Fatalf("expected unsupported egress profile error, got %T %v", err, err)
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
	var gotBody string
	var gotUserAgent string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected token endpoint path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected token method %s", r.Method)
		}
		gotUserAgent = r.Header.Get("User-Agent")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access-token","refresh_token":"rotated-refresh-token","id_token":"id-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

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
		Account: contract.AccountRuntime{
			AccountID:      1,
			RuntimeClass:   "oauth_refresh",
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"oauth_token_url": tokenServer.URL + "/oauth/token", "user_agent": "codex-cli/test"},
			Credential:     map[string]any{"access_token": "old-token", "refresh_token": "refresh-token"},
		},
	})
	if err != nil {
		t.Fatalf("refresh with token: %v", err)
	}
	if resp.Credential["access_token"] != "new-access-token" ||
		resp.Credential["refresh_token"] != "rotated-refresh-token" ||
		resp.Credential["id_token"] != "id-token" ||
		resp.Credential["token_type"] != "Bearer" ||
		resp.Credential["expires_at"] == "" ||
		resp.RefreshedAt == "" {
		t.Fatalf("unexpected refresh response: %+v", resp)
	}
	if !strings.Contains(gotBody, "grant_type=refresh_token") ||
		!strings.Contains(gotBody, "refresh_token=refresh-token") ||
		!strings.Contains(gotBody, "client_id="+url.QueryEscape(codexOAuthClientID)) {
		t.Fatalf("unexpected refresh body %q", gotBody)
	}
	if gotUserAgent != "codex-cli/test" {
		t.Fatalf("expected account user agent, got %q", gotUserAgent)
	}
	metrics := svc.Metrics()
	if metrics.OAuthRefreshTotal["credential_missing"] != 1 || metrics.OAuthRefreshTotal["success"] != 1 {
		t.Fatalf("unexpected refresh metrics: %+v", metrics)
	}
}

func TestClaudeRefreshUsesJSONTokenRequest(t *testing.T) {
	var gotUserAgent string
	var gotPayload map[string]string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" {
			t.Fatalf("unexpected Claude token endpoint path %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("unexpected Claude token headers: %+v", r.Header)
		}
		gotUserAgent = r.Header.Get("User-Agent")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode Claude refresh body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"claude-access","refresh_token":"claude-refresh-rotated","token_type":"Bearer","expires_in":3600,"account":{"email_address":"claude@example.test"}}`))
	}))
	defer tokenServer.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	resp, err := svc.Refresh(context.Background(), contract.RefreshRequest{
		Account: contract.AccountRuntime{
			AccountID:      3,
			RuntimeClass:   "oauth_refresh",
			UpstreamClient: ptrString("claude_code_cli"),
			Metadata:       map[string]any{"oauth_token_url": tokenServer.URL + "/v1/oauth/token", "user_agent": "claude-cli/test"},
			Credential:     map[string]any{"refresh_token": "claude-refresh"},
		},
	})
	if err != nil {
		t.Fatalf("refresh Claude token: %v", err)
	}
	if resp.Credential["access_token"] != "claude-access" || resp.Credential["refresh_token"] != "claude-refresh-rotated" {
		t.Fatalf("unexpected Claude refresh response: %+v", resp)
	}
	if gotPayload["grant_type"] != "refresh_token" ||
		gotPayload["refresh_token"] != "claude-refresh" ||
		gotPayload["client_id"] != claudeCodeOAuthClientID {
		t.Fatalf("unexpected Claude refresh payload: %+v", gotPayload)
	}
	if gotUserAgent != "claude-cli/test" {
		t.Fatalf("expected Claude user agent, got %q", gotUserAgent)
	}
}

func TestAntigravityRefreshUsesClientSecretFormTokenRequest(t *testing.T) {
	var gotForm url.Values
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("unexpected Antigravity token headers: %+v", r.Header)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse Antigravity refresh form: %v", err)
		}
		gotForm = cloneURLValues(r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"antigravity-access","refresh_token":"antigravity-refresh-rotated","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	resp, err := svc.Refresh(context.Background(), contract.RefreshRequest{
		Account: contract.AccountRuntime{
			AccountID:      4,
			RuntimeClass:   "oauth_refresh",
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"oauth_token_url": tokenServer.URL},
			Credential:     map[string]any{"refresh_token": "antigravity-refresh", "oauth_client_secret": "antigravity-secret"},
		},
	})
	if err != nil {
		t.Fatalf("refresh Antigravity token: %v", err)
	}
	if resp.Credential["access_token"] != "antigravity-access" || resp.Credential["refresh_token"] != "antigravity-refresh-rotated" {
		t.Fatalf("unexpected Antigravity refresh response: %+v", resp)
	}
	if gotForm.Get("grant_type") != "refresh_token" ||
		gotForm.Get("refresh_token") != "antigravity-refresh" ||
		gotForm.Get("client_id") != antigravityOAuthClientID ||
		gotForm.Get("client_secret") != "antigravity-secret" {
		t.Fatalf("unexpected Antigravity refresh form: %v", gotForm)
	}
}

func TestAntigravityRefreshRequiresClientSecret(t *testing.T) {
	upstreamCalled := false
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tokenServer.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Refresh(context.Background(), contract.RefreshRequest{
		Account: contract.AccountRuntime{
			AccountID:      5,
			RuntimeClass:   "oauth_refresh",
			UpstreamClient: ptrString("antigravity_desktop"),
			Metadata:       map[string]any{"oauth_token_url": tokenServer.URL, "oauth_client_secret": "metadata-secret"},
			Credential:     map[string]any{"refresh_token": "antigravity-refresh"},
		},
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) || runtimeErr.Class != "credential_missing" {
		t.Fatalf("expected credential_missing runtime error, got %T %v", err, err)
	}
	if upstreamCalled {
		t.Fatal("missing Antigravity client secret should not call upstream")
	}
}

func TestRuntimeRefreshClassifiesInvalidGrantWithoutOverwritingCredential(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh_token_reused"}`))
	}))
	defer tokenServer.Close()

	svc, err := New(nil)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	_, err = svc.Refresh(context.Background(), contract.RefreshRequest{
		Account: contract.AccountRuntime{
			AccountID:      2,
			RuntimeClass:   "oauth_refresh",
			UpstreamClient: ptrString("codex_cli"),
			Metadata:       map[string]any{"oauth_token_url": tokenServer.URL},
			Credential:     map[string]any{"access_token": "old-token", "refresh_token": "bad-refresh-token"},
		},
	})
	var runtimeErr contract.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("expected runtime error, got %T %v", err, err)
	}
	if runtimeErr.Class != "session_invalid" {
		t.Fatalf("expected session_invalid, got %+v", runtimeErr)
	}
	metrics := svc.Metrics()
	if metrics.OAuthRefreshTotal["session_invalid"] != 1 {
		t.Fatalf("unexpected refresh metrics: %+v", metrics)
	}
}

func ptrString(value string) *string {
	return &value
}

func cloneURLValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, vals := range values {
		out[key] = append([]string(nil), vals...)
	}
	return out
}
