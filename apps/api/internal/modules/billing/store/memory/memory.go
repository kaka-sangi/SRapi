package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.LedgerEntry
}

func New() *Store {
	return &Store{nextID: 1, byID: map[int]contract.LedgerEntry{}}
}

func (s *Store) Create(_ context.Context, input contract.LedgerEntry) (contract.LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := cloneEntry(input)
	entry.ID = s.nextID
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.byID[entry.ID] = entry
	s.nextID++
	return cloneEntry(entry), nil
}

func (s *Store) List(_ context.Context) ([]contract.LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.LedgerEntry, 0, len(s.byID))
	for _, entry := range s.byID {
		out = append(out, cloneEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneEntry(value contract.LedgerEntry) contract.LedgerEntry {
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}
