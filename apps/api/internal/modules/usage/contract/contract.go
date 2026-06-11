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
	CacheCreationTokens   int
	TotalTokens           int
	UsageEstimated        bool
	LatencyMS             int
	Success               bool
	ErrorClass            *string
	Cost                  string
	ActualCost            string
	RateMultiplier        string
	BillableCost          string
	InputCost             string
	OutputCost            string
	CacheReadCost         string
	CacheWriteCost        string
	RequestedModel        string
	UpstreamModel         string
	BillingMode           string
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
	CacheCreationTokens   int
	UsageEstimated        bool
	LatencyMS             int
	Success               bool
	ErrorClass            *string
	Cost                  string
	ActualCost            string
	RateMultiplier        string
	BillableCost          string
	InputCost             string
	OutputCost            string
	CacheReadCost         string
	CacheWriteCost        string
	RequestedModel        string
	UpstreamModel         string
	BillingMode           string
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

// UserWindowFilter bounds a hot-path user quota summary. Stores must apply the
// user and time predicates before materializing rows.
type UserWindowFilter struct {
	UserID      int
	ProviderID  *int
	Start       time.Time
	End         time.Time
	SuccessOnly bool
}

// UserWindowSummary contains bounded usage totals for one user.
type UserWindowSummary struct {
	UserID       int
	ProviderID   *int
	Start        time.Time
	End          time.Time
	SuccessOnly  bool
	TotalTokens  int
	BillableCost string
}

// AccountWindowFilter bounds account-scoped usage reads used by runtime
// account snapshots. Stores must apply account and time predicates before
// materializing rows and honor Limit when it is positive.
type AccountWindowFilter struct {
	AccountID int
	Start     time.Time
	End       time.Time
	Limit     int
}

// CleanupFilter bounds an operator on-demand deletion of usage records. It
// complements the background retention worker (which only purges by age):
// here an operator can target a model and/or a time range, capped by MaxDelete
// so a single call can never delete more than an intended batch. Model is
// matched case-insensitively. DryRun reports the match count without deleting.
type CleanupFilter struct {
	Model     string
	Start     *time.Time
	End       *time.Time
	DryRun    bool
	MaxDelete int
}

// CleanupResult summarizes one usage-record cleanup pass. Matched counts the
// records the filter selected; Deleted counts those actually removed (always 0
// for a dry run); Limited reports that the MaxDelete cap left matched records
// in place.
type CleanupResult struct {
	Matched   int
	Deleted   int
	DryRun    bool
	MaxDelete int
	Limited   bool
}

// APIKeyUsageSummary contains key-scoped usage aggregates for client-facing Gateway usage snapshots.
type APIKeyUsageSummary struct {
	APIKeyID       int
	WindowDays     int
	RequestCount   int
	SuccessCount   int
	ErrorCount     int
	InputTokens    int
	OutputTokens   int
	CachedTokens   int
	TotalTokens    int
	TotalCost      string
	InputCost      string
	OutputCost     string
	CacheReadCost  string
	CacheWriteCost string
	Currency       string
	Today          UsageAggregate
	ModelStats     []UsageAggregate
	DailyUsage     []UsageAggregate
	RecentLogs     []UsageLog
	GeneratedAt    time.Time
}

// UsageAggregate contains usage totals for one aggregation key and dimension.
type UsageAggregate struct {
	Key            string
	Type           AggregateDimension
	RequestCount   int
	SuccessCount   int
	ErrorCount     int
	InputTokens    int
	OutputTokens   int
	CachedTokens   int
	TotalTokens    int
	TotalCost      string
	InputCost      string
	OutputCost     string
	CacheReadCost  string
	CacheWriteCost string
	Currency       string
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
	ListByAccountWindow(ctx context.Context, filter AccountWindowFilter) ([]UsageLog, error)
	SummarizeUserWindow(ctx context.Context, filter UserWindowFilter) (UserWindowSummary, error)
	// CleanupLogs performs a bounded delete of usage records matching filter.
	// Implementations must honor filter.MaxDelete and filter.DryRun and return
	// the matched/deleted counts so the caller can report whether the cap was hit.
	CleanupLogs(ctx context.Context, filter CleanupFilter) (CleanupResult, error)
}

// WindowReader is an optional Store capability that lists usage logs inside a
// time window with the predicates applied by the store (instead of loading the
// whole table and filtering in memory). Start is inclusive and End exclusive,
// matching QueryFilter semantics. A positive limit caps the result at the
// `limit` newest matching rows; rows are returned in ascending id order either
// way. Admin reporting surfaces prefer this reader when present.
type WindowReader interface {
	ListWindow(ctx context.Context, filter QueryFilter, limit int) ([]UsageLog, error)
}
