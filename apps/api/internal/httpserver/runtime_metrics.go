package httpserver

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(newRuntimeMetricsCollector(r.Context(), s.runtime)); err != nil {
		s.logger.Error("failed to register metrics collector", "error", err)
		http.Error(w, "failed to register metrics collector", http.StatusInternalServerError)
		return
	}
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{ErrorHandling: promhttp.HTTPErrorOnError}).ServeHTTP(w, r)
}

type runtimeMetricsCollector struct {
	ctx   context.Context
	rt    *runtimeState
	descs runtimeMetricDescs
}

type runtimeMetricDescs struct {
	gatewayRequests               *prometheus.Desc
	gatewayDuration               *prometheus.Desc
	gatewayInflight               *prometheus.Desc
	realtimeActiveSlots           *prometheus.Desc
	realtimeActiveSlotsByEndpoint *prometheus.Desc
	realtimeSlots                 *prometheus.Desc
	gatewayErrors                 *prometheus.Desc
	gatewayFailover               *prometheus.Desc
	schedulerDecisions            *prometheus.Desc
	providerErrors                *prometheus.Desc
	providerProbeLatency          *prometheus.Desc
	usageTokens                   *prometheus.Desc
	reverseProxyBanSignals        *prometheus.Desc
	reverseProxyRequests          *prometheus.Desc
	reverseProxyRequestSuccesses  *prometheus.Desc
	reverseProxyRequestErrors     *prometheus.Desc
	reverseProxyChallenges        *prometheus.Desc
	reverseProxyAccountLocked     *prometheus.Desc
	reverseProxyAccountBanned     *prometheus.Desc
	reverseProxyOAuthRefreshes    *prometheus.Desc
}

func newRuntimeMetricsCollector(ctx context.Context, rt *runtimeState) *runtimeMetricsCollector {
	return &runtimeMetricsCollector{
		ctx: ctx,
		rt:  rt,
		descs: runtimeMetricDescs{
			gatewayRequests: prometheus.NewDesc(
				"srapi_gateway_requests_total",
				"Gateway requests recorded by endpoint family, model, provider protocol, and result.",
				[]string{"endpoint_family", "model", "provider_protocol", "result"},
				nil,
			),
			gatewayDuration: prometheus.NewDesc(
				"srapi_gateway_request_duration_seconds",
				"Gateway request latency histogram derived from usage logs.",
				[]string{"endpoint_family", "model", "provider_protocol", "result"},
				nil,
			),
			gatewayInflight: prometheus.NewDesc(
				"srapi_gateway_inflight_requests",
				"Gateway requests with pending scheduler leases.",
				nil,
				nil,
			),
			realtimeActiveSlots: prometheus.NewDesc(
				"srapi_realtime_active_slots",
				"Active realtime WebSocket slots.",
				nil,
				nil,
			),
			realtimeActiveSlotsByEndpoint: prometheus.NewDesc(
				"srapi_realtime_active_slots_by_endpoint",
				"Active realtime WebSocket slots by source endpoint.",
				[]string{"source_endpoint"},
				nil,
			),
			realtimeSlots: prometheus.NewDesc(
				"srapi_realtime_slots_total",
				"Realtime WebSocket slot lifecycle events.",
				[]string{"event"},
				nil,
			),
			gatewayErrors: prometheus.NewDesc(
				"srapi_gateway_errors_total",
				"Gateway request errors recorded by error class.",
				[]string{"error_class"},
				nil,
			),
			gatewayFailover: prometheus.NewDesc(
				"srapi_gateway_failover_total",
				"Gateway fallback attempts by endpoint family, model, provider protocol, and result.",
				[]string{"endpoint_family", "model", "provider_protocol", "result"},
				nil,
			),
			schedulerDecisions: prometheus.NewDesc(
				"srapi_scheduler_decisions_total",
				"Scheduler decisions by strategy and outcome.",
				[]string{"strategy", "outcome", "reason"},
				nil,
			),
			providerErrors: prometheus.NewDesc(
				"srapi_provider_errors_total",
				"Provider-facing errors recorded by protocol and error class.",
				[]string{"provider_protocol", "error_class"},
				nil,
			),
			providerProbeLatency: prometheus.NewDesc(
				"srapi_provider_probe_latency_seconds",
				"Provider account probe latency histogram derived from latest health snapshots.",
				[]string{"provider_protocol", "status"},
				nil,
			),
			usageTokens: prometheus.NewDesc(
				"srapi_usage_tokens_total",
				"Usage tokens by model, provider protocol, and token kind.",
				[]string{"model", "provider_protocol", "token_kind"},
				nil,
			),
			reverseProxyBanSignals: prometheus.NewDesc(
				"srapi_reverse_proxy_ban_signals_total",
				"Reverse proxy ban signals observed by risk class.",
				[]string{"risk_class"},
				nil,
			),
			reverseProxyRequests: prometheus.NewDesc("reverse_proxy_request_total", "Reverse proxy requests.", nil, nil),
			reverseProxyRequestSuccesses: prometheus.NewDesc(
				"reverse_proxy_request_success_total",
				"Reverse proxy successful requests.",
				nil,
				nil,
			),
			reverseProxyRequestErrors: prometheus.NewDesc(
				"reverse_proxy_request_error_total",
				"Reverse proxy request errors by class.",
				[]string{"error_class"},
				nil,
			),
			reverseProxyChallenges: prometheus.NewDesc(
				"reverse_proxy_challenge_total",
				"Reverse proxy challenges by strategy.",
				[]string{"strategy"},
				nil,
			),
			reverseProxyAccountLocked: prometheus.NewDesc("reverse_proxy_account_locked_total", "Reverse proxy account locked events.", nil, nil),
			reverseProxyAccountBanned: prometheus.NewDesc("reverse_proxy_account_banned_total", "Reverse proxy account banned events.", nil, nil),
			reverseProxyOAuthRefreshes: prometheus.NewDesc(
				"reverse_proxy_oauth_refresh_total",
				"Reverse proxy OAuth refresh attempts by status.",
				[]string{"status"},
				nil,
			),
		},
	}
}

