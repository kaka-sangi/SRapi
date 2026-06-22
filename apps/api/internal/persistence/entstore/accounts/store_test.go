package accounts

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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

	groupIDsByAccount, err := store.ListGroupIDsByAccounts(ctx, []int{account.ID, account.ID})
	if err != nil {
		t.Fatalf("list group ids by accounts: %v", err)
	}
	got := groupIDsByAccount[account.ID]
	if len(got) != 2 || got[0] != groupA.ID || got[1] != groupB.ID {
		t.Fatalf("unexpected batched group ids: %v", groupIDsByAccount)
	}
}

func TestStoreCreatesUpdatesAndListsProxies(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	created, err := store.CreateProxy(ctx, contract.CreateStoredProxy{
		Name:          "proxy-us-east",
		Type:          contract.ProxyTypeHTTPS,
		URLCiphertext: "ciphertext",
		URLVersion:    "v1",
		Status:        contract.ProxyStatusActive,
		Metadata:      map[string]any{"region": "us-east"},
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if created.URLCiphertext != "ciphertext" || created.URLVersion != "v1" || created.Metadata["region"] != "us-east" {
		t.Fatalf("unexpected proxy: %+v", created)
	}

	loaded, err := store.FindProxyByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find proxy: %v", err)
	}
	loaded.Name = "proxy-us-east-2"
	loaded.Status = contract.ProxyStatusDisabled
	updated, err := store.UpdateProxy(ctx, loaded)
	if err != nil {
		t.Fatalf("update proxy: %v", err)
	}
	if updated.Name != "proxy-us-east-2" || updated.Status != contract.ProxyStatusDisabled {
		t.Fatalf("unexpected updated proxy: %+v", updated)
	}

	items, err := store.ListProxies(ctx)
	if err != nil {
		t.Fatalf("list proxies: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("unexpected proxies list: %+v", items)
	}
}

func TestStoreListsLatestQuotaSnapshotPerType(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	account, err := store.Create(ctx, contract.CreateStoredAccount{
		ProviderID:           9,
		Name:                 "quota-bucketed",
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
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	snapshots := []contract.AccountQuotaSnapshot{
		{QuotaType: "codex_5h_percent", Remaining: "80", RemainingRatio: 0.80, SnapshotAt: now.Add(-3 * time.Minute)},
		{QuotaType: "codex_5h_percent", Remaining: "40", RemainingRatio: 0.40, SnapshotAt: now.Add(-time.Minute)},
		{QuotaType: "codex_7d_percent", Remaining: "90", RemainingRatio: 0.90, SnapshotAt: now.Add(-2 * time.Minute)},
		{QuotaType: "codex_7d_percent", Remaining: "70", RemainingRatio: 0.70, SnapshotAt: now.Add(-30 * time.Second)},
	}
	for _, snapshot := range snapshots {
		snapshot.AccountID = account.ID
		snapshot.ProviderID = account.ProviderID
		snapshot.QuotaLimit = "100"
		if _, err := store.RecordQuotaSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("record quota %s: %v", snapshot.QuotaType, err)
		}
	}

	latest, err := store.ListQuotaSnapshotsByAccount(ctx, account.ID, 1)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected one latest snapshot per quota type, got %+v", latest)
	}
	got := map[string]float32{}
	for _, snapshot := range latest {
		got[snapshot.QuotaType] = snapshot.RemainingRatio
	}
	if got["codex_5h_percent"] != 0.40 || got["codex_7d_percent"] != 0.70 {
		t.Fatalf("expected latest ratio per quota type, got %+v", latest)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "accounts.db") + "?_fk=1"
}

// TestStoreListPagePushesFiltersToSQL anchors the ent.ListPage behaviour
// against the memory implementation: same predicates, same archived hiding,
// same DESC-by-id ordering, same group membership join. Without this anchor a
// drift between the two stores would only surface as a wire-contract bug seen
// in production (the admin handler talks to whichever Store the runtime is
// configured with).
func TestStoreListPagePushesFiltersToSQL(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()
	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()

	mk := func(name string, providerID int, runtime contract.RuntimeClass, status contract.Status) contract.ProviderAccount {
		t.Helper()
		acct, err := store.Create(ctx, contract.CreateStoredAccount{
			ProviderID:           providerID,
			Name:                 name,
			RuntimeClass:         runtime,
			CredentialCiphertext: "ct",
			CredentialVersion:    "v1",
			Status:               status,
			Priority:             0,
			Weight:               1,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return acct
	}

	a := mk("alpha-api-key", 1, contract.RuntimeClassAPIKey, contract.StatusActive)
	b := mk("alpha-oauth", 1, contract.RuntimeClassOauthRefresh, contract.StatusActive)
	c := mk("alpha-needs-reauth", 1, contract.RuntimeClassOauthRefresh, contract.StatusNeedsReauth)
	d := mk("beta-api-key", 2, contract.RuntimeClassAPIKey, contract.StatusActive)
	e := mk("beta-cookie", 2, contract.RuntimeClassWebSessionCookie, contract.StatusDisabled)
	f := mk("beta-archived", 2, contract.RuntimeClassAPIKey, contract.StatusArchived)

	group, err := store.CreateGroup(ctx, contract.CreateStoredAccountGroup{Name: "beta-pool", Status: contract.GroupStatusActive})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := store.AddAccountToGroup(ctx, d.ID, group.ID); err != nil {
		t.Fatalf("add d to group: %v", err)
	}
	if _, err := store.AddAccountToGroup(ctx, e.ID, group.ID); err != nil {
		t.Fatalf("add e to group: %v", err)
	}

	check := func(label string, filter contract.ListFilter, wantTotal int, wantIDs []int) {
		t.Helper()
		result, err := store.ListPage(ctx, filter, 0, 0)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if result.Total != wantTotal {
			t.Errorf("%s total: got %d want %d", label, result.Total, wantTotal)
		}
		got := make([]int, 0, len(result.Items))
		for _, item := range result.Items {
			got = append(got, item.ID)
		}
		if !sameIntSet(got, wantIDs) {
			t.Errorf("%s ids: got %v want %v", label, got, wantIDs)
		}
	}

	check("default hides archived", contract.ListFilter{}, 5, []int{a.ID, b.ID, c.ID, d.ID, e.ID})
	check("include archived surfaces them", contract.ListFilter{IncludeArchived: true}, 6, []int{a.ID, b.ID, c.ID, d.ID, e.ID, f.ID})
	check("status narrows exact", contract.ListFilter{Status: contract.StatusNeedsReauth}, 1, []int{c.ID})
	pid := 2
	check("provider id", contract.ListFilter{ProviderID: &pid}, 2, []int{d.ID, e.ID})
	check("runtime class", contract.ListFilter{RuntimeClass: contract.RuntimeClassWebSessionCookie}, 1, []int{e.ID})
	check("search by name substring", contract.ListFilter{Search: "needs-reauth"}, 1, []int{c.ID})
	check("search by exact id", contract.ListFilter{Search: strconv.Itoa(c.ID)}, 1, []int{c.ID})
	gid := group.ID
	check("group narrows", contract.ListFilter{GroupID: &gid}, 2, []int{d.ID, e.ID})
	check("group + status + provider", contract.ListFilter{GroupID: &gid, Status: contract.StatusActive, ProviderID: &pid}, 1, []int{d.ID})

	// Pagination: limit + offset slice the result set newest-first.
	page1, err := store.ListPage(ctx, contract.ListFilter{}, 2, 0)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if page1.Total != 5 || len(page1.Items) != 2 {
		t.Fatalf("page1 metadata: total=%d items=%d", page1.Total, len(page1.Items))
	}
	if page1.Items[0].ID <= page1.Items[1].ID {
		t.Fatalf("page1 not newest-first: %v", []int{page1.Items[0].ID, page1.Items[1].ID})
	}
	page2, err := store.ListPage(ctx, contract.ListFilter{}, 2, 2)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Items) != 2 || page2.Items[0].ID >= page1.Items[1].ID {
		t.Fatalf("page2 continuity: page1[1]=%d page2[0]=%d", page1.Items[1].ID, page2.Items[0].ID)
	}

	// Group with no members: predicate must short-circuit to "no rows".
	emptyGroup, err := store.CreateGroup(ctx, contract.CreateStoredAccountGroup{Name: "empty-pool", Status: contract.GroupStatusActive})
	if err != nil {
		t.Fatalf("create empty group: %v", err)
	}
	emptyID := emptyGroup.ID
	emptyResult, err := store.ListPage(ctx, contract.ListFilter{GroupID: &emptyID}, 0, 0)
	if err != nil {
		t.Fatalf("empty group: %v", err)
	}
	if emptyResult.Total != 0 || len(emptyResult.Items) != 0 {
		t.Fatalf("empty group must produce zero rows, got total=%d items=%d", emptyResult.Total, len(emptyResult.Items))
	}
}

func sameIntSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

