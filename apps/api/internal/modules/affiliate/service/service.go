package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type AuditRecorder interface {
	Record(ctx context.Context, req auditcontract.RecordRequest) (auditcontract.Log, error)
}

type EventEnqueuer interface {
	Enqueue(ctx context.Context, req eventscontract.EnqueueRequest) (eventscontract.OutboxEvent, error)
}

type Dependencies struct {
	Audit  AuditRecorder
	Events EventEnqueuer
}

type Service struct {
	store contract.Store
	deps  Dependencies
	clock Clock

	// rebateOverrideMu guards the rebateOverrides map below. The map is a
	// runtime overlay used by BatchSetUserRebateRate to store per-user rebate
	// rate overrides until srapi grows a persistent user_affiliates table to
	// mirror sub2api's. Values are *float64 by design (nil = explicit clear,
	// non-nil = the per-user percentage). Map presence ≠ override active —
	// only non-nil values participate; absent users fall back to the rule.
	rebateOverrideMu sync.RWMutex
	rebateOverrides  map[int]*float64
}

func New(store contract.Store, deps Dependencies, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, deps: deps, clock: clock, rebateOverrides: make(map[int]*float64)}, nil
}

// BatchSetUserRebateRateMaxItems caps the number of items per
// BatchSetUserRebateRate call (1000 — operator-facing constant matching the
// other batch endpoints).
const BatchSetUserRebateRateMaxItems = 1000

// BatchSetUserRebateRate sets (or clears) the per-user affiliate rebate-rate
// override on N users in one call. Verbatim port of sub2api's
// AffiliateHandler.BatchSetRate (affiliate_handler.go) → AffiliateService.
// AdminBatchSetUserRebateRate → AffiliateRepository.BatchSetUserRebateRate
// (affiliate_repo.go). sub2api persists to the user_affiliates table; srapi
// has no such schema yet, so the override is held in an in-memory overlay
// keyed on user_id. The future rebate-computation path can consult this
// overlay before falling back to the AffiliateRule rate. Per-row failures
// (invalid id, rate out of [0,1] range, duplicate id in batch) surface in
// results[i].Error without aborting the batch.
//
// Outer error is reserved for precondition failures (empty input, > max
// items).
func (s *Service) BatchSetUserRebateRate(ctx context.Context, items []contract.BatchSetUserRebateRateItem) ([]contract.BatchSetUserRebateRateResult, error) {
	if len(items) == 0 {
		return nil, ErrInvalidInput
	}
	if len(items) > BatchSetUserRebateRateMaxItems {
		return nil, ErrInvalidInput
	}
	results := make([]contract.BatchSetUserRebateRateResult, 0, len(items))
	seen := make(map[int]struct{}, len(items))
	s.rebateOverrideMu.Lock()
	defer s.rebateOverrideMu.Unlock()
	if s.rebateOverrides == nil {
		s.rebateOverrides = make(map[int]*float64)
	}
	for i, item := range items {
		row := contract.BatchSetUserRebateRateResult{Index: i, UserID: item.UserID}
		if item.UserID <= 0 {
			row.Error = "invalid id"
			results = append(results, row)
			continue
		}
		if _, dup := seen[item.UserID]; dup {
			row.Error = "duplicate id in batch"
			results = append(results, row)
			continue
		}
		seen[item.UserID] = struct{}{}
		if !item.ClearOverride {
			if item.RatePercent == nil {
				row.Error = "rate_percent is required unless clear=true"
				results = append(results, row)
				continue
			}
			if *item.RatePercent < 0 || *item.RatePercent > 1 {
				row.Error = "rate_percent must be in [0, 1]"
				results = append(results, row)
				continue
			}
			rate := *item.RatePercent
			s.rebateOverrides[item.UserID] = &rate
		} else {
			delete(s.rebateOverrides, item.UserID)
		}
		results = append(results, row)
	}
	return results, nil
}

// UserRebateOverride returns the active per-user rebate override (rate as a
// 0..1 fraction) when one is configured via BatchSetUserRebateRate, or nil
// when no override applies. Cheap thread-safe read for the future
// rebate-computation hot path; today's tests assert it.
func (s *Service) UserRebateOverride(userID int) *float64 {
	s.rebateOverrideMu.RLock()
	defer s.rebateOverrideMu.RUnlock()
	if v, ok := s.rebateOverrides[userID]; ok {
		return v
	}
	return nil
}

func (s *Service) CreateInviteCode(ctx context.Context, req contract.CreateInviteCodeRequest) (contract.InviteCode, error) {
	code := strings.TrimSpace(req.Code)
	if req.UserID <= 0 || code == "" {
		return contract.InviteCode{}, ErrInvalidInput
	}
	now := s.clock.Now()
	return s.store.CreateInviteCode(ctx, contract.InviteCode{
		UserID:    req.UserID,
		Code:      code,
		Status:    contract.InviteCodeStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: cloneTime(req.ExpiresAt),
	})
}

