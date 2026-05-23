package contract

import (
	"context"
	"time"
)

type UsageLog struct {
	ID                    int
	RequestID             string
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
	Currency              string
	CompatibilityWarnings []string
	CreatedAt             time.Time
}

type RecordRequest struct {
	RequestID             string
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
	Currency              string
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
