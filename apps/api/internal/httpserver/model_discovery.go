package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	defaultModelDiscoveryLimit = 200
	maxModelDiscoveryBody      = 2 << 20
)

var (
	errModelDiscoveryUnsupported  = errors.New("model discovery is unsupported for this account")
	errModelDiscoveryInvalidInput = errors.New("invalid model discovery request")
	errModelDiscoveryAuth         = errors.New("model discovery credential missing")
	errModelDiscoveryUpstream     = errors.New("model discovery upstream failed")
)

type modelDiscoverySource string

const (
	modelDiscoveryOpenAI    modelDiscoverySource = "openai-compatible"
	modelDiscoveryAnthropic modelDiscoverySource = "anthropic-compatible"
	modelDiscoveryGemini    modelDiscoverySource = "gemini-compatible"
)

func (rt *runtimeState) discoverAccountModels(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, req apiopenapi.DiscoverAccountModelsRequest) (apiopenapi.AccountModelDiscovery, error) {
	if account.RuntimeClass != accountcontract.RuntimeClassAPIKey {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryUnsupported
	}
	source, ok := modelDiscoverySourceForProvider(provider)
	if !ok {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryUnsupported
	}
	limit := defaultModelDiscoveryLimit
	if req.Limit != nil {
		if *req.Limit <= 0 || *req.Limit > 1000 {
			return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryInvalidInput
		}
		limit = *req.Limit
	}
	credential, err := rt.accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryAuth
	}
	endpoint := modelDiscoveryEndpoint(source, provider, account)
	if endpoint == "" {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryInvalidInput
	}
	if !validModelDiscoveryEndpoint(endpoint) {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryInvalidInput
	}
	requestEndpoint := endpoint
	headers, err := modelDiscoveryHeaders(source, provider, account, credential, &requestEndpoint)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, requestEndpoint, nil)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryInvalidInput
	}
	httpReq.Header = headers

	client := &http.Client{Timeout: rt.cfg.Gateway.RequestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryUpstream
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelDiscoveryBody))
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryUpstream
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryUpstream
	}
	modelIDs, err := parseDiscoveredModelIDs(source, body, limit)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, err
	}

	checkedAt := time.Now().UTC()
	persisted := req.Persist != nil && *req.Persist
	if persisted {
		metadata := cloneMetadata(account.Metadata)
		metadata["supported_models"] = append([]string(nil), modelIDs...)
		metadata["model_discovery_source"] = string(source)
		metadata["model_discovery_endpoint"] = endpoint
		metadata["model_discovery_last_seen_at"] = checkedAt.Format(time.RFC3339)
		if _, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata}); err != nil {
			return apiopenapi.AccountModelDiscovery{}, err
		}
	}

	return apiopenapi.AccountModelDiscovery{
		AccountId:  apiopenapi.Id(strconv.Itoa(account.ID)),
		CheckedAt:  checkedAt,
		Endpoint:   endpoint,
		ModelIds:   modelIDs,
		Persisted:  persisted,
		ProviderId: apiopenapi.Id(strconv.Itoa(provider.ID)),
		Source:     apiopenapi.AccountModelDiscoverySource(source),
	}, nil
}

func modelDiscoverySourceForProvider(provider providercontract.Provider) (modelDiscoverySource, bool) {
	for _, value := range []string{provider.Protocol, provider.AdapterType} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "openai-compatible", "native-openai":
			return modelDiscoveryOpenAI, true
		case "anthropic-compatible", "claude-compatible":
			return modelDiscoveryAnthropic, true
		case "gemini-compatible", "native-gemini":
			return modelDiscoveryGemini, true
		}
	}
	return "", false
}

func modelDiscoveryEndpoint(source modelDiscoverySource, provider providercontract.Provider, account accountcontract.ProviderAccount) string {
	baseURL := upstreamModelDiscoveryBaseURL(source, provider, account)
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/models") {
		return baseURL
	}
	return strings.TrimRight(baseURL, "/") + "/models"
}

func validModelDiscoveryEndpoint(rawURL string) bool {
	parsed, err := parsedModelDiscoveryURL(rawURL)
	return err == nil && parsed != nil
}

