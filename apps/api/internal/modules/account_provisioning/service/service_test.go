package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/account_provisioning/contract"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func TestStartAuthorizationURLBuildsPKCERedirect(t *testing.T) {
	svc := New()
	res, err := svc.StartAuthorizationURL(contract.ProviderOAuthConfig{
		ClientID:     "client-abc",
		AuthorizeURL: "https://provider.example/authorize",
		TokenURL:     "https://provider.example/token",
		RedirectURI:  "http://localhost:8080/admin/accounts/oauth/callback",
		Scopes:       []string{"openid", "offline_access"},
		UsePKCE:      true,
	})
	if err != nil {
		t.Fatalf("start authorize url: %v", err)
	}
	if res.SessionID == "" || res.State == "" {
		t.Fatalf("expected session id and state, got %+v", res)
	}
	parsed, err := url.Parse(res.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := parsed.Query()
	if q.Get("response_type") != "code" || q.Get("client_id") != "client-abc" {
		t.Fatalf("unexpected authorize query: %s", res.AuthorizationURL)
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected PKCE challenge in %s", res.AuthorizationURL)
	}
	if q.Get("state") != res.State {
		t.Fatalf("state in url %q != %q", q.Get("state"), res.State)
	}
	if q.Get("scope") != "openid offline_access" {
		t.Fatalf("unexpected scope: %q", q.Get("scope"))
	}
}

func TestStartAuthorizationURLRejectsBadConfig(t *testing.T) {
	svc := New()
	cases := []contract.ProviderOAuthConfig{
		{ClientID: "", AuthorizeURL: "https://p.example/a", TokenURL: "https://p.example/t", RedirectURI: "https://app/cb"},
		{ClientID: "c", AuthorizeURL: "http://p.example/a", TokenURL: "https://p.example/t", RedirectURI: "https://app/cb"}, // http authorize not allowed
		{ClientID: "c", AuthorizeURL: "https://p.example/a", TokenURL: "", RedirectURI: "https://app/cb"},
		{ClientID: "c", AuthorizeURL: "https://p.example/a", TokenURL: "https://p.example/t", RedirectURI: "ftp://app/cb"},
	}
	for i, cfg := range cases {
		if _, err := svc.StartAuthorizationURL(cfg); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("case %d: expected ErrInvalidInput, got %v", i, err)
		}
	}
}

