package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apikeymemory "github.com/srapi/srapi/apps/api/internal/modules/api_keys/store/memory"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	authmemory "github.com/srapi/srapi/apps/api/internal/modules/auth/store/memory"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	modelmemory "github.com/srapi/srapi/apps/api/internal/modules/models/store/memory"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	usermemory "github.com/srapi/srapi/apps/api/internal/modules/users/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	sessionCookieName       = "srapi_session"
	csrfHeaderName          = "X-CSRF-Token"
	rateLimitCooldownWindow = 30 * time.Second
)

var errRequestTooLarge = errors.New("request body too large")

type runtimeState struct {
	cfg               config.Config
	logger            *slog.Logger
	users             *usersservice.Service
	auth              *authservice.Service
	apiKeys           *apikeyservice.Service
	audit             *auditservice.Service
	billing           *billingservice.Service
	events            *eventsservice.Service
	gateway           *gatewayservice.Service
	providers         *providerservice.Service
	models            *modelservice.Service
	adapters          *provideradapterservice.Service
	reverseProxy      *reverseproxyservice.Service
	accounts          *accountservice.Service
	scheduler         *schedulerservice.Service
	subscriptions     *subscriptionservice.Service
	payments          *paymentservice.Service
	usage             *usageservice.Service
	userStore         userscontract.Store
	sessionStore      *authmemory.Store
	apiKeyStore       apikeycontract.Store
	auditStore        auditcontract.Store
	billingStore      billingcontract.Store
	eventsStore       eventscontract.Store
	providerStore     providercontract.Store
	modelStore        modelcontract.Store
	accountStore      accountcontract.Store
	paymentStore      paymentcontract.Store
	schedulerStore    schedulercontract.Store
	subscriptionStore subscriptioncontract.Store
	usageStore        usagecontract.Store
	capabilities      []capabilitiescontract.Definition
	databaseProbe     dependencyPinger
	redisProbe        dependencyPinger
}

type dependencyHealth struct {
	Database apiopenapi.HealthDependencyStatus
	Redis    apiopenapi.HealthDependencyStatus
}

func newRuntimeState(cfg config.Config, logger *slog.Logger, opts runtimeOptions) (*runtimeState, error) {
	userStore := opts.users
	if userStore == nil {
		userStore = usermemory.New()
	}
	usersSvc, err := usersservice.New(userStore, nil)
	if err != nil {
		return nil, err
	}

	sessionStore := authmemory.New()
	authSvc, err := authservice.New(usersSvc, sessionStore, 0, nil)
	if err != nil {
		return nil, err
	}

	apiKeyStore := opts.apiKeys
	if apiKeyStore == nil {
		apiKeyStore = apikeymemory.New()
	}
	apiKeysSvc, err := apikeyservice.New(apiKeyStore, cfg.Security.APIKeyPepper, nil)
	if err != nil {
		return nil, err
	}

	auditStore := opts.audit
	if auditStore == nil {
		auditStore = auditmemory.New()
	}
	auditSvc, err := auditservice.New(auditStore, nil)
	if err != nil {
		return nil, err
	}

	billingStore := opts.billing
	if billingStore == nil {
		billingStore = billingmemory.New()
	}
	billingSvc, err := billingservice.New(billingStore, nil)
	if err != nil {
		return nil, err
	}

	eventsStore := opts.events
	if eventsStore == nil {
		eventsStore = eventsmemory.New()
	}
	eventsSvc, err := eventsservice.New(eventsStore, nil)
	if err != nil {
		return nil, err
	}

	gatewaySvc, err := gatewayservice.New()
	if err != nil {
		return nil, err
	}

	providerStore := opts.providers
	if providerStore == nil {
		providerStore = providermemory.New()
	}
	providersSvc, err := providerservice.New(providerStore, nil)
	if err != nil {
		return nil, err
	}

	modelStore := opts.models
	if modelStore == nil {
		modelStore = modelmemory.New()
	}
	modelsSvc, err := modelservice.New(modelStore, nil)
	if err != nil {
		return nil, err
	}

	reverseProxySvc, err := reverseproxyservice.New(nil)
	if err != nil {
		return nil, err
	}

	adaptersSvc, err := provideradapterservice.NewWithReverseProxy(&http.Client{Timeout: cfg.Gateway.RequestTimeout}, reverseProxySvc)
	if err != nil {
		return nil, err
	}

	accountStore := opts.accounts
	if accountStore == nil {
		accountStore = accountmemory.New()
	}
	accountsSvc, err := accountservice.New(accountStore, cfg.Security.MasterKey, nil)
	if err != nil {
		return nil, err
	}

	schedulerStore := opts.scheduler
	if schedulerStore == nil {
		schedulerStore = schedulermemory.New()
	}
	schedulerSvc, err := schedulerservice.New(schedulerStore, nil)
	if err != nil {
		return nil, err
	}

	subscriptionStore := opts.subscriptions
	if subscriptionStore == nil {
		subscriptionStore = subscriptionmemory.New()
	}
	subscriptionSvc, err := subscriptionservice.New(subscriptionStore, nil)
	if err != nil {
		return nil, err
	}

	paymentStore := opts.payments
	if paymentStore == nil {
		paymentStore = paymentmemory.New()
	}
	paymentsSvc, err := paymentservice.New(paymentStore, cfg.Security.MasterKey, paymentservice.Dependencies{
		Billing:       billingSvc,
		Subscriptions: subscriptionSvc,
		Audit:         auditSvc,
		Events:        eventsSvc,
	}, nil)
	if err != nil {
		return nil, err
	}

	usageStore := opts.usage
	if usageStore == nil {
		usageStore = usagememory.New()
	}
	usageSvc, err := usageservice.New(usageStore, nil)
	if err != nil {
		return nil, err
	}

	rt := &runtimeState{
		cfg:               cfg,
		logger:            logger,
		users:             usersSvc,
		auth:              authSvc,
		apiKeys:           apiKeysSvc,
		audit:             auditSvc,
		billing:           billingSvc,
		events:            eventsSvc,
		gateway:           gatewaySvc,
		providers:         providersSvc,
		models:            modelsSvc,
		adapters:          adaptersSvc,
		reverseProxy:      reverseProxySvc,
		accounts:          accountsSvc,
		scheduler:         schedulerSvc,
		subscriptions:     subscriptionSvc,
		payments:          paymentsSvc,
		usage:             usageSvc,
		userStore:         userStore,
		sessionStore:      sessionStore,
		apiKeyStore:       apiKeyStore,
		auditStore:        auditStore,
		billingStore:      billingStore,
		eventsStore:       eventsStore,
		providerStore:     providerStore,
		modelStore:        modelStore,
		accountStore:      accountStore,
		paymentStore:      paymentStore,
		schedulerStore:    schedulerStore,
		subscriptionStore: subscriptionStore,
		usageStore:        usageStore,
		capabilities:      seedCapabilities(),
		databaseProbe:     opts.database,
		redisProbe:        opts.redis,
	}
	if err := rt.bootstrapAdmin(context.Background()); err != nil {
		return nil, err
	}
	if err := rt.bootstrapGatewayCatalog(context.Background()); err != nil {
		return nil, err
	}
	return rt, nil
}

func (rt *runtimeState) bootstrapAdmin(ctx context.Context) error {
	email := strings.TrimSpace(rt.cfg.Bootstrap.AdminEmail)
	password := rt.cfg.Bootstrap.AdminPassword
	name := strings.TrimSpace(rt.cfg.Bootstrap.AdminName)
	if email == "" || password == "" {
		return nil
	}
	if _, err := rt.userStore.FindByEmail(ctx, email); err == nil {
		return nil
	}
	_, err := rt.users.Create(ctx, usersservice.CreateRequest{
		Email:    email,
		Name:     name,
		Password: password,
		Roles:    []userscontract.Role{userscontract.RoleAdmin},
	})
	return err
}

func (rt *runtimeState) bootstrapGatewayCatalog(ctx context.Context) error {
	if _, err := rt.providerStore.FindByName(ctx, "openai-compatible"); err != nil {
		if _, err := rt.providers.Create(ctx, providercontract.CreateRequest{
			Name:        "openai-compatible",
			DisplayName: "OpenAI Compatible",
			AdapterType: "openai-compatible",
			Protocol:    "openai-compatible",
			Status:      ptrProviderStatus(providercontract.StatusActive),
		}); err != nil {
			return err
		}
	}

	model, err := rt.modelStore.FindByCanonicalName(ctx, "gpt-4o-mini")
	if err != nil {
		if _, err := rt.models.Create(ctx, modelcontract.CreateRequest{
			CanonicalName:   "gpt-4o-mini",
			DisplayName:     "GPT-4o mini",
			Family:          ptrString("gpt-4o"),
			ContextWindow:   ptrInt(128000),
			MaxOutputTokens: ptrInt(16384),
			QualityTier:     ptrString("standard"),
			Status:          ptrModelStatus(modelcontract.StatusActive),
			Capabilities: []capabilitiescontract.Descriptor{
				{Key: capabilitiescontract.KeyStreaming, Level: capabilitiescontract.DescriptorLevelRequired, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
				{Key: capabilitiescontract.KeyToolCalling, Level: capabilitiescontract.DescriptorLevelOptional, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
				{Key: capabilitiescontract.KeyJSONMode, Level: capabilitiescontract.DescriptorLevelOptional, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
				{Key: capabilitiescontract.KeyStructuredOutput, Level: capabilitiescontract.DescriptorLevelOptional, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
			},
		}); err != nil {
			return err
		}
		model, err = rt.modelStore.FindByCanonicalName(ctx, "gpt-4o-mini")
		if err != nil {
			return err
		}
	}

	provider, err := rt.providerStore.FindByName(ctx, "openai-compatible")
	if err != nil {
		return err
	}
	if _, err := rt.modelStore.FindMapping(ctx, model.ID, provider.ID, "gpt-4o-mini"); err != nil {
		if _, err := rt.models.CreateMapping(ctx, model.ID, modelcontract.CreateMappingRequest{
			ProviderID:        provider.ID,
			UpstreamModelName: "gpt-4o-mini",
			Status:            ptrModelStatus(modelcontract.StatusActive),
		}); err != nil {
			return err
		}
	}

	accounts, err := rt.accountStore.List(ctx)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if strings.EqualFold(account.Name, "openai-compatible-seed") {
			return nil
		}
	}
	if _, err := rt.accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   provider.ID,
		Name:         "openai-compatible-seed",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "seed-openai-compatible"},
		Status:       ptrAccountStatus(accountcontract.StatusActive),
		Priority:     ptrInt(100),
		Weight:       ptrFloat32(1.0),
	}); err != nil {
		return err
	}
	return nil
}

func (rt *runtimeState) healthResponse(ctx context.Context, requestID string) apiopenapi.HealthResponse {
	deps := rt.dependencyHealth(ctx)
	status := apiopenapi.HealthDataStatusOk
	if deps.Database != apiopenapi.HealthDependencyStatusOk || deps.Redis != apiopenapi.HealthDependencyStatusOk {
		status = apiopenapi.HealthDataStatusDegraded
	}

	data := apiopenapi.HealthData{
		Status:  status,
		Version: rt.cfg.Server.Version,
	}
	data.Dependencies.Database = deps.Database
	data.Dependencies.Redis = deps.Redis

	return apiopenapi.HealthResponse{
		Data:      data,
		RequestId: requestID,
	}
}

func (rt *runtimeState) dependencyHealth(ctx context.Context) dependencyHealth {
	return dependencyHealth{
		Database: dependencyStatus(probeStatus(ctx, rt.databaseProbe, rt.cfg.Database.Address())),
		Redis:    dependencyStatus(probeStatus(ctx, rt.redisProbe, rt.cfg.Redis.Address())),
	}
}

func probeStatus(ctx context.Context, probe dependencyPinger, fallbackAddress string) string {
	if probe != nil {
		if err := probe.Ping(ctx); err != nil {
			return "unavailable"
		}
		return "ok"
	}
	return tcpStatus(ctx, fallbackAddress)
}

