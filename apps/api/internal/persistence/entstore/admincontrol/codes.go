package admincontrol

import (
	"context"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	entpromocode "github.com/srapi/srapi/apps/api/ent/promocode"
	entredeemcode "github.com/srapi/srapi/apps/api/ent/redeemcode"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

// ListRedeemCodes returns every redeem code row. The admin_control service
// owns status derivation, filtering, sorting, and paging.
func (s *Store) ListRedeemCodes(ctx context.Context) ([]admincontrolcontract.RedeemCode, error) {
	if s == nil || s.client == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	rows, err := s.client.RedeemCode.Query().
		Order(entredeemcode.ByCreatedAt(entsql.OrderDesc()), entredeemcode.ByID(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]admincontrolcontract.RedeemCode, 0, len(rows))
	for _, row := range rows {
		items = append(items, toRedeemCode(row))
	}
	return items, nil
}

func (s *Store) CreateRedeemCode(ctx context.Context, code admincontrolcontract.RedeemCode) (admincontrolcontract.RedeemCode, error) {
	if s == nil || s.client == nil {
		return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrInvalidInput
	}
	create := s.client.RedeemCode.Create().
		SetCode(normalizeCode(code.Code)).
		SetType(string(code.Type)).
		SetStatus(string(code.Status)).
		SetValue(code.Value).
		SetCurrency(normalizeCurrency(code.Currency)).
		SetMaxRedemptions(code.MaxRedemptions).
		SetRedeemedCount(code.RedeemedCount).
		SetNillableExpiresAt(code.ExpiresAt).
		SetNote(code.Note).
		SetDisabledReason(code.DisabledReason)
	if !code.CreatedAt.IsZero() {
		create.SetCreatedAt(code.CreatedAt)
	}
	if !code.UpdatedAt.IsZero() {
		create.SetUpdatedAt(code.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrConflict
		}
		return admincontrolcontract.RedeemCode{}, err
	}
	return toRedeemCode(row), nil
}

func (s *Store) DeleteRedeemCode(ctx context.Context, id int) (admincontrolcontract.RedeemCode, error) {
	if s == nil || s.client == nil || id <= 0 {
		return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrNotFound
	}
	row, err := s.client.RedeemCode.Query().Where(entredeemcode.IDEQ(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.RedeemCode{}, err
	}
	if err := s.client.RedeemCode.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.RedeemCode{}, err
	}
	return toRedeemCode(row), nil
}

// DisableRedeemCodes is the bulk soft-disable. Pre-fetches each row so it can
// classify the outcome per-id; the service then aggregates this into
// per_item_reasons + disabled_reason_breakdown for the admin UI.
//
// Reasons:
//   - "not_found"          id not present
//   - "already_disabled"   row already has status=disabled; nothing changes
//   - "expired"            row has expires_at in the past; still disabled but
//     tagged with disabled_reason=expired so the operator
//     knows why it counts as "no-op-ish"
//   - "admin_action"       normal operator-driven disable
//
// The note is written onto every row we actually flip (not rows we skip).
// Two writes at most: one UPDATE per non-empty reason bucket.
func (s *Store) DisableRedeemCodes(ctx context.Context, ids []int, note string, at time.Time) (map[int]string, error) {
	if s == nil || s.client == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	requested := uniquePositiveInts(ids)
	if len(requested) == 0 {
		return map[int]string{}, nil
	}
	rows, err := s.client.RedeemCode.Query().
		Where(entredeemcode.IDIn(requested...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]*ent.RedeemCode, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	reasons := make(map[int]string, len(requested))
	toUpdate := map[string][]int{}
	for _, id := range requested {
		row, ok := byID[id]
		if !ok {
			reasons[id] = admincontrolcontract.RedeemDisabledReasonNotFound
			continue
		}
		if row.Status == string(admincontrolcontract.RedeemCodeStatusDisabled) {
			reasons[id] = admincontrolcontract.RedeemDisabledReasonAlreadyDisabled
			continue
		}
		reason := admincontrolcontract.RedeemDisabledReasonAdminAction
		if row.ExpiresAt != nil && row.ExpiresAt.Before(now) {
			reason = admincontrolcontract.RedeemDisabledReasonExpired
		}
		reasons[id] = reason
		toUpdate[reason] = append(toUpdate[reason], id)
	}
	for reason, idsForReason := range toUpdate {
		if len(idsForReason) == 0 {
			continue
		}
		if _, err := s.client.RedeemCode.Update().
			Where(entredeemcode.IDIn(idsForReason...)).
			SetStatus(string(admincontrolcontract.RedeemCodeStatusDisabled)).
			SetNote(note).
			SetDisabledReason(reason).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return nil, err
		}
	}
	return reasons, nil
}

// EnableRedeemCodes is the inverse of DisableRedeemCodes — flips DISABLED rows
// back to ACTIVE. Rows in any other status are skipped (we don't reanimate
// redeemed/expired codes via the bulk action).
func (s *Store) EnableRedeemCodes(ctx context.Context, ids []int, at time.Time) ([]int, error) {
	if s == nil || s.client == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	requested := uniquePositiveInts(ids)
	if len(requested) == 0 {
		return []int{}, nil
	}
	eligibleIDs, err := s.client.RedeemCode.Query().
		Where(
			entredeemcode.IDIn(requested...),
			entredeemcode.StatusEQ(string(admincontrolcontract.RedeemCodeStatusDisabled)),
		).
		IDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(eligibleIDs) == 0 {
		return []int{}, nil
	}
	if _, err := s.client.RedeemCode.Update().
		Where(entredeemcode.IDIn(eligibleIDs...)).
		SetStatus(string(admincontrolcontract.RedeemCodeStatusActive)).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return nil, err
	}
	return eligibleIDs, nil
}

