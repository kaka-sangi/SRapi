// Package balancereservation persists in-flight gateway-cost reservations in
// Redis so concurrent gateway requests can be admitted atomically without
// over-spending a user's balance.
//
// Design: each user has a hash at "srapi:balance_reservation:{user_id}" whose
// fields are unique idempotency keys (request_id + ":attempt:" + N — already
// stamped on the usage_log) and whose values are the reserved amounts in
// 1e8-scaled integer "ticks" (matching the codebase's 8-decimal money scale).
// A Lua script atomically (a) sums the current reservations, (b) verifies
// balance - sum >= amount, (c) writes the new field, all in one Redis round
// trip. Release deletes the field. A TTL on the hash key bounds reservation
// leaks if release is ever missed.
//
// Why Lua and not WATCH/MULTI: WATCH races under contention (the gate is the
// contention point we're trying to close). Lua scripts are atomic on Redis
// even under heavy load. This mirrors the realtime/scripts.go slot pattern.
package balancereservation

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrInvalidInput is returned for malformed inputs (negative amount, blank key,
// etc.). The handler should treat this as a programmer bug, not a user error.
var ErrInvalidInput = errors.New("balancereservation: invalid input")

// Money values flow through the system as decimal strings with 8 fractional
// digits (e.g. "1.50000000"). Reservation math runs in 1e8-scaled integer
// "ticks" so neither Lua nor go's decimal parsing introduces drift.
const moneyScale = 8

var moneyScaleFactor = func() *big.Int {
	out := big.NewInt(10)
	return out.Exp(out, big.NewInt(moneyScale), nil)
}()

// reserveScript atomically (a) returns success if the idempotency key already
// reserved, (b) computes balance - sum(existing reservations), (c) if it's
// >= the requested amount, writes the new field and (re)sets the TTL.
//
// KEYS[1] = hash key ("srapi:balance_reservation:{user_id}")
// ARGV[1] = idempotency key
// ARGV[2] = balance in 1e8 ticks (int as string)
// ARGV[3] = amount in 1e8 ticks (int as string)
// ARGV[4] = ttl seconds (int as string)
// Returns: 1 = reserved (or idempotent re-reserve), 0 = insufficient available.
var reserveScript = redis.NewScript(`
local hkey = KEYS[1]
local idkey = ARGV[1]
local balance = tonumber(ARGV[2])
local amount = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local existing = redis.call("HGET", hkey, idkey)
if existing then
  -- Idempotent: same caller, same attempt — already reserved. Refresh TTL
  -- so a slow upstream doesn't expire the reservation under it.
  redis.call("EXPIRE", hkey, ttl)
  return 1
end

local reserved = 0
local fields = redis.call("HVALS", hkey)
for _, v in ipairs(fields) do
  reserved = reserved + tonumber(v)
end

if balance - reserved < amount then
  return 0
end

redis.call("HSET", hkey, idkey, ARGV[3])
redis.call("EXPIRE", hkey, ttl)
return 1
`)

// releaseScript removes one reservation. Idempotent — silently succeeds if
// the field was already gone (so a duplicate release on retry doesn't double-
// count).
//
// KEYS[1] = hash key
// ARGV[1] = idempotency key
// Returns: amount that was reserved (or "0" if nothing).
var releaseScript = redis.NewScript(`
local existing = redis.call("HGET", KEYS[1], ARGV[1])
if not existing then
  return "0"
end
redis.call("HDEL", KEYS[1], ARGV[1])
return existing
`)

// reservedTotalScript returns the current sum of reservations on a user, for
// diagnostics / admin tooling. Not used on the hot path (the gate computes
// the sum inside reserveScript atomically).
var reservedTotalScript = redis.NewScript(`
local fields = redis.call("HVALS", KEYS[1])
local total = 0
for _, v in ipairs(fields) do
  total = total + tonumber(v)
end
return tostring(total)
`)

// Store is the production Redis-backed reservation store.
type Store struct {
	client     redis.UniversalClient
	keyPrefix  string
	defaultTTL time.Duration
}

// New wires a Redis client into the reservation store.
//
// keyPrefix is prepended to every Redis key so multi-tenant deploys can share
// one Redis instance (default: "srapi:balance_reservation").
//
// defaultTTL bounds the lifetime of a leaked reservation if release is somehow
// missed (e.g. a panic between dispatch and recordGatewayUsage). 10 minutes
// is generous enough for any reasonable upstream timeout and short enough that
// a permanently lost reservation doesn't poison the user's available balance
// for long.
func New(client redis.UniversalClient, keyPrefix string, defaultTTL time.Duration) *Store {
	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = "srapi:balance_reservation"
	}
	if defaultTTL <= 0 {
		defaultTTL = 10 * time.Minute
	}
	return &Store{client: client, keyPrefix: keyPrefix, defaultTTL: defaultTTL}
}

