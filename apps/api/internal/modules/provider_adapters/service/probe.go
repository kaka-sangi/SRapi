package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const maxProbeResponseBytes = 512 << 10

// ProbeAccount performs a lightweight upstream health check for one provider account.
func (s *Service) ProbeAccount(ctx context.Context, req contract.ProbeRequest) (contract.ProbeResponse, error) {
	if req.Account.ID <= 0 || req.Provider.ID <= 0 {
		return contract.ProbeResponse{}, ErrInvalidInput
	}
	startedAt := time.Now().UTC()
	probeReq, err := buildProbeHTTPRequest(req)
	if err != nil {
		return probeFailure(startedAt, "invalid_request", http.StatusBadRequest), nil
	}
	if probeReq.Endpoint == "" {
		return probeFailure(startedAt, "invalid_request", http.StatusBadRequest), nil
	}
	headers, err := probeHeaders(req, &probeReq.Endpoint)
	if err != nil {
		return probeFailure(startedAt, "auth_failed", http.StatusUnauthorized), nil
	}
	mergeProbeHeaders(headers, probeReq.Headers)
	resp, err := s.doProbe(ctx, req.Account, probeReq, headers)
	if err != nil {
		return probeResponseFromError(startedAt, err), nil
	}
	if !probeStatusAllowed(resp.StatusCode, probeReq.ExpectedStatusCodes) {
		providerErr := classifyProviderHTTPError(resp.StatusCode, resp.Body)
		return probeResponseFromProviderError(startedAt, providerErr), nil
	}
	if !probeBodyExpectationMet(resp.Body, probeReq) {
		return probeFailure(startedAt, "invalid_response", http.StatusBadGateway), nil
	}
	return contract.ProbeResponse{
		OK:         true,
		StatusCode: resp.StatusCode,
		LatencyMS:  latencySince(startedAt),
		Metadata: map[string]any{
			"endpoint": probeReq.Endpoint,
			"method":   probeReq.Method,
		},
	}, nil
}

func (s *Service) doProbe(ctx context.Context, account accountcontract.ProviderAccount, req probeHTTPRequest, headers http.Header) (probeHTTPResponse, error) {
	if account.RuntimeClass != accountcontract.RuntimeClassAPIKey {
		return probeHTTPResponse{}, contract.ProviderError{Class: "unsupported_runtime", StatusCode: http.StatusBadRequest, Message: "account runtime cannot be probed without reverse proxy runtime"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.Endpoint, bytes.NewReader(req.Body))
	if err != nil {
		return probeHTTPResponse{}, err
	}
	httpReq.Header = headers
	if len(req.Body) > 0 && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.egressHTTPClient(account, nil).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return probeHTTPResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider probe timed out"}
		}
		return probeHTTPResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider probe failed"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeResponseBytes))
	if err != nil {
		return probeHTTPResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider probe response read failed"}
	}
	return probeHTTPResponse{StatusCode: resp.StatusCode, Body: body}, nil
}

type probeHTTPResponse struct {
	StatusCode int
	Body       []byte
}

type probeHTTPRequest struct {
	Endpoint            string
	Method              string
	Headers             map[string]string
	Body                []byte
	ExpectedStatusCodes []int
	ResponsePath        string
	ResponseContains    string
}

func buildProbeHTTPRequest(req contract.ProbeRequest) (probeHTTPRequest, error) {
	values := probeConfigMaps(req)
	method := strings.ToUpper(firstMapString(values, "health_probe_method", "probe_method"))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost:
	default:
		return probeHTTPRequest{}, errors.New("unsupported probe method")
	}
	body, err := probeBody(values)
	if err != nil {
		return probeHTTPRequest{}, err
	}
	return probeHTTPRequest{
		Endpoint:            probeEndpoint(req),
		Method:              method,
		Headers:             probeHeaderMap(values),
		Body:                body,
		ExpectedStatusCodes: probeExpectedStatusCodes(values),
		ResponsePath:        firstMapString(values, "health_probe_response_path", "probe_response_path"),
		ResponseContains:    firstMapString(values, "health_probe_response_contains", "probe_response_contains"),
	}, nil
}

