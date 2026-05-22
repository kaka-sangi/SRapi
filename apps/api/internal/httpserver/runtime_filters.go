package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func filterProviders(providers []providercontract.Provider, status, q string) []providercontract.Provider {
	status = strings.TrimSpace(status)
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]providercontract.Provider, 0, len(providers))
	for _, provider := range providers {
		if status != "" && string(provider.Status) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(provider.Name), q) && !strings.Contains(strings.ToLower(provider.DisplayName), q) {
			continue
		}
		out = append(out, provider)
	}
	return out
}

func filterModels(models []modelcontract.Model, status, q string) []modelcontract.Model {
	status = strings.TrimSpace(status)
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]modelcontract.Model, 0, len(models))
	for _, model := range models {
		if status != "" && string(model.Status) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(model.CanonicalName), q) && !strings.Contains(strings.ToLower(model.DisplayName), q) {
			continue
		}
		out = append(out, model)
	}
	return out
}

func filterAccounts(accounts []accountcontract.ProviderAccount, status, providerID string) []accountcontract.ProviderAccount {
	status = strings.TrimSpace(status)
	providerID = strings.TrimSpace(providerID)
	out := make([]accountcontract.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		if status != "" && string(account.Status) != status {
			continue
		}
		if providerID != "" && strconv.Itoa(account.ProviderID) != providerID {
			continue
		}
		out = append(out, account)
	}
	return out
}

