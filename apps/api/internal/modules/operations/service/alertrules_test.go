package service

import (
	"testing"
	"time"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func TestAlertRuleCRUDValidatesAndDefaults(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{Name: "  "}); err != ErrInvalidInput {
		t.Fatalf("expected invalid input for blank name, got %v", err)
	}

	providerID := 7
	created, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:      "Chat error rate",
		Threshold: 0.1,
		Scope:     contract.AlertRuleScope{SourceEndpoint: "/v1/chat/completions", ProviderID: &providerID},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if created.ID == 0 ||
		created.MetricType != contract.AlertMetricErrorRate ||
		created.Operator != contract.AlertOperatorGT ||
		created.Severity != contract.AlertSeverityWarning ||
		!created.Enabled ||
		created.WindowSeconds != 3600 {
		t.Fatalf("unexpected rule defaults: %+v", created)
	}

	disabled := false
	updated, err := svc.UpdateAlertRule(t.Context(), created.ID, contract.UpdateAlertRuleRequest{
		Enabled:    &disabled,
		MetricType: metricPtr(contract.AlertMetricLatencyP95),
		Operator:   operatorPtr(contract.AlertOperatorGTE),
		Threshold:  floatPtr(2500),
		Severity:   severityPtr(contract.AlertSeverityCritical),
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if updated.Enabled || updated.MetricType != contract.AlertMetricLatencyP95 || updated.Severity != contract.AlertSeverityCritical {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	rules, err := svc.ListAlertRules(t.Context())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one rule, got %+v", rules)
	}

	if err := svc.DeleteAlertRule(t.Context(), created.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	rules, err = svc.ListAlertRules(t.Context())
	if err != nil {
		t.Fatalf("list rules after delete: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected zero rules after delete, got %+v", rules)
	}
}

func TestEnsureBuiltinAlertRulesCreatesMissingBaselines(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.EnsureBuiltinAlertRules(t.Context())
	if err != nil {
		t.Fatalf("ensure builtins: %v", err)
	}
	if len(created) != len(builtinAlertRules) {
		t.Fatalf("expected %d builtin rules, got %+v", len(builtinAlertRules), created)
	}
	if created[0].Name != builtinAlertRuleGatewayErrorRate ||
		created[0].MetricType != contract.AlertMetricErrorRate ||
		created[0].Severity != contract.AlertSeverityCritical ||
		created[0].Threshold != 0.05 ||
		created[0].WindowSeconds != 300 ||
		created[0].MinRequestCount != 20 ||
		!created[0].BuiltinBaseline ||
		created[0].BaselineKey != "gateway.error_rate" ||
		!created[0].Enabled ||
		!created[0].CreatedAt.Equal(now) {
		t.Fatalf("unexpected gateway error-rate baseline: %+v", created[0])
	}
	if created[1].Name != builtinAlertRuleGatewayLatencyP95 ||
		created[1].MetricType != contract.AlertMetricLatencyP95 ||
		created[1].Threshold != 15000 ||
		created[1].Severity != contract.AlertSeverityWarning {
		t.Fatalf("unexpected latency baseline: %+v", created[1])
	}
	if created[2].Name != builtinAlertRuleChatCompletionsError ||
		created[2].Scope.SourceEndpoint != string(gatewaycontract.EndpointChatCompletions) ||
		created[2].MinRequestCount != 10 {
		t.Fatalf("unexpected chat baseline: %+v", created[2])
	}
	assertBuiltinRule(t, created, builtinAlertRuleChatCompletionsLatency, contract.AlertMetricLatencyP95, string(gatewaycontract.EndpointChatCompletions), 10)
	assertBuiltinRule(t, created, builtinAlertRuleResponsesError, contract.AlertMetricErrorRate, string(gatewaycontract.EndpointResponses), 10)
	assertBuiltinRule(t, created, builtinAlertRuleMessagesError, contract.AlertMetricErrorRate, string(gatewaycontract.EndpointMessages), 10)
	assertBuiltinRule(t, created, builtinAlertRuleResponsesWebSocketError, contract.AlertMetricErrorRate, "/v1/responses/ws", 5)
	assertBuiltinRule(t, created, builtinAlertRuleRealtimeTranscriptsError, contract.AlertMetricErrorRate, string(gatewaycontract.EndpointRealtime), 5)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleSchedulerNoAccount, "no_available_account", contract.AlertSeverityCritical, 5)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleProviderAuthFailure, "auth_failed", contract.AlertSeverityWarning, 5)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleProviderQuotaExhausted, "quota_exhausted", contract.AlertSeverityWarning, 5)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleProvider5xxSpike, "provider_5xx", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleRateLimitSpike, "rate_limit", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleTimeoutSpike, "timeout", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleNetworkErrorSpike, "network_error", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleInvalidResponseSpike, "invalid_response", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRulePolicyErrorSpike, "policy_error", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleUpstreamErrorSpike, "upstream_error", contract.AlertSeverityWarning, 10)
	assertBuiltinErrorClassRule(t, created, builtinAlertRuleOverloadedSpike, "overloaded", contract.AlertSeverityWarning, 10)

	again, err := svc.EnsureBuiltinAlertRules(t.Context())
	if err != nil {
		t.Fatalf("ensure builtins again: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent ensure to create no rules, got %+v", again)
	}
}

