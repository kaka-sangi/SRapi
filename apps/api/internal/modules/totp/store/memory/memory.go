package memory

import (
	"context"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/totp/contract"
)

type Store struct {
	mu     sync.Mutex
	nextID int
	byUser map[int]contract.Secret
}

func New() *Store {
	return &Store{nextID: 1, byUser: map[int]contract.Secret{}}
}

func (s *Store) FindByUserID(_ context.Context, userID int) (contract.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, ok := s.byUser[userID]
	if !ok {
		return contract.Secret{}, contract.ErrSecretNotFound
	}
	return cloneSecret(secret), nil
}

func (s *Store) UpsertSetup(_ context.Context, input contract.UpsertSecretInput) (contract.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.UserID <= 0 || input.SecretCiphertext == "" || input.SecretVersion == "" {
		return contract.Secret{}, contract.ErrInvalidInput
	}
	now := normalizedNow(input.Now)
	secret, ok := s.byUser[input.UserID]
	if !ok {
		secret.ID = s.nextID
		secret.UserID = input.UserID
		secret.CreatedAt = now
		s.nextID++
	}
	secret.SecretCiphertext = input.SecretCiphertext
	secret.SecretVersion = input.SecretVersion
	secret.Enabled = input.Enabled
	secret.RecoveryCodeHashes = cloneStrings(input.RecoveryCodeHashes)
	secret.LastUsedAt = nil
	secret.UpdatedAt = now
	s.byUser[input.UserID] = secret
	return cloneSecret(secret), nil
}

func (s *Store) Enable(_ context.Context, input contract.EnableSecretInput) (contract.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, ok := s.byUser[input.UserID]
	if !ok {
		return contract.Secret{}, contract.ErrSecretNotFound
	}
	secret.Enabled = true
	secret.RecoveryCodeHashes = cloneStrings(input.RecoveryCodeHashes)
	secret.UpdatedAt = normalizedNow(input.Now)
	s.byUser[input.UserID] = secret
	return cloneSecret(secret), nil
}

func (s *Store) Disable(_ context.Context, input contract.DisableSecretInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byUser[input.UserID]; !ok {
		return contract.ErrSecretNotFound
	}
	delete(s.byUser, input.UserID)
	return nil
}

func (s *Store) MarkUsed(_ context.Context, input contract.MarkUsedInput) (contract.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, ok := s.byUser[input.UserID]
	if !ok {
		return contract.Secret{}, contract.ErrSecretNotFound
	}
	lastUsedAt := input.LastUsedAt.UTC()
	secret.RecoveryCodeHashes = cloneStrings(input.RecoveryCodeHashes)
	secret.LastUsedAt = &lastUsedAt
	secret.UpdatedAt = lastUsedAt
	s.byUser[input.UserID] = secret
	return cloneSecret(secret), nil
}

func normalizedNow(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now().UTC()
	}
	return at.UTC()
}

func cloneSecret(secret contract.Secret) contract.Secret {
	secret.RecoveryCodeHashes = cloneStrings(secret.RecoveryCodeHashes)
	if secret.LastUsedAt != nil {
		lastUsedAt := *secret.LastUsedAt
		secret.LastUsedAt = &lastUsedAt
	}
	return secret
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
