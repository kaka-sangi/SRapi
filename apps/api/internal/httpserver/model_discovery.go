package httpserver

import (
	"bytes"
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

	"regexp"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
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
	modelDiscoveryOpenAI      modelDiscoverySource = "openai-compatible"
	modelDiscoveryAnthropic   modelDiscoverySource = "anthropic-compatible"
	modelDiscoveryGemini      modelDiscoverySource = "gemini-compatible"
	modelDiscoveryAntigravity modelDiscoverySource = "reverse-proxy-antigravity"
	modelDiscoveryChatGPTWeb  modelDiscoverySource = "reverse-proxy-chatgpt-web"
)

type modelDiscoveryHTTPRequest struct {
	Method          string
	Endpoint        string
	RequestURL      string
	Headers         http.Header
	Body            []byte
	ViaReverseProxy bool
}

type antigravityProjectBootstrap struct {
	ProjectID    string
	Bootstrapped bool
	Endpoint     string
}

func (rt *runtimeState) discoverAccountModels(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, req apiopenapi.DiscoverAccountModelsRequest) (apiopenapi.AccountModelDiscovery, error) {
	source, ok := modelDiscoverySourceForProvider(provider)
	if !ok {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryUnsupported
	}
	if !modelDiscoveryRuntimeSupported(source, account) {
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
	bootstrap := antigravityProjectBootstrap{}
	if source == modelDiscoveryAntigravity {
		bootstrap, err = rt.ensureAntigravityDiscoveryProject(ctx, provider, account, credential)
		if err != nil {
			return apiopenapi.AccountModelDiscovery{}, err
		}
	}
	discoveryReq, err := modelDiscoveryRequest(source, provider, account, credential, bootstrap.ProjectID)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, err
	}
	if discoveryReq.Endpoint == "" {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryInvalidInput
	}
	if !validModelDiscoveryEndpoint(discoveryReq.Endpoint) {
		return apiopenapi.AccountModelDiscovery{}, errModelDiscoveryInvalidInput
	}
	body, err := rt.executeModelDiscoveryRequest(ctx, account, credential, discoveryReq)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, err
	}
	modelIDs, err := parseDiscoveredModelIDs(source, body, limit)
	if err != nil {
		return apiopenapi.AccountModelDiscovery{}, err
	}

	checkedAt := time.Now().UTC()
	persisted := req.Persist != nil && *req.Persist
	if persisted {
		metadata := cloneMetadata(account.Metadata)
		if len(modelIDs) > 0 {
			metadata["supported_models"] = append([]string(nil), modelIDs...)
		} else {
			delete(metadata, "supported_models")
		}
		metadata["model_discovery_source"] = string(source)
		metadata["model_discovery_endpoint"] = discoveryReq.Endpoint
		metadata["model_discovery_last_seen_at"] = checkedAt.Format(time.RFC3339)
		if source == modelDiscoveryAntigravity && bootstrap.ProjectID != "" {
			metadata["project_id"] = bootstrap.ProjectID
			metadata["antigravity_project_id"] = bootstrap.ProjectID
			metadata["cloudaicompanion_project"] = bootstrap.ProjectID
			if bootstrap.Bootstrapped {
				metadata["antigravity_project_bootstrapped_at"] = checkedAt.Format(time.RFC3339)
			}
			if bootstrap.Endpoint != "" {
				metadata["antigravity_project_bootstrap_endpoint"] = bootstrap.Endpoint
			}
		}
		if _, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata}); err != nil {
			return apiopenapi.AccountModelDiscovery{}, err
		}
		if source == modelDiscoveryChatGPTWeb {
			rt.ensureDiscoveredModelsRegistered(ctx, provider, modelIDs)
		}
	}

	return apiopenapi.AccountModelDiscovery{
		AccountId:  apiopenapi.Id(strconv.Itoa(account.ID)),
		CheckedAt:  checkedAt,
		Endpoint:   discoveryReq.Endpoint,
		ModelIds:   modelIDs,
		Persisted:  persisted,
		ProviderId: apiopenapi.Id(strconv.Itoa(provider.ID)),
		Source:     apiopenapi.AccountModelDiscoverySource(source),
	}, nil
}

