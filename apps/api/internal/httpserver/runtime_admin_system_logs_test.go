package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminOpsSystemLogsListAndCleanup(t *testing.T) {
	operationsStore := operationsmemory.New()
	handler := New(config.Load(), nil, WithOperationsStore(operationsStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, err := operationsStore.CreateSystemLog(context.Background(), operationscontract.OpsSystemLog{
		Level:     operationscontract.OpsSystemLogLevelWarn,
		Source:    "ops.dashboard",
		Message:   "rotate logs",
		RequestID: "req_cleanup",
		TraceID:   "trace_cleanup",
		CreatedAt: time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("seed system log: %v", err)
	}
	_, err = operationsStore.CreateSystemLog(context.Background(), operationscontract.OpsSystemLog{
		Level:     operationscontract.OpsSystemLogLevelError,
		Source:    "ops.dashboard",
		Message:   "rotate logs failed",
		RequestID: "req_cleanup",
		TraceID:   "trace_other",
		CreatedAt: time.Now().UTC().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed second system log: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/system-logs?level=warn&q=rotate", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.OpsSystemLogListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].RequestId == nil || *listResp.Data[0].RequestId != "req_cleanup" {
		t.Fatalf("unexpected system log list response: %+v", listResp.Data)
	}

	invalidStartReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/system-logs?start=not-a-timestamp", nil)
	invalidStartReq.AddCookie(sessionCookie)
	invalidStartRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidStartRec, invalidStartReq)
	if invalidStartRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid start to fail with 400, got %d body=%s", invalidStartRec.Code, invalidStartRec.Body.String())
	}

	invalidEndReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/system-logs?end=not-a-timestamp", nil)
	invalidEndReq.AddCookie(sessionCookie)
	invalidEndRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidEndRec, invalidEndReq)
	if invalidEndRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid end to fail with 400, got %d body=%s", invalidEndRec.Code, invalidEndRec.Body.String())
	}

	exactReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/system-logs?request_id=req_cleanup&trace_id=trace_cleanup", nil)
	exactReq.AddCookie(sessionCookie)
	exactRec := httptest.NewRecorder()
	handler.ServeHTTP(exactRec, exactReq)
	if exactRec.Code != http.StatusOK {
		t.Fatalf("expected exact list 200, got %d body=%s", exactRec.Code, exactRec.Body.String())
	}
	var exactResp apiopenapi.OpsSystemLogListResponse
	if err := json.NewDecoder(exactRec.Body).Decode(&exactResp); err != nil {
		t.Fatalf("decode exact list response: %v", err)
	}
	if len(exactResp.Data) != 1 || exactResp.Data[0].TraceId == nil || *exactResp.Data[0].TraceId != "trace_cleanup" {
		t.Fatalf("unexpected request/trace filtered response: %+v", exactResp.Data)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/system-logs/health", nil)
	healthReq.AddCookie(sessionCookie)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d body=%s", healthRec.Code, healthRec.Body.String())
	}
	var healthResp apiopenapi.OpsSystemLogHealthResponse
	if err := json.NewDecoder(healthRec.Body).Decode(&healthResp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if healthResp.Data.StorageMode != "durable" || !healthResp.Data.Writable || healthResp.Data.TotalCount != 2 || healthResp.Data.Stale {
		t.Fatalf("unexpected health response: %+v", healthResp.Data)
	}

	cleanupReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ops/system-logs/cleanup", strings.NewReader(`{"source":"ops.dashboard","q":"rotate","request_id":"req_cleanup","trace_id":"trace_cleanup","dry_run":true,"max_delete":1}`))
	cleanupReq.Header.Set("Content-Type", "application/json")
	cleanupReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	cleanupReq.AddCookie(sessionCookie)
	cleanupRec := httptest.NewRecorder()
	handler.ServeHTTP(cleanupRec, cleanupReq)
	if cleanupRec.Code != http.StatusOK {
		t.Fatalf("expected dry-run cleanup 200, got %d body=%s", cleanupRec.Code, cleanupRec.Body.String())
	}
	var cleanupResp apiopenapi.OpsSystemLogCleanupResponse
	if err := json.NewDecoder(cleanupRec.Body).Decode(&cleanupResp); err != nil {
		t.Fatalf("decode cleanup response: %v", err)
	}
	if !cleanupResp.Data.DryRun || cleanupResp.Data.Matched != 1 || cleanupResp.Data.Deleted != 0 {
		t.Fatalf("unexpected cleanup response: %+v", cleanupResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	audit := mustFindAuditLog(t, auditResp.Data, "ops_system_log.cleanup")
	if _, ok := audit.After["q"]; ok {
		t.Fatalf("cleanup audit must not expose raw query strings: %+v", audit.After)
	}
	if audit.After["request_id"] != "req_cleanup" || audit.After["trace_id"] != "trace_cleanup" {
		t.Fatalf("cleanup audit must include exact correlation filters: %+v", audit.After)
	}
}
