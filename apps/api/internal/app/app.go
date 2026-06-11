package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	"github.com/srapi/srapi/apps/api/internal/httpserver"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"
	entschedulerstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/scheduler"
	redisrealtimestore "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/realtime"
	redisschedulerstore "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/scheduler"
	redissessionaffinitystore "github.com/srapi/srapi/apps/api/internal/persistence/redisstore/sessionaffinity"
	platformdb "github.com/srapi/srapi/apps/api/internal/platform/db"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	platformredis "github.com/srapi/srapi/apps/api/internal/platform/redis"
	accountquotaalertworker "github.com/srapi/srapi/apps/api/internal/workers/account_quota_alert"
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
	qualityevalworker "github.com/srapi/srapi/apps/api/internal/workers/quality_eval"
	quotarefreshworker "github.com/srapi/srapi/apps/api/internal/workers/quota_refresh"
	retentionworker "github.com/srapi/srapi/apps/api/internal/workers/retention"
	scheduledtestworker "github.com/srapi/srapi/apps/api/internal/workers/scheduled_test"
	sloevaluatorworker "github.com/srapi/srapi/apps/api/internal/workers/slo_evaluator"
	subscriptionexpirerworker "github.com/srapi/srapi/apps/api/internal/workers/subscription_expirer"
	"github.com/srapi/srapi/apps/api/migrations"
)

const defaultReadHeaderTimeout = 10 * time.Second

type App struct {
	cfg              config.Config
	logger           *slog.Logger
	server           *http.Server
	db               *platformdb.Client
	redis            *platformredis.Client
	tracer           platformotel.ShutdownFunc
	outbox           *outboxworker.Worker
	retention        *retentionworker.Worker
	availability     *availabilityrollupworker.Worker
	backup           *backupworker.Worker
	authClean        *authcleanupworker.Worker
	idemClean        *idempotencycleanupworker.Worker
	quotaRefresh     *quotarefreshworker.Worker
	liteLLMPricing   *litellmpricingworker.Worker
	connectivityTest *connectivitytestworker.Worker
	scheduledTest    *scheduledtestworker.Worker
	channelMonitor   *channelmonitorworker.Worker
	expirer          *orderexpirerworker.Worker
	reconcile        *paymentreconcileworker.Worker
	subExpiry        *subscriptionexpirerworker.Worker
	quota            *accountquotaalertworker.Worker
	balance          *balancechargerworker.Worker
	health           *healthprobeworker.Worker
	quality          *qualityevalworker.Worker
	sloEval          *sloevaluatorworker.Worker
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
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

	handler, outbox, retention, availability, backup, authClean, expirer, reconcile, subExpiry, quota, balance, health, quality, sloEval, idemClean, quotaRefresh, liteLLMPricing, connectivityTest, scheduledTest, channelMonitor, err := newHandler(cfg, logger, dbClient, redisClient)
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
	}
	return &App{
		cfg:              cfg,
		logger:           logger,
		server:           server,
		db:               dbClient,
		redis:            redisClient,
		tracer:           tracerShutdown,
		outbox:           outbox,
		retention:        retention,
		availability:     availability,
		backup:           backup,
		authClean:        authClean,
		idemClean:        idemClean,
		quotaRefresh:     quotaRefresh,
		liteLLMPricing:   liteLLMPricing,
		connectivityTest: connectivityTest,
		scheduledTest:    scheduledTest,
		channelMonitor:   channelMonitor,
		expirer:          expirer,
		reconcile:        reconcile,
		subExpiry:        subExpiry,
		quota:            quota,
		balance:          balance,
		health:           health,
		quality:          quality,
		sloEval:          sloEval,
	}, nil
}