func TestEnsureBuiltinAlertRulesRespectsExistingOperatorRule(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	disabled := false
	existing, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            builtinAlertRuleGatewayErrorRate,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.9,
		Severity:        contract.AlertSeverityTicket,
		Enabled:         &disabled,
		WindowSeconds:   1800,
		MinRequestCount: 99,
	})
	if err != nil {
		t.Fatalf("create operator rule: %v", err)
	}

	created, err := svc.EnsureBuiltinAlertRules(t.Context())
	if err != nil {
		t.Fatalf("ensure builtins: %v", err)
	}
	if len(created) != len(builtinAlertRules)-1 {
		t.Fatalf("expected only missing builtins, got %+v", created)
	}
	found, err := svc.ListAlertRules(t.Context())
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	var preserved contract.AlertRule
	for _, rule := range found {
		if rule.ID == existing.ID {
			preserved = rule
			break
		}
	}
	if preserved.ID == 0 || preserved.Enabled || preserved.Threshold != 0.9 ||
		preserved.Severity != contract.AlertSeverityTicket ||
		preserved.MinRequestCount != 99 ||
		!preserved.BuiltinBaseline ||
		preserved.BaselineKey != "gateway.error_rate" {
		t.Fatalf("expected existing operator rule preserved, got %+v from %+v", preserved, found)
	}
}

func assertBuiltinRule(t *testing.T, rules []contract.AlertRule, name string, metric contract.AlertMetricType, endpoint string, minRequestCount int) {
	t.Helper()
	for _, rule := range rules {
		if rule.Name != name {
			continue
		}
		if rule.MetricType != metric || rule.Scope.SourceEndpoint != endpoint || rule.MinRequestCount != minRequestCount || !rule.Enabled {
			t.Fatalf("unexpected builtin rule %q: %+v", name, rule)
		}
		return
	}
	t.Fatalf("builtin rule %q not found in %+v", name, rules)
}

func assertBuiltinErrorClassRule(t *testing.T, rules []contract.AlertRule, name string, errorClass string, severity contract.AlertSeverity, threshold float64) {
	t.Helper()
	for _, rule := range rules {
		if rule.Name != name {
			continue
		}
		if rule.MetricType != contract.AlertMetricRequestCount ||
			rule.Scope.ErrorClass != errorClass ||
			rule.Severity != severity ||
			rule.Threshold != threshold ||
			rule.MinRequestCount != 1 ||
			!rule.Enabled {
			t.Fatalf("unexpected builtin error-class rule %q: %+v", name, rule)
		}
		return
	}
	t.Fatalf("builtin error-class rule %q not found in %+v", name, rules)
}

