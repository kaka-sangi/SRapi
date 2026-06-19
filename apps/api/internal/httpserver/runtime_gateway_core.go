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
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	provideradapterservice "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/service"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	"github.com/srapi/srapi/apps/api/internal/platform/circuitbreaker"
	"github.com/srapi/srapi/apps/api/internal/platform/glob"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
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
	CacheCreationTokens   int
	UsageEstimated        bool
	Pricing               gatewayPricingEvidence
	RequestedModel        string
	UpstreamModel         string
	CompatibilityWarnings []string
	ProviderQuotaSignals  []provideradaptercontract.QuotaSignal
	ProviderRetryAfter    *time.Time
	ProviderErrorMessage  string
	// ProviderErrorBodyExcerpt mirrors sub2api's upstream_error_detail
	// (ops_error_logs.upstream_error_detail / migrations/034). It carries
	// the upstream error envelope ({class, type, code, message}) compacted
	// into a single string so the admin panel can render the verbatim
	// upstream error without re-reading the upstream response body.
	ProviderErrorBodyExcerpt string
	// Headers carries the upstream provider response headers from a failed
	// attempt (e.g. the x-codex-* rate-limit telemetry). It is populated only on
	// failure paths so the cooldown stage can fold provider quota telemetry into
	// account metadata without re-reading the upstream response.
	Headers http.Header
	// UpstreamRequestID carries the upstream provider's request id extracted
	// from the failing response headers (x-request-id / openai-request-id /
	// x-codex-request-id). Empty when the upstream did not return one.
	UpstreamRequestID string
	// StreamCompletionState captures gateway stream terminal state in a
	// low-cardinality form: completed, interrupted, idle_timeout, failed, or
	// unknown. It must never carry provider-native frames or response bodies.
	StreamCompletionState string
	// ErrorPhase / ErrorOwner / ErrorSource classify the failure for the
	// admin error-log panel. See runtime_gateway_failover.go for the
	// taxonomy mapping helpers.
	ErrorPhase  string
	ErrorOwner  string
	ErrorSource string
	// UpstreamErrors is the per-attempt failover history for this request:
	// one event per failed candidate attempt. Carried into the persisted
	// usage_log so the admin panel can render the timeline.
	UpstreamErrors []gatewayUpstreamErrorEvent
	// DiagnosticMetadata carries low-cardinality, sanitized operator evidence
	// for system logs. Keep it free of prompts, credentials, raw account names,
	// and other high-sensitive request data.
	DiagnosticMetadata map[string]any
	QualityPrompt      string
	QualityOutput      string
	FeedbackID         int
}

// gatewayUpstreamErrorEvent mirrors the persisted UpstreamErrorEvent for the
// in-process failover state; converted 1:1 when handed off to the usage layer.
type gatewayUpstreamErrorEvent struct {
	AtUnixMs           int64
	AttemptNo          int
	AccountID          *int
	AccountName        string
	UpstreamStatusCode int
	UpstreamRequestID  string
	UpstreamURL        string
	Kind               string
	Message            string
	BodyExcerpt        string
}

type gatewayAdmission struct {
	EstimatedUsage gatewaycontract.Usage
	Pricing        gatewayPricingEvidence
	Entitlement    subscriptioncontract.EntitlementDecision
	RateLimit      ratelimit.Decision
}

type gatewayPricingEvidence struct {
	Amount           string
	Currency         string
	PricingRuleID    *int
	BillingMode      billingcontract.BillingMode
	InputCost        string
	OutputCost       string
	CacheReadCost    string
	CacheWriteCost   string
	PricingSource    string
	PricingEstimated bool
	ActualCost       string
	BillableCost     string
}

type providerDispatchState struct {
	credential        map[string]any
	concurrencyLeases []ratelimit.ConcurrencyLease
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
	if e.BillingMode == "" {
		e.BillingMode = billingcontract.BillingModeToken
	}
	if strings.TrimSpace(e.InputCost) == "" {
		e.InputCost = "0.00000000"
	}
	if strings.TrimSpace(e.OutputCost) == "" {
		e.OutputCost = "0.00000000"
	}
	if strings.TrimSpace(e.CacheReadCost) == "" {
		e.CacheReadCost = "0.00000000"
	}
	if strings.TrimSpace(e.CacheWriteCost) == "" {
		e.CacheWriteCost = "0.00000000"
	}
	if strings.TrimSpace(e.ActualCost) == "" {
		e.ActualCost = e.Amount
	}
	if strings.TrimSpace(e.BillableCost) == "" {
		e.BillableCost = e.ActualCost
	}
	return e
}

func (rt *runtimeState) scheduleGatewayRequest(ctx context.Context, req schedulercontract.ScheduleRequest, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey) (schedulercontract.ScheduleResult, error) {
	candidates, err := rt.gatewayCandidates(ctx, modelID, forcedProviderKey, apiKey, req.SourceEndpoint)
	if err != nil {
		return schedulercontract.ScheduleResult{}, err
	}
	if len(req.AccountGroupScope) > 0 {
		candidates, err = rt.filterCandidatesByAccountGroupScope(ctx, candidates, req.AccountGroupScope)
		if err != nil {
			return schedulercontract.ScheduleResult{}, err
		}
	}
	candidates = rt.filterCandidatesByAvailableProxy(ctx, candidates)
	// Drop accounts restricted to official client signatures the caller doesn't
	// match (e.g. an OAuth account marked codex-only), so a generic client can't
	// drive an account that would get banned for it.
	candidates = filterCandidatesByAllowedClients(candidates, gatewayInboundClientFromContext(ctx))
	// Drop accounts already at their per-account active-session cap (max_sessions),
	// excluding this conversation so it is never evicted from its own account.
	candidates = rt.filterCandidatesBySessionLimit(ctx, candidates, req.SessionAffinityKey)
	candidates = rt.filterCandidatesByEnabledChannels(ctx, candidates)
	if req.StickyAccountID == nil && strings.TrimSpace(req.SessionAffinityKey) != "" {
		// Prefer a persisted session→account binding (automatic stickiness across
		// turns); only honor it when the bound account is still a live candidate
		// for this request, so a drained/disabled account never traps a session.
		if accountID, ok := rt.lookupGatewaySessionAffinity(ctx, req.APIKeyID, req.SessionAffinityKey); ok && candidatesContainAccount(candidates, accountID) {
			boundAccountID := accountID
			req.StickyAccountID = &boundAccountID
		} else {
			// Fall back to operator-pinned account metadata affinity keys.
			req.StickyAccountID = stickyAccountIDFromCandidates(candidates, req.SessionAffinityKey)
		}
	}
	candidates = rt.applyGatewayQualityScores(ctx, candidates, req.Model)
	req.Candidates = candidates
	rt.applyGatewayStrategyRollout(ctx, &req, apiKey)
	result, err := rt.scheduler.Schedule(ctx, req)
	if err != nil {
		if rt.metrics != nil {
			rt.metrics.RecordSchedulerDecision(result.Decision)
		}
		return result, err
	}
	if rt.metrics != nil {
		rt.metrics.RecordSchedulerDecision(result.Decision)
	}
	return applyAccountModelMapping(result, req.Model, req.SourceEndpoint), nil
}

