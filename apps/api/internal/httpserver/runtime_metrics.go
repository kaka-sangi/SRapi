package httpserver

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
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
	schedulerCostScore            *prometheus.Desc
	schedulerStrategySelected     *prometheus.Desc
	schedulerStrategyFallback     *prometheus.Desc
	schedulerStrategyShadowDiff   *prometheus.Desc
	schedulerStrategyCostDelta    *prometheus.Desc
	schedulerStrategyLatencyDelta *prometheus.Desc
	schedulerStrategyErrorRate    *prometheus.Desc
	schedulerStrategyRejectReason *prometheus.Desc
	opsAlertEvents                *prometheus.Desc
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
	proxyProbeAttempts            *prometheus.Desc
	proxyProbeOutcomes            *prometheus.Desc
	accountsTokenRefreshAttempts  *prometheus.Desc
	accountsTokenRefreshOutcomes  *prometheus.Desc
	opsErrorLogQueueDepth         *prometheus.Desc
	opsErrorLogQueueCapacity      *prometheus.Desc
	opsErrorLogEnqueued           *prometheus.Desc
	opsErrorLogProcessed          *prometheus.Desc
	opsErrorLogDropped            *prometheus.Desc
	opsErrorLogWriteFailures      *prometheus.Desc
}

func newRuntimeMetricsCollector(ctx context.Context, rt *runtimeState) *runtimeMetricsCollector {
	return &runtimeMetricsCollector{
		ctx:   ctx,
		rt:    rt,
		descs: newRuntimeMetricDescs(),
	}
}

func newRuntimeMetricDescs() runtimeMetricDescs {
	descs := runtimeMetricDescs{}
	initGatewayMetricDescs(&descs)
	initSchedulerMetricDescs(&descs)
	initProviderMetricDescs(&descs)
	initWorkerMetricDescs(&descs)
	initOpsErrorLogMetricDescs(&descs)
	return descs
}

func initGatewayMetricDescs(descs *runtimeMetricDescs) {
	descs.gatewayRequests = prometheus.NewDesc(
		"srapi_gateway_requests_total",
		"Gateway requests recorded by endpoint family, model, provider protocol, and result.",
		[]string{"endpoint_family", "model", "provider_protocol", "result"},
		nil,
	)
	descs.gatewayDuration = prometheus.NewDesc(
		"srapi_gateway_request_duration_seconds",
		"Gateway request latency histogram derived from usage logs.",
		[]string{"endpoint_family", "model", "provider_protocol", "result"},
		nil,
	)
	descs.gatewayInflight = prometheus.NewDesc(
		"srapi_gateway_inflight_requests",
		"Gateway requests with pending scheduler leases.",
		nil,
		nil,
	)
	descs.realtimeActiveSlots = prometheus.NewDesc(
		"srapi_realtime_active_slots",
		"Active realtime WebSocket slots.",
		nil,
		nil,
	)
	descs.realtimeActiveSlotsByEndpoint = prometheus.NewDesc(
		"srapi_realtime_active_slots_by_endpoint",
		"Active realtime WebSocket slots by source endpoint.",
		[]string{"source_endpoint"},
		nil,
	)
	descs.realtimeSlots = prometheus.NewDesc(
		"srapi_realtime_slots_total",
		"Realtime WebSocket slot lifecycle events.",
		[]string{"event"},
		nil,
	)
	descs.gatewayErrors = prometheus.NewDesc(
		"srapi_gateway_errors_total",
		"Gateway request errors recorded by error class.",
		[]string{"error_class"},
		nil,
	)
	descs.gatewayFailover = prometheus.NewDesc(
		"srapi_gateway_failover_total",
		"Gateway fallback attempts by endpoint family, model, provider protocol, and result.",
		[]string{"endpoint_family", "model", "provider_protocol", "result"},
		nil,
	)
}

