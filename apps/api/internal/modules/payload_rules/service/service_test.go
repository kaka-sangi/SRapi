package service_test

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/payload_rules/service"
	payloadrulesmemory "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/store/memory"
)

func TestResolveInvalidatesCacheOnRuleWrites(t *testing.T) {
	store := payloadrulesmemory.New()
	svc, err := service.New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := t.Context()

	rule, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:       "default temp",
		Enabled:    true,
		Priority:   10,
		Action:     contract.ActionDefault,
		MatchModel: "gpt-*",
		Params:     map[string]any{"temperature": 0.2},
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	resolved := svc.Resolve(ctx, "gpt-4o", "openai-compatible")
	if len(resolved) != 1 || resolved[0].Path != "temperature" {
		t.Fatalf("expected initial transform, got %+v", resolved)
	}

	enabled := false
	if _, err := svc.UpdateRule(ctx, rule.ID, contract.UpdateRule{Enabled: &enabled}); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
	if resolved = svc.Resolve(ctx, "gpt-4o", "openai-compatible"); len(resolved) != 0 {
		t.Fatalf("expected disabled rule to disappear without waiting for TTL, got %+v", resolved)
	}

	rule, err = svc.CreateRule(ctx, contract.CreateRule{
		Name:       "override effort",
		Enabled:    true,
		Priority:   5,
		Action:     contract.ActionOverride,
		MatchModel: "gpt-*",
		Params:     map[string]any{"reasoning.effort": "low"},
	})
	if err != nil {
		t.Fatalf("create replacement rule: %v", err)
	}
	resolved = svc.Resolve(ctx, "gpt-4o", "openai-compatible")
	if len(resolved) != 1 || resolved[0].Path != "reasoning.effort" {
		t.Fatalf("expected newly created rule after cache invalidation, got %+v", resolved)
	}
	if err := svc.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if resolved = svc.Resolve(ctx, "gpt-4o", "openai-compatible"); len(resolved) != 0 {
		t.Fatalf("expected deleted rule to disappear without waiting for TTL, got %+v", resolved)
	}
}
