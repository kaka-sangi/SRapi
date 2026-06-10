package domain

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

func TestApplyCostUsageResetsExpiredWindowsAndAddsCost(t *testing.T) {
	at := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	recent := at.Add(-time.Hour)
	expired5h := at.Add(-6 * time.Hour)
	expired1d := at.Add(-25 * time.Hour)
	expired7d := at.Add(-8 * 24 * time.Hour)

	got := ApplyCostUsage(contract.APIKey{
		CostUsed:          "1.25000000",
		CostUsed5h:        "2.00000000",
		CostWindowStart5h: &expired5h,
		CostUsed1d:        "3.00000000",
		CostWindowStart1d: &expired1d,
		CostUsed7d:        "4.00000000",
		CostWindowStart7d: &expired7d,
	}, contract.CostUsageUpdate{BillableCost: "0.125", OccurredAt: at})

	if got.CostUsed != "1.37500000" {
		t.Fatalf("CostUsed = %s", got.CostUsed)
	}
	if got.CostUsed5h != "0.12500000" || got.CostUsed1d != "0.12500000" || got.CostUsed7d != "0.12500000" {
		t.Fatalf("window costs = %s/%s/%s", got.CostUsed5h, got.CostUsed1d, got.CostUsed7d)
	}
	if !got.CostWindowStart5h.Equal(at) || !got.CostWindowStart1d.Equal(at) || !got.CostWindowStart7d.Equal(at) {
		t.Fatalf("expired windows were not reset to occurrence time")
	}

	got = ApplyCostUsage(contract.APIKey{
		CostUsed:          "1",
		CostUsed5h:        "2",
		CostWindowStart5h: &recent,
		CostUsed1d:        "3",
		CostWindowStart1d: &recent,
		CostUsed7d:        "4",
		CostWindowStart7d: &recent,
	}, contract.CostUsageUpdate{BillableCost: "-9.99", OccurredAt: at})

	if got.CostUsed != "-8.99000000" || got.CostUsed5h != "-7.99000000" || got.CostUsed1d != "-6.99000000" || got.CostUsed7d != "-5.99000000" {
		t.Fatalf("negative cost boundary changed, got %s/%s/%s/%s", got.CostUsed, got.CostUsed5h, got.CostUsed1d, got.CostUsed7d)
	}
}

func TestResetExpiredCostWindowsInitializesNilStarts(t *testing.T) {
	at := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got := ResetExpiredCostWindows(contract.APIKey{
		CostUsed:   "2.5",
		CostUsed5h: "bad",
		CostUsed1d: "3.125",
		CostUsed7d: "4.5",
	}, at)

	if got.CostUsed != "2.5" || got.CostUsed5h != "0.00000000" || got.CostUsed1d != "0.00000000" || got.CostUsed7d != "0.00000000" {
		t.Fatalf("normalized costs = %s/%s/%s/%s", got.CostUsed, got.CostUsed5h, got.CostUsed1d, got.CostUsed7d)
	}
	if got.CostWindowStart5h == nil || got.CostWindowStart1d == nil || got.CostWindowStart7d == nil {
		t.Fatalf("nil starts were not initialized")
	}
}
