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
