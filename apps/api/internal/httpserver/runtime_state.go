package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

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
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	authmemory "github.com/srapi/srapi/apps/api/internal/modules/auth/store/memory"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	modelmemory "github.com/srapi/srapi/apps/api/internal/modules/models/store/memory"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
	realtimeservice "github.com/srapi/srapi/apps/api/internal/modules/realtime/service"
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
	realtime          *realtimeservice.Service
	reverseProxy      *reverseproxyservice.Service
	accounts          *accountservice.Service
	scheduler         *schedulerservice.Service
	subscriptions     *subscriptionservice.Service
	payments          *paymentservice.Service
	operations        *operationsservice.Service
	usage             *usageservice.Service
	userStore         userscontract.Store
	sessionStore      *authmemory.Store
	apiKeyStore       apikeycontract.Store
	auditStore        auditcontract.Store
	billingStore      billingcontract.Store
	eventsStore       eventscontract.Store
	operationsStore   operationscontract.Store
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
	realtimeSvc, err := realtimeservice.New(realtimeservice.Limits{
		MaxOpenSlots:       cfg.Gateway.RealtimeMaxOpenSlots,
		MaxOpenSlotsPerKey: cfg.Gateway.RealtimeMaxOpenSlotsPerKey,
	}, nil)
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

	operationsStore := opts.operations
	if operationsStore == nil {
		operationsStore = operationsmemory.NewWithUsageStore(usageStore)
	}
	operationsSvc, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
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
		realtime:          realtimeSvc,
		reverseProxy:      reverseProxySvc,
		accounts:          accountsSvc,
		scheduler:         schedulerSvc,
		subscriptions:     subscriptionSvc,
		payments:          paymentsSvc,
		operations:        operationsSvc,
		usage:             usageSvc,
		userStore:         userStore,
		sessionStore:      sessionStore,
		apiKeyStore:       apiKeyStore,
		auditStore:        auditStore,
		billingStore:      billingStore,
		eventsStore:       eventsStore,
		operationsStore:   operationsStore,
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
			Name:         "openai-compatible",
			DisplayName:  "OpenAI Compatible",
			AdapterType:  "openai-compatible",
			Protocol:     "openai-compatible",
			Status:       ptrProviderStatus(providercontract.StatusActive),
			Capabilities: map[string]any{capabilitiescontract.KeyEmbeddings: true, capabilitiescontract.KeyImages: true, capabilitiescontract.KeyAudioTranscriptions: true, capabilitiescontract.KeyAudioSpeech: true, capabilitiescontract.KeyModerations: true},
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
	if provider.Capabilities[capabilitiescontract.KeyEmbeddings] != true || provider.Capabilities[capabilitiescontract.KeyImages] != true || provider.Capabilities[capabilitiescontract.KeyAudioTranscriptions] != true || provider.Capabilities[capabilitiescontract.KeyAudioSpeech] != true || provider.Capabilities[capabilitiescontract.KeyModerations] != true {
		capabilities := cloneAnyMap(provider.Capabilities)
		capabilities[capabilitiescontract.KeyEmbeddings] = true
		capabilities[capabilitiescontract.KeyImages] = true
		capabilities[capabilitiescontract.KeyAudioTranscriptions] = true
		capabilities[capabilitiescontract.KeyAudioSpeech] = true
		capabilities[capabilitiescontract.KeyModerations] = true
		if _, err := rt.providers.Update(ctx, provider.ID, providercontract.UpdateRequest{Capabilities: &capabilities}); err != nil {
			return err
		}
		provider.Capabilities = capabilities
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
