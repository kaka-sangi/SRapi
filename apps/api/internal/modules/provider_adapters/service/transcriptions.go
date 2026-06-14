package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeAudioTranscription(ctx context.Context, req contract.AudioTranscriptionRequest) (contract.AudioTranscriptionResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || len(req.Audio) == 0 {
		return contract.AudioTranscriptionResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLTranscriptions(req); baseURL != "" {
		if isReverseProxyAudioTranscriptionRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleAudioTranscription(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleAudioTranscription(ctx, req, baseURL)
	}
	if isReverseProxyAudioTranscriptionRuntime(req) {
		return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	return synthesizeLocalAudioTranscription(req), nil
}

func (s *Service) invokeOpenAICompatibleAudioTranscription(ctx context.Context, req contract.AudioTranscriptionRequest, baseURL string) (contract.AudioTranscriptionResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	body, contentType, err := openAIAudioTranscriptionMultipart(req)
	if err != nil {
		return contract.AudioTranscriptionResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/audio/transcriptions", bytes.NewReader(body))
	if err != nil {
		return contract.AudioTranscriptionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.AudioTranscriptionResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, raw)
	}
	parsed, err := parseOpenAICompatibleAudioTranscription(raw, resp.StatusCode, req)
	if err != nil {
		return contract.AudioTranscriptionResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleAudioTranscription(ctx context.Context, req contract.AudioTranscriptionRequest, baseURL string) (contract.AudioTranscriptionResponse, error) {
	if s.reverseProxy == nil {
		return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	body, contentType, err := openAIAudioTranscriptionMultipart(req)
	if err != nil {
		return contract.AudioTranscriptionResponse{}, err
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
		URL:    strings.TrimRight(baseURL, "/") + "/audio/transcriptions",
		Headers: http.Header{
			"Content-Type": {contentType},
		},
		Body: body,
	})
	if err != nil {
		return contract.AudioTranscriptionResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.AudioTranscriptionResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAICompatibleAudioTranscription(runtimeResp.Body, runtimeResp.StatusCode, req)
	if err != nil {
		return contract.AudioTranscriptionResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

func openAIAudioTranscriptionMultipart(req contract.AudioTranscriptionRequest) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writeFormField(writer, "model", req.Mapping.UpstreamModelName)
	writeFormField(writer, "language", req.Language)
	writeFormField(writer, "prompt", req.Prompt)
	writeFormField(writer, "response_format", audioTranscriptionResponseFormat(req))
	if req.Temperature != nil {
		writeFormField(writer, "temperature", fmt.Sprintf("%g", *req.Temperature))
	}
	writeFormField(writer, "user", req.User)
	for key, value := range req.Extra {
		if key == "" || value == nil {
			continue
		}
		switch key {
		case "file", "model", "language", "prompt", "response_format", "temperature", "user":
			continue
		}
		writeFormField(writer, key, fmt.Sprint(value))
	}
	if err := writeAudioFilePart(writer, req); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func writeAudioFilePart(writer *multipart.Writer, req contract.AudioTranscriptionRequest) error {
	filename := strings.TrimSpace(req.FileName)
	if filename == "" {
		filename = "audio"
	}
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartFilename(filename)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(req.Audio)
	return err
}

func writeFormField(writer *multipart.Writer, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	_ = writer.WriteField(key, value)
}

func escapeMultipartFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

func audioTranscriptionResponseFormat(req contract.AudioTranscriptionRequest) string {
	format := strings.TrimSpace(req.ResponseFormat)
	if format == "" {
		return "json"
	}
	return format
}

type openAIAudioTranscriptionResponse struct {
	Text     string                            `json:"text"`
	Task     string                            `json:"task"`
	Language string                            `json:"language"`
	Duration *float32                          `json:"duration"`
	Segments []openAIAudioTranscriptionSegment `json:"segments"`
	Usage    openAIUsage                       `json:"usage"`
}

type openAIAudioTranscriptionSegment struct {
	ID               *int           `json:"id"`
	Seek             *int           `json:"seek"`
	Start            *float32       `json:"start"`
	End              *float32       `json:"end"`
	Text             string         `json:"text"`
	Tokens           []int          `json:"tokens"`
	Temperature      *float32       `json:"temperature"`
	AvgLogprob       *float32       `json:"avg_logprob"`
	CompressionRatio *float32       `json:"compression_ratio"`
	NoSpeechProb     *float32       `json:"no_speech_prob"`
	Metadata         map[string]any `json:"-"`
}

func parseOpenAICompatibleAudioTranscription(body []byte, statusCode int, req contract.AudioTranscriptionRequest) (contract.AudioTranscriptionResponse, error) {
	var decoded openAIAudioTranscriptionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		text := strings.TrimSpace(string(body))
		if text == "" {
			return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned empty transcription"}
		}
		return contract.AudioTranscriptionResponse{
			ID:         fmt.Sprintf("transcription_%s", url.PathEscape(req.Mapping.UpstreamModelName)),
			Text:       text,
			Model:      req.Mapping.UpstreamModelName,
			StatusCode: statusCode,
			Usage:      estimatedAudioTranscriptionUsage(req),
		}, nil
	}
	text := strings.TrimSpace(decoded.Text)
	if text == "" {
		return contract.AudioTranscriptionResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no transcription text"}
	}
	return contract.AudioTranscriptionResponse{
		ID:         fmt.Sprintf("transcription_%s", url.PathEscape(req.Mapping.UpstreamModelName)),
		Text:       text,
		Task:       strings.TrimSpace(decoded.Task),
		Language:   strings.TrimSpace(decoded.Language),
		Duration:   cloneFloat32Provider(decoded.Duration),
		Segments:   audioTranscriptionSegments(decoded.Segments),
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: statusCode,
		Usage:      audioTranscriptionUsage(decoded.Usage, req),
	}, nil
}

func audioTranscriptionSegments(values []openAIAudioTranscriptionSegment) []contract.AudioTranscriptionSegment {
	if len(values) == 0 {
		return nil
	}
	out := make([]contract.AudioTranscriptionSegment, 0, len(values))
	for _, value := range values {
		out = append(out, contract.AudioTranscriptionSegment{
			ID:               cloneIntProvider(value.ID),
			Seek:             cloneIntProvider(value.Seek),
			Start:            cloneFloat32Provider(value.Start),
			End:              cloneFloat32Provider(value.End),
			Text:             strings.TrimSpace(value.Text),
			Tokens:           append([]int(nil), value.Tokens...),
			Temperature:      cloneFloat32Provider(value.Temperature),
			AvgLogprob:       cloneFloat32Provider(value.AvgLogprob),
			CompressionRatio: cloneFloat32Provider(value.CompressionRatio),
			NoSpeechProb:     cloneFloat32Provider(value.NoSpeechProb),
			Metadata:         cloneMap(value.Metadata),
		})
	}
	return out
}

func audioTranscriptionUsage(usage openAIUsage, req contract.AudioTranscriptionRequest) contract.Usage {
	if usage.HasTokenUsage() {
		out := usage.ToUsage(req.Prompt)
		out.OutputTokens = 0
		return out
	}
	return estimatedAudioTranscriptionUsage(req)
}

func upstreamBaseURLTranscriptions(req contract.AudioTranscriptionRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url", "audio_base_url", "transcriptions_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return presetReverseProxyBaseURL(req.Provider.AdapterType)
}

func isReverseProxyAudioTranscriptionRuntime(req contract.AudioTranscriptionRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func synthesizeLocalAudioTranscription(req contract.AudioTranscriptionRequest) contract.AudioTranscriptionResponse {
	filename := strings.TrimSpace(req.FileName)
	if filename == "" {
		filename = "audio"
	}
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "und"
	}
	return contract.AudioTranscriptionResponse{
		ID:         "transcription_local",
		Text:       "SRapi local transcription for " + filename,
		Task:       "transcribe",
		Language:   language,
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: http.StatusOK,
		Usage:      estimatedAudioTranscriptionUsage(req),
	}
}

func estimatedAudioTranscriptionUsage(req contract.AudioTranscriptionRequest) contract.Usage {
	input := estimateTokens(req.Prompt)
	if len(req.Audio) > 0 {
		input += max(1, len(req.Audio)/1024)
	}
	return contract.Usage{
		InputTokens: input,
		Estimated:   true,
	}
}

func cloneIntProvider(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat32Provider(value *float32) *float32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
