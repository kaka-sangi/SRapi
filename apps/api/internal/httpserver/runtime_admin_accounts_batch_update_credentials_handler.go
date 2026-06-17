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

// handleBatchUpdateAdminAccountCredentials rotates the stored credential on
// N accounts in one call, each row carrying its own partial-credential patch.
// Verbatim port of sub2api's AccountHandler.BatchUpdateCredentials
// (account_handler.go); the only shape difference is sub2api accepted a single
// shared {field, value} for the whole batch, srapi accepts a per-row patch so
// a single call can rotate disjoint fields (refresh_token on one account,
// api_key on another).
//
// Best-effort across the batch: a single-row failure populates that row's
// error and the rest of the batch continues. NotFound is idempotent. The
// audit snapshot records the requested / succeeded / failed counts + the
// failed id list; credential bytes are NEVER recorded (the only structural
// guard against accidental secret exposure is the audit shape — there is no
// place in this code path to leak the patch).
//
// New file per the architecture-test size limit on
// runtime_admin_catalog_handlers.go (2200 lines).
func (s *Server) handleBatchUpdateAdminAccountCredentials(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchUpdateAdminAccountCredentialsRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}

	items := make([]accountcontract.BatchUpdateAccountCredentialItem, 0, len(body.Items))
	for _, raw := range body.Items {
		id, parseErr := strconv.Atoi(string(raw.AccountId))
		if parseErr != nil || id <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id in batch", requestID)
			return
		}
		// The codegen renders JsonObject as map[string]interface{}; the
		// service layer's per-row "empty patch" guard makes a zero-length
		// map a per-row failure rather than a whole-batch reject.
		credential := map[string]any(raw.Credential)
		items = append(items, accountcontract.BatchUpdateAccountCredentialItem{
			AccountID:  id,
			Credential: credential,
		})
	}

	results, err := s.runtime.accounts.BatchUpdateAccountCredentials(r.Context(), items)
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch credential update request", requestID)
		default:
			writeAdminControlError(w, err, requestID)
		}
		return
	}

	updatedCount := 0
	errorRows := make([]apiopenapi.BatchUpdateAdminAccountCredentialErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			updatedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchUpdateAdminAccountCredentialErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.AccountID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.AccountID)
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"provider_account.batch_update_credentials",
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

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateAdminAccountCredentialsResponse{
		Data: apiopenapi.BatchUpdateAdminAccountCredentialsResult{
			UpdatedCount: updatedCount,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
