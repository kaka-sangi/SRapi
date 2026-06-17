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
	backupsnapcontract "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	copilotconvcontract "github.com/srapi/srapi/apps/api/internal/modules/copilot/contract"
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	healthrollupscontract "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
	idempotencycontract "github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	modelratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	scheduledtestscontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	sessionaffinitycontract "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	totpcontract "github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userplatformquotascontract "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
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

// backupSnapshotsTrigger is the worker hand-off used by the admin "Snapshot
// now" handler. The backup worker satisfies it via RunOnceTriggered.
type backupSnapshotsTrigger interface {
	RunOnceTriggered(ctx context.Context, userID int) (int, error)
}

// ProxyProbeMetricsSnapshot mirrors the proxy_probe worker's MetricsSnapshot
// so /metrics can render the counters without importing the worker package
// (which would invert the dependency direction). Populate via
// WithProxyProbeMetricsProvider.
type ProxyProbeMetricsSnapshot struct {
	ProbeAttempted int
	ProbeSucceeded int
	ProbeFailed    int
}

// TokenRefreshMetricsSnapshot mirrors the accounts_token_refresh worker's
// MetricsSnapshot for the same reason as ProxyProbeMetricsSnapshot.
type TokenRefreshMetricsSnapshot struct {
	RefreshAttempted         int
	RefreshSucceeded         int
	RefreshFailedPermanent   int
	RefreshFailedTransient   int
	RefreshThresholdExceeded int
}

// ProxyProbeMetricsProvider returns the current proxy_probe counter
// snapshot. Called once per /metrics scrape.
type ProxyProbeMetricsProvider func() ProxyProbeMetricsSnapshot

// TokenRefreshMetricsProvider returns the current accounts_token_refresh
// counter snapshot. Called once per /metrics scrape.
type TokenRefreshMetricsProvider func() TokenRefreshMetricsSnapshot

// usageAggregator applies the cross-table billing aggregation (subscription
// materialized usage + API-key cost usage) for a usage_log row exactly once,
// gated by usage_log.aggregated_at. The gateway applies it eagerly off the hot
// path; a reconciler sweeps any dropped rows.
type usageAggregator interface {
	ApplyAggregation(ctx context.Context, usageLogID int) (bool, error)
}

type Option func(*runtimeOptions)

type runtimeOptions struct {
	adminControl        admincontrolcontract.Store
	database            dependencyPinger
	redis               dependencyPinger
	users               userscontract.Store
	apiKeys             apikeycontract.Store
	providers           providercontract.Store
	models              modelcontract.Store
	accounts            accountcontract.Store
	audit               auditcontract.Store
	authSessions        authcontract.Store
	backupSnapshots     backupsnapcontract.Store
	backupSnapshotsTrigger backupSnapshotsTrigger
	billing             billingcontract.Store
	events              eventscontract.Store
	affiliate           affiliatecontract.Store
	idempotency         idempotencycontract.Store
	operations          operationscontract.Store
	payments            paymentcontract.Store
	qualityEval         qualitycontract.Store
	realtime            realtimecontract.Store
	balanceReservation  balanceReservationStore
	rateLimiter         *ratelimit.Limiter
	scheduler           schedulercontract.Store
	sessionAffinity     sessionaffinitycontract.Store
	subscriptions       subscriptioncontract.Store
	totp                totpcontract.Store
	usage               usagecontract.Store
	userAttributes      userattributescontract.Store
	errorPassthrough    errorpassthroughcontract.Store
	tlsProfiles         tlsprofilescontract.Store
	healthRollups       healthrollupscontract.Store
	modelRateLimits     modelratelimitscontract.Store
	groupRateLimits     groupratelimitscontract.Store
	userPlatformQuotas  userplatformquotascontract.Store
	payloadRules        payloadrulescontract.Store
	scheduledTests      scheduledtestscontract.Store
	channelMonitors     channelmonitorscontract.Store
	copilotConvs        copilotconvcontract.ConversationStore
	metrics             *runtimeMetricsState
	backgroundDrainSink *func(context.Context)
	usageAggregator     usageAggregator
	proxyProbeMetrics   ProxyProbeMetricsProvider
	tokenRefreshMetrics TokenRefreshMetricsProvider
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

// WithBackupSnapshotsStore wires the persistent backup-snapshot history
// store. Without it the admin /backups endpoints return 503 — there's no
// in-memory fallback because the dump files only live on the API host's
// disk, which a memory-only runtime can't reach anyway.
func WithBackupSnapshotsStore(store backupsnapcontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.backupSnapshots = store
	}
}

// WithBackupSnapshotsTrigger hooks the backup worker into the "Snapshot
// now" admin action. Without it the trigger endpoint returns a 400 — the
// list/get/delete/download endpoints still work because they only touch
// the history store.
func WithBackupSnapshotsTrigger(trigger backupSnapshotsTrigger) Option {
	return func(opts *runtimeOptions) {
		opts.backupSnapshotsTrigger = trigger
	}
}

// WithProxyProbeMetricsProvider wires the proxy_probe worker's counter
// snapshot into /metrics. Optional; without it the per-worker counters
// are absent from the scrape output but everything else still works.
func WithProxyProbeMetricsProvider(provider ProxyProbeMetricsProvider) Option {
	return func(opts *runtimeOptions) {
		opts.proxyProbeMetrics = provider
	}
}

// WithTokenRefreshMetricsProvider does the same for the
// accounts_token_refresh worker.
func WithTokenRefreshMetricsProvider(provider TokenRefreshMetricsProvider) Option {
	return func(opts *runtimeOptions) {
		opts.tokenRefreshMetrics = provider
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

func WithModelRateLimitsStore(store modelratelimitscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.modelRateLimits = store
	}
}

func WithGroupRateLimitsStore(store groupratelimitscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.groupRateLimits = store
	}
}

func WithUserPlatformQuotasStore(store userplatformquotascontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.userPlatformQuotas = store
	}
}

func WithPayloadRulesStore(store payloadrulescontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.payloadRules = store
	}
}

func WithScheduledTestsStore(store scheduledtestscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.scheduledTests = store
	}
}

func WithChannelMonitorsStore(store channelmonitorscontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.channelMonitors = store
	}
}

