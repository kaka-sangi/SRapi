package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	contentsafetycontract "github.com/srapi/srapi/apps/api/internal/modules/content_safety/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
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
	CompatibilityWarnings []string
	ProviderQuotaSignals  []provideradaptercontract.QuotaSignal
	ProviderRetryAfter    *time.Time
	ProviderErrorMessage  string
	QualityPrompt         string
	QualityOutput         string
	FeedbackID            int
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
	PricingSource    string
	PricingEstimated bool
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
	// Drop accounts restricted to official client signatures the caller doesn't
	// match (e.g. an OAuth account marked codex-only), so a generic client can't
	// drive an account that would get banned for it.
	candidates = filterCandidatesByAllowedClients(candidates, gatewayInboundClientFromContext(ctx))
	// Drop accounts already at their per-account active-session cap (max_sessions),
	// excluding this conversation so it is never evicted from its own account.
	candidates = rt.filterCandidatesBySessionLimit(ctx, candidates, req.SessionAffinityKey)
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
		return result, err
	}
	return applyAccountModelMapping(result, req.Model), nil
}

// accountModelMappingMetadataKey is the provider-account metadata key holding a
// per-account "canonical catalog model -> upstream model name" override map. It
// lets two accounts of the same provider serve the same catalog model from
// different upstream model names (sub2api per-channel model_mapping parity).
const accountModelMappingMetadataKey = "model_mapping"

// applyAccountModelMapping overrides the scheduled candidate's upstream model
// name when its account carries a per-account model_mapping override for the
// requested canonical model. The failover loop re-schedules on every attempt,
// so applying to result.Candidate here covers each attempt's chosen account.
func applyAccountModelMapping(result schedulercontract.ScheduleResult, canonicalModel string) schedulercontract.ScheduleResult {
	if override := accountModelOverride(result.Candidate.Account, canonicalModel); override != "" {
		result.Candidate.Mapping.UpstreamModelName = override
	}
	return result
}

// accountModelOverride returns the per-account upstream model name configured
// for canonicalModel, or "" when the account has no (valid, non-blank) override.
func accountModelOverride(account accountcontract.ProviderAccount, canonicalModel string) string {
	model := strings.TrimSpace(canonicalModel)
	if model == "" || len(account.Metadata) == 0 {
		return ""
	}
	mapping, ok := account.Metadata[accountModelMappingMetadataKey].(map[string]any)
	if !ok {
		return ""
	}
	override, _ := mapping[model].(string)
	return strings.TrimSpace(override)
}

func (rt *runtimeState) prepareGatewayAdmission(ctx context.Context, canonical *gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution, modelID int) (gatewayAdmission, error) {
	return rt.prepareGatewayAdmissionWithOptions(ctx, canonical, resolution, modelID, true)
}

func (rt *runtimeState) prepareGatewayAdmissionWithoutContentSafety(ctx context.Context, canonical *gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution, modelID int) (gatewayAdmission, error) {
	return rt.prepareGatewayAdmissionWithOptions(ctx, canonical, resolution, modelID, false)
}

