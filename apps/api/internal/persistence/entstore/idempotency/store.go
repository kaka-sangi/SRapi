package idempotency

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entidem "github.com/srapi/srapi/apps/api/ent/idempotencyrecord"
	"github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
)

// ErrInvalidStore is returned when the store is constructed without an ent client.
var ErrInvalidStore = errors.New("invalid idempotency ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) InsertOrGet(ctx context.Context, input contract.BeginInput) (bool, contract.Record, error) {
	row, err := s.client.IdempotencyRecord.Create().
		SetIdempotencyKey(input.Key).
		SetMethod(input.Method).
		SetPath(input.Path).
		SetRequestHash(input.RequestHash).
		SetStatus(string(contract.StatusInProgress)).
		SetLockedUntil(input.LockedUntil).
		SetExpiresAt(input.ExpiresAt).
		SetCreatedAt(input.Now).
		SetUpdatedAt(input.Now).
		Save(ctx)
	if err == nil {
		return true, toRecord(row), nil
	}
	if !ent.IsConstraintError(err) {
		return false, contract.Record{}, err
	}
	existing, findErr := s.find(ctx, input.Key, input.Method, input.Path)
	if findErr != nil {
		return false, contract.Record{}, findErr
	}
	return false, existing, nil
}

func (s *Store) Reacquire(ctx context.Context, input contract.BeginInput) (contract.Record, error) {
	affected, err := s.client.IdempotencyRecord.Update().
		Where(
			entidem.IdempotencyKeyEQ(input.Key),
			entidem.MethodEQ(input.Method),
			entidem.PathEQ(input.Path),
			entidem.StatusEQ(string(contract.StatusInProgress)),
			entidem.LockedUntilLT(input.Now),
		).
		SetRequestHash(input.RequestHash).
		SetStatus(string(contract.StatusInProgress)).
		SetLockedUntil(input.LockedUntil).
		SetExpiresAt(input.ExpiresAt).
		SetResponseSnapshotJSON(map[string]any{}).
		SetUpdatedAt(input.Now).
		Save(ctx)
	if err != nil {
		return contract.Record{}, err
	}
	if affected == 0 {
		return contract.Record{}, contract.ErrNotFound
	}
	return s.find(ctx, input.Key, input.Method, input.Path)
}

func (s *Store) Complete(ctx context.Context, key, method, path string, snapshot *contract.Snapshot, now time.Time) (contract.Record, error) {
	update := s.client.IdempotencyRecord.Update().
		Where(
			entidem.IdempotencyKeyEQ(key),
			entidem.MethodEQ(method),
			entidem.PathEQ(path),
		).
		SetStatus(string(contract.StatusCompleted)).
		ClearLockedUntil().
		SetResponseSnapshotJSON(snapshotToJSON(snapshot)).
		SetUpdatedAt(now)
	affected, err := update.Save(ctx)
	if err != nil {
		return contract.Record{}, err
	}
	if affected == 0 {
		return contract.Record{}, contract.ErrNotFound
	}
	return s.find(ctx, key, method, path)
}

func (s *Store) DeleteExpired(ctx context.Context, before time.Time) (int, error) {
	return s.client.IdempotencyRecord.Delete().
		Where(entidem.ExpiresAtLT(before)).
		Exec(ctx)
}

func (s *Store) find(ctx context.Context, key, method, path string) (contract.Record, error) {
	row, err := s.client.IdempotencyRecord.Query().
		Where(
			entidem.IdempotencyKeyEQ(key),
			entidem.MethodEQ(method),
			entidem.PathEQ(path),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Record{}, contract.ErrNotFound
		}
		return contract.Record{}, err
	}
	return toRecord(row), nil
}

func toRecord(row *ent.IdempotencyRecord) contract.Record {
	record := contract.Record{
		Key:         row.IdempotencyKey,
		Method:      row.Method,
		Path:        row.Path,
		RequestHash: row.RequestHash,
		Status:      contract.Status(row.Status),
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.LockedUntil != nil {
		lockedUntil := *row.LockedUntil
		record.LockedUntil = &lockedUntil
	}
	record.Snapshot = snapshotFromJSON(row.ResponseSnapshotJSON)
	return record
}

func snapshotToJSON(snapshot *contract.Snapshot) map[string]any {
	if snapshot == nil {
		return map[string]any{}
	}
	headers := map[string]any{}
	for key, values := range snapshot.Headers {
		items := make([]any, 0, len(values))
		for _, value := range values {
			items = append(items, value)
		}
		headers[key] = items
	}
	return map[string]any{
		"status_code": snapshot.StatusCode,
		"headers":     headers,
		"body_b64":    base64.StdEncoding.EncodeToString(snapshot.Body),
	}
}

func snapshotFromJSON(payload map[string]any) *contract.Snapshot {
	if len(payload) == 0 {
		return nil
	}
	snapshot := &contract.Snapshot{Headers: map[string][]string{}}
	switch code := payload["status_code"].(type) {
	case float64:
		snapshot.StatusCode = int(code)
	case int:
		snapshot.StatusCode = code
	}
	if rawHeaders, ok := payload["headers"].(map[string]any); ok {
		for key, raw := range rawHeaders {
			values, ok := raw.([]any)
			if !ok {
				continue
			}
			out := make([]string, 0, len(values))
			for _, item := range values {
				if text, ok := item.(string); ok {
					out = append(out, text)
				}
			}
			snapshot.Headers[key] = out
		}
	}
	if encoded, ok := payload["body_b64"].(string); ok {
		if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			snapshot.Body = decoded
		}
	}
	if snapshot.StatusCode == 0 && len(snapshot.Body) == 0 && len(snapshot.Headers) == 0 {
		return nil
	}
	return snapshot
}
