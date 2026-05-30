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
		return nil, nil // fail-open: a group lookup error must not block traffic
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

// releaseGatewayConcurrency releases every held lease (empty leases are skipped
// by releaseProviderAccountConcurrency).
func (rt *runtimeState) releaseGatewayConcurrency(leases []ratelimit.ConcurrencyLease) {
	for _, lease := range leases {
		rt.releaseProviderAccountConcurrency(lease)
	}
}
