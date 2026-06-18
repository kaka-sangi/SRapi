package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/service"
)

// blockingStore counts FindByGroup calls and blocks them on a shared channel so
// the test can pin "N concurrent callers -> 1 store hit" semantics for the new
// singleflight protection (ports sub2api's userGroupRateResolver behaviour).
type blockingStore struct {
	contract.Store // embedded — only FindByGroup is exercised
	calls          atomic.Int32
	release        chan struct{}
	limit          contract.Limit
}

func newBlockingStore(limit contract.Limit) *blockingStore {
	return &blockingStore{release: make(chan struct{}), limit: limit}
}

func (s *blockingStore) FindByGroup(_ context.Context, groupID int) (contract.Limit, error) {
	s.calls.Add(1)
	<-s.release
	if s.limit.GroupID == 0 {
		s.limit.GroupID = groupID
	}
	return s.limit, nil
}

func (s *blockingStore) UpsertLimit(_ context.Context, in contract.UpsertLimit) (contract.Limit, error) {
	return contract.Limit{GroupID: in.GroupID}, nil
}
func (s *blockingStore) DeleteByGroup(_ context.Context, _ int) error           { return nil }
func (s *blockingStore) ListLimits(_ context.Context) ([]contract.Limit, error) { return nil, nil }

func TestGroupRateLimits_Singleflight_CoalescesConcurrentLookups(t *testing.T) {
	store := newBlockingStore(contract.Limit{GroupID: 7, RPMLimit: 120, Enabled: true})
	svc, err := service.New(store)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const fanout = 16
	var wg sync.WaitGroup
	results := make(chan int, fanout)
	for i := 0; i < fanout; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- svc.RPMForGroup(context.Background(), 7)
		}()
	}

	// Let goroutines pile up at the blocked FindByGroup call, then release them.
	time.Sleep(50 * time.Millisecond)
	close(store.release)
	wg.Wait()
	close(results)

	if got := store.calls.Load(); got != 1 {
		t.Fatalf("FindByGroup calls = %d, want 1 (singleflight should collapse)", got)
	}
	for r := range results {
		if r != 120 {
			t.Fatalf("RPMForGroup result = %d, want 120", r)
		}
	}
}
