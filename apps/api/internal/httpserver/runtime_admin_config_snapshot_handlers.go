package httpserver

import (
	"context"
	"net/http"
	"time"

	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// snapshotConfigVersion identifies the config snapshot document shape so future
// importers can detect compatibility.
const snapshotConfigVersion = "1"

// snapshotSection lists a config collection and converts each item to its public
// shape, so the snapshot mirrors the per-resource admin list endpoints.
func snapshotSection[T any, R any](ctx context.Context, list func(context.Context) ([]T, error), conv func(T) R) ([]R, error) {
	items, err := list(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]R, 0, len(items))
	for _, item := range items {
		out = append(out, conv(item))
	}
	return out, nil
}

// handleAdminConfigSnapshot returns a single versioned, re-importable JSON
// snapshot of operator-managed configuration (providers, models, groups, rate
// limits, error-passthrough rules, TLS profiles, user attributes, plans, pricing
// rules, settings). It is read-only and excludes account credentials and
// operational data (usage/audit/snapshots), which have their own surfaces.
func (s *Server) handleAdminConfigSnapshot(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	ctx := r.Context()

	providers, err := snapshotSection(ctx, s.runtime.providers.List, toAPIProvider)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	models, err := snapshotSection(ctx, s.runtime.models.List, toAPIModel)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	groups, err := snapshotSection(ctx, s.runtime.accounts.ListGroups, toAPIAccountGroup)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	plans, err := snapshotSection(ctx, s.runtime.subscriptions.ListPlans, toAPISubscriptionPlan)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	pricingRules, err := snapshotSection(ctx, s.runtime.billing.ListPricingRules, toAPIPricingRule)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	modelRateLimits, err := s.snapshotModelRateLimits(ctx)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	groupRateLimits, err := s.snapshotGroupRateLimits(ctx)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	errorRules, err := snapshotSection(ctx, s.runtime.errorPassthrough.ListRules, toErrorPassthroughRulePayload)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	tlsProfiles, err := snapshotSection(ctx, s.runtime.tlsProfiles.ListProfiles, toTLSProfilePayload)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	userAttributes, err := snapshotSection(ctx, s.runtime.userAttributes.ListDefinitions, toUserAttributeDefinitionPayload)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(ctx)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"snapshot_version":           snapshotConfigVersion,
			"generated_at":               time.Now().UTC(),
			"providers":                  providers,
			"models":                     models,
			"account_groups":             groups,
			"subscription_plans":         plans,
			"pricing_rules":              pricingRules,
			"model_rate_limits":          modelRateLimits,
			"group_rate_limits":          groupRateLimits,
			"error_passthrough_rules":    errorRules,
			"tls_profiles":               tlsProfiles,
			"user_attribute_definitions": userAttributes,
			"settings":                   s.toAPIAdminSettings(settings),
		},
		"request_id": requestID,
	})
}

func (s *Server) writeConfigSnapshotError(w http.ResponseWriter, requestID string) {
	writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to assemble config snapshot", requestID)
}

// snapshotModelRateLimit / snapshotGroupRateLimit denormalize the model/group
// natural key (name) onto each rate limit so import can remap to the target
// environment's IDs (integer IDs do not port across environments).
type snapshotModelRateLimit struct {
	ModelID        int    `json:"model_id"`
	ModelName      string `json:"model_name"`
	RPMLimit       int    `json:"rpm_limit"`
	TPMLimit       int    `json:"tpm_limit"`
	MaxConcurrency int    `json:"max_concurrency"`
	Enabled        bool   `json:"enabled"`
}

type snapshotGroupRateLimit struct {
	GroupID        int    `json:"account_group_id"`
	GroupName      string `json:"account_group_name"`
	RPMLimit       int    `json:"rpm_limit"`
	TPMLimit       int    `json:"tpm_limit"`
	MaxConcurrency int    `json:"max_concurrency"`
	Enabled        bool   `json:"enabled"`
}

func (s *Server) snapshotModelRateLimits(ctx context.Context) ([]snapshotModelRateLimit, error) {
	limits, err := s.runtime.modelRateLimits.ListLimits(ctx)
	if err != nil {
		return nil, err
	}
	models, err := s.runtime.models.List(ctx)
	if err != nil {
		return nil, err
	}
	nameByID := make(map[int]string, len(models))
	for _, model := range models {
		nameByID[model.ID] = model.CanonicalName
	}
	out := make([]snapshotModelRateLimit, 0, len(limits))
	for _, limit := range limits {
		out = append(out, snapshotModelRateLimit{
			ModelID:        limit.ModelID,
			ModelName:      nameByID[limit.ModelID],
			RPMLimit:       limit.RPMLimit,
			TPMLimit:       limit.TPMLimit,
			MaxConcurrency: limit.MaxConcurrency,
			Enabled:        limit.Enabled,
		})
	}
	return out, nil
}

func (s *Server) snapshotGroupRateLimits(ctx context.Context) ([]snapshotGroupRateLimit, error) {
	limits, err := s.runtime.groupRateLimits.ListLimits(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.runtime.accounts.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	nameByID := make(map[int]string, len(groups))
	for _, group := range groups {
		nameByID[group.ID] = group.Name
	}
	out := make([]snapshotGroupRateLimit, 0, len(limits))
	for _, limit := range limits {
		out = append(out, snapshotGroupRateLimit{
			GroupID:        limit.GroupID,
			GroupName:      nameByID[limit.GroupID],
			RPMLimit:       limit.RPMLimit,
			TPMLimit:       limit.TPMLimit,
			MaxConcurrency: limit.MaxConcurrency,
			Enabled:        limit.Enabled,
		})
	}
	return out, nil
}
