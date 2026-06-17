package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchUpdateAdminAccountConcurrency lives in its own file because
// runtime_admin_catalog_handlers.go is already past the architecture-test
// size limit (TestHTTPRuntimeFilesStayPartitioned: 2200 lines). New admin
// handlers go to new files until that file is split.
//
// Bulk-update is best-effort per row: the response carries per-id failures
// without aborting the call. Idempotent on NotFound — a missing id counts as
// success because the caller's intent ("this id should have max_concurrency
// X") is moot for a row that does not exist. Audit snapshot records only the
// outcome (requested / succeeded / failed counts + the failed id list); the
// per-row credential payload is never touched by this endpoint so there is
// no secret exposure.
func (s *Server) handleBatchUpdateAdminAccountConcurrency(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchUpdateAccountConcurrencyRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}

	items := make([]accountcontract.BatchUpdateConcurrencyItem, 0, len(body.Items))
	for _, raw := range body.Items {
		id, parseErr := strconv.Atoi(string(raw.AccountId))
		if parseErr != nil || id <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id in batch", requestID)
			return
		}
		items = append(items, accountcontract.BatchUpdateConcurrencyItem{
			AccountID:      id,
			MaxConcurrency: raw.MaxConcurrency,
		})
	}

	results, err := s.runtime.accounts.BatchUpdateConcurrency(r.Context(), items)
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch concurrency request", requestID)
		default:
			writeAdminControlError(w, err, requestID)
		}
		return
	}

	updatedCount := 0
	errorRows := make([]apiopenapi.BatchUpdateAccountConcurrencyErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			updatedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchUpdateAccountConcurrencyErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.AccountID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.AccountID)
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"provider_account.batch_concurrency",
		"provider_account",
		"",
		nil,
		map[string]any{
			"requested":  len(items),
			"succeeded":  updatedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateAccountConcurrencyResponse{
		Data: apiopenapi.BatchUpdateAccountConcurrencyResult{
			UpdatedCount: updatedCount,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
