package eventsub

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Subscriber struct {
	ch     chan Event
	done   chan struct{}
	closed bool
	mu     sync.Mutex
}

func (s *Subscriber) Events() <-chan Event { return s.ch }
func (s *Subscriber) Done() <-chan struct{} { return s.done }

func (s *Subscriber) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.done)
}

func (s *Subscriber) send(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- e:
	default:
		// drop if subscriber is slow
	}
}

type Hub struct {
	mu   sync.RWMutex
	subs map[*Subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[*Subscriber]struct{})}
}

func (h *Hub) Subscribe(bufSize int) *Subscriber {
	if bufSize <= 0 {
		bufSize = 64
	}
	s := &Subscriber{
		ch:   make(chan Event, bufSize),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

func (h *Hub) Unsubscribe(s *Subscriber) {
	s.Close()
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

func (h *Hub) Publish(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs {
		s.send(e)
	}
}

func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

func MarshalSSE(e Event) []byte {
	data, _ := json.Marshal(e.Data)
	buf := make([]byte, 0, len("event: ")+len(e.Type)+len("\ndata: ")+len(data)+len("\n\n"))
	buf = append(buf, "event: "...)
	buf = append(buf, e.Type...)
	buf = append(buf, "\ndata: "...)
	buf = append(buf, data...)
	buf = append(buf, "\n\n"...)
	return buf
}
