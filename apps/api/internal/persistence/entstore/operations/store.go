package operations

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	entaccounthealthsnapshot "github.com/srapi/srapi/apps/api/ent/accounthealthsnapshot"
	entauditlog "github.com/srapi/srapi/apps/api/ent/auditlog"
	entobsalertevent "github.com/srapi/srapi/apps/api/ent/obsalertevent"
	entobsslodefinition "github.com/srapi/srapi/apps/api/ent/obsslodefinition"
	entopssystemlog "github.com/srapi/srapi/apps/api/ent/opssystemlog"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	entschedulerdecision "github.com/srapi/srapi/apps/api/ent/schedulerdecision"
	entschedulerfeedback "github.com/srapi/srapi/apps/api/ent/schedulerfeedback"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

var ErrInvalidStore = errors.New("invalid operations ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Cleanup(ctx context.Context, cutoffs contract.RetentionCutoffs) (contract.CleanupResult, error) {
	var result contract.CleanupResult
	batchLimit := cutoffs.BatchLimit
	if batchLimit <= 0 {
		batchLimit = 1000
	}
	if cutoffs.UsageLogs != nil {
		deleted, limited, err := cleanupUsageLogs(ctx, s.client, *cutoffs.UsageLogs, batchLimit)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.UsageLogs = deleted
		result.Limited = result.Limited || limited
	}
	if cutoffs.SchedulerFeedbacks != nil {
		deleted, limited, err := cleanupSchedulerFeedbacks(ctx, s.client, *cutoffs.SchedulerFeedbacks, batchLimit)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.SchedulerFeedbacks = deleted
		result.Limited = result.Limited || limited
	}
	if cutoffs.SchedulerDecisions != nil {
		deleted, limited, err := cleanupSchedulerDecisions(ctx, s.client, *cutoffs.SchedulerDecisions, batchLimit)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.SchedulerDecisions = deleted
		result.Limited = result.Limited || limited
	}
	if cutoffs.AuditLogs != nil {
		deleted, limited, err := cleanupAuditLogs(ctx, s.client, *cutoffs.AuditLogs, batchLimit)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.AuditLogs = deleted
		result.Limited = result.Limited || limited
	}
	if cutoffs.AccountHealthSnapshots != nil {
		deleted, limited, err := cleanupAccountHealthSnapshots(ctx, s.client, *cutoffs.AccountHealthSnapshots, batchLimit)
		if err != nil {
			return contract.CleanupResult{}, err
		}
		result.AccountHealthSnapshots = deleted
		result.Limited = result.Limited || limited
	}
	return result, nil
}

func (s *Store) CreateSystemLog(ctx context.Context, input contract.OpsSystemLog) (contract.OpsSystemLog, error) {
	if strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Message) == "" || !input.Level.Valid() {
		return contract.OpsSystemLog{}, contract.ErrInvalidInput
	}
	create := s.client.OpsSystemLog.Create().
		SetLevel(string(input.Level)).
		SetSource(strings.TrimSpace(input.Source)).
		SetMessage(strings.TrimSpace(input.Message)).
		SetRequestID(strings.TrimSpace(input.RequestID)).
		SetTraceID(strings.TrimSpace(input.TraceID)).
		SetMetadataJSON(cloneMap(input.Metadata))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt.UTC())
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.OpsSystemLog{}, err
	}
	return toSystemLog(row), nil
}

func (s *Store) ListSystemLogs(ctx context.Context, opts contract.SystemLogListOptions) (contract.SystemLogList, error) {
	if opts.Level != "" && !opts.Level.Valid() {
		return contract.SystemLogList{}, contract.ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return contract.SystemLogList{}, contract.ErrInvalidInput
	}
	page, pageSize := normalizePage(opts.Page, opts.PageSize)
	filter := contract.SystemLogCleanupFilter{
		Level:     opts.Level,
		Source:    opts.Source,
		Query:     opts.Query,
		RequestID: opts.RequestID,
		TraceID:   opts.TraceID,
		Start:     opts.Start,
		End:       opts.End,
	}
	predicates := systemLogPredicates(filter)
	total, err := s.client.OpsSystemLog.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return contract.SystemLogList{}, err
	}
	rows, err := s.client.OpsSystemLog.Query().
		Where(predicates...).
		Order(entopssystemlog.ByCreatedAt(entsql.OrderDesc()), entopssystemlog.ByID(entsql.OrderDesc())).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return contract.SystemLogList{}, err
	}
	items := make([]contract.OpsSystemLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSystemLog(row))
	}
	return contract.SystemLogList{Items: items, Total: total}, nil
}

