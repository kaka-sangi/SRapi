package httpserver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

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
		"provider_exists": true,
		"provider_active": provider.Status == providercontract.StatusActive,
		"account_count":   total,
		"active_accounts": active,
		"adapter_type":    provider.AdapterType,
		"protocol":        provider.Protocol,
	})
}

func (rt *runtimeState) testAccount(ctx context.Context, provider providercontract.Provider, account accountcontract.ProviderAccount, startedAt time.Time) apiopenapi.AdminTestResult {
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
	return adminTestResult(true, "provider account is testable", startedAt, apiopenapi.Id(strconv.Itoa(provider.ID)), ptrID(account.ID), checks)
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
