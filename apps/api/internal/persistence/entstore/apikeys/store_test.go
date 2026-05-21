package apikeys

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreCreatesAndLoadsAPIKey(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	created, err := store.Create(ctx, contract.CreateStoredKey{
		UserID:        42,
		Name:          "default",
		Prefix:        "sk_abcdef",
		Hash:          "hmac-sha256:hash",
		Status:        contract.StatusActive,
		Scopes:        []string{"gateway:invoke"},
		AllowedModels: []string{"gpt-4o-mini"},
		GroupIDs:      []int{2, 1, 2},
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if created.Hash != "hmac-sha256:hash" {
		t.Fatalf("expected stored hash, got %s", created.Hash)
	}
	if len(created.GroupIDs) != 2 || created.GroupIDs[0] != 2 || created.GroupIDs[1] != 1 {
		t.Fatalf("unexpected group ids: %v", created.GroupIDs)
	}

	found, err := store.FindByPrefix(ctx, "sk_abcdef")
	if err != nil {
		t.Fatalf("find by prefix: %v", err)
	}
	if found.ID != created.ID || found.UserID != 42 || found.Scopes[0] != "gateway:invoke" {
		t.Fatalf("unexpected api key: %+v", found)
	}

	usedAt := time.Now().UTC().Truncate(time.Second)
	if err := store.TouchLastUsed(ctx, created.ID, usedAt); err != nil {
		t.Fatalf("touch last used: %v", err)
	}
	found, err = store.FindByPrefix(ctx, "sk_abcdef")
	if err != nil {
		t.Fatalf("find touched key: %v", err)
	}
	if found.LastUsedAt == nil || !found.LastUsedAt.Equal(usedAt) {
		t.Fatalf("expected last used %s, got %v", usedAt, found.LastUsedAt)
	}
}

func TestStoreListByUser(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	if _, err := store.Create(ctx, newKey(1, "sk_first")); err != nil {
		t.Fatalf("create first key: %v", err)
	}
	if _, err := store.Create(ctx, newKey(2, "sk_other")); err != nil {
		t.Fatalf("create other key: %v", err)
	}
	if _, err := store.Create(ctx, newKey(1, "sk_second")); err != nil {
		t.Fatalf("create second key: %v", err)
	}

	keys, err := store.ListByUser(ctx, 1)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	for _, key := range keys {
		if key.UserID != 1 {
			t.Fatalf("unexpected user id in key: %+v", key)
		}
	}
}

func TestStoreUpdateAPIKeyPreservesSecretMaterialAndReplacesGroups(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	created, err := store.Create(ctx, contract.CreateStoredKey{
		UserID:        42,
		Name:          "default",
		Prefix:        "sk_update",
		Hash:          "hmac-sha256:original",
		Status:        contract.StatusActive,
		Scopes:        []string{"gateway:invoke"},
		AllowedModels: []string{"gpt-4o-mini"},
		GroupIDs:      []int{1, 2},
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	created.Name = "renamed"
	created.Status = contract.StatusDisabled
	created.Scopes = []string{"gateway:invoke", "usage:read"}
	created.AllowedModels = []string{"claude-sonnet"}
	created.GroupIDs = []int{3, 3, 4}
	updated, err := store.Update(ctx, created)
	if err != nil {
		t.Fatalf("update api key: %v", err)
	}

	if updated.Name != "renamed" || updated.Status != contract.StatusDisabled {
		t.Fatalf("unexpected updated api key: %+v", updated)
	}
	if updated.Prefix != "sk_update" || updated.Hash != "hmac-sha256:original" {
		t.Fatalf("update must preserve prefix and hash, got prefix=%s hash=%s", updated.Prefix, updated.Hash)
	}
	if len(updated.GroupIDs) != 2 || updated.GroupIDs[0] != 3 || updated.GroupIDs[1] != 4 {
		t.Fatalf("expected replaced unique group ids [3 4], got %v", updated.GroupIDs)
	}

	found, err := store.FindByPrefix(ctx, "sk_update")
	if err != nil {
		t.Fatalf("find updated api key: %v", err)
	}
	if found.Hash != "hmac-sha256:original" || found.Name != "renamed" {
		t.Fatalf("unexpected persisted api key: %+v", found)
	}
}

func newKey(userID int, prefix string) contract.CreateStoredKey {
	return contract.CreateStoredKey{
		UserID: userID,
		Name:   "default",
		Prefix: prefix,
		Hash:   "hmac-sha256:" + prefix,
		Status: contract.StatusActive,
		Scopes: []string{"gateway:invoke"},
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "store.db") + "?_fk=1"
}
