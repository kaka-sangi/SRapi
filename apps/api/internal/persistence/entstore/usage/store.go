package usage

import (
	"context"
	stdsql "database/sql"
	"errors"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

var ErrInvalidStore = errors.New("invalid usage ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.UsageLog) (contract.UsageLog, error) {
	create := s.client.UsageLog.Create().
		SetRequestID(input.RequestID).
		SetAttemptNo(input.AttemptNo).
		SetUserID(input.UserID).
		SetAPIKeyID(input.APIKeyID).
		SetNillableProviderID(input.ProviderID).
		SetNillableAccountID(input.AccountID).
		SetSourceProtocol(input.SourceProtocol).
		SetSourceEndpoint(input.SourceEndpoint).
		SetTargetProtocol(input.TargetProtocol).
		SetModel(input.Model).
		SetInputTokens(input.InputTokens).
		SetOutputTokens(input.OutputTokens).
		SetCachedTokens(input.CachedTokens).
		SetCacheCreationTokens(input.CacheCreationTokens).
		SetTotalTokens(input.TotalTokens).
		SetUsageEstimated(input.UsageEstimated).
		SetLatencyMs(input.LatencyMS).
		SetSuccess(input.Success).
		SetNillableErrorClass(input.ErrorClass).
		SetCost(input.Cost).
		SetActualCost(actualCostOrCost(input.ActualCost, input.Cost)).
		SetRateMultiplier(rateMultiplierOrDefault(input.RateMultiplier)).
		SetBillableCost(billableCostOrActualCost(input.BillableCost, input.ActualCost, input.Cost)).
		SetInputCost(moneyOrZero(input.InputCost)).
		SetOutputCost(moneyOrZero(input.OutputCost)).
		SetCacheReadCost(moneyOrZero(input.CacheReadCost)).
		SetCacheWriteCost(moneyOrZero(input.CacheWriteCost)).
		SetRequestedModel(requestedModelOrModel(input.RequestedModel, input.Model)).
		SetUpstreamModel(strings.TrimSpace(input.UpstreamModel)).
		SetBillingMode(billingModeOrToken(input.BillingMode)).
		SetCurrency(input.Currency).
		SetNillableChargedAt(input.ChargedAt).
		SetCompatibilityWarningsJSON(cloneStrings(input.CompatibilityWarnings))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.UsageLog{}, err
	}
	return toUsageLog(created), nil
}

