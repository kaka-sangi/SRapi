package httpserver

import (
	"context"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIUser(user userscontract.User) apiopenapi.User {
	roles := make([]apiopenapi.UserRole, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, apiopenapi.UserRole(role))
	}
	return apiopenapi.User{
		Balance:         user.Balance,
		CreatedAt:       user.CreatedAt,
		Currency:        user.Currency,
		Email:           openapi_types.Email(user.Email),
		Id:              apiopenapi.Id(strconv.Itoa(user.ID)),
		LastLoginAt:     user.LastLoginAt,
		Name:            user.Name,
		Roles:           roles,
		RpmLimit:        user.RPMLimit,
		Status:          apiopenapi.UserStatus(user.Status),
		EmailVerifiedAt: user.EmailVerifiedAt,
		AvatarUrl:       optionalString(user.AvatarURL),
		AvatarMime:      optionalUserAvatarMime(user.AvatarMIME),
		AvatarByteSize:  optionalPositiveInt(user.AvatarByteSize),
		AvatarSha256:    optionalString(user.AvatarSHA256),
		AvatarUpdatedAt: user.AvatarUpdatedAt,
	}
}

func toAPICurrentUserAuthIdentities(identities []userscontract.UserAuthIdentity) []apiopenapi.CurrentUserAuthIdentity {
	out := make([]apiopenapi.CurrentUserAuthIdentity, 0, len(identities))
	for _, identity := range identities {
		out = append(out, apiopenapi.CurrentUserAuthIdentity{
			AvatarUrl:       optionalString(identity.AvatarURL),
			CanUnbind:       identity.CanUnbind,
			CreatedAt:       identity.CreatedAt,
			DisplayName:     optionalString(identity.DisplayName),
			Email:           optionalAuthIdentityEmail(identity.Email),
			EmailVerified:   identity.EmailVerified,
			External:        identity.External,
			Id:              optionalIdentityID(identity.ID),
			LastUsedAt:      identity.LastUsedAt,
			Provider:        apiopenapi.AuthIdentityProvider(identity.Provider),
			ProviderKey:     identity.ProviderKey,
			SubjectHint:     optionalString(identity.SubjectHint),
			UnbindBlockedBy: optionalString(identity.UnbindBlockedBy),
			UpdatedAt:       identity.UpdatedAt,
			UserId:          apiopenapi.Id(strconv.Itoa(identity.UserID)),
			VerifiedAt:      identity.VerifiedAt,
		})
	}
	return out
}

func optionalUserAvatarMime(value string) *apiopenapi.UserAvatarMime {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	out := apiopenapi.UserAvatarMime(value)
	return &out
}

