package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const (
	chatGPTWebImagePreparePath      = "/backend-api/f/conversation/prepare"
	chatGPTWebImageConversationPath = "/backend-api/f/conversation"
)

type chatGPTWebImageReference struct {
	FileID    string
	Sediment  bool
	Width     int
	Height    int
	SizeBytes int
}

func (s *Service) invokeReverseProxyChatGPTWebImageGeneration(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	if chatGPTWebImageRuntimeIsAPIKey(req) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "chatgpt web reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	slotKey := chatGPTWebImageSlotKey(req.Account.ID, credentialString(req.Credential, "access_token"))
	if slotKey != "" {
		cap := chatGPTWebImageAccountConcurrencyCap(req)
		if err := chatGPTWebImageSlotLimiter().Acquire(ctx, slotKey, cap); err != nil {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "rate_limited", StatusCode: http.StatusTooManyRequests, Message: "chatgpt web image account at concurrency cap"}
		}
		defer chatGPTWebImageSlotLimiter().Release(slotKey)
	}

	origin := chatGPTWebOrigin(baseURL)
	count := req.Count
	if count <= 0 {
		count = 1
	}
	images := make([]contract.Image, 0, count)
	for len(images) < count {
		requirements, err := s.chatGPTWebRequirements(ctx, chatGPTWebConversationRequestFromImage(req, nil), origin)
		if err != nil {
			return contract.ImageGenerationResponse{}, err
		}
		conduitToken, err := s.chatGPTWebPrepareImageConversation(ctx, req, origin, requirements)
		if err != nil {
			return contract.ImageGenerationResponse{}, err
		}
		batch, conversationID, err := s.chatGPTWebRunImageConversation(ctx, req, origin, requirements, conduitToken)
		if err != nil {
			return contract.ImageGenerationResponse{}, err
		}
		if len(batch) == 0 && conversationID != "" {
			batch, err = s.chatGPTWebImagesFromConversationDetail(ctx, req, origin, conversationID)
			if err != nil {
				return contract.ImageGenerationResponse{}, err
			}
		}
		if len(batch) == 0 {
			break
		}
		images = append(images, batch...)
	}
	if len(images) == 0 {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image response contained no images"}
	}
	if len(images) > count {
		images = images[:count]
	}
	return contract.ImageGenerationResponse{
		Created:    time.Now().Unix(),
		Data:       images,
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: http.StatusOK,
		Usage:      estimatedImageUsage(req),
	}, nil
}

func (s *Service) chatGPTWebPrepareImageConversation(ctx context.Context, req contract.ImageGenerationRequest, origin string, requirements chatGPTWebSentinelRequirements) (string, error) {
	body, err := json.Marshal(map[string]any{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     chatGPTWebStableID(chatGPTWebConversationRequestFromImage(req, nil), "image-prepare-parent"),
		"model":                 req.Mapping.UpstreamModelName,
		"client_prepare_state":  "success",
		"timezone_offset_min":   chatGPTWebImageIntSetting(req, chatGPTWebDefaultTimezoneOffsetMinutes, "chatgpt_timezone_offset_min", "timezone_offset_min"),
		"timezone":              chatGPTWebImageStringSetting(req, chatGPTWebDefaultTimezone, "chatgpt_timezone", "timezone"),
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"system_hints":          []string{"picture_v2"},
		"partial_query": map[string]any{
			"id":      chatGPTWebStableID(chatGPTWebConversationRequestFromImage(req, nil), "image-prepare-query"),
			"author":  map[string]string{"role": "user"},
			"content": map[string]any{"content_type": "text", "parts": []string{chatGPTWebImagePrompt(req)}},
		},
		"supports_buffering":     true,
		"supported_encodings":    []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
	})
	if err != nil {
		return "", err
	}
	resp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: chatGPTWebImageReverseProxyAccount(req),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(origin, "/") + chatGPTWebImagePreparePath,
		Headers: chatGPTWebImageJSONHeaders(req, origin, chatGPTWebImagePreparePath, requirements, ""),
		Body:    body,
	})
	if err != nil {
		return "", providerErrorFromReverseProxy(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Headers, resp.Body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image prepare returned invalid json"}
	}
	token := mapStringAny(decoded, "conduit_token")
	if token == "" {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image prepare returned no conduit token"}
	}
	return token, nil
}