func WithCopilotConversationStore(store copilotconvcontract.ConversationStore) Option {
	return func(opts *runtimeOptions) {
		opts.copilotConvs = store
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

// WithBalanceReservationStore wires the atomic-reservation gate. When unset
// the gateway falls back to the read-only balance check (single-instance
// gate), which still rejects requests from zero-balance users but cannot
// prevent the concurrent-overspend race.
func WithBalanceReservationStore(store balanceReservationStore) Option {
	return func(opts *runtimeOptions) {
		opts.balanceReservation = store
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

func WithSessionAffinityStore(store sessionaffinitycontract.Store) Option {
	return func(opts *runtimeOptions) {
		opts.sessionAffinity = store
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

func withRuntimeMetricsState(metrics *runtimeMetricsState) Option {
	return func(opts *runtimeOptions) {
		opts.metrics = metrics
	}
}

// WithBackgroundDrainHook captures the runtime's background-writer drain into
// sink. The application calls it during graceful shutdown — after the HTTP
// server stops accepting connections and before the database closes — so
// in-flight asynchronous usage/billing writes are flushed rather than lost.
func WithBackgroundDrainHook(sink *func(context.Context)) Option {
	return func(opts *runtimeOptions) {
		opts.backgroundDrainSink = sink
	}
}

// WithUsageAggregator wires the cross-table billing-aggregation coordinator so
// the gateway applies a usage_log row's subscription/api-key increments eagerly
// (off the hot path) under the aggregated_at idempotency marker. When unset, the
// gateway falls back to the direct (unmarked) increment path.
func WithUsageAggregator(agg usageAggregator) Option {
	return func(opts *runtimeOptions) {
		opts.usageAggregator = agg
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
	if opts.backgroundDrainSink != nil {
		*opts.backgroundDrainSink = runtime.drainUsageWriters
	}
	server := &Server{cfg: cfg, logger: logger, runtime: runtime}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", server.handleLive)
	mux.HandleFunc("GET /readyz", server.handleReady)
	mux.HandleFunc("GET /metrics", server.handleMetrics)
	mux.HandleFunc("GET /api/v1/health", server.handleHealth)
	server.registerPublicRoutes(mux)
	server.registerCurrentUserRoutes(mux)
	mux.HandleFunc("GET /api/v1/payment/methods", server.handleListPaymentMethods)
	mux.HandleFunc("POST /api/v1/payment/orders", server.withConsoleIdempotency(server.handleCreatePaymentOrder))
	mux.HandleFunc("GET /api/v1/payment/orders", server.handleListPaymentOrders)
	mux.HandleFunc("GET /api/v1/payment/orders/{id}", server.handleGetPaymentOrder)
	mux.HandleFunc("POST /api/v1/payment/orders/{id}/cancel", server.handleCancelPaymentOrder)
	mux.HandleFunc("POST /api/v1/webhooks/payments/{provider}", server.handlePaymentWebhook)
	mux.HandleFunc("GET /api/v1/webhooks/payments/{provider}", server.handlePaymentWebhookGET)
	mux.HandleFunc("GET /api/v1/api-keys", server.handleListApiKeys)
	mux.HandleFunc("POST /api/v1/api-keys", server.withConsoleIdempotency(server.handleCreateApiKey))
	mux.HandleFunc("PATCH /api/v1/api-keys/{id}", server.handleUpdateApiKey)
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", server.handleDeleteApiKey)
	mux.HandleFunc("GET /api/v1/api-keys/{id}/usage", server.handleCurrentUserApiKeyUsage)
	mux.HandleFunc("GET /api/v1/admin/overview", server.handleAdminOverview)
	mux.HandleFunc("GET /api/v1/admin/dashboard", server.handleAdminDashboard)
	mux.HandleFunc("GET /api/v1/admin/dashboard/snapshot", server.handleAdminDashboardSnapshot)
	mux.HandleFunc("GET /api/v1/admin/permission-catalog", server.handleAdminPermissionCatalog)
	mux.HandleFunc("GET /api/v1/admin/roles", server.handleListAdminRoles)
	mux.HandleFunc("POST /api/v1/admin/roles", server.handleCreateAdminRole)
	mux.HandleFunc("PATCH /api/v1/admin/roles/{id}", server.handleUpdateAdminRole)
	mux.HandleFunc("DELETE /api/v1/admin/roles/{id}", server.handleDeleteAdminRole)
	mux.HandleFunc("GET /api/v1/admin/api-keys", server.handleListAdminApiKeys)
	mux.HandleFunc("PATCH /api/v1/admin/api-keys/{id}", server.handleUpdateAdminApiKey)
	mux.HandleFunc("POST /api/v1/admin/api-keys/{id}/reset-usage", server.handleResetAdminApiKeyUsage)
	mux.HandleFunc("GET /api/v1/admin/api-keys/{id}/usage", server.handleAdminApiKeyUsage)
	mux.HandleFunc("GET /api/v1/admin/users", server.handleListAdminUsers)
	mux.HandleFunc("POST /api/v1/admin/users", server.handleCreateAdminUser)
	mux.HandleFunc("PATCH /api/v1/admin/users/batch", server.handleBatchUpdateAdminUsers)
	mux.HandleFunc("GET /api/v1/admin/users/{id}", server.handleGetAdminUser)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}", server.handleUpdateAdminUser)
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", server.handleDeleteAdminUser)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/balance", server.handleUpdateAdminUserBalance)
	mux.HandleFunc("GET /api/v1/admin/users/{id}/balance-history", server.handleAdminUserBalanceHistory)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/rpm-limit", server.handleUpdateAdminUserRpmLimit)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/disable", server.handleDisableAdminUser)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/enable", server.handleEnableAdminUser)
	server.registerCapabilityAdminRoutes(mux)
	mux.HandleFunc("GET /api/v1/admin/providers", server.handleListAdminProviders)
	mux.HandleFunc("POST /api/v1/admin/providers", server.handleCreateAdminProvider)
	mux.HandleFunc("POST /api/v1/admin/providers/preset/install", server.handleInstallAdminProviderPresets)
	mux.HandleFunc("POST /api/v1/admin/quick-setup", server.handleAdminQuickSetup)
	mux.HandleFunc("POST /api/v1/admin/models/quick-map", server.handleAdminQuickMapModels)
	mux.HandleFunc("GET /api/v1/admin/settings/captcha", server.handleGetAdminCaptchaSettings)
	mux.HandleFunc("PUT /api/v1/admin/settings/captcha", server.handleUpdateAdminCaptchaSettings)
	mux.HandleFunc("GET /api/v1/admin/providers/{id}/oauth-config", server.handleGetAdminProviderOAuthConfig)
	mux.HandleFunc("PATCH /api/v1/admin/providers/{id}", server.handleUpdateAdminProvider)
	mux.HandleFunc("DELETE /api/v1/admin/providers/{id}", server.handleDeleteAdminProvider)
	mux.HandleFunc("POST /api/v1/admin/providers/{id}/test", server.handleTestAdminProvider)
	mux.HandleFunc("GET /api/v1/admin/models", server.handleListAdminModels)
	mux.HandleFunc("POST /api/v1/admin/models", server.handleCreateAdminModel)
	mux.HandleFunc("PATCH /api/v1/admin/models/{id}", server.handleUpdateAdminModel)
	mux.HandleFunc("DELETE /api/v1/admin/models/{id}", server.handleDeleteAdminModel)
	mux.HandleFunc("GET /api/v1/admin/models/{id}/aliases", server.handleListAdminModelAliases)
	mux.HandleFunc("POST /api/v1/admin/models/{id}/aliases", server.handleCreateAdminModelAlias)
	mux.HandleFunc("PATCH /api/v1/admin/models/{id}/aliases/{aliasId}", server.handleUpdateAdminModelAlias)
	mux.HandleFunc("DELETE /api/v1/admin/models/{id}/aliases/{aliasId}", server.handleDeleteAdminModelAlias)
	mux.HandleFunc("GET /api/v1/admin/models/{id}/mappings", server.handleListAdminModelMappings)
	mux.HandleFunc("POST /api/v1/admin/models/{id}/mappings", server.handleCreateAdminModelMapping)
	mux.HandleFunc("PATCH /api/v1/admin/models/{id}/mappings/{mappingId}", server.handleUpdateAdminModelMapping)
	mux.HandleFunc("DELETE /api/v1/admin/models/{id}/mappings/{mappingId}", server.handleDeleteAdminModelMapping)
	mux.HandleFunc("GET /api/v1/admin/accounts", server.handleListAdminAccounts)
	mux.HandleFunc("POST /api/v1/admin/accounts", server.handleCreateAdminAccount)
	mux.HandleFunc("GET /api/v1/admin/accounts/export", server.handleExportAdminAccounts)
	mux.HandleFunc("POST /api/v1/admin/accounts/import", server.handleImportAdminAccounts)
	mux.HandleFunc("POST /api/v1/admin/accounts/batch", server.handleBatchCreateAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/batch-delete", server.handleBatchDeleteAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/batch-concurrency", server.handleBatchUpdateAdminAccountConcurrency)
	mux.HandleFunc("PATCH /api/v1/admin/accounts/batch", server.handleBatchUpdateAdminAccounts)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}", server.handleGetAdminAccount)
	mux.HandleFunc("PATCH /api/v1/admin/accounts/{id}", server.handleUpdateAdminAccount)
	mux.HandleFunc("DELETE /api/v1/admin/accounts/{id}", server.handleDeleteAdminAccount)
	mux.HandleFunc("PATCH /api/v1/admin/accounts/{id}/proxy", server.handleBindAdminAccountProxy)
	mux.HandleFunc("GET /api/v1/admin/backups", server.handleListAdminBackupSnapshots)
	mux.HandleFunc("POST /api/v1/admin/backups", server.handleTriggerAdminBackupSnapshot)
	mux.HandleFunc("GET /api/v1/admin/backups/{id}", server.handleGetAdminBackupSnapshot)
	mux.HandleFunc("DELETE /api/v1/admin/backups/{id}", server.handleDeleteAdminBackupSnapshot)
	mux.HandleFunc("GET /api/v1/admin/backups/{id}/download", server.handleDownloadAdminBackupSnapshot)
	mux.HandleFunc("GET /api/v1/admin/proxies", server.handleListAdminProxies)
	mux.HandleFunc("POST /api/v1/admin/proxies", server.handleCreateAdminProxy)
	mux.HandleFunc("POST /api/v1/admin/proxies/batch", server.handleBatchCreateAdminProxies)
	mux.HandleFunc("POST /api/v1/admin/proxies/batch-delete", server.handleBatchDeleteAdminProxies)
	mux.HandleFunc("PATCH /api/v1/admin/proxies/{id}", server.handleUpdateAdminProxy)
	mux.HandleFunc("DELETE /api/v1/admin/proxies/{id}", server.handleDeleteAdminProxy)
	mux.HandleFunc("POST /api/v1/admin/proxies/{id}/test", server.handleTestAdminProxy)
	mux.HandleFunc("POST /api/v1/admin/proxies/batch-test", server.handleBatchTestAdminProxies)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/test", server.handleTestAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/discover-models", server.handleDiscoverAdminAccountModels)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/disable", server.handleDisableAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/enable", server.handleEnableAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/recover", server.handleRecoverAdminAccount)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/clear-error", server.handleClearAdminAccountError)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/refresh", server.handleRefreshAdminAccount)
	mux.HandleFunc("GET /api/v1/admin/accounts/health-summary", server.handleAdminAccountsHealthSummary)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/health", server.handleAdminAccountHealth)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/quota", server.handleAdminAccountQuota)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/reset-quota", server.handleAdminAccountResetQuota)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/rpm-status", server.handleAdminAccountRpmStatus)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/proxy-quality", server.handleAdminAccountProxyQuality)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/usage-windows", server.handleGetAdminAccountUsageWindows)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/usage-daily", server.handleGetAdminAccountUsageDaily)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/usage-today", server.handleGetAdminAccountUsageToday)
	mux.HandleFunc("GET /api/v1/admin/accounts/usage-today/batch", server.handleBatchGetAdminAccountsUsageToday)
	mux.HandleFunc("GET /api/v1/admin/users/spending-today/batch", server.handleBatchGetAdminUsersSpendingToday)
	mux.HandleFunc("GET /api/v1/admin/account-groups", server.handleListAdminAccountGroups)
	mux.HandleFunc("POST /api/v1/admin/account-groups", server.handleCreateAdminAccountGroup)
	mux.HandleFunc("PATCH /api/v1/admin/account-groups/{id}", server.handleUpdateAdminAccountGroup)
	mux.HandleFunc("DELETE /api/v1/admin/account-groups/{id}", server.handleDeleteAdminAccountGroup)
	mux.HandleFunc("POST /api/v1/admin/account-groups/batch-rate-multipliers", server.handleBatchSetAdminAccountGroupRateMultipliers)
	mux.HandleFunc("POST /api/v1/admin/account-groups/batch-rpm-overrides", server.handleBatchSetAdminAccountGroupRPMOverrides)
	mux.HandleFunc("GET /api/v1/admin/account-groups/{id}/accounts", server.handleListAdminAccountGroupMembers)
	mux.HandleFunc("POST /api/v1/admin/account-groups/{id}/accounts/{account_id}", server.handleAddAdminAccountGroupMember)
	mux.HandleFunc("DELETE /api/v1/admin/account-groups/{id}/accounts/{account_id}", server.handleRemoveAdminAccountGroupMember)
	mux.HandleFunc("GET /api/v1/admin/usage-logs", server.handleListAdminUsageLogs)
	mux.HandleFunc("GET /api/v1/admin/error-logs", server.handleListAdminErrorLogs)
	mux.HandleFunc("GET /api/v1/admin/error-logs/{id}", server.handleGetAdminErrorLog)
	mux.HandleFunc("GET /api/v1/admin/ops/error-logs", server.handleListAdminOpsErrorLogs)
	mux.HandleFunc("PATCH /api/v1/admin/ops/error-logs/{id}", server.handleUpdateAdminOpsErrorLogResolution)
	mux.HandleFunc("GET /api/v1/admin/usage/daily", server.handleAdminUsageDaily)
	mux.HandleFunc("GET /api/v1/admin/usage/aggregates", server.handleAdminUsageAggregates)
	mux.HandleFunc("GET /api/v1/admin/usage/trends", server.handleGetAdminUsageTrends)
	mux.HandleFunc("GET /api/v1/admin/usage/error-distribution", server.handleGetAdminUsageErrorDistribution)
	mux.HandleFunc("GET /api/v1/admin/usage/distribution", server.handleGetAdminUsageDistribution)
	mux.HandleFunc("GET /api/v1/admin/usage/export", server.handleAdminUsageExport)
	mux.HandleFunc("GET /api/v1/admin/diagnostics/circuit-breakers", server.handleAdminCircuitBreakers)
	mux.HandleFunc("POST /api/v1/admin/diagnostics/circuit-breakers/{accountId}/reset", server.handleAdminResetCircuitBreaker)
	mux.HandleFunc("GET /api/v1/admin/diagnostics/cache-stats", server.handleAdminCacheStats)
	mux.HandleFunc("POST /api/v1/admin/diagnostics/cache/clear", server.handleAdminClearCache)
	mux.HandleFunc("GET /api/v1/admin/events", server.handleAdminEventStream)
	mux.HandleFunc("POST /api/v1/admin/accounts/sync/crs/preview", server.handleAdminCRSPreview)
	mux.HandleFunc("POST /api/v1/admin/accounts/sync/crs", server.handleAdminCRSSync)
	mux.HandleFunc("GET /api/v1/admin/audit-logs", server.handleListAdminAuditLogs)
	mux.HandleFunc("GET /api/v1/admin/billing-ledger", server.handleListAdminBillingLedger)
	mux.HandleFunc("GET /api/v1/admin/affiliates/invites", server.handleListAdminAffiliateInvites)
	mux.HandleFunc("GET /api/v1/admin/affiliates/rebates", server.handleListAdminAffiliateRebates)
	mux.HandleFunc("GET /api/v1/admin/affiliates/transfers", server.handleListAdminAffiliateTransfers)
	mux.HandleFunc("GET /api/v1/admin/affiliates/manual-adjustments", server.handleListAdminAffiliateManualAdjustments)
	mux.HandleFunc("POST /api/v1/admin/affiliates/manual-adjustments", server.handleCreateAdminAffiliateManualAdjustment)
	mux.HandleFunc("POST /api/v1/admin/affiliates/withdrawals/{id}/approve", server.handleApproveAdminAffiliateWithdrawal)
	mux.HandleFunc("POST /api/v1/admin/affiliates/withdrawals/{id}/cancel", server.handleCancelAdminAffiliateWithdrawal)
	mux.HandleFunc("GET /api/v1/admin/affiliate-rules", server.handleListAdminAffiliateRules)
	mux.HandleFunc("POST /api/v1/admin/affiliate-rules", server.handleCreateAdminAffiliateRule)
	mux.HandleFunc("PATCH /api/v1/admin/affiliate-rules/{id}", server.handleUpdateAdminAffiliateRule)
	mux.HandleFunc("GET /api/v1/admin/payments/providers", server.handleListAdminPaymentProviders)
	mux.HandleFunc("POST /api/v1/admin/payments/providers", server.handleCreateAdminPaymentProvider)
	mux.HandleFunc("PATCH /api/v1/admin/payments/providers/{id}", server.handleUpdateAdminPaymentProvider)
	mux.HandleFunc("DELETE /api/v1/admin/payments/providers/{id}", server.handleDeleteAdminPaymentProvider)
	mux.HandleFunc("POST /api/v1/admin/payments/providers/{id}/test", server.handleTestAdminPaymentProvider)
	mux.HandleFunc("GET /api/v1/admin/payments/orders", server.handleListAdminPaymentOrders)
	mux.HandleFunc("GET /api/v1/admin/payments/dashboard", server.handleGetAdminPaymentDashboard)
	mux.HandleFunc("POST /api/v1/admin/payments/orders/{id}/refund", server.handleRefundAdminPaymentOrder)
	mux.HandleFunc("GET /api/v1/admin/payment-orders/{id}/audit-logs", server.handleListAdminPaymentOrderAuditLogs)
	mux.HandleFunc("GET /api/v1/admin/subscription-plans", server.handleListAdminSubscriptionPlans)
	mux.HandleFunc("POST /api/v1/admin/subscription-plans", server.handleCreateAdminSubscriptionPlan)
	mux.HandleFunc("PATCH /api/v1/admin/subscription-plans/{id}", server.handleUpdateAdminSubscriptionPlan)
	mux.HandleFunc("DELETE /api/v1/admin/subscription-plans/{id}", server.handleDeleteAdminSubscriptionPlan)
	mux.HandleFunc("GET /api/v1/admin/user-subscriptions", server.handleListAdminUserSubscriptions)
	mux.HandleFunc("POST /api/v1/admin/user-subscriptions", server.handleCreateAdminUserSubscription)
	mux.HandleFunc("DELETE /api/v1/admin/user-subscriptions/{id}", server.handleDeleteAdminUserSubscription)
	mux.HandleFunc("GET /api/v1/admin/pricing-rules", server.handleListAdminPricingRules)
	mux.HandleFunc("POST /api/v1/admin/pricing-rules", server.handleCreateAdminPricingRule)
	mux.HandleFunc("POST /api/v1/admin/pricing-rules:bulk", server.handleBulkImportAdminPricingRules)
	mux.HandleFunc("PATCH /api/v1/admin/pricing-rules/{id}", server.handleUpdateAdminPricingRule)
	mux.HandleFunc("DELETE /api/v1/admin/pricing-rules/{id}", server.handleDeleteAdminPricingRule)
	server.registerAdminOpsRoutes(mux)
	server.registerAlertRulesRoutes(mux)
	mux.HandleFunc("GET /api/v1/admin/settings", server.handleGetAdminSettings)
	mux.HandleFunc("PUT /api/v1/admin/settings", server.handleUpdateAdminSettings)
	mux.HandleFunc("POST /api/v1/admin/settings/send-test-email", server.handleSendAdminTestEmail)
	mux.HandleFunc("GET /api/v1/admin/copilot/config", server.handleAdminCopilotConfig)
	mux.HandleFunc("POST /api/v1/admin/copilot/chat", server.handleAdminCopilotChat)
	mux.HandleFunc("GET /api/v1/admin/copilot/conversations", server.handleListAdminCopilotConversations)
	mux.HandleFunc("POST /api/v1/admin/copilot/conversations", server.handleCreateAdminCopilotConversation)
	mux.HandleFunc("GET /api/v1/admin/copilot/conversations/{id}", server.handleGetAdminCopilotConversation)
	mux.HandleFunc("PUT /api/v1/admin/copilot/conversations/{id}", server.handleUpdateAdminCopilotConversation)
	mux.HandleFunc("PATCH /api/v1/admin/copilot/conversations/{id}", server.handleRenameAdminCopilotConversation)
	mux.HandleFunc("DELETE /api/v1/admin/copilot/conversations/{id}", server.handleDeleteAdminCopilotConversation)
	mux.HandleFunc("GET /api/v1/admin/notifications/email-templates", server.handleListAdminNotificationEmailTemplates)
	mux.HandleFunc("POST /api/v1/admin/notifications/email-template-preview", server.handlePreviewAdminNotificationEmailTemplate)
	mux.HandleFunc("GET /api/v1/admin/notifications/email-templates/{event}", server.handleGetAdminNotificationEmailTemplate)
	mux.HandleFunc("PUT /api/v1/admin/notifications/email-templates/{event}", server.handleUpdateAdminNotificationEmailTemplate)
	mux.HandleFunc("POST /api/v1/admin/notifications/email-templates/{event}/restore", server.handleRestoreAdminNotificationEmailTemplate)
	server.registerAdminPromotionRoutes(mux)
	mux.HandleFunc("GET /api/v1/admin/risk-control/config", server.handleGetAdminRiskControlConfig)
	mux.HandleFunc("PUT /api/v1/admin/risk-control/config", server.handleUpdateAdminRiskControlConfig)
	mux.HandleFunc("GET /api/v1/admin/risk-control/status", server.handleGetAdminRiskControlStatus)
	mux.HandleFunc("GET /api/v1/admin/risk-control/logs", server.handleListAdminRiskControlLogs)
	mux.HandleFunc("GET /api/v1/admin/content-safety/config", server.handleGetAdminContentSafetyConfig)
	mux.HandleFunc("PUT /api/v1/admin/content-safety/config", server.handleUpdateAdminContentSafetyConfig)
	mux.HandleFunc("GET /api/v1/admin/capabilities", server.handleListAdminCapabilities)
	mux.HandleFunc("GET /api/v1/admin/scheduler/overview", server.handleAdminSchedulerOverview)
	mux.HandleFunc("GET /api/v1/admin/scheduler/decisions", server.handleListAdminSchedulerDecisions)
	mux.HandleFunc("GET /api/v1/admin/scheduler/strategies", server.handleListSchedulerStrategies)
	mux.HandleFunc("POST /api/v1/admin/scheduler/strategies", server.handleCreateSchedulerStrategy)
	mux.HandleFunc("PATCH /api/v1/admin/scheduler/strategies/{id}", server.handleUpdateSchedulerStrategy)
	mux.HandleFunc("DELETE /api/v1/admin/scheduler/strategies/{id}", server.handleDeprecateSchedulerStrategy)
	mux.HandleFunc("POST /api/v1/admin/scheduler/strategies/{id}/activate", server.handleActivateSchedulerStrategy)
	mux.HandleFunc("POST /api/v1/admin/scheduler/simulate", server.handleSimulateSchedulerStrategy)
	mux.HandleFunc("POST /api/v1/admin/scheduler/replay", server.handleReplaySchedulerStrategy)
	server.registerGatewayEndpointRoutes(mux)
	server.registerGatewayProviderAliases(mux)

	handler := server.adminRBACMiddleware(mux)
	// Admin copilot dispatches approved admin calls in-process through the same
	// RBAC gate used by external admin requests.
	runtime.internalRouter = handler
	return securityHeadersMiddleware(requestIDMiddleware(server.tracingMiddleware(server.gatewayConcurrencyMiddleware(handler))))
}

