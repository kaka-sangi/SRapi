package memory

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

func TestUpdatePreservesLimitsExpiryAndLastUsed(t *testing.T) {
	store := New()
	ctx := context.Background()
	rpmLimit := 100
	tpmLimit := 1000
	expiresAt := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	created, err := store.Create(ctx, contract.CreateStoredKey{
		UserID:    7,
		Name:      "gateway",
		Prefix:    "sk_prefix",
		Hash:      "hmac-sha256:hash",
		Status:    contract.StatusActive,
		RPMLimit:  &rpmLimit,
		TPMLimit:  &tpmLimit,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	usedAt := expiresAt.Add(-time.Hour)
	if err := store.TouchLastUsed(ctx, created.ID, usedAt); err != nil {
		t.Fatalf("touch last used: %v", err)
	}

	created.Name = "renamed"
	created.Status = contract.StatusDisabled
	updated, err := store.Update(ctx, created)
	if err != nil {
		t.Fatalf("update key: %v", err)
	}
	if updated.RPMLimit == nil || *updated.RPMLimit != rpmLimit {
		t.Fatalf("expected rpm limit %d, got %v", rpmLimit, updated.RPMLimit)
	}
	if updated.TPMLimit == nil || *updated.TPMLimit != tpmLimit {
		t.Fatalf("expected tpm limit %d, got %v", tpmLimit, updated.TPMLimit)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiry %s, got %v", expiresAt, updated.ExpiresAt)
	}
	if updated.LastUsedAt == nil || !updated.LastUsedAt.Equal(usedAt) {
		t.Fatalf("expected last used %s, got %v", usedAt, updated.LastUsedAt)
	}
}

func TestFindByPrefixReturnsDefensiveCopy(t *testing.T) {
	store := New()
	ctx := context.Background()
	rpmLimit := 100
	created, err := store.Create(ctx, contract.CreateStoredKey{
		UserID:        7,
		Name:          "gateway",
		Prefix:        "sk_prefix",
		Hash:          "hmac-sha256:hash",
		Status:        contract.StatusActive,
		Scopes:        []string{"gateway:invoke"},
		AllowedModels: []string{"gpt-4o-mini"},
		GroupIDs:      []int{1},
		RPMLimit:      &rpmLimit,
	})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	found, err := store.FindByPrefix(ctx, created.Prefix)
	if err != nil {
		t.Fatalf("find key: %v", err)
	}
	found.Scopes[0] = "mutated"
	found.AllowedModels[0] = "mutated"
	found.GroupIDs[0] = 99
	*found.RPMLimit = 1

	foundAgain, err := store.FindByPrefix(ctx, created.Prefix)
	if err != nil {
		t.Fatalf("find key again: %v", err)
	}
	if foundAgain.Scopes[0] != "gateway:invoke" || foundAgain.AllowedModels[0] != "gpt-4o-mini" || foundAgain.GroupIDs[0] != 1 {
		t.Fatalf("stored slices were mutated through returned key: %+v", foundAgain)
	}
	if foundAgain.RPMLimit == nil || *foundAgain.RPMLimit != rpmLimit {
		t.Fatalf("stored rpm limit was mutated through returned key: %v", foundAgain.RPMLimit)
	}
}
