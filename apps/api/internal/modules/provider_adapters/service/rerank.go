package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func (s *Service) InvokeRerank(ctx context.Context, req contract.RerankRequest) (contract.RerankResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" || strings.TrimSpace(req.Query) == "" || len(req.Documents) == 0 {
		return contract.RerankResponse{}, ErrInvalidInput
	}
	if baseURL := upstreamBaseURLRerank(req); baseURL != "" {
		if isReverseProxyRerankRuntime(req) {
			return s.invokeReverseProxyRerankCompatible(ctx, req, baseURL)
		}
		return s.invokeRerankCompatible(ctx, req, baseURL)
	}
	if isReverseProxyRerankRuntime(req) {
		return contract.RerankResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy upstream base url missing"}
	}
	return synthesizeLocalRerank(req), nil
}

func (s *Service) invokeRerankCompatible(ctx context.Context, req contract.RerankRequest, baseURL string) (contract.RerankResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.RerankResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := json.Marshal(rerankPayload(req))
	if err != nil {
		return contract.RerankResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/rerank", bytes.NewReader(raw))
	if err != nil {
		return contract.RerankResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.RerankResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.RerankResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.RerankResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.RerankResponse{}, classifyProviderHTTPError(resp.StatusCode, body)
	}
	return parseRerankCompatibleResponse(body, resp.StatusCode, req)
}

func (s *Service) invokeReverseProxyRerankCompatible(ctx context.Context, req contract.RerankRequest, baseURL string) (contract.RerankResponse, error) {
	if s.reverseProxy == nil {
		return contract.RerankResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "reverse proxy runtime unavailable"}
	}
	raw, err := json.Marshal(rerankPayload(req))
	if err != nil {
		return contract.RerankResponse{}, err
	}
	runtimeResp, err := s.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: reverseproxycontract.AccountRuntime{
			AccountID:      req.Account.ID,
			RuntimeClass:   string(req.Account.RuntimeClass),
			UpstreamClient: req.Account.UpstreamClient,
			ProxyID:        req.Account.ProxyID,
			UserAgent:      mapString(req.Account.Metadata, "user_agent"),
			Credential:     req.Credential,
		},
		Method: http.MethodPost,
		URL:    strings.TrimRight(baseURL, "/") + "/rerank",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: raw,
	})
	if err != nil {
		return contract.RerankResponse{}, providerErrorFromReverseProxy(err)
	}
	if runtimeResp.StatusCode < 200 || runtimeResp.StatusCode >= 300 {
		return contract.RerankResponse{}, classifyProviderHTTPError(runtimeResp.StatusCode, runtimeResp.Body)
	}
	return parseRerankCompatibleResponse(runtimeResp.Body, runtimeResp.StatusCode, req)
}

type rerankCompatibleRequest struct {
	Model           string `json:"model"`
	Query           string `json:"query"`
	Documents       []any  `json:"documents"`
	TopN            *int   `json:"top_n,omitempty"`
	ReturnDocuments bool   `json:"return_documents,omitempty"`
	User            string `json:"user,omitempty"`
}

func rerankPayload(req contract.RerankRequest) rerankCompatibleRequest {
	documents := make([]any, 0, len(req.Documents))
	for _, document := range req.Documents {
		if len(document.Fields) > 0 {
			documents = append(documents, cloneMap(document.Fields))
			continue
		}
		documents = append(documents, strings.TrimSpace(document.Text))
	}
	return rerankCompatibleRequest{
		Model:           req.Mapping.UpstreamModelName,
		Query:           strings.TrimSpace(req.Query),
		Documents:       documents,
		TopN:            cloneIntPtr(req.TopN),
		ReturnDocuments: req.ReturnDocuments,
		User:            strings.TrimSpace(req.User),
	}
}

type rerankCompatibleResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Index          int            `json:"index"`
		RelevanceScore float32        `json:"relevance_score"`
		Document       map[string]any `json:"document"`
	} `json:"results"`
	Usage openAIUsage `json:"usage"`
	Meta  struct {
		BilledUnits *struct {
			SearchUnits *int `json:"search_units"`
		} `json:"billed_units"`
	} `json:"meta"`
}

