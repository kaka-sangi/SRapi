package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	headers := codexImageGenerationHeaders(req, payload)
	raw, outboundState := codexApplyOutboundWiring(req.Account, headers, raw)
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: codexImageGenerationReverseProxyAccount(req),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(baseURL, "/") + "/responses",
		Headers: headers,
		Body:    raw,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyCodexProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	body := codexCaptureInboundWiring(outboundState, codexReasoningReplayScope{}, runtimeResp.Body)
	parsed, err := parseCodexImageGenerationResponse(body, runtimeResp.StatusCode, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyCodexImageEdit(ctx context.Context, req contract.ImageEditRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	imageReq := imageGenerationRequestFromEdit(req)
	if codexImageGenerationRuntimeIsAPIKey(imageReq) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	payload, err := codexImageEditResponsesPayload(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	headers := codexImageGenerationHeaders(imageReq, payload)
	raw, outboundState := codexApplyOutboundWiring(req.Account, headers, raw)
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: codexImageGenerationReverseProxyAccount(imageReq),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(baseURL, "/") + "/responses",
		Headers: headers,
		Body:    raw,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyCodexProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	body := codexCaptureInboundWiring(outboundState, codexReasoningReplayScope{}, runtimeResp.Body)
	parsed, err := parseCodexImageGenerationResponse(body, runtimeResp.StatusCode, imageReq)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	if parsed.Usage.Estimated {
		parsed.Usage = imageEditUsage(req)
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyCodexImageVariation(ctx context.Context, req contract.ImageVariationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	imageReq := imageGenerationRequestFromVariation(req)
	if codexImageGenerationRuntimeIsAPIKey(imageReq) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	payload, err := codexImageVariationResponsesPayload(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	headers := codexImageGenerationHeaders(imageReq, payload)
	raw, outboundState := codexApplyOutboundWiring(req.Account, headers, raw)
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: codexImageGenerationReverseProxyAccount(imageReq),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(baseURL, "/") + "/responses",
		Headers: headers,
		Body:    raw,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyCodexProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	body := codexCaptureInboundWiring(outboundState, codexReasoningReplayScope{}, runtimeResp.Body)
	parsed, err := parseCodexImageGenerationResponse(body, runtimeResp.StatusCode, imageReq)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	if parsed.Usage.Estimated {
		parsed.Usage = imageVariationUsage(req)
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func (s *Service) streamReverseProxyCodexImageEdit(ctx context.Context, req contract.ImageEditRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	streamer, ok := s.reverseProxy.(reverseproxycontract.StreamRuntime)
	if !ok {
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
	}
	imageReq := imageGenerationRequestFromEdit(req)
	if codexImageGenerationRuntimeIsAPIKey(imageReq) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	payload, err := codexImageEditResponsesPayload(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	headers := codexImageGenerationHeaders(imageReq, payload)
	raw, outboundState := codexApplyOutboundWiring(req.Account, headers, raw)
	runtimeResp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      codexImageGenerationReverseProxyAccount(imageReq),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(baseURL, "/") + "/responses",
		Headers:      headers,
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	return contract.ImageGenerationResponse{
		StatusCode:   runtimeResp.StatusCode,
		Headers:      runtimeResp.Headers,
		QuotaSignals: codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC()),
		StreamBody:   newCodexImageGenerationStreamBody(codexExposeStreamBody(runtimeResp.Body, outboundState, nil), imageReq),
		StreamParse: func(body []byte, statusCode int) (contract.ImageGenerationResponse, error) {
			parsed, err := parseCodexImageGenerationRenderedStream(body, statusCode, imageReq)
			if err != nil {
				return contract.ImageGenerationResponse{}, err
			}
			if parsed.Usage.Estimated {
				parsed.Usage = imageEditUsage(req)
			}
			parsed.QuotaSignals = codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
			return parsed, nil
		},
	}, nil
}

func (s *Service) StreamImageGeneration(ctx context.Context, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	if !req.Stream {
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || strings.TrimSpace(req.Prompt) == "" {
		return contract.ImageGenerationResponse{}, ErrInvalidInput
	}
	baseURL := upstreamBaseURLImages(req)
	if baseURL == "" {
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
	}
	if isReverseProxyImageRuntime(req) {
		if isCodexImageGenerationReverseProxy(req) {
			return s.streamReverseProxyCodexImageGeneration(ctx, req, baseURL)
		}
		if openAICompatibleImageStreamReverseProxy(req) {
			return s.streamReverseProxyOpenAICompatibleImages(ctx, req, baseURL)
		}
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
	}
	return s.streamOpenAICompatibleImages(ctx, req, baseURL)
}

func (s *Service) streamReverseProxyCodexImageGeneration(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	streamer, ok := s.reverseProxy.(reverseproxycontract.StreamRuntime)
	if !ok {
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
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
	headers := codexImageGenerationHeaders(req, payload)
	raw, outboundState := codexApplyOutboundWiring(req.Account, headers, raw)
	runtimeResp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      codexImageGenerationReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(baseURL, "/") + "/responses",
		Headers:      headers,
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	return contract.ImageGenerationResponse{
		StatusCode:   runtimeResp.StatusCode,
		Headers:      runtimeResp.Headers,
		QuotaSignals: codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC()),
		StreamBody:   newCodexImageGenerationStreamBody(codexExposeStreamBody(runtimeResp.Body, outboundState, nil), req),
		StreamParse: func(body []byte, statusCode int) (contract.ImageGenerationResponse, error) {
			parsed, err := parseCodexImageGenerationRenderedStream(body, statusCode, req)
			if err != nil {
				return contract.ImageGenerationResponse{}, err
			}
			parsed.QuotaSignals = codexQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
			return parsed, nil
		},
	}, nil
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
	codexApplyImageGenerationSessionSettings(req, payload)
	codexEnsureReasoningEncryptedInclude(payload)
	payload["store"] = codexResponsesDefaultInternalStoreValue
	return payload, nil
}

func codexImageEditResponsesPayload(req contract.ImageEditRequest) (map[string]any, error) {
	imageReq := imageGenerationRequestFromEdit(req)
	payload, err := codexImageGenerationResponsesPayload(imageReq)
	if err != nil {
		return nil, err
	}
	tools, _ := payload["tools"].([]any)
	if len(tools) == 0 {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex image edit tool missing"}
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex image edit tool invalid"}
	}
	tool["action"] = "edit"
	if value := strings.TrimSpace(codexStringValue(req.Extra["input_fidelity"])); value != "" {
		tool["input_fidelity"] = value
	}
	if req.Mask != nil {
		maskURL, err := codexImageInputDataURL(*req.Mask, "mask")
		if err != nil {
			return nil, err
		}
		tool["input_image_mask"] = map[string]any{"image_url": maskURL}
	}
	input, err := codexImageEditInput(req)
	if err != nil {
		return nil, err
	}
	payload["input"] = input
	return payload, nil
}

func codexImageVariationResponsesPayload(req contract.ImageVariationRequest) (map[string]any, error) {
	imageReq := imageGenerationRequestFromVariation(req)
	payload, err := codexImageGenerationResponsesPayload(imageReq)
	if err != nil {
		return nil, err
	}
	tools, _ := payload["tools"].([]any)
	if len(tools) == 0 {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex image variation tool missing"}
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex image variation tool invalid"}
	}
	tool["action"] = "edit"
	if value := strings.TrimSpace(codexStringValue(req.Extra["input_fidelity"])); value != "" {
		tool["input_fidelity"] = value
	}
	input, err := codexImageVariationInput(req)
	if err != nil {
		return nil, err
	}
	payload["input"] = input
	return payload, nil
}

func codexImageEditInput(req contract.ImageEditRequest) ([]any, error) {
	content := make([]any, 0, 1+len(req.Images))
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		content = append(content, map[string]any{
			"type": "input_text",
			"text": prompt,
		})
	}
	for idx, image := range req.Images {
		imageURL, err := codexImageInputDataURL(image, fmt.Sprintf("image %d", idx+1))
		if err != nil {
			return nil, err
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": imageURL,
		})
	}
	return []any{map[string]any{
		"type":    "message",
		"role":    "user",
		"content": content,
	}}, nil
}

func codexImageVariationInput(req contract.ImageVariationRequest) ([]any, error) {
	imageURL, err := codexImageInputDataURL(req.Image, "image")
	if err != nil {
		return nil, err
	}
	return []any{map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type":      "input_image",
			"image_url": imageURL,
		}},
	}}, nil
}

func codexImageInputDataURL(image contract.ImageInput, label string) (string, error) {
	if len(image.Bytes) == 0 {
		return "", contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: strings.TrimSpace(label) + " is empty"}
	}
	contentType := strings.TrimSpace(image.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(image.Bytes), nil
}

func codexApplyImageGenerationSessionSettings(req contract.ImageGenerationRequest, payload map[string]any) {
	if payload == nil {
		return
	}
	if promptCacheKey := codexImageGenerationSetting(req, "codex_prompt_cache_key", "prompt_cache_key"); promptCacheKey != "" {
		payload["prompt_cache_key"] = promptCacheKey
	}
	codexApplyImageGenerationClientMetadataSettings(req, payload)
}

func codexApplyImageGenerationClientMetadataSettings(req contract.ImageGenerationRequest, payload map[string]any) {
	if payload == nil {
		return
	}
	metadata := codexPayloadClientMetadata(payload)
	setMetadata := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			metadata[key] = value
		}
	}
	setMetadata("x-codex-installation-id", codexImageGenerationSetting(req, "codex_installation_id", "x_codex_installation_id"))
	setMetadata("x-codex-turn-metadata", codexImageGenerationSetting(req, "codex_turn_metadata", "x_codex_turn_metadata", "X-Codex-Turn-Metadata"))
	setMetadata("x-codex-window-id", codexImageGenerationSetting(req, "codex_window_id", "x_codex_window_id", "X-Codex-Window-Id"))
	setMetadata("x-codex-beta-features", codexImageGenerationSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"))
	setMetadata("x-responsesapi-include-timing-metrics", codexImageGenerationSetting(req, "x_responsesapi_include_timing_metrics", "X-ResponsesAPI-Include-Timing-Metrics"))
	if len(metadata) > 0 {
		payload["client_metadata"] = metadata
	}
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

func newCodexImageGenerationStreamBody(upstream io.ReadCloser, req contract.ImageGenerationRequest) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		defer upstream.Close()
		err := transformCodexImageGenerationStream(upstream, writer, req)
		_ = writer.CloseWithError(err)
	}()
	return reader
}

func transformCodexImageGenerationStream(upstream io.Reader, downstream io.Writer, req contract.ImageGenerationRequest) error {
	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 0, 64*1024), 52_428_800)
	pending := make([]codexResponsesOutputItem, 0, 1)
	streamMeta := codexImageGenerationStreamMetaFromRequest(req)
	createdAt := time.Now().Unix()
	emittedDone := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "codex image stream returned invalid json"}
		}
		if providerErr, ok := codexEventProviderError(event); ok {
			if err := writeCodexImageGenerationErrorFrame(downstream, providerErr); err != nil {
				return err
			}
			emittedDone = true
			break
		}
		if providerErr, ok := codexImageGenerationIncompleteEventError(event); ok {
			if err := writeCodexImageGenerationErrorFrame(downstream, providerErr); err != nil {
				return err
			}
			emittedDone = true
			break
		}
		codexMergeImageGenerationEventMeta(&streamMeta, event)
		if event.Response != nil {
			if event.Response.Usage.HasTokenUsage() {
				streamMeta.usage = event.Response.Usage
			}
			if created := codexImageGenerationCreatedAt(event.Response); created > 0 {
				createdAt = created
			}
		}
		switch event.Type {
		case "response.image_generation_call.partial_image":
			if strings.TrimSpace(event.PartialImage) == "" {
				continue
			}
			if err := writeCodexImageGenerationFrame(downstream, "image_generation.partial_image", codexImageGenerationPartialPayload(req, event, streamMeta, createdAt)); err != nil {
				return err
			}
		case "response.output_item.done":
			if event.Item != nil && codexImageGenerationItemHasResult(*event.Item) {
				pending = append(pending, *event.Item)
			}
		case "response.completed":
			items := pending
			if event.Response != nil && len(event.Response.Output) > 0 {
				items = event.Response.Output
			}
			if len(items) == 0 {
				return contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "codex image stream contained no final images"}
			}
			for _, item := range items {
				if !codexImageGenerationItemHasResult(item) {
					continue
				}
				if err := writeCodexImageGenerationFrame(downstream, "image_generation.completed", codexImageGenerationCompletedPayload(req, item, streamMeta, createdAt)); err != nil {
					return err
				}
			}
			emittedDone = true
			break
		}
		if emittedDone {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if _, err := io.WriteString(downstream, "data: [DONE]\n\n"); err != nil {
		return err
	}
	return nil
}

