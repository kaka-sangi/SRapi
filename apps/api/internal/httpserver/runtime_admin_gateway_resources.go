package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Server) handleGetAdminGatewayResources(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	input, err := s.gatewayResourceSummaryInput(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to build gateway resources", requestID)
		return
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.GatewayResourceSummaryResponse{
		Data:      buildGatewayResourceSummary(input),
		RequestId: requestID,
	})
}

func (s *Server) gatewayResourceSummaryInput(ctx context.Context) (gatewayResourceSummaryInput, error) {
	providers, err := s.runtime.providers.List(ctx)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	proxies, err := s.runtime.accounts.ListProxies(ctx)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	models, err := s.runtime.models.List(ctx)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	modelMappings, err := s.runtime.models.ListMappings(ctx)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	apiKeys, err := s.runtime.apiKeys.List(ctx)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	accounts = activeResourceInventoryAccounts(accounts)
	accountIDs := make([]int, 0, len(accounts))
	for _, account := range accounts {
		accountIDs = append(accountIDs, account.ID)
	}
	groupIDsByAccount, err := s.runtime.accounts.ListGroupIDsByAccounts(ctx, accountIDs)
	if err != nil {
		return gatewayResourceSummaryInput{}, err
	}
	healthByAccount, err := s.runtime.accounts.LatestHealthSnapshotsByAccounts(ctx, accountIDs)
	if err != nil {
		healthByAccount = nil
	}
	quotasByAccount, err := s.runtime.accounts.LatestQuotaSnapshotsByAccounts(ctx, accountIDs)
	if err != nil {
		quotasByAccount = nil
	}
	return gatewayResourceSummaryInput{
		Providers:         providers,
		Accounts:          accounts,
		Proxies:           proxies,
		Models:            models,
		ModelMappings:     modelMappings,
		APIKeys:           apiKeys,
		HealthByAccount:   healthByAccount,
		QuotasByAccount:   quotasByAccount,
		GroupIDsByAccount: groupIDsByAccount,
		Now:               time.Now().UTC(),
	}, nil
}

type gatewayResourceSummaryInput struct {
	Providers         []providercontract.Provider
	Accounts          []accountcontract.ProviderAccount
	Proxies           []accountcontract.ProxyDefinition
	Models            []modelcontract.Model
	ModelMappings     []modelcontract.ModelProviderMapping
	APIKeys           []apikeycontract.APIKey
	HealthByAccount   map[int]accountcontract.AccountHealthSnapshot
	QuotasByAccount   map[int][]accountcontract.AccountQuotaSnapshot
	GroupIDsByAccount map[int][]int
	Now               time.Time
}

