package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func derefStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(*values))
	copy(cloned, *values)
	return cloned
}

func idsToInts(values *[]apiopenapi.Id) ([]int, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]int, 0, len(*values))
	for _, value := range *values {
		parsed, err := strconv.Atoi(string(value))
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", value, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func apiIDs(values []int) []apiopenapi.Id {
	if values == nil {
		return []apiopenapi.Id{}
	}
	out := make([]apiopenapi.Id, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.Id(strconv.Itoa(value)))
	}
	return out
}

func apiIDsPtr(values []int) *[]apiopenapi.Id {
	if len(values) == 0 {
		return nil
	}
	out := apiIDs(values)
	return &out
}

func apiIDsToInts(values *[]apiopenapi.Id) ([]int, error) {
	ids, err := idsToInts(values)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("invalid id %d", id)
		}
	}
	return ids, nil
}

func accountGroupMemberPathIDs(w http.ResponseWriter, r *http.Request, requestID string) (int, int, bool) {
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || groupID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account group id", requestID)
		return 0, 0, false
	}
	accountID, err := strconv.Atoi(r.PathValue("account_id"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return 0, 0, false
	}
	return groupID, accountID, true
}

func paginateApiKeys(keys []apikeycontract.APIKey, page, pageSize int) ([]apikeycontract.APIKey, int, bool) {
	total := len(keys)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= total {
		return []apikeycontract.APIKey{}, total, false
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return keys[start:end], total, end < total
}

func filterApiKeys(keys []apikeycontract.APIKey, status string) []apikeycontract.APIKey {
	status = strings.TrimSpace(status)
	if status == "" {
		return keys
	}
	out := make([]apikeycontract.APIKey, 0, len(keys))
	for _, key := range keys {
		if string(key.Status) == status {
			out = append(out, key)
		}
	}
	return out
}

func filterGatewayModels(models []apiopenapi.OpenAIModel, allowed []string) []apiopenapi.OpenAIModel {
	if len(allowed) == 0 {
		out := make([]apiopenapi.OpenAIModel, len(models))
		copy(out, models)
		return out
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, model := range allowed {
		allowedSet[model] = struct{}{}
	}
	out := make([]apiopenapi.OpenAIModel, 0, len(models))
	for _, model := range models {
		if _, ok := allowedSet[model.Id]; ok {
			out = append(out, model)
		}
	}
	return out
}

// hideGatewayModels drops models whose Id is in the hidden set (canonical
// names hidden by per-account excluded_models wildcards).
func hideGatewayModels(models []apiopenapi.OpenAIModel, hidden map[string]struct{}) []apiopenapi.OpenAIModel {
	if len(hidden) == 0 {
		return models
	}
	out := make([]apiopenapi.OpenAIModel, 0, len(models))
	for _, model := range models {
		if _, ok := hidden[model.Id]; ok {
			continue
		}
		out = append(out, model)
	}
	return out
}

func toGatewayModels(models []modelcontract.Model) []apiopenapi.OpenAIModel {
	out := make([]apiopenapi.OpenAIModel, 0, len(models))
	for _, model := range models {
		if model.Status != modelcontract.StatusActive {
			continue
		}
		created := int(model.CreatedAt.Unix())
		out = append(out, apiopenapi.OpenAIModel{
			Created: &created,
			Id:      model.CanonicalName,
			Object:  apiopenapi.OpenAIModelObjectModel,
			OwnedBy: "srapi",
		})
	}
	return out
}

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

func toAPIUsageLog(log usagecontract.UsageLog) apiopenapi.UsageLog {
	return apiopenapi.UsageLog{
		AccountId:             optionalIDString(log.AccountID),
		ApiKeyId:              apiopenapi.Id(strconv.Itoa(log.APIKeyID)),
		AttemptNo:             log.AttemptNo,
		CachedTokens:          log.CachedTokens,
		CacheCreationTokens:   ptrInt(log.CacheCreationTokens),
		CompatibilityWarnings: nonNilStrings(log.CompatibilityWarnings),
		Cost:                  log.Cost,
		CreatedAt:             log.CreatedAt,
		Currency:              log.Currency,
		ErrorClass:            log.ErrorClass,
		Id:                    apiopenapi.Id(strconv.Itoa(log.ID)),
		InputTokens:           log.InputTokens,
		LatencyMs:             log.LatencyMS,
		Model:                 log.Model,
		OutputTokens:          log.OutputTokens,
		ProviderId:            optionalIDString(log.ProviderID),
		RequestId:             log.RequestID,
		SourceEndpoint:        log.SourceEndpoint,
		SourceProtocol:        log.SourceProtocol,
		Success:               log.Success,
		TargetProtocol:        optionalString(log.TargetProtocol),
		TotalTokens:           log.TotalTokens,
		UsageEstimated:        log.UsageEstimated,
		UserId:                apiopenapi.Id(strconv.Itoa(log.UserID)),
	}
}

func toAPIUsageAggregate(aggregate usagecontract.UsageAggregate) apiopenapi.UsageAggregate {
	return apiopenapi.UsageAggregate{
		AggregateId:   aggregate.AggregateID,
		AggregateType: apiopenapi.UsageAggregateDimension(aggregate.AggregateType),
		CachedTokens:  aggregate.CachedTokens,
		Currency:      aggregate.Currency,
		ErrorCount:    aggregate.ErrorCount,
		InputTokens:   aggregate.InputTokens,
		OutputTokens:  aggregate.OutputTokens,
		RequestCount:  aggregate.RequestCount,
		SuccessCount:  aggregate.SuccessCount,
		TotalCost:     aggregate.TotalCost,
		TotalTokens:   aggregate.TotalTokens,
	}
}

func toAPIUsageAggregates(aggregates []usagecontract.UsageAggregate) []apiopenapi.UsageAggregate {
	out := make([]apiopenapi.UsageAggregate, 0, len(aggregates))
	for _, aggregate := range aggregates {
		out = append(out, toAPIUsageAggregate(aggregate))
	}
	return out
}

func toAPIUsageExport(exported usagecontract.UsageExport) apiopenapi.UsageExport {
	logs := make([]apiopenapi.UsageLog, 0, len(exported.Logs))
	for _, log := range exported.Logs {
		logs = append(logs, toAPIUsageLog(log))
	}
	return apiopenapi.UsageExport{
		ByAccount:   toAPIUsageAggregates(exported.ByAccount),
		ByModel:     toAPIUsageAggregates(exported.ByModel),
		ByUser:      toAPIUsageAggregates(exported.ByUser),
		Daily:       toAPIUsageAggregates(exported.Daily),
		GeneratedAt: exported.GeneratedAt,
		Logs:        logs,
	}
}

func toAPIAuditLog(log auditcontract.Log) apiopenapi.AuditLog {
	return apiopenapi.AuditLog{
		Action:       log.Action,
		ActorUserId:  optionalIDString(log.ActorUserID),
		After:        jsonObject(log.After),
		Before:       jsonObject(log.Before),
		CreatedAt:    log.CreatedAt,
		Id:           apiopenapi.Id(strconv.Itoa(log.ID)),
		Ip:           log.IP,
		ResourceId:   log.ResourceID,
		ResourceType: log.ResourceType,
		TraceId:      log.TraceID,
		UserAgent:    log.UserAgent,
	}
}

func toAPIBillingLedgerEntry(entry billingcontract.LedgerEntry) apiopenapi.BillingLedgerEntry {
	return apiopenapi.BillingLedgerEntry{
		Amount:        entry.Amount,
		BalanceAfter:  entry.BalanceAfter,
		BalanceBefore: entry.BalanceBefore,
		CreatedAt:     entry.CreatedAt,
		Currency:      entry.Currency,
		Id:            apiopenapi.Id(strconv.Itoa(entry.ID)),
		Metadata:      jsonObject(entry.Metadata),
		ReferenceId:   entry.ReferenceID,
		ReferenceType: entry.ReferenceType,
		Type:          apiopenapi.BillingLedgerEntryType(entry.Type),
		UserId:        apiopenapi.Id(strconv.Itoa(entry.UserID)),
	}
}

func toAPIAffiliateInviteRecord(item affiliatecontract.InviteRelationship) apiopenapi.AffiliateInviteRecord {
	return apiopenapi.AffiliateInviteRecord{
		CreatedAt:     item.CreatedAt,
		FirstPaidAt:   cloneTimePtr(item.FirstPaidAt),
		Id:            apiopenapi.Id(strconv.Itoa(item.ID)),
		InviteCodeId:  apiopenapi.Id(strconv.Itoa(item.InviteCodeID)),
		InviteeUserId: apiopenapi.Id(strconv.Itoa(item.InviteeUserID)),
		InviterUserId: apiopenapi.Id(strconv.Itoa(item.InviterUserID)),
		Status:        apiopenapi.AffiliateRelationshipStatus(item.Status),
		UpdatedAt:     item.UpdatedAt,
	}
}

func toAPIAffiliateLedgerEntry(item affiliatecontract.AffiliateLedger) apiopenapi.AffiliateLedgerEntry {
	return apiopenapi.AffiliateLedgerEntry{
		Amount:         item.Amount,
		CreatedAt:      item.CreatedAt,
		Currency:       item.Currency,
		Id:             apiopenapi.Id(strconv.Itoa(item.ID)),
		Metadata:       jsonObject(item.Metadata),
		PaymentOrderId: optionalAPIID(item.PaymentOrderID),
		ReferenceId:    item.ReferenceID,
		RelatedUserId:  apiopenapi.Id(strconv.Itoa(item.RelatedUserID)),
		SettledAt:      cloneTimePtr(item.SettledAt),
		Status:         apiopenapi.AffiliateLedgerEntryStatus(item.Status),
		SubscriptionId: optionalAPIID(item.SubscriptionID),
		Type:           apiopenapi.AffiliateLedgerEntryType(item.Type),
		UpdatedAt:      item.UpdatedAt,
		UserId:         apiopenapi.Id(strconv.Itoa(item.UserID)),
	}
}

func toAPIAffiliateSummary(summary affiliatecontract.AffiliateSummary) apiopenapi.AffiliateSummary {
	balances := make([]apiopenapi.AffiliateCurrencySummary, 0, len(summary.Balances))
	for _, balance := range summary.Balances {
		balances = append(balances, apiopenapi.AffiliateCurrencySummary{
			AccruedAmount:              balance.AccruedAmount,
			AvailableBalance:           balance.AvailableBalance,
			Currency:                   balance.Currency,
			ManualAdjustmentAmount:     balance.ManualAdjustmentAmount,
			RefundCompensatedAmount:    balance.RefundCompensatedAmount,
			SettledAmount:              balance.SettledAmount,
			TransferredToBalanceAmount: balance.TransferredToBalanceAmount,
			WithdrawnAmount:            balance.WithdrawnAmount,
		})
	}
	return apiopenapi.AffiliateSummary{
		Balances: balances,
		UserId:   apiopenapi.Id(strconv.Itoa(summary.UserID)),
	}
}

func toAPIAffiliateTransferToBalanceResult(result affiliatecontract.TransferToBalanceResult) apiopenapi.AffiliateTransferToBalanceResult {
	return apiopenapi.AffiliateTransferToBalanceResult{
		AffiliateLedger: toAPIAffiliateLedgerEntry(result.AffiliateLedger),
		Applied:         result.Applied,
		BalanceAfter:    result.BalanceAfter,
		BalanceBefore:   result.BalanceBefore,
		BillingLedgerId: apiopenapi.Id(strconv.Itoa(result.BillingLedgerID)),
		Reason:          optionalString(result.Reason),
	}
}

func toAPIPaymentMethod(method paymentcontract.PaymentMethod) apiopenapi.PaymentMethod {
	return apiopenapi.PaymentMethod{
		Metadata:           jsonObject(method.Metadata),
		Method:             method.Method,
		Name:               method.Name,
		Provider:           method.Provider,
		ProviderInstanceId: apiopenapi.Id(strconv.Itoa(method.ProviderInstanceID)),
	}
}

func toAPIPaymentProviderInstance(provider paymentcontract.PaymentProviderInstance) apiopenapi.PaymentProviderInstance {
	return apiopenapi.PaymentProviderInstance{
		ConfigVersion:    provider.ConfigVersion,
		CreatedAt:        provider.CreatedAt,
		Id:               apiopenapi.Id(strconv.Itoa(provider.ID)),
		Limits:           jsonObject(provider.Limits),
		Metadata:         jsonObject(provider.Metadata),
		Name:             provider.Name,
		Provider:         provider.Provider,
		SortOrder:        provider.SortOrder,
		Status:           apiopenapi.PaymentProviderStatus(provider.Status),
		SupportedMethods: append([]string(nil), provider.SupportedMethods...),
		UpdatedAt:        provider.UpdatedAt,
	}
}

func toAPIPaymentOrder(order paymentcontract.PaymentOrder) apiopenapi.PaymentOrder {
	return apiopenapi.PaymentOrder{
		Amount:                order.Amount,
		ClosedAt:              cloneTimePtr(order.ClosedAt),
		CreatedAt:             order.CreatedAt,
		Currency:              order.Currency,
		DiscountAmount:        defaultString(order.DiscountAmount, "0.00000000"),
		ExpiresAt:             cloneTimePtr(order.ExpiresAt),
		Id:                    apiopenapi.Id(strconv.Itoa(order.ID)),
		Metadata:              jsonObject(order.Metadata),
		OrderNo:               order.OrderNo,
		OriginalAmount:        defaultString(order.OriginalAmount, order.Amount),
		PaidAt:                cloneTimePtr(order.PaidAt),
		ProductId:             order.ProductID,
		ProductType:           apiopenapi.PaymentProductType(order.ProductType),
		PromoCodeId:           optionalAPIID(order.PromoCodeID),
		ProviderInstanceId:    apiopenapi.Id(strconv.Itoa(order.ProviderInstanceID)),
		ProviderSnapshot:      jsonObject(order.ProviderSnapshot),
		ProviderTransactionId: cloneStringPtr(order.ProviderTransactionID),
		Status:                apiopenapi.PaymentOrderStatus(order.Status),
		UpdatedAt:             order.UpdatedAt,
		UserId:                apiopenapi.Id(strconv.Itoa(order.UserID)),
	}
}

func toAPISubscriptionPlan(plan subscriptioncontract.SubscriptionPlan) apiopenapi.SubscriptionPlan {
	return apiopenapi.SubscriptionPlan{
		CreatedAt:    plan.CreatedAt,
		Currency:     plan.Currency,
		Description:  optionalString(plan.Description),
		Entitlements: jsonObject(plan.Entitlements),
		ForSale:      plan.ForSale,
		Id:           apiopenapi.Id(strconv.Itoa(plan.ID)),
		Name:         plan.Name,
		Price:        plan.Price,
		SortOrder:    plan.SortOrder,
		Status:       apiopenapi.SubscriptionPlanStatus(plan.Status),
		UpdatedAt:    plan.UpdatedAt,
		ValidityDays: plan.ValidityDays,
	}
}

func toAPIUserSubscription(subscription subscriptioncontract.UserSubscription) apiopenapi.UserSubscription {
	return apiopenapi.UserSubscription{
		CreatedAt:            subscription.CreatedAt,
		EntitlementsSnapshot: jsonObject(subscription.EntitlementsSnapshot),
		ExpiresAt:            subscription.ExpiresAt,
		Id:                   apiopenapi.Id(strconv.Itoa(subscription.ID)),
		PlanId:               apiopenapi.Id(strconv.Itoa(subscription.PlanID)),
		SourceId:             subscription.SourceID,
		SourceType:           subscription.SourceType,
		StartsAt:             subscription.StartsAt,
		Status:               apiopenapi.UserSubscriptionStatus(subscription.Status),
		UpdatedAt:            subscription.UpdatedAt,
		UserId:               apiopenapi.Id(strconv.Itoa(subscription.UserID)),
	}
}

func toAPIPricingRule(rule subscriptioncontract.PricingRule) apiopenapi.PricingRule {
	return apiopenapi.PricingRule{
		CacheReadPricePerMillionTokens:  rule.CacheReadPricePerMillionTokens,
		CacheWritePricePerMillionTokens: rule.CacheWritePricePerMillionTokens,
		CreatedAt:                       rule.CreatedAt,
		Currency:                        rule.Currency,
		EffectiveFrom:                   cloneTimePtr(rule.EffectiveFrom),
		EffectiveTo:                     cloneTimePtr(rule.EffectiveTo),
		Id:                              apiopenapi.Id(strconv.Itoa(rule.ID)),
		InputPricePerMillionTokens:      rule.InputPricePerMillionTokens,
		ModelId:                         apiopenapi.Id(strconv.Itoa(rule.ModelID)),
		OutputPricePerMillionTokens:     rule.OutputPricePerMillionTokens,
		ProviderId:                      apiopenapi.Id(strconv.Itoa(rule.ProviderID)),
		UpdatedAt:                       rule.UpdatedAt,
	}
}

func toAPIDomainEventOutbox(event eventscontract.OutboxEvent) apiopenapi.DomainEventOutbox {
	return apiopenapi.DomainEventOutbox{
		AggregateId:    event.AggregateID,
		AggregateType:  event.AggregateType,
		AttemptCount:   event.AttemptCount,
		CausationId:    event.CausationID,
		CorrelationId:  event.CorrelationID,
		CreatedAt:      event.CreatedAt,
		EventId:        event.EventID,
		EventType:      event.EventType,
		EventVersion:   event.EventVersion,
		Id:             apiopenapi.Id(strconv.Itoa(event.ID)),
		IdempotencyKey: event.IdempotencyKey,
		LastError:      event.LastError,
		Metadata:       jsonObject(event.Metadata),
		NextRetryAt:    event.NextRetryAt,
		Payload:        jsonObject(event.Payload),
		ProducerModule: event.ProducerModule,
		PublishedAt:    event.PublishedAt,
		Status:         apiopenapi.DomainEventOutboxStatus(event.Status),
	}
}

func toAPIRealtimeActiveSlot(slot realtimecontract.Slot) apiopenapi.RealtimeActiveSlot {
	return apiopenapi.RealtimeActiveSlot{
		AcquiredAt:             slot.AcquiredAt,
		ApiKeyId:               apiopenapi.Id(strconv.Itoa(slot.APIKeyID)),
		Id:                     slot.ID,
		Kind:                   apiopenapi.RealtimeSlotKind(slot.Kind),
		RequestId:              apiopenapi.RequestId(slot.RequestID),
		SessionAffinityKeyHash: slot.SessionAffinityKeyHash,
		SessionAffinitySource:  slot.SessionAffinitySource,
		SourceEndpoint:         apiopenapi.RealtimeActiveSlotSourceEndpoint(slot.SourceEndpoint),
		StickyAccountId:        optionalAPIID(slot.StickyAccountID),
		StickyStrength:         slot.StickyStrength,
		UserId:                 apiopenapi.Id(strconv.Itoa(slot.UserID)),
	}
}

func toAPIRealtimeActiveSlotCounters(list realtimecontract.ActiveSlotList) apiopenapi.RealtimeActiveSlotCounters {
	return apiopenapi.RealtimeActiveSlotCounters{
		AcquiredTotal:    list.Snapshot.AcquiredTotal,
		ActiveByApiKeyId: intKeyedCounts(list.ActiveByAPIKeyID),
		ActiveByEndpoint: copyStringCounts(list.Snapshot.ActiveByEndpoint),
		ActiveByKind:     slotKindCounts(list.ActiveByKind),
		ActiveSlots:      list.Snapshot.ActiveSlots,
		RejectedTotal:    list.Snapshot.RejectedTotal,
		ReleasedTotal:    list.Snapshot.ReleasedTotal,
	}
}

func toAPIOpsSLO(item operationscontract.SLOWithEvaluation) apiopenapi.OpsSLO {
	return apiopenapi.OpsSLO{
		Definition: toAPIOpsSLODefinition(item.Definition),
		Evaluation: apiopenapi.OpsSLOEvaluation{
			BadRequests:         item.Evaluation.BadRequests,
			BurnRate:            float32(item.Evaluation.BurnRate),
			ErrorBudgetConsumed: float32(item.Evaluation.ErrorBudgetConsumed),
			ErrorRate:           float32(item.Evaluation.ErrorRate),
			GoodRequests:        item.Evaluation.GoodRequests,
			Objective:           float32(item.Evaluation.Objective),
			TotalRequests:       item.Evaluation.TotalRequests,
			WindowEnd:           item.Evaluation.WindowEnd,
			WindowStart:         item.Evaluation.WindowStart,
		},
	}
}

func toAPIOpsSLODefinition(item operationscontract.SLODefinition) apiopenapi.OpsSLODefinition {
	return apiopenapi.OpsSLODefinition{
		AlertPolicy: toAPIOpsAlertPolicy(item.AlertPolicy),
		CreatedAt:   item.CreatedAt,
		Filter:      toAPIOpsSLOFilter(item.Filter),
		Id:          apiopenapi.Id(strconv.Itoa(item.ID)),
		Name:        item.Name,
		Objective:   float32(item.Objective),
		SliType:     apiopenapi.OpsSLIType(item.SLIType),
		Status:      apiopenapi.OpsSLOStatus(item.Status),
		UpdatedAt:   item.UpdatedAt,
		WindowDays:  item.WindowDays,
	}
}

func toAPIOpsSLOFilter(filter operationscontract.SLOFilter) apiopenapi.OpsSLOFilter {
	out := apiopenapi.OpsSLOFilter{
		ErrorOwnerExclude: make([]apiopenapi.OpsSLOFilterErrorOwnerExclude, 0, len(filter.ErrorOwnerExclude)),
		Model:             filter.Model,
		ProviderId:        optionalIDString(filter.ProviderID),
		SourceEndpoint:    filter.SourceEndpoint,
	}
	for _, value := range filter.ErrorOwnerExclude {
		out.ErrorOwnerExclude = append(out.ErrorOwnerExclude, apiopenapi.OpsSLOFilterErrorOwnerExclude(value))
	}
	return out
}

func toAPIOpsAlertPolicy(policy operationscontract.AlertPolicy) apiopenapi.OpsAlertPolicy {
	out := apiopenapi.OpsAlertPolicy{
		Name:       policy.Name,
		Thresholds: make([]apiopenapi.OpsBurnRateThreshold, 0, len(policy.Thresholds)),
	}
	for _, threshold := range policy.Thresholds {
		out.Thresholds = append(out.Thresholds, apiopenapi.OpsBurnRateThreshold{
			BurnRate:           float32(threshold.BurnRate),
			LongWindowSeconds:  int(threshold.LongWindow / time.Second),
			MinRequestCount:    threshold.MinRequestCount,
			Severity:           apiopenapi.OpsAlertSeverity(threshold.Severity),
			ShortWindowSeconds: int(threshold.ShortWindow / time.Second),
		})
	}
	return out
}

func toAPIOpsAlert(alert operationscontract.AlertEvent) apiopenapi.OpsAlertEvent {
	return apiopenapi.OpsAlertEvent{
		AcknowledgedAt: alert.AcknowledgedAt,
		AcknowledgedBy: optionalIDString(alert.AcknowledgedBy),
		CreatedAt:      alert.CreatedAt,
		Details:        jsonObject(alert.Details),
		Fingerprint:    alert.Fingerprint,
		Id:             apiopenapi.Id(strconv.Itoa(alert.ID)),
		ResolvedAt:     alert.ResolvedAt,
		RuleId:         alert.RuleID,
		Severity:       apiopenapi.OpsAlertSeverity(alert.Severity),
		SloId:          optionalIDString(alert.SLOID),
		StartedAt:      alert.StartedAt,
		Status:         apiopenapi.OpsAlertStatus(alert.Status),
		Summary:        alert.Summary,
		SuppressedBy:   alert.SuppressedBy,
		UpdatedAt:      alert.UpdatedAt,
	}
}

func toAPISchedulerDecision(decision schedulercontract.Decision) apiopenapi.SchedulerDecision {
	return apiopenapi.SchedulerDecision{
		ApiKeyId:               apiopenapi.Id(strconv.Itoa(decision.APIKeyID)),
		AttemptNo:              decision.AttemptNo,
		CacheAffinityHit:       decision.CacheAffinityHit,
		CandidateCount:         decision.CandidateCount,
		CompatibilityWarnings:  nonNilStrings(decision.CompatibilityWarnings),
		CreatedAt:              decision.CreatedAt,
		Currency:               decision.Currency,
		EstimatedCost:          decision.EstimatedCost,
		FallbackFromDecisionId: optionalIDString(decision.FallbackFromDecisionID),
		Id:                     apiopenapi.Id(strconv.Itoa(decision.ID)),
		Model:                  decision.Model,
		RejectReasons:          jsonObject(decision.RejectReasons),
		RejectedCount:          decision.RejectedCount,
		RequestId:              decision.RequestID,
		Scores:                 jsonObject(decision.Scores),
		SelectedAccountId:      optionalIDString(decision.SelectedAccountID),
		SelectedProviderId:     optionalIDString(decision.SelectedProviderID),
		SelectionRationale:     decision.SelectionRationale,
		SourceEndpoint:         decision.SourceEndpoint,
		SourceProtocol:         decision.SourceProtocol,
		StickyHit:              decision.StickyHit,
		Strategy:               apiopenapi.SchedulerDecisionStrategy(decision.Strategy),
		StrategyConfigHash:     decision.StrategyConfigHash,
		StrategyVersion:        decision.StrategyVersion,
		StrategyWeights:        jsonObject(decision.StrategyWeights),
		TargetProtocol:         decision.TargetProtocol,
		UserId:                 apiopenapi.Id(strconv.Itoa(decision.UserID)),
	}
}

func toSchedulerSimulationRequest(body apiopenapi.SchedulerSimulationRequest) (schedulercontract.StrategySimulationRequest, error) {
	request, err := toSchedulerSimulationScheduleRequest(body.Request)
	if err != nil {
		return schedulercontract.StrategySimulationRequest{}, err
	}
	var current schedulercontract.StrategyName
	if body.CurrentStrategy != nil {
		current = schedulercontract.StrategyName(*body.CurrentStrategy)
	}
	return schedulercontract.StrategySimulationRequest{
		Request:              request,
		CurrentStrategy:      current,
		ShadowStrategy:       schedulercontract.StrategyName(body.ShadowStrategy),
		ShadowRolloutPercent: float64PtrFromFloat32(body.ShadowRolloutPercent),
		RolloutKey:           optionalStringValue(body.RolloutKey),
	}, nil
}

func toSchedulerSimulationScheduleRequest(profile apiopenapi.SchedulerSimulationProfile) (schedulercontract.ScheduleRequest, error) {
	userID, err := apiIDToInt(profile.UserId)
	if err != nil {
		return schedulercontract.ScheduleRequest{}, err
	}
	apiKeyID, err := apiIDToInt(profile.ApiKeyId)
	if err != nil {
		return schedulercontract.ScheduleRequest{}, err
	}
	pricingRuleID, err := optionalAPIIDToInt(profile.PricingRuleId)
	if err != nil {
		return schedulercontract.ScheduleRequest{}, err
	}
	stickyAccountID, err := optionalAPIIDToInt(profile.StickyAccountId)
	if err != nil {
		return schedulercontract.ScheduleRequest{}, err
	}
	excludedAccountIDs, err := idsToInts(profile.ExcludedAccountIds)
	if err != nil {
		return schedulercontract.ScheduleRequest{}, err
	}
	candidates := make([]schedulercontract.Candidate, 0, len(profile.Candidates))
	for _, candidate := range profile.Candidates {
		converted, err := toSchedulerSimulationCandidate(candidate, profile.Model)
		if err != nil {
			return schedulercontract.ScheduleRequest{}, err
		}
		candidates = append(candidates, converted)
	}
	return schedulercontract.ScheduleRequest{
		RequestID:               strings.TrimSpace(string(profile.RequestId)),
		AttemptNo:               intValue(profile.AttemptNo),
		UserID:                  userID,
		APIKeyID:                apiKeyID,
		SourceProtocol:          optionalStringValue(profile.SourceProtocol),
		SourceEndpoint:          profile.SourceEndpoint,
		TargetProtocol:          optionalStringValue(profile.TargetProtocol),
		Model:                   profile.Model,
		ModelAlias:              optionalStringValue(profile.ModelAlias),
		FallbackModels:          derefStrings(profile.FallbackModels),
		SessionAffinityKey:      optionalStringValue(profile.SessionAffinityKey),
		SessionAffinitySource:   optionalStringValue(profile.SessionAffinitySource),
		UserTier:                schedulerSimulationUserTier(profile.UserTier),
		UserBalanceInsufficient: boolPtrValue(profile.UserBalanceInsufficient),
		EstimatedInputTokens:    intValue(profile.EstimatedInputTokens),
		EstimatedOutputTokens:   intValue(profile.EstimatedOutputTokens),
		EstimatedCost:           optionalStringValue(profile.EstimatedCost),
		Currency:                optionalStringValue(profile.Currency),
		PricingRuleID:           pricingRuleID,
		PricingSource:           optionalStringValue(profile.PricingSource),
		PricingEstimated:        boolPtrValue(profile.PricingEstimated),
		IsStream:                boolPtrValue(profile.IsStream),
		StickyAccountID:         stickyAccountID,
		StickyStrength:          schedulerSimulationStickyStrength(profile.StickyStrength),
		Warnings:                derefStrings(profile.Warnings),
		RequestCapabilities:     toCapabilityDescriptors(profile.RequestCapabilities),
		Candidates:              candidates,
		ExcludedAccountIDs:      excludedAccountIDs,
	}, nil
}

func toSchedulerSimulationCandidate(input apiopenapi.SchedulerSimulationCandidate, defaultModel string) (schedulercontract.Candidate, error) {
	accountID, err := apiIDToInt(input.AccountId)
	if err != nil {
		return schedulercontract.Candidate{}, err
	}
	providerID, err := apiIDToInt(input.ProviderId)
	if err != nil {
		return schedulercontract.Candidate{}, err
	}
	mappingID, err := optionalAPIIDToIntValue(input.MappingId)
	if err != nil {
		return schedulercontract.Candidate{}, err
	}
	modelID, err := optionalAPIIDToIntValue(input.ModelId)
	if err != nil {
		return schedulercontract.Candidate{}, err
	}
	return schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{
			ID:                   accountID,
			ProviderID:           providerID,
			RuntimeClass:         schedulerSimulationRuntimeClass(input.AccountRuntimeClass),
			CredentialCiphertext: schedulerSimulationCredential(input.AccountHasCredential),
			Status:               schedulerSimulationAccountStatus(input.AccountStatus),
			Weight:               schedulerSimulationWeight(input.AccountWeight),
			RiskLevel:            input.AccountRiskLevel,
			Metadata:             jsonObjectToMap(input.AccountMetadata),
		},
		Provider: providercontract.Provider{
			ID:           providerID,
			Protocol:     schedulerSimulationProviderProtocol(input.ProviderProtocol),
			Status:       schedulerSimulationProviderStatus(input.ProviderStatus),
			Capabilities: jsonObjectToMap(input.ProviderCapabilities),
			ConfigSchema: jsonObjectToMap(input.ProviderConfig),
		},
		Mapping: modelcontract.ModelProviderMapping{
			ID:                mappingID,
			ModelID:           modelID,
			ProviderID:        providerID,
			UpstreamModelName: schedulerSimulationUpstreamModel(input.UpstreamModelName, defaultModel),
			Status:            schedulerSimulationMappingStatus(input.MappingStatus),
			PricingOverride:   jsonObjectToMap(input.PricingOverride),
		},
		EffectiveCapabilities: toCapabilityDescriptors(input.EffectiveCapabilities),
		RuntimeState:          toSchedulerSimulationRuntimeState(input.RuntimeState),
		Limits:                toSchedulerSimulationRuntimeLimits(input.Limits),
	}, nil
}

func toSchedulerSimulationRuntimeState(input *apiopenapi.SchedulerSimulationRuntimeState) schedulercontract.RuntimeState {
	if input == nil {
		return schedulercontract.RuntimeState{}
	}
	return schedulercontract.RuntimeState{
		QuotaExhausted:      boolPtrValue(input.QuotaExhausted),
		HealthScore:         float64PtrFromFloat32(input.HealthScore),
		QuotaRemainingRatio: float64PtrFromFloat32(input.QuotaRemainingRatio),
		LatencyP95MS:        cloneIntPtr(input.LatencyP95Ms),
		CircuitOpen:         boolPtrValue(input.CircuitOpen),
		CooldownActive:      boolPtrValue(input.CooldownActive),
		CurrentConcurrency:  intValue(input.CurrentConcurrency),
		RPMUsed:             intValue(input.RpmUsed),
		TPMUsed:             intValue(input.TpmUsed),
	}
}

func toSchedulerSimulationRuntimeLimits(input *apiopenapi.SchedulerSimulationLimits) schedulercontract.RuntimeLimits {
	if input == nil {
		return schedulercontract.RuntimeLimits{}
	}
	return schedulercontract.RuntimeLimits{
		MaxConcurrency: cloneIntPtr(input.MaxConcurrency),
		RPMLimit:       cloneIntPtr(input.RpmLimit),
		TPMLimit:       cloneIntPtr(input.TpmLimit),
	}
}

func toAPISchedulerSimulationResult(result schedulercontract.StrategySimulationResult) apiopenapi.SchedulerSimulationResult {
	return apiopenapi.SchedulerSimulationResult{
		Current: toAPISchedulerSimulationDecision(result.Current),
		Shadow:  toAPISchedulerSimulationDecision(result.Shadow),
		Diff:    toAPISchedulerSimulationDiff(result.Diff),
		Rollout: toAPISchedulerSimulationRollout(result.Rollout),
		DryRun:  result.DryRun,
	}
}

func toSchedulerReplayRequest(body apiopenapi.SchedulerReplayRequest) schedulercontract.StrategyReplayRequest {
	var current schedulercontract.StrategyName
	if body.CurrentStrategy != nil {
		current = schedulercontract.StrategyName(*body.CurrentStrategy)
	}
	limit := 0
	if body.Limit != nil {
		limit = *body.Limit
	}
	return schedulercontract.StrategyReplayRequest{
		CurrentStrategy:      current,
		ShadowStrategy:       schedulercontract.StrategyName(body.ShadowStrategy),
		ShadowRolloutPercent: float64PtrFromFloat32(body.ShadowRolloutPercent),
		Limit:                limit,
		Since:                cloneTimePtr(body.Since),
		Until:                cloneTimePtr(body.Until),
		Model:                optionalStringValue(body.Model),
		RequestID:            optionalStringValue(body.RequestId),
	}
}

func toAPISchedulerReplayResult(result schedulercontract.StrategyReplayResult) apiopenapi.SchedulerReplayResult {
	items := make([]apiopenapi.SchedulerReplayItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAPISchedulerReplayItem(item))
	}
	return apiopenapi.SchedulerReplayResult{
		AverageCostScoreDelta:    float32(result.AverageCostScoreDelta),
		AverageFinalScoreDelta:   float32(result.AverageFinalScoreDelta),
		AverageLatencyScoreDelta: float32(result.AverageLatencyScoreDelta),
		AverageQualityScoreDelta: float32(result.AverageQualityScoreDelta),
		AverageRiskPenaltyDelta:  float32(result.AverageRiskPenaltyDelta),
		CurrentWinCounts:         jsonObject(intCountsToAny(result.CurrentWinCounts)),
		DryRun:                   result.DryRun,
		Items:                    items,
		Replayed:                 result.Replayed,
		Requested:                result.Requested,
		ShadowWinCounts:          jsonObject(intCountsToAny(result.ShadowWinCounts)),
		Skipped:                  result.Skipped,
		WinnerChanged:            result.WinnerChanged,
	}
}

