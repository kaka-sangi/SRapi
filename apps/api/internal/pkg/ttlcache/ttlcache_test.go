package ttlcache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestGetLoadsOnceWithinTTL(t *testing.T) {
	current := time.Unix(0, 0)
	cache := New[int](3*time.Second, func() time.Time { return current })
	loads := 0
	load := func(context.Context) (int, error) {
		loads++
		return 42, nil
	}
	for i := 0; i < 5; i++ {
		got, err := cache.Get(context.Background(), load)
		if err != nil || got != 42 {
			t.Fatalf("Get = %d, %v", got, err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}

	current = current.Add(3 * time.Second)
	if _, err := cache.Get(context.Background(), load); err != nil {
		t.Fatal(err)
	}
	if loads != 2 {
		t.Fatalf("loads after expiry = %d, want 2", loads)
	}
}

func TestInvalidateForcesReload(t *testing.T) {
	cache := New[string](time.Minute, nil)
	value := "first"
	load := func(context.Context) (string, error) { return value, nil }
	if got, _ := cache.Get(context.Background(), load); got != "first" {
		t.Fatalf("got %q", got)
	}
	value = "second"
	if got, _ := cache.Get(context.Background(), load); got != "first" {
		t.Fatalf("cached read = %q, want first", got)
	}
	cache.Invalidate()
	if got, _ := cache.Get(context.Background(), load); got != "second" {
		t.Fatalf("post-invalidate read = %q, want second", got)
	}
}

func TestStaleValueServedOnLoadError(t *testing.T) {
	current := time.Unix(0, 0)
	cache := New[int](time.Second, func() time.Time { return current })
	healthy := true
	load := func(context.Context) (int, error) {
		if healthy {
			return 7, nil
		}
		return 0, errors.New("store down")
	}
	if got, err := cache.Get(context.Background(), load); err != nil || got != 7 {
		t.Fatalf("Get = %d, %v", got, err)
	}
	healthy = false
	current = current.Add(time.Hour)
	got, err := cache.Get(context.Background(), load)
	if err != nil || got != 7 {
		t.Fatalf("stale Get = %d, %v; want 7, nil", got, err)
	}
}

func TestErrorPropagatesWithoutPriorValue(t *testing.T) {
	cache := New[int](time.Second, nil)
	wantErr := errors.New("boom")
	if _, err := cache.Get(context.Background(), func(context.Context) (int, error) { return 0, wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestInvalidateDropsStaleFallback(t *testing.T) {
	cache := New[int](time.Minute, nil)
	if _, err := cache.Get(context.Background(), func(context.Context) (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate()
	wantErr := errors.New("boom")
	if _, err := cache.Get(context.Background(), func(context.Context) (int, error) { return 0, wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v (no stale fallback after Invalidate)", err, wantErr)
	}
}

func TestZeroTTLAlwaysLoads(t *testing.T) {
	cache := New[int](0, nil)
	loads := 0
	load := func(context.Context) (int, error) {
		loads++
		return loads, nil
	}
	cache.Get(context.Background(), load)
	cache.Get(context.Background(), load)
	if loads != 2 {
		t.Fatalf("loads = %d, want 2", loads)
	}
}

func TestConcurrentGetsCollapse(t *testing.T) {
	current := time.Unix(0, 0)
	cache := New[int](time.Minute, func() time.Time { return current })
	loads := 0
	load := func(context.Context) (int, error) {
		loads++
		time.Sleep(5 * time.Millisecond)
		return 9, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := cache.Get(context.Background(), load); err != nil || got != 9 {
				t.Errorf("Get = %d, %v", got, err)
			}
		}()
	}
	wg.Wait()
	if loads != 1 {
		t.Fatalf("loads = %d, want 1 (misses must collapse)", loads)
	}
}
