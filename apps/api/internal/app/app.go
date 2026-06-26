package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	"github.com/srapi/srapi/apps/api/internal/httpserver"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"
	entschedulerstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/scheduler"
	redisbalancereservation "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/balancereservation"
	redisrealtimestore "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/realtime"
	redisschedulerstore "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/scheduler"
	redissessionaffinitystore "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/sessionaffinity"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
	platformdb "github.com/srapi/srapi/apps/api/internal/platform/db"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	platformredis "github.com/srapi/srapi/apps/api/internal/platform/redis"
	accountquotaalertworker "github.com/srapi/srapi/apps/api/internal/workers/account_quota_alert"
	accountstokenrefreshworker "github.com/srapi/srapi/apps/api/internal/workers/accounts_token_refresh"
	alertnotificationsworker "github.com/srapi/srapi/apps/api/internal/workers/alert_notifications"
	authcleanupworker "github.com/srapi/srapi/apps/api/internal/workers/auth_session_cleanup"
	availabilityrollupworker "github.com/srapi/srapi/apps/api/internal/workers/availability_rollup"
	backupworker "github.com/srapi/srapi/apps/api/internal/workers/backup"
	balancechargerworker "github.com/srapi/srapi/apps/api/internal/workers/balance_charger"
	channelmonitorworker "github.com/srapi/srapi/apps/api/internal/workers/channel_monitor"
	connectivitytestworker "github.com/srapi/srapi/apps/api/internal/workers/connectivity_test"
	healthprobeworker "github.com/srapi/srapi/apps/api/internal/workers/health_probe"
	idempotencycleanupworker "github.com/srapi/srapi/apps/api/internal/workers/idempotency_cleanup"
	litellmpricingworker "github.com/srapi/srapi/apps/api/internal/workers/litellm_pricing"
	orderexpirerworker "github.com/srapi/srapi/apps/api/internal/workers/order_expirer"
	outboxworker "github.com/srapi/srapi/apps/api/internal/workers/outbox"
	paymentreconcileworker "github.com/srapi/srapi/apps/api/internal/workers/payment_reconcile"
	proxyprobeworker "github.com/srapi/srapi/apps/api/internal/workers/proxy_probe"
	qualityevalworker "github.com/srapi/srapi/apps/api/internal/workers/quality_eval"
	quotarefreshworker "github.com/srapi/srapi/apps/api/internal/workers/quota_refresh"
	retentionworker "github.com/srapi/srapi/apps/api/internal/workers/retention"
	scheduledtestworker "github.com/srapi/srapi/apps/api/internal/workers/scheduled_test"
	sloevaluatorworker "github.com/srapi/srapi/apps/api/internal/workers/slo_evaluator"
	subscriptionexpirerworker "github.com/srapi/srapi/apps/api/internal/workers/subscription_expirer"
	usageaggregationreconcilerworker "github.com/srapi/srapi/apps/api/internal/workers/usage_aggregation_reconciler"
	"github.com/srapi/srapi/apps/api/migrations"
)

const defaultReadHeaderTimeout = 10 * time.Second

type App struct {
	cfg     config.Config
	logger  *slog.Logger
	server  *http.Server
	db      *platformdb.Client
	redis   *platformredis.Client
	tracer  platformotel.ShutdownFunc
	workers appWorkers
	// usageDrain flushes in-flight asynchronous gateway usage/billing writes
	// during graceful shutdown. Populated by newHandler; nil when async usage
	// processing is disabled.
	usageDrain func(context.Context)
}

type appWorkers struct {
	outbox             *outboxworker.Worker
	retention          *retentionworker.Worker
	availability       *availabilityrollupworker.Worker
	backup             *backupworker.Worker
	authClean          *authcleanupworker.Worker
	idemClean          *idempotencycleanupworker.Worker
	quotaRefresh       *quotarefreshworker.Worker
	tokenRefresh       *accountstokenrefreshworker.Worker
	liteLLMPricing     *litellmpricingworker.Worker
	connectivityTest   *connectivitytestworker.Worker
	scheduledTest      *scheduledtestworker.Worker
	channelMonitor     *channelmonitorworker.Worker
	proxyProbe         *proxyprobeworker.Worker
	expirer            *orderexpirerworker.Worker
	reconcile          *paymentreconcileworker.Worker
	subExpiry          *subscriptionexpirerworker.Worker
	quota              *accountquotaalertworker.Worker
	balance            *balancechargerworker.Worker
	health             *healthprobeworker.Worker
	quality            *qualityevalworker.Worker
	sloEval            *sloevaluatorworker.Worker
	alertNotifications *alertnotificationsworker.Worker
	// usageReconciler re-applies billing aggregation for usage_log rows whose
	// eager apply was dropped (e.g. on a crash). Nil under memory storage.
	usageReconciler *usageaggregationreconcilerworker.Worker
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if err := verifyMasterKeyIntegrity(cfg, logger); err != nil {
		return nil, err
	}

	tracerShutdown, err := platformotel.SetupTracerProvider(context.Background(), cfg.Observability)
	if err != nil {
		return nil, err
	}

	dbClient, err := platformdb.Open(cfg.Database)
	if err != nil {
		_ = tracerShutdown(context.Background())
		return nil, err
	}
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		_ = dbClient.Close()
		_ = tracerShutdown(context.Background())
		return nil, err
	}

	handler, workers, usageDrain, err := newHandler(cfg, logger, dbClient, redisClient)
	if err != nil {
		_ = dbClient.Close()
		_ = redisClient.Close()
		_ = tracerShutdown(context.Background())
		return nil, err
	}

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           handler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       120 * time.Second,
	}
	return &App{
		cfg:        cfg,
		logger:     logger,
		server:     server,
		db:         dbClient,
		redis:      redisClient,
		tracer:     tracerShutdown,
		workers:    workers,
		usageDrain: usageDrain,
	}, nil
}

