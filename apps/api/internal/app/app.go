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
	platformredis "github.com/srapi/srapi/apps/api/internal/platform/redis"
	balancechargerworker "github.com/srapi/srapi/apps/api/internal/workers/balance_charger"
	orderexpirerworker "github.com/srapi/srapi/apps/api/internal/workers/order_expirer"
	outboxworker "github.com/srapi/srapi/apps/api/internal/workers/outbox"
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
	outbox    *outboxworker.Worker
	retention *retentionworker.Worker
	expirer   *orderexpirerworker.Worker
	subExpiry *subscriptionexpirerworker.Worker
	balance   *balancechargerworker.Worker
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}

	dbClient, err := platformdb.Open(cfg.Database)
	if err != nil {
		return nil, err
	}
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		_ = dbClient.Close()
		return nil, err
	}

	handler, outbox, retention, expirer, subExpiry, balance, err := newHandler(cfg, logger, dbClient, redisClient)
	if err != nil {
		_ = dbClient.Close()
		_ = redisClient.Close()
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
		outbox:    outbox,
		retention: retention,
		expirer:   expirer,
		subExpiry: subExpiry,
		balance:   balance,
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
	return errors.Join(errs...)
}

func (a *App) Address() string {
	return a.server.Addr
}

func Healthcheck(ctx context.Context, cfg config.Config) error {
	return httpserver.Healthcheck(ctx, cfg.HealthcheckAddress())
}

func newHandler(cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client) (http.Handler, *outboxworker.Worker, *retentionworker.Worker, *orderexpirerworker.Worker, *subscriptionexpirerworker.Worker, *balancechargerworker.Worker, error) {
	var (
		handler http.Handler
		err     error
	)

	options := []httpserver.Option{
		httpserver.WithDatabasePinger(dbClient),
		httpserver.WithRedisPinger(redisClient),
	}
	realtimeStore, err := realtimeSlotStore(context.Background(), cfg, logger, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	if realtimeStore != nil {
		options = append(options, httpserver.WithRealtimeStore(realtimeStore))
	}
	stores, err := persistentStores(context.Background(), cfg, logger, dbClient, redisClient)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	outbox, err := domainEventsWorker(stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	retention, err := retentionCleanupWorker(cfg, stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	expirer, err := paymentOrderExpirerWorker(cfg, stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	subExpiry, err := subscriptionExpirerWorker(stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	balance, err := balanceChargerWorker(stores, logger)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
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

	return handler, outbox, retention, expirer, subExpiry, balance, err
}

func persistentStores(ctx context.Context, cfg config.Config, logger *slog.Logger, dbClient *platformdb.Client, redisClient *platformredis.Client) (*entstore.Stores, error) {
	if dbClient == nil || dbClient.Ent() == nil {
		return nil, nil
	}

	pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	err := dbClient.Ping(pingCtx)
	cancel()
	if err != nil {
		if cfg.Server.Mode == "release" {
			return nil, fmt.Errorf("database unavailable for persistent stores: %w", err)
		}
		logger.Warn("database unavailable; using in-memory stores", "error", err)
		return nil, nil
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
	return errors.Join(errs...)
}

type dependencyPinger interface {
	Ping(context.Context) error
}
