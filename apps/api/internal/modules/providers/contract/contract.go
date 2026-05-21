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
	StatusArchived Status = "archived"
)

type Provider struct {
	ID           int
	Name         string
	DisplayName  string
	AdapterType  string
	Protocol     string
	Status       Status
	Capabilities map[string]any
	ConfigSchema map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *int64
}

type CreateRequest struct {
	Name         string
	DisplayName  string
	AdapterType  string
	Protocol     string
	Status       *Status
	Capabilities map[string]any
	ConfigSchema map[string]any
}

type UpdateRequest struct {
	DisplayName  *string
	AdapterType  *string
	Protocol     *string
	Status       *Status
	Capabilities *map[string]any
	ConfigSchema *map[string]any
}

type CreateStoredProvider struct {
	Name         string
	DisplayName  string
	AdapterType  string
	Protocol     string
	Status       Status
	Capabilities map[string]any
	ConfigSchema map[string]any
}

type Store interface {
	Create(ctx context.Context, input CreateStoredProvider) (Provider, error)
	Update(ctx context.Context, provider Provider) (Provider, error)
	FindByID(ctx context.Context, id int) (Provider, error)
	FindByName(ctx context.Context, name string) (Provider, error)
	List(ctx context.Context) ([]Provider, error)
}