// accountModelMappingMetadataKey is the provider-account metadata key holding a
// per-account "canonical catalog model -> upstream model name" override map. It
// lets two accounts of the same provider serve the same catalog model from
// different upstream model names (sub2api per-channel model_mapping parity).
const accountModelMappingMetadataKey = "model_mapping"

// accountCompactModelMappingMetadataKey is the provider-account metadata key
// holding compact-only upstream model overrides. It is evaluated only for
// /responses/compact requests, matching sub2api's compact_model_mapping
// behavior without changing normal /responses or chat traffic.
const accountCompactModelMappingMetadataKey = "compact_model_mapping"

// applyAccountModelMapping overrides the scheduled candidate's upstream model
// name when its account carries a per-account model_mapping override for the
// requested canonical model. The failover loop re-schedules on every attempt,
// so applying to result.Candidate here covers each attempt's chosen account.
func applyAccountModelMapping(result schedulercontract.ScheduleResult, canonicalModel string, sourceEndpoint string) schedulercontract.ScheduleResult {
	result.Candidate.Mapping = providerEffectiveModelMapping(result.Candidate.Provider, accountEffectiveModelMapping(result.Candidate.Mapping, result.Candidate.Account, canonicalModel, sourceEndpoint))
	return result
}

func accountEffectiveModelMapping(mapping modelcontract.ModelProviderMapping, account accountcontract.ProviderAccount, canonicalModel string, sourceEndpoint string) modelcontract.ModelProviderMapping {
	if override := accountModelOverride(account, canonicalModel, sourceEndpoint); override != "" {
		mapping.UpstreamModelName = override
	}
	return mapping
}

// accountModelOverride returns the per-account upstream model name configured
// for canonicalModel, or "" when the account has no (valid, non-blank) override.
func accountModelOverride(account accountcontract.ProviderAccount, canonicalModel string, sourceEndpoint string) string {
	model := strings.TrimSpace(canonicalModel)
	if model == "" || len(account.Metadata) == 0 {
		return ""
	}
	if gatewaySourceEndpointIsResponsesCompact(sourceEndpoint) {
		if override := accountModelOverrideFromMetadata(account.Metadata, accountCompactModelMappingMetadataKey, model); override != "" {
			return override
		}
	}
	return accountModelOverrideFromMetadata(account.Metadata, accountModelMappingMetadataKey, model)
}

func accountModelOverrideFromMetadata(metadata map[string]any, key string, model string) string {
	if metadata == nil {
		return ""
	}
	mapping := accountModelMappingFromMetadataValue(metadata[key])
	if len(mapping) == 0 {
		return ""
	}
	if override := accountModelExactOverride(mapping, model); override != "" {
		return override
	}
	suffix := accountModelSuffix(model)
	baseModel := strings.TrimSpace(strings.TrimSuffix(model, suffix))
	if override := accountModelExactOverride(mapping, baseModel); override != "" {
		return accountModelApplySuffix(override, suffix)
	}
	return accountModelApplySuffix(accountModelWildcardOverride(mapping, baseModel), suffix)
}

func accountModelMappingFromMetadataValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return nil
	}
}

func accountModelExactOverride(mapping map[string]any, model string) string {
	if override, ok := mapping[model].(string); ok {
		return strings.TrimSpace(override)
	}
	for pattern, raw := range mapping {
		if !strings.EqualFold(strings.TrimSpace(pattern), model) {
			continue
		}
		override, ok := raw.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(override); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func accountModelWildcardOverride(mapping map[string]any, model string) string {
	type wildcardMatch struct {
		pattern  string
		override string
	}
	matches := make([]wildcardMatch, 0)
	for pattern, raw := range mapping {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || !strings.Contains(pattern, "*") || !glob.Match(pattern, model) {
			continue
		}
		override, ok := raw.(string)
		if !ok {
			continue
		}
		override = strings.TrimSpace(override)
		if override == "" {
			continue
		}
		matches = append(matches, wildcardMatch{pattern: pattern, override: override})
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		if len(matches[i].pattern) != len(matches[j].pattern) {
			return len(matches[i].pattern) > len(matches[j].pattern)
		}
		return matches[i].pattern < matches[j].pattern
	})
	return matches[0].override
}

func accountModelSuffix(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || !strings.HasSuffix(model, ")") {
		return ""
	}
	open := strings.LastIndex(model, "(")
	if open <= 0 || open == len(model)-2 {
		return ""
	}
	return model[open:]
}

func accountModelApplySuffix(override string, suffix string) string {
	override = strings.TrimSpace(override)
	if override == "" || suffix == "" || accountModelSuffix(override) != "" {
		return override
	}
	return override + suffix
}

func gatewaySourceEndpointIsResponsesCompact(sourceEndpoint string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(sourceEndpoint)), "/responses/compact")
}

// Gateway balance-gate and entitlement-error mapping helpers live in
// runtime_gateway_entitlement.go to keep this route-family file within the
// architecture size budget.