func (s *Store) List(ctx context.Context) ([]contract.UsageLog, error) {
	rows, err := s.client.UsageLog.Query().
		Order(entusagelog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return toUsageLogs(rows), nil
}

// ListWindow lists usage logs inside [Start, End) with the predicates applied
// in SQL. A positive limit keeps the newest matching rows; output is ascending
// by id either way. Implements contract.WindowReader.
func (s *Store) ListWindow(ctx context.Context, filter contract.QueryFilter, limit int) ([]contract.UsageLog, error) {
	query := s.client.UsageLog.Query()
	if filter.Start != nil {
		query = query.Where(entusagelog.CreatedAtGTE(*filter.Start))
	}
	if filter.End != nil {
		query = query.Where(entusagelog.CreatedAtLT(*filter.End))
	}
	query = query.Order(ent.Desc(entusagelog.FieldID))
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := toUsageLogs(rows)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ListPage implements contract.PageReader: filter, count, and slice in SQL
// with ORDER BY id DESC so the newest matching rows come back first. Avoids
// the prior pattern of loading every usage row before paginating in Go memory,
// which was the main contributor to slow /usage and /admin/usage page loads
// on busy gateways. A non-positive limit returns every matching row.
func (s *Store) ListPage(ctx context.Context, filter contract.ListFilter, limit, offset int) (contract.ListPageResult, error) {
	predicates := listPagePredicates(filter)
	base := s.client.UsageLog.Query()
	if len(predicates) > 0 {
		base = base.Where(predicates...)
	}
	total, err := base.Clone().Count(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	query := base.Order(ent.Desc(entusagelog.FieldID))
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	return contract.ListPageResult{Items: toUsageLogs(rows), Total: total}, nil
}

func listPagePredicates(filter contract.ListFilter) []predicate.UsageLog {
	predicates := make([]predicate.UsageLog, 0, 8)
	if filter.UserID != nil {
		predicates = append(predicates, entusagelog.UserIDEQ(*filter.UserID))
	}
	if filter.APIKeyID != nil {
		predicates = append(predicates, entusagelog.APIKeyIDEQ(*filter.APIKeyID))
	}
	if filter.AccountID != nil {
		predicates = append(predicates, entusagelog.AccountIDEQ(*filter.AccountID))
	}
	if filter.ProviderID != nil {
		predicates = append(predicates, entusagelog.ProviderIDEQ(*filter.ProviderID))
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		predicates = append(predicates, entusagelog.ModelContainsFold(model))
	}
	if endpoint := strings.TrimSpace(filter.SourceEndpoint); endpoint != "" {
		predicates = append(predicates, entusagelog.SourceEndpointContainsFold(endpoint))
	}
	if mode := strings.TrimSpace(filter.BillingMode); mode != "" {
		predicates = append(predicates, entusagelog.BillingModeEqualFold(mode))
	}
	if class := strings.TrimSpace(filter.ErrorClass); class != "" {
		predicates = append(predicates, entusagelog.ErrorClassEqualFold(class))
	}
	if filter.Success != nil {
		predicates = append(predicates, entusagelog.SuccessEQ(*filter.Success))
	}
	if filter.Start != nil {
		predicates = append(predicates, entusagelog.CreatedAtGTE(filter.Start.UTC()))
	}
	if filter.End != nil {
		predicates = append(predicates, entusagelog.CreatedAtLT(filter.End.UTC()))
	}
	if needle := strings.TrimSpace(filter.Q); needle != "" {
		// Only request_id is persisted in the ent schema today — upstream and
		// provider error message live on the Go struct but never reach the DB,
		// so SQL search is naturally limited to gateway request_id substrings.
		predicates = append(predicates, entusagelog.RequestIDContainsFold(needle))
	}
	return predicates
}

// ListByRequestID lists all usage attempts for one exact gateway request id.
// Implements contract.RequestReader for operator drilldowns.
func (s *Store) ListByRequestID(ctx context.Context, requestID string) ([]contract.UsageLog, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return []contract.UsageLog{}, nil
	}
	rows, err := s.client.UsageLog.Query().
		Where(entusagelog.RequestIDEQ(requestID)).
		Order(entusagelog.ByAttemptNo(), entusagelog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return toUsageLogs(rows), nil
}

func (s *Store) ListByUser(ctx context.Context, userID int) ([]contract.UsageLog, error) {
	rows, err := s.client.UsageLog.Query().
		Where(entusagelog.UserIDEQ(userID)).
		Order(entusagelog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return toUsageLogs(rows), nil
}

func (s *Store) ListByAccountWindow(ctx context.Context, filter contract.AccountWindowFilter) ([]contract.UsageLog, error) {
	query := s.client.UsageLog.Query().
		Where(
			entusagelog.AccountIDEQ(filter.AccountID),
			entusagelog.CreatedAtGTE(filter.Start.UTC()),
			entusagelog.CreatedAtLT(filter.End.UTC()),
		)
	if filter.Limit > 0 {
		query = query.Order(ent.Desc(entusagelog.FieldID)).Limit(filter.Limit)
	} else {
		query = query.Order(entusagelog.ByID())
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	if filter.Limit > 0 {
		for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
			rows[left], rows[right] = rows[right], rows[left]
		}
	}
	return toUsageLogs(rows), nil
}

func (s *Store) SummarizeUserWindow(ctx context.Context, filter contract.UserWindowFilter) (contract.UserWindowSummary, error) {
	predicates := []predicate.UsageLog{
		entusagelog.UserIDEQ(filter.UserID),
		entusagelog.CreatedAtGTE(filter.Start.UTC()),
		entusagelog.CreatedAtLT(filter.End.UTC()),
	}
	if filter.SuccessOnly {
		predicates = append(predicates, entusagelog.SuccessEQ(true))
	}
	if filter.ProviderID != nil {
		predicates = append(predicates, entusagelog.ProviderIDEQ(*filter.ProviderID))
	}
	var rows []userWindowSummaryRow
	err := s.client.UsageLog.Query().
		Where(predicates...).
		Aggregate(
			ent.As(ent.Sum(entusagelog.FieldTotalTokens), "total_tokens"),
			ent.As(sumBillableCost(), "billable_cost"),
		).
		Scan(ctx, &rows)
	if err != nil {
		return contract.UserWindowSummary{}, err
	}
	summary := contract.UserWindowSummary{
		UserID:      filter.UserID,
		ProviderID:  cloneInt(filter.ProviderID),
		Start:       filter.Start.UTC(),
		End:         filter.End.UTC(),
		SuccessOnly: filter.SuccessOnly,
	}
	if len(rows) == 0 {
		summary.BillableCost = "0.00000000"
		return summary, nil
	}
	summary.TotalTokens = int(rows[0].TotalTokens.Int64)
	summary.BillableCost = normalizeSummaryCost(rows[0].BillableCost)
	return summary, nil
}

type userWindowSummaryRow struct {
	TotalTokens  stdsql.NullInt64  `sql:"total_tokens"`
	BillableCost stdsql.NullString `sql:"billable_cost"`
}

func sumBillableCost() ent.AggregateFunc {
	return func(selector *sql.Selector) string {
		return "COALESCE(SUM(CAST(" + selector.C(entusagelog.FieldBillableCost) + " AS NUMERIC)), 0)"
	}
}

func normalizeSummaryCost(value stdsql.NullString) string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return "0.00000000"
	}
	if cost, ok := money.DecimalRat(value.String); ok {
		return money.FormatRatFixed(cost, 8)
	}
	return "0.00000000"
}

// CleanupLogs counts the matching records and deletes the oldest up to
// filter.MaxDelete. The cap is enforced by selecting the oldest matching IDs
// first (ordered by ID) and deleting only those, so a single call never removes
// more than the intended batch. DryRun returns the match count without deleting.
func (s *Store) CleanupLogs(ctx context.Context, filter contract.CleanupFilter) (contract.CleanupResult, error) {
	predicates := cleanupPredicates(filter)
	matched, err := s.client.UsageLog.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return contract.CleanupResult{}, err
	}
	result := contract.CleanupResult{
		Matched:   matched,
		DryRun:    filter.DryRun,
		MaxDelete: filter.MaxDelete,
	}
	if filter.DryRun {
		result.Limited = matched > filter.MaxDelete
		return result, nil
	}
	ids, err := s.client.UsageLog.Query().
		Where(predicates...).
		Order(entusagelog.ByID()).
		Limit(filter.MaxDelete).
		IDs(ctx)
	if err != nil {
		return contract.CleanupResult{}, err
	}
	if len(ids) > 0 {
		deleted, err := s.client.UsageLog.Delete().
			Where(entusagelog.IDIn(ids...)).
			Exec(ctx)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.Deleted = deleted
	}
	result.Limited = matched > result.Deleted
	return result, nil
}

func cleanupPredicates(filter contract.CleanupFilter) []predicate.UsageLog {
	predicates := make([]predicate.UsageLog, 0, 3)
	if model := strings.TrimSpace(filter.Model); model != "" {
		predicates = append(predicates, entusagelog.ModelEqualFold(model))
	}
	if filter.Start != nil {
		predicates = append(predicates, entusagelog.CreatedAtGTE(filter.Start.UTC()))
	}
	if filter.End != nil {
		predicates = append(predicates, entusagelog.CreatedAtLT(filter.End.UTC()))
	}
	return predicates
}

func toUsageLogs(rows []*ent.UsageLog) []contract.UsageLog {
	out := make([]contract.UsageLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, toUsageLog(row))
	}
	return out
}

