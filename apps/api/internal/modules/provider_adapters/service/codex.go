package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const codexOriginator = "codex_cli_rs"

type codexResponsesRequest struct {
	Model           string                    `json:"model"`
	Input           []codexResponsesInputItem `json:"input"`
	Instructions    string                    `json:"instructions"`
	Stream          bool                      `json:"stream"`
	Temperature     *float32                  `json:"temperature,omitempty"`
	TopP            *float32                  `json:"top_p,omitempty"`
	MaxOutputTokens *int                      `json:"max_output_tokens,omitempty"`
	Stop            []string                  `json:"stop,omitempty"`
	Tools           []map[string]any          `json:"tools,omitempty"`
	ToolChoice      any                       `json:"tool_choice,omitempty"`
	ResponseFormat  map[string]any            `json:"response_format,omitempty"`
	PromptCacheKey  string                    `json:"prompt_cache_key,omitempty"`
}

type codexResponsesInputItem struct {
	Type    string                       `json:"type"`
	Role    string                       `json:"role,omitempty"`
	Content []codexResponsesInputContent `json:"content"`
}

type codexResponsesInputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type codexResponsesEvent struct {
	Type        string                    `json:"type"`
	Delta       string                    `json:"delta"`
	Text        string                    `json:"text"`
	Item        *codexResponsesOutputItem `json:"item"`
	OutputIndex *int                      `json:"output_index"`
	Response    *codexResponsesResponse   `json:"response"`
	Usage       *openAIUsage              `json:"usage"`
	Error       *codexResponsesError      `json:"error"`
	Message     string                    `json:"message"`
	Code        string                    `json:"code"`
}

type codexResponsesResponse struct {
	Output     []codexResponsesOutputItem `json:"output"`
	OutputText string                     `json:"output_text"`
	Usage      openAIUsage                `json:"usage"`
	Error      *codexResponsesError       `json:"error"`
}

type codexResponsesOutputItem struct {
	Type    string                        `json:"type"`
	Text    string                        `json:"text"`
	Content []codexResponsesOutputContent `json:"content"`
}

type codexResponsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexResponsesError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Type    string `json:"type"`
}

func (s *Service) invokeReverseProxyCodexResponses(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	if s.reverseProxy == nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if codexReverseProxyRuntimeIsAPIKey(req) {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	raw, err := json.Marshal(codexResponsesPayload(req))
	if err != nil {
		return contract.TextResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account:      codexReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(baseURL, "/") + "/responses",
		Headers:      codexResponsesHeaders(req),
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.TextResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	return parseCodexResponsesBody(runtimeResp.Body, runtimeResp.StatusCode)
}

func codexResponsesPayload(req contract.TextRequest) codexResponsesRequest {
	payload := codexResponsesRequest{
		Model:           req.Mapping.UpstreamModelName,
		Input:           codexResponsesInput(req),
		Instructions:    codexResponsesInstructions(req),
		Stream:          true,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxOutputTokens,
		Stop:            cloneStrings(req.Stop),
		Tools:           cloneMapSlice(req.Tools),
		ToolChoice:      cloneAny(req.ToolChoice),
		ResponseFormat:  cloneMap(req.ResponseFormat),
	}
	if promptCacheKey := requestSetting(req, "codex_prompt_cache_key", "prompt_cache_key"); promptCacheKey != "" {
		payload.PromptCacheKey = promptCacheKey
	}
	return payload
}

func codexResponsesInput(req contract.TextRequest) []codexResponsesInputItem {
	out := make([]codexResponsesInputItem, 0, len(req.Messages)+1)
	for _, message := range req.Messages {
		role := codexResponsesRole(message.Role)
		if role == "system" || role == "developer" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		contentType := "input_text"
		if role == "assistant" {
			contentType = "output_text"
		}
		out = append(out, codexResponsesInputItem{
			Type: "message",
			Role: role,
			Content: []codexResponsesInputContent{{
				Type: contentType,
				Text: content,
			}},
		})
	}
	if len(out) == 0 {
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			prompt = strings.TrimSpace(req.Instructions)
		}
		out = append(out, codexResponsesInputItem{
			Type:    "message",
			Role:    "user",
			Content: []codexResponsesInputContent{{Type: "input_text", Text: prompt}},
		})
	}
	return out
}

func codexResponsesRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "developer":
		return "developer"
	default:
		return "user"
	}
}

