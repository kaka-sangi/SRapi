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

func setupStatusNeedsSetup(t *testing.T, handler http.Handler) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.SetupStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	return resp.Data.NeedsSetup
}

func TestSetupWizardFlow(t *testing.T) {
	cfg := config.Load()
	cfg.Bootstrap.AdminEmail = ""
	cfg.Bootstrap.AdminPassword = ""
	handler := New(cfg, nil)

	if !setupStatusNeedsSetup(t, handler) {
		t.Fatal("expected needs_setup=true on a fresh system without a bootstrap admin")
	}

	body := `{"email":"owner@example.com","name":"Owner","password":"sup3rsecret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("complete setup expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	if setupStatusNeedsSetup(t, handler) {
		t.Fatal("expected needs_setup=false after the owner account is created")
	}

	// A second attempt must be rejected once setup is complete.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second setup attempt expected 409, got %d body=%s", rec2.Code, rec2.Body.String())
	}

	// The created owner can sign in.
	loginBody := `{"email":"owner@example.com","password":"sup3rsecret"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("owner login expected 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
}

func TestSetupStatusFalseWhenBootstrapAdminExists(t *testing.T) {
	handler := New(config.Load(), nil) // default bootstrap provisions an admin
	if setupStatusNeedsSetup(t, handler) {
		t.Fatal("expected needs_setup=false when a bootstrap admin already exists")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(`{"email":"x@example.com","name":"X","password":"sup3rsecret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("setup on a provisioned system expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}
