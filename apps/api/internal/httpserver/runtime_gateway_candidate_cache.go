package httpserver

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	"golang.org/x/sync/singleflight"
)

// candidateAccountCache coalesces concurrent ListActiveByProviderIDs reads
// under burst traffic. sub2api uses a singleflight + 200ms TTL cache in its
// ConcurrencyService.GetAccountsLoadBatch to avoid N identical Redis calls
// when N requests for the same model arrive simultaneously. SRapi's equivalent
// hot path is the SQL read in gatewayCandidates; this cache applies the same
// pattern at the DB layer.
//
// The 200ms TTL is short enough that metadata changes (cooldown, quota) are
// visible within one scheduling cycle (~5 requests at 40 req/s), while still
// coalescing bursts of 10–100 concurrent calls into a single DB read.
type candidateAccountCache struct {
	accounts *accountservice.Service
	group    singleflight.Group

	mu    sync.Mutex
	cache map[string]*cachedAccountsBatch
}

type cachedAccountsBatch struct {
	accounts []accountcontract.ProviderAccount
	fetchedAt time.Time
}

const candidateCacheTTL = 200 * time.Millisecond

func newCandidateAccountCache(accounts *accountservice.Service) *candidateAccountCache {
	return &candidateAccountCache{
		accounts: accounts,
		cache:    make(map[string]*cachedAccountsBatch),
	}
}

func (c *candidateAccountCache) ListActiveByProviderIDs(ctx context.Context, providerIDs []int) ([]accountcontract.ProviderAccount, error) {
	if c == nil || c.accounts == nil {
		return nil, nil
	}
	key := providerIDsCacheKey(providerIDs)

	c.mu.Lock()
	if cached, ok := c.cache[key]; ok && time.Since(cached.fetchedAt) < candidateCacheTTL {
		c.mu.Unlock()
		return cached.accounts, nil
	}
	c.mu.Unlock()

	result, err, _ := c.group.Do(key, func() (any, error) {
		accounts, err := c.accounts.ListActiveByProviderIDs(ctx, providerIDs)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.cache[key] = &cachedAccountsBatch{
			accounts:  accounts,
			fetchedAt: time.Now(),
		}
		if len(c.cache) > 256 {
			c.evictLocked()
		}
		c.mu.Unlock()
		return accounts, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]accountcontract.ProviderAccount), nil
}

func (c *candidateAccountCache) evictLocked() {
	now := time.Now()
	for k, v := range c.cache {
		if now.Sub(v.fetchedAt) > candidateCacheTTL {
			delete(c.cache, k)
		}
	}
}

func providerIDsCacheKey(ids []int) string {
	sorted := make([]int, len(ids))
	copy(sorted, ids)
	sort.Ints(sorted)
	parts := make([]string, len(sorted))
	for i, id := range sorted {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
