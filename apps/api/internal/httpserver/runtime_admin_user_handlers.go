package httpserver

import (
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	s.handleAdminOverview(w, r)
}

func (s *Server) handleListAdminUsers(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	status := optionalUserStatusFromQuery(r)
	var rolePtr *userscontract.Role
	if raw := strings.TrimSpace(r.URL.Query().Get("role")); raw != "" {
		role := userscontract.Role(raw)
		rolePtr = &role
	}
	users, err := s.runtime.users.List(r.Context(), usersservice.ListRequest{
		Status: status,
		Query:  r.URL.Query().Get("q"),
		Role:   rolePtr,
	})
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.User, 0, len(users))
	for _, user := range users {
		data = append(data, toAPIUser(user.User))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.UserListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminUser(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateAdminUserRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user request", requestID)
		return
	}
	user, err := s.runtime.users.Create(r.Context(), usersservice.CreateRequest{
		Email:    string(body.Email),
		Name:     body.Name,
		Password: body.Password,
		Roles:    apiUserRoles(body.Roles),
		Status:   toUserStatusPtr(body.Status),
		Balance:  optionalStringValue(body.Balance),
		Currency: optionalStringValue(body.Currency),
		RPMLimit: body.RpmLimit,
	})
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.create", "user", strconv.Itoa(user.ID), nil, userAuditSnapshot(user)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.UserResponse{
		Data:      toAPIUser(user.User),
		RequestId: requestID,
	})
}

func (s *Server) handleGetAdminUser(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	user, err := s.runtime.users.FindByID(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(user.User),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminUser(w http.ResponseWriter, r *http.Request) {
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
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	before, err := s.runtime.users.FindByID(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	var body apiopenapi.UpdateAdminUserRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user update request", requestID)
		return
	}
	updated, err := s.runtime.users.Update(r.Context(), userID, usersservice.UpdateRequest{
		Email:    optionalEmailString(body.Email),
		Name:     body.Name,
		Password: body.Password,
		Roles:    apiUserRolesPtr(body.Roles),
		Status:   toUserStatusPtr(body.Status),
		RPMLimit: optionalIntPtr(body.RpmLimit),
	})
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.update", "user", strconv.Itoa(updated.ID), userAuditSnapshot(before), userAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(updated.User),
		RequestId: requestID,
	})
}

func (s *Server) handleDeleteAdminUser(w http.ResponseWriter, r *http.Request) {
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
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	// Self-delete would log the acting admin out and could lock the panel
	// (e.g. if they are the only owner) — refuse it.
	if userID == session.User.ID {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "cannot delete your own account", requestID)
		return
	}
	before, err := s.runtime.users.FindByID(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	// Revoke all API keys before soft-deleting the user so that keys
	// belonging to the deleted user can no longer authenticate requests.
	if err := s.runtime.apiKeys.RevokeByUser(r.Context(), userID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to revoke user api keys", requestID)
		return
	}
	// Invalidate all active console sessions so the deleted user is
	// immediately signed out everywhere.
	if err := s.runtime.auth.LogoutUser(r.Context(), userID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to invalidate user sessions", requestID)
		return
	}
	if err := s.runtime.users.Delete(r.Context(), userID); err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.delete", "user", strconv.Itoa(userID), userAuditSnapshot(before), nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": userID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleUpdateAdminUserBalance(w http.ResponseWriter, r *http.Request) {
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
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	before, err := s.runtime.users.FindByID(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	var body apiopenapi.UpdateUserBalanceRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid balance update request", requestID)
		return
	}
	updated, err := s.runtime.users.UpdateBalance(r.Context(), userID, usersservice.BalanceUpdateRequest{
		Operation: usersservice.BalanceOperation(body.Operation),
		Amount:    body.Amount,
		Currency:  optionalStringValue(body.Currency),
	})
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	ledgerAmount := adminBalanceLedgerAmount(body.Operation, before.Balance, updated.Balance)
	_, err = s.runtime.billing.Record(r.Context(), billingcontract.RecordRequest{
		UserID:        updated.ID,
		Type:          billingcontract.LedgerTypeAdjustment,
		Amount:        ledgerAmount,
		Currency:      updated.Currency,
		BalanceBefore: before.Balance,
		BalanceAfter:  updated.Balance,
		ReferenceType: "admin_balance_update",
		ReferenceID:   requestID,
		Metadata: map[string]any{
			"operation": string(body.Operation),
			"note":      optionalStringValue(body.Note),
			"actor_id":  session.User.ID,
		},
	})
	if err != nil {
		// Balance was already changed -- returning an error would be
		// misleading because the caller might retry and double-apply.
		// Return success with a warning so the admin knows the ledger
		// entry failed and can reconcile manually.
		s.logger.Error("CRITICAL: billing ledger record failed after successful balance update — ledger is now inconsistent",
			"error", err, "user_id", updated.ID, "request_id", requestID,
			"balance_before", before.Balance, "balance_after", updated.Balance)
		s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.balance_update", "user", strconv.Itoa(updated.ID), userAuditSnapshot(before), userAuditSnapshot(updated)))
		writeJSONAny(w, http.StatusOK, map[string]any{
			"data":       toAPIUser(updated.User),
			"warning":    "balance updated but ledger record failed — check server logs",
			"request_id": requestID,
		})
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.balance_update", "user", strconv.Itoa(updated.ID), userAuditSnapshot(before), userAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(updated.User),
		RequestId: requestID,
	})
}

