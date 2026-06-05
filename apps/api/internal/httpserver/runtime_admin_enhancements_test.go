package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminUserManagementEnhancements(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(`{"email":"managed@srapi.local","name":"Managed User","password":"password123","roles":["user"],"balance":"1.25000000","currency":"usd","rpm_limit":50}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp apiopenapi.UserResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode user create: %v", err)
	}
	if createResp.Data.Balance != "1.25000000" || createResp.Data.Currency != "USD" || createResp.Data.RpmLimit == nil || *createResp.Data.RpmLimit != 50 {
		t.Fatalf("unexpected created user: %+v", createResp.Data)
	}

	userID := string(createResp.Data.Id)
	balanceReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+userID+"/balance", strings.NewReader(`{"operation":"increment","amount":"0.33333333","note":"manual topup"}`))
	balanceReq.Header.Set("Content-Type", "application/json")
	balanceReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	balanceReq.AddCookie(sessionCookie)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected balance update 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp apiopenapi.UserResponse
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance update: %v", err)
	}
	if balanceResp.Data.Balance != "1.58333333" {
		t.Fatalf("expected decimal balance update, got %+v", balanceResp.Data)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+userID+"/disable", nil)
	disableReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	disableReq.AddCookie(sessionCookie)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected disable user 200, got %d body=%s", disableRec.Code, disableRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?q=managed&status=disabled", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list users 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.UserListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode user list: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].Id != createResp.Data.Id {
		t.Fatalf("unexpected user list: %+v", listResp.Data)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+userID+"/balance-history", nil)
	historyReq.AddCookie(sessionCookie)
	historyRec := httptest.NewRecorder()
	handler.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("expected balance history 200, got %d body=%s", historyRec.Code, historyRec.Body.String())
	}
	var historyResp apiopenapi.BillingLedgerListResponse
	if err := json.NewDecoder(historyRec.Body).Decode(&historyResp); err != nil {
		t.Fatalf("decode balance history: %v", err)
	}
	if len(historyResp.Data) != 1 || historyResp.Data[0].Amount != "0.33333333" || historyResp.Data[0].BalanceAfter != "1.58333333" {
		t.Fatalf("unexpected balance history: %+v", historyResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"user.create", "user.balance_update", "user.disable"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
}

func TestAdminAccountManagementEnhancements(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"enhance-provider","display_name":"Enhance Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	resetAt := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"enhance-account","runtime_class":"api_key","credential":{"api_key":"secret"},"proxy_id":"proxy-us","metadata":{"rpm_used":4,"rpm_limit":10,"rpm_window_seconds":60,"rpm_reset_at":"`+resetAt+`","last_error_class":"rate_limit","last_error_message":"too many requests","cooldown_active":true,"proxy_region":"us-east","egress_ip_hash":"hash","proxy_sample_count":5,"proxy_success_rate":0.8,"proxy_error_rate":0.2,"proxy_latency_p95_ms":321},"status":"active"}`)

	rpmReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/rpm-status", nil)
	rpmReq.AddCookie(sessionCookie)
	rpmRec := httptest.NewRecorder()
	handler.ServeHTTP(rpmRec, rpmReq)
	if rpmRec.Code != http.StatusOK {
		t.Fatalf("expected rpm status 200, got %d body=%s", rpmRec.Code, rpmRec.Body.String())
	}
	var rpmResp apiopenapi.AccountRpmStatusResponse
	if err := json.NewDecoder(rpmRec.Body).Decode(&rpmResp); err != nil {
		t.Fatalf("decode rpm status: %v", err)
	}
	if rpmResp.Data.RpmUsed != 4 || rpmResp.Data.RpmLimit == nil || *rpmResp.Data.RpmLimit != 10 || rpmResp.Data.WindowSeconds != 60 {
		t.Fatalf("unexpected rpm status: %+v", rpmResp.Data)
	}

	qualityReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/proxy-quality", nil)
	qualityReq.AddCookie(sessionCookie)
	qualityRec := httptest.NewRecorder()
	handler.ServeHTTP(qualityRec, qualityReq)
	if qualityRec.Code != http.StatusOK {
		t.Fatalf("expected proxy quality 200, got %d body=%s", qualityRec.Code, qualityRec.Body.String())
	}
	var qualityResp apiopenapi.AccountProxyQualityResponse
	if err := json.NewDecoder(qualityRec.Body).Decode(&qualityResp); err != nil {
		t.Fatalf("decode proxy quality: %v", err)
	}
	if qualityResp.Data.ProxyId == nil || *qualityResp.Data.ProxyId != "proxy-us" || qualityResp.Data.SampleCount != 5 || qualityResp.Data.LatencyP95Ms != 321 {
		t.Fatalf("unexpected proxy quality: %+v", qualityResp.Data)
	}
	if qualityResp.Data.Metadata == nil || (*qualityResp.Data.Metadata)["proxy_region"] != "us-east" {
		t.Fatalf("unexpected proxy quality metadata: %+v", qualityResp.Data.Metadata)
	}

	clearReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/clear-error", nil)
	clearReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	clearReq.AddCookie(sessionCookie)
	clearRec := httptest.NewRecorder()
	handler.ServeHTTP(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected clear error 200, got %d body=%s", clearRec.Code, clearRec.Body.String())
	}
	var clearResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(clearRec.Body).Decode(&clearResp); err != nil {
		t.Fatalf("decode clear error: %v", err)
	}
	if clearResp.Data.Metadata == nil || (*clearResp.Data.Metadata)["last_error_class"] != nil || (*clearResp.Data.Metadata)["last_error_cleared_at"] == nil {
		t.Fatalf("expected cleared metadata, got %+v", clearResp.Data.Metadata)
	}

	batchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/batch", strings.NewReader(`{"account_ids":["`+string(accountResp.Data.Id)+`"],"status":"disabled"}`))
	batchReq.Header.Set("Content-Type", "application/json")
	batchReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	batchReq.AddCookie(sessionCookie)
	batchRec := httptest.NewRecorder()
	handler.ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusOK {
		t.Fatalf("expected account batch update 200, got %d body=%s", batchRec.Code, batchRec.Body.String())
	}
	var batchResp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(batchRec.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode account batch update: %v", err)
	}
	if batchResp.Data.UpdatedCount != 1 || batchResp.Data.UpdatedIds[0] != accountResp.Data.Id {
		t.Fatalf("unexpected account batch update: %+v", batchResp.Data)
	}
}

