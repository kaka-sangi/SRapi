// Package service implements the upstream-account OAuth provisioning state
// machine: an interactive authorization-code (PKCE) flow and an RFC 8628
// device-code flow that mint access/refresh tokens for an upstream provider
// account, replacing the operator hand-paste of access_token/refresh_token.
//
// Pending sessions are held in a short-lived in-memory TTL store on the
// service; nothing is persisted, so a process restart simply drops in-flight
// authorizations (the operator restarts the wizard). The HTTP client and clock
// are injected so the whole machine is deterministic under test against a stub
// token endpoint.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/account_provisioning/contract"
)

var (
	// ErrInvalidInput is returned for malformed provider configs or arguments.
	ErrInvalidInput = errors.New("account_provisioning: invalid input")
	// ErrSessionNotFound is returned when a pending session id is unknown.
	ErrSessionNotFound = errors.New("account_provisioning: session not found")
	// ErrSessionExpired is returned when a session TTL has elapsed.
	ErrSessionExpired = errors.New("account_provisioning: session expired")
	// ErrStateMismatch is returned when the authorization-code callback state
	// does not match the pending session (CSRF / mix-up protection).
	ErrStateMismatch = errors.New("account_provisioning: state mismatch")
	// ErrWrongMode is returned when an operation is applied to a session whose
	// flow mode does not support it (e.g. polling an authorization-code session).
	ErrWrongMode = errors.New("account_provisioning: wrong session mode")
	// ErrAuthorizationPending mirrors the RFC 8628 authorization_pending error:
	// the device-code grant is not yet authorized; the caller should keep polling.
	ErrAuthorizationPending = errors.New("account_provisioning: authorization pending")
	// ErrSlowDown mirrors the RFC 8628 slow_down error.
	ErrSlowDown = errors.New("account_provisioning: slow down")
	// ErrProviderRejected is returned when the provider terminally denies the grant.
	ErrProviderRejected = errors.New("account_provisioning: provider rejected authorization")
	// ErrProviderUnavailable is returned for transport/5xx failures talking to the provider.
	ErrProviderUnavailable = errors.New("account_provisioning: provider unavailable")
)

const (
	tokenAuthMethodBasic   = "client_secret_basic"
	tokenAuthMethodPost    = "client_secret_post"
	tokenAuthMethodNone    = "none"
	defaultSessionTTL      = 10 * time.Minute
	defaultDeviceInterval  = 5
	maxDeviceInterval      = 60
	providerBodyLimit      = 1 << 20
	sessionIDBytes         = 24
	stateBytes             = 24
	codeVerifierBytes      = 32
	defaultProviderTimeout = 15 * time.Second
)

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Service is the provisioning state machine plus its in-memory session store.
type Service struct {
	mu         sync.Mutex
	sessions   map[string]*contract.PendingSession
	client     *http.Client
	clock      Clock
	sessionTTL time.Duration
}

// Option configures the Service.
type Option func(*Service)

// WithHTTPClient overrides the provider HTTP client (used in tests to point at
// a stub token endpoint).
func WithHTTPClient(client *http.Client) Option {
	return func(s *Service) {
		if client != nil {
			s.client = client
		}
	}
}

// WithClock overrides the clock for deterministic TTL behavior under test.
func WithClock(clock Clock) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithSessionTTL overrides the pending-session lifetime.
func WithSessionTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.sessionTTL = ttl
		}
	}
}

