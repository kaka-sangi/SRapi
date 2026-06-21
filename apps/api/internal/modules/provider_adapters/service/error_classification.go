package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/httputil"
)

func classifyProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	return classifyProviderHTTPErrorWithHeaders(statusCode, nil, body)
}

func classifyProviderHTTPErrorWithHeaders(statusCode int, headers http.Header, body []byte) contract.ProviderError {
	now := time.Now()
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	class := providerClassForHTTPStatus(statusCode)
	metadata := map[string]any(nil)
	if statusCode == http.StatusForbidden {
		class, metadata = classifyForbiddenProviderError(body, message)
	}
	// A Cloudflare JS interstitial ("just a moment", cf-mitigated: challenge,
	// etc.) on a 403/429 is a transient anti-bot challenge, not a genuine auth
	// failure or rate limit. Classify it distinctly so failure handling does not
	// park the account on a cooldown: "cloudflare_challenge" is intentionally
	// absent from gatewayErrorClassUsesCooldown, which defaults to NO cooldown.
	if (statusCode == http.StatusForbidden || statusCode == http.StatusTooManyRequests) &&
		httputil.IsCloudflareChallengeResponse(statusCode, headers, body) {
		class = "cloudflare_challenge"
		if metadata == nil {
			metadata = map[string]any{}
		}
		if rayID := httputil.ExtractCloudflareRayID(headers, body); rayID != "" {
			metadata["cf_ray"] = rayID
		}
	}
	if statusCode == http.StatusUnauthorized && providerErrorBodyIndicatesSessionInvalid(body, message) {
		class = "session_invalid"
	}
	if providerErrorBodyIndicatesQuotaExhausted(body, message) {
		class = "quota_exhausted"
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message, Headers: cloneGenericHeaders(headers), RetryAfter: providerRetryAfter(headers, body, now), Metadata: metadata, QuotaSignals: providerQuotaSignalsFromHTTPError(headers, body, now)}
}

func classifyAnthropicProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	return classifyAnthropicProviderHTTPErrorWithHeaders(statusCode, nil, body)
}

func classifyAnthropicProviderHTTPErrorWithHeaders(statusCode int, headers http.Header, body []byte) contract.ProviderError {
	now := time.Now()
	var decoded struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	class := providerClassForHTTPStatus(statusCode)
	if err := json.Unmarshal(body, &decoded); err == nil {
		if decoded.Error.Message != "" {
			message = strings.TrimSpace(decoded.Error.Message)
		}
		if decoded.Error.Type != "" {
			class = providerClassForAnthropicError(decoded.Error.Type, statusCode)
		}
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message, Headers: cloneGenericHeaders(headers), RetryAfter: providerRetryAfter(headers, body, now), QuotaSignals: providerQuotaSignalsFromHTTPError(headers, body, now)}
}

func classifyGeminiProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	return classifyGeminiProviderHTTPErrorWithHeaders(statusCode, nil, body)
}

func classifyGeminiProviderHTTPErrorWithHeaders(statusCode int, headers http.Header, body []byte) contract.ProviderError {
	now := time.Now()
	var decoded struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	class := providerClassForHTTPStatus(statusCode)
	if err := json.Unmarshal(body, &decoded); err == nil {
		if decoded.Error.Message != "" {
			message = strings.TrimSpace(decoded.Error.Message)
		}
		if decoded.Error.Status != "" {
			class = providerClassForGeminiStatus(decoded.Error.Status, statusCode)
		}
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	if providerErrorBodyIndicatesGeminiQuotaExhausted(body, message) {
		class = "quota_exhausted"
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message, Headers: cloneGenericHeaders(headers), RetryAfter: providerRetryAfter(headers, body, now), QuotaSignals: providerQuotaSignalsFromHTTPError(headers, body, now)}
}

func providerQuotaSignalsFromErrorHeaders(headers http.Header, now time.Time) []contract.QuotaSignal {
	return providerQuotaSignalsFromHeaders(headers, now)
}

func providerQuotaSignalsFromHTTPError(headers http.Header, body []byte, now time.Time) []contract.QuotaSignal {
	signals := providerQuotaSignalsFromErrorHeaders(headers, now)
	signals = append(signals, googleModelRateLimitSignalsFromErrorBody(body, now)...)
	return signals
}

func providerQuotaSignalsFromHeaders(headers http.Header, now time.Time) []contract.QuotaSignal {
	if headers == nil {
		return nil
	}
	signals := codexQuotaSignalsFromHeaders(headers, now)
	signals = append(signals, anthropicQuotaSignalsFromHeaders(headers, now)...)
	return signals
}

func providerClassForGeminiStatus(status string, statusCode int) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "INVALID_ARGUMENT", "FAILED_PRECONDITION", "OUT_OF_RANGE":
		return "invalid_request"
	case "UNAUTHENTICATED", "PERMISSION_DENIED":
		return "auth_failed"
	case "RESOURCE_EXHAUSTED":
		return "rate_limit"
	case "NOT_FOUND":
		return "model_unavailable"
	case "DEADLINE_EXCEEDED":
		return "timeout"
	case "UNAVAILABLE", "INTERNAL", "UNKNOWN":
		return "provider_5xx"
	}
	return providerClassForHTTPStatus(statusCode)
}

