// Package service implements the request_log_files contract: a per-request
// file-based capture of the gateway's outbound + inbound HTTP envelope.
//
// The on-disk format mirrors CLIProxyAPI's request_logger.go so existing
// operator tooling can read either system's dumps without a new parser. Each
// file looks like:
//
//	=== REQUEST INFO ===
//	Request-ID: req_abc
//	User-ID: 42
//	API-Key-ID: 7
//	Account-ID: 12
//	Source-Protocol: openai
//	Source-Endpoint: /v1/chat/completions
//	Started-At: 2026-06-18T03:04:05.123Z
//
//	=== REQUEST 1 ===
//	POST https://api.openai.com/v1/chat/completions
//	Header-Key: value
//	...
//
//	{body bytes}
//
//	=== RESPONSE 1 ===
//	Status: 200
//	Header-Key: value
//
//	{body bytes}
//
//	=== REQUEST 2 ===
//	... (next failover attempt)
//	=== RESPONSE 2 ===
//	...
//
//	=== SUMMARY ===
//	Success: true
//	Status: 200
//	Latency-MS: 1234
//
// Bodies are written verbatim. Headers with names known to carry credentials
// (Authorization, Cookie, Set-Cookie, X-Api-Key, x-goog-api-key) are masked.
//
// Capture is gated by an enabled flag at the FileWriter level: when disabled,
// Begin returns a zero-path Handle and every other method is a no-op. The
// enabled decision is made by the caller (typically reading
// SRAPI_REQUEST_LOG_ENABLED + account/group metadata) and passed in via the
// Config.Enabled field; the writer itself doesn't read env.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
)

// DefaultLogDir is the on-disk location used when Config.LogDir is empty
// and the SRAPI_REQUEST_LOG_DIR env var is unset.
const DefaultLogDir = "./logs/gateway"

// EnvLogDir is the environment variable that overrides Config.LogDir when
// Config.LogDir is empty. Resolved by ResolveLogDir.
const EnvLogDir = "SRAPI_REQUEST_LOG_DIR"

// EnvEnabled is the environment variable that flips capture on globally when
// Config.Enabled is false. Resolved by ResolveEnabled. Values "1", "t",
// "true", "yes", "on" (case-insensitive) enable; everything else (including
// unset) disables.
const EnvEnabled = "SRAPI_REQUEST_LOG_ENABLED"

// Config is the FileWriter constructor input.
type Config struct {
	// Enabled controls whether Begin actually creates a file. When false,
	// Begin returns a zero-path Handle and all other methods are no-ops.
	Enabled bool
	// LogDir is the directory captured files live in. Created on demand.
	// Defaults to DefaultLogDir.
	LogDir string
}

// ResolveLogDir returns the directory request-log files should live in,
// honouring (in order): explicit configValue, the SRAPI_REQUEST_LOG_DIR env
// var, then DefaultLogDir.
func ResolveLogDir(configValue string) string {
	if value := strings.TrimSpace(configValue); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(EnvLogDir)); value != "" {
		return value
	}
	return DefaultLogDir
}

