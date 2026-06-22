package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/srapi/srapi/apps/api/ent"
	entauditlog "github.com/srapi/srapi/apps/api/ent/auditlog"
	"github.com/srapi/srapi/apps/api/ent/predicate"
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

// ListPage implements contract.PageReader: filter, count, and slice in SQL
// with ORDER BY id DESC so the newest audit entries come back first.
func (s *Store) ListPage(ctx context.Context, filter contract.ListFilter, limit, offset int) (contract.ListPageResult, error) {
	predicates := auditPagePredicates(filter)
	base := s.client.AuditLog.Query()
	if len(predicates) > 0 {
		base = base.Where(predicates...)
	}
	total, err := base.Clone().Count(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	query := base.Order(ent.Desc(entauditlog.FieldID))
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	out := make([]contract.Log, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLog(row))
	}
	return contract.ListPageResult{Items: out, Total: total}, nil
}

func auditPagePredicates(filter contract.ListFilter) []predicate.AuditLog {
	predicates := make([]predicate.AuditLog, 0, 4)
	if action := strings.TrimSpace(filter.Action); action != "" {
		predicates = append(predicates, entauditlog.ActionEQ(action))
	}
	if resourceType := strings.TrimSpace(filter.ResourceType); resourceType != "" {
		predicates = append(predicates, entauditlog.ResourceTypeEQ(resourceType))
	}
	if filter.ActorUserID != nil {
		predicates = append(predicates, entauditlog.ActorUserIDEQ(*filter.ActorUserID))
	}
	if filter.Since != nil {
		predicates = append(predicates, entauditlog.CreatedAtGTE(filter.Since.UTC()))
	}
	return predicates
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
