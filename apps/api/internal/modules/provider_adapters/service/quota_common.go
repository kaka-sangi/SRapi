package service

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

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
