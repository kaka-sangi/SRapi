package contract

import (
	"context"
	"time"
)

type UsageLog struct {
	ID                    int
	RequestID             string
	AttemptNo             int
	UserID                int
	APIKeyID              int
	ProviderID            *int
	AccountID             *int
	SourceProtocol        string
	SourceEndpoint        string
	TargetProtocol        string
	Model                 string
	InputTokens           int
	OutputTokens          int
	CachedTokens          int
	TotalTokens           int
	UsageEstimated        bool
	LatencyMS             int
	Success               bool
	ErrorClass            *string
	Cost                  string
	BillableCost          string
	Currency              string
	ChargedAt             *time.Time
	CompatibilityWarnings []string
	CreatedAt             time.Time
}

type RecordRequest struct {
	RequestID             string
	AttemptNo             int
	UserID                int
	APIKeyID              int
	ProviderID            *int
	AccountID             *int
	SourceProtocol        string
	SourceEndpoint        string
	TargetProtocol        string
	Model                 string
	InputTokens           int
	OutputTokens          int
	CachedTokens          int
	UsageEstimated        bool
	LatencyMS             int
	Success               bool
	ErrorClass            *string
	Cost                  string
	BillableCost          string
	Currency              string
	ChargedAt             *time.Time
	CompatibilityWarnings []string
}

type AggregateDimension string

const (
	AggregateDimensionDay     AggregateDimension = "day"
	AggregateDimensionModel   AggregateDimension = "model"
	AggregateDimensionUser    AggregateDimension = "user"
	AggregateDimensionAccount AggregateDimension = "account"
)

type QueryFilter struct {
	Start *time.Time
	End   *time.Time
}

// APIKeyUsageSummary contains key-scoped usage aggregates for client-facing Gateway usage snapshots.
type APIKeyUsageSummary struct {
	APIKeyID     int
	WindowDays   int
	RequestCount int
	SuccessCount int
	ErrorCount   int
	InputTokens  int
	OutputTokens int
	CachedTokens int
	TotalTokens  int
	TotalCost    string
	Currency     string
	Today        UsageWindowSummary
	ModelStats   []UsageModelSummary
	DailyUsage   []UsageDailySummary
	RecentLogs   []UsageLog
	GeneratedAt  time.Time
}

// UsageWindowSummary contains usage totals for one UTC date window.
type UsageWindowSummary struct {
	Date         string
	RequestCount int
	SuccessCount int
	ErrorCount   int
	InputTokens  int
	OutputTokens int
	CachedTokens int
	TotalTokens  int
	TotalCost    string
	Currency     string
}

// UsageModelSummary contains usage totals grouped by canonical model name.
type UsageModelSummary struct {
	Model        string
	RequestCount int
	SuccessCount int
	ErrorCount   int
	InputTokens  int
	OutputTokens int
	CachedTokens int
	TotalTokens  int
	TotalCost    string
	Currency     string
}

// UsageDailySummary contains usage totals grouped by UTC date.
type UsageDailySummary struct {
	Date         string
	RequestCount int
	SuccessCount int
	ErrorCount   int
	InputTokens  int
	OutputTokens int
	CachedTokens int
	TotalTokens  int
	TotalCost    string
	Currency     string
}

type UsageAggregate struct {
	AggregateID   string
	AggregateType AggregateDimension
	RequestCount  int
	SuccessCount  int
	ErrorCount    int
	InputTokens   int
	OutputTokens  int
	CachedTokens  int
	TotalTokens   int
	TotalCost     string
	Currency      string
}

type UsageExport struct {
	Logs        []UsageLog
	Daily       []UsageAggregate
	ByModel     []UsageAggregate
	ByUser      []UsageAggregate
	ByAccount   []UsageAggregate
	GeneratedAt time.Time
}

type Store interface {
	Create(ctx context.Context, input UsageLog) (UsageLog, error)
	List(ctx context.Context) ([]UsageLog, error)
	ListByUser(ctx context.Context, userID int) ([]UsageLog, error)
}
