package users

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
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
