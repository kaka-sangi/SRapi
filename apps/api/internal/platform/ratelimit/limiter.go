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

// FailureMode controls how the limiter behaves when Redis errors out
// during a check. Operators pick the trade-off:
//
//   - FailOpen (default): allow the request through. Trades quota
//     fidelity for availability. Matches sub2api's default and the
//     common production posture — Redis unavailability should not
//     take down the gateway.
//   - FailClose: reject the request. Trades availability for quota
//     fidelity. Correct for compliance-sensitive deployments where
//     unbounded traffic during Redis outages is unacceptable.
type FailureMode int

const (
	// FailOpen allows the request through on Redis errors. Default.
	FailOpen FailureMode = iota
	// FailClose rejects the request on Redis errors.
	FailClose
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
	client      *redis.Client
	prefix      string
	failureMode FailureMode
}

// New creates a Redis-backed rate limiter. Default failure mode is
// FailOpen — see FailureMode for the rationale.
func New(client *redis.Client) (*Limiter, error) {
	if client == nil {
		return nil, ErrInvalidLimiter
	}
	return &Limiter{client: client, prefix: defaultKeyPrefix, failureMode: FailOpen}, nil
}

// SetFailureMode reconfigures the limiter's behavior on Redis errors.
// Safe to call at any time; subsequent Allow / AcquireConcurrency
// calls honor the new mode.
func (l *Limiter) SetFailureMode(mode FailureMode) {
	if l == nil {
		return
	}
	l.failureMode = mode
}

// FailureMode reports the limiter's current configured mode.
func (l *Limiter) FailureMode() FailureMode {
	if l == nil {
		return FailOpen
	}
	return l.failureMode
}

// failureModeAllowed returns the Decision to surface (and the error
// to propagate) when a Redis call errored out. FailOpen swallows the
// error and returns Allowed=true; FailClose surfaces the error so
// the caller rejects the request. Concurrency call sites use the
// same convention by treating a returned error as "rejected".
func (l *Limiter) failureModeAllowed(err error) (Decision, error) {
	if l == nil || l.failureMode == FailOpen {
		return Decision{Allowed: true}, nil
	}
	return Decision{}, err
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
		// Distinguish Redis transport errors (eligible for the
		// configured failure mode) from script logic errors
		// (always propagate — those indicate a code bug).
		if isRedisAvailabilityError(err) {
			return l.failureModeAllowed(err)
		}
		return Decision{}, err
	}
	decision, err := parseScriptDecision(result, now)
	if err != nil {
		return Decision{}, err
	}
	return decision, nil
}

// isRedisAvailabilityError reports whether err looks like a Redis
// availability failure (connection refused, EOF, timeout, redis
// not loaded) — the class the failure-mode setting governs. Script
// logic errors and our own parse errors are NOT availability
// failures and stay opaque to the caller.
func isRedisAvailabilityError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, redis.ErrClosed) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		// Context cancellation is the caller's signal; do not
		// fail-open through it. Treat as a non-availability
		// error.
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"broken pipe",
		"network is unreachable",
		"no route to host",
		"eof",
		"dial tcp",
		"loading the dataset",
		"masterdown",
		"clusterdown",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
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
		if isRedisAvailabilityError(err) {
			decision, modeErr := l.failureModeAllowed(err)
			return ConcurrencyLease{}, decision, modeErr
		}
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
