// Package service is the OpsErrorLogs service: it sanitises inbound
// upstream-error records, enforces bounded payload sizes, and delegates
// persistence to a contract.Store. Ported from sub2api's
// OpsService.RecordError + GetErrorLogs + UpdateErrorResolution flow at
// service/ops_service.go:139-453 — collapsed into the smaller srapi surface
// the gateway hot path needs.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
)

// MaxBodyExcerptBytes bounds the redacted body excerpt persisted per entry.
// Mirrors sub2api's opsMaxStoredErrorBodyBytes; the gateway path passes
// pre-trimmed input but the service still caps as a defence in depth.
const MaxBodyExcerptBytes = 8 * 1024

// MaxMessageBytes bounds the upstream error message.
const MaxMessageBytes = 2048

// MaxUpstreamErrorEvents bounds the per-row failover timeline shown in the
// admin panel. Real gateway failover is much smaller; this cap protects
// manual/test callers and future integrations.
const MaxUpstreamErrorEvents = 20

// DefaultRetention is the upper bound on how long an entry is kept by an
// optional retention sweep. Aligns with sub2api ops defaults (30d).
const DefaultRetention = 30 * 24 * time.Hour

// ErrInvalidInput is returned by service methods when required fields are
// missing or malformed.
var ErrInvalidInput = errors.New("ops_error_logs: invalid input")

// Service is the ops_error_logs application service.
type Service struct {
	store contract.Store
	now   func() time.Time
}

// New constructs a Service backed by the given Store. The clock is injectable
// for tests; nil falls back to time.Now.
func New(store contract.Store, now func() time.Time) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, now: now}, nil
}

// RecordError persists a single upstream-failure observation. Callers on the
// gateway hot path should treat this as best-effort, but the service boundary
// still returns ErrInvalidInput when the request carries no usable evidence.
func (s *Service) RecordError(ctx context.Context, req contract.RecordRequest) error {
	if s == nil || s.store == nil {
		return ErrInvalidInput
	}
	entry, ok := s.prepareEntry(req)
	if !ok {
		return ErrInvalidInput
	}
	if _, err := s.store.Insert(ctx, entry); err != nil {
		return err
	}
	return nil
}

// List paginates the persisted entries for the admin console. Mirrors
// sub2api's GetErrorLogs.
func (s *Service) List(ctx context.Context, filter contract.ListFilter) (contract.ListResult, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	return s.store.List(ctx, filter)
}

// Get returns a single entry by id.
func (s *Service) Get(ctx context.Context, id int64) (contract.Entry, error) {
	if id <= 0 {
		return contract.Entry{}, ErrInvalidInput
	}
	return s.store.Get(ctx, id)
}

// UpdateResolution sets the operator-supplied resolution status + note.
// Mirrors sub2api's UpdateErrorResolution but uses a richer enum.
func (s *Service) UpdateResolution(ctx context.Context, req contract.UpdateResolutionRequest) (contract.Entry, error) {
	if req.ID <= 0 {
		return contract.Entry{}, ErrInvalidInput
	}
	if req.Resolution == "" {
		return contract.Entry{}, ErrInvalidInput
	}
	if !validResolution(req.Resolution) {
		return contract.Entry{}, ErrInvalidInput
	}
	if req.At.IsZero() {
		req.At = s.now().UTC()
	} else {
		req.At = req.At.UTC()
	}
	req.Note = truncate(strings.TrimSpace(req.Note), MaxMessageBytes)
	return s.store.UpdateResolution(ctx, req)
}

// SweepOlderThan deletes entries older than the supplied cutoff. Returns the
// number removed. Intended to be called from a retention worker; for the
// initial wiring we expose it directly so a scheduled task can drive it.
func (s *Service) SweepOlderThan(ctx context.Context, before time.Time) (int, error) {
	if before.IsZero() {
		return 0, ErrInvalidInput
	}
	return s.store.DeleteOlderThan(ctx, before.UTC())
}

