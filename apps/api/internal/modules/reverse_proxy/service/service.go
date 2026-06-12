package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	"nhooyr.io/websocket"
)

const (
	maxReverseProxyResponseBytes = 8 << 20
	statusClientClosedRequest    = 499
	oauthRefreshEncodingForm     = "form"
	oauthRefreshEncodingJSON     = "json"
)

type oauthRefreshConfig struct {
	TokenEndpoint string
	ClientID      string
	ClientSecret  string
	Scope         string
	Encoding      string
}

type ClientFactory func(account contract.AccountRuntime) (*http.Client, error)

type Service struct {
	mu             sync.Mutex
	clients        map[string]*http.Client
	factory        ClientFactory
	defaultClient  *http.Client
	refreshLocks   map[int]*sync.Mutex
	requestTotal   int
	successTotal   int
	errorTotal     map[string]int
	challengeTotal map[string]int
	lockedTotal    int
	bannedTotal    int
	refreshTotal   map[string]int
}

var (
	_ contract.Runtime          = (*Service)(nil)
	_ contract.WebSocketRuntime = (*Service)(nil)
)

// Option configures a reverse-proxy Service.
type Option func(*serviceConfig)

type serviceConfig struct {
	blockPrivateEgress bool
}

// WithBlockedPrivateEgress blocks direct (non-proxied) upstream dials whose
// resolved IP is in a private/loopback/link-local/metadata range. Enable it
// outside local mode; the zero value (and a plain New(nil)) leaves egress
// unscreened for local/dev/test against loopback servers.
func WithBlockedPrivateEgress(blocked bool) Option {
	return func(c *serviceConfig) { c.blockPrivateEgress = blocked }
}

func New(factory ClientFactory, opts ...Option) (*Service, error) {
	cfg := serviceConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	defaultClient, err := newIsolatedClient(contract.AccountRuntime{}, cfg.blockPrivateEgress)
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = func(account contract.AccountRuntime) (*http.Client, error) {
			return newIsolatedClient(account, cfg.blockPrivateEgress)
		}
	}
	return &Service{
		clients:        map[string]*http.Client{},
		factory:        factory,
		defaultClient:  defaultClient,
		refreshLocks:   map[int]*sync.Mutex{},
		errorTotal:     map[string]int{},
		challengeTotal: map[string]int{},
		refreshTotal:   map[string]int{},
	}, nil
}

func (s *Service) Do(ctx context.Context, req contract.Request) (contract.Response, error) {
	if req.Account.AccountID <= 0 || strings.TrimSpace(req.Method) == "" || strings.TrimSpace(req.URL) == "" {
		return contract.Response{}, ErrInvalidInput
	}
	if err := guardBody(req.Body); err != nil {
		s.recordError(errorClass(err))
		return contract.Response{}, err
	}
	profile, err := resolveEgressProfile(req.Account)
	if err != nil {
		s.recordError(errorClass(err))
		return contract.Response{}, err
	}
	if err := validateEgressTargetURL(req.URL, profile); err != nil {
		s.recordError(errorClass(err))
		return contract.Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		s.recordError("invalid_request")
		return contract.Response{}, err
	}
	httpReq.Header = sanitizeHeadersForProfile(req.Headers, profile, sanitizeHeaderOptionsForAccount(req.Account))
	applyEgressStaticHeaders(httpReq.Header, profile)
	if len(req.Body) > 0 && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	injectAuth(httpReq.Header, req.Account)
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", userAgentForProfile(req.Account, profile))
	}

	client, err := s.clientFor(req.Account)
	if err != nil {
		s.recordError(errorClass(err))
		return contract.Response{}, err
	}
	s.recordRequest()
	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			runtimeErr := contract.RuntimeError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "reverse proxy request timed out"}
			s.recordError(runtimeErr.Class)
			return contract.Response{}, runtimeErr
		}
		runtimeErr := contract.RuntimeError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy request failed"}
		s.recordError(runtimeErr.Class)
		return contract.Response{}, runtimeErr
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReverseProxyResponseBytes))
	if err != nil {
		s.recordError("network_error")
		return contract.Response{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		runtimeErr := classifyRuntimeError(resp.StatusCode, body)
		s.recordError(runtimeErr.Class)
		return contract.Response{}, runtimeErr
	}
	s.recordSuccess()
	return contract.Response{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeaders(resp.Header),
		Body:       body,
	}, nil
}

