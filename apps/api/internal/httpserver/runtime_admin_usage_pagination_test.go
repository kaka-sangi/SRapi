package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestAdminUsageLogsPagination proves the admin usage-logs endpoint paginates
// server-side (slices to the requested page) and reports the full total — not
// the prior behavior of returning every row with page_size ignored.
func TestAdminUsageLogsPagination(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"pagi-provider","display_name":"Pagi","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"pagi-model","display_name":"Pagi Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"pagi-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pagi-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	const n = 12
	for i := 0; i < n; i++ {
		mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"pagi-model","messages":[{"role":"user","content":"hi"}]}`)
	}

	get := func(query string) apiopenapi.UsageLogListResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs"+query, nil)
		req.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("usage-logs %s: expected 200, got %d body=%s", query, rec.Code, rec.Body.String())
		}
		var resp apiopenapi.UsageLogListResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode usage-logs: %v", err)
		}
		return resp
	}

	// Page 1 of size 5: a full page, total reflects all rows, more pages remain.
	p1 := get("?page=1&page_size=5&model=pagi-model")
	if len(p1.Data) != 5 {
		t.Fatalf("page 1 size 5: expected 5 rows, got %d", len(p1.Data))
	}
	if p1.Pagination.Total != n {
		t.Fatalf("expected total %d, got %d", n, p1.Pagination.Total)
	}
	if !p1.Pagination.HasNext {
		t.Fatalf("expected has_next=true on page 1")
	}

	// Last page: the remainder (12 - 2*5 = 2 rows), no further pages.
	p3 := get("?page=3&page_size=5&model=pagi-model")
	if len(p3.Data) != 2 {
		t.Fatalf("page 3 size 5: expected 2 rows, got %d", len(p3.Data))
	}
	if p3.Pagination.HasNext {
		t.Fatalf("expected has_next=false on the last page")
	}
}
