package httpserver

import (
	"net/http"
	"strconv"

	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminAffiliateInvites(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.affiliate.ListRelationships(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list affiliate invites", requestID)
		return
	}
	data := make([]apiopenapi.AffiliateInviteRecord, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIAffiliateInviteRecord(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateInviteRecordListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminAffiliateRebates(w http.ResponseWriter, r *http.Request) {
	s.handleListAdminAffiliateLedgers(w, r, "rebate")
}

func (s *Server) handleListAdminAffiliateTransfers(w http.ResponseWriter, r *http.Request) {
	s.handleListAdminAffiliateLedgers(w, r, "transfer")
}

func (s *Server) handleListAdminAffiliateRules(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.affiliate.ListRules(r.Context())
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.AffiliateRule, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIAffiliateRule(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateRuleListResponse{
		Data:       data,
		Pagination: paginationWithRequest(r, len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminAffiliateRule(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateAffiliateRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid affiliate rule request", requestID)
		return
	}
	rule, err := s.runtime.affiliate.CreateRule(r.Context(), affiliatecontract.CreateRuleRequest{
		Name:            body.Name,
		Status:          toAffiliateRuleStatusPtr(body.Status),
		TriggerType:     affiliatecontract.TriggerType(body.TriggerType),
		Rate:            optionalStringValue(body.Rate),
		FixedAmount:     optionalStringValue(body.FixedAmount),
		Currency:        optionalStringValue(body.Currency),
		MaxRebateAmount: optionalStringValue(body.MaxRebateAmount),
		ValidFrom:       body.ValidFrom,
		ValidTo:         body.ValidTo,
		Metadata:        jsonObjectValueToMap(optionalJsonObjectValue(body.Metadata)),
	})
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "affiliate_rule.create", "affiliate_rule", strconv.Itoa(rule.ID), nil, affiliateRuleAuditSnapshot(rule)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AffiliateRuleResponse{
		Data:      toAPIAffiliateRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminAffiliateRule(w http.ResponseWriter, r *http.Request) {
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
	ruleID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ruleID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid affiliate rule id", requestID)
		return
	}
	var body apiopenapi.UpdateAffiliateRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid affiliate rule request", requestID)
		return
	}
	rule, err := s.runtime.affiliate.UpdateRule(r.Context(), ruleID, affiliatecontract.UpdateRuleRequest{
		Name:            body.Name,
		Status:          toAffiliateRuleStatusPtr(body.Status),
		TriggerType:     toAffiliateRuleTriggerPtr(body.TriggerType),
		Rate:            body.Rate,
		FixedAmount:     body.FixedAmount,
		Currency:        body.Currency,
		MaxRebateAmount: body.MaxRebateAmount,
		ValidFrom:       body.ValidFrom,
		ValidTo:         body.ValidTo,
		Metadata:        jsonObjectToMapPtr(body.Metadata),
	})
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "affiliate_rule.update", "affiliate_rule", strconv.Itoa(rule.ID), nil, affiliateRuleAuditSnapshot(rule)))
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateRuleResponse{
		Data:      toAPIAffiliateRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleCreateAdminAffiliateManualAdjustment(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.AdminAffiliateManualAdjustmentRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid affiliate manual adjustment request", requestID)
		return
	}
	userID, err := strconv.Atoi(string(body.UserId))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	ledger, _, err := s.runtime.affiliate.CreateManualAdjustment(r.Context(), affiliatecontract.ManualAdjustmentRequest{
		AdminUserID: session.User.ID,
		UserID:      userID,
		Amount:      body.Amount,
		Currency:    optionalStringValue(body.Currency),
		Reason:      body.Reason,
		ReferenceID: optionalStringValue(body.ReferenceId),
		Metadata:    jsonObjectValueToMap(optionalJsonObjectValue(body.Metadata)),
	})
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "affiliate_manual_adjustment.create", "affiliate_ledger", strconv.Itoa(ledger.ID), nil, affiliateLedgerAuditSnapshot(ledger)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AffiliateLedgerEntryResponse{
		Data:      toAPIAffiliateLedgerEntry(ledger),
		RequestId: requestID,
	})
}

func (s *Server) handleApproveAdminAffiliateWithdrawal(w http.ResponseWriter, r *http.Request) {
	s.handleAdminAffiliateWithdrawalDecision(w, r, true)
}

func (s *Server) handleCancelAdminAffiliateWithdrawal(w http.ResponseWriter, r *http.Request) {
	s.handleAdminAffiliateWithdrawalDecision(w, r, false)
}

func (s *Server) handleAdminAffiliateWithdrawalDecision(w http.ResponseWriter, r *http.Request, approve bool) {
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
	ledgerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || ledgerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid affiliate withdrawal id", requestID)
		return
	}
	var body apiopenapi.AdminAffiliateWithdrawalDecisionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid affiliate withdrawal decision request", requestID)
		return
	}
	req := affiliatecontract.WithdrawDecisionRequest{
		AdminUserID: session.User.ID,
		LedgerID:    ledgerID,
		Reason:      optionalStringValue(body.Reason),
	}
	var ledger affiliatecontract.AffiliateLedger
	if approve {
		ledger, err = s.runtime.affiliate.ApproveWithdraw(r.Context(), req)
	} else {
		ledger, err = s.runtime.affiliate.CancelWithdraw(r.Context(), req)
	}
	if err != nil {
		writeAffiliateServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateLedgerEntryResponse{
		Data:      toAPIAffiliateLedgerEntry(ledger),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminAffiliateLedgers(w http.ResponseWriter, r *http.Request, view string) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	userID, ok := optionalUserIDQuery(w, r, requestID)
	if !ok {
		return
	}
	items, err := s.runtime.affiliate.ListLedgers(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list affiliate ledger", requestID)
		return
	}
	data := make([]apiopenapi.AffiliateLedgerEntry, 0, len(items))
	for _, item := range items {
		if userID != nil && item.UserID != *userID {
			continue
		}
		if !affiliateLedgerMatchesView(item.Type, view) {
			continue
		}
		data = append(data, toAPIAffiliateLedgerEntry(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AffiliateLedgerEntryListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func toAffiliateRuleStatusPtr(value *apiopenapi.AffiliateRuleStatus) *affiliatecontract.RuleStatus {
	if value == nil {
		return nil
	}
	status := affiliatecontract.RuleStatus(*value)
	return &status
}

func toAffiliateRuleTriggerPtr(value *apiopenapi.AffiliateRuleTriggerType) *affiliatecontract.TriggerType {
	if value == nil {
		return nil
	}
	trigger := affiliatecontract.TriggerType(*value)
	return &trigger
}

func optionalJsonObjectValue(value *apiopenapi.JsonObject) apiopenapi.JsonObject {
	if value == nil {
		return apiopenapi.JsonObject{}
	}
	return *value
}

func affiliateRuleAuditSnapshot(rule affiliatecontract.AffiliateRule) map[string]any {
	return map[string]any{
		"id":                rule.ID,
		"name":              rule.Name,
		"status":            string(rule.Status),
		"trigger_type":      string(rule.TriggerType),
		"rate":              rule.Rate,
		"fixed_amount":      rule.FixedAmount,
		"currency":          rule.Currency,
		"max_rebate_amount": rule.MaxRebateAmount,
	}
}

func affiliateLedgerAuditSnapshot(ledger affiliatecontract.AffiliateLedger) map[string]any {
	return map[string]any{
		"id":           ledger.ID,
		"user_id":      ledger.UserID,
		"type":         string(ledger.Type),
		"amount":       ledger.Amount,
		"currency":     ledger.Currency,
		"status":       string(ledger.Status),
		"reference_id": ledger.ReferenceID,
	}
}

func optionalUserIDQuery(w http.ResponseWriter, r *http.Request, requestID string) (*int, bool) {
	raw := r.URL.Query().Get("user_id")
	if raw == "" {
		return nil, true
	}
	userID, err := strconv.Atoi(raw)
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return nil, false
	}
	return &userID, true
}

func affiliateLedgerMatchesView(value affiliatecontract.LedgerType, view string) bool {
	switch view {
	case "rebate":
		return value == affiliatecontract.LedgerTypeAccrue || value == affiliatecontract.LedgerTypeRefundCompensation
	case "transfer":
		return value == affiliatecontract.LedgerTypeTransferToBalance || value == affiliatecontract.LedgerTypeWithdraw
	case "manual_adjustment":
		return value == affiliatecontract.LedgerTypeManualAdjustment
	default:
		return false
	}
}
