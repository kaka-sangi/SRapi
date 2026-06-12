package service

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func anthropicQuotaSignalsFromHeaders(headers http.Header, now time.Time) []contract.QuotaSignal {
	if headers == nil {
		return nil
	}
	windows := []struct {
		quotaType string
		header    string
	}{
		{quotaType: "anthropic_5h", header: "anthropic-ratelimit-unified-5h"},
		{quotaType: "anthropic_7d", header: "anthropic-ratelimit-unified-7d"},
	}
	signals := make([]contract.QuotaSignal, 0, len(windows))
	for _, window := range windows {
		signal, ok := anthropicQuotaSignalFromHeader(window.quotaType, headers.Values(window.header), now)
		if ok {
			signals = append(signals, signal)
			continue
		}
		signal, ok = anthropicQuotaSignalFromSplitHeaders(window.quotaType, window.header, headers, now)
		if ok {
			signals = append(signals, signal)
		}
	}
	return signals
}

func anthropicQuotaSignalFromHeader(quotaType string, values []string, now time.Time) (contract.QuotaSignal, bool) {
	for _, value := range values {
		fields := parseQuotaHeaderFields(value)
		if len(fields) == 0 {
			continue
		}
		if signal, ok := anthropicQuotaSignalFromFields(quotaType, fields, now); ok {
			return signal, true
		}
	}
	return contract.QuotaSignal{}, false
}

func anthropicQuotaSignalFromSplitHeaders(quotaType string, prefix string, headers http.Header, now time.Time) (contract.QuotaSignal, bool) {
	usedRatio, ok := anthropicUsedRatioFromSplitHeaders(headers, prefix)
	if !ok {
		return contract.QuotaSignal{}, false
	}
	usedRatio = clampFloat(usedRatio, 0, 1)
	remainingRatio := 1 - usedRatio
	return contract.QuotaSignal{
		QuotaType:      quotaType,
		Remaining:      formatPercentQuotaValue(remainingRatio * 100),
		Used:           formatPercentQuotaValue(usedRatio * 100),
		QuotaLimit:     "100",
		RemainingRatio: float32(remainingRatio),
		ResetAt:        anthropicSplitResetAtFromHeaders(headers, prefix),
		SnapshotAt:     now.UTC(),
	}, true
}

func anthropicUsedRatioFromSplitHeaders(headers http.Header, prefix string) (float64, bool) {
	if value, ok := parseHeaderFloat(headers, prefix+"-utilization"); ok {
		return value, true
	}
	if anthropicSplitSurpassedThreshold(headers, prefix) {
		return 1, true
	}
	return 0, false
}

func anthropicQuotaSignalFromFields(quotaType string, fields map[string]string, now time.Time) (contract.QuotaSignal, bool) {
	remainingRatio, ok := anthropicRemainingRatioFromFields(fields)
	if !ok {
		return contract.QuotaSignal{}, false
	}
	remainingRatio = clampFloat(remainingRatio, 0, 1)
	usedRatio := 1 - remainingRatio
	resetAt := anthropicResetAtFromFields(fields, now)
	return contract.QuotaSignal{
		QuotaType:      quotaType,
		Remaining:      formatPercentQuotaValue(remainingRatio * 100),
		Used:           formatPercentQuotaValue(usedRatio * 100),
		QuotaLimit:     "100",
		RemainingRatio: float32(remainingRatio),
		ResetAt:        resetAt,
		SnapshotAt:     now.UTC(),
	}, true
}

func anthropicRemainingRatioFromFields(fields map[string]string) (float64, bool) {
	if remaining, remainingOK := quotaFieldFloat(fields, "remaining", "remaining_tokens", "remaining_requests"); remainingOK {
		if limit, limitOK := quotaFieldFloat(fields, "limit", "quota", "total", "max"); limitOK && limit > 0 {
			return remaining / limit, true
		}
	}
	if used, usedOK := quotaFieldFloat(fields, "used", "usage", "consumed"); usedOK {
		if limit, limitOK := quotaFieldFloat(fields, "limit", "quota", "total", "max"); limitOK && limit > 0 {
			return 1 - used/limit, true
		}
	}
	if percent, ok := quotaFieldFloat(fields, "remaining_percent", "remaining_pct"); ok {
		return normalizeQuotaPercent(percent), true
	}
	if percent, ok := quotaFieldFloat(fields, "used_percent", "used_pct", "usage_percent", "usage_pct"); ok {
		return 1 - normalizeQuotaPercent(percent), true
	}
	return 0, false
}

