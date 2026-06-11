package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// FetchAccountQuota performs an active, out-of-band quota/subscription fetch for
// an account. It is config-driven: the quota endpoint and field paths come from
// provider config / account metadata, so each provider (Codex, Antigravity,
// Gemini CLI, …) is supported by configuration rather than hardcoded API shapes.
// When no quota endpoint is configured it returns a report with Supported=false.
func (s *Service) FetchAccountQuota(ctx context.Context, req contract.ProbeRequest) (contract.QuotaReport, error) {
	if req.Account.ID <= 0 || req.Provider.ID <= 0 {
		return contract.QuotaReport{}, ErrInvalidInput
	}
	if s.quotaCache == nil {
		s.quotaCache = newQuotaFetchCache()
	}
	now := time.Now().UTC()
	report := contract.QuotaReport{
		Provider:  req.Provider.Name,
		Supported: false,
		Source:    "none",
		FetchedAt: now,
	}
	endpoint := quotaEndpoint(req)
	if endpoint == "" {
		return report, nil
	}
	cacheKey := quotaCacheKey(req, endpoint)
	if cached, ok := s.quotaCache.get(cacheKey, now); ok {
		return cached.report, cached.err
	}
	return s.quotaCache.do(cacheKey, func() (contract.QuotaReport, error) {
		return s.fetchAccountQuotaUncached(ctx, req, report, endpoint, now)
	})
}

func (s *Service) fetchAccountQuotaUncached(ctx context.Context, req contract.ProbeRequest, report contract.QuotaReport, endpoint string, now time.Time) (contract.QuotaReport, error) {
	headers, err := quotaHeaders(req, &endpoint)
	if err != nil {
		return report, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "quota fetch auth failed"}
	}
	mergeQuotaHeaders(headers, req)
	method := strings.ToUpper(firstMapString(quotaConfigMaps(req), "quota_method", "subscription_method"))
	if method == "" {
		method = http.MethodGet
	}
	status, body, respHeaders, err := s.doQuotaFetch(ctx, req.Account, method, endpoint, headers)
	if err != nil {
		return report, err
	}
	report.StatusCode = status
	if status < 200 || status >= 300 {
		providerErr := classifyProviderHTTPErrorWithHeaders(status, respHeaders, body)
		return report, providerErr
	}
	report.Supported = true
	report.Source = "endpoint"

	values := quotaConfigMaps(req)
	parsed := decodeQuotaJSON(body)
	if parsed != nil {
		report.Plan = quotaFieldFromPaths(parsed, values, "quota_plan_path", "subscription_plan_path")
		report.CreditsRemaining = quotaFieldFromPaths(parsed, values, "quota_credits_remaining_path", "credits_remaining_path")
		report.CreditsUsed = quotaFieldFromPaths(parsed, values, "quota_credits_used_path", "credits_used_path")
		report.CreditsLimit = quotaFieldFromPaths(parsed, values, "quota_credits_limit_path", "credits_limit_path")
		report.Currency = quotaFieldFromPaths(parsed, values, "quota_currency_path", "credits_currency_path")
		if isCodexQuotaProbe(req) {
			applyCodexAccountsCheckQuotaFallback(parsed, req, &report)
		}
		if isAntigravityQuotaProbe(req) {
			applyAntigravityCreditsQuotaFallback(parsed, &report)
		}
	}

	// Fold in provider header signals (e.g. Codex rate-limit windows) so the
	// report carries the same QuotaSignals the gateway records in-band.
	report.QuotaSignals = codexQuotaSignalsFromHeaders(respHeaders, now)
	report.QuotaSignals = append(report.QuotaSignals, anthropicQuotaSignalsFromUsageBody(parsed, now)...)
	return report, nil
}

func (s *Service) doQuotaFetch(ctx context.Context, account accountcontract.ProviderAccount, method, endpoint string, headers http.Header) (int, []byte, http.Header, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(nil))
	if err != nil {
		return 0, nil, nil, err
	}
	httpReq.Header = headers
	resp, err := s.egressHTTPClient(account, nil).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, nil, nil, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "quota fetch timed out"}
		}
		return 0, nil, nil, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "quota fetch failed"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeResponseBytes))
	if err != nil {
		return 0, nil, nil, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "quota fetch response read failed"}
	}
	return resp.StatusCode, body, resp.Header, nil
}