// ValidateInviteCode confirms that a code can be used for a new invite binding.
func (s *Service) ValidateInviteCode(ctx context.Context, code string) (contract.InviteCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return contract.InviteCode{}, ErrInvalidInput
	}
	inviteCode, err := s.store.FindInviteCodeByCode(ctx, code)
	if err != nil {
		return contract.InviteCode{}, err
	}
	now := s.clock.Now()
	if inviteCode.Status != contract.InviteCodeStatusActive || (inviteCode.ExpiresAt != nil && !inviteCode.ExpiresAt.After(now)) {
		return contract.InviteCode{}, ErrInvalidInput
	}
	return inviteCode, nil
}

func (s *Service) BindInvite(ctx context.Context, req contract.BindInviteRequest) (contract.InviteRelationship, error) {
	code := strings.TrimSpace(req.Code)
	if req.InviteeUserID <= 0 || code == "" {
		return contract.InviteRelationship{}, ErrInvalidInput
	}
	inviteCode, err := s.store.FindInviteCodeByCode(ctx, code)
	if err != nil {
		return contract.InviteRelationship{}, err
	}
	now := s.clock.Now()
	if inviteCode.UserID == req.InviteeUserID || inviteCode.Status != contract.InviteCodeStatusActive || (inviteCode.ExpiresAt != nil && !inviteCode.ExpiresAt.After(now)) {
		return contract.InviteRelationship{}, ErrInvalidInput
	}
	relationship, err := s.store.CreateRelationship(ctx, contract.InviteRelationship{
		InviterUserID: inviteCode.UserID,
		InviteeUserID: req.InviteeUserID,
		InviteCodeID:  inviteCode.ID,
		Status:        contract.RelationshipStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return contract.InviteRelationship{}, err
	}
	return relationship, nil
}

// ListInviteCodesByUser returns invite codes generated by one user.
func (s *Service) ListInviteCodesByUser(ctx context.Context, userID int) ([]contract.InviteCode, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListInviteCodesByUser(ctx, userID)
}

func (s *Service) CreateRule(ctx context.Context, req contract.CreateRuleRequest) (contract.AffiliateRule, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || req.TriggerType == "" {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	rate, ok := normalizeMoney(req.Rate)
	if strings.TrimSpace(req.Rate) == "" {
		rate = "0.00000000"
		ok = true
	}
	if !ok {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	fixedAmount, ok := normalizeMoney(req.FixedAmount)
	if strings.TrimSpace(req.FixedAmount) == "" {
		fixedAmount = "0.00000000"
		ok = true
	}
	if !ok {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	if compareMoney(rate, "0.00000000") <= 0 && compareMoney(fixedAmount, "0.00000000") <= 0 {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	maxRebateAmount, ok := normalizeMoney(req.MaxRebateAmount)
	if strings.TrimSpace(req.MaxRebateAmount) == "" {
		maxRebateAmount = "0.00000000"
		ok = true
	}
	if !ok {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	status := contract.RuleStatusActive
	if req.Status != nil {
		if !validRuleStatus(*req.Status) {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		status = *req.Status
	}
	now := s.clock.Now()
	return s.store.CreateRule(ctx, contract.AffiliateRule{
		Name:            name,
		Status:          status,
		TriggerType:     req.TriggerType,
		Rate:            rate,
		FixedAmount:     fixedAmount,
		Currency:        normalizeCurrency(req.Currency),
		MaxRebateAmount: maxRebateAmount,
		ValidFrom:       cloneTime(req.ValidFrom),
		ValidTo:         cloneTime(req.ValidTo),
		Metadata:        cloneMap(req.Metadata),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
}

// ListRules returns all affiliate rules for admin management.
func (s *Service) ListRules(ctx context.Context) ([]contract.AffiliateRule, error) {
	return s.store.ListRules(ctx)
}

// UpdateRule updates an existing affiliate rule while preserving omitted fields.
func (s *Service) UpdateRule(ctx context.Context, id int, req contract.UpdateRuleRequest) (contract.AffiliateRule, error) {
	if id <= 0 {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	current, err := s.store.GetRule(ctx, id)
	if err != nil {
		return contract.AffiliateRule{}, err
	}
	updated := current
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		updated.Name = name
	}
	if req.Status != nil {
		if !validRuleStatus(*req.Status) {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		updated.Status = *req.Status
	}
	if req.TriggerType != nil {
		if *req.TriggerType == "" {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		updated.TriggerType = *req.TriggerType
	}
	if req.Rate != nil {
		rate, ok := normalizeMoney(*req.Rate)
		if strings.TrimSpace(*req.Rate) == "" {
			rate = "0.00000000"
			ok = true
		}
		if !ok {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		updated.Rate = rate
	}
	if req.FixedAmount != nil {
		fixedAmount, ok := normalizeMoney(*req.FixedAmount)
		if strings.TrimSpace(*req.FixedAmount) == "" {
			fixedAmount = "0.00000000"
			ok = true
		}
		if !ok {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		updated.FixedAmount = fixedAmount
	}
	if compareMoney(updated.Rate, "0.00000000") <= 0 && compareMoney(updated.FixedAmount, "0.00000000") <= 0 {
		return contract.AffiliateRule{}, ErrInvalidInput
	}
	if req.Currency != nil {
		updated.Currency = normalizeCurrency(*req.Currency)
	}
	if req.MaxRebateAmount != nil {
		maxRebateAmount, ok := normalizeMoney(*req.MaxRebateAmount)
		if strings.TrimSpace(*req.MaxRebateAmount) == "" {
			maxRebateAmount = "0.00000000"
			ok = true
		}
		if !ok {
			return contract.AffiliateRule{}, ErrInvalidInput
		}
		updated.MaxRebateAmount = maxRebateAmount
	}
	if req.ValidFrom != nil {
		updated.ValidFrom = cloneTime(req.ValidFrom)
	}
	if req.ValidTo != nil {
		updated.ValidTo = cloneTime(req.ValidTo)
	}
	if req.Metadata != nil {
		updated.Metadata = cloneMap(*req.Metadata)
	}
	updated.UpdatedAt = s.clock.Now()
	return s.store.UpdateRule(ctx, updated)
}

func (s *Service) AccrueRebate(ctx context.Context, req contract.AccrueRebateRequest) (contract.RebateResult, error) {
	orderNo := strings.TrimSpace(req.OrderNo)
	amount, ok := normalizeMoney(req.Amount)
	currency := normalizeCurrency(req.Currency)
	if req.OrderID <= 0 || orderNo == "" || req.InviteeUserID <= 0 || !ok || compareMoney(amount, "0.00000000") <= 0 {
		return contract.RebateResult{}, ErrInvalidInput
	}
	paidAt := req.PaidAt.UTC()
	if paidAt.IsZero() {
		paidAt = s.clock.Now()
	}
	relationship, err := s.store.FindRelationshipByInvitee(ctx, req.InviteeUserID)
	if err != nil {
		if errorsIsNotFound(err) {
			return contract.RebateResult{Applied: false, Reason: "no_invite_relationship"}, nil
		}
		return contract.RebateResult{}, err
	}
	if relationship.Status != contract.RelationshipStatusActive {
		return contract.RebateResult{Applied: false, Reason: "invite_relationship_inactive"}, nil
	}
	rule, err := s.store.GetEffectiveRule(ctx, contract.TriggerTypePaymentPaid, currency, paidAt)
	if err != nil {
		if errorsIsNotFound(err) {
			return contract.RebateResult{Applied: false, Reason: "no_effective_rule"}, nil
		}
		return contract.RebateResult{}, err
	}
	rebateAmount, ok := calculateRebate(amount, rule)
	if !ok || compareMoney(rebateAmount, "0.00000000") <= 0 {
		return contract.RebateResult{Applied: false, Reason: "zero_rebate"}, nil
	}
	ledger, created, err := s.store.AppendLedger(ctx, contract.AffiliateLedger{
		UserID:         relationship.InviterUserID,
		RelatedUserID:  relationship.InviteeUserID,
		PaymentOrderID: &req.OrderID,
		Type:           contract.LedgerTypeAccrue,
		Amount:         rebateAmount,
		Currency:       currency,
		Status:         contract.LedgerStatusPending,
		ReferenceID:    accrueReference(orderNo),
		Metadata: map[string]any{
			"affiliate_rule_id":       rule.ID,
			"invite_relationship_id":  relationship.ID,
			"invite_code_id":          relationship.InviteCodeID,
			"payment_order_no":        orderNo,
			"payment_amount":          amount,
			"provider_transaction_id": strings.TrimSpace(req.ProviderTransactionID),
		},
		CreatedAt: paidAt,
		UpdatedAt: paidAt,
	})
	if err != nil {
		return contract.RebateResult{}, err
	}
	if !created {
		return contract.RebateResult{Applied: false, Reason: "duplicate_rebate", Ledgers: []contract.AffiliateLedger{ledger}}, nil
	}
	if relationship.FirstPaidAt == nil {
		_, _ = s.store.MarkRelationshipFirstPaid(ctx, relationship.ID, paidAt)
	}
	s.recordAudit(ctx, "affiliate_rebate.accrue", "affiliate_ledger", strconv.Itoa(ledger.ID), nil, affiliateLedgerSnapshot(ledger))
	s.enqueueAccrued(ctx, ledger, rule)
	return contract.RebateResult{Applied: true, Ledgers: []contract.AffiliateLedger{ledger}}, nil
}

func (s *Service) CompensateRefund(ctx context.Context, req contract.CompensateRefundRequest) (contract.RebateResult, error) {
	refundID := strings.TrimSpace(req.RefundID)
	refundAmount, ok := normalizeMoney(req.RefundAmount)
	currency := normalizeCurrency(req.Currency)
	if req.OrderID <= 0 || req.UserID <= 0 || refundID == "" || !ok || compareMoney(refundAmount, "0.00000000") <= 0 {
		return contract.RebateResult{}, ErrInvalidInput
	}
	refundedAt := req.RefundedAt.UTC()
	if refundedAt.IsZero() {
		refundedAt = s.clock.Now()
	}
	ledgers, err := s.store.ListLedgersByPaymentOrder(ctx, req.OrderID)
	if err != nil {
		return contract.RebateResult{}, err
	}
	accruals := make([]contract.AffiliateLedger, 0)
	compensatedByOriginal := map[int]*big.Rat{}
	for _, ledger := range ledgers {
		switch ledger.Type {
		case contract.LedgerTypeAccrue:
			if ledger.RelatedUserID == req.UserID && ledger.Currency == currency && compareMoney(ledger.Amount, "0.00000000") > 0 {
				accruals = append(accruals, ledger)
			}
		case contract.LedgerTypeRefundCompensation:
			originalID := metadataInt(ledger.Metadata, "original_ledger_id")
			if originalID > 0 {
				amount, amountOK := money.RequiredDecimalRat(ledger.Amount)
				if amountOK {
					if compensatedByOriginal[originalID] == nil {
						compensatedByOriginal[originalID] = new(big.Rat)
					}
					compensatedByOriginal[originalID].Add(compensatedByOriginal[originalID], absRat(amount))
				}
			}
		}
	}
	if len(accruals) == 0 {
		return contract.RebateResult{Applied: false, Reason: "no_rebate_to_compensate"}, nil
	}
	createdLedgers := make([]contract.AffiliateLedger, 0, len(accruals))
	seenLedgers := make([]contract.AffiliateLedger, 0, len(accruals))
	for _, original := range accruals {
		paymentAmount := metadataString(original.Metadata, "payment_amount")
		compensationAmount, ok := calculateCompensation(original.Amount, paymentAmount, refundAmount, compensatedByOriginal[original.ID])
		if !ok || compareMoney(compensationAmount, "0.00000000") <= 0 {
			continue
		}
		negativeAmount := "-" + compensationAmount
		ledger, created, err := s.store.AppendLedger(ctx, contract.AffiliateLedger{
			UserID:         original.UserID,
			RelatedUserID:  original.RelatedUserID,
			PaymentOrderID: original.PaymentOrderID,
			SubscriptionID: original.SubscriptionID,
			Type:           contract.LedgerTypeRefundCompensation,
			Amount:         negativeAmount,
			Currency:       original.Currency,
			Status:         contract.LedgerStatusCompensated,
			ReferenceID:    compensationReference(refundID, original.ID),
			Metadata: map[string]any{
				"original_ledger_id": original.ID,
				"source_refund_id":   refundID,
				"refund_amount":      refundAmount,
				"refund_reason":      strings.TrimSpace(req.Reason),
			},
			CreatedAt: refundedAt,
			UpdatedAt: refundedAt,
			SettledAt: &refundedAt,
		})
		if err != nil {
			return contract.RebateResult{}, err
		}
		if created {
			createdLedgers = append(createdLedgers, ledger)
			s.recordAudit(ctx, "affiliate_rebate.compensate_refund", "affiliate_ledger", strconv.Itoa(ledger.ID), affiliateLedgerSnapshot(original), affiliateLedgerSnapshot(ledger))
			s.enqueueCompensated(ctx, ledger, original, refundID, req.Reason)
		} else {
			seenLedgers = append(seenLedgers, ledger)
		}
	}
	if len(createdLedgers) == 0 && len(seenLedgers) > 0 {
		return contract.RebateResult{Applied: false, Reason: "duplicate_compensation", Ledgers: seenLedgers}, nil
	}
	if len(createdLedgers) == 0 {
		return contract.RebateResult{Applied: false, Reason: "rebate_already_compensated"}, nil
	}
	return contract.RebateResult{Applied: true, Ledgers: createdLedgers}, nil
}

func (s *Service) TransferToBalance(ctx context.Context, req contract.TransferToBalanceRequest) (contract.TransferToBalanceResult, error) {
	amount, ok := normalizeMoney(req.Amount)
	currency := normalizeCurrency(req.Currency)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if req.UserID <= 0 || idempotencyKey == "" || !ok || compareMoney(amount, "0.00000000") <= 0 {
		return contract.TransferToBalanceResult{}, ErrInvalidInput
	}
	requestedAt := req.RequestedAt.UTC()
	if requestedAt.IsZero() {
		requestedAt = s.clock.Now()
	}
	result, created, err := s.store.TransferToBalance(ctx, contract.TransferToBalanceInput{
		UserID:      req.UserID,
		Amount:      amount,
		Currency:    currency,
		ReferenceID: transferReference(idempotencyKey),
		Metadata: map[string]any{
			"idempotency_key": idempotencyKey,
		},
		CreatedAt: requestedAt,
	})
	if err != nil {
		return contract.TransferToBalanceResult{}, err
	}
	result.Applied = created
	if !created {
		result.Reason = "duplicate_transfer"
		return result, nil
	}
	s.recordAudit(
		ctx,
		"affiliate_rebate.transfer_to_balance",
		"affiliate_ledger",
		strconv.Itoa(result.AffiliateLedger.ID),
		nil,
		transferSnapshot(result),
	)
	return result, nil
}

// RequestWithdraw reserves affiliate balance in a pending withdrawal ledger.
func (s *Service) RequestWithdraw(ctx context.Context, req contract.WithdrawRequest) (contract.AffiliateLedger, bool, error) {
	amount, ok := normalizeMoney(req.Amount)
	currency := normalizeCurrency(req.Currency)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if req.UserID <= 0 || idempotencyKey == "" || !ok || compareMoney(amount, "0.00000000") <= 0 {
		return contract.AffiliateLedger{}, false, ErrInvalidInput
	}
	requestedAt := req.RequestedAt.UTC()
	if requestedAt.IsZero() {
		requestedAt = s.clock.Now()
	}
	metadata := map[string]any{
		"idempotency_key": idempotencyKey,
	}
	if destination := strings.TrimSpace(req.Destination); destination != "" {
		metadata["destination"] = destination
	}
	ledger, created, err := s.store.CreateWithdrawal(ctx, contract.WithdrawalInput{
		UserID:      req.UserID,
		Amount:      amount,
		Currency:    currency,
		ReferenceID: withdrawReference(idempotencyKey),
		Metadata:    metadata,
		CreatedAt:   requestedAt,
	})
	if err != nil {
		return contract.AffiliateLedger{}, false, err
	}
	if created {
		s.recordAudit(ctx, "affiliate_rebate.withdraw.request", "affiliate_ledger", strconv.Itoa(ledger.ID), nil, affiliateLedgerSnapshot(ledger))
	}
	return ledger, created, nil
}

// ApproveWithdraw settles a pending affiliate withdrawal request.
func (s *Service) ApproveWithdraw(ctx context.Context, req contract.WithdrawDecisionRequest) (contract.AffiliateLedger, error) {
	return s.decideWithdraw(ctx, req, contract.LedgerStatusSettled, "affiliate_rebate.withdraw.approve")
}

// CancelWithdraw cancels a pending affiliate withdrawal request and releases the reserved balance.
func (s *Service) CancelWithdraw(ctx context.Context, req contract.WithdrawDecisionRequest) (contract.AffiliateLedger, error) {
	return s.decideWithdraw(ctx, req, contract.LedgerStatusCanceled, "affiliate_rebate.withdraw.cancel")
}

func (s *Service) decideWithdraw(ctx context.Context, req contract.WithdrawDecisionRequest, status contract.LedgerStatus, auditAction string) (contract.AffiliateLedger, error) {
	if req.AdminUserID <= 0 || req.LedgerID <= 0 {
		return contract.AffiliateLedger{}, ErrInvalidInput
	}
	ledger, err := s.store.GetLedger(ctx, req.LedgerID)
	if err != nil {
		return contract.AffiliateLedger{}, err
	}
	if ledger.Type != contract.LedgerTypeWithdraw {
		return contract.AffiliateLedger{}, ErrInvalidInput
	}
	if ledger.Status != contract.LedgerStatusPending {
		return contract.AffiliateLedger{}, contract.ErrConflict
	}
	decidedAt := req.DecidedAt.UTC()
	if decidedAt.IsZero() {
		decidedAt = s.clock.Now()
	}
	before := affiliateLedgerSnapshot(ledger)
	updated := ledger
	updated.Status = status
	updated.UpdatedAt = decidedAt
	updated.Metadata = cloneMap(ledger.Metadata)
	switch status {
	case contract.LedgerStatusSettled:
		updated.SettledAt = cloneTime(&decidedAt)
		updated.Metadata["approved_by"] = req.AdminUserID
		if reason := strings.TrimSpace(req.Reason); reason != "" {
			updated.Metadata["approval_reason"] = reason
		}
	case contract.LedgerStatusCanceled:
		updated.SettledAt = nil
		updated.Metadata["canceled_by"] = req.AdminUserID
		if reason := strings.TrimSpace(req.Reason); reason != "" {
			updated.Metadata["cancel_reason"] = reason
		}
	default:
		return contract.AffiliateLedger{}, ErrInvalidInput
	}
	updated, err = s.store.UpdateLedger(ctx, updated, contract.LedgerStatusPending)
	if err != nil {
		return contract.AffiliateLedger{}, err
	}
	s.recordAudit(ctx, auditAction, "affiliate_ledger", strconv.Itoa(updated.ID), before, affiliateLedgerSnapshot(updated))
	return updated, nil
}

// CreateManualAdjustment records an admin-created affiliate balance adjustment.
func (s *Service) CreateManualAdjustment(ctx context.Context, req contract.ManualAdjustmentRequest) (contract.AffiliateLedger, bool, error) {
	amount, ok := normalizeSignedMoney(req.Amount)
	currency := normalizeCurrency(req.Currency)
	reason := strings.TrimSpace(req.Reason)
	if req.AdminUserID <= 0 || req.UserID <= 0 || !ok || compareMoney(absMoney(amount), "0.00000000") <= 0 || reason == "" {
		return contract.AffiliateLedger{}, false, ErrInvalidInput
	}
	if strings.HasPrefix(amount, "-") {
		debit := strings.TrimPrefix(amount, "-")
		if available, err := s.availableBalance(ctx, req.UserID, currency); err != nil {
			return contract.AffiliateLedger{}, false, err
		} else if compareMoney(available, debit) < 0 {
			return contract.AffiliateLedger{}, false, contract.ErrInsufficientBalance
		}
	}
	createdAt := req.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = s.clock.Now()
	}
	referenceID := strings.TrimSpace(req.ReferenceID)
	if referenceID == "" {
		referenceID = manualAdjustmentReference(req.AdminUserID, req.UserID, reason, createdAt)
	} else {
		referenceID = "manual_adjustment:" + referenceID
	}
	metadata := cloneMap(req.Metadata)
	metadata["reason"] = reason
	metadata["admin_user_id"] = req.AdminUserID
	ledger, created, err := s.store.AppendLedger(ctx, contract.AffiliateLedger{
		UserID:      req.UserID,
		Type:        contract.LedgerTypeManualAdjustment,
		Amount:      amount,
		Currency:    currency,
		Status:      contract.LedgerStatusSettled,
		ReferenceID: referenceID,
		Metadata:    metadata,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
		SettledAt:   cloneTime(&createdAt),
	})
	if err != nil {
		return contract.AffiliateLedger{}, false, err
	}
	if created {
		s.recordAudit(ctx, "affiliate_rebate.manual_adjustment", "affiliate_ledger", strconv.Itoa(ledger.ID), nil, affiliateLedgerSnapshot(ledger))
	}
	return ledger, created, nil
}

func (s *Service) ListLedgers(ctx context.Context) ([]contract.AffiliateLedger, error) {
	return s.store.ListLedgers(ctx)
}

// ListLedgersByUser returns affiliate ledger entries owned by one user.
func (s *Service) ListLedgersByUser(ctx context.Context, userID int) ([]contract.AffiliateLedger, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListLedgersByUser(ctx, userID)
}

// GetSummary returns the user's affiliate balances grouped by currency.
func (s *Service) GetSummary(ctx context.Context, userID int) (contract.AffiliateSummary, error) {
	if userID <= 0 {
		return contract.AffiliateSummary{}, ErrInvalidInput
	}
	ledgers, err := s.store.ListLedgersByUser(ctx, userID)
	if err != nil {
		return contract.AffiliateSummary{}, err
	}
	inviteCodes, err := s.store.ListInviteCodesByUser(ctx, userID)
	if err != nil {
		return contract.AffiliateSummary{}, err
	}
	relationships, err := s.store.ListRelationships(ctx)
	if err != nil {
		return contract.AffiliateSummary{}, err
	}
	invitedCount := 0
	for _, relationship := range relationships {
		if relationship.InviterUserID == userID && relationship.Status == contract.RelationshipStatusActive {
			invitedCount++
		}
	}
	summary := contract.AffiliateSummary{
		UserID:       userID,
		Balances:     affiliateCurrencySummaries(ledgers),
		InviteCodes:  inviteCodes,
		InvitedCount: invitedCount,
	}
	return summary, nil
}

func (s *Service) availableBalance(ctx context.Context, userID int, currency string) (string, error) {
	ledgers, err := s.store.ListLedgersByUser(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, summary := range affiliateCurrencySummaries(ledgers) {
		if summary.Currency == currency {
			return summary.AvailableBalance, nil
		}
	}
	return "0.00000000", nil
}

func (s *Service) ListRelationships(ctx context.Context) ([]contract.InviteRelationship, error) {
	return s.store.ListRelationships(ctx)
}

func (s *Service) recordAudit(ctx context.Context, action string, resourceType string, resourceID string, before map[string]any, after map[string]any) {
	if s.deps.Audit == nil {
		return
	}
	_, _ = s.deps.Audit.Record(ctx, auditcontract.RecordRequest{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
	})
}

func (s *Service) enqueueAccrued(ctx context.Context, ledger contract.AffiliateLedger, rule contract.AffiliateRule) {
	if s.deps.Events == nil {
		return
	}
	_, _ = s.deps.Events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "AffiliateRebateAccrued",
		EventVersion:   "v1",
		ProducerModule: "affiliate",
		AggregateType:  "affiliate_ledger",
		AggregateID:    strconv.Itoa(ledger.ID),
		IdempotencyKey: "affiliate_accrued:" + ledger.ReferenceID,
		Payload: map[string]any{
			"affiliate_ledger_id": ledger.ID,
			"inviter_user_id":     ledger.UserID,
			"invitee_user_id":     ledger.RelatedUserID,
			"source_order_id":     intPtrValue(ledger.PaymentOrderID),
			"rebate_amount":       ledger.Amount,
			"currency":            ledger.Currency,
			"rule_id":             rule.ID,
			"status":              string(ledger.Status),
		},
	})
}

func (s *Service) enqueueCompensated(ctx context.Context, ledger contract.AffiliateLedger, original contract.AffiliateLedger, refundID string, reason string) {
	if s.deps.Events == nil {
		return
	}
	_, _ = s.deps.Events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "AffiliateRebateCompensated",
		EventVersion:   "v1",
		ProducerModule: "affiliate",
		AggregateType:  "affiliate_ledger",
		AggregateID:    strconv.Itoa(ledger.ID),
		IdempotencyKey: "affiliate_compensated:" + ledger.ReferenceID,
		Payload: map[string]any{
			"compensation_ledger_id": ledger.ID,
			"original_ledger_id":     original.ID,
			"source_refund_id":       strings.TrimSpace(refundID),
			"amount":                 ledger.Amount,
			"currency":               ledger.Currency,
			"reason":                 strings.TrimSpace(reason),
		},
	})
}

func calculateRebate(paymentAmount string, rule contract.AffiliateRule) (string, bool) {
	payment, ok := money.RequiredDecimalRat(paymentAmount)
	if !ok {
		return "", false
	}
	rate, ok := money.RequiredDecimalRat(rule.Rate)
	if !ok {
		return "", false
	}
	fixed, ok := money.RequiredDecimalRat(rule.FixedAmount)
	if !ok {
		return "", false
	}
	rebate := new(big.Rat).Mul(payment, rate)
	rebate.Add(rebate, fixed)
	if max, ok := money.RequiredDecimalRat(rule.MaxRebateAmount); ok && max.Sign() > 0 && rebate.Cmp(max) > 0 {
		rebate = max
	}
	if rebate.Sign() < 0 {
		return "", false
	}
	return money.FormatRatFixed(rebate, 8), true
}

func calculateCompensation(originalAmount string, paymentAmount string, refundAmount string, alreadyCompensated *big.Rat) (string, bool) {
	original, ok := money.RequiredDecimalRat(originalAmount)
	if !ok || original.Sign() <= 0 {
		return "", false
	}
	payment, ok := money.RequiredDecimalRat(paymentAmount)
	if !ok || payment.Sign() <= 0 {
		return "", false
	}
	refund, ok := money.RequiredDecimalRat(refundAmount)
	if !ok || refund.Sign() <= 0 {
		return "", false
	}
	compensation := new(big.Rat).Mul(original, refund)
	compensation.Quo(compensation, payment)
	if alreadyCompensated == nil {
		alreadyCompensated = new(big.Rat)
	}
	remaining := new(big.Rat).Sub(original, alreadyCompensated)
	if remaining.Sign() <= 0 {
		return "0.00000000", true
	}
	if compensation.Cmp(remaining) > 0 {
		compensation = remaining
	}
	return money.FormatRatFixed(compensation, 8), true
}

func accrueReference(orderNo string) string {
	return "payment_paid:" + strings.TrimSpace(orderNo)
}

func compensationReference(refundID string, originalLedgerID int) string {
	return "refund_compensation:" + strings.TrimSpace(refundID) + ":" + strconv.Itoa(originalLedgerID)
}

func transferReference(idempotencyKey string) string {
	return "transfer_to_balance:" + strings.TrimSpace(idempotencyKey)
}

func withdrawReference(idempotencyKey string) string {
	return "withdraw:" + strings.TrimSpace(idempotencyKey)
}

func manualAdjustmentReference(adminUserID int, userID int, reason string, createdAt time.Time) string {
	return "manual_adjustment:" + strconv.Itoa(adminUserID) + ":" + strconv.Itoa(userID) + ":" + strconv.FormatInt(createdAt.UnixNano(), 10) + ":" + strconv.Itoa(len(reason))
}

func normalizeMoney(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	rat, ok := money.RequiredDecimalRat(value)
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return money.FormatRatFixed(rat, 8), true
}

func normalizeSignedMoney(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	rat, ok := money.RequiredDecimalRat(value)
	if !ok {
		return "", false
	}
	return money.FormatRatFixed(rat, 8), true
}

func absMoney(value string) string {
	if strings.HasPrefix(value, "-") {
		return strings.TrimPrefix(value, "-")
	}
	return value
}

func compareMoney(left string, right string) int {
	leftRat, leftOK := money.RequiredDecimalRat(left)
	rightRat, rightOK := money.RequiredDecimalRat(right)
	if !leftOK || !rightOK {
		return strings.Compare(left, right)
	}
	return leftRat.Cmp(rightRat)
}

func normalizeCurrency(value string) string {
	return money.NormalizeCurrency(value)
}

func validRuleStatus(status contract.RuleStatus) bool {
	switch status {
	case contract.RuleStatusActive, contract.RuleStatusDisabled, contract.RuleStatusArchived:
		return true
	default:
		return false
	}
}

type affiliateCurrencyAccumulator struct {
	available            *big.Rat
	accrued              *big.Rat
	refundCompensated    *big.Rat
	transferredToBalance *big.Rat
	withdrawn            *big.Rat
	manualAdjustment     *big.Rat
}

func newAffiliateCurrencyAccumulator() affiliateCurrencyAccumulator {
	return affiliateCurrencyAccumulator{
		available:            new(big.Rat),
		accrued:              new(big.Rat),
		refundCompensated:    new(big.Rat),
		transferredToBalance: new(big.Rat),
		withdrawn:            new(big.Rat),
		manualAdjustment:     new(big.Rat),
	}
}

func affiliateCurrencySummaries(ledgers []contract.AffiliateLedger) []contract.AffiliateCurrencySummary {
	accumulators := map[string]affiliateCurrencyAccumulator{}
	for _, ledger := range ledgers {
		if ledger.Status == contract.LedgerStatusCanceled {
			continue
		}
		amount, ok := money.RequiredDecimalRat(ledger.Amount)
		if !ok {
			continue
		}
		currency := normalizeCurrency(ledger.Currency)
		accumulator, ok := accumulators[currency]
		if !ok {
			accumulator = newAffiliateCurrencyAccumulator()
		}
		accumulator.available.Add(accumulator.available, amount)
		absoluteAmount := absRat(amount)
		switch ledger.Type {
		case contract.LedgerTypeAccrue:
			accumulator.accrued.Add(accumulator.accrued, amount)
		case contract.LedgerTypeRefundCompensation:
			accumulator.refundCompensated.Add(accumulator.refundCompensated, absoluteAmount)
		case contract.LedgerTypeTransferToBalance:
			accumulator.transferredToBalance.Add(accumulator.transferredToBalance, absoluteAmount)
		case contract.LedgerTypeWithdraw:
			accumulator.withdrawn.Add(accumulator.withdrawn, absoluteAmount)
		case contract.LedgerTypeManualAdjustment:
			accumulator.manualAdjustment.Add(accumulator.manualAdjustment, amount)
		}
		accumulators[currency] = accumulator
	}
	currencies := make([]string, 0, len(accumulators))
	for currency := range accumulators {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	out := make([]contract.AffiliateCurrencySummary, 0, len(currencies))
	for _, currency := range currencies {
		accumulator := accumulators[currency]
		out = append(out, contract.AffiliateCurrencySummary{
			Currency:                   currency,
			AvailableBalance:           money.FormatRatFixed(accumulator.available, 8),
			AccruedAmount:              money.FormatRatFixed(accumulator.accrued, 8),
			RefundCompensatedAmount:    money.FormatRatFixed(accumulator.refundCompensated, 8),
			TransferredToBalanceAmount: money.FormatRatFixed(accumulator.transferredToBalance, 8),
			WithdrawnAmount:            money.FormatRatFixed(accumulator.withdrawn, 8),
			ManualAdjustmentAmount:     money.FormatRatFixed(accumulator.manualAdjustment, 8),
		})
	}
	return out
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, contract.ErrNotFound)
}

func metadataString(value map[string]any, key string) string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(toString(raw))
}

func metadataInt(value map[string]any, key string) int {
	raw, ok := value[key]
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(strings.Trim(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(formatAny(value)), "\n", ""), "\t", ""), `"`))
	}
}

func formatAny(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func absRat(value *big.Rat) *big.Rat {
	if value == nil {
		return new(big.Rat)
	}
	if value.Sign() < 0 {
		return new(big.Rat).Neg(value)
	}
	return new(big.Rat).Set(value)
}

func affiliateLedgerSnapshot(ledger contract.AffiliateLedger) map[string]any {
	return map[string]any{
		"id":               ledger.ID,
		"user_id":          ledger.UserID,
		"related_user_id":  ledger.RelatedUserID,
		"payment_order_id": intPtrValue(ledger.PaymentOrderID),
		"type":             string(ledger.Type),
		"amount":           ledger.Amount,
		"currency":         ledger.Currency,
		"status":           string(ledger.Status),
		"reference_id":     ledger.ReferenceID,
	}
}

func transferSnapshot(result contract.TransferToBalanceResult) map[string]any {
	snapshot := affiliateLedgerSnapshot(result.AffiliateLedger)
	snapshot["billing_ledger_id"] = result.BillingLedgerID
	snapshot["balance_before"] = result.BalanceBefore
	snapshot["balance_after"] = result.BalanceAfter
	return snapshot
}

func intPtrValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
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

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
