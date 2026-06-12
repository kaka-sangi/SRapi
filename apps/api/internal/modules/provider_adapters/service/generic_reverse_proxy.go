package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const genericReverseProxyAdapterType = "generic-reverse-proxy"

func (s *Service) invokeGenericReverseProxyText(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	payload, err := genericReverseProxyTextPayload(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	if req.Stream {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint, err := genericReverseProxyEndpoint(req, baseURL, "chat_path", "/chat/completions")
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	headers := genericReverseProxyHeaders(req, "chat")
	runtimeResp, err := s.doGenericReverseProxy(ctx, req.Account, req.Credential, http.MethodPost, endpoint, headers, raw, req.Stream)
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromGenericReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	if req.Stream {
		resp, err := parseOpenAICompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
		if err == nil {
			resp.Headers = runtimeResp.Headers
		}
		return resp, err
	}
	resp, err := parseGenericReverseProxyText(runtimeResp.Body, runtimeResp.StatusCode, req)
	if err == nil {
		resp.Headers = runtimeResp.Headers
	}
	return resp, err
}

func (s *Service) invokeGenericReverseProxyEmbeddings(ctx context.Context, req contract.EmbeddingRequest, baseURL string) (contract.EmbeddingResponse, error) {
	payload, err := genericReverseProxyEmbeddingPayload(req)
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	endpoint, err := genericReverseProxyEndpointFromMaps([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, baseURL, "embeddings_path", "/embeddings")
	if err != nil {
		return contract.EmbeddingResponse{}, err
	}
	headers := genericReverseProxyHeadersFromMaps([]map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "embeddings")
	runtimeResp, err := s.doGenericReverseProxy(ctx, req.Account, req.Credential, http.MethodPost, endpoint, headers, raw, false)
	if err != nil {
		return contract.EmbeddingResponse{}, providerErrorFromGenericReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.EmbeddingResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	return parseOpenAICompatibleEmbeddings(runtimeResp.Body, runtimeResp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
}

func (s *Service) doGenericReverseProxy(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, method string, endpoint string, headers http.Header, body []byte, expectStream bool) (reverseproxycontract.Response, error) {
	if genericReverseProxyUsesRuntime(account) {
		if s.reverseProxy == nil {
			return reverseproxycontract.Response{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
		}
		return s.reverseProxy.Do(ctx, reverseproxycontract.Request{
			Account:      genericReverseProxyAccount(account, credential),
			Method:       method,
			URL:          endpoint,
			Headers:      headers,
			Body:         body,
			ExpectStream: expectStream,
		})
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return reverseproxycontract.Response{}, err
	}
	httpReq.Header = headers
	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return reverseproxycontract.Response{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return reverseproxycontract.Response{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		if expectStream {
			return reverseproxycontract.Response{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return reverseproxycontract.Response{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	return reverseproxycontract.Response{StatusCode: resp.StatusCode, Headers: cloneGenericHeaders(resp.Header), Body: respBody}, nil
}

func genericReverseProxyTextPayload(req contract.ConversationRequest) (map[string]any, error) {
	raw, err := json.Marshal(openAICompatiblePayload(req))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	applyGenericBodyMapping(payload, genericReverseProxyBodyMapping([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "chat"))
	return payload, nil
}

func genericReverseProxyEmbeddingPayload(req contract.EmbeddingRequest) (map[string]any, error) {
	raw, err := json.Marshal(openAIEmbeddingPayload(req))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	applyGenericBodyMapping(payload, genericReverseProxyBodyMapping([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "embeddings"))
	return payload, nil
}

func parseGenericReverseProxyText(body []byte, statusCode int, req contract.ConversationRequest) (contract.ConversationResponse, error) {
	if statusCode < 200 || statusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPError(statusCode, body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	textPath := genericReverseProxyResponsePath(req, "text_path", "choices.0.message.content")
	text := strings.TrimSpace(genericPathString(decoded, textPath))
	if text == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	usage := genericReverseProxyUsage(decoded, req, text)
	return conversationTextResponse(text, statusCode, usage), nil
}

func genericReverseProxyUsage(decoded map[string]any, req contract.ConversationRequest, text string) contract.Usage {
	if usage, ok := genericPath(decoded, genericReverseProxyResponsePath(req, "usage_path", "usage")).(openAIUsage); ok {
		return usage.ToUsage(text)
	}
	if usage, ok := genericPath(decoded, genericReverseProxyResponsePath(req, "usage_path", "usage")).(map[string]any); ok {
		return genericReverseProxyOpenAIUsage(usage).ToUsage(text)
	}
	return estimatedUsage(text)
}

func genericReverseProxyOpenAIUsage(usage map[string]any) openAIUsage {
	parsed := openAIUsage{
		PromptTokens:             genericIntPtr(usage["prompt_tokens"]),
		CompletionTokens:         genericIntPtr(usage["completion_tokens"]),
		TotalTokens:              genericIntPtr(usage["total_tokens"]),
		InputTokens:              genericIntPtr(usage["input_tokens"]),
		OutputTokens:             genericIntPtr(usage["output_tokens"]),
		CachedTokens:             genericIntPtr(usage["cached_tokens"]),
		CacheReadTokens:          genericIntPtr(usage["cache_read_tokens"]),
		CacheReadInputTokens:     genericIntPtr(usage["cache_read_input_tokens"]),
		CacheCreationTokens:      genericIntPtr(usage["cache_creation_tokens"]),
		CacheCreationInputTokens: genericIntPtr(usage["cache_creation_input_tokens"]),
		CacheCreation5mTokens:    genericIntPtr(usage["cache_creation_ephemeral_5m_input_tokens"]),
		CacheCreation1hTokens:    genericIntPtr(usage["cache_creation_ephemeral_1h_input_tokens"]),
	}
	if details, ok := usage["input_tokens_details"].(map[string]any); ok {
		parsed.InputTokensDetails = &struct {
			CachedTokens        *int `json:"cached_tokens"`
			CacheCreationTokens *int `json:"cache_creation_tokens"`
		}{
			CachedTokens:        genericIntPtr(details["cached_tokens"]),
			CacheCreationTokens: genericIntPtr(details["cache_creation_tokens"]),
		}
	}
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		parsed.PromptTokensDetails = &struct {
			CachedTokens *int `json:"cached_tokens"`
		}{CachedTokens: genericIntPtr(details["cached_tokens"])}
	}
	if details, ok := usage["completion_tokens_details"].(map[string]any); ok {
		parsed.CompletionTokensDetails = &struct {
			ReasoningTokens *int `json:"reasoning_tokens"`
		}{ReasoningTokens: genericIntPtr(details["reasoning_tokens"])}
	}
	if details, ok := usage["output_tokens_details"].(map[string]any); ok {
		parsed.OutputTokensDetails = &struct {
			ImageTokens     *int `json:"image_tokens"`
			ReasoningTokens *int `json:"reasoning_tokens"`
		}{
			ImageTokens:     genericIntPtr(details["image_tokens"]),
			ReasoningTokens: genericIntPtr(details["reasoning_tokens"]),
		}
	}
	return parsed
}

func genericReverseProxyHeaders(req contract.ConversationRequest, endpointKind string) http.Header {
	return genericReverseProxyHeadersFromMaps([]map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, endpointKind)
}

func genericReverseProxyHeadersFromMaps(values []map[string]any, endpointKind string) http.Header {
	headers := http.Header{"Content-Type": {"application/json"}}
	for key, value := range genericReverseProxyStaticHeaders(values, endpointKind) {
		headers.Set(key, value)
	}
	authName, authValue := genericReverseProxyAuthHeader(values)
	if authName != "" && authValue != "" {
		headers.Set(authName, authValue)
	}
	return headers
}

func genericReverseProxyAuthHeader(values []map[string]any) (string, string) {
	template := firstMapString(values, "auth_header_template", "auth_header")
	token := firstMapString(values, "api_key", "access_token", "token")
	if template == "" || token == "" {
		if token == "" {
			return "", ""
		}
		template = "Authorization: Bearer {{api_key}}"
	}
	parts := strings.SplitN(template, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	name := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if name == "" || value == "" {
		return "", ""
	}
	value = strings.ReplaceAll(value, "{{api_key}}", token)
	value = strings.ReplaceAll(value, "{{access_token}}", token)
	value = strings.ReplaceAll(value, "{{token}}", token)
	return name, value
}

func genericReverseProxyStaticHeaders(values []map[string]any, endpointKind string) map[string]string {
	raw, ok := firstMapValue(values, endpointKind+"_headers", "headers").(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range raw {
		header := strings.TrimSpace(key)
		if header == "" {
			continue
		}
		stringValue := strings.TrimSpace(fmt.Sprint(value))
		if stringValue == "" {
			continue
		}
		out[header] = stringValue
	}
	return out
}

func genericReverseProxyEndpoint(req contract.ConversationRequest, baseURL string, pathKey string, defaultPath string) (string, error) {
	return genericReverseProxyEndpointFromMaps([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, baseURL, pathKey, defaultPath)
}

func genericReverseProxyEndpointFromMaps(values []map[string]any, baseURL string, pathKey string, defaultPath string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := firstMapString(values, pathKey, "path")
	if path == "" {
		path = defaultPath
	}
	parsed, err := url.Parse(path)
	if err == nil && parsed.IsAbs() {
		return parsed.String(), nil
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if _, err := url.ParseRequestURI(baseURL + path); err != nil {
		return "", contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "generic reverse proxy endpoint invalid"}
	}
	return baseURL + path, nil
}

func genericReverseProxyAccount(account accountcontract.ProviderAccount, credential map[string]any) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      account.ID,
		RuntimeClass:   string(account.RuntimeClass),
		UpstreamClient: account.UpstreamClient,
		ProxyID:        account.ProxyID,
		UserAgent:      mapString(account.Metadata, "user_agent"),
		Metadata:       account.Metadata,
		Credential:     credential,
	}
}

func genericReverseProxyBodyMapping(values []map[string]any, endpointKind string) map[string]any {
	raw, ok := firstMapValue(values, endpointKind+"_body_mapping_rules", "body_mapping_rules").(map[string]any)
	if !ok {
		return nil
	}
	return raw
}

func applyGenericBodyMapping(payload map[string]any, rules map[string]any) {
	for key, value := range rules {
		switch strings.TrimSpace(key) {
		case "model_field":
			renameGenericPayloadField(payload, "model", value)
		case "messages_field":
			renameGenericPayloadField(payload, "messages", value)
		case "input_field":
			renameGenericPayloadField(payload, "input", value)
		case "stream_field":
			renameGenericPayloadField(payload, "stream", value)
		case "remove_fields":
			removeGenericPayloadFields(payload, value)
		case "extra":
			mergeGenericPayloadExtra(payload, value)
		}
	}
}

func renameGenericPayloadField(payload map[string]any, from string, rawTo any) {
	to := strings.TrimSpace(fmt.Sprint(rawTo))
	if to == "" || to == from {
		return
	}
	value, ok := payload[from]
	if !ok {
		return
	}
	delete(payload, from)
	payload[to] = value
}

func removeGenericPayloadFields(payload map[string]any, raw any) {
	for _, field := range genericStringSlice(raw) {
		delete(payload, field)
	}
}

func mergeGenericPayloadExtra(payload map[string]any, raw any) {
	extra, ok := raw.(map[string]any)
	if !ok {
		return
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		payload[key] = value
	}
}

func genericReverseProxyResponsePath(req contract.ConversationRequest, key string, fallback string) string {
	if responseRules, ok := firstMapValue([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, "response_path_rules").(map[string]any); ok {
		if value := mapString(responseRules, key); value != "" {
			return value
		}
	}
	if value := firstMapString([]map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}, key); value != "" {
		return value
	}
	return fallback
}

func genericPathString(values map[string]any, path string) string {
	value := genericPath(values, path)
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func genericPath(value any, path string) any {
	current := value
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, ok := parseGenericIndex(part)
			if !ok || index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}
	return current
}

func parseGenericIndex(value string) (int, bool) {
	index, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return index, true
}

func genericIntPtr(value any) *int {
	switch typed := value.(type) {
	case float64:
		out := int(typed)
		return &out
	case int:
		out := typed
		return &out
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			out := int(parsed)
			return &out
		}
	case string:
		var out int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &out); err == nil {
			return &out
		}
	}
	return nil
}

func genericStringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmt.Sprint(item))
			if value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			return nil
		}
		return []string{value}
	}
}

func firstMapString(values []map[string]any, keys ...string) string {
	value, _ := firstMapValue(values, keys...).(string)
	return strings.TrimSpace(value)
}

func firstMapValue(values []map[string]any, keys ...string) any {
	for _, items := range values {
		for _, key := range keys {
			if value, ok := items[key]; ok && value != nil {
				return value
			}
		}
	}
	return nil
}

func isGenericReverseProxy(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), genericReverseProxyAdapterType)
}

func isGenericReverseProxyEmbeddings(req contract.EmbeddingRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), genericReverseProxyAdapterType)
}

func genericReverseProxyUsesRuntime(account accountcontract.ProviderAccount) bool {
	runtimeClass := strings.TrimSpace(string(account.RuntimeClass))
	return runtimeClass != "" && runtimeClass != string(accountcontract.RuntimeClassAPIKey)
}

func providerErrorFromGenericReverseProxy(err error) error {
	var providerErr contract.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	return providerErrorFromReverseProxy(err)
}

func cloneGenericHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	out := http.Header{}
	for key, values := range headers {
		out[key] = append([]string(nil), values...)
	}
	return out
}
