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
	Mode  string
	Model string
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

// testAccountLiveProbe issues a real minimal chat-completions round-trip through
// the same adapter the gateway uses, so OAuth/reverse-proxy/api_key accounts are
// verified against the real upstream (closing the "fake green light" gap where
// the default test only inspected fields). The model comes from opts.Model or an
// account/provider probe-model hint; without one the probe is skipped.
func (rt *runtimeState) testAccountLiveProbe(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, credential map[string]any, startedAt time.Time, opts adminAccountTestOptions, checks map[string]any) apiopenapi.AdminTestResult {
	checks["mode"] = adminAccountTestModeLive
	model := responsesCompactProbeModel(opts.Model, account, provider)
	if model == "" {
		checks["live_probe"] = "skipped_no_model"
		checks["missing_requirements"] = []string{"model"}
		return adminTestResult(false, "live account test requires a model (pass \"model\" or set account metadata test_model)", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	checks["probe_model"] = model

	raw, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": "Reply with OK."}},
	})
	if err != nil {
		checks["error"] = "live_probe_payload_failed"
		return adminTestResult(false, "live account test failed", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	resp, err := rt.adapters.InvokeConversation(ctx, provideradaptercontract.ConversationRequest{
		RequestID:      fmt.Sprintf("admin_account_%d_live_test", account.ID),
		SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SourceEndpoint: string(gatewaycontract.EndpointChatCompletions),
		TargetProtocol: provider.Protocol,
		Model:          model,
		InputParts:     []provideradaptercontract.ContentPart{{Kind: provideradaptercontract.ContentPartText, Text: "Reply with OK."}},
		RawBody:        raw,
		Provider:       provider,
		Account:        account,
		Mapping:        responsesCompactProbeMapping(model),
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
	model := responsesCompactProbeModel(opts.Model, account, provider)
	if model == "" {
		checks["missing_requirements"] = []string{"model"}
		return adminTestResult(false, "responses compact account test requires model", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
	}
	checks["probe_model"] = model

	raw, err := json.Marshal(map[string]any{
		"model": model,
		"input": "Respond with OK.",
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
		Model:          model,
		InputParts: []provideradaptercontract.ContentPart{{
			Kind: provideradaptercontract.ContentPartText,
			Text: "Respond with OK.",
		}},
		RawBody:    raw,
		Provider:   provider,
		Account:    account,
		Mapping:    responsesCompactProbeMapping(model),
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

func responsesCompactProbeModel(requested string, account accountcontract.ProviderAccount, provider providercontract.Provider) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	for _, values := range []map[string]any{account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		for _, key := range []string{"responses_compact_probe_model", "compact_probe_model", "test_model"} {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func responsesCompactProbeMapping(model string) modelcontract.ModelProviderMapping {
	return modelcontract.ModelProviderMapping{UpstreamModelName: model}
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
	if mapString(credential, "access_token") == "" && mapString(credential, "session_cookie") == "" {
		missing = append(missing, "credential.access_token_or_session_cookie")
	}
	if account.UpstreamClient == nil || strings.TrimSpace(*account.UpstreamClient) == "" {
		missing = append(missing, "upstream_client")
	}
	return missing
}

func upstreamBaseURLForTest(provider providercontract.Provider, account accountcontract.ProviderAccount) string {
	for _, values := range []map[string]any{account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		for _, key := range []string{"base_url", "upstream_base_url", "openai_base_url"} {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
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