// QuotaConfigured reports whether a quota endpoint is configured for the account
// or provider, without needing the credential.
func (s *Service) QuotaConfigured(req contract.ProbeRequest) bool {
	return quotaEndpoint(req) != ""
}

func quotaHeaders(req contract.ProbeRequest, endpoint *string) (http.Header, error) {
	if isCodexQuotaProbe(req) {
		return codexQuotaHeaders(req)
	}
	if probeSource(req) == "anthropic" {
		accessToken := firstCredentialString(req.Credential, "access_token", "oauth_access_token")
		if accessToken != "" {
			headers := http.Header{"Accept": {"application/json"}}
			headers.Set("Authorization", "Bearer "+accessToken)
			version := firstMapString(append([]map[string]any{req.Credential}, quotaConfigMaps(req)...), "anthropic_version", "anthropic-version")
			if version == "" {
				version = "2023-06-01"
			}
			headers.Set("anthropic-version", version)
			return headers, nil
		}
	}
	return probeHeaders(req, endpoint)
}

func codexQuotaHeaders(req contract.ProbeRequest) (http.Header, error) {
	accessToken := firstCredentialString(req.Credential, "access_token", "oauth_access_token", "cli_client_token")
	sessionCookie := firstCredentialString(req.Credential, "session_cookie", "cookie")
	headers := http.Header{"Accept": {"application/json"}}
	if accessToken != "" {
		headers.Set("Authorization", "Bearer "+accessToken)
	} else if sessionCookie != "" {
		headers.Set("Cookie", sessionCookie)
	} else {
		return nil, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider access token missing"}
	}
	headers.Set("Origin", "https://chatgpt.com")
	headers.Set("Referer", "https://chatgpt.com/")
	headers.Set("User-Agent", firstNonEmpty(codexQuotaSetting(req, "user_agent"), codexDefaultUserAgent))
	if accountID := codexQuotaSetting(req, "chatgpt_account_id", "account_id"); accountID != "" {
		headers.Set("ChatGPT-Account-ID", accountID)
	}
	return headers, nil
}

func quotaConfigMaps(req contract.ProbeRequest) []map[string]any {
	return []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}
}

func mergeQuotaHeaders(headers http.Header, req contract.ProbeRequest) {
	raw := firstMapValue(quotaConfigMaps(req), "quota_headers", "subscription_headers")
	switch value := raw.(type) {
	case map[string]string:
		for key, item := range value {
			setQuotaHeader(headers, key, expandQuotaHeaderValue(item, req))
		}
	case map[string]any:
		for key, item := range value {
			setQuotaHeader(headers, key, expandQuotaHeaderValue(mapString(map[string]any{"value": item}, "value"), req))
		}
	case string:
		var parsed map[string]string
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			for key, item := range parsed {
				setQuotaHeader(headers, key, expandQuotaHeaderValue(item, req))
			}
		}
	}
}

func setQuotaHeader(headers http.Header, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" || quotaHeaderForbidden(key) {
		return
	}
	headers.Set(key, value)
}

func quotaHeaderForbidden(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization", "cookie", "set-cookie", "host", "content-length", "connection", "transfer-encoding", "upgrade", "proxy-authorization", "proxy-authenticate":
		return true
	default:
		return false
	}
}

func expandQuotaHeaderValue(value string, req contract.ProbeRequest) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{{") || !strings.HasSuffix(value, "}}") {
		return value
	}
	key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{{"), "}}"))
	if key == "" {
		return ""
	}
	return firstMapString(append([]map[string]any{req.Credential}, quotaConfigMaps(req)...), key)
}

func quotaEndpoint(req contract.ProbeRequest) string {
	endpoint := firstMapString(quotaConfigMaps(req), "quota_url", "subscription_url", "credits_url", "quota_endpoint")
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint != "" {
		return endpoint
	}
	if isCodexQuotaProbe(req) {
		return codexAccountsCheckEndpoint(req)
	}
	return ""
}

func quotaFieldFromPaths(parsed any, values []map[string]any, keys ...string) string {
	path := firstMapString(values, keys...)
	if path == "" {
		return ""
	}
	return probeJSONPathString(parsed, path)
}