func optionalPositiveInt(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func optionalAuthIdentityEmail(value string) *openapi_types.Email {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	email := openapi_types.Email(value)
	return &email
}

func optionalIdentityID(id int) *apiopenapi.Id {
	if id <= 0 {
		return nil
	}
	value := apiopenapi.Id(strconv.Itoa(id))
	return &value
}

func optionalStringValuePtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func toAPIRole(role userscontract.RoleDefinition) apiopenapi.Role {
	return apiopenapi.Role{
		CreatedAt:   role.CreatedAt,
		Description: role.Description,
		Id:          apiopenapi.Id(strconv.Itoa(role.ID)),
		Name:        apiopenapi.UserRole(role.Name),
		Permissions: append([]string(nil), role.Permissions...),
		UpdatedAt:   role.UpdatedAt,
	}
}

func toAPIKey(key apikeycontract.APIKey) apiopenapi.ApiKey {
	groupIDs := make([]apiopenapi.Id, 0, len(key.GroupIDs))
	for _, id := range key.GroupIDs {
		groupIDs = append(groupIDs, apiopenapi.Id(strconv.Itoa(id)))
	}
	return apiopenapi.ApiKey{
		AllowedModels:    append([]string(nil), key.AllowedModels...),
		AllowedIps:       append([]string{}, key.AllowedIPs...),
		DeniedIps:        append([]string{}, key.DeniedIPs...),
		CreatedAt:        key.CreatedAt,
		CostLimit1d:      key.CostLimit1d,
		CostLimit5h:      key.CostLimit5h,
		CostLimit7d:      key.CostLimit7d,
		CostQuota:        key.CostQuota,
		CostUsed:         optionalStringValuePtr(key.CostUsed),
		CostUsed1d:       optionalStringValuePtr(key.CostUsed1d),
		CostUsed5h:       optionalStringValuePtr(key.CostUsed5h),
		CostUsed7d:       optionalStringValuePtr(key.CostUsed7d),
		ExpiresAt:        key.ExpiresAt,
		GroupIds:         groupIDs,
		Id:               apiopenapi.Id(strconv.Itoa(key.ID)),
		LastUsedAt:       key.LastUsedAt,
		Name:             key.Name,
		Prefix:           key.Prefix,
		ConcurrencyLimit: key.ConcurrencyLimit,
		RpmLimit:         key.RPMLimit,
		RequestLimit5h:   key.RequestLimit5h,
		RequestLimit1d:   key.RequestLimit1d,
		RequestLimit7d:   key.RequestLimit7d,
		Scopes:           append([]string(nil), key.Scopes...),
		Status:           apiopenapi.ApiKeyStatus(key.Status),
		TpmLimit:         key.TPMLimit,
	}
}

func toAPIProvider(provider providercontract.Provider) apiopenapi.Provider {
	return apiopenapi.Provider{
		AdapterType:    apiopenapi.ProviderAdapterType(provider.AdapterType),
		AuthMethods:    providerAuthMethodsAPI(provider.ConfigSchema),
		Capabilities:   mapToJsonObjectPtr(provider.Capabilities),
		ConfigSchema:   mapToJsonObjectPtr(provider.ConfigSchema),
		CreatedAt:      provider.CreatedAt,
		DisplayName:    provider.DisplayName,
		Id:             apiopenapi.Id(strconv.Itoa(provider.ID)),
		Name:           provider.Name,
		PlatformFamily: providerPlatformFamilyAPI(provider.ConfigSchema),
		Protocol:       apiopenapi.ProviderProtocol(provider.Protocol),
		Status:         apiopenapi.ResourceStatus(provider.Status),
	}
}

// providerAuthMethodsAPI surfaces the provider's auth_methods allowlist (stored
// in config_schema by the preset installer) as the typed OpenAPI field so the
// admin UI can scope the authentication-method selector per provider. Returns
// nil when the provider carries no allowlist (no restriction).
func providerAuthMethodsAPI(configSchema map[string]any) *[]apiopenapi.RuntimeClass {
	methods := providerAuthMethodStrings(configSchema)
	if len(methods) == 0 {
		return nil
	}
	out := make([]apiopenapi.RuntimeClass, 0, len(methods))
	for _, method := range methods {
		out = append(out, apiopenapi.RuntimeClass(method))
	}
	return &out
}

func providerPlatformFamilyAPI(configSchema map[string]any) *apiopenapi.PlatformFamily {
	family, ok := configSchema["platform_family"].(string)
	if !ok || family == "" {
		return nil
	}
	value := apiopenapi.PlatformFamily(family)
	return &value
}

func toAPIModel(model modelcontract.Model) apiopenapi.Model {
	return apiopenapi.Model{
		CanonicalName:   model.CanonicalName,
		Capabilities:    toAPICapabilityDescriptors(model.Capabilities),
		ContextWindow:   model.ContextWindow,
		CreatedAt:       model.CreatedAt,
		DisplayName:     model.DisplayName,
		Family:          model.Family,
		Id:              apiopenapi.Id(strconv.Itoa(model.ID)),
		MaxOutputTokens: model.MaxOutputTokens,
		QualityTier:     model.QualityTier,
		Status:          apiopenapi.ResourceStatus(model.Status),
	}
}

func toAPIModelAlias(alias modelcontract.ModelAlias) apiopenapi.ModelAlias {
	return apiopenapi.ModelAlias{
		Alias:          alias.Alias,
		CreatedAt:      alias.CreatedAt,
		FallbackModels: alias.FallbackModels,
		Id:             apiopenapi.Id(strconv.Itoa(alias.ID)),
		ModelId:        apiopenapi.Id(strconv.Itoa(alias.ModelID)),
		Status:         apiopenapi.ResourceStatus(alias.Status),
		StrategyHint:   alias.StrategyHint,
	}
}

func toAPIModelProviderMapping(mapping modelcontract.ModelProviderMapping) apiopenapi.ModelProviderMapping {
	return apiopenapi.ModelProviderMapping{
		CapabilityOverride: toAPICapabilityDescriptorsPtr(mapping.CapabilityOverride),
		CreatedAt:          mapping.CreatedAt,
		Id:                 apiopenapi.Id(strconv.Itoa(mapping.ID)),
		ModelId:            apiopenapi.Id(strconv.Itoa(mapping.ModelID)),
		PricingOverride:    mapToJsonObjectPtr(mapping.PricingOverride),
		ProviderId:         apiopenapi.Id(strconv.Itoa(mapping.ProviderID)),
		Status:             apiopenapi.ResourceStatus(mapping.Status),
		UpstreamModelName:  mapping.UpstreamModelName,
	}
}

func toAPIAccount(account accountcontract.ProviderAccount) apiopenapi.ProviderAccount {
	return apiopenapi.ProviderAccount{
		CreatedAt:      account.CreatedAt,
		GroupIds:       []apiopenapi.Id{},
		Id:             apiopenapi.Id(strconv.Itoa(account.ID)),
		Metadata:       mapToJsonObjectPtr(account.Metadata),
		Name:           account.Name,
		Priority:       account.Priority,
		ProviderId:     apiopenapi.Id(strconv.Itoa(account.ProviderID)),
		RiskLevel:      account.RiskLevel,
		RuntimeClass:   apiopenapi.RuntimeClass(account.RuntimeClass),
		Status:         apiopenapi.ProviderAccountStatus(account.Status),
		UpstreamClient: account.UpstreamClient,
		Weight:         account.Weight,
	}
}

func apiStringPtr[T ~string](value *string) *T {
	if value == nil {
		return nil
	}
	converted := T(*value)
	return &converted
}

func stringPtrFromAPI[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	converted := string(*value)
	return &converted
}

func toAPIProxyDefinition(proxy accountcontract.ProxyDefinition) apiopenapi.ProxyDefinition {
	return apiopenapi.ProxyDefinition{
		CreatedAt:     proxy.CreatedAt,
		Id:            apiopenapi.Id(strconv.Itoa(proxy.ID)),
		Metadata:      mapToJsonObjectPtr(proxy.Metadata),
		Name:          proxy.Name,
		Status:        apiopenapi.ProxyDefinitionStatus(proxy.Status),
		Type:          apiopenapi.ProxyDefinitionType(proxy.Type),
		UpdatedAt:     proxy.UpdatedAt,
		UrlConfigured: proxy.URLCiphertext != "",
	}
}

func (s *Server) apiAccount(ctx context.Context, account accountcontract.ProviderAccount) apiopenapi.ProviderAccount {
	out := toAPIAccount(account)
	groupIDs, err := s.runtime.accounts.ListGroupIDsByAccount(ctx, account.ID)
	if err == nil {
		out.GroupIds = apiIDs(groupIDs)
	}
	return out
}

func toAPIAccountGroup(group accountcontract.AccountGroup) apiopenapi.AccountGroup {
	return apiopenapi.AccountGroup{
		CreatedAt:     group.CreatedAt,
		Description:   group.Description,
		Id:            apiopenapi.Id(strconv.Itoa(group.ID)),
		ModelScope:    jsonObject(group.ModelScope),
		Name:          group.Name,
		ProviderScope: jsonObject(group.ProviderScope),
		Status:        apiopenapi.AccountGroupStatus(group.Status),
		StrategyHint:  group.StrategyHint,
	}
}

func toAPIAccountGroupMember(member accountcontract.AccountGroupMember) apiopenapi.AccountGroupMember {
	return apiopenapi.AccountGroupMember{
		AccountGroupId: apiopenapi.Id(strconv.Itoa(member.AccountGroupID)),
		AccountId:      apiopenapi.Id(strconv.Itoa(member.AccountID)),
		CreatedAt:      member.CreatedAt,
		Id:             apiopenapi.Id(strconv.Itoa(member.ID)),
	}
}

func toAPIAccountQuotaSnapshot(snapshot accountcontract.AccountQuotaSnapshot) apiopenapi.AccountQuotaSnapshot {
	return apiopenapi.AccountQuotaSnapshot{
		AccountId:      apiopenapi.Id(strconv.Itoa(snapshot.AccountID)),
		ProviderId:     apiopenapi.Id(strconv.Itoa(snapshot.ProviderID)),
		QuotaLimit:     snapshot.QuotaLimit,
		QuotaType:      snapshot.QuotaType,
		Remaining:      snapshot.Remaining,
		RemainingRatio: snapshot.RemainingRatio,
		ResetAt:        snapshot.ResetAt,
		SnapshotAt:     snapshot.SnapshotAt,
		Used:           snapshot.Used,
	}
}

func accountHealthSnapshotFromAPI(snapshot apiopenapi.AccountHealthSnapshot) accountcontract.AccountHealthSnapshot {
	accountID, _ := strconv.Atoi(string(snapshot.AccountId))
	providerID, _ := strconv.Atoi(string(snapshot.ProviderId))
	return accountcontract.AccountHealthSnapshot{
		AccountID:      accountID,
		ProviderID:     providerID,
		Status:         snapshot.Status,
		SuccessRate:    snapshot.SuccessRate,
		ErrorRate:      snapshot.ErrorRate,
		LatencyP50MS:   snapshot.LatencyP50Ms,
		LatencyP95MS:   snapshot.LatencyP95Ms,
		RateLimitCount: snapshot.RateLimitCount,
		TimeoutCount:   snapshot.TimeoutCount,
		CooldownUntil:  cloneTimePtr(snapshot.CooldownUntil),
		CircuitState:   snapshot.CircuitState,
		SnapshotAt:     snapshot.SnapshotAt,
	}
}

func accountQuotaSnapshotFromAPI(snapshot apiopenapi.AccountQuotaSnapshot) accountcontract.AccountQuotaSnapshot {
	accountID, _ := strconv.Atoi(string(snapshot.AccountId))
	providerID, _ := strconv.Atoi(string(snapshot.ProviderId))
	return accountcontract.AccountQuotaSnapshot{
		AccountID:      accountID,
		ProviderID:     providerID,
		QuotaType:      snapshot.QuotaType,
		Remaining:      snapshot.Remaining,
		Used:           snapshot.Used,
		QuotaLimit:     snapshot.QuotaLimit,
		RemainingRatio: snapshot.RemainingRatio,
		ResetAt:        cloneTimePtr(snapshot.ResetAt),
		SnapshotAt:     snapshot.SnapshotAt,
	}
}

func overlayAccountHealthSnapshot(target *apiopenapi.AccountHealthSnapshot, latest accountcontract.AccountHealthSnapshot) {
	target.Status = latest.Status
	target.SuccessRate = latest.SuccessRate
	target.ErrorRate = latest.ErrorRate
	target.LatencyP50Ms = latest.LatencyP50MS
	target.LatencyP95Ms = latest.LatencyP95MS
	target.RateLimitCount = latest.RateLimitCount
	target.TimeoutCount = latest.TimeoutCount
	target.CooldownUntil = cloneTimePtr(latest.CooldownUntil)
	target.CircuitState = latest.CircuitState
	target.SnapshotAt = latest.SnapshotAt
}

func overlayAccountQuotaOnHealth(target *apiopenapi.AccountHealthSnapshot, latest accountcontract.AccountQuotaSnapshot) {
	target.QuotaRemainingRatio = latest.RemainingRatio
	target.QuotaExhausted = latest.RemainingRatio <= 0
}

func latestRealQuotaSnapshot(snapshots []accountcontract.AccountQuotaSnapshot) (accountcontract.AccountQuotaSnapshot, bool) {
	for _, snapshot := range snapshots {
		if !accountcontract.IsSyntheticQuotaSnapshot(snapshot) {
			return snapshot, true
		}
	}
	return accountcontract.AccountQuotaSnapshot{}, false
}
