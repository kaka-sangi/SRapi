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

func TestAdminRBACEnforcesPermissionCatalog(t *testing.T) {
	handler := New(config.Load(), nil)
	adminLogin, adminCookie := mustLoginAdmin(t, handler)

	operatorLogin, operatorCookie := mustCreateAdminManagedUser(t, handler, adminLogin.Data.CsrfToken, adminCookie, "operator@srapi.local", "operator")
	userLogin, userCookie := mustCreateAdminManagedUser(t, handler, adminLogin.Data.CsrfToken, adminCookie, "plain-user-rbac@srapi.local", "user")

	catalogReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/permission-catalog", nil)
	catalogReq.AddCookie(operatorCookie)
	catalogRec := httptest.NewRecorder()
	handler.ServeHTTP(catalogRec, catalogReq)
	if catalogRec.Code != http.StatusOK {
		t.Fatalf("expected operator catalog read 200, got %d body=%s", catalogRec.Code, catalogRec.Body.String())
	}
	var catalog apiopenapi.PermissionCatalogResponse
	if err := json.NewDecoder(catalogRec.Body).Decode(&catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(catalog.Data) == 0 {
		t.Fatalf("expected permission catalog items")
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	readReq.AddCookie(operatorCookie)
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected operator user read 200, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(`{"email":"blocked@srapi.local","name":"Blocked","password":"password123","roles":["user"]}`))
	writeReq.Header.Set("Content-Type", "application/json")
	writeReq.Header.Set("X-CSRF-Token", operatorLogin.Data.CsrfToken)
	writeReq.AddCookie(operatorCookie)
	writeRec := httptest.NewRecorder()
	handler.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusForbidden {
		t.Fatalf("expected operator user write forbidden, got %d body=%s", writeRec.Code, writeRec.Body.String())
	}

	plainUserReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	plainUserReq.Header.Set("X-CSRF-Token", userLogin.Data.CsrfToken)
	plainUserReq.AddCookie(userCookie)
	plainUserRec := httptest.NewRecorder()
	handler.ServeHTTP(plainUserRec, plainUserReq)
	if plainUserRec.Code != http.StatusForbidden {
		t.Fatalf("expected plain user forbidden, got %d body=%s", plainUserRec.Code, plainUserRec.Body.String())
	}
}

func mustCreateAdminManagedUser(t *testing.T, handler http.Handler, adminCSRF string, adminCookie *http.Cookie, email, role string) (apiopenapi.LoginResponse, *http.Cookie) {
	t.Helper()
	userReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(`{"email":"`+email+`","name":"RBAC User","password":"password123","roles":["`+role+`"]}`))
	userReq.Header.Set("Content-Type", "application/json")
	userReq.Header.Set("X-CSRF-Token", adminCSRF)
	userReq.AddCookie(adminCookie)
	userRec := httptest.NewRecorder()
	handler.ServeHTTP(userRec, userReq)
	if userRec.Code != http.StatusCreated {
		t.Fatalf("expected user create 201, got %d body=%s", userRec.Code, userRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+email+`","password":"password123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	return loginResp, cookies[0]
}