func decodeQuotaJSON(body []byte) any {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return parsed
}

func applyCodexAccountsCheckQuotaFallback(parsed any, req contract.ProbeRequest, report *contract.QuotaReport) {
	if report == nil || strings.TrimSpace(report.Plan) != "" {
		return
	}
	root, ok := parsed.(map[string]any)
	if !ok {
		return
	}
	rawAccounts, ok := root["accounts"].(map[string]any)
	if !ok {
		return
	}
	account := selectCodexAccountsCheckAccount(rawAccounts, req)
	if account == nil {
		return
	}
	report.Plan = codexAccountsCheckPlan(account)
}

func selectCodexAccountsCheckAccount(accounts map[string]any, req contract.ProbeRequest) map[string]any {
	if len(accounts) == 0 {
		return nil
	}
	candidateIDs := codexAccountsCheckCandidateIDs(req)
	for _, candidateID := range candidateIDs {
		if account, ok := mapStringAccount(accounts[candidateID]); ok && codexAccountsCheckPlan(account) != "" {
			return account
		}
	}
	keys := sortedMapKeys(accounts)
	for _, candidateID := range candidateIDs {
		for _, key := range keys {
			account, ok := mapStringAccount(accounts[key])
			if ok && codexAccountsCheckPlan(account) != "" && codexAccountsCheckAccountMatches(account, candidateID) {
				return account
			}
		}
	}

	var defaultAccount map[string]any
	var paidAccount map[string]any
	var anyAccount map[string]any
	for _, key := range keys {
		account, ok := mapStringAccount(accounts[key])
		if !ok {
			continue
		}
		plan := codexAccountsCheckPlan(account)
		if plan == "" {
			continue
		}
		if anyAccount == nil {
			anyAccount = account
		}
		if paidAccount == nil && !strings.EqualFold(plan, "free") {
			paidAccount = account
		}
		if nestedBoolField(account, "account", "is_default") {
			defaultAccount = account
		}
	}
	switch {
	case defaultAccount != nil:
		return defaultAccount
	case paidAccount != nil:
		return paidAccount
	default:
		return anyAccount
	}
}

