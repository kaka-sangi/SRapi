package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const (
	oauthAuthorizationFlowV1  = "v1"
	oauthAuthorizationFlowAAD = "auth.oauth_flow:v1"
	oauthAuthorizationFlowTTL = 10 * time.Minute
	oauthStateBytes           = 32
	oauthCodeVerifierBytes    = 32
	oauthNonceBytes           = 24
)

type OAuthAuthorizationProviderConfig struct {
	Provider     userscontract.AuthIdentityProvider
	ProviderKey  string
	ClientID     string
	AuthorizeURL string
	RedirectURI  string
	Scopes       []string
}

type StartOAuthAuthorizationRequest struct {
	Intent     authcontract.PendingOAuthIntent
	Provider   userscontract.AuthIdentityProvider
	RedirectTo string
	Config     OAuthAuthorizationProviderConfig
}

type OAuthAuthorizationFlowState struct {
	Version      string                             `json:"version"`
	Intent       authcontract.PendingOAuthIntent    `json:"intent"`
	Provider     userscontract.AuthIdentityProvider `json:"provider"`
	ProviderKey  string                             `json:"provider_key"`
	ClientID     string                             `json:"client_id"`
	RedirectURI  string                             `json:"redirect_uri"`
	RedirectTo   string                             `json:"redirect_to"`
	State        string                             `json:"state"`
	CodeVerifier string                             `json:"code_verifier"`
	Nonce        string                             `json:"nonce"`
	CreatedAt    time.Time                          `json:"created_at"`
	ExpiresAt    time.Time                          `json:"expires_at"`
}

type StartOAuthAuthorizationResult struct {
	AuthorizationURL string
	FlowCookieValue  string
	ExpiresAt        time.Time
}

func (s *Service) StartOAuthAuthorization(req StartOAuthAuthorizationRequest) (StartOAuthAuthorizationResult, error) {
	if len(s.resetTokenKey) == 0 {
		return StartOAuthAuthorizationResult{}, ErrOAuthUnavailable
	}
	intent := normalizePendingOAuthIntent(req.Intent)
	if intent == "" {
		intent = authcontract.PendingOAuthIntentLogin
	}
	provider := normalizeAuthIdentityProvider(req.Provider)
	configProvider := normalizeAuthIdentityProvider(req.Config.Provider)
	if provider == "" || configProvider == "" || provider != configProvider {
		return StartOAuthAuthorizationResult{}, ErrInvalidInput
	}
	providerKey := strings.TrimSpace(req.Config.ProviderKey)
	clientID := strings.TrimSpace(req.Config.ClientID)
	authorizeURL := strings.TrimSpace(req.Config.AuthorizeURL)
	redirectURI := strings.TrimSpace(req.Config.RedirectURI)
	if providerKey == "" || clientID == "" || !validOAuthAuthorizeURL(authorizeURL) || !validOAuthRedirectURI(redirectURI) {
		return StartOAuthAuthorizationResult{}, ErrInvalidInput
	}

	state, err := randomRawToken(oauthStateBytes)
	if err != nil {
		return StartOAuthAuthorizationResult{}, err
	}
	codeVerifier, err := randomRawToken(oauthCodeVerifierBytes)
	if err != nil {
		return StartOAuthAuthorizationResult{}, err
	}
	nonce, err := randomRawToken(oauthNonceBytes)
	if err != nil {
		return StartOAuthAuthorizationResult{}, err
	}
	now := s.clock.Now()
	flow := OAuthAuthorizationFlowState{
		Version:      oauthAuthorizationFlowV1,
		Intent:       intent,
		Provider:     provider,
		ProviderKey:  providerKey,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		RedirectTo:   normalizePendingOAuthRedirect(req.RedirectTo),
		State:        state,
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
		CreatedAt:    now,
		ExpiresAt:    now.Add(oauthAuthorizationFlowTTL),
	}
	cookieValue, err := s.encryptOAuthAuthorizationFlow(flow)
	if err != nil {
		return StartOAuthAuthorizationResult{}, err
	}
	authURL, err := buildOAuthAuthorizationURL(authorizeURL, clientID, redirectURI, scopesForOAuthAuthorization(provider, req.Config.Scopes), state, codeVerifier, nonce)
	if err != nil {
		return StartOAuthAuthorizationResult{}, err
	}
	return StartOAuthAuthorizationResult{
		AuthorizationURL: authURL,
		FlowCookieValue:  cookieValue,
		ExpiresAt:        flow.ExpiresAt,
	}, nil
}

