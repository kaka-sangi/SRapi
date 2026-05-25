package app

import (
	"io"
	"log/slog"
	"net"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/srapi/srapi/apps/api/internal/config"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"
	platformdb "github.com/srapi/srapi/apps/api/internal/platform/db"
	platformredis "github.com/srapi/srapi/apps/api/internal/platform/redis"
)

func TestNewBuildsServerAtConfiguredAddress(t *testing.T) {
	cfg := config.Load()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 9090
	cfg.Storage.Backend = config.StorageBackendMemory
	cfg.Database.Host = "127.0.0.1"
	cfg.Database.Port = 1

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
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
	store, err := schedulerLeaseStore(t.Context(), cfg, logger, redisClient)
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
	if _, err := schedulerLeaseStore(t.Context(), cfg, logger, redisClient); err == nil {
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
	store, err := realtimeSlotStore(t.Context(), cfg, logger, redisClient)
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
	if _, err := realtimeSlotStore(t.Context(), cfg, logger, redisClient); err == nil {
		t.Fatal("expected release mode to require redis-backed realtime slots")
	}
}

func TestDomainEventsWorkerRequiresPersistentEventStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := domainEventsWorker(nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := domainEventsWorker(&entstore.Stores{}, logger); err != nil || worker != nil {
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

	worker, err := domainEventsWorker(&entstore.Stores{Events: eventsStore}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestRetentionWorkerRequiresPersistentOperationsStore(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if worker, err := retentionCleanupWorker(config.Load(), nil, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without persistent stores, worker=%v err=%v", worker, err)
	}
	if worker, err := retentionCleanupWorker(config.Load(), &entstore.Stores{}, logger); err != nil || worker != nil {
		t.Fatalf("expected nil worker without operations store, worker=%v err=%v", worker, err)
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
