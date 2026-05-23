package admincontrol

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entsetting "github.com/srapi/srapi/apps/api/ent/setting"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

var ErrInvalidStore = errors.New("invalid admin control ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Get(ctx context.Context, key string) (map[string]any, bool, error) {
	if key == "" {
		return nil, false, admincontrolcontract.ErrInvalidInput
	}
	row, err := s.client.Setting.Query().Where(entsetting.KeyEQ(key)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return cloneMap(row.ValueJSON), true, nil
}

func (s *Store) Set(ctx context.Context, key string, value map[string]any, updatedBy *int) error {
	if key == "" {
		return admincontrolcontract.ErrInvalidInput
	}
	affected, err := s.client.Setting.Update().
		Where(entsetting.KeyEQ(key)).
		SetValueJSON(cloneMap(value)).
		SetIsSecret(false).
		SetDescription("admin control plane state").
		SetNillableUpdatedBy(updatedBy).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	_, err = s.client.Setting.Create().
		SetKey(key).
		SetValueJSON(cloneMap(value)).
		SetIsSecret(false).
		SetDescription("admin control plane state").
		SetNillableUpdatedBy(updatedBy).
		Save(ctx)
	if err != nil && ent.IsConstraintError(err) {
		_, err = s.client.Setting.Update().
			Where(entsetting.KeyEQ(key)).
			SetValueJSON(cloneMap(value)).
			SetIsSecret(false).
			SetDescription("admin control plane state").
			SetNillableUpdatedBy(updatedBy).
			Save(ctx)
	}
	return err
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
