// Package backup_snapshots is the ent-backed implementation of the
// BackupSnapshot Store. The schema lives in apps/api/ent/schema/
// backupsnapshot.go; the domain model lives in
// internal/modules/backup_snapshots/contract.
package backup_snapshots

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entbackup "github.com/srapi/srapi/apps/api/ent/backupsnapshot"
	"github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
)

// ErrInvalidStore is returned when New is called with a nil client.
var ErrInvalidStore = errors.New("invalid backup snapshots ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateSnapshot) (contract.BackupSnapshot, error) {
	now := time.Now().UTC()
	started := input.StartedAt.UTC()
	if started.IsZero() {
		started = now
	}
	kind := input.Kind
	if kind == "" {
		kind = contract.KindScheduled
	}
	row, err := s.client.BackupSnapshot.Create().
		SetKind(kind).
		SetStartedAt(started).
		SetStatus(contract.StatusRunning).
		SetFilePath(input.FilePath).
		SetTriggeredByUserID(input.TriggeredByUserID).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.BackupSnapshot{}, err
	}
	return toContract(row), nil
}

func (s *Store) MarkComplete(ctx context.Context, id int, completedAt time.Time, sizeBytes int64, sha256 string) (contract.BackupSnapshot, error) {
	if id <= 0 {
		return contract.BackupSnapshot{}, ErrInvalidStore
	}
	row, err := s.client.BackupSnapshot.UpdateOneID(id).
		SetCompletedAt(completedAt.UTC()).
		SetSizeBytes(sizeBytes).
		SetSha256(sha256).
		SetStatus(contract.StatusSuccess).
		SetErrorMessage("").
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return contract.BackupSnapshot{}, mapNotFound(err)
	}
	return toContract(row), nil
}

func (s *Store) MarkFailed(ctx context.Context, id int, completedAt time.Time, errorMessage string) (contract.BackupSnapshot, error) {
	if id <= 0 {
		return contract.BackupSnapshot{}, ErrInvalidStore
	}
	if len(errorMessage) > 1024 {
		errorMessage = errorMessage[:1024]
	}
	row, err := s.client.BackupSnapshot.UpdateOneID(id).
		SetCompletedAt(completedAt.UTC()).
		SetStatus(contract.StatusFailed).
		SetErrorMessage(errorMessage).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return contract.BackupSnapshot{}, mapNotFound(err)
	}
	return toContract(row), nil
}

func (s *Store) MarkSuperseded(ctx context.Context, id int) (contract.BackupSnapshot, error) {
	if id <= 0 {
		return contract.BackupSnapshot{}, ErrInvalidStore
	}
	row, err := s.client.BackupSnapshot.UpdateOneID(id).
		SetStatus(contract.StatusSuperseded).
		SetFilePath("").
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return contract.BackupSnapshot{}, mapNotFound(err)
	}
	return toContract(row), nil
}

func (s *Store) List(ctx context.Context, opts contract.ListOptions) (contract.ListResult, error) {
	query := s.client.BackupSnapshot.Query()
	if opts.Status != "" {
		query = query.Where(entbackup.Status(opts.Status))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return contract.ListResult{}, err
	}
	listQuery := query.Order(ent.Desc(entbackup.FieldStartedAt), ent.Desc(entbackup.FieldID))
	if opts.Offset > 0 {
		listQuery = listQuery.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		listQuery = listQuery.Limit(opts.Limit)
	}
	rows, err := listQuery.All(ctx)
	if err != nil {
		return contract.ListResult{}, err
	}
	items := make([]contract.BackupSnapshot, 0, len(rows))
	for _, row := range rows {
		items = append(items, toContract(row))
	}
	return contract.ListResult{Items: items, Total: total}, nil
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.BackupSnapshot, error) {
	if id <= 0 {
		return contract.BackupSnapshot{}, ErrInvalidStore
	}
	row, err := s.client.BackupSnapshot.Get(ctx, id)
	if err != nil {
		return contract.BackupSnapshot{}, mapNotFound(err)
	}
	return toContract(row), nil
}

func (s *Store) Delete(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if err := s.client.BackupSnapshot.DeleteOneID(id).Exec(ctx); err != nil {
		return mapNotFound(err)
	}
	return nil
}

func toContract(row *ent.BackupSnapshot) contract.BackupSnapshot {
	snap := contract.BackupSnapshot{
		ID:                row.ID,
		Kind:              row.Kind,
		StartedAt:         row.StartedAt.UTC(),
		SizeBytes:         row.SizeBytes,
		SHA256:            row.Sha256,
		Status:            row.Status,
		FilePath:          row.FilePath,
		ErrorMessage:      row.ErrorMessage,
		TriggeredByUserID: row.TriggeredByUserID,
		CreatedAt:         row.CreatedAt.UTC(),
		UpdatedAt:         row.UpdatedAt.UTC(),
	}
	if row.CompletedAt != nil {
		completed := row.CompletedAt.UTC()
		snap.CompletedAt = &completed
	}
	return snap
}

func mapNotFound(err error) error {
	if ent.IsNotFound(err) {
		return contract.ErrNotFound
	}
	return err
}
