package admincontrol

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entpromocode "github.com/srapi/srapi/apps/api/ent/promocode"
	entuserpromocodeapplication "github.com/srapi/srapi/apps/api/ent/userpromocodeapplication"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

// PreviewPromoCodeWithClient evaluates a promo code against an Ent client without mutating state.
func PreviewPromoCodeWithClient(ctx context.Context, client *ent.Client, input admincontrolcontract.PromoCodePreviewInput) (admincontrolcontract.PromoCodeApplication, error) {
	if client == nil || input.UserID <= 0 || strings.TrimSpace(input.Code) == "" {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrInvalidInput
	}
	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	item, _, err := findPromoCodeByCode(ctx, client, input.Code, now)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	userUses, err := countActivePromoUses(ctx, client, item.ID, input.UserID)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	return previewPromoCode(item, input.UserID, input.Amount, input.Currency, now, userUses)
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

	item, _, err := findPromoCodeByCode(ctx, client, input.Code, now)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	userUses, err := countActivePromoUses(ctx, client, item.ID, input.UserID)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	application, err := previewPromoCode(item, input.UserID, input.OriginalAmount, input.Currency, now, userUses)
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
	if _, err := client.PromoCode.UpdateOneID(item.ID).
		SetUsedCount(item.UsedCount).
		SetStatus(string(item.Status)).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return admincontrolcontract.PromoCodeApplication{}, err
	}
	return ToPromoCodeApplication(row), nil
}

func ReleasePromoCodeWithClient(ctx context.Context, client *ent.Client, input admincontrolcontract.PromoCodeReleaseInput) (admincontrolcontract.PromoCodeApplication, bool, error) {
	if client == nil || input.PaymentOrderID <= 0 {
		return admincontrolcontract.PromoCodeApplication{}, false, admincontrolcontract.ErrInvalidInput
	}
	releasedAt := input.ReleasedAt.UTC()
	if releasedAt.IsZero() {
		releasedAt = time.Now().UTC()
	}
	row, err := client.UserPromoCodeApplication.Query().
		Where(entuserpromocodeapplication.PaymentOrderIDEQ(input.PaymentOrderID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.PromoCodeApplication{}, false, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	if promoApplicationReleased(row.MetadataJSON) {
		return ToPromoCodeApplication(row), false, nil
	}
	codeRow, err := client.PromoCode.Query().Where(entpromocode.IDEQ(row.PromoCodeID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.PromoCodeApplication{}, false, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	item := toPromoCode(codeRow)
	if item.UsedCount > 0 {
		item.UsedCount--
	}
	if item.Status == admincontrolcontract.PromoCodeStatusExpired && item.UsedCount < item.MaxUses && (item.ExpiresAt == nil || item.ExpiresAt.After(releasedAt)) {
		item.Status = admincontrolcontract.PromoCodeStatusActive
	}
	item.UpdatedAt = releasedAt
	if _, err := client.PromoCode.UpdateOneID(item.ID).
		SetUsedCount(item.UsedCount).
		SetStatus(string(item.Status)).
		SetUpdatedAt(releasedAt).
		Save(ctx); err != nil {
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	metadata := cloneMap(row.MetadataJSON)
	metadata["released"] = true
	metadata["released_at"] = releasedAt.Format(time.RFC3339Nano)
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		metadata["release_reason"] = reason
	}
	updated, err := client.UserPromoCodeApplication.UpdateOneID(row.ID).
		SetMetadataJSON(metadata).
		SetUpdatedAt(releasedAt).
		Save(ctx)
	if err != nil {
		return admincontrolcontract.PromoCodeApplication{}, false, err
	}
	return ToPromoCodeApplication(updated), true, nil
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

// findPromoCodeByCode loads a single promo code row by its normalized code and
// applies derived (expiry/exhaustion) status. ErrNotFound is returned when no
// row matches. Callers that mutate the row should run inside a serializable
// transaction.
func findPromoCodeByCode(ctx context.Context, client *ent.Client, code string, now time.Time) (admincontrolcontract.PromoCode, *ent.PromoCode, error) {
	row, err := client.PromoCode.Query().Where(entpromocode.CodeEQ(normalizeCode(code))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.PromoCode{}, nil, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.PromoCode{}, nil, err
	}
	return promoCodeWithDerivedStatus(toPromoCode(row), now), row, nil
}

func previewPromoCode(item admincontrolcontract.PromoCode, userID int, amount string, currency string, now time.Time, userUses int) (admincontrolcontract.PromoCodeApplication, error) {
	item = promoCodeWithDerivedStatus(item, now)
	if item.Status != admincontrolcontract.PromoCodeStatusActive || item.MaxUses <= 0 || item.UsedCount >= item.MaxUses {
		return admincontrolcontract.PromoCodeApplication{}, admincontrolcontract.ErrConflict
	}
	if item.PerUserLimit > 0 && userUses >= item.PerUserLimit {
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
	if minOrderAmount, ok := money.RequiredDecimalRat(item.MinOrderAmount); ok && minOrderAmount.Sign() > 0 && inputAmount.Cmp(minOrderAmount) < 0 {
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

func countActivePromoUses(ctx context.Context, client *ent.Client, promoCodeID int, userID int) (int, error) {
	rows, err := client.UserPromoCodeApplication.Query().
		Where(
			entuserpromocodeapplication.PromoCodeIDEQ(promoCodeID),
			entuserpromocodeapplication.UserIDEQ(userID),
		).
		All(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		if !promoApplicationReleased(row.MetadataJSON) {
			count++
		}
	}
	return count, nil
}

func promoApplicationReleased(metadata map[string]any) bool {
	value, ok := metadata["released"].(bool)
	return ok && value
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
		Metadata:       cloneMap(row.MetadataJSON),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
