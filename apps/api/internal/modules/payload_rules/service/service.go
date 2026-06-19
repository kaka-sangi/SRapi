package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/ttlcache"
	"github.com/srapi/srapi/apps/api/internal/platform/glob"
)

// ErrInvalidInput is returned for malformed rules.
var ErrInvalidInput = errors.New("invalid payload rule")

// resolveCacheTTL bounds staleness of the enabled-rule snapshot Resolve serves
// from. Resolve runs on every gateway dispatch, so it must not re-read and
// re-sort the rule table per request. Same-instance rule writes invalidate
// immediately; cross-instance writes converge within the TTL.
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
	input.MatchModel = normalizeModel(input.MatchModel)
	input.MatchProtocol = strings.TrimSpace(input.MatchProtocol)
	input.Params = cleanParams(input.Params)
	if len(input.Params) == 0 {
		return contract.Rule{}, ErrInvalidInput
	}
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

// Resolve flattens every enabled rule that matches the upstream model + protocol
// into an ordered list of transforms (priority asc, then id). Returns nil when
// nothing matches, so the gateway leaves the body untouched. Rules are served
// from a short-TTL snapshot so the per-request cost is pure in-memory matching.
func (s *Service) Resolve(ctx context.Context, model, protocol string) []contract.ResolvedTransform {
	enabled, err := s.enabledRules.Get(ctx, s.loadEnabledRules)
	if err != nil {
		return nil
	}
	var out []contract.ResolvedTransform
	for _, rule := range enabled {
		if !glob.Match(rule.MatchModel, model) {
			continue
		}
		if !protocolMatch(rule.MatchProtocol, protocol) {
			continue
		}
		for _, path := range sortedParamPaths(rule.Params) {
			value := rule.Params[path]
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
	rule.Params = cleanParams(rule.Params)
	return rule
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

func sortedParamPaths(params map[string]any) []string {
	paths := make([]string, 0, len(params))
	for path := range params {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func protocolMatch(rule, actual string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true
	}
	return strings.EqualFold(rule, strings.TrimSpace(actual))
}
