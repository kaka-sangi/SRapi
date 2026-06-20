package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
	_ "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator/translators"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/httputil"
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

	// Wiring #3: per-account image-generation concurrency cap. chatgpt2api
	// gates this in account_service.get_available_access_token; we apply
	// the same cap here whenever the request is flagged as image-gen.
	slotKey := ""
	if chatGPTWebRequestIsImageGeneration(req) {
		slotKey = chatGPTWebImageSlotKey(req.Account.ID, credentialString(req.Credential, "access_token"))
		if slotKey != "" {
			cap := chatGPTWebAccountConcurrencyCap(req)
			if err := chatGPTWebImageSlotLimiter().Acquire(ctx, slotKey, cap); err != nil {
				return contract.ConversationResponse{}, contract.ProviderError{Class: "rate_limited", StatusCode: http.StatusTooManyRequests, Message: "chatgpt web image account at concurrency cap"}
			}
			defer chatGPTWebImageSlotLimiter().Release(slotKey)
		}
	}

	path := chatGPTWebPath(req)
	endpoint := chatGPTWebConversationEndpoint(baseURL, path)
	proxyURL := chatGPTWebProxyURLForRequest(req)

	headers, err := s.chatGPTWebConversationHeaders(ctx, req, baseURL, path)
	if err != nil {
		return contract.ConversationResponse{}, err
	}

	// Wiring #2: upload any binary InputPart and replace it with the
	// asset_pointer; build a multimodal payload when uploads succeed.
	rawPayload, err := s.chatGPTWebBuildConversationBody(ctx, req, baseURL, headers)
	if err != nil {
		return contract.ConversationResponse{}, err
	}

	// Wiring #1: inject any cached CF clearance cookies on the outbound
	// request. A miss is silent; the request proceeds normally and we'll
	// detect a challenge on the response side.
	account := chatGPTWebReverseProxyAccount(req)
	applyClearanceHeaders(headers, &account, endpoint, proxyURL)

	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account:      account,
		Method:       http.MethodPost,
		URL:          endpoint,
		Headers:      headers,
		Body:         rawPayload,
		ExpectStream: true,
	})

	// Wiring #1 (continued): if the upstream returned a CF challenge,
	// invalidate the cache, resolve a new bundle via the configured
	// provider (FlareSolverr), and retry once. Matches chatgpt2api's
	// reset_session_status_codes={403} policy. The reverse-proxy runtime
	// returns the failure as a RuntimeError that carries the body in the
	// message; we look there too because the response headers were
	// consumed.
	if challenge, statusCode, errBody := chatGPTWebDetectChallenge(runtimeResp, err); challenge {
		host := httputil.HostFromURL(endpoint)
		chatGPTWebClearanceCache().Invalidate(host, proxyURL)
		ok, resolveErr := resolveAndCacheClearance(ctx, endpoint, proxyURL)
		if ok {
			applyClearanceHeaders(headers, &account, endpoint, proxyURL)
			runtimeResp, err = s.reverseProxy.Do(ctx, reverseproxycontract.Request{
				Account:      account,
				Method:       http.MethodPost,
				URL:          endpoint,
				Headers:      headers,
				Body:         rawPayload,
				ExpectStream: true,
			})
		} else if resolveErr != nil {
			// Provider unconfigured: surface a clear configuration error to
			// the operator so they know to either disable the upstream or
			// stand up a FlareSolverr container.
			_ = statusCode
			return contract.ConversationResponse{}, contract.ProviderError{
				Class:      "challenge_required",
				StatusCode: http.StatusForbidden,
				Message:    "chatgpt web cloudflare challenge; configure FLARESOLVERR_URL to auto-resolve: " + resolveErr.Error() + httputil.FormatCloudflareChallengeMessage("", nil, errBody),
			}
		}
	}
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}

	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}

	// Wiring #4: record SSE / WS fallback metrics + surface a debug
	// header so ops can grep for fallback occurrences.
	outcome := ChatGPTWebWSFallbackInspect(runtimeResp.Body)
	parsed, err := parseChatGPTWebConversationBody(runtimeResp.Body, runtimeResp.StatusCode)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	resp := withConversationResponseHeaders(parsed, runtimeResp.Headers)
	if hv := chatGPTWebWSFallbackHeaderValue(outcome); hv != "" {
		if resp.Headers == nil {
			resp.Headers = http.Header{}
		}
		resp.Headers.Set(ChatGPTWebWSFallbackResponseHeader, hv)
	}
	return resp, nil
}

