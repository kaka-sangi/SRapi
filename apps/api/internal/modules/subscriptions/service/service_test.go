package service_test

import (
	"testing"
	"time"

	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
)

func TestDeletePlanSoftDeletesAndPreservesExistingSubscriptions(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	store := subscriptionmemory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}

	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "usd",
		ValidityDays: 30,
		Entitlements: map[string]any{"allowed_models": []any{"allowed-model"}},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{UserID: 1, PlanID: plan.ID}); err != nil {
		t.Fatalf("create user subscription: %v", err)
	}

	if err := svc.DeletePlan(t.Context(), plan.ID); err != nil {
		t.Fatalf("delete plan: %v", err)
	}

	// The plan is gone from lookup and listing.
	if _, err := svc.FindPlanByID(t.Context(), plan.ID); err == nil {
		t.Fatalf("expected deleted plan to be unfindable")
	}
	plans, err := svc.ListPlans(t.Context())
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	for _, p := range plans {
		if p.ID == plan.ID {
			t.Fatalf("deleted plan must not appear in listing: %+v", p)
		}
	}

	// The existing subscriber keeps their entitlements — they were snapshotted at
	// subscription time, so deleting the plan never strips access already granted.
	cached, err := store.ListActiveEntitlements(t.Context(), 1, clock.now)
	if err != nil {
		t.Fatalf("list active entitlements: %v", err)
	}
	if len(cached) == 0 {
		t.Fatalf("deleting a plan must not revoke an existing subscriber's entitlements")
	}

	// Re-deleting is a not-found, not a crash.
	if err := svc.DeletePlan(t.Context(), plan.ID); err == nil {
		t.Fatalf("expected error deleting an already-deleted plan")
	}
}

func TestCheckEntitlementRejectsBeforeSchedulingAndCarriesRoutingPolicy(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	store := subscriptionmemory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}

	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "usd",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"allowed_models":      []any{"allowed-model", "alias-model"},
			"account_group_scope": []any{10, 20},
			"scheduler_strategy":  "cost_saver",
			"monthly_token_quota": 12,
			"monthly_cost_quota":  "0.00001000",
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID: 1,
		PlanID: plan.ID,
	}); err != nil {
		t.Fatalf("create user subscription: %v", err)
	}
	cached, err := store.ListActiveEntitlements(t.Context(), 1, clock.now)
	if err != nil {
		t.Fatalf("list cached entitlements: %v", err)
	}
	if len(cached) != 5 {
		t.Fatalf("expected entitlement cache rows, got %+v", cached)
	}

	allowed, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:          1,
		ModelReferences: []string{"allowed-model"},
		EstimatedTokens: 2,
		EstimatedCost:   "0.00000100",
		RequestTime:     clock.now,
	})
	if err != nil {
		t.Fatalf("check allowed entitlement: %v", err)
	}
	if !allowed.Allowed || allowed.Reason != "allowed" || allowed.SchedulerStrategy != "cost_saver" {
		t.Fatalf("unexpected allowed entitlement: %+v", allowed)
	}
	if len(allowed.AccountGroupScope) != 2 || allowed.AccountGroupScope[0] != 10 || allowed.AccountGroupScope[1] != 20 {
		t.Fatalf("expected routing group scope, got %+v", allowed.AccountGroupScope)
	}
	if allowed.MonthlyTokenQuota == nil || *allowed.MonthlyTokenQuota != 12 {
		t.Fatalf("expected token quota snapshot, got %+v", allowed.MonthlyTokenQuota)
	}
	if allowed.MonthlyCostQuota == nil || *allowed.MonthlyCostQuota != "0.00001000" {
		t.Fatalf("expected normalized cost quota, got %+v", allowed.MonthlyCostQuota)
	}

	disallowedModel, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:          1,
		ModelReferences: []string{"blocked-model"},
		EstimatedTokens: 1,
		EstimatedCost:   "0.00000100",
		RequestTime:     clock.now,
	})
	if err != nil {
		t.Fatalf("check model entitlement: %v", err)
	}
	if disallowedModel.Allowed || disallowedModel.Reason != "model_not_allowed" {
		t.Fatalf("expected model entitlement rejection, got %+v", disallowedModel)
	}

	tokenQuota, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:             1,
		ModelReferences:    []string{"allowed-model"},
		EstimatedTokens:    3,
		TokensUsedInPeriod: 10,
		EstimatedCost:      "0.00000100",
		RequestTime:        clock.now,
	})
	if err != nil {
		t.Fatalf("check token quota entitlement: %v", err)
	}
	if tokenQuota.Allowed || tokenQuota.Reason != "monthly_token_quota_exceeded" {
		t.Fatalf("expected token quota rejection, got %+v", tokenQuota)
	}

	costQuota, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:           1,
		ModelReferences:  []string{"allowed-model"},
		EstimatedTokens:  1,
		CostUsedInPeriod: "0.00000900",
		EstimatedCost:    "0.00000200",
		RequestTime:      clock.now,
	})
	if err != nil {
		t.Fatalf("check cost quota entitlement: %v", err)
	}
	if costQuota.Allowed || costQuota.Reason != "monthly_cost_quota_exceeded" {
		t.Fatalf("expected cost quota rejection, got %+v", costQuota)
	}
}