// DoStream performs the upstream request like Do but returns the live response
// body for incremental streaming instead of buffering it with io.ReadAll. The
// caller owns StreamResponse.Body and MUST close it. On a non-2xx upstream
// status the body is consumed (bounded) and closed here and a classified
// RuntimeError is returned, so callers can fail over before writing anything
// downstream. This shares Do's egress profile, SSRF guard, auth injection, and
// client selection so streamed traffic is byte-for-byte identical on the wire.
func (s *Service) DoStream(ctx context.Context, req contract.Request) (contract.StreamResponse, error) {
	if req.Account.AccountID <= 0 || strings.TrimSpace(req.Method) == "" || strings.TrimSpace(req.URL) == "" {
		return contract.StreamResponse{}, ErrInvalidInput
	}
	if err := guardBody(req.Body); err != nil {
		s.recordError(errorClass(err))
		return contract.StreamResponse{}, err
	}
	profile, err := resolveEgressProfile(req.Account)
	if err != nil {
		s.recordError(errorClass(err))
		return contract.StreamResponse{}, err
	}
	if err := validateEgressTargetURL(req.URL, profile); err != nil {
		s.recordError(errorClass(err))
		return contract.StreamResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		s.recordError("invalid_request")
		return contract.StreamResponse{}, err
	}
	httpReq.Header = sanitizeHeadersForProfile(req.Headers, profile, sanitizeHeaderOptionsForAccount(req.Account))
	applyEgressStaticHeaders(httpReq.Header, profile)
	if len(req.Body) > 0 && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	injectAuth(httpReq.Header, req.Account)
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", userAgentForProfile(req.Account, profile))
	}

	client, err := s.clientFor(req.Account)
	if err != nil {
		s.recordError(errorClass(err))
		return contract.StreamResponse{}, err
	}
	s.recordRequest()
	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			runtimeErr := contract.RuntimeError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "reverse proxy request timed out"}
			s.recordError(runtimeErr.Class)
			return contract.StreamResponse{}, runtimeErr
		}
		runtimeErr := contract.RuntimeError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy request failed"}
		s.recordError(runtimeErr.Class)
		return contract.StreamResponse{}, runtimeErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxReverseProxyResponseBytes))
		_ = resp.Body.Close()
		runtimeErr := classifyRuntimeError(resp.StatusCode, body)
		s.recordError(runtimeErr.Class)
		return contract.StreamResponse{}, runtimeErr
	}
	s.recordSuccess()
	return contract.StreamResponse{
		StatusCode: resp.StatusCode,
		Headers:    cloneHeaders(resp.Header),
		Body:       resp.Body,
	}, nil
}

func (s *Service) RelayWebSocket(ctx context.Context, req contract.WebSocketRelayRequest) (contract.WebSocketRelayResult, error) {
	if req.Account.AccountID <= 0 || strings.TrimSpace(req.URL) == "" || req.ClientToUpstream == nil || req.UpstreamToClient == nil {
		return contract.WebSocketRelayResult{}, ErrInvalidInput
	}
	profile, err := resolveEgressProfile(req.Account)
	if err != nil {
		s.recordError(errorClass(err))
		return contract.WebSocketRelayResult{}, err
	}
	if err := validateEgressTargetURL(req.URL, profile); err != nil {
		s.recordError(errorClass(err))
		return contract.WebSocketRelayResult{}, err
	}
	headers := sanitizeHeadersForProfile(req.Headers, profile, sanitizeHeaderOptionsForAccount(req.Account))
	applyEgressStaticHeaders(headers, profile)
	injectAuth(headers, req.Account)
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", userAgentForProfile(req.Account, profile))
	}
	client, err := s.clientFor(req.Account)
	if err != nil {
		s.recordError(errorClass(err))
		return contract.WebSocketRelayResult{}, err
	}
	startedAt := time.Now().UTC()
	s.recordRequest()
	conn, resp, err := websocket.Dial(ctx, req.URL, &websocket.DialOptions{
		HTTPClient:      client,
		HTTPHeader:      headers,
		Subprotocols:    append([]string(nil), req.Subprotocols...),
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		class := "network_error"
		statusCode := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			class = "timeout"
			statusCode = http.StatusGatewayTimeout
		} else if resp != nil && resp.StatusCode > 0 {
			statusCode = resp.StatusCode
		}
		runtimeErr := contract.RuntimeError{Class: class, StatusCode: statusCode, Message: "reverse proxy websocket dial failed"}
		s.recordError(runtimeErr.Class)
		return contract.WebSocketRelayResult{}, runtimeErr
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	result := contract.WebSocketRelayResult{
		UpstreamStatusCode: http.StatusSwitchingProtocols,
		Subprotocol:        conn.Subprotocol(),
		StartedAt:          startedAt,
	}
	if resp != nil && resp.StatusCode > 0 {
		result.UpstreamStatusCode = resp.StatusCode
	}

	stats := &webSocketRelayStats{}
	err = relayWebSocketMessages(ctx, conn, req.ClientToUpstream, req.UpstreamToClient, stats)
	statsSnapshot := stats.snapshot()
	result.MessagesUpstream = statsSnapshot.messagesUpstream
	result.MessagesDownstream = statsSnapshot.messagesDownstream
	result.BytesUpstream = statsSnapshot.bytesUpstream
	result.BytesDownstream = statsSnapshot.bytesDownstream
	result.EndedAt = time.Now().UTC()
	if err != nil {
		runtimeErr := websocketRelayError(ctx, err)
		if runtimeErr.Class != "" {
			s.recordError(runtimeErr.Class)
			return result, runtimeErr
		}
		s.recordSuccess()
		return result, nil
	}
	s.recordSuccess()
	return result, nil
}