func (rt *runtimeState) prepareGatewayAdmissionWithOptions(ctx context.Context, canonical *gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution, modelID int, applyContentSafety bool) (gatewayAdmission, error) {
	if canonical == nil {
		return gatewayAdmission{}, errors.New("canonical gateway request is nil")
	}
	if applyContentSafety {
		*canonical = rt.applyGatewayContentSafety(ctx, *canonical)
	}
	estimatedUsage := estimateGatewayRequestUsage(*canonical)
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
		ModelReferences:    gatewayModelReferences(*canonical, resolution),
		EstimatedTokens:    estimatedUsage.InputTokens + estimatedUsage.OutputTokens + estimatedUsage.CachedTokens,
		EstimatedCost:      pricing.Amount,
		TokensUsedInPeriod: tokensUsed,
		CostUsedInPeriod:   costUsed,
		RequestTime:        time.Now().UTC(),
	})
	if err != nil {
		return gatewayAdmission{}, err
	}
	admission := gatewayAdmission{EstimatedUsage: estimatedUsage, Pricing: pricing, Entitlement: entitlement}
	if !entitlement.Allowed {
		return admission, nil
	}
	denied, err := rt.gatewayBalanceGate(ctx, canonical.UserID, entitlement, pricing)
	if err != nil {
		return gatewayAdmission{}, err
	}
	if denied {
		admission.Entitlement.Allowed = false
		admission.Entitlement.Reason = "insufficient_balance"
		return admission, nil
	}
	rateLimit, err := rt.checkGatewayRateLimit(ctx, *canonical, estimatedUsage, modelID)
	if err != nil {
		return gatewayAdmission{}, err
	}
	admission.RateLimit = rateLimit
	if !rateLimit.Allowed {
		admission.Entitlement.Allowed = false
		admission.Entitlement.Reason = gatewayRateLimitReason(rateLimit.Name)
	}
	return admission, nil
}

func (rt *runtimeState) applyGatewayContentSafety(ctx context.Context, canonical gatewaycontract.CanonicalRequest) gatewaycontract.CanonicalRequest {
	if rt.contentSafety == nil {
		return canonical
	}
	updated, result := rt.contentSafety.Apply(canonical)
	if result.Changed {
		updated.RawBody = nil
	}
	if len(result.Findings) == 0 {
		return updated
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		ActorUserID:  ptrInt(canonical.UserID),
		Action:       "gateway.content_safety",
		ResourceType: "gateway_request",
		ResourceID:   canonical.RequestID,
		After: map[string]any{
			"request_id":      canonical.RequestID,
			"source_endpoint": canonical.SourceEndpoint,
			"model":           canonical.CanonicalModel,
			"changed":         result.Changed,
			"warnings":        nonNilStrings(result.Warnings),
			"findings":        contentSafetyFindingsAudit(result.Findings),
		},
		TraceID: requestIDFromContext(ctx),
	})
	return updated
}

func contentSafetyFindingsAudit(findings []contentsafetycontract.Finding) []map[string]any {
	out := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		out = append(out, map[string]any{
			"kind":     string(finding.Kind),
			"severity": string(finding.Severity),
			"count":    finding.Count,
			"redacted": finding.Redacted,
		})
	}
	return out
}

