package channelmonitors

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entdef "github.com/srapi/srapi/apps/api/ent/monitordefinition"
	enttpl "github.com/srapi/srapi/apps/api/ent/monitorrequesttemplate"
	entrun "github.com/srapi/srapi/apps/api/ent/monitorrunresult"
	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
)

var ErrInvalidStore = errors.New("invalid channel monitor ent store")

// Store is the Ent-backed implementation of the channel-monitor store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	now := time.Now().UTC()
	row, err := s.client.MonitorDefinition.Create().
		SetName(input.Name).
		SetEnabled(input.Enabled).
		SetScope(string(input.Scope)).
		SetScopeRef(input.ScopeRef).
		SetIntervalSeconds(input.Interval).
		SetModel(input.Model).
		SetRequestMethod(input.Request.Method).
		SetRequestURL(input.Request.URL).
		SetRequestHeaders(cloneHeaders(input.Request.Headers)).
		SetRequestBody(input.Request.Body).
		SetExpectedStatusCodes(cloneInts(input.Request.ExpectedStatusCodes)).
		SetResponseJSONPath(input.Request.ResponseJSONPath).
		SetResponseContains(input.Request.ResponseContains).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Definition{}, err
	}
	return toDefinition(row), nil
}

func (s *Store) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidStore
	}
	update := s.client.MonitorDefinition.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if input.Name != nil {
		update.SetName(*input.Name)
	}
	if input.Enabled != nil {
		update.SetEnabled(*input.Enabled)
	}
	if input.Scope != nil {
		update.SetScope(string(*input.Scope))
	}
	if input.ScopeRef != nil {
		update.SetScopeRef(*input.ScopeRef)
	}
	if input.Interval != nil {
		update.SetIntervalSeconds(*input.Interval)
	}
	if input.Model != nil {
		update.SetModel(*input.Model)
	}
	if input.Request != nil {
		update.
			SetRequestMethod(input.Request.Method).
			SetRequestURL(input.Request.URL).
			SetRequestHeaders(cloneHeaders(input.Request.Headers)).
			SetRequestBody(input.Request.Body).
			SetExpectedStatusCodes(cloneInts(input.Request.ExpectedStatusCodes)).
			SetResponseJSONPath(input.Request.ResponseJSONPath).
			SetResponseContains(input.Request.ResponseContains)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Definition{}, contract.ErrNotFound
		}
		return contract.Definition{}, err
	}
	return toDefinition(row), nil
}

func (s *Store) DeleteDefinition(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if _, err := s.client.MonitorRunResult.Delete().Where(entrun.MonitorIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if err := s.client.MonitorDefinition.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) GetDefinition(ctx context.Context, id int) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidStore
	}
	row, err := s.client.MonitorDefinition.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Definition{}, contract.ErrNotFound
		}
		return contract.Definition{}, err
	}
	return toDefinition(row), nil
}

func (s *Store) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	rows, err := s.client.MonitorDefinition.Query().Order(ent.Asc(entdef.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Definition, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDefinition(row))
	}
	return out, nil
}

func (s *Store) CreateTemplate(ctx context.Context, input contract.CreateTemplate) (contract.Template, error) {
	now := time.Now().UTC()
	row, err := s.client.MonitorRequestTemplate.Create().
		SetName(input.Name).
		SetDescription(input.Description).
		SetRequestMethod(input.Request.Method).
		SetRequestURL(input.Request.URL).
		SetRequestHeaders(cloneHeaders(input.Request.Headers)).
		SetRequestBody(input.Request.Body).
		SetExpectedStatusCodes(cloneInts(input.Request.ExpectedStatusCodes)).
		SetResponseJSONPath(input.Request.ResponseJSONPath).
		SetResponseContains(input.Request.ResponseContains).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Template{}, err
	}
	return toTemplate(row), nil
}

func (s *Store) UpdateTemplate(ctx context.Context, id int, input contract.UpdateTemplate) (contract.Template, error) {
	if id <= 0 {
		return contract.Template{}, ErrInvalidStore
	}
	update := s.client.MonitorRequestTemplate.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if input.Name != nil {
		update.SetName(*input.Name)
	}
	if input.Description != nil {
		update.SetDescription(*input.Description)
	}
	if input.Request != nil {
		update.
			SetRequestMethod(input.Request.Method).
			SetRequestURL(input.Request.URL).
			SetRequestHeaders(cloneHeaders(input.Request.Headers)).
			SetRequestBody(input.Request.Body).
			SetExpectedStatusCodes(cloneInts(input.Request.ExpectedStatusCodes)).
			SetResponseJSONPath(input.Request.ResponseJSONPath).
			SetResponseContains(input.Request.ResponseContains)
	}
	row, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Template{}, contract.ErrNotFound
		}
		return contract.Template{}, err
	}
	return toTemplate(row), nil
}

