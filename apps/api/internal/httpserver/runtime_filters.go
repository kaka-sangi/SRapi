package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
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

func filterModelMappings(mappings []modelcontract.ModelProviderMapping, status string) []modelcontract.ModelProviderMapping {
	status = strings.TrimSpace(status)
	out := make([]modelcontract.ModelProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if status != "" && string(mapping.Status) != status {
			continue
		}
		out = append(out, mapping)
	}
	return out
}

func (s *Server) filterAPIPricingRules(ctx context.Context, rules []apiopenapi.PricingRule, modelID, providerID, q string) []apiopenapi.PricingRule {
	modelID = strings.TrimSpace(modelID)
	providerID = strings.TrimSpace(providerID)
	q = strings.ToLower(strings.TrimSpace(q))
	if modelID == "" && providerID == "" && q == "" {
		return rules
	}
	modelLabels := map[string]string{}
	providerLabels := map[string]string{}
	if q != "" {
		modelLabels = s.pricingRuleModelLabels(ctx)
		providerLabels = s.pricingRuleProviderLabels(ctx)
	}
	out := make([]apiopenapi.PricingRule, 0, len(rules))
	for _, rule := range rules {
		if modelID != "" && string(rule.ModelId) != modelID {
			continue
		}
		if providerID != "" && string(rule.ProviderId) != providerID {
			continue
		}
		if q != "" && !pricingRuleMatchesQuery(rule, modelLabels, providerLabels, q) {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func (s *Server) pricingRuleModelLabels(ctx context.Context) map[string]string {
	labels := map[string]string{"0": "model family any model family"}
	if s.runtime == nil || s.runtime.models == nil {
		return labels
	}
	models, err := s.runtime.models.List(ctx)
	if err != nil {
		return labels
	}
	for _, model := range models {
		labels[strconv.Itoa(model.ID)] = rowTextLower(model.CanonicalName, model.DisplayName)
	}
	return labels
}

func (s *Server) pricingRuleProviderLabels(ctx context.Context) map[string]string {
	labels := map[string]string{"0": "any provider"}
	if s.runtime == nil || s.runtime.providers == nil {
		return labels
	}
	providers, err := s.runtime.providers.List(ctx)
	if err != nil {
		return labels
	}
	for _, provider := range providers {
		labels[strconv.Itoa(provider.ID)] = rowTextLower(provider.Name, provider.DisplayName, string(provider.Protocol), string(provider.AdapterType))
	}
	return labels
}

func pricingRuleMatchesQuery(rule apiopenapi.PricingRule, modelLabels map[string]string, providerLabels map[string]string, q string) bool {
	return strings.Contains(rowTextLower(
		string(rule.Id),
		string(rule.ModelId),
		string(rule.ProviderId),
		optionalAPIPricingRuleModelFamily(rule),
		string(rule.BillingMode),
		rule.Currency,
		rule.InputPricePerMillionTokens,
		rule.OutputPricePerMillionTokens,
		rule.PerRequestPrice,
		modelLabels[string(rule.ModelId)],
		providerLabels[string(rule.ProviderId)],
	), q)
}

func optionalAPIPricingRuleModelFamily(rule apiopenapi.PricingRule) string {
	if rule.ModelFamily == nil {
		return ""
	}
	return *rule.ModelFamily
}

func rowTextLower(values ...string) string {
	return strings.ToLower(strings.Join(values, " "))
}

// usageListFilterFromRequest parses the admin/console usage-log query string
// into the store-level ListFilter. Same keys as the legacy in-memory filter
// (user_id/api_key_id/account_id/provider_id/model/source_endpoint/billing_mode/
// error_class/success/start/end), so the wire contract is unchanged — only the
// execution moves from "load everything, filter in Go" down to "filter in SQL".
// Returns ok=false when any numeric or boolean field is malformed so the caller
// can short-circuit with a 400.
func usageListFilterFromRequest(r *http.Request) (usagecontract.ListFilter, bool) {
	q := r.URL.Query()
	filter := usagecontract.ListFilter{
		Model:          strings.TrimSpace(q.Get("model")),
		SourceEndpoint: strings.TrimSpace(q.Get("source_endpoint")),
		BillingMode:    strings.TrimSpace(q.Get("billing_mode")),
		ErrorClass:     strings.TrimSpace(q.Get("error_class")),
	}
	if start := parseUsageFilterTime(q.Get("start")); !start.IsZero() {
		t := start
		filter.Start = &t
	}
	if end := parseUsageFilterTime(q.Get("end")); !end.IsZero() {
		t := end
		filter.End = &t
	}
	if ptr, ok := optionalIDFilter(q.Get("user_id")); ok {
		filter.UserID = ptr
	} else {
		return usagecontract.ListFilter{}, false
	}
	if ptr, ok := optionalIDFilter(q.Get("api_key_id")); ok {
		filter.APIKeyID = ptr
	} else {
		return usagecontract.ListFilter{}, false
	}
	if ptr, ok := optionalIDFilter(q.Get("account_id")); ok {
		filter.AccountID = ptr
	} else {
		return usagecontract.ListFilter{}, false
	}
	if ptr, ok := optionalIDFilter(q.Get("provider_id")); ok {
		filter.ProviderID = ptr
	} else {
		return usagecontract.ListFilter{}, false
	}
	switch strings.TrimSpace(q.Get("success")) {
	case "true":
		v := true
		filter.Success = &v
	case "false":
		v := false
		filter.Success = &v
	case "", "all":
		// unset — match either
	default:
		return usagecontract.ListFilter{}, false
	}
	return filter, true
}

// optionalIDFilter parses a query-string positive integer. An empty string
// returns (nil, true) meaning "no filter". A malformed or non-positive value
// returns (nil, false) so the caller can return a 400 Invalid Request.
func optionalIDFilter(raw string) (*int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, true
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return nil, false
	}
	return &id, true
}

// filterUsageLogs applies the admin usage-log query filters in memory over the
// already-loaded slice. It supports user/model plus account, api-key, provider,
// source-endpoint, billing-mode, error-class, success and a created-at date
// range (start/end as RFC3339 or YYYY-MM-DD), matching sub2api's usage filters.
func filterUsageLogs(items []usagecontract.UsageLog, r *http.Request) []usagecontract.UsageLog {
	q := r.URL.Query()
	userID := strings.TrimSpace(q.Get("user_id"))
	model := strings.ToLower(strings.TrimSpace(q.Get("model")))
	apiKeyID := strings.TrimSpace(q.Get("api_key_id"))
	accountID := strings.TrimSpace(q.Get("account_id"))
	providerID := strings.TrimSpace(q.Get("provider_id"))
	sourceEndpoint := strings.ToLower(strings.TrimSpace(q.Get("source_endpoint")))
	billingMode := strings.TrimSpace(q.Get("billing_mode"))
	errorClass := strings.TrimSpace(q.Get("error_class"))
	success := strings.TrimSpace(q.Get("success"))
	start := parseUsageFilterTime(q.Get("start"))
	end := parseUsageFilterTime(q.Get("end"))

	out := make([]usagecontract.UsageLog, 0, len(items))
	for _, item := range items {
		if userID != "" && strconv.Itoa(item.UserID) != userID {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(item.Model), model) {
			continue
		}
		if apiKeyID != "" && strconv.Itoa(item.APIKeyID) != apiKeyID {
			continue
		}
		if accountID != "" && (item.AccountID == nil || strconv.Itoa(*item.AccountID) != accountID) {
			continue
		}
		if providerID != "" && (item.ProviderID == nil || strconv.Itoa(*item.ProviderID) != providerID) {
			continue
		}
		if sourceEndpoint != "" && !strings.Contains(strings.ToLower(item.SourceEndpoint), sourceEndpoint) {
			continue
		}
		if billingMode != "" && !strings.EqualFold(item.BillingMode, billingMode) {
			continue
		}
		if errorClass != "" && (item.ErrorClass == nil || !strings.EqualFold(*item.ErrorClass, errorClass)) {
			continue
		}
		if success == "true" && !item.Success {
			continue
		}
		if success == "false" && item.Success {
			continue
		}
		if !start.IsZero() && item.CreatedAt.Before(start) {
			continue
		}
		if !end.IsZero() && item.CreatedAt.After(end) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// parseUsageFilterTime accepts RFC3339 or a bare YYYY-MM-DD date; returns the
// zero time when empty/unparseable (treated as no bound).
func parseUsageFilterTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
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

func filterSchedulerDecisions(items []schedulercontract.Decision, requestID, model, sourceEndpoint, accountID, providerID string, start, end *time.Time) []schedulercontract.Decision {
	requestID = strings.TrimSpace(requestID)
	model = strings.ToLower(strings.TrimSpace(model))
	sourceEndpoint = strings.ToLower(strings.TrimSpace(sourceEndpoint))
	accountIDValue, hasAccountID, validAccountID := positiveIDFilter(accountID)
	providerIDValue, hasProviderID, validProviderID := positiveIDFilter(providerID)
	if !validAccountID || !validProviderID {
		return nil
	}
	out := make([]schedulercontract.Decision, 0, len(items))
	for _, item := range items {
		if requestID != "" && item.RequestID != requestID {
			continue
		}
		if start != nil && item.CreatedAt.Before(*start) {
			continue
		}
		if end != nil && !item.CreatedAt.Before(*end) {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(item.Model), model) {
			continue
		}
		if sourceEndpoint != "" && !strings.Contains(strings.ToLower(item.SourceEndpoint), sourceEndpoint) {
			continue
		}
		if hasAccountID && !schedulerDecisionMentionsAccount(item, accountIDValue) {
			continue
		}
		if hasProviderID && (item.SelectedProviderID == nil || *item.SelectedProviderID != providerIDValue) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func schedulerDecisionWindowFromRequest(r *http.Request) (*time.Time, *time.Time, error) {
	q := r.URL.Query()
	start, err := parseOptionalRFC3339(q.Get("start"))
	if err != nil {
		return nil, nil, errors.New("invalid start timestamp")
	}
	end, err := parseOptionalRFC3339(q.Get("end"))
	if err != nil {
		return nil, nil, errors.New("invalid end timestamp")
	}
	if start != nil && end != nil && !start.Before(*end) {
		return nil, nil, errors.New("start must be before end")
	}
	return start, end, nil
}

func schedulerDecisionMentionsAccount(item schedulercontract.Decision, accountID int) bool {
	if item.SelectedAccountID != nil && *item.SelectedAccountID == accountID {
		return true
	}
	if schedulerEvidenceMapHasAccount(item.Scores, accountID) {
		return true
	}
	return schedulerEvidenceMapHasAccount(item.RejectReasons, accountID)
}

func schedulerEvidenceMapHasAccount(values map[string]any, accountID int) bool {
	if len(values) == 0 {
		return false
	}
	for key := range values {
		if schedulerAccountIDFromEvidenceKey(key) == accountID {
			return true
		}
	}
	return false
}

func schedulerAccountIDFromEvidenceKey(key string) int {
	value := strings.TrimSpace(key)
	value = strings.TrimPrefix(value, "account_")
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func positiveIDFilter(raw string) (value int, present bool, valid bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false, true
	}
	id, err := strconv.Atoi(trimmed)
	if err != nil || id <= 0 {
		return 0, true, false
	}
	return id, true, true
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
		QuotaType:      accountcontract.QuotaTypeSyntheticMonthlyTokens,
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

// setSSEResponseHeaders sets the standard Server-Sent-Events headers. The
// X-Accel-Buffering: no header is the important one: without it nginx (and many
// ingress/CDN proxies) buffer the whole response before flushing, which turns
// real token-by-token streaming into a single all-at-once delivery downstream.
func setSSEResponseHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
}

func writeSSEJSONChunks(w http.ResponseWriter, payloads []map[string]any) {
	setSSEResponseHeaders(w)
	for _, payload := range payloads {
		writeSSEJSONAny(w, payload)
	}
	writeSSEDone(w)
}

func writeSSEEvents(w http.ResponseWriter, events []gatewayservice.StreamEvent) {
	writeSSEEventStream(w, events, true)
}

// writeSSEEventsNoDone writes named SSE events without the OpenAI-only
// `data: [DONE]` sentinel. The Anthropic Messages stream terminates with its
// own `message_stop` event; appending [DONE] is a foreign Chat-Completions
// artifact that strict Anthropic clients should not see.
func writeSSEEventsNoDone(w http.ResponseWriter, events []gatewayservice.StreamEvent) {
	writeSSEEventStream(w, events, false)
}

func writeSSEEventStream(w http.ResponseWriter, events []gatewayservice.StreamEvent, done bool) {
	setSSEResponseHeaders(w)
	for _, event := range events {
		if name := strings.TrimSpace(event.Event); name != "" {
			_, _ = fmt.Fprintf(w, "event: %s\n", name)
		}
		writeSSEJSONAny(w, event.Data)
	}
	if done {
		writeSSEDone(w)
	}
}

func writeRawSSEResponse(w http.ResponseWriter, raw []byte) {
	setSSEResponseHeaders(w)
	_, _ = w.Write(raw)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
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
