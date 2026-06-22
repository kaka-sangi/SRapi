package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
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

// ListPage implements contract.PageReader: mirrors the SQL store with
// newest-first ordering and offset/limit slicing.
func (s *Store) ListPage(_ context.Context, filter contract.ListFilter, limit, offset int) (contract.ListPageResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]contract.Log, 0)
	for _, log := range s.byID {
		if !auditPageMatches(log, filter) {
			continue
		}
		matched = append(matched, cloneLog(log))
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID > matched[j].ID })
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return contract.ListPageResult{Items: []contract.Log{}, Total: total}, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return contract.ListPageResult{Items: matched[offset:end], Total: total}, nil
}

func auditPageMatches(log contract.Log, filter contract.ListFilter) bool {
	if action := strings.TrimSpace(filter.Action); action != "" && log.Action != action {
		return false
	}
	if resourceType := strings.TrimSpace(filter.ResourceType); resourceType != "" && log.ResourceType != resourceType {
		return false
	}
	if filter.ActorUserID != nil {
		if log.ActorUserID == nil || *log.ActorUserID != *filter.ActorUserID {
			return false
		}
	}
	if filter.Since != nil && log.CreatedAt.Before(filter.Since.UTC()) {
		return false
	}
	return true
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
