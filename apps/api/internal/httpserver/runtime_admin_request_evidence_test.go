package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminRequestEvidence_ListMergesUsageOpsErrorsAndDumps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SRAPI_REQUEST_LOG_DIR", dir)
	t.Setenv("SRAPI_REQUEST_LOG_ENABLED", "false")

	base := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	usageStore := usagememory.New()
	errorClass := "server_bad"
	accountID := 9
	providerID := 3
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:             "req_merge",
		AttemptNo:             1,
		UserID:                42,
		APIKeyID:              7,
		AccountID:             &accountID,
		ProviderID:            &providerID,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/chat/completions",
		TargetProtocol:        "openai",
		Model:                 "merge-model",
		InputTokens:           11,
		OutputTokens:          13,
		TotalTokens:           24,
		UsageEstimated:        true,
		LatencyMS:             900,
		Success:               false,
		ErrorClass:            &errorClass,
		ProviderErrorMessage:  "upstream failed",
		StatusCode:            503,
		UpstreamRequestID:     "up_req_merge",
		ErrorPhase:            "upstream",
		ErrorOwner:            "provider",
		ErrorSource:           "upstream_http",
		Cost:                  "0.00000000",
		Currency:              "USD",
		CompatibilityWarnings: []string{},
		CreatedAt:             base.Add(-20 * time.Minute),
		CacheCreationTokens:   0,
		CachedTokens:          0,
		ActualCost:            "0.00000000",
		RateMultiplier:        "1.00000000",
		BillableCost:          "0.00000000",
		InputCost:             "0.00000000",
		OutputCost:            "0.00000000",
		CacheReadCost:         "0.00000000",
		CacheWriteCost:        "0.00000000",
		RequestedModel:        "merge-model",
		UpstreamModel:         "merge-upstream",
		BillingMode:           "token",
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:             "req_success",
		AttemptNo:             1,
		UserID:                42,
		APIKeyID:              7,
		AccountID:             &accountID,
		ProviderID:            &providerID,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/responses",
		TargetProtocol:        "openai",
		Model:                 "success-model",
		InputTokens:           3,
		OutputTokens:          5,
		TotalTokens:           8,
		LatencyMS:             120,
		Success:               true,
		Cost:                  "0.01000000",
		Currency:              "USD",
		CompatibilityWarnings: []string{},
		CreatedAt:             base.Add(-10 * time.Minute),
	})

	opsStore := opserrorlogsmemory.New()
	opsSvc, err := opserrorlogsservice.New(opsStore, func() time.Time { return base })
	if err != nil {
		t.Fatal(err)
	}
	status := 503
	if err := opsSvc.RecordError(t.Context(), opserrorlogscontract.RecordRequest{
		OccurredAt:        base.Add(-19 * time.Minute),
		RequestID:         "req_merge",
		UserID:            intPtr(42),
		APIKeyID:          intPtr(7),
		AccountID:         &accountID,
		ProviderID:        &providerID,
		Platform:          "openai-compatible",
		SourceEndpoint:    "/v1/chat/completions",
		TargetProtocol:    "openai",
		Model:             "merge-model",
		StatusCode:        &status,
		UpstreamRequestID: "up_req_merge",
		AttemptNo:         1,
		LatencyMS:         905,
		InputTokens:       11,
		OutputTokens:      13,
		UsageEstimated:    true,
		ErrorClass:        "server_bad",
		ErrorPhase:        "upstream",
		ErrorOwner:        "provider",
		ErrorSource:       "upstream_http",
		ErrorMessage:      "ops upstream failed",
	}); err != nil {
		t.Fatal(err)
	}

	mergeDumpName := "error-" + strconv.FormatInt(base.Add(-18*time.Minute).UnixMilli(), 10) + "-req_merge.log"
	dumpOnlyName := "request-" + strconv.FormatInt(base.Add(-5*time.Minute).UnixMilli(), 10) + "-req_dump_only.log"
	writeRequestEvidenceDump(t, dir, mergeDumpName, "req_merge", false, 503, "server_bad", 910, base.Add(-18*time.Minute))
	writeRequestEvidenceDump(t, dir, dumpOnlyName, "req_dump_only", true, 200, "", 44, base.Add(-5*time.Minute))

	handler := New(config.Load(), nil, WithUsageStore(usageStore), WithOpsErrorLogsStore(opsStore))
	_, sessionCookie := mustLoginAdmin(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence?start=2026-06-19T07:00:00Z&end=2026-06-19T09:00:00Z&page_size=10", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list request evidence: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.RequestEvidenceListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Pagination.Total != 3 || len(resp.Data) != 3 {
		t.Fatalf("expected 3 evidence rows, got total=%d data=%d (%+v)", resp.Pagination.Total, len(resp.Data), resp.Data)
	}
	byRequest := map[string]apiopenapi.RequestEvidenceRow{}
	for _, row := range resp.Data {
		byRequest[row.RequestId] = row
	}
	merged := byRequest["req_merge"]
	if merged.RequestId == "" {
		t.Fatalf("missing merged request row: %+v", resp.Data)
	}
	if merged.Kind != apiopenapi.RequestEvidenceKindError || merged.EvidenceSource != apiopenapi.RequestEvidenceSourceUsage {
		t.Fatalf("merged row should be usage-backed error, got kind=%s source=%s", merged.Kind, merged.EvidenceSource)
	}
	if !merged.HasUsageLog || !merged.HasOpsErrorLog || !merged.HasRequestDump {
		t.Fatalf("merged row should carry all evidence flags: %+v", merged)
	}
	if merged.UsageLogId == nil || merged.OpsErrorLogId == nil || merged.LatestRequestDumpName == nil || *merged.LatestRequestDumpName != mergeDumpName {
		t.Fatalf("merged row missing ids/dump name: %+v", merged)
	}
	if merged.RequestDumpCount != 1 || merged.RequestDumpErrorCount != 1 {
		t.Fatalf("merged row dump counts mismatch: %+v", merged)
	}

	success := byRequest["req_success"]
	if success.Kind != apiopenapi.RequestEvidenceKindSuccess || !success.HasUsageLog || success.HasOpsErrorLog || success.HasRequestDump {
		t.Fatalf("success row mismatch: %+v", success)
	}

	dumpOnly := byRequest["req_dump_only"]
	if dumpOnly.Kind != apiopenapi.RequestEvidenceKindSuccess || dumpOnly.EvidenceSource != apiopenapi.RequestEvidenceSourceRequestDump || !dumpOnly.HasRequestDump || dumpOnly.HasUsageLog {
		t.Fatalf("dump-only row mismatch: %+v", dumpOnly)
	}

	errorReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence?start=2026-06-19T07:00:00Z&end=2026-06-19T09:00:00Z&kind=error", nil)
	errorReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, errorReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("error filter: got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 || resp.Data[0].RequestId != "req_merge" {
		t.Fatalf("kind=error should return only req_merge, got %+v", resp.Data)
	}

	sourceReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence?start=2026-06-19T07:00:00Z&end=2026-06-19T09:00:00Z&evidence_source=ops_error", nil)
	sourceReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, sourceReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("evidence_source filter: got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 || resp.Data[0].RequestId != "req_merge" || !resp.Data[0].HasUsageLog {
		t.Fatalf("evidence_source=ops_error should keep the merged usage row, got %+v", resp.Data)
	}
}