func modelDiscoveryRuntimeSupported(source modelDiscoverySource, account accountcontract.ProviderAccount) bool {
	if source == modelDiscoveryAntigravity || source == modelDiscoveryChatGPTWeb {
		return account.RuntimeClass != accountcontract.RuntimeClassAPIKey
	}
	return account.RuntimeClass == accountcontract.RuntimeClassAPIKey
}

func modelDiscoveryRequest(source modelDiscoverySource, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, antigravityProjectID string) (modelDiscoveryHTTPRequest, error) {
	endpoint := modelDiscoveryEndpoint(source, provider, account)
	if endpoint == "" {
		return modelDiscoveryHTTPRequest{}, errModelDiscoveryInvalidInput
	}
	if !validModelDiscoveryEndpoint(endpoint) {
		return modelDiscoveryHTTPRequest{}, errModelDiscoveryInvalidInput
	}
	requestEndpoint := endpoint
	headers, err := modelDiscoveryHeaders(source, provider, account, credential, &requestEndpoint)
	if err != nil {
		return modelDiscoveryHTTPRequest{}, err
	}
	out := modelDiscoveryHTTPRequest{
		Method:     http.MethodGet,
		Endpoint:   endpoint,
		RequestURL: requestEndpoint,
		Headers:    headers,
	}
	if source == modelDiscoveryAntigravity {
		body, err := antigravityModelDiscoveryBody(provider, account, credential, antigravityProjectID)
		if err != nil {
			return modelDiscoveryHTTPRequest{}, err
		}
		out.Method = http.MethodPost
		out.Body = body
		out.ViaReverseProxy = true
	}
	if source == modelDiscoveryChatGPTWeb {
		out.ViaReverseProxy = true
	}
	return out, nil
}

func (rt *runtimeState) executeModelDiscoveryRequest(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, req modelDiscoveryHTTPRequest) ([]byte, error) {
	if req.ViaReverseProxy {
		if err := rt.materializeProviderProxy(ctx, &account); err != nil {
			return nil, errModelDiscoveryUpstream
		}
		if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, account, credential); err != nil {
			return nil, errModelDiscoveryUpstream
		} else if ok {
			credential = refreshed
		}
		resp, err := rt.reverseProxy.Do(ctx, reverseproxycontract.Request{
			Account: antigravityModelDiscoveryRuntime(account, credential),
			Method:  req.Method,
			URL:     req.modelDiscoveryRequestURL(),
			Headers: req.Headers,
			Body:    req.Body,
		})
		if err != nil {
			return nil, errModelDiscoveryUpstream
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, errModelDiscoveryUpstream
		}
		return resp.Body, nil
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.modelDiscoveryRequestURL(), bytes.NewReader(req.Body))
	if err != nil {
		return nil, errModelDiscoveryInvalidInput
	}
	httpReq.Header = req.Headers

	client := &http.Client{Timeout: rt.cfg.Gateway.RequestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, errModelDiscoveryUpstream
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelDiscoveryBody))
	if err != nil {
		return nil, errModelDiscoveryUpstream
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errModelDiscoveryUpstream
	}
	return body, nil
}

func (req modelDiscoveryHTTPRequest) modelDiscoveryRequestURL() string {
	if strings.TrimSpace(req.RequestURL) != "" {
		return req.RequestURL
	}
	return req.Endpoint
}

