package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Store.UpdateResolved when no row matches id.
var ErrNotFound = errors.New("usage log not found")

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
	// ProviderErrorMessage carries the upstream's verbatim error.message
	// (truncated + redacted) so operators can see what Codex / OpenAI /
	// Anthropic actually returned for a failed request. Mirrors sub2api's
	// ops_error_logs.upstream_error_message (migrations/034_ops_upstream_error_events.sql).
	// Empty for successful requests.
	ProviderErrorMessage string
	// ProviderErrorBodyExcerpt is a bounded slice of the raw upstream
	// response body. Mirrors sub2api's upstream_error_detail field — kept
	// short enough to be safe to inline in the admin panel but long enough
	// to surface the relevant error code/field. Empty for successful requests.
	ProviderErrorBodyExcerpt string
	// StatusCode is the upstream HTTP status code for the final failing
	// attempt (0 when no HTTP response was ever received, e.g. transport
	// failure). Mirrors sub2api ops_error_logs.status_code.
	StatusCode int
	// UpstreamRequestID carries the upstream provider's request id (the
	// value of x-request-id / openai-request-id / x-codex-request-id on
	// the failing response). Empty when the upstream did not return one.
	UpstreamRequestID string
	// ErrorPhase / ErrorOwner / ErrorSource classify the failure for the
	// admin panel. Phase: request|auth|routing|upstream|network|internal.
	// Owner: client|provider|platform. Source: client_request|upstream_http|gateway.
	ErrorPhase  string
	ErrorOwner  string
	ErrorSource string
	// Resolved marks an operator-acknowledged error log; ResolvedBy /
	// ResolvedAt record who and when.
	Resolved   bool
	ResolvedBy *int
	ResolvedAt *time.Time
	// UpstreamErrors is the per-attempt history mirroring sub2api's
	// ops_upstream_error_events — one entry per failed candidate attempt.
	UpstreamErrors        []UpstreamErrorEvent
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

// UpstreamErrorEvent captures one failed candidate attempt for an aggregate
// gateway request. Multiple events under the same RequestID form the failover
// timeline shown in the admin panel. Mirrors sub2api's ops_upstream_error_events
// row (one per attempt). AtUnixMs is wall-clock ms since epoch when the failure
// was observed; AttemptNo is the cross-candidate attempt index (1-based).
type UpstreamErrorEvent struct {
	AtUnixMs           int64
	AttemptNo          int
	AccountID          *int
	AccountName        string
	UpstreamStatusCode int
	UpstreamRequestID  string
	UpstreamURL        string
	// Kind is one of: http_error / request_error / retry_exhausted / failover.
	Kind        string
	Message     string
	BodyExcerpt string
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
	// ProviderErrorMessage / ProviderErrorBodyExcerpt carry the upstream's
	// verbatim error.message + body excerpt for failed requests. See the
	// matching fields on UsageLog for the sub2api parity rationale.
	ProviderErrorMessage     string
	ProviderErrorBodyExcerpt string
	// New: see UsageLog for the per-field rationale.
	StatusCode            int
	UpstreamRequestID     string
	ErrorPhase            string
	ErrorOwner            string
	ErrorSource           string
	Resolved              bool
	ResolvedBy            *int
	ResolvedAt            *time.Time
	UpstreamErrors        []UpstreamErrorEvent
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

// ResolveUpdater is an optional Store capability that toggles an error log's
// resolved state. Stores that do not implement it cause the admin PATCH
// /api/v1/admin/error-logs/{id}/resolve endpoint to return 501.
type ResolveUpdater interface {
	UpdateResolved(ctx context.Context, id int, resolved bool, resolvedBy *int, resolvedAt *time.Time) (UsageLog, error)
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