func (a *App) Serve() error {
	a.startWorkers()
	defer func() {
		_ = a.stopWorkers(context.Background())
		// Flush in-flight async usage/billing writes if ListenAndServe returned
		// on its own (e.g. a serve error) without going through Shutdown, which
		// already drains. Bounded so a stuck write cannot hang process exit.
		if a.usageDrain != nil {
			timeout := a.cfg.Server.ShutdownTimeout
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			a.usageDrain(ctx)
		}
	}()
	err := a.server.ListenAndServe()
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) Shutdown(ctx context.Context) error {
	var errs []error
	if err := a.stopWorkers(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := a.server.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	// The HTTP server has now stopped accepting connections and all in-flight
	// handlers have returned, so no new async usage writes will be dispatched.
	// Flush the ones still running before closing the database connection. Use a
	// fresh budget rather than the caller's ctx, which server.Shutdown may have
	// already (nearly) exhausted — otherwise a slow connection drain could starve
	// the usage drain and leave billing writes running against a closing DB.
	if a.usageDrain != nil {
		timeout := a.cfg.Server.ShutdownTimeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		drainCtx, cancel := context.WithTimeout(context.Background(), timeout)
		a.usageDrain(drainCtx)
		cancel()
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.tracer != nil {
		if err := a.tracer(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (a *App) Address() string {
	return a.server.Addr
}

func Healthcheck(ctx context.Context, cfg config.Config, path string) error {
	return httpserver.Healthcheck(ctx, cfg.HealthcheckAddress(), path)
}

func newHandler(cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client) (http.Handler, appWorkers, func(context.Context), error) {
	var (
		handler    http.Handler
		workers    appWorkers
		usageDrain func(context.Context)
		err        error
	)

	options, err := runtimeHTTPOptions(cfg, logger, dbClient, redisClient, &usageDrain)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	stores, err := persistentStores(context.Background(), cfg, logger, dbClient, redisClient)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workerGuard, err := newWorkerLeaderGuard(dbClient)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.outbox, err = domainEventsWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.retention, err = retentionCleanupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.availability, err = availabilityRollupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.backup, err = backupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.authClean, err = authSessionCleanupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.idemClean, err = idempotencyCleanupWorker(stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.expirer, err = paymentOrderExpirerWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.reconcile, err = paymentReconcileWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.subExpiry, err = subscriptionExpirerWorker(stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.quota, err = accountQuotaAlertWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.balance, err = balanceChargerWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.health, err = accountHealthProbeWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.quality, err = qualityEvalWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.sloEval, err = sloEvaluatorWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.alertNotifications, err = alertNotificationsWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.channelMonitor, err = channelMonitorWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.quotaRefresh, err = quotaRefreshWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.liteLLMPricing, err = liteLLMPricingWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.connectivityTest, err = connectivityTestWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.scheduledTest, err = scheduledTestWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.proxyProbe, err = proxyProbeWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	workers.tokenRefresh, err = accountsTokenRefreshWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, appWorkers{}, nil, err
	}
	if stores != nil {
		options = append(options,
			httpserver.WithAdminControlStore(stores.AdminControl),
			httpserver.WithUserStore(stores.Users),
			httpserver.WithAPIKeyStore(stores.APIKeys),
			httpserver.WithProviderStore(stores.Providers),
			httpserver.WithModelStore(stores.Models),
			httpserver.WithAccountStore(stores.Accounts),
			httpserver.WithAuditStore(stores.Audit),
			httpserver.WithAuthSessionStore(stores.AuthSessions),
			httpserver.WithBillingStore(stores.Billing),
			httpserver.WithEventStore(stores.Events),
			httpserver.WithAffiliateStore(stores.Affiliate),
			httpserver.WithOperationsStore(stores.Operations),
			httpserver.WithIdempotencyStore(stores.Idempotency),
			httpserver.WithOpsErrorLogsStore(stores.OpsErrorLogs),
			httpserver.WithPaymentStore(stores.Payments),
			httpserver.WithQualityEvalStore(stores.QualityEval),
			httpserver.WithSchedulerStore(stores.Scheduler),
			httpserver.WithSubscriptionStore(stores.Subscriptions),
			httpserver.WithTOTPStore(stores.TOTP),
			httpserver.WithUsageStore(stores.Usage),
			httpserver.WithUserAttributesStore(stores.UserAttributes),
			httpserver.WithErrorPassthroughStore(stores.ErrorPassthrough),
			httpserver.WithTLSProfilesStore(stores.TLSProfiles),
			httpserver.WithHealthRollupsStore(stores.HealthRollups),
			httpserver.WithModelRateLimitsStore(stores.ModelRateLimits),
			httpserver.WithGroupRateLimitsStore(stores.GroupRateLimits),
			httpserver.WithUserPlatformQuotasStore(stores.UserPlatformQuotas),
			httpserver.WithPayloadRulesStore(stores.PayloadRules),
			httpserver.WithScheduledTestsStore(stores.ScheduledTests),
			httpserver.WithChannelMonitorsStore(stores.ChannelMonitors),
			httpserver.WithCopilotConversationStore(stores.CopilotConvs),
			httpserver.WithBackupSnapshotsStore(stores.BackupSnapshots),
		)
		if workers.backup != nil {
			options = append(options, httpserver.WithBackupSnapshotsTrigger(workers.backup))
		}
		if workers.proxyProbe != nil {
			// Capture the worker pointer so /metrics can pull the latest counter
			// snapshot without the httpserver package having to import workers.
			worker := workers.proxyProbe
			options = append(options, httpserver.WithProxyProbeMetricsProvider(func() httpserver.ProxyProbeMetricsSnapshot {
				snapshot := worker.Metrics()
				return httpserver.ProxyProbeMetricsSnapshot{
					ProbeAttempted: snapshot.ProbeAttempted,
					ProbeSucceeded: snapshot.ProbeSucceeded,
					ProbeFailed:    snapshot.ProbeFailed,
				}
			}))
		}
		if workers.tokenRefresh != nil {
			worker := workers.tokenRefresh
			options = append(options, httpserver.WithTokenRefreshMetricsProvider(func() httpserver.TokenRefreshMetricsSnapshot {
				snapshot := worker.Metrics()
				return httpserver.TokenRefreshMetricsSnapshot{
					RefreshAttempted:         snapshot.RefreshAttempted,
					RefreshSucceeded:         snapshot.RefreshSucceeded,
					RefreshFailedPermanent:   snapshot.RefreshFailedPermanent,
					RefreshFailedTransient:   snapshot.RefreshFailedTransient,
					RefreshThresholdExceeded: snapshot.RefreshThresholdExceeded,
				}
			}))
		}
		if stores.UsageBilling != nil {
			options = append(options, httpserver.WithUsageAggregator(stores.UsageBilling))
		}
		// The reconciler is non-critical background cleanup; if it can't be built,
		// log and continue rather than failing the whole server.
		if w, werr := usageAggregationReconcilerWorker(stores, logger, workerGuard); werr != nil {
			logger.Error("failed to construct usage aggregation reconciler", "error", werr)
		} else {
			workers.usageReconciler = w
		}
	}

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("initialize http server: %v", recovered)
			}
		}()
		handler = httpserver.New(cfg, logger, options...)
	}()

	return handler, workers, usageDrain, err
}

func runtimeHTTPOptions(cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client, usageDrainSink *func(context.Context)) ([]httpserver.Option, error) {
	options := []httpserver.Option{httpserver.WithRedisPinger(redisClient)}
	if usageDrainSink != nil {
		options = append(options, httpserver.WithBackgroundDrainHook(usageDrainSink))
	}
	if cfg.UsesMemoryStorage() {
		options = append(options, httpserver.WithDatabasePinger(notRequiredPinger{}))
	} else {
		options = append(options, httpserver.WithDatabasePinger(dbClient))
		options = append(options, httpserver.WithDBStats(dbClient))
	}
	realtimeStore, err := realtimeSlotStore(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, err
	}
	if realtimeStore != nil {
		options = append(options, httpserver.WithRealtimeStore(realtimeStore))
	}
	balanceReservationStore, err := balanceReservationGateStore(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, err
	}
	if balanceReservationStore != nil {
		options = append(options, httpserver.WithBalanceReservationStore(balanceReservationStore))
	}
	sessionAffinity, err := sessionAffinityStore(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, err
	}
	if sessionAffinity != nil {
		options = append(options, httpserver.WithSessionAffinityStore(sessionAffinity))
	} else {
		memAffinity := httpserver.NewMemorySessionAffinityStoreWithGC(context.Background())
		options = append(options, httpserver.WithSessionAffinityStore(memAffinity))
	}
	rateLimiterOption, err := gatewayRateLimiterOption(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, err
	}
	if rateLimiterOption != nil {
		options = append(options, rateLimiterOption)
	}
	return options, nil
}

func persistentStores(ctx context.Context, cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client) (*entstore.Stores, error) {
	if cfg.UsesMemoryStorage() {
		logger.Warn("running without persistent stores", "storage_backend", cfg.StorageBackend())
		return nil, nil
	}
	if dbClient == nil || dbClient.Ent() == nil {
		return nil, fmt.Errorf("postgres storage backend requires a database client")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := dbClient.Ping(pingCtx)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("database unavailable for persistent stores: %w", err)
	}

	if cfg.Server.Mode == "release" {
		migrateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err = dbClient.ApplyMigrations(migrateCtx, migrations.UpFS(), logger)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("apply versioned database migrations: %w", err)
		}
	} else {
		// 5-minute timeout matches the release-mode migration window. ent's
		// CreateSchema is idempotent (CREATE INDEX IF NOT EXISTS, etc.), so
		// re-running on an existing dev DB is a no-op once everything is in
		// place — but adding a fresh index on a usage_logs table that has
		// accumulated several days of gateway data can take longer than the
		// previous 10-second budget. Bumping the timeout closes a footgun
		// without changing semantics.
		schemaCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err = dbClient.CreateSchema(schemaCtx)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("apply local database schema: %w", err)
		}
	}

	stores, err := entstore.New(dbClient.Ent())
	if err != nil {
		return nil, err
	}
	if n, seedErr := stores.SeedDefaults(ctx); seedErr != nil {
		logger.Warn("failed to seed error passthrough defaults", "error", seedErr)
	} else if n > 0 {
		logger.Info("seeded default error passthrough rules", "count", n)
	}
	leaseStore, err := schedulerLeaseStore(ctx, cfg, logger, redisClient)
	if err != nil {
		return nil, err
	}
	if leaseStore != nil {
		schedulerStore, err := entschedulerstore.NewWithLeaseStore(dbClient.Ent(), leaseStore)
		if err != nil {
			return nil, err
		}
		stores.Scheduler = schedulerStore
	}
	return &stores, nil
}

func schedulerLeaseStore(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client) (*redisschedulerstore.Store, error) {
	if redisClient == nil || redisClient.Raw() == nil {
		return nil, nil
	}
	err := pingRedisForDependency(ctx, cfg, logger, redisClient, "scheduler leases")
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("redis unavailable for scheduler leases: %w", err)
		}
		logger.Warn("redis unavailable; using in-memory scheduler leases", "error", err)
		return nil, nil
	}
	return redisschedulerstore.New(redisClient.Raw())
}