func modelDiscoverySourceForProvider(provider providercontract.Provider) (modelDiscoverySource, bool) {
	switch strings.ToLower(strings.TrimSpace(provider.AdapterType)) {
	case "reverse-proxy-antigravity":
		return modelDiscoveryAntigravity, true
	case "reverse-proxy-chatgpt-web", "reverse-proxy-codex-cli":
		return modelDiscoveryChatGPTWeb, true
	}
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
	switch source {
	case modelDiscoveryAntigravity:
		if strings.HasSuffix(baseURL, "/v1internal:fetchAvailableModels") {
			return baseURL
		}
		return strings.TrimRight(baseURL, "/") + "/v1internal:fetchAvailableModels"
	case modelDiscoveryChatGPTWeb:
		parsed, err := url.Parse(baseURL)
		if err != nil {
			return ""
		}
		parsed.Path = "/backend-api/models"
		parsed.RawQuery = "history_and_training_disabled=false"
		return parsed.String()
	default:
		if strings.HasSuffix(baseURL, "/models") {
			return baseURL
		}
		return strings.TrimRight(baseURL, "/") + "/models"
	}
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
	case modelDiscoveryAntigravity:
		keys = append([]string{"antigravity_models_url", "antigravity_base_url"}, keys...)
	case modelDiscoveryChatGPTWeb:
		keys = append([]string{"chatgpt_models_url"}, keys...)
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
	if source == modelDiscoveryAntigravity {
		if mapString(credential, "access_token") == "" {
			return nil, errModelDiscoveryAuth
		}
		headers.Set("Content-Type", "application/json")
		return headers, nil
	}
	if source == modelDiscoveryChatGPTWeb {
		if mapString(credential, "access_token") == "" {
			return nil, errModelDiscoveryAuth
		}
		chatGPTWebModelDiscoveryHeaders(headers, provider, account)
		return headers, nil
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
	var (
		ids []string
		ok  bool
	)
	switch source {
	case modelDiscoveryOpenAI, modelDiscoveryAnthropic:
		ids, ok = parseObjectModelIDs(body)
	case modelDiscoveryGemini:
		ids, ok = parseGeminiModelIDs(body)
	case modelDiscoveryAntigravity:
		ids, ok = parseAntigravityModelIDs(body)
	case modelDiscoveryChatGPTWeb:
		ids, ok = parseChatGPTWebModelIDs(body)
	default:
		return nil, errModelDiscoveryUnsupported
	}
	// Only an UNRECOGNIZED/malformed body (e.g. a 200 error payload) is an
	// upstream failure. A recognized response that simply lists no models is a
	// valid "no models available" result, not a 502.
	if !ok {
		return nil, errModelDiscoveryUpstream
	}
	return normalizeModelIDs(ids, limit), nil
}

func parseObjectModelIDs(body []byte) ([]string, bool) {
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
		return nil, false
	}
	// Neither key present -> not a recognized model-listing envelope.
	if decoded.Data == nil && decoded.Models == nil {
		return nil, false
	}
	ids := make([]string, 0, len(decoded.Data)+len(decoded.Models))
	for _, model := range decoded.Data {
		ids = append(ids, firstNonEmpty(model.ID, model.Name))
	}
	for _, model := range decoded.Models {
		ids = append(ids, firstNonEmpty(model.ID, model.Name))
	}
	return ids, true
}

func parseChatGPTWebModelIDs(body []byte) ([]string, bool) {
	var decoded struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, false
	}
	if decoded.Models == nil {
		return nil, false
	}
	ids := make([]string, 0, len(decoded.Models))
	for _, m := range decoded.Models {
		if slug := strings.TrimSpace(m.Slug); slug != "" {
			ids = append(ids, slug)
		}
	}
	return ids, true
}

func parseGeminiModelIDs(body []byte) ([]string, bool) {
	var decoded struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, false
	}
	if decoded.Models == nil {
		return nil, false
	}
	ids := make([]string, 0, len(decoded.Models))
	for _, model := range decoded.Models {
		ids = append(ids, strings.TrimPrefix(strings.TrimSpace(model.Name), "models/"))
	}
	return ids, true
}

func parseAntigravityModelIDs(body []byte) ([]string, bool) {
	var decoded struct {
		Models json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Models) == 0 {
		return nil, false
	}
	// `models` may be an object map {id: {...}} or an array [{id,...}].
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(decoded.Models, &asObject); err == nil && asObject != nil {
		ids := make([]string, 0, len(asObject))
		for id := range asObject {
			if !isInternalAntigravityModelID(id) {
				ids = append(ids, id)
			}
		}
		return ids, true
	}
	var asArray []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(decoded.Models, &asArray); err != nil {
		return nil, false
	}
	ids := make([]string, 0, len(asArray))
	for _, model := range asArray {
		id := firstNonEmpty(model.ID, model.Name)
		if !isInternalAntigravityModelID(id) {
			ids = append(ids, id)
		}
	}
	return ids, true
}

