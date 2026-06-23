package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountprovisioningservice "github.com/srapi/srapi/apps/api/internal/modules/account_provisioning/service"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	affiliatememory "github.com/srapi/srapi/apps/api/internal/modules/affiliate/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apikeymemory "github.com/srapi/srapi/apps/api/internal/modules/api_keys/store/memory"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authservice "github.com/srapi/srapi/apps/api/internal/modules/auth/service"
	authmemory "github.com/srapi/srapi/apps/api/internal/modules/auth/store/memory"
	backupsnapcontract "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
	backupsnapservice "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/service"
	backupsnapmemory "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/store/memory"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	captchaservice "github.com/srapi/srapi/apps/api/internal/modules/captcha/service"
	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	channelmonitorsservice "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/service"
	channelmonitorsmemory "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/store/memory"
	concurrencyslotsservice "github.com/srapi/srapi/apps/api/internal/modules/concurrency_slots/service"
	contentsafetyservice "github.com/srapi/srapi/apps/api/internal/modules/content_safety/service"
	"github.com/srapi/srapi/apps/api/internal/modules/copilot"
	copilotconvcontract "github.com/srapi/srapi/apps/api/internal/modules/copilot/contract"
	copilotconvmemory "github.com/srapi/srapi/apps/api/internal/modules/copilot/store/memory"
	erroreventstreamservice "github.com/srapi/srapi/apps/api/internal/modules/error_event_stream/service"
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	errorpassthroughservice "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/service"
	errorpassthroughmemory "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/store/memory"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	groupratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/service"
	groupratelimitsmemory "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/store/memory"
	healthrollupscontract "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
	healthrollupsservice "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/service"
	healthrollupsmemory "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/store/memory"
	idempotencyservice "github.com/srapi/srapi/apps/api/internal/modules/idempotency/service"
	idempotencymemory "github.com/srapi/srapi/apps/api/internal/modules/idempotency/store/memory"
	modelratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	modelratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/service"
	modelratelimitsmemory "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/store/memory"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	modelmemory "github.com/srapi/srapi/apps/api/internal/modules/models/store/memory"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	payloadrulesservice "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/service"
	payloadrulesmemory "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/store/memory"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	qualityservice "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/service"
	qualitymemory "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/store/memory"
	ratelimitcooldownservice "github.com/srapi/srapi/apps/api/internal/modules/rate_limit_cooldown/service"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	realtimeservice "github.com/srapi/srapi/apps/api/internal/modules/realtime/service"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
	scheduledtestscontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledtestsservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
	scheduledtestsmemory "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/store/memory"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	sessionaffinitycontract "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
	sessionaffinitymemory "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	tlsprofilesservice "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/service"
	tlsprofilesmemory "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/store/memory"
	totpcontract "github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
	totpservice "github.com/srapi/srapi/apps/api/internal/modules/totp/service"
	totpmemory "github.com/srapi/srapi/apps/api/internal/modules/totp/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	userplatformquotascontract "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
	userplatformquotasservice "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/service"
	userplatformquotasmemory "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/store/memory"
	userattributescontract "github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
	userattributesservice "github.com/srapi/srapi/apps/api/internal/modules/userattributes/service"
	userattributesmemory "github.com/srapi/srapi/apps/api/internal/modules/userattributes/store/memory"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersservice "github.com/srapi/srapi/apps/api/internal/modules/users/service"
	usermemory "github.com/srapi/srapi/apps/api/internal/modules/users/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/platform/circuitbreaker"
	"github.com/srapi/srapi/apps/api/internal/platform/eventsub"
	"github.com/srapi/srapi/apps/api/internal/platform/localcache"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
	scheduledtestworker "github.com/srapi/srapi/apps/api/internal/workers/scheduled_test"
)

const (
	sessionCookieName         = "srapi_session"
	oauthFlowCookieName       = "srapi_oauth_flow"
	oauthFlowCookiePath       = "/api/v1/auth/oauth"
	oauthPendingCookieName    = "srapi_oauth_pending"
	oauthPendingCookiePath    = "/api/v1/auth/oauth/pending"
	csrfHeaderName            = "X-CSRF-Token"
	rateLimitCooldownWindow   = 30 * time.Second
	overloadCooldownWindow    = 10 * time.Minute
	authFailureCooldownWindow = 10 * time.Minute
)

var errRequestTooLarge = errors.New("request body too large")

