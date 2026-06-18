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

// TestAdminAccountBulkUpdateExplicitIDs pins the explicit-IDs path:
// caller supplies account_ids and a subset of fields, every selected
// account picks up only those fields, others stay untouched. Mirrors
// sub2api `BulkUpdate` parity — every editable field is optional and
// the response carries per-row updated_ids + errors.
func TestAdminAccountBulkUpdateExplicitIDs(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"bulk-update-provider","display_name":"Bulk Update Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	a := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"bulk-a","runtime_class":"api_key","credential":{"api_key":"k-a"},"status":"active","priority":1}`)
	b := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"bulk-b","runtime_class":"api_key","credential":{"api_key":"k-b"},"status":"active","priority":1}`)

	body := `{"account_ids":["` + string(a.Data.Id) + `","` + string(b.Data.Id) + `"],"priority":7,"max_concurrency":4}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode bulk-update: %v", err)
	}
	if resp.Data.UpdatedCount != 2 || len(resp.Data.UpdatedIds) != 2 || len(resp.Data.Errors) != 0 {
		t.Fatalf("unexpected bulk-update result: %+v", resp.Data)
	}

	// Verify priority + metadata.max_concurrency landed; status untouched.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(a.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get 200, got %d", getRec.Code)
	}
	var got apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Data.Priority != 7 {
		t.Fatalf("expected priority=7, got %d", got.Data.Priority)
	}
	if got.Data.Status != apiopenapi.ProviderAccountStatusActive {
		t.Fatalf("expected status untouched (active), got %s", got.Data.Status)
	}
	if got.Data.Metadata == nil || (*got.Data.Metadata)["max_concurrency"] == nil {
		t.Fatalf("expected metadata.max_concurrency set, got %+v", got.Data.Metadata)
	}
}

// TestAdminAccountBulkUpdateFiltersByStatus pins the server-side filter
// resolution path used by the "Edit Filtered" UI shortcut: caller passes
// `filters.status` instead of explicit IDs and the handler resolves the
// matching accounts via the same list path the admin table uses, then
// applies the field updates. Accounts outside the filter MUST NOT be
// touched — the empty-filter guard in resolveBulkUpdateTargets is the
// safety belt against an accidental whole-fleet rewrite.
func TestAdminAccountBulkUpdateFiltersByStatus(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"bulk-filter-provider","display_name":"Bulk Filter Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	disabledOne := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"filter-disabled-1","runtime_class":"api_key","credential":{"api_key":"d1"},"status":"disabled"}`)
	disabledTwo := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"filter-disabled-2","runtime_class":"api_key","credential":{"api_key":"d2"},"status":"disabled"}`)
	activeOne := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"filter-active-1","runtime_class":"api_key","credential":{"api_key":"a1"},"status":"active"}`)

	body := `{"filters":{"status":"disabled","provider_id":"` + string(providerResp.Data.Id) + `"},"status":"active"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode bulk-update: %v", err)
	}
	if resp.Data.UpdatedCount != 2 {
		t.Fatalf("expected 2 disabled accounts reactivated, got %d (ids=%v errors=%v)", resp.Data.UpdatedCount, resp.Data.UpdatedIds, resp.Data.Errors)
	}
	// The already-active account must NOT appear in the updated list.
	for _, id := range resp.Data.UpdatedIds {
		if id == activeOne.Data.Id {
			t.Fatalf("filter=disabled must not include active account, got %s in %v", id, resp.Data.UpdatedIds)
		}
	}
	gotIDs := map[apiopenapi.Id]bool{}
	for _, id := range resp.Data.UpdatedIds {
		gotIDs[id] = true
	}
	if !gotIDs[disabledOne.Data.Id] || !gotIDs[disabledTwo.Data.Id] {
		t.Fatalf("expected both disabled accounts in updated list, got %v", resp.Data.UpdatedIds)
	}
}

// TestAdminAccountBulkUpdateEmptyFiltersRejected guards against a
// foot-gun: empty filters MUST NOT silently match every account in the
// system. sub2api parity (its bulk-update also refuses an empty filter
// set). Returns 400 instead of running a fleet-wide rewrite.
func TestAdminAccountBulkUpdateEmptyFiltersRejected(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	body := `{"filters":{},"status":"disabled"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected empty-filter rejection 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminAccountBulkUpdateNoFieldsRejected pins the no-op guard: a
// caller that supplies a target selection but no editable fields MUST
// get a 400 instead of a silent successful response with zero
// observable effect. Catches UI bugs that submit an empty form.
func TestAdminAccountBulkUpdateNoFieldsRejected(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"bulk-empty-provider","display_name":"Bulk Empty","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	a := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"bulk-empty","runtime_class":"api_key","credential":{"api_key":"x"},"status":"active"}`)
	body := `{"account_ids":["` + string(a.Data.Id) + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected no-fields rejection 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminAccountBulkUpdateAdminGating ensures the endpoint refuses
// unauthenticated callers (403 on missing session) — same admin-gating
// floor as every other bulk endpoint.
func TestAdminAccountBulkUpdateAdminGating(t *testing.T) {
	handler := New(config.Load(), nil)
	body := `{"account_ids":["1"],"status":"active"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected admin gate 403, got %d", rec.Code)
	}
}
