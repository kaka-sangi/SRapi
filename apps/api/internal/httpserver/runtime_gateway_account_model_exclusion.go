package httpserver

import (
	"context"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	"github.com/srapi/srapi/apps/api/internal/platform/glob"
)

// Model exclusion (CLIProxyAPI oauth-excluded-models parity). Distinct from the
// supported_models inclusion whitelist: excluded_models holds "*" wildcard
// patterns and HIDES models. Provider-level exclusions live in
// provider.config_schema and account-level exclusions live in account.metadata.
// Exclusion is checked first at scheduling (gatewayCandidates) and used to hide
// models from /v1/models.

// accountExcludedModelsMetadataKey is the provider-account metadata key holding
// a list of "*" wildcard patterns. providerExcludedModelsConfigKey is the same
// policy at provider scope. Any catalog model or upstream model name matching
// either list is hidden. Exclusion takes precedence over supported_models.
const accountExcludedModelsMetadataKey = "excluded_models"
const accountExcludedModelsHyphenMetadataKey = "excluded-models"
const providerExcludedModelsConfigKey = "excluded_models"
const providerExcludedModelsHyphenConfigKey = "excluded-models"

// accountExcludesModel reports whether the account's excluded_models wildcard
// list matches any of the supplied model names (typically the catalog canonical
// name and the channel upstream model name). Blank patterns are ignored so an
// empty list never excludes everything. Names are compared after stripping the
// discovery "models/" prefix so patterns work regardless of that prefix.
func accountExcludesModel(metadata map[string]any, modelNames ...string) bool {
	patterns, ok := metadataStringList(metadata, accountExcludedModelsMetadataKey)
	if !ok {
		patterns, ok = metadataStringList(metadata, accountExcludedModelsHyphenMetadataKey)
	}
	return patternsExcludeModel(patterns, ok, modelNames...)
}

func providerExcludesModel(configSchema map[string]any, modelNames ...string) bool {
	patterns, ok := metadataStringList(configSchema, providerExcludedModelsConfigKey)
	if !ok {
		patterns, ok = metadataStringList(configSchema, providerExcludedModelsHyphenConfigKey)
	}
	return patternsExcludeModel(patterns, ok, modelNames...)
}

func providerAccountExcludesModel(provider providercontract.Provider, account accountcontract.ProviderAccount, modelNames ...string) bool {
	return providerExcludesModel(provider.ConfigSchema, modelNames...) || accountExcludesModel(account.Metadata, modelNames...)
}

func patternsExcludeModel(patterns []string, ok bool, modelNames ...string) bool {
	if !ok || len(patterns) == 0 {
		return false
	}
	for _, name := range modelNames {
		normalized := normalizeDiscoveredModelID(name)
		if normalized == "" {
			continue
		}
		if glob.MatchAny(patterns, normalized) {
			return true
		}
	}
	return false
}

// modelsHiddenByAvailability returns the catalog canonical model names that
// should be hidden from model-listing endpoints because every active account in
// scope that could serve the model is blocked by excluded_models or by the
// discovery-derived supported_models allowlist. A model with no active serving
// account remains visible: catalog-only listings are an existing compatibility
// behavior, while account metadata may only remove a concrete per-account
// surface. Store/provider lookup errors degrade open instead of silently
// dropping models.
func (rt *runtimeState) modelsHiddenByAvailability(ctx context.Context, models []modelcontract.Model, apiKey apikeycontract.APIKey, sourceEndpoint string, forcedProviderKey string) map[string]struct{} {
	mappingsByModel := make(map[int][]modelcontract.ModelProviderMapping, len(models))
	providerIDs := []int{}
	for _, model := range models {
		mappings, err := rt.models.ListMappingsByModel(ctx, model.ID)
		if err != nil {
			return nil
		}
		mappings = activeModelMappings(mappings)
		mappingsByModel[model.ID] = mappings
		providerIDs = append(providerIDs, providerIDsForMappings(mappings)...)
	}
	accounts, err := rt.accounts.ListActiveByProviderIDs(ctx, providerIDs)
	if err != nil {
		return nil
	}
	groupIDsByAccount := map[int][]int{}
	if len(apiKey.GroupIDs) > 0 {
		accountIDs := make([]int, 0, len(accounts))
		for _, account := range accounts {
			accountIDs = append(accountIDs, account.ID)
		}
		groupIDsByAccount, err = rt.accounts.ListGroupIDsByAccounts(ctx, accountIDs)
		if err != nil {
			return nil
		}
	}
	providersByID := map[int]providercontract.Provider{}
	forcedProviderKey = strings.TrimSpace(forcedProviderKey)
	hidden := map[string]struct{}{}
	for _, model := range models {
		mappings := mappingsByModel[model.ID]
		if len(mappings) == 0 {
			continue
		}
		serving := 0
		unavailable := 0
		for _, mapping := range mappings {
			provider, ok := providersByID[mapping.ProviderID]
			if !ok {
				var err error
				provider, err = rt.providers.FindByID(ctx, mapping.ProviderID)
				if err != nil {
					continue
				}
				providersByID[mapping.ProviderID] = provider
			}
			if provider.ID <= 0 {
				continue
			}
			if forcedProviderKey != "" && provider.Name != forcedProviderKey {
				continue
			}
			for _, account := range accounts {
				if account.ProviderID != mapping.ProviderID {
					continue
				}
				if len(apiKey.GroupIDs) > 0 && !intersectsInt(apiKey.GroupIDs, groupIDsByAccount[account.ID]) {
					continue
				}
				serving++
				effectiveMapping := accountEffectiveModelMapping(mapping, account, model.CanonicalName, sourceEndpoint)
				effectiveMapping = providerEffectiveModelMapping(provider, effectiveMapping)
				if accountUnavailableForModel(provider, account, model.CanonicalName, effectiveMapping.UpstreamModelName) {
					unavailable++
				}
			}
		}
		if serving > 0 && unavailable == serving {
			hidden[model.CanonicalName] = struct{}{}
		}
	}
	return hidden
}

func activeModelMappings(mappings []modelcontract.ModelProviderMapping) []modelcontract.ModelProviderMapping {
	out := make([]modelcontract.ModelProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Status != modelcontract.StatusActive {
			continue
		}
		out = append(out, mapping)
	}
	return out
}

func accountUnavailableForModel(provider providercontract.Provider, account accountcontract.ProviderAccount, canonicalModel string, upstreamModel string) bool {
	if providerAccountExcludesModel(provider, account, canonicalModel, upstreamModel) {
		return true
	}
	return !accountRoutableForModel(provider, account.Metadata, upstreamModel)
}
