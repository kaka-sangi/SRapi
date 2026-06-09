package billing

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entbillingledger "github.com/srapi/srapi/apps/api/ent/billingledger"
	entmodelregistry "github.com/srapi/srapi/apps/api/ent/modelregistry"
	entpricinginterval "github.com/srapi/srapi/apps/api/ent/pricinginterval"
	entpricingrule "github.com/srapi/srapi/apps/api/ent/pricingrule"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
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
	req.Currency = money.NormalizeCurrency(req.Currency)
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

	// Serializable isolation prevents a lost-update race when concurrent charges
	// hit the same user's balance (read-modify-write). Matches the affiliate and
	// payments stores; a serialization failure simply leaves the usage logs
	// pending and the charge worker retries on its next pass.
	tx, err := s.client.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return contract.ChargeUsageResult{}, err
	}

	rows, err := tx.UsageLog.Query().
		Where(
			entusagelog.IDIn(usageLogIDs...),
			entusagelog.UserIDEQ(req.UserID),
			entusagelog.SuccessEQ(true),
			entusagelog.CurrencyEQ(money.NormalizeCurrency(req.Currency)),
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
	balanceBefore, ok := money.DecimalRat(user.Balance)
	if !ok {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	amountRat, ok := money.DecimalRat(amount)
	if !ok {
		_ = tx.Rollback()
		return contract.ChargeUsageResult{}, ErrInvalidStore
	}
	balanceAfter := new(big.Rat).Sub(balanceBefore, amountRat)
	disabled := balanceAfter.Sign() < 0

	normalizedCurrency := money.NormalizeCurrency(req.Currency)
	_, err = tx.User.UpdateOneID(user.ID).
		Where(entuser.DeletedAtIsNil()).
		SetBalance(money.FormatRatFixed(balanceAfter, 8)).
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
		SetBalanceBefore(money.FormatRatFixed(balanceBefore, 8)).
		SetBalanceAfter(money.FormatRatFixed(balanceAfter, 8)).
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
		BalanceBefore:      money.FormatRatFixed(balanceBefore, 8),
		BalanceAfter:       money.FormatRatFixed(balanceAfter, 8),
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

func sumUsageCosts(rows []*ent.UsageLog) (string, error) {
	total := new(big.Rat)
	for _, row := range rows {
		// Charge the billable portion (cost minus subscription-allowance coverage,
		// WP-1180). Fall back to full cost if billable was never set.
		value := row.BillableCost
		if strings.TrimSpace(value) == "" {
			value = row.Cost
		}
		amount, ok := money.DecimalRat(value)
		if !ok {
			return "", ErrInvalidStore
		}
		total.Add(total, amount)
	}
	return money.FormatRatFixed(total, 8), nil
}

func (s *Store) CreatePricingRule(ctx context.Context, input contract.PricingRule) (contract.PricingRule, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.PricingRule{}, err
	}
	create := tx.PricingRule.Create().
		SetModelID(input.ModelID).
		SetProviderID(input.ProviderID).
		SetBillingMode(billingModeOrToken(input.BillingMode)).
		SetInputPricePerMillion(moneyOrZero(input.InputPricePerMillionTokens)).
		SetOutputPricePerMillion(moneyOrZero(input.OutputPricePerMillionTokens)).
		SetCacheReadPricePerMillion(moneyOrZero(input.CacheReadPricePerMillionTokens)).
		SetCacheWritePricePerMillion(moneyOrZero(input.CacheWritePricePerMillionTokens)).
		SetPerRequestPrice(moneyOrZero(input.PerRequestPrice)).
		SetCurrency(money.NormalizeCurrency(input.Currency)).
		SetNillableEffectiveFrom(input.EffectiveFrom).
		SetNillableEffectiveTo(input.EffectiveTo)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return contract.PricingRule{}, err
	}
	if err := replacePricingIntervals(ctx, tx, created.ID, input.Intervals); err != nil {
		_ = tx.Rollback()
		return contract.PricingRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return contract.PricingRule{}, err
	}
	return s.FindPricingRuleByID(ctx, created.ID)
}

