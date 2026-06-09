package affiliate

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entaffiliateledger "github.com/srapi/srapi/apps/api/ent/affiliateledger"
	entaffiliaterule "github.com/srapi/srapi/apps/api/ent/affiliaterule"
	entinvitecode "github.com/srapi/srapi/apps/api/ent/invitecode"
	entinviterelationship "github.com/srapi/srapi/apps/api/ent/inviterelationship"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	"github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

var ErrInvalidStore = errors.New("invalid affiliate ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateInviteCode(ctx context.Context, input contract.InviteCode) (contract.InviteCode, error) {
	create := s.client.InviteCode.Create().
		SetUserID(input.UserID).
		SetCode(input.Code).
		SetStatus(string(input.Status)).
		SetNillableExpiresAt(input.ExpiresAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.InviteCode{}, contract.ErrConflict
		}
		return contract.InviteCode{}, err
	}
	return toInviteCode(row), nil
}

func (s *Store) FindInviteCodeByCode(ctx context.Context, code string) (contract.InviteCode, error) {
	row, err := s.client.InviteCode.Query().
		Where(entinvitecode.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.InviteCode{}, contract.ErrNotFound
		}
		return contract.InviteCode{}, err
	}
	return toInviteCode(row), nil
}

func (s *Store) CreateRelationship(ctx context.Context, input contract.InviteRelationship) (contract.InviteRelationship, error) {
	create := s.client.InviteRelationship.Create().
		SetInviterUserID(input.InviterUserID).
		SetInviteeUserID(input.InviteeUserID).
		SetInviteCodeID(input.InviteCodeID).
		SetStatus(string(input.Status)).
		SetNillableFirstPaidAt(input.FirstPaidAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.InviteRelationship{}, contract.ErrConflict
		}
		return contract.InviteRelationship{}, err
	}
	return toRelationship(row), nil
}

func (s *Store) FindRelationshipByInvitee(ctx context.Context, inviteeUserID int) (contract.InviteRelationship, error) {
	row, err := s.client.InviteRelationship.Query().
		Where(entinviterelationship.InviteeUserIDEQ(inviteeUserID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.InviteRelationship{}, contract.ErrNotFound
		}
		return contract.InviteRelationship{}, err
	}
	return toRelationship(row), nil
}

func (s *Store) ListRelationships(ctx context.Context) ([]contract.InviteRelationship, error) {
	rows, err := s.client.InviteRelationship.Query().
		Order(entinviterelationship.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.InviteRelationship, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRelationship(row))
	}
	return out, nil
}

func (s *Store) MarkRelationshipFirstPaid(ctx context.Context, id int, firstPaidAt time.Time) (contract.InviteRelationship, error) {
	row, err := s.client.InviteRelationship.UpdateOneID(id).
		Where(entinviterelationship.FirstPaidAtIsNil()).
		SetFirstPaidAt(firstPaidAt).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			found, findErr := s.client.InviteRelationship.Get(ctx, id)
			if findErr != nil {
				if ent.IsNotFound(findErr) {
					return contract.InviteRelationship{}, contract.ErrNotFound
				}
				return contract.InviteRelationship{}, findErr
			}
			return toRelationship(found), nil
		}
		return contract.InviteRelationship{}, err
	}
	return toRelationship(row), nil
}

func (s *Store) CreateRule(ctx context.Context, input contract.AffiliateRule) (contract.AffiliateRule, error) {
	create := s.client.AffiliateRule.Create().
		SetName(input.Name).
		SetStatus(string(input.Status)).
		SetTriggerType(string(input.TriggerType)).
		SetRate(input.Rate).
		SetFixedAmount(input.FixedAmount).
		SetCurrency(input.Currency).
		SetMaxRebateAmount(input.MaxRebateAmount).
		SetNillableValidFrom(input.ValidFrom).
		SetNillableValidTo(input.ValidTo).
		SetMetadataJSON(cloneMap(input.Metadata))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.AffiliateRule{}, err
	}
	return toRule(row), nil
}

func (s *Store) GetEffectiveRule(ctx context.Context, trigger contract.TriggerType, currency string, at time.Time) (contract.AffiliateRule, error) {
	row, err := s.client.AffiliateRule.Query().
		Where(
			entaffiliaterule.TriggerTypeEQ(string(trigger)),
			entaffiliaterule.CurrencyEQ(currency),
			entaffiliaterule.StatusEQ(string(contract.RuleStatusActive)),
			entaffiliaterule.Or(entaffiliaterule.ValidFromIsNil(), entaffiliaterule.ValidFromLTE(at)),
			entaffiliaterule.Or(entaffiliaterule.ValidToIsNil(), entaffiliaterule.ValidToGT(at)),
		).
		Order(entaffiliaterule.ByValidFrom(), entaffiliaterule.ByID()).
		All(ctx)
	if err != nil {
		return contract.AffiliateRule{}, err
	}
	if len(row) == 0 {
		return contract.AffiliateRule{}, contract.ErrNotFound
	}
	return toRule(row[len(row)-1]), nil
}

