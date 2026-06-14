package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	ententitlement "github.com/srapi/srapi/apps/api/ent/entitlement"
	entsubscriptionplan "github.com/srapi/srapi/apps/api/ent/subscriptionplan"
	entusersubscription "github.com/srapi/srapi/apps/api/ent/usersubscription"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
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

func (s *Store) MaterializedUsageForUser(ctx context.Context, userID int, at time.Time) (contract.MaterializedUsage, error) {
	at = at.UTC()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.MaterializedUsage{}, err
	}
	row, err := activeSubscriptionForUser(ctx, tx, userID, at)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.MaterializedUsage{}, nil
		}
		return contract.MaterializedUsage{}, err
	}
	subscription := resetExpiredUsage(toSubscription(row), at)
	if _, err := updateSubscriptionUsage(ctx, tx, subscription); err != nil {
		_ = tx.Rollback()
		return contract.MaterializedUsage{}, err
	}
	if err := tx.Commit(); err != nil {
		return contract.MaterializedUsage{}, err
	}
	return materializedUsageFromSubscription(subscription, at), nil
}

func (s *Store) IncrementMaterializedUsage(ctx context.Context, delta contract.UsageDelta) (contract.MaterializedUsage, error) {
	at := delta.OccurredAt.UTC()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.MaterializedUsage{}, err
	}
	row, err := activeSubscriptionForUser(ctx, tx, delta.UserID, at)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.MaterializedUsage{}, nil
		}
		return contract.MaterializedUsage{}, err
	}
	updated := applyUsageDelta(toSubscription(row), delta)
	if _, err := updateSubscriptionUsage(ctx, tx, updated); err != nil {
		_ = tx.Rollback()
		return contract.MaterializedUsage{}, err
	}
	if err := tx.Commit(); err != nil {
		return contract.MaterializedUsage{}, err
	}
	return materializedUsageFromSubscription(updated, at), nil
}

// ApplyUsageDeltaTx applies a materialized-usage delta within the caller's
// transaction (the caller owns commit/rollback). It is a no-op when the user has
// no active subscription. Used by the cross-table billing-aggregation coordinator
// so the subscription increment, the API-key cost increment and the usage_log
// marker all commit atomically. Reuses the same window-roll logic as
// IncrementMaterializedUsage.
func (s *Store) ApplyUsageDeltaTx(ctx context.Context, tx *ent.Tx, delta contract.UsageDelta) error {
	row, err := activeSubscriptionForUser(ctx, tx, delta.UserID, delta.OccurredAt.UTC())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return err
	}
	updated := applyUsageDelta(toSubscription(row), delta)
	_, err = updateSubscriptionUsage(ctx, tx, updated)
	return err
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

func activeSubscriptionForUser(ctx context.Context, tx *ent.Tx, userID int, at time.Time) (*ent.UserSubscription, error) {
	return tx.UserSubscription.Query().
		Where(
			entusersubscription.UserIDEQ(userID),
			entusersubscription.StatusEQ(string(contract.SubscriptionStatusActive)),
			entusersubscription.StartsAtLTE(at),
			entusersubscription.ExpiresAtGT(at),
		).
		Order(ent.Desc(entusersubscription.FieldStartsAt), ent.Desc(entusersubscription.FieldID)).
		First(ctx)
}

func updateSubscriptionUsage(ctx context.Context, tx *ent.Tx, subscription contract.UserSubscription) (*ent.UserSubscription, error) {
	return tx.UserSubscription.UpdateOneID(subscription.ID).
		SetDailyUsageUsd(money.NormalizeAmount(subscription.DailyUsageUSD)).
		SetNillableDailyUsageWindowStart(subscription.DailyWindowStart).
		SetWeeklyUsageUsd(money.NormalizeAmount(subscription.WeeklyUsageUSD)).
		SetNillableWeeklyUsageWindowStart(subscription.WeeklyWindowStart).
		SetMonthlyUsageUsd(money.NormalizeAmount(subscription.MonthlyUsageUSD)).
		SetNillableMonthlyUsageWindowStart(subscription.MonthlyWindowStart).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
}