func codexAccountsCheckCandidateIDs(req contract.ProbeRequest) []string {
	values := append([]map[string]any{req.Credential}, quotaConfigMaps(req)...)
	seen := map[string]bool{}
	out := []string{}
	for _, valuesMap := range values {
		for _, key := range []string{"chatgpt_account_id", "account_id", "organization_id", "org_id", "poid"} {
			value := mapString(valuesMap, key)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func codexAccountsCheckAccountMatches(account map[string]any, candidateID string) bool {
	if strings.TrimSpace(candidateID) == "" {
		return false
	}
	for _, key := range []string{"id", "account_id"} {
		if mapString(account, key) == candidateID {
			return true
		}
	}
	nested, ok := account["account"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"id", "account_id"} {
		if mapString(nested, key) == candidateID {
			return true
		}
	}
	return false
}

func codexAccountsCheckPlan(account map[string]any) string {
	if nested, ok := account["account"].(map[string]any); ok {
		if plan := mapString(nested, "plan_type"); plan != "" {
			return plan
		}
	}
	if nested, ok := account["entitlement"].(map[string]any); ok {
		if plan := mapString(nested, "subscription_plan"); plan != "" {
			return plan
		}
	}
	return ""
}

func nestedBoolField(value map[string]any, objectKey string, fieldKey string) bool {
	nested, ok := value[objectKey].(map[string]any)
	if !ok {
		return false
	}
	raw, ok := nested[fieldKey].(bool)
	return ok && raw
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mapStringAccount(value any) (map[string]any, bool) {
	account, ok := value.(map[string]any)
	return account, ok
}

func isCodexQuotaProbe(req contract.ProbeRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-codex-cli")
}

func isAntigravityQuotaProbe(req contract.ProbeRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-antigravity")
}

func applyAntigravityCreditsQuotaFallback(parsed any, report *contract.QuotaReport) {
	if report == nil {
		return
	}
	root, ok := parsed.(map[string]any)
	if !ok {
		return
	}
	paidTier, ok := root["paidTier"].(map[string]any)
	if !ok {
		if plan := mapString(root, "paidTier"); plan != "" && strings.TrimSpace(report.Plan) == "" {
			report.Plan = plan
		}
		return
	}
	if strings.TrimSpace(report.Plan) == "" {
		report.Plan = firstNonEmpty(mapString(paidTier, "id"), mapString(paidTier, "name"))
	}
	if strings.TrimSpace(report.CreditsRemaining) != "" {
		return
	}
	credit := antigravityCreditRecord(paidTier["availableCredits"])
	if credit == nil {
		return
	}
	if amount := mapString(credit, "creditAmount"); amount != "" {
		report.CreditsRemaining = amount
	}
	if currency := mapString(credit, "creditType"); currency != "" && strings.TrimSpace(report.Currency) == "" {
		report.Currency = currency
	}
	if strings.TrimSpace(report.CreditsLimit) == "" {
		report.CreditsLimit = firstNonEmpty(mapString(credit, "creditLimit"), mapString(credit, "limit"))
	}
	if strings.TrimSpace(report.CreditsUsed) == "" {
		report.CreditsUsed = firstNonEmpty(mapString(credit, "creditUsed"), mapString(credit, "used"))
	}
}

func antigravityCreditRecord(value any) map[string]any {
	credits, ok := value.([]any)
	if !ok || len(credits) == 0 {
		return nil
	}
	var first map[string]any
	for _, raw := range credits {
		credit, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if first == nil {
			first = credit
		}
		if strings.EqualFold(mapString(credit, "creditType"), "GOOGLE_ONE_AI") {
			return credit
		}
	}
	return first
}

func codexAccountsCheckEndpoint(req contract.ProbeRequest) string {
	const defaultEndpoint = "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27"
	baseURL := firstMapString(quotaConfigMaps(req), "base_url", "upstream_base_url")
	if baseURL == "" {
		return defaultEndpoint
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return defaultEndpoint
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/backend-api/codex"):
		parsed.Path = strings.TrimSuffix(path, "/codex") + "/accounts/check/v4-2023-04-27"
	case path == "/backend-api":
		parsed.Path = path + "/accounts/check/v4-2023-04-27"
	case strings.HasPrefix(path, "/backend-api/"):
		parsed.Path = "/backend-api/accounts/check/v4-2023-04-27"
	case path == "" || path == "/":
		parsed.Path = "/backend-api/accounts/check/v4-2023-04-27"
	default:
		if !strings.EqualFold(parsed.Host, "chatgpt.com") {
			return defaultEndpoint
		}
		parsed.Path = "/backend-api/accounts/check/v4-2023-04-27"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func codexQuotaSetting(req contract.ProbeRequest, keys ...string) string {
	values := append([]map[string]any{req.Credential}, quotaConfigMaps(req)...)
	return firstMapString(values, keys...)
}

type quotaFetchCache struct {
	mu       sync.Mutex
	entries  map[string]quotaFetchCacheEntry
	inflight map[string]*quotaFetchCall
}

type quotaFetchCacheEntry struct {
	report    contract.QuotaReport
	err       error
	expiresAt time.Time
}

type quotaFetchCall struct {
	done   chan struct{}
	report contract.QuotaReport
	err    error
}

func newQuotaFetchCache() *quotaFetchCache {
	return &quotaFetchCache{
		entries:  map[string]quotaFetchCacheEntry{},
		inflight: map[string]*quotaFetchCall{},
	}
}

func (c *quotaFetchCache) get(key string, now time.Time) (quotaFetchCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !entry.expiresAt.After(now) {
		return quotaFetchCacheEntry{}, false
	}
	return entry, true
}

func (c *quotaFetchCache) do(key string, fn func() (contract.QuotaReport, error)) (contract.QuotaReport, error) {
	c.mu.Lock()
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.report, call.err
	}
	call := &quotaFetchCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.report, call.err = fn()
	ttl := successfulQuotaCacheTTL
	if call.err != nil {
		ttl = failedQuotaCacheTTL
	}
	c.mu.Lock()
	c.entries[key] = quotaFetchCacheEntry{report: call.report, err: call.err, expiresAt: time.Now().UTC().Add(quotaCacheTTL(key, ttl))}
	delete(c.inflight, key)
	close(call.done)
	c.mu.Unlock()
	return call.report, call.err
}

const (
	successfulQuotaCacheTTL = 30 * time.Second
	failedQuotaCacheTTL     = 10 * time.Second
)

func quotaCacheKey(req contract.ProbeRequest, endpoint string) string {
	return strconv.Itoa(req.Provider.ID) + ":" + strconv.Itoa(req.Account.ID) + ":" + strings.ToUpper(quotaMethod(req)) + ":" + endpoint
}

func quotaMethod(req contract.ProbeRequest) string {
	method := strings.ToUpper(firstMapString(quotaConfigMaps(req), "quota_method", "subscription_method"))
	if method == "" {
		return http.MethodGet
	}
	return method
}

func quotaCacheTTL(key string, base time.Duration) time.Duration {
	jitterWindow := base / 5
	if jitterWindow <= 0 {
		return base
	}
	jitter := time.Duration(crc32.ChecksumIEEE([]byte(key)) % uint32(jitterWindow))
	return base + jitter
}

func anthropicQuotaSignalsFromUsageBody(parsed any, now time.Time) []contract.QuotaSignal {
	if parsed == nil {
		return nil
	}
	windows := []struct {
		quotaType string
		keys      []string
	}{
		{quotaType: "anthropic_5h", keys: []string{"five_hour", "5h", "fiveHour"}},
		{quotaType: "anthropic_7d", keys: []string{"seven_day", "7d", "sevenDay"}},
		{quotaType: "anthropic_7d_sonnet", keys: []string{"seven_day_sonnet", "7d_sonnet", "sevenDaySonnet"}},
	}
	signals := make([]contract.QuotaSignal, 0, len(windows))
	for _, window := range windows {
		value, ok := nestedMapAny(parsed, window.keys...)
		if !ok {
			continue
		}
		signal, ok := anthropicQuotaSignalFromWindow(window.quotaType, value, now)
		if ok {
			signals = append(signals, signal)
		}
	}
	return signals
}

func anthropicQuotaSignalFromWindow(quotaType string, value any, now time.Time) (contract.QuotaSignal, bool) {
	utilization, ok := numericField(value, "utilization", "used_ratio", "usage_ratio")
	if !ok {
		used, usedOK := numericField(value, "used", "usage", "consumed")
		limit, limitOK := numericField(value, "limit", "quota", "quota_limit")
		if usedOK && limitOK && limit > 0 {
			utilization = used / limit
			ok = true
		}
	}
	if !ok || math.IsNaN(utilization) || math.IsInf(utilization, 0) {
		return contract.QuotaSignal{}, false
	}
	usedRatio := clampFloat(utilization, 0, 1)
	remainingRatio := 1 - usedRatio
	resetAt := optionalTimeField(value, "resets_at", "reset_at", "resetAt")
	return contract.QuotaSignal{
		QuotaType:      quotaType,
		Remaining:      formatPercentQuotaValue(remainingRatio * 100),
		Used:           formatPercentQuotaValue(usedRatio * 100),
		QuotaLimit:     "100",
		RemainingRatio: float32(remainingRatio),
		ResetAt:        resetAt,
		SnapshotAt:     now.UTC(),
	}, true
}

func nestedMapAny(value any, keys ...string) (any, bool) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	for _, key := range keys {
		if raw, ok := root[key]; ok {
			return raw, true
		}
	}
	for _, raw := range root {
		if found, ok := nestedMapAny(raw, keys...); ok {
			return found, true
		}
	}
	return nil, false
}

func numericField(value any, keys ...string) (float64, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return 0, false
	}
	for _, key := range keys {
		if raw, ok := object[key]; ok {
			switch typed := raw.(type) {
			case float64:
				return typed, true
			case int:
				return float64(typed), true
			case json.Number:
				parsed, err := typed.Float64()
				return parsed, err == nil
			case string:
				parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
				return parsed, err == nil
			}
		}
	}
	return 0, false
}

func optionalTimeField(value any, keys ...string) *time.Time {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range keys {
		raw, ok := object[key]
		if !ok {
			continue
		}
		text, ok := raw.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
		if err != nil {
			continue
		}
		parsed = parsed.UTC()
		return &parsed
	}
	return nil
}
