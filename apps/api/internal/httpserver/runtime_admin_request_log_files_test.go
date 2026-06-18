package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestAdminRequestLogFiles_ListGetDownloadDelete walks the four admin
// endpoints against a directory pre-populated with two captured files.
// We avoid the full gateway round-trip — the writer's behaviour is covered
// by the module-level tests — and focus on the admin layer: filename
// projection, error_only filter, raw download, and deletion.
func TestAdminRequestLogFiles_ListGetDownloadDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SRAPI_REQUEST_LOG_DIR", dir)
	t.Setenv("SRAPI_REQUEST_LOG_ENABLED", "false")

	now := time.Now().UTC()
	okName := "request-1000-req_ok.log"
	errName := "error-2000-req_err.log"
	if err := os.WriteFile(filepath.Join(dir, okName), []byte("OK BODY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, errName), []byte("ERR BODY"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, okName), now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, errName), now.Add(-1*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	// List without filters: newest first, both files present.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files", nil)
	listReq.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, listReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d body=%s", rec.Code, rec.Body.String())
	}
	var list struct {
		Data       []map[string]any      `json:"data"`
		Pagination apiopenapi.Pagination `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Data) != 2 {
		t.Fatalf("expected 2 files, got %d (%+v)", len(list.Data), list.Data)
	}
	if list.Data[0]["name"] != errName {
		t.Fatalf("expected newer error file first, got %v", list.Data[0]["name"])
	}
	if list.Pagination.Total != 2 || list.Pagination.PageSize != 100 || list.Pagination.HasNext {
		t.Fatalf("unexpected list pagination: %+v", list.Pagination)
	}

	// Limit truncates the payload but keeps the real filtered total so the
	// operator knows more evidence exists on disk.
	limitedReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files?limit=1", nil)
	limitedReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, limitedReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("list limit: got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Pagination.Total != 2 || list.Pagination.PageSize != 1 || !list.Pagination.HasNext {
		t.Fatalf("limit should keep real total and has_next; got data=%d pagination=%+v", len(list.Data), list.Pagination)
	}

	// List with error_only=true: just the error file.
	listErrReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files?error_only=true", nil)
	listErrReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, listErrReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("list error_only: got %d", rec.Code)
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0]["name"] != errName {
		t.Fatalf("error_only=true should return only error file; got %+v", list.Data)
	}
	if list.Data[0]["is_error_only"] != true {
		t.Fatalf("expected is_error_only=true on error file, got %v", list.Data[0]["is_error_only"])
	}

	// List by request_id prefix: this is the correlation path from
	// ops_error_logs.request_id to the matching raw HTTP envelope dump.
	listByRequestReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files?request_id=req_o", nil)
	listByRequestReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, listByRequestReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("list by request_id: got %d", rec.Code)
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0]["name"] != okName {
		t.Fatalf("request_id prefix should return only matching file; got %+v", list.Data)
	}

	invalidTimeReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files?from=not-a-time", nil)
	invalidTimeReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, invalidTimeReq)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid from timestamp should be rejected, got %d body=%s", rec.Code, rec.Body.String())
	}

	invalidLimitReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files?limit=501", nil)
	invalidLimitReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, invalidLimitReq)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range limit should be rejected, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Get by name.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files/"+okName, nil)
	getReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, getReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: got %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Data["request_id"] != "req_ok" {
		t.Fatalf("expected request_id=req_ok, got %v", got.Data["request_id"])
	}

	// Download raw bytes.
	dlReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files/"+okName+"/download", nil)
	dlReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, dlReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "OK BODY" {
		t.Fatalf("download body mismatch: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain, got %q", ct)
	}

	// Get with an unknown name → 404.
	missReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files/request-9999-nope.log", nil)
	missReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, missReq)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on unknown file, got %d", rec.Code)
	}

	// Delete is a state-changing admin operation and must reject missing CSRF.
	delNoCSRFReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/request-log-files/"+errName, nil)
	delNoCSRFReq.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, delNoCSRFReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete without csrf should be forbidden, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Delete the error file.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/request-log-files/"+errName, nil)
	delReq.AddCookie(sessionCookie)
	delReq.Header.Set("X-CSRF-Token", csrf)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, delReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, errName)); !os.IsNotExist(err) {
		t.Fatalf("expected error file removed, stat err=%v", err)
	}
}

// TestAdminRequestLogFiles_RequiresAdmin asserts the surface is gated by
// the admin session middleware (anonymous callers receive 403).
func TestAdminRequestLogFiles_RequiresAdmin(t *testing.T) {
	t.Setenv("SRAPI_REQUEST_LOG_DIR", t.TempDir())
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/request-log-files", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 403/401 for anonymous caller, got %d body=%s", rec.Code, rec.Body.String())
	}
}
