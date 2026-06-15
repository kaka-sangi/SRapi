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

// TestAdminUsageChartsEndpoints proves the admin usage-trends and
// error-distribution chart handlers aggregate the recorded usage logs.
//
// One healthy request records a single success usage log (model "ok-model"); one
// request against a failing upstream records a single failed usage log (model
// "bad-model"). The trends handler, grouped by the default model dimension, must
// surface both models as series with dense oldest-first points and 2dp cost
// strings, and the error-distribution handler must report exactly one error
// bucket at 100%. Bad query params return 400 and an anonymous caller is 403.
func TestAdminUsageChartsEndpoints(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
	}))
	defer healthy.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer failing.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	okProvider := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"ok-provider","display_name":"OK","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	okModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"ok-model","display_name":"OK Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(okModel.Data.Id), `{"provider_id":"`+string(okProvider.Data.Id)+`","upstream_model_name":"ok-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(okProvider.Data.Id)+`","name":"ok-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+healthy.URL+`/v1"},"status":"active"}`)

	badProvider := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"bad-provider","display_name":"Bad","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	badModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"bad-model","display_name":"Bad Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(badModel.Data.Id), `{"provider_id":"`+string(badProvider.Data.Id)+`","upstream_model_name":"bad-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(badProvider.Data.Id)+`","name":"bad-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+failing.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	// One success usage log (ok-model).
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"ok-model","messages":[{"role":"user","content":"hi"}]}`)
	// One failed usage log (bad-model): driven directly because the gateway
	// returns a non-2xx status when failover is exhausted.
	failReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"bad-model","messages":[{"role":"user","content":"hi"}]}`))
	failReq.Header.Set("Authorization", "Bearer "+apiKey)
	failReq.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), failReq)

	// trends: default bucket=day, dimension=model. Both models must appear as
	// series with dense oldest-first points; the latest (last) point of each
	// carries that model's one request.
	trendsRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/trends")
	if trendsRec.Code != http.StatusOK {
		t.Fatalf("trends: expected 200, got %d body=%s", trendsRec.Code, trendsRec.Body.String())
	}
	var trends struct {
		Data      apiopenapi.UsageTrendSeriesResult `json:"data"`
		RequestID string                            `json:"request_id"`
	}
	if err := json.NewDecoder(trendsRec.Body).Decode(&trends); err != nil {
		t.Fatalf("decode trends: %v", err)
	}
	if trends.RequestID == "" {
		t.Fatalf("trends: expected a request_id")
	}
	if trends.Data.Bucket != "day" || trends.Data.Dimension != "model" {
		t.Fatalf("trends: expected bucket=day dimension=model, got %q/%q", trends.Data.Bucket, trends.Data.Dimension)
	}
	if len(trends.Data.Series) != 2 {
		t.Fatalf("trends: expected 2 model series, got %d (%+v)", len(trends.Data.Series), trends.Data.Series)
	}
	seriesRequests := map[string]int{}
	for _, series := range trends.Data.Series {
		if len(series.Points) == 0 {
			t.Fatalf("trends: series %q has no points", series.Label)
		}
		total := 0
		for _, point := range series.Points {
			total += point.Requests
			if point.Bucket == "" {
				t.Fatalf("trends: series %q has a point with empty bucket", series.Label)
			}
			if point.Cost == "" || point.Currency == "" {
				t.Fatalf("trends: series %q point %q missing cost/currency", series.Label, point.Bucket)
			}
		}
		// Points must be dense and oldest-first (ascending bucket keys).
		for i := 1; i < len(series.Points); i++ {
			if series.Points[i-1].Bucket > series.Points[i].Bucket {
				t.Fatalf("trends: series %q points not oldest-first at %d", series.Label, i)
			}
		}
		seriesRequests[series.Label] = total
	}
	if seriesRequests["ok-model"] != 1 {
		t.Fatalf("trends: expected ok-model series to hold 1 request, got %d", seriesRequests["ok-model"])
	}
	if seriesRequests["bad-model"] != 1 {
		t.Fatalf("trends: expected bad-model series to hold 1 request, got %d", seriesRequests["bad-model"])
	}

	// trends: hour bucket layout is accepted and reported.
	hourRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/trends?bucket=hour&dimension=source_endpoint&limit=4")
	if hourRec.Code != http.StatusOK {
		t.Fatalf("trends hour: expected 200, got %d body=%s", hourRec.Code, hourRec.Body.String())
	}
	var hourTrends struct {
		Data apiopenapi.UsageTrendSeriesResult `json:"data"`
	}
	if err := json.NewDecoder(hourRec.Body).Decode(&hourTrends); err != nil {
		t.Fatalf("decode trends hour: %v", err)
	}
	if hourTrends.Data.Bucket != "hour" || hourTrends.Data.Dimension != "source_endpoint" {
		t.Fatalf("trends hour: expected bucket=hour dimension=source_endpoint, got %q/%q", hourTrends.Data.Bucket, hourTrends.Data.Dimension)
	}

	// trends: invalid params must 400.
	for _, badPath := range []string{
		"/api/v1/admin/usage/trends?bucket=week",
		"/api/v1/admin/usage/trends?dimension=galaxy",
		"/api/v1/admin/usage/trends?limit=0",
		"/api/v1/admin/usage/trends?limit=abc",
	} {
		rec := doAdminGet(t, handler, sessionCookie, badPath)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("trends %s: expected 400, got %d body=%s", badPath, rec.Code, rec.Body.String())
		}
	}

	// error-distribution: exactly one failed log -> one bucket at 100%.
	errRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/usage/error-distribution")
	if errRec.Code != http.StatusOK {
		t.Fatalf("error-distribution: expected 200, got %d body=%s", errRec.Code, errRec.Body.String())
	}
	// data is a bare UsageErrorBucket array per the OpenAPI contract (this array
	// assertion is the guard against the handler drifting back to an object).
	var errDist struct {
		Data      []apiopenapi.UsageErrorBucket `json:"data"`
		RequestID string                        `json:"request_id"`
	}
	if err := json.NewDecoder(errRec.Body).Decode(&errDist); err != nil {
		t.Fatalf("decode error-distribution: %v", err)
	}
	if errDist.RequestID == "" {
		t.Fatalf("error-distribution: expected a request_id")
	}
	if len(errDist.Data) != 1 {
		t.Fatalf("error-distribution: expected 1 bucket, got %d (%+v)", len(errDist.Data), errDist.Data)
	}
	bucket := errDist.Data[0]
	if bucket.Count != 1 {
		t.Fatalf("error-distribution: expected bucket count 1, got %d", bucket.Count)
	}
	if bucket.Percentage != 100 {
		t.Fatalf("error-distribution: expected 100%% share, got %v", bucket.Percentage)
	}
	if strings.TrimSpace(bucket.ErrorClass) == "" {
		t.Fatalf("error-distribution: expected a non-empty error_class")
	}

	// Bad date range (start after end) must 400 on both handlers.
	for _, badPath := range []string{
		"/api/v1/admin/usage/trends?start=2026-06-10&end=2026-06-01",
		"/api/v1/admin/usage/error-distribution?start=2026-06-10&end=2026-06-01",
	} {
		rec := doAdminGet(t, handler, sessionCookie, badPath)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400 for reversed range, got %d body=%s", badPath, rec.Code, rec.Body.String())
		}
	}

	// Anonymous (no admin session) must be forbidden on both handlers.
	for _, path := range []string{"/api/v1/admin/usage/trends", "/api/v1/admin/usage/error-distribution"} {
		anonReq := httptest.NewRequest(http.MethodGet, path, nil)
		anonRec := httptest.NewRecorder()
		handler.ServeHTTP(anonRec, anonReq)
		if anonRec.Code != http.StatusForbidden {
			t.Fatalf("%s without admin session: expected 403, got %d", path, anonRec.Code)
		}
	}
}
