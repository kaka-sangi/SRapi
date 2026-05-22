package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const (
	chatGPTWebConversationPath             = "/backend-api/conversation"
	chatGPTWebAnonConversationPath         = "/backend-anon/conversation"
	chatGPTWebDefaultAcceptLanguage        = "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7"
	chatGPTWebDefaultLanguage              = "zh-CN"
	chatGPTWebDefaultTimezone              = "Asia/Shanghai"
	chatGPTWebDefaultTimezoneOffsetMinutes = -480
	chatGPTWebDefaultClientVersion         = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
	chatGPTWebDefaultClientBuildNumber     = "5955942"
)

type chatGPTWebConversationRequest struct {
	Action                     string                  `json:"action"`
	Messages                   []chatGPTWebMessage     `json:"messages"`
	Model                      string                  `json:"model"`
	ParentMessageID            string                  `json:"parent_message_id"`
	ConversationMode           map[string]string       `json:"conversation_mode"`
	ConversationOrigin         any                     `json:"conversation_origin"`
	ForceParagen               bool                    `json:"force_paragen"`
	ForceParagenModelSlug      string                  `json:"force_paragen_model_slug"`
	ForceRateLimit             bool                    `json:"force_rate_limit"`
	ForceUseSSE                bool                    `json:"force_use_sse"`
	HistoryAndTrainingDisabled bool                    `json:"history_and_training_disabled"`
	ResetRateLimits            bool                    `json:"reset_rate_limits"`
	Suggestions                []any                   `json:"suggestions"`
	SupportedEncodings         []string                `json:"supported_encodings"`
	SystemHints                []string                `json:"system_hints"`
	Timezone                   string                  `json:"timezone"`
	TimezoneOffsetMin          int                     `json:"timezone_offset_min"`
	VariantPurpose             string                  `json:"variant_purpose"`
	WebsocketRequestID         string                  `json:"websocket_request_id"`
	ClientContextualInfo       chatGPTWebClientContext `json:"client_contextual_info"`
}

type chatGPTWebMessage struct {
	ID      string                `json:"id"`
	Author  chatGPTWebAuthor      `json:"author"`
	Content chatGPTWebTextContent `json:"content"`
}

type chatGPTWebAuthor struct {
	Role string `json:"role"`
}

type chatGPTWebTextContent struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}

type chatGPTWebClientContext struct {
	IsDarkMode      bool    `json:"is_dark_mode"`
	TimeSinceLoaded int     `json:"time_since_loaded"`
	PageHeight      int     `json:"page_height"`
	PageWidth       int     `json:"page_width"`
	PixelRatio      float64 `json:"pixel_ratio"`
	ScreenHeight    int     `json:"screen_height"`
	ScreenWidth     int     `json:"screen_width"`
	AppName         string  `json:"app_name,omitempty"`
}

