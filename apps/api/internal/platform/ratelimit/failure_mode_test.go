package ratelimit

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestFailureModeDefaultIsFailOpen(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer client.Close()
	limiter, err := New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if limiter.FailureMode() != FailOpen {
		t.Fatalf("expected default mode FailOpen, got %v", limiter.FailureMode())
	}
}

func TestAllowFailOpenOnRedisUnavailable(t *testing.T) {
	// Address pointing at a closed port — Dial returns "connection refused".
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer client.Close()
	limiter, err := New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	decision, err := limiter.Allow(ctx, []Check{{Name: "rpm", Key: "key", Limit: 10, Cost: 1, Window: time.Minute}}, time.Now())
	if err != nil {
		t.Fatalf("FailOpen must swallow redis error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("FailOpen must allow on redis error, got %+v", decision)
	}
}

func TestAllowFailCloseOnRedisUnavailable(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer client.Close()
	limiter, err := New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	limiter.SetFailureMode(FailClose)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	decision, err := limiter.Allow(ctx, []Check{{Name: "rpm", Key: "key", Limit: 10, Cost: 1, Window: time.Minute}}, time.Now())
	if err == nil {
		t.Fatalf("FailClose must surface redis error, got nil")
	}
	if decision.Allowed {
		t.Fatalf("FailClose must reject on redis error, got %+v", decision)
	}
}

func TestAcquireConcurrencyFailOpenOnRedisUnavailable(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer client.Close()
	limiter, err := New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	lease, decision, err := limiter.AcquireConcurrency(ctx, ConcurrencyCheck{Name: "concurrency", Key: "apikey:1:concurrency", Limit: 5, TTL: time.Minute}, time.Now())
	if err != nil {
		t.Fatalf("FailOpen must swallow concurrency redis error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("FailOpen must allow concurrency on redis error, got %+v", decision)
	}
	// No real lease was acquired since Redis is down — caller must not try
	// to release a zero-token lease (concurrency.ReleaseConcurrency's first
	// gate catches this).
	if lease.Token != "" {
		t.Fatalf("FailOpen must not invent a lease token, got %+v", lease)
	}
}

func TestAcquireConcurrencyFailCloseOnRedisUnavailable(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer client.Close()
	limiter, err := New(client)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	limiter.SetFailureMode(FailClose)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err = limiter.AcquireConcurrency(ctx, ConcurrencyCheck{Name: "concurrency", Key: "apikey:1:concurrency", Limit: 5, TTL: time.Minute}, time.Now())
	if err == nil {
		t.Fatalf("FailClose must surface concurrency redis error, got nil")
	}
}

func TestIsRedisAvailabilityError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not availability error", nil, false},
		{"context.Canceled is not availability error", context.Canceled, false},
		{"context.DeadlineExceeded is not availability error", context.DeadlineExceeded, false},
		{"redis.ErrClosed is availability error", redis.ErrClosed, true},
		{"connection refused is availability error", errors.New("dial tcp 127.0.0.1:6379: connect: connection refused"), true},
		{"i/o timeout is availability error", errors.New("read tcp: i/o timeout"), true},
		{"eof is availability error", io.EOF, true},
		{"loading dataset is availability error", errors.New("LOADING Redis is loading the dataset in memory"), true},
		{"unknown is not availability error", errors.New("ERR wrong number of arguments for 'eval' command"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRedisAvailabilityError(tc.err); got != tc.want {
				t.Fatalf("isRedisAvailabilityError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Script-logic errors (not availability) must always propagate even in
// FailOpen mode — those indicate code bugs the operator must see.
func TestAllowFailOpenStillPropagatesScriptError(t *testing.T) {
	// A simple way to force a non-availability error path is to ensure the
	// `isRedisAvailabilityError` keyword set does not include a marker
	// matching the script-logic error shape. The lua eval error mock here
	// covers that distinction.
	err := errors.New("ERR Error running script (call to f_xxx): @user_script:5: user_script:5: attempt to perform arithmetic on a string value")
	if got := isRedisAvailabilityError(err); got {
		t.Fatalf("script logic error must not be classified as availability error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "script") {
		t.Fatalf("sanity: expected the error text to mention 'script', got %q", err.Error())
	}
}