func (s *Service) Refresh(ctx context.Context, req contract.RefreshRequest) (contract.RefreshResponse, error) {
	if req.Account.AccountID > 0 {
		lock := s.refreshLock(req.Account.AccountID)
		lock.Lock()
		defer lock.Unlock()
	}
	return s.refreshOAuthCredential(ctx, req)
}

func (s *Service) refreshOAuthCredential(ctx context.Context, req contract.RefreshRequest) (contract.RefreshResponse, error) {
	if !supportsOAuthRefresh(req.Account.RuntimeClass) {
		s.recordRefresh("invalid_request")
		return contract.RefreshResponse{}, ErrInvalidInput
	}
	refreshToken := credentialString(req.Account.Credential, "refresh_token")
	if refreshToken == "" {
		s.recordRefresh("credential_missing")
		return contract.RefreshResponse{}, contract.RuntimeError{Class: "credential_missing", StatusCode: http.StatusBadRequest, Message: "oauth refresh token missing"}
	}
	config := oauthRefreshSettings(req.Account)
	if config.TokenEndpoint == "" || config.ClientID == "" {
		s.recordRefresh("credential_missing")
		return contract.RefreshResponse{}, contract.RuntimeError{Class: "credential_missing", StatusCode: http.StatusBadRequest, Message: "oauth refresh configuration missing"}
	}
	if oauthRefreshRequiresClientSecret(req.Account) && config.ClientSecret == "" {
		s.recordRefresh("credential_missing")
		return contract.RefreshResponse{}, contract.RuntimeError{Class: "credential_missing", StatusCode: http.StatusBadRequest, Message: "oauth client secret missing"}
	}
	httpReq, err := newOAuthRefreshHTTPRequest(ctx, config, refreshToken)
	if err != nil {
		s.recordRefresh("invalid_request")
		return contract.RefreshResponse{}, err
	}
	profile, err := resolveEgressProfile(req.Account)
	if err != nil {
		s.recordRefresh(errorClass(err))
		return contract.RefreshResponse{}, err
	}
	if err := validateEgressTargetURL(config.TokenEndpoint, profile); err != nil {
		s.recordRefresh(errorClass(err))
		return contract.RefreshResponse{}, err
	}
	if ua := userAgentForProfile(req.Account, profile); ua != "" {
		httpReq.Header.Set("User-Agent", ua)
	}
	client, err := s.clientFor(req.Account)
	if err != nil {
		s.recordRefresh(errorClass(err))
		return contract.RefreshResponse{}, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		status := "network_error"
		statusCode := http.StatusBadGateway
		message := "oauth refresh request failed"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = "timeout"
			statusCode = http.StatusGatewayTimeout
			message = "oauth refresh request timed out"
		}
		s.recordRefresh(status)
		return contract.RefreshResponse{}, contract.RuntimeError{Class: status, StatusCode: statusCode, Message: message}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReverseProxyResponseBytes))
	if err != nil {
		s.recordRefresh("network_error")
		return contract.RefreshResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		runtimeErr := classifyOAuthRefreshError(resp.StatusCode, body)
		s.recordRefresh(runtimeErr.Class)
		return contract.RefreshResponse{}, runtimeErr
	}
	refreshed, refreshedAt, err := mergeOAuthTokenResponse(req.Account.Credential, body)
	if err != nil {
		s.recordRefresh("invalid_response")
		return contract.RefreshResponse{}, err
	}
	s.recordRefresh("success")
	return contract.RefreshResponse{AccountID: req.Account.AccountID, Credential: refreshed, RefreshedAt: refreshedAt}, nil
}

