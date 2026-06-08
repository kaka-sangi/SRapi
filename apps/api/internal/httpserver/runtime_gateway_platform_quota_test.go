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
	adminUserID, err := strconv.Atoi(loginResp.Data.User.Id)
	if err != nil {
		t.Fatalf("parse admin user id %q: %v", loginResp.Data.User.Id, err)
	}
	providerID := seededProviderID(t, handler, sessionCookie, "openai-compatible")

	// $10 of prior successful spend today on the platform.
	if _, err := usageStore.Create(context.Background(), usagecontract.UsageLog{
		RequestID:  "seed-platform-spend",
		UserID:     adminUserID,
		ProviderID: &providerID,
		Model:      "gpt-4o-mini",
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
		Platform:   "openai-compatible",
		DailyLimit: &daily,
		Currency:   "USD",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("seed platform quota: %v", err)
	}

	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"platform-quota-gateway","scopes":["gateway:invoke"]}`)
	apiKey := keyResp.Data.PlaintextKey

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"over the platform cap"}]}`))
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

func seededProviderID(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, name string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list providers: %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	for _, provider := range resp.Data {
		if provider.Name == name {
			id, err := strconv.Atoi(provider.Id)
			if err != nil {
				t.Fatalf("parse provider id %q: %v", provider.Id, err)
			}
			return id
		}
	}
	t.Fatalf("seeded provider %q not found", name)
	return 0
}
