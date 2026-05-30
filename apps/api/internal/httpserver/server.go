package httpserver

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	healthrollupscontract "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
	idempotencycontract "github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	totpcontract "github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userattributescontract "github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	platformlogger "github.com/srapi/srapi/apps/api/internal/platform/logger"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
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
	adminControl     admincontrolcontract.Store
	database         dependencyPinger
	redis            dependencyPinger
	users            userscontract.Store
	apiKeys          apikeycontract.Store
	providers        providercontract.Store
	models           modelcontract.Store
	accounts         accountcontract.Store
	audit            auditcontract.Store
	authSessions     authcontract.Store
	billing          billingcontract.Store
	events           eventscontract.Store
	affiliate        affiliatecontract.Store
	idempotency      idempotencycontract.Store
	operations       operationscontract.Store
	payments         paymentcontract.Store
	qualityEval      qualitycontract.Store
	realtime         realtimecontract.Store
	rateLimiter      *ratelimit.Limiter
	scheduler        schedulercontract.Store
	subscriptions    subscriptioncontract.Store
	totp             totpcontract.Store
	usage            usagecontract.Store
	userAttributes   userattributescontract.Store
	errorPassthrough errorpassthroughcontract.Store
	tlsProfiles      tlsprofilescontract.Store
	healthRollups    healthrollupscontract.Store
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

func WithIdempotencyStore(store idempotencycontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.idempotency = store
	}
}

func WithUserAttributesStore(store userattributescontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.userAttributes = store
	}
}

func WithErrorPassthroughStore(store errorpassthroughcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.errorPassthrough = store
	}
}

func WithTLSProfilesStore(store tlsprofilescontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.tlsProfiles = store
	}
}

func WithHealthRollupsStore(store healthrollupscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.healthRollups = store
	}
}

func WithPaymentStore(store paymentcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.payments = store
	}
}

