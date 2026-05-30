package admincontrol

import (
	"context"
	"crypto/sha256"
	stdsql "database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	entopssystemlog "github.com/srapi/srapi/apps/api/ent/opssystemlog"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	entsetting "github.com/srapi/srapi/apps/api/ent/setting"
	entsubscriptionplan "github.com/srapi/srapi/apps/api/ent/subscriptionplan"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	entuserannouncementread "github.com/srapi/srapi/apps/api/ent/userannouncementread"
	entuserredeemcoderedemption "github.com/srapi/srapi/apps/api/ent/userredeemcoderedemption"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
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

func (s *Store) ListAnnouncementReads(ctx context.Context, userID int, announcementIDs []int) ([]admincontrolcontract.AnnouncementRead, error) {
	if userID <= 0 {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	query := s.client.UserAnnouncementRead.Query().
		Where(entuserannouncementread.UserIDEQ(userID))
	if len(announcementIDs) > 0 {
		query = query.Where(entuserannouncementread.AnnouncementIDIn(uniquePositiveInts(announcementIDs)...))
	}
	rows, err := query.Order(entuserannouncementread.ByReadAt(entsql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]admincontrolcontract.AnnouncementRead, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAnnouncementRead(row))
	}
	return items, nil
}

func (s *Store) MarkAnnouncementRead(ctx context.Context, userID int, announcementID int, at time.Time) (admincontrolcontract.AnnouncementRead, error) {
	if userID <= 0 || announcementID <= 0 {
		return admincontrolcontract.AnnouncementRead{}, admincontrolcontract.ErrInvalidInput
	}
	readAt := at.UTC()
	if readAt.IsZero() {
		readAt = time.Now().UTC()
	}
	row, err := s.client.UserAnnouncementRead.Create().
		SetUserID(userID).
		SetAnnouncementID(announcementID).
		SetReadAt(readAt).
		Save(ctx)
	if err == nil {
		return toAnnouncementRead(row), nil
	}
	if !ent.IsConstraintError(err) {
		return admincontrolcontract.AnnouncementRead{}, err
	}
	affected, updateErr := s.client.UserAnnouncementRead.Update().
		Where(
			entuserannouncementread.UserIDEQ(userID),
			entuserannouncementread.AnnouncementIDEQ(announcementID),
		).
		SetReadAt(readAt).
		Save(ctx)
	if updateErr != nil {
		return admincontrolcontract.AnnouncementRead{}, updateErr
	}
	if affected == 0 {
		return admincontrolcontract.AnnouncementRead{}, err
	}
	row, err = s.client.UserAnnouncementRead.Query().
		Where(
			entuserannouncementread.UserIDEQ(userID),
			entuserannouncementread.AnnouncementIDEQ(announcementID),
		).
		Only(ctx)
	if err != nil {
		return admincontrolcontract.AnnouncementRead{}, err
	}
	return toAnnouncementRead(row), nil
}

func (s *Store) RedeemCode(ctx context.Context, input admincontrolcontract.RedeemCodeRedemptionInput) (admincontrolcontract.RedeemCodeRedemptionResult, error) {
	if s == nil || s.client == nil || input.UserID <= 0 || strings.TrimSpace(input.Code) == "" {
		return admincontrolcontract.RedeemCodeRedemptionResult{}, admincontrolcontract.ErrInvalidInput
	}
	now := input.RedeemedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	codeValue := normalizeCode(input.Code)

	tx, err := s.client.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemptionResult{}, err
	}
	setting, err := tx.Setting.Query().Where(entsetting.KeyEQ(settingsKeyRedeemCodes)).Only(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return admincontrolcontract.RedeemCodeRedemptionResult{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.RedeemCodeRedemptionResult{}, err
	}
	collection, err := redeemCodeCollectionFromMap(setting.ValueJSON)
	if err != nil {
		_ = tx.Rollback()
		return admincontrolcontract.RedeemCodeRedemptionResult{}, err
	}
	for idx, item := range collection.Items {
		item = redeemCodeWithDerivedStatus(item, now)
		if item.Code != codeValue {
			collection.Items[idx] = item
			continue
		}
		if existing, ok, err := findRedemption(ctx, tx.Client(), input.UserID, item.ID); err != nil {
			_ = tx.Rollback()
			return admincontrolcontract.RedeemCodeRedemptionResult{}, err
		} else if ok {
			_ = tx.Rollback()
			return admincontrolcontract.RedeemCodeRedemptionResult{
				Redemption:      existing,
				RedeemCode:      item,
				AlreadyRedeemed: true,
			}, nil
		}
		if item.Status != admincontrolcontract.RedeemCodeStatusActive || item.RedeemedCount >= item.MaxRedemptions {
			collection.Items[idx] = item
			if err := saveRedeemCodeSetting(ctx, tx.Client(), setting.ID, collection); err != nil {
				_ = tx.Rollback()
				return admincontrolcontract.RedeemCodeRedemptionResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return admincontrolcontract.RedeemCodeRedemptionResult{}, err
			}
			return admincontrolcontract.RedeemCodeRedemptionResult{}, admincontrolcontract.ErrConflict
		}
		redemption, err := fulfillRedeemCode(ctx, tx.Client(), input.UserID, item, now)
		if err != nil {
			_ = tx.Rollback()
			return admincontrolcontract.RedeemCodeRedemptionResult{}, err
		}
		item.RedeemedCount++
		item.UpdatedAt = now
		if item.RedeemedCount >= item.MaxRedemptions {
			item.Status = admincontrolcontract.RedeemCodeStatusRedeemed
		}
		collection.Items[idx] = item
		if err := saveRedeemCodeSetting(ctx, tx.Client(), setting.ID, collection); err != nil {
			_ = tx.Rollback()
			return admincontrolcontract.RedeemCodeRedemptionResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return admincontrolcontract.RedeemCodeRedemptionResult{}, err
		}
		return admincontrolcontract.RedeemCodeRedemptionResult{
			Redemption: redemption,
			RedeemCode: item,
		}, nil
	}
	if err := saveRedeemCodeSetting(ctx, tx.Client(), setting.ID, collection); err != nil {
		_ = tx.Rollback()
		return admincontrolcontract.RedeemCodeRedemptionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return admincontrolcontract.RedeemCodeRedemptionResult{}, err
	}
	return admincontrolcontract.RedeemCodeRedemptionResult{}, admincontrolcontract.ErrNotFound
}

func (s *Store) PreviewPromoCode(ctx context.Context, input admincontrolcontract.PromoCodePreviewInput) (admincontrolcontract.PromoCodeApplication, error) {
	if s == nil || s.client == nil {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	return PreviewPromoCodeWithClient(ctx, s.client, input)
}

func (s *Store) FinalizePromoCode(ctx context.Context, input admincontrolcontract.PromoCodeFinalizeInput) (admincontrolcontract.PromoCodeApplication, error) {
	if s == nil || s.client == nil {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	tx, err := s.client.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	application, err := FinalizePromoCodeWithClient(ctx, tx.Client(), input)
	if err != nil {
		_ = tx.Rollback()
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	if err := tx.Commit(); err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	return application, nil
}

func (s *Store) CreateSystemLog(ctx context.Context, input admincontrolcontract.OpsSystemLog) (admincontrolcontract.OpsSystemLog, error) {
	if strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Message) == "" || !input.Level.Valid() {
		return admincontrolcontract.OpsSystemLog{}, admincontrolcontract.ErrInvalidInput
	}
	create := s.client.OpsSystemLog.Create().
		SetLevel(string(input.Level)).
		SetSource(strings.TrimSpace(input.Source)).
		SetMessage(strings.TrimSpace(input.Message)).
		SetRequestID(strings.TrimSpace(input.RequestID)).
		SetTraceID(strings.TrimSpace(input.TraceID)).
		SetMetadataJSON(cloneMap(input.Metadata))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt.UTC())
	}
	row, err := create.Save(ctx)
	if err != nil {
		return admincontrolcontract.OpsSystemLog{}, err
	}
	return toSystemLog(row), nil
}

func (s *Store) ListSystemLogs(ctx context.Context, opts admincontrolcontract.SystemLogListOptions) (admincontrolcontract.SystemLogList, error) {
	if err := validateSystemLogListOptions(opts); err != nil {
		return admincontrolcontract.SystemLogList{}, err
	}
	page, pageSize := normalizePage(opts.Page, opts.PageSize)
	filter := admincontrolcontract.SystemLogCleanupFilter{
		Level:  opts.Level,
		Source: opts.Source,
		Query:  opts.Query,
		Start:  opts.Start,
		End:    opts.End,
	}
	predicates := systemLogPredicates(filter)
	total, err := s.client.OpsSystemLog.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return admincontrolcontract.SystemLogList{}, err
	}
	rows, err := s.client.OpsSystemLog.Query().
		Where(predicates...).
		Order(entopssystemlog.ByCreatedAt(entsql.OrderDesc()), entopssystemlog.ByID(entsql.OrderDesc())).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return admincontrolcontract.SystemLogList{}, err
	}
	items := make([]admincontrolcontract.OpsSystemLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSystemLog(row))
	}
	return admincontrolcontract.SystemLogList{Items: items, Total: total}, nil
}

