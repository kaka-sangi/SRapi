package userattributes

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entdefinition "github.com/srapi/srapi/apps/api/ent/userattributedefinition"
	entvalue "github.com/srapi/srapi/apps/api/ent/userattributevalue"
	"github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
)

var ErrInvalidStore = errors.New("invalid user attribute ent store")

// Store is the Ent-backed implementation of the user attribute store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	now := time.Now().UTC()
	row, err := s.client.UserAttributeDefinition.Create().
		SetKey(input.Key).
		SetName(input.Name).
		SetDataType(string(input.DataType)).
		SetOptionsJSON(cloneStrings(input.Options)).
		SetRequired(input.Required).
		SetDisplayOrder(input.DisplayOrder).
		SetEnabled(input.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.Definition{}, contract.ErrDuplicateKey
		}
		return contract.Definition{}, err
	}
	return toDefinition(row), nil
}

func (s *Store) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidStore
	}
	update := s.client.UserAttributeDefinition.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if input.Name != nil {
		update.SetName(*input.Name)
	}
	if input.DataType != nil {
		update.SetDataType(string(*input.DataType))
	}
	if input.Options != nil {
		update.SetOptionsJSON(cloneStrings(*input.Options))
	}
	if input.Required != nil {
		update.SetRequired(*input.Required)
	}
	if input.DisplayOrder != nil {
		update.SetDisplayOrder(*input.DisplayOrder)
	}
	if input.Enabled != nil {
		update.SetEnabled(*input.Enabled)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Definition{}, contract.ErrNotFound
		}
		return contract.Definition{}, err
	}
	return toDefinition(row), nil
}

func (s *Store) DeleteDefinition(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if _, err := s.client.UserAttributeValue.Delete().
		Where(entvalue.DefinitionIDEQ(id)).
		Exec(ctx); err != nil {
		return err
	}
	err := s.client.UserAttributeDefinition.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) FindDefinitionByID(ctx context.Context, id int) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidStore
	}
	row, err := s.client.UserAttributeDefinition.Query().
		Where(entdefinition.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Definition{}, contract.ErrNotFound
		}
		return contract.Definition{}, err
	}
	return toDefinition(row), nil
}

func (s *Store) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	rows, err := s.client.UserAttributeDefinition.Query().
		Order(ent.Asc(entdefinition.FieldDisplayOrder), ent.Asc(entdefinition.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Definition, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDefinition(row))
	}
	return out, nil
}

func (s *Store) UpsertValue(ctx context.Context, input contract.SetValue) (contract.Value, error) {
	now := time.Now().UTC()
	affected, err := s.client.UserAttributeValue.Update().
		Where(entvalue.UserIDEQ(input.UserID), entvalue.DefinitionIDEQ(input.DefinitionID)).
		SetValue(input.Value).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Value{}, err
	}
	if affected == 0 {
		row, err := s.client.UserAttributeValue.Create().
			SetUserID(input.UserID).
			SetDefinitionID(input.DefinitionID).
			SetValue(input.Value).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				return s.UpsertValue(ctx, input)
			}
			return contract.Value{}, err
		}
		return toValue(row), nil
	}
	row, err := s.client.UserAttributeValue.Query().
		Where(entvalue.UserIDEQ(input.UserID), entvalue.DefinitionIDEQ(input.DefinitionID)).
		Only(ctx)
	if err != nil {
		return contract.Value{}, err
	}
	return toValue(row), nil
}

func (s *Store) ListValuesByUser(ctx context.Context, userID int) ([]contract.Value, error) {
	if userID <= 0 {
		return nil, ErrInvalidStore
	}
	rows, err := s.client.UserAttributeValue.Query().
		Where(entvalue.UserIDEQ(userID)).
		Order(ent.Asc(entvalue.FieldDefinitionID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Value, 0, len(rows))
	for _, row := range rows {
		out = append(out, toValue(row))
	}
	return out, nil
}

func (s *Store) DeleteValuesByUser(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalidStore
	}
	_, err := s.client.UserAttributeValue.Delete().
		Where(entvalue.UserIDEQ(userID)).
		Exec(ctx)
	return err
}

func toDefinition(row *ent.UserAttributeDefinition) contract.Definition {
	return contract.Definition{
		ID:           row.ID,
		Key:          row.Key,
		Name:         row.Name,
		DataType:     contract.DataType(row.DataType),
		Options:      cloneStrings(row.OptionsJSON),
		Required:     row.Required,
		DisplayOrder: row.DisplayOrder,
		Enabled:      row.Enabled,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func toValue(row *ent.UserAttributeValue) contract.Value {
	return contract.Value{
		ID:           row.ID,
		UserID:       row.UserID,
		DefinitionID: row.DefinitionID,
		Value:        row.Value,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