func (s *Store) AppendLedger(ctx context.Context, input contract.AffiliateLedger) (contract.AffiliateLedger, bool, error) {
	if existing, err := s.findLedgerByReference(ctx, input.ReferenceID); err == nil {
		return existing, false, nil
	} else if !ent.IsNotFound(err) {
		return contract.AffiliateLedger{}, false, err
	}
	create := s.client.AffiliateLedger.Create().
		SetUserID(input.UserID).
		SetRelatedUserID(input.RelatedUserID).
		SetNillablePaymentOrderID(input.PaymentOrderID).
		SetNillableSubscriptionID(input.SubscriptionID).
		SetType(string(input.Type)).
		SetAmount(input.Amount).
		SetCurrency(input.Currency).
		SetStatus(string(input.Status)).
		SetReferenceID(input.ReferenceID).
		SetMetadataJSON(cloneMap(input.Metadata)).
		SetNillableSettledAt(input.SettledAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findLedgerByReference(ctx, input.ReferenceID); findErr == nil {
				return existing, false, nil
			}
		}
		return contract.AffiliateLedger{}, false, err
	}
	return toLedger(row), true, nil
}

func (s *Store) TransferToBalance(ctx context.Context, input contract.TransferToBalanceInput) (contract.TransferToBalanceResult, bool, error) {
	tx, err := s.client.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if existing, err := findLedgerByReference(ctx, tx.Client(), input.ReferenceID); err == nil {
		return transferResultFromLedger(existing), false, nil
	} else if !ent.IsNotFound(err) {
		return contract.TransferToBalanceResult{}, false, err
	}

	amount, ok := money.RequiredDecimalRat(input.Amount)
	if !ok || amount.Sign() <= 0 {
		return contract.TransferToBalanceResult{}, false, contract.ErrInsufficientBalance
	}
	available, err := availableAffiliateBalance(ctx, tx.Client(), input.UserID, input.Currency)
	if err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	if available.Cmp(amount) < 0 {
		return contract.TransferToBalanceResult{}, false, contract.ErrInsufficientBalance
	}

	user, err := tx.User.Query().
		Where(entuser.IDEQ(input.UserID), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	balanceBefore := user.Balance
	if balanceBefore == "" {
		balanceBefore = "0.00000000"
	}
	balanceBeforeRat, ok := money.DecimalRat(balanceBefore)
	if !ok {
		balanceBeforeRat = new(big.Rat)
	}
	balanceAfter := money.FormatRatFixed(new(big.Rat).Add(balanceBeforeRat, amount), 8)

	createdAt := input.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	metadata := cloneMap(input.Metadata)
	metadata["transfer_amount"] = input.Amount
	metadata["balance_before"] = balanceBefore
	metadata["balance_after"] = balanceAfter
	ledger, err := tx.AffiliateLedger.Create().
		SetUserID(input.UserID).
		SetRelatedUserID(0).
		SetType(string(contract.LedgerTypeTransferToBalance)).
		SetAmount("-" + input.Amount).
		SetCurrency(input.Currency).
		SetStatus(string(contract.LedgerStatusSettled)).
		SetReferenceID(input.ReferenceID).
		SetMetadataJSON(metadata).
		SetSettledAt(createdAt).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findLedgerByReference(ctx, input.ReferenceID); findErr == nil {
				return transferResultFromLedger(existing), false, nil
			}
		}
		return contract.TransferToBalanceResult{}, false, err
	}
	billing, err := tx.BillingLedger.Create().
		SetUserID(input.UserID).
		SetType(string(billingcontract.LedgerTypeAffiliateTransfer)).
		SetAmount(input.Amount).
		SetCurrency(input.Currency).
		SetBalanceBefore(balanceBefore).
		SetBalanceAfter(balanceAfter).
		SetReferenceType("affiliate_ledger").
		SetReferenceID(strconv.Itoa(ledger.ID)).
		SetMetadataJSON(map[string]any{
			"affiliate_ledger_id": ledger.ID,
			"affiliate_reference": input.ReferenceID,
		}).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	if err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	metadata["billing_ledger_id"] = billing.ID
	ledger, err = tx.AffiliateLedger.UpdateOneID(ledger.ID).
		SetMetadataJSON(metadata).
		Save(ctx)
	if err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	if _, err := tx.User.UpdateOneID(input.UserID).
		Where(entuser.DeletedAtIsNil()).
		SetBalance(balanceAfter).
		SetCurrency(input.Currency).
		Save(ctx); err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return contract.TransferToBalanceResult{}, false, err
	}
	committed = true
	return contract.TransferToBalanceResult{
		AffiliateLedger: toLedger(ledger),
		BillingLedgerID: billing.ID,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    balanceAfter,
	}, true, nil
}