func (s *Service) invokeReverseProxyChatGPTWebConversation(ctx context.Context, req contract.TextRequest, baseURL string) (contract.TextResponse, error) {
	if s.reverseProxy == nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if chatGPTWebRuntimeIsAPIKey(req) {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "chatgpt web reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	path := chatGPTWebPath(req)
	headers, err := chatGPTWebConversationHeaders(req, baseURL, path)
	if err != nil {
		return contract.TextResponse{}, err
	}
	raw, err := json.Marshal(chatGPTWebConversationPayload(req))
	if err != nil {
		return contract.TextResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account:      chatGPTWebReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          chatGPTWebConversationEndpoint(baseURL, path),
		Headers:      headers,
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.TextResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.TextResponse{}, classifyProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	return parseChatGPTWebConversationBody(runtimeResp.Body, runtimeResp.StatusCode)
}

func chatGPTWebConversationPayload(req contract.TextRequest) chatGPTWebConversationRequest {
	timezone := requestSetting(req, "chatgpt_timezone", "timezone")
	if timezone == "" {
		timezone = chatGPTWebDefaultTimezone
	}
	return chatGPTWebConversationRequest{
		Action:                     "next",
		Messages:                   chatGPTWebMessages(req),
		Model:                      req.Mapping.UpstreamModelName,
		ParentMessageID:            chatGPTWebSettingOrStableID(req, "parent", "chatgpt_parent_message_id", "parent_message_id"),
		ConversationMode:           map[string]string{"kind": "primary_assistant"},
		ConversationOrigin:         nil,
		ForceParagen:               false,
		ForceParagenModelSlug:      "",
		ForceRateLimit:             false,
		ForceUseSSE:                true,
		HistoryAndTrainingDisabled: true,
		ResetRateLimits:            false,
		Suggestions:                []any{},
		SupportedEncodings:         []string{},
		SystemHints:                []string{},
		Timezone:                   timezone,
		TimezoneOffsetMin:          chatGPTWebIntSetting(req, chatGPTWebDefaultTimezoneOffsetMinutes, "chatgpt_timezone_offset_min", "timezone_offset_min"),
		VariantPurpose:             "comparison_implicit",
		WebsocketRequestID:         chatGPTWebSettingOrStableID(req, "websocket", "chatgpt_websocket_request_id", "websocket_request_id"),
		ClientContextualInfo:       chatGPTWebClientContextInfo(req),
	}
}

func chatGPTWebMessages(req contract.TextRequest) []chatGPTWebMessage {
	out := make([]chatGPTWebMessage, 0, len(req.Messages)+2)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		out = append(out, chatGPTWebMessage{
			ID:      chatGPTWebStableID(req, "instructions"),
			Author:  chatGPTWebAuthor{Role: "system"},
			Content: chatGPTWebTextContent{ContentType: "text", Parts: []string{instructions}},
		})
	}
	hasConversationMessage := false
	for idx, message := range req.Messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := chatGPTWebRole(message.Role)
		if role != "system" {
			hasConversationMessage = true
		}
		out = append(out, chatGPTWebMessage{
			ID:      chatGPTWebStableID(req, fmt.Sprintf("message-%d", idx)),
			Author:  chatGPTWebAuthor{Role: role},
			Content: chatGPTWebTextContent{ContentType: "text", Parts: []string{content}},
		})
	}
	if !hasConversationMessage {
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			if len(out) == 0 {
				prompt = strings.TrimSpace(req.Instructions)
			}
		}
		if prompt == "" && len(out) > 0 {
			return out
		}
		out = append(out, chatGPTWebMessage{
			ID:      chatGPTWebStableID(req, "prompt"),
			Author:  chatGPTWebAuthor{Role: "user"},
			Content: chatGPTWebTextContent{ContentType: "text", Parts: []string{prompt}},
		})
	}
	return out
}

