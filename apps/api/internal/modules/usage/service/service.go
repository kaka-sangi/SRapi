package service

import (
	"context"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store contract.Store
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Record(ctx context.Context, req contract.RecordRequest) (contract.UsageLog, error) {
	if strings.TrimSpace(req.RequestID) == "" || req.UserID <= 0 || req.APIKeyID <= 0 || strings.TrimSpace(req.SourceEndpoint) == "" || strings.TrimSpace(req.Model) == "" {
		return contract.UsageLog{}, ErrInvalidInput
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "USD"
	}
	cost := strings.TrimSpace(req.Cost)
	if cost == "" {
		cost = "0.00000000"
	}
	actualCost := strings.TrimSpace(req.ActualCost)
	if actualCost == "" {
		actualCost = cost
	}
	rateMultiplier := strings.TrimSpace(req.RateMultiplier)
	if rateMultiplier == "" {
		rateMultiplier = "1.00000000"
	}
	billableCost := strings.TrimSpace(req.BillableCost)
	if billableCost == "" {
		billableCost = actualCost
	}
	inputCost := strings.TrimSpace(req.InputCost)
	if inputCost == "" {
		inputCost = "0.00000000"
	}
	outputCost := strings.TrimSpace(req.OutputCost)
	if outputCost == "" {
		outputCost = "0.00000000"
	}
	cacheReadCost := strings.TrimSpace(req.CacheReadCost)
	if cacheReadCost == "" {
		cacheReadCost = "0.00000000"
	}
	cacheWriteCost := strings.TrimSpace(req.CacheWriteCost)
	if cacheWriteCost == "" {
		cacheWriteCost = "0.00000000"
	}
	requestedModel := strings.TrimSpace(req.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(req.Model)
	}
	billingMode := strings.TrimSpace(req.BillingMode)
	if billingMode == "" {
		billingMode = "token"
	}
	sourceProtocol := strings.TrimSpace(req.SourceProtocol)
	if sourceProtocol == "" {
		sourceProtocol = "openai-compatible"
	}
	totalTokens := req.InputTokens + req.OutputTokens + req.CachedTokens + req.CacheCreationTokens
	attemptNo := req.AttemptNo
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return s.store.Create(ctx, contract.UsageLog{
		RequestID:             strings.TrimSpace(req.RequestID),
		AttemptNo:             attemptNo,
		UserID:                req.UserID,
		APIKeyID:              req.APIKeyID,
		ProviderID:            req.ProviderID,
		AccountID:             req.AccountID,
		SourceProtocol:        sourceProtocol,
		SourceEndpoint:        strings.TrimSpace(req.SourceEndpoint),
		TargetProtocol:        strings.TrimSpace(req.TargetProtocol),
		Model:                 strings.TrimSpace(req.Model),
		InputTokens:           req.InputTokens,
		OutputTokens:          req.OutputTokens,
		CachedTokens:          req.CachedTokens,
		CacheCreationTokens:   req.CacheCreationTokens,
		TotalTokens:           totalTokens,
		UsageEstimated:        req.UsageEstimated,
		LatencyMS:             req.LatencyMS,
		Success:               req.Success,
		ErrorClass:            req.ErrorClass,
		Cost:                  cost,
		ActualCost:            actualCost,
		RateMultiplier:        rateMultiplier,
		BillableCost:          billableCost,
		InputCost:             inputCost,
		OutputCost:            outputCost,
		CacheReadCost:         cacheReadCost,
		CacheWriteCost:        cacheWriteCost,
		RequestedModel:        requestedModel,
		UpstreamModel:         strings.TrimSpace(req.UpstreamModel),
		BillingMode:           billingMode,
		Currency:              currency,
		ChargedAt:             req.ChargedAt,
		CompatibilityWarnings: cloneStrings(req.CompatibilityWarnings),
		CreatedAt:             s.clock.Now(),
	})
}

func (s *Service) List(ctx context.Context) ([]contract.UsageLog, error) {
	return s.store.List(ctx)
}

func (s *Service) ListFiltered(ctx context.Context, filter contract.QueryFilter) ([]contract.UsageLog, error) {
	logs, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	return filterLogs(logs, filter), nil
}

func (s *Service) ListByUser(ctx context.Context, userID int) ([]contract.UsageLog, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListByUser(ctx, userID)
}

func (s *Service) ListByAccountWindow(ctx context.Context, filter contract.AccountWindowFilter) ([]contract.UsageLog, error) {
	if filter.AccountID <= 0 || filter.Start.IsZero() || filter.End.IsZero() || !filter.End.After(filter.Start) {
		return nil, ErrInvalidInput
	}
	filter.Start = filter.Start.UTC()
	filter.End = filter.End.UTC()
	return s.store.ListByAccountWindow(ctx, filter)
}

func (s *Service) SummarizeUserWindow(ctx context.Context, filter contract.UserWindowFilter) (contract.UserWindowSummary, error) {
	if filter.UserID <= 0 || filter.Start.IsZero() || filter.End.IsZero() || !filter.End.After(filter.Start) {
		return contract.UserWindowSummary{}, ErrInvalidInput
	}
	filter.Start = filter.Start.UTC()
	filter.End = filter.End.UTC()
	return s.store.SummarizeUserWindow(ctx, filter)
}

// SummarizeAPIKey returns recent usage aggregates scoped to a single Gateway API key.
func (s *Service) SummarizeAPIKey(ctx context.Context, apiKeyID int, windowDays int) (contract.APIKeyUsageSummary, error) {
	if apiKeyID <= 0 {
		return contract.APIKeyUsageSummary{}, ErrInvalidInput
	}
	if windowDays <= 0 {
		windowDays = 30
	}
	if windowDays > 90 {
		windowDays = 90
	}
	now := s.clock.Now().UTC()
	windowStart := startOfUTCDay(now).AddDate(0, 0, -(windowDays - 1))
	allLogs, err := s.store.List(ctx)
	if err != nil {
		return contract.APIKeyUsageSummary{}, err
	}
	logs := make([]contract.UsageLog, 0)
	for _, log := range allLogs {
		if log.APIKeyID != apiKeyID || log.CreatedAt.Before(windowStart) {
			continue
		}
		logs = append(logs, log)
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].CreatedAt.Equal(logs[j].CreatedAt) {
			return logs[i].ID < logs[j].ID
		}
		return logs[i].CreatedAt.Before(logs[j].CreatedAt)
	})

	summary := contract.APIKeyUsageSummary{
		APIKeyID:    apiKeyID,
		WindowDays:  windowDays,
		Currency:    "USD",
		GeneratedAt: now,
	}
	byModel := map[string]*usageAccumulator{}
	byDay := map[string]*usageAccumulator{}
	todayID := now.Format("2006-01-02")
	today := newUsageAccumulator(todayID, contract.AggregateDimensionDay, "USD")
	totalCost := new(big.Rat)
	for _, log := range logs {
		accumulateUsageSummary(&summary, log)
		if cost, ok := money.DecimalRat(log.Cost); ok {
			totalCost.Add(totalCost, cost)
		}
		if summary.Currency == "" || summary.Currency == "USD" {
			summary.Currency = money.NormalizeCurrency(log.Currency)
		}
		modelID := strings.TrimSpace(log.Model)
		if modelID == "" {
			modelID = "unknown"
		}
		accumulateUsageLog(accumulatorFor(byModel, modelID, contract.AggregateDimensionModel, log.Currency), log)
		dayID := log.CreatedAt.UTC().Format("2006-01-02")
		accumulateUsageLog(accumulatorFor(byDay, dayID, contract.AggregateDimensionDay, log.Currency), log)
		if dayID == todayID {
			accumulateUsageLog(today, log)
		}
	}
	summary.TotalCost = money.FormatRatFixed(totalCost, 8)
	summary.Today = today.aggregate()
	summary.ModelStats = aggregateAccumulatorValues(byModel, sortAggregatesByTokensDesc)
	summary.DailyUsage = aggregateAccumulatorValues(byDay, sortAggregatesByKeyAsc)
	summary.RecentLogs = recentUsageLogs(logs, 20)
	return summary, nil
}

