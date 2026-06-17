// Package memory is the in-memory implementation of the BackupSnapshot
// Store. Used by service tests and the no-DB runtime so the rest of the
// backup-history stack can be exercised without postgres.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
)

type Store struct {
	mu    sync.Mutex
	rows  map[int]contract.BackupSnapshot
	seq   int
	clock func() time.Time
}

func New() *Store {
	return &Store{
		rows:  make(map[int]contract.BackupSnapshot),
		clock: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) Create(_ context.Context, input contract.CreateSnapshot) (contract.BackupSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	now := s.now()
	started := input.StartedAt
	if started.IsZero() {
		started = now
	}
	kind := input.Kind
	if kind == "" {
		kind = contract.KindScheduled
	}
	row := contract.BackupSnapshot{
		ID:                s.seq,
		Kind:              kind,
		StartedAt:         started.UTC(),
		Status:            contract.StatusRunning,
		FilePath:          input.FilePath,
		TriggeredByUserID: input.TriggeredByUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *Store) MarkComplete(_ context.Context, id int, completedAt time.Time, sizeBytes int64, sha256 string) (contract.BackupSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return contract.BackupSnapshot{}, contract.ErrNotFound
	}
	completed := completedAt.UTC()
	row.CompletedAt = &completed
	row.SizeBytes = sizeBytes
	row.SHA256 = sha256
	row.Status = contract.StatusSuccess
	row.ErrorMessage = ""
	row.UpdatedAt = s.now()
	s.rows[id] = row
	return row, nil
}

func (s *Store) MarkFailed(_ context.Context, id int, completedAt time.Time, errorMessage string) (contract.BackupSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return contract.BackupSnapshot{}, contract.ErrNotFound
	}
	completed := completedAt.UTC()
	row.CompletedAt = &completed
	row.Status = contract.StatusFailed
	row.ErrorMessage = errorMessage
	row.UpdatedAt = s.now()
	s.rows[id] = row
	return row, nil
}

func (s *Store) MarkSuperseded(_ context.Context, id int) (contract.BackupSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return contract.BackupSnapshot{}, contract.ErrNotFound
	}
	row.Status = contract.StatusSuperseded
	row.FilePath = ""
	row.UpdatedAt = s.now()
	s.rows[id] = row
	return row, nil
}

func (s *Store) List(_ context.Context, opts contract.ListOptions) (contract.ListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]contract.BackupSnapshot, 0, len(s.rows))
	for _, row := range s.rows {
		if opts.Status != "" && row.Status != opts.Status {
			continue
		}
		all = append(all, row)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].StartedAt.Equal(all[j].StartedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].StartedAt.After(all[j].StartedAt)
	})
	total := len(all)
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Offset >= total {
		return contract.ListResult{Items: []contract.BackupSnapshot{}, Total: total}, nil
	}
	end := total
	if opts.Limit > 0 && opts.Offset+opts.Limit < end {
		end = opts.Offset + opts.Limit
	}
	return contract.ListResult{Items: all[opts.Offset:end], Total: total}, nil
}

func (s *Store) FindByID(_ context.Context, id int) (contract.BackupSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return contract.BackupSnapshot{}, contract.ErrNotFound
	}
	return row, nil
}

func (s *Store) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}