func (rt *runtimeState) checkGatewayRateLimit(ctx context.Context, canonical gatewaycontract.CanonicalRequest, usage gatewaycontract.Usage, modelID int) (ratelimit.Decision, error) {
	if rt.rateLimiter == nil || canonical.UserID <= 0 || canonical.APIKeyID <= 0 {
		return ratelimit.Decision{Allowed: true}, nil
	}
	apiKey, err := rt.apiKeyByID(ctx, canonical.UserID, canonical.APIKeyID)
	if err != nil {
		return ratelimit.Decision{}, err
	}
	user, err := rt.users.FindByID(ctx, canonical.UserID)
	if err != nil {
		return ratelimit.Decision{}, err
	}

	checks := make([]ratelimit.Check, 0, 3)
	if limit := positiveLimit(apiKey.RPMLimit); limit > 0 {
		checks = append(checks, ratelimit.Check{
			Name:   "rpm",
			Key:    fmt.Sprintf("apikey:%d:rpm", apiKey.ID),
			Limit:  limit,
			Cost:   1,
			Window: time.Minute,
		})
	}
	// Per-key multi-window request ceilings (5h / 1d / 7d).
	checks = append(checks, gatewayAPIKeyWindowChecks(apiKey)...)
	if limit := positiveLimit(user.RPMLimit); limit > 0 {
		checks = append(checks, ratelimit.Check{
			Name:   "rpm",
			Key:    fmt.Sprintf("user:%d:rpm", user.ID),
			Limit:  limit,
			Cost:   1,
			Window: time.Minute,
		})
	}
	if limit := positiveLimit(apiKey.TPMLimit); limit > 0 {
		checks = append(checks, ratelimit.Check{
			Name:   "tpm",
			Key:    fmt.Sprintf("apikey:%d:tpm", apiKey.ID),
			Limit:  limit,
			Cost:   max(1, usage.InputTokens+usage.OutputTokens+usage.CachedTokens),
			Window: time.Minute,
		})
	}
	// Global per-model RPM/TPM ceilings (WP-1190/1260): protect an upstream model
	// from overload across all users, on top of the per-key / per-user limits.
	if rt.modelRateLimits != nil && modelID > 0 {
		if limit := rt.modelRateLimits.RPMForModel(ctx, modelID); limit > 0 {
			checks = append(checks, ratelimit.Check{
				Name:   "model_rpm",
				Key:    fmt.Sprintf("model:%d:rpm", modelID),
				Limit:  limit,
				Cost:   1,
				Window: time.Minute,
			})
		}
		if limit := rt.modelRateLimits.TPMForModel(ctx, modelID); limit > 0 {
			checks = append(checks, ratelimit.Check{
				Name:   "model_tpm",
				Key:    fmt.Sprintf("model:%d:tpm", modelID),
				Limit:  limit,
				Cost:   max(1, usage.InputTokens+usage.OutputTokens+usage.CachedTokens),
				Window: time.Minute,
			})
		}
	}
	return rt.rateLimiter.Allow(ctx, checks, time.Now().UTC())
}

func (rt *runtimeState) reserveGatewayAccountQuota(ctx context.Context, usage gatewaycontract.Usage, candidate schedulercontract.Candidate) error {
	if rt.rateLimiter == nil || candidate.Account.ID <= 0 {
		return nil
	}
	checks := make([]ratelimit.Check, 0, 2)
	if limit := positiveLimit(candidate.Limits.RPMLimit); limit > 0 {
		checks = append(checks, ratelimit.Check{
			Name:   "account_rpm",
			Key:    fmt.Sprintf("account:%d:rpm", candidate.Account.ID),
			Limit:  limit,
			Cost:   1,
			Window: accountRuntimeQuotaWindow(candidate.Account.Metadata),
		})
	}
	if limit := positiveLimit(candidate.Limits.TPMLimit); limit > 0 {
		checks = append(checks, ratelimit.Check{
			Name:   "account_tpm",
			Key:    fmt.Sprintf("account:%d:tpm", candidate.Account.ID),
			Limit:  limit,
			Cost:   max(1, usage.InputTokens+usage.OutputTokens+usage.CachedTokens),
			Window: accountRuntimeQuotaWindow(candidate.Account.Metadata),
		})
	}
	// Per-account-group RPM capacity (WP-1200): the selected account may belong to
	// groups with a requests-per-minute ceiling; exceeding any one triggers the
	// same 429-class failover as the per-account limits.
	if rt.groupRateLimits != nil {
		if groupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, candidate.Account.ID); err == nil {
			for _, groupID := range groupIDs {
				if limit := rt.groupRateLimits.RPMForGroup(ctx, groupID); limit > 0 {
					checks = append(checks, ratelimit.Check{
						Name:   "group_rpm",
						Key:    fmt.Sprintf("group:%d:rpm", groupID),
						Limit:  limit,
						Cost:   1,
						Window: time.Minute,
					})
				}
				if limit := rt.groupRateLimits.TPMForGroup(ctx, groupID); limit > 0 {
					checks = append(checks, ratelimit.Check{
						Name:   "group_tpm",
						Key:    fmt.Sprintf("group:%d:tpm", groupID),
						Limit:  limit,
						Cost:   max(1, usage.InputTokens+usage.OutputTokens+usage.CachedTokens),
						Window: time.Minute,
					})
				}
			}
		}
	}
	if len(checks) == 0 {
		return nil
	}
	decision, err := rt.rateLimiter.Allow(ctx, checks, time.Now().UTC())
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	return provideradaptercontract.ProviderError{
		Class:      gatewayAccountQuotaErrorClass(decision.Name),
		StatusCode: http.StatusTooManyRequests,
		Message:    "provider account rate limit exceeded",
	}
}

