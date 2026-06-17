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
	codeNeedle := strings.ToLower(strings.TrimSpace(opts.Code))
	items := make([]admincontrol.RedeemCode, 0, len(stored))
	for _, item := range stored {
		item = redeemCodeWithDerivedStatus(item, now)
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		if codeNeedle != "" && !strings.Contains(strings.ToLower(item.Code), codeNeedle) {
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

// redeemBatchDisableNoteMaxLen caps the free-text audit note for bulk-disable.
// Kept short enough to comfortably fit in a database varchar and to survive a
// roundtrip in audit/notification payloads without truncation surprises.
const redeemBatchDisableNoteMaxLen = 500

// BatchDisableRedeemCodes is the operator-facing bulk soft-disable. Accepts
// an optional free-text audit note (validated ≤500 chars) and returns a
// per-id reason map + aggregate breakdown so the admin UI can show operators
// *why* the call did / didn't change each row.
//
// Success/failure accounting:
//   - succeeded == rows that flipped to disabled this call (admin_action or
//     expired — both ended up in disabled status with disabled_reason set)
//   - failed    == rows that did NOT change (already_disabled or not_found)
//
// "expired" counts as succeeded because the store does perform the disable
// write — the reason tag is just operator feedback that the expiry was the
// reason, not a fresh policy decision.
func (s *Service) BatchDisableRedeemCodes(ctx context.Context, ids []int, note string, actorUserID int) (admincontrol.BatchOperationResult, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return admincontrol.BatchOperationResult{}, admincontrol.ErrInvalidInput
	}
	note = strings.TrimSpace(note)
	if len(note) > redeemBatchDisableNoteMaxLen {
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
	reasons, err := s.store.DisableRedeemCodes(ctx, requested, note, s.clock.Now())
	if err != nil {
		return admincontrol.BatchOperationResult{}, err
	}
	breakdown := map[string]int{}
	failedIDs := make([]int, 0)
	succeeded := 0
	for _, id := range requested {
		reason, ok := reasons[id]
		if !ok {
			// Defensive: store didn't classify this id. Treat as not_found so
			// the breakdown is internally consistent.
			reason = admincontrol.RedeemDisabledReasonNotFound
			reasons[id] = reason
		}
		breakdown[reason]++
		switch reason {
		case admincontrol.RedeemDisabledReasonAdminAction, admincontrol.RedeemDisabledReasonExpired:
			succeeded++
		default:
			failedIDs = append(failedIDs, id)
		}
	}
	sort.Ints(failedIDs)
	return admincontrol.BatchOperationResult{
		Requested:               len(ids),
		Succeeded:               succeeded,
		Failed:                  len(failedIDs),
		FailedIDs:               failedIDs,
		PerItemReasons:          reasons,
		DisabledReasonBreakdown: breakdown,
	}, nil
}

// BatchEnableRedeemCodes flips selected disabled codes back to active. Codes
// that aren't disabled (active / redeemed / expired) report as failures so the
// admin sees which rows weren't touched. Mirrors BatchDisable's accounting.
func (s *Service) BatchEnableRedeemCodes(ctx context.Context, ids []int, actorUserID int) (admincontrol.BatchOperationResult, error) {
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
	succeededIDs, err := s.store.EnableRedeemCodes(ctx, requested, s.clock.Now())
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

// BatchExtendRedeemCodes sets a new ExpiresAt on each listed code. Codes whose
// lifecycle is over (fully consumed) are reported as failures in the result so
// the admin sees exactly which rows weren't touched. Mirrors the BatchDisable
// shape — same input dedup + same result accounting.
func (s *Service) BatchExtendRedeemCodes(ctx context.Context, ids []int, expiresAt time.Time, actorUserID int) (admincontrol.BatchOperationResult, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return admincontrol.BatchOperationResult{}, admincontrol.ErrInvalidInput
	}
	if expiresAt.IsZero() {
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
	succeededIDs, err := s.store.ExtendRedeemCodes(ctx, requested, expiresAt, s.clock.Now())
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

// BatchUpdateRedeemCodes applies per-row partial updates to N redeem codes
// in one call. Verbatim port of sub2api's RedeemService.BatchUpdate (which
// shared the partial-update payload across the whole batch); srapi's shape
// is per-row partial because the task spec was explicit about that.
//
// Per-row semantics:
//   - NotFound is idempotent — counts as success (matches the other batch ops).
//   - Sub2api's "core-field updates on already-redeemed codes are rejected"
//     gate is enforced here: a row whose Status is already "redeemed" surfaces
//     an Error.
//   - At least one non-nil field per row (HasChanges); a row with no changes
//     surfaces "no fields to update".
//
// Outer error guards: empty / > 1000 items returns ErrInvalidInput. Per-row
// store / validation failures stay in the result slice (best-effort).
func (s *Service) BatchUpdateRedeemCodes(ctx context.Context, items []admincontrol.BatchUpdateRedeemCodeItem, actorUserID int) ([]admincontrol.BatchUpdateRedeemCodeResult, error) {
	if len(items) == 0 || len(items) > 1000 {
		return nil, admincontrol.ErrInvalidInput
	}
	results := make([]admincontrol.BatchUpdateRedeemCodeResult, 0, len(items))
	seen := make(map[int]struct{}, len(items))
	now := s.clock.Now()
	for i, item := range items {
		row := admincontrol.BatchUpdateRedeemCodeResult{Index: i, ID: item.ID}
		if item.ID <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[item.ID]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[item.ID] = struct{}{}
		// HasChanges check: at least one field must be set so a row of nothing
		// surfaces as an explicit no-op-with-error rather than a silent skip.
		if item.Value == nil && item.MaxRedemptions == nil && !item.ExpiresAtSet && item.Note == nil {
			row.Error = "no fields to update"
			results = append(results, row)
			continue
		}
		// Per-row value validation. The amount (Value) must be a positive
		// decimal when set, mirroring CreateRedeemCode's validRedeemCodeValue
		// gate.
		if item.Value != nil {
			amount, ok := new(big.Rat).SetString(strings.TrimSpace(*item.Value))
			if !ok || amount.Sign() <= 0 {
				row.Error = "invalid amount"
				results = append(results, row)
				continue
			}
		}
		if item.MaxRedemptions != nil && *item.MaxRedemptions <= 0 {
			row.Error = "max_redemptions must be > 0"
			results = append(results, row)
			continue
		}
		if item.ExpiresAtSet && item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
			row.Error = "expires_at must be in the future"
			results = append(results, row)
			continue
		}
		// Sub2api gate: reject core-field updates on already-redeemed codes
		// (value mutation on a consumed code would corrupt accounting). srapi
		// extends this to the whole row — any change on a redeemed code is
		// rejected to mirror sub2api's TouchesUsedSensitiveFields check.
		// NotFound is treated as idempotent success.
		existing, err := s.findRedeemCodeForUpdate(ctx, item.ID)
		if err != nil {
			if err == admincontrol.ErrNotFound {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		if existing.Status == admincontrol.RedeemCodeStatusRedeemed {
			row.Error = "cannot update an already-redeemed code"
			results = append(results, row)
			continue
		}
		fields := admincontrol.RedeemCodeFieldUpdate{
			Value:          item.Value,
			MaxRedemptions: item.MaxRedemptions,
			ExpiresAtSet:   item.ExpiresAtSet,
			ExpiresAt:      item.ExpiresAt,
			Note:           item.Note,
		}
		if _, err := s.store.UpdateRedeemCodeFields(ctx, item.ID, fields, now); err != nil {
			// Idempotent NotFound (handles the rare race where the row was
			// deleted between findRedeemCodeForUpdate and the write).
			if err == admincontrol.ErrNotFound {
				results = append(results, row)
				continue
			}
			row.Error = err.Error()
			results = append(results, row)
			continue
		}
		results = append(results, row)
	}
	return results, nil
}

// findRedeemCodeForUpdate looks up a redeem code by id. Used by
// BatchUpdateRedeemCodes to gate updates on Status (and any other future
// pre-write checks). Returns ErrNotFound for a missing id so the batch caller
// can treat NotFound as idempotent.
func (s *Service) findRedeemCodeForUpdate(ctx context.Context, id int) (admincontrol.RedeemCode, error) {
	stored, err := s.store.ListRedeemCodes(ctx)
	if err != nil {
		return admincontrol.RedeemCode{}, err
	}
	for _, item := range stored {
		if item.ID == id {
			return item, nil
		}
	}
	return admincontrol.RedeemCode{}, admincontrol.ErrNotFound
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

// BatchDeleteRedeemCodes hard-deletes the named codes. Loops over the
// per-id DeleteRedeemCode so a missing/already-deleted id surfaces in
// FailedIDs without failing the whole call. Same {ids,bulk}-shape as
// BatchDisableRedeemCodes so the admin UI can swap mutations without
// reshaping the result.
func (s *Service) BatchDeleteRedeemCodes(ctx context.Context, ids []int, actorUserID int) (admincontrol.BatchOperationResult, error) {
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
	succeeded := 0
	failedIDs := make([]int, 0)
	for _, id := range requested {
		if _, err := s.store.DeleteRedeemCode(ctx, id); err != nil {
			failedIDs = append(failedIDs, id)
			continue
		}
		succeeded++
	}
	sort.Ints(failedIDs)
	return admincontrol.BatchOperationResult{
		Requested: len(ids),
		Succeeded: succeeded,
		Failed:    len(failedIDs),
		FailedIDs: failedIDs,
	}, nil
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