func (s *Server) registerPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/site-config", s.handleSiteConfig)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", s.handleRegister)
	mux.HandleFunc("GET /api/v1/auth/registration-attributes", s.handleRegistrationAttributes)
	mux.HandleFunc("GET /api/v1/setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /api/v1/setup", s.handleCompleteSetup)
	mux.HandleFunc("POST /api/v1/auth/password-reset/request", s.handleRequestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password-reset/confirm", s.handleConfirmPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/email-verification/request", s.handleRequestEmailVerification)
	mux.HandleFunc("POST /api/v1/auth/email-verification/confirm", s.handleConfirmEmailVerification)
	mux.HandleFunc("POST /api/v1/auth/passwordless/request", s.handleRequestPasswordlessCode)
	mux.HandleFunc("POST /api/v1/auth/passwordless/login", s.handlePasswordlessLogin)
	mux.HandleFunc("GET /api/v1/auth/oauth/providers", s.handleListOAuthProviders)
	mux.HandleFunc("GET /api/v1/auth/captcha", s.handleAuthCaptchaConfig)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", s.handleStartOAuthAuthorization)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", s.handleCompleteOAuthAuthorization)
	mux.HandleFunc("GET /api/v1/auth/oauth/pending", s.handleGetPendingOAuthSession)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/bind-current-user", s.handleBindPendingOAuthCurrentUser)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/send-verify-code", s.handleSendPendingOAuthEmailCompletion)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/email-completion/confirm", s.handleConfirmPendingOAuthEmailCompletion)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/create-account", s.handleCreatePendingOAuthAccount)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/bind-login", s.handleBindPendingOAuthLogin)
	mux.HandleFunc("POST /api/v1/auth/oauth/pending/bind-login/2fa", s.handleCompletePendingOAuthBindLoginTwoFactor)
	mux.HandleFunc("POST /api/v1/auth/login/2fa", s.handleLoginSecondFactor)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/notifications/unsubscribe", s.handlePreviewNotificationUnsubscribe)
	mux.HandleFunc("POST /api/v1/notifications/unsubscribe", s.handleNotificationUnsubscribe)
	mux.HandleFunc("GET /api/v1/subscription-plans", s.handleListPublicSubscriptionPlans)
}

