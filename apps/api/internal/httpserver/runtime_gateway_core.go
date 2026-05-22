package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type gatewayUsageRecord struct {
	RequestID             string
	Authed                apikeycontract.AuthResult
	DecisionID            int
	AttemptNo             int
	ProviderID            *int
	AccountID             *int
	SourceProtocol        string
	SourceEndpoint        string
	TargetProtocol        string
	Model                 string
	Success               bool
	ErrorClass            *string
	StatusCode            *int
	LatencyMS             int
	InputTokens           int
	OutputTokens          int
	CachedTokens          int
	UsageEstimated        bool
	Pricing               gatewayPricingEvidence
	CompatibilityWarnings []string
}

type gatewayAdmission struct {
	EstimatedUsage gatewaycontract.Usage
	Pricing        gatewayPricingEvidence
	Entitlement    subscriptioncontract.EntitlementDecision
}

type gatewayPricingEvidence struct {
	Amount           string
	Currency         string
	PricingRuleID    *int
	PricingSource    string
	PricingEstimated bool
}

func (e gatewayPricingEvidence) withDefaults() gatewayPricingEvidence {
	if strings.TrimSpace(e.Amount) == "" {
		e.Amount = "0.00000000"
	}
	if strings.TrimSpace(e.Currency) == "" {
		e.Currency = "USD"
	}
	if strings.TrimSpace(e.PricingSource) == "" {
		e.PricingSource = "default_zero"
	}
	return e
}

func (rt *runtimeState) scheduleGatewayRequest(ctx context.Context, req schedulercontract.ScheduleRequest, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey) (schedulercontract.ScheduleResult, error) {
	candidates, err := rt.gatewayCandidates(ctx, modelID, forcedProviderKey, apiKey)
	if err != nil {
		return schedulercontract.ScheduleResult{}, err
	}
	if len(req.AccountGroupScope) > 0 {
		candidates, err = rt.filterCandidatesByAccountGroupScope(ctx, candidates, req.AccountGroupScope)
		if err != nil {
			return schedulercontract.ScheduleResult{}, err
		}
	}
	if req.StickyAccountID == nil && strings.TrimSpace(req.SessionAffinityKey) != "" {
		req.StickyAccountID = stickyAccountIDFromCandidates(candidates, req.SessionAffinityKey)
	}
	req.Candidates = candidates
	return rt.scheduler.Schedule(ctx, req)
}

func (rt *runtimeState) prepareGatewayAdmission(ctx context.Context, canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution, modelID int) (gatewayAdmission, error) {
	estimatedUsage := estimateGatewayRequestUsage(canonical)
	pricing := rt.gatewayPricing(ctx, subscriptioncontract.PricingRequest{
		ModelID:      modelID,
		ProviderID:   0,
		InputTokens:  estimatedUsage.InputTokens,
		OutputTokens: estimatedUsage.OutputTokens,
		At:           time.Now().UTC(),
	}, true)
	tokensUsed, costUsed, err := rt.gatewayUserPeriodUsage(ctx, canonical.UserID, time.Now().UTC())
	if err != nil {
		return gatewayAdmission{}, err
	}
	entitlement, err := rt.subscriptions.CheckEntitlement(ctx, subscriptioncontract.EntitlementCheckRequest{
		UserID:             canonical.UserID,
		ModelReferences:    gatewayModelReferences(canonical, resolution),
		EstimatedTokens:    estimatedUsage.InputTokens + estimatedUsage.OutputTokens + estimatedUsage.CachedTokens,
		EstimatedCost:      pricing.Amount,
		TokensUsedInPeriod: tokensUsed,
		CostUsedInPeriod:   costUsed,
		RequestTime:        time.Now().UTC(),
	})
	if err != nil {
		return gatewayAdmission{}, err
	}
	return gatewayAdmission{EstimatedUsage: estimatedUsage, Pricing: pricing, Entitlement: entitlement}, nil
}

func (rt *runtimeState) applyGatewayAdmission(req *schedulercontract.ScheduleRequest, admission gatewayAdmission) {
	req.EstimatedInputTokens = admission.EstimatedUsage.InputTokens
	req.EstimatedOutputTokens = admission.EstimatedUsage.OutputTokens
	req.EstimatedCost = admission.Pricing.Amount
	req.Currency = admission.Pricing.Currency
	req.PricingRuleID = admission.Pricing.PricingRuleID
	req.PricingSource = admission.Pricing.PricingSource
	req.PricingEstimated = true
	req.AccountGroupScope = append([]int(nil), admission.Entitlement.AccountGroupScope...)
	if strategy := schedulerStrategyName(admission.Entitlement.SchedulerStrategy); strategy != "" {
		req.Strategy = strategy
	}
}

func (rt *runtimeState) filterCandidatesByAccountGroupScope(ctx context.Context, candidates []schedulercontract.Candidate, scope []int) ([]schedulercontract.Candidate, error) {
	if len(scope) == 0 {
		return candidates, nil
	}
	out := make([]schedulercontract.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		groupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, candidate.Account.ID)
		if err != nil {
			return nil, err
		}
		if intersectsInt(scope, groupIDs) {
			out = append(out, candidate)
		}
	}
	return out, nil
}

func (rt *runtimeState) gatewayPricing(ctx context.Context, req subscriptioncontract.PricingRequest, estimated bool) gatewayPricingEvidence {
	result, err := rt.subscriptions.EstimatePrice(ctx, req)
	if err != nil {
		rt.logger.Warn("failed to estimate gateway price", "error", err, "model_id", req.ModelID, "provider_id", req.ProviderID)
		return gatewayPricingEvidence{Amount: "0.00000000", Currency: "USD", PricingSource: "pricing_error", PricingEstimated: estimated}
	}
	source := "default_zero"
	if len(req.PricingOverride) > 0 {
		source = "mapping_override"
	} else if result.PricingRuleID != nil {
		source = "pricing_rule"
	}
	return gatewayPricingEvidence{
		Amount:           result.Amount,
		Currency:         result.Currency,
		PricingRuleID:    cloneIntPtr(result.PricingRuleID),
		PricingSource:    source,
		PricingEstimated: estimated,
	}.withDefaults()
}

func gatewayPricingRequest(modelID int, candidate schedulercontract.Candidate, usage gatewaycontract.Usage) subscriptioncontract.PricingRequest {
	return subscriptioncontract.PricingRequest{
		ModelID:         modelID,
		ProviderID:      candidate.Provider.ID,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		CacheReadTokens: usage.CachedTokens,
		At:              time.Now().UTC(),
		PricingOverride: cloneAnyMap(candidate.Mapping.PricingOverride),
	}
}

func (rt *runtimeState) gatewayUserPeriodUsage(ctx context.Context, userID int, now time.Time) (int, string, error) {
	logs, err := rt.usage.ListByUser(ctx, userID)
	if err != nil {
		return 0, "", err
	}
	start := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	tokens := 0
	cost := "0.00000000"
	for _, log := range logs {
		if !log.Success || log.CreatedAt.Before(start) {
			continue
		}
		tokens += log.TotalTokens
		cost = addDecimalMoney(cost, log.Cost)
	}
	return tokens, cost, nil
}

