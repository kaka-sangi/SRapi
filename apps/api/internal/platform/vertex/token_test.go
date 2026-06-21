package vertex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustSeedServiceAccount(t *testing.T, tokenURI string) *ServiceAccount {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkcs1 := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1})
	sa, err := ParseServiceAccount(mustJSON(t, map[string]any{
		"type":            "service_account",
		"project_id":      "demo",
		"private_key_id":  "kid-1",
		"private_key":     string(pemBytes),
		"client_email":    "demo@demo.iam.gserviceaccount.com",
		"token_uri":       tokenURI,
		"universe_domain": "googleapis.com",
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return sa
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func TestTokenSource_FetchesAndCaches(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Fatalf("wrong grant_type: %s", r.Form.Get("grant_type"))
		}
		assertion := r.Form.Get("assertion")
		if strings.Count(assertion, ".") != 2 {
			t.Fatalf("assertion is not a 3-part JWT: %s", assertion)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.test",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer upstream.Close()

	sa := mustSeedServiceAccount(t, upstream.URL+"/")
	src := NewTokenSource(nil)

	first, err := src.Token(context.Background(), sa, "")
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	if first.Value != "ya29.test" || !first.Valid() {
		t.Fatalf("unexpected token: %+v", first)
	}

	second, err := src.Token(context.Background(), sa, "")
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if second.Value != first.Value {
		t.Fatalf("cache returned a different token: %+v", second)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("cache should collapse identical calls, got %d upstream", got)
	}
}

func TestTokenSource_InvalidateForcesRefetch(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + r.FormValue("assertion")[:6],
			"expires_in":   3600,
		})
	}))
	defer upstream.Close()
	sa := mustSeedServiceAccount(t, upstream.URL+"/")
	src := NewTokenSource(nil)
	if _, err := src.Token(context.Background(), sa, ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	src.Invalidate(sa, "")
	if _, err := src.Token(context.Background(), sa, ""); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("invalidate should force a second upstream call, got %d", got)
	}
}

func TestTokenSource_RejectsUpstreamFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
	}))
	defer upstream.Close()
	sa := mustSeedServiceAccount(t, upstream.URL+"/")
	src := NewTokenSource(nil)
	if _, err := src.Token(context.Background(), sa, ""); err == nil {
		t.Fatal("expected error from upstream 400")
	}
}

func TestAccessTokenValid_GuardsAgainstNearExpiry(t *testing.T) {
	if (AccessToken{}.Valid()) {
		t.Fatal("zero token must not be valid")
	}
	about := AccessToken{Value: "x", ExpiresAt: time.Now().Add(10 * time.Second)}
	if about.Valid() {
		t.Fatal("token within the 60s guard must not be valid")
	}
	healthy := AccessToken{Value: "x", ExpiresAt: time.Now().Add(10 * time.Minute)}
	if !healthy.Valid() {
		t.Fatal("token with healthy ttl must be valid")
	}
}
