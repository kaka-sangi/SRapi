package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	adminAccountTestModeDefault          = "default"
	adminAccountTestModeResponsesCompact = "responses_compact"
	adminAccountTestModeLive             = "live"
	upstreamCapabilityUnsupportedStatus  = 501
)

type adminAccountTestOptions struct {
	Mode   string
	Model  string
	Prompt string
}

type accountTestModelSelection struct {
	Model   modelcontract.Model
	Mapping modelcontract.ModelProviderMapping
}

func (rt *runtimeState) testProvider(ctx context.Context, provider providercontract.Provider, startedAt time.Time) apiopenapi.AdminTestResult {
	accounts, err := rt.accounts.List(ctx)
	if err != nil {
		return adminTestResult(false, "provider account list failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), nil, map[string]any{
			"provider_exists": false,
			"error":           "account_list_failed",
		})
	}
	total := 0
	active := 0
	for _, account := range accounts {
		if account.ProviderID != provider.ID {
			continue
		}
		total++
		if account.Status == accountcontract.StatusActive {
			active++
		}
	}
	ok := provider.Status == providercontract.StatusActive && active > 0
	message := "provider is testable"
	if provider.Status != providercontract.StatusActive {
		message = "provider is not active"
	} else if active == 0 {
		message = "provider has no active accounts"
	}
	return adminTestResult(ok, message, startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), nil, map[string]any{
		"provider_exists":  true,
		"provider_active":  provider.Status == providercontract.StatusActive,
		"account_count":    total,
		"active_accounts":  active,
		"adapter_type":     provider.AdapterType,
		"protocol":         provider.Protocol,
		"provider_key":     mapString(provider.ConfigSchema, "provider_key"),
		"platform_family":  mapString(provider.ConfigSchema, "platform_family"),
		"default_base_url": mapString(provider.ConfigSchema, "default_base_url"),
	})
}

