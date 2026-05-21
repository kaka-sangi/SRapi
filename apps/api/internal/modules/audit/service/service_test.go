package service_test

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
)

func TestRecordStoresHighRiskAdminWriteEvidence(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	svc, err := service.New(auditmemory.New(), clock)
	if err != nil {
		t.Fatalf("new audit service: %v", err)
	}
	actorID := 7

	log, err := svc.Record(t.Context(), contract.RecordRequest{
		ActorUserID:  &actorID,
		Action:       "provider_account.update",
		ResourceType: "provider_account",
		ResourceID:   "42",
		Before:       map[string]any{"status": "active"},
		After:        map[string]any{"status": "disabled", "credential_redacted": true},
		IP:           "127.0.0.1",
		UserAgent:    "srapi-test",
		TraceID:      "req_audit_write",
	})
	if err != nil {
		t.Fatalf("record audit log: %v", err)
	}

	if log.ID == 0 || log.ActorUserID == nil || *log.ActorUserID != actorID || !log.CreatedAt.Equal(clock.now) {
		t.Fatalf("unexpected audit identity fields: %+v", log)
	}
	if log.Action != "provider_account.update" || log.ResourceID != "42" || log.TraceID != "req_audit_write" {
		t.Fatalf("unexpected audit resource fields: %+v", log)
	}
	if log.After["credential_ciphertext"] != nil || log.After["credential_redacted"] != true {
		t.Fatalf("audit after snapshot should stay redacted, got %+v", log.After)
	}
}

func TestRecordClonesSnapshots(t *testing.T) {
	svc, err := service.New(auditmemory.New(), nil)
	if err != nil {
		t.Fatalf("new audit service: %v", err)
	}
	before := map[string]any{"status": "active"}
	after := map[string]any{"status": "disabled"}

	log, err := svc.Record(t.Context(), contract.RecordRequest{
		Action:       "provider.update",
		ResourceType: "provider",
		ResourceID:   "9",
		Before:       before,
		After:        after,
	})
	if err != nil {
		t.Fatalf("record audit log: %v", err)
	}
	before["status"] = "mutated"
	after["status"] = "mutated"

	items, err := svc.List(t.Context())
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(items) != 1 || items[0].ID != log.ID || items[0].Before["status"] != "active" || items[0].After["status"] != "disabled" {
		t.Fatalf("expected cloned audit snapshots, got %+v", items)
	}
}

func TestRecordRejectsAuditWithoutActionOrResourceType(t *testing.T) {
	svc, err := service.New(auditmemory.New(), nil)
	if err != nil {
		t.Fatalf("new audit service: %v", err)
	}
	if _, err := svc.Record(t.Context(), contract.RecordRequest{ResourceType: "provider"}); err == nil {
		t.Fatal("expected missing action to be rejected")
	}
	if _, err := svc.Record(t.Context(), contract.RecordRequest{Action: "provider.update"}); err == nil {
		t.Fatal("expected missing resource type to be rejected")
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
