package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

func (rt *runtimeState) recordAudit(ctx context.Context, req auditcontract.RecordRequest) {
	if _, err := rt.audit.Record(ctx, req); err != nil {
		rt.logger.Warn("failed to record audit log", "error", err, "action", req.Action, "resource_type", req.ResourceType, "resource_id", req.ResourceID)
	}
}

func auditRecordFromRequest(r *http.Request, actorUserID int, action, resourceType, resourceID string, before, after map[string]any) auditcontract.RecordRequest {
	return auditcontract.RecordRequest{
		ActorUserID:  ptrInt(actorUserID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
		IP:           clientIP(r),
		UserAgent:    r.UserAgent(),
		TraceID:      requestIDFromContext(r.Context()),
	}
}

func adminControlAuditSnapshot(value any) map[string]any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"value": value}
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return map[string]any{"value": value}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

func opsSLOAuditSnapshot(slo operationscontract.SLODefinition) map[string]any {
	return map[string]any{
		"name":         slo.Name,
		"sli_type":     slo.SLIType,
		"objective":    slo.Objective,
		"window_days":  slo.WindowDays,
		"status":       slo.Status,
		"filter":       opsSLOFilterAuditSnapshot(slo.Filter),
		"alert_policy": opsAlertPolicyAuditSnapshot(slo.AlertPolicy),
	}
}

func opsSLOFilterAuditSnapshot(filter operationscontract.SLOFilter) map[string]any {
	out := map[string]any{
		"source_endpoint":     filter.SourceEndpoint,
		"model":               filter.Model,
		"error_owner_exclude": nonNilStrings(filter.ErrorOwnerExclude),
	}
	if filter.ProviderID != nil {
		out["provider_id"] = *filter.ProviderID
	}
	return out
}

func opsAlertPolicyAuditSnapshot(policy operationscontract.AlertPolicy) map[string]any {
	thresholds := make([]map[string]any, 0, len(policy.Thresholds))
	for _, threshold := range policy.Thresholds {
		thresholds = append(thresholds, map[string]any{
			"severity":             threshold.Severity,
			"short_window_seconds": int(threshold.ShortWindow / time.Second),
			"long_window_seconds":  int(threshold.LongWindow / time.Second),
			"burn_rate":            threshold.BurnRate,
			"min_request_count":    threshold.MinRequestCount,
		})
	}
	return map[string]any{
		"name":       policy.Name,
		"thresholds": thresholds,
	}
}

func opsAlertAckAuditSnapshot(alert operationscontract.AlertEvent) map[string]any {
	out := map[string]any{
		"status":      alert.Status,
		"fingerprint": alert.Fingerprint,
		"severity":    alert.Severity,
		"rule_id":     alert.RuleID,
	}
	if alert.SLOID != nil {
		out["slo_id"] = *alert.SLOID
	}
	if alert.AcknowledgedBy != nil {
		out["acknowledged_by"] = *alert.AcknowledgedBy
	}
	if alert.AcknowledgedAt != nil {
		out["acknowledged_at"] = *alert.AcknowledgedAt
	}
	return out
}

func providerAuditSnapshot(provider providercontract.Provider) map[string]any {
	return map[string]any{
		"name":          provider.Name,
		"display_name":  provider.DisplayName,
		"adapter_type":  provider.AdapterType,
		"protocol":      provider.Protocol,
		"status":        provider.Status,
		"capabilities":  provider.Capabilities,
		"config_schema": provider.ConfigSchema,
	}
}

func modelAuditSnapshot(model modelcontract.Model) map[string]any {
	return map[string]any{
		"canonical_name":    model.CanonicalName,
		"display_name":      model.DisplayName,
		"family":            model.Family,
		"context_window":    model.ContextWindow,
		"max_output_tokens": model.MaxOutputTokens,
		"quality_tier":      model.QualityTier,
		"status":            model.Status,
		"capabilities":      model.Capabilities,
	}
}

func accountAuditSnapshot(account accountcontract.ProviderAccount) map[string]any {
	return map[string]any{
		"provider_id":        account.ProviderID,
		"name":               account.Name,
		"runtime_class":      account.RuntimeClass,
		"upstream_client":    account.UpstreamClient,
		"proxy_id":           account.ProxyID,
		"status":             account.Status,
		"priority":           account.Priority,
		"weight":             account.Weight,
		"risk_level":         account.RiskLevel,
		"metadata":           account.Metadata,
		"credential_version": account.CredentialVersion,
	}
}

func accountGroupAuditSnapshot(group accountcontract.AccountGroup) map[string]any {
	return map[string]any{
		"name":           group.Name,
		"description":    group.Description,
		"provider_scope": cloneAnyMap(group.ProviderScope),
		"model_scope":    cloneAnyMap(group.ModelScope),
		"strategy_hint":  group.StrategyHint,
		"status":         group.Status,
	}
}

func userAuditSnapshot(user userscontract.StoredUser) map[string]any {
	return map[string]any{
		"email":       user.Email,
		"name":        user.Name,
		"status":      user.Status,
		"roles":       append([]userscontract.Role(nil), user.Roles...),
		"balance":     user.Balance,
		"currency":    user.Currency,
		"rpm_limit":   user.RPMLimit,
		"last_login":  user.LastLoginAt,
		"created_at":  user.CreatedAt,
		"verified_at": user.EmailVerifiedAt,
	}
}

func apiKeyAuditSnapshot(key apikeycontract.APIKey) map[string]any {
	return map[string]any{
		"name":           key.Name,
		"prefix":         key.Prefix,
		"status":         key.Status,
		"scopes":         append([]string(nil), key.Scopes...),
		"allowed_models": append([]string(nil), key.AllowedModels...),
		"group_ids":      append([]int(nil), key.GroupIDs...),
	}
}

func subscriptionPlanAuditSnapshot(plan subscriptioncontract.SubscriptionPlan) map[string]any {
	return map[string]any{
		"name":          plan.Name,
		"description":   plan.Description,
		"price":         plan.Price,
		"currency":      plan.Currency,
		"validity_days": plan.ValidityDays,
		"entitlements":  cloneAnyMap(plan.Entitlements),
		"for_sale":      plan.ForSale,
		"sort_order":    plan.SortOrder,
		"status":        plan.Status,
	}
}

func userSubscriptionAuditSnapshot(subscription subscriptioncontract.UserSubscription) map[string]any {
	return map[string]any{
		"user_id":               subscription.UserID,
		"plan_id":               subscription.PlanID,
		"status":                subscription.Status,
		"starts_at":             subscription.StartsAt,
		"expires_at":            subscription.ExpiresAt,
		"entitlements_snapshot": cloneAnyMap(subscription.EntitlementsSnapshot),
		"source_type":           subscription.SourceType,
		"source_id":             subscription.SourceID,
	}
}

func pricingRuleAuditSnapshot(rule subscriptioncontract.PricingRule) map[string]any {
	return map[string]any{
		"model_id":                             rule.ModelID,
		"provider_id":                          rule.ProviderID,
		"input_price_per_million_tokens":       rule.InputPricePerMillionTokens,
		"output_price_per_million_tokens":      rule.OutputPricePerMillionTokens,
		"cache_read_price_per_million_tokens":  rule.CacheReadPricePerMillionTokens,
		"cache_write_price_per_million_tokens": rule.CacheWritePricePerMillionTokens,
		"currency":                             rule.Currency,
		"effective_from":                       rule.EffectiveFrom,
		"effective_to":                         rule.EffectiveTo,
	}
}

func cloneMetadata(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
