package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
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
