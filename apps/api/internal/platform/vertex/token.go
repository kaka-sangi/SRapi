package vertex

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ServiceAccount holds the fields lifted out of a Google Cloud service-account
// JSON blob that the JWT signer needs. The full blob is preserved on the
// account as the encrypted credential — this struct is the parsed, runtime
// view.
type ServiceAccount struct {
	Type            string
	ProjectID       string
	PrivateKeyID    string
	PrivateKey      string
	ClientEmail     string
	TokenURI        string
	UniverseDomain  string
}

// ParseServiceAccount unmarshals a normalized service-account JSON blob into
// the runtime view. Callers should run NormalizeServiceAccountJSON first so
// the private_key field is guaranteed to be valid PKCS#1 RSA PEM.
func ParseServiceAccount(raw []byte) (*ServiceAccount, error) {
	if len(raw) == 0 {
		return nil, errors.New("service account payload is empty")
	}
	var payload struct {
		Type            string `json:"type"`
		ProjectID       string `json:"project_id"`
		PrivateKeyID    string `json:"private_key_id"`
		PrivateKey      string `json:"private_key"`
		ClientEmail     string `json:"client_email"`
		TokenURI        string `json:"token_uri"`
		UniverseDomain  string `json:"universe_domain"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	if strings.TrimSpace(payload.ClientEmail) == "" {
		return nil, errors.New("service account missing client_email")
	}
	if strings.TrimSpace(payload.PrivateKey) == "" {
		return nil, errors.New("service account missing private_key")
	}
	if strings.TrimSpace(payload.TokenURI) == "" {
		payload.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &ServiceAccount{
		Type:           payload.Type,
		ProjectID:      payload.ProjectID,
		PrivateKeyID:   payload.PrivateKeyID,
		PrivateKey:     payload.PrivateKey,
		ClientEmail:    payload.ClientEmail,
		TokenURI:       payload.TokenURI,
		UniverseDomain: payload.UniverseDomain,
	}, nil
}

// AccessToken is the cached result of a JWT-bearer exchange against Google's
// token endpoint.
type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

// Valid reports whether the token still has more than a 60-second guard
// window before expiry.
func (t AccessToken) Valid() bool {
	return t.Value != "" && time.Until(t.ExpiresAt) > 60*time.Second
}

// TokenSource fetches OAuth2 access tokens for a service account using the
// JWT-bearer grant. Each TokenSource is keyed on the (client_email,
// private_key_id, scope) tuple so the in-memory cache is shared across
// callers that target the same identity.
type TokenSource struct {
	httpClient *http.Client

	mu     sync.Mutex
	cache  map[string]AccessToken
	nowFn  func() time.Time
}

// NewTokenSource builds a TokenSource backed by the provided HTTP client (or
// a default 10s client if nil).
func NewTokenSource(client *http.Client) *TokenSource {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &TokenSource{
		httpClient: client,
		cache:      map[string]AccessToken{},
		nowFn:      time.Now,
	}
}

// Token returns a valid access token for the supplied service account and
// scope, fetching a fresh one if the cached entry is missing or about to
// expire. Concurrent callers targeting the same key serialize on the
// internal mutex; this is fine because the upstream call typically completes
// in under 200ms and the cached result is reused for the next ~50 minutes.
func (s *TokenSource) Token(ctx context.Context, sa *ServiceAccount, scope string) (AccessToken, error) {
	if sa == nil {
		return AccessToken{}, errors.New("service account is nil")
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "https://www.googleapis.com/auth/cloud-platform"
	}
	key := tokenCacheKey(sa, scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.cache[key]; ok && cached.Valid() {
		return cached, nil
	}
	token, err := s.exchange(ctx, sa, scope)
	if err != nil {
		return AccessToken{}, err
	}
	s.cache[key] = token
	return token, nil
}

// Invalidate drops the cached token for (sa, scope) — used when the upstream
// returns 401 so the next call forces a fresh exchange.
func (s *TokenSource) Invalidate(sa *ServiceAccount, scope string) {
	if sa == nil {
		return
	}
	if scope == "" {
		scope = "https://www.googleapis.com/auth/cloud-platform"
	}
	key := tokenCacheKey(sa, scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, key)
}

func (s *TokenSource) exchange(ctx context.Context, sa *ServiceAccount, scope string) (AccessToken, error) {
	now := s.nowFn()
	assertion, err := signJWT(sa, scope, now)
	if err != nil {
		return AccessToken{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return AccessToken{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return AccessToken{}, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != http.StatusOK {
		return AccessToken{}, fmt.Errorf("token exchange %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return AccessToken{}, fmt.Errorf("decode token response: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return AccessToken{}, errors.New("token response missing access_token")
	}
	ttl := time.Duration(parsed.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	// Subtract a 5-minute guard so callers always see a token with usable
	// life left even if exchange was delayed in flight.
	expiresAt := now.Add(ttl).Add(-5 * time.Minute)
	if expiresAt.Before(now.Add(time.Minute)) {
		expiresAt = now.Add(ttl)
	}
	return AccessToken{Value: parsed.AccessToken, ExpiresAt: expiresAt}, nil
}

// signJWT builds and RS256-signs the JWT assertion required by the
// JWT-bearer grant flow.
func signJWT(sa *ServiceAccount, scope string, now time.Time) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	if id := strings.TrimSpace(sa.PrivateKeyID); id != "" {
		header["kid"] = id
	}
	payload := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": scope,
		"aud":   sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	encHeader, err := jsonBase64URL(header)
	if err != nil {
		return "", fmt.Errorf("encode jwt header: %w", err)
	}
	encPayload, err := jsonBase64URL(payload)
	if err != nil {
		return "", fmt.Errorf("encode jwt payload: %w", err)
	}
	signingInput := encHeader + "." + encPayload
	key, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseRSAPrivateKey(rawPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(rawPEM))
	if block == nil {
		return nil, errors.New("private key is not valid PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not an RSA key")
	}
	return rsaKey, nil
}

func jsonBase64URL(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func tokenCacheKey(sa *ServiceAccount, scope string) string {
	h := sha256.New()
	h.Write([]byte(sa.ClientEmail))
	h.Write([]byte{0})
	h.Write([]byte(sa.PrivateKeyID))
	h.Write([]byte{0})
	h.Write([]byte(scope))
	return hex.EncodeToString(h.Sum(nil))
}