func codexResponsesInstructions(req contract.TextRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		parts = append(parts, instructions)
	}
	for _, message := range req.Messages {
		role := codexResponsesRole(message.Role)
		if role != "system" && role != "developer" {
			continue
		}
		if content := strings.TrimSpace(message.Content); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(uniqueTrimmedStrings(parts), "\n")
}

func codexResponsesHeaders(req contract.TextRequest) http.Header {
	headers := http.Header{
		"Accept":       {"text/event-stream"},
		"Content-Type": {"application/json"},
	}
	headers.Set("Originator", codexOriginator)
	if accountID := requestSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("Chatgpt-Account-Id", accountID)
	}
	if betaFeatures := requestSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		headers.Set("X-Codex-Beta-Features", betaFeatures)
	}
	if version := requestSetting(req, "codex_version", "version", "Version"); version != "" {
		headers.Set("Version", version)
	}
	if turnMetadata := requestSetting(req, "codex_turn_metadata", "x_codex_turn_metadata", "X-Codex-Turn-Metadata"); turnMetadata != "" {
		headers.Set("X-Codex-Turn-Metadata", turnMetadata)
	}
	if requestID := requestSetting(req, "codex_client_request_id", "x_client_request_id", "X-Client-Request-Id"); requestID != "" {
		headers.Set("X-Client-Request-Id", requestID)
	} else if strings.TrimSpace(req.RequestID) != "" {
		headers.Set("X-Client-Request-Id", strings.TrimSpace(req.RequestID))
	}
	if sessionID := requestSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("Session_id", sessionID)
	} else if strings.Contains(requestSetting(req, "user_agent"), "Mac OS") && strings.TrimSpace(req.RequestID) != "" {
		headers.Set("Session_id", strings.TrimSpace(req.RequestID))
	}
	return headers
}

func codexReverseProxyRuntimeIsAPIKey(req contract.TextRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func codexReverseProxyAccount(req contract.TextRequest) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: req.Account.UpstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Credential:     req.Credential,
	}
}

func parseCodexResponsesBody(body []byte, statusCode int) (contract.TextResponse, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:")) {
		return parseCodexResponsesStream(trimmed, statusCode)
	}
	return parseCodexResponsesJSON(trimmed, statusCode)
}

