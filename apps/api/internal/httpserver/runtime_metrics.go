package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	lines := s.runtime.metricsLines(r.Context())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

func (rt *runtimeState) metricsLines(ctx context.Context) []string {
	lines := []string{
		"# HELP srapi_gateway_requests_total Gateway requests recorded by endpoint family, model, provider protocol, and result.",
		"# TYPE srapi_gateway_requests_total counter",
		"# HELP srapi_gateway_request_duration_seconds Gateway request latency summary derived from usage logs.",
		"# TYPE srapi_gateway_request_duration_seconds summary",
		"# HELP srapi_gateway_inflight_requests Gateway requests with pending scheduler leases.",
		"# TYPE srapi_gateway_inflight_requests gauge",
		"# HELP srapi_realtime_active_slots Active realtime WebSocket slots.",
		"# TYPE srapi_realtime_active_slots gauge",
		"# HELP srapi_realtime_active_slots_by_endpoint Active realtime WebSocket slots by source endpoint.",
		"# TYPE srapi_realtime_active_slots_by_endpoint gauge",
		"# HELP srapi_realtime_slots_total Realtime WebSocket slot lifecycle events.",
		"# TYPE srapi_realtime_slots_total counter",
		"# HELP srapi_gateway_errors_total Gateway request errors recorded by error class.",
		"# TYPE srapi_gateway_errors_total counter",
		"# HELP srapi_scheduler_decisions_total Scheduler decisions by strategy and outcome.",
		"# TYPE srapi_scheduler_decisions_total counter",
		"# HELP srapi_provider_errors_total Provider-facing errors recorded by protocol and error class.",
		"# TYPE srapi_provider_errors_total counter",
		"# HELP srapi_usage_tokens_total Usage tokens by model, provider protocol, and token kind.",
		"# TYPE srapi_usage_tokens_total counter",
		"# HELP srapi_reverse_proxy_ban_signals_total Reverse proxy ban signals observed by risk class.",
		"# TYPE srapi_reverse_proxy_ban_signals_total counter",
		"# HELP reverse_proxy_request_total Reverse proxy requests.",
		"# TYPE reverse_proxy_request_total counter",
		"# HELP reverse_proxy_request_success_total Reverse proxy successful requests.",
		"# TYPE reverse_proxy_request_success_total counter",
	}

	usageLogs, usageErr := rt.usage.List(ctx)
	if usageErr == nil {
		lines = append(lines, gatewayUsageMetricLines(usageLogs)...)
		lines = append(lines, providerErrorMetricLines(usageLogs)...)
		lines = append(lines, usageTokenMetricLines(usageLogs)...)
	} else {
		rt.logger.Warn("failed to collect usage metrics", "error", usageErr)
	}

	decisions, decisionErr := rt.scheduler.ListDecisions(ctx)
	if decisionErr == nil {
		lines = append(lines, schedulerDecisionMetricLines(decisions)...)
	} else {
		rt.logger.Warn("failed to collect scheduler decision metrics", "error", decisionErr)
	}

	leases, leaseErr := rt.scheduler.ListLeases(ctx)
	if leaseErr == nil {
		lines = append(lines, fmt.Sprintf("srapi_gateway_inflight_requests %d", pendingLeaseCount(leases)))
	} else {
		rt.logger.Warn("failed to collect scheduler lease metrics", "error", leaseErr)
		lines = append(lines, "srapi_gateway_inflight_requests 0")
	}
	realtimeSnapshot := rt.realtime.Snapshot(ctx)
	lines = append(lines,
		fmt.Sprintf("srapi_realtime_active_slots %d", realtimeSnapshot.ActiveSlots),
		fmt.Sprintf(`srapi_realtime_slots_total{event="acquired"} %d`, realtimeSnapshot.AcquiredTotal),
		fmt.Sprintf(`srapi_realtime_slots_total{event="released"} %d`, realtimeSnapshot.ReleasedTotal),
		fmt.Sprintf(`srapi_realtime_slots_total{event="rejected"} %d`, realtimeSnapshot.RejectedTotal),
	)
	for endpoint, count := range realtimeSnapshot.ActiveByEndpoint {
		lines = append(lines, fmt.Sprintf("srapi_realtime_active_slots_by_endpoint{source_endpoint=%q} %d", endpoint, count))
	}

	metrics := rt.reverseProxy.Metrics()
	lines = append(lines,
		fmt.Sprintf(`srapi_reverse_proxy_ban_signals_total{risk_class="account_locked"} %d`, metrics.AccountLockedTotal),
		fmt.Sprintf(`srapi_reverse_proxy_ban_signals_total{risk_class="account_banned"} %d`, metrics.AccountBannedTotal),
		fmt.Sprintf("reverse_proxy_request_total %d", metrics.RequestTotal),
		fmt.Sprintf("reverse_proxy_request_success_total %d", metrics.RequestSuccessTotal),
	)
	for class, count := range metrics.RequestErrorTotal {
		lines = append(lines, fmt.Sprintf("reverse_proxy_request_error_total{error_class=%q} %d", class, count))
	}
	for class, count := range metrics.ChallengeTotal {
		lines = append(lines, fmt.Sprintf("reverse_proxy_challenge_total{strategy=%q} %d", class, count))
	}
	lines = append(lines,
		fmt.Sprintf("reverse_proxy_account_locked_total %d", metrics.AccountLockedTotal),
		fmt.Sprintf("reverse_proxy_account_banned_total %d", metrics.AccountBannedTotal),
	)
	for status, count := range metrics.OAuthRefreshTotal {
		lines = append(lines, fmt.Sprintf("reverse_proxy_oauth_refresh_total{status=%q} %d", status, count))
	}
	lines = appendZeroValueBaselineMetrics(lines)
	sortMetricLines(lines)
	return lines
}