func gatewayModelReferences(canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution) []string {
	refs := []string{canonical.CanonicalModel, canonical.Model, resolution.Model.CanonicalName}
	if resolution.Alias != nil {
		refs = append(refs, resolution.Alias.Alias)
		refs = append(refs, resolution.Alias.FallbackModels...)
	}
	return uniqueNonEmptyStrings(refs)
}

func gatewayEntitlementErrorClass(decision subscriptioncontract.EntitlementDecision) string {
	switch strings.TrimSpace(decision.Reason) {
	case "model_not_allowed":
		return "entitlement_model_not_allowed"
	case "monthly_token_quota_exceeded":
		return "monthly_token_quota_exceeded"
	case "monthly_cost_quota_exceeded":
		return "monthly_cost_quota_exceeded"
	default:
		return "entitlement_denied"
	}
}

func gatewayEntitlementHTTPStatus(errorClass string) int {
	switch errorClass {
	case "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded":
		return http.StatusTooManyRequests
	default:
		return http.StatusForbidden
	}
}

func gatewayEntitlementErrorType(errorClass string) apiopenapi.GatewayErrorObjectType {
	switch errorClass {
	case "monthly_token_quota_exceeded", "monthly_cost_quota_exceeded":
		return apiopenapi.RateLimitError
	default:
		return apiopenapi.PermissionError
	}
}

func gatewayEntitlementMessage(errorClass string) string {
	switch errorClass {
	case "entitlement_model_not_allowed":
		return "model not allowed by subscription entitlement"
	case "monthly_token_quota_exceeded":
		return "monthly token quota exceeded"
	case "monthly_cost_quota_exceeded":
		return "monthly cost quota exceeded"
	default:
		return "request not allowed by subscription entitlement"
	}
}

func estimateGatewayRequestUsage(canonical gatewaycontract.CanonicalRequest) gatewaycontract.Usage {
	inputTokens := estimateGatewayTokens(gatewayRequestText(canonical))
	outputTokens := max(1, inputTokens/2)
	if canonical.MaxOutputTokens != nil && *canonical.MaxOutputTokens > 0 {
		outputTokens = *canonical.MaxOutputTokens
	}
	return gatewaycontract.Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Estimated:    true,
	}
}

func gatewayRequestText(canonical gatewaycontract.CanonicalRequest) string {
	parts := []string{canonical.Prompt, canonical.Instructions}
	for _, message := range canonical.Messages {
		parts = append(parts, canonicalContentText(message.Content))
	}
	parts = append(parts, canonicalContentText(canonical.InputItems))
	return strings.Join(uniqueNonEmptyStrings(parts), "\n")
}

func estimateGatewayTokens(text string) int {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		if strings.TrimSpace(text) == "" {
			return 1
		}
		return max(1, len(text)/4)
	}
	return max(1, len(fields)*2)
}

func schedulerStrategyName(value string) schedulercontract.StrategyName {
	switch schedulercontract.StrategyName(strings.TrimSpace(value)) {
	case schedulercontract.StrategyBalanced:
		return schedulercontract.StrategyBalanced
	case schedulercontract.StrategyCostSaver:
		return schedulercontract.StrategyCostSaver
	default:
		return ""
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(trimmed)]; ok {
			continue
		}
		seen[strings.ToLower(trimmed)] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func addDecimalMoney(left string, right string) string {
	leftRat, ok := new(big.Rat).SetString(defaultDecimalMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := new(big.Rat).SetString(defaultDecimalMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return formatDecimalFixed(leftRat.Add(leftRat, rightRat), 8)
}

func defaultDecimalMoney(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.00000000"
	}
	return value
}

func formatDecimalFixed(value *big.Rat, scale int) string {
	if value == nil {
		value = new(big.Rat)
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(multiplier))
	numerator := new(big.Int).Set(scaled.Num())
	denominator := new(big.Int).Set(scaled.Denom())
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	doubleRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	if doubleRemainder.Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	raw := quotient.String()
	if scale == 0 {
		return raw
	}
	for len(raw) <= scale {
		raw = "0" + raw
	}
	return raw[:len(raw)-scale] + "." + raw[len(raw)-scale:]
}

func gatewayScheduleRequest(r *http.Request, canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution) schedulercontract.ScheduleRequest {
	req := schedulercontract.ScheduleRequest{
		RequestID:           canonical.RequestID,
		UserID:              canonical.UserID,
		APIKeyID:            canonical.APIKeyID,
		SourceProtocol:      string(canonical.SourceProtocol),
		SourceEndpoint:      canonical.SourceEndpoint,
		Model:               canonical.CanonicalModel,
		Strategy:            schedulercontract.StrategyBalanced,
		Warnings:            canonical.CompatibilityWarnings,
		RequestCapabilities: gatewayservice.CapabilityDescriptors(canonical),
	}
	if resolution.Alias != nil {
		req.ModelAlias = resolution.Alias.Alias
		req.FallbackModels = append([]string(nil), resolution.Alias.FallbackModels...)
		if strategy := schedulerStrategyHint(resolution.Alias.StrategyHint); strategy != "" {
			req.Strategy = strategy
		}
	}
	req.StickyAccountID, req.StickyStrength, req.SessionAffinityKey, req.SessionAffinitySource = gatewaySessionAffinity(r)
	return req
}

func schedulerStrategyHint(value *string) schedulercontract.StrategyName {
	if value == nil {
		return ""
	}
	switch schedulercontract.StrategyName(strings.TrimSpace(*value)) {
	case schedulercontract.StrategyBalanced:
		return schedulercontract.StrategyBalanced
	case schedulercontract.StrategyCostSaver:
		return schedulercontract.StrategyCostSaver
	default:
		return ""
	}
}

func gatewaySessionAffinity(r *http.Request) (*int, schedulercontract.StickyStrength, string, string) {
	strength := schedulerStickyStrength(firstNonEmpty(
		r.Header.Get("X-SRapi-Sticky-Strength"),
		r.URL.Query().Get("sticky_strength"),
	))
	accountID, accountSource := gatewayStickyAccountID(r)
	key, keySource := gatewaySessionAffinityKey(r)
	if strength == "" && (accountID != nil || key != "") {
		strength = schedulercontract.StickyStrengthSoft
	}
	if accountSource != "" {
		return accountID, strength, key, accountSource
	}
	return accountID, strength, key, keySource
}

func gatewayStickyAccountID(r *http.Request) (*int, string) {
	for _, candidate := range []struct {
		value  string
		source string
	}{
		{r.Header.Get("X-SRapi-Sticky-Account-ID"), "header:x-srapi-sticky-account-id"},
		{r.URL.Query().Get("sticky_account_id"), "query:sticky_account_id"},
	} {
		value := strings.TrimSpace(candidate.value)
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			return &parsed, candidate.source
		}
	}
	return nil, ""
}

func gatewaySessionAffinityKey(r *http.Request) (string, string) {
	for _, candidate := range []struct {
		value  string
		source string
	}{
		{r.Header.Get("X-SRapi-Session-Affinity-Key"), "header:x-srapi-session-affinity-key"},
		{r.URL.Query().Get("session_affinity_key"), "query:session_affinity_key"},
	} {
		value := strings.TrimSpace(candidate.value)
		if value != "" {
			return value, candidate.source
		}
	}
	return "", ""
}

func schedulerStickyStrength(value string) schedulercontract.StickyStrength {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(schedulercontract.StickyStrengthHard):
		return schedulercontract.StickyStrengthHard
	case string(schedulercontract.StickyStrengthSoft):
		return schedulercontract.StickyStrengthSoft
	default:
		return ""
	}
}