// ExtendRedeemCodes sets a new ExpiresAt on the named codes. Mirrors the
// memory store: fully-consumed codes (redeemed_count >= max_redemptions) are
// skipped so the result accurately reports rows that were actually touched.
func (s *Store) ExtendRedeemCodes(ctx context.Context, ids []int, expiresAt time.Time, now time.Time) ([]int, error) {
	if s == nil || s.client == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	stamp := now.UTC()
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	}
	requested := uniquePositiveInts(ids)
	if len(requested) == 0 {
		return []int{}, nil
	}
	rows, err := s.client.RedeemCode.Query().
		Where(entredeemcode.IDIn(requested...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	eligible := make([]int, 0, len(rows))
	for _, row := range rows {
		// Skip codes that have already hit their redemption cap — extending
		// their expiry would have no effect. Codes whose status is "redeemed"
		// or "disabled" are still touched (operators may legitimately want
		// to push back the expiry on a disabled code before re-enabling it).
		if row.MaxRedemptions > 0 && row.RedeemedCount >= row.MaxRedemptions {
			continue
		}
		eligible = append(eligible, row.ID)
	}
	if len(eligible) == 0 {
		return []int{}, nil
	}
	want := expiresAt.UTC()
	if _, err := s.client.RedeemCode.Update().
		Where(entredeemcode.IDIn(eligible...)).
		SetExpiresAt(want).
		SetUpdatedAt(stamp).
		Save(ctx); err != nil {
		return nil, err
	}
	return eligible, nil
}

// UpdateRedeemCodeFields applies a partial update to one redeem code row.
// Each non-nil field on RedeemCodeFieldUpdate is set; nil fields are left
// alone. ExpiresAtSet=true + ExpiresAt=nil clears the expiry (uses Ent's
// ClearExpiresAt). Returns ErrNotFound when the row doesn't exist.
//
// The service's BatchUpdateRedeemCodes wraps this for per-row best-effort
// semantics + already-redeemed gating. The store performs the write
// unconditionally on any row it finds, mirroring the other partial-update
// helpers in this package (UpdatePromoCode etc.).
func (s *Store) UpdateRedeemCodeFields(ctx context.Context, id int, fields admincontrolcontract.RedeemCodeFieldUpdate, now time.Time) (admincontrolcontract.RedeemCode, error) {
	if s == nil || s.client == nil || id <= 0 {
		return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrNotFound
	}
	stamp := now.UTC()
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	}
	update := s.client.RedeemCode.UpdateOneID(id).SetUpdatedAt(stamp)
	if fields.Value != nil {
		update.SetValue(*fields.Value)
	}
	if fields.MaxRedemptions != nil {
		update.SetMaxRedemptions(*fields.MaxRedemptions)
	}
	if fields.ExpiresAtSet {
		if fields.ExpiresAt == nil {
			update.ClearExpiresAt()
		} else {
			update.SetExpiresAt(fields.ExpiresAt.UTC())
		}
	}
	if fields.Note != nil {
		update.SetNote(*fields.Note)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.RedeemCode{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.RedeemCode{}, err
	}
	return toRedeemCode(row), nil
}

// ListPromoCodes returns every promo code row. The admin_control service owns
// status derivation, filtering, sorting, and paging.
func (s *Store) ListPromoCodes(ctx context.Context) ([]admincontrolcontract.PromoCode, error) {
	if s == nil || s.client == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	rows, err := s.client.PromoCode.Query().
		Order(entpromocode.ByCreatedAt(entsql.OrderDesc()), entpromocode.ByID(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]admincontrolcontract.PromoCode, 0, len(rows))
	for _, row := range rows {
		items = append(items, toPromoCode(row))
	}
	return items, nil
}

func (s *Store) CreatePromoCode(ctx context.Context, code admincontrolcontract.PromoCode) (admincontrolcontract.PromoCode, error) {
	if s == nil || s.client == nil {
		return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrInvalidInput
	}
	create := s.client.PromoCode.Create().
		SetCode(normalizeCode(code.Code)).
		SetStatus(string(code.Status)).
		SetDiscountType(string(code.DiscountType)).
		SetDiscountValue(code.DiscountValue).
		SetCurrency(normalizeCurrency(code.Currency)).
		SetMaxUses(code.MaxUses).
		SetPerUserLimit(code.PerUserLimit).
		SetMinOrderAmount(code.MinOrderAmount).
		SetUsedCount(code.UsedCount).
		SetNillableStartsAt(code.StartsAt).
		SetNillableExpiresAt(code.ExpiresAt)
	if !code.CreatedAt.IsZero() {
		create.SetCreatedAt(code.CreatedAt)
	}
	if !code.UpdatedAt.IsZero() {
		create.SetUpdatedAt(code.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrConflict
		}
		return admincontrolcontract.PromoCode{}, err
	}
	return toPromoCode(row), nil
}

func (s *Store) UpdatePromoCode(ctx context.Context, code admincontrolcontract.PromoCode) (admincontrolcontract.PromoCode, error) {
	if s == nil || s.client == nil || code.ID <= 0 {
		return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrNotFound
	}
	update := s.client.PromoCode.UpdateOneID(code.ID).
		SetCode(normalizeCode(code.Code)).
		SetStatus(string(code.Status)).
		SetDiscountType(string(code.DiscountType)).
		SetDiscountValue(code.DiscountValue).
		SetCurrency(normalizeCurrency(code.Currency)).
		SetMaxUses(code.MaxUses).
		SetPerUserLimit(code.PerUserLimit).
		SetMinOrderAmount(code.MinOrderAmount).
		SetUsedCount(code.UsedCount).
		ClearStartsAt().
		ClearExpiresAt().
		SetNillableStartsAt(code.StartsAt).
		SetNillableExpiresAt(code.ExpiresAt)
	if !code.UpdatedAt.IsZero() {
		update.SetUpdatedAt(code.UpdatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrNotFound
		}
		if ent.IsConstraintError(err) {
			return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrConflict
		}
		return admincontrolcontract.PromoCode{}, err
	}
	return toPromoCode(row), nil
}

func (s *Store) DeletePromoCode(ctx context.Context, id int) (admincontrolcontract.PromoCode, error) {
	if s == nil || s.client == nil || id <= 0 {
		return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrNotFound
	}
	row, err := s.client.PromoCode.Query().Where(entpromocode.IDEQ(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.PromoCode{}, err
	}
	if err := s.client.PromoCode.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return admincontrolcontract.PromoCode{}, admincontrolcontract.ErrNotFound
		}
		return admincontrolcontract.PromoCode{}, err
	}
	return toPromoCode(row), nil
}