// registerAdminOpsRoutes registers the operational monitoring + maintenance
// admin surfaces (overview/trends, system + usage log cleanup, realtime slots,
// SLOs, and alerts). Kept out of New so that function stays under its size cap.
func (s *Server) registerAdminOpsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/ops/events/outbox", s.handleListAdminOutboxEvents)
	mux.HandleFunc("GET /api/v1/admin/ops/overview", s.handleAdminOpsOverview)
	mux.HandleFunc("GET /api/v1/admin/ops/throughput-trend", s.handleAdminOpsThroughputTrend)
	mux.HandleFunc("GET /api/v1/admin/ops/error-trend", s.handleAdminOpsErrorTrend)
	mux.HandleFunc("GET /api/v1/admin/ops/error-distribution", s.handleAdminOpsErrorDistribution)
	mux.HandleFunc("GET /api/v1/admin/ops/latency-histogram", s.handleAdminOpsLatencyHistogram)
	mux.HandleFunc("GET /api/v1/admin/ops/concurrency", s.handleAdminOpsConcurrency)
	mux.HandleFunc("GET /api/v1/admin/ops/system-logs", s.handleListAdminOpsSystemLogs)
	mux.HandleFunc("POST /api/v1/admin/ops/system-logs/cleanup", s.handleCleanupAdminOpsSystemLogs)
	// Operator on-demand usage-record cleanup — the counterpart to the
	// background retention worker; lives here alongside the system-log cleanup.
	mux.HandleFunc("POST /api/v1/admin/usage/cleanup", s.handleCleanupAdminUsage)
	mux.HandleFunc("GET /api/v1/admin/ops/alert-events", s.handleListAdminOpsAlerts)
	mux.HandleFunc("PUT /api/v1/admin/ops/settings", s.handleUpdateAdminOpsSettings)
	mux.HandleFunc("GET /api/v1/admin/ops/realtime/slots", s.handleListAdminOpsRealtimeSlots)
	mux.HandleFunc("GET /api/v1/admin/ops/slo", s.handleListAdminOpsSLOs)
	mux.HandleFunc("POST /api/v1/admin/ops/slo", s.handleCreateAdminOpsSLO)
	mux.HandleFunc("PATCH /api/v1/admin/ops/slo/{id}", s.handleUpdateAdminOpsSLO)
	mux.HandleFunc("DELETE /api/v1/admin/ops/slo/{id}", s.handleDeleteAdminOpsSLO)
	mux.HandleFunc("GET /api/v1/admin/ops/alerts", s.handleListAdminOpsAlerts)
	mux.HandleFunc("POST /api/v1/admin/ops/alerts/{id}/ack", s.handleAcknowledgeAdminOpsAlert)
}

