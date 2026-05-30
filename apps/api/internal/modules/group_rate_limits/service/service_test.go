package service_test

import (
	"context"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/service"
	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/store/memory"
)

func newService(t *testing.T) *service.Service {
	t.Helper()
	svc, err := service.New(memory.New())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestUpsertAndRPMForGroup(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if got := svc.RPMForGroup(ctx, 4); got != 0 {
		t.Fatalf("RPMForGroup with no rule = %d, want 0", got)
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 4, RPMLimit: 200, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := svc.RPMForGroup(ctx, 4); got != 200 {
		t.Fatalf("RPMForGroup enabled = %d, want 200", got)
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 4, RPMLimit: 200, Enabled: false}); err != nil {
		t.Fatalf("upsert disable: %v", err)
	}
	if got := svc.RPMForGroup(ctx, 4); got != 0 {
		t.Fatalf("RPMForGroup disabled = %d, want 0", got)
	}
}

func TestUpsertValidationAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 0, RPMLimit: 10, Enabled: true}); err == nil {
		t.Fatal("expected error for non-positive group id")
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 2, RPMLimit: -1, Enabled: true}); err == nil {
		t.Fatal("expected error for negative rpm")
	}
	if err := svc.DeleteLimit(ctx, 2); err == nil {
		t.Fatal("expected not-found deleting absent group limit")
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 2, RPMLimit: 10, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := svc.DeleteLimit(ctx, 2); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
