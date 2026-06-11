package service

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func providerRetryAfter(headers http.Header, body []byte, now time.Time) *time.Time {
	if resetAt := retryAfterFromOpenAIErrorBody(body, now); resetAt != nil {
		return resetAt
	}
	if resetAt := retryAfterFromGeminiErrorBody(body, now); resetAt != nil {
		return resetAt
	}
	if resetAt := retryAfterFromHeader(headers, now); resetAt != nil {
		return resetAt
	}
	return codexRetryAfterFromHeaders(headers, now)
}

func retryAfterFromHeader(headers http.Header, now time.Time) *time.Time {
	if headers == nil {
		return nil
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		value := now.UTC().Add(time.Duration(seconds) * time.Second)
		return &value
	}
	if value, err := http.ParseTime(raw); err == nil {
		resetAt := value.UTC()
		return &resetAt
	}
	return nil
}

func retryAfterFromOpenAIErrorBody(body []byte, now time.Time) *time.Time {
	var decoded struct {
		Error struct {
			Type            string `json:"type"`
			Code            string `json:"code"`
			ResetsAt        any    `json:"resets_at"`
			ResetsInSeconds any    `json:"resets_in_seconds"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	errorType := strings.TrimSpace(decoded.Error.Type)
	errorCode := strings.TrimSpace(decoded.Error.Code)
	if errorType != "usage_limit_reached" && errorType != "rate_limit_exceeded" && errorCode != "usage_limit_reached" && errorCode != "rate_limit_exceeded" {
		return nil
	}
	if resetAt := retryAfterTimestampValue(decoded.Error.ResetsAt, now); resetAt != nil {
		return resetAt
	}
	if seconds, ok := positiveInt64(decoded.Error.ResetsInSeconds); ok {
		resetAt := now.UTC().Add(time.Duration(seconds) * time.Second)
		return &resetAt
	}
	return nil
}

func retryAfterFromGeminiErrorBody(body []byte, now time.Time) *time.Time {
	var decoded struct {
		Error struct {
			Details []map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	for _, detail := range decoded.Error.Details {
		if resetAt := retryAfterFromGoogleDuration(detail["retryDelay"], now); resetAt != nil {
			return resetAt
		}
		metadata, ok := detail["metadata"].(map[string]any)
		if !ok {
			continue
		}
		if resetAt := retryAfterFromGoogleDuration(metadata["quotaResetDelay"], now); resetAt != nil {
			return resetAt
		}
	}
	return nil
}

func retryAfterFromGoogleDuration(raw any, now time.Time) *time.Time {
	text := strings.TrimSpace(fmt.Sprint(raw))
	if text == "" || text == "<nil>" {
		return nil
	}
	if strings.HasSuffix(text, "s") {
		seconds, err := strconv.ParseFloat(strings.TrimSuffix(text, "s"), 64)
		if err == nil && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds >= 0 {
			value := now.UTC().Add(time.Duration(seconds * float64(time.Second)))
			return &value
		}
	}
	if duration, err := time.ParseDuration(text); err == nil && duration >= 0 {
		value := now.UTC().Add(duration)
		return &value
	}
	return nil
}

func retryAfterTimestampValue(value any, now time.Time) *time.Time {
	if unixSeconds, ok := positiveInt64(value); ok {
		resetAt := time.Unix(unixSeconds, 0).UTC()
		if resetAt.After(now.UTC()) {
			return &resetAt
		}
	}
	if text, ok := value.(string); ok {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			resetAt := parsed.UTC()
			if resetAt.After(now.UTC()) {
				return &resetAt
			}
		}
		if parsed, err := http.ParseTime(trimmed); err == nil {
			resetAt := parsed.UTC()
			if resetAt.After(now.UTC()) {
				return &resetAt
			}
		}
	}
	return nil
}

func codexRetryAfterFromHeaders(headers http.Header, now time.Time) *time.Time {
	signals := codexQuotaSignalsFromHeaders(headers, now)
	var resetAt *time.Time
	for _, signal := range signals {
		if signal.ResetAt == nil {
			continue
		}
		if resetAt == nil || signal.ResetAt.After(*resetAt) {
			value := *signal.ResetAt
			resetAt = &value
		}
	}
	return resetAt
}

func positiveInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if typed > 0 && !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return int64(typed), true
		}
	case int64:
		return typed, typed > 0
	case int:
		return int64(typed), typed > 0
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil && parsed > 0
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil && parsed > 0
	}
	return 0, false
}
