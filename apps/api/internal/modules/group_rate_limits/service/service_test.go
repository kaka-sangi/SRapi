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

func TestConcurrencyForGroup(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if got := svc.ConcurrencyForGroup(ctx, 9); got != 0 {
		t.Fatalf("ConcurrencyForGroup with no rule = %d, want 0", got)
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 9, MaxConcurrency: 16, Enabled: true}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := svc.ConcurrencyForGroup(ctx, 9); got != 16 {
		t.Fatalf("ConcurrencyForGroup enabled = %d, want 16", got)
	}
	// RPM and concurrency are independent on the same rule.
	if got := svc.RPMForGroup(ctx, 9); got != 0 {
		t.Fatalf("RPMForGroup with only concurrency set = %d, want 0", got)
	}
	// TPM is independent too.
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 11, TPMLimit: 9000, Enabled: true}); err != nil {
		t.Fatalf("upsert tpm: %v", err)
	}
	if got := svc.TPMForGroup(ctx, 11); got != 9000 {
		t.Fatalf("TPMForGroup = %d, want 9000", got)
	}
	if got := svc.ConcurrencyForGroup(ctx, 11); got != 0 {
		t.Fatalf("ConcurrencyForGroup with only tpm set = %d, want 0", got)
	}
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 9, MaxConcurrency: 16, Enabled: false}); err != nil {
		t.Fatalf("upsert disable: %v", err)
	}
	if got := svc.ConcurrencyForGroup(ctx, 9); got != 0 {
		t.Fatalf("ConcurrencyForGroup disabled = %d, want 0", got)
	}
}

// TestBatchSetRPMOverridesAllSuccess pins the happy path: every item updates
// the right group's RPM ceiling, leaving TPM + MaxConcurrency intact on rows
// that already had non-zero values.
func TestBatchSetRPMOverridesAllSuccess(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	if _, err := svc.UpsertLimit(ctx, contract.UpsertLimit{GroupID: 1, RPMLimit: 100, TPMLimit: 2000, MaxConcurrency: 4, Enabled: true}); err != nil {
		t.Fatalf("seed group 1: %v", err)
	}
	five := 500
	twenty := 2000
	items := []contract.BatchSetRPMOverrideItem{
		{GroupID: 1, RPMOverride: &five},
		{GroupID: 2, RPMOverride: &twenty},
	}
	results, err := svc.BatchSetRPMOverrides(ctx, items)
	if err != nil {
		t.Fatalf("BatchSetRPMOverrides: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d failed: %+v", i, row)
		}
	}
	if got := svc.RPMForGroup(ctx, 1); got != 500 {
		t.Fatalf("group 1 RPM: want 500 got %d", got)
	}
	if got := svc.RPMForGroup(ctx, 2); got != 2000 {
		t.Fatalf("group 2 RPM: want 2000 got %d", got)
	}
	if got := svc.ConcurrencyForGroup(ctx, 1); got != 4 {
		t.Fatalf("group 1 MaxConcurrency should be preserved at 4, got %d", got)
	}
}

// TestBatchSetRPMOverridesPerRowFailureSurfaces pins the per-row error
// contract: an invalid id or negative value reports an Error without aborting
// the rest of the batch.
func TestBatchSetRPMOverridesPerRowFailureSurfaces(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	bad := -1
	ok := 100
	items := []contract.BatchSetRPMOverrideItem{
		{GroupID: 1, RPMOverride: &ok},
		{GroupID: 0, RPMOverride: &ok},  // invalid id
		{GroupID: 3, RPMOverride: &bad}, // invalid value
		{GroupID: 4, RPMOverride: nil},  // nil clears — idempotent on missing
	}
	results, err := svc.BatchSetRPMOverrides(ctx, items)
	if err != nil {
		t.Fatalf("BatchSetRPMOverrides: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("row 0 should succeed: %+v", results[0])
	}
	if results[1].Error == "" {
		t.Fatalf("row 1 should report invalid id, got %+v", results[1])
	}
	if results[2].Error == "" {
		t.Fatalf("row 2 should report invalid value, got %+v", results[2])
	}
	if results[3].Error != "" {
		t.Fatalf("row 3 (nil clear on missing) should be idempotent success, got %+v", results[3])
	}
}

// TestBatchSetRPMOverridesDedupesWithinBatch: an accidental double-id must
// surface as a duplicate on the second occurrence.
func TestBatchSetRPMOverridesDedupesWithinBatch(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	a, b := 10, 20
	items := []contract.BatchSetRPMOverrideItem{
		{GroupID: 7, RPMOverride: &a},
		{GroupID: 7, RPMOverride: &b},
	}
	results, err := svc.BatchSetRPMOverrides(ctx, items)
	if err != nil {
		t.Fatalf("BatchSetRPMOverrides: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("first occurrence should succeed: %+v", results[0])
	}
	if results[1].Error != "duplicate id in batch" {
		t.Fatalf("second occurrence should report duplicate, got: %+v", results[1])
	}
}

// TestBatchSetRPMOverridesRejectsEmptyAndOversize: outer error guards.
func TestBatchSetRPMOverridesRejectsEmptyAndOversize(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	if _, err := svc.BatchSetRPMOverrides(ctx, nil); err == nil {
		t.Fatal("empty should fail")
	}
	oversize := make([]contract.BatchSetRPMOverrideItem, service.BatchSetRPMOverridesMaxItems+1)
	for i := range oversize {
		v := 1
		oversize[i] = contract.BatchSetRPMOverrideItem{GroupID: i + 1, RPMOverride: &v}
	}
	if _, err := svc.BatchSetRPMOverrides(ctx, oversize); err == nil {
		t.Fatal(">MaxItems should fail")
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
