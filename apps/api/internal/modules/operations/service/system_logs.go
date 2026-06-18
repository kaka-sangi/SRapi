package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

const (
	defaultSystemLogCleanupMax = 1000
	maxSystemLogCleanupMax     = 10000
	systemLogStaleAfter        = 24 * time.Hour
	systemLogMetadataMaxKeys   = 64
	systemLogMetadataMaxDepth  = 4
	systemLogMetadataMaxString = 512
	systemLogIndexedFieldMax   = 128
)

var (
	systemLogAuthorizationPattern    = regexp.MustCompile(`(?i)\b(authorization|proxy_authorization)(\s*[:=]\s*)(bearer|basic)\s+[A-Za-z0-9._~+/\-=]+`)
	systemLogCredentialPattern       = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/\-=]+`)
	systemLogSecretAssignmentPattern = regexp.MustCompile(`(?i)\b(access_token|refresh_token|id_token|api_key|client_secret|password|cookie)(\s*[:=]\s*)([^&\s,;}]+)`)
	systemLogOpenAIKeyPattern        = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{10,}\b`)
	systemLogSRapiKeyPattern         = regexp.MustCompile(`\b(sk_[0-9a-fA-F]+)_[0-9a-fA-F]{10,}\b`)
)

func (s *Service) RecordSystemLog(ctx context.Context, req contract.RecordSystemLogRequest) (contract.OpsSystemLog, error) {
	if s == nil || s.systemLogStore == nil {
		return contract.OpsSystemLog{}, ErrInvalidInput
	}
	log, err := systemLogFromRecordRequest(req, s.clock.Now())
	if err != nil {
		return contract.OpsSystemLog{}, err
	}
	return s.systemLogStore.CreateSystemLog(ctx, log)
}

func (s *Service) ListSystemLogs(ctx context.Context, opts contract.SystemLogListOptions) (contract.SystemLogList, error) {
	if s == nil || s.systemLogStore == nil {
		return contract.SystemLogList{}, ErrInvalidInput
	}
	normalized, err := normalizeSystemLogListOptions(opts)
	if err != nil {
		return contract.SystemLogList{}, err
	}
	return s.systemLogStore.ListSystemLogs(ctx, normalized)
}

func (s *Service) CleanupSystemLogs(ctx context.Context, filter contract.SystemLogCleanupFilter) (contract.SystemLogCleanupResult, error) {
	if s == nil || s.systemLogStore == nil {
		return contract.SystemLogCleanupResult{}, ErrInvalidInput
	}
	normalized, err := normalizeSystemLogCleanupFilter(filter)
	if err != nil {
		return contract.SystemLogCleanupResult{}, err
	}
	return s.systemLogStore.CleanupSystemLogs(ctx, normalized)
}

func (s *Service) SystemLogHealth(ctx context.Context) (contract.SystemLogHealth, error) {
	now := time.Now().UTC()
	if s != nil && s.clock != nil {
		now = s.clock.Now()
	}
	health := contract.SystemLogHealth{
		StorageMode: "unavailable",
		Writable:    false,
		Degraded:    true,
		Stale:       true,
		LevelCounts: map[contract.OpsSystemLogLevel]int{},
		CheckedAt:   now,
	}
	if s == nil || s.systemLogStore == nil {
		return health, nil
	}
	health.StorageMode = "durable"
	health.Writable = true
	health.Degraded = false

	stats, err := s.systemLogStore.SystemLogStats(ctx)
	if err != nil {
		return contract.SystemLogHealth{}, err
	}
	health.TotalCount = stats.TotalCount
	health.LevelCounts = stats.LevelCounts
	if health.LevelCounts == nil {
		health.LevelCounts = map[contract.OpsSystemLogLevel]int{}
	}
	if stats.LastLog != nil {
		last := stats.LastLog.CreatedAt
		health.LastLogAt = &last
		health.Stale = now.Sub(last) > systemLogStaleAfter
	}
	if stats.LastError != nil {
		errorAt := stats.LastError.CreatedAt
		health.LastErrorAt = &errorAt
		health.LastErrorSource = stats.LastError.Source
		health.LastErrorMessage = stats.LastError.Message
	}
	if health.TotalCount == 0 {
		health.Stale = true
	}
	return health, nil
}

func systemLogFromRecordRequest(req contract.RecordSystemLogRequest, now time.Time) (contract.OpsSystemLog, error) {
	level := req.Level
	if level == "" {
		level = contract.OpsSystemLogLevelInfo
	}
	message := scrubSystemLogString(req.Message)
	source := sanitizeSystemLogIndexedField(req.Source)
	if !level.Valid() || message == "" || source == "" {
		return contract.OpsSystemLog{}, ErrInvalidInput
	}
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return contract.OpsSystemLog{
		Level:     level,
		Message:   message,
		Source:    source,
		RequestID: sanitizeSystemLogIndexedField(req.RequestID),
		TraceID:   sanitizeSystemLogIndexedField(req.TraceID),
		Metadata:  sanitizeSystemLogMetadata(req.Metadata),
		CreatedAt: createdAt.UTC(),
	}, nil
}

