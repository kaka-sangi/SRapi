package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

const defaultCurrency = "USD"

var defaultSubscriptionReminderDays = []int{7, 3, 1}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type EventEnqueuer interface {
	Enqueue(ctx context.Context, req eventscontract.EnqueueRequest) (eventscontract.OutboxEvent, error)
}

type Dependencies struct {
	Events EventEnqueuer
}

type Service struct {
	store contract.Store
	deps  Dependencies
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	return NewWithDependencies(store, Dependencies{}, clock)
}

func NewWithDependencies(store contract.Store, deps Dependencies, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, deps: deps, clock: clock}, nil
}

func (s *Service) CreatePlan(ctx context.Context, req contract.CreatePlanRequest) (contract.SubscriptionPlan, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || req.ValidityDays <= 0 {
		return contract.SubscriptionPlan{}, ErrInvalidInput
	}
	price, ok := normalizeMoney(req.Price)
	if !ok {
		return contract.SubscriptionPlan{}, ErrInvalidInput
	}
	currency := normalizeCurrency(req.Currency)
	forSale := true
	if req.ForSale != nil {
		forSale = *req.ForSale
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	status := contract.PlanStatusActive
	if req.Status != nil {
		if !validPlanStatus(*req.Status) {
			return contract.SubscriptionPlan{}, ErrInvalidInput
		}
		status = *req.Status
	}
	return s.store.CreatePlan(ctx, contract.CreateStoredPlan{
		Name:         name,
		Description:  strings.TrimSpace(req.Description),
		Price:        price,
		Currency:     currency,
		ValidityDays: req.ValidityDays,
		Entitlements: cloneMap(req.Entitlements),
		ForSale:      forSale,
		SortOrder:    sortOrder,
		Status:       status,
	})
}

// UpdatePlan applies a partial edit to a plan. Only the fields present in req are
// validated and forwarded; nil fields leave the stored value untouched. Mirrors
// CreatePlan's validation (money/currency/status/validity) per field.
func (s *Service) UpdatePlan(ctx context.Context, id int, req contract.UpdatePlanRequest) (contract.SubscriptionPlan, error) {
	if id <= 0 {
		return contract.SubscriptionPlan{}, ErrInvalidInput
	}
	var stored contract.UpdateStoredPlan
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.SubscriptionPlan{}, ErrInvalidInput
		}
		stored.Name = &name
	}
	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		stored.Description = &desc
	}
	if req.Price != nil {
		price, ok := normalizeMoney(*req.Price)
		if !ok {
			return contract.SubscriptionPlan{}, ErrInvalidInput
		}
		stored.Price = &price
	}
	if req.Currency != nil {
		currency := normalizeCurrency(*req.Currency)
		stored.Currency = &currency
	}
	if req.ValidityDays != nil {
		if *req.ValidityDays <= 0 {
			return contract.SubscriptionPlan{}, ErrInvalidInput
		}
		stored.ValidityDays = req.ValidityDays
	}
	if req.Entitlements != nil {
		cloned := cloneMap(*req.Entitlements)
		stored.Entitlements = &cloned
	}
	if req.ForSale != nil {
		stored.ForSale = req.ForSale
	}
	if req.SortOrder != nil {
		stored.SortOrder = req.SortOrder
	}
	if req.Status != nil {
		if !validPlanStatus(*req.Status) {
			return contract.SubscriptionPlan{}, ErrInvalidInput
		}
		stored.Status = req.Status
	}
	return s.store.UpdatePlan(ctx, id, stored)
}

func (s *Service) ListPlans(ctx context.Context) ([]contract.SubscriptionPlan, error) {
	return s.store.ListPlans(ctx)
}

// ListForSalePlans returns only the plans the storefront should expose: active
// status and explicitly flagged for_sale. Filtered in service rather than via a
// new store predicate because the catalog is small.
func (s *Service) ListForSalePlans(ctx context.Context) ([]contract.SubscriptionPlan, error) {
	all, err := s.store.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.SubscriptionPlan, 0, len(all))
	for _, p := range all {
		if p.ForSale && p.Status == contract.PlanStatusActive {
			out = append(out, p)
		}
	}
	return out, nil
}

