package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
)

// Store is an in-memory implementation of the error-passthrough rule store.
type Store struct {
	mu    sync.Mutex
	rules map[int]contract.Rule
	seq   int
	clock func() time.Time
}

func New() *Store {
	return &Store{rules: make(map[int]contract.Rule), clock: time.Now}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) CreateRule(ctx context.Context, input contract.CreateRule) (contract.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	now := s.now()
	rule := contract.Rule{
		ID:          s.seq,
		Name:        input.Name,
		Enabled:     input.Enabled,
		Priority:    input.Priority,
		Action:      input.Action,
		StatusCodes: append([]int(nil), input.StatusCodes...),
		Classes:     append([]string(nil), input.Classes...),
		Keywords:    append([]string(nil), input.Keywords...),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.rules[rule.ID] = rule
	return rule, nil
}

func (s *Store) UpdateRule(ctx context.Context, id int, input contract.UpdateRule) (contract.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := s.rules[id]
	if !ok {
		return contract.Rule{}, contract.ErrNotFound
	}
	if input.Name != nil {
		rule.Name = *input.Name
	}
	if input.Enabled != nil {
		rule.Enabled = *input.Enabled
	}
	if input.Priority != nil {
		rule.Priority = *input.Priority
	}
	if input.Action != nil {
		rule.Action = *input.Action
	}
	if input.StatusCodes != nil {
		rule.StatusCodes = append([]int(nil), *input.StatusCodes...)
	}
	if input.Classes != nil {
		rule.Classes = append([]string(nil), *input.Classes...)
	}
	if input.Keywords != nil {
		rule.Keywords = append([]string(nil), *input.Keywords...)
	}
	rule.UpdatedAt = s.now()
	s.rules[id] = rule
	return rule, nil
}

func (s *Store) DeleteRule(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rules[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.rules, id)
	return nil
}

func (s *Store) ListRules(ctx context.Context) ([]contract.Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Rule, 0, len(s.rules))
	for _, rule := range s.rules {
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