// ResolveEnabled returns true when request-log capture is enabled, either
// because configValue is true or the SRAPI_REQUEST_LOG_ENABLED env var
// matches a truthy value.
func ResolveEnabled(configValue bool) bool {
	if configValue {
		return true
	}
	value := strings.ToLower(strings.TrimSpace(os.Getenv(EnvEnabled)))
	switch value {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

// FileWriter is the disk-backed contract.Writer implementation.
//
// One FileWriter is shared across the whole process. The per-Handle writes
// open + close the file each time so distinct Handles never contend for the
// same fd; a small per-handle mutex serialises appends within one request
// (the gateway loop is single-goroutine per request, but the mutex is cheap
// insurance against future fan-out).
type FileWriter struct {
	enabled bool
	logDir  string

	// handleMu serialises section appends per Handle by storing one
	// *sync.Mutex per file path. We use sync.Map because the lifetime of
	// each entry is bounded by a single request and contention is low.
	handleMu sync.Map
}

// NewFileWriter constructs the writer. The logDir is resolved through
// ResolveLogDir so callers can pass an empty string to opt into the default
// + env behaviour.
func NewFileWriter(cfg Config) *FileWriter {
	return &FileWriter{
		enabled: cfg.Enabled,
		logDir:  ResolveLogDir(cfg.LogDir),
	}
}

// LogDir returns the resolved directory (after env fallback) the writer is
// using. Exposed for the cleaner + reader to share the same value without
// re-reading config.
func (w *FileWriter) LogDir() string {
	if w == nil {
		return ""
	}
	return w.logDir
}

// Enabled reports whether the writer will actually persist files. When
// false the Writer methods are all no-ops.
func (w *FileWriter) Enabled() bool {
	if w == nil {
		return false
	}
	return w.enabled
}

// Begin opens the per-request log file. The file is named
// request-{unix_ms}-{request_id}.log; Finalize may rename it to error-* if
// the request ultimately failed.
func (w *FileWriter) Begin(_ context.Context, req rlfcontract.BeginRequest) (rlfcontract.Handle, error) {
	if w == nil || !w.enabled {
		return rlfcontract.Handle{}, nil
	}
	startedAt := req.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(w.logDir, 0o755); err != nil {
		return rlfcontract.Handle{}, fmt.Errorf("request_log_files: ensure log dir: %w", err)
	}
	name := buildRequestFileName(startedAt, req.RequestID)
	path := filepath.Join(w.logDir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return rlfcontract.Handle{}, fmt.Errorf("request_log_files: open log file: %w", err)
	}
	defer f.Close()

	if err := writeRequestInfo(f, req, startedAt); err != nil {
		return rlfcontract.Handle{}, fmt.Errorf("request_log_files: write header: %w", err)
	}
	return rlfcontract.NewHandle(req.RequestID, path, startedAt), nil
}

// AppendOutboundRequest writes "=== REQUEST {attempt} ===" + URL/method,
// headers, then the body bytes.
func (w *FileWriter) AppendOutboundRequest(h rlfcontract.Handle, attempt int, url, method string, headers map[string][]string, body []byte) error {
	if w == nil || !w.enabled || h.Path() == "" {
		return nil
	}
	mu := w.muFor(h.Path())
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(h.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("request_log_files: open for request append: %w", err)
	}
	defer f.Close()

	header := fmt.Sprintf("\n=== REQUEST %d ===\n", attempt)
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "%s %s\n", strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(url)); err != nil {
		return err
	}
	if err := writeHeaders(f, headers); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}
	if len(body) > 0 {
		if _, err := f.Write(body); err != nil {
			return err
		}
		if !endsInNewline(body) {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

// AppendUpstreamResponse writes "=== RESPONSE {attempt} ===" + status,
// headers, then the body bytes.
func (w *FileWriter) AppendUpstreamResponse(h rlfcontract.Handle, attempt int, status int, headers map[string][]string, body []byte) error {
	if w == nil || !w.enabled || h.Path() == "" {
		return nil
	}
	mu := w.muFor(h.Path())
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(h.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("request_log_files: open for response append: %w", err)
	}
	defer f.Close()

	header := fmt.Sprintf("\n=== RESPONSE %d ===\n", attempt)
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "Status: %d\n", status); err != nil {
		return err
	}
	if err := writeHeaders(f, headers); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}
	if len(body) > 0 {
		if _, err := f.Write(body); err != nil {
			return err
		}
		if !endsInNewline(body) {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

// Finalize stamps the "=== SUMMARY ===" block and renames the file from
// request-* to error-* when the request failed.
func (w *FileWriter) Finalize(h rlfcontract.Handle, result rlfcontract.FinalizeResult) error {
	if w == nil || !w.enabled || h.Path() == "" {
		return nil
	}
	mu := w.muFor(h.Path())
	mu.Lock()

	f, err := os.OpenFile(h.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		mu.Unlock()
		return fmt.Errorf("request_log_files: open for summary: %w", err)
	}
	_, _ = f.WriteString("\n=== SUMMARY ===\n")
	if result.Success {
		_, _ = f.WriteString("Success: true\n")
	} else {
		_, _ = f.WriteString("Success: false\n")
	}
	if result.ErrorClass != "" {
		fmt.Fprintf(f, "Error-Class: %s\n", result.ErrorClass)
	}
	if result.StatusCode > 0 {
		fmt.Fprintf(f, "Status: %d\n", result.StatusCode)
	}
	fmt.Fprintf(f, "Latency-MS: %d\n", result.LatencyMS)
	closeErr := f.Close()
	mu.Unlock()

	// Drop the per-handle mutex once we are done with the file. The
	// path-keyed mutex lives in a sync.Map; we delete the entry so the
	// next request that happens to reuse the path (extremely rare given
	// the unix_ms + request_id naming) gets a fresh mutex.
	w.handleMu.Delete(h.Path())

	if closeErr != nil {
		return closeErr
	}

	if !result.Success {
		newPath := renameToErrorPath(h.Path())
		if newPath != h.Path() {
			if err := os.Rename(h.Path(), newPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("request_log_files: rename to error file: %w", err)
			}
		}
	}
	return nil
}

func (w *FileWriter) muFor(path string) *sync.Mutex {
	mu := &sync.Mutex{}
	actual, _ := w.handleMu.LoadOrStore(path, mu)
	return actual.(*sync.Mutex)
}

// buildRequestFileName returns the request-* filename for a captured
// request. The unix_ms prefix sorts files chronologically; sanitizing the
// request id keeps the name filesystem-safe.
func buildRequestFileName(startedAt time.Time, requestID string) string {
	id := sanitizeRequestID(requestID)
	if id == "" {
		id = "noid"
	}
	return fmt.Sprintf("request-%d-%s.log", startedAt.UnixMilli(), id)
}

// renameToErrorPath returns the path with the request- prefix swapped for
// error-. When the input does not start with request- it is returned
// unchanged (Finalize then skips the rename).
func renameToErrorPath(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "request-") {
		return path
	}
	return filepath.Join(dir, "error-"+strings.TrimPrefix(base, "request-"))
}

// sanitizeRequestID strips characters that would be unsafe in a filename.
// We deliberately keep the format permissive (anything that's not a
// path-separator-ish character or a wildcard is preserved) so the original
// request id is still readable in the filename.
func sanitizeRequestID(id string) string {
	if id == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range id {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', ' ', '\t', '\n', '\r':
			b.WriteByte('-')
		default:
			if r < 0x20 {
				b.WriteByte('-')
				continue
			}
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return ""
	}
	return out
}

// writeRequestInfo writes the "=== REQUEST INFO ===" header section.
func writeRequestInfo(f *os.File, req rlfcontract.BeginRequest, startedAt time.Time) error {
	if _, err := f.WriteString("=== REQUEST INFO ===\n"); err != nil {
		return err
	}
	if req.RequestID != "" {
		fmt.Fprintf(f, "Request-ID: %s\n", req.RequestID)
	}
	if req.UserID != nil {
		fmt.Fprintf(f, "User-ID: %d\n", *req.UserID)
	}
	if req.APIKeyID != nil {
		fmt.Fprintf(f, "API-Key-ID: %d\n", *req.APIKeyID)
	}
	if req.AccountID != nil {
		fmt.Fprintf(f, "Account-ID: %d\n", *req.AccountID)
	}
	if req.SourceProtocol != "" {
		fmt.Fprintf(f, "Source-Protocol: %s\n", req.SourceProtocol)
	}
	if req.SourceEndpoint != "" {
		fmt.Fprintf(f, "Source-Endpoint: %s\n", req.SourceEndpoint)
	}
	fmt.Fprintf(f, "Started-At: %s\n", startedAt.UTC().Format(time.RFC3339Nano))
	return nil
}

// writeHeaders writes each header on its own line. Sensitive values are
// replaced with [REDACTED] so credentials never hit disk.
func writeHeaders(f *os.File, headers map[string][]string) error {
	if len(headers) == 0 {
		return nil
	}
	// Sorted iteration keeps the on-disk format stable so admins can diff
	// two captures of the same request without spurious noise.
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	// Simple sort to avoid pulling in sort package import noise; the cost
	// is irrelevant — header maps in HTTP traffic are tiny.
	sortStrings(keys)
	for _, key := range keys {
		for _, value := range headers[key] {
			if isSensitiveHeader(key) {
				value = "[REDACTED]"
			}
			if _, err := fmt.Fprintf(f, "%s: %s\n", key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// sortStrings is an in-place insertion sort. Inputs are tiny (HTTP header
// maps); we avoid the sort package to keep the writer's import surface
// minimal.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}

// sensitiveHeaders is the small list of header names that may carry
// bearer-style credentials. Anything matching (case-insensitively) is
// redacted in the on-disk dump.
var sensitiveHeaders = map[string]struct{}{
	"authorization":   {},
	"proxy-authorization": {},
	"cookie":          {},
	"set-cookie":      {},
	"x-api-key":       {},
	"x-goog-api-key":  {},
	"x-anthropic-api-key": {},
}

func isSensitiveHeader(name string) bool {
	_, ok := sensitiveHeaders[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func endsInNewline(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return body[len(body)-1] == '\n'
}
