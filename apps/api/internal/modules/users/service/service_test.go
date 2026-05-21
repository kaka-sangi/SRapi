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
