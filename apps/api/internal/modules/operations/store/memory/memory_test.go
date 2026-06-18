package memory

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

func TestSystemLogQueryMatchesMetadataEvidence(t *testing.T) {
	store := New()
	ctx := t.Context()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)

	target, err := store.CreateSystemLog(ctx, contract.OpsSystemLog{
		Level:     contract.OpsSystemLogLevelWarn,
		Source:    "gateway.auth",
		Message:   "gateway key rejected",
		Metadata:  map[string]any{"attempted_key_prefix": "sk_deadbeef0000", "reason": "deleted_key"},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create target log: %v", err)
	}
	if _, err := store.CreateSystemLog(ctx, contract.OpsSystemLog{
		Level:     contract.OpsSystemLogLevelWarn,
		Source:    "gateway.auth",
		Message:   "gateway key rejected",
		Metadata:  map[string]any{"attempted_key_prefix": "sk_other000000"},
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create other log: %v", err)
	}

	list, err := store.ListSystemLogs(ctx, contract.SystemLogListOptions{Query: "DEADBEEF"})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != target.ID {
		t.Fatalf("expected metadata query to match only target, got %+v", list)
	}

	cleanup, err := store.CleanupSystemLogs(ctx, contract.SystemLogCleanupFilter{Query: "deleted_key", MaxDelete: 10})
	if err != nil {
		t.Fatalf("cleanup logs: %v", err)
	}
	if cleanup.Matched != 1 || cleanup.Deleted != 1 || cleanup.Limited {
		t.Fatalf("expected metadata cleanup to target one row, got %+v", cleanup)
	}
}