func chatGPTWebConversationHeaders(req contract.TextRequest, baseURL string, path string) (http.Header, error) {
	requirementsToken := requestSetting(req, "chatgpt_requirements_token", "openai_sentinel_chat_requirements_token", "sentinel_chat_requirements_token")
	if requirementsToken == "" {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "chatgpt web requirements token missing"}
	}
	origin := chatGPTWebOrigin(baseURL)
	headers := http.Header{
		"Accept":                      {"text/event-stream"},
		"Content-Type":                {"application/json"},
		"Origin":                      {origin},
		"Referer":                     {strings.TrimRight(origin, "/") + "/"},
		"Accept-Language":             {chatGPTWebStringSetting(req, chatGPTWebDefaultAcceptLanguage, "chatgpt_accept_language", "accept_language")},
		"Cache-Control":               {"no-cache"},
		"Pragma":                      {"no-cache"},
		"Priority":                    {"u=1, i"},
		"Sec-Ch-Ua":                   {chatGPTWebStringSetting(req, `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`, "sec_ch_ua", "Sec-Ch-Ua")},
		"Sec-Ch-Ua-Arch":              {chatGPTWebStringSetting(req, `"x86"`, "sec_ch_ua_arch", "Sec-Ch-Ua-Arch")},
		"Sec-Ch-Ua-Bitness":           {chatGPTWebStringSetting(req, `"64"`, "sec_ch_ua_bitness", "Sec-Ch-Ua-Bitness")},
		"Sec-Ch-Ua-Full-Version":      {chatGPTWebStringSetting(req, `"143.0.3650.96"`, "sec_ch_ua_full_version", "Sec-Ch-Ua-Full-Version")},
		"Sec-Ch-Ua-Full-Version-List": {chatGPTWebStringSetting(req, `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`, "sec_ch_ua_full_version_list", "Sec-Ch-Ua-Full-Version-List")},
		"Sec-Ch-Ua-Mobile":            {chatGPTWebStringSetting(req, "?0", "sec_ch_ua_mobile", "Sec-Ch-Ua-Mobile")},
		"Sec-Ch-Ua-Model":             {chatGPTWebStringSetting(req, `""`, "sec_ch_ua_model", "Sec-Ch-Ua-Model")},
		"Sec-Ch-Ua-Platform":          {chatGPTWebStringSetting(req, `"Windows"`, "sec_ch_ua_platform", "Sec-Ch-Ua-Platform")},
		"Sec-Ch-Ua-Platform-Version":  {chatGPTWebStringSetting(req, `"19.0.0"`, "sec_ch_ua_platform_version", "Sec-Ch-Ua-Platform-Version")},
		"Sec-Fetch-Dest":              {"empty"},
		"Sec-Fetch-Mode":              {"cors"},
		"Sec-Fetch-Site":              {"same-origin"},
		"OAI-Device-Id":               {chatGPTWebSettingOrStableID(req, "device", "oai_device_id", "chatgpt_device_id", "device_id")},
		"OAI-Session-Id":              {chatGPTWebSettingOrStableID(req, "session", "oai_session_id", "chatgpt_session_id", "session_id")},
		"OAI-Language":                {chatGPTWebStringSetting(req, chatGPTWebDefaultLanguage, "oai_language", "chatgpt_language")},
		"OAI-Client-Version":          {chatGPTWebStringSetting(req, chatGPTWebDefaultClientVersion, "oai_client_version", "chatgpt_client_version")},
		"OAI-Client-Build-Number":     {chatGPTWebStringSetting(req, chatGPTWebDefaultClientBuildNumber, "oai_client_build_number", "chatgpt_client_build_number")},
		"X-OpenAI-Target-Path":        {path},
		"X-OpenAI-Target-Route":       {path},
		"OpenAI-Sentinel-Chat-Requirements-Token": {requirementsToken},
	}
	if proofToken := requestSetting(req, "chatgpt_proof_token", "openai_sentinel_proof_token", "sentinel_proof_token"); proofToken != "" {
		headers.Set("OpenAI-Sentinel-Proof-Token", proofToken)
	}
	if turnstileToken := requestSetting(req, "chatgpt_turnstile_token", "openai_sentinel_turnstile_token", "sentinel_turnstile_token"); turnstileToken != "" {
		headers.Set("OpenAI-Sentinel-Turnstile-Token", turnstileToken)
	}
	if soToken := requestSetting(req, "chatgpt_so_token", "openai_sentinel_so_token", "sentinel_so_token"); soToken != "" {
		headers.Set("OpenAI-Sentinel-SO-Token", soToken)
	}
	return headers, nil
}

