package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestApiKeyUsageDrilldownOwnerAndAdminScopes(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys",
		strings.NewReader(`{"name":"usage-key","scopes":["gateway:invoke"]}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(sessionCookie)
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create api key, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	keyID := string(createResp.Data.ApiKey.Id)

	// Owner drilldown (cookie session) returns the key-scoped usage shape.
	ownerReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys/"+keyID+"/usage?days=7", nil)
	ownerReq.AddCookie(sessionCookie)
	ownerRec := httptest.NewRecorder()
	handler.ServeHTTP(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("expected owner usage 200, got %d body=%s", ownerRec.Code, ownerRec.Body.String())
	}
	var usage apiopenapi.GatewayUsageResponse
	if err := json.NewDecoder(ownerRec.Body).Decode(&usage); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if string(usage.ApiKeyId) != keyID || usage.WindowDays != 7 {
		t.Fatalf("unexpected usage envelope: id=%s window=%d", usage.ApiKeyId, usage.WindowDays)
	}

	// Unauthenticated owner route is rejected.
	anonReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys/"+keyID+"/usage", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected anon usage 401, got %d", anonRec.Code)
	}

	// A key id the caller does not own is 404, not a cross-tenant leak.
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys/999999/usage", nil)
	missingReq.AddCookie(sessionCookie)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing key 404, got %d", missingRec.Code)
	}

	// Admin drilldown resolves the same key by id.
	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys/"+keyID+"/usage", nil)
	adminReq.AddCookie(sessionCookie)
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected admin usage 200, got %d body=%s", adminRec.Code, adminRec.Body.String())
	}

	// Admin route requires an authenticated admin session.
	adminAnonReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys/"+keyID+"/usage", nil)
	adminAnonRec := httptest.NewRecorder()
	handler.ServeHTTP(adminAnonRec, adminAnonReq)
	if adminAnonRec.Code != http.StatusForbidden {
		t.Fatalf("expected admin anon 403, got %d", adminAnonRec.Code)
	}
}
