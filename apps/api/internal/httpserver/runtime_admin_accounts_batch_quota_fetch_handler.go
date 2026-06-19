package httpserver

import (
	"net/http"
	"strconv"

	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchQuotaFetchAdminAccounts is the sub2api `batch-refresh-tier`
// port: fans out the per-account `quota-fetch` operation across the given
// account IDs in one HTTP call so an operator can re-poll quota across a
// fleet of OAuth accounts without N round-trips. Best-effort — per-row
// failures (provider error, network, credential refresh failure, account
// not found) collect in the response without aborting the batch. Each
// successful row persists a fresh quota snapshot via the same path
// `/admin/accounts/{id}/quota-fetch` uses, so the rest of the admin UI
// sees the new numbers immediately.
func (s *Server) handleBatchQuotaFetchAdminAccounts(w http.ResponseWriter, r *http.Request) {
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

	var body apiopenapi.BatchQuotaFetchRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid batch quota fetch request", requestID)
		return
	}
	accountIDs, err := apiIDsValueToInts(body.AccountIds)
	if err != nil || len(accountIDs) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account ids", requestID)
		return
	}

	rows := make([]apiopenapi.BatchQuotaFetchRow, 0, len(accountIDs))
	successCount := 0
	failedCount := 0
	for _, accountID := range accountIDs {
		row := apiopenapi.BatchQuotaFetchRow{
			AccountId: apiopenapi.Id(strconv.Itoa(accountID)),
		}
		if _, err := s.fetchAccountQuotaReportOnce(r, accountID); err != nil {
			row.Success = false
			msg := err.Error()
			row.Error = &msg
			failedCount++
		} else {
			row.Success = true
			successCount++
		}
		rows = append(rows, row)
	}

	failedIDs := make([]int, 0, failedCount)
	for i, row := range rows {
		if !row.Success {
			failedIDs = append(failedIDs, accountIDs[i])
		}
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.batch_quota_fetch", "provider_account", "bulk", nil, map[string]any{
		"account_ids": accountIDs,
		"total":       len(accountIDs),
		"success":     successCount,
		"failed":      failedCount,
		"failed_ids":  failedIDs,
	}))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchQuotaFetchResponse{
		Data: apiopenapi.BatchQuotaFetchResult{
			Total:   len(accountIDs),
			Success: successCount,
			Failed:  failedCount,
			Rows:    rows,
		},
		RequestId: requestID,
	})
}
