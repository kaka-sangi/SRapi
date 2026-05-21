package accounts

import (
	"context"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreCreatesUpdatesAndListsAccounts(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	proxyID := "proxy-us-east"
	upstreamClient := "codex_cli"
	created, err := store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           9,
		Name:                 "primary",
		RuntimeClass:         contract.RuntimeClassOauthRefresh,
		UpstreamClient:       &upstreamClient,
		CredentialCiphertext: "ciphertext",
		CredentialVersion:    "v1",
		ProxyID:              &proxyID,
		Status:               contract.StatusActive,
		Priority:             10,
		Weight:               1.5,
		Metadata:             map[string]any{"base_url": "https://example.invalid/v1"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if created.ProxyID == nil || *created.ProxyID != proxyID || created.CredentialCiphertext != "ciphertext" || created.CredentialVersion != "v1" {
		t.Fatalf("unexpected account: %+v", created)
	}

	loaded, err := store.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	loaded.Name = "primary-updated"
	loaded.ProxyID = nil
	loaded.Status = contract.StatusDisabled
	updated, err := store.Update(ctx, loaded)
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	if updated.Name != "primary-updated" || updated.ProxyID != nil || updated.Status != contract.StatusDisabled {
		t.Fatalf("unexpected updated account: %+v", updated)
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("unexpected accounts list: %+v", items)
	}
}

func TestStoreListsAccountGroupMemberships(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	account, err := store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           9,
		Name:                 "grouped",
		RuntimeClass:         contract.RuntimeClassAPIKey,
		CredentialCiphertext: "ciphertext",
		CredentialVersion:    "v1",
		Status:               contract.StatusActive,
		Priority:             1,
		Weight:               1,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	groupA, err := client.AccountGroup.Create().SetName("group-a").Save(ctx)
	if err != nil {
		t.Fatalf("create group a: %v", err)
	}
	groupB, err := client.AccountGroup.Create().SetName("group-b").Save(ctx)
	if err != nil {
		t.Fatalf("create group b: %v", err)
	}
	if _, err := client.AccountGroupMember.Create().SetAccountID(account.ID).SetAccountGroupID(groupA.ID).Save(ctx); err != nil {
		t.Fatalf("create group a membership: %v", err)
	}
	if _, err := client.AccountGroupMember.Create().SetAccountID(account.ID).SetAccountGroupID(groupB.ID).Save(ctx); err != nil {
		t.Fatalf("create group b membership: %v", err)
	}

	groupIDs, err := store.ListGroupIDsByAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("list group ids: %v", err)
	}
	if len(groupIDs) != 2 || groupIDs[0] != groupA.ID || groupIDs[1] != groupB.ID {
		t.Fatalf("unexpected group ids: %v", groupIDs)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "accounts.db") + "?_fk=1"
}
