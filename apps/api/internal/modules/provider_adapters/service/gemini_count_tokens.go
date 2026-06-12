package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeTokenCount(ctx context.Context, req contract.TokenCountRequest) (contract.TokenCountResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || len(req.RawBody) == 0 {
		return contract.TokenCountResponse{}, ErrInvalidInput
	}
	baseURL := upstreamBaseURLTokenCount(req)
	if baseURL == "" {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "token count upstream base url missing"}
	}
	if isAnthropicTokenCountCompatible(req) {
		if isReverseProxyTokenCountRuntime(req) {
			return s.invokeReverseProxyAnthropicTokenCount(ctx, req, baseURL)
		}
		return s.invokeAnthropicTokenCount(ctx, req, baseURL)
	}
	if isGeminiTokenCountCompatible(req) {
		if isReverseProxyTokenCountRuntime(req) {
			return s.invokeReverseProxyGeminiTokenCount(ctx, req, baseURL)
		}
		return s.invokeGeminiTokenCount(ctx, req, baseURL)
	}
	return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "token count adapter unsupported"}
}

func (s *Service) invokeGeminiTokenCount(ctx context.Context, req contract.TokenCountRequest, baseURL string) (contract.TokenCountResponse, error) {
	apiKey := geminiTokenCountAPIKey(req)
	if apiKey == "" {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	endpoint := geminiCountTokensEndpoint(baseURL, req.Mapping.UpstreamModelName)
	headers := geminiTokenCountHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(req.RawBody))
	if err != nil {
		return contract.TokenCountResponse{}, err
	}
	httpReq.Header = headers

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.TokenCountResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.TokenCountResponse{}, classifyGeminiProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	return parseGeminiTokenCountResponse(body, resp.StatusCode)
}

func (s *Service) invokeReverseProxyGeminiTokenCount(ctx context.Context, req contract.TokenCountRequest, baseURL string) (contract.TokenCountResponse, error) {
	if s.reverseProxy == nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Metadata:       req.Account.Metadata,
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    geminiCountTokensEndpoint(baseURL, req.Mapping.UpstreamModelName),
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: req.RawBody,
	})
	if err != nil {
		return contract.TokenCountResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.TokenCountResponse{}, classifyGeminiProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	return parseGeminiTokenCountResponse(runtimeResp.Body, runtimeResp.StatusCode)
}

func (s *Service) invokeAnthropicTokenCount(ctx context.Context, req contract.TokenCountRequest, baseURL string) (contract.TokenCountResponse, error) {
	apiKey := anthropicAPIKeyFromTokenCount(req)
	if apiKey == "" {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	body, err := anthropicTokenCountPayload(req)
	if err != nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid Anthropic count_tokens request"}
	}
	endpoint := anthropicCountTokensEndpoint(baseURL)
	headers := anthropicTokenCountHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return contract.TokenCountResponse{}, err
	}
	httpReq.Header = headers

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.TokenCountResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.TokenCountResponse{}, classifyAnthropicProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, respBody)
	}
	return parseAnthropicTokenCountResponse(respBody, resp.StatusCode)
}

func (s *Service) invokeReverseProxyAnthropicTokenCount(ctx context.Context, req contract.TokenCountRequest, baseURL string) (contract.TokenCountResponse, error) {
	if s.reverseProxy == nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if isClaudeCodeTokenCountReverseProxy(req) && claudeCodeTokenCountReverseProxyRuntimeIsAPIKey(req) {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "Claude Code reverse proxy requires OAuth or client-token runtime"}
	}
	body, err := anthropicTokenCountPayload(req)
	if err != nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid Anthropic count_tokens request"}
	}
	endpoint := anthropicCountTokensEndpoint(baseURL)
	headers := http.Header{
		"Content-Type": {"application/json"},
	}
	if isClaudeCodeTokenCountReverseProxy(req) {
		body, err = claudeCodeTokenCountPayload(req, body)
		if err != nil {
			return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid Claude Code count_tokens request"}
		}
		endpoint = claudeCodeCountTokensEndpoint(baseURL)
		headers = claudeCodeTokenCountHeaders(req)
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Metadata:       req.Account.Metadata,
			Credential:     req.Credential,
		},
		Method:  http.MethodPost,
		URL:     endpoint,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		return contract.TokenCountResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.TokenCountResponse{}, classifyAnthropicProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	return parseAnthropicTokenCountResponse(runtimeResp.Body, runtimeResp.StatusCode)
}

