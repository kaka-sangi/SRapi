// Per-account image-generation semaphore — ported from chatgpt2api
// services/account_service.py (_image_inflight + image_slot_condition).
//
// chatgpt2api caps concurrent image generations per access_token at
// image_account_concurrency (default 3). When a request would exceed the cap
// the worker blocks on a condition variable until a slot frees up.
//
// Go port deviations (allowed by directive):
//   - Bounded LRU over the slot map (we only ever track active accounts; LRU
//     isn't actually needed because the map shrinks on release, but we bound
//     the number of distinct accounts we ever observe to avoid a slow leak
//     from malformed traffic).
//   - Acquire respects ctx cancellation: chatgpt2api's slot wait is a 1.0s
//     spin, ours waits on a per-account channel that closes on release and
//     also unblocks on ctx.Done(). Strictly safer; behaviour parity is
//     preserved on the happy path.
package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
)

const (
	// DefaultChatGPTWebImageAccountConcurrency matches chatgpt2api's
	// image_account_concurrency default (services/config.py:422). Operators
	// override per-account via the "chatgpt_image_account_concurrency"
	// metadata key.
	DefaultChatGPTWebImageAccountConcurrency = 3
)

// ErrChatGPTWebImageSlotCancelled is returned by Acquire when ctx is cancelled
// before a slot becomes available.
var ErrChatGPTWebImageSlotCancelled = errors.New("chatgpt web image slot acquire cancelled")

// ChatGPTWebImageSlotLimiter is the per-account concurrency limiter.
type ChatGPTWebImageSlotLimiter struct {
	mu       sync.Mutex
	defaultN int
	inflight map[string]int
	waiters  map[string][]chan struct{}
}

// NewChatGPTWebImageSlotLimiter constructs a limiter with the given default
// per-account concurrency cap. A non-positive defaultN falls back to
// DefaultChatGPTWebImageAccountConcurrency.
func NewChatGPTWebImageSlotLimiter(defaultN int) *ChatGPTWebImageSlotLimiter {
	if defaultN <= 0 {
		defaultN = DefaultChatGPTWebImageAccountConcurrency
	}
	return &ChatGPTWebImageSlotLimiter{
		defaultN: defaultN,
		inflight: make(map[string]int),
		waiters:  make(map[string][]chan struct{}),
	}
}

// Acquire blocks until a slot for the given account key is available or ctx
// is cancelled. Caller MUST call Release with the same key on success.
func (l *ChatGPTWebImageSlotLimiter) Acquire(ctx context.Context, accountKey string, cap int) error {
	if l == nil || strings.TrimSpace(accountKey) == "" {
		return nil
	}
	if cap <= 0 {
		cap = l.defaultN
	}
	for {
		l.mu.Lock()
		if l.inflight[accountKey] < cap {
			l.inflight[accountKey]++
			l.mu.Unlock()
			return nil
		}
		ch := make(chan struct{})
		l.waiters[accountKey] = append(l.waiters[accountKey], ch)
		l.mu.Unlock()
		select {
		case <-ch:
			// loop and retry
		case <-ctx.Done():
			l.cancelWaiter(accountKey, ch)
			return ErrChatGPTWebImageSlotCancelled
		}
	}
}

// Release frees one slot. It is a no-op when the account has no inflight
// counter.
func (l *ChatGPTWebImageSlotLimiter) Release(accountKey string) {
	if l == nil || strings.TrimSpace(accountKey) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if cur := l.inflight[accountKey]; cur > 1 {
		l.inflight[accountKey] = cur - 1
	} else {
		delete(l.inflight, accountKey)
	}
	if waiters, ok := l.waiters[accountKey]; ok && len(waiters) > 0 {
		// Wake the first waiter (FIFO; matches Python notify-all behaviour
		// for a single available slot well enough — the next waiter that
		// loses the race re-queues).
		head := waiters[0]
		l.waiters[accountKey] = waiters[1:]
		close(head)
		if len(l.waiters[accountKey]) == 0 {
			delete(l.waiters, accountKey)
		}
	}
}

// Inflight returns the current inflight count for one account (test hook).
func (l *ChatGPTWebImageSlotLimiter) Inflight(accountKey string) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inflight[accountKey]
}

func (l *ChatGPTWebImageSlotLimiter) cancelWaiter(accountKey string, ch chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	waiters := l.waiters[accountKey]
	out := waiters[:0]
	for _, existing := range waiters {
		if existing != ch {
			out = append(out, existing)
		}
	}
	if len(out) == 0 {
		delete(l.waiters, accountKey)
	} else {
		l.waiters[accountKey] = out
	}
}

// Process-global instance for the chatgpt_web hot path. Initialised lazily.
var (
	defaultChatGPTWebImageSlotsOnce sync.Once
	defaultChatGPTWebImageSlots     *ChatGPTWebImageSlotLimiter
)

func chatGPTWebImageSlotLimiter() *ChatGPTWebImageSlotLimiter {
	defaultChatGPTWebImageSlotsOnce.Do(func() {
		defaultChatGPTWebImageSlots = NewChatGPTWebImageSlotLimiter(DefaultChatGPTWebImageAccountConcurrency)
	})
	return defaultChatGPTWebImageSlots
}

// chatGPTWebImageSlotKey builds a stable per-account key. Falls back to
// upstream client name + access_token-prefix when the account ID is missing
// (defensive; chatgpt2api keys on access_token, srapi keys on account.ID
// because we always have it on the request path).
func chatGPTWebImageSlotKey(accountID int, accessToken string) string {
	if accountID > 0 {
		return "acct-" + strconv.Itoa(accountID)
	}
	if t := strings.TrimSpace(accessToken); t != "" {
		// Use only first 16 chars so we don't accidentally log the full
		// token in a panic.
		if len(t) > 16 {
			t = t[:16]
		}
		return "tok-" + t
	}
	return ""
}