type webSocketRelayStats struct {
	mu                 sync.Mutex
	messagesUpstream   int
	messagesDownstream int
	bytesUpstream      int
	bytesDownstream    int
}

func (s *webSocketRelayStats) recordUpstream(bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesUpstream++
	s.bytesUpstream += bytes
}

func (s *webSocketRelayStats) recordDownstream(bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesDownstream++
	s.bytesDownstream += bytes
}

func (s *webSocketRelayStats) snapshot() webSocketRelayStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return webSocketRelayStats{
		messagesUpstream:   s.messagesUpstream,
		messagesDownstream: s.messagesDownstream,
		bytesUpstream:      s.bytesUpstream,
		bytesDownstream:    s.bytesDownstream,
	}
}

type webSocketRelayDirection string

const (
	webSocketRelayClient   webSocketRelayDirection = "client"
	webSocketRelayUpstream webSocketRelayDirection = "upstream"
)

type webSocketRelayDone struct {
	direction webSocketRelayDirection
	err       error
}

func relayWebSocketMessages(ctx context.Context, conn *websocket.Conn, clientToUpstream <-chan contract.WebSocketMessage, upstreamToClient chan<- contract.WebSocketMessage, stats *webSocketRelayStats) error {
	doneCh := make(chan webSocketRelayDone, 2)
	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		doneCh <- webSocketRelayDone{direction: webSocketRelayClient, err: relayClientWebSocketMessages(relayCtx, conn, clientToUpstream, stats)}
	}()
	go func() {
		doneCh <- webSocketRelayDone{direction: webSocketRelayUpstream, err: relayUpstreamWebSocketMessages(relayCtx, conn, upstreamToClient, stats)}
	}()
	first := <-doneCh
	if first.err != nil || first.direction == webSocketRelayUpstream {
		cancel()
		<-doneCh
		return first.err
	}
	second := <-doneCh
	return second.err
}

func relayClientWebSocketMessages(ctx context.Context, conn *websocket.Conn, in <-chan contract.WebSocketMessage, stats *webSocketRelayStats) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-in:
			if !ok {
				return nil
			}
			if err := conn.Write(ctx, websocketMessageType(msg.Type), msg.Data); err != nil {
				return err
			}
			stats.recordUpstream(len(msg.Data))
		}
	}
}

func relayUpstreamWebSocketMessages(ctx context.Context, conn *websocket.Conn, out chan<- contract.WebSocketMessage, stats *webSocketRelayStats) error {
	defer close(out)
	for {
		msgType, payload, err := conn.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				return nil
			}
			return err
		}
		msg := contract.WebSocketMessage{Type: contractWebSocketMessageType(msgType), Data: append([]byte(nil), payload...)}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- msg:
		}
		stats.recordDownstream(len(payload))
	}
}

func websocketMessageType(value contract.WebSocketMessageType) websocket.MessageType {
	switch value {
	case contract.WebSocketMessageBinary:
		return websocket.MessageBinary
	default:
		return websocket.MessageText
	}
}

func contractWebSocketMessageType(value websocket.MessageType) contract.WebSocketMessageType {
	switch value {
	case websocket.MessageBinary:
		return contract.WebSocketMessageBinary
	default:
		return contract.WebSocketMessageText
	}
}

func websocketRelayError(ctx context.Context, err error) contract.RuntimeError {
	if err == nil {
		return contract.RuntimeError{}
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return contract.RuntimeError{Class: "client_closed", StatusCode: statusClientClosedRequest, Message: "websocket relay closed"}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return contract.RuntimeError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "websocket relay timed out"}
	}
	status := websocket.CloseStatus(err)
	if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
		return contract.RuntimeError{}
	}
	return contract.RuntimeError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "websocket relay failed"}
}

func (s *Service) Metrics() contract.MetricsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return contract.MetricsSnapshot{
		RequestTotal:        s.requestTotal,
		RequestSuccessTotal: s.successTotal,
		RequestErrorTotal:   cloneIntMap(s.errorTotal),
		ChallengeTotal:      cloneIntMap(s.challengeTotal),
		AccountLockedTotal:  s.lockedTotal,
		AccountBannedTotal:  s.bannedTotal,
		OAuthRefreshTotal:   cloneIntMap(s.refreshTotal),
	}
}

