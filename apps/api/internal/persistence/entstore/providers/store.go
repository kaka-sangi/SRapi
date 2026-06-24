package providers

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entprovider "github.com/srapi/srapi/apps/api/ent/provider"
	"github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

var ErrInvalidStore = errors.New("invalid providers ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateStoredProvider) (contract.Provider, error) {
	created, err := s.client.Provider.Create().
		SetName(input.Name).
		SetDisplayName(input.DisplayName).
		SetAdapterType(input.AdapterType).
		SetProtocol(input.Protocol).
		SetStatus(string(input.Status)).
		SetCapabilitiesJSON(cloneMap(input.Capabilities)).
		SetConfigSchemaJSON(cloneMap(input.ConfigSchema)).
		Save(ctx)
	if err != nil {
		return contract.Provider{}, err
	}
	return toProvider(created), nil
}

func (s *Store) Update(ctx context.Context, provider contract.Provider) (contract.Provider, error) {
	update := s.client.Provider.UpdateOneID(provider.ID).
		SetDisplayName(provider.DisplayName).
		SetAdapterType(provider.AdapterType).
		SetProtocol(provider.Protocol).
		SetStatus(string(provider.Status)).
		SetCapabilitiesJSON(cloneMap(provider.Capabilities)).
		SetConfigSchemaJSON(cloneMap(provider.ConfigSchema))
	if !provider.UpdatedAt.IsZero() {
		update.SetUpdatedAt(provider.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.Provider{}, err
	}
	return toProvider(updated), nil
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.Provider, error) {
	found, err := s.client.Provider.Query().
		Where(entprovider.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return contract.Provider{}, err
	}
	return toProvider(found), nil
}

func (s *Store) FindByName(ctx context.Context, name string) (contract.Provider, error) {
	found, err := s.client.Provider.Query().
		Where(entprovider.NameEqualFold(name)).
		Only(ctx)
	if err != nil {
		return contract.Provider{}, err
	}
	return toProvider(found), nil
}

func (s *Store) List(ctx context.Context) ([]contract.Provider, error) {
	rows, err := s.client.Provider.Query().
		Order(entprovider.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Provider, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProvider(row))
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, id int) error {
	err := s.client.Provider.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("provider not found")
		}
		return err
	}
	return nil
}

func toProvider(row *ent.Provider) contract.Provider {
	return contract.Provider{
		ID:           row.ID,
		Name:         row.Name,
		DisplayName:  row.DisplayName,
		AdapterType:  row.AdapterType,
		Protocol:     row.Protocol,
		Status:       contract.Status(row.Status),
		Capabilities: cloneMap(row.CapabilitiesJSON),
		ConfigSchema: cloneMap(row.ConfigSchemaJSON),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
	}
	return cloned
}

