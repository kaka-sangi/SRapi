package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	ententitlement "github.com/srapi/srapi/apps/api/ent/entitlement"
	entmodelregistry "github.com/srapi/srapi/apps/api/ent/modelregistry"
	entpricingrule "github.com/srapi/srapi/apps/api/ent/pricingrule"
	entsubscriptionplan "github.com/srapi/srapi/apps/api/ent/subscriptionplan"
	entusersubscription "github.com/srapi/srapi/apps/api/ent/usersubscription"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
)

var ErrInvalidStore = errors.New("invalid subscriptions ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreatePlan(ctx context.Context, input contract.CreateStoredPlan) (contract.SubscriptionPlan, error) {
	created, err := s.client.SubscriptionPlan.Create().
		SetName(input.Name).
		SetDescription(input.Description).
		SetPrice(input.Price).
		SetCurrency(input.Currency).
		SetValidityDays(input.ValidityDays).
		SetEntitlementsJSON(cloneMap(input.Entitlements)).
		SetForSale(input.ForSale).
		SetSortOrder(input.SortOrder).
		SetStatus(string(input.Status)).
		Save(ctx)
	if err != nil {
		return contract.SubscriptionPlan{}, err
	}
	return toPlan(created), nil
}

func (s *Store) UpdatePlan(ctx context.Context, id int, input contract.UpdateStoredPlan) (contract.SubscriptionPlan, error) {
	update := s.client.SubscriptionPlan.UpdateOneID(id).
		Where(entsubscriptionplan.DeletedAtIsNil()).
		SetNillableName(input.Name).
		SetNillableDescription(input.Description).
		SetNillablePrice(input.Price).
		SetNillableCurrency(input.Currency).
		SetNillableValidityDays(input.ValidityDays).
		SetNillableForSale(input.ForSale).
		SetNillableSortOrder(input.SortOrder)
	if input.Status != nil {
		update = update.SetStatus(string(*input.Status))
	}
	if input.Entitlements != nil {
		update = update.SetEntitlementsJSON(cloneMap(*input.Entitlements))
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.SubscriptionPlan{}, contract.ErrNotFound
		}
		return contract.SubscriptionPlan{}, err
	}
	return toPlan(updated), nil
}

func (s *Store) FindPlanByID(ctx context.Context, id int) (contract.SubscriptionPlan, error) {
	found, err := s.client.SubscriptionPlan.Query().
		Where(entsubscriptionplan.IDEQ(id), entsubscriptionplan.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.SubscriptionPlan{}, err
	}
	return toPlan(found), nil
}

func (s *Store) ListPlans(ctx context.Context) ([]contract.SubscriptionPlan, error) {
	rows, err := s.client.SubscriptionPlan.Query().
		Where(entsubscriptionplan.DeletedAtIsNil()).
		Order(entsubscriptionplan.BySortOrder(), entsubscriptionplan.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.SubscriptionPlan, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPlan(row))
	}
	return out, nil
}

func (s *Store) DeletePlan(ctx context.Context, id int) error {
	affected, err := s.client.SubscriptionPlan.Update().
		Where(entsubscriptionplan.IDEQ(id), entsubscriptionplan.DeletedAtIsNil()).
		SetDeletedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func (s *Store) CreateUserSubscription(ctx context.Context, input contract.CreateStoredSubscription) (contract.UserSubscription, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.UserSubscription{}, err
	}
	created, err := tx.UserSubscription.Create().
		SetUserID(input.UserID).
		SetPlanID(input.PlanID).
		SetStatus(string(input.Status)).
		SetStartsAt(input.StartsAt).
		SetExpiresAt(input.ExpiresAt).
		SetEntitlementsSnapshotJSON(cloneMap(input.EntitlementsSnapshot)).
		SetSourceType(input.SourceType).
		SetSourceID(input.SourceID).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.UserSubscription{}, contract.ErrNotFound
		}
		return contract.UserSubscription{}, err
	}
	if input.Status == contract.SubscriptionStatusActive {
		for key, value := range cloneMap(input.EntitlementsSnapshot) {
			create := tx.Entitlement.Create().
				SetUserID(input.UserID).
				SetScopeType("user").
				SetScopeID(input.UserID).
				SetFeatureKey(key).
				SetValueJSON(entitlementValue(value)).
				SetNillableQuotaLimit(entitlementQuotaLimit(key, value)).
				SetExpiresAt(input.ExpiresAt).
				SetSourceSubscriptionID(created.ID)
			if _, err := create.Save(ctx); err != nil {
				_ = tx.Rollback()
				return contract.UserSubscription{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return contract.UserSubscription{}, err
	}
	return toSubscription(created), nil
}

func (s *Store) FindUserSubscriptionBySource(ctx context.Context, sourceType string, sourceID string) (contract.UserSubscription, error) {
	found, err := s.client.UserSubscription.Query().
		Where(
			entusersubscription.SourceTypeEQ(sourceType),
			entusersubscription.SourceIDEQ(sourceID),
		).
		Order(entusersubscription.ByID()).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.UserSubscription{}, contract.ErrNotFound
		}
		return contract.UserSubscription{}, err
	}
	return toSubscription(found), nil
}

