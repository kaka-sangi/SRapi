package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	totpservice "github.com/srapi/srapi/apps/api/internal/modules/totp/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	platformlogger "github.com/srapi/srapi/apps/api/internal/platform/logger"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

var (
	errGatewayConcurrencyLimited = errors.New("gateway concurrency limit exceeded")
)

type gatewayConcurrencyLimitError struct {
	decision ratelimit.Decision
}

func (e gatewayConcurrencyLimitError) Error() string {
	return errGatewayConcurrencyLimited.Error()
}

func (e gatewayConcurrencyLimitError) Unwrap() error {
	return errGatewayConcurrencyLimited
}

type gatewayConcurrencyState struct {
	mu       sync.Mutex
	apiKeyID int
	acquired bool
	lease    ratelimit.ConcurrencyLease
	released bool
}

func (s *gatewayConcurrencyState) hasLeaseFor(apiKeyID int) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acquired && !s.released && s.apiKeyID == apiKeyID
}

func (s *gatewayConcurrencyState) storeLease(apiKeyID int, lease ratelimit.ConcurrencyLease) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKeyID = apiKeyID
	s.lease = lease
	s.acquired = true
	s.released = false
}

func (s *gatewayConcurrencyState) releaseLease() (ratelimit.ConcurrencyLease, bool) {
	if s == nil {
		return ratelimit.ConcurrencyLease{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.acquired || s.released {
		return ratelimit.ConcurrencyLease{}, false
	}
	s.released = true
	return s.lease, true
}

func sanitizedExportMetadata(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		if sensitiveMetadataKey(key) {
			continue
		}
		out[key] = sanitizeExportMetadataValue(item)
	}
	return out
}

func sanitizeExportMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizedExportMetadata(typed)
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for idx, item := range typed {
			out[idx] = sanitizedExportMetadata(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = sanitizeExportMetadataValue(item)
		}
		return out
	default:
		return typed
	}
}

func sensitiveMetadataKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.NewReplacer("-", "_", " ", "_").Replace(key)
	if key == "key" || strings.HasSuffix(key, "_key") {
		return true
	}
	sensitiveMarkers := []string{
		"authorization",
		"bearer",
		"cookie",
		"credential",
		"password",
		"passwd",
		"private_key",
		"secret",
		"session_affinity_key",
		"token",
	}
	for _, marker := range sensitiveMarkers {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, len(values))
	for idx, value := range values {
		out[idx] = cloneAnyMap(value)
	}
	return out
}

func cloneAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneAnyValue(item)
	}
	return out
}

func cloneBoolMap(value map[string]bool) map[string]bool {
	if value == nil {
		return nil
	}
	out := make(map[string]bool, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func cloneFloat32Map(value map[string]float32) map[string]float32 {
	if value == nil {
		return nil
	}
	out := make(map[string]float32, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func cloneStringSliceMap(value map[string][]string) map[string][]string {
	if value == nil {
		return nil
	}
	out := make(map[string][]string, len(value))
	for key, item := range value {
		out[key] = append([]string(nil), item...)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []map[string]any:
		return cloneMapSlice(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAnyValue(item)
		}
		return out
	default:
		return typed
	}
}

func elapsedMillis(startedAt time.Time) int {
	return max(0, int(time.Since(startedAt).Milliseconds()))
}

func fallbackModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "unknown"
	}
	return model
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) requireConsoleSession(r *http.Request) (authcontract.LoginResult, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return authcontract.LoginResult{}, authservice.ErrSessionNotFound
	}
	session, err := s.runtime.auth.AuthenticateSession(r.Context(), cookie.Value)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	*r = *r.WithContext(platformlogger.WithUserID(r.Context(), session.User.ID))
	return session, nil
}

func (s *Server) requireAdminSession(r *http.Request) (authcontract.LoginResult, error) {
	session, err := s.requireConsoleSession(r)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	if userHasAdminSurfaceAccess(session.User) {
		return session, nil
	}
	return authcontract.LoginResult{}, errors.New("admin access required")
}

func (s *Server) requireAdminPermission(r *http.Request, permission string) (authcontract.LoginResult, error) {
	session, err := s.requireConsoleSession(r)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	if !userscontract.IsKnownPermission(permission) {
		return authcontract.LoginResult{}, errors.New("unknown permission")
	}
	for _, role := range session.User.Roles {
		if role == userscontract.RoleOwner {
			return session, nil
		}
	}
	for _, granted := range session.User.Permissions {
		if granted == permission {
			return session, nil
		}
	}
	return authcontract.LoginResult{}, errors.New("permission required")
}

func userHasAdminSurfaceAccess(user userscontract.User) bool {
	for _, role := range user.Roles {
		switch role {
		case userscontract.RoleOwner, userscontract.RoleAdmin, userscontract.RoleOperator:
			return true
		}
	}
	for _, permission := range user.Permissions {
		if userscontract.IsKnownPermission(permission) {
			return true
		}
	}
	return false
}

func (s *Server) requireGatewayKey(r *http.Request) (apikeycontract.AuthResult, error) {
	apiKey, ok := bearerGatewayAPIKey(r)
	if !ok {
		if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
			s.recordGatewayAuthFailure(r, "", apikeyservice.ErrInvalidKey)
		}
		return apikeycontract.AuthResult{}, apikeyservice.ErrInvalidKey
	}
	return s.requireGatewayKeyPlaintextWithAuthFailureLog(r, apiKey)
}

