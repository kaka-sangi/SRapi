package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
)

type Store struct {
	mu            sync.Mutex
	nextID        int
	nextPricingID int
	byID          map[int]contract.LedgerEntry
	pricingRules  map[int]contract.PricingRule
}

func New() *Store {
	return &Store{
		nextID:        1,
		nextPricingID: 1,
		byID:          map[int]contract.LedgerEntry{},
		pricingRules:  map[int]contract.PricingRule{},
	}
}

func (s *Store) Create(_ context.Context, input contract.LedgerEntry) (contract.LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := cloneEntry(input)
	entry.ID = s.nextID
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.byID[entry.ID] = entry
	s.nextID++
	return cloneEntry(entry), nil
}

func (s *Store) List(_ context.Context) ([]contract.LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.LedgerEntry, 0, len(s.byID))
	for _, entry := range s.byID {
		out = append(out, cloneEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListPage mirrors the ent store's WHERE + ORDER BY created_at DESC, id DESC +
// LIMIT/OFFSET path, but in memory. Used by unit tests and dev runs that don't
// stand up Postgres.
func (s *Store) ListPage(_ context.Context, filter contract.LedgerListFilter) (contract.LedgerListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]contract.LedgerEntry, 0, len(s.byID))
	refType := strings.TrimSpace(filter.ReferenceType)
	for _, entry := range s.byID {
		if filter.UserID != nil && entry.UserID != *filter.UserID {
			continue
		}
		if refType != "" && entry.ReferenceType != refType {
			continue
		}
		matched = append(matched, cloneEntry(entry))
	}
	// Newest first; tie-break by id desc to match the ent ordering.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})
	total := len(matched)
	if filter.Offset >= total {
		return contract.LedgerListResult{Items: []contract.LedgerEntry{}, Total: total}, nil
	}
	end := total
	if filter.Limit > 0 {
		end = filter.Offset + filter.Limit
		if end > total {
			end = total
		}
	}
	page := append([]contract.LedgerEntry(nil), matched[filter.Offset:end]...)
	return contract.LedgerListResult{Items: page, Total: total}, nil
}

func (s *Store) CreatePricingRule(_ context.Context, input contract.PricingRule) (contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	rule := clonePricingRule(input)
	rule.ID = s.nextPricingID
	rule.CreatedAt = now
	rule.UpdatedAt = now
	s.pricingRules[rule.ID] = rule
	s.nextPricingID++
	return clonePricingRule(rule), nil
}

func (s *Store) UpdatePricingRule(_ context.Context, id int, input contract.UpdatePricingRuleRequest) (contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := s.pricingRules[id]
	if !ok {
		return contract.PricingRule{}, contract.ErrNotFound
	}
	if input.BillingMode != nil {
		rule.BillingMode = *input.BillingMode
	}
	if input.InputPricePerMillionTokens != nil {
		rule.InputPricePerMillionTokens = *input.InputPricePerMillionTokens
	}
	if input.OutputPricePerMillionTokens != nil {
		rule.OutputPricePerMillionTokens = *input.OutputPricePerMillionTokens
	}
	if input.CacheReadPricePerMillionTokens != nil {
		rule.CacheReadPricePerMillionTokens = *input.CacheReadPricePerMillionTokens
	}
	if input.CacheWritePricePerMillionTokens != nil {
		rule.CacheWritePricePerMillionTokens = *input.CacheWritePricePerMillionTokens
	}
	if input.CacheWrite5mPricePerMillionTokens != nil {
		rule.CacheWrite5mPricePerMillionTokens = *input.CacheWrite5mPricePerMillionTokens
	}
	if input.CacheWrite1hPricePerMillionTokens != nil {
		rule.CacheWrite1hPricePerMillionTokens = *input.CacheWrite1hPricePerMillionTokens
	}
	if input.ImageOutputPricePerMillionTokens != nil {
		rule.ImageOutputPricePerMillionTokens = *input.ImageOutputPricePerMillionTokens
	}
	if input.PerRequestPrice != nil {
		rule.PerRequestPrice = *input.PerRequestPrice
	}
	if input.ServiceTierMultipliers != nil {
		rule.ServiceTierMultipliers = cloneStringMap(*input.ServiceTierMultipliers)
	}
	if input.LongContextThresholdTokens != nil {
		rule.LongContextThresholdTokens = cloneIntPtr(*input.LongContextThresholdTokens)
	}
	if input.LongContextMultiplier != nil {
		rule.LongContextMultiplier = *input.LongContextMultiplier
	}
	if input.Intervals != nil {
		rule.Intervals = clonePricingIntervals(*input.Intervals)
		for idx := range rule.Intervals {
			rule.Intervals[idx].PricingRuleID = rule.ID
		}
	}
	if input.Currency != nil {
		rule.Currency = *input.Currency
	}
	if input.EffectiveFrom != nil {
		rule.EffectiveFrom = cloneTime(*input.EffectiveFrom)
	}
	if input.EffectiveTo != nil {
		rule.EffectiveTo = cloneTime(*input.EffectiveTo)
	}
	rule.UpdatedAt = time.Now().UTC()
	s.pricingRules[id] = rule
	return clonePricingRule(rule), nil
}

