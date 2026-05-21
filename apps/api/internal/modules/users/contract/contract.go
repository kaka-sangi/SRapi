package contract

import (
	"context"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusPending  Status = "pending"
)

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleUser     Role = "user"
)

type User struct {
	ID          int
	Email       string
	Name        string
	Status      Status
	Roles       []Role
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

type StoredUser struct {
	User
	PasswordHash    string
	EmailVerifiedAt *time.Time
	Balance         string
	Currency        string
}

type CreateStoredUser struct {
	Email           string
	Name            string
	PasswordHash    string
	Status          Status
	Roles           []Role
	EmailVerifiedAt *time.Time
	Balance         string
	Currency        string
}

type Store interface {
	Create(ctx context.Context, input CreateStoredUser) (StoredUser, error)
	FindByID(ctx context.Context, id int) (StoredUser, error)
	FindByEmail(ctx context.Context, email string) (StoredUser, error)
	ListByIDs(ctx context.Context, ids []int) ([]StoredUser, error)
	UpdateLastLogin(ctx context.Context, id int, at time.Time) error
}
