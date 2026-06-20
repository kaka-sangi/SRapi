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

func TestAdminGatewayResourceDeletesUseOpenAPIDeleteResponse(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken

	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"delete-openapi-provider","display_name":"Delete OpenAPI Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"delete-openapi-model","display_name":"Delete OpenAPI Model","status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, csrfToken, `{"name":"delete-openapi-group","description":"Delete OpenAPI Group","status":"active"}`)
	aliasResp := mustCreateModelAliasForDeleteTest(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"alias":"delete-openapi-alias","status":"active"}`)
	mappingResp := mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"delete-openapi-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"delete-openapi-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"status":"active"}`)

	assertAdminDeleteResponseHasOnlyDeleted(t, handler, sessionCookie, csrfToken, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/aliases/"+string(aliasResp.Data.Id), "model alias")
	assertAdminDeleteResponseHasOnlyDeleted(t, handler, sessionCookie, csrfToken, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings/"+string(mappingResp.Data.Id), "model mapping")
	assertAdminDeleteResponseHasOnlyDeleted(t, handler, sessionCookie, csrfToken, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), "provider account")
	assertAdminDeleteResponseHasOnlyDeleted(t, handler, sessionCookie, csrfToken, "/api/v1/admin/models/"+string(modelResp.Data.Id), "model")
	assertAdminDeleteResponseHasOnlyDeleted(t, handler, sessionCookie, csrfToken, "/api/v1/admin/account-groups/"+string(groupResp.Data.Id), "account group")
	assertAdminDeleteResponseHasOnlyDeleted(t, handler, sessionCookie, csrfToken, "/api/v1/admin/providers/"+string(providerResp.Data.Id), "provider")
}

func mustCreateModelAliasForDeleteTest(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, modelID, body string) apiopenapi.ModelAliasResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+modelID+"/aliases", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected model alias create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelAliasResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model alias response: %v", err)
	}
	return resp
}

func assertAdminDeleteResponseHasOnlyDeleted(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, path, name string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s delete 200, got %d body=%s", name, rec.Code, rec.Body.String())
	}

	raw := rec.Body.Bytes()
	var resp apiopenapi.DeleteResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode %s delete response: %v", name, err)
	}
	if !resp.Data.Deleted {
		t.Fatalf("expected %s delete response deleted=true, got %+v", name, resp.Data)
	}

	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode %s delete raw response: %v", name, err)
	}
	if _, ok := envelope.Data["deleted"]; !ok || len(envelope.Data) != 1 {
		t.Fatalf("expected %s delete data to contain only deleted, got %s", name, string(raw))
	}
}