func initSchedulerMetricDescs(descs *runtimeMetricDescs) {
	descs.schedulerDecisions = prometheus.NewDesc(
		"srapi_scheduler_decisions_total",
		"Scheduler decisions by strategy and outcome.",
		[]string{"strategy", "outcome", "reason"},
		nil,
	)
	descs.schedulerCostScore = prometheus.NewDesc(
		"srapi_scheduler_cost_score_avg",
		"Average scheduler cost score by strategy, derived from persisted decision score breakdowns.",
		[]string{"strategy"},
		nil,
	)
	descs.schedulerStrategySelected = prometheus.NewDesc(
		"scheduler_strategy_selected_total",
		"Scheduler selections by strategy and version.",
		[]string{"strategy", "version"},
		nil,
	)
	descs.schedulerStrategyFallback = prometheus.NewDesc(
		"scheduler_strategy_fallback_total",
		"Scheduler fallback attempts by strategy and version.",
		[]string{"strategy", "version"},
		nil,
	)
	descs.schedulerStrategyShadowDiff = prometheus.NewDesc(
		"scheduler_strategy_shadow_diff",
		"Scheduler real-traffic shadow rollout decisions by selected side.",
		[]string{"strategy", "version", "shadow_strategy", "selection"},
		nil,
	)
	descs.schedulerStrategyCostDelta = prometheus.NewDesc(
		"scheduler_strategy_cost_delta",
		"Average selected cost score minus candidate-set average cost score by strategy and version.",
		[]string{"strategy", "version"},
		nil,
	)
	descs.schedulerStrategyLatencyDelta = prometheus.NewDesc(
		"scheduler_strategy_latency_delta",
		"Average selected latency score minus candidate-set average latency score by strategy and version.",
		[]string{"strategy", "version"},
		nil,
	)
	descs.schedulerStrategyErrorRate = prometheus.NewDesc(
		"scheduler_strategy_error_rate",
		"Usage-log error rate for scheduler decisions by strategy and version.",
		[]string{"strategy", "version"},
		nil,
	)
	descs.schedulerStrategyRejectReason = prometheus.NewDesc(
		"scheduler_strategy_reject_reason_total",
		"Scheduler rejected candidates by strategy, version, and reject reason.",
		[]string{"strategy", "version", "reason"},
		nil,
	)
}

func initProviderMetricDescs(descs *runtimeMetricDescs) {
	descs.opsAlertEvents = prometheus.NewDesc(
		"srapi_ops_alert_events",
		"Current operational alert events by severity and status.",
		[]string{"severity", "status"},
		nil,
	)
	descs.providerErrors = prometheus.NewDesc(
		"srapi_provider_errors_total",
		"Provider-facing errors recorded by protocol and error class.",
		[]string{"provider_protocol", "error_class"},
		nil,
	)
	descs.providerProbeLatency = prometheus.NewDesc(
		"srapi_provider_probe_latency_seconds",
		"Provider account probe availability signal derived from materialized health rollups.",
		[]string{"provider_protocol", "status"},
		nil,
	)
	descs.usageTokens = prometheus.NewDesc(
		"srapi_usage_tokens_total",
		"Usage tokens by model, provider protocol, and token kind.",
		[]string{"model", "provider_protocol", "token_kind"},
		nil,
	)
	descs.reverseProxyBanSignals = prometheus.NewDesc(
		"srapi_reverse_proxy_ban_signals_total",
		"Reverse proxy ban signals observed by risk class.",
		[]string{"risk_class"},
		nil,
	)
	descs.reverseProxyRequests = prometheus.NewDesc("reverse_proxy_request_total", "Reverse proxy requests.", nil, nil)
	descs.reverseProxyRequestSuccesses = prometheus.NewDesc(
		"reverse_proxy_request_success_total",
		"Reverse proxy successful requests.",
		nil,
		nil,
	)
	descs.reverseProxyRequestErrors = prometheus.NewDesc(
		"reverse_proxy_request_error_total",
		"Reverse proxy request errors by class.",
		[]string{"error_class"},
		nil,
	)
	descs.reverseProxyChallenges = prometheus.NewDesc(
		"reverse_proxy_challenge_total",
		"Reverse proxy challenges by strategy.",
		[]string{"strategy"},
		nil,
	)
	descs.reverseProxyAccountLocked = prometheus.NewDesc("reverse_proxy_account_locked_total", "Reverse proxy account locked events.", nil, nil)
	descs.reverseProxyAccountBanned = prometheus.NewDesc("reverse_proxy_account_banned_total", "Reverse proxy account banned events.", nil, nil)
	descs.reverseProxyOAuthRefreshes = prometheus.NewDesc(
		"reverse_proxy_oauth_refresh_total",
		"Reverse proxy OAuth refresh attempts by status.",
		[]string{"status"},
		nil,
	)
}