func (s *Service) clientFor(account contract.AccountRuntime) (*http.Client, error) {
	cacheKey, err := clientCacheKey(account)
	if err != nil {
		return nil, err
	}
	if account.AccountID <= 0 {
		if cacheKey != defaultClientCacheKey {
			return s.factory(account)
		}
		return s.defaultClient, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if client, ok := s.clients[cacheKey]; ok {
		return client, nil
	}
	client, err := s.factory(account)
	if err != nil {
		return nil, err
	}
	s.clients[cacheKey] = client
	return client, nil
}

// ManagedEgressClient returns the per-account egress client (proxy + uTLS
// fingerprint + SSRF guard) when the account is configured for managed egress,
// reusing the same cached client as live reverse-proxy traffic. It reports false
// for accounts with no egress configuration so the caller keeps its own client.
func (s *Service) ManagedEgressClient(account contract.AccountRuntime) (*http.Client, bool, error) {
	if !accountHasManagedEgress(account) {
		return nil, false, nil
	}
	client, err := s.clientFor(account)
	if err != nil {
		return nil, false, err
	}
	return client, true, nil
}

// accountHasManagedEgress reports whether the account configures transport-level
// egress (an explicit proxy or a uTLS/HTTP-version egress profile). Static header
// or user-agent overrides are applied by Do, not by the client, so they do not
// imply a managed transport here.
func accountHasManagedEgress(account contract.AccountRuntime) bool {
	if accountHasProxy(account) {
		return true
	}
	profile, err := resolveEgressProfile(account)
	if err != nil {
		return false
	}
	return profile.TLSTemplate != "" || profile.requiresHTTP1()
}

func (s *Service) refreshLock(accountID int) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, ok := s.refreshLocks[accountID]
	if !ok {
		lock = &sync.Mutex{}
		s.refreshLocks[accountID] = lock
	}
	return lock
}

func newIsolatedClient(account contract.AccountRuntime, blockPrivateEgress bool) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxyFunc(account.ProxyID)
	transport.DisableCompression = true
	// Screen direct (non-proxied) dials against private networks. A configured
	// egress proxy is operator-trusted and routed without screening.
	guardPrivateEgress := blockPrivateEgress && !accountHasProxy(account)
	if guardPrivateEgress {
		transport.DialContext = egressDialer(true).DialContext
	}
	profile, err := resolveEgressProfile(account)
	if err != nil {
		return nil, err
	}
	if err := configureTransportForEgress(transport, account, profile, guardPrivateEgress); err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   30 * time.Second,
	}, nil
}

func proxyFunc(proxyID *string) func(*http.Request) (*url.URL, error) {
	if proxyID == nil || strings.TrimSpace(*proxyID) == "" {
		return http.ProxyFromEnvironment
	}
	raw := strings.TrimSpace(*proxyID)
	return func(*http.Request) (*url.URL, error) {
		if strings.Contains(raw, "://") {
			return url.Parse(raw)
		}
		return nil, nil
	}
}

func sanitizeHeaders(headers http.Header) http.Header {
	return sanitizeHeadersForProfile(headers, egressProfile{})
}

type sanitizeHeaderOptions struct {
	AllowStainlessHeaders bool
}

func sanitizeHeaderOptionsForAccount(account contract.AccountRuntime) sanitizeHeaderOptions {
	return sanitizeHeaderOptions{
		AllowStainlessHeaders: upstreamClientIs(account, "claude_code_cli"),
	}
}

