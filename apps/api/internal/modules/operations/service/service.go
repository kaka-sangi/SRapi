package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

const burnRateAlertRulePrefix = "slo.burn_rate."

const (
	defaultRetentionBatchLimit = 1000
	maxRetentionBatchLimit     = 5000
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	retentionStore     contract.RetentionStore
	observabilityStore contract.ObservabilityStore
	systemLogStore     contract.SystemLogStore
	clock              Clock
}

func New(store contract.RetentionStore, clock Clock) (*Service, error) {
	return NewWithStores(store, nil, clock)
}

func NewWithStores(retentionStore contract.RetentionStore, observabilityStore contract.ObservabilityStore, clock Clock) (*Service, error) {
	if retentionStore == nil && observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	systemLogStore, _ := observabilityStore.(contract.SystemLogStore)
	if systemLogStore == nil {
		systemLogStore, _ = retentionStore.(contract.SystemLogStore)
	}
	return &Service{
		retentionStore:     retentionStore,
		observabilityStore: observabilityStore,
		systemLogStore:     systemLogStore,
		clock:              clock,
	}, nil
}

func (s *Service) CleanupRetention(ctx context.Context, policy contract.RetentionPolicy) (contract.CleanupResult, error) {
	if s == nil || s.retentionStore == nil {
		return contract.CleanupResult{}, ErrInvalidInput
	}
	now := s.clock.Now()
	return s.retentionStore.Cleanup(ctx, contract.RetentionCutoffs{
		UsageLogs:              cutoff(now, policy.UsageLogs),
		SchedulerDecisions:     cutoff(now, policy.SchedulerDecisions),
		SchedulerFeedbacks:     cutoff(now, policy.SchedulerFeedbacks),
		AuditLogs:              cutoff(now, policy.AuditLogs),
		AccountHealthSnapshots: cutoff(now, policy.AccountHealthSnapshots),
		BatchLimit:             normalizeRetentionBatchLimit(policy.BatchLimit),
	})
}

func normalizeRetentionBatchLimit(value int) int {
	if value <= 0 {
		return defaultRetentionBatchLimit
	}
	if value > maxRetentionBatchLimit {
		return maxRetentionBatchLimit
	}
	return value
}

func cutoff(now time.Time, retention time.Duration) *time.Time {
	if retention <= 0 {
		return nil
	}
	cutoff := now.Add(-retention)
	return &cutoff
}

func (s *Service) CreateSLO(ctx context.Context, req contract.CreateSLORequest) (contract.SLODefinition, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.SLODefinition{}, ErrInvalidInput
	}
	definition, err := normalizeCreateSLO(req, s.clock.Now())
	if err != nil {
		return contract.SLODefinition{}, err
	}
	return s.observabilityStore.CreateSLO(ctx, definition)
}

func (s *Service) UpdateSLO(ctx context.Context, id int, req contract.UpdateSLORequest) (contract.SLODefinition, error) {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return contract.SLODefinition{}, ErrInvalidInput
	}
	current, err := s.observabilityStore.FindSLOByID(ctx, id)
	if err != nil {
		return contract.SLODefinition{}, err
	}
	if req.Name != nil {
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.Objective != nil {
		current.Objective = normalizeObjective(*req.Objective)
	}
	if req.WindowDays != nil {
		current.WindowDays = *req.WindowDays
	}
	if req.Status != nil {
		current.Status = *req.Status
	}
	if req.Filter != nil {
		current.Filter = normalizeSLOFilter(*req.Filter)
	}
	if req.AlertPolicy != nil {
		current.AlertPolicy = normalizeAlertPolicy(*req.AlertPolicy)
	}
	current.UpdatedAt = s.clock.Now()
	if err := validateSLODefinition(current); err != nil {
		return contract.SLODefinition{}, err
	}
	return s.observabilityStore.UpdateSLO(ctx, current)
}

func (s *Service) DeleteSLO(ctx context.Context, id int) error {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return ErrInvalidInput
	}
	return s.observabilityStore.DeleteSLO(ctx, id)
}

