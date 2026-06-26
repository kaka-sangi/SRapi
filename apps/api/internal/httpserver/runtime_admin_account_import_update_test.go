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

func TestAdminAccountImportUpdatesExistingGenericAccount(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"generic-import-update-provider","display_name":"Generic Import Update","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	providerID := string(providerResp.Data.Id)
	accountResp, _ := mustCreateAdminAccountRaw(t, handler, sessionCookie, csrf, `{"provider_id":"`+providerID+`","name":"generic-import-update-account","runtime_class":"api_key","credential":{"api_key":"old-secret"},"metadata":{"base_url":"https://old.example.test/v1","region":"old"},"status":"active","priority":1,"weight":1}`)

	importResp, _ := mustImportAdminAccountsRaw(t, handler, sessionCookie, csrf, `{"accounts":[{"provider_id":"`+providerID+`","name":"generic-import-update-account","runtime_class":"api_key","credential":{"api_key":"new-secret"},"metadata":{"base_url":"https://new.example.test/v1","region":"new"},"status":"disabled","priority":7,"weight":2}]}`)
	if importResp.Data.CreatedCount != 0 || importResp.Data.UpdatedCount != 1 || len(importResp.Data.UpdatedIds) != 1 {
		t.Fatalf("expected one updated account, got %+v", importResp.Data)
	}
	if importResp.Data.UpdatedIds[0] != accountResp.Data.Id {
		t.Fatalf("expected updated id %s, got %s", accountResp.Data.Id, importResp.Data.UpdatedIds[0])
	}
	if len(importResp.Data.Items) != 1 || importResp.Data.Items[0].Action != apiopenapi.SessionImportItemActionUpdated {
		t.Fatalf("expected updated import item, got %+v", importResp.Data.Items)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account get 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var got apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(strings.NewReader(getRec.Body.String())).Decode(&got); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if got.Data.Status != apiopenapi.ProviderAccountStatusDisabled || got.Data.Priority != 7 || got.Data.Weight != 2 {
		t.Fatalf("expected updated mutable account fields, got %+v", got.Data)
	}
	if got.Data.Metadata == nil || (*got.Data.Metadata)["base_url"] != "https://new.example.test/v1" || (*got.Data.Metadata)["region"] != "new" {
		t.Fatalf("expected updated metadata, got %+v", got.Data.Metadata)
	}
}

func TestAdminAccountImportDoesNotCollapseSharedChatGPTAccountID(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"chatgpt-shared-account-import-provider","display_name":"ChatGPT Shared Account Import","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	providerID := string(providerResp.Data.Id)

	body := `{"accounts":[` +
		`{"provider_id":"` + providerID + `","name":"alice@example.test","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"access-a","refresh_token":"refresh-a"},"metadata":{"chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-a","email":"alice@example.test"},"status":"active"},` +
		`{"provider_id":"` + providerID + `","name":"bob@example.test","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"access-b","refresh_token":"refresh-b"},"metadata":{"chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-b","email":"bob@example.test"},"status":"active"},` +
		`{"provider_id":"` + providerID + `","name":"carol@example.test","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"access-c","refresh_token":"refresh-c"},"metadata":{"chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-c","email":"carol@example.test"},"status":"active"}` +
		`]}`

	importResp, raw := mustImportAdminAccountsRaw(t, handler, sessionCookie, csrf, body)
	if importResp.Data.CreatedCount != 3 || importResp.Data.SkippedCount != 0 || importResp.Data.UpdatedCount != 0 || importResp.Data.FailedCount != 0 {
		t.Fatalf("expected all shared-account-id rows to create, got %+v raw=%s", importResp.Data, raw)
	}
	if len(importResp.Data.CreatedIds) != 3 || len(importResp.Data.Items) != 3 {
		t.Fatalf("expected three created ids/items, got %+v", importResp.Data)
	}
	for _, item := range importResp.Data.Items {
		if item.Action != apiopenapi.SessionImportItemActionCreated || item.AccountId == nil {
			t.Fatalf("expected created item with account id, got %+v", item)
		}
	}
	if strings.Contains(raw, "access-a") || strings.Contains(raw, "refresh-a") {
		t.Fatalf("import response leaked credentials: %s", raw)
	}
}
