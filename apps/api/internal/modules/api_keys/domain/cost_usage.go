package domain

import (
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	"github.com/srapi/srapi/apps/api/internal/pkg/usagewindow"
)

// ApplyCostUsage resets expired cost windows and appends billable USD cost to
// an API key's lifetime and rolling spend counters.
func ApplyCostUsage(key contract.APIKey, input contract.CostUsageUpdate) contract.APIKey {
	at := input.OccurredAt.UTC()
	updated := ResetExpiredCostWindows(key, at)
	cost := money.NormalizeAmount(input.BillableCost)
	updated.CostUsed = money.AddMoney(updated.CostUsed, cost)
	updated.CostUsed5h = money.AddMoney(updated.CostUsed5h, cost)
	updated.CostUsed1d = money.AddMoney(updated.CostUsed1d, cost)
	updated.CostUsed7d = money.AddMoney(updated.CostUsed7d, cost)
	return updated
}

// ResetExpiredCostWindows resets rolling cost windows whose stored starts have
// fallen behind the current trailing-window start.
func ResetExpiredCostWindows(key contract.APIKey, at time.Time) contract.APIKey {
	at = at.UTC()
	if key.CostWindowStart5h == nil || usagewindow.RollingCounterExpired(*key.CostWindowStart5h, at, usagewindow.FiveHours) {
		key.CostUsed5h = money.ZeroAmount
		key.CostWindowStart5h = &at
	}
	if key.CostWindowStart1d == nil || usagewindow.RollingCounterExpired(*key.CostWindowStart1d, at, usagewindow.OneDay) {
		key.CostUsed1d = money.ZeroAmount
		key.CostWindowStart1d = &at
	}
	if key.CostWindowStart7d == nil || usagewindow.RollingCounterExpired(*key.CostWindowStart7d, at, usagewindow.SevenDays) {
		key.CostUsed7d = money.ZeroAmount
		key.CostWindowStart7d = &at
	}
	key.CostUsed = money.NormalizeAmount(key.CostUsed)
	key.CostUsed5h = money.NormalizeAmount(key.CostUsed5h)
	key.CostUsed1d = money.NormalizeAmount(key.CostUsed1d)
	key.CostUsed7d = money.NormalizeAmount(key.CostUsed7d)
	return key
}
