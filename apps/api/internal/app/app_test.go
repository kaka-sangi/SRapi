package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/srapi/srapi/apps/api/internal/config"
	authmemory "github.com/srapi/srapi/apps/api/internal/modules/auth/store/memory"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	qualitymemory "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/store/memory"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"
	platformdb "github.com/srapi/srapi/apps/api/internal/platform/db"
	platformredis "github.com/srapi/srapi/apps/api/internal/platform/redis"
)

func TestAddressReturnsServerAddress(t *testing.T) {
	application := &App{server: &http.Server{Addr: "127.0.0.1:9090"}}
	if got := application.Address(); got != "127.0.0.1:9090" {
		t.Fatalf("expected server address 127.0.0.1:9090, got %s", got)
	}
}

func TestHealthcheckAddressUsesLoopbackForWildcardHost(t *testing.T) {
	cfg := config.Load()
	cfg.Server.Host = "0.0.0.0"
	if got := cfg.HealthcheckAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("expected loopback healthcheck address, got %s", got)
	}
}

func TestSchedulerLeaseStoreFallsBackLocallyWhenRedisUnavailable(t *testing.T) {
	cfg := config.Load()
	cfg.Server.Mode = "local"
	cfg.Redis.Host = "127.0.0.1"
	cfg.Redis.Port = 1
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer redisClient.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := schedulerLeaseStore(fastRedisUnavailableContext(t), cfg, logger, redisClient)
	if err != nil {
		t.Fatalf("expected local redis lease fallback without error, got %v", err)
	}
	if store != nil {
		t.Fatalf("expected nil redis lease store when redis is unavailable in local mode")
	}
}

func TestSchedulerLeaseStoreRequiresRedisInRelease(t *testing.T) {
	cfg := config.Load()
	cfg.Server.Mode = "release"
	cfg.Redis.Host = "127.0.0.1"
	cfg.Redis.Port = 1
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer redisClient.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := schedulerLeaseStore(fastRedisUnavailableContext(t), cfg, logger, redisClient); err == nil {
		t.Fatal("expected release mode to require redis-backed scheduler leases")
	}
}

func TestPersistentStoresFailFastWhenPostgresUnavailable(t *testing.T) {
	cfg := config.Load()
	cfg.Storage.Backend = config.StorageBackendPostgres
	cfg.Database.Host = "127.0.0.1"
	cfg.Database.Port = 1
	dbClient, err := platformdb.Open(cfg.Database)
	if err != nil {
		t.Fatalf("open database client: %v", err)
	}
	defer dbClient.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := persistentStores(t.Context(), cfg, logger, dbClient, nil); err == nil {
		t.Fatal("expected postgres storage backend to fail when database is unavailable")
	}
}

func TestPersistentStoresAllowExplicitMemoryBackend(t *testing.T) {
	cfg := config.Load()
	cfg.Storage.Backend = config.StorageBackendMemory

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stores, err := persistentStores(t.Context(), cfg, logger, nil, nil)
	if err != nil {
		t.Fatalf("expected explicit memory backend to skip persistent stores, got %v", err)
	}
	if stores != nil {
		t.Fatalf("expected no persistent stores for explicit memory backend")
	}
}

func TestRealtimeSlotStoreUsesRedisWhenAvailable(t *testing.T) {
	server := miniredis.RunT(t)
	host, portRaw, err := net.SplitHostPort(server.Addr())
	if err != nil {
		t.Fatalf("split miniredis addr: %v", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatalf("parse miniredis port: %v", err)
	}
	cfg := config.Load()
	cfg.Redis.Host = host
	cfg.Redis.Port = port
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer redisClient.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := realtimeSlotStore(t.Context(), cfg, logger, redisClient)
	if err != nil {
		t.Fatalf("expected redis realtime store, got error %v", err)
	}
	if store == nil {
		t.Fatal("expected redis-backed realtime slot store when redis is available")
	}
}

func TestRealtimeSlotStoreFallsBackLocallyWhenRedisUnavailable(t *testing.T) {
	cfg := config.Load()
	cfg.Server.Mode = "local"
	cfg.Redis.Host = "127.0.0.1"
	cfg.Redis.Port = 1
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer redisClient.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := realtimeSlotStore(fastRedisUnavailableContext(t), cfg, logger, redisClient)
	if err != nil {
		t.Fatalf("expected local redis realtime fallback without error, got %v", err)
	}
	if store != nil {
		t.Fatalf("expected nil redis realtime store when redis is unavailable in local mode")
	}
}

func TestRealtimeSlotStoreRequiresRedisInRelease(t *testing.T) {
	cfg := config.Load()
	cfg.Server.Mode = "release"
	cfg.Redis.Host = "127.0.0.1"
	cfg.Redis.Port = 1
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer redisClient.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := realtimeSlotStore(fastRedisUnavailableContext(t), cfg, logger, redisClient); err == nil {
		t.Fatal("expected release mode to require redis-backed realtime slots")
	}
}

func fastRedisUnavailableContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

func TestReleaseRedisDependencyPingRetriesTransientStartupFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve redis port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release redis port: %v", err)
	}
	host, portRaw, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split redis addr: %v", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		t.Fatalf("parse redis port: %v", err)
	}

	cfg := config.Load()
	cfg.Server.Mode = "release"
	cfg.Redis.Host = host
	cfg.Redis.Port = port
	cfg.Redis.DialTimeoutSeconds = 1
	cfg.Redis.ReadTimeoutSeconds = 1
	cfg.Redis.PoolTimeoutSeconds = 1
	redisClient, err := platformredis.Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer redisClient.Close()

	servers := make(chan *miniredis.Miniredis, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		server := miniredis.NewMiniRedis()
		if err := server.StartAddr(addr); err != nil {
			servers <- nil
			return
		}
		servers <- server
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := pingRedisForDependency(t.Context(), cfg, logger, redisClient, "test"); err != nil {
		t.Fatalf("expected transient redis startup to succeed after retry, got %v", err)
	}

	select {
	case server := <-servers:
		if server == nil {
			t.Fatal("miniredis did not start")
		}
		server.Close()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for miniredis")
	}
}

