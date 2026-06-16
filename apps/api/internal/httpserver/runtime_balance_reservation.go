package httpserver

import (
	"context"
	"time"
)

// balanceReservationStore is the small consumer interface the gateway uses
// to atomically reserve / release per-user balance. The concrete Redis impl
// lives in apps/api/internal/persistence/redisstore/balancereservation; tests
// can substitute a fake.
//
// The interface stays inside the httpserver package because (a) only this
// package consumes it, and (b) the architecture rule forbids httpserver from
// importing module services — keeping the contract local keeps the wiring
// clean.
type balanceReservationStore interface {
	// Reserve atomically checks whether balance - already_reserved >= amount
	// and, if so, records a new reservation under idempotencyKey. Returns
	// ok=false when the gate should deny the request. Idempotent on the same
	// key (so retried gates don't double-reserve).
	Reserve(ctx context.Context, userID int, idempotencyKey, balance, amount string, ttl time.Duration) (bool, error)
	// Release removes a reservation. Idempotent — silently succeeds if the
	// key was never reserved (or was already released).
	Release(ctx context.Context, userID int, idempotencyKey string) error
}