func TestCheckEntitlementUsesDailyAndWeeklyCostQuotas(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	store := subscriptionmemory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "quota",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"daily_cost_quota":   "0.01000000",
			"weekly_cost_quota":  "0.10000000",
			"monthly_cost_quota": "1.00000000",
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{UserID: 1, PlanID: plan.ID}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	daily, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:        1,
		EstimatedCost: "0.00200000",
		RequestTime:   clock.now,
		MaterializedUsage: &contract.MaterializedUsage{
			DailyUsageUSD:   "0.00900000",
			WeeklyUsageUSD:  "0.05000000",
			MonthlyUsageUSD: "0.50000000",
		},
	})
	if err != nil {
		t.Fatalf("check daily quota: %v", err)
	}
	if daily.Allowed || daily.Reason != "daily_cost_quota_exceeded" {
		t.Fatalf("expected daily quota denial, got %+v", daily)
	}

	weekly, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:        1,
		EstimatedCost: "0.00500000",
		RequestTime:   clock.now,
		MaterializedUsage: &contract.MaterializedUsage{
			DailyUsageUSD:   "0.00100000",
			WeeklyUsageUSD:  "0.09600000",
			MonthlyUsageUSD: "0.50000000",
		},
	})
	if err != nil {
		t.Fatalf("check weekly quota: %v", err)
	}
	if weekly.Allowed || weekly.Reason != "weekly_cost_quota_exceeded" {
		t.Fatalf("expected weekly quota denial, got %+v", weekly)
	}
}

func TestCostAllowanceReturnsDailyWeeklyMonthlyQuotas(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	store := subscriptionmemory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "allowance",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"cost_quota_mode":    "allowance",
			"daily_cost_quota":   "0.01000000",
			"weekly_cost_quota":  "0.10000000",
			"monthly_cost_quota": "1.00000000",
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{UserID: 1, PlanID: plan.ID}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	allowance, err := svc.CostAllowance(t.Context(), 1, clock.now)
	if err != nil {
		t.Fatalf("cost allowance: %v", err)
	}
	if allowance.Mode != "allowance" ||
		allowance.DailyQuota == nil || *allowance.DailyQuota != "0.01000000" ||
		allowance.WeeklyQuota == nil || *allowance.WeeklyQuota != "0.10000000" ||
		allowance.Quota == nil || *allowance.Quota != "1.00000000" {
		t.Fatalf("unexpected allowance: %+v", allowance)
	}
}

func TestCreateUserSubscriptionIsIdempotentBySource(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	svc, err := service.New(subscriptionmemory.New(), clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	first, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID:     1,
		PlanID:     plan.ID,
		SourceType: "payment_order",
		SourceID:   "pay_1",
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	duplicate, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID:     1,
		PlanID:     plan.ID,
		SourceType: "payment_order",
		SourceID:   "pay_1",
	})
	if err != nil {
		t.Fatalf("create duplicate subscription: %v", err)
	}
	if duplicate.ID != first.ID {
		t.Fatalf("expected duplicate source to return existing subscription, first=%+v duplicate=%+v", first, duplicate)
	}
	subscriptions, err := svc.ListUserSubscriptionsByUser(t.Context(), 1)
	if err != nil {
		t.Fatalf("list user subscriptions: %v", err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("expected one subscription after duplicate source, got %+v", subscriptions)
	}
}

