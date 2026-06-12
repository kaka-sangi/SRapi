package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeImageVariation(ctx context.Context, req contract.ImageVariationRequest) (contract.ImageGenerationResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || len(req.Image.Bytes) == 0 {
		return contract.ImageGenerationResponse{}, ErrInvalidInput
	}
	if imageGenerationDisabledForImageVariation(req) {
		return contract.ImageGenerationResponse{}, imageGenerationDisabledError()
	}
	if baseURL := upstreamBaseURLImageVariations(req); baseURL != "" {
		if isReverseProxyImageVariationRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleImageVariation(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleImageVariation(ctx, req, baseURL)
	}
	if isReverseProxyImageVariationRuntime(req) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	if !s.allowLocalStub {
		return contract.ImageGenerationResponse{}, errUpstreamBaseURLMissing("image variation")
	}
	return synthesizeLocalImageVariation(req), nil
}

func (s *Service) invokeOpenAICompatibleImageVariation(ctx context.Context, req contract.ImageVariationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	body, contentType, err := openAIImageVariationMultipart(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/images/variations", bytes.NewReader(body))
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, raw)
	}
	parsed, err := parseOpenAICompatibleImageVariation(raw, resp.StatusCode, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleImageVariation(ctx context.Context, req contract.ImageVariationRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	body, contentType, err := openAIImageVariationMultipart(req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Metadata:       req.Account.Metadata,
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/images/variations",
		Headers: http.Header{
			"Content-Type": {contentType},
		},
		Body: body,
	})
	if err != nil {
		return contract.ImageGenerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ImageGenerationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAICompatibleImageVariation(runtimeResp.Body, runtimeResp.StatusCode, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func openAIImageVariationMultipart(req contract.ImageVariationRequest) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeFormField(writer, "model", req.Mapping.UpstreamModelName)
	if req.Count > 0 {
		writeFormField(writer, "n", fmt.Sprintf("%d", req.Count))
	}
	writeFormField(writer, "size", req.Size)
	writeFormField(writer, "response_format", req.ResponseFormat)
	writeFormField(writer, "user", req.User)
	for key, value := range req.Extra {
		if key == "" || value == nil || reservedImageVariationField(key) {
			continue
		}
		writeFormField(writer, key, fmt.Sprint(value))
	}
	if err := writeImageEditFilePart(writer, "image", req.Image); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func reservedImageVariationField(key string) bool {
	switch strings.TrimSpace(key) {
	case "image", "model", "n", "size", "response_format", "user":
		return true
	default:
		return false
	}
}

func parseOpenAICompatibleImageVariation(body []byte, statusCode int, req contract.ImageVariationRequest) (contract.ImageGenerationResponse, error) {
	imageReq := imageGenerationRequestFromVariation(req)
	resp, err := parseOpenAICompatibleImages(body, statusCode, req.Mapping.UpstreamModelName, imageReq)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	if resp.Usage.Estimated {
		resp.Usage = imageVariationUsage(req)
	}
	return resp, nil
}

func imageGenerationRequestFromVariation(req contract.ImageVariationRequest) contract.ImageGenerationRequest {
	return contract.ImageGenerationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: req.SourceProtocol,
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.Model,
		Prompt:         "",
		Count:          req.Count,
		Size:           req.Size,
		ResponseFormat: req.ResponseFormat,
		User:           req.User,
		Extra:          cloneMap(req.Extra),
		Provider:       req.Provider,
		Account:        req.Account,
		Mapping:        req.Mapping,
		Credential:     cloneMap(req.Credential),
	}
}

func imageVariationUsage(req contract.ImageVariationRequest) contract.Usage {
	usage := estimatedImageUsage(imageGenerationRequestFromVariation(req))
	if len(req.Image.Bytes) > 0 {
		usage.InputTokens += max(1, len(req.Image.Bytes)/1024)
	}
	return usage
}

func upstreamBaseURLImageVariations(req contract.ImageVariationRequest) string {
	return upstreamBaseURLImages(imageGenerationRequestFromVariation(req))
}

func isReverseProxyImageVariationRuntime(req contract.ImageVariationRequest) bool {
	return isReverseProxyImageRuntime(imageGenerationRequestFromVariation(req))
}

func synthesizeLocalImageVariation(req contract.ImageVariationRequest) contract.ImageGenerationResponse {
	resp := synthesizeLocalImages(imageGenerationRequestFromVariation(req))
	resp.Usage = imageVariationUsage(req)
	return resp
}
