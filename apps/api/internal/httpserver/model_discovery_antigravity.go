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

func (rt *runtimeState) ensureAntigravityDiscoveryProject(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any) (antigravityProjectBootstrap, error) {
	if projectID := modelDiscoverySetting(provider, account, credential, "project_id", "antigravity_project_id", "cloudaicompanion_project"); projectID != "" {
		return antigravityProjectBootstrap{ProjectID: projectID}, nil
	}
	baseURL := upstreamModelDiscoveryBaseURL(modelDiscoveryAntigravity, provider, account)
	if baseURL == "" {
		return antigravityProjectBootstrap{}, errModelDiscoveryInvalidInput
	}
	if mapString(credential, "access_token") == "" {
		return antigravityProjectBootstrap{}, errModelDiscoveryAuth
	}

	loadEndpoint := strings.TrimRight(baseURL, "/") + "/v1internal:loadCodeAssist"
	loadResp, err := rt.executeAntigravityBootstrapRequest(ctx, account, credential, loadEndpoint, antigravityLoadCodeAssistBody(account), false)
	if err != nil {
		return antigravityProjectBootstrap{}, err
	}
	if projectID := extractAntigravityProjectID(loadResp); projectID != "" {
		return antigravityProjectBootstrap{ProjectID: projectID, Bootstrapped: true, Endpoint: loadEndpoint}, nil
	}

	onboardEndpoint := strings.TrimRight(baseURL, "/") + "/v1internal:onboardUser"
	onboardBody := antigravityOnboardUserBody(account, antigravityDefaultTierID(loadResp))
	for attempt := 0; attempt < 5; attempt++ {
		onboardResp, err := rt.executeAntigravityBootstrapRequest(ctx, account, credential, onboardEndpoint, onboardBody, true)
		if err != nil {
			return antigravityProjectBootstrap{}, err
		}
		if done, ok := onboardResp["done"].(bool); ok && !done {
			continue
		}
		if response, ok := onboardResp["response"].(map[string]any); ok {
			if projectID := extractAntigravityProjectID(response); projectID != "" {
				return antigravityProjectBootstrap{ProjectID: projectID, Bootstrapped: true, Endpoint: onboardEndpoint}, nil
			}
		}
		if projectID := extractAntigravityProjectID(onboardResp); projectID != "" {
			return antigravityProjectBootstrap{ProjectID: projectID, Bootstrapped: true, Endpoint: onboardEndpoint}, nil
		}
	}
	return antigravityProjectBootstrap{}, errModelDiscoveryUpstream
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

func (rt *runtimeState) executeAntigravityBootstrapRequest(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, endpoint string, body []byte, nodeClient bool) (map[string]any, error) {
	if !validModelDiscoveryEndpoint(endpoint) {
		return nil, errModelDiscoveryInvalidInput
	}
	if err := rt.materializeProviderProxy(ctx, &account); err != nil {
		return nil, errModelDiscoveryUpstream
	}
	// Refresh an expired OAuth/reverse-proxy token before dispatch, mirroring the
	// gateway path — otherwise antigravity bootstrap fails on an expired token.
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, account, credential); err != nil {
		return nil, errModelDiscoveryUpstream
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
		return nil, errModelDiscoveryUpstream
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errModelDiscoveryUpstream
	}
	var decoded map[string]any
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, errModelDiscoveryUpstream
	}
	return decoded, nil
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
