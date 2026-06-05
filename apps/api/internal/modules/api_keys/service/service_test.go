package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

const testPepper = "0123456789abcdef0123456789abcdef"

func TestCreateReturnsPlaintextOnceAndStoresOnlyHash(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)

	created, err := svc.Create(context.Background(), contract.CreateRequest{
		UserID: 42,
		Name:   "default",
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if created.PlaintextKey == "" {
		t.Fatal("expected plaintext key")
	}
	if !strings.HasPrefix(created.PlaintextKey, created.Key.Prefix+"_") {
		t.Fatalf("plaintext key does not contain public prefix")
	}
	if created.Key.Hash != "" {
		t.Fatal("hash must not be returned to caller")
	}

	stored, err := store.FindByPrefix(context.Background(), created.Key.Prefix)
	if err != nil {
		t.Fatalf("find stored key: %v", err)
	}
	if stored.Hash == "" {
		t.Fatal("stored key must include hash")
	}
	if stored.Hash == created.PlaintextKey {
		t.Fatal("stored hash must not equal plaintext key")
	}
	if strings.Contains(stored.Hash, created.PlaintextKey) {
		t.Fatal("stored hash must not contain plaintext key")
	}
}

func TestAuthenticateAcceptsValidKeyAndTouchesLastUsed(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	svc := newTestServiceWithClock(t, store, fixedClock{now: now})
	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 7, Name: "gateway"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	result, err := svc.Authenticate(context.Background(), created.PlaintextKey)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if result.UserID != 7 {
		t.Fatalf("expected user id 7, got %d", result.UserID)
	}
	if result.Key.Hash != "" {
		t.Fatal("authenticated key must not expose hash")
	}
	if result.Key.LastUsedAt == nil || !result.Key.LastUsedAt.Equal(now) {
		t.Fatalf("expected last used at %s, got %v", now, result.Key.LastUsedAt)
	}
}

func TestAuthenticateRejectsTamperedSecretWithoutLeakingPrefixValidity(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 7, Name: "gateway"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	tampered := created.Key.Prefix + "_not-the-secret"

	_, err = svc.Authenticate(context.Background(), tampered)
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestAuthenticateRejectsDisabledKey(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 7, Name: "gateway"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	store.setStatus(created.Key.Prefix, contract.StatusDisabled)

	_, err = svc.Authenticate(context.Background(), created.PlaintextKey)
	if !errors.Is(err, ErrKeyDisabled) {
		t.Fatalf("expected ErrKeyDisabled, got %v", err)
	}
}

func TestAuthenticateRejectsExpiredKey(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Second)
	store := newMemoryStore()
	svc := newTestServiceWithClock(t, store, fixedClock{now: now})
	created, err := svc.Create(context.Background(), contract.CreateRequest{
		UserID:    7,
		Name:      "gateway",
		ExpiresAt: &expiredAt,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	_, err = svc.Authenticate(context.Background(), created.PlaintextKey)
	if !errors.Is(err, ErrKeyExpired) {
		t.Fatalf("expected ErrKeyExpired, got %v", err)
	}
}

func TestUpdateChangesMutableFieldsAndPreservesSecretMaterial(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	rpmLimit := 120
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), contract.CreateRequest{
		UserID:        7,
		Name:          "gateway",
		Scopes:        []string{"gateway:invoke"},
		AllowedModels: []string{"gpt-4o-mini"},
		GroupIDs:      []int{1},
		RPMLimit:      &rpmLimit,
		ExpiresAt:     &expiresAt,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	storedBefore, err := store.FindByPrefix(context.Background(), created.Key.Prefix)
	if err != nil {
		t.Fatalf("find stored key: %v", err)
	}

	name := "updated"
	status := contract.StatusDisabled
	scopes := []string{"gateway:invoke", "usage:read"}
	allowedModels := []string{"claude-sonnet"}
	groupIDs := []int{3, 4}
	updated, err := svc.Update(context.Background(), contract.UpdateRequest{
		UserID:        7,
		KeyID:         created.Key.ID,
		Name:          &name,
		Status:        &status,
		Scopes:        &scopes,
		AllowedModels: &allowedModels,
		GroupIDs:      &groupIDs,
	})
	if err != nil {
		t.Fatalf("update api key: %v", err)
	}
	if updated.Name != name || updated.Status != status || updated.Hash != "" {
		t.Fatalf("unexpected updated key: %+v", updated)
	}
	if strings.Join(updated.Scopes, ",") != "gateway:invoke,usage:read" {
		t.Fatalf("unexpected scopes: %v", updated.Scopes)
	}
	if strings.Join(updated.AllowedModels, ",") != "claude-sonnet" {
		t.Fatalf("unexpected allowed models: %v", updated.AllowedModels)
	}
	if len(updated.GroupIDs) != 2 || updated.GroupIDs[0] != 3 || updated.GroupIDs[1] != 4 {
		t.Fatalf("unexpected group ids: %v", updated.GroupIDs)
	}

	storedAfter, err := store.FindByPrefix(context.Background(), created.Key.Prefix)
	if err != nil {
		t.Fatalf("find updated key: %v", err)
	}
	if storedAfter.Hash != storedBefore.Hash || storedAfter.Prefix != storedBefore.Prefix {
		t.Fatal("update must preserve stored hash and prefix")
	}
	if storedAfter.RPMLimit == nil || *storedAfter.RPMLimit != rpmLimit || storedAfter.ExpiresAt == nil || !storedAfter.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("update must preserve limits and expiry, got rpm=%v expires=%v", storedAfter.RPMLimit, storedAfter.ExpiresAt)
	}
}

func TestUpdateChangesExpiryWhenProvided(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), contract.CreateRequest{
		UserID:    7,
		Name:      "gateway",
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	newExpiry := time.Now().UTC().Add(72 * time.Hour)
	if _, err := svc.Update(context.Background(), contract.UpdateRequest{
		UserID:    7,
		KeyID:     created.Key.ID,
		ExpiresAt: &newExpiry,
	}); err != nil {
		t.Fatalf("update api key: %v", err)
	}

	stored, err := store.FindByPrefix(context.Background(), created.Key.Prefix)
	if err != nil {
		t.Fatalf("find updated key: %v", err)
	}
	if stored.ExpiresAt == nil || !stored.ExpiresAt.Equal(newExpiry) {
		t.Fatalf("expiry must update to the provided timestamp, got %v", stored.ExpiresAt)
	}
}

func TestUpdateRejectsOtherUserKey(t *testing.T) {
	store := newMemoryStore()
	svc := newTestService(t, store)
	created, err := svc.Create(context.Background(), contract.CreateRequest{UserID: 7, Name: "gateway"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	name := "stolen"
	_, err = svc.Update(context.Background(), contract.UpdateRequest{UserID: 8, KeyID: created.Key.ID, Name: &name})
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestNewRejectsShortPepper(t *testing.T) {
	_, err := New(newMemoryStore(), "short", nil)
	if !errors.Is(err, ErrPepperUnavailable) {
		t.Fatalf("expected ErrPepperUnavailable, got %v", err)
	}
}

func TestPrefixFromPlaintextRejectsMalformedKeys(t *testing.T) {
	for _, input := range []string{"", "sk", "sk_only", "pk_prefix_secret", "sk__secret", "sk_prefix_"} {
		if prefix, ok := PrefixFromPlaintext(input); ok {
			t.Fatalf("expected malformed key %q to be rejected, got prefix %q", input, prefix)
		}
	}
}

func newTestService(t *testing.T, store contract.Store) *Service {
	t.Helper()
	return newTestServiceWithClock(t, store, fixedClock{now: time.Now().UTC()})
}

func newTestServiceWithClock(t *testing.T, store contract.Store, clock Clock) *Service {
	t.Helper()
	svc, err := New(store, testPepper, clock)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}
