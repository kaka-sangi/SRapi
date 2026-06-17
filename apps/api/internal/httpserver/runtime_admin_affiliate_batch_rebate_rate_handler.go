package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchSetAdminAffiliateRebateRate bulk-sets (or clears) the per-user
// affiliate rebate-rate override on N users in one call. Verbatim port of
// sub2api's AffiliateHandler.BatchSetRate (affiliate_handler.go) →
// AffiliateService.AdminBatchSetUserRebateRate → repository.BatchSetUserRebateRate
// (affiliate_repo.go). sub2api persists this to a user_affiliates table; srapi
// has no such schema yet, so the override is held in an in-memory overlay on
// the affiliate service.
//
// Per-row failures (invalid id, rate out of [0,1] range, duplicate in batch)
// surface in errors[] without aborting the batch. Outer error is reserved for
// precondition failures.
//
// New file (rather than appending to runtime_admin_affiliate_handlers.go) to
// keep the bulk endpoint's audit-snapshot wiring focused.
func (s *Server) handleBatchSetAdminAffiliateRebateRate(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchSetAdminAffiliateRebateRateRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}

	items := make([]affiliatecontract.BatchSetUserRebateRateItem, 0, len(body.Items))
	for _, raw := range body.Items {
		uid, parseErr := strconv.Atoi(string(raw.UserId))
		if parseErr != nil || uid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id in batch", requestID)
			return
		}
		clear := false
		if raw.Clear != nil {
			clear = *raw.Clear
		}
		items = append(items, affiliatecontract.BatchSetUserRebateRateItem{
			UserID:        uid,
			RatePercent:   raw.RatePercent,
			ClearOverride: clear,
		})
	}

	results, err := s.runtime.affiliate.BatchSetUserRebateRate(r.Context(), items)
	if err != nil {
		switch {
		case errors.Is(err, affiliateservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch rebate-rate request", requestID)
		default:
			writeAffiliateServiceError(w, err, requestID)
		}
		return
	}

	updatedCount := 0
	errorRows := make([]apiopenapi.BatchSetAdminAffiliateRebateRateErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			updatedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchSetAdminAffiliateRebateRateErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.UserID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.UserID)
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"affiliate.batch_rebate_rate",
		"affiliate",
		"",
		nil,
		map[string]any{
			"requested":  len(items),
			"succeeded":  updatedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchSetAdminAffiliateRebateRateResponse{
		Data: apiopenapi.BatchSetAdminAffiliateRebateRateResult{
			UpdatedCount: updatedCount,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
