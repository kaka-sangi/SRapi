package httpserver

import (
	"strings"
	"sync"

	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

type runtimeMetricsState struct {
	mu                 sync.RWMutex
	requests           map[string]gatewayRequestAggregate
	failovers          map[string]int
	providerErrors     map[string]int
	gatewayErrors      map[string]int
	tokenCounts        map[string]int
	schedulerDecisions []schedulercontract.Decision
	usageByAttempt     map[string]usagecontract.UsageLog
}

type runtimeMetricsSnapshot struct {
	requests           map[string]gatewayRequestAggregate
	failovers          map[string]int
	providerErrors     map[string]int
	gatewayErrors      map[string]int
	tokenCounts        map[string]int
	schedulerDecisions []schedulercontract.Decision
	usageLogs          []usagecontract.UsageLog
}

func newRuntimeMetricsState() *runtimeMetricsState {
	return &runtimeMetricsState{
		requests:       map[string]gatewayRequestAggregate{},
		failovers:      map[string]int{},
		providerErrors: map[string]int{},
		gatewayErrors:  map[string]int{},
		tokenCounts:    map[string]int{},
		usageByAttempt: map[string]usagecontract.UsageLog{},
	}
}

func (m *runtimeMetricsState) RecordGatewayUsage(log usagecontract.UsageLog) {
	if m == nil {
		return
	}
	requests, failovers, providerErrors, gatewayErrors, tokenCounts := aggregateUsageMetrics([]usagecontract.UsageLog{log})
	m.mu.Lock()
	defer m.mu.Unlock()
	mergeGatewayRequestAggregates(m.requests, requests)
	mergeIntMap(m.failovers, failovers)
	mergeIntMap(m.providerErrors, providerErrors)
	mergeIntMap(m.gatewayErrors, gatewayErrors)
	mergeIntMap(m.tokenCounts, tokenCounts)
	if key := schedulerAttemptKey(log.RequestID, log.AttemptNo); strings.TrimSpace(key) != "" {
		m.usageByAttempt[key] = log
	}
}

func (m *runtimeMetricsState) RecordSchedulerDecision(decision schedulercontract.Decision) {
	if m == nil || strings.TrimSpace(decision.RequestID) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedulerDecisions = append(m.schedulerDecisions, decision)
}

func (m *runtimeMetricsState) Snapshot() runtimeMetricsSnapshot {
	if m == nil {
		return runtimeMetricsSnapshot{
			requests:       map[string]gatewayRequestAggregate{},
			failovers:      map[string]int{},
			providerErrors: map[string]int{},
			gatewayErrors:  map[string]int{},
			tokenCounts:    map[string]int{},
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	usageLogs := make([]usagecontract.UsageLog, 0, len(m.usageByAttempt))
	for _, log := range m.usageByAttempt {
		usageLogs = append(usageLogs, log)
	}
	return runtimeMetricsSnapshot{
		requests:           cloneGatewayRequestAggregates(m.requests),
		failovers:          cloneIntMap(m.failovers),
		providerErrors:     cloneIntMap(m.providerErrors),
		gatewayErrors:      cloneIntMap(m.gatewayErrors),
		tokenCounts:        cloneIntMap(m.tokenCounts),
		schedulerDecisions: append([]schedulercontract.Decision(nil), m.schedulerDecisions...),
		usageLogs:          usageLogs,
	}
}

func mergeGatewayRequestAggregates(dst map[string]gatewayRequestAggregate, src map[string]gatewayRequestAggregate) {
	for key, value := range src {
		current := dst[key]
		current.count += value.count
		current.latencyMS += value.latencyMS
		if current.buckets == nil {
			current.buckets = emptyBuckets()
		}
		for bucket, count := range value.buckets {
			current.buckets[bucket] += count
		}
		dst[key] = current
	}
}

func mergeIntMap(dst map[string]int, src map[string]int) {
	for key, value := range src {
		dst[key] += value
	}
}

func cloneGatewayRequestAggregates(src map[string]gatewayRequestAggregate) map[string]gatewayRequestAggregate {
	dst := make(map[string]gatewayRequestAggregate, len(src))
	for key, value := range src {
		value.buckets = cloneUint64Map(value.buckets)
		dst[key] = value
	}
	return dst
}

func cloneIntMap(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneUint64Map(src map[float64]uint64) map[float64]uint64 {
	dst := make(map[float64]uint64, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
