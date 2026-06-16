package httpserver

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/metacoerce"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

// recordGatewayUsage records a completed gateway request. The durable
// source-of-truth — the usage_log row and its derived completion event — is
// written SYNCHRONOUSLY so it survives a crash and the billing/feedback state
// derived from it stays reconstructable. usage_log is an append, not the
// row-locked aggregation, so it adds negligible latency/contention to the hot
// path. The contended follow-on writes (the two row-locked billing
// aggregations, scheduler feedback, cooldown, snapshot refresh) are then
// dispatched off the request critical path when async usage writing is armed
// (see startUsageWriters), with a context detached via context.WithoutCancel so
// a client disconnect can't cancel them. When async is disabled or saturated
// they run inline — identical to the historical behavior.
func (rt *runtimeState) recordGatewayUsage(ctx context.Context, rec gatewayUsageRecord) {
	if rec.AttemptNo == 0 {
		rec.AttemptNo = 1
	}
	model := fallbackModelName(rec.Model)
	pricing := rec.Pricing.withDefaults()
	rt.warnDefaultZeroGatewayPricing(rec, model, pricing)
	rateMultiplier := rt.gatewayAccountRateMultiplier(ctx, rec.AccountID)
	pricing = rt.gatewayUsageCost(ctx, rec, pricing, rateMultiplier)
	usageLog, usageErr := rt.usage.Record(ctx, usagecontract.RecordRequest{
		RequestID:             rec.RequestID,
		AttemptNo:             rec.AttemptNo,
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
		CacheCreationTokens:   rec.CacheCreationTokens,
		UsageEstimated:        rec.UsageEstimated,
		LatencyMS:             rec.LatencyMS,
		Success:               rec.Success,
		ErrorClass:            rec.ErrorClass,
		Cost:                  pricing.Amount,
		ActualCost:            pricing.ActualCost,
		RateMultiplier:        rateMultiplier,
		BillableCost:          pricing.BillableCost,
		InputCost:             pricing.InputCost,
		OutputCost:            pricing.OutputCost,
		CacheReadCost:         pricing.CacheReadCost,
		CacheWriteCost:        pricing.CacheWriteCost,
		RequestedModel:        gatewayUsageRequestedModel(rec, model),
		UpstreamModel:         strings.TrimSpace(rec.UpstreamModel),
		BillingMode:           string(pricing.BillingMode),
		Currency:              pricing.Currency,
		CompatibilityWarnings: rec.CompatibilityWarnings,
	})
	if usageErr != nil {
		rt.logger.Warn("failed to record usage log", "error", usageErr, "request_id", rec.RequestID)
		rt.enqueueGatewayUsageFailureEvent(ctx, rec, model)
	} else {
		if rt.metrics != nil {
			rt.metrics.RecordGatewayUsage(usageLog)
		}
		rt.enqueueGatewayUsageEvent(ctx, usageLog)
	}

	usageLogID := 0
	if usageErr == nil {
		usageLogID = usageLog.ID
	}
	// Release the atomic balance reservation taken at admission. Idempotent
	// on the store, so repeated recordGatewayUsage calls from a failover loop
	// just succeed-on-the-first and no-op on subsequent calls. Skipped when
	// the reservation gate isn't wired (Redis not configured) or there's no
	// user (anonymous / api-key auth without a backing user).
	rt.releaseGatewayReservation(ctx, rec.Authed.UserID, rec.RequestID)

	detached := context.WithoutCancel(ctx)
	rt.dispatchUsageWrite(detached, func(c context.Context) {
		rt.recordGatewayUsageEffects(c, rec, model, pricing, usageLogID)
	})
}

// recordGatewayUsageEffects applies the follow-on writes that are not the
// durable source of truth and are safe to run off the request critical path:
// the row-locked billing aggregation, scheduler feedback, provider-account
// cooldown, risk signals and the account snapshot refresh. On a hard crash an
// in-flight invocation may be lost, but every value here is derivable from the
// usage_log row recorded synchronously by recordGatewayUsage.
func (rt *runtimeState) recordGatewayUsageEffects(ctx context.Context, rec gatewayUsageRecord, model string, pricing gatewayPricingEvidence, usageLogID int) {
	if rec.Success {
		// Prefer the marked, idempotent cross-table aggregation when wired; it
		// applies the same subscription + api-key increments atomically and lets
		// the reconciler recover any dropped applies. Fall back to the direct
		// increments when no aggregator is configured (tests, memory storage) or
		// the usage_log row wasn't persisted.
		if rt.usageAggregator != nil && usageLogID > 0 {
			if _, err := rt.usageAggregator.ApplyAggregation(ctx, usageLogID); err != nil {
				rt.logger.Warn("failed to aggregate gateway usage billing", "error", err, "request_id", rec.RequestID, "usage_log_id", usageLogID)
			}
		} else {
			rt.recordGatewayMaterializedCosts(ctx, rec, pricing.BillableCost)
		}
	}
	if rec.DecisionID <= 0 || rec.AccountID == nil || rec.ProviderID == nil {
		return
	}
	feedback, feedbackErr := rt.scheduler.RecordFeedback(ctx, schedulercontract.RecordFeedbackRequest{
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
		ActualCost:   pricing.ActualCost,
		Currency:     pricing.Currency,
	})
	if feedbackErr != nil {
		rt.logger.Warn("failed to record scheduler feedback", "error", feedbackErr, "request_id", rec.RequestID)
	} else if rec.QualityPrompt != "" && rec.QualityOutput != "" {
		rec.FeedbackID = feedback.ID
		rt.captureGatewayQualitySample(ctx, rec, rec.QualityPrompt, rec.QualityOutput)
	}
	if !rec.Success && rec.ErrorClass != nil {
		rt.applyProviderAccountCooldown(ctx, rec)
	}
	rt.recordGatewayRiskFailure(ctx, rec)
	rt.enqueueGatewayAccountSnapshotRefresh(ctx, rec)
}