func toUsageLog(row *ent.UsageLog) contract.UsageLog {
	return contract.UsageLog{
		ID:                    row.ID,
		RequestID:             row.RequestID,
		AttemptNo:             row.AttemptNo,
		UserID:                row.UserID,
		APIKeyID:              row.APIKeyID,
		ProviderID:            cloneInt(row.ProviderID),
		AccountID:             cloneInt(row.AccountID),
		SourceProtocol:        row.SourceProtocol,
		SourceEndpoint:        row.SourceEndpoint,
		TargetProtocol:        row.TargetProtocol,
		Model:                 row.Model,
		InputTokens:           row.InputTokens,
		OutputTokens:          row.OutputTokens,
		CachedTokens:          row.CachedTokens,
		CacheCreationTokens:   row.CacheCreationTokens,
		TotalTokens:           row.TotalTokens,
		UsageEstimated:        row.UsageEstimated,
		LatencyMS:             row.LatencyMs,
		Success:               row.Success,
		ErrorClass:            cloneString(row.ErrorClass),
		Cost:                  row.Cost,
		ActualCost:            row.ActualCost,
		RateMultiplier:        row.RateMultiplier,
		BillableCost:          row.BillableCost,
		InputCost:             row.InputCost,
		OutputCost:            row.OutputCost,
		CacheReadCost:         row.CacheReadCost,
		CacheWriteCost:        row.CacheWriteCost,
		RequestedModel:        row.RequestedModel,
		UpstreamModel:         row.UpstreamModel,
		BillingMode:           row.BillingMode,
		Currency:              row.Currency,
		ChargedAt:             cloneTime(row.ChargedAt),
		CompatibilityWarnings: cloneStrings(row.CompatibilityWarningsJSON),
		CreatedAt:             row.CreatedAt,
	}
}

func actualCostOrCost(actualCost, cost string) string {
	if strings.TrimSpace(actualCost) == "" {
		return cost
	}
	return actualCost
}

func rateMultiplierOrDefault(rateMultiplier string) string {
	if strings.TrimSpace(rateMultiplier) == "" {
		return "1.00000000"
	}
	return rateMultiplier
}

// billableCostOrActualCost falls back to actual_cost, then cost, preserving the
// pre-multiplier behavior for callers that do not compute subscription coverage.
func billableCostOrActualCost(billable, actualCost, cost string) string {
	if strings.TrimSpace(billable) != "" {
		return billable
	}
	if strings.TrimSpace(actualCost) != "" {
		return actualCost
	}
	return cost
}

func moneyOrZero(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0.00000000"
	}
	return value
}

func requestedModelOrModel(requestedModel, model string) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return strings.TrimSpace(model)
	}
	return requestedModel
}

func billingModeOrToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "token"
	}
	return value
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
