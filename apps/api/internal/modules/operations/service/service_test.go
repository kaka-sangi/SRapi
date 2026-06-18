package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func TestCleanupRetentionBuildsConfiguredCutoffs(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	store := &captureRetentionStore{}
	svc, err := New(store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.CleanupRetention(t.Context(), contract.RetentionPolicy{
		UsageLogs:              90 * 24 * time.Hour,
		SchedulerDecisions:     30 * 24 * time.Hour,
		SchedulerFeedbacks:     45 * 24 * time.Hour,
		AuditLogs:              365 * 24 * time.Hour,
		AccountHealthSnapshots: 15 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("cleanup retention: %v", err)
	}
	if result.UsageLogs != 1 {
		t.Fatalf("expected store result, got %+v", result)
	}

	assertCutoff(t, store.cutoffs.UsageLogs, now.Add(-90*24*time.Hour))
	assertCutoff(t, store.cutoffs.SchedulerDecisions, now.Add(-30*24*time.Hour))
	assertCutoff(t, store.cutoffs.SchedulerFeedbacks, now.Add(-45*24*time.Hour))
	assertCutoff(t, store.cutoffs.AuditLogs, now.Add(-365*24*time.Hour))
	assertCutoff(t, store.cutoffs.AccountHealthSnapshots, now.Add(-15*24*time.Hour))
}

func TestCleanupRetentionSkipsDisabledPolicies(t *testing.T) {
	store := &captureRetentionStore{}
	svc, err := New(store, fixedClock{now: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.CleanupRetention(t.Context(), contract.RetentionPolicy{}); err != nil {
		t.Fatalf("cleanup retention: %v", err)
	}
	if store.cutoffs.UsageLogs != nil ||
		store.cutoffs.SchedulerDecisions != nil ||
		store.cutoffs.SchedulerFeedbacks != nil ||
		store.cutoffs.AuditLogs != nil ||
		store.cutoffs.AccountHealthSnapshots != nil {
		t.Fatalf("expected nil cutoffs for disabled retention, got %+v", store.cutoffs)
	}
}

func TestSystemLogsRecordListCleanupAndHealth(t *testing.T) {
	now := time.Date(2026, 5, 28, 15, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	first, err := svc.RecordSystemLog(t.Context(), contract.RecordSystemLogRequest{
		Level:     contract.OpsSystemLogLevelWarn,
		Source:    "gateway",
		Message:   "provider quota warning",
		RequestID: "req_warn",
		TraceID:   "trace_warn",
		Metadata:  map[string]any{"safe": true},
	})
	if err != nil {
		t.Fatalf("record first system log: %v", err)
	}
	if first.ID == 0 || first.CreatedAt.IsZero() {
		t.Fatalf("expected generated id/time, got %+v", first)
	}
	if _, err := svc.RecordSystemLog(t.Context(), contract.RecordSystemLogRequest{
		Level:     contract.OpsSystemLogLevelError,
		Source:    "gateway",
		Message:   "upstream failed",
		RequestID: "req_error",
		CreatedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record second system log: %v", err)
	}

	list, err := svc.ListSystemLogs(t.Context(), contract.SystemLogListOptions{Level: contract.OpsSystemLogLevelWarn, Query: "quota"})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].RequestID != "req_warn" {
		t.Fatalf("unexpected filtered list: %+v", list)
	}
	list, err = svc.ListSystemLogs(t.Context(), contract.SystemLogListOptions{RequestID: "req_warn", TraceID: "trace_warn"})
	if err != nil {
		t.Fatalf("list system logs by request/trace: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].RequestID != "req_warn" {
		t.Fatalf("unexpected request/trace filtered list: %+v", list)
	}

	health, err := svc.SystemLogHealth(t.Context())
	if err != nil {
		t.Fatalf("system log health: %v", err)
	}
	if health.StorageMode != "durable" || !health.Writable || health.Degraded || health.Stale || health.TotalCount != 2 {
		t.Fatalf("unexpected health basics: %+v", health)
	}
	if health.LevelCounts[contract.OpsSystemLogLevelWarn] != 1 || health.LevelCounts[contract.OpsSystemLogLevelError] != 1 {
		t.Fatalf("unexpected level counts: %+v", health.LevelCounts)
	}
	if health.LastErrorAt == nil || health.LastErrorSource != "gateway" || health.LastErrorMessage != "upstream failed" {
		t.Fatalf("unexpected last error evidence: %+v", health)
	}

	cleanup, err := svc.CleanupSystemLogs(t.Context(), contract.SystemLogCleanupFilter{
		Level:     contract.OpsSystemLogLevelWarn,
		MaxDelete: 10,
	})
	if err != nil {
		t.Fatalf("cleanup system logs: %v", err)
	}
	if cleanup.Matched != 1 || cleanup.Deleted != 1 || cleanup.Limited {
		t.Fatalf("unexpected cleanup result: %+v", cleanup)
	}

	remaining, err := svc.ListSystemLogs(t.Context(), contract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list remaining logs: %v", err)
	}
	if remaining.Total != 1 || remaining.Items[0].RequestID != "req_error" {
		t.Fatalf("unexpected remaining logs: %+v", remaining)
	}
}

func TestRecordSystemLogSanitizesMetadataAtServiceBoundary(t *testing.T) {
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	long := strings.Repeat("x", systemLogMetadataMaxString+8)
	created, err := svc.RecordSystemLog(t.Context(), contract.RecordSystemLogRequest{
		Level:   contract.OpsSystemLogLevelError,
		Source:  "gateway",
		Message: "upstream failed",
		Metadata: map[string]any{
			"access_token":              "raw-access-token",
			"authorization":             "Bearer raw-token",
			"cookie":                    "session=raw-cookie",
			"prompt":                    "user prompt",
			"body_excerpt":              `{"prompt":"secret"}`,
			"api_key":                   "sk_111111111111_22222222222222222222222222222222",
			"api_key_id":                42,
			"api_key_prefix":            "sk_111111111111",
			"attempted_key_prefix":      "sk_aaaaaaaaaaaa",
			"deleted_key_id":            24,
			"deleted_key_owner_user_id": 7,
			"deleted_key_name":          "deleted-gateway",
			"max_tokens":                8192,
			"prompt_tokens":             32,
			"provider_error_url":        "https://upstream.example/v1?access_token=raw&client_secret=hidden",
			"provider_error_detail":     "provider said Authorization: Bearer nested-token refresh_token: nested-refresh key sk_111111111111_22222222222222222222222222222222",
			"long":                      long,
			"opaque":                    struct{ Secret string }{Secret: "raw-secret"},
			"headers": map[string]any{
				"Authorization": "Bearer nested",
			},
			"safe_nested": map[string]any{
				"request_id":    "req_safe",
				"refresh_token": "nested-refresh",
			},
		},
	})
	if err != nil {
		t.Fatalf("record system log: %v", err)
	}
	meta := created.Metadata
	for _, key := range []string{"access_token", "authorization", "cookie", "prompt", "body_excerpt", "api_key", "headers"} {
		if meta[key] != "[REDACTED]" {
			t.Fatalf("expected %s to be redacted, got %#v in %#v", key, meta[key], meta)
		}
	}
	if meta["api_key_id"] != 42 || meta["api_key_prefix"] != "sk_111111111111" || meta["attempted_key_prefix"] != "sk_aaaaaaaaaaaa" ||
		meta["deleted_key_id"] != 24 || meta["deleted_key_owner_user_id"] != 7 || meta["deleted_key_name"] != "deleted-gateway" {
		t.Fatalf("expected low-sensitive api key references preserved, got %#v", meta)
	}
	if meta["max_tokens"] != 8192 || meta["prompt_tokens"] != 32 {
		t.Fatalf("token count fields should be preserved, got %#v", meta)
	}
	if got, _ := meta["provider_error_url"].(string); got != "https://upstream.example/v1?access_token=[REDACTED]&client_secret=[REDACTED]" {
		t.Fatalf("expected secret query values scrubbed, got %q", got)
	}
	if got, _ := meta["provider_error_detail"].(string); got != "provider said Authorization: Bearer [REDACTED] refresh_token: [REDACTED] key sk_111111111111_[REDACTED]" {
		t.Fatalf("expected bearer credential scrubbed, got %q", got)
	}
	if got, _ := meta["opaque"].(string); got != "struct { Secret string }" {
		t.Fatalf("expected unknown metadata type name only, got %q", got)
	}
	if got, _ := meta["long"].(string); !strings.HasSuffix(got, "...[TRUNCATED]") {
		t.Fatalf("expected long metadata string truncated, got length=%d", len(got))
	}
	nested, ok := meta["safe_nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected safe_nested object, got %#v", meta["safe_nested"])
	}
	if nested["request_id"] != "req_safe" || nested["refresh_token"] != "[REDACTED]" {
		t.Fatalf("unexpected nested metadata: %#v", nested)
	}
}

func TestRecordSystemLogSanitizesMessageAtServiceBoundary(t *testing.T) {
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	longSuffix := strings.Repeat("x", systemLogMetadataMaxString)
	created, err := svc.RecordSystemLog(t.Context(), contract.RecordSystemLogRequest{
		Level:   contract.OpsSystemLogLevelError,
		Source:  "gateway",
		Message: "upstream Authorization: Bearer raw-token refresh_token=raw-refresh api_key=sk-abc123456789 " + longSuffix,
	})
	if err != nil {
		t.Fatalf("record system log: %v", err)
	}
	if strings.Contains(created.Message, "raw-token") ||
		strings.Contains(created.Message, "raw-refresh") ||
		strings.Contains(created.Message, "sk-abc123456789") {
		t.Fatalf("system log message leaked secret: %q", created.Message)
	}
	if !strings.Contains(created.Message, "Bearer [REDACTED]") ||
		!strings.Contains(created.Message, "refresh_token=[REDACTED]") ||
		!strings.Contains(created.Message, "api_key=[REDACTED]") {
		t.Fatalf("expected redacted markers in system log message, got %q", created.Message)
	}
	if !strings.HasSuffix(created.Message, "...[TRUNCATED]") {
		t.Fatalf("expected long system log message to be truncated, got length=%d", len([]rune(created.Message)))
	}
}

type captureRetentionStore struct {
	cutoffs contract.RetentionCutoffs
}

func (s *captureRetentionStore) Cleanup(_ context.Context, cutoffs contract.RetentionCutoffs) (contract.CleanupResult, error) {
	s.cutoffs = cutoffs
	return contract.CleanupResult{UsageLogs: 1}, nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func assertCutoff(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil || !got.Equal(want) {
		t.Fatalf("expected cutoff %s, got %v", want, got)
	}
}

func TestCreateAndListSLOEvaluatesAvailabilityBurnRate(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_good", SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", Success: true, CreatedAt: now.Add(-time.Hour)},
		{RequestID: "req_provider_bad", SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-time.Hour)},
		{RequestID: "req_client_bad", SourceEndpoint: "/v1/chat/completions", Model: "gpt-4o-mini", Success: false, ErrorClass: ptrString("invalid_request"), CreatedAt: now.Add(-time.Hour)},
		{RequestID: "req_other_endpoint", SourceEndpoint: "/v1/messages", Model: "gpt-4o-mini", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-time.Hour)},
	}
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.CreateSLO(t.Context(), contract.CreateSLORequest{
		Name:       "Chat availability",
		SLIType:    contract.SLITypeAvailability,
		Objective:  0.99,
		WindowDays: 28,
		Filter: contract.SLOFilter{
			SourceEndpoint:    "/v1/chat/completions",
			ErrorOwnerExclude: []string{"client", "business"},
		},
	})
	if err != nil {
		t.Fatalf("create slo: %v", err)
	}
	if created.Status != contract.SLOStatusActive || created.AlertPolicy.Name != "multi_window_burn_rate" {
		t.Fatalf("unexpected created slo defaults: %+v", created)
	}

	items, err := svc.ListSLOs(t.Context())
	if err != nil {
		t.Fatalf("list slos: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one slo, got %+v", items)
	}
	evaluation := items[0].Evaluation
	if evaluation.TotalRequests != 2 || evaluation.GoodRequests != 1 || evaluation.BadRequests != 1 {
		t.Fatalf("unexpected evaluation counts: %+v", evaluation)
	}
	if evaluation.BurnRate < 49.9 || evaluation.BurnRate > 50.1 {
		t.Fatalf("expected burn rate near 50, got %+v", evaluation)
	}
}

func TestAcknowledgeAlertMarksActorAndTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	alert, err := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "slo_chat_availability",
		Severity:    contract.AlertSeverityCritical,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "sha256:test",
		Summary:     "burn rate high",
		Details:     map[string]any{"burn_rate": 14.4},
		StartedAt:   now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	updated, err := svc.AcknowledgeAlert(t.Context(), alert.ID, contract.AckAlertRequest{ActorUserID: 42})
	if err != nil {
		t.Fatalf("ack alert: %v", err)
	}
	if updated.Status != contract.AlertStatusAcknowledged || updated.AcknowledgedAt == nil || !updated.AcknowledgedAt.Equal(now) || updated.AcknowledgedBy == nil || *updated.AcknowledgedBy != 42 {
		t.Fatalf("unexpected acknowledged alert: %+v", updated)
	}
}

func TestEvaluateSLOAlertsCreatesUpdatesAndResolvesBurnRateAlerts(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	store := newCaptureObservabilityStore()
	svc, err := NewWithStores(nil, store, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.CreateSLO(t.Context(), contract.CreateSLORequest{
		Name:       "Chat availability",
		SLIType:    contract.SLITypeAvailability,
		Objective:  0.99,
		WindowDays: 1,
		Filter: contract.SLOFilter{
			SourceEndpoint:    "/v1/chat/completions",
			ErrorOwnerExclude: []string{"client", "business"},
		},
		AlertPolicy: contract.AlertPolicy{
			Thresholds: []contract.BurnRateThreshold{{
				Severity:        contract.AlertSeverityCritical,
				LongWindow:      time.Hour,
				ShortWindow:     5 * time.Minute,
				BurnRate:        2,
				MinRequestCount: 2,
			}},
		},
	}); err != nil {
		t.Fatalf("create slo: %v", err)
	}
	manualAlert, err := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "manual.operator",
		Severity:    contract.AlertSeverityWarning,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "manual:fingerprint",
		Summary:     "manual alert",
		Details:     map[string]any{"source": "operator"},
		StartedAt:   now.Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed manual alert: %v", err)
	}
	prefixOnlyAlert, err := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "slo.burn_rate.external",
		Severity:    contract.AlertSeverityWarning,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "external:fingerprint",
		Summary:     "external alert",
		Details:     map[string]any{"source": "external"},
		StartedAt:   now.Add(-9 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed prefix-only alert: %v", err)
	}
	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_1", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("upstream_error"), CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_bad_2", SourceEndpoint: "/v1/chat/completions", Success: false, ErrorClass: ptrString("timeout"), CreatedAt: now.Add(-3 * time.Minute)},
	}

	result, err := svc.EvaluateSLOAlerts(t.Context())
	if err != nil {
		t.Fatalf("evaluate slo alerts: %v", err)
	}
	if result.Evaluated != 1 || result.Breached != 1 || result.Created != 1 || result.Updated != 0 || result.Resolved != 0 {
		t.Fatalf("unexpected create result: %+v", result)
	}
	alerts, err := svc.ListAlerts(t.Context())
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(alerts) != 3 {
		t.Fatalf("expected manual, prefix-only, and burn-rate alerts, got %+v", alerts)
	}
	burnAlert := findAlertByRule(t, alerts, "slo.burn_rate.critical")
	if burnAlert.Status != contract.AlertStatusFiring || burnAlert.ResolvedAt != nil {
		t.Fatalf("unexpected new burn-rate alert: %+v", burnAlert)
	}
	if burnAlert.Details["long_window_seconds"] != int(time.Hour/time.Second) || burnAlert.Details["short_window_seconds"] != int((5*time.Minute)/time.Second) {
		t.Fatalf("unexpected burn-rate alert windows: %+v", burnAlert.Details)
	}

	result, err = svc.EvaluateSLOAlerts(t.Context())
	if err != nil {
		t.Fatalf("reevaluate slo alerts: %v", err)
	}
	if result.Created != 0 || result.Updated != 1 || result.Resolved != 0 {
		t.Fatalf("expected existing burn-rate alert update, got %+v", result)
	}

	store.usageLogs = []usagecontract.UsageLog{
		{RequestID: "req_ok_1", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-2 * time.Minute)},
		{RequestID: "req_ok_2", SourceEndpoint: "/v1/chat/completions", Success: true, CreatedAt: now.Add(-3 * time.Minute)},
	}
	result, err = svc.EvaluateSLOAlerts(t.Context())
	if err != nil {
		t.Fatalf("evaluate recovery: %v", err)
	}
	if result.Breached != 0 || result.Resolved != 1 {
		t.Fatalf("expected burn-rate alert resolution, got %+v", result)
	}
	manualAfter, err := store.FindAlertByID(t.Context(), manualAlert.ID)
	if err != nil {
		t.Fatalf("find manual alert: %v", err)
	}
	if manualAfter.Status != contract.AlertStatusFiring {
		t.Fatalf("manual alert should not be auto-resolved: %+v", manualAfter)
	}
	prefixOnlyAfter, err := store.FindAlertByID(t.Context(), prefixOnlyAlert.ID)
	if err != nil {
		t.Fatalf("find prefix-only alert: %v", err)
	}
	if prefixOnlyAfter.Status != contract.AlertStatusFiring {
		t.Fatalf("prefix-only alert should not be auto-resolved: %+v", prefixOnlyAfter)
	}
	burnAfter, err := store.FindAlertByID(t.Context(), burnAlert.ID)
	if err != nil {
		t.Fatalf("find burn-rate alert: %v", err)
	}
	if burnAfter.Status != contract.AlertStatusResolved || burnAfter.ResolvedAt == nil || !burnAfter.ResolvedAt.Equal(now) {
		t.Fatalf("expected resolved burn-rate alert, got %+v", burnAfter)
	}
}