func (s *Store) CleanupSystemLogs(ctx context.Context, filter admincontrolcontract.SystemLogCleanupFilter) (admincontrolcontract.SystemLogCleanupResult, error) {
	normalized, err := normalizeSystemLogCleanupFilter(filter)
	if err != nil {
		return admincontrolcontract.SystemLogCleanupResult{}, err
	}
	predicates := systemLogPredicates(normalized)
	matched, err := s.client.OpsSystemLog.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return admincontrolcontract.SystemLogCleanupResult{}, err
	}
	result := admincontrolcontract.SystemLogCleanupResult{
		Matched:   matched,
		DryRun:    normalized.DryRun,
		MaxDelete: normalized.MaxDelete,
	}
	if normalized.DryRun || matched == 0 {
		return result, nil
	}
	rows, err := s.client.OpsSystemLog.Query().
		Where(predicates...).
		Order(entopssystemlog.ByCreatedAt(), entopssystemlog.ByID()).
		Limit(normalized.MaxDelete).
		IDs(ctx)
	if err != nil {
		return admincontrolcontract.SystemLogCleanupResult{}, err
	}
	deleted, err := s.client.OpsSystemLog.Delete().
		Where(entopssystemlog.IDIn(rows...)).
		Exec(ctx)
	if err != nil {
		return admincontrolcontract.SystemLogCleanupResult{}, err
	}
	result.Deleted = deleted
	result.Limited = matched > deleted
	return result, nil
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

