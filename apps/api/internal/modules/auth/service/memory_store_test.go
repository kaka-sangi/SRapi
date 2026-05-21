package service

import (
	"context"
	"sync"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
)

type memoryStore struct {
	mu       sync.Mutex
	sessions map[string]authcontract.Session
}

func newMemoryStore() *memoryStore {
	return &memoryStore{sessions: map[string]authcontract.Session{}}
}

func (s *memoryStore) Create(_ context.Context, input authcontract.CreateSession) (authcontract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := authcontract.Session{
		ID:        input.ID,
		UserID:    input.UserID,
		CSRFToken: input.CSRFToken,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: input.CreatedAt,
	}
	s.sessions[session.ID] = session
	return session, nil
}

func (s *memoryStore) FindByID(_ context.Context, id string) (authcontract.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return authcontract.Session{}, ErrSessionNotFound
	}
	return session, nil
}

func (s *memoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *memoryStore) Touch(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	session.LastSeenAt = &at
	s.sessions[id] = session
	return nil
}