func probeConfigMaps(req contract.ProbeRequest) []map[string]any {
	return []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}
}

func probeEndpoint(req contract.ProbeRequest) string {
	if endpoint := firstMapString(probeConfigMaps(req), "health_probe_url", "probe_url", "models_url", "models_endpoint"); endpoint != "" {
		return strings.TrimRight(endpoint, "/")
	}
	baseURL := probeBaseURL(req)
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/models") {
		return baseURL
	}
	return strings.TrimRight(baseURL, "/") + "/models"
}

func probeBaseURL(req contract.ProbeRequest) string {
	keys := []string{"base_url", "upstream_base_url"}
	switch probeSource(req) {
	case "openai":
		keys = append([]string{"openai_base_url"}, keys...)
	case "anthropic":
		keys = append([]string{"anthropic_base_url"}, keys...)
	case "gemini":
		keys = append([]string{"gemini_base_url"}, keys...)
	}
	for _, values := range probeConfigMaps(req) {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func probeHeaders(req contract.ProbeRequest, endpoint *string) (http.Header, error) {
	switch probeSource(req) {
	case "anthropic":
		apiKey := firstCredentialString(req.Credential, "api_key", "x_api_key", "anthropic_api_key", "access_token")
		if apiKey == "" {
			return nil, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
		}
		headers := http.Header{"Accept": {"application/json"}}
		headers.Set("x-api-key", apiKey)
		version := firstMapString(append([]map[string]any{req.Credential}, probeConfigMaps(req)...), "anthropic_version", "anthropic-version")
		if version == "" {
			version = "2023-06-01"
		}
		headers.Set("anthropic-version", version)
		return headers, nil
	case "gemini":
		apiKey := firstCredentialString(req.Credential, "api_key", "gemini_api_key", "google_api_key", "access_token")
		if apiKey == "" {
			return nil, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
		}
		headers := http.Header{"Accept": {"application/json"}}
		authMode := firstMapString(append([]map[string]any{req.Credential}, probeConfigMaps(req)...), "auth_mode")
		if strings.EqualFold(authMode, "bearer") {
			headers.Set("Authorization", "Bearer "+apiKey)
			return headers, nil
		}
		if strings.EqualFold(authMode, "custom_header") {
			headerName := firstMapString(append([]map[string]any{req.Credential}, probeConfigMaps(req)...), "custom_header_name", "auth_header", "api_key_header")
			if headerName == "" {
				headerName = "x-goog-api-key"
			}
			headers.Set(headerName, apiKey)
			return headers, nil
		}
		*endpoint = appendAPIKeyQuery(*endpoint, apiKey, firstNonEmpty(firstMapString(append([]map[string]any{req.Credential}, probeConfigMaps(req)...), "api_key_query_param", "query_param"), "key"))
		return headers, nil
	default:
		apiKey := firstCredentialString(req.Credential, "api_key", "openai_api_key", "access_token")
		if apiKey == "" {
			return nil, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
		}
		headers := http.Header{"Accept": {"application/json"}}
		headers.Set("Authorization", "Bearer "+apiKey)
		return headers, nil
	}
}

func probeSource(req contract.ProbeRequest) string {
	for _, value := range []string{req.Provider.AdapterType, req.Provider.Protocol} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "anthropic-compatible", "claude-compatible":
			return "anthropic"
		case "gemini-compatible", "native-gemini":
			return "gemini"
		case "openai-compatible", "native-openai", genericReverseProxyAdapterType:
			return "openai"
		}
	}
	return "openai"
}

func probeHeaderMap(values []map[string]any) map[string]string {
	headers := map[string]string{}
	raw := firstMapValue(values, "health_probe_headers", "probe_headers")
	switch value := raw.(type) {
	case map[string]string:
		for key, item := range value {
			setProbeHeader(headers, key, item)
		}
	case map[string]any:
		for key, item := range value {
			setProbeHeader(headers, key, mapString(map[string]any{"value": item}, "value"))
		}
	}
	return headers
}

func setProbeHeader(headers map[string]string, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" || probeHeaderForbidden(key) {
		return
	}
	headers[key] = value
}

