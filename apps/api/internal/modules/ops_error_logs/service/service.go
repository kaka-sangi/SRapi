// Package service is the OpsErrorLogs service: it sanitises inbound
// upstream-error records, enforces bounded payload sizes, and delegates
// persistence to a contract.Store. Ported from sub2api's
// OpsService.RecordError + GetErrorLogs + UpdateErrorResolution flow at
// service/ops_service.go:139-453 — collapsed into the smaller srapi surface
// the gateway hot path needs.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
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

// DefaultFingerprintWindow is the default lookback for real-time fingerprint
// aggregation when the caller does not provide an explicit time range.
const DefaultFingerprintWindow = 24 * time.Hour

// DefaultFingerprintLimit is the default number of fingerprint groups returned.
const DefaultFingerprintLimit = 20

// MaxFingerprintLimit caps the grouped response size.
const MaxFingerprintLimit = 100

// MaxFingerprintScanRows bounds the live row scan used before a durable rollup
// table exists.
const MaxFingerprintScanRows = 5000

// ErrInvalidInput is returned by service methods when required fields are
// missing or malformed.
var ErrInvalidInput = errors.New("ops_error_logs: invalid input")

var (
	authorizationPattern    = regexp.MustCompile(`(?i)\b(authorization|proxy_authorization)(\s*[:=]\s*)(bearer|basic)\s+[A-Za-z0-9._~+/\-=]+`)
	credentialPattern       = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/\-=]+`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(access_token|refresh_token|id_token|api_key|client_secret|password|cookie|session|session_id|token|secret)(\s*[:=]\s*)([^&\s,;}]+)`)
	secretQueryPattern      = regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|id_token|api_key|client_secret|password|session|session_id|token|secret)=)([^&#\s]+)`)
	apiKeyPlaintextPattern  = regexp.MustCompile(`(?i)^(sk_[0-9a-fA-F]{12})_[0-9a-fA-F]{16,}$`)
	openAIKeyPattern        = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{10,}\b`)
	srapiKeyPattern         = regexp.MustCompile(`\b(sk_[0-9a-fA-F]+)_[0-9a-fA-F]{10,}\b`)
	fingerprintURLPattern   = regexp.MustCompile(`https?://[^\s]+`)
	fingerprintUUIDPattern  = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	fingerprintHexPattern   = regexp.MustCompile(`\b[0-9a-fA-F]{16,}\b`)
	fingerprintReqPattern   = regexp.MustCompile(`(?i)\breq[_-]?[A-Za-z0-9._-]{6,}\b`)
	fingerprintNumPattern   = regexp.MustCompile(`\d+`)
	fingerprintSpacePattern = regexp.MustCompile(`\s+`)
)

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
	normalized, err := normalizeListFilter(filter)
	if err != nil {
		return contract.ListResult{}, err
	}
	return s.store.List(ctx, normalized)
}

