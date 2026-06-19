package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

const alertRuleIDPrefix = "rule."

// ListAlertRules returns all configured generic metric alert rules.
func (s *Service) ListAlertRules(ctx context.Context) ([]contract.AlertRule, error) {
	result, err := s.ListAlertRulesWithPosture(ctx)
	if err != nil {
		return nil, err
	}
	return result.Rules, nil
}

// ListAlertRulesWithPosture returns alert rules plus service-derived built-in
// baseline posture metadata for AdminOps.
func (s *Service) ListAlertRulesWithPosture(ctx context.Context) (contract.AlertRuleListResult, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.AlertRuleListResult{}, ErrInvalidInput
	}
	rules, err := s.observabilityStore.ListAlertRules(ctx)
	if err != nil {
		return contract.AlertRuleListResult{}, err
	}
	for idx := range rules {
		rules[idx] = cloneAlertRule(rules[idx])
	}
	return contract.AlertRuleListResult{
		Rules:           rules,
		BaselinePosture: alertRuleBaselinePosture(rules),
	}, nil
}

// CreateAlertRule validates and persists a new generic metric alert rule.
func (s *Service) CreateAlertRule(ctx context.Context, req contract.CreateAlertRuleRequest) (contract.AlertRule, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.AlertRule{}, ErrInvalidInput
	}
	now := s.clock.Now()
	rule := contract.AlertRule{
		Name:            strings.TrimSpace(req.Name),
		MetricType:      req.MetricType,
		Operator:        req.Operator,
		Threshold:       req.Threshold,
		Severity:        req.Severity,
		Enabled:         true,
		WindowSeconds:   req.WindowSeconds,
		CooldownSeconds: req.CooldownSeconds,
		MinRequestCount: req.MinRequestCount,
		Scope:           normalizeAlertRuleScope(req.Scope),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	applyAlertRuleDefaults(&rule)
	if err := validateAlertRule(rule); err != nil {
		return contract.AlertRule{}, err
	}
	stored, err := s.observabilityStore.CreateAlertRule(ctx, rule)
	if err != nil {
		return contract.AlertRule{}, err
	}
	return cloneAlertRule(stored), nil
}

// UpdateAlertRule applies a partial update to an existing alert rule.
func (s *Service) UpdateAlertRule(ctx context.Context, id int, req contract.UpdateAlertRuleRequest) (contract.AlertRule, error) {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return contract.AlertRule{}, ErrInvalidInput
	}
	current, err := s.observabilityStore.FindAlertRuleByID(ctx, id)
	if err != nil {
		return contract.AlertRule{}, err
	}
	if req.Name != nil {
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.MetricType != nil {
		current.MetricType = *req.MetricType
	}
	if req.Operator != nil {
		current.Operator = *req.Operator
	}
	if req.Threshold != nil {
		current.Threshold = *req.Threshold
	}
	if req.Severity != nil {
		current.Severity = *req.Severity
	}
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if req.WindowSeconds != nil {
		current.WindowSeconds = *req.WindowSeconds
	}
	if req.CooldownSeconds != nil {
		current.CooldownSeconds = *req.CooldownSeconds
	}
	if req.MinRequestCount != nil {
		current.MinRequestCount = *req.MinRequestCount
	}
	if req.Scope != nil {
		current.Scope = normalizeAlertRuleScope(*req.Scope)
	}
	applyAlertRuleDefaults(&current)
	current.UpdatedAt = s.clock.Now()
	if err := validateAlertRule(current); err != nil {
		return contract.AlertRule{}, err
	}
	updated, err := s.observabilityStore.UpdateAlertRule(ctx, current)
	if err != nil {
		return contract.AlertRule{}, err
	}
	return cloneAlertRule(updated), nil
}

// DeleteAlertRule removes an alert rule by id.
func (s *Service) DeleteAlertRule(ctx context.Context, id int) error {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return ErrInvalidInput
	}
	return s.observabilityStore.DeleteAlertRule(ctx, id)
}

