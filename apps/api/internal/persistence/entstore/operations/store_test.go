package operations

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestCleanupDeletesExpiredOperationalRows(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/operations-retention.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	old := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	fresh := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	createUsageLog(t, client, "req_old_usage", old)
	createUsageLog(t, client, "req_fresh_usage", fresh)
	client.SchedulerDecision.Create().
		SetRequestID("req_old_decision").
		SetAttemptNo(1).
		SetUserID(1).
		SetAPIKeyID(1).
		SetSourceProtocol("openai-compatible").
		SetSourceEndpoint("/v1/chat/completions").
		SetTargetProtocol("openai-compatible").
		SetModel("gpt-4o-mini").
		SetStrategy("balanced").
		SetStrategyVersion("v1").
		SetStrategyConfigHash("sha256:test").
		SetCandidateCount(0).
		SetRejectedCount(0).
		SetScoresJSON(map[string]any{}).
		SetRejectReasonsJSON(map[string]any{}).
		SetStrategyWeightsJSON(map[string]any{}).
		SetCompatibilityWarningsJSON([]string{}).
		SetStickyHit(false).
		SetCacheAffinityHit(false).
		SetEstimatedCost("0.00000000").
		SetCurrency("USD").
		SetCreatedAt(old).
		SetUpdatedAt(old).
		SaveX(ctx)
	client.SchedulerFeedback.Create().
		SetRequestID("req_old_feedback").
		SetDecisionID(1).
		SetAttemptNo(1).
		SetAccountID(1).
		SetProviderID(1).
		SetModel("gpt-4o-mini").
		SetSuccess(false).
		SetLatencyMs(100).
		SetInputTokens(1).
		SetOutputTokens(1).
		SetCachedTokens(0).
		SetActualCost("0.00000000").
		SetCurrency("USD").
		SetCreatedAt(old).
		SetUpdatedAt(old).
		SaveX(ctx)
	client.AuditLog.Create().
		SetAction("old.audit").
		SetResourceType("test").
		SetResourceID("1").
		SetBeforeJSON(map[string]any{}).
		SetAfterJSON(map[string]any{}).
		SetIP("").
		SetUserAgent("").
		SetTraceID("").
		SetCreatedAt(old).
		SetUpdatedAt(old).
		SaveX(ctx)
	client.AccountHealthSnapshot.Create().
		SetAccountID(1).
		SetProviderID(1).
		SetStatus("healthy").
		SetSuccessRate(1).
		SetErrorRate(0).
		SetLatencyP50Ms(50).
		SetLatencyP95Ms(100).
		SetRateLimitCount(0).
		SetTimeoutCount(0).
		SetCircuitState("closed").
		SetSnapshotAt(old).
		SaveX(ctx)

	result, err := store.Cleanup(ctx, contract.RetentionCutoffs{
		UsageLogs:              &cutoff,
		SchedulerDecisions:     &cutoff,
		SchedulerFeedbacks:     &cutoff,
		AuditLogs:              &cutoff,
		AccountHealthSnapshots: &cutoff,
		BatchLimit:             1000,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.UsageLogs != 1 ||
		result.SchedulerDecisions != 1 ||
		result.SchedulerFeedbacks != 1 ||
		result.AuditLogs != 1 ||
		result.AccountHealthSnapshots != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}

	remaining, err := client.UsageLog.Query().Order(entusagelog.ByID()).All(ctx)
	if err != nil {
		t.Fatalf("list remaining usage logs: %v", err)
	}
	if len(remaining) != 1 || remaining[0].RequestID != "req_fresh_usage" {
		t.Fatalf("expected fresh usage log to remain, got %+v", remaining)
	}
}

func TestCleanupRetentionHonorsBatchLimit(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/operations-retention-batch.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	old := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for idx := 0; idx < 5; idx++ {
		createUsageLog(t, client, "req_old_usage_batch_"+strconv.Itoa(idx), old)
	}

	result, err := store.Cleanup(ctx, contract.RetentionCutoffs{
		UsageLogs:  &cutoff,
		BatchLimit: 3,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.UsageLogs != 3 || !result.Limited {
		t.Fatalf("expected limited cleanup of 3 usage logs, got %+v", result)
	}
	remaining, err := client.UsageLog.Query().Order(entusagelog.ByID()).All(ctx)
	if err != nil {
		t.Fatalf("list remaining usage logs: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 retained rows after bounded cleanup, got %+v", remaining)
	}
}

func TestSystemLogQueryMatchesMetadataEvidence(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/operations-system-log-search.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	target, err := store.CreateSystemLog(ctx, contract.OpsSystemLog{
		Level:     contract.OpsSystemLogLevelError,
		Source:    "gateway.auth",
		Message:   "gateway key rejected",
		Metadata:  map[string]any{"attempted_key_prefix": "sk_deadbeef0000", "reason": "deleted_key"},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create target log: %v", err)
	}
	if _, err := store.CreateSystemLog(ctx, contract.OpsSystemLog{
		Level:     contract.OpsSystemLogLevelError,
		Source:    "gateway.auth",
		Message:   "gateway key rejected",
		Metadata:  map[string]any{"attempted_key_prefix": "sk_other000000", "reason": "rate_limited"},
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create other log: %v", err)
	}
	if _, err := store.CreateSystemLog(ctx, contract.OpsSystemLog{
		Level:     contract.OpsSystemLogLevelError,
		Source:    "gateway.auth",
		Message:   "gateway key rejected",
		Metadata:  map[string]any{"reason": "literal%value"},
		CreatedAt: now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("create wildcard log: %v", err)
	}

	list, err := store.ListSystemLogs(ctx, contract.SystemLogListOptions{Query: "DEADBEEF"})
	if err != nil {
		t.Fatalf("list by metadata prefix: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != target.ID {
		t.Fatalf("expected metadata query to match only target, got %+v", list)
	}

	list, err = store.ListSystemLogs(ctx, contract.SystemLogListOptions{Query: "%"})
	if err != nil {
		t.Fatalf("list by literal percent: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Metadata["reason"] != "literal%value" {
		t.Fatalf("expected escaped percent query to match only literal percent metadata, got %+v", list)
	}
}

func TestObservabilityStorePersistsSLOsAlertsAndUsageEvidence(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/operations-observability.db?_fk=1")
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	providerID := 42
	errorClass := "rate_limit"
	createUsageLog(t, client, "req_ops_good", now.Add(-time.Hour))
	client.UsageLog.Create().
		SetRequestID("req_ops_bad").
		SetUserID(1).
		SetAPIKeyID(1).
		SetProviderID(providerID).
		SetSourceProtocol("openai-compatible").
		SetSourceEndpoint("/v1/chat/completions").
		SetTargetProtocol("openai-compatible").
		SetModel("slo-model").
		SetInputTokens(1).
		SetOutputTokens(1).
		SetCachedTokens(0).
		SetTotalTokens(2).
		SetUsageEstimated(false).
		SetLatencyMs(100).
		SetSuccess(false).
		SetErrorClass(errorClass).
		SetCost("0.00000000").
		SetCurrency("USD").
		SetCompatibilityWarningsJSON([]string{"fallback"}).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SaveX(ctx)

	createdSLO, err := store.CreateSLO(ctx, contract.SLODefinition{
		Name:       "Gateway availability",
		SLIType:    contract.SLITypeAvailability,
		Objective:  0.995,
		WindowDays: 28,
		Status:     contract.SLOStatusActive,
		Filter: contract.SLOFilter{
			SourceEndpoint:    "/v1/chat/completions",
			Model:             "slo-model",
			ProviderID:        &providerID,
			ErrorOwnerExclude: []string{"client"},
		},
		AlertPolicy: contract.AlertPolicy{
			Name: "multi_window_burn_rate",
			Thresholds: []contract.BurnRateThreshold{{
				Severity:        contract.AlertSeverityCritical,
				ShortWindow:     time.Hour,
				LongWindow:      6 * time.Hour,
				BurnRate:        14,
				MinRequestCount: 25,
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create slo: %v", err)
	}
	createdSLO.Status = contract.SLOStatusDisabled
	updatedSLO, err := store.UpdateSLO(ctx, createdSLO)
	if err != nil {
		t.Fatalf("update slo: %v", err)
	}
	if updatedSLO.ID == 0 || updatedSLO.Status != contract.SLOStatusDisabled || updatedSLO.Filter.ProviderID == nil || len(updatedSLO.AlertPolicy.Thresholds) != 1 {
		t.Fatalf("unexpected persisted slo: %+v", updatedSLO)
	}

	alert, err := store.CreateAlert(ctx, contract.AlertEvent{
		SLOID:       &createdSLO.ID,
		RuleID:      "slo.gateway.availability",
		Severity:    contract.AlertSeverityWarning,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "slo:gateway:availability",
		Summary:     "Gateway availability burn rate high",
		Details:     map[string]any{"burn_rate": 14},
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}
	ackAt := now.Add(time.Minute)
	ackBy := 7
	alert.Status = contract.AlertStatusAcknowledged
	alert.AcknowledgedAt = &ackAt
	alert.AcknowledgedBy = &ackBy
	updatedAlert, err := store.UpdateAlert(ctx, alert)
	if err != nil {
		t.Fatalf("update alert: %v", err)
	}
	if updatedAlert.AcknowledgedBy == nil || *updatedAlert.AcknowledgedBy != ackBy || updatedAlert.Details["burn_rate"] == nil {
		t.Fatalf("unexpected persisted alert: %+v", updatedAlert)
	}

	logs, err := store.ListUsageLogs(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(logs) != 2 || logs[1].ProviderID == nil || logs[1].ErrorClass == nil {
		t.Fatalf("expected usage evidence with optional fields, got %+v", logs)
	}
	if _, err := store.FindAlertByID(ctx, 999); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected not found mapping, got %v", err)
	}
}

func createUsageLog(t *testing.T, client *ent.Client, requestID string, createdAt time.Time) {
	t.Helper()
	client.UsageLog.Create().
		SetRequestID(requestID).
		SetUserID(1).
		SetAPIKeyID(1).
		SetSourceProtocol("openai-compatible").
		SetSourceEndpoint("/v1/chat/completions").
		SetTargetProtocol("openai-compatible").
		SetModel("gpt-4o-mini").
		SetInputTokens(1).
		SetOutputTokens(1).
		SetCachedTokens(0).
		SetTotalTokens(2).
		SetUsageEstimated(false).
		SetLatencyMs(100).
		SetSuccess(true).
		SetCost("0.00000000").
		SetCurrency("USD").
		SetCompatibilityWarningsJSON([]string{}).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		SaveX(t.Context())
}
