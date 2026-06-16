package contract

import (
	"context"
	"time"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusPending  Status = "pending"
	StatusArchived Status = "archived"
)

type Model struct {
	ID              int
	CanonicalName   string
	DisplayName     string
	Family          *string
	ContextWindow   *int
	MaxOutputTokens *int
	QualityTier     *string
	Status          Status
	Capabilities    []capabilitiescontract.Descriptor
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type CreateRequest struct {
	CanonicalName   string
	DisplayName     string
	Family          *string
	ContextWindow   *int
	MaxOutputTokens *int
	QualityTier     *string
	Status          *Status
	Capabilities    []capabilitiescontract.Descriptor
}

type UpdateRequest struct {
	DisplayName     *string
	Family          **string
	ContextWindow   **int
	MaxOutputTokens **int
	QualityTier     **string
	Status          *Status
	Capabilities    *[]capabilitiescontract.Descriptor
}

type ModelAlias struct {
	ID             int
	Alias          string
	ModelID        int
	StrategyHint   *string
	FallbackModels []string
	Status         Status
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ModelResolution struct {
	Model Model
	Alias *ModelAlias
}

type CreateAliasRequest struct {
	Alias          string
	StrategyHint   *string
	FallbackModels []string
	Status         *Status
}

type CreateStoredAlias struct {
	Alias          string
	ModelID        int
	StrategyHint   *string
	FallbackModels []string
	Status         Status
}

type ModelProviderMapping struct {
	ID                 int
	ModelID            int
	ProviderID         int
	UpstreamModelName  string
	Status             Status
	CapabilityOverride []capabilitiescontract.Descriptor
	PricingOverride    map[string]any
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreateMappingRequest struct {
	ProviderID         int
	UpstreamModelName  string
	Status             *Status
	CapabilityOverride []capabilitiescontract.Descriptor
	PricingOverride    map[string]any
}

type CreateStoredMapping struct {
	ModelID            int
	ProviderID         int
	UpstreamModelName  string
	Status             Status
	CapabilityOverride []capabilitiescontract.Descriptor
	PricingOverride    map[string]any
}

// UpdateAliasRequest is the PATCH body accepted by the HTTP handler.
// All fields are optional; omitting a field leaves it unchanged.
type UpdateAliasRequest struct {
	Alias          *string
	StrategyHint   **string
	FallbackModels *[]string
	Status         *Status
}

// UpdateStoredAlias is the fully-resolved struct passed down to the store.
// Every field is present (patched from the current stored value).
type UpdateStoredAlias struct {
	Alias          string
	StrategyHint   *string
	FallbackModels []string
	Status         Status
}

// UpdateMappingRequest is the PATCH body accepted by the HTTP handler.
// All fields are optional; omitting a field leaves it unchanged.
type UpdateMappingRequest struct {
	UpstreamModelName  *string
	Status             *Status
	CapabilityOverride *[]capabilitiescontract.Descriptor
	PricingOverride    *map[string]any
}

// UpdateStoredMapping is the fully-resolved struct passed down to the store.
type UpdateStoredMapping struct {
	UpstreamModelName  string
	Status             Status
	CapabilityOverride []capabilitiescontract.Descriptor
	PricingOverride    map[string]any
}

type CreateStoredModel struct {
	CanonicalName   string
	DisplayName     string
	Family          *string
	ContextWindow   *int
	MaxOutputTokens *int
	QualityTier     *string
	Status          Status
	Capabilities    []capabilitiescontract.Descriptor
}

type Store interface {
	Create(ctx context.Context, input CreateStoredModel) (Model, error)
	Update(ctx context.Context, model Model) (Model, error)
	FindByID(ctx context.Context, id int) (Model, error)
	FindByCanonicalName(ctx context.Context, canonicalName string) (Model, error)
	FindByAlias(ctx context.Context, alias string) (ModelAlias, error)
	CreateAlias(ctx context.Context, input CreateStoredAlias) (ModelAlias, error)
	ListAliasesByModel(ctx context.Context, modelID int) ([]ModelAlias, error)
	FindAliasByID(ctx context.Context, id int) (ModelAlias, error)
	DeleteAlias(ctx context.Context, id int) error
	UpdateAlias(ctx context.Context, id int, input UpdateStoredAlias) (ModelAlias, error)
	FindMapping(ctx context.Context, modelID int, providerID int, upstreamModelName string) (ModelProviderMapping, error)
	CreateMapping(ctx context.Context, input CreateStoredMapping) (ModelProviderMapping, error)
	ListMappingsByModel(ctx context.Context, modelID int) ([]ModelProviderMapping, error)
	FindMappingByID(ctx context.Context, id int) (ModelProviderMapping, error)
	DeleteMapping(ctx context.Context, id int) error
	UpdateMapping(ctx context.Context, id int, input UpdateStoredMapping) (ModelProviderMapping, error)
	List(ctx context.Context) ([]Model, error)
	// Delete removes a model and cascades its aliases and provider mappings so a
	// removed model can't still be resolved via a lingering alias/mapping.
	Delete(ctx context.Context, id int) error
}