func materializedUsageFromSubscription(subscription contract.UserSubscription, at time.Time) contract.MaterializedUsage {
	updated := resetExpiredUsage(subscription, at)
	return contract.MaterializedUsage{
		SubscriptionID:     updated.ID,
		UserID:             updated.UserID,
		DailyUsageUSD:      money.NormalizeAmount(updated.DailyUsageUSD),
		DailyWindowStart:   cloneTime(updated.DailyWindowStart),
		WeeklyUsageUSD:     money.NormalizeAmount(updated.WeeklyUsageUSD),
		WeeklyWindowStart:  cloneTime(updated.WeeklyWindowStart),
		MonthlyUsageUSD:    money.NormalizeAmount(updated.MonthlyUsageUSD),
		MonthlyWindowStart: cloneTime(updated.MonthlyWindowStart),
	}
}

func applyUsageDelta(subscription contract.UserSubscription, delta contract.UsageDelta) contract.UserSubscription {
	updated := resetExpiredUsage(subscription, delta.OccurredAt)
	cost := money.NormalizeAmount(delta.BillableCost)
	updated.DailyUsageUSD = money.AddMoney(updated.DailyUsageUSD, cost)
	updated.WeeklyUsageUSD = money.AddMoney(updated.WeeklyUsageUSD, cost)
	updated.MonthlyUsageUSD = money.AddMoney(updated.MonthlyUsageUSD, cost)
	return updated
}

func resetExpiredUsage(subscription contract.UserSubscription, at time.Time) contract.UserSubscription {
	at = at.UTC()
	dayStart := startOfDayUTC(at)
	weekStart := startOfWeekUTC(at)
	monthStart := startOfMonthUTC(at)
	if subscription.DailyWindowStart == nil || windowRolledForward(*subscription.DailyWindowStart, dayStart) {
		subscription.DailyUsageUSD = money.ZeroAmount
		subscription.DailyWindowStart = &dayStart
	}
	if subscription.WeeklyWindowStart == nil || windowRolledForward(*subscription.WeeklyWindowStart, weekStart) {
		subscription.WeeklyUsageUSD = money.ZeroAmount
		subscription.WeeklyWindowStart = &weekStart
	}
	if subscription.MonthlyWindowStart == nil || windowRolledForward(*subscription.MonthlyWindowStart, monthStart) {
		subscription.MonthlyUsageUSD = money.ZeroAmount
		subscription.MonthlyWindowStart = &monthStart
	}
	subscription.DailyUsageUSD = money.NormalizeAmount(subscription.DailyUsageUSD)
	subscription.WeeklyUsageUSD = money.NormalizeAmount(subscription.WeeklyUsageUSD)
	subscription.MonthlyUsageUSD = money.NormalizeAmount(subscription.MonthlyUsageUSD)
	return subscription
}

func startOfDayUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfWeekUTC(value time.Time) time.Time {
	dayStart := startOfDayUTC(value)
	offset := (int(dayStart.Weekday()) + 6) % 7
	return dayStart.AddDate(0, 0, -offset)
}

func startOfMonthUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// windowRolledForward reports whether expectedStart (the period containing the
// event time) is a strictly newer period than the stored window start. It is
// deliberately forward-only: the live path always advances time, so this matches
// the previous "different period" behavior there, but a reconciler replaying an
// older usage_log row (OccurredAt in a past period) must NOT roll a current
// window backward — that would zero out the current period's accumulated usage.
func windowRolledForward(storedStart, expectedStart time.Time) bool {
	return expectedStart.UTC().After(storedStart.UTC())
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
		DailyUsageUSD:        money.NormalizeAmount(row.DailyUsageUsd),
		DailyWindowStart:     cloneTime(row.DailyUsageWindowStart),
		WeeklyUsageUSD:       money.NormalizeAmount(row.WeeklyUsageUsd),
		WeeklyWindowStart:    cloneTime(row.WeeklyUsageWindowStart),
		MonthlyUsageUSD:      money.NormalizeAmount(row.MonthlyUsageUsd),
		MonthlyWindowStart:   cloneTime(row.MonthlyUsageWindowStart),
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

func entitlementValue(value any) map[string]any {
	return map[string]any{"value": cloneAny(value)}
}

func entitlementQuotaLimit(key string, value any) *string {
	switch key {
	case "monthly_token_quota", "daily_cost_quota", "weekly_cost_quota", "monthly_cost_quota":
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