func (s *Service) ListSLOs(ctx context.Context) ([]contract.SLOWithEvaluation, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	definitions, err := s.observabilityStore.ListSLOs(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	usageLogs, err := s.observabilityStore.ListUsageLogsSince(ctx, sloUsageLookback(definitions, now))
	if err != nil {
		return nil, err
	}
	out := make([]contract.SLOWithEvaluation, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, contract.SLOWithEvaluation{
			Definition: cloneSLODefinition(definition),
			Evaluation: evaluateSLO(definition, usageLogs, now),
		})
	}
	return out, nil
}

func (s *Service) ListAlerts(ctx context.Context) ([]contract.AlertEvent, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	alerts, err := s.observabilityStore.ListAlerts(ctx)
	if err != nil {
		return nil, err
	}
	for idx := range alerts {
		alerts[idx] = cloneAlert(alerts[idx])
	}
	return alerts, nil
}

// EvaluateSLOAlerts evaluates active availability SLO burn-rate thresholds and persists alert transitions.
func (s *Service) EvaluateSLOAlerts(ctx context.Context) (contract.AlertEvaluationResult, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.AlertEvaluationResult{}, ErrInvalidInput
	}
	definitions, err := s.observabilityStore.ListSLOs(ctx)
	if err != nil {
		return contract.AlertEvaluationResult{}, err
	}
	now := s.clock.Now()
	usageLogs, err := s.observabilityStore.ListUsageLogsSince(ctx, sloUsageLookback(definitions, now))
	if err != nil {
		return contract.AlertEvaluationResult{}, err
	}
	alerts, err := s.observabilityStore.ListAlerts(ctx)
	if err != nil {
		return contract.AlertEvaluationResult{}, err
	}
	silences, err := s.observabilityStore.ListAlertSilences(ctx)
	if err != nil {
		return contract.AlertEvaluationResult{}, err
	}
	return s.evaluateAlerts(ctx, definitions, usageLogs, alerts, silences, now)
}

func (s *Service) AcknowledgeAlert(ctx context.Context, id int, req contract.AckAlertRequest) (contract.AlertEvent, error) {
	if s == nil || s.observabilityStore == nil || id <= 0 || req.ActorUserID <= 0 {
		return contract.AlertEvent{}, ErrInvalidInput
	}
	alert, err := s.observabilityStore.FindAlertByID(ctx, id)
	if err != nil {
		return contract.AlertEvent{}, err
	}
	now := req.Now
	if now.IsZero() {
		now = s.clock.Now()
	}
	alert.Status = contract.AlertStatusAcknowledged
	alert.AcknowledgedAt = &now
	alert.AcknowledgedBy = &req.ActorUserID
	alert.UpdatedAt = now
	return s.observabilityStore.UpdateAlert(ctx, alert)
}

