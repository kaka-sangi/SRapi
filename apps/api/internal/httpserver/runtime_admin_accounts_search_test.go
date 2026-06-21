package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// TestListAdminAccountsHonorsSearch verifies the GET /admin/accounts
// endpoint narrows results by the `search` query parameter against
// name, upstream client, and stringified id. Mirrors the bulk-update
// filter behavior so operators can find an account quickly when the
// fleet grows.
func TestListAdminAccountsHonorsSearch(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"name":"search-provider","display_name":"Search Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	provID := string(providerResp.Data.Id)

	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"provider_id":"`+provID+`","name":"alpha-prod","runtime_class":"api_key","credential":{"api_key":"a"},"status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"provider_id":"`+provID+`","name":"beta-staging","runtime_class":"api_key","upstream_client":"openai_codex_cli","credential":{"api_key":"b"},"status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"provider_id":"`+provID+`","name":"gamma","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"tok"},"status":"active"}`)

	cases := []struct {
		name      string
		query     string
		wantNames []string
	}{
		{"name substring matches", "search=alpha", []string{"alpha-prod"}},
		{"case insensitive name", "search=GAMMA", []string{"gamma"}},
		{"upstream client matches exact", "search=codex_cli", []string{"gamma", "beta-staging"}},
		{"empty search returns all", "", []string{"alpha-prod", "beta-staging", "gamma"}},
		{"runtime_class filter", "runtime_class=cli_client_token", []string{"gamma"}},
		{"runtime_class + search compose", "runtime_class=api_key&search=staging", []string{"beta-staging"}},
		{"search across runtime_class boundary", "runtime_class=api_key&search=gamma", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/admin/accounts"
			if tc.query != "" {
				path += "?" + tc.query
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(sessionCookie)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200 got %d body=%s", rec.Code, rec.Body.String())
			}
			body, _ := io.ReadAll(rec.Result().Body)
			var resp struct {
				Data []struct {
					Name string `json:"name"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := make([]string, 0, len(resp.Data))
			for _, item := range resp.Data {
				got = append(got, item.Name)
			}
			if !sameNameSet(got, tc.wantNames) {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.wantNames)
			}
		})
	}
}

func sameNameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	for _, g := range got {
		if _, ok := wantSet[g]; !ok {
			return false
		}
	}
	return true
}

// Guards a regression where a search hit that lives outside the
// status==archived bucket leaks the archived rows back. Default list
// hides archived accounts; search must not undo that.
func TestListAdminAccountsSearchSkipsArchived(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"name":"search-archive-provider","display_name":"Search Archive","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	provID := string(providerResp.Data.Id)

	active := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"provider_id":"`+provID+`","name":"keep-me","runtime_class":"api_key","credential":{"api_key":"k"},"status":"active"}`)
	archived := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken,
		`{"provider_id":"`+provID+`","name":"keep-archived","runtime_class":"api_key","credential":{"api_key":"k2"},"status":"archived"}`)
	_ = active
	_ = archived

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?search=keep", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "keep-me") {
		t.Fatalf("expected keep-me in default list, body=%s", body)
	}
	if strings.Contains(body, "keep-archived") {
		t.Fatalf("default list must not surface archived rows via search, body=%s", body)
	}
}