func initWorkerMetricDescs(descs *runtimeMetricDescs) {
	descs.proxyProbeAttempts = prometheus.NewDesc(
		"srapi_proxy_probe_attempts_total",
		"Proxy probe attempts emitted by the proxy_probe worker.",
		nil,
		nil,
	)
	descs.proxyProbeOutcomes = prometheus.NewDesc(
		"srapi_proxy_probe_outcomes_total",
		"Proxy probe outcomes (succeeded/failed) emitted by the proxy_probe worker.",
		[]string{"outcome"},
		nil,
	)
	descs.accountsTokenRefreshAttempts = prometheus.NewDesc(
		"srapi_accounts_token_refresh_attempts_total",
		"OAuth token refresh attempts emitted by the accounts_token_refresh worker.",
		nil,
		nil,
	)
	descs.accountsTokenRefreshOutcomes = prometheus.NewDesc(
		"srapi_accounts_token_refresh_outcomes_total",
		"OAuth token refresh outcomes by class emitted by the accounts_token_refresh worker.",
		[]string{"outcome"},
		nil,
	)
}

func initOpsErrorLogMetricDescs(descs *runtimeMetricDescs) {
	descs.opsErrorLogQueueDepth = prometheus.NewDesc(
		"srapi_ops_error_log_queue_depth",
		"Current queued ops_error_logs records waiting for asynchronous persistence.",
		nil,
		nil,
	)
	descs.opsErrorLogQueueCapacity = prometheus.NewDesc(
		"srapi_ops_error_log_queue_capacity",
		"Capacity of the asynchronous ops_error_logs queue.",
		nil,
		nil,
	)
	descs.opsErrorLogEnqueued = prometheus.NewDesc(
		"srapi_ops_error_log_enqueued_total",
		"Ops error log records accepted into the asynchronous persistence queue.",
		nil,
		nil,
	)
	descs.opsErrorLogProcessed = prometheus.NewDesc(
		"srapi_ops_error_log_processed_total",
		"Ops error log records processed by the asynchronous persistence worker.",
		nil,
		nil,
	)
	descs.opsErrorLogDropped = prometheus.NewDesc(
		"srapi_ops_error_log_dropped_total",
		"Ops error log records dropped because the asynchronous queue was full or draining.",
		nil,
		nil,
	)
	descs.opsErrorLogWriteFailures = prometheus.NewDesc(
		"srapi_ops_error_log_write_failures_total",
		"Ops error log records whose asynchronous persistence write failed.",
		nil,
		nil,
	)
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
		d.schedulerCostScore,
		d.schedulerStrategySelected,
		d.schedulerStrategyFallback,
		d.schedulerStrategyShadowDiff,
		d.schedulerStrategyCostDelta,
		d.schedulerStrategyLatencyDelta,
		d.schedulerStrategyErrorRate,
		d.schedulerStrategyRejectReason,
		d.opsAlertEvents,
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
		d.proxyProbeAttempts,
		d.proxyProbeOutcomes,
		d.accountsTokenRefreshAttempts,
		d.accountsTokenRefreshOutcomes,
		d.opsErrorLogQueueDepth,
		d.opsErrorLogQueueCapacity,
		d.opsErrorLogEnqueued,
		d.opsErrorLogProcessed,
		d.opsErrorLogDropped,
		d.opsErrorLogWriteFailures,
	}
}

