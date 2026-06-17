package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"

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
	// sf collapses concurrent cache-miss store reads for the same group_id into
	// a single round-trip. Mirrors sub2api's userGroupRateResolver (see
	// /backend/internal/service/user_group_rate_resolver.go): N concurrent
	// gateway requests for the same group survive a brief cache stampede
	// without N parallel DB hits.
	sf singleflight.Group
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
	// Collapse concurrent stampedes onto a single store read. The shared
	// closure re-checks the cache first so a caller that arrives after the
	// owner populated it still gets a hit (and the owner's followers reuse
	// the same Limit without a second store hop).
	value, err, _ := s.sf.Do(key, func() (any, error) {
		if cached, ok := s.cache.Get(key); ok {
			return cached, nil
		}
		limit, storeErr := s.store.FindByGroup(ctx, groupID)
		switch {
		case storeErr == nil:
			s.cache.Set(key, limit)
			return limit, nil
		case errors.Is(storeErr, contract.ErrNotFound):
			s.cache.Set(key, contract.Limit{})
			return contract.Limit{}, nil
		default:
			// Transient store error: do NOT cache the zero value (mirrors the
			// pre-singleflight behaviour — fail-open on this request only).
			return contract.Limit{}, storeErr
		}
	})
	if err != nil {
		return contract.Limit{}
	}
	limit, ok := value.(contract.Limit)
	if !ok {
		return contract.Limit{}
	}
	return limit
}

// BatchSetRPMOverridesMaxItems caps the number of items per
// BatchSetRPMOverrides call. Same operator-facing cap as the other batch ops.
const BatchSetRPMOverridesMaxItems = 1000

// BatchSetRPMOverrides sets the per-group RPM ceiling for N account groups
// in one call. Verbatim port of sub2api's BatchSetGroupRPMOverrides
// (admin_service.go) — sub2api scopes RPM overrides to user-groups but srapi
// stores the RPM ceiling on the per-group rate-limit row, so the per-row
// identifier is account_group_id and the override updates the same
// AccountGroupRateLimit row that single-row UpsertLimit writes.
//
// Best-effort across the batch: a single-row failure populates that row's
// Error and the rest continues. Dedups within the batch.
//
// Per-row validation mirrors sub2api's `RPMOverride >= 0` check (the sub2api
// impl rejects negatives with INVALID_RPM_OVERRIDE). nil clears the override
// (sets RPMLimit=0 + leaves the row's other fields untouched) — this matches
// sub2api's nil-pointer-means-clear contract.
//
// Cache invalidation: each successful row's group cache entry is dropped
// (the same path the single-row UpsertLimit takes), so the next request for
// that group reads the fresh ceiling without waiting for the TTL.
func (s *Service) BatchSetRPMOverrides(ctx context.Context, items []contract.BatchSetRPMOverrideItem) ([]contract.BatchSetRPMOverrideResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchSetRPMOverridesMaxItems {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchSetRPMOverrideResult, 0, len(items))
	seen := make(map[int]struct{}, len(items))
	for i, item := range items {
		row := contract.BatchSetRPMOverrideResult{Index: i, GroupID: item.GroupID}
		if item.GroupID <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if item.RPMOverride != nil && *item.RPMOverride < 0 {
			row.Error = "rpm_override must be >= 0"
			results = append(results, row)
			continue
		}
		if _, dup := seen[item.GroupID]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[item.GroupID] = struct{}{}

		// Read the existing row (if any) so we preserve TPM/MaxConcurrency
		// and only change the RPM ceiling. nil means "clear the override" →
		// rpm_limit becomes 0 (which is "no limit / unlimited" per the rest
		// of this module). Idempotent NotFound: when neither a row nor an
		// override is requested (nil + no row), it's a no-op success.
		existing, err := s.store.FindByGroup(ctx, item.GroupID)
		if err != nil && !errors.Is(err, contract.ErrNotFound) {
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		// If the row didn't exist and the operator is clearing → idempotent
		// success without a write.
		if errors.Is(err, contract.ErrNotFound) && item.RPMOverride == nil {
			results = append(results, row)
			continue
		}
		input := contract.UpsertLimit{
			GroupID:        item.GroupID,
			RPMLimit:       0,
			TPMLimit:       existing.TPMLimit,
			MaxConcurrency: existing.MaxConcurrency,
			Enabled:        true,
		}
		if errors.Is(err, contract.ErrNotFound) {
			// Brand-new row — enable so the override takes effect at all.
			input.Enabled = true
		} else {
			input.Enabled = existing.Enabled
		}
		if item.RPMOverride != nil {
			input.RPMLimit = *item.RPMOverride
		}
		if _, err := s.UpsertLimit(ctx, input); err != nil {
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
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
