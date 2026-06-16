package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestAdminUsageDistributionEndpoint proves the share-by-dimension distribution
// handler groups usage logs by the chosen dimension and computes each bucket's
// percentage share of the chosen metric. Three "alpha" logs and one "beta" log
// are seeded with distinct billing modes and costs so the model dimension, the
// metric switch (requests vs cost re-orders the buckets), the billing-mode
// dimension, the top-N cap and the validation/auth paths are all exercised.
func TestAdminUsageDistributionEndpoint(t *testing.T) {
	usageStore := usagememory.New()
	handler := New(config.Load(), nil, WithUsageStore(usageStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	userID := apiIDToIntForTest(t, loginResp.Data.User.Id)

	now := time.Now().UTC()
	// Three alpha logs (subscription billing, cheap) and one beta log (payg
	// billing, expensive). alpha leads by request count; beta leads by cost.
	for i := 0; i < 3; i++ {
		seedUsageLog(t, usageStore, usagecontract.UsageLog{
			RequestID:    "req_alpha_" + string(rune('a'+i)),
			UserID:       userID,
			Model:        "alpha",
			BillingMode:  "subscription",
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
			Success:      true,
			Cost:         "0.10000000",
			Currency:     "USD",
			CreatedAt:    now.Add(-time.Duration(i) * time.Minute),
		})
	}
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:    "req_beta_1",
		UserID:       userID,
		Model:        "beta",
		BillingMode:  "payg",
		InputTokens:  4,
		OutputTokens: 2,
		TotalTokens:  6,
		Success:      true,
		Cost:         "0.50000000",
		Currency:     "USD",
		CreatedAt:    now,
	})

	// model dimension, requests metric: alpha (3 reqs, 75%) leads beta (1, 25%).
	var byModel struct {
		Data      apiopenapi.UsageDistributionResult `json:"data"`
		RequestID string                             `json:"request_id"`
	}
	rec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/distribution?dimension=model&metric=requests")
	if rec.Code != http.StatusOK {
		t.Fatalf("distribution model: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&byModel); err != nil {
		t.Fatalf("decode distribution model: %v", err)
	}
	if byModel.RequestID == "" {
		t.Fatalf("distribution model: expected a request_id")
	}
	if byModel.Data.Dimension != "model" || byModel.Data.Metric != "requests" {
		t.Fatalf("distribution model: echoed dimension/metric mismatch: %+v", byModel.Data)
	}
	if len(byModel.Data.Buckets) != 2 {
		t.Fatalf("distribution model: expected 2 buckets, got %+v", byModel.Data.Buckets)
	}
	alpha := byModel.Data.Buckets[0]
	if alpha.Label != "alpha" || alpha.Requests != 3 || alpha.TotalTokens != 45 || alpha.Cost != "0.30" {
		t.Fatalf("distribution model: unexpected leading bucket %+v", alpha)
	}
	if alpha.Percentage != 75 {
		t.Fatalf("distribution model: expected alpha 75%% of requests, got %v", alpha.Percentage)
	}
	if beta := byModel.Data.Buckets[1]; beta.Label != "beta" || beta.Requests != 1 || beta.Percentage != 25 {
		t.Fatalf("distribution model: unexpected trailing bucket %+v", beta)
	}

	// cost metric re-orders: beta (0.50, ~62.5%) now leads alpha (0.30, ~37.5%).
	var byCost struct {
		Data apiopenapi.UsageDistributionResult `json:"data"`
	}
	costRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/distribution?dimension=model&metric=cost")
	if costRec.Code != http.StatusOK {
		t.Fatalf("distribution cost: expected 200, got %d body=%s", costRec.Code, costRec.Body.String())
	}
	if err := json.NewDecoder(costRec.Body).Decode(&byCost); err != nil {
		t.Fatalf("decode distribution cost: %v", err)
	}
	if len(byCost.Data.Buckets) != 2 || byCost.Data.Buckets[0].Label != "beta" {
		t.Fatalf("distribution cost: expected beta to lead by cost, got %+v", byCost.Data.Buckets)
	}
	if got := byCost.Data.Buckets[0].Percentage; got < 62 || got > 63 {
		t.Fatalf("distribution cost: expected beta ~62.5%% of cost, got %v", got)
	}

	// billing_mode dimension, requests metric: subscription (3) vs payg (1).
	var byBilling struct {
		Data apiopenapi.UsageDistributionResult `json:"data"`
	}
	billingRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/distribution?dimension=billing_mode")
	if billingRec.Code != http.StatusOK {
		t.Fatalf("distribution billing_mode: expected 200, got %d body=%s", billingRec.Code, billingRec.Body.String())
	}
	if err := json.NewDecoder(billingRec.Body).Decode(&byBilling); err != nil {
		t.Fatalf("decode distribution billing_mode: %v", err)
	}
	if len(byBilling.Data.Buckets) != 2 || byBilling.Data.Buckets[0].Label != "subscription" || byBilling.Data.Buckets[0].Requests != 3 {
		t.Fatalf("distribution billing_mode: unexpected buckets %+v", byBilling.Data.Buckets)
	}

	// limit caps to the single top bucket.
	var capped struct {
		Data apiopenapi.UsageDistributionResult `json:"data"`
	}
	cappedRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/distribution?dimension=model&limit=1")
	if cappedRec.Code != http.StatusOK {
		t.Fatalf("distribution limit=1: expected 200, got %d", cappedRec.Code)
	}
	if err := json.NewDecoder(cappedRec.Body).Decode(&capped); err != nil {
		t.Fatalf("decode distribution limit: %v", err)
	}
	if len(capped.Data.Buckets) != 1 || capped.Data.Buckets[0].Label != "alpha" {
		t.Fatalf("distribution limit=1: expected only alpha, got %+v", capped.Data.Buckets)
	}

	// Invalid params and reversed ranges must 400.
	for _, badPath := range []string{
		"/api/v1/admin/usage/distribution?dimension=galaxy",
		"/api/v1/admin/usage/distribution?metric=vibes",
		"/api/v1/admin/usage/distribution?limit=0",
		"/api/v1/admin/usage/distribution?limit=abc",
		"/api/v1/admin/usage/distribution?start=2026-06-10&end=2026-06-01",
	} {
		badRec := doAdminGet(t, handler, sessionCookie, badPath)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("distribution %s: expected 400, got %d body=%s", badPath, badRec.Code, badRec.Body.String())
		}
	}

	// Anonymous (no admin session) must be forbidden.
	anonReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/distribution", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusForbidden {
		t.Fatalf("distribution without admin session: expected 403, got %d", anonRec.Code)
	}
}