func (c *runtimeMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	emitted := map[string]bool{}
	metrics := c.rt.metrics
	if metrics == nil {
		metrics = newRuntimeMetricsState()
	}
	snapshot := metrics.Snapshot()
	c.collectUsageMetrics(ch, emitted, snapshot)
	c.collectSchedulerMetrics(ch, emitted, snapshot)
	c.collectRealtimeMetrics(ch, emitted)
	c.collectReverseProxyMetrics(ch, emitted)
	c.collectOpsAlertMetrics(ch, emitted)
	c.collectProviderProbeMetrics(ch, emitted)
	c.collectWorkerMetrics(ch, emitted)
	c.collectBaselineMetrics(ch, emitted)
}

func (c *runtimeMetricsCollector) collectWorkerMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	if c.rt.opsErrorLogRecorder != nil {
		snapshot := c.rt.opsErrorLogRecorder.snapshot()
		emitConstMetric(ch, emitted, "srapi_ops_error_log_queue_depth", c.descs.opsErrorLogQueueDepth, prometheus.GaugeValue, float64(snapshot.Queued))
		emitConstMetric(ch, emitted, "srapi_ops_error_log_queue_capacity", c.descs.opsErrorLogQueueCapacity, prometheus.GaugeValue, float64(snapshot.Capacity))
		emitConstMetric(ch, emitted, "srapi_ops_error_log_enqueued_total", c.descs.opsErrorLogEnqueued, prometheus.CounterValue, float64(snapshot.Enqueued))
		emitConstMetric(ch, emitted, "srapi_ops_error_log_processed_total", c.descs.opsErrorLogProcessed, prometheus.CounterValue, float64(snapshot.Processed))
		emitConstMetric(ch, emitted, "srapi_ops_error_log_dropped_total", c.descs.opsErrorLogDropped, prometheus.CounterValue, float64(snapshot.Dropped))
		emitConstMetric(ch, emitted, "srapi_ops_error_log_write_failures_total", c.descs.opsErrorLogWriteFailures, prometheus.CounterValue, float64(snapshot.WriteFailed))
	}
	if c.rt.proxyProbeMetrics != nil {
		snapshot := c.rt.proxyProbeMetrics()
		emitConstMetric(ch, emitted, "srapi_proxy_probe_attempts_total", c.descs.proxyProbeAttempts, prometheus.CounterValue, float64(snapshot.ProbeAttempted))
		emitConstMetric(ch, emitted, "srapi_proxy_probe_outcomes_total", c.descs.proxyProbeOutcomes, prometheus.CounterValue, float64(snapshot.ProbeSucceeded), "succeeded")
		emitConstMetric(ch, emitted, "srapi_proxy_probe_outcomes_total", c.descs.proxyProbeOutcomes, prometheus.CounterValue, float64(snapshot.ProbeFailed), "failed")
	}
	if c.rt.tokenRefreshMetrics != nil {
		snapshot := c.rt.tokenRefreshMetrics()
		emitConstMetric(ch, emitted, "srapi_accounts_token_refresh_attempts_total", c.descs.accountsTokenRefreshAttempts, prometheus.CounterValue, float64(snapshot.RefreshAttempted))
		emitConstMetric(ch, emitted, "srapi_accounts_token_refresh_outcomes_total", c.descs.accountsTokenRefreshOutcomes, prometheus.CounterValue, float64(snapshot.RefreshSucceeded), "succeeded")
		emitConstMetric(ch, emitted, "srapi_accounts_token_refresh_outcomes_total", c.descs.accountsTokenRefreshOutcomes, prometheus.CounterValue, float64(snapshot.RefreshFailedPermanent), "failed_permanent")
		emitConstMetric(ch, emitted, "srapi_accounts_token_refresh_outcomes_total", c.descs.accountsTokenRefreshOutcomes, prometheus.CounterValue, float64(snapshot.RefreshFailedTransient), "failed_transient")
		emitConstMetric(ch, emitted, "srapi_accounts_token_refresh_outcomes_total", c.descs.accountsTokenRefreshOutcomes, prometheus.CounterValue, float64(snapshot.RefreshThresholdExceeded), "threshold_exceeded")
	}
}