func (s *Store) SystemLogStats(ctx context.Context) (contract.SystemLogStats, error) {
	stats := contract.SystemLogStats{LevelCounts: map[contract.OpsSystemLogLevel]int{}}
	total, err := s.client.OpsSystemLog.Query().Count(ctx)
	if err != nil {
		return contract.SystemLogStats{}, err
	}
	stats.TotalCount = total
	for _, level := range []contract.OpsSystemLogLevel{
		contract.OpsSystemLogLevelDebug,
		contract.OpsSystemLogLevelInfo,
		contract.OpsSystemLogLevelWarn,
		contract.OpsSystemLogLevelError,
	} {
		count, err := s.client.OpsSystemLog.Query().Where(entopssystemlog.LevelEQ(string(level))).Count(ctx)
		if err != nil {
			return contract.SystemLogStats{}, err
		}
		stats.LevelCounts[level] = count
	}
	row, err := s.client.OpsSystemLog.Query().
		Order(entopssystemlog.ByCreatedAt(entsql.OrderDesc()), entopssystemlog.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return contract.SystemLogStats{}, err
	}
	if row != nil {
		lastLog := toSystemLog(row)
		stats.LastLog = &lastLog
	}
	errorRow, err := s.client.OpsSystemLog.Query().
		Where(entopssystemlog.LevelEQ(string(contract.OpsSystemLogLevelError))).
		Order(entopssystemlog.ByCreatedAt(entsql.OrderDesc()), entopssystemlog.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return contract.SystemLogStats{}, err
	}
	if errorRow != nil {
		lastError := toSystemLog(errorRow)
		stats.LastError = &lastError
	}
	return stats, nil
}

func (s *Store) CleanupSystemLogs(ctx context.Context, filter contract.SystemLogCleanupFilter) (contract.SystemLogCleanupResult, error) {
	normalized, err := normalizeSystemLogCleanupFilter(filter)
	if err != nil {
		return contract.SystemLogCleanupResult{}, err
	}
	predicates := systemLogPredicates(normalized)
	matched, err := s.client.OpsSystemLog.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return contract.SystemLogCleanupResult{}, err
	}
	result := contract.SystemLogCleanupResult{
		Matched:   matched,
		DryRun:    normalized.DryRun,
		MaxDelete: normalized.MaxDelete,
	}
	if normalized.DryRun || matched == 0 {
		return result, nil
	}
	rows, err := s.client.OpsSystemLog.Query().
		Where(predicates...).
		Order(entopssystemlog.ByCreatedAt(), entopssystemlog.ByID()).
		Limit(normalized.MaxDelete).
		IDs(ctx)
	if err != nil {
		return contract.SystemLogCleanupResult{}, err
	}
	deleted, err := s.client.OpsSystemLog.Delete().
		Where(entopssystemlog.IDIn(rows...)).
		Exec(ctx)
	if err != nil {
		return contract.SystemLogCleanupResult{}, err
	}
	result.Deleted = deleted
	result.Limited = matched > deleted
	return result, nil
}

func cleanupUsageLogs(ctx context.Context, client *ent.Client, cutoff time.Time, batchLimit int) (int, bool, error) {
	ids, err := client.UsageLog.Query().
		Where(entusagelog.CreatedAtLT(cutoff)).
		Order(entusagelog.ByID()).
		Limit(batchLimit + 1).
		IDs(ctx)
	if err != nil {
		return 0, false, err
	}
	limited := len(ids) > batchLimit
	ids = capIDs(ids, batchLimit)
	if len(ids) == 0 {
		return 0, false, nil
	}
	deleted, err := client.UsageLog.Delete().
		Where(entusagelog.IDIn(ids...)).
		Exec(ctx)
	return deleted, limited, err
}

func cleanupSchedulerFeedbacks(ctx context.Context, client *ent.Client, cutoff time.Time, batchLimit int) (int, bool, error) {
	ids, err := client.SchedulerFeedback.Query().
		Where(entschedulerfeedback.CreatedAtLT(cutoff)).
		Order(entschedulerfeedback.ByID()).
		Limit(batchLimit + 1).
		IDs(ctx)
	if err != nil {
		return 0, false, err
	}
	limited := len(ids) > batchLimit
	ids = capIDs(ids, batchLimit)
	if len(ids) == 0 {
		return 0, false, nil
	}
	deleted, err := client.SchedulerFeedback.Delete().
		Where(entschedulerfeedback.IDIn(ids...)).
		Exec(ctx)
	return deleted, limited, err
}