func toAPISchedulerReplayItem(item schedulercontract.StrategyReplayItem) apiopenapi.SchedulerReplayItem {
	return apiopenapi.SchedulerReplayItem{
		AttemptNo:                 item.AttemptNo,
		CreatedAt:                 item.CreatedAt,
		Current:                   toAPISchedulerSimulationDecision(item.Current),
		DecisionId:                apiopenapi.Id(strconv.Itoa(item.DecisionID)),
		Diff:                      toAPISchedulerSimulationDiff(item.Diff),
		OriginalSelectedAccountId: optionalIDString(item.OriginalSelectedAccountID),
		OriginalStrategy:          apiopenapi.SchedulerStrategyName(item.OriginalStrategy),
		RequestId:                 item.RequestID,
		Rollout:                   toAPISchedulerSimulationRollout(item.Rollout),
		Shadow:                    toAPISchedulerSimulationDecision(item.Shadow),
		SnapshotId:                apiopenapi.Id(strconv.Itoa(item.SnapshotID)),
	}
}

func toAPISchedulerSimulationDiff(diff schedulercontract.StrategySimulationDiff) apiopenapi.SchedulerSimulationDiff {
	return apiopenapi.SchedulerSimulationDiff{
		WinnerChanged:             diff.WinnerChanged,
		CurrentSelectedAccountId:  optionalIDString(diff.CurrentSelectedAccountID),
		ShadowSelectedAccountId:   optionalIDString(diff.ShadowSelectedAccountID),
		CurrentSelectedProviderId: optionalIDString(diff.CurrentSelectedProviderID),
		ShadowSelectedProviderId:  optionalIDString(diff.ShadowSelectedProviderID),
		FinalScoreDelta:           float32(diff.FinalScoreDelta),
		CostScoreDelta:            float32(diff.CostScoreDelta),
		LatencyScoreDelta:         float32(diff.LatencyScoreDelta),
		QualityScoreDelta:         float32(diff.QualityScoreDelta),
		RiskPenaltyDelta:          float32(diff.RiskPenaltyDelta),
	}
}

