package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrInvalidLimiter reports a nil or unusable Redis client.
	ErrInvalidLimiter = errors.New("invalid rate limiter")
	// ErrInvalidCheck reports a malformed positive-limit check.
	ErrInvalidCheck = errors.New("invalid rate limit check")
)

const (
	defaultKeyPrefix = "srapi:rl:"
	defaultWindow    = time.Minute
)

// Check describes one fixed-window limit dimension.
type Check struct {
	Name   string
	Key    string
	Limit  int
	Cost   int
	Window time.Duration
}

// Decision reports whether a set of checks was allowed.
type Decision struct {
	Allowed    bool
	Name       string
	Limit      int
	Used       int
	Cost       int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}

// ConcurrencyCheck describes one Redis ZSet-backed concurrent request limit.
type ConcurrencyCheck struct {
	Name  string
	Key   string
	Limit int
	TTL   time.Duration
}

// ConcurrencyLease identifies an acquired concurrent request slot.
type ConcurrencyLease struct {
	Name      string
	Key       string
	Token     string
	ExpiresAt time.Time
}

// Limiter enforces fixed-window counters in Redis.
type Limiter struct {
	client *redis.Client
	prefix string
}

// New creates a Redis-backed rate limiter.
func New(client *redis.Client) (*Limiter, error) {
	if client == nil {
		return nil, ErrInvalidLimiter
	}
	return &Limiter{client: client, prefix: defaultKeyPrefix}, nil
}

