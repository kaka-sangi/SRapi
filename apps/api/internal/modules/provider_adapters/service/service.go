package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

type Service struct {
	client       *http.Client
	reverseProxy reverseproxycontract.Runtime
}

func New(client *http.Client) (*Service, error) {
	return NewWithReverseProxy(client, nil)
}

func NewWithReverseProxy(client *http.Client, runtime reverseproxycontract.Runtime) (*Service, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Service{client: client, reverseProxy: runtime}, nil
}

func (s *Service) InvokeText(ctx context.Context, req contract.TextRequest) (contract.TextResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.TextResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURL(req); baseURL != "" {
		if isReverseProxyRuntime(req) {
			return s.invokeReverseProxyOpenAICompatible(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatible(ctx, req, baseURL)
	}
	if isReverseProxyRuntime(req) {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	text := synthesizeLocalText(req.Model, req.Prompt)
	return contract.TextResponse{
		Text:       text,
		StatusCode: http.StatusOK,
		Usage:      estimatedUsage(text),
	}, nil
}

func (s *Service) invokeOpenAICompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	payload := openAICompatiblePayload(req)
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return contract.TextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.TextResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if req.Stream {
			return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyProviderHTTPError(resp.StatusCode, body)
	}
	if req.Stream {
		return parseOpenAICompatibleStream(body, resp.StatusCode)
	}

	var decoded openAIChatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: resp.StatusCode,
		Usage:      decoded.Usage.ToUsage(text),
	}, nil
}

func (s *Service) invokeReverseProxyOpenAICompatible(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	if s.reverseProxy == nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	payload := openAICompatiblePayload(req)
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.TextResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/chat/completions",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body:         raw,
		ExpectStream: req.Stream,
	})
	if err != nil {
		return contract.TextResponse{}, providerErrorFromReverseProxy(err)
	}
	if req.Stream {
		return parseOpenAICompatibleStream(runtimeResp.Body, runtimeResp.StatusCode)
	}
	var decoded openAIChatCompletionResponse
	if err := json.Unmarshal(runtimeResp.Body, &decoded); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(decoded.FirstText())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: runtimeResp.StatusCode,
		Usage:      decoded.Usage.ToUsage(text),
	}, nil
}

type openAIChatCompletionRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIChatMessage  `json:"messages"`
	Stream         bool                 `json:"stream"`
	StreamOptions  *openAIStreamOptions `json:"stream_options,omitempty"`
	Temperature    *float32             `json:"temperature,omitempty"`
	TopP           *float32             `json:"top_p,omitempty"`
	MaxTokens      *int                 `json:"max_tokens,omitempty"`
	Stop           []string             `json:"stop,omitempty"`
	Tools          []map[string]any     `json:"tools,omitempty"`
	ToolChoice     any                  `json:"tool_choice,omitempty"`
	ResponseFormat map[string]any       `json:"response_format,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func openAICompatiblePayload(req contract.TextRequest) openAIChatCompletionRequest {
	payload := openAIChatCompletionRequest{
		Model:          req.Mapping.UpstreamModelName,
		Messages:       openAICompatibleMessages(req),
		Stream:         req.Stream,
		Temperature:    req.Temperature,
		TopP:           req.TopP,
		MaxTokens:      req.MaxOutputTokens,
		Stop:           cloneStrings(req.Stop),
		Tools:          cloneMapSlice(req.Tools),
		ToolChoice:     cloneAny(req.ToolChoice),
		ResponseFormat: cloneMap(req.ResponseFormat),
	}
	if req.Stream {
		payload.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}
	return payload
}

func openAICompatibleMessages(req contract.TextRequest) []openAIChatMessage {
	out := make([]openAIChatMessage, 0, len(req.Messages)+2)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		out = append(out, openAIChatMessage{Role: "system", Content: instructions})
	}
	hasConversationMessage := false
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		out = append(out, openAIChatMessage{Role: role, Content: content})
		hasConversationMessage = true
	}
	if !hasConversationMessage {
		out = append(out, openAIChatMessage{Role: "user", Content: strings.TrimSpace(req.Prompt)})
	}
	return out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, len(values))
	for idx, value := range values {
		out[idx] = cloneMap(value)
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneAny(item)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []map[string]any:
		return cloneMapSlice(typed)
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneAny(item)
		}
		return out
	default:
		return typed
	}
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
	Usage openAIUsage `json:"usage"`
}

func (r openAIChatCompletionResponse) FirstText() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}

type openAIChatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage"`
}

type openAIUsage struct {
	PromptTokens        *int `json:"prompt_tokens"`
	CompletionTokens    *int `json:"completion_tokens"`
	TotalTokens         *int `json:"total_tokens"`
	InputTokens         *int `json:"input_tokens"`
	OutputTokens        *int `json:"output_tokens"`
	CachedTokens        *int `json:"cached_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u openAIUsage) ToUsage(text string) contract.Usage {
	input := valueOrZero(u.InputTokens)
	if input == 0 {
		input = valueOrZero(u.PromptTokens)
	}
	output := valueOrZero(u.OutputTokens)
	if output == 0 {
		output = valueOrZero(u.CompletionTokens)
	}
	cached := valueOrZero(u.CachedTokens)
	if cached == 0 && u.PromptTokensDetails != nil {
		cached = valueOrZero(u.PromptTokensDetails.CachedTokens)
	}
	total := input + output + cached
	if u.TotalTokens != nil && *u.TotalTokens > 0 && total == 0 {
		total = *u.TotalTokens
	}
	if total > 0 && output == 0 {
		output = max(0, total-input-cached)
	}
	if input == 0 && output == 0 && cached == 0 {
		return estimatedUsage(text)
	}
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: output,
		CachedTokens: cached,
		Estimated:    false,
	}
}

func parseOpenAICompatibleStream(body []byte, statusCode int) (contract.TextResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var builder strings.Builder
	var usage *openAIUsage
	done := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			done = true
			break
		}
		var chunk openAIChatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		if chunk.Usage != nil {
			copied := *chunk.Usage
			usage = &copied
		}
		for _, choice := range chunk.Choices {
			builder.WriteString(choice.Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	if !done {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before done"}
	}
	text := strings.TrimSpace(builder.String())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	parsedUsage := estimatedUsage(text)
	if usage != nil {
		parsedUsage = usage.ToUsage(text)
	}
	return contract.TextResponse{
		Text:       text,
		StatusCode: statusCode,
		Usage:      parsedUsage,
	}, nil
}

func upstreamBaseURL(req contract.TextRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func isReverseProxyRuntime(req contract.TextRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func credentialString(values map[string]any, key string) string {
	return mapString(values, key)
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func classifyProviderHTTPError(statusCode int, body []byte) contract.ProviderError {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return contract.ProviderError{Class: providerClassForHTTPStatus(statusCode), StatusCode: statusCode, Message: message}
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
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "timeout"
	default:
		if statusCode >= 500 {
			return "provider_5xx"
		}
	}
	return "unknown"
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

func synthesizeLocalText(model, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "SRapi local response for " + model
	}
	return "SRapi local response for " + model + ": " + prompt
}

func estimatedUsage(text string) contract.Usage {
	total := estimateTokens(text)
	input := total / 2
	return contract.Usage{
		InputTokens:  input,
		OutputTokens: total - input,
		CachedTokens: 0,
		Estimated:    true,
	}
}

func estimateTokens(text string) int {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		if text == "" {
			return 1
		}
		return max(1, len(text)/4)
	}
	return max(1, len(fields)*2)
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
