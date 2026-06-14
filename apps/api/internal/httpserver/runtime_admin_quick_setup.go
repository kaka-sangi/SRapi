package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type quickSetupRequest struct {
	Platform       string         `json:"platform"`
	Credential     map[string]any `json:"credential"`
	Name           string         `json:"name,omitempty"`
	RuntimeClass   string         `json:"runtime_class,omitempty"`
	ModelCatalog   []string       `json:"model_catalog,omitempty"`
	DiscoverModels bool           `json:"discover_models,omitempty"`
	ProxyID        *string        `json:"proxy_id,omitempty"`
	Priority       *int           `json:"priority,omitempty"`
	Weight         *float32       `json:"weight,omitempty"`
	RiskLevel      *string        `json:"risk_level,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type quickSetupResponse struct {
	Provider        any      `json:"provider"`
	Account         any      `json:"account"`
	ModelsCreated   int      `json:"models_created"`
	MappingsCreated int      `json:"mappings_created"`
	ModelNames      []string `json:"model_names,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func (s *Server) handleAdminQuickSetup(w http.ResponseWriter, r *http.Request) {
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

	var body quickSetupRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid quick setup request", requestID)
		return
	}
	if body.Platform == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "platform is required", requestID)
		return
	}
	if len(body.Credential) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "credential is required", requestID)
		return
	}

	preset, ok := providerpreset.Default().Lookup(body.Platform)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "unknown platform: "+body.Platform, requestID)
		return
	}

	result := quickSetupResponse{Warnings: []string{}}

	// Step 1: Create or find the provider.
	provider, err := s.runtime.providers.FindByName(r.Context(), preset.ProviderKey)
	if err != nil {
		status := providercontract.StatusActive
		provider, err = s.runtime.providers.Create(r.Context(), providerPresetCreateRequest(preset, status))
		if err != nil && !errors.Is(err, providerservice.ErrProviderExists) {
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create provider", requestID)
			return
		}
		if errors.Is(err, providerservice.ErrProviderExists) {
			provider, _ = s.runtime.providers.FindByName(r.Context(), preset.ProviderKey)
		}
	}

	// Activate if disabled.
	if provider.Status != providercontract.StatusActive {
		activeStatus := providercontract.StatusActive
		provider, _ = s.runtime.providers.Update(r.Context(), provider.ID, providercontract.UpdateRequest{
			Status: &activeStatus,
		})
	}
	result.Provider = toAPIProvider(provider)

	// Step 2: Resolve runtime class.
	runtimeClass := resolveQuickSetupRuntimeClass(body.RuntimeClass, preset, body.Credential)
	if !accountRuntimeClassAllowed(provider.ConfigSchema, runtimeClass) {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "authentication method not allowed for this provider", requestID)
		return
	}

	// Step 3: Build account metadata from template, then overlay user-provided metadata.
	metadata := map[string]any{}
	var upstreamClient *string
	if preset.AccountTemplate != nil {
		for k, v := range preset.AccountTemplate.DefaultMetadata {
			metadata[k] = v
		}
		if preset.AccountTemplate.UpstreamClient != "" {
			uc := preset.AccountTemplate.UpstreamClient
			upstreamClient = &uc
		}
	}
	for k, v := range body.Metadata {
		metadata[k] = v
	}

	// Step 4: Create the account.
	accountName := body.Name
	if accountName == "" {
		accountName = preset.DisplayName + " Account"
	}

	credential := body.Credential
	credential, err = s.refreshImportCredential(r.Context(), runtimeClass, upstreamClient, metadata, nil, credential)
	if err != nil {
		result.Warnings = append(result.Warnings, "oauth refresh skipped: "+err.Error())
		credential = body.Credential
	}

	account, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
		ProviderID:     provider.ID,
		Name:           accountName,
		RuntimeClass:   runtimeClass,
		Credential:     credential,
		Metadata:       metadata,
		UpstreamClient: upstreamClient,
		ProxyID:        body.ProxyID,
		Priority:       body.Priority,
		Weight:         body.Weight,
		RiskLevel:      body.RiskLevel,
	})
	if err != nil {
		switch {
		case errors.Is(err, accountservice.ErrCredentialMissing), errors.Is(err, accountservice.ErrInvalidInput):
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account: "+err.Error(), requestID)
		default:
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to create account", requestID)
		}
		return
	}
	result.Account = s.apiAccount(r.Context(), account)

	// Step 5: Create models + mappings from catalog.
	catalog := body.ModelCatalog
	if len(catalog) == 0 && preset.AccountTemplate != nil {
		catalog = preset.AccountTemplate.ModelCatalog
	}
	if len(catalog) > 0 {
		presetModelMapping := accountModelMappingFromMetadataValue(metadata[accountModelMappingMetadataKey])
		created, mapped, warnings := s.quickMapModels(r.Context(), provider, catalog, presetModelMapping)
		result.ModelsCreated = created
		result.MappingsCreated = mapped
		result.ModelNames = catalog
		result.Warnings = append(result.Warnings, warnings...)
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "quick_setup", "provider_account", strconv.Itoa(account.ID), nil, map[string]any{
		"platform":         body.Platform,
		"provider_id":      provider.ID,
		"account_id":       account.ID,
		"models_created":   result.ModelsCreated,
		"mappings_created": result.MappingsCreated,
	}))

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       result,
		"request_id": requestID,
	})
}