func (s *Service) chatGPTWebRunImageConversation(ctx context.Context, req contract.ImageGenerationRequest, origin string, requirements chatGPTWebSentinelRequirements, conduitToken string) ([]contract.Image, string, error) {
	body, err := json.Marshal(chatGPTWebImageConversationPayload(req))
	if err != nil {
		return nil, "", err
	}
	resp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account:      chatGPTWebImageReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(origin, "/") + chatGPTWebImageConversationPath,
		Headers:      chatGPTWebImageJSONHeaders(req, origin, chatGPTWebImageConversationPath, requirements, conduitToken),
		Body:         body,
		ExpectStream: true,
	})
	if err != nil {
		return nil, "", providerErrorFromReverseProxy(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Headers, resp.Body)
	}
	conversationID, refs, err := chatGPTWebImageRefsFromSSE(resp.Body)
	if err != nil {
		return nil, "", err
	}
	images, err := s.chatGPTWebDownloadImages(ctx, req, origin, conversationID, refs)
	if err != nil {
		return nil, conversationID, err
	}
	return images, conversationID, nil
}

func chatGPTWebImageConversationPayload(req contract.ImageGenerationRequest) map[string]any {
	conversationReq := chatGPTWebConversationRequestFromImage(req, nil)
	return map[string]any{
		"action":                               "next",
		"messages":                             []any{chatGPTWebImageUserMessage(req)},
		"parent_message_id":                    chatGPTWebStableID(conversationReq, "image-parent"),
		"model":                                req.Mapping.UpstreamModelName,
		"client_prepare_state":                 "sent",
		"timezone_offset_min":                  chatGPTWebImageIntSetting(req, chatGPTWebDefaultTimezoneOffsetMinutes, "chatgpt_timezone_offset_min", "timezone_offset_min"),
		"timezone":                             chatGPTWebImageStringSetting(req, chatGPTWebDefaultTimezone, "chatgpt_timezone", "timezone"),
		"conversation_mode":                    map[string]string{"kind": "primary_assistant"},
		"enable_message_followups":             true,
		"system_hints":                         []string{"picture_v2"},
		"supports_buffering":                   true,
		"supported_encodings":                  []string{"v1"},
		"client_contextual_info":               chatGPTWebClientContextInfo(conversationReq),
		"force_parallel_switch":                "auto",
		"paragen_cot_summary_display_override": "allow",
	}
}

func chatGPTWebImageUserMessage(req contract.ImageGenerationRequest) map[string]any {
	return map[string]any{
		"id":          chatGPTWebStableID(chatGPTWebConversationRequestFromImage(req, nil), "image-user"),
		"author":      map[string]string{"role": "user"},
		"create_time": time.Now().Unix(),
		"content":     map[string]any{"content_type": "text", "parts": []string{chatGPTWebImagePrompt(req)}},
		"metadata": map[string]any{
			"developer_mode_connector_ids": []any{},
			"selected_github_repos":        []any{},
			"selected_all_github_repos":    false,
			"system_hints":                 []string{"picture_v2"},
			"serialization_metadata":       map[string]any{"custom_symbol_offsets": []any{}},
		},
	}
}

func chatGPTWebImageRefsFromSSE(body []byte) (string, []chatGPTWebImageReference, error) {
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimSpace(body)))
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	conversationID := ""
	refs := make([]chatGPTWebImageReference, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return "", nil, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image stream returned invalid json"}
		}
		chatGPTWebCollectImageRefs(event, &conversationID, &refs)
	}
	if err := scanner.Err(); err != nil {
		return "", nil, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "chatgpt web image stream interrupted"}
	}
	return conversationID, refs, nil
}

func (s *Service) chatGPTWebImagesFromConversationDetail(ctx context.Context, req contract.ImageGenerationRequest, origin string, conversationID string) ([]contract.Image, error) {
	path := "/backend-api/conversation/" + url.PathEscape(conversationID)
	resp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: chatGPTWebImageReverseProxyAccount(req),
		Method:  http.MethodGet,
		URL:     strings.TrimRight(origin, "/") + path,
		Headers: chatGPTWebImageJSONHeaders(req, origin, path, chatGPTWebSentinelRequirements{}, ""),
	})
	if err != nil {
		return nil, providerErrorFromReverseProxy(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Headers, resp.Body)
	}
	var payload any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image conversation returned invalid json"}
	}
	refs := make([]chatGPTWebImageReference, 0)
	chatGPTWebCollectImageRefs(payload, &conversationID, &refs)
	return s.chatGPTWebDownloadImages(ctx, req, origin, conversationID, refs)
}

