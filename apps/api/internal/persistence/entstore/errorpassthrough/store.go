package errorpassthrough

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entrule "github.com/srapi/srapi/apps/api/ent/errorpassthroughrule"
	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
)

var ErrInvalidStore = errors.New("invalid error passthrough ent store")

// Store is the Ent-backed implementation of the error-passthrough rule store.
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
	row, err := s.client.ErrorPassthroughRule.Create().
		SetName(input.Name).
		SetEnabled(input.Enabled).
		SetPriority(input.Priority).
		SetAction(string(input.Action)).
		SetMatchStatusCodes(cloneInts(input.StatusCodes)).
		SetMatchClasses(cloneStrings(input.Classes)).
		SetMatchKeywords(cloneStrings(input.Keywords)).
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
	update := s.client.ErrorPassthroughRule.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
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
	if input.StatusCodes != nil {
		update.SetMatchStatusCodes(cloneInts(*input.StatusCodes))
	}
	if input.Classes != nil {
		update.SetMatchClasses(cloneStrings(*input.Classes))
	}
	if input.Keywords != nil {
		update.SetMatchKeywords(cloneStrings(*input.Keywords))
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
	if err := s.client.ErrorPassthroughRule.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) ListRules(ctx context.Context) ([]contract.Rule, error) {
	rows, err := s.client.ErrorPassthroughRule.Query().
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

func toRule(row *ent.ErrorPassthroughRule) contract.Rule {
	return contract.Rule{
		ID:          row.ID,
		Name:        row.Name,
		Enabled:     row.Enabled,
		Priority:    row.Priority,
		Action:      contract.Action(row.Action),
		StatusCodes: cloneInts(row.MatchStatusCodes),
		Classes:     cloneStrings(row.MatchClasses),
		Keywords:    cloneStrings(row.MatchKeywords),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	return append([]int(nil), values...)
}
