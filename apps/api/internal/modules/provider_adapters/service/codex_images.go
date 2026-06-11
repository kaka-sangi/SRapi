package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const codexImageGenerationDefaultResponsesModel = "gpt-5.4-mini"

func (s *Service) invokeReverseProxyCodexImageGeneration(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if codexImageGenerationRuntimeIsAPIKey(req) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	payload, err := codexImageGenerationResponsesPayload(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: codexImageGenerationReverseProxyAccount(req),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(baseURL, "/") + "/responses",
		Headers: codexImageGenerationHeaders(req, payload),
		Body:    raw,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyCodexProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	return parseCodexImageGenerationResponse(runtimeResp.Body, runtimeResp.StatusCode, req)
}

func codexImageGenerationResponsesPayload(req contract.ImageGenerationRequest) (map[string]any, error) {
	model := codexImageGenerationResponsesModel(req)
	if strings.TrimSpace(model) == "gpt-5.3-codex-spark" {
		return nil, contract.ProviderError{
			Class:      "not_supported",
			StatusCode: http.StatusBadRequest,
			Message:    "codex spark model does not support image generation",
		}
	}
	tool := map[string]any{
		"type":   "image_generation",
		"action": "generate",
		"model":  codexImageGenerationToolModel(req),
	}
	if req.Count > 0 {
		tool["n"] = req.Count
	}
	for _, item := range []struct {
		key   string
		value any
	}{
		{key: "size", value: req.Size},
		{key: "quality", value: req.Quality},
		{key: "background", value: req.Extra["background"]},
		{key: "output_format", value: req.Extra["output_format"]},
		{key: "moderation", value: req.Extra["moderation"]},
	} {
		if value := strings.TrimSpace(codexStringValue(item.value)); value != "" {
			tool[item.key] = value
		}
	}
	for _, key := range []string{"output_compression", "partial_images"} {
		if value, ok := codexImageGenerationIntValue(req.Extra[key]); ok {
			tool[key] = value
		}
	}
	payload := map[string]any{
		"model":               model,
		"stream":              true,
		"reasoning":           map[string]any{"effort": "medium", "summary": "auto"},
		"parallel_tool_calls": true,
		"input":               codexStringInputMessage(req.Prompt),
		"tools":               []any{tool},
		"tool_choice":         map[string]any{"type": "image_generation"},
	}
	codexEnsureResponsesInstructions(codexImageGenerationConversationRequest(req), payload)
	codexApplyImageGenerationInstructions(payload)
	codexEnsureReasoningEncryptedInclude(payload)
	payload["store"] = codexResponsesDefaultInternalStoreValue
	return payload, nil
}

func codexImageGenerationResponsesModel(req contract.ImageGenerationRequest) string {
	if model := contract.NormalizeCodexUpstreamModelName(codexImageGenerationSetting(req, "codex_image_generation_responses_model", "image_generation_responses_model", "gpt_image_2_base_model")); model != "" {
		return model
	}
	model := contract.NormalizeCodexUpstreamModelName(req.Mapping.UpstreamModelName)
	if model == "" || codexImageGenerationIsImageModel(model) {
		return codexImageGenerationDefaultResponsesModel
	}
	return model
}

func codexImageGenerationToolModel(req contract.ImageGenerationRequest) string {
	if model := strings.TrimSpace(codexStringValue(req.Extra["model"])); model != "" {
		return model
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		return model
	}
	return req.Mapping.UpstreamModelName
}

func codexImageGenerationIsImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-")
}

func codexImageGenerationMIMEType(outputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func codexImageGenerationIntValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed), true
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		if parsed, err := strconv.Atoi(trimmed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func parseCodexImageGenerationResponse(body []byte, statusCode int, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	parsed, err := parseCodexResponsesBody(body, statusCode)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	images := make([]contract.Image, 0, len(parsed.Parts))
	for _, part := range parsed.Parts {
		if part.Kind != contract.ContentPartImage || strings.TrimSpace(part.MediaBase64) == "" {
			continue
		}
		image := contract.Image{
			Metadata:      cloneMap(part.Metadata),
			RevisedPrompt: strings.TrimSpace(mapString(part.Metadata, "revised_prompt")),
		}
		if strings.EqualFold(strings.TrimSpace(req.ResponseFormat), "url") {
			image.URL = "data:" + codexImageGenerationMIMEType(mapString(part.Metadata, "output_format")) + ";base64," + strings.TrimSpace(part.MediaBase64)
		} else {
			image.Base64JSON = strings.TrimSpace(part.MediaBase64)
		}
		images = append(images, image)
	}
	if len(images) == 0 {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no images"}
	}
	return contract.ImageGenerationResponse{
		Created:    time.Now().Unix(),
		Data:       images,
		Model:      strings.TrimSpace(req.Mapping.UpstreamModelName),
		StatusCode: statusCode,
		Usage:      parsed.Usage,
	}, nil
}

func isCodexImageGenerationReverseProxy(req contract.ImageGenerationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-codex-cli")
}

func codexImageGenerationRuntimeIsAPIKey(req contract.ImageGenerationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func codexImageGenerationReverseProxyAccount(req contract.ImageGenerationRequest) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: req.Account.UpstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Metadata:       req.Account.Metadata,
		Credential:     req.Credential,
	}
}

func codexImageGenerationHeaders(req contract.ImageGenerationRequest, payload map[string]any) http.Header {
	headers := http.Header{
		"Accept":       {"text/event-stream"},
		"Content-Type": {"application/json"},
	}
	headers.Set("OpenAI-Beta", codexResponsesBetaHeaderValue)
	headers.Set("Originator", codexImageGenerationSetting(req, "codex_originator", "originator"))
	if headers.Get("Originator") == "" {
		headers.Set("Originator", codexOriginator)
	}
	headers.Set("User-Agent", codexImageGenerationSetting(req, "user_agent"))
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", codexDefaultUserAgent)
	}
	if accountID := codexImageGenerationSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	if version := codexImageGenerationSetting(req, "codex_version", "version", "Version"); version != "" {
		headers.Set("Version", version)
	} else {
		headers.Set("Version", codexDefaultVersion)
	}
	if requestID := codexImageGenerationSetting(req, "codex_client_request_id", "x_client_request_id", "X-Client-Request-Id"); requestID != "" {
		headers.Set("X-Client-Request-Id", requestID)
	} else if strings.TrimSpace(req.RequestID) != "" {
		headers.Set("X-Client-Request-Id", strings.TrimSpace(req.RequestID))
	}
	if sessionID := codexImageGenerationSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("Session_id", sessionID)
	} else if req.Account.ID > 0 {
		headers.Set("Session_id", codexDefaultAccountSessionID(req.Account.ID))
	}
	codexApplySessionIdentityHeaders(headers, codexPayloadPromptCacheKey(payload))
	return headers
}

func codexImageGenerationSetting(req contract.ImageGenerationRequest, keys ...string) string {
	for _, values := range []map[string]any{req.Credential, req.Extra, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func codexImageGenerationConversationRequest(req contract.ImageGenerationRequest) contract.ConversationRequest {
	return contract.ConversationRequest{
		RequestID:  req.RequestID,
		Model:      req.Model,
		Provider:   req.Provider,
		Account:    req.Account,
		Mapping:    req.Mapping,
		Credential: req.Credential,
	}
}