func parseRerankCompatibleResponse(body []byte, statusCode int, req contract.RerankRequest) (contract.RerankResponse, error) {
	var decoded rerankCompatibleResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return contract.RerankResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid json"}
	}
	results := make([]contract.RerankResult, 0, len(decoded.Results))
	for _, item := range decoded.Results {
		result := contract.RerankResult{
			Index:          item.Index,
			RelevanceScore: item.RelevanceScore,
		}
		if item.Document != nil {
			document := rerankDocumentFromObject(item.Document)
			result.Document = &document
		} else if req.ReturnDocuments && item.Index >= 0 && item.Index < len(req.Documents) {
			document := cloneRerankDocument(req.Documents[item.Index])
			result.Document = &document
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return contract.RerankResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response contained no rerank results"}
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = strings.TrimSpace(req.Mapping.UpstreamModelName)
	}
	id := strings.TrimSpace(decoded.ID)
	if id == "" {
		id = fmt.Sprintf("rerank_%s", url.PathEscape(model))
	}
	return contract.RerankResponse{
		ID:         id,
		Results:    results,
		Model:      model,
		StatusCode: statusCode,
		Usage:      rerankUsage(decoded.Usage, decoded.Meta.BilledUnits, req),
	}, nil
}

func rerankUsage(usage openAIUsage, billedUnits *struct {
	SearchUnits *int `json:"search_units"`
}, req contract.RerankRequest) contract.Usage {
	if usage.HasTokenUsage() {
		out := usage.ToUsage(rerankUsageText(req))
		out.OutputTokens = 0
		return out
	}
	if billedUnits != nil && billedUnits.SearchUnits != nil && *billedUnits.SearchUnits > 0 {
		return contract.Usage{InputTokens: *billedUnits.SearchUnits, Estimated: false}
	}
	return estimatedRerankUsage(req)
}

func upstreamBaseURLRerank(req contract.RerankRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "rerank_base_url"} {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func isReverseProxyRerankRuntime(req contract.RerankRequest) bool {
	runtimeClass := strings.TrimSpace(string(req.Account.RuntimeClass))
	if runtimeClass != "" && runtimeClass != "api_key" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-")
}

func synthesizeLocalRerank(req contract.RerankRequest) contract.RerankResponse {
	results := make([]contract.RerankResult, 0, len(req.Documents))
	queryTerms := uniqueTrimmedStrings(strings.Fields(strings.ToLower(req.Query)))
	for idx, document := range req.Documents {
		score := rerankLocalScore(queryTerms, strings.ToLower(document.Text), idx)
		result := contract.RerankResult{Index: idx, RelevanceScore: score}
		if req.ReturnDocuments {
			doc := cloneRerankDocument(document)
			result.Document = &doc
		}
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].RelevanceScore == results[j].RelevanceScore {
			return results[i].Index < results[j].Index
		}
		return results[i].RelevanceScore > results[j].RelevanceScore
	})
	if req.TopN != nil && *req.TopN < len(results) {
		results = results[:*req.TopN]
	}
	return contract.RerankResponse{
		ID:         "rerank_local",
		Results:    results,
		Model:      req.Mapping.UpstreamModelName,
		StatusCode: http.StatusOK,
		Usage:      estimatedRerankUsage(req),
	}
}

func rerankLocalScore(queryTerms []string, document string, index int) float32 {
	if len(queryTerms) == 0 || strings.TrimSpace(document) == "" {
		return 0
	}
	matches := 0
	for _, term := range queryTerms {
		if strings.Contains(document, term) {
			matches++
		}
	}
	base := float64(matches) / float64(len(queryTerms))
	tiebreak := 1 / float64((index+1)*1000)
	return float32(math.Min(1, base+tiebreak))
}

func estimatedRerankUsage(req contract.RerankRequest) contract.Usage {
	return contract.Usage{
		InputTokens: estimateTokens(rerankUsageText(req)),
		Estimated:   true,
	}
}

func rerankUsageText(req contract.RerankRequest) string {
	parts := make([]string, 0, len(req.Documents)+1)
	parts = append(parts, strings.TrimSpace(req.Query))
	for _, document := range req.Documents {
		if text := strings.TrimSpace(document.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func cloneRerankDocument(value contract.RerankDocument) contract.RerankDocument {
	return contract.RerankDocument{
		Text:     value.Text,
		Fields:   cloneMap(value.Fields),
		Original: cloneAny(value.Original),
	}
}

func rerankDocumentFromObject(value map[string]any) contract.RerankDocument {
	fields := cloneMap(value)
	return contract.RerankDocument{
		Text:     rerankObjectText(fields),
		Fields:   fields,
		Original: cloneMap(fields),
	}
}

func rerankObjectText(fields map[string]any) string {
	for _, key := range []string{"text", "content", "document"} {
		if value, ok := fields[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	return ""
}