// Allow atomically evaluates every check and increments counters only when all pass.
func (l *Limiter) Allow(ctx context.Context, checks []Check, now time.Time) (Decision, error) {
	if l == nil || l.client == nil {
		return Decision{Allowed: true}, nil
	}
	normalized, err := normalizeChecks(checks)
	if err != nil {
		return Decision{}, err
	}
	if len(normalized) == 0 {
		return Decision{Allowed: true}, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	keys := make([]string, 0, len(normalized))
	args := make([]any, 0, len(normalized)*4)
	for _, check := range normalized {
		keys = append(keys, l.prefix+check.Key)
		args = append(args, check.Name, check.Limit, check.Cost, ttlMillis(check.Window))
	}
	result, err := multiLimitScript.Run(ctx, l.client, keys, args...).Slice()
	if err != nil {
		return Decision{}, err
	}
	decision, err := parseScriptDecision(result, now)
	if err != nil {
		return Decision{}, err
	}
	return decision, nil
}

// AcquireConcurrency atomically acquires one concurrent request slot.
func (l *Limiter) AcquireConcurrency(ctx context.Context, check ConcurrencyCheck, now time.Time) (ConcurrencyLease, Decision, error) {
	if l == nil || l.client == nil {
		return ConcurrencyLease{}, Decision{Allowed: true}, nil
	}
	normalized, err := normalizeConcurrencyCheck(check)
	if err != nil {
		return ConcurrencyLease{}, Decision{}, err
	}
	if normalized.Limit <= 0 {
		return ConcurrencyLease{}, Decision{Allowed: true}, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	token, err := randomToken()
	if err != nil {
		return ConcurrencyLease{}, Decision{}, err
	}
	key := l.prefix + normalized.Key
	ttl := ttlMillis(normalized.TTL)
	result, err := acquireConcurrencyScript.Run(ctx, l.client, []string{key}, normalized.Name, normalized.Limit, now.UnixMilli(), ttl, token).Slice()
	if err != nil {
		return ConcurrencyLease{}, Decision{}, err
	}
	decision, err := parseConcurrencyDecision(result, normalized.Name, now)
	if err != nil {
		return ConcurrencyLease{}, Decision{}, err
	}
	if !decision.Allowed {
		return ConcurrencyLease{}, decision, nil
	}
	return ConcurrencyLease{
		Name:      normalized.Name,
		Key:       normalized.Key,
		Token:     token,
		ExpiresAt: now.UTC().Add(normalized.TTL),
	}, decision, nil
}

// ReleaseConcurrency releases a previously acquired concurrent request slot.
func (l *Limiter) ReleaseConcurrency(ctx context.Context, lease ConcurrencyLease) error {
	if l == nil || l.client == nil || strings.TrimSpace(lease.Key) == "" || strings.TrimSpace(lease.Token) == "" {
		return nil
	}
	return releaseConcurrencyScript.Run(ctx, l.client, []string{l.prefix + lease.Key}, lease.Token).Err()
}

// Release refunds windowed-counter reservations previously made by Allow for the
// given checks (used to undo a reservation when a gateway failover attempt fails).
// Best-effort: only decrements counters that still exist (same window) and never
// drives a counter below zero.
func (l *Limiter) Release(ctx context.Context, checks []Check) error {
	if l == nil || l.client == nil {
		return nil
	}
	normalized, err := normalizeChecks(checks)
	if err != nil {
		return err
	}
	if len(normalized) == 0 {
		return nil
	}
	keys := make([]string, 0, len(normalized))
	args := make([]any, 0, len(normalized))
	for _, check := range normalized {
		keys = append(keys, l.prefix+check.Key)
		args = append(args, check.Cost)
	}
	return releaseLimitScript.Run(ctx, l.client, keys, args...).Err()
}

func normalizeChecks(checks []Check) ([]Check, error) {
	out := make([]Check, 0, len(checks))
	for _, check := range checks {
		if check.Limit <= 0 {
			continue
		}
		check.Name = strings.TrimSpace(check.Name)
		check.Key = strings.TrimSpace(check.Key)
		if check.Name == "" || check.Key == "" {
			return nil, ErrInvalidCheck
		}
		if check.Cost <= 0 {
			return nil, ErrInvalidCheck
		}
		if check.Window <= 0 {
			check.Window = defaultWindow
		}
		out = append(out, check)
	}
	return out, nil
}

func normalizeConcurrencyCheck(check ConcurrencyCheck) (ConcurrencyCheck, error) {
	check.Name = strings.TrimSpace(check.Name)
	check.Key = strings.TrimSpace(check.Key)
	if check.Limit <= 0 {
		return check, nil
	}
	if check.Name == "" || check.Key == "" {
		return ConcurrencyCheck{}, ErrInvalidCheck
	}
	if check.TTL <= 0 {
		check.TTL = defaultWindow
	}
	return check, nil
}

func parseScriptDecision(values []any, now time.Time) (Decision, error) {
	if len(values) == 0 {
		return Decision{}, fmt.Errorf("empty rate limit script result")
	}
	code := stringValue(values[0])
	switch code {
	case "ok":
		if len(values) < 3 {
			return Decision{}, fmt.Errorf("unexpected rate limit ok result: %v", values)
		}
		limit := intValue(values[1])
		used := intValue(values[2])
		return Decision{
			Allowed:   true,
			Limit:     limit,
			Used:      used,
			Remaining: max(0, limit-used),
		}, nil
	case "limited":
		if len(values) < 6 {
			return Decision{}, fmt.Errorf("unexpected rate limit limited result: %v", values)
		}
		limit := intValue(values[2])
		used := intValue(values[3])
		cost := intValue(values[4])
		retryAfter := time.Duration(max(1, intValue(values[5]))) * time.Millisecond
		return Decision{
			Allowed:    false,
			Name:       stringValue(values[1]),
			Limit:      limit,
			Used:       used,
			Cost:       cost,
			Remaining:  max(0, limit-used),
			RetryAfter: retryAfter,
			ResetAt:    now.UTC().Add(retryAfter),
		}, nil
	default:
		return Decision{}, fmt.Errorf("unexpected rate limit script code: %s", code)
	}
}

func parseConcurrencyDecision(values []any, fallbackName string, now time.Time) (Decision, error) {
	if len(values) == 0 {
		return Decision{}, fmt.Errorf("empty concurrency limit script result")
	}
	code := stringValue(values[0])
	switch code {
	case "ok":
		if len(values) < 4 {
			return Decision{}, fmt.Errorf("unexpected concurrency ok result: %v", values)
		}
		limit := intValue(values[2])
		used := intValue(values[3])
		return Decision{
			Allowed:   true,
			Name:      stringValueOr(values[1], fallbackName),
			Limit:     limit,
			Used:      used,
			Cost:      1,
			Remaining: max(0, limit-used),
		}, nil
	case "limited":
		if len(values) < 6 {
			return Decision{}, fmt.Errorf("unexpected concurrency limited result: %v", values)
		}
		limit := intValue(values[2])
		used := intValue(values[3])
		retryAfter := time.Duration(max(1, intValue(values[5]))) * time.Millisecond
		return Decision{
			Allowed:    false,
			Name:       stringValueOr(values[1], fallbackName),
			Limit:      limit,
			Used:       used,
			Cost:       1,
			Remaining:  max(0, limit-used),
			RetryAfter: retryAfter,
			ResetAt:    now.UTC().Add(retryAfter),
		}, nil
	default:
		return Decision{}, fmt.Errorf("unexpected concurrency limit script code: %s", code)
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func stringValueOr(value any, fallback string) string {
	if out := strings.TrimSpace(stringValue(value)); out != "" {
		return out
	}
	return fallback
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int64:
		return int(typed)
	case int:
		return typed
	case string:
		var parsed int
		_, _ = fmt.Sscan(typed, &parsed)
		return parsed
	case []byte:
		var parsed int
		_, _ = fmt.Sscan(string(typed), &parsed)
		return parsed
	default:
		return 0
	}
}

func ttlMillis(value time.Duration) int64 {
	if value <= 0 {
		value = defaultWindow
	}
	return int64(value / time.Millisecond)
}

func randomToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