type codexImageGenerationStreamMeta struct {
	model        string
	outputFormat string
	background   string
	quality      string
	size         string
	usage        openAIUsage
}

func codexImageGenerationStreamMetaFromRequest(req contract.ImageGenerationRequest) codexImageGenerationStreamMeta {
	return codexImageGenerationStreamMeta{
		model:        strings.TrimSpace(codexImageGenerationToolModel(req)),
		outputFormat: strings.TrimSpace(codexStringValue(req.Extra["output_format"])),
		background:   strings.TrimSpace(codexStringValue(req.Extra["background"])),
		quality:      strings.TrimSpace(req.Quality),
		size:         strings.TrimSpace(req.Size),
	}
}

func codexMergeImageGenerationEventMeta(meta *codexImageGenerationStreamMeta, event codexResponsesEvent) {
	if meta == nil {
		return
	}
	if value := strings.TrimSpace(event.OutputFormat); value != "" {
		meta.outputFormat = value
	}
	if value := strings.TrimSpace(event.Background); value != "" {
		meta.background = value
	}
	if event.Response == nil {
		return
	}
	if value := strings.TrimSpace(event.Response.Model); value != "" && meta.model == "" {
		meta.model = value
	}
	for _, tool := range event.Response.Tools {
		codexMergeImageGenerationToolMeta(meta, tool)
	}
	for _, item := range event.Response.Output {
		codexMergeImageGenerationItemMeta(meta, item)
	}
}

