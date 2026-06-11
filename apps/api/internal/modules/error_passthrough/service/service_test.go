package service_test

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/service"
	errorpassthroughmemory "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/store/memory"
)

func TestResolveInvalidatesCacheOnRuleWrites(t *testing.T) {
	store := errorpassthroughmemory.New()
	svc, err := service.New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := t.Context()

	rule, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:        "expose rate limit",
		Enabled:     true,
		Priority:    10,
		Action:      contract.ActionExpose,
		StatusCodes: []int{429},
		Classes:     []string{"rate_limit"},
		Keywords:    []string{"quota"},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if action, ok := svc.Resolve(ctx, "rate_limit", 429, "quota exceeded"); !ok || action != contract.ActionExpose {
		t.Fatalf("expected initial expose rule, got %q matched=%v", action, ok)
	}

	enabled := false
	if _, err := svc.UpdateRule(ctx, rule.ID, contract.UpdateRule{Enabled: &enabled}); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	if action, ok := svc.Resolve(ctx, "rate_limit", 429, "quota exceeded"); ok {
		t.Fatalf("expected disabled rule to disappear without waiting for TTL, got %q", action)
	}

	rule, err = svc.CreateRule(ctx, contract.CreateRule{
		Name:        "mask upstream",
		Enabled:     true,
		Priority:    5,
		Action:      contract.ActionMask,
		StatusCodes: []int{500},
		Keywords:    []string{"internal"},
	})
	if err != nil {
		t.Fatalf("create replacement rule: %v", err)
	}
	if action, ok := svc.Resolve(ctx, "", 500, "internal provider error"); !ok || action != contract.ActionMask {
		t.Fatalf("expected newly created mask rule after cache invalidation, got %q matched=%v", action, ok)
	}
	if err := svc.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if action, ok := svc.Resolve(ctx, "", 500, "internal provider error"); ok {
		t.Fatalf("expected deleted rule to disappear without waiting for TTL, got %q", action)
	}
}
