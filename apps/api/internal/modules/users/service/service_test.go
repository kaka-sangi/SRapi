package service

import (
	"context"
	"errors"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

func TestCreateHashesPasswordAndDefaultsRole(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if created.PasswordHash == "" {
		t.Fatal("expected password hash")
	}
	if len(created.Roles) != 1 || created.Roles[0] != contract.RoleUser {
		t.Fatalf("expected default user role, got %#v", created.Roles)
	}
	if created.Balance != "0.00000000" || created.Currency != "USD" {
		t.Fatalf("expected default balance, got %s %s", created.Balance, created.Currency)
	}
}

func TestAuthenticatePasswordAcceptsValidPassword(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
		Roles:    []contract.Role{contract.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	authed, err := svc.AuthenticatePassword(context.Background(), created.Email, "password123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if authed.ID != created.ID {
		t.Fatalf("expected user id %d, got %d", created.ID, authed.ID)
	}
}

func TestCustomRoleCarriesPermissions(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	role, err := svc.CreateRole(context.Background(), CreateRoleRequest{
		Name:        "payment_reader",
		Description: "Payment reader",
		Permissions: []string{"payment_order:read", "payment_order:read"},
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.Name != "payment_reader" || len(role.Permissions) != 1 || role.Permissions[0] != "payment_order:read" {
		t.Fatalf("unexpected role definition: %+v", role)
	}
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "reader@srapi.local",
		Name:     "Reader",
		Password: "password123",
		Roles:    []contract.Role{"payment_reader"},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if len(created.Permissions) != 1 || created.Permissions[0] != "payment_order:read" {
		t.Fatalf("expected role permission on user, got %+v", created.Permissions)
	}
}

func TestAuthenticatePasswordRejectsWrongPassword(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = svc.AuthenticatePassword(context.Background(), created.Email, "wrongpassword")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticatePasswordRejectsDisabledUser(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "Admin@Srapi.Local",
		Name:     "Admin",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	store.setStatus(created.ID, contract.StatusDisabled)

	_, err = svc.AuthenticatePassword(context.Background(), created.Email, "password123")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}

func TestUpdateBalanceUsesDecimalMath(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := svc.Create(context.Background(), CreateRequest{
		Email:    "billing@srapi.local",
		Name:     "Billing",
		Password: "password123",
		Balance:  "1.00000000",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	updated, err := svc.UpdateBalance(context.Background(), created.ID, BalanceUpdateRequest{
		Operation: BalanceOperationIncrement,
		Amount:    "0.33333333",
	})
	if err != nil {
		t.Fatalf("update balance: %v", err)
	}
	if updated.Balance != "1.33333333" {
		t.Fatalf("expected exact decimal balance, got %s", updated.Balance)
	}

	updated, err = svc.UpdateBalance(context.Background(), created.ID, BalanceUpdateRequest{
		Operation: BalanceOperationDecrement,
		Amount:    "0.33333333",
	})
	if err != nil {
		t.Fatalf("update balance: %v", err)
	}
	if updated.Balance != "1.00000000" {
		t.Fatalf("expected exact decimal balance, got %s", updated.Balance)
	}
}

func TestListUpdateAndBatchUsers(t *testing.T) {
	store := newMemoryStore()
	svc, err := New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	first, err := svc.Create(context.Background(), CreateRequest{
		Email:    "first@srapi.local",
		Name:     "First",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := svc.Create(context.Background(), CreateRequest{
		Email:    "second@srapi.local",
		Name:     "Second",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	rpmLimit := 120
	rpmLimitPtr := &rpmLimit
	status := contract.StatusDisabled
	roles := []contract.Role{contract.RoleOperator}
	result := svc.BatchUpdate(context.Background(), BatchUpdateRequest{
		UserIDs:  []int{first.ID, second.ID},
		Status:   &status,
		Roles:    &roles,
		RPMLimit: &rpmLimitPtr,
	})
	if len(result.Errors) != 0 || len(result.Updated) != 2 {
		t.Fatalf("unexpected batch result: %+v", result)
	}
	listed, err := svc.List(context.Background(), ListRequest{Status: &status, Query: "first"})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != first.ID || listed[0].RPMLimit == nil || *listed[0].RPMLimit != rpmLimit {
		t.Fatalf("unexpected listed users: %+v", listed)
	}
}
