package service

import (
	"context"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

const settingsKeyPromoCodes = "admin_control.promo_codes"

func (s *Service) ListPromoCodes(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.PromoCodeList, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCodeList{}, err
	}
	now := s.clock.Now()
	items := make([]admincontrol.PromoCode, 0, len(collection.Items))
	for _, item := range collection.Items {
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
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCode{}, err
	}
	item, err := promoCodeFromRequest(req, nextID(collection.NextID, len(collection.Items)), s.clock.Now(), nil)
	if err != nil {
		return admincontrol.PromoCode{}, err
	}
	if promoCodeExists(collection.Items, item.Code) {
		return admincontrol.PromoCode{}, admincontrol.ErrConflict
	}
	collection.Items = append(collection.Items, item)
	collection.NextID = item.ID + 1
	if err := s.saveTyped(ctx, settingsKeyPromoCodes, collection, actorUserID); err != nil {
		return admincontrol.PromoCode{}, err
	}
	return item, nil
}

func (s *Service) UpdatePromoCode(ctx context.Context, id int, req admincontrol.PromoCodeRequest, actorUserID int) (admincontrol.PromoCode, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCode{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		updated, err := promoCodeFromRequest(req, id, s.clock.Now(), &item)
		if err != nil {
			return admincontrol.PromoCode{}, err
		}
		if !strings.EqualFold(item.Code, updated.Code) && promoCodeExists(collection.Items, updated.Code) {
			return admincontrol.PromoCode{}, admincontrol.ErrConflict
		}
		collection.Items[idx] = updated
		if err := s.saveTyped(ctx, settingsKeyPromoCodes, collection, actorUserID); err != nil {
			return admincontrol.PromoCode{}, err
		}
		return updated, nil
	}
	return admincontrol.PromoCode{}, admincontrol.ErrNotFound
}

func (s *Service) DeletePromoCode(ctx context.Context, id int, actorUserID int) (admincontrol.PromoCode, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCode{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		collection.Items = append(collection.Items[:idx], collection.Items[idx+1:]...)
		if err := s.saveTyped(ctx, settingsKeyPromoCodes, collection, actorUserID); err != nil {
			return admincontrol.PromoCode{}, err
		}
		return item, nil
	}
	return admincontrol.PromoCode{}, admincontrol.ErrNotFound
}

type promoCodeCollection struct {
	NextID int                      `json:"next_id"`
	Items  []admincontrol.PromoCode `json:"items"`
}

func promoCodeFromRequest(req admincontrol.PromoCodeRequest, id int, now time.Time, existing *admincontrol.PromoCode) (admincontrol.PromoCode, error) {
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
		ID:             id,
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

func promoCodeExists(items []admincontrol.PromoCode, code string) bool {
	code = normalizeCode(code)
	for _, item := range items {
		if normalizeCode(item.Code) == code {
			return true
		}
	}
	return false
}

func promoCodeWithDerivedStatus(item admincontrol.PromoCode, now time.Time) admincontrol.PromoCode {
	if item.Status == admincontrol.PromoCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.PromoCodeStatusExpired
	}
	return item
}