func toRedeemCode(row *ent.RedeemCode) admincontrolcontract.RedeemCode {
	if row == nil {
		return admincontrolcontract.RedeemCode{}
	}
	return admincontrolcontract.RedeemCode{
		ID:             row.ID,
		Code:           row.Code,
		Type:           admincontrolcontract.RedeemCodeType(row.Type),
		Status:         admincontrolcontract.RedeemCodeStatus(row.Status),
		Value:          row.Value,
		Currency:       row.Currency,
		MaxRedemptions: row.MaxRedemptions,
		RedeemedCount:  row.RedeemedCount,
		ExpiresAt:      cloneTimePtr(row.ExpiresAt),
		Note:           row.Note,
		DisabledReason: row.DisabledReason,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func toPromoCode(row *ent.PromoCode) admincontrolcontract.PromoCode {
	if row == nil {
		return admincontrolcontract.PromoCode{}
	}
	return admincontrolcontract.PromoCode{
		ID:             row.ID,
		Code:           row.Code,
		Status:         admincontrolcontract.PromoCodeStatus(row.Status),
		DiscountType:   admincontrolcontract.PromoDiscountType(row.DiscountType),
		DiscountValue:  row.DiscountValue,
		Currency:       row.Currency,
		MaxUses:        row.MaxUses,
		PerUserLimit:   row.PerUserLimit,
		MinOrderAmount: row.MinOrderAmount,
		UsedCount:      row.UsedCount,
		StartsAt:       cloneTimePtr(row.StartsAt),
		ExpiresAt:      cloneTimePtr(row.ExpiresAt),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