// ListFingerprints returns a bounded real-time aggregation of recent
// ops_error_logs rows. It intentionally groups on low-sensitivity dimensions
// only; request ids, user/account ids, API key ids, prompts, and body excerpts
// are not part of the fingerprint key.
func (s *Service) ListFingerprints(ctx context.Context, filter contract.FingerprintFilter) (contract.FingerprintResult, error) {
	if s == nil || s.store == nil {
		return contract.FingerprintResult{}, ErrInvalidInput
	}
	normalized, err := normalizeListFilter(filter.ListFilter)
	if err != nil {
		return contract.FingerprintResult{}, err
	}
	now := s.now().UTC()
	if normalized.To == nil {
		to := now
		normalized.To = &to
	}
	if normalized.From == nil {
		from := normalized.To.Add(-DefaultFingerprintWindow)
		normalized.From = &from
	}
	limit := normalizeFingerprintLimit(filter.Limit)
	scanFilter := normalized
	scanFilter.PageSize = 200
	groups := make(map[string]*contract.FingerprintSummary)
	scanned := 0
	matched := 0
	for page := 1; scanned < MaxFingerprintScanRows; page++ {
		scanFilter.Page = page
		res, err := s.store.List(ctx, scanFilter)
		if err != nil {
			return contract.FingerprintResult{}, err
		}
		if page == 1 {
			matched = res.Total
		}
		if len(res.Items) == 0 {
			break
		}
		for _, entry := range res.Items {
			aggregateFingerprint(groups, entry)
			scanned++
			if scanned >= MaxFingerprintScanRows {
				break
			}
		}
		if page*scanFilter.PageSize >= res.Total {
			break
		}
	}
	items := fingerprintSummaries(groups)
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return contract.FingerprintResult{
		Items:       items,
		Total:       total,
		Scanned:     scanned,
		Truncated:   matched > scanned,
		WindowStart: normalized.From,
		WindowEnd:   normalized.To,
	}, nil
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
	if req.ResolvedByID != nil && *req.ResolvedByID <= 0 {
		return contract.Entry{}, ErrInvalidInput
	}
	if req.At.IsZero() {
		req.At = s.now().UTC()
	} else {
		req.At = req.At.UTC()
	}
	req.Note = truncate(redactSecretText(req.Note), MaxMessageBytes)
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
	apiKeyPrefix := sanitizeAPIKeyPrefix(req.APIKeyPrefix)
	entry := contract.Entry{
		OccurredAt:        occurred.UTC(),
		RequestID:         truncate(cleanLogText(req.RequestID), 128),
		TraceID:           truncate(cleanLogText(req.TraceID), 128),
		UserID:            req.UserID,
		APIKeyID:          req.APIKeyID,
		APIKeyPrefix:      apiKeyPrefix,
		AccountID:         req.AccountID,
		ProviderID:        req.ProviderID,
		Platform:          truncate(cleanLogText(req.Platform), 64),
		SourceEndpoint:    truncate(cleanLogText(req.SourceEndpoint), 128),
		TargetProtocol:    truncate(cleanLogText(req.TargetProtocol), 64),
		Model:             truncate(cleanLogText(req.Model), 128),
		StatusCode:        req.StatusCode,
		UpstreamRequestID: truncate(cleanLogText(req.UpstreamRequestID), 128),
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

func sanitizeAPIKeyPrefix(value string) string {
	value = cleanLogText(value)
	if value == "" {
		return ""
	}
	if match := apiKeyPlaintextPattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1]
	}
	if strings.HasPrefix(strings.ToLower(value), "sk_") && len([]rune(value)) > 16 {
		return "sk_[REDACTED]"
	}
	return truncate(value, 32)
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
			UpstreamURL:        truncate(redactSecretText(event.UpstreamURL), 256),
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

func normalizeListFilter(filter contract.ListFilter) (contract.ListFilter, error) {
	if filter.Resolution != "" && !validResolution(filter.Resolution) {
		return contract.ListFilter{}, ErrInvalidInput
	}
	if filter.StatusCodeMin != nil && !validHTTPStatus(*filter.StatusCodeMin) {
		return contract.ListFilter{}, ErrInvalidInput
	}
	if filter.StatusCodeMax != nil && !validHTTPStatus(*filter.StatusCodeMax) {
		return contract.ListFilter{}, ErrInvalidInput
	}
	if filter.StatusCodeMin != nil && filter.StatusCodeMax != nil && *filter.StatusCodeMin > *filter.StatusCodeMax {
		return contract.ListFilter{}, ErrInvalidInput
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return contract.ListFilter{}, ErrInvalidInput
	}
	filter.Platform = truncate(cleanLogText(filter.Platform), 64)
	filter.Model = truncate(cleanLogText(filter.Model), 128)
	filter.ErrorClass = truncate(cleanLogText(filter.ErrorClass), 64)
	filter.Query = truncate(redactSecretText(filter.Query), MaxMessageBytes)
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	return filter, nil
}

func normalizeFingerprintLimit(limit int) int {
	if limit <= 0 {
		return DefaultFingerprintLimit
	}
	if limit > MaxFingerprintLimit {
		return MaxFingerprintLimit
	}
	return limit
}

func aggregateFingerprint(groups map[string]*contract.FingerprintSummary, entry contract.Entry) {
	fingerprint, messagePattern, statusClass := entryFingerprint(entry)
	group, ok := groups[fingerprint]
	if !ok {
		group = &contract.FingerprintSummary{
			Fingerprint:     fingerprint,
			FirstOccurredAt: entry.OccurredAt,
			LastOccurredAt:  entry.OccurredAt,
			SourceEndpoint:  entry.SourceEndpoint,
			TargetProtocol:  entry.TargetProtocol,
			Model:           entry.Model,
			StatusCode:      cloneIntPointer(entry.StatusCode),
			StatusClass:     statusClass,
			ErrorClass:      entry.ErrorClass,
			ErrorPhase:      entry.ErrorPhase,
			ErrorOwner:      entry.ErrorOwner,
			ErrorSource:     entry.ErrorSource,
			MessagePattern:  messagePattern,
		}
		groups[fingerprint] = group
	}
	group.Count++
	switch entry.Resolution {
	case contract.ResolutionInvestigating:
		group.InvestigatingCount++
	case contract.ResolutionResolved:
		group.ResolvedCount++
	case contract.ResolutionMuted:
		group.MutedCount++
	default:
		group.OpenCount++
	}
	if group.FirstOccurredAt.IsZero() || entry.OccurredAt.Before(group.FirstOccurredAt) {
		group.FirstOccurredAt = entry.OccurredAt
	}
	if group.LastOccurredAt.IsZero() || entry.OccurredAt.After(group.LastOccurredAt) {
		group.LastOccurredAt = entry.OccurredAt
		group.ExampleEntryID = entry.ID
		group.ExampleRequestID = entry.RequestID
		group.ExampleErrorMessage = entry.ErrorMessage
	}
	if group.ExampleEntryID == 0 {
		group.ExampleEntryID = entry.ID
		group.ExampleRequestID = entry.RequestID
		group.ExampleErrorMessage = entry.ErrorMessage
	}
}

func fingerprintSummaries(groups map[string]*contract.FingerprintSummary) []contract.FingerprintSummary {
	items := make([]contract.FingerprintSummary, 0, len(groups))
	for _, group := range groups {
		items = append(items, *group)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		if !items[i].LastOccurredAt.Equal(items[j].LastOccurredAt) {
			return items[i].LastOccurredAt.After(items[j].LastOccurredAt)
		}
		return items[i].Fingerprint < items[j].Fingerprint
	})
	return items
}