// Reserve atomically checks whether balance - already_reserved >= amount and,
// if so, adds amount to the user's reservation hash under idempotencyKey.
// Re-reserving with the same key is a no-op success (returns true) so a
// retried request doesn't deny itself.
//
// balance and amount are decimal strings with up to 8 fractional digits.
// Returns ok=false when the gate should deny the request.
func (s *Store) Reserve(
	ctx context.Context,
	userID int,
	idempotencyKey, balance, amount string,
	ttl time.Duration,
) (bool, error) {
	if s == nil || s.client == nil {
		return false, ErrInvalidInput
	}
	if userID <= 0 || strings.TrimSpace(idempotencyKey) == "" {
		return false, ErrInvalidInput
	}
	balanceTicks, err := decimalToTicks(balance)
	if err != nil {
		return false, fmt.Errorf("parse balance: %w", err)
	}
	amountTicks, err := decimalToTicks(amount)
	if err != nil {
		return false, fmt.Errorf("parse amount: %w", err)
	}
	// Zero or negative balance can never cover anything; reject without a
	// Redis round trip. The outer gate already checks this, but defending in
	// depth avoids a wasted ticks-arithmetic round trip too.
	if balanceTicks.Sign() <= 0 {
		return false, nil
	}
	// A zero-cost request can be admitted without taking a reservation slot —
	// e.g. a cached / sub-cent request. Avoids burning a hash field per call.
	if amountTicks.Sign() <= 0 {
		return true, nil
	}
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	hkey := s.hashKey(userID)
	res, err := reserveScript.Run(
		ctx,
		s.client,
		[]string{hkey},
		idempotencyKey,
		balanceTicks.String(),
		amountTicks.String(),
		int64(ttl.Seconds()),
	).Result()
	if err != nil {
		return false, err
	}
	// The Lua script returns an integer 1/0 — go-redis surfaces this as int64.
	switch v := res.(type) {
	case int64:
		return v == 1, nil
	case string:
		return v == "1", nil
	default:
		return false, fmt.Errorf("unexpected reserve reply type %T", res)
	}
}

// Release removes the reservation associated with idempotencyKey. Idempotent
// — releasing a key that was never reserved (or already released) returns nil
// without error.
func (s *Store) Release(ctx context.Context, userID int, idempotencyKey string) error {
	if s == nil || s.client == nil {
		return ErrInvalidInput
	}
	if userID <= 0 || strings.TrimSpace(idempotencyKey) == "" {
		return ErrInvalidInput
	}
	_, err := releaseScript.Run(
		ctx,
		s.client,
		[]string{s.hashKey(userID)},
		idempotencyKey,
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}

// Reserved reports the current reservation total for a user as an 8-decimal
// string (e.g. "0.50000000"). Used by admin diagnostics; not on the hot path.
func (s *Store) Reserved(ctx context.Context, userID int) (string, error) {
	if s == nil || s.client == nil {
		return "0.00000000", ErrInvalidInput
	}
	if userID <= 0 {
		return "0.00000000", ErrInvalidInput
	}
	res, err := reservedTotalScript.Run(ctx, s.client, []string{s.hashKey(userID)}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "0.00000000", nil
		}
		return "0.00000000", err
	}
	totalStr := fmt.Sprint(res)
	ticks, ok := new(big.Int).SetString(totalStr, 10)
	if !ok {
		return "0.00000000", fmt.Errorf("invalid reservation total %q", totalStr)
	}
	return ticksToDecimal(ticks), nil
}

func (s *Store) hashKey(userID int) string {
	return fmt.Sprintf("%s:%d", s.keyPrefix, userID)
}

// decimalToTicks parses a decimal string like "1.50000000" into 1e8-scaled
// integer ticks (here 150000000). Lossless for any input with <= 8 fractional
// digits — anything past 8 places is truncated (we never charge in fractions
// of a tick).
func decimalToTicks(value string) (*big.Int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return big.NewInt(0), nil
	}
	negative := false
	if strings.HasPrefix(trimmed, "-") {
		negative = true
		trimmed = trimmed[1:]
	}
	dotIdx := strings.IndexByte(trimmed, '.')
	var whole, fractional string
	if dotIdx < 0 {
		whole = trimmed
	} else {
		whole = trimmed[:dotIdx]
		fractional = trimmed[dotIdx+1:]
	}
	if whole == "" {
		whole = "0"
	}
	// Right-pad or truncate the fractional part to exactly moneyScale digits.
	if len(fractional) > moneyScale {
		fractional = fractional[:moneyScale]
	} else if len(fractional) < moneyScale {
		fractional += strings.Repeat("0", moneyScale-len(fractional))
	}
	combined := whole + fractional
	// Strip leading zeros — but keep one digit so SetString never gets "".
	combined = strings.TrimLeft(combined, "0")
	if combined == "" {
		combined = "0"
	}
	ticks, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", value)
	}
	if negative {
		ticks.Neg(ticks)
	}
	return ticks, nil
}

// ticksToDecimal renders 1e8-scaled ticks back into the codebase's standard
// 8-decimal-place string format (e.g. 150000000 → "1.50000000").
func ticksToDecimal(ticks *big.Int) string {
	negative := ticks.Sign() < 0
	abs := new(big.Int).Abs(ticks)
	whole, frac := new(big.Int).QuoRem(abs, moneyScaleFactor, new(big.Int))
	fracStr := frac.String()
	if len(fracStr) < moneyScale {
		fracStr = strings.Repeat("0", moneyScale-len(fracStr)) + fracStr
	}
	out := whole.String() + "." + fracStr
	if negative {
		out = "-" + out
	}
	return out
}