func dependencyStatus(status string) apiopenapi.HealthDependencyStatus {
	switch status {
	case "ok":
		return apiopenapi.HealthDependencyStatusOk
	case "degraded":
		return apiopenapi.HealthDependencyStatusDegraded
	case "not_configured":
		return apiopenapi.HealthDependencyStatusNotConfigured
	default:
		return apiopenapi.HealthDependencyStatusUnavailable
	}
}

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
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid payment webhook request", requestID)
		return
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
		UserID:        session.User.ID,
		Name:          body.Name,
		Scopes:        derefStrings(body.Scopes),
		AllowedModels: derefStrings(body.AllowedModels),
		GroupIDs:      groupIDs,
		RPMLimit:      body.RpmLimit,
		TPMLimit:      body.TpmLimit,
		ExpiresAt:     body.ExpiresAt,
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

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	decisions, err := s.runtime.scheduler.ListDecisions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduler decisions", requestID)
		return
	}
	activeAccounts := 0
	for _, account := range accounts {
		if account.Status == accountcontract.StatusActive {
			activeAccounts++
		}
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminOverviewResponse{
		Data: apiopenapi.AdminOverview{
			AccountCount:           len(accounts),
			ActiveAccountCount:     activeAccounts,
			ModelCount:             len(models),
			ProviderCount:          len(providers),
			RequestSuccessRate:     usageSuccessRate(usageLogs),
			SchedulerDecisionCount: len(decisions),
			UsageLogCount:          len(usageLogs),
		},
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminProviders(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	providers = filterProviders(providers, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	data := make([]apiopenapi.Provider, 0, len(providers))
	for _, provider := range providers {
		data = append(data, toAPIProvider(provider))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminProvider(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateProviderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid provider request", requestID)
		return
	}
	provider, err := s.runtime.providers.Create(r.Context(), providercontract.CreateRequest{
		Name:         body.Name,
		DisplayName:  body.DisplayName,
		AdapterType:  string(body.AdapterType),
		Protocol:     string(body.Protocol),
		Status:       toProviderStatusPtr(body.Status),
		Capabilities: jsonObjectToMap(body.Capabilities),
		ConfigSchema: jsonObjectToMap(body.ConfigSchema),
	})
	if err != nil {
		switch {
		case errors.Is(err, providerservice.ErrProviderExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "provider already exists", requestID)
		case errors.Is(err, providerservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create provider", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.create", "provider", strconv.Itoa(provider.ID), nil, map[string]any{
		"name":         provider.Name,
		"display_name": provider.DisplayName,
		"adapter_type": provider.AdapterType,
		"protocol":     provider.Protocol,
		"status":       provider.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ProviderResponse{
		Data:      toAPIProvider(provider),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminProvider(w http.ResponseWriter, r *http.Request) {
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
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	before, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider not found", requestID)
		return
	}
	var body apiopenapi.UpdateProviderRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid provider update request", requestID)
		return
	}
	provider, err := s.runtime.providers.Update(r.Context(), providerID, providercontract.UpdateRequest{
		DisplayName:  body.DisplayName,
		AdapterType:  providerAdapterTypeString(body.AdapterType),
		Protocol:     providerProtocolString(body.Protocol),
		Status:       toProviderStatusPtr(body.Status),
		Capabilities: jsonObjectToMapPtr(body.Capabilities),
		ConfigSchema: jsonObjectToMapPtr(body.ConfigSchema),
	})
	if err != nil {
		switch {
		case errors.Is(err, providerservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update provider", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.update", "provider", strconv.Itoa(provider.ID), providerAuditSnapshot(before), providerAuditSnapshot(provider)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderResponse{
		Data:      toAPIProvider(provider),
		RequestId: requestID,
	})
}

func (s *Server) handleTestAdminProvider(w http.ResponseWriter, r *http.Request) {
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
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider not found", requestID)
		return
	}
	startedAt := time.Now()
	result := s.runtime.testProvider(r.Context(), provider, startedAt)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider.test", "provider", strconv.Itoa(provider.ID), nil, map[string]any{
		"ok":     result.Ok,
		"status": result.Status,
		"checks": result.Checks,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminTestResultResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	models = filterModels(models, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	data := make([]apiopenapi.Model, 0, len(models))
	for _, model := range models {
		data = append(data, toAPIModel(model))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminModel(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateModelRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model request", requestID)
		return
	}
	model, err := s.runtime.models.Create(r.Context(), modelcontract.CreateRequest{
		CanonicalName:   body.CanonicalName,
		DisplayName:     body.DisplayName,
		Family:          body.Family,
		ContextWindow:   body.ContextWindow,
		MaxOutputTokens: body.MaxOutputTokens,
		QualityTier:     body.QualityTier,
		Status:          toModelStatusPtr(body.Status),
		Capabilities:    toCapabilityDescriptors(body.Capabilities),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model already exists", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create model", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.create", "model", strconv.Itoa(model.ID), nil, map[string]any{
		"canonical_name": model.CanonicalName,
		"display_name":   model.DisplayName,
		"status":         model.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ModelResponse{
		Data:      toAPIModel(model),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminModel(w http.ResponseWriter, r *http.Request) {
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
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	before, err := s.runtime.models.FindByID(r.Context(), modelID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		return
	}
	var body apiopenapi.UpdateModelRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model update request", requestID)
		return
	}
	model, err := s.runtime.models.Update(r.Context(), modelID, modelcontract.UpdateRequest{
		DisplayName:     body.DisplayName,
		Family:          optionalNullableString(body.Family),
		ContextWindow:   optionalNullableInt(body.ContextWindow),
		MaxOutputTokens: optionalNullableInt(body.MaxOutputTokens),
		QualityTier:     optionalNullableString(body.QualityTier),
		Status:          toModelStatusPtr(body.Status),
		Capabilities:    toCapabilityDescriptorsPtrContract(body.Capabilities),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update model", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.update", "model", strconv.Itoa(model.ID), modelAuditSnapshot(before), modelAuditSnapshot(model)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ModelResponse{
		Data:      toAPIModel(model),
		RequestId: requestID,
	})
}

func (s *Server) handleCreateAdminModelAlias(w http.ResponseWriter, r *http.Request) {
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
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	var body apiopenapi.CreateModelAliasRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model alias request", requestID)
		return
	}
	alias, err := s.runtime.models.CreateAlias(r.Context(), modelID, modelcontract.CreateAliasRequest{
		Alias:          body.Alias,
		StrategyHint:   body.StrategyHint,
		FallbackModels: derefStrings(body.FallbackModels),
		Status:         toModelStatusPtr(body.Status),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrAliasExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model alias already exists", requestID)
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model alias request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create model alias", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_alias.create", "model_alias", strconv.Itoa(alias.ID), nil, map[string]any{
		"alias":           alias.Alias,
		"model_id":        alias.ModelID,
		"fallback_models": alias.FallbackModels,
		"status":          alias.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ModelAliasResponse{
		Data:      toAPIModelAlias(alias),
		RequestId: requestID,
	})
}

func (s *Server) handleCreateAdminModelMapping(w http.ResponseWriter, r *http.Request) {
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
	modelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model id", requestID)
		return
	}
	var body apiopenapi.CreateModelProviderMappingRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid model mapping request", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	if _, err := s.runtime.providers.FindByID(r.Context(), providerID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	mapping, err := s.runtime.models.CreateMapping(r.Context(), modelID, modelcontract.CreateMappingRequest{
		ProviderID:         providerID,
		UpstreamModelName:  body.UpstreamModelName,
		Status:             toModelStatusPtr(body.Status),
		CapabilityOverride: toCapabilityDescriptors(body.CapabilityOverride),
		PricingOverride:    jsonObjectToMap(body.PricingOverride),
	})
	if err != nil {
		switch {
		case errors.Is(err, modelservice.ErrMappingExists):
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "model provider mapping already exists", requestID)
		case errors.Is(err, modelservice.ErrModelNotFound):
			writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "model not found", requestID)
		case errors.Is(err, modelservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid model mapping request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create model mapping", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model_provider_mapping.create", "model_provider_mapping", strconv.Itoa(mapping.ID), nil, map[string]any{
		"model_id":            mapping.ModelID,
		"provider_id":         mapping.ProviderID,
		"upstream_model_name": mapping.UpstreamModelName,
		"status":              mapping.Status,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ModelProviderMappingResponse{
		Data:      toAPIModelProviderMapping(mapping),
		RequestId: requestID,
	})
}

func (s *Server) handleListAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	accounts = filterAccounts(accounts, r.URL.Query().Get("status"), r.URL.Query().Get("provider_id"))
	data := make([]apiopenapi.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		data = append(data, s.apiAccount(r.Context(), account))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleGetAdminAccount(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleCreateAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateProviderAccountRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account request", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	if _, err := s.runtime.providers.FindByID(r.Context(), providerID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	account, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
		ProviderID:     providerID,
		Name:           body.Name,
		RuntimeClass:   accountcontract.RuntimeClass(body.RuntimeClass),
		Credential:     derefMap(body.Credential),
		Metadata:       jsonObjectToMap(body.Metadata),
		ProxyID:        body.ProxyId,
		Status:         toAccountStatusPtr(body.Status),
		Priority:       body.Priority,
		Weight:         body.Weight,
		UpstreamClient: body.UpstreamClient,
	})
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create account", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.create", "provider_account", strconv.Itoa(account.ID), nil, map[string]any{
		"provider_id":   account.ProviderID,
		"name":          account.Name,
		"runtime_class": account.RuntimeClass,
		"status":        account.Status,
		"priority":      account.Priority,
		"weight":        account.Weight,
	}))
	writeJSONAny(w, http.StatusCreated, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleExportAdminAccounts(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to export accounts", requestID)
		return
	}
	data := make([]apiopenapi.ProviderAccountExportItem, 0, len(accounts))
	for _, account := range accounts {
		groupIDs, _ := s.runtime.accounts.ListGroupIDsByAccount(r.Context(), account.ID)
		data = append(data, apiopenapi.ProviderAccountExportItem{
			CredentialExported: false,
			GroupIds:           apiIDsPtr(groupIDs),
			Metadata:           mapToJsonObjectPtr(sanitizedExportMetadata(account.Metadata)),
			Name:               account.Name,
			Priority:           account.Priority,
			ProviderId:         apiopenapi.Id(strconv.Itoa(account.ProviderID)),
			ProxyId:            account.ProxyID,
			RuntimeClass:       apiopenapi.RuntimeClass(account.RuntimeClass),
			Status:             apiopenapi.ProviderAccountStatus(account.Status),
			UpstreamClient:     account.UpstreamClient,
			Weight:             account.Weight,
		})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountExportResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) handleImportAdminAccounts(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.ProviderAccountImportRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account import request", requestID)
		return
	}
	createdIDs := make([]apiopenapi.Id, 0)
	importErrors := make([]string, 0)
	skipped := 0
	for idx, item := range body.Accounts {
		providerID, err := strconv.Atoi(string(item.ProviderId))
		if err != nil || providerID <= 0 {
			skipped++
			importErrors = append(importErrors, fmt.Sprintf("accounts[%d].provider_id invalid", idx))
			continue
		}
		if _, err := s.runtime.providers.FindByID(r.Context(), providerID); err != nil {
			skipped++
			importErrors = append(importErrors, fmt.Sprintf("accounts[%d].provider_id not found", idx))
			continue
		}
		credential := derefMap(item.Credential)
		if len(credential) == 0 {
			skipped++
			importErrors = append(importErrors, fmt.Sprintf("accounts[%d].credential required", idx))
			continue
		}
		account, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
			ProviderID:     providerID,
			Name:           item.Name,
			RuntimeClass:   accountcontract.RuntimeClass(item.RuntimeClass),
			Credential:     credential,
			Metadata:       jsonObjectToMap(item.Metadata),
			ProxyID:        item.ProxyId,
			Status:         toAccountStatusPtr(item.Status),
			Priority:       item.Priority,
			Weight:         item.Weight,
			UpstreamClient: item.UpstreamClient,
		})
		if err != nil {
			skipped++
			importErrors = append(importErrors, fmt.Sprintf("accounts[%d] create failed", idx))
			continue
		}
		createdIDs = append(createdIDs, apiopenapi.Id(strconv.Itoa(account.ID)))
		groupIDs, err := apiIDsToInts(item.GroupIds)
		if err != nil {
			importErrors = append(importErrors, fmt.Sprintf("accounts[%d].group_ids invalid", idx))
			continue
		}
		for _, groupID := range groupIDs {
			if _, err := s.runtime.accounts.AddAccountToGroup(r.Context(), account.ID, groupID); err != nil {
				importErrors = append(importErrors, fmt.Sprintf("accounts[%d].group_ids[%d] add failed", idx, groupID))
			}
		}
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.import", "provider_account", "bulk", nil, map[string]any{
		"created_count": len(createdIDs),
		"skipped_count": skipped,
		"error_count":   len(importErrors),
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountImportResponse{
		Data: apiopenapi.ProviderAccountImportResult{
			CreatedCount: len(createdIDs),
			CreatedIds:   createdIDs,
			Errors:       importErrors,
			SkippedCount: skipped,
		},
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	var body apiopenapi.UpdateProviderAccountRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account update request", requestID)
		return
	}
	account, err := s.runtime.accounts.Update(r.Context(), accountID, accountcontract.UpdateRequest{
		Name:           body.Name,
		RuntimeClass:   toAccountRuntimeClassPtr(body.RuntimeClass),
		Credential:     optionalCredential(body.Credential),
		Metadata:       jsonObjectToMapPtr(body.Metadata),
		ProxyID:        optionalNullableString(body.ProxyId),
		Status:         toAccountStatusPtr(body.Status),
		Priority:       body.Priority,
		Weight:         body.Weight,
		UpstreamClient: optionalNullableString(body.UpstreamClient),
	})
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account update request", requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update account", requestID)
		}
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.update", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleBindAdminAccountProxy(w http.ResponseWriter, r *http.Request) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	var body apiopenapi.BindProviderAccountProxyRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account proxy request", requestID)
		return
	}
	account, err := s.runtime.accounts.BindProxy(r.Context(), accountID, body.ProxyId)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account proxy request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.proxy_bind", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleDisableAdminAccount(w http.ResponseWriter, r *http.Request) {
	s.handleSetAdminAccountStatus(w, r, accountcontract.StatusDisabled, "provider_account.disable")
}

func (s *Server) handleEnableAdminAccount(w http.ResponseWriter, r *http.Request) {
	s.handleSetAdminAccountStatus(w, r, accountcontract.StatusActive, "provider_account.enable")
}

func (s *Server) handleRecoverAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	account, err := s.runtime.accounts.Recover(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to recover account", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.recover", "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleTestAdminAccount(w http.ResponseWriter, r *http.Request) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), account.ProviderID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}
	startedAt := time.Now()
	result := s.runtime.testAccount(r.Context(), provider, account, startedAt)
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.test", "provider_account", strconv.Itoa(account.ID), nil, map[string]any{
		"ok":     result.Ok,
		"status": result.Status,
		"checks": result.Checks,
	}))
	s.runtime.recordAccountTestHealthSnapshot(r.Context(), account, result)
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminTestResultResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) handleSetAdminAccountStatus(w http.ResponseWriter, r *http.Request, status accountcontract.Status, action string) {
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
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	account, err := s.runtime.accounts.Update(r.Context(), accountID, accountcontract.UpdateRequest{Status: &status})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to update account status", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, action, "provider_account", strconv.Itoa(account.ID), accountAuditSnapshot(before), accountAuditSnapshot(account)))
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderAccountResponse{
		Data:      s.apiAccount(r.Context(), account),
		RequestId: requestID,
	})
}

func (s *Server) handleAdminAccountHealth(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	snapshot := buildAccountHealthSnapshot(account, usageLogsForAccount(usageLogs, account.ID), time.Now().UTC())
	if latest, err := s.runtime.accounts.LatestHealthSnapshotByAccount(r.Context(), account.ID); err == nil {
		overlayAccountHealthSnapshot(&snapshot, latest)
	}
	if quotas, err := s.runtime.accounts.ListQuotaSnapshotsByAccount(r.Context(), account.ID, 1); err == nil && len(quotas) > 0 {
		overlayAccountQuotaOnHealth(&snapshot, quotas[0])
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountHealthResponse{
		Data:      snapshot,
		RequestId: requestID,
	})
}

func (s *Server) handleAdminAccountQuota(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}
	account, err := s.runtime.accounts.FindByID(r.Context(), accountID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account not found", requestID)
		return
	}
	usageLogs, err := s.runtime.usage.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list usage logs", requestID)
		return
	}
	snapshots, err := s.runtime.accounts.ListQuotaSnapshotsByAccount(r.Context(), account.ID, 50)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account quota snapshots", requestID)
		return
	}
	data := make([]apiopenapi.AccountQuotaSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		data = append(data, toAPIAccountQuotaSnapshot(snapshot))
	}
	if len(data) == 0 {
		data = append(data, buildAccountQuotaSnapshot(account, usageLogsForAccount(usageLogs, account.ID), time.Now().UTC()))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountQuotaListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleListAdminAccountGroups(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	groups, err := s.runtime.accounts.ListGroups(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list account groups", requestID)
		return
	}
	data := make([]apiopenapi.AccountGroup, 0, len(groups))
	for _, group := range groups {
		data = append(data, toAPIAccountGroup(group))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupListResponse{
		Data:       data,
		Pagination: pagination(len(data)),
		RequestId:  requestID,
	})
}

func (s *Server) handleCreateAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CreateAccountGroupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account group request", requestID)
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	group, err := s.runtime.accounts.CreateGroup(r.Context(), accountcontract.CreateGroupRequest{
		Name:          body.Name,
		Description:   description,
		ProviderScope: jsonObjectToMap(body.ProviderScope),
		ModelScope:    jsonObjectToMap(body.ModelScope),
		StrategyHint:  body.StrategyHint,
		Status:        toAccountGroupStatusPtr(body.Status),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.create", "account_group", strconv.Itoa(group.ID), nil, accountGroupAuditSnapshot(group)))
	writeJSONAny(w, http.StatusCreated, apiopenapi.AccountGroupResponse{
		Data:      toAPIAccountGroup(group),
		RequestId: requestID,
	})
}

func (s *Server) handleUpdateAdminAccountGroup(w http.ResponseWriter, r *http.Request) {
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
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return
	}
	before, err := s.runtime.accounts.FindGroupByID(r.Context(), groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account group not found", requestID)
		return
	}
	var body apiopenapi.UpdateAccountGroupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid account group update request", requestID)
		return
	}
	group, err := s.runtime.accounts.UpdateGroup(r.Context(), groupID, accountcontract.UpdateGroupRequest{
		Name:          body.Name,
		Description:   body.Description,
		ProviderScope: jsonObjectToMapPtr(body.ProviderScope),
		ModelScope:    jsonObjectToMapPtr(body.ModelScope),
		StrategyHint:  body.StrategyHint,
		Status:        toAccountGroupStatusPtr(body.Status),
	})
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group update request", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.update", "account_group", strconv.Itoa(group.ID), accountGroupAuditSnapshot(before), accountGroupAuditSnapshot(group)))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupResponse{
		Data:      toAPIAccountGroup(group),
		RequestId: requestID,
	})
}

func (s *Server) handleAddAdminAccountGroupMember(w http.ResponseWriter, r *http.Request) {
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
	groupID, accountID, ok := accountGroupMemberPathIDs(w, r, requestID)
	if !ok {
		return
	}
	member, err := s.runtime.accounts.AddAccountToGroup(r.Context(), accountID, groupID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "account or group not found", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.member_add", "account_group", strconv.Itoa(groupID), nil, map[string]any{
		"account_id": accountID,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.AccountGroupMemberResponse{
		Data:      toAPIAccountGroupMember(member),
		RequestId: requestID,
	})
}

func (s *Server) handleRemoveAdminAccountGroupMember(w http.ResponseWriter, r *http.Request) {
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
	groupID, accountID, ok := accountGroupMemberPathIDs(w, r, requestID)
	if !ok {
		return
	}
	if err := s.runtime.accounts.RemoveAccountFromGroup(r.Context(), accountID, groupID); err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to remove account group membership", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "account_group.member_remove", "account_group", strconv.Itoa(groupID), map[string]any{
		"account_id": accountID,
	}, nil))
	w.WriteHeader(http.StatusNoContent)
}

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

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		switch {
		case errors.Is(err, apikeyservice.ErrInvalidKey), errors.Is(err, apikeyservice.ErrInvalidInput):
			writeGatewayError(w, http.StatusUnauthorized, apiopenapi.AuthenticationError, "invalid API key", "invalid_api_key")
		case errors.Is(err, apikeyservice.ErrKeyDisabled), errors.Is(err, apikeyservice.ErrKeyExpired):
			writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "API key disabled or expired", "api_key_disabled")
		default:
			writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to authenticate API key", "internal_error")
		}
		return
	}

	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to list models", "internal_error")
		return
	}
	gatewayModels := toGatewayModels(models)
	gatewayModels = filterGatewayModels(gatewayModels, authed.Key.AllowedModels)
	writeJSONAny(w, http.StatusOK, apiopenapi.OpenAIModelList{
		Object: apiopenapi.List,
		Data:   gatewayModels,
	})
	_ = requestID
}

func (s *Server) handleCreateChatCompletion(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/chat/completions")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.ChatCompletionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid chat completion request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(body.Model),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeChatCompletions(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, forcedProviderKey, authed.Key)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			DecisionID:            result.Decision.ID,
			AttemptNo:             result.Decision.AttemptNo,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("no_available_account"),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	providerResp, err := s.runtime.invokeProviderText(r.Context(), providerTextRequest(canonical, result.Candidate))
	if err != nil {
		errorClass, upstreamStatus, errorType := providerGatewayError(err)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			DecisionID:            result.Decision.ID,
			AttemptNo:             result.Decision.AttemptNo,
			ProviderID:            ptrInt(result.Candidate.Provider.ID),
			AccountID:             ptrInt(result.Candidate.Account.ID),
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			TargetProtocol:        result.Candidate.Provider.Protocol,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			StatusCode:            ptrInt(upstreamStatus),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalTextResponse(canonical, providerResp.Text, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequest(model.ID, result.Candidate, canonicalResp.Usage), canonicalResp.Usage.Estimated)
	s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		ProviderID:            ptrInt(result.Candidate.Provider.ID),
		AccountID:             ptrInt(result.Candidate.Account.ID),
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		TargetProtocol:        result.Candidate.Provider.Protocol,
		Model:                 canonical.CanonicalModel,
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
	})
	if canonical.Stream {
		writeSSEJSON(w, s.runtime.gateway.RenderChatStreamChunk(canonicalResp))
		return
	}
	writeJSONAny(w, http.StatusOK, s.runtime.gateway.RenderChatCompletions(canonicalResp))
}

