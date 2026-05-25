package balancecharger

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usersmemory "github.com/srapi/srapi/apps/api/internal/modules/users/store/memory"
)

func TestRunOnceSuspendsAndAuditsNegativeBalanceUser(t *testing.T) {
	users := usersmemory.New()
	audit := auditmemory.New()
	user, err := users.Create(t.Context(), userscontract.CreateStoredUser{
		Email:        "negative@srapi.local",
		Name:         "Negative Balance",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "-0.15000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	store := &fakeUsageChargeStore{
		pending: []billingcontract.PendingUsageCharge{{
			UsageLogID: 42,
			RequestID:  "req_negative_balance",
			UserID:     user.ID,
			Cost:       "0.25000000",
			Currency:   "USD",
		}},
		result: billingcontract.ChargeUsageResult{
			UserID:             user.ID,
			ChargedUsageLogIDs: []int{42},
			BalanceBefore:      "0.10000000",
			BalanceAfter:       "-0.15000000",
			UserDisabled:       true,
			LedgerEntry: billingcontract.LedgerEntry{
				ID:            7,
				Type:          billingcontract.LedgerTypeUsageCharge,
				Amount:        "0.25000000",
				Currency:      "USD",
				ReferenceType: "usage_log_batch",
				ReferenceID:   "42",
				CreatedAt:     time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Users: users,
		Audit: audit,
		Clock: fixedClock{now: time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Charged != 1 {
		t.Fatalf("unexpected charge result: %+v", result)
	}

	updated, err := users.FindByID(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if updated.Status != userscontract.StatusDisabled {
		t.Fatalf("expected user disabled, got %+v", updated)
	}

	logs, err := audit.List(t.Context())
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one audit log, got %+v", logs)
	}
	if logs[0].Action != "user.suspend" || logs[0].After["reason"] != "insufficient_balance" || logs[0].After["status"] != "disabled" {
		t.Fatalf("unexpected audit log: %+v", logs[0])
	}
}

func TestRunOnceDrainsConfiguredBatches(t *testing.T) {
	store := &fakeUsageChargeStore{
		pending: makePendingCharges(10000),
	}
	worker, err := New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}

	if result.Selected != 10000 || result.Charged != 10000 {
		t.Fatalf("expected one run to charge 10000 pending usage logs, got %+v", result)
	}
	if len(store.pending) != 0 {
		t.Fatalf("expected pending queue drained, got %d entries", len(store.pending))
	}
	if store.listCalls != defaultMaxBatches || store.chargeCalls != defaultMaxBatches {
		t.Fatalf("expected %d list/charge batches, got list=%d charge=%d", defaultMaxBatches, store.listCalls, store.chargeCalls)
	}
}

type fakeUsageChargeStore struct {
	pending     []billingcontract.PendingUsageCharge
	result      billingcontract.ChargeUsageResult
	listCalls   int
	chargeCalls int
}

func (s *fakeUsageChargeStore) ListPendingUsageCharges(_ context.Context, limit int) ([]billingcontract.PendingUsageCharge, error) {
	s.listCalls++
	if limit <= 0 || limit > len(s.pending) {
		limit = len(s.pending)
	}
	out := make([]billingcontract.PendingUsageCharge, limit)
	copy(out, s.pending[:limit])
	return out, nil
}

func (s *fakeUsageChargeStore) ChargeUsage(_ context.Context, req billingcontract.ChargeUsageRequest) (billingcontract.ChargeUsageResult, error) {
	s.chargeCalls++
	result := s.result
	if result.UserID == 0 {
		result.UserID = req.UserID
	}
	if result.LedgerEntry.ID == 0 {
		result.LedgerEntry.ID = s.chargeCalls
	}
	result.ChargedUsageLogIDs = append([]int(nil), req.UsageLogIDs...)
	if result.LedgerEntry.ReferenceID == "" {
		result.LedgerEntry.ReferenceID = req.ReferenceID
	}
	charged := map[int]struct{}{}
	for _, id := range req.UsageLogIDs {
		charged[id] = struct{}{}
	}
	remaining := s.pending[:0]
	for _, item := range s.pending {
		if _, ok := charged[item.UsageLogID]; !ok {
			remaining = append(remaining, item)
		}
	}
	s.pending = remaining
	return result, nil
}

func makePendingCharges(count int) []billingcontract.PendingUsageCharge {
	out := make([]billingcontract.PendingUsageCharge, 0, count)
	for id := 1; id <= count; id++ {
		out = append(out, billingcontract.PendingUsageCharge{
			UsageLogID: id,
			RequestID:  "req_batch",
			UserID:     1,
			Cost:       "0.00010000",
			Currency:   "USD",
		})
	}
	return out
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
