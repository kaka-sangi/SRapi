// Package memory is an in-memory ConversationStore used when the app runs
// without a database (memory-storage mode). It is per-process and not durable.
package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/copilot/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	rows   map[int]contract.Conversation
}

func New() *Store {
	return &Store{nextID: 1, rows: map[int]contract.Conversation{}}
}

func (s *Store) ListByAdmin(_ context.Context, adminUserID, limit int) ([]contract.ConversationSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []contract.ConversationSummary
	for _, row := range s.rows {
		if row.AdminUserID == adminUserID {
			out = append(out, contract.ConversationSummary{ID: row.ID, Title: row.Title, UpdatedAt: row.UpdatedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) Get(_ context.Context, adminUserID, id int) (contract.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok || row.AdminUserID != adminUserID {
		return contract.Conversation{}, contract.ErrNotFound
	}
	return row, nil
}

func (s *Store) Create(_ context.Context, adminUserID int, title string, messages json.RawMessage) (contract.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	row := contract.Conversation{
		ID:          s.nextID,
		AdminUserID: adminUserID,
		Title:       title,
		Messages:    normalize(messages),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.rows[s.nextID] = row
	s.nextID++
	return row, nil
}

func (s *Store) Update(_ context.Context, adminUserID, id int, title string, messages json.RawMessage) (contract.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok || row.AdminUserID != adminUserID {
		return contract.Conversation{}, contract.ErrNotFound
	}
	row.Title = title
	row.Messages = normalize(messages)
	row.UpdatedAt = time.Now().UTC()
	s.rows[id] = row
	return row, nil
}

func (s *Store) Rename(_ context.Context, adminUserID, id int, title string) (contract.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok || row.AdminUserID != adminUserID {
		return contract.Conversation{}, contract.ErrNotFound
	}
	row.Title = title
	row.UpdatedAt = time.Now().UTC()
	s.rows[id] = row
	return row, nil
}

func (s *Store) Delete(_ context.Context, adminUserID, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok || row.AdminUserID != adminUserID {
		return contract.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}

func normalize(messages json.RawMessage) json.RawMessage {
	if len(messages) == 0 {
		return json.RawMessage("[]")
	}
	return messages
}
