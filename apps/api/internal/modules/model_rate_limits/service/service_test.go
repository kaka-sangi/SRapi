package service_test

import (
	"context"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/service"
	"github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/store/memory"
)

func newService(t *testing.T) *service.Service {
	t.Helper()
	svc, err := service.New(memory.New())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestUpsertAndRPMForModel(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	// No rule yet → unlimited.
	if got := svc.RPMForModel(ctx, 7); got != 0 {
		t.Fatalf("RPMForModel with no rule = %d, want 0", got)
	}

	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 7, RPMLimit: 120, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := svc.RPMForModel(ctx, 7); got != 120 {
		t.Fatalf("RPMForModel enabled = %d, want 120", got)
	}

	// Disabled → 0 (no limit applied).
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 7, RPMLimit: 120, Enabled: false}); err != nil {
		t.Fatalf("upsert disable: %v", err)
	}
	if got := svc.RPMForModel(ctx, 7); got != 0 {
		t.Fatalf("RPMForModel disabled = %d, want 0", got)
	}

	// Zero limit → treated as unlimited even when enabled.
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 7, RPMLimit: 0, Enabled: true}); err != nil {
		t.Fatalf("upsert zero: %v", err)
	}
	if got := svc.RPMForModel(ctx, 7); got != 0 {
		t.Fatalf("RPMForModel zero = %d, want 0", got)
	}
}

func TestConcurrencyForModel(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if got := svc.ConcurrencyForModel(ctx, 5); got != 0 {
		t.Fatalf("ConcurrencyForModel with no rule = %d, want 0", got)
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 5, MaxConcurrency: 8, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := svc.ConcurrencyForModel(ctx, 5); got != 8 {
		t.Fatalf("ConcurrencyForModel enabled = %d, want 8", got)
	}
	if got := svc.RPMForModel(ctx, 5); got != 0 {
		t.Fatalf("RPMForModel with only concurrency set = %d, want 0", got)
	}
	// TPM is independent of RPM and concurrency on the same rule.
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 6, TPMLimit: 5000, Enabled: true}); err != nil {
		t.Fatalf("upsert tpm: %v", err)
	}
	if got := svc.TPMForModel(ctx, 6); got != 5000 {
		t.Fatalf("TPMForModel = %d, want 5000", got)
	}
	if got := svc.RPMForModel(ctx, 6); got != 0 {
		t.Fatalf("RPMForModel with only tpm set = %d, want 0", got)
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 5, MaxConcurrency: 8, Enabled: false}); err != nil {
		t.Fatalf("upsert disable: %v", err)
	}
	if got := svc.ConcurrencyForModel(ctx, 5); got != 0 {
		t.Fatalf("ConcurrencyForModel disabled = %d, want 0", got)
	}
}

func TestUpsertValidationAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 0, RPMLimit: 10, Enabled: true}); err == nil {
		t.Fatal("expected error for non-positive model id")
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 3, RPMLimit: -1, Enabled: true}); err == nil {
		t.Fatal("expected error for negative rpm")
	}
	if err := svc.DeleteLimit(ctx, 3); err == nil {
		t.Fatal("expected not-found deleting absent model limit")
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 3, RPMLimit: 10, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := svc.DeleteLimit(ctx, 3); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// countingStore wraps the memory store and counts FindByModel calls so the test
// can assert the service's short-TTL cache actually collapses the per-request
// reads (RPM + TPM + concurrency all touch the same row).
type countingStore struct {
	*memory.Store
	finds int
}

func (s *countingStore) FindByModel(ctx context.Context, modelID int) (contract.Limit, error) {
	s.finds++
	return s.Store.FindByModel(ctx, modelID)
}

func TestRuleLookupsCacheAndInvalidateOnWrite(t *testing.T) {
	ctx := context.Background()
	cs := &countingStore{Store: memory.New()}
	svc, err := service.New(cs)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 7, RPMLimit: 120, TPMLimit: 9000, MaxConcurrency: 4, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// The upsert invalidated the entry, so the next lookup reads through once;
	// the sibling reads for the same model are then served from cache.
	cs.finds = 0
	if got := svc.RPMForModel(ctx, 7); got != 120 {
		t.Fatalf("RPMForModel = %d, want 120", got)
	}
	if got := svc.TPMForModel(ctx, 7); got != 9000 {
		t.Fatalf("TPMForModel = %d, want 9000", got)
	}
	if got := svc.ConcurrencyForModel(ctx, 7); got != 4 {
		t.Fatalf("ConcurrencyForModel = %d, want 4", got)
	}
	if got := svc.RPMForModel(ctx, 7); got != 120 {
		t.Fatalf("RPMForModel (repeat) = %d, want 120", got)
	}
	if cs.finds != 1 {
		t.Fatalf("expected 1 store read across 4 cached lookups, got %d", cs.finds)
	}

	// A write must invalidate the cached entry so the next read reflects it.
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{ModelID: 7, RPMLimit: 60, Enabled: true}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if got := svc.RPMForModel(ctx, 7); got != 60 {
		t.Fatalf("RPMForModel after update = %d, want 60", got)
	}
	if cs.finds != 2 {
		t.Fatalf("expected 1 more store read after invalidation, got %d total", cs.finds)
	}
}