func gatewayUsageMetricLines(logs []usagecontract.UsageLog) []string {
	type aggregate struct {
		count     int
		latencyMS int
	}
	requests := map[string]*aggregate{}
	errors := map[string]int{}
	for _, log := range logs {
		result := "success"
		if !log.Success {
			result = "error"
		}
		key := strings.Join([]string{
			endpointFamily(log.SourceEndpoint),
			metricLabelValue(log.Model, "unknown"),
			metricLabelValue(log.TargetProtocol, "unknown"),
			result,
		}, "\xff")
		if requests[key] == nil {
			requests[key] = &aggregate{}
		}
		requests[key].count++
		requests[key].latencyMS += log.LatencyMS
		if !log.Success {
			errorClass := metricLabelValue(derefString(log.ErrorClass), "unknown")
			errors[errorClass]++
		}
	}
	keys := sortedKeys(requests)
	lines := make([]string, 0, len(keys)*3+len(errors))
	for _, key := range keys {
		parts := strings.Split(key, "\xff")
		value := requests[key]
		labels := fmt.Sprintf(`endpoint_family=%q,model=%q,provider_protocol=%q,result=%q`, parts[0], parts[1], parts[2], parts[3])
		lines = append(lines,
			fmt.Sprintf("srapi_gateway_requests_total{%s} %d", labels, value.count),
			fmt.Sprintf("srapi_gateway_request_duration_seconds_count{%s} %d", labels, value.count),
			fmt.Sprintf("srapi_gateway_request_duration_seconds_sum{%s} %.6f", labels, float64(value.latencyMS)/1000),
		)
	}
	for _, errorClass := range sortedIntKeys(errors) {
		lines = append(lines, fmt.Sprintf("srapi_gateway_errors_total{error_class=%q} %d", errorClass, errors[errorClass]))
	}
	return lines
}

func schedulerDecisionMetricLines(decisions []schedulercontract.Decision) []string {
	counts := map[string]int{}
	for _, decision := range decisions {
		outcome := "selected"
		reason := "selected"
		if decision.SelectedAccountID == nil {
			outcome = "rejected"
			reason = primaryRejectReason(decision.RejectReasons)
		}
		key := strings.Join([]string{string(decision.Strategy), outcome, reason}, "\xff")
		counts[key]++
	}
	lines := make([]string, 0, len(counts))
	for _, key := range sortedIntKeys(counts) {
		parts := strings.Split(key, "\xff")
		lines = append(lines, fmt.Sprintf("srapi_scheduler_decisions_total{strategy=%q,outcome=%q,reason=%q} %d", parts[0], parts[1], parts[2], counts[key]))
	}
	return lines
}

func providerErrorMetricLines(logs []usagecontract.UsageLog) []string {
	counts := map[string]int{}
	for _, log := range logs {
		if log.Success || log.ProviderID == nil {
			continue
		}
		protocol := metricLabelValue(log.TargetProtocol, "unknown")
		errorClass := metricLabelValue(derefString(log.ErrorClass), "unknown")
		counts[strings.Join([]string{protocol, errorClass}, "\xff")]++
	}
	lines := make([]string, 0, len(counts))
	for _, key := range sortedIntKeys(counts) {
		parts := strings.Split(key, "\xff")
		lines = append(lines, fmt.Sprintf("srapi_provider_errors_total{provider_protocol=%q,error_class=%q} %d", parts[0], parts[1], counts[key]))
	}
	return lines
}