// chatGPTWebDetectChallenge inspects both a successful runtime response
// (rare for CF) and the error returned by Do when classifyRuntimeError
// consumed the body. Returns (true, status, body) when a CF challenge is
// indicated. chatgpt2api uses reset_session_status_codes=(403,) so we
// match 403 and 429 (CF's two challenge codes).
func chatGPTWebDetectChallenge(resp reverseproxycontract.Response, err error) (bool, int, []byte) {
	if err != nil {
		var rerr reverseproxycontract.RuntimeError
		if errors.As(err, &rerr) {
			body := []byte(rerr.Message)
			if httputil.IsCloudflareChallengeResponse(rerr.StatusCode, nil, body) {
				return true, rerr.StatusCode, body
			}
		}
		return false, 0, nil
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if httputil.IsCloudflareChallengeResponse(resp.StatusCode, resp.Headers, resp.Body) {
			return true, resp.StatusCode, resp.Body
		}
	}
	return false, 0, nil
}

// chatGPTWebBuildConversationBody is the payload builder that consults the
// file-upload helper (Wiring #2) for any binary InputParts before falling
// back to the text-only payload.
//
// Stage-2 translator registry wiring: route the synthesised bytes
// through the registered chatgpt_web → openai_responses translator
// (currently an identity — see
// translator/translators/chatgpt_web_to_openai_responses.go for the
// rationale; the inline shape helpers own canonical synthesis). The
// PR-3 transport-layer wiring (FlareSolverr clearance, file upload,
// image slot, WS fallback) stays in this file because none of it
// touches the payload bytes — it manipulates headers / cookies /
// concurrency slots which are not payload transforms.
func (s *Service) chatGPTWebBuildConversationBody(ctx context.Context, req contract.ConversationRequest, baseURL string, headers http.Header) ([]byte, error) {
	uploadedParts, attachments, err := s.chatGPTWebUploadInputParts(ctx, req, baseURL, headers)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if len(uploadedParts) > 0 {
		raw, err = chatGPTWebMultimodalPayloadBytes(req, uploadedParts, attachments)
	} else {
		raw, err = json.Marshal(chatGPTWebConversationPayload(req))
	}
	if err != nil {
		return nil, err
	}
	raw = translator.Default().TranslateRequest(
		translator.FormatChatGPTWeb,
		translator.FormatOpenAIResponses,
		req.Mapping.UpstreamModelName,
		raw,
		req.Stream,
	)
	return raw, nil
}

