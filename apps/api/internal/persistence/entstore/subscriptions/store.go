package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
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

func (s *Store) CreateUserSubscription(ctx context.Context, input contract.CreateStoredSubscription) (contract.UserSubscription, error) {
	created, err := s.client.UserSubscription.Create().
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
		return contract.UserSubscription{}, err
	}
	return toSubscription(created), nil
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

func (s *Store) ListPricingRules(ctx context.Context) ([]contract.PricingRule, error) {
	rows, err := s.client.PricingRule.Query().
		Order(entpricingrule.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PricingRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPricingRule(row))
	}
	return out, nil
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

func toPricingRule(row *ent.PricingRule) contract.PricingRule {
	return contract.PricingRule{
		ID:                              row.ID,
		ModelID:                         row.ModelID,
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

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