type captureObservabilityStore struct {
	nextSLOID     int
	nextAlertID   int
	nextRuleID    int
	nextSilenceID int
	nextLogID     int
	slos          map[int]contract.SLODefinition
	alerts        map[int]contract.AlertEvent
	rules         map[int]contract.AlertRule
	silences      map[int]contract.AlertSilence
	systemLogs    []contract.OpsSystemLog
	usageLogs     []usagecontract.UsageLog
}

func newCaptureObservabilityStore() *captureObservabilityStore {
	return &captureObservabilityStore{
		nextSLOID:     1,
		nextAlertID:   1,
		nextRuleID:    1,
		nextSilenceID: 1,
		nextLogID:     1,
		slos:          map[int]contract.SLODefinition{},
		alerts:        map[int]contract.AlertEvent{},
		rules:         map[int]contract.AlertRule{},
		silences:      map[int]contract.AlertSilence{},
	}
}

func (s *captureObservabilityStore) CreateSystemLog(_ context.Context, input contract.OpsSystemLog) (contract.OpsSystemLog, error) {
	input.ID = s.nextLogID
	s.nextLogID++
	s.systemLogs = append(s.systemLogs, input)
	return input, nil
}

func (s *captureObservabilityStore) ListSystemLogs(_ context.Context, opts contract.SystemLogListOptions) (contract.SystemLogList, error) {
	items := make([]contract.OpsSystemLog, 0, len(s.systemLogs))
	filter := contract.SystemLogCleanupFilter{
		Level:     opts.Level,
		Source:    opts.Source,
		Query:     opts.Query,
		RequestID: opts.RequestID,
		TraceID:   opts.TraceID,
		Start:     opts.Start,
		End:       opts.End,
	}
	for _, item := range s.systemLogs {
		if systemLogMatches(item, filter) {
			items = append(items, item)
		}
	}
	sortSystemLogsNewestFirst(items)
	return contract.SystemLogList{Items: items, Total: len(items)}, nil
}

