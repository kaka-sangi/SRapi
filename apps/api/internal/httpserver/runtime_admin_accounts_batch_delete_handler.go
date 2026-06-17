package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchDeleteAdminAccount is split into its own file because
// runtime_admin_catalog_handlers.go is already past the architecture-test
// size limit (TestHTTPRuntimeFilesStayPartitioned: 2200 lines). New
// admin handlers go to new files until that file is split.
//
// Soft-delete is best-effort per id; the response carries per-id failures
// without aborting the call. Idempotent: NotFound on a row is treated as
// success because the caller's "this id should not exist" intent is
// already satisfied. Audit snapshot records the bulk outcome (requested,
// succeeded, failed counts + the failed id list); per-id credentials are
// never persisted.
func (s *Server) handleBatchDeleteAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.Gateway.MaxBodySize+1))
	if err != nil || int64(len(bodyBytes)) > s.cfg.Gateway.MaxBodySize {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	var body apiopenapi.BatchDeleteProviderAccountsRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}

	ids := make([]int, 0, len(body.AccountIds))
	for _, raw := range body.AccountIds {
		id, parseErr := strconv.Atoi(string(raw))
		if parseErr != nil || id <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id in batch", requestID)
			return
		}
		ids = append(ids, id)
	}

	results, err := s.runtime.accounts.BatchDeleteAccounts(r.Context(), ids)
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch delete request", requestID)
		default:
			writeAdminControlError(w, err, requestID)
		}
		return
	}

	deletedIDs := make([]apiopenapi.Id, 0, len(results))
	errorRows := make([]apiopenapi.BatchDeleteProviderAccountsErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			deletedIDs = append(deletedIDs, apiopenapi.Id(strconv.Itoa(row.AccountID)))
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchDeleteProviderAccountsErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.AccountID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.AccountID)
	}

	// Audit — record the outcome but never the per-id credentials. Deleted
	// ids + failed ids + per-failure messages are all operator-readable
	// (no secret leakage).
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"provider_account.batch_delete",
		"provider_account",
		"",
		nil,
		map[string]any{
			"requested":  len(ids),
			"succeeded":  len(deletedIDs),
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchDeleteProviderAccountsResponse{
		Data: apiopenapi.BatchDeleteProviderAccountsResult{
			DeletedCount: len(deletedIDs),
			DeletedIds:   deletedIDs,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
