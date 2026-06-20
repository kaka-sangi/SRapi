package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
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

func TestAdminProxyRegistrySupportsSOCKS5H(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(`{"name":"remote-dns-egress","type":"socks5h","url":"socks5h://proxy-user:proxy-pass@example.invalid:1080","status":"active"}`))
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
	if created.Data.Name != "remote-dns-egress" || !created.Data.UrlConfigured || created.Data.Type != apiopenapi.Socks5h {
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
	if len(listed.Data) != 1 || listed.Data[0].Type != apiopenapi.Socks5h || !listed.Data[0].UrlConfigured {
		t.Fatalf("unexpected proxy list: %+v", listed.Data)
	}
}

func TestAdminProxyRegistryPaginatesListAndUsesDeleteResponse(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	for _, body := range []string{
		`{"name":"first-egress","type":"http","url":"http://proxy-one.example.invalid:8080","status":"active"}`,
		`{"name":"second-egress","type":"http","url":"http://proxy-two.example.invalid:8080","status":"active"}`,
	} {
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(body))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
		createReq.AddCookie(sessionCookie)
		createRec := httptest.NewRecorder()
		handler.ServeHTTP(createRec, createReq)
		if createRec.Code != http.StatusCreated {
			t.Fatalf("expected proxy create 201, got %d body=%s", createRec.Code, createRec.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxies?page=2&page_size=1", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected proxy list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed apiopenapi.ProxyDefinitionListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list proxy response: %v", err)
	}
	if len(listed.Data) != 1 || listed.Pagination.Page != 2 || listed.Pagination.PageSize != 1 || listed.Pagination.Total != 2 || listed.Pagination.HasNext {
		t.Fatalf("unexpected paginated proxy list: data=%+v pagination=%+v", listed.Data, listed.Pagination)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/proxies/"+string(listed.Data[0].Id), nil)
	deleteReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	deleteReq.AddCookie(sessionCookie)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected proxy delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	var deleted apiopenapi.DeleteResponse
	if err := json.NewDecoder(deleteRec.Body).Decode(&deleted); err != nil {
		t.Fatalf("decode delete proxy response: %v", err)
	}
	if !deleted.Data.Deleted {
		t.Fatalf("expected delete response deleted=true, got %+v", deleted.Data)
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

	missingBindReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/proxy", strings.NewReader(`{"proxy_id":"999"}`))
	missingBindReq.Header.Set("Content-Type", "application/json")
	missingBindReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	missingBindReq.AddCookie(sessionCookie)
	missingBindRec := httptest.NewRecorder()
	handler.ServeHTTP(missingBindRec, missingBindReq)
	if missingBindRec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing proxy_id bind 400, got %d body=%s", missingBindRec.Code, missingBindRec.Body.String())
	}

	missingUpdateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"proxy_id":"999"}`))
	missingUpdateReq.Header.Set("Content-Type", "application/json")
	missingUpdateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	missingUpdateReq.AddCookie(sessionCookie)
	missingUpdateRec := httptest.NewRecorder()
	handler.ServeHTTP(missingUpdateRec, missingUpdateReq)
	if missingUpdateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing proxy_id update 400, got %d body=%s", missingUpdateRec.Code, missingUpdateRec.Body.String())
	}

	disabledProxyReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(`{"name":"disabled-egress","type":"http","url":"http://proxy.example.invalid:8080","status":"disabled"}`))
	disabledProxyReq.Header.Set("Content-Type", "application/json")
	disabledProxyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	disabledProxyReq.AddCookie(sessionCookie)
	disabledProxyRec := httptest.NewRecorder()
	handler.ServeHTTP(disabledProxyRec, disabledProxyReq)
	if disabledProxyRec.Code != http.StatusCreated {
		t.Fatalf("expected disabled proxy create 201, got %d body=%s", disabledProxyRec.Code, disabledProxyRec.Body.String())
	}
	var disabledProxy apiopenapi.ProxyDefinitionResponse
	if err := json.NewDecoder(disabledProxyRec.Body).Decode(&disabledProxy); err != nil {
		t.Fatalf("decode disabled proxy response: %v", err)
	}
	disabledBindReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/proxy", strings.NewReader(`{"proxy_id":"`+string(disabledProxy.Data.Id)+`"}`))
	disabledBindReq.Header.Set("Content-Type", "application/json")
	disabledBindReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	disabledBindReq.AddCookie(sessionCookie)
	disabledBindRec := httptest.NewRecorder()
	handler.ServeHTTP(disabledBindRec, disabledBindReq)
	if disabledBindRec.Code != http.StatusBadRequest {
		t.Fatalf("expected disabled proxy_id bind 400, got %d body=%s", disabledBindRec.Code, disabledBindRec.Body.String())
	}
}

func TestGatewaySkipsAccountWithUnavailableProxy(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gateway-proxy-provider","display_name":"Gateway Proxy Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gateway-proxy-model","display_name":"Gateway Proxy Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gateway-proxy-upstream","status":"active"}`)

	createProxyReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/proxies", strings.NewReader(`{"name":"gateway-disabled-egress","type":"http","url":"http://proxy.example.invalid:8080","status":"active"}`))
	createProxyReq.Header.Set("Content-Type", "application/json")
	createProxyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProxyReq.AddCookie(sessionCookie)
	createProxyRec := httptest.NewRecorder()
	handler.ServeHTTP(createProxyRec, createProxyReq)
	if createProxyRec.Code != http.StatusCreated {
		t.Fatalf("expected proxy create 201, got %d body=%s", createProxyRec.Code, createProxyRec.Body.String())
	}
	var proxyResp apiopenapi.ProxyDefinitionResponse
	if err := json.NewDecoder(createProxyRec.Body).Decode(&proxyResp); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}

	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"gateway-proxy-account","runtime_class":"api_key","credential":{"api_key":"secret"},"proxy_id":"`+string(proxyResp.Data.Id)+`","metadata":{"base_url":"https://upstream.example.invalid/v1"},"status":"active"}`)

	disableProxyReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/proxies/"+string(proxyResp.Data.Id), strings.NewReader(`{"status":"disabled"}`))
	disableProxyReq.Header.Set("Content-Type", "application/json")
	disableProxyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	disableProxyReq.AddCookie(sessionCookie)
	disableProxyRec := httptest.NewRecorder()
	handler.ServeHTTP(disableProxyRec, disableProxyReq)
	if disableProxyRec.Code != http.StatusOK {
		t.Fatalf("expected proxy disable 200, got %d body=%s", disableProxyRec.Code, disableProxyRec.Body.String())
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gateway-proxy-model","messages":[{"role":"user","content":"hi"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected no available account 503, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	if !strings.Contains(chatRec.Body.String(), `"code":"no_available_account"`) {
		t.Fatalf("expected no_available_account body, got %s", chatRec.Body.String())
	}
}

func TestAdminAccountRefresherAdapterMaterializesProxyDefinitionID(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	proxyURL := "http://proxy.example.invalid:8080"
	proxy, err := accounts.CreateProxy(ctx, accountcontract.CreateProxyRequest{
		Name: "refresh-egress",
		Type: accountcontract.ProxyTypeHTTP,
		URL:  proxyURL,
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	proxyID := strconv.Itoa(proxy.ID)
	refresher := &capturingRefresher{credential: map[string]any{"access_token": "new-token"}}
	adapter := adminAccountRefresherAdapter{refresher: refresher, accounts: accounts}
	if _, err := adapter.RefreshAccount(ctx, accountservice.RefreshRequest{
		AccountID:    42,
		RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
		ProxyID:      &proxyID,
		Credential:   map[string]any{"refresh_token": "old-token"},
	}); err != nil {
		t.Fatalf("refresh account: %v", err)
	}
	if refresher.proxyID == nil || *refresher.proxyID != proxyURL {
		t.Fatalf("expected runtime proxy url %q, got %v", proxyURL, refresher.proxyID)
	}
}

type capturingRefresher struct {
	credential map[string]any
	proxyID    *string
}

func (r *capturingRefresher) Refresh(_ context.Context, req reverseproxycontract.RefreshRequest) (reverseproxycontract.RefreshResponse, error) {
	r.proxyID = cloneString(req.Account.ProxyID)
	return reverseproxycontract.RefreshResponse{Credential: r.credential}, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