func (s *Server) handleAdminUserBalanceHistory(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	limit, offset, page, pageSize := paginationParams(r)
	result, err := s.runtime.billing.ListPage(r.Context(), billingcontract.LedgerListFilter{
		UserID: &userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.logger.Error("failed to list balance history", "error", err, "user_id", userID, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list balance history", requestID)
		return
	}
	data := make([]apiopenapi.BillingLedgerEntry, 0, len(result.Items))
	for _, item := range result.Items {
		data = append(data, toAPIBillingLedgerEntry(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.BillingLedgerListResponse{
		Data:       data,
		Pagination: paginationFromTotal(result.Total, page, pageSize),
		RequestId:  requestID,
	})
}

func (s *Server) handleUpdateAdminUserRpmLimit(w http.ResponseWriter, r *http.Request) {
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
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	before, err := s.runtime.users.FindByID(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	var body apiopenapi.UpdateUserRpmLimitRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid rpm limit request", requestID)
		return
	}
	updated, err := s.runtime.users.UpdateRPMLimit(r.Context(), userID, body.RpmLimit)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.rpm_limit_update", "user", strconv.Itoa(updated.ID), userAuditSnapshot(before), userAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(updated.User),
		RequestId: requestID,
	})
}

func (s *Server) handleEnableAdminUser(w http.ResponseWriter, r *http.Request) {
	s.handleAdminUserStatus(w, r, userscontract.StatusActive, "user.enable")
}

func (s *Server) handleDisableAdminUser(w http.ResponseWriter, r *http.Request) {
	s.handleAdminUserStatus(w, r, userscontract.StatusDisabled, "user.disable")
}

func (s *Server) handleAdminUserStatus(w http.ResponseWriter, r *http.Request, status userscontract.Status, action string) {
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
	userID, ok := adminPathID(w, r, requestID, "user")
	if !ok {
		return
	}
	before, err := s.runtime.users.FindByID(r.Context(), userID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	updated, err := s.runtime.users.SetStatus(r.Context(), userID, status)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, action, "user", strconv.Itoa(updated.ID), userAuditSnapshot(before), userAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(updated.User),
		RequestId: requestID,
	})
}

func (s *Server) handleBatchUpdateAdminUsers(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.BatchUpdateUsersRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid batch user update request", requestID)
		return
	}
	userIDs, err := apiIDsValueToInts(body.UserIds)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user ids", requestID)
		return
	}
	result := s.runtime.users.BatchUpdate(r.Context(), usersservice.BatchUpdateRequest{
		UserIDs:  userIDs,
		Status:   toUserStatusPtr(body.Status),
		Roles:    apiUserRolesPtr(body.Roles),
		RPMLimit: optionalIntPtr(body.RpmLimit),
	})
	updatedIDs := make([]apiopenapi.Id, 0, len(result.Updated))
	for _, updated := range result.Updated {
		updatedIDs = append(updatedIDs, apiopenapi.Id(strconv.Itoa(updated.ID)))
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user.batch_update", "user", "bulk", nil, map[string]any{
		"user_ids":      userIDs,
		"updated_ids":   updatedIDs,
		"updated_count": len(updatedIDs),
		"errors":        result.Errors,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchUpdateUsersResponse{
		Data: apiopenapi.BatchUpdateUsersResult{
			Errors:       result.Errors,
			UpdatedCount: len(updatedIDs),
			UpdatedIds:   updatedIDs,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleAdminUsageDaily(w http.ResponseWriter, r *http.Request) {
	s.handleAdminUsageAggregatesWithDimension(w, r, usagecontract.AggregateDimensionDay)
}

func (s *Server) handleAdminUsageAggregates(w http.ResponseWriter, r *http.Request) {
	dimension := usagecontract.AggregateDimension(strings.TrimSpace(r.URL.Query().Get("dimension")))
	s.handleAdminUsageAggregatesWithDimension(w, r, dimension)
}

func (s *Server) handleAdminUsageAggregatesWithDimension(w http.ResponseWriter, r *http.Request, dimension usagecontract.AggregateDimension) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	filter, err := adminUsageDateFilter(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid usage date range", requestID)
		return
	}
	aggregates, err := s.runtime.usage.Aggregate(r.Context(), filter, dimension)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid usage aggregate request", requestID)
		return
	}
	data := toAPIUsageAggregates(aggregates)
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageAggregateListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
	})
}