func normalizeSystemLogListOptions(opts contract.SystemLogListOptions) (contract.SystemLogListOptions, error) {
	if opts.Level != "" && !opts.Level.Valid() {
		return contract.SystemLogListOptions{}, ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return contract.SystemLogListOptions{}, ErrInvalidInput
	}
	rawSource := opts.Source
	opts.Source = sanitizeSystemLogIndexedField(opts.Source)
	if opts.Source == "" && strings.TrimSpace(rawSource) != "" {
		return contract.SystemLogListOptions{}, ErrInvalidInput
	}
	rawQuery := opts.Query
	opts.Query = sanitizeSystemLogSearchField(opts.Query)
	if opts.Query == "" && strings.TrimSpace(rawQuery) != "" {
		return contract.SystemLogListOptions{}, ErrInvalidInput
	}
	rawRequestID := opts.RequestID
	opts.RequestID = sanitizeSystemLogIndexedField(opts.RequestID)
	if opts.RequestID == "" && strings.TrimSpace(rawRequestID) != "" {
		return contract.SystemLogListOptions{}, ErrInvalidInput
	}
	rawTraceID := opts.TraceID
	opts.TraceID = sanitizeSystemLogIndexedField(opts.TraceID)
	if opts.TraceID == "" && strings.TrimSpace(rawTraceID) != "" {
		return contract.SystemLogListOptions{}, ErrInvalidInput
	}
	return opts, nil
}