func (s *Server) requireGeminiGatewayKey(r *http.Request) (apikeycontract.AuthResult, error) {
	if apiKey := strings.TrimSpace(r.Header.Get("x-goog-api-key")); apiKey != "" {
		return s.requireGatewayKeyPlaintextWithAuthFailureLog(r, apiKey)
	}
	if apiKey, ok := bearerGatewayAPIKey(r); ok {
		return s.requireGatewayKeyPlaintextWithAuthFailureLog(r, apiKey)
	}
	if apiKey := strings.TrimSpace(r.Header.Get("x-api-key")); apiKey != "" {
		return s.requireGatewayKeyPlaintextWithAuthFailureLog(r, apiKey)
	}
	if allowsGeminiQueryAPIKey(r.URL.Path) {
		if apiKey := strings.TrimSpace(r.URL.Query().Get("key")); apiKey != "" {
			return s.requireGatewayKeyPlaintextWithAuthFailureLog(r, apiKey)
		}
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		s.recordGatewayAuthFailure(r, "", apikeyservice.ErrInvalidKey)
	}
	return apikeycontract.AuthResult{}, apikeyservice.ErrInvalidKey
}

func bearerGatewayAPIKey(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func allowsGeminiQueryAPIKey(path string) bool {
	return path == "/v1beta/models" || strings.HasPrefix(path, "/v1beta/models/")
}

func (s *Server) requireGatewayKeyPlaintextWithAuthFailureLog(r *http.Request, apiKey string) (apikeycontract.AuthResult, error) {
	authed, err := s.requireGatewayKeyPlaintext(r, apiKey)
	if err != nil {
		s.recordGatewayAuthFailure(r, apiKey, err)
	}
	return authed, err
}

func (s *Server) recordGatewayAuthFailure(r *http.Request, plaintext string, err error) {
	if s == nil || s.runtime == nil {
		return
	}
	s.runtime.recordGatewayAuthFailure(r.Context(), gatewayAuthFailureRecord{
		RequestID:          requestIDFromContext(r.Context()),
		SourceEndpoint:     r.URL.Path,
		Method:             r.Method,
		Reason:             gatewayAuthFailureReason(err),
		AttemptedKeyPrefix: gatewayAttemptedKeyPrefix(plaintext, err),
		DeletedKey:         s.gatewayDeletedKeyMatch(r, plaintext, err),
	})
}

type gatewayAuthFailureRecord struct {
	RequestID          string
	SourceEndpoint     string
	Method             string
	Reason             string
	AttemptedKeyPrefix string
	DeletedKey         *apikeycontract.DeletedKeyMatch
}

func (rt *runtimeState) recordGatewayAuthFailure(ctx context.Context, rec gatewayAuthFailureRecord) {
	if rt == nil || rt.operations == nil {
		return
	}
	metadata := map[string]any{
		"request_id":      rec.RequestID,
		"source_endpoint": rec.SourceEndpoint,
		"method":          rec.Method,
		"reason":          rec.Reason,
	}
	if rec.AttemptedKeyPrefix != "" {
		metadata["attempted_key_prefix"] = rec.AttemptedKeyPrefix
	}
	if rec.DeletedKey != nil {
		metadata["deleted_key_id"] = rec.DeletedKey.KeyID
		metadata["deleted_key_owner_user_id"] = rec.DeletedKey.UserID
		metadata["deleted_key_name"] = rec.DeletedKey.Name
	}
	if _, err := rt.operations.RecordSystemLog(ctx, operationscontract.RecordSystemLogRequest{
		Level:     operationscontract.OpsSystemLogLevelWarn,
		Message:   "gateway API key authentication failed",
		Source:    "gateway.auth",
		RequestID: rec.RequestID,
		TraceID:   traceIDFromContext(ctx),
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	}); err != nil && rt.logger != nil {
		rt.logger.Warn("operations RecordSystemLog failed", "request_id", rec.RequestID, "error", err)
	}
}

func gatewayAuthFailureReason(err error) string {
	var concurrencyErr gatewayConcurrencyLimitError
	switch {
	case errors.As(err, &concurrencyErr):
		return "concurrency_limit_exceeded"
	case errors.Is(err, errGatewayKeyIPNotAllowed):
		return "ip_not_allowed"
	case errors.Is(err, errGatewayRiskControlBlocked):
		return "risk_control_blocked"
	case errors.Is(err, apikeyservice.ErrKeyDisabled):
		return "api_key_disabled"
	case errors.Is(err, apikeyservice.ErrKeyExpired):
		return "api_key_expired"
	case errors.Is(err, apikeyservice.ErrInvalidKey), errors.Is(err, apikeyservice.ErrInvalidInput):
		return "invalid_api_key"
	default:
		return "internal_error"
	}
}

func gatewayAttemptedKeyPrefix(plaintext string, err error) string {
	var concurrencyErr gatewayConcurrencyLimitError
	if errors.Is(err, errGatewayRiskControlBlocked) || errors.As(err, &concurrencyErr) {
		return ""
	}
	if !gatewayAuthFailureReasonAllowsPrefix(gatewayAuthFailureReason(err)) {
		return ""
	}
	prefix, ok := apikeyservice.PrefixFromPlaintext(strings.TrimSpace(plaintext))
	if !ok {
		return ""
	}
	return prefix
}

func (s *Server) gatewayDeletedKeyMatch(r *http.Request, plaintext string, err error) *apikeycontract.DeletedKeyMatch {
	if s == nil || s.runtime == nil || s.runtime.apiKeys == nil || !errors.Is(err, apikeyservice.ErrInvalidKey) {
		return nil
	}
	match, ok, lookupErr := s.runtime.apiKeys.DeletedKeyMatchFromPlaintext(r.Context(), plaintext)
	if lookupErr != nil {
		if s.runtime.logger != nil {
			s.runtime.logger.Warn("deleted api key tombstone lookup failed", "request_id", requestIDFromContext(r.Context()), "error", lookupErr)
		}
		return nil
	}
	if !ok {
		return nil
	}
	return &match
}

func gatewayAuthFailureReasonAllowsPrefix(reason string) bool {
	switch reason {
	case "invalid_api_key", "api_key_disabled", "api_key_expired", "ip_not_allowed":
		return true
	default:
		return false
	}
}

func (s *Server) requireGatewayKeyPlaintext(r *http.Request, apiKey string) (apikeycontract.AuthResult, error) {
	authed, err := s.runtime.apiKeys.Authenticate(r.Context(), strings.TrimSpace(apiKey))
	if err != nil {
		return apikeycontract.AuthResult{}, err
	}
	// Per-key RPM counter increment for the gateway hot path. Cheap atomic
	// add — flushed on a schedule by the runtime-owned ticker. Mirrors
	// sub2api's billing_cache_service.checkRPM increment site.
	if counter := s.runtime.apiKeys.RPMCounter(); counter != nil {
		counter.Increment(authed.Key.ID)
	}
	if err := gatewayKeyIPAllowed(authed.Key, clientIP(r)); err != nil {
		return apikeycontract.AuthResult{}, err
	}
	if err := s.runtime.enforceGatewayRiskControl(r.Context(), authed, clientIP(r)); err != nil {
		return apikeycontract.AuthResult{}, err
	}
	if err := s.acquireGatewayConcurrency(r.Context(), authed.Key); err != nil {
		return apikeycontract.AuthResult{}, err
	}
	ctx := platformlogger.WithUserID(r.Context(), authed.UserID)
	ctx = platformlogger.WithAPIKeyID(ctx, authed.Key.ID)
	*r = *r.WithContext(ctx)
	return authed, nil
}

func (s *Server) acquireGatewayConcurrency(ctx context.Context, key apikeycontract.APIKey) error {
	if s.runtime == nil || s.runtime.rateLimiter == nil {
		return nil
	}
	limit := positiveLimit(key.ConcurrencyLimit)
	if limit <= 0 {
		return nil
	}
	state, _ := ctx.Value(gatewayConcurrencyContextKey{}).(*gatewayConcurrencyState)
	if state == nil || state.hasLeaseFor(key.ID) {
		return nil
	}
	lease, decision, err := s.runtime.rateLimiter.AcquireConcurrency(ctx, ratelimit.ConcurrencyCheck{
		Name:  "concurrency",
		Key:   "apikey:" + strconv.Itoa(key.ID) + ":concurrency",
		Limit: limit,
		TTL:   s.cfg.Gateway.RequestTimeout,
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return gatewayConcurrencyLimitError{decision: decision}
	}
	state.storeLease(key.ID, lease)
	return nil
}

func (s *Server) apiKeyByUser(ctx context.Context, userID, keyID int) (apikeycontract.APIKey, error) {
	keys, err := s.runtime.apiKeys.ListByUser(ctx, userID)
	if err != nil {
		return apikeycontract.APIKey{}, err
	}
	for _, key := range keys {
		if key.ID == keyID {
			return key, nil
		}
	}
	return apikeycontract.APIKey{}, apikeyservice.ErrKeyNotFound
}

func (s *Server) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return err
	}
	return nil
}