func parseCodexResponsesStream(body []byte, statusCode int) (contract.TextResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var deltaBuilder strings.Builder
	var completedText string
	var usage *openAIUsage
	indexedItems := map[int]codexResponsesOutputItem{}
	fallbackItems := []codexResponsesOutputItem{}
	seenEvent := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
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
			break
		}
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		seenEvent = true
		if providerErr, ok := codexEventProviderError(event); ok {
			return contract.TextResponse{}, providerErr
		}
		if event.Usage != nil && event.Usage.HasTokenUsage() {
			copied := *event.Usage
			usage = &copied
		}
		if event.Type == "response.output_item.done" && event.Item != nil {
			if event.OutputIndex != nil {
				indexedItems[*event.OutputIndex] = *event.Item
			} else {
				fallbackItems = append(fallbackItems, *event.Item)
			}
		}
		if event.Response != nil && len(event.Response.Output) == 0 {
			event.Response.Output = codexCollectedOutputItems(indexedItems, fallbackItems)
		}
		if event.Response != nil && event.Response.Usage.HasTokenUsage() {
			copied := event.Response.Usage
			usage = &copied
		}
		switch event.Type {
		case "response.output_text.delta":
			deltaBuilder.WriteString(event.Delta)
		case "response.output_text.done":
			if strings.TrimSpace(event.Text) != "" {
				completedText = event.Text
			}
		case "response.completed", "response.done":
			if text := codexEventText(event); strings.TrimSpace(text) != "" {
				completedText = text
			}
		default:
			if text := codexEventText(event); strings.TrimSpace(text) != "" && strings.TrimSpace(completedText) == "" {
				completedText = text
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	if !seenEvent {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream ended before chunk"}
	}
	text := strings.TrimSpace(completedText)
	if text == "" {
		text = strings.TrimSpace(deltaBuilder.String())
	}
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	parsedUsage := estimatedUsage(text)
	if usage != nil {
		parsedUsage = usage.ToUsage(text)
	}
	return contract.TextResponse{Text: text, StatusCode: statusCode, Usage: parsedUsage}, nil
}

func parseCodexResponsesJSON(body []byte, statusCode int) (contract.TextResponse, error) {
	var event codexResponsesEvent
	if err := json.Unmarshal(body, &event); err == nil {
		if providerErr, ok := codexEventProviderError(event); ok {
			return contract.TextResponse{}, providerErr
		}
		text := strings.TrimSpace(codexEventText(event))
		if text != "" {
			return contract.TextResponse{Text: text, StatusCode: statusCode, Usage: codexEventUsage(event, text)}, nil
		}
	}
	var response codexResponsesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	if response.Error != nil {
		return contract.TextResponse{}, codexProviderError(*response.Error)
	}
	text := strings.TrimSpace(response.Text())
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return contract.TextResponse{Text: text, StatusCode: statusCode, Usage: response.Usage.ToUsage(text)}, nil
}

func codexEventText(event codexResponsesEvent) string {
	if strings.TrimSpace(event.Text) != "" {
		return event.Text
	}
	if event.Response != nil {
		return event.Response.Text()
	}
	return ""
}

func codexEventUsage(event codexResponsesEvent, text string) contract.Usage {
	if event.Response != nil && event.Response.Usage.HasTokenUsage() {
		return event.Response.Usage.ToUsage(text)
	}
	if event.Usage != nil {
		return event.Usage.ToUsage(text)
	}
	return estimatedUsage(text)
}

func codexCollectedOutputItems(indexed map[int]codexResponsesOutputItem, fallback []codexResponsesOutputItem) []codexResponsesOutputItem {
	if len(indexed) == 0 {
		return append([]codexResponsesOutputItem(nil), fallback...)
	}
	indexes := make([]int, 0, len(indexed))
	for index := range indexed {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]codexResponsesOutputItem, 0, len(indexed)+len(fallback))
	for _, index := range indexes {
		out = append(out, indexed[index])
	}
	out = append(out, fallback...)
	return out
}

func (r codexResponsesResponse) Text() string {
	if strings.TrimSpace(r.OutputText) != "" {
		return r.OutputText
	}
	parts := make([]string, 0, len(r.Output))
	for _, item := range r.Output {
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
		for _, content := range item.Content {
			contentType := strings.ToLower(strings.TrimSpace(content.Type))
			if strings.TrimSpace(content.Text) == "" || (contentType != "" && !strings.Contains(contentType, "text")) {
				continue
			}
			parts = append(parts, strings.TrimSpace(content.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func codexEventProviderError(event codexResponsesEvent) (contract.ProviderError, bool) {
	if event.Response != nil && event.Response.Error != nil {
		return codexProviderError(*event.Response.Error), true
	}
	if event.Error != nil {
		return codexProviderError(*event.Error), true
	}
	if event.Type != "error" && event.Type != "response.failed" {
		return contract.ProviderError{}, false
	}
	err := codexResponsesError{Message: event.Message, Code: event.Code}
	if err.Message == "" {
		err.Message = "codex upstream returned terminal error event"
	}
	return codexProviderError(err), true
}

func codexProviderError(err codexResponsesError) contract.ProviderError {
	message := strings.TrimSpace(err.Message)
	if message == "" {
		message = strings.TrimSpace(err.Code)
	}
	if message == "" {
		message = strings.TrimSpace(err.Type)
	}
	if message == "" {
		message = "codex upstream returned an error"
	}
	class := "provider_5xx"
	lowerCode := strings.ToLower(strings.TrimSpace(err.Code))
	lowerMessage := strings.ToLower(message)
	if strings.Contains(lowerCode, "context") ||
		strings.Contains(lowerMessage, "context length") ||
		strings.Contains(lowerMessage, "context window") ||
		strings.Contains(lowerMessage, "too many tokens") {
		class = "invalid_request"
	}
	return contract.ProviderError{Class: class, StatusCode: http.StatusBadGateway, Message: message}
}
