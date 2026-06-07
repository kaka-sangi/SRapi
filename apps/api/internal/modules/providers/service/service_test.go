package service

import (
	"context"
	"errors"
	"testing"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

func TestCreateProviderAndList(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), contract.CreateRequest{
		Name:        "openai-compatible",
		DisplayName: "OpenAI Compatible",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("expected id 1, got %d", created.ID)
	}

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(items))
	}
}

func TestDeleteProviderSoftDeletesAndHides(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()

	created, err := svc.Create(ctx, contract.CreateRequest{
		Name:        "openai-compatible",
		DisplayName: "OpenAI Compatible",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete provider: %v", err)
	}

	// The soft-deleted provider is hidden from lookup and listing, and its name
	// is freed for reuse.
	if _, err := svc.FindByID(ctx, created.ID); err == nil {
		t.Fatalf("expected deleted provider to be unfindable")
	}
	items, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected deleted provider excluded from listing, got %d", len(items))
	}

	// Re-deleting is a not-found, not a crash.
	if err := svc.Delete(ctx, created.ID); err == nil {
		t.Fatalf("expected error deleting an already-deleted provider")
	}
	// Invalid id is rejected.
	if err := svc.Delete(ctx, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for id 0, got %v", err)
	}
}

func TestCreateProviderNormalizesConvenienceCapabilityKeys(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), contract.CreateRequest{
		Name:         "openai-compatible",
		DisplayName:  "OpenAI Compatible",
		AdapterType:  "openai-compatible",
		Protocol:     "openai-compatible",
		Capabilities: map[string]any{"supports_stream": true, "tools": true, "supports_speech": true},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if created.Capabilities[capabilitiescontract.KeyStreaming] != true || created.Capabilities[capabilitiescontract.KeyToolCalling] != true || created.Capabilities[capabilitiescontract.KeyAudioSpeech] != true {
		t.Fatalf("expected canonical provider capability keys, got %+v", created.Capabilities)
	}
}

func TestCreateProviderRejectsUnknownCapabilityKey(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.Create(context.Background(), contract.CreateRequest{
		Name:         "openai-compatible",
		DisplayName:  "OpenAI Compatible",
		AdapterType:  "openai-compatible",
		Protocol:     "openai-compatible",
		Capabilities: map[string]any{"streamng": true},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unknown provider capability, got %v", err)
	}
}

func TestCreateProviderRejectsDuplicateName(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		Name:        "openai-compatible",
		DisplayName: "OpenAI Compatible",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		Name:        "openai-compatible",
		DisplayName: "OpenAI Compatible 2",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
	})
	if !errors.Is(err, ErrProviderExists) {
		t.Fatalf("expected ErrProviderExists, got %v", err)
	}
}