func (s *Service) evaluateAlerts(ctx context.Context, definitions []contract.SLODefinition, usageLogs []usagecontract.UsageLog, alerts []contract.AlertEvent, silences []contract.AlertSilence, now time.Time) (contract.AlertEvaluationResult, error) {
	result := contract.AlertEvaluationResult{}
	activeAlerts := activeBurnRateAlertsByFingerprint(alerts)
	seenBreaches := map[string]struct{}{}
	for _, definition := range definitions {
		if !sloAlertEligible(definition) {
			continue
		}
		result.Evaluated++
		for _, threshold := range definition.AlertPolicy.Thresholds {
			if threshold.BurnRate <= 0 {
				continue
			}
			longWindow := thresholdLongWindow(definition, threshold)
			shortWindow := thresholdShortWindow(longWindow, threshold)
			longEval := evaluateSLOWindow(definition, usageLogs, now, longWindow)
			shortEval := evaluateSLOWindow(definition, usageLogs, now, shortWindow)
			fingerprint := burnRateFingerprint(definition.ID, threshold, longWindow, shortWindow)
			if !thresholdBreached(threshold, longEval, shortEval) {
				continue
			}
			seenBreaches[fingerprint] = struct{}{}
			result.Breached++
			suppressedBy := matchingSilenceIDForSLO(silences, definition, threshold.Severity, now)
			alert, ok := activeAlerts[fingerprint]
			if !ok {
				created := burnRateAlert(definition, threshold, longEval, shortEval, now)
				applySilence(&created, suppressedBy)
				stored, err := s.observabilityStore.CreateAlert(ctx, created)
				if err != nil {
					return result, err
				}
				if err := s.enqueueAlertNotifications(ctx, stored); err != nil {
					return result, err
				}
				result.Created++
				continue
			}
			updated := updateBurnRateAlert(alert, definition, threshold, longEval, shortEval, now)
			applySilence(&updated, suppressedBy)
			stored, err := s.observabilityStore.UpdateAlert(ctx, updated)
			if err != nil {
				return result, err
			}
			if err := s.enqueueAlertNotifications(ctx, stored); err != nil {
				return result, err
			}
			result.Updated++
		}
	}
	for fingerprint, alert := range activeAlerts {
		if _, ok := seenBreaches[fingerprint]; ok {
			continue
		}
		alert.Status = contract.AlertStatusResolved
		resolvedAt := now
		alert.ResolvedAt = &resolvedAt
		alert.UpdatedAt = now
		stored, err := s.observabilityStore.UpdateAlert(ctx, alert)
		if err != nil {
			return result, err
		}
		if err := s.enqueueAlertNotifications(ctx, stored); err != nil {
			return result, err
		}
		result.Resolved++
	}
	return result, nil
}