func WithQualityEvalStore(store qualitycontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.qualityEval = store
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

func WithTOTPStore(store totpcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.totp = store
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
	mux.HandleFunc("POST /api/v1/auth/register", server.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/password-reset/request", server.handleRequestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password-reset/confirm", server.handleConfirmPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/email-verification/request", server.handleRequestEmailVerification)
	mux.HandleFunc("POST /api/v1/auth/email-verification/confirm", server.handleConfirmEmailVerification)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", server.handleStartOAuthAuthorization)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", server.handleCompleteOAuthAuthorization)
	mux.HandleFunc("GET /api/v1/auth/oauth/pending", server.handleGetPendingOAuthSession)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/bind-current-user", server.handleBindPendingOAuthCurrentUser)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/send-verify-code", server.handleSendPendingOAuthEmailCompletion)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/email-completion/confirm", server.handleConfirmPendingOAuthEmailCompletion)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/create-account", server.handleCreatePendingOAuthAccount)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/bind-login", server.handleBindPendingOAuthLogin)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/bind-login/2fa", server.handleCompletePendingOAuthBindLoginTwoFactor)
	mux.HandleFunc("POST /api/v1/auth/login/2fa", server.handleLoginSecondFactor)
	mux.HandleFunc("POST /api/v1/auth/logout", server.handleLogout)
	mux.HandleFunc("GET /api/v1/notifications/unsubscribe", server.handlePreviewNotificationUnsubscribe)
	mux.HandleFunc("POST /api/v1/notifications/unsubscribe", server.handleNotificationUnsubscribe)
	server.registerCurrentUserRoutes(mux)
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
	mux.HandleFunc("GET /api/v1/admin/roles", server.handleListAdminRoles)
	mux.HandleFunc("POST /api/v1/admin/roles", server.handleCreateAdminRole)
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
	server.registerCapabilityAdminRoutes(mux)
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
	mux.HandleFunc("PATCH /api/v1/admin/payments/providers/{id}", server.handleUpdateAdminPaymentProvider)
	mux.HandleFunc("POST /api/v1/admin/payments/providers/{id}/test", server.handleTestAdminPaymentProvider)
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
	mux.HandleFunc("POST /api/v1/admin/ops/system-logs/cleanup", server.handleCleanupAdminOpsSystemLogs)
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
	mux.HandleFunc("GET /api/v1/admin/notifications/email-templates", server.handleListAdminNotificationEmailTemplates)
	mux.HandleFunc("POST /api/v1/admin/notifications/email-template-preview", server.handlePreviewAdminNotificationEmailTemplate)
	mux.HandleFunc("GET /api/v1/admin/notifications/email-templates/{event}", server.handleGetAdminNotificationEmailTemplate)
	mux.HandleFunc("PUT /api/v1/admin/notifications/email-templates/{event}", server.handleUpdateAdminNotificationEmailTemplate)
	mux.HandleFunc("POST /api/v1/admin/notifications/email-templates/{event}/restore", server.handleRestoreAdminNotificationEmailTemplate)
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
	mux.HandleFunc("POST /api/v1/admin/scheduler/replay", server.handleReplaySchedulerStrategy)
	mux.HandleFunc("GET /v1/models", server.handleListModels)
	mux.HandleFunc("GET /v1/usage", server.handleGatewayUsage)
	mux.HandleFunc("GET /v1beta/models", server.handleListGeminiModels)
	mux.HandleFunc("GET /v1beta/models/", server.handleGetGeminiModel)
	mux.HandleFunc("POST /v1/chat/completions", server.withGatewayIdempotency(server.handleCreateChatCompletion))
	mux.HandleFunc("POST /v1/responses", server.withGatewayIdempotency(server.handleCreateResponse))
	mux.HandleFunc("GET /v1/responses/{response_id}/input_items", server.handleListResponseInputItems)
	mux.HandleFunc("POST /v1/responses/compact", server.withGatewaySourceEndpoint(string(gatewaycontract.EndpointResponsesCompact), server.handleCreateResponse))
	mux.HandleFunc("GET /v1/responses/ws", server.handleResponsesWebSocket)
	mux.HandleFunc("GET /v1/realtime", server.handleRealtimeWebSocket)
	mux.HandleFunc("POST /v1/messages", server.withGatewayIdempotency(server.handleCreateMessage))
	mux.HandleFunc("POST /v1/messages/count_tokens", server.handleAnthropicCountTokens)
	mux.HandleFunc("POST /v1/embeddings", server.withGatewayIdempotency(server.handleCreateEmbedding))
	mux.HandleFunc("POST /v1/images/generations", server.handleCreateImageGeneration)
	mux.HandleFunc("POST /v1/images/edits", server.handleCreateImageEdit)
	mux.HandleFunc("POST /v1/images/variations", server.handleCreateImageVariation)
	mux.HandleFunc("POST /v1/audio/transcriptions", server.handleCreateAudioTranscription)
	mux.HandleFunc("POST /v1/audio/speech", server.handleCreateAudioSpeech)
	mux.HandleFunc("POST /v1/moderations", server.handleCreateModeration)
	mux.HandleFunc("POST /v1/rerank", server.handleCreateRerank)
	mux.HandleFunc("POST /v1beta/models/", server.handleGeminiModelAction)
	server.registerGatewayProviderAliases(mux)

	return requestIDMiddleware(server.tracingMiddleware(server.gatewayConcurrencyMiddleware(mux)))
}