func (c *runtimeMetricsCollector) collectUsageMetrics(ch chan<- prometheus.Metric, emitted map[string]bool, snapshot runtimeMetricsSnapshot) {
	for _, key := range sortedKeys(snapshot.requests) {
		labels := strings.Split(key, "\xff")
		value := snapshot.requests[key]
		emitConstMetric(ch, emitted, "srapi_gateway_requests_total", c.descs.gatewayRequests, prometheus.CounterValue, float64(value.count), labels...)
		emitConstHistogram(ch, emitted, "srapi_gateway_request_duration_seconds", c.descs.gatewayDuration, value.count, float64(value.latencyMS)/1000, value.buckets, labels...)
	}
	for _, key := range sortedKeys(snapshot.failovers) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_gateway_failover_total", c.descs.gatewayFailover, prometheus.CounterValue, float64(snapshot.failovers[key]), labels...)
	}
	for _, key := range sortedKeys(snapshot.providerErrors) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_provider_errors_total", c.descs.providerErrors, prometheus.CounterValue, float64(snapshot.providerErrors[key]), labels...)
	}
	for _, key := range sortedKeys(snapshot.gatewayErrors) {
		emitConstMetric(ch, emitted, "srapi_gateway_errors_total", c.descs.gatewayErrors, prometheus.CounterValue, float64(snapshot.gatewayErrors[key]), key)
	}
	for _, key := range sortedKeys(snapshot.tokenCounts) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_usage_tokens_total", c.descs.usageTokens, prometheus.CounterValue, float64(snapshot.tokenCounts[key]), labels...)
	}
}

func (c *runtimeMetricsCollector) collectSchedulerMetrics(ch chan<- prometheus.Metric, emitted map[string]bool, snapshot runtimeMetricsSnapshot) {
	counts := schedulerDecisionCounts(snapshot.schedulerDecisions)
	for _, key := range sortedKeys(counts) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_scheduler_decisions_total", c.descs.schedulerDecisions, prometheus.CounterValue, float64(counts[key]), labels...)
	}
	costScores := schedulerCostScoreAverages(snapshot.schedulerDecisions)
	for _, strategy := range sortedKeys(costScores) {
		emitConstMetric(ch, emitted, "srapi_scheduler_cost_score_avg", c.descs.schedulerCostScore, prometheus.GaugeValue, costScores[strategy], strategy)
	}
	strategyMetrics := schedulerStrategyMetrics(snapshot.schedulerDecisions, snapshot.usageLogs)
	for _, key := range sortedKeys(strategyMetrics.selected) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_selected_total", c.descs.schedulerStrategySelected, prometheus.CounterValue, float64(strategyMetrics.selected[key]), labels...)
	}
	for _, key := range sortedKeys(strategyMetrics.fallback) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_fallback_total", c.descs.schedulerStrategyFallback, prometheus.CounterValue, float64(strategyMetrics.fallback[key]), labels...)
	}
	for _, key := range sortedKeys(strategyMetrics.shadowDiff) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_shadow_diff", c.descs.schedulerStrategyShadowDiff, prometheus.CounterValue, float64(strategyMetrics.shadowDiff[key]), labels...)
	}
	for _, key := range sortedKeys(strategyMetrics.costDelta) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_cost_delta", c.descs.schedulerStrategyCostDelta, prometheus.GaugeValue, strategyMetrics.costDelta[key], labels...)
	}
	for _, key := range sortedKeys(strategyMetrics.latencyDelta) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_latency_delta", c.descs.schedulerStrategyLatencyDelta, prometheus.GaugeValue, strategyMetrics.latencyDelta[key], labels...)
	}
	for _, key := range sortedKeys(strategyMetrics.errorRate) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_error_rate", c.descs.schedulerStrategyErrorRate, prometheus.GaugeValue, strategyMetrics.errorRate[key], labels...)
	}
	for _, key := range sortedKeys(strategyMetrics.rejectReason) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "scheduler_strategy_reject_reason_total", c.descs.schedulerStrategyRejectReason, prometheus.CounterValue, float64(strategyMetrics.rejectReason[key]), labels...)
	}
	emitConstMetric(ch, emitted, "srapi_gateway_inflight_requests", c.descs.gatewayInflight, prometheus.GaugeValue, float64(c.rt.scheduler.ActiveLeaseCount(c.ctx)))
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