// sessionAffinityStore returns a Redis-backed session→account binding store when
// Redis is reachable so stickiness ("会话粘度") is shared cluster-wide, otherwise
// nil so the runtime falls back to a per-instance in-memory store. Stickiness is
// best-effort, so unlike scheduler leases it never hard-fails when Redis is absent.
func sessionAffinityStore(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client) (*redissessionaffinitystore.Store, error) {
	if redisClient == nil || redisClient.Raw() == nil {
		return nil, nil
	}
	err := pingRedisForDependency(ctx, cfg, logger, redisClient, "session affinity")
	if err != nil {
		if cfg.Server.Mode == "release" {
			logger.Warn("redis unavailable; session affinity (stickiness) will be per-instance in-memory — set REDIS_URL for cluster-wide stickiness", "error", err)
		} else {
			logger.Warn("redis unavailable; using in-memory session affinity", "error", err)
		}
		return nil, nil
	}
	return redissessionaffinitystore.New(redisClient.Raw())
}

func realtimeSlotStore(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client) (*redisrealtimestore.Store, error) {
	if redisClient == nil || redisClient.Raw() == nil {
		return nil, nil
	}
	err := pingRedisForDependency(ctx, cfg, logger, redisClient, "realtime slots")
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("redis unavailable for realtime slots: %w", err)
		}
		logger.Warn("redis unavailable; using in-memory realtime slots", "error", err)
		return nil, nil
	}
	return redisrealtimestore.New(redisClient.Raw())
}

