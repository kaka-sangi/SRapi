package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("user not found")
	ErrAlreadyExists = errors.New("user already exists")
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

type AuthIdentityProvider string

const (
	AuthIdentityProviderEmail    AuthIdentityProvider = "email"
	AuthIdentityProviderOIDC     AuthIdentityProvider = "oidc"
	AuthIdentityProviderGitHub   AuthIdentityProvider = "github"
	AuthIdentityProviderGoogle   AuthIdentityProvider = "google"
	AuthIdentityProviderLinuxDo  AuthIdentityProvider = "linuxdo"
	AuthIdentityProviderWeChat   AuthIdentityProvider = "wechat"
	AuthIdentityProviderDingTalk AuthIdentityProvider = "dingtalk"
)

// UserAuthIdentity describes a console sign-in identity visible to a user.
type UserAuthIdentity struct {
	ID              int
	UserID          int
	Provider        AuthIdentityProvider
	ProviderKey     string
	SubjectHint     string
	DisplayName     string
	Email           string
	EmailVerified   bool
	AvatarURL       string
	External        bool
	VerifiedAt      *time.Time
	LastUsedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CanUnbind       bool
	UnbindBlockedBy string
}

// CreateUserAuthIdentity is the persistence payload for a verified external sign-in identity.
type CreateUserAuthIdentity struct {
	UserID              int
	Provider            AuthIdentityProvider
	ProviderKey         string
	ProviderSubjectHash string
	SubjectHint         string
	DisplayName         string
	Email               string
	EmailVerified       bool
	AvatarURL           string
	VerifiedAt          *time.Time
	LastUsedAt          *time.Time
}

type User struct {
	ID              int
	Email           string
	Name            string
	Status          Status
	WorkspaceID     *int
	Roles           []Role
	Permissions     []string
	Balance         string
	Currency        string
	RPMLimit        *int
	CreatedAt       time.Time
	LastLoginAt     *time.Time
	EmailVerifiedAt *time.Time
	AvatarURL       string
	AvatarMIME      string
	AvatarByteSize  int
	AvatarSHA256    string
	AvatarUpdatedAt *time.Time
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

// UpdateStoredRole is the store-level payload for updating a role definition.
// The role name (identity) is immutable; only description and permissions change.
type UpdateStoredRole struct {
	Description *string
	Permissions *[]string
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
	Email           *string
	Name            *string
	PasswordHash    *string
	Status          *Status
	WorkspaceID     **int
	Roles           *[]Role
	Balance         *string
	Currency        *string
	RPMLimit        **int
	EmailVerifiedAt **time.Time
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
	Delete(ctx context.Context, id int) error
	UpdateLastLogin(ctx context.Context, id int, at time.Time) error
	CreateRole(ctx context.Context, input CreateStoredRole) (RoleDefinition, error)
	ListRoles(ctx context.Context) ([]RoleDefinition, error)
	UpdateRole(ctx context.Context, id int, input UpdateStoredRole) (RoleDefinition, error)
	DeleteRole(ctx context.Context, id int) error
	ListAuthIdentities(ctx context.Context, userID int) ([]UserAuthIdentity, error)
	FindAuthIdentityByProviderSubject(ctx context.Context, provider AuthIdentityProvider, providerKey string, providerSubjectHash string) (UserAuthIdentity, error)
	UpsertAuthIdentity(ctx context.Context, input CreateUserAuthIdentity) (UserAuthIdentity, error)
	DeleteAuthIdentity(ctx context.Context, userID int, identityID int) error
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