func providerErrorBodyIndicatesQuotaExhausted(body []byte, message string) bool {
	text := strings.ToLower(strings.TrimSpace(message + " " + string(body)))
	if text == "" {
		return false
	}
	for _, keyword := range []string{
		"quota_exhausted",
		"quota exhausted",
		"insufficient_quota",
		"insufficient quota",
		"google_one_ai",
		"insufficient credit",
		"insufficient credits",
		"not enough credit",
		"not enough credits",
		"credit exhausted",
		"credits exhausted",
		"credit balance",
		"minimumcreditamountforusage",
		"minimum credit amount for usage",
		"minimum credit",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// providerErrorBodyIndicatesSessionInvalid reports whether a 401 response body
// carries an OAuth revocation marker (a permanently rejected refresh token),
// rather than a transient access-token failure. It mirrors the marker detection
// in reverse_proxy classifyOAuthRefreshError so the resulting "session_invalid"
// class maps (via reverseProxyAccountFailureStatus) to NeedsReauth, stopping the
// scheduler from re-selecting accounts whose refresh token was revoked.
func providerErrorBodyIndicatesSessionInvalid(body []byte, message string) bool {
	text := strings.ToLower(strings.TrimSpace(message + " " + string(body)))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"invalid_grant",
		"refresh_token_reused",
		"invalid refresh",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func providerErrorBodyIndicatesGeminiQuotaExhausted(body []byte, message string) bool {
	// Antigravity surfaces credit-balance exhaustion as
	// `google.rpc.ErrorInfo.reason = INSUFFICIENT_G1_CREDITS_BALANCE`, with
	// no retryDelay and no quota_exhausted keyword in the human message.
	// Treat that as a hard quota_exhausted explicitly so the cooldown
	// promotes from the generic rate-limit default to the quota window
	// the rate-limit module reserves for true exhaustion. Matches the
	// CLIProxyAPI antigravity executor's `antigravityHasExplicit
	// CreditsBalanceExhaustedReason` gate.
	if providerErrorBodyHasGoogleReason(body, "INSUFFICIENT_G1_CREDITS_BALANCE", "QUOTA_EXHAUSTED") {
		return true
	}
	if providerErrorBodyHasGoogleReason(body, "RATE_LIMIT_EXCEEDED", "MODEL_CAPACITY_EXHAUSTED") {
		return false
	}
	return providerErrorBodyIndicatesQuotaExhausted(body, message)
}

func providerErrorBodyHasGoogleReason(body []byte, reasons ...string) bool {
	if len(body) == 0 || len(reasons) == 0 {
		return false
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return false
	}
	wanted := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		normalized := strings.ToUpper(strings.TrimSpace(reason))
		if normalized != "" {
			wanted[normalized] = struct{}{}
		}
	}
	return googleReasonMatches(decoded, wanted)
}

func googleReasonMatches(value any, wanted map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]any:
		if raw, ok := typed["reason"]; ok {
			if _, ok := wanted[strings.ToUpper(strings.TrimSpace(fmt.Sprint(raw)))]; ok {
				return true
			}
		}
		for _, raw := range typed {
			if googleReasonMatches(raw, wanted) {
				return true
			}
		}
	case []any:
		for _, raw := range typed {
			if googleReasonMatches(raw, wanted) {
				return true
			}
		}
	}
	return false
}

func googleModelRateLimitSignalsFromErrorBody(body []byte, now time.Time) []contract.QuotaSignal {
	var decoded struct {
		Error struct {
			Details []map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Error.Details) == 0 {
		return nil
	}
	resetAt := googleRetryResetAtFromDetails(decoded.Error.Details, now)
	signals := make([]contract.QuotaSignal, 0, 1)
	for _, detail := range decoded.Error.Details {
		if !googleDetailReasonIs(detail, "RATE_LIMIT_EXCEEDED") {
			continue
		}
		metadata, _ := detail["metadata"].(map[string]any)
		model := googleQuotaModelName(metadata)
		quotaType := googleModelRateLimitQuotaType(model)
		if quotaType == "" {
			continue
		}
		signals = append(signals, contract.QuotaSignal{
			QuotaType:      quotaType,
			Remaining:      "0",
			Used:           "100",
			QuotaLimit:     "100",
			RemainingRatio: 0,
			ResetAt:        resetAt,
			SnapshotAt:     now.UTC(),
		})
	}
	return signals
}

func googleRetryResetAtFromDetails(details []map[string]any, now time.Time) *time.Time {
	for _, detail := range details {
		if resetAt := googleRetryResetAtFromDetail(detail, now); resetAt != nil {
			return resetAt
		}
	}
	return nil
}

func googleRetryResetAtFromDetail(detail map[string]any, now time.Time) *time.Time {
	if resetAt := retryAfterFromGoogleDuration(detail["retryDelay"], now); resetAt != nil {
		return resetAt
	}
	metadata, ok := detail["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	if resetAt := retryAfterFromGoogleDuration(metadata["quotaResetDelay"], now); resetAt != nil {
		return resetAt
	}
	return retryAfterTimestampValue(metadata["quotaResetTimeStamp"], now)
}

func googleDetailReasonIs(detail map[string]any, reason string) bool {
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(detail["reason"])), reason)
}

