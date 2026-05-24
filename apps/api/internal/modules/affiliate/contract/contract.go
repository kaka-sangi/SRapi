package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("affiliate resource not found")
	ErrConflict            = errors.New("affiliate resource conflict")
	ErrInsufficientBalance = errors.New("affiliate balance insufficient")
)

type InviteCodeStatus string

const (
	InviteCodeStatusActive   InviteCodeStatus = "active"
	InviteCodeStatusDisabled InviteCodeStatus = "disabled"
	InviteCodeStatusExpired  InviteCodeStatus = "expired"
)

type RelationshipStatus string

const (
	RelationshipStatusActive   RelationshipStatus = "active"
	RelationshipStatusDisabled RelationshipStatus = "disabled"
)

type RuleStatus string

const (
	RuleStatusActive   RuleStatus = "active"
	RuleStatusDisabled RuleStatus = "disabled"
	RuleStatusArchived RuleStatus = "archived"
)

type TriggerType string

const (
	TriggerTypePaymentPaid TriggerType = "payment_paid"
)

type LedgerType string

const (
	LedgerTypeAccrue             LedgerType = "accrue"
	LedgerTypeSettle             LedgerType = "settle"
	LedgerTypeTransferToBalance  LedgerType = "transfer_to_balance"
	LedgerTypeWithdraw           LedgerType = "withdraw"
	LedgerTypeRefundCompensation LedgerType = "refund_compensation"
	LedgerTypeManualAdjustment   LedgerType = "manual_adjustment"
)

type LedgerStatus string

const (
	LedgerStatusPending     LedgerStatus = "pending"
	LedgerStatusSettled     LedgerStatus = "settled"
	LedgerStatusCanceled    LedgerStatus = "canceled"
	LedgerStatusCompensated LedgerStatus = "compensated"
)

type InviteCode struct {
	ID        int
	UserID    int
	Code      string
	Status    InviteCodeStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt *time.Time
}

type InviteRelationship struct {
	ID            int
	InviterUserID int
	InviteeUserID int
	InviteCodeID  int
	Status        RelationshipStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
	FirstPaidAt   *time.Time
}

type AffiliateRule struct {
	ID              int
	Name            string
	Status          RuleStatus
	TriggerType     TriggerType
	Rate            string
	FixedAmount     string
	Currency        string
	MaxRebateAmount string
	ValidFrom       *time.Time
	ValidTo         *time.Time
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AffiliateLedger struct {
	ID             int
	UserID         int
	RelatedUserID  int
	PaymentOrderID *int
	SubscriptionID *int
	Type           LedgerType
	Amount         string
	Currency       string
	Status         LedgerStatus
	ReferenceID    string
	Metadata       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
	SettledAt      *time.Time
}

type CreateInviteCodeRequest struct {
	UserID    int
	Code      string
	ExpiresAt *time.Time
}

type BindInviteRequest struct {
	InviteeUserID int
	Code          string
}

type CreateRuleRequest struct {
	Name            string
	Status          *RuleStatus
	TriggerType     TriggerType
	Rate            string
	FixedAmount     string
	Currency        string
	MaxRebateAmount string
	ValidFrom       *time.Time
	ValidTo         *time.Time
	Metadata        map[string]any
}

type AccrueRebateRequest struct {
	OrderID               int
	OrderNo               string
	InviteeUserID         int
	Amount                string
	Currency              string
	PaidAt                time.Time
	ProviderTransactionID string
}

type CompensateRefundRequest struct {
	OrderID      int
	RefundID     string
	UserID       int
	RefundAmount string
	Currency     string
	Reason       string
	RefundedAt   time.Time
}

type TransferToBalanceRequest struct {
	UserID         int
	Amount         string
	Currency       string
	IdempotencyKey string
	RequestedAt    time.Time
}

type RebateResult struct {
	Applied bool
	Reason  string
	Ledgers []AffiliateLedger
}

type TransferToBalanceInput struct {
	UserID      int
	Amount      string
	Currency    string
	ReferenceID string
	Metadata    map[string]any
	CreatedAt   time.Time
}

type TransferToBalanceResult struct {
	Applied         bool
	Reason          string
	AffiliateLedger AffiliateLedger
	BillingLedgerID int
	BalanceBefore   string
	BalanceAfter    string
}

type Store interface {
	CreateInviteCode(ctx context.Context, input InviteCode) (InviteCode, error)
	FindInviteCodeByCode(ctx context.Context, code string) (InviteCode, error)
	CreateRelationship(ctx context.Context, input InviteRelationship) (InviteRelationship, error)
	FindRelationshipByInvitee(ctx context.Context, inviteeUserID int) (InviteRelationship, error)
	ListRelationships(ctx context.Context) ([]InviteRelationship, error)
	MarkRelationshipFirstPaid(ctx context.Context, id int, firstPaidAt time.Time) (InviteRelationship, error)
	CreateRule(ctx context.Context, input AffiliateRule) (AffiliateRule, error)
	GetEffectiveRule(ctx context.Context, trigger TriggerType, currency string, at time.Time) (AffiliateRule, error)
	AppendLedger(ctx context.Context, input AffiliateLedger) (AffiliateLedger, bool, error)
	TransferToBalance(ctx context.Context, input TransferToBalanceInput) (TransferToBalanceResult, bool, error)
	ListLedgers(ctx context.Context) ([]AffiliateLedger, error)
	ListLedgersByPaymentOrder(ctx context.Context, paymentOrderID int) ([]AffiliateLedger, error)
}