func TestDomainEventsWorkerRequiresPersistentEventStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Load()
	if worker, err := domainEventsWorker(cfg, nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := domainEventsWorker(cfg, &entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without event store, worker=%v err=%v", worker, err)
	}
}

func TestDomainEventsWorkerDispatchesPersistentOutbox(t *testing.T) {
	eventsStore := eventsmemory.New()
	eventsSvc, err := eventsservice.New(eventsStore, nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	enqueued, err := eventsSvc.Enqueue(t.Context(), eventscontract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_app_worker",
	})
	if err != nil {
		t.Fatalf("enqueue event: %v", err)
	}

	worker, err := domainEventsWorker(config.Load(), &entstore.Stores{Events: eventsStore}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("create domain events worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected worker for persistent event store")
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run domain events worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	inbox, err := eventsSvc.ListInbox(t.Context())
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 1 || inbox[0].EventID != enqueued.EventID {
		t.Fatalf("expected outbox worker to record inbox, got %+v", inbox)
	}
}

func TestQualityEvalWorkerRequiresEnabledPersistentStoreAndJudgeConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Load()
	if worker, err := qualityEvalWorker(cfg, nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := qualityEvalWorker(cfg, &entstore.Stores{QualityEval: qualitymemory.New()}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker when quality eval is disabled, worker=%v err=%v", worker, err)
	}

	cfg.QualityEval.Enabled = true
	if worker, err := qualityEvalWorker(cfg, &entstore.Stores{QualityEval: qualitymemory.New()}, logger); err == nil || worker != nil {
		t.Fatalf("expected enabled worker to require judge config, worker=%v err=%v", worker, err)
	}

	cfg.QualityEval.OpenAIAPIKey = "judge-key"
	worker, err := qualityEvalWorker(cfg, &entstore.Stores{QualityEval: qualitymemory.New()}, logger)
	if err != nil {
		t.Fatalf("create quality eval worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected enabled quality eval worker")
	}
}

func TestRetentionWorkerRequiresPersistentOperationsStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := retentionCleanupWorker(config.Load(), nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := retentionCleanupWorker(config.Load(), &entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without operations store, worker=%v err=%v", worker, err)
	}
}

func TestAuthSessionCleanupWorkerRequiresPersistentCleanupStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := authSessionCleanupWorker(config.Load(), nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := authSessionCleanupWorker(config.Load(), &entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without auth session store, worker=%v err=%v", worker, err)
	}

	worker, err := authSessionCleanupWorker(config.Load(), &entstore.Stores{AuthSessions: authmemory.New()}, logger)
	if err != nil {
		t.Fatalf("create auth session cleanup worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected worker for persistent auth cleanup store")
	}
}

func TestSLOEvaluatorWorkerRequiresPersistentOperationsStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := sloEvaluatorWorker(config.Load(), nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := sloEvaluatorWorker(config.Load(), &entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without operations store, worker=%v err=%v", worker, err)
	}

	worker, err := sloEvaluatorWorker(config.Load(), &entstore.Stores{Operations: operationsmemory.New()}, logger)
	if err != nil {
		t.Fatalf("create SLO evaluator worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected worker for persistent operations store")
	}
}

func TestLiteLLMPricingWorkerRequiresSourceURLAndBillingStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Load()
	if worker, err := liteLLMPricingWorker(cfg, nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := liteLLMPricingWorker(cfg, &entstore.Stores{Pricing: billingmemory.New()}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without source URL, worker=%v err=%v", worker, err)
	}

	cfg.LiteLLMPricing.SourceURL = "https://prices.example.com/model_prices.json"
	worker, err := liteLLMPricingWorker(cfg, &entstore.Stores{Pricing: billingmemory.New()}, logger)
	if err != nil {
		t.Fatalf("create LiteLLM pricing worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected LiteLLM pricing worker")
	}
}

func TestPaymentOrderExpirerWorkerRequiresPersistentPaymentStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := paymentOrderExpirerWorker(config.Load(), nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := paymentOrderExpirerWorker(config.Load(), &entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without payment store, worker=%v err=%v", worker, err)
	}

	worker, err := paymentOrderExpirerWorker(config.Load(), &entstore.Stores{Payments: paymentmemory.New()}, logger)
	if err != nil {
		t.Fatalf("create payment order expirer worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected worker for persistent payment store")
	}
}

func TestSubscriptionExpirerWorkerRequiresPersistentSubscriptionStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := subscriptionExpirerWorker(nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := subscriptionExpirerWorker(&entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without subscription store, worker=%v err=%v", worker, err)
	}

	worker, err := subscriptionExpirerWorker(&entstore.Stores{Subscriptions: subscriptionmemory.New()}, logger)
	if err != nil {
		t.Fatalf("create subscription expirer worker: %v", err)
	}
	if worker == nil {
		t.Fatal("expected worker for persistent subscription store")
	}
}