func googleQuotaModelName(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range []string{"model", "modelName", "model_name"} {
		value := strings.TrimSpace(fmt.Sprint(metadata[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func googleModelRateLimitQuotaType(model string) string {
	normalized := normalizeGoogleQuotaTypeSegment(model)
	if normalized == "" {
		return ""
	}
	return "google_model_rate_limit_" + normalized
}

func normalizeGoogleQuotaTypeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	normalized := strings.Trim(builder.String(), "_")
	if len(normalized) > 96 {
		normalized = strings.TrimRight(normalized[:96], "_")
	}
	return normalized
}

func providerClassForAnthropicError(errorType string, statusCode int) string {
	switch strings.ToLower(strings.TrimSpace(errorType)) {
	case "authentication_error", "permission_error":
		return "auth_failed"
	case "rate_limit_error":
		return "rate_limit"
	case "invalid_request_error":
		return "invalid_request"
	case "not_found_error":
		return "model_unavailable"
	case "overloaded_error", "api_error":
		return "provider_5xx"
	}
	return providerClassForHTTPStatus(statusCode)
}

func providerClassForHTTPStatus(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "auth_failed"
	case http.StatusNotFound:
		return "model_unavailable"
	case http.StatusTooManyRequests:
		return "rate_limit"
	case 529:
		return "overloaded"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "timeout"
	default:
		if statusCode >= 500 {
			return "provider_5xx"
		}
	}
	return "unknown"
}

var validationURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

func classifyForbiddenProviderError(body []byte, fallbackMessage string) (string, map[string]any) {
	message := strings.TrimSpace(fallbackMessage)
	class := "forbidden"
	metadata := map[string]any{}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err == nil {
		if value := findStringField(decoded, "message", "error_description", "detail", "reason"); value != "" {
			message = value
		}
		if code := findStringField(decoded, "type", "code", "error", "status"); code != "" {
			class = forbiddenClassFromText(code, class)
		}
		if validationURL := findStringField(decoded, "validation_url", "verification_url", "url"); validationURL != "" {
			metadata["validation_url"] = validationURL
		}
	}
	class = forbiddenClassFromText(message, class)
	if _, ok := metadata["validation_url"]; !ok {
		if validationURL := validationURLPattern.FindString(string(body)); validationURL != "" {
			metadata["validation_url"] = strings.TrimRight(validationURL, ".,;)")
		}
	}
	if message != "" {
		metadata["provider_message"] = message
	}
	metadata["provider_error_kind"] = class
	return class, metadata
}

func forbiddenClassFromText(value string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "validation"), strings.Contains(normalized, "verify"), strings.Contains(normalized, "verification"):
		return "validation_required"
	case strings.Contains(normalized, "violation"), strings.Contains(normalized, "suspended"), strings.Contains(normalized, "blocked"), strings.Contains(normalized, "banned"):
		return "policy_violation"
	case strings.Contains(normalized, "permission"), strings.Contains(normalized, "forbidden"):
		return "forbidden"
	default:
		return fallback
	}
}

func findStringField(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, ok := typed[key]; ok {
				if text := stringFieldValue(raw); text != "" {
					return text
				}
			}
		}
		for _, raw := range typed {
			if found := findStringField(raw, keys...); found != "" {
				return found
			}
		}
	case []any:
		for _, raw := range typed {
			if found := findStringField(raw, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func stringFieldValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func providerErrorFromReverseProxy(err error) error {
	var runtimeErr reverseproxycontract.RuntimeError
	if errors.As(err, &runtimeErr) {
		statusCode := runtimeErr.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		class := strings.TrimSpace(runtimeErr.Class)
		if class == "" {
			class = "unknown"
		}
		if class == "upstream_error" {
			class = providerClassForHTTPStatus(statusCode)
		}
		message := strings.TrimSpace(runtimeErr.Message)
		if message == "" {
			message = runtimeErr.Error()
		}
		return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
	}
	return contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy request failed"}
}