func (c *runtimeMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descs.all() {
		ch <- desc
	}
}

func (d runtimeMetricDescs) all() []*prometheus.Desc {
	return []*prometheus.Desc{
		d.gatewayRequests,
		d.gatewayDuration,
		d.gatewayInflight,
		d.realtimeActiveSlots,
		d.realtimeActiveSlotsByEndpoint,
		d.realtimeSlots,
		d.gatewayErrors,
		d.gatewayFailover,
		d.schedulerDecisions,
		d.providerErrors,
		d.providerProbeLatency,
		d.usageTokens,
		d.reverseProxyBanSignals,
		d.reverseProxyRequests,
		d.reverseProxyRequestSuccesses,
		d.reverseProxyRequestErrors,
		d.reverseProxyChallenges,
		d.reverseProxyAccountLocked,
		d.reverseProxyAccountBanned,
		d.reverseProxyOAuthRefreshes,
	}
}

func (c *runtimeMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	emitted := map[string]bool{}
	c.collectUsageMetrics(ch, emitted)
	c.collectSchedulerMetrics(ch, emitted)
	c.collectRealtimeMetrics(ch, emitted)
	c.collectReverseProxyMetrics(ch, emitted)
	c.collectProviderProbeMetrics(ch, emitted)
	c.collectBaselineMetrics(ch, emitted)
}

func (c *runtimeMetricsCollector) collectUsageMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	logs, err := c.rt.usage.List(c.ctx)
	if err != nil {
		c.rt.logger.Warn("failed to collect usage metrics", "error", err)
		return
	}
	requests, failovers, providerErrors, gatewayErrors, tokenCounts := aggregateUsageMetrics(logs)
	for _, key := range sortedKeys(requests) {
		labels := strings.Split(key, "\xff")
		value := requests[key]
		emitConstMetric(ch, emitted, "srapi_gateway_requests_total", c.descs.gatewayRequests, prometheus.CounterValue, float64(value.count), labels...)
		emitConstHistogram(ch, emitted, "srapi_gateway_request_duration_seconds", c.descs.gatewayDuration, value.count, float64(value.latencyMS)/1000, value.buckets, labels...)
	}
	for _, key := range sortedKeys(failovers) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_gateway_failover_total", c.descs.gatewayFailover, prometheus.CounterValue, float64(failovers[key]), labels...)
	}
	for _, key := range sortedKeys(providerErrors) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_provider_errors_total", c.descs.providerErrors, prometheus.CounterValue, float64(providerErrors[key]), labels...)
	}
	for _, key := range sortedKeys(gatewayErrors) {
		emitConstMetric(ch, emitted, "srapi_gateway_errors_total", c.descs.gatewayErrors, prometheus.CounterValue, float64(gatewayErrors[key]), key)
	}
	for _, key := range sortedKeys(tokenCounts) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_usage_tokens_total", c.descs.usageTokens, prometheus.CounterValue, float64(tokenCounts[key]), labels...)
	}
}

