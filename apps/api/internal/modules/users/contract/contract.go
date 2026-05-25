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

const (
	// PermissionPaymentOrderRead permits read-only access to admin payment orders.
	PermissionPaymentOrderRead = "payment_order:read"
)

type BalanceOperation string

const (
	BalanceOperationSet       BalanceOperation = "set"
	BalanceOperationIncrement BalanceOperation = "increment"
	BalanceOperationDecrement BalanceOperation = "decrement"
)

type BalanceUpdateRequest struct {
	Operation BalanceOperation
	Amount    string
	Currency  string
}

type User struct {
	ID          int
	Email       string
	Name        string
	Status      Status
	WorkspaceID *int
	Roles       []Role
	Permissions []string
	Balance     string
	Currency    string
	RPMLimit    *int
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

// RoleDefinition is a persisted role catalog entry used to resolve user permissions.
type RoleDefinition struct {
	ID          int
	Name        Role
	Description string
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateStoredRole is the store-level payload for creating a role definition.
type CreateStoredRole struct {
	Name        Role
	Description string
	Permissions []string
}

type StoredUser struct {
	User
	PasswordHash    string
	EmailVerifiedAt *time.Time
}

type CreateStoredUser struct {
	Email           string
	Name            string
	PasswordHash    string
	Status          Status
	WorkspaceID     *int
	Roles           []Role
	EmailVerifiedAt *time.Time
	Balance         string
	Currency        string
	RPMLimit        *int
}

type UpdateStoredUser struct {
	Email        *string
	Name         *string
	PasswordHash *string
	Status       *Status
	WorkspaceID  **int
	Roles        *[]Role
	Balance      *string
	Currency     *string
	RPMLimit     **int
}

type ListUsersFilter struct {
	Status *Status
	Query  string
}

type Store interface {
	Create(ctx context.Context, input CreateStoredUser) (StoredUser, error)
	FindByID(ctx context.Context, id int) (StoredUser, error)
	FindByEmail(ctx context.Context, email string) (StoredUser, error)
	List(ctx context.Context, filter ListUsersFilter) ([]StoredUser, error)
	ListByIDs(ctx context.Context, ids []int) ([]StoredUser, error)
	Update(ctx context.Context, id int, input UpdateStoredUser) (StoredUser, error)
	UpdateLastLogin(ctx context.Context, id int, at time.Time) error
	CreateRole(ctx context.Context, input CreateStoredRole) (RoleDefinition, error)
	ListRoles(ctx context.Context) ([]RoleDefinition, error)
}

// IsBuiltInRole reports whether a role is one of SRapi's bootstrap roles.
func IsBuiltInRole(role Role) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleOperator, RoleUser:
		return true
	default:
		return false
	}
}