func (rt *runtimeState) acquireProviderAccountConcurrency(ctx context.Context, account accountcontract.ProviderAccount) (ratelimit.ConcurrencyLease, error) {
	if rt.rateLimiter == nil || account.ID <= 0 {
		return ratelimit.ConcurrencyLease{}, nil
	}
	limit := positiveLimit(metadataOptionalInt(account.Metadata, "max_concurrency"))
	if limit <= 0 {
		return ratelimit.ConcurrencyLease{}, nil
	}
	lease, decision, err := rt.rateLimiter.AcquireConcurrency(ctx, ratelimit.ConcurrencyCheck{
		Name:  "account_concurrency",
		Key:   fmt.Sprintf("account:%d:concurrency", account.ID),
		Limit: limit,
		TTL:   rt.providerAccountConcurrencyTTL(),
	}, time.Now().UTC())
	if err != nil {
		return ratelimit.ConcurrencyLease{}, err
	}
	if !decision.Allowed {
		return ratelimit.ConcurrencyLease{}, provideradaptercontract.ProviderError{
			Class:      "concurrency_limit_exceeded",
			StatusCode: http.StatusTooManyRequests,
			Message:    "provider account concurrency limit exceeded",
		}
	}
	return lease, nil
}

func (rt *runtimeState) releaseProviderAccountConcurrency(lease ratelimit.ConcurrencyLease) {
	if rt == nil || rt.rateLimiter == nil || strings.TrimSpace(lease.Key) == "" || strings.TrimSpace(lease.Token) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rt.rateLimiter.ReleaseConcurrency(ctx, lease); err != nil {
		rt.logger.Warn("failed to release provider account concurrency slot", "error", err, "lease_key", lease.Key)
	}
}

func (rt *runtimeState) providerAccountConcurrencyTTL() time.Duration {
	if rt == nil || rt.cfg.Gateway.RequestTimeout <= 0 {
		return time.Minute
	}
	return rt.cfg.Gateway.RequestTimeout
}

func (rt *runtimeState) prepareProviderDispatch(ctx context.Context, account *accountcontract.ProviderAccount, modelID int) (providerDispatchState, error) {
	if account == nil || account.ID <= 0 {
		return providerDispatchState{}, provideradaptercontract.ProviderError{Class: "no_available_account", StatusCode: http.StatusServiceUnavailable, Message: "provider account missing"}
	}
	if err := rt.materializeProviderProxy(ctx, account); err != nil {
		return providerDispatchState{}, err
	}
	accountLease, err := rt.acquireProviderAccountConcurrency(ctx, *account)
	if err != nil {
		return providerDispatchState{}, err
	}
	leases := []ratelimit.ConcurrencyLease{accountLease}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			rt.releaseGatewayConcurrency(leases)
		}
	}()
	modelLease, err := rt.acquireModelConcurrency(ctx, modelID)
	if err != nil {
		return providerDispatchState{}, err
	}
	leases = append(leases, modelLease)
	groupLeases, err := rt.acquireAccountGroupConcurrency(ctx, *account)
	if err != nil {
		return providerDispatchState{}, err
	}
	leases = append(leases, groupLeases...)
	credential, err := rt.accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		return providerDispatchState{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential unavailable"}
	}
	if refreshed, ok, err := rt.refreshReverseProxyCredential(ctx, *account, credential); err != nil {
		return providerDispatchState{}, provideradaptercontract.ProviderError{Class: "auth_failed", StatusCode: http.StatusBadGateway, Message: "provider credential refresh failed"}
	} else if ok {
		credential = refreshed
	}
	releaseOnError = false
	return providerDispatchState{credential: credential, concurrencyLeases: leases}, nil
}

