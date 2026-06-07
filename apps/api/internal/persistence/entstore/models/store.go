package models

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entmodelalias "github.com/srapi/srapi/apps/api/ent/modelalias"
	entmodelmapping "github.com/srapi/srapi/apps/api/ent/modelprovidermapping"
	entmodel "github.com/srapi/srapi/apps/api/ent/modelregistry"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/models/contract"
)

var ErrInvalidStore = errors.New("invalid models ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateStoredModel) (contract.Model, error) {
	created, err := s.client.ModelRegistry.Create().
		SetCanonicalName(input.CanonicalName).
		SetDisplayName(input.DisplayName).
		SetNillableFamily(input.Family).
		SetNillableContextWindow(input.ContextWindow).
		SetNillableMaxOutputTokens(input.MaxOutputTokens).
		SetNillableQualityTier(input.QualityTier).
		SetStatus(string(input.Status)).
		SetCapabilitiesJSON(descriptorsToMaps(input.Capabilities)).
		Save(ctx)
	if err != nil {
		return contract.Model{}, err
	}
	return toModel(created), nil
}

func (s *Store) Update(ctx context.Context, model contract.Model) (contract.Model, error) {
	update := s.client.ModelRegistry.UpdateOneID(model.ID).
		Where(entmodel.DeletedAtIsNil()).
		SetDisplayName(model.DisplayName).
		SetStatus(string(model.Status)).
		SetCapabilitiesJSON(descriptorsToMaps(model.Capabilities))
	if model.Family == nil {
		update.SetFamily("")
	} else {
		update.SetFamily(*model.Family)
	}
	if model.ContextWindow == nil {
		update.ClearContextWindow()
	} else {
		update.SetContextWindow(*model.ContextWindow)
	}
	if model.MaxOutputTokens == nil {
		update.ClearMaxOutputTokens()
	} else {
		update.SetMaxOutputTokens(*model.MaxOutputTokens)
	}
	if model.QualityTier == nil {
		update.SetQualityTier("")
	} else {
		update.SetQualityTier(*model.QualityTier)
	}
	if !model.UpdatedAt.IsZero() {
		update.SetUpdatedAt(model.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.Model{}, err
	}
	return toModel(updated), nil
}

func (s *Store) Delete(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	// Remove routing sub-resources first so a tombstoned model can never be
	// resolved through a lingering alias/mapping, then soft-delete the model row
	// (all queries filter DeletedAtIsNil) to keep historical usage references.
	if _, err := s.client.ModelAlias.Delete().Where(entmodelalias.ModelIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if _, err := s.client.ModelProviderMapping.Delete().Where(entmodelmapping.ModelIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if _, err := s.client.ModelRegistry.Update().
		Where(entmodel.IDEQ(id), entmodel.DeletedAtIsNil()).
		SetDeletedAt(time.Now().UTC()).
		Save(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.Model, error) {
	found, err := s.client.ModelRegistry.Query().
		Where(entmodel.IDEQ(id), entmodel.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.Model{}, err
	}
	return toModel(found), nil
}

func (s *Store) FindByCanonicalName(ctx context.Context, canonicalName string) (contract.Model, error) {
	found, err := s.client.ModelRegistry.Query().
		Where(entmodel.CanonicalNameEqualFold(canonicalName), entmodel.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.Model{}, err
	}
	return toModel(found), nil
}

func (s *Store) FindByAlias(ctx context.Context, alias string) (contract.ModelAlias, error) {
	found, err := s.client.ModelAlias.Query().
		Where(entmodelalias.AliasEqualFold(alias)).
		Only(ctx)
	if err != nil {
		return contract.ModelAlias{}, err
	}
	return toAlias(found), nil
}

func (s *Store) CreateAlias(ctx context.Context, input contract.CreateStoredAlias) (contract.ModelAlias, error) {
	created, err := s.client.ModelAlias.Create().
		SetAlias(input.Alias).
		SetModelID(input.ModelID).
		SetNillableStrategyHint(input.StrategyHint).
		SetFallbackModelsJSON(cloneStrings(input.FallbackModels)).
		SetStatus(string(input.Status)).
		Save(ctx)
	if err != nil {
		return contract.ModelAlias{}, err
	}
	return toAlias(created), nil
}

func (s *Store) ListAliasesByModel(ctx context.Context, modelID int) ([]contract.ModelAlias, error) {
	rows, err := s.client.ModelAlias.Query().
		Where(entmodelalias.ModelIDEQ(modelID)).
		Order(entmodelalias.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ModelAlias, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlias(row))
	}
	return out, nil
}

func (s *Store) FindAliasByID(ctx context.Context, id int) (contract.ModelAlias, error) {
	found, err := s.client.ModelAlias.Get(ctx, id)
	if err != nil {
		return contract.ModelAlias{}, err
	}
	return toAlias(found), nil
}

func (s *Store) DeleteAlias(ctx context.Context, id int) error {
	return s.client.ModelAlias.DeleteOneID(id).Exec(ctx)
}

func (s *Store) FindMapping(ctx context.Context, modelID int, providerID int, upstreamModelName string) (contract.ModelProviderMapping, error) {
	found, err := s.client.ModelProviderMapping.Query().
		Where(
			entmodelmapping.ModelIDEQ(modelID),
			entmodelmapping.ProviderIDEQ(providerID),
			entmodelmapping.UpstreamModelNameEQ(upstreamModelName),
		).
		Only(ctx)
	if err != nil {
		return contract.ModelProviderMapping{}, err
	}
	return toMapping(found), nil
}

func (s *Store) CreateMapping(ctx context.Context, input contract.CreateStoredMapping) (contract.ModelProviderMapping, error) {
	created, err := s.client.ModelProviderMapping.Create().
		SetModelID(input.ModelID).
		SetProviderID(input.ProviderID).
		SetUpstreamModelName(input.UpstreamModelName).
		SetStatus(string(input.Status)).
		SetCapabilityOverrideJSON(descriptorsToMaps(input.CapabilityOverride)).
		SetPricingOverrideJSON(cloneMap(input.PricingOverride)).
		Save(ctx)
	if err != nil {
		return contract.ModelProviderMapping{}, err
	}
	return toMapping(created), nil
}

func (s *Store) ListMappingsByModel(ctx context.Context, modelID int) ([]contract.ModelProviderMapping, error) {
	rows, err := s.client.ModelProviderMapping.Query().
		Where(entmodelmapping.ModelIDEQ(modelID)).
		Order(entmodelmapping.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ModelProviderMapping, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMapping(row))
	}
	return out, nil
}

func (s *Store) FindMappingByID(ctx context.Context, id int) (contract.ModelProviderMapping, error) {
	found, err := s.client.ModelProviderMapping.Get(ctx, id)
	if err != nil {
		return contract.ModelProviderMapping{}, err
	}
	return toMapping(found), nil
}

func (s *Store) DeleteMapping(ctx context.Context, id int) error {
	return s.client.ModelProviderMapping.DeleteOneID(id).Exec(ctx)
}

func (s *Store) List(ctx context.Context) ([]contract.Model, error) {
	rows, err := s.client.ModelRegistry.Query().
		Where(entmodel.DeletedAtIsNil()).
		Order(entmodel.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Model, 0, len(rows))
	for _, row := range rows {
		out = append(out, toModel(row))
	}
	return out, nil
}

func toModel(row *ent.ModelRegistry) contract.Model {
	return contract.Model{
		ID:              row.ID,
		CanonicalName:   row.CanonicalName,
		DisplayName:     row.DisplayName,
		Family:          nonEmptyStringPtr(row.Family),
		ContextWindow:   cloneInt(row.ContextWindow),
		MaxOutputTokens: cloneInt(row.MaxOutputTokens),
		QualityTier:     nonEmptyStringPtr(row.QualityTier),
		Status:          contract.Status(row.Status),
		Capabilities:    mapsToDescriptors(row.CapabilitiesJSON),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		DeletedAt:       cloneTime(row.DeletedAt),
	}
}

func toAlias(row *ent.ModelAlias) contract.ModelAlias {
	return contract.ModelAlias{
		ID:             row.ID,
		Alias:          row.Alias,
		ModelID:        row.ModelID,
		StrategyHint:   nonEmptyStringPtr(row.StrategyHint),
		FallbackModels: cloneStrings(row.FallbackModelsJSON),
		Status:         contract.Status(row.Status),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func toMapping(row *ent.ModelProviderMapping) contract.ModelProviderMapping {
	return contract.ModelProviderMapping{
		ID:                 row.ID,
		ModelID:            row.ModelID,
		ProviderID:         row.ProviderID,
		UpstreamModelName:  row.UpstreamModelName,
		Status:             contract.Status(row.Status),
		CapabilityOverride: mapsToDescriptors(row.CapabilityOverrideJSON),
		PricingOverride:    cloneMap(row.PricingOverrideJSON),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func descriptorsToMaps(values []capabilitiescontract.Descriptor) []map[string]any {
	if values == nil {
		return nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func mapsToDescriptors(values []map[string]any) []capabilitiescontract.Descriptor {
	if values == nil {
		return nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	var out []capabilitiescontract.Descriptor
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func nonEmptyStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}
