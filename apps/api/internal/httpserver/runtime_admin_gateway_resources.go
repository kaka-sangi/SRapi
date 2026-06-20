package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
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
		Context:           ctx,
		Providers:         providers,
		Accounts:          accounts,
		Proxies:           proxies,
		Models:            models,
		ModelMappings:     modelMappings,
		APIKeys:           apiKeys,
		HealthByAccount:   healthByAccount,
		QuotasByAccount:   quotasByAccount,
		GroupIDsByAccount: groupIDsByAccount,
		Billing:           s.runtime.billing,
		Now:               time.Now().UTC(),
	}, nil
}

type gatewayResourceSummaryInput struct {
	Context           context.Context
	Providers         []providercontract.Provider
	Accounts          []accountcontract.ProviderAccount
	Proxies           []accountcontract.ProxyDefinition
	Models            []modelcontract.Model
	ModelMappings     []modelcontract.ModelProviderMapping
	APIKeys           []apikeycontract.APIKey
	HealthByAccount   map[int]accountcontract.AccountHealthSnapshot
	QuotasByAccount   map[int][]accountcontract.AccountQuotaSnapshot
	GroupIDsByAccount map[int][]int
	Billing           *billingservice.Service
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
		ModelRows:              buildGatewayModelResourceRows(input.Context, activeModels, activeModelMappings, input.Providers, routableAccountsByProvider, activeKeys, input.Billing, now),
		Providers:              len(input.Providers),
		ProxiedAccounts:        sumProviderResourceRows(rows, func(row apiopenapi.GatewayProviderResourceRow) int { return row.ProxiedAccounts }),
		ProxyAttentionAccounts: sumProviderResourceRows(rows, func(row apiopenapi.GatewayProviderResourceRow) int { return row.ProxyAttentionAccounts }),
		RoutableAccounts:       sumProviderResourceRows(rows, func(row apiopenapi.GatewayProviderResourceRow) int { return row.RoutableAccounts }),
		RouteRows:              buildGatewayRouteResourceRows(input.Context, activeModels, activeModelMappings, input.Providers, routableAccountsByProvider, providerGroupIDs, activeKeys, input.Billing, now),
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
	ctx context.Context,
	models []modelcontract.Model,
	mappings []modelcontract.ModelProviderMapping,
	providers []providercontract.Provider,
	routableAccountsByProvider map[int][]accountcontract.ProviderAccount,
	activeKeys []apikeycontract.APIKey,
	billing *billingservice.Service,
	now time.Time,
) []apiopenapi.GatewayModelResourceRow {
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
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
		seenProviders := make(map[int]struct{}, len(modelMappings))
		endpointAccounts := newGatewayEndpointResourceCounts()
		pricingCoverage := newGatewayPricingCoverageCounts()
		for _, mapping := range modelMappings {
			if _, ok := activeProviderIDs[mapping.ProviderID]; !ok {
				continue
			}
			provider, ok := providerByID(providers, mapping.ProviderID)
			if !ok {
				continue
			}
			if _, ok := seenProviders[mapping.ProviderID]; !ok {
				seenProviders[mapping.ProviderID] = struct{}{}
				activeProviderCount++
			}
			for _, account := range routableAccountsByProvider[mapping.ProviderID] {
				endpointAccounts.addAccount(ctx, model, mapping, provider, account, billing, now, &pricingCoverage)
			}
		}
		routableAccountCount := endpointAccounts.uniqueAccountCount()
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
			Endpoints:           endpointAccounts.rows(),
			Model:               toAPIModel(model),
			Pricing:             pricingCoverage.row(),
			Reasons:             reasons,
			RoutableAccounts:    routableAccountCount,
			ScopedKeyCount:      scopedKeyCount,
			Status:              status,
		})
	}
	return rows
}