func (c *runtimeMetricsCollector) collectOpsAlertMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	alerts, err := c.rt.operations.ListAlerts(c.ctx)
	if err != nil {
		c.rt.logger.Warn("failed to collect ops alert metrics", "error", err)
		return
	}
	counts := opsAlertCounts(alerts)
	for _, key := range sortedKeys(counts) {
		labels := strings.Split(key, "\xff")
		emitConstMetric(ch, emitted, "srapi_ops_alert_events", c.descs.opsAlertEvents, prometheus.GaugeValue, float64(counts[key]), labels...)
	}
}

func (c *runtimeMetricsCollector) collectProviderProbeMetrics(ch chan<- prometheus.Metric, emitted map[string]bool) {
	if c.rt.healthRollups == nil {
		return
	}
	rollups, err := c.rt.healthRollups.ListRecent(c.ctx, 1, time.Now().UTC())
	if err != nil {
		c.rt.logger.Warn("failed to collect provider probe metrics", "error", err)
		return
	}
	protocols := map[int]string{}
	aggregates := map[string]*histogramAggregate{}
	for _, rollup := range rollups {
		if rollup.TotalSamples <= 0 {
			continue
		}
		protocol := protocols[rollup.ProviderID]
		if protocol == "" {
			protocol = "unknown"
			if provider, err := c.rt.providers.FindByID(c.ctx, rollup.ProviderID); err == nil {
				protocol = metricLabelValue(provider.Protocol, "unknown")
			}
			protocols[rollup.ProviderID] = protocol
		}
		status := rollupProbeStatus(rollup.AvailabilityRatio)
		key := strings.Join([]string{protocol, status}, "\xff")
		if aggregates[key] == nil {
			aggregates[key] = newHistogramAggregate()
		}
		aggregates[key].observe(float64(1 - rollup.AvailabilityRatio))
	}
	for _, key := range sortedKeys(aggregates) {
		labels := strings.Split(key, "\xff")
		value := aggregates[key]
		emitConstHistogram(ch, emitted, "srapi_provider_probe_latency_seconds", c.descs.providerProbeLatency, value.count, value.sum, value.buckets, labels...)
	}
}

