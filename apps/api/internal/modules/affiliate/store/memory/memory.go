package memory

import (
	"context"
	"encoding/json"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
)

type Store struct {
	mu                  sync.Mutex
	nextInviteCodeID    int
	nextRelationshipID  int
	nextRuleID          int
	nextLedgerID        int
	inviteCodes         map[int]contract.InviteCode
	inviteCodeIDByCode  map[string]int
	relationships       map[int]contract.InviteRelationship
	relationshipByUser  map[int]int
	rules               map[int]contract.AffiliateRule
	ledgers             map[int]contract.AffiliateLedger
	ledgerByReference   map[string]int
	nextBillingLedgerID int
	userBalances        map[int]map[string]string
}

func New() *Store {
	return &Store{
		nextInviteCodeID:    1,
		nextRelationshipID:  1,
		nextRuleID:          1,
		nextLedgerID:        1,
		inviteCodes:         map[int]contract.InviteCode{},
		inviteCodeIDByCode:  map[string]int{},
		relationships:       map[int]contract.InviteRelationship{},
		relationshipByUser:  map[int]int{},
		rules:               map[int]contract.AffiliateRule{},
		ledgers:             map[int]contract.AffiliateLedger{},
		ledgerByReference:   map[string]int{},
		nextBillingLedgerID: 1,
		userBalances:        map[int]map[string]string{},
	}
}

func (s *Store) CreateInviteCode(_ context.Context, input contract.InviteCode) (contract.InviteCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.inviteCodeIDByCode[input.Code]; ok {
		return contract.InviteCode{}, contract.ErrConflict
	}
	now := time.Now().UTC()
	row := cloneInviteCode(input)
	row.ID = s.nextInviteCodeID
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	s.inviteCodes[row.ID] = row
	s.inviteCodeIDByCode[row.Code] = row.ID
	s.nextInviteCodeID++
	return cloneInviteCode(row), nil
}

func (s *Store) FindInviteCodeByCode(_ context.Context, code string) (contract.InviteCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.inviteCodeIDByCode[code]
	if !ok {
		return contract.InviteCode{}, contract.ErrNotFound
	}
	return cloneInviteCode(s.inviteCodes[id]), nil
}

func (s *Store) CreateRelationship(_ context.Context, input contract.InviteRelationship) (contract.InviteRelationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.InviterUserID == input.InviteeUserID {
		return contract.InviteRelationship{}, contract.ErrConflict
	}
	if _, ok := s.relationshipByUser[input.InviteeUserID]; ok {
		return contract.InviteRelationship{}, contract.ErrConflict
	}
	if _, ok := s.inviteCodes[input.InviteCodeID]; !ok {
		return contract.InviteRelationship{}, contract.ErrNotFound
	}
	now := time.Now().UTC()
	row := cloneRelationship(input)
	row.ID = s.nextRelationshipID
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	s.relationships[row.ID] = row
	s.relationshipByUser[row.InviteeUserID] = row.ID
	s.nextRelationshipID++
	return cloneRelationship(row), nil
}

func (s *Store) FindRelationshipByInvitee(_ context.Context, inviteeUserID int) (contract.InviteRelationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.relationshipByUser[inviteeUserID]
	if !ok {
		return contract.InviteRelationship{}, contract.ErrNotFound
	}
	return cloneRelationship(s.relationships[id]), nil
}