func (s *Server) handleCreateResponse(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/responses")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.ResponsesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid responses request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(body.Model),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "openai-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeResponses(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, forcedProviderKey, authed.Key)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			DecisionID:            result.Decision.ID,
			AttemptNo:             result.Decision.AttemptNo,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("no_available_account"),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	providerResp, err := s.runtime.invokeProviderText(r.Context(), providerTextRequest(canonical, result.Candidate))
	if err != nil {
		errorClass, upstreamStatus, errorType := providerGatewayError(err)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			DecisionID:            result.Decision.ID,
			AttemptNo:             result.Decision.AttemptNo,
			ProviderID:            ptrInt(result.Candidate.Provider.ID),
			AccountID:             ptrInt(result.Candidate.Account.ID),
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			TargetProtocol:        result.Candidate.Provider.Protocol,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			StatusCode:            ptrInt(upstreamStatus),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalTextResponse(canonical, providerResp.Text, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequest(model.ID, result.Candidate, canonicalResp.Usage), canonicalResp.Usage.Estimated)
	s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		ProviderID:            ptrInt(result.Candidate.Provider.ID),
		AccountID:             ptrInt(result.Candidate.Account.ID),
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		TargetProtocol:        result.Candidate.Provider.Protocol,
		Model:                 canonical.CanonicalModel,
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
	})
	response := s.runtime.gateway.RenderResponses(canonicalResp)
	if canonical.Stream {
		writeSSEEvents(w, s.runtime.gateway.RenderResponsesStreamEvents(canonicalResp))
		return
	}
	writeJSONAny(w, http.StatusOK, response)
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	sourceEndpoint := gatewaySourceEndpoint(r.Context(), "/v1/messages")
	forcedProviderKey := gatewayForcedProviderKey(r.Context())
	startedAt := time.Now()
	authed, err := s.requireGatewayKey(r)
	if err != nil {
		writeGatewayAuthError(w, err, requestID)
		return
	}
	var body apiopenapi.AnthropicMessagesRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "anthropic-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          "unknown",
			Success:        false,
			ErrorClass:     ptrStringValue("invalid_request"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, jsonDecodeStatus(err), apiopenapi.InvalidRequestError, "invalid messages request", "invalid_request")
		return
	}
	modelResolution, err := s.runtime.models.ResolveModelReference(r.Context(), body.Model)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "anthropic-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          fallbackModelName(body.Model),
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_found"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusNotFound, apiopenapi.InvalidRequestError, "model not found", "model_not_found")
		return
	}
	model := modelResolution.Model
	if !apiKeyAllowsModelReference(authed.Key.AllowedModels, modelResolution) {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:      requestID,
			Authed:         authed,
			SourceProtocol: "anthropic-compatible",
			SourceEndpoint: sourceEndpoint,
			Model:          model.CanonicalName,
			Success:        false,
			ErrorClass:     ptrStringValue("model_not_allowed"),
			LatencyMS:      elapsedMillis(startedAt),
			UsageEstimated: true,
		})
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "model not allowed for this api key", "model_not_allowed")
		return
	}
	canonical := s.runtime.gateway.NormalizeAnthropicMessages(body, gatewayservice.RequestMeta{
		RequestID:      requestID,
		SourceEndpoint: sourceEndpoint,
		UserID:         authed.UserID,
		APIKeyID:       authed.Key.ID,
		CanonicalModel: model.CanonicalName,
	})
	admission, err := s.runtime.prepareGatewayAdmission(r.Context(), canonical, modelResolution, model.ID)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("entitlement_check_failed"),
			LatencyMS:             elapsedMillis(startedAt),
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to check gateway entitlement", "entitlement_check_failed")
		return
	}
	if !admission.Entitlement.Allowed {
		errorClass := gatewayEntitlementErrorClass(admission.Entitlement)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, gatewayEntitlementHTTPStatus(errorClass), gatewayEntitlementErrorType(errorClass), gatewayEntitlementMessage(errorClass), errorClass)
		return
	}
	scheduleReq := gatewayScheduleRequest(r, canonical, modelResolution)
	s.runtime.applyGatewayAdmission(&scheduleReq, admission)
	result, err := s.runtime.scheduleGatewayRequest(r.Context(), scheduleReq, model.ID, forcedProviderKey, authed.Key)
	if err != nil {
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			DecisionID:            result.Decision.ID,
			AttemptNo:             result.Decision.AttemptNo,
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue("no_available_account"),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, http.StatusServiceUnavailable, apiopenapi.ServiceUnavailableError, "no available account", "no_available_account")
		return
	}
	providerResp, err := s.runtime.invokeProviderText(r.Context(), providerTextRequest(canonical, result.Candidate))
	if err != nil {
		errorClass, upstreamStatus, errorType := providerGatewayError(err)
		s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
			RequestID:             canonical.RequestID,
			Authed:                authed,
			DecisionID:            result.Decision.ID,
			AttemptNo:             result.Decision.AttemptNo,
			ProviderID:            ptrInt(result.Candidate.Provider.ID),
			AccountID:             ptrInt(result.Candidate.Account.ID),
			SourceProtocol:        string(canonical.SourceProtocol),
			SourceEndpoint:        canonical.SourceEndpoint,
			TargetProtocol:        result.Candidate.Provider.Protocol,
			Model:                 canonical.CanonicalModel,
			Success:               false,
			ErrorClass:            ptrStringValue(errorClass),
			StatusCode:            ptrInt(upstreamStatus),
			LatencyMS:             elapsedMillis(startedAt),
			InputTokens:           admission.EstimatedUsage.InputTokens,
			OutputTokens:          admission.EstimatedUsage.OutputTokens,
			CachedTokens:          admission.EstimatedUsage.CachedTokens,
			UsageEstimated:        true,
			Pricing:               admission.Pricing,
			CompatibilityWarnings: canonical.CompatibilityWarnings,
		})
		writeGatewayError(w, providerGatewayHTTPStatus(upstreamStatus), errorType, providerGatewayMessage(errorClass), errorClass)
		return
	}
	usage := gatewayUsageFromProvider(providerResp)
	canonicalResp := s.runtime.gateway.BuildCanonicalTextResponse(canonical, providerResp.Text, usage)
	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequest(model.ID, result.Candidate, canonicalResp.Usage), canonicalResp.Usage.Estimated)
	s.runtime.recordGatewayUsage(r.Context(), gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		ProviderID:            ptrInt(result.Candidate.Provider.ID),
		AccountID:             ptrInt(result.Candidate.Account.ID),
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		TargetProtocol:        result.Candidate.Provider.Protocol,
		Model:                 canonical.CanonicalModel,
		Success:               true,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           canonicalResp.Usage.InputTokens,
		OutputTokens:          canonicalResp.Usage.OutputTokens,
		CachedTokens:          canonicalResp.Usage.CachedTokens,
		UsageEstimated:        canonicalResp.Usage.Estimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonicalResp.CompatibilityWarnings,
	})
	response := s.runtime.gateway.RenderAnthropicMessages(canonicalResp)
	if canonical.Stream {
		writeSSEEvents(w, s.runtime.gateway.RenderAnthropicMessagesStreamEvents(canonicalResp))
		return
	}
	writeJSONAny(w, http.StatusOK, response)
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeGatewayError(w, http.StatusNotImplemented, apiopenapi.InternalError, "endpoint not implemented yet", "not_implemented")
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	lines := s.runtime.metricsLines(r.Context())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

func (rt *runtimeState) metricsLines(ctx context.Context) []string {
	lines := []string{
		"# HELP srapi_gateway_requests_total Gateway requests recorded by endpoint family, model, provider protocol, and result.",
		"# TYPE srapi_gateway_requests_total counter",
		"# HELP srapi_gateway_request_duration_seconds Gateway request latency summary derived from usage logs.",
		"# TYPE srapi_gateway_request_duration_seconds summary",
		"# HELP srapi_gateway_inflight_requests Gateway requests with pending scheduler leases.",
		"# TYPE srapi_gateway_inflight_requests gauge",
		"# HELP srapi_gateway_errors_total Gateway request errors recorded by error class.",
		"# TYPE srapi_gateway_errors_total counter",
		"# HELP srapi_scheduler_decisions_total Scheduler decisions by strategy and outcome.",
		"# TYPE srapi_scheduler_decisions_total counter",
		"# HELP srapi_provider_errors_total Provider-facing errors recorded by protocol and error class.",
		"# TYPE srapi_provider_errors_total counter",
		"# HELP srapi_usage_tokens_total Usage tokens by model, provider protocol, and token kind.",
		"# TYPE srapi_usage_tokens_total counter",
		"# HELP srapi_reverse_proxy_ban_signals_total Reverse proxy ban signals observed by risk class.",
		"# TYPE srapi_reverse_proxy_ban_signals_total counter",
		"# HELP reverse_proxy_request_total Reverse proxy requests.",
		"# TYPE reverse_proxy_request_total counter",
		"# HELP reverse_proxy_request_success_total Reverse proxy successful requests.",
		"# TYPE reverse_proxy_request_success_total counter",
	}

	usageLogs, usageErr := rt.usage.List(ctx)
	if usageErr == nil {
		lines = append(lines, gatewayUsageMetricLines(usageLogs)...)
		lines = append(lines, providerErrorMetricLines(usageLogs)...)
		lines = append(lines, usageTokenMetricLines(usageLogs)...)
	} else {
		rt.logger.Warn("failed to collect usage metrics", "error", usageErr)
	}

	decisions, decisionErr := rt.scheduler.ListDecisions(ctx)
	if decisionErr == nil {
		lines = append(lines, schedulerDecisionMetricLines(decisions)...)
	} else {
		rt.logger.Warn("failed to collect scheduler decision metrics", "error", decisionErr)
	}

	leases, leaseErr := rt.scheduler.ListLeases(ctx)
	if leaseErr == nil {
		lines = append(lines, fmt.Sprintf("srapi_gateway_inflight_requests %d", pendingLeaseCount(leases)))
	} else {
		rt.logger.Warn("failed to collect scheduler lease metrics", "error", leaseErr)
		lines = append(lines, "srapi_gateway_inflight_requests 0")
	}

	metrics := rt.reverseProxy.Metrics()
	lines = append(lines,
		fmt.Sprintf(`srapi_reverse_proxy_ban_signals_total{risk_class="account_locked"} %d`, metrics.AccountLockedTotal),
		fmt.Sprintf(`srapi_reverse_proxy_ban_signals_total{risk_class="account_banned"} %d`, metrics.AccountBannedTotal),
		fmt.Sprintf("reverse_proxy_request_total %d", metrics.RequestTotal),
		fmt.Sprintf("reverse_proxy_request_success_total %d", metrics.RequestSuccessTotal),
	)
	for class, count := range metrics.RequestErrorTotal {
		lines = append(lines, fmt.Sprintf("reverse_proxy_request_error_total{error_class=%q} %d", class, count))
	}
	for class, count := range metrics.ChallengeTotal {
		lines = append(lines, fmt.Sprintf("reverse_proxy_challenge_total{strategy=%q} %d", class, count))
	}
	lines = append(lines,
		fmt.Sprintf("reverse_proxy_account_locked_total %d", metrics.AccountLockedTotal),
		fmt.Sprintf("reverse_proxy_account_banned_total %d", metrics.AccountBannedTotal),
	)
	for status, count := range metrics.OAuthRefreshTotal {
		lines = append(lines, fmt.Sprintf("reverse_proxy_oauth_refresh_total{status=%q} %d", status, count))
	}
	lines = appendZeroValueBaselineMetrics(lines)
	sortMetricLines(lines)
	return lines
}

func gatewayUsageMetricLines(logs []usagecontract.UsageLog) []string {
	type aggregate struct {
		count     int
		latencyMS int
	}
	requests := map[string]*aggregate{}
	errors := map[string]int{}
	for _, log := range logs {
		result := "success"
		if !log.Success {
			result = "error"
		}
		key := strings.Join([]string{
			endpointFamily(log.SourceEndpoint),
			metricLabelValue(log.Model, "unknown"),
			metricLabelValue(log.TargetProtocol, "unknown"),
			result,
		}, "\xff")
		if requests[key] == nil {
			requests[key] = &aggregate{}
		}
		requests[key].count++
		requests[key].latencyMS += log.LatencyMS
		if !log.Success {
			errorClass := metricLabelValue(derefString(log.ErrorClass), "unknown")
			errors[errorClass]++
		}
	}
	keys := sortedKeys(requests)
	lines := make([]string, 0, len(keys)*3+len(errors))
	for _, key := range keys {
		parts := strings.Split(key, "\xff")
		value := requests[key]
		labels := fmt.Sprintf(`endpoint_family=%q,model=%q,provider_protocol=%q,result=%q`, parts[0], parts[1], parts[2], parts[3])
		lines = append(lines,
			fmt.Sprintf("srapi_gateway_requests_total{%s} %d", labels, value.count),
			fmt.Sprintf("srapi_gateway_request_duration_seconds_count{%s} %d", labels, value.count),
			fmt.Sprintf("srapi_gateway_request_duration_seconds_sum{%s} %.6f", labels, float64(value.latencyMS)/1000),
		)
	}
	for _, errorClass := range sortedIntKeys(errors) {
		lines = append(lines, fmt.Sprintf("srapi_gateway_errors_total{error_class=%q} %d", errorClass, errors[errorClass]))
	}
	return lines
}

func schedulerDecisionMetricLines(decisions []schedulercontract.Decision) []string {
	counts := map[string]int{}
	for _, decision := range decisions {
		outcome := "selected"
		reason := "selected"
		if decision.SelectedAccountID == nil {
			outcome = "rejected"
			reason = primaryRejectReason(decision.RejectReasons)
		}
		key := strings.Join([]string{string(decision.Strategy), outcome, reason}, "\xff")
		counts[key]++
	}
	lines := make([]string, 0, len(counts))
	for _, key := range sortedIntKeys(counts) {
		parts := strings.Split(key, "\xff")
		lines = append(lines, fmt.Sprintf("srapi_scheduler_decisions_total{strategy=%q,outcome=%q,reason=%q} %d", parts[0], parts[1], parts[2], counts[key]))
	}
	return lines
}

func providerErrorMetricLines(logs []usagecontract.UsageLog) []string {
	counts := map[string]int{}
	for _, log := range logs {
		if log.Success || log.ProviderID == nil {
			continue
		}
		protocol := metricLabelValue(log.TargetProtocol, "unknown")
		errorClass := metricLabelValue(derefString(log.ErrorClass), "unknown")
		counts[strings.Join([]string{protocol, errorClass}, "\xff")]++
	}
	lines := make([]string, 0, len(counts))
	for _, key := range sortedIntKeys(counts) {
		parts := strings.Split(key, "\xff")
		lines = append(lines, fmt.Sprintf("srapi_provider_errors_total{provider_protocol=%q,error_class=%q} %d", parts[0], parts[1], counts[key]))
	}
	return lines
}

func usageTokenMetricLines(logs []usagecontract.UsageLog) []string {
	counts := map[string]int{}
	for _, log := range logs {
		model := metricLabelValue(log.Model, "unknown")
		protocol := metricLabelValue(log.TargetProtocol, "unknown")
		counts[strings.Join([]string{model, protocol, "input"}, "\xff")] += log.InputTokens
		counts[strings.Join([]string{model, protocol, "output"}, "\xff")] += log.OutputTokens
		counts[strings.Join([]string{model, protocol, "cached"}, "\xff")] += log.CachedTokens
	}
	lines := make([]string, 0, len(counts))
	for _, key := range sortedIntKeys(counts) {
		parts := strings.Split(key, "\xff")
		lines = append(lines, fmt.Sprintf("srapi_usage_tokens_total{model=%q,provider_protocol=%q,token_kind=%q} %d", parts[0], parts[1], parts[2], counts[key]))
	}
	return lines
}

func pendingLeaseCount(leases []schedulercontract.Lease) int {
	count := 0
	for _, lease := range leases {
		if lease.Status == schedulercontract.LeaseStatusPending {
			count++
		}
	}
	return count
}

func endpointFamily(endpoint string) string {
	switch {
	case strings.Contains(endpoint, "/responses"):
		return "responses"
	case strings.Contains(endpoint, "/messages"):
		return "messages"
	case strings.Contains(endpoint, "/chat/completions"):
		return "chat_completions"
	case strings.TrimSpace(endpoint) == "":
		return "unknown"
	default:
		return strings.Trim(strings.ReplaceAll(endpoint, "/", "_"), "_")
	}
}

func primaryRejectReason(reasons map[string]any) string {
	if len(reasons) == 0 {
		return "no_candidate"
	}
	keys := make([]string, 0, len(reasons))
	for key := range reasons {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if reason, ok := reasons[key].(string); ok && strings.TrimSpace(reason) != "" {
			return metricLabelValue(reason, "rejected")
		}
	}
	return "rejected"
}

func metricLabelValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sortedIntKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortMetricLines(lines []string) {
	if len(lines) == 0 {
		return
	}
	headerEnd := 0
	for headerEnd < len(lines) && strings.HasPrefix(lines[headerEnd], "#") {
		headerEnd++
	}
	sort.Strings(lines[headerEnd:])
}

func appendZeroValueBaselineMetrics(lines []string) []string {
	for _, sample := range []string{
		`srapi_gateway_requests_total{endpoint_family="unknown",model="unknown",provider_protocol="unknown",result="success"} 0`,
		`srapi_gateway_request_duration_seconds_count{endpoint_family="unknown",model="unknown",provider_protocol="unknown",result="success"} 0`,
		`srapi_gateway_request_duration_seconds_sum{endpoint_family="unknown",model="unknown",provider_protocol="unknown",result="success"} 0.000000`,
		`srapi_gateway_inflight_requests 0`,
		`srapi_gateway_errors_total{error_class="unknown"} 0`,
		`srapi_scheduler_decisions_total{strategy="unknown",outcome="selected",reason="selected"} 0`,
		`srapi_provider_errors_total{provider_protocol="unknown",error_class="unknown"} 0`,
		`srapi_usage_tokens_total{model="unknown",provider_protocol="unknown",token_kind="input"} 0`,
		`srapi_reverse_proxy_ban_signals_total{risk_class="account_locked"} 0`,
		`srapi_reverse_proxy_ban_signals_total{risk_class="account_banned"} 0`,
	} {
		if !hasMetricName(lines, metricName(sample)) {
			lines = append(lines, sample)
		}
	}
	return lines
}

func hasMetricName(lines []string, name string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"{") {
			return true
		}
	}
	return false
}

func metricName(sample string) string {
	if idx := strings.IndexAny(sample, " {"); idx > 0 {
		return sample[:idx]
	}
	return sample
}

type gatewayUsageRecord struct {
	RequestID             string
	Authed                apikeycontract.AuthResult
	DecisionID            int
	AttemptNo             int
	ProviderID            *int
	AccountID             *int
	SourceProtocol        string
	SourceEndpoint        string
	TargetProtocol        string
	Model                 string
	Success               bool
	ErrorClass            *string
	StatusCode            *int
	LatencyMS             int
	InputTokens           int
	OutputTokens          int
	CachedTokens          int
	UsageEstimated        bool
	Pricing               gatewayPricingEvidence
	CompatibilityWarnings []string
}

type gatewayAdmission struct {
	EstimatedUsage gatewaycontract.Usage
	Pricing        gatewayPricingEvidence
	Entitlement    subscriptioncontract.EntitlementDecision
}

type gatewayPricingEvidence struct {
	Amount           string
	Currency         string
	PricingRuleID    *int
	PricingSource    string
	PricingEstimated bool
}

func (e gatewayPricingEvidence) withDefaults() gatewayPricingEvidence {
	if strings.TrimSpace(e.Amount) == "" {
		e.Amount = "0.00000000"
	}
	if strings.TrimSpace(e.Currency) == "" {
		e.Currency = "USD"
	}
	if strings.TrimSpace(e.PricingSource) == "" {
		e.PricingSource = "default_zero"
	}
	return e
}

