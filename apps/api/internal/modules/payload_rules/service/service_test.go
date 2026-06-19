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

func TestResolveOrdersRulesAndParamsDeterministically(t *testing.T) {
	store := payloadrulesmemory.New()
	svc, err := service.New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := t.Context()

	if _, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:       "late rule",
		Enabled:    true,
		Priority:   20,
		Action:     contract.ActionOverride,
		MatchModel: "gpt-*",
		Params: map[string]any{
			"zeta": 1,
			"beta": 2,
		},
	}); err != nil {
		t.Fatalf("create late rule: %v", err)
	}
	if _, err := svc.CreateRule(ctx, contract.CreateRule{
		Name:       "early rule",
		Enabled:    true,
		Priority:   10,
		Action:     contract.ActionDefault,
		MatchModel: "gpt-*",
		Params: map[string]any{
			"nested.two": 2,
			"alpha":      1,
		},
	}); err != nil {
		t.Fatalf("create early rule: %v", err)
	}

	resolved := svc.Resolve(ctx, "gpt-4o", "openai-compatible")
	got := make([]string, 0, len(resolved))
	for _, transform := range resolved {
		got = append(got, transform.Action+":"+transform.Path)
	}
	want := []string{
		"default:alpha",
		"default:nested.two",
		"override:beta",
		"override:zeta",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d transforms, got %+v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected transform order: got %+v want %+v", got, want)
		}
	}
}
