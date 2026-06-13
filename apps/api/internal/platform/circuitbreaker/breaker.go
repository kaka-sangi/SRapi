package circuitbreaker

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type State int

const (
	StateClosed   State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type Config struct {
	FailureThreshold    int
	SuccessThreshold    int
	Timeout             time.Duration
	MaxTimeout          time.Duration
	MaxHalfOpenRequests int
	OnStateChange       func(from, to State)
}

func (c *Config) defaults() {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.SuccessThreshold <= 0 {
		c.SuccessThreshold = 2
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxTimeout <= 0 {
		c.MaxTimeout = 5 * time.Minute
	}
	if c.MaxHalfOpenRequests <= 0 {
		c.MaxHalfOpenRequests = 1
	}
}

type Counts struct {
	Requests             int64
	TotalSuccesses       int64
	TotalFailures        int64
	ConsecutiveSuccesses int64
	ConsecutiveFailures  int64
}

type Breaker struct {
	cfg Config

	mu                   sync.Mutex
	state                State
	consecutiveSuccesses int64
	consecutiveFailures  int64
	halfOpenInflight     int
	openedAt             time.Time
	currentTimeout       time.Duration

	requests       atomic.Int64
	totalSuccesses atomic.Int64
	totalFailures  atomic.Int64
}

func New(cfg Config) *Breaker {
	cfg.defaults()
	return &Breaker{
		cfg:            cfg,
		state:          StateClosed,
		currentTimeout: cfg.Timeout,
	}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *Breaker) Counts() Counts {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Counts{
		Requests:             b.requests.Load(),
		TotalSuccesses:       b.totalSuccesses.Load(),
		TotalFailures:        b.totalFailures.Load(),
		ConsecutiveSuccesses: b.consecutiveSuccesses,
		ConsecutiveFailures:  b.consecutiveFailures,
	}
}

func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setState(StateClosed)
	b.consecutiveSuccesses = 0
	b.consecutiveFailures = 0
	b.halfOpenInflight = 0
	b.currentTimeout = b.cfg.Timeout
}

// Allow checks whether a request is permitted. On success it returns a done
// callback that the caller MUST invoke with the outcome of the operation.
func (b *Breaker) Allow() (done func(success bool), err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests.Add(1)

	switch b.state {
	case StateClosed:
		return b.recordResult, nil

	case StateOpen:
		if time.Since(b.openedAt) < b.currentTimeout {
			return nil, ErrCircuitOpen
		}
		b.setState(StateHalfOpen)
		b.consecutiveSuccesses = 0
		b.halfOpenInflight = 1
		return b.recordResult, nil

	case StateHalfOpen:
		if b.halfOpenInflight >= b.cfg.MaxHalfOpenRequests {
			return nil, ErrCircuitOpen
		}
		b.halfOpenInflight++
		return b.recordResult, nil
	}
	return nil, ErrCircuitOpen
}

func (b *Breaker) recordResult(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if success {
		b.totalSuccesses.Add(1)
		b.consecutiveSuccesses++
		b.consecutiveFailures = 0
		if b.state == StateHalfOpen {
			b.halfOpenInflight--
			if b.consecutiveSuccesses >= int64(b.cfg.SuccessThreshold) {
				b.setState(StateClosed)
				b.currentTimeout = b.cfg.Timeout
				b.halfOpenInflight = 0
			}
		}
		return
	}

	b.totalFailures.Add(1)
	b.consecutiveFailures++
	b.consecutiveSuccesses = 0

	switch b.state {
	case StateClosed:
		if b.consecutiveFailures >= int64(b.cfg.FailureThreshold) {
			b.trip()
		}
	case StateHalfOpen:
		b.halfOpenInflight--
		// Double the timeout on each re-trip (exponential backoff).
		b.currentTimeout = min(b.currentTimeout*2, b.cfg.MaxTimeout)
		b.trip()
	}
}

func (b *Breaker) trip() {
	b.setState(StateOpen)
	b.openedAt = time.Now()
	b.consecutiveSuccesses = 0
	b.halfOpenInflight = 0
}

func (b *Breaker) setState(to State) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	if b.cfg.OnStateChange != nil {
		b.cfg.OnStateChange(from, to)
	}
}