func (s *captureObservabilityStore) SystemLogStats(context.Context) (contract.SystemLogStats, error) {
	stats := contract.SystemLogStats{
		TotalCount:  len(s.systemLogs),
		LevelCounts: map[contract.OpsSystemLogLevel]int{},
	}
	for _, item := range s.systemLogs {
		stats.LevelCounts[item.Level]++
		if stats.LastLog == nil || systemLogIsNewerForTest(item, *stats.LastLog) {
			cloned := item
			stats.LastLog = &cloned
		}
		if item.Level != contract.OpsSystemLogLevelError {
			continue
		}
		if stats.LastError == nil || systemLogIsNewerForTest(item, *stats.LastError) {
			cloned := item
			stats.LastError = &cloned
		}
	}
	return stats, nil
}

func (s *captureObservabilityStore) CleanupSystemLogs(_ context.Context, filter contract.SystemLogCleanupFilter) (contract.SystemLogCleanupResult, error) {
	kept := s.systemLogs[:0]
	var matched, deleted int
	for _, item := range s.systemLogs {
		if !systemLogMatches(item, filter) {
			kept = append(kept, item)
			continue
		}
		matched++
		if filter.DryRun || deleted >= filter.MaxDelete {
			kept = append(kept, item)
			continue
		}
		deleted++
	}
	if !filter.DryRun {
		s.systemLogs = kept
	}
	return contract.SystemLogCleanupResult{
		Matched:   matched,
		Deleted:   deleted,
		DryRun:    filter.DryRun,
		MaxDelete: filter.MaxDelete,
		Limited:   matched > deleted && !filter.DryRun,
	}, nil
}