type runtimeState struct {
	cfg                  config.Config
	logger               *slog.Logger
	setupComplete        atomic.Bool
	users                *usersservice.Service
	auth                 *authservice.Service
	apiKeys              *apikeyservice.Service
	audit                *auditservice.Service
	billing              *billingservice.Service
	events               *eventsservice.Service
	affiliate            *affiliateservice.Service
	idempotency          *idempotencyservice.Service
	notificationContacts *notificationsservice.ContactService
	userAvatars          *usersservice.AvatarService
	contentSafety        *contentsafetyservice.Service
	gateway              *gatewayservice.Service
	providers            *providerservice.Service
	models               *modelservice.Service
	adapters             *provideradapterservice.Service
	realtime             *realtimeservice.Service
	reverseProxy         *reverseproxyservice.Service
	accounts             *accountservice.Service
	adminControl         *admincontrolservice.Service
	qualityEval          *qualityservice.Service
	scheduler            *schedulerservice.Service
	subscriptions        *subscriptionservice.Service
	totp                 *totpservice.Service
	payments             *paymentservice.Service
	operations           *operationsservice.Service
	usage                *usageservice.Service
	userAttributes       *userattributesservice.Service
	errorPassthrough     *errorpassthroughservice.Service
	opsErrorLogs         *opserrorlogsservice.Service
	opsErrorLogsStore    opserrorlogscontract.Store
	opsErrorLogRecorder  *opsErrorLogRecorder
	tlsProfiles          *tlsprofilesservice.Service
	captcha              *captchaservice.Service
	healthRollups        *healthrollupsservice.Service
	modelRateLimits      *modelratelimitsservice.Service
	groupRateLimits      *groupratelimitsservice.Service
	userPlatformQuotas   *userplatformquotasservice.Service
	payloadRules         *payloadrulesservice.Service
	scheduledTests       *scheduledtestsservice.Service
	scheduledTestRunner  *scheduledtestworker.Runner
	backupSnapshots      *backupsnapservice.Service
	backupSnapshotsStore backupsnapcontract.Store
	accountProvisioning  *accountprovisioningservice.Service
	channelMonitors      *channelmonitorsservice.Service
	copilotEngine        *copilot.Engine
	moderationClients    moderationClientCache
	internalRouter       http.Handler
	userStore            userscontract.Store
	sessionStore         authcontract.Store
	apiKeyStore          apikeycontract.Store
	auditStore           auditcontract.Store
	billingStore         billingcontract.Store
	eventsStore          eventscontract.Store
	affiliateStore       affiliatecontract.Store
	operationsStore      operationscontract.Store
	providerStore        providercontract.Store
	modelStore           modelcontract.Store
	accountStore         accountcontract.Store
	adminControlStore    admincontrolcontract.Store
	paymentStore         paymentcontract.Store
	qualityEvalStore     qualitycontract.Store
	realtimeStore        realtimecontract.Store
	// balanceReservation is the optional atomic-reservation gate that prevents
	// concurrent gateway requests from collectively over-spending a user's
	// balance before the asynchronous balance_charger has a chance to debit.
	// Nil when Redis isn't configured — the balance gate then falls back to
	// the single-instance read-only check.
	balanceReservation      balanceReservationStore
	rateLimiter             *ratelimit.Limiter
	schedulerStore          schedulercontract.Store
	sessionAffinity         sessionaffinitycontract.Store
	subscriptionStore       subscriptioncontract.Store
	totpStore               totpcontract.Store
	usageStore              usagecontract.Store
	userAttributesStore     userattributescontract.Store
	errorPassthroughStore   errorpassthroughcontract.Store
	tlsProfilesStore        tlsprofilescontract.Store
	healthRollupsStore      healthrollupscontract.Store
	modelRateLimitsStore    modelratelimitscontract.Store
	groupRateLimitsStore    groupratelimitscontract.Store
	userPlatformQuotasStore userplatformquotascontract.Store
	payloadRulesStore       payloadrulescontract.Store
	scheduledTestsStore     scheduledtestscontract.Store
	channelMonitorsStore    channelmonitorscontract.Store
	copilotConvsStore       copilotconvcontract.ConversationStore
	metrics                 *runtimeMetricsState
	capabilities            []capabilitiescontract.Definition
	databaseProbe           dependencyPinger
	redisProbe              dependencyPinger
	// credentialRefreshGroup coalesces concurrent OAuth refreshes per account so
	// rotating refresh tokens are never consumed twice in parallel (which would
	// invalidate the session and park the account).
	credentialRefreshGroup singleflight.Group

	accountBreakers   map[int]*circuitbreaker.Breaker
	accountBreakersMu sync.RWMutex

	// concurrencySlots is the in-process per-account concurrency-slot
	// manager (ported from sub2api's ConcurrencyService). The gateway hot
	// path acquires a slot AFTER the scheduler picks a candidate but BEFORE
	// the upstream call, gated by metadata flag `concurrency_slot_enabled`
	// to keep existing deployments unchanged. Always non-nil after
	// newRuntimeState; nil-checks elsewhere are defensive only.
	concurrencySlots *concurrencyslotsservice.Service

	// rateLimitCooldown is the in-process per-account 429-cooldown registry
	// (ported from sub2api's RateLimitService). The gateway records hits
	// when ClassifyUpstreamError returns class=transient with a non-zero
	// RetryAfterMs, and consults it before scheduling to add cooldowned
	// accounts to the excluded list.
	rateLimitCooldown *ratelimitcooldownservice.Service

	// accountMetaLocks serializes read-modify-write updates to a provider
	// account's metadata (cooldown escalation and runtime-quota snapshots) so
	// concurrent gateway-side writes for the same account — now that the usage
	// path is asynchronous — don't clobber each other's changes by each cloning a
	// stale metadata map and writing back the full document. Keyed by account ID;
	// values are *sync.Mutex. Cross-subsystem edits (admin) are out of scope and
	// still rely on last-write-wins.
	accountMetaLocks sync.Map

	modelResolutionCache *localcache.Cache[modelcontract.ModelResolution]

	eventHub *eventsub.Hub

	// usageSem bounds the number of in-flight asynchronous gateway usage / billing
	// writes. When nil (struct-literal test runtimes, or async disabled via
	// GATEWAY_USAGE_MAX_CONCURRENCY=0) recordGatewayUsage runs inline. When the
	// semaphore is saturated the caller falls back to inline processing
	// (backpressure) rather than dropping billing data. usageWG tracks the
	// outstanding async writes so graceful shutdown can drain them before the DB
	// connection closes.
	// proxyProbeMetrics + tokenRefreshMetrics return the latest worker counter
	// snapshots so /metrics can render them. Nil when the worker is disabled or
	// the test runtime doesn't bother wiring them; the collector treats nil as
	// "emit nothing".
	proxyProbeMetrics   ProxyProbeMetricsProvider
	tokenRefreshMetrics TokenRefreshMetricsProvider

	usageSem chan struct{}
	usageWG  sync.WaitGroup
	// usageMu guards usageDraining. Dispatch takes the read lock around its
	// WaitGroup.Add (so the Add is ordered before drain's Wait), and drain takes
	// the write lock to set usageDraining before waiting. Once draining is set,
	// dispatch runs inline instead of spawning a tracked goroutine, so no new
	// Add can race a Wait (which would panic) even if graceful shutdown timed
	// out with a handler still in flight.
	usageMu       sync.RWMutex
	usageDraining bool

	// usageAggregator, when set, applies a completed request's billing
	// aggregation (subscription materialized usage + api-key cost usage) under the
	// usage_log.aggregated_at idempotency marker. When nil the gateway uses the
	// direct (unmarked) increment path.
	usageAggregator usageAggregator

	// billingBreaker guards the billing aggregation write path. When
	// persistently failing it trips open so request processing continues
	// without billing — the reconciler recovers dropped rows from the
	// durable usage_log. Inspired by sub2api's BillingCircuitBreaker.
	billingBreaker *circuitbreaker.Breaker

	// streamBillingDedup prevents double-billing when a client retries a
	// streaming request. Keyed by RequestID; entries expire after 5 minutes.
	streamBillingDedup *localcache.Cache[bool]

	// authRateLimit tracks per-IP login attempt counts to prevent brute-force
	// attacks. Each entry holds the count of attempts in the current window;
	// entries expire after 15 minutes.
	authRateLimit *localcache.Cache[int]

	// requestLogFiles holds the optional file-based per-request capture
	// (gateway HTTP envelope dumps). Lazily populated by
	// ensureRequestLogFilesState; nil until the first call. Disabling
	// SRAPI_REQUEST_LOG_ENABLED leaves the writer in no-op mode but still
	// constructs a reader so admin endpoints can browse pre-existing dumps.
	requestLogFilesMu sync.Mutex
	requestLogFiles   *requestLogFilesState

	// errorEventStream is the in-memory pub/sub backing the admin SSE
	// error-stream endpoint. Mirrors CLIProxyAPI's redisqueue.SubscribeErrors:
	// every recordGatewayProviderAttemptFailure publishes a contract.Event;
	// admin SSE subscribers receive them live with a 256-entry per-subscriber
	// buffer (drop-on-overflow). Always non-nil after newRuntimeState.
	errorEventStream *erroreventstreamservice.MemoryPublisher

	// copilotSummary* caches the system-state snapshot used in the copilot's
	// system prompt so we don't re-query accounts/providers/models on every
	// chat message (TTL: 60 s).
	copilotSummaryCache    string
	copilotSummaryCachedAt time.Time
	copilotSummaryMu       sync.Mutex
}