func (s *Store) ListUserSubscriptions(ctx context.Context) ([]contract.UserSubscription, error) {
	rows, err := s.client.UserSubscription.Query().
		Order(entusersubscription.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.UserSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubscription(row))
	}
	return out, nil
}

func (s *Store) ListUserSubscriptionsByUser(ctx context.Context, userID int) ([]contract.UserSubscription, error) {
	rows, err := s.client.UserSubscription.Query().
		Where(entusersubscription.UserIDEQ(userID)).
		Order(entusersubscription.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.UserSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubscription(row))
	}
	return out, nil
}

func (s *Store) ListActiveUserSubscriptions(ctx context.Context, userID int, at time.Time) ([]contract.UserSubscription, error) {
	rows, err := s.client.UserSubscription.Query().
		Where(
			entusersubscription.UserIDEQ(userID),
			entusersubscription.StatusEQ(string(contract.SubscriptionStatusActive)),
			entusersubscription.StartsAtLTE(at),
			entusersubscription.ExpiresAtGT(at),
		).
		Order(entusersubscription.ByStartsAt(), entusersubscription.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.UserSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubscription(row))
	}
	return out, nil
}

func (s *Store) ListActiveEntitlements(ctx context.Context, userID int, at time.Time) ([]contract.Entitlement, error) {
	at = at.UTC()
	rows, err := s.client.Entitlement.Query().
		Where(
			ententitlement.UserIDEQ(userID),
			ententitlement.ScopeTypeEQ("user"),
			ententitlement.ExpiresAtGT(at),
		).
		Order(ententitlement.BySourceSubscriptionID(), ententitlement.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	activeSubscriptions, err := s.activeSubscriptionIDs(ctx, rows, at)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Entitlement, 0, len(rows))
	for _, row := range rows {
		if !activeSubscriptions[row.SourceSubscriptionID] {
			continue
		}
		out = append(out, toEntitlement(row))
	}
	return out, nil
}

func (s *Store) activeSubscriptionIDs(ctx context.Context, rows []*ent.Entitlement, at time.Time) (map[int]bool, error) {
	ids := make([]int, 0, len(rows))
	seen := map[int]bool{}
	for _, row := range rows {
		if seen[row.SourceSubscriptionID] {
			continue
		}
		seen[row.SourceSubscriptionID] = true
		ids = append(ids, row.SourceSubscriptionID)
	}
	if len(ids) == 0 {
		return map[int]bool{}, nil
	}
	subscriptions, err := s.client.UserSubscription.Query().
		Where(
			entusersubscription.IDIn(ids...),
			entusersubscription.StatusEQ(string(contract.SubscriptionStatusActive)),
			entusersubscription.StartsAtLTE(at),
			entusersubscription.ExpiresAtGT(at),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	active := make(map[int]bool, len(subscriptions))
	for _, subscription := range subscriptions {
		active[subscription.ID] = true
	}
	return active, nil
}

func (s *Store) ListExpiredActiveUserSubscriptions(ctx context.Context, now time.Time) ([]contract.UserSubscription, error) {
	rows, err := s.client.UserSubscription.Query().
		Where(
			entusersubscription.StatusEQ(string(contract.SubscriptionStatusActive)),
			entusersubscription.ExpiresAtLT(now.UTC()),
		).
		Order(entusersubscription.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.UserSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubscription(row))
	}
	return out, nil
}