// registerCapabilityAdminRoutes registers the sub2api gap-closure admin surfaces
// (user attributes, error-passthrough rules, account availability/quota, and TLS
// fingerprint profiles).
func (s *Server) registerCapabilityAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/users/{id}/attributes", s.handleListAdminUserAttributeValues)
	mux.HandleFunc("PUT /api/v1/admin/users/{id}/attributes/{definitionId}", s.handleSetAdminUserAttributeValue)
	mux.HandleFunc("GET /api/v1/admin/user-attributes", s.handleListAdminUserAttributeDefinitions)
	mux.HandleFunc("POST /api/v1/admin/user-attributes", s.handleCreateAdminUserAttributeDefinition)
	mux.HandleFunc("PATCH /api/v1/admin/user-attributes/{id}", s.handleUpdateAdminUserAttributeDefinition)
	mux.HandleFunc("DELETE /api/v1/admin/user-attributes/{id}", s.handleDeleteAdminUserAttributeDefinition)
	mux.HandleFunc("GET /api/v1/admin/error-passthrough-rules", s.handleListAdminErrorPassthroughRules)
	mux.HandleFunc("POST /api/v1/admin/error-passthrough-rules", s.handleCreateAdminErrorPassthroughRule)
	mux.HandleFunc("PATCH /api/v1/admin/error-passthrough-rules/{id}", s.handleUpdateAdminErrorPassthroughRule)
	mux.HandleFunc("DELETE /api/v1/admin/error-passthrough-rules/{id}", s.handleDeleteAdminErrorPassthroughRule)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/availability", s.handleAdminAccountAvailability)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/quota-fetch", s.handleAdminAccountQuotaFetch)
	mux.HandleFunc("GET /api/v1/admin/tls-profiles", s.handleListAdminTLSProfiles)
	mux.HandleFunc("POST /api/v1/admin/tls-profiles", s.handleCreateAdminTLSProfile)
	mux.HandleFunc("PATCH /api/v1/admin/tls-profiles/{id}", s.handleUpdateAdminTLSProfile)
	mux.HandleFunc("DELETE /api/v1/admin/tls-profiles/{id}", s.handleDeleteAdminTLSProfile)
}

func (s *Server) registerCurrentUserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me", s.handleCurrentUser)
	mux.HandleFunc("GET /api/v1/me/auth-identities", s.handleCurrentUserAuthIdentities)
	mux.HandleFunc("DELETE /api/v1/me/auth-identities/{id}", s.handleUnbindCurrentUserAuthIdentity)
	mux.HandleFunc("PATCH /api/v1/me", s.handleUpdateCurrentUser)
	mux.HandleFunc("PUT /api/v1/me/avatar", s.handleUploadCurrentUserAvatar)
	mux.HandleFunc("DELETE /api/v1/me/avatar", s.handleDeleteCurrentUserAvatar)
	mux.HandleFunc("GET /api/v1/users/{id}/avatar", s.handleGetUserAvatar)
	mux.HandleFunc("POST /api/v1/me/password", s.handleChangeCurrentUserPassword)
	mux.HandleFunc("GET /api/v1/me/notification-contacts", s.handleCurrentUserNotificationContacts)
	mux.HandleFunc("POST /api/v1/me/notification-contacts", s.handleRequestCurrentUserNotificationContactVerification)
	mux.HandleFunc("POST /api/v1/me/notification-contacts/verify", s.handleConfirmCurrentUserNotificationContactVerification)
	mux.HandleFunc("PATCH /api/v1/me/notification-contacts/{id}", s.handleUpdateCurrentUserNotificationContact)
	mux.HandleFunc("DELETE /api/v1/me/notification-contacts/{id}", s.handleDeleteCurrentUserNotificationContact)
	mux.HandleFunc("GET /api/v1/me/notification-preferences", s.handleCurrentUserNotificationPreferences)
	mux.HandleFunc("PUT /api/v1/me/notification-preferences", s.handleUpdateCurrentUserNotificationPreferences)
	mux.HandleFunc("GET /api/v1/me/announcements", s.handleCurrentUserAnnouncements)
	mux.HandleFunc("POST /api/v1/me/announcements/{id}/read", s.handleMarkCurrentUserAnnouncementRead)
	mux.HandleFunc("GET /api/v1/me/totp/status", s.handleCurrentUserTOTPStatus)
	mux.HandleFunc("POST /api/v1/me/totp/setup", s.handleCurrentUserTOTPSetup)
	mux.HandleFunc("POST /api/v1/me/totp/enable", s.handleCurrentUserTOTPEnable)
	mux.HandleFunc("POST /api/v1/me/totp/disable", s.handleCurrentUserTOTPDisable)
	mux.HandleFunc("GET /api/v1/me/balance", s.handleCurrentUserBalance)
	mux.HandleFunc("POST /api/v1/me/redeem-codes/redeem", s.handleRedeemCurrentUserRedeemCode)
	mux.HandleFunc("GET /api/v1/me/affiliate", s.handleCurrentUserAffiliate)
	mux.HandleFunc("GET /api/v1/me/affiliate/ledger", s.handleCurrentUserAffiliateLedger)
	mux.HandleFunc("POST /api/v1/me/affiliate/transfer-to-balance", s.handleCurrentUserAffiliateTransferToBalance)
	mux.HandleFunc("GET /api/v1/me/usage", s.handleCurrentUserUsage)
	mux.HandleFunc("GET /api/v1/me/subscriptions", s.handleCurrentUserSubscriptions)
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
		ctx = platformlogger.WithRequestID(ctx, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) tracingMiddleware(next http.Handler) http.Handler {
	tracer := otel.Tracer("github.com/srapi/srapi/apps/api/internal/httpserver")
	propagator := otel.GetTextMapPropagator()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		requestID := requestIDFromContext(ctx)
		route := normalizedHTTPPath(r.URL.Path)
		ctx, span := tracer.Start(ctx,
			r.Method+" "+route,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("url.path", r.URL.Path),
				attribute.String("http.route", route),
				attribute.String("srapi.request_id", requestID),
			),
		)
		if traceID := span.SpanContext().TraceID(); traceID.IsValid() {
			ctx = platformlogger.WithTraceID(ctx, traceID.String())
		}

		recorder := &traceResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r.WithContext(ctx))
		span.SetAttributes(
			attribute.Int("http.response.status_code", recorder.status),
			attribute.Int64("http.response.body.size", recorder.bytes),
		)
		if recorder.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(recorder.status))
		}
		span.End()
	})
}

type traceResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *traceResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *traceResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	written, err := w.ResponseWriter.Write(data)
	w.bytes += int64(written)
	return written, err
}

func (w *traceResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *traceResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *traceResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *traceResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		read, err := readerFrom.ReadFrom(reader)
		w.bytes += read
		return read, err
	}
	written, err := io.Copy(w.ResponseWriter, reader)
	w.bytes += written
	return written, err
}

func (w *traceResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func normalizedHTTPPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for idx, part := range parts {
		if isPathID(part) {
			parts[idx] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func isPathID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
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

func (s *Server) withGatewaySourceEndpoint(sourceEndpoint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		route := gatewayRouteContext{SourceEndpoint: strings.TrimSpace(sourceEndpoint)}
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
			s.registerGatewayAliasRouteForMethod(mux, seen, http.MethodGet, preset.ProviderKey, prefix, "responses/{response_id}/input_items", s.handleListResponseInputItems, presetSupports(preset, capabilitiescontract.KeyResponses))
			s.registerGatewayAliasRoute(mux, seen, preset.ProviderKey, prefix, "responses/compact", s.handleCreateResponse, presetSupports(preset, capabilitiescontract.KeyResponsesCompact))
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
	s.registerGatewayAliasRouteForMethod(mux, seen, http.MethodPost, providerKey, prefix, endpoint, handler, enabled)
}

func (s *Server) registerGatewayAliasRouteForMethod(mux *http.ServeMux, seen map[string]struct{}, method, providerKey, prefix, endpoint string, handler http.HandlerFunc, enabled bool) {
	if !enabled {
		return
	}
	path := prefix + "/v1/" + endpoint
	if strings.HasSuffix(prefix, "/v1") {
		path = prefix + "/" + endpoint
	}
	pattern := strings.ToUpper(strings.TrimSpace(method)) + " " + path
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
