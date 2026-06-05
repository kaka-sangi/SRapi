package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
)

// Store is an in-memory implementation of the channel-monitor store.
type Store struct {
	mu          sync.Mutex
	definitions map[int]contract.Definition
	templates   map[int]contract.Template
	runs        map[int]contract.RunResult
	defSeq      int
	tplSeq      int
	runSeq      int
	clock       func() time.Time
}

func New() *Store {
	return &Store{
		definitions: make(map[int]contract.Definition),
		templates:   make(map[int]contract.Template),
		runs:        make(map[int]contract.RunResult),
		clock:       time.Now,
	}
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func cloneRequest(req contract.CustomRequest) contract.CustomRequest {
	out := req
	if req.Headers != nil {
		headers := make(map[string]string, len(req.Headers))
		for k, v := range req.Headers {
			headers[k] = v
		}
		out.Headers = headers
	}
	if req.ExpectedStatusCodes != nil {
		out.ExpectedStatusCodes = append([]int(nil), req.ExpectedStatusCodes...)
	}
	return out
}

func cloneResults(results []contract.CheckResult) []contract.CheckResult {
	if results == nil {
		return nil
	}
	out := make([]contract.CheckResult, len(results))
	copy(out, results)
	return out
}

func (s *Store) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defSeq++
	now := s.now()
	def := contract.Definition{
		ID:        s.defSeq,
		Name:      input.Name,
		Enabled:   input.Enabled,
		Scope:     input.Scope,
		ScopeRef:  input.ScopeRef,
		Interval:  input.Interval,
		Model:     input.Model,
		Request:   cloneRequest(input.Request),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.definitions[def.ID] = def
	return def, nil
}

func (s *Store) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def, ok := s.definitions[id]
	if !ok {
		return contract.Definition{}, contract.ErrNotFound
	}
	if input.Name != nil {
		def.Name = *input.Name
	}
	if input.Enabled != nil {
		def.Enabled = *input.Enabled
	}
	if input.Scope != nil {
		def.Scope = *input.Scope
	}
	if input.ScopeRef != nil {
		def.ScopeRef = *input.ScopeRef
	}
	if input.Interval != nil {
		def.Interval = *input.Interval
	}
	if input.Model != nil {
		def.Model = *input.Model
	}
	if input.Request != nil {
		def.Request = cloneRequest(*input.Request)
	}
	def.UpdatedAt = s.now()
	s.definitions[id] = def
	return def, nil
}

func (s *Store) DeleteDefinition(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.definitions[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.definitions, id)
	for runID, run := range s.runs {
		if run.MonitorID == id {
			delete(s.runs, runID)
		}
	}
	return nil
}

func (s *Store) GetDefinition(ctx context.Context, id int) (contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def, ok := s.definitions[id]
	if !ok {
		return contract.Definition{}, contract.ErrNotFound
	}
	return def, nil
}

func (s *Store) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Definition, 0, len(s.definitions))
	for _, def := range s.definitions {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) CreateTemplate(ctx context.Context, input contract.CreateTemplate) (contract.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tplSeq++
	now := s.now()
	tpl := contract.Template{
		ID:          s.tplSeq,
		Name:        input.Name,
		Description: input.Description,
		Request:     cloneRequest(input.Request),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.templates[tpl.ID] = tpl
	return tpl, nil
}

func (s *Store) UpdateTemplate(ctx context.Context, id int, input contract.UpdateTemplate) (contract.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tpl, ok := s.templates[id]
	if !ok {
		return contract.Template{}, contract.ErrNotFound
	}
	if input.Name != nil {
		tpl.Name = *input.Name
	}
	if input.Description != nil {
		tpl.Description = *input.Description
	}
	if input.Request != nil {
		tpl.Request = cloneRequest(*input.Request)
	}
	tpl.UpdatedAt = s.now()
	s.templates[id] = tpl
	return tpl, nil
}

func (s *Store) DeleteTemplate(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.templates[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.templates, id)
	return nil
}

func (s *Store) GetTemplate(ctx context.Context, id int) (contract.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tpl, ok := s.templates[id]
	if !ok {
		return contract.Template{}, contract.ErrNotFound
	}
	return tpl, nil
}

func (s *Store) ListTemplates(ctx context.Context) ([]contract.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Template, 0, len(s.templates))
	for _, tpl := range s.templates {
		out = append(out, tpl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) RecordRun(ctx context.Context, input contract.RecordRun) (contract.RunResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runSeq++
	run := contract.RunResult{
		ID:           s.runSeq,
		MonitorID:    input.MonitorID,
		RunID:        input.RunID,
		OK:           input.OK,
		CheckedCount: input.CheckedCount,
		OKCount:      input.OKCount,
		LatencyMS:    input.LatencyMS,
		Trigger:      input.Trigger,
		Results:      cloneResults(input.Results),
		CreatedAt:    s.now(),
	}
	s.runs[run.ID] = run
	return run, nil
}

func (s *Store) ListRuns(ctx context.Context, monitorID int, limit int) ([]contract.RunResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.RunResult, 0, len(s.runs))
	for _, run := range s.runs {
		if run.MonitorID == monitorID {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
