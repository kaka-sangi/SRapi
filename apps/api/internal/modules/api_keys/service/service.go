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
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
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
	store      contract.Store
	pepper     []byte
	clock      Clock
	authCache  *AuthCache  // optional; nil disables the cache (wiring + tests)
	rpmCounter *RPMCounter // optional; nil disables per-key RPM counting
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

// SetAuthCache attaches an in-memory auth cache. Calling with nil clears it.
// Wiring is split from New() so the runtime can layer the cache on top of an
// already-constructed service without churning the New() signature.
func (s *Service) SetAuthCache(cache *AuthCache) {
	if s == nil {
		return
	}
	s.authCache = cache
}

// SetRPMCounter attaches a per-key RPM counter for the wired gateway hot path.
func (s *Service) SetRPMCounter(counter *RPMCounter) {
	if s == nil {
		return
	}
	s.rpmCounter = counter
}

// AuthCache exposes the wired cache so the runtime can bust it from non-Service
// invalidation paths (e.g. an admin tool that bypasses Service.Update). Nil
// when no cache is wired.
func (s *Service) AuthCache() *AuthCache {
	if s == nil {
		return nil
	}
	return s.authCache
}

// RPMCounter exposes the wired counter (nil when not wired). Tests + admin
// observability use this to read live counter state.
func (s *Service) RPMCounter() *RPMCounter {
	if s == nil {
		return nil
	}
	return s.rpmCounter
}

// RPMStats returns a point-in-time projection of every key's in-flight
// per-key request counter. Returns nil when no counter is wired. The
// projection trades type-safety (contract.APIKeyRPMStats) for accessibility
// — admin handlers can consume it without importing the worker types.
func (s *Service) RPMStats() []contract.APIKeyRPMStats {
	if s == nil || s.rpmCounter == nil {
		return nil
	}
	snaps := s.rpmCounter.Snapshot()
	if len(snaps) == 0 {
		return nil
	}
	out := make([]contract.APIKeyRPMStats, 0, len(snaps))
	for _, snap := range snaps {
		out = append(out, contract.APIKeyRPMStats{KeyID: snap.KeyID, Requests: snap.Requests})
	}
	return out
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
	costQuota, ok := optionalMoney(req.CostQuota)
	if !ok {
		return contract.CreatedKey{}, ErrInvalidInput
	}
	costLimit5h, ok := optionalMoney(req.CostLimit5h)
	if !ok {
		return contract.CreatedKey{}, ErrInvalidInput
	}
	costLimit1d, ok := optionalMoney(req.CostLimit1d)
	if !ok {
		return contract.CreatedKey{}, ErrInvalidInput
	}
	costLimit7d, ok := optionalMoney(req.CostLimit7d)
	if !ok {
		return contract.CreatedKey{}, ErrInvalidInput
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
		CostQuota:        cloneStringPointer(costQuota),
		CostLimit5h:      cloneStringPointer(costLimit5h),
		CostLimit1d:      cloneStringPointer(costLimit1d),
		CostLimit7d:      cloneStringPointer(costLimit7d),
		AllowedIPs:       cloneStrings(req.AllowedIPs),
		DeniedIPs:        cloneStrings(req.DeniedIPs),
		ExpiresAt:        cloneTimePointer(req.ExpiresAt),
	})
	if err != nil {
		return contract.CreatedKey{}, err
	}

	return contract.CreatedKey{Key: withoutHash(stored), PlaintextKey: plaintext}, nil
}

func (s *Service) Delete(ctx context.Context, userID, keyID int) error {
	if userID <= 0 || keyID <= 0 {
		return ErrInvalidInput
	}
	keys, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	found := false
	for _, key := range keys {
		if key.ID == keyID {
			found = true
			break
		}
	}
	if !found {
		return ErrKeyNotFound
	}
	if err := s.store.Delete(ctx, keyID); err != nil {
		return err
	}
	// Bust the auth cache so a request mid-flight with the deleted key's
	// plaintext can no longer authenticate from the cached snapshot.
	if s.authCache != nil {
		s.authCache.InvalidateByKeyID(keyID)
	}
	return nil
}

// ResetUsage zeros the rolling cost-used counters on an API key (admin-only
// recovery action). Delegates to the store, which does it in a single UPDATE
// so the reset can't lose a race against an in-flight ApplyCostUsage.
func (s *Service) ResetUsage(ctx context.Context, keyID int) (contract.APIKey, error) {
	if keyID <= 0 {
		return contract.APIKey{}, ErrInvalidInput
	}
	key, err := s.store.ResetUsage(ctx, keyID)
	if err != nil {
		return contract.APIKey{}, err
	}
	// ResetUsage zeroes cost-used counters baked into the cached snapshot
	// (CostUsed5h etc.), so a stale cache hit would mislead downstream
	// quota checks until the TTL expired. Bust eagerly.
	if s.authCache != nil {
		s.authCache.InvalidateByKeyID(keyID)
	}
	return withoutHash(key), nil
}

