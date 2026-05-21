package memory

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

type Store struct {
	mu      sync.Mutex
	nextID  int
	byID    map[int]contract.StoredUser
	byEmail map[string]int
}

func New() *Store {
	return &Store{
		nextID:  1,
		byID:    map[int]contract.StoredUser{},
		byEmail: map[string]int{},
	}
}

func (s *Store) Create(_ context.Context, input contract.CreateStoredUser) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if _, ok := s.byEmail[email]; ok {
		return contract.StoredUser{}, errors.New("user already exists")
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

func (s *Store) FindByID(_ context.Context, id int) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, errors.New("user not found")
	}
	return user, nil
}

func (s *Store) FindByEmail(_ context.Context, email string) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return contract.StoredUser{}, errors.New("user not found")
	}
	return s.byID[id], nil
}

func (s *Store) ListByIDs(_ context.Context, ids []int) ([]contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := make([]contract.StoredUser, 0, len(ids))
	for _, id := range ids {
		user, ok := s.byID[id]
		if !ok {
			return nil, errors.New("user not found")
		}
		users = append(users, user)
	}
	return users, nil
}

func (s *Store) UpdateLastLogin(_ context.Context, id int, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return errors.New("user not found")
	}
	user.LastLoginAt = &at
	s.byID[id] = user
	return nil
}