func (s *Store) UpdatePricingRule(ctx context.Context, id int, input contract.UpdatePricingRuleRequest) (contract.PricingRule, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.PricingRule{}, err
	}
	update := tx.PricingRule.UpdateOneID(id).
		SetNillableInputPricePerMillion(input.InputPricePerMillionTokens).
		SetNillableOutputPricePerMillion(input.OutputPricePerMillionTokens).
		SetNillableCacheReadPricePerMillion(input.CacheReadPricePerMillionTokens).
		SetNillableCacheWritePricePerMillion(input.CacheWritePricePerMillionTokens).
		SetNillablePerRequestPrice(input.PerRequestPrice).
		SetNillableCurrency(input.Currency)
	if input.BillingMode != nil {
		update = update.SetBillingMode(billingModeOrToken(*input.BillingMode))
	}
	if input.EffectiveFrom != nil {
		if *input.EffectiveFrom == nil {
			update = update.ClearEffectiveFrom()
		} else {
			update = update.SetEffectiveFrom(**input.EffectiveFrom)
		}
	}
	if input.EffectiveTo != nil {
		if *input.EffectiveTo == nil {
			update = update.ClearEffectiveTo()
		} else {
			update = update.SetEffectiveTo(**input.EffectiveTo)
		}
	}
	updated, err := update.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.PricingRule{}, contract.ErrNotFound
		}
		return contract.PricingRule{}, err
	}
	if input.Intervals != nil {
		if err := replacePricingIntervals(ctx, tx, id, *input.Intervals); err != nil {
			_ = tx.Rollback()
			return contract.PricingRule{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return contract.PricingRule{}, err
	}
	return s.FindPricingRuleByID(ctx, updated.ID)
}

func (s *Store) FindPricingRuleByID(ctx context.Context, id int) (contract.PricingRule, error) {
	found, err := s.client.PricingRule.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PricingRule{}, contract.ErrNotFound
		}
		return contract.PricingRule{}, err
	}
	intervals, err := s.pricingIntervals(ctx, []int{found.ID})
	if err != nil {
		return contract.PricingRule{}, err
	}
	return toPricingRuleWithFamily(found, "", intervals[found.ID]), nil
}

func (s *Store) ListPricingRules(ctx context.Context) ([]contract.PricingRule, error) {
	rows, err := s.client.PricingRule.Query().
		Order(entpricingrule.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	families, err := s.pricingRuleModelFamilies(ctx, rows)
	if err != nil {
		return nil, err
	}
	intervals, err := s.pricingIntervals(ctx, pricingRuleIDs(rows))
	if err != nil {
		return nil, err
	}
	out := make([]contract.PricingRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPricingRuleWithFamily(row, families[row.ModelID], intervals[row.ID]))
	}
	return out, nil
}

func pricingRuleIDs(rows []*ent.PricingRule) []int {
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func (s *Store) pricingRuleModelFamilies(ctx context.Context, rows []*ent.PricingRule) (map[int]string, error) {
	ids := make([]int, 0, len(rows))
	seen := map[int]struct{}{}
	for _, row := range rows {
		if row.ModelID <= 0 {
			continue
		}
		if _, ok := seen[row.ModelID]; ok {
			continue
		}
		seen[row.ModelID] = struct{}{}
		ids = append(ids, row.ModelID)
	}
	if len(ids) == 0 {
		return map[int]string{}, nil
	}
	models, err := s.client.ModelRegistry.Query().
		Where(entmodelregistry.IDIn(ids...), entmodelregistry.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int]string, len(models))
	for _, model := range models {
		out[model.ID] = model.Family
	}
	return out, nil
}

