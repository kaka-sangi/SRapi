package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.Log
}

func New() *Store {
	return &Store{nextID: 1, byID: map[int]contract.Log{}}
}

func (s *Store) Create(_ context.Context, input contract.Log) (contract.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log := cloneLog(input)
	log.ID = s.nextID
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	s.byID[log.ID] = log
	s.nextID++
	return cloneLog(log), nil
}

func (s *Store) List(_ context.Context) ([]contract.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Log, 0, len(s.byID))
	for _, log := range s.byID {
		out = append(out, cloneLog(log))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneLog(value contract.Log) contract.Log {
	value.Before = cloneMap(value.Before)
	value.After = cloneMap(value.After)
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
