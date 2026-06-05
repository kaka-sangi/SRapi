package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
)

// ErrInvalidInput is returned for malformed monitor definitions or templates.
var ErrInvalidInput = errors.New("invalid channel monitor")

// Service provides admin-managed channel-monitor CRUD, run history, and template apply.
type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

func (s *Service) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	return s.store.ListDefinitions(ctx)
}

func (s *Service) GetDefinition(ctx context.Context, id int) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidInput
	}
	return s.store.GetDefinition(ctx, id)
}

func (s *Service) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Definition{}, ErrInvalidInput
	}
	scope, ok := normalizeScope(input.Scope)
	if !ok {
		return contract.Definition{}, ErrInvalidInput
	}
	input.Scope = scope
	input.ScopeRef = strings.TrimSpace(input.ScopeRef)
	input.Interval = normalizeInterval(input.Interval)
	input.Model = strings.TrimSpace(input.Model)
	req, err := normalizeRequest(input.Request)
	if err != nil {
		return contract.Definition{}, ErrInvalidInput
	}
	input.Request = req
	return s.store.CreateDefinition(ctx, input)
}

func (s *Service) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Scope != nil {
		scope, ok := normalizeScope(*input.Scope)
		if !ok {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Scope = &scope
	}
	if input.ScopeRef != nil {
		ref := strings.TrimSpace(*input.ScopeRef)
		input.ScopeRef = &ref
	}
	if input.Interval != nil {
		interval := normalizeInterval(*input.Interval)
		input.Interval = &interval
	}
	if input.Model != nil {
		model := strings.TrimSpace(*input.Model)
		input.Model = &model
	}
	if input.Request != nil {
		req, err := normalizeRequest(*input.Request)
		if err != nil {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Request = &req
	}
	return s.store.UpdateDefinition(ctx, id, input)
}

func (s *Service) DeleteDefinition(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteDefinition(ctx, id)
}

func (s *Service) ListTemplates(ctx context.Context) ([]contract.Template, error) {
	return s.store.ListTemplates(ctx)
}

func (s *Service) GetTemplate(ctx context.Context, id int) (contract.Template, error) {
	if id <= 0 {
		return contract.Template{}, ErrInvalidInput
	}
	return s.store.GetTemplate(ctx, id)
}

func (s *Service) CreateTemplate(ctx context.Context, input contract.CreateTemplate) (contract.Template, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Template{}, ErrInvalidInput
	}
	input.Description = strings.TrimSpace(input.Description)
	req, err := normalizeRequest(input.Request)
	if err != nil {
		return contract.Template{}, ErrInvalidInput
	}
	input.Request = req
	return s.store.CreateTemplate(ctx, input)
}

func (s *Service) UpdateTemplate(ctx context.Context, id int, input contract.UpdateTemplate) (contract.Template, error) {
	if id <= 0 {
		return contract.Template{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Template{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		input.Description = &desc
	}
	if input.Request != nil {
		req, err := normalizeRequest(*input.Request)
		if err != nil {
			return contract.Template{}, ErrInvalidInput
		}
		input.Request = &req
	}
	return s.store.UpdateTemplate(ctx, id, input)
}

func (s *Service) DeleteTemplate(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteTemplate(ctx, id)
}

// ApplyTemplate copies a template's custom request onto every supplied monitor
// definition and returns the updated definitions. monitorIDs that do not exist
// are skipped; the count of applied monitors is returned.
func (s *Service) ApplyTemplate(ctx context.Context, templateID int, monitorIDs []int) ([]contract.Definition, error) {
	if templateID <= 0 || len(monitorIDs) == 0 {
		return nil, ErrInvalidInput
	}
	template, err := s.store.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	request := template.Request
	applied := make([]contract.Definition, 0, len(monitorIDs))
	for _, id := range monitorIDs {
		if id <= 0 {
			continue
		}
		def, err := s.store.UpdateDefinition(ctx, id, contract.UpdateDefinition{Request: &request})
		if err != nil {
			if errors.Is(err, contract.ErrNotFound) {
				continue
			}
			return nil, err
		}
		applied = append(applied, def)
	}
	return applied, nil
}

func (s *Service) RecordRun(ctx context.Context, input contract.RecordRun) (contract.RunResult, error) {
	if input.MonitorID <= 0 || strings.TrimSpace(input.RunID) == "" {
		return contract.RunResult{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.Trigger) == "" {
		input.Trigger = "manual"
	}
	return s.store.RecordRun(ctx, input)
}

func (s *Service) ListRuns(ctx context.Context, monitorID int, limit int) ([]contract.RunResult, error) {
	if monitorID <= 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListRuns(ctx, monitorID, limit)
}

func normalizeScope(scope contract.Scope) (contract.Scope, bool) {
	switch contract.Scope(strings.ToLower(strings.TrimSpace(string(scope)))) {
	case contract.ScopeAccount, "":
		return contract.ScopeAccount, true
	case contract.ScopeGroup:
		return contract.ScopeGroup, true
	case contract.ScopeProvider:
		return contract.ScopeProvider, true
	case contract.ScopeModel:
		return contract.ScopeModel, true
	default:
		return "", false
	}
}

func normalizeInterval(interval int) int {
	if interval < 30 {
		return 30
	}
	if interval > 86400 {
		return 86400
	}
	return interval
}

func normalizeRequest(req contract.CustomRequest) (contract.CustomRequest, error) {
	out := contract.CustomRequest{
		Method:           strings.ToUpper(strings.TrimSpace(req.Method)),
		URL:              strings.TrimSpace(req.URL),
		Body:             strings.TrimSpace(req.Body),
		ResponseJSONPath: strings.TrimSpace(req.ResponseJSONPath),
		ResponseContains: strings.TrimSpace(req.ResponseContains),
	}
	switch out.Method {
	case "", "GET", "HEAD", "POST":
	default:
		return contract.CustomRequest{}, errors.New("unsupported probe method")
	}
	if out.Body != "" && !json.Valid([]byte(out.Body)) {
		return contract.CustomRequest{}, errors.New("probe body must be valid json")
	}
	out.Headers = cleanHeaders(req.Headers)
	out.ExpectedStatusCodes = cleanStatusCodes(req.ExpectedStatusCodes)
	return out, nil
}

func cleanHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanStatusCodes(codes []int) []int {
	if len(codes) == 0 {
		return nil
	}
	out := make([]int, 0, len(codes))
	seen := map[int]struct{}{}
	for _, code := range codes {
		if code < 100 || code > 599 {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
