package service_test

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
)

func TestRecordUsageChargePreservesDecimalStrings(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)}
	svc, err := service.New(billingmemory.New(), clock)
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}

	entry, err := svc.Record(t.Context(), contract.RecordRequest{
		UserID:        1,
		Type:          contract.LedgerTypeUsageCharge,
		Amount:        "0.00012345",
		Currency:      "USD",
		BalanceBefore: "10.00000000",
		BalanceAfter:  "9.99987655",
		ReferenceType: "usage_log",
		ReferenceID:   "42",
		Metadata: map[string]any{
			"request_id":      "req_billing_usage",
			"total_tokens":    17,
			"usage_estimated": false,
		},
	})
	if err != nil {
		t.Fatalf("record billing ledger: %v", err)
	}

	if entry.Amount != "0.00012345" || entry.BalanceBefore != "10.00000000" || entry.BalanceAfter != "9.99987655" {
		t.Fatalf("expected numeric-safe decimal strings, got %+v", entry)
	}
	if entry.ReferenceType != "usage_log" || entry.ReferenceID != "42" || !entry.CreatedAt.Equal(clock.now) {
		t.Fatalf("unexpected usage ledger reference: %+v", entry)
	}
	if entry.Metadata["request_id"] != "req_billing_usage" {
		t.Fatalf("expected gateway metadata, got %+v", entry.Metadata)
	}
}

func TestRecordDefaultsMoneyWithoutUsingFloatFields(t *testing.T) {
	svc, err := service.New(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}

	entry, err := svc.Record(t.Context(), contract.RecordRequest{
		UserID: 1,
		Type:   contract.LedgerTypeAdjustment,
	})
	if err != nil {
		t.Fatalf("record billing ledger: %v", err)
	}
	if entry.Amount != "0.00000000" || entry.BalanceBefore != "0.00000000" || entry.BalanceAfter != "0.00000000" || entry.Currency != "USD" {
		t.Fatalf("unexpected money defaults: %+v", entry)
	}
}

func TestRecordRejectsLedgerWithoutUserOrType(t *testing.T) {
	svc, err := service.New(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}
	if _, err := svc.Record(t.Context(), contract.RecordRequest{UserID: 1}); err == nil {
		t.Fatal("expected missing ledger type to be rejected")
	}
	if _, err := svc.Record(t.Context(), contract.RecordRequest{Type: contract.LedgerTypeUsageCharge}); err == nil {
		t.Fatal("expected missing user id to be rejected")
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
