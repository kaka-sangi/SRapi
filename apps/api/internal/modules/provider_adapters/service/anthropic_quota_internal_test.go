package service

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestAnthropicRetryAfterFromSplitHeaders(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(5 * time.Minute)
	sevenDayReset := now.Add(30 * time.Minute)

	tests := []struct {
		name string
		h    http.Header
		want *time.Time
	}{
		{
			name: "only 5h exceeded",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "1.02",
				"5h-reset":       strconv.FormatInt(fiveHourReset.Unix(), 10),
				"7d-utilization": "0.32",
				"7d-reset":       strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &fiveHourReset,
		},
		{
			name: "only 7d exceeded",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "0.50",
				"5h-reset":       strconv.FormatInt(fiveHourReset.Unix(), 10),
				"7d-utilization": "1.05",
				"7d-reset":       strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &sevenDayReset,
		},
		{
			name: "both exceeded chooses longer 7d reset",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "1.10",
				"5h-reset":       strconv.FormatInt(fiveHourReset.Unix(), 10),
				"7d-utilization": "1.02",
				"7d-reset":       strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &sevenDayReset,
		},
		{
			name: "surpassed threshold counts as exceeded",
			h: anthropicSplitHeaders(map[string]string{
				"5h-surpassed-threshold": "true",
				"5h-reset":               strconv.FormatInt(fiveHourReset.Unix(), 10),
				"7d-surpassed-threshold": "false",
				"7d-reset":               strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &fiveHourReset,
		},
		{
			name: "utilization exactly one counts as exceeded",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "1.0",
				"5h-reset":       strconv.FormatInt(fiveHourReset.Unix(), 10),
				"7d-utilization": "0.5",
				"7d-reset":       strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &fiveHourReset,
		},
		{
			name: "neither exceeded uses earlier reset",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "0.95",
				"5h-reset":       strconv.FormatInt(fiveHourReset.Unix(), 10),
				"7d-utilization": "0.80",
				"7d-reset":       strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &fiveHourReset,
		},
		{
			name: "only 5h reset",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "1.05",
				"5h-reset":       strconv.FormatInt(fiveHourReset.Unix(), 10),
			}),
			want: &fiveHourReset,
		},
		{
			name: "only 7d reset",
			h: anthropicSplitHeaders(map[string]string{
				"7d-utilization": "1.03",
				"7d-reset":       strconv.FormatInt(sevenDayReset.Unix(), 10),
			}),
			want: &sevenDayReset,
		},
		{
			name: "aggregated reset alone is ignored",
			h: http.Header{
				"anthropic-ratelimit-unified-reset": []string{strconv.FormatInt(sevenDayReset.Unix(), 10)},
			},
			want: nil,
		},
		{
			name: "past reset is ignored",
			h: anthropicSplitHeaders(map[string]string{
				"5h-utilization": "1.05",
				"5h-reset":       strconv.FormatInt(now.Add(-time.Minute).Unix(), 10),
			}),
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropicRetryAfterFromHeaders(tc.h, now)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("expected nil reset, got %s", got.Format(time.RFC3339))
				}
				return
			}
			if got == nil || !got.Equal(*tc.want) {
				t.Fatalf("expected reset %s, got %v", tc.want.Format(time.RFC3339), got)
			}
		})
	}
}

func TestAnthropicQuotaSignalsFromSplitHeaders(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	resetAt := now.Add(10 * time.Minute)
	headers := anthropicSplitHeaders(map[string]string{
		"5h-utilization": "1.02",
		"5h-reset":       strconv.FormatInt(resetAt.Unix(), 10),
		"7d-utilization": "0.32",
		"7d-reset":       strconv.FormatInt(now.Add(30*time.Minute).Unix(), 10),
	})

	signals := anthropicQuotaSignalsFromHeaders(headers, now)
	if len(signals) != 2 {
		t.Fatalf("expected two Anthropic quota signals, got %+v", signals)
	}
	assertAnthropicQuotaSignal(t, signals, "anthropic_5h", "100", "0", 0, resetAt)
	assertAnthropicQuotaSignal(t, signals, "anthropic_7d", "32", "68", 0.68, now.Add(30*time.Minute))
}

func anthropicSplitHeaders(values map[string]string) http.Header {
	headers := http.Header{}
	for suffix, value := range values {
		headers.Set("anthropic-ratelimit-unified-"+suffix, value)
	}
	return headers
}

func assertAnthropicQuotaSignal(t *testing.T, signals []contract.QuotaSignal, quotaType string, used string, remaining string, remainingRatio float32, resetAt time.Time) {
	t.Helper()
	for _, signal := range signals {
		if signal.QuotaType != quotaType {
			continue
		}
		if signal.Used != used || signal.Remaining != remaining || signal.QuotaLimit != "100" || signal.RemainingRatio != remainingRatio || signal.ResetAt == nil || !signal.ResetAt.Equal(resetAt) {
			t.Fatalf("unexpected quota signal for %s: %+v", quotaType, signal)
		}
		return
	}
	t.Fatalf("missing quota signal %q in %+v", quotaType, signals)
}