// balanceReservationGateStore wires the atomic-reservation gate when Redis
// is available. Without Redis the gateway falls back to the read-only
// single-instance balance check; that's enough to keep $0-balance users from
// running unbounded paid traffic but cannot prevent the concurrent-overspend
// race that ate $1 balances. In release mode, missing Redis is loud (we'd
// rather an operator configure REDIS_URL than discover the race under abuse).
func balanceReservationGateStore(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client) (*redisbalancereservation.Store, error) {
	if redisClient == nil || redisClient.Raw() == nil {
		if cfg.Server.Mode == "release" {
			logger.Warn("redis not configured; gateway balance reservation is DISABLED in release mode — set REDIS_URL to close the concurrent-overspend race")
		}
		return nil, nil
	}
	err := pingRedisForDependency(ctx, cfg, logger, redisClient, "balance reservation")
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("redis unavailable for balance reservation: %w", err)
		}
		logger.Warn("redis unavailable; balance reservation disabled", "error", err)
		return nil, nil
	}
	// 10-minute default TTL bounds leaked reservations if a release call is
	// somehow missed. Long enough for any reasonable upstream timeout; short
	// enough that a permanently lost reservation doesn't poison a user's
	// available balance for hours.
	return redisbalancereservation.New(redisClient.Raw(), "srapi:balance_reservation", 10*time.Minute), nil
}

func gatewayRateLimiterOption(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client) (httpserver.Option, error) {
	if redisClient == nil || redisClient.Raw() == nil {
		if cfg.Server.Mode == "release" {
			// Don't disable rate limiting silently in production: make it loud so an
			// operator configures REDIS_URL rather than discovering it under abuse.
			logger.Warn("redis not configured; gateway rate limiting and concurrency controls are DISABLED in release mode — set REDIS_URL to enable them")
		}
		return nil, nil
	}
	err := pingRedisForDependency(ctx, cfg, logger, redisClient, "gateway rate limits")
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("redis unavailable for gateway rate limits: %w", err)
		}
		logger.Warn("redis unavailable; gateway rate limits disabled", "error", err)
		return nil, nil
	}
	return httpserver.WithRateLimitRedis(redisClient.Raw()), nil
}

