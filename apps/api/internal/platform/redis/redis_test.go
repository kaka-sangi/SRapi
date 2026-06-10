package redis

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestOpenConfiguresPoolAndTimeoutOptions(t *testing.T) {
	cfg := config.Load()
	cfg.Redis.Host = "redis.internal"
	cfg.Redis.Port = 6380
	cfg.Redis.Password = "secret"
	cfg.Redis.Database = "5"
	cfg.Redis.PoolSize = 14
	cfg.Redis.MinIdleConns = 3
	cfg.Redis.DialTimeoutSeconds = 4
	cfg.Redis.ReadTimeoutSeconds = 5
	cfg.Redis.WriteTimeoutSeconds = 6
	cfg.Redis.PoolTimeoutSeconds = 7

	client, err := Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer client.Close()

	opts := client.Options()
	if opts == nil {
		t.Fatal("expected redis options")
	}
	if opts.Addr != "redis.internal:6380" ||
		opts.Password != "secret" ||
		opts.DB != 5 ||
		opts.PoolSize != 14 ||
		opts.MinIdleConns != 3 ||
		opts.DialTimeout != 4*time.Second ||
		opts.ReadTimeout != 5*time.Second ||
		opts.WriteTimeout != 6*time.Second ||
		opts.PoolTimeout != 7*time.Second {
		t.Fatalf("unexpected redis options: %+v", opts)
	}
}

func TestOpenUsesBoundedDefaultPoolAndTimeoutOptions(t *testing.T) {
	cfg := config.Load()
	client, err := Open(cfg.Redis)
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	defer client.Close()

	opts := client.Options()
	if opts.PoolSize != 32 ||
		opts.MinIdleConns != 4 ||
		opts.DialTimeout != 3*time.Second ||
		opts.ReadTimeout != 2*time.Second ||
		opts.WriteTimeout != 2*time.Second ||
		opts.PoolTimeout != 3*time.Second {
		t.Fatalf("unexpected default redis options: %+v", opts)
	}
}