func (rt *runtimeState) testAccount(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, startedAt time.Time, opts adminAccountTestOptions) apiopenapi.AdminTestResult {
	checks := map[string]any{
		"provider_exists":     true,
		"provider_active":     provider.Status == providercontract.StatusActive,
		"account_active":      account.Status == accountcontract.StatusActive,
		"credential_decrypts": false,
		"runtime_class":       account.RuntimeClass,
		"adapter_type":        provider.AdapterType,
		"protocol":            provider.Protocol,
	}
	credential, err := rt.accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		checks["credential_error"] = "decrypt_failed"
		return adminTestResult(false, "provider account credential could not be decrypted", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	checks["credential_decrypts"] = true
	checks["credential_fields"] = credentialFieldNames(credential)

	missing := accountTestMissingRequirements(provider, account, credential)
	if len(missing) > 0 {
		checks["missing_requirements"] = missing
		return adminTestResult(false, "provider account is missing required runtime configuration", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	if opts.Mode == adminAccountTestModeResponsesCompact {
		return rt.testAccountResponsesCompact(ctx, provider, account, credential, startedAt, opts, checks)
	}
	if opts.Mode == adminAccountTestModeLive {
		return rt.testAccountLiveProbe(ctx, provider, account, credential, startedAt, opts, checks)
	}
	// Default mode is a cheap structural check only. For OAuth/reverse-proxy
	// runtimes it cannot verify the credential actually works upstream, so say
	// so plainly rather than implying a verified connection (run mode "live" for
	// a real upstream round-trip).
	checks["live_probe"] = "not_run"
	message := "provider account passed structural checks"
	if account.RuntimeClass != accountcontract.RuntimeClassAPIKey {
		message = "provider account passed structural checks (credential not verified upstream; run a live test)"
	}
	return adminTestResult(true, message, startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
}

// testAccountLiveProbe issues a real minimal round-trip through the same adapter
// the gateway uses, so OAuth/reverse-proxy/api_key accounts are verified against
// the real upstream. The model is selected from registered active models mapped
// to this provider/account, matching the gateway scheduling surface.
func (rt *runtimeState) testAccountLiveProbe(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, startedAt time.Time, opts adminAccountTestOptions, checks map[string]any) apiopenapi.AdminTestResult {
	checks["mode"] = adminAccountTestModeLive
	sourceEndpoint := string(gatewaycontract.EndpointChatCompletions)
	if strings.EqualFold(strings.TrimSpace(provider.AdapterType), "reverse-proxy-codex-cli") {
		sourceEndpoint = string(gatewaycontract.EndpointResponses)
	}
	selection, err := rt.selectAccountTestModel(ctx, provider, account, opts.Model, sourceEndpoint)
	if err != nil {
		checks["live_probe"] = "skipped_no_model"
		checks["missing_requirements"] = []string{"model"}
		checks["error"] = err.Error()
		return adminTestResult(false, "live account test requires a registered active model mapped to this provider", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	checks["probe_model"] = selection.Model.CanonicalName
	checks["probe_upstream_model"] = selection.Mapping.UpstreamModelName
	prompt := adminAccountTestPrompt(opts.Prompt)

	var raw []byte
	if sourceEndpoint == string(gatewaycontract.EndpointResponses) {
		checks["live_probe_endpoint"] = sourceEndpoint
	} else {
		var err error
		raw, err = json.Marshal(map[string]any{
			"model":    selection.Mapping.UpstreamModelName,
			"messages": []map[string]any{{"role": "user", "content": prompt}},
		})
		if err != nil {
			checks["error"] = "live_probe_payload_failed"
			return adminTestResult(false, "live account test failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
		}
		checks["live_probe_endpoint"] = sourceEndpoint
	}
	resp, err := rt.adapters.InvokeConversation(ctx, provideradaptercontract.ConversationRequest{
		RequestID:      fmt.Sprintf("admin_account_%d_live_test", account.ID),
		SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SourceEndpoint: sourceEndpoint,
		TargetProtocol: provider.Protocol,
		Model:          selection.Model.CanonicalName,
		InputParts:     []provideradaptercontract.ContentPart{{Kind: provideradaptercontract.ContentPartText, Text: prompt}},
		RawBody:        raw,
		Provider:       provider,
		Account:        account,
		Mapping:        selection.Mapping,
		Credential:     credential,
	})
	if err == nil {
		statusCode := resp.StatusCode
		if statusCode <= 0 {
			statusCode = http.StatusOK
		}
		checks["live_probe"] = "ok"
		checks["upstream_reachable"] = true
		checks["upstream_status"] = statusCode
		if len(resp.QuotaSignals) > 0 {
			rt.recordProviderQuotaSignals(ctx, account, resp.QuotaSignals, time.Now().UTC())
			checks["quota_signals_persisted"] = len(resp.QuotaSignals)
		}
		return adminTestResult(true, "provider account verified against upstream", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}

	var providerErr provideradaptercontract.ProviderError
	statusCode := http.StatusBadGateway
	errorClass := "provider_probe_failed"
	errorMessage := err.Error()
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode > 0 {
			statusCode = providerErr.StatusCode
		}
		if strings.TrimSpace(providerErr.Class) != "" {
			errorClass = strings.TrimSpace(providerErr.Class)
		}
		errorMessage = providerErr.Error()
	}
	checks["live_probe"] = "failed"
	checks["upstream_reachable"] = false
	checks["upstream_status"] = statusCode
	checks["error_class"] = errorClass
	checks["error_message"] = truncateAdminTestMessage(errorMessage, 512)
	return adminTestResult(false, "live account test failed: "+errorClass, startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
}

func (rt *runtimeState) testAccountResponsesCompact(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, startedAt time.Time, opts adminAccountTestOptions, checks map[string]any) apiopenapi.AdminTestResult {
	checks["mode"] = adminAccountTestModeResponsesCompact
	selection, err := rt.selectAccountTestModel(ctx, provider, account, opts.Model, string(gatewaycontract.EndpointResponsesCompact))
	if err != nil {
		checks["missing_requirements"] = []string{"model"}
		checks["error"] = err.Error()
		return adminTestResult(false, "responses compact account test requires a registered active model mapped to this provider", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	checks["probe_model"] = selection.Model.CanonicalName
	checks["probe_upstream_model"] = selection.Mapping.UpstreamModelName
	prompt := adminAccountTestPrompt(opts.Prompt)

	raw, err := json.Marshal(map[string]any{
		"model": selection.Mapping.UpstreamModelName,
		"input": prompt,
	})
	if err != nil {
		checks["error"] = "compact_probe_payload_failed"
		return adminTestResult(false, "responses compact account test failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	resp, err := rt.adapters.InvokeConversation(ctx, provideradaptercontract.ConversationRequest{
		RequestID:      fmt.Sprintf("admin_account_%d_responses_compact_test", account.ID),
		SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SourceEndpoint: string(gatewaycontract.EndpointResponsesCompact),
		TargetProtocol: provider.Protocol,
		Model:          selection.Model.CanonicalName,
		InputParts: []provideradaptercontract.ContentPart{{
			Kind: provideradaptercontract.ContentPartText,
			Text: prompt,
		}},
		RawBody:    raw,
		Provider:   provider,
		Account:    account,
		Mapping:    selection.Mapping,
		Credential: credential,
	})
	if err == nil {
		statusCode := resp.StatusCode
		if statusCode <= 0 {
			statusCode = http.StatusOK
		}
		checks["responses_compact_supported"] = true
		checks["upstream_status"] = statusCode
		if err := rt.persistResponsesCompactProbe(ctx, account, true, statusCode, "", time.Now().UTC()); err != nil {
			checks["metadata_update_error"] = "account_update_failed"
			return adminTestResult(false, "responses compact probe succeeded but metadata update failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
		}
		checks["metadata_persisted"] = true
		if len(resp.QuotaSignals) > 0 {
			rt.recordProviderQuotaSignals(ctx, account, resp.QuotaSignals, time.Now().UTC())
			checks["quota_signals_persisted"] = len(resp.QuotaSignals)
		}
		return adminTestResult(true, "provider account supports responses compact", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}

	var providerErr provideradaptercontract.ProviderError
	statusCode := http.StatusBadGateway
	errorClass := "provider_probe_failed"
	errorMessage := err.Error()
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode > 0 {
			statusCode = providerErr.StatusCode
		}
		if strings.TrimSpace(providerErr.Class) != "" {
			errorClass = strings.TrimSpace(providerErr.Class)
		}
		errorMessage = providerErr.Error()
	}
	checks["responses_compact_supported"] = false
	checks["upstream_status"] = statusCode
	checks["error_class"] = errorClass
	checks["error_message"] = truncateAdminTestMessage(errorMessage, 512)
	if responsesCompactUnsupported(statusCode, errorMessage) {
		if err := rt.persistResponsesCompactProbe(ctx, account, false, statusCode, errorMessage, time.Now().UTC()); err != nil {
			checks["metadata_update_error"] = "account_update_failed"
			return adminTestResult(false, "responses compact probe failed and metadata update failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
		}
		checks["metadata_persisted"] = true
		return adminTestResult(false, "provider account does not support responses compact", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	checks["metadata_persisted"] = false
	return adminTestResult(false, "responses compact account test failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
}

func (rt *runtimeState) persistResponsesCompactProbe(ctx context.Context, account accountcontract.ProviderAccount, supported bool, statusCode int, errorMessage string, checkedAt time.Time) error {
	updates := map[string]any{
		"capability_responses_compact":      supported,
		"responses_compact_checked_at":      checkedAt.UTC().Format(time.RFC3339),
		"responses_compact_last_status":     statusCode,
		"responses_compact_last_error":      truncateAdminTestMessage(errorMessage, 512),
		"responses_compact_probe_source":    "admin_account_test",
		"responses_compact_probe_succeeded": supported,
	}
	if strings.TrimSpace(errorMessage) == "" {
		updates["responses_compact_last_error"] = ""
	}
	metadata := mergeAccountMetadata(account.Metadata, &updates)
	_, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata})
	return err
}

func (rt *runtimeState) selectAccountTestModel(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, requested string, sourceEndpoint string) (accountTestModelSelection, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		resolution, err := rt.models.ResolveModelReference(ctx, requested)
		if err != nil || resolution.Model.Status != modelcontract.StatusActive {
			return accountTestModelSelection{}, fmt.Errorf("requested model is not a registered active model")
		}
		mapping, err := activeProviderMappingForModel(ctx, rt.models, resolution.Model.ID, provider, account, resolution.Model.CanonicalName, sourceEndpoint)
		if err != nil {
			return accountTestModelSelection{}, err
		}
		return accountTestModelSelection{Model: resolution.Model, Mapping: mapping}, nil
	}

	models, err := rt.models.List(ctx)
	if err != nil {
		return accountTestModelSelection{}, fmt.Errorf("model list failed")
	}
	sort.Slice(models, func(i, j int) bool {
		return strings.ToLower(models[i].CanonicalName) < strings.ToLower(models[j].CanonicalName)
	})
	for _, model := range models {
		if model.Status != modelcontract.StatusActive {
			continue
		}
		mapping, err := activeProviderMappingForModel(ctx, rt.models, model.ID, provider, account, model.CanonicalName, sourceEndpoint)
		if err == nil {
			return accountTestModelSelection{Model: model, Mapping: mapping}, nil
		}
	}
	return accountTestModelSelection{}, fmt.Errorf("no registered active model is mapped to this provider")
}

func activeProviderMappingForModel(ctx context.Context, models interface {
	ListMappingsByModel(context.Context, int) ([]modelcontract.ModelProviderMapping, error)
}, modelID int, provider providercontract.Provider, account accountcontract.ProviderAccount, canonicalName string, sourceEndpoint string) (modelcontract.ModelProviderMapping, error) {
	mappings, err := models.ListMappingsByModel(ctx, modelID)
	if err != nil {
		return modelcontract.ModelProviderMapping{}, fmt.Errorf("model mapping list failed")
	}
	for _, mapping := range mappings {
		if mapping.ProviderID != provider.ID || mapping.Status != modelcontract.StatusActive || strings.TrimSpace(mapping.UpstreamModelName) == "" {
			continue
		}
		effectiveMapping := accountEffectiveModelMapping(mapping, account, canonicalName, sourceEndpoint)
		effectiveMapping = providerEffectiveModelMapping(provider, effectiveMapping)
		if providerAccountExcludesModel(provider, account, canonicalName, effectiveMapping.UpstreamModelName) {
			continue
		}
		if !accountRoutableForModel(provider, account, effectiveMapping.UpstreamModelName) {
			continue
		}
		return effectiveMapping, nil
	}
	return modelcontract.ModelProviderMapping{}, fmt.Errorf("registered model is not mapped to this provider")
}

func responsesCompactUnsupported(statusCode int, message string) bool {
	switch statusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed, upstreamCapabilityUnsupportedStatus:
		return true
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity:
		lower := strings.ToLower(strings.TrimSpace(message))
		if !strings.Contains(lower, "compact") {
			return false
		}
		for _, keyword := range []string{"unsupported", "not support", "does not support", "not available", "disabled"} {
			if strings.Contains(lower, keyword) {
				return true
			}
		}
	}
	return false
}

func normalizeAdminAccountTestMode(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", adminAccountTestModeDefault:
		return adminAccountTestModeDefault, true
	case adminAccountTestModeLive:
		return adminAccountTestModeLive, true
	case adminAccountTestModeResponsesCompact, "compact":
		return adminAccountTestModeResponsesCompact, true
	default:
		return "", false
	}
}

func truncateAdminTestMessage(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func adminAccountTestPrompt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Reply with OK."
	}
	return value
}

func adminTestResult(ok bool, message string, startedAt time.Time, providerID apiopenapi.Id, accountID *apiopenapi.Id, checks map[string]any) apiopenapi.AdminTestResult {
	status := "failed"
	if ok {
		status = "ok"
	}
	return apiopenapi.AdminTestResult{
		AccountId:  accountID,
		CheckedAt:  time.Now().UTC(),
		Checks:     mapToJsonObjectPtr(checks),
		LatencyMs:  ptrInt(elapsedMillis(startedAt)),
		Message:    ptrString(message),
		Ok:         ok,
		ProviderId: &providerID,
		Status:     apiopenapi.AdminTestResultStatus(status),
	}
}

func accountTestMissingRequirements(provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any) []string {
	missing := []string{}
	if provider.Status != providercontract.StatusActive {
		missing = append(missing, "provider_active")
	}
	if account.Status != accountcontract.StatusActive {
		missing = append(missing, "account_active")
	}
	if account.RuntimeClass == accountcontract.RuntimeClassAPIKey {
		if mapString(credential, "api_key") == "" {
			missing = append(missing, "credential.api_key")
		}
		return missing
	}
	if strings.EqualFold(strings.TrimSpace(provider.AdapterType), "reverse-proxy-codex-cli") && mapString(credential, "cli_client_token") != "" {
		if account.UpstreamClient == nil || strings.TrimSpace(*account.UpstreamClient) == "" {
			missing = append(missing, "upstream_client")
		}
		return missing
	}
	if mapString(credential, "access_token") == "" && mapString(credential, "session_cookie") == "" {
		missing = append(missing, "credential.access_token_or_session_cookie")
	}
	if account.UpstreamClient == nil || strings.TrimSpace(*account.UpstreamClient) == "" {
		missing = append(missing, "upstream_client")
	}
	return missing
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func credentialFieldNames(credential map[string]any) []string {
	fields := make([]string, 0, len(credential))
	for key := range credential {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

func ptrID(id int) *apiopenapi.Id {
	value := apiopenapi.Id(strconv.Itoa(id))
	return &value
}

func reverseProxyAccountRuntime(account accountcontract.ProviderAccount, credential map[string]any) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      account.ID,
		RuntimeClass:   string(account.RuntimeClass),
		UpstreamClient: account.UpstreamClient,
		ProxyID:        account.ProxyID,
		UserAgent:      mapString(account.Metadata, "user_agent"),
		Metadata:       account.Metadata,
		Credential:     credential,
	}
}