func (s *Store) ListActiveUserSubscriptionsExpiringBetween(ctx context.Context, from time.Time, until time.Time) ([]contract.UserSubscription, error) {
	rows, err := s.client.UserSubscription.Query().
		Where(
			entusersubscription.StatusEQ(string(contract.SubscriptionStatusActive)),
			entusersubscription.ExpiresAtGTE(from.UTC()),
			entusersubscription.ExpiresAtLTE(until.UTC()),
		).
		Order(entusersubscription.ByExpiresAt(), entusersubscription.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.UserSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubscription(row))
	}
	return out, nil
}

func (s *Store) ExpireUserSubscription(ctx context.Context, id int, now time.Time) (contract.UserSubscription, bool, error) {
	now = now.UTC()
	updated, err := s.client.UserSubscription.UpdateOneID(id).
		Where(
			entusersubscription.StatusEQ(string(contract.SubscriptionStatusActive)),
			entusersubscription.ExpiresAtLT(now),
		).
		SetStatus(string(contract.SubscriptionStatusExpired)).
		SetUpdatedAt(now).
		Save(ctx)
	if err == nil {
		return toSubscription(updated), true, nil
	}
	if !ent.IsNotFound(err) {
		return contract.UserSubscription{}, false, err
	}
	subscriptions, findErr := s.client.UserSubscription.Query().
		Where(entusersubscription.IDEQ(id)).
		Only(ctx)
	if findErr != nil {
		return contract.UserSubscription{}, false, findErr
	}
	return toSubscription(subscriptions), false, nil
}

func (s *Store) DeleteUserSubscription(ctx context.Context, id int) error {
	if err := s.client.UserSubscription.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) CreatePricingRule(ctx context.Context, input contract.PricingRule) (contract.PricingRule, error) {
	create := s.client.PricingRule.Create().
		SetModelID(input.ModelID).
		SetProviderID(input.ProviderID).
		SetInputPricePerMillion(input.InputPricePerMillionTokens).
		SetOutputPricePerMillion(input.OutputPricePerMillionTokens).
		SetCacheReadPricePerMillion(input.CacheReadPricePerMillionTokens).
		SetCacheWritePricePerMillion(input.CacheWritePricePerMillionTokens).
		SetCurrency(input.Currency).
		SetNillableEffectiveFrom(input.EffectiveFrom).
		SetNillableEffectiveTo(input.EffectiveTo)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.PricingRule{}, err
	}
	return toPricingRule(created), nil
}

func (s *Store) UpdatePricingRule(ctx context.Context, id int, input contract.UpdatePricingRuleRequest) (contract.PricingRule, error) {
	update := s.client.PricingRule.UpdateOneID(id).
		SetNillableInputPricePerMillion(input.InputPricePerMillionTokens).
		SetNillableOutputPricePerMillion(input.OutputPricePerMillionTokens).
		SetNillableCacheReadPricePerMillion(input.CacheReadPricePerMillionTokens).
		SetNillableCacheWritePricePerMillion(input.CacheWritePricePerMillionTokens).
		SetNillableCurrency(input.Currency)
	if input.EffectiveFrom != nil {
		if *input.EffectiveFrom == nil {
			update = update.ClearEffectiveFrom()
		} else {
			update = update.SetEffectiveFrom(**input.EffectiveFrom)
		}
	}
	if input.EffectiveTo != nil {
		if *input.EffectiveTo == nil {
			update = update.ClearEffectiveTo()
		} else {
			update = update.SetEffectiveTo(**input.EffectiveTo)
		}
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PricingRule{}, contract.ErrNotFound
		}
		return contract.PricingRule{}, err
	}
	return toPricingRule(updated), nil
}

func (s *Store) FindPricingRuleByID(ctx context.Context, id int) (contract.PricingRule, error) {
	found, err := s.client.PricingRule.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PricingRule{}, contract.ErrNotFound
		}
		return contract.PricingRule{}, err
	}
	return toPricingRule(found), nil
}

