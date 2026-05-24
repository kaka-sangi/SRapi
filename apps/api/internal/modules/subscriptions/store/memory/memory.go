package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
)

type Store struct {
	mu            sync.Mutex
	nextPlanID    int
	nextSubID     int
	nextPricingID int
	plans         map[int]contract.SubscriptionPlan
	subscriptions map[int]contract.UserSubscription
	pricingRules  map[int]contract.PricingRule
}

func New() *Store {
	return &Store{
		nextPlanID:    1,
		nextSubID:     1,
		nextPricingID: 1,
		plans:         map[int]contract.SubscriptionPlan{},
		subscriptions: map[int]contract.UserSubscription{},
		pricingRules:  map[int]contract.PricingRule{},
	}
}

func (s *Store) CreatePlan(_ context.Context, input contract.CreateStoredPlan) (contract.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	plan := contract.SubscriptionPlan{
		ID:           s.nextPlanID,
		Name:         input.Name,
		Description:  input.Description,
		Price:        input.Price,
		Currency:     input.Currency,
		ValidityDays: input.ValidityDays,
		Entitlements: cloneMap(input.Entitlements),
		ForSale:      input.ForSale,
		SortOrder:    input.SortOrder,
		Status:       input.Status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.plans[plan.ID] = plan
	s.nextPlanID++
	return clonePlan(plan), nil
}

func (s *Store) FindPlanByID(_ context.Context, id int) (contract.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.plans[id]
	if !ok {
		return contract.SubscriptionPlan{}, errors.New("subscription plan not found")
	}
	return clonePlan(plan), nil
}

func (s *Store) ListPlans(_ context.Context) ([]contract.SubscriptionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.SubscriptionPlan, 0, len(s.plans))
	for _, plan := range s.plans {
		out = append(out, clonePlan(plan))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].ID < out[j].ID
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out, nil
}

func (s *Store) CreateUserSubscription(_ context.Context, input contract.CreateStoredSubscription) (contract.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[input.PlanID]; !ok {
		return contract.UserSubscription{}, errors.New("subscription plan not found")
	}
	now := time.Now().UTC()
	subscription := contract.UserSubscription{
		ID:                   s.nextSubID,
		UserID:               input.UserID,
		PlanID:               input.PlanID,
		Status:               input.Status,
		StartsAt:             input.StartsAt,
		ExpiresAt:            input.ExpiresAt,
		EntitlementsSnapshot: cloneMap(input.EntitlementsSnapshot),
		SourceType:           input.SourceType,
		SourceID:             input.SourceID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	s.subscriptions[subscription.ID] = subscription
	s.nextSubID++
	return cloneSubscription(subscription), nil
}

func (s *Store) FindUserSubscriptionBySource(_ context.Context, sourceType string, sourceID string) (contract.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sourceType = strings.TrimSpace(sourceType)
	sourceID = strings.TrimSpace(sourceID)
	for _, subscription := range s.subscriptions {
		if subscription.SourceType == sourceType && subscription.SourceID == sourceID {
			return cloneSubscription(subscription), nil
		}
	}
	return contract.UserSubscription{}, contract.ErrNotFound
}

func (s *Store) ListUserSubscriptions(_ context.Context) ([]contract.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UserSubscription, 0, len(s.subscriptions))
	for _, subscription := range s.subscriptions {
		out = append(out, cloneSubscription(subscription))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListUserSubscriptionsByUser(_ context.Context, userID int) ([]contract.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UserSubscription, 0)
	for _, subscription := range s.subscriptions {
		if subscription.UserID == userID {
			out = append(out, cloneSubscription(subscription))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListActiveUserSubscriptions(_ context.Context, userID int, at time.Time) ([]contract.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UserSubscription, 0)
	for _, subscription := range s.subscriptions {
		if subscription.UserID != userID || subscription.Status != contract.SubscriptionStatusActive {
			continue
		}
		if at.Before(subscription.StartsAt) || !at.Before(subscription.ExpiresAt) {
			continue
		}
		out = append(out, cloneSubscription(subscription))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out, nil
}

func (s *Store) ListExpiredActiveUserSubscriptions(_ context.Context, now time.Time) ([]contract.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	out := make([]contract.UserSubscription, 0)
	for _, subscription := range s.subscriptions {
		if subscription.Status != contract.SubscriptionStatusActive || !subscription.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, cloneSubscription(subscription))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ExpireUserSubscription(_ context.Context, id int, now time.Time) (contract.UserSubscription, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subscription, ok := s.subscriptions[id]
	if !ok {
		return contract.UserSubscription{}, false, errors.New("user subscription not found")
	}
	now = now.UTC()
	if subscription.Status != contract.SubscriptionStatusActive || !subscription.ExpiresAt.Before(now) {
		return cloneSubscription(subscription), false, nil
	}
	subscription.Status = contract.SubscriptionStatusExpired
	subscription.UpdatedAt = now
	s.subscriptions[subscription.ID] = subscription
	return cloneSubscription(subscription), true, nil
}

func (s *Store) CreatePricingRule(_ context.Context, input contract.PricingRule) (contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	rule := clonePricingRule(input)
	rule.ID = s.nextPricingID
	rule.CreatedAt = now
	rule.UpdatedAt = now
	s.pricingRules[rule.ID] = rule
	s.nextPricingID++
	return clonePricingRule(rule), nil
}

func (s *Store) ListPricingRules(_ context.Context) ([]contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.PricingRule, 0, len(s.pricingRules))
	for _, rule := range s.pricingRules {
		out = append(out, clonePricingRule(rule))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func clonePlan(value contract.SubscriptionPlan) contract.SubscriptionPlan {
	value.Entitlements = cloneMap(value.Entitlements)
	value.DeletedAt = cloneTime(value.DeletedAt)
	return value
}

func cloneSubscription(value contract.UserSubscription) contract.UserSubscription {
	value.EntitlementsSnapshot = cloneMap(value.EntitlementsSnapshot)
	return value
}

func clonePricingRule(value contract.PricingRule) contract.PricingRule {
	value.EffectiveFrom = cloneTime(value.EffectiveFrom)
	value.EffectiveTo = cloneTime(value.EffectiveTo)
	return value
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

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