// ListAlertSilences returns all configured alert silences.
func (s *Service) ListAlertSilences(ctx context.Context) ([]contract.AlertSilence, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	silences, err := s.observabilityStore.ListAlertSilences(ctx)
	if err != nil {
		return nil, err
	}
	for idx := range silences {
		silences[idx] = cloneAlertSilence(silences[idx])
	}
	return silences, nil
}

// CreateAlertSilence validates and persists a new alert silence window.
func (s *Service) CreateAlertSilence(ctx context.Context, req contract.CreateAlertSilenceRequest) (contract.AlertSilence, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.AlertSilence{}, ErrInvalidInput
	}
	now := s.clock.Now()
	silence := contract.AlertSilence{
		Comment:   strings.TrimSpace(req.Comment),
		Matcher:   normalizeAlertSilenceMatcher(req.Matcher),
		StartsAt:  req.StartsAt,
		EndsAt:    req.EndsAt,
		CreatedBy: req.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if silence.StartsAt.IsZero() {
		silence.StartsAt = now
	}
	if err := validateAlertSilence(silence); err != nil {
		return contract.AlertSilence{}, err
	}
	return s.observabilityStore.CreateAlertSilence(ctx, silence)
}

// DeleteAlertSilence removes an alert silence by id.
func (s *Service) DeleteAlertSilence(ctx context.Context, id int) error {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return ErrInvalidInput
	}
	return s.observabilityStore.DeleteAlertSilence(ctx, id)
}

// EvaluateAlertRules evaluates enabled generic metric rules and persists alert
// transitions, suppressing matching events with active silences.
func (s *Service) EvaluateAlertRules(ctx context.Context) (contract.AlertRuleEvaluationResult, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.AlertRuleEvaluationResult{}, ErrInvalidInput
	}
	rules, err := s.observabilityStore.ListAlertRules(ctx)
	if err != nil {
		return contract.AlertRuleEvaluationResult{}, err
	}
	now := s.clock.Now()
	usageLogs, err := s.observabilityStore.ListUsageLogsSince(ctx, alertRuleUsageLookback(rules, now))
	if err != nil {
		return contract.AlertRuleEvaluationResult{}, err
	}
	alerts, err := s.observabilityStore.ListAlerts(ctx)
	if err != nil {
		return contract.AlertRuleEvaluationResult{}, err
	}
	silences, err := s.observabilityStore.ListAlertSilences(ctx)
	if err != nil {
		return contract.AlertRuleEvaluationResult{}, err
	}
	return s.evaluateAlertRules(ctx, rules, usageLogs, alerts, silences, now)
}

