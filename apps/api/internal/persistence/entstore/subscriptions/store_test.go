package subscriptions

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStorePersistsPlansSubscriptionsAndPricingRules(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	plan, err := store.CreatePlan(ctx, contract.CreateStoredPlan{
		Name:         "pro",
		Description:  "Pro plan",
		Price:        "19.99000000",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"allowed_models":      []any{"pro-model"},
			"account_group_scope": []any{7},
			"monthly_cost_quota":  "1.00000000",
		},
		ForSale:   true,
		SortOrder: 10,
		Status:    contract.PlanStatusActive,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	foundPlan, err := store.FindPlanByID(ctx, plan.ID)
	if err != nil {
		t.Fatalf("find plan: %v", err)
	}
	if foundPlan.ID != plan.ID || foundPlan.Entitlements["monthly_cost_quota"] != "1.00000000" {
		t.Fatalf("expected persisted plan entitlements, got %+v", foundPlan)
	}

	subscription, err := store.CreateUserSubscription(ctx, contract.CreateStoredSubscription{
		UserID:               1,
		PlanID:               plan.ID,
		Status:               contract.SubscriptionStatusActive,
		StartsAt:             now.Add(-time.Hour),
		ExpiresAt:            now.Add(time.Hour),
		EntitlementsSnapshot: map[string]any{"allowed_models": []any{"snapshot-model"}},
		SourceType:           "manual",
		SourceID:             "seed",
	})
	if err != nil {
		t.Fatalf("create user subscription: %v", err)
	}
	if _, err := store.CreateUserSubscription(ctx, contract.CreateStoredSubscription{
		UserID:               1,
		PlanID:               plan.ID,
		Status:               contract.SubscriptionStatusExpired,
		StartsAt:             now.Add(-48 * time.Hour),
		ExpiresAt:            now.Add(-24 * time.Hour),
		EntitlementsSnapshot: map[string]any{"allowed_models": []any{"expired-model"}},
	}); err != nil {
		t.Fatalf("create expired subscription: %v", err)
	}
	byUser, err := store.ListUserSubscriptionsByUser(ctx, 1)
	if err != nil {
		t.Fatalf("list subscriptions by user: %v", err)
	}
	if len(byUser) != 2 || byUser[0].ID != subscription.ID {
		t.Fatalf("expected persisted subscriptions by user, got %+v", byUser)
	}
	active, err := store.ListActiveUserSubscriptions(ctx, 1, now)
	if err != nil {
		t.Fatalf("list active subscriptions: %v", err)
	}
	if len(active) != 1 || active[0].ID != subscription.ID || active[0].EntitlementsSnapshot["allowed_models"] == nil {
		t.Fatalf("expected only active subscription with snapshot, got %+v", active)
	}
	entitlements, err := store.ListActiveEntitlements(ctx, 1, now)
	if err != nil {
		t.Fatalf("list active entitlements: %v", err)
	}
	if len(entitlements) != 1 || entitlements[0].FeatureKey != "allowed_models" || entitlements[0].SourceSubscriptionID != subscription.ID {
		t.Fatalf("expected cached entitlement from active subscription, got %+v", entitlements)
	}
	expiredActive, err := store.ListExpiredActiveUserSubscriptions(ctx, now)
	if err != nil {
		t.Fatalf("list expired active subscriptions: %v", err)
	}
	if len(expiredActive) != 0 {
		t.Fatalf("expected no active subscriptions expired before now, got %+v", expiredActive)
	}
	future := now.Add(time.Hour + time.Minute)
	expiredActive, err = store.ListExpiredActiveUserSubscriptions(ctx, future)
	if err != nil {
		t.Fatalf("list future expired active subscriptions: %v", err)
	}
	if len(expiredActive) != 1 || expiredActive[0].ID != subscription.ID {
		t.Fatalf("expected active subscription to be expired in future scan, got %+v", expiredActive)
	}
	expired, changed, err := store.ExpireUserSubscription(ctx, subscription.ID, future)
	if err != nil {
		t.Fatalf("expire user subscription: %v", err)
	}
	if !changed || expired.Status != contract.SubscriptionStatusExpired {
		t.Fatalf("expected subscription to expire, changed=%v subscription=%+v", changed, expired)
	}
	_, changed, err = store.ExpireUserSubscription(ctx, subscription.ID, future.Add(time.Minute))
	if err != nil {
		t.Fatalf("expire user subscription again: %v", err)
	}
	if changed {
		t.Fatal("expected second expiration to be a no-op")
	}

	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)
	rule, err := store.CreatePricingRule(ctx, contract.PricingRule{
		ModelID:                         11,
		ProviderID:                      22,
		InputPricePerMillionTokens:      "1.25000000",
		OutputPricePerMillionTokens:     "2.50000000",
		CacheReadPricePerMillionTokens:  "0.10000000",
		CacheWritePricePerMillionTokens: "0.20000000",
		Currency:                        "USD",
		EffectiveFrom:                   &from,
		EffectiveTo:                     &to,
	})
	if err != nil {
		t.Fatalf("create pricing rule: %v", err)
	}
	rules, err := store.ListPricingRules(ctx)
	if err != nil {
		t.Fatalf("list pricing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID || rules[0].EffectiveFrom == nil || rules[0].EffectiveTo == nil {
		t.Fatalf("expected persisted pricing rule with effectivity, got %+v", rules)
	}
}

func TestStoreUpdatePlanPartialAndNotFound(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	plan, err := store.CreatePlan(ctx, contract.CreateStoredPlan{
		Name:         "starter",
		Description:  "Starter plan",
		Price:        "9.99000000",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{"allowed_models": []any{"a"}},
		ForSale:      true,
		SortOrder:    0,
		Status:       contract.PlanStatusActive,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	newPrice := "14.99000000"
	newStatus := contract.PlanStatusDisabled
	newEnt := map[string]any{"monthly_token_quota": float64(1000)}
	updated, err := store.UpdatePlan(ctx, plan.ID, contract.UpdateStoredPlan{
		Price:        &newPrice,
		Status:       &newStatus,
		Entitlements: &newEnt,
	})
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}
	if updated.Price != newPrice || updated.Status != contract.PlanStatusDisabled {
		t.Fatalf("expected price/status updated, got %+v", updated)
	}
	// Untouched fields (nil pointers) must be preserved.
	if updated.Name != "starter" || updated.ValidityDays != 30 || !updated.ForSale {
		t.Fatalf("expected untouched fields preserved, got %+v", updated)
	}
	// Entitlements are fully replaced, not merged.
	if _, ok := updated.Entitlements["monthly_token_quota"]; !ok {
		t.Fatalf("expected entitlements replaced, got %+v", updated.Entitlements)
	}
	if _, ok := updated.Entitlements["allowed_models"]; ok {
		t.Fatalf("expected old entitlement key dropped, got %+v", updated.Entitlements)
	}

	if _, err := store.UpdatePlan(ctx, 99999, contract.UpdateStoredPlan{Price: &newPrice}); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing plan, got %v", err)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "subscriptions.db") + "?_fk=1"
}