func cleanupSchedulerDecisions(ctx context.Context, client *ent.Client, cutoff time.Time, batchLimit int) (int, bool, error) {
	ids, err := client.SchedulerDecision.Query().
		Where(entschedulerdecision.CreatedAtLT(cutoff)).
		Order(entschedulerdecision.ByID()).
		Limit(batchLimit + 1).
		IDs(ctx)
	if err != nil {
		return 0, false, err
	}
	limited := len(ids) > batchLimit
	ids = capIDs(ids, batchLimit)
	if len(ids) == 0 {
		return 0, false, nil
	}
	deleted, err := client.SchedulerDecision.Delete().
		Where(entschedulerdecision.IDIn(ids...)).
		Exec(ctx)
	return deleted, limited, err
}

func cleanupAuditLogs(ctx context.Context, client *ent.Client, cutoff time.Time, batchLimit int) (int, bool, error) {
	ids, err := client.AuditLog.Query().
		Where(entauditlog.CreatedAtLT(cutoff)).
		Order(entauditlog.ByID()).
		Limit(batchLimit + 1).
		IDs(ctx)
	if err != nil {
		return 0, false, err
	}
	limited := len(ids) > batchLimit
	ids = capIDs(ids, batchLimit)
	if len(ids) == 0 {
		return 0, false, nil
	}
	deleted, err := client.AuditLog.Delete().
		Where(entauditlog.IDIn(ids...)).
		Exec(ctx)
	return deleted, limited, err
}

func cleanupAccountHealthSnapshots(ctx context.Context, client *ent.Client, cutoff time.Time, batchLimit int) (int, bool, error) {
	ids, err := client.AccountHealthSnapshot.Query().
		Where(entaccounthealthsnapshot.SnapshotAtLT(cutoff)).
		Order(entaccounthealthsnapshot.ByID()).
		Limit(batchLimit + 1).
		IDs(ctx)
	if err != nil {
		return 0, false, err
	}
	limited := len(ids) > batchLimit
	ids = capIDs(ids, batchLimit)
	if len(ids) == 0 {
		return 0, false, nil
	}
	deleted, err := client.AccountHealthSnapshot.Delete().
		Where(entaccounthealthsnapshot.IDIn(ids...)).
		Exec(ctx)
	return deleted, limited, err
}

func capIDs(ids []int, limit int) []int {
	if limit <= 0 {
		return nil
	}
	if len(ids) > limit {
		return ids[:limit]
	}
	return ids
}

func (s *Store) CreateSLO(ctx context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	create := s.client.ObsSLODefinition.Create().
		SetName(input.Name).
		SetSliType(string(input.SLIType)).
		SetObjective(input.Objective).
		SetWindowDays(input.WindowDays).
		SetStatus(string(input.Status)).
		SetFilterJSON(sloFilterJSON(input.Filter)).
		SetAlertPolicyJSON(alertPolicyJSON(input.AlertPolicy))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.SLODefinition{}, err
	}
	return toSLO(row), nil
}

func (s *Store) UpdateSLO(ctx context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	update := s.client.ObsSLODefinition.UpdateOneID(input.ID).
		SetName(input.Name).
		SetSliType(string(input.SLIType)).
		SetObjective(input.Objective).
		SetWindowDays(input.WindowDays).
		SetStatus(string(input.Status)).
		SetFilterJSON(sloFilterJSON(input.Filter)).
		SetAlertPolicyJSON(alertPolicyJSON(input.AlertPolicy))
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.SLODefinition{}, mapNotFound(err)
	}
	return toSLO(row), nil
}

func (s *Store) FindSLOByID(ctx context.Context, id int) (contract.SLODefinition, error) {
	row, err := s.client.ObsSLODefinition.Get(ctx, id)
	if err != nil {
		return contract.SLODefinition{}, mapNotFound(err)
	}
	return toSLO(row), nil
}

