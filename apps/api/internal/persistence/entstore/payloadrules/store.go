package payloadrules

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entrule "github.com/srapi/srapi/apps/api/ent/payloadrule"
	"github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
)

var ErrInvalidStore = errors.New("invalid payload rule ent store")

// Store is the Ent-backed implementation of the payload-rule store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateRule(ctx context.Context, input contract.CreateRule) (contract.Rule, error) {
	now := time.Now().UTC()
	row, err := s.client.PayloadRule.Create().
		SetName(input.Name).
		SetEnabled(input.Enabled).
		SetPriority(input.Priority).
		SetAction(string(input.Action)).
		SetMatchModel(input.MatchModel).
		SetMatchProtocol(input.MatchProtocol).
		SetParamsJSON(cloneParams(input.Params)).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Rule{}, err
	}
	return toRule(row), nil
}

func (s *Store) UpdateRule(ctx context.Context, id int, input contract.UpdateRule) (contract.Rule, error) {
	if id <= 0 {
		return contract.Rule{}, ErrInvalidStore
	}
	update := s.client.PayloadRule.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if input.Name != nil {
		update.SetName(*input.Name)
	}
	if input.Enabled != nil {
		update.SetEnabled(*input.Enabled)
	}
	if input.Priority != nil {
		update.SetPriority(*input.Priority)
	}
	if input.Action != nil {
		update.SetAction(string(*input.Action))
	}
	if input.MatchModel != nil {
		update.SetMatchModel(*input.MatchModel)
	}
	if input.MatchProtocol != nil {
		update.SetMatchProtocol(*input.MatchProtocol)
	}
	if input.Params != nil {
		update.SetParamsJSON(cloneParams(*input.Params))
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Rule{}, contract.ErrNotFound
		}
		return contract.Rule{}, err
	}
	return toRule(row), nil
}

func (s *Store) DeleteRule(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if err := s.client.PayloadRule.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) ListRules(ctx context.Context) ([]contract.Rule, error) {
	rows, err := s.client.PayloadRule.Query().
		Order(ent.Asc(entrule.FieldPriority), ent.Asc(entrule.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Rule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRule(row))
	}
	return out, nil
}

func toRule(row *ent.PayloadRule) contract.Rule {
	return contract.Rule{
		ID:            row.ID,
		Name:          row.Name,
		Enabled:       row.Enabled,
		Priority:      row.Priority,
		Action:        contract.Action(row.Action),
		MatchModel:    row.MatchModel,
		MatchProtocol: row.MatchProtocol,
		Params:        cloneParams(row.ParamsJSON),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func cloneParams(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(params))
	for key, value := range params {
		out[key] = value
	}
	return out
}
