package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// Manual scheduling pause handlers. Split out of runtime_admin_catalog_handlers
// so that file stays under the architecture file-size cap; the manual-pause
// surface is also a self-contained slice (POST + DELETE on the same path)
// that benefits from co-located ownership.

// handleApplyAdminAccountManualPause records an operator-driven scheduling
// skip window on the account. The window auto-expires at the supplied
// timestamp; the account row's status field stays unchanged so the
// admin-list "active vs disabled" distinction continues to track the
// logical disable, not the temporary skip.
func (s *Server) handleApplyAdminAccountManualPause(w http.ResponseWriter, r *http.Request) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	var body apiopenapi.AdminAccountManualPauseRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	if body.Until.IsZero() {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "until must be supplied", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	reason := ""
	if body.Reason != nil {
		reason = *body.Reason
	}
	updated, err := s.runtime.accounts.ApplyManualPause(r.Context(), accountID, accountservice.ManualPauseRequest{
		Until:  body.Until,
		Reason: reason,
	})
	if err != nil {
		if errors.Is(err, accountservice.ErrInvalidInput) {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "until must be in the future", requestID)
			return
		}
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to record manual pause", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.manual_pause.apply", "provider_account", strconv.Itoa(updated.ID), accountAuditSnapshot(before), accountAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), updated),
		RequestId: requestID,
	})
}

// handleClearAdminAccountManualPause removes the operator-driven pause
// keys. Idempotent: a successful no-op when no pause is active.
func (s *Server) handleClearAdminAccountManualPause(w http.ResponseWriter, r *http.Request) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	updated, err := s.runtime.accounts.ClearManualPause(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to clear manual pause", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.manual_pause.clear", "provider_account", strconv.Itoa(updated.ID), accountAuditSnapshot(before), accountAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), updated),
		RequestId: requestID,
	})
}
