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
)

const maxReverseProxyResponseBytes = 8 << 20

type ClientFactory func(account contract.AccountRuntime) (*http.Client, error)

type Service struct {
	mu             sync.Mutex
	clients        map[int]*http.Client
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

func New(factory ClientFactory) (*Service, error) {
	defaultClient, err := newIsolatedClient(contract.AccountRuntime{})
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = newIsolatedClient
	}
	return &Service{
		clients:        map[int]*http.Client{},
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
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		s.recordError("invalid_request")
		return contract.Response{}, err
	}
	httpReq.Header = sanitizeHeaders(req.Headers)
	if len(req.Body) > 0 && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	injectAuth(httpReq.Header, req.Account)
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", userAgent(req.Account))
	}

	client, err := s.clientFor(req.Account)
	if err != nil {
		s.recordError("network_error")
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

func (s *Service) Refresh(ctx context.Context, req contract.RefreshRequest) (contract.RefreshResponse, error) {
	if req.Account.AccountID <= 0 {
		s.recordRefresh("invalid_request")
		return contract.RefreshResponse{}, ErrInvalidInput
	}
	lock := s.refreshLock(req.Account.AccountID)
	lock.Lock()
	defer lock.Unlock()

	refreshToken := credentialString(req.Account.Credential, "refresh_token")
	if refreshToken == "" {
		s.recordRefresh("credential_missing")
		return contract.RefreshResponse{}, contract.RuntimeError{Class: "credential_missing", StatusCode: http.StatusBadRequest, Message: "oauth refresh token missing"}
	}
	refreshed := cloneCredential(req.Account.Credential)
	refreshed["access_token"] = refreshToken
	now := time.Now().UTC().Format(time.RFC3339)
	refreshed["refreshed_at"] = now
	_ = ctx
	s.recordRefresh("success")
	return contract.RefreshResponse{AccountID: req.Account.AccountID, Credential: refreshed, RefreshedAt: now}, nil
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
	if account.AccountID <= 0 {
		return s.defaultClient, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if client, ok := s.clients[account.AccountID]; ok {
		return client, nil
	}
	client, err := s.factory(account)
	if err != nil {
		return nil, err
	}
	s.clients[account.AccountID] = client
	return client, nil
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

func newIsolatedClient(account contract.AccountRuntime) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxyFunc(account.ProxyID)
	transport.DisableCompression = true
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
	out := http.Header{}
	for key, values := range headers {
		if forbiddenHeader(key, values) {
			continue
		}
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}

func forbiddenHeader(key string, values []string) bool {
	canonical := http.CanonicalHeaderKey(strings.TrimSpace(key))
	lower := strings.ToLower(canonical)
	if lower == "x-request-id" || lower == "x-forwarded-for" || lower == "x-forwarded-host" || lower == "x-forwarded-proto" || lower == "forwarded" || lower == "via" || lower == "server" {
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
	case "cli_client_token":
		if token := firstCredentialString(account.Credential, "cli_client_token", "cli_token", "device_token", "access_token"); token != "" {
			headers.Set("Authorization", "Bearer "+token)
		}
	case "oauth_refresh", "oauth_device_code", "desktop_client_token", "ide_plugin_token":
		if token := credentialString(account.Credential, "access_token"); token != "" {
			headers.Set("Authorization", "Bearer "+token)
		}
	case "web_session_cookie":
		headers.Del("Authorization")
		if cookie := credentialString(account.Credential, "cookie"); cookie != "" {
			headers.Set("Cookie", cookie)
		}
	default:
		if token := credentialString(account.Credential, "access_token"); token != "" {
			headers.Set("Authorization", "Bearer "+token)
		}
	}
}

func userAgent(account contract.AccountRuntime) string {
	if strings.TrimSpace(account.UserAgent) != "" {
		return strings.TrimSpace(account.UserAgent)
	}
	if value := credentialString(account.Credential, "user_agent"); value != "" && !strings.HasPrefix(strings.ToLower(value), "srapi/") {
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
