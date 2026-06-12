package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
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
	if providerErrorBodyIndicatesQuotaExhausted(body, message) {
		class = "quota_exhausted"
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message, RetryAfter: providerRetryAfter(headers, body, now), Metadata: metadata, QuotaSignals: providerQuotaSignalsFromErrorHeaders(headers, now)}
}

func classifyAnthropicProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
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
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
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
	if providerErrorBodyIndicatesQuotaExhausted(body, message) {
		class = "quota_exhausted"
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message, RetryAfter: providerRetryAfter(headers, body, now), QuotaSignals: providerQuotaSignalsFromErrorHeaders(headers, now)}
}

func providerQuotaSignalsFromErrorHeaders(headers http.Header, now time.Time) []contract.QuotaSignal {
	return providerQuotaSignalsFromHeaders(headers, now)
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
		"resource has been exhausted",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
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
