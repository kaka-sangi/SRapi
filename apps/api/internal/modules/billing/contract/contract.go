package contract

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("billing resource not found")

type BillingMode string

const (
	BillingModeToken      BillingMode = "token"
	BillingModePerRequest BillingMode = "per_request"
	BillingModeImage      BillingMode = "image"
)

type LedgerType string

const (
	LedgerTypeUsageCharge       LedgerType = "usage_charge"
	LedgerTypePaymentCredit     LedgerType = "payment_credit"
	LedgerTypeRefund            LedgerType = "refund"
	LedgerTypeAdjustment        LedgerType = "adjustment"
	LedgerTypeCompensation      LedgerType = "compensation"
	LedgerTypeAffiliateTransfer LedgerType = "affiliate_transfer"
	LedgerTypeRedeemCodeCredit  LedgerType = "redeem_code_credit"
)

type LedgerEntry struct {
	ID            int
	UserID        int
	Type          LedgerType
	Amount        string
	Currency      string
	BalanceBefore string
	BalanceAfter  string
	ReferenceType string
	ReferenceID   string
	Metadata      map[string]any
	CreatedAt     time.Time
}

type PricingRule struct {
	ID                                int
	ModelID                           int
	ModelFamily                       string
	ProviderID                        int
	BillingMode                       BillingMode
	InputPricePerMillionTokens        string
	OutputPricePerMillionTokens       string
	CacheReadPricePerMillionTokens    string
	CacheWritePricePerMillionTokens   string
	CacheWrite5mPricePerMillionTokens string
	CacheWrite1hPricePerMillionTokens string
	ImageOutputPricePerMillionTokens  string
	PerRequestPrice                   string
	ServiceTierMultipliers            map[string]string
	LongContextThresholdTokens        *int
	LongContextMultiplier             string
	Intervals                         []PricingInterval
	Currency                          string
	EffectiveFrom                     *time.Time
	EffectiveTo                       *time.Time
	CreatedAt                         time.Time
	UpdatedAt                         time.Time
}

