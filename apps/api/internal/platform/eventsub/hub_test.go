package eventsub

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPubSub(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(8)
	defer h.Unsubscribe(sub)

	h.Publish(Event{Type: "test", Data: map[string]string{"msg": "hello"}})

	select {
	case e := <-sub.Events():
		if e.Type != "test" {
			t.Fatalf("expected type 'test', got %q", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	h := NewHub()
	s1 := h.Subscribe(8)
	s2 := h.Subscribe(8)
	defer h.Unsubscribe(s1)
	defer h.Unsubscribe(s2)

	h.Publish(Event{Type: "x", Data: "y"})

	for _, s := range []*Subscriber{s1, s2} {
		select {
		case e := <-s.Events():
			if e.Type != "x" {
				t.Fatalf("expected 'x', got %q", e.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}

	if h.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", h.SubscriberCount())
	}
}

func TestUnsubscribe(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(8)
	h.Unsubscribe(sub)

	if h.SubscriberCount() != 0 {
		t.Fatal("expected 0 subscribers after unsubscribe")
	}

	// Publishing after unsubscribe should not panic.
	h.Publish(Event{Type: "test", Data: nil})
}

func TestSlowSubscriberDrops(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(2) // tiny buffer
	defer h.Unsubscribe(sub)

	for i := range 10 {
		h.Publish(Event{Type: "flood", Data: i})
	}

	// Should have received at most bufSize events.
	count := 0
	for {
		select {
		case <-sub.Events():
			count++
		default:
			goto done
		}
	}
done:
	if count > 2 {
		t.Fatalf("expected at most 2 events (buf size), got %d", count)
	}
}

func TestConcurrentPubSub(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup

	for i := range 4 {
		sub := h.Subscribe(64)
		wg.Add(1)
		go func(s *Subscriber, id int) {
			defer wg.Done()
			count := 0
			for {
				select {
				case <-s.Events():
					count++
				case <-s.Done():
					return
				}
			}
		}(sub, i)

		// Unsubscribe after a short delay.
		go func(s *Subscriber) {
			time.Sleep(50 * time.Millisecond)
			h.Unsubscribe(s)
		}(sub)
	}

	for i := range 100 {
		h.Publish(Event{Type: "concurrent", Data: i})
		time.Sleep(time.Millisecond)
	}

	wg.Wait()
}

func TestMarshalSSE(t *testing.T) {
	e := Event{Type: "health_change", Data: map[string]string{"id": "123"}}
	out := string(MarshalSSE(e))
	if !strings.HasPrefix(out, "event: health_change\n") {
		t.Fatalf("unexpected SSE prefix: %q", out)
	}
	if !strings.Contains(out, `"id":"123"`) {
		t.Fatalf("data not serialized: %q", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Fatalf("SSE must end with double newline: %q", out)
	}
}

func TestDoubleClose(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(4)
	h.Unsubscribe(sub)
	h.Unsubscribe(sub) // should not panic
}