func buildGatewayResourceSummary(input gatewayResourceSummaryInput) apiopenapi.GatewayResourceSummary {
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	healthByAccount := input.HealthByAccount
	if healthByAccount == nil {
		healthByAccount = map[int]accountcontract.AccountHealthSnapshot{}
	}
	quotaExhaustedByAccount := quotaExhaustedAccounts(input.QuotasByAccount, now)
	proxyStates := buildProxyResourceStates(input.Proxies, now)
	activeKeys := activeAPIKeys(input.APIKeys)
	activeModels := activeModels(input.Models)
	activeModelIDs := make(map[int]struct{}, len(activeModels))
	for _, model := range activeModels {
		activeModelIDs[model.ID] = struct{}{}
	}
	activeModelMappings := activeProviderModelMappings(input.ModelMappings, activeModelIDs)
	scopedKeyCount := 0
	for _, key := range activeKeys {
		if apiKeyIsScoped(key) {
			scopedKeyCount++
		}
	}

	activeProxies := 0
	availableProxies := 0
	expiredProxies := 0
	for _, proxy := range input.Proxies {
		if proxy.Status != accountcontract.ProxyStatusActive {
			continue
		}
		activeProxies++
		state, ok := proxyStates[strconv.Itoa(proxy.ID)]
		if ok && state.Available {
			availableProxies++
		}
		if ok && state.Expired {
			expiredProxies++
		}
	}

	providerAccounts := accountsByProvider(input.Accounts)
	providerGroupIDs := groupIDsByProvider(input.Accounts, input.GroupIDsByAccount)
	providerMappings := mappingsByProvider(activeModelMappings)
	routableAccountsByProvider := make(map[int][]accountcontract.ProviderAccount, len(input.Providers))
	rows := make([]apiopenapi.GatewayProviderResourceRow, 0, len(input.Providers))
	for _, provider := range input.Providers {
		accounts := providerAccounts[provider.ID]
		activeAccounts := filterActiveAccounts(accounts)
		routableAccounts := make([]accountcontract.ProviderAccount, 0, len(activeAccounts))
		for _, account := range activeAccounts {
			health, hasHealth := healthByAccount[account.ID]
			if accountIsGatewayResourceRoutable(account, health, hasHealth, quotaExhaustedByAccount[account.ID]) &&
				accountProxyCanRouteResource(account, proxyStates) {
				routableAccounts = append(routableAccounts, account)
			}
		}
		routableAccountsByProvider[provider.ID] = routableAccounts
		proxiedAccounts := 0
		proxyAttentionAccounts := 0
		for _, account := range activeAccounts {
			if accountProxyID(account) == "" {
				continue
			}
			proxiedAccounts++
			if accountNeedsProxyResourceAttention(account, proxyStates) {
				proxyAttentionAccounts++
			}
		}
		providerKeys := filterProviderAPIKeys(activeKeys, providerGroupIDs[provider.ID])
		scopedProviderKeys := 0
		for _, key := range providerKeys {
			if apiKeyIsScoped(key) {
				scopedProviderKeys++
			}
		}
		reasons := gatewayProviderResourceReasons(provider, len(activeModels), len(providerMappings[provider.ID]), len(activeAccounts), len(routableAccounts), proxyAttentionAccounts, len(providerKeys))
		status := apiopenapi.GatewayProviderResourceStatusReady
		if len(reasons) > 0 {
			status = apiopenapi.GatewayProviderResourceStatusBlocked
			if len(routableAccounts) > 0 {
				status = apiopenapi.GatewayProviderResourceStatusLimited
			}
		}
		rows = append(rows, apiopenapi.GatewayProviderResourceRow{
			ActiveModelMappings:    len(providerMappings[provider.ID]),
			ApiKeyCount:            len(providerKeys),
			AttentionAccounts:      len(activeAccounts) - len(routableAccounts),
			Provider:               toAPIProvider(provider),
			ProxiedAccounts:        proxiedAccounts,
			ProxyAttentionAccounts: proxyAttentionAccounts,
			Reasons:                reasons,
			RoutableAccounts:       len(routableAccounts),
			ScopedKeyCount:         scopedProviderKeys,
			Status:                 status,
			TotalAccounts:          len(accounts),
		})
	}
	activeProviders := 0
	activeAccounts := 0
	for _, provider := range input.Providers {
		if provider.Status == providercontract.StatusActive {
			activeProviders++
		}
	}
	for _, account := range input.Accounts {
		if account.Status == accountcontract.StatusActive {
			activeAccounts++
		}
	}
	return apiopenapi.GatewayResourceSummary{
		ActiveAccounts:         activeAccounts,
		ActiveApiKeys:          len(activeKeys),
		ActiveModelMappings:    len(activeModelMappings),
		ActiveModels:           len(activeModels),
		ActiveProviders:        activeProviders,
		ActiveProxies:          activeProxies,
		AvailableProxies:       availableProxies,
		ExpiredProxies:         expiredProxies,
		ModelRows:              buildGatewayModelResourceRows(activeModels, activeModelMappings, input.Providers, routableAccountsByProvider, activeKeys),
		Providers:              len(input.Providers),
		ProxiedAccounts:        sumProviderResourceRows(rows, func(row apiopenapi.GatewayProviderResourceRow) int { return row.ProxiedAccounts }),
		ProxyAttentionAccounts: sumProviderResourceRows(rows, func(row apiopenapi.GatewayProviderResourceRow) int { return row.ProxyAttentionAccounts }),
		RoutableAccounts:       sumProviderResourceRows(rows, func(row apiopenapi.GatewayProviderResourceRow) int { return row.RoutableAccounts }),
		Rows:                   rows,
		ScopedApiKeys:          scopedKeyCount,
	}
}

