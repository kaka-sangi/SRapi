package service

import (
	"context"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

const (
	settingsKeySystemLogs = "admin_control.system_logs"

	defaultSystemLogCleanupMax = 1000
	maxSystemLogCleanupMax     = 10000
)

func (s *Service) RecordSystemLog(ctx context.Context, req admincontrol.RecordSystemLogRequest) (admincontrol.OpsSystemLog, error) {
	log, err := systemLogFromRecordRequest(req, s.clock.Now())
	if err != nil {
		return admincontrol.OpsSystemLog{}, err
	}
	if store, ok := s.systemLogStore(); ok {
		return store.CreateSystemLog(ctx, log)
	}
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.OpsSystemLog{}, err
	}
	log.ID = nextID(collection.NextID, len(collection.Items))
	collection.Items = append(collection.Items, log)
	collection.NextID = log.ID + 1
	if err := s.saveTyped(ctx, settingsKeySystemLogs, collection, 0); err != nil {
		return admincontrol.OpsSystemLog{}, err
	}
	return log, nil
}

func (s *Service) ListSystemLogs(ctx context.Context, opts admincontrol.SystemLogListOptions) (admincontrol.SystemLogList, error) {
	if err := validateSystemLogListOptions(opts); err != nil {
		return admincontrol.SystemLogList{}, err
	}
	if store, ok := s.systemLogStore(); ok {
		return store.ListSystemLogs(ctx, opts)
	}
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.SystemLogList{}, err
	}
	items := make([]admincontrol.OpsSystemLog, 0, len(collection.Items))
	for _, item := range collection.Items {
		if !systemLogMatches(item, systemLogFilterFromListOptions(opts)) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.SystemLogList{Items: pageItems(items, listOptionsFromSystemLogOptions(opts)), Total: len(items)}, nil
}

func (s *Service) CleanupSystemLogs(ctx context.Context, filter admincontrol.SystemLogCleanupFilter) (admincontrol.SystemLogCleanupResult, error) {
	normalized, err := normalizeSystemLogCleanupFilter(filter)
	if err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	if store, ok := s.systemLogStore(); ok {
		return store.CleanupSystemLogs(ctx, normalized)
	}
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	kept := collection.Items[:0]
	var matched, deleted int
	for _, item := range collection.Items {
		if !systemLogMatches(item, normalized) {
			kept = append(kept, item)
			continue
		}
		matched++
		if normalized.DryRun || deleted >= normalized.MaxDelete {
			kept = append(kept, item)
			continue
		}
		deleted++
	}
	result := admincontrol.SystemLogCleanupResult{
		Matched:   matched,
		Deleted:   deleted,
		DryRun:    normalized.DryRun,
		MaxDelete: normalized.MaxDelete,
		Limited:   matched > deleted && !normalized.DryRun,
	}
	if normalized.DryRun {
		return result, nil
	}
	collection.Items = kept
	if err := s.saveTyped(ctx, settingsKeySystemLogs, collection, 0); err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	return result, nil
}

type systemLogCollection struct {
	NextID int                         `json:"next_id"`
	Items  []admincontrol.OpsSystemLog `json:"items"`
}

func systemLogFromRecordRequest(req admincontrol.RecordSystemLogRequest, now time.Time) (admincontrol.OpsSystemLog, error) {
	level := req.Level
	if level == "" {
		level = admincontrol.OpsSystemLogLevelInfo
	}
	message := strings.TrimSpace(req.Message)
	source := strings.TrimSpace(req.Source)
	if !level.Valid() || message == "" || source == "" {
		return admincontrol.OpsSystemLog{}, admincontrol.ErrInvalidInput
	}
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return admincontrol.OpsSystemLog{
		Level:     level,
		Message:   message,
		Source:    source,
		RequestID: strings.TrimSpace(req.RequestID),
		TraceID:   strings.TrimSpace(req.TraceID),
		Metadata:  cloneAnyMap(req.Metadata),
		CreatedAt: createdAt.UTC(),
	}, nil
}

func validateSystemLogListOptions(opts admincontrol.SystemLogListOptions) error {
	if opts.Level != "" && !opts.Level.Valid() {
		return admincontrol.ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return admincontrol.ErrInvalidInput
	}
	return nil
}

func normalizeSystemLogCleanupFilter(filter admincontrol.SystemLogCleanupFilter) (admincontrol.SystemLogCleanupFilter, error) {
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Level != "" && !filter.Level.Valid() {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	if filter.Level == "" && filter.Source == "" && filter.Query == "" && filter.Start == nil && filter.End == nil {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	if filter.MaxDelete == 0 {
		filter.MaxDelete = defaultSystemLogCleanupMax
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > maxSystemLogCleanupMax {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	return filter, nil
}

func systemLogFilterFromListOptions(opts admincontrol.SystemLogListOptions) admincontrol.SystemLogCleanupFilter {
	return admincontrol.SystemLogCleanupFilter{
		Level:  opts.Level,
		Source: strings.TrimSpace(opts.Source),
		Query:  strings.TrimSpace(opts.Query),
		Start:  opts.Start,
		End:    opts.End,
	}
}

func listOptionsFromSystemLogOptions(opts admincontrol.SystemLogListOptions) admincontrol.ListOptions {
	return admincontrol.ListOptions{Page: opts.Page, PageSize: opts.PageSize, Level: string(opts.Level)}
}

func systemLogMatches(log admincontrol.OpsSystemLog, filter admincontrol.SystemLogCleanupFilter) bool {
	if filter.Level != "" && log.Level != filter.Level {
		return false
	}
	if filter.Source != "" && !strings.EqualFold(log.Source, filter.Source) {
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
		if !strings.Contains(strings.ToLower(log.Message), query) && !strings.Contains(strings.ToLower(log.Source), query) && !strings.Contains(strings.ToLower(log.RequestID), query) && !strings.Contains(strings.ToLower(log.TraceID), query) {
			return false
		}
	}
	return true
}
