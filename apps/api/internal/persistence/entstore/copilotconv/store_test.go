package copilotconv

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/copilot/contract"

	_ "github.com/mattn/go-sqlite3"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:"+filepath.Join(t.TempDir(), "copilotconv.db")+"?_fk=1")
	t.Cleanup(func() { _ = client.Close() })
	store, err := New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func TestConversationCRUDAndAdminIsolation(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	// Admin A creates two conversations.
	a1, err := store.Create(ctx, 1, "first", json.RawMessage(`[{"role":"user","content":"hi"}]`))
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if _, err := store.Create(ctx, 1, "second", json.RawMessage(`[]`)); err != nil {
		t.Fatalf("create a2: %v", err)
	}
	// Admin B creates one.
	b1, err := store.Create(ctx, 2, "b-first", json.RawMessage(`[]`))
	if err != nil {
		t.Fatalf("create b1: %v", err)
	}

	// List is scoped per admin.
	listA, err := store.ListByAdmin(ctx, 1, 0)
	if err != nil || len(listA) != 2 {
		t.Fatalf("admin A should see 2 conversations, got %d (err=%v)", len(listA), err)
	}
	listB, _ := store.ListByAdmin(ctx, 2, 0)
	if len(listB) != 1 {
		t.Fatalf("admin B should see 1 conversation, got %d", len(listB))
	}

	// Admin B cannot read admin A's conversation.
	if _, err := store.Get(ctx, 2, a1.ID); err != contract.ErrNotFound {
		t.Fatalf("cross-admin Get must be ErrNotFound, got %v", err)
	}
	// Admin B cannot update or delete admin A's conversation.
	if _, err := store.Update(ctx, 2, a1.ID, "hacked", json.RawMessage(`[]`)); err != contract.ErrNotFound {
		t.Fatalf("cross-admin Update must be ErrNotFound, got %v", err)
	}
	if err := store.Delete(ctx, 2, a1.ID); err != contract.ErrNotFound {
		t.Fatalf("cross-admin Delete must be ErrNotFound, got %v", err)
	}

	// Owner can update + rename + delete.
	updated, err := store.Update(ctx, 1, a1.ID, "renamed-on-save", json.RawMessage(`[{"role":"user","content":"again"}]`))
	if err != nil || updated.Title != "renamed-on-save" {
		t.Fatalf("update: %v title=%q", err, updated.Title)
	}
	renamed, err := store.Rename(ctx, 1, a1.ID, "final-name")
	if err != nil || renamed.Title != "final-name" {
		t.Fatalf("rename: %v title=%q", err, renamed.Title)
	}
	if err := store.Delete(ctx, 1, a1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, 1, a1.ID); err != contract.ErrNotFound {
		t.Fatalf("deleted conversation must be gone, got %v", err)
	}

	// Admin B's data is untouched.
	if _, err := store.Get(ctx, 2, b1.ID); err != nil {
		t.Fatalf("admin B conversation should survive: %v", err)
	}
}
