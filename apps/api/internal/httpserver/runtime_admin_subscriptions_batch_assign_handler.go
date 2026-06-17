package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleBatchAssignAdminUserSubscriptions bulk-assigns a subscription plan to
// N users in one call. Verbatim port of sub2api's
// SubscriptionService.BulkAssignSubscription (subscription_service.go) →
// SubscriptionHandler.BulkAssign (subscription_handler.go).
//
// Per-row outcome is one of `created` / `reused` / `failed`. sub2api's reused
// path matches an existing (user, group) subscription; srapi's port matches on
// (source_type, source_id) when both are set, which CreateUserSubscription
// already short-circuits on. Per-row failures surface in errors[] without
// aborting the batch.
//
// Plan-feature-gated by requireSubscriptionPlansEnabled so the route returns
// the same "subscriptions disabled" 403/404 used by the single-assign endpoint
// when the platform setting is off.
func (s *Server) handleBatchAssignAdminUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if !s.requireSubscriptionPlansEnabled(w, r, requestID) {
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
	var body apiopenapi.BatchAssignAdminUserSubscriptionsRequest
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
		return
	}

	items := make([]subscriptioncontract.BatchAssignSubscriptionItem, 0, len(body.Items))
	for _, raw := range body.Items {
		uid, parseErr := strconv.Atoi(string(raw.UserId))
		if parseErr != nil || uid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id in batch", requestID)
			return
		}
		pid, parseErr := strconv.Atoi(string(raw.PlanId))
		if parseErr != nil || pid <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid plan id in batch", requestID)
			return
		}
		sourceType := ""
		if raw.SourceType != nil {
			sourceType = *raw.SourceType
		}
		sourceID := ""
		if raw.SourceId != nil {
			sourceID = *raw.SourceId
		}
		items = append(items, subscriptioncontract.BatchAssignSubscriptionItem{
			UserID:     uid,
			PlanID:     pid,
			ExpiresAt:  raw.ExpiresAt,
			SourceType: sourceType,
			SourceID:   sourceID,
		})
	}

	results, err := s.runtime.subscriptions.BatchAssignSubscriptions(r.Context(), items)
	if err != nil {
		switch {
		case errors.Is(err, subscriptionservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid batch assign request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "batch assign failed", requestID)
		}
		return
	}

	createdCount := 0
	reusedCount := 0
	failedIDs := make([]int, 0)
	rows := make([]apiopenapi.BatchAssignAdminUserSubscriptionsRow, 0, len(results))
	errorRows := make([]apiopenapi.BatchAssignAdminUserSubscriptionsErrorRow, 0)
	for _, row := range results {
		outRow := apiopenapi.BatchAssignAdminUserSubscriptionsRow{
			UserId:  apiopenapi.Id(strconv.Itoa(row.UserID)),
			PlanId:  apiopenapi.Id(strconv.Itoa(row.PlanID)),
			Outcome: apiopenapi.BatchAssignAdminUserSubscriptionsRowOutcome(row.Outcome),
		}
		if row.SubscriptionID > 0 {
			id := apiopenapi.Id(strconv.Itoa(row.SubscriptionID))
			outRow.SubscriptionId = &id
		}
		rows = append(rows, outRow)
		switch row.Outcome {
		case "created":
			createdCount++
		case "reused":
			reusedCount++
		}
		if row.Error != "" {
			errorRows = append(errorRows, apiopenapi.BatchAssignAdminUserSubscriptionsErrorRow{
				Id:      apiopenapi.Id(strconv.Itoa(row.UserID)),
				Message: row.Error,
			})
			failedIDs = append(failedIDs, row.UserID)
		}
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(
		r,
		session.User.ID,
		"user_subscription.batch_assign",
		"user_subscription",
		"",
		nil,
		map[string]any{
			"requested":  len(items),
			"created":    createdCount,
			"reused":     reusedCount,
			"failed":     len(errorRows),
			"failed_ids": failedIDs,
		},
	))

	writeJSONAny(w, http.StatusOK, apiopenapi.BatchAssignAdminUserSubscriptionsResponse{
		Data: apiopenapi.BatchAssignAdminUserSubscriptionsResult{
			CreatedCount: createdCount,
			ReusedCount:  reusedCount,
			Rows:         rows,
			Errors:       errorRows,
		},
		RequestId: apiopenapi.RequestId(requestID),
	})
}