func (s *Store) DeleteSLO(ctx context.Context, id int) error {
	if err := s.client.ObsSLODefinition.DeleteOneID(id).Exec(ctx); err != nil {
		return mapNotFound(err)
	}
	return nil
}

func (s *Store) ListSLOs(ctx context.Context) ([]contract.SLODefinition, error) {
	rows, err := s.client.ObsSLODefinition.Query().
		Order(entobsslodefinition.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.SLODefinition, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSLO(row))
	}
	return out, nil
}

func (s *Store) CreateAlert(ctx context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	create := s.client.ObsAlertEvent.Create().
		SetNillableSloID(input.SLOID).
		SetRuleID(input.RuleID).
		SetSeverity(string(input.Severity)).
		SetStatus(string(input.Status)).
		SetFingerprint(input.Fingerprint).
		SetSummary(input.Summary).
		SetDetailsJSON(cloneMap(input.Details)).
		SetStartedAt(input.StartedAt).
		SetNillableResolvedAt(input.ResolvedAt).
		SetNillableAcknowledgedAt(input.AcknowledgedAt).
		SetNillableAcknowledgedBy(input.AcknowledgedBy).
		SetNillableSuppressedBy(input.SuppressedBy)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.AlertEvent{}, err
	}
	return toAlert(row), nil
}

func (s *Store) UpdateAlert(ctx context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	update := s.client.ObsAlertEvent.UpdateOneID(input.ID).
		SetRuleID(input.RuleID).
		SetSeverity(string(input.Severity)).
		SetStatus(string(input.Status)).
		SetFingerprint(input.Fingerprint).
		SetSummary(input.Summary).
		SetDetailsJSON(cloneMap(input.Details)).
		SetStartedAt(input.StartedAt)
	if input.SLOID == nil {
		update.ClearSloID()
	} else {
		update.SetSloID(*input.SLOID)
	}
	if input.ResolvedAt == nil {
		update.ClearResolvedAt()
	} else {
		update.SetResolvedAt(*input.ResolvedAt)
	}
	if input.AcknowledgedAt == nil {
		update.ClearAcknowledgedAt()
	} else {
		update.SetAcknowledgedAt(*input.AcknowledgedAt)
	}
	if input.AcknowledgedBy == nil {
		update.ClearAcknowledgedBy()
	} else {
		update.SetAcknowledgedBy(*input.AcknowledgedBy)
	}
	if input.SuppressedBy == nil {
		update.ClearSuppressedBy()
	} else {
		update.SetSuppressedBy(*input.SuppressedBy)
	}
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.AlertEvent{}, mapNotFound(err)
	}
	return toAlert(row), nil
}

func (s *Store) FindAlertByID(ctx context.Context, id int) (contract.AlertEvent, error) {
	row, err := s.client.ObsAlertEvent.Get(ctx, id)
	if err != nil {
		return contract.AlertEvent{}, mapNotFound(err)
	}
	return toAlert(row), nil
}

func (s *Store) ListAlerts(ctx context.Context) ([]contract.AlertEvent, error) {
	rows, err := s.client.ObsAlertEvent.Query().
		Order(entobsalertevent.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AlertEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlert(row))
	}
	return out, nil
}

func (s *Store) ListUsageLogs(ctx context.Context) ([]usagecontract.UsageLog, error) {
	return s.ListUsageLogsSince(ctx, time.Time{})
}

// ListUsageLogsSince returns usage logs created at or after `since`. A zero
// `since` returns all logs. Observability aggregations pass their own lookback
// window so they never scan the whole (retention-bounded but potentially huge)
// usage_logs table into memory.
func (s *Store) ListUsageLogsSince(ctx context.Context, since time.Time) ([]usagecontract.UsageLog, error) {
	query := s.client.UsageLog.Query().Order(entusagelog.ByID())
	if !since.IsZero() {
		query = query.Where(entusagelog.CreatedAtGTE(since))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]usagecontract.UsageLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, usagecontract.UsageLog{
			ID:                    row.ID,
			RequestID:             row.RequestID,
			AttemptNo:             row.AttemptNo,
			UserID:                row.UserID,
			APIKeyID:              row.APIKeyID,
			ProviderID:            cloneInt(row.ProviderID),
			AccountID:             cloneInt(row.AccountID),
			SourceProtocol:        row.SourceProtocol,
			SourceEndpoint:        row.SourceEndpoint,
			TargetProtocol:        row.TargetProtocol,
			Model:                 row.Model,
			InputTokens:           row.InputTokens,
			OutputTokens:          row.OutputTokens,
			CachedTokens:          row.CachedTokens,
			TotalTokens:           row.TotalTokens,
			UsageEstimated:        row.UsageEstimated,
			LatencyMS:             row.LatencyMs,
			Success:               row.Success,
			ErrorClass:            cloneString(row.ErrorClass),
			Cost:                  row.Cost,
			Currency:              row.Currency,
			ChargedAt:             cloneTime(row.ChargedAt),
			CompatibilityWarnings: cloneStrings(row.CompatibilityWarningsJSON),
			CreatedAt:             row.CreatedAt,
		})
	}
	return out, nil
}