type dependencyHealth struct {
	Database apiopenapi.HealthDependencyStatus
	Redis    apiopenapi.HealthDependencyStatus
}

func newRuntimeState(cfg config.Config, logger *slog.Logger, opts runtimeOptions) (*runtimeState, error) {
	allowMemoryStores := cfg.UsesMemoryStorage()
	if allowMemoryStores {
		logger.Warn("running with ephemeral in-memory stores", "storage_backend", cfg.StorageBackend())
	}

	access, err := newAccessRuntime(cfg, opts, allowMemoryStores)
	if err != nil {
		return nil, err
	}

	contentSafetySvc := contentsafetyservice.New(contentsafetyservice.DefaultConfig())

	gatewaySvc, err := gatewayservice.New()
	if err != nil {
		return nil, err
	}

	providerStore := opts.providers
	if providerStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("providers")
		}
		providerStore = providermemory.New()
	}
	providersSvc, err := providerservice.New(providerStore, nil)
	if err != nil {
		return nil, err
	}

	modelStore := opts.models
	if modelStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("models")
		}
		modelStore = modelmemory.New()
	}
	modelsSvc, err := modelservice.New(modelStore, nil)
	if err != nil {
		return nil, err
	}

	adapterRuntime, err := newGatewayAdapterRuntime(cfg)
	if err != nil {
		return nil, err
	}
	realtimeLimits := realtimeservice.Limits{
		MaxOpenSlots:       cfg.Gateway.RealtimeMaxOpenSlots,
		MaxOpenSlotsPerKey: cfg.Gateway.RealtimeMaxOpenSlotsPerKey,
	}
	var realtimeSvc *realtimeservice.Service
	if opts.realtime != nil {
		realtimeSvc, err = realtimeservice.NewWithStore(realtimeLimits, nil, opts.realtime)
	} else {
		realtimeSvc, err = realtimeservice.New(realtimeLimits, nil)
	}
	if err != nil {
		return nil, err
	}

	accountStore := opts.accounts
	if accountStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("accounts")
		}
		accountStore = accountmemory.New()
	}
	accountsSvc, err := accountservice.New(accountStore, cfg.Security.MasterKey, nil)
	if err != nil {
		return nil, err
	}

	qualityEvalStore := opts.qualityEval
	if qualityEvalStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("quality eval")
		}
		qualityEvalStore = qualitymemory.New()
	}
	qualityEvalSvc, err := qualityservice.New(qualityEvalStore, cfg.Security.MasterKey, nil)
	if err != nil {
		return nil, err
	}

	schedulerStore := opts.scheduler
	if schedulerStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("scheduler")
		}
		schedulerStore = schedulermemory.New()
	}
	schedulerSvc, err := schedulerservice.New(schedulerStore, nil)
	if err != nil {
		return nil, err
	}

	subscriptionStore, subscriptionSvc, paymentStore, paymentsSvc, err := newCommerceRuntime(cfg, opts, access.billingSvc, access.auditSvc, access.eventsSvc, access.usersSvc, allowMemoryStores)
	if err != nil {
		return nil, err
	}

	adminControlStore := opts.adminControl
	if adminControlStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("admin control")
		}
		adminControlStore = admincontrolmemory.NewWithFulfillment(access.userStore, access.billingStore, subscriptionStore)
	}
	adminControlSvc, err := admincontrolservice.New(adminControlStore, nil)
	if err != nil {
		return nil, err
	}
	notificationContacts, err := notificationsservice.NewContactService(adminControlStore, cfg.Security.MasterKey, cfg.Email.PublicBaseURL, access.eventsSvc)
	if err != nil {
		return nil, err
	}
	userAvatars, err := usersservice.NewAvatarService(adminControlStore, nil)
	if err != nil {
		return nil, err
	}

	usageStore, usageSvc, operationsStore, operationsSvc, err := newUsageRuntime(opts, allowMemoryStores)
	if err != nil {
		return nil, err
	}
	sessionAffinityStore := sessionAffinityStoreForRuntime(opts, allowMemoryStores)

	rt := assembleRuntimeState(cfg, logger, opts, runtimeAssembly{
		usersSvc:                access.usersSvc,
		authSvc:                 access.authSvc,
		apiKeysSvc:              access.apiKeysSvc,
		auditSvc:                access.auditSvc,
		billingSvc:              access.billingSvc,
		eventsSvc:               access.eventsSvc,
		affiliateSvc:            access.affiliateSvc,
		notificationContactsSvc: notificationContacts,
		userAvatarsSvc:          userAvatars,
		contentSafetySvc:        contentSafetySvc,
		gatewaySvc:              gatewaySvc,
		providersSvc:            providersSvc,
		modelsSvc:               modelsSvc,
		adaptersSvc:             adapterRuntime.adaptersSvc,
		realtimeSvc:             realtimeSvc,
		reverseProxySvc:         adapterRuntime.reverseProxySvc,
		accountsSvc:             accountsSvc,
		adminControlSvc:         adminControlSvc,
		qualityEvalSvc:          qualityEvalSvc,
		schedulerSvc:            schedulerSvc,
		subscriptionSvc:         subscriptionSvc,
		totpSvc:                 access.totpSvc,
		paymentsSvc:             paymentsSvc,
		operationsSvc:           operationsSvc,
		usageSvc:                usageSvc,
		userStore:               access.userStore,
		sessionStore:            access.sessionStore,
		apiKeyStore:             access.apiKeyStore,
		auditStore:              access.auditStore,
		billingStore:            access.billingStore,
		eventsStore:             access.eventsStore,
		affiliateStore:          access.affiliateStore,
		operationsStore:         operationsStore,
		providerStore:           providerStore,
		modelStore:              modelStore,
		accountStore:            accountStore,
		adminControlStore:       adminControlStore,
		paymentStore:            paymentStore,
		qualityEvalStore:        qualityEvalStore,
		schedulerStore:          schedulerStore,
		subscriptionStore:       subscriptionStore,
		totpStore:               access.totpStore,
		usageStore:              usageStore,
	})
	rt.sessionAffinity = sessionAffinityStore
	idempotencyStore := opts.idempotency
	if idempotencyStore == nil {
		if !allowMemoryStores {
			return nil, missingRuntimeStoreError("idempotency")
		}
		idempotencyStore = idempotencymemory.New()
	}
	idempotencySvc, err := idempotencyservice.New(idempotencyStore, nil, 0, 0)
	if err != nil {
		return nil, err
	}
	rt.idempotency = idempotencySvc
	if err := rt.buildCapabilityServices(cfg, opts, allowMemoryStores); err != nil {
		return nil, err
	}
	if err := rt.bootstrapAdmin(context.Background()); err != nil {
		return nil, err
	}
	if err := rt.bootstrapGatewayCatalog(context.Background()); err != nil {
		return nil, err
	}
	rt.startUsageWriters(cfg.Gateway.UsageMaxConcurrency)
	return rt, nil
}

