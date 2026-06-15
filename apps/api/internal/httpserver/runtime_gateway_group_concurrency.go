package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

// acquireAccountGroupConcurrency acquires a concurrency lease for each group the
// account belongs to that has a max-concurrency ceiling (WP-1210). On any
// failure it rolls back the leases already taken so none leak. Returns nil when
// no group limits apply.
func (rt *runtimeState) acquireAccountGroupConcurrency(ctx context.Context, account accountcontract.ProviderAccount) ([]ratelimit.ConcurrencyLease, error) {
	if rt.rateLimiter == nil || rt.groupRateLimits == nil || account.ID <= 0 {
		return nil, nil
	}
	groupIDs, err := rt.accounts.ListGroupIDsByAccount(ctx, account.ID)
	if err != nil {
		// fail-open: a group lookup error must not block traffic, but surface it
		// so silently bypassed group concurrency limits stay observable.
		if rt.logger != nil {
			rt.logger.Warn("group concurrency lookup failed; bypassing group limits", "account_id", account.ID, "error", err)
		}
		return nil, nil
	}
	var leases []ratelimit.ConcurrencyLease
	for _, groupID := range groupIDs {
		limit := rt.groupRateLimits.ConcurrencyForGroup(ctx, groupID)
		if limit <= 0 {
			continue
		}
		lease, decision, err := rt.rateLimiter.AcquireConcurrency(ctx, ratelimit.ConcurrencyCheck{
			Name:  "group_concurrency",
			Key:   fmt.Sprintf("group:%d:concurrency", groupID),
			Limit: limit,
			TTL:   rt.providerAccountConcurrencyTTL(),
		}, time.Now().UTC())
		if err != nil {
			rt.releaseGatewayConcurrency(leases)
			return nil, err
		}
		if !decision.Allowed {
			rt.releaseGatewayConcurrency(leases)
			return nil, provideradaptercontract.ProviderError{
				Class:      "concurrency_limit_exceeded",
				StatusCode: http.StatusTooManyRequests,
				Message:    "account group concurrency limit exceeded",
			}
		}
		leases = append(leases, lease)
	}
	return leases, nil
}

// acquireModelConcurrency acquires a global concurrency lease for a model that
// has a max-concurrency ceiling (WP-1220). Returns an empty lease when no limit
// applies.
func (rt *runtimeState) acquireModelConcurrency(ctx context.Context, modelID int) (ratelimit.ConcurrencyLease, error) {
	if rt.rateLimiter == nil || rt.modelRateLimits == nil || modelID <= 0 {
		return ratelimit.ConcurrencyLease{}, nil
	}
	limit := rt.modelRateLimits.ConcurrencyForModel(ctx, modelID)
	if limit <= 0 {
		return ratelimit.ConcurrencyLease{}, nil
	}
	lease, decision, err := rt.rateLimiter.AcquireConcurrency(ctx, ratelimit.ConcurrencyCheck{
		Name:  "model_concurrency",
		Key:   fmt.Sprintf("model:%d:concurrency", modelID),
		Limit: limit,
		TTL:   rt.providerAccountConcurrencyTTL(),
	}, time.Now().UTC())
	if err != nil {
		return ratelimit.ConcurrencyLease{}, err
	}
	if !decision.Allowed {
		return ratelimit.ConcurrencyLease{}, provideradaptercontract.ProviderError{
			Class:      "concurrency_limit_exceeded",
			StatusCode: http.StatusTooManyRequests,
			Message:    "model concurrency limit exceeded",
		}
	}
	return lease, nil
}

// releaseGatewayConcurrency releases every held lease (empty leases are skipped
// by releaseProviderAccountConcurrency).
func (rt *runtimeState) releaseGatewayConcurrency(leases []ratelimit.ConcurrencyLease) {
	for _, lease := range leases {
		rt.releaseProviderAccountConcurrency(lease)
	}
}