func (s *Store) DeletePricingRule(ctx context.Context, id int) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.PricingInterval.Delete().Where(entpricinginterval.PricingRuleIDEQ(id)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.PricingRule.DeleteOneID(id).Exec(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return tx.Commit()
}

func toPricingRule(row *ent.PricingRule) contract.PricingRule {
	return toPricingRuleWithFamily(row, "", nil)
}

func toPricingRuleWithFamily(row *ent.PricingRule, modelFamily string, intervals []contract.PricingInterval) contract.PricingRule {
	return contract.PricingRule{
		ID:                              row.ID,
		ModelID:                         row.ModelID,
		ModelFamily:                     modelFamily,
		ProviderID:                      row.ProviderID,
		BillingMode:                     contract.BillingMode(row.BillingMode),
		InputPricePerMillionTokens:      row.InputPricePerMillion,
		OutputPricePerMillionTokens:     row.OutputPricePerMillion,
		CacheReadPricePerMillionTokens:  row.CacheReadPricePerMillion,
		CacheWritePricePerMillionTokens: row.CacheWritePricePerMillion,
		PerRequestPrice:                 row.PerRequestPrice,
		Intervals:                       clonePricingIntervals(intervals),
		Currency:                        row.Currency,
		EffectiveFrom:                   cloneTime(row.EffectiveFrom),
		EffectiveTo:                     cloneTime(row.EffectiveTo),
		CreatedAt:                       row.CreatedAt,
		UpdatedAt:                       row.UpdatedAt,
	}
}

func (s *Store) pricingIntervals(ctx context.Context, ruleIDs []int) (map[int][]contract.PricingInterval, error) {
	if len(ruleIDs) == 0 {
		return map[int][]contract.PricingInterval{}, nil
	}
	rows, err := s.client.PricingInterval.Query().
		Where(entpricinginterval.PricingRuleIDIn(ruleIDs...)).
		Order(entpricinginterval.ByPricingRuleID(), entpricinginterval.ByMinTokens(), entpricinginterval.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int][]contract.PricingInterval, len(ruleIDs))
	for _, row := range rows {
		out[row.PricingRuleID] = append(out[row.PricingRuleID], toPricingInterval(row))
	}
	return out, nil
}

func replacePricingIntervals(ctx context.Context, tx *ent.Tx, pricingRuleID int, intervals []contract.PricingInterval) error {
	if _, err := tx.PricingInterval.Delete().Where(entpricinginterval.PricingRuleIDEQ(pricingRuleID)).Exec(ctx); err != nil {
		return err
	}
	for _, interval := range intervals {
		create := tx.PricingInterval.Create().
			SetPricingRuleID(pricingRuleID).
			SetMinTokens(interval.MinTokens).
			SetNillableMaxTokens(interval.MaxTokens).
			SetTierLabel(interval.TierLabel).
			SetImageSize(interval.ImageSize).
			SetInputPricePerMillion(moneyOrZero(interval.InputPricePerMillionTokens)).
			SetOutputPricePerMillion(moneyOrZero(interval.OutputPricePerMillionTokens)).
			SetCacheReadPricePerMillion(moneyOrZero(interval.CacheReadPricePerMillionTokens)).
			SetCacheWritePricePerMillion(moneyOrZero(interval.CacheWritePricePerMillionTokens)).
			SetPerImagePrice(moneyOrZero(interval.PerImagePrice))
		if !interval.CreatedAt.IsZero() {
			create.SetCreatedAt(interval.CreatedAt).SetUpdatedAt(interval.CreatedAt)
		}
		if _, err := create.Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func toPricingInterval(row *ent.PricingInterval) contract.PricingInterval {
	return contract.PricingInterval{
		ID:                              row.ID,
		PricingRuleID:                   row.PricingRuleID,
		MinTokens:                       row.MinTokens,
		MaxTokens:                       cloneInt(row.MaxTokens),
		TierLabel:                       row.TierLabel,
		ImageSize:                       row.ImageSize,
		InputPricePerMillionTokens:      row.InputPricePerMillion,
		OutputPricePerMillionTokens:     row.OutputPricePerMillion,
		CacheReadPricePerMillionTokens:  row.CacheReadPricePerMillion,
		CacheWritePricePerMillionTokens: row.CacheWritePricePerMillion,
		PerImagePrice:                   row.PerImagePrice,
		CreatedAt:                       row.CreatedAt,
		UpdatedAt:                       row.UpdatedAt,
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func clonePricingIntervals(values []contract.PricingInterval) []contract.PricingInterval {
	if values == nil {
		return nil
	}
	out := make([]contract.PricingInterval, len(values))
	copy(out, values)
	for idx := range out {
		out[idx].MaxTokens = cloneInt(out[idx].MaxTokens)
	}
	return out
}

func billingModeOrToken(value contract.BillingMode) string {
	switch value {
	case contract.BillingModePerRequest, contract.BillingModeImage:
		return string(value)
	default:
		return string(contract.BillingModeToken)
	}
}

func moneyOrZero(value string) string {
	if strings.TrimSpace(value) == "" {
		return money.ZeroAmount
	}
	return value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
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