func gatewayUsageRequestedModel(rec gatewayUsageRecord, recordedModel string) string {
	if model := strings.TrimSpace(rec.RequestedModel); model != "" {
		return model
	}
	return fallbackModelName(recordedModel)
}

func (rt *runtimeState) gatewayUsageCost(ctx context.Context, rec gatewayUsageRecord, pricing gatewayPricingEvidence, rateMultiplier string) gatewayPricingEvidence {
	allowance := rt.gatewayAllowanceCostInputs(ctx, rec, pricing.Amount)
	result := rt.billing.PriceGatewayCost(billingcontract.GatewayCostRequest{
		Amount:               pricing.Amount,
		Currency:             pricing.Currency,
		PricingRuleID:        pricing.PricingRuleID,
		BillingMode:          pricing.BillingMode,
		InputCost:            pricing.InputCost,
		OutputCost:           pricing.OutputCost,
		CacheReadCost:        pricing.CacheReadCost,
		CacheWriteCost:       pricing.CacheWriteCost,
		Source:               pricing.PricingSource,
		Estimated:            pricing.PricingEstimated,
		RateMultiplier:       rateMultiplier,
		Success:              rec.Success,
		AllowanceMode:        allowance.mode,
		DailyAllowanceQuota:  allowance.dailyQuota,
		WeeklyAllowanceQuota: allowance.weeklyQuota,
		AllowanceQuota:       allowance.monthlyQuota,
		DailyUsedCost:        allowance.dailyUsedCost,
		WeeklyUsedCost:       allowance.weeklyUsedCost,
		UsedCost:             allowance.monthlyUsedCost,
	})
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

type gatewayAllowanceCostInput struct {
	mode            string
	dailyQuota      *string
	weeklyQuota     *string
	monthlyQuota    *string
	dailyUsedCost   string
	weeklyUsedCost  string
	monthlyUsedCost string
}

func (rt *runtimeState) gatewayAllowanceCostInputs(ctx context.Context, rec gatewayUsageRecord, cost string) gatewayAllowanceCostInput {
	if rt.subscriptions == nil || !rec.Success || rec.Authed.UserID <= 0 {
		return gatewayAllowanceCostInput{}
	}
	trimmed := strings.TrimSpace(cost)
	if trimmed == "" || trimmed == "0.00000000" || trimmed == "0" {
		return gatewayAllowanceCostInput{}
	}
	now := time.Now().UTC()
	allowance, err := rt.subscriptions.CostAllowance(ctx, rec.Authed.UserID, now)
	if err != nil || allowance.Mode != "allowance" || (allowance.DailyQuota == nil && allowance.WeeklyQuota == nil && allowance.Quota == nil) {
		return gatewayAllowanceCostInput{}
	}
	input := gatewayAllowanceCostInput{
		mode:         allowance.Mode,
		dailyQuota:   allowance.DailyQuota,
		weeklyQuota:  allowance.WeeklyQuota,
		monthlyQuota: allowance.Quota,
	}
	if materialized, err := rt.subscriptions.MaterializedUsageForUser(ctx, rec.Authed.UserID, now); err == nil {
		input.dailyUsedCost = materialized.DailyUsageUSD
		input.weeklyUsedCost = materialized.WeeklyUsageUSD
		input.monthlyUsedCost = materialized.MonthlyUsageUSD
		return input
	}
	if periodUsage, err := rt.gatewayUserPeriodUsage(ctx, rec.Authed.UserID, now); err == nil {
		input.monthlyUsedCost = periodUsage.BillableCost
	}
	return input
}

func (rt *runtimeState) recordGatewayMaterializedCosts(ctx context.Context, rec gatewayUsageRecord, billableCost string) {
	if strings.TrimSpace(billableCost) == "" {
		return
	}
	now := time.Now().UTC()
	if rt.subscriptions != nil && rec.Authed.UserID > 0 {
		if _, err := rt.subscriptions.IncrementMaterializedUsage(ctx, subscriptioncontract.UsageDelta{
			UserID:       rec.Authed.UserID,
			BillableCost: billableCost,
			OccurredAt:   now,
		}); err != nil {
			rt.logger.Warn("failed to update subscription materialized usage", "error", err, "request_id", rec.RequestID)
		}
	}
	if rt.apiKeys != nil && rec.Authed.Key.ID > 0 {
		if _, err := rt.apiKeys.ApplyCostUsage(ctx, apikeycontract.CostUsageUpdate{
			KeyID:        rec.Authed.Key.ID,
			BillableCost: billableCost,
			OccurredAt:   now,
		}); err != nil {
			rt.logger.Warn("failed to update api key cost usage", "error", err, "request_id", rec.RequestID)
		}
	}
}

func (rt *runtimeState) gatewayAccountRateMultiplier(ctx context.Context, accountID *int) string {
	if rt == nil || rt.accounts == nil || accountID == nil || *accountID <= 0 {
		return "1.00000000"
	}
	groupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, *accountID)
	if err != nil || len(groupIDs) == 0 {
		return "1.00000000"
	}
	groups, err := rt.accounts.FindGroupsByID(ctx, groupIDs)
	if err != nil || len(groups) == 0 {
		return "1.00000000"
	}
	multiplier := big.NewRat(1, 1)
	found := false
	for _, group := range groups {
		if group.Status != accountcontract.GroupStatusActive {
			continue
		}
		rate, ok := money.DecimalRat(group.RateMultiplier)
		if !ok || rate.Sign() < 0 {
			if rt.logger != nil {
				rt.logger.Warn("invalid account group rate multiplier", "account_id", *accountID, "group_id", group.ID, "rate_multiplier", group.RateMultiplier)
			}
			continue
		}
		multiplier.Mul(multiplier, rate)
		found = true
	}
	if !found {
		return "1.00000000"
	}
	return money.FormatRatFixed(multiplier, 8)
}