func sanitizeHeadersForProfile(headers http.Header, profile egressProfile, opts ...sanitizeHeaderOptions) http.Header {
	options := sanitizeHeaderOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	out := http.Header{}
	for key, values := range headers {
		if forbiddenHeader(key, values, options) || profile.forbidsHeader(key) {
			continue
		}
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}

func forbiddenHeader(key string, values []string, opts ...sanitizeHeaderOptions) bool {
	options := sanitizeHeaderOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	canonical := http.CanonicalHeaderKey(strings.TrimSpace(key))
	lower := strings.ToLower(canonical)
	if lower == "x-request-id" || lower == "x-forwarded-for" || lower == "x-forwarded-host" || lower == "x-forwarded-proto" || lower == "x-forwarded-port" || lower == "x-real-ip" || lower == "forwarded" || lower == "via" || lower == "server" {
		return true
	}
	if lower == "authorization" || lower == "cookie" {
		return true
	}
	if lower == "host" || lower == "connection" || lower == "keep-alive" || lower == "upgrade" || lower == "te" || lower == "trailer" || lower == "transfer-encoding" || lower == "proxy-authorization" || lower == "proxy-authenticate" || lower == "proxy-connection" {
		return true
	}
	if lower == "accept-encoding" || lower == "http-referer" || lower == "referer" || lower == "priority" || lower == "x-title" {
		return true
	}
	if strings.HasPrefix(lower, "sec-websocket-") {
		return true
	}
	if strings.HasPrefix(lower, "sec-ch-") || strings.HasPrefix(lower, "sec-fetch-") {
		return true
	}
	if strings.HasPrefix(lower, "x-stainless-") && !options.AllowStainlessHeaders {
		return true
	}
	if strings.HasPrefix(lower, "x-srapi-") || strings.HasPrefix(lower, "x-gateway-") {
		return true
	}
	if lower == "user-agent" {
		for _, value := range values {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "srapi/") {
				return true
			}
		}
	}
	return false
}

func injectAuth(headers http.Header, account contract.AccountRuntime) {
	switch account.RuntimeClass {
	case "api_key":
		return
	case "cli_client_token":
		if token := firstCredentialString(account.Credential, "cli_client_token", "cli_token", "device_token", "access_token"); token != "" {
			headers.Set("Authorization", "Bearer "+token)
		}
	case "oauth_refresh", "oauth_device_code":
		if token := credentialString(account.Credential, "access_token"); token != "" {
			headers.Set("Authorization", "Bearer "+token)
		}
	case "web_session_cookie":
		headers.Del("Authorization")
		if cookie := credentialString(account.Credential, "cookie"); cookie != "" {
			headers.Set("Cookie", cookie)
		}
	}
}

func userAgent(account contract.AccountRuntime) string {
	profile, _ := resolveEgressProfile(account)
	return userAgentForProfile(account, profile)
}

func userAgentForProfile(account contract.AccountRuntime, profile egressProfile) string {
	if strings.TrimSpace(account.UserAgent) != "" {
		return strings.TrimSpace(account.UserAgent)
	}
	if value := credentialString(account.Credential, "user_agent"); value != "" && !strings.HasPrefix(strings.ToLower(value), "srapi/") {
		return value
	}
	if profile.UserAgent != "" {
		return profile.UserAgent
	}
	if value := credentialString(account.Metadata, "user_agent"); value != "" && !strings.HasPrefix(strings.ToLower(value), "srapi/") {
		return value
	}
	if account.UpstreamClient != nil && strings.TrimSpace(*account.UpstreamClient) != "" {
		if ua := defaultUserAgentForUpstreamClient(*account.UpstreamClient); ua != "" {
			return ua
		}
		return strings.TrimSpace(*account.UpstreamClient)
	}
	return "Mozilla/5.0"
}

func firstCredentialString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := credentialString(values, key); value != "" {
			return value
		}
	}
	return ""
}

func defaultUserAgentForUpstreamClient(upstreamClient string) string {
	switch strings.ToLower(strings.TrimSpace(upstreamClient)) {
	case "codex_cli":
		return "Codex/1.0"
	case "claude_code_cli":
		return "Claude-Code/1.0"
	case "gemini_cli":
		return "Gemini-CLI/1.0"
	case "antigravity_desktop", "antigravity":
		return "Antigravity/1.0"
	default:
		return ""
	}
}

func guardBody(body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if containsForbiddenBodyKey(payload) {
		return contract.RuntimeError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy body contains SRapi internal fields"}
	}
	return nil
}

func containsForbiddenBodyKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, val := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			if lower == "request_id" || lower == "compatibility_warnings" || lower == "srapi" || strings.HasPrefix(lower, "srapi_") {
				return true
			}
			if lower == "metadata" && containsForbiddenMetadata(val) {
				return true
			}
			if containsForbiddenBodyKey(val) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsForbiddenBodyKey(item) {
				return true
			}
		}
	}
	return false
}

func containsForbiddenMetadata(value any) bool {
	typed, ok := value.(map[string]any)
	if !ok {
		return containsForbiddenBodyKey(value)
	}
	for key, val := range typed {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "srapi" || lower == "request_id" || lower == "compatibility_warnings" || strings.HasPrefix(lower, "srapi_") {
			return true
		}
		if containsForbiddenMetadata(val) {
			return true
		}
	}
	return false
}