func toAPISchedulerSimulationRollout(rollout schedulercontract.StrategySimulationRollout) apiopenapi.SchedulerSimulationRollout {
	return apiopenapi.SchedulerSimulationRollout{
		Bucket:         float32(rollout.Bucket),
		Enabled:        rollout.Enabled,
		KeyHash:        rollout.KeyHash,
		Percent:        float32(rollout.Percent),
		ShadowSelected: rollout.ShadowSelected,
	}
}

func intCountsToAny(values map[string]int) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func toAPISchedulerSimulationDecision(result schedulercontract.SimulatedStrategyDecision) apiopenapi.SchedulerSimulationDecision {
	decision := result.Decision
	return apiopenapi.SchedulerSimulationDecision{
		ApiKeyId:              apiopenapi.Id(strconv.Itoa(decision.APIKeyID)),
		AttemptNo:             decision.AttemptNo,
		CacheAffinityHit:      decision.CacheAffinityHit,
		CandidateCount:        decision.CandidateCount,
		CompatibilityWarnings: nonNilStrings(decision.CompatibilityWarnings),
		CreatedAt:             decision.CreatedAt,
		Currency:              decision.Currency,
		Error:                 result.Error,
		EstimatedCost:         decision.EstimatedCost,
		Model:                 decision.Model,
		RejectReasons:         jsonObject(decision.RejectReasons),
		RejectedCount:         decision.RejectedCount,
		RequestId:             decision.RequestID,
		Scores:                jsonObject(decision.Scores),
		SelectedAccountId:     optionalIDString(decision.SelectedAccountID),
		SelectedProviderId:    optionalIDString(decision.SelectedProviderID),
		SourceEndpoint:        decision.SourceEndpoint,
		SourceProtocol:        decision.SourceProtocol,
		StickyHit:             decision.StickyHit,
		Strategy:              apiopenapi.SchedulerStrategyName(decision.Strategy),
		StrategyConfigHash:    decision.StrategyConfigHash,
		StrategyVersion:       decision.StrategyVersion,
		StrategyWeights:       jsonObject(decision.StrategyWeights),
		TargetProtocol:        decision.TargetProtocol,
		UserId:                apiopenapi.Id(strconv.Itoa(decision.UserID)),
	}
}

