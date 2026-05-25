package service

import (
	"errors"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliatememory "github.com/srapi/srapi/apps/api/internal/modules/affiliate/store/memory"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
)

func TestAffiliateRejectsSelfInviteAndDuplicateBinding(t *testing.T) {
	h := newHarness(t)
	if _, err := h.affiliate.CreateInviteCode(t.Context(), contract.CreateInviteCodeRequest{UserID: 1, Code: "INVITE1"}); err != nil {
		t.Fatalf("create invite code: %v", err)
	}
	if _, err := h.affiliate.BindInvite(t.Context(), contract.BindInviteRequest{InviteeUserID: 1, Code: "INVITE1"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected self invite rejection, got %v", err)
	}
	if _, err := h.affiliate.CreateInviteCode(t.Context(), contract.CreateInviteCodeRequest{UserID: 3, Code: "INVITE3"}); err != nil {
		t.Fatalf("create second invite code: %v", err)
	}
	if _, err := h.affiliate.BindInvite(t.Context(), contract.BindInviteRequest{InviteeUserID: 2, Code: "INVITE1"}); err != nil {
		t.Fatalf("bind invite: %v", err)
	}
	if _, err := h.affiliate.BindInvite(t.Context(), contract.BindInviteRequest{InviteeUserID: 2, Code: "INVITE3"}); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected duplicate binding conflict, got %v", err)
	}
}

func TestAccrueRebateFromPaymentIsIdempotent(t *testing.T) {
	h := newHarness(t)
	seedInviteAndRule(t, h)

	result, err := h.affiliate.AccrueRebate(t.Context(), contract.AccrueRebateRequest{
		OrderID:               101,
		OrderNo:               "pay_101",
		InviteeUserID:         20,
		Amount:                "100.00",
		Currency:              "usd",
		PaidAt:                h.clock.now,
		ProviderTransactionID: "txn_101",
	})
	if err != nil {
		t.Fatalf("accrue rebate: %v", err)
	}
	if !result.Applied || len(result.Ledgers) != 1 || result.Ledgers[0].Amount != "10.00000000" {
		t.Fatalf("unexpected accrual result: %+v", result)
	}

	duplicate, err := h.affiliate.AccrueRebate(t.Context(), contract.AccrueRebateRequest{
		OrderID:       101,
		OrderNo:       "pay_101",
		InviteeUserID: 20,
		Amount:        "100.00",
		Currency:      "USD",
		PaidAt:        h.clock.now,
	})
	if err != nil {
		t.Fatalf("duplicate accrue rebate: %v", err)
	}
	if duplicate.Applied || duplicate.Reason != "duplicate_rebate" {
		t.Fatalf("expected duplicate no-op, got %+v", duplicate)
	}
	ledgers, err := h.affiliate.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list ledgers: %v", err)
	}
	if len(ledgers) != 1 {
		t.Fatalf("expected one ledger after duplicate accrual, got %+v", ledgers)
	}
	outbox, err := h.events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "AffiliateRebateAccrued" {
		t.Fatalf("expected one affiliate accrued event, got %+v", outbox)
	}
	relationship, err := h.store.FindRelationshipByInvitee(t.Context(), 20)
	if err != nil {
		t.Fatalf("find relationship: %v", err)
	}
	if relationship.FirstPaidAt == nil || !relationship.FirstPaidAt.Equal(h.clock.now) {
		t.Fatalf("expected first_paid_at to be set, got %+v", relationship)
	}
}

