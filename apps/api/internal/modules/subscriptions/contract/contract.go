package contract

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("subscription resource not found")

type PlanStatus string

const (
	PlanStatusActive   PlanStatus = "active"
	PlanStatusDisabled PlanStatus = "disabled"
	PlanStatusArchived PlanStatus = "archived"
)

type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusExpired   SubscriptionStatus = "expired"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
	SubscriptionStatusSuspended SubscriptionStatus = "suspended"
)

type SubscriptionPlan struct {
	ID           int
	Name         string
	Description  string
	Price        string
	Currency     string
	ValidityDays int
	Entitlements map[string]any
	ForSale      bool
	SortOrder    int
	Status       PlanStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

type UserSubscription struct {
	ID                   int
	UserID               int
	PlanID               int
	Status               SubscriptionStatus
	StartsAt             time.Time
	ExpiresAt            time.Time
	EntitlementsSnapshot map[string]any
	SourceType           string
	SourceID             string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type PricingRule struct {
	ID                              int
	ModelID                         int
	ProviderID                      int
	InputPricePerMillionTokens      string
	OutputPricePerMillionTokens     string
	CacheReadPricePerMillionTokens  string
	CacheWritePricePerMillionTokens string
	Currency                        string
	EffectiveFrom                   *time.Time
	EffectiveTo                     *time.Time
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

type CreatePlanRequest struct {
	Name         string
	Description  string
	Price        string
	Currency     string
	ValidityDays int
	Entitlements map[string]any
	ForSale      *bool
	SortOrder    *int
	Status       *PlanStatus
}

type CreateStoredPlan struct {
	Name         string
	Description  string
	Price        string
	Currency     string
	ValidityDays int
	Entitlements map[string]any
	ForSale      bool
	SortOrder    int
	Status       PlanStatus
}

type CreateSubscriptionRequest struct {
	UserID     int
	PlanID     int
	Status     *SubscriptionStatus
	StartsAt   *time.Time
	ExpiresAt  *time.Time
	SourceType string
	SourceID   string
}

type CreateStoredSubscription struct {
	UserID               int
	PlanID               int
	Status               SubscriptionStatus
	StartsAt             time.Time
	ExpiresAt            time.Time
	EntitlementsSnapshot map[string]any
	SourceType           string
	SourceID             string
}

type CreatePricingRuleRequest struct {
	ModelID                         int
	ProviderID                      int
	InputPricePerMillionTokens      string
	OutputPricePerMillionTokens     string
	CacheReadPricePerMillionTokens  string
	CacheWritePricePerMillionTokens string
	Currency                        string
	EffectiveFrom                   *time.Time
	EffectiveTo                     *time.Time
}

type EntitlementCheckRequest struct {
	UserID             int
	ModelReferences    []string
	EstimatedTokens    int
	EstimatedCost      string
	TokensUsedInPeriod int
	CostUsedInPeriod   string
	RequestTime        time.Time
}

type EntitlementDecision struct {
	Allowed           bool
	Reason            string
	Entitlements      map[string]any
	AccountGroupScope []int
	SchedulerStrategy string
	MonthlyTokenQuota *int
	MonthlyCostQuota  *string
}

type PricingRequest struct {
	ModelID          int
	ProviderID       int
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	At               time.Time
	PricingOverride  map[string]any
}

type PricingResult struct {
	Amount        string
	Currency      string
	PricingRuleID *int
}

type Store interface {
	CreatePlan(ctx context.Context, input CreateStoredPlan) (SubscriptionPlan, error)
	FindPlanByID(ctx context.Context, id int) (SubscriptionPlan, error)
	ListPlans(ctx context.Context) ([]SubscriptionPlan, error)
	CreateUserSubscription(ctx context.Context, input CreateStoredSubscription) (UserSubscription, error)
	FindUserSubscriptionBySource(ctx context.Context, sourceType string, sourceID string) (UserSubscription, error)
	ListUserSubscriptions(ctx context.Context) ([]UserSubscription, error)
	ListUserSubscriptionsByUser(ctx context.Context, userID int) ([]UserSubscription, error)
	ListActiveUserSubscriptions(ctx context.Context, userID int, at time.Time) ([]UserSubscription, error)
	CreatePricingRule(ctx context.Context, input PricingRule) (PricingRule, error)
	ListPricingRules(ctx context.Context) ([]PricingRule, error)
}