func activeAPIKeys(keys []apikeycontract.APIKey) []apikeycontract.APIKey {
	out := make([]apikeycontract.APIKey, 0, len(keys))
	for _, key := range keys {
		if key.Status == apikeycontract.StatusActive {
			out = append(out, key)
		}
	}
	return out
}

func activeModels(models []modelcontract.Model) []modelcontract.Model {
	out := make([]modelcontract.Model, 0, len(models))
	for _, model := range models {
		if model.Status == modelcontract.StatusActive {
			out = append(out, model)
		}
	}
	return out
}

func activeProviderModelMappings(mappings []modelcontract.ModelProviderMapping, activeModelIDs map[int]struct{}) []modelcontract.ModelProviderMapping {
	out := make([]modelcontract.ModelProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Status != modelcontract.StatusActive {
			continue
		}
		if _, ok := activeModelIDs[mapping.ModelID]; !ok {
			continue
		}
		out = append(out, mapping)
	}
	return out
}

func buildGatewayModelResourceRows(
	models []modelcontract.Model,
	mappings []modelcontract.ModelProviderMapping,
	providers []providercontract.Provider,
	routableAccountsByProvider map[int][]accountcontract.ProviderAccount,
	activeKeys []apikeycontract.APIKey,
) []apiopenapi.GatewayModelResourceRow {
	activeProviderIDs := make(map[int]struct{}, len(providers))
	for _, provider := range providers {
		if provider.Status == providercontract.StatusActive {
			activeProviderIDs[provider.ID] = struct{}{}
		}
	}
	mappingsByModelID := make(map[int][]modelcontract.ModelProviderMapping)
	for _, mapping := range mappings {
		mappingsByModelID[mapping.ModelID] = append(mappingsByModelID[mapping.ModelID], mapping)
	}

	rows := make([]apiopenapi.GatewayModelResourceRow, 0, len(models))
	for _, model := range models {
		modelMappings := mappingsByModelID[model.ID]
		activeProviderCount := 0
		routableAccountCount := 0
		seenProviders := make(map[int]struct{}, len(modelMappings))
		for _, mapping := range modelMappings {
			if _, ok := seenProviders[mapping.ProviderID]; ok {
				continue
			}
			seenProviders[mapping.ProviderID] = struct{}{}
			if _, ok := activeProviderIDs[mapping.ProviderID]; !ok {
				continue
			}
			activeProviderCount++
			routableAccountCount += len(routableAccountsByProvider[mapping.ProviderID])
		}
		apiKeyCount, scopedKeyCount := apiKeysForGatewayModel(activeKeys, model)
		reasons := gatewayModelResourceReasons(len(modelMappings), activeProviderCount, routableAccountCount, apiKeyCount)
		status := apiopenapi.GatewayProviderResourceStatusReady
		if len(reasons) > 0 {
			status = apiopenapi.GatewayProviderResourceStatusBlocked
			if len(modelMappings) > 0 && (routableAccountCount > 0 || apiKeyCount > 0) {
				status = apiopenapi.GatewayProviderResourceStatusLimited
			}
		}
		rows = append(rows, apiopenapi.GatewayModelResourceRow{
			ActiveModelMappings: len(modelMappings),
			ActiveProviders:     activeProviderCount,
			ApiKeyCount:         apiKeyCount,
			Model:               toAPIModel(model),
			Reasons:             reasons,
			RoutableAccounts:    routableAccountCount,
			ScopedKeyCount:      scopedKeyCount,
			Status:              status,
		})
	}
	return rows
}

func apiKeysForGatewayModel(keys []apikeycontract.APIKey, model modelcontract.Model) (int, int) {
	count := 0
	scoped := 0
	for _, key := range keys {
		if apiKeyAllowsModel(key.AllowedModels, model.CanonicalName) {
			count++
			if apiKeyIsScoped(key) {
				scoped++
			}
		}
	}
	return count, scoped
}

func gatewayModelResourceReasons(activeMappings int, activeProviders int, routableAccounts int, apiKeys int) []apiopenapi.GatewayProviderResourceReason {
	reasons := make([]apiopenapi.GatewayProviderResourceReason, 0, 3)
	if activeMappings == 0 || activeProviders == 0 {
		reasons = append(reasons, apiopenapi.NoModelMappings)
	}
	if routableAccounts == 0 {
		reasons = append(reasons, apiopenapi.NoRoutableAccounts)
	}
	if apiKeys == 0 {
		reasons = append(reasons, apiopenapi.NoApiKeys)
	}
	return reasons
}

