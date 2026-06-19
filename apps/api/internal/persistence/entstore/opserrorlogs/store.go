package opserrorlogs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	entopserrorlog "github.com/srapi/srapi/apps/api/ent/opserrorlog"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
)

var ErrInvalidStore = errors.New("invalid ops error logs ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Insert(ctx context.Context, entry contract.Entry) (contract.Entry, error) {
	now := time.Now().UTC()
	occurred := entry.OccurredAt.UTC()
	if occurred.IsZero() {
		occurred = now
	}
	createdAt := entry.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := entry.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	resolution := entry.Resolution
	if resolution == "" {
		resolution = contract.ResolutionOpen
	}
	create := s.client.OpsErrorLog.Create().
		SetOccurredAt(occurred).
		SetRequestID(trim(entry.RequestID, 128)).
		SetTraceID(trim(entry.TraceID, 128)).
		SetNillableUserID(entry.UserID).
		SetNillableAPIKeyID(entry.APIKeyID).
		SetAPIKeyPrefix(trim(entry.APIKeyPrefix, 32)).
		SetNillableAccountID(entry.AccountID).
		SetNillableProviderID(entry.ProviderID).
		SetPlatform(trim(entry.Platform, 64)).
		SetSourceEndpoint(trim(entry.SourceEndpoint, 128)).
		SetTargetProtocol(trim(entry.TargetProtocol, 64)).
		SetModel(trim(entry.Model, 128)).
		SetNillableStatusCode(entry.StatusCode).
		SetUpstreamRequestID(trim(entry.UpstreamRequestID, 128)).
		SetStreamCompletionState(trim(entry.StreamCompletionState, 32)).
		SetAttemptNo(positiveOrDefault(entry.AttemptNo, 1)).
		SetLatencyMs(positiveOrZero(entry.LatencyMS)).
		SetInputTokens(positiveOrZero(entry.InputTokens)).
		SetOutputTokens(positiveOrZero(entry.OutputTokens)).
		SetUsageEstimated(entry.UsageEstimated).
		SetErrorClass(trim(defaultString(entry.ErrorClass, "unknown"), 64)).
		SetErrorPhase(trim(defaultString(entry.ErrorPhase, "upstream"), 64)).
		SetErrorOwner(trim(defaultString(entry.ErrorOwner, "provider"), 64)).
		SetErrorSource(trim(defaultString(entry.ErrorSource, "upstream_http"), 64)).
		SetErrorMessage(trim(entry.ErrorMessage, 2048)).
		SetErrorBodyExcerpt(trim(entry.ErrorBodyExcerpt, 8*1024)).
		SetUpstreamErrorsJSON(upstreamErrorsToJSON(entry.UpstreamErrors)).
		SetResolution(string(resolution)).
		SetResolutionNote(trim(entry.ResolutionNote, 2048)).
		SetNillableResolvedAt(entry.ResolvedAt).
		SetNillableResolvedByID(entry.ResolvedByID).
		SetCreatedAt(createdAt).
		SetUpdatedAt(updatedAt)
	row, err := create.Save(ctx)
	if err != nil {
		return contract.Entry{}, err
	}
	return toContract(row), nil
}

func (s *Store) List(ctx context.Context, filter contract.ListFilter) (contract.ListResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	predicates := listPredicates(filter)
	total, err := s.client.OpsErrorLog.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return contract.ListResult{}, err
	}
	rows, err := s.client.OpsErrorLog.Query().
		Where(predicates...).
		Order(entopserrorlog.ByOccurredAt(sql.OrderDesc()), entopserrorlog.ByID(sql.OrderDesc())).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return contract.ListResult{}, err
	}
	items := make([]contract.Entry, 0, len(rows))
	for _, row := range rows {
		items = append(items, toContract(row))
	}
	return contract.ListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Store) Get(ctx context.Context, id int64) (contract.Entry, error) {
	if id <= 0 {
		return contract.Entry{}, ErrInvalidStore
	}
	row, err := s.client.OpsErrorLog.Get(ctx, int(id))
	if err != nil {
		return contract.Entry{}, mapNotFound(err)
	}
	return toContract(row), nil
}