func TestAdminAccountBatchActions(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"batch-action-provider","display_name":"Batch Action Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountA := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"batch-action-a","runtime_class":"api_key","credential":{"api_key":"secret-a"},"metadata":{"last_error_class":"rate_limit","last_error_message":"too many requests","cooldown_active":true},"status":"dead"}`)
	accountB := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"batch-action-b","runtime_class":"api_key","credential":{"api_key":"secret-b"},"metadata":{"cooldown_active":true},"status":"suspended"}`)

	doBatchAction := func(t *testing.T, body string, withCSRF bool, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-action", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if withCSRF {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// clear_error happy path across two accounts.
	clearRec := doBatchAction(t, `{"account_ids":["`+string(accountA.Data.Id)+`","`+string(accountB.Data.Id)+`"],"action":"clear_error"}`, true, sessionCookie)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected clear_error batch 200, got %d body=%s", clearRec.Code, clearRec.Body.String())
	}
	var clearResp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(clearRec.Body).Decode(&clearResp); err != nil {
		t.Fatalf("decode clear_error batch: %v", err)
	}
	if clearResp.Data.UpdatedCount != 2 || len(clearResp.Data.UpdatedIds) != 2 || len(clearResp.Data.Errors) != 0 {
		t.Fatalf("unexpected clear_error batch result: %+v", clearResp.Data)
	}

	// Verify account A error metadata was cleared and reactivated.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountA.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get account 200, got %d", getRec.Code)
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode get account: %v", err)
	}
	if getResp.Data.Status != apiopenapi.ProviderAccountStatusActive {
		t.Fatalf("expected account reactivated, got status %s", getResp.Data.Status)
	}
	if getResp.Data.Metadata == nil || (*getResp.Data.Metadata)["last_error_class"] != nil || (*getResp.Data.Metadata)["last_error_cleared_at"] == nil {
		t.Fatalf("expected cleared error metadata, got %+v", getResp.Data.Metadata)
	}

	// recover happy path.
	recoverRec := doBatchAction(t, `{"account_ids":["`+string(accountB.Data.Id)+`"],"action":"recover"}`, true, sessionCookie)
	if recoverRec.Code != http.StatusOK {
		t.Fatalf("expected recover batch 200, got %d body=%s", recoverRec.Code, recoverRec.Body.String())
	}
	var recoverResp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(recoverRec.Body).Decode(&recoverResp); err != nil {
		t.Fatalf("decode recover batch: %v", err)
	}
	if recoverResp.Data.UpdatedCount != 1 || recoverResp.Data.UpdatedIds[0] != accountB.Data.Id || len(recoverResp.Data.Errors) != 0 {
		t.Fatalf("unexpected recover batch result: %+v", recoverResp.Data)
	}

	// partial failure: one valid id + one missing id accumulates an error but still updates the valid one.
	partialRec := doBatchAction(t, `{"account_ids":["`+string(accountA.Data.Id)+`","999999"],"action":"recover"}`, true, sessionCookie)
	if partialRec.Code != http.StatusOK {
		t.Fatalf("expected partial recover 200, got %d body=%s", partialRec.Code, partialRec.Body.String())
	}
	var partialResp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(partialRec.Body).Decode(&partialResp); err != nil {
		t.Fatalf("decode partial recover: %v", err)
	}
	if partialResp.Data.UpdatedCount != 1 || partialResp.Data.UpdatedIds[0] != accountA.Data.Id || len(partialResp.Data.Errors) != 1 {
		t.Fatalf("expected partial result with 1 update + 1 error, got %+v", partialResp.Data)
	}

	// invalid action -> 400.
	invalidRec := doBatchAction(t, `{"account_ids":["`+string(accountA.Data.Id)+`"],"action":"nuke"}`, true, sessionCookie)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid action 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}

	// admin gating: no session -> 403.
	unauthRec := doBatchAction(t, `{"account_ids":["`+string(accountA.Data.Id)+`"],"action":"recover"}`, false, nil)
	if unauthRec.Code != http.StatusForbidden {
		t.Fatalf("expected unauthenticated 403, got %d body=%s", unauthRec.Code, unauthRec.Body.String())
	}

	// audit trail recorded for the batch action.
	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	if !auditLogHasAction(auditResp.Data, "provider_account.batch_action") {
		t.Fatalf("expected provider_account.batch_action audit, got %+v", auditResp.Data)
	}
}

func TestAdminUsageDashboardAggregatesAndExport(t *testing.T) {
	usageStore := usagememory.New()
	handler := New(config.Load(), nil, WithUsageStore(usageStore))
	_, sessionCookie := mustLoginAdmin(t, handler)
	accountID := 7
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_admin_usage_1",
		AccountID:      &accountID,
		SourceEndpoint: "/v1/chat/completions",
		Model:          "usage-model",
		InputTokens:    3,
		OutputTokens:   4,
		TotalTokens:    7,
		Success:        true,
		Cost:           "0.10000000",
		CreatedAt:      time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_admin_usage_2",
		AccountID:      &accountID,
		SourceEndpoint: "/v1/chat/completions",
		Model:          "usage-model",
		InputTokens:    5,
		OutputTokens:   6,
		TotalTokens:    11,
		Success:        false,
		Cost:           "0.20000000",
		CreatedAt:      time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC),
	})

	aggregateReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/aggregates?dimension=model&start=2026-05-22&end=2026-05-22", nil)
	aggregateReq.AddCookie(sessionCookie)
	aggregateRec := httptest.NewRecorder()
	handler.ServeHTTP(aggregateRec, aggregateReq)
	if aggregateRec.Code != http.StatusOK {
		t.Fatalf("expected usage aggregate 200, got %d body=%s", aggregateRec.Code, aggregateRec.Body.String())
	}
	var aggregateResp apiopenapi.UsageAggregateListResponse
	if err := json.NewDecoder(aggregateRec.Body).Decode(&aggregateResp); err != nil {
		t.Fatalf("decode aggregate: %v", err)
	}
	if len(aggregateResp.Data) != 1 || aggregateResp.Data[0].RequestCount != 2 || aggregateResp.Data[0].ErrorCount != 1 || aggregateResp.Data[0].TotalTokens != 18 || aggregateResp.Data[0].TotalCost != "0.30000000" {
		t.Fatalf("unexpected aggregate: %+v", aggregateResp.Data)
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	dashboardReq.AddCookie(sessionCookie)
	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, dashboardReq)
	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d body=%s", dashboardRec.Code, dashboardRec.Body.String())
	}
	var dashboardResp apiopenapi.AdminDashboardResponse
	if err := json.NewDecoder(dashboardRec.Body).Decode(&dashboardResp); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if dashboardResp.Data.TotalRequestCount != 2 || dashboardResp.Data.TotalTokenCount != 18 || dashboardResp.Data.TotalCost != "0.30000000" || dashboardResp.Data.UserCount == 0 {
		t.Fatalf("unexpected dashboard: %+v", dashboardResp.Data)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/export?start=2026-05-22&end=2026-05-22", nil)
	exportReq.AddCookie(sessionCookie)
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected usage export 200, got %d body=%s", exportRec.Code, exportRec.Body.String())
	}
	var exportResp apiopenapi.UsageExportResponse
	if err := json.NewDecoder(exportRec.Body).Decode(&exportResp); err != nil {
		t.Fatalf("decode usage export: %v", err)
	}
	if len(exportResp.Data.Logs) != 2 || len(exportResp.Data.Daily) != 1 || len(exportResp.Data.ByModel) != 1 || len(exportResp.Data.ByAccount) != 1 {
		t.Fatalf("unexpected usage export: %+v", exportResp.Data)
	}

	id, err := strconv.Atoi(string(exportResp.Data.ByAccount[0].AggregateId))
	if err != nil || id != accountID {
		t.Fatalf("unexpected account aggregate id: %+v err=%v", exportResp.Data.ByAccount, err)
	}
}
