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
	if len(importResp.Data.Items) != 1 || importResp.Data.Items[0].Action != apiopenapi.CodexSessionImportItemActionUpdated {
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
