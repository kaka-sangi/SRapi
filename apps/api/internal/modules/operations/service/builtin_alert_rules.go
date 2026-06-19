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
)

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
		created = append(created, stored)
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
}

func normalizedRuleName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