func rollupProbeStatus(availabilityRatio float32) string {
	if availabilityRatio >= 0.99 {
		return "healthy"
	}
	if availabilityRatio > 0 {
		return "degraded"
	}
	return "dead"
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
	if !emitted["srapi_scheduler_cost_score_avg"] {
		emitConstMetric(ch, emitted, "srapi_scheduler_cost_score_avg", c.descs.schedulerCostScore, prometheus.GaugeValue, 0, "unknown")
	}
	if !emitted["scheduler_strategy_selected_total"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_selected_total", c.descs.schedulerStrategySelected, prometheus.CounterValue, 0, "unknown", "unknown")
	}
	if !emitted["scheduler_strategy_fallback_total"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_fallback_total", c.descs.schedulerStrategyFallback, prometheus.CounterValue, 0, "unknown", "unknown")
	}
	if !emitted["scheduler_strategy_shadow_diff"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_shadow_diff", c.descs.schedulerStrategyShadowDiff, prometheus.CounterValue, 0, "unknown", "unknown", "unknown", "current")
	}
	if !emitted["scheduler_strategy_cost_delta"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_cost_delta", c.descs.schedulerStrategyCostDelta, prometheus.GaugeValue, 0, "unknown", "unknown")
	}
	if !emitted["scheduler_strategy_latency_delta"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_latency_delta", c.descs.schedulerStrategyLatencyDelta, prometheus.GaugeValue, 0, "unknown", "unknown")
	}
	if !emitted["scheduler_strategy_error_rate"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_error_rate", c.descs.schedulerStrategyErrorRate, prometheus.GaugeValue, 0, "unknown", "unknown")
	}
	if !emitted["scheduler_strategy_reject_reason_total"] {
		emitConstMetric(ch, emitted, "scheduler_strategy_reject_reason_total", c.descs.schedulerStrategyRejectReason, prometheus.CounterValue, 0, "unknown", "unknown", "unknown")
	}
	if !emitted["srapi_ops_alert_events"] {
		emitConstMetric(ch, emitted, "srapi_ops_alert_events", c.descs.opsAlertEvents, prometheus.GaugeValue, 0, "unknown", "unknown")
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

func schedulerCostScoreAverages(decisions []schedulercontract.Decision) map[string]float64 {
	type aggregate struct {
		sum   float64
		count int
	}
	aggregates := map[string]aggregate{}
	for _, decision := range decisions {
		strategy := metricLabelValue(string(decision.Strategy), "unknown")
		for _, score := range decision.Scores {
			costScore, ok := scoreCostValue(score)
			if !ok {
				continue
			}
			aggregate := aggregates[strategy]
			aggregate.sum += costScore
			aggregate.count++
			aggregates[strategy] = aggregate
		}
	}
	out := make(map[string]float64, len(aggregates))
	for strategy, aggregate := range aggregates {
		if aggregate.count == 0 {
			continue
		}
		out[strategy] = aggregate.sum / float64(aggregate.count)
	}
	return out
}

func opsAlertCounts(alerts []operationscontract.AlertEvent) map[string]int {
	counts := map[string]int{}
	for _, alert := range alerts {
		severity := metricLabelValue(string(alert.Severity), "unknown")
		status := metricLabelValue(string(alert.Status), "unknown")
		counts[strings.Join([]string{severity, status}, "\xff")]++
	}
	return counts
}

type schedulerStrategyMetricSet struct {
	selected     map[string]int
	fallback     map[string]int
	shadowDiff   map[string]int
	costDelta    map[string]float64
	latencyDelta map[string]float64
	errorRate    map[string]float64
	rejectReason map[string]int
}

type averageMetric struct {
	sum   float64
	count int
}

type rateMetric struct {
	total  int
	errors int
}

func schedulerStrategyMetrics(decisions []schedulercontract.Decision, usageLogs []usagecontract.UsageLog) schedulerStrategyMetricSet {
	metrics := schedulerStrategyMetricSet{
		selected:     map[string]int{},
		fallback:     map[string]int{},
		shadowDiff:   map[string]int{},
		costDelta:    map[string]float64{},
		latencyDelta: map[string]float64{},
		errorRate:    map[string]float64{},
		rejectReason: map[string]int{},
	}
	decisionByAttempt := map[string]schedulercontract.Decision{}
	costDeltas := map[string]averageMetric{}
	latencyDeltas := map[string]averageMetric{}
	for _, decision := range decisions {
		key := schedulerStrategyKey(decision)
		decisionByAttempt[schedulerAttemptKey(decision.RequestID, decision.AttemptNo)] = decision
		if decision.SelectedAccountID != nil {
			metrics.selected[key]++
			if delta, ok := selectedScoreDelta(decision.Scores, *decision.SelectedAccountID, "cost_score"); ok {
				costDeltas[key] = observeAverage(costDeltas[key], delta)
			}
			if delta, ok := selectedScoreDelta(decision.Scores, *decision.SelectedAccountID, "latency_score"); ok {
				latencyDeltas[key] = observeAverage(latencyDeltas[key], delta)
			}
		}
		if decision.FallbackFromDecisionID != nil || decision.AttemptNo > 1 {
			metrics.fallback[key]++
		}
		for _, reason := range schedulerRejectReasons(decision.RejectReasons) {
			metrics.rejectReason[strings.Join([]string{key, reason}, "\xff")]++
		}
		if shadowStrategy, selection, ok := schedulerRolloutSelection(decision.Scores); ok {
			metrics.shadowDiff[strings.Join([]string{key, shadowStrategy, selection}, "\xff")]++
		}
	}

	rates := map[string]rateMetric{}
	for _, log := range usageLogs {
		decision, ok := decisionByAttempt[schedulerAttemptKey(log.RequestID, log.AttemptNo)]
		if !ok {
			continue
		}
		key := schedulerStrategyKey(decision)
		rate := rates[key]
		rate.total++
		if !log.Success {
			rate.errors++
		}
		rates[key] = rate
	}

	for key, aggregate := range costDeltas {
		if aggregate.count > 0 {
			metrics.costDelta[key] = aggregate.sum / float64(aggregate.count)
		}
	}
	for key, aggregate := range latencyDeltas {
		if aggregate.count > 0 {
			metrics.latencyDelta[key] = aggregate.sum / float64(aggregate.count)
		}
	}
	for key, rate := range rates {
		if rate.total > 0 {
			metrics.errorRate[key] = float64(rate.errors) / float64(rate.total)
		}
	}
	return metrics
}

func schedulerStrategyKey(decision schedulercontract.Decision) string {
	return strings.Join([]string{
		metricLabelValue(string(decision.Strategy), "unknown"),
		metricLabelValue(decision.StrategyVersion, "unknown"),
	}, "\xff")
}

func schedulerAttemptKey(requestID string, attemptNo int) string {
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return strings.TrimSpace(requestID) + "\xff" + strconv.Itoa(attemptNo)
}

func observeAverage(aggregate averageMetric, value float64) averageMetric {
	aggregate.sum += value
	aggregate.count++
	return aggregate
}

func selectedScoreDelta(scores map[string]any, selectedAccountID int, component string) (float64, bool) {
	if len(scores) == 0 {
		return 0, false
	}
	var selected float64
	var selectedOK bool
	var sum float64
	var count int
	for _, raw := range scores {
		accountID, ok := scoreAccountID(raw)
		if !ok {
			continue
		}
		value, ok := scoreComponentValue(raw, component)
		if !ok {
			continue
		}
		sum += value
		count++
		if accountID == selectedAccountID {
			selected = value
			selectedOK = true
		}
	}
	if !selectedOK || count == 0 {
		return 0, false
	}
	return selected - sum/float64(count), true
}

func scoreAccountID(score any) (int, bool) {
	values, ok := score.(map[string]any)
	if !ok {
		return 0, false
	}
	value, ok := values["account_id"]
	if !ok {
		return 0, false
	}
	floatValue, ok := metricFloatValue(value)
	if !ok {
		return 0, false
	}
	return int(floatValue), true
}

func scoreComponentValue(score any, component string) (float64, bool) {
	values, ok := score.(map[string]any)
	if !ok {
		return 0, false
	}
	value, ok := values[component]
	if !ok {
		return 0, false
	}
	return metricFloatValue(value)
}

func schedulerRejectReasons(reasons map[string]any) []string {
	if len(reasons) == 0 {
		return nil
	}
	out := make([]string, 0, len(reasons))
	for _, raw := range reasons {
		reason, ok := raw.(string)
		if !ok {
			continue
		}
		if label := metricLabelValue(reason, ""); label != "" {
			out = append(out, label)
		}
	}
	return out
}

func schedulerRolloutSelection(scores map[string]any) (string, string, bool) {
	routingHints, ok := scores["routing_hints"].(map[string]any)
	if !ok {
		return "", "", false
	}
	rawRollout, ok := routingHints["strategy_rollout"].(map[string]any)
	if !ok {
		return "", "", false
	}
	rawStrategy, ok := rawRollout["shadow_strategy"].(string)
	if !ok {
		return "", "", false
	}
	selection := "current"
	if shadowSelected, ok := rawRollout["shadow_selected"].(bool); ok && shadowSelected {
		selection = "shadow"
	}
	return metricLabelValue(rawStrategy, "unknown"), selection, true
}

func scoreCostValue(score any) (float64, bool) {
	values, ok := score.(map[string]any)
	if !ok {
		return 0, false
	}
	value, ok := values["cost_score"]
	if !ok {
		return 0, false
	}
	return metricFloatValue(value)
}

func metricFloatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
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