func (s *Service) DecodeOAuthAuthorizationFlow(cookieValue string) (OAuthAuthorizationFlowState, error) {
	cookieValue = strings.TrimSpace(cookieValue)
	if cookieValue == "" {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return OAuthAuthorizationFlowState{}, ErrOAuthUnavailable
	}
	parts := strings.Split(cookieValue, ":")
	if len(parts) != 3 || parts[0] != oauthAuthorizationFlowV1 {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	block, err := aes.NewCipher(s.resetTokenKey)
	if err != nil {
		return OAuthAuthorizationFlowState{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return OAuthAuthorizationFlowState{}, err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(oauthAuthorizationFlowAAD))
	if err != nil {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	var flow OAuthAuthorizationFlowState
	if err := json.Unmarshal(raw, &flow); err != nil {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	if flow.Version != oauthAuthorizationFlowV1 || flow.ClientID == "" || !validOAuthRedirectURI(flow.RedirectURI) || flow.State == "" || flow.CodeVerifier == "" || flow.ExpiresAt.IsZero() || !flow.ExpiresAt.After(s.clock.Now()) {
		return OAuthAuthorizationFlowState{}, ErrInvalidInput
	}
	return flow, nil
}

func (s *Service) encryptOAuthAuthorizationFlow(flow OAuthAuthorizationFlowState) (string, error) {
	raw, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.resetTokenKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, raw, []byte(oauthAuthorizationFlowAAD))
	return strings.Join([]string{
		oauthAuthorizationFlowV1,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

func buildOAuthAuthorizationURL(authorizeURL, clientID, redirectURI string, scopes []string, state string, codeVerifier string, nonce string) (string, error) {
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	values.Set("response_type", "code")
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("state", state)
	values.Set("code_challenge_method", "S256")
	values.Set("code_challenge", oauthCodeChallenge(codeVerifier))
	if len(scopes) > 0 {
		values.Set("scope", strings.Join(scopes, " "))
	}
	if scopesContainOpenID(scopes) {
		values.Set("nonce", nonce)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func oauthCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func scopesForOAuthAuthorization(provider userscontract.AuthIdentityProvider, scopes []string) []string {
	scopes = uniqueOAuthScopes(scopes)
	if len(scopes) > 0 {
		return scopes
	}
	switch provider {
	case userscontract.AuthIdentityProviderOIDC, userscontract.AuthIdentityProviderGoogle:
		return []string{"openid", "email", "profile"}
	case userscontract.AuthIdentityProviderGitHub:
		return []string{"read:user", "user:email"}
	case userscontract.AuthIdentityProviderLinuxDo:
		return []string{"user"}
	case userscontract.AuthIdentityProviderWeChat:
		return []string{"snsapi_login"}
	case userscontract.AuthIdentityProviderDingTalk:
		return []string{"openid"}
	default:
		return []string{}
	}
}

func uniqueOAuthScopes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, scope := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			key := strings.ToLower(scope)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, scope)
		}
	}
	return out
}

func scopesContainOpenID(scopes []string) bool {
	for _, scope := range scopes {
		if strings.EqualFold(strings.TrimSpace(scope), "openid") {
			return true
		}
	}
	return false
}

func validOAuthAuthorizeURL(value string) bool {
	parsed, ok := parseOAuthAbsoluteURL(value)
	return ok && parsed.Scheme == "https"
}

func validOAuthRedirectURI(value string) bool {
	parsed, ok := parseOAuthAbsoluteURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localOAuthRedirectHost(parsed.Hostname())
}

func parseOAuthAbsoluteURL(value string) (*url.URL, bool) {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, false
	}
	return parsed, true
}

func localOAuthRedirectHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