// chatGPTWebUploadInputParts walks req.InputParts and the parts of each
// req.Messages entry, uploads any image content with a non-empty
// MediaBase64 / MediaURL pointing at a data: URI, and returns the resulting
// multimodal parts + attachments arrays.
//
// Upload failures are fatal: a request that carries image parts should not
// silently degrade into a text-only prompt, because that hides broken file
// handling from the caller and changes the request semantics.
func (s *Service) chatGPTWebUploadInputParts(ctx context.Context, req contract.ConversationRequest, baseURL string, headers http.Header) ([]map[string]any, []map[string]any, error) {
	binaries := chatGPTWebCollectBinaryInputs(req)
	if len(binaries) == 0 {
		return nil, nil, nil
	}
	uploader := newChatGPTWebFileUploader(s.reverseProxy)
	sess := ChatGPTWebUploadSession{
		Account:   chatGPTWebReverseProxyAccount(req),
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Origin:    chatGPTWebOrigin(baseURL),
		UserAgent: chatGPTWebUserAgent(req),
		Headers:   headers,
	}
	parts := make([]map[string]any, 0, len(binaries))
	attachments := make([]map[string]any, 0, len(binaries))
	for _, bin := range binaries {
		asset, err := uploader.uploadImage(ctx, sess, bin.body, bin.mime, bin.name)
		if err != nil {
			return nil, nil, err
		}
		if asset == nil {
			return nil, nil, fmt.Errorf("chatgpt web uploader returned no asset for %s", bin.name)
		}
		if part := chatGPTWebAssetPointerPart(asset); part != nil {
			parts = append(parts, part)
		}
		if att := chatGPTWebAttachmentEntry(asset); att != nil {
			attachments = append(attachments, att)
		}
	}
	return parts, attachments, nil
}

type chatGPTWebBinaryInput struct {
	body []byte
	mime string
	name string
}

// chatGPTWebCollectBinaryInputs gathers the binary content parts we know
// how to upload. Only image parts with MediaBase64 are uploaded today —
// chatgpt2api supports data URIs + filesystem paths; the gateway only
// receives base64-bound traffic.
func chatGPTWebCollectBinaryInputs(req contract.ConversationRequest) []chatGPTWebBinaryInput {
	out := make([]chatGPTWebBinaryInput, 0)
	collect := func(parts []contract.ContentPart) {
		for _, part := range parts {
			if part.Kind != contract.ContentPartImage {
				continue
			}
			if data, mime, ok := decodeBase64MediaPart(part); ok {
				out = append(out, chatGPTWebBinaryInput{body: data, mime: mime, name: ""})
			}
		}
	}
	collect(req.InputParts)
	for _, msg := range req.Messages {
		collect(msg.Parts)
	}
	return out
}

// decodeBase64MediaPart pulls the bytes out of a base64-encoded
// MediaBase64 (or a data: URI in MediaURL). Returns ok=false when the part
// has no decodable bytes.
func decodeBase64MediaPart(part contract.ContentPart) ([]byte, string, bool) {
	mime := strings.TrimSpace(part.MIMEType)
	if data := strings.TrimSpace(part.MediaBase64); data != "" {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, "", false
		}
		return decoded, mime, true
	}
	if u := strings.TrimSpace(part.MediaURL); strings.HasPrefix(u, "data:") {
		idx := strings.Index(u, ",")
		if idx < 0 {
			return nil, "", false
		}
		header := u[5:idx]
		payload := u[idx+1:]
		if semi := strings.Index(header, ";"); semi >= 0 {
			mime = header[:semi]
		} else if header != "" {
			mime = header
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", false
		}
		return decoded, mime, true
	}
	return nil, "", false
}

// chatGPTWebMultimodalPayloadBytes constructs the chatgpt2api-shaped
// multimodal payload (parts = [<asset_pointer>..., "<prompt text>"]).
func chatGPTWebMultimodalPayloadBytes(req contract.ConversationRequest, assetParts []map[string]any, attachments []map[string]any) ([]byte, error) {
	base := chatGPTWebConversationPayload(req)
	prompt := chatGPTWebMultimodalPrompt(req)
	parts := make([]any, 0, len(assetParts)+1)
	for _, p := range assetParts {
		parts = append(parts, p)
	}
	parts = append(parts, prompt)
	content := map[string]any{
		"content_type": "multimodal_text",
		"parts":        parts,
	}
	metadata := map[string]any{
		"system_hints": []string{"picture_v2"},
		"serialization_metadata": map[string]any{
			"custom_symbol_offsets": []any{},
		},
	}
	if len(attachments) > 0 {
		metadata["attachments"] = attachments
	}
	userMessage := map[string]any{
		"id":       chatGPTWebStableID(req, "multimodal-user"),
		"author":   map[string]any{"role": "user"},
		"content":  content,
		"metadata": metadata,
	}
	// Marshal base then merge: keeps every chatgpt_web payload field we
	// already set (timezone, conversation_mode, etc.) and only swaps the
	// `messages` array for the multimodal user message.
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	payload["messages"] = []any{userMessage}
	return json.Marshal(payload)
}