func apiIDToInt(value apiopenapi.Id) (int, error) {
	parsed, err := strconv.Atoi(string(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid id %q", value)
	}
	return parsed, nil
}

func optionalAPIIDToInt(value *apiopenapi.Id) (*int, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := apiIDToInt(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalAPIIDToIntValue(value *apiopenapi.Id) (int, error) {
	parsed, err := optionalAPIIDToInt(value)
	if err != nil || parsed == nil {
		return 0, err
	}
	return *parsed, nil
}

func schedulerSimulationUserTier(value *apiopenapi.SchedulerSimulationProfileUserTier) schedulercontract.UserTier {
	if value == nil {
		return schedulercontract.UserTierFree
	}
	return schedulercontract.UserTier(*value)
}

func schedulerSimulationStickyStrength(value *apiopenapi.SchedulerSimulationStickyStrength) schedulercontract.StickyStrength {
	if value == nil || *value == apiopenapi.SchedulerSimulationStickyStrengthNone {
		return schedulercontract.StickyStrengthNone
	}
	return schedulercontract.StickyStrength(*value)
}

func schedulerSimulationRuntimeClass(value *apiopenapi.RuntimeClass) accountcontract.RuntimeClass {
	if value == nil {
		return accountcontract.RuntimeClassAPIKey
	}
	return accountcontract.RuntimeClass(*value)
}

func schedulerSimulationCredential(hasCredential *bool) string {
	if hasCredential != nil && !*hasCredential {
		return ""
	}
	return "simulation-credential"
}

func schedulerSimulationAccountStatus(value *apiopenapi.ProviderAccountStatus) accountcontract.Status {
	if value == nil {
		return accountcontract.StatusActive
	}
	return accountcontract.Status(*value)
}

func schedulerSimulationWeight(value *float32) float32 {
	if value == nil {
		return 1
	}
	return *value
}

func schedulerSimulationProviderStatus(value *apiopenapi.ResourceStatus) providercontract.Status {
	if value == nil {
		return providercontract.StatusActive
	}
	return providercontract.Status(*value)
}

func schedulerSimulationProviderProtocol(value *apiopenapi.ProviderProtocol) string {
	if value == nil {
		return "openai-compatible"
	}
	return string(*value)
}

func schedulerSimulationMappingStatus(value *apiopenapi.ResourceStatus) modelcontract.Status {
	if value == nil {
		return modelcontract.StatusActive
	}
	return modelcontract.Status(*value)
}

func schedulerSimulationUpstreamModel(value *string, fallback string) string {
	if model := optionalStringValue(value); model != "" {
		return model
	}
	return fallback
}

func boolPtrValue(value *bool) bool {
	return value != nil && *value
}

func float64PtrFromFloat32(value *float32) *float64 {
	if value == nil {
		return nil
	}
	out := float64(*value)
	return &out
}

func toAPICapabilityDefinition(def capabilitiescontract.Definition) apiopenapi.CapabilityDefinition {
	return apiopenapi.CapabilityDefinition{
		Category:       def.Category,
		Description:    def.Description,
		Key:            def.Key,
		ReplacementKey: def.ReplacementKey,
		Schema:         mapToJsonObjectPtr(def.Schema),
		Status:         apiopenapi.CapabilityDefinitionStatus(def.Status),
		Version:        def.Version,
	}
}

func toAPICapabilityDescriptors(values []capabilitiescontract.Descriptor) []apiopenapi.CapabilityDescriptor {
	out := make([]apiopenapi.CapabilityDescriptor, 0, len(values))
	for _, value := range values {
		out = append(out, apiopenapi.CapabilityDescriptor{
			Key:      value.Key,
			Level:    apiopenapi.CapabilityDescriptorLevel(value.Level),
			Metadata: mapToJsonObjectPtr(value.Metadata),
			Status:   apiopenapi.CapabilityDescriptorStatus(value.Status),
			Version:  value.Version,
		})
	}
	return out
}

func toAPICapabilityDescriptorsPtr(values []capabilitiescontract.Descriptor) *[]apiopenapi.CapabilityDescriptor {
	if values == nil {
		return nil
	}
	out := toAPICapabilityDescriptors(values)
	return &out
}

func toCapabilityDescriptors(values *[]apiopenapi.CapabilityDescriptor) []capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(*values))
	for _, value := range *values {
		out = append(out, capabilitiescontract.Descriptor{
			Key:      value.Key,
			Level:    capabilitiescontract.DescriptorLevel(value.Level),
			Metadata: jsonObjectToMap(value.Metadata),
			Status:   capabilitiescontract.DescriptorStatus(value.Status),
			Version:  value.Version,
		})
	}
	return out
}

func toCapabilityDescriptorsPtrContract(values *[]apiopenapi.CapabilityDescriptor) *[]capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	out := toCapabilityDescriptors(values)
	return &out
}

