package httpserver

import (
	"context"
	"net/http"
	"sort"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	errorpassthroughservice "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/service"
	groupratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
	groupratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/service"
	modelratelimitscontract "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
	modelratelimitsservice "github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/service"
	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	payloadrulesservice "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/service"
	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	userattributescontract "github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
	userattributesservice "github.com/srapi/srapi/apps/api/internal/modules/userattributes/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// handleAdminConfigImport applies the importable sections of a config snapshot by
// natural-key upsert (create when the key is new, update otherwise). With
// ?dry_run=true it reports the create/update counts without writing.
func (s *Server) handleAdminConfigImport(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	dryRunPtr, err := parseBoolQuery(r.URL.Query().Get("dry_run"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid dry_run parameter", requestID)
		return
	}
	dryRun := dryRunPtr != nil && *dryRunPtr
	var body apiopenapi.ConfigImportRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid config import request", requestID)
		return
	}
	ctx := r.Context()

	tlsResult, err := s.importTLSProfiles(ctx, optionalSlice(body.TlsProfiles), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	attrResult, err := s.importUserAttributeDefinitions(ctx, optionalSlice(body.UserAttributeDefinitions), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	ruleResult, err := s.importErrorPassthroughRules(ctx, optionalSlice(body.ErrorPassthroughRules), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	payloadRuleResult, err := s.importPayloadRules(ctx, optionalSlice(body.PayloadRules), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	proxyResult, err := s.importProxies(ctx, optionalSlice(body.Proxies), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	scheduledTestPlanResult, err := s.importScheduledTestPlans(ctx, optionalSlice(body.ScheduledTestPlans), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	modelLimitResult, err := s.importModelRateLimits(ctx, optionalSlice(body.ModelRateLimits), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	groupLimitResult, err := s.importGroupRateLimits(ctx, optionalSlice(body.GroupRateLimits), dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}

	data := struct {
		DryRun                   bool                           `json:"dry_run"`
		ErrorPassthroughRules    apiopenapi.ImportSectionResult `json:"error_passthrough_rules"`
		GroupRateLimits          apiopenapi.ImportRemapResult   `json:"group_rate_limits"`
		ModelRateLimits          apiopenapi.ImportRemapResult   `json:"model_rate_limits"`
		PayloadRules             apiopenapi.ImportSectionResult `json:"payload_rules"`
		Proxies                  apiopenapi.ImportRemapResult   `json:"proxies"`
		ScheduledTestPlans       apiopenapi.ImportRemapResult   `json:"scheduled_test_plans"`
		TlsProfiles              apiopenapi.ImportSectionResult `json:"tls_profiles"`
		UserAttributeDefinitions apiopenapi.ImportSectionResult `json:"user_attribute_definitions"`
	}{
		DryRun:                   dryRun,
		TlsProfiles:              tlsResult,
		UserAttributeDefinitions: attrResult,
		ErrorPassthroughRules:    ruleResult,
		PayloadRules:             payloadRuleResult,
		Proxies:                  proxyResult,
		ScheduledTestPlans:       scheduledTestPlanResult,
		ModelRateLimits:          modelLimitResult,
		GroupRateLimits:          groupLimitResult,
	}
	if !dryRun {
		s.runtime.recordAudit(ctx, auditRecordFromRequest(r, session.User.ID, "config_snapshot.import", "config_snapshot", "import", nil, map[string]any{
			"dry_run":                    data.DryRun,
			"tls_profiles":               data.TlsProfiles,
			"user_attribute_definitions": data.UserAttributeDefinitions,
			"error_passthrough_rules":    data.ErrorPassthroughRules,
			"payload_rules":              data.PayloadRules,
			"proxies":                    data.Proxies,
			"scheduled_test_plans":       data.ScheduledTestPlans,
			"model_rate_limits":          data.ModelRateLimits,
			"group_rate_limits":          data.GroupRateLimits,
		}))
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.ConfigImportResponse{
		Data:      data,
		RequestId: requestID,
	})
}

func (s *Server) importModelRateLimits(ctx context.Context, items []apiopenapi.ImportModelRateLimit, dryRun bool) (apiopenapi.ImportRemapResult, error) {
	var result apiopenapi.ImportRemapResult
	if len(items) == 0 {
		return result, nil
	}
	models, err := s.runtime.models.List(ctx)
	if err != nil {
		return result, err
	}
	idByName := make(map[string]int, len(models))
	for _, model := range models {
		idByName[model.CanonicalName] = model.ID
	}
	existing, err := s.runtime.modelRateLimits.ListLimits(ctx)
	if err != nil {
		return result, err
	}
	hasLimit := make(map[int]struct{}, len(existing))
	for _, limit := range existing {
		hasLimit[limit.ModelID] = struct{}{}
	}
	for _, item := range items {
		modelID, ok := idByName[item.ModelName]
		if !ok {
			result.Skipped++
			continue
		}
		if _, found := hasLimit[modelID]; found {
			result.Updated++
		} else {
			result.Created++
		}
		if dryRun {
			continue
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		rpmLimit, ok := nonNegativeIntFromInt64Ptr(item.RpmLimit)
		if !ok {
			return result, modelratelimitsservice.ErrInvalidInput
		}
		tpmLimit, ok := nonNegativeIntFromInt64Ptr(item.TpmLimit)
		if !ok {
			return result, modelratelimitsservice.ErrInvalidInput
		}
		maxConcurrency, ok := nonNegativeIntFromInt64Ptr(item.MaxConcurrency)
		if !ok {
			return result, modelratelimitsservice.ErrInvalidInput
		}
		if _, err := s.runtime.modelRateLimits.UpsertLimit(ctx, modelratelimitscontract.UpsertLimit{
			ModelID:        modelID,
			RPMLimit:       rpmLimit,
			TPMLimit:       tpmLimit,
			MaxConcurrency: maxConcurrency,
			Enabled:        enabled,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importGroupRateLimits(ctx context.Context, items []apiopenapi.ImportGroupRateLimit, dryRun bool) (apiopenapi.ImportRemapResult, error) {
	var result apiopenapi.ImportRemapResult
	if len(items) == 0 {
		return result, nil
	}
	groups, err := s.runtime.accounts.ListGroups(ctx)
	if err != nil {
		return result, err
	}
	idByName := make(map[string]int, len(groups))
	for _, group := range groups {
		idByName[group.Name] = group.ID
	}
	existing, err := s.runtime.groupRateLimits.ListLimits(ctx)
	if err != nil {
		return result, err
	}
	hasLimit := make(map[int]struct{}, len(existing))
	for _, limit := range existing {
		hasLimit[limit.GroupID] = struct{}{}
	}
	for _, item := range items {
		groupID, ok := idByName[item.AccountGroupName]
		if !ok {
			result.Skipped++
			continue
		}
		if _, found := hasLimit[groupID]; found {
			result.Updated++
		} else {
			result.Created++
		}
		if dryRun {
			continue
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		rpmLimit, ok := nonNegativeIntFromInt64Ptr(item.RpmLimit)
		if !ok {
			return result, groupratelimitsservice.ErrInvalidInput
		}
		tpmLimit, ok := nonNegativeIntFromInt64Ptr(item.TpmLimit)
		if !ok {
			return result, groupratelimitsservice.ErrInvalidInput
		}
		maxConcurrency, ok := nonNegativeIntFromInt64Ptr(item.MaxConcurrency)
		if !ok {
			return result, groupratelimitsservice.ErrInvalidInput
		}
		if _, err := s.runtime.groupRateLimits.UpsertLimit(ctx, groupratelimitscontract.UpsertLimit{
			GroupID:        groupID,
			RPMLimit:       rpmLimit,
			TPMLimit:       tpmLimit,
			MaxConcurrency: maxConcurrency,
			Enabled:        enabled,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importPayloadRules(ctx context.Context, items []apiopenapi.CreatePayloadRuleRequest, dryRun bool) (apiopenapi.ImportSectionResult, error) {
	var result apiopenapi.ImportSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.payloadRules.ListRules(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]payloadrulescontract.Rule, len(existing))
	for _, rule := range existing {
		byName[rule.Name] = rule
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		priorityPtr, ok := intPtrFromInt64Ptr(item.Priority)
		if !ok {
			return result, payloadrulesservice.ErrInvalidInput
		}
		priority := 0
		if priorityPtr != nil {
			priority = *priorityPtr
		}
		action := payloadrulescontract.Action(item.Action)
		matchModel := openapiOptionalString(item.MatchModel)
		matchProtocol := openapiOptionalString(item.MatchProtocol)
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.payloadRules.UpdateRule(ctx, current.ID, payloadrulescontract.UpdateRule{
				Enabled:       &enabled,
				Priority:      &priority,
				Action:        &action,
				MatchModel:    &matchModel,
				MatchProtocol: &matchProtocol,
				Params:        &item.Params,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.payloadRules.CreateRule(ctx, payloadrulescontract.CreateRule{
			Name:          item.Name,
			Enabled:       enabled,
			Priority:      priority,
			Action:        action,
			MatchModel:    matchModel,
			MatchProtocol: matchProtocol,
			Params:        item.Params,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importProxies(ctx context.Context, items []apiopenapi.ImportProxyDefinition, dryRun bool) (apiopenapi.ImportRemapResult, error) {
	var result apiopenapi.ImportRemapResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.accounts.ListProxies(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]accountcontract.ProxyDefinition, len(existing))
	for _, proxy := range existing {
		byName[proxy.Name] = proxy
	}
	ordered := append([]apiopenapi.ImportProxyDefinition(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].BackupProxyName == nil && ordered[j].BackupProxyName != nil
	})
	for _, item := range ordered {
		proxyType := accountcontract.ProxyType(item.Type)
		status := toProxyStatusPtr(item.Status)
		metadata := jsonObjectToMapPtr(item.Metadata)
		backupProxyID := proxyIDByName(byName, item.BackupProxyName)
		proxyURL := item.Url
		if proxyURL != nil && strings.TrimSpace(*proxyURL) == "" {
			proxyURL = nil
		}
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			var updateType *accountcontract.ProxyType
			if proxyURL != nil {
				updateType = &proxyType
			}
			updated, err := s.runtime.accounts.UpdateProxy(ctx, current.ID, accountcontract.UpdateProxyRequest{
				Type:          updateType,
				URL:           proxyURL,
				Status:        status,
				Metadata:      metadata,
				CountryCode:   item.CountryCode,
				CountryName:   item.CountryName,
				ExpiresAt:     item.ExpiresAt,
				FallbackMode:  toProxyFallbackModePtr(item.FallbackMode),
				BackupProxyID: backupProxyID,
			})
			if err != nil {
				return result, err
			}
			byName[updated.Name] = updated
			continue
		}
		if proxyURL == nil {
			result.Skipped++
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		created, err := s.runtime.accounts.CreateProxy(ctx, accountcontract.CreateProxyRequest{
			Name:          item.Name,
			Type:          proxyType,
			URL:           *proxyURL,
			Status:        status,
			Metadata:      jsonObjectToMap(item.Metadata),
			CountryCode:   item.CountryCode,
			CountryName:   item.CountryName,
			ExpiresAt:     item.ExpiresAt,
			FallbackMode:  toProxyFallbackModePtr(item.FallbackMode),
			BackupProxyID: backupProxyID,
		})
		if err != nil {
			return result, err
		}
		byName[created.Name] = created
	}
	return result, nil
}

func proxyIDByName(items map[string]accountcontract.ProxyDefinition, name *string) *int {
	if name == nil {
		return nil
	}
	proxy, ok := items[strings.TrimSpace(*name)]
	if !ok {
		return nil
	}
	id := proxy.ID
	return &id
}

func (s *Server) importScheduledTestPlans(ctx context.Context, items []apiopenapi.ImportScheduledTestPlan, dryRun bool) (apiopenapi.ImportRemapResult, error) {
	var result apiopenapi.ImportRemapResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.scheduledTests.ListPlans(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]scheduledcontract.Plan, len(existing))
	for _, plan := range existing {
		if _, found := byName[plan.Name]; !found {
			byName[plan.Name] = plan
		}
	}
	scopeResolver, err := s.newScheduledTestPlanScopeResolver(ctx)
	if err != nil {
		return result, err
	}
	for _, item := range items {
		scopeType := scheduledcontract.ScopeType(item.ScopeType)
		scopeID, ok := scopeResolver.resolve(item)
		if !ok {
			result.Skipped++
			continue
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		intervalSeconds, ok := intPtrFromInt64Ptr(item.IntervalSeconds)
		if !ok {
			return result, scheduledservice.ErrInvalidInput
		}
		maxResults, ok := intPtrFromInt64Ptr(item.MaxResults)
		if !ok {
			return result, scheduledservice.ErrInvalidInput
		}
		autoRecover := false
		if item.AutoRecover != nil {
			autoRecover = *item.AutoRecover
		}
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.scheduledTests.UpdatePlan(ctx, current.ID, scheduledcontract.UpdatePlan{
				Enabled:         &enabled,
				ScopeType:       &scopeType,
				ScopeID:         scopeID,
				ClearScopeID:    scopeType == scheduledcontract.ScopeAll,
				IntervalSeconds: intervalSeconds,
				CronExpression:  openapiOptionalStringPtr(item.CronExpression),
				ProbeModel:      openapiOptionalStringPtr(item.ProbeModel),
				MaxResults:      maxResults,
				AutoRecover:     &autoRecover,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		input := scheduledcontract.CreatePlan{
			Name:           item.Name,
			Enabled:        enabled,
			ScopeType:      scopeType,
			ScopeID:        scopeID,
			CronExpression: openapiOptionalString(item.CronExpression),
			ProbeModel:     openapiOptionalString(item.ProbeModel),
			AutoRecover:    autoRecover,
		}
		if intervalSeconds != nil {
			input.IntervalSeconds = *intervalSeconds
		}
		if maxResults != nil {
			input.MaxResults = *maxResults
		}
		if _, err := s.runtime.scheduledTests.CreatePlan(ctx, input); err != nil {
			return result, err
		}
	}
	return result, nil
}

type scheduledTestPlanScopeResolver struct {
	accountIDByProviderAndName map[string]int
	uniqueAccountIDByName      map[string]int
	ambiguousAccountNames      map[string]struct{}
	groupIDByName              map[string]int
}

func (s *Server) newScheduledTestPlanScopeResolver(ctx context.Context) (scheduledTestPlanScopeResolver, error) {
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return scheduledTestPlanScopeResolver{}, err
	}
	groups, err := s.runtime.accounts.ListGroups(ctx)
	if err != nil {
		return scheduledTestPlanScopeResolver{}, err
	}
	providers, err := s.runtime.providers.List(ctx)
	if err != nil {
		return scheduledTestPlanScopeResolver{}, err
	}
	providerNameByID := make(map[int]string, len(providers))
	for _, provider := range providers {
		providerNameByID[provider.ID] = provider.Name
	}
	resolver := scheduledTestPlanScopeResolver{
		accountIDByProviderAndName: make(map[string]int, len(accounts)),
		uniqueAccountIDByName:      make(map[string]int, len(accounts)),
		ambiguousAccountNames:      make(map[string]struct{}),
		groupIDByName:              make(map[string]int, len(groups)),
	}
	for _, account := range accounts {
		providerName := providerNameByID[account.ProviderID]
		if providerName != "" {
			resolver.accountIDByProviderAndName[providerName+"\x00"+account.Name] = account.ID
		}
		if _, exists := resolver.uniqueAccountIDByName[account.Name]; exists {
			delete(resolver.uniqueAccountIDByName, account.Name)
			resolver.ambiguousAccountNames[account.Name] = struct{}{}
		} else if _, ambiguous := resolver.ambiguousAccountNames[account.Name]; !ambiguous {
			resolver.uniqueAccountIDByName[account.Name] = account.ID
		}
	}
	for _, group := range groups {
		resolver.groupIDByName[group.Name] = group.ID
	}
	return resolver, nil
}

func (r scheduledTestPlanScopeResolver) resolve(item apiopenapi.ImportScheduledTestPlan) (*int, bool) {
	switch scheduledcontract.ScopeType(item.ScopeType) {
	case scheduledcontract.ScopeAll:
		return nil, true
	case scheduledcontract.ScopeAccount:
		accountName := openapiOptionalString(item.ScopeAccountName)
		if accountName == "" {
			return nil, false
		}
		providerName := openapiOptionalString(item.ScopeAccountProviderName)
		if providerName != "" {
			if accountID, ok := r.accountIDByProviderAndName[providerName+"\x00"+accountName]; ok {
				return &accountID, true
			}
			return nil, false
		}
		if accountID, ok := r.uniqueAccountIDByName[accountName]; ok {
			return &accountID, true
		}
		return nil, false
	case scheduledcontract.ScopeGroup:
		groupName := openapiOptionalString(item.ScopeGroupName)
		if groupName == "" {
			return nil, false
		}
		if groupID, ok := r.groupIDByName[groupName]; ok {
			return &groupID, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func (s *Server) importTLSProfiles(ctx context.Context, items []apiopenapi.CreateTLSProfileRequest, dryRun bool) (apiopenapi.ImportSectionResult, error) {
	var result apiopenapi.ImportSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.tlsProfiles.ListProfiles(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]tlsprofilescontract.Profile, len(existing))
	for _, profile := range existing {
		byName[profile.Name] = profile
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.tlsProfiles.UpdateProfile(ctx, current.ID, tlsprofilescontract.UpdateProfile{
				TLSTemplate:       openapiOptionalStringPtr(item.TlsTemplate),
				HTTPVersionPolicy: openapiOptionalStringPtr(item.HttpVersionPolicy),
				UserAgent:         openapiOptionalStringPtr(item.UserAgent),
				ExtraHeaders:      openapiOptionalStringMapPtr(item.ExtraHeaders),
				Enabled:           &enabled,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.tlsProfiles.CreateProfile(ctx, tlsprofilescontract.CreateProfile{
			Name:              item.Name,
			TLSTemplate:       openapiOptionalString(item.TlsTemplate),
			HTTPVersionPolicy: openapiOptionalString(item.HttpVersionPolicy),
			UserAgent:         openapiOptionalString(item.UserAgent),
			ExtraHeaders:      openapiOptionalStringMap(item.ExtraHeaders),
			Enabled:           enabled,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importUserAttributeDefinitions(ctx context.Context, items []apiopenapi.CreateUserAttributeDefinitionRequest, dryRun bool) (apiopenapi.ImportSectionResult, error) {
	var result apiopenapi.ImportSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.userAttributes.ListDefinitions(ctx)
	if err != nil {
		return result, err
	}
	byKey := make(map[string]userattributescontract.Definition, len(existing))
	for _, def := range existing {
		byKey[def.Key] = def
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		options := openapiOptionalStringSlice(item.Options)
		required := openapiOptionalBool(item.Required)
		displayOrder, ok := nonNegativeIntFromInt64Ptr(item.DisplayOrder)
		if !ok {
			return result, userattributesservice.ErrInvalidInput
		}
		dataType := userattributescontract.DataType(item.DataType)
		current, found := byKey[item.Key]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.userAttributes.UpdateDefinition(ctx, current.ID, userattributescontract.UpdateDefinition{
				Name:         &item.Name,
				DataType:     &dataType,
				Options:      &options,
				Required:     &required,
				DisplayOrder: &displayOrder,
				Enabled:      &enabled,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.userAttributes.CreateDefinition(ctx, userattributescontract.CreateDefinition{
			Key:          item.Key,
			Name:         item.Name,
			DataType:     dataType,
			Options:      options,
			Required:     required,
			DisplayOrder: displayOrder,
			Enabled:      enabled,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importErrorPassthroughRules(ctx context.Context, items []apiopenapi.CreateErrorPassthroughRuleRequest, dryRun bool) (apiopenapi.ImportSectionResult, error) {
	var result apiopenapi.ImportSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.errorPassthrough.ListRules(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]errorpassthroughcontract.Rule, len(existing))
	for _, rule := range existing {
		byName[rule.Name] = rule
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		action := errorpassthroughcontract.Action(item.Action)
		priority, ok := nonNegativeIntFromInt64Ptr(item.Priority)
		if !ok {
			return result, errorpassthroughservice.ErrInvalidInput
		}
		statusCodes, ok := nonNegativeIntSliceFromInt64Ptr(item.StatusCodes)
		if !ok {
			return result, errorpassthroughservice.ErrInvalidInput
		}
		responseStatus := firstInt64PtrAsInt(item.ResponseStatus, item.ResponseCode)
		classes := openapiOptionalStringSlice(item.Classes)
		keywords := openapiOptionalStringSlice(item.Keywords)
		customMessage := openapiOptionalString(item.CustomMessage)
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.errorPassthrough.UpdateRule(ctx, current.ID, errorpassthroughcontract.UpdateRule{
				Enabled:        &enabled,
				Priority:       &priority,
				Action:         &action,
				StatusCodes:    &statusCodes,
				Classes:        &classes,
				Keywords:       &keywords,
				ResponseStatus: &responseStatus,
				CustomMessage:  &customMessage,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.errorPassthrough.CreateRule(ctx, errorpassthroughcontract.CreateRule{
			Name:           item.Name,
			Enabled:        enabled,
			Priority:       priority,
			Action:         action,
			StatusCodes:    statusCodes,
			Classes:        classes,
			Keywords:       keywords,
			ResponseStatus: responseStatus,
			CustomMessage:  customMessage,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) writeConfigImportError(w http.ResponseWriter, err error, requestID string) {
	_ = err
	writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "config import failed", requestID)
}
