package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	contentsafetycontract "github.com/srapi/srapi/apps/api/internal/modules/content_safety/contract"
	contentsafetyservice "github.com/srapi/srapi/apps/api/internal/modules/content_safety/service"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	"github.com/srapi/srapi/apps/api/internal/pkg/usagewindow"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

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
	contentSafetyResult := contentsafetycontract.Result{}
	if applyContentSafety {
		updated, result, err := rt.applyGatewayContentSafety(ctx, *canonical)
		if err != nil {
			return gatewayAdmission{}, err
		}
		*canonical = updated
		contentSafetyResult = result
	}
	estimatedUsage := estimateGatewayRequestUsage(*canonical)
	// Only reject when the estimate clearly exceeds the context window.
	// The estimator is coarse (words×2), so a 1.5x margin avoids false
	// rejections on borderline requests while still catching obvious misses.
	if cw := resolution.Model.ContextWindow; cw != nil && *cw > 0 {
		totalEstimated := estimatedUsage.InputTokens + estimatedUsage.OutputTokens
		if totalEstimated > *cw*3/2 {
			return gatewayAdmission{
				EstimatedUsage: estimatedUsage,
				Pricing:        zeroGatewayPricing(),
				Entitlement: subscriptioncontract.EntitlementDecision{
					Allowed: false,
					Reason:  "context_window_exceeded",
				},
			}, nil
		}
	}
	if contentSafetyResult.Blocked {
		return gatewayAdmission{
			EstimatedUsage: estimatedUsage,
			Pricing:        zeroGatewayPricing(),
			Entitlement: subscriptioncontract.EntitlementDecision{
				Allowed: false,
				Reason:  "content_safety_blocked",
			},
		}, nil
	}
	// Admission runs before scheduling picks an account, so accountID is nil.
	// Resolve the API key's group IDs so the multiplier reflects the user's
	// channel rate — without this, multiplier > 1.0 under-estimates cost and
	// allows overdraft. The real per-account multiplier is still applied
	// post-dispatch for billing accuracy.
	var admissionGroupIDs []int
	if canonical.APIKeyID > 0 {
		if earlyKey, err := rt.apiKeyByID(ctx, canonical.UserID, canonical.APIKeyID); err == nil {
			admissionGroupIDs = earlyKey.GroupIDs
		}
	}
	pricing := rt.gatewayPricing(ctx, billingcontract.PricingRequest{
		ModelID:      modelID,
		ModelFamily:  optionalStringValue(resolution.Model.Family),
		ProviderID:   0,
		InputTokens:  estimatedUsage.InputTokens,
		OutputTokens: estimatedUsage.OutputTokens,
		At:           time.Now().UTC(),
	}, nil, admissionGroupIDs, true)
	now := time.Now().UTC()
	periodUsage, err := rt.gatewayUserPeriodUsage(ctx, canonical.UserID, now)
	if err != nil {
		return gatewayAdmission{}, err
	}
	entitlement, err := rt.subscriptions.CheckEntitlement(ctx, subscriptioncontract.EntitlementCheckRequest{
		UserID:             canonical.UserID,
		ModelReferences:    gatewayModelReferences(*canonical, resolution),
		EstimatedTokens:    estimatedUsage.InputTokens + estimatedUsage.OutputTokens + estimatedUsage.CachedTokens,
		EstimatedCost:      pricing.Amount,
		TokensUsedInPeriod: periodUsage.TotalTokens,
		CostUsedInPeriod:   periodUsage.BillableCost,
		MaterializedUsage:  &periodUsage.SubscriptionUsage,
		RequestTime:        now,
	})
	if err != nil {
		return gatewayAdmission{}, err
	}
	admission := gatewayAdmission{EstimatedUsage: estimatedUsage, Pricing: pricing, Entitlement: entitlement}
	if !entitlement.Allowed {
		return admission, nil
	}
	// Fetch the user record once and reuse it for both the balance gate and the
	// per-user rate-limit check, which previously each performed an identical
	// users.FindByID on every gateway request. Only load it when one of those
	// consumers would actually read it, mirroring their existing preconditions so
	// no request path gains an extra store lookup.
	var user userscontract.StoredUser
	balanceNeedsUser := rt.cfg.Gateway.RequirePositiveBalance && gatewayEntitlementBalanceBilled(entitlement)
	rateLimitNeedsUser := rt.rateLimiter != nil && canonical.APIKeyID > 0
	if canonical.UserID > 0 && (balanceNeedsUser || rateLimitNeedsUser) {
		user, err = rt.users.FindByID(ctx, canonical.UserID)
		if err != nil {
			return gatewayAdmission{}, err
		}
	}
	// Resolve user attribute overrides (group, RPM, cost multiplier) once per
	// request. The result is cached for 30s to avoid per-request DB hits.
	attrOverrides := rt.resolveUserAttributeOverrides(ctx, canonical.UserID)
	admission.UserAttrOverrides = attrOverrides
	if attrOverrides.RPMOverride > 0 {
		user.RPMLimit = &attrOverrides.RPMOverride
	}

	// Use the request ID (not per-attempt) as the reservation key — admission
	// runs once per request, while the failover loop inside the dispatcher may
	// record multiple usage rows. The reservation needs to span the whole
	// request and be released exactly once at usage record (idempotently).
	denied, err := rt.gatewayBalanceGate(ctx, user, entitlement, pricing, canonical.RequestID)
	if err != nil {
		return gatewayAdmission{}, err
	}
	if denied {
		admission.Entitlement.Allowed = false
		admission.Entitlement.Reason = "insufficient_balance"
		return admission, nil
	}
	apiKey, err := rt.apiKeyByID(ctx, canonical.UserID, canonical.APIKeyID)
	if err != nil {
		return gatewayAdmission{}, err
	}
	if reason := gatewayAPIKeyCostLimitExceeded(apiKey, pricing.BillableCost, now); reason != "" {
		admission.Entitlement.Allowed = false
		admission.Entitlement.Reason = reason
		return admission, nil
	}
	rateLimit, err := rt.checkGatewayRateLimit(ctx, *canonical, user, estimatedUsage, modelID)
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

