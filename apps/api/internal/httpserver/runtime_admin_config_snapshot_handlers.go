package httpserver

import (
	"context"
	"net/http"
	"time"

	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
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
// limits, scheduled tests, payload rules, error-passthrough rules, TLS profiles,
// user attributes, plans, pricing rules, settings). It is read-only and excludes account
// credentials and operational data (usage/audit/snapshots), which have their
// own surfaces.
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
	payloadRules, err := snapshotSection(ctx, s.runtime.payloadRules.ListRules, toImportPayloadRule)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	proxies, err := s.snapshotProxies(ctx)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	scheduledTestPlans, err := s.snapshotScheduledTestPlans(ctx)
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
	errorRules, err := snapshotSection(ctx, s.runtime.errorPassthrough.ListRules, toAPIErrorPassthroughRule)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	tlsProfiles, err := snapshotSection(ctx, s.runtime.tlsProfiles.ListProfiles, toAPITLSProfile)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	userAttributes, err := snapshotSection(ctx, s.runtime.userAttributes.ListDefinitions, toAPIUserAttributeDefinition)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(ctx)
	if err != nil {
		s.writeConfigSnapshotError(w, requestID)
		return
	}

	writeJSONAny(w, http.StatusOK, apiopenapi.ConfigSnapshotResponse{
		Data: struct {
			AccountGroups            *[]apiopenapi.AccountGroup             `json:"account_groups,omitempty"`
			ErrorPassthroughRules    *[]apiopenapi.ErrorPassthroughRule     `json:"error_passthrough_rules,omitempty"`
			GeneratedAt              time.Time                              `json:"generated_at"`
			GroupRateLimits          *[]apiopenapi.SnapshotGroupRateLimit   `json:"group_rate_limits,omitempty"`
			ModelRateLimits          *[]apiopenapi.SnapshotModelRateLimit   `json:"model_rate_limits,omitempty"`
			Models                   *[]apiopenapi.Model                    `json:"models,omitempty"`
			PayloadRules             *[]apiopenapi.CreatePayloadRuleRequest `json:"payload_rules,omitempty"`
			PricingRules             *[]apiopenapi.PricingRule              `json:"pricing_rules,omitempty"`
			Providers                *[]apiopenapi.Provider                 `json:"providers,omitempty"`
			Proxies                  *[]apiopenapi.SnapshotProxyDefinition  `json:"proxies,omitempty"`
			ScheduledTestPlans       *[]apiopenapi.ImportScheduledTestPlan  `json:"scheduled_test_plans,omitempty"`
			Settings                 *apiopenapi.AdminSettings              `json:"settings,omitempty"`
			SnapshotVersion          string                                 `json:"snapshot_version"`
			SubscriptionPlans        *[]apiopenapi.SubscriptionPlan         `json:"subscription_plans,omitempty"`
			TlsProfiles              *[]apiopenapi.TLSProfile               `json:"tls_profiles,omitempty"`
			UserAttributeDefinitions *[]apiopenapi.UserAttributeDefinition  `json:"user_attribute_definitions,omitempty"`
		}{
			SnapshotVersion:          snapshotConfigVersion,
			GeneratedAt:              time.Now().UTC(),
			Providers:                &providers,
			Models:                   &models,
			AccountGroups:            &groups,
			SubscriptionPlans:        &plans,
			PricingRules:             &pricingRules,
			PayloadRules:             &payloadRules,
			Proxies:                  &proxies,
			ScheduledTestPlans:       &scheduledTestPlans,
			ModelRateLimits:          &modelRateLimits,
			GroupRateLimits:          &groupRateLimits,
			ErrorPassthroughRules:    &errorRules,
			TlsProfiles:              &tlsProfiles,
			UserAttributeDefinitions: &userAttributes,
			Settings:                 ptrAPIAdminSettings(s.toAPIAdminSettings(settings)),
		},
		RequestId: requestID,
	})
}

