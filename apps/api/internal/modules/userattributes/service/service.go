package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
)

// ErrInvalidInput is returned for malformed definitions or values.
var ErrInvalidInput = errors.New("invalid user attribute input")

type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

func (s *Service) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	return s.store.ListDefinitions(ctx)
}

func (s *Service) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	input.Key = normalizeKey(input.Key)
	input.Name = strings.TrimSpace(input.Name)
	input.DataType = normalizeDataType(input.DataType)
	if input.Key == "" || input.Name == "" || input.DataType == "" {
		return contract.Definition{}, ErrInvalidInput
	}
	if input.DataType == contract.DataTypeSelect && len(cleanOptions(input.Options)) == 0 {
		return contract.Definition{}, ErrInvalidInput
	}
	input.Options = cleanOptions(input.Options)
	return s.store.CreateDefinition(ctx, input)
}

func (s *Service) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.DataType != nil {
		dt := normalizeDataType(*input.DataType)
		if dt == "" {
			return contract.Definition{}, ErrInvalidInput
		}
		input.DataType = &dt
	}
	if input.Options != nil {
		opts := cleanOptions(*input.Options)
		input.Options = &opts
	}
	return s.store.UpdateDefinition(ctx, id, input)
}

func (s *Service) DeleteDefinition(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteDefinition(ctx, id)
}

func (s *Service) ListUserValues(ctx context.Context, userID int) ([]contract.Value, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListValuesByUser(ctx, userID)
}

// SetUserValue validates the value against its definition's type and stores it.
func (s *Service) SetUserValue(ctx context.Context, input contract.SetValue) (contract.Value, error) {
	if input.UserID <= 0 || input.DefinitionID <= 0 {
		return contract.Value{}, ErrInvalidInput
	}
	def, err := s.store.FindDefinitionByID(ctx, input.DefinitionID)
	if err != nil {
		return contract.Value{}, err
	}
	if !def.Enabled {
		return contract.Value{}, ErrInvalidInput
	}
	value := strings.TrimSpace(input.Value)
	if def.Required && value == "" {
		return contract.Value{}, ErrInvalidInput
	}
	if value != "" {
		if err := validateValueForType(def, value); err != nil {
			return contract.Value{}, err
		}
	}
	input.Value = value
	return s.store.UpsertValue(ctx, input)
}

func (s *Service) DeleteUserValues(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteValuesByUser(ctx, userID)
}

func validateValueForType(def contract.Definition, value string) error {
	switch def.DataType {
	case contract.DataTypeNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return ErrInvalidInput
		}
	case contract.DataTypeBoolean:
		if _, err := strconv.ParseBool(value); err != nil {
			return ErrInvalidInput
		}
	case contract.DataTypeSelect:
		for _, option := range def.Options {
			if option == value {
				return nil
			}
		}
		return ErrInvalidInput
	}
	return nil
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func normalizeDataType(dt contract.DataType) contract.DataType {
	switch contract.DataType(strings.ToLower(strings.TrimSpace(string(dt)))) {
	case contract.DataTypeString:
		return contract.DataTypeString
	case contract.DataTypeNumber:
		return contract.DataTypeNumber
	case contract.DataTypeBoolean:
		return contract.DataTypeBoolean
	case contract.DataTypeSelect:
		return contract.DataTypeSelect
	default:
		return ""
	}
}

func cleanOptions(options []string) []string {
	out := make([]string, 0, len(options))
	seen := map[string]struct{}{}
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}
		if _, ok := seen[option]; ok {
			continue
		}
		seen[option] = struct{}{}
		out = append(out, option)
	}
	return out
}