func (s *Store) ListPricingRules(ctx context.Context) ([]contract.PricingRule, error) {
	rows, err := s.client.PricingRule.Query().
		Order(entpricingrule.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	families, err := s.pricingRuleModelFamilies(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PricingRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPricingRuleWithFamily(row, families[row.ModelID]))
	}
	return out, nil
}

func (s *Store) pricingRuleModelFamilies(ctx context.Context, rows []*ent.PricingRule) (map[int]string, error) {
	ids := make([]int, 0, len(rows))
	seen := map[int]struct{}{}
	for _, row := range rows {
		if row.ModelID <= 0 {
			continue
		}
		if _, ok := seen[row.ModelID]; ok {
			continue
		}
		seen[row.ModelID] = struct{}{}
		ids = append(ids, row.ModelID)
	}
	if len(ids) == 0 {
		return map[int]string{}, nil
	}
	models, err := s.client.ModelRegistry.Query().
		Where(entmodelregistry.IDIn(ids...), entmodelregistry.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int]string, len(models))
	for _, model := range models {
		out[model.ID] = model.Family
	}
	return out, nil
}

func (s *Store) DeletePricingRule(ctx context.Context, id int) error {
	if err := s.client.PricingRule.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func toPlan(row *ent.SubscriptionPlan) contract.SubscriptionPlan {
	return contract.SubscriptionPlan{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		Price:        row.Price,
		Currency:     row.Currency,
		ValidityDays: row.ValidityDays,
		Entitlements: cloneMap(row.EntitlementsJSON),
		ForSale:      row.ForSale,
		SortOrder:    row.SortOrder,
		Status:       contract.PlanStatus(row.Status),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		DeletedAt:    cloneTime(row.DeletedAt),
	}
}

func toSubscription(row *ent.UserSubscription) contract.UserSubscription {
	return contract.UserSubscription{
		ID:                   row.ID,
		UserID:               row.UserID,
		PlanID:               row.PlanID,
		Status:               contract.SubscriptionStatus(row.Status),
		StartsAt:             row.StartsAt,
		ExpiresAt:            row.ExpiresAt,
		EntitlementsSnapshot: cloneMap(row.EntitlementsSnapshotJSON),
		SourceType:           row.SourceType,
		SourceID:             row.SourceID,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func toEntitlement(row *ent.Entitlement) contract.Entitlement {
	return contract.Entitlement{
		ID:                   row.ID,
		UserID:               row.UserID,
		ScopeType:            row.ScopeType,
		ScopeID:              row.ScopeID,
		FeatureKey:           row.FeatureKey,
		Value:                cloneMap(row.ValueJSON),
		QuotaLimit:           cloneString(row.QuotaLimit),
		ExpiresAt:            row.ExpiresAt,
		SourceSubscriptionID: row.SourceSubscriptionID,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func toPricingRule(row *ent.PricingRule) contract.PricingRule {
	return toPricingRuleWithFamily(row, "")
}

func toPricingRuleWithFamily(row *ent.PricingRule, modelFamily string) contract.PricingRule {
	return contract.PricingRule{
		ID:                              row.ID,
		ModelID:                         row.ModelID,
		ModelFamily:                     modelFamily,
		ProviderID:                      row.ProviderID,
		InputPricePerMillionTokens:      row.InputPricePerMillion,
		OutputPricePerMillionTokens:     row.OutputPricePerMillion,
		CacheReadPricePerMillionTokens:  row.CacheReadPricePerMillion,
		CacheWritePricePerMillionTokens: row.CacheWritePricePerMillion,
		Currency:                        row.Currency,
		EffectiveFrom:                   cloneTime(row.EffectiveFrom),
		EffectiveTo:                     cloneTime(row.EffectiveTo),
		CreatedAt:                       row.CreatedAt,
		UpdatedAt:                       row.UpdatedAt,
	}
}

func entitlementValue(value any) map[string]any {
	return map[string]any{"value": cloneAny(value)}
}

func entitlementQuotaLimit(key string, value any) *string {
	switch key {
	case "monthly_token_quota", "monthly_cost_quota":
		quota := fmt.Sprint(value)
		return &quota
	default:
		return nil
	}
}

func cloneAny(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
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

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