func (s *Service) chatGPTWebDownloadImages(ctx context.Context, req contract.ImageGenerationRequest, origin string, conversationID string, refs []chatGPTWebImageReference) ([]contract.Image, error) {
	refs = chatGPTWebUniqueImageRefs(refs)
	if len(refs) == 0 {
		return nil, nil
	}
	if req.Count > 0 && len(refs) > req.Count {
		refs = refs[:req.Count]
	}
	images := make([]contract.Image, 0, len(refs))
	for _, ref := range refs {
		downloadURL, err := s.chatGPTWebImageDownloadURL(ctx, req, origin, conversationID, ref)
		if err != nil {
			return nil, err
		}
		image := contract.Image{
			URL:           "",
			Base64JSON:    "",
			RevisedPrompt: strings.TrimSpace(req.Prompt),
			Metadata: map[string]any{
				"conversation_id": conversationID,
				"file_id":         ref.FileID,
			},
		}
		if ref.Sediment {
			image.Metadata["source"] = "sediment"
		} else {
			image.Metadata["source"] = "file-service"
		}
		if strings.EqualFold(strings.TrimSpace(req.ResponseFormat), "url") {
			image.URL = downloadURL
		} else {
			body, err := s.chatGPTWebDownloadImageBytes(ctx, req, downloadURL)
			if err != nil {
				return nil, err
			}
			image.Base64JSON = base64.StdEncoding.EncodeToString(body)
		}
		images = append(images, image)
	}
	return images, nil
}

func (s *Service) chatGPTWebImageDownloadURL(ctx context.Context, req contract.ImageGenerationRequest, origin string, conversationID string, ref chatGPTWebImageReference) (string, error) {
	path := ""
	if ref.Sediment {
		if conversationID == "" {
			return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web sediment image missing conversation id"}
		}
		path = "/backend-api/conversation/" + url.PathEscape(conversationID) + "/attachment/" + url.PathEscape(ref.FileID) + "/download"
	} else {
		path = "/backend-api/files/" + url.PathEscape(ref.FileID) + "/download"
	}
	resp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: chatGPTWebImageReverseProxyAccount(req),
		Method:  http.MethodGet,
		URL:     strings.TrimRight(origin, "/") + path,
		Headers: chatGPTWebImageJSONHeaders(req, origin, path, chatGPTWebSentinelRequirements{}, ""),
	})
	if err != nil {
		return "", providerErrorFromReverseProxy(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Headers, resp.Body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image download metadata returned invalid json"}
	}
	downloadURL := mapStringAny(decoded, "download_url")
	if downloadURL == "" {
		downloadURL = mapStringAny(decoded, "url")
	}
	if downloadURL == "" {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image download metadata returned no url"}
	}
	return downloadURL, nil
}

func (s *Service) chatGPTWebDownloadImageBytes(ctx context.Context, req contract.ImageGenerationRequest, downloadURL string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	client := s.client
	if s.reverseProxy != nil {
		if managed, ok, err := s.reverseProxy.ManagedEgressClient(chatGPTWebImageReverseProxyAccount(req)); err == nil && ok && managed != nil {
			client = managed
		}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classifyTransportError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image download failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	if len(body) == 0 {
		return nil, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "chatgpt web image download returned empty body"}
	}
	return body, nil
}

func chatGPTWebCollectImageRefs(value any, conversationID *string, refs *[]chatGPTWebImageReference) {
	switch typed := value.(type) {
	case map[string]any:
		if conversationID != nil && *conversationID == "" {
			for _, key := range []string{"conversation_id", "conversationId"} {
				if id := mapStringAny(typed, key); id != "" {
					*conversationID = id
					break
				}
			}
		}
		if ref, ok := chatGPTWebImageRefFromMap(typed); ok {
			*refs = append(*refs, ref)
		}
		for _, child := range typed {
			chatGPTWebCollectImageRefs(child, conversationID, refs)
		}
	case []any:
		for _, child := range typed {
			chatGPTWebCollectImageRefs(child, conversationID, refs)
		}
	}
}

func chatGPTWebImageRefFromMap(value map[string]any) (chatGPTWebImageReference, bool) {
	pointer := mapStringAny(value, "asset_pointer")
	if pointer == "" {
		pointer = mapStringAny(value, "assetPointer")
	}
	if strings.HasPrefix(pointer, "file-service://") {
		id := strings.TrimSpace(strings.TrimPrefix(pointer, "file-service://"))
		if id != "" {
			return chatGPTWebImageReference{FileID: id, Width: chatGPTWebIntFromAny(value["width"]), Height: chatGPTWebIntFromAny(value["height"]), SizeBytes: chatGPTWebIntFromAny(value["size_bytes"])}, true
		}
	}
	if strings.HasPrefix(pointer, "sediment://") {
		id := strings.TrimSpace(strings.TrimPrefix(pointer, "sediment://"))
		if id != "" {
			return chatGPTWebImageReference{FileID: id, Sediment: true, Width: chatGPTWebIntFromAny(value["width"]), Height: chatGPTWebIntFromAny(value["height"]), SizeBytes: chatGPTWebIntFromAny(value["size_bytes"])}, true
		}
	}
	if id := mapStringAny(value, "file_id"); strings.HasPrefix(id, "file_") {
		return chatGPTWebImageReference{FileID: id, Width: chatGPTWebIntFromAny(value["width"]), Height: chatGPTWebIntFromAny(value["height"]), SizeBytes: chatGPTWebIntFromAny(value["size_bytes"])}, true
	}
	return chatGPTWebImageReference{}, false
}

