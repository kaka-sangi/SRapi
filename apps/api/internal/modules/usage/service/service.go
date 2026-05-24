package service

import (
	"context"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
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
	sourceProtocol := strings.TrimSpace(req.SourceProtocol)
	if sourceProtocol == "" {
		sourceProtocol = "openai-compatible"
	}
	totalTokens := req.InputTokens + req.OutputTokens + req.CachedTokens
	return s.store.Create(ctx, contract.UsageLog{
		RequestID:             strings.TrimSpace(req.RequestID),
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
		TotalTokens:           totalTokens,
		UsageEstimated:        req.UsageEstimated,
		LatencyMS:             req.LatencyMS,
		Success:               req.Success,
		ErrorClass:            req.ErrorClass,
		Cost:                  cost,
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
				AggregateID:   id,
				AggregateType: dimension,
				Currency:      normalizeCurrency(log.Currency),
				totalCost:     new(big.Rat),
			}
			byID[id] = accumulator
		}
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
		if cost, ok := decimalRat(log.Cost); ok {
			accumulator.totalCost.Add(accumulator.totalCost, cost)
		}
		if accumulator.Currency == "" {
			accumulator.Currency = normalizeCurrency(log.Currency)
		}
	}
	out := make([]contract.UsageAggregate, 0, len(byID))
	for _, accumulator := range byID {
		out = append(out, accumulator.aggregate())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AggregateID < out[j].AggregateID
	})
	return out
}

type usageAccumulator struct {
	AggregateID   string
	AggregateType contract.AggregateDimension
	RequestCount  int
	SuccessCount  int
	ErrorCount    int
	InputTokens   int
	OutputTokens  int
	CachedTokens  int
	TotalTokens   int
	Currency      string
	totalCost     *big.Rat
}

func (a *usageAccumulator) aggregate() contract.UsageAggregate {
	return contract.UsageAggregate{
		AggregateID:   a.AggregateID,
		AggregateType: a.AggregateType,
		RequestCount:  a.RequestCount,
		SuccessCount:  a.SuccessCount,
		ErrorCount:    a.ErrorCount,
		InputTokens:   a.InputTokens,
		OutputTokens:  a.OutputTokens,
		CachedTokens:  a.CachedTokens,
		TotalTokens:   a.TotalTokens,
		TotalCost:     formatRatFixed(a.totalCost, 8),
		Currency:      normalizeCurrency(a.Currency),
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
