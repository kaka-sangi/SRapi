package service

import (
	"context"
	"strings"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

const (
	builtinAlertRuleGatewayErrorRate         = "SRapi Gateway error rate baseline"
	builtinAlertRuleGatewayLatencyP95        = "SRapi Gateway p95 latency baseline"
	builtinAlertRuleChatCompletionsError     = "SRapi Chat Completions error rate baseline"
	builtinAlertRuleChatCompletionsLatency   = "SRapi Chat Completions p95 latency baseline"
	builtinAlertRuleResponsesError           = "SRapi Responses error rate baseline"
	builtinAlertRuleMessagesError            = "SRapi Messages error rate baseline"
	builtinAlertRuleResponsesWebSocketError  = "SRapi Responses WebSocket error rate baseline"
	builtinAlertRuleRealtimeTranscriptsError = "SRapi Realtime Transcripts error rate baseline"
	builtinAlertRuleSchedulerNoAccount       = "SRapi Scheduler no available account baseline"
	builtinAlertRuleProviderAuthFailure      = "SRapi Provider auth failure baseline"
	builtinAlertRuleProviderQuotaExhausted   = "SRapi Provider quota exhausted baseline"
	builtinAlertRuleProvider5xxSpike         = "SRapi Provider 5xx error baseline"
	builtinAlertRuleRateLimitSpike           = "SRapi Provider rate limit baseline"
	builtinAlertRuleTimeoutSpike             = "SRapi Provider timeout baseline"
	builtinAlertRuleNetworkErrorSpike        = "SRapi Provider network error baseline"
	builtinAlertRuleInvalidResponseSpike     = "SRapi Provider invalid response baseline"
	builtinAlertRulePolicyErrorSpike         = "SRapi Provider policy error baseline"
	builtinAlertRuleUpstreamErrorSpike       = "SRapi Provider upstream error baseline"
	builtinAlertRuleOverloadedSpike          = "SRapi Provider overloaded baseline"
)

var builtinAlertRuleKeys = map[string]string{
	normalizedRuleName(builtinAlertRuleGatewayErrorRate):         "gateway.error_rate",
	normalizedRuleName(builtinAlertRuleGatewayLatencyP95):        "gateway.latency_p95",
	normalizedRuleName(builtinAlertRuleChatCompletionsError):     "gateway.chat_completions.error_rate",
	normalizedRuleName(builtinAlertRuleChatCompletionsLatency):   "gateway.chat_completions.latency_p95",
	normalizedRuleName(builtinAlertRuleResponsesError):           "gateway.responses.error_rate",
	normalizedRuleName(builtinAlertRuleMessagesError):            "gateway.messages.error_rate",
	normalizedRuleName(builtinAlertRuleResponsesWebSocketError):  "gateway.responses_ws.error_rate",
	normalizedRuleName(builtinAlertRuleRealtimeTranscriptsError): "gateway.realtime.error_rate",
	normalizedRuleName(builtinAlertRuleSchedulerNoAccount):       "scheduler.no_available_account",
	normalizedRuleName(builtinAlertRuleProviderAuthFailure):      "provider.auth_failed",
	normalizedRuleName(builtinAlertRuleProviderQuotaExhausted):   "provider.quota_exhausted",
	normalizedRuleName(builtinAlertRuleProvider5xxSpike):         "provider.provider_5xx",
	normalizedRuleName(builtinAlertRuleRateLimitSpike):           "provider.rate_limit",
	normalizedRuleName(builtinAlertRuleTimeoutSpike):             "provider.timeout",
	normalizedRuleName(builtinAlertRuleNetworkErrorSpike):        "provider.network_error",
	normalizedRuleName(builtinAlertRuleInvalidResponseSpike):     "provider.invalid_response",
	normalizedRuleName(builtinAlertRulePolicyErrorSpike):         "provider.policy_error",
	normalizedRuleName(builtinAlertRuleUpstreamErrorSpike):       "provider.upstream_error",
	normalizedRuleName(builtinAlertRuleOverloadedSpike):          "provider.overloaded",
}

// EnsureBuiltinAlertRules creates missing baseline Ops alert rules without
// overwriting operator-edited rules that already use the same stable names.
func (s *Service) EnsureBuiltinAlertRules(ctx context.Context) ([]contract.AlertRule, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	existing, err := s.observabilityStore.ListAlertRules(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{}, len(existing))
	for _, rule := range existing {
		names[normalizedRuleName(rule.Name)] = struct{}{}
	}

	now := s.clock.Now()
	created := make([]contract.AlertRule, 0, len(builtinAlertRules))
	for _, rule := range builtinAlertRules {
		if _, ok := names[normalizedRuleName(rule.Name)]; ok {
			continue
		}
		rule.CreatedAt = now
		rule.UpdatedAt = now
		applyAlertRuleDefaults(&rule)
		if err := validateAlertRule(rule); err != nil {
			return nil, err
		}
		stored, err := s.observabilityStore.CreateAlertRule(ctx, rule)
		if err != nil {
			return nil, err
		}
		created = append(created, cloneAlertRule(stored))
		names[normalizedRuleName(rule.Name)] = struct{}{}
	}
	return created, nil
}

var builtinAlertRules = []contract.AlertRule{
	{
		Name:            builtinAlertRuleGatewayErrorRate,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.05,
		Severity:        contract.AlertSeverityCritical,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 20,
	},
	{
		Name:            builtinAlertRuleGatewayLatencyP95,
		MetricType:      contract.AlertMetricLatencyP95,
		Operator:        contract.AlertOperatorGT,
		Threshold:       15000,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   10 * 60,
		CooldownSeconds: 15 * 60,
		MinRequestCount: 20,
	},
	{
		Name:            builtinAlertRuleChatCompletionsError,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.1,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 10,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: string(gatewaycontract.EndpointChatCompletions),
		},
	},
	{
		Name:            builtinAlertRuleChatCompletionsLatency,
		MetricType:      contract.AlertMetricLatencyP95,
		Operator:        contract.AlertOperatorGT,
		Threshold:       20000,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   10 * 60,
		CooldownSeconds: 15 * 60,
		MinRequestCount: 10,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: string(gatewaycontract.EndpointChatCompletions),
		},
	},
	{
		Name:            builtinAlertRuleResponsesError,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.1,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 10,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: string(gatewaycontract.EndpointResponses),
		},
	},
	{
		Name:            builtinAlertRuleMessagesError,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.1,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 10,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: string(gatewaycontract.EndpointMessages),
		},
	},
	{
		Name:            builtinAlertRuleResponsesWebSocketError,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.1,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 5,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: "/v1/responses/ws",
		},
	},
	{
		Name:            builtinAlertRuleRealtimeTranscriptsError,
		MetricType:      contract.AlertMetricErrorRate,
		Operator:        contract.AlertOperatorGT,
		Threshold:       0.1,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 5,
		Scope: contract.AlertRuleScope{
			SourceEndpoint: string(gatewaycontract.EndpointRealtime),
		},
	},
	{
		Name:            builtinAlertRuleSchedulerNoAccount,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       5,
		Severity:        contract.AlertSeverityCritical,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "no_available_account",
		},
	},
	{
		Name:            builtinAlertRuleProviderAuthFailure,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       5,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "auth_failed",
		},
	},
	{
		Name:            builtinAlertRuleProviderQuotaExhausted,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       5,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "quota_exhausted",
		},
	},
	{
		Name:            builtinAlertRuleProvider5xxSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "provider_5xx",
		},
	},
	{
		Name:            builtinAlertRuleRateLimitSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "rate_limit",
		},
	},
	{
		Name:            builtinAlertRuleTimeoutSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "timeout",
		},
	},
	{
		Name:            builtinAlertRuleNetworkErrorSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "network_error",
		},
	},
	{
		Name:            builtinAlertRuleInvalidResponseSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "invalid_response",
		},
	},
	{
		Name:            builtinAlertRulePolicyErrorSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "policy_error",
		},
	},
	{
		Name:            builtinAlertRuleUpstreamErrorSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "upstream_error",
		},
	},
	{
		Name:            builtinAlertRuleOverloadedSpike,
		MetricType:      contract.AlertMetricRequestCount,
		Operator:        contract.AlertOperatorGT,
		Threshold:       10,
		Severity:        contract.AlertSeverityWarning,
		Enabled:         true,
		WindowSeconds:   5 * 60,
		CooldownSeconds: 10 * 60,
		MinRequestCount: 1,
		Scope: contract.AlertRuleScope{
			ErrorClass: "overloaded",
		},
	},
}

func normalizedRuleName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func builtinAlertRuleKey(name string) string {
	return builtinAlertRuleKeys[normalizedRuleName(name)]
}
