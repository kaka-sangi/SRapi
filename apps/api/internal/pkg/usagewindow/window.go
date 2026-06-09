package usagewindow

import "time"

const (
	FiveHours = 5 * time.Hour
	OneDay    = 24 * time.Hour
	SevenDays = 7 * 24 * time.Hour
)

// StartOfDayUTC returns the UTC calendar-day start containing at.
func StartOfDayUTC(at time.Time) time.Time {
	utc := at.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

// StartOfWeekUTC returns the UTC ISO-week start (Monday 00:00) containing at.
func StartOfWeekUTC(at time.Time) time.Time {
	day := StartOfDayUTC(at)
	offset := int(day.Weekday() - time.Monday)
	if offset < 0 {
		offset += 7
	}
	return day.AddDate(0, 0, -offset)
}

// StartOfMonthUTC returns the UTC calendar-month start containing at.
func StartOfMonthUTC(at time.Time) time.Time {
	utc := at.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// RollingStartUTC returns the trailing rolling-window start for at.
func RollingStartUTC(at time.Time, window time.Duration) time.Time {
	utc := at.UTC()
	if window <= 0 {
		return utc
	}
	return utc.Add(-window)
}

// RollingCounterExpired reports whether a materialized rolling-window counter
// should reset. Unlike RollingStartUTC, materialized counters keep their own
// start and reset only after the configured duration has elapsed.
func RollingCounterExpired(storedStart time.Time, at time.Time, window time.Duration) bool {
	if storedStart.IsZero() {
		return true
	}
	if window <= 0 {
		return false
	}
	return !at.UTC().Before(storedStart.UTC().Add(window))
}

// IsExpired reports whether storedStart no longer matches the expected
// materialized-window start. Zero starts are always expired so callers can
// lazily initialize newly-created or migrated rows.
func IsExpired(storedStart time.Time, expectedStart time.Time) bool {
	if storedStart.IsZero() {
		return true
	}
	return !storedStart.UTC().Equal(expectedStart.UTC())
}