func entryFingerprint(entry contract.Entry) (string, string, string) {
	messagePattern := normalizeFingerprintMessage(entry.ErrorMessage)
	if messagePattern == "" {
		messagePattern = normalizeFingerprintMessage(entry.ErrorClass)
	}
	statusClass := statusClass(entry.StatusCode)
	statusCode := ""
	if entry.StatusCode != nil {
		statusCode = strconv.Itoa(*entry.StatusCode)
	}
	key := strings.Join([]string{
		entry.SourceEndpoint,
		entry.TargetProtocol,
		entry.Model,
		statusCode,
		statusClass,
		entry.ErrorClass,
		entry.ErrorPhase,
		entry.ErrorOwner,
		entry.ErrorSource,
		messagePattern,
	}, "\x1f")
	sum := sha256.Sum256([]byte(key))
	return "errfp_" + hex.EncodeToString(sum[:8]), messagePattern, statusClass
}

func normalizeFingerprintMessage(message string) string {
	message = strings.ToLower(redactSecretText(message))
	message = fingerprintURLPattern.ReplaceAllString(message, "{url}")
	message = fingerprintUUIDPattern.ReplaceAllString(message, "{uuid}")
	message = fingerprintReqPattern.ReplaceAllString(message, "{request}")
	message = fingerprintHexPattern.ReplaceAllString(message, "{hex}")
	message = fingerprintNumPattern.ReplaceAllString(message, "{n}")
	message = fingerprintSpacePattern.ReplaceAllString(message, " ")
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	return truncate(message, 160)
}

func statusClass(status *int) string {
	if status == nil {
		return "unknown"
	}
	if *status < 100 || *status > 599 {
		return "unknown"
	}
	return fmt.Sprintf("%dxx", *status/100)
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
	return truncate(redactSecretText(raw), maxBytes)
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
	case string:
		return redactSecretText(t)
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
	return redactSecretText(msg)
}

func redactSecretText(value string) string {
	value = cleanLogText(value)
	if value == "" {
		return ""
	}
	value = authorizationPattern.ReplaceAllString(value, "${1}${2}${3} [REDACTED]")
	value = credentialPattern.ReplaceAllString(value, "${1} [REDACTED]")
	value = secretQueryPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = secretAssignmentPattern.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = openAIKeyPattern.ReplaceAllString(value, "sk-[REDACTED]")
	value = srapiKeyPattern.ReplaceAllString(value, "${1}_[REDACTED]")
	return strings.TrimSpace(value)
}

func cleanLogText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == '\n' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
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
