package service

import (
	"context"
	"testing"
	"time"

	contract "github.com/srapi/srapi/apps/api/internal/modules/error_event_stream/contract"
)

func TestMemoryPublisher_FanOut(t *testing.T) {
	t.Parallel()
	p := NewMemoryPublisher(Config{})
	defer p.Close()

	ctx := context.Background()
	subs := make([]contract.Subscriber, 0, 3)
	for i := 0; i < 3; i++ {
		s, err := p.Subscribe(ctx, contract.SubscribeOptions{})
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		subs = append(subs, s)
	}

	ev := contract.Event{
		AtUnixMs:   time.Now().UnixMilli(),
		RequestID:  "req-1",
		StatusCode: 500,
		ErrorClass: "server_bad",
		Message:    "boom",
	}
	if err := p.Publish(ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for i, s := range subs {
		select {
		case got, ok := <-s.Receive():
			if !ok {
				t.Fatalf("subscriber %d channel closed unexpectedly", i)
			}
			if got.RequestID != ev.RequestID {
				t.Fatalf("subscriber %d got request_id %q want %q", i, got.RequestID, ev.RequestID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}

func TestMemoryPublisher_SubscriberDropOnOverflow(t *testing.T) {
	t.Parallel()
	// Tiny queue makes overflow deterministic.
	p := NewMemoryPublisher(Config{SubscriberQueueSize: 2})
	defer p.Close()

	ctx := context.Background()
	s, err := p.Subscribe(ctx, contract.SubscribeOptions{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Publish more than the queue can hold without draining. The 3rd publish
	// must evict the subscriber (channel closed, removed from publisher).
	for i := 0; i < 5; i++ {
		if err := p.Publish(ctx, contract.Event{
			AtUnixMs:   time.Now().UnixMilli(),
			RequestID:  "req",
			StatusCode: 500,
		}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	// Drain the buffered entries then expect the channel to be closed.
	drained := 0
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-s.Receive():
			if !ok {
				if drained < 2 {
					t.Fatalf("drained=%d, expected at least 2 before close", drained)
				}
				if p.SubscriberCount() != 0 {
					t.Fatalf("evicted subscriber still tracked: %d", p.SubscriberCount())
				}
				return
			}
			drained++
		case <-deadline:
			t.Fatalf("never observed channel close (drained=%d)", drained)
		}
	}
}

func TestMemoryPublisher_FilterMatching(t *testing.T) {
	t.Parallel()
	p := NewMemoryPublisher(Config{})
	defer p.Close()

	ctx := context.Background()
	acct := 42
	s, err := p.Subscribe(ctx, contract.SubscribeOptions{
		AccountID:     &acct,
		ErrorClass:    "server_bad",
		MinStatusCode: 500,
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	other := 7
	cases := []struct {
		name string
		ev   contract.Event
		want bool
	}{
		{
			name: "match",
			ev:   contract.Event{AccountID: &acct, ErrorClass: "server_bad", StatusCode: 502, RequestID: "ok"},
			want: true,
		},
		{
			name: "wrong account",
			ev:   contract.Event{AccountID: &other, ErrorClass: "server_bad", StatusCode: 502, RequestID: "nope-acct"},
			want: false,
		},
		{
			name: "wrong class",
			ev:   contract.Event{AccountID: &acct, ErrorClass: "client_bad", StatusCode: 502, RequestID: "nope-class"},
			want: false,
		},
		{
			name: "too low status",
			ev:   contract.Event{AccountID: &acct, ErrorClass: "server_bad", StatusCode: 400, RequestID: "nope-status"},
			want: false,
		},
	}

	for _, c := range cases {
		if err := p.Publish(ctx, c.ev); err != nil {
			t.Fatalf("publish %s: %v", c.name, err)
		}
	}

	got := make([]string, 0, 1)
	timeout := time.After(200 * time.Millisecond)
collect:
	for {
		select {
		case ev, ok := <-s.Receive():
			if !ok {
				break collect
			}
			got = append(got, ev.RequestID)
		case <-timeout:
			break collect
		}
	}
	if len(got) != 1 || got[0] != "ok" {
		t.Fatalf("filter mismatch: got %v want [ok]", got)
	}

	// Verify SinceUnixMs filter drops older events.
	since := time.Now().Add(time.Hour).UnixMilli()
	s2, err := p.Subscribe(ctx, contract.SubscribeOptions{SinceUnixMs: since})
	if err != nil {
		t.Fatalf("subscribe2: %v", err)
	}
	if err := p.Publish(ctx, contract.Event{AtUnixMs: time.Now().UnixMilli() - 1000, RequestID: "old"}); err != nil {
		t.Fatalf("publish old: %v", err)
	}
	select {
	case ev := <-s2.Receive():
		t.Fatalf("expected SinceUnixMs filter to drop event, got %#v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestMemoryPublisher_MaxSubscribers(t *testing.T) {
	t.Parallel()
	p := NewMemoryPublisher(Config{MaxSubscribers: 2})
	defer p.Close()

	ctx := context.Background()
	if _, err := p.Subscribe(ctx, contract.SubscribeOptions{}); err != nil {
		t.Fatalf("sub 1: %v", err)
	}
	if _, err := p.Subscribe(ctx, contract.SubscribeOptions{}); err != nil {
		t.Fatalf("sub 2: %v", err)
	}
	if _, err := p.Subscribe(ctx, contract.SubscribeOptions{}); err != contract.ErrTooManySubscribers {
		t.Fatalf("expected ErrTooManySubscribers, got %v", err)
	}
}

func TestMemoryPublisher_Replay(t *testing.T) {
	t.Parallel()
	p := NewMemoryPublisher(Config{RingSize: 8})
	defer p.Close()

	ctx := context.Background()
	base := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		_ = p.Publish(ctx, contract.Event{AtUnixMs: base + int64(i), RequestID: "r", StatusCode: 500})
	}

	s, err := p.Subscribe(ctx, contract.SubscribeOptions{SinceUnixMs: base + 2})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	count := 0
	timeout := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case _, ok := <-s.Receive():
			if !ok {
				break loop
			}
			count++
			if count >= 3 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 replayed entries (at base+2,+3,+4), got %d", count)
	}
}
