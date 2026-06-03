package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
)

// ErrInvalidInput is returned for malformed rules.
var ErrInvalidInput = errors.New("invalid payload rule")

type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
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
	input.MatchModel = normalizeModel(input.MatchModel)
	input.MatchProtocol = strings.TrimSpace(input.MatchProtocol)
	input.Params = cleanParams(input.Params)
	if len(input.Params) == 0 {
		return contract.Rule{}, ErrInvalidInput
	}
	return s.store.CreateRule(ctx, input)
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
	if input.MatchModel != nil {
		model := normalizeModel(*input.MatchModel)
		input.MatchModel = &model
	}
	if input.MatchProtocol != nil {
		protocol := strings.TrimSpace(*input.MatchProtocol)
		input.MatchProtocol = &protocol
	}
	if input.Params != nil {
		params := cleanParams(*input.Params)
		if len(params) == 0 {
			return contract.Rule{}, ErrInvalidInput
		}
		input.Params = &params
	}
	return s.store.UpdateRule(ctx, id, input)
}

func (s *Service) DeleteRule(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteRule(ctx, id)
}

// Resolve flattens every enabled rule that matches the upstream model + protocol
// into an ordered list of transforms (priority asc, then id). Returns nil when
// nothing matches, so the gateway leaves the body untouched.
func (s *Service) Resolve(ctx context.Context, model, protocol string) []contract.ResolvedTransform {
	rules, err := s.store.ListRules(ctx)
	if err != nil {
		return nil
	}
	enabled := make([]contract.Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		if enabled[i].Priority != enabled[j].Priority {
			return enabled[i].Priority < enabled[j].Priority
		}
		return enabled[i].ID < enabled[j].ID
	})
	var out []contract.ResolvedTransform
	for _, rule := range enabled {
		if !globMatch(rule.MatchModel, model) {
			continue
		}
		if !protocolMatch(rule.MatchProtocol, protocol) {
			continue
		}
		for path, value := range rule.Params {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			op := contract.ResolvedTransform{Action: string(rule.Action), Path: path}
			if rule.Action != contract.ActionFilter {
				op.Value = value
			}
			out = append(out, op)
		}
	}
	return out
}

func normalizeAction(action contract.Action) contract.Action {
	switch contract.Action(strings.ToLower(strings.TrimSpace(string(action)))) {
	case contract.ActionDefault:
		return contract.ActionDefault
	case contract.ActionOverride:
		return contract.ActionOverride
	case contract.ActionFilter:
		return contract.ActionFilter
	default:
		return ""
	}
}

func normalizeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "*"
	}
	return model
}

func cleanParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for key, value := range params {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func protocolMatch(rule, actual string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true
	}
	return strings.EqualFold(rule, strings.TrimSpace(actual))
}

// globMatch supports "*" wildcards: exact, "prefix*", "*suffix", "*contains*",
// and "*" (match-any). Case-insensitive.
func globMatch(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" || pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(value[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && idx != 0 {
			return false // anchored prefix (pattern does not start with '*')
		}
		pos += idx + len(part)
	}
	if last := parts[len(parts)-1]; last != "" && !strings.HasSuffix(value, last) {
		return false // anchored suffix (pattern does not end with '*')
	}
	return true
}