func (s *Service) evaluateAlertRules(ctx context.Context, rules []contract.AlertRule, usageLogs []usagecontract.UsageLog, alerts []contract.AlertEvent, silences []contract.AlertSilence, now time.Time) (contract.AlertRuleEvaluationResult, error) {
	result := contract.AlertRuleEvaluationResult{}
	active := activeRuleAlertsByFingerprint(alerts)
	seen := map[string]struct{}{}
	for _, rule := range rules {
		if !rule.Enabled || rule.ID <= 0 {
			continue
		}
		result.Evaluated++
		metric := computeRuleMetric(rule, usageLogs, now)
		fingerprint := ruleFingerprint(rule)
		if !ruleBreached(rule, metric) {
			continue
		}
		seen[fingerprint] = struct{}{}
		result.Breached++
		suppressedBy := matchingSilenceID(silences, rule, now)
		existing, ok := active[fingerprint]
		if !ok {
			alert := ruleAlert(rule, metric, fingerprint, now)
			applySilence(&alert, suppressedBy)
			stored, err := s.observabilityStore.CreateAlert(ctx, alert)
			if err != nil {
				return result, err
			}
			if err := s.enqueueAlertNotifications(ctx, stored); err != nil {
				return result, err
			}
			result.Created++
			if suppressedBy != "" {
				result.Suppressed++
			}
			continue
		}
		updated := updateRuleAlert(existing, rule, metric, now)
		applySilence(&updated, suppressedBy)
		stored, err := s.observabilityStore.UpdateAlert(ctx, updated)
		if err != nil {
			return result, err
		}
		if err := s.enqueueAlertNotifications(ctx, stored); err != nil {
			return result, err
		}
		result.Updated++
		if suppressedBy != "" {
			result.Suppressed++
		}
	}
	for fingerprint, alert := range active {
		if _, ok := seen[fingerprint]; ok {
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

func applySilence(alert *contract.AlertEvent, silenceID string) {
	if silenceID == "" {
		return
	}
	alert.Status = contract.AlertStatusSuppressed
	alert.SuppressedBy = &silenceID
}

func activeRuleAlertsByFingerprint(alerts []contract.AlertEvent) map[string]contract.AlertEvent {
	out := map[string]contract.AlertEvent{}
	for _, alert := range alerts {
		if !strings.HasPrefix(alert.RuleID, alertRuleIDPrefix) || alert.Fingerprint == "" {
			continue
		}
		switch alert.Status {
		case contract.AlertStatusFiring, contract.AlertStatusAcknowledged, contract.AlertStatusSuppressed:
			if existing, ok := out[alert.Fingerprint]; !ok || alert.StartedAt.After(existing.StartedAt) {
				out[alert.Fingerprint] = cloneAlert(alert)
			}
		}
	}
	return out
}

func computeRuleMetric(rule contract.AlertRule, usageLogs []usagecontract.UsageLog, now time.Time) ruleMetric {
	window := time.Duration(rule.WindowSeconds) * time.Second
	if window <= 0 {
		window = time.Hour
	}
	windowStart := now.Add(-window)
	metric := ruleMetric{}
	latencies := make([]int, 0, len(usageLogs))
	for _, log := range usageLogs {
		if log.CreatedAt.Before(windowStart) || log.CreatedAt.After(now) {
			continue
		}
		if !ruleScopeMatches(rule.Scope, log) {
			continue
		}
		metric.total++
		if log.Success {
			metric.good++
		} else {
			metric.bad++
		}
		latencies = append(latencies, log.LatencyMS)
	}
	if metric.total > 0 {
		metric.errorRate = float64(metric.bad) / float64(metric.total)
		metric.successRate = float64(metric.good) / float64(metric.total)
	}
	metric.latencyP95 = percentile(latencies, 0.95)
	return metric
}

type ruleMetric struct {
	total       int
	good        int
	bad         int
	errorRate   float64
	successRate float64
	latencyP95  float64
}

func ruleObservedValue(rule contract.AlertRule, metric ruleMetric) float64 {
	switch rule.MetricType {
	case contract.AlertMetricSuccessRate:
		return metric.successRate
	case contract.AlertMetricLatencyP95:
		return metric.latencyP95
	case contract.AlertMetricRequestCount:
		return float64(metric.total)
	default:
		return metric.errorRate
	}
}

func ruleBreached(rule contract.AlertRule, metric ruleMetric) bool {
	if rule.MinRequestCount > 0 && metric.total < rule.MinRequestCount {
		return false
	}
	value := ruleObservedValue(rule, metric)
	switch rule.Operator {
	case contract.AlertOperatorGTE:
		return value >= rule.Threshold
	case contract.AlertOperatorLT:
		return value < rule.Threshold
	case contract.AlertOperatorLTE:
		return value <= rule.Threshold
	default:
		return value > rule.Threshold
	}
}

func percentile(values []int, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	if len(sorted) == 1 {
		return float64(sorted[0])
	}
	rank := q * float64(len(sorted)-1)
	lower := int(rank)
	if lower >= len(sorted)-1 {
		return float64(sorted[len(sorted)-1])
	}
	frac := rank - float64(lower)
	return float64(sorted[lower]) + frac*float64(sorted[lower+1]-sorted[lower])
}

func ruleScopeMatches(scope contract.AlertRuleScope, log usagecontract.UsageLog) bool {
	if scope.SourceEndpoint != "" && scope.SourceEndpoint != strings.TrimSpace(log.SourceEndpoint) {
		return false
	}
	if scope.Model != "" && scope.Model != strings.TrimSpace(log.Model) {
		return false
	}
	if scope.ErrorClass != "" && !usageLogErrorClassMatches(scope.ErrorClass, log) {
		return false
	}
	if scope.ProviderID != nil {
		if log.ProviderID == nil || *log.ProviderID != *scope.ProviderID {
			return false
		}
	}
	return true
}

func matchingSilenceID(silences []contract.AlertSilence, rule contract.AlertRule, now time.Time) string {
	ruleID := ruleAlertRuleID(rule)
	for _, silence := range silences {
		if now.Before(silence.StartsAt) || now.After(silence.EndsAt) {
			continue
		}
		if !silenceMatchesRule(silence.Matcher, rule, ruleID) {
			continue
		}
		return fmt.Sprintf("silence.%d", silence.ID)
	}
	return ""
}

func silenceMatchesRule(matcher contract.AlertSilenceMatcher, rule contract.AlertRule, ruleID string) bool {
	if matcher.RuleID != "" && matcher.RuleID != ruleID {
		return false
	}
	if matcher.Severity != "" && matcher.Severity != rule.Severity {
		return false
	}
	if matcher.SourceEndpoint != "" && matcher.SourceEndpoint != rule.Scope.SourceEndpoint {
		return false
	}
	if matcher.Model != "" && matcher.Model != rule.Scope.Model {
		return false
	}
	if matcher.ErrorClass != "" && canonicalAlertErrorClass(matcher.ErrorClass) != canonicalAlertErrorClass(rule.Scope.ErrorClass) {
		return false
	}
	if matcher.ProviderID != nil {
		if rule.Scope.ProviderID == nil || *rule.Scope.ProviderID != *matcher.ProviderID {
			return false
		}
	}
	return true
}

func ruleAlertRuleID(rule contract.AlertRule) string {
	return fmt.Sprintf("%s%d", alertRuleIDPrefix, rule.ID)
}

func ruleFingerprint(rule contract.AlertRule) string {
	return fmt.Sprintf("rule:%d:%s:%s", rule.ID, rule.MetricType, rule.Operator)
}

func ruleAlert(rule contract.AlertRule, metric ruleMetric, fingerprint string, now time.Time) contract.AlertEvent {
	return contract.AlertEvent{
		RuleID:      ruleAlertRuleID(rule),
		Severity:    rule.Severity,
		Status:      contract.AlertStatusFiring,
		Fingerprint: fingerprint,
		Summary:     ruleSummary(rule, metric),
		Details:     ruleDetails(rule, metric, now),
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func updateRuleAlert(alert contract.AlertEvent, rule contract.AlertRule, metric ruleMetric, now time.Time) contract.AlertEvent {
	alert.RuleID = ruleAlertRuleID(rule)
	alert.Severity = rule.Severity
	if alert.Status != contract.AlertStatusAcknowledged {
		alert.Status = contract.AlertStatusFiring
		alert.SuppressedBy = nil
	}
	alert.Summary = ruleSummary(rule, metric)
	alert.Details = ruleDetails(rule, metric, now)
	alert.ResolvedAt = nil
	alert.UpdatedAt = now
	return alert
}

func ruleSummary(rule contract.AlertRule, metric ruleMetric) string {
	return fmt.Sprintf("%s %s %s %.4g (observed %.4g)", rule.Name, rule.MetricType, rule.Operator, rule.Threshold, ruleObservedValue(rule, metric))
}

func ruleDetails(rule contract.AlertRule, metric ruleMetric, now time.Time) map[string]any {
	windowSeconds := rule.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = 3600
	}
	windowEnd := now.UTC()
	windowStart := windowEnd.Add(-time.Duration(windowSeconds) * time.Second)
	details := map[string]any{
		"rule_id":           rule.ID,
		"rule_name":         rule.Name,
		"metric_type":       string(rule.MetricType),
		"operator":          string(rule.Operator),
		"threshold":         rule.Threshold,
		"observed_value":    ruleObservedValue(rule, metric),
		"window_seconds":    windowSeconds,
		"window_start":      windowStart.Format(time.RFC3339Nano),
		"window_end":        windowEnd.Format(time.RFC3339Nano),
		"total_requests":    metric.total,
		"good_requests":     metric.good,
		"bad_requests":      metric.bad,
		"error_rate":        metric.errorRate,
		"success_rate":      metric.successRate,
		"latency_p95_ms":    metric.latencyP95,
		"min_request_count": rule.MinRequestCount,
	}
	addAlertRuleScopeDetails(details, rule.Scope)
	return details
}

func applyAlertRuleDefaults(rule *contract.AlertRule) {
	if rule.MetricType == "" {
		rule.MetricType = contract.AlertMetricErrorRate
	}
	if rule.Operator == "" {
		rule.Operator = contract.AlertOperatorGT
	}
	if rule.Severity == "" {
		rule.Severity = contract.AlertSeverityWarning
	}
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = 3600
	}
	if rule.CooldownSeconds < 0 {
		rule.CooldownSeconds = 0
	}
	if rule.MinRequestCount < 0 {
		rule.MinRequestCount = 0
	}
}

func validateAlertRule(rule contract.AlertRule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return ErrInvalidInput
	}
	switch rule.MetricType {
	case contract.AlertMetricErrorRate, contract.AlertMetricSuccessRate, contract.AlertMetricLatencyP95, contract.AlertMetricRequestCount:
	default:
		return ErrInvalidInput
	}
	switch rule.Operator {
	case contract.AlertOperatorGT, contract.AlertOperatorGTE, contract.AlertOperatorLT, contract.AlertOperatorLTE:
	default:
		return ErrInvalidInput
	}
	switch rule.Severity {
	case contract.AlertSeverityCritical, contract.AlertSeverityWarning, contract.AlertSeverityTicket:
	default:
		return ErrInvalidInput
	}
	if rule.WindowSeconds <= 0 || rule.WindowSeconds > 30*24*3600 {
		return ErrInvalidInput
	}
	return nil
}

func validateAlertSilence(silence contract.AlertSilence) error {
	if silence.StartsAt.IsZero() || silence.EndsAt.IsZero() {
		return ErrInvalidInput
	}
	if !silence.EndsAt.After(silence.StartsAt) {
		return ErrInvalidInput
	}
	if silence.Matcher.Severity != "" {
		switch silence.Matcher.Severity {
		case contract.AlertSeverityCritical, contract.AlertSeverityWarning, contract.AlertSeverityTicket:
		default:
			return ErrInvalidInput
		}
	}
	return nil
}

func normalizeAlertRuleScope(scope contract.AlertRuleScope) contract.AlertRuleScope {
	scope.SourceEndpoint = strings.TrimSpace(scope.SourceEndpoint)
	scope.Model = strings.TrimSpace(scope.Model)
	scope.ErrorClass = canonicalAlertErrorClass(scope.ErrorClass)
	if scope.ProviderID != nil {
		providerID := *scope.ProviderID
		scope.ProviderID = &providerID
	}
	return scope
}

func normalizeAlertSilenceMatcher(matcher contract.AlertSilenceMatcher) contract.AlertSilenceMatcher {
	matcher.RuleID = strings.TrimSpace(matcher.RuleID)
	matcher.SourceEndpoint = strings.TrimSpace(matcher.SourceEndpoint)
	matcher.Model = strings.TrimSpace(matcher.Model)
	matcher.ErrorClass = canonicalAlertErrorClass(matcher.ErrorClass)
	if matcher.ProviderID != nil {
		providerID := *matcher.ProviderID
		matcher.ProviderID = &providerID
	}
	return matcher
}

func cloneAlertRule(value contract.AlertRule) contract.AlertRule {
	if value.Scope.ProviderID != nil {
		providerID := *value.Scope.ProviderID
		value.Scope.ProviderID = &providerID
	}
	if key := builtinAlertRuleKey(value.Name); key != "" {
		value.BuiltinBaseline = true
		value.BaselineKey = key
	} else {
		value.BuiltinBaseline = false
		value.BaselineKey = ""
	}
	return value
}

func usageLogErrorClassMatches(errorClass string, log usagecontract.UsageLog) bool {
	if log.ErrorClass == nil {
		return false
	}
	return canonicalAlertErrorClass(*log.ErrorClass) == canonicalAlertErrorClass(errorClass)
}

func canonicalAlertErrorClass(value string) string {
	class := strings.ToLower(strings.TrimSpace(value))
	switch class {
	case "rate_limited", "rate_limit_error", "too_many_requests":
		return "rate_limit"
	case "auth_error", "authentication_error", "credential_error":
		return "auth_failed"
	case "permission_error", "permission_denied", "forbidden":
		return "permission_denied"
	case "transport_error":
		return "network_error"
	case "bad_gateway", "server_error", "upstream_5xx":
		return "provider_5xx"
	}
	return class
}

func cloneAlertSilence(value contract.AlertSilence) contract.AlertSilence {
	if value.Matcher.ProviderID != nil {
		providerID := *value.Matcher.ProviderID
		value.Matcher.ProviderID = &providerID
	}
	if value.CreatedBy != nil {
		createdBy := *value.CreatedBy
		value.CreatedBy = &createdBy
	}
	return value
}

// sloUsageLookback returns the earliest usage-log timestamp the SLO evaluations
// need: the longest SLO budget window across definitions plus a safety buffer.
// Zero definitions -> `now`, so no logs are scanned when nothing uses them.
func sloUsageLookback(definitions []contract.SLODefinition, now time.Time) time.Time {
	maxDays := 0
	for _, d := range definitions {
		if d.WindowDays > maxDays {
			maxDays = d.WindowDays
		}
	}
	if maxDays <= 0 {
		return now
	}
	return now.Add(-(time.Duration(maxDays)*24*time.Hour + time.Hour))
}

// alertRuleUsageLookback returns the earliest usage-log timestamp the generic
// metric alert-rule evaluations need: the longest rule window plus a buffer.
func alertRuleUsageLookback(rules []contract.AlertRule, now time.Time) time.Time {
	maxSeconds := 0
	for _, r := range rules {
		if r.WindowSeconds > maxSeconds {
			maxSeconds = r.WindowSeconds
		}
	}
	if maxSeconds <= 0 {
		return now
	}
	return now.Add(-(time.Duration(maxSeconds)*time.Second + time.Hour))
}

// matchingSilenceIDForSLO returns the id of an active silence that should
// suppress an SLO burn-rate alert, or "". Silences that target a specific
// generic rule (RuleID matcher) never match SLO alerts; severity/scope-only
// silences do, so operators can quiet SLO burn-rate noise the same way.
func matchingSilenceIDForSLO(silences []contract.AlertSilence, definition contract.SLODefinition, severity contract.AlertSeverity, now time.Time) string {
	for _, silence := range silences {
		if now.Before(silence.StartsAt) || now.After(silence.EndsAt) {
			continue
		}
		m := silence.Matcher
		if m.RuleID != "" {
			continue
		}
		if m.Severity != "" && m.Severity != severity {
			continue
		}
		if m.SourceEndpoint != "" && m.SourceEndpoint != definition.Filter.SourceEndpoint {
			continue
		}
		if m.Model != "" && m.Model != definition.Filter.Model {
			continue
		}
		if m.ProviderID != nil {
			if definition.Filter.ProviderID == nil || *definition.Filter.ProviderID != *m.ProviderID {
				continue
			}
		}
		return fmt.Sprintf("silence.%d", silence.ID)
	}
	return ""
}