func codexMergeImageGenerationToolMeta(meta *codexImageGenerationStreamMeta, tool map[string]any) {
	if meta == nil || !strings.EqualFold(mapString(tool, "type"), "image_generation") {
		return
	}
	if value := mapString(tool, "model"); value != "" {
		meta.model = value
	}
	if value := mapString(tool, "output_format"); value != "" {
		meta.outputFormat = value
	}
	if value := mapString(tool, "background"); value != "" {
		meta.background = value
	}
	if value := mapString(tool, "quality"); value != "" {
		meta.quality = value
	}
	if value := mapString(tool, "size"); value != "" {
		meta.size = value
	}
}

func codexMergeImageGenerationItemMeta(meta *codexImageGenerationStreamMeta, item codexResponsesOutputItem) {
	if meta == nil {
		return
	}
	if value := strings.TrimSpace(item.OutputFormat); value != "" {
		meta.outputFormat = value
	}
}

func codexImageGenerationCreatedAt(response *codexResponsesResponse) int64 {
	if response == nil {
		return 0
	}
	if response.CreatedAt > 0 {
		return response.CreatedAt
	}
	return response.Created
}

func codexImageGenerationPartialPayload(req contract.ImageGenerationRequest, event codexResponsesEvent, meta codexImageGenerationStreamMeta, createdAt int64) map[string]any {
	payload := codexImageGenerationBaseStreamPayload("image_generation.partial_image", req, meta, createdAt)
	payload["partial_image_index"] = codexImageGenerationPartialIndex(event.PartialIndex)
	codexImageGenerationSetImagePayload(payload, req, strings.TrimSpace(event.PartialImage), meta.outputFormat)
	return payload
}

