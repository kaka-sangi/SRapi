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

// handleBatchRefreshAdminAccounts triggers an OAuth refresh against N
// accounts in one call. Verbatim port of sub2api's AccountHandler.BatchRefresh
// (account_handler.go): sub2api fanned out via errgroup with maxConcurrency=10
// and surfaced per-row outcomes. srapi reuses RefreshAccessTokenWithOutcome on
// the accounts service so the bookkeeping rules (refresh_attempts /
// needs_reauth_at / token_expires_at) stay in one place; the structured
// RefreshOutcomeClass surfaces per-row in the rows[] array.
//
// Best-effort across the batch: a single-row failure populates that row's
// outcome (Error + OutcomeClass) and the rest of the batch continues. NotFound
// is idempotent — a missing id counts as success (caller's intent "this id
// has been refreshed" is moot for a row that does not exist), matching the
// other batch endpoints. Audit snapshot records the requested / succeeded /
// failed counts + the failed id list; credential bytes are NEVER recorded.
//
// New file (rather than appending to runtime_admin_catalog_handlers.go) per
// architecture-test limit (TestHTTPRuntimeFilesStayPartitioned: 2200 lines).
func (s *Server) handleBatchRefreshAdminAccounts(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchRefreshAdminAccountsRequest
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
	if s.runtime.reverseProxy == nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "reverse proxy refresher unavailable", requestID)
		return
	}
	adapter := adminAccountRefresherAdapter{refresher: s.runtime.reverseProxy}
	results, err := s.runtime.accounts.BatchRefreshAccounts(r.Context(), ids, adapter)
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch refresh request", requestID)
		default:
			writeAdminControlError(w, err, requestID)
		}
		return
	}

	refreshedCount := 0
	rows := make([]apiopenapi.BatchRefreshAdminAccountsRow, 0, len(results))
	errorRows := make([]apiopenapi.BatchRefreshAdminAccountsErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		attempts := row.Attempts
		flipped := row.NeedsReauthFlipped
		rows = append(rows, apiopenapi.BatchRefreshAdminAccountsRow{
			AccountId:          apiopenapi.Id(strconv.Itoa(row.AccountID)),
			OutcomeClass:       row.OutcomeClass,
			Attempts:           &attempts,
			NeedsReauthFlipped: &flipped,
		})
		if row.Error == "" {
			refreshedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchRefreshAdminAccountsErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.AccountID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.AccountID)
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"provider_account.batch_refresh",
		"provider_account",
		"",
		nil,
		map[string]any{
			"requested":  len(ids),
			"succeeded":  refreshedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchRefreshAdminAccountsResponse{
		Data: apiopenapi.BatchRefreshAdminAccountsResult{
			RefreshedCount: refreshedCount,
			Rows:           rows,
			Errors:         errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