// New constructs a provisioning service.
func New(opts ...Option) *Service {
	s := &Service{
		sessions:   map[string]*contract.PendingSession{},
		client:     &http.Client{Timeout: defaultProviderTimeout},
		clock:      systemClock{},
		sessionTTL: defaultSessionTTL,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AuthorizeURLResult is returned by StartAuthorizationURL.
type AuthorizeURLResult struct {
	SessionID        string
	AuthorizationURL string
	State            string
	ExpiresAt        time.Time
}

// StartAuthorizationURL begins an authorization-code (PKCE) provisioning
// session: it stores the pending state and returns the provider authorization
// URL the operator opens to consent, plus a short-lived session id.
func (s *Service) StartAuthorizationURL(config contract.ProviderOAuthConfig) (AuthorizeURLResult, error) {
	normalized, err := normalizeAuthCodeConfig(config)
	if err != nil {
		return AuthorizeURLResult{}, err
	}
	sessionID, err := randomToken(sessionIDBytes)
	if err != nil {
		return AuthorizeURLResult{}, err
	}
	state, err := randomToken(stateBytes)
	if err != nil {
		return AuthorizeURLResult{}, err
	}
	codeVerifier := ""
	if normalized.UsePKCE {
		codeVerifier, err = randomToken(codeVerifierBytes)
		if err != nil {
			return AuthorizeURLResult{}, err
		}
	}
	authURL, err := buildAuthorizationURL(normalized, state, codeVerifier)
	if err != nil {
		return AuthorizeURLResult{}, err
	}
	now := s.clock.Now()
	session := &contract.PendingSession{
		ID:           sessionID,
		Mode:         contract.ModeAuthorizationCode,
		Status:       contract.StatusPending,
		ClientID:     normalized.ClientID,
		ClientSecret: normalized.ClientSecret,
		TokenURL:     normalized.TokenURL,
		RedirectURI:  normalized.RedirectURI,
		Scopes:       append([]string(nil), normalized.Scopes...),
		State:        state,
		CodeVerifier: codeVerifier,
		UsePKCE:      normalized.UsePKCE,
		CreatedAt:    now,
		ExpiresAt:    now.Add(s.sessionTTL),
	}
	s.putSession(session)
	return AuthorizeURLResult{
		SessionID:        sessionID,
		AuthorizationURL: authURL,
		State:            state,
		ExpiresAt:        session.ExpiresAt,
	}, nil
}

// ExchangeCode redeems the authorization code returned to the operator's
// redirect into access/refresh tokens, completing the session.
func (s *Service) ExchangeCode(ctx context.Context, sessionID, code, state string) (contract.MintedCredential, error) {
	session, err := s.takePending(sessionID)
	if err != nil {
		return contract.MintedCredential{}, err
	}
	if session.Mode != contract.ModeAuthorizationCode {
		return contract.MintedCredential{}, ErrWrongMode
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return contract.MintedCredential{}, ErrInvalidInput
	}
	if strings.TrimSpace(state) == "" || state != session.State {
		s.failSession(session.ID, "state mismatch")
		return contract.MintedCredential{}, ErrStateMismatch
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", session.RedirectURI)
	if session.UsePKCE && session.CodeVerifier != "" {
		form.Set("code_verifier", session.CodeVerifier)
	}
	credential, err := s.postTokenRequest(ctx, session, form)
	if err != nil {
		s.failSession(session.ID, exchangeFailureReason(err))
		return contract.MintedCredential{}, err
	}
	s.completeSession(session.ID)
	return credential, nil
}

// DeviceCodeResult is returned by StartDeviceCode.
type DeviceCodeResult struct {
	SessionID       string
	UserCode        string
	VerificationURI string
	// VerificationURIComplete embeds the user code so the operator can open one link.
	VerificationURIComplete string
	IntervalSecs            int
	ExpiresAt               time.Time
}

// StartDeviceCode begins an RFC 8628 device-authorization session by calling
// the provider device-authorization endpoint, then stores the device_code so
// PollDeviceCode can redeem it.
func (s *Service) StartDeviceCode(ctx context.Context, config contract.ProviderOAuthConfig) (DeviceCodeResult, error) {
	normalized, err := normalizeDeviceConfig(config)
	if err != nil {
		return DeviceCodeResult{}, err
	}
	form := url.Values{}
	form.Set("client_id", normalized.ClientID)
	if len(normalized.Scopes) > 0 {
		form.Set("scope", strings.Join(normalized.Scopes, " "))
	}
	resp, err := s.postForm(ctx, normalized.DeviceAuthorizeURL, normalized, form)
	if err != nil {
		return DeviceCodeResult{}, err
	}
	deviceResp, err := parseDeviceAuthorizationResponse(resp)
	if err != nil {
		return DeviceCodeResult{}, err
	}
	sessionID, err := randomToken(sessionIDBytes)
	if err != nil {
		return DeviceCodeResult{}, err
	}
	now := s.clock.Now()
	interval := deviceResp.Interval
	if interval <= 0 {
		interval = defaultDeviceInterval
	}
	if interval > maxDeviceInterval {
		interval = maxDeviceInterval
	}
	ttl := s.sessionTTL
	if deviceResp.ExpiresIn > 0 {
		providerTTL := time.Duration(deviceResp.ExpiresIn) * time.Second
		if providerTTL < ttl {
			ttl = providerTTL
		}
	}
	session := &contract.PendingSession{
		ID:           sessionID,
		Mode:         contract.ModeDeviceCode,
		Status:       contract.StatusPending,
		ClientID:     normalized.ClientID,
		ClientSecret: normalized.ClientSecret,
		TokenURL:     normalized.TokenURL,
		Scopes:       append([]string(nil), normalized.Scopes...),
		DeviceCode:   deviceResp.DeviceCode,
		UserCode:     deviceResp.UserCode,
		IntervalSecs: interval,
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
	}
	s.putSession(session)
	return DeviceCodeResult{
		SessionID:               sessionID,
		UserCode:                deviceResp.UserCode,
		VerificationURI:         deviceResp.VerificationURI,
		VerificationURIComplete: deviceResp.VerificationURIComplete,
		IntervalSecs:            interval,
		ExpiresAt:               session.ExpiresAt,
	}, nil
}

// PollDeviceCode redeems the stored device_code once. It returns
// ErrAuthorizationPending / ErrSlowDown while the operator has not yet
// approved, and a minted credential on success.
func (s *Service) PollDeviceCode(ctx context.Context, sessionID string) (contract.MintedCredential, error) {
	session, err := s.peekPending(sessionID)
	if err != nil {
		return contract.MintedCredential{}, err
	}
	if session.Mode != contract.ModeDeviceCode {
		return contract.MintedCredential{}, ErrWrongMode
	}
	s.markPolled(session.ID)
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", session.DeviceCode)
	credential, err := s.postTokenRequest(ctx, session, form)
	if err != nil {
		switch {
		case errors.Is(err, ErrAuthorizationPending), errors.Is(err, ErrSlowDown):
			return contract.MintedCredential{}, err
		default:
			s.failSession(session.ID, exchangeFailureReason(err))
			return contract.MintedCredential{}, err
		}
	}
	s.completeSession(session.ID)
	return credential, nil
}

// Status returns a snapshot copy of a pending session, expiring it lazily.
func (s *Service) Status(sessionID string) (contract.PendingSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return contract.PendingSession{}, ErrSessionNotFound
	}
	if session.Status == contract.StatusPending && !s.clock.Now().Before(session.ExpiresAt) {
		session.Status = contract.StatusExpired
		session.FailureReason = "expired"
	}
	return *session, nil
}

// ---- internal helpers ----

func (s *Service) putSession(session *contract.PendingSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	s.sessions[session.ID] = session
}

// gcLocked drops sessions whose TTL elapsed more than one TTL ago, keeping the
// map bounded without a background goroutine. Caller holds s.mu.
func (s *Service) gcLocked() {
	cutoff := s.clock.Now().Add(-s.sessionTTL)
	for id, session := range s.sessions {
		if session.ExpiresAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

// takePending fetches a pending, unexpired session for a terminal operation.
func (s *Service) takePending(sessionID string) (contract.PendingSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return contract.PendingSession{}, ErrSessionNotFound
	}
	if !s.clock.Now().Before(session.ExpiresAt) {
		session.Status = contract.StatusExpired
		session.FailureReason = "expired"
		return contract.PendingSession{}, ErrSessionExpired
	}
	if session.Status != contract.StatusPending {
		return contract.PendingSession{}, ErrInvalidInput
	}
	return *session, nil
}

func (s *Service) peekPending(sessionID string) (contract.PendingSession, error) {
	return s.takePending(sessionID)
}

func (s *Service) markPolled(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[sessionID]; ok {
		session.LastPolledAt = s.clock.Now()
	}
}

func (s *Service) completeSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[sessionID]; ok {
		session.Status = contract.StatusCompleted
		session.CompletedAt = s.clock.Now()
		// Drop the device_code / verifier once consumed; they are single-use.
		session.DeviceCode = ""
		session.CodeVerifier = ""
	}
}

func (s *Service) failSession(sessionID, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[sessionID]; ok {
		session.Status = contract.StatusFailed
		session.FailureReason = reason
	}
}

// postTokenRequest sends a token-endpoint POST and parses the credential,
// applying client authentication per the session's configured auth method.
func (s *Service) postTokenRequest(ctx context.Context, session contract.PendingSession, form url.Values) (contract.MintedCredential, error) {
	cfg := contract.ProviderOAuthConfig{
		ClientID:        session.ClientID,
		ClientSecret:    session.ClientSecret,
		TokenAuthMethod: clientAuthMethod(session.ClientSecret),
	}
	resp, err := s.postForm(ctx, session.TokenURL, cfg, form)
	if err != nil {
		return contract.MintedCredential{}, err
	}
	return parseTokenResponse(resp)
}

// postForm performs a form-encoded POST applying client authentication, returns
// the decoded JSON body and maps transport/status errors to sentinel errors.
func (s *Service) postForm(ctx context.Context, endpoint string, cfg contract.ProviderOAuthConfig, form url.Values) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	authMethod := strings.ToLower(strings.TrimSpace(cfg.TokenAuthMethod))
	switch authMethod {
	case tokenAuthMethodPost:
		form.Set("client_id", cfg.ClientID)
		if cfg.ClientSecret != "" {
			form.Set("client_secret", cfg.ClientSecret)
		}
	case tokenAuthMethodBasic:
		// handled via header below
	default:
		// public client / none: include client_id when not already present.
		if form.Get("client_id") == "" {
			form.Set("client_id", cfg.ClientID)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, ErrInvalidInput
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if authMethod == tokenAuthMethodBasic && cfg.ClientSecret != "" {
		req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, ErrProviderUnavailable
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, providerBodyLimit))
	if readErr != nil {
		return nil, ErrProviderUnavailable
	}
	decoded := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &decoded); err != nil {
			if resp.StatusCode >= http.StatusInternalServerError {
				return nil, ErrProviderUnavailable
			}
			return nil, ErrProviderRejected
		}
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return decoded, nil
	}
	return nil, classifyProviderError(resp.StatusCode, decoded)
}