func (s *Server) registerAdminPromotionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/announcements", s.handleListAdminAnnouncements)
	mux.HandleFunc("POST /api/v1/admin/announcements", s.handleCreateAdminAnnouncement)
	mux.HandleFunc("PUT /api/v1/admin/announcements/{id}", s.handleUpdateAdminAnnouncement)
	mux.HandleFunc("DELETE /api/v1/admin/announcements/{id}", s.handleDeleteAdminAnnouncement)
	mux.HandleFunc("GET /api/v1/admin/announcements/{id}/reads", s.handleListAdminAnnouncementReads)
	mux.HandleFunc("GET /api/v1/admin/redeem-codes", s.handleListAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes", s.handleCreateAdminRedeemCode)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-generate", s.handleBatchGenerateAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-disable", s.handleBatchDisableAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-update", s.handleBatchUpdateAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-enable", s.handleBatchEnableAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-extend", s.handleBatchExtendAdminRedeemCodes)
	mux.HandleFunc("POST /api/v1/admin/redeem-codes/batch-delete", s.handleBatchDeleteAdminRedeemCodes)
	mux.HandleFunc("DELETE /api/v1/admin/redeem-codes/{id}", s.handleDeleteAdminRedeemCode)
	mux.HandleFunc("GET /api/v1/admin/redeem-codes/stats", s.handleAdminRedeemCodeStats)
	mux.HandleFunc("GET /api/v1/admin/promo-codes", s.handleListAdminPromoCodes)
	mux.HandleFunc("POST /api/v1/admin/promo-codes", s.handleCreateAdminPromoCode)
	mux.HandleFunc("PUT /api/v1/admin/promo-codes/{id}", s.handleUpdateAdminPromoCode)
	mux.HandleFunc("DELETE /api/v1/admin/promo-codes/{id}", s.handleDeleteAdminPromoCode)
	mux.HandleFunc("GET /api/v1/admin/promo-codes/{id}/usages", s.handleListAdminPromoCodeUsages)
}

