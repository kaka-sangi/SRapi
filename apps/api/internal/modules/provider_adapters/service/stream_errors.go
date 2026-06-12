package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

type streamErrorPayload struct {
	Type    string             `json:"type"`
	Message string             `json:"message"`
	Code    any                `json:"code"`
	Status  string             `json:"status"`
	Error   *streamErrorObject `json:"error"`
}

type streamErrorObject struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    any    `json:"code"`
	Status  string `json:"status"`
}

func providerErrorFromStreamFrame(frame sseFrame, data string, protocol string) (contract.ProviderError, bool) {
	if strings.TrimSpace(data) == "" || data == "[DONE]" {
		return contract.ProviderError{}, false
	}
	var payload streamErrorPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		if strings.EqualFold(strings.TrimSpace(frame.Event), "error") {
			return streamProviderError(protocol, "", "", strings.TrimSpace(data), http.StatusBadGateway), true
		}
		return contract.ProviderError{}, false
	}
	if payload.Error != nil {
		statusCode := streamErrorStatusCode(payload.Error.Code)
		errorType := firstNonEmpty(payload.Error.Type, payload.Type)
		status := firstNonEmpty(payload.Error.Status, payload.Status)
		code := streamErrorCode(payload.Error.Code)
		message := firstPresent(payload.Error.Message, payload.Message, code, errorType, status)
		providerErr := streamProviderError(protocol, firstNonEmpty(errorType, code), status, message, statusCode)
		providerErr.RetryAfter = providerRetryAfter(nil, []byte(data), time.Now())
		return providerErr, true
	}
	if !strings.EqualFold(strings.TrimSpace(frame.Event), "error") && !strings.EqualFold(strings.TrimSpace(payload.Type), "error") {
		return contract.ProviderError{}, false
	}
	statusCode := streamErrorStatusCode(payload.Code)
	code := streamErrorCode(payload.Code)
	message := firstPresent(payload.Message, code, payload.Type, "provider stream returned error event")
	providerErr := streamProviderError(protocol, firstNonEmpty(payload.Type, code), payload.Status, message, statusCode)
	providerErr.RetryAfter = providerRetryAfter(nil, []byte(data), time.Now())
	return providerErr, true
}

func streamProviderError(protocol string, errorType string, status string, message string, statusCode int) contract.ProviderError {
	if statusCode <= 0 {
		statusCode = http.StatusBadGateway
	}
	class := streamProviderErrorClass(protocol, errorType, status, statusCode)
	if streamProviderErrorIndicatesQuotaExhausted(protocol, message+" "+errorType+" "+status) {
		class = "quota_exhausted"
	}
	if statusCode == http.StatusBadGateway {
		statusCode = providerStatusCodeForClass(class)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "provider stream returned error event"
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
}

func streamProviderErrorIndicatesQuotaExhausted(protocol string, message string) bool {
	if strings.TrimSpace(protocol) == "gemini-compatible" {
		return providerErrorBodyIndicatesGeminiQuotaExhausted(nil, message)
	}
	return providerErrorBodyIndicatesQuotaExhausted(nil, message)
}

func streamProviderErrorClass(protocol string, errorType string, status string, statusCode int) string {
	switch strings.TrimSpace(protocol) {
	case "anthropic-compatible":
		if errorType != "" {
			return providerClassForAnthropicError(errorType, statusCode)
		}
	case "gemini-compatible":
		if status != "" {
			return providerClassForGeminiStatus(status, statusCode)
		}
	}
	return providerClassForOpenAIStreamError(errorType, statusCode)
}

func providerClassForOpenAIStreamError(errorType string, statusCode int) string {
	value := strings.ToLower(strings.TrimSpace(errorType))
	switch {
	case value == "authentication_error" || value == "permission_error":
		return "auth_failed"
	case value == "rate_limit_error" || value == "rate_limit_exceeded" || value == "insufficient_quota":
		return "rate_limit"
	case value == "invalid_request_error" || value == "invalid_request" || strings.Contains(value, "context"):
		return "invalid_request"
	case value == "not_found_error" || value == "model_not_found":
		return "model_unavailable"
	case value == "timeout" || value == "deadline_exceeded":
		return "timeout"
	case value == "server_error" || value == "api_error" || value == "internal_error" || value == "service_unavailable":
		return "provider_5xx"
	}
	return providerClassForHTTPStatus(statusCode)
}

func providerStatusCodeForClass(class string) int {
	switch class {
	case "invalid_request":
		return http.StatusBadRequest
	case "auth_failed":
		return http.StatusUnauthorized
	case "model_unavailable":
		return http.StatusNotFound
	case "rate_limit":
		return http.StatusTooManyRequests
	case "timeout":
		return http.StatusGatewayTimeout
	case "overloaded":
		return 529
	default:
		return http.StatusBadGateway
	}
}

func streamErrorStatusCode(value any) int {
	switch typed := value.(type) {
	case float64:
		code := int(typed)
		if code >= 400 && code <= 599 {
			return code
		}
	case string:
		code, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil && code >= 400 && code <= 599 {
			return code
		}
	case json.Number:
		code, err := typed.Int64()
		if err == nil && code >= 400 && code <= 599 {
			return int(code)
		}
	case int:
		if typed >= 400 && typed <= 599 {
			return typed
		}
	}
	return 0
}

func streamErrorCode(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func firstPresent(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
