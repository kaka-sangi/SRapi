package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	var body apiopenapi.LoginRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid login request", requestID)
		return
	}

	result, err := s.runtime.auth.Login(r.Context(), string(body.Email), body.Password)
	if err != nil {
		switch {
		case errors.Is(err, usersservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid login request", requestID)
		case errors.Is(err, usersservice.ErrInvalidCredentials):
			writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "invalid credentials", requestID)
		case errors.Is(err, usersservice.ErrUserDisabled):
			writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "user disabled", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to login", requestID)
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    result.Session.ID,
		Path:     "/",
		Expires:  result.Session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})

	writeJSONAny(w, http.StatusOK, apiopenapi.LoginResponse{
		Data: apiopenapi.SessionData{
			CsrfToken: result.Session.CSRFToken,
			ExpiresAt: result.Session.ExpiresAt,
			User:      toAPIUser(result.User),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	if err := s.runtime.auth.Logout(r.Context(), session.Session.ID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to logout", requestID)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Server.Mode != "local",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCurrentUser(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}

	writeJSONAny(w, http.StatusOK, apiopenapi.UserResponse{
		Data:      toAPIUser(session.User),
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserBalance(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	user, err := s.runtime.users.FindByID(r.Context(), session.User.ID)
	if err != nil {
		writeUserServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.UserBalanceResponse{
		Data: apiopenapi.UserBalance{
			UserId:   apiopenapi.Id(strconv.Itoa(user.ID)),
			Balance:  user.Balance,
			Currency: user.Currency,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCurrentUserUsage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.usage.ListByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
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

func (s *Server) handleCurrentUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.subscriptions.ListUserSubscriptionsByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list subscriptions", requestID)
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

func (s *Server) handleListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireConsoleSession(r); err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.payments.ListMethods(r.Context())
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	data := make([]apiopenapi.PaymentMethod, 0, len(items))
	for _, item := range items {
		data = append(data, toAPIPaymentMethod(item))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentMethodListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreatePaymentOrder(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body apiopenapi.CreatePaymentOrderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment order request", requestID)
		return
	}
	order, err := s.runtime.payments.CreateOrder(r.Context(), paymentcontract.CreateOrderRequest{
		UserID:      session.User.ID,
		Method:      body.Method,
		Amount:      body.Amount,
		Currency:    optionalStringValue(body.Currency),
		ProductType: paymentcontract.ProductType(body.ProductType),
		ProductID:   optionalStringValue(body.ProductId),
		ExpiresAt:   body.ExpiresAt,
		Metadata:    jsonObjectToMap(body.Metadata),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusCreated, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handleListPaymentOrders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.payments.ListOrdersByUser(r.Context(), session.User.ID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
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

func (s *Server) handleGetPaymentOrder(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment order id", requestID)
		return
	}
	order, err := s.runtime.payments.FindOrderByID(r.Context(), orderID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	if order.UserID != session.User.ID {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "payment order not found", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handleCancelPaymentOrder(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
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
	order, err := s.runtime.payments.CancelOrder(r.Context(), session.User.ID, orderID)
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentOrderResponse{
		Data:      toAPIPaymentOrder(order),
		RequestId: requestID,
	})
}

func (s *Server) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	provider := strings.TrimSpace(r.PathValue("provider"))
	var body apiopenapi.PaymentWebhookRequest
	if provider == "stripe" || provider == "wechat" {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize))
		if err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
			return
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
			return
		}
		body["raw_body"] = string(raw)
	} else {
		if err := s.decodeJSONBody(w, r, &body); err != nil {
			writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
			return
		}
	}
	result, err := s.runtime.payments.HandleWebhook(r.Context(), paymentcontract.WebhookRequest{
		Provider: provider,
		Headers:  singleValueHeaders(r.Header),
		Payload:  map[string]any(body),
	})
	if err != nil {
		writePaymentServiceError(w, err, requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PaymentWebhookResponse{
		Data: apiopenapi.PaymentWebhookResult{
			Handled: result.Handled,
			Order:   toAPIPaymentOrder(result.Order),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleListApiKeys(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}

	keys, err := s.runtime.apiKeys.ListByUser(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list api keys", requestID)
		return
	}
	keys = filterApiKeys(keys, r.URL.Query().Get("status"))
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].CreatedAt.Before(keys[j].CreatedAt)
	})

	page := 1
	pageSize := 20
	if params := r.URL.Query().Get("page"); params != "" {
		if v, err := strconv.Atoi(params); err == nil && v > 0 {
			page = v
		}
	}
	if params := r.URL.Query().Get("page_size"); params != "" {
		if v, err := strconv.Atoi(params); err == nil && v > 0 {
			pageSize = v
		}
	}

	paged, total, hasNext := paginateApiKeys(keys, page, pageSize)
	data := make([]apiopenapi.ApiKey, 0, len(paged))
	for _, key := range paged {
		data = append(data, toAPIKey(key))
	}

	writeJSONAny(w, http.StatusOK, apiopenapi.ApiKeyListResponse{
		Data: data,
		Pagination: apiopenapi.Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
			HasNext:  hasNext,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleCreateApiKey(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}

	var body apiopenapi.CreateApiKeyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid api key request", requestID)
		return
	}

	groupIDs, err := idsToInts(body.GroupIds)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group ids", requestID)
		return
	}

	result, err := s.runtime.apiKeys.Create(r.Context(), apikeycontract.CreateRequest{
		UserID:           session.User.ID,
		Name:             body.Name,
		Scopes:           derefStrings(body.Scopes),
		AllowedModels:    derefStrings(body.AllowedModels),
		GroupIDs:         groupIDs,
		RPMLimit:         body.RpmLimit,
		TPMLimit:         body.TpmLimit,
		ConcurrencyLimit: body.ConcurrencyLimit,
		ExpiresAt:        body.ExpiresAt,
	})
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create api key", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "api_key.create", "api_key", strconv.Itoa(result.Key.ID), nil, map[string]any{
		"name":           result.Key.Name,
		"prefix":         result.Key.Prefix,
		"scopes":         result.Key.Scopes,
		"allowed_models": result.Key.AllowedModels,
	}))

	writeJSONAny(w, http.StatusCreated, apiopenapi.CreateApiKeyResponse{
		Data: apiopenapi.ApiKeySecretData{
			ApiKey:       toAPIKey(result.Key),
			PlaintextKey: result.PlaintextKey,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateApiKey(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}

	keyID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key id", requestID)
		return
	}
	before, err := s.apiKeyByUser(r.Context(), session.User.ID, keyID)
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrKeyNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to load api key", requestID)
		}
		return
	}

	var body apiopenapi.UpdateApiKeyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid api key update request", requestID)
		return
	}

	var groupIDs *[]int
	if body.GroupIds != nil {
		parsed, err := idsToInts(body.GroupIds)
		if err != nil {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid group ids", requestID)
			return
		}
		groupIDs = &parsed
	}

	updated, err := s.runtime.apiKeys.Update(r.Context(), apikeycontract.UpdateRequest{
		UserID:        session.User.ID,
		KeyID:         keyID,
		Name:          body.Name,
		Status:        toAPIKeyStatusPtr(body.Status),
		Scopes:        body.Scopes,
		AllowedModels: body.AllowedModels,
		GroupIDs:      groupIDs,
	})
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrKeyNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "api key not found", requestID)
		case errors.Is(err, apikeyservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid api key update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update api key", requestID)
		}
		return
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "api_key.update", "api_key", strconv.Itoa(updated.ID), apiKeyAuditSnapshot(before), apiKeyAuditSnapshot(updated)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ApiKeyResponse{
		Data:      toAPIKey(updated),
		RequestId: requestID,
	})
}