func activeResourceInventoryAccounts(accounts []accountcontract.ProviderAccount) []accountcontract.ProviderAccount {
	out := make([]accountcontract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Status == accountcontract.StatusArchived {
			continue
		}
		out = append(out, account)
	}
	return out
}

func accountsByProvider(accounts []accountcontract.ProviderAccount) map[int][]accountcontract.ProviderAccount {
	out := make(map[int][]accountcontract.ProviderAccount)
	for _, account := range accounts {
		out[account.ProviderID] = append(out[account.ProviderID], account)
	}
	return out
}

func groupIDsByProvider(accounts []accountcontract.ProviderAccount, groupIDsByAccount map[int][]int) map[int]map[int]struct{} {
	out := make(map[int]map[int]struct{})
	for _, account := range accounts {
		groupIDs := groupIDsByAccount[account.ID]
		if len(groupIDs) == 0 {
			continue
		}
		groups, ok := out[account.ProviderID]
		if !ok {
			groups = make(map[int]struct{})
			out[account.ProviderID] = groups
		}
		for _, groupID := range groupIDs {
			groups[groupID] = struct{}{}
		}
	}
	return out
}

func mappingsByProvider(mappings []modelcontract.ModelProviderMapping) map[int][]modelcontract.ModelProviderMapping {
	out := make(map[int][]modelcontract.ModelProviderMapping)
	for _, mapping := range mappings {
		out[mapping.ProviderID] = append(out[mapping.ProviderID], mapping)
	}
	return out
}

func filterActiveAccounts(accounts []accountcontract.ProviderAccount) []accountcontract.ProviderAccount {
	out := make([]accountcontract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Status == accountcontract.StatusActive {
			out = append(out, account)
		}
	}
	return out
}

func filterProviderAPIKeys(keys []apikeycontract.APIKey, providerGroupIDs map[int]struct{}) []apikeycontract.APIKey {
	out := make([]apikeycontract.APIKey, 0, len(keys))
	for _, key := range keys {
		if len(key.GroupIDs) == 0 {
			out = append(out, key)
			continue
		}
		for _, groupID := range key.GroupIDs {
			if _, ok := providerGroupIDs[groupID]; ok {
				out = append(out, key)
				break
			}
		}
	}
	return out
}

func apiKeyIsScoped(key apikeycontract.APIKey) bool {
	return len(key.AllowedModels) > 0 || len(key.GroupIDs) > 0
}

func accountIsGatewayResourceRoutable(account accountcontract.ProviderAccount, health accountcontract.AccountHealthSnapshot, hasHealth bool, quotaExhausted bool) bool {
	if account.Status != accountcontract.StatusActive {
		return false
	}
	if quotaExhausted || accountQuotaExhausted(account, accountQuotaRemainingRatio(account)) {
		return false
	}
	if !hasHealth {
		return true
	}
	if health.CircuitState == "open" {
		return false
	}
	switch health.Status {
	case string(accountcontract.StatusDead), string(accountcontract.StatusSuspended):
		return false
	default:
		return true
	}
}

func quotaExhaustedAccounts(quotasByAccount map[int][]accountcontract.AccountQuotaSnapshot, now time.Time) map[int]bool {
	out := make(map[int]bool)
	for accountID, snapshots := range quotasByAccount {
		if snapshot, ok := mostConstrainedActiveRealQuotaSnapshotAt(snapshots, now); ok && snapshot.RemainingRatio <= 0 {
			out[accountID] = true
		}
	}
	return out
}

func mostConstrainedActiveRealQuotaSnapshotAt(snapshots []accountcontract.AccountQuotaSnapshot, now time.Time) (accountcontract.AccountQuotaSnapshot, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var selected accountcontract.AccountQuotaSnapshot
	found := false
	for _, snapshot := range snapshots {
		if accountcontract.IsSyntheticQuotaSnapshot(snapshot) || quotaSnapshotWindowReset(snapshot, now) {
			continue
		}
		if !found || snapshot.RemainingRatio < selected.RemainingRatio {
			selected = snapshot
			found = true
		}
	}
	return selected, found
}

