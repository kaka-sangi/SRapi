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

func TestAdminProxyRegistryDoesNotExposeRawURL(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(`{"name":"us-east-egress","type":"https","url":"https://proxy-user:proxy-pass@example.invalid:8443","status":"active","metadata":{"region":"us-east"}}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected proxy create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if strings.Contains(createRec.Body.String(), "proxy-pass") {
		t.Fatalf("proxy response leaked raw url: %s", createRec.Body.String())
	}
	var created apiopenapi.ProxyDefinitionResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create proxy response: %v", err)
	}
	if created.Data.Name != "us-east-egress" || !created.Data.UrlConfigured || created.Data.Type != apiopenapi.Https {
		t.Fatalf("unexpected created proxy: %+v", created.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected proxy list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if strings.Contains(listRec.Body.String(), "proxy-pass") {
		t.Fatalf("proxy list leaked raw url: %s", listRec.Body.String())
	}
	var listed apiopenapi.ProxyDefinitionListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list proxy response: %v", err)
	}
	if len(listed.Data) != 1 || listed.Data[0].Id != created.Data.Id || !listed.Data[0].UrlConfigured {
		t.Fatalf("unexpected proxy list: %+v", listed.Data)
	}
}

func TestAdminProxyRegistryBindsAccountByProxyID(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"proxy-provider","display_name":"Proxy Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"proxy-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(`{"name":"bindable-egress","type":"http","url":"http://proxy.example.invalid:8080","status":"active"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected proxy create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var proxyResp apiopenapi.ProxyDefinitionResponse
	if err := json.NewDecoder(createRec.Body).Decode(&proxyResp); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}

	bindReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/proxy", strings.NewReader(`{"proxy_id":"`+string(proxyResp.Data.Id)+`"}`))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	bindReq.AddCookie(sessionCookie)
	bindRec := httptest.NewRecorder()
	handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("expected proxy bind 200, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}

	qualityReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/proxy-quality", nil)
	qualityReq.AddCookie(sessionCookie)
	qualityRec := httptest.NewRecorder()
	handler.ServeHTTP(qualityRec, qualityReq)
	if qualityRec.Code != http.StatusOK {
		t.Fatalf("expected proxy quality 200, got %d body=%s", qualityRec.Code, qualityRec.Body.String())
	}
	var quality apiopenapi.AccountProxyQualityResponse
	if err := json.NewDecoder(qualityRec.Body).Decode(&quality); err != nil {
		t.Fatalf("decode proxy quality response: %v", err)
	}
	if quality.Data.ProxyId == nil || *quality.Data.ProxyId != string(proxyResp.Data.Id) {
		t.Fatalf("expected account bound to proxy id %s, got %+v", proxyResp.Data.Id, quality.Data.ProxyId)
	}
}

func TestAdminAccountRejectsRawProxyIDValues(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"proxy-reject-provider","display_name":"Proxy Reject Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	rawProxyURL := "https://proxy-user:proxy-pass@example.invalid:8443"

	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","name":"raw-proxy-account","runtime_class":"api_key","credential":{"api_key":"secret"},"proxy_id":"`+rawProxyURL+`","status":"active"}`))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountReq.AddCookie(sessionCookie)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusBadRequest {
		t.Fatalf("expected raw proxy_id create 400, got %d body=%s", createAccountRec.Code, createAccountRec.Body.String())
	}

	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"proxy-reject-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)
	updateAccountReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"proxy_id":"proxy-us"}`))
	updateAccountReq.Header.Set("Content-Type", "application/json")
	updateAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateAccountReq.AddCookie(sessionCookie)
	updateAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(updateAccountRec, updateAccountReq)
	if updateAccountRec.Code != http.StatusBadRequest {
		t.Fatalf("expected non-numeric proxy_id update 400, got %d body=%s", updateAccountRec.Code, updateAccountRec.Body.String())
	}

	bindReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/proxy", strings.NewReader(`{"proxy_id":"`+rawProxyURL+`"}`))
	bindReq.Header.Set("Content-Type", "application/json")
	bindReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	bindReq.AddCookie(sessionCookie)
	bindRec := httptest.NewRecorder()
	handler.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusBadRequest {
		t.Fatalf("expected raw proxy_id bind 400, got %d body=%s", bindRec.Code, bindRec.Body.String())
	}
}
