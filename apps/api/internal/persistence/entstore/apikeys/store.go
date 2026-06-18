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
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
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
		SetNillableCostQuota(input.CostQuota).
		SetNillableCostLimit5h(input.CostLimit5h).
		SetNillableCostLimit1d(input.CostLimit1d).
		SetNillableCostLimit7d(input.CostLimit7d).
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
		SetNillableCostQuota(key.CostQuota).
		SetCostUsed(key.CostUsed).
		SetNillableCostLimit5h(key.CostLimit5h).
		SetCostUsed5h(key.CostUsed5h).
		SetNillableCostWindowStart5h(key.CostWindowStart5h).
		SetNillableCostLimit1d(key.CostLimit1d).
		SetCostUsed1d(key.CostUsed1d).
		SetNillableCostWindowStart1d(key.CostWindowStart1d).
		SetNillableCostLimit7d(key.CostLimit7d).
		SetCostUsed7d(key.CostUsed7d).
		SetNillableCostWindowStart7d(key.CostWindowStart7d).
		SetAllowedIpsJSON(cloneStrings(key.AllowedIPs)).
		SetDeniedIpsJSON(cloneStrings(key.DeniedIPs)).
		SetNillableExpiresAt(key.ExpiresAt).
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

// ResetUsage zeros the rolling cost-used counters in a single UPDATE so it
// can't lose a race against ApplyCostUsage (which would otherwise increment
// between a FindByID + Update pair). Clears the window-start timestamps too —
// the next charge opens a fresh rolling window.
func (s *Store) ResetUsage(ctx context.Context, id int) (contract.APIKey, error) {
	n, err := s.client.APIKey.Update().
		Where(entapikey.IDEQ(id), entapikey.DeletedAtIsNil()).
		SetCostUsed("0").
		SetCostUsed5h("0").
		ClearCostWindowStart5h().
		SetCostUsed1d("0").
		ClearCostWindowStart1d().
		SetCostUsed7d("0").
		ClearCostWindowStart7d().
		Save(ctx)
	if err != nil {
		return contract.APIKey{}, err
	}
	if n == 0 {
		return contract.APIKey{}, contract.ErrKeyNotFound
	}
	return s.FindByID(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id int) error {
	now := time.Now().UTC()
	n, err := s.client.APIKey.Update().
		Where(entapikey.IDEQ(id), entapikey.DeletedAtIsNil()).
		SetDeletedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return contract.ErrKeyNotFound
	}
	return nil
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

func (s *Store) FindDeletedByPrefix(ctx context.Context, prefix string) (contract.APIKey, error) {
	found, err := s.client.APIKey.Query().
		Where(entapikey.PrefixEQ(prefix), entapikey.DeletedAtNotNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.APIKey{}, contract.ErrKeyNotFound
		}
		return contract.APIKey{}, err
	}
	return s.toAPIKey(ctx, found)
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.APIKey, error) {
	found, err := s.client.APIKey.Query().
		Where(entapikey.IDEQ(id), entapikey.DeletedAtIsNil()).
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

func (s *Store) ApplyCostUsage(ctx context.Context, input contract.CostUsageUpdate) (contract.APIKey, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.APIKey{}, err
	}
	if err := s.ApplyCostUsageTx(ctx, tx, input); err != nil {
		_ = tx.Rollback()
		return contract.APIKey{}, err
	}
	if err := tx.Commit(); err != nil {
		return contract.APIKey{}, err
	}
	return s.FindByID(ctx, input.KeyID)
}

// ApplyCostUsageTx applies a cost-usage delta to an API key within the caller's
// transaction (the caller owns commit/rollback). Returns contract.ErrKeyNotFound
// when the key is absent. Used by the cross-table billing-aggregation
// coordinator so the cost increment commits atomically with the subscription
// increment and the usage_log marker.
func (s *Store) ApplyCostUsageTx(ctx context.Context, tx *ent.Tx, input contract.CostUsageUpdate) error {
	stored, err := tx.APIKey.Query().
		Where(entapikey.IDEQ(input.KeyID), entapikey.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrKeyNotFound
		}
		return err
	}
	key, err := s.toAPIKeyWithTx(ctx, tx, stored)
	if err != nil {
		return err
	}
	key = applyAPIKeyCostUsage(key, input)
	_, err = tx.APIKey.UpdateOneID(stored.ID).
		Where(entapikey.DeletedAtIsNil()).
		SetCostUsed(key.CostUsed).
		SetCostUsed5h(key.CostUsed5h).
		SetNillableCostWindowStart5h(key.CostWindowStart5h).
		SetCostUsed1d(key.CostUsed1d).
		SetNillableCostWindowStart1d(key.CostWindowStart1d).
		SetCostUsed7d(key.CostUsed7d).
		SetNillableCostWindowStart7d(key.CostWindowStart7d).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrKeyNotFound
		}
		return err
	}
	return nil
}

