package httpserver

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleListAdminApiKeys lists API keys across every user (the user-scoped
// endpoint only ever returns the caller's own keys). Each row is attributed to
// its owner (user_id + email) so an admin can audit and revoke keys globally.
func (s *Server) handleListAdminApiKeys(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	keys, err := s.runtime.apiKeys.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list api keys", requestID)
		return
	}
	keys = filterApiKeys(keys, r.URL.Query().Get("status"))
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		if uid, convErr := strconv.Atoi(raw); convErr == nil {
			filtered := make([]apikeycontract.APIKey, 0, len(keys))
			for _, key := range keys {
				if key.UserID == uid {
					filtered = append(filtered, key)
				}
			}
			keys = filtered
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].CreatedAt.Before(keys[j].CreatedAt) })

	page := 1
	pageSize := 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil && n > 0 {
			pageSize = n
		}
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	paged, total, hasNext := paginateApiKeys(keys, page, pageSize)

	data := make([]apiopenapi.ApiKey, 0, len(paged))
	for _, key := range paged {
		data = append(data, s.toAdminAPIKey(r, key))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ApiKeyListResponse{
		Data: data,
		Pagination: apiopenapi.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
			HasNext:  hasNext,
		},
		RequestId: requestID,
	})
}

// handleUpdateAdminApiKey lets an admin change a key's status across users —
// the primary use is revoking (disabling) a key. It looks the key up by ID and
// reuses the owner-scoped Update so all the usual validation still applies.
func (s *Server) handleUpdateAdminApiKey(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	keyID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || keyID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key id", requestID)
		return
	}
	var body apiopenapi.AdminUpdateApiKeyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid api key request", requestID)
		return
	}
	existing, err := s.runtime.apiKeys.GetByID(r.Context(), keyID)
	if err != nil {
		s.writeAdminApiKeyError(w, err, requestID)
		return
	}
	status := body.Status
	updated, err := s.runtime.apiKeys.Update(r.Context(), apikeycontract.UpdateRequest{
		UserID: existing.UserID,
		KeyID:  keyID,
		Status: toAPIKeyStatusPtr(&status),
	})
	if err != nil {
		s.writeAdminApiKeyError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "admin_api_key.update", "api_key", strconv.Itoa(keyID), apiKeyAuditSnapshot(existing), apiKeyAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ApiKeyResponse{
		Data:      s.toAdminAPIKey(r, updated),
		RequestId: requestID,
	})
}

// toAdminAPIKey maps a key to the API shape and attaches owner attribution
// (user_id always, email when the owner is resolvable).
func (s *Server) toAdminAPIKey(r *http.Request, key apikeycontract.APIKey) apiopenapi.ApiKey {
	api := toAPIKey(key)
	uid := apiopenapi.Id(strconv.Itoa(key.UserID))
	api.UserId = &uid
	if user, err := s.runtime.users.FindByID(r.Context(), key.UserID); err == nil {
		if email := user.User.Email; email != "" {
			api.UserEmail = &email
		}
	}
	return api
}

func (s *Server) writeAdminApiKeyError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, apikeyservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key request", requestID)
	case errors.Is(err, apikeycontract.ErrKeyNotFound), errors.Is(err, apikeyservice.ErrKeyNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "api key service failed", requestID)
	}
}
