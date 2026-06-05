package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

type Store struct {
	mu            sync.Mutex
	nextSLOID     int
	nextAlertID   int
	nextRuleID    int
	nextSilenceID int
	slos          map[int]contract.SLODefinition
	alerts        map[int]contract.AlertEvent
	rules         map[int]contract.AlertRule
	silences      map[int]contract.AlertSilence
	usage         usagecontract.Store
}

func New() *Store {
	return NewWithUsageStore(nil)
}

func NewWithUsageStore(usage usagecontract.Store) *Store {
	return &Store{
		nextSLOID:     1,
		nextAlertID:   1,
		nextRuleID:    1,
		nextSilenceID: 1,
		slos:          map[int]contract.SLODefinition{},
		alerts:        map[int]contract.AlertEvent{},
		rules:         map[int]contract.AlertRule{},
		silences:      map[int]contract.AlertSilence{},
		usage:         usage,
	}
}

func (s *Store) Cleanup(context.Context, contract.RetentionCutoffs) (contract.CleanupResult, error) {
	return contract.CleanupResult{}, nil
}

func (s *Store) CreateSLO(_ context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := cloneSLO(input)
	item.ID = s.nextSLOID
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	s.slos[item.ID] = item
	s.nextSLOID++
	return cloneSLO(item), nil
}

func (s *Store) UpdateSLO(_ context.Context, input contract.SLODefinition) (contract.SLODefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.slos[input.ID]; !ok {
		return contract.SLODefinition{}, contract.ErrNotFound
	}
	item := cloneSLO(input)
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	s.slos[item.ID] = item
	return cloneSLO(item), nil
}

func (s *Store) FindSLOByID(_ context.Context, id int) (contract.SLODefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.slos[id]
	if !ok {
		return contract.SLODefinition{}, contract.ErrNotFound
	}
	return cloneSLO(item), nil
}

func (s *Store) ListSLOs(_ context.Context) ([]contract.SLODefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.SLODefinition, 0, len(s.slos))
	for _, item := range s.slos {
		out = append(out, cloneSLO(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) CreateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := cloneAlert(input)
	item.ID = s.nextAlertID
	if item.StartedAt.IsZero() {
		item.StartedAt = time.Now().UTC()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = item.StartedAt
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	s.alerts[item.ID] = item
	s.nextAlertID++
	return cloneAlert(item), nil
}

func (s *Store) UpdateAlert(_ context.Context, input contract.AlertEvent) (contract.AlertEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.alerts[input.ID]; !ok {
		return contract.AlertEvent{}, contract.ErrNotFound
	}
	item := cloneAlert(input)
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	s.alerts[item.ID] = item
	return cloneAlert(item), nil
}

func (s *Store) FindAlertByID(_ context.Context, id int) (contract.AlertEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.alerts[id]
	if !ok {
		return contract.AlertEvent{}, contract.ErrNotFound
	}
	return cloneAlert(item), nil
}

func (s *Store) ListAlerts(_ context.Context) ([]contract.AlertEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AlertEvent, 0, len(s.alerts))
	for _, item := range s.alerts {
		out = append(out, cloneAlert(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListUsageLogs(ctx context.Context) ([]usagecontract.UsageLog, error) {
	if s.usage == nil {
		return nil, nil
	}
	return s.usage.List(ctx)
}

func (s *Store) ListUsageLogsSince(ctx context.Context, since time.Time) ([]usagecontract.UsageLog, error) {
	logs, err := s.ListUsageLogs(ctx)
	if err != nil || since.IsZero() {
		return logs, err
	}
	out := make([]usagecontract.UsageLog, 0, len(logs))
	for _, log := range logs {
		if !log.CreatedAt.Before(since) {
			out = append(out, log)
		}
	}
	return out, nil
}

func (s *Store) CreateAlertRule(_ context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := cloneRule(input)
	item.ID = s.nextRuleID
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	s.rules[item.ID] = item
	s.nextRuleID++
	return cloneRule(item), nil
}

func (s *Store) UpdateAlertRule(_ context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rules[input.ID]; !ok {
		return contract.AlertRule{}, contract.ErrNotFound
	}
	item := cloneRule(input)
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	s.rules[item.ID] = item
	return cloneRule(item), nil
}

func (s *Store) FindAlertRuleByID(_ context.Context, id int) (contract.AlertRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.rules[id]
	if !ok {
		return contract.AlertRule{}, contract.ErrNotFound
	}
	return cloneRule(item), nil
}

func (s *Store) ListAlertRules(_ context.Context) ([]contract.AlertRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AlertRule, 0, len(s.rules))
	for _, item := range s.rules {
		out = append(out, cloneRule(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) DeleteAlertRule(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rules[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.rules, id)
	return nil
}

func (s *Store) CreateAlertSilence(_ context.Context, input contract.AlertSilence) (contract.AlertSilence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := cloneSilence(input)
	item.ID = s.nextSilenceID
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	s.silences[item.ID] = item
	s.nextSilenceID++
	return cloneSilence(item), nil
}

func (s *Store) ListAlertSilences(_ context.Context) ([]contract.AlertSilence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.AlertSilence, 0, len(s.silences))
	for _, item := range s.silences {
		out = append(out, cloneSilence(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) DeleteAlertSilence(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.silences[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.silences, id)
	return nil
}

func cloneRule(value contract.AlertRule) contract.AlertRule {
	value.Scope.ProviderID = cloneInt(value.Scope.ProviderID)
	return value
}

func cloneSilence(value contract.AlertSilence) contract.AlertSilence {
	value.Matcher.ProviderID = cloneInt(value.Matcher.ProviderID)
	value.CreatedBy = cloneInt(value.CreatedBy)
	return value
}

func cloneSLO(value contract.SLODefinition) contract.SLODefinition {
	if value.Filter.ProviderID != nil {
		providerID := *value.Filter.ProviderID
		value.Filter.ProviderID = &providerID
	}
	value.Filter.ErrorOwnerExclude = cloneStrings(value.Filter.ErrorOwnerExclude)
	value.AlertPolicy.Thresholds = append([]contract.BurnRateThreshold(nil), value.AlertPolicy.Thresholds...)
	return value
}

func cloneAlert(value contract.AlertEvent) contract.AlertEvent {
	value.SLOID = cloneInt(value.SLOID)
	value.Details = cloneMap(value.Details)
	value.ResolvedAt = cloneTime(value.ResolvedAt)
	value.AcknowledgedAt = cloneTime(value.AcknowledgedAt)
	value.AcknowledgedBy = cloneInt(value.AcknowledgedBy)
	value.SuppressedBy = cloneString(value.SuppressedBy)
	return value
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
	if err := json.Unmarshal(raw, &out); err != nil {
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

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