func upstreamModelDiscoveryBaseURL(source modelDiscoverySource, provider providercontract.Provider, account accountcontract.ProviderAccount) string {
	keys := []string{"models_url", "models_endpoint", "base_url", "upstream_base_url"}
	switch source {
	case modelDiscoveryOpenAI:
		keys = append([]string{"openai_models_url", "openai_base_url"}, keys...)
	case modelDiscoveryAnthropic:
		keys = append([]string{"anthropic_models_url", "anthropic_base_url"}, keys...)
	case modelDiscoveryGemini:
		keys = append([]string{"gemini_models_url", "gemini_base_url"}, keys...)
	}
	for _, values := range []map[string]any{account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return ""
}

func modelDiscoveryHeaders(source modelDiscoverySource, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, endpoint *string) (http.Header, error) {
	headers := http.Header{
		"Accept": {"application/json"},
	}
	apiKey := modelDiscoveryAPIKey(source, credential)
	if apiKey == "" {
		return nil, errModelDiscoveryAuth
	}
	authMode := strings.ToLower(modelDiscoverySetting(provider, account, credential, "auth_mode"))
	switch source {
	case modelDiscoveryAnthropic:
		if authMode == "" {
			authMode = "x_api_key"
		}
	case modelDiscoveryGemini:
		if authMode == "" {
			authMode = "api_key_query"
		}
	default:
		if authMode == "" {
			authMode = "bearer"
		}
	}
	switch authMode {
	case "bearer":
		headers.Set("Authorization", "Bearer "+apiKey)
	case "x_api_key", "api_key_header":
		if source == modelDiscoveryGemini {
			headers.Set("x-goog-api-key", apiKey)
		} else {
			headers.Set("x-api-key", apiKey)
		}
	case "x_goog_api_key":
		headers.Set("x-goog-api-key", apiKey)
	case "custom_header":
		headerName := modelDiscoverySetting(provider, account, credential, "custom_header_name", "auth_header", "api_key_header")
		if headerName == "" {
			return nil, errModelDiscoveryInvalidInput
		}
		headers.Set(headerName, apiKey)
	case "api_key_query":
		if endpoint == nil {
			return nil, errModelDiscoveryInvalidInput
		}
		param := modelDiscoverySetting(provider, account, credential, "api_key_query_param", "query_param")
		if param == "" {
			param = "key"
		}
		*endpoint = appendModelDiscoveryQuery(*endpoint, param, apiKey)
	default:
		return nil, errModelDiscoveryInvalidInput
	}
	return headers, nil
}

func modelDiscoveryAPIKey(source modelDiscoverySource, credential map[string]any) string {
	keys := []string{"api_key", "access_token"}
	switch source {
	case modelDiscoveryAnthropic:
		keys = []string{"api_key", "x_api_key", "anthropic_api_key", "access_token"}
	case modelDiscoveryGemini:
		keys = []string{"api_key", "gemini_api_key", "google_api_key", "access_token"}
	}
	for _, key := range keys {
		if value := mapString(credential, key); value != "" {
			return value
		}
	}
	return ""
}

func modelDiscoverySetting(provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, keys ...string) string {
	for _, values := range []map[string]any{credential, account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		for _, key := range keys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func appendModelDiscoveryQuery(rawURL string, key string, value string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		separator := "?"
		if strings.Contains(rawURL, "?") {
			separator = "&"
		}
		return rawURL + separator + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func parsedModelDiscoveryURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, errModelDiscoveryInvalidInput
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errModelDiscoveryInvalidInput
	}
	if parsed.Host == "" {
		return nil, errModelDiscoveryInvalidInput
	}
	return parsed, nil
}

func parseDiscoveredModelIDs(source modelDiscoverySource, body []byte, limit int) ([]string, error) {
	var ids []string
	switch source {
	case modelDiscoveryOpenAI, modelDiscoveryAnthropic:
		ids = parseObjectModelIDs(body)
	case modelDiscoveryGemini:
		ids = parseGeminiModelIDs(body)
	default:
		return nil, errModelDiscoveryUnsupported
	}
	ids = normalizeModelIDs(ids, limit)
	if len(ids) == 0 {
		return nil, errModelDiscoveryUpstream
	}
	return ids, nil
}

func parseObjectModelIDs(body []byte) []string {
	var decoded struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	ids := make([]string, 0, len(decoded.Data)+len(decoded.Models))
	for _, model := range decoded.Data {
		ids = append(ids, firstNonEmpty(model.ID, model.Name))
	}
	for _, model := range decoded.Models {
		ids = append(ids, firstNonEmpty(model.ID, model.Name))
	}
	return ids
}

func parseGeminiModelIDs(body []byte) []string {
	var decoded struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	ids := make([]string, 0, len(decoded.Models))
	for _, model := range decoded.Models {
		ids = append(ids, strings.TrimPrefix(strings.TrimSpace(model.Name), "models/"))
	}
	return ids
}

func normalizeModelIDs(values []string, limit int) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		id := normalizeDiscoveredModelID(value)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizeDiscoveredModelID(value string) string {
	id := strings.TrimSpace(value)
	id = strings.TrimPrefix(id, "models/")
	id = strings.TrimSpace(id)
	return id
}
