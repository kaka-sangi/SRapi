package domain

import (
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	"github.com/srapi/srapi/apps/api/internal/pkg/usagewindow"
)

// MaterializedUsageFromSubscription returns the subscription's persisted usage
// counters with expired windows reset for at.
func MaterializedUsageFromSubscription(subscription contract.UserSubscription, at time.Time) contract.MaterializedUsage {
	updated := ResetExpiredUsage(subscription, at)
	return contract.MaterializedUsage{
		SubscriptionID:     updated.ID,
		UserID:             updated.UserID,
		DailyUsageUSD:      money.NormalizeAmount(updated.DailyUsageUSD),
		DailyWindowStart:   cloneTime(updated.DailyWindowStart),
		WeeklyUsageUSD:     money.NormalizeAmount(updated.WeeklyUsageUSD),
		WeeklyWindowStart:  cloneTime(updated.WeeklyWindowStart),
		MonthlyUsageUSD:    money.NormalizeAmount(updated.MonthlyUsageUSD),
		MonthlyWindowStart: cloneTime(updated.MonthlyWindowStart),
	}
}

// ApplyUsageDelta resets expired windows and adds the billable USD delta to the
// subscription's daily, weekly, and monthly materialized counters.
func ApplyUsageDelta(subscription contract.UserSubscription, delta contract.UsageDelta) contract.UserSubscription {
	updated := ResetExpiredUsage(subscription, delta.OccurredAt)
	cost := money.NormalizeAmount(delta.BillableCost)
	updated.DailyUsageUSD = money.AddMoney(updated.DailyUsageUSD, cost)
	updated.WeeklyUsageUSD = money.AddMoney(updated.WeeklyUsageUSD, cost)
	updated.MonthlyUsageUSD = money.AddMoney(updated.MonthlyUsageUSD, cost)
	return updated
}

// ResetExpiredUsage resets materialized cost windows whose stored start no
// longer matches the current UTC window.
func ResetExpiredUsage(subscription contract.UserSubscription, at time.Time) contract.UserSubscription {
	at = at.UTC()
	dayStart := usagewindow.StartOfDayUTC(at)
	weekStart := usagewindow.StartOfWeekUTC(at)
	monthStart := usagewindow.StartOfMonthUTC(at)
	if subscription.DailyWindowStart == nil || usagewindow.IsExpired(*subscription.DailyWindowStart, dayStart) {
		subscription.DailyUsageUSD = money.ZeroAmount
		subscription.DailyWindowStart = &dayStart
	}
	if subscription.WeeklyWindowStart == nil || usagewindow.IsExpired(*subscription.WeeklyWindowStart, weekStart) {
		subscription.WeeklyUsageUSD = money.ZeroAmount
		subscription.WeeklyWindowStart = &weekStart
	}
	if subscription.MonthlyWindowStart == nil || usagewindow.IsExpired(*subscription.MonthlyWindowStart, monthStart) {
		subscription.MonthlyUsageUSD = money.ZeroAmount
		subscription.MonthlyWindowStart = &monthStart
	}
	subscription.DailyUsageUSD = money.NormalizeAmount(subscription.DailyUsageUSD)
	subscription.WeeklyUsageUSD = money.NormalizeAmount(subscription.WeeklyUsageUSD)
	subscription.MonthlyUsageUSD = money.NormalizeAmount(subscription.MonthlyUsageUSD)
	return subscription
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