// FindPlanByID returns a single plan, used by the update handler to capture the
// pre-change audit snapshot.
func (s *Service) FindPlanByID(ctx context.Context, id int) (contract.SubscriptionPlan, error) {
	return s.store.FindPlanByID(ctx, id)
}

// DeletePlan soft-deletes a plan so it stops being offered. Existing user
// subscriptions are unaffected: they carry their own entitlements snapshot, so a
// removed plan never strips access already granted.
func (s *Service) DeletePlan(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeletePlan(ctx, id)
}

func (s *Service) CreateUserSubscription(ctx context.Context, req contract.CreateSubscriptionRequest) (contract.UserSubscription, error) {
	if req.UserID <= 0 || req.PlanID <= 0 {
		return contract.UserSubscription{}, ErrInvalidInput
	}
	sourceType := strings.TrimSpace(req.SourceType)
	sourceID := strings.TrimSpace(req.SourceID)
	if sourceType != "" && sourceID != "" {
		existing, err := s.store.FindUserSubscriptionBySource(ctx, sourceType, sourceID)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, contract.ErrNotFound) {
			return contract.UserSubscription{}, err
		}
	}
	plan, err := s.store.FindPlanByID(ctx, req.PlanID)
	if err != nil {
		return contract.UserSubscription{}, err
	}
	startsAt := s.clock.Now()
	if req.StartsAt != nil {
		startsAt = req.StartsAt.UTC()
	}
	// H5: Reject if the user already has an active subscription on the same plan.
	// This prevents duplicate subscriptions from double-paid orders, webhook
	// replays, or concurrent checkout sessions.
	activeSubs, err := s.store.ListActiveUserSubscriptions(ctx, req.UserID, startsAt)
	if err != nil {
		return contract.UserSubscription{}, err
	}
	now := s.clock.Now()
	for _, sub := range activeSubs {
		if sub.PlanID == req.PlanID && sub.ExpiresAt.After(now) {
			return contract.UserSubscription{}, ErrDuplicateSubscription
		}
	}
	expiresAt := startsAt.AddDate(0, 0, plan.ValidityDays)
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt.UTC()
	}
	if !expiresAt.After(startsAt) {
		return contract.UserSubscription{}, ErrInvalidInput
	}
	status := contract.SubscriptionStatusActive
	if req.Status != nil {
		if !validSubscriptionStatus(*req.Status) {
			return contract.UserSubscription{}, ErrInvalidInput
		}
		status = *req.Status
	}
	return s.store.CreateUserSubscription(ctx, contract.CreateStoredSubscription{
		UserID:               req.UserID,
		PlanID:               req.PlanID,
		Status:               status,
		StartsAt:             startsAt,
		ExpiresAt:            expiresAt,
		EntitlementsSnapshot: cloneMap(plan.Entitlements),
		SourceType:           sourceType,
		SourceID:             sourceID,
	})
}

func (s *Service) ListUserSubscriptions(ctx context.Context) ([]contract.UserSubscription, error) {
	return s.store.ListUserSubscriptions(ctx)
}

// BatchAssignSubscriptionsMaxItems caps the number of items per
// BatchAssignSubscriptions call (1000 — matching the other admin batch
// endpoints).
const BatchAssignSubscriptionsMaxItems = 1000

