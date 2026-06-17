package httpserver

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGatewayDedupKey_StableAcrossMapKeyOrder(t *testing.T) {
	body1 := map[string]any{
		"model":       "gpt-4o",
		"temperature": 0.7,
		"top_p":       0.95,
	}
	body2 := map[string]any{
		"top_p":       0.95,
		"temperature": 0.7,
		"model":       "gpt-4o",
	}
	messages := []any{
		map[string]any{"role": "user", "content": "hello"},
	}

	k1 := GatewayDedupKey(body1, messages, false)
	k2 := GatewayDedupKey(body2, messages, false)
	if k1 != k2 {
		t.Fatalf("keys differ across map ordering: %q vs %q", k1, k2)
	}
}

func TestGatewayDedupKey_DifferentStreamFlagDifferentKey(t *testing.T) {
	body := map[string]any{"model": "gpt-4o"}
	messages := []any{map[string]any{"role": "user", "content": "hi"}}

	if GatewayDedupKey(body, messages, false) == GatewayDedupKey(body, messages, true) {
		t.Fatalf("stream flag must change key")
	}
}

func TestGatewayDedupKey_IgnoresNonCacheableKeys(t *testing.T) {
	base := map[string]any{"model": "gpt-4o", "temperature": 0.5}
	messages := []any{map[string]any{"role": "user", "content": "hi"}}
	keyA := GatewayDedupKey(base, messages, false)

	withGarbage := map[string]any{"model": "gpt-4o", "temperature": 0.5, "request_id": "abc-123"}
	keyB := GatewayDedupKey(withGarbage, messages, false)
	if keyA != keyB {
		t.Fatalf("non-cacheable key affected hash: %q vs %q", keyA, keyB)
	}
}

func TestGatewayDedupGetOrCompute_CoalescesParallelCallers(t *testing.T) {
	d := newGatewayCompletionDedup(0, 0)
	defer d.Clear()

	var calls atomic.Int32
	release := make(chan struct{})

	const fanout = 12
	var wg sync.WaitGroup
	wg.Add(fanout)
	results := make([]gatewayDedupResult, fanout)
	errs := make([]error, fanout)
	for i := 0; i < fanout; i++ {
		i := i
		go func() {
			defer wg.Done()
			res, err := d.GetOrCompute(context.Background(), "key1", func() (gatewayDedupResult, error) {
				calls.Add(1)
				<-release
				return gatewayDedupResult{Body: []byte(`{"id":"r1"}`)}, nil
			})
			results[i] = res
			errs[i] = err
		}()
	}

	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("compute() calls = %d, want 1", got)
	}
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("caller %d err = %v", i, errs[i])
		}
		if string(r.Body) != `{"id":"r1"}` {
			t.Fatalf("caller %d body = %q", i, string(r.Body))
		}
	}
}

func TestGatewayDedupGetOrCompute_TTLExpiresCache(t *testing.T) {
	d := newGatewayCompletionDedup(15*time.Millisecond, 0)
	defer d.Clear()

	var calls atomic.Int32
	compute := func() (gatewayDedupResult, error) {
		calls.Add(1)
		return gatewayDedupResult{Body: []byte("ok")}, nil
	}

	if _, err := d.GetOrCompute(context.Background(), "k", compute); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := d.GetOrCompute(context.Background(), "k", compute); err != nil {
		t.Fatalf("cached call: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("within TTL calls = %d, want 1", got)
	}

	time.Sleep(30 * time.Millisecond)
	if _, err := d.GetOrCompute(context.Background(), "k", compute); err != nil {
		t.Fatalf("post-ttl call: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("after TTL calls = %d, want 2", got)
	}
}

func TestGatewayDedupGetOrCompute_ErrorNotCached(t *testing.T) {
	d := newGatewayCompletionDedup(0, 0)
	defer d.Clear()

	var calls atomic.Int32
	target := errors.New("upstream blew up")
	compute := func() (gatewayDedupResult, error) {
		calls.Add(1)
		return gatewayDedupResult{}, target
	}

	if _, err := d.GetOrCompute(context.Background(), "k", compute); !errors.Is(err, target) {
		t.Fatalf("first err = %v, want %v", err, target)
	}
	if _, err := d.GetOrCompute(context.Background(), "k", compute); !errors.Is(err, target) {
		t.Fatalf("second err = %v, want %v", err, target)
	}
	// Errors must NOT be cached — both callers should invoke compute.
	if got := calls.Load(); got != 2 {
		t.Fatalf("compute calls = %d, want 2 (errors must not be cached)", got)
	}
}

func TestGatewayDedupGetOrCompute_FollowerHonoursContextCancel(t *testing.T) {
	d := newGatewayCompletionDedup(0, 0)
	defer d.Clear()

	release := make(chan struct{})
	go func() {
		_, _ = d.GetOrCompute(context.Background(), "k", func() (gatewayDedupResult, error) {
			<-release
			return gatewayDedupResult{Body: []byte("ok")}, nil
		})
	}()

	// Let the owner register the in-flight call.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() {
		_, err := d.GetOrCompute(ctx, "k", func() (gatewayDedupResult, error) {
			t.Errorf("follower compute should not run")
			return gatewayDedupResult{}, nil
		})
		doneCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-doneCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("follower err = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follower did not return after cancel")
	}
	close(release)
}

func TestGatewayDedupLRUBounded(t *testing.T) {
	d := newGatewayCompletionDedup(time.Hour, 4)
	defer d.Clear()

	for i := 0; i < 10; i++ {
		key := strconv.Itoa(i)
		_, err := d.GetOrCompute(context.Background(), key, func() (gatewayDedupResult, error) {
			return gatewayDedupResult{Body: []byte(key)}, nil
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	d.mu.Lock()
	size := d.entryOrder.Len()
	d.mu.Unlock()
	if size != 4 {
		t.Fatalf("LRU size = %d, want 4", size)
	}
}

func TestShouldDedupChatCompletion(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"nil", nil, false},
		{"stream true", map[string]any{"stream": true, "messages": []any{map[string]any{"role": "user"}}}, false},
		{"non-streaming with messages", map[string]any{"stream": false, "messages": []any{map[string]any{"role": "user"}}}, true},
		{"stream absent with messages", map[string]any{"messages": []any{map[string]any{"role": "user"}}}, true},
		{"empty messages", map[string]any{"messages": []any{}}, false},
		{"no messages key", map[string]any{"model": "gpt-4o"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldDedupChatCompletion(tc.body); got != tc.want {
				t.Fatalf("ShouldDedupChatCompletion = %v, want %v", got, tc.want)
			}
		})
	}
}
