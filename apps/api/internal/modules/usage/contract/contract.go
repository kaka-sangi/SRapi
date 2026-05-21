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

type Store interface {
	Create(ctx context.Context, input UsageLog) (UsageLog, error)
	List(ctx context.Context) ([]UsageLog, error)
	ListByUser(ctx context.Context, userID int) ([]UsageLog, error)
}