func (s *Store) DeleteTemplate(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidStore
	}
	if err := s.client.MonitorRequestTemplate.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return contract.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) GetTemplate(ctx context.Context, id int) (contract.Template, error) {
	if id <= 0 {
		return contract.Template{}, ErrInvalidStore
	}
	row, err := s.client.MonitorRequestTemplate.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Template{}, contract.ErrNotFound
		}
		return contract.Template{}, err
	}
	return toTemplate(row), nil
}

func (s *Store) ListTemplates(ctx context.Context) ([]contract.Template, error) {
	rows, err := s.client.MonitorRequestTemplate.Query().Order(ent.Asc(enttpl.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Template, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTemplate(row))
	}
	return out, nil
}

func (s *Store) RecordRun(ctx context.Context, input contract.RecordRun) (contract.RunResult, error) {
	row, err := s.client.MonitorRunResult.Create().
		SetMonitorID(input.MonitorID).
		SetRunID(input.RunID).
		SetOk(input.OK).
		SetCheckedCount(input.CheckedCount).
		SetOkCount(input.OKCount).
		SetLatencyMs(input.LatencyMS).
		SetTrigger(input.Trigger).
		SetResults(resultsToJSON(input.Results)).
		SetCreatedAt(time.Now().UTC()).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return contract.RunResult{}, err
	}
	return toRun(row), nil
}

func (s *Store) ListRuns(ctx context.Context, monitorID int, limit int) ([]contract.RunResult, error) {
	query := s.client.MonitorRunResult.Query().
		Where(entrun.MonitorIDEQ(monitorID)).
		Order(ent.Desc(entrun.FieldID))
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.RunResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRun(row))
	}
	return out, nil
}

func toDefinition(row *ent.MonitorDefinition) contract.Definition {
	return contract.Definition{
		ID:       row.ID,
		Name:     row.Name,
		Enabled:  row.Enabled,
		Scope:    contract.Scope(row.Scope),
		ScopeRef: row.ScopeRef,
		Interval: row.IntervalSeconds,
		Model:    row.Model,
		Request: contract.CustomRequest{
			Method:              row.RequestMethod,
			URL:                 row.RequestURL,
			Headers:             cloneHeaders(row.RequestHeaders),
			Body:                row.RequestBody,
			ExpectedStatusCodes: cloneInts(row.ExpectedStatusCodes),
			ResponseJSONPath:    row.ResponseJSONPath,
			ResponseContains:    row.ResponseContains,
		},
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toTemplate(row *ent.MonitorRequestTemplate) contract.Template {
	return contract.Template{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Request: contract.CustomRequest{
			Method:              row.RequestMethod,
			URL:                 row.RequestURL,
			Headers:             cloneHeaders(row.RequestHeaders),
			Body:                row.RequestBody,
			ExpectedStatusCodes: cloneInts(row.ExpectedStatusCodes),
			ResponseJSONPath:    row.ResponseJSONPath,
			ResponseContains:    row.ResponseContains,
		},
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toRun(row *ent.MonitorRunResult) contract.RunResult {
	return contract.RunResult{
		ID:           row.ID,
		MonitorID:    row.MonitorID,
		RunID:        row.RunID,
		OK:           row.Ok,
		CheckedCount: row.CheckedCount,
		OKCount:      row.OkCount,
		LatencyMS:    row.LatencyMs,
		Trigger:      row.Trigger,
		Results:      resultsFromJSON(row.Results),
		CreatedAt:    row.CreatedAt,
	}
}

func resultsToJSON(results []contract.CheckResult) []map[string]any {
	if len(results) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"account_id":   r.AccountID,
			"account_name": r.AccountName,
			"provider_id":  r.ProviderID,
			"model":        r.Model,
			"ok":           r.OK,
			"status_code":  r.StatusCode,
			"latency_ms":   r.LatencyMS,
			"error_class":  r.ErrorClass,
			"metadata":     r.Metadata,
		})
	}
	return out
}

func resultsFromJSON(rows []map[string]any) []contract.CheckResult {
	if len(rows) == 0 {
		return nil
	}
	out := make([]contract.CheckResult, 0, len(rows))
	for _, row := range rows {
		result := contract.CheckResult{
			AccountID:   jsonInt(row["account_id"]),
			AccountName: jsonString(row["account_name"]),
			ProviderID:  jsonInt(row["provider_id"]),
			Model:       jsonString(row["model"]),
			OK:          jsonBool(row["ok"]),
			StatusCode:  jsonInt(row["status_code"]),
			LatencyMS:   jsonInt(row["latency_ms"]),
			ErrorClass:  jsonString(row["error_class"]),
		}
		if metadata, ok := row["metadata"].(map[string]any); ok {
			result.Metadata = metadata
		}
		out = append(out, result)
	}
	return out
}

func jsonInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func jsonString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func jsonBool(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}

func cloneInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	return append([]int(nil), values...)
}