func toSLO(row *ent.ObsSLODefinition) contract.SLODefinition {
	return contract.SLODefinition{
		ID:          row.ID,
		Name:        row.Name,
		SLIType:     contract.SLIType(row.SliType),
		Objective:   row.Objective,
		WindowDays:  row.WindowDays,
		Status:      contract.SLOStatus(row.Status),
		Filter:      sloFilterFromJSON(row.FilterJSON),
		AlertPolicy: alertPolicyFromJSON(row.AlertPolicyJSON),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toAlert(row *ent.ObsAlertEvent) contract.AlertEvent {
	return contract.AlertEvent{
		ID:             row.ID,
		SLOID:          cloneInt(row.SloID),
		RuleID:         row.RuleID,
		Severity:       contract.AlertSeverity(row.Severity),
		Status:         contract.AlertStatus(row.Status),
		Fingerprint:    row.Fingerprint,
		Summary:        row.Summary,
		Details:        cloneMap(row.DetailsJSON),
		StartedAt:      row.StartedAt,
		ResolvedAt:     cloneTime(row.ResolvedAt),
		AcknowledgedAt: cloneTime(row.AcknowledgedAt),
		AcknowledgedBy: cloneInt(row.AcknowledgedBy),
		SuppressedBy:   cloneString(row.SuppressedBy),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func sloFilterJSON(filter contract.SLOFilter) map[string]any {
	out := map[string]any{
		"source_endpoint":     filter.SourceEndpoint,
		"model":               filter.Model,
		"error_owner_exclude": cloneStrings(filter.ErrorOwnerExclude),
	}
	if filter.ProviderID != nil {
		out["provider_id"] = *filter.ProviderID
	}
	return out
}

func sloFilterFromJSON(value map[string]any) contract.SLOFilter {
	filter := contract.SLOFilter{
		SourceEndpoint:    stringFromMap(value, "source_endpoint"),
		Model:             stringFromMap(value, "model"),
		ErrorOwnerExclude: stringsFromMap(value, "error_owner_exclude"),
	}
	if providerID, ok := intFromMap(value, "provider_id"); ok {
		filter.ProviderID = &providerID
	}
	return filter
}

func alertPolicyJSON(policy contract.AlertPolicy) map[string]any {
	thresholds := make([]any, 0, len(policy.Thresholds))
	for _, threshold := range policy.Thresholds {
		thresholds = append(thresholds, map[string]any{
			"severity":             string(threshold.Severity),
			"short_window_seconds": int(threshold.ShortWindow / time.Second),
			"long_window_seconds":  int(threshold.LongWindow / time.Second),
			"burn_rate":            threshold.BurnRate,
			"min_request_count":    threshold.MinRequestCount,
		})
	}
	return map[string]any{
		"name":       policy.Name,
		"thresholds": thresholds,
	}
}

func alertPolicyFromJSON(value map[string]any) contract.AlertPolicy {
	policy := contract.AlertPolicy{Name: stringFromMap(value, "name")}
	if items, ok := value["thresholds"].([]any); ok {
		policy.Thresholds = make([]contract.BurnRateThreshold, 0, len(items))
		for _, item := range items {
			raw, ok := item.(map[string]any)
			if !ok {
				continue
			}
			shortWindow, _ := intFromMap(raw, "short_window_seconds")
			longWindow, _ := intFromMap(raw, "long_window_seconds")
			burnRate, _ := floatFromMap(raw, "burn_rate")
			minRequestCount, _ := intFromMap(raw, "min_request_count")
			policy.Thresholds = append(policy.Thresholds, contract.BurnRateThreshold{
				Severity:        contract.AlertSeverity(stringFromMap(raw, "severity")),
				ShortWindow:     time.Duration(shortWindow) * time.Second,
				LongWindow:      time.Duration(longWindow) * time.Second,
				BurnRate:        burnRate,
				MinRequestCount: minRequestCount,
			})
		}
	}
	return policy
}

func mapNotFound(err error) error {
	if ent.IsNotFound(err) {
		return contract.ErrNotFound
	}
	return err
}

func systemLogPredicates(filter contract.SystemLogCleanupFilter) []predicate.OpsSystemLog {
	var predicates []predicate.OpsSystemLog
	if filter.Level != "" {
		predicates = append(predicates, entopssystemlog.LevelEQ(string(filter.Level)))
	}
	if source := strings.TrimSpace(filter.Source); source != "" {
		predicates = append(predicates, entopssystemlog.SourceEqualFold(source))
	}
	if requestID := strings.TrimSpace(filter.RequestID); requestID != "" {
		predicates = append(predicates, entopssystemlog.RequestIDEQ(requestID))
	}
	if traceID := strings.TrimSpace(filter.TraceID); traceID != "" {
		predicates = append(predicates, entopssystemlog.TraceIDEQ(traceID))
	}
	if filter.Start != nil {
		predicates = append(predicates, entopssystemlog.CreatedAtGTE(filter.Start.UTC()))
	}
	if filter.End != nil {
		predicates = append(predicates, entopssystemlog.CreatedAtLT(filter.End.UTC()))
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		predicates = append(predicates, entopssystemlog.Or(
			entopssystemlog.MessageContainsFold(query),
			entopssystemlog.SourceContainsFold(query),
			entopssystemlog.RequestIDContainsFold(query),
			entopssystemlog.TraceIDContainsFold(query),
			systemLogMetadataContainsFold(query),
		))
	}
	return predicates
}

func systemLogMetadataContainsFold(query string) predicate.OpsSystemLog {
	return predicate.OpsSystemLog(func(s *entsql.Selector) {
		column := s.C(entopssystemlog.FieldMetadataJSON)
		s.Where(entsql.ExprP("LOWER(CAST("+column+" AS TEXT)) LIKE ? ESCAPE '\\'", "%"+escapeSystemLogLikePattern(strings.ToLower(query))+"%"))
	})
}

func escapeSystemLogLikePattern(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func normalizeSystemLogCleanupFilter(filter contract.SystemLogCleanupFilter) (contract.SystemLogCleanupFilter, error) {
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Query = strings.TrimSpace(filter.Query)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	filter.TraceID = strings.TrimSpace(filter.TraceID)
	if filter.Level != "" && !filter.Level.Valid() {
		return contract.SystemLogCleanupFilter{}, contract.ErrInvalidInput
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return contract.SystemLogCleanupFilter{}, contract.ErrInvalidInput
	}
	if filter.Level == "" && filter.Source == "" && filter.Query == "" && filter.RequestID == "" && filter.TraceID == "" && filter.Start == nil && filter.End == nil {
		return contract.SystemLogCleanupFilter{}, contract.ErrInvalidInput
	}
	if filter.MaxDelete == 0 {
		filter.MaxDelete = 1000
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > 10000 {
		return contract.SystemLogCleanupFilter{}, contract.ErrInvalidInput
	}
	return filter, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return page, pageSize
}

func toSystemLog(row *ent.OpsSystemLog) contract.OpsSystemLog {
	if row == nil {
		return contract.OpsSystemLog{}
	}
	return contract.OpsSystemLog{
		ID:        row.ID,
		Level:     contract.OpsSystemLogLevel(row.Level),
		Message:   row.Message,
		Source:    row.Source,
		RequestID: row.RequestID,
		TraceID:   row.TraceID,
		Metadata:  cloneMap(row.MetadataJSON),
		CreatedAt: row.CreatedAt,
	}
}

func stringFromMap(value map[string]any, key string) string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(toString(typed))
	}
}

func stringsFromMap(value map[string]any, key string) []string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return cloneStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(toString(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func intFromMap(value map[string]any, key string) (int, bool) {
	raw, ok := value[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch typed := raw.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func floatFromMap(value map[string]any, key string) (float64, bool) {
	raw, ok := value[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch typed := raw.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return ""
	}
	switch typed := decoded.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return strings.Trim(string(raw), `"`)
	}
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return map[string]any{}
	}
	return out
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