func newGatewayAdapterRuntime(cfg config.Config) (gatewayAdapterRuntime, error) {
	reverseProxySvc, err := reverseproxyservice.New(nil, reverseproxyservice.WithBlockedPrivateEgress(cfg.Server.Mode != "local"))
	if err != nil {
		return gatewayAdapterRuntime{}, err
	}
	adaptersSvc, err := provideradapterservice.NewWithReverseProxy(
		&http.Client{Timeout: cfg.Gateway.RequestTimeout},
		reverseProxySvc,
		// Synthesize fake upstream responses only in local/dev. In any other mode a
		// missing base_url must hard-error so customers are never billed for fakes.
		provideradapterservice.WithLocalStub(cfg.Server.Mode == "local"),
		// Plumb the deployment config so the codex.go request-build path can
		// consult the global OAuth model-alias map and DisableImageGeneration
		// enum (ported from CLIProxyAPI). nil-safe at the helper level.
		provideradapterservice.WithConfig(&cfg),
	)
	if err != nil {
		return gatewayAdapterRuntime{}, err
	}
	return gatewayAdapterRuntime{adaptersSvc: adaptersSvc, reverseProxySvc: reverseProxySvc}, nil
}

// buildCapabilityServices wires the sub2api gap-closure services (user
// attributes, error-passthrough rules, TLS fingerprint profiles, captcha, and
// availability rollups) onto the runtime, falling back to memory stores when
// permitted.
func (rt *runtimeState) buildCapabilityServices(cfg config.Config, opts runtimeOptions, allowMemoryStores bool) error {
	userAttributesStore := opts.userAttributes
	if userAttributesStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("user attributes")
		}
		userAttributesStore = userattributesmemory.New()
	}
	rt.userAttributesStore = userAttributesStore
	userAttributesSvc, err := userattributesservice.New(userAttributesStore)
	if err != nil {
		return err
	}
	rt.userAttributes = userAttributesSvc

	errorPassthroughStore := opts.errorPassthrough
	if errorPassthroughStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("error passthrough")
		}
		errorPassthroughStore = errorpassthroughmemory.New()
	}
	rt.errorPassthroughStore = errorPassthroughStore
	errorPassthroughSvc, err := errorpassthroughservice.New(errorPassthroughStore)
	if err != nil {
		return err
	}
	rt.errorPassthrough = errorPassthroughSvc

	if err := rt.buildOpsErrorLogsService(opts, allowMemoryStores); err != nil {
		return err
	}

	tlsProfilesStore := opts.tlsProfiles
	if tlsProfilesStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("tls profiles")
		}
		tlsProfilesStore = tlsprofilesmemory.New()
	}
	rt.tlsProfilesStore = tlsProfilesStore
	tlsProfilesSvc, err := tlsprofilesservice.New(tlsProfilesStore)
	if err != nil {
		return err
	}
	rt.tlsProfiles = tlsProfilesSvc
	rt.captcha = captchaservice.New(captchaservice.Config{
		Enabled:   cfg.Captcha.Enabled,
		Provider:  cfg.Captcha.Provider,
		SecretKey: cfg.Captcha.SecretKey,
		SiteKey:   cfg.Captcha.SiteKey,
		VerifyURL: cfg.Captcha.VerifyURL,
	}, nil)
	reverseproxyservice.SetNamedProfileExpander(rt.expandEgressProfileMetadata)

	healthRollupsStore := opts.healthRollups
	if healthRollupsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("health rollups")
		}
		healthRollupsStore = healthrollupsmemory.New()
	}
	rt.healthRollupsStore = healthRollupsStore
	healthRollupsSvc, err := healthrollupsservice.New(healthRollupsStore)
	if err != nil {
		return err
	}
	rt.healthRollups = healthRollupsSvc

	modelRateLimitsStore := opts.modelRateLimits
	if modelRateLimitsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("model rate limits")
		}
		modelRateLimitsStore = modelratelimitsmemory.New()
	}
	rt.modelRateLimitsStore = modelRateLimitsStore
	modelRateLimitsSvc, err := modelratelimitsservice.New(modelRateLimitsStore)
	if err != nil {
		return err
	}
	rt.modelRateLimits = modelRateLimitsSvc

	groupRateLimitsStore := opts.groupRateLimits
	if groupRateLimitsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("group rate limits")
		}
		groupRateLimitsStore = groupratelimitsmemory.New()
	}
	rt.groupRateLimitsStore = groupRateLimitsStore
	groupRateLimitsSvc, err := groupratelimitsservice.New(groupRateLimitsStore)
	if err != nil {
		return err
	}
	rt.groupRateLimits = groupRateLimitsSvc

	userPlatformQuotasStore := opts.userPlatformQuotas
	if userPlatformQuotasStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("user platform quotas")
		}
		userPlatformQuotasStore = userplatformquotasmemory.New()
	}
	rt.userPlatformQuotasStore = userPlatformQuotasStore
	userPlatformQuotasSvc, err := userplatformquotasservice.New(userPlatformQuotasStore)
	if err != nil {
		return err
	}
	rt.userPlatformQuotas = userPlatformQuotasSvc

	payloadRulesStore := opts.payloadRules
	if payloadRulesStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("payload rules")
		}
		payloadRulesStore = payloadrulesmemory.New()
	}
	rt.payloadRulesStore = payloadRulesStore
	payloadRulesSvc, err := payloadrulesservice.New(payloadRulesStore)
	if err != nil {
		return err
	}
	rt.payloadRules = payloadRulesSvc

	scheduledTestsStore := opts.scheduledTests
	if scheduledTestsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("scheduled tests")
		}
		scheduledTestsStore = scheduledtestsmemory.New()
	}
	rt.scheduledTestsStore = scheduledTestsStore
	scheduledTestsSvc, err := scheduledtestsservice.New(scheduledTestsStore, nil)
	if err != nil {
		return err
	}
	rt.scheduledTests = scheduledTestsSvc
	scheduledTestRunner, err := scheduledtestworker.NewRunner(rt.accounts, rt.providers, scheduledTestsSvc, scheduledtestworker.RealProber(rt.adapters))
	if err != nil {
		return err
	}
	rt.scheduledTestRunner = scheduledTestRunner

	// Backup snapshots: the persistent store is wired by the bootstrap layer
	// (apps/api/internal/app/app.go) since the dump files only live on the
	// API host's disk. Memory-only runtimes use an in-memory store so the
	// rest of the wiring (admin handlers, service tests) stays coherent.
	backupSnapshotsStore := opts.backupSnapshots
	if backupSnapshotsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("backup snapshots")
		}
		backupSnapshotsStore = backupsnapmemory.New()
	}
	rt.backupSnapshotsStore = backupSnapshotsStore
	backupSnapshotsSvc, err := backupsnapservice.New(backupSnapshotsStore, opts.backupSnapshotsTrigger, nil)
	if err != nil {
		return err
	}
	rt.backupSnapshots = backupSnapshotsSvc

	// Upstream-account OAuth provisioning keeps pending sessions in-memory only
	// (short-lived, single-process); a restart simply drops in-flight wizards.
	rt.accountProvisioning = accountprovisioningservice.New()
	channelMonitorsStore := opts.channelMonitors
	if channelMonitorsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("channel monitors")
		}
		channelMonitorsStore = channelmonitorsmemory.New()
	}
	rt.channelMonitorsStore = channelMonitorsStore
	channelMonitorsSvc, err := channelmonitorsservice.New(channelMonitorsStore)
	if err != nil {
		return err
	}
	rt.channelMonitors = channelMonitorsSvc

	copilotConvsStore := opts.copilotConvs
	if copilotConvsStore == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("copilot conversations")
		}
		copilotConvsStore = copilotconvmemory.New()
	}
	rt.copilotConvsStore = copilotConvsStore

	// The admin copilot loads its tool catalog from the embedded OpenAPI spec. A
	// parse failure disables the copilot (handlers return 503) but must not block
	// the rest of the server from starting.
	if catalog, err := copilot.LoadCatalog(); err != nil {
		rt.logger.Error("failed to load admin copilot catalog", "error", err)
	} else {
		rt.copilotEngine = copilot.NewEngine(catalog)
	}
	return nil
}

