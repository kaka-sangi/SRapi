package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

func TestCreateEncryptsCredential(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "main",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if created.CredentialCiphertext == "" {
		t.Fatal("expected ciphertext")
	}
	if strings.Contains(created.CredentialCiphertext, "secret-value") {
		t.Fatal("ciphertext leaked plaintext")
	}
}

func TestCreateRejectsMissingCredential(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "main",
		RuntimeClass: contract.RuntimeClassAPIKey,
	})
	if !errors.Is(err, ErrCredentialMissing) {
		t.Fatalf("expected ErrCredentialMissing, got %v", err)
	}
}
