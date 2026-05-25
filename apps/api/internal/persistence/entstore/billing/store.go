package billing

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entbillingledger "github.com/srapi/srapi/apps/api/ent/billingledger"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
)

var ErrInvalidStore = errors.New("invalid billing ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.LedgerEntry) (contract.LedgerEntry, error) {
	create := s.client.BillingLedger.Create().
		SetUserID(input.UserID).
		SetType(string(input.Type)).
		SetAmount(input.Amount).
		SetCurrency(input.Currency).
		SetBalanceBefore(input.BalanceBefore).
		SetBalanceAfter(input.BalanceAfter).
		SetReferenceType(input.ReferenceType).
		SetReferenceID(input.ReferenceID).
		SetMetadataJSON(cloneMap(input.Metadata))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.LedgerEntry{}, err
	}
	return toLedgerEntry(created), nil
}

func (s *Store) List(ctx context.Context) ([]contract.LedgerEntry, error) {
	rows, err := s.client.BillingLedger.Query().
		Order(entbillingledger.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.LedgerEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLedgerEntry(row))
	}
	return out, nil
}

func (s *Store) ListPendingUsageCharges(ctx context.Context, limit int) ([]contract.PendingUsageCharge, error) {
	if s == nil || s.client == nil {
		return nil, ErrInvalidStore
	}
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.client.UsageLog.Query().
		Where(
			entusagelog.SuccessEQ(true),
			entusagelog.ChargedAtIsNil(),
			entusagelog.CostNEQ(""),
		).
		Order(entusagelog.ByCreatedAt(), entusagelog.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PendingUsageCharge, 0, len(rows))
	for _, row := range rows {
		out = append(out, contract.PendingUsageCharge{
			UsageLogID: row.ID,
			RequestID:  row.RequestID,
			AttemptNo:  row.AttemptNo,
			UserID:     row.UserID,
			Cost:       row.Cost,
			Currency:   row.Currency,
			CreatedAt:  row.CreatedAt,
		})
	}
	return out, nil
}

func (s *Store) ChargeUsage(ctx context.Context, req contract.ChargeUsageRequest) (contract.ChargeUsageResult, error) {
	if s == nil || s.client == nil {
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	if req.UserID <= 0 || len(req.UsageLogIDs) == 0 {
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	if strings.TrimSpace(req.Currency) == "" {
		req.Currency = "USD"
	}
	if strings.TrimSpace(req.ReferenceType) == "" {
		req.ReferenceType = "usage_log_batch"
	}
	if strings.TrimSpace(req.ReferenceID) == "" {
		req.ReferenceID = usageChargeReferenceID(req.UsageLogIDs)
	}
	if req.ChargedAt.IsZero() {
		req.ChargedAt = time.Now().UTC()
	}
	usageLogIDs := uniqueSortedIDs(req.UsageLogIDs)
	if len(usageLogIDs) == 0 {
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.ChargeUsageResult{}, err
	}

	rows, err := tx.UsageLog.Query().
		Where(
			entusagelog.IDIn(usageLogIDs...),
			entusagelog.UserIDEQ(req.UserID),
			entusagelog.SuccessEQ(true),
			entusagelog.CurrencyEQ(normalizeCurrency(req.Currency)),
			entusagelog.ChargedAtIsNil(),
		).
		Order(entusagelog.ByID()).
		All(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, err
	}
	if len(rows) == 0 {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, nil
	}
	if len(rows) != len(usageLogIDs) {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}

	amount, err := sumUsageCosts(rows)
	if err != nil {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, err
	}

	user, err := tx.User.Query().
		Where(entuser.IDEQ(req.UserID), entuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, err
	}
	balanceBefore, ok := decimalRat(user.Balance)
	if !ok {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	amountRat, ok := decimalRat(amount)
	if !ok {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	balanceAfter := new(big.Rat).Sub(balanceBefore, amountRat)
	disabled := balanceAfter.Sign() < 0

	normalizedCurrency := normalizeCurrency(req.Currency)
	_, err = tx.User.UpdateOneID(user.ID).
		Where(entuser.DeletedAtIsNil()).
		SetBalance(formatRatFixed(balanceAfter, 8)).
		SetCurrency(normalizedCurrency).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, err
	}
	ledgerMetadata := map[string]any{
		"usage_log_ids": usageLogIDs,
		"request_ids":   requestIDs(rows),
		"charged_at":    req.ChargedAt.UTC().Format(time.RFC3339Nano),
		"user_disabled": disabled,
	}
	ledger, err := tx.BillingLedger.Create().
		SetUserID(req.UserID).
		SetType(string(contract.LedgerTypeUsageCharge)).
		SetAmount(amount).
		SetCurrency(normalizedCurrency).
		SetBalanceBefore(formatRatFixed(balanceBefore, 8)).
		SetBalanceAfter(formatRatFixed(balanceAfter, 8)).
		SetReferenceType(strings.TrimSpace(req.ReferenceType)).
		SetReferenceID(strings.TrimSpace(req.ReferenceID)).
		SetMetadataJSON(ledgerMetadata).
		SetCreatedAt(req.ChargedAt).
		SetUpdatedAt(req.ChargedAt).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, err
	}
	updatedCount, err := tx.UsageLog.Update().
		Where(entusagelog.IDIn(usageLogIDs...), entusagelog.ChargedAtIsNil()).
		SetChargedAt(req.ChargedAt).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, err
	}
	if updatedCount != len(rows) {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	if err := tx.Commit(); err != nil {
		return contract.ChargeUsageResult{}, err
	}
	return contract.ChargeUsageResult{
		UserID:             user.ID,
		LedgerEntry:        toLedgerEntry(ledger),
		ChargedUsageLogIDs: usageLogIDs,
		BalanceBefore:      formatRatFixed(balanceBefore, 8),
		BalanceAfter:       formatRatFixed(balanceAfter, 8),
		UserDisabled:       disabled,
	}, nil
}

func toLedgerEntry(row *ent.BillingLedger) contract.LedgerEntry {
	return contract.LedgerEntry{
		ID:            row.ID,
		UserID:        row.UserID,
		Type:          contract.LedgerType(row.Type),
		Amount:        row.Amount,
		Currency:      row.Currency,
		BalanceBefore: row.BalanceBefore,
		BalanceAfter:  row.BalanceAfter,
		ReferenceType: row.ReferenceType,
		ReferenceID:   row.ReferenceID,
		Metadata:      cloneMap(row.MetadataJSON),
		CreatedAt:     row.CreatedAt,
	}
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "USD"
	}
	return value
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	return rat, ok
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	return value.FloatString(places)
}

func sumUsageCosts(rows []*ent.UsageLog) (string, error) {
	total := new(big.Rat)
	for _, row := range rows {
		amount, ok := decimalRat(row.Cost)
		if !ok {
			return "", ErrInvalidStore
		}
		total.Add(total, amount)
	}
	return formatRatFixed(total, 8), nil
}

func uniqueSortedIDs(ids []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func requestIDs(rows []*ent.UsageLog) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.RequestID) != "" {
			out = append(out, row.RequestID)
		}
	}
	return out
}

func usageChargeReferenceID(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	if len(ids) == 1 {
		return strconv.Itoa(ids[0])
	}
	return strconv.Itoa(ids[0]) + "-" + strconv.Itoa(ids[len(ids)-1])
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
