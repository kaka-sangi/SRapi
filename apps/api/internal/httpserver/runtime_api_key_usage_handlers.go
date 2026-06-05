package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleCurrentUserApiKeyUsage returns the usage drilldown for one of the
// caller's own API keys, reusing the same aggregation + response shape as the
// gateway-bearer GET /v1/usage but authenticated by the console session.
func (s *Server) handleCurrentUserApiKeyUsage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	keyID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || keyID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key id", requestID)
		return
	}
	key, err := s.apiKeyByUser(r.Context(), session.User.ID, keyID)
	if err != nil {
		if errors.Is(err, apikeyservice.ErrKeyNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load api key", requestID)
		return
	}
	days, ok := gatewayUsageDays(r)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "days must be an integer between 1 and 90", requestID)
		return
	}
	summary, err := s.runtime.usage.SummarizeAPIKey(r.Context(), key.ID, days)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load usage", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, gatewayUsageResponse(key, session.User, summary))
}

// handleAdminApiKeyUsage returns the usage drilldown for any key by id, scoped
// to admins, attributing the owning user's balance/currency.
func (s *Server) handleAdminApiKeyUsage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	keyID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || keyID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key id", requestID)
		return
	}
	key, err := s.runtime.apiKeys.GetByID(r.Context(), keyID)
	if err != nil {
		if errors.Is(err, apikeyservice.ErrKeyNotFound) {
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load api key", requestID)
		return
	}
	days, ok := gatewayUsageDays(r)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "days must be an integer between 1 and 90", requestID)
		return
	}
	summary, err := s.runtime.usage.SummarizeAPIKey(r.Context(), key.ID, days)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load usage", requestID)
		return
	}
	owner, err := s.runtime.users.FindByID(r.Context(), key.UserID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load usage", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, gatewayUsageResponse(key, owner.User, summary))
}