func jsonObjectToMap(value *apiopenapi.JsonObject) map[string]any {
	if value == nil {
		return nil
	}
	return map[string]any(*value)
}

func jsonObjectValueToMap(value apiopenapi.JsonObject) map[string]any {
	if value == nil {
		return nil
	}
	return map[string]any(value)
}

func jsonObjectToMapPtr(value *apiopenapi.JsonObject) *map[string]any {
	if value == nil {
		return nil
	}
	out := jsonObjectToMap(value)
	return &out
}

func mapToJsonObjectPtr(value map[string]any) *apiopenapi.JsonObject {
	if value == nil {
		return nil
	}
	object := apiopenapi.JsonObject(value)
	return &object
}

func jsonObject(value map[string]any) apiopenapi.JsonObject {
	if value == nil {
		return apiopenapi.JsonObject{}
	}
	return apiopenapi.JsonObject(value)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func optionalIDString(value *int) *string {
	if value == nil {
		return nil
	}
	out := strconv.Itoa(*value)
	return &out
}

func optionalAPIID(value *int) *apiopenapi.Id {
	if value == nil {
		return nil
	}
	out := apiopenapi.Id(strconv.Itoa(*value))
	return &out
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func optionalNullableString(value *string) **string {
	if value == nil {
		return nil
	}
	return &value
}

func optionalNullableInt(value *int) **int {
	if value == nil {
		return nil
	}
	return &value
}

func addOptionalInt(target map[string]any, key string, value *int) {
	if value != nil {
		target[key] = *value
	}
}

func derefMap(value *map[string]interface{}) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(*value))
	for key, val := range *value {
		out[key] = val
	}
	return out
}

