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

func TestAdminTLSProfilesUseOpenAPIWireTypesAndExpandEgressProfile(t *testing.T) {
	handler, srv := newWithServer(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	createResp := createAdminTLSProfile(t, handler, sessionCookie, csrfToken, `{"name":"chrome-egress","tls_template":"chrome","http_version_policy":"prefer_h2","user_agent":"SRapi Test UA","extra_headers":{"X-Test-Header":"enabled"},"enabled":true}`)
	if createResp.Data.Id == 0 ||
		createResp.Data.Name != "chrome-egress" ||
		createResp.Data.TlsTemplate != "chrome" ||
		createResp.Data.HttpVersionPolicy != "prefer_h2" ||
		createResp.Data.UserAgent != "SRapi Test UA" ||
		createResp.Data.ExtraHeaders["X-Test-Header"] != "enabled" ||
		!createResp.Data.Enabled {
		t.Fatalf("unexpected tls profile create response: %+v", createResp.Data)
	}

	expanded, ok := srv.runtime.expandEgressProfileMetadata(map[string]any{"tls_profile": "chrome-egress"})
	if !ok {
		t.Fatalf("expected tls profile reference to expand")
	}
	egressProfile, ok := expanded["egress_profile"].(map[string]any)
	if !ok {
		t.Fatalf("expected egress_profile map, got %+v", expanded)
	}
	if egressProfile["tls_template"] != "chrome" ||
		egressProfile["http_version_policy"] != "prefer_h2" ||
		egressProfile["user_agent"] != "SRapi Test UA" {
		t.Fatalf("unexpected expanded egress profile: %+v", egressProfile)
	}
	headers, ok := egressProfile["extra_static_headers"].(map[string]any)
	if !ok || headers["X-Test-Header"] != "enabled" {
		t.Fatalf("unexpected expanded headers: %+v", egressProfile)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tls-profiles?page=1&page_size=10", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected tls profile list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.TLSProfileListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode tls profile list response: %v", err)
	}
	if listResp.Pagination.Total != 1 || !tlsProfileListHasName(listResp.Data, "chrome-egress") {
		t.Fatalf("unexpected tls profile list: %+v pagination=%+v", listResp.Data, listResp.Pagination)
	}

	updateResp := updateAdminTLSProfile(t, handler, sessionCookie, csrfToken, createResp.Data.Id, `{"enabled":false}`)
	if updateResp.Data.Enabled ||
		updateResp.Data.TlsTemplate != "chrome" ||
		updateResp.Data.HttpVersionPolicy != "prefer_h2" ||
		updateResp.Data.UserAgent != "SRapi Test UA" {
		t.Fatalf("unexpected tls profile update response: %+v", updateResp.Data)
	}
	if _, ok := srv.runtime.expandEgressProfileMetadata(map[string]any{"tls_profile": "chrome-egress"}); ok {
		t.Fatalf("disabled tls profile should not expand")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tls-profiles/"+strconv.FormatInt(createResp.Data.Id, 10), nil)
	deleteReq.Header.Set("X-CSRF-Token", csrfToken)
	deleteReq.AddCookie(sessionCookie)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected tls profile delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	var deleteResp apiopenapi.DeleteResponse
	if err := json.NewDecoder(deleteRec.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("decode tls profile delete response: %v", err)
	}
	if !deleteResp.Data.Deleted {
		t.Fatalf("unexpected tls profile delete response: %+v", deleteResp.Data)
	}
}

func createAdminTLSProfile(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.TLSProfileResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tls-profiles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected tls profile create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.TLSProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode tls profile create response: %v", err)
	}
	return resp
}

func updateAdminTLSProfile(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, profileID int64, body string) apiopenapi.TLSProfileResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/tls-profiles/"+strconv.FormatInt(profileID, 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected tls profile update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.TLSProfileResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode tls profile update response: %v", err)
	}
	return resp
}

func tlsProfileListHasName(items []apiopenapi.TLSProfile, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
