package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func antigravityModelDiscoveryBody(provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, projectID string) ([]byte, error) {
	payload := map[string]any{}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = modelDiscoverySetting(provider, account, credential, "project_id", "antigravity_project_id", "cloudaicompanion_project")
	}
	if projectID != "" {
		payload["project"] = projectID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, errModelDiscoveryInvalidInput
	}
	return raw, nil
}

// antigravityBootstrapBaseURLs mirrors sub2api's BaseURLs fallback list: prod is
// tried first, the daily sandbox second. Kept identical to the upstream
// (sub2api internal/pkg/antigravity/oauth.go) so the loadCodeAssist/onboardUser
// fallback behavior matches the proven client.
const (
	antigravityProdBaseURL  = "https://cloudcode-pa.googleapis.com"
	antigravityDailyBaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
)

var antigravityBootstrapBaseURLs = []string{
	antigravityProdBaseURL,  // prod (preferred)
	antigravityDailyBaseURL, // daily sandbox (fallback)
}

// shouldFallbackToNextAntigravityURL ports sub2api's shouldFallbackToNextURL:
// connection/transport failures (surfaced here as statusCode == 0) plus
// 429/408/404/5xx trigger advancing to the next base URL. Mirrors
// Antigravity-Manager behavior.
func shouldFallbackToNextAntigravityURL(statusCode int) bool {
	if statusCode == 0 {
		return true
	}
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusNotFound ||
		statusCode >= 500
}

// antigravityBootstrapCandidateURLs builds the ordered base-URL list to try. An
// operator-configured override (existing upstreamModelDiscoveryBaseURL behavior)
// is honored first, then the fixed prod -> daily fallback list, deduped. With no
// override this is exactly sub2api's BaseURLs.
func antigravityBootstrapCandidateURLs(provider providercontract.Provider, account accountcontract.ProviderAccount) []string {
	candidates := make([]string, 0, len(antigravityBootstrapBaseURLs)+1)
	seen := make(map[string]struct{}, len(antigravityBootstrapBaseURLs)+1)
	add := func(raw string) {
		trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		candidates = append(candidates, trimmed)
	}
	add(upstreamModelDiscoveryBaseURL(modelDiscoveryAntigravity, provider, account))
	for _, base := range antigravityBootstrapBaseURLs {
		add(base)
	}
	return candidates
}

func (rt *runtimeState) ensureAntigravityDiscoveryProject(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any) (antigravityProjectBootstrap, error) {
	if projectID := modelDiscoverySetting(provider, account, credential, "project_id", "antigravity_project_id", "cloudaicompanion_project"); projectID != "" {
		return antigravityProjectBootstrap{ProjectID: projectID}, nil
	}
	baseURLs := antigravityBootstrapCandidateURLs(provider, account)
	if len(baseURLs) == 0 {
		return antigravityProjectBootstrap{}, errModelDiscoveryInvalidInput
	}
	if mapString(credential, "access_token") == "" {
		return antigravityProjectBootstrap{}, errModelDiscoveryAuth
	}

	var lastErr error = errModelDiscoveryUpstream
	for urlIdx, baseURL := range baseURLs {
		bootstrap, err, fellBack := rt.bootstrapAntigravityProjectAt(ctx, account, credential, baseURL, urlIdx < len(baseURLs)-1)
		if err == nil {
			return bootstrap, nil
		}
		lastErr = err
		if fellBack {
			// shouldFallbackToNextAntigravityURL matched and a next URL exists:
			// advance to the next base URL.
			continue
		}
		return antigravityProjectBootstrap{}, err
	}
	return antigravityProjectBootstrap{}, lastErr
}

// bootstrapAntigravityProjectAt runs loadCodeAssist (then onboardUser if needed)
// against a single base URL. It returns fellBack=true when the failure is
// fallback-eligible per shouldFallbackToNextAntigravityURL AND a next URL exists
// (hasNext), so the caller advances to the next base URL; otherwise the error is
// terminal.
func (rt *runtimeState) bootstrapAntigravityProjectAt(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, baseURL string, hasNext bool) (antigravityProjectBootstrap, error, bool) {
	loadEndpoint := strings.TrimRight(baseURL, "/") + "/v1internal:loadCodeAssist"
	loadResp, loadStatus, err := rt.executeAntigravityBootstrapRequest(ctx, account, credential, loadEndpoint, antigravityLoadCodeAssistBody(account), false)
	if err != nil {
		return antigravityProjectBootstrap{}, err, hasNext && shouldFallbackToNextAntigravityURL(loadStatus)
	}
	if projectID := extractAntigravityProjectID(loadResp); projectID != "" {
		return antigravityProjectBootstrap{ProjectID: projectID, Bootstrapped: true, Endpoint: loadEndpoint}, nil, false
	}

	onboardEndpoint := strings.TrimRight(baseURL, "/") + "/v1internal:onboardUser"
	onboardBody := antigravityOnboardUserBody(account, antigravityDefaultTierID(loadResp))
	for attempt := 0; attempt < 5; attempt++ {
		onboardResp, onboardStatus, err := rt.executeAntigravityBootstrapRequest(ctx, account, credential, onboardEndpoint, onboardBody, true)
		if err != nil {
			return antigravityProjectBootstrap{}, err, hasNext && shouldFallbackToNextAntigravityURL(onboardStatus)
		}
		if done, ok := onboardResp["done"].(bool); ok && !done {
			continue
		}
		if response, ok := onboardResp["response"].(map[string]any); ok {
			if projectID := extractAntigravityProjectID(response); projectID != "" {
				return antigravityProjectBootstrap{ProjectID: projectID, Bootstrapped: true, Endpoint: onboardEndpoint}, nil, false
			}
		}
		if projectID := extractAntigravityProjectID(onboardResp); projectID != "" {
			return antigravityProjectBootstrap{ProjectID: projectID, Bootstrapped: true, Endpoint: onboardEndpoint}, nil, false
		}
	}
	return antigravityProjectBootstrap{}, errModelDiscoveryUpstream, false
}

