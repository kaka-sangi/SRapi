package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
)

const defaultCurrency = "USD"

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
}

func New(store contract.Store, deps Dependencies, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, deps: deps, clock: clock}, nil
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
				amount, amountOK := decimalRat(ledger.Amount)
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

func (s *Service) ListLedgers(ctx context.Context) ([]contract.AffiliateLedger, error) {
	return s.store.ListLedgers(ctx)
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
	payment, ok := decimalRat(paymentAmount)
	if !ok {
		return "", false
	}
	rate, ok := decimalRat(rule.Rate)
	if !ok {
		return "", false
	}
	fixed, ok := decimalRat(rule.FixedAmount)
	if !ok {
		return "", false
	}
	rebate := new(big.Rat).Mul(payment, rate)
	rebate.Add(rebate, fixed)
	if max, ok := decimalRat(rule.MaxRebateAmount); ok && max.Sign() > 0 && rebate.Cmp(max) > 0 {
		rebate = max
	}
	if rebate.Sign() < 0 {
		return "", false
	}
	return formatRatFixed(rebate, 8), true
}

func calculateCompensation(originalAmount string, paymentAmount string, refundAmount string, alreadyCompensated *big.Rat) (string, bool) {
	original, ok := decimalRat(originalAmount)
	if !ok || original.Sign() <= 0 {
		return "", false
	}
	payment, ok := decimalRat(paymentAmount)
	if !ok || payment.Sign() <= 0 {
		return "", false
	}
	refund, ok := decimalRat(refundAmount)
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
	return formatRatFixed(compensation, 8), true
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

func normalizeMoney(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	rat, ok := decimalRat(value)
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return formatRatFixed(rat, 8), true
}

func compareMoney(left string, right string) int {
	leftRat, leftOK := decimalRat(left)
	rightRat, rightOK := decimalRat(right)
	if !leftOK || !rightOK {
		return strings.Compare(left, right)
	}
	return leftRat.Cmp(rightRat)
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, false
	}
	return rat, true
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	return value.FloatString(places)
}

func normalizeCurrency(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" {
		return defaultCurrency
	}
	return value
}

func validRuleStatus(status contract.RuleStatus) bool {
	switch status {
	case contract.RuleStatusActive, contract.RuleStatusDisabled, contract.RuleStatusArchived:
		return true
	default:
		return false
	}
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