func probeHeaderForbidden(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "x-api-key", "x-goog-api-key", "cookie", "set-cookie", "host", "content-length", "connection", "transfer-encoding", "upgrade", "proxy-authorization", "proxy-authenticate":
		return true
	default:
		return false
	}
}

func mergeProbeHeaders(headers http.Header, overrides map[string]string) {
	for key, value := range overrides {
		headers.Set(key, value)
	}
}

func probeBody(values []map[string]any) ([]byte, error) {
	raw := firstMapValue(values, "health_probe_body", "probe_body")
	if raw == nil {
		return nil, nil
	}
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil
		}
		if !json.Valid([]byte(value)) {
			return nil, errors.New("probe body must be valid json")
		}
		return []byte(value), nil
	case []byte:
		if !json.Valid(value) {
			return nil, errors.New("probe body must be valid json")
		}
		return append([]byte(nil), value...), nil
	default:
		body, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
}

func probeExpectedStatusCodes(values []map[string]any) []int {
	raw := firstMapValue(values, "health_probe_expected_status_codes", "probe_expected_status_codes", "health_probe_expected_status")
	if raw == nil {
		return nil
	}
	return providerStatusCodeList(raw)
}

func providerStatusCodeList(value any) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0)
	appendStatus := func(raw any) {
		status := providerStatusCode(raw)
		if status < 100 || status > 599 {
			return
		}
		if _, ok := seen[status]; ok {
			return
		}
		seen[status] = struct{}{}
		out = append(out, status)
	}
	switch value := value.(type) {
	case []int:
		for _, item := range value {
			appendStatus(item)
		}
	case []string:
		for _, item := range value {
			for _, part := range splitProbeList(item) {
				appendStatus(part)
			}
		}
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				for _, part := range splitProbeList(text) {
					appendStatus(part)
				}
				continue
			}
			appendStatus(item)
		}
	case string:
		for _, part := range splitProbeList(value) {
			appendStatus(part)
		}
	default:
		appendStatus(value)
	}
	return out
}

func providerStatusCode(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func splitProbeList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func probeStatusAllowed(status int, allowed []int) bool {
	if len(allowed) == 0 {
		return status >= 200 && status < 300
	}
	for _, item := range allowed {
		if item == status {
			return true
		}
	}
	return false
}

func probeBodyExpectationMet(body []byte, req probeHTTPRequest) bool {
	if strings.TrimSpace(req.ResponsePath) != "" {
		if !json.Valid(body) {
			return false
		}
		var decoded any
		if err := json.Unmarshal(body, &decoded); err == nil {
			if strings.TrimSpace(probeJSONPathString(decoded, req.ResponsePath)) == "" {
				return false
			}
		}
	}
	if needle := strings.TrimSpace(req.ResponseContains); needle != "" {
		return strings.Contains(string(body), needle)
	}
	return true
}

func probeJSONPathString(value any, path string) string {
	current := value
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return ""
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	return mapString(map[string]any{"value": current}, "value")
}

func firstCredentialString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := credentialString(values, key); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func probeResponseFromError(startedAt time.Time, err error) contract.ProbeResponse {
	var providerErr contract.ProviderError
	if errors.As(err, &providerErr) {
		return probeResponseFromProviderError(startedAt, providerErr)
	}
	return probeFailure(startedAt, "network_error", http.StatusBadGateway)
}

func probeResponseFromProviderError(startedAt time.Time, err contract.ProviderError) contract.ProbeResponse {
	class := strings.TrimSpace(err.Class)
	if class == "" {
		class = providerClassForHTTPStatus(err.StatusCode)
	}
	if err.StatusCode <= 0 {
		err.StatusCode = http.StatusBadGateway
	}
	return probeFailure(startedAt, class, err.StatusCode)
}

func probeFailure(startedAt time.Time, class string, statusCode int) contract.ProbeResponse {
	return contract.ProbeResponse{
		OK:         false,
		ErrorClass: class,
		StatusCode: statusCode,
		LatencyMS:  latencySince(startedAt),
	}
}

func latencySince(startedAt time.Time) int {
	latency := int(time.Since(startedAt) / time.Millisecond)
	if latency < 0 {
		return 0
	}
	return latency
}