func codexImageGenerationCompletedPayload(req contract.ImageGenerationRequest, item codexResponsesOutputItem, meta codexImageGenerationStreamMeta, createdAt int64) map[string]any {
	codexMergeImageGenerationItemMeta(&meta, item)
	payload := codexImageGenerationBaseStreamPayload("image_generation.completed", req, meta, createdAt)
	codexImageGenerationSetImagePayload(payload, req, strings.TrimSpace(item.Result), firstNonEmpty(strings.TrimSpace(item.OutputFormat), meta.outputFormat))
	if value := strings.TrimSpace(item.RevisedPrompt); value != "" {
		payload["revised_prompt"] = value
	}
	if usage := codexImageGenerationUsagePayload(meta.usage); len(usage) > 0 {
		payload["usage"] = usage
	}
	return payload
}

func codexImageGenerationBaseStreamPayload(eventType string, req contract.ImageGenerationRequest, meta codexImageGenerationStreamMeta, createdAt int64) map[string]any {
	payload := map[string]any{"type": eventType}
	if createdAt > 0 {
		payload["created_at"] = createdAt
	}
	if value := strings.TrimSpace(meta.model); value != "" {
		payload["model"] = value
	}
	if value := strings.TrimSpace(meta.outputFormat); value != "" {
		payload["output_format"] = value
	}
	if value := strings.TrimSpace(meta.background); value != "" {
		payload["background"] = value
	}
	if value := strings.TrimSpace(meta.quality); value != "" {
		payload["quality"] = value
	}
	if value := strings.TrimSpace(meta.size); value != "" {
		payload["size"] = value
	}
	return payload
}

