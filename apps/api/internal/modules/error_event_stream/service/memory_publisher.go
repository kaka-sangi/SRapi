// Package service implements the in-memory error event publisher used by the
// admin SSE endpoint. It is a faithful port of CLIProxyAPI's redisqueue
// SubscribeErrors fan-out — a process-local pub/sub with a bounded ring of
// recent events for late-joining subscribers and a 256-entry per-subscriber
// channel that drops on overflow (the lagging subscriber is closed, mirroring
// the CPA behaviour where a full channel evicts the subscriber).
package service

import (
	"context"
	"log/slog"
	"sync"

	contract "github.com/srapi/srapi/apps/api/internal/modules/error_event_stream/contract"
)

// Defaults mirror CLIProxyAPI's queue.go errorSubscriberBuffer and a sensible
// 1024-entry replay ring plus 64-subscriber cap (one per admin tab + a margin).
const (
	DefaultRingSize        = 1024
	DefaultSubscriberQueue = 256
	DefaultMaxSubscribers  = 64
)

// Config tunes the in-memory publisher. Zero values pick the defaults above.
type Config struct {
	// RingSize is the size of the recent-events ring buffer used for replay
	// when a subscriber sets SinceUnixMs.
	RingSize int
	// SubscriberQueueSize is the per-subscriber buffered channel capacity.
	SubscriberQueueSize int
	// MaxSubscribers caps the number of concurrent subscribers; further
	// Subscribe calls return contract.ErrTooManySubscribers.
	MaxSubscribers int
	// Logger receives the one-shot drop / eviction warnings. When nil the
	// default slog logger is used.
	Logger *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.RingSize <= 0 {
		c.RingSize = DefaultRingSize
	}
	if c.SubscriberQueueSize <= 0 {
		c.SubscriberQueueSize = DefaultSubscriberQueue
	}
	if c.MaxSubscribers <= 0 {
		c.MaxSubscribers = DefaultMaxSubscribers
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// MemoryPublisher implements contract.Publisher with an in-process ring buffer
// + fan-out. Safe for concurrent use.
type MemoryPublisher struct {
	cfg Config

	mu          sync.Mutex
	ring        []contract.Event
	ringHead    int
	ringLen     int
	subscribers map[uint64]*memorySubscriber
	nextSubID   uint64
}

// NewMemoryPublisher constructs a publisher with the given configuration.
func NewMemoryPublisher(cfg Config) *MemoryPublisher {
	cfg = cfg.withDefaults()
	return &MemoryPublisher{
		cfg:         cfg,
		ring:        make([]contract.Event, cfg.RingSize),
		subscribers: make(map[uint64]*memorySubscriber),
	}
}

// Publish broadcasts the event to every matching subscriber and records it in
// the ring buffer for late-joining subscribers.
func (p *MemoryPublisher) Publish(_ context.Context, ev contract.Event) error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	p.recordRingLocked(ev)
	// Snapshot the matching subscribers under the lock then deliver outside
	// the lock so a slow subscriber's drop / Close does not block other
	// subscribers' deliveries.
	type target struct {
		id  uint64
		sub *memorySubscriber
	}
	targets := make([]target, 0, len(p.subscribers))
	for id, sub := range p.subscribers {
		if sub.opts.Match(ev) {
			targets = append(targets, target{id: id, sub: sub})
		}
	}
	p.mu.Unlock()

	for _, t := range targets {
		select {
		case t.sub.ch <- ev:
		default:
			// Subscriber is lagging; evict per CPA queue.go semantics.
			p.evict(t.id, t.sub, "buffer_full")
		}
	}
	return nil
}

// Subscribe registers a new subscriber. When opts.SinceUnixMs > 0 the publisher
// replays matching ring buffer entries before returning.
func (p *MemoryPublisher) Subscribe(_ context.Context, opts contract.SubscribeOptions) (contract.Subscriber, error) {
	if p == nil {
		return nil, contract.ErrClosed
	}

	p.mu.Lock()
	if len(p.subscribers) >= p.cfg.MaxSubscribers {
		p.mu.Unlock()
		return nil, contract.ErrTooManySubscribers
	}
	p.nextSubID++
	id := p.nextSubID
	sub := &memorySubscriber{
		id:        id,
		opts:      opts,
		ch:        make(chan contract.Event, p.cfg.SubscriberQueueSize),
		publisher: p,
	}
	p.subscribers[id] = sub

	// Replay matching ring entries when a since cursor was requested.
	if opts.SinceUnixMs > 0 {
		p.replayLocked(sub)
	}
	p.mu.Unlock()
	return sub, nil
}

// SubscriberCount returns the number of currently active subscribers (mainly
// for tests + admin telemetry).
func (p *MemoryPublisher) SubscriberCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.subscribers)
}

