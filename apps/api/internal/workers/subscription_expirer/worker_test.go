package subscriptionexpirer

import (
	"io"
	"log/slog"
	"testing"
	"time"

	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
)

func TestRunOnceExpiresActiveSubscriptions(t *testing.T) {
	store := subscriptionmemory.New()
	eventsStore := eventsmemory.New()
	now := time.Date(2026, 5, 22, 12, 5, 0, 0, time.UTC)
	eventsSvc, err := eventsservice.New(eventsStore, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	subscriptions, err := service.NewWithDependencies(store, service.Dependencies{Events: eventsSvc}, fixedClock{now: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := subscriptions.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	startsAt := now.Add(-2 * time.Hour)
	expiresAt := now.Add(-time.Minute)
	subscription, err := subscriptions.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID:    7,
		PlanID:    plan.ID,
		StartsAt:  &startsAt,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	worker, err := New(store, Dependencies{Events: eventsSvc}, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Expired != 1 {
		t.Fatalf("expected one expired subscription, got %+v", result)
	}
	items, err := subscriptions.ListUserSubscriptionsByUser(t.Context(), 7)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(items) != 1 || items[0].ID != subscription.ID || items[0].Status != contract.SubscriptionStatusExpired {
		t.Fatalf("expected expired subscription, got %+v", items)
	}
	outbox, err := eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "SubscriptionExpired" {
		t.Fatalf("expected subscription expiration event, got %+v", outbox)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
