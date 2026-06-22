package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store contract.Store
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Record(ctx context.Context, req contract.RecordRequest) (contract.Log, error) {
	action := strings.TrimSpace(req.Action)
	resourceType := strings.TrimSpace(req.ResourceType)
	if action == "" || resourceType == "" {
		return contract.Log{}, ErrInvalidInput
	}
	return s.store.Create(ctx, contract.Log{
		ActorUserID:  req.ActorUserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   strings.TrimSpace(req.ResourceID),
		Before:       cloneMap(req.Before),
		After:        cloneMap(req.After),
		IP:           strings.TrimSpace(req.IP),
		UserAgent:    strings.TrimSpace(req.UserAgent),
		TraceID:      strings.TrimSpace(req.TraceID),
		CreatedAt:    s.clock.Now(),
	})
}

func (s *Service) List(ctx context.Context) ([]contract.Log, error) {
	return s.store.List(ctx)
}

// ListPage delegates to PageReader when supported so admin/audit reads do
// not materialize the entire table. Falls back to List + in-memory filter for
// store implementations (mostly test doubles) that omit the capability.
func (s *Service) ListPage(ctx context.Context, filter contract.ListFilter, limit, offset int) (contract.ListPageResult, error) {
	if reader, ok := s.store.(contract.PageReader); ok {
		return reader.ListPage(ctx, filter, limit, offset)
	}
	all, err := s.store.List(ctx)
	if err != nil {
		return contract.ListPageResult{}, err
	}
	matched := make([]contract.Log, 0, len(all))
	for _, log := range all {
		if !auditPageMatchesFallback(log, filter) {
			continue
		}
		matched = append(matched, log)
	}
	// Newest-first by id.
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return contract.ListPageResult{Items: []contract.Log{}, Total: total}, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return contract.ListPageResult{Items: matched[offset:end], Total: total}, nil
}

func auditPageMatchesFallback(log contract.Log, filter contract.ListFilter) bool {
	if action := strings.TrimSpace(filter.Action); action != "" && log.Action != action {
		return false
	}
	if resourceType := strings.TrimSpace(filter.ResourceType); resourceType != "" && log.ResourceType != resourceType {
		return false
	}
	if filter.ActorUserID != nil {
		if log.ActorUserID == nil || *log.ActorUserID != *filter.ActorUserID {
			return false
		}
	}
	if filter.Since != nil && log.CreatedAt.Before(filter.Since.UTC()) {
		return false
	}
	return true
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}
