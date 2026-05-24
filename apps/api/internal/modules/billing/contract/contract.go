package contract

import (
	"context"
	"time"
)

type LedgerType string

const (
	LedgerTypeUsageCharge       LedgerType = "usage_charge"
	LedgerTypePaymentCredit     LedgerType = "payment_credit"
	LedgerTypeRefund            LedgerType = "refund"
	LedgerTypeAdjustment        LedgerType = "adjustment"
	LedgerTypeCompensation      LedgerType = "compensation"
	LedgerTypeAffiliateTransfer LedgerType = "affiliate_transfer"
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