type geminiTokenCountResponse struct {
	TotalTokens             int                        `json:"totalTokens"`
	CachedContentTokenCount *int                       `json:"cachedContentTokenCount"`
	PromptTokensDetails     []geminiModalityTokenCount `json:"promptTokensDetails"`
	CacheTokensDetails      []geminiModalityTokenCount `json:"cacheTokensDetails"`
	Extra                   map[string]any             `json:"-"`
}

type geminiModalityTokenCount struct {
	Modality   string         `json:"modality"`
	TokenCount int            `json:"tokenCount"`
	Extra      map[string]any `json:"-"`
}

type anthropicTokenCountResponse struct {
	InputTokens int            `json:"input_tokens"`
	Extra       map[string]any `json:"-"`
}

func parseGeminiTokenCountResponse(body []byte, statusCode int) (contract.TokenCountResponse, error) {
	var decoded geminiTokenCountResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	if decoded.TotalTokens < 0 {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid token count"}
	}
	return contract.TokenCountResponse{
		TotalTokens:             decoded.TotalTokens,
		CachedContentTokenCount: cloneIntPtr(decoded.CachedContentTokenCount),
		PromptTokensDetails:     tokenCountDetails(decoded.PromptTokensDetails),
		CacheTokensDetails:      tokenCountDetails(decoded.CacheTokensDetails),
		StatusCode:              statusCode,
		Metadata:                cloneMap(decoded.Extra),
	}, nil
}

func (r *geminiTokenCountResponse) UnmarshalJSON(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	type alias geminiTokenCountResponse
	var decoded alias
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	delete(raw, "totalTokens")
	delete(raw, "cachedContentTokenCount")
	delete(raw, "promptTokensDetails")
	delete(raw, "cacheTokensDetails")
	*r = geminiTokenCountResponse(decoded)
	r.Extra = raw
	return nil
}

func (r *geminiModalityTokenCount) UnmarshalJSON(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	type alias geminiModalityTokenCount
	var decoded alias
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	delete(raw, "modality")
	delete(raw, "tokenCount")
	*r = geminiModalityTokenCount(decoded)
	r.Extra = raw
	return nil
}

func tokenCountDetails(values []geminiModalityTokenCount) []contract.ModalityTokenCount {
	if len(values) == 0 {
		return nil
	}
	out := make([]contract.ModalityTokenCount, 0, len(values))
	for _, value := range values {
		out = append(out, contract.ModalityTokenCount{
			Modality:   strings.TrimSpace(value.Modality),
			TokenCount: value.TokenCount,
			Metadata:   cloneMap(value.Extra),
		})
	}
	return out
}

func parseAnthropicTokenCountResponse(body []byte, statusCode int) (contract.TokenCountResponse, error) {
	var decoded anthropicTokenCountResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	if decoded.InputTokens < 0 {
		return contract.TokenCountResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid token count"}
	}
	return contract.TokenCountResponse{
		TotalTokens: decoded.InputTokens,
		StatusCode:  statusCode,
		Metadata:    cloneMap(decoded.Extra),
	}, nil
}

func (r *anthropicTokenCountResponse) UnmarshalJSON(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	type alias anthropicTokenCountResponse
	var decoded alias
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	delete(raw, "input_tokens")
	*r = anthropicTokenCountResponse(decoded)
	r.Extra = raw
	return nil
}

func anthropicTokenCountPayload(req contract.TokenCountRequest) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.RawBody, &payload); err != nil {
		return nil, err
	}
	if model := strings.TrimSpace(req.Mapping.UpstreamModelName); model != "" {
		payload["model"] = model
	}
	return json.Marshal(payload)
}

func upstreamBaseURLTokenCount(req contract.TokenCountRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "gemini_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func isGeminiTokenCountCompatible(req contract.TokenCountRequest) bool {
	for _, value := range []string{req.Provider.Protocol, req.Provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "gemini-compatible", "native-gemini", "reverse-proxy-gemini-cli":
			return true
		}
	}
	return false
}

