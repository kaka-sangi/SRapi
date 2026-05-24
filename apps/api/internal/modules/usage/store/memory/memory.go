package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.UsageLog
}

func New() *Store {
	return &Store{
		nextID: 1,
		byID:   map[int]contract.UsageLog{},
	}
}

func (s *Store) Create(_ context.Context, input contract.UsageLog) (contract.UsageLog, error) {
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

func (s *Store) List(_ context.Context) ([]contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UsageLog, 0, len(s.byID))
	for _, log := range s.byID {
		out = append(out, cloneLog(log))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListByUser(_ context.Context, userID int) ([]contract.UsageLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.UsageLog, 0)
	for _, log := range s.byID {
		if log.UserID == userID {
			out = append(out, cloneLog(log))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneLog(value contract.UsageLog) contract.UsageLog {
	value.CompatibilityWarnings = cloneStrings(value.CompatibilityWarnings)
	if value.ChargedAt != nil {
		cloned := *value.ChargedAt
		value.ChargedAt = &cloned
	}
	return value
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