func (s *Server) snapshotProxies(ctx context.Context) ([]apiopenapi.SnapshotProxyDefinition, error) {
	items, err := s.runtime.accounts.ListProxies(ctx)
	if err != nil {
		return nil, err
	}
	namesByID := make(map[int]string, len(items))
	for _, proxy := range items {
		namesByID[proxy.ID] = proxy.Name
	}
	out := make([]apiopenapi.SnapshotProxyDefinition, 0, len(items))
	for _, proxy := range items {
		out = append(out, toSnapshotProxyDefinition(proxy, namesByID))
	}
	return out, nil
}

func (s *Server) writeConfigSnapshotError(w http.ResponseWriter, requestID string) {
	writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to assemble config snapshot", requestID)
}

func (s *Server) snapshotScheduledTestPlans(ctx context.Context) ([]apiopenapi.ImportScheduledTestPlan, error) {
	plans, err := s.runtime.scheduledTests.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.runtime.accounts.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	providers, err := s.runtime.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	providerNameByID := make(map[int]string, len(providers))
	for _, provider := range providers {
		providerNameByID[provider.ID] = provider.Name
	}
	accountByID := make(map[int]struct {
		name         string
		providerName string
	}, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = struct {
			name         string
			providerName string
		}{
			name:         account.Name,
			providerName: providerNameByID[account.ProviderID],
		}
	}
	groupNameByID := make(map[int]string, len(groups))
	for _, group := range groups {
		groupNameByID[group.ID] = group.Name
	}

	out := make([]apiopenapi.ImportScheduledTestPlan, 0, len(plans))
	for _, plan := range plans {
		item := apiopenapi.ImportScheduledTestPlan{
			Name:            plan.Name,
			Enabled:         &plan.Enabled,
			ScopeType:       apiopenapi.ImportScheduledTestPlanScopeType(plan.ScopeType),
			IntervalSeconds: int64Ptr(plan.IntervalSeconds),
			CronExpression:  stringPtr(plan.CronExpression),
			ProbeModel:      stringPtr(plan.ProbeModel),
			MaxResults:      int64Ptr(plan.MaxResults),
			AutoRecover:     &plan.AutoRecover,
		}
		switch plan.ScopeType {
		case scheduledcontract.ScopeAccount:
			if plan.ScopeID != nil {
				if account, ok := accountByID[*plan.ScopeID]; ok {
					item.ScopeAccountName = optionalNonEmptyStringPtr(account.name)
					item.ScopeAccountProviderName = optionalNonEmptyStringPtr(account.providerName)
				}
			}
		case scheduledcontract.ScopeGroup:
			if plan.ScopeID != nil {
				item.ScopeGroupName = optionalNonEmptyStringPtr(groupNameByID[*plan.ScopeID])
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// snapshotModelRateLimit / snapshotGroupRateLimit denormalize the model/group
// natural key (name) onto each rate limit so import can remap to the target
// environment's IDs (integer IDs do not port across environments).
func (s *Server) snapshotModelRateLimits(ctx context.Context) ([]apiopenapi.SnapshotModelRateLimit, error) {
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
	out := make([]apiopenapi.SnapshotModelRateLimit, 0, len(limits))
	for _, limit := range limits {
		out = append(out, apiopenapi.SnapshotModelRateLimit{
			ModelId:        int64(limit.ModelID),
			ModelName:      nameByID[limit.ModelID],
			RpmLimit:       int64(limit.RPMLimit),
			TpmLimit:       int64(limit.TPMLimit),
			MaxConcurrency: int64(limit.MaxConcurrency),
			Enabled:        limit.Enabled,
		})
	}
	return out, nil
}

func (s *Server) snapshotGroupRateLimits(ctx context.Context) ([]apiopenapi.SnapshotGroupRateLimit, error) {
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
	out := make([]apiopenapi.SnapshotGroupRateLimit, 0, len(limits))
	for _, limit := range limits {
		out = append(out, apiopenapi.SnapshotGroupRateLimit{
			AccountGroupId:   int64(limit.GroupID),
			AccountGroupName: nameByID[limit.GroupID],
			RpmLimit:         int64(limit.RPMLimit),
			TpmLimit:         int64(limit.TPMLimit),
			MaxConcurrency:   int64(limit.MaxConcurrency),
			Enabled:          limit.Enabled,
		})
	}
	return out, nil
}
