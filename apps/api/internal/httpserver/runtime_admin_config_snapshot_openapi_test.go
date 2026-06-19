package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminRateLimitConfigSnapshotUsesOpenAPIWireTypes(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"openapi-rate-limit-model","display_name":"OpenAPI Rate Limit Model","status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, csrfToken, `{"name":"openapi-rate-limit-group","description":"OpenAPI Rate Limit Group","status":"active"}`)
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"openapi-scheduled-provider","display_name":"OpenAPI Scheduled Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"openapi-scheduled-account","runtime_class":"api_key","credential":{"api_key":"scheduled-secret"},"status":"active"}`)
	modelID := mustParseOpenAPIID(t, modelResp.Data.Id)
	groupID := mustParseOpenAPIID(t, groupResp.Data.Id)
	accountID := mustParseOpenAPIID(t, accountResp.Data.Id)

	modelLimitResp := upsertModelRateLimit(t, handler, sessionCookie, csrfToken, `{"model_id":`+strconv.FormatInt(modelID, 10)+`,"rpm_limit":120,"tpm_limit":4000,"max_concurrency":3,"enabled":true}`)
	if modelLimitResp.Data.ModelId != modelID || modelLimitResp.Data.RpmLimit != 120 || modelLimitResp.Data.TpmLimit != 4000 || modelLimitResp.Data.MaxConcurrency != 3 || !modelLimitResp.Data.Enabled {
		t.Fatalf("unexpected model rate limit response: %+v", modelLimitResp.Data)
	}

	groupLimitResp := upsertGroupRateLimit(t, handler, sessionCookie, csrfToken, `{"account_group_id":`+strconv.FormatInt(groupID, 10)+`,"rpm_limit":90,"tpm_limit":3000,"max_concurrency":2,"enabled":true}`)
	if groupLimitResp.Data.AccountGroupId != groupID || groupLimitResp.Data.RpmLimit != 90 || groupLimitResp.Data.TpmLimit != 3000 || groupLimitResp.Data.MaxConcurrency != 2 || !groupLimitResp.Data.Enabled {
		t.Fatalf("unexpected group rate limit response: %+v", groupLimitResp.Data)
	}

	payloadRuleResp := createAdminPayloadRule(t, handler, sessionCookie, csrfToken, `{"name":"openapi-snapshot-payload-rule","enabled":true,"priority":5,"action":"override","match_model":"gpt-*","match_protocol":"openai-compatible","params":{"temperature":0.2}}`)
	if payloadRuleResp.Data.Name != "openapi-snapshot-payload-rule" || payloadRuleResp.Data.Priority != 5 || payloadRuleResp.Data.Action != apiopenapi.PayloadRuleActionOverride {
		t.Fatalf("unexpected payload rule response: %+v", payloadRuleResp.Data)
	}

	proxyResp := createAdminProxy(t, handler, sessionCookie, csrfToken, `{"name":"openapi-snapshot-proxy","type":"https","url":"https://proxy-user:proxy-pass@example.invalid:8443","status":"active","metadata":{"region":"us-east"},"country_code":"US","country_name":"United States"}`)
	if proxyResp.Data.Name != "openapi-snapshot-proxy" || proxyResp.Data.Type != apiopenapi.Https || !proxyResp.Data.UrlConfigured {
		t.Fatalf("unexpected proxy response: %+v", proxyResp.Data)
	}

	scheduledPlanResp := createAdminScheduledTestPlan(t, handler, sessionCookie, csrfToken, `{"name":"openapi-snapshot-scheduled-account","enabled":true,"scope_type":"account","scope_id":`+strconv.FormatInt(accountID, 10)+`,"interval_seconds":3600,"cron_expression":"0 */6 * * *","probe_model":"gpt-probe","max_results":7,"auto_recover":true}`)
	if scheduledPlanResp.Data.Name != "openapi-snapshot-scheduled-account" || scheduledPlanResp.Data.ScopeId == nil || *scheduledPlanResp.Data.ScopeId != accountID || scheduledPlanResp.Data.ProbeModel != "gpt-probe" {
		t.Fatalf("unexpected scheduled test plan response: %+v", scheduledPlanResp.Data)
	}

	modelListReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/model-rate-limits?page=1&page_size=10", nil)
	modelListReq.AddCookie(sessionCookie)
	modelListRec := httptest.NewRecorder()
	handler.ServeHTTP(modelListRec, modelListReq)
	if modelListRec.Code != http.StatusOK {
		t.Fatalf("expected model rate limit list 200, got %d body=%s", modelListRec.Code, modelListRec.Body.String())
	}
	var modelListResp apiopenapi.ModelRateLimitListResponse
	if err := json.NewDecoder(modelListRec.Body).Decode(&modelListResp); err != nil {
		t.Fatalf("decode model rate limit list response: %v", err)
	}
	if modelListResp.Pagination.Total != 1 || !modelRateLimitListHasModelID(modelListResp.Data, modelID) {
		t.Fatalf("unexpected model rate limit list: %+v pagination=%+v", modelListResp.Data, modelListResp.Pagination)
	}

	groupListReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/group-rate-limits?page=1&page_size=10", nil)
	groupListReq.AddCookie(sessionCookie)
	groupListRec := httptest.NewRecorder()
	handler.ServeHTTP(groupListRec, groupListReq)
	if groupListRec.Code != http.StatusOK {
		t.Fatalf("expected group rate limit list 200, got %d body=%s", groupListRec.Code, groupListRec.Body.String())
	}
	var groupListResp apiopenapi.GroupRateLimitListResponse
	if err := json.NewDecoder(groupListRec.Body).Decode(&groupListResp); err != nil {
		t.Fatalf("decode group rate limit list response: %v", err)
	}
	if groupListResp.Pagination.Total != 1 || !groupRateLimitListHasGroupID(groupListResp.Data, groupID) {
		t.Fatalf("unexpected group rate limit list: %+v pagination=%+v", groupListResp.Data, groupListResp.Pagination)
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config-snapshot", nil)
	snapshotReq.AddCookie(sessionCookie)
	snapshotRec := httptest.NewRecorder()
	handler.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("expected config snapshot 200, got %d body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}
	if strings.Contains(snapshotRec.Body.String(), "proxy-pass") {
		t.Fatalf("config snapshot leaked proxy credential: %s", snapshotRec.Body.String())
	}
	var snapshotResp apiopenapi.ConfigSnapshotResponse
	if err := json.NewDecoder(snapshotRec.Body).Decode(&snapshotResp); err != nil {
		t.Fatalf("decode config snapshot response: %v", err)
	}
	if snapshotResp.Data.SnapshotVersion == "" || snapshotResp.Data.ModelRateLimits == nil || snapshotResp.Data.GroupRateLimits == nil || snapshotResp.Data.PayloadRules == nil || snapshotResp.Data.Proxies == nil || snapshotResp.Data.ScheduledTestPlans == nil {
		t.Fatalf("unexpected config snapshot response: %+v", snapshotResp.Data)
	}
	snapshotPayloadRule := findImportPayloadRuleByName(*snapshotResp.Data.PayloadRules, "openapi-snapshot-payload-rule")
	if snapshotPayloadRule == nil || snapshotPayloadRule.Priority == nil || *snapshotPayloadRule.Priority != 5 || snapshotPayloadRule.Action != apiopenapi.CreatePayloadRuleRequestActionOverride {
		t.Fatalf("unexpected snapshot payload rules: %+v", *snapshotResp.Data.PayloadRules)
	}
	snapshotProxy := findSnapshotProxyByName(*snapshotResp.Data.Proxies, "openapi-snapshot-proxy")
	if snapshotProxy == nil || snapshotProxy.Type != apiopenapi.Https || !snapshotProxy.UrlConfigured || snapshotProxy.CountryCode == nil || *snapshotProxy.CountryCode != "US" {
		t.Fatalf("unexpected snapshot proxies: %+v", *snapshotResp.Data.Proxies)
	}
	snapshotScheduledPlan := findImportScheduledTestPlanByName(*snapshotResp.Data.ScheduledTestPlans, "openapi-snapshot-scheduled-account")
	if snapshotScheduledPlan == nil ||
		snapshotScheduledPlan.ScopeAccountProviderName == nil ||
		*snapshotScheduledPlan.ScopeAccountProviderName != "openapi-scheduled-provider" ||
		snapshotScheduledPlan.ScopeAccountName == nil ||
		*snapshotScheduledPlan.ScopeAccountName != "openapi-scheduled-account" ||
		snapshotScheduledPlan.ScopeType != apiopenapi.ImportScheduledTestPlanScopeTypeAccount ||
		snapshotScheduledPlan.IntervalSeconds == nil ||
		*snapshotScheduledPlan.IntervalSeconds != 3600 ||
		snapshotScheduledPlan.MaxResults == nil ||
		*snapshotScheduledPlan.MaxResults != 7 ||
		snapshotScheduledPlan.AutoRecover == nil ||
		!*snapshotScheduledPlan.AutoRecover {
		t.Fatalf("unexpected snapshot scheduled test plans: %+v", *snapshotResp.Data.ScheduledTestPlans)
	}
	snapshotModelLimit := findSnapshotModelRateLimit(*snapshotResp.Data.ModelRateLimits, "openapi-rate-limit-model")
	if snapshotModelLimit == nil || snapshotModelLimit.RpmLimit != 120 || snapshotModelLimit.TpmLimit != 4000 || snapshotModelLimit.MaxConcurrency != 3 || !snapshotModelLimit.Enabled {
		t.Fatalf("unexpected snapshot model rate limits: %+v", *snapshotResp.Data.ModelRateLimits)
	}
	snapshotGroupLimit := findSnapshotGroupRateLimit(*snapshotResp.Data.GroupRateLimits, "openapi-rate-limit-group")
	if snapshotGroupLimit == nil || snapshotGroupLimit.RpmLimit != 90 || snapshotGroupLimit.TpmLimit != 3000 || snapshotGroupLimit.MaxConcurrency != 2 || !snapshotGroupLimit.Enabled {
		t.Fatalf("unexpected snapshot group rate limits: %+v", *snapshotResp.Data.GroupRateLimits)
	}

	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/config-snapshot/import?dry_run=true", strings.NewReader(`{"payload_rules":[{"name":"openapi-snapshot-payload-rule","enabled":false,"priority":9,"action":"filter","match_model":"gpt-*","match_protocol":"openai-compatible","params":{"metadata.trace":true}},{"name":"openapi-snapshot-payload-rule-new","enabled":true,"priority":3,"action":"default","params":{"temperature":0.4}}],"proxies":[{"name":"openapi-snapshot-proxy","type":"http","status":"disabled","metadata":{"region":"eu-west"},"country_code":"DE","country_name":"Germany"},{"name":"openapi-snapshot-proxy-new","type":"http","url":"http://proxy-new.example.invalid:8080","status":"active","metadata":{"region":"ap-south"}},{"name":"openapi-snapshot-proxy-missing-url","type":"socks5","status":"active"}],"scheduled_test_plans":[{"name":"openapi-snapshot-scheduled-account","enabled":false,"scope_type":"account","scope_account_provider_name":"openapi-scheduled-provider","scope_account_name":"openapi-scheduled-account","interval_seconds":7200,"cron_expression":"*/30 * * * *","probe_model":"gpt-updated","max_results":3,"auto_recover":false},{"name":"openapi-snapshot-scheduled-group-new","enabled":true,"scope_type":"group","scope_group_name":"openapi-rate-limit-group","interval_seconds":600,"probe_model":"gpt-group","max_results":2,"auto_recover":true},{"name":"openapi-snapshot-scheduled-missing-account","scope_type":"account","scope_account_provider_name":"missing-openapi-scheduled-provider","scope_account_name":"openapi-scheduled-account"},{"name":"openapi-snapshot-scheduled-missing-group","scope_type":"group","scope_group_name":"missing-openapi-rate-limit-group"}],"model_rate_limits":[{"model_name":"openapi-rate-limit-model","rpm_limit":121,"tpm_limit":4001,"max_concurrency":4,"enabled":true},{"model_name":"missing-openapi-rate-limit-model","rpm_limit":1}],"group_rate_limits":[{"account_group_name":"openapi-rate-limit-group","rpm_limit":91,"tpm_limit":3001,"max_concurrency":3,"enabled":true},{"account_group_name":"missing-openapi-rate-limit-group","rpm_limit":1}]}`))
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("X-CSRF-Token", csrfToken)
	importReq.AddCookie(sessionCookie)
	importRec := httptest.NewRecorder()
	handler.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected config import dry run 200, got %d body=%s", importRec.Code, importRec.Body.String())
	}
	var importResp apiopenapi.ConfigImportResponse
	if err := json.NewDecoder(importRec.Body).Decode(&importResp); err != nil {
		t.Fatalf("decode config import response: %v", err)
	}
	if !importResp.Data.DryRun ||
		importResp.Data.ModelRateLimits.Updated != 1 ||
		importResp.Data.ModelRateLimits.Skipped != 1 ||
		importResp.Data.GroupRateLimits.Updated != 1 ||
		importResp.Data.GroupRateLimits.Skipped != 1 ||
		importResp.Data.PayloadRules.Updated != 1 ||
		importResp.Data.PayloadRules.Created != 1 ||
		importResp.Data.Proxies.Updated != 1 ||
		importResp.Data.Proxies.Created != 1 ||
		importResp.Data.Proxies.Skipped != 1 ||
		importResp.Data.ScheduledTestPlans.Updated != 1 ||
		importResp.Data.ScheduledTestPlans.Created != 1 ||
		importResp.Data.ScheduledTestPlans.Skipped != 2 {
		t.Fatalf("unexpected config import dry run response: %+v", importResp.Data)
	}

	importPayloadRulesReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/config-snapshot/import", strings.NewReader(`{"payload_rules":[{"name":"openapi-snapshot-payload-rule","enabled":false,"priority":9,"action":"filter","match_model":"gpt-*","match_protocol":"openai-compatible","params":{"metadata.trace":true}},{"name":"openapi-snapshot-payload-rule-new","enabled":true,"priority":3,"action":"default","params":{"temperature":0.4}}],"proxies":[{"name":"openapi-snapshot-proxy","type":"http","status":"disabled","metadata":{"region":"eu-west"},"country_code":"DE","country_name":"Germany"},{"name":"openapi-snapshot-proxy-new","type":"http","url":"http://proxy-new.example.invalid:8080","status":"active","metadata":{"region":"ap-south"}},{"name":"openapi-snapshot-proxy-missing-url","type":"socks5","status":"active"}],"scheduled_test_plans":[{"name":"openapi-snapshot-scheduled-account","enabled":false,"scope_type":"account","scope_account_provider_name":"openapi-scheduled-provider","scope_account_name":"openapi-scheduled-account","interval_seconds":7200,"cron_expression":"*/30 * * * *","probe_model":"gpt-updated","max_results":3,"auto_recover":false},{"name":"openapi-snapshot-scheduled-group-new","enabled":true,"scope_type":"group","scope_group_name":"openapi-rate-limit-group","interval_seconds":600,"probe_model":"gpt-group","max_results":2,"auto_recover":true},{"name":"openapi-snapshot-scheduled-missing-account","scope_type":"account","scope_account_provider_name":"missing-openapi-scheduled-provider","scope_account_name":"openapi-scheduled-account"},{"name":"openapi-snapshot-scheduled-missing-group","scope_type":"group","scope_group_name":"missing-openapi-rate-limit-group"}]}`))
	importPayloadRulesReq.Header.Set("Content-Type", "application/json")
	importPayloadRulesReq.Header.Set("X-CSRF-Token", csrfToken)
	importPayloadRulesReq.AddCookie(sessionCookie)
	importPayloadRulesRec := httptest.NewRecorder()
	handler.ServeHTTP(importPayloadRulesRec, importPayloadRulesReq)
	if importPayloadRulesRec.Code != http.StatusOK {
		t.Fatalf("expected config import 200, got %d body=%s", importPayloadRulesRec.Code, importPayloadRulesRec.Body.String())
	}
	var importPayloadRulesResp apiopenapi.ConfigImportResponse
	if err := json.NewDecoder(importPayloadRulesRec.Body).Decode(&importPayloadRulesResp); err != nil {
		t.Fatalf("decode payload rules config import response: %v", err)
	}
	if importPayloadRulesResp.Data.PayloadRules.Updated != 1 || importPayloadRulesResp.Data.PayloadRules.Created != 1 {
		t.Fatalf("unexpected payload rules config import response: %+v", importPayloadRulesResp.Data)
	}
	if importPayloadRulesResp.Data.Proxies.Updated != 1 || importPayloadRulesResp.Data.Proxies.Created != 1 || importPayloadRulesResp.Data.Proxies.Skipped != 1 {
		t.Fatalf("unexpected proxies config import response: %+v", importPayloadRulesResp.Data)
	}
	if importPayloadRulesResp.Data.ScheduledTestPlans.Updated != 1 || importPayloadRulesResp.Data.ScheduledTestPlans.Created != 1 || importPayloadRulesResp.Data.ScheduledTestPlans.Skipped != 2 {
		t.Fatalf("unexpected scheduled tests config import response: %+v", importPayloadRulesResp.Data)
	}

	payloadListReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payload-rules?page=1&page_size=10", nil)
	payloadListReq.AddCookie(sessionCookie)
	payloadListRec := httptest.NewRecorder()
	handler.ServeHTTP(payloadListRec, payloadListReq)
	if payloadListRec.Code != http.StatusOK {
		t.Fatalf("expected payload rule list 200, got %d body=%s", payloadListRec.Code, payloadListRec.Body.String())
	}
	var payloadListResp apiopenapi.PayloadRuleListResponse
	if err := json.NewDecoder(payloadListRec.Body).Decode(&payloadListResp); err != nil {
		t.Fatalf("decode payload rule list response: %v", err)
	}
	updatedPayloadRule := findPayloadRuleByName(payloadListResp.Data, "openapi-snapshot-payload-rule")
	newPayloadRule := findPayloadRuleByName(payloadListResp.Data, "openapi-snapshot-payload-rule-new")
	if updatedPayloadRule == nil || updatedPayloadRule.Enabled || updatedPayloadRule.Priority != 9 || updatedPayloadRule.Action != apiopenapi.PayloadRuleActionFilter {
		t.Fatalf("unexpected imported updated payload rule: %+v", updatedPayloadRule)
	}
	if newPayloadRule == nil || !newPayloadRule.Enabled || newPayloadRule.Priority != 3 || newPayloadRule.Action != apiopenapi.PayloadRuleActionDefault {
		t.Fatalf("unexpected imported new payload rule: %+v", newPayloadRule)
	}

	proxyListReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies?page=1&page_size=10", nil)
	proxyListReq.AddCookie(sessionCookie)
	proxyListRec := httptest.NewRecorder()
	handler.ServeHTTP(proxyListRec, proxyListReq)
	if proxyListRec.Code != http.StatusOK {
		t.Fatalf("expected proxy list 200, got %d body=%s", proxyListRec.Code, proxyListRec.Body.String())
	}
	if strings.Contains(proxyListRec.Body.String(), "proxy-new.example") || strings.Contains(proxyListRec.Body.String(), "proxy-pass") {
		t.Fatalf("proxy list leaked raw proxy URL: %s", proxyListRec.Body.String())
	}
	var proxyListResp apiopenapi.ProxyDefinitionListResponse
	if err := json.NewDecoder(proxyListRec.Body).Decode(&proxyListResp); err != nil {
		t.Fatalf("decode proxy list response: %v", err)
	}
	updatedProxy := findProxyByName(proxyListResp.Data, "openapi-snapshot-proxy")
	newProxy := findProxyByName(proxyListResp.Data, "openapi-snapshot-proxy-new")
	missingURLProxy := findProxyByName(proxyListResp.Data, "openapi-snapshot-proxy-missing-url")
	if updatedProxy == nil || updatedProxy.Type != apiopenapi.Https || updatedProxy.Status != apiopenapi.ProxyDefinitionStatusDisabled || !updatedProxy.UrlConfigured || updatedProxy.CountryCode == nil || *updatedProxy.CountryCode != "DE" {
		t.Fatalf("unexpected imported updated proxy: %+v", updatedProxy)
	}
	if newProxy == nil || newProxy.Type != apiopenapi.Http || newProxy.Status != apiopenapi.ProxyDefinitionStatusActive || !newProxy.UrlConfigured {
		t.Fatalf("unexpected imported new proxy: %+v", newProxy)
	}
	if missingURLProxy != nil {
		t.Fatalf("proxy without import URL should be skipped, got %+v", missingURLProxy)
	}

	scheduledListReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduled-test-plans?page=1&page_size=10", nil)
	scheduledListReq.AddCookie(sessionCookie)
	scheduledListRec := httptest.NewRecorder()
	handler.ServeHTTP(scheduledListRec, scheduledListReq)
	if scheduledListRec.Code != http.StatusOK {
		t.Fatalf("expected scheduled test plan list 200, got %d body=%s", scheduledListRec.Code, scheduledListRec.Body.String())
	}
	var scheduledListResp apiopenapi.ScheduledTestPlanListResponse
	if err := json.NewDecoder(scheduledListRec.Body).Decode(&scheduledListResp); err != nil {
		t.Fatalf("decode scheduled test plan list response: %v", err)
	}
	updatedScheduledPlan := findScheduledTestPlanByName(scheduledListResp.Data, "openapi-snapshot-scheduled-account")
	newScheduledPlan := findScheduledTestPlanByName(scheduledListResp.Data, "openapi-snapshot-scheduled-group-new")
	missingAccountScheduledPlan := findScheduledTestPlanByName(scheduledListResp.Data, "openapi-snapshot-scheduled-missing-account")
	missingGroupScheduledPlan := findScheduledTestPlanByName(scheduledListResp.Data, "openapi-snapshot-scheduled-missing-group")
	if updatedScheduledPlan == nil ||
		updatedScheduledPlan.Enabled ||
		updatedScheduledPlan.ScopeType != apiopenapi.ScheduledTestPlanScopeTypeAccount ||
		updatedScheduledPlan.ScopeId == nil ||
		*updatedScheduledPlan.ScopeId != accountID ||
		updatedScheduledPlan.IntervalSeconds != 7200 ||
		updatedScheduledPlan.CronExpression != "*/30 * * * *" ||
		updatedScheduledPlan.ProbeModel != "gpt-updated" ||
		updatedScheduledPlan.MaxResults != 3 ||
		updatedScheduledPlan.AutoRecover {
		t.Fatalf("unexpected imported updated scheduled test plan: %+v", updatedScheduledPlan)
	}
	if newScheduledPlan == nil ||
		!newScheduledPlan.Enabled ||
		newScheduledPlan.ScopeType != apiopenapi.ScheduledTestPlanScopeTypeGroup ||
		newScheduledPlan.ScopeId == nil ||
		*newScheduledPlan.ScopeId != groupID ||
		newScheduledPlan.IntervalSeconds != 600 ||
		newScheduledPlan.ProbeModel != "gpt-group" ||
		newScheduledPlan.MaxResults != 2 ||
		!newScheduledPlan.AutoRecover {
		t.Fatalf("unexpected imported new scheduled test plan: %+v", newScheduledPlan)
	}
	if missingAccountScheduledPlan != nil || missingGroupScheduledPlan != nil {
		t.Fatalf("unresolved scheduled test plans should be skipped, got account=%+v group=%+v", missingAccountScheduledPlan, missingGroupScheduledPlan)
	}
}

func upsertModelRateLimit(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ModelRateLimitResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/model-rate-limits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected model rate limit upsert 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelRateLimitResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model rate limit response: %v", err)
	}
	return resp
}