func optionalMap(value *map[string]interface{}) *map[string]any {
	if value == nil {
		return nil
	}
	out := derefMap(value)
	return &out
}

func optionalCredential(value *map[string]interface{}) *map[string]any {
	if value == nil {
		return nil
	}
	out := derefMap(value)
	return &out
}

func toProviderStatusPtr(value *apiopenapi.ResourceStatus) *providercontract.Status {
	if value == nil {
		return nil
	}
	status := providercontract.Status(*value)
	return &status
}

func toModelStatusPtr(value *apiopenapi.ResourceStatus) *modelcontract.Status {
	if value == nil {
		return nil
	}
	status := modelcontract.Status(*value)
	return &status
}

func toAccountStatusPtr(value *apiopenapi.ProviderAccountStatus) *accountcontract.Status {
	if value == nil {
		return nil
	}
	status := accountcontract.Status(*value)
	return &status
}

func toProxyStatusPtr(value *apiopenapi.ProxyDefinitionStatus) *accountcontract.ProxyStatus {
	if value == nil {
		return nil
	}
	status := accountcontract.ProxyStatus(*value)
	return &status
}

func toProxyTypePtr(value *apiopenapi.ProxyDefinitionType) *accountcontract.ProxyType {
	if value == nil {
		return nil
	}
	proxyType := accountcontract.ProxyType(*value)
	return &proxyType
}

