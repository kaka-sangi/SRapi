package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

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
	return s.runtime.auth.AuthenticateSession(r.Context(), cookie.Value)
}

func (s *Server) requireAdminSession(r *http.Request) (authcontract.LoginResult, error) {
	session, err := s.requireConsoleSession(r)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	for _, role := range session.User.Roles {
		if role == userscontract.RoleOwner || role == userscontract.RoleAdmin {
			return session, nil
		}
	}
	return authcontract.LoginResult{}, errors.New("admin access required")
}

func (s *Server) requireGatewayKey(r *http.Request) (apikeycontract.AuthResult, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return apikeycontract.AuthResult{}, apikeyservice.ErrInvalidKey
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return apikeycontract.AuthResult{}, apikeyservice.ErrInvalidKey
	}
	return s.runtime.apiKeys.Authenticate(r.Context(), parts[1])
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

func jsonDecodeStatus(err error) int {
	if errors.Is(err, errRequestTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
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
	switch {
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

func writeGeminiGatewayAuthError(w http.ResponseWriter, err error) {
	switch {
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
	case "rate_limit", "rate_limit_exceeded", "rpm_limit_exceeded", "tpm_limit_exceeded", "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded":
		return "RESOURCE_EXHAUSTED"
	case "auth_failed", "auth_error", "permission_denied", "credential_error", "entitlement_model_not_allowed", "entitlement_denied":
		return "PERMISSION_DENIED"
	case "model_not_found":
		return "NOT_FOUND"
	case "no_available_account", "model_unavailable", "provider_5xx", "timeout", "network_error", "stream_interrupted":
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