func TestMaterializedUsageAccumulatesAndResetsWindows(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	store := subscriptionmemory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{UserID: 1, PlanID: plan.ID}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	firstAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	if _, err := svc.IncrementMaterializedUsage(t.Context(), contract.UsageDelta{UserID: 1, BillableCost: "0.01000000", OccurredAt: firstAt}); err != nil {
		t.Fatalf("increment first: %v", err)
	}
	usage, err := svc.IncrementMaterializedUsage(t.Context(), contract.UsageDelta{UserID: 1, BillableCost: "0.02000000", OccurredAt: firstAt.Add(time.Hour)})
	if err != nil {
		t.Fatalf("increment second: %v", err)
	}
	if usage.MonthlyUsageUSD != "0.03000000" || usage.WeeklyUsageUSD != "0.03000000" || usage.DailyUsageUSD != "0.03000000" {
		t.Fatalf("expected two increments summed, got %+v", usage)
	}

	nextMonth := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	usage, err = svc.MaterializedUsageForUser(t.Context(), 1, nextMonth)
	if err != nil {
		t.Fatalf("read next month usage: %v", err)
	}
	if usage.MonthlyUsageUSD != "0.00000000" || usage.DailyUsageUSD != "0.00000000" {
		t.Fatalf("expected expired windows reset, got %+v", usage)
	}
}

func TestExpireActiveUserSubscriptionsMarksExpiredAndEnqueuesEvent(t *testing.T) {
	store := subscriptionmemory.New()
	eventsStore := eventsmemory.New()
	eventsSvc, err := eventsservice.New(eventsStore, fixedClock{now: time.Date(2026, 5, 22, 12, 5, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	svc, err := service.NewWithDependencies(store, service.Dependencies{Events: eventsSvc}, fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{"allowed_models": []any{"pro-model"}},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	now := time.Date(2026, 5, 22, 12, 5, 0, 0, time.UTC)
	expiredStart := now.Add(-2 * time.Hour)
	expiredAt := now.Add(-time.Minute)
	expiredSub, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID:    1,
		PlanID:    plan.ID,
		StartsAt:  &expiredStart,
		ExpiresAt: &expiredAt,
	})
	if err != nil {
		t.Fatalf("create expired candidate subscription: %v", err)
	}
	futureStart := now.Add(-time.Hour)
	futureExpiry := now.Add(time.Hour)
	futureSub, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID:    1,
		PlanID:    plan.ID,
		StartsAt:  &futureStart,
		ExpiresAt: &futureExpiry,
	})
	if err != nil {
		t.Fatalf("create future subscription: %v", err)
	}

	result, err := svc.ExpireActiveUserSubscriptions(t.Context(), now)
	if err != nil {
		t.Fatalf("expire active subscriptions: %v", err)
	}
	if result.Selected != 1 || result.Expired != 1 {
		t.Fatalf("unexpected expiration result: %+v", result)
	}
	subscriptions, err := svc.ListUserSubscriptionsByUser(t.Context(), 1)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	statuses := map[int]contract.SubscriptionStatus{}
	for _, subscription := range subscriptions {
		statuses[subscription.ID] = subscription.Status
	}
	if statuses[expiredSub.ID] != contract.SubscriptionStatusExpired || statuses[futureSub.ID] != contract.SubscriptionStatusActive {
		t.Fatalf("unexpected subscription statuses: %+v", statuses)
	}
	outbox, err := eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "SubscriptionExpired" || outbox[0].ProducerModule != "subscriptions" {
		t.Fatalf("expected subscription expiration event, got %+v", outbox)
	}

	second, err := svc.ExpireActiveUserSubscriptions(t.Context(), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expire active subscriptions again: %v", err)
	}
	if second.Expired != 0 {
		t.Fatalf("expiration should be idempotent on second run, got %+v", second)
	}
	outbox, err = eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox again: %v", err)
	}
	if len(outbox) != 1 {
		t.Fatalf("expected one expiration event after second run, got %+v", outbox)
	}
}

