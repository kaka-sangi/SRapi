package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
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
		readyRow.AccountBlockers.Inactive != 1 ||
		readyRow.AccountBlockers.Proxy != 0 ||
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

	modelRow := findGatewayModelResourceRow(t, summary, "model-1000")
	if modelRow.Status != apiopenapi.GatewayProviderResourceStatusReady ||
		modelRow.ActiveProviders != 1 ||
		modelRow.ActiveModelMappings != 1 ||
		modelRow.RoutableAccounts != 1 ||
		modelRow.ApiKeyCount != 1 ||
		modelRow.ScopedKeyCount != 1 ||
		len(modelRow.Reasons) != 0 {
		t.Fatalf("unexpected model resource row: %+v", modelRow)
	}
	assertGatewayEndpointRow(t, modelRow, apiopenapi.GatewayEndpointResourceRowKeyChatCompletions, 1, apiopenapi.GatewayProviderResourceStatusReady)
	assertGatewayEndpointRow(t, modelRow, apiopenapi.GatewayEndpointResourceRowKeyResponses, 1, apiopenapi.GatewayProviderResourceStatusReady)
	assertGatewayEndpointDiagnostics(t, modelRow, apiopenapi.GatewayEndpointResourceRowKeyEmbeddings, endpointDiagnostics{
		sourceEndpoint:      "/v1/embeddings",
		candidateAccounts:   1,
		unsupportedAccounts: 0,
		routableAccounts:    1,
		status:              apiopenapi.GatewayProviderResourceStatusReady,
	})

	routeRow := findGatewayRouteResourceRow(t, summary, "model-1000", "ready-provider")
	if routeRow.Status != apiopenapi.GatewayProviderResourceStatusReady ||
		routeRow.UpstreamModel != "model-1000" ||
		routeRow.RoutableAccounts != 1 ||
		routeRow.ApiKeyCount != 1 ||
		routeRow.ScopedKeyCount != 1 ||
		len(routeRow.Reasons) != 0 {
		t.Fatalf("unexpected route resource row: %+v", routeRow)
	}
	assertGatewayRouteEndpointRow(t, routeRow, apiopenapi.GatewayEndpointResourceRowKeyChatCompletions, 1, apiopenapi.GatewayProviderResourceStatusReady)
	assertGatewayRouteEndpointRow(t, routeRow, apiopenapi.GatewayEndpointResourceRowKeyResponses, 1, apiopenapi.GatewayProviderResourceStatusReady)
}

func TestBuildGatewayResourceSummaryReportsBlockedModels(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	providers := []providercontract.Provider{
		testGatewayResourceProvider(1, "ready-provider", providercontract.StatusActive),
		testGatewayResourceProvider(2, "disabled-provider", providercontract.StatusDisabled),
	}

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: providers,
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil),
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
			testGatewayResourceModel(2000, modelcontract.StatusActive),
			testGatewayResourceModel(3000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, Status: modelcontract.StatusActive},
			{ID: 501, ModelID: 2000, ProviderID: 2, Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive, AllowedModels: []string{"model-1000"}},
		},
		Now: now,
	})

	ready := findGatewayModelResourceRow(t, summary, "model-1000")
	if ready.Status != apiopenapi.GatewayProviderResourceStatusReady || len(ready.Reasons) != 0 {
		t.Fatalf("unexpected ready model row: %+v", ready)
	}

	disabledProvider := findGatewayModelResourceRow(t, summary, "model-2000")
	if disabledProvider.Status != apiopenapi.GatewayProviderResourceStatusBlocked ||
		disabledProvider.ActiveProviders != 0 ||
		disabledProvider.RoutableAccounts != 0 ||
		disabledProvider.ApiKeyCount != 0 ||
		!slices.Equal(disabledProvider.Reasons, []apiopenapi.GatewayProviderResourceReason{
			apiopenapi.NoModelMappings,
			apiopenapi.NoRoutableAccounts,
			apiopenapi.NoApiKeys,
		}) {
		t.Fatalf("unexpected disabled-provider model row: %+v", disabledProvider)
	}

	unmapped := findGatewayModelResourceRow(t, summary, "model-3000")
	if unmapped.Status != apiopenapi.GatewayProviderResourceStatusBlocked ||
		unmapped.ActiveModelMappings != 0 ||
		!slices.Equal(unmapped.Reasons, []apiopenapi.GatewayProviderResourceReason{
			apiopenapi.NoModelMappings,
			apiopenapi.NoRoutableAccounts,
			apiopenapi.NoApiKeys,
		}) {
		t.Fatalf("unexpected unmapped model row: %+v", unmapped)
	}
}

