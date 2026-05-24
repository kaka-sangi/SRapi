package httpserver

import (
	"context"
	"strconv"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (rt *runtimeState) recordGatewayUsage(ctx context.Context, rec gatewayUsageRecord) {
	model := fallbackModelName(rec.Model)
	if rec.AttemptNo == 0 {
		rec.AttemptNo = 1
	}
	pricing := rec.Pricing.withDefaults()
	rt.warnDefaultZeroGatewayPricing(rec, model, pricing)
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

func (rt *runtimeState) warnDefaultZeroGatewayPricing(rec gatewayUsageRecord, model string, pricing gatewayPricingEvidence) {
	if pricing.PricingSource != "default_zero" {
		return
	}
	rt.logger.Warn("gateway usage recorded with default zero pricing", "request_id", rec.RequestID, "model", model, "source_endpoint", rec.SourceEndpoint)
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
		},
	})
	if err != nil {
		rt.logger.Warn("failed to enqueue gateway usage failure event", "error", err, "request_id", rec.RequestID)
	}
}
