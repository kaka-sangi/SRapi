package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// systemNeedsSetup reports whether first-run setup is required: true until an
// owner/admin account exists. The "an administrator exists" result is memoized
// (an administrator never has to be re-discovered), so the public status
// endpoint stays O(1) on already-provisioned systems and only scans users while
// the system is still unconfigured (when there are essentially none).
func (rt *runtimeState) systemNeedsSetup(ctx context.Context) (bool, error) {
	if rt.setupComplete.Load() {
		return false, nil
	}
	users, err := rt.users.List(ctx, usersservice.ListRequest{})
	if err != nil {
		return false, err
	}
	for _, user := range users {
		for _, role := range user.Roles {
			if role == userscontract.RoleOwner || role == userscontract.RoleAdmin {
				rt.setupComplete.Store(true)
				return false, nil
			}
		}
	}
	return true, nil
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	needsSetup, err := s.runtime.systemNeedsSetup(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to determine setup status", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SetupStatusResponse{
		Data:      apiopenapi.SetupStatus{NeedsSetup: needsSetup},
		RequestId: requestID,
	})
}

func (s *Server) handleCompleteSetup(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	needsSetup, err := s.runtime.systemNeedsSetup(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to determine setup status", requestID)
		return
	}
	if !needsSetup {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "setup has already been completed", requestID)
		return
	}

	var body apiopenapi.CompleteSetupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid setup request", requestID)
		return
	}

	created, err := s.runtime.users.Create(r.Context(), usersservice.CreateRequest{
		Email:    string(body.Email),
		Name:     body.Name,
		Password: body.Password,
		Roles:    []userscontract.Role{userscontract.RoleOwner},
	})
	if err != nil {
		switch {
		case errors.Is(err, usersservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid setup request", requestID)
		case errors.Is(err, usersservice.ErrUserAlreadyExists):
			// An account was created concurrently; setup is effectively done.
			s.runtime.setupComplete.Store(true)
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "setup has already been completed", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to complete setup", requestID)
		}
		return
	}

	s.runtime.setupComplete.Store(true)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, created.ID, "setup.complete", "user", strconv.Itoa(created.ID), nil, map[string]any{
		"email": created.Email,
		"roles": []string{string(userscontract.RoleOwner)},
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.SetupStatusResponse{
		Data:      apiopenapi.SetupStatus{NeedsSetup: false},
		RequestId: requestID,
	})
}
