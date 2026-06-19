package operations

import (
	"context"

	"github.com/srapi/srapi/apps/api/ent"
	entobsalertrule "github.com/srapi/srapi/apps/api/ent/obsalertrule"
	entobsalertsilence "github.com/srapi/srapi/apps/api/ent/obsalertsilence"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

func (s *Store) CreateAlertRule(ctx context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	create := s.client.ObsAlertRule.Create().
		SetName(input.Name).
		SetMetricType(string(input.MetricType)).
		SetOperator(string(input.Operator)).
		SetThreshold(input.Threshold).
		SetSeverity(string(input.Severity)).
		SetEnabled(input.Enabled).
		SetWindowSeconds(input.WindowSeconds).
		SetCooldownSeconds(input.CooldownSeconds).
		SetMinRequestCount(input.MinRequestCount).
		SetScopeJSON(ruleScopeJSON(input.Scope))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.AlertRule{}, err
	}
	return toAlertRule(row), nil
}

func (s *Store) UpdateAlertRule(ctx context.Context, input contract.AlertRule) (contract.AlertRule, error) {
	update := s.client.ObsAlertRule.UpdateOneID(input.ID).
		SetName(input.Name).
		SetMetricType(string(input.MetricType)).
		SetOperator(string(input.Operator)).
		SetThreshold(input.Threshold).
		SetSeverity(string(input.Severity)).
		SetEnabled(input.Enabled).
		SetWindowSeconds(input.WindowSeconds).
		SetCooldownSeconds(input.CooldownSeconds).
		SetMinRequestCount(input.MinRequestCount).
		SetScopeJSON(ruleScopeJSON(input.Scope))
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.AlertRule{}, mapNotFound(err)
	}
	return toAlertRule(row), nil
}

func (s *Store) FindAlertRuleByID(ctx context.Context, id int) (contract.AlertRule, error) {
	row, err := s.client.ObsAlertRule.Get(ctx, id)
	if err != nil {
		return contract.AlertRule{}, mapNotFound(err)
	}
	return toAlertRule(row), nil
}

func (s *Store) ListAlertRules(ctx context.Context) ([]contract.AlertRule, error) {
	rows, err := s.client.ObsAlertRule.Query().
		Order(entobsalertrule.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AlertRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlertRule(row))
	}
	return out, nil
}

func (s *Store) DeleteAlertRule(ctx context.Context, id int) error {
	if err := s.client.ObsAlertRule.DeleteOneID(id).Exec(ctx); err != nil {
		return mapNotFound(err)
	}
	return nil
}

func (s *Store) CreateAlertSilence(ctx context.Context, input contract.AlertSilence) (contract.AlertSilence, error) {
	create := s.client.ObsAlertSilence.Create().
		SetComment(input.Comment).
		SetMatcherJSON(silenceMatcherJSON(input.Matcher)).
		SetStartsAt(input.StartsAt).
		SetEndsAt(input.EndsAt).
		SetNillableCreatedBy(input.CreatedBy)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.AlertSilence{}, err
	}
	return toAlertSilence(row), nil
}

func (s *Store) ListAlertSilences(ctx context.Context) ([]contract.AlertSilence, error) {
	rows, err := s.client.ObsAlertSilence.Query().
		Order(entobsalertsilence.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.AlertSilence, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlertSilence(row))
	}
	return out, nil
}

func (s *Store) DeleteAlertSilence(ctx context.Context, id int) error {
	if err := s.client.ObsAlertSilence.DeleteOneID(id).Exec(ctx); err != nil {
		return mapNotFound(err)
	}
	return nil
}

func toAlertRule(row *ent.ObsAlertRule) contract.AlertRule {
	return contract.AlertRule{
		ID:              row.ID,
		Name:            row.Name,
		MetricType:      contract.AlertMetricType(row.MetricType),
		Operator:        contract.AlertOperator(row.Operator),
		Threshold:       row.Threshold,
		Severity:        contract.AlertSeverity(row.Severity),
		Enabled:         row.Enabled,
		WindowSeconds:   row.WindowSeconds,
		CooldownSeconds: row.CooldownSeconds,
		MinRequestCount: row.MinRequestCount,
		Scope:           ruleScopeFromJSON(row.ScopeJSON),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func toAlertSilence(row *ent.ObsAlertSilence) contract.AlertSilence {
	return contract.AlertSilence{
		ID:        row.ID,
		Comment:   row.Comment,
		Matcher:   silenceMatcherFromJSON(row.MatcherJSON),
		StartsAt:  row.StartsAt,
		EndsAt:    row.EndsAt,
		CreatedBy: cloneInt(row.CreatedBy),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func ruleScopeJSON(scope contract.AlertRuleScope) map[string]any {
	out := map[string]any{
		"source_endpoint": scope.SourceEndpoint,
		"model":           scope.Model,
		"error_class":     scope.ErrorClass,
	}
	if scope.ProviderID != nil {
		out["provider_id"] = *scope.ProviderID
	}
	return out
}

func ruleScopeFromJSON(value map[string]any) contract.AlertRuleScope {
	scope := contract.AlertRuleScope{
		SourceEndpoint: stringFromMap(value, "source_endpoint"),
		Model:          stringFromMap(value, "model"),
		ErrorClass:     stringFromMap(value, "error_class"),
	}
	if providerID, ok := intFromMap(value, "provider_id"); ok {
		scope.ProviderID = &providerID
	}
	return scope
}

func silenceMatcherJSON(matcher contract.AlertSilenceMatcher) map[string]any {
	out := map[string]any{
		"rule_id":         matcher.RuleID,
		"severity":        string(matcher.Severity),
		"source_endpoint": matcher.SourceEndpoint,
		"model":           matcher.Model,
		"error_class":     matcher.ErrorClass,
	}
	if matcher.ProviderID != nil {
		out["provider_id"] = *matcher.ProviderID
	}
	return out
}

func silenceMatcherFromJSON(value map[string]any) contract.AlertSilenceMatcher {
	matcher := contract.AlertSilenceMatcher{
		RuleID:         stringFromMap(value, "rule_id"),
		Severity:       contract.AlertSeverity(stringFromMap(value, "severity")),
		SourceEndpoint: stringFromMap(value, "source_endpoint"),
		Model:          stringFromMap(value, "model"),
		ErrorClass:     stringFromMap(value, "error_class"),
	}
	if providerID, ok := intFromMap(value, "provider_id"); ok {
		matcher.ProviderID = &providerID
	}
	return matcher
}
