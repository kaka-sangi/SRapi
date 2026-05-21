package models

import (
	"context"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/models/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreCreatesModelAliasAndMapping(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	family := "gpt-4o"
	contextWindow := 128000
	model, err := store.Create(ctx, contract.CreateStoredModel{
		CanonicalName: "gpt-4o-mini",
		DisplayName:   "GPT-4o mini",
		Family:        &family,
		ContextWindow: &contextWindow,
		Status:        contract.StatusActive,
		Capabilities: []capabilitiescontract.Descriptor{{
			Key:     "streaming",
			Level:   capabilitiescontract.DescriptorLevelRequired,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: "v1",
		}},
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}

	loaded, err := store.FindByCanonicalName(ctx, "GPT-4O-MINI")
	if err != nil {
		t.Fatalf("find by canonical name: %v", err)
	}
	if loaded.ID != model.ID || loaded.Family == nil || *loaded.Family != family || len(loaded.Capabilities) != 1 {
		t.Fatalf("unexpected model: %+v", loaded)
	}

	strategy := "balanced"
	alias, err := store.CreateAlias(ctx, contract.CreateStoredAlias{
		Alias:          "fast-model",
		ModelID:        model.ID,
		StrategyHint:   &strategy,
		FallbackModels: []string{"gpt-4o-mini"},
		Status:         contract.StatusActive,
	})
	if err != nil {
		t.Fatalf("create alias: %v", err)
	}
	foundAlias, err := store.FindByAlias(ctx, "FAST-MODEL")
	if err != nil {
		t.Fatalf("find alias: %v", err)
	}
	if foundAlias.ID != alias.ID || foundAlias.StrategyHint == nil || *foundAlias.StrategyHint != strategy {
		t.Fatalf("unexpected alias: %+v", foundAlias)
	}

	mapping, err := store.CreateMapping(ctx, contract.CreateStoredMapping{
		ModelID:           model.ID,
		ProviderID:        7,
		UpstreamModelName: "upstream-gpt-4o-mini",
		Status:            contract.StatusActive,
		CapabilityOverride: []capabilitiescontract.Descriptor{{
			Key:     "json_mode",
			Level:   capabilitiescontract.DescriptorLevelOptional,
			Status:  capabilitiescontract.DescriptorStatusStable,
			Version: "v1",
		}},
		PricingOverride: map[string]any{"currency": "USD"},
	})
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	foundMapping, err := store.FindMapping(ctx, model.ID, 7, "upstream-gpt-4o-mini")
	if err != nil {
		t.Fatalf("find mapping: %v", err)
	}
	if foundMapping.ID != mapping.ID || len(foundMapping.CapabilityOverride) != 1 || foundMapping.PricingOverride["currency"] != "USD" {
		t.Fatalf("unexpected mapping: %+v", foundMapping)
	}
}

func TestStoreUpdateModelClearsOptionalFields(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	family := "claude"
	contextWindow := 200000
	model, err := store.Create(ctx, contract.CreateStoredModel{
		CanonicalName: "claude-sonnet",
		DisplayName:   "Claude Sonnet",
		Family:        &family,
		ContextWindow: &contextWindow,
		Status:        contract.StatusActive,
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	model.Family = nil
	model.ContextWindow = nil
	model.Status = contract.StatusDisabled
	updated, err := store.Update(ctx, model)
	if err != nil {
		t.Fatalf("update model: %v", err)
	}
	if updated.Family != nil || updated.ContextWindow != nil || updated.Status != contract.StatusDisabled {
		t.Fatalf("unexpected updated model: %+v", updated)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "models.db") + "?_fk=1"
}