func (s *Server) decodeJSONBodyWithRaw(w http.ResponseWriter, r *http.Request, dst any) ([]byte, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errRequestTooLarge
		}
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return nil, err
	}
	return append([]byte(nil), raw...), nil
}

func jsonDecodeStatus(err error) int {
	if errors.Is(err, errRequestTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func gatewayUsageDays(r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("days"))
	if raw == "" {
		return 30, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > 90 {
		return 0, false
	}
	return days, true
}

func writeJSONAny(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeStandardError(w http.ResponseWriter, status int, code apiopenapi.ErrorCode, message, requestID string) {
	writeJSONAny(w, status, apiopenapi.ErrorResponse{
		Error: apiopenapi.ErrorObject{
			Code:    code,
			Message: message,
			TraceId: requestID,
		},
		RequestId: requestID,
	})
}

func writeGatewayError(w http.ResponseWriter, status int, typ apiopenapi.GatewayErrorObjectType, message, code string) {
	setDefaultRetryAfter(w, status)
	var codePtr *string
	if code != "" {
		codePtr = &code
	}
	writeJSONAny(w, status, apiopenapi.GatewayErrorResponse{
		Error: apiopenapi.GatewayErrorObject{
			Code:    codePtr,
			Message: message,
			Param:   nil,
			Type:    typ,
		},
	})
}

func writeGatewayAuthError(w http.ResponseWriter, err error, requestID string) {
	var concurrencyErr gatewayConcurrencyLimitError
	switch {
	case errors.As(err, &concurrencyErr):
		setRetryAfterFromDecision(w, concurrencyErr.decision)
		writeGatewayError(w, http.StatusTooManyRequests, apiopenapi.RateLimitError, "API key concurrency limit exceeded", "concurrency_limit_exceeded")
	case errors.Is(err, errGatewayKeyIPNotAllowed):
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "API key not permitted from this IP address", "ip_not_allowed")
	case errors.Is(err, errGatewayRiskControlBlocked):
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "request blocked by risk control", "risk_control_blocked")
	case errors.Is(err, apikeyservice.ErrInvalidKey), errors.Is(err, apikeyservice.ErrInvalidInput):
		writeGatewayError(w, http.StatusUnauthorized, apiopenapi.AuthenticationError, "invalid API key", "invalid_api_key")
	case errors.Is(err, apikeyservice.ErrKeyDisabled), errors.Is(err, apikeyservice.ErrKeyExpired):
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "API key disabled or expired", "api_key_disabled")
	default:
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to authenticate API key", "internal_error")
	}
	_ = requestID
}