func chatGPTWebMultimodalPrompt(req contract.ConversationRequest) string {
	prompt := conversationPrompt(req)
	if prompt == "" {
		prompt = strings.TrimSpace(req.Instructions)
	}
	return prompt
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
	headers := http.Header{
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
	if accountID := chatGPTWebAccountSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("chatgpt-account-id", accountID)
	}
	if originator := chatGPTWebAccountSetting(req, "originator", "chatgpt_originator"); originator != "" {
		headers.Set("originator", originator)
	}
	if version := chatGPTWebAccountSetting(req, "version", "chatgpt_version", "client_version"); version != "" {
		headers.Set("version", version)
	}
	return headers
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
	difficulty = strings.ToLower(strings.TrimSpace(difficulty))
	if difficulty == "" || len(difficulty) > 8 || !chatGPTWebPoWHexDifficulty(difficulty) {
		return chatGPTWebPoWFallback(seed), false
	}
	if limit <= 0 {
		return chatGPTWebPoWFallback(seed), false
	}
	start := time.Now()
	for nonce := 0; nonce < limit; nonce++ {
		answer, err := chatGPTWebPoWRunCheck(start, seed, difficulty, config, nonce)
		if err != nil {
			return chatGPTWebPoWFallback(seed), false
		}
		if answer != "" {
			return answer, true
		}
	}
	return chatGPTWebPoWFallback(seed), false
}

func chatGPTWebPoWRunCheck(start time.Time, seed string, difficulty string, config []any, nonce int) (string, error) {
	if len(config) < 10 {
		return "", nil
	}
	config[3] = nonce
	config[9] = int64(math.Round(float64(time.Since(start)) / float64(time.Millisecond)))
	encoded, err := chatGPTWebPoWEncodeFingerprint(config)
	if err != nil {
		return "", err
	}
	hash := chatGPTWebPoWFNV1aHex(seed + encoded)
	if len(hash) >= len(difficulty) && hash[:len(difficulty)] <= difficulty {
		return encoded + "~S", nil
	}
	return "", nil
}

func chatGPTWebPoWEncodeFingerprint(config []any) (string, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func chatGPTWebPoWHexDifficulty(difficulty string) bool {
	for _, ch := range difficulty {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func chatGPTWebPoWFNV1a32(value string) uint32 {
	const (
		offset uint32 = 2166136261
		prime  uint32 = 16777619
	)
	hash := offset
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= prime
	}
	hash ^= hash >> 16
	hash *= 2246822507
	hash ^= hash >> 13
	hash *= 3266489909
	hash ^= hash >> 16
	return hash
}

func chatGPTWebPoWFNV1aHex(value string) string {
	return fmt.Sprintf("%08x", chatGPTWebPoWFNV1a32(value))
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
	nowSeconds := float64(now.UnixNano()/int64(time.Millisecond)) / 1000
	return []any{
		"0",
		now.Format(time.RubyDate),
		"0",
		0,
		0.0,
		chatGPTWebUserAgent(req),
		scriptSource,
		dataBuild,
		"en-US",
		0,
		"en-US,es-US,en,es",
		0.0,
		"",
		"",
		nowSeconds,
		chatGPTWebStableID(req, fmt.Sprintf("pow-%d", now.UnixNano())),
		"",
		"Win32",
		nowSeconds,
		0,
		0,
		0,
		1,
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

func chatGPTWebAccountSetting(req contract.ConversationRequest, keys ...string) string {
	for _, values := range []map[string]any{req.RequestSettings, req.Credential, req.Account.Metadata, req.Provider.ConfigSchema} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
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
