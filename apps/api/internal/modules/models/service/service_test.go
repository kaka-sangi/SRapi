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
