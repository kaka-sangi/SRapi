package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/ttlcache"
)

// ErrInvalidInput is returned for malformed rules.
var ErrInvalidInput = errors.New("invalid error passthrough rule")

// resolveCacheTTL bounds staleness of the enabled-rule snapshot Resolve serves
// from; Resolve runs on the gateway error path for every upstream failure.
// Same-instance rule writes invalidate immediately; cross-instance writes
// converge within the TTL.
const resolveCacheTTL = 3 * time.Second

type Service struct {
	store contract.Store
	// enabledRules caches the enabled rules pre-sorted by (priority, id). The
	// cached slice is shared across requests and must be treated as read-only.
	enabledRules *ttlcache.Value[[]contract.Rule]
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{
		store:        store,
		enabledRules: ttlcache.New[[]contract.Rule](resolveCacheTTL, nil),
	}, nil
}

func (s *Service) ListRules(ctx context.Context) ([]contract.Rule, error) {
	return s.store.ListRules(ctx)
}

func (s *Service) CreateRule(ctx context.Context, input contract.CreateRule) (contract.Rule, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Rule{}, ErrInvalidInput
	}
	input.Action = normalizeAction(input.Action)
	if input.Action == "" {
		return contract.Rule{}, ErrInvalidInput
	}
	input.Classes = cleanStrings(input.Classes)
	input.Keywords = cleanStrings(input.Keywords)
	statusCodes, ok := cleanStatusCodes(input.StatusCodes)
	if !ok {
		return contract.Rule{}, ErrInvalidInput
	}
	input.StatusCodes = statusCodes
	if input.ResponseStatus != nil {
		status, ok := cleanResponseStatus(input.ResponseStatus)
		if !ok {
			return contract.Rule{}, ErrInvalidInput
		}
		input.ResponseStatus = status
	}
	input.CustomMessage = cleanCustomMessage(input.CustomMessage)
	rule, err := s.store.CreateRule(ctx, input)
	if err == nil {
		s.enabledRules.Invalidate()
	}
	return rule, err
}

func (s *Service) UpdateRule(ctx context.Context, id int, input contract.UpdateRule) (contract.Rule, error) {
	if id <= 0 {
		return contract.Rule{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Rule{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Action != nil {
		action := normalizeAction(*input.Action)
		if action == "" {
			return contract.Rule{}, ErrInvalidInput
		}
		input.Action = &action
	}
	if input.Classes != nil {
		classes := cleanStrings(*input.Classes)
		input.Classes = &classes
	}
	if input.Keywords != nil {
		keywords := cleanStrings(*input.Keywords)
		input.Keywords = &keywords
	}
	if input.StatusCodes != nil {
		codes, ok := cleanStatusCodes(*input.StatusCodes)
		if !ok {
			return contract.Rule{}, ErrInvalidInput
		}
		input.StatusCodes = &codes
	}
	if input.ResponseStatus != nil {
		status, ok := cleanResponseStatus(*input.ResponseStatus)
		if !ok {
			return contract.Rule{}, ErrInvalidInput
		}
		input.ResponseStatus = &status
	}
	if input.CustomMessage != nil {
		message := cleanCustomMessage(*input.CustomMessage)
		input.CustomMessage = &message
	}
	rule, err := s.store.UpdateRule(ctx, id, input)
	if err == nil {
		s.enabledRules.Invalidate()
	}
	return rule, err
}

func (s *Service) DeleteRule(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	err := s.store.DeleteRule(ctx, id)
	if err == nil {
		s.enabledRules.Invalidate()
	}
	return err
}

// Resolve evaluates enabled rules in priority order and returns the first
// matching rule's response policy. matched is false when no rule applies, in
// which case the caller should fall back to its existing per-account behavior.
// Rules are served from a short-TTL snapshot so the per-failure cost is pure
// in-memory matching.
func (s *Service) Resolve(ctx context.Context, errorClass string, upstreamStatus int, message string) (contract.Resolution, bool) {
	enabled, err := s.enabledRules.Get(ctx, s.loadEnabledRules)
	if err != nil {
		return contract.Resolution{}, false
	}
	lowerMessage := strings.ToLower(message)
	for _, rule := range enabled {
		if !statusMatches(rule.StatusCodes, upstreamStatus) {
			continue
		}
		if !classMatches(rule.Classes, errorClass) {
			continue
		}
		if !keywordMatches(rule.Keywords, lowerMessage) {
			continue
		}
		return contract.Resolution{
			Action:         rule.Action,
			ResponseStatus: cloneIntPtr(rule.ResponseStatus),
			CustomMessage:  rule.CustomMessage,
		}, true
	}
	return contract.Resolution{}, false
}

// loadEnabledRules snapshots the enabled rules pre-sorted by (priority, id).
func (s *Service) loadEnabledRules(ctx context.Context) ([]contract.Rule, error) {
	rules, err := s.store.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]contract.Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, cloneRule(rule))
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		if enabled[i].Priority != enabled[j].Priority {
			return enabled[i].Priority < enabled[j].Priority
		}
		return enabled[i].ID < enabled[j].ID
	})
	return enabled, nil
}

func cloneRule(rule contract.Rule) contract.Rule {
	rule.StatusCodes = append([]int(nil), rule.StatusCodes...)
	rule.Classes = append([]string(nil), rule.Classes...)
	rule.Keywords = append([]string(nil), rule.Keywords...)
	rule.ResponseStatus = cloneIntPtr(rule.ResponseStatus)
	return rule
}

func statusMatches(codes []int, status int) bool {
	if len(codes) == 0 {
		return true
	}
	for _, code := range codes {
		if code == status {
			return true
		}
	}
	return false
}

func classMatches(classes []string, errorClass string) bool {
	if len(classes) == 0 {
		return true
	}
	for _, class := range classes {
		if strings.EqualFold(class, errorClass) {
			return true
		}
	}
	return false
}

func keywordMatches(keywords []string, lowerMessage string) bool {
	if len(keywords) == 0 {
		return true
	}
	for _, keyword := range keywords {
		if strings.Contains(lowerMessage, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func normalizeAction(action contract.Action) contract.Action {
	switch contract.Action(strings.ToLower(strings.TrimSpace(string(action)))) {
	case contract.ActionExpose:
		return contract.ActionExpose
	case contract.ActionMask:
		return contract.ActionMask
	default:
		return ""
	}
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cleanStatusCodes(codes []int) ([]int, bool) {
	out := make([]int, 0, len(codes))
	seen := map[int]struct{}{}
	for _, code := range codes {
		if code < 100 || code > 599 {
			return nil, false
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out, true
}

func cleanResponseStatus(status *int) (*int, bool) {
	if status == nil || *status == 0 {
		return nil, true
	}
	if *status < 100 || *status > 599 {
		return nil, false
	}
	cleaned := *status
	return &cleaned, true
}

func cleanCustomMessage(message string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
