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
		Capabilities: map[string]any{"supports_stream": true, "tools": true},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if created.Capabilities[capabilitiescontract.KeyStreaming] != true || created.Capabilities[capabilitiescontract.KeyToolCalling] != true {
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
