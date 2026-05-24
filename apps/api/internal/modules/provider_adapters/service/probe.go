package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
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
	endpoint := probeEndpoint(req)
	if endpoint == "" {
		return probeFailure(startedAt, "invalid_request", http.StatusBadRequest), nil
	}
	headers, err := probeHeaders(req, &endpoint)
	if err != nil {
		return probeFailure(startedAt, "auth_failed", http.StatusUnauthorized), nil
	}
	resp, err := s.doProbe(ctx, req.Account, endpoint, headers)
	if err != nil {
		return probeResponseFromError(startedAt, err), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		providerErr := classifyProviderHTTPError(resp.StatusCode, resp.Body)
		return probeResponseFromProviderError(startedAt, providerErr), nil
	}
	return contract.ProbeResponse{
		OK:         true,
		StatusCode: resp.StatusCode,
		LatencyMS:  latencySince(startedAt),
		Metadata: map[string]any{
			"endpoint": endpoint,
		},
	}, nil
}

func (s *Service) doProbe(ctx context.Context, account accountcontract.ProviderAccount, endpoint string, headers http.Header) (probeHTTPResponse, error) {
	if account.RuntimeClass != accountcontract.RuntimeClassAPIKey {
		return probeHTTPResponse{}, contract.ProviderError{Class: "unsupported_runtime", StatusCode: http.StatusBadRequest, Message: "account runtime cannot be probed without reverse proxy runtime"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, bytes.NewReader(nil))
	if err != nil {
		return probeHTTPResponse{}, err
	}
	httpReq.Header = headers
	resp, err := s.client.Do(httpReq)
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

func probeEndpoint(req contract.ProbeRequest) string {
	if endpoint := firstMapString([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "health_probe_url", "probe_url", "models_url", "models_endpoint"); endpoint != "" {
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
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
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
		version := firstMapString([]map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "anthropic_version", "anthropic-version")
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
		authMode := firstMapString([]map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "auth_mode")
		if strings.EqualFold(authMode, "bearer") {
			headers.Set("Authorization", "Bearer "+apiKey)
			return headers, nil
		}
		if strings.EqualFold(authMode, "custom_header") {
			headerName := firstMapString([]map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "custom_header_name", "auth_header", "api_key_header")
			if headerName == "" {
				headerName = "x-goog-api-key"
			}
			headers.Set(headerName, apiKey)
			return headers, nil
		}
		*endpoint = appendAPIKeyQuery(*endpoint, apiKey, firstNonEmpty(firstMapString([]map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "api_key_query_param", "query_param"), "key"))
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
