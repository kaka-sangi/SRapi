package contract

import (
	"context"
	"errors"
	"time"
)

var ErrKeyNotFound = errors.New("api key not found")

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusExpired  Status = "expired"
)

type APIKey struct {
	ID               int
	UserID           int
	WorkspaceID      *int
	Name             string
	Prefix           string
	Hash             string
	Status           Status
	Scopes           []string
	AllowedModels    []string
	GroupIDs         []int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	AllowedIPs       []string
	DeniedIPs        []string
	ExpiresAt        *time.Time
	LastUsedAt       *time.Time
	CreatedAt        time.Time
}

type CreateRequest struct {
	UserID           int
	WorkspaceID      *int
	Name             string
	Scopes           []string
	AllowedModels    []string
	GroupIDs         []int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	AllowedIPs       []string
	DeniedIPs        []string
	ExpiresAt        *time.Time
}

type UpdateRequest struct {
	UserID           int
	KeyID            int
	Name             *string
	Status           *Status
	Scopes           *[]string
	AllowedModels    *[]string
	GroupIDs         *[]int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	AllowedIPs       *[]string
	DeniedIPs        *[]string
	ExpiresAt        *time.Time
}

type CreatedKey struct {
	Key          APIKey
	PlaintextKey string
}

type AuthResult struct {
	Key    APIKey
	UserID int
}

type Store interface {
	Create(ctx context.Context, input CreateStoredKey) (APIKey, error)
	Update(ctx context.Context, key APIKey) (APIKey, error)
	FindByPrefix(ctx context.Context, prefix string) (APIKey, error)
	FindByID(ctx context.Context, id int) (APIKey, error)
	List(ctx context.Context) ([]APIKey, error)
	ListByUser(ctx context.Context, userID int) ([]APIKey, error)
	TouchLastUsed(ctx context.Context, id int, usedAt time.Time) error
}

type CreateStoredKey struct {
	UserID           int
	WorkspaceID      *int
	Name             string
	Prefix           string
	Hash             string
	Status           Status
	Scopes           []string
	AllowedModels    []string
	GroupIDs         []int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	AllowedIPs       []string
	DeniedIPs        []string
	ExpiresAt        *time.Time
}