func stickyAccountIDFromCandidates(candidates []schedulercontract.Candidate, bindingKey string) *int {
	bindingKey = strings.TrimSpace(bindingKey)
	if bindingKey == "" {
		return nil
	}
	for _, candidate := range candidates {
		if accountMatchesAffinityKey(candidate.Account.Metadata, bindingKey) {
			accountID := candidate.Account.ID
			return &accountID
		}
	}
	return nil
}

func accountMatchesAffinityKey(metadata map[string]any, bindingKey string) bool {
	for _, key := range []string{"session_affinity_key", "sticky_binding_key", "sticky_session_key"} {
		if strings.EqualFold(metadataString(metadata, key), bindingKey) {
			return true
		}
	}
	for _, key := range []string{"session_affinity_keys", "sticky_binding_keys", "sticky_session_keys"} {
		if metadataStringListContains(metadata, key, bindingKey) {
			return true
		}
	}
	return false
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func metadataStringListContains(metadata map[string]any, key string, target string) bool {
	if metadata == nil {
		return false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return false
	}
	switch value := value.(type) {
	case []string:
		for _, item := range value {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprint(item)), target) {
				return true
			}
		}
	case string:
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	}
	return false
}

func metadataStringList(metadata map[string]any, key string) ([]string, bool) {
	if metadata == nil {
		return nil, false
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return nil, false
	}
	switch value := value.(type) {
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, strings.TrimSpace(item))
		}
		return out, true
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return out, true
	case string:
		out := make([]string, 0)
		for _, item := range strings.Split(value, ",") {
			out = append(out, strings.TrimSpace(item))
		}
		return out, true
	default:
		return []string{strings.TrimSpace(fmt.Sprint(value))}, true
	}
}

func accountSupportsUpstreamModel(metadata map[string]any, upstreamModelName string) bool {
	supportedModels, ok := metadataStringList(metadata, "supported_models")
	if !ok {
		return true
	}
	if len(supportedModels) == 0 {
		return false
	}
	target := normalizeDiscoveredModelID(upstreamModelName)
	if target == "" {
		return false
	}
	for _, supported := range supportedModels {
		if strings.EqualFold(normalizeDiscoveredModelID(supported), target) {
			return true
		}
	}
	return false
}

func (rt *runtimeState) gatewayCandidates(ctx context.Context, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey) ([]schedulercontract.Candidate, error) {
	model, err := rt.models.FindByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	mappings, err := rt.models.ListMappingsByModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	accounts, err := rt.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]schedulercontract.Candidate, 0)
	forcedProviderKey = strings.TrimSpace(forcedProviderKey)
	for _, mapping := range mappings {
		provider, err := rt.providers.FindByID(ctx, mapping.ProviderID)
		if err != nil {
			continue
		}
		if forcedProviderKey != "" && provider.Name != forcedProviderKey {
			continue
		}
		for _, account := range accounts {
			if account.ProviderID != mapping.ProviderID {
				continue
			}
			if !accountSupportsUpstreamModel(account.Metadata, mapping.UpstreamModelName) {
				continue
			}
			allowed, err := rt.apiKeyAllowsAccount(ctx, apiKey, account.ID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			runtimeState := rt.accountSchedulerRuntimeState(ctx, account)
			candidates = append(candidates, schedulercontract.Candidate{
				Account:               account,
				Provider:              provider,
				Mapping:               mapping,
				EffectiveCapabilities: effectiveCapabilities(model, mapping, provider, account),
				RuntimeState:          runtimeState,
				Limits:                schedulerRuntimeLimits(account.Metadata),
			})
		}
	}
	return candidates, nil
}

func (rt *runtimeState) accountSchedulerRuntimeState(ctx context.Context, account accountcontract.ProviderAccount) schedulercontract.RuntimeState {
	state := schedulerRuntimeState(account.Metadata)
	if latest, err := rt.accounts.LatestHealthSnapshotByAccount(ctx, account.ID); err == nil {
		healthScore := float64(latest.SuccessRate)
		state.HealthScore = &healthScore
		p95 := latest.LatencyP95MS
		state.LatencyP95MS = &p95
		state.CircuitOpen = state.CircuitOpen || strings.EqualFold(latest.CircuitState, "open")
		state.CooldownActive = state.CooldownActive || (latest.CooldownUntil != nil && latest.CooldownUntil.After(time.Now().UTC()))
	}
	if quotas, err := rt.accounts.ListQuotaSnapshotsByAccount(ctx, account.ID, 1); err == nil && len(quotas) > 0 {
		remainingRatio := float64(quotas[0].RemainingRatio)
		state.QuotaRemainingRatio = &remainingRatio
		state.QuotaExhausted = state.QuotaExhausted || quotas[0].RemainingRatio <= 0
	}
	return state
}

func (rt *runtimeState) apiKeyAllowsAccount(ctx context.Context, apiKey apikeycontract.APIKey, accountID int) (bool, error) {
	if len(apiKey.GroupIDs) == 0 {
		return true, nil
	}
	accountGroupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, accountID)
	if err != nil {
		return false, err
	}
	return intersectsInt(apiKey.GroupIDs, accountGroupIDs), nil
}

func intersectsInt(left []int, right []int) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[int]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func gatewaySourceEndpoint(ctx context.Context, fallback string) string {
	if route, ok := ctx.Value(gatewayRouteContextKey{}).(gatewayRouteContext); ok {
		if sourceEndpoint := strings.TrimSpace(route.SourceEndpoint); sourceEndpoint != "" {
			return sourceEndpoint
		}
	}
	return fallback
}

func gatewayForcedProviderKey(ctx context.Context) string {
	if route, ok := ctx.Value(gatewayRouteContextKey{}).(gatewayRouteContext); ok {
		return strings.TrimSpace(route.ForcedProviderKey)
	}
	return ""
}