func (s *Service) Aggregate(ctx context.Context, filter contract.QueryFilter, dimension contract.AggregateDimension) ([]contract.UsageAggregate, error) {
	if !validAggregateDimension(dimension) {
		return nil, ErrInvalidInput
	}
	logs, err := s.ListFiltered(ctx, filter)
	if err != nil {
		return nil, err
	}
	return aggregateLogs(logs, dimension), nil
}

func (s *Service) Export(ctx context.Context, filter contract.QueryFilter) (contract.UsageExport, error) {
	logs, err := s.ListFiltered(ctx, filter)
	if err != nil {
		return contract.UsageExport{}, err
	}
	return contract.UsageExport{
		Logs:        logs,
		Daily:       aggregateLogs(logs, contract.AggregateDimensionDay),
		ByModel:     aggregateLogs(logs, contract.AggregateDimensionModel),
		ByUser:      aggregateLogs(logs, contract.AggregateDimensionUser),
		ByAccount:   aggregateLogs(logs, contract.AggregateDimensionAccount),
		GeneratedAt: s.clock.Now(),
	}, nil
}

const (
	defaultCleanupMaxDelete = 1000
	maxCleanupMaxDelete     = 10000
)

// CleanupLogs performs a bounded, operator-triggered deletion of usage records.
// It requires at least one bounding filter (model, start, or end) so a cleanup
// can never accidentally target the whole table, and caps the batch at
// MaxDelete. DryRun reports the match count without deleting anything.
func (s *Service) CleanupLogs(ctx context.Context, filter contract.CleanupFilter) (contract.CleanupResult, error) {
	normalized, err := normalizeCleanupFilter(filter)
	if err != nil {
		return contract.CleanupResult{}, err
	}
	return s.store.CleanupLogs(ctx, normalized)
}