func writeGeminiGatewayError(w http.ResponseWriter, status int, rpcStatus, message string) {
	setDefaultRetryAfter(w, status)
	writeJSONAny(w, status, apiopenapi.GeminiErrorResponse{
		Error: apiopenapi.GeminiErrorObject{
			Code:    status,
			Message: message,
			Status:  strings.TrimSpace(rpcStatus),
		},
	})
}

func setDefaultRetryAfter(w http.ResponseWriter, status int) {
	if status == http.StatusTooManyRequests && strings.TrimSpace(w.Header().Get("Retry-After")) == "" {
		w.Header().Set("Retry-After", "60")
	}
}

func setRetryAfterFromDecision(w http.ResponseWriter, decision ratelimit.Decision) {
	if decision.RetryAfter <= 0 {
		return
	}
	seconds := int((decision.RetryAfter + time.Second - time.Nanosecond) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
}

func writeGeminiGatewayAuthError(w http.ResponseWriter, err error) {
	var concurrencyErr gatewayConcurrencyLimitError
	switch {
	case errors.As(err, &concurrencyErr):
		setRetryAfterFromDecision(w, concurrencyErr.decision)
		writeGeminiGatewayError(w, http.StatusTooManyRequests, "RESOURCE_EXHAUSTED", "API key concurrency limit exceeded")
	case errors.Is(err, errGatewayKeyIPNotAllowed):
		writeGeminiGatewayError(w, http.StatusForbidden, "PERMISSION_DENIED", "API key not permitted from this IP address")
	case errors.Is(err, errGatewayRiskControlBlocked):
		writeGeminiGatewayError(w, http.StatusForbidden, "PERMISSION_DENIED", "request blocked by risk control")
	case errors.Is(err, apikeyservice.ErrInvalidKey), errors.Is(err, apikeyservice.ErrInvalidInput):
		writeGeminiGatewayError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid API key")
	case errors.Is(err, apikeyservice.ErrKeyDisabled), errors.Is(err, apikeyservice.ErrKeyExpired):
		writeGeminiGatewayError(w, http.StatusForbidden, "PERMISSION_DENIED", "API key disabled or expired")
	default:
		writeGeminiGatewayError(w, http.StatusInternalServerError, "INTERNAL", "failed to authenticate API key")
	}
}

func geminiStatusForGatewayErrorClass(errorClass string, status int) string {
	switch errorClass {
	case "invalid_request":
		return "INVALID_ARGUMENT"
	case "rate_limit", "rate_limit_exceeded", "rpm_limit_exceeded", "tpm_limit_exceeded", "concurrency_limit_exceeded", "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded":
		return "RESOURCE_EXHAUSTED"
	case "auth_failed", "auth_error", "permission_denied", "credential_error", "entitlement_model_not_allowed", "entitlement_denied":
		return "PERMISSION_DENIED"
	case "model_not_found":
		return "NOT_FOUND"
	case "no_available_account", "model_unavailable", "provider_5xx", "timeout", "network_error", "stream_interrupted", "empty_completion":
		return "UNAVAILABLE"
	default:
		return geminiStatusForHTTPStatus(status)
	}
}

func geminiStatusForHTTPStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return "UNAVAILABLE"
	default:
		if status >= 500 {
			return "INTERNAL"
		}
		return "UNKNOWN"
	}
}

func writePaymentServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, paymentservice.ErrInvalidInput), errors.Is(err, paymentservice.ErrInvalidTransition), errors.Is(err, paymentservice.ErrOrderMismatch):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment request", requestID)
	case errors.Is(err, paymentservice.ErrSignatureInvalid):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid payment webhook signature", requestID)
	case errors.Is(err, paymentservice.ErrProviderUnavailable):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "payment provider unavailable", requestID)
	case errors.Is(err, paymentcontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "payment resource not found", requestID)
	case errors.Is(err, paymentcontract.ErrConflict):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "payment resource conflict", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "payment service error", requestID)
	}
}

func writeAffiliateServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, affiliateservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid affiliate request", requestID)
	case errors.Is(err, affiliatecontract.ErrInsufficientBalance):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "affiliate balance insufficient", requestID)
	case errors.Is(err, affiliatecontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "affiliate resource not found", requestID)
	case errors.Is(err, affiliatecontract.ErrConflict):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "affiliate resource conflict", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "affiliate service error", requestID)
	}
}

func writeTOTPServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, totpservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid totp request", requestID)
	case errors.Is(err, totpservice.ErrInvalidCode):
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid totp code", requestID)
	case errors.Is(err, totpservice.ErrSecretNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "totp setup not found", requestID)
	case errors.Is(err, totpservice.ErrSecretDisabled):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "totp is not enabled", requestID)
	case errors.Is(err, totpservice.ErrSecretAlreadyEnabled):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "totp is already enabled", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "totp service error", requestID)
	}
}

func writeOperationsServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, operationsservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid operations request", requestID)
	case errors.Is(err, operationscontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "operations resource not found", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "operations service error", requestID)
	}
}

func writeAdminControlError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, admincontrol.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid admin control request", requestID)
	case errors.Is(err, admincontrol.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "admin control resource not found", requestID)
	case errors.Is(err, admincontrol.ErrConflict):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "admin control resource conflict", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "admin control service error", requestID)
	}
}

func validateCSRF(session authcontract.Session, token string) error {
	return authservice.ValidateCSRF(session, token)
}

// withCSRFRotation wraps a handler so that on successful (2xx) responses, a new
// CSRF token is generated, sent via X-CSRF-Token-Rotated, and committed to the
// session store. The pre-generated token is set as a header before the handler
// writes; the store commit is deferred until the response succeeds. The old
// token remains valid for the current request (already validated by the handler).
func (s *Server) withCSRFRotation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CSRF-Rotation") == "" {
			next(w, r)
			return
		}
		session, err := s.requireConsoleSession(r)
		if err != nil {
			next(w, r)
			return
		}
		newToken, genErr := authservice.GenerateCSRFToken()
		if genErr != nil {
			next(w, r)
			return
		}
		w.Header().Set("X-CSRF-Token-Rotated", newToken)
		rec := &statusRecorder{ResponseWriter: w}
		next(rec, r)
		if rec.status >= 200 && rec.status < 300 {
			rotator, ok := s.runtime.sessionStore.(authcontract.CSRFRotator)
			if ok {
				_ = rotator.UpdateCSRFToken(r.Context(), session.Session.ID, newToken)
			}
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (s *Server) rotateCSRFTokenQuiet(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	rotator, ok := s.runtime.sessionStore.(authcontract.CSRFRotator)
	if !ok {
		return ""
	}
	newToken, err := authservice.GenerateCSRFToken()
	if err != nil {
		return ""
	}
	if err := rotator.UpdateCSRFToken(ctx, sessionID, newToken); err != nil {
		return ""
	}
	return newToken
}
