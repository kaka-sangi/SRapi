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
	entredeemcode "github.com/srapi/srapi/apps/api/ent/redeemcode"
	entsetting "github.com/srapi/srapi/apps/api/ent/setting"
	entsubscriptionplan "github.com/srapi/srapi/apps/api/ent/subscriptionplan"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	entuserannouncementread "github.com/srapi/srapi/apps/api/ent/userannouncementread"
	entuserredeemcoderedemption "github.com/srapi/srapi/apps/api/ent/userredeemcoderedemption"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
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

func (s *Store) ListAnnouncementReadsByAnnouncement(ctx context.Context, announcementID, limit int) ([]admincontrolcontract.AnnouncementRead, error) {
	if announcementID <= 0 {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.client.UserAnnouncementRead.Query().
		Where(entuserannouncementread.AnnouncementIDEQ(announcementID)).
		Order(entuserannouncementread.ByReadAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
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
	row, err := tx.RedeemCode.Query().Where(entredeemcode.CodeEQ(codeValue)).Only(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return admincontrolcontract.RedeemCodeRedemptionResult{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.RedeemCodeRedemptionResult{}, err
	}
	item := redeemCodeWithDerivedStatus(toRedeemCode(row), now)
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
		_ = tx.Rollback()
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
	if _, err := tx.RedeemCode.UpdateOneID(item.ID).
		SetRedeemedCount(item.RedeemedCount).
		SetStatus(string(item.Status)).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
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

func (s *Store) ReleasePromoCode(ctx context.Context, input admincontrolcontract.PromoCodeReleaseInput) (admincontrolcontract.PromoCodeApplication, bool, error) {
	if s == nil || s.client == nil {
		return admincontrolcontract.PromoCodeApplication{}, false, admincontrolcontract.ErrInvalidInput
	}
	tx, err := s.client.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	application, released, err := ReleasePromoCodeWithClient(ctx, tx.Client(), input)
	if err != nil {
		_ = tx.Rollback()
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	return application, released, nil
}

func (s *Store) ListPromoCodeUsages(ctx context.Context, promoCodeID, limit int) ([]admincontrolcontract.PromoCodeApplication, error) {
	if s == nil || s.client == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	return ListPromoCodeUsagesWithClient(ctx, s.client, promoCodeID, limit)
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
	amount, ok := money.RequiredDecimalRat(code.Value)
	if !ok || amount.Sign() <= 0 {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrInvalidInput
	}
	before, ok := money.RequiredDecimalRat(user.Balance)
	if !ok {
		return admincontrolcontract.RedeemCodeRedemption{}, admincontrolcontract.ErrInvalidInput
	}
	after := new(big.Rat).Add(before, amount)
	balanceBefore := money.FormatRatFixed(before, 8)
	balanceAfter := money.FormatRatFixed(after, 8)
	normalizedAmount := money.FormatRatFixed(amount, 8)
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
	return money.NormalizeCurrency(value)
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