func (rt *runtimeState) scheduleGatewayRequest(ctx context.Context, req schedulercontract.ScheduleRequest, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey) (schedulercontract.ScheduleResult, error) {
	candidates, err := rt.gatewayCandidates(ctx, modelID, forcedProviderKey, apiKey)
	if err != nil {
		return schedulercontract.ScheduleResult{}, err
	}
	if len(req.AccountGroupScope) > 0 {
		candidates, err = rt.filterCandidatesByAccountGroupScope(ctx, candidates, req.AccountGroupScope)
		if err != nil {
			return schedulercontract.ScheduleResult{}, err
		}
	}
	if req.StickyAccountID == nil && strings.TrimSpace(req.SessionAffinityKey) != "" {
		req.StickyAccountID = stickyAccountIDFromCandidates(candidates, req.SessionAffinityKey)
	}
	req.Candidates = candidates
	return rt.scheduler.Schedule(ctx, req)
}

func (rt *runtimeState) prepareGatewayAdmission(ctx context.Context, canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution, modelID int) (gatewayAdmission, error) {
	estimatedUsage := estimateGatewayRequestUsage(canonical)
	pricing := rt.gatewayPricing(ctx, subscriptioncontract.PricingRequest{
		ModelID:      modelID,
		ProviderID:   0,
		InputTokens:  estimatedUsage.InputTokens,
		OutputTokens: estimatedUsage.OutputTokens,
		At:           time.Now().UTC(),
	}, true)
	tokensUsed, costUsed, err := rt.gatewayUserPeriodUsage(ctx, canonical.UserID, time.Now().UTC())
	if err != nil {
		return gatewayAdmission{}, err
	}
	entitlement, err := rt.subscriptions.CheckEntitlement(ctx, subscriptioncontract.EntitlementCheckRequest{
		UserID:             canonical.UserID,
		ModelReferences:    gatewayModelReferences(canonical, resolution),
		EstimatedTokens:    estimatedUsage.InputTokens + estimatedUsage.OutputTokens + estimatedUsage.CachedTokens,
		EstimatedCost:      pricing.Amount,
		TokensUsedInPeriod: tokensUsed,
		CostUsedInPeriod:   costUsed,
		RequestTime:        time.Now().UTC(),
	})
	if err != nil {
		return gatewayAdmission{}, err
	}
	return gatewayAdmission{EstimatedUsage: estimatedUsage, Pricing: pricing, Entitlement: entitlement}, nil
}

func (rt *runtimeState) applyGatewayAdmission(req *schedulercontract.ScheduleRequest, admission gatewayAdmission) {
	req.EstimatedInputTokens = admission.EstimatedUsage.InputTokens
	req.EstimatedOutputTokens = admission.EstimatedUsage.OutputTokens
	req.EstimatedCost = admission.Pricing.Amount
	req.Currency = admission.Pricing.Currency
	req.PricingRuleID = admission.Pricing.PricingRuleID
	req.PricingSource = admission.Pricing.PricingSource
	req.PricingEstimated = true
	req.AccountGroupScope = append([]int(nil), admission.Entitlement.AccountGroupScope...)
	if strategy := schedulerStrategyName(admission.Entitlement.SchedulerStrategy); strategy != "" {
		req.Strategy = strategy
	}
}

func (rt *runtimeState) filterCandidatesByAccountGroupScope(ctx context.Context, candidates []schedulercontract.Candidate, scope []int) ([]schedulercontract.Candidate, error) {
	if len(scope) == 0 {
		return candidates, nil
	}
	out := make([]schedulercontract.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		groupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, candidate.Account.ID)
		if err != nil {
			return nil, err
		}
		if intersectsInt(scope, groupIDs) {
			out = append(out, candidate)
		}
	}
	return out, nil
}

func (rt *runtimeState) gatewayPricing(ctx context.Context, req subscriptioncontract.PricingRequest, estimated bool) gatewayPricingEvidence {
	result, err := rt.subscriptions.EstimatePrice(ctx, req)
	if err != nil {
		rt.logger.Warn("failed to estimate gateway price", "error", err, "model_id", req.ModelID, "provider_id", req.ProviderID)
		return gatewayPricingEvidence{Amount: "0.00000000", Currency: "USD", PricingSource: "pricing_error", PricingEstimated: estimated}
	}
	source := "default_zero"
	if len(req.PricingOverride) > 0 {
		source = "mapping_override"
	} else if result.PricingRuleID != nil {
		source = "pricing_rule"
	}
	return gatewayPricingEvidence{
		Amount:           result.Amount,
		Currency:         result.Currency,
		PricingRuleID:    cloneIntPtr(result.PricingRuleID),
		PricingSource:    source,
		PricingEstimated: estimated,
	}.withDefaults()
}

func gatewayPricingRequest(modelID int, candidate schedulercontract.Candidate, usage gatewaycontract.Usage) subscriptioncontract.PricingRequest {
	return subscriptioncontract.PricingRequest{
		ModelID:         modelID,
		ProviderID:      candidate.Provider.ID,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		CacheReadTokens: usage.CachedTokens,
		At:              time.Now().UTC(),
		PricingOverride: cloneAnyMap(candidate.Mapping.PricingOverride),
	}
}

func (rt *runtimeState) gatewayUserPeriodUsage(ctx context.Context, userID int, now time.Time) (int, string, error) {
	logs, err := rt.usage.ListByUser(ctx, userID)
	if err != nil {
		return 0, "", err
	}
	start := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	tokens := 0
	cost := "0.00000000"
	for _, log := range logs {
		if !log.Success || log.CreatedAt.Before(start) {
			continue
		}
		tokens += log.TotalTokens
		cost = addDecimalMoney(cost, log.Cost)
	}
	return tokens, cost, nil
}

func gatewayModelReferences(canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution) []string {
	refs := []string{canonical.CanonicalModel, canonical.Model, resolution.Model.CanonicalName}
	if resolution.Alias != nil {
		refs = append(refs, resolution.Alias.Alias)
		refs = append(refs, resolution.Alias.FallbackModels...)
	}
	return uniqueNonEmptyStrings(refs)
}

func gatewayEntitlementErrorClass(decision subscriptioncontract.EntitlementDecision) string {
	switch strings.TrimSpace(decision.Reason) {
	case "model_not_allowed":
		return "entitlement_model_not_allowed"
	case "monthly_token_quota_exceeded":
		return "monthly_token_quota_exceeded"
	case "monthly_cost_quota_exceeded":
		return "monthly_cost_quota_exceeded"
	default:
		return "entitlement_denied"
	}
}

func gatewayEntitlementHTTPStatus(errorClass string) int {
	switch errorClass {
	case "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded":
		return http.StatusTooManyRequests
	default:
		return http.StatusForbidden
	}
}

func gatewayEntitlementErrorType(errorClass string) apiopenapi.GatewayErrorObjectType {
	switch errorClass {
	case "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded":
		return apiopenapi.RateLimitError
	default:
		return apiopenapi.PermissionError
	}
}

func gatewayEntitlementMessage(errorClass string) string {
	switch errorClass {
	case "entitlement_model_not_allowed":
		return "model not allowed by subscription entitlement"
	case "monthly_token_quota_exceeded":
		return "monthly token quota exceeded"
	case "monthly_cost_quota_exceeded":
		return "monthly cost quota exceeded"
	default:
		return "request not allowed by subscription entitlement"
	}
}

func estimateGatewayRequestUsage(canonical gatewaycontract.CanonicalRequest) gatewaycontract.Usage {
	inputTokens := estimateGatewayTokens(gatewayRequestText(canonical))
	outputTokens := max(1, inputTokens/2)
	if canonical.MaxOutputTokens != nil && *canonical.MaxOutputTokens > 0 {
		outputTokens = *canonical.MaxOutputTokens
	}
	return gatewaycontract.Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Estimated:    true,
	}
}

func gatewayRequestText(canonical gatewaycontract.CanonicalRequest) string {
	parts := []string{canonical.Prompt, canonical.Instructions}
	for _, message := range canonical.Messages {
		parts = append(parts, canonicalContentText(message.Content))
	}
	parts = append(parts, canonicalContentText(canonical.InputItems))
	return strings.Join(uniqueNonEmptyStrings(parts), "\n")
}

func estimateGatewayTokens(text string) int {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		if strings.TrimSpace(text) == "" {
			return 1
		}
		return max(1, len(text)/4)
	}
	return max(1, len(fields)*2)
}

func schedulerStrategyName(value string) schedulercontract.StrategyName {
	switch schedulercontract.StrategyName(strings.TrimSpace(value)) {
	case schedulercontract.StrategyBalanced:
		return schedulercontract.StrategyBalanced
	case schedulercontract.StrategyCostSaver:
		return schedulercontract.StrategyCostSaver
	default:
		return ""
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(trimmed)]; ok {
			continue
		}
		seen[strings.ToLower(trimmed)] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func addDecimalMoney(left string, right string) string {
	leftRat, ok := new(big.Rat).SetString(defaultDecimalMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := new(big.Rat).SetString(defaultDecimalMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return formatDecimalFixed(leftRat.Add(leftRat, rightRat), 8)
}

func defaultDecimalMoney(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.00000000"
	}
	return value
}

func formatDecimalFixed(value *big.Rat, scale int) string {
	if value == nil {
		value = new(big.Rat)
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(multiplier))
	numerator := new(big.Int).Set(scaled.Num())
	denominator := new(big.Int).Set(scaled.Denom())
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	doubleRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	if doubleRemainder.Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	raw := quotient.String()
	if scale == 0 {
		return raw
	}
	for len(raw) <= scale {
		raw = "0" + raw
	}
	return raw[:len(raw)-scale] + "." + raw[len(raw)-scale:]
}

func gatewayScheduleRequest(r *http.Request, canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution) schedulercontract.ScheduleRequest {
	req := schedulercontract.ScheduleRequest{
		RequestID:           canonical.RequestID,
		UserID:              canonical.UserID,
		APIKeyID:            canonical.APIKeyID,
		SourceProtocol:      string(canonical.SourceProtocol),
		SourceEndpoint:      canonical.SourceEndpoint,
		Model:               canonical.CanonicalModel,
		Strategy:            schedulercontract.StrategyBalanced,
		Warnings:            canonical.CompatibilityWarnings,
		RequestCapabilities: gatewayservice.CapabilityDescriptors(canonical),
	}
	if resolution.Alias != nil {
		req.ModelAlias = resolution.Alias.Alias
		req.FallbackModels = append([]string(nil), resolution.Alias.FallbackModels...)
		if strategy := schedulerStrategyHint(resolution.Alias.StrategyHint); strategy != "" {
			req.Strategy = strategy
		}
	}
	req.StickyAccountID, req.StickyStrength, req.SessionAffinityKey, req.SessionAffinitySource = gatewaySessionAffinity(r)
	return req
}

func schedulerStrategyHint(value *string) schedulercontract.StrategyName {
	if value == nil {
		return ""
	}
	switch schedulercontract.StrategyName(strings.TrimSpace(*value)) {
	case schedulercontract.StrategyBalanced:
		return schedulercontract.StrategyBalanced
	case schedulercontract.StrategyCostSaver:
		return schedulercontract.StrategyCostSaver
	default:
		return ""
	}
}

func gatewaySessionAffinity(r *http.Request) (*int, schedulercontract.StickyStrength, string, string) {
	strength := schedulerStickyStrength(firstNonEmpty(
		r.Header.Get("X-SRapi-Sticky-Strength"),
		r.URL.Query().Get("sticky_strength"),
	))
	accountID, accountSource := gatewayStickyAccountID(r)
	key, keySource := gatewaySessionAffinityKey(r)
	if strength == "" && (accountID != nil || key != "") {
		strength = schedulercontract.StickyStrengthSoft
	}
	if accountSource != "" {
		return accountID, strength, key, accountSource
	}
	return accountID, strength, key, keySource
}

func gatewayStickyAccountID(r *http.Request) (*int, string) {
	for _, candidate := range []struct {
		value  string
		source string
	}{
		{r.Header.Get("X-SRapi-Sticky-Account-ID"), "header:x-srapi-sticky-account-id"},
		{r.URL.Query().Get("sticky_account_id"), "query:sticky_account_id"},
	} {
		value := strings.TrimSpace(candidate.value)
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			return &parsed, candidate.source
		}
	}
	return nil, ""
}

func gatewaySessionAffinityKey(r *http.Request) (string, string) {
	for _, candidate := range []struct {
		value  string
		source string
	}{
		{r.Header.Get("X-SRapi-Session-Affinity-Key"), "header:x-srapi-session-affinity-key"},
		{r.URL.Query().Get("session_affinity_key"), "query:session_affinity_key"},
	} {
		value := strings.TrimSpace(candidate.value)
		if value != "" {
			return value, candidate.source
		}
	}
	return "", ""
}

func schedulerStickyStrength(value string) schedulercontract.StickyStrength {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(schedulercontract.StickyStrengthHard):
		return schedulercontract.StickyStrengthHard
	case string(schedulercontract.StickyStrengthSoft):
		return schedulercontract.StickyStrengthSoft
	default:
		return ""
	}
}

func stickyAccountIDFromCandidates(candidates []schedulercontract.Candidate, bindingKey string) *int {
	bindingKey = strings.TrimSpace(bindingKey)
	if bindingKey == "" {
		return nil
	}
	for _, candidate := range candidates {
		if accountMatchesAffinityKey(candidate.Account.Metadata, bindingKey) {
			accountID := candidate.Account.ID
			return &accountID
		}
	}
	return nil
}

func accountMatchesAffinityKey(metadata map[string]any, bindingKey string) bool {
	for _, key := range []string{"session_affinity_key", "sticky_binding_key", "sticky_session_key"} {
		if strings.EqualFold(metadataString(metadata, key), bindingKey) {
			return true
		}
	}
	for _, key := range []string{"session_affinity_keys", "sticky_binding_keys", "sticky_session_keys"} {
		if metadataStringListContains(metadata, key, bindingKey) {
			return true
		}
	}
	return false
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func metadataStringListContains(metadata map[string]any, key string, target string) bool {
	if metadata == nil {
		return false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return false
	}
	switch value := value.(type) {
	case []string:
		for _, item := range value {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprint(item)), target) {
				return true
			}
		}
	case string:
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	}
	return false
}

func (rt *runtimeState) gatewayCandidates(ctx context.Context, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey) ([]schedulercontract.Candidate, error) {
	model, err := rt.models.FindByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	mappings, err := rt.models.ListMappingsByModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	accounts, err := rt.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]schedulercontract.Candidate, 0)
	forcedProviderKey = strings.TrimSpace(forcedProviderKey)
	for _, mapping := range mappings {
		provider, err := rt.providers.FindByID(ctx, mapping.ProviderID)
		if err != nil {
			continue
		}
		if forcedProviderKey != "" && provider.Name != forcedProviderKey {
			continue
		}
		for _, account := range accounts {
			if account.ProviderID != mapping.ProviderID {
				continue
			}
			allowed, err := rt.apiKeyAllowsAccount(ctx, apiKey, account.ID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			runtimeState := rt.accountSchedulerRuntimeState(ctx, account)
			candidates = append(candidates, schedulercontract.Candidate{
				Account:               account,
				Provider:              provider,
				Mapping:               mapping,
				EffectiveCapabilities: effectiveCapabilities(model, mapping, provider, account),
				RuntimeState:          runtimeState,
				Limits:                schedulerRuntimeLimits(account.Metadata),
			})
		}
	}
	return candidates, nil
}

func (rt *runtimeState) accountSchedulerRuntimeState(ctx context.Context, account accountcontract.ProviderAccount) schedulercontract.RuntimeState {
	state := schedulerRuntimeState(account.Metadata)
	if latest, err := rt.accounts.LatestHealthSnapshotByAccount(ctx, account.ID); err == nil {
		healthScore := float64(latest.SuccessRate)
		state.HealthScore = &healthScore
		p95 := latest.LatencyP95MS
		state.LatencyP95MS = &p95
		state.CircuitOpen = state.CircuitOpen || strings.EqualFold(latest.CircuitState, "open")
		state.CooldownActive = state.CooldownActive || (latest.CooldownUntil != nil && latest.CooldownUntil.After(time.Now().UTC()))
	}
	if quotas, err := rt.accounts.ListQuotaSnapshotsByAccount(ctx, account.ID, 1); err == nil && len(quotas) > 0 {
		remainingRatio := float64(quotas[0].RemainingRatio)
		state.QuotaRemainingRatio = &remainingRatio
		state.QuotaExhausted = state.QuotaExhausted || quotas[0].RemainingRatio <= 0
	}
	return state
}

func (rt *runtimeState) apiKeyAllowsAccount(ctx context.Context, apiKey apikeycontract.APIKey, accountID int) (bool, error) {
	if len(apiKey.GroupIDs) == 0 {
		return true, nil
	}
	accountGroupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, accountID)
	if err != nil {
		return false, err
	}
	return intersectsInt(apiKey.GroupIDs, accountGroupIDs), nil
}

func intersectsInt(left []int, right []int) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[int]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func gatewaySourceEndpoint(ctx context.Context, fallback string) string {
	if route, ok := ctx.Value(gatewayRouteContextKey{}).(gatewayRouteContext); ok {
		if sourceEndpoint := strings.TrimSpace(route.SourceEndpoint); sourceEndpoint != "" {
			return sourceEndpoint
		}
	}
	return fallback
}

func gatewayForcedProviderKey(ctx context.Context) string {
	if route, ok := ctx.Value(gatewayRouteContextKey{}).(gatewayRouteContext); ok {
		return strings.TrimSpace(route.ForcedProviderKey)
	}
	return ""
}