// registerGatewayEndpointRoutes registers the OpenAI/Anthropic/Gemini-compatible
// gateway request endpoints (chat, responses, messages, embeddings, images,
// audio, etc.) that the proxy core serves under /v1 and /v1beta.
func (s *Server) registerGatewayEndpointRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/models", s.handleListModels)
	mux.HandleFunc("GET /v1/usage", s.handleGatewayUsage)
	mux.HandleFunc("GET /v1beta/models", s.handleListGeminiModels)
	mux.HandleFunc("GET /v1beta/models/", s.handleGetGeminiModel)
	mux.HandleFunc("POST /v1/chat/completions", s.withGatewayIdempotency(s.handleCreateChatCompletion))
	mux.HandleFunc("POST /v1/responses", s.withGatewayIdempotency(s.handleCreateResponse))
	mux.HandleFunc("GET /v1/responses/{response_id}/input_items", s.handleListResponseInputItems)
	mux.HandleFunc("POST /v1/responses/compact", s.withGatewaySourceEndpoint(string(gatewaycontract.EndpointResponsesCompact), s.handleCreateResponse))
	mux.HandleFunc("GET /v1/responses/ws", s.handleResponsesWebSocket)
	mux.HandleFunc("GET /v1/realtime", s.handleRealtimeWebSocket)
	mux.HandleFunc("POST /v1/messages", s.withGatewayIdempotency(s.handleCreateMessage))
	mux.HandleFunc("POST /v1/messages/count_tokens", s.handleAnthropicCountTokens)
	mux.HandleFunc("POST /v1/embeddings", s.withGatewayIdempotency(s.handleCreateEmbedding))
	mux.HandleFunc("POST /v1/images/generations", s.handleCreateImageGeneration)
	mux.HandleFunc("POST /v1/images/edits", s.handleCreateImageEdit)
	mux.HandleFunc("POST /v1/images/variations", s.handleCreateImageVariation)
	mux.HandleFunc("POST /v1/audio/transcriptions", s.handleCreateAudioTranscription)
	mux.HandleFunc("POST /v1/audio/speech", s.handleCreateAudioSpeech)
	mux.HandleFunc("POST /v1/moderations", s.handleCreateModeration)
	mux.HandleFunc("POST /v1/rerank", s.handleCreateRerank)
	mux.HandleFunc("POST /v1beta/models/", s.handleGeminiModelAction)
}