func (rt *runtimeState) buildOpsErrorLogsService(opts runtimeOptions, allowMemoryStores bool) error {
	store := opts.opsErrorLogs
	if store == nil {
		if !allowMemoryStores {
			return missingRuntimeStoreError("ops error logs")
		}
		store = opserrorlogsmemory.New()
	}

	rt.opsErrorLogsStore = store
	service, err := opserrorlogsservice.New(store, nil)
	if err != nil {
		return err
	}
	rt.opsErrorLogs = service
	rt.opsErrorLogRecorder = newOpsErrorLogRecorder(service, rt.logger, opsErrorLogRecorderConfig{})
	return nil
}

type accessRuntime struct {
	usersSvc       *usersservice.Service
	authSvc        *authservice.Service
	apiKeysSvc     *apikeyservice.Service
	auditSvc       *auditservice.Service
	billingSvc     *billingservice.Service
	eventsSvc      *eventsservice.Service
	affiliateSvc   *affiliateservice.Service
	totpSvc        *totpservice.Service
	userStore      userscontract.Store
	sessionStore   authcontract.Store
	apiKeyStore    apikeycontract.Store
	auditStore     auditcontract.Store
	billingStore   billingcontract.Store
	eventsStore    eventscontract.Store
	affiliateStore affiliatecontract.Store
	totpStore      totpcontract.Store
}

