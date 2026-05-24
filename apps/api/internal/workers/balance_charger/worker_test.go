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

type fakeUsageChargeStore struct {
	pending []billingcontract.PendingUsageCharge
	result  billingcontract.ChargeUsageResult
}

func (s *fakeUsageChargeStore) ListPendingUsageCharges(context.Context, int) ([]billingcontract.PendingUsageCharge, error) {
	out := make([]billingcontract.PendingUsageCharge, len(s.pending))
	copy(out, s.pending)
	return out, nil
}

func (s *fakeUsageChargeStore) ChargeUsage(_ context.Context, req billingcontract.ChargeUsageRequest) (billingcontract.ChargeUsageResult, error) {
	result := s.result
	result.ChargedUsageLogIDs = append([]int(nil), req.UsageLogIDs...)
	if result.LedgerEntry.ReferenceID == "" {
		result.LedgerEntry.ReferenceID = req.ReferenceID
	}
	return result, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