func filterUsageLogs(items []usagecontract.UsageLog, userID, model string) []usagecontract.UsageLog {
	userID = strings.TrimSpace(userID)
	model = strings.ToLower(strings.TrimSpace(model))
	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if userID != "" && strconv.Itoa(item.UserID) != userID {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(item.Model), model) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterAuditLogs(items []auditcontract.Log, action, resourceType string) []auditcontract.Log {
	action = strings.TrimSpace(action)
	resourceType = strings.TrimSpace(resourceType)
	out := make([]auditcontract.Log, 0, len(items))
	for _, item := range items {
		if action != "" && item.Action != action {
			continue
		}
		if resourceType != "" && item.ResourceType != resourceType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterBillingLedger(items []billingcontract.LedgerEntry, userID, referenceType string) []billingcontract.LedgerEntry {
	userID = strings.TrimSpace(userID)
	referenceType = strings.TrimSpace(referenceType)
	out := make([]billingcontract.LedgerEntry, 0, len(items))
	for _, item := range items {
		if userID != "" && strconv.Itoa(item.UserID) != userID {
			continue
		}
		if referenceType != "" && item.ReferenceType != referenceType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterPaymentOrders(items []paymentcontract.PaymentOrder, status string) []paymentcontract.PaymentOrder {
	status = strings.TrimSpace(status)
	if status == "" {
		return items
	}
	out := make([]paymentcontract.PaymentOrder, 0, len(items))
	for _, item := range items {
		if string(item.Status) == status {
			out = append(out, item)
		}
	}
	return out
}

func filterOutboxEvents(items []eventscontract.OutboxEvent, status, eventType string) []eventscontract.OutboxEvent {
	status = strings.TrimSpace(status)
	eventType = strings.TrimSpace(eventType)
	out := make([]eventscontract.OutboxEvent, 0, len(items))
	for _, item := range items {
		if status != "" && string(item.Status) != status {
			continue
		}
		if eventType != "" && item.EventType != eventType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterOpsAlerts(items []operationscontract.AlertEvent, status, severity string) []operationscontract.AlertEvent {
	status = strings.TrimSpace(status)
	severity = strings.TrimSpace(severity)
	out := make([]operationscontract.AlertEvent, 0, len(items))
	for _, item := range items {
		if status != "" && string(item.Status) != status {
			continue
		}
		if severity != "" && string(item.Severity) != severity {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterSchedulerDecisions(items []schedulercontract.Decision, requestID, model string) []schedulercontract.Decision {
	requestID = strings.TrimSpace(requestID)
	model = strings.ToLower(strings.TrimSpace(model))
	out := make([]schedulercontract.Decision, 0, len(items))
	for _, item := range items {
		if requestID != "" && item.RequestID != requestID {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(item.Model), model) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func buildSchedulerOverview(decisions []schedulercontract.Decision, usageLogs []usagecontract.UsageLog) apiopenapi.SchedulerOverview {
	selected := 0
	stickyHits := 0
	cacheHits := 0
	strategyCounts := map[string]any{}
	for _, decision := range decisions {
		if decision.SelectedAccountID != nil {
			selected++
		}
		if decision.StickyHit {
			stickyHits++
		}
		if decision.CacheAffinityHit {
			cacheHits++
		}
		key := string(decision.Strategy)
		if key == "" {
			key = "unknown"
		}
		count, _ := strategyCounts[key].(int)
		strategyCounts[key] = count + 1
	}
	return apiopenapi.SchedulerOverview{
		AverageLatencyMs:      averageLatency(usageLogs),
		CacheAffinityHitCount: cacheHits,
		FailedDecisions:       len(decisions) - selected,
		SelectedDecisions:     selected,
		StickyHitCount:        stickyHits,
		StrategyCounts:        apiopenapi.JsonObject(strategyCounts),
		SuccessRate:           usageSuccessRate(usageLogs),
		TotalDecisions:        len(decisions),
	}
}

func buildAccountHealthSnapshot(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) apiopenapi.AccountHealthSnapshot {
	status := accountHealthStatus(account, logs)
	successRate := usageSuccessRate(logs)
	quotaRemainingRatio := accountQuotaRemainingRatio(account)
	return apiopenapi.AccountHealthSnapshot{
		AccountId:           apiopenapi.Id(strconv.Itoa(account.ID)),
		CircuitState:        accountCircuitState(account),
		CooldownReason:      nullableMetadataString(account.Metadata, "cooldown_reason"),
		CooldownUntil:       metadataOptionalTime(account.Metadata, "cooldown_until"),
		ErrorClass:          accountHealthErrorClass(account, logs),
		ErrorRate:           1 - successRate,
		LatencyP50Ms:        latencyPercentile(logs, 50),
		LatencyP95Ms:        latencyPercentile(logs, 95),
		ProviderId:          apiopenapi.Id(strconv.Itoa(account.ProviderID)),
		QuotaExhausted:      accountQuotaExhausted(account, quotaRemainingRatio),
		QuotaRemainingRatio: quotaRemainingRatio,
		RateLimitCount:      errorClassCount(logs, "rate_limit"),
		RuntimeClass:        apiopenapi.RuntimeClass(account.RuntimeClass),
		SnapshotAt:          now,
		Status:              status,
		SuccessRate:         successRate,
		TimeoutCount:        errorClassCount(logs, "timeout"),
	}
}

func buildAccountQuotaSnapshot(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog, now time.Time) apiopenapi.AccountQuotaSnapshot {
	usedTokens := 0
	for _, log := range logs {
		usedTokens += log.TotalTokens
	}
	return apiopenapi.AccountQuotaSnapshot{
		AccountId:      apiopenapi.Id(strconv.Itoa(account.ID)),
		ProviderId:     apiopenapi.Id(strconv.Itoa(account.ProviderID)),
		QuotaLimit:     "unlimited",
		QuotaType:      "monthly_tokens",
		Remaining:      "unlimited",
		RemainingRatio: 1,
		SnapshotAt:     now,
		Used:           strconv.Itoa(usedTokens),
	}
}

func usageLogsForAccount(logs []usagecontract.UsageLog, accountID int) []usagecontract.UsageLog {
	out := make([]usagecontract.UsageLog, 0, len(logs))
	for _, log := range logs {
		if log.AccountID != nil && *log.AccountID == accountID {
			out = append(out, log)
		}
	}
	return out
}

func usageSuccessRate(logs []usagecontract.UsageLog) float32 {
	if len(logs) == 0 {
		return 1
	}
	success := 0
	for _, log := range logs {
		if log.Success {
			success++
		}
	}
	return float32(success) / float32(len(logs))
}

func averageLatency(logs []usagecontract.UsageLog) int {
	if len(logs) == 0 {
		return 0
	}
	total := 0
	for _, log := range logs {
		total += log.LatencyMS
	}
	return total / len(logs)
}

func latencyPercentile(logs []usagecontract.UsageLog, percentile int) int {
	if len(logs) == 0 {
		return 0
	}
	values := make([]int, 0, len(logs))
	for _, log := range logs {
		values = append(values, log.LatencyMS)
	}
	sort.Ints(values)
	index := (len(values)*percentile + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(values) {
		index = len(values)
	}
	return values[index-1]
}

func errorClassCount(logs []usagecontract.UsageLog, errorClass string) int {
	count := 0
	for _, log := range logs {
		if log.ErrorClass != nil && *log.ErrorClass == errorClass {
			count++
		}
	}
	return count
}

func accountHealthErrorClass(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog) *string {
	if value := nullableMetadataString(account.Metadata, "last_error_class"); value != nil {
		return value
	}
	for i := len(logs) - 1; i >= 0; i-- {
		if logs[i].ErrorClass != nil && strings.TrimSpace(*logs[i].ErrorClass) != "" {
			return ptrStringValue(strings.TrimSpace(*logs[i].ErrorClass))
		}
	}
	return nil
}

func accountQuotaRemainingRatio(account accountcontract.ProviderAccount) float32 {
	if value := metadataOptionalFloat(account.Metadata, "remaining_ratio", "quota_remaining_ratio"); value != nil {
		if *value < 0 {
			return 0
		}
		if *value > 1 {
			return 1
		}
		return float32(*value)
	}
	if metadataBool(account.Metadata, "quota_exhausted") {
		return 0
	}
	return 1
}

func accountQuotaExhausted(account accountcontract.ProviderAccount, remainingRatio float32) bool {
	return metadataBool(account.Metadata, "quota_exhausted") || remainingRatio <= 0
}

func nullableMetadataString(metadata map[string]any, key string) *string {
	value := metadataString(metadata, key)
	if value == "" {
		return nil
	}
	return ptrStringValue(value)
}

func metadataOptionalTime(metadata map[string]any, key string) *time.Time {
	value := metadataString(metadata, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func accountHealthStatus(account accountcontract.ProviderAccount, logs []usagecontract.UsageLog) string {
	if account.Status != accountcontract.StatusActive {
		return string(account.Status)
	}
	if len(logs) > 0 && usageSuccessRate(logs) < 0.5 {
		return "degraded"
	}
	return "healthy"
}

func accountCircuitState(account accountcontract.ProviderAccount) string {
	if account.Status == accountcontract.StatusActive {
		return "closed"
	}
	return "open"
}

func filterCapabilityDefinitions(defs []capabilitiescontract.Definition, category, status string) []capabilitiescontract.Definition {
	category = strings.TrimSpace(category)
	status = strings.TrimSpace(status)
	out := make([]capabilitiescontract.Definition, 0, len(defs))
	for _, def := range defs {
		if category != "" && def.Category != category {
			continue
		}
		if status != "" && string(def.Status) != status {
			continue
		}
		out = append(out, def)
	}
	return out
}

func apiKeyAllowsModel(allowed []string, model string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, value := range allowed {
		if value == model {
			return true
		}
	}
	return false
}

func apiKeyAllowsModelReference(allowed []string, resolution modelcontract.ModelResolution) bool {
	if apiKeyAllowsModel(allowed, resolution.Model.CanonicalName) {
		return true
	}
	if resolution.Alias != nil && apiKeyAllowsModel(allowed, resolution.Alias.Alias) {
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func singleValueHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers)*2)
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		value := values[0]
		out[key] = value
		out[http.CanonicalHeaderKey(key)] = value
	}
	return out
}

func writeSSEJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	writeSSEJSONAny(w, payload)
	writeSSEDone(w)
}

func writeSSEEvents(w http.ResponseWriter, events []gatewayservice.StreamEvent) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for _, event := range events {
		if name := strings.TrimSpace(event.Event); name != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", name)
		}
		writeSSEJSONAny(w, event.Data)
	}
	writeSSEDone(w)
}

func writeSSEJSONAny(w http.ResponseWriter, payload any) {
	encoder := json.NewEncoder(w)
	w.Write([]byte("data: "))
	_ = encoder.Encode(payload)
	w.Write([]byte("\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func ptrStringValue(value string) *string {
	return &value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func seedCapabilities() []capabilitiescontract.Definition {
	return capabilitiescontract.DefaultDefinitions()
}