func TestBuildGatewayResourceSummaryReportsEndpointCapabilities(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	provider := testGatewayResourceProvider(1, "compact-disabled-provider", providercontract.StatusActive)
	provider.Capabilities = map[string]any{
		capabilitiescontract.KeyResponsesCompact: false,
	}

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: []providercontract.Provider{provider},
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil),
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, UpstreamModelName: "upstream-model", Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive},
		},
		Now: now,
	})

	row := findGatewayModelResourceRow(t, summary, "model-1000")
	if row.Status != apiopenapi.GatewayProviderResourceStatusReady || row.RoutableAccounts != 1 {
		t.Fatalf("unexpected model endpoint row status: %+v", row)
	}
	assertGatewayEndpointRow(t, row, apiopenapi.GatewayEndpointResourceRowKeyChatCompletions, 1, apiopenapi.GatewayProviderResourceStatusReady)
	assertGatewayEndpointRow(t, row, apiopenapi.GatewayEndpointResourceRowKeyResponses, 1, apiopenapi.GatewayProviderResourceStatusReady)
	assertGatewayEndpointRow(t, row, apiopenapi.GatewayEndpointResourceRowKeyResponsesCompact, 0, apiopenapi.GatewayProviderResourceStatusBlocked)
	assertGatewayEndpointDiagnostics(t, row, apiopenapi.GatewayEndpointResourceRowKeyResponsesCompact, endpointDiagnostics{
		sourceEndpoint:      "/v1/responses/compact",
		candidateAccounts:   1,
		unsupportedAccounts: 1,
		routableAccounts:    0,
		status:              apiopenapi.GatewayProviderResourceStatusBlocked,
	})
}

func TestBuildGatewayResourceSummaryReportsEndpointModelAvailability(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	account := testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil)
	account.Metadata = map[string]any{accountExcludedModelsMetadataKey: []any{"model-1000"}}

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: []providercontract.Provider{
			testGatewayResourceProvider(1, "blocked-model-provider", providercontract.StatusActive),
		},
		Accounts: []accountcontract.ProviderAccount{account},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, UpstreamModelName: "upstream-model", Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive},
		},
		Now: now,
	})

	row := findGatewayRouteResourceRow(t, summary, "model-1000", "blocked-model-provider")
	if row.Status != apiopenapi.GatewayProviderResourceStatusLimited ||
		row.RoutableAccounts != 0 ||
		row.ApiKeyCount != 1 ||
		!slices.Equal(row.Reasons, []apiopenapi.GatewayProviderResourceReason{apiopenapi.NoRoutableAccounts}) {
		t.Fatalf("unexpected blocked model route row: %+v", row)
	}
	assertGatewayRouteEndpointDiagnostics(t, row, apiopenapi.GatewayEndpointResourceRowKeyChatCompletions, endpointDiagnostics{
		sourceEndpoint:           "/v1/chat/completions",
		candidateAccounts:        1,
		unavailableModelAccounts: 1,
		routableAccounts:         0,
		status:                   apiopenapi.GatewayProviderResourceStatusBlocked,
	})
}

func TestBuildGatewayResourceSummaryReportsPricingCoverage(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	billing, err := billingservice.NewPricing(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
	}
	rule, err := billing.CreatePricingRule(t.Context(), billingcontract.CreatePricingRuleRequest{
		ModelID:                         1000,
		ProviderID:                      1,
		BillingMode:                     billingcontract.BillingModeToken,
		InputPricePerMillionTokens:      "1",
		OutputPricePerMillionTokens:     "2",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create pricing rule: %v", err)
	}

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Context: context.Background(),
		Billing: billing,
		Providers: []providercontract.Provider{
			testGatewayResourceProvider(1, "priced-provider", providercontract.StatusActive),
		},
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil),
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, UpstreamModelName: "upstream-model", Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive},
		},
		Now: now,
	})

	row := findGatewayModelResourceRow(t, summary, "model-1000")
	if row.Pricing.Status != apiopenapi.GatewayPricingCoverageStatusPriced ||
		row.Pricing.Source != apiopenapi.GatewayPricingCoverageSourcePricingRule ||
		row.Pricing.PricingRuleId == nil ||
		*row.Pricing.PricingRuleId != rule.ID ||
		row.Pricing.PricedRoutes == 0 ||
		row.Pricing.PricedRoutes != row.Pricing.TotalRoutes {
		t.Fatalf("unexpected priced coverage: %+v", row.Pricing)
	}
}

