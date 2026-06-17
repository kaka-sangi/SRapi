// Package contract defines the public surface of the request_log_files module:
// a file-based, per-request HTTP-envelope log capture parallel to the
// structured usage_log. Each captured request writes ONE flat file containing
// every outbound upstream attempt (request URL/method/headers/body) and the
// corresponding inbound upstream response (status/headers/body). The format is
// a port of CLIProxyAPI's request_logger.go ("=== REQUEST {n} ===" /
// "=== RESPONSE {n} ===" sections) so existing operator tooling can read the
// dumps without any new parser.
//
// The capture is OFF by default (disk cost) and gated by either the global
// env flag SRAPI_REQUEST_LOG_ENABLED=true or an account/group metadata
// opt-in. Retention is enforced by a background cleaner: files older than
// the configured age, AND error files exceeding the per-file-count cap, are
// removed.
//
// Storage layout: one file per request, named
//
//	request-{unix_ms}-{request_id}.log     -- result.Success == true
//	error-{unix_ms}-{request_id}.log       -- result.Success == false
//
// under the configured directory (default ./logs/gateway, override via env
// SRAPI_REQUEST_LOG_DIR or runtime config).
package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by the reader when a lookup misses.
var ErrNotFound = errors.New("request_log_files: file not found")

// ErrInvalidName is returned for filenames containing path separators or
// other characters that would let a caller escape the configured directory.
var ErrInvalidName = errors.New("request_log_files: invalid file name")

// BeginRequest is the input handed to Writer.Begin at the top of the gateway
// hot path. It carries the identifiers needed to name and route the log file.
type BeginRequest struct {
	RequestID      string
	UserID         *int
	APIKeyID       *int
	AccountID      *int
	SourceProtocol string
	SourceEndpoint string
	StartedAt      time.Time
}

// Handle is the opaque token returned by Writer.Begin and passed to the
// per-attempt append calls plus the final Finalize. It must be safe to pass
// across goroutines (the writer's mutex serialises section writes).
type Handle struct {
	// RequestID is the canonical request id used in the eventual filename.
	RequestID string
	// path is the absolute file path the writer is appending to. Empty when
	// the writer is disabled (Begin then becomes a no-op and every other
	// method returns nil).
	path string
	// startedAt is the BeginRequest.StartedAt captured by the writer so the
	// finalize step can stamp the latency-ms header.
	startedAt time.Time
	// success is set by Finalize so the cleanup step (rename to error-*) is
	// triggered for failed requests.
	// (We rename on Finalize rather than picking the right prefix at Begin
	// because Begin runs before any upstream call resolves.)
}

// Path returns the on-disk file path the handle is writing to. Empty when the
// writer was disabled. Exposed primarily for tests; the hot path should not
// rely on it.
func (h Handle) Path() string { return h.path }

// StartedAt returns the BeginRequest.StartedAt the handle was created with.
func (h Handle) StartedAt() time.Time { return h.startedAt }

// NewHandle constructs a Handle. Exposed for the service layer to populate
// the unexported fields without leaking the struct shape.
func NewHandle(requestID, path string, startedAt time.Time) Handle {
	return Handle{RequestID: requestID, path: path, startedAt: startedAt}
}

// FinalizeResult is the outcome handed to Writer.Finalize at the bottom of
// the gateway hot path. The writer uses Success to decide between the
// request-* / error-* filename prefix, and stamps the trailing summary block
// in the log file with the remaining fields.
type FinalizeResult struct {
	Success    bool
	ErrorClass string
	StatusCode int
	LatencyMS  int
}

// Writer is the per-request capture surface. Implementations MUST be safe to
// call from multiple goroutines for distinct Handles; appends for one Handle
// happen in attempt order from the gateway loop and are not concurrent.
type Writer interface {
	// Begin opens (or stubs) the log file for the request. The returned
	// Handle is passed to all subsequent calls. When the writer is disabled
	// the handle is a zero-path no-op; callers do not have to branch.
	Begin(ctx context.Context, req BeginRequest) (Handle, error)

	// AppendOutboundRequest writes a "=== REQUEST {attempt} ===" section
	// containing the outbound URL, method, headers, and body.
	AppendOutboundRequest(h Handle, attempt int, url, method string, headers map[string][]string, body []byte) error

	// AppendUpstreamResponse writes a "=== RESPONSE {attempt} ===" section
	// containing the inbound status, headers, and body.
	AppendUpstreamResponse(h Handle, attempt int, status int, headers map[string][]string, body []byte) error

	// Finalize stamps the trailing "=== SUMMARY ===" block (success,
	// error class, status, latency) and, when result.Success == false,
	// renames the file from request-* to error-*. Calling Finalize twice
	// for the same handle is a no-op.
	Finalize(h Handle, result FinalizeResult) error
}

// FileDescriptor is the lightweight projection the admin list endpoint
// returns for one captured log file. It carries the metadata needed for the
// list UI without forcing a read of the (potentially large) body.
type FileDescriptor struct {
	Name        string
	Size        int64
	CreatedAt   time.Time
	RequestID   string
	IsErrorOnly bool
}

// Reader exposes the listing + download surface backing the admin API. It
// lives separately from Writer so a future implementation can serve the read
// path from a different backend (e.g. object storage) without rewiring the
// gateway.
type Reader interface {
	// List returns descriptors filtered by RequestID prefix (when non-empty)
	// and ErrorOnly. The returned slice is sorted newest-first.
	List(ctx context.Context, filter ListFilter) ([]FileDescriptor, error)
	// Get returns the descriptor for a single file by its on-disk name.
	Get(ctx context.Context, name string) (FileDescriptor, error)
	// Open returns the raw file content as bytes (suitable for the
	// /download endpoint).
	Open(ctx context.Context, name string) ([]byte, error)
	// Delete removes one captured file.
	Delete(ctx context.Context, name string) error
}

// ListFilter is the input to Reader.List. All fields are optional.
type ListFilter struct {
	// RequestIDPrefix narrows the result set to files whose embedded
	// request id starts with the prefix.
	RequestIDPrefix string
	// ErrorOnly, when true, keeps only error-* files.
	ErrorOnly bool
	// From, when set, drops files created before the cutoff.
	From *time.Time
	// To, when set, drops files created at or after the cutoff.
	To *time.Time
	// Limit caps the number of returned entries (after sort). Zero means
	// no cap.
	Limit int
}

// Cleaner is the retention surface. It runs as a background goroutine and
// removes files older than RetentionAge plus the surplus when more than
// MaxFiles error-* files exist.
type Cleaner interface {
	// SweepOnce performs one immediate retention sweep. Exposed primarily
	// for tests; the runtime spawns a goroutine that calls SweepOnce on
	// startup and on every cleanup interval.
	SweepOnce(ctx context.Context) (deleted int, err error)
}
