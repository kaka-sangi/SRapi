package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// parseRedeemExpiresAt accepts the RFC3339 timestamp the OpenAPI Timestamp
// schema documents. RFC3339Nano (subsecond precision) is also accepted so
// timestamps round-tripped through JSON marshalling stay valid.
func parseRedeemExpiresAt(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, raw)
}

// handleBatchUpdateAdminRedeemCodes lives in its own file because
// runtime_admin_catalog_handlers.go is past the architecture-test size
// limit (TestHTTPRuntimeFilesStayPartitioned: 2200 lines). New admin
// handlers go to new files until that file is split.
//
// Per-row partial update across N redeem codes. Idempotent NotFound;
// per-row failures (invalid amount, already-redeemed gate, store error)
// surface in errors[]. Audit snapshot records the bulk outcome (counts +
// failed id list); per-id values are NOT logged so an accidental amount
// leak doesn't end up in the audit table.
func (s *Server) handleBatchUpdateAdminRedeemCodes(w http.ResponseWriter, r *http.Request) {
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

	// Decode into a permissive shape so we can detect the `expires_at: null`
	// case (clear-expiry) distinctly from "field missing" (leave alone). The
	// generated apiopenapi.BatchUpdateRedeemCodeItem treats both as nil, but
	// the wire-level distinction matters for NullableTimeUpdate semantics.
	var raw struct {
		Items []struct {
			ID             apiopenapi.Id `json:"id"`
			Amount         *string       `json:"amount"`
			MaxRedemptions *int          `json:"max_redemptions"`
			ExpiresAt      *string       `json:"expires_at"`
			ExpiresAtSet   bool          `json:"-"`
			Note           *string       `json:"note"`
		} `json:"items"`
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	// Re-parse to detect ExpiresAt presence ("null" vs missing). The above
	// pointer-decoder yields nil for both — we re-walk the raw JSON to set
	// ExpiresAtSet for keys that are present.
	var presence struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(bodyBytes, &presence); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}
	for i, m := range presence.Items {
		if _, ok := m["expires_at"]; ok && i < len(raw.Items) {
			raw.Items[i].ExpiresAtSet = true
		}
	}

	items := make([]admincontrol.BatchUpdateRedeemCodeItem, 0, len(raw.Items))
	for _, r := range raw.Items {
		id, parseErr := strconv.Atoi(string(r.ID))
		if parseErr != nil || id <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid redeem code id in batch", requestID)
			return
		}
		item := admincontrol.BatchUpdateRedeemCodeItem{
			ID:             id,
			Value:          r.Amount,
			MaxRedemptions: r.MaxRedemptions,
			ExpiresAtSet:   r.ExpiresAtSet,
			Note:           r.Note,
		}
		if r.ExpiresAt != nil {
			parsed, err := parseRedeemExpiresAt(*r.ExpiresAt)
			if err != nil {
				writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid expires_at in batch", requestID)
				return
			}
			item.ExpiresAt = &parsed
		}
		items = append(items, item)
	}

	results, err := s.runtime.adminControl.BatchUpdateRedeemCodes(r.Context(), items, session.User.ID)
	if err != nil {
		if errors.Is(err, admincontrol.ErrInvalidInput) {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch update request", requestID)
			return
		}
		writeAdminControlError(w, err, requestID)
		return
	}

	updatedCount := 0
	errorRows := make([]apiopenapi.BatchUpdateRedeemCodeErrorRow, 0)
	failedIDs := make([]int, 0)
	for _, row := range results {
		if row.Error == "" {
			updatedCount++
			continue
		}
		errorRows = append(errorRows, apiopenapi.BatchUpdateRedeemCodeErrorRow{
			Id:      apiopenapi.Id(strconv.Itoa(row.ID)),
			Message: row.Error,
		})
		failedIDs = append(failedIDs, row.ID)
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"redeem_code.batch_update",
		"redeem_code",
		"bulk",
		nil,
		map[string]any{
			"requested":  len(items),
			"succeeded":  updatedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateRedeemCodesResponse{
		Data: apiopenapi.BatchUpdateRedeemCodesResult{
			UpdatedCount: updatedCount,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