// BatchAssignSubscriptions assigns a subscription plan to N users in one
// call. Verbatim port of sub2api's SubscriptionService.BulkAssignSubscription
// (subscription_service.go) — sub2api iterates per user calling
// assignSubscriptionWithReuse, which is idempotent on (user_id, group_id):
// an existing subscription on the same (user, plan) is returned with
// reused=true rather than re-created. srapi's port reuses CreateUserSubscription
// which already short-circuits on a matching FindUserSubscriptionBySource, so
// the (user_id, source_type, source_id) tuple plays the same idempotent role
// here. Per-row failures (invalid id, missing plan, store error) surface in
// results[i].Error without aborting the batch. Outer error is reserved for
// precondition failures (empty input, > max items).
//
// Dedups (user_id, plan_id) tuples within the batch — the first occurrence
// wins, subsequent duplicates flag "duplicate id in batch" so the operator
// notices a typo without the silent reuse path masking it.
func (s *Service) BatchAssignSubscriptions(ctx context.Context, items []contract.BatchAssignSubscriptionItem) ([]contract.BatchAssignSubscriptionResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchAssignSubscriptionsMaxItems {
		return nil, ErrInvalidInput
	}
	type key struct{ user, plan int }
	results := make([]contract.BatchAssignSubscriptionResult, 0, len(items))
	seen := make(map[key]struct{}, len(items))
	for i, item := range items {
		row := contract.BatchAssignSubscriptionResult{Index: i, UserID: item.UserID, PlanID: item.PlanID}
		if item.UserID <= 0 || item.PlanID <= 0 {
			row.Outcome = "failed"
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		k := key{user: item.UserID, plan: item.PlanID}
		if _, dup := seen[k]; dup {
			row.Outcome = "failed"
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[k] = struct{}{}
		// Idempotent reuse: when (source_type, source_id) is set,
		// CreateUserSubscription returns the existing row instead of creating a
		// new one. Detect reuse by snapshotting beforehand so the result row
		// reports the "reused" outcome — mirrors sub2api's reused_count.
		var preexisting *contract.UserSubscription
		if strings.TrimSpace(item.SourceType) != "" && strings.TrimSpace(item.SourceID) != "" {
			if existing, err := s.store.FindUserSubscriptionBySource(ctx, item.SourceType, item.SourceID); err == nil {
				preexisting = &existing
			}
		}
		sub, err := s.CreateUserSubscription(ctx, contract.CreateSubscriptionRequest{
			UserID:     item.UserID,
			PlanID:     item.PlanID,
			ExpiresAt:  item.ExpiresAt,
			SourceType: item.SourceType,
			SourceID:   item.SourceID,
		})
		if err != nil {
			row.Outcome = "failed"
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		row.SubscriptionID = sub.ID
		if preexisting != nil && preexisting.ID == sub.ID {
			row.Outcome = "reused"
		} else {
			row.Outcome = "created"
		}
		results = append(results, row)
	}
	return results, nil
}

func (s *Service) DeleteUserSubscription(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteUserSubscription(ctx, id)
}

func (s *Service) ListUserSubscriptionsByUser(ctx context.Context, userID int) ([]contract.UserSubscription, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListUserSubscriptionsByUser(ctx, userID)
}

// MaterializedUsageForUser returns the active subscription's persisted usage
// counters after lazily resetting any expired cost windows.
func (s *Service) MaterializedUsageForUser(ctx context.Context, userID int, at time.Time) (contract.MaterializedUsage, error) {
	if userID <= 0 {
		return contract.MaterializedUsage{}, ErrInvalidInput
	}
	if at.IsZero() {
		at = s.clock.Now()
	}
	return s.store.MaterializedUsageForUser(ctx, userID, at.UTC())
}

// IncrementMaterializedUsage adds a successful request's billable USD cost to
// the active subscription's persisted usage counters.
func (s *Service) IncrementMaterializedUsage(ctx context.Context, delta contract.UsageDelta) (contract.MaterializedUsage, error) {
	if delta.UserID <= 0 {
		return contract.MaterializedUsage{}, ErrInvalidInput
	}
	cost, ok := normalizeMoney(delta.BillableCost)
	if !ok {
		return contract.MaterializedUsage{}, ErrInvalidInput
	}
	if delta.OccurredAt.IsZero() {
		delta.OccurredAt = s.clock.Now()
	}
	delta.BillableCost = cost
	delta.OccurredAt = delta.OccurredAt.UTC()
	return s.store.IncrementMaterializedUsage(ctx, delta)
}

func (s *Service) ExpireActiveUserSubscriptions(ctx context.Context, now time.Time) (contract.ExpireSubscriptionsResult, error) {
	if now.IsZero() {
		now = s.clock.Now()
	}
	now = now.UTC()
	subscriptions, err := s.store.ListExpiredActiveUserSubscriptions(ctx, now)
	if err != nil {
		return contract.ExpireSubscriptionsResult{}, err
	}
	result := contract.ExpireSubscriptionsResult{Selected: len(subscriptions)}
	for _, subscription := range subscriptions {
		updated, expired, err := s.store.ExpireUserSubscription(ctx, subscription.ID, now)
		if err != nil {
			return result, err
		}
		if !expired {
			continue
		}
		s.enqueueSubscriptionExpired(ctx, updated, now)
		result.Expired++
	}
	return result, nil
}

// EnqueueSubscriptionExpiryReminders scans active subscriptions and emits optional reminder events for supported pre-expiry windows.
func (s *Service) EnqueueSubscriptionExpiryReminders(ctx context.Context, now time.Time) (contract.ReminderSubscriptionsResult, error) {
	if s.deps.Events == nil {
		return contract.ReminderSubscriptionsResult{}, nil
	}
	if now.IsZero() {
		now = s.clock.Now()
	}
	now = now.UTC()
	windowEnd := now.AddDate(0, 0, maxReminderDays(defaultSubscriptionReminderDays))
	subscriptions, err := s.store.ListActiveUserSubscriptionsExpiringBetween(ctx, now, windowEnd)
	if err != nil {
		return contract.ReminderSubscriptionsResult{}, err
	}
	result := contract.ReminderSubscriptionsResult{Selected: len(subscriptions)}
	for _, subscription := range subscriptions {
		days, ok := subscriptionReminderDay(now, subscription.ExpiresAt)
		if !ok {
			continue
		}
		enqueued, err := s.enqueueSubscriptionExpiryReminder(ctx, subscription, days, now)
		if err != nil {
			return result, err
		}
		if enqueued {
			result.Enqueued++
		}
	}
	return result, nil
}

// EnsureUserSubscriptionsCurrent lazily flips any user-owned subscription whose
// ExpiresAt has elapsed but whose status is still "active". This is a
// per-request hot-path admission gate so a request from a user whose
// subscription expired between worker passes is rejected as
// "subscription_expired" instead of silently consuming a now-expired
// entitlement. The check is bounded — a user typically has 0-1 active
// subscriptions — and store calls reuse the same connection as the entitlement
// lookup that already runs in CheckEntitlement.
func (s *Service) EnsureUserSubscriptionsCurrent(ctx context.Context, userID int, now time.Time) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidInput
	}
	if now.IsZero() {
		now = s.clock.Now()
	}
	now = now.UTC()
	subs, err := s.store.ListUserSubscriptionsByUser(ctx, userID)
	if err != nil {
		return false, err
	}
	anyActive := false
	for _, sub := range subs {
		if sub.Status != contract.SubscriptionStatusActive {
			continue
		}
		if !sub.ExpiresAt.IsZero() && !sub.ExpiresAt.After(now) {
			updated, expired, err := s.store.ExpireUserSubscription(ctx, sub.ID, now)
			if err != nil {
				return anyActive, err
			}
			if expired {
				s.enqueueSubscriptionExpired(ctx, updated, now)
			}
			continue
		}
		anyActive = true
	}
	return anyActive, nil
}

func (s *Service) CheckEntitlement(ctx context.Context, req contract.EntitlementCheckRequest) (contract.EntitlementDecision, error) {
	if req.UserID <= 0 {
		return contract.EntitlementDecision{}, ErrInvalidInput
	}
	now := req.RequestTime
	if now.IsZero() {
		now = s.clock.Now()
	}
	// Hot-path lazy expiry: flip any user subscription whose ExpiresAt has
	// elapsed but whose status row is still "active" before reading
	// entitlements. This prevents a between-worker-pass window where an
	// expired subscription continues to grant gateway access.
	if _, err := s.EnsureUserSubscriptionsCurrent(ctx, req.UserID, now); err != nil {
		return contract.EntitlementDecision{}, err
	}
	active, err := s.store.ListActiveEntitlements(ctx, req.UserID, now)
	if err != nil {
		return contract.EntitlementDecision{}, err
	}
	if len(active) == 0 {
		return contract.EntitlementDecision{
			Allowed:      true,
			Reason:       "system_default",
			Entitlements: map[string]any{},
		}, nil
	}
	entitlements := mergeEntitlementRows(active)
	decision := contract.EntitlementDecision{
		Allowed:           true,
		Reason:            "allowed",
		Entitlements:      cloneMap(entitlements),
		AccountGroupScope: entitlementIntSlice(entitlements, "account_group_scope"),
		SchedulerStrategy: entitlementString(entitlements, "scheduler_strategy"),
		MonthlyTokenQuota: entitlementOptionalInt(entitlements, "monthly_token_quota"),
		DailyCostQuota:    entitlementOptionalMoney(entitlements, "daily_cost_quota"),
		WeeklyCostQuota:   entitlementOptionalMoney(entitlements, "weekly_cost_quota"),
		MonthlyCostQuota:  entitlementOptionalMoney(entitlements, "monthly_cost_quota"),
		CostQuotaMode:     normalizeCostQuotaMode(entitlementString(entitlements, "cost_quota_mode")),
	}
	if allowedModels := entitlementStringSlice(entitlements, "allowed_models"); len(allowedModels) > 0 && !modelAllowed(req.ModelReferences, allowedModels) {
		decision.Allowed = false
		decision.Reason = "model_not_allowed"
		return decision, nil
	}
	if decision.MonthlyTokenQuota != nil && req.TokensUsedInPeriod+maxInt(req.EstimatedTokens, 0) > *decision.MonthlyTokenQuota {
		decision.Allowed = false
		decision.Reason = "monthly_token_quota_exceeded"
		return decision, nil
	}
	if decision.CostQuotaMode != costQuotaModeAllowance {
		for _, quota := range costQuotaWindows(decision, req) {
			totalCost := money.AddMoney(quota.usedCost, req.EstimatedCost)
			if compareMoney(totalCost, quota.limit) > 0 {
				decision.Allowed = false
				decision.Reason = quota.reason
				return decision, nil
			}
		}
	}
	return decision, nil
}

type costQuotaWindow struct {
	limit    string
	usedCost string
	reason   string
}

func costQuotaWindows(decision contract.EntitlementDecision, req contract.EntitlementCheckRequest) []costQuotaWindow {
	windows := make([]costQuotaWindow, 0, 3)
	if decision.DailyCostQuota != nil {
		usedCost := ""
		if req.MaterializedUsage != nil {
			usedCost = req.MaterializedUsage.DailyUsageUSD
		}
		windows = append(windows, costQuotaWindow{limit: *decision.DailyCostQuota, usedCost: usedCost, reason: "daily_cost_quota_exceeded"})
	}
	if decision.WeeklyCostQuota != nil {
		usedCost := ""
		if req.MaterializedUsage != nil {
			usedCost = req.MaterializedUsage.WeeklyUsageUSD
		}
		windows = append(windows, costQuotaWindow{limit: *decision.WeeklyCostQuota, usedCost: usedCost, reason: "weekly_cost_quota_exceeded"})
	}
	if decision.MonthlyCostQuota != nil {
		usedCost := req.CostUsedInPeriod
		if req.MaterializedUsage != nil {
			usedCost = req.MaterializedUsage.MonthlyUsageUSD
		}
		windows = append(windows, costQuotaWindow{limit: *decision.MonthlyCostQuota, usedCost: usedCost, reason: "monthly_cost_quota_exceeded"})
	}
	return windows
}

const (
	costQuotaModeHardCap   = "hard_cap"
	costQuotaModeAllowance = "allowance"
)

func normalizeCostQuotaMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), costQuotaModeAllowance) {
		return costQuotaModeAllowance
	}
	return costQuotaModeHardCap
}