func (c *runtimeMetricsCollector) collectSchedulerMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	decisions, err := c.rt.scheduler.ListDecisions(c.ctx)
	if err != nil {
		c.rt.logger.Warn("failed to collect scheduler decision metrics", "error", err)
	} else {
		counts := schedulerDecisionCounts(decisions)
		for _, key := range sortedKeys(counts) {
			labels := strings.Split(key, "\xff")
			emitConstMetric(ch, emitted, "srapi_scheduler_decisions_total", c.descs.schedulerDecisions, prometheus.CounterValue, float64(counts[key]), labels...)
		}
	}
	leases, err := c.rt.scheduler.ListLeases(c.ctx)
	if err != nil {
		c.rt.logger.Warn("failed to collect scheduler lease metrics", "error", err)
		emitConstMetric(ch, emitted, "srapi_gateway_inflight_requests", c.descs.gatewayInflight, prometheus.GaugeValue, 0)
		return
	}
	emitConstMetric(ch, emitted, "srapi_gateway_inflight_requests", c.descs.gatewayInflight, prometheus.GaugeValue, float64(pendingLeaseCount(leases)))
}

func (c *runtimeMetricsCollector) collectRealtimeMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	snapshot, err := c.rt.realtime.Snapshot(c.ctx)
	if err != nil {
		c.rt.logger.Warn("failed to collect realtime slot metrics", "error", err)
		snapshot = realtimeEmptySnapshot()
	}
	emitConstMetric(ch, emitted, "srapi_realtime_active_slots", c.descs.realtimeActiveSlots, prometheus.GaugeValue, float64(snapshot.ActiveSlots))
	for _, event := range []struct {
		name  string
		count int
	}{
		{name: "acquired", count: snapshot.AcquiredTotal},
		{name: "released", count: snapshot.ReleasedTotal},
		{name: "rejected", count: snapshot.RejectedTotal},
	} {
		emitConstMetric(ch, emitted, "srapi_realtime_slots_total", c.descs.realtimeSlots, prometheus.CounterValue, float64(event.count), event.name)
	}
	for _, endpoint := range sortedKeys(snapshot.ActiveByEndpoint) {
		emitConstMetric(ch, emitted, "srapi_realtime_active_slots_by_endpoint", c.descs.realtimeActiveSlotsByEndpoint, prometheus.GaugeValue, float64(snapshot.ActiveByEndpoint[endpoint]), endpoint)
	}
}

func (c *runtimeMetricsCollector) collectReverseProxyMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	metrics := c.rt.reverseProxy.Metrics()
	emitConstMetric(ch, emitted, "srapi_reverse_proxy_ban_signals_total", c.descs.reverseProxyBanSignals, prometheus.CounterValue, float64(metrics.AccountLockedTotal), "account_locked")
	emitConstMetric(ch, emitted, "srapi_reverse_proxy_ban_signals_total", c.descs.reverseProxyBanSignals, prometheus.CounterValue, float64(metrics.AccountBannedTotal), "account_banned")
	emitConstMetric(ch, emitted, "reverse_proxy_request_total", c.descs.reverseProxyRequests, prometheus.CounterValue, float64(metrics.RequestTotal))
	emitConstMetric(ch, emitted, "reverse_proxy_request_success_total", c.descs.reverseProxyRequestSuccesses, prometheus.CounterValue, float64(metrics.RequestSuccessTotal))
	for _, class := range sortedKeys(metrics.RequestErrorTotal) {
		emitConstMetric(ch, emitted, "reverse_proxy_request_error_total", c.descs.reverseProxyRequestErrors, prometheus.CounterValue, float64(metrics.RequestErrorTotal[class]), class)
	}
	for _, strategy := range sortedKeys(metrics.ChallengeTotal) {
		emitConstMetric(ch, emitted, "reverse_proxy_challenge_total", c.descs.reverseProxyChallenges, prometheus.CounterValue, float64(metrics.ChallengeTotal[strategy]), strategy)
	}
	emitConstMetric(ch, emitted, "reverse_proxy_account_locked_total", c.descs.reverseProxyAccountLocked, prometheus.CounterValue, float64(metrics.AccountLockedTotal))
	emitConstMetric(ch, emitted, "reverse_proxy_account_banned_total", c.descs.reverseProxyAccountBanned, prometheus.CounterValue, float64(metrics.AccountBannedTotal))
	for _, status := range sortedKeys(metrics.OAuthRefreshTotal) {
		emitConstMetric(ch, emitted, "reverse_proxy_oauth_refresh_total", c.descs.reverseProxyOAuthRefreshes, prometheus.CounterValue, float64(metrics.OAuthRefreshTotal[status]), status)
	}
}