// recordRingLocked appends ev to the ring. Caller must hold p.mu.
func (p *MemoryPublisher) recordRingLocked(ev contract.Event) {
	if len(p.ring) == 0 {
		return
	}
	if p.ringLen < len(p.ring) {
		idx := (p.ringHead + p.ringLen) % len(p.ring)
		p.ring[idx] = ev
		p.ringLen++
		return
	}
	// Full — overwrite the oldest slot and advance head.
	p.ring[p.ringHead] = ev
	p.ringHead = (p.ringHead + 1) % len(p.ring)
}

// replayLocked pushes matching ring entries to sub. Drops are silent during
// replay because the subscriber just opened — overflow at this point means the
// caller's SinceUnixMs window is wider than the buffer.
func (p *MemoryPublisher) replayLocked(sub *memorySubscriber) {
	for i := 0; i < p.ringLen; i++ {
		ev := p.ring[(p.ringHead+i)%len(p.ring)]
		if !sub.opts.Match(ev) {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			return
		}
	}
}

// evict closes a lagging subscriber's channel and removes it from the map.
// Mirrors CPA queue.go publishToSubscribers' default-branch behaviour.
func (p *MemoryPublisher) evict(id uint64, sub *memorySubscriber, reason string) {
	p.mu.Lock()
	if existing, ok := p.subscribers[id]; ok && existing == sub {
		delete(p.subscribers, id)
	} else {
		// Already removed (concurrent Close); nothing to do.
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	sub.closeOnce.Do(func() {
		close(sub.ch)
		sub.closed = true
	})
	if p.cfg.Logger != nil {
		// One-shot per drop is enough — operators tail the log to spot the
		// pattern and bump SubscriberQueueSize / fix the slow consumer.
		p.cfg.Logger.Warn("error_event_stream subscriber evicted",
			"subscriber_id", id,
			"reason", reason,
		)
	}
}

// Close closes every subscriber. Used by tests + graceful shutdown.
func (p *MemoryPublisher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	subs := make([]*memorySubscriber, 0, len(p.subscribers))
	for _, sub := range p.subscribers {
		subs = append(subs, sub)
	}
	p.subscribers = make(map[uint64]*memorySubscriber)
	p.mu.Unlock()

	for _, sub := range subs {
		sub.closeOnce.Do(func() {
			close(sub.ch)
			sub.closed = true
		})
	}
}

// memorySubscriber is the per-consumer handle returned by MemoryPublisher.
type memorySubscriber struct {
	id        uint64
	opts      contract.SubscribeOptions
	ch        chan contract.Event
	publisher *MemoryPublisher
	closeOnce sync.Once
	closed    bool
}

func (s *memorySubscriber) Receive() <-chan contract.Event {
	return s.ch
}

func (s *memorySubscriber) Close() error {
	if s == nil {
		return contract.ErrClosed
	}
	s.publisher.removeSubscriber(s)
	return nil
}

// removeSubscriber detaches sub from the publisher and closes its channel.
// Idempotent.
func (p *MemoryPublisher) removeSubscriber(sub *memorySubscriber) {
	p.mu.Lock()
	if existing, ok := p.subscribers[sub.id]; ok && existing == sub {
		delete(p.subscribers, sub.id)
	}
	p.mu.Unlock()

	sub.closeOnce.Do(func() {
		close(sub.ch)
		sub.closed = true
	})
}

// Ensure interface compliance.
var _ contract.Publisher = (*MemoryPublisher)(nil)
