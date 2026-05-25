package memory

import (
	"context"
	"errors"
	"sort"
	"strconv"
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
			ID:          s.nextID,
			Email:       email,
			Name:        input.Name,
			Status:      input.Status,
			WorkspaceID: cloneInt(input.WorkspaceID),
			Roles:       append([]contract.Role(nil), input.Roles...),
			Balance:     input.Balance,
			Currency:    input.Currency,
			RPMLimit:    cloneInt(input.RPMLimit),
			CreatedAt:   now,
		},
		PasswordHash:    input.PasswordHash,
		EmailVerifiedAt: input.EmailVerifiedAt,
	}
	s.byID[user.ID] = user
	s.byEmail[email] = user.ID
	s.nextID++
	return cloneUser(user), nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, errors.New("user not found")
	}
	return cloneUser(user), nil
}

func (s *Store) FindByEmail(_ context.Context, email string) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return contract.StoredUser{}, errors.New("user not found")
	}
	return cloneUser(s.byID[id]), nil
}

func (s *Store) List(_ context.Context, filter contract.ListUsersFilter) ([]contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	out := make([]contract.StoredUser, 0, len(s.byID))
	for _, user := range s.byID {
		if filter.Status != nil && user.Status != *filter.Status {
			continue
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(user.Email), query) &&
			!strings.Contains(strings.ToLower(user.Name), query) &&
			!strings.Contains(strconv.Itoa(user.ID), query) {
			continue
		}
		out = append(out, cloneUser(user))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
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
		users = append(users, cloneUser(user))
	}
	return users, nil
}

func (s *Store) Update(_ context.Context, id int, input contract.UpdateStoredUser) (contract.StoredUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return contract.StoredUser{}, errors.New("user not found")
	}
	if input.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*input.Email))
		if existingID, exists := s.byEmail[email]; exists && existingID != id {
			return contract.StoredUser{}, errors.New("user already exists")
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
	if input.WorkspaceID != nil {
		user.WorkspaceID = cloneInt(*input.WorkspaceID)
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

func cloneUser(user contract.StoredUser) contract.StoredUser {
	user.Roles = append([]contract.Role(nil), user.Roles...)
	user.WorkspaceID = cloneInt(user.WorkspaceID)
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

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