func newAccessRuntime(cfg config.Config, opts runtimeOptions, allowMemoryStores bool) (accessRuntime, error) {
	userStore := opts.users
	if userStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("users")
		}
		userStore = usermemory.New()
	}
	usersSvc, err := usersservice.NewWithPasswordCost(userStore, nil, cfg.Security.PasswordHashCost)
	if err != nil {
		return accessRuntime{}, err
	}

	sessionStore := opts.authSessions
	if sessionStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("auth sessions")
		}
		sessionStore = authmemory.New()
	}

	totpStore := opts.totp
	if totpStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("totp")
		}
		totpStore = totpmemory.New()
	}
	totpSvc, err := totpservice.New(totpStore, cfg.Security.TOTPEncryptionKey, "SRapi", nil)
	if err != nil {
		return accessRuntime{}, err
	}

	authSvc, err := authservice.NewWithSecondFactor(usersSvc, sessionStore, 0, nil, totpSvc, cfg.Security.MasterKey)
	if err != nil {
		return accessRuntime{}, err
	}

	apiKeyStore := opts.apiKeys
	if apiKeyStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("api keys")
		}
		apiKeyStore = apikeymemory.New()
	}
	apiKeysSvc, err := apikeyservice.New(apiKeyStore, cfg.Security.APIKeyPepper, nil)
	if err != nil {
		return accessRuntime{}, err
	}
	// Wire the in-memory auth cache + per-key RPM counter (port of
	// sub2api's api_key_auth_cache + counter). The cache short-circuits
	// the gateway hot path's SQL FindByPrefix; the counter is consulted by
	// observability tooling. Both are no-op-safe when the service runs
	// without them, so wiring is unconditional.
	apiKeysSvc.SetAuthCache(apikeyservice.NewAuthCache())
	rpmCounter := apikeyservice.NewRPMCounter(apikeyservice.RPMCounterConfig{})
	apiKeysSvc.SetRPMCounter(rpmCounter)
	// Start the periodic flush loop with a never-cancelled context — the
	// goroutine is bounded (single ticker) and process exit reaps it. We
	// can't easily plumb a per-process stop signal because httpserver.New
	// is called from app.newHandler which already discards the runtime
	// after construction. Future work: hoist the runtime stop hook into
	// the app worker pool so this can be drained on graceful shutdown.
	rpmCounter.Start(context.Background())

	auditStore := opts.audit
	if auditStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("audit")
		}
		auditStore = auditmemory.New()
	}
	auditSvc, err := auditservice.New(auditStore, nil)
	if err != nil {
		return accessRuntime{}, err
	}

	billingStore := opts.billing
	if billingStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("billing")
		}
		billingStore = billingmemory.New()
	}
	billingSvc, err := billingservice.New(billingStore, nil)
	if err != nil {
		return accessRuntime{}, err
	}

	eventsStore := opts.events
	if eventsStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("events")
		}
		eventsStore = eventsmemory.New()
	}
	eventsSvc, err := eventsservice.New(eventsStore, nil)
	if err != nil {
		return accessRuntime{}, err
	}
	authSvc.SetEventEnqueuer(eventsSvc)

	affiliateStore := opts.affiliate
	if affiliateStore == nil {
		if !allowMemoryStores {
			return accessRuntime{}, missingRuntimeStoreError("affiliate")
		}
		affiliateStore = affiliatememory.New()
	}
	affiliateSvc, err := affiliateservice.New(affiliateStore, affiliateservice.Dependencies{
		Audit:  auditSvc,
		Events: eventsSvc,
	}, nil)
	if err != nil {
		return accessRuntime{}, err
	}

	return accessRuntime{
		usersSvc:       usersSvc,
		authSvc:        authSvc,
		apiKeysSvc:     apiKeysSvc,
		auditSvc:       auditSvc,
		billingSvc:     billingSvc,
		eventsSvc:      eventsSvc,
		affiliateSvc:   affiliateSvc,
		totpSvc:        totpSvc,
		userStore:      userStore,
		sessionStore:   sessionStore,
		apiKeyStore:    apiKeyStore,
		auditStore:     auditStore,
		billingStore:   billingStore,
		eventsStore:    eventsStore,
		affiliateStore: affiliateStore,
		totpStore:      totpStore,
	}, nil
}

func newCommerceRuntime(
	cfg config.Config,
	opts runtimeOptions,
	billingSvc *billingservice.Service,
	auditSvc *auditservice.Service,
	eventsSvc *eventsservice.Service,
	usersSvc *usersservice.Service,
	allowMemoryStores bool,
) (subscriptioncontract.Store, *subscriptionservice.Service, paymentcontract.Store, *paymentservice.Service, error) {
	subscriptionStore := opts.subscriptions
	if subscriptionStore == nil {
		if !allowMemoryStores {
			return nil, nil, nil, nil, missingRuntimeStoreError("subscriptions")
		}
		subscriptionStore = subscriptionmemory.New()
	}
	subscriptionSvc, err := subscriptionservice.New(subscriptionStore, nil)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	paymentStore := opts.payments
	if paymentStore == nil {
		if !allowMemoryStores {
			return nil, nil, nil, nil, missingRuntimeStoreError("payments")
		}
		paymentStore = paymentmemory.New()
	}
	paymentsSvc, err := paymentservice.New(paymentStore, cfg.Security.MasterKey, paymentservice.Dependencies{
		Billing:       billingSvc,
		Subscriptions: subscriptionSvc,
		Audit:         auditSvc,
		Events:        eventsSvc,
		Balance:       paymentsBalanceAdapter{users: usersSvc},
	}, nil)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return subscriptionStore, subscriptionSvc, paymentStore, paymentsSvc, nil
}

func newUsageRuntime(opts runtimeOptions, allowMemoryStores bool) (usagecontract.Store, *usageservice.Service, operationscontract.Store, *operationsservice.Service, error) {
	usageStore := opts.usage
	if usageStore == nil {
		if !allowMemoryStores {
			return nil, nil, nil, nil, missingRuntimeStoreError("usage")
		}
		usageStore = usagememory.New()
	}
	usageSvc, err := usageservice.New(usageStore, nil)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	operationsStore, operationsSvc, err := newOperationsRuntime(opts.operations, usageStore, allowMemoryStores)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return usageStore, usageSvc, operationsStore, operationsSvc, nil
}

type runtimeAssembly struct {
	usersSvc                *usersservice.Service
	authSvc                 *authservice.Service
	apiKeysSvc              *apikeyservice.Service
	auditSvc                *auditservice.Service
	billingSvc              *billingservice.Service
	eventsSvc               *eventsservice.Service
	affiliateSvc            *affiliateservice.Service
	notificationContactsSvc *notificationsservice.ContactService
	userAvatarsSvc          *usersservice.AvatarService
	contentSafetySvc        *contentsafetyservice.Service
	gatewaySvc              *gatewayservice.Service
	providersSvc            *providerservice.Service
	modelsSvc               *modelservice.Service
	adaptersSvc             *provideradapterservice.Service
	realtimeSvc             *realtimeservice.Service
	reverseProxySvc         *reverseproxyservice.Service
	accountsSvc             *accountservice.Service
	adminControlSvc         *admincontrolservice.Service
	qualityEvalSvc          *qualityservice.Service
	schedulerSvc            *schedulerservice.Service
	subscriptionSvc         *subscriptionservice.Service
	totpSvc                 *totpservice.Service
	paymentsSvc             *paymentservice.Service
	operationsSvc           *operationsservice.Service
	usageSvc                *usageservice.Service
	userStore               userscontract.Store
	sessionStore            authcontract.Store
	apiKeyStore             apikeycontract.Store
	auditStore              auditcontract.Store
	billingStore            billingcontract.Store
	eventsStore             eventscontract.Store
	affiliateStore          affiliatecontract.Store
	operationsStore         operationscontract.Store
	providerStore           providercontract.Store
	modelStore              modelcontract.Store
	accountStore            accountcontract.Store
	adminControlStore       admincontrolcontract.Store
	paymentStore            paymentcontract.Store
	qualityEvalStore        qualitycontract.Store
	schedulerStore          schedulercontract.Store
	subscriptionStore       subscriptioncontract.Store
	totpStore               totpcontract.Store
	usageStore              usagecontract.Store
}