func antigravityLoadCodeAssistBody(account accountcontract.ProviderAccount) []byte {
	raw, err := json.Marshal(map[string]any{
		"metadata": map[string]string{
			"ideType":    "ANTIGRAVITY",
			"ideVersion": antigravityVersionFromAccount(account),
			"ideName":    "antigravity",
		},
	})
	if err != nil {
		return []byte(`{"metadata":{"ideType":"ANTIGRAVITY","ideVersion":"1.0","ideName":"antigravity"}}`)
	}
	return raw
}

func antigravityOnboardUserBody(account accountcontract.ProviderAccount, tierID string) []byte {
	raw, err := json.Marshal(map[string]any{
		"tier_id": strings.TrimSpace(tierID),
		"metadata": map[string]string{
			"ide_type":    "ANTIGRAVITY",
			"ide_version": antigravityVersionFromAccount(account),
			"ide_name":    "antigravity",
		},
	})
	if err != nil {
		return []byte(`{"tier_id":"free-tier","metadata":{"ide_type":"ANTIGRAVITY","ide_version":"1.0","ide_name":"antigravity"}}`)
	}
	return raw
}

// executeAntigravityBootstrapRequest dispatches a single bootstrap call and
// returns the decoded body together with the upstream HTTP status code. The
// status code is reported so the caller can apply sub2api's URL-fallback policy
// (shouldFallbackToNextAntigravityURL). A status of 0 denotes a pre-dispatch or
// transport failure, which that policy treats as a connection error.
func (rt *runtimeState) executeAntigravityBootstrapRequest(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, endpoint string, body []byte, nodeClient bool) (map[string]any, int, error) {
	if !validModelDiscoveryEndpoint(endpoint) {
		return nil, 0, errModelDiscoveryInvalidInput
	}
	if err := rt.materializeProviderProxy(ctx, &account); err != nil {
		return nil, 0, errModelDiscoveryUpstream
	}
	// Refresh an expired OAuth/reverse-proxy token before dispatch, mirroring the
	// gateway path — otherwise antigravity bootstrap fails on an expired token.
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, account, credential); err != nil {
		return nil, 0, errModelDiscoveryUpstream
	} else if ok {
		credential = refreshed
	}
	headers := http.Header{
		"Accept":       {"*/*"},
		"Content-Type": {"application/json"},
	}
	if nodeClient {
		headers.Set("X-Goog-Api-Client", "gl-node/22.0.0 gdcl/10.3.0")
	}
	resp, err := rt.reverseProxy.Do(ctx, reverseproxycontract.Request{
		Account: antigravityModelDiscoveryRuntime(account, credential),
		Method:  http.MethodPost,
		URL:     endpoint,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		// Transport failure: report status 0 so fallback treats it as a
		// connection error (sub2api IsConnectionError path).
		return nil, 0, errModelDiscoveryUpstream
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, errModelDiscoveryUpstream
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, resp.StatusCode, errModelDiscoveryUpstream
	}
	return decoded, resp.StatusCode, nil
}

func extractAntigravityProjectID(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"cloudaicompanionProject", "projectId", "project"} {
		switch value := data[key].(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case map[string]any:
			if id, ok := value["id"].(string); ok {
				if trimmed := strings.TrimSpace(id); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func antigravityDefaultTierID(loadResp map[string]any) string {
	if tiers, ok := loadResp["allowedTiers"].([]any); ok {
		for _, rawTier := range tiers {
			tier, ok := rawTier.(map[string]any)
			if !ok {
				continue
			}
			if isDefault, ok := tier["isDefault"].(bool); !ok || !isDefault {
				continue
			}
			if id, ok := tier["id"].(string); ok {
				if trimmed := strings.TrimSpace(id); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if currentTier, ok := loadResp["currentTier"].(map[string]any); ok {
		if id, ok := currentTier["id"].(string); ok {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				return trimmed
			}
		}
	}
	if currentTier, ok := loadResp["currentTier"].(string); ok {
		if trimmed := strings.TrimSpace(currentTier); trimmed != "" {
			return trimmed
		}
	}
	return "free-tier"
}

func antigravityVersionFromAccount(account accountcontract.ProviderAccount) string {
	ua := mapString(account.Metadata, "user_agent")
	if ua == "" {
		return "1.0"
	}
	for _, field := range strings.Fields(ua) {
		lower := strings.ToLower(field)
		if !strings.HasPrefix(lower, "antigravity/") {
			continue
		}
		version := strings.TrimSpace(strings.TrimPrefix(field, "antigravity/"))
		version = strings.TrimSpace(strings.TrimPrefix(version, "Antigravity/"))
		if version != "" {
			return version
		}
	}
	return "1.0"
}

func antigravityModelDiscoveryRuntime(account accountcontract.ProviderAccount, credential map[string]any) reverseproxycontract.AccountRuntime {
	upstreamClient := account.UpstreamClient
	if upstreamClient == nil || strings.TrimSpace(*upstreamClient) == "" {
		value := "antigravity_desktop"
		upstreamClient = &value
	}
	return reverseproxycontract.AccountRuntime{
		AccountID:      account.ID,
		RuntimeClass:   string(account.RuntimeClass),
		UpstreamClient: upstreamClient,
		ProxyID:        account.ProxyID,
		UserAgent:      mapString(account.Metadata, "user_agent"),
		Metadata:       account.Metadata,
		Credential:     credential,
	}
}