func parseGeminiModelAction(path string) (string, bool, bool) {
	raw := strings.TrimPrefix(path, "/v1beta/models/")
	if raw == path || strings.TrimSpace(raw) == "" {
		return "", false, false
	}
	action := ""
	stream := false
	switch {
	case strings.HasSuffix(raw, ":streamGenerateContent"):
		action = ":streamGenerateContent"
		stream = true
	case strings.HasSuffix(raw, ":generateContent"):
		action = ":generateContent"
	default:
		return "", false, false
	}
	model := strings.TrimSuffix(raw, action)
	model = strings.Trim(model, "/")
	if model == "" {
		return "", false, false
	}
	if decoded, err := url.PathUnescape(model); err == nil {
		model = decoded
	}
	model = strings.TrimPrefix(model, "models/")
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false, false
	}
	return model, stream, true
}

func effectiveCapabilities(model modelcontract.Model, mapping modelcontract.ModelProviderMapping, provider providercontract.Provider, account accountcontract.ProviderAccount) []capabilitiescontract.Descriptor {
	merged := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range model.Capabilities {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			merged[normalized.Key] = normalized
		}
	}
	for _, descriptor := range mapping.CapabilityOverride {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			merged[normalized.Key] = normalized
		}
	}
	providerScoped := map[string]capabilitiescontract.Descriptor{}
	for _, descriptor := range mapping.CapabilityOverride {
		if normalized, err := capabilitiescontract.NormalizeDescriptor(descriptor); err == nil {
			providerScoped[normalized.Key] = normalized
		}
	}
	for key, value := range provider.Capabilities {
		capabilityKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(key)
		if ok && boolValue(value) {
			merged[capabilityKey] = capabilityRequirement(capabilityKey)
			providerScoped[capabilityKey] = capabilityRequirement(capabilityKey)
		}
	}
	for key, value := range account.Metadata {
		if strings.HasPrefix(key, "capability_") && boolValue(value) {
			capabilityKey := strings.TrimPrefix(key, "capability_")
			if canonicalKey, ok := capabilitiescontract.CanonicalKeyFromConvenience(capabilityKey); ok {
				merged[canonicalKey] = capabilityRequirement(canonicalKey)
				providerScoped[canonicalKey] = capabilityRequirement(canonicalKey)
			}
		}
	}
	for _, key := range []string{capabilitiescontract.KeyEmbeddings, capabilitiescontract.KeyImages, capabilitiescontract.KeyAudioTranscriptions, capabilitiescontract.KeyAudioSpeech, capabilitiescontract.KeyModerations, capabilitiescontract.KeyRerank, capabilitiescontract.KeyRealtimeWebSocket} {
		if _, ok := providerScoped[key]; !ok {
			delete(merged, key)
		}
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(merged))
	for _, descriptor := range merged {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func capabilityRequirement(key string) capabilitiescontract.Descriptor {
	return capabilitiescontract.Descriptor{
		Key:     key,
		Level:   capabilitiescontract.DescriptorLevelRequired,
		Status:  capabilitiescontract.DescriptorStatusStable,
		Version: "v1",
	}
}

func dedupeCapabilityDescriptors(values []capabilitiescontract.Descriptor) []capabilitiescontract.Descriptor {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]capabilitiescontract.Descriptor{}
	for _, value := range values {
		seen[value.Key] = value
	}
	out := make([]capabilitiescontract.Descriptor, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func schedulerRuntimeState(metadata map[string]any) schedulercontract.RuntimeState {
	quotaRemainingRatio := metadataOptionalFloat(metadata, "remaining_ratio", "quota_remaining_ratio")
	quotaExhausted := metadataBool(metadata, "quota_exhausted")
	if quotaRemainingRatio != nil && *quotaRemainingRatio <= 0 {
		quotaExhausted = true
	}
	return schedulercontract.RuntimeState{
		QuotaExhausted:      quotaExhausted,
		HealthScore:         metadataOptionalFloat(metadata, "health_score"),
		QuotaRemainingRatio: quotaRemainingRatio,
		LatencyP95MS:        metadataOptionalInt(metadata, "latency_p95_ms", "p95_latency_ms", "latency_p95"),
		CircuitOpen:         metadataBool(metadata, "circuit_open"),
		CooldownActive:      metadataBool(metadata, "cooldown_active") || metadataCooldownActive(metadata, time.Now().UTC()),
		CurrentConcurrency:  metadataInt(metadata, "current_concurrency"),
		RPMUsed:             metadataInt(metadata, "rpm_used"),
		TPMUsed:             metadataInt(metadata, "tpm_used"),
	}
}

func schedulerRuntimeLimits(metadata map[string]any) schedulercontract.RuntimeLimits {
	return schedulercontract.RuntimeLimits{
		MaxConcurrency: metadataOptionalInt(metadata, "max_concurrency"),
		RPMLimit:       metadataOptionalInt(metadata, "rpm_limit"),
		TPMLimit:       metadataOptionalInt(metadata, "tpm_limit"),
	}
}

func metadataBool(metadata map[string]any, key string) bool {
	return boolValue(metadata[key])
}

func boolValue(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return err == nil && parsed
	default:
		return false
	}
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	switch value := value.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return int(parsed)
		}
		floatValue, err := value.Float64()
		if err == nil {
			return int(floatValue)
		}
	case string:
		raw := strings.TrimSpace(value)
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			return parsed
		}
		floatValue, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return int(floatValue)
		}
	}
	return 0
}

func metadataOptionalInt(metadata map[string]any, keys ...string) *int {
	if _, ok := metadataValue(metadata, keys...); !ok {
		return nil
	}
	value := metadataInt(metadata, keys...)
	return &value
}

