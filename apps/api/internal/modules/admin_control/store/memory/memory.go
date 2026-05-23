package memory

import (
	"context"
	"sync"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

type Store struct {
	mu     sync.Mutex
	values map[string]map[string]any
}

func New() *Store {
	return &Store{values: map[string]map[string]any{}}
}

func (s *Store) Get(_ context.Context, key string) (map[string]any, bool, error) {
	if key == "" {
		return nil, false, admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return nil, false, nil
	}
	return cloneMap(value), true, nil
}

func (s *Store) Set(_ context.Context, key string, value map[string]any, _ *int) error {
	if key == "" {
		return admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = cloneMap(value)
	return nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