func (s *Service) prepareEntry(req contract.RecordRequest) (contract.Entry, bool) {
	now := s.now().UTC()
	occurred := req.OccurredAt
	if occurred.IsZero() {
		occurred = now
	}
	phase := strings.TrimSpace(req.ErrorPhase)
	if phase == "" {
		phase = "upstream"
	}
	class := strings.TrimSpace(req.ErrorClass)
	if class == "" {
		class = "unknown"
	}
	if req.StatusCode != nil && !validHTTPStatus(*req.StatusCode) {
		return contract.Entry{}, false
	}
	entry := contract.Entry{
		OccurredAt:        occurred.UTC(),
		RequestID:         truncate(strings.TrimSpace(req.RequestID), 128),
		TraceID:           truncate(strings.TrimSpace(req.TraceID), 128),
		UserID:            req.UserID,
		APIKeyID:          req.APIKeyID,
		APIKeyPrefix:      truncate(strings.TrimSpace(req.APIKeyPrefix), 32),
		AccountID:         req.AccountID,
		ProviderID:        req.ProviderID,
		Platform:          truncate(strings.TrimSpace(req.Platform), 64),
		SourceEndpoint:    truncate(strings.TrimSpace(req.SourceEndpoint), 128),
		TargetProtocol:    truncate(strings.TrimSpace(req.TargetProtocol), 64),
		Model:             truncate(strings.TrimSpace(req.Model), 128),
		StatusCode:        req.StatusCode,
		UpstreamRequestID: truncate(strings.TrimSpace(req.UpstreamRequestID), 128),
		AttemptNo:         positiveOrDefault(req.AttemptNo, 1),
		LatencyMS:         positiveOrZero(req.LatencyMS),
		InputTokens:       positiveOrZero(req.InputTokens),
		OutputTokens:      positiveOrZero(req.OutputTokens),
		UsageEstimated:    req.UsageEstimated,
		ErrorClass:        truncate(class, 64),
		ErrorPhase:        truncate(phase, 64),
		ErrorOwner:        truncate(defaultString(req.ErrorOwner, "provider"), 64),
		ErrorSource:       truncate(defaultString(req.ErrorSource, "upstream_http"), 64),
		ErrorMessage:      truncate(sanitizeMessage(req.ErrorMessage), MaxMessageBytes),
		ErrorBodyExcerpt:  redactExcerpt(req.ErrorBodyExcerpt, MaxBodyExcerptBytes),
		UpstreamErrors:    sanitizeUpstreamErrors(req.UpstreamErrors),
		Resolution:        contract.ResolutionOpen,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if entry.RequestID == "" && entry.StatusCode == nil && entry.ErrorMessage == "" && entry.ErrorBodyExcerpt == "" {
		return contract.Entry{}, false
	}
	return entry, true
}

func validHTTPStatus(status int) bool {
	return status >= 100 && status <= 599
}

func validUpstreamStatus(status int) int {
	if validHTTPStatus(status) {
		return status
	}
	return 0
}

func sanitizeUpstreamErrors(events []contract.UpstreamErrorEvent) []contract.UpstreamErrorEvent {
	if len(events) == 0 {
		return nil
	}
	if len(events) > MaxUpstreamErrorEvents {
		events = events[len(events)-MaxUpstreamErrorEvents:]
	}
	out := make([]contract.UpstreamErrorEvent, 0, len(events))
	for _, event := range events {
		out = append(out, contract.UpstreamErrorEvent{
			AtUnixMs:           positiveInt64(event.AtUnixMs),
			AttemptNo:          positiveOrDefault(event.AttemptNo, 1),
			AccountID:          event.AccountID,
			AccountName:        truncate(strings.TrimSpace(event.AccountName), 128),
			UpstreamStatusCode: validUpstreamStatus(event.UpstreamStatusCode),
			UpstreamRequestID:  truncate(strings.TrimSpace(event.UpstreamRequestID), 128),
			UpstreamURL:        truncate(strings.TrimSpace(event.UpstreamURL), 256),
			Kind:               truncate(defaultString(event.Kind, "request_error"), 64),
			Message:            truncate(sanitizeMessage(event.Message), MaxMessageBytes),
			BodyExcerpt:        redactExcerpt(event.BodyExcerpt, MaxBodyExcerptBytes),
		})
	}
	return out
}

func validResolution(r contract.Resolution) bool {
	switch r {
	case contract.ResolutionOpen, contract.ResolutionInvestigating, contract.ResolutionResolved, contract.ResolutionMuted:
		return true
	}
	return false
}

// redactExcerpt sanitises a body excerpt: if it parses as JSON we recursively
// replace sensitive keys with "[REDACTED]"; otherwise we fall back to a
// length-bounded passthrough. Mirrors sub2api's sanitizeErrorBodyForStorage +
// redactSensitiveJSON pair.
func redactExcerpt(raw string, maxBytes int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		decoded = redactSensitiveJSON(decoded)
		if encoded, err := json.Marshal(decoded); err == nil {
			return truncate(string(encoded), maxBytes)
		}
	}
	return truncate(raw, maxBytes)
}

func redactSensitiveJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			if isSensitiveKey(k) {
				out[k] = "[REDACTED]"
				continue
			}
			out[k] = redactSensitiveJSON(vv)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, vv := range t {
			out = append(out, redactSensitiveJSON(vv))
		}
		return out
	default:
		return v
	}
}

// isSensitiveKey returns true for keys that frequently leak credentials in
// upstream error bodies. The set is intentionally narrow — broader scrubbing
// happens upstream via sanitizedExportMetadata before the excerpt is built.
func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "authorization", "auth", "api_key", "apikey", "key", "secret",
		"password", "token", "access_token", "refresh_token", "id_token",
		"x-api-key", "anthropic-api-key", "openai-api-key", "set-cookie",
		"cookie", "session", "session_id", "credential", "credentials":
		return true
	}
	return false
}

// sanitizeMessage strips control characters that would otherwise wreck a
// log viewer. Mirrors sub2api's sanitizeUpstreamErrorMessage.
func sanitizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(msg))
	for _, r := range msg {
		if r == '\n' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func positiveOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveOrZero(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func positiveInt64(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}