func effectiveCapabilities(model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, account accountcontract.ProviderAccount) []capabilitiescontract.Descriptor {
	merged := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range model.Capabilities {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			merged[normalized.Key] = normalized
		}
	}
	for _, descriptor := range mapping.CapabilityOverride {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			merged[normalized.Key] = normalized
		}
	}
	for key, value := range provider.Capabilities {
		capabilityKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(key)
		if ok && boolValue(value) {
			merged[capabilityKey] = capabilityRequirement(capabilityKey)
		}
	}
	for key, value := range account.Metadata {
		if strings.HasPrefix(key, "capability_") && boolValue(value) {
			capabilityKey := strings.TrimPrefix(key, "capability_")
			if canonicalKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(capabilityKey); ok {
				merged[canonicalKey] = capabilityRequirement(canonicalKey)
			}
		}
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(merged))
	for _, descriptor := range merged {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func capabilityRequirement(key string) capabilitiescontract.Descriptor {
	return capabilitiescontract.Descriptor{
		Key:     key,
		Level:   capabilitiescontract.DescriptorLevelRequired,
		Status:  capabilitiescontract.DescriptorStatusStable,
		Version: "v1",
	}
}

func dedupeCapabilityDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]capabilitiescontract.Descriptor{}
	for _, value := range values {
		seen[value.Key] = value
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func schedulerRuntimeState(metadata map[string]any) schedulercontract.RuntimeState {
	quotaRemainingRatio := metadataOptionalFloat(metadata, "remaining_ratio", "quota_remaining_ratio")
	quotaExhausted := metadataBool(metadata, "quota_exhausted")
	if quotaRemainingRatio != nil && *quotaRemainingRatio <= 0 {
		quotaExhausted = true
	}
	return schedulercontract.RuntimeState{
		QuotaExhausted:      quotaExhausted,
		HealthScore:         metadataOptionalFloat(metadata, "health_score"),
		QuotaRemainingRatio: quotaRemainingRatio,
		LatencyP95MS:        metadataOptionalInt(metadata, "latency_p95_ms", "p95_latency_ms", "latency_p95"),
		CircuitOpen:         metadataBool(metadata, "circuit_open"),
		CooldownActive:      metadataBool(metadata, "cooldown_active") || metadataCooldownActive(metadata, time.Now().UTC()),
		CurrentConcurrency:  metadataInt(metadata, "current_concurrency"),
		RPMUsed:             metadataInt(metadata, "rpm_used"),
		TPMUsed:             metadataInt(metadata, "tpm_used"),
	}
}

func schedulerRuntimeLimits(metadata map[string]any) schedulercontract.RuntimeLimits {
	return schedulercontract.RuntimeLimits{
		MaxConcurrency: metadataOptionalInt(metadata, "max_concurrency"),
		RPMLimit:       metadataOptionalInt(metadata, "rpm_limit"),
		TPMLimit:       metadataOptionalInt(metadata, "tpm_limit"),
	}
}

func metadataBool(metadata map[string]any, key string) bool {
	return boolValue(metadata[key])
}

func boolValue(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	default:
		return false
	}
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	switch value := value.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return int(parsed)
		}
		floatValue, err := value.Float64()
		if err == nil {
			return int(floatValue)
		}
	case string:
		raw := strings.TrimSpace(value)
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			return parsed
		}
		floatValue, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return int(floatValue)
		}
	}
	return 0
}

func metadataOptionalInt(metadata map[string]any, keys ...string) *int {
	if _, ok := metadataValue(metadata, keys...); !ok {
		return nil
	}
	value := metadataInt(metadata, keys...)
	return &value
}

func metadataOptionalFloat(metadata map[string]any, keys ...string) *float64 {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	switch value := value.(type) {
	case int:
		out := float64(value)
		return &out
	case int8:
		out := float64(value)
		return &out
	case int16:
		out := float64(value)
		return &out
	case int32:
		out := float64(value)
		return &out
	case int64:
		out := float64(value)
		return &out
	case uint:
		out := float64(value)
		return &out
	case uint8:
		out := float64(value)
		return &out
	case uint16:
		out := float64(value)
		return &out
	case uint32:
		out := float64(value)
		return &out
	case uint64:
		out := float64(value)
		return &out
	case float32:
		out := float64(value)
		return &out
	case float64:
		return &value
	case json.Number:
		parsed, err := value.Float64()
		if err == nil {
			return &parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func metadataValue(metadata map[string]any, keys ...string) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func metadataCooldownActive(metadata map[string]any, now time.Time) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata["cooldown_until"]
	if !ok {
		return false
	}
	var raw string
	switch value := value.(type) {
	case string:
		raw = value
	default:
		raw = fmt.Sprint(value)
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	return err == nil && now.Before(until)
}

func (rt *runtimeState) invokeProviderText(ctx context.Context, req provideradaptercontract.TextRequest) (provideradaptercontract.TextResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.TextResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.TextResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.TextResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeText(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.TextResponse{}, err
	}
	return resp, nil
}

func providerTextRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.TextRequest {
	return provideradaptercontract.TextRequest{
		RequestID:       req.RequestID,
		SourceProtocol:  string(req.SourceProtocol),
		SourceEndpoint:  req.SourceEndpoint,
		Model:           req.CanonicalModel,
		Prompt:          req.Prompt,
		Messages:        providerTextMessages(req),
		Instructions:    req.Instructions,
		Stream:          req.Stream,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxOutputTokens,
		Stop:            append([]string(nil), req.Stop...),
		Tools:           cloneMapSlice(req.Tools),
		ToolChoice:      cloneAnyValue(req.ToolChoice),
		ResponseFormat:  cloneAnyMap(req.ResponseFormat),
		Provider:        candidate.Provider,
		Account:         candidate.Account,
		Mapping:         candidate.Mapping,
	}
}

func providerTextMessages(req gatewaycontract.CanonicalRequest) []provideradaptercontract.TextMessage {
	out := make([]provideradaptercontract.TextMessage, 0, len(req.Messages)+1)
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		content := canonicalContentText(message.Content)
		if content == "" {
			continue
		}
		out = append(out, provideradaptercontract.TextMessage{Role: role, Content: content})
	}
	if len(out) == 0 {
		content := canonicalContentText(req.InputItems)
		if content != "" {
			out = append(out, provideradaptercontract.TextMessage{Role: "user", Content: content})
		}
	}
	return out
}

func canonicalContentText(blocks []gatewaycontract.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func (rt *runtimeState) refreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	if !shouldRefreshCredential(account, credential) {
		return credential, false, nil
	}
	before, err := rt.accounts.FindByID(ctx, account.ID)
	if err != nil {
		rt.logger.Warn("failed to load provider account before refresh", "error", err, "account_id", account.ID)
		return credential, false, err
	}
	response, err := rt.reverseProxy.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseProxyAccountRuntime(account, credential),
	})
	if err != nil {
		rt.recordAudit(ctx, auditcontract.RecordRequest{
			Action:       "provider_account.oauth_refresh_failed",
			ResourceType: "provider_account",
			ResourceID:   strconv.Itoa(account.ID),
			Before:       accountAuditSnapshot(before),
			After:        map[string]any{"refresh_status": "failed", "error_class": errorClassName(err)},
			TraceID:      requestIDFromContext(ctx),
		})
		return credential, false, err
	}
	refreshed := response.Credential
	updated, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Credential: &refreshed})
	if err != nil {
		rt.logger.Warn("failed to persist refreshed provider credential", "error", err, "account_id", account.ID)
		return credential, false, err
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.oauth_refresh",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(account.ID),
		Before:       accountAuditSnapshot(before),
		After: map[string]any{
			"refresh_status":     "success",
			"refreshed_at":       response.RefreshedAt,
			"credential_version": updated.CredentialVersion,
		},
		TraceID: requestIDFromContext(ctx),
	})
	return refreshed, true, nil
}

func shouldRefreshCredential(account accountcontract.ProviderAccount, credential map[string]any) bool {
	if account.RuntimeClass != accountcontract.RuntimeClassOauthRefresh && account.RuntimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		return false
	}
	if metadataBool(account.Metadata, "force_refresh") || metadataBool(account.Metadata, "access_token_expired") {
		return true
	}
	expiresAt := mapString(credential, "expires_at")
	if expiresAt == "" {
		return mapString(credential, "access_token") == ""
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	return err == nil && time.Now().UTC().After(parsed.Add(-30*time.Second))
}

func errorClassName(err error) string {
	var runtimeErr reverseproxycontract.RuntimeError
	if errors.As(err, &runtimeErr) && strings.TrimSpace(runtimeErr.Class) != "" {
		return runtimeErr.Class
	}
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) && strings.TrimSpace(providerErr.Class) != "" {
		return providerErr.Class
	}
	return "unknown"
}

func (rt *runtimeState) applyProviderAccountProtection(ctx context.Context, account accountcontract.ProviderAccount, err error) {
	if account.ID <= 0 || account.RuntimeClass == accountcontract.RuntimeClassAPIKey {
		return
	}
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return
	}
	nextStatus, ok := reverseProxyAccountFailureStatus(providerErr.Class)
	if !ok || account.Status == nextStatus {
		return
	}
	before, findErr := rt.accounts.FindByID(ctx, account.ID)
	if findErr != nil {
		rt.logger.Warn("failed to load reverse proxy account for protection", "error", findErr, "account_id", account.ID)
		return
	}
	updated, updateErr := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Status: &nextStatus})
	if updateErr != nil {
		rt.logger.Warn("failed to protect reverse proxy account", "error", updateErr, "account_id", account.ID, "status", nextStatus)
		return
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.auto_protect",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(account.ID),
		Before:       accountAuditSnapshot(before),
		After:        accountAuditSnapshot(updated),
		TraceID:      requestIDFromContext(ctx),
	})
}

func reverseProxyAccountFailureStatus(class string) (accountcontract.Status, bool) {
	switch strings.TrimSpace(class) {
	case "session_invalid", "account_locked", "device_unrecognized":
		return accountcontract.StatusNeedsReauth, true
	case "account_banned", "abuse_detected":
		return accountcontract.StatusDisabled, true
	default:
		return "", false
	}
}

func gatewayUsageFromProvider(resp provideradaptercontract.TextResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func providerGatewayError(err error) (string, int, apiopenapi.GatewayErrorObjectType) {
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) {
		errorClass := strings.TrimSpace(providerErr.Class)
		if errorClass == "" {
			errorClass = "upstream_error"
		}
		statusCode := providerErr.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		return errorClass, statusCode, gatewayErrorTypeForProviderClass(errorClass)
	}
	return "upstream_error", http.StatusBadGateway, apiopenapi.UpstreamError
}

func gatewayErrorTypeForProviderClass(errorClass string) apiopenapi.GatewayErrorObjectType {
	switch errorClass {
	case "invalid_request":
		return apiopenapi.InvalidRequestError
	case "rate_limit":
		return apiopenapi.RateLimitError
	case "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized":
		return apiopenapi.PermissionError
	case "timeout", "network_error", "stream_interrupted", "no_available_account":
		return apiopenapi.ServiceUnavailableError
	default:
		return apiopenapi.UpstreamError
	}
}

func providerGatewayHTTPStatus(upstreamStatus int) int {
	switch upstreamStatus {
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusBadGateway
	case http.StatusBadRequest:
		return http.StatusBadRequest
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return http.StatusGatewayTimeout
	default:
		if upstreamStatus >= 500 {
			return http.StatusBadGateway
		}
		return http.StatusBadGateway
	}
}

func providerGatewayMessage(errorClass string) string {
	switch errorClass {
	case "rate_limit":
		return "provider rate limit"
	case "auth_failed", "auth_error", "credential_error":
		return "provider authentication failed"
	case "invalid_request":
		return "provider rejected request"
	case "model_unavailable":
		return "provider model unavailable"
	case "provider_5xx":
		return "provider service error"
	case "session_invalid":
		return "provider session invalid"
	case "account_locked":
		return "provider account locked"
	case "account_banned":
		return "provider account banned"
	case "abuse_detected":
		return "provider abuse signal detected"
	case "challenge_required", "captcha_required":
		return "provider challenge required"
	case "geo_blocked":
		return "provider geo blocked"
	case "device_unrecognized":
		return "provider device unrecognized"
	case "upstream_client_outdated":
		return "provider upstream client outdated"
	case "timeout":
		return "provider request timed out"
	case "network_error":
		return "provider network error"
	case "stream_interrupted":
		return "provider stream interrupted"
	default:
		return "provider request failed"
	}
}

func (rt *runtimeState) recordGatewayUsage(ctx context.Context, rec gatewayUsageRecord) {
	model := fallbackModelName(rec.Model)
	if rec.AttemptNo == 0 {
		rec.AttemptNo = 1
	}
	pricing := rec.Pricing.withDefaults()
	usageLog, usageErr := rt.usage.Record(ctx, usagecontract.RecordRequest{
		RequestID:             rec.RequestID,
		UserID:                rec.Authed.UserID,
		APIKeyID:              rec.Authed.Key.ID,
		ProviderID:            rec.ProviderID,
		AccountID:             rec.AccountID,
		SourceProtocol:        rec.SourceProtocol,
		SourceEndpoint:        rec.SourceEndpoint,
		TargetProtocol:        rec.TargetProtocol,
		Model:                 model,
		InputTokens:           rec.InputTokens,
		OutputTokens:          rec.OutputTokens,
		CachedTokens:          rec.CachedTokens,
		UsageEstimated:        rec.UsageEstimated,
		LatencyMS:             rec.LatencyMS,
		Success:               rec.Success,
		ErrorClass:            rec.ErrorClass,
		Cost:                  pricing.Amount,
		Currency:              pricing.Currency,
		CompatibilityWarnings: rec.CompatibilityWarnings,
	})
	if usageErr != nil {
		rt.logger.Warn("failed to record usage log", "error", usageErr, "request_id", rec.RequestID)
		rt.enqueueGatewayUsageFailureEvent(ctx, rec, model)
	} else {
		rt.recordUsageBilling(ctx, usageLog, pricing)
		rt.enqueueGatewayUsageEvent(ctx, usageLog)
	}
	if rec.DecisionID <= 0 || rec.AccountID == nil || rec.ProviderID == nil {
		return
	}
	_, feedbackErr := rt.scheduler.RecordFeedback(ctx, schedulercontract.RecordFeedbackRequest{
		RequestID:    rec.RequestID,
		DecisionID:   rec.DecisionID,
		AttemptNo:    rec.AttemptNo,
		AccountID:    *rec.AccountID,
		ProviderID:   *rec.ProviderID,
		Model:        model,
		Success:      rec.Success,
		ErrorClass:   rec.ErrorClass,
		StatusCode:   rec.StatusCode,
		LatencyMS:    rec.LatencyMS,
		InputTokens:  rec.InputTokens,
		OutputTokens: rec.OutputTokens,
		CachedTokens: rec.CachedTokens,
		ActualCost:   pricing.Amount,
		Currency:     pricing.Currency,
	})
	if feedbackErr != nil {
		rt.logger.Warn("failed to record scheduler feedback", "error", feedbackErr, "request_id", rec.RequestID)
	}
	if !rec.Success && rec.ErrorClass != nil && *rec.ErrorClass == "rate_limit" {
		rt.applyProviderRateLimitCooldown(ctx, *rec.AccountID)
	}
	rt.recordGatewayAccountSnapshots(ctx, rec)
}

func (rt *runtimeState) applyProviderRateLimitCooldown(ctx context.Context, accountID int) {
	if accountID <= 0 {
		return
	}
	account, err := rt.accounts.FindByID(ctx, accountID)
	if err != nil {
		rt.logger.Warn("failed to load rate-limited provider account", "error", err, "account_id", accountID)
		return
	}
	metadata := cloneMetadata(account.Metadata)
	metadata["cooldown_active"] = true
	metadata["cooldown_reason"] = "rate_limit"
	metadata["cooldown_until"] = time.Now().UTC().Add(rateLimitCooldownWindow).Format(time.RFC3339)
	metadata["last_error_class"] = "rate_limit"
	before := accountAuditSnapshot(account)
	updated, err := rt.accounts.Update(ctx, accountID, accountcontract.UpdateRequest{Metadata: &metadata})
	if err != nil {
		rt.logger.Warn("failed to apply provider account rate limit cooldown", "error", err, "account_id", accountID)
		return
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.cooldown",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(accountID),
		Before:       before,
		After:        accountAuditSnapshot(updated),
		TraceID:      requestIDFromContext(ctx),
	})
}

func (rt *runtimeState) recordGatewayAccountSnapshots(ctx context.Context, rec gatewayUsageRecord) {
	if rec.AccountID == nil || rec.ProviderID == nil {
		return
	}
	account, err := rt.accounts.FindByID(ctx, *rec.AccountID)
	if err != nil {
		rt.logger.Warn("failed to load provider account for snapshot", "error", err, "account_id", *rec.AccountID)
		return
	}
	usageLogs, err := rt.usage.List(ctx)
	if err != nil {
		rt.logger.Warn("failed to list usage logs for account snapshot", "error", err, "account_id", *rec.AccountID)
		return
	}
	now := time.Now().UTC()
	health := buildAccountHealthSnapshot(account, usageLogsForAccount(usageLogs, account.ID), now)
	if _, err := rt.accounts.RecordHealthSnapshot(ctx, accountHealthSnapshotFromAPI(health)); err != nil {
		rt.logger.Warn("failed to record account health snapshot", "error", err, "account_id", account.ID)
	}
	quota := buildAccountQuotaSnapshot(account, usageLogsForAccount(usageLogs, account.ID), now)
	if _, err := rt.accounts.RecordQuotaSnapshot(ctx, accountQuotaSnapshotFromAPI(quota)); err != nil {
		rt.logger.Warn("failed to record account quota snapshot", "error", err, "account_id", account.ID)
	}
}

func (rt *runtimeState) recordAccountTestHealthSnapshot(ctx context.Context, account accountcontract.ProviderAccount, result apiopenapi.AdminTestResult) {
	status := "healthy"
	successRate := float32(1)
	errorRate := float32(0)
	if !result.Ok {
		status = "degraded"
		successRate = 0
		errorRate = 1
	}
	latencyMS := 0
	if result.LatencyMs != nil {
		latencyMS = *result.LatencyMs
	}
	_, err := rt.accounts.RecordHealthSnapshot(ctx, accountcontract.AccountHealthSnapshot{
		AccountID:     account.ID,
		ProviderID:    account.ProviderID,
		Status:        status,
		SuccessRate:   successRate,
		ErrorRate:     errorRate,
		LatencyP50MS:  latencyMS,
		LatencyP95MS:  latencyMS,
		CircuitState:  accountCircuitState(account),
		SnapshotAt:    result.CheckedAt,
		CooldownUntil: metadataOptionalTime(account.Metadata, "cooldown_until"),
	})
	if err != nil {
		rt.logger.Warn("failed to record account test health snapshot", "error", err, "account_id", account.ID)
	}
}