func gatewayAccountQuotaErrorClass(name string) string {
	switch strings.TrimSpace(name) {
	case "account_rpm":
		return "rpm_limit_exceeded"
	case "account_tpm":
		return "tpm_limit_exceeded"
	default:
		return "rate_limit"
	}
}

func (rt *runtimeState) apiKeyByID(ctx context.Context, userID int, apiKeyID int) (apikeycontract.APIKey, error) {
	keys, err := rt.apiKeys.ListByUser(ctx, userID)
	if err != nil {
		return apikeycontract.APIKey{}, err
	}
	for _, key := range keys {
		if key.ID == apiKeyID {
			return key, nil
		}
	}
	return apikeycontract.APIKey{}, apikeycontract.ErrKeyNotFound
}

func positiveLimit(value *int) int {
	if value == nil || *value <= 0 {
		return 0
	}
	return *value
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
		ModelID:          modelID,
		ProviderID:       candidate.Provider.ID,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CachedTokens,
		CacheWriteTokens: usage.CacheCreationTokens,
		At:               time.Now().UTC(),
		PricingOverride:  cloneAnyMap(candidate.Mapping.PricingOverride),
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

// gatewayUserPlatformSpend sums the user's successful spend on a single platform
// (one provider) across the daily / weekly / monthly windows, reusing the same
// ListByUser scan the entitlement period-usage check uses. Day and month windows
// are UTC-calendar-aligned (month start matches the monthly cost entitlement);
// the week window is a rolling trailing 7 days.
func (rt *runtimeState) gatewayUserPlatformSpend(ctx context.Context, userID, providerID int, now time.Time) (string, string, string) {
	daily, weekly, monthly := "0.00000000", "0.00000000", "0.00000000"
	if rt.usage == nil || userID <= 0 || providerID <= 0 {
		return daily, weekly, monthly
	}
	logs, err := rt.usage.ListByUser(ctx, userID)
	if err != nil {
		return daily, weekly, monthly
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := now.Add(-7 * 24 * time.Hour)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, log := range logs {
		if !log.Success || log.ProviderID == nil || *log.ProviderID != providerID {
			continue
		}
		created := log.CreatedAt.UTC()
		if !created.Before(monthStart) {
			monthly = addDecimalMoney(monthly, log.Cost)
		}
		if !created.Before(weekStart) {
			weekly = addDecimalMoney(weekly, log.Cost)
		}
		if !created.Before(dayStart) {
			daily = addDecimalMoney(daily, log.Cost)
		}
	}
	return daily, weekly, monthly
}

// effectivePlatformLimits resolves the daily/weekly/monthly USD caps for a user
// on a platform: an enabled per-user UserPlatformQuota override wins; otherwise
// the subscription plan default carried in entitlements applies. ok is false
// when no cap is configured.
func (rt *runtimeState) effectivePlatformLimits(ctx context.Context, userID int, platform string, admission gatewayAdmission) (daily, weekly, monthly *string, ok bool) {
	if rt.userPlatformQuotas != nil {
		if quota, found := rt.userPlatformQuotas.EffectiveQuota(ctx, userID, platform); found {
			return quota.DailyLimit, quota.WeeklyLimit, quota.MonthlyLimit, true
		}
	}
	return platformDefaultsFromEntitlements(admission.Entitlement.Entitlements, platform)
}

// platformDefaultsFromEntitlements reads plan-level platform caps from the
// entitlements map, shaped as: platform_spend_quotas: { "<platform>": { daily,
// weekly, monthly } } (each value a decimal USD string).
func platformDefaultsFromEntitlements(entitlements map[string]any, platform string) (*string, *string, *string, bool) {
	raw, ok := entitlements["platform_spend_quotas"]
	if !ok {
		return nil, nil, nil, false
	}
	byPlatform, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, nil, false
	}
	entry, ok := byPlatform[platform].(map[string]any)
	if !ok {
		return nil, nil, nil, false
	}
	daily := optionalMoneyField(entry, "daily")
	weekly := optionalMoneyField(entry, "weekly")
	monthly := optionalMoneyField(entry, "monthly")
	if daily == nil && weekly == nil && monthly == nil {
		return nil, nil, nil, false
	}
	return daily, weekly, monthly, true
}

func optionalMoneyField(fields map[string]any, key string) *string {
	value, ok := fields[key].(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// enforceUserPlatformQuota hard-denies a request when the user's accumulated
// spend on the scheduled platform (plus this request's estimate) would exceed
// any configured window cap. Returns a 402 platform_quota_exceeded ProviderError
// — a non-failover class — so the request is blocked rather than rerouted.
func (rt *runtimeState) enforceUserPlatformQuota(ctx context.Context, userID, providerID int, platform string, admission gatewayAdmission) error {
	platform = strings.TrimSpace(platform)
	if rt.userPlatformQuotas == nil || userID <= 0 || providerID <= 0 || platform == "" {
		return nil
	}
	daily, weekly, monthly, ok := rt.effectivePlatformLimits(ctx, userID, platform, admission)
	if !ok {
		return nil
	}
	spendDaily, spendWeekly, spendMonthly := rt.gatewayUserPlatformSpend(ctx, userID, providerID, time.Now().UTC())
	estimated := admission.Pricing.Amount
	windows := []struct {
		limit *string
		spent string
		name  string
	}{
		{daily, spendDaily, "daily"},
		{weekly, spendWeekly, "weekly"},
		{monthly, spendMonthly, "monthly"},
	}
	for _, window := range windows {
		if window.limit == nil {
			continue
		}
		if compareDecimalMoney(addDecimalMoney(window.spent, estimated), *window.limit) > 0 {
			return provideradaptercontract.ProviderError{
				Class:      "platform_quota_exceeded",
				StatusCode: http.StatusPaymentRequired,
				Message:    fmt.Sprintf("%s platform %s spend quota exceeded", platform, window.name),
			}
		}
	}
	return nil
}

func gatewayModelReferences(canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution) []string {
	refs := []string{canonical.CanonicalModel, canonical.Model, resolution.Model.CanonicalName}
	if resolution.Alias != nil {
		refs = append(refs, resolution.Alias.Alias)
		refs = append(refs, resolution.Alias.FallbackModels...)
	}
	return uniqueNonEmptyStrings(refs)
}

// Gateway balance-gate and entitlement-error mapping helpers live in
// runtime_gateway_entitlement.go to keep this route-family file within the
// architecture size budget.

func gatewayRateLimitReason(name string) string {
	switch strings.TrimSpace(name) {
	case "rpm":
		return "rpm_limit_exceeded"
	case "tpm":
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

// compareDecimalMoney returns -1, 0 or 1 for left < / == / > right, parsing the
// same fixed-decimal money strings addDecimalMoney produces.
func compareDecimalMoney(left string, right string) int {
	leftRat, ok := new(big.Rat).SetString(defaultDecimalMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := new(big.Rat).SetString(defaultDecimalMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Cmp(rightRat)
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
			if accountExcludesModel(account.Metadata, model.CanonicalName, mapping.UpstreamModelName) {
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
	// Live in-flight concurrency makes load-aware scoring (saturation penalty,
	// concurrency-full reject) reflect real traffic instead of always seeing 0,
	// so the scheduler spreads load across equally-capable accounts under
	// pressure rather than hammering one until its hard cap fails. Take the max
	// with any metadata-provided value so a manual override/floor is preserved.
	if rt.scheduler != nil {
		if live := rt.scheduler.AccountConcurrency(ctx, account.ID); live > state.CurrentConcurrency {
			state.CurrentConcurrency = live
		}
		// Least-recently-used marker for fair rotation across equal-scored accounts.
		if lastUsed := rt.scheduler.AccountLastUsed(ctx, account.ID); lastUsed > state.LastUsedUnixMs {
			state.LastUsedUnixMs = lastUsed
		}
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
	if rt.payloadRules == nil {
		return req
	}
	model := strings.TrimSpace(req.Mapping.UpstreamModelName)
	if model == "" {
		model = req.Model
	}
	resolved := rt.payloadRules.Resolve(ctx, model, req.TargetProtocol)
	transforms := make([]provideradaptercontract.PayloadTransform, 0, len(resolved)+1)
	for _, op := range resolved {
		transforms = append(transforms, provideradaptercontract.PayloadTransform{
			Action: op.Action,
			Path:   op.Path,
			Value:  op.Value,
		})
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
	defer rt.releaseGatewayConcurrency(dispatch.concurrencyLeases)
	req.Credential = dispatch.credential
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
	return rt.doRefreshReverseProxyCredential(ctx, account, credential)
}

func (rt *runtimeState) refreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
	if !shouldRefreshCredential(account, credential) {
		return credential, false, nil
	}
	return rt.doRefreshReverseProxyCredential(ctx, account, credential)
}

func (rt *runtimeState) doRefreshReverseProxyCredential(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (map[string]any, bool, error) {
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
		// A permanently rejected refresh token (session_invalid) parks the account
		// for re-auth so the scheduler stops selecting it and we stop replaying a
		// dead refresh token; transient classes (rate_limit/timeout/upstream_error)
		// map to no status and leave the account untouched for the next attempt.
		rt.protectProviderAccountForClass(ctx, account, errorClassName(err))
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
	if !gatewayAccountFailureStatusHandled(account.Metadata, &providerErr.StatusCode) {
		return
	}
	rt.protectProviderAccountForClass(ctx, account, providerErr.Class)
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

func gatewayTokenCountFromProvider(resp provideradaptercontract.TokenCountResponse) gatewaycontract.TokenCountResponse {
	return gatewaycontract.TokenCountResponse{
		TotalTokens:             resp.TotalTokens,
		CachedContentTokenCount: cloneIntPtr(resp.CachedContentTokenCount),
		PromptTokensDetails:     gatewayModalityTokenCountsFromProvider(resp.PromptTokensDetails),
		CacheTokensDetails:      gatewayModalityTokenCountsFromProvider(resp.CacheTokensDetails),
		Metadata:                cloneAnyMap(resp.Metadata),
	}
}

func gatewayModalityTokenCountsFromProvider(values []provideradaptercontract.ModalityTokenCount) []gatewaycontract.ModalityTokenCount {
	if len(values) == 0 {
		return nil
	}
	out := make([]gatewaycontract.ModalityTokenCount, 0, len(values))
	for _, value := range values {
		out = append(out, gatewaycontract.ModalityTokenCount{
			Modality:   value.Modality,
			TokenCount: value.TokenCount,
			Metadata:   cloneAnyMap(value.Metadata),
		})
	}
	return out
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
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CachedTokens:        usage.CachedTokens,
		CacheCreationTokens: usage.CacheCreationTokens,
		Estimated:           usage.Estimated,
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
	case "rate_limit", "rpm_limit_exceeded", "tpm_limit_exceeded", "concurrency_limit_exceeded", "platform_quota_exceeded":
		return apiopenapi.RateLimitError
	case "auth_failed", "auth_error", "permission_denied", "session_invalid", "account_locked", "account_banned", "abuse_detected", "device_unrecognized":
		return apiopenapi.PermissionError
	case "timeout", "network_error", "configuration_error", "stream_interrupted", "no_available_account", "overloaded":
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
