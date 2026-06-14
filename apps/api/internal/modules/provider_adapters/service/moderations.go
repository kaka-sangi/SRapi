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
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeModerations(ctx context.Context, req contract.ModerationRequest) (contract.ModerationResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || len(req.Input) == 0 {
		return contract.ModerationResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLModerations(req); baseURL != "" {
		if isReverseProxyModerationRuntime(req) {
			return s.invokeReverseProxyOpenAICompatibleModerations(ctx, req, baseURL)
		}
		return s.invokeOpenAICompatibleModerations(ctx, req, baseURL)
	}
	if isReverseProxyModerationRuntime(req) {
		return contract.ModerationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	return synthesizeLocalModerations(req), nil
}

func (s *Service) invokeOpenAICompatibleModerations(ctx context.Context, req contract.ModerationRequest, baseURL string) (contract.ModerationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ModerationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := json.Marshal(openAIModerationPayload(req))
	if err != nil {
		return contract.ModerationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/moderations", bytes.NewReader(raw))
	if err != nil {
		return contract.ModerationResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ModerationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ModerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.ModerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ModerationResponse{}, classifyProviderHTTPErrorWithHeaders(resp.StatusCode, resp.Header, body)
	}
	parsed, err := parseOpenAICompatibleModerations(body, resp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
	if err != nil {
		return contract.ModerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(resp.Header)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(resp.Header, time.Now().UTC())
	return parsed, nil
}

func (s *Service) invokeReverseProxyOpenAICompatibleModerations(ctx context.Context, req contract.ModerationRequest, baseURL string) (contract.ModerationResponse, error) {
	if s.reverseProxy == nil {
		return contract.ModerationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	raw, err := json.Marshal(openAIModerationPayload(req))
	if err != nil {
		return contract.ModerationResponse{}, err
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
		URL:    strings.TrimRight(baseURL, "/") + "/moderations",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.ModerationResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.ModerationResponse{}, classifyProviderHTTPErrorWithHeaders(runtimeResp.StatusCode, runtimeResp.Headers, runtimeResp.Body)
	}
	parsed, err := parseOpenAICompatibleModerations(runtimeResp.Body, runtimeResp.StatusCode, req.Mapping.UpstreamModelName, req.Input)
	if err != nil {
		return contract.ModerationResponse{}, err
	}
	parsed.Headers = cloneGenericHeaders(runtimeResp.Headers)
	parsed.QuotaSignals = providerQuotaSignalsFromHeaders(runtimeResp.Headers, time.Now().UTC())
	return parsed, nil
}

type openAIModerationRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
	User  string   `json:"user,omitempty"`
}

func openAIModerationPayload(req contract.ModerationRequest) openAIModerationRequest {
	return openAIModerationRequest{
		Model: req.Mapping.UpstreamModelName,
		Input: append([]string(nil), req.Input...),
		User:  strings.TrimSpace(req.User),
	}
}

type openAIModerationResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Flagged                   bool                `json:"flagged"`
		Categories                map[string]bool     `json:"categories"`
		CategoryScores            map[string]float32  `json:"category_scores"`
		CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	} `json:"results"`
	Usage openAIUsage `json:"usage"`
}

func parseOpenAICompatibleModerations(body []byte, statusCode int, fallbackModel string, input []string) (contract.ModerationResponse, error) {
	var decoded openAIModerationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.ModerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	results := make([]contract.ModerationResult, 0, len(decoded.Results))
	for _, item := range decoded.Results {
		results = append(results, contract.ModerationResult{
			Flagged:                   item.Flagged,
			Categories:                cloneBoolMap(item.Categories),
			CategoryScores:            cloneFloat32Map(item.CategoryScores),
			CategoryAppliedInputTypes: cloneStringSliceMap(item.CategoryAppliedInputTypes),
		})
	}
	if len(results) == 0 {
		return contract.ModerationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no moderation results"}
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	id := strings.TrimSpace(decoded.ID)
	if id == "" {
		id = fmt.Sprintf("modr_%s", url.PathEscape(model))
	}
	return contract.ModerationResponse{
		ID:         id,
		Results:    results,
		Model:      model,
		StatusCode: statusCode,
		Usage:      decoded.Usage.ToModerationUsage(input),
	}, nil
}

func upstreamBaseURLModerations(req contract.ModerationRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url", "moderations_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return presetReverseProxyBaseURL(req.Provider.AdapterType)
}

func isReverseProxyModerationRuntime(req contract.ModerationRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func synthesizeLocalModerations(req contract.ModerationRequest) contract.ModerationResponse {
	results := make([]contract.ModerationResult, 0, len(req.Input))
	for _, value := range req.Input {
		score := float32(0)
		if strings.TrimSpace(value) != "" {
			score = 0.001
		}
		results = append(results, contract.ModerationResult{
			Flagged: false,
			Categories: map[string]bool{
				"violence": false,
			},
			CategoryScores: map[string]float32{
				"violence": score,
			},
		})
	}
	return contract.ModerationResponse{
		ID:         "modr_local",
		Results:    results,
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: http.StatusOK,
		Usage:      estimatedModerationUsage(req.Input),
	}
}

func estimatedModerationUsage(input []string) contract.Usage {
	return contract.Usage{
		InputTokens: estimateTokens(strings.Join(input, "\n")),
		Estimated:   true,
	}
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneFloat32Map(values map[string]float32) map[string]float32 {
	if values == nil {
		return nil
	}
	out := make(map[string]float32, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if values == nil {
		return nil
	}
	out := make(map[string][]string, len(values))
	for key, value := range values {
		out[key] = append([]string(nil), value...)
	}
	return out
}
