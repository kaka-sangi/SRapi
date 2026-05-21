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