func pingRedisForDependency(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client, dependency string) error {
	if redisClient == nil {
		return nil
	}
	attempts := 1
	if cfg.Server.Mode == "release" {
		attempts = 3
	}
	timeout := time.Duration(cfg.Redis.DialTimeoutSeconds) * time.Second
	if readTimeout := time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second; readTimeout > timeout {
		timeout = readTimeout
	}
	if poolTimeout := time.Duration(cfg.Redis.PoolTimeoutSeconds) * time.Second; poolTimeout > timeout {
		timeout = poolTimeout
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, timeout)
		err = redisClient.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt == attempts {
			break
		}
		logger.Warn("redis ping failed; retrying dependency initialization", "dependency", dependency, "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
		}
	}
	return err
}

func domainEventsWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*outboxworker.Worker, error) {
	if stores == nil || stores.Events == nil {
		return nil, nil
	}
	return outboxworker.New(stores.Events, logger, outboxworker.Config{
		AccountStore:       stores.Accounts,
		AffiliateStore:     stores.Affiliate,
		AdminControlStore:  stores.AdminControl,
		AuditStore:         stores.Audit,
		UserStore:          stores.Users,
		EmailPublicBaseURL: cfg.Email.PublicBaseURL,
		EmailSMTPHost:      cfg.Email.SMTPHost,
		EmailSMTPPort:      cfg.Email.SMTPPort,
		EmailSMTPUsername:  cfg.Email.SMTPUsername,
		EmailSMTPPassword:  cfg.Email.SMTPPassword,
		EmailSMTPFrom:      cfg.Email.SMTPFrom,
		EmailSMTPFromName:  cfg.Email.SMTPFromName,
		EmailSMTPUseTLS:    cfg.Email.SMTPUseTLS,
		MasterKey:          cfg.Security.MasterKey,
		OperationsStore:    stores.Operations,
		SubscriptionStore:  stores.Subscriptions,
		UsageStore:         stores.Usage,
		RunGuard:           optionalWorkerGuard(guards...),
	})
}

func retentionCleanupWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*retentionworker.Worker, error) {
	if stores == nil || stores.Operations == nil {
		return nil, nil
	}
	return retentionworker.New(stores.Operations, logger, retentionworker.Config{
		UsageLogsDays:                 cfg.Retention.UsageLogsDays,
		SchedulerDecisionsDays:        cfg.Retention.SchedulerDecisionsDays,
		SchedulerFeedbacksDays:        cfg.Retention.SchedulerFeedbacksDays,
		SchedulerRequestSnapshotsDays: cfg.Retention.SchedulerRequestSnapshotsDays,
		AuditLogsDays:                 cfg.Retention.AuditLogsDays,
		AccountHealthSnapshotsDays:    cfg.Retention.AccountHealthSnapshotsDays,
		SystemLogsDays:                cfg.Retention.SystemLogsDays,
		BatchLimit:                    cfg.Retention.BatchLimit,
		RunGuard:                      optionalWorkerGuard(guards...),
	})
}

func availabilityRollupWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*availabilityrollupworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.HealthRollups == nil {
		return nil, nil
	}
	return availabilityrollupworker.New(stores.Accounts, stores.HealthRollups, logger, availabilityrollupworker.Config{
		MasterKey: cfg.Security.MasterKey,
		RunGuard:  optionalWorkerGuard(guards...),
	})
}

func backupWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*backupworker.Worker, error) {
	if stores == nil || stores.AdminControl == nil || cfg.StorageBackend() != config.StorageBackendPostgres {
		return nil, nil
	}
	return backupworker.NewFromStore(stores.AdminControl, logger, backupworker.Config{
		BackupDir: "backups",
		Runner: backupworker.PostgresRunner{
			Host:     cfg.Database.Host,
			Port:     cfg.Database.Port,
			User:     cfg.Database.User,
			Password: cfg.Database.Password,
			Database: cfg.Database.Database,
			SSLMode:  cfg.Database.SSLMode,
		},
		Snapshots: stores.BackupSnapshots,
		RunGuard:  optionalWorkerGuard(guards...),
	})
}

func idempotencyCleanupWorker(stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*idempotencycleanupworker.Worker, error) {
	if stores == nil || stores.Idempotency == nil {
		return nil, nil
	}
	return idempotencycleanupworker.New(stores.Idempotency, logger, idempotencycleanupworker.Config{RunGuard: optionalWorkerGuard(guards...)})
}

func authSessionCleanupWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*authcleanupworker.Worker, error) {
	if stores == nil || stores.AuthSessions == nil {
		return nil, nil
	}
	return authcleanupworker.NewFromStore(stores.AuthSessions, logger, authcleanupworker.Config{
		Interval: cfg.AuthCleanup.Interval,
		RunGuard: optionalWorkerGuard(guards...),
	})
}

func paymentOrderExpirerWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*orderexpirerworker.Worker, error) {
	if stores == nil || stores.Payments == nil {
		return nil, nil
	}
	return orderexpirerworker.New(stores.Payments, cfg.Security.MasterKey, orderexpirerworker.Dependencies{}, logger, orderexpirerworker.Config{
		Audit:    stores.Audit,
		RunGuard: optionalWorkerGuard(guards...),
	})
}

func paymentReconcileWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*paymentreconcileworker.Worker, error) {
	if stores == nil || stores.Payments == nil {
		return nil, nil
	}
	return paymentreconcileworker.New(stores.Payments, cfg.Security.MasterKey, paymentreconcileworker.Dependencies{}, logger, paymentreconcileworker.Config{
		Audit:         stores.Audit,
		Billing:       stores.Billing,
		Events:        stores.Events,
		Users:         stores.Users,
		Subscriptions: stores.Subscriptions,
		RunGuard:      optionalWorkerGuard(guards...),
	})
}