type gatewayAdapterRuntime struct {
	adaptersSvc     *provideradapterservice.Service
	reverseProxySvc *reverseproxyservice.Service
}

func assembleRuntimeState(cfg config.Config, logger *slog.Logger, opts runtimeOptions, assembly runtimeAssembly) *runtimeState {
	return &runtimeState{
		cfg:                  cfg,
		logger:               logger,
		users:                assembly.usersSvc,
		auth:                 assembly.authSvc,
		apiKeys:              assembly.apiKeysSvc,
		audit:                assembly.auditSvc,
		billing:              assembly.billingSvc,
		events:               assembly.eventsSvc,
		affiliate:            assembly.affiliateSvc,
		notificationContacts: assembly.notificationContactsSvc,
		userAvatars:          assembly.userAvatarsSvc,
		contentSafety:        assembly.contentSafetySvc,
		gateway:              assembly.gatewaySvc,
		providers:            assembly.providersSvc,
		models:               assembly.modelsSvc,
		adapters:             assembly.adaptersSvc,
		realtime:             assembly.realtimeSvc,
		reverseProxy:         assembly.reverseProxySvc,
		accounts:             assembly.accountsSvc,
		adminControl:         assembly.adminControlSvc,
		qualityEval:          assembly.qualityEvalSvc,
		scheduler:            assembly.schedulerSvc,
		subscriptions:        assembly.subscriptionSvc,
		totp:                 assembly.totpSvc,
		payments:             assembly.paymentsSvc,
		operations:           assembly.operationsSvc,
		usage:                assembly.usageSvc,
		userStore:            assembly.userStore,
		sessionStore:         assembly.sessionStore,
		apiKeyStore:          assembly.apiKeyStore,
		auditStore:           assembly.auditStore,
		billingStore:         assembly.billingStore,
		eventsStore:          assembly.eventsStore,
		affiliateStore:       assembly.affiliateStore,
		operationsStore:      assembly.operationsStore,
		providerStore:        assembly.providerStore,
		modelStore:           assembly.modelStore,
		accountStore:         assembly.accountStore,
		adminControlStore:    assembly.adminControlStore,
		paymentStore:         assembly.paymentStore,
		qualityEvalStore:     assembly.qualityEvalStore,
		realtimeStore:        opts.realtime,
		balanceReservation:   opts.balanceReservation,
		rateLimiter:          opts.rateLimiter,
		schedulerStore:       assembly.schedulerStore,
		sessionAffinity:      opts.sessionAffinity,
		subscriptionStore:    assembly.subscriptionStore,
		totpStore:            assembly.totpStore,
		usageStore:           assembly.usageStore,
		metrics:              runtimeMetricsFromOptions(opts),
		capabilities:         seedCapabilities(),
		databaseProbe:        opts.database,
		redisProbe:           opts.redis,
		accountBreakers:      make(map[int]*circuitbreaker.Breaker),
		concurrencySlots:     concurrencyslotsservice.New(),
		rateLimitCooldown: func() *ratelimitcooldownservice.Service {
			svc := ratelimitcooldownservice.New()
			if opts.redisCmd != nil {
				svc.SetHitCounter(ratelimitcooldownservice.NewRedisHitCounter(opts.redisCmd))
			}
			return svc
		}(),
		modelResolutionCache: localcache.New[modelcontract.ModelResolution](localcache.Config{
			MaxEntries: 512,
			DefaultTTL: 30 * time.Second,
		}),
		eventHub:            eventsub.NewHub(),
		usageAggregator: opts.usageAggregator,
		streamBillingDedup: localcache.New[bool](localcache.Config{
			MaxEntries: 4096,
			DefaultTTL: 5 * time.Minute,
		}),
		authRateLimit: localcache.New[int](localcache.Config{
			MaxEntries: 8192,
			DefaultTTL: 15 * time.Minute,
		}),
		billingBreaker: circuitbreaker.New(circuitbreaker.Config{
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          30 * time.Second,
			MaxTimeout:       5 * time.Minute,
			OnStateChange: func(from, to circuitbreaker.State) {
				logger.Warn("billing aggregation circuit breaker state change",
					"from", from.String(), "to", to.String())
			},
		}),
		proxyProbeMetrics:   opts.proxyProbeMetrics,
		tokenRefreshMetrics: opts.tokenRefreshMetrics,
		errorEventStream:    erroreventstreamservice.NewMemoryPublisher(erroreventstreamservice.Config{Logger: logger}),
	}
}

func (rt *runtimeState) resolveModelCached(ctx context.Context, name string) (modelcontract.ModelResolution, error) {
	if cached, ok := rt.modelResolutionCache.Get(name); ok {
		return cached, nil
	}
	resolution, err := rt.models.ResolveModelReference(ctx, name)
	if err != nil {
		return resolution, err
	}
	rt.modelResolutionCache.Set(name, resolution)
	return resolution, nil
}

func (rt *runtimeState) invalidateModelCache(names ...string) {
	if rt.modelResolutionCache == nil {
		return
	}
	for _, name := range names {
		rt.modelResolutionCache.Delete(name)
	}
}

func (rt *runtimeState) closeLocalCaches() {
	if rt.modelResolutionCache != nil {
		rt.modelResolutionCache.Close()
	}
}