func buildGatewayRouteResourceRows(
	ctx context.Context,
	models []modelcontract.Model,
	mappings []modelcontract.ModelProviderMapping,
	providers []providercontract.Provider,
	routableAccountsByProvider map[int][]accountcontract.ProviderAccount,
	providerGroupIDs map[int]map[int]struct{},
	activeKeys []apikeycontract.APIKey,
	billing *billingservice.Service,
	now time.Time,
) []apiopenapi.GatewayRouteResourceRow {
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	modelsByID := make(map[int]modelcontract.Model, len(models))
	for _, model := range models {
		modelsByID[model.ID] = model
	}
	rows := make([]apiopenapi.GatewayRouteResourceRow, 0, len(mappings))
	for _, mapping := range mappings {
		model, ok := modelsByID[mapping.ModelID]
		if !ok {
			continue
		}
		provider, ok := providerByID(providers, mapping.ProviderID)
		if !ok || provider.Status != providercontract.StatusActive {
			continue
		}
		endpointAccounts := newGatewayEndpointResourceCounts()
		pricingCoverage := newGatewayPricingCoverageCounts()
		upstreamModel := strings.TrimSpace(mapping.UpstreamModelName)
		for _, account := range routableAccountsByProvider[mapping.ProviderID] {
			endpointAccounts.addAccount(ctx, model, mapping, provider, account, billing, now, &pricingCoverage)
			upstreamModel = gatewayRouteDisplayUpstreamModel(model, mapping, provider, account, upstreamModel)
		}
		routableAccountCount := endpointAccounts.uniqueAccountCount()
		apiKeyCount, scopedKeyCount := apiKeysForGatewayRoute(activeKeys, providerGroupIDs[mapping.ProviderID], model)
		reasons := gatewayRouteResourceReasons(routableAccountCount, apiKeyCount)
		status := apiopenapi.GatewayProviderResourceStatusReady
		if len(reasons) > 0 {
			status = apiopenapi.GatewayProviderResourceStatusBlocked
			if routableAccountCount > 0 || apiKeyCount > 0 {
				status = apiopenapi.GatewayProviderResourceStatusLimited
			}
		}
		rows = append(rows, apiopenapi.GatewayRouteResourceRow{
			ApiKeyCount:      apiKeyCount,
			Endpoints:        endpointAccounts.rows(),
			MappingId:        apiopenapi.Id(strconv.Itoa(mapping.ID)),
			Model:            toAPIModel(model),
			Pricing:          pricingCoverage.row(),
			Provider:         toAPIProvider(provider),
			Reasons:          reasons,
			RoutableAccounts: routableAccountCount,
			ScopedKeyCount:   scopedKeyCount,
			Status:           status,
			UpstreamModel:    upstreamModel,
		})
	}
	return rows
}

func gatewayRouteDisplayUpstreamModel(model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, account accountcontract.ProviderAccount, fallback string) string {
	for _, key := range gatewayEndpointResourceKeys {
		effectiveMapping := gatewayResourceEffectiveModelMapping(model, mapping, provider, account, gatewayEndpointSourceEndpoint(key))
		if !gatewayResourceAccountCanServeMapping(model, provider, account, effectiveMapping) {
			continue
		}
		if upstream := strings.TrimSpace(effectiveMapping.UpstreamModelName); upstream != "" {
			return upstream
		}
	}
	if fallback != "" {
		return fallback
	}
	return model.CanonicalName
}

type gatewayEndpointResourceCounts struct {
	accountIDs map[string]map[int]struct{}
}

type gatewayPricingCoverageCounts struct {
	TotalRoutes           int
	PricedRoutes          int
	MappingOverrideRoutes int
	PricingRuleRoutes     int
	DefaultZero           int
	Errors                int
	Source                apiopenapi.GatewayPricingCoverageSource
	PricingRuleID         *int
	Currency              string
	BillingMode           apiopenapi.BillingMode
}

func newGatewayEndpointResourceCounts() gatewayEndpointResourceCounts {
	return gatewayEndpointResourceCounts{accountIDs: make(map[string]map[int]struct{}, len(gatewayEndpointResourceKeys))}
}

func newGatewayPricingCoverageCounts() gatewayPricingCoverageCounts {
	return gatewayPricingCoverageCounts{
		Source:      apiopenapi.GatewayPricingCoverageSourceDefaultZero,
		Currency:    "USD",
		BillingMode: apiopenapi.Token,
	}
}

var gatewayEndpointResourceKeys = []string{
	capabilitiescontract.KeyChatCompletions,
	capabilitiescontract.KeyResponses,
	capabilitiescontract.KeyResponsesCompact,
	capabilitiescontract.KeyResponsesInputItems,
	capabilitiescontract.KeyMessages,
	capabilitiescontract.KeyAnthropicCountTokens,
	capabilitiescontract.KeyGeminiGenerateContent,
	capabilitiescontract.KeyGeminiCountTokens,
}

