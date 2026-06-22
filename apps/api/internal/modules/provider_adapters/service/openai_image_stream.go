package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) streamOpenAICompatibleImages(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := json.Marshal(openAIImageGenerationPayload(req))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/images/generations", bytes.NewReader(raw))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		return contract.ImageGenerationResponse{}, classifyTransportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if readErr != nil {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
		}
		return contract.ImageGenerationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	return openAICompatibleImageStreamResponse(resp.StatusCode, resp.Header, resp.Body, req, providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC()), nil)
}

func (s *Service) streamReverseProxyOpenAICompatibleImages(ctx context.Context, req contract.ImageGenerationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	streamer, ok := s.reverseProxy.(reverseproxycontract.StreamRuntime)
	if !ok {
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
	}
	raw, err := json.Marshal(openAIImageGenerationPayload(req))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	runtimeResp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      openAICompatibleImageReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(baseURL, "/") + "/images/generations",
		Headers:      openAICompatibleImageStreamHeaders("application/json"),
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	return openAICompatibleImageStreamResponse(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body, req, providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC()), nil)
}

func (s *Service) streamOpenAICompatibleImageEdit(ctx context.Context, req contract.ImageEditRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	body, contentType, err := openAIImageEditMultipart(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/images/edits", bytes.NewReader(body))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		return contract.ImageGenerationResponse{}, classifyTransportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		if readErr != nil {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
		}
		return contract.ImageGenerationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, raw)
	}
	imageReq := imageGenerationRequestFromEdit(req)
	return openAICompatibleImageStreamResponse(resp.StatusCode, resp.Header, resp.Body, imageReq, providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC()), func(parsed contract.ImageGenerationResponse) contract.ImageGenerationResponse {
		if parsed.Usage.Estimated {
			parsed.Usage = imageEditUsage(req)
		}
		return parsed
	})
}

func (s *Service) streamReverseProxyOpenAICompatibleImageEdit(ctx context.Context, req contract.ImageEditRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	streamer, ok := s.reverseProxy.(reverseproxycontract.StreamRuntime)
	if !ok {
		return contract.ImageGenerationResponse{}, contract.ErrStreamingUnsupported
	}
	body, contentType, err := openAIImageEditMultipart(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	imageReq := imageGenerationRequestFromEdit(req)
	runtimeResp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      openAICompatibleImageReverseProxyAccount(imageReq),
		Method:       http.MethodPost,
		URL:          strings.TrimRight(baseURL, "/") + "/images/edits",
		Headers:      openAICompatibleImageStreamHeaders(contentType),
		Body:         body,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	return openAICompatibleImageStreamResponse(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body, imageReq, providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC()), func(parsed contract.ImageGenerationResponse) contract.ImageGenerationResponse {
		if parsed.Usage.Estimated {
			parsed.Usage = imageEditUsage(req)
		}
		return parsed
	})
}

func openAICompatibleImageStreamHeaders(contentType string) http.Header {
	headers := http.Header{"Accept": {"text/event-stream"}}
	if strings.TrimSpace(contentType) != "" {
		headers.Set("Content-Type", contentType)
	}
	return headers
}

func openAICompatibleImageStreamReverseProxy(req contract.ImageGenerationRequest) bool {
	adapterType := strings.ToLower(strings.TrimSpace(req.Provider.AdapterType))
	protocol := strings.ToLower(strings.TrimSpace(req.Provider.Protocol))
	name := strings.ToLower(strings.TrimSpace(req.Provider.Name))
	switch {
	case adapterType == "":
		return protocol == "" || protocol == "openai-compatible"
	case strings.Contains(adapterType, "openai-compatible"):
		return true
	case adapterType == "generic-reverse-proxy" || adapterType == "reverse-proxy-openai-compatible":
		return true
	case name == "openai" || name == "openai-compatible":
		return true
	default:
		return false
	}
}

