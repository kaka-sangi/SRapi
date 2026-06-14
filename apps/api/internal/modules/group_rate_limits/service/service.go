package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	"github.com/srapi/srapi/apps/api/internal/platform/localcache"
)

// ErrInvalidInput is returned for malformed input.
var ErrInvalidInput = errors.New("invalid account group rate limit")

// ruleCacheTTL bounds how long a group's rate-limit rule is memoized. Lookups
// run on every gateway request (RPM + TPM + concurrency each touch the same row)
// but the rules change rarely, so we cache them for a short window and
// invalidate on write. Live counting stays in Redis — only the static ceilings
// are cached.
const ruleCacheTTL = 15 * time.Second

type Service struct {
	store contract.Store
	cache *localcache.Cache[contract.Limit]
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{
		store: store,
		cache: localcache.New[contract.Limit](localcache.Config{
			MaxEntries: 512,
			DefaultTTL: ruleCacheTTL,
		}),
	}, nil
}

func (s *Service) ListLimits(ctx context.Context) ([]contract.Limit, error) {
	return s.store.ListLimits(ctx)
}

func (s *Service) UpsertLimit(ctx context.Context, input contract.UpsertLimit) (contract.Limit, error) {
	if input.GroupID <= 0 || input.RPMLimit < 0 || input.TPMLimit < 0 || input.MaxConcurrency < 0 {
		return contract.Limit{}, ErrInvalidInput
	}
	limit, err := s.store.UpsertLimit(ctx, input)
	s.cache.Delete(strconv.Itoa(input.GroupID))
	return limit, err
}

func (s *Service) DeleteLimit(ctx context.Context, groupID int) error {
	if groupID <= 0 {
		return ErrInvalidInput
	}
	err := s.store.DeleteByGroup(ctx, groupID)
	s.cache.Delete(strconv.Itoa(groupID))
	return err
}

// findCached returns the group's active limit (a zero-value Limit when no rule
// applies), memoized for ruleCacheTTL. The three For* helpers each previously
// re-read the same row per request; this collapses them to one lookup and also
// survives across requests since writes invalidate the entry. A genuine
// "not found" is cached as the zero value (Enabled=false → no limit), but a
// transient store error is NOT cached — we stay fail-open without suppressing a
// real rule for the whole TTL window on a momentary blip.
func (s *Service) findCached(ctx context.Context, groupID int) contract.Limit {
	key := strconv.Itoa(groupID)
	if cached, ok := s.cache.Get(key); ok {
		return cached
	}
	limit, err := s.store.FindByGroup(ctx, groupID)
	switch {
	case err == nil:
		s.cache.Set(key, limit)
		return limit
	case errors.Is(err, contract.ErrNotFound):
		s.cache.Set(key, contract.Limit{})
		return contract.Limit{}
	default:
		return contract.Limit{}
	}
}

// RPMForGroup returns the active RPM ceiling for a group, or 0 when none applies
// (no rule, disabled, non-positive limit, or error — fail-open so rate-limit
// lookups never block traffic).
func (s *Service) RPMForGroup(ctx context.Context, groupID int) int {
	if groupID <= 0 {
		return 0
	}
	limit := s.findCached(ctx, groupID)
	if !limit.Enabled || limit.RPMLimit <= 0 {
		return 0
	}
	return limit.RPMLimit
}

// TPMForGroup returns the active tokens-per-minute ceiling for a group, or 0
// when none applies (fail-open).
func (s *Service) TPMForGroup(ctx context.Context, groupID int) int {
	if groupID <= 0 {
		return 0
	}
	limit := s.findCached(ctx, groupID)
	if !limit.Enabled || limit.TPMLimit <= 0 {
		return 0
	}
	return limit.TPMLimit
}

// ConcurrencyForGroup returns the active max-concurrency ceiling for a group, or
// 0 when none applies (fail-open).
func (s *Service) ConcurrencyForGroup(ctx context.Context, groupID int) int {
	if groupID <= 0 {
		return 0
	}
	limit := s.findCached(ctx, groupID)
	if !limit.Enabled || limit.MaxConcurrency <= 0 {
		return 0
	}
	return limit.MaxConcurrency
}
