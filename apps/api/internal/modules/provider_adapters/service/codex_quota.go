package service

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type codexQuotaHeaderWindow struct {
	usedPercent  float64
	resetSeconds *int
	windowMin    int
}

func codexQuotaSignalsFromHeaders(headers http.Header, now time.Time) []contract.QuotaSignal {
	if headers == nil {
		return nil
	}
	now = now.UTC()
	windows := []codexQuotaHeaderWindow{
		codexQuotaHeaderWindowFromHeaders(headers, "x-codex-primary"),
		codexQuotaHeaderWindowFromHeaders(headers, "x-codex-secondary"),
	}
	signals := make([]contract.QuotaSignal, 0, len(windows))
	for _, window := range windows {
		if !codexQuotaHeaderWindowValid(window) {
			continue
		}
		quotaType := codexQuotaTypeForWindow(window.windowMin)
		if quotaType == "" {
			continue
		}
		used := clampFloat(window.usedPercent, 0, 100)
		remaining := 100 - used
		var resetAt *time.Time
		if window.resetSeconds != nil {
			seconds := *window.resetSeconds
			if seconds < 0 {
				seconds = 0
			}
			value := now.Add(time.Duration(seconds) * time.Second)
			resetAt = &value
		}
		signals = append(signals, contract.QuotaSignal{
			QuotaType:      quotaType,
			Remaining:      formatPercentQuotaValue(remaining),
			Used:           formatPercentQuotaValue(used),
			QuotaLimit:     "100",
			RemainingRatio: float32(remaining / 100),
			ResetAt:        resetAt,
			SnapshotAt:     now,
		})
	}
	return signals
}

func codexQuotaHeaderWindowFromHeaders(headers http.Header, prefix string) codexQuotaHeaderWindow {
	window := codexQuotaHeaderWindow{usedPercent: math.NaN()}
	if value, ok := parseHeaderFloat(headers, prefix+"-used-percent"); ok {
		window.usedPercent = value
	}
	if value, ok := parseHeaderInt(headers, prefix+"-reset-after-seconds"); ok {
		window.resetSeconds = &value
	}
	if value, ok := parseHeaderInt(headers, prefix+"-window-minutes"); ok {
		window.windowMin = value
	}
	return window
}

func codexQuotaHeaderWindowValid(window codexQuotaHeaderWindow) bool {
	return !math.IsNaN(window.usedPercent)
}

func codexQuotaTypeForWindow(windowMin int) string {
	switch windowMin {
	case 0:
		return ""
	case 300:
		return "codex_5h_percent"
	case 10080:
		return "codex_7d_percent"
	default:
		return ""
	}
}

func parseHeaderFloat(headers http.Header, key string) (float64, bool) {
	raw := strings.TrimSpace(headers.Get(key))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func parseHeaderInt(headers http.Header, key string) (int, bool) {
	raw := strings.TrimSpace(headers.Get(key))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func formatPercentQuotaValue(value float64) string {
	return strconv.FormatFloat(value, 'f', percentQuotaPrecision(value), 64)
}

func percentQuotaPrecision(value float64) int {
	if math.Abs(value-math.Round(value)) < 0.000001 {
		return 0
	}
	return 6
}

func parseQuotaHeaderFields(value string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ','
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, raw, ok := strings.Cut(part, "=")
		if !ok {
			key, raw, ok = strings.Cut(part, ":")
		}
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		raw = strings.Trim(strings.TrimSpace(raw), `"`)
		if key != "" && raw != "" {
			fields[key] = raw
		}
	}
	return fields
}

func quotaFieldFloat(fields map[string]string, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw := strings.TrimSpace(fields[key])
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
			return value, true
		}
	}
	return 0, false
}

func normalizeQuotaPercent(value float64) float64 {
	if value > 1 {
		return value / 100
	}
	return value
}

func withCodexQuotaSignals(resp contract.ConversationResponse, headers http.Header) contract.ConversationResponse {
	resp.QuotaSignals = codexQuotaSignalsFromHeaders(headers, time.Now())
	return resp
}

func withCodexInputItemsQuotaSignals(resp contract.ResponseInputItemsResponse, headers http.Header) contract.ResponseInputItemsResponse {
	resp.QuotaSignals = codexQuotaSignalsFromHeaders(headers, time.Now())
	return resp
}
