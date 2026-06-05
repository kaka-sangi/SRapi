package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

const (
	defaultPrefixBytes = 6
	defaultSecretBytes = 32
	keyPrefix          = "sk"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type Service struct {
	store  contract.Store
	pepper []byte
	clock  Clock
}

func New(store contract.Store, pepper string, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if len(pepper) < 32 {
		return nil, ErrPepperUnavailable
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, pepper: []byte(pepper), clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, req contract.CreateRequest) (contract.CreatedKey, error) {
	if req.UserID <= 0 || strings.TrimSpace(req.Name) == "" {
		return contract.CreatedKey{}, ErrInvalidInput
	}
	if err := validateIPEntries(req.AllowedIPs); err != nil {
		return contract.CreatedKey{}, err
	}
	if err := validateIPEntries(req.DeniedIPs); err != nil {
		return contract.CreatedKey{}, err
	}
	plaintext, prefix, err := GeneratePlaintextKey()
	if err != nil {
		return contract.CreatedKey{}, err
	}
	hash := s.HashPlaintext(plaintext)

	stored, err := s.store.Create(ctx, contract.CreateStoredKey{
		UserID:           req.UserID,
		WorkspaceID:      cloneIntPointer(req.WorkspaceID),
		Name:             strings.TrimSpace(req.Name),
		Prefix:           prefix,
		Hash:             hash,
		Status:           contract.StatusActive,
		Scopes:           withDefaultScopes(req.Scopes),
		AllowedModels:    cloneStrings(req.AllowedModels),
		GroupIDs:         cloneInts(req.GroupIDs),
		RPMLimit:         cloneIntPointer(req.RPMLimit),
		TPMLimit:         cloneIntPointer(req.TPMLimit),
		ConcurrencyLimit: cloneIntPointer(req.ConcurrencyLimit),
		RequestLimit5h:   cloneIntPointer(req.RequestLimit5h),
		RequestLimit1d:   cloneIntPointer(req.RequestLimit1d),
		RequestLimit7d:   cloneIntPointer(req.RequestLimit7d),
		AllowedIPs:       cloneStrings(req.AllowedIPs),
		DeniedIPs:        cloneStrings(req.DeniedIPs),
		ExpiresAt:        cloneTimePointer(req.ExpiresAt),
	})
	if err != nil {
		return contract.CreatedKey{}, err
	}

	return contract.CreatedKey{Key: withoutHash(stored), PlaintextKey: plaintext}, nil
}

func (s *Service) ListByUser(ctx context.Context, userID int) ([]contract.APIKey, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	keys, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.APIKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, withoutHash(key))
	}
	return out, nil
}

func (s *Service) List(ctx context.Context) ([]contract.APIKey, error) {
	keys, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.APIKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, withoutHash(key))
	}
	return out, nil
}

// GetByID returns a single key by its ID (without the secret hash), regardless
// of owner — used by admin tooling that operates across users.
func (s *Service) GetByID(ctx context.Context, id int) (contract.APIKey, error) {
	if id <= 0 {
		return contract.APIKey{}, ErrInvalidInput
	}
	key, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.APIKey{}, err
	}
	return withoutHash(key), nil
}

func (s *Service) Update(ctx context.Context, req contract.UpdateRequest) (contract.APIKey, error) {
	if req.UserID <= 0 || req.KeyID <= 0 {
		return contract.APIKey{}, ErrInvalidInput
	}
	keys, err := s.store.ListByUser(ctx, req.UserID)
	if err != nil {
		return contract.APIKey{}, err
	}
	var key contract.APIKey
	found := false
	for _, candidate := range keys {
		if candidate.ID == req.KeyID {
			key = candidate
			found = true
			break
		}
	}
	if !found {
		return contract.APIKey{}, ErrKeyNotFound
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.APIKey{}, ErrInvalidInput
		}
		key.Name = name
	}
	if req.Status != nil {
		if !validStatus(*req.Status) {
			return contract.APIKey{}, ErrInvalidInput
		}
		key.Status = *req.Status
	}
	if req.Scopes != nil {
		key.Scopes = withDefaultScopes(*req.Scopes)
	}
	if req.AllowedModels != nil {
		key.AllowedModels = cloneStrings(*req.AllowedModels)
	}
	if req.GroupIDs != nil {
		key.GroupIDs = cloneInts(*req.GroupIDs)
	}
	if req.RPMLimit != nil {
		key.RPMLimit = cloneIntPointer(req.RPMLimit)
	}
	if req.TPMLimit != nil {
		key.TPMLimit = cloneIntPointer(req.TPMLimit)
	}
	if req.ConcurrencyLimit != nil {
		key.ConcurrencyLimit = cloneIntPointer(req.ConcurrencyLimit)
	}
	if req.RequestLimit5h != nil {
		key.RequestLimit5h = cloneIntPointer(req.RequestLimit5h)
	}
	if req.RequestLimit1d != nil {
		key.RequestLimit1d = cloneIntPointer(req.RequestLimit1d)
	}
	if req.RequestLimit7d != nil {
		key.RequestLimit7d = cloneIntPointer(req.RequestLimit7d)
	}
	if req.AllowedIPs != nil {
		if err := validateIPEntries(*req.AllowedIPs); err != nil {
			return contract.APIKey{}, err
		}
		key.AllowedIPs = cloneStrings(*req.AllowedIPs)
	}
	if req.DeniedIPs != nil {
		if err := validateIPEntries(*req.DeniedIPs); err != nil {
			return contract.APIKey{}, err
		}
		key.DeniedIPs = cloneStrings(*req.DeniedIPs)
	}
	updated, err := s.store.Update(ctx, key)
	if err != nil {
		if errors.Is(err, contract.ErrKeyNotFound) {
			return contract.APIKey{}, ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}
	return withoutHash(updated), nil
}

