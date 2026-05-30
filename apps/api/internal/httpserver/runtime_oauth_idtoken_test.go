package httpserver

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeOIDCProvider serves an OIDC discovery document and a JWKS so go-oidc can
// verify RS256-signed id_tokens against it.
type fakeOIDCProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	issuer string
}

func newFakeOIDCProvider(t *testing.T) *fakeOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	p := &fakeOIDCProvider{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 p.issuer,
			"authorization_endpoint": p.issuer + "/authorize",
			"token_endpoint":         p.issuer + "/token",
			"jwks_uri":               p.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"kid": "k1",
				"use": "sig",
				"alg": "RS256",
				"n":   b64url(key.N.Bytes()),
				"e":   b64url(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})
	p.server = httptest.NewServer(mux)
	p.issuer = p.server.URL
	return p
}

func (p *fakeOIDCProvider) close() { p.server.Close() }

func (p *fakeOIDCProvider) signIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := b64url([]byte(`{"alg":"RS256","typ":"JWT","kid":"k1"}`))
	payloadJSON, _ := json.Marshal(claims)
	payload := b64url(payloadJSON)
	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + b64url(sig)
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func TestVerifyOIDCIDToken(t *testing.T) {
	p := newFakeOIDCProvider(t)
	defer p.close()
	ctx := context.Background()
	const clientID = "client-123"
	const nonce = "the-nonce"

	baseClaims := func() map[string]any {
		return map[string]any{
			"iss":   p.issuer,
			"aud":   clientID,
			"sub":   "user-1",
			"exp":   time.Now().Add(time.Hour).Unix(),
			"iat":   time.Now().Unix(),
			"nonce": nonce,
		}
	}

	// Valid token verifies.
	if err := verifyOIDCIDToken(ctx, p.issuer, clientID, p.signIDToken(t, baseClaims()), nonce); err != nil {
		t.Fatalf("valid id_token rejected: %v", err)
	}

	// Nonce mismatch is rejected (our replay/CSRF check on top of go-oidc).
	if err := verifyOIDCIDToken(ctx, p.issuer, clientID, p.signIDToken(t, baseClaims()), "other-nonce"); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("nonce mismatch = %v, want nonce error", err)
	}

	// Wrong audience is rejected by go-oidc.
	wrongAud := baseClaims()
	wrongAud["aud"] = "someone-else"
	if err := verifyOIDCIDToken(ctx, p.issuer, clientID, p.signIDToken(t, wrongAud), nonce); err == nil {
		t.Fatal("wrong aud accepted, want rejection")
	}

	// Expired token is rejected by go-oidc.
	expired := baseClaims()
	expired["exp"] = time.Now().Add(-time.Hour).Unix()
	if err := verifyOIDCIDToken(ctx, p.issuer, clientID, p.signIDToken(t, expired), nonce); err == nil {
		t.Fatal("expired token accepted, want rejection")
	}

	// Missing token is rejected without a network round-trip.
	if err := verifyOIDCIDToken(ctx, p.issuer, clientID, "", nonce); err == nil {
		t.Fatal("empty id_token accepted, want rejection")
	}

	// A token signed by a different key fails signature verification.
	other := newFakeOIDCProvider(t)
	defer other.close()
	forged := other.signIDToken(t, baseClaims())
	if err := verifyOIDCIDToken(ctx, p.issuer, clientID, forged, nonce); err == nil {
		t.Fatal("token signed by wrong key accepted, want signature rejection")
	}
}