func classifyRuntimeError(statusCode int, body []byte) contract.RuntimeError {
	message := strings.TrimSpace(string(body))
	lower := strings.ToLower(extractErrorText(body))
	class := "upstream_error"
	switch {
	case strings.Contains(lower, "challenge_required"):
		class = "challenge_required"
	case strings.Contains(lower, "captcha_required"):
		class = "captcha_required"
	case strings.Contains(lower, "session_invalid") || strings.Contains(lower, "invalid session"):
		class = "session_invalid"
	case strings.Contains(lower, "account_locked"):
		class = "account_locked"
	case strings.Contains(lower, "account_banned"):
		class = "account_banned"
	case strings.Contains(lower, "abuse_detected"):
		class = "abuse_detected"
	case strings.Contains(lower, "geo_blocked"):
		class = "geo_blocked"
	case strings.Contains(lower, "device_unrecognized"):
		class = "device_unrecognized"
	case strings.Contains(lower, "upstream_client_outdated"):
		class = "upstream_client_outdated"
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		class = "session_invalid"
	case statusCode == http.StatusTooManyRequests:
		class = "rate_limit"
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		class = "timeout"
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return contract.RuntimeError{Class: class, StatusCode: statusCode, Message: message}
}

func extractErrorText(body []byte) string {
	message := strings.TrimSpace(string(body))
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return message
	}
	values := make([]string, 0, 4)
	collectErrorText(payload, &values)
	if len(values) == 0 {
		return message
	}
	return strings.Join(values, " ")
}

func collectErrorText(value any, out *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, val := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			if lower == "error" || lower == "code" || lower == "type" || lower == "message" || lower == "error_code" {
				collectErrorText(val, out)
			}
		}
	case []any:
		for _, item := range typed {
			collectErrorText(item, out)
		}
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			*out = append(*out, trimmed)
		}
	case json.Number:
		*out = append(*out, typed.String())
	case float64:
		*out = append(*out, strconv.FormatFloat(typed, 'f', -1, 64))
	case bool:
		*out = append(*out, strconv.FormatBool(typed))
	}
}

func credentialString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return strings.TrimSpace(strings.ReplaceAll(fmt.Sprint(value), "\n", " "))
	}
}

