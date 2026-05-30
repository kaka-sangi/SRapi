package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
)

// Store is an in-memory implementation of the user attribute store, used for
// tests and memory-mode bootstraps.
type Store struct {
	mu     sync.Mutex
	defs   map[int]contract.Definition
	values map[int]contract.Value
	defSeq int
	valSeq int
	clock  func() time.Time
}

func New() *Store {
	return &Store{
		defs:   make(map[int]contract.Definition),
		values: make(map[int]contract.Value),
		clock:  time.Now,
	}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, def := range s.defs {
		if def.Key == input.Key {
			return contract.Definition{}, contract.ErrDuplicateKey
		}
	}
	s.defSeq++
	now := s.now()
	def := contract.Definition{
		ID:           s.defSeq,
		Key:          input.Key,
		Name:         input.Name,
		DataType:     input.DataType,
		Options:      append([]string(nil), input.Options...),
		Required:     input.Required,
		DisplayOrder: input.DisplayOrder,
		Enabled:      input.Enabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.defs[def.ID] = def
	return def, nil
}

func (s *Store) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def, ok := s.defs[id]
	if !ok {
		return contract.Definition{}, contract.ErrNotFound
	}
	if input.Name != nil {
		def.Name = *input.Name
	}
	if input.DataType != nil {
		def.DataType = *input.DataType
	}
	if input.Options != nil {
		def.Options = append([]string(nil), *input.Options...)
	}
	if input.Required != nil {
		def.Required = *input.Required
	}
	if input.DisplayOrder != nil {
		def.DisplayOrder = *input.DisplayOrder
	}
	if input.Enabled != nil {
		def.Enabled = *input.Enabled
	}
	def.UpdatedAt = s.now()
	s.defs[id] = def
	return def, nil
}

func (s *Store) DeleteDefinition(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.defs[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.defs, id)
	for vid, value := range s.values {
		if value.DefinitionID == id {
			delete(s.values, vid)
		}
	}
	return nil
}

func (s *Store) FindDefinitionByID(ctx context.Context, id int) (contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def, ok := s.defs[id]
	if !ok {
		return contract.Definition{}, contract.ErrNotFound
	}
	return def, nil
}

func (s *Store) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Definition, 0, len(s.defs))
	for _, def := range s.defs {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayOrder != out[j].DisplayOrder {
			return out[i].DisplayOrder < out[j].DisplayOrder
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) UpsertValue(ctx context.Context, input contract.SetValue) (contract.Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for vid, value := range s.values {
		if value.UserID == input.UserID && value.DefinitionID == input.DefinitionID {
			value.Value = input.Value
			value.UpdatedAt = now
			s.values[vid] = value
			return value, nil
		}
	}
	s.valSeq++
	value := contract.Value{
		ID:           s.valSeq,
		UserID:       input.UserID,
		DefinitionID: input.DefinitionID,
		Value:        input.Value,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.values[value.ID] = value
	return value, nil
}

func (s *Store) ListValuesByUser(ctx context.Context, userID int) ([]contract.Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Value, 0)
	for _, value := range s.values {
		if value.UserID == userID {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DefinitionID < out[j].DefinitionID })
	return out, nil
}

func (s *Store) DeleteValuesByUser(ctx context.Context, userID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for vid, value := range s.values {
		if value.UserID == userID {
			delete(s.values, vid)
		}
	}
	return nil
}