func usageTokenMetricLines(logs []usagecontract.UsageLog) []string {
	counts := map[string]int{}
	for _, log := range logs {
		model := metricLabelValue(log.Model, "unknown")
		protocol := metricLabelValue(log.TargetProtocol, "unknown")
		counts[strings.Join([]string{model, protocol, "input"}, "\xff")] += log.InputTokens
		counts[strings.Join([]string{model, protocol, "output"}, "\xff")] += log.OutputTokens
		counts[strings.Join([]string{model, protocol, "cached"}, "\xff")] += log.CachedTokens
	}
	lines := make([]string, 0, len(counts))
	for _, key := range sortedIntKeys(counts) {
		parts := strings.Split(key, "\xff")
		lines = append(lines, fmt.Sprintf("srapi_usage_tokens_total{model=%q,provider_protocol=%q,token_kind=%q} %d", parts[0], parts[1], parts[2], counts[key]))
	}
	return lines
}

func pendingLeaseCount(leases []schedulercontract.Lease) int {
	count := 0
	for _, lease := range leases {
		if lease.Status == schedulercontract.LeaseStatusPending {
			count++
		}
	}
	return count
}

func endpointFamily(endpoint string) string {
	switch {
	case strings.Contains(endpoint, ":generateContent") || strings.Contains(endpoint, ":streamGenerateContent"):
		return "gemini_generate_content"
	case strings.Contains(endpoint, "/responses"):
		return "responses"
	case strings.Contains(endpoint, "/messages"):
		return "messages"
	case strings.Contains(endpoint, "/chat/completions"):
		return "chat_completions"
	case strings.TrimSpace(endpoint) == "":
		return "unknown"
	default:
		return strings.Trim(strings.ReplaceAll(endpoint, "/", "_"), "_")
	}
}

func primaryRejectReason(reasons map[string]any) string {
	if len(reasons) == 0 {
		return "no_candidate"
	}
	keys := make([]string, 0, len(reasons))
	for key := range reasons {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if reason, ok := reasons[key].(string); ok && strings.TrimSpace(reason) != "" {
			return metricLabelValue(reason, "rejected")
		}
	}
	return "rejected"
}

func metricLabelValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sortedIntKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortMetricLines(lines []string) {
	if len(lines) == 0 {
		return
	}
	headerEnd := 0
	for headerEnd < len(lines) && strings.HasPrefix(lines[headerEnd], "#") {
		headerEnd++
	}
	sort.Strings(lines[headerEnd:])
}

func appendZeroValueBaselineMetrics(lines []string) []string {
	for _, sample := range []string{
		`srapi_gateway_requests_total{endpoint_family="unknown",model="unknown",provider_protocol="unknown",result="success"} 0`,
		`srapi_gateway_request_duration_seconds_count{endpoint_family="unknown",model="unknown",provider_protocol="unknown",result="success"} 0`,
		`srapi_gateway_request_duration_seconds_sum{endpoint_family="unknown",model="unknown",provider_protocol="unknown",result="success"} 0.000000`,
		`srapi_gateway_inflight_requests 0`,
		`srapi_realtime_active_slots 0`,
		`srapi_realtime_slots_total{event="acquired"} 0`,
		`srapi_gateway_errors_total{error_class="unknown"} 0`,
		`srapi_scheduler_decisions_total{strategy="unknown",outcome="selected",reason="selected"} 0`,
		`srapi_provider_errors_total{provider_protocol="unknown",error_class="unknown"} 0`,
		`srapi_usage_tokens_total{model="unknown",provider_protocol="unknown",token_kind="input"} 0`,
		`srapi_reverse_proxy_ban_signals_total{risk_class="account_locked"} 0`,
		`srapi_reverse_proxy_ban_signals_total{risk_class="account_banned"} 0`,
	} {
		if !hasMetricName(lines, metricName(sample)) {
			lines = append(lines, sample)
		}
	}
	return lines
}

func hasMetricName(lines []string, name string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"{") {
			return true
		}
	}
	return false
}

func metricName(sample string) string {
	if idx := strings.IndexAny(sample, " {"); idx > 0 {
		return sample[:idx]
	}
	return sample
}