func (rt *runtimeState) recordUsageBilling(ctx context.Context, log usagecontract.UsageLog, pricing gatewayPricingEvidence) {
	if !log.Success {
		return
	}
	pricing = pricing.withDefaults()
	metadata := map[string]any{
		"request_id":        log.RequestID,
		"model":             log.Model,
		"source_endpoint":   log.SourceEndpoint,
		"total_tokens":      log.TotalTokens,
		"usage_estimated":   log.UsageEstimated,
		"pricing_source":    pricing.PricingSource,
		"pricing_estimated": pricing.PricingEstimated,
	}
	if pricing.PricingRuleID != nil {
		metadata["pricing_rule_id"] = *pricing.PricingRuleID
	}
	_, err := rt.billing.Record(ctx, billingcontract.RecordRequest{
		UserID:        log.UserID,
		Type:          billingcontract.LedgerTypeUsageCharge,
		Amount:        log.Cost,
		Currency:      log.Currency,
		BalanceBefore: "0.00000000",
		BalanceAfter:  "0.00000000",
		ReferenceType: "usage_log",
		ReferenceID:   strconv.Itoa(log.ID),
		Metadata:      metadata,
	})
	if err != nil {
		rt.logger.Warn("failed to record billing ledger", "error", err, "request_id", log.RequestID)
	}
}

func (rt *runtimeState) enqueueGatewayUsageEvent(ctx context.Context, log usagecontract.UsageLog) {
	payload := map[string]any{
		"usage_log_id":           log.ID,
		"request_id":             log.RequestID,
		"user_id":                log.UserID,
		"api_key_id":             log.APIKeyID,
		"source_protocol":        log.SourceProtocol,
		"source_endpoint":        log.SourceEndpoint,
		"target_protocol":        log.TargetProtocol,
		"model":                  log.Model,
		"input_tokens":           log.InputTokens,
		"output_tokens":          log.OutputTokens,
		"cached_tokens":          log.CachedTokens,
		"total_tokens":           log.TotalTokens,
		"success":                log.Success,
		"usage_estimated":        log.UsageEstimated,
		"compatibility_warnings": nonNilStrings(log.CompatibilityWarnings),
	}
	addOptionalInt(payload, "provider_id", log.ProviderID)
	addOptionalInt(payload, "account_id", log.AccountID)
	if log.ErrorClass != nil {
		payload["error_class"] = *log.ErrorClass
	}
	_, err := rt.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "usage_log",
		AggregateID:    strconv.Itoa(log.ID),
		CorrelationID:  log.RequestID,
		CausationID:    log.RequestID,
		IdempotencyKey: log.RequestID,
		Payload:        payload,
		Metadata: map[string]any{
			"source_protocol": log.SourceProtocol,
			"source_endpoint": log.SourceEndpoint,
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway usage event", "error", err, "request_id", log.RequestID)
	}
}

func (rt *runtimeState) enqueueGatewayUsageFailureEvent(ctx context.Context, rec gatewayUsageRecord, model string) {
	payload := map[string]any{
		"request_id":             rec.RequestID,
		"user_id":                rec.Authed.UserID,
		"api_key_id":             rec.Authed.Key.ID,
		"source_protocol":        rec.SourceProtocol,
		"source_endpoint":        rec.SourceEndpoint,
		"target_protocol":        rec.TargetProtocol,
		"model":                  model,
		"input_tokens":           rec.InputTokens,
		"output_tokens":          rec.OutputTokens,
		"cached_tokens":          rec.CachedTokens,
		"total_tokens":           rec.InputTokens + rec.OutputTokens + rec.CachedTokens,
		"success":                rec.Success,
		"usage_estimated":        rec.UsageEstimated,
		"compatibility_warnings": nonNilStrings(rec.CompatibilityWarnings),
	}
	addOptionalInt(payload, "provider_id", rec.ProviderID)
	addOptionalInt(payload, "account_id", rec.AccountID)
	if rec.ErrorClass != nil {
		payload["error_class"] = *rec.ErrorClass
	}
	_, err := rt.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "gateway_request",
		AggregateID:    rec.RequestID,
		CorrelationID:  rec.RequestID,
		CausationID:    rec.RequestID,
		IdempotencyKey: rec.RequestID,
		Payload:        payload,
		Metadata: map[string]any{
			"source_protocol": rec.SourceProtocol,
			"source_endpoint": rec.SourceEndpoint,
			"usage_recorded":  false,
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway usage failure event", "error", err, "request_id", rec.RequestID)
	}
}

func (rt *runtimeState) recordAudit(ctx context.Context, req auditcontract.RecordRequest) {
	if _, err := rt.audit.Record(ctx, req); err != nil {
		rt.logger.Warn("failed to record audit log", "error", err, "action", req.Action, "resource_type", req.ResourceType, "resource_id", req.ResourceID)
	}
}

func auditRecordFromRequest(r *http.Request, actorUserID int, action, resourceType, resourceID string, before, after map[string]any) auditcontract.RecordRequest {
	return auditcontract.RecordRequest{
		ActorUserID:  ptrInt(actorUserID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
		IP:           clientIP(r),
		UserAgent:    r.UserAgent(),
		TraceID:      requestIDFromContext(r.Context()),
	}
}

func providerAuditSnapshot(provider providercontract.Provider) map[string]any {
	return map[string]any{
		"name":          provider.Name,
		"display_name":  provider.DisplayName,
		"adapter_type":  provider.AdapterType,
		"protocol":      provider.Protocol,
		"status":        provider.Status,
		"capabilities":  provider.Capabilities,
		"config_schema": provider.ConfigSchema,
	}
}

func modelAuditSnapshot(model modelcontract.Model) map[string]any {
	return map[string]any{
		"canonical_name":    model.CanonicalName,
		"display_name":      model.DisplayName,
		"family":            model.Family,
		"context_window":    model.ContextWindow,
		"max_output_tokens": model.MaxOutputTokens,
		"quality_tier":      model.QualityTier,
		"status":            model.Status,
		"capabilities":      model.Capabilities,
	}
}

func accountAuditSnapshot(account accountcontract.ProviderAccount) map[string]any {
	return map[string]any{
		"provider_id":        account.ProviderID,
		"name":               account.Name,
		"runtime_class":      account.RuntimeClass,
		"upstream_client":    account.UpstreamClient,
		"proxy_id":           account.ProxyID,
		"status":             account.Status,
		"priority":           account.Priority,
		"weight":             account.Weight,
		"risk_level":         account.RiskLevel,
		"metadata":           account.Metadata,
		"credential_version": account.CredentialVersion,
	}
}

func accountGroupAuditSnapshot(group accountcontract.AccountGroup) map[string]any {
	return map[string]any{
		"name":           group.Name,
		"description":    group.Description,
		"provider_scope": cloneAnyMap(group.ProviderScope),
		"model_scope":    cloneAnyMap(group.ModelScope),
		"strategy_hint":  group.StrategyHint,
		"status":         group.Status,
	}
}

func apiKeyAuditSnapshot(key apikeycontract.APIKey) map[string]any {
	return map[string]any{
		"name":           key.Name,
		"prefix":         key.Prefix,
		"status":         key.Status,
		"scopes":         append([]string(nil), key.Scopes...),
		"allowed_models": append([]string(nil), key.AllowedModels...),
		"group_ids":      append([]int(nil), key.GroupIDs...),
	}
}

func subscriptionPlanAuditSnapshot(plan subscriptioncontract.SubscriptionPlan) map[string]any {
	return map[string]any{
		"name":          plan.Name,
		"description":   plan.Description,
		"price":         plan.Price,
		"currency":      plan.Currency,
		"validity_days": plan.ValidityDays,
		"entitlements":  cloneAnyMap(plan.Entitlements),
		"for_sale":      plan.ForSale,
		"sort_order":    plan.SortOrder,
		"status":        plan.Status,
	}
}

func userSubscriptionAuditSnapshot(subscription subscriptioncontract.UserSubscription) map[string]any {
	return map[string]any{
		"user_id":               subscription.UserID,
		"plan_id":               subscription.PlanID,
		"status":                subscription.Status,
		"starts_at":             subscription.StartsAt,
		"expires_at":            subscription.ExpiresAt,
		"entitlements_snapshot": cloneAnyMap(subscription.EntitlementsSnapshot),
		"source_type":           subscription.SourceType,
		"source_id":             subscription.SourceID,
	}
}

func pricingRuleAuditSnapshot(rule subscriptioncontract.PricingRule) map[string]any {
	return map[string]any{
		"model_id":                             rule.ModelID,
		"provider_id":                          rule.ProviderID,
		"input_price_per_million_tokens":       rule.InputPricePerMillionTokens,
		"output_price_per_million_tokens":      rule.OutputPricePerMillionTokens,
		"cache_read_price_per_million_tokens":  rule.CacheReadPricePerMillionTokens,
		"cache_write_price_per_million_tokens": rule.CacheWritePricePerMillionTokens,
		"currency":                             rule.Currency,
		"effective_from":                       rule.EffectiveFrom,
		"effective_to":                         rule.EffectiveTo,
	}
}

func cloneMetadata(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func sanitizedExportMetadata(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		if sensitiveMetadataKey(key) {
			continue
		}
		out[key] = sanitizeExportMetadataValue(item)
	}
	return out
}

func sanitizeExportMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizedExportMetadata(typed)
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for idx, item := range typed {
			out[idx] = sanitizedExportMetadata(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = sanitizeExportMetadataValue(item)
		}
		return out
	default:
		return typed
	}
}

func sensitiveMetadataKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.NewReplacer("-", "_", " ", "_").Replace(key)
	if key == "key" || strings.HasSuffix(key, "_key") {
		return true
	}
	sensitiveMarkers := []string{
		"authorization",
		"bearer",
		"cookie",
		"credential",
		"password",
		"passwd",
		"private_key",
		"secret",
		"session_affinity_key",
		"token",
	}
	for _, marker := range sensitiveMarkers {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, len(values))
	for idx, value := range values {
		out[idx] = cloneAnyMap(value)
	}
	return out
}

func cloneAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneAnyValue(item)
	}
	return out
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []map[string]any:
		return cloneMapSlice(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAnyValue(item)
		}
		return out
	default:
		return typed
	}
}

func elapsedMillis(startedAt time.Time) int {
	return max(0, int(time.Since(startedAt).Milliseconds()))
}

func fallbackModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "unknown"
	}
	return model
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) requireConsoleSession(r *http.Request) (authcontract.LoginResult, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return authcontract.LoginResult{}, authservice.ErrSessionNotFound
	}
	return s.runtime.auth.AuthenticateSession(r.Context(), cookie.Value)
}

func (s *Server) requireAdminSession(r *http.Request) (authcontract.LoginResult, error) {
	session, err := s.requireConsoleSession(r)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	for _, role := range session.User.Roles {
		if role == userscontract.RoleOwner || role == userscontract.RoleAdmin {
			return session, nil
		}
	}
	return authcontract.LoginResult{}, errors.New("admin access required")
}

func (s *Server) requireGatewayKey(r *http.Request) (apikeycontract.AuthResult, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return apikeycontract.AuthResult{}, apikeyservice.ErrInvalidKey
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return apikeycontract.AuthResult{}, apikeyservice.ErrInvalidKey
	}
	return s.runtime.apiKeys.Authenticate(r.Context(), parts[1])
}

func (s *Server) apiKeyByUser(ctx context.Context, userID, keyID int) (apikeycontract.APIKey, error) {
	keys, err := s.runtime.apiKeys.ListByUser(ctx, userID)
	if err != nil {
		return apikeycontract.APIKey{}, err
	}
	for _, key := range keys {
		if key.ID == keyID {
			return key, nil
		}
	}
	return apikeycontract.APIKey{}, apikeyservice.ErrKeyNotFound
}

func (s *Server) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	limited := http.MaxBytesReader(w, r.Body, s.cfg.Gateway.MaxBodySize)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return err
	}
	return nil
}

func jsonDecodeStatus(err error) int {
	if errors.Is(err, errRequestTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func writeJSONAny(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeStandardError(w http.ResponseWriter, status int, code apiopenapi.ErrorCode, message, requestID string) {
	writeJSONAny(w, status, apiopenapi.ErrorResponse{
		Error: apiopenapi.ErrorObject{
			Code:    code,
			Message: message,
			TraceId: requestID,
		},
		RequestId: requestID,
	})
}

func writeGatewayError(w http.ResponseWriter, status int, typ apiopenapi.GatewayErrorObjectType, message, code string) {
	var codePtr *string
	if code != "" {
		codePtr = &code
	}
	writeJSONAny(w, status, apiopenapi.GatewayErrorResponse{
		Error: apiopenapi.GatewayErrorObject{
			Code:    codePtr,
			Message: message,
			Param:   nil,
			Type:    typ,
		},
	})
}

func writeGatewayAuthError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, apikeyservice.ErrInvalidKey), errors.Is(err, apikeyservice.ErrInvalidInput):
		writeGatewayError(w, http.StatusUnauthorized, apiopenapi.AuthenticationError, "invalid API key", "invalid_api_key")
	case errors.Is(err, apikeyservice.ErrKeyDisabled), errors.Is(err, apikeyservice.ErrKeyExpired):
		writeGatewayError(w, http.StatusForbidden, apiopenapi.PermissionError, "API key disabled or expired", "api_key_disabled")
	default:
		writeGatewayError(w, http.StatusInternalServerError, apiopenapi.InternalError, "failed to authenticate API key", "internal_error")
	}
	_ = requestID
}

func writePaymentServiceError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, paymentservice.ErrInvalidInput), errors.Is(err, paymentservice.ErrInvalidTransition), errors.Is(err, paymentservice.ErrOrderMismatch):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid payment request", requestID)
	case errors.Is(err, paymentservice.ErrSignatureInvalid):
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid payment webhook signature", requestID)
	case errors.Is(err, paymentservice.ErrProviderUnavailable):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "payment provider unavailable", requestID)
	case errors.Is(err, paymentcontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "payment resource not found", requestID)
	case errors.Is(err, paymentcontract.ErrConflict):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "payment resource conflict", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "payment service error", requestID)
	}
}

func validateCSRF(session authcontract.Session, token string) error {
	return authservice.ValidateCSRF(session, token)
}

func derefStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(*values))
	copy(cloned, *values)
	return cloned
}

func idsToInts(values *[]apiopenapi.Id) ([]int, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]int, 0, len(*values))
	for _, value := range *values {
		parsed, err := strconv.Atoi(string(value))
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", value, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func apiIDs(values []int) []apiopenapi.Id {
	if values == nil {
		return []apiopenapi.Id{}
	}
	out := make([]apiopenapi.Id, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.Id(strconv.Itoa(value)))
	}
	return out
}

func apiIDsPtr(values []int) *[]apiopenapi.Id {
	if len(values) == 0 {
		return nil
	}
	out := apiIDs(values)
	return &out
}

func apiIDsToInts(values *[]apiopenapi.Id) ([]int, error) {
	ids, err := idsToInts(values)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("invalid id %d", id)
		}
	}
	return ids, nil
}

func accountGroupMemberPathIDs(w http.ResponseWriter, r *http.Request, requestID string) (int, int, bool) {
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return 0, 0, false
	}
	accountID, err := strconv.Atoi(r.PathValue("account_id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return 0, 0, false
	}
	return groupID, accountID, true
}

