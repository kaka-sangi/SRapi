package memory

import (
	"context"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
)

// Store is an in-process idempotency store for tests and memory-storage mode.
type Store struct {
	mu      sync.Mutex
	records map[string]contract.Record
}

func New() *Store {
	return &Store{records: map[string]contract.Record{}}
}

func recordID(key, method, path string) string {
	return key + "\x00" + method + "\x00" + path
}

func (s *Store) InsertOrGet(_ context.Context, input contract.BeginInput) (bool, contract.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := recordID(input.Key, input.Method, input.Path)
	if existing, ok := s.records[id]; ok {
		return false, cloneRecord(existing), nil
	}
	lockedUntil := input.LockedUntil
	record := contract.Record{
		Key:         input.Key,
		Method:      input.Method,
		Path:        input.Path,
		RequestHash: input.RequestHash,
		Status:      contract.StatusInProgress,
		LockedUntil: &lockedUntil,
		ExpiresAt:   input.ExpiresAt,
		CreatedAt:   input.Now,
		UpdatedAt:   input.Now,
	}
	s.records[id] = record
	return true, cloneRecord(record), nil
}

func (s *Store) Reacquire(_ context.Context, input contract.BeginInput) (contract.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := recordID(input.Key, input.Method, input.Path)
	record, ok := s.records[id]
	if !ok {
		return contract.Record{}, contract.ErrNotFound
	}
	lockedUntil := input.LockedUntil
	record.RequestHash = input.RequestHash
	record.Status = contract.StatusInProgress
	record.Snapshot = nil
	record.LockedUntil = &lockedUntil
	record.ExpiresAt = input.ExpiresAt
	record.UpdatedAt = input.Now
	s.records[id] = record
	return cloneRecord(record), nil
}

func (s *Store) Complete(_ context.Context, key, method, path string, snapshot *contract.Snapshot, now time.Time) (contract.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := recordID(key, method, path)
	record, ok := s.records[id]
	if !ok {
		return contract.Record{}, contract.ErrNotFound
	}
	record.Status = contract.StatusCompleted
	record.Snapshot = cloneSnapshot(snapshot)
	record.LockedUntil = nil
	record.UpdatedAt = now
	s.records[id] = record
	return cloneRecord(record), nil
}

func (s *Store) DeleteExpired(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for id, record := range s.records {
		if record.ExpiresAt.Before(before) {
			delete(s.records, id)
			deleted++
		}
	}
	return deleted, nil
}

func cloneRecord(record contract.Record) contract.Record {
	if record.LockedUntil != nil {
		lockedUntil := *record.LockedUntil
		record.LockedUntil = &lockedUntil
	}
	record.Snapshot = cloneSnapshot(record.Snapshot)
	return record
}

func cloneSnapshot(snapshot *contract.Snapshot) *contract.Snapshot {
	if snapshot == nil {
		return nil
	}
	cloned := contract.Snapshot{StatusCode: snapshot.StatusCode}
	if len(snapshot.Body) > 0 {
		cloned.Body = append([]byte(nil), snapshot.Body...)
	}
	if len(snapshot.Headers) > 0 {
		cloned.Headers = make(map[string][]string, len(snapshot.Headers))
		for key, values := range snapshot.Headers {
			cloned.Headers[key] = append([]string(nil), values...)
		}
	}
	return &cloned
}