func openAICompatibleImageReverseProxyAccount(req contract.ImageGenerationRequest) reverseproxycontract.AccountRuntime {
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

func openAICompatibleImageStreamResponse(statusCode int, headers http.Header, upstream io.ReadCloser, req contract.ImageGenerationRequest, quotaSignals []contract.QuotaSignal, adjust func(contract.ImageGenerationResponse) contract.ImageGenerationResponse) (contract.ImageGenerationResponse, error) {
	if openAICompatibleImageStreamShouldBuffer(headers) {
		return openAICompatibleBufferedImageStreamResponse(statusCode, headers, upstream, req, quotaSignals, adjust)
	}
	return contract.ImageGenerationResponse{
		StatusCode:   statusCode,
		Headers:      cloneGenericHeaders(headers),
		QuotaSignals: quotaSignals,
		StreamBody:   newOpenAICompatibleImageStreamBody(upstream, req),
		StreamParse: func(body []byte, statusCode int) (contract.ImageGenerationResponse, error) {
			parsed, err := parseCodexImageGenerationRenderedStream(body, statusCode, req)
			if err != nil {
				return contract.ImageGenerationResponse{}, err
			}
			parsed.QuotaSignals = quotaSignals
			if adjust != nil {
				parsed = adjust(parsed)
			}
			return parsed, nil
		},
	}, nil
}

func openAICompatibleImageStreamShouldBuffer(headers http.Header) bool {
	contentType := strings.ToLower(strings.TrimSpace(headers.Get("Content-Type")))
	return strings.Contains(contentType, "application/json")
}

func openAICompatibleBufferedImageStreamResponse(statusCode int, headers http.Header, upstream io.ReadCloser, req contract.ImageGenerationRequest, quotaSignals []contract.QuotaSignal, adjust func(contract.ImageGenerationResponse) contract.ImageGenerationResponse) (contract.ImageGenerationResponse, error) {
	defer upstream.Close()
	raw, err := io.ReadAll(io.LimitReader(upstream, 16<<20))
	if err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	parsed, err := parseOpenAICompatibleImages(raw, statusCode, req.Mapping.UpstreamModelName, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(headers)
	parsed.QuotaSignals = quotaSignals
	if adjust != nil {
		parsed = adjust(parsed)
	}
	rendered, err := renderOpenAICompatibleBufferedImageStream(parsed, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	return contract.ImageGenerationResponse{
		StatusCode:   statusCode,
		Headers:      cloneGenericHeaders(headers),
		QuotaSignals: quotaSignals,
		StreamBody:   io.NopCloser(bytes.NewReader(rendered)),
		StreamParse: func(body []byte, statusCode int) (contract.ImageGenerationResponse, error) {
			streamParsed, err := parseCodexImageGenerationRenderedStream(body, statusCode, req)
			if err != nil {
				return contract.ImageGenerationResponse{}, err
			}
			streamParsed.QuotaSignals = quotaSignals
			if adjust != nil {
				streamParsed = adjust(streamParsed)
			}
			return streamParsed, nil
		},
	}, nil
}

func renderOpenAICompatibleBufferedImageStream(resp contract.ImageGenerationResponse, req contract.ImageGenerationRequest) ([]byte, error) {
	var out bytes.Buffer
	meta := codexImageGenerationStreamMetaFromRequest(req)
	if model := strings.TrimSpace(resp.Model); model != "" {
		meta.model = model
	}
	createdAt := resp.Created
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	for _, image := range resp.Data {
		payload := codexImageGenerationBaseStreamPayload("image_generation.completed", req, meta, createdAt)
		for key, value := range image.Metadata {
			if key != "" && value != nil {
				payload[key] = value
			}
		}
		codexImageGenerationSetImagePayload(payload, req, strings.TrimSpace(image.Base64JSON), mapString(image.Metadata, "output_format"))
		if value := strings.TrimSpace(image.URL); value != "" {
			payload["url"] = value
		}
		if value := strings.TrimSpace(image.RevisedPrompt); value != "" {
			payload["revised_prompt"] = value
		}
		if usage := imageGenerationUsagePayloadFromContract(resp.Usage); len(usage) > 0 {
			payload["usage"] = usage
		}
		if err := writeCodexImageGenerationFrame(&out, "image_generation.completed", payload); err != nil {
			return nil, err
		}
	}
	if err := writeOpenAICompatibleImageStreamDone(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func imageGenerationUsagePayloadFromContract(usage contract.Usage) map[string]any {
	if !usage.Observed && !usage.Estimated && usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CachedTokens == 0 && usage.ImageOutputTokens == 0 {
		return nil
	}
	payload := map[string]any{
		"input_tokens":  usage.InputTokens + usage.CachedTokens,
		"output_tokens": usage.OutputTokens,
		"total_tokens":  usage.InputTokens + usage.CachedTokens + usage.OutputTokens,
	}
	if usage.CachedTokens > 0 {
		payload["cached_tokens"] = usage.CachedTokens
		payload["input_tokens_details"] = map[string]any{"cached_tokens": usage.CachedTokens}
	}
	if usage.ImageOutputTokens > 0 {
		payload["output_tokens_details"] = map[string]any{"image_tokens": usage.ImageOutputTokens}
	}
	return payload
}

func newOpenAICompatibleImageStreamBody(upstream io.ReadCloser, req contract.ImageGenerationRequest) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		defer upstream.Close()
		err := transformOpenAICompatibleImageStream(upstream, writer, req)
		_ = writer.CloseWithError(err)
	}()
	return reader
}

func transformOpenAICompatibleImageStream(upstream io.Reader, downstream io.Writer, req contract.ImageGenerationRequest) error {
	scanner, releaseScanner := acquireSSEScanner(upstream, 52_428_800)
	defer releaseScanner()
	acc := newSSEFrameAccumulator()
	state := newOpenAICompatibleImageStreamState(req)
	for scanner.Scan() {
		frames := acc.AddLine(scanner.Text())
		for _, frame := range frames {
			done, err := state.HandleFrame(downstream, frame)
			if err != nil {
				return err
			}
			if done {
				return writeOpenAICompatibleImageStreamDone(downstream)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, frame := range acc.Flush() {
		done, err := state.HandleFrame(downstream, frame)
		if err != nil {
			return err
		}
		if done {
			return writeOpenAICompatibleImageStreamDone(downstream)
		}
	}
	return writeOpenAICompatibleImageStreamDone(downstream)
}

func writeOpenAICompatibleImageStreamDone(w io.Writer) error {
	_, err := io.WriteString(w, "data: [DONE]\n\n")
	return err
}

type openAICompatibleImageStreamState struct {
	req       contract.ImageGenerationRequest
	meta      codexImageGenerationStreamMeta
	createdAt int64
	pending   []codexResponsesOutputItem
}

func newOpenAICompatibleImageStreamState(req contract.ImageGenerationRequest) *openAICompatibleImageStreamState {
	return &openAICompatibleImageStreamState{
		req:       req,
		meta:      codexImageGenerationStreamMetaFromRequest(req),
		createdAt: time.Now().Unix(),
		pending:   make([]codexResponsesOutputItem, 0, 1),
	}
}

func (s *openAICompatibleImageStreamState) HandleFrame(w io.Writer, frame sseFrame) (bool, error) {
	data := strings.TrimSpace(frame.Data)
	if data == "" {
		return false, nil
	}
	if data == "[DONE]" {
		return true, nil
	}
	if providerErr, ok := providerErrorFromStreamFrame(frame, data, "openai-compatible"); ok {
		if err := writeCodexImageGenerationErrorFrame(w, providerErr); err != nil {
			return false, err
		}
		return true, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "openai-compatible image stream returned invalid json"}
	}
	eventType := strings.TrimSpace(frame.EventType(mapString(payload, "type")))
	switch eventType {
	case "response.created", "response.in_progress", "response.output_item.added":
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err == nil {
			s.mergeResponsesEvent(event)
		}
	case "response.image_generation_call.partial_image":
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "openai-compatible image stream returned invalid partial image event"}
		}
		s.mergeResponsesEvent(event)
		if strings.TrimSpace(event.PartialImage) == "" {
			return false, nil
		}
		return false, writeCodexImageGenerationFrame(w, "image_generation.partial_image", codexImageGenerationPartialPayload(s.req, event, s.meta, s.createdAt))
	case "image_generation.partial_image":
		return false, s.writeNativeImageFrame(w, "image_generation.partial_image", payload)
	case "response.output_item.done":
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "openai-compatible image stream returned invalid output item event"}
		}
		s.mergeResponsesEvent(event)
		if event.Item != nil && codexImageGenerationItemHasResult(*event.Item) {
			s.pending = append(s.pending, *event.Item)
		}
	case "response.completed":
		var event codexResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "openai-compatible image stream returned invalid completed event"}
		}
		s.mergeResponsesEvent(event)
		items := s.pending
		if event.Response != nil && len(event.Response.Output) > 0 {
			items = event.Response.Output
		}
		if len(items) == 0 {
			return false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "openai-compatible image stream contained no final images"}
		}
		for _, item := range items {
			if !codexImageGenerationItemHasResult(item) {
				continue
			}
			if err := writeCodexImageGenerationFrame(w, "image_generation.completed", codexImageGenerationCompletedPayload(s.req, item, s.meta, s.createdAt)); err != nil {
				return false, err
			}
		}
		return true, nil
	case "image_generation.completed", "image_generation.result":
		if err := s.writeNativeImageFrame(w, "image_generation.completed", payload); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *openAICompatibleImageStreamState) mergeResponsesEvent(event codexResponsesEvent) {
	codexMergeImageGenerationEventMeta(&s.meta, event)
	if event.Response == nil {
		return
	}
	if event.Response.Usage.HasTokenUsage() {
		s.meta.usage = event.Response.Usage
	}
	if created := codexImageGenerationCreatedAt(event.Response); created > 0 {
		s.createdAt = created
	}
}

