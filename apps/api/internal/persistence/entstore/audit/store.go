package audit

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entauditlog "github.com/srapi/srapi/apps/api/ent/auditlog"
	"github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
)

var ErrInvalidStore = errors.New("invalid audit ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.Log) (contract.Log, error) {
	create := s.client.AuditLog.Create().
		SetNillableActorUserID(input.ActorUserID).
		SetAction(input.Action).
		SetResourceType(input.ResourceType).
		SetResourceID(input.ResourceID).
		SetBeforeJSON(cloneMap(input.Before)).
		SetAfterJSON(cloneMap(input.After)).
		SetIP(input.IP).
		SetUserAgent(input.UserAgent).
		SetTraceID(input.TraceID)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Log{}, err
	}
	return toLog(created), nil
}

func (s *Store) List(ctx context.Context) ([]contract.Log, error) {
	rows, err := s.client.AuditLog.Query().
		Order(entauditlog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Log, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLog(row))
	}
	return out, nil
}

func toLog(row *ent.AuditLog) contract.Log {
	return contract.Log{
		ID:           row.ID,
		ActorUserID:  cloneInt(row.ActorUserID),
		Action:       row.Action,
		ResourceType: row.ResourceType,
		ResourceID:   row.ResourceID,
		Before:       cloneMap(row.BeforeJSON),
		After:        cloneMap(row.AfterJSON),
		IP:           row.IP,
		UserAgent:    row.UserAgent,
		TraceID:      row.TraceID,
		CreatedAt:    row.CreatedAt,
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}
