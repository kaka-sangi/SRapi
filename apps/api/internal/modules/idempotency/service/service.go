package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
)

// ErrInvalidInput is returned for malformed begin/complete input.
var ErrInvalidInput = errors.New("invalid idempotency input")

const (
	defaultLockTTL   = 5 * time.Minute
	defaultRecordTTL = 24 * time.Hour
)

// Outcome is the decision returned by Begin.
type Outcome int

const (
	// OutcomeProceed: the caller owns execution and must call Complete after.
	OutcomeProceed Outcome = iota
	// OutcomeReplay: a completed snapshot exists and should be returned verbatim.
	OutcomeReplay
	// OutcomeInFlight: an identical request is still executing (or completed
	// without a replayable snapshot) — the caller should return 409.
	OutcomeInFlight
	// OutcomeMismatch: the key was reused with a different request body (return 422).
	OutcomeMismatch
)

type BeginResult struct {
	Outcome Outcome
	Record  contract.Record
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store     contract.Store
	clock     Clock
	lockTTL   time.Duration
	recordTTL time.Duration
}

func New(store contract.Store, clock Clock, lockTTL, recordTTL time.Duration) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = systemClock{}
	}
	if lockTTL <= 0 {
		lockTTL = defaultLockTTL
	}
	if recordTTL <= 0 {
		recordTTL = defaultRecordTTL
	}
	return &Service{store: store, clock: clock, lockTTL: lockTTL, recordTTL: recordTTL}, nil
}

// Begin claims execution for an idempotent request, or reports that the request
// should replay a prior response, wait on an in-flight twin, or be rejected as a
// key reuse with a different body.
func (s *Service) Begin(ctx context.Context, key, method, path, requestHash string) (BeginResult, error) {
	key = strings.TrimSpace(key)
	method = strings.TrimSpace(method)
	path = strings.TrimSpace(path)
	requestHash = strings.TrimSpace(requestHash)
	if key == "" || method == "" || path == "" || requestHash == "" {
		return BeginResult{}, ErrInvalidInput
	}
	now := s.clock.Now()
	input := contract.BeginInput{
		Key:         key,
		Method:      method,
		Path:        path,
		RequestHash: requestHash,
		LockedUntil: now.Add(s.lockTTL),
		ExpiresAt:   now.Add(s.recordTTL),
		Now:         now,
	}
	inserted, existing, err := s.store.InsertOrGet(ctx, input)
	if err != nil {
		return BeginResult{}, err
	}
	if inserted {
		return BeginResult{Outcome: OutcomeProceed, Record: existing}, nil
	}
	if existing.RequestHash != requestHash {
		return BeginResult{Outcome: OutcomeMismatch, Record: existing}, nil
	}
	if existing.Status == contract.StatusCompleted {
		if existing.Snapshot != nil {
			return BeginResult{Outcome: OutcomeReplay, Record: existing}, nil
		}
		// Completed without a replayable snapshot (e.g. a streamed first response):
		// never re-execute, but there is nothing to replay.
		return BeginResult{Outcome: OutcomeInFlight, Record: existing}, nil
	}
	// In progress: a live twin holds the lock, unless the lock is stale (the prior
	// owner crashed) in which case this caller re-acquires it and proceeds.
	if existing.LockedUntil != nil && existing.LockedUntil.After(now) {
		return BeginResult{Outcome: OutcomeInFlight, Record: existing}, nil
	}
	reacquired, err := s.store.Reacquire(ctx, input)
	if err != nil {
		if errors.Is(err, contract.ErrNotFound) {
			return BeginResult{Outcome: OutcomeInFlight, Record: existing}, nil
		}
		return BeginResult{}, err
	}
	return BeginResult{Outcome: OutcomeProceed, Record: reacquired}, nil
}

// Complete stores the response snapshot and marks the record completed. A nil
// snapshot marks completion without a replayable body (e.g. an oversize or
// non-2xx response).
func (s *Service) Complete(ctx context.Context, key, method, path string, snapshot *contract.Snapshot) error {
	key = strings.TrimSpace(key)
	method = strings.TrimSpace(method)
	path = strings.TrimSpace(path)
	if key == "" || method == "" || path == "" {
		return ErrInvalidInput
	}
	_, err := s.store.Complete(ctx, key, method, path, snapshot, s.clock.Now())
	return err
}