func (s *Store) UpdateResolution(ctx context.Context, req contract.UpdateResolutionRequest) (contract.Entry, error) {
	if req.ID <= 0 {
		return contract.Entry{}, ErrInvalidStore
	}
	at := req.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	update := s.client.OpsErrorLog.UpdateOneID(int(req.ID)).
		SetResolution(string(req.Resolution)).
		SetResolutionNote(trim(req.Note, 2048)).
		SetUpdatedAt(at)
	if req.ResolvedByID != nil {
		update.SetResolvedByID(*req.ResolvedByID)
	} else {
		update.ClearResolvedByID()
	}
	if req.Resolution == contract.ResolutionResolved {
		update.SetResolvedAt(at)
	} else {
		update.ClearResolvedAt()
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.Entry{}, mapNotFound(err)
	}
	return toContract(row), nil
}

func (s *Store) DeleteOlderThan(ctx context.Context, before time.Time) (int, error) {
	if before.IsZero() {
		return 0, ErrInvalidStore
	}
	return s.client.OpsErrorLog.Delete().
		Where(entopserrorlog.OccurredAtLT(before.UTC())).
		Exec(ctx)
}

func listPredicates(filter contract.ListFilter) []predicate.OpsErrorLog {
	var predicates []predicate.OpsErrorLog
	if filter.UserID != nil {
		predicates = append(predicates, entopserrorlog.UserIDEQ(*filter.UserID))
	}
	if filter.AccountID != nil {
		predicates = append(predicates, entopserrorlog.AccountIDEQ(*filter.AccountID))
	}
	if filter.ProviderID != nil {
		predicates = append(predicates, entopserrorlog.ProviderIDEQ(*filter.ProviderID))
	}
	if requestID := strings.TrimSpace(filter.RequestID); requestID != "" {
		predicates = append(predicates, entopserrorlog.RequestIDEQ(requestID))
	}
	if platform := strings.TrimSpace(filter.Platform); platform != "" {
		predicates = append(predicates, entopserrorlog.PlatformEQ(platform))
	}
	if sourceEndpoint := strings.TrimSpace(filter.SourceEndpoint); sourceEndpoint != "" {
		predicates = append(predicates, entopserrorlog.SourceEndpointEQ(sourceEndpoint))
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		predicates = append(predicates, entopserrorlog.ModelEQ(model))
	}
	if errorClass := strings.TrimSpace(filter.ErrorClass); errorClass != "" {
		predicates = append(predicates, entopserrorlog.ErrorClassIn(contract.ErrorClassFilterAliases(errorClass)...))
	}
	if phase := strings.TrimSpace(filter.ErrorPhase); phase != "" {
		predicates = append(predicates, entopserrorlog.ErrorPhaseEQ(phase))
	}
	if owner := strings.TrimSpace(filter.ErrorOwner); owner != "" {
		predicates = append(predicates, entopserrorlog.ErrorOwnerEQ(owner))
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		predicates = append(predicates, entopserrorlog.Or(
			entopserrorlog.RequestIDContainsFold(query),
			entopserrorlog.TraceIDContainsFold(query),
			entopserrorlog.APIKeyPrefixContainsFold(query),
			entopserrorlog.SourceEndpointContainsFold(query),
			entopserrorlog.TargetProtocolContainsFold(query),
			entopserrorlog.ModelContainsFold(query),
			entopserrorlog.UpstreamRequestIDContainsFold(query),
			entopserrorlog.StreamCompletionStateContainsFold(query),
			entopserrorlog.ErrorClassContainsFold(query),
			entopserrorlog.ErrorPhaseContainsFold(query),
			entopserrorlog.ErrorOwnerContainsFold(query),
			entopserrorlog.ErrorSourceContainsFold(query),
			entopserrorlog.ErrorMessageContainsFold(query),
			entopserrorlog.ErrorBodyExcerptContainsFold(query),
		))
	}
	if filter.Resolution != "" {
		predicates = append(predicates, entopserrorlog.ResolutionEQ(string(filter.Resolution)))
	}
	if filter.StatusCodeMin != nil {
		predicates = append(predicates, entopserrorlog.StatusCodeGTE(*filter.StatusCodeMin))
	}
	if filter.StatusCodeMax != nil {
		predicates = append(predicates, entopserrorlog.StatusCodeLTE(*filter.StatusCodeMax))
	}
	if filter.From != nil {
		predicates = append(predicates, entopserrorlog.OccurredAtGTE(filter.From.UTC()))
	}
	if filter.To != nil {
		predicates = append(predicates, entopserrorlog.OccurredAtLTE(filter.To.UTC()))
	}
	return predicates
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func toContract(row *ent.OpsErrorLog) contract.Entry {
	if row == nil {
		return contract.Entry{}
	}
	entry := contract.Entry{
		ID:                    int64(row.ID),
		OccurredAt:            row.OccurredAt.UTC(),
		RequestID:             row.RequestID,
		TraceID:               row.TraceID,
		APIKeyPrefix:          row.APIKeyPrefix,
		UserID:                row.UserID,
		APIKeyID:              row.APIKeyID,
		AccountID:             row.AccountID,
		ProviderID:            row.ProviderID,
		Platform:              row.Platform,
		SourceEndpoint:        row.SourceEndpoint,
		TargetProtocol:        row.TargetProtocol,
		Model:                 row.Model,
		StatusCode:            row.StatusCode,
		UpstreamRequestID:     row.UpstreamRequestID,
		StreamCompletionState: row.StreamCompletionState,
		AttemptNo:             row.AttemptNo,
		LatencyMS:             row.LatencyMs,
		InputTokens:           row.InputTokens,
		OutputTokens:          row.OutputTokens,
		UsageEstimated:        row.UsageEstimated,
		ErrorClass:            row.ErrorClass,
		ErrorPhase:            row.ErrorPhase,
		ErrorOwner:            row.ErrorOwner,
		ErrorSource:           row.ErrorSource,
		ErrorMessage:          row.ErrorMessage,
		ErrorBodyExcerpt:      row.ErrorBodyExcerpt,
		UpstreamErrors:        upstreamErrorsFromJSON(row.UpstreamErrorsJSON),
		Resolution:            contract.Resolution(row.Resolution),
		ResolutionNote:        row.ResolutionNote,
		ResolvedAt:            row.ResolvedAt,
		ResolvedByID:          row.ResolvedByID,
		CreatedAt:             row.CreatedAt.UTC(),
		UpdatedAt:             row.UpdatedAt.UTC(),
	}
	if entry.Resolution == "" {
		entry.Resolution = contract.ResolutionOpen
	}
	return entry
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func trim(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func upstreamErrorsToJSON(events []contract.UpstreamErrorEvent) []map[string]any {
	if len(events) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		item := map[string]any{
			"at_unix_ms":           event.AtUnixMs,
			"attempt_no":           event.AttemptNo,
			"account_name":         event.AccountName,
			"upstream_status_code": event.UpstreamStatusCode,
			"upstream_request_id":  event.UpstreamRequestID,
			"upstream_url":         event.UpstreamURL,
			"kind":                 event.Kind,
			"message":              event.Message,
			"body_excerpt":         event.BodyExcerpt,
		}
		if event.AccountID != nil {
			item["account_id"] = *event.AccountID
		}
		out = append(out, item)
	}
	return out
}

func upstreamErrorsFromJSON(values []map[string]any) []contract.UpstreamErrorEvent {
	if len(values) == 0 {
		return nil
	}
	out := make([]contract.UpstreamErrorEvent, 0, len(values))
	for _, value := range values {
		out = append(out, contract.UpstreamErrorEvent{
			AtUnixMs:           mapInt64(value, "at_unix_ms"),
			AttemptNo:          int(mapInt64(value, "attempt_no")),
			AccountID:          mapOptionalInt(value, "account_id"),
			AccountName:        mapString(value, "account_name"),
			UpstreamStatusCode: int(mapInt64(value, "upstream_status_code")),
			UpstreamRequestID:  mapString(value, "upstream_request_id"),
			UpstreamURL:        mapString(value, "upstream_url"),
			Kind:               mapString(value, "kind"),
			Message:            mapString(value, "message"),
			BodyExcerpt:        mapString(value, "body_excerpt"),
		})
	}
	return out
}

func mapString(value map[string]any, key string) string {
	if v, ok := value[key].(string); ok {
		return v
	}
	return ""
}

func mapOptionalInt(value map[string]any, key string) *int {
	if _, ok := value[key]; !ok {
		return nil
	}
	n := int(mapInt64(value, key))
	return &n
}

func mapInt64(value map[string]any, key string) int64 {
	switch v := value[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		var n int64
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n
		}
	}
	return 0
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

func mapNotFound(err error) error {
	if ent.IsNotFound(err) {
		return contract.ErrNotFound
	}
	return err
}