func isInternalAntigravityModelID(value string) bool {
	switch strings.TrimSpace(value) {
	case "chat_20706", "chat_23310", "tab_flash_lite_preview", "tab_jump_flash_lite_preview", "gemini-2.5-flash-thinking", "gemini-2.5-pro":
		return true
	default:
		return false
	}
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

// ensureDiscoveredModelsRegistered creates Model records and ModelProviderMappings
// for web slugs discovered from ChatGPT's /backend-api/models. Each slug (e.g.
// "gpt-5-2") is converted to a canonical name (e.g. "gpt-5.2") for the Model
// record, and the original slug is preserved as UpstreamModelName in the mapping.
func (rt *runtimeState) ensureDiscoveredModelsRegistered(ctx context.Context, provider providercontract.Provider, slugs []string) {
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" || slug == "auto" || slug == "research" {
			continue
		}
		canonical := chatGPTWebSlugToCanonical(slug)
		model, err := rt.models.FindByCanonicalName(ctx, canonical)
		if err != nil {
			model, err = rt.models.Create(ctx, modelcontract.CreateRequest{
				CanonicalName: canonical,
				DisplayName:   canonical,
			})
			if err != nil {
				model, _ = rt.models.FindByCanonicalName(ctx, canonical)
			}
		}
		if model.ID == 0 {
			continue
		}
		rt.models.CreateMapping(ctx, model.ID, modelcontract.CreateMappingRequest{
			ProviderID:        provider.ID,
			UpstreamModelName: slug,
		})
	}
}

// chatGPTWebSlugToCanonical converts a ChatGPT Web slug to the canonical
// model name used by SRapi. The web UI uses dashes (gpt-5-2) while the API
// uses dots (gpt-5.2) for the version separator.
//
//	gpt-5-2       → gpt-5.2
//	gpt-5-4-t-mini → gpt-5.4-t-mini
//	gpt-5-mini    → gpt-5-mini    (no minor version digit, kept as-is)
//	auto          → auto
var chatGPTWebVersionPattern = regexp.MustCompile(`^(gpt-\d+)-(\d+)(.*)$`)

func chatGPTWebSlugToCanonical(slug string) string {
	if m := chatGPTWebVersionPattern.FindStringSubmatch(slug); m != nil {
		return m[1] + "." + m[2] + m[3]
	}
	return slug
}

func chatGPTWebModelDiscoveryHeaders(headers http.Header, provider providercontract.Provider, account accountcontract.ProviderAccount) {
	baseURL := mapString(account.Metadata, "base_url")
	if baseURL == "" {
		baseURL = "https://chatgpt.com"
	}
	origin := strings.TrimRight(baseURL, "/")
	path := "/backend-api/models"

	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	headers.Set("Origin", origin)
	headers.Set("Referer", origin+"/")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Pragma", "no-cache")
	headers.Set("Sec-Ch-Ua", firstNonEmpty(mapString(account.Metadata, "sec_ch_ua"), `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`))
	headers.Set("Sec-Ch-Ua-Mobile", firstNonEmpty(mapString(account.Metadata, "sec_ch_ua_mobile"), "?0"))
	headers.Set("Sec-Ch-Ua-Platform", firstNonEmpty(mapString(account.Metadata, "sec_ch_ua_platform"), `"Windows"`))
	headers.Set("Sec-Fetch-Dest", "empty")
	headers.Set("Sec-Fetch-Mode", "cors")
	headers.Set("Sec-Fetch-Site", "same-origin")
	headers.Set("X-OpenAI-Target-Path", path)
	headers.Set("X-OpenAI-Target-Route", path)
	if accountID := firstNonEmpty(mapString(account.Metadata, "chatgpt_account_id"), mapString(account.Metadata, "upstream_account_id")); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
}