func normalizeCreateSLO(req contract.CreateSLORequest, now time.Time) (contract.SLODefinition, error) {
	status := contract.SLOStatusActive
	if req.Status != nil {
		status = *req.Status
	}
	definition := contract.SLODefinition{
		Name:        strings.TrimSpace(req.Name),
		SLIType:     req.SLIType,
		Objective:   normalizeObjective(req.Objective),
		WindowDays:  req.WindowDays,
		Status:      status,
		Filter:      normalizeSLOFilter(req.Filter),
		AlertPolicy: normalizeAlertPolicy(req.AlertPolicy),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if definition.SLIType == "" {
		definition.SLIType = contract.SLITypeAvailability
	}
	if definition.WindowDays == 0 {
		definition.WindowDays = 28
	}
	if err := validateSLODefinition(definition); err != nil {
		return contract.SLODefinition{}, err
	}
	return definition, nil
}

func validateSLODefinition(definition contract.SLODefinition) error {
	if strings.TrimSpace(definition.Name) == "" {
		return ErrInvalidInput
	}
	switch definition.SLIType {
	case contract.SLITypeAvailability, contract.SLITypeLatency, contract.SLITypeFreshness, contract.SLITypeQuality:
	default:
		return ErrInvalidInput
	}
	if definition.Objective <= 0 || definition.Objective >= 1 {
		return ErrInvalidInput
	}
	if definition.WindowDays <= 0 || definition.WindowDays > 365 {
		return ErrInvalidInput
	}
	switch definition.Status {
	case contract.SLOStatusActive, contract.SLOStatusDisabled:
	default:
		return ErrInvalidInput
	}
	return nil
}

func normalizeObjective(objective float64) float64 {
	if objective > 1 {
		return objective / 100
	}
	return objective
}

func normalizeSLOFilter(filter contract.SLOFilter) contract.SLOFilter {
	filter.SourceEndpoint = strings.TrimSpace(filter.SourceEndpoint)
	filter.Model = strings.TrimSpace(filter.Model)
	filter.ErrorOwnerExclude = uniqueLowerStrings(filter.ErrorOwnerExclude)
	return filter
}

func normalizeAlertPolicy(policy contract.AlertPolicy) contract.AlertPolicy {
	policy.Name = strings.TrimSpace(policy.Name)
	if policy.Name == "" {
		policy.Name = "multi_window_burn_rate"
	}
	thresholds := make([]contract.BurnRateThreshold, 0, len(policy.Thresholds))
	for _, threshold := range policy.Thresholds {
		if threshold.Severity == "" {
			threshold.Severity = contract.AlertSeverityWarning
		}
		if threshold.BurnRate <= 0 {
			continue
		}
		thresholds = append(thresholds, threshold)
	}
	sort.SliceStable(thresholds, func(i, j int) bool {
		return thresholds[i].BurnRate > thresholds[j].BurnRate
	})
	policy.Thresholds = thresholds
	return policy
}

func evaluateSLO(definition contract.SLODefinition, usageLogs []usagecontract.UsageLog, now time.Time) contract.SLOEvaluation {
	windowDays := definition.WindowDays
	if windowDays <= 0 {
		windowDays = 28
	}
	return evaluateSLOWindow(definition, usageLogs, now, time.Duration(windowDays)*24*time.Hour)
}

func evaluateSLOWindow(definition contract.SLODefinition, usageLogs []usagecontract.UsageLog, now time.Time, window time.Duration) contract.SLOEvaluation {
	if window <= 0 {
		window = 28 * 24 * time.Hour
	}
	windowEnd := now
	windowStart := now.Add(-window)
	evaluation := contract.SLOEvaluation{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Objective:   definition.Objective,
	}
	if definition.SLIType != contract.SLITypeAvailability || definition.Status != contract.SLOStatusActive {
		return evaluation
	}
	for _, log := range usageLogs {
		if log.CreatedAt.Before(windowStart) || log.CreatedAt.After(windowEnd) {
			continue
		}
		if !sloFilterMatches(definition.Filter, log) {
			continue
		}
		if excludedErrorOwner(definition.Filter.ErrorOwnerExclude, log) {
			continue
		}
		evaluation.TotalRequests++
		if log.Success {
			evaluation.GoodRequests++
		} else {
			evaluation.BadRequests++
		}
	}
	if evaluation.TotalRequests == 0 {
		return evaluation
	}
	evaluation.ErrorRate = float64(evaluation.BadRequests) / float64(evaluation.TotalRequests)
	budget := 1 - definition.Objective
	if budget > 0 {
		evaluation.BurnRate = evaluation.ErrorRate / budget
		evaluation.ErrorBudgetConsumed = math.Min(1, evaluation.ErrorRate/budget)
	}
	return evaluation
}

func sloAlertEligible(definition contract.SLODefinition) bool {
	return definition.ID > 0 &&
		definition.Status == contract.SLOStatusActive &&
		definition.SLIType == contract.SLITypeAvailability &&
		len(definition.AlertPolicy.Thresholds) > 0
}

func thresholdLongWindow(definition contract.SLODefinition, threshold contract.BurnRateThreshold) time.Duration {
	if threshold.LongWindow > 0 {
		return threshold.LongWindow
	}
	windowDays := definition.WindowDays
	if windowDays <= 0 {
		windowDays = 28
	}
	return time.Duration(windowDays) * 24 * time.Hour
}

func thresholdShortWindow(longWindow time.Duration, threshold contract.BurnRateThreshold) time.Duration {
	if threshold.ShortWindow > 0 {
		return threshold.ShortWindow
	}
	if longWindow <= 0 {
		return time.Hour
	}
	shortWindow := longWindow / 12
	if shortWindow < time.Minute {
		return time.Minute
	}
	return shortWindow
}

func thresholdBreached(threshold contract.BurnRateThreshold, longEval contract.SLOEvaluation, shortEval contract.SLOEvaluation) bool {
	if threshold.MinRequestCount > 0 && (longEval.TotalRequests < threshold.MinRequestCount || shortEval.TotalRequests < threshold.MinRequestCount) {
		return false
	}
	return longEval.BurnRate >= threshold.BurnRate && shortEval.BurnRate >= threshold.BurnRate
}

func activeBurnRateAlertsByFingerprint(alerts []contract.AlertEvent) map[string]contract.AlertEvent {
	out := map[string]contract.AlertEvent{}
	for _, alert := range alerts {
		if alert.SLOID == nil || alert.Fingerprint == "" || !strings.HasPrefix(alert.RuleID, burnRateAlertRulePrefix) {
			continue
		}
		switch alert.Status {
		case contract.AlertStatusFiring, contract.AlertStatusAcknowledged:
			if existing, ok := out[alert.Fingerprint]; !ok || alert.StartedAt.After(existing.StartedAt) {
				out[alert.Fingerprint] = cloneAlert(alert)
			}
		}
	}
	return out
}

func burnRateFingerprint(sloID int, threshold contract.BurnRateThreshold, longWindow, shortWindow time.Duration) string {
	return fmt.Sprintf("slo:%d:burn_rate:%s:%d:%d:%.6f", sloID, threshold.Severity, int(longWindow/time.Second), int(shortWindow/time.Second), threshold.BurnRate)
}

func burnRateAlert(definition contract.SLODefinition, threshold contract.BurnRateThreshold, longEval contract.SLOEvaluation, shortEval contract.SLOEvaluation, now time.Time) contract.AlertEvent {
	sloID := definition.ID
	longWindow := thresholdLongWindow(definition, threshold)
	shortWindow := thresholdShortWindow(longWindow, threshold)
	return contract.AlertEvent{
		SLOID:       &sloID,
		RuleID:      burnRateRuleID(threshold),
		Severity:    threshold.Severity,
		Status:      contract.AlertStatusFiring,
		Fingerprint: burnRateFingerprint(definition.ID, threshold, longWindow, shortWindow),
		Summary:     burnRateSummary(definition, threshold),
		Details:     burnRateDetails(definition, threshold, longEval, shortEval),
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func updateBurnRateAlert(alert contract.AlertEvent, definition contract.SLODefinition, threshold contract.BurnRateThreshold, longEval contract.SLOEvaluation, shortEval contract.SLOEvaluation, now time.Time) contract.AlertEvent {
	alert.RuleID = burnRateRuleID(threshold)
	alert.Severity = threshold.Severity
	if alert.Status != contract.AlertStatusAcknowledged {
		alert.Status = contract.AlertStatusFiring
	}
	alert.Summary = burnRateSummary(definition, threshold)
	alert.Details = burnRateDetails(definition, threshold, longEval, shortEval)
	alert.UpdatedAt = now
	return alert
}

func burnRateRuleID(threshold contract.BurnRateThreshold) string {
	severity := strings.TrimSpace(string(threshold.Severity))
	if severity == "" {
		severity = string(contract.AlertSeverityWarning)
	}
	return burnRateAlertRulePrefix + severity
}

func burnRateSummary(definition contract.SLODefinition, threshold contract.BurnRateThreshold) string {
	return fmt.Sprintf("%s SLO burn rate threshold %.2fx exceeded", definition.Name, threshold.BurnRate)
}

func burnRateDetails(definition contract.SLODefinition, threshold contract.BurnRateThreshold, longEval contract.SLOEvaluation, shortEval contract.SLOEvaluation) map[string]any {
	details := map[string]any{
		"slo_id":                definition.ID,
		"slo_name":              definition.Name,
		"sli_type":              string(definition.SLIType),
		"objective":             definition.Objective,
		"window_days":           definition.WindowDays,
		"severity":              string(threshold.Severity),
		"burn_rate_threshold":   threshold.BurnRate,
		"long_window_seconds":   int(thresholdLongWindow(definition, threshold) / time.Second),
		"short_window_seconds":  int(thresholdShortWindow(thresholdLongWindow(definition, threshold), threshold) / time.Second),
		"long_burn_rate":        longEval.BurnRate,
		"short_burn_rate":       shortEval.BurnRate,
		"long_total_requests":   longEval.TotalRequests,
		"short_total_requests":  shortEval.TotalRequests,
		"long_bad_requests":     longEval.BadRequests,
		"short_bad_requests":    shortEval.BadRequests,
		"error_budget_consumed": longEval.ErrorBudgetConsumed,
	}
	addSLOFilterDetails(details, definition.Filter)
	return details
}

func addSLOFilterDetails(details map[string]any, filter contract.SLOFilter) {
	if strings.TrimSpace(filter.SourceEndpoint) != "" {
		details["source_endpoint"] = strings.TrimSpace(filter.SourceEndpoint)
	}
	if strings.TrimSpace(filter.Model) != "" {
		details["model"] = strings.TrimSpace(filter.Model)
	}
	if filter.ProviderID != nil {
		details["provider_id"] = *filter.ProviderID
	}
	if len(filter.ErrorOwnerExclude) > 0 {
		details["error_owner_exclude"] = append([]string(nil), filter.ErrorOwnerExclude...)
	}
}

func addAlertRuleScopeDetails(details map[string]any, scope contract.AlertRuleScope) {
	if strings.TrimSpace(scope.SourceEndpoint) != "" {
		details["source_endpoint"] = strings.TrimSpace(scope.SourceEndpoint)
	}
	if strings.TrimSpace(scope.Model) != "" {
		details["model"] = strings.TrimSpace(scope.Model)
	}
	if scope.ProviderID != nil {
		details["provider_id"] = *scope.ProviderID
	}
}

func sloFilterMatches(filter contract.SLOFilter, log usagecontract.UsageLog) bool {
	if filter.SourceEndpoint != "" && filter.SourceEndpoint != strings.TrimSpace(log.SourceEndpoint) {
		return false
	}
	if filter.Model != "" && filter.Model != strings.TrimSpace(log.Model) {
		return false
	}
	if filter.ProviderID != nil {
		if log.ProviderID == nil || *log.ProviderID != *filter.ProviderID {
			return false
		}
	}
	return true
}

func excludedErrorOwner(exclusions []string, log usagecontract.UsageLog) bool {
	if log.Success || log.ErrorClass == nil {
		return false
	}
	owner := errorOwner(*log.ErrorClass)
	for _, exclusion := range exclusions {
		if owner == strings.ToLower(strings.TrimSpace(exclusion)) {
			return true
		}
	}
	return false
}

func errorOwner(errorClass string) string {
	switch strings.TrimSpace(errorClass) {
	case "invalid_request", "model_not_found", "model_not_allowed", "invalid_api_key", "api_key_disabled", "content_policy":
		return "client"
	case "user_balance_insufficient", "subscription_expired", "quota_exceeded":
		return "business"
	case "no_available_account", "lease_failed", "concurrency_full":
		return "scheduler"
	case "session_invalid", "account_locked", "account_banned", "challenge_required", "device_unrecognized":
		return "reverse_proxy"
	case "internal_error", "internal":
		return "internal"
	default:
		return "provider"
	}
}

func uniqueLowerStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func cloneSLODefinition(value contract.SLODefinition) contract.SLODefinition {
	if value.Filter.ProviderID != nil {
		providerID := *value.Filter.ProviderID
		value.Filter.ProviderID = &providerID
	}
	value.Filter.ErrorOwnerExclude = append([]string(nil), value.Filter.ErrorOwnerExclude...)
	value.AlertPolicy.Thresholds = append([]contract.BurnRateThreshold(nil), value.AlertPolicy.Thresholds...)
	return value
}

func cloneAlert(value contract.AlertEvent) contract.AlertEvent {
	value.Details = cloneMap(value.Details)
	if value.SLOID != nil {
		sloID := *value.SLOID
		value.SLOID = &sloID
	}
	if value.ResolvedAt != nil {
		resolvedAt := *value.ResolvedAt
		value.ResolvedAt = &resolvedAt
	}
	if value.AcknowledgedAt != nil {
		acknowledgedAt := *value.AcknowledgedAt
		value.AcknowledgedAt = &acknowledgedAt
	}
	if value.AcknowledgedBy != nil {
		acknowledgedBy := *value.AcknowledgedBy
		value.AcknowledgedBy = &acknowledgedBy
	}
	if value.SuppressedBy != nil {
		suppressedBy := *value.SuppressedBy
		value.SuppressedBy = &suppressedBy
	}
	return value
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