// registerCapabilityAdminRoutes registers the sub2api gap-closure admin surfaces
// (user attributes, error-passthrough rules, account availability/quota, and TLS
// fingerprint profiles).
func (s *Server) registerCapabilityAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/users/{id}/attributes", s.handleListAdminUserAttributeValues)
	mux.HandleFunc("GET /api/v1/admin/users/attributes/batch", s.handleBatchListAdminUserAttributeValues)
	mux.HandleFunc("PUT /api/v1/admin/users/{id}/attributes/{definitionId}", s.handleSetAdminUserAttributeValue)
	mux.HandleFunc("GET /api/v1/admin/user-attributes", s.handleListAdminUserAttributeDefinitions)
	mux.HandleFunc("POST /api/v1/admin/user-attributes", s.handleCreateAdminUserAttributeDefinition)
	mux.HandleFunc("PATCH /api/v1/admin/user-attributes/{id}", s.handleUpdateAdminUserAttributeDefinition)
	mux.HandleFunc("DELETE /api/v1/admin/user-attributes/{id}", s.handleDeleteAdminUserAttributeDefinition)
	mux.HandleFunc("GET /api/v1/admin/error-passthrough-rules", s.handleListAdminErrorPassthroughRules)
	mux.HandleFunc("POST /api/v1/admin/error-passthrough-rules", s.handleCreateAdminErrorPassthroughRule)
	mux.HandleFunc("PATCH /api/v1/admin/error-passthrough-rules/{id}", s.handleUpdateAdminErrorPassthroughRule)
	mux.HandleFunc("DELETE /api/v1/admin/error-passthrough-rules/{id}", s.handleDeleteAdminErrorPassthroughRule)
	mux.HandleFunc("GET /api/v1/admin/payload-rules", s.handleListAdminPayloadRules)
	mux.HandleFunc("POST /api/v1/admin/payload-rules", s.handleCreateAdminPayloadRule)
	mux.HandleFunc("PATCH /api/v1/admin/payload-rules/{id}", s.handleUpdateAdminPayloadRule)
	mux.HandleFunc("DELETE /api/v1/admin/payload-rules/{id}", s.handleDeleteAdminPayloadRule)
	mux.HandleFunc("POST /api/v1/admin/accounts/batch-action", s.handleBatchActionAdminAccounts)
	mux.HandleFunc("GET /api/v1/admin/scheduled-test-plans", s.handleListAdminScheduledTestPlans)
	mux.HandleFunc("POST /api/v1/admin/scheduled-test-plans", s.handleCreateAdminScheduledTestPlan)
	mux.HandleFunc("PATCH /api/v1/admin/scheduled-test-plans/{id}", s.handleUpdateAdminScheduledTestPlan)
	mux.HandleFunc("DELETE /api/v1/admin/scheduled-test-plans/{id}", s.handleDeleteAdminScheduledTestPlan)
	mux.HandleFunc("GET /api/v1/admin/scheduled-test-plans/{id}/runs", s.handleListAdminScheduledTestPlanRuns)
	mux.HandleFunc("POST /api/v1/admin/scheduled-test-plans/{id}/run", s.handleRunAdminScheduledTestPlan)
	mux.HandleFunc("GET /api/v1/admin/channel-monitors", s.handleListAdminChannelMonitors)
	mux.HandleFunc("POST /api/v1/admin/channel-monitors", s.handleCreateAdminChannelMonitor)
	mux.HandleFunc("PATCH /api/v1/admin/channel-monitors/{id}", s.handleUpdateAdminChannelMonitor)
	mux.HandleFunc("DELETE /api/v1/admin/channel-monitors/{id}", s.handleDeleteAdminChannelMonitor)
	mux.HandleFunc("POST /api/v1/admin/channel-monitors/{id}/run", s.handleRunAdminChannelMonitor)
	mux.HandleFunc("GET /api/v1/admin/channel-monitors/{id}/runs", s.handleListAdminChannelMonitorRuns)
	mux.HandleFunc("GET /api/v1/admin/channel-monitor-templates", s.handleListAdminChannelMonitorTemplates)
	mux.HandleFunc("POST /api/v1/admin/channel-monitor-templates", s.handleCreateAdminChannelMonitorTemplate)
	mux.HandleFunc("PATCH /api/v1/admin/channel-monitor-templates/{id}", s.handleUpdateAdminChannelMonitorTemplate)
	mux.HandleFunc("DELETE /api/v1/admin/channel-monitor-templates/{id}", s.handleDeleteAdminChannelMonitorTemplate)
	mux.HandleFunc("POST /api/v1/admin/channel-monitor-templates/{id}/apply", s.handleApplyAdminChannelMonitorTemplate)
	mux.HandleFunc("GET /api/v1/admin/accounts/availability", s.handleListAdminAccountsAvailability)
	mux.HandleFunc("GET /api/v1/admin/accounts/{id}/availability", s.handleAdminAccountAvailability)
	mux.HandleFunc("POST /api/v1/admin/accounts/{id}/quota-fetch", s.handleAdminAccountQuotaFetch)
	s.registerAdminAccountOAuthRoutes(mux)
	mux.HandleFunc("GET /api/v1/admin/tls-profiles", s.handleListAdminTLSProfiles)
	mux.HandleFunc("POST /api/v1/admin/tls-profiles", s.handleCreateAdminTLSProfile)
	mux.HandleFunc("PATCH /api/v1/admin/tls-profiles/{id}", s.handleUpdateAdminTLSProfile)
	mux.HandleFunc("DELETE /api/v1/admin/tls-profiles/{id}", s.handleDeleteAdminTLSProfile)
	mux.HandleFunc("GET /api/v1/admin/model-rate-limits", s.handleListAdminModelRateLimits)
	mux.HandleFunc("PUT /api/v1/admin/model-rate-limits", s.handleUpsertAdminModelRateLimit)
	mux.HandleFunc("DELETE /api/v1/admin/model-rate-limits/{modelId}", s.handleDeleteAdminModelRateLimit)
	mux.HandleFunc("GET /api/v1/admin/users/{id}/platform-quotas", s.handleListAdminUserPlatformQuotas)
	mux.HandleFunc("PUT /api/v1/admin/users/{id}/platform-quotas", s.handleUpsertAdminUserPlatformQuota)
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}/platform-quotas/{platform}", s.handleDeleteAdminUserPlatformQuota)
	mux.HandleFunc("GET /api/v1/admin/group-rate-limits", s.handleListAdminGroupRateLimits)
	mux.HandleFunc("PUT /api/v1/admin/group-rate-limits", s.handleUpsertAdminGroupRateLimit)
	mux.HandleFunc("DELETE /api/v1/admin/group-rate-limits/{groupId}", s.handleDeleteAdminGroupRateLimit)
	mux.HandleFunc("GET /api/v1/admin/config-snapshot", s.handleAdminConfigSnapshot)
	mux.HandleFunc("POST /api/v1/admin/config-snapshot/import", s.handleAdminConfigImport)
	mux.HandleFunc("POST /api/v1/admin/accounts/import/codex-session", s.handleImportAdminCodexSession)
}

