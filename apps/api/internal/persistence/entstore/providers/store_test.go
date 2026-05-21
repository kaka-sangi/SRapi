package providers

import (
	"context"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/providers/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreCreatesUpdatesAndListsProviders(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	created, err := store.Create(ctx, contract.CreateStoredProvider{
		Name:         "openai-compatible",
		DisplayName:  "OpenAI Compatible",
		AdapterType:  "openai-compatible",
		Protocol:     "openai-compatible",
		Status:       contract.StatusActive,
		Capabilities: map[string]any{capabilitiescontract.KeyStreaming: true},
		ConfigSchema: map[string]any{"base_url": "https://api.openai.com/v1"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	found, err := store.FindByName(ctx, "OPENAI-COMPATIBLE")
	if err != nil {
		t.Fatalf("find by name: %v", err)
	}
	if found.ID != created.ID || found.Capabilities[capabilitiescontract.KeyStreaming] != true {
		t.Fatalf("unexpected provider: %+v", found)
	}

	found.DisplayName = "OpenAI Updated"
	found.Status = contract.StatusDisabled
	updated, err := store.Update(ctx, found)
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if updated.DisplayName != "OpenAI Updated" || updated.Status != contract.StatusDisabled {
		t.Fatalf("unexpected updated provider: %+v", updated)
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("unexpected providers list: %+v", items)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "providers.db") + "?_fk=1"
}
