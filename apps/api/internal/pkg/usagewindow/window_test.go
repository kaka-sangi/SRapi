package usagewindow

import (
	"testing"
	"time"
)

func TestCalendarWindowStartsUseUTC(t *testing.T) {
	at := time.Date(2026, time.June, 9, 23, 45, 12, 34, time.FixedZone("UTC+8", 8*60*60))

	if got := StartOfDayUTC(at); !got.Equal(time.Date(2026, time.June, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day start = %s", got)
	}
	if got := StartOfWeekUTC(at); !got.Equal(time.Date(2026, time.June, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("week start = %s", got)
	}
	if got := StartOfMonthUTC(at); !got.Equal(time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("month start = %s", got)
	}
}

func TestStartOfWeekUTCUsesMondayAcrossSundayBoundary(t *testing.T) {
	sunday := time.Date(2026, time.June, 14, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, time.June, 8, 0, 0, 0, 0, time.UTC)
	if got := StartOfWeekUTC(sunday); !got.Equal(want) {
		t.Fatalf("week start = %s want %s", got, want)
	}
}

func TestRollingStartUTC(t *testing.T) {
	at := time.Date(2026, time.June, 9, 12, 30, 0, 0, time.UTC)
	cases := []struct {
		name   string
		window time.Duration
		want   time.Time
	}{
		{name: "five hours", window: FiveHours, want: time.Date(2026, time.June, 9, 7, 30, 0, 0, time.UTC)},
		{name: "one day", window: OneDay, want: time.Date(2026, time.June, 8, 12, 30, 0, 0, time.UTC)},
		{name: "seven days", window: SevenDays, want: time.Date(2026, time.June, 2, 12, 30, 0, 0, time.UTC)},
		{name: "disabled", window: 0, want: at},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RollingStartUTC(at, tc.window); !got.Equal(tc.want) {
				t.Fatalf("rolling start = %s want %s", got, tc.want)
			}
		})
	}
}

func TestIsExpired(t *testing.T) {
	expected := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	if !IsExpired(time.Time{}, expected) {
		t.Fatal("zero start should be expired")
	}
	if IsExpired(expected, expected) {
		t.Fatal("matching start should not be expired")
	}
	if !IsExpired(time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC), expected) {
		t.Fatal("different start should be expired")
	}
}

func TestRollingCounterExpired(t *testing.T) {
	start := time.Date(2026, time.June, 9, 12, 0, 0, 0, time.UTC)
	if !RollingCounterExpired(time.Time{}, start, FiveHours) {
		t.Fatal("zero rolling counter start should be expired")
	}
	if RollingCounterExpired(start, start.Add(4*time.Hour+59*time.Minute), FiveHours) {
		t.Fatal("counter should stay open before the rolling duration elapses")
	}
	if !RollingCounterExpired(start, start.Add(FiveHours), FiveHours) {
		t.Fatal("counter should expire once the rolling duration elapses")
	}
}