func anthropicResetAtFromFields(fields map[string]string, now time.Time) *time.Time {
	for _, key := range []string{"reset_after_seconds", "reset_after", "retry_after_seconds"} {
		if seconds, ok := quotaFieldFloat(fields, key); ok && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) {
			if seconds < 0 {
				seconds = 0
			}
			value := now.UTC().Add(time.Duration(seconds * float64(time.Second)))
			return &value
		}
	}
	for _, key := range []string{"resets_at", "reset_at"} {
		text := strings.TrimSpace(fields[key])
		if text == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, text); err == nil {
			value := parsed.UTC()
			return &value
		}
	}
	return nil
}

func anthropicRetryAfterFromHeaders(headers http.Header, now time.Time) *time.Time {
	if headers == nil {
		return nil
	}
	now = now.UTC()
	reset5h := anthropicFutureResetAt(anthropicSplitResetAtFromHeaders(headers, "anthropic-ratelimit-unified-5h"), now)
	reset7d := anthropicFutureResetAt(anthropicSplitResetAtFromHeaders(headers, "anthropic-ratelimit-unified-7d"), now)
	if reset5h == nil && reset7d == nil {
		return nil
	}

	exceeded5h := anthropicSplitWindowExceeded(headers, "anthropic-ratelimit-unified-5h")
	exceeded7d := anthropicSplitWindowExceeded(headers, "anthropic-ratelimit-unified-7d")
	switch {
	case exceeded5h && exceeded7d:
		if reset7d != nil {
			return reset7d
		}
		return reset5h
	case exceeded5h:
		return reset5h
	case exceeded7d:
		return reset7d
	default:
		return earlierTime(reset5h, reset7d)
	}
}

func anthropicFutureResetAt(resetAt *time.Time, now time.Time) *time.Time {
	if resetAt == nil || !resetAt.After(now) {
		return nil
	}
	value := resetAt.UTC()
	return &value
}

func anthropicSplitResetAtFromHeaders(headers http.Header, prefix string) *time.Time {
	raw := strings.TrimSpace(headers.Get(prefix + "-reset"))
	if raw == "" {
		return nil
	}
	unixSeconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || unixSeconds <= 0 {
		return nil
	}
	value := time.Unix(unixSeconds, 0).UTC()
	return &value
}

func anthropicSplitWindowExceeded(headers http.Header, prefix string) bool {
	if anthropicSplitSurpassedThreshold(headers, prefix) {
		return true
	}
	utilization, ok := parseHeaderFloat(headers, prefix+"-utilization")
	return ok && utilization >= 1-1e-9
}

func anthropicSplitSurpassedThreshold(headers http.Header, prefix string) bool {
	return strings.EqualFold(strings.TrimSpace(headers.Get(prefix+"-surpassed-threshold")), "true")
}

func earlierTime(left *time.Time, right *time.Time) *time.Time {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	case left.Before(*right):
		return left
	default:
		return right
	}
}

func withAnthropicQuotaSignals(resp contract.ConversationResponse, headers http.Header) contract.ConversationResponse {
	resp.QuotaSignals = append(resp.QuotaSignals, anthropicQuotaSignalsFromHeaders(headers, time.Now())...)
	return resp
}

func anthropicStreamParserWithHeaders(headers http.Header) func([]byte, int) (contract.ConversationResponse, error) {
	return func(body []byte, statusCode int) (contract.ConversationResponse, error) {
		resp, err := parseAnthropicCompatibleStream(body, statusCode)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		return withAnthropicQuotaSignals(resp, headers), nil
	}
}

func anthropicStreamConversationResponse(resp reverseproxycontract.StreamResponse) contract.ConversationResponse {
	return contract.ConversationResponse{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Headers,
		StreamBody:  resp.Body,
		StreamParse: anthropicStreamParserWithHeaders(resp.Headers),
	}
}
