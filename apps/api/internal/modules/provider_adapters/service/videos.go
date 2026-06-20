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
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const (
	xaiVideosGenerationsPath = "/videos/generations"
	defaultXAIVideoModel     = "grok-imagine-video"
	soraVideoModel           = "sora-2"
)

func (s *Service) InvokeVideo(ctx context.Context, req contract.VideoRequest) (contract.VideoResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.VideoResponse{}, ErrInvalidInput
	}
	baseURL := upstreamBaseURLVideos(req)
	if baseURL == "" {
		return contract.VideoResponse{}, errUpstreamBaseURLMissing("video")
	}
	switch req.Operation {
	case contract.VideoOperationCreate:
		if strings.TrimSpace(req.Prompt) == "" {
			return contract.VideoResponse{}, ErrInvalidInput
		}
		return s.invokeXAIVideoCreate(ctx, req, baseURL)
	case contract.VideoOperationRetrieve:
		if strings.TrimSpace(req.VideoID) == "" {
			return contract.VideoResponse{}, ErrInvalidInput
		}
		return s.invokeXAIVideoRetrieve(ctx, req, baseURL)
	default:
		return contract.VideoResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "unsupported video operation"}
	}
}

func (s *Service) InvokeVideoContent(ctx context.Context, req contract.VideoRequest) (contract.VideoContentResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.VideoID) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.VideoContentResponse{}, ErrInvalidInput
	}
	baseURL := upstreamBaseURLVideos(req)
	if baseURL == "" {
		return contract.VideoContentResponse{}, errUpstreamBaseURLMissing("video content")
	}
	video, err := s.invokeXAIVideoRetrieve(ctx, req, baseURL)
	if err != nil {
		return contract.VideoContentResponse{}, err
	}
	contentURL, err := validatedVideoContentURL(video.ContentURL)
	if err != nil {
		return contract.VideoContentResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return contract.VideoContentResponse{}, err
	}
	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.VideoContentResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.VideoContentResponse{}, classifyTransportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return contract.VideoContentResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
		}
		return contract.VideoContentResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return contract.VideoContentResponse{
		StatusCode:   resp.StatusCode,
		ContentType:  contentType,
		Content:      resp.Body,
		Headers:      cloneVideoContentHeaders(resp.Header),
		QuotaSignals: video.QuotaSignals,
	}, nil
}

func (s *Service) invokeXAIVideoCreate(ctx context.Context, req contract.VideoRequest, baseURL string) (contract.VideoResponse, error) {
	raw, err := json.Marshal(xaiVideoCreatePayload(req))
	if err != nil {
		return contract.VideoResponse{}, err
	}
	return s.doXAIVideoJSON(ctx, req, http.MethodPost, strings.TrimRight(baseURL, "/")+xaiVideosGenerationsPath, raw)
}

func (s *Service) invokeXAIVideoRetrieve(ctx context.Context, req contract.VideoRequest, baseURL string) (contract.VideoResponse, error) {
	videoID := url.PathEscape(strings.TrimSpace(req.VideoID))
	return s.doXAIVideoJSON(ctx, req, http.MethodGet, strings.TrimRight(baseURL, "/")+"/videos/"+videoID, nil)
}

func (s *Service) doXAIVideoJSON(ctx context.Context, req contract.VideoRequest, method string, endpoint string, body []byte) (contract.VideoResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.VideoResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return contract.VideoResponse{}, err
	}
	if len(body) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if idempotencyKey := videoRequestSetting(req, "idempotency_key"); idempotencyKey != "" {
		httpReq.Header.Set("x-idempotency-key", idempotencyKey)
	}
	s.applyAccountRequestHeaders(httpReq.Header, req.Account, req.Credential)

	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.VideoResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.VideoResponse{}, classifyTransportError(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return contract.VideoResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.VideoResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, respBody)
	}
	parsed, err := parseXAIVideoResponse(respBody, resp.StatusCode, req)
	if err != nil {
		return contract.VideoResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
}

func xaiVideoCreatePayload(req contract.VideoRequest) map[string]any {
	payload := cloneMap(req.Extra)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["model"] = xaiVideoModel(req.Mapping.UpstreamModelName)
	payload["prompt"] = strings.TrimSpace(req.Prompt)
	duration := req.Seconds
	if duration <= 0 {
		duration = 4
	}
	if duration > 15 {
		duration = 15
	}
	if len(req.ReferenceImages) > 0 && duration > 10 {
		duration = 10
	}
	payload["duration"] = duration
	if strings.TrimSpace(req.AspectRatio) != "" {
		payload["aspect_ratio"] = strings.TrimSpace(req.AspectRatio)
	}
	if strings.TrimSpace(req.Resolution) != "" {
		payload["resolution"] = strings.TrimSpace(req.Resolution)
	}
	applyXAIVideoSize(payload, req.Size)
	if input := strings.TrimSpace(req.InputReference); input != "" {
		payload["image"] = map[string]any{"url": input}
	}
	if len(req.ReferenceImages) > 0 {
		refs := make([]map[string]any, 0, len(req.ReferenceImages))
		for _, ref := range req.ReferenceImages {
			if ref = strings.TrimSpace(ref); ref != "" {
				refs = append(refs, map[string]any{"url": ref})
			}
		}
		if len(refs) > 0 {
			payload["reference_images"] = refs
		}
	}
	return payload
}

