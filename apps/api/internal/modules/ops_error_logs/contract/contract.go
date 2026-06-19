// Package contract defines the data types and Store interface for the
// ops_error_logs module — a structured operator-facing log of upstream
// failures observed by the gateway hot path. Ported from sub2api's
// OpsService.RecordError / GetErrorLogs / UpdateErrorResolution but
// adapted to srapi's contract+store+service module layout.
package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Store implementations when a lookup misses.
var ErrNotFound = errors.New("ops_error_logs: entry not found")

// Resolution captures the operator-supplied status of an error log entry. Mirrors
// sub2api's resolved/resolving/open ternary; we widen to four explicit values
// so the admin UI can surface "investigating" without overloading "resolved".
type Resolution string

const (
	ResolutionOpen          Resolution = "open"
	ResolutionInvestigating Resolution = "investigating"
	ResolutionResolved      Resolution = "resolved"
	ResolutionMuted         Resolution = "muted"
)

// Entry is a persisted (or in-memory) record of a single upstream failure
// captured on the gateway hot path. The struct mirrors the columns sub2api
// writes via opsRepo.InsertErrorLog: status code, error class/phase, account
// + user + request identifiers, a redacted body excerpt, and resolution
// metadata. Bodies are pre-sanitized before they reach the store.
type Entry struct {
	ID                int64
	OccurredAt        time.Time
	RequestID         string
	TraceID           string
	UserID            *int
	APIKeyID          *int
	APIKeyPrefix      string
	AccountID         *int
	ProviderID        *int
	Platform          string
	SourceEndpoint    string
	TargetProtocol    string
	Model             string
	StatusCode        *int
	UpstreamRequestID string
	AttemptNo         int
	LatencyMS         int
	InputTokens       int
	OutputTokens      int
	UsageEstimated    bool
	ErrorClass        string
	ErrorPhase        string
	ErrorOwner        string
	ErrorSource       string
	ErrorMessage      string
	ErrorBodyExcerpt  string
	UpstreamErrors    []UpstreamErrorEvent
	Resolution        Resolution
	ResolutionNote    string
	ResolvedAt        *time.Time
	ResolvedByID      *int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RecordRequest is the write input handed to Service.RecordError. It carries
// raw fields; the service normalises, sanitises, and truncates before
// persisting.
type RecordRequest struct {
	OccurredAt        time.Time
	RequestID         string
	TraceID           string
	UserID            *int
	APIKeyID          *int
	APIKeyPrefix      string
	AccountID         *int
	ProviderID        *int
	Platform          string
	SourceEndpoint    string
	TargetProtocol    string
	Model             string
	StatusCode        *int
	UpstreamRequestID string
	AttemptNo         int
	LatencyMS         int
	InputTokens       int
	OutputTokens      int
	UsageEstimated    bool
	ErrorClass        string
	ErrorPhase        string
	ErrorOwner        string
	ErrorSource       string
	ErrorMessage      string
	ErrorBodyExcerpt  string
	UpstreamErrors    []UpstreamErrorEvent
}

// UpstreamErrorEvent captures one failed candidate attempt inside a gateway
// request's failover history. It intentionally contains only operational
// evidence: no raw request body, headers, credentials, prompts, or response
// payload beyond the bounded/redacted body excerpt.
type UpstreamErrorEvent struct {
	AtUnixMs           int64
	AttemptNo          int
	AccountID          *int
	AccountName        string
	UpstreamStatusCode int
	UpstreamRequestID  string
	UpstreamURL        string
	Kind               string
	Message            string
	BodyExcerpt        string
}

// ListFilter narrows admin list queries. Matches sub2api's OpsErrorLogFilter
// surface (paginated list with optional user/account/status/resolution
// filters and a time window).
type ListFilter struct {
	UserID         *int
	AccountID      *int
	ProviderID     *int
	RequestID      string
	Platform       string
	SourceEndpoint string
	Model          string
	ErrorClass     string
	ErrorPhase     string
	ErrorOwner     string
	Query          string
	Resolution     Resolution
	StatusCodeMin  *int
	StatusCodeMax  *int
	From           *time.Time
	To             *time.Time
	Page           int
	PageSize       int
}

// ListResult is the paginated envelope returned by Service.List.
type ListResult struct {
	Items    []Entry
	Total    int
	Page     int
	PageSize int
}

// FingerprintFilter narrows real-time error fingerprint aggregation. It reuses
// ListFilter's safe operator filters and adds an item limit for the grouped
// response.
type FingerprintFilter struct {
	ListFilter
	Limit int
}

// FingerprintSummary groups related ops_error_logs rows by low-cardinality,
// low-sensitivity dimensions and a normalized error-message pattern.
type FingerprintSummary struct {
	Fingerprint         string
	Count               int
	OpenCount           int
	InvestigatingCount  int
	ResolvedCount       int
	MutedCount          int
	FirstOccurredAt     time.Time
	LastOccurredAt      time.Time
	ExampleEntryID      int64
	ExampleRequestID    string
	ExampleErrorMessage string
	SourceEndpoint      string
	TargetProtocol      string
	Model               string
	StatusCode          *int
	StatusClass         string
	ErrorClass          string
	ErrorPhase          string
	ErrorOwner          string
	ErrorSource         string
	MessagePattern      string
}

// FingerprintResult is the grouped operator view over a bounded scan window.
// Total counts discovered groups before the Limit is applied. When Truncated is
// true, Total only covers the scanned sample and is not a full-window group
// count. Scanned and Truncated describe the underlying row scan so callers know
// when the summary is a recent sample rather than a complete historical rollup.
type FingerprintResult struct {
	Items       []FingerprintSummary
	Total       int
	Scanned     int
	Truncated   bool
	WindowStart *time.Time
	WindowEnd   *time.Time
}

// UpdateResolutionRequest captures the operator-supplied resolution update.
type UpdateResolutionRequest struct {
	ID           int64
	Resolution   Resolution
	Note         string
	ResolvedByID *int
	At           time.Time
}

// Store is the persistence boundary. The memory implementation lives under
// apps/api/internal/modules/ops_error_logs/store/memory; an entstore variant
// can later replace it without altering the service.
type Store interface {
	Insert(ctx context.Context, entry Entry) (Entry, error)
	List(ctx context.Context, filter ListFilter) (ListResult, error)
	Get(ctx context.Context, id int64) (Entry, error)
	UpdateResolution(ctx context.Context, req UpdateResolutionRequest) (Entry, error)
	// DeleteOlderThan removes entries strictly older than the cutoff and
	// returns the count removed. Used by an optional retention sweep.
	DeleteOlderThan(ctx context.Context, before time.Time) (int, error)
}