func (rt *runtimeState) accountBreaker(accountID int) *circuitbreaker.Breaker {
	rt.accountBreakersMu.RLock()
	b, ok := rt.accountBreakers[accountID]
	rt.accountBreakersMu.RUnlock()
	if ok {
		return b
	}

	rt.accountBreakersMu.Lock()
	defer rt.accountBreakersMu.Unlock()
	if b, ok = rt.accountBreakers[accountID]; ok {
		return b
	}
	b = circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		MaxTimeout:          5 * time.Minute,
		MaxHalfOpenRequests: 1,
		OnStateChange: func(from, to circuitbreaker.State) {
			if rt.logger != nil {
				rt.logger.Info("account circuit breaker state change",
					"account_id", accountID,
					"from", from.String(),
					"to", to.String())
			}
			if rt.eventHub != nil {
				rt.eventHub.Publish(eventsub.Event{
					Type: "circuit_breaker",
					Data: map[string]any{
						"account_id": accountID,
						"from":       from.String(),
						"to":         to.String(),
					},
				})
			}
		},
	})
	rt.accountBreakers[accountID] = b
	return b
}

func runtimeMetricsFromOptions(opts runtimeOptions) *runtimeMetricsState {
	if opts.metrics != nil {
		return opts.metrics
	}
	return newRuntimeMetricsState()
}

func newOperationsRuntime(store operationscontract.Store, usageStore usagecontract.Store, allowMemoryStores bool) (operationscontract.Store, *operationsservice.Service, error) {
	if store == nil {
		if !allowMemoryStores {
			return nil, nil, missingRuntimeStoreError("operations")
		}
		store = operationsmemory.NewWithUsageStore(usageStore)
	}
	service, err := operationsservice.NewWithStores(store, store, nil)
	if err != nil {
		return nil, nil, err
	}
	if _, err := service.EnsureBuiltinAlertRules(context.Background()); err != nil {
		return nil, nil, err
	}
	return store, service, nil
}

// NewMemorySessionAffinityStore builds a per-instance in-memory session affinity
// store. The app bootstrap uses it as a best-effort fallback when Redis is not
// configured (it cannot import the module package directly under the architecture
// rules). Memory-storage runtimes also fall back to this store so gateway
// resources that need follow-up account affinity can work in local/test mode.
func NewMemorySessionAffinityStore() sessionaffinitycontract.Store {
	return sessionaffinitymemory.New()
}

func sessionAffinityStoreForRuntime(opts runtimeOptions, allowMemoryStores bool) sessionaffinitycontract.Store {
	if opts.sessionAffinity != nil {
		return opts.sessionAffinity
	}
	if !allowMemoryStores {
		return nil
	}
	return sessionaffinitymemory.New()
}

func missingRuntimeStoreError(name string) error {
	return fmt.Errorf("missing %s store: inject a persistent store or set STORAGE_BACKEND=memory for explicit ephemeral mode", name)
}

func (rt *runtimeState) bootstrapAdmin(ctx context.Context) error {
	email := strings.TrimSpace(rt.cfg.Bootstrap.AdminEmail)
	password := rt.cfg.Bootstrap.AdminPassword
	name := strings.TrimSpace(rt.cfg.Bootstrap.AdminName)
	if email == "" || password == "" {
		return nil
	}
	if existing, err := rt.userStore.FindByEmail(ctx, email); err == nil {
		// The bootstrap admin is seeded only on first start. If the operator later
		// edits BOOTSTRAP_ADMIN_PASSWORD it is silently ignored — warn loudly so a
		// subsequent failed login is diagnosable instead of looking like a typo.
		if usersservice.ComparePassword(existing.PasswordHash, password) != nil {
			rt.logger.Warn("bootstrap admin already exists and BOOTSTRAP_ADMIN_PASSWORD no longer matches the stored password; it is only applied on first start and is ignored now — sign in with the original password or reset it via the password-reset flow",
				"email", email)
		}
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
			Capabilities: map[string]any{capabilitiescontract.KeyResponses: true, capabilitiescontract.KeyEmbeddings: true, capabilitiescontract.KeyImageGenerations: true, capabilitiescontract.KeyImageEdits: true, capabilitiescontract.KeyImageVariations: true, capabilitiescontract.KeyAudioTranscriptions: true, capabilitiescontract.KeyAudioSpeech: true, capabilitiescontract.KeyModerations: true},
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
	if provider.Capabilities[capabilitiescontract.KeyResponses] != true || provider.Capabilities[capabilitiescontract.KeyEmbeddings] != true || provider.Capabilities[capabilitiescontract.KeyImageGenerations] != true || provider.Capabilities[capabilitiescontract.KeyImageEdits] != true || provider.Capabilities[capabilitiescontract.KeyImageVariations] != true || provider.Capabilities[capabilitiescontract.KeyAudioTranscriptions] != true || provider.Capabilities[capabilitiescontract.KeyAudioSpeech] != true || provider.Capabilities[capabilitiescontract.KeyModerations] != true {
		capabilities := cloneAnyMap(provider.Capabilities)
		delete(capabilities, "images")
		capabilities[capabilitiescontract.KeyResponses] = true
		capabilities[capabilitiescontract.KeyEmbeddings] = true
		capabilities[capabilitiescontract.KeyImageGenerations] = true
		capabilities[capabilitiescontract.KeyImageEdits] = true
		capabilities[capabilitiescontract.KeyImageVariations] = true
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
		Database: dependencyStatus(probeStatus(ctx, rt.databaseProbe)),
		Redis:    dependencyStatus(probeStatus(ctx, rt.redisProbe)),
	}
}

func probeStatus(ctx context.Context, probe dependencyPinger) string {
	if probe != nil {
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := probe.Ping(probeCtx); err != nil {
			return "unavailable"
		}
		return "ok"
	}
	return "not_configured"
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

// paymentsBalanceAdapter lets the payments service credit/debit a user's
// spendable balance through the users service, keeping the two modules
// decoupled (payments depends on the narrow BalanceAdjuster interface).
type paymentsBalanceAdapter struct {
	users *usersservice.Service
}

func (a paymentsBalanceAdapter) CreditBalance(ctx context.Context, userID int, amount, currency string) error {
	_, err := a.users.UpdateBalance(ctx, userID, usersservice.BalanceUpdateRequest{
		Operation: userscontract.BalanceOperationIncrement,
		Amount:    amount,
		Currency:  currency,
	})
	return err
}

func (a paymentsBalanceAdapter) DebitBalance(ctx context.Context, userID int, amount, currency string) error {
	_, err := a.users.UpdateBalance(ctx, userID, usersservice.BalanceUpdateRequest{
		Operation: userscontract.BalanceOperationDecrement,
		Amount:    amount,
		Currency:  currency,
	})
	return err
}
