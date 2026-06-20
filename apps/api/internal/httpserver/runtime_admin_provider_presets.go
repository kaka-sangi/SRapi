package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleInstallAdminProviderPresets(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}

	presets := providerpreset.Default().List()
	result := apiopenapi.BatchOperationResult{
		Requested: len(presets),
	}
	installedNames := []string{}
	for _, preset := range presets {
		if _, err := s.runtime.providers.FindByName(r.Context(), preset.ProviderKey); err == nil {
			continue
		}
		// Activate providers that have a known upstream URL. They are
		// ready to use immediately once credentials are added.
		status := providercontract.StatusDisabled
		if preset.DefaultBaseURL != "" {
			status = providercontract.StatusActive
		}
		provider, err := s.runtime.providers.Create(r.Context(), providerPresetCreateRequest(preset, status))
		if err != nil {
			if errors.Is(err, providerservice.ErrProviderExists) {
				continue
			}
			result.Failed++
			failedIDs := []apiopenapi.Id{apiopenapi.Id(preset.ProviderKey)}
			if result.FailedIds != nil {
				failedIDs = append(*result.FailedIds, failedIDs...)
			}
			result.FailedIds = &failedIDs
			s.logger.Warn("failed to install provider preset", "provider_key", preset.ProviderKey, "error", err)
			continue
		}
		result.Succeeded++
		installedNames = append(installedNames, provider.Name)
	}
	skipped := result.Requested - result.Succeeded - result.Failed

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_preset.install", "provider", "bulk", nil, map[string]any{
		"installed_count": result.Succeeded,
		"skipped_count":   skipped,
		"failed_count":    result.Failed,
		"installed_names": installedNames,
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.BatchOperationResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) handleGetAdminProviderOAuthConfig(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || providerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider not found", requestID)
		return
	}
	config, ok := providerOAuthConfigFromSchema(provider.ConfigSchema)
	if !ok {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "provider oauth config not found", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ProviderOAuthConfigResponse{
		Data: apiopenapi.ProviderOAuthConfig{
			ProviderId:   apiopenapi.Id(strconv.Itoa(provider.ID)),
			ProviderName: provider.Name,
			Config:       config,
		},
		RequestId: requestID,
	})
}

func providerPresetCreateRequest(preset providerpreset.Preset, status providercontract.Status) providercontract.CreateRequest {
	return providercontract.CreateRequest{
		Name:         preset.ProviderKey,
		DisplayName:  preset.DisplayName,
		AdapterType:  presetAdapterType(preset),
		Protocol:     presetProtocol(preset),
		Status:       &status,
		Capabilities: presetCapabilityMap(preset),
		ConfigSchema: presetConfigSchema(preset),
	}
}

func presetAdapterType(preset providerpreset.Preset) string {
	if preset.ProviderKey == "chatgpt-web" {
		return "reverse-proxy-chatgpt-web"
	}
	switch preset.PlatformFamily {
	case providerpreset.PlatformFamilyAnthropicCompatible:
		return "anthropic-compatible"
	case providerpreset.PlatformFamilyGeminiCompatible:
		return "gemini-compatible"
	case providerpreset.PlatformFamilyXAICompatible:
		return "native-grok"
	case providerpreset.PlatformFamilyReverseProxyAntigravity:
		return "reverse-proxy-antigravity"
	case providerpreset.PlatformFamilyRerankCompatible:
		return "rerank-compatible"
	case providerpreset.PlatformFamilyCodexCLI:
		return "reverse-proxy-codex-cli"
	default:
		return "openai-compatible"
	}
}

func presetProtocol(preset providerpreset.Preset) string {
	switch preset.PlatformFamily {
	case providerpreset.PlatformFamilyAnthropicCompatible:
		return "anthropic-compatible"
	case providerpreset.PlatformFamilyGeminiCompatible:
		return "gemini-compatible"
	case providerpreset.PlatformFamilyXAICompatible:
		return "openai-compatible"
	case providerpreset.PlatformFamilyRerankCompatible:
		return "rerank-compatible"
	default:
		return "openai-compatible"
	}
}

func presetCapabilityMap(preset providerpreset.Preset) map[string]any {
	out := make(map[string]any, len(preset.Capabilities))
	for key, value := range preset.Capabilities {
		out[key] = value
	}
	return out
}