func isAnthropicTokenCountCompatible(req contract.TokenCountRequest) bool {
	for _, value := range []string{req.Provider.Protocol, req.Provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "anthropic-compatible", "native-anthropic", "reverse-proxy-claude-code-cli":
			return true
		}
	}
	return false
}

func isReverseProxyTokenCountRuntime(req contract.TokenCountRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func geminiTokenCountAPIKey(req contract.TokenCountRequest) string {
	for _, key := range []string{"api_key", "gemini_api_key", "google_api_key", "access_token"} {
		if value := credentialString(req.Credential, key); value != "" {
			return value
		}
	}
	return ""
}

func anthropicAPIKeyFromTokenCount(req contract.TokenCountRequest) string {
	for _, key := range []string{"api_key", "x_api_key", "anthropic_api_key", "access_token"} {
		if value := credentialString(req.Credential, key); value != "" {
			return value
		}
	}
	return ""
}

func geminiTokenCountHeaders(req contract.TokenCountRequest, apiKey string, endpoint *string) http.Header {
	headers := http.Header{
		"Content-Type": {"application/json"},
	}
	if apiKey == "" {
		return headers
	}
	switch strings.ToLower(requestSettingTokenCount(req, "auth_mode")) {
	case "bearer":
		headers.Set("Authorization", "Bearer "+apiKey)
	case "custom_header":
		headerName := requestSettingTokenCount(req, "custom_header_name", "auth_header", "api_key_header")
		if headerName == "" {
			headerName = "x-goog-api-key"
		}
		headers.Set(headerName, apiKey)
	case "api_key_header", "x_goog_api_key":
		headers.Set("x-goog-api-key", apiKey)
	case "api_key_query", "":
		if endpoint != nil {
			*endpoint = appendAPIKeyQuery(*endpoint, apiKey, requestSettingTokenCount(req, "api_key_query_param", "query_param"))
		}
	default:
		if endpoint != nil {
			*endpoint = appendAPIKeyQuery(*endpoint, apiKey, "key")
		}
	}
	return headers
}

func geminiCountTokensEndpoint(baseURL string, model string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	model = strings.Trim(strings.TrimSpace(model), "/")
	model = strings.TrimPrefix(model, "models/")
	escapedModel := strings.ReplaceAll(url.PathEscape(model), "%2F", "/")
	switch {
	case strings.HasSuffix(baseURL, ":countTokens"), strings.HasSuffix(baseURL, ":generateContent"), strings.HasSuffix(baseURL, ":streamGenerateContent"):
		idx := strings.LastIndex(baseURL, ":")
		return baseURL[:idx] + ":countTokens"
	case strings.Contains(baseURL, "/models/") || strings.HasSuffix(baseURL, "/models"):
		return baseURL + "/" + escapedModel + ":countTokens"
	default:
		return baseURL + "/models/" + escapedModel + ":countTokens"
	}
}

func anthropicCountTokensEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/messages/count_tokens") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/messages") {
		return strings.TrimSuffix(baseURL, "/messages") + "/messages/count_tokens"
	}
	return baseURL + "/messages/count_tokens"
}

func requestSettingTokenCount(req contract.TokenCountRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func anthropicTokenCountHeaders(req contract.TokenCountRequest, apiKey string, endpoint *string) http.Header {
	headers := http.Header{
		"Content-Type":      {"application/json"},
		"anthropic-version": {"2023-06-01"},
	}
	if version := requestSettingTokenCount(req, "anthropic_version", "anthropic-version"); version != "" {
		headers.Set("anthropic-version", version)
	}
	if apiKey == "" {
		return headers
	}
	switch strings.ToLower(requestSettingTokenCount(req, "auth_mode")) {
	case "bearer":
		headers.Set("Authorization", "Bearer "+apiKey)
	case "api_key_query":
		if endpoint != nil {
			*endpoint = appendAPIKeyQuery(*endpoint, apiKey, requestSettingTokenCount(req, "api_key_query_param", "query_param"))
		}
	case "custom_header":
		headerName := requestSettingTokenCount(req, "custom_header_name", "auth_header", "api_key_header")
		if headerName == "" {
			headerName = "x-api-key"
		}
		headers.Set(headerName, apiKey)
	case "x_api_key", "api_key_header", "":
		headers.Set("x-api-key", apiKey)
	default:
		headers.Set("x-api-key", apiKey)
	}
	return headers
}
