package authsessioncleanup

import (
	"io"
	"log/slog"
	"testing"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	authmemory "github.com/srapi/srapi/apps/api/internal/modules/auth/store/memory"
)

func TestRunOnceCleansExpiredSessions(t *testing.T) {
	store := authmemory.New()
	now := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	if _, err := store.Create(t.Context(), authcontract.CreateSession{
		ID:        "sess_expired",
		UserID:    7,
		CSRFToken: "csrf_expired",
		ExpiresAt: now.Add(-time.Minute),
		CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if _, err := store.Create(t.Context(), authcontract.CreateSession{
		ID:        "sess_active",
		UserID:    7,
		CSRFToken: "csrf_active",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create active session: %v", err)
	}

	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Selected != 1 || result.Expired != 1 {
		t.Fatalf("expected one expired session cleanup, got %+v", result)
	}
	if _, err := store.FindByID(t.Context(), "sess_expired"); err == nil {
		t.Fatal("expected expired session to be removed")
	}
	if _, err := store.FindByID(t.Context(), "sess_active"); err != nil {
		t.Fatalf("expected active session to remain: %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
