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
	if resolution, ok := svc.Resolve(ctx, "rate_limit", 429, "quota exceeded"); !ok || resolution.Action != contract.ActionExpose {
		t.Fatalf("expected initial expose rule, got %+v matched=%v", resolution, ok)
	}

	enabled := false
	if _, err := svc.UpdateRule(ctx, rule.ID, contract.UpdateRule{Enabled: &enabled}); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	if resolution, ok := svc.Resolve(ctx, "rate_limit", 429, "quota exceeded"); ok {
		t.Fatalf("expected disabled rule to disappear without waiting for TTL, got %+v", resolution)
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
	if resolution, ok := svc.Resolve(ctx, "", 500, "internal provider error"); !ok || resolution.Action != contract.ActionMask {
		t.Fatalf("expected newly created mask rule after cache invalidation, got %+v matched=%v", resolution, ok)
	}
	if err := svc.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if resolution, ok := svc.Resolve(ctx, "", 500, "internal provider error"); ok {
		t.Fatalf("expected deleted rule to disappear without waiting for TTL, got %+v", resolution)
	}
}

func TestResolveReturnsResponseOverridesAndClearsThem(t *testing.T) {
	store := errorpassthroughmemory.New()
	svc, err := service.New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := t.Context()
	status := 422
	rule, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:           "custom invalid request",
		Enabled:        true,
		Action:         contract.ActionMask,
		StatusCodes:    []int{400},
		Classes:        []string{"invalid_request"},
		Keywords:       []string{"schema"},
		ResponseStatus: &status,
		CustomMessage:  " upstream rejected schema ",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	resolution, ok := svc.Resolve(ctx, "invalid_request", 400, "schema mismatch")
	if !ok {
		t.Fatal("expected rule to match")
	}
	if resolution.ResponseStatus == nil || *resolution.ResponseStatus != 422 {
		t.Fatalf("expected response status 422, got %+v", resolution.ResponseStatus)
	}
	if resolution.CustomMessage != "upstream rejected schema" {
		t.Fatalf("expected normalized custom message, got %q", resolution.CustomMessage)
	}

	var clearedStatus *int
	emptyMessage := ""
	if _, err := svc.UpdateRule(ctx, rule.ID, contract.UpdateRule{
		ResponseStatus: &clearedStatus,
		CustomMessage:  &emptyMessage,
	}); err != nil {
		t.Fatalf("clear overrides: %v", err)
	}
	resolution, ok = svc.Resolve(ctx, "invalid_request", 400, "schema mismatch")
	if !ok {
		t.Fatal("expected rule to still match")
	}
	if resolution.ResponseStatus != nil {
		t.Fatalf("expected response status override to clear, got %+v", resolution.ResponseStatus)
	}
	if resolution.CustomMessage != "" {
		t.Fatalf("expected custom message to clear, got %q", resolution.CustomMessage)
	}
}

func TestRejectsInvalidStatusCodes(t *testing.T) {
	store := errorpassthroughmemory.New()
	svc, err := service.New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := t.Context()

	if _, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:        "invalid status",
		Enabled:     true,
		Action:      contract.ActionExpose,
		StatusCodes: []int{99},
	}); err == nil {
		t.Fatal("expected invalid create status code to fail")
	}

	rule, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:        "valid status",
		Enabled:     true,
		Action:      contract.ActionExpose,
		StatusCodes: []int{400},
	})
	if err != nil {
		t.Fatalf("create valid rule: %v", err)
	}
	invalidCodes := []int{600}
	if _, err := svc.UpdateRule(ctx, rule.ID, contract.UpdateRule{StatusCodes: &invalidCodes}); err == nil {
		t.Fatal("expected invalid update status code to fail")
	}
}