func (s *Store) ListRelationships(_ context.Context) ([]contract.InviteRelationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.InviteRelationship, 0, len(s.relationships))
	for _, row := range s.relationships {
		out = append(out, cloneRelationship(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) MarkRelationshipFirstPaid(_ context.Context, id int, firstPaidAt time.Time) (contract.InviteRelationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.relationships[id]
	if !ok {
		return contract.InviteRelationship{}, contract.ErrNotFound
	}
	if row.FirstPaidAt == nil {
		row.FirstPaidAt = cloneTime(&firstPaidAt)
	}
	row.UpdatedAt = time.Now().UTC()
	s.relationships[id] = row
	return cloneRelationship(row), nil
}

func (s *Store) CreateRule(_ context.Context, input contract.AffiliateRule) (contract.AffiliateRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	row := cloneRule(input)
	row.ID = s.nextRuleID
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	s.rules[row.ID] = row
	s.nextRuleID++
	return cloneRule(row), nil
}

func (s *Store) GetEffectiveRule(_ context.Context, trigger contract.TriggerType, currency string, at time.Time) (contract.AffiliateRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var matches []contract.AffiliateRule
	for _, row := range s.rules {
		if row.Status != contract.RuleStatusActive || row.TriggerType != trigger || row.Currency != currency {
			continue
		}
		if row.ValidFrom != nil && row.ValidFrom.After(at) {
			continue
		}
		if row.ValidTo != nil && !row.ValidTo.After(at) {
			continue
		}
		matches = append(matches, row)
	}
	if len(matches) == 0 {
		return contract.AffiliateRule{}, contract.ErrNotFound
	}
	sort.SliceStable(matches, func(i, j int) bool {
		leftFrom := time.Time{}
		rightFrom := time.Time{}
		if matches[i].ValidFrom != nil {
			leftFrom = *matches[i].ValidFrom
		}
		if matches[j].ValidFrom != nil {
			rightFrom = *matches[j].ValidFrom
		}
		if leftFrom.Equal(rightFrom) {
			return matches[i].ID > matches[j].ID
		}
		return leftFrom.After(rightFrom)
	})
	return cloneRule(matches[0]), nil
}

func (s *Store) AppendLedger(_ context.Context, input contract.AffiliateLedger) (contract.AffiliateLedger, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.ledgerByReference[input.ReferenceID]; ok {
		return cloneLedger(s.ledgers[id]), false, nil
	}
	now := time.Now().UTC()
	row := cloneLedger(input)
	row.ID = s.nextLedgerID
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	s.ledgers[row.ID] = row
	s.ledgerByReference[row.ReferenceID] = row.ID
	s.nextLedgerID++
	return cloneLedger(row), true, nil
}

func (s *Store) TransferToBalance(_ context.Context, input contract.TransferToBalanceInput) (contract.TransferToBalanceResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.ledgerByReference[input.ReferenceID]; ok {
		return transferResultFromLedger(s.ledgers[id]), false, nil
	}
	amount, ok := decimalRat(input.Amount)
	if !ok || amount.Sign() <= 0 {
		return contract.TransferToBalanceResult{}, false, contract.ErrInsufficientBalance
	}
	available := s.availableAffiliateBalance(input.UserID, input.Currency)
	if available.Cmp(amount) < 0 {
		return contract.TransferToBalanceResult{}, false, contract.ErrInsufficientBalance
	}
	now := time.Now().UTC()
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	balanceBefore := s.userBalance(input.UserID, input.Currency)
	balanceBeforeRat, ok := decimalRat(balanceBefore)
	if !ok {
		balanceBeforeRat = new(big.Rat)
	}
	balanceAfterRat := new(big.Rat).Add(balanceBeforeRat, amount)
	balanceAfter := formatRatFixed(balanceAfterRat, 8)
	billingLedgerID := s.nextBillingLedgerID
	s.nextBillingLedgerID++

	metadata := cloneMap(input.Metadata)
	metadata["transfer_amount"] = input.Amount
	metadata["billing_ledger_id"] = billingLedgerID
	metadata["balance_before"] = balanceBefore
	metadata["balance_after"] = balanceAfter
	row := contract.AffiliateLedger{
		ID:            s.nextLedgerID,
		UserID:        input.UserID,
		RelatedUserID: 0,
		Type:          contract.LedgerTypeTransferToBalance,
		Amount:        "-" + input.Amount,
		Currency:      input.Currency,
		Status:        contract.LedgerStatusSettled,
		ReferenceID:   input.ReferenceID,
		Metadata:      metadata,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		SettledAt:     cloneTime(&createdAt),
	}
	s.ledgers[row.ID] = cloneLedger(row)
	s.ledgerByReference[row.ReferenceID] = row.ID
	s.nextLedgerID++
	s.setUserBalance(input.UserID, input.Currency, balanceAfter)
	return transferResultFromLedger(row), true, nil
}

func (s *Store) ListLedgers(_ context.Context) ([]contract.AffiliateLedger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AffiliateLedger, 0, len(s.ledgers))
	for _, row := range s.ledgers {
		out = append(out, cloneLedger(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListLedgersByUser(_ context.Context, userID int) ([]contract.AffiliateLedger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AffiliateLedger, 0)
	for _, row := range s.ledgers {
		if row.UserID == userID {
			out = append(out, cloneLedger(row))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) availableAffiliateBalance(userID int, currency string) *big.Rat {
	total := new(big.Rat)
	for _, row := range s.ledgers {
		if row.UserID != userID || row.Currency != currency || row.Status == contract.LedgerStatusCanceled {
			continue
		}
		amount, ok := decimalRat(row.Amount)
		if !ok {
			continue
		}
		total.Add(total, amount)
	}
	return total
}

func (s *Store) userBalance(userID int, currency string) string {
	if balances, ok := s.userBalances[userID]; ok {
		if balance := balances[currency]; balance != "" {
			return balance
		}
	}
	return "0.00000000"
}

func (s *Store) setUserBalance(userID int, currency string, balance string) {
	if _, ok := s.userBalances[userID]; !ok {
		s.userBalances[userID] = map[string]string{}
	}
	s.userBalances[userID][currency] = balance
}

func (s *Store) ListLedgersByPaymentOrder(_ context.Context, paymentOrderID int) ([]contract.AffiliateLedger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AffiliateLedger, 0)
	for _, row := range s.ledgers {
		if row.PaymentOrderID != nil && *row.PaymentOrderID == paymentOrderID {
			out = append(out, cloneLedger(row))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneInviteCode(value contract.InviteCode) contract.InviteCode {
	value.ExpiresAt = cloneTime(value.ExpiresAt)
	return value
}

func cloneRelationship(value contract.InviteRelationship) contract.InviteRelationship {
	value.FirstPaidAt = cloneTime(value.FirstPaidAt)
	return value
}

func cloneRule(value contract.AffiliateRule) contract.AffiliateRule {
	value.ValidFrom = cloneTime(value.ValidFrom)
	value.ValidTo = cloneTime(value.ValidTo)
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func cloneLedger(value contract.AffiliateLedger) contract.AffiliateLedger {
	value.PaymentOrderID = cloneInt(value.PaymentOrderID)
	value.SubscriptionID = cloneInt(value.SubscriptionID)
	value.Metadata = cloneMap(value.Metadata)
	value.SettledAt = cloneTime(value.SettledAt)
	return value
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

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func decimalRat(value string) (*big.Rat, bool) {
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

func transferResultFromLedger(ledger contract.AffiliateLedger) contract.TransferToBalanceResult {
	return contract.TransferToBalanceResult{
		AffiliateLedger: cloneLedger(ledger),
		BillingLedgerID: metadataInt(ledger.Metadata, "billing_ledger_id"),
		BalanceBefore:   metadataString(ledger.Metadata, "balance_before"),
		BalanceAfter:    metadataString(ledger.Metadata, "balance_after"),
	}
}

func metadataString(value map[string]any, key string) string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return ""
	}
	if typed, ok := raw.(string); ok {
		return typed
	}
	return ""
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
	default:
		return 0
	}
}