// classifyProviderError maps an OAuth error response body to a sentinel error,
// recognizing the RFC 8628 polling errors.
func classifyProviderError(statusCode int, body map[string]any) error {
	switch strings.ToLower(strings.TrimSpace(stringValue(body["error"]))) {
	case "authorization_pending":
		return ErrAuthorizationPending
	case "slow_down":
		return ErrSlowDown
	case "access_denied", "expired_token", "invalid_grant":
		return ErrProviderRejected
	}
	if statusCode >= http.StatusInternalServerError {
		return ErrProviderUnavailable
	}
	return ErrProviderRejected
}

type deviceAuthorizationResponse struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Interval                int
	ExpiresIn               int
}

func parseDeviceAuthorizationResponse(body map[string]any) (deviceAuthorizationResponse, error) {
	out := deviceAuthorizationResponse{
		DeviceCode:              stringValue(body["device_code"]),
		UserCode:                stringValue(body["user_code"]),
		VerificationURI:         firstString(body, "verification_uri", "verification_url"),
		VerificationURIComplete: firstString(body, "verification_uri_complete", "verification_url_complete"),
		Interval:                intValue(body["interval"]),
		ExpiresIn:               intValue(body["expires_in"]),
	}
	if out.DeviceCode == "" || out.UserCode == "" || out.VerificationURI == "" {
		return deviceAuthorizationResponse{}, ErrProviderRejected
	}
	return out, nil
}

