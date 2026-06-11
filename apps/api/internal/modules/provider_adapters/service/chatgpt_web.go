package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	"golang.org/x/crypto/sha3"
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
	chatGPTWebDefaultPoWScript             = "https://chatgpt.com/backend-api/sentinel/sdk.js"
	chatGPTWebPoWLimit                     = 500000
)

var (
	chatGPTWebScriptSrcRE = regexp.MustCompile(`<script[^>]+src=["']([^"']+)`)
	chatGPTWebDataBuildRE = regexp.MustCompile(`<html[^>]*data-build=["']([^"']*)`)
	chatGPTWebScriptBuild = regexp.MustCompile(`c/[^/]*/_`)
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

type chatGPTWebSentinelRequirements struct {
	Token          string
	ProofToken     string
	TurnstileToken string
	SOToken        string
}

type chatGPTWebRequirementsResponse struct {
	Token       string `json:"token"`
	SOToken     string `json:"so_token"`
	ProofOfWork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	Turnstile struct {
		Required bool   `json:"required"`
		DX       string `json:"dx"`
	} `json:"turnstile"`
	Arkose struct {
		Required bool `json:"required"`
	} `json:"arkose"`
}

func (s *Service) invokeReverseProxyChatGPTWebConversation(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if chatGPTWebRuntimeIsAPIKey(req) {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "chatgpt web reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	path := chatGPTWebPath(req)
	headers, err := s.chatGPTWebConversationHeaders(ctx, req, baseURL, path)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	raw, err := json.Marshal(chatGPTWebConversationPayload(req))
	if err != nil {
		return contract.ConversationResponse{}, err
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
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseChatGPTWebConversationBody(runtimeResp.Body, runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	return withConversationResponseHeaders(parsed, runtimeResp.Headers), nil
}

func chatGPTWebConversationPayload(req contract.ConversationRequest) chatGPTWebConversationRequest {
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

func chatGPTWebMessages(req contract.ConversationRequest) []chatGPTWebMessage {
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
		content := conversationMessageText(message)
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
		prompt := conversationPrompt(req)
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

func (s *Service) chatGPTWebConversationHeaders(ctx context.Context, req contract.ConversationRequest, baseURL string, path string) (http.Header, error) {
	requirements, err := s.chatGPTWebRequirements(ctx, req, baseURL)
	if err != nil {
		return nil, err
	}
	headers := chatGPTWebBaseHeaders(req, baseURL, path)
	headers.Set("Accept", "text/event-stream")
	headers.Set("Content-Type", "application/json")
	headers.Set("OpenAI-Sentinel-Chat-Requirements-Token", requirements.Token)
	if requirements.ProofToken != "" {
		headers.Set("OpenAI-Sentinel-Proof-Token", requirements.ProofToken)
	}
	if requirements.TurnstileToken != "" {
		headers.Set("OpenAI-Sentinel-Turnstile-Token", requirements.TurnstileToken)
	}
	if requirements.SOToken != "" {
		headers.Set("OpenAI-Sentinel-SO-Token", requirements.SOToken)
	}
	return headers, nil
}

func (s *Service) chatGPTWebRequirements(ctx context.Context, req contract.ConversationRequest, baseURL string) (chatGPTWebSentinelRequirements, error) {
	requirements := chatGPTWebSentinelRequirements{
		Token:          requestSetting(req, "chatgpt_requirements_token", "openai_sentinel_chat_requirements_token", "sentinel_chat_requirements_token"),
		ProofToken:     requestSetting(req, "chatgpt_proof_token", "openai_sentinel_proof_token", "sentinel_proof_token"),
		TurnstileToken: requestSetting(req, "chatgpt_turnstile_token", "openai_sentinel_turnstile_token", "sentinel_turnstile_token"),
		SOToken:        requestSetting(req, "chatgpt_so_token", "openai_sentinel_so_token", "sentinel_so_token"),
	}
	if requirements.Token != "" {
		return requirements, nil
	}
	if !chatGPTWebAutoRequirementsEnabled(req) {
		return chatGPTWebSentinelRequirements{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "chatgpt web requirements token missing"}
	}
	return s.fetchChatGPTWebRequirements(ctx, req, baseURL)
}

func (s *Service) fetchChatGPTWebRequirements(ctx context.Context, req contract.ConversationRequest, baseURL string) (chatGPTWebSentinelRequirements, error) {
	origin := chatGPTWebOrigin(baseURL)
	bootstrapResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: chatGPTWebReverseProxyAccount(req),
		Method:  http.MethodGet,
		URL:     strings.TrimRight(origin, "/") + "/",
		Headers: chatGPTWebBootstrapHeaders(req, origin),
	})
	if err != nil {
		return chatGPTWebSentinelRequirements{}, providerErrorFromReverseProxy(err)
	}
	powSources, dataBuild := chatGPTWebPoWResources(bootstrapResp.Body)
	legacyToken := requestSetting(req, "chatgpt_requirements_p", "chatgpt_legacy_requirements_token", "sentinel_requirements_p")
	if legacyToken == "" {
		legacyToken = chatGPTWebLegacyRequirementsToken(req, powSources, dataBuild)
	}
	raw, err := json.Marshal(map[string]string{"p": legacyToken})
	if err != nil {
		return chatGPTWebSentinelRequirements{}, err
	}
	path := chatGPTWebRequirementsPath(req)
	requirementsResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: chatGPTWebReverseProxyAccount(req),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(origin, "/") + path,
		Headers: chatGPTWebJSONHeaders(req, baseURL, path),
		Body:    raw,
	})
	if err != nil {
		return chatGPTWebSentinelRequirements{}, providerErrorFromReverseProxy(err)
	}
	var decoded chatGPTWebRequirementsResponse
	if err := json.Unmarshal(requirementsResp.Body, &decoded); err != nil {
		return chatGPTWebSentinelRequirements{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web requirements returned invalid json"}
	}
	return chatGPTWebBuildRequirements(req, decoded, legacyToken, powSources, dataBuild)
}

func chatGPTWebBuildRequirements(req contract.ConversationRequest, decoded chatGPTWebRequirementsResponse, legacyToken string, powSources []string, dataBuild string) (chatGPTWebSentinelRequirements, error) {
	requirements := chatGPTWebSentinelRequirements{
		Token:          strings.TrimSpace(decoded.Token),
		TurnstileToken: requestSetting(req, "chatgpt_turnstile_token", "openai_sentinel_turnstile_token", "sentinel_turnstile_token"),
		SOToken:        strings.TrimSpace(decoded.SOToken),
	}
	if requirements.Token == "" {
		return chatGPTWebSentinelRequirements{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web requirements response contained no token"}
	}
	if decoded.Arkose.Required {
		return chatGPTWebSentinelRequirements{}, contract.ProviderError{Class: "challenge_required", StatusCode: http.StatusForbidden, Message: "chatgpt web requirements require arkose challenge"}
	}
	if decoded.Turnstile.Required && requirements.TurnstileToken == "" {
		return chatGPTWebSentinelRequirements{}, contract.ProviderError{Class: "captcha_required", StatusCode: http.StatusForbidden, Message: "chatgpt web requirements require turnstile token"}
	}
	if decoded.ProofOfWork.Required {
		proofToken, err := chatGPTWebProofToken(req, decoded.ProofOfWork.Seed, decoded.ProofOfWork.Difficulty, powSources, dataBuild)
		if err != nil {
			return chatGPTWebSentinelRequirements{}, err
		}
		requirements.ProofToken = proofToken
	}
	_ = legacyToken
	return requirements, nil
}

func chatGPTWebBaseHeaders(req contract.ConversationRequest, baseURL string, path string) http.Header {
	origin := chatGPTWebOrigin(baseURL)
	return http.Header{
		"Accept":                      {"text/event-stream"},
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
	}
}

func chatGPTWebJSONHeaders(req contract.ConversationRequest, baseURL string, path string) http.Header {
	headers := chatGPTWebBaseHeaders(req, baseURL, path)
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	return headers
}

func chatGPTWebBootstrapHeaders(req contract.ConversationRequest, origin string) http.Header {
	return http.Header{
		"User-Agent":                {chatGPTWebUserAgent(req)},
		"Accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
		"Accept-Language":           {chatGPTWebStringSetting(req, chatGPTWebDefaultAcceptLanguage, "chatgpt_accept_language", "accept_language")},
		"Sec-Ch-Ua":                 {chatGPTWebStringSetting(req, `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`, "sec_ch_ua", "Sec-Ch-Ua")},
		"Sec-Ch-Ua-Mobile":          {chatGPTWebStringSetting(req, "?0", "sec_ch_ua_mobile", "Sec-Ch-Ua-Mobile")},
		"Sec-Ch-Ua-Platform":        {chatGPTWebStringSetting(req, `"Windows"`, "sec_ch_ua_platform", "Sec-Ch-Ua-Platform")},
		"Sec-Fetch-Dest":            {"document"},
		"Sec-Fetch-Mode":            {"navigate"},
		"Sec-Fetch-Site":            {"none"},
		"Sec-Fetch-User":            {"?1"},
		"Upgrade-Insecure-Requests": {"1"},
		"Origin":                    {origin},
		"Referer":                   {strings.TrimRight(origin, "/") + "/"},
	}
}

func chatGPTWebPoWResources(body []byte) ([]string, string) {
	text := string(body)
	sources := make([]string, 0, 4)
	for _, match := range chatGPTWebScriptSrcRE.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			source := strings.TrimSpace(match[1])
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		sources = append(sources, chatGPTWebDefaultPoWScript)
	}
	dataBuild := ""
	for _, source := range sources {
		if match := chatGPTWebScriptBuild.FindString(source); match != "" {
			dataBuild = match
			break
		}
	}
	if dataBuild == "" {
		if match := chatGPTWebDataBuildRE.FindStringSubmatch(text); len(match) > 1 {
			dataBuild = strings.TrimSpace(match[1])
		}
	}
	return sources, dataBuild
}

func chatGPTWebLegacyRequirementsToken(req contract.ConversationRequest, scriptSources []string, dataBuild string) string {
	seed := fmt.Sprintf("0.%d", time.Now().UnixNano())
	config := chatGPTWebPoWConfig(req, scriptSources, dataBuild)
	answer, _ := chatGPTWebPoWGenerate(seed, "0fffff", config, chatGPTWebPoWLimit)
	return "gAAAAAC" + answer
}

func chatGPTWebProofToken(req contract.ConversationRequest, seed string, difficulty string, scriptSources []string, dataBuild string) (string, error) {
	seed = strings.TrimSpace(seed)
	difficulty = strings.TrimSpace(difficulty)
	if seed == "" || difficulty == "" {
		return "", contract.ProviderError{Class: "challenge_required", StatusCode: http.StatusForbidden, Message: "chatgpt web proof token challenge missing seed or difficulty"}
	}
	config := chatGPTWebPoWConfig(req, scriptSources, dataBuild)
	answer, solved := chatGPTWebPoWGenerate(seed, difficulty, config, chatGPTWebPoWLimit)
	if !solved {
		return "", contract.ProviderError{Class: "challenge_required", StatusCode: http.StatusForbidden, Message: "chatgpt web proof token challenge not solved within budget"}
	}
	return "gAAAAAB" + answer, nil
}

func chatGPTWebPoWGenerate(seed string, difficulty string, config []any, limit int) (string, bool) {
	target, err := parseHexDifficulty(difficulty)
	if err != nil || len(target) == 0 {
		return chatGPTWebPoWFallback(seed), false
	}
	diffLen := len(target)
	seedBytes := []byte(seed)
	for i := range limit {
		config[3] = i
		config[9] = i >> 1
		raw, err := json.Marshal(config)
		if err != nil {
			return chatGPTWebPoWFallback(seed), false
		}
		encoded := base64.StdEncoding.EncodeToString(raw)
		digest := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytes.Compare(digest[:diffLen], target) <= 0 {
			return encoded, true
		}
	}
	return chatGPTWebPoWFallback(seed), false
}

func parseHexDifficulty(difficulty string) ([]byte, error) {
	if len(difficulty)%2 != 0 {
		difficulty = "0" + difficulty
	}
	out := make([]byte, len(difficulty)/2)
	for i := range out {
		parsed, err := strconv.ParseUint(difficulty[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out[i] = byte(parsed)
	}
	return out, nil
}

func chatGPTWebPoWFallback(seed string) string {
	raw, err := json.Marshal(seed)
	if err != nil {
		raw = []byte(strconv.Quote(seed))
	}
	return "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString(raw)
}

func chatGPTWebPoWConfig(req contract.ConversationRequest, scriptSources []string, dataBuild string) []any {
	scriptSource := chatGPTWebDefaultPoWScript
	if len(scriptSources) > 0 && strings.TrimSpace(scriptSources[0]) != "" {
		scriptSource = strings.TrimSpace(scriptSources[0])
	}
	now := time.Now()
	return []any{
		4000,
		now.UTC().Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		chatGPTWebUserAgent(req),
		scriptSource,
		dataBuild,
		"en-US",
		"en-US,es-US,en,es",
		0,
		"webdriver-false",
		"_reactListeningo743lnnpvdg",
		"window",
		float64(now.UnixNano()/int64(time.Millisecond)) / 1000,
		chatGPTWebStableID(req, fmt.Sprintf("pow-%d", now.UnixNano())),
		"",
		32,
		float64(now.UnixNano()/int64(time.Millisecond)) / 1000,
	}
}

func chatGPTWebAutoRequirementsEnabled(req contract.ConversationRequest) bool {
	value := strings.ToLower(requestSetting(req, "chatgpt_requirements_auto", "requirements_auto"))
	return value != "false" && value != "0" && value != "disabled"
}

func chatGPTWebRequirementsPath(req contract.ConversationRequest) string {
	if strings.EqualFold(requestSetting(req, "chatgpt_anon", "anonymous"), "true") {
		return "/backend-anon/sentinel/chat-requirements"
	}
	if path := requestSetting(req, "chatgpt_requirements_path", "requirements_path"); strings.HasPrefix(path, "/") {
		return path
	}
	return "/backend-api/sentinel/chat-requirements"
}

func chatGPTWebUserAgent(req contract.ConversationRequest) string {
	if value := requestSetting(req, "user_agent", "chatgpt_user_agent"); value != "" {
		return value
	}
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
}

func parseChatGPTWebConversationBody(body []byte, statusCode int) (contract.ConversationResponse, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no text"}
	}
	if !bytes.Contains(trimmed, []byte("data:")) {
		text, err := chatGPTWebTextFromJSON(trimmed)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		return conversationTextResponse(text, statusCode, estimatedUsage(text)), nil
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
			return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid stream json"}
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
		return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
	}
	text := strings.TrimSpace(latestFullText)
	if text == "" {
		text = strings.TrimSpace(deltas.String())
	}
	if text == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider stream contained no text"}
	}
	return conversationTextResponse(text, statusCode, estimatedUsage(text)), nil
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

func isChatGPTWebReverseProxy(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-chatgpt-web")
}

func chatGPTWebRuntimeIsAPIKey(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func chatGPTWebReverseProxyAccount(req contract.ConversationRequest) reverseproxycontract.AccountRuntime {
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

func chatGPTWebPath(req contract.ConversationRequest) string {
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

func chatGPTWebClientContextInfo(req contract.ConversationRequest) chatGPTWebClientContext {
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

func chatGPTWebStringSetting(req contract.ConversationRequest, fallback string, keys ...string) string {
	if value := requestSetting(req, keys...); value != "" {
		return value
	}
	return fallback
}

func chatGPTWebSettingOrStableID(req contract.ConversationRequest, suffix string, keys ...string) string {
	if value := requestSetting(req, keys...); value != "" {
		return value
	}
	return chatGPTWebStableID(req, suffix)
}

func chatGPTWebStableID(req contract.ConversationRequest, suffix string) string {
	seed := strings.TrimSpace(req.RequestID)
	if seed == "" {
		seed = strings.TrimSpace(req.Model)
	}
	sum := sha256.Sum256([]byte(seed + ":" + suffix))
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func chatGPTWebIntSetting(req contract.ConversationRequest, fallback int, keys ...string) int {
	if value := requestSetting(req, keys...); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func chatGPTWebFloatSetting(req contract.ConversationRequest, fallback float64, keys ...string) float64 {
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
