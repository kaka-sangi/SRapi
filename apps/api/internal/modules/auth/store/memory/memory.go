package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
)

type Store struct {
	mu       sync.Mutex
	sessions map[string]contract.Session
}

func New() *Store {
	return &Store{sessions: map[string]contract.Session{}}
}

func (s *Store) Create(_ context.Context, input contract.CreateSession) (contract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := contract.Session{
		ID:        input.ID,
		UserID:    input.UserID,
		CSRFToken: input.CSRFToken,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: input.CreatedAt,
	}
	s.sessions[session.ID] = session
	return session, nil
}

func (s *Store) FindByID(_ context.Context, id string) (contract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return contract.Session{}, errors.New("session not found")
	}
	return session, nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *Store) Touch(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return errors.New("session not found")
	}
	session.LastSeenAt = &at
	s.sessions[id] = session
	return nil
}

func (s *Store) CleanupExpiredSessions(_ context.Context, now time.Time) (contract.CleanupExpiredSessionsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := contract.CleanupExpiredSessionsResult{}
	for id, session := range s.sessions {
		if session.ExpiresAt.IsZero() || session.ExpiresAt.After(now) {
			continue
		}
		result.Selected++
		delete(s.sessions, id)
		result.Expired++
	}
	return result, nil
}