func codexImageGenerationSetImagePayload(payload map[string]any, req contract.ImageGenerationRequest, b64 string, outputFormat string) {
	if b64 == "" {
		return
	}
	payload["b64_json"] = b64
	if strings.EqualFold(strings.TrimSpace(req.ResponseFormat), "url") {
		payload["url"] = "data:" + codexImageGenerationMIMEType(outputFormat) + ";base64," + b64
	}
}

func codexImageGenerationUsagePayload(usage openAIUsage) map[string]any {
	payload := map[string]any{}
	if usage.InputTokens != nil {
		payload["input_tokens"] = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		payload["output_tokens"] = *usage.OutputTokens
	}
	if usage.PromptTokens != nil {
		payload["prompt_tokens"] = *usage.PromptTokens
	}
	if usage.CompletionTokens != nil {
		payload["completion_tokens"] = *usage.CompletionTokens
	}
	if usage.TotalTokens != nil {
		payload["total_tokens"] = *usage.TotalTokens
	}
	if usage.CachedTokens != nil {
		payload["cached_tokens"] = *usage.CachedTokens
	}
	if usage.InputTokensDetails != nil {
		payload["input_tokens_details"] = usage.InputTokensDetails
	}
	if usage.PromptTokensDetails != nil {
		payload["prompt_tokens_details"] = usage.PromptTokensDetails
	}
	if usage.OutputTokensDetails != nil {
		payload["output_tokens_details"] = usage.OutputTokensDetails
	}
	return payload
}

func codexImageGenerationIncompleteEventError(event codexResponsesEvent) (contract.ProviderError, bool) {
	if !strings.EqualFold(strings.TrimSpace(event.Type), "response.incomplete") {
		return contract.ProviderError{}, false
	}
	reason := ""
	if event.Response != nil && event.Response.IncompleteDetails != nil {
		reason = strings.TrimSpace(event.Response.IncompleteDetails.Reason)
	}
	message := "codex image stream incomplete"
	if reason != "" {
		message += ": " + reason
	}
	return contract.ProviderError{
		Class:      "incomplete",
		StatusCode: http.StatusBadGateway,
		Message:    message,
		Metadata: map[string]any{
			"type": "incomplete",
			"code": firstNonEmpty(reason, "incomplete"),
		},
	}, true
}

func codexImageGenerationPartialIndex(value any) int {
	if parsed, ok := codexImageGenerationIntValue(value); ok {
		return parsed
	}
	return 0
}

func codexImageGenerationItemHasResult(item codexResponsesOutputItem) bool {
	return strings.EqualFold(strings.TrimSpace(item.Type), "image_generation_call") && strings.TrimSpace(item.Result) != ""
}

func writeCodexImageGenerationFrame(w io.Writer, eventName string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, raw)
	return err
}

func writeCodexImageGenerationErrorFrame(w io.Writer, providerErr contract.ProviderError) error {
	errorType := strings.TrimSpace(providerErr.Class)
	errorCode := errorType
	if providerErr.Metadata != nil {
		if value := mapString(providerErr.Metadata, "type"); value != "" {
			errorType = value
		}
		if value := mapString(providerErr.Metadata, "code"); value != "" {
			errorCode = value
		}
	}
	payload := map[string]any{
		"type": "error",
		"error": map[string]any{
			"message": providerErr.Message,
			"type":    errorType,
			"code":    errorCode,
		},
	}
	if providerErr.StatusCode > 0 {
		payload["status_code"] = providerErr.StatusCode
	}
	return writeCodexImageGenerationFrame(w, "error", payload)
}