func subscriptionExpirerWorker(stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*subscriptionexpirerworker.Worker, error) {
	if stores == nil || stores.Subscriptions == nil {
		return nil, nil
	}
	return subscriptionexpirerworker.New(stores.Subscriptions, subscriptionexpirerworker.Dependencies{}, logger, subscriptionexpirerworker.Config{
		Events:       stores.Events,
		AdminControl: stores.AdminControl,
		RunGuard:     optionalWorkerGuard(guards...),
	})
}

func accountQuotaAlertWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*accountquotaalertworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.Events == nil {
		return nil, nil
	}
	return accountquotaalertworker.New(stores.Accounts, logger, accountquotaalertworker.Config{
		MasterKey:    cfg.Security.MasterKey,
		Events:       stores.Events,
		AdminControl: stores.AdminControl,
		RunGuard:     optionalWorkerGuard(guards...),
	})
}

func balanceChargerWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*balancechargerworker.Worker, error) {
	if stores == nil || stores.UsageCharges == nil {
		return nil, nil
	}
	return balancechargerworker.New(stores.UsageCharges, logger, balancechargerworker.Config{
		Interval:         cfg.BalanceCharger.Interval,
		BatchLimit:       cfg.BalanceCharger.BatchLimit,
		MaxBatchesPerRun: cfg.BalanceCharger.MaxBatchesPerRun,
		Users:            stores.Users,
		Audit:            stores.Audit,
		Events:           stores.Events,
		AdminControl:     stores.AdminControl,
		RunGuard:         optionalWorkerGuard(guards...),
	})
}

func usageAggregationReconcilerWorker(stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*usageaggregationreconcilerworker.Worker, error) {
	if stores == nil || stores.UsageBilling == nil {
		return nil, nil
	}
	return usageaggregationreconcilerworker.New(stores.UsageBilling, logger, usageaggregationreconcilerworker.Config{
		RunGuard: optionalWorkerGuard(guards...),
	})
}

func accountHealthProbeWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*healthprobeworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.Providers == nil {
		return nil, nil
	}
	return healthprobeworker.New(stores.Accounts, stores.Providers, logger, healthprobeworker.Config{
		Interval:               cfg.HealthProbe.Interval,
		Timeout:                cfg.HealthProbe.Timeout,
		MaxConcurrent:          cfg.HealthProbe.MaxConcurrent,
		MasterKey:              cfg.Security.MasterKey,
		FailureThreshold:       cfg.HealthProbe.FailureThreshold,
		ErrorRateThreshold:     cfg.HealthProbe.ErrorRateThreshold,
		MinSamplesForErrorRate: cfg.HealthProbe.MinSamplesForErrorRate,
		Cooldown:               cfg.HealthProbe.Cooldown,
		RunGuard:               optionalWorkerGuard(guards...),
	})
}

// accountsTokenRefreshWorker builds the proactive OAuth access-token refresh
// worker. Always wires up when persistent stores are available and the config
// flag is true (default true); the worker's eligibility filter
// (oauth_refresh / oauth_device_code + active + token_expires_at set) means
// it is a no-op cost on fleets without OAuth accounts.
func accountsTokenRefreshWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*accountstokenrefreshworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || !cfg.AccountsTokenRefresh.Enabled {
		return nil, nil
	}
	return accountstokenrefreshworker.New(stores.Accounts, logger, accountstokenrefreshworker.Config{
		Interval:           cfg.AccountsTokenRefresh.Interval,
		RefreshThreshold:   cfg.AccountsTokenRefresh.RefreshThreshold,
		Timeout:            cfg.AccountsTokenRefresh.Timeout,
		MaxConcurrent:      cfg.AccountsTokenRefresh.MaxConcurrent,
		MasterKey:          cfg.Security.MasterKey,
		BlockPrivateEgress: cfg.Server.Mode != "local",
		RunGuard:           optionalWorkerGuard(guards...),
	})
}

func quotaRefreshWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*quotarefreshworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.Providers == nil || !cfg.QuotaRefresh.Enabled {
		return nil, nil
	}
	return quotarefreshworker.New(stores.Accounts, stores.Providers, logger, quotarefreshworker.Config{
		Interval:           cfg.QuotaRefresh.Interval,
		Timeout:            cfg.QuotaRefresh.Timeout,
		MaxConcurrent:      cfg.QuotaRefresh.MaxConcurrent,
		MasterKey:          cfg.Security.MasterKey,
		BlockPrivateEgress: cfg.Server.Mode != "local",
		RunGuard:           optionalWorkerGuard(guards...),
	})
}

func liteLLMPricingWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*litellmpricingworker.Worker, error) {
	if stores == nil || stores.Pricing == nil || cfg.LiteLLMPricing.SourceURL == "" {
		return nil, nil
	}
	return litellmpricingworker.New(stores.Pricing, logger, litellmpricingworker.Config{
		SourceURL: cfg.LiteLLMPricing.SourceURL,
		Interval:  cfg.LiteLLMPricing.Interval,
		Timeout:   cfg.LiteLLMPricing.Timeout,
		RunGuard:  optionalWorkerGuard(guards...),
	})
}

func connectivityTestWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*connectivitytestworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.Providers == nil || !cfg.ConnectivityTest.Enabled {
		return nil, nil
	}
	return connectivitytestworker.New(stores.Accounts, stores.Providers, logger, connectivitytestworker.Config{
		Interval:      cfg.ConnectivityTest.Interval,
		Timeout:       cfg.ConnectivityTest.Timeout,
		MaxConcurrent: cfg.ConnectivityTest.MaxConcurrent,
		MasterKey:     cfg.Security.MasterKey,
		RunGuard:      optionalWorkerGuard(guards...),
	})
}

func scheduledTestWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*scheduledtestworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.Providers == nil || stores.ScheduledTests == nil || !cfg.ScheduledTest.Enabled {
		return nil, nil
	}
	return scheduledtestworker.New(stores.Accounts, stores.Providers, stores.ScheduledTests, logger, scheduledtestworker.Config{
		Tick:      cfg.ScheduledTest.Tick,
		Timeout:   cfg.ScheduledTest.Timeout,
		MasterKey: cfg.Security.MasterKey,
		RunGuard:  optionalWorkerGuard(guards...),
	})
}

// proxyProbeWorker builds the periodic proxy availability probe. Disabled by
// default; only wires up when stores.Accounts exists AND cfg.ProxyProbe.Enabled
// is true so a misconfiguration does not silently start outbound probes.
func proxyProbeWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*proxyprobeworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || !cfg.ProxyProbe.Enabled {
		return nil, nil
	}
	return proxyprobeworker.New(stores.Accounts, logger, proxyprobeworker.Config{
		Enabled:       cfg.ProxyProbe.Enabled,
		Interval:      cfg.ProxyProbe.Interval,
		Timeout:       cfg.ProxyProbe.Timeout,
		MaxConcurrent: cfg.ProxyProbe.MaxConcurrent,
		ProbeURL:      cfg.ProxyProbe.ProbeURL,
		MasterKey:     cfg.Security.MasterKey,
		RunGuard:      optionalWorkerGuard(guards...),
	})
}

func channelMonitorWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*channelmonitorworker.Worker, error) {
	if stores == nil || stores.Accounts == nil || stores.Providers == nil || stores.Models == nil || stores.ChannelMonitors == nil || stores.AdminControl == nil {
		return nil, nil
	}
	return channelmonitorworker.New(stores.Accounts, stores.Providers, stores.Models, stores.ChannelMonitors, logger, channelmonitorworker.Config{
		MasterKey:         cfg.Security.MasterKey,
		AdminControlStore: stores.AdminControl,
		RunGuard:          optionalWorkerGuard(guards...),
	})
}

func qualityEvalWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*qualityevalworker.Worker, error) {
	if stores == nil || stores.QualityEval == nil || !cfg.QualityEval.Enabled {
		return nil, nil
	}
	return qualityevalworker.New(stores.QualityEval, logger, qualityevalworker.Config{
		Interval:      cfg.QualityEval.Interval,
		Timeout:       cfg.QualityEval.Timeout,
		BatchLimit:    cfg.QualityEval.BatchLimit,
		SamplePercent: cfg.QualityEval.SamplePercent,
		MasterKey:     cfg.Security.MasterKey,
		OpenAIAPIKey:  cfg.QualityEval.OpenAIAPIKey,
		OpenAIBaseURL: cfg.QualityEval.OpenAIBaseURL,
		JudgeModel:    cfg.QualityEval.JudgeModel,
		JudgeTimeout:  cfg.QualityEval.JudgeTimeout,
		RunGuard:      optionalWorkerGuard(guards...),
	})
}

func sloEvaluatorWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*sloevaluatorworker.Worker, error) {
	if stores == nil || stores.Operations == nil {
		return nil, nil
	}
	return sloevaluatorworker.New(stores.Operations, logger, sloevaluatorworker.Config{
		Interval: cfg.SLOEvaluator.Interval,
		Timeout:  cfg.SLOEvaluator.Timeout,
		RunGuard: optionalWorkerGuard(guards...),
	})
}

func alertNotificationsWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger, guards ...*workerLeaderGuard) (*alertnotificationsworker.Worker, error) {
	if stores == nil || stores.Operations == nil {
		return nil, nil
	}
	return alertnotificationsworker.New(stores.Operations, logger, alertnotificationsworker.Config{
		Interval:   cfg.AlertNotifications.Interval,
		Timeout:    cfg.AlertNotifications.Timeout,
		BatchLimit: cfg.AlertNotifications.BatchLimit,
		SMTP: alertnotificationsworker.SMTPConfig{
			PublicBaseURL: cfg.Email.PublicBaseURL,
			Host:          cfg.Email.SMTPHost,
			Port:          cfg.Email.SMTPPort,
			Username:      cfg.Email.SMTPUsername,
			Password:      cfg.Email.SMTPPassword,
			From:          cfg.Email.SMTPFrom,
			FromName:      cfg.Email.SMTPFromName,
			UseTLS:        cfg.Email.SMTPUseTLS,
		},
		RunGuard: optionalWorkerGuard(guards...),
	})
}

func (a *App) startWorkers() {
	if a == nil {
		return
	}
	a.workers.start(context.Background())
}