func accountSetting(account contract.AccountRuntime, keys ...string) string {
	for _, values := range []map[string]any{account.Credential, account.Metadata} {
		for _, key := range keys {
			if value := credentialString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func supportsOAuthRefresh(runtimeClass string) bool {
	switch strings.ToLower(strings.TrimSpace(runtimeClass)) {
	case "oauth_refresh", "oauth_device_code":
		return true
	default:
		return false
	}
}

func oauthRefreshSettings(account contract.AccountRuntime) oauthRefreshConfig {
	tokenEndpoint := accountSetting(account, "oauth_token_url", "token_url", "oauth_refresh_url", "refresh_url")
	clientID := accountSetting(account, "oauth_client_id", "client_id")
	scope := accountSetting(account, "oauth_scope", "scope")
	config := oauthRefreshConfig{
		TokenEndpoint: tokenEndpoint,
		ClientID:      clientID,
		ClientSecret:  credentialString(account.Credential, "oauth_client_secret"),
		Scope:         scope,
		Encoding:      oauthRefreshEncodingForm,
	}
	if config.ClientSecret == "" {
		config.ClientSecret = credentialString(account.Credential, "client_secret")
	}
	switch {
	case upstreamClientIs(account, "codex_cli"):
		if config.TokenEndpoint == "" {
			config.TokenEndpoint = contract.CodexOAuthTokenURL
		}
		if config.ClientID == "" {
			config.ClientID = contract.CodexOAuthClientID
		}
		if config.Scope == "" {
			config.Scope = contract.CodexOAuthRefreshScope
		}
	case upstreamClientIs(account, "claude_code_cli"):
		if config.TokenEndpoint == "" {
			config.TokenEndpoint = contract.ClaudeCodeOAuthTokenURL
		}
		if config.ClientID == "" {
			config.ClientID = contract.ClaudeCodeOAuthClientID
		}
		config.Encoding = oauthRefreshEncodingJSON
	case upstreamClientIs(account, "antigravity_desktop") || upstreamClientIs(account, "antigravity"):
		if config.TokenEndpoint == "" {
			config.TokenEndpoint = contract.AntigravityOAuthTokenURL
		}
		if config.ClientID == "" {
			config.ClientID = contract.AntigravityOAuthClientID
		}
	}
	if encoding := strings.ToLower(accountSetting(account, "oauth_request_encoding", "oauth_encoding", "token_request_encoding")); encoding != "" {
		config.Encoding = encoding
	}
	return config
}

func newOAuthRefreshHTTPRequest(ctx context.Context, config oauthRefreshConfig, refreshToken string) (*http.Request, error) {
	if strings.EqualFold(config.Encoding, oauthRefreshEncodingJSON) {
		payload := map[string]any{
			"client_id":     config.ClientID,
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
		}
		if config.ClientSecret != "" {
			payload["client_secret"] = config.ClientSecret
		}
		if config.Scope != "" {
			payload["scope"] = config.Scope
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenEndpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	form := url.Values{
		"client_id":     {config.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	if config.ClientSecret != "" {
		form.Set("client_secret", config.ClientSecret)
	}
	if config.Scope != "" {
		form.Set("scope", config.Scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func upstreamClientIs(account contract.AccountRuntime, expected string) bool {
	if account.UpstreamClient == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*account.UpstreamClient), expected)
}

func oauthRefreshRequiresClientSecret(account contract.AccountRuntime) bool {
	return upstreamClientIs(account, "antigravity_desktop") || upstreamClientIs(account, "antigravity")
}

func classifyOAuthRefreshError(statusCode int, body []byte) contract.RuntimeError {
	message := strings.TrimSpace(extractErrorText(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	lower := strings.ToLower(message)
	class := "auth_failed"
	switch {
	case strings.Contains(lower, "invalid_grant"), strings.Contains(lower, "refresh_token_reused"), strings.Contains(lower, "invalid refresh"):
		class = "session_invalid"
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		class = "auth_failed"
	case statusCode == http.StatusTooManyRequests:
		class = "rate_limit"
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		class = "timeout"
	case statusCode >= 500:
		class = "upstream_error"
	}
	return contract.RuntimeError{Class: class, StatusCode: statusCode, Message: message}
}

func mergeOAuthTokenResponse(existing map[string]any, body []byte) (map[string]any, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", contract.RuntimeError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "oauth refresh returned invalid json"}
	}
	accessToken := credentialString(payload, "access_token")
	if accessToken == "" {
		return nil, "", contract.RuntimeError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "oauth refresh response missing access token"}
	}
	now := time.Now().UTC()
	refreshed := cloneCredential(existing)
	refreshed["access_token"] = accessToken
	if refreshToken := credentialString(payload, "refresh_token"); refreshToken != "" {
		refreshed["refresh_token"] = refreshToken
	}
	if idToken := credentialString(payload, "id_token"); idToken != "" {
		refreshed["id_token"] = idToken
	}
	if tokenType := credentialString(payload, "token_type"); tokenType != "" {
		refreshed["token_type"] = tokenType
	}
	if expiresIn := tokenExpiresIn(payload["expires_in"]); expiresIn > 0 {
		refreshed["expires_at"] = now.Add(expiresIn).Format(time.RFC3339)
	}
	refreshedAt := now.Format(time.RFC3339)
	refreshed["refreshed_at"] = refreshedAt
	return refreshed, refreshedAt, nil
}

func tokenExpiresIn(value any) time.Duration {
	switch typed := value.(type) {
	case json.Number:
		parsed, _ := typed.Int64()
		return time.Duration(parsed) * time.Second
	case int:
		return time.Duration(typed) * time.Second
	case int64:
		return time.Duration(typed) * time.Second
	case float64:
		return time.Duration(typed) * time.Second
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return time.Duration(parsed) * time.Second
	default:
		return 0
	}
}

func cloneHeaders(headers http.Header) http.Header {
	out := http.Header{}
	for key, values := range headers {
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}

func cloneCredential(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func errorClass(err error) string {
	var runtimeErr contract.RuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr.Class != "" {
		return runtimeErr.Class
	}
	return "unknown"
}

func (s *Service) recordRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestTotal++
}

func (s *Service) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.successTotal++
}

func (s *Service) recordError(class string) {
	if strings.TrimSpace(class) == "" {
		class = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorTotal[class]++
	switch class {
	case "challenge_required", "captcha_required":
		s.challengeTotal[class]++
	case "account_locked":
		s.lockedTotal++
	case "account_banned":
		s.bannedTotal++
	}
}

func (s *Service) recordRefresh(status string) {
	if strings.TrimSpace(status) == "" {
		status = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshTotal[status]++
}

func cloneIntMap(values map[string]int) map[string]int {
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