func parseTokenResponse(body map[string]any) (contract.MintedCredential, error) {
	credential := contract.MintedCredential{
		AccessToken:  stringValue(body["access_token"]),
		RefreshToken: stringValue(body["refresh_token"]),
		TokenType:    stringValue(body["token_type"]),
		Scope:        stringValue(body["scope"]),
		IDToken:      stringValue(body["id_token"]),
		ExpiresInSec: intValue(body["expires_in"]),
		Raw:          body,
	}
	if credential.AccessToken == "" && credential.RefreshToken == "" {
		return contract.MintedCredential{}, ErrProviderRejected
	}
	return credential, nil
}

func normalizeAuthCodeConfig(config contract.ProviderOAuthConfig) (contract.ProviderOAuthConfig, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.AuthorizeURL = strings.TrimSpace(config.AuthorizeURL)
	config.TokenURL = strings.TrimSpace(config.TokenURL)
	config.RedirectURI = strings.TrimSpace(config.RedirectURI)
	if config.ClientID == "" || !validAuthorizeURL(config.AuthorizeURL) || !validBackchannelURL(config.TokenURL) || !validRedirectURI(config.RedirectURI) {
		return contract.ProviderOAuthConfig{}, ErrInvalidInput
	}
	config.Scopes = normalizeScopes(config.Scopes)
	return config, nil
}

