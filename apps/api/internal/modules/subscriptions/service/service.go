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

func (s *Service) ListPlans(ctx context.Context) ([]contract.SubscriptionPlan, error) {
	return s.store.ListPlans(ctx)
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

func (s *Service) ListUserSubscriptionsByUser(ctx context.Context, userID int) ([]contract.UserSubscription, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListUserSubscriptionsByUser(ctx, userID)
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

func (s *Service) CreatePricingRule(ctx context.Context, req contract.CreatePricingRuleRequest) (contract.PricingRule, error) {
	rule, err := pricingRuleFromRequest(req)
	if err != nil {
		return contract.PricingRule{}, err
	}
	return s.store.CreatePricingRule(ctx, rule)
}

// ValidatePricingRule validates a pricing-rule request without persisting it.
func (s *Service) ValidatePricingRule(req contract.CreatePricingRuleRequest) error {
	_, err := pricingRuleFromRequest(req)
	return err
}

func pricingRuleFromRequest(req contract.CreatePricingRuleRequest) (contract.PricingRule, error) {
	if req.ModelID <= 0 || req.ProviderID < 0 {
		return contract.PricingRule{}, ErrInvalidInput
	}
	input, ok := normalizeMoney(req.InputPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	output, ok := normalizeMoney(req.OutputPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	cacheRead, ok := normalizeMoney(req.CacheReadPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	cacheWrite, ok := normalizeMoney(req.CacheWritePricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	if req.EffectiveFrom != nil && req.EffectiveTo != nil && !req.EffectiveTo.After(*req.EffectiveFrom) {
		return contract.PricingRule{}, ErrInvalidInput
	}
	return contract.PricingRule{
		ModelID:                         req.ModelID,
		ProviderID:                      req.ProviderID,
		InputPricePerMillionTokens:      input,
		OutputPricePerMillionTokens:     output,
		CacheReadPricePerMillionTokens:  cacheRead,
		CacheWritePricePerMillionTokens: cacheWrite,
		Currency:                        normalizeCurrency(req.Currency),
		EffectiveFrom:                   cloneTime(req.EffectiveFrom),
		EffectiveTo:                     cloneTime(req.EffectiveTo),
	}, nil
}

func (s *Service) ListPricingRules(ctx context.Context) ([]contract.PricingRule, error) {
	return s.store.ListPricingRules(ctx)
}

func (s *Service) CheckEntitlement(ctx context.Context, req contract.EntitlementCheckRequest) (contract.EntitlementDecision, error) {
	if req.UserID <= 0 {
		return contract.EntitlementDecision{}, ErrInvalidInput
	}
	now := req.RequestTime
	if now.IsZero() {
		now = s.clock.Now()
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
		MonthlyCostQuota:  entitlementOptionalMoney(entitlements, "monthly_cost_quota"),
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
	if decision.MonthlyCostQuota != nil {
		totalCost := addMoney(req.CostUsedInPeriod, req.EstimatedCost)
		if compareMoney(totalCost, *decision.MonthlyCostQuota) > 0 {
			decision.Allowed = false
			decision.Reason = "monthly_cost_quota_exceeded"
			return decision, nil
		}
	}
	return decision, nil
}

func (s *Service) EstimatePrice(ctx context.Context, req contract.PricingRequest) (contract.PricingResult, error) {
	if req.ModelID <= 0 || req.ProviderID < 0 {
		return contract.PricingResult{}, ErrInvalidInput
	}
	at := req.At
	if at.IsZero() {
		at = s.clock.Now()
	}
	if len(req.PricingOverride) > 0 {
		result, ok := priceFromPayload(req.PricingOverride, req, nil)
		if ok {
			return result, nil
		}
	}
	rules, err := s.store.ListPricingRules(ctx)
	if err != nil {
		return contract.PricingResult{}, err
	}
	rule, ok := selectPricingRule(rules, req.ModelID, req.ProviderID, at)
	if !ok {
		return contract.PricingResult{Amount: "0.00000000", Currency: defaultCurrency}, nil
	}
	ruleID := rule.ID
	return priceFromRule(rule, req, &ruleID), nil
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

func mergeEntitlements(subscriptions []contract.UserSubscription) map[string]any {
	sort.SliceStable(subscriptions, func(i, j int) bool {
		return subscriptions[i].StartsAt.Before(subscriptions[j].StartsAt)
	})
	merged := map[string]any{}
	for _, sub := range subscriptions {
		for key, value := range sub.EntitlementsSnapshot {
			merged[key] = cloneAny(value)
		}
	}
	return merged
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

func selectPricingRule(rules []contract.PricingRule, modelID int, providerID int, at time.Time) (contract.PricingRule, bool) {
	var selected contract.PricingRule
	found := false
	for _, rule := range rules {
		if rule.ModelID != modelID {
			continue
		}
		if rule.ProviderID != providerID && rule.ProviderID != 0 {
			continue
		}
		if !pricingRuleActive(rule, at) {
			continue
		}
		if !found || moreSpecificPricingRule(rule, selected) {
			selected = rule
			found = true
		}
	}
	return selected, found
}

func pricingRuleActive(rule contract.PricingRule, at time.Time) bool {
	if rule.EffectiveFrom != nil && at.Before(*rule.EffectiveFrom) {
		return false
	}
	if rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo) {
		return false
	}
	return true
}

func moreSpecificPricingRule(candidate contract.PricingRule, current contract.PricingRule) bool {
	if candidate.ProviderID != 0 && current.ProviderID == 0 {
		return true
	}
	if candidate.ProviderID == current.ProviderID && candidate.ID > current.ID {
		return true
	}
	return false
}

func priceFromRule(rule contract.PricingRule, req contract.PricingRequest, ruleID *int) contract.PricingResult {
	amount := usagePrice(req.InputTokens, rule.InputPricePerMillionTokens)
	amount = addMoney(amount, usagePrice(req.OutputTokens, rule.OutputPricePerMillionTokens))
	amount = addMoney(amount, usagePrice(req.CacheReadTokens, rule.CacheReadPricePerMillionTokens))
	amount = addMoney(amount, usagePrice(req.CacheWriteTokens, rule.CacheWritePricePerMillionTokens))
	return contract.PricingResult{Amount: amount, Currency: normalizeCurrency(rule.Currency), PricingRuleID: ruleID}
}

func priceFromPayload(payload map[string]any, req contract.PricingRequest, ruleID *int) (contract.PricingResult, bool) {
	input := payloadMoney(payload, "input_price_per_million_tokens", "input_price_per_million")
	output := payloadMoney(payload, "output_price_per_million_tokens", "output_price_per_million")
	cacheRead := payloadMoney(payload, "cache_read_price_per_million_tokens", "cache_read_price_per_million")
	cacheWrite := payloadMoney(payload, "cache_write_price_per_million_tokens", "cache_write_price_per_million")
	if input == "" && output == "" && cacheRead == "" && cacheWrite == "" {
		return contract.PricingResult{}, false
	}
	rule := contract.PricingRule{
		InputPricePerMillionTokens:      defaultMoney(input),
		OutputPricePerMillionTokens:     defaultMoney(output),
		CacheReadPricePerMillionTokens:  defaultMoney(cacheRead),
		CacheWritePricePerMillionTokens: defaultMoney(cacheWrite),
		Currency:                        payloadString(payload, "currency"),
	}
	return priceFromRule(rule, req, ruleID), true
}

func usagePrice(tokens int, pricePerMillion string) string {
	if tokens <= 0 {
		return "0.00000000"
	}
	price, ok := decimalRat(pricePerMillion)
	if !ok {
		return "0.00000000"
	}
	price.Mul(price, big.NewRat(int64(tokens), 1000000))
	return formatRatFixed(price, 8)
}

func addMoney(left string, right string) string {
	leftRat, ok := decimalRat(defaultMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := decimalRat(defaultMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return formatRatFixed(leftRat.Add(leftRat, rightRat), 8)
}

func compareMoney(left string, right string) int {
	leftRat, ok := decimalRat(defaultMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := decimalRat(defaultMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Cmp(rightRat)
}

func normalizeMoney(value string) (string, bool) {
	rat, ok := decimalRat(defaultMoney(value))
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return formatRatFixed(rat, 8), true
}

func defaultMoney(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.00000000"
	}
	return value
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return big.NewRat(0, 1), true
	}
	rat, ok := new(big.Rat).SetString(value)
	return rat, ok
}

func formatRatFixed(value *big.Rat, scale int) string {
	if value == nil {
		value = new(big.Rat)
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(multiplier))
	numerator := new(big.Int).Set(scaled.Num())
	denominator := new(big.Int).Set(scaled.Denom())
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	doubleRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	if doubleRemainder.Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	raw := quotient.String()
	if scale == 0 {
		return raw
	}
	for len(raw) <= scale {
		raw = "0" + raw
	}
	return raw[:len(raw)-scale] + "." + raw[len(raw)-scale:]
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return defaultCurrency
	}
	return value
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

func payloadMoney(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		normalized, ok := normalizeMoney(toString(value))
		if ok {
			return normalized
		}
	}
	return ""
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return toString(value)
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

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
