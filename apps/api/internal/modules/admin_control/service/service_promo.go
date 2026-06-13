package service

import (
	"context"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

func (s *Service) ListPromoCodes(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.PromoCodeList, error) {
	stored, err := s.store.ListPromoCodes(ctx)
	if err != nil {
		return admincontrol.PromoCodeList{}, err
	}
	now := s.clock.Now()
	items := make([]admincontrol.PromoCode, 0, len(stored))
	for _, item := range stored {
		item = promoCodeWithDerivedStatus(item, now)
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.PromoCodeList{Items: pageItems(items, opts), Total: len(items)}, nil
}

// ListPromoCodeUsages returns the redemption history for one promo code.
func (s *Service) ListPromoCodeUsages(ctx context.Context, promoCodeID, limit int) ([]admincontrol.PromoCodeApplication, error) {
	if promoCodeID <= 0 {
		return nil, admincontrol.ErrInvalidInput
	}
	return s.store.ListPromoCodeUsages(ctx, promoCodeID, limit)
}

func (s *Service) CreatePromoCode(ctx context.Context, req admincontrol.PromoCodeRequest, actorUserID int) (admincontrol.PromoCode, error) {
	item, err := promoCodeFromRequest(req, s.clock.Now(), nil)
	if err != nil {
		return admincontrol.PromoCode{}, err
	}
	return s.store.CreatePromoCode(ctx, item)
}

func (s *Service) UpdatePromoCode(ctx context.Context, id int, req admincontrol.PromoCodeRequest, actorUserID int) (admincontrol.PromoCode, error) {
	if id <= 0 {
		return admincontrol.PromoCode{}, admincontrol.ErrNotFound
	}
	stored, err := s.store.ListPromoCodes(ctx)
	if err != nil {
		return admincontrol.PromoCode{}, err
	}
	var existing *admincontrol.PromoCode
	for idx := range stored {
		if stored[idx].ID == id {
			existing = &stored[idx]
			break
		}
	}
	if existing == nil {
		return admincontrol.PromoCode{}, admincontrol.ErrNotFound
	}
	updated, err := promoCodeFromRequest(req, s.clock.Now(), existing)
	if err != nil {
		return admincontrol.PromoCode{}, err
	}
	updated.ID = id
	return s.store.UpdatePromoCode(ctx, updated)
}

func (s *Service) DeletePromoCode(ctx context.Context, id int, actorUserID int) (admincontrol.PromoCode, error) {
	if id <= 0 {
		return admincontrol.PromoCode{}, admincontrol.ErrNotFound
	}
	return s.store.DeletePromoCode(ctx, id)
}

func promoCodeFromRequest(req admincontrol.PromoCodeRequest, now time.Time, existing *admincontrol.PromoCode) (admincontrol.PromoCode, error) {
	if strings.TrimSpace(req.Code) == "" || !req.DiscountType.Valid() || !validDecimal(req.DiscountValue) || !validTimeRange(req.StartsAt, req.ExpiresAt) {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	if req.DiscountType == admincontrol.PromoDiscountTypeAmount && !validPositiveDecimal(req.DiscountValue) {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	if req.DiscountType == admincontrol.PromoDiscountTypePercent && !validPercentDecimal(req.DiscountValue) {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	maxUses := req.MaxUses
	if maxUses == 0 {
		maxUses = 1
	}
	if maxUses <= 0 {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	perUserLimit := req.PerUserLimit
	if perUserLimit < 0 || perUserLimit > maxUses {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	minOrderAmount := strings.TrimSpace(req.MinOrderAmount)
	if minOrderAmount != "" && !validPositiveDecimal(minOrderAmount) {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	status := req.Status
	if status == "" {
		status = admincontrol.PromoCodeStatusActive
	}
	if !status.Valid() {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	createdAt := now
	usedCount := 0
	if existing != nil {
		createdAt = existing.CreatedAt
		usedCount = existing.UsedCount
	}
	return admincontrol.PromoCode{
		Code:           normalizeCode(req.Code),
		Status:         status,
		DiscountType:   req.DiscountType,
		DiscountValue:  strings.TrimSpace(req.DiscountValue),
		Currency:       normalizeCurrency(req.Currency),
		MaxUses:        maxUses,
		PerUserLimit:   perUserLimit,
		MinOrderAmount: minOrderAmount,
		UsedCount:      usedCount,
		StartsAt:       req.StartsAt,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}, nil
}

func promoCodeWithDerivedStatus(item admincontrol.PromoCode, now time.Time) admincontrol.PromoCode {
	if item.Status == admincontrol.PromoCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.PromoCodeStatusExpired
	}
	return item
}