func metadataOptionalFloat(metadata map[string]any, keys ...string) *float64 {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	switch value := value.(type) {
	case int:
		out := float64(value)
		return &out
	case int8:
		out := float64(value)
		return &out
	case int16:
		out := float64(value)
		return &out
	case int32:
		out := float64(value)
		return &out
	case int64:
		out := float64(value)
		return &out
	case uint:
		out := float64(value)
		return &out
	case uint8:
		out := float64(value)
		return &out
	case uint16:
		out := float64(value)
		return &out
	case uint32:
		out := float64(value)
		return &out
	case uint64:
		out := float64(value)
		return &out
	case float32:
		out := float64(value)
		return &out
	case float64:
		return &value
	case json.Number:
		parsed, err := value.Float64()
		if err == nil {
			return &parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func metadataValue(metadata map[string]any, keys ...string) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func metadataCooldownActive(metadata map[string]any, now time.Time) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata["cooldown_until"]
	if !ok {
		return false
	}
	var raw string
	switch value := value.(type) {
	case string:
		raw = value
	default:
		raw = fmt.Sprint(value)
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	return err == nil && now.Before(until)
}

func (rt *runtimeState) invokeProviderText(ctx context.Context, req provideradaptercontract.TextRequest) (provideradaptercontract.TextResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.TextResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.TextResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.TextResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeText(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.TextResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderEmbeddings(ctx context.Context, req provideradaptercontract.EmbeddingRequest) (provideradaptercontract.EmbeddingResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.EmbeddingResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.EmbeddingResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.EmbeddingResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeEmbeddings(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.EmbeddingResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderImageGeneration(ctx context.Context, req provideradaptercontract.ImageGenerationRequest) (provideradaptercontract.ImageGenerationResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.ImageGenerationResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.ImageGenerationResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.ImageGenerationResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeImageGeneration(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderAudioTranscription(ctx context.Context, req provideradaptercontract.AudioTranscriptionRequest) (provideradaptercontract.AudioTranscriptionResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.AudioTranscriptionResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.AudioTranscriptionResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.AudioTranscriptionResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeAudioTranscription(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.AudioTranscriptionResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderAudioSpeech(ctx context.Context, req provideradaptercontract.AudioSpeechRequest) (provideradaptercontract.AudioSpeechResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.AudioSpeechResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.AudioSpeechResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.AudioSpeechResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeAudioSpeech(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.AudioSpeechResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderModerations(ctx context.Context, req provideradaptercontract.ModerationRequest) (provideradaptercontract.ModerationResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.ModerationResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.ModerationResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.ModerationResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeModerations(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ModerationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderRerank(ctx context.Context, req provideradaptercontract.RerankRequest) (provideradaptercontract.RerankResponse, error) {
	if req.Account.ID <= 0 {
		return provideradaptercontract.RerankResponse{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	credential, err := rt.accounts.DecryptCredential(ctx, req.Account.ID)
	if err != nil {
		return provideradaptercontract.RerankResponse{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, req.Account, credential); err != nil {
		return provideradaptercontract.RerankResponse{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	req.Credential = credential
	resp, err := rt.adapters.InvokeRerank(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.RerankResponse{}, err
	}
	return resp, nil
}

func providerTextRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.TextRequest {
	return provideradaptercontract.TextRequest{
		RequestID:       req.RequestID,
		SourceProtocol:  string(req.SourceProtocol),
		SourceEndpoint:  req.SourceEndpoint,
		Model:           req.CanonicalModel,
		Prompt:          req.Prompt,
		Messages:        providerTextMessages(req),
		Instructions:    req.Instructions,
		Stream:          req.Stream,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxOutputTokens,
		Stop:            append([]string(nil), req.Stop...),
		Tools:           cloneMapSlice(req.Tools),
		ToolChoice:      cloneAnyValue(req.ToolChoice),
		ResponseFormat:  cloneAnyMap(req.ResponseFormat),
		Provider:        candidate.Provider,
		Account:         candidate.Account,
		Mapping:         candidate.Mapping,
	}
}

func providerEmbeddingRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.EmbeddingRequest {
	return provideradaptercontract.EmbeddingRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Input:          append([]string(nil), req.EmbeddingInput...),
		EncodingFormat: req.EmbeddingEncoding,
		Dimensions:     cloneIntPtr(req.EmbeddingDimensions),
		User:           req.EmbeddingUser,
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerImageGenerationRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ImageGenerationRequest {
	return provideradaptercontract.ImageGenerationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Prompt:         req.ImagePrompt,
		Count:          req.ImageCount,
		Size:           req.ImageSize,
		Quality:        req.ImageQuality,
		Style:          req.ImageStyle,
		ResponseFormat: req.ImageResponseFormat,
		User:           req.ImageUser,
		Extra:          cloneAnyMap(req.ImageExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerAudioTranscriptionRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.AudioTranscriptionRequest {
	return provideradaptercontract.AudioTranscriptionRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		FileName:       req.AudioFileName,
		ContentType:    req.AudioContentType,
		Audio:          append([]byte(nil), req.AudioBytes...),
		Language:       req.AudioLanguage,
		Prompt:         req.AudioPrompt,
		ResponseFormat: req.AudioResponseFormat,
		Temperature:    cloneFloat32Ptr(req.AudioTemperature),
		User:           req.AudioUser,
		Extra:          cloneAnyMap(req.AudioExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerAudioSpeechRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.AudioSpeechRequest {
	return provideradaptercontract.AudioSpeechRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Input:          req.SpeechInput,
		Voice:          req.SpeechVoice,
		ResponseFormat: req.SpeechResponseFormat,
		Speed:          cloneFloat32Ptr(req.SpeechSpeed),
		Instructions:   req.SpeechInstructions,
		User:           req.SpeechUser,
		Extra:          cloneAnyMap(req.SpeechExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerModerationRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ModerationRequest {
	return provideradaptercontract.ModerationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Input:          append([]string(nil), req.ModerationInput...),
		User:           req.ModerationUser,
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerRerankRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.RerankRequest {
	return provideradaptercontract.RerankRequest{
		RequestID:       req.RequestID,
		SourceProtocol:  string(req.SourceProtocol),
		SourceEndpoint:  req.SourceEndpoint,
		Model:           req.CanonicalModel,
		Query:           req.RerankQuery,
		Documents:       providerRerankDocuments(req.RerankDocuments),
		TopN:            cloneIntPtr(req.RerankTopN),
		ReturnDocuments: req.RerankReturnDocuments,
		User:            req.RerankUser,
		Provider:        candidate.Provider,
		Account:         candidate.Account,
		Mapping:         candidate.Mapping,
	}
}

func providerRerankDocuments(values []gatewaycontract.RerankDocument) []provideradaptercontract.RerankDocument {
	if values == nil {
		return nil
	}
	out := make([]provideradaptercontract.RerankDocument, len(values))
	for idx, value := range values {
		out[idx] = provideradaptercontract.RerankDocument{
			Text:     value.Text,
			Fields:   cloneAnyMap(value.Fields),
			Original: cloneAnyValue(value.Original),
		}
	}
	return out
}

func providerTextMessages(req gatewaycontract.CanonicalRequest) []provideradaptercontract.TextMessage {
	out := make([]provideradaptercontract.TextMessage, 0, len(req.Messages)+1)
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		content := canonicalContentText(message.Content)
		if content == "" {
			continue
		}
		out = append(out, provideradaptercontract.TextMessage{Role: role, Content: content})
	}
	if len(out) == 0 {
		content := canonicalContentText(req.InputItems)
		if content != "" {
			out = append(out, provideradaptercontract.TextMessage{Role: "user", Content: content})
		}
	}
	return out
}

func canonicalContentText(blocks []gatewaycontract.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func (rt *runtimeState) refreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	if !shouldRefreshCredential(account, credential) {
		return credential, false, nil
	}
	before, err := rt.accounts.FindByID(ctx, account.ID)
	if err != nil {
		rt.logger.Warn("failed to load provider account before refresh", "error", err, "account_id", account.ID)
		return credential, false, err
	}
	response, err := rt.reverseProxy.Refresh(ctx, reverseproxycontract.RefreshRequest{
		Account: reverseProxyAccountRuntime(account, credential),
	})
	if err != nil {
		rt.recordAudit(ctx, auditcontract.RecordRequest{
			Action:       "provider_account.oauth_refresh_failed",
			ResourceType: "provider_account",
			ResourceID:   strconv.Itoa(account.ID),
			Before:       accountAuditSnapshot(before),
			After:        map[string]any{"refresh_status": "failed", "error_class": errorClassName(err)},
			TraceID:      requestIDFromContext(ctx),
		})
		return credential, false, err
	}
	refreshed := response.Credential
	updated, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Credential: &refreshed})
	if err != nil {
		rt.logger.Warn("failed to persist refreshed provider credential", "error", err, "account_id", account.ID)
		return credential, false, err
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.oauth_refresh",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(account.ID),
		Before:       accountAuditSnapshot(before),
		After: map[string]any{
			"refresh_status":     "success",
			"refreshed_at":       response.RefreshedAt,
			"credential_version": updated.CredentialVersion,
		},
		TraceID: requestIDFromContext(ctx),
	})
	return refreshed, true, nil
}

func shouldRefreshCredential(account accountcontract.ProviderAccount, credential map[string]any) bool {
	if account.RuntimeClass != accountcontract.RuntimeClassOauthRefresh && account.RuntimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		return false
	}
	if metadataBool(account.Metadata, "force_refresh") || metadataBool(account.Metadata, "access_token_expired") {
		return true
	}
	expiresAt := mapString(credential, "expires_at")
	if expiresAt == "" {
		return mapString(credential, "access_token") == ""
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	return err == nil && time.Now().UTC().After(parsed.Add(-30*time.Second))
}

func errorClassName(err error) string {
	var runtimeErr reverseproxycontract.RuntimeError
	if errors.As(err, &runtimeErr) && strings.TrimSpace(runtimeErr.Class) != "" {
		return runtimeErr.Class
	}
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) && strings.TrimSpace(providerErr.Class) != "" {
		return providerErr.Class
	}
	return "unknown"
}

func (rt *runtimeState) applyProviderAccountProtection(ctx context.Context, account accountcontract.ProviderAccount, err error) {
	if account.ID <= 0 || account.RuntimeClass == accountcontract.RuntimeClassAPIKey {
		return
	}
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return
	}
	nextStatus, ok := reverseProxyAccountFailureStatus(providerErr.Class)
	if !ok || account.Status == nextStatus {
		return
	}
	before, findErr := rt.accounts.FindByID(ctx, account.ID)
	if findErr != nil {
		rt.logger.Warn("failed to load reverse proxy account for protection", "error", findErr, "account_id", account.ID)
		return
	}
	updated, updateErr := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Status: &nextStatus})
	if updateErr != nil {
		rt.logger.Warn("failed to protect reverse proxy account", "error", updateErr, "account_id", account.ID, "status", nextStatus)
		return
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.auto_protect",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(account.ID),
		Before:       accountAuditSnapshot(before),
		After:        accountAuditSnapshot(updated),
		TraceID:      requestIDFromContext(ctx),
	})
}

func reverseProxyAccountFailureStatus(class string) (accountcontract.Status, bool) {
	switch strings.TrimSpace(class) {
	case "session_invalid", "account_locked", "device_unrecognized":
		return accountcontract.StatusNeedsReauth, true
	case "account_banned", "abuse_detected":
		return accountcontract.StatusDisabled, true
	default:
		return "", false
	}
}

func gatewayUsageFromProvider(resp provideradaptercontract.TextResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromEmbeddingProvider(resp provideradaptercontract.EmbeddingResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromImageProvider(resp provideradaptercontract.ImageGenerationResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromAudioTranscriptionProvider(resp provideradaptercontract.AudioTranscriptionResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromAudioSpeechProvider(resp provideradaptercontract.AudioSpeechResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromModerationProvider(resp provideradaptercontract.ModerationResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromRerankProvider(resp provideradaptercontract.RerankResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayEmbeddingsFromProvider(resp provideradaptercontract.EmbeddingResponse) []gatewaycontract.Embedding {
	out := make([]gatewaycontract.Embedding, 0, len(resp.Data))
	for _, item := range resp.Data {
		out = append(out, gatewaycontract.Embedding{
			Index:        item.Index,
			Vector:       append([]float32(nil), item.Vector...),
			Base64Vector: item.Base64Vector,
		})
	}
	return out
}

func gatewayImagesFromProvider(resp provideradaptercontract.ImageGenerationResponse) []gatewaycontract.Image {
	out := make([]gatewaycontract.Image, 0, len(resp.Data))
	for _, item := range resp.Data {
		out = append(out, gatewaycontract.Image{
			URL:           item.URL,
			Base64JSON:    item.Base64JSON,
			RevisedPrompt: item.RevisedPrompt,
			Metadata:      cloneAnyMap(item.Metadata),
		})
	}
	return out
}

func gatewayAudioTranscriptionSegmentsFromProvider(resp provideradaptercontract.AudioTranscriptionResponse) []gatewaycontract.AudioTranscriptionSegment {
	if len(resp.Segments) == 0 {
		return nil
	}
	out := make([]gatewaycontract.AudioTranscriptionSegment, 0, len(resp.Segments))
	for _, item := range resp.Segments {
		out = append(out, gatewaycontract.AudioTranscriptionSegment{
			ID:               cloneIntPtr(item.ID),
			Seek:             cloneIntPtr(item.Seek),
			Start:            cloneFloat32Ptr(item.Start),
			End:              cloneFloat32Ptr(item.End),
			Text:             item.Text,
			Tokens:           append([]int(nil), item.Tokens...),
			Temperature:      cloneFloat32Ptr(item.Temperature),
			AvgLogprob:       cloneFloat32Ptr(item.AvgLogprob),
			CompressionRatio: cloneFloat32Ptr(item.CompressionRatio),
			NoSpeechProb:     cloneFloat32Ptr(item.NoSpeechProb),
			Metadata:         cloneAnyMap(item.Metadata),
		})
	}
	return out
}

func gatewayModerationResultsFromProvider(resp provideradaptercontract.ModerationResponse) []gatewaycontract.ModerationResult {
	out := make([]gatewaycontract.ModerationResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		out = append(out, gatewaycontract.ModerationResult{
			Flagged:                   item.Flagged,
			Categories:                cloneBoolMap(item.Categories),
			CategoryScores:            cloneFloat32Map(item.CategoryScores),
			CategoryAppliedInputTypes: cloneStringSliceMap(item.CategoryAppliedInputTypes),
		})
	}
	return out
}

func gatewayRerankResultsFromProvider(resp provideradaptercontract.RerankResponse) []gatewaycontract.RerankResult {
	out := make([]gatewaycontract.RerankResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		result := gatewaycontract.RerankResult{
			Index:          item.Index,
			RelevanceScore: item.RelevanceScore,
			Metadata:       cloneAnyMap(item.Metadata),
		}
		if item.Document != nil {
			document := gatewayRerankDocumentFromProvider(*item.Document)
			result.Document = &document
		}
		out = append(out, result)
	}
	return out
}

func gatewayRerankDocumentFromProvider(value provideradaptercontract.RerankDocument) gatewaycontract.RerankDocument {
	return gatewaycontract.RerankDocument{
		Text:     value.Text,
		Fields:   cloneAnyMap(value.Fields),
		Original: cloneAnyValue(value.Original),
	}
}

func providerGatewayError(err error) (string, int, apiopenapi.GatewayErrorObjectType) {
	var providerErr provideradaptercontract.ProviderError
	if errors.As(err, &providerErr) {
		errorClass := strings.TrimSpace(providerErr.Class)
		if errorClass == "" {
			errorClass = "upstream_error"
		}
		statusCode := providerErr.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		return errorClass, statusCode, gatewayErrorTypeForProviderClass(errorClass)
	}
	return "upstream_error", http.StatusBadGateway, apiopenapi.UpstreamError
}

func gatewayErrorTypeForProviderClass(errorClass string) apiopenapi.GatewayErrorObjectType {
	switch errorClass {
	case "invalid_request":
		return apiopenapi.InvalidRequestError
	case "rate_limit":
		return apiopenapi.RateLimitError
	case "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized":
		return apiopenapi.PermissionError
	case "timeout", "network_error", "stream_interrupted", "no_available_account":
		return apiopenapi.ServiceUnavailableError
	default:
		return apiopenapi.UpstreamError
	}
}

func providerGatewayHTTPStatus(upstreamStatus int) int {
	switch upstreamStatus {
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusBadGateway
	case http.StatusBadRequest:
		return http.StatusBadRequest
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return http.StatusGatewayTimeout
	default:
		if upstreamStatus >= 500 {
			return http.StatusBadGateway
		}
		return http.StatusBadGateway
	}
}

func providerGatewayMessage(errorClass string) string {
	switch errorClass {
	case "rate_limit":
		return "provider rate limit"
	case "auth_failed", "auth_error", "credential_error":
		return "provider authentication failed"
	case "invalid_request":
		return "provider rejected request"
	case "model_unavailable":
		return "provider model unavailable"
	case "provider_5xx":
		return "provider service error"
	case "session_invalid":
		return "provider session invalid"
	case "account_locked":
		return "provider account locked"
	case "account_banned":
		return "provider account banned"
	case "abuse_detected":
		return "provider abuse signal detected"
	case "challenge_required", "captcha_required":
		return "provider challenge required"
	case "geo_blocked":
		return "provider geo blocked"
	case "device_unrecognized":
		return "provider device unrecognized"
	case "upstream_client_outdated":
		return "provider upstream client outdated"
	case "timeout":
		return "provider request timed out"
	case "network_error":
		return "provider network error"
	case "stream_interrupted":
		return "provider stream interrupted"
	default:
		return "provider request failed"
	}
}

func (rt *runtimeState) recordGatewayUsage(ctx context.Context, rec gatewayUsageRecord) {
	model := fallbackModelName(rec.Model)
	if rec.AttemptNo == 0 {
		rec.AttemptNo = 1
	}
	pricing := rec.Pricing.withDefaults()
	usageLog, usageErr := rt.usage.Record(ctx, usagecontract.RecordRequest{
		RequestID:             rec.RequestID,
		UserID:                rec.Authed.UserID,
		APIKeyID:              rec.Authed.Key.ID,
		ProviderID:            rec.ProviderID,
		AccountID:             rec.AccountID,
		SourceProtocol:        rec.SourceProtocol,
		SourceEndpoint:        rec.SourceEndpoint,
		TargetProtocol:        rec.TargetProtocol,
		Model:                 model,
		InputTokens:           rec.InputTokens,
		OutputTokens:          rec.OutputTokens,
		CachedTokens:          rec.CachedTokens,
		UsageEstimated:        rec.UsageEstimated,
		LatencyMS:             rec.LatencyMS,
		Success:               rec.Success,
		ErrorClass:            rec.ErrorClass,
		Cost:                  pricing.Amount,
		Currency:              pricing.Currency,
		CompatibilityWarnings: rec.CompatibilityWarnings,
	})
	if usageErr != nil {
		rt.logger.Warn("failed to record usage log", "error", usageErr, "request_id", rec.RequestID)
		rt.enqueueGatewayUsageFailureEvent(ctx, rec, model)
	} else {
		rt.recordUsageBilling(ctx, usageLog, pricing)
		rt.enqueueGatewayUsageEvent(ctx, usageLog)
	}
	if rec.DecisionID <= 0 || rec.AccountID == nil || rec.ProviderID == nil {
		return
	}
	_, feedbackErr := rt.scheduler.RecordFeedback(ctx, schedulercontract.RecordFeedbackRequest{
		RequestID:    rec.RequestID,
		DecisionID:   rec.DecisionID,
		AttemptNo:    rec.AttemptNo,
		AccountID:    *rec.AccountID,
		ProviderID:   *rec.ProviderID,
		Model:        model,
		Success:      rec.Success,
		ErrorClass:   rec.ErrorClass,
		StatusCode:   rec.StatusCode,
		LatencyMS:    rec.LatencyMS,
		InputTokens:  rec.InputTokens,
		OutputTokens: rec.OutputTokens,
		CachedTokens: rec.CachedTokens,
		ActualCost:   pricing.Amount,
		Currency:     pricing.Currency,
	})
	if feedbackErr != nil {
		rt.logger.Warn("failed to record scheduler feedback", "error", feedbackErr, "request_id", rec.RequestID)
	}
	if !rec.Success && rec.ErrorClass != nil && *rec.ErrorClass == "rate_limit" {
		rt.applyProviderRateLimitCooldown(ctx, *rec.AccountID)
	}
	rt.recordGatewayAccountSnapshots(ctx, rec)
}

func (rt *runtimeState) applyProviderRateLimitCooldown(ctx context.Context, accountID int) {
	if accountID <= 0 {
		return
	}
	account, err := rt.accounts.FindByID(ctx, accountID)
	if err != nil {
		rt.logger.Warn("failed to load rate-limited provider account", "error", err, "account_id", accountID)
		return
	}
	metadata := cloneMetadata(account.Metadata)
	metadata["cooldown_active"] = true
	metadata["cooldown_reason"] = "rate_limit"
	metadata["cooldown_until"] = time.Now().UTC().Add(rateLimitCooldownWindow).Format(time.RFC3339)
	metadata["last_error_class"] = "rate_limit"
	before := accountAuditSnapshot(account)
	updated, err := rt.accounts.Update(ctx, accountID, accountcontract.UpdateRequest{Metadata: &metadata})
	if err != nil {
		rt.logger.Warn("failed to apply provider account rate limit cooldown", "error", err, "account_id", accountID)
		return
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.cooldown",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(accountID),
		Before:       before,
		After:        accountAuditSnapshot(updated),
		TraceID:      requestIDFromContext(ctx),
	})
}

func (rt *runtimeState) recordGatewayAccountSnapshots(ctx context.Context, rec gatewayUsageRecord) {
	if rec.AccountID == nil || rec.ProviderID == nil {
		return
	}
	account, err := rt.accounts.FindByID(ctx, *rec.AccountID)
	if err != nil {
		rt.logger.Warn("failed to load provider account for snapshot", "error", err, "account_id", *rec.AccountID)
		return
	}
	usageLogs, err := rt.usage.List(ctx)
	if err != nil {
		rt.logger.Warn("failed to list usage logs for account snapshot", "error", err, "account_id", *rec.AccountID)
		return
	}
	now := time.Now().UTC()
	health := buildAccountHealthSnapshot(account, usageLogsForAccount(usageLogs, account.ID), now)
	if _, err := rt.accounts.RecordHealthSnapshot(ctx, accountHealthSnapshotFromAPI(health)); err != nil {
		rt.logger.Warn("failed to record account health snapshot", "error", err, "account_id", account.ID)
	}
	quota := buildAccountQuotaSnapshot(account, usageLogsForAccount(usageLogs, account.ID), now)
	if _, err := rt.accounts.RecordQuotaSnapshot(ctx, accountQuotaSnapshotFromAPI(quota)); err != nil {
		rt.logger.Warn("failed to record account quota snapshot", "error", err, "account_id", account.ID)
	}
}

func (rt *runtimeState) recordAccountTestHealthSnapshot(ctx context.Context, account accountcontract.ProviderAccount, result apiopenapi.AdminTestResult) {
	status := "healthy"
	successRate := float32(1)
	errorRate := float32(0)
	if !result.Ok {
		status = "degraded"
		successRate = 0
		errorRate = 1
	}
	latencyMS := 0
	if result.LatencyMs != nil {
		latencyMS = *result.LatencyMs
	}
	_, err := rt.accounts.RecordHealthSnapshot(ctx, accountcontract.AccountHealthSnapshot{
		AccountID:     account.ID,
		ProviderID:    account.ProviderID,
		Status:        status,
		SuccessRate:   successRate,
		ErrorRate:     errorRate,
		LatencyP50MS:  latencyMS,
		LatencyP95MS:  latencyMS,
		CircuitState:  accountCircuitState(account),
		SnapshotAt:    result.CheckedAt,
		CooldownUntil: metadataOptionalTime(account.Metadata, "cooldown_until"),
	})
	if err != nil {
		rt.logger.Warn("failed to record account test health snapshot", "error", err, "account_id", account.ID)
	}
}

func (rt *runtimeState) recordUsageBilling(ctx context.Context, log usagecontract.UsageLog, pricing gatewayPricingEvidence) {
	if !log.Success {
		return
	}
	pricing = pricing.withDefaults()
	metadata := map[string]any{
		"request_id":        log.RequestID,
		"model":             log.Model,
		"source_endpoint":   log.SourceEndpoint,
		"total_tokens":      log.TotalTokens,
		"usage_estimated":   log.UsageEstimated,
		"pricing_source":    pricing.PricingSource,
		"pricing_estimated": pricing.PricingEstimated,
	}
	if pricing.PricingRuleID != nil {
		metadata["pricing_rule_id"] = *pricing.PricingRuleID
	}
	_, err := rt.billing.Record(ctx, billingcontract.RecordRequest{
		UserID:        log.UserID,
		Type:          billingcontract.LedgerTypeUsageCharge,
		Amount:        log.Cost,
		Currency:      log.Currency,
		BalanceBefore: "0.00000000",
		BalanceAfter:  "0.00000000",
		ReferenceType: "usage_log",
		ReferenceID:   strconv.Itoa(log.ID),
		Metadata:      metadata,
	})
	if err != nil {
		rt.logger.Warn("failed to record billing ledger", "error", err, "request_id", log.RequestID)
	}
}

func (rt *runtimeState) enqueueGatewayUsageEvent(ctx context.Context, log usagecontract.UsageLog) {
	payload := map[string]any{
		"usage_log_id":           log.ID,
		"request_id":             log.RequestID,
		"user_id":                log.UserID,
		"api_key_id":             log.APIKeyID,
		"source_protocol":        log.SourceProtocol,
		"source_endpoint":        log.SourceEndpoint,
		"target_protocol":        log.TargetProtocol,
		"model":                  log.Model,
		"input_tokens":           log.InputTokens,
		"output_tokens":          log.OutputTokens,
		"cached_tokens":          log.CachedTokens,
		"total_tokens":           log.TotalTokens,
		"success":                log.Success,
		"usage_estimated":        log.UsageEstimated,
		"compatibility_warnings": nonNilStrings(log.CompatibilityWarnings),
	}
	addOptionalInt(payload, "provider_id", log.ProviderID)
	addOptionalInt(payload, "account_id", log.AccountID)
	if log.ErrorClass != nil {
		payload["error_class"] = *log.ErrorClass
	}
	_, err := rt.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "usage_log",
		AggregateID:    strconv.Itoa(log.ID),
		CorrelationID:  log.RequestID,
		CausationID:    log.RequestID,
		IdempotencyKey: log.RequestID,
		Payload:        payload,
		Metadata: map[string]any{
			"source_protocol": log.SourceProtocol,
			"source_endpoint": log.SourceEndpoint,
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway usage event", "error", err, "request_id", log.RequestID)
	}
}

func (rt *runtimeState) enqueueGatewayUsageFailureEvent(ctx context.Context, rec gatewayUsageRecord, model string) {
	payload := map[string]any{
		"request_id":             rec.RequestID,
		"user_id":                rec.Authed.UserID,
		"api_key_id":             rec.Authed.Key.ID,
		"source_protocol":        rec.SourceProtocol,
		"source_endpoint":        rec.SourceEndpoint,
		"target_protocol":        rec.TargetProtocol,
		"model":                  model,
		"input_tokens":           rec.InputTokens,
		"output_tokens":          rec.OutputTokens,
		"cached_tokens":          rec.CachedTokens,
		"total_tokens":           rec.InputTokens + rec.OutputTokens + rec.CachedTokens,
		"success":                rec.Success,
		"usage_estimated":        rec.UsageEstimated,
		"compatibility_warnings": nonNilStrings(rec.CompatibilityWarnings),
	}
	addOptionalInt(payload, "provider_id", rec.ProviderID)
	addOptionalInt(payload, "account_id", rec.AccountID)
	if rec.ErrorClass != nil {
		payload["error_class"] = *rec.ErrorClass
	}
	_, err := rt.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "gateway_request",
		AggregateID:    rec.RequestID,
		CorrelationID:  rec.RequestID,
		CausationID:    rec.RequestID,
		IdempotencyKey: rec.RequestID,
		Payload:        payload,
		Metadata: map[string]any{
			"source_protocol": rec.SourceProtocol,
			"source_endpoint": rec.SourceEndpoint,
			"usage_recorded":  false,
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway usage failure event", "error", err, "request_id", rec.RequestID)
	}
}