func (c *runtimeMetricsCollector) collectProviderProbeMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	accounts, err := c.rt.accounts.List(c.ctx)
	if err != nil {
		c.rt.logger.Warn("failed to collect provider probe metrics", "error", err)
		return
	}
	protocols := map[int]string{}
	aggregates := map[string]*histogramAggregate{}
	for _, account := range accounts {
		snapshot, err := c.rt.accounts.LatestHealthSnapshotByAccount(c.ctx, account.ID)
		if err != nil {
			continue
		}
		protocol := protocols[account.ProviderID]
		if protocol == "" {
			protocol = "unknown"
			if provider, err := c.rt.providers.FindByID(c.ctx, account.ProviderID); err == nil {
				protocol = metricLabelValue(provider.Protocol, "unknown")
			}
			protocols[account.ProviderID] = protocol
		}
		status := metricLabelValue(snapshot.Status, "unknown")
		key := strings.Join([]string{protocol, status}, "\xff")
		if aggregates[key] == nil {
			aggregates[key] = newHistogramAggregate()
		}
		aggregates[key].observe(float64(snapshot.LatencyP95MS) / 1000)
	}
	for _, key := range sortedKeys(aggregates) {
		labels := strings.Split(key, "\xff")
		value := aggregates[key]
		emitConstHistogram(ch, emitted, "srapi_provider_probe_latency_seconds", c.descs.providerProbeLatency, value.count, value.sum, value.buckets, labels...)
	}
}

func (c *runtimeMetricsCollector) collectBaselineMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	if !emitted["srapi_gateway_requests_total"] {
		emitConstMetric(ch, emitted, "srapi_gateway_requests_total", c.descs.gatewayRequests, prometheus.CounterValue, 0, "unknown", "unknown", "unknown", "success")
	}
	if !emitted["srapi_gateway_request_duration_seconds"] {
		emitConstHistogram(ch, emitted, "srapi_gateway_request_duration_seconds", c.descs.gatewayDuration, 0, 0, emptyBuckets(), "unknown", "unknown", "unknown", "success")
	}
	if !emitted["srapi_gateway_inflight_requests"] {
		emitConstMetric(ch, emitted, "srapi_gateway_inflight_requests", c.descs.gatewayInflight, prometheus.GaugeValue, 0)
	}
	if !emitted["srapi_realtime_active_slots"] {
		emitConstMetric(ch, emitted, "srapi_realtime_active_slots", c.descs.realtimeActiveSlots, prometheus.GaugeValue, 0)
	}
	if !emitted["srapi_realtime_slots_total"] {
		emitConstMetric(ch, emitted, "srapi_realtime_slots_total", c.descs.realtimeSlots, prometheus.CounterValue, 0, "acquired")
	}
	if !emitted["srapi_gateway_errors_total"] {
		emitConstMetric(ch, emitted, "srapi_gateway_errors_total", c.descs.gatewayErrors, prometheus.CounterValue, 0, "unknown")
	}
	if !emitted["srapi_gateway_failover_total"] {
		emitConstMetric(ch, emitted, "srapi_gateway_failover_total", c.descs.gatewayFailover, prometheus.CounterValue, 0, "unknown", "unknown", "unknown", "success")
	}
	if !emitted["srapi_scheduler_decisions_total"] {
		emitConstMetric(ch, emitted, "srapi_scheduler_decisions_total", c.descs.schedulerDecisions, prometheus.CounterValue, 0, "unknown", "selected", "selected")
	}
	if !emitted["srapi_provider_errors_total"] {
		emitConstMetric(ch, emitted, "srapi_provider_errors_total", c.descs.providerErrors, prometheus.CounterValue, 0, "unknown", "unknown")
	}
	if !emitted["srapi_provider_probe_latency_seconds"] {
		emitConstHistogram(ch, emitted, "srapi_provider_probe_latency_seconds", c.descs.providerProbeLatency, 0, 0, emptyBuckets(), "unknown", "unknown")
	}
	if !emitted["srapi_usage_tokens_total"] {
		emitConstMetric(ch, emitted, "srapi_usage_tokens_total", c.descs.usageTokens, prometheus.CounterValue, 0, "unknown", "unknown", "input")
	}
}

