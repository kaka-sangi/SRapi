package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

const requestIDHeader = "X-Request-ID"

type Server struct {
	cfg     config.Config
	logger  *slog.Logger
	runtime *runtimeState
}

type dependencyPinger interface {
	Ping(context.Context) error
}

type Option func(*runtimeOptions)

type runtimeOptions struct {
	adminControl  admincontrolcontract.Store
	database      dependencyPinger
	redis         dependencyPinger
	users         userscontract.Store
	apiKeys       apikeycontract.Store
	providers     providercontract.Store
	models        modelcontract.Store
	accounts      accountcontract.Store
	audit         auditcontract.Store
	authSessions  authcontract.Store
	billing       billingcontract.Store
	events        eventscontract.Store
	affiliate     affiliatecontract.Store
	operations    operationscontract.Store
	payments      paymentcontract.Store
	realtime      realtimecontract.Store
	rateLimiter   *ratelimit.Limiter
	scheduler     schedulercontract.Store
	subscriptions subscriptioncontract.Store
	usage         usagecontract.Store
}

func WithAdminControlStore(store admincontrolcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.adminControl = store
	}
}

func WithDatabasePinger(p dependencyPinger) Option {
	return func(opts *runtimeOptions) {
		opts.database = p
	}
}

func WithRedisPinger(p dependencyPinger) Option {
	return func(opts *runtimeOptions) {
		opts.redis = p
	}
}

func WithUserStore(store userscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.users = store
	}
}

func WithAPIKeyStore(store apikeycontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.apiKeys = store
	}
}

func WithProviderStore(store providercontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.providers = store
	}
}

func WithModelStore(store modelcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.models = store
	}
}

func WithAccountStore(store accountcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.accounts = store
	}
}

func WithAuditStore(store auditcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.audit = store
	}
}

func WithAuthSessionStore(store authcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.authSessions = store
	}
}

func WithBillingStore(store billingcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.billing = store
	}
}

func WithEventStore(store eventscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.events = store
	}
}

func WithAffiliateStore(store affiliatecontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.affiliate = store
	}
}

func WithOperationsStore(store operationscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.operations = store
	}
}

func WithPaymentStore(store paymentcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.payments = store
	}
}

func WithRealtimeStore(store realtimecontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.realtime = store
	}
}

func WithRateLimiter(limiter *ratelimit.Limiter) Option {
	return func(opts *runtimeOptions) {
		opts.rateLimiter = limiter
	}
}

func WithRateLimitRedis(client *redis.Client) Option {
	limiter, err := ratelimit.New(client)
	if err != nil {
		return func(*runtimeOptions) {}
	}
	return WithRateLimiter(limiter)
}

func WithSchedulerStore(store schedulercontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.scheduler = store
	}
}

func WithSubscriptionStore(store subscriptioncontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.subscriptions = store
	}
}

func WithUsageStore(store usagecontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.usage = store
	}
}

type envelope struct {
	Data      healthData `json:"data"`
	RequestID string     `json:"request_id"`
}