func mustParseOpenAPIID(t *testing.T, id apiopenapi.Id) int64 {
	t.Helper()
	parsed, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil {
		t.Fatalf("parse OpenAPI id %q: %v", id, err)
	}
	return parsed
}

func upsertGroupRateLimit(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.GroupRateLimitResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/group-rate-limits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected group rate limit upsert 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.GroupRateLimitResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode group rate limit response: %v", err)
	}
	return resp
}

func modelRateLimitListHasModelID(items []apiopenapi.ModelRateLimit, modelID int64) bool {
	for _, item := range items {
		if item.ModelId == modelID {
			return true
		}
	}
	return false
}

func groupRateLimitListHasGroupID(items []apiopenapi.AccountGroupRateLimit, groupID int64) bool {
	for _, item := range items {
		if item.AccountGroupId == groupID {
			return true
		}
	}
	return false
}

func findSnapshotModelRateLimit(items []apiopenapi.SnapshotModelRateLimit, modelName string) *apiopenapi.SnapshotModelRateLimit {
	for i := range items {
		if items[i].ModelName == modelName {
			return &items[i]
		}
	}
	return nil
}

func findSnapshotGroupRateLimit(items []apiopenapi.SnapshotGroupRateLimit, groupName string) *apiopenapi.SnapshotGroupRateLimit {
	for i := range items {
		if items[i].AccountGroupName == groupName {
			return &items[i]
		}
	}
	return nil
}

func findPayloadRuleByName(items []apiopenapi.PayloadRule, name string) *apiopenapi.PayloadRule {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func findImportPayloadRuleByName(items []apiopenapi.CreatePayloadRuleRequest, name string) *apiopenapi.CreatePayloadRuleRequest {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func createAdminProxy(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ProxyDefinitionResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected proxy create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProxyDefinitionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode proxy create response: %v", err)
	}
	return resp
}

func createAdminScheduledTestPlan(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ScheduledTestPlanResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scheduled-test-plans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected scheduled test plan create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ScheduledTestPlanResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode scheduled test plan create response: %v", err)
	}
	return resp
}

func findSnapshotProxyByName(items []apiopenapi.SnapshotProxyDefinition, name string) *apiopenapi.SnapshotProxyDefinition {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func findImportScheduledTestPlanByName(items []apiopenapi.ImportScheduledTestPlan, name string) *apiopenapi.ImportScheduledTestPlan {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func findScheduledTestPlanByName(items []apiopenapi.ScheduledTestPlan, name string) *apiopenapi.ScheduledTestPlan {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func findProxyByName(items []apiopenapi.ProxyDefinition, name string) *apiopenapi.ProxyDefinition {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}