func (rt *runtimeState) applyGatewayContentSafety(ctx context.Context, canonical gatewaycontract.CanonicalRequest) (gatewaycontract.CanonicalRequest, contentsafetycontract.Result, error) {
	if rt.contentSafety == nil {
		return canonical, contentsafetycontract.Result{}, nil
	}
	config := contentsafetyservice.DefaultConfig()
	var storedModeration admincontrolcontract.ContentSafetyModerationConfig
	if rt.adminControl != nil {
		stored, err := rt.adminControl.GetContentSafetyConfig(ctx)
		if err != nil {
			if rt.logger != nil {
				rt.logger.Warn("content safety config unavailable; using defaults", "error", err, "request_id", canonical.RequestID)
			}
		} else {
			config = contentSafetyConfigFromAdminControl(stored)
			storedModeration = stored.Moderation
		}
	}
	if config.Moderation.Enabled {
		if provider, err := rt.buildModerationProvider(storedModeration); err != nil {
			// Fail-open: skip the upstream pass and record the warning so the
			// next /v1/* request surfaces the misconfiguration via the audit
			// log without dropping the user's traffic.
			if rt.logger != nil {
				rt.logger.Warn("moderation provider build failed; skipping upstream pass", "error", err, "request_id", canonical.RequestID)
			}
			config.Moderation.Enabled = false
		} else {
			config.Moderation.Provider = provider
		}
	}
	updated, result := rt.contentSafety.ApplyWithContext(ctx, canonical, config)
	if result.Changed {
		updated.RawBody = nil
	}
	if len(result.Findings) == 0 {
		return updated, result, nil
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
			"blocked":         result.Blocked,
			"block_reason":    result.Reason,
			"mode":            string(config.Mode),
			"warnings":        nonNilStrings(result.Warnings),
			"findings":        contentSafetyFindingsAudit(result.Findings),
		},
		TraceID: traceIDFromContext(ctx),
	})
	return updated, result, nil
}