func normalizeCleanupFilter(filter contract.CleanupFilter) (contract.CleanupFilter, error) {
	filter.Model = strings.TrimSpace(filter.Model)
	if filter.Start != nil {
		start := filter.Start.UTC()
		filter.Start = &start
	}
	if filter.End != nil {
		end := filter.End.UTC()
		filter.End = &end
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return contract.CleanupFilter{}, ErrInvalidInput
	}
	if filter.Model == "" && filter.Start == nil && filter.End == nil {
		return contract.CleanupFilter{}, ErrInvalidInput
	}
	if filter.MaxDelete == 0 {
		filter.MaxDelete = defaultCleanupMaxDelete
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > maxCleanupMaxDelete {
		return contract.CleanupFilter{}, ErrInvalidInput
	}
	return filter, nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func validAggregateDimension(dimension contract.AggregateDimension) bool {
	switch dimension {
	case contract.AggregateDimensionDay, contract.AggregateDimensionModel, contract.AggregateDimensionUser, contract.AggregateDimensionAccount:
		return true
	default:
		return false
	}
}

func filterLogs(logs []contract.UsageLog, filter contract.QueryFilter) []contract.UsageLog {
	out := make([]contract.UsageLog, 0, len(logs))
	for _, log := range logs {
		if filter.Start != nil && log.CreatedAt.Before(*filter.Start) {
			continue
		}
		if filter.End != nil && !log.CreatedAt.Before(*filter.End) {
			continue
		}
		out = append(out, log)
	}
	return out
}

func aggregateLogs(logs []contract.UsageLog, dimension contract.AggregateDimension) []contract.UsageAggregate {
	byID := map[string]*usageAccumulator{}
	for _, log := range logs {
		id := aggregateID(log, dimension)
		if id == "" {
			id = "unknown"
		}
		accumulator := byID[id]
		if accumulator == nil {
			accumulator = &usageAccumulator{
				Key:            id,
				Type:           dimension,
				Currency:       money.NormalizeCurrency(log.Currency),
				totalCost:      new(big.Rat),
				inputCost:      new(big.Rat),
				outputCost:     new(big.Rat),
				cacheReadCost:  new(big.Rat),
				cacheWriteCost: new(big.Rat),
			}
			byID[id] = accumulator
		}
		accumulateUsageLog(accumulator, log)
	}
	out := make([]contract.UsageAggregate, 0, len(byID))
	for _, accumulator := range byID {
		out = append(out, accumulator.aggregate())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

type usageAccumulator struct {
	Key            string
	Type           contract.AggregateDimension
	RequestCount   int
	SuccessCount   int
	ErrorCount     int
	InputTokens    int
	OutputTokens   int
	CachedTokens   int
	TotalTokens    int
	Currency       string
	totalCost      *big.Rat
	inputCost      *big.Rat
	outputCost     *big.Rat
	cacheReadCost  *big.Rat
	cacheWriteCost *big.Rat
}

func (a *usageAccumulator) aggregate() contract.UsageAggregate {
	return contract.UsageAggregate{
		Key:            a.Key,
		Type:           a.Type,
		RequestCount:   a.RequestCount,
		SuccessCount:   a.SuccessCount,
		ErrorCount:     a.ErrorCount,
		InputTokens:    a.InputTokens,
		OutputTokens:   a.OutputTokens,
		CachedTokens:   a.CachedTokens,
		TotalTokens:    a.TotalTokens,
		TotalCost:      money.FormatRatFixed(a.totalCost, 8),
		InputCost:      money.FormatRatFixed(a.inputCost, 8),
		OutputCost:     money.FormatRatFixed(a.outputCost, 8),
		CacheReadCost:  money.FormatRatFixed(a.cacheReadCost, 8),
		CacheWriteCost: money.FormatRatFixed(a.cacheWriteCost, 8),
		Currency:       money.NormalizeCurrency(a.Currency),
	}
}

func aggregateID(log contract.UsageLog, dimension contract.AggregateDimension) string {
	switch dimension {
	case contract.AggregateDimensionDay:
		return log.CreatedAt.UTC().Format("2006-01-02")
	case contract.AggregateDimensionModel:
		return strings.TrimSpace(log.Model)
	case contract.AggregateDimensionUser:
		return strconv.Itoa(log.UserID)
	case contract.AggregateDimensionAccount:
		if log.AccountID == nil {
			return "unknown"
		}
		return strconv.Itoa(*log.AccountID)
	default:
		return ""
	}
}

func accumulateUsageSummary(summary *contract.APIKeyUsageSummary, log contract.UsageLog) {
	summary.RequestCount++
	if log.Success {
		summary.SuccessCount++
	} else {
		summary.ErrorCount++
	}
	summary.InputTokens += log.InputTokens
	summary.OutputTokens += log.OutputTokens
	summary.CachedTokens += log.CachedTokens
	summary.TotalTokens += log.TotalTokens
	summary.TotalCost = money.AddMoney(summary.TotalCost, log.Cost)
	summary.InputCost = money.AddMoney(summary.InputCost, log.InputCost)
	summary.OutputCost = money.AddMoney(summary.OutputCost, log.OutputCost)
	summary.CacheReadCost = money.AddMoney(summary.CacheReadCost, log.CacheReadCost)
	summary.CacheWriteCost = money.AddMoney(summary.CacheWriteCost, log.CacheWriteCost)
}

func accumulateUsageLog(accumulator *usageAccumulator, log contract.UsageLog) {
	accumulator.RequestCount++
	if log.Success {
		accumulator.SuccessCount++
	} else {
		accumulator.ErrorCount++
	}
	accumulator.InputTokens += log.InputTokens
	accumulator.OutputTokens += log.OutputTokens
	accumulator.CachedTokens += log.CachedTokens
	accumulator.TotalTokens += log.TotalTokens
	if cost, ok := money.DecimalRat(log.Cost); ok {
		accumulator.totalCost.Add(accumulator.totalCost, cost)
	}
	addCost(accumulator.inputCost, log.InputCost)
	addCost(accumulator.outputCost, log.OutputCost)
	addCost(accumulator.cacheReadCost, log.CacheReadCost)
	addCost(accumulator.cacheWriteCost, log.CacheWriteCost)
	if accumulator.Currency == "" || accumulator.Currency == "USD" {
		accumulator.Currency = money.NormalizeCurrency(log.Currency)
	}
}

func addCost(total *big.Rat, amount string) {
	if cost, ok := money.DecimalRat(amount); ok {
		total.Add(total, cost)
	}
}

func accumulatorFor(values map[string]*usageAccumulator, id string, dimension contract.AggregateDimension, currency string) *usageAccumulator {
	accumulator := values[id]
	if accumulator != nil {
		return accumulator
	}
	accumulator = newUsageAccumulator(id, dimension, currency)
	values[id] = accumulator
	return accumulator
}

func newUsageAccumulator(id string, dimension contract.AggregateDimension, currency string) *usageAccumulator {
	return &usageAccumulator{
		Key:            id,
		Type:           dimension,
		Currency:       money.NormalizeCurrency(currency),
		totalCost:      new(big.Rat),
		inputCost:      new(big.Rat),
		outputCost:     new(big.Rat),
		cacheReadCost:  new(big.Rat),
		cacheWriteCost: new(big.Rat),
	}
}

type aggregateSort func(out []contract.UsageAggregate)

func aggregateAccumulatorValues(values map[string]*usageAccumulator, sortFn aggregateSort) []contract.UsageAggregate {
	out := make([]contract.UsageAggregate, 0, len(values))
	for _, accumulator := range values {
		out = append(out, accumulator.aggregate())
	}
	sortFn(out)
	return out
}

func sortAggregatesByTokensDesc(out []contract.UsageAggregate) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalTokens == out[j].TotalTokens {
			return out[i].Key < out[j].Key
		}
		return out[i].TotalTokens > out[j].TotalTokens
	})
}

func sortAggregatesByKeyAsc(out []contract.UsageAggregate) {
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
}

func recentUsageLogs(logs []contract.UsageLog, limit int) []contract.UsageLog {
	if limit <= 0 || len(logs) == 0 {
		return nil
	}
	start := len(logs) - limit
	if start < 0 {
		start = 0
	}
	out := make([]contract.UsageLog, 0, len(logs)-start)
	for i := len(logs) - 1; i >= start; i-- {
		out = append(out, logs[i])
	}
	return out
}

func startOfUTCDay(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
