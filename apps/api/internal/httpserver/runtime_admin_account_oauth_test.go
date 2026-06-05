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

func adminAccountOAuthDo(t *testing.T, handler http.Handler, method, path, body string, sessionCookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAdminAccountOAuthAuthorizeURLShape(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	body := `{"config":{"client_id":"client-xyz","authorize_url":"https://provider.example/authorize","token_url":"https://provider.example/token","redirect_uri":"http://localhost:8080/admin/accounts/oauth/callback","scopes":["openid","offline_access"],"use_pkce":true}}`
	rec := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/authorize-url", body, sessionCookie, loginResp.Data.CsrfToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AccountOAuthAuthorizeUrlResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.SessionId == "" || resp.Data.State == "" {
		t.Fatalf("expected session id and state, got %+v", resp.Data)
	}
	parsed, err := url.Parse(resp.Data.AuthorizationUrl)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "client-xyz" || q.Get("response_type") != "code" {
		t.Fatalf("unexpected authorize url: %s", resp.Data.AuthorizationUrl)
	}
	if q.Get("code_challenge") == "" || q.Get("state") != resp.Data.State {
		t.Fatalf("expected PKCE challenge and matching state: %s", resp.Data.AuthorizationUrl)
	}

	// Pending status reads back as pending.
	statusRec := adminAccountOAuthDo(t, handler, http.MethodGet, "/api/v1/admin/accounts/oauth/pending/"+resp.Data.SessionId, "", sessionCookie, "")
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var statusResp apiopenapi.AccountOAuthPendingResponse
	if err := json.NewDecoder(statusRec.Body).Decode(&statusResp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if statusResp.Data.Status != apiopenapi.AccountOAuthPendingStatusPending {
		t.Fatalf("expected pending, got %s", statusResp.Data.Status)
	}
}

func TestAdminAccountOAuthExchangeHappyPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"acc-9","refresh_token":"ref-9","token_type":"Bearer","expires_in":3600}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	startBody := `{"config":{"client_id":"c","authorize_url":"https://provider.example/authorize","token_url":"` + upstream.URL + `/token","redirect_uri":"http://localhost:8080/cb"}}`
	startRec := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/authorize-url", startBody, sessionCookie, loginResp.Data.CsrfToken)
	if startRec.Code != http.StatusCreated {
		t.Fatalf("expected start 201, got %d body=%s", startRec.Code, startRec.Body.String())
	}
	var startResp apiopenapi.AccountOAuthAuthorizeUrlResponse
	if err := json.NewDecoder(startRec.Body).Decode(&startResp); err != nil {
		t.Fatalf("decode start: %v", err)
	}

	exchangeBody := `{"session_id":"` + startResp.Data.SessionId + `","code":"auth-code-x","state":"` + startResp.Data.State + `"}`
	exchangeRec := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/exchange", exchangeBody, sessionCookie, loginResp.Data.CsrfToken)
	if exchangeRec.Code != http.StatusOK {
		t.Fatalf("expected exchange 200, got %d body=%s", exchangeRec.Code, exchangeRec.Body.String())
	}
	var credResp apiopenapi.AccountOAuthCredentialResponse
	if err := json.NewDecoder(exchangeRec.Body).Decode(&credResp); err != nil {
		t.Fatalf("decode credential: %v", err)
	}
	if credResp.Data.Credential == nil {
		t.Fatalf("expected credential map")
	}
	cred := credResp.Data.Credential
	if cred["access_token"] != "acc-9" || cred["refresh_token"] != "ref-9" {
		t.Fatalf("unexpected credential: %v", cred)
	}
	if credResp.Data.HasRefreshToken == nil || !*credResp.Data.HasRefreshToken {
		t.Fatalf("expected has_refresh_token true")
	}
}

