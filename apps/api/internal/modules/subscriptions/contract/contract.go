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
	DailyUsageUSD        string
	DailyWindowStart     *time.Time
	WeeklyUsageUSD       string
	WeeklyWindowStart    *time.Time
	MonthlyUsageUSD      string
	MonthlyWindowStart   *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// MaterializedUsage tracks subscription spend persisted on the active
// subscription row, avoiding hot-path usage-log rescans for allowance checks.
type MaterializedUsage struct {
	SubscriptionID     int
	UserID             int
	DailyUsageUSD      string
	DailyWindowStart   *time.Time
	WeeklyUsageUSD     string
	WeeklyWindowStart  *time.Time
	MonthlyUsageUSD    string
	MonthlyWindowStart *time.Time
}

// UsageDelta is the cost snapshot appended to active subscriptions after a
// successful gateway request has been priced.
type UsageDelta struct {
	UserID       int
	BillableCost string
	OccurredAt   time.Time
}

// Entitlement is the active query-cache row derived from a subscription snapshot.
type Entitlement struct {
	ID                   int
	UserID               int
	ScopeType            string
	ScopeID              int
	FeatureKey           string
	Value                map[string]any
	QuotaLimit           *string
	ExpiresAt            time.Time
	SourceSubscriptionID int
	CreatedAt            time.Time
	UpdatedAt            time.Time
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

// UpdatePlanRequest carries a partial plan edit: every field is optional, and a
// nil pointer means "leave unchanged". Mirrors the PATCH semantics of the
// /admin/subscription-plans/{id} endpoint.
type UpdatePlanRequest struct {
	Name         *string
	Description  *string
	Price        *string
	Currency     *string
	ValidityDays *int
	Entitlements *map[string]any
	ForSale      *bool
	SortOrder    *int
	Status       *PlanStatus
}

// UpdateStoredPlan is the normalized, validated form of an UpdatePlanRequest
// handed to the store; nil fields are skipped during the update.
type UpdateStoredPlan struct {
	Name         *string
	Description  *string
	Price        *string
	Currency     *string
	ValidityDays *int
	Entitlements *map[string]any
	ForSale      *bool
	SortOrder    *int
	Status       *PlanStatus
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

type EntitlementCheckRequest struct {
	UserID             int
	ModelReferences    []string
	EstimatedTokens    int
	EstimatedCost      string
	TokensUsedInPeriod int
	CostUsedInPeriod   string
	MaterializedUsage  *MaterializedUsage
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
	// CostQuotaMode is "hard_cap" (default — deny when the monthly cost quota is
	// exceeded) or "allowance" (treat the quota as an included allowance and bill
	// the overage to balance instead of denying). WP-1180.
	CostQuotaMode string
}

// CostAllowance describes a user's active subscription cost allowance, used to
// split per-request cost into subscription-covered vs balance-billable.
type CostAllowance struct {
	Mode  string  // "" / "hard_cap" / "allowance"
	Quota *string // monthly cost quota (allowance ceiling), nil when unset
}

// ExpireSubscriptionsResult reports the outcome of a subscription expiration pass.
type ExpireSubscriptionsResult struct {
	Selected int
	Expired  int
}

// ReminderSubscriptionsResult reports the outcome of a subscription reminder pass.
type ReminderSubscriptionsResult struct {
	Selected int
	Enqueued int
}

type Store interface {
	CreatePlan(ctx context.Context, input CreateStoredPlan) (SubscriptionPlan, error)
	UpdatePlan(ctx context.Context, id int, input UpdateStoredPlan) (SubscriptionPlan, error)
	FindPlanByID(ctx context.Context, id int) (SubscriptionPlan, error)
	ListPlans(ctx context.Context) ([]SubscriptionPlan, error)
	DeletePlan(ctx context.Context, id int) error
	CreateUserSubscription(ctx context.Context, input CreateStoredSubscription) (UserSubscription, error)
	FindUserSubscriptionBySource(ctx context.Context, sourceType string, sourceID string) (UserSubscription, error)
	ListUserSubscriptions(ctx context.Context) ([]UserSubscription, error)
	ListUserSubscriptionsByUser(ctx context.Context, userID int) ([]UserSubscription, error)
	ListActiveUserSubscriptions(ctx context.Context, userID int, at time.Time) ([]UserSubscription, error)
	MaterializedUsageForUser(ctx context.Context, userID int, at time.Time) (MaterializedUsage, error)
	IncrementMaterializedUsage(ctx context.Context, delta UsageDelta) (MaterializedUsage, error)
	ListActiveEntitlements(ctx context.Context, userID int, at time.Time) ([]Entitlement, error)
	ListExpiredActiveUserSubscriptions(ctx context.Context, now time.Time) ([]UserSubscription, error)
	ListActiveUserSubscriptionsExpiringBetween(ctx context.Context, from time.Time, until time.Time) ([]UserSubscription, error)
	ExpireUserSubscription(ctx context.Context, id int, now time.Time) (UserSubscription, bool, error)
	DeleteUserSubscription(ctx context.Context, id int) error
}