func (s *Store) FindPricingRuleByID(_ context.Context, id int) (contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := s.pricingRules[id]
	if !ok {
		return contract.PricingRule{}, contract.ErrNotFound
	}
	return clonePricingRule(rule), nil
}

func (s *Store) DeletePricingRule(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pricingRules[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.pricingRules, id)
	return nil
}

func (s *Store) ListPricingRules(_ context.Context) ([]contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.PricingRule, 0, len(s.pricingRules))
	for _, rule := range s.pricingRules {
		out = append(out, clonePricingRule(rule))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) QueryPricingRules(_ context.Context, query contract.PricingRuleQuery) ([]contract.PricingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := query.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	out := make([]contract.PricingRule, 0)
	for _, rule := range s.pricingRules {
		if rule.ProviderID != query.ProviderID && rule.ProviderID != 0 {
			continue
		}
		if !pricingRuleActive(rule, at) {
			continue
		}
		if !pricingRuleMatchesQuery(rule, query) {
			continue
		}
		out = append(out, clonePricingRule(rule))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func pricingRuleActive(rule contract.PricingRule, at time.Time) bool {
	if rule.EffectiveFrom != nil && at.Before(*rule.EffectiveFrom) {
		return false
	}
	if rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo) {
		return false
	}
	return true
}

func pricingRuleMatchesQuery(rule contract.PricingRule, query contract.PricingRuleQuery) bool {
	if rule.ModelID == query.ModelID {
		return true
	}
	family := strings.ToLower(strings.TrimSpace(query.ModelFamily))
	if family != "" && pricingFamilyMatches(family, rule.ModelFamily) {
		return true
	}
	for _, name := range []string{query.RequestedModel, query.UpstreamModel} {
		if pricingFamilyMatches(strings.ToLower(strings.TrimSpace(name)), rule.ModelFamily) {
			return true
		}
	}
	return false
}

func pricingFamilyMatches(requestFamily string, ruleFamily string) bool {
	ruleFamily = strings.ToLower(strings.TrimSpace(ruleFamily))
	return requestFamily != "" && ruleFamily != "" && (requestFamily == ruleFamily || strings.Contains(requestFamily, ruleFamily) || strings.Contains(ruleFamily, requestFamily))
}

func cloneEntry(value contract.LedgerEntry) contract.LedgerEntry {
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func clonePricingRule(value contract.PricingRule) contract.PricingRule {
	value.EffectiveFrom = cloneTime(value.EffectiveFrom)
	value.EffectiveTo = cloneTime(value.EffectiveTo)
	value.ServiceTierMultipliers = cloneStringMap(value.ServiceTierMultipliers)
	value.LongContextThresholdTokens = cloneIntPtr(value.LongContextThresholdTokens)
	value.Intervals = clonePricingIntervals(value.Intervals)
	return value
}

func clonePricingIntervals(values []contract.PricingInterval) []contract.PricingInterval {
	if values == nil {
		return nil
	}
	out := make([]contract.PricingInterval, len(values))
	copy(out, values)
	for idx := range out {
		if out[idx].MaxTokens != nil {
			cloned := *out[idx].MaxTokens
			out[idx].MaxTokens = &cloned
		}
	}
	return out
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
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
