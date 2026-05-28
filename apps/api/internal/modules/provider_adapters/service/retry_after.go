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
	if resetAt := retryAfterFromOpenAIErrorBody(body); resetAt != nil {
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

func retryAfterFromOpenAIErrorBody(body []byte) *time.Time {
	var decoded struct {
		Error struct {
			ResetsAt int64 `json:"resets_at"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || decoded.Error.ResetsAt <= 0 {
		return nil
	}
	resetAt := time.Unix(decoded.Error.ResetsAt, 0).UTC()
	return &resetAt
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