func (rt *runtimeState) checkGatewayRateLimit(ctx context.Context, canonical gatewaycontract.CanonicalRequest, user userscontract.StoredUser, usage gatewaycontract.Usage, modelID int) (ratelimit.Decision, error) {
	if rt.rateLimiter == nil || canonical.UserID <= 0 || canonical.APIKeyID <= 0 {
		return ratelimit.Decision{Allowed: true}, nil
	}
	apiKey, err := rt.apiKeyByID(ctx, canonical.UserID, canonical.APIKeyID)
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

// buildGatewayAccountQuotaChecks assembles the per-account rpm/tpm and
// per-account-group rpm/tpm windowed limit checks for a scheduled candidate. It is
// shared by reserveGatewayAccountQuota (which Allow-increments these counters) and
// releaseGatewayAccountQuota (which refunds them on failover) so the reserved and
// refunded checks match exactly.
func (rt *runtimeState) buildGatewayAccountQuotaChecks(ctx context.Context, usage gatewaycontract.Usage, candidate schedulercontract.Candidate) []ratelimit.Check {
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
	return checks
}

func (rt *runtimeState) reserveGatewayAccountQuota(ctx context.Context, usage gatewaycontract.Usage, candidate schedulercontract.Candidate) error {
	if rt.rateLimiter == nil || candidate.Account.ID <= 0 {
		return nil
	}
	checks := rt.buildGatewayAccountQuotaChecks(ctx, usage, candidate)
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

// releaseGatewayAccountQuota refunds a reservation made by reserveGatewayAccountQuota
// when a failover attempt for that candidate fails, so the failed attempt does not
// permanently consume the account's rpm/tpm/group window. Best-effort.
func (rt *runtimeState) releaseGatewayAccountQuota(ctx context.Context, usage gatewaycontract.Usage, candidate schedulercontract.Candidate) {
	if rt.rateLimiter == nil || candidate.Account.ID <= 0 {
		return
	}
	checks := rt.buildGatewayAccountQuotaChecks(ctx, usage, candidate)
	if len(checks) == 0 {
		return
	}
	if err := rt.rateLimiter.Release(ctx, checks); err != nil && rt.logger != nil {
		rt.logger.Warn("failed to refund account quota reservation after failover", "account_id", candidate.Account.ID, "error", err)
	}
}

func (rt *runtimeState) acquireProviderAccountConcurrency(ctx context.Context, account accountcontract.ProviderAccount) (ratelimit.ConcurrencyLease, error) {
	if rt.rateLimiter == nil || account.ID <= 0 {
		return ratelimit.ConcurrencyLease{}, nil
	}
	limit := positiveLimit(accountConcurrencyLimit(account))
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
	// Validate credential has a usable authentication field before routing
	// upstream. Empty or corrupt credentials (e.g. after a failed rotation)
	// would otherwise produce cryptic upstream errors; catching early lets
	// the failover loop skip to the next candidate immediately.
	if !credentialHasAuth(credential) {
		return providerDispatchState{}, provideradaptercontract.ProviderError{Class: "credential_error", StatusCode: http.StatusBadGateway, Message: "provider credential empty or missing authentication fields"}
	}
	releaseOnError = false
	return providerDispatchState{credential: credential, concurrencyLeases: leases}, nil
}

func gatewayAccountQuotaErrorClass(name string) string {
	switch strings.TrimSpace(name) {
	case "account_rpm", "group_rpm":
		return "rpm_limit_exceeded"
	case "account_tpm", "group_tpm":
		return "tpm_limit_exceeded"
	default:
		return "rate_limit"
	}
}

func (rt *runtimeState) apiKeyByID(ctx context.Context, userID int, apiKeyID int) (apikeycontract.APIKey, error) {
	key, err := rt.apiKeys.GetByID(ctx, apiKeyID)
	if err != nil {
		return apikeycontract.APIKey{}, err
	}
	if key.UserID != userID {
		return apikeycontract.APIKey{}, apikeycontract.ErrKeyNotFound
	}
	return key, nil
}

func positiveLimit(value *int) int {
	if value == nil || *value <= 0 {
		return 0
	}
	return *value
}

func (rt *runtimeState) applyGatewayAdmission(ctx context.Context, req *schedulercontract.ScheduleRequest, admission gatewayAdmission) {
	req.EstimatedInputTokens = admission.EstimatedUsage.InputTokens
	req.EstimatedOutputTokens = admission.EstimatedUsage.OutputTokens
	req.EstimatedCost = admission.Pricing.Amount
	req.Currency = admission.Pricing.Currency
	req.PricingRuleID = admission.Pricing.PricingRuleID
	req.PricingSource = admission.Pricing.PricingSource
	req.PricingEstimated = true
	req.AccountGroupScope = append([]int(nil), admission.Entitlement.AccountGroupScope...)
	if groupName := admission.UserAttrOverrides.GroupOverride; groupName != "" {
		if groupIDs := rt.resolveAccountGroupByName(ctx, groupName); len(groupIDs) > 0 {
			req.AccountGroupScope = groupIDs
		}
	}
	if strategy := schedulerStrategyName(admission.Entitlement.SchedulerStrategy); strategy != "" {
		req.Strategy = strategy
	}
}

func (rt *runtimeState) filterCandidatesByAccountGroupScope(ctx context.Context, candidates []schedulercontract.Candidate, scope []int) ([]schedulercontract.Candidate, error) {
	if len(scope) == 0 {
		return candidates, nil
	}
	accountIDs := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		accountIDs = append(accountIDs, candidate.Account.ID)
	}
	groupIDsByAccount, err := rt.accounts.ListGroupIDsByAccounts(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	out := make([]schedulercontract.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if intersectsInt(scope, groupIDsByAccount[candidate.Account.ID]) {
			out = append(out, candidate)
		}
	}
	return out, nil
}

// gatewayPricing computes the priced cost of a usage record. accountID may
// be nil at admission time (no account selected yet) — the multiplier then
// defaults to 1.0x. Post-dispatch callers pass the scheduled account so the
// estimated price returned to the caller matches the eventual debit by the
// balance_charger worker (which reads the multiplier from the persisted usage
// log, see runtime_gateway_usage.go:66).
func (rt *runtimeState) gatewayPricing(ctx context.Context, req billingcontract.PricingRequest, accountID *int, apiKeyGroupIDs []int, estimated bool) gatewayPricingEvidence {
	rateMultiplier := rt.gatewayAccountRateMultiplier(ctx, accountID, apiKeyGroupIDs)
	gatewayReq := contractGatewayPricingRequest(req, rateMultiplier, estimated)
	result, err := rt.billing.PriceGatewayUsage(ctx, gatewayReq)
	if err != nil {
		rt.logger.Warn("failed to estimate gateway price", "error", err, "model_id", req.ModelID, "provider_id", req.ProviderID)
		return gatewayPricingEvidence{Amount: "0.00000000", Currency: "USD", PricingSource: "pricing_error", PricingEstimated: estimated}
	}
	return gatewayPricingEvidence{
		Amount:           result.Amount,
		Currency:         result.Currency,
		PricingRuleID:    cloneIntPtr(result.PricingRuleID),
		BillingMode:      result.BillingMode,
		InputCost:        result.InputCost,
		OutputCost:       result.OutputCost,
		CacheReadCost:    result.CacheReadCost,
		CacheWriteCost:   result.CacheWriteCost,
		PricingSource:    result.Source,
		PricingEstimated: result.Estimated,
		ActualCost:       result.ActualCost,
		BillableCost:     result.BillableCost,
	}.withDefaults()
}

type gatewayPeriodUsage struct {
	TotalTokens       int
	BillableCost      string
	SubscriptionUsage subscriptioncontract.MaterializedUsage
}

func (rt *runtimeState) gatewayUserPeriodUsage(ctx context.Context, userID int, now time.Time) (gatewayPeriodUsage, error) {
	start := usagewindow.StartOfMonthUTC(now)
	summary, err := rt.usage.SummarizeUserWindow(ctx, usagecontract.UserWindowFilter{
		UserID:      userID,
		Start:       start,
		End:         now.UTC().Add(time.Nanosecond),
		SuccessOnly: true,
	})
	if err != nil {
		return gatewayPeriodUsage{}, err
	}
	usage, err := rt.subscriptions.MaterializedUsageForUser(ctx, userID, now.UTC())
	if err != nil {
		return gatewayPeriodUsage{}, err
	}
	cost := money.NormalizeAmount(summary.BillableCost)
	if strings.TrimSpace(usage.MonthlyUsageUSD) != "" {
		cost = money.NormalizeAmount(usage.MonthlyUsageUSD)
	}
	return gatewayPeriodUsage{
		TotalTokens:       summary.TotalTokens,
		BillableCost:      cost,
		SubscriptionUsage: usage,
	}, nil
}

// gatewayUserPlatformSpend sums successful billable spend on a single platform
// with bounded store summaries. Day and month are UTC-calendar-aligned; week is
// the sub2api-compatible rolling trailing 7-day window.
func (rt *runtimeState) gatewayUserPlatformSpend(ctx context.Context, userID, providerID int, now time.Time) (string, string, string) {
	daily, weekly, monthly := "0.00000000", "0.00000000", "0.00000000"
	if rt.usage == nil || userID <= 0 || providerID <= 0 {
		return daily, weekly, monthly
	}
	end := now.UTC().Add(time.Nanosecond)
	daily = rt.gatewayUserProviderWindowSpend(ctx, userID, providerID, usagewindow.StartOfDayUTC(now), end)
	weekly = rt.gatewayUserProviderWindowSpend(ctx, userID, providerID, usagewindow.RollingStartUTC(now, 7*24*time.Hour), end)
	monthly = rt.gatewayUserProviderWindowSpend(ctx, userID, providerID, usagewindow.StartOfMonthUTC(now), end)
	return daily, weekly, monthly
}

func (rt *runtimeState) gatewayUserProviderWindowSpend(ctx context.Context, userID, providerID int, start, end time.Time) string {
	summary, err := rt.usage.SummarizeUserWindow(ctx, usagecontract.UserWindowFilter{
		UserID:      userID,
		ProviderID:  &providerID,
		Start:       start,
		End:         end,
		SuccessOnly: true,
	})
	if err != nil {
		return "0.00000000"
	}
	return money.NormalizeAmount(summary.BillableCost)
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
		if !validPlatformQuotaLimit(window.limit) {
			continue
		}
		if compareMoney(money.AddMoney(window.spent, estimated), *window.limit) > 0 {
			return provideradaptercontract.ProviderError{
				Class:      "platform_quota_exceeded",
				StatusCode: http.StatusPaymentRequired,
				Message:    fmt.Sprintf("%s platform %s spend quota exceeded", platform, window.name),
			}
		}
	}
	return nil
}

func validPlatformQuotaLimit(limit *string) bool {
	if limit == nil {
		return false
	}
	if strings.TrimSpace(*limit) == "" {
		return false
	}
	rat, ok := money.DecimalRat(money.NormalizeAmount(*limit))
	return ok && rat.Sign() >= 0
}

func gatewayModelReferences(canonical gatewaycontract.CanonicalRequest, resolution modelcontract.ModelResolution) []string {
	refs := []string{canonical.CanonicalModel, canonical.Model, resolution.Model.CanonicalName}
	if resolution.Alias != nil {
		refs = append(refs, resolution.Alias.Alias)
		refs = append(refs, resolution.Alias.FallbackModels...)
	}
	return uniqueNonEmptyStrings(refs)
}

// credentialHasAuth returns true when the decrypted credential map contains at
// least one non-empty string value in a known authentication field. This catches
// empty or corrupt credentials (e.g. after a failed rotation) before they reach
// the upstream provider, letting the failover loop skip to the next candidate
// instead of producing a cryptic upstream error.
func credentialHasAuth(cred map[string]any) bool {
	if len(cred) == 0 {
		return false
	}
	for _, key := range []string{
		"api_key", "access_token", "token",
		"aws_access_key_id", "service_account_json",
		"session_cookie", "cookie", "cli_client_token",
	} {
		if v, ok := cred[key].(string); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