func parseCodexImageGenerationRenderedStream(body []byte, statusCode int, req contract.ImageGenerationRequest) (contract.ImageGenerationResponse, error) {
	frames, err := parseSSEFrames(body)
	if err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider image stream parse failed"}
	}
	images := make([]contract.Image, 0)
	var usage openAIUsage
	var created int64
	model := strings.TrimSpace(req.Mapping.UpstreamModelName)
	for _, frame := range frames {
		if strings.TrimSpace(frame.Data) == "[DONE]" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
			continue
		}
		eventType := frame.EventType(mapString(payload, "type"))
		if eventType == "error" {
			return contract.ImageGenerationResponse{}, codexImageGenerationStreamError(payload)
		}
		if eventType != "image_generation.completed" {
			continue
		}
		if created == 0 {
			if value, ok := codexImageGenerationIntValue(payload["created_at"]); ok {
				created = int64(value)
			}
		}
		if value := mapString(payload, "model"); value != "" {
			model = value
		}
		image := contract.Image{
			URL:           strings.TrimSpace(mapString(payload, "url")),
			Base64JSON:    strings.TrimSpace(mapString(payload, "b64_json")),
			RevisedPrompt: strings.TrimSpace(mapString(payload, "revised_prompt")),
			Metadata:      cloneMap(payload),
		}
		delete(image.Metadata, "type")
		delete(image.Metadata, "url")
		delete(image.Metadata, "b64_json")
		delete(image.Metadata, "revised_prompt")
		delete(image.Metadata, "usage")
		if image.URL != "" || image.Base64JSON != "" {
			images = append(images, image)
		}
		if usageValue, ok := payload["usage"]; ok {
			raw, _ := json.Marshal(usageValue)
			_ = json.Unmarshal(raw, &usage)
		}
	}
	if len(images) == 0 {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider image stream contained no final images"}
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	return contract.ImageGenerationResponse{
		Created:    created,
		Data:       images,
		Model:      model,
		StatusCode: statusCode,
		Usage:      usage.ToImageUsage(req),
	}, nil
}

func codexImageGenerationStreamError(payload map[string]any) contract.ProviderError {
	errPayload, _ := payload["error"].(map[string]any)
	message := mapString(errPayload, "message")
	if message == "" {
		message = mapString(payload, "message")
	}
	class := mapString(errPayload, "type")
	if class == "" {
		class = mapString(errPayload, "code")
	}
	if class == "" {
		class = "upstream_error"
	}
	statusCode := http.StatusBadGateway
	if value, ok := codexImageGenerationIntValue(payload["status_code"]); ok && value > 0 {
		statusCode = value
	}
	if message == "" {
		message = class
	}
	return contract.ProviderError{Class: class, StatusCode: statusCode, Message: message}
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
	if turnMetadata := codexImageGenerationSetting(req, "codex_turn_metadata", "x_codex_turn_metadata", "X-Codex-Turn-Metadata"); turnMetadata != "" {
		headers.Set("X-Codex-Turn-Metadata", turnMetadata)
	}
	windowID := codexImageGenerationSetting(req, "codex_window_id", "x_codex_window_id", "X-Codex-Window-Id")
	if windowID != "" {
		headers.Set("X-Codex-Window-Id", windowID)
	}
	if betaFeatures := codexImageGenerationSetting(req, "codex_beta_features", "x_codex_beta_features", "X-Codex-Beta-Features"); betaFeatures != "" {
		headers.Set("X-Codex-Beta-Features", betaFeatures)
	}
	if includeTiming := codexImageGenerationSetting(req, "x_responsesapi_include_timing_metrics", "X-ResponsesAPI-Include-Timing-Metrics"); includeTiming != "" {
		headers.Set("X-ResponsesAPI-Include-Timing-Metrics", includeTiming)
	}
	if sessionID := codexImageGenerationSetting(req, "codex_session_id", "session_id", "Session_id"); sessionID != "" {
		headers.Set("Session_id", sessionID)
	} else if req.Account.ID > 0 {
		headers.Set("Session_id", codexDefaultAccountSessionID(req.Account.ID))
	}
	codexApplySessionIdentityHeaders(headers, codexPayloadPromptCacheKey(payload), windowID)
	if al := codexImageGenerationSetting(req, "accept-language"); al != "" {
		headers.Set("Accept-Language", al)
	}
	return headers
}

func codexImageGenerationSetting(req contract.ImageGenerationRequest, keys ...string) string {
	for _, values := range []map[string]any{req.RequestSettings, req.Credential, req.Extra, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
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
		RequestID:       req.RequestID,
		Model:           req.Model,
		Provider:        req.Provider,
		Account:         req.Account,
		Mapping:         req.Mapping,
		Credential:      req.Credential,
		RequestSettings: req.RequestSettings,
	}
}