func normalizeDeviceConfig(config contract.ProviderOAuthConfig) (contract.ProviderOAuthConfig, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.DeviceAuthorizeURL = strings.TrimSpace(config.DeviceAuthorizeURL)
	config.TokenURL = strings.TrimSpace(config.TokenURL)
	if config.ClientID == "" || !validBackchannelURL(config.DeviceAuthorizeURL) || !validBackchannelURL(config.TokenURL) {
		return contract.ProviderOAuthConfig{}, ErrInvalidInput
	}
	config.Scopes = normalizeScopes(config.Scopes)
	return config, nil
}

func buildAuthorizationURL(config contract.ProviderOAuthConfig, state, codeVerifier string) (string, error) {
	parsed, err := url.Parse(config.AuthorizeURL)
	if err != nil {
		return "", ErrInvalidInput
	}
	values := parsed.Query()
	values.Set("response_type", "code")
	values.Set("client_id", config.ClientID)
	values.Set("redirect_uri", config.RedirectURI)
	values.Set("state", state)
	if len(config.Scopes) > 0 {
		values.Set("scope", strings.Join(config.Scopes, " "))
	}
	if config.UsePKCE && codeVerifier != "" {
		values.Set("code_challenge_method", "S256")
		values.Set("code_challenge", codeChallenge(codeVerifier))
	}
	for key, value := range config.ExtraAuthorizeParams {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func clientAuthMethod(clientSecret string) string {
	if strings.TrimSpace(clientSecret) == "" {
		return tokenAuthMethodNone
	}
	return tokenAuthMethodPost
}

func exchangeFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrProviderRejected):
		return "provider rejected authorization"
	case errors.Is(err, ErrProviderUnavailable):
		return "provider unavailable"
	case errors.Is(err, ErrStateMismatch):
		return "state mismatch"
	default:
		return "exchange failed"
	}
}

func normalizeScopes(values []string) []string {
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

func validAuthorizeURL(value string) bool {
	parsed, ok := parseAbsoluteURL(value)
	return ok && parsed.Scheme == "https"
}

func validBackchannelURL(value string) bool {
	parsed, ok := parseAbsoluteURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localHost(parsed.Hostname())
}

func validRedirectURI(value string) bool {
	parsed, ok := parseAbsoluteURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localHost(parsed.Hostname())
}

func parseAbsoluteURL(value string) (*url.URL, bool) {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Fragment != "" {
		return nil, false
	}
	return parsed, true
}

func localHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := stringValue(values[key]); v != "" {
			return v
		}
	}
	return ""
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