func TestAdminRequestEvidence_RejectsAnonymousAndInvalidQuery(t *testing.T) {
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous request rejected, got %d body=%s", rec.Code, rec.Body.String())
	}

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_ = loginResp
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence?start=2026-06-19T09:00:00Z&end=2026-06-19T07:00:00Z", nil)
	badReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, badReq)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid window rejected, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminRequestEvidence_DetailUsesExactHistoricalRequestID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SRAPI_REQUEST_LOG_DIR", dir)
	t.Setenv("SRAPI_REQUEST_LOG_ENABLED", "false")

	base := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	usageStore := usagememory.New()
	errorClass := "timeout"
	accountID := 12
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:             "req_historical",
		AttemptNo:             1,
		UserID:                44,
		APIKeyID:              8,
		AccountID:             &accountID,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/responses",
		TargetProtocol:        "openai",
		Model:                 "detail-model",
		InputTokens:           4,
		OutputTokens:          6,
		TotalTokens:           10,
		LatencyMS:             1500,
		Success:               false,
		ErrorClass:            &errorClass,
		ProviderErrorMessage:  "timed out upstream",
		StatusCode:            504,
		Cost:                  "0.00000000",
		Currency:              "USD",
		CompatibilityWarnings: []string{},
		CreatedAt:             base.Add(-48 * time.Hour),
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:             "req_historical_neighbor",
		AttemptNo:             1,
		UserID:                44,
		APIKeyID:              8,
		AccountID:             &accountID,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/responses",
		TargetProtocol:        "openai",
		Model:                 "detail-model",
		InputTokens:           1,
		OutputTokens:          1,
		TotalTokens:           2,
		LatencyMS:             20,
		Success:               true,
		Cost:                  "0.00000000",
		Currency:              "USD",
		CompatibilityWarnings: []string{},
		CreatedAt:             base.Add(-48 * time.Hour),
	})

	opsStore := opserrorlogsmemory.New()
	opsSvc, err := opserrorlogsservice.New(opsStore, func() time.Time { return base })
	if err != nil {
		t.Fatal(err)
	}
	status := 504
	if err := opsSvc.RecordError(t.Context(), opserrorlogscontract.RecordRequest{
		OccurredAt:     base.Add(-48*time.Hour + time.Minute),
		RequestID:      "req_historical",
		UserID:         intPtr(44),
		APIKeyID:       intPtr(8),
		AccountID:      &accountID,
		Platform:       "openai-compatible",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai",
		Model:          "detail-model",
		StatusCode:     &status,
		AttemptNo:      1,
		LatencyMS:      1510,
		ErrorClass:     "timeout",
		ErrorMessage:   "ops timeout",
	}); err != nil {
		t.Fatal(err)
	}

	dumpName := "error-" + strconv.FormatInt(base.Add(-48*time.Hour+2*time.Minute).UnixMilli(), 10) + "-req_historical.log"
	writeRequestEvidenceDump(t, dir, dumpName, "req_historical", false, 504, "timeout", 1515, base.Add(-48*time.Hour+2*time.Minute))
	for i := 0; i < 60; i++ {
		createdAt := base.Add(-47*time.Hour + time.Duration(i)*time.Second)
		neighborID := "req_historical_neighbor_" + strconv.Itoa(i)
		neighborDump := "error-" + strconv.FormatInt(createdAt.UnixMilli(), 10) + "-" + neighborID + ".log"
		writeRequestEvidenceDump(t, dir, neighborDump, neighborID, false, 500, "other", 20, createdAt)
	}

	handler := New(config.Load(), nil, WithUsageStore(usageStore), WithOpsErrorLogsStore(opsStore))
	_, sessionCookie := mustLoginAdmin(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence/req_historical", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail request evidence: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.RequestEvidenceDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.EvidenceRequestId != "req_historical" {
		t.Fatalf("wrong evidence request id: %+v", resp)
	}
	if resp.Summary.Kind != apiopenapi.RequestEvidenceKindError || resp.Summary.UsageLogCount != 1 || resp.Summary.OpsErrorLogCount != 1 || resp.Summary.RequestDumpCount != 1 {
		t.Fatalf("summary mismatch: %+v", resp.Summary)
	}
	if len(resp.Attempts) != 1 || !resp.Attempts[0].HasUsageLog || !resp.Attempts[0].HasOpsErrorLog || !resp.Attempts[0].HasRequestDump {
		t.Fatalf("attempt evidence mismatch: %+v", resp.Attempts)
	}
	if len(resp.RequestDumps) != 1 || resp.RequestDumps[0].Name != dumpName {
		t.Fatalf("dump exact filter mismatch: %+v", resp.RequestDumps)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/request-evidence/req_missing", nil)
	missingReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, missingReq)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing evidence should return 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func writeRequestEvidenceDump(t *testing.T, dir, name, requestID string, success bool, status int, errorClass string, latency int, createdAt time.Time) {
	t.Helper()
	body := "=== REQUEST INFO ===\n" +
		"Request-ID: " + requestID + "\n" +
		"User-ID: 42\n" +
		"API-Key-ID: 7\n" +
		"Account-ID: 9\n" +
		"Source-Protocol: openai-compatible\n" +
		"Source-Endpoint: /v1/chat/completions\n" +
		"Started-At: " + createdAt.Add(-time.Second).Format(time.RFC3339) + "\n\n" +
		"=== REQUEST 1 ===\nPOST https://upstream.invalid/v1/chat/completions\n\n" +
		"=== RESPONSE 1 ===\nStatus: " + strconv.Itoa(status) + "\n\n" +
		"=== SUMMARY ===\n" +
		"Success: " + strconv.FormatBool(success) + "\n" +
		"Status: " + strconv.Itoa(status) + "\n" +
		"Latency-MS: " + strconv.Itoa(latency) + "\n"
	if errorClass != "" {
		body += "Error-Class: " + errorClass + "\n"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, createdAt, createdAt); err != nil {
		t.Fatal(err)
	}
}

func intPtr(value int) *int {
	return &value
}
