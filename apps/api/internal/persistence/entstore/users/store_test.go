package users

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entworkspace "github.com/srapi/srapi/apps/api/ent/workspace"
	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreCreatesAndLoadsUserWithRoles(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	created, err := store.Create(ctx, contract.CreateStoredUser{
		Email:        "Admin@SRapi.Local",
		Name:         "Admin",
		PasswordHash: "hash",
		Status:       contract.StatusActive,
		Roles:        []contract.Role{contract.RoleAdmin, contract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.Email != "admin@srapi.local" {
		t.Fatalf("expected normalized email, got %s", created.Email)
	}
	if len(created.Roles) != 2 || created.Roles[0] != contract.RoleAdmin || created.Roles[1] != contract.RoleUser {
		t.Fatalf("unexpected roles: %v", created.Roles)
	}
	if created.Balance != "0.00000000" || created.Currency != "USD" {
		t.Fatalf("unexpected balance: %s %s", created.Balance, created.Currency)
	}
	if created.WorkspaceID == nil {
		t.Fatalf("expected personal workspace id, got nil")
	}
	workspace, err := client.Workspace.Query().Where(entworkspace.IDEQ(*created.WorkspaceID)).Only(ctx)
	if err != nil {
		t.Fatalf("load personal workspace: %v", err)
	}
	if workspace.Slug != "personal-"+strconv.Itoa(created.ID) || workspace.Type != "personal" || workspace.Status != "active" {
		t.Fatalf("unexpected personal workspace: %+v", workspace)
	}
	if workspace.OwnerUserID == nil || *workspace.OwnerUserID != created.ID {
		t.Fatalf("expected owner user %d, got %v", created.ID, workspace.OwnerUserID)
	}

	loaded, err := store.FindByEmail(ctx, "ADMIN@srapi.local")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	if loaded.ID != created.ID || loaded.PasswordHash != "hash" {
		t.Fatalf("unexpected loaded user: %+v", loaded)
	}

	lastLogin := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateLastLogin(ctx, created.ID, lastLogin); err != nil {
		t.Fatalf("update last login: %v", err)
	}
	loaded, err = store.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if loaded.LastLoginAt == nil || !loaded.LastLoginAt.Equal(lastLogin) {
		t.Fatalf("expected last login %s, got %v", lastLogin, loaded.LastLoginAt)
	}
}

func TestStoreCreatesRoleAndLoadsPermissions(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	role, err := store.CreateRole(ctx, contract.CreateStoredRole{
		Name:        "payment_reader",
		Description: "Payment reader",
		Permissions: []string{contract.PermissionPaymentOrderRead},
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.ID == 0 || len(role.Permissions) != 1 || role.Permissions[0] != contract.PermissionPaymentOrderRead {
		t.Fatalf("unexpected role: %+v", role)
	}
	created, err := store.Create(ctx, contract.CreateStoredUser{
		Email:        "reader@srapi.local",
		Name:         "Reader",
		PasswordHash: "hash",
		Status:       contract.StatusActive,
		Roles:        []contract.Role{"payment_reader"},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if len(created.Permissions) != 1 || created.Permissions[0] != contract.PermissionPaymentOrderRead {
		t.Fatalf("expected role permission on user, got %+v", created.Permissions)
	}
}

func TestStoreListByIDsPreservesOrder(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	first, err := store.Create(ctx, newUser("first@srapi.local"))
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	second, err := store.Create(ctx, newUser("second@srapi.local"))
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	users, err := store.ListByIDs(ctx, []int{second.ID, first.ID})
	if err != nil {
		t.Fatalf("list by ids: %v", err)
	}
	if len(users) != 2 || users[0].ID != second.ID || users[1].ID != first.ID {
		t.Fatalf("unexpected order: %+v", users)
	}
}

func TestStoreFindsAuthIdentityAndRejectsOwnershipTransfer(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	first, err := store.Create(ctx, newUser("first-identity@srapi.local"))
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	second, err := store.Create(ctx, newUser("second-identity@srapi.local"))
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}
	verifiedAt := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	created, err := store.UpsertAuthIdentity(ctx, contract.CreateUserAuthIdentity{
		UserID:              first.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:subject",
		SubjectHint:         "oidc:subject",
		DisplayName:         "OIDC User",
		Email:               "OIDC@Example.COM",
		EmailVerified:       true,
		VerifiedAt:          &verifiedAt,
	})
	if err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	found, err := store.FindAuthIdentityByProviderSubject(ctx, contract.AuthIdentityProviderOIDC, "issuer-main", "sha256:subject")
	if err != nil {
		t.Fatalf("find identity: %v", err)
	}
	if found.ID != created.ID || found.UserID != first.ID || found.Email != "oidc@example.com" {
		t.Fatalf("unexpected found identity: %+v", found)
	}

	if _, err := store.UpsertAuthIdentity(ctx, contract.CreateUserAuthIdentity{
		UserID:              second.ID,
		Provider:            contract.AuthIdentityProviderOIDC,
		ProviderKey:         "issuer-main",
		ProviderSubjectHash: "sha256:subject",
		SubjectHint:         "oidc:subject",
	}); !errors.Is(err, contract.ErrAlreadyExists) {
		t.Fatalf("expected ownership transfer to be rejected, got %v", err)
	}
	found, err = store.FindAuthIdentityByProviderSubject(ctx, contract.AuthIdentityProviderOIDC, "issuer-main", "sha256:subject")
	if err != nil {
		t.Fatalf("find identity after rejected transfer: %v", err)
	}
	if found.UserID != first.ID {
		t.Fatalf("expected original owner %d, got %+v", first.ID, found)
	}
}

func TestStoreListsAndUpdatesUsers(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	created, err := store.Create(ctx, newUser("first@srapi.local"))
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	status := contract.StatusDisabled
	balance := "5.00000000"
	currency := "EUR"
	rpmLimit := 99
	rpmLimitPtr := &rpmLimit
	roles := []contract.Role{contract.RoleOperator}
	updated, err := store.Update(ctx, created.ID, contract.UpdateStoredUser{
		Status:   &status,
		Roles:    &roles,
		Balance:  &balance,
		Currency: &currency,
		RPMLimit: &rpmLimitPtr,
	})
	if err != nil {
		t.Fatalf("update user: %v", err)
	}
	if updated.Status != status || updated.Balance != balance || updated.Currency != currency || len(updated.Roles) != 1 || updated.Roles[0] != contract.RoleOperator {
		t.Fatalf("unexpected updated user: %+v", updated)
	}
	if updated.RPMLimit == nil || *updated.RPMLimit != rpmLimit {
		t.Fatalf("expected rpm limit %d, got %v", rpmLimit, updated.RPMLimit)
	}

	listed, err := store.List(ctx, contract.ListUsersFilter{Status: &status, Query: "first"})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("unexpected list result: %+v", listed)
	}

	var cleared *int
	updated, err = store.Update(ctx, created.ID, contract.UpdateStoredUser{RPMLimit: &cleared})
	if err != nil {
		t.Fatalf("clear rpm limit: %v", err)
	}
	if updated.RPMLimit != nil {
		t.Fatalf("expected cleared rpm limit, got %v", updated.RPMLimit)
	}
}

func newUser(email string) contract.CreateStoredUser {
	return contract.CreateStoredUser{
		Email:        email,
		Name:         "User",
		PasswordHash: "hash",
		Status:       contract.StatusActive,
		Roles:        []contract.Role{contract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "store.db") + "?_fk=1"
}