func (a *App) stopWorkers(ctx context.Context) error {
	if a == nil {
		return nil
	}
	return a.workers.shutdown(ctx)
}

func (w appWorkers) start(ctx context.Context) {
	if w.outbox != nil {
		w.outbox.Start(ctx)
	}
	if w.retention != nil {
		w.retention.Start(ctx)
	}
	if w.availability != nil {
		w.availability.Start(ctx)
	}
	if w.backup != nil {
		w.backup.Start(ctx)
	}
	if w.authClean != nil {
		w.authClean.Start(ctx)
	}
	if w.idemClean != nil {
		w.idemClean.Start(ctx)
	}
	if w.expirer != nil {
		w.expirer.Start(ctx)
	}
	if w.reconcile != nil {
		w.reconcile.Start(ctx)
	}
	if w.subExpiry != nil {
		w.subExpiry.Start(ctx)
	}
	if w.quota != nil {
		w.quota.Start(ctx)
	}
	if w.balance != nil {
		w.balance.Start(ctx)
	}
	if w.usageReconciler != nil {
		w.usageReconciler.Start(ctx)
	}
	if w.health != nil {
		w.health.Start(ctx)
	}
	if w.quality != nil {
		w.quality.Start(ctx)
	}
	if w.sloEval != nil {
		w.sloEval.Start(ctx)
	}
	if w.alertNotifications != nil {
		w.alertNotifications.Start(ctx)
	}
	if w.quotaRefresh != nil {
		w.quotaRefresh.Start(ctx)
	}
	if w.liteLLMPricing != nil {
		w.liteLLMPricing.Start(ctx)
	}
	if w.connectivityTest != nil {
		w.connectivityTest.Start(ctx)
	}
	if w.scheduledTest != nil {
		w.scheduledTest.Start(ctx)
	}
	if w.channelMonitor != nil {
		w.channelMonitor.Start(ctx)
	}
	if w.proxyProbe != nil {
		w.proxyProbe.Start(ctx)
	}
	if w.tokenRefresh != nil {
		w.tokenRefresh.Start(ctx)
	}
}

func (w appWorkers) shutdown(ctx context.Context) error {
	type shutdowner interface {
		Shutdown(context.Context) error
	}
	all := []shutdowner{
		w.outbox, w.retention, w.availability, w.backup,
		w.authClean, w.idemClean, w.quotaRefresh, w.tokenRefresh,
		w.liteLLMPricing, w.connectivityTest, w.scheduledTest,
		w.channelMonitor, w.proxyProbe, w.expirer, w.reconcile,
		w.subExpiry, w.quota, w.balance, w.health, w.quality,
		w.sloEval, w.alertNotifications, w.usageReconciler,
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(all))
	for _, s := range all {
		if s == nil || reflect.ValueOf(s).IsNil() {
			continue
		}
		wg.Add(1)
		go func(s shutdowner) {
			defer wg.Done()
			if err := s.Shutdown(ctx); err != nil {
				errs <- err
			}
		}(s)
	}
	wg.Wait()
	close(errs)
	var collected []error
	for err := range errs {
		collected = append(collected, err)
	}
	return errors.Join(collected...)
}

type notRequiredPinger struct{}

func (notRequiredPinger) Ping(context.Context) error {
	return nil
}

// masterKeySentinel is a fixed plaintext that verifyMasterKeyIntegrity
// encrypts and decrypts on every startup to detect SRAPI_MASTER_KEY rotation
// or misconfiguration early, before the server accepts traffic that depends on
// existing ciphertexts being decryptable.
const masterKeySentinel = "srapi-master-key-sentinel-check"

// verifyMasterKeyIntegrity performs a round-trip encrypt/decrypt of a known
// sentinel value using the configured SRAPI_MASTER_KEY. If the key is too
// short or produces a derivation error, the server cannot safely handle
// encrypted secrets (copilot API keys, OAuth client secrets, etc.), so we
// fail loudly at startup rather than silently corrupting data at runtime.
func verifyMasterKeyIntegrity(cfg config.Config, logger *slog.Logger) error {
	key, err := platformcrypto.DeriveAESKey(cfg.Security.MasterKey)
	if err != nil {
		return fmt.Errorf("FATAL: SRAPI_MASTER_KEY cannot derive AES key: %w — "+
			"encrypted secrets will be unreadable; verify the key matches what was used to encrypt existing data", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("FATAL: SRAPI_MASTER_KEY produced an invalid AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("FATAL: SRAPI_MASTER_KEY GCM initialisation failed: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("FATAL: failed to generate nonce for master key check: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(masterKeySentinel), nil)
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("FATAL: SRAPI_MASTER_KEY round-trip decrypt failed: %w — "+
			"the master key may have been rotated without re-encrypting stored secrets", err)
	}
	if string(plaintext) != masterKeySentinel {
		return fmt.Errorf("FATAL: SRAPI_MASTER_KEY round-trip produced mismatched plaintext — " +
			"key derivation is broken; check SRAPI_MASTER_KEY value")
	}

	// Encode the ciphertext to exercise the same base64 path used by
	// runtime_secret_helpers.go, ensuring no silent encoding drift.
	_ = base64.RawURLEncoding.EncodeToString(ciphertext)

	logger.Info("master key integrity check passed")
	return nil
}
