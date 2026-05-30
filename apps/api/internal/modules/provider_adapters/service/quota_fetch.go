package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
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
	headers, err := probeHeaders(req, &endpoint)
	if err != nil {
		return report, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "quota fetch auth failed"}
	}
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
		providerErr := classifyProviderHTTPError(status, body)
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
	}

	// Fold in provider header signals (e.g. Codex rate-limit windows) so the
	// report carries the same QuotaSignals the gateway records in-band.
	report.QuotaSignals = codexQuotaSignalsFromHeaders(respHeaders, now)
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

func quotaConfigMaps(req contract.ProbeRequest) []map[string]any {
	return []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities}
}

func quotaEndpoint(req contract.ProbeRequest) string {
	endpoint := firstMapString(quotaConfigMaps(req), "quota_url", "subscription_url", "credits_url", "quota_endpoint")
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
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