func toUserStatusPtr(value *apiopenapi.UserStatus) *userscontract.Status {
	if value == nil {
		return nil
	}
	status := userscontract.Status(*value)
	return &status
}

func toAccountGroupStatusPtr(value *apiopenapi.AccountGroupStatus) *accountcontract.GroupStatus {
	if value == nil {
		return nil
	}
	status := accountcontract.GroupStatus(*value)
	return &status
}

func toAPIKeyStatusPtr(value *apiopenapi.ApiKeyStatus) *apikeycontract.Status {
	if value == nil {
		return nil
	}
	status := apikeycontract.Status(*value)
	return &status
}

func toSubscriptionPlanStatusPtr(value *apiopenapi.SubscriptionPlanStatus) *subscriptioncontract.PlanStatus {
	if value == nil {
		return nil
	}
	status := subscriptioncontract.PlanStatus(*value)
	return &status
}

func toUserSubscriptionStatusPtr(value *apiopenapi.UserSubscriptionStatus) *subscriptioncontract.SubscriptionStatus {
	if value == nil {
		return nil
	}
	status := subscriptioncontract.SubscriptionStatus(*value)
	return &status
}

func toPaymentProviderStatusPtr(value *apiopenapi.PaymentProviderStatus) *paymentcontract.ProviderStatus {
	if value == nil {
		return nil
	}
	status := paymentcontract.ProviderStatus(*value)
	return &status
}

