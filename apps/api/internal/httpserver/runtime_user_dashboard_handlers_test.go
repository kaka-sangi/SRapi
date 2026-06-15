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

// TestCurrentUserUsageDashboardAggregates exercises the four end-user dashboard
// aggregate endpoints against a single seeded console user: it asserts the
// throughput window math, model-share grouping/ordering, dense day-bucketed
// trend series and prompt-cache rollups, plus the shared 401 path.
func TestCurrentUserUsageDashboardAggregates(t *testing.T) {
	usageStore := usagememory.New()
	handler := New(config.Load(), nil, WithUsageStore(usageStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	userID := apiIDToIntForTest(t, loginResp.Data.User.Id)

	now := time.Now().UTC()
	// Two requests inside the 60-minute throughput window in the same minute so
	// the per-minute peak is distinguishable from the averaged rate, plus an
	// older request that only the model/cache rollups should pick up.
	recent := now.Add(-2 * time.Minute).Truncate(time.Minute).Add(30 * time.Second)
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:           "req_user_dash_1",
		UserID:              userID,
		Model:               "model-a",
		InputTokens:         10,
		OutputTokens:        5,
		CachedTokens:        4,
		CacheCreationTokens: 2,
		TotalTokens:         15,
		Success:             true,
		Cost:                "0.10000000",
		CacheReadCost:       "0.01000000",
		Currency:            "USD",
		CreatedAt:           recent,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:           "req_user_dash_2",
		UserID:              userID,
		Model:               "model-a",
		InputTokens:         20,
		OutputTokens:        10,
		CachedTokens:        6,
		CacheCreationTokens: 3,
		TotalTokens:         30,
		Success:             true,
		Cost:                "0.20000000",
		CacheReadCost:       "0.02000000",
		Currency:            "USD",
		CreatedAt:           recent,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:    "req_user_dash_3",
		UserID:       userID,
		Model:        "model-b",
		InputTokens:  1,
		OutputTokens: 1,
		TotalTokens:  2,
		Success:      true,
		Cost:         "0.05000000",
		Currency:     "USD",
		// Within the model/trend look-back but outside the 60-minute window.
		CreatedAt: now.Add(-3 * time.Hour),
	})
	// A different user's log must never leak into the current-user rollups.
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:    "req_other_user",
		UserID:       userID + 1000,
		Model:        "model-z",
		InputTokens:  999,
		OutputTokens: 999,
		TotalTokens:  1998,
		Success:      true,
		Cost:         "9.99000000",
		Currency:     "USD",
		CreatedAt:    recent,
	})

	// 401 when unauthenticated (shared by every dashboard endpoint).
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, httptest.NewRequest(http.MethodGet, "/api/v1/user/usage/dashboard/throughput", nil))
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected throughput 401, got %d body=%s", unauthRec.Code, unauthRec.Body.String())
	}

	// Throughput: only the two in-window requests count.
	var throughput apiopenapi.UsageThroughputResponse
	doDashboardGet(t, handler, sessionCookie, "/api/v1/user/usage/dashboard/throughput", &throughput)
	if throughput.Data.WindowMinutes != 60 {
		t.Fatalf("expected 60-minute window, got %d", throughput.Data.WindowMinutes)
	}
	if throughput.Data.TotalRequests != 2 || throughput.Data.TotalTokens != 45 {
		t.Fatalf("unexpected throughput totals: %+v", throughput.Data)
	}
	if throughput.Data.PeakRpm != 2 || throughput.Data.PeakTpm != 45 {
		t.Fatalf("expected single-minute peaks rpm=2 tpm=45, got %+v", throughput.Data)
	}
	if throughput.Data.Rpm != float32(2)/float32(60) || throughput.Data.Tpm != float32(45)/float32(60) {
		t.Fatalf("unexpected averaged rates: %+v", throughput.Data)
	}

	// Models: both models grouped, model-a (more requests) ordered first.
	var models apiopenapi.UsageModelShareListResponse
	doDashboardGet(t, handler, sessionCookie, "/api/v1/user/usage/dashboard/models", &models)
	if len(models.Data) != 2 {
		t.Fatalf("expected two model rows, got %+v", models.Data)
	}
	first := models.Data[0]
	if first.Model != "model-a" || first.Requests != 2 || first.InputTokens != 30 || first.OutputTokens != 15 || first.TotalTokens != 45 || first.Cost != "0.30" || first.Currency != "USD" {
		t.Fatalf("unexpected leading model row: %+v", first)
	}
	if models.Data[1].Model != "model-b" || models.Data[1].Requests != 1 {
		t.Fatalf("unexpected trailing model row: %+v", models.Data[1])
	}

	// Trend: dense day series, today's bucket holds all three current-user logs.
	var trend apiopenapi.UsageTrendPointListResponse
	doDashboardGet(t, handler, sessionCookie, "/api/v1/user/usage/dashboard/trend?days=7&bucket=day", &trend)
	if len(trend.Data) != 7 {
		t.Fatalf("expected 7 dense day buckets, got %d", len(trend.Data))
	}
	todayKey := now.Format("2006-01-02")
	todayPoint := trend.Data[len(trend.Data)-1]
	if todayPoint.Bucket != todayKey {
		t.Fatalf("expected last bucket %q, got %q", todayKey, todayPoint.Bucket)
	}
	if todayPoint.Requests != 3 || todayPoint.InputTokens != 31 || todayPoint.OutputTokens != 16 || todayPoint.Cost != "0.35" {
		t.Fatalf("unexpected today trend point: %+v", todayPoint)
	}

	// Cache metrics: cache-read/creation/input rollups across all current-user logs.
	var cache apiopenapi.UsageCacheMetricsResponse
	doDashboardGet(t, handler, sessionCookie, "/api/v1/user/usage/dashboard/cache-metrics", &cache)
	if cache.Data.CacheReadTokens != 10 || cache.Data.CacheCreationTokens != 5 || cache.Data.TotalInputTokens != 31 {
		t.Fatalf("unexpected cache token rollups: %+v", cache.Data)
	}
	if cache.Data.CacheCostSaved != "0.03" || cache.Data.Currency != "USD" {
		t.Fatalf("unexpected cache cost saved: %+v", cache.Data)
	}
	// cache_hit_rate = cache_read / (cache_read + input) = 10 / 41.
	if want := float32(10) / float32(41); cache.Data.CacheHitRate != want {
		t.Fatalf("expected cache hit rate %v, got %v", want, cache.Data.CacheHitRate)
	}

	// Bad query params are rejected before the store is touched.
	badRec := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/user/usage/dashboard/trend?bucket=week", nil)
	badReq.AddCookie(sessionCookie)
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid bucket 400, got %d body=%s", badRec.Code, badRec.Body.String())
	}
}

func doDashboardGet(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, path string, out any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s expected 200, got %d body=%s", path, rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
		t.Fatalf("decode %s response: %v", path, err)
	}
}
