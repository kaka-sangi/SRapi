package memory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byID   map[int]contract.ProviderAccount
	byName map[string]int
}

func New() *Store {
	return &Store{
		nextID: 1,
		byID:   map[int]contract.ProviderAccount{},
		byName: map[string]int{},
	}
}

func (s *Store) Create(_ context.Context, input contract.CreateStoredAccount) (contract.ProviderAccount, error) {
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

func (s *Store) Update(_ context.Context, account contract.ProviderAccount) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[account.ID]; !ok {
		return contract.ProviderAccount{}, errors.New("account not found")
	}
	stored := account
	stored.Metadata = cloneMap(account.Metadata)
	s.byID[stored.ID] = stored
	s.byName[strings.ToLower(stored.Name)] = stored.ID
	return cloneAccount(stored), nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.byID[id]
	if !ok {
		return contract.ProviderAccount{}, errors.New("account not found")
	}
	return cloneAccount(account), nil
}

func (s *Store) List(_ context.Context) ([]contract.ProviderAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.ProviderAccount, 0, len(s.byID))
	for _, account := range s.byID {
		out = append(out, cloneAccount(account))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListGroupIDsByAccount(_ context.Context, accountID int) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[accountID]; !ok {
		return nil, errors.New("account not found")
	}
	return nil, nil
}

func cloneAccount(value contract.ProviderAccount) contract.ProviderAccount {
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
	}
	return cloned
}
