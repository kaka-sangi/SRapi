package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleListAdminUsageLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	items = filterUsageLogs(items, r.URL.Query().Get("user_id"), r.URL.Query().Get("model"))
	data := make([]apiopenapi.UsageLog, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIUsageLog(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UsageLogListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.audit.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list audit logs", requestID)
		return
	}
	items = filterAuditLogs(items, r.URL.Query().Get("action"), r.URL.Query().Get("resource_type"))
	data := make([]apiopenapi.AuditLog, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIAuditLog(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AuditLogListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminBillingLedger(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.billing.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list billing ledger", requestID)
		return
	}
	items = filterBillingLedger(items, r.URL.Query().Get("user_id"), r.URL.Query().Get("reference_type"))
	data := make([]apiopenapi.BillingLedgerEntry, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIBillingLedgerEntry(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.BillingLedgerListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminPaymentProviders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.payments.ListProviderInstances(r.Context())
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentProviderInstance, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentProviderInstance(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentProviderInstanceListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminPaymentProvider(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePaymentProviderInstanceRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment provider request", requestID)
		return
	}
	provider, err := s.runtime.payments.CreateProviderInstance(r.Context(), paymentcontract.CreateProviderInstanceRequest{
		Provider:         body.Provider,
		Name:             body.Name,
		Status:           toPaymentProviderStatusPtr(body.Status),
		Config:           jsonObjectValueToMap(body.Config),
		SupportedMethods: derefStrings(body.SupportedMethods),
		Limits:           jsonObjectToMap(body.Limits),
		SortOrder:        body.SortOrder,
		Metadata:         jsonObjectToMap(body.Metadata),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "payment_provider.create", "payment_provider", strconv.Itoa(provider.ID), nil, map[string]any{
		"provider":          provider.Provider,
		"name":              provider.Name,
		"status":            provider.Status,
		"supported_methods": provider.SupportedMethods,
		"sort_order":        provider.SortOrder,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.PaymentProviderInstanceResponse{
		Data:      toAPIPaymentProviderInstance(provider),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminPaymentOrders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	var (
		items []paymentcontract.PaymentOrder
		err   error
	)
	if userIDRaw := strings.TrimSpace(r.URL.Query().Get("user_id")); userIDRaw != "" {
		userID, parseErr := strconv.Atoi(userIDRaw)
		if parseErr != nil || userID <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
			return
		}
		items, err = s.runtime.payments.ListOrdersByUser(r.Context(), userID)
	} else {
		items, err = s.runtime.payments.ListOrders(r.Context())
	}
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	items = filterPaymentOrders(items, r.URL.Query().Get("status"))
	data := make([]apiopenapi.PaymentOrder, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentOrder(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleRefundAdminPaymentOrder(w http.ResponseWriter, r *http.Request) {
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
	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment order id", requestID)
		return
	}
	var body apiopenapi.RefundPaymentOrderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid refund request", requestID)
		return
	}
	order, err := s.runtime.payments.RequestRefund(r.Context(), paymentcontract.RefundRequest{
		ActorUserID: session.User.ID,
		OrderID:     orderID,
		Amount:      optionalStringValue(body.Amount),
		Reason:      optionalStringValue(body.Reason),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.subscriptions.ListPlans(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list subscription plans", requestID)
		return
	}
	data := make([]apiopenapi.SubscriptionPlan, 0, len(items))
	for _, item := range items {
		data = append(data, toAPISubscriptionPlan(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SubscriptionPlanListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateSubscriptionPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid subscription plan request", requestID)
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	plan, err := s.runtime.subscriptions.CreatePlan(r.Context(), subscriptioncontract.CreatePlanRequest{
		Name:         body.Name,
		Description:  description,
		Price:        body.Price,
		Currency:     body.Currency,
		ValidityDays: body.ValidityDays,
		Entitlements: jsonObjectToMap(body.Entitlements),
		ForSale:      body.ForSale,
		SortOrder:    body.SortOrder,
		Status:       toSubscriptionPlanStatusPtr(body.Status),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid subscription plan request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "subscription_plan.create", "subscription_plan", strconv.Itoa(plan.ID), nil, subscriptionPlanAuditSnapshot(plan)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.SubscriptionPlanResponse{
		Data:      toAPISubscriptionPlan(plan),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	var (
		items []subscriptioncontract.UserSubscription
		err   error
	)
	if userIDRaw := strings.TrimSpace(r.URL.Query().Get("user_id")); userIDRaw != "" {
		userID, parseErr := strconv.Atoi(userIDRaw)
		if parseErr != nil || userID <= 0 {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
			return
		}
		items, err = s.runtime.subscriptions.ListUserSubscriptionsByUser(r.Context(), userID)
	} else {
		items, err = s.runtime.subscriptions.ListUserSubscriptions(r.Context())
	}
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list user subscriptions", requestID)
		return
	}
	data := make([]apiopenapi.UserSubscription, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIUserSubscription(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserSubscriptionListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminUserSubscription(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateUserSubscriptionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user subscription request", requestID)
		return
	}
	userID, err := strconv.Atoi(string(body.UserId))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	planID, err := strconv.Atoi(string(body.PlanId))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid plan id", requestID)
		return
	}
	if _, err := s.runtime.users.FindByID(r.Context(), userID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "user not found", requestID)
		return
	}
	sourceType := ""
	if body.SourceType != nil {
		sourceType = *body.SourceType
	}
	sourceID := ""
	if body.SourceId != nil {
		sourceID = *body.SourceId
	}
	subscription, err := s.runtime.subscriptions.CreateUserSubscription(r.Context(), subscriptioncontract.CreateSubscriptionRequest{
		UserID:     userID,
		PlanID:     planID,
		Status:     toUserSubscriptionStatusPtr(body.Status),
		StartsAt:   body.StartsAt,
		ExpiresAt:  body.ExpiresAt,
		SourceType: sourceType,
		SourceID:   sourceID,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user subscription request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_subscription.create", "user_subscription", strconv.Itoa(subscription.ID), nil, userSubscriptionAuditSnapshot(subscription)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.UserSubscriptionResponse{
		Data:      toAPIUserSubscription(subscription),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminPricingRules(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.subscriptions.ListPricingRules(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list pricing rules", requestID)
		return
	}
	data := make([]apiopenapi.PricingRule, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPricingRule(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PricingRuleListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminPricingRule(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreatePricingRuleRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid pricing rule request", requestID)
		return
	}
	modelID, err := strconv.Atoi(string(body.ModelId))
	if err != nil || modelID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil || providerID < 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	if _, err := s.runtime.models.FindByID(r.Context(), modelID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "model not found", requestID)
		return
	}
	if providerID > 0 {
		if _, err := s.runtime.providers.FindByID(r.Context(), providerID); err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
			return
		}
	}
	rule, err := s.runtime.subscriptions.CreatePricingRule(r.Context(), subscriptioncontract.CreatePricingRuleRequest{
		ModelID:                         modelID,
		ProviderID:                      providerID,
		InputPricePerMillionTokens:      body.InputPricePerMillionTokens,
		OutputPricePerMillionTokens:     body.OutputPricePerMillionTokens,
		CacheReadPricePerMillionTokens:  body.CacheReadPricePerMillionTokens,
		CacheWritePricePerMillionTokens: body.CacheWritePricePerMillionTokens,
		Currency:                        body.Currency,
		EffectiveFrom:                   body.EffectiveFrom,
		EffectiveTo:                     body.EffectiveTo,
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid pricing rule request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "pricing_rule.create", "pricing_rule", strconv.Itoa(rule.ID), nil, pricingRuleAuditSnapshot(rule)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.PricingRuleResponse{
		Data:      toAPIPricingRule(rule),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminOutboxEvents(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.events.ListOutbox(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list outbox events", requestID)
		return
	}
	items = filterOutboxEvents(items, r.URL.Query().Get("status"), r.URL.Query().Get("event_type"))
	data := make([]apiopenapi.DomainEventOutbox, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIDomainEventOutbox(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.DomainEventOutboxListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminOpsRealtimeSlots(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	list := s.runtime.realtime.ListActiveSlots(r.Context())
	data := make([]apiopenapi.RealtimeActiveSlot, 0, len(list.Slots))
	for _, slot := range list.Slots {
		data = append(data, toAPIRealtimeActiveSlot(slot))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.RealtimeActiveSlotListResponse{
		Counters:   toAPIRealtimeActiveSlotCounters(list),
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminOpsSLOs(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.operations.ListSLOs(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.OpsSLO, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsSLO(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSLOListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminOpsSLO(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateOpsSLORequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	createReq, err := toCreateSLORequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	created, err := s.runtime.operations.CreateSLO(r.Context(), createReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_slo.create", "ops_slo", strconv.Itoa(created.ID), nil, opsSLOAuditSnapshot(created)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.OpsSLODefinitionResponse{
		Data:      toAPIOpsSLODefinition(created),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminOpsSLO(w http.ResponseWriter, r *http.Request) {
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
	sloID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || sloID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo id", requestID)
		return
	}
	current, err := s.runtime.operations.ListSLOs(r.Context())
	var beforeSnapshot map[string]any
	if err == nil {
		for _, item := range current {
			if item.Definition.ID == sloID {
				beforeSnapshot = opsSLOAuditSnapshot(item.Definition)
				break
			}
		}
	}
	var body apiopenapi.UpdateOpsSLORequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	updateReq, err := toUpdateSLORequest(body)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid slo request", requestID)
		return
	}
	updated, err := s.runtime.operations.UpdateSLO(r.Context(), sloID, updateReq)
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_slo.update", "ops_slo", strconv.Itoa(updated.ID), beforeSnapshot, opsSLOAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsSLODefinitionResponse{
		Data:      toAPIOpsSLODefinition(updated),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminOpsAlerts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.operations.ListAlerts(r.Context())
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	items = filterOpsAlerts(items, r.URL.Query().Get("status"), r.URL.Query().Get("severity"))
	data := make([]apiopenapi.OpsAlertEvent, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIOpsAlert(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleAcknowledgeAdminOpsAlert(w http.ResponseWriter, r *http.Request) {
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
	alertID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || alertID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid alert id", requestID)
		return
	}
	acknowledged, err := s.runtime.operations.AcknowledgeAlert(r.Context(), alertID, operationscontract.AckAlertRequest{
		ActorUserID: session.User.ID,
	})
	if err != nil {
		writeOperationsServiceError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "ops_alert.ack", "ops_alert", strconv.Itoa(acknowledged.ID), nil, opsAlertAckAuditSnapshot(acknowledged)))
	writeJSONAny(w, http.StatusOK, apiopenapi.OpsAlertResponse{
		Data:      toAPIOpsAlert(acknowledged),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminCapabilities(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items := filterCapabilityDefinitions(s.runtime.capabilities, r.URL.Query().Get("category"), r.URL.Query().Get("status"))
	data := make([]apiopenapi.CapabilityDefinition, 0, len(items))
	for _, item := range items {
		data = append(data, toAPICapabilityDefinition(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.CapabilityListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleAdminSchedulerOverview(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	decisions, err := s.runtime.scheduler.ListDecisions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerOverviewResponse{
		Data:      buildSchedulerOverview(decisions, usageLogs),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminSchedulerDecisions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	items, err := s.runtime.scheduler.ListDecisions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	items = filterSchedulerDecisions(items, r.URL.Query().Get("request_id"), r.URL.Query().Get("model"))
	data := make([]apiopenapi.SchedulerDecision, 0, len(items))
	for _, item := range items {
		data = append(data, toAPISchedulerDecision(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerDecisionListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListSchedulerStrategies(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	now := time.Now().UTC()
	strategies := s.runtime.scheduler.ListStrategies()
	data := make([]apiopenapi.SchedulerStrategy, 0, len(strategies))
	for index, strategy := range strategies {
		data = append(data, apiopenapi.SchedulerStrategy{
			Id:          apiopenapi.Id(strconv.Itoa(index + 1)),
			Name:        apiopenapi.SchedulerStrategyName(strategy.Name),
			Version:     strategy.Version,
			Status:      apiopenapi.SchedulerStrategyStatus(strategy.Status),
			Config:      jsonObject(strategy.Config),
			ConfigHash:  strategy.ConfigHash,
			CreatedAt:   now,
			ActivatedAt: &now,
		})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.SchedulerStrategyListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}