func systemLogIsNewerForTest(left, right contract.OpsSystemLog) bool {
	if left.CreatedAt.Equal(right.CreatedAt) {
		return left.ID > right.ID
	}
	return left.CreatedAt.After(right.CreatedAt)
}

func (s *captureObservabilityStore) CreateSLO(_ context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	input.ID = s.nextSLOID
	s.nextSLOID++
	s.slos[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) UpdateSLO(_ context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	if _, ok := s.slos[input.ID]; !ok {
		return contract.SLODefinition{}, ErrNotFound
	}
	s.slos[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) DeleteSLO(_ context.Context, id int) error {
	if _, ok := s.slos[id]; !ok {
		return ErrNotFound
	}
	delete(s.slos, id)
	return nil
}

func (s *captureObservabilityStore) FindSLOByID(_ context.Context, id int) (contract.SLODefinition, error) {
	value, ok := s.slos[id]
	if !ok {
		return contract.SLODefinition{}, ErrNotFound
	}
	return value, nil
}

func (s *captureObservabilityStore) ListSLOs(_ context.Context) ([]contract.SLODefinition, error) {
	out := make([]contract.SLODefinition, 0, len(s.slos))
	for _, value := range s.slos {
		out = append(out, value)
	}
	return out, nil
}

func (s *captureObservabilityStore) CreateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	input.ID = s.nextAlertID
	s.nextAlertID++
	s.alerts[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) UpdateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	if _, ok := s.alerts[input.ID]; !ok {
		return contract.AlertEvent{}, ErrNotFound
	}
	s.alerts[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) FindAlertByID(_ context.Context, id int) (contract.AlertEvent, error) {
	value, ok := s.alerts[id]
	if !ok {
		return contract.AlertEvent{}, ErrNotFound
	}
	return value, nil
}

func (s *captureObservabilityStore) ListAlerts(_ context.Context) ([]contract.AlertEvent, error) {
	out := make([]contract.AlertEvent, 0, len(s.alerts))
	for _, value := range s.alerts {
		out = append(out, value)
	}
	return out, nil
}

func (s *captureObservabilityStore) ListUsageLogs(_ context.Context) ([]usagecontract.UsageLog, error) {
	return append([]usagecontract.UsageLog(nil), s.usageLogs...), nil
}

func (s *captureObservabilityStore) ListUsageLogsSince(_ context.Context, since time.Time) ([]usagecontract.UsageLog, error) {
	out := make([]usagecontract.UsageLog, 0, len(s.usageLogs))
	for _, log := range s.usageLogs {
		if since.IsZero() || !log.CreatedAt.Before(since) {
			out = append(out, log)
		}
	}
	return out, nil
}

func (s *captureObservabilityStore) CreateAlertRule(_ context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	input.ID = s.nextRuleID
	s.nextRuleID++
	s.rules[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) UpdateAlertRule(_ context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	if _, ok := s.rules[input.ID]; !ok {
		return contract.AlertRule{}, ErrNotFound
	}
	s.rules[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) FindAlertRuleByID(_ context.Context, id int) (contract.AlertRule, error) {
	value, ok := s.rules[id]
	if !ok {
		return contract.AlertRule{}, ErrNotFound
	}
	return value, nil
}

func (s *captureObservabilityStore) ListAlertRules(_ context.Context) ([]contract.AlertRule, error) {
	out := make([]contract.AlertRule, 0, len(s.rules))
	for _, value := range s.rules {
		out = append(out, value)
	}
	return out, nil
}

func (s *captureObservabilityStore) DeleteAlertRule(_ context.Context, id int) error {
	if _, ok := s.rules[id]; !ok {
		return ErrNotFound
	}
	delete(s.rules, id)
	return nil
}

func (s *captureObservabilityStore) CreateAlertSilence(_ context.Context, input contract.AlertSilence) (contract.AlertSilence, error) {
	input.ID = s.nextSilenceID
	s.nextSilenceID++
	s.silences[input.ID] = input
	return input, nil
}

func (s *captureObservabilityStore) ListAlertSilences(_ context.Context) ([]contract.AlertSilence, error) {
	out := make([]contract.AlertSilence, 0, len(s.silences))
	for _, value := range s.silences {
		out = append(out, value)
	}
	return out, nil
}

func (s *captureObservabilityStore) DeleteAlertSilence(_ context.Context, id int) error {
	if _, ok := s.silences[id]; !ok {
		return ErrNotFound
	}
	delete(s.silences, id)
	return nil
}

func findAlertByRule(t *testing.T, alerts []contract.AlertEvent, ruleID string) contract.AlertEvent {
	t.Helper()
	for _, alert := range alerts {
		if alert.RuleID == ruleID {
			return alert
		}
	}
	t.Fatalf("alert with rule %q not found in %+v", ruleID, alerts)
	return contract.AlertEvent{}
}

func ptrString(value string) *string { return &value }