func TestRefundCompensationAppendsReverseLedgers(t *testing.T) {
	h := newHarness(t)
	seedInviteAndRule(t, h)
	if _, err := h.affiliate.AccrueRebate(t.Context(), contract.AccrueRebateRequest{
		OrderID:       202,
		OrderNo:       "pay_202",
		InviteeUserID: 20,
		Amount:        "100.00",
		Currency:      "USD",
		PaidAt:        h.clock.now,
	}); err != nil {
		t.Fatalf("accrue rebate: %v", err)
	}

	partial, err := h.affiliate.CompensateRefund(t.Context(), contract.CompensateRefundRequest{
		OrderID:      202,
		RefundID:     "refund_202_partial",
		UserID:       20,
		RefundAmount: "40.00",
		Currency:     "USD",
		Reason:       "customer request",
		RefundedAt:   h.clock.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("partial compensate: %v", err)
	}
	if !partial.Applied || len(partial.Ledgers) != 1 || partial.Ledgers[0].Amount != "-4.00000000" {
		t.Fatalf("unexpected partial compensation: %+v", partial)
	}
	duplicate, err := h.affiliate.CompensateRefund(t.Context(), contract.CompensateRefundRequest{
		OrderID:      202,
		RefundID:     "refund_202_partial",
		UserID:       20,
		RefundAmount: "40.00",
		Currency:     "USD",
		RefundedAt:   h.clock.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("duplicate compensate: %v", err)
	}
	if duplicate.Applied || duplicate.Reason != "duplicate_compensation" {
		t.Fatalf("expected duplicate compensation no-op, got %+v", duplicate)
	}
	full, err := h.affiliate.CompensateRefund(t.Context(), contract.CompensateRefundRequest{
		OrderID:      202,
		RefundID:     "refund_202_full",
		UserID:       20,
		RefundAmount: "100.00",
		Currency:     "USD",
		Reason:       "full refund",
		RefundedAt:   h.clock.now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("full compensate: %v", err)
	}
	if !full.Applied || len(full.Ledgers) != 1 || full.Ledgers[0].Amount != "-6.00000000" {
		t.Fatalf("expected remaining compensation capped at original rebate, got %+v", full)
	}
	ledgers, err := h.affiliate.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list ledgers: %v", err)
	}
	if len(ledgers) != 3 {
		t.Fatalf("expected accrual plus two compensations, got %+v", ledgers)
	}
	outbox, err := h.events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 3 {
		t.Fatalf("expected accrued and compensated events, got %+v", outbox)
	}
}

func TestTransferToBalanceWritesLedgerAndAuditIdempotently(t *testing.T) {
	h := newHarness(t)
	seedInviteAndRule(t, h)
	if _, err := h.affiliate.AccrueRebate(t.Context(), contract.AccrueRebateRequest{
		OrderID:       303,
		OrderNo:       "pay_303",
		InviteeUserID: 20,
		Amount:        "100.00",
		Currency:      "USD",
		PaidAt:        h.clock.now,
	}); err != nil {
		t.Fatalf("accrue rebate: %v", err)
	}

	result, err := h.affiliate.TransferToBalance(t.Context(), contract.TransferToBalanceRequest{
		UserID:         10,
		Amount:         "6.00",
		Currency:       "USD",
		IdempotencyKey: "transfer_303",
		RequestedAt:    h.clock.now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("transfer to balance: %v", err)
	}
	if !result.Applied || result.AffiliateLedger.Type != contract.LedgerTypeTransferToBalance || result.AffiliateLedger.Amount != "-6.00000000" {
		t.Fatalf("unexpected transfer result: %+v", result)
	}
	if result.BillingLedgerID == 0 || result.BalanceBefore != "0.00000000" || result.BalanceAfter != "6.00000000" {
		t.Fatalf("expected billing and balance evidence, got %+v", result)
	}

	duplicate, err := h.affiliate.TransferToBalance(t.Context(), contract.TransferToBalanceRequest{
		UserID:         10,
		Amount:         "6.00",
		Currency:       "USD",
		IdempotencyKey: "transfer_303",
		RequestedAt:    h.clock.now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("duplicate transfer to balance: %v", err)
	}
	if duplicate.Applied || duplicate.Reason != "duplicate_transfer" || duplicate.AffiliateLedger.ID != result.AffiliateLedger.ID {
		t.Fatalf("expected duplicate transfer no-op, got %+v", duplicate)
	}

	if _, err := h.affiliate.TransferToBalance(t.Context(), contract.TransferToBalanceRequest{
		UserID:         10,
		Amount:         "5.00",
		Currency:       "USD",
		IdempotencyKey: "transfer_overdraft",
	}); !errors.Is(err, contract.ErrInsufficientBalance) {
		t.Fatalf("expected insufficient affiliate balance, got %v", err)
	}
	ledgers, err := h.affiliate.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list ledgers: %v", err)
	}
	if len(ledgers) != 2 {
		t.Fatalf("expected accrual and transfer ledger only, got %+v", ledgers)
	}
	auditRows, err := h.audit.List(t.Context())
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(auditRows) != 2 || auditRows[1].Action != "affiliate_rebate.transfer_to_balance" {
		t.Fatalf("expected transfer audit after accrual audit, got %+v", auditRows)
	}
}

func TestAffiliateSummaryGroupsLedgerAmountsByCurrency(t *testing.T) {
	h := newHarness(t)
	seedInviteAndRule(t, h)
	if _, err := h.affiliate.AccrueRebate(t.Context(), contract.AccrueRebateRequest{
		OrderID:       404,
		OrderNo:       "pay_404",
		InviteeUserID: 20,
		Amount:        "100.00",
		Currency:      "USD",
		PaidAt:        h.clock.now,
	}); err != nil {
		t.Fatalf("accrue rebate: %v", err)
	}
	if _, err := h.affiliate.CompensateRefund(t.Context(), contract.CompensateRefundRequest{
		OrderID:      404,
		RefundID:     "refund_404",
		UserID:       20,
		RefundAmount: "40.00",
		Currency:     "USD",
		RefundedAt:   h.clock.now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("compensate refund: %v", err)
	}
	if _, err := h.affiliate.TransferToBalance(t.Context(), contract.TransferToBalanceRequest{
		UserID:         10,
		Amount:         "3.00",
		Currency:       "USD",
		IdempotencyKey: "transfer_404",
		RequestedAt:    h.clock.now.Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("transfer to balance: %v", err)
	}

	summary, err := h.affiliate.GetSummary(t.Context(), 10)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary.UserID != 10 || len(summary.Balances) != 1 {
		t.Fatalf("unexpected summary shape: %+v", summary)
	}
	balance := summary.Balances[0]
	if balance.Currency != "USD" ||
		balance.AvailableBalance != "3.00000000" ||
		balance.AccruedAmount != "10.00000000" ||
		balance.RefundCompensatedAmount != "4.00000000" ||
		balance.TransferredToBalanceAmount != "3.00000000" {
		t.Fatalf("unexpected summary balance: %+v", balance)
	}
}

type harness struct {
	store     *affiliatememory.Store
	affiliate *Service
	audit     *auditservice.Service
	events    *eventsservice.Service
	clock     fixedClock
}

func newHarness(t *testing.T) harness {
	t.Helper()
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	store := affiliatememory.New()
	auditSvc, err := auditservice.New(auditmemory.New(), clock)
	if err != nil {
		t.Fatalf("new audit service: %v", err)
	}
	eventsSvc, err := eventsservice.New(eventsmemory.New(), clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	affiliateSvc, err := New(store, Dependencies{Audit: auditSvc, Events: eventsSvc}, clock)
	if err != nil {
		t.Fatalf("new affiliate service: %v", err)
	}
	return harness{store: store, affiliate: affiliateSvc, audit: auditSvc, events: eventsSvc, clock: clock}
}

func seedInviteAndRule(t *testing.T, h harness) {
	t.Helper()
	if _, err := h.affiliate.CreateInviteCode(t.Context(), contract.CreateInviteCodeRequest{UserID: 10, Code: "INVITE10"}); err != nil {
		t.Fatalf("create invite code: %v", err)
	}
	if _, err := h.affiliate.BindInvite(t.Context(), contract.BindInviteRequest{InviteeUserID: 20, Code: "INVITE10"}); err != nil {
		t.Fatalf("bind invite: %v", err)
	}
	if _, err := h.affiliate.CreateRule(t.Context(), contract.CreateRuleRequest{
		Name:        "ten-percent",
		TriggerType: contract.TriggerTypePaymentPaid,
		Rate:        "0.10",
		Currency:    "USD",
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