func TestEvaluateAlertRulesFiresUpdatesAndResolvesEvents(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	channel, err := svc.CreateNotificationChannel(t.Context(), contract.CreateNotificationChannelRequest{
		Name:            "Ops email",
		Type:            contract.NotificationChannelTypeEmail,
		MinSeverity:     contract.AlertSeverityWarning,
		EmailRecipients: []string{"OnCall@Example.COM", "oncall@example.com"},
	})
	if err != nil {
		t.Fatalf("create notification channel: %v", err)
	}
	if len(channel.EmailRecipients) != 1 || channel.EmailRecipients[0] != "oncall@example.com" {
		t.Fatalf("expected normalized channel recipients, got %+v", channel)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Chat error rate",
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.25,
		Severity:        contract.AlertSeverityCritical,
		WindowSeconds:   3600,
		MinRequestCount: 2,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: "/v1/chat/completions",
			Model:          "gpt-ops",
			ProviderID:     intPtr(7),
		},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Model: "gpt-ops", ProviderID: intPtr(7), Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_1", SourceEndpoint: "/v1/chat/completions", Model: "gpt-ops", ProviderID: intPtr(7), Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-3 * time.Minute)},
		{RequestID: "req_bad_2", SourceEndpoint: "/v1/chat/completions", Model: "gpt-ops", ProviderID: intPtr(7), Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-4 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 || result.Suppressed != 0 {
		t.Fatalf("unexpected first evaluation result: %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %+v", alerts)
	}
	if alerts[0].RuleID != "rule.1" || alerts[0].Status != contract.AlertStatusFiring || alerts[0].Severity != contract.AlertSeverityCritical {
		t.Fatalf("unexpected fired alert: %+v", alerts[0])
	}
	if alerts[0].Details["source_endpoint"] != "/v1/chat/completions" || alerts[0].Details["model"] != "gpt-ops" || alerts[0].Details["provider_id"] != 7 {
		t.Fatalf("expected alert rule scope in details, got %+v", alerts[0].Details)
	}
	if alerts[0].Details["window_start"] != now.Add(-time.Hour).Format(time.RFC3339) || alerts[0].Details["window_end"] != now.Format(time.RFC3339) {
		t.Fatalf("expected alert evaluation window in details, got %+v", alerts[0].Details)
	}
	deliveries, err := svc.ListNotificationDeliveries(t.Context(), contract.DeliveryListOptions{})
	if err != nil {
		t.Fatalf("list notification deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].ChannelID != channel.ID || deliveries[0].AlertEventID != alerts[0].ID || deliveries[0].Target != "oncall@example.com" || deliveries[0].Status != contract.NotificationDeliveryStatusPending {
		t.Fatalf("unexpected firing delivery: %+v", deliveries)
	}

	result, err = svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("reevaluate rules: %v", err)
	}
	if result.Created != 0 || result.Updated != 1 {
		t.Fatalf("expected existing alert update, got %+v", result)
	}
	deliveries, err = svc.ListNotificationDeliveries(t.Context(), contract.DeliveryListOptions{})
	if err != nil {
		t.Fatalf("list notification deliveries after update: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected firing delivery to be deduplicated, got %+v", deliveries)
	}

	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok_1", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_ok_2", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-3 * time.Minute)},
	}
	result, err = svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate recovery: %v", err)
	}
	if result.Breached != 0 || result.Resolved != 1 {
		t.Fatalf("expected resolution, got %+v", result)
	}
	deliveries, err = svc.ListNotificationDeliveries(t.Context(), contract.DeliveryListOptions{})
	if err != nil {
		t.Fatalf("list notification deliveries after resolve: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("expected firing and resolved deliveries, got %+v", deliveries)
	}
	var resolvedDelivery contract.NotificationDelivery
	for _, delivery := range deliveries {
		if delivery.AlertStatus == contract.AlertStatusResolved {
			resolvedDelivery = delivery
		}
	}
	if resolvedDelivery.ID == 0 || resolvedDelivery.Target != "oncall@example.com" {
		t.Fatalf("expected resolved delivery, got %+v", deliveries)
	}
}

func TestNotificationChannelValidation(t *testing.T) {
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateNotificationChannel(t.Context(), contract.CreateNotificationChannelRequest{
		Name:            "bad",
		Type:            contract.NotificationChannelTypeEmail,
		EmailRecipients: []string{"not-an-email"},
	}); err != ErrInvalidInput {
		t.Fatalf("expected invalid recipient rejection, got %v", err)
	}
	if _, err := svc.CreateNotificationChannel(t.Context(), contract.CreateNotificationChannelRequest{
		Name:            "unsupported",
		Type:            contract.NotificationChannelType("webhook"),
		EmailRecipients: []string{"ops@example.com"},
	}); err != ErrInvalidInput {
		t.Fatalf("expected unsupported channel rejection, got %v", err)
	}
}