type PricingInterval struct {
	ID                              int
	PricingRuleID                   int
	MinTokens                       int
	MaxTokens                       *int
	TierLabel                       string
	ImageSize                       string
	InputPricePerMillionTokens      string
	OutputPricePerMillionTokens     string
	CacheReadPricePerMillionTokens  string
	CacheWritePricePerMillionTokens string
	PerImagePrice                   string
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

type CreatePricingRuleRequest struct {
	ModelID                           int
	ProviderID                        int
	BillingMode                       BillingMode
	InputPricePerMillionTokens        string
	OutputPricePerMillionTokens       string
	CacheReadPricePerMillionTokens    string
	CacheWritePricePerMillionTokens   string
	CacheWrite5mPricePerMillionTokens string
	CacheWrite1hPricePerMillionTokens string
	ImageOutputPricePerMillionTokens  string
	PerRequestPrice                   string
	ServiceTierMultipliers            map[string]string
	LongContextThresholdTokens        *int
	LongContextMultiplier             string
	Intervals                         []PricingInterval
	Currency                          string
	EffectiveFrom                     *time.Time
	EffectiveTo                       *time.Time
}

// UpdatePricingRuleRequest carries a partial pricing-rule edit: nil pointer
// means "leave unchanged".
type UpdatePricingRuleRequest struct {
	BillingMode                       *BillingMode
	InputPricePerMillionTokens        *string
	OutputPricePerMillionTokens       *string
	CacheReadPricePerMillionTokens    *string
	CacheWritePricePerMillionTokens   *string
	CacheWrite5mPricePerMillionTokens *string
	CacheWrite1hPricePerMillionTokens *string
	ImageOutputPricePerMillionTokens  *string
	PerRequestPrice                   *string
	ServiceTierMultipliers            *map[string]string
	LongContextThresholdTokens        **int
	LongContextMultiplier             *string
	Intervals                         *[]PricingInterval
	Currency                          *string
	EffectiveFrom                     **time.Time
	EffectiveTo                       **time.Time
}

type PricingRequest struct {
	ModelID            int
	ModelFamily        string
	ProviderID         int
	RequestedModel     string
	UpstreamModel      string
	BillingModelSource string
	ServiceTier        string
	InputTokens        int
	OutputTokens       int
	CacheReadTokens    int
	CacheWriteTokens   int
	CacheWrite5mTokens int
	CacheWrite1hTokens int
	ImageOutputTokens  int
	ImageCount         int
	ImageSize          string
	At                 time.Time
	PricingOverride    map[string]any
}

// PricingRuleQuery narrows the hot-path price lookup. Stores should apply the
// provider, effective window, model id, and family/name predicates before
// materializing candidate rules.
type PricingRuleQuery struct {
	ModelID            int
	ModelFamily        string
	RequestedModel     string
	UpstreamModel      string
	BillingModelSource string
	ProviderID         int
	At                 time.Time
}

type PricingResult struct {
	Amount         string
	Currency       string
	PricingRuleID  *int
	BillingMode    BillingMode
	InputCost      string
	OutputCost     string
	CacheReadCost  string
	CacheWriteCost string
}

type GatewayPricingRequest struct {
	PricingRequest
	RateMultiplier       string
	Success              bool
	AllowanceMode        string
	DailyAllowanceQuota  *string
	WeeklyAllowanceQuota *string
	AllowanceQuota       *string
	DailyUsedCost        string
	WeeklyUsedCost       string
	UsedCost             string
	Estimated            bool
}

type GatewayCostRequest struct {
	Amount               string
	Currency             string
	PricingRuleID        *int
	BillingMode          BillingMode
	InputCost            string
	OutputCost           string
	CacheReadCost        string
	CacheWriteCost       string
	Source               string
	Estimated            bool
	RateMultiplier       string
	Success              bool
	AllowanceMode        string
	DailyAllowanceQuota  *string
	WeeklyAllowanceQuota *string
	AllowanceQuota       *string
	DailyUsedCost        string
	WeeklyUsedCost       string
	UsedCost             string
}

type GatewayPricingResult struct {
	Amount         string
	Currency       string
	PricingRuleID  *int
	BillingMode    BillingMode
	InputCost      string
	OutputCost     string
	CacheReadCost  string
	CacheWriteCost string
	Source         string
	Estimated      bool
	ActualCost     string
	BillableCost   string
}

type RecordRequest struct {
	UserID        int
	Type          LedgerType
	Amount        string
	Currency      string
	BalanceBefore string
	BalanceAfter  string
	ReferenceType string
	ReferenceID   string
	Metadata      map[string]any
}

type PendingUsageCharge struct {
	UsageLogID int
	RequestID  string
	AttemptNo  int
	UserID     int
	Cost       string
	Currency   string
	CreatedAt  time.Time
}

type ChargeUsageRequest struct {
	UserID        int
	Currency      string
	UsageLogIDs   []int
	ChargedAt     time.Time
	ReferenceID   string
	ReferenceType string
}

type ChargeUsageResult struct {
	UserID             int
	LedgerEntry        LedgerEntry
	ChargedUsageLogIDs []int
	BalanceBefore      string
	BalanceAfter       string
	UserDisabled       bool
}

type ChargePendingUsageRequest struct {
	Limit     int
	ChargedAt time.Time
}

type ChargePendingUsageResult struct {
	Selected int
	Charged  int
	Batches  []ChargeUsageResult
}

type Store interface {
	Create(ctx context.Context, input LedgerEntry) (LedgerEntry, error)
	List(ctx context.Context) ([]LedgerEntry, error)
}

type UsageChargeStore interface {
	ListPendingUsageCharges(ctx context.Context, limit int) ([]PendingUsageCharge, error)
	ChargeUsage(ctx context.Context, req ChargeUsageRequest) (ChargeUsageResult, error)
}

type PricingStore interface {
	CreatePricingRule(ctx context.Context, input PricingRule) (PricingRule, error)
	UpdatePricingRule(ctx context.Context, id int, input UpdatePricingRuleRequest) (PricingRule, error)
	FindPricingRuleByID(ctx context.Context, id int) (PricingRule, error)
	QueryPricingRules(ctx context.Context, query PricingRuleQuery) ([]PricingRule, error)
	ListPricingRules(ctx context.Context) ([]PricingRule, error)
	DeletePricingRule(ctx context.Context, id int) error
}
