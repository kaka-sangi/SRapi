package service

import (
	"context"
	"sort"
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
			Balance:   input.Balance,
			Currency:  input.Currency,
			RPMLimit:  cloneInt(input.RPMLimit),
			CreatedAt: now,
		},
		PasswordHash:    input.PasswordHash,
		EmailVerifiedAt: input.EmailVerifiedAt,
	}
	s.byID[user.ID] = user
	s.byEmail[email] = user.ID
	s.nextID++
	return cloneUser(user), nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	return cloneUser(user), nil
}

func (s *memoryStore) FindByEmail(_ context.Context, email string) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	return cloneUser(s.byID[id]), nil
}

func (s *memoryStore) List(_ context.Context, filter contract.ListUsersFilter) ([]contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	users := make([]contract.StoredUser, 0, len(s.byID))
	for _, user := range s.byID {
		if filter.Status != nil && user.Status != *filter.Status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(user.Email), query) && !strings.Contains(strings.ToLower(user.Name), query) {
			continue
		}
		users = append(users, cloneUser(user))
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	return users, nil
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
		users = append(users, cloneUser(user))
	}
	return users, nil
}

func (s *memoryStore) Update(_ context.Context, id int, input contract.UpdateStoredUser) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, ErrUserNotFound
	}
	if input.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*input.Email))
		if existingID, exists := s.byEmail[email]; exists && existingID != id {
			return contract.StoredUser{}, ErrUserAlreadyExists
		}
		delete(s.byEmail, strings.ToLower(user.Email))
		user.Email = email
		s.byEmail[email] = id
	}
	if input.Name != nil {
		user.Name = strings.TrimSpace(*input.Name)
	}
	if input.PasswordHash != nil {
		user.PasswordHash = *input.PasswordHash
	}
	if input.Status != nil {
		user.Status = *input.Status
	}
	if input.Roles != nil {
		user.Roles = append([]contract.Role(nil), (*input.Roles)...)
	}
	if input.Balance != nil {
		user.Balance = *input.Balance
	}
	if input.Currency != nil {
		user.Currency = *input.Currency
	}
	if input.RPMLimit != nil {
		user.RPMLimit = cloneInt(*input.RPMLimit)
	}
	s.byID[id] = user
	return cloneUser(user), nil
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

func cloneUser(user contract.StoredUser) contract.StoredUser {
	user.Roles = append([]contract.Role(nil), user.Roles...)
	user.RPMLimit = cloneInt(user.RPMLimit)
	if user.LastLoginAt != nil {
		lastLoginAt := *user.LastLoginAt
		user.LastLoginAt = &lastLoginAt
	}
	if user.EmailVerifiedAt != nil {
		emailVerifiedAt := *user.EmailVerifiedAt
		user.EmailVerifiedAt = &emailVerifiedAt
	}
	return user
}