const settingsKeyRedeemCodes = "admin_control.redeem_codes"

type redeemCodeCollection struct {
	NextID int                               `json:"next_id"`
	Items  []admincontrolcontract.RedeemCode `json:"items"`
}

func redeemCodeCollectionFromMap(value map[string]any) (redeemCodeCollection, error) {
	var collection redeemCodeCollection
	raw, err := json.Marshal(value)
	if err != nil {
		return redeemCodeCollection{}, err
	}
	if err := json.Unmarshal(raw, &collection); err != nil {
		return redeemCodeCollection{}, err
	}
	return collection, nil
}

func saveRedeemCodeSetting(ctx context.Context, client *ent.Client, settingID int, collection redeemCodeCollection) error {
	value, err := redeemCodeCollectionToMap(collection)
	if err != nil {
		return err
	}
	_, err = client.Setting.UpdateOneID(settingID).
		SetValueJSON(value).
		Save(ctx)
	return err
}

func redeemCodeCollectionToMap(collection redeemCodeCollection) (map[string]any, error) {
	raw, err := json.Marshal(collection)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func findRedemption(ctx context.Context, client *ent.Client, userID int, redeemCodeID int) (admincontrolcontract.RedeemCodeRedemption, bool, error) {
	row, err := client.UserRedeemCodeRedemption.Query().
		Where(
			entuserredeemcoderedemption.UserIDEQ(userID),
			entuserredeemcoderedemption.RedeemCodeIDEQ(redeemCodeID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.RedeemCodeRedemption{}, false, nil
		}
		return admincontrolcontract.RedeemCodeRedemption{}, false, err
	}
	return toRedeemCodeRedemption(row), true, nil
}

func fulfillRedeemCode(ctx context.Context, client *ent.Client, userID int, code admincontrolcontract.RedeemCode, now time.Time) (admincontrolcontract.RedeemCodeRedemption, error) {
	switch code.Type {
	case admincontrolcontract.RedeemCodeTypeBalance:
		return fulfillBalanceRedeemCode(ctx, client, userID, code, now)
	case admincontrolcontract.RedeemCodeTypeSubscription:
		return fulfillSubscriptionRedeemCode(ctx, client, userID, code, now)
	default:
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrInvalidInput
	}
}

func fulfillBalanceRedeemCode(ctx context.Context, client *ent.Client, userID int, code admincontrolcontract.RedeemCode, now time.Time) (admincontrolcontract.RedeemCodeRedemption, error) {
	user, err := client.User.Query().
		Where(entuser.IDEQ(userID), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	if userscontract.Status(user.Status) != userscontract.StatusActive {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrConflict
	}
	amount, ok := decimalRat(code.Value)
	if !ok || amount.Sign() <= 0 {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrInvalidInput
	}
	before, ok := decimalRat(user.Balance)
	if !ok {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrInvalidInput
	}
	after := new(big.Rat).Add(before, amount)
	balanceBefore := formatRatFixed(before, 8)
	balanceAfter := formatRatFixed(after, 8)
	normalizedAmount := formatRatFixed(amount, 8)
	currency := normalizeCurrency(code.Currency)
	if _, err := client.User.UpdateOneID(user.ID).
		Where(entuser.DeletedAtIsNil()).
		SetBalance(balanceAfter).
		SetCurrency(currency).
		Save(ctx); err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	ledger, err := client.BillingLedger.Create().
		SetUserID(userID).
		SetType(string(billingcontract.LedgerTypeRedeemCodeCredit)).
		SetAmount(normalizedAmount).
		SetCurrency(currency).
		SetBalanceBefore(balanceBefore).
		SetBalanceAfter(balanceAfter).
		SetReferenceType("redeem_code").
		SetReferenceID(strconv.Itoa(code.ID)).
		SetMetadataJSON(map[string]any{"redeem_code_id": code.ID}).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	redemption, err := createRedemption(ctx, client, redemptionCreateInput{
		UserID:          userID,
		Code:            code,
		Amount:          normalizedAmount,
		Currency:        currency,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    balanceAfter,
		BillingLedgerID: &ledger.ID,
		RedeemedAt:      now,
	})
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	return redemption, nil
}

func fulfillSubscriptionRedeemCode(ctx context.Context, client *ent.Client, userID int, code admincontrolcontract.RedeemCode, now time.Time) (admincontrolcontract.RedeemCodeRedemption, error) {
	user, err := client.User.Query().
		Where(entuser.IDEQ(userID), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	if userscontract.Status(user.Status) != userscontract.StatusActive {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrConflict
	}
	planID, err := strconv.Atoi(strings.TrimSpace(code.Value))
	if err != nil || planID <= 0 {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrInvalidInput
	}
	plan, err := client.SubscriptionPlan.Query().
		Where(
			entsubscriptionplan.IDEQ(planID),
			entsubscriptionplan.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	if subscriptioncontract.PlanStatus(plan.Status) != subscriptioncontract.PlanStatusActive || plan.ValidityDays <= 0 {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrConflict
	}
	expiresAt := now.AddDate(0, 0, plan.ValidityDays)
	subscription, err := client.UserSubscription.Create().
		SetUserID(userID).
		SetPlanID(plan.ID).
		SetStatus(string(subscriptioncontract.SubscriptionStatusActive)).
		SetStartsAt(now).
		SetExpiresAt(expiresAt).
		SetEntitlementsSnapshotJSON(cloneMap(plan.EntitlementsJSON)).
		SetSourceType("redeem_code").
		SetSourceID(strconv.Itoa(code.ID)).
		Save(ctx)
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	for key, value := range cloneMap(plan.EntitlementsJSON) {
		create := client.Entitlement.Create().
			SetUserID(userID).
			SetScopeType("user").
			SetScopeID(userID).
			SetFeatureKey(key).
			SetValueJSON(entitlementValue(value)).
			SetNillableQuotaLimit(entitlementQuotaLimit(key, value)).
			SetExpiresAt(expiresAt).
			SetSourceSubscriptionID(subscription.ID)
		if _, err := create.Save(ctx); err != nil {
			return admincontrolcontract.RedeemCodeRedemption{}, err
		}
	}
	redemption, err := createRedemption(ctx, client, redemptionCreateInput{
		UserID:             userID,
		Code:               code,
		Amount:             "0.00000000",
		Currency:           normalizeCurrency(plan.Currency),
		BalanceBefore:      user.Balance,
		BalanceAfter:       user.Balance,
		UserSubscriptionID: &subscription.ID,
		RedeemedAt:         now,
	})
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	return redemption, nil
}

type redemptionCreateInput struct {
	UserID             int
	Code               admincontrolcontract.RedeemCode
	Amount             string
	Currency           string
	BalanceBefore      string
	BalanceAfter       string
	BillingLedgerID    *int
	UserSubscriptionID *int
	RedeemedAt         time.Time
}

func createRedemption(ctx context.Context, client *ent.Client, input redemptionCreateInput) (admincontrolcontract.RedeemCodeRedemption, error) {
	create := client.UserRedeemCodeRedemption.Create().
		SetUserID(input.UserID).
		SetRedeemCodeID(input.Code.ID).
		SetCodeDigest(codeDigest(input.Code.Code)).
		SetType(string(input.Code.Type)).
		SetAmount(input.Amount).
		SetCurrency(normalizeCurrency(input.Currency)).
		SetBalanceBefore(input.BalanceBefore).
		SetBalanceAfter(input.BalanceAfter).
		SetNillableBillingLedgerID(input.BillingLedgerID).
		SetNillableUserSubscriptionID(input.UserSubscriptionID).
		SetRedeemedAt(input.RedeemedAt).
		SetMetadataJSON(map[string]any{"redeem_code_id": input.Code.ID}).
		SetCreatedAt(input.RedeemedAt).
		SetUpdatedAt(input.RedeemedAt)
	row, err := create.Save(ctx)
	if err != nil {
		return admincontrolcontract.RedeemCodeRedemption{}, err
	}
	return toRedeemCodeRedemption(row), nil
}

func toRedeemCodeRedemption(row *ent.UserRedeemCodeRedemption) admincontrolcontract.RedeemCodeRedemption {
	return admincontrolcontract.RedeemCodeRedemption{
		ID:                 row.ID,
		UserID:             row.UserID,
		RedeemCodeID:       row.RedeemCodeID,
		Type:               admincontrolcontract.RedeemCodeType(row.Type),
		Amount:             row.Amount,
		Currency:           row.Currency,
		BalanceBefore:      row.BalanceBefore,
		BalanceAfter:       row.BalanceAfter,
		BillingLedgerID:    cloneInt(row.BillingLedgerID),
		UserSubscriptionID: cloneInt(row.UserSubscriptionID),
		RedeemedAt:         row.RedeemedAt,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func redeemCodeWithDerivedStatus(item admincontrolcontract.RedeemCode, now time.Time) admincontrolcontract.RedeemCode {
	if item.Status == admincontrolcontract.RedeemCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrolcontract.RedeemCodeStatusExpired
	}
	if item.Status == admincontrolcontract.RedeemCodeStatusActive && item.MaxRedemptions > 0 && item.RedeemedCount >= item.MaxRedemptions {
		item.Status = admincontrolcontract.RedeemCodeStatusRedeemed
	}
	return item
}

func codeDigest(value string) string {
	sum := sha256.Sum256([]byte(normalizeCode(value)))
	return hex.EncodeToString(sum[:])
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeCurrency(value string) string {
	currency := strings.ToUpper(strings.TrimSpace(value))
	if currency == "" {
		return "USD"
	}
	return currency
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	return rat, ok
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	return value.FloatString(places)
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

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func uniquePositiveInts(values []int) []int {
	out := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func validateSystemLogListOptions(opts admincontrolcontract.SystemLogListOptions) error {
	if opts.Level != "" && !opts.Level.Valid() {
		return admincontrolcontract.ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return admincontrolcontract.ErrInvalidInput
	}
	return nil
}

func normalizeSystemLogCleanupFilter(filter admincontrolcontract.SystemLogCleanupFilter) (admincontrolcontract.SystemLogCleanupFilter, error) {
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Level != "" && !filter.Level.Valid() {
		return admincontrolcontract.SystemLogCleanupFilter{}, admincontrolcontract.ErrInvalidInput
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return admincontrolcontract.SystemLogCleanupFilter{}, admincontrolcontract.ErrInvalidInput
	}
	if filter.Level == "" && filter.Source == "" && filter.Query == "" && filter.Start == nil && filter.End == nil {
		return admincontrolcontract.SystemLogCleanupFilter{}, admincontrolcontract.ErrInvalidInput
	}
	if filter.MaxDelete == 0 {
		filter.MaxDelete = 1000
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > 10000 {
		return admincontrolcontract.SystemLogCleanupFilter{}, admincontrolcontract.ErrInvalidInput
	}
	return filter, nil
}

func systemLogPredicates(filter admincontrolcontract.SystemLogCleanupFilter) []predicate.OpsSystemLog {
	var predicates []predicate.OpsSystemLog
	if filter.Level != "" {
		predicates = append(predicates, entopssystemlog.LevelEQ(string(filter.Level)))
	}
	if source := strings.TrimSpace(filter.Source); source != "" {
		predicates = append(predicates, entopssystemlog.SourceEqualFold(source))
	}
	if filter.Start != nil {
		predicates = append(predicates, entopssystemlog.CreatedAtGTE(filter.Start.UTC()))
	}
	if filter.End != nil {
		predicates = append(predicates, entopssystemlog.CreatedAtLT(filter.End.UTC()))
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		predicates = append(predicates, entopssystemlog.Or(
			entopssystemlog.MessageContainsFold(query),
			entopssystemlog.SourceContainsFold(query),
			entopssystemlog.RequestIDContainsFold(query),
			entopssystemlog.TraceIDContainsFold(query),
		))
	}
	return predicates
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return page, pageSize
}

func toSystemLog(row *ent.OpsSystemLog) admincontrolcontract.OpsSystemLog {
	if row == nil {
		return admincontrolcontract.OpsSystemLog{}
	}
	return admincontrolcontract.OpsSystemLog{
		ID:        row.ID,
		Level:     admincontrolcontract.OpsSystemLogLevel(row.Level),
		Message:   row.Message,
		Source:    row.Source,
		RequestID: row.RequestID,
		TraceID:   row.TraceID,
		Metadata:  cloneMap(row.MetadataJSON),
		CreatedAt: row.CreatedAt,
	}
}

func toAnnouncementRead(row *ent.UserAnnouncementRead) admincontrolcontract.AnnouncementRead {
	if row == nil {
		return admincontrolcontract.AnnouncementRead{}
	}
	return admincontrolcontract.AnnouncementRead{
		ID:             row.ID,
		UserID:         row.UserID,
		AnnouncementID: row.AnnouncementID,
		ReadAt:         row.ReadAt,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