func (s *Store) ListLedgers(ctx context.Context) ([]contract.AffiliateLedger, error) {
	rows, err := s.client.AffiliateLedger.Query().
		Order(entaffiliateledger.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AffiliateLedger, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLedger(row))
	}
	return out, nil
}

func (s *Store) ListLedgersByUser(ctx context.Context, userID int) ([]contract.AffiliateLedger, error) {
	rows, err := s.client.AffiliateLedger.Query().
		Where(entaffiliateledger.UserIDEQ(userID)).
		Order(entaffiliateledger.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AffiliateLedger, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLedger(row))
	}
	return out, nil
}

func (s *Store) ListLedgersByPaymentOrder(ctx context.Context, paymentOrderID int) ([]contract.AffiliateLedger, error) {
	rows, err := s.client.AffiliateLedger.Query().
		Where(entaffiliateledger.PaymentOrderIDEQ(paymentOrderID)).
		Order(entaffiliateledger.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AffiliateLedger, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLedger(row))
	}
	return out, nil
}

func (s *Store) findLedgerByReference(ctx context.Context, referenceID string) (contract.AffiliateLedger, error) {
	return findLedgerByReference(ctx, s.client, referenceID)
}

func findLedgerByReference(ctx context.Context, client *ent.Client, referenceID string) (contract.AffiliateLedger, error) {
	row, err := client.AffiliateLedger.Query().
		Where(entaffiliateledger.ReferenceIDEQ(referenceID)).
		Only(ctx)
	if err != nil {
		return contract.AffiliateLedger{}, err
	}
	return toLedger(row), nil
}

func availableAffiliateBalance(ctx context.Context, client *ent.Client, userID int, currency string) (*big.Rat, error) {
	rows, err := client.AffiliateLedger.Query().
		Where(
			entaffiliateledger.UserIDEQ(userID),
			entaffiliateledger.CurrencyEQ(currency),
			entaffiliateledger.StatusNEQ(string(contract.LedgerStatusCanceled)),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	total := new(big.Rat)
	for _, row := range rows {
		amount, ok := money.RequiredDecimalRat(row.Amount)
		if !ok {
			continue
		}
		total.Add(total, amount)
	}
	return total, nil
}

func toInviteCode(row *ent.InviteCode) contract.InviteCode {
	return contract.InviteCode{
		ID:        row.ID,
		UserID:    row.UserID,
		Code:      row.Code,
		Status:    contract.InviteCodeStatus(row.Status),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		ExpiresAt: cloneTime(row.ExpiresAt),
	}
}

func toRelationship(row *ent.InviteRelationship) contract.InviteRelationship {
	return contract.InviteRelationship{
		ID:            row.ID,
		InviterUserID: row.InviterUserID,
		InviteeUserID: row.InviteeUserID,
		InviteCodeID:  row.InviteCodeID,
		Status:        contract.RelationshipStatus(row.Status),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		FirstPaidAt:   cloneTime(row.FirstPaidAt),
	}
}

func toRule(row *ent.AffiliateRule) contract.AffiliateRule {
	return contract.AffiliateRule{
		ID:              row.ID,
		Name:            row.Name,
		Status:          contract.RuleStatus(row.Status),
		TriggerType:     contract.TriggerType(row.TriggerType),
		Rate:            row.Rate,
		FixedAmount:     row.FixedAmount,
		Currency:        row.Currency,
		MaxRebateAmount: row.MaxRebateAmount,
		ValidFrom:       cloneTime(row.ValidFrom),
		ValidTo:         cloneTime(row.ValidTo),
		Metadata:        cloneMap(row.MetadataJSON),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func toLedger(row *ent.AffiliateLedger) contract.AffiliateLedger {
	return contract.AffiliateLedger{
		ID:             row.ID,
		UserID:         row.UserID,
		RelatedUserID:  row.RelatedUserID,
		PaymentOrderID: cloneInt(row.PaymentOrderID),
		SubscriptionID: cloneInt(row.SubscriptionID),
		Type:           contract.LedgerType(row.Type),
		Amount:         row.Amount,
		Currency:       row.Currency,
		Status:         contract.LedgerStatus(row.Status),
		ReferenceID:    row.ReferenceID,
		Metadata:       cloneMap(row.MetadataJSON),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		SettledAt:      cloneTime(row.SettledAt),
	}
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

func transferResultFromLedger(ledger contract.AffiliateLedger) contract.TransferToBalanceResult {
	return contract.TransferToBalanceResult{
		AffiliateLedger: ledger,
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