func chatGPTWebUniqueImageRefs(refs []chatGPTWebImageReference) []chatGPTWebImageReference {
	out := make([]chatGPTWebImageReference, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		ref.FileID = strings.TrimSpace(ref.FileID)
		if ref.FileID == "" {
			continue
		}
		key := fmt.Sprintf("%t:%s", ref.Sediment, ref.FileID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func chatGPTWebImageJSONHeaders(req contract.ImageGenerationRequest, origin string, path string, requirements chatGPTWebSentinelRequirements, conduitToken string) http.Header {
	conversationReq := chatGPTWebConversationRequestFromImage(req, nil)
	headers := chatGPTWebBaseHeaders(conversationReq, origin, path)
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	if requirements.Token != "" {
		headers.Set("OpenAI-Sentinel-Chat-Requirements-Token", requirements.Token)
	}
	if requirements.ProofToken != "" {
		headers.Set("OpenAI-Sentinel-Proof-Token", requirements.ProofToken)
	}
	if requirements.TurnstileToken != "" {
		headers.Set("OpenAI-Sentinel-Turnstile-Token", requirements.TurnstileToken)
	}
	if requirements.SOToken != "" {
		headers.Set("OpenAI-Sentinel-SO-Token", requirements.SOToken)
	}
	if conduitToken != "" {
		headers.Set("X-Conduit-Token", conduitToken)
	}
	if path == chatGPTWebImageConversationPath {
		headers.Set("Accept", "text/event-stream")
		headers.Set("X-Oai-Turn-Trace-Id", chatGPTWebStableID(conversationReq, "image-turn-trace"))
	}
	return headers
}

func chatGPTWebConversationRequestFromImage(req contract.ImageGenerationRequest, settings map[string]any) contract.ConversationRequest {
	return contract.ConversationRequest{
		RequestID:       req.RequestID,
		SourceProtocol:  req.SourceProtocol,
		SourceEndpoint:  req.SourceEndpoint,
		Model:           req.Model,
		InputParts:      []contract.ContentPart{{Kind: contract.ContentPartText, Text: req.Prompt}},
		Provider:        req.Provider,
		Account:         req.Account,
		Mapping:         req.Mapping,
		Credential:      req.Credential,
		RequestSettings: chatGPTWebMergeMaps(req.RequestSettings, settings),
	}
}

func chatGPTWebImagePrompt(req contract.ImageGenerationRequest) string {
	prompt := strings.TrimSpace(req.Prompt)
	hints := make([]string, 0, 2)
	if size := strings.TrimSpace(req.Size); size != "" {
		hints = append(hints, "Output image size: "+size+".")
	}
	if quality := strings.TrimSpace(req.Quality); quality != "" {
		hints = append(hints, "Output image quality: "+quality+".")
	}
	if len(hints) == 0 {
		return prompt
	}
	return strings.TrimSpace(prompt + "\n\n" + strings.Join(hints, " "))
}

func chatGPTWebImageRuntimeIsAPIKey(req contract.ImageGenerationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func isChatGPTWebImageGenerationReverseProxy(req contract.ImageGenerationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-chatgpt-web")
}

func chatGPTWebImageReverseProxyAccount(req contract.ImageGenerationRequest) reverseproxycontract.AccountRuntime {
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

func chatGPTWebImageAccountConcurrencyCap(req contract.ImageGenerationRequest) int {
	for _, key := range []string{"chatgpt_image_account_concurrency", "image_account_concurrency"} {
		if v := mapString(req.Account.Metadata, key); v != "" {
			if n := parseIntOrZero(v); n > 0 {
				return n
			}
		}
	}
	return DefaultChatGPTWebImageAccountConcurrency
}

func chatGPTWebImageStringSetting(req contract.ImageGenerationRequest, fallback string, keys ...string) string {
	for _, values := range []map[string]any{req.RequestSettings, req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return fallback
}

func chatGPTWebImageIntSetting(req contract.ImageGenerationRequest, fallback int, keys ...string) int {
	value := chatGPTWebImageStringSetting(req, "", keys...)
	if value == "" {
		return fallback
	}
	parsed := parseIntOrZero(value)
	if parsed == 0 && value != "0" {
		return fallback
	}
	return parsed
}

func chatGPTWebMergeMaps(left map[string]any, right map[string]any) map[string]any {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	out := make(map[string]any, len(left)+len(right))
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}

func chatGPTWebIntFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	case string:
		return parseIntOrZero(typed)
	default:
		return 0
	}
}