type gatewayRequestAggregate struct {
	count     uint64
	latencyMS int
	buckets   map[float64]uint64
}

type histogramAggregate struct {
	count   uint64
	sum     float64
	buckets map[float64]uint64
}

func newHistogramAggregate() *histogramAggregate {
	return &histogramAggregate{buckets: emptyBuckets()}
}

func (h *histogramAggregate) observe(value float64) {
	h.count++
	h.sum += value
	for _, bucket := range metricDurationBuckets {
		if value <= bucket {
			h.buckets[bucket]++
		}
	}
}

func aggregateUsageMetrics(logs []usagecontract.UsageLog) (
	map[string]gatewayRequestAggregate,
	map[string]int,
	map[string]int,
	map[string]int,
	map[string]int,
) {
	requests := map[string]gatewayRequestAggregate{}
	failovers := map[string]int{}
	providerErrors := map[string]int{}
	gatewayErrors := map[string]int{}
	tokenCounts := map[string]int{}
	for _, log := range logs {
		result := "success"
		if !log.Success {
			result = "error"
		}
		labels := []string{
			endpointFamily(log.SourceEndpoint),
			metricLabelValue(log.Model, "unknown"),
			metricLabelValue(log.TargetProtocol, "unknown"),
			result,
		}
		key := strings.Join(labels, "\xff")
		aggregate := requests[key]
		if aggregate.buckets == nil {
			aggregate.buckets = emptyBuckets()
		}
		aggregate.count++
		aggregate.latencyMS += log.LatencyMS
		for _, bucket := range metricDurationBuckets {
			if float64(log.LatencyMS)/1000 <= bucket {
				aggregate.buckets[bucket]++
			}
		}
		requests[key] = aggregate

		if log.AttemptNo > 1 {
			failovers[key]++
		}
		if !log.Success {
			errorClass := metricLabelValue(derefString(log.ErrorClass), "unknown")
			gatewayErrors[errorClass]++
			if log.ProviderID != nil {
				providerErrors[strings.Join([]string{metricLabelValue(log.TargetProtocol, "unknown"), errorClass}, "\xff")]++
			}
		}
		model := metricLabelValue(log.Model, "unknown")
		protocol := metricLabelValue(log.TargetProtocol, "unknown")
		tokenCounts[strings.Join([]string{model, protocol, "input"}, "\xff")] += log.InputTokens
		tokenCounts[strings.Join([]string{model, protocol, "output"}, "\xff")] += log.OutputTokens
		tokenCounts[strings.Join([]string{model, protocol, "cached"}, "\xff")] += log.CachedTokens
	}
	return requests, failovers, providerErrors, gatewayErrors, tokenCounts
}

func schedulerDecisionCounts(decisions []schedulercontract.Decision) map[string]int {
	counts := map[string]int{}
	for _, decision := range decisions {
		outcome := "selected"
		reason := "selected"
		if decision.SelectedAccountID == nil {
			outcome = "rejected"
			reason = primaryRejectReason(decision.RejectReasons)
		}
		counts[strings.Join([]string{string(decision.Strategy), outcome, reason}, "\xff")]++
	}
	return counts
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

func realtimeEmptySnapshot() realtimecontract.Snapshot {
	return realtimecontract.Snapshot{ActiveByEndpoint: map[string]int{}}
}

var metricDurationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func emptyBuckets() map[float64]uint64 {
	buckets := make(map[float64]uint64, len(metricDurationBuckets))
	for _, bucket := range metricDurationBuckets {
		buckets[bucket] = 0
	}
	return buckets
}

func emitConstMetric(ch chan<- prometheus.Metric, emitted map[string]bool, name string, desc *prometheus.Desc, valueType prometheus.ValueType, value float64, labelValues ...string) {
	metric, err := prometheus.NewConstMetric(desc, valueType, value, labelValues...)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(desc, err)
		return
	}
	emitted[name] = true
	ch <- metric
}

func emitConstHistogram(ch chan<- prometheus.Metric, emitted map[string]bool, name string, desc *prometheus.Desc, count uint64, sum float64, buckets map[float64]uint64, labelValues ...string) {
	metric, err := prometheus.NewConstHistogram(desc, count, sum, buckets, labelValues...)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(desc, err)
		return
	}
	emitted[name] = true
	ch <- metric
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

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
