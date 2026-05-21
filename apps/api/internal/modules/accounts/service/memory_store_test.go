package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

type memoryStore struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.ProviderAccount
	byName map[string]int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		nextID: 1,
		byID:   map[int]contract.ProviderAccount{},
		byName: map[string]int{},
	}
}

func (s *memoryStore) Create(_ context.Context, input contract.CreateStoredAccount) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	account := contract.ProviderAccount{
		ID:                   s.nextID,
		ProviderID:           input.ProviderID,
		Name:                 input.Name,
		RuntimeClass:         input.RuntimeClass,
		CredentialCiphertext: input.CredentialCiphertext,
		CredentialVersion:    input.CredentialVersion,
		ProxyID:              input.ProxyID,
		Status:               input.Status,
		Priority:             input.Priority,
		Weight:               input.Weight,
		UpstreamClient:       input.UpstreamClient,
		Metadata:             input.Metadata,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	s.byID[account.ID] = account
	s.byName[strings.ToLower(account.Name)] = account.ID
	s.nextID++
	return account, nil
}

func (s *memoryStore) Update(_ context.Context, account contract.ProviderAccount) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[account.ID]; !ok {
		return contract.ProviderAccount{}, errors.New("account not found")
	}
	s.byID[account.ID] = account
	s.byName[strings.ToLower(account.Name)] = account.ID
	return account, nil
}

func (s *memoryStore) FindByID(_ context.Context, id int) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.byID[id]
	if !ok {
		return contract.ProviderAccount{}, errors.New("account not found")
	}
	return account, nil
}

func (s *memoryStore) List(_ context.Context) ([]contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.ProviderAccount, 0, len(s.byID))
	for _, account := range s.byID {
		out = append(out, account)
	}
	return out, nil
}

func (s *memoryStore) ListGroupIDsByAccount(_ context.Context, accountID int) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[accountID]; !ok {
		return nil, errors.New("account not found")
	}
	return nil, nil
}
