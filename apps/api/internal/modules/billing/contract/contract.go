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

type Store interface {
	Create(ctx context.Context, input LedgerEntry) (LedgerEntry, error)
	List(ctx context.Context) ([]LedgerEntry, error)
}