func (counts gatewayEndpointResourceCounts) addAccount(ctx context.Context, model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, account accountcontract.ProviderAccount, billing *billingservice.Service, now time.Time, pricing *gatewayPricingCoverageCounts) {
	supported := gatewaySupportedCapabilityKeys(effectiveCapabilities(model, mapping, provider, account))
	for _, key := range gatewayEndpointResourceKeys {
		if _, ok := supported[key]; !ok {
			continue
		}
		effectiveMapping := gatewayResourceEffectiveModelMapping(model, mapping, provider, account, gatewayEndpointSourceEndpoint(key))
		if !gatewayResourceAccountCanServeMapping(model, provider, account, effectiveMapping) {
			continue
		}
		values, ok := counts.accountIDs[key]
		if !ok {
			values = make(map[int]struct{})
			counts.accountIDs[key] = values
		}
		values[account.ID] = struct{}{}
		if pricing != nil {
			pricing.addRoute(ctx, model, effectiveMapping, provider, billing, now)
		}
	}
}

func (counts *gatewayPricingCoverageCounts) addRoute(ctx context.Context, model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, billing *billingservice.Service, now time.Time) {
	counts.TotalRoutes++
	if billing == nil {
		counts.Errors++
		counts.Source = apiopenapi.GatewayPricingCoverageSourcePricingError
		return
	}
	coverage, err := billing.PricingCoverage(ctx, billingcontract.PricingRequest{
		ModelID:            model.ID,
		ModelFamily:        optionalStringValue(model.Family),
		ProviderID:         provider.ID,
		RequestedModel:     model.CanonicalName,
		UpstreamModel:      mapping.UpstreamModelName,
		BillingModelSource: mapString(mapping.PricingOverride, "billing_model_source"),
		At:                 now,
		PricingOverride:    cloneAnyMap(mapping.PricingOverride),
	})
	if err != nil {
		counts.Errors++
		counts.Source = apiopenapi.GatewayPricingCoverageSourcePricingError
		return
	}
	counts.Currency = strings.TrimSpace(coverage.Currency)
	counts.BillingMode = apiopenapi.BillingMode(coverage.BillingMode)
	switch coverage.Source {
	case billingcontract.PricingCoverageSourceMappingOverride:
		counts.PricedRoutes++
		counts.MappingOverrideRoutes++
		counts.Source = apiopenapi.GatewayPricingCoverageSourceMappingOverride
	case billingcontract.PricingCoverageSourcePricingRule:
		counts.PricedRoutes++
		counts.PricingRuleRoutes++
		counts.Source = apiopenapi.GatewayPricingCoverageSourcePricingRule
		if counts.PricingRuleID == nil {
			counts.PricingRuleID = cloneIntPtr(coverage.PricingRuleID)
		}
	default:
		counts.DefaultZero++
		if counts.Source != apiopenapi.GatewayPricingCoverageSourcePricingError {
			counts.Source = apiopenapi.GatewayPricingCoverageSourceDefaultZero
		}
	}
}

func (counts gatewayPricingCoverageCounts) row() apiopenapi.GatewayPricingCoverage {
	status := apiopenapi.GatewayPricingCoverageStatusEstimatedZero
	if counts.Errors > 0 {
		status = apiopenapi.GatewayPricingCoverageStatusError
	} else if counts.TotalRoutes > 0 && counts.PricedRoutes == counts.TotalRoutes {
		status = apiopenapi.GatewayPricingCoverageStatusPriced
	}
	source := counts.Source
	switch {
	case counts.Errors > 0:
		source = apiopenapi.GatewayPricingCoverageSourcePricingError
	case counts.DefaultZero > 0 || counts.PricedRoutes == 0:
		source = apiopenapi.GatewayPricingCoverageSourceDefaultZero
	case counts.MappingOverrideRoutes > 0:
		source = apiopenapi.GatewayPricingCoverageSourceMappingOverride
	case counts.PricingRuleRoutes > 0:
		source = apiopenapi.GatewayPricingCoverageSourcePricingRule
	}
	currency := strings.TrimSpace(counts.Currency)
	if currency == "" {
		currency = "USD"
	}
	billingMode := counts.BillingMode
	if billingMode == "" {
		billingMode = apiopenapi.Token
	}
	return apiopenapi.GatewayPricingCoverage{
		BillingMode:   &billingMode,
		Currency:      &currency,
		PricedRoutes:  counts.PricedRoutes,
		PricingRuleId: cloneIntPtr(counts.PricingRuleID),
		Source:        source,
		Status:        status,
		TotalRoutes:   counts.TotalRoutes,
	}
}

