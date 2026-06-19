package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	userplatformquotascontract "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
	userplatformquotasmemory "github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestGatewayEnforcesUserPlatformSpendQuota seeds prior spend on a platform that
// exceeds a per-user daily cap and asserts the gateway hard-denies (402) the
// next request routed to that platform.
func TestGatewayEnforcesUserPlatformSpendQuota(t *testing.T) {
	usageStore := usagememory.New()
	quotaStore := userplatformquotasmemory.New()
	handler := New(config.Load(), nil, WithUsageStore(usageStore), WithUserPlatformQuotasStore(quotaStore))

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateOpenAIChatGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, "platform-quota-provider", "platform-quota-model")
	adminUserID, err := strconv.Atoi(loginResp.Data.User.Id)
	if err != nil {
		t.Fatalf("parse admin user id %q: %v", loginResp.Data.User.Id, err)
	}
	providerID, err := strconv.Atoi(string(providerResp.Data.Id))
	if err != nil {
		t.Fatalf("parse provider id %q: %v", providerResp.Data.Id, err)
	}

	// $10 of prior successful spend today on the platform.
	if _, err := usageStore.Create(context.Background(), usagecontract.UsageLog{
		RequestID:  "seed-platform-spend",
		UserID:     adminUserID,
		ProviderID: &providerID,
		Model:      "platform-quota-model",
		Success:    true,
		Cost:       "10.00000000",
		Currency:   "USD",
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed usage log: %v", err)
	}

	// Cap daily spend on the platform at $1 — already exceeded by the seed.
	daily := "1.00000000"
	if _, err := quotaStore.UpsertQuota(context.Background(), userplatformquotascontract.UpsertQuota{
		UserID:     adminUserID,
		Platform:   providerResp.Data.Name,
		DailyLimit: &daily,
		Currency:   "USD",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("seed platform quota: %v", err)
	}

	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"platform-quota-gateway","scopes":["gateway:invoke"]}`)
	apiKey := keyResp.Data.PlaintextKey

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"platform-quota-model","messages":[{"role":"user","content":"over the platform cap"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 platform quota deny, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "quota") {
		t.Fatalf("expected platform quota error body, got %s", rec.Body.String())
	}
}

func TestGatewayIgnoresInvalidStoredPlatformSpendQuotaLimit(t *testing.T) {
	quotaStore := userplatformquotasmemory.New()
	handler := New(config.Load(), nil, WithUserPlatformQuotasStore(quotaStore))

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateOpenAIChatGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, "bad-platform-quota-provider", "bad-platform-quota-model")
	adminUserID, err := strconv.Atoi(loginResp.Data.User.Id)
	if err != nil {
		t.Fatalf("parse admin user id %q: %v", loginResp.Data.User.Id, err)
	}

	invalidDaily := "not-money"
	if _, err := quotaStore.UpsertQuota(context.Background(), userplatformquotascontract.UpsertQuota{
		UserID:     adminUserID,
		Platform:   providerResp.Data.Name,
		DailyLimit: &invalidDaily,
		Currency:   "USD",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("seed invalid stored platform quota: %v", err)
	}

	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"bad-platform-quota-gateway","scopes":["gateway:invoke"]}`)
	apiKey := keyResp.Data.PlaintextKey

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"bad-platform-quota-model","messages":[{"role":"user","content":"invalid stored quota should fail open"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected gateway to ignore invalid stored quota limit, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserPlatformQuotaRejectsInvalidMoneyLimits(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	adminUserID, err := strconv.Atoi(loginResp.Data.User.Id)
	if err != nil {
		t.Fatalf("parse admin user id %q: %v", loginResp.Data.User.Id, err)
	}

	cases := []struct {
		name string
		body string
	}{
		{
			name: "invalid decimal",
			body: `{"platform":"openai-compatible","daily_limit":"not-money","enabled":true}`,
		},
		{
			name: "negative decimal",
			body: `{"platform":"openai-compatible","weekly_limit":"-0.01000000","enabled":true}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+strconv.Itoa(adminUserID)+"/platform-quotas", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfHeaderName, loginResp.Data.CsrfToken)
			req.AddCookie(sessionCookie)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid platform quota 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}

	validReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+strconv.Itoa(adminUserID)+"/platform-quotas", strings.NewReader(`{"platform":"openai-compatible","daily_limit":" 1.5 ","monthly_limit":"","enabled":true}`))
	validReq.Header.Set("Content-Type", "application/json")
	validReq.Header.Set(csrfHeaderName, loginResp.Data.CsrfToken)
	validReq.AddCookie(sessionCookie)
	validRec := httptest.NewRecorder()
	handler.ServeHTTP(validRec, validReq)
	if validRec.Code != http.StatusOK {
		t.Fatalf("expected valid platform quota 200, got %d body=%s", validRec.Code, validRec.Body.String())
	}
	var resp apiopenapi.UserPlatformQuotaResponse
	if err := json.NewDecoder(validRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode platform quota response: %v", err)
	}
	if resp.Data.DailyLimit == nil || *resp.Data.DailyLimit != "1.50000000" {
		t.Fatalf("expected normalized daily limit, got %+v", resp.Data.DailyLimit)
	}
	if resp.Data.MonthlyLimit != nil {
		t.Fatalf("expected blank monthly limit to clear, got %+v", resp.Data.MonthlyLimit)
	}
}

func TestCurrentUserPlatformQuotasListsOwnQuotas(t *testing.T) {
	quotaStore := userplatformquotasmemory.New()
	handler := New(config.Load(), nil, WithUserPlatformQuotasStore(quotaStore))

	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	adminUserID, err := strconv.Atoi(loginResp.Data.User.Id)
	if err != nil {
		t.Fatalf("parse admin user id %q: %v", loginResp.Data.User.Id, err)
	}

	daily := "3.00000000"
	if _, err := quotaStore.UpsertQuota(context.Background(), userplatformquotascontract.UpsertQuota{
		UserID:     adminUserID,
		Platform:   "anthropic-compatible",
		DailyLimit: &daily,
		Currency:   "USD",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("seed current user platform quota: %v", err)
	}
	if _, err := quotaStore.UpsertQuota(context.Background(), userplatformquotascontract.UpsertQuota{
		UserID:   adminUserID + 1,
		Platform: "openai-compatible",
		Currency: "USD",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("seed other user platform quota: %v", err)
	}

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/platform-quotas", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated platform quotas 401, got %d body=%s", unauthRec.Code, unauthRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/platform-quotas", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected platform quotas 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.UserPlatformQuotaListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode platform quotas: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one current-user quota, got %+v", resp.Data)
	}
	quota := resp.Data[0]
	if quota.UserId != int64(adminUserID) || quota.Platform != "anthropic-compatible" {
		t.Fatalf("unexpected current-user quota: %+v", quota)
	}
	if quota.DailyLimit == nil || *quota.DailyLimit != daily {
		t.Fatalf("expected daily limit %q, got %+v", daily, quota.DailyLimit)
	}
	if resp.Pagination.Total != 1 {
		t.Fatalf("expected pagination total 1, got %+v", resp.Pagination)
	}
}
