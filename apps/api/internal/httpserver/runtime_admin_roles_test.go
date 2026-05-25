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

func TestAdminRolePermissionAllowsPaymentOrderReadOnly(t *testing.T) {
	handler := New(config.Load(), nil)
	adminLogin, adminCookie := mustLoginAdmin(t, handler)

	roleReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/roles", strings.NewReader(`{"name":"payment_reader","description":"Payment reader","permissions":["payment_order:read"]}`))
	roleReq.Header.Set("Content-Type", "application/json")
	roleReq.Header.Set("X-CSRF-Token", adminLogin.Data.CsrfToken)
	roleReq.AddCookie(adminCookie)
	roleRec := httptest.NewRecorder()
	handler.ServeHTTP(roleRec, roleReq)
	if roleRec.Code != http.StatusCreated {
		t.Fatalf("expected role create 201, got %d body=%s", roleRec.Code, roleRec.Body.String())
	}

	createUser := func(email, role string) (apiopenapi.LoginResponse, *http.Cookie) {
		t.Helper()
		userReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", strings.NewReader(`{"email":"`+email+`","name":"Reader","password":"password123","roles":["`+role+`"]}`))
		userReq.Header.Set("Content-Type", "application/json")
		userReq.Header.Set("X-CSRF-Token", adminLogin.Data.CsrfToken)
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

	_, readerCookie := createUser("reader@srapi.local", "payment_reader")
	readerReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payments/orders", nil)
	readerReq.AddCookie(readerCookie)
	readerRec := httptest.NewRecorder()
	handler.ServeHTTP(readerRec, readerReq)
	if readerRec.Code != http.StatusOK {
		t.Fatalf("expected payment_reader to list payment orders, got %d body=%s", readerRec.Code, readerRec.Body.String())
	}

	_, userCookie := createUser("plain-user@srapi.local", "user")
	userReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payments/orders", nil)
	userReq.AddCookie(userCookie)
	userRec := httptest.NewRecorder()
	handler.ServeHTTP(userRec, userReq)
	if userRec.Code != http.StatusForbidden {
		t.Fatalf("expected plain user forbidden, got %d body=%s", userRec.Code, userRec.Body.String())
	}
}