func TestEnqueueSubscriptionExpiryRemindersUsesReminderWindowsAndIdempotency(t *testing.T) {
	store := subscriptionmemory.New()
	eventsStore := eventsmemory.New()
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	eventsSvc, err := eventsservice.New(eventsStore, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	svc, err := service.NewWithDependencies(store, service.Dependencies{Events: eventsSvc}, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "Pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	createSubscription := func(userID int, expiresAt time.Time) {
		t.Helper()
		startsAt := now.Add(-time.Hour)
		if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
			UserID:    userID,
			PlanID:    plan.ID,
			StartsAt:  &startsAt,
			ExpiresAt: &expiresAt,
		}); err != nil {
			t.Fatalf("create subscription for user %d: %v", userID, err)
		}
	}
	createSubscription(1, now.Add(7*24*time.Hour))
	createSubscription(2, now.Add(3*24*time.Hour))
	createSubscription(3, now.Add(1*24*time.Hour))
	createSubscription(4, now.Add(2*24*time.Hour))
	createSubscription(5, now.Add(8*24*time.Hour))

	result, err := svc.EnqueueSubscriptionExpiryReminders(t.Context(), now)
	if err != nil {
		t.Fatalf("enqueue subscription expiry reminders: %v", err)
	}
	if result.Selected != 4 || result.Enqueued != 3 {
		t.Fatalf("unexpected reminder result: %+v", result)
	}
	outbox, err := eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 3 {
		t.Fatalf("expected three reminder events, got %+v", outbox)
	}
	seenKeys := map[string]bool{}
	for _, event := range outbox {
		if event.EventType != notificationscontract.EventSubscriptionExpiryReminder || event.ProducerModule != "subscriptions" || event.AggregateType != "user_subscription" {
			t.Fatalf("unexpected reminder event metadata: %+v", event)
		}
		if event.Payload["recipient_user_id"] == nil || event.Payload["subscription_name"] != "Pro" || event.Payload["recipient_email_hash"] != nil {
			t.Fatalf("expected safe reminder payload with plan name and no email hash, got %+v", event.Payload)
		}
		key, ok := event.Payload["reminder_key"].(string)
		if !ok {
			t.Fatalf("expected reminder key string, got %+v", event.Payload)
		}
		seenKeys[key] = true
	}
	if !seenKeys["7d"] || !seenKeys["3d"] || !seenKeys["1d"] || seenKeys["2d"] {
		t.Fatalf("unexpected reminder keys: %+v", seenKeys)
	}

	if _, err := svc.EnqueueSubscriptionExpiryReminders(t.Context(), now); err != nil {
		t.Fatalf("enqueue reminders again: %v", err)
	}
	outbox, err = eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox again: %v", err)
	}
	if len(outbox) != 3 {
		t.Fatalf("expected idempotent reminder outbox after second run, got %+v", outbox)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

// TestUpdatePlanStatusOnlyToggle locks the contract the iter-46 frontend
// subscription-plan toggle relies on: PATCH with only Status set keeps
// every other plan field intact.
func TestUpdatePlanStatusOnlyToggle(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	store := subscriptionmemory.New()
	svc, err := service.New(store, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "usd",
		ValidityDays: 30,
		Entitlements: map[string]any{"allowed_models": []any{"a-model"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if plan.Status != contract.PlanStatusActive {
		t.Fatalf("expected created status active, got %q", plan.Status)
	}
	disabled := contract.PlanStatusDisabled
	updated, err := svc.UpdatePlan(t.Context(), plan.ID, contract.UpdatePlanRequest{Status: &disabled})
	if err != nil {
		t.Fatalf("update status-only: %v", err)
	}
	if updated.Status != contract.PlanStatusDisabled {
		t.Fatalf("expected status disabled, got %q", updated.Status)
	}
	if updated.Name != "pro" {
		t.Fatalf("name leaked: %q", updated.Name)
	}
	if updated.Price != plan.Price {
		t.Fatalf("price drifted: before %q after %q", plan.Price, updated.Price)
	}
	if updated.ValidityDays != plan.ValidityDays {
		t.Fatalf("validity drifted: before %d after %d", plan.ValidityDays, updated.ValidityDays)
	}
}