func TestExchangeCodeHappyPathMintsCredential(t *testing.T) {
	var gotForm url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"acc-1","refresh_token":"ref-1","token_type":"Bearer","expires_in":3600,"scope":"openid"}`)
	}))
	defer upstream.Close()

	svc := New(WithHTTPClient(upstream.Client()))
	start, err := svc.StartAuthorizationURL(contract.ProviderOAuthConfig{
		ClientID:     "client-abc",
		AuthorizeURL: "https://provider.example/authorize",
		TokenURL:     upstream.URL + "/token",
		RedirectURI:  "http://localhost:8080/cb",
		UsePKCE:      true,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	cred, err := svc.ExchangeCode(context.Background(), start.SessionID, "auth-code-1", start.State)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if cred.AccessToken != "acc-1" || cred.RefreshToken != "ref-1" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
	if gotForm.Get("grant_type") != "authorization_code" || gotForm.Get("code") != "auth-code-1" {
		t.Fatalf("unexpected token form: %v", gotForm)
	}
	if gotForm.Get("code_verifier") == "" {
		t.Fatalf("expected PKCE code_verifier in token form")
	}
	credMap := cred.Credential()
	if credMap["access_token"] != "acc-1" || credMap["refresh_token"] != "ref-1" {
		t.Fatalf("unexpected credential map: %v", credMap)
	}
	status, err := svc.Status(start.SessionID)
	if err != nil || status.Status != contract.StatusCompleted {
		t.Fatalf("expected completed status, got %v err=%v", status.Status, err)
	}
}

func TestExchangeCodeStateMismatchFails(t *testing.T) {
	svc := New()
	start, err := svc.StartAuthorizationURL(contract.ProviderOAuthConfig{
		ClientID:     "c",
		AuthorizeURL: "https://provider.example/authorize",
		TokenURL:     "https://provider.example/token",
		RedirectURI:  "https://app.example/cb",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := svc.ExchangeCode(context.Background(), start.SessionID, "code", "wrong-state"); !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("expected ErrStateMismatch, got %v", err)
	}
	status, _ := svc.Status(start.SessionID)
	if status.Status != contract.StatusFailed {
		t.Fatalf("expected failed status, got %v", status.Status)
	}
}

func TestExchangeCodeProviderDenyFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	defer upstream.Close()
	svc := New(WithHTTPClient(upstream.Client()))
	start, _ := svc.StartAuthorizationURL(contract.ProviderOAuthConfig{
		ClientID:     "c",
		AuthorizeURL: "https://provider.example/authorize",
		TokenURL:     upstream.URL + "/token",
		RedirectURI:  "https://app.example/cb",
	})
	if _, err := svc.ExchangeCode(context.Background(), start.SessionID, "code", start.State); !errors.Is(err, ErrProviderRejected) {
		t.Fatalf("expected ErrProviderRejected, got %v", err)
	}
}

func TestExchangeCodeExpiredSession(t *testing.T) {
	clk := &fixedClock{now: time.Now().UTC()}
	svc := New(WithClock(clk), WithSessionTTL(time.Minute))
	start, _ := svc.StartAuthorizationURL(contract.ProviderOAuthConfig{
		ClientID:     "c",
		AuthorizeURL: "https://provider.example/authorize",
		TokenURL:     "https://provider.example/token",
		RedirectURI:  "https://app.example/cb",
	})
	clk.now = clk.now.Add(2 * time.Minute)
	if _, err := svc.ExchangeCode(context.Background(), start.SessionID, "code", start.State); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestDeviceCodeStartAndPollSucceeds(t *testing.T) {
	var pollCount int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			_, _ = io.WriteString(w, `{"device_code":"dev-1","user_code":"WXYZ-1234","verification_uri":"https://provider.example/device","verification_uri_complete":"https://provider.example/device?code=WXYZ-1234","interval":1,"expires_in":600}`)
		case "/token":
			if r.PostForm.Get("device_code") != "dev-1" {
				t.Fatalf("unexpected device_code: %q", r.PostForm.Get("device_code"))
			}
			pollCount++
			if pollCount < 2 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"dev-acc","refresh_token":"dev-ref","token_type":"Bearer"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	svc := New(WithHTTPClient(upstream.Client()))
	start, err := svc.StartDeviceCode(context.Background(), contract.ProviderOAuthConfig{
		ClientID:           "c",
		DeviceAuthorizeURL: upstream.URL + "/device",
		TokenURL:           upstream.URL + "/token",
		Scopes:             []string{"offline_access"},
	})
	if err != nil {
		t.Fatalf("start device: %v", err)
	}
	if start.UserCode != "WXYZ-1234" || start.VerificationURI == "" || start.IntervalSecs != 1 {
		t.Fatalf("unexpected device start: %+v", start)
	}
	if !strings.Contains(start.SessionID, "") || start.SessionID == "" {
		t.Fatalf("expected session id")
	}
	if _, err := svc.PollDeviceCode(context.Background(), start.SessionID); !errors.Is(err, ErrAuthorizationPending) {
		t.Fatalf("expected ErrAuthorizationPending on first poll, got %v", err)
	}
	cred, err := svc.PollDeviceCode(context.Background(), start.SessionID)
	if err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if cred.AccessToken != "dev-acc" || cred.RefreshToken != "dev-ref" {
		t.Fatalf("unexpected device credential: %+v", cred)
	}
	status, _ := svc.Status(start.SessionID)
	if status.Status != contract.StatusCompleted {
		t.Fatalf("expected completed, got %v", status.Status)
	}
}

func TestDeviceCodePollWrongModeRejected(t *testing.T) {
	svc := New()
	start, _ := svc.StartAuthorizationURL(contract.ProviderOAuthConfig{
		ClientID:     "c",
		AuthorizeURL: "https://provider.example/authorize",
		TokenURL:     "https://provider.example/token",
		RedirectURI:  "https://app.example/cb",
	})
	if _, err := svc.PollDeviceCode(context.Background(), start.SessionID); !errors.Is(err, ErrWrongMode) {
		t.Fatalf("expected ErrWrongMode, got %v", err)
	}
}

func TestStatusUnknownSession(t *testing.T) {
	svc := New()
	if _, err := svc.Status("nope"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestProvisioningUserAgent(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
	}{
		{"https://auth.openai.com/oauth/token", "codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color"},
		{"https://chatgpt.com/backend-api/oauth/token", "codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color"},
		{"https://console.anthropic.com/v1/oauth/token", "axios/1.13.6"},
		{"https://claude.ai/oauth/token", "axios/1.13.6"},
		{"https://example.com/token", "axios/1.13.6"},
		{"not a url", "axios/1.13.6"},
	}
	for _, tc := range cases {
		if got := provisioningUserAgent(tc.endpoint); got != tc.want {
			t.Fatalf("provisioningUserAgent(%q) = %q, want %q", tc.endpoint, got, tc.want)
		}
	}
}
