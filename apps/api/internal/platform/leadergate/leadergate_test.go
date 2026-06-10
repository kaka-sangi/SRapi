package leadergate

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const postgresDSNEnv = "SRAPI_LEADERGATE_TEST_DSN"

func TestLockKeyIsStable(t *testing.T) {
	first := LockKey("quota_refresh")
	second := LockKey("quota_refresh")
	if first != second {
		t.Fatalf("expected stable lock key, got %d then %d", first, second)
	}
	if first == LockKey("balance_charger") {
		t.Fatalf("expected distinct worker names to use distinct lock keys")
	}
}

func TestGateAllowsOnlyOneLeaderAndReleases(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(postgresDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to run PostgreSQL advisory-lock leader gate test", postgresDSNEnv)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	first, err := New(db)
	if err != nil {
		t.Fatalf("new first gate: %v", err)
	}
	second, err := New(db)
	if err != nil {
		t.Fatalf("new second gate: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	started := make(chan struct{})
	release := make(chan struct{})
	var executed int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ran, err := first.Do(ctx, "quota_refresh", func(context.Context) error {
			atomic.AddInt32(&executed, 1)
			close(started)
			<-release
			return nil
		})
		if err != nil {
			t.Errorf("first Do: %v", err)
			return
		}
		if !ran {
			t.Error("expected first gate to run")
		}
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("first gate did not start: %v", ctx.Err())
	}

	ran, err := second.Do(ctx, "quota_refresh", func(context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("second Do while first holds lock: %v", err)
	}
	if ran {
		t.Fatal("expected second gate to skip while first holds lock")
	}

	close(release)
	wg.Wait()

	ran, err = second.Do(ctx, "quota_refresh", func(context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("second Do after release: %v", err)
	}
	if !ran {
		t.Fatal("expected second gate to take over after first releases")
	}
	if got := atomic.LoadInt32(&executed); got != 2 {
		t.Fatalf("expected exactly two executions, got %d", got)
	}
}
