package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestBuildGatewayResourceSummaryAggregatesReadiness(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	providers := []providercontract.Provider{
		testGatewayResourceProvider(1, "ready-provider", providercontract.StatusActive),
		testGatewayResourceProvider(2, "limited-provider", providercontract.StatusActive),
	}
	backupProxyID := 2
	expiredAt := now.Add(-time.Minute)

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: providers,
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, ptrString("1")),
			testGatewayResourceAccount(11, 1, accountcontract.StatusDisabled, nil),
			testGatewayResourceAccount(20, 2, accountcontract.StatusActive, nil),
		},
		GroupIDsByAccount: map[int][]int{
			10: {100},
			11: {100},
		},
		Proxies: []accountcontract.ProxyDefinition{
			{
				ID:            1,
				Name:          "expired-primary",
				URLCiphertext: "encrypted",
				Status:        accountcontract.ProxyStatusActive,
				ExpiresAt:     &expiredAt,
				FallbackMode:  accountcontract.ProxyFallbackModeProxy,
				BackupProxyID: &backupProxyID,
			},
			{
				ID:            2,
				Name:          "backup",
				URLCiphertext: "encrypted",
				Status:        accountcontract.ProxyStatusActive,
				FallbackMode:  accountcontract.ProxyFallbackModeNone,
			},
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
			testGatewayResourceModel(1001, modelcontract.StatusDisabled),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, Status: modelcontract.StatusActive},
			{ID: 501, ModelID: 1001, ProviderID: 2, Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive, GroupIDs: []int{100}},
			{ID: 2, Status: apikeycontract.StatusDisabled},
		},
		Now: now,
	})

	if summary.Providers != 2 ||
		summary.ActiveProviders != 2 ||
		summary.ActiveModels != 1 ||
		summary.ActiveModelMappings != 1 ||
		summary.ActiveApiKeys != 1 ||
		summary.ActiveAccounts != 2 ||
		summary.RoutableAccounts != 2 ||
		summary.ActiveProxies != 2 ||
		summary.AvailableProxies != 2 ||
		summary.ExpiredProxies != 1 ||
		summary.ProxiedAccounts != 1 ||
		summary.ProxyAttentionAccounts != 1 ||
		summary.ScopedApiKeys != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	readyRow := findGatewayResourceRow(t, summary, "ready-provider")
	if readyRow.Status != apiopenapi.GatewayProviderResourceStatusLimited ||
		readyRow.TotalAccounts != 2 ||
		readyRow.RoutableAccounts != 1 ||
		readyRow.AttentionAccounts != 0 ||
		readyRow.ProxiedAccounts != 1 ||
		readyRow.ProxyAttentionAccounts != 1 ||
		readyRow.ActiveModelMappings != 1 ||
		readyRow.ApiKeyCount != 1 ||
		readyRow.ScopedKeyCount != 1 ||
		!slices.Equal(readyRow.Reasons, []apiopenapi.GatewayProviderResourceReason{apiopenapi.ProxyAttention}) {
		t.Fatalf("unexpected ready provider row: %+v", readyRow)
	}

	limitedRow := findGatewayResourceRow(t, summary, "limited-provider")
	if limitedRow.Status != apiopenapi.GatewayProviderResourceStatusLimited ||
		limitedRow.RoutableAccounts != 1 ||
		!slices.Equal(limitedRow.Reasons, []apiopenapi.GatewayProviderResourceReason{
			apiopenapi.NoModelMappings,
			apiopenapi.NoApiKeys,
		}) {
		t.Fatalf("unexpected limited provider row: %+v", limitedRow)
	}
}

func TestAdminGatewayResourcesEndpointReturnsSummary(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gateway-resources-provider","display_name":"Gateway Resources Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gateway-resources-model","display_name":"Gateway Resources Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gateway-resources-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"gateway-resources-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"status":"active"}`)
	mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/gateway-resources", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected gateway resources 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.GatewayResourceSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gateway resources response: %v", err)
	}
	if resp.Data.Providers == 0 || resp.Data.ActiveModels == 0 || resp.Data.ActiveApiKeys == 0 {
		t.Fatalf("unexpected empty summary: %+v", resp.Data)
	}
	row := findGatewayResourceRow(t, resp.Data, "gateway-resources-provider")
	if row.Status != apiopenapi.GatewayProviderResourceStatusReady ||
		row.RoutableAccounts != 1 ||
		row.ActiveModelMappings != 1 ||
		row.ApiKeyCount == 0 ||
		len(row.Reasons) != 0 {
		t.Fatalf("unexpected endpoint row: %+v", row)
	}
}