func (rt *runtimeState) warnDefaultZeroGatewayPricing(rec gatewayUsageRecord, model string, pricing gatewayPricingEvidence) {
	if pricing.PricingSource != "default_zero" {
		return
	}
	rt.logger.Warn("gateway usage recorded with default zero pricing", "request_id", rec.RequestID, "model", model, "source_endpoint", rec.SourceEndpoint)
}

// networkErrorCooldownWindow is the SHORT account cooldown applied after a
// transport-level failure (proxy / DNS / TCP / TLS — no HTTP status received).
// It mirrors sub2api's transport-error handling, which temporarily unschedules
// an account on a persistent transport fault (openAITransportErrorTempUnschedDuration,
// 10m) so a dead-proxy / DNS-failed account is not immediately reschedulable and
// hammered in a tight loop. We deliberately keep this a few minutes — long enough
// to break the hot loop, short enough that a transient blip recovers quickly —
// not the long auth/overload window. The geometric strike backoff in
// escalateCooldownWindow still escalates a persistently-failing account.
const networkErrorCooldownWindow = 5 * time.Minute

func gatewayErrorClassUsesCooldown(errorClass string) bool {
	switch errorClass {
	case "rate_limit", "overloaded", "auth_failed", "forbidden", "validation_required", "policy_violation", "network_error":
		return true
	default:
		return false
	}
}

type gatewayCooldownDecision struct {
	Reason         string
	LastErrorClass string
	Window         time.Duration
	RetryAfter     *time.Time
}

type gatewayErrorCooldownRule struct {
	StatusCode *int
	ErrorClass string
	Keywords   []string
	Window     time.Duration
	Reason     string
}

const maxGatewayConfiguredCooldownWindow = 2 * time.Hour

const (
	// cooldownStrikeResetAfter resets the consecutive-failure counter once an
	// account has gone this long without a new cooldown (i.e. it recovered).
	cooldownStrikeResetAfter = 15 * time.Minute
	// maxCooldownStrikeShift caps the geometric backoff exponent (also bounded by
	// maxGatewayConfiguredCooldownWindow).
	maxCooldownStrikeShift = 6
)

// escalateCooldownWindow grows the cooldown window geometrically for consecutive
// failures (capped at maxGatewayConfiguredCooldownWindow), and resets the strike
// count when failures stop being recent. It returns the window to apply and the
// strike count to persist.
func escalateCooldownWindow(base time.Duration, strikes int, lastCooldownAt *time.Time, now time.Time) (time.Duration, int) {
	if base <= 0 {
		base = rateLimitCooldownWindow
	}
	if strikes < 0 {
		strikes = 0
	}
	if lastCooldownAt != nil && now.Sub(*lastCooldownAt) > cooldownStrikeResetAfter {
		strikes = 0
	}
	shift := strikes
	if shift > maxCooldownStrikeShift {
		shift = maxCooldownStrikeShift
	}
	window := base << uint(shift)
	if window <= 0 || window > maxGatewayConfiguredCooldownWindow {
		window = maxGatewayConfiguredCooldownWindow
	}
	return window, strikes + 1
}