type healthData struct {
	Status       string            `json:"status"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

func New(cfg config.Config, logger *slog.Logger, options ...Option) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	opts := runtimeOptions{}
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	runtime, err := newRuntimeState(cfg, logger, opts)
	if err != nil {
		logger.Error("failed to initialize runtime", "error", err)
		panic(err)
	}
	server := &Server{cfg: cfg, logger: logger, runtime: runtime}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", server.handleLive)
	mux.HandleFunc("GET /readyz", server.handleReady)
	mux.HandleFunc("GET /metrics", server.handleMetrics)
	mux.HandleFunc("GET /api/v1/health", server.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", server.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", server.handleLogout)
	mux.HandleFunc("GET /api/v1/me", server.handleCurrentUser)
	mux.HandleFunc("GET /api/v1/me/balance", server.handleCurrentUserBalance)
	mux.HandleFunc("GET /api/v1/me/usage", server.handleCurrentUserUsage)
	mux.HandleFunc("GET /api/v1/me/subscriptions", server.handleCurrentUserSubscriptions)
	mux.HandleFunc("GET /api/v1/payment/methods", server.handleListPaymentMethods)
	mux.HandleFunc("POST /api/v1/payment/orders", server.handleCreatePaymentOrder)
	mux.HandleFunc("GET /api/v1/payment/orders", server.handleListPaymentOrders)
	mux.HandleFunc("GET /api/v1/payment/orders/{id}", server.handleGetPaymentOrder)
	mux.HandleFunc("POST /api/v1/payment/orders/{id}/cancel", server.handleCancelPaymentOrder)
	mux.HandleFunc("POST /api/v1/webhooks/payments/{provider}", server.handlePaymentWebhook)
	mux.HandleFunc("GET /api/v1/api-keys", server.handleListApiKeys)
	mux.HandleFunc("POST /api/v1/api-keys", server.handleCreateApiKey)
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", server.handleUpdateApiKey)
	mux.HandleFunc("GET /api/v1/admin/overview", server.handleAdminOverview)
	mux.HandleFunc("GET /api/v1/admin/dashboard", server.handleAdminDashboard)
	mux.HandleFunc("GET /api/v1/admin/dashboard/snapshot", server.handleAdminDashboardSnapshot)
	mux.HandleFunc("GET /api/v1/admin/users", server.handleListAdminUsers)
	mux.HandleFunc("POST /api/v1/admin/users", server.handleCreateAdminUser)
	mux.HandleFunc("PATCH /api/v1/admin/users/batch", server.handleBatchUpdateAdminUsers)
	mux.HandleFunc("GET /api/v1/admin/users/{id}", server.handleGetAdminUser)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}", server.handleUpdateAdminUser)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/balance", server.handleUpdateAdminUserBalance)
	mux.HandleFunc("GET /api/v1/admin/users/{id}/balance-history", server.handleAdminUserBalanceHistory)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/rpm-limit", server.handleUpdateAdminUserRpmLimit)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/disable", server.handleDisableAdminUser)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/enable", server.handleEnableAdminUser)
	mux.HandleFunc("GET /api/v1/admin/providers", server.handleListAdminProviders)
	mux.HandleFunc("POST /api/v1/admin/providers", server.handleCreateAdminProvider)
	mux.HandleFunc("POST /api/v1/admin/providers/preset/install", server.handleInstallAdminProviderPresets)
	mux.HandleFunc("PATCH /api/v1/admin/providers/{id}", server.handleUpdateAdminProvider)
	mux.HandleFunc("POST /api/v1/admin/providers/{id}/test", server.handleTestAdminProvider)
	mux.HandleFunc("GET /api/v1/admin/models", server.handleListAdminModels)
	mux.HandleFunc("POST /api/v1/admin/models", server.handleCreateAdminModel)
	mux.HandleFunc("PATCH /api/v1/admin/models/{id}", server.handleUpdateAdminModel)
	mux.HandleFunc("POST /api/v1/admin/models/{id}/aliases", server.handleCreateAdminModelAlias)
	mux.HandleFunc("POST /api/v1/admin/models/{id}/mappings", server.handleCreateAdminModelMapping)
	mux.HandleFunc("GET /api/v1/admin/accounts", server.handleListAdminAccounts)
	mux.HandleFunc("POST /api/v1/admin/accounts", server.handleCreateAdminAccount)
	mux.HandleFunc("GET /api/v1/admin/accounts/export", server.handleExportAdminAccounts)
	mux.HandleFunc("POST /api/v1/admin/accounts/import", server.handleImportAdminAccounts)
	mux.HandleFunc("PATCH /api/v1/admin/accounts/batch", server.handleBatchUpdateAdminAccounts)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}", server.handleGetAdminAccount)
	mux.HandleFunc("PATCH /api/v1/admin/accounts/{id}", server.handleUpdateAdminAccount)
	mux.HandleFunc("PATCH /api/v1/admin/accounts/{id}/proxy", server.handleBindAdminAccountProxy)
	mux.HandleFunc("GET /api/v1/admin/proxies", server.handleListAdminProxies)
	mux.HandleFunc("POST /api/v1/admin/proxies", server.handleCreateAdminProxy)
	mux.HandleFunc("PATCH /api/v1/admin/proxies/{id}", server.handleUpdateAdminProxy)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/test", server.handleTestAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/discover-models", server.handleDiscoverAdminAccountModels)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/disable", server.handleDisableAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/enable", server.handleEnableAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/recover", server.handleRecoverAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/clear-error", server.handleClearAdminAccountError)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/health", server.handleAdminAccountHealth)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/quota", server.handleAdminAccountQuota)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/rpm-status", server.handleAdminAccountRpmStatus)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/proxy-quality", server.handleAdminAccountProxyQuality)
	mux.HandleFunc("GET /api/v1/admin/account-groups", server.handleListAdminAccountGroups)
	mux.HandleFunc("POST /api/v1/admin/account-groups", server.handleCreateAdminAccountGroup)
	mux.HandleFunc("PATCH /api/v1/admin/account-groups/{id}", server.handleUpdateAdminAccountGroup)
	mux.HandleFunc("POST /api/v1/admin/account-groups/{id}/accounts/{account_id}", server.handleAddAdminAccountGroupMember)
	mux.HandleFunc("DELETE /api/v1/admin/account-groups/{id}/accounts/{account_id}", server.handleRemoveAdminAccountGroupMember)
	mux.HandleFunc("GET /api/v1/admin/usage-logs", server.handleListAdminUsageLogs)
	mux.HandleFunc("GET /api/v1/admin/usage/daily", server.handleAdminUsageDaily)
	mux.HandleFunc("GET /api/v1/admin/usage/aggregates", server.handleAdminUsageAggregates)
	mux.HandleFunc("GET /api/v1/admin/usage/export", server.handleAdminUsageExport)
	mux.HandleFunc("GET /api/v1/admin/audit-logs", server.handleListAdminAuditLogs)
	mux.HandleFunc("GET /api/v1/admin/billing-ledger", server.handleListAdminBillingLedger)
	mux.HandleFunc("GET /api/v1/admin/affiliates/invites", server.handleListAdminAffiliateInvites)
	mux.HandleFunc("GET /api/v1/admin/affiliates/rebates", server.handleListAdminAffiliateRebates)
	mux.HandleFunc("GET /api/v1/admin/affiliates/transfers", server.handleListAdminAffiliateTransfers)
	mux.HandleFunc("GET /api/v1/admin/payments/providers", server.handleListAdminPaymentProviders)
	mux.HandleFunc("POST /api/v1/admin/payments/providers", server.handleCreateAdminPaymentProvider)
	mux.HandleFunc("GET /api/v1/admin/payments/orders", server.handleListAdminPaymentOrders)
	mux.HandleFunc("POST /api/v1/admin/payments/orders/{id}/refund", server.handleRefundAdminPaymentOrder)
	mux.HandleFunc("GET /api/v1/admin/subscription-plans", server.handleListAdminSubscriptionPlans)
	mux.HandleFunc("POST /api/v1/admin/subscription-plans", server.handleCreateAdminSubscriptionPlan)
	mux.HandleFunc("GET /api/v1/admin/user-subscriptions", server.handleListAdminUserSubscriptions)
	mux.HandleFunc("POST /api/v1/admin/user-subscriptions", server.handleCreateAdminUserSubscription)
	mux.HandleFunc("GET /api/v1/admin/pricing-rules", server.handleListAdminPricingRules)
	mux.HandleFunc("POST /api/v1/admin/pricing-rules", server.handleCreateAdminPricingRule)
	mux.HandleFunc("POST /api/v1/admin/pricing-rules:bulk", server.handleBulkImportAdminPricingRules)
	mux.HandleFunc("GET /api/v1/admin/ops/events/outbox", server.handleListAdminOutboxEvents)
	mux.HandleFunc("GET /api/v1/admin/ops/overview", server.handleAdminOpsOverview)
	mux.HandleFunc("GET /api/v1/admin/ops/throughput-trend", server.handleAdminOpsThroughputTrend)
	mux.HandleFunc("GET /api/v1/admin/ops/error-trend", server.handleAdminOpsErrorTrend)
	mux.HandleFunc("GET /api/v1/admin/ops/error-distribution", server.handleAdminOpsErrorDistribution)
	mux.HandleFunc("GET /api/v1/admin/ops/latency-histogram", server.handleAdminOpsLatencyHistogram)
	mux.HandleFunc("GET /api/v1/admin/ops/concurrency", server.handleAdminOpsConcurrency)
	mux.HandleFunc("GET /api/v1/admin/ops/system-logs", server.handleListAdminOpsSystemLogs)
	mux.HandleFunc("GET /api/v1/admin/ops/alert-events", server.handleListAdminOpsAlerts)
	mux.HandleFunc("PUT /api/v1/admin/ops/settings", server.handleUpdateAdminOpsSettings)
	mux.HandleFunc("GET /api/v1/admin/ops/realtime/slots", server.handleListAdminOpsRealtimeSlots)
	mux.HandleFunc("GET /api/v1/admin/ops/slo", server.handleListAdminOpsSLOs)
	mux.HandleFunc("POST /api/v1/admin/ops/slo", server.handleCreateAdminOpsSLO)
	mux.HandleFunc("PATCH /api/v1/admin/ops/slo/{id}", server.handleUpdateAdminOpsSLO)
	mux.HandleFunc("GET /api/v1/admin/ops/alerts", server.handleListAdminOpsAlerts)
	mux.HandleFunc("POST /api/v1/admin/ops/alerts/{id}/ack", server.handleAcknowledgeAdminOpsAlert)
	mux.HandleFunc("GET /api/v1/admin/settings", server.handleGetAdminSettings)
	mux.HandleFunc("PUT /api/v1/admin/settings", server.handleUpdateAdminSettings)
	mux.HandleFunc("GET /api/v1/admin/announcements", server.handleListAdminAnnouncements)
	mux.HandleFunc("POST /api/v1/admin/announcements", server.handleCreateAdminAnnouncement)
	mux.HandleFunc("PUT /api/v1/admin/announcements/{id}", server.handleUpdateAdminAnnouncement)
	mux.HandleFunc("DELETE /api/v1/admin/announcements/{id}", server.handleDeleteAdminAnnouncement)
	mux.HandleFunc("GET /api/v1/admin/redeem-codes", server.handleListAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes", server.handleCreateAdminRedeemCode)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-generate", server.handleBatchGenerateAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-disable", server.handleBatchDisableAdminRedeemCodes)
	mux.HandleFunc("GET /api/v1/admin/redeem-codes/stats", server.handleAdminRedeemCodeStats)
	mux.HandleFunc("GET /api/v1/admin/promo-codes", server.handleListAdminPromoCodes)
	mux.HandleFunc("POST /api/v1/admin/promo-codes", server.handleCreateAdminPromoCode)
	mux.HandleFunc("PUT /api/v1/admin/promo-codes/{id}", server.handleUpdateAdminPromoCode)
	mux.HandleFunc("DELETE /api/v1/admin/promo-codes/{id}", server.handleDeleteAdminPromoCode)
	mux.HandleFunc("GET /api/v1/admin/risk-control/config", server.handleGetAdminRiskControlConfig)
	mux.HandleFunc("PUT /api/v1/admin/risk-control/config", server.handleUpdateAdminRiskControlConfig)
	mux.HandleFunc("GET /api/v1/admin/risk-control/status", server.handleGetAdminRiskControlStatus)
	mux.HandleFunc("GET /api/v1/admin/risk-control/logs", server.handleListAdminRiskControlLogs)
	mux.HandleFunc("GET /api/v1/admin/capabilities", server.handleListAdminCapabilities)
	mux.HandleFunc("GET /api/v1/admin/scheduler/overview", server.handleAdminSchedulerOverview)
	mux.HandleFunc("GET /api/v1/admin/scheduler/decisions", server.handleListAdminSchedulerDecisions)
	mux.HandleFunc("GET /api/v1/admin/scheduler/strategies", server.handleListSchedulerStrategies)
	mux.HandleFunc("POST /api/v1/admin/scheduler/simulate", server.handleSimulateSchedulerStrategy)
	mux.HandleFunc("GET /v1/models", server.handleListModels)
	mux.HandleFunc("GET /v1beta/models", server.handleListGeminiModels)
	mux.HandleFunc("POST /v1/chat/completions", server.handleCreateChatCompletion)
	mux.HandleFunc("POST /v1/responses", server.handleCreateResponse)
	mux.HandleFunc("GET /v1/responses/ws", server.handleResponsesWebSocket)
	mux.HandleFunc("GET /v1/realtime", server.handleRealtimeWebSocket)
	mux.HandleFunc("POST /v1/messages", server.handleCreateMessage)
	mux.HandleFunc("POST /v1/messages/count_tokens", server.handleAnthropicCountTokens)
	mux.HandleFunc("POST /v1/embeddings", server.handleCreateEmbedding)
	mux.HandleFunc("POST /v1/images/generations", server.handleCreateImageGeneration)
	mux.HandleFunc("POST /v1/images/edits", server.handleCreateImageEdit)
	mux.HandleFunc("POST /v1/images/variations", server.handleCreateImageVariation)
	mux.HandleFunc("POST /v1/audio/transcriptions", server.handleCreateAudioTranscription)
	mux.HandleFunc("POST /v1/audio/speech", server.handleCreateAudioSpeech)
	mux.HandleFunc("POST /v1/moderations", server.handleCreateModeration)
	mux.HandleFunc("POST /v1/rerank", server.handleCreateRerank)
	mux.HandleFunc("POST /v1beta/models/", server.handleGeminiModelAction)
	server.registerGatewayProviderAliases(mux)

	return requestIDMiddleware(server.gatewayConcurrencyMiddleware(mux))
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	writeJSON(w, http.StatusOK, envelope{
		Data: healthData{
			Status:       "ok",
			Version:      s.cfg.Server.Version,
			Dependencies: map[string]string{},
		},
		RequestID: requestID,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	dependencies := s.checkDependencies(r.Context())
	status := aggregateStatus(dependencies)
	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, envelope{
		Data: healthData{
			Status:       status,
			Version:      s.cfg.Server.Version,
			Dependencies: dependencies,
		},
		RequestID: requestID,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	writeJSONAny(w, http.StatusOK, s.runtime.healthResponse(r.Context(), requestID))
}

func (s *Server) checkDependencies(ctx context.Context) map[string]string {
	return map[string]string{
		"database": probeStatus(ctx, s.runtime.databaseProbe, s.cfg.Database.Address()),
		"redis":    probeStatus(ctx, s.runtime.redisProbe, s.cfg.Redis.Address()),
	}
}

func aggregateStatus(dependencies map[string]string) string {
	for _, status := range dependencies {
		if status != "ok" {
			return "degraded"
		}
	}
	return "ok"
}

func tcpStatus(ctx context.Context, address string) string {
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(probeCtx, "tcp", address)
	if err != nil {
		return "unavailable"
	}
	_ = conn.Close()
	return "ok"
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set(requestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) gatewayConcurrencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &gatewayConcurrencyState{}
		ctx := context.WithValue(r.Context(), gatewayConcurrencyContextKey{}, state)
		defer s.releaseGatewayConcurrency(state)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) releaseGatewayConcurrency(state *gatewayConcurrencyState) {
	if state == nil || s.runtime == nil || s.runtime.rateLimiter == nil {
		return
	}
	lease, ok := state.releaseLease()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.runtime.rateLimiter.ReleaseConcurrency(ctx, lease); err != nil {
		s.logger.Warn("failed to release gateway concurrency slot", "error", err, "lease_key", lease.Key)
	}
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type requestIDContextKey struct{}

func requestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDContextKey{}).(string)
	if !ok || requestID == "" {
		return newRequestID()
	}
	return requestID
}

type gatewayConcurrencyContextKey struct{}

type gatewayRouteContextKey struct{}

type gatewayRouteContext struct {
	ForcedProviderKey string
	SourceEndpoint    string
}

func (s *Server) withGatewayProviderAlias(providerKey string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		route := gatewayRouteContext{
			ForcedProviderKey: strings.TrimSpace(providerKey),
			SourceEndpoint:    r.URL.Path,
		}
		next(w, r.WithContext(context.WithValue(r.Context(), gatewayRouteContextKey{}, route)))
	}
}

func (s *Server) registerGatewayProviderAliases(mux *http.ServeMux) {
	seen := map[string]struct{}{}
	for _, preset := range providerpreset.Default().List() {
		for _, alias := range preset.RouteAliases {
			prefix := strings.TrimRight(alias, "/")
			if prefix == "" {
				continue
			}
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "chat/completions", s.handleCreateChatCompletion, presetSupports(preset, capabilitiescontract.KeyChatCompletions))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "responses", s.handleCreateResponse, presetSupports(preset, capabilitiescontract.KeyResponses))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "messages", s.handleCreateMessage, presetSupports(preset, capabilitiescontract.KeyMessages))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "messages/count_tokens", s.handleAnthropicCountTokens, presetSupports(preset, capabilitiescontract.KeyTokenCounting))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "embeddings", s.handleCreateEmbedding, presetSupports(preset, capabilitiescontract.KeyEmbeddings))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "images/generations", s.handleCreateImageGeneration, presetSupports(preset, capabilitiescontract.KeyImages))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "images/edits", s.handleCreateImageEdit, presetSupports(preset, capabilitiescontract.KeyImages))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "images/variations", s.handleCreateImageVariation, presetSupports(preset, capabilitiescontract.KeyImages))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "audio/transcriptions", s.handleCreateAudioTranscription, presetSupports(preset, capabilitiescontract.KeyAudioTranscriptions))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "audio/speech", s.handleCreateAudioSpeech, presetSupports(preset, capabilitiescontract.KeyAudioSpeech))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "moderations", s.handleCreateModeration, presetSupports(preset, capabilitiescontract.KeyModerations))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "rerank", s.handleCreateRerank, presetSupports(preset, capabilitiescontract.KeyRerank))
		}
		for _, alias := range preset.GeminiRouteAliases {
			prefix := strings.TrimRight(alias, "/")
			if prefix == "" {
				continue
			}
			s.registerGatewayGeminiAliasRoute(mux, seen, preset.ProviderKey, prefix)
		}
	}
}

func presetSupports(preset providerpreset.Preset, capabilityKey string) bool {
	return preset.Capabilities != nil && preset.Capabilities[capabilityKey]
}

func (s *Server) registerGatewayAliasRoute(mux *http.ServeMux, seen map[string]struct{}, providerKey, prefix, endpoint string, handler http.HandlerFunc, enabled bool) {
	if !enabled {
		return
	}
	path := prefix + "/v1/" + endpoint
	if strings.HasSuffix(prefix, "/v1") {
		path = prefix + "/" + endpoint
	}
	pattern := "POST " + path
	if _, ok := seen[pattern]; ok {
		return
	}
	seen[pattern] = struct{}{}
	mux.HandleFunc(pattern, s.withGatewayProviderAlias(providerKey, handler))
}

func (s *Server) registerGatewayGeminiAliasRoute(mux *http.ServeMux, seen map[string]struct{}, providerKey, prefix string) {
	path := prefix + "/models/"
	pattern := "POST " + path
	if _, ok := seen[pattern]; ok {
		return
	}
	seen[pattern] = struct{}{}
	mux.HandleFunc(pattern, s.withGatewayGeminiProviderAlias(providerKey, prefix))
}

func (s *Server) withGatewayGeminiProviderAlias(providerKey string, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aliasPath := r.URL.Path
		runtimePath := "/v1beta" + strings.TrimPrefix(aliasPath, prefix)
		route := gatewayRouteContext{
			ForcedProviderKey: strings.TrimSpace(providerKey),
			SourceEndpoint:    aliasPath,
		}
		cloned := r.Clone(context.WithValue(r.Context(), gatewayRouteContextKey{}, route))
		cloned.URL.Path = runtimePath
		cloned.RequestURI = runtimePath
		if r.URL.RawQuery != "" {
			cloned.RequestURI += "?" + r.URL.RawQuery
		}
		s.handleGeminiModelAction(w, cloned)
	}
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(bytes[:])
}

func Healthcheck(ctx context.Context, address string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/livez", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New(response.Status)
	}
	return nil
}