func parseChatGPTWebConversationBody(body []byte, statusCode int) (contract.TextResponse, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	if !bytes.Contains(trimmed, []byte("data:")) {
		text, err := chatGPTWebTextFromJSON(trimmed)
		if err != nil {
			return contract.TextResponse{}, err
		}
		return contract.TextResponse{Text: text, StatusCode: statusCode, Usage: estimatedUsage(text)}, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var deltas strings.Builder
	var latestFullText string
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
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
		}
		if delta := chatGPTWebDelta(event); delta != "" {
			deltas.WriteString(delta)
			continue
		}
		if fullText := chatGPTWebAssistantText(event); fullText != "" {
			latestFullText = fullText
		}
	}
	if err := scanner.Err(); err != nil {
		return contract.TextResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	text := strings.TrimSpace(latestFullText)
	if text == "" {
		text = strings.TrimSpace(deltas.String())
	}
	if text == "" {
		return contract.TextResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	return contract.TextResponse{Text: text, StatusCode: statusCode, Usage: estimatedUsage(text)}, nil
}

func chatGPTWebTextFromJSON(body []byte) (string, error) {
	var event map[string]any
	if err := json.Unmarshal(body, &event); err != nil {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	text := strings.TrimSpace(chatGPTWebDelta(event))
	if text == "" {
		text = strings.TrimSpace(chatGPTWebAssistantText(event))
	}
	if text == "" {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	return text, nil
}

func chatGPTWebDelta(event map[string]any) string {
	if strings.EqualFold(mapStringAny(event, "type"), "conversation.delta") {
		return mapStringAny(event, "delta")
	}
	return ""
}

func chatGPTWebAssistantText(event map[string]any) string {
	message, ok := event["message"].(map[string]any)
	if !ok {
		return chatGPTWebContentText(event)
	}
	if author, ok := message["author"].(map[string]any); ok {
		role := strings.ToLower(strings.TrimSpace(mapStringAny(author, "role")))
		if role != "" && role != "assistant" {
			return ""
		}
	}
	return chatGPTWebContentText(message)
}

func chatGPTWebContentText(value map[string]any) string {
	if text := mapStringAny(value, "text"); text != "" {
		return text
	}
	rawContent, ok := value["content"]
	if !ok || rawContent == nil {
		return ""
	}
	if contentText, ok := rawContent.(string); ok {
		return strings.TrimSpace(contentText)
	}
	content, ok := rawContent.(map[string]any)
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case string:
			if typed != "" {
				out = append(out, typed)
			}
		case map[string]any:
			if text := mapStringAny(typed, "text"); text != "" {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "")
}

func isChatGPTWebReverseProxy(req contract.TextRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-chatgpt-web")
}

func chatGPTWebRuntimeIsAPIKey(req contract.TextRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func chatGPTWebReverseProxyAccount(req contract.TextRequest) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: req.Account.UpstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Credential:     req.Credential,
	}
}

func chatGPTWebPath(req contract.TextRequest) string {
	if path := requestSetting(req, "chatgpt_conversation_path", "conversation_path"); strings.HasPrefix(path, "/") {
		return path
	}
	if strings.EqualFold(requestSetting(req, "chatgpt_anon", "anonymous"), "true") {
		return chatGPTWebAnonConversationPath
	}
	return chatGPTWebConversationPath
}

func chatGPTWebConversationEndpoint(baseURL string, path string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	for _, suffix := range []string{
		"/v1/chat/completions",
		"/chat/completions",
		"/v1",
		chatGPTWebConversationPath,
		chatGPTWebAnonConversationPath,
		"/backend-api",
		"/backend-anon",
	} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimRight(strings.TrimSuffix(base, suffix), "/")
			break
		}
	}
	return base + path
}

func chatGPTWebOrigin(baseURL string) string {
	endpoint := chatGPTWebConversationEndpoint(baseURL, chatGPTWebConversationPath)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(baseURL, "/")
	}
	return parsed.Scheme + "://" + parsed.Host
}

func chatGPTWebClientContextInfo(req contract.TextRequest) chatGPTWebClientContext {
	return chatGPTWebClientContext{
		IsDarkMode:      false,
		TimeSinceLoaded: chatGPTWebIntSetting(req, 120, "chatgpt_time_since_loaded", "time_since_loaded"),
		PageHeight:      chatGPTWebIntSetting(req, 900, "chatgpt_page_height", "page_height"),
		PageWidth:       chatGPTWebIntSetting(req, 1400, "chatgpt_page_width", "page_width"),
		PixelRatio:      chatGPTWebFloatSetting(req, 2, "chatgpt_pixel_ratio", "pixel_ratio"),
		ScreenHeight:    chatGPTWebIntSetting(req, 1440, "chatgpt_screen_height", "screen_height"),
		ScreenWidth:     chatGPTWebIntSetting(req, 2560, "chatgpt_screen_width", "screen_width"),
	}
}

func chatGPTWebRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "developer":
		return "system"
	default:
		return "user"
	}
}

func chatGPTWebStringSetting(req contract.TextRequest, fallback string, keys ...string) string {
	if value := requestSetting(req, keys...); value != "" {
		return value
	}
	return fallback
}

func chatGPTWebSettingOrStableID(req contract.TextRequest, suffix string, keys ...string) string {
	if value := requestSetting(req, keys...); value != "" {
		return value
	}
	return chatGPTWebStableID(req, suffix)
}

func chatGPTWebStableID(req contract.TextRequest, suffix string) string {
	seed := strings.TrimSpace(req.RequestID)
	if seed == "" {
		seed = strings.TrimSpace(req.Model)
	}
	sum := sha256.Sum256([]byte(seed + ":" + suffix))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func chatGPTWebIntSetting(req contract.TextRequest, fallback int, keys ...string) int {
	if value := requestSetting(req, keys...); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func chatGPTWebFloatSetting(req contract.TextRequest, fallback float64, keys ...string) float64 {
	if value := requestSetting(req, keys...); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func mapStringAny(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