func (s *Server) handleAdminUsageExport(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	filter, err := adminUsageDateFilter(r)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid usage date range", requestID)
		return
	}
	exported, err := s.runtime.usage.Export(r.Context(), filter)
	if err != nil {
		s.logger.Error("failed to export usage", "error", err, "request_id", requestID)
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to export usage", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageExportResponse{
		Data:      toAPIUsageExport(exported),
		RequestId: requestID,
	})
}

func optionalUserStatusFromQuery(r *http.Request) *userscontract.Status {
	value := strings.TrimSpace(r.URL.Query().Get("status"))
	if value == "" {
		return nil
	}
	status := userscontract.Status(value)
	return &status
}

func optionalEmailString(value *openapi_types.Email) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

func apiUserRoles(value *[]apiopenapi.UserRole) []userscontract.Role {
	if value == nil {
		return nil
	}
	out := make([]userscontract.Role, 0, len(*value))
	for _, role := range *value {
		out = append(out, userscontract.Role(role))
	}
	return out
}

func apiUserRolesPtr(value *[]apiopenapi.UserRole) *[]userscontract.Role {
	if value == nil {
		return nil
	}
	out := apiUserRoles(value)
	return &out
}

func optionalIntPtr(value *int) **int {
	if value == nil {
		return nil
	}
	return &value
}

func adminPathID(w http.ResponseWriter, r *http.Request, requestID string, label string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid "+label+" id", requestID)
		return 0, false
	}
	return id, true
}

func apiIDsValueToInts(values []apiopenapi.Id) ([]int, error) {
	out := make([]int, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.Atoi(string(value))
		if err != nil || parsed <= 0 {
			return nil, errors.New("invalid id")
		}
		out = append(out, parsed)
	}
	return out, nil
}

func writeUserServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, usersservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user request", requestID)
	case errors.Is(err, usersservice.ErrInvalidCredentials):
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid credentials", requestID)
	case errors.Is(err, usersservice.ErrUserDisabled):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
	case errors.Is(err, usersservice.ErrUserNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user not found", requestID)
	case errors.Is(err, usersservice.ErrIdentityNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user auth identity not found", requestID)
	case errors.Is(err, usersservice.ErrIdentityAlreadyBound):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "user auth identity already bound", requestID)
	case errors.Is(err, usersservice.ErrIdentityUnbindBlocked):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "bind another sign-in method before unbinding this identity", requestID)
	case errors.Is(err, usersservice.ErrUserAlreadyExists):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "user already exists", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "user service failed", requestID)
	}
}

func writeChangePasswordError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, usersservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid password change request", requestID)
	case errors.Is(err, usersservice.ErrInvalidCredentials):
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid credentials", requestID)
	case errors.Is(err, usersservice.ErrUserDisabled):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
	case errors.Is(err, usersservice.ErrUserNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user not found", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "password change failed", requestID)
	}
}

func adminUsageDateFilter(r *http.Request) (usagecontract.QueryFilter, error) {
	var filter usagecontract.QueryFilter
	if start := strings.TrimSpace(r.URL.Query().Get("start")); start != "" {
		parsed, err := time.Parse("2006-01-02", start)
		if err != nil {
			return filter, err
		}
		filter.Start = &parsed
	}
	if end := strings.TrimSpace(r.URL.Query().Get("end")); end != "" {
		parsed, err := time.Parse("2006-01-02", end)
		if err != nil {
			return filter, err
		}
		endExclusive := parsed.AddDate(0, 0, 1)
		filter.End = &endExclusive
	}
	return filter, nil
}

func adminBalanceLedgerAmount(operation apiopenapi.UpdateUserBalanceRequestOperation, before string, after string) string {
	switch operation {
	case apiopenapi.Set:
		return subtractDecimalStrings(after, before)
	case apiopenapi.Increment:
		return subtractDecimalStrings(after, before)
	case apiopenapi.Decrement:
		return subtractDecimalStrings(after, before)
	default:
		return "0.00000000"
	}
}

func addDecimalStrings(left string, right string) string {
	leftRat, ok := decimalStringRat(left)
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := decimalStringRat(right)
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Add(leftRat, rightRat).FloatString(8)
}

func subtractDecimalStrings(left string, right string) string {
	leftRat, ok := decimalStringRat(left)
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := decimalStringRat(right)
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Sub(leftRat, rightRat).FloatString(8)
}

func decimalStringRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	return new(big.Rat).SetString(value)
}