// CostAllowance returns the user's active subscription cost allowance, used by
// the gateway to split per-request cost into subscription-covered vs billable.
func (s *Service) CostAllowance(ctx context.Context, userID int, now time.Time) (contract.CostAllowance, error) {
	if userID <= 0 {
		return contract.CostAllowance{}, ErrInvalidInput
	}
	if now.IsZero() {
		now = s.clock.Now()
	}
	active, err := s.store.ListActiveEntitlements(ctx, userID, now)
	if err != nil {
		return contract.CostAllowance{}, err
	}
	if len(active) == 0 {
		return contract.CostAllowance{}, nil
	}
	entitlements := mergeEntitlementRows(active)
	return contract.CostAllowance{
		Mode:        normalizeCostQuotaMode(entitlementString(entitlements, "cost_quota_mode")),
		DailyQuota:  entitlementOptionalMoney(entitlements, "daily_cost_quota"),
		WeeklyQuota: entitlementOptionalMoney(entitlements, "weekly_cost_quota"),
		Quota:       entitlementOptionalMoney(entitlements, "monthly_cost_quota"),
	}, nil
}

func (s *Service) enqueueSubscriptionExpired(ctx context.Context, subscription contract.UserSubscription, expiredAt time.Time) {
	if s.deps.Events == nil {
		return
	}
	_, _ = s.deps.Events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "SubscriptionExpired",
		ProducerModule: "subscriptions",
		AggregateType:  "user_subscription",
		AggregateID:    strconv.Itoa(subscription.ID),
		IdempotencyKey: "subscription_expired:" + strconv.Itoa(subscription.ID),
		Payload: map[string]any{
			"subscription_id": subscription.ID,
			"user_id":         subscription.UserID,
			"plan_id":         subscription.PlanID,
			"expired_at":      expiredAt.Format(time.RFC3339Nano),
		},
	})
}