func normalizeSystemLogCleanupFilter(filter contract.SystemLogCleanupFilter) (contract.SystemLogCleanupFilter, error) {
	if filter.Level != "" && !filter.Level.Valid() {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	rawSource := filter.Source
	filter.Source = sanitizeSystemLogIndexedField(filter.Source)
	if filter.Source == "" && strings.TrimSpace(rawSource) != "" {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	rawQuery := filter.Query
	filter.Query = sanitizeSystemLogSearchField(filter.Query)
	if filter.Query == "" && strings.TrimSpace(rawQuery) != "" {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	rawRequestID := filter.RequestID
	filter.RequestID = sanitizeSystemLogIndexedField(filter.RequestID)
	if filter.RequestID == "" && strings.TrimSpace(rawRequestID) != "" {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	rawTraceID := filter.TraceID
	filter.TraceID = sanitizeSystemLogIndexedField(filter.TraceID)
	if filter.TraceID == "" && strings.TrimSpace(rawTraceID) != "" {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	if filter.Level == "" && filter.Source == "" && filter.Query == "" && filter.RequestID == "" && filter.TraceID == "" && filter.Start == nil && filter.End == nil {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	if filter.MaxDelete == 0 {
		filter.MaxDelete = defaultSystemLogCleanupMax
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > maxSystemLogCleanupMax {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	return filter, nil
}

func sanitizeSystemLogIndexedField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return truncateSystemLogRunes(strings.TrimSpace(b.String()), systemLogIndexedFieldMax)
}

func sanitizeSystemLogSearchField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return scrubSystemLogString(strings.Join(strings.Fields(b.String()), " "))
}

func systemLogMatches(log contract.OpsSystemLog, filter contract.SystemLogCleanupFilter) bool {
	if filter.Level != "" && log.Level != filter.Level {
		return false
	}
	if filter.Source != "" && !strings.EqualFold(log.Source, filter.Source) {
		return false
	}
	if filter.RequestID != "" && log.RequestID != filter.RequestID {
		return false
	}
	if filter.TraceID != "" && log.TraceID != filter.TraceID {
		return false
	}
	if filter.Start != nil && log.CreatedAt.Before(filter.Start.UTC()) {
		return false
	}
	if filter.End != nil && !log.CreatedAt.Before(filter.End.UTC()) {
		return false
	}
	if filter.Query != "" {
		query := strings.ToLower(filter.Query)
		if !strings.Contains(strings.ToLower(log.Message), query) &&
			!strings.Contains(strings.ToLower(log.Source), query) &&
			!strings.Contains(strings.ToLower(log.RequestID), query) &&
			!strings.Contains(strings.ToLower(log.TraceID), query) &&
			!strings.Contains(systemLogMetadataSearchText(log.Metadata), query) {
			return false
		}
	}
	return true
}

func sortSystemLogsNewestFirst(items []contract.OpsSystemLog) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}

func sanitizeSystemLogMetadata(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]any, min(len(keys), systemLogMetadataMaxKeys+1))
	for idx, key := range keys {
		if idx >= systemLogMetadataMaxKeys {
			out["metadata_truncated"] = true
			break
		}
		cleanKey := strings.TrimSpace(key)
		if systemLogMetadataKeyNeedsRedaction(cleanKey) {
			out[cleanKey] = "[REDACTED]"
			continue
		}
		if sanitized, ok := sanitizeSystemLogMetadataValue(value[key], 0); ok {
			out[cleanKey] = sanitized
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeSystemLogMetadataValue(value any, depth int) (any, bool) {
	if value == nil {
		return nil, true
	}
	if depth >= systemLogMetadataMaxDepth {
		return "[TRUNCATED]", true
	}
	switch typed := value.(type) {
	case string:
		return scrubSystemLogString(typed), true
	case bool:
		return typed, true
	case int:
		return typed, true
	case int8:
		return typed, true
	case int16:
		return typed, true
	case int32:
		return typed, true
	case int64:
		return typed, true
	case uint:
		return typed, true
	case uint8:
		return typed, true
	case uint16:
		return typed, true
	case uint32:
		return typed, true
	case uint64:
		return typed, true
	case float32:
		return typed, true
	case float64:
		return typed, true
	case json.Number:
		return typed.String(), true
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano), true
	case error:
		return scrubSystemLogString(typed.Error()), true
	case fmt.Stringer:
		return scrubSystemLogString(typed.String()), true
	case map[string]any:
		return sanitizeSystemLogMetadataMapValue(typed, depth+1), true
	case map[string]string:
		nested := make(map[string]any, len(typed))
		for k, v := range typed {
			nested[k] = v
		}
		return sanitizeSystemLogMetadataMapValue(nested, depth+1), true
	case []any:
		return sanitizeSystemLogMetadataListValue(typed, depth+1), true
	case []string:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return sanitizeSystemLogMetadataListValue(values, depth+1), true
	default:
		return fmt.Sprintf("%T", typed), true
	}
}

func sanitizeSystemLogMetadataMapValue(value map[string]any, depth int) map[string]any {
	if value == nil {
		return nil
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make(map[string]any, min(len(keys), systemLogMetadataMaxKeys+1))
	for idx, key := range keys {
		if idx >= systemLogMetadataMaxKeys {
			out["metadata_truncated"] = true
			break
		}
		cleanKey := strings.TrimSpace(key)
		if systemLogMetadataKeyNeedsRedaction(cleanKey) {
			out[cleanKey] = "[REDACTED]"
			continue
		}
		if sanitized, ok := sanitizeSystemLogMetadataValue(value[key], depth); ok {
			out[cleanKey] = sanitized
		}
	}
	return out
}

func sanitizeSystemLogMetadataListValue(values []any, depth int) []any {
	if len(values) == 0 {
		return nil
	}
	limit := len(values)
	if limit > systemLogMetadataMaxKeys {
		limit = systemLogMetadataMaxKeys
	}
	out := make([]any, 0, limit+1)
	for idx := 0; idx < limit; idx++ {
		if sanitized, ok := sanitizeSystemLogMetadataValue(values[idx], depth); ok {
			out = append(out, sanitized)
		}
	}
	if len(values) > limit {
		out = append(out, "[TRUNCATED]")
	}
	return out
}

func systemLogMetadataSearchText(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return strings.ToLower(string(raw))
}

func systemLogMetadataKeyNeedsRedaction(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(normalized)
	if normalized == "" {
		return false
	}
	if systemLogMetadataTokenCountKey(normalized) {
		return false
	}
	if systemLogMetadataSafeAPIKeyReference(normalized) {
		return false
	}
	switch normalized {
	case "body", "body_excerpt", "request_body", "response_body", "raw_body",
		"prompt", "prompts", "messages", "input", "output", "headers", "header",
		"authorization", "proxy_authorization", "cookie", "set_cookie",
		"api_key", "access_token", "refresh_token", "id_token", "session_token",
		"csrf_token", "client_secret", "private_key", "secret", "password",
		"credential", "credentials", "jwt":
		return true
	}
	if strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "private_key") ||
		strings.Contains(normalized, "api_key") ||
		strings.Contains(normalized, "body") ||
		strings.Contains(normalized, "prompt") ||
		strings.Contains(normalized, "payload") {
		return true
	}
	return strings.Contains(normalized, "token")
}

func systemLogMetadataSafeAPIKeyReference(key string) bool {
	switch key {
	case "api_key_id", "api_key_prefix", "attempted_key_prefix",
		"deleted_key_id", "deleted_key_owner_user_id", "deleted_key_name":
		return true
	default:
		return false
	}
}

func systemLogMetadataTokenCountKey(key string) bool {
	switch key {
	case "max_tokens", "max_output_tokens", "max_input_tokens", "max_completion_tokens",
		"max_tokens_to_sample", "budget_tokens", "prompt_tokens", "completion_tokens",
		"input_tokens", "output_tokens", "total_tokens", "token_count", "estimated_tokens":
		return true
	default:
		return false
	}
}

func scrubSystemLogString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = systemLogAuthorizationPattern.ReplaceAllString(value, "${1}${2}${3} [REDACTED]")
	value = systemLogCredentialPattern.ReplaceAllString(value, "${1} [REDACTED]")
	value = systemLogSecretAssignmentPattern.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = systemLogOpenAIKeyPattern.ReplaceAllString(value, "sk-[REDACTED]")
	value = systemLogSRapiKeyPattern.ReplaceAllString(value, "${1}_[REDACTED]")
	return truncateSystemLogString(value)
}

func truncateSystemLogString(value string) string {
	runes := []rune(value)
	if len(runes) <= systemLogMetadataMaxString {
		return value
	}
	return string(runes[:systemLogMetadataMaxString]) + "...[TRUNCATED]"
}

func truncateSystemLogRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
