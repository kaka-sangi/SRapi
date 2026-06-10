package domain

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/usagewindow"
)

func TestApplyUsageDeltaResetsCalendarWindowsAndAddsCost(t *testing.T) {
	at := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	sameDay := usagewindow.StartOfDayUTC(at)
	oldWeek := usagewindow.StartOfWeekUTC(at).Add(-7 * 24 * time.Hour)
	oldMonth := usagewindow.StartOfMonthUTC(at).AddDate(0, -1, 0)

	got := ApplyUsageDelta(contract.UserSubscription{
		DailyUsageUSD:      "1.00000000",
		DailyWindowStart:   &sameDay,
		WeeklyUsageUSD:     "2.00000000",
		WeeklyWindowStart:  &oldWeek,
		MonthlyUsageUSD:    "3.00000000",
		MonthlyWindowStart: &oldMonth,
	}, contract.UsageDelta{BillableCost: "0.333333333", OccurredAt: at})

	if got.DailyUsageUSD != "1.33333333" {
		t.Fatalf("daily = %s", got.DailyUsageUSD)
	}
	if got.WeeklyUsageUSD != "0.33333333" || got.MonthlyUsageUSD != "0.33333333" {
		t.Fatalf("weekly/monthly = %s/%s", got.WeeklyUsageUSD, got.MonthlyUsageUSD)
	}
	if !got.WeeklyWindowStart.Equal(usagewindow.StartOfWeekUTC(at)) || !got.MonthlyWindowStart.Equal(usagewindow.StartOfMonthUTC(at)) {
		t.Fatalf("expired calendar windows were not reset")
	}
}

func TestResetExpiredUsageInitializesNilStartsAndPreservesNegativeCostBoundary(t *testing.T) {
	at := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got := ResetExpiredUsage(contract.UserSubscription{
		DailyUsageUSD:   "-1",
		WeeklyUsageUSD:  "bad",
		MonthlyUsageUSD: "4.25",
	}, at)

	if got.DailyUsageUSD != "0.00000000" || got.WeeklyUsageUSD != "0.00000000" || got.MonthlyUsageUSD != "0.00000000" {
		t.Fatalf("nil windows should reset costs, got %s/%s/%s", got.DailyUsageUSD, got.WeeklyUsageUSD, got.MonthlyUsageUSD)
	}
	if got.DailyWindowStart == nil || got.WeeklyWindowStart == nil || got.MonthlyWindowStart == nil {
		t.Fatalf("nil starts were not initialized")
	}

	got = ApplyUsageDelta(got, contract.UsageDelta{BillableCost: "-9", OccurredAt: at})
	if got.DailyUsageUSD != "-9.00000000" || got.WeeklyUsageUSD != "-9.00000000" || got.MonthlyUsageUSD != "-9.00000000" {
		t.Fatalf("negative delta boundary changed, got %s/%s/%s", got.DailyUsageUSD, got.WeeklyUsageUSD, got.MonthlyUsageUSD)
	}
}