func (s *Service) Authenticate(ctx context.Context, plaintext string) (contract.AuthResult, error) {
	prefix, ok := PrefixFromPlaintext(plaintext)
	if !ok {
		return contract.AuthResult{}, ErrInvalidKey
	}
	key, err := s.store.FindByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, contract.ErrKeyNotFound) || errors.Is(err, ErrKeyNotFound) {
			return contract.AuthResult{}, ErrInvalidKey
		}
		return contract.AuthResult{}, err
	}
	if key.Status == contract.StatusDisabled {
		return contract.AuthResult{}, ErrKeyDisabled
	}
	if key.Status == contract.StatusExpired || isExpired(key.ExpiresAt, s.clock.Now()) {
		return contract.AuthResult{}, ErrKeyExpired
	}
	if !hmac.Equal([]byte(key.Hash), []byte(s.HashPlaintext(plaintext))) {
		return contract.AuthResult{}, ErrInvalidKey
	}
	now := s.clock.Now()
	if err := s.store.TouchLastUsed(ctx, key.ID, now); err != nil {
		return contract.AuthResult{}, err
	}
	key.LastUsedAt = &now
	return contract.AuthResult{Key: withoutHash(key), UserID: key.UserID}, nil
}

func (s *Service) HashPlaintext(plaintext string) string {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write([]byte(plaintext))
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func GeneratePlaintextKey() (plaintext string, prefix string, err error) {
	prefixBytes, err := randomBytes(defaultPrefixBytes)
	if err != nil {
		return "", "", err
	}
	secretBytes, err := randomBytes(defaultSecretBytes)
	if err != nil {
		return "", "", err
	}
	prefix = keyPrefix + "_" + hex.EncodeToString(prefixBytes)
	secret := hex.EncodeToString(secretBytes)
	return prefix + "_" + secret, prefix, nil
}

func PrefixFromPlaintext(plaintext string) (string, bool) {
	rest, ok := strings.CutPrefix(plaintext, keyPrefix+"_")
	if !ok || rest == "" {
		return "", false
	}
	prefixPart, secretPart, ok := strings.Cut(rest, "_")
	if !ok || prefixPart == "" || secretPart == "" {
		return "", false
	}
	if _, err := hex.DecodeString(prefixPart); err != nil {
		return "", false
	}
	if _, err := hex.DecodeString(secretPart); err != nil {
		return "", false
	}
	return keyPrefix + "_" + prefixPart, true
}

func randomBytes(size int) ([]byte, error) {
	bytes := make([]byte, size)
	_, err := rand.Read(bytes)
	return bytes, err
}

func isExpired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !expiresAt.After(now)
}

func withoutHash(key contract.APIKey) contract.APIKey {
	key.Hash = ""
	return key
}

func withDefaultScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{"gateway:invoke"}
	}
	return cloneStrings(scopes)
}

func validStatus(status contract.Status) bool {
	switch status {
	case contract.StatusActive, contract.StatusDisabled, contract.StatusExpired:
		return true
	default:
		return false
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

// validateIPEntries rejects an IP allow/deny list containing any entry that is
// not a valid IP address or CIDR block. Empty/blank entries are rejected too.
func validateIPEntries(entries []string) error {
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return ErrInvalidInput
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return ErrInvalidInput
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			return ErrInvalidInput
		}
	}
	return nil
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	cloned := make([]int, len(values))
	copy(cloned, values)
	return cloned
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