func TestAdminGatewayResourcesEndpointRequiresAdmin(t *testing.T) {
	handler := New(config.Load(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/gateway-resources", strings.NewReader(""))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected gateway resources 403 without admin session, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuildGatewayResourceSummaryBlocksUnroutableAccounts(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	provider := testGatewayResourceProvider(1, "blocked-provider", providercontract.StatusActive)

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: []providercontract.Provider{provider},
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil),
			testGatewayResourceAccount(11, 1, accountcontract.StatusActive, nil),
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive},
		},
		HealthByAccount: map[int]accountcontract.AccountHealthSnapshot{
			10: {AccountID: 10, ProviderID: 1, Status: "dead", CircuitState: "closed"},
		},
		QuotasByAccount: map[int][]accountcontract.AccountQuotaSnapshot{
			11: {
				{
					AccountID:      11,
					ProviderID:     1,
					QuotaType:      accountcontract.QuotaTypeProviderCredits,
					RemainingRatio: 0,
					SnapshotAt:     now,
				},
			},
		},
		Now: now,
	})

	row := findGatewayResourceRow(t, summary, "blocked-provider")
	if row.Status != apiopenapi.GatewayProviderResourceStatusBlocked ||
		row.RoutableAccounts != 0 ||
		row.AttentionAccounts != 2 ||
		!slices.Equal(row.Reasons, []apiopenapi.GatewayProviderResourceReason{apiopenapi.NoRoutableAccounts}) {
		t.Fatalf("unexpected blocked row: %+v", row)
	}
}

func TestBuildGatewayResourceSummaryTreatsResetQuotaAsRoutable(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(-time.Minute)
	provider := testGatewayResourceProvider(1, "reset-quota-provider", providercontract.StatusActive)

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: []providercontract.Provider{provider},
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil),
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive},
		},
		QuotasByAccount: map[int][]accountcontract.AccountQuotaSnapshot{
			10: {
				{
					AccountID:      10,
					ProviderID:     1,
					QuotaType:      accountcontract.QuotaTypeProviderCredits,
					RemainingRatio: 0,
					ResetAt:        &resetAt,
					SnapshotAt:     now.Add(-time.Hour),
				},
			},
		},
		Now: now,
	})

	row := findGatewayResourceRow(t, summary, "reset-quota-provider")
	if row.Status != apiopenapi.GatewayProviderResourceStatusReady ||
		row.RoutableAccounts != 1 ||
		len(row.Reasons) != 0 {
		t.Fatalf("unexpected reset quota row: %+v", row)
	}
}

func findGatewayResourceRow(t *testing.T, summary apiopenapi.GatewayResourceSummary, providerName string) apiopenapi.GatewayProviderResourceRow {
	t.Helper()
	for _, row := range summary.Rows {
		if row.Provider.Name == providerName {
			return row
		}
	}
	t.Fatalf("provider %q not found in %+v", providerName, summary.Rows)
	return apiopenapi.GatewayProviderResourceRow{}
}

func testGatewayResourceProvider(id int, name string, status providercontract.Status) providercontract.Provider {
	return providercontract.Provider{
		ID:          id,
		Name:        name,
		DisplayName: name,
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
		Status:      status,
	}
}

func testGatewayResourceModel(id int, status modelcontract.Status) modelcontract.Model {
	return modelcontract.Model{
		ID:            id,
		CanonicalName: "model",
		DisplayName:   "Model",
		Status:        status,
	}
}

func testGatewayResourceAccount(id int, providerID int, status accountcontract.Status, proxyID *string) accountcontract.ProviderAccount {
	return accountcontract.ProviderAccount{
		ID:           id,
		ProviderID:   providerID,
		Name:         "account",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		ProxyID:      proxyID,
		Status:       status,
		Weight:       1,
	}
}
