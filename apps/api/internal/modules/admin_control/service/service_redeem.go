package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

func (s *Service) ListRedeemCodes(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.RedeemCodeList, error) {
	stored, err := s.store.ListRedeemCodes(ctx)
	if err != nil {
		return admincontrol.RedeemCodeList{}, err
	}
	now := s.clock.Now()
	items := make([]admincontrol.RedeemCode, 0, len(stored))
	for _, item := range stored {
		item = redeemCodeWithDerivedStatus(item, now)
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.RedeemCodeList{Items: pageItems(items, opts), Total: len(items)}, nil
}

func (s *Service) CreateRedeemCode(ctx context.Context, req admincontrol.CreateRedeemCodeRequest, actorUserID int) (admincontrol.RedeemCode, error) {
	code, err := redeemCodeFromCreateRequest(req, s.clock.Now())
	if err != nil {
		return admincontrol.RedeemCode{}, err
	}
	return s.store.CreateRedeemCode(ctx, code)
}

func (s *Service) BatchGenerateRedeemCodes(ctx context.Context, req admincontrol.BatchGenerateRedeemCodesRequest, actorUserID int) ([]admincontrol.RedeemCode, error) {
	if req.Count <= 0 || req.Count > 1000 {
		return nil, admincontrol.ErrInvalidInput
	}
	now := s.clock.Now()
	created := make([]admincontrol.RedeemCode, 0, req.Count)
	generated := map[string]bool{}
	for len(created) < req.Count {
		generatedCode, err := randomCode(defaultString(req.Prefix, "SR"))
		if err != nil {
			return nil, err
		}
		if generated[normalizeCode(generatedCode)] {
			continue
		}
		code, err := redeemCodeFromBatchRequest(req, generatedCode, now)
		if err != nil {
			return nil, err
		}
		stored, err := s.store.CreateRedeemCode(ctx, code)
		if err != nil {
			// Collisions with already-persisted codes are skipped, matching the
			// previous behavior of generating a unique batch.
			if err == admincontrol.ErrConflict {
				continue
			}
			return nil, err
		}
		generated[normalizeCode(stored.Code)] = true
		created = append(created, stored)
	}
	return created, nil
}

func (s *Service) BatchDisableRedeemCodes(ctx context.Context, ids []int, actorUserID int) (admincontrol.BatchOperationResult, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return admincontrol.BatchOperationResult{}, admincontrol.ErrInvalidInput
	}
	requested := make([]int, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if id <= 0 {
			return admincontrol.BatchOperationResult{}, admincontrol.ErrInvalidInput
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		requested = append(requested, id)
	}
	succeededIDs, err := s.store.DisableRedeemCodes(ctx, requested, s.clock.Now())
	if err != nil {
		return admincontrol.BatchOperationResult{}, err
	}
	succeededSet := map[int]bool{}
	for _, id := range succeededIDs {
		succeededSet[id] = true
	}
	failedIDs := make([]int, 0, len(requested))
	for _, id := range requested {
		if !succeededSet[id] {
			failedIDs = append(failedIDs, id)
		}
	}
	sort.Ints(failedIDs)
	return admincontrol.BatchOperationResult{
		Requested: len(ids),
		Succeeded: len(succeededIDs),
		Failed:    len(failedIDs),
		FailedIDs: failedIDs,
	}, nil
}

func (s *Service) RedeemCodeStats(ctx context.Context) (admincontrol.RedeemCodeStats, error) {
	stored, err := s.store.ListRedeemCodes(ctx)
	if err != nil {
		return admincontrol.RedeemCodeStats{}, err
	}
	now := s.clock.Now()
	stats := admincontrol.RedeemCodeStats{Total: len(stored)}
	for _, item := range stored {
		switch redeemCodeWithDerivedStatus(item, now).Status {
		case admincontrol.RedeemCodeStatusActive:
			stats.Active++
		case admincontrol.RedeemCodeStatusRedeemed:
			stats.Redeemed++
		case admincontrol.RedeemCodeStatusDisabled:
			stats.Disabled++
		case admincontrol.RedeemCodeStatusExpired:
			stats.Expired++
		}
	}
	return stats, nil
}

func (s *Service) DeleteRedeemCode(ctx context.Context, id int, actorUserID int) (admincontrol.RedeemCode, error) {
	if id <= 0 {
		return admincontrol.RedeemCode{}, admincontrol.ErrNotFound
	}
	return s.store.DeleteRedeemCode(ctx, id)
}

func (s *Service) RedeemCode(ctx context.Context, user userscontract.User, req admincontrol.RedeemCodeRedemptionRequest) (admincontrol.RedeemCodeRedemptionResult, error) {
	if user.ID <= 0 {
		return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrInvalidInput
	}
	code := normalizeCode(req.Code)
	if code == "" {
		return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrInvalidInput
	}
	return s.store.RedeemCode(ctx, admincontrol.RedeemCodeRedemptionInput{
		UserID:     user.ID,
		Code:       code,
		RedeemedAt: s.clock.Now(),
	})
}

func redeemCodeFromCreateRequest(req admincontrol.CreateRedeemCodeRequest, now time.Time) (admincontrol.RedeemCode, error) {
	if !req.Type.Valid() || strings.TrimSpace(req.Code) == "" || !validRedeemCodeValue(req.Type, req.Value) {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	maxRedemptions := req.MaxRedemptions
	if maxRedemptions == 0 {
		maxRedemptions = 1
	}
	if maxRedemptions <= 0 {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	return admincontrol.RedeemCode{
		Code:           normalizeCode(req.Code),
		Type:           req.Type,
		Status:         admincontrol.RedeemCodeStatusActive,
		Value:          strings.TrimSpace(req.Value),
		Currency:       normalizeCurrency(req.Currency),
		MaxRedemptions: maxRedemptions,
		RedeemedCount:  0,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func redeemCodeFromBatchRequest(req admincontrol.BatchGenerateRedeemCodesRequest, code string, now time.Time) (admincontrol.RedeemCode, error) {
	if !req.Type.Valid() || !validRedeemCodeValue(req.Type, req.Value) {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	maxRedemptions := req.MaxRedemptions
	if maxRedemptions == 0 {
		maxRedemptions = 1
	}
	if maxRedemptions <= 0 {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	return admincontrol.RedeemCode{
		Code:           normalizeCode(code),
		Type:           req.Type,
		Status:         admincontrol.RedeemCodeStatusActive,
		Value:          strings.TrimSpace(req.Value),
		Currency:       normalizeCurrency(req.Currency),
		MaxRedemptions: maxRedemptions,
		RedeemedCount:  0,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func validRedeemCodeValue(codeType admincontrol.RedeemCodeType, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	switch codeType {
	case admincontrol.RedeemCodeTypeBalance:
		amount, ok := new(big.Rat).SetString(value)
		return ok && amount.Sign() > 0
	case admincontrol.RedeemCodeTypeSubscription:
		planID, err := strconv.Atoi(value)
		return err == nil && planID > 0
	default:
		return false
	}
}

func redeemCodeWithDerivedStatus(item admincontrol.RedeemCode, now time.Time) admincontrol.RedeemCode {
	if item.Status == admincontrol.RedeemCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.RedeemCodeStatusExpired
	}
	return item
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func randomCode(prefix string) (string, error) {
	prefix = normalizeCode(prefix)
	if prefix == "" {
		prefix = "SR"
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "-" + strings.ToUpper(hex.EncodeToString(buf)), nil
}