func TestBuildGatewayResourceSummaryScopesRouteApiKeysByProviderGroups(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: []providercontract.Provider{
			testGatewayResourceProvider(1, "provider-one", providercontract.StatusActive),
			testGatewayResourceProvider(2, "provider-two", providercontract.StatusActive),
		},
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusActive, nil),
			testGatewayResourceAccount(20, 2, accountcontract.StatusActive, nil),
		},
		GroupIDsByAccount: map[int][]int{
			10: {100},
			20: {200},
		},
		Models: []modelcontract.Model{
			testGatewayResourceModel(1000, modelcontract.StatusActive),
		},
		ModelMappings: []modelcontract.ModelProviderMapping{
			{ID: 500, ModelID: 1000, ProviderID: 1, UpstreamModelName: "provider-one-upstream", Status: modelcontract.StatusActive},
			{ID: 501, ModelID: 1000, ProviderID: 2, UpstreamModelName: "provider-two-upstream", Status: modelcontract.StatusActive},
		},
		APIKeys: []apikeycontract.APIKey{
			{ID: 1, Status: apikeycontract.StatusActive, GroupIDs: []int{100}, AllowedModels: []string{"model-1000"}},
		},
		Now: now,
	})

	providerOne := findGatewayRouteResourceRow(t, summary, "model-1000", "provider-one")
	if providerOne.Status != apiopenapi.GatewayProviderResourceStatusReady ||
		providerOne.ApiKeyCount != 1 ||
		providerOne.ScopedKeyCount != 1 ||
		providerOne.UpstreamModel != "provider-one-upstream" {
		t.Fatalf("unexpected provider-one route row: %+v", providerOne)
	}

	providerTwo := findGatewayRouteResourceRow(t, summary, "model-1000", "provider-two")
	if providerTwo.Status != apiopenapi.GatewayProviderResourceStatusLimited ||
		providerTwo.ApiKeyCount != 0 ||
		!slices.Equal(providerTwo.Reasons, []apiopenapi.GatewayProviderResourceReason{apiopenapi.NoApiKeys}) {
		t.Fatalf("unexpected provider-two route row: %+v", providerTwo)
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
	if len(resp.Data.ModelRows) == 0 || len(resp.Data.RouteRows) == 0 {
		t.Fatalf("expected model resource rows in summary: %+v", resp.Data)
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

func findGatewayModelResourceRow(t *testing.T, summary apiopenapi.GatewayResourceSummary, modelName string) apiopenapi.GatewayModelResourceRow {
	t.Helper()
	for _, row := range summary.ModelRows {
		if row.Model.CanonicalName == modelName {
			return row
		}
	}
	t.Fatalf("model %q not found in %+v", modelName, summary.ModelRows)
	return apiopenapi.GatewayModelResourceRow{}
}

func findGatewayRouteResourceRow(t *testing.T, summary apiopenapi.GatewayResourceSummary, modelName string, providerName string) apiopenapi.GatewayRouteResourceRow {
	t.Helper()
	for _, row := range summary.RouteRows {
		if row.Model.CanonicalName == modelName && row.Provider.Name == providerName {
			return row
		}
	}
	t.Fatalf("route %q/%q not found in %+v", modelName, providerName, summary.RouteRows)
	return apiopenapi.GatewayRouteResourceRow{}
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
		row.AccountBlockers.Health != 1 ||
		row.AccountBlockers.Quota != 1 ||
		!slices.Equal(row.Reasons, []apiopenapi.GatewayProviderResourceReason{apiopenapi.NoRoutableAccounts}) {
		t.Fatalf("unexpected blocked row: %+v", row)
	}
}

func TestBuildGatewayResourceSummaryReportsProviderAccountBlockers(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)

	summary := buildGatewayResourceSummary(gatewayResourceSummaryInput{
		Providers: []providercontract.Provider{
			testGatewayResourceProvider(1, "blocker-provider", providercontract.StatusActive),
		},
		Accounts: []accountcontract.ProviderAccount{
			testGatewayResourceAccount(10, 1, accountcontract.StatusDisabled, nil),
			testGatewayResourceAccount(11, 1, accountcontract.StatusActive, nil),
			testGatewayResourceAccount(12, 1, accountcontract.StatusActive, nil),
			testGatewayResourceAccount(13, 1, accountcontract.StatusActive, ptrString("1")),
			testGatewayResourceAccount(14, 1, accountcontract.StatusActive, nil),
		},
		Proxies: []accountcontract.ProxyDefinition{
			{
				ID:            1,
				Name:          "expired",
				URLCiphertext: "encrypted",
				Status:        accountcontract.ProxyStatusActive,
				ExpiresAt:     &expiredAt,
				FallbackMode:  accountcontract.ProxyFallbackModeNone,
			},
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
			11: {AccountID: 11, ProviderID: 1, Status: string(accountcontract.StatusSuspended), CircuitState: "closed"},
		},
		QuotasByAccount: map[int][]accountcontract.AccountQuotaSnapshot{
			12: {
				{
					AccountID:      12,
					ProviderID:     1,
					QuotaType:      accountcontract.QuotaTypeProviderCredits,
					RemainingRatio: 0,
					SnapshotAt:     now,
				},
			},
		},
		Now: now,
	})

	row := findGatewayResourceRow(t, summary, "blocker-provider")
	if row.Status != apiopenapi.GatewayProviderResourceStatusLimited ||
		row.RoutableAccounts != 1 ||
		row.AttentionAccounts != 3 ||
		row.AccountBlockers.Inactive != 1 ||
		row.AccountBlockers.Health != 1 ||
		row.AccountBlockers.Quota != 1 ||
		row.AccountBlockers.Proxy != 1 {
		t.Fatalf("unexpected provider account blockers: %+v", row)
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

func assertGatewayEndpointRow(t *testing.T, row apiopenapi.GatewayModelResourceRow, key apiopenapi.GatewayEndpointResourceRowKey, accounts int, status apiopenapi.GatewayProviderResourceStatus) {
	t.Helper()
	endpoint := findGatewayEndpointRow(t, row.Endpoints, key)
	if endpoint.RoutableAccounts != accounts || endpoint.Status != status {
		t.Fatalf("endpoint %s = %+v, want accounts=%d status=%s", key, endpoint, accounts, status)
	}
}

func assertGatewayRouteEndpointRow(t *testing.T, row apiopenapi.GatewayRouteResourceRow, key apiopenapi.GatewayEndpointResourceRowKey, accounts int, status apiopenapi.GatewayProviderResourceStatus) {
	t.Helper()
	endpoint := findGatewayEndpointRow(t, row.Endpoints, key)
	if endpoint.RoutableAccounts != accounts || endpoint.Status != status {
		t.Fatalf("route endpoint %s = %+v, want accounts=%d status=%s", key, endpoint, accounts, status)
	}
}

type endpointDiagnostics struct {
	sourceEndpoint           string
	candidateAccounts        int
	unsupportedAccounts      int
	unavailableModelAccounts int
	routableAccounts         int
	status                   apiopenapi.GatewayProviderResourceStatus
}

func assertGatewayEndpointDiagnostics(t *testing.T, row apiopenapi.GatewayModelResourceRow, key apiopenapi.GatewayEndpointResourceRowKey, want endpointDiagnostics) {
	t.Helper()
	endpoint := findGatewayEndpointRow(t, row.Endpoints, key)
	assertEndpointDiagnostics(t, endpoint, want)
}

func assertGatewayRouteEndpointDiagnostics(t *testing.T, row apiopenapi.GatewayRouteResourceRow, key apiopenapi.GatewayEndpointResourceRowKey, want endpointDiagnostics) {
	t.Helper()
	endpoint := findGatewayEndpointRow(t, row.Endpoints, key)
	assertEndpointDiagnostics(t, endpoint, want)
}

func assertEndpointDiagnostics(t *testing.T, endpoint apiopenapi.GatewayEndpointResourceRow, want endpointDiagnostics) {
	t.Helper()
	if endpoint.SourceEndpoint != want.sourceEndpoint ||
		endpoint.CandidateAccounts != want.candidateAccounts ||
		endpoint.UnsupportedAccounts != want.unsupportedAccounts ||
		endpoint.UnavailableModelAccounts != want.unavailableModelAccounts ||
		endpoint.RoutableAccounts != want.routableAccounts ||
		endpoint.Status != want.status {
		t.Fatalf("endpoint diagnostics = %+v, want %+v", endpoint, want)
	}
}

func findGatewayEndpointRow(t *testing.T, endpoints []apiopenapi.GatewayEndpointResourceRow, key apiopenapi.GatewayEndpointResourceRowKey) apiopenapi.GatewayEndpointResourceRow {
	t.Helper()
	for _, endpoint := range endpoints {
		if endpoint.Key != key {
			continue
		}
		return endpoint
	}
	t.Fatalf("endpoint %s not found in %+v", key, endpoints)
	return apiopenapi.GatewayEndpointResourceRow{}
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
		CanonicalName: "model-" + strconv.Itoa(id),
		DisplayName:   "Model " + strconv.Itoa(id),
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
