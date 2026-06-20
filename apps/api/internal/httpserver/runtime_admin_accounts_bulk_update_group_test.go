package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestBulkUpdateAdminAccountsAddsGroupOnly(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"bulk-group-provider","display_name":"Bulk Group Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"bulk-group-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, csrf, `{"name":"bulk-group-target","description":"Bulk target","status":"active"}`)

	body := `{"account_ids":["` + string(accountResp.Data.Id) + `"],"add_group_id":"` + string(groupResp.Data.Id) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected bulk-update group add 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.BatchUpdateAccountsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode bulk-update response: %v", err)
	}
	if resp.Data.UpdatedCount != 1 || len(resp.Data.UpdatedIds) != 1 || resp.Data.UpdatedIds[0] != accountResp.Data.Id || len(resp.Data.Errors) != 0 {
		t.Fatalf("unexpected bulk-update response: %+v", resp.Data)
	}

	groupID, err := strconv.Atoi(string(groupResp.Data.Id))
	if err != nil {
		t.Fatalf("parse group id: %v", err)
	}
	membersReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/account-groups/"+strconv.Itoa(groupID)+"/accounts", nil)
	membersReq.AddCookie(sessionCookie)
	membersRec := httptest.NewRecorder()
	handler.ServeHTTP(membersRec, membersReq)
	if membersRec.Code != http.StatusOK {
		t.Fatalf("expected members list 200, got %d body=%s", membersRec.Code, membersRec.Body.String())
	}
	var membersResp apiopenapi.AccountGroupMemberListResponse
	if err := json.NewDecoder(membersRec.Body).Decode(&membersResp); err != nil {
		t.Fatalf("decode members response: %v", err)
	}
	if len(membersResp.Data) != 1 || membersResp.Data[0].AccountId != accountResp.Data.Id {
		t.Fatalf("expected account to be group member, got %+v", membersResp.Data)
	}
}

func TestBootstrapGatewayCatalogDropsLegacyImagesCapability(t *testing.T) {
	ctx := context.Background()
	providerStore := providermemory.New()
	if _, err := providerStore.Create(ctx, providercontract.CreateStoredProvider{
		Name:        "openai-compatible",
		DisplayName: "OpenAI Compatible",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
		Status:      providercontract.StatusActive,
		Capabilities: map[string]any{
			"images": true,
		},
	}); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	rt, err := newRuntimeState(config.Load(), slog.Default(), runtimeOptions{providers: providerStore})
	if err != nil {
		t.Fatalf("newRuntimeState: %v", err)
	}

	provider, err := rt.providerStore.FindByName(ctx, "openai-compatible")
	if err != nil {
		t.Fatalf("find provider: %v", err)
	}
	if _, ok := provider.Capabilities["images"]; ok {
		t.Fatalf("legacy images capability should be removed: %+v", provider.Capabilities)
	}
	for _, key := range []string{
		capabilitiescontract.KeyImageGenerations,
		capabilitiescontract.KeyImageEdits,
		capabilitiescontract.KeyImageVariations,
	} {
		if provider.Capabilities[key] != true {
			t.Fatalf("expected %s capability to be true, got %+v", key, provider.Capabilities)
		}
	}
}