func TestAdminAccountOAuthExchangeProviderDeny(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	startBody := `{"config":{"client_id":"c","authorize_url":"https://provider.example/authorize","token_url":"` + upstream.URL + `/token","redirect_uri":"http://localhost:8080/cb"}}`
	startRec := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/authorize-url", startBody, sessionCookie, loginResp.Data.CsrfToken)
	var startResp apiopenapi.AccountOAuthAuthorizeUrlResponse
	_ = json.NewDecoder(startRec.Body).Decode(&startResp)

	exchangeBody := `{"session_id":"` + startResp.Data.SessionId + `","code":"code","state":"` + startResp.Data.State + `"}`
	exchangeRec := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/exchange", exchangeBody, sessionCookie, loginResp.Data.CsrfToken)
	if exchangeRec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on provider deny, got %d body=%s", exchangeRec.Code, exchangeRec.Body.String())
	}
}

func TestAdminAccountOAuthDeviceCodeStartAndPoll(t *testing.T) {
	var pollCount int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			_, _ = io.WriteString(w, `{"device_code":"dev-9","user_code":"ABCD-9","verification_uri":"https://provider.example/device","interval":1,"expires_in":600}`)
		case "/token":
			pollCount++
			if pollCount < 2 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"dev-acc-9","refresh_token":"dev-ref-9","token_type":"Bearer"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	startBody := `{"config":{"client_id":"c","device_authorize_url":"` + upstream.URL + `/device","token_url":"` + upstream.URL + `/token","scopes":["offline_access"]}}`
	startRec := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/device-code/start", startBody, sessionCookie, loginResp.Data.CsrfToken)
	if startRec.Code != http.StatusCreated {
		t.Fatalf("expected device start 201, got %d body=%s", startRec.Code, startRec.Body.String())
	}
	var deviceResp apiopenapi.AccountOAuthDeviceCodeResponse
	if err := json.NewDecoder(startRec.Body).Decode(&deviceResp); err != nil {
		t.Fatalf("decode device start: %v", err)
	}
	if deviceResp.Data.UserCode != "ABCD-9" || deviceResp.Data.VerificationUri == "" {
		t.Fatalf("unexpected device start: %+v", deviceResp.Data)
	}

	pollBody := `{"session_id":"` + deviceResp.Data.SessionId + `"}`
	firstPoll := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/device-code/poll", pollBody, sessionCookie, loginResp.Data.CsrfToken)
	if firstPoll.Code != http.StatusAccepted {
		t.Fatalf("expected first poll 202, got %d body=%s", firstPoll.Code, firstPoll.Body.String())
	}
	secondPoll := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/device-code/poll", pollBody, sessionCookie, loginResp.Data.CsrfToken)
	if secondPoll.Code != http.StatusOK {
		t.Fatalf("expected second poll 200, got %d body=%s", secondPoll.Code, secondPoll.Body.String())
	}
	var credResp apiopenapi.AccountOAuthCredentialResponse
	if err := json.NewDecoder(secondPoll.Body).Decode(&credResp); err != nil {
		t.Fatalf("decode device credential: %v", err)
	}
	cred := credResp.Data.Credential
	if cred["access_token"] != "dev-acc-9" || cred["refresh_token"] != "dev-ref-9" {
		t.Fatalf("unexpected device credential: %v", cred)
	}
}

func TestAdminAccountOAuthRequiresAdminAndCSRF(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	body := `{"config":{"client_id":"c","authorize_url":"https://provider.example/authorize","token_url":"https://provider.example/token","redirect_uri":"http://localhost:8080/cb"}}`

	// No session at all -> forbidden.
	noAuth := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/authorize-url", body, nil, "")
	if noAuth.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without session, got %d", noAuth.Code)
	}

	// Session but missing CSRF -> forbidden.
	noCSRF := adminAccountOAuthDo(t, handler, http.MethodPost, "/api/v1/admin/accounts/oauth/authorize-url", body, sessionCookie, "")
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without csrf, got %d body=%s", noCSRF.Code, noCSRF.Body.String())
	}

	// Read-only status endpoint does not require CSRF but requires admin.
	statusNoAuth := adminAccountOAuthDo(t, handler, http.MethodGet, "/api/v1/admin/accounts/oauth/pending/whatever", "", nil, "")
	if statusNoAuth.Code != http.StatusForbidden {
		t.Fatalf("expected 403 status without session, got %d", statusNoAuth.Code)
	}
	_ = loginResp
}