func (a *App) Serve() error {
	a.startWorkers()
	defer func() {
		_ = a.stopWorkers(context.Background())
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

func newHandler(cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client) (http.Handler, *outboxworker.Worker, *retentionworker.Worker, *availabilityrollupworker.Worker, *backupworker.Worker, *authcleanupworker.Worker, *orderexpirerworker.Worker, *paymentreconcileworker.Worker, *subscriptionexpirerworker.Worker, *accountquotaalertworker.Worker, *balancechargerworker.Worker, *healthprobeworker.Worker, *qualityevalworker.Worker, *sloevaluatorworker.Worker, *idempotencycleanupworker.Worker, *quotarefreshworker.Worker, *litellmpricingworker.Worker, *connectivitytestworker.Worker, *scheduledtestworker.Worker, *channelmonitorworker.Worker, error) {
	var (
		handler http.Handler
		err     error
	)

	options := []httpserver.Option{httpserver.WithRedisPinger(redisClient)}
	if cfg.UsesMemoryStorage() {
		options = append(options, httpserver.WithDatabasePinger(notRequiredPinger{}))
	} else {
		options = append(options, httpserver.WithDatabasePinger(dbClient))
	}
	realtimeStore, err := realtimeSlotStore(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	if realtimeStore != nil {
		options = append(options, httpserver.WithRealtimeStore(realtimeStore))
	}
	sessionAffinity, err := sessionAffinityStore(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	if sessionAffinity != nil {
		options = append(options, httpserver.WithSessionAffinityStore(sessionAffinity))
	} else {
		// No Redis: fall back to per-instance in-memory stickiness (best-effort).
		options = append(options, httpserver.WithSessionAffinityStore(httpserver.NewMemorySessionAffinityStore()))
	}
	rateLimiterOption, err := gatewayRateLimiterOption(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	if rateLimiterOption != nil {
		options = append(options, rateLimiterOption)
	}
	stores, err := persistentStores(context.Background(), cfg, logger, dbClient, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	workerGuard, err := newWorkerLeaderGuard(dbClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	outbox, err := domainEventsWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	retention, err := retentionCleanupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	availability, err := availabilityRollupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	backup, err := backupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	authClean, err := authSessionCleanupWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	idemClean, err := idempotencyCleanupWorker(stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	expirer, err := paymentOrderExpirerWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	reconcile, err := paymentReconcileWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	subExpiry, err := subscriptionExpirerWorker(stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	quota, err := accountQuotaAlertWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	balance, err := balanceChargerWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	health, err := accountHealthProbeWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	quality, err := qualityEvalWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	sloEval, err := sloEvaluatorWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	channelMonitor, err := channelMonitorWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	quotaRefresh, err := quotaRefreshWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	liteLLMPricing, err := liteLLMPricingWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	connectivityTest, err := connectivityTestWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	scheduledTest, err := scheduledTestWorker(cfg, stores, logger, workerGuard)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
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
		)
	}

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("initialize http server: %v", recovered)
			}
		}()
		handler = httpserver.New(cfg, logger, options...)
	}()

	return handler, outbox, retention, availability, backup, authClean, expirer, reconcile, subExpiry, quota, balance, health, quality, sloEval, idemClean, quotaRefresh, liteLLMPricing, connectivityTest, scheduledTest, channelMonitor, err
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
		schemaCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
		UsageLogsDays:              cfg.Retention.UsageLogsDays,
		SchedulerDecisionsDays:     cfg.Retention.SchedulerDecisionsDays,
		SchedulerFeedbacksDays:     cfg.Retention.SchedulerFeedbacksDays,
		AuditLogsDays:              cfg.Retention.AuditLogsDays,
		AccountHealthSnapshotsDays: cfg.Retention.AccountHealthSnapshotsDays,
		BatchLimit:                 cfg.Retention.BatchLimit,
		RunGuard:                   optionalWorkerGuard(guards...),
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
		RunGuard: optionalWorkerGuard(guards...),
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

func (a *App) startWorkers() {
	if a == nil {
		return
	}
	if a.outbox != nil {
		a.outbox.Start(context.Background())
	}
	if a.retention != nil {
		a.retention.Start(context.Background())
	}
	if a.availability != nil {
		a.availability.Start(context.Background())
	}
	if a.backup != nil {
		a.backup.Start(context.Background())
	}
	if a.authClean != nil {
		a.authClean.Start(context.Background())
	}
	if a.idemClean != nil {
		a.idemClean.Start(context.Background())
	}
	if a.expirer != nil {
		a.expirer.Start(context.Background())
	}
	if a.reconcile != nil {
		a.reconcile.Start(context.Background())
	}
	if a.subExpiry != nil {
		a.subExpiry.Start(context.Background())
	}
	if a.quota != nil {
		a.quota.Start(context.Background())
	}
	if a.balance != nil {
		a.balance.Start(context.Background())
	}
	if a.health != nil {
		a.health.Start(context.Background())
	}
	if a.quality != nil {
		a.quality.Start(context.Background())
	}
	if a.sloEval != nil {
		a.sloEval.Start(context.Background())
	}
	if a.quotaRefresh != nil {
		a.quotaRefresh.Start(context.Background())
	}
	if a.liteLLMPricing != nil {
		a.liteLLMPricing.Start(context.Background())
	}
	if a.connectivityTest != nil {
		a.connectivityTest.Start(context.Background())
	}
	if a.scheduledTest != nil {
		a.scheduledTest.Start(context.Background())
	}
	if a.channelMonitor != nil {
		a.channelMonitor.Start(context.Background())
	}
}

func (a *App) stopWorkers(ctx context.Context) error {
	if a == nil {
		return nil
	}
	var errs []error
	if a.outbox != nil {
		errs = append(errs, a.outbox.Shutdown(ctx))
	}
	if a.retention != nil {
		errs = append(errs, a.retention.Shutdown(ctx))
	}
	if a.availability != nil {
		errs = append(errs, a.availability.Shutdown(ctx))
	}
	if a.backup != nil {
		errs = append(errs, a.backup.Shutdown(ctx))
	}
	if a.authClean != nil {
		errs = append(errs, a.authClean.Shutdown(ctx))
	}
	if a.idemClean != nil {
		errs = append(errs, a.idemClean.Shutdown(ctx))
	}
	if a.expirer != nil {
		errs = append(errs, a.expirer.Shutdown(ctx))
	}
	if a.reconcile != nil {
		errs = append(errs, a.reconcile.Shutdown(ctx))
	}
	if a.subExpiry != nil {
		errs = append(errs, a.subExpiry.Shutdown(ctx))
	}
	if a.quota != nil {
		errs = append(errs, a.quota.Shutdown(ctx))
	}
	if a.balance != nil {
		errs = append(errs, a.balance.Shutdown(ctx))
	}
	if a.health != nil {
		errs = append(errs, a.health.Shutdown(ctx))
	}
	if a.quality != nil {
		errs = append(errs, a.quality.Shutdown(ctx))
	}
	if a.sloEval != nil {
		errs = append(errs, a.sloEval.Shutdown(ctx))
	}
	if a.quotaRefresh != nil {
		errs = append(errs, a.quotaRefresh.Shutdown(ctx))
	}
	if a.liteLLMPricing != nil {
		errs = append(errs, a.liteLLMPricing.Shutdown(ctx))
	}
	if a.connectivityTest != nil {
		errs = append(errs, a.connectivityTest.Shutdown(ctx))
	}
	if a.scheduledTest != nil {
		errs = append(errs, a.scheduledTest.Shutdown(ctx))
	}
	if a.channelMonitor != nil {
		errs = append(errs, a.channelMonitor.Shutdown(ctx))
	}
	return errors.Join(errs...)
}

type dependencyPinger interface {
	Ping(context.Context) error
}

type notRequiredPinger struct{}

func (notRequiredPinger) Ping(context.Context) error {
	return nil
}
