package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

const (
	defaultSystemLogCleanupMax = 1000
	maxSystemLogCleanupMax     = 10000
	systemLogStaleAfter        = 24 * time.Hour
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
	if err := validateSystemLogListOptions(opts); err != nil {
		return contract.SystemLogList{}, err
	}
	return s.systemLogStore.ListSystemLogs(ctx, opts)
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
	message := strings.TrimSpace(req.Message)
	source := strings.TrimSpace(req.Source)
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
		RequestID: strings.TrimSpace(req.RequestID),
		TraceID:   strings.TrimSpace(req.TraceID),
		Metadata:  cloneAnyMap(req.Metadata),
		CreatedAt: createdAt.UTC(),
	}, nil
}

func validateSystemLogListOptions(opts contract.SystemLogListOptions) error {
	if opts.Level != "" && !opts.Level.Valid() {
		return ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return ErrInvalidInput
	}
	return nil
}

func normalizeSystemLogCleanupFilter(filter contract.SystemLogCleanupFilter) (contract.SystemLogCleanupFilter, error) {
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Query = strings.TrimSpace(filter.Query)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	filter.TraceID = strings.TrimSpace(filter.TraceID)
	if filter.Level != "" && !filter.Level.Valid() {
		return contract.SystemLogCleanupFilter{}, ErrInvalidInput
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
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
			!strings.Contains(strings.ToLower(log.TraceID), query) {
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

func cloneAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
