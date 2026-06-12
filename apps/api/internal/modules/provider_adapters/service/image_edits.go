package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeImageEdit(ctx context.Context, req contract.ImageEditRequest) (contract.ImageGenerationResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || strings.TrimSpace(req.Prompt) == "" || len(req.Images) == 0 {
		return contract.ImageGenerationResponse{}, ErrInvalidInput
	}
	if imageGenerationDisabledForImageEdit(req) {
		return contract.ImageGenerationResponse{}, imageGenerationDisabledError()
	}
	if baseURL := upstreamBaseURLImageEdits(req); baseURL != "" {
		for _, image := range req.Images {
			if len(image.Bytes) == 0 {
				return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "image edit input image is empty"}
			}
		}
		if isReverseProxyImageEditRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleImageEdit(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleImageEdit(ctx, req, baseURL)
	}
	if isReverseProxyImageEditRuntime(req) {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	if !s.allowLocalStub {
		return contract.ImageGenerationResponse{}, errUpstreamBaseURLMissing("image edit")
	}
	return synthesizeLocalImageEdit(req), nil
}

func (s *Service) invokeOpenAICompatibleImageEdit(ctx context.Context, req contract.ImageEditRequest, baseURL string) (contract.ImageGenerationResponse, error) {
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
	parsed, err := parseOpenAICompatibleImageEdit(raw, resp.StatusCode, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleImageEdit(ctx context.Context, req contract.ImageEditRequest, baseURL string) (contract.ImageGenerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ImageGenerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	body, contentType, err := openAIImageEditMultipart(req)
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
		URL:    strings.TrimRight(baseURL, "/") + "/images/edits",
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
	parsed, err := parseOpenAICompatibleImageEdit(runtimeResp.Body, runtimeResp.StatusCode, req)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func openAIImageEditMultipart(req contract.ImageEditRequest) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeFormField(writer, "model", req.Mapping.UpstreamModelName)
	writeFormField(writer, "prompt", req.Prompt)
	if req.Count > 0 {
		writeFormField(writer, "n", fmt.Sprintf("%d", req.Count))
	}
	writeFormField(writer, "size", req.Size)
	writeFormField(writer, "quality", req.Quality)
	writeFormField(writer, "response_format", req.ResponseFormat)
	writeFormField(writer, "user", req.User)
	for key, value := range req.Extra {
		if key == "" || value == nil || reservedImageEditField(key) {
			continue
		}
		writeFormField(writer, key, fmt.Sprint(value))
	}
	imageFieldName := "image"
	if len(req.Images) > 1 {
		imageFieldName = "image[]"
	}
	for _, image := range req.Images {
		if err := writeImageEditFilePart(writer, imageFieldName, image); err != nil {
			return nil, "", err
		}
	}
	if req.Mask != nil {
		if err := writeImageEditFilePart(writer, "mask", *req.Mask); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func writeImageEditFilePart(writer *multipart.Writer, fieldName string, image contract.ImageInput) error {
	filename := strings.TrimSpace(image.FileName)
	if filename == "" {
		filename = "image"
	}
	contentType := strings.TrimSpace(image.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, escapeMultipartFilename(filename)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(image.Bytes)
	return err
}

func reservedImageEditField(key string) bool {
	switch strings.TrimSpace(key) {
	case "image", "image[]", "mask", "model", "prompt", "n", "size", "quality", "response_format", "stream", "user":
		return true
	default:
		return false
	}
}

func parseOpenAICompatibleImageEdit(body []byte, statusCode int, req contract.ImageEditRequest) (contract.ImageGenerationResponse, error) {
	imageReq := imageGenerationRequestFromEdit(req)
	resp, err := parseOpenAICompatibleImages(body, statusCode, req.Mapping.UpstreamModelName, imageReq)
	if err != nil {
		return contract.ImageGenerationResponse{}, err
	}
	if resp.Usage.Estimated {
		resp.Usage = imageEditUsage(req)
	}
	return resp, nil
}

func imageGenerationRequestFromEdit(req contract.ImageEditRequest) contract.ImageGenerationRequest {
	return contract.ImageGenerationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: req.SourceProtocol,
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.Model,
		Prompt:         req.Prompt,
		Count:          req.Count,
		Size:           req.Size,
		Quality:        req.Quality,
		ResponseFormat: req.ResponseFormat,
		User:           req.User,
		Extra:          cloneMap(req.Extra),
		Provider:       req.Provider,
		Account:        req.Account,
		Mapping:        req.Mapping,
		Credential:     cloneMap(req.Credential),
	}
}

func imageEditUsage(req contract.ImageEditRequest) contract.Usage {
	usage := estimatedImageUsage(imageGenerationRequestFromEdit(req))
	for _, image := range req.Images {
		if len(image.Bytes) > 0 {
			usage.InputTokens += max(1, len(image.Bytes)/1024)
		}
	}
	if req.Mask != nil && len(req.Mask.Bytes) > 0 {
		usage.InputTokens += max(1, len(req.Mask.Bytes)/1024)
	}
	return usage
}

func upstreamBaseURLImageEdits(req contract.ImageEditRequest) string {
	return upstreamBaseURLImages(imageGenerationRequestFromEdit(req))
}

func isReverseProxyImageEditRuntime(req contract.ImageEditRequest) bool {
	return isReverseProxyImageRuntime(imageGenerationRequestFromEdit(req))
}

func synthesizeLocalImageEdit(req contract.ImageEditRequest) contract.ImageGenerationResponse {
	resp := synthesizeLocalImages(imageGenerationRequestFromEdit(req))
	resp.Usage = imageEditUsage(req)
	return resp
}