func (s *Service) enqueueSubscriptionExpiryReminder(ctx context.Context, subscription contract.UserSubscription, daysRemaining int, now time.Time) (bool, error) {
	if s.deps.Events == nil {
		return false, nil
	}
	planName := "Subscription"
	plan, err := s.store.FindPlanByID(ctx, subscription.PlanID)
	if err == nil && strings.TrimSpace(plan.Name) != "" {
		planName = strings.TrimSpace(plan.Name)
	}
	_, err = s.deps.Events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      notificationscontract.EventSubscriptionExpiryReminder,
		ProducerModule: "subscriptions",
		AggregateType:  "user_subscription",
		AggregateID:    strconv.Itoa(subscription.ID),
		IdempotencyKey: "subscription_expiry_reminder:" + strconv.Itoa(subscription.ID) + ":" + strconv.Itoa(daysRemaining) + "d",
		Payload: map[string]any{
			"subscription_id":   subscription.ID,
			"recipient_user_id": subscription.UserID,
			"plan_id":           subscription.PlanID,
			"subscription_name": planName,
			"days_remaining":    daysRemaining,
			"reminder_key":      strconv.Itoa(daysRemaining) + "d",
			"expires_at":        subscription.ExpiresAt.Format(time.RFC3339Nano),
			"triggered_at":      now.Format(time.RFC3339Nano),
			"subscription_url":  "/subscriptions",
		},
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func subscriptionReminderDay(now time.Time, expiresAt time.Time) (int, bool) {
	if expiresAt.Before(now) {
		return 0, false
	}
	remaining := expiresAt.Sub(now)
	days := int(remaining / (24 * time.Hour))
	if remaining%(24*time.Hour) != 0 {
		days++
	}
	for _, allowed := range defaultSubscriptionReminderDays {
		if days == allowed {
			return days, true
		}
	}
	return 0, false
}

func maxReminderDays(days []int) int {
	max := 0
	for _, day := range days {
		if day > max {
			max = day
		}
	}
	return max
}

func mergeEntitlementRows(entitlements []contract.Entitlement) map[string]any {
	sort.SliceStable(entitlements, func(i, j int) bool {
		if entitlements[i].SourceSubscriptionID == entitlements[j].SourceSubscriptionID {
			return entitlements[i].ID < entitlements[j].ID
		}
		return entitlements[i].SourceSubscriptionID < entitlements[j].SourceSubscriptionID
	})
	merged := map[string]any{}
	for _, entitlement := range entitlements {
		if entitlement.FeatureKey == "" {
			continue
		}
		value, ok := entitlement.Value["value"]
		if !ok {
			value = entitlement.Value
		}
		merged[entitlement.FeatureKey] = cloneAny(value)
	}
	return merged
}

func compareMoney(left string, right string) int {
	leftRat, ok := money.DecimalRat(money.NormalizeAmount(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := money.DecimalRat(money.NormalizeAmount(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Cmp(rightRat)
}

func normalizeMoney(value string) (string, bool) {
	rat, ok := money.DecimalRat(money.NormalizeAmount(value))
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return money.FormatRatFixed(rat, 8), true
}

func normalizeCurrency(value string) string {
	return money.NormalizeCurrency(value)
}

func validPlanStatus(status contract.PlanStatus) bool {
	switch status {
	case contract.PlanStatusActive, contract.PlanStatusDisabled, contract.PlanStatusArchived:
		return true
	default:
		return false
	}
}

func validSubscriptionStatus(status contract.SubscriptionStatus) bool {
	switch status {
	case contract.SubscriptionStatusActive, contract.SubscriptionStatusExpired, contract.SubscriptionStatusCancelled, contract.SubscriptionStatusSuspended:
		return true
	default:
		return false
	}
}

func modelAllowed(modelReferences []string, allowed []string) bool {
	seen := map[string]struct{}{}
	for _, item := range allowed {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized != "" {
			seen[normalized] = struct{}{}
		}
	}
	for _, item := range modelReferences {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if _, ok := seen[normalized]; ok {
			return true
		}
	}
	return false
}

func entitlementStringSlice(entitlements map[string]any, key string) []string {
	value, ok := entitlements[key]
	if !ok || value == nil {
		return nil
	}
	switch value := value.(type) {
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if trimmed := strings.TrimSpace(toString(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, item := range parts {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		return nil
	}
}

func entitlementIntSlice(entitlements map[string]any, key string) []int {
	value, ok := entitlements[key]
	if !ok || value == nil {
		return nil
	}
	switch value := value.(type) {
	case []int:
		out := make([]int, len(value))
		copy(out, value)
		return out
	case []any:
		out := make([]int, 0, len(value))
		for _, item := range value {
			if parsed, ok := toInt(item); ok {
				out = append(out, parsed)
			}
		}
		return out
	default:
		return nil
	}
}

func entitlementOptionalInt(entitlements map[string]any, key string) *int {
	value, ok := entitlements[key]
	if !ok || value == nil {
		return nil
	}
	parsed, ok := toInt(value)
	if !ok {
		return nil
	}
	return &parsed
}

func entitlementOptionalMoney(entitlements map[string]any, key string) *string {
	value, ok := entitlements[key]
	if !ok || value == nil {
		return nil
	}
	normalized, ok := normalizeMoney(toString(value))
	if !ok {
		return nil
	}
	return &normalized
}

func entitlementString(entitlements map[string]any, key string) string {
	value, ok := entitlements[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(toString(value))
}

func toInt(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		if value > int64(math.MaxInt) || value < int64(math.MinInt) {
			return 0, false
		}
		return int(value), true
	case float64:
		if value != math.Trunc(value) {
			return 0, false
		}
		return int(value), true
	case json.Number:
		parsed, err := value.Int64()
		if err != nil || parsed > int64(math.MaxInt) || parsed < int64(math.MinInt) {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func toString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return strings.TrimSpace(strings.Trim(fmt.Sprint(value), "\""))
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}

func cloneAny(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return value
	}
	return cloned
}
