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

func TestAdminQuickMapModelsUsesPresetDefaultModelMapping(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustInstallProviderPresets(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	provider := mustFindProviderByName(t, handler, sessionCookie, "antigravity")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/quick-map", strings.NewReader(`{"provider_id":"`+string(provider.Id)+`","models":["gemini-3-pro-preview","claude-haiku-4-5"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected quick-map 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	assertQuickMappedUpstreamModel(t, handler, sessionCookie, "gemini-3-pro-preview", "gemini-3-pro-high")
	assertQuickMappedUpstreamModel(t, handler, sessionCookie, "claude-haiku-4-5", "claude-sonnet-4-6")
}

func TestAdminQuickSetupUsesCurrentPresetDefaultModelMappingForExistingProvider(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	provider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity","display_name":"Antigravity","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/quick-setup", strings.NewReader(`{"platform":"antigravity","runtime_class":"custom_reverse_proxy","credential":{"api_key":"antigravity-secret"},"model_catalog":["gemini-3-pro-preview"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected quick-setup 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	updatedProvider := mustFindProviderByName(t, handler, sessionCookie, "antigravity")
	if updatedProvider.Id != provider.Data.Id {
		t.Fatalf("quick setup should reuse existing provider, created=%s updated=%s", provider.Data.Id, updatedProvider.Id)
	}
	assertQuickMappedUpstreamModel(t, handler, sessionCookie, "gemini-3-pro-preview", "gemini-3-pro-high")
}

func TestAdminModelMappingsAllListsMappingsWithPaginationAndStatus(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	provider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"mapping-list-provider","display_name":"Mapping List Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelA := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"mapping-list-a","display_name":"Mapping List A","status":"active"}`)
	modelB := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"mapping-list-b","display_name":"Mapping List B","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelA.Data.Id), `{"provider_id":"`+string(provider.Data.Id)+`","upstream_model_name":"mapping-list-upstream-a","status":"active"}`)
	disabledMapping := mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelB.Data.Id), `{"provider_id":"`+string(provider.Data.Id)+`","upstream_model_name":"mapping-list-upstream-b","status":"disabled"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/model-mappings?status=disabled&page=1&page_size=500", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected model mappings all 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelProviderMappingPagedListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model mappings all: %v", err)
	}
	foundDisabled := false
	for _, mapping := range resp.Data {
		if mapping.Status != apiopenapi.ResourceStatus("disabled") {
			t.Fatalf("expected only disabled mappings, got %+v", resp.Data)
		}
		if mapping.Id == disabledMapping.Data.Id {
			foundDisabled = true
		}
	}
	if !foundDisabled {
		t.Fatalf("expected disabled mapping %s, got %+v", disabledMapping.Data.Id, resp.Data)
	}
	if resp.Pagination.PageSize != 500 || resp.Pagination.Total < 1 {
		t.Fatalf("expected disabled mapping pagination, got %+v", resp.Pagination)
	}
}

func assertQuickMappedUpstreamModel(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, canonicalName string, upstreamModelName string) {
	t.Helper()
	model := mustFindAdminModelByCanonicalName(t, handler, sessionCookie, canonicalName)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/models/"+string(model.Id)+"/mappings", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected model mappings 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelProviderMappingListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model mappings: %v", err)
	}
	for _, mapping := range resp.Data {
		if mapping.UpstreamModelName == upstreamModelName {
			return
		}
	}
	t.Fatalf("expected %s to map to %s, got %+v", canonicalName, upstreamModelName, resp.Data)
}

func mustFindAdminModelByCanonicalName(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, canonicalName string) apiopenapi.Model {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/models", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected model list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model list: %v", err)
	}
	for _, model := range resp.Data {
		if model.CanonicalName == canonicalName {
			return model
		}
	}
	t.Fatalf("model %s not found in %+v", canonicalName, resp.Data)
	return apiopenapi.Model{}
}