func (s *openAICompatibleImageStreamState) writeNativeImageFrame(w io.Writer, eventName string, payload map[string]any) error {
	normalized := codexImageGenerationBaseStreamPayload(eventName, s.req, s.meta, s.createdAt)
	for key, value := range payload {
		if key == "type" || value == nil {
			continue
		}
		normalized[key] = value
	}
	outputFormat := firstNonEmpty(mapString(payload, "output_format"), s.meta.outputFormat)
	b64 := firstNonEmpty(mapString(payload, "b64_json"), firstNonEmpty(mapString(payload, "partial_image_b64"), mapString(payload, "result")))
	codexImageGenerationSetImagePayload(normalized, s.req, b64, outputFormat)
	if eventName == "image_generation.partial_image" {
		if _, ok := normalized["partial_image_index"]; !ok {
			normalized["partial_image_index"] = 0
		}
	}
	if value := strings.TrimSpace(mapString(payload, "revised_prompt")); value != "" {
		normalized["revised_prompt"] = value
	}
	if usageValue, ok := payload["usage"]; ok && usageValue != nil {
		normalized["usage"] = usageValue
	}
	return writeCodexImageGenerationFrame(w, eventName, normalized)
}

type sseFrameAccumulator struct {
	current   sseFrame
	dataLines []string
}

func newSSEFrameAccumulator() *sseFrameAccumulator {
	return &sseFrameAccumulator{}
}

func (a *sseFrameAccumulator) AddLine(line string) []sseFrame {
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return a.Flush()
	}
	if strings.HasPrefix(line, ":") {
		return nil
	}
	field, value, ok := strings.Cut(line, ":")
	if !ok {
		field = line
		value = ""
	}
	if strings.HasPrefix(value, " ") {
		value = strings.TrimPrefix(value, " ")
	}
	switch field {
	case "event":
		a.current.Event = value
	case "data":
		a.dataLines = append(a.dataLines, value)
	}
	return nil
}

func (a *sseFrameAccumulator) Flush() []sseFrame {
	if len(a.dataLines) == 0 {
		a.current = sseFrame{}
		return nil
	}
	a.current.Data = strings.Join(a.dataLines, "\n")
	frame := a.current
	a.current = sseFrame{}
	a.dataLines = nil
	return []sseFrame{frame}
}