func toCreateSLORequest(body apiopenapi.CreateOpsSLORequest) (operationscontract.CreateSLORequest, error) {
	filter, err := toSLOFilterPtr(body.Filter)
	if err != nil {
		return operationscontract.CreateSLORequest{}, err
	}
	policy := operationscontract.AlertPolicy{}
	if body.AlertPolicy != nil {
		policy = toAlertPolicy(*body.AlertPolicy)
	}
	return operationscontract.CreateSLORequest{
		Name:        body.Name,
		SLIType:     toSLITypeValue(body.SliType),
		Objective:   float64(body.Objective),
		WindowDays:  intValue(body.WindowDays),
		Status:      toSLOStatusPtr(body.Status),
		Filter:      filter,
		AlertPolicy: policy,
	}, nil
}

func toUpdateSLORequest(body apiopenapi.UpdateOpsSLORequest) (operationscontract.UpdateSLORequest, error) {
	var filter *operationscontract.SLOFilter
	if body.Filter != nil {
		converted, err := toSLOFilterPtr(body.Filter)
		if err != nil {
			return operationscontract.UpdateSLORequest{}, err
		}
		filter = &converted
	}
	var policy *operationscontract.AlertPolicy
	if body.AlertPolicy != nil {
		converted := toAlertPolicy(*body.AlertPolicy)
		policy = &converted
	}
	var objective *float64
	if body.Objective != nil {
		converted := float64(*body.Objective)
		objective = &converted
	}
	return operationscontract.UpdateSLORequest{
		Name:        body.Name,
		Objective:   objective,
		WindowDays:  body.WindowDays,
		Status:      toSLOStatusPtr(body.Status),
		Filter:      filter,
		AlertPolicy: policy,
	}, nil
}

func toSLOFilterPtr(value *apiopenapi.OpsSLOFilter) (operationscontract.SLOFilter, error) {
	if value == nil {
		return operationscontract.SLOFilter{}, nil
	}
	filter := operationscontract.SLOFilter{
		SourceEndpoint: value.SourceEndpoint,
		Model:          value.Model,
	}
	if value.ProviderId != nil {
		providerID, err := strconv.Atoi(string(*value.ProviderId))
		if err != nil || providerID <= 0 {
			return operationscontract.SLOFilter{}, operationsservice.ErrInvalidInput
		}
		filter.ProviderID = &providerID
	}
	filter.ErrorOwnerExclude = make([]string, 0, len(value.ErrorOwnerExclude))
	for _, owner := range value.ErrorOwnerExclude {
		filter.ErrorOwnerExclude = append(filter.ErrorOwnerExclude, string(owner))
	}
	return filter, nil
}

func toAlertPolicy(value apiopenapi.OpsAlertPolicy) operationscontract.AlertPolicy {
	policy := operationscontract.AlertPolicy{
		Name:       value.Name,
		Thresholds: make([]operationscontract.BurnRateThreshold, 0, len(value.Thresholds)),
	}
	for _, threshold := range value.Thresholds {
		policy.Thresholds = append(policy.Thresholds, operationscontract.BurnRateThreshold{
			Severity:        operationscontract.AlertSeverity(threshold.Severity),
			ShortWindow:     time.Duration(threshold.ShortWindowSeconds) * time.Second,
			LongWindow:      time.Duration(threshold.LongWindowSeconds) * time.Second,
			BurnRate:        float64(threshold.BurnRate),
			MinRequestCount: threshold.MinRequestCount,
		})
	}
	return policy
}

func toSLITypeValue(value *apiopenapi.OpsSLIType) operationscontract.SLIType {
	if value == nil {
		return ""
	}
	return operationscontract.SLIType(*value)
}

func toSLOStatusPtr(value *apiopenapi.OpsSLOStatus) *operationscontract.SLOStatus {
	if value == nil {
		return nil
	}
	status := operationscontract.SLOStatus(*value)
	return &status
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func toAccountRuntimeClassPtr(value *apiopenapi.RuntimeClass) *accountcontract.RuntimeClass {
	if value == nil {
		return nil
	}
	runtimeClass := accountcontract.RuntimeClass(*value)
	return &runtimeClass
}

func providerAdapterTypeString(value *apiopenapi.ProviderAdapterType) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

func providerProtocolString(value *apiopenapi.ProviderProtocol) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

func ptrString(value string) *string { return &value }

func ptrInt(value int) *int { return &value }

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func ptrFloat32(value float32) *float32 { return &value }

func cloneFloat32Ptr(value *float32) *float32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func ptrProviderStatus(value providercontract.Status) *providercontract.Status { return &value }

func ptrModelStatus(value modelcontract.Status) *modelcontract.Status { return &value }

func ptrAccountStatus(value accountcontract.Status) *accountcontract.Status { return &value }

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func pagination(total int) apiopenapi.Pagination {
	return apiopenapi.Pagination{Page: 1, PageSize: total, Total: total, HasNext: false}
}

func copyStringCounts(values map[string]int) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func slotKindCounts(values map[realtimecontract.SlotKind]int) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[string(key)] = value
	}
	return out
}

func intKeyedCounts(values map[int]int) map[string]int {
	if values == nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[strconv.Itoa(key)] = value
	}
	return out
}
