package apikeys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entapikey "github.com/srapi/srapi/apps/api/ent/apikey"
	entapikeygroup "github.com/srapi/srapi/apps/api/ent/apikeygroup"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	"github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
)

var ErrInvalidStore = errors.New("invalid api keys ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateStoredKey) (contract.APIKey, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.APIKey{}, err
	}
	workspaceID, err := workspaceIDForKey(ctx, tx, input)
	if err != nil {
		_ = tx.Rollback()
		return contract.APIKey{}, err
	}

	created, err := tx.APIKey.Create().
		SetUserID(input.UserID).
		SetNillableWorkspaceID(workspaceID).
		SetName(input.Name).
		SetPrefix(input.Prefix).
		SetHash(input.Hash).
		SetStatus(string(input.Status)).
		SetScopesJSON(cloneStrings(input.Scopes)).
		SetAllowedModelsJSON(cloneStrings(input.AllowedModels)).
		SetNillableRpmLimit(input.RPMLimit).
		SetNillableTpmLimit(input.TPMLimit).
		SetNillableConcurrencyLimit(input.ConcurrencyLimit).
		SetNillableRequestLimit5h(input.RequestLimit5h).
		SetNillableRequestLimit1d(input.RequestLimit1d).
		SetNillableRequestLimit7d(input.RequestLimit7d).
		SetAllowedIpsJSON(cloneStrings(input.AllowedIPs)).
		SetDeniedIpsJSON(cloneStrings(input.DeniedIPs)).
		SetNillableExpiresAt(input.ExpiresAt).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.APIKey{}, err
	}

	for _, groupID := range uniqueInts(input.GroupIDs) {
		if _, err := tx.APIKeyGroup.Create().SetAPIKeyID(created.ID).SetAccountGroupID(groupID).Save(ctx); err != nil {
			_ = tx.Rollback()
			return contract.APIKey{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return contract.APIKey{}, err
	}
	return s.FindByPrefix(ctx, created.Prefix)
}

func (s *Store) Update(ctx context.Context, key contract.APIKey) (contract.APIKey, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.APIKey{}, err
	}

	stored, err := tx.APIKey.Query().
		Where(entapikey.IDEQ(key.ID), entapikey.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.APIKey{}, contract.ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}

	_, err = tx.APIKey.UpdateOneID(stored.ID).
		Where(entapikey.DeletedAtIsNil()).
		SetName(key.Name).
		SetStatus(string(key.Status)).
		SetScopesJSON(cloneStrings(key.Scopes)).
		SetAllowedModelsJSON(cloneStrings(key.AllowedModels)).
		SetNillableRpmLimit(key.RPMLimit).
		SetNillableTpmLimit(key.TPMLimit).
		SetNillableConcurrencyLimit(key.ConcurrencyLimit).
		SetNillableRequestLimit5h(key.RequestLimit5h).
		SetNillableRequestLimit1d(key.RequestLimit1d).
		SetNillableRequestLimit7d(key.RequestLimit7d).
		SetAllowedIpsJSON(cloneStrings(key.AllowedIPs)).
		SetDeniedIpsJSON(cloneStrings(key.DeniedIPs)).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.APIKey{}, contract.ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}

	if _, err := tx.APIKeyGroup.Delete().Where(entapikeygroup.APIKeyIDEQ(stored.ID)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return contract.APIKey{}, err
	}
	for _, groupID := range uniqueInts(key.GroupIDs) {
		if _, err := tx.APIKeyGroup.Create().SetAPIKeyID(stored.ID).SetAccountGroupID(groupID).Save(ctx); err != nil {
			_ = tx.Rollback()
			return contract.APIKey{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return contract.APIKey{}, err
	}
	return s.FindByPrefix(ctx, stored.Prefix)
}

func (s *Store) FindByPrefix(ctx context.Context, prefix string) (contract.APIKey, error) {
	found, err := s.client.APIKey.Query().
		Where(entapikey.PrefixEQ(prefix), entapikey.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.APIKey{}, contract.ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}
	return s.toAPIKey(ctx, found)
}

func (s *Store) ListByUser(ctx context.Context, userID int) ([]contract.APIKey, error) {
	keys, err := s.client.APIKey.Query().
		Where(entapikey.UserIDEQ(userID), entapikey.DeletedAtIsNil()).
		Order(entapikey.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.APIKey, 0, len(keys))
	for _, key := range keys {
		mapped, err := s.toAPIKey(ctx, key)
		if err != nil {
			return nil, err
		}
		out = append(out, mapped)
	}
	return out, nil
}

func (s *Store) List(ctx context.Context) ([]contract.APIKey, error) {
	keys, err := s.client.APIKey.Query().
		Where(entapikey.DeletedAtIsNil()).
		Order(entapikey.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.APIKey, 0, len(keys))
	for _, key := range keys {
		mapped, err := s.toAPIKey(ctx, key)
		if err != nil {
			return nil, err
		}
		out = append(out, mapped)
	}
	return out, nil
}

func (s *Store) TouchLastUsed(ctx context.Context, id int, usedAt time.Time) error {
	_, err := s.client.APIKey.UpdateOneID(id).
		Where(entapikey.DeletedAtIsNil()).
		SetLastUsedAt(usedAt).
		Save(ctx)
	return err
}

func (s *Store) toAPIKey(ctx context.Context, key *ent.APIKey) (contract.APIKey, error) {
	groupIDs, err := s.groupIDs(ctx, key.ID)
	if err != nil {
		return contract.APIKey{}, err
	}
	return contract.APIKey{
		ID:               key.ID,
		UserID:           key.UserID,
		WorkspaceID:      cloneIntPointer(key.WorkspaceID),
		Name:             key.Name,
		Prefix:           key.Prefix,
		Hash:             key.Hash,
		Status:           contract.Status(key.Status),
		Scopes:           cloneStrings(key.ScopesJSON),
		AllowedModels:    cloneStrings(key.AllowedModelsJSON),
		GroupIDs:         groupIDs,
		RPMLimit:         key.RpmLimit,
		TPMLimit:         key.TpmLimit,
		ConcurrencyLimit: key.ConcurrencyLimit,
		RequestLimit5h:   key.RequestLimit5h,
		RequestLimit1d:   key.RequestLimit1d,
		RequestLimit7d:   key.RequestLimit7d,
		AllowedIPs:       cloneStrings(key.AllowedIpsJSON),
		DeniedIPs:        cloneStrings(key.DeniedIpsJSON),
		ExpiresAt:        key.ExpiresAt,
		LastUsedAt:       key.LastUsedAt,
		CreatedAt:        key.CreatedAt,
	}, nil
}

func (s *Store) groupIDs(ctx context.Context, apiKeyID int) ([]int, error) {
	rows, err := s.client.APIKeyGroup.Query().
		Where(entapikeygroup.APIKeyIDEQ(apiKeyID)).
		Order(entapikeygroup.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AccountGroupID)
	}
	return out, nil
}

func workspaceIDForKey(ctx context.Context, tx *ent.Tx, input contract.CreateStoredKey) (*int, error) {
	if input.WorkspaceID != nil {
		return cloneIntPointer(input.WorkspaceID), nil
	}
	owner, err := tx.User.Query().
		Where(entuser.IDEQ(input.UserID), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("api key owner user %d not found", input.UserID)
		}
		return nil, err
	}
	return cloneIntPointer(owner.WorkspaceID), nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func uniqueInts(values []int) []int {
	out := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
