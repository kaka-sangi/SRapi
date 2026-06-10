package httpserver

import (
	"context"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/platform/glob"
)

// Per-account model EXCLUSION (CLIProxyAPI per-channel parity). Distinct from
// the supported_models inclusion whitelist (exact-match): excluded_models holds
// "*" wildcard patterns and HIDES models. Both live in account.metadata with no
// schema migration, like model_mapping. Exclusion is checked first at
// scheduling (gatewayCandidates) and used to hide models from /v1/models.

// accountExcludedModelsMetadataKey is the provider-account metadata key holding
// a list of "*" wildcard patterns. Any catalog model (by canonical name) or
// upstream model name that matches a pattern is hidden for this account: the
// account is skipped as a scheduling candidate for that model, and the model is
// hidden from /v1/models when every account that could serve it excludes it.
// Exclusion takes precedence over the supported_models inclusion whitelist.
const accountExcludedModelsMetadataKey = "excluded_models"

// accountExcludesModel reports whether the account's excluded_models wildcard
// list matches any of the supplied model names (typically the catalog canonical
// name and the channel upstream model name). Blank patterns are ignored so an
// empty list never excludes everything. Names are compared after stripping the
// discovery "models/" prefix so patterns work regardless of that prefix.
func accountExcludesModel(metadata map[string]any, modelNames ...string) bool {
	patterns, ok := metadataStringList(metadata, accountExcludedModelsMetadataKey)
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

// modelsHiddenByExclusion returns the set of catalog canonical model names that
// should be hidden from /v1/models for the given key because every account that
// could serve the model (within the key's account-group restriction) excludes
// it via its excluded_models wildcard list. A model with no serving account at
// all is NOT hidden — listing it has always been allowed and exclusion only
// removes per-channel surface, never adds new hiding behavior. Errors loading a
// model's mappings/accounts are treated as "not hidden" so the listing degrades
// open rather than silently dropping models.
func (rt *runtimeState) modelsHiddenByExclusion(ctx context.Context, models []modelcontract.Model, apiKey apikeycontract.APIKey) map[string]struct{} {
	accounts, err := rt.accounts.List(ctx)
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
	hidden := map[string]struct{}{}
	for _, model := range models {
		mappings, err := rt.models.ListMappingsByModel(ctx, model.ID)
		if err != nil || len(mappings) == 0 {
			continue
		}
		serving := 0
		excluded := 0
		for _, mapping := range mappings {
			for _, account := range accounts {
				if account.ProviderID != mapping.ProviderID {
					continue
				}
				if len(apiKey.GroupIDs) > 0 && !intersectsInt(apiKey.GroupIDs, groupIDsByAccount[account.ID]) {
					continue
				}
				serving++
				if accountExcludesModel(account.Metadata, model.CanonicalName, mapping.UpstreamModelName) {
					excluded++
				}
			}
		}
		if serving > 0 && excluded == serving {
			hidden[model.CanonicalName] = struct{}{}
		}
	}
	return hidden
}