func presetConfigSchema(preset providerpreset.Preset) map[string]any {
	schema := map[string]any{
		"provider_key":         preset.ProviderKey,
		"platform_family":      string(preset.PlatformFamily),
		"default_base_url":     preset.DefaultBaseURL,
		"route_aliases":        stringSliceAny(preset.RouteAliases),
		"gemini_route_aliases": stringSliceAny(preset.GeminiRouteAliases),
		"auth_modes":           authModesAny(preset.AuthModes),
		"model_catalog_owner":  preset.ModelCatalogOwner,
		"auth_methods":         runtimeClassesAny(preset.RuntimeClassAllowlist),
		"installed_from":       "provider_preset",
	}
	// Expose base_url at the provider level so upstreamBaseURL() resolves it
	// without requiring per-account metadata.
	if preset.DefaultBaseURL != "" {
		schema["base_url"] = preset.DefaultBaseURL
	}
	if preset.AccountTemplate != nil {
		schema["account_template"] = map[string]any{
			"upstream_client":  preset.AccountTemplate.UpstreamClient,
			"default_metadata": preset.AccountTemplate.DefaultMetadata,
			"model_catalog":    preset.AccountTemplate.ModelCatalog,
			"metadata_hints":   preset.AccountTemplate.MetadataHints,
		}
	}
	if preset.OAuthConfig != nil {
		schema["oauth_config"] = map[string]any{
			"client_id":            preset.OAuthConfig.ClientID,
			"authorize_url":        preset.OAuthConfig.AuthorizeURL,
			"token_url":            preset.OAuthConfig.TokenURL,
			"device_authorize_url": preset.OAuthConfig.DeviceAuthorizeURL,
			"redirect_uri":         preset.OAuthConfig.RedirectURI,
			"scopes":               preset.OAuthConfig.Scopes,
			"use_pkce":             preset.OAuthConfig.UsePKCE,
		}
	}
	for k, v := range preset.QuotaConfig {
		schema[k] = v
	}
	return schema
}

func stringSliceAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func authModesAny(values []providerpreset.AuthMode) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func runtimeClassesAny(values []accountcontract.RuntimeClass) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// accountRuntimeClassAllowed reports whether runtimeClass may be attached to a
// provider whose preset declared the given auth_methods allowlist (read from
// config_schema). An empty allowlist means "no restriction"; this keeps legacy
// and manually-created providers, which carry no preset metadata, working.
func accountRuntimeClassAllowed(configSchema map[string]any, runtimeClass accountcontract.RuntimeClass) bool {
	allowed := providerAuthMethodStrings(configSchema)
	if len(allowed) == 0 {
		return true
	}
	for _, method := range allowed {
		if method == string(runtimeClass) {
			return true
		}
	}
	return false
}

// providerAuthMethodStrings reads the auth_methods allowlist a provider preset
// stored in its config_schema. An empty result means "no restriction".
func providerAuthMethodStrings(configSchema map[string]any) []string {
	raw, ok := configSchema["auth_methods"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		if s, ok := value.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func providerOAuthConfigFromSchema(configSchema map[string]any) (apiopenapi.AccountOAuthProviderConfig, bool) {
	raw, ok := configSchema["oauth_config"]
	if !ok {
		return apiopenapi.AccountOAuthProviderConfig{}, false
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return apiopenapi.AccountOAuthProviderConfig{}, false
	}
	clientID := strings.TrimSpace(oauthConfigString(values, "client_id"))
	if clientID == "" {
		return apiopenapi.AccountOAuthProviderConfig{}, false
	}
	config := apiopenapi.AccountOAuthProviderConfig{ClientId: clientID}
	if value := strings.TrimSpace(oauthConfigString(values, "authorize_url")); value != "" {
		config.AuthorizeUrl = &value
	}
	if value := strings.TrimSpace(oauthConfigString(values, "token_url")); value != "" {
		config.TokenUrl = &value
	}
	if value := strings.TrimSpace(oauthConfigString(values, "device_authorize_url")); value != "" {
		config.DeviceAuthorizeUrl = &value
	}
	if value := strings.TrimSpace(oauthConfigString(values, "redirect_uri")); value != "" {
		config.RedirectUri = &value
	}
	if scopes := oauthConfigStringSlice(values["scopes"]); len(scopes) > 0 {
		config.Scopes = &scopes
	}
	if usePKCE, ok := values["use_pkce"].(bool); ok {
		config.UsePkce = &usePKCE
	}
	return config, true
}

func oauthConfigString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func oauthConfigStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}
