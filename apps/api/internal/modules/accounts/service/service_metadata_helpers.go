package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

func probeHealthState(account contract.ProviderAccount, result contract.AccountProbeResult, history []contract.AccountHealthSnapshot, policy contract.AccountProbePolicy) (contract.AccountHealthSnapshot, contract.ProviderAccount) {
	samples := probeSamples(result, history, policy.HistoryLimit)
	successes := 0
	failures := 0
	latencies := make([]int, 0, len(samples))
	for _, sample := range samples {
		if sample.success {
			successes++
		} else {
			failures++
		}
		if sample.latencyMS > 0 {
			latencies = append(latencies, sample.latencyMS)
		}
	}
	total := successes + failures
	successRate := float32(0)
	errorRate := float32(0)
	if total > 0 {
		successRate = float32(successes) / float32(total)
		errorRate = float32(failures) / float32(total)
	}
	consecutiveFailures := consecutiveProbeFailures(samples)
	unhealthy := consecutiveFailures >= policy.FailureThreshold ||
		(total >= policy.MinSamplesForErrorRate && errorRate > policy.ErrorRateThreshold)

	status := "healthy"
	circuitState := "closed"
	var cooldownUntil *time.Time
	if unhealthy {
		status = "unhealthy"
		circuitState = "open"
		until := result.CheckedAt.Add(policy.Cooldown)
		cooldownUntil = &until
	} else if !result.OK {
		status = "degraded"
		circuitState = "half_open"
	}

	snapshot := contract.AccountHealthSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		Status:         status,
		SuccessRate:    clampRatio(successRate),
		ErrorRate:      clampRatio(errorRate),
		LatencyP50MS:   percentileLatency(latencies, 0.50),
		LatencyP95MS:   percentileLatency(latencies, 0.95),
		RateLimitCount: probeErrorCount(samples, "rate_limit"),
		TimeoutCount:   probeErrorCount(samples, "timeout"),
		CooldownUntil:  cooldownUntil,
		CircuitState:   circuitState,
		SnapshotAt:     result.CheckedAt,
	}

	updated := account
	metadata := cloneMap(account.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["health_score"] = snapshot.SuccessRate
	metadata["health_state"] = status
	metadata["last_probe_at"] = result.CheckedAt.Format(time.RFC3339)
	metadata["last_probe_ok"] = result.OK
	metadata["last_probe_latency_ms"] = result.LatencyMS
	metadata["last_probe_status_code"] = result.StatusCode
	metadata["consecutive_probe_failures"] = consecutiveFailures
	for key, value := range result.Metadata {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		metadata["last_probe_"+key] = value
	}
	errorClass := strings.TrimSpace(result.ErrorClass)
	if errorClass == "" {
		errorClass = "probe_failed"
	}
	if unhealthy {
		metadata["cooldown_active"] = true
		metadata["cooldown_reason"] = errorClass
		metadata["cooldown_until"] = cooldownUntil.Format(time.RFC3339)
		metadata["circuit_open"] = true
		if !result.OK {
			metadata["last_error_class"] = errorClass
			metadata["last_error_message"] = errorClass
		}
	} else if result.OK {
		delete(metadata, "cooldown_active")
		delete(metadata, "cooldown_reason")
		delete(metadata, "cooldown_until")
		delete(metadata, "circuit_open")
		delete(metadata, "last_error_class")
		delete(metadata, "last_error_message")
	} else {
		metadata["last_error_class"] = errorClass
		metadata["last_error_message"] = errorClass
		delete(metadata, "cooldown_active")
		delete(metadata, "cooldown_reason")
		delete(metadata, "cooldown_until")
		delete(metadata, "circuit_open")
	}
	updated.Metadata = metadata
	updated.UpdatedAt = result.CheckedAt
	return snapshot, updated
}

type probeSample struct {
	success    bool
	latencyMS  int
	errorClass string
}

func probeSamples(result contract.AccountProbeResult, history []contract.AccountHealthSnapshot, limit int) []probeSample {
	samples := make([]probeSample, 0, len(history)+1)
	samples = append(samples, probeSample{
		success:    result.OK,
		latencyMS:  result.LatencyMS,
		errorClass: strings.TrimSpace(result.ErrorClass),
	})
	for _, snapshot := range history {
		samples = append(samples, probeSample{
			success:    snapshot.SuccessRate >= 0.5 && !strings.EqualFold(snapshot.CircuitState, "open"),
			latencyMS:  snapshot.LatencyP50MS,
			errorClass: snapshotStatusErrorClass(snapshot),
		})
		if len(samples) >= limit {
			break
		}
	}
	return samples
}

func consecutiveProbeFailures(samples []probeSample) int {
	count := 0
	for _, sample := range samples {
		if sample.success {
			break
		}
		count++
	}
	return count
}

func probeErrorCount(samples []probeSample, errorClass string) int {
	count := 0
	for _, sample := range samples {
		if strings.EqualFold(sample.errorClass, errorClass) {
			count++
		}
	}
	return count
}

func percentileLatency(values []int, percentile float64) int {
	if len(values) == 0 {
		return 0
	}
	sort.Ints(values)
	idx := int(float64(len(values))*percentile+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func snapshotStatusErrorClass(snapshot contract.AccountHealthSnapshot) string {
	if snapshot.TimeoutCount > 0 {
		return "timeout"
	}
	if snapshot.RateLimitCount > 0 {
		return "rate_limit"
	}
	if !strings.EqualFold(snapshot.Status, "healthy") {
		return strings.TrimSpace(snapshot.Status)
	}
	return ""
}

func metadataOptionalInt(metadata map[string]any, keys ...string) *int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	parsed := intValue(value)
	return &parsed
}

func metadataInt(metadata map[string]any, keys ...string) int {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	return intValue(value)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
		floatValue, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return int(floatValue)
		}
	}
	return 0
}

func metadataFloat32(metadata map[string]any, keys ...string) float32 {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float32:
		return clampRatio(typed)
	case float64:
		return clampRatio(float32(typed))
	case int:
		return clampRatio(float32(typed))
	case int64:
		return clampRatio(float32(typed))
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return clampRatio(float32(parsed))
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 32)
		if err == nil {
			return clampRatio(float32(parsed))
		}
	}
	return 0
}

func metadataOptionalTime(metadata map[string]any, keys ...string) *time.Time {
	value, ok := metadataValue(metadata, keys...)
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		cloned := typed
		return &cloned
	case string:
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(typed))
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func metadataValue(metadata map[string]any, keys ...string) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func proxyQualityMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for _, key := range []string{
		"proxy_provider",
		"proxy_region",
		"proxy_country",
		"proxy_city",
		"proxy_type",
		"egress_ip_hash",
	} {
		if value, ok := metadata[key]; ok {
			out[key] = value
		}
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func normalizeProxyID(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(trimmed)
	if err != nil || id <= 0 {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}

func normalizePositiveIDs(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