func gatewayProviderResourceReasons(provider providercontract.Provider, activeModels int, activeMappings int, activeAccounts int, routableAccounts int, proxyAttentionAccounts int, apiKeys int) []apiopenapi.GatewayProviderResourceReason {
	reasons := make([]apiopenapi.GatewayProviderResourceReason, 0, 4)
	if provider.Status != providercontract.StatusActive {
		reasons = append(reasons, apiopenapi.ProviderDisabled)
	}
	if activeModels == 0 {
		reasons = append(reasons, apiopenapi.NoActiveModels)
	} else if activeMappings == 0 {
		reasons = append(reasons, apiopenapi.NoModelMappings)
	}
	if activeAccounts == 0 {
		reasons = append(reasons, apiopenapi.NoActiveAccounts)
	} else if routableAccounts == 0 {
		reasons = append(reasons, apiopenapi.NoRoutableAccounts)
	}
	if proxyAttentionAccounts > 0 {
		reasons = append(reasons, apiopenapi.ProxyAttention)
	}
	if apiKeys == 0 {
		reasons = append(reasons, apiopenapi.NoApiKeys)
	}
	return reasons
}

type proxyResourceState struct {
	Available bool
	Attention bool
	Expired   bool
}

func buildProxyResourceStates(proxies []accountcontract.ProxyDefinition, now time.Time) map[string]proxyResourceState {
	byID := make(map[string]accountcontract.ProxyDefinition, len(proxies))
	for _, proxy := range proxies {
		byID[strconv.Itoa(proxy.ID)] = proxy
	}
	states := make(map[string]proxyResourceState, len(proxies))
	resolving := make(map[string]struct{})
	var resolve func(id string) (proxyResourceState, bool)
	resolve = func(id string) (proxyResourceState, bool) {
		if state, ok := states[id]; ok {
			return state, true
		}
		proxy, ok := byID[id]
		if !ok {
			return proxyResourceState{}, false
		}
		expired := proxyIsExpiredResource(proxy, now)
		if _, ok := resolving[id]; ok {
			return proxyResourceState{Available: false, Attention: true, Expired: expired}, true
		}
		resolving[id] = struct{}{}
		usablePrimary := proxy.Status == accountcontract.ProxyStatusActive && proxy.URLCiphertext != ""
		available := usablePrimary && !expired
		if !available && usablePrimary && expired {
			switch proxy.FallbackMode {
			case accountcontract.ProxyFallbackModeDirect:
				available = true
			case accountcontract.ProxyFallbackModeProxy:
				if proxy.BackupProxyID != nil {
					if backup, ok := resolve(strconv.Itoa(*proxy.BackupProxyID)); ok {
						available = backup.Available
					}
				}
			}
		}
		state := proxyResourceState{
			Available: available,
			Attention: proxy.Status != accountcontract.ProxyStatusActive || proxy.URLCiphertext == "" || expired,
			Expired:   expired,
		}
		states[id] = state
		delete(resolving, id)
		return state, true
	}
	for _, proxy := range proxies {
		resolve(strconv.Itoa(proxy.ID))
	}
	return states
}

func accountProxyCanRouteResource(account accountcontract.ProviderAccount, proxies map[string]proxyResourceState) bool {
	proxyID := accountProxyID(account)
	if proxyID == "" {
		return true
	}
	state, ok := proxies[proxyID]
	return ok && state.Available
}

func accountNeedsProxyResourceAttention(account accountcontract.ProviderAccount, proxies map[string]proxyResourceState) bool {
	proxyID := accountProxyID(account)
	if proxyID == "" {
		return false
	}
	state, ok := proxies[proxyID]
	return !ok || state.Attention
}

func accountProxyID(account accountcontract.ProviderAccount) string {
	if account.ProxyID == nil {
		return ""
	}
	return strings.TrimSpace(*account.ProxyID)
}

func proxyIsExpiredResource(proxy accountcontract.ProxyDefinition, now time.Time) bool {
	if proxy.ExpiresAt == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !proxy.ExpiresAt.After(now)
}

func sumProviderResourceRows(rows []apiopenapi.GatewayProviderResourceRow, value func(apiopenapi.GatewayProviderResourceRow) int) int {
	sum := 0
	for _, row := range rows {
		sum += value(row)
	}
	return sum
}
