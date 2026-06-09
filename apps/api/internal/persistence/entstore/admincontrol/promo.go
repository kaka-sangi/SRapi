package admincontrol

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entsetting "github.com/srapi/srapi/apps/api/ent/setting"
	entuserpromocodeapplication "github.com/srapi/srapi/apps/api/ent/userpromocodeapplication"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

const settingsKeyPromoCodes = "admin_control.promo_codes"

type promoCodeCollection struct {
	NextID int                              `json:"next_id"`
	Items  []admincontrolcontract.PromoCode `json:"items"`
}

// PreviewPromoCodeWithClient evaluates a promo code against an Ent client without mutating state.
func PreviewPromoCodeWithClient(ctx context.Context, client *ent.Client, input admincontrolcontract.PromoCodePreviewInput) (admincontrolcontract.PromoCodeApplication, error) {
	if client == nil || input.UserID <= 0 || strings.TrimSpace(input.Code) == "" {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	collection, _, err := LoadPromoCodes(ctx, client)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	item, _, ok := findPromoCode(collection.Items, input.Code, now)
	if !ok {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrNotFound
	}
	return previewPromoCode(item, input.UserID, input.Amount, input.Currency, now)
}

// FinalizePromoCodeWithClient creates the durable application receipt and increments promo usage.
func FinalizePromoCodeWithClient(ctx context.Context, client *ent.Client, input admincontrolcontract.PromoCodeFinalizeInput) (admincontrolcontract.PromoCodeApplication, error) {
	if client == nil || input.UserID <= 0 || strings.TrimSpace(input.Code) == "" || input.PaymentOrderID <= 0 || strings.TrimSpace(input.OrderNo) == "" {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	now := input.AppliedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if existing, err := client.UserPromoCodeApplication.Query().
		Where(entuserpromocodeapplication.PaymentOrderIDEQ(input.PaymentOrderID)).
		Only(ctx); err == nil {
		return ToPromoCodeApplication(existing), nil
	} else if !ent.IsNotFound(err) {
		return admincontrolcontract.PromoCodeApplication{}, err
	}

	collection, setting, err := LoadPromoCodes(ctx, client)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	item, idx, ok := findPromoCode(collection.Items, input.Code, now)
	if !ok {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrNotFound
	}
	application, err := previewPromoCode(item, input.UserID, input.OriginalAmount, input.Currency, now)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	if normalizeCurrency(application.Currency) != normalizeCurrency(input.Currency) || application.FinalAmount != formatInputMoney(input.FinalAmount) {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	row, err := client.UserPromoCodeApplication.Create().
		SetUserID(input.UserID).
		SetPromoCodeID(item.ID).
		SetCodeDigest(codeDigest(item.Code)).
		SetPaymentOrderID(input.PaymentOrderID).
		SetOrderNo(strings.TrimSpace(input.OrderNo)).
		SetOriginalAmount(application.OriginalAmount).
		SetDiscountAmount(application.DiscountAmount).
		SetFinalAmount(application.FinalAmount).
		SetCurrency(application.Currency).
		SetDiscountType(string(item.DiscountType)).
		SetAppliedAt(now).
		SetMetadataJSON(map[string]any{"promo_code_id": item.ID}).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := client.UserPromoCodeApplication.Query().
				Where(entuserpromocodeapplication.PaymentOrderIDEQ(input.PaymentOrderID)).
				Only(ctx); findErr == nil {
				return ToPromoCodeApplication(existing), nil
			}
		}
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	item.UsedCount++
	item.UpdatedAt = now
	if item.UsedCount >= item.MaxUses {
		item.Status = admincontrolcontract.PromoCodeStatusExpired
	}
	collection.Items[idx] = item
	if err := SavePromoCodeSetting(ctx, client, setting.ID, collection); err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	return ToPromoCodeApplication(row), nil
}

// ListPromoCodeUsagesWithClient returns the redemption rows for one promo code,
// newest first, bounded by limit. It reads the durably-indexed application table
// (promo_code_id index) written at finalize time.
func ListPromoCodeUsagesWithClient(ctx context.Context, client *ent.Client, promoCodeID, limit int) ([]admincontrolcontract.PromoCodeApplication, error) {
	if client == nil || promoCodeID <= 0 {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := client.UserPromoCodeApplication.Query().
		Where(entuserpromocodeapplication.PromoCodeIDEQ(promoCodeID)).
		Order(ent.Desc(entuserpromocodeapplication.FieldAppliedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	usages := make([]admincontrolcontract.PromoCodeApplication, 0, len(rows))
	for _, row := range rows {
		usages = append(usages, ToPromoCodeApplication(row))
	}
	return usages, nil
}

func LoadPromoCodes(ctx context.Context, client *ent.Client) (promoCodeCollection, *ent.Setting, error) {
	setting, err := client.Setting.Query().Where(entsetting.KeyEQ(settingsKeyPromoCodes)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return promoCodeCollection{}, nil, admincontrolcontract.ErrNotFound
		}
		return promoCodeCollection{}, nil, err
	}
	collection, err := promoCodeCollectionFromMap(setting.ValueJSON)
	if err != nil {
		return promoCodeCollection{}, nil, err
	}
	return collection, setting, nil
}

func SavePromoCodeSetting(ctx context.Context, client *ent.Client, settingID int, collection promoCodeCollection) error {
	value, err := promoCodeCollectionToMap(collection)
	if err != nil {
		return err
	}
	_, err = client.Setting.UpdateOneID(settingID).
		SetValueJSON(value).
		Save(ctx)
	return err
}

func promoCodeCollectionFromMap(value map[string]any) (promoCodeCollection, error) {
	var collection promoCodeCollection
	raw, err := json.Marshal(value)
	if err != nil {
		return promoCodeCollection{}, err
	}
	if err := json.Unmarshal(raw, &collection); err != nil {
		return promoCodeCollection{}, err
	}
	return collection, nil
}

func promoCodeCollectionToMap(collection promoCodeCollection) (map[string]any, error) {
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

func findPromoCode(items []admincontrolcontract.PromoCode, code string, now time.Time) (admincontrolcontract.PromoCode, int, bool) {
	code = normalizeCode(code)
	for idx, item := range items {
		item = promoCodeWithDerivedStatus(item, now)
		if normalizeCode(item.Code) == code {
			return item, idx, true
		}
	}
	return admincontrolcontract.PromoCode{}, -1, false
}

func previewPromoCode(item admincontrolcontract.PromoCode, userID int, amount string, currency string, now time.Time) (admincontrolcontract.PromoCodeApplication, error) {
	item = promoCodeWithDerivedStatus(item, now)
	if item.Status != admincontrolcontract.PromoCodeStatusActive || item.MaxUses <= 0 || item.UsedCount >= item.MaxUses {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrConflict
	}
	if item.StartsAt != nil && item.StartsAt.After(now) {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrConflict
	}
	inputAmount, ok := money.RequiredDecimalRat(amount)
	if !ok || inputAmount.Sign() <= 0 {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	normalizedCurrency := normalizeCurrency(currency)
	if item.DiscountType == admincontrolcontract.PromoDiscountTypeAmount && normalizeCurrency(item.Currency) != normalizedCurrency {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrConflict
	}
	discount, err := promoDiscountAmount(item, inputAmount)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	if discount.Sign() <= 0 || discount.Cmp(inputAmount) >= 0 {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	finalAmount := new(big.Rat).Sub(inputAmount, discount)
	return admincontrolcontract.PromoCodeApplication{
		UserID:         userID,
		PromoCodeID:    item.ID,
		OriginalAmount: money.FormatRatFixed(inputAmount, 8),
		DiscountAmount: money.FormatRatFixed(discount, 8),
		FinalAmount:    money.FormatRatFixed(finalAmount, 8),
		Currency:       normalizedCurrency,
		DiscountType:   item.DiscountType,
		AppliedAt:      now,
	}, nil
}

func promoDiscountAmount(item admincontrolcontract.PromoCode, amount *big.Rat) (*big.Rat, error) {
	value, ok := money.RequiredDecimalRat(item.DiscountValue)
	if !ok || value.Sign() <= 0 {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	switch item.DiscountType {
	case admincontrolcontract.PromoDiscountTypeAmount:
		return value, nil
	case admincontrolcontract.PromoDiscountTypePercent:
		if value.Cmp(big.NewRat(1, 1)) > 0 {
			return nil, admincontrolcontract.ErrInvalidInput
		}
		return new(big.Rat).Mul(amount, value), nil
	default:
		return nil, admincontrolcontract.ErrInvalidInput
	}
}

func promoCodeWithDerivedStatus(item admincontrolcontract.PromoCode, now time.Time) admincontrolcontract.PromoCode {
	if item.Status == admincontrolcontract.PromoCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrolcontract.PromoCodeStatusExpired
	}
	if item.Status == admincontrolcontract.PromoCodeStatusActive && item.MaxUses > 0 && item.UsedCount >= item.MaxUses {
		item.Status = admincontrolcontract.PromoCodeStatusExpired
	}
	return item
}

func formatInputMoney(value string) string {
	rat, ok := money.RequiredDecimalRat(value)
	if !ok {
		return ""
	}
	return money.FormatRatFixed(rat, 8)
}

// ToPromoCodeApplication converts the generated Ent row into the module contract.
func ToPromoCodeApplication(row *ent.UserPromoCodeApplication) admincontrolcontract.PromoCodeApplication {
	if row == nil {
		return admincontrolcontract.PromoCodeApplication{}
	}
	return admincontrolcontract.PromoCodeApplication{
		ID:             row.ID,
		UserID:         row.UserID,
		PromoCodeID:    row.PromoCodeID,
		PaymentOrderID: row.PaymentOrderID,
		OrderNo:        row.OrderNo,
		OriginalAmount: row.OriginalAmount,
		DiscountAmount: row.DiscountAmount,
		FinalAmount:    row.FinalAmount,
		Currency:       row.Currency,
		DiscountType:   admincontrolcontract.PromoDiscountType(row.DiscountType),
		AppliedAt:      row.AppliedAt,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