func (s *Server) registerCurrentUserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me", s.handleCurrentUser)
	mux.HandleFunc("DELETE /api/v1/me", s.handleDeleteCurrentUser)
	mux.HandleFunc("GET /api/v1/me/auth-identities", s.handleCurrentUserAuthIdentities)
	mux.HandleFunc("DELETE /api/v1/me/auth-identities/{id}", s.handleUnbindCurrentUserAuthIdentity)
	mux.HandleFunc("PATCH /api/v1/me", s.handleUpdateCurrentUser)
	mux.HandleFunc("GET /api/v1/me/attributes", s.handleCurrentUserAttributes)
	mux.HandleFunc("PUT /api/v1/me/attributes", s.handleUpdateCurrentUserAttributes)
	mux.HandleFunc("PUT /api/v1/me/avatar", s.handleUploadCurrentUserAvatar)
	mux.HandleFunc("DELETE /api/v1/me/avatar", s.handleDeleteCurrentUserAvatar)
	mux.HandleFunc("GET /api/v1/users/{id}/avatar", s.handleGetUserAvatar)
	mux.HandleFunc("POST /api/v1/me/password", s.handleChangeCurrentUserPassword)
	mux.HandleFunc("POST /api/v1/me/sessions/revoke-all", s.handleRevokeAllCurrentUserSessions)
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
	mux.HandleFunc("GET /api/v1/me/billing-history", s.handleCurrentUserBillingHistory)
	mux.HandleFunc("GET /api/v1/me/platform-quotas", s.handleListCurrentUserPlatformQuotas)
	mux.HandleFunc("POST /api/v1/me/redeem-codes/redeem", s.withConsoleIdempotency(s.handleRedeemCurrentUserRedeemCode))
	mux.HandleFunc("GET /api/v1/me/affiliate", s.handleCurrentUserAffiliate)
	mux.HandleFunc("GET /api/v1/me/affiliate/invite-codes", s.handleListCurrentUserAffiliateInviteCodes)
	mux.HandleFunc("POST /api/v1/me/affiliate/invite-codes", s.handleCreateCurrentUserAffiliateInviteCode)
	mux.HandleFunc("GET /api/v1/me/affiliate/ledger", s.handleCurrentUserAffiliateLedger)
	mux.HandleFunc("POST /api/v1/me/affiliate/transfer-to-balance", s.handleCurrentUserAffiliateTransferToBalance)
	mux.HandleFunc("POST /api/v1/me/affiliate/withdrawals", s.handleCurrentUserAffiliateWithdrawal)
	mux.HandleFunc("GET /api/v1/me/usage", s.handleCurrentUserUsage)
	mux.HandleFunc("GET /api/v1/user/usage/dashboard/throughput", s.handleGetCurrentUserUsageThroughput)
	mux.HandleFunc("GET /api/v1/user/usage/dashboard/models", s.handleGetCurrentUserUsageModels)
	mux.HandleFunc("GET /api/v1/user/usage/dashboard/trend", s.handleGetCurrentUserUsageTrend)
	mux.HandleFunc("GET /api/v1/user/usage/dashboard/cache-metrics", s.handleGetCurrentUserUsageCacheMetrics)
	mux.HandleFunc("GET /api/v1/me/subscriptions", s.handleCurrentUserSubscriptions)
	mux.HandleFunc("GET /api/v1/me/playground/models", s.handleMePlaygroundModels)
	mux.HandleFunc("GET /api/v1/me/available-models", s.handleCurrentUserAvailableModels)
	mux.HandleFunc("POST /api/v1/me/playground/chat", s.handleMePlaygroundChat)
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
		"database": probeStatus(ctx, s.runtime.databaseProbe),
		"redis":    probeStatus(ctx, s.runtime.redisProbe),
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

// securityHeadersMiddleware sets defensive response headers on every response.
// The API only ever emits JSON/SSE (never trusted HTML with active content), so
// a deny-all CSP plus anti-sniffing/anti-framing headers are safe and block
// XSS/clickjacking on anything a proxy or browser might render from a body.
// TLS/HSTS is intentionally left to the terminating reverse proxy.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
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
	s.registerGatewayGeminiAliasRouteForMethod(mux, seen, http.MethodGet, providerKey, prefix, strings.TrimRight(prefix+"/models", "/"), s.handleListGeminiModels)
	s.registerGatewayGeminiAliasRouteForMethod(mux, seen, http.MethodGet, providerKey, prefix, path, s.handleGetGeminiModel)
	s.registerGatewayGeminiAliasRouteForMethod(mux, seen, http.MethodPost, providerKey, prefix, path, s.handleGeminiModelAction)
}

func (s *Server) registerGatewayGeminiAliasRouteForMethod(mux *http.ServeMux, seen map[string]struct{}, method, providerKey, prefix, path string, handler http.HandlerFunc) {
	pattern := strings.ToUpper(strings.TrimSpace(method)) + " " + path
	if _, ok := seen[pattern]; ok {
		return
	}
	seen[pattern] = struct{}{}
	mux.HandleFunc(pattern, s.withGatewayGeminiProviderAlias(providerKey, prefix, handler))
}

func (s *Server) withGatewayGeminiProviderAlias(providerKey string, prefix string, handler http.HandlerFunc) http.HandlerFunc {
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
		handler(w, cloned)
	}
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(bytes[:])
}

func Healthcheck(ctx context.Context, address, path string) error {
	if strings.TrimSpace(path) == "" {
		path = "/readyz"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+path, nil)
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