func TestEvaluateAlertRulesSuppressesWithMatchingSilence(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Chat error rate",
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.25,
		Severity:        contract.AlertSeverityCritical,
		MinRequestCount: 2,
		Scope:           contract.AlertRuleScope{SourceEndpoint: "/v1/chat/completions"},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := svc.CreateAlertSilence(t.Context(), contract.CreateAlertSilenceRequest{
		Comment:  "maintenance",
		Matcher:  contract.AlertSilenceMatcher{RuleID: "rule.1"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create silence: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_1", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-3 * time.Minute)},
		{RequestID: "req_bad_2", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-4 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Breached != 1 || result.Created != 1 || result.Suppressed != 1 {
		t.Fatalf("expected suppressed creation, got %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one alert, got %+v", alerts)
	}
	if alerts[0].Status != contract.AlertStatusSuppressed || alerts[0].SuppressedBy == nil || *alerts[0].SuppressedBy != "silence.1" {
		t.Fatalf("expected suppressed alert with silence attribution, got %+v", alerts[0])
	}
}

func TestEvaluateAlertRulesFiltersByErrorClass(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Provider 5xx spike",
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       1,
		Severity:        contract.AlertSeverityWarning,
		WindowSeconds:   300,
		MinRequestCount: 1,
		Scope:           contract.AlertRuleScope{ErrorClass: "provider_5xx"},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", Success: true, CreatedAt: now.Add(-time.Minute)},
		{RequestID: "req_provider_1", Success: false, ErrorClass: ptrString("provider_5xx"), CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_provider_2", Success: false, ErrorClass: ptrString("provider_5xx"), CreatedAt: now.Add(-3 * time.Minute)},
		{RequestID: "req_timeout", Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-4 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 {
		t.Fatalf("unexpected evaluation result: %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Details["error_class"] != "provider_5xx" || alerts[0].Details["total_requests"] != 2 {
		t.Fatalf("expected scoped provider_5xx alert, got %+v", alerts)
	}
}

func TestEvaluateAlertRulesMatchesCanonicalErrorClassAliases(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Rate limit spike",
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       2,
		Severity:        contract.AlertSeverityWarning,
		WindowSeconds:   300,
		MinRequestCount: 1,
		Scope:           contract.AlertRuleScope{ErrorClass: "rate_limited"},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_rate_limit", Success: false, ErrorClass: ptrString("rate_limit"), CreatedAt: now.Add(-time.Minute)},
		{RequestID: "req_rate_limited", Success: false, ErrorClass: ptrString("rate_limited"), CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_rate_limit_error", Success: false, ErrorClass: ptrString("rate_limit_error"), CreatedAt: now.Add(-3 * time.Minute)},
		{RequestID: "req_timeout", Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-4 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 {
		t.Fatalf("unexpected evaluation result: %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Details["error_class"] != "rate_limit" || alerts[0].Details["total_requests"] != 3 {
		t.Fatalf("expected canonical rate_limit alert, got %+v", alerts)
	}
}

func TestEvaluateAlertRulesSuppressesWithCanonicalErrorClassAlias(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertRule(t.Context(), contract.CreateAlertRuleRequest{
		Name:            "Auth failures",
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       1,
		Severity:        contract.AlertSeverityWarning,
		WindowSeconds:   300,
		MinRequestCount: 1,
		Scope:           contract.AlertRuleScope{ErrorClass: "auth_error"},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := svc.CreateAlertSilence(t.Context(), contract.CreateAlertSilenceRequest{
		Comment:  "credential rotation",
		Matcher:  contract.AlertSilenceMatcher{ErrorClass: "credential_error"},
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create silence: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_auth_failed", Success: false, ErrorClass: ptrString("auth_failed"), CreatedAt: now.Add(-time.Minute)},
		{RequestID: "req_auth_error", Success: false, ErrorClass: ptrString("auth_error"), CreatedAt: now.Add(-2 * time.Minute)},
	}

	result, err := svc.EvaluateAlertRules(t.Context())
	if err != nil {
		t.Fatalf("evaluate rules: %v", err)
	}
	if result.Breached != 1 || result.Created != 1 || result.Suppressed != 1 {
		t.Fatalf("expected alias-matched silence, got %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Status != contract.AlertStatusSuppressed || alerts[0].Details["error_class"] != "auth_failed" {
		t.Fatalf("expected suppressed canonical auth alert, got %+v", alerts)
	}
}

func TestCreateAlertSilenceRejectsInvalidWindow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateAlertSilence(t.Context(), contract.CreateAlertSilenceRequest{
		StartsAt: now,
		EndsAt:   now.Add(-time.Minute),
	}); err != ErrInvalidInput {
		t.Fatalf("expected invalid input for inverted window, got %v", err)
	}
}

func metricPtr(v contract.AlertMetricType) *contract.AlertMetricType { return &v }
func operatorPtr(v contract.AlertOperator) *contract.AlertOperator   { return &v }
func severityPtr(v contract.AlertSeverity) *contract.AlertSeverity   { return &v }
func floatPtr(v float64) *float64                                    { return &v }
func intPtr(v int) *int                                              { return &v }
