package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchSetAdminAccountGroupRateMultipliers + RPMOverrides live in their
// own file because runtime_admin_catalog_handlers.go is past the
// architecture-test size limit (TestHTTPRuntimeFilesStayPartitioned: 2200
// lines). New admin handlers go to new files until that file is split.
//
// Bulk-update is best-effort per row — per-id failures surface in the response
// without aborting the call. Idempotent on NotFound. Audit snapshot records
// the bulk outcome (requested / succeeded / failed counts + the failed id
// list). No credential payload, no secret leakage.

func (s *Server) handleBatchSetAdminAccountGroupRateMultipliers(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchSetGroupRateMultipliersRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	items := make([]accountcontract.BatchSetGroupRateMultiplierItem, 0, len(body.Items))
	for _, raw := range body.Items {
		gid, parseErr := strconv.Atoi(string(raw.GroupId))
		if parseErr != nil || gid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group id in batch", requestID)
			return
		}
		items = append(items, accountcontract.BatchSetGroupRateMultiplierItem{
			GroupID:    gid,
			Multiplier: raw.Multiplier,
		})
	}
	results, err := s.runtime.accounts.BatchSetGroupRateMultipliers(r.Context(), items)
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch rate-multipliers request", requestID)
		default:
			writeAdminControlError(w, err, requestID)
		}
		return
	}
	updatedCount := 0
	errorRows := make([]apiopenapi.BatchSetGroupRateMultiplierErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			updatedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchSetGroupRateMultiplierErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.GroupID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.GroupID)
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"account_group.batch_rate_multipliers",
		"account_group",
		"",
		nil,
		map[string]any{
			"requested":  len(items),
			"succeeded":  updatedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchSetGroupRateMultipliersResponse{
		Data: apiopenapi.BatchSetGroupRateMultipliersResult{
			UpdatedCount: updatedCount,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}

func (s *Server) handleBatchSetAdminAccountGroupRPMOverrides(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchSetGroupRPMOverridesRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	items := make([]groupratelimitscontract.BatchSetRPMOverrideItem, 0, len(body.Items))
	for _, raw := range body.Items {
		gid, parseErr := strconv.Atoi(string(raw.GroupId))
		if parseErr != nil || gid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group id in batch", requestID)
			return
		}
		var override *int
		if raw.RpmOverride != nil {
			v := *raw.RpmOverride
			override = &v
		}
		items = append(items, groupratelimitscontract.BatchSetRPMOverrideItem{
			GroupID:     gid,
			RPMOverride: override,
		})
	}
	results, err := s.runtime.groupRateLimits.BatchSetRPMOverrides(r.Context(), items)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch rpm-overrides request", requestID)
		return
	}
	updatedCount := 0
	errorRows := make([]apiopenapi.BatchSetGroupRPMOverrideErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			updatedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchSetGroupRPMOverrideErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.GroupID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.GroupID)
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"account_group.batch_rpm_overrides",
		"account_group",
		"",
		nil,
		map[string]any{
			"requested":  len(items),
			"succeeded":  updatedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchSetGroupRPMOverridesResponse{
		Data: apiopenapi.BatchSetGroupRPMOverridesResult{
			UpdatedCount: updatedCount,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
