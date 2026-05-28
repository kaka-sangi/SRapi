package httpserver

import (
	"context"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
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
		ActualCost:   pricing.Amount,
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
	rt.recordGatewayAccountSnapshots(ctx, rec)
}

func (rt *runtimeState) warnDefaultZeroGatewayPricing(rec gatewayUsageRecord, model string, pricing gatewayPricingEvidence) {
	if pricing.PricingSource != "default_zero" {
		return
	}
	rt.logger.Warn("gateway usage recorded with default zero pricing", "request_id", rec.RequestID, "model", model, "source_endpoint", rec.SourceEndpoint)
}

func gatewayErrorClassUsesCooldown(errorClass string) bool {
	switch errorClass {
	case "rate_limit", "overloaded", "auth_failed":
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

func (rt *runtimeState) applyProviderAccountCooldown(ctx context.Context, rec gatewayUsageRecord) {
	if rec.AccountID == nil || *rec.AccountID <= 0 || rec.ErrorClass == nil {
		return
	}
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
	cooldownUntil := time.Now().UTC().Add(decision.Window)
	if decision.RetryAfter != nil && decision.RetryAfter.After(cooldownUntil) {
		cooldownUntil = decision.RetryAfter.UTC()
	}
	metadata := cloneMetadata(account.Metadata)
	metadata["cooldown_active"] = true
	metadata["cooldown_reason"] = decision.Reason
	metadata["cooldown_until"] = cooldownUntil.Format(time.RFC3339)
	metadata["last_error_class"] = decision.LastErrorClass
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
	if value, ok := metadataValue(metadata, "handled_error_status_codes", "account_error_status_codes"); ok {
		return gatewayStatusCodeAllowed(gatewayStatusCodeList(value), statusCode)
	}
	if metadataBool(metadata, "custom_error_codes_enabled") {
		value, ok := metadataValue(metadata, "custom_error_codes")
		if !ok {
			return true
		}
		statusCodes := gatewayStatusCodeList(value)
		if len(statusCodes) == 0 {
			return true
		}
		return gatewayStatusCodeAllowed(statusCodes, statusCode)
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
	case "auth_failed":
		return authFailureCooldownWindow
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
	var rules []gatewayErrorCooldownRule
	rules = append(rules, parseGatewayErrorCooldownRules(metadata["error_cooldown_rules"], false)...)
	rules = append(rules, parseGatewayErrorCooldownRules(metadata["temporary_cooldown_rules"], false)...)
	if metadataBool(metadata, "temp_unschedulable_enabled") {
		rules = append(rules, parseGatewayErrorCooldownRules(metadata["temp_unschedulable_rules"], true)...)
	}
	return rules
}

func parseGatewayErrorCooldownRules(value any, legacyTempRule bool) []gatewayErrorCooldownRule {
	items := mapList(value)
	if len(items) == 0 {
		return nil
	}
	rules := make([]gatewayErrorCooldownRule, 0, len(items))
	for _, item := range items {
		rule, ok := parseGatewayErrorCooldownRule(item, legacyTempRule)
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func parseGatewayErrorCooldownRule(values map[string]any, legacyTempRule bool) (gatewayErrorCooldownRule, bool) {
	if len(values) == 0 {
		return gatewayErrorCooldownRule{}, false
	}
	statusCode := metadataOptionalInt(values, "status_code", "error_code", "http_status")
	errorClass := metadataString(values, "error_class")
	keywords, _ := metadataStringList(values, "keywords")
	window := gatewayConfiguredCooldownWindow(values, legacyTempRule)
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
		Reason:     gatewayConfiguredCooldownReason(values, legacyTempRule),
	}
	return rule, true
}

func gatewayConfiguredCooldownWindow(values map[string]any, legacyTempRule bool) time.Duration {
	seconds := metadataInt(values, "cooldown_seconds", "duration_seconds")
	if seconds <= 0 {
		minutes := metadataInt(values, "cooldown_minutes", "duration_minutes")
		if minutes <= 0 && legacyTempRule {
			minutes = 10
		}
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

func gatewayConfiguredCooldownReason(values map[string]any, legacyTempRule bool) string {
	reason := metadataString(values, "reason")
	if reason == "" {
		reason = metadataString(values, "cooldown_reason")
	}
	if reason == "" && legacyTempRule {
		reason = metadataString(values, "description")
	}
	if reason == "" && legacyTempRule {
		return "temp_unschedulable"
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
	usageLogs, err := rt.usage.List(ctx)
	if err != nil {
		rt.logger.Warn("failed to list usage logs for account snapshot", "error", err, "account_id", *rec.AccountID)
		return
	}
	now := time.Now().UTC()
	accountLogs := usageLogsForAccount(usageLogs, account.ID)
	if err := rt.updateAccountRuntimeQuotaMetadata(ctx, account, accountLogs, now); err != nil {
		rt.logger.Warn("failed to update account runtime quota metadata", "error", err, "account_id", account.ID)
	}
	rt.recordProviderQuotaSignals(ctx, account, rec.ProviderQuotaSignals, now)
	health := buildAccountHealthSnapshot(account, accountLogs, now)
	if _, err := rt.accounts.RecordHealthSnapshot(ctx, accountHealthSnapshotFromAPI(health)); err != nil {
		rt.logger.Warn("failed to record account health snapshot", "error", err, "account_id", account.ID)
	}
	quota := buildAccountQuotaSnapshot(account, accountLogs, now)
	if _, err := rt.accounts.RecordQuotaSnapshot(ctx, accountQuotaSnapshotFromAPI(quota)); err != nil {
		rt.logger.Warn("failed to record account quota snapshot", "error", err, "account_id", account.ID)
	}
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
}

func (rt *runtimeState) updateAccountRuntimeQuotaMetadata(ctx context.Context, account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) error {
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

	metadata := cloneMetadata(account.Metadata)
	windowSeconds := int(window / time.Second)
	resetAt := now.Add(window).Format(time.RFC3339)
	metadata["rpm_used"] = rpmUsed
	metadata["tpm_used"] = tpmUsed
	metadata["rpm_window_seconds"] = windowSeconds
	metadata["tpm_window_seconds"] = windowSeconds
	metadata["rpm_reset_at"] = resetAt
	metadata["tpm_reset_at"] = resetAt
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