func applyAPIKeyCostUsage(key contract.APIKey, input contract.CostUsageUpdate) contract.APIKey {
	at := input.OccurredAt.UTC()
	key = resetAPIKeyExpiredCostWindows(key, at)
	cost := money.NormalizeAmount(input.BillableCost)
	key.CostUsed = money.AddMoney(key.CostUsed, cost)
	key.CostUsed5h = money.AddMoney(key.CostUsed5h, cost)
	key.CostUsed1d = money.AddMoney(key.CostUsed1d, cost)
	key.CostUsed7d = money.AddMoney(key.CostUsed7d, cost)
	return key
}

func resetAPIKeyExpiredCostWindows(key contract.APIKey, at time.Time) contract.APIKey {
	at = at.UTC()
	if key.CostWindowStart5h == nil || rollingCounterExpired(*key.CostWindowStart5h, at, 5*time.Hour) {
		key.CostUsed5h = money.ZeroAmount
		key.CostWindowStart5h = &at
	}
	if key.CostWindowStart1d == nil || rollingCounterExpired(*key.CostWindowStart1d, at, 24*time.Hour) {
		key.CostUsed1d = money.ZeroAmount
		key.CostWindowStart1d = &at
	}
	if key.CostWindowStart7d == nil || rollingCounterExpired(*key.CostWindowStart7d, at, 7*24*time.Hour) {
		key.CostUsed7d = money.ZeroAmount
		key.CostWindowStart7d = &at
	}
	key.CostUsed = money.NormalizeAmount(key.CostUsed)
	key.CostUsed5h = money.NormalizeAmount(key.CostUsed5h)
	key.CostUsed1d = money.NormalizeAmount(key.CostUsed1d)
	key.CostUsed7d = money.NormalizeAmount(key.CostUsed7d)
	return key
}

func rollingCounterExpired(storedStart, now time.Time, duration time.Duration) bool {
	if duration <= 0 {
		return false
	}
	return !storedStart.UTC().Add(duration).After(now.UTC())
}

func (s *Store) toAPIKey(ctx context.Context, key *ent.APIKey) (contract.APIKey, error) {
	groupIDs, err := s.groupIDs(ctx, s.client.APIKeyGroup, key.ID)
	if err != nil {
		return contract.APIKey{}, err
	}
	return apiKeyFromEnt(key, groupIDs), nil
}

func (s *Store) toAPIKeyWithTx(ctx context.Context, tx *ent.Tx, key *ent.APIKey) (contract.APIKey, error) {
	groupIDs, err := s.groupIDs(ctx, tx.APIKeyGroup, key.ID)
	if err != nil {
		return contract.APIKey{}, err
	}
	return apiKeyFromEnt(key, groupIDs), nil
}

func apiKeyFromEnt(key *ent.APIKey, groupIDs []int) contract.APIKey {
	return contract.APIKey{
		ID:                key.ID,
		UserID:            key.UserID,
		WorkspaceID:       cloneIntPointer(key.WorkspaceID),
		Name:              key.Name,
		Prefix:            key.Prefix,
		Hash:              key.Hash,
		Status:            contract.Status(key.Status),
		Scopes:            cloneStrings(key.ScopesJSON),
		AllowedModels:     cloneStrings(key.AllowedModelsJSON),
		GroupIDs:          groupIDs,
		RPMLimit:          key.RpmLimit,
		TPMLimit:          key.TpmLimit,
		ConcurrencyLimit:  key.ConcurrencyLimit,
		RequestLimit5h:    key.RequestLimit5h,
		RequestLimit1d:    key.RequestLimit1d,
		RequestLimit7d:    key.RequestLimit7d,
		CostQuota:         cloneStringPointer(key.CostQuota),
		CostUsed:          key.CostUsed,
		CostLimit5h:       cloneStringPointer(key.CostLimit5h),
		CostUsed5h:        key.CostUsed5h,
		CostWindowStart5h: cloneTimePointer(key.CostWindowStart5h),
		CostLimit1d:       cloneStringPointer(key.CostLimit1d),
		CostUsed1d:        key.CostUsed1d,
		CostWindowStart1d: cloneTimePointer(key.CostWindowStart1d),
		CostLimit7d:       cloneStringPointer(key.CostLimit7d),
		CostUsed7d:        key.CostUsed7d,
		CostWindowStart7d: cloneTimePointer(key.CostWindowStart7d),
		AllowedIPs:        cloneStrings(key.AllowedIpsJSON),
		DeniedIPs:         cloneStrings(key.DeniedIpsJSON),
		ExpiresAt:         key.ExpiresAt,
		LastUsedAt:        key.LastUsedAt,
		CreatedAt:         key.CreatedAt,
	}
}

type apiKeyGroupQuery interface {
	Query() *ent.APIKeyGroupQuery
}

func (s *Store) groupIDs(ctx context.Context, groups apiKeyGroupQuery, apiKeyID int) ([]int, error) {
	rows, err := groups.Query().
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

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
