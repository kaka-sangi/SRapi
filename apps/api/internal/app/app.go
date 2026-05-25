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
	platformdb "github.com/srapi/srapi/apps/api/internal/platform/db"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	platformredis "github.com/srapi/srapi/apps/api/internal/platform/redis"
	balancechargerworker "github.com/srapi/srapi/apps/api/internal/workers/balance_charger"
	healthprobeworker "github.com/srapi/srapi/apps/api/internal/workers/health_probe"
	orderexpirerworker "github.com/srapi/srapi/apps/api/internal/workers/order_expirer"
	outboxworker "github.com/srapi/srapi/apps/api/internal/workers/outbox"
	qualityevalworker "github.com/srapi/srapi/apps/api/internal/workers/quality_eval"
	retentionworker "github.com/srapi/srapi/apps/api/internal/workers/retention"
	subscriptionexpirerworker "github.com/srapi/srapi/apps/api/internal/workers/subscription_expirer"
)

const defaultReadHeaderTimeout = 10 * time.Second

type App struct {
	cfg       config.Config
	logger    *slog.Logger
	server    *http.Server
	db        *platformdb.Client
	redis     *platformredis.Client
	tracer    platformotel.ShutdownFunc
	outbox    *outboxworker.Worker
	retention *retentionworker.Worker
	expirer   *orderexpirerworker.Worker
	subExpiry *subscriptionexpirerworker.Worker
	balance   *balancechargerworker.Worker
	health    *healthprobeworker.Worker
	quality   *qualityevalworker.Worker
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

	handler, outbox, retention, expirer, subExpiry, balance, health, quality, err := newHandler(cfg, logger, dbClient, redisClient)
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
		cfg:       cfg,
		logger:    logger,
		server:    server,
		db:        dbClient,
		redis:     redisClient,
		tracer:    tracerShutdown,
		outbox:    outbox,
		retention: retention,
		expirer:   expirer,
		subExpiry: subExpiry,
		balance:   balance,
		health:    health,
		quality:   quality,
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

func Healthcheck(ctx context.Context, cfg config.Config) error {
	return httpserver.Healthcheck(ctx, cfg.HealthcheckAddress())
}

func newHandler(cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client) (http.Handler, *outboxworker.Worker, *retentionworker.Worker, *orderexpirerworker.Worker, *subscriptionexpirerworker.Worker, *balancechargerworker.Worker, *healthprobeworker.Worker, *qualityevalworker.Worker, error) {
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
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	if realtimeStore != nil {
		options = append(options, httpserver.WithRealtimeStore(realtimeStore))
	}
	rateLimiterOption, err := gatewayRateLimiterOption(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	if rateLimiterOption != nil {
		options = append(options, rateLimiterOption)
	}
	stores, err := persistentStores(context.Background(), cfg, logger, dbClient, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	outbox, err := domainEventsWorker(stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	retention, err := retentionCleanupWorker(cfg, stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	expirer, err := paymentOrderExpirerWorker(cfg, stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	subExpiry, err := subscriptionExpirerWorker(stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	balance, err := balanceChargerWorker(stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	health, err := accountHealthProbeWorker(cfg, stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	quality, err := qualityEvalWorker(cfg, stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, err
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
			httpserver.WithPaymentStore(stores.Payments),
			httpserver.WithQualityEvalStore(stores.QualityEval),
			httpserver.WithSchedulerStore(stores.Scheduler),
			httpserver.WithSubscriptionStore(stores.Subscriptions),
			httpserver.WithUsageStore(stores.Usage),
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

	return handler, outbox, retention, expirer, subExpiry, balance, health, quality, err
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

	if cfg.Server.Mode != "release" {
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
	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := redisClient.Ping(pingCtx)
	cancel()
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("redis unavailable for scheduler leases: %w", err)
		}
		logger.Warn("redis unavailable; using in-memory scheduler leases", "error", err)
		return nil, nil
	}
	return redisschedulerstore.New(redisClient.Raw())
}

func realtimeSlotStore(ctx context.Context, cfg config.Config, logger *slog.Logger, redisClient *platformredis.Client) (*redisrealtimestore.Store, error) {
	if redisClient == nil || redisClient.Raw() == nil {
		return nil, nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := redisClient.Ping(pingCtx)
	cancel()
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
		return nil, nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := redisClient.Ping(pingCtx)
	cancel()
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("redis unavailable for gateway rate limits: %w", err)
		}
		logger.Warn("redis unavailable; gateway rate limits disabled", "error", err)
		return nil, nil
	}
	return httpserver.WithRateLimitRedis(redisClient.Raw()), nil
}

func domainEventsWorker(stores *entstore.Stores, logger *slog.Logger) (*outboxworker.Worker, error) {
	if stores == nil || stores.Events == nil {
		return nil, nil
	}
	return outboxworker.New(stores.Events, logger, outboxworker.Config{
		AffiliateStore:    stores.Affiliate,
		AuditStore:        stores.Audit,
		SubscriptionStore: stores.Subscriptions,
	})
}

func retentionCleanupWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger) (*retentionworker.Worker, error) {
	if stores == nil || stores.Operations == nil {
		return nil, nil
	}
	return retentionworker.New(stores.Operations, logger, retentionworker.Config{
		UsageLogsDays:              cfg.Retention.UsageLogsDays,
		SchedulerDecisionsDays:     cfg.Retention.SchedulerDecisionsDays,
		SchedulerFeedbacksDays:     cfg.Retention.SchedulerFeedbacksDays,
		AuditLogsDays:              cfg.Retention.AuditLogsDays,
		AccountHealthSnapshotsDays: cfg.Retention.AccountHealthSnapshotsDays,
	})
}

func paymentOrderExpirerWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger) (*orderexpirerworker.Worker, error) {
	if stores == nil || stores.Payments == nil {
		return nil, nil
	}
	return orderexpirerworker.New(stores.Payments, cfg.Security.MasterKey, orderexpirerworker.Dependencies{}, logger, orderexpirerworker.Config{
		Audit: stores.Audit,
	})
}

func subscriptionExpirerWorker(stores *entstore.Stores, logger *slog.Logger) (*subscriptionexpirerworker.Worker, error) {
	if stores == nil || stores.Subscriptions == nil {
		return nil, nil
	}
	return subscriptionexpirerworker.New(stores.Subscriptions, subscriptionexpirerworker.Dependencies{}, logger, subscriptionexpirerworker.Config{
		Events: stores.Events,
	})
}

func balanceChargerWorker(stores *entstore.Stores, logger *slog.Logger) (*balancechargerworker.Worker, error) {
	if stores == nil || stores.UsageCharges == nil {
		return nil, nil
	}
	return balancechargerworker.New(stores.UsageCharges, logger, balancechargerworker.Config{
		Users: stores.Users,
		Audit: stores.Audit,
	})
}

func accountHealthProbeWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger) (*healthprobeworker.Worker, error) {
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
	})
}

func qualityEvalWorker(cfg config.Config, stores *entstore.Stores, logger *slog.Logger) (*qualityevalworker.Worker, error) {
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
	if a.expirer != nil {
		a.expirer.Start(context.Background())
	}
	if a.subExpiry != nil {
		a.subExpiry.Start(context.Background())
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
	if a.expirer != nil {
		errs = append(errs, a.expirer.Shutdown(ctx))
	}
	if a.subExpiry != nil {
		errs = append(errs, a.subExpiry.Shutdown(ctx))
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
	return errors.Join(errs...)
}

type dependencyPinger interface {
	Ping(context.Context) error
}

type notRequiredPinger struct{}

func (notRequiredPinger) Ping(context.Context) error {
	return nil
}