// RevokeByUser soft-deletes every API key owned by userID and busts the
// auth cache for each one. Intended for the user-deletion path so that a
// soft-deleted user's keys can no longer authenticate gateway requests.
func (s *Service) RevokeByUser(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalidInput
	}
	keys, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := s.store.Delete(ctx, key.ID); err != nil {
			return err
		}
		if s.authCache != nil {
			s.authCache.InvalidateByKeyID(key.ID)
		}
	}
	return nil
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
	if req.CostQuota != nil {
		costQuota, ok := optionalMoney(req.CostQuota)
		if !ok {
			return contract.APIKey{}, ErrInvalidInput
		}
		key.CostQuota = cloneStringPointer(costQuota)
	}
	if req.CostLimit5h != nil {
		costLimit, ok := optionalMoney(req.CostLimit5h)
		if !ok {
			return contract.APIKey{}, ErrInvalidInput
		}
		key.CostLimit5h = cloneStringPointer(costLimit)
	}
	if req.CostLimit1d != nil {
		costLimit, ok := optionalMoney(req.CostLimit1d)
		if !ok {
			return contract.APIKey{}, ErrInvalidInput
		}
		key.CostLimit1d = cloneStringPointer(costLimit)
	}
	if req.CostLimit7d != nil {
		costLimit, ok := optionalMoney(req.CostLimit7d)
		if !ok {
			return contract.APIKey{}, ErrInvalidInput
		}
		key.CostLimit7d = cloneStringPointer(costLimit)
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
	if req.ExpiresAt != nil {
		// Set-if-present: a supplied timestamp updates expiry. Clearing an
		// existing expiry is not exposed through edit (mirror create-only set).
		key.ExpiresAt = cloneTimePointer(req.ExpiresAt)
	}
	updated, err := s.store.Update(ctx, key)
	if err != nil {
		if errors.Is(err, contract.ErrKeyNotFound) {
			return contract.APIKey{}, ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}
	// Status/scope/limit changes alter the cached snapshot's enforcement
	// outcome — bust so the next auth re-reads from SQL. Notably this is
	// the disable-key invalidation path the port directive calls out.
	if s.authCache != nil {
		s.authCache.InvalidateByKeyID(updated.ID)
	}
	return withoutHash(updated), nil
}

// ApplyCostUsage persists a successful request's billable USD cost to the key's
// materialized lifetime and rolling-window cost counters.
func (s *Service) ApplyCostUsage(ctx context.Context, input contract.CostUsageUpdate) (contract.APIKey, error) {
	if input.KeyID <= 0 {
		return contract.APIKey{}, ErrInvalidInput
	}
	cost, ok := optionalMoney(&input.BillableCost)
	if !ok || cost == nil {
		return contract.APIKey{}, ErrInvalidInput
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = s.clock.Now()
	}
	input.BillableCost = *cost
	input.OccurredAt = input.OccurredAt.UTC()
	updated, err := s.store.ApplyCostUsage(ctx, input)
	if err != nil {
		if errors.Is(err, contract.ErrKeyNotFound) {
			return contract.APIKey{}, ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}
	return withoutHash(updated), nil
}

func (s *Service) Authenticate(ctx context.Context, plaintext string) (contract.AuthResult, error) {
	// Cache fast-path: consult the in-memory LRU before SQL. The cache key
	// is sha256(plaintext) so brute-forcing prefixes doesn't probe membership.
	// A "notFound" cache hit short-circuits as ErrInvalidKey — identical to
	// the SQL-not-found branch below. Critically, the cache bypasses
	// TouchLastUsed() — that's intentional: last_used_at is observational,
	// not load-bearing, and writing it on every cached hit would defeat the
	// cache's purpose. The async RPM counter (Increment below at the call
	// site, plus the periodic flush) carries the "this key is hot" signal.
	if s.authCache != nil {
		if cached, userID, notFound, found := s.authCache.Get(ctx, plaintext); found {
			if notFound {
				return contract.AuthResult{}, ErrInvalidKey
			}
			return contract.AuthResult{Key: cached, UserID: userID, CachedAuth: true}, nil
		}
	}

	prefix, ok := PrefixFromPlaintext(plaintext)
	if !ok {
		return contract.AuthResult{}, ErrInvalidKey
	}
	key, err := s.store.FindByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, contract.ErrKeyNotFound) || errors.Is(err, ErrKeyNotFound) {
			// Negative-cache: only seed when the plaintext is well-formed.
			// A malformed plaintext would never round-trip to the SQL store,
			// so caching it under sha256 would waste a slot.
			if s.authCache != nil {
				s.authCache.PutNotFound(ctx, plaintext)
			}
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
	result := contract.AuthResult{Key: withoutHash(key), UserID: key.UserID}
	if s.authCache != nil {
		s.authCache.PutPositive(ctx, plaintext, result.Key, result.UserID)
	}
	return result, nil
}

// DeletedKeyMatchFromPlaintext returns low-sensitive tombstone evidence when
// plaintext exactly matches a soft-deleted API key. It is intended for
// operator-facing auth-failure logs; callers must not use it for authentication.
func (s *Service) DeletedKeyMatchFromPlaintext(ctx context.Context, plaintext string) (contract.DeletedKeyMatch, bool, error) {
	if s == nil {
		return contract.DeletedKeyMatch{}, false, ErrInvalidInput
	}
	prefix, ok := PrefixFromPlaintext(plaintext)
	if !ok {
		return contract.DeletedKeyMatch{}, false, nil
	}
	key, err := s.store.FindDeletedByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, contract.ErrKeyNotFound) || errors.Is(err, ErrKeyNotFound) {
			return contract.DeletedKeyMatch{}, false, nil
		}
		return contract.DeletedKeyMatch{}, false, err
	}
	if !hmac.Equal([]byte(key.Hash), []byte(s.HashPlaintext(plaintext))) {
		return contract.DeletedKeyMatch{}, false, nil
	}
	return contract.DeletedKeyMatch{
		KeyID:  key.ID,
		UserID: key.UserID,
		Name:   key.Name,
		Prefix: key.Prefix,
	}, true, nil
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

func optionalMoney(value *string) (*string, bool) {
	if value == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, true
	}
	rat, ok := money.DecimalRat(money.NormalizeAmount(trimmed))
	if !ok || rat.Sign() < 0 {
		return nil, false
	}
	normalized := money.FormatRatFixed(rat, 8)
	return &normalized, true
}

func cloneStringPointer(value *string) *string {
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
