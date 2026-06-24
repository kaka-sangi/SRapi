package contract

import (
	"context"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
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
	Delete(ctx context.Context, id int) error
}

// PresetBaseURL resolves the canonical upstream base URL declared by the
// built-in provider preset identified by presetKey, with any trailing slash
// trimmed. It returns "" when no preset matches the key (the same fallback
// callers get when a preset carries no default base URL). This is the stable
// boundary other modules use to consult preset defaults without depending on
// the providers/preset package directly.
func PresetBaseURL(presetKey string) string {
	matched, ok := preset.Default().Lookup(presetKey)
	if !ok {
		return ""
	}
	return strings.TrimRight(matched.DefaultBaseURL, "/")
}