func gatewayRateLimitReason(name string) string {
	switch strings.TrimSpace(name) {
	case "rpm", "model_rpm", "group_rpm":
		return "rpm_limit_exceeded"
	case "tpm", "model_tpm", "group_tpm":
		return "tpm_limit_exceeded"
	default:
		return "rate_limit_exceeded"
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
	return normalizeSchedulerStrategyName(value)
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

func compareMoney(left string, right string) int {
	leftRat, ok := money.DecimalRat(money.NormalizeAmount(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := money.DecimalRat(money.NormalizeAmount(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Cmp(rightRat)
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
	if req.StickyAccountID == nil && strings.TrimSpace(req.SessionAffinityKey) == "" {
		// No explicit sticky header: derive a session key from the request so
		// off-the-shelf clients still get automatic cross-turn affinity.
		if key, source := deriveGatewaySessionAffinity(r, canonical); key != "" {
			req.SessionAffinityKey = key
			req.SessionAffinitySource = source
			if req.StickyStrength == "" {
				req.StickyStrength = schedulercontract.StickyStrengthSoft
			}
		}
	}
	return req
}

func schedulerStrategyHint(value *string) schedulercontract.StrategyName {
	if value == nil {
		return ""
	}
	return normalizeSchedulerStrategyName(*value)
}

func normalizeSchedulerStrategyName(value string) schedulercontract.StrategyName {
	switch schedulercontract.StrategyName(strings.TrimSpace(value)) {
	case schedulercontract.StrategyBalanced:
		return schedulercontract.StrategyBalanced
	case schedulercontract.StrategyCostSaver:
		return schedulercontract.StrategyCostSaver
	case schedulercontract.StrategyLatencyFirst, "low_latency":
		return schedulercontract.StrategyLatencyFirst
	case schedulercontract.StrategyQuotaProtect:
		return schedulercontract.StrategyQuotaProtect
	case schedulercontract.StrategyStickyFirst:
		return schedulercontract.StrategyStickyFirst
	case schedulercontract.StrategyCacheAffinityFirst, "cache_affinity":
		return schedulercontract.StrategyCacheAffinityFirst
	case schedulercontract.StrategyPremiumQuality, "quality_first":
		return schedulercontract.StrategyPremiumQuality
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
		pattern := normalizeDiscoveredModelID(supported)
		if pattern == "" {
			continue
		}
		if glob.Match(pattern, target) {
			return true
		}
	}
	return false
}

// isCodexCLIReverseProxyProvider reports whether the provider proxies to the
// ChatGPT/Codex backend through the Codex CLI reverse-proxy adapter.
func isCodexCLIReverseProxyProvider(provider providercontract.Provider) bool {
	return strings.EqualFold(strings.TrimSpace(provider.AdapterType), "reverse-proxy-codex-cli")
}

// accountRoutableForModel reports whether the account may be selected to serve
// the given upstream model.
//
// For most providers this enforces the discovery-derived supported_models
// allowlist. Codex CLI accounts deliberately skip it: the ChatGPT/Codex
// model-discovery list under-reports what the /responses endpoint actually
// serves — a free account can call gpt-5.5 even though discovery never
// advertises it — so enforcing the allowlist here pre-emptively rejects
// requests the upstream would accept. Mirror the sub2api reference: forward and
// let the upstream decide. Explicit admin exclusions (accountExcludesModel) are
// applied separately and still hold.
func accountRoutableForModel(provider providercontract.Provider, metadata map[string]any, upstreamModelName string) bool {
	if isCodexCLIReverseProxyProvider(provider) {
		return true
	}
	return accountSupportsUpstreamModel(metadata, upstreamModelName)
}

func (rt *runtimeState) gatewayCandidates(ctx context.Context, modelID int, forcedProviderKey string, apiKey apikeycontract.APIKey, sourceEndpoint string) ([]schedulercontract.Candidate, error) {
	model, err := rt.models.FindByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	mappings, err := rt.models.ListMappingsByModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	mappings = activeModelMappings(mappings)
	providerIDs := providerIDsForMappings(mappings)
	accounts, err := rt.accounts.ListActiveByProviderIDs(ctx, providerIDs)
	if err != nil {
		return nil, err
	}
	groupIDsByAccount := map[int][]int{}
	if len(apiKey.GroupIDs) > 0 {
		accountIDs := make([]int, 0, len(accounts))
		for _, account := range accounts {
			accountIDs = append(accountIDs, account.ID)
		}
		groupIDsByAccount, err = rt.accounts.ListGroupIDsByAccounts(ctx, accountIDs)
		if err != nil {
			return nil, err
		}
	}
	candidates := make([]schedulercontract.Candidate, 0)
	providersByID := make(map[int]providercontract.Provider, len(providerIDs))
	forcedProviderKey = strings.TrimSpace(forcedProviderKey)
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
			effectiveMapping := accountEffectiveModelMapping(mapping, account, model.CanonicalName, sourceEndpoint)
			effectiveMapping = providerEffectiveModelMapping(provider, effectiveMapping)
			if accountExcludesModel(account.Metadata, model.CanonicalName, effectiveMapping.UpstreamModelName) {
				continue
			}
			if !accountRoutableForModel(provider, account.Metadata, effectiveMapping.UpstreamModelName) {
				continue
			}
			if len(apiKey.GroupIDs) > 0 && !intersectsInt(apiKey.GroupIDs, groupIDsByAccount[account.ID]) {
				continue
			}
			candidates = append(candidates, schedulercontract.Candidate{
				Account:               account,
				Provider:              provider,
				Mapping:               effectiveMapping,
				ModelFamily:           optionalStringValue(model.Family),
				QualityTier:           optionalStringValue(model.QualityTier),
				EffectiveCapabilities: effectiveCapabilities(model, mapping, provider, account),
				Limits:                schedulerRuntimeLimits(account.Metadata),
			})
		}
	}
	rt.fillCandidateRuntimeStates(ctx, candidates)
	return candidates, nil
}

func (rt *runtimeState) filterCandidatesByAvailableProxy(ctx context.Context, candidates []schedulercontract.Candidate) []schedulercontract.Candidate {
	if len(candidates) == 0 || rt == nil || rt.accounts == nil {
		return candidates
	}
	filtered := make([]schedulercontract.Candidate, 0, len(candidates))
	proxyAvailability := make(map[string]bool)
	for _, candidate := range candidates {
		proxyID := optionalStringValue(candidate.Account.ProxyID)
		if proxyID == "" {
			filtered = append(filtered, candidate)
			continue
		}
		available, ok := proxyAvailability[proxyID]
		if !ok {
			_, err := rt.accounts.ResolveProxyURL(ctx, candidate.Account.ProxyID)
			available = err == nil
			proxyAvailability[proxyID] = available
			if err != nil && rt.logger != nil {
				rt.logger.Debug("skipping gateway candidate with unavailable proxy", "account_id", candidate.Account.ID, "proxy_id", proxyID, "error", err)
			}
		}
		if available {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func providerEffectiveModelMapping(provider providercontract.Provider, mapping modelcontract.ModelProviderMapping) modelcontract.ModelProviderMapping {
	if isCodexCLIReverseProxyProvider(provider) {
		mapping.UpstreamModelName = provideradaptercontract.NormalizeCodexUpstreamModelName(mapping.UpstreamModelName)
	}
	return mapping
}

// fillCandidateRuntimeStates populates Candidate.RuntimeState for every
// candidate using a constant number of batched reads (latest health and quota
// snapshots, live concurrency, last-used markers) instead of four round trips
// per candidate. Candidates sharing an account share the same resolved state.
func (rt *runtimeState) fillCandidateRuntimeStates(ctx context.Context, candidates []schedulercontract.Candidate) {
	if len(candidates) == 0 {
		return
	}
	accountIDs := make([]int, 0, len(candidates))
	seen := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.Account.ID]; ok {
			continue
		}
		seen[candidate.Account.ID] = struct{}{}
		accountIDs = append(accountIDs, candidate.Account.ID)
	}
	healthByAccount, err := rt.accounts.LatestHealthSnapshotsByAccounts(ctx, accountIDs)
	if err != nil {
		healthByAccount = nil
	}
	quotasByAccount, err := rt.accounts.LatestQuotaSnapshotsByAccounts(ctx, accountIDs)
	if err != nil {
		quotasByAccount = nil
	}
	var concurrencyByAccount map[int]int
	var lastUsedByAccount map[int]int64
	if rt.scheduler != nil {
		concurrencyByAccount = rt.scheduler.AccountConcurrencyBatch(ctx, accountIDs)
		lastUsedByAccount = rt.scheduler.AccountLastUsedBatch(ctx, accountIDs)
	}
	now := time.Now().UTC()
	for i := range candidates {
		account := candidates[i].Account
		state := schedulerRuntimeState(account.Metadata)
		if latest, ok := healthByAccount[account.ID]; ok {
			healthScore := float64(latest.SuccessRate)
			state.HealthScore = &healthScore
			p95 := latest.LatencyP95MS
			state.LatencyP95MS = &p95
			state.CircuitOpen = state.CircuitOpen || strings.EqualFold(latest.CircuitState, "open")
			state.CooldownActive = state.CooldownActive || (latest.CooldownUntil != nil && latest.CooldownUntil.After(now))
		}
		if quotas := quotasByAccount[account.ID]; len(quotas) > 0 {
			if constrained, ok := mostConstrainedRealQuotaSnapshot(quotas); ok {
				remainingRatio := float64(constrained.RemainingRatio)
				state.QuotaRemainingRatio = &remainingRatio
				state.QuotaExhausted = state.QuotaExhausted || constrained.RemainingRatio <= 0
			}
			state.QuotaAutoPaused = state.QuotaAutoPaused || quotaAutoPausedByMetadata(account.Metadata, quotas, now)
		}
		// Live in-flight concurrency makes load-aware scoring (saturation penalty,
		// concurrency-full reject) reflect real traffic instead of always seeing 0,
		// so the scheduler spreads load across equally-capable accounts under
		// pressure rather than hammering one until its hard cap fails. Take the max
		// with any metadata-provided value so a manual override/floor is preserved.
		if live := concurrencyByAccount[account.ID]; live > state.CurrentConcurrency {
			state.CurrentConcurrency = live
		}
		// Overlay gateway-level circuit breaker state. The per-account breaker
		// tracks recent upstream failures with exponential backoff and trips
		// faster than the health-probe cycle, so it catches bursts of errors
		// that the periodic probe misses.
		rt.accountBreakersMu.RLock()
		if b, ok := rt.accountBreakers[account.ID]; ok {
			if b.State() == circuitbreaker.StateOpen {
				state.CircuitOpen = true
			}
		}
		rt.accountBreakersMu.RUnlock()
		// Least-recently-used marker for fair rotation across equal-scored accounts.
		if lastUsed := lastUsedByAccount[account.ID]; lastUsed > state.LastUsedUnixMs {
			state.LastUsedUnixMs = lastUsed
		}
		candidates[i].RuntimeState = state
	}
}

func providerIDsForMappings(mappings []modelcontract.ModelProviderMapping) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.ProviderID <= 0 {
			continue
		}
		if _, exists := seen[mapping.ProviderID]; exists {
			continue
		}
		seen[mapping.ProviderID] = struct{}{}
		out = append(out, mapping.ProviderID)
	}
	return out
}

const quotaAutoPauseSnapshotStaleAfter = 2 * time.Hour

func quotaAutoPausedByMetadata(metadata map[string]any, snapshots []accountcontract.AccountQuotaSnapshot, now time.Time) bool {
	if len(metadata) == 0 || len(snapshots) == 0 {
		return false
	}
	for _, snapshot := range snapshots {
		if accountcontract.IsSyntheticQuotaSnapshot(snapshot) || quotaSnapshotWindowReset(snapshot, now) || quotaSnapshotStaleForAutoPause(snapshot, now) {
			continue
		}
		threshold, ok := quotaAutoPauseThreshold(metadata, snapshot.QuotaType)
		if !ok || threshold <= 0 {
			continue
		}
		utilization := 1 - float64(snapshot.RemainingRatio)
		if utilization >= threshold {
			return true
		}
	}
	return false
}

func quotaAutoPauseThreshold(metadata map[string]any, quotaType string) (float64, bool) {
	window := quotaAutoPauseWindow(quotaType)
	if window != "" && metadataBool(metadata, "auto_pause_"+window+"_disabled") {
		return 0, false
	}
	keys := []string{}
	if window != "" {
		keys = append(keys, "auto_pause_"+window+"_threshold", "quota_auto_pause_"+window+"_threshold")
	}
	keys = append(keys, "quota_auto_pause_threshold", "auto_pause_quota_threshold")
	value := metadataOptionalFloat(metadata, keys...)
	if value == nil {
		return 0, false
	}
	return clampFloat64(*value, 0, 1), true
}

func quotaAutoPauseWindow(quotaType string) string {
	normalized := strings.ToLower(strings.TrimSpace(quotaType))
	switch {
	case strings.Contains(normalized, "5h") || strings.Contains(normalized, "five_hour"):
		return "5h"
	case strings.Contains(normalized, "7d") || strings.Contains(normalized, "seven_day"):
		return "7d"
	default:
		return ""
	}
}

func quotaSnapshotWindowReset(snapshot accountcontract.AccountQuotaSnapshot, now time.Time) bool {
	return snapshot.ResetAt != nil && !now.Before(snapshot.ResetAt.UTC())
}

func quotaSnapshotStaleForAutoPause(snapshot accountcontract.AccountQuotaSnapshot, now time.Time) bool {
	if snapshot.SnapshotAt.IsZero() {
		return false
	}
	return now.Sub(snapshot.SnapshotAt.UTC()) >= quotaAutoPauseSnapshotStaleAfter
}

func clampFloat64(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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

func parseGeminiCountTokens(path string) (string, bool) {
	raw := strings.TrimPrefix(path, "/v1beta/models/")
	if raw == path || strings.TrimSpace(raw) == "" || !strings.HasSuffix(raw, ":countTokens") {
		return "", false
	}
	model := strings.TrimSuffix(raw, ":countTokens")
	model = strings.Trim(model, "/")
	if model == "" {
		return "", false
	}
	if decoded, err := url.PathUnescape(model); err == nil {
		model = decoded
	}
	model = strings.TrimPrefix(model, "models/")
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false
	}
	return model, true
}

// applyPayloadRules resolves operator payload-transform rules for the request's
// upstream model + protocol and attaches them, so the body-builders mutate the
// marshaled upstream payload (default / override / filter) before dispatch.
func (rt *runtimeState) applyPayloadRules(ctx context.Context, req provideradaptercontract.ConversationRequest) provideradaptercontract.ConversationRequest {
	transforms := make([]provideradaptercontract.PayloadTransform, 0, 1)
	if rt.payloadRules != nil && rt.requestShaperEnabled(ctx) {
		model := strings.TrimSpace(req.Mapping.UpstreamModelName)
		if model == "" {
			model = req.Model
		}
		resolved := rt.payloadRules.Resolve(ctx, model, req.TargetProtocol)
		if len(resolved) > 0 {
			transforms = make([]provideradaptercontract.PayloadTransform, 0, len(resolved)+1)
		}
		for _, op := range resolved {
			transforms = append(transforms, provideradaptercontract.PayloadTransform{
				Action: op.Action,
				Path:   op.Path,
				Value:  op.Value,
			})
		}
	}
	// Session-id spoofing for Anthropic: pin metadata.user_id to the stable
	// per-conversation id via an override transform (codex uses prompt_cache_key,
	// injected directly in the codex body builder).
	if transform := anthropicSpoofSessionTransform(req); transform != nil {
		transforms = append(transforms, *transform)
	}
	if len(transforms) > 0 {
		req.PayloadTransforms = transforms
	}
	return req
}

func (rt *runtimeState) requestShaperEnabled(ctx context.Context) bool {
	if rt == nil || rt.adminControl == nil {
		return true
	}
	settings, err := rt.adminControl.GetAdminSettings(ctx)
	if err != nil {
		return false
	}
	return settings.Gateway.RequestShaperEnabled
}

// anthropicSpoofSessionTransform returns a metadata.user_id override transform
// when an Anthropic-targeted request carries a spoof session id, else nil.
func anthropicSpoofSessionTransform(req provideradaptercontract.ConversationRequest) *provideradaptercontract.PayloadTransform {
	spoof := strings.TrimSpace(req.SpoofSessionID)
	if spoof == "" || req.TargetProtocol != string(gatewaycontract.ProtocolAnthropicCompatible) {
		return nil
	}
	return &provideradaptercontract.PayloadTransform{Action: "override", Path: "metadata.user_id", Value: spoof}
}

func (rt *runtimeState) invokeProviderConversation(ctx context.Context, req provideradaptercontract.ConversationRequest) (provideradaptercontract.ConversationResponse, error) {
	req = rt.applyPayloadRules(ctx, req)
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.ConversationResponse{}, err
	}
	// The concurrency lease normally lasts for this call. For a streamed
	// passthrough the response body outlives the call, so ownership of the lease
	// transfers to the stream and is released by StreamBody.Close() instead.
	releaseLeases := true
	defer func() {
		if releaseLeases {
			rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
		}
	}()
	req.Credential = dispatch.credential
	if req.Stream {
		streamed, streamErr := rt.adapters.StreamConversation(ctx, req)
		if streamErr == nil {
			// Cross-protocol streaming: wrap the live upstream body in a reader that
			// transcodes it into the client's protocol on the fly, and route the
			// usage/billing parse through the upstream parser on the retained raw
			// bytes (the client-facing bytes are no longer the upstream format).
			if proto := strings.TrimSpace(streamed.TranscodeUpstreamProtocol); proto != "" && streamed.StreamBody != nil {
				if parser, ok := provideradapterservice.NewUpstreamStreamParser(proto); ok {
					if transcoder, ok := newClientStreamTranscoder(rt.gateway, req); ok {
						reader := newCrossProtocolStreamReader(streamed.StreamBody, parser, transcoder)
						if origParse := streamed.StreamParse; origParse != nil {
							streamed.StreamParse = func(_ []byte, statusCode int) (provideradaptercontract.ConversationResponse, error) {
								return origParse(reader.rawBytes(), statusCode)
							}
						}
						streamed.StreamBody = reader
					}
				}
			}
			leases := dispatch.concurrencyLeases
			streamed.StreamBody = newStreamLeaseCloser(streamed.StreamBody, func() {
				rt.releaseGatewayConcurrency(leases)
			})
			releaseLeases = false
			return streamed, nil
		}
		if !errors.Is(streamErr, provideradaptercontract.ErrStreamingUnsupported) {
			if refreshed, retried := rt.retryAfterAuthRefresh(ctx, req.Account, dispatch.credential, streamErr); retried {
				req.Credential = refreshed
				if streamed2, streamErr2 := rt.adapters.StreamConversation(ctx, req); streamErr2 == nil {
					leases := dispatch.concurrencyLeases
					streamed2.StreamBody = newStreamLeaseCloser(streamed2.StreamBody, func() {
						rt.releaseGatewayConcurrency(leases)
					})
					releaseLeases = false
					return streamed2, nil
				}
			}
			rt.applyProviderAccountProtection(ctx, req.Account, streamErr)
			return provideradaptercontract.ConversationResponse{}, streamErr
		}
		// Streaming passthrough not eligible for this request/candidate; fall
		// back to the buffered path below.
	}
	resp, err := rt.adapters.InvokeConversation(ctx, req)
	if err != nil {
		if refreshed, retried := rt.retryAfterAuthRefresh(ctx, req.Account, dispatch.credential, err); retried {
			req.Credential = refreshed
			if resp2, err2 := rt.adapters.InvokeConversation(ctx, req); err2 == nil {
				return resp2, nil
			}
		}
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ConversationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderTokenCount(ctx context.Context, req provideradaptercontract.TokenCountRequest) (provideradaptercontract.TokenCountResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.TokenCountResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeTokenCount(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.TokenCountResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderResponseInputItems(ctx context.Context, req provideradaptercontract.ResponseInputItemsRequest) (provideradaptercontract.ResponseInputItemsResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.ResponseInputItemsResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeResponseInputItems(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ResponseInputItemsResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderEmbeddings(ctx context.Context, req provideradaptercontract.EmbeddingRequest) (provideradaptercontract.EmbeddingResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.EmbeddingResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeEmbeddings(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.EmbeddingResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderImageGeneration(ctx context.Context, req provideradaptercontract.ImageGenerationRequest) (provideradaptercontract.ImageGenerationResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	releaseLeases := true
	defer func() {
		if releaseLeases {
			rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
		}
	}()
	req.Credential = dispatch.credential
	if req.Stream {
		streamed, streamErr := rt.adapters.StreamImageGeneration(ctx, req)
		if streamErr == nil {
			leases := dispatch.concurrencyLeases
			streamed.StreamBody = newStreamLeaseCloser(streamed.StreamBody, func() {
				rt.releaseGatewayConcurrency(leases)
			})
			releaseLeases = false
			return streamed, nil
		}
		if !errors.Is(streamErr, provideradaptercontract.ErrStreamingUnsupported) {
			if refreshed, retried := rt.retryAfterAuthRefresh(ctx, req.Account, dispatch.credential, streamErr); retried {
				req.Credential = refreshed
				if streamed2, streamErr2 := rt.adapters.StreamImageGeneration(ctx, req); streamErr2 == nil {
					leases := dispatch.concurrencyLeases
					streamed2.StreamBody = newStreamLeaseCloser(streamed2.StreamBody, func() {
						rt.releaseGatewayConcurrency(leases)
					})
					releaseLeases = false
					return streamed2, nil
				}
			}
			rt.applyProviderAccountProtection(ctx, req.Account, streamErr)
			return provideradaptercontract.ImageGenerationResponse{}, streamErr
		}
	}
	resp, err := rt.adapters.InvokeImageGeneration(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderImageEdit(ctx context.Context, req provideradaptercontract.ImageEditRequest) (provideradaptercontract.ImageGenerationResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeImageEdit(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderImageVariation(ctx context.Context, req provideradaptercontract.ImageVariationRequest) (provideradaptercontract.ImageGenerationResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeImageVariation(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ImageGenerationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderAudioTranscription(ctx context.Context, req provideradaptercontract.AudioTranscriptionRequest) (provideradaptercontract.AudioTranscriptionResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.AudioTranscriptionResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeAudioTranscription(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.AudioTranscriptionResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderAudioSpeech(ctx context.Context, req provideradaptercontract.AudioSpeechRequest) (provideradaptercontract.AudioSpeechResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.AudioSpeechResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeAudioSpeech(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.AudioSpeechResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderModerations(ctx context.Context, req provideradaptercontract.ModerationRequest) (provideradaptercontract.ModerationResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.ModerationResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeModerations(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.ModerationResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) invokeProviderRerank(ctx context.Context, req provideradaptercontract.RerankRequest) (provideradaptercontract.RerankResponse, error) {
	dispatch, err := rt.prepareProviderDispatch(ctx, &req.Account, req.Mapping.ModelID)
	if err != nil {
		return provideradaptercontract.RerankResponse{}, err
	}
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
	resp, err := rt.adapters.InvokeRerank(ctx, req)
	if err != nil {
		rt.applyProviderAccountProtection(ctx, req.Account, err)
		return provideradaptercontract.RerankResponse{}, err
	}
	return resp, nil
}

func (rt *runtimeState) materializeProviderProxy(ctx context.Context, account *accountcontract.ProviderAccount) error {
	if account == nil || account.ProxyID == nil || strings.TrimSpace(*account.ProxyID) == "" {
		return nil
	}
	runtimeProxyURL, err := rt.accounts.ResolveProxyURL(ctx, account.ProxyID)
	if err != nil {
		return provideradaptercontract.ProviderError{Class: "proxy_unavailable", StatusCode: http.StatusBadGateway, Message: "provider account proxy unavailable"}
	}
	account.ProxyID = runtimeProxyURL
	return nil
}

func (rt *runtimeState) forceRefreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	return rt.singleflightRefreshReverseProxyCredential(ctx, account, credential, true)
}

func (rt *runtimeState) refreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	if !accountcontract.ShouldRefreshOAuthCredential(account, credential, time.Now().UTC()) {
		return credential, false, nil
	}
	return rt.singleflightRefreshReverseProxyCredential(ctx, account, credential, false)
}

type credentialRefreshOutcome struct {
	credential map[string]any
	refreshed  bool
}

// singleflightRefreshReverseProxyCredential coalesces concurrent refreshes of
// the same account: providers rotate refresh tokens, so two parallel refreshes
// burn the same token twice and the second one invalidates the whole session
// (parking the account). Followers share the leader's result; a caller that
// arrives after a refresh just completed gets the already-rotated stored
// credential instead of replaying its stale token.
func (rt *runtimeState) singleflightRefreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, force bool) (map[string]any, bool, error) {
	result, err, _ := rt.credentialRefreshGroup.Do(strconv.Itoa(account.ID), func() (any, error) {
		if current, err := rt.accounts.DecryptCredential(ctx, account.ID); err == nil && len(current) > 0 {
			latestAccount := account
			if latest, err := rt.accounts.FindByID(ctx, account.ID); err == nil {
				latestAccount = latest
			}
			currentToken := mapString(current, "access_token")
			if force {
				if currentToken != "" && currentToken != mapString(credential, "access_token") {
					return credentialRefreshOutcome{credential: current, refreshed: true}, nil
				}
			} else if !accountcontract.ShouldRefreshOAuthCredential(latestAccount, current, time.Now().UTC()) {
				return credentialRefreshOutcome{credential: current, refreshed: true}, nil
			}
			credential = current
			account = latestAccount
		}
		refreshed, ok, err := rt.doRefreshReverseProxyCredential(ctx, account, credential)
		if err != nil {
			return nil, err
		}
		return credentialRefreshOutcome{credential: refreshed, refreshed: ok}, nil
	})
	if err != nil {
		return credential, false, err
	}
	outcome, ok := result.(credentialRefreshOutcome)
	if !ok {
		return credential, false, nil
	}
	return outcome.credential, outcome.refreshed, nil
}

func (rt *runtimeState) doRefreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	before, err := rt.accounts.FindByID(ctx, account.ID)
	if err != nil {
		rt.logger.Warn("failed to load provider account before refresh", "error", err, "account_id", account.ID)
		return credential, false, err
	}
	// Route through accounts.RefreshAccessTokenWithOutcome so the inline
	// 401-driven OAuth refresh path actually updates refresh_attempts,
	// refresh_last_error and needs_reauth_at — those fields shipped in
	// Item A (commit 3a8c4b34) but the inline path was calling
	// rt.reverseProxy.Refresh + rt.accounts.Update directly, bypassing
	// all of them. That's why production Codex accounts could sit in
	// needs_reauth with refresh_attempts permanently 0 — the worker
	// ran fine but every real 401 just silently failed the inline
	// retry without bookkeeping. Uses the same adapter the operator-
	// initiated /admin/accounts/{id}/refresh handler uses, so the two
	// paths produce identical state transitions.
	adapter := adminAccountRefresherAdapter{refresher: rt.reverseProxy, accounts: rt.accounts}
	outcome, refreshErr := rt.accounts.RefreshAccessTokenWithOutcome(ctx, account.ID, adapter)
	if refreshErr != nil {
		rt.recordAudit(ctx, auditcontract.RecordRequest{
			Action:       "provider_account.oauth_refresh_failed",
			ResourceType: "provider_account",
			ResourceID:   strconv.Itoa(account.ID),
			Before:       accountAuditSnapshot(before),
			After: map[string]any{
				"refresh_status":       "failed",
				"error_class":          errorClassName(refreshErr),
				"outcome_class":        string(outcome.Class),
				"refresh_attempts":     outcome.Attempts,
				"needs_reauth_flipped": outcome.NeedsReauthFlipped,
			},
			TraceID: traceIDFromContext(ctx),
		})
		// A permanently rejected refresh token (session_invalid) parks the
		// account for re-auth via Status so the scheduler stops selecting
		// it. RefreshAccessTokenWithOutcome already sets the newer
		// needs_reauth_at timestamp on permanent or threshold-exceeded
		// outcomes; we keep BOTH so existing UI / scheduler code reading
		// Status keeps working without a coordinated migration.
		rt.protectProviderAccountForClass(ctx, account, errorClassName(refreshErr))
		return credential, false, refreshErr
	}
	// Fetch the refreshed credential — RefreshAccessTokenWithOutcome
	// persisted it inside the per-account mutex, so the read here gets
	// the post-rotation value the inline retry needs.
	refreshed, fetchErr := rt.accounts.DecryptCredential(ctx, account.ID)
	if fetchErr != nil {
		rt.logger.Warn("failed to read refreshed provider credential", "error", fetchErr, "account_id", account.ID)
		return credential, false, fetchErr
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.oauth_refresh",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(account.ID),
		Before:       accountAuditSnapshot(before),
		After: map[string]any{
			"refresh_status":     "success",
			"outcome_class":      string(outcome.Class),
			"refresh_attempts":   outcome.Attempts,
			"credential_version": outcome.Account.CredentialVersion,
		},
		TraceID: traceIDFromContext(ctx),
	})
	return refreshed, true, nil
}

// retryAfterAuthRefresh attempts an OAuth token refresh when the upstream
// rejects the current credential with a session/auth error. Returns the
// refreshed credential and true if a refresh succeeded; the caller should
// retry the request once. Returns (nil, false) for non-OAuth accounts or
// non-auth error classes.
func (rt *runtimeState) retryAfterAuthRefresh(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any, upstreamErr error) (map[string]any, bool) {
	if account.RuntimeClass != accountcontract.RuntimeClassOauthRefresh && account.RuntimeClass != accountcontract.RuntimeClassOauthDeviceCode {
		return nil, false
	}
	class := errorClassName(upstreamErr)
	if class != "session_invalid" && class != "auth_failed" && class != "auth_error" {
		return nil, false
	}
	if mapString(credential, "refresh_token") == "" {
		return nil, false
	}
	rt.logger.Info("attempting OAuth refresh after upstream auth rejection", "account_id", account.ID, "error_class", class)
	refreshed, ok, err := rt.forceRefreshReverseProxyCredential(ctx, account, credential)
	if err != nil || !ok {
		return nil, false
	}
	return refreshed, true
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
	hasProviderErr := errors.As(err, &providerErr)
	// Layer the structured failover classifier on top of the existing
	// class-based protection. The decision is observation-only here — actual
	// failover routing is owned by gatewayShouldFailover further up the stack,
	// and blacklist transitions stay driven by protectProviderAccountForClass.
	// Surfacing it lets operators audit the upstream-error policy without
	// changing existing protection semantics.
	rt.logUpstreamFailoverDecision(ctx, account, hasProviderErr, providerErr, err)
	if !hasProviderErr {
		return
	}
	if !gatewayAccountFailureStatusHandled(account.Metadata, &providerErr.StatusCode) {
		return
	}
	rt.protectProviderAccountForClass(ctx, account, providerErr.Class)
}

// logUpstreamFailoverDecision records the structured ClassifyUpstreamError
// verdict so the existing protection path and the new directive-shaped failover
// decision stay observable side-by-side. No state is mutated.
func (rt *runtimeState) logUpstreamFailoverDecision(ctx context.Context, account accountcontract.ProviderAccount, hasProviderErr bool, providerErr provideradaptercontract.ProviderError, err error) {
	if rt == nil || rt.logger == nil {
		return
	}
	statusCode := 0
	if hasProviderErr {
		statusCode = providerErr.StatusCode
	}
	decision := ClassifyUpstreamError(statusCode, nil, err)
	rt.logger.Debug("upstream failover classification",
		"account_id", account.ID,
		"status_code", statusCode,
		"class", decision.Class,
		"should_failover", decision.ShouldFailover,
		"should_blacklist", decision.ShouldBlacklist,
		"retry_after_ms", decision.RetryAfterMs,
		"trace_id", traceIDFromContext(ctx),
	)
}

// protectProviderAccountForClass transitions a non-api_key account to the
// protective status implied by an upstream failure class (e.g. session_invalid
// -> needs_reauth) and records an auto_protect audit. It is a no-op when the
// class maps to no protective status or the account already holds it. Shared by
// adapter-invoke failures and serve-time OAuth refresh failures so a permanently
// rejected refresh token parks the account instead of being retried forever.
func (rt *runtimeState) protectProviderAccountForClass(ctx context.Context, account accountcontract.ProviderAccount, class string) {
	nextStatus, ok := reverseProxyAccountFailureStatus(class)
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
		TraceID:      traceIDFromContext(ctx),
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

func gatewayContentBlocksFromProvider(parts []provideradaptercontract.ContentPart) []gatewaycontract.ContentBlock {
	if len(parts) == 0 {
		return nil
	}
	out := make([]gatewaycontract.ContentBlock, 0, len(parts))
	for _, part := range parts {
		block := gatewaycontract.ContentBlock{
			Type:              gatewayContentBlockTypeFromProvider(part.Kind),
			Role:              "assistant",
			Text:              strings.TrimSpace(part.Text),
			MediaURL:          strings.TrimSpace(part.MediaURL),
			MediaBase64:       strings.TrimSpace(part.MediaBase64),
			MIMEType:          strings.TrimSpace(part.MIMEType),
			FileID:            strings.TrimSpace(part.FileID),
			ToolCallID:        strings.TrimSpace(part.ToolCallID),
			ToolName:          strings.TrimSpace(part.ToolName),
			ToolArgumentsJSON: strings.TrimSpace(part.ToolArgumentsJSON),
			ToolResultForID:   strings.TrimSpace(part.ToolResultForID),
			ToolResultIsError: part.ToolResultIsError,
			Metadata:          cloneAnyMap(part.Metadata),
			Raw:               append([]byte(nil), part.Raw...),
			OriginProtocol:    strings.TrimSpace(part.OriginProtocol),
		}
		out = append(out, block)
	}
	return out
}

func gatewayStreamEventsFromProvider(events []provideradaptercontract.ConversationStreamEvent) []gatewaycontract.StreamEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]gatewaycontract.StreamEvent, 0, len(events))
	for _, event := range events {
		out = append(out, gatewaycontract.StreamEvent{
			Index:          event.Index,
			Type:           gatewayStreamEventTypeFromProvider(event.Type),
			ContentIndex:   event.ContentIndex,
			Delta:          gatewayStreamDeltaFromProvider(event.Delta),
			Usage:          gatewayUsageFromProviderUsage(event.Usage),
			StopReason:     gatewayStopReasonFromProvider(event.StopReason),
			RawEventType:   strings.TrimSpace(event.RawEventType),
			Raw:            append([]byte(nil), event.Raw...),
			OriginProtocol: strings.TrimSpace(event.OriginProtocol),
			Metadata:       cloneAnyMap(event.Metadata),
		})
	}
	return out
}

func gatewayStreamDeltaFromProvider(part provideradaptercontract.ContentPart) gatewaycontract.ContentBlock {
	return gatewaycontract.ContentBlock{
		Type:              gatewayContentBlockTypeFromProvider(part.Kind),
		Role:              "assistant",
		Text:              part.Text,
		MediaURL:          strings.TrimSpace(part.MediaURL),
		MediaBase64:       strings.TrimSpace(part.MediaBase64),
		MIMEType:          strings.TrimSpace(part.MIMEType),
		FileID:            strings.TrimSpace(part.FileID),
		ToolCallID:        strings.TrimSpace(part.ToolCallID),
		ToolName:          strings.TrimSpace(part.ToolName),
		ToolArgumentsJSON: part.ToolArgumentsJSON,
		ToolResultForID:   strings.TrimSpace(part.ToolResultForID),
		ToolResultIsError: part.ToolResultIsError,
		Metadata:          cloneAnyMap(part.Metadata),
		Raw:               append([]byte(nil), part.Raw...),
		OriginProtocol:    strings.TrimSpace(part.OriginProtocol),
	}
}

func gatewayStreamEventTypeFromProvider(eventType provideradaptercontract.ConversationStreamEventType) gatewaycontract.StreamEventType {
	switch eventType {
	case provideradaptercontract.ConversationStreamEventToolCallDelta:
		return gatewaycontract.StreamEventToolCallDelta
	case provideradaptercontract.ConversationStreamEventToolResult:
		return gatewaycontract.StreamEventToolResult
	case provideradaptercontract.ConversationStreamEventReasoning:
		return gatewaycontract.StreamEventReasoning
	case provideradaptercontract.ConversationStreamEventMetadata:
		return gatewaycontract.StreamEventMetadata
	case provideradaptercontract.ConversationStreamEventUsage:
		return gatewaycontract.StreamEventUsage
	case provideradaptercontract.ConversationStreamEventStop:
		return gatewaycontract.StreamEventStop
	default:
		return gatewaycontract.StreamEventContentDelta
	}
}

func gatewayContentBlockTypeFromProvider(kind provideradaptercontract.ContentPartKind) gatewaycontract.ContentBlockType {
	switch kind {
	case provideradaptercontract.ContentPartImage:
		return gatewaycontract.ContentBlockImage
	case provideradaptercontract.ContentPartAudio:
		return gatewaycontract.ContentBlockAudio
	case provideradaptercontract.ContentPartFile:
		return gatewaycontract.ContentBlockFile
	case provideradaptercontract.ContentPartToolUse:
		return gatewaycontract.ContentBlockToolCall
	case provideradaptercontract.ContentPartToolResult:
		return gatewaycontract.ContentBlockToolResult
	case provideradaptercontract.ContentPartThinking:
		return gatewaycontract.ContentBlockReasoning
	case provideradaptercontract.ContentPartRefusal:
		return gatewaycontract.ContentBlockRefusal
	case provideradaptercontract.ContentPartMetadata:
		return gatewaycontract.ContentBlockMetadata
	default:
		return gatewaycontract.ContentBlockText
	}
}

func gatewayStopReasonFromProvider(reason provideradaptercontract.StopReason) string {
	switch reason {
	case provideradaptercontract.StopReasonMaxTokens:
		return "max_tokens"
	case provideradaptercontract.StopReasonToolUse:
		return "tool_use"
	case provideradaptercontract.StopReasonContentFilter:
		return "content_filter"
	case provideradaptercontract.StopReasonRefusal:
		return "refusal"
	default:
		return "end_turn"
	}
}

func gatewayUsageFromProvider(resp provideradaptercontract.ConversationResponse) gatewaycontract.Usage {
	return gatewayUsageFromProviderUsage(resp.Usage)
}

func gatewayUsageFromProviderUsage(usage provideradaptercontract.Usage) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		ImageOutputTokens:     usage.ImageOutputTokens,
		CachedTokens:          usage.CachedTokens,
		CacheCreationTokens:   usage.CacheCreationTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
		Observed:              usage.Observed,
		Estimated:             usage.Estimated,
	}
}

func gatewayUsageFromEmbeddingProvider(resp provideradaptercontract.EmbeddingResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Observed:     resp.Usage.Observed,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromImageProvider(resp provideradaptercontract.ImageGenerationResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:       resp.Usage.InputTokens,
		OutputTokens:      resp.Usage.OutputTokens,
		ImageOutputTokens: resp.Usage.ImageOutputTokens,
		CachedTokens:      resp.Usage.CachedTokens,
		Observed:          resp.Usage.Observed,
		Estimated:         resp.Usage.Estimated,
	}
}

func gatewayUsageFromAudioTranscriptionProvider(resp provideradaptercontract.AudioTranscriptionResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Observed:     resp.Usage.Observed,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromAudioSpeechProvider(resp provideradaptercontract.AudioSpeechResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Observed:     resp.Usage.Observed,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromModerationProvider(resp provideradaptercontract.ModerationResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Observed:     resp.Usage.Observed,
		Estimated:    resp.Usage.Estimated,
	}
}

func gatewayUsageFromRerankProvider(resp provideradaptercontract.RerankResponse) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CachedTokens,
		Observed:     resp.Usage.Observed,
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

func providerImageInputs(values []gatewaycontract.ImageInput) []provideradaptercontract.ImageInput {
	if values == nil {
		return nil
	}
	out := make([]provideradaptercontract.ImageInput, len(values))
	for idx, value := range values {
		out[idx] = provideradaptercontract.ImageInput{
			FileName:    value.FileName,
			ContentType: value.ContentType,
			Bytes:       append([]byte(nil), value.Bytes...),
		}
	}
	return out
}

func providerImageInputPtr(value *gatewaycontract.ImageInput) *provideradaptercontract.ImageInput {
	if value == nil {
		return nil
	}
	return &provideradaptercontract.ImageInput{
		FileName:    value.FileName,
		ContentType: value.ContentType,
		Bytes:       append([]byte(nil), value.Bytes...),
	}
}

func providerImageInputValue(values []gatewaycontract.ImageInput) provideradaptercontract.ImageInput {
	if len(values) == 0 {
		return provideradaptercontract.ImageInput{}
	}
	value := values[0]
	return provideradaptercontract.ImageInput{
		FileName:    value.FileName,
		ContentType: value.ContentType,
		Bytes:       append([]byte(nil), value.Bytes...),
	}
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
	case "rate_limit", "quota_exhausted", "rpm_limit_exceeded", "tpm_limit_exceeded", "concurrency_limit_exceeded", "platform_quota_exceeded":
		return apiopenapi.RateLimitError
	case "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized":
		return apiopenapi.PermissionError
	case "timeout", "network_error", "configuration_error", "proxy_unavailable", "stream_interrupted", "no_available_account", "overloaded":
		return apiopenapi.ServiceUnavailableError
	default:
		return apiopenapi.UpstreamError
	}
}

func providerGatewayHTTPStatus(upstreamStatus int) int {
	switch upstreamStatus {
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests
	case http.StatusPaymentRequired:
		// Per-user platform spend-quota denials surface as 402 to the caller.
		return http.StatusPaymentRequired
	case 529:
		return http.StatusServiceUnavailable
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
	case "quota_exhausted":
		return "provider quota exhausted"
	case "rpm_limit_exceeded":
		return "provider account RPM limit exceeded"
	case "tpm_limit_exceeded":
		return "provider account TPM limit exceeded"
	case "concurrency_limit_exceeded":
		return "provider account concurrency limit exceeded"
	case "platform_quota_exceeded":
		return "platform spend quota exceeded"
	case "auth_failed", "auth_error", "credential_error":
		return "provider authentication failed"
	case "configuration_error":
		return "provider configuration unavailable"
	case "proxy_unavailable":
		return "provider proxy unavailable"
	case "invalid_request":
		return "provider rejected request"
	case "model_unavailable":
		return "provider model unavailable"
	case "provider_5xx":
		return "provider service error"
	case "overloaded":
		return "provider overloaded"
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
	case "empty_completion":
		return "provider returned empty completion"
	default:
		return "provider request failed"
	}
}