func applyXAIVideoSize(payload map[string]any, size string) {
	switch strings.TrimSpace(size) {
	case "1280x720":
		payload["aspect_ratio"] = "16:9"
		payload["resolution"] = "720p"
	case "720x1280":
		payload["aspect_ratio"] = "9:16"
		payload["resolution"] = "720p"
	case "1024x1024":
		payload["aspect_ratio"] = "1:1"
		payload["resolution"] = "720p"
	}
}

func parseXAIVideoResponse(body []byte, statusCode int, req contract.VideoRequest) (contract.VideoResponse, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return contract.VideoResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	id := firstVideoString(raw, "id", "request_id", "video_id")
	if id == "" {
		id = strings.TrimSpace(req.VideoID)
	}
	if id == "" {
		return contract.VideoResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no video id"}
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = responseVideoModel(firstVideoString(raw, "model"), req.Mapping.UpstreamModelName)
	}
	seconds := optionalVideoInt(raw, "seconds", "duration")
	size := firstVideoString(raw, "size")
	status := videoStatus(raw)
	progress := optionalVideoInt(raw, "progress")
	createdAt := optionalVideoInt64(raw, "created_at", "created", "createdAt")
	completedAt := optionalVideoInt64(raw, "completed_at", "completedAt")
	expiresAt := optionalVideoInt64(raw, "expires_at", "expiresAt")
	metadata := cloneMap(raw)
	for _, key := range []string{"id", "request_id", "video_id", "model", "status", "progress", "prompt", "seconds", "duration", "size", "created_at", "created", "createdAt", "completed_at", "completedAt", "expires_at", "expiresAt", "error"} {
		delete(metadata, key)
	}
	return contract.VideoResponse{
		ID:          id,
		Model:       model,
		Status:      status,
		Progress:    progress,
		Prompt:      firstNonEmptyVideoString(firstVideoString(raw, "prompt"), req.Prompt),
		Seconds:     seconds,
		Size:        firstNonEmptyVideoString(size, req.Size),
		CreatedAt:   createdAt,
		CompletedAt: completedAt,
		ExpiresAt:   expiresAt,
		Error:       videoError(raw),
		ContentURL:  xaiVideoContentURL(raw),
		Metadata:    metadata,
		StatusCode:  statusCode,
	}, nil
}

func videoStatus(raw map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(firstVideoString(raw, "status", "state"))) {
	case "completed", "complete", "succeeded", "success", "ready":
		return "completed"
	case "failed", "error", "cancelled", "canceled":
		return "failed"
	case "running", "processing", "in_progress", "generating":
		return "in_progress"
	default:
		return "queued"
	}
}

func videoError(raw map[string]any) *contract.VideoError {
	value, ok := raw["error"]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		message := strings.TrimSpace(typed)
		if message == "" {
			return nil
		}
		return &contract.VideoError{Message: message}
	case map[string]any:
		err := &contract.VideoError{
			Code:     firstVideoString(typed, "code", "type"),
			Message:  firstVideoString(typed, "message", "error"),
			Metadata: cloneMap(typed),
		}
		delete(err.Metadata, "code")
		delete(err.Metadata, "type")
		delete(err.Metadata, "message")
		delete(err.Metadata, "error")
		return err
	default:
		message := strings.TrimSpace(fmt.Sprint(typed))
		if message == "" {
			return nil
		}
		return &contract.VideoError{Message: message}
	}
}

func xaiVideoContentURL(raw map[string]any) string {
	video, ok := raw["video"].(map[string]any)
	if !ok {
		return ""
	}
	return firstVideoString(video, "url")
}

func validatedVideoContentURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no video content url"}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Host == "" {
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained invalid video content url"}
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return value, nil
	default:
		return "", contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained unsupported video content url"}
	}
}

func xaiVideoModel(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "", soraVideoModel:
		return defaultXAIVideoModel
	default:
		return strings.TrimSpace(model)
	}
}

func responseVideoModel(upstreamModel string, fallback string) string {
	if strings.EqualFold(strings.TrimSpace(upstreamModel), defaultXAIVideoModel) {
		return soraVideoModel
	}
	if value := strings.TrimSpace(upstreamModel); value != "" {
		return value
	}
	if strings.EqualFold(strings.TrimSpace(fallback), defaultXAIVideoModel) {
		return soraVideoModel
	}
	return strings.TrimSpace(fallback)
}

func upstreamBaseURLVideos(req contract.VideoRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"videos_base_url", "base_url", "upstream_base_url", "openai_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return presetReverseProxyBaseURL(req.Provider.AdapterType)
}

func cloneVideoContentHeaders(headers http.Header) http.Header {
	out := http.Header{}
	for _, key := range []string{"Content-Length", "Content-Disposition", "Cache-Control", "ETag", "Last-Modified"} {
		for _, value := range headers.Values(key) {
			if strings.TrimSpace(value) != "" {
				out.Add(key, value)
			}
		}
	}
	return out
}

func videoRequestSetting(req contract.VideoRequest, keys ...string) string {
	if req.RequestSettings == nil {
		return ""
	}
	for _, key := range keys {
		if value := mapString(req.RequestSettings, key); value != "" {
			return value
		}
	}
	return ""
}

func firstVideoString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := mapString(values, key); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyVideoString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func optionalVideoInt(values map[string]any, keys ...string) *int {
	for _, key := range keys {
		if value, ok := videoNumber(values[key]); ok {
			parsed := int(value)
			return &parsed
		}
	}
	return nil
}

func optionalVideoInt64(values map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		if value, ok := videoNumber(values[key]); ok {
			return &value
		}
	}
	return nil
}

func videoNumber(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