// handleAdminQuickMapModels bulk-creates model records and model-provider
// mappings from a list of model names.
func (s *Server) handleAdminQuickMapModels(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		ProviderID string   `json:"provider_id"`
		Models     []string `json:"models"`
	}
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request", requestID)
		return
	}
	providerID, err := strconv.Atoi(body.ProviderID)
	if err != nil || providerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider_id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider not found", requestID)
		return
	}

	created, mapped, warnings := s.quickMapModels(r.Context(), provider, body.Models, nil)

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "model.quick_map", "model", "bulk", nil, map[string]any{
		"provider_id":      providerID,
		"models_created":   created,
		"mappings_created": mapped,
		"model_count":      len(body.Models),
	}))

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"models_created":   created,
			"mappings_created": mapped,
			"warnings":         warnings,
		},
		"request_id": requestID,
	})
}

func (s *Server) quickMapModels(ctx context.Context, provider providercontract.Provider, modelNames []string, mappingOverride map[string]any) (modelsCreated, mappingsCreated int, warnings []string) {
	defaultMapping := mappingOverride
	if len(defaultMapping) == 0 {
		defaultMapping = providerQuickMapModelMapping(provider)
	}
	for _, name := range modelNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		model, err := s.runtime.models.FindByCanonicalName(ctx, name)
		if err != nil {
			model, err = s.runtime.models.Create(ctx, modelcontract.CreateRequest{
				CanonicalName: name,
				DisplayName:   name,
			})
			if err != nil {
				if errors.Is(err, modelservice.ErrModelExists) {
					model, _ = s.runtime.models.FindByCanonicalName(ctx, name)
				} else {
					warnings = append(warnings, "failed to create model: "+name)
					continue
				}
			} else {
				modelsCreated++
			}
		}
		if model.ID == 0 {
			warnings = append(warnings, "model not found after create: "+name)
			continue
		}
		upstreamModelName := quickMapUpstreamModelName(defaultMapping, name)
		_, err = s.runtime.models.CreateMapping(ctx, model.ID, modelcontract.CreateMappingRequest{
			ProviderID:        provider.ID,
			UpstreamModelName: upstreamModelName,
		})
		if err != nil {
			if !errors.Is(err, modelservice.ErrMappingExists) {
				warnings = append(warnings, "failed to map model: "+name)
			}
			continue
		}
		mappingsCreated++
	}
	return
}

func providerQuickMapModelMapping(provider providercontract.Provider) map[string]any {
	accountTemplate := anyMapValue(provider.ConfigSchema["account_template"])
	defaultMetadata := anyMapValue(accountTemplate["default_metadata"])
	return accountModelMappingFromMetadataValue(defaultMetadata[accountModelMappingMetadataKey])
}

func quickMapUpstreamModelName(mapping map[string]any, modelName string) string {
	if override := accountModelOverrideFromMetadata(map[string]any{accountModelMappingMetadataKey: mapping}, accountModelMappingMetadataKey, modelName); override != "" {
		return override
	}
	return modelName
}

func resolveQuickSetupRuntimeClass(requested string, preset providerpreset.Preset, credential map[string]any) accountcontract.RuntimeClass {
	if requested != "" {
		return accountcontract.RuntimeClass(requested)
	}
	// Infer from credential shape.
	if _, ok := credential["refresh_token"]; ok {
		return accountcontract.RuntimeClassOauthRefresh
	}
	if _, ok := credential["api_key"]; ok {
		return accountcontract.RuntimeClassAPIKey
	}
	if _, ok := credential["cookie"]; ok {
		return accountcontract.RuntimeClassWebSessionCookie
	}
	if _, ok := credential["access_token"]; ok {
		if len(preset.RuntimeClassAllowlist) > 0 {
			for _, rc := range preset.RuntimeClassAllowlist {
				if rc == accountcontract.RuntimeClassOauthRefresh || rc == accountcontract.RuntimeClassCliClientToken {
					return rc
				}
			}
		}
		return accountcontract.RuntimeClassCliClientToken
	}
	// Fall back to the first allowed class.
	if len(preset.RuntimeClassAllowlist) > 0 {
		return preset.RuntimeClassAllowlist[0]
	}
	return accountcontract.RuntimeClassAPIKey
}