func gatewaySupportedCapabilityKeys(capabilities []capabilitiescontract.Descriptor) map[string]struct{} {
	supported := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability.Level == capabilitiescontract.DescriptorLevelUnsupported || capability.Status == capabilitiescontract.DescriptorStatusDeprecated {
			continue
		}
		supported[strings.TrimSpace(capability.Key)] = struct{}{}
	}
	return supported
}

func gatewayResourceEffectiveModelMapping(model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, account accountcontract.ProviderAccount, sourceEndpoint string) modelcontract.ModelProviderMapping {
	effectiveMapping := accountEffectiveModelMapping(mapping, account, model.CanonicalName, sourceEndpoint)
	return providerEffectiveModelMapping(provider, effectiveMapping)
}

func gatewayResourceAccountCanServeMapping(model modelcontract.Model, provider providercontract.Provider, account accountcontract.ProviderAccount, mapping modelcontract.ModelProviderMapping) bool {
	if accountExcludesModel(account.Metadata, model.CanonicalName, mapping.UpstreamModelName) {
		return false
	}
	return accountRoutableForModel(provider, account.Metadata, mapping.UpstreamModelName)
}

func gatewayEndpointSourceEndpoint(key string) string {
	switch key {
	case capabilitiescontract.KeyChatCompletions:
		return "/v1/chat/completions"
	case capabilitiescontract.KeyResponses:
		return "/v1/responses"
	case capabilitiescontract.KeyResponsesCompact:
		return "/v1/responses/compact"
	case capabilitiescontract.KeyResponsesInputItems:
		return "/v1/responses/{response_id}/input_items"
	case capabilitiescontract.KeyMessages:
		return "/v1/messages"
	case capabilitiescontract.KeyAnthropicCountTokens:
		return "/v1/messages/count_tokens"
	case capabilitiescontract.KeyGeminiGenerateContent:
		return "/v1beta/models/{model}:generateContent"
	case capabilitiescontract.KeyGeminiCountTokens:
		return "/v1beta/models/{model}:countTokens"
	default:
		return ""
	}
}

func (counts gatewayEndpointResourceCounts) uniqueAccountCount() int {
	seen := make(map[int]struct{})
	for _, values := range counts.accountIDs {
		for accountID := range values {
			seen[accountID] = struct{}{}
		}
	}
	return len(seen)
}

func (counts gatewayEndpointResourceCounts) rows() []apiopenapi.GatewayEndpointResourceRow {
	rows := make([]apiopenapi.GatewayEndpointResourceRow, 0, len(gatewayEndpointResourceKeys))
	for _, key := range gatewayEndpointResourceKeys {
		status := apiopenapi.GatewayProviderResourceStatusBlocked
		routableAccounts := len(counts.accountIDs[key])
		if routableAccounts > 0 {
			status = apiopenapi.GatewayProviderResourceStatusReady
		}
		rows = append(rows, apiopenapi.GatewayEndpointResourceRow{
			Key:              apiopenapi.GatewayEndpointResourceRowKey(key),
			RoutableAccounts: routableAccounts,
			Status:           status,
		})
	}
	return rows
}

func providerByID(providers []providercontract.Provider, providerID int) (providercontract.Provider, bool) {
	for _, provider := range providers {
		if provider.ID == providerID {
			return provider, true
		}
	}
	return providercontract.Provider{}, false
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

func apiKeysForGatewayRoute(keys []apikeycontract.APIKey, providerGroupIDs map[int]struct{}, model modelcontract.Model) (int, int) {
	return apiKeysForGatewayModel(filterProviderAPIKeys(keys, providerGroupIDs), model)
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

func gatewayRouteResourceReasons(routableAccounts int, apiKeys int) []apiopenapi.GatewayProviderResourceReason {
	reasons := make([]apiopenapi.GatewayProviderResourceReason, 0, 2)
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
