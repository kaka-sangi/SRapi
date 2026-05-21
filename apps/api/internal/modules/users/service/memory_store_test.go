package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

type memoryStore struct {
	mu      sync.Mutex
	nextID  int
	byID    map[int]contract.StoredUser
	byEmail map[string]int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		nextID:  1,
		byID:    map[int]contract.StoredUser{},
		byEmail: map[string]int{},
	}
}

func (s *memoryStore) Create(_ context.Context, input contract.CreateStoredUser) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if _, ok := s.byEmail[email]; ok {
		return contract.StoredUser{}, ErrUserAlreadyExists
	}
	now := time.Now().UTC()
	user := contract.StoredUser{
		User: contract.User{
			ID:        s.nextID,
			Email:     email,
			Name:      input.Name,
			Status:    input.Status,
			Roles:     append([]contract.Role(nil), input.Roles...),
			CreatedAt: now,
		},
		PasswordHash:    input.PasswordHash,
		EmailVerifiedAt: input.EmailVerifiedAt,
		Balance:         input.Balance,
		Currency:        input.Currency,
	}
	s.byID[user.ID] = user
	s.byEmail[email] = user.ID
	s.nextID++
	return user, nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	return user, nil
}

func (s *memoryStore) FindByEmail(_ context.Context, email string) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	return s.byID[id], nil
}

func (s *memoryStore) ListByIDs(_ context.Context, ids []int) ([]contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := make([]contract.StoredUser, 0, len(ids))
	for _, id := range ids {
		user, ok := s.byID[id]
		if !ok {
			return nil, ErrUserNotFound
		}
		users = append(users, user)
	}
	return users, nil
}

func (s *memoryStore) UpdateLastLogin(_ context.Context, id int, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return ErrUserNotFound
	}
	user.LastLoginAt = &at
	s.byID[id] = user
	return nil
}

func (s *memoryStore) setStatus(id int, status contract.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.byID[id]
	user.Status = status
	s.byID[id] = user
}