// accountMetaLock returns the per-account mutex that serializes read-modify-write
// updates to a provider account's metadata document, so concurrent gateway-side
// writers for the same account don't overwrite each other.
func (rt *runtimeState) accountMetaLock(accountID int) *sync.Mutex {
	actual, _ := rt.accountMetaLocks.LoadOrStore(accountID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (rt *runtimeState) applyProviderAccountCooldown(ctx context.Context, rec gatewayUsageRecord) {
	if rec.AccountID == nil || *rec.AccountID <= 0 || rec.ErrorClass == nil {
		return
	}
	// Hold the per-account lock across the whole read-modify-write so a
	// concurrent cooldown or quota-metadata update for the same account can't
	// clobber the strike/cooldown fields we write back.
	mu := rt.accountMetaLock(*rec.AccountID)
	mu.Lock()
	defer mu.Unlock()
	account, err := rt.accounts.FindByID(ctx, *rec.AccountID)
	if err != nil {
		rt.logger.Warn("failed to load cooling provider account", "error", err, "account_id", *rec.AccountID)
		return
	}
	if !gatewayAccountFailureStatusHandled(account.Metadata, rec.StatusCode) {
		return
	}
	decision, ok := gatewayCooldownDecisionForFailure(account.Metadata, *rec.ErrorClass, rec.StatusCode, rec.ProviderErrorMessage, rec.ProviderRetryAfter)
	if !ok {
		return
	}
	now := time.Now().UTC()
	// Escalate the window for consecutive failures so a persistently-failing
	// account is backed off harder, while a one-off blip recovers quickly.
	strikes := metadataInt(account.Metadata, "cooldown_strikes")
	window, nextStrikes := escalateCooldownWindow(decision.Window, strikes, metadataOptionalTime(account.Metadata, "cooldown_last_at"), now)
	cooldownUntil := now.Add(window)
	// An explicit provider Retry-After/reset still wins when it is later.
	if decision.RetryAfter != nil && decision.RetryAfter.After(cooldownUntil) {
		cooldownUntil = decision.RetryAfter.UTC()
	}
	metadata := cloneMetadata(account.Metadata)
	metadata["cooldown_active"] = true
	metadata["cooldown_reason"] = decision.Reason
	metadata["cooldown_until"] = cooldownUntil.Format(time.RFC3339)
	metadata["cooldown_strikes"] = nextStrikes
	metadata["cooldown_last_at"] = now.Format(time.RFC3339)
	metadata["last_error_class"] = decision.LastErrorClass
	// Fold any Codex 429 rate-limit headers from the failing response into the
	// persisted metadata so quota windows survive across cooldown (sub2api parity).
	for key, value := range codexCooldownMetadataUpdates(rec.Headers, now) {
		metadata[key] = value
	}
	before := accountAuditSnapshot(account)
	updated, err := rt.accounts.Update(ctx, *rec.AccountID, accountcontract.UpdateRequest{Metadata: &metadata})
	if err != nil {
		rt.logger.Warn("failed to apply provider account cooldown", "error", err, "account_id", *rec.AccountID, "error_class", *rec.ErrorClass)
		return
	}
	rt.recordAudit(ctx, auditcontract.RecordRequest{
		Action:       "provider_account.cooldown",
		ResourceType: "provider_account",
		ResourceID:   strconv.Itoa(*rec.AccountID),
		Before:       before,
		After:        accountAuditSnapshot(updated),
		TraceID:      requestIDFromContext(ctx),
	})
}

func gatewayCooldownDecisionForFailure(metadata map[string]any, errorClass string, statusCode *int, providerMessage string, retryAfter *time.Time) (gatewayCooldownDecision, bool) {
	if rule, ok := gatewayConfiguredErrorCooldownRule(metadata, errorClass, statusCode, providerMessage); ok {
		return gatewayCooldownDecision{
			Reason:         rule.Reason,
			LastErrorClass: errorClass,
			Window:         rule.Window,
			RetryAfter:     retryAfter,
		}, true
	}
	if !gatewayErrorClassUsesCooldown(errorClass) {
		return gatewayCooldownDecision{}, false
	}
	if errorClass == "forbidden" {
		return gatewayCooldownDecision{
			Reason:         "auth_failed",
			LastErrorClass: "auth_failed",
			Window:         authFailureCooldownWindow,
			RetryAfter:     retryAfter,
		}, true
	}
	return gatewayCooldownDecision{
		Reason:         errorClass,
		LastErrorClass: errorClass,
		Window:         gatewayCooldownWindow(errorClass),
		RetryAfter:     retryAfter,
	}, true
}

func gatewayAccountFailureStatusHandled(metadata map[string]any, statusCode *int) bool {
	if statusCode == nil || *statusCode <= 0 {
		return true
	}
	if value, ok := metacoerce.Value(metadata, "handled_error_status_codes"); ok {
		return gatewayStatusCodeAllowed(gatewayStatusCodeList(value), statusCode)
	}
	if metadataBool(metadata, "pool_mode") {
		return false
	}
	return true
}

func gatewayStatusCodeAllowed(statusCodes []int, statusCode *int) bool {
	if len(statusCodes) == 0 {
		return true
	}
	for _, allowed := range statusCodes {
		if allowed == *statusCode {
			return true
		}
	}
	return false
}

func gatewayStatusCodeList(value any) []int {
	seen := make(map[int]struct{})
	out := make([]int, 0)
	appendStatus := func(raw any) {
		status, ok := gatewayStatusCode(raw)
		if !ok {
			return
		}
		if _, exists := seen[status]; exists {
			return
		}
		seen[status] = struct{}{}
		out = append(out, status)
	}
	switch value := value.(type) {
	case []int:
		for _, item := range value {
			appendStatus(item)
		}
	case []string:
		for _, item := range value {
			for _, part := range gatewaySplitStatusCodes(item) {
				appendStatus(part)
			}
		}
	case []any:
		for _, item := range value {
			if raw, ok := item.(string); ok {
				for _, part := range gatewaySplitStatusCodes(raw) {
					appendStatus(part)
				}
				continue
			}
			appendStatus(item)
		}
	case string:
		for _, item := range gatewaySplitStatusCodes(value) {
			appendStatus(item)
		}
	default:
		appendStatus(value)
	}
	return out
}

func gatewaySplitStatusCodes(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func gatewayStatusCode(value any) (int, bool) {
	status := metadataInt(map[string]any{"status_code": value}, "status_code")
	if status < 100 || status > 599 {
		return 0, false
	}
	return status, true
}

func gatewayCooldownWindow(errorClass string) time.Duration {
	switch errorClass {
	case "overloaded":
		return overloadCooldownWindow
	case "auth_failed", "forbidden", "validation_required", "policy_violation":
		return authFailureCooldownWindow
	case "network_error":
		// SHORT, transport-fault cooldown (sub2api transport-error parity) — not
		// the long auth/overload window.
		return networkErrorCooldownWindow
	default:
		return rateLimitCooldownWindow
	}
}

func gatewayConfiguredErrorCooldownRule(metadata map[string]any, errorClass string, statusCode *int, providerMessage string) (gatewayErrorCooldownRule, bool) {
	rules := gatewayConfiguredErrorCooldownRules(metadata)
	for _, rule := range rules {
		if gatewayErrorCooldownRuleMatches(rule, errorClass, statusCode, providerMessage) {
			return rule, true
		}
	}
	return gatewayErrorCooldownRule{}, false
}

func gatewayConfiguredErrorCooldownRules(metadata map[string]any) []gatewayErrorCooldownRule {
	if metadata == nil {
		return nil
	}
	return parseGatewayErrorCooldownRules(metadata["error_cooldown_rules"])
}

func parseGatewayErrorCooldownRules(value any) []gatewayErrorCooldownRule {
	items := mapList(value)
	if len(items) == 0 {
		return nil
	}
	rules := make([]gatewayErrorCooldownRule, 0, len(items))
	for _, item := range items {
		rule, ok := parseGatewayErrorCooldownRule(item)
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func parseGatewayErrorCooldownRule(values map[string]any) (gatewayErrorCooldownRule, bool) {
	if len(values) == 0 {
		return gatewayErrorCooldownRule{}, false
	}
	statusCode := metadataOptionalInt(values, "status_code", "error_code", "http_status")
	errorClass := metadataString(values, "error_class")
	keywords, _ := metadataStringList(values, "keywords")
	window := gatewayConfiguredCooldownWindow(values)
	if window <= 0 {
		return gatewayErrorCooldownRule{}, false
	}
	if statusCode == nil && errorClass == "" && len(nonEmptyStrings(keywords)) == 0 {
		return gatewayErrorCooldownRule{}, false
	}
	rule := gatewayErrorCooldownRule{
		StatusCode: statusCode,
		ErrorClass: errorClass,
		Keywords:   nonEmptyStrings(keywords),
		Window:     window,
		Reason:     gatewayConfiguredCooldownReason(values),
	}
	return rule, true
}

func gatewayConfiguredCooldownWindow(values map[string]any) time.Duration {
	seconds := metadataInt(values, "cooldown_seconds", "duration_seconds")
	if seconds <= 0 {
		minutes := metadataInt(values, "cooldown_minutes", "duration_minutes")
		seconds = minutes * 60
	}
	if seconds <= 0 {
		return 0
	}
	window := time.Duration(seconds) * time.Second
	if window > maxGatewayConfiguredCooldownWindow {
		return maxGatewayConfiguredCooldownWindow
	}
	return window
}

func gatewayConfiguredCooldownReason(values map[string]any) string {
	reason := metadataString(values, "reason")
	if reason == "" {
		reason = metadataString(values, "cooldown_reason")
	}
	return sanitizeGatewayCooldownReason(reason)
}

func sanitizeGatewayCooldownReason(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "configured_error_rule"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
		if b.Len() >= 80 {
			break
		}
	}
	out := strings.Trim(b.String(), "_.-")
	if out == "" {
		return "configured_error_rule"
	}
	return out
}

func gatewayErrorCooldownRuleMatches(rule gatewayErrorCooldownRule, errorClass string, statusCode *int, providerMessage string) bool {
	if rule.StatusCode != nil && (statusCode == nil || *rule.StatusCode != *statusCode) {
		return false
	}
	if rule.ErrorClass != "" && !strings.EqualFold(rule.ErrorClass, errorClass) {
		return false
	}
	if len(rule.Keywords) == 0 {
		return true
	}
	message := strings.ToLower(providerMessage)
	if message == "" {
		return false
	}
	for _, keyword := range rule.Keywords {
		if strings.Contains(message, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func mapList(value any) []map[string]any {
	switch value := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), value...)
	case []any:
		out := make([]map[string]any, 0, len(value))
		for _, item := range value {
			mapped, ok := item.(map[string]any)
			if ok {
				out = append(out, mapped)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{value}
	default:
		return nil
	}
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
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
	now := time.Now().UTC()
	usageLogs, err := rt.accountSnapshotUsageLogs(ctx, account, now)
	if err != nil {
		rt.logger.Warn("failed to list account usage logs for snapshot", "error", err, "account_id", *rec.AccountID)
		return
	}
	if err := rt.updateAccountRuntimeQuotaMetadata(ctx, account, usageLogs, now); err != nil {
		rt.logger.Warn("failed to update account runtime quota metadata", "error", err, "account_id", account.ID)
	}
	rt.recordProviderQuotaSignals(ctx, account, rec.ProviderQuotaSignals, now)
	health := buildAccountHealthSnapshot(account, usageLogs, now)
	if _, err := rt.accounts.RecordHealthSnapshot(ctx, accountHealthSnapshotFromAPI(health)); err != nil {
		rt.logger.Warn("failed to record account health snapshot", "error", err, "account_id", account.ID)
	}
	quota := buildAccountQuotaSnapshot(account, usageLogs, now)
	if _, err := rt.accounts.RecordQuotaSnapshot(ctx, accountQuotaSnapshotFromAPI(quota)); err != nil {
		rt.logger.Warn("failed to record account quota snapshot", "error", err, "account_id", account.ID)
	}
}

func (rt *runtimeState) accountSnapshotUsageLogs(ctx context.Context, account accountcontract.ProviderAccount, now time.Time) ([]usagecontract.UsageLog, error) {
	window := accountRuntimeQuotaWindow(account.Metadata)
	if costWindow := accountCostWindow(account.Metadata); costWindow > window {
		window = costWindow
	}
	return rt.usage.ListByAccountWindow(ctx, usagecontract.AccountWindowFilter{
		AccountID: account.ID,
		Start:     now.UTC().Add(-window),
		End:       now.UTC().Add(time.Nanosecond),
		Limit:     accountSnapshotUsageLogLimit(account.Metadata),
	})
}

func accountSnapshotUsageLogLimit(metadata map[string]any) int {
	limit := metadataInt(metadata, "runtime_snapshot_usage_limit")
	if limit <= 0 {
		return 5000
	}
	return limit
}

func (rt *runtimeState) enqueueGatewayAccountSnapshotRefresh(ctx context.Context, rec gatewayUsageRecord) {
	if rt.events == nil || rec.AccountID == nil || rec.ProviderID == nil {
		return
	}
	payload := map[string]any{
		"request_id":    rec.RequestID,
		"attempt_no":    rec.AttemptNo,
		"account_id":    *rec.AccountID,
		"provider_id":   *rec.ProviderID,
		"quota_signals": gatewayQuotaSignalsPayload(rec.ProviderQuotaSignals),
	}
	_, err := rt.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "GatewayAccountSnapshotRefreshRequested",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "provider_account",
		AggregateID:    strconv.Itoa(*rec.AccountID),
		CorrelationID:  rec.RequestID,
		CausationID:    rec.RequestID,
		IdempotencyKey: gatewayAccountSnapshotEventIdempotencyKey(rec.RequestID, rec.AttemptNo),
		Payload:        payload,
		Metadata: map[string]any{
			"source_endpoint": rec.SourceEndpoint,
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway account snapshot refresh", "error", err, "request_id", rec.RequestID)
	}
}

func gatewayAccountSnapshotEventIdempotencyKey(requestID string, attemptNo int) string {
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return requestID + ":account_snapshot:" + strconv.Itoa(attemptNo)
}

func gatewayQuotaSignalsPayload(signals []provideradaptercontract.QuotaSignal) []map[string]any {
	out := make([]map[string]any, 0, len(signals))
	for _, signal := range signals {
		item := map[string]any{
			"quota_type":      signal.QuotaType,
			"remaining":       signal.Remaining,
			"used":            signal.Used,
			"quota_limit":     signal.QuotaLimit,
			"remaining_ratio": float64(signal.RemainingRatio),
		}
		if signal.ResetAt != nil {
			item["reset_at"] = signal.ResetAt.UTC().Format(time.RFC3339Nano)
		}
		if !signal.SnapshotAt.IsZero() {
			item["snapshot_at"] = signal.SnapshotAt.UTC().Format(time.RFC3339Nano)
		}
		if len(signal.Metadata) > 0 {
			item["metadata"] = cloneAnyMap(signal.Metadata)
		}
		out = append(out, item)
	}
	return out
}

func (rt *runtimeState) recordProviderQuotaSignals(ctx context.Context, account accountcontract.ProviderAccount, signals []provideradaptercontract.QuotaSignal, now time.Time) {
	for _, signal := range signals {
		if signal.QuotaType == "" {
			continue
		}
		snapshotAt := signal.SnapshotAt
		if snapshotAt.IsZero() {
			snapshotAt = now
		}
		_, err := rt.accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
			AccountID:      account.ID,
			ProviderID:     account.ProviderID,
			QuotaType:      signal.QuotaType,
			Remaining:      signal.Remaining,
			Used:           signal.Used,
			QuotaLimit:     signal.QuotaLimit,
			RemainingRatio: signal.RemainingRatio,
			ResetAt:        cloneTimePtr(signal.ResetAt),
			SnapshotAt:     snapshotAt,
		})
		if err != nil {
			rt.logger.Warn("failed to record provider quota signal", "error", err, "account_id", account.ID, "quota_type", signal.QuotaType)
		}
	}
	rt.persistCodexQuotaUsageMetadata(ctx, account, signals)
}

// persistCodexQuotaUsageMetadata mirrors sub2api's buildCodexUsageExtraUpdates:
// it copies the raw Codex primary/secondary used-percent + reset-after-seconds
// and the snapshot's usage-updated-at marker out of the quota signals and onto
// account.Metadata, so the quota windows survive an offline period (a restart or
// a stretch with no traffic) instead of living only in the per-signal snapshot
// table. The signal Metadata is the same document codexQuotaMetadataFromHeaders
// builds from the upstream x-codex-* headers, so the values are carried through
// verbatim. The write is merged onto the freshest metadata under the per-account
// lock — like updateAccountRuntimeQuotaMetadata / applyProviderAccountCooldown —
// so it does not clobber concurrently-written cooldown / runtime-quota fields.
func (rt *runtimeState) persistCodexQuotaUsageMetadata(ctx context.Context, account accountcontract.ProviderAccount, signals []provideradaptercontract.QuotaSignal) {
	if rt == nil || rt.accounts == nil {
		return
	}
	updates := codexQuotaUsageMetadataUpdates(signals)
	if len(updates) == 0 {
		return
	}
	mu := rt.accountMetaLock(account.ID)
	mu.Lock()
	defer mu.Unlock()
	// Rebase onto the freshest metadata inside the lock so concurrent cooldown /
	// runtime-quota fields written since `account` was loaded are preserved.
	base := account
	if fresh, err := rt.accounts.FindByID(ctx, account.ID); err == nil {
		base = fresh
	}
	metadata := cloneMetadata(base.Metadata)
	for key, value := range updates {
		metadata[key] = value
	}
	if _, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata}); err != nil {
		rt.logger.Warn("failed to persist codex quota usage metadata", "error", err, "account_id", account.ID)
	}
}

// codexQuotaUsageMetadataUpdates extracts the Codex usage fields the durable
// account metadata mirrors from the per-signal Metadata documents. It ports the
// "copy each field when present" shape of sub2api's buildCodexUsageExtraUpdates,
// limited to the keys whose persistence keeps the quota windows alive offline.
// All signals from a single header parse carry the same Metadata, so a later
// signal's value simply reaffirms an earlier one.
func codexQuotaUsageMetadataUpdates(signals []provideradaptercontract.QuotaSignal) map[string]any {
	updates := make(map[string]any)
	for _, signal := range signals {
		if len(signal.Metadata) == 0 {
			continue
		}
		copyMetadataFloat(updates, signal.Metadata, "codex_primary_used_percent")
		copyMetadataInt(updates, signal.Metadata, "codex_primary_reset_after_seconds")
		copyMetadataFloat(updates, signal.Metadata, "codex_secondary_used_percent")
		copyMetadataInt(updates, signal.Metadata, "codex_secondary_reset_after_seconds")
		copyMetadataString(updates, signal.Metadata, "codex_usage_updated_at")
	}
	if len(updates) == 0 {
		return nil
	}
	return updates
}

func copyMetadataFloat(dst, src map[string]any, key string) {
	if value, ok := metacoerce.Float(src[key]); ok {
		dst[key] = value
	}
}

func copyMetadataInt(dst, src map[string]any, key string) {
	if value, ok := metacoerce.Int(src[key]); ok {
		dst[key] = value
	}
}

func copyMetadataString(dst, src map[string]any, key string) {
	if value := metadataString(src, key); value != "" {
		dst[key] = value
	}
}

func (rt *runtimeState) updateAccountRuntimeQuotaMetadata(ctx context.Context, account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) error {
	// Serialize this read-modify-write against a concurrent cooldown / quota
	// update for the same account, and rebase onto the freshest metadata below,
	// so the two writers can't overwrite each other's fields.
	mu := rt.accountMetaLock(account.ID)
	mu.Lock()
	defer mu.Unlock()
	window := accountRuntimeQuotaWindow(account.Metadata)
	windowStart := now.Add(-window)
	rpmUsed := 0
	tpmUsed := 0
	for _, log := range logs {
		if log.CreatedAt.Before(windowStart) {
			continue
		}
		rpmUsed++
		tpmUsed += log.TotalTokens
	}

	// Rolling cost window (default 5h) so an operator can cap an account's spend
	// over a window — the scheduler skips an account once it exceeds cost_window_limit.
	costWindow := accountCostWindow(account.Metadata)
	costWindowStart := now.Add(-costWindow)
	costUsed := new(big.Rat)
	for _, log := range logs {
		if log.CreatedAt.Before(costWindowStart) {
			continue
		}
		amount := strings.TrimSpace(log.BillableCost)
		if amount == "" {
			amount = strings.TrimSpace(log.Cost)
		}
		if parsed, ok := money.DecimalRat(amount); ok {
			costUsed.Add(costUsed, parsed)
		}
	}

	// Rebase onto the freshest metadata inside the lock so concurrent cooldown
	// fields written since `account` was loaded are preserved.
	base := account
	if fresh, err := rt.accounts.FindByID(ctx, account.ID); err == nil {
		base = fresh
	}
	metadata := cloneMetadata(base.Metadata)
	windowSeconds := int(window / time.Second)
	resetAt := now.Add(window).Format(time.RFC3339)
	metadata["rpm_used"] = rpmUsed
	metadata["tpm_used"] = tpmUsed
	metadata["rpm_window_seconds"] = windowSeconds
	metadata["tpm_window_seconds"] = windowSeconds
	metadata["rpm_reset_at"] = resetAt
	metadata["tpm_reset_at"] = resetAt
	metadata["cost_window_used"] = money.FormatRatFixed(costUsed, 8)
	metadata["cost_window_seconds"] = int(costWindow / time.Second)
	metadata["cost_window_reset_at"] = now.Add(costWindow).Format(time.RFC3339)
	metadata["runtime_quota_updated_at"] = now.Format(time.RFC3339)
	_, err := rt.accounts.Update(ctx, account.ID, accountcontract.UpdateRequest{Metadata: &metadata})
	return err
}

func accountRuntimeQuotaWindow(metadata map[string]any) time.Duration {
	seconds := metadataInt(metadata, "runtime_quota_window_seconds", "quota_window_seconds", "rpm_window_seconds", "tpm_window_seconds", "window_seconds")
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

// accountCostWindow is the rolling window over which an account's spend is
// summed for cost_window_limit enforcement (default 5h, sub2api parity).
func accountCostWindow(metadata map[string]any) time.Duration {
	seconds := metadataInt(metadata, "cost_window_seconds")
	if seconds <= 0 {
		seconds = 5 * 60 * 60
	}
	return time.Duration(seconds) * time.Second
}

// recordAccountRecoverySnapshot writes a fresh healthy snapshot (circuit closed,
// no cooldown) so a manual recover/clear-error actually re-enables an account for
// scheduling. accountSchedulerRuntimeState OR-s the latest snapshot's
// circuit/cooldown into runtime state, so without this the stale "open" snapshot
// from the failure keeps the account parked even after the account row is healed.
func (rt *runtimeState) recordAccountRecoverySnapshot(ctx context.Context, account accountcontract.ProviderAccount) {
	if account.Status != accountcontract.StatusActive {
		return
	}
	if _, err := rt.accounts.RecordHealthSnapshot(ctx, accountcontract.AccountHealthSnapshot{
		AccountID:     account.ID,
		ProviderID:    account.ProviderID,
		Status:        "healthy",
		SuccessRate:   1,
		ErrorRate:     0,
		CircuitState:  "closed",
		SnapshotAt:    time.Now().UTC(),
		CooldownUntil: nil,
	}); err != nil {
		rt.logger.Warn("failed to record account recovery health snapshot", "error", err, "account_id", account.ID)
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

func (rt *runtimeState) enqueueGatewayUsageEvent(ctx context.Context, log usagecontract.UsageLog) {
	payload := map[string]any{
		"usage_log_id":           log.ID,
		"request_id":             log.RequestID,
		"attempt_no":             log.AttemptNo,
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
		IdempotencyKey: gatewayUsageEventIdempotencyKey(log.RequestID, log.AttemptNo),
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
		"attempt_no":             rec.AttemptNo,
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
		IdempotencyKey: gatewayUsageEventIdempotencyKey(rec.RequestID, rec.AttemptNo),
		Payload:        payload,
		Metadata: map[string]any{
			"source_protocol": rec.SourceProtocol,
			"source_endpoint": rec.SourceEndpoint,
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway usage failure event", "error", err, "request_id", rec.RequestID)
	}
}

func gatewayUsageEventIdempotencyKey(requestID string, attemptNo int) string {
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return requestID + ":attempt:" + strconv.Itoa(attemptNo)
}
