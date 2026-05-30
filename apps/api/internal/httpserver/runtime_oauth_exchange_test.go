package httpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
)

func TestExchangeOAuthAuthorizationCodeClientSecret(t *testing.T) {
	var captured url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		captured = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok","token_type":"bearer"}`)
	}))
	defer srv.Close()

	config := admincontrolcontract.OAuthProviderConfig{TokenURL: srv.URL}
	flow := authservice.OAuthAuthorizationFlowState{ClientID: "cid", RedirectURI: "https://app.example/cb", CodeVerifier: "verif"}

	// Confidential client: secret is sent, PKCE is preserved.
	tok, _, err := exchangeOAuthAuthorizationCode(context.Background(), srv.Client(), config, flow, "the-code", "shh")
	if err != nil || tok != "tok" {
		t.Fatalf("exchange = %q err=%v, want tok", tok, err)
	}
	if got := captured.Get("client_secret"); got != "shh" {
		t.Fatalf("client_secret = %q, want shh", got)
	}
	if got := captured.Get("code_verifier"); got != "verif" {
		t.Fatalf("code_verifier = %q, want verif (PKCE must coexist)", got)
	}

	// Public client: no secret is sent.
	if _, _, err := exchangeOAuthAuthorizationCode(context.Background(), srv.Client(), config, flow, "the-code", ""); err != nil {
		t.Fatalf("public exchange err=%v", err)
	}
	if got := captured.Get("client_secret"); got != "" {
		t.Fatalf("public client sent client_secret=%q, want empty", got)
	}
}
