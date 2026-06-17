package service

import (
	"context"
	"errors"
	"testing"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/models/contract"
)

func TestCreateModelAndList(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini",
		Capabilities: []capabilitiescontract.Descriptor{{
			Key:     capabilitiescontract.KeyStreaming,
			Level:   capabilitiescontract.DescriptorLevelRequired,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: "v1",
		}},
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("expected id 1, got %d", created.ID)
	}
	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 model, got %d", len(items))
	}
}

func TestCreateModelRejectsUnknownCapabilityKey(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini",
		Capabilities: []capabilitiescontract.Descriptor{{
			Key:     "supports_stream",
			Level:   capabilitiescontract.DescriptorLevelRequired,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: "v1",
		}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for legacy capability key, got %v", err)
	}
}

func TestCreateModelRejectsDuplicateCanonicalName(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini",
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini 2",
	})
	if !errors.Is(err, ErrModelExists) {
		t.Fatalf("expected ErrModelExists, got %v", err)
	}
}

func TestCreateAliasAndResolveModel(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	model, err := svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "claude-sonnet-4",
		DisplayName:   "Claude Sonnet 4",
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}

	alias, err := svc.CreateAlias(context.Background(), model.ID, contract.CreateAliasRequest{
		Alias:          "claude-sonnet",
		FallbackModels: []string{"claude-haiku"},
	})
	if err != nil {
		t.Fatalf("create alias: %v", err)
	}
	if alias.ModelID != model.ID {
		t.Fatalf("expected alias model id %d, got %d", model.ID, alias.ModelID)
	}

	resolved, err := svc.ResolveModel(context.Background(), "claude-sonnet")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	if resolved.CanonicalName != "claude-sonnet-4" {
		t.Fatalf("expected canonical model claude-sonnet-4, got %s", resolved.CanonicalName)
	}

	_, err = svc.CreateAlias(context.Background(), model.ID, contract.CreateAliasRequest{Alias: "claude-sonnet"})
	if !errors.Is(err, ErrAliasExists) {
		t.Fatalf("expected ErrAliasExists, got %v", err)
	}
}

func TestCreateMappingAndListByModel(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	model, err := svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini",
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}

	mapping, err := svc.CreateMapping(context.Background(), model.ID, contract.CreateMappingRequest{
		ProviderID:        10,
		UpstreamModelName: "openai/gpt-4o-mini",
		PricingOverride:   map[string]any{"currency": "USD"},
	})
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	if mapping.ModelID != model.ID || mapping.ProviderID != 10 {
		t.Fatalf("unexpected mapping: %+v", mapping)
	}

	items, err := svc.ListMappingsByModel(context.Background(), model.ID)
	if err != nil {
		t.Fatalf("list mappings: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(items))
	}

	_, err = svc.CreateMapping(context.Background(), model.ID, contract.CreateMappingRequest{
		ProviderID:        10,
		UpstreamModelName: "openai/gpt-4o-mini",
	})
	if !errors.Is(err, ErrMappingExists) {
		t.Fatalf("expected ErrMappingExists, got %v", err)
	}
}

func TestDeleteAliasAndMapping(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	model, err := svc.Create(ctx, contract.CreateRequest{CanonicalName: "gpt-4o-mini", DisplayName: "GPT-4o mini"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	other, err := svc.Create(ctx, contract.CreateRequest{CanonicalName: "claude-3-5", DisplayName: "Claude 3.5"})
	if err != nil {
		t.Fatalf("create other model: %v", err)
	}

	alias, err := svc.CreateAlias(ctx, model.ID, contract.CreateAliasRequest{Alias: "4o-mini"})
	if err != nil {
		t.Fatalf("create alias: %v", err)
	}
	mapping, err := svc.CreateMapping(ctx, model.ID, contract.CreateMappingRequest{ProviderID: 10, UpstreamModelName: "openai/gpt-4o-mini"})
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	// An alias cannot be deleted via the wrong model (cross-model isolation).
	if err := svc.DeleteAlias(ctx, other.ID, alias.ID); !errors.Is(err, ErrAliasNotFound) {
		t.Fatalf("expected ErrAliasNotFound deleting alias under wrong model, got %v", err)
	}
	if err := svc.DeleteMapping(ctx, other.ID, mapping.ID); !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("expected ErrMappingNotFound deleting mapping under wrong model, got %v", err)
	}

	if err := svc.DeleteAlias(ctx, model.ID, alias.ID); err != nil {
		t.Fatalf("delete alias: %v", err)
	}
	if err := svc.DeleteMapping(ctx, model.ID, mapping.ID); err != nil {
		t.Fatalf("delete mapping: %v", err)
	}

	aliases, err := svc.ListAliasesByModel(ctx, model.ID)
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}
	if len(aliases) != 0 {
		t.Fatalf("expected no aliases after delete, got %d", len(aliases))
	}
	mappings, err := svc.ListMappingsByModel(ctx, model.ID)
	if err != nil {
		t.Fatalf("list mappings: %v", err)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected no mappings after delete, got %d", len(mappings))
	}
	// The freed alias name can be recreated.
	if _, err := svc.CreateAlias(ctx, model.ID, contract.CreateAliasRequest{Alias: "4o-mini"}); err != nil {
		t.Fatalf("recreate alias after delete: %v", err)
	}
	// Re-deleting the original alias is a not-found.
	if err := svc.DeleteAlias(ctx, model.ID, alias.ID); !errors.Is(err, ErrAliasNotFound) {
		t.Fatalf("expected ErrAliasNotFound re-deleting alias, got %v", err)
	}
}

func TestCreateMappingRejectsUnknownCapabilityOverride(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	model, err := svc.Create(context.Background(), contract.CreateRequest{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini",
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}

	_, err = svc.CreateMapping(context.Background(), model.ID, contract.CreateMappingRequest{
		ProviderID:        10,
		UpstreamModelName: "openai/gpt-4o-mini",
		CapabilityOverride: []capabilitiescontract.Descriptor{{
			Key:     "tool_callng",
			Level:   capabilitiescontract.DescriptorLevelRequired,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: "v1",
		}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for misspelled capability override, got %v", err)
	}
}

// TestUpdateStatusOnlyToggle locks in the contract the iter-42 frontend
// inline status toggle relies on: PATCH with only Status set must not
// drift display_name / capabilities / other fields.
func TestUpdateStatusOnlyToggle(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	created, err := svc.Create(ctx, contract.CreateRequest{
		CanonicalName: "gpt-4o",
		DisplayName:   "GPT-4o",
		Capabilities: []capabilitiescontract.Descriptor{{
			Key:     capabilitiescontract.KeyStreaming,
			Level:   capabilitiescontract.DescriptorLevelRequired,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: "v1",
		}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != contract.StatusActive {
		t.Fatalf("expected created Status active, got %q", created.Status)
	}
	disabled := contract.StatusDisabled
	updated, err := svc.Update(ctx, created.ID, contract.UpdateRequest{Status: &disabled})
	if err != nil {
		t.Fatalf("update status-only: %v", err)
	}
	if updated.Status != contract.StatusDisabled {
		t.Fatalf("expected Status disabled, got %q", updated.Status)
	}
	if updated.DisplayName != "GPT-4o" {
		t.Fatalf("display_name leaked: %q", updated.DisplayName)
	}
	if len(updated.Capabilities) != 1 {
		t.Fatalf("capabilities leaked: %+v", updated.Capabilities)
	}
}
