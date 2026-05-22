package operations

import (
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