func paginateApiKeys(keys []apikeycontract.APIKey, page, pageSize int) ([]apikeycontract.APIKey, int, bool) {
	total := len(keys)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= total {
		return []apikeycontract.APIKey{}, total, false
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return keys[start:end], total, end < total
}

func filterApiKeys(keys []apikeycontract.APIKey, status string) []apikeycontract.APIKey {
	status = strings.TrimSpace(status)
	if status == "" {
		return keys
	}
	out := make([]apikeycontract.APIKey, 0, len(keys))
	for _, key := range keys {
		if string(key.Status) == status {
			out = append(out, key)
		}
	}
	return out
}

func filterGatewayModels(models []apiopenapi.OpenAIModel, allowed []string) []apiopenapi.OpenAIModel {
	if len(allowed) == 0 {
		out := make([]apiopenapi.OpenAIModel, len(models))
		copy(out, models)
		return out
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, model := range allowed {
		allowedSet[model] = struct{}{}
	}
	out := make([]apiopenapi.OpenAIModel, 0, len(models))
	for _, model := range models {
		if _, ok := allowedSet[model.Id]; ok {
			out = append(out, model)
		}
	}
	return out
}

func toGatewayModels(models []modelcontract.Model) []apiopenapi.OpenAIModel {
	out := make([]apiopenapi.OpenAIModel, 0, len(models))
	for _, model := range models {
		if model.Status != modelcontract.StatusActive {
			continue
		}
		created := int(model.CreatedAt.Unix())
		out = append(out, apiopenapi.OpenAIModel{
			Created: &created,
			Id:      model.CanonicalName,
			Object:  apiopenapi.OpenAIModelObjectModel,
			OwnedBy: "srapi",
		})
	}
	return out
}

func toAPIUser(user userscontract.User) apiopenapi.User {
	roles := make([]apiopenapi.UserRole, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, apiopenapi.UserRole(role))
	}
	return apiopenapi.User{
		CreatedAt: user.CreatedAt,
		Email:     openapi_types.Email(user.Email),
		Id:        apiopenapi.Id(strconv.Itoa(user.ID)),
		Name:      user.Name,
		Roles:     roles,
		Status:    apiopenapi.UserStatus(user.Status),
	}
}

func toAPIKey(key apikeycontract.APIKey) apiopenapi.ApiKey {
	groupIDs := make([]apiopenapi.Id, 0, len(key.GroupIDs))
	for _, id := range key.GroupIDs {
		groupIDs = append(groupIDs, apiopenapi.Id(strconv.Itoa(id)))
	}
	return apiopenapi.ApiKey{
		AllowedModels: append([]string(nil), key.AllowedModels...),
		CreatedAt:     key.CreatedAt,
		ExpiresAt:     key.ExpiresAt,
		GroupIds:      groupIDs,
		Id:            apiopenapi.Id(strconv.Itoa(key.ID)),
		LastUsedAt:    key.LastUsedAt,
		Name:          key.Name,
		Prefix:        key.Prefix,
		RpmLimit:      key.RPMLimit,
		Scopes:        append([]string(nil), key.Scopes...),
		Status:        apiopenapi.ApiKeyStatus(key.Status),
		TpmLimit:      key.TPMLimit,
	}
}

func toAPIProvider(provider providercontract.Provider) apiopenapi.Provider {
	return apiopenapi.Provider{
		AdapterType:  apiopenapi.ProviderAdapterType(provider.AdapterType),
		Capabilities: mapToJsonObjectPtr(provider.Capabilities),
		ConfigSchema: mapToJsonObjectPtr(provider.ConfigSchema),
		CreatedAt:    provider.CreatedAt,
		DisplayName:  provider.DisplayName,
		Id:           apiopenapi.Id(strconv.Itoa(provider.ID)),
		Name:         provider.Name,
		Protocol:     apiopenapi.ProviderProtocol(provider.Protocol),
		Status:       apiopenapi.ResourceStatus(provider.Status),
	}
}

func toAPIModel(model modelcontract.Model) apiopenapi.Model {
	return apiopenapi.Model{
		CanonicalName:   model.CanonicalName,
		Capabilities:    toAPICapabilityDescriptors(model.Capabilities),
		ContextWindow:   model.ContextWindow,
		CreatedAt:       model.CreatedAt,
		DisplayName:     model.DisplayName,
		Family:          model.Family,
		Id:              apiopenapi.Id(strconv.Itoa(model.ID)),
		MaxOutputTokens: model.MaxOutputTokens,
		QualityTier:     model.QualityTier,
		Status:          apiopenapi.ResourceStatus(model.Status),
	}
}

func toAPIModelAlias(alias modelcontract.ModelAlias) apiopenapi.ModelAlias {
	return apiopenapi.ModelAlias{
		Alias:          alias.Alias,
		CreatedAt:      alias.CreatedAt,
		FallbackModels: alias.FallbackModels,
		Id:             apiopenapi.Id(strconv.Itoa(alias.ID)),
		ModelId:        apiopenapi.Id(strconv.Itoa(alias.ModelID)),
		Status:         apiopenapi.ResourceStatus(alias.Status),
		StrategyHint:   alias.StrategyHint,
	}
}

func toAPIModelProviderMapping(mapping modelcontract.ModelProviderMapping) apiopenapi.ModelProviderMapping {
	return apiopenapi.ModelProviderMapping{
		CapabilityOverride: toAPICapabilityDescriptorsPtr(mapping.CapabilityOverride),
		CreatedAt:          mapping.CreatedAt,
		Id:                 apiopenapi.Id(strconv.Itoa(mapping.ID)),
		ModelId:            apiopenapi.Id(strconv.Itoa(mapping.ModelID)),
		PricingOverride:    mapToJsonObjectPtr(mapping.PricingOverride),
		ProviderId:         apiopenapi.Id(strconv.Itoa(mapping.ProviderID)),
		Status:             apiopenapi.ResourceStatus(mapping.Status),
		UpstreamModelName:  mapping.UpstreamModelName,
	}
}

func toAPIAccount(account accountcontract.ProviderAccount) apiopenapi.ProviderAccount {
	return apiopenapi.ProviderAccount{
		CreatedAt:      account.CreatedAt,
		GroupIds:       []apiopenapi.Id{},
		Id:             apiopenapi.Id(strconv.Itoa(account.ID)),
		Metadata:       mapToJsonObjectPtr(account.Metadata),
		Name:           account.Name,
		Priority:       account.Priority,
		ProviderId:     apiopenapi.Id(strconv.Itoa(account.ProviderID)),
		RiskLevel:      account.RiskLevel,
		RuntimeClass:   apiopenapi.RuntimeClass(account.RuntimeClass),
		Status:         apiopenapi.ProviderAccountStatus(account.Status),
		UpstreamClient: account.UpstreamClient,
		Weight:         account.Weight,
	}
}

func (s *Server) apiAccount(ctx context.Context, account accountcontract.ProviderAccount) apiopenapi.ProviderAccount {
	out := toAPIAccount(account)
	groupIDs, err := s.runtime.accounts.ListGroupIDsByAccount(ctx, account.ID)
	if err == nil {
		out.GroupIds = apiIDs(groupIDs)
	}
	return out
}

func toAPIAccountGroup(group accountcontract.AccountGroup) apiopenapi.AccountGroup {
	return apiopenapi.AccountGroup{
		CreatedAt:     group.CreatedAt,
		Description:   group.Description,
		Id:            apiopenapi.Id(strconv.Itoa(group.ID)),
		ModelScope:    jsonObject(group.ModelScope),
		Name:          group.Name,
		ProviderScope: jsonObject(group.ProviderScope),
		Status:        apiopenapi.AccountGroupStatus(group.Status),
		StrategyHint:  group.StrategyHint,
	}
}

func toAPIAccountGroupMember(member accountcontract.AccountGroupMember) apiopenapi.AccountGroupMember {
	return apiopenapi.AccountGroupMember{
		AccountGroupId: apiopenapi.Id(strconv.Itoa(member.AccountGroupID)),
		AccountId:      apiopenapi.Id(strconv.Itoa(member.AccountID)),
		CreatedAt:      member.CreatedAt,
		Id:             apiopenapi.Id(strconv.Itoa(member.ID)),
	}
}

func toAPIAccountQuotaSnapshot(snapshot accountcontract.AccountQuotaSnapshot) apiopenapi.AccountQuotaSnapshot {
	return apiopenapi.AccountQuotaSnapshot{
		AccountId:      apiopenapi.Id(strconv.Itoa(snapshot.AccountID)),
		ProviderId:     apiopenapi.Id(strconv.Itoa(snapshot.ProviderID)),
		QuotaLimit:     snapshot.QuotaLimit,
		QuotaType:      snapshot.QuotaType,
		Remaining:      snapshot.Remaining,
		RemainingRatio: snapshot.RemainingRatio,
		ResetAt:        snapshot.ResetAt,
		SnapshotAt:     snapshot.SnapshotAt,
		Used:           snapshot.Used,
	}
}

func accountHealthSnapshotFromAPI(snapshot apiopenapi.AccountHealthSnapshot) accountcontract.AccountHealthSnapshot {
	accountID, _ := strconv.Atoi(string(snapshot.AccountId))
	providerID, _ := strconv.Atoi(string(snapshot.ProviderId))
	return accountcontract.AccountHealthSnapshot{
		AccountID:      accountID,
		ProviderID:     providerID,
		Status:         snapshot.Status,
		SuccessRate:    snapshot.SuccessRate,
		ErrorRate:      snapshot.ErrorRate,
		LatencyP50MS:   snapshot.LatencyP50Ms,
		LatencyP95MS:   snapshot.LatencyP95Ms,
		RateLimitCount: snapshot.RateLimitCount,
		TimeoutCount:   snapshot.TimeoutCount,
		CooldownUntil:  cloneTimePtr(snapshot.CooldownUntil),
		CircuitState:   snapshot.CircuitState,
		SnapshotAt:     snapshot.SnapshotAt,
	}
}

func accountQuotaSnapshotFromAPI(snapshot apiopenapi.AccountQuotaSnapshot) accountcontract.AccountQuotaSnapshot {
	accountID, _ := strconv.Atoi(string(snapshot.AccountId))
	providerID, _ := strconv.Atoi(string(snapshot.ProviderId))
	return accountcontract.AccountQuotaSnapshot{
		AccountID:      accountID,
		ProviderID:     providerID,
		QuotaType:      snapshot.QuotaType,
		Remaining:      snapshot.Remaining,
		Used:           snapshot.Used,
		QuotaLimit:     snapshot.QuotaLimit,
		RemainingRatio: snapshot.RemainingRatio,
		ResetAt:        cloneTimePtr(snapshot.ResetAt),
		SnapshotAt:     snapshot.SnapshotAt,
	}
}

func overlayAccountHealthSnapshot(target *apiopenapi.AccountHealthSnapshot, latest accountcontract.AccountHealthSnapshot) {
	target.Status = latest.Status
	target.SuccessRate = latest.SuccessRate
	target.ErrorRate = latest.ErrorRate
	target.LatencyP50Ms = latest.LatencyP50MS
	target.LatencyP95Ms = latest.LatencyP95MS
	target.RateLimitCount = latest.RateLimitCount
	target.TimeoutCount = latest.TimeoutCount
	target.CooldownUntil = cloneTimePtr(latest.CooldownUntil)
	target.CircuitState = latest.CircuitState
	target.SnapshotAt = latest.SnapshotAt
}

func overlayAccountQuotaOnHealth(target *apiopenapi.AccountHealthSnapshot, latest accountcontract.AccountQuotaSnapshot) {
	target.QuotaRemainingRatio = latest.RemainingRatio
	target.QuotaExhausted = latest.RemainingRatio <= 0
}

func toAPIUsageLog(log usagecontract.UsageLog) apiopenapi.UsageLog {
	return apiopenapi.UsageLog{
		AccountId:             optionalIDString(log.AccountID),
		ApiKeyId:              apiopenapi.Id(strconv.Itoa(log.APIKeyID)),
		CachedTokens:          log.CachedTokens,
		CompatibilityWarnings: nonNilStrings(log.CompatibilityWarnings),
		Cost:                  log.Cost,
		CreatedAt:             log.CreatedAt,
		Currency:              log.Currency,
		ErrorClass:            log.ErrorClass,
		Id:                    apiopenapi.Id(strconv.Itoa(log.ID)),
		InputTokens:           log.InputTokens,
		LatencyMs:             log.LatencyMS,
		Model:                 log.Model,
		OutputTokens:          log.OutputTokens,
		ProviderId:            optionalIDString(log.ProviderID),
		RequestId:             log.RequestID,
		SourceEndpoint:        log.SourceEndpoint,
		SourceProtocol:        log.SourceProtocol,
		Success:               log.Success,
		TargetProtocol:        optionalString(log.TargetProtocol),
		TotalTokens:           log.TotalTokens,
		UsageEstimated:        log.UsageEstimated,
		UserId:                apiopenapi.Id(strconv.Itoa(log.UserID)),
	}
}

func toAPIAuditLog(log auditcontract.Log) apiopenapi.AuditLog {
	return apiopenapi.AuditLog{
		Action:       log.Action,
		ActorUserId:  optionalIDString(log.ActorUserID),
		After:        jsonObject(log.After),
		Before:       jsonObject(log.Before),
		CreatedAt:    log.CreatedAt,
		Id:           apiopenapi.Id(strconv.Itoa(log.ID)),
		Ip:           log.IP,
		ResourceId:   log.ResourceID,
		ResourceType: log.ResourceType,
		TraceId:      log.TraceID,
		UserAgent:    log.UserAgent,
	}
}

func toAPIBillingLedgerEntry(entry billingcontract.LedgerEntry) apiopenapi.BillingLedgerEntry {
	return apiopenapi.BillingLedgerEntry{
		Amount:        entry.Amount,
		BalanceAfter:  entry.BalanceAfter,
		BalanceBefore: entry.BalanceBefore,
		CreatedAt:     entry.CreatedAt,
		Currency:      entry.Currency,
		Id:            apiopenapi.Id(strconv.Itoa(entry.ID)),
		Metadata:      jsonObject(entry.Metadata),
		ReferenceId:   entry.ReferenceID,
		ReferenceType: entry.ReferenceType,
		Type:          apiopenapi.BillingLedgerEntryType(entry.Type),
		UserId:        apiopenapi.Id(strconv.Itoa(entry.UserID)),
	}
}

func toAPIPaymentMethod(method paymentcontract.PaymentMethod) apiopenapi.PaymentMethod {
	return apiopenapi.PaymentMethod{
		Metadata:           jsonObject(method.Metadata),
		Method:             method.Method,
		Name:               method.Name,
		Provider:           method.Provider,
		ProviderInstanceId: apiopenapi.Id(strconv.Itoa(method.ProviderInstanceID)),
	}
}

func toAPIPaymentProviderInstance(provider paymentcontract.PaymentProviderInstance) apiopenapi.PaymentProviderInstance {
	return apiopenapi.PaymentProviderInstance{
		ConfigVersion:    provider.ConfigVersion,
		CreatedAt:        provider.CreatedAt,
		Id:               apiopenapi.Id(strconv.Itoa(provider.ID)),
		Limits:           jsonObject(provider.Limits),
		Metadata:         jsonObject(provider.Metadata),
		Name:             provider.Name,
		Provider:         provider.Provider,
		SortOrder:        provider.SortOrder,
		Status:           apiopenapi.PaymentProviderStatus(provider.Status),
		SupportedMethods: append([]string(nil), provider.SupportedMethods...),
		UpdatedAt:        provider.UpdatedAt,
	}
}

func toAPIPaymentOrder(order paymentcontract.PaymentOrder) apiopenapi.PaymentOrder {
	return apiopenapi.PaymentOrder{
		Amount:                order.Amount,
		ClosedAt:              cloneTimePtr(order.ClosedAt),
		CreatedAt:             order.CreatedAt,
		Currency:              order.Currency,
		ExpiresAt:             cloneTimePtr(order.ExpiresAt),
		Id:                    apiopenapi.Id(strconv.Itoa(order.ID)),
		Metadata:              jsonObject(order.Metadata),
		OrderNo:               order.OrderNo,
		PaidAt:                cloneTimePtr(order.PaidAt),
		ProductId:             order.ProductID,
		ProductType:           apiopenapi.PaymentProductType(order.ProductType),
		ProviderInstanceId:    apiopenapi.Id(strconv.Itoa(order.ProviderInstanceID)),
		ProviderSnapshot:      jsonObject(order.ProviderSnapshot),
		ProviderTransactionId: cloneStringPtr(order.ProviderTransactionID),
		Status:                apiopenapi.PaymentOrderStatus(order.Status),
		UpdatedAt:             order.UpdatedAt,
		UserId:                apiopenapi.Id(strconv.Itoa(order.UserID)),
	}
}

func toAPISubscriptionPlan(plan subscriptioncontract.SubscriptionPlan) apiopenapi.SubscriptionPlan {
	return apiopenapi.SubscriptionPlan{
		CreatedAt:    plan.CreatedAt,
		Currency:     plan.Currency,
		Description:  optionalString(plan.Description),
		Entitlements: jsonObject(plan.Entitlements),
		ForSale:      plan.ForSale,
		Id:           apiopenapi.Id(strconv.Itoa(plan.ID)),
		Name:         plan.Name,
		Price:        plan.Price,
		SortOrder:    plan.SortOrder,
		Status:       apiopenapi.SubscriptionPlanStatus(plan.Status),
		UpdatedAt:    plan.UpdatedAt,
		ValidityDays: plan.ValidityDays,
	}
}

func toAPIUserSubscription(subscription subscriptioncontract.UserSubscription) apiopenapi.UserSubscription {
	return apiopenapi.UserSubscription{
		CreatedAt:            subscription.CreatedAt,
		EntitlementsSnapshot: jsonObject(subscription.EntitlementsSnapshot),
		ExpiresAt:            subscription.ExpiresAt,
		Id:                   apiopenapi.Id(strconv.Itoa(subscription.ID)),
		PlanId:               apiopenapi.Id(strconv.Itoa(subscription.PlanID)),
		SourceId:             subscription.SourceID,
		SourceType:           subscription.SourceType,
		StartsAt:             subscription.StartsAt,
		Status:               apiopenapi.UserSubscriptionStatus(subscription.Status),
		UpdatedAt:            subscription.UpdatedAt,
		UserId:               apiopenapi.Id(strconv.Itoa(subscription.UserID)),
	}
}

func toAPIPricingRule(rule subscriptioncontract.PricingRule) apiopenapi.PricingRule {
	return apiopenapi.PricingRule{
		CacheReadPricePerMillionTokens:  rule.CacheReadPricePerMillionTokens,
		CacheWritePricePerMillionTokens: rule.CacheWritePricePerMillionTokens,
		CreatedAt:                       rule.CreatedAt,
		Currency:                        rule.Currency,
		EffectiveFrom:                   cloneTimePtr(rule.EffectiveFrom),
		EffectiveTo:                     cloneTimePtr(rule.EffectiveTo),
		Id:                              apiopenapi.Id(strconv.Itoa(rule.ID)),
		InputPricePerMillionTokens:      rule.InputPricePerMillionTokens,
		ModelId:                         apiopenapi.Id(strconv.Itoa(rule.ModelID)),
		OutputPricePerMillionTokens:     rule.OutputPricePerMillionTokens,
		ProviderId:                      apiopenapi.Id(strconv.Itoa(rule.ProviderID)),
		UpdatedAt:                       rule.UpdatedAt,
	}
}

func toAPIDomainEventOutbox(event eventscontract.OutboxEvent) apiopenapi.DomainEventOutbox {
	return apiopenapi.DomainEventOutbox{
		AggregateId:    event.AggregateID,
		AggregateType:  event.AggregateType,
		AttemptCount:   event.AttemptCount,
		CausationId:    event.CausationID,
		CorrelationId:  event.CorrelationID,
		CreatedAt:      event.CreatedAt,
		EventId:        event.EventID,
		EventType:      event.EventType,
		EventVersion:   event.EventVersion,
		Id:             apiopenapi.Id(strconv.Itoa(event.ID)),
		IdempotencyKey: event.IdempotencyKey,
		LastError:      event.LastError,
		Metadata:       jsonObject(event.Metadata),
		NextRetryAt:    event.NextRetryAt,
		Payload:        jsonObject(event.Payload),
		ProducerModule: event.ProducerModule,
		PublishedAt:    event.PublishedAt,
		Status:         apiopenapi.DomainEventOutboxStatus(event.Status),
	}
}

func toAPISchedulerDecision(decision schedulercontract.Decision) apiopenapi.SchedulerDecision {
	return apiopenapi.SchedulerDecision{
		ApiKeyId:              apiopenapi.Id(strconv.Itoa(decision.APIKeyID)),
		AttemptNo:             decision.AttemptNo,
		CacheAffinityHit:      decision.CacheAffinityHit,
		CandidateCount:        decision.CandidateCount,
		CompatibilityWarnings: nonNilStrings(decision.CompatibilityWarnings),
		CreatedAt:             decision.CreatedAt,
		Currency:              decision.Currency,
		EstimatedCost:         decision.EstimatedCost,
		Id:                    apiopenapi.Id(strconv.Itoa(decision.ID)),
		Model:                 decision.Model,
		RejectReasons:         jsonObject(decision.RejectReasons),
		RejectedCount:         decision.RejectedCount,
		RequestId:             decision.RequestID,
		Scores:                jsonObject(decision.Scores),
		SelectedAccountId:     optionalIDString(decision.SelectedAccountID),
		SelectedProviderId:    optionalIDString(decision.SelectedProviderID),
		SourceEndpoint:        decision.SourceEndpoint,
		SourceProtocol:        decision.SourceProtocol,
		StickyHit:             decision.StickyHit,
		Strategy:              apiopenapi.SchedulerDecisionStrategy(decision.Strategy),
		StrategyConfigHash:    decision.StrategyConfigHash,
		StrategyVersion:       decision.StrategyVersion,
		StrategyWeights:       jsonObject(decision.StrategyWeights),
		TargetProtocol:        decision.TargetProtocol,
		UserId:                apiopenapi.Id(strconv.Itoa(decision.UserID)),
	}
}

func toAPICapabilityDefinition(def capabilitiescontract.Definition) apiopenapi.CapabilityDefinition {
	return apiopenapi.CapabilityDefinition{
		Category:       def.Category,
		Description:    def.Description,
		Key:            def.Key,
		ReplacementKey: def.ReplacementKey,
		Schema:         mapToJsonObjectPtr(def.Schema),
		Status:         apiopenapi.CapabilityDefinitionStatus(def.Status),
		Version:        def.Version,
	}
}

func toAPICapabilityDescriptors(values []capabilitiescontract.Descriptor) []apiopenapi.CapabilityDescriptor {
	out := make([]apiopenapi.CapabilityDescriptor, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.CapabilityDescriptor{
			Key:      value.Key,
			Level:    apiopenapi.CapabilityDescriptorLevel(value.Level),
			Metadata: mapToJsonObjectPtr(value.Metadata),
			Status:   apiopenapi.CapabilityDescriptorStatus(value.Status),
			Version:  value.Version,
		})
	}
	return out
}

func toAPICapabilityDescriptorsPtr(values []capabilitiescontract.Descriptor) *[]apiopenapi.CapabilityDescriptor {
	if values == nil {
		return nil
	}
	out := toAPICapabilityDescriptors(values)
	return &out
}

func toCapabilityDescriptors(values *[]apiopenapi.CapabilityDescriptor) []capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(*values))
	for _, value := range *values {
		out = append(out, capabilitiescontract.Descriptor{
			Key:      value.Key,
			Level:    capabilitiescontract.DescriptorLevel(value.Level),
			Metadata: jsonObjectToMap(value.Metadata),
			Status:   capabilitiescontract.DescriptorStatus(value.Status),
			Version:  value.Version,
		})
	}
	return out
}

func toCapabilityDescriptorsPtrContract(values *[]apiopenapi.CapabilityDescriptor) *[]capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	out := toCapabilityDescriptors(values)
	return &out
}

func jsonObjectToMap(value *apiopenapi.JsonObject) map[string]any {
	if value == nil {
		return nil
	}
	return map[string]any(*value)
}

func jsonObjectValueToMap(value apiopenapi.JsonObject) map[string]any {
	if value == nil {
		return nil
	}
	return map[string]any(value)
}

func jsonObjectToMapPtr(value *apiopenapi.JsonObject) *map[string]any {
	if value == nil {
		return nil
	}
	out := jsonObjectToMap(value)
	return &out
}

func mapToJsonObjectPtr(value map[string]any) *apiopenapi.JsonObject {
	if value == nil {
		return nil
	}
	object := apiopenapi.JsonObject(value)
	return &object
}

func jsonObject(value map[string]any) apiopenapi.JsonObject {
	if value == nil {
		return apiopenapi.JsonObject{}
	}
	return apiopenapi.JsonObject(value)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func optionalIDString(value *int) *string {
	if value == nil {
		return nil
	}
	out := strconv.Itoa(*value)
	return &out
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func optionalNullableString(value *string) **string {
	if value == nil {
		return nil
	}
	return &value
}

func optionalNullableInt(value *int) **int {
	if value == nil {
		return nil
	}
	return &value
}

func addOptionalInt(target map[string]any, key string, value *int) {
	if value != nil {
		target[key] = *value
	}
}

func derefMap(value *map[string]interface{}) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(*value))
	for key, val := range *value {
		out[key] = val
	}
	return out
}

func optionalCredential(value *map[string]interface{}) *map[string]any {
	if value == nil {
		return nil
	}
	out := derefMap(value)
	return &out
}

func toProviderStatusPtr(value *apiopenapi.ResourceStatus) *providercontract.Status {
	if value == nil {
		return nil
	}
	status := providercontract.Status(*value)
	return &status
}

func toModelStatusPtr(value *apiopenapi.ResourceStatus) *modelcontract.Status {
	if value == nil {
		return nil
	}
	status := modelcontract.Status(*value)
	return &status
}

func toAccountStatusPtr(value *apiopenapi.ProviderAccountStatus) *accountcontract.Status {
	if value == nil {
		return nil
	}
	status := accountcontract.Status(*value)
	return &status
}

func toAccountGroupStatusPtr(value *apiopenapi.AccountGroupStatus) *accountcontract.GroupStatus {
	if value == nil {
		return nil
	}
	status := accountcontract.GroupStatus(*value)
	return &status
}

func toAPIKeyStatusPtr(value *apiopenapi.ApiKeyStatus) *apikeycontract.Status {
	if value == nil {
		return nil
	}
	status := apikeycontract.Status(*value)
	return &status
}

func toSubscriptionPlanStatusPtr(value *apiopenapi.SubscriptionPlanStatus) *subscriptioncontract.PlanStatus {
	if value == nil {
		return nil
	}
	status := subscriptioncontract.PlanStatus(*value)
	return &status
}

func toUserSubscriptionStatusPtr(value *apiopenapi.UserSubscriptionStatus) *subscriptioncontract.SubscriptionStatus {
	if value == nil {
		return nil
	}
	status := subscriptioncontract.SubscriptionStatus(*value)
	return &status
}

func toPaymentProviderStatusPtr(value *apiopenapi.PaymentProviderStatus) *paymentcontract.ProviderStatus {
	if value == nil {
		return nil
	}
	status := paymentcontract.ProviderStatus(*value)
	return &status
}

func toAccountRuntimeClassPtr(value *apiopenapi.RuntimeClass) *accountcontract.RuntimeClass {
	if value == nil {
		return nil
	}
	runtimeClass := accountcontract.RuntimeClass(*value)
	return &runtimeClass
}

func providerAdapterTypeString(value *apiopenapi.ProviderAdapterType) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

func providerProtocolString(value *apiopenapi.ProviderProtocol) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

func ptrString(value string) *string { return &value }

func ptrInt(value int) *int { return &value }

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func ptrFloat32(value float32) *float32 { return &value }

func ptrProviderStatus(value providercontract.Status) *providercontract.Status { return &value }

func ptrModelStatus(value modelcontract.Status) *modelcontract.Status { return &value }

func ptrAccountStatus(value accountcontract.Status) *accountcontract.Status { return &value }

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func pagination(total int) apiopenapi.Pagination {
	return apiopenapi.Pagination{Page: 1, PageSize: total, Total: total, HasNext: false}
}

func filterProviders(providers []providercontract.Provider, status, q string) []providercontract.Provider {
	status = strings.TrimSpace(status)
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]providercontract.Provider, 0, len(providers))
	for _, provider := range providers {
		if status != "" && string(provider.Status) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(provider.Name), q) && !strings.Contains(strings.ToLower(provider.DisplayName), q) {
			continue
		}
		out = append(out, provider)
	}
	return out
}

func filterModels(models []modelcontract.Model, status, q string) []modelcontract.Model {
	status = strings.TrimSpace(status)
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]modelcontract.Model, 0, len(models))
	for _, model := range models {
		if status != "" && string(model.Status) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(model.CanonicalName), q) && !strings.Contains(strings.ToLower(model.DisplayName), q) {
			continue
		}
		out = append(out, model)
	}
	return out
}

func filterAccounts(accounts []accountcontract.ProviderAccount, status, providerID string) []accountcontract.ProviderAccount {
	status = strings.TrimSpace(status)
	providerID = strings.TrimSpace(providerID)
	out := make([]accountcontract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		if status != "" && string(account.Status) != status {
			continue
		}
		if providerID != "" && strconv.Itoa(account.ProviderID) != providerID {
			continue
		}
		out = append(out, account)
	}
	return out
}

func filterUsageLogs(items []usagecontract.UsageLog, userID, model string) []usagecontract.UsageLog {
	userID = strings.TrimSpace(userID)
	model = strings.ToLower(strings.TrimSpace(model))
	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if userID != "" && strconv.Itoa(item.UserID) != userID {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(item.Model), model) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterAuditLogs(items []auditcontract.Log, action, resourceType string) []auditcontract.Log {
	action = strings.TrimSpace(action)
	resourceType = strings.TrimSpace(resourceType)
	out := make([]auditcontract.Log, 0, len(items))
	for _, item := range items {
		if action != "" && item.Action != action {
			continue
		}
		if resourceType != "" && item.ResourceType != resourceType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterBillingLedger(items []billingcontract.LedgerEntry, userID, referenceType string) []billingcontract.LedgerEntry {
	userID = strings.TrimSpace(userID)
	referenceType = strings.TrimSpace(referenceType)
	out := make([]billingcontract.LedgerEntry, 0, len(items))
	for _, item := range items {
		if userID != "" && strconv.Itoa(item.UserID) != userID {
			continue
		}
		if referenceType != "" && item.ReferenceType != referenceType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterPaymentOrders(items []paymentcontract.PaymentOrder, status string) []paymentcontract.PaymentOrder {
	status = strings.TrimSpace(status)
	if status == "" {
		return items
	}
	out := make([]paymentcontract.PaymentOrder, 0, len(items))
	for _, item := range items {
		if string(item.Status) == status {
			out = append(out, item)
		}
	}
	return out
}

func filterOutboxEvents(items []eventscontract.OutboxEvent, status, eventType string) []eventscontract.OutboxEvent {
	status = strings.TrimSpace(status)
	eventType = strings.TrimSpace(eventType)
	out := make([]eventscontract.OutboxEvent, 0, len(items))
	for _, item := range items {
		if status != "" && string(item.Status) != status {
			continue
		}
		if eventType != "" && item.EventType != eventType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterSchedulerDecisions(items []schedulercontract.Decision, requestID, model string) []schedulercontract.Decision {
	requestID = strings.TrimSpace(requestID)
	model = strings.ToLower(strings.TrimSpace(model))
	out := make([]schedulercontract.Decision, 0, len(items))
	for _, item := range items {
		if requestID != "" && item.RequestID != requestID {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(item.Model), model) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func buildSchedulerOverview(decisions []schedulercontract.Decision, usageLogs []usagecontract.UsageLog) apiopenapi.SchedulerOverview {
	selected := 0
	stickyHits := 0
	cacheHits := 0
	strategyCounts := map[string]any{}
	for _, decision := range decisions {
		if decision.SelectedAccountID != nil {
			selected++
		}
		if decision.StickyHit {
			stickyHits++
		}
		if decision.CacheAffinityHit {
			cacheHits++
		}
		key := string(decision.Strategy)
		if key == "" {
			key = "unknown"
		}
		count, _ := strategyCounts[key].(int)
		strategyCounts[key] = count + 1
	}
	return apiopenapi.SchedulerOverview{
		AverageLatencyMs:      averageLatency(usageLogs),
		CacheAffinityHitCount: cacheHits,
		FailedDecisions:       len(decisions) - selected,
		SelectedDecisions:     selected,
		StickyHitCount:        stickyHits,
		StrategyCounts:        apiopenapi.JsonObject(strategyCounts),
		SuccessRate:           usageSuccessRate(usageLogs),
		TotalDecisions:        len(decisions),
	}
}

func buildAccountHealthSnapshot(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) apiopenapi.AccountHealthSnapshot {
	status := accountHealthStatus(account, logs)
	successRate := usageSuccessRate(logs)
	quotaRemainingRatio := accountQuotaRemainingRatio(account)
	return apiopenapi.AccountHealthSnapshot{
		AccountId:           apiopenapi.Id(strconv.Itoa(account.ID)),
		CircuitState:        accountCircuitState(account),
		CooldownReason:      nullableMetadataString(account.Metadata, "cooldown_reason"),
		CooldownUntil:       metadataOptionalTime(account.Metadata, "cooldown_until"),
		ErrorClass:          accountHealthErrorClass(account, logs),
		ErrorRate:           1 - successRate,
		LatencyP50Ms:        latencyPercentile(logs, 50),
		LatencyP95Ms:        latencyPercentile(logs, 95),
		ProviderId:          apiopenapi.Id(strconv.Itoa(account.ProviderID)),
		QuotaExhausted:      accountQuotaExhausted(account, quotaRemainingRatio),
		QuotaRemainingRatio: quotaRemainingRatio,
		RateLimitCount:      errorClassCount(logs, "rate_limit"),
		RuntimeClass:        apiopenapi.RuntimeClass(account.RuntimeClass),
		SnapshotAt:          now,
		Status:              status,
		SuccessRate:         successRate,
		TimeoutCount:        errorClassCount(logs, "timeout"),
	}
}

func buildAccountQuotaSnapshot(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) apiopenapi.AccountQuotaSnapshot {
	usedTokens := 0
	for _, log := range logs {
		usedTokens += log.TotalTokens
	}
	return apiopenapi.AccountQuotaSnapshot{
		AccountId:      apiopenapi.Id(strconv.Itoa(account.ID)),
		ProviderId:     apiopenapi.Id(strconv.Itoa(account.ProviderID)),
		QuotaLimit:     "unlimited",
		QuotaType:      "monthly_tokens",
		Remaining:      "unlimited",
		RemainingRatio: 1,
		SnapshotAt:     now,
		Used:           strconv.Itoa(usedTokens),
	}
}

func usageLogsForAccount(logs []usagecontract.UsageLog, accountID int) []usagecontract.UsageLog {
	out := make([]usagecontract.UsageLog, 0, len(logs))
	for _, log := range logs {
		if log.AccountID != nil && *log.AccountID == accountID {
			out = append(out, log)
		}
	}
	return out
}

func usageSuccessRate(logs []usagecontract.UsageLog) float32 {
	if len(logs) == 0 {
		return 1
	}
	success := 0
	for _, log := range logs {
		if log.Success {
			success++
		}
	}
	return float32(success) / float32(len(logs))
}

func averageLatency(logs []usagecontract.UsageLog) int {
	if len(logs) == 0 {
		return 0
	}
	total := 0
	for _, log := range logs {
		total += log.LatencyMS
	}
	return total / len(logs)
}

func latencyPercentile(logs []usagecontract.UsageLog, percentile int) int {
	if len(logs) == 0 {
		return 0
	}
	values := make([]int, 0, len(logs))
	for _, log := range logs {
		values = append(values, log.LatencyMS)
	}
	sort.Ints(values)
	index := (len(values)*percentile + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(values) {
		index = len(values)
	}
	return values[index-1]
}

func errorClassCount(logs []usagecontract.UsageLog, errorClass string) int {
	count := 0
	for _, log := range logs {
		if log.ErrorClass != nil && *log.ErrorClass == errorClass {
			count++
		}
	}
	return count
}

func accountHealthErrorClass(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog) *string {
	if value := nullableMetadataString(account.Metadata, "last_error_class"); value != nil {
		return value
	}
	for i := len(logs) - 1; i >= 0; i-- {
		if logs[i].ErrorClass != nil && strings.TrimSpace(*logs[i].ErrorClass) != "" {
			return ptrStringValue(strings.TrimSpace(*logs[i].ErrorClass))
		}
	}
	return nil
}

func accountQuotaRemainingRatio(account accountcontract.ProviderAccount) float32 {
	if value := metadataOptionalFloat(account.Metadata, "remaining_ratio", "quota_remaining_ratio"); value != nil {
		if *value < 0 {
			return 0
		}
		if *value > 1 {
			return 1
		}
		return float32(*value)
	}
	if metadataBool(account.Metadata, "quota_exhausted") {
		return 0
	}
	return 1
}

func accountQuotaExhausted(account accountcontract.ProviderAccount, remainingRatio float32) bool {
	return metadataBool(account.Metadata, "quota_exhausted") || remainingRatio <= 0
}

func nullableMetadataString(metadata map[string]any, key string) *string {
	value := metadataString(metadata, key)
	if value == "" {
		return nil
	}
	return ptrStringValue(value)
}

func metadataOptionalTime(metadata map[string]any, key string) *time.Time {
	value := metadataString(metadata, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func accountHealthStatus(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog) string {
	if account.Status != accountcontract.StatusActive {
		return string(account.Status)
	}
	if len(logs) > 0 && usageSuccessRate(logs) < 0.5 {
		return "degraded"
	}
	return "healthy"
}

func accountCircuitState(account accountcontract.ProviderAccount) string {
	if account.Status == accountcontract.StatusActive {
		return "closed"
	}
	return "open"
}

func filterCapabilityDefinitions(defs []capabilitiescontract.Definition, category, status string) []capabilitiescontract.Definition {
	category = strings.TrimSpace(category)
	status = strings.TrimSpace(status)
	out := make([]capabilitiescontract.Definition, 0, len(defs))
	for _, def := range defs {
		if category != "" && def.Category != category {
			continue
		}
		if status != "" && string(def.Status) != status {
			continue
		}
		out = append(out, def)
	}
	return out
}

func apiKeyAllowsModel(allowed []string, model string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, value := range allowed {
		if value == model {
			return true
		}
	}
	return false
}

func apiKeyAllowsModelReference(allowed []string, resolution modelcontract.ModelResolution) bool {
	if apiKeyAllowsModel(allowed, resolution.Model.CanonicalName) {
		return true
	}
	if resolution.Alias != nil && apiKeyAllowsModel(allowed, resolution.Alias.Alias) {
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func singleValueHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers)*2)
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		value := values[0]
		out[key] = value
		out[http.CanonicalHeaderKey(key)] = value
	}
	return out
}

func writeSSEJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	writeSSEJSONAny(w, payload)
	writeSSEDone(w)
}

func writeSSEEvents(w http.ResponseWriter, events []gatewayservice.StreamEvent) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for _, event := range events {
		if name := strings.TrimSpace(event.Event); name != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", name)
		}
		writeSSEJSONAny(w, event.Data)
	}
	writeSSEDone(w)
}

func writeSSEJSONAny(w http.ResponseWriter, payload any) {
	encoder := json.NewEncoder(w)
	w.Write([]byte("data: "))
	_ = encoder.Encode(payload)
	w.Write([]byte("\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func ptrStringValue(value string) *string {
	return &value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func seedCapabilities() []capabilitiescontract.Definition {
	return capabilitiescontract.DefaultDefinitions()
}
