package httpserver

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	alipaysdk "github.com/smartwalle/alipay/v3"
	"github.com/srapi/srapi/apps/api/internal/config"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	paymentcontract "github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

func TestMain(m *testing.M) {
	original, hadOriginal := os.LookupEnv("STORAGE_BACKEND")
	_ = os.Setenv("STORAGE_BACKEND", config.StorageBackendMemory)
	code := m.Run()
	if hadOriginal {
		_ = os.Setenv("STORAGE_BACKEND", original)
	} else {
		_ = os.Unsetenv("STORAGE_BACKEND")
	}
	os.Exit(code)
}

type upstreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func TestHealthIncludesRequestIDAndDependencies(t *testing.T) {
	handler := New(config.Load(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("X-Request-ID", "req_test")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if got := response.Header().Get("X-Request-ID"); got != "req_test" {
		t.Fatalf("expected request id header to be preserved, got %q", got)
	}

	var body apiopenapi.HealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestId != "req_test" {
		t.Fatalf("expected body request id req_test, got %q", body.RequestId)
	}
	if body.Data.Dependencies.Database == "" {
		t.Fatal("expected database dependency status")
	}
	if body.Data.Dependencies.Redis == "" {
		t.Fatal("expected redis dependency status")
	}
}

func TestLivezDoesNotRequireDependencies(t *testing.T) {
	handler := New(config.Load(), nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected generated request id")
	}
}

func TestRuntimeRequiresStoresUnlessMemoryBackendIsExplicit(t *testing.T) {
	cfg := config.Load()
	cfg.Storage.Backend = config.StorageBackendPostgres

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected runtime initialization to fail without injected stores")
		}
		if !strings.Contains(fmt.Sprint(recovered), "missing users store") {
			t.Fatalf("expected missing users store error, got %v", recovered)
		}
	}()

	_ = New(cfg, nil)
}

func TestGatewayMaxBodySizeRejectsOversizedJSON(t *testing.T) {
	cfg := config.Load()
	cfg.Gateway.MaxBodySize = int64(len(`{"email":"a","password":"b"}`) - 1)
	handler := New(cfg, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a","password":"b"}`))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized request, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestReadyzFailsWhenDependenciesAreUnavailable(t *testing.T) {
	cfg := config.Load()
	cfg.Database.Host = "127.0.0.1"
	cfg.Database.Port = 1
	cfg.Redis.Host = "127.0.0.1"
	cfg.Redis.Port = 1
	handler := New(cfg, nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}

func TestReadyzUsesInjectedDependencyProbes(t *testing.T) {
	handler := New(config.Load(), nil,
		WithDatabasePinger(stubDependencyPinger{err: nil}),
		WithRedisPinger(stubDependencyPinger{err: errors.New("redis unavailable")}),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}

	var body apiopenapi.HealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Dependencies.Database != apiopenapi.HealthDependencyStatusOk {
		t.Fatalf("expected database dependency ok, got %s", body.Data.Dependencies.Database)
	}
	if body.Data.Dependencies.Redis != apiopenapi.HealthDependencyStatusUnavailable {
		t.Fatalf("expected redis dependency unavailable, got %s", body.Data.Dependencies.Redis)
	}
}

func TestMetricsExposeBaselineSRapiSignals(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"metrics smoke"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected gateway success 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d", metricsRec.Code)
	}
	body := metricsRec.Body.String()
	for _, metric := range []string{
		"srapi_gateway_requests_total",
		"srapi_gateway_request_duration_seconds_bucket",
		"srapi_gateway_request_duration_seconds_count",
		"srapi_gateway_request_duration_seconds_sum",
		"srapi_gateway_inflight_requests",
		"srapi_gateway_errors_total",
		"srapi_gateway_failover_total",
		"srapi_scheduler_decisions_total",
		"srapi_scheduler_cost_score_avg",
		"srapi_provider_errors_total",
		"srapi_provider_probe_latency_seconds_bucket",
		"srapi_usage_tokens_total",
		"srapi_reverse_proxy_ban_signals_total",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("expected metrics body to contain %s, got:\n%s", metric, body)
		}
	}
	if !strings.Contains(body, `srapi_gateway_requests_total{endpoint_family="chat_completions",model="gpt-4o-mini",provider_protocol="openai-compatible",result="success"} 1`) {
		t.Fatalf("expected gateway request metric, got:\n%s", body)
	}
	for _, bucket := range []string{`le="0.05"`, `le="0.1"`, `le="0.25"`, `le="0.5"`, `le="1"`, `le="2.5"`, `le="5"`, `le="10"`, `le="+Inf"`} {
		if !strings.Contains(body, `srapi_gateway_request_duration_seconds_bucket{endpoint_family="chat_completions",model="gpt-4o-mini",provider_protocol="openai-compatible",result="success",`+bucket+`}`) {
			t.Fatalf("expected gateway duration bucket %s, got:\n%s", bucket, body)
		}
		if !strings.Contains(body, `srapi_provider_probe_latency_seconds_bucket{provider_protocol="openai-compatible",status="healthy",`+bucket+`}`) {
			t.Fatalf("expected provider probe latency bucket %s, got:\n%s", bucket, body)
		}
	}
	if !strings.Contains(body, `srapi_usage_tokens_total{model="gpt-4o-mini",provider_protocol="openai-compatible",token_kind="input"}`) {
		t.Fatalf("expected usage token metric, got:\n%s", body)
	}
}

func TestGatewayEnforcesAPIKeyRPMLimit(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	handler := New(config.Load(), nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"limited-gateway","scopes":["gateway:invoke"],"rpm_limit":1}`)
	apiKey := keyResp.Data.PlaintextKey

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"first limited request"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"second limited request"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second gateway request 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if secondRec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on rate limited gateway request")
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(secondRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode rate limit error response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != "rpm_limit_exceeded" || errResp.Error.Type != apiopenapi.RateLimitError {
		t.Fatalf("unexpected rate limit error response: %+v", errResp)
	}
}

func TestGatewayEnforcesAPIKeyConcurrencyLimit(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var startOnce sync.Once
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseUpstream) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		startOnce.Do(func() {
			close(upstreamStarted)
			<-releaseUpstream
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"concurrency ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	cfg := config.Load()
	cfg.Gateway.RequestTimeout = time.Minute
	handler := New(cfg, nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"api-key-concurrency-provider","display_name":"API Key Concurrency","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"api-key-concurrency-model","display_name":"API Key Concurrency Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"api-key-concurrency-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"api-key-concurrency-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"limited-concurrency","scopes":["gateway:invoke"],"concurrency_limit":1}`)
	if keyResp.Data.ApiKey.ConcurrencyLimit == nil || *keyResp.Data.ApiKey.ConcurrencyLimit != 1 {
		t.Fatalf("expected API key concurrency_limit 1, got %+v", keyResp.Data.ApiKey.ConcurrencyLimit)
	}
	apiKey := keyResp.Data.PlaintextKey

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"api-key-concurrency-model","messages":[{"role":"user","content":"first"}]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		firstDone <- rec
	}()

	select {
	case <-upstreamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first gateway request to reach upstream")
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"api-key-concurrency-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected concurrent gateway request 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if secondRec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on concurrency limited gateway request")
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(secondRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode concurrency limit error response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != "concurrency_limit_exceeded" || errResp.Error.Type != apiopenapi.RateLimitError {
		t.Fatalf("unexpected concurrency limit error response: %+v", errResp)
	}

	releaseOnce.Do(func() { close(releaseUpstream) })
	var firstRec *httptest.ResponseRecorder
	select {
	case firstRec = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first gateway request to finish")
	}
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"api-key-concurrency-model","messages":[{"role":"user","content":"third"}]}`))
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdReq.Header.Set("Authorization", "Bearer "+apiKey)
	thirdRec := httptest.NewRecorder()
	handler.ServeHTTP(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusOK {
		t.Fatalf("expected released concurrency slot to allow third request 200, got %d body=%s", thirdRec.Code, thirdRec.Body.String())
	}
}

func TestGatewayEnforcesProviderAccountConcurrencyAcrossNodes(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var startOnce sync.Once
	var releaseOnce sync.Once
	var upstreamMu sync.Mutex
	upstreamHits := 0
	defer releaseOnce.Do(func() { close(releaseUpstream) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		upstreamMu.Lock()
		upstreamHits++
		hitNo := upstreamHits
		upstreamMu.Unlock()
		if hitNo == 1 {
			startOnce.Do(func() {
				close(upstreamStarted)
				<-releaseUpstream
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"account concurrency ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	cfg := config.Load()
	cfg.Gateway.RequestTimeout = time.Minute
	node := func(t *testing.T, suffix string) (http.Handler, string) {
		t.Helper()
		handler := New(cfg, nil, WithRateLimiter(limiter))
		loginResp, sessionCookie := mustLoginAdmin(t, handler)
		providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-concurrency-provider-`+suffix+`","display_name":"Account Concurrency","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
		modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"account-concurrency-model","display_name":"Account Concurrency Model","status":"active"}`)
		mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"account-concurrency-upstream","status":"active"}`)
		mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"account-concurrency-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","max_concurrency":1},"status":"active"}`)
		keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-concurrency-key","scopes":["gateway:invoke"]}`)
		return handler, keyResp.Data.PlaintextKey
	}
	firstNode, firstKey := node(t, "a")
	secondNode, secondKey := node(t, "b")

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-concurrency-model","messages":[{"role":"user","content":"first"}]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+firstKey)
		rec := httptest.NewRecorder()
		firstNode.ServeHTTP(rec, req)
		firstDone <- rec
	}()

	select {
	case <-upstreamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first gateway request to reach upstream")
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-concurrency-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+secondKey)
	secondRec := httptest.NewRecorder()
	secondNode.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second node account concurrency rejection 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if secondRec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on account concurrency rejection")
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(secondRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode account concurrency response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != "concurrency_limit_exceeded" || errResp.Error.Type != apiopenapi.RateLimitError {
		t.Fatalf("unexpected account concurrency response: %+v", errResp)
	}
	upstreamMu.Lock()
	hitsWhileLimited := upstreamHits
	upstreamMu.Unlock()
	if hitsWhileLimited != 1 {
		t.Fatalf("expected account concurrency to block second upstream dispatch, upstream hits=%d", hitsWhileLimited)
	}

	releaseOnce.Do(func() { close(releaseUpstream) })
	var firstRec *httptest.ResponseRecorder
	select {
	case firstRec = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first gateway request to finish")
	}
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-concurrency-model","messages":[{"role":"user","content":"third"}]}`))
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdReq.Header.Set("Authorization", "Bearer "+secondKey)
	thirdRec := httptest.NewRecorder()
	secondNode.ServeHTTP(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusOK {
		t.Fatalf("expected released account concurrency slot to allow third request 200, got %d body=%s", thirdRec.Code, thirdRec.Body.String())
	}
}

func TestGatewayGeminiRateLimitUsesGoogleEnvelopeAndRetryAfter(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"gemini ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"limited-gemini","scopes":["gateway:invoke"],"rpm_limit":1}`)
	apiKey := keyResp.Data.PlaintextKey
	body := `{"contents":[{"role":"user","parts":[{"text":"gemini limited prompt"}]}]}`

	firstReq := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-route-model:generateContent", strings.NewReader(body))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first gemini gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-route-model:generateContent", strings.NewReader(body))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second gemini gateway request 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if secondRec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on rate limited gemini request")
	}
	var errResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(secondRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode gemini rate limit error response: %v", err)
	}
	if errResp.Error.Code != http.StatusTooManyRequests || errResp.Error.Status != "RESOURCE_EXHAUSTED" {
		t.Fatalf("unexpected gemini rate limit error response: %+v", errResp)
	}
}

func TestAuthAndGatewayFlow(t *testing.T) {
	handler := New(config.Load(), nil)

	loginBody := `{"email":"admin@srapi.local","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 login, got %d", loginRec.Code)
	}

	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Data.CsrfToken == "" {
		t.Fatal("expected csrf token in login response")
	}
	if len(loginRec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
	sessionCookie := loginRec.Result().Cookies()[0]
	if sessionCookie.Name != sessionCookieName || !sessionCookie.HttpOnly || sessionCookie.Path != "/" || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.Secure {
		t.Fatalf("unexpected local session cookie attributes: %+v", sessionCookie)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected 200 me, got %d", meRec.Code)
	}

	var meResp apiopenapi.UserResponse
	if err := json.NewDecoder(meRec.Body).Decode(&meResp); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if meResp.Data.Email != "admin@srapi.local" {
		t.Fatalf("expected admin email, got %s", meResp.Data.Email)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/balance", nil)
	balanceReq.AddCookie(sessionCookie)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected 200 balance, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp apiopenapi.UserBalanceResponse
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if balanceResp.Data.UserId != meResp.Data.Id || balanceResp.Data.Balance != meResp.Data.Balance || balanceResp.Data.Currency != meResp.Data.Currency {
		t.Fatalf("expected current user balance snapshot, got %+v for user %+v", balanceResp.Data, meResp.Data)
	}

	createBody := `{"name":"default","scopes":["gateway:invoke"]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(sessionCookie)
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create api key, got %d", createRec.Code)
	}

	var createResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}
	if createResp.Data.PlaintextKey == "" {
		t.Fatal("expected plaintext key in create response")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 api keys list, got %d", listRec.Code)
	}
	var listResp apiopenapi.ApiKeyListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode api keys list: %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatal("expected at least one api key")
	}
	if body := listRec.Body.String(); strings.Contains(body, createResp.Data.PlaintextKey) || strings.Contains(body, "plaintext_key") || strings.Contains(body, "hmac-sha256") || strings.Contains(body, `"hash"`) {
		t.Fatalf("api key list leaked secret material: %s", body)
	}
	keyID := createResp.Data.ApiKey.Id
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+string(keyID), strings.NewReader(`{"name":"renamed","status":"disabled","allowed_models":["gpt-4o-mini"],"group_ids":["2","3"]}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(sessionCookie)
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 update api key, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	if strings.Contains(updateRec.Body.String(), "plaintext_key") {
		t.Fatalf("update response must not include plaintext key: %s", updateRec.Body.String())
	}
	var updateResp apiopenapi.ApiKeyResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode api key update response: %v", err)
	}
	if updateResp.Data.Name != "renamed" || updateResp.Data.Status != apiopenapi.ApiKeyStatusDisabled {
		t.Fatalf("unexpected updated api key: %+v", updateResp.Data)
	}
	if len(updateResp.Data.GroupIds) != 2 || updateResp.Data.GroupIds[0] != "2" || updateResp.Data.GroupIds[1] != "3" {
		t.Fatalf("unexpected updated group ids: %+v", updateResp.Data.GroupIds)
	}

	modelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+createResp.Data.PlaintextKey)
	modelsRec := httptest.NewRecorder()
	handler.ServeHTTP(modelsRec, modelsReq)
	if modelsRec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled api key to be rejected with 403, got %d", modelsRec.Code)
	}

	enableReq := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+string(keyID), strings.NewReader(`{"status":"active"}`))
	enableReq.Header.Set("Content-Type", "application/json")
	enableReq.AddCookie(sessionCookie)
	enableReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("expected 200 enable api key, got %d body=%s", enableRec.Code, enableRec.Body.String())
	}

	modelsReq = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+createResp.Data.PlaintextKey)
	modelsRec = httptest.NewRecorder()
	handler.ServeHTTP(modelsRec, modelsReq)
	if modelsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 models, got %d", modelsRec.Code)
	}

	var modelsResp apiopenapi.OpenAIModelList
	if err := json.NewDecoder(modelsRec.Body).Decode(&modelsResp); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if len(modelsResp.Data) == 0 {
		t.Fatal("expected seeded model list")
	}

	expiredReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"expired","scopes":["gateway:invoke"],"expires_at":"2000-01-01T00:00:00Z"}`))
	expiredReq.Header.Set("Content-Type", "application/json")
	expiredReq.AddCookie(sessionCookie)
	expiredReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	expiredRec := httptest.NewRecorder()
	handler.ServeHTTP(expiredRec, expiredReq)
	if expiredRec.Code != http.StatusCreated {
		t.Fatalf("expected expired api key create 201, got %d body=%s", expiredRec.Code, expiredRec.Body.String())
	}
	var expiredResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(expiredRec.Body).Decode(&expiredResp); err != nil {
		t.Fatalf("decode expired api key response: %v", err)
	}
	expiredModelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	expiredModelsReq.Header.Set("Authorization", "Bearer "+expiredResp.Data.PlaintextKey)
	expiredModelsRec := httptest.NewRecorder()
	handler.ServeHTTP(expiredModelsRec, expiredModelsReq)
	if expiredModelsRec.Code != http.StatusForbidden {
		t.Fatalf("expected expired api key to be rejected with 403, got %d body=%s", expiredModelsRec.Code, expiredModelsRec.Body.String())
	}
	if !strings.Contains(expiredModelsRec.Body.String(), "api_key_disabled") {
		t.Fatalf("expected stable disabled-or-expired error code, got %s", expiredModelsRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	createAudit := mustFindAuditLog(t, auditResp.Data, "api_key.create")
	if _, ok := createAudit.After["hash"]; ok {
		t.Fatalf("api key create audit must not expose hash: %+v", createAudit.After)
	}
	if _, ok := createAudit.After["plaintext_key"]; ok {
		t.Fatalf("api key create audit must not expose plaintext key: %+v", createAudit.After)
	}
	updateAudit := mustFindAuditLog(t, auditResp.Data, "api_key.update")
	if updateAudit.Before["name"] != "default" || updateAudit.After["name"] != "renamed" {
		t.Fatalf("expected api key audit before/after name change, got before=%+v after=%+v", updateAudit.Before, updateAudit.After)
	}
	if _, ok := updateAudit.After["hash"]; ok {
		t.Fatalf("api key audit must not expose hash: %+v", updateAudit.After)
	}
}

func TestConsoleWriteRoutesRequireCSRF(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"csrf-provider","display_name":"CSRF Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"csrf-model","display_name":"CSRF Model","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"csrf-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"csrf-group","description":"CSRF Group","status":"active"}`)
	apiKeyResp, _ := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	writeRoutes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/auth/logout", `{}`},
		{http.MethodPost, "/api/v1/api-keys", `{"name":"blocked"}`},
		{http.MethodPatch, "/api/v1/api-keys/" + string(apiKeyResp.Data.ApiKey.Id), `{"name":"blocked"}`},
		{http.MethodPost, "/api/v1/admin/providers", `{"name":"blocked","display_name":"Blocked","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`},
		{http.MethodPatch, "/api/v1/admin/providers/" + string(providerResp.Data.Id), `{"display_name":"Blocked"}`},
		{http.MethodPost, "/api/v1/admin/providers/" + string(providerResp.Data.Id) + "/test", `{}`},
		{http.MethodPost, "/api/v1/admin/models", `{"canonical_name":"blocked-model","display_name":"Blocked Model","status":"active"}`},
		{http.MethodPatch, "/api/v1/admin/models/" + string(modelResp.Data.Id), `{"display_name":"Blocked Model"}`},
		{http.MethodPost, "/api/v1/admin/models/" + string(modelResp.Data.Id) + "/aliases", `{"alias":"blocked-alias","status":"active"}`},
		{http.MethodPost, "/api/v1/admin/models/" + string(modelResp.Data.Id) + "/mappings", `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"blocked","status":"active"}`},
		{http.MethodPost, "/api/v1/admin/accounts", `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"blocked-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`},
		{http.MethodPost, "/api/v1/admin/accounts/import", `{"accounts":[{"provider_id":"` + string(providerResp.Data.Id) + `","name":"blocked-import","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}]}`},
		{http.MethodPatch, "/api/v1/admin/accounts/" + string(accountResp.Data.Id), `{"name":"blocked-account"}`},
		{http.MethodPatch, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/proxy", `{"proxy_id":"proxy-blocked"}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/test", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/disable", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/enable", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/recover", `{}`},
		{http.MethodPost, "/api/v1/admin/account-groups", `{"name":"blocked-group","status":"active"}`},
		{http.MethodPatch, "/api/v1/admin/account-groups/" + string(groupResp.Data.Id), `{"description":"blocked"}`},
		{http.MethodPost, "/api/v1/admin/account-groups/" + string(groupResp.Data.Id) + "/accounts/" + string(accountResp.Data.Id), `{}`},
		{http.MethodDelete, "/api/v1/admin/account-groups/" + string(groupResp.Data.Id) + "/accounts/" + string(accountResp.Data.Id), `{}`},
		{http.MethodPost, "/api/v1/admin/subscription-plans", `{"name":"blocked-plan","price":"1.00","currency":"USD","validity_days":30}`},
		{http.MethodPost, "/api/v1/admin/user-subscriptions", `{"user_id":"` + string(loginResp.Data.User.Id) + `","plan_id":"1"}`},
		{http.MethodPost, "/api/v1/admin/pricing-rules", `{"model_id":"` + string(modelResp.Data.Id) + `","provider_id":"` + string(providerResp.Data.Id) + `","input_price_per_million_tokens":"1","output_price_per_million_tokens":"2","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"USD"}`},
		{http.MethodPost, "/api/v1/admin/pricing-rules:bulk", `{"items":[{"model_id":"` + string(modelResp.Data.Id) + `","provider_id":"` + string(providerResp.Data.Id) + `","input_price_per_million_tokens":"1","output_price_per_million_tokens":"2","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"USD"}]}`},
		{http.MethodPut, "/api/v1/admin/settings", `{"general":{"site_name":"SRapi","logo_url":"","version_label":"","custom_menus":[]},"agreement":{"user_agreement":"","privacy_policy":""},"features":{"enabled_channels":[],"channel_monitoring_enabled":true,"invitation_rebate_enabled":false,"payments_enabled":false},"security":{"admin_api_key":{"configured":false},"registration_enabled":true,"oauth_enabled":false,"oauth_providers":[]},"users":{"default_balance":"0","default_group":"default","user_self_delete_enabled":false,"rpm_limit_default":0},"gateway":{"overload_cooldown_seconds":30,"rate_limit_cooldown_seconds":30,"stream_timeout_seconds":600,"request_shaper_enabled":true,"beta_strategy":"allow_configured"},"payment":{"enabled":false,"providers":[],"subscription_plans_enabled":false},"email":{"smtp_configured":false,"templates":{}},"backup":{"enabled":false,"retention_days":30}}`},
		{http.MethodPost, "/api/v1/admin/announcements", `{"title":"Hello","content":"World"}`},
		{http.MethodPost, "/api/v1/admin/redeem-codes", `{"code":"CSRF-REDEEM","type":"balance","value":"1.00"}`},
		{http.MethodPost, "/api/v1/admin/promo-codes", `{"code":"CSRF-PROMO","discount_type":"amount","discount_value":"1.00"}`},
		{http.MethodPut, "/api/v1/admin/risk-control/config", `{"enabled":true,"mode":"monitor","max_failed_requests_per_minute":10,"max_cost_per_day":"100","cooldown_seconds":60,"blocked_countries":[],"blocked_ips":[]}`},
		{http.MethodPost, "/api/v1/admin/ops/slo", `{"name":"blocked-slo","objective":99}`},
		{http.MethodPatch, "/api/v1/admin/ops/slo/1", `{"status":"disabled"}`},
		{http.MethodPost, "/api/v1/admin/ops/alerts/1/ack", `{}`},
	}

	for _, route := range writeRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s without csrf: expected 403, got %d body=%s", route.method, route.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAdminControlPlaneV1EndpointsAndAudit(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, _ = mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	settingsReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(`{"general":{"site_name":"SRapi Console","logo_url":"","version_label":"v1","custom_menus":[]},"agreement":{"user_agreement":"terms","privacy_policy":"privacy"},"features":{"enabled_channels":["openai-compatible"],"channel_monitoring_enabled":true,"invitation_rebate_enabled":false,"payments_enabled":false},"security":{"admin_api_key":{"configured":false},"registration_enabled":true,"oauth_enabled":false,"oauth_providers":[]},"users":{"default_balance":"0","default_group":"default","user_self_delete_enabled":false,"rpm_limit_default":60},"gateway":{"overload_cooldown_seconds":30,"rate_limit_cooldown_seconds":30,"stream_timeout_seconds":600,"request_shaper_enabled":true,"beta_strategy":"allow_configured"},"payment":{"enabled":false,"providers":[],"subscription_plans_enabled":false},"email":{"smtp_configured":false,"templates":{}},"backup":{"enabled":false,"retention_days":30}}`))
	settingsReq.Header.Set("Content-Type", "application/json")
	settingsReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	settingsReq.AddCookie(sessionCookie)
	settingsRec := httptest.NewRecorder()
	handler.ServeHTTP(settingsRec, settingsReq)
	if settingsRec.Code != http.StatusOK {
		t.Fatalf("expected settings update 200, got %d body=%s", settingsRec.Code, settingsRec.Body.String())
	}

	announcementReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", strings.NewReader(`{"title":"Maintenance","content":"Window","status":"published","severity":"info","audience":"all"}`))
	announcementReq.Header.Set("Content-Type", "application/json")
	announcementReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	announcementReq.AddCookie(sessionCookie)
	announcementRec := httptest.NewRecorder()
	handler.ServeHTTP(announcementRec, announcementReq)
	if announcementRec.Code != http.StatusCreated {
		t.Fatalf("expected announcement create 201, got %d body=%s", announcementRec.Code, announcementRec.Body.String())
	}

	redeemReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/redeem-codes", strings.NewReader(`{"code":"WELCOME10","type":"balance","value":"10.00","currency":"USD"}`))
	redeemReq.Header.Set("Content-Type", "application/json")
	redeemReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	redeemReq.AddCookie(sessionCookie)
	redeemRec := httptest.NewRecorder()
	handler.ServeHTTP(redeemRec, redeemReq)
	if redeemRec.Code != http.StatusCreated {
		t.Fatalf("expected redeem create 201, got %d body=%s", redeemRec.Code, redeemRec.Body.String())
	}

	batchReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/redeem-codes/batch-generate", strings.NewReader(`{"prefix":"BATCH","count":2,"type":"balance","value":"1.00","currency":"USD"}`))
	batchReq.Header.Set("Content-Type", "application/json")
	batchReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	batchReq.AddCookie(sessionCookie)
	batchRec := httptest.NewRecorder()
	handler.ServeHTTP(batchRec, batchReq)
	if batchRec.Code != http.StatusCreated {
		t.Fatalf("expected redeem batch create 201, got %d body=%s", batchRec.Code, batchRec.Body.String())
	}

	promoReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promo-codes", strings.NewReader(`{"code":"PROMO5","discount_type":"amount","discount_value":"5.00","currency":"USD"}`))
	promoReq.Header.Set("Content-Type", "application/json")
	promoReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	promoReq.AddCookie(sessionCookie)
	promoRec := httptest.NewRecorder()
	handler.ServeHTTP(promoRec, promoReq)
	if promoRec.Code != http.StatusCreated {
		t.Fatalf("expected promo create 201, got %d body=%s", promoRec.Code, promoRec.Body.String())
	}

	riskReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/risk-control/config", strings.NewReader(`{"enabled":true,"mode":"monitor","max_failed_requests_per_minute":10,"max_cost_per_day":"100.00","cooldown_seconds":60,"blocked_countries":["ZZ"],"blocked_ips":[]}`))
	riskReq.Header.Set("Content-Type", "application/json")
	riskReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	riskReq.AddCookie(sessionCookie)
	riskRec := httptest.NewRecorder()
	handler.ServeHTTP(riskRec, riskReq)
	if riskRec.Code != http.StatusOK {
		t.Fatalf("expected risk config update 200, got %d body=%s", riskRec.Code, riskRec.Body.String())
	}

	readEndpoints := []string{
		"/api/v1/admin/dashboard/snapshot",
		"/api/v1/admin/ops/overview",
		"/api/v1/admin/ops/throughput-trend",
		"/api/v1/admin/ops/error-trend",
		"/api/v1/admin/ops/error-distribution",
		"/api/v1/admin/ops/latency-histogram",
		"/api/v1/admin/ops/concurrency",
		"/api/v1/admin/ops/system-logs",
		"/api/v1/admin/ops/alert-events",
		"/api/v1/admin/settings",
		"/api/v1/admin/announcements",
		"/api/v1/admin/redeem-codes",
		"/api/v1/admin/redeem-codes/stats",
		"/api/v1/admin/promo-codes",
		"/api/v1/admin/risk-control/config",
		"/api/v1/admin/risk-control/status",
		"/api/v1/admin/risk-control/logs",
	}
	for _, path := range readEndpoints {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"admin_settings.update", "announcement.create", "redeem_code.create", "redeem_code.batch_generate", "promo_code.create", "risk_control.update"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
}

func TestAdminOpsSLOAndAlertControlPlane(t *testing.T) {
	usageStore := usagememory.New()
	operationsStore := operationsmemory.NewWithUsageStore(usageStore)
	auditStore := auditmemory.New()
	handler := New(config.Load(), nil,
		WithUsageStore(usageStore),
		WithOperationsStore(operationsStore),
		WithAuditStore(auditStore),
	)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	ctx := t.Context()
	now := time.Now().UTC().Add(-time.Minute)
	clientError := "invalid_request"
	providerError := "rate_limit"
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_ops_good",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "ops-model",
		Success:        true,
		CreatedAt:      now,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_ops_bad",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "ops-model",
		Success:        false,
		ErrorClass:     &providerError,
		CreatedAt:      now,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_ops_client_bad",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "ops-model",
		Success:        false,
		ErrorClass:     &clientError,
		CreatedAt:      now,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ops/slo", strings.NewReader(`{"name":"Gateway availability","sli_type":"availability","objective":99,"window_days":28,"filter":{"source_endpoint":"/v1/chat/completions","model":"ops-model","error_owner_exclude":["client"]},"alert_policy":{"name":"multi_window_burn_rate","thresholds":[{"severity":"critical","short_window_seconds":300,"long_window_seconds":3600,"burn_rate":14,"min_request_count":2}]}}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected slo create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp apiopenapi.OpsSLODefinitionResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode slo create: %v", err)
	}
	if createResp.Data.Objective < 0.989 || createResp.Data.Objective > 0.991 {
		t.Fatalf("expected percent objective normalized to ratio, got %+v", createResp.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/slo", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected slo list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.OpsSLOListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode slo list: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].Evaluation.TotalRequests != 2 || listResp.Data[0].Evaluation.BadRequests != 1 {
		t.Fatalf("expected evaluated slo to count provider-owned failure only, got %+v", listResp.Data)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/ops/slo/"+string(createResp.Data.Id), strings.NewReader(`{"status":"disabled","objective":0.995}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected slo update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updateResp apiopenapi.OpsSLODefinitionResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode slo update: %v", err)
	}
	if updateResp.Data.Status != apiopenapi.OpsSLOStatusDisabled || updateResp.Data.Objective < 0.994 || updateResp.Data.Objective > 0.996 {
		t.Fatalf("unexpected slo update response: %+v", updateResp.Data)
	}

	sloID, err := strconv.Atoi(string(createResp.Data.Id))
	if err != nil {
		t.Fatalf("parse slo id: %v", err)
	}
	alert, err := operationsStore.CreateAlert(ctx, operationscontract.AlertEvent{
		SLOID:       &sloID,
		RuleID:      "slo.gateway.availability",
		Severity:    operationscontract.AlertSeverityCritical,
		Status:      operationscontract.AlertStatusFiring,
		Fingerprint: "slo:gateway:availability",
		Summary:     "Gateway availability burn rate high",
		Details:     map[string]any{"burn_rate": 50, "sample": "not-copied"},
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("seed alert: %v", err)
	}

	alertsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/alerts?status=firing&severity=critical", nil)
	alertsReq.AddCookie(sessionCookie)
	alertsRec := httptest.NewRecorder()
	handler.ServeHTTP(alertsRec, alertsReq)
	if alertsRec.Code != http.StatusOK {
		t.Fatalf("expected alert list 200, got %d body=%s", alertsRec.Code, alertsRec.Body.String())
	}
	var alertsResp apiopenapi.OpsAlertListResponse
	if err := json.NewDecoder(alertsRec.Body).Decode(&alertsResp); err != nil {
		t.Fatalf("decode alert list: %v", err)
	}
	if len(alertsResp.Data) != 1 || alertsResp.Data[0].Status != apiopenapi.Firing {
		t.Fatalf("expected firing critical alert, got %+v", alertsResp.Data)
	}

	ackReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ops/alerts/"+strconv.Itoa(alert.ID)+"/ack", nil)
	ackReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	ackReq.AddCookie(sessionCookie)
	ackRec := httptest.NewRecorder()
	handler.ServeHTTP(ackRec, ackReq)
	if ackRec.Code != http.StatusOK {
		t.Fatalf("expected alert ack 200, got %d body=%s", ackRec.Code, ackRec.Body.String())
	}
	var ackResp apiopenapi.OpsAlertResponse
	if err := json.NewDecoder(ackRec.Body).Decode(&ackResp); err != nil {
		t.Fatalf("decode alert ack: %v", err)
	}
	if ackResp.Data.Status != apiopenapi.Acknowledged || ackResp.Data.AcknowledgedBy == nil {
		t.Fatalf("expected acknowledged alert response, got %+v", ackResp.Data)
	}

	auditLogs, err := auditStore.List(ctx)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if !auditContractLogHasAction(auditLogs, "ops_slo.create") || !auditContractLogHasAction(auditLogs, "ops_slo.update") || !auditContractLogHasAction(auditLogs, "ops_alert.ack") {
		t.Fatalf("expected ops audit actions, got %+v", auditLogs)
	}
	for _, item := range auditLogs {
		if item.Action == "ops_alert.ack" {
			if _, ok := item.After["details"]; ok {
				t.Fatalf("alert ack audit must not expose alert details: %+v", item.After)
			}
		}
	}
}

func TestAdminSubscriptionPricingControlPlane(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"subscription-provider","display_name":"Subscription Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"subscription-model","display_name":"Subscription Model","status":"active"}`)

	planReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscription-plans", strings.NewReader(`{"name":"commercial-pro","description":"Commercial Pro","price":"19.99","currency":"usd","validity_days":30,"entitlements":{"allowed_models":["subscription-model"],"monthly_token_quota":1000,"scheduler_strategy":"cost_saver"},"for_sale":true,"sort_order":5,"status":"active"}`))
	planReq.Header.Set("Content-Type", "application/json")
	planReq.AddCookie(sessionCookie)
	planReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	planRec := httptest.NewRecorder()
	handler.ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusCreated {
		t.Fatalf("expected subscription plan create 201, got %d body=%s", planRec.Code, planRec.Body.String())
	}
	var planResp apiopenapi.SubscriptionPlanResponse
	if err := json.NewDecoder(planRec.Body).Decode(&planResp); err != nil {
		t.Fatalf("decode subscription plan: %v", err)
	}
	if planResp.Data.Price != "19.99000000" || planResp.Data.Currency != "USD" || planResp.Data.Status != apiopenapi.SubscriptionPlanStatusActive {
		t.Fatalf("unexpected subscription plan response: %+v", planResp.Data)
	}
	if planResp.Data.Entitlements["scheduler_strategy"] != "cost_saver" {
		t.Fatalf("expected entitlement snapshot in plan response, got %+v", planResp.Data.Entitlements)
	}

	userSubReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user-subscriptions", strings.NewReader(`{"user_id":"`+string(loginResp.Data.User.Id)+`","plan_id":"`+string(planResp.Data.Id)+`","source_type":"manual","source_id":"admin-seed"}`))
	userSubReq.Header.Set("Content-Type", "application/json")
	userSubReq.AddCookie(sessionCookie)
	userSubReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	userSubRec := httptest.NewRecorder()
	handler.ServeHTTP(userSubRec, userSubReq)
	if userSubRec.Code != http.StatusCreated {
		t.Fatalf("expected user subscription create 201, got %d body=%s", userSubRec.Code, userSubRec.Body.String())
	}
	var userSubResp apiopenapi.UserSubscriptionResponse
	if err := json.NewDecoder(userSubRec.Body).Decode(&userSubResp); err != nil {
		t.Fatalf("decode user subscription: %v", err)
	}
	if userSubResp.Data.UserId != loginResp.Data.User.Id || userSubResp.Data.PlanId != planResp.Data.Id || userSubResp.Data.Status != apiopenapi.Active {
		t.Fatalf("unexpected user subscription response: %+v", userSubResp.Data)
	}
	if userSubResp.Data.EntitlementsSnapshot["scheduler_strategy"] != "cost_saver" {
		t.Fatalf("expected entitlement snapshot copied to subscription, got %+v", userSubResp.Data.EntitlementsSnapshot)
	}

	pricingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules", strings.NewReader(`{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"1.25","output_price_per_million_tokens":"2.50","cache_read_price_per_million_tokens":"0.10","cache_write_price_per_million_tokens":"0.20","currency":"usd"}`))
	pricingReq.Header.Set("Content-Type", "application/json")
	pricingReq.AddCookie(sessionCookie)
	pricingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	pricingRec := httptest.NewRecorder()
	handler.ServeHTTP(pricingRec, pricingReq)
	if pricingRec.Code != http.StatusCreated {
		t.Fatalf("expected pricing rule create 201, got %d body=%s", pricingRec.Code, pricingRec.Body.String())
	}
	var pricingResp apiopenapi.PricingRuleResponse
	if err := json.NewDecoder(pricingRec.Body).Decode(&pricingResp); err != nil {
		t.Fatalf("decode pricing rule: %v", err)
	}
	if pricingResp.Data.InputPricePerMillionTokens != "1.25000000" || pricingResp.Data.OutputPricePerMillionTokens != "2.50000000" || pricingResp.Data.Currency != "USD" {
		t.Fatalf("expected normalized decimal pricing rule, got %+v", pricingResp.Data)
	}

	bulkDryRunReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules:bulk", strings.NewReader(`{"dry_run":true,"items":[{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"3","output_price_per_million_tokens":"4","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"usd"},{"model_id":"bad","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"3","output_price_per_million_tokens":"4","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"usd"}]}`))
	bulkDryRunReq.Header.Set("Content-Type", "application/json")
	bulkDryRunReq.AddCookie(sessionCookie)
	bulkDryRunReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	bulkDryRunRec := httptest.NewRecorder()
	handler.ServeHTTP(bulkDryRunRec, bulkDryRunReq)
	if bulkDryRunRec.Code != http.StatusOK {
		t.Fatalf("expected pricing rule bulk dry-run 200, got %d body=%s", bulkDryRunRec.Code, bulkDryRunRec.Body.String())
	}
	var bulkDryRunResp apiopenapi.BulkPricingRuleImportResponse
	if err := json.NewDecoder(bulkDryRunRec.Body).Decode(&bulkDryRunResp); err != nil {
		t.Fatalf("decode pricing rule bulk dry-run: %v", err)
	}
	if !bulkDryRunResp.Data.DryRun || bulkDryRunResp.Data.Requested != 2 || bulkDryRunResp.Data.Validated != 1 || bulkDryRunResp.Data.Created != 0 || len(bulkDryRunResp.Data.Rules) != 0 || len(bulkDryRunResp.Data.Errors) != 1 {
		t.Fatalf("unexpected pricing rule bulk dry-run response: %+v", bulkDryRunResp.Data)
	}

	bulkReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules:bulk", strings.NewReader(`[{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"3","output_price_per_million_tokens":"4","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"usd"}]`))
	bulkReq.Header.Set("Content-Type", "application/json")
	bulkReq.AddCookie(sessionCookie)
	bulkReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	bulkRec := httptest.NewRecorder()
	handler.ServeHTTP(bulkRec, bulkReq)
	if bulkRec.Code != http.StatusOK {
		t.Fatalf("expected pricing rule bulk import 200, got %d body=%s", bulkRec.Code, bulkRec.Body.String())
	}
	var bulkResp apiopenapi.BulkPricingRuleImportResponse
	if err := json.NewDecoder(bulkRec.Body).Decode(&bulkResp); err != nil {
		t.Fatalf("decode pricing rule bulk import: %v", err)
	}
	if bulkResp.Data.DryRun || bulkResp.Data.Requested != 1 || bulkResp.Data.Validated != 1 || bulkResp.Data.Created != 1 || len(bulkResp.Data.Errors) != 0 || len(bulkResp.Data.Rules) != 1 {
		t.Fatalf("unexpected pricing rule bulk import response: %+v", bulkResp.Data)
	}
	if bulkResp.Data.Rules[0].InputPricePerMillionTokens != "3.00000000" || bulkResp.Data.Rules[0].Currency != "USD" {
		t.Fatalf("expected normalized bulk pricing rule, got %+v", bulkResp.Data.Rules[0])
	}

	plansReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscription-plans", nil)
	plansReq.AddCookie(sessionCookie)
	plansRec := httptest.NewRecorder()
	handler.ServeHTTP(plansRec, plansReq)
	if plansRec.Code != http.StatusOK {
		t.Fatalf("expected subscription plan list 200, got %d body=%s", plansRec.Code, plansRec.Body.String())
	}
	var plansResp apiopenapi.SubscriptionPlanListResponse
	if err := json.NewDecoder(plansRec.Body).Decode(&plansResp); err != nil {
		t.Fatalf("decode subscription plan list: %v", err)
	}
	if len(plansResp.Data) != 1 || plansResp.Data[0].Id != planResp.Data.Id {
		t.Fatalf("unexpected subscription plan list: %+v", plansResp.Data)
	}

	adminSubsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/user-subscriptions?user_id="+string(loginResp.Data.User.Id), nil)
	adminSubsReq.AddCookie(sessionCookie)
	adminSubsRec := httptest.NewRecorder()
	handler.ServeHTTP(adminSubsRec, adminSubsReq)
	if adminSubsRec.Code != http.StatusOK {
		t.Fatalf("expected admin user subscription list 200, got %d body=%s", adminSubsRec.Code, adminSubsRec.Body.String())
	}
	var adminSubsResp apiopenapi.UserSubscriptionListResponse
	if err := json.NewDecoder(adminSubsRec.Body).Decode(&adminSubsResp); err != nil {
		t.Fatalf("decode admin user subscription list: %v", err)
	}
	if len(adminSubsResp.Data) != 1 || adminSubsResp.Data[0].Id != userSubResp.Data.Id {
		t.Fatalf("unexpected admin user subscription list: %+v", adminSubsResp.Data)
	}

	meSubsReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/subscriptions", nil)
	meSubsReq.AddCookie(sessionCookie)
	meSubsRec := httptest.NewRecorder()
	handler.ServeHTTP(meSubsRec, meSubsReq)
	if meSubsRec.Code != http.StatusOK {
		t.Fatalf("expected current user subscription list 200, got %d body=%s", meSubsRec.Code, meSubsRec.Body.String())
	}
	var meSubsResp apiopenapi.UserSubscriptionListResponse
	if err := json.NewDecoder(meSubsRec.Body).Decode(&meSubsResp); err != nil {
		t.Fatalf("decode current user subscription list: %v", err)
	}
	if len(meSubsResp.Data) != 1 || meSubsResp.Data[0].Id != userSubResp.Data.Id {
		t.Fatalf("unexpected current user subscription list: %+v", meSubsResp.Data)
	}

	rulesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-rules", nil)
	rulesReq.AddCookie(sessionCookie)
	rulesRec := httptest.NewRecorder()
	handler.ServeHTTP(rulesRec, rulesReq)
	if rulesRec.Code != http.StatusOK {
		t.Fatalf("expected pricing rule list 200, got %d body=%s", rulesRec.Code, rulesRec.Body.String())
	}
	var rulesResp apiopenapi.PricingRuleListResponse
	if err := json.NewDecoder(rulesRec.Body).Decode(&rulesResp); err != nil {
		t.Fatalf("decode pricing rule list: %v", err)
	}
	if len(rulesResp.Data) != 2 || !pricingRuleListHasID(rulesResp.Data, pricingResp.Data.Id) || !pricingRuleListHasID(rulesResp.Data, bulkResp.Data.Rules[0].Id) {
		t.Fatalf("unexpected pricing rule list: %+v", rulesResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit log list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"subscription_plan.create", "user_subscription.create", "pricing_rule.create", "pricing_rule.bulk_import"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
}

func TestAdminPaymentProviderUpdateAndTest(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createResp := mustCreatePaymentProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider":"easypay","name":"easypay-primary","config":{"gateway_url":"https://pay.example/submit","merchant_id":"merchant-1","webhook_secret":"payment-provider-secret","notify_url":"https://api.example/api/v1/webhooks/payments/easypay","return_url":"https://app.example/payments/return"},"supported_methods":["alipay"],"metadata":{"display_name":"EasyPay"}}`)
	if createResp.Data.Provider != "easypay" || createResp.Data.Name != "easypay-primary" || len(createResp.Data.SupportedMethods) != 1 {
		t.Fatalf("unexpected payment provider create response: %+v", createResp.Data)
	}

	testResp := mustTestPaymentProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, createResp.Data.Id)
	if !testResp.Data.Ok || testResp.Data.ProviderId == nil || *testResp.Data.ProviderId != createResp.Data.Id {
		t.Fatalf("expected active payment provider test to pass, got %+v", testResp.Data)
	}
	if testResp.Data.Checks == nil || (*testResp.Data.Checks)["config_decrypts"] != true {
		t.Fatalf("expected payment provider test to report decryptable config, got %+v", testResp.Data.Checks)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/payments/providers/"+string(createResp.Data.Id), strings.NewReader(`{"name":"easypay-renamed","status":"disabled","supported_methods":["wechat","alipay","wechat"],"metadata":{"display_name":"EasyPay Renamed"},"sort_order":7}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(sessionCookie)
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected payment provider update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	if strings.Contains(updateRec.Body.String(), "payment-provider-secret") {
		t.Fatalf("payment provider response leaked config secret: %s", updateRec.Body.String())
	}
	var updateResp apiopenapi.PaymentProviderInstanceResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode payment provider update: %v", err)
	}
	if updateResp.Data.Name != "easypay-renamed" || updateResp.Data.Status != apiopenapi.PaymentProviderStatusDisabled || strings.Join(updateResp.Data.SupportedMethods, ",") != "alipay,wechat" || updateResp.Data.SortOrder != 7 {
		t.Fatalf("unexpected payment provider update response: %+v", updateResp.Data)
	}

	testResp = mustTestPaymentProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, createResp.Data.Id)
	if testResp.Data.Ok || testResp.Data.Status != "failed" || testResp.Data.Message == nil || *testResp.Data.Message != "payment provider instance is not active" {
		t.Fatalf("expected disabled payment provider test to fail active check, got %+v", testResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit log list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	if strings.Contains(auditRec.Body.String(), "payment-provider-secret") {
		t.Fatalf("payment provider audit leaked config secret: %s", auditRec.Body.String())
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"payment_provider.create", "payment_provider.update", "payment_provider.test"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
}

func TestAlipayPaymentWebhookRespondsWithChannelAck(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keys := newAlipayWebhookHTTPTestKeys(t)
	providerBody := mustMarshalJSON(t, map[string]any{
		"provider": "alipay",
		"name":     "alipay-http-primary",
		"config": map[string]any{
			"app_id":            "app_test_123",
			"private_key":       keys.merchantPrivateKey,
			"alipay_public_key": keys.alipayPublicKey,
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
			"gateway_url":       "https://openapi.alipay.test/gateway.do",
		},
		"supported_methods": []string{"alipay_http_smoke"},
		"limits":            map[string]any{"currency": "CNY"},
	})
	mustCreatePaymentProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, providerBody)

	orderReq := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", strings.NewReader(`{"method":"alipay_http_smoke","amount":"1.00","currency":"CNY","product_type":"balance_credit"}`))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.AddCookie(sessionCookie)
	orderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	orderRec := httptest.NewRecorder()
	handler.ServeHTTP(orderRec, orderReq)
	if orderRec.Code != http.StatusCreated {
		t.Fatalf("expected alipay payment order create 201, got %d body=%s", orderRec.Code, orderRec.Body.String())
	}
	var orderResp apiopenapi.PaymentOrderResponse
	if err := json.NewDecoder(orderRec.Body).Decode(&orderResp); err != nil {
		t.Fatalf("decode alipay payment order response: %v", err)
	}

	payload := signedAlipayHTTPNotification(t, keys, map[string]string{
		"notify_id":    "notify_http_paid_1",
		"notify_type":  alipaysdk.NotifyTypeTradeStatusSync,
		"out_trade_no": orderResp.Data.OrderNo,
		"trade_no":     "2026052622001400000009",
		"trade_status": string(alipaysdk.TradeStatusSuccess),
		"total_amount": "1.00",
		"app_id":       "app_test_123",
		"charset":      "utf-8",
		"version":      "1.0",
	})
	webhookReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payments/alipay", strings.NewReader(mustMarshalJSON(t, payload)))
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected alipay payment webhook 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if strings.TrimSpace(webhookRec.Body.String()) != "success" {
		t.Fatalf("expected alipay webhook channel ack success, got %q", webhookRec.Body.String())
	}
	if got := webhookRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("expected alipay webhook text/plain content type, got %q", got)
	}
}

func TestWechatPaymentWebhookAcceptsSignedNotification(t *testing.T) {
	paymentStore := paymentmemory.New()
	handler := New(config.Load(), nil, WithPaymentStore(paymentStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keys := newWechatWebhookHTTPTestKeys(t)
	providerBody := mustMarshalJSON(t, map[string]any{
		"provider": "wechat",
		"name":     "wechat-http-primary",
		"config": map[string]any{
			"app_id":                  "wx_app_123",
			"mch_id":                  "mch_123",
			"api_v3_key":              keys.apiV3Key,
			"serial_no":               "merchant_serial_123",
			"private_key":             keys.merchantPrivateKey,
			"notify_url":              "https://api.example/api/v1/webhooks/payments/wechat",
			"wechatpay_public_key":    keys.platformPublicKey,
			"wechatpay_public_key_id": keys.platformPublicKeyID,
		},
		"supported_methods": []string{"wechat_http_smoke"},
		"limits":            map[string]any{"currency": "CNY"},
	})
	providerResp := mustCreatePaymentProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, providerBody)
	providerID, err := strconv.Atoi(string(providerResp.Data.Id))
	if err != nil {
		t.Fatalf("parse wechat provider id: %v", err)
	}
	userID, err := strconv.Atoi(string(loginResp.Data.User.Id))
	if err != nil {
		t.Fatalf("parse login user id: %v", err)
	}
	order, err := paymentStore.CreateOrder(t.Context(), paymentcontract.CreateStoredOrder{
		UserID:             userID,
		OrderNo:            "pay_wechat_http_123",
		ProviderInstanceID: providerID,
		Amount:             "1.00000000",
		Currency:           "CNY",
		Status:             paymentcontract.OrderStatusPending,
		ProductType:        paymentcontract.ProductTypeBalanceCredit,
		ProviderSnapshot: map[string]any{
			"provider":             "wechat",
			"provider_instance_id": providerID,
			"name":                 "wechat-http-primary",
			"method":               "wechat_http_smoke",
		},
	})
	if err != nil {
		t.Fatalf("create stored wechat payment order: %v", err)
	}

	rawBody, headers := signedWechatHTTPNotification(t, keys, map[string]any{
		"appid":          "wx_app_123",
		"mchid":          "mch_123",
		"out_trade_no":   order.OrderNo,
		"transaction_id": "4200000000202605261234567890",
		"trade_state":    "SUCCESS",
		"trade_type":     "NATIVE",
		"amount": map[string]any{
			"total":    100,
			"currency": "CNY",
		},
	})
	webhookReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payments/wechat", strings.NewReader(rawBody))
	webhookReq.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		webhookReq.Header.Set(key, value)
	}
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected wechat payment webhook 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	var webhookResp apiopenapi.PaymentWebhookResponse
	if err := json.NewDecoder(webhookRec.Body).Decode(&webhookResp); err != nil {
		t.Fatalf("decode wechat payment webhook response: %v", err)
	}
	if !webhookResp.Data.Handled || webhookResp.Data.Order.Status != apiopenapi.PaymentOrderStatusFulfilled {
		t.Fatalf("expected fulfilled wechat payment webhook response, got %+v", webhookResp.Data)
	}
	if webhookResp.Data.Order.ProviderTransactionId == nil || *webhookResp.Data.Order.ProviderTransactionId != "4200000000202605261234567890" {
		t.Fatalf("expected wechat transaction id to be preserved, got %+v", webhookResp.Data.Order.ProviderTransactionId)
	}

	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payments/wechat", strings.NewReader(rawBody))
	duplicateReq.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		duplicateReq.Header.Set(key, value)
	}
	duplicateRec := httptest.NewRecorder()
	handler.ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusOK {
		t.Fatalf("expected duplicate wechat payment webhook 200, got %d body=%s", duplicateRec.Code, duplicateRec.Body.String())
	}
	var duplicateResp apiopenapi.PaymentWebhookResponse
	if err := json.NewDecoder(duplicateRec.Body).Decode(&duplicateResp); err != nil {
		t.Fatalf("decode duplicate wechat payment webhook response: %v", err)
	}
	if duplicateResp.Data.Handled {
		t.Fatalf("expected duplicate wechat webhook to be idempotent, got %+v", duplicateResp.Data)
	}
}

func TestBulkImportAdminPricingRulesAcceptsCSV(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"pricing-csv-provider","display_name":"Pricing CSV Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelIDs := make([]apiopenapi.Id, 0, 50)
	for idx := 0; idx < 50; idx++ {
		modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, fmt.Sprintf(`{"canonical_name":"pricing-csv-model-%02d","display_name":"Pricing CSV Model %02d","status":"active"}`, idx, idx))
		modelIDs = append(modelIDs, modelResp.Data.Id)
	}

	var csvBody strings.Builder
	csvBody.WriteString("model_id,provider_id,input_price_per_million_tokens,output_price_per_million_tokens,cache_read_price_per_million_tokens,cache_write_price_per_million_tokens,currency,effective_from,effective_to\n")
	for idx, modelID := range modelIDs {
		csvBody.WriteString(fmt.Sprintf("%s,%s,1.%02d,2.%02d,0,0,usd,,\n", modelID, providerResp.Data.Id, idx, idx))
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules:bulk", strings.NewReader(csvBody.String()))
	req.Header.Set("Content-Type", "text/csv")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected pricing rule csv import 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.BulkPricingRuleImportResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode pricing rule csv import: %v", err)
	}
	if resp.Data.Requested != 50 || resp.Data.Validated != 50 || resp.Data.Created != 50 || len(resp.Data.Errors) != 0 || len(resp.Data.Rules) != 50 {
		t.Fatalf("unexpected pricing rule csv import response: %+v", resp.Data)
	}

	badQueryReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules:bulk?dry_run=maybe", strings.NewReader("[]"))
	badQueryReq.Header.Set("Content-Type", "application/json")
	badQueryReq.AddCookie(sessionCookie)
	badQueryReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	badQueryRec := httptest.NewRecorder()
	handler.ServeHTTP(badQueryRec, badQueryReq)
	if badQueryRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid dry_run query to return 400, got %d body=%s", badQueryRec.Code, badQueryRec.Body.String())
	}
}

func TestAdminAccountInspectAndExportStaySecretSafe(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-ops-provider","display_name":"Account Ops Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"account-ops-account","runtime_class":"api_key","credential":{"api_key":"account-ops-secret"},"metadata":{"base_url":"https://upstream.example/v1","access_token":"metadata-access-token","cookie":"metadata-cookie","nested":{"refresh_token":"nested-refresh-token","safe":"kept"},"labels":[{"api_key":"nested-api-key","region":"us"}]},"status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-ops-group","description":"ops","provider_scope":{"provider_id":"`+string(providerResp.Data.Id)+`"},"model_scope":{"family":"claude"},"strategy_hint":"balanced","status":"active"}`)

	addMemberReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-groups/"+string(groupResp.Data.Id)+"/accounts/"+string(accountResp.Data.Id), nil)
	addMemberReq.AddCookie(sessionCookie)
	addMemberReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	addMemberRec := httptest.NewRecorder()
	handler.ServeHTTP(addMemberRec, addMemberReq)
	if addMemberRec.Code != http.StatusOK {
		t.Fatalf("expected account group member add 200, got %d body=%s", addMemberRec.Code, addMemberRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account inspect: %v", err)
	}
	if getResp.Data.Id != accountResp.Data.Id || len(getResp.Data.GroupIds) != 1 || getResp.Data.GroupIds[0] != groupResp.Data.Id {
		t.Fatalf("expected inspected account with group id, got %+v", getResp.Data)
	}
	if strings.Contains(getRec.Body.String(), "account-ops-secret") {
		t.Fatalf("account inspect leaked credential material: %s", getRec.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/export", nil)
	exportReq.AddCookie(sessionCookie)
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected account export 200, got %d body=%s", exportRec.Code, exportRec.Body.String())
	}
	if body := exportRec.Body.String(); strings.Contains(body, "account-ops-secret") ||
		strings.Contains(body, "metadata-access-token") ||
		strings.Contains(body, "metadata-cookie") ||
		strings.Contains(body, "nested-refresh-token") ||
		strings.Contains(body, "nested-api-key") {
		t.Fatalf("account export leaked sensitive material: %s", body)
	}
	var exportResp apiopenapi.ProviderAccountExportResponse
	if err := json.NewDecoder(exportRec.Body).Decode(&exportResp); err != nil {
		t.Fatalf("decode account export: %v", err)
	}
	item := findProviderAccountExportByName(exportResp.Data, "account-ops-account")
	if item == nil {
		t.Fatalf("exported account not found in %+v", exportResp.Data)
	}
	if item.CredentialExported || item.GroupIds == nil || len(*item.GroupIds) != 1 || (*item.GroupIds)[0] != groupResp.Data.Id {
		t.Fatalf("expected non-secret export with group id, got %+v", item)
	}
	if item.Metadata == nil || (*item.Metadata)["base_url"] != "https://upstream.example/v1" || (*item.Metadata)["nested"].(map[string]any)["safe"] != "kept" {
		t.Fatalf("expected safe metadata retained, got %+v", item.Metadata)
	}
}

func TestAdminAccountModelDiscoveryPreviewOpenAICompatible(t *testing.T) {
	var upstreamSeenAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		upstreamSeenAuth = r.Header.Get("Authorization")
		if r.Header.Get("X-Request-ID") != "" || r.Header.Get("Cookie") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi/client header leakage: %+v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"z-model"},{"id":"a-model"},{"id":"a-model"}]}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-openai-provider","display_name":"Discovery OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovery-openai-account","runtime_class":"api_key","credential":{"api_key":"discovery-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"limit":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamSeenAuth != "Bearer discovery-secret" {
		t.Fatalf("expected upstream bearer auth, got %q", upstreamSeenAuth)
	}
	if strings.Contains(rec.Body.String(), "discovery-secret") {
		t.Fatalf("discovery response leaked credential: %s", rec.Body.String())
	}
	var resp apiopenapi.AccountModelDiscoveryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if resp.Data.Source != apiopenapi.AccountModelDiscoverySourceOpenaiCompatible || resp.Data.Persisted || len(resp.Data.ModelIds) != 2 || resp.Data.ModelIds[0] != "a-model" || resp.Data.ModelIds[1] != "z-model" {
		t.Fatalf("unexpected discovery response: %+v", resp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata != nil && jsonObjectContainsString(*getResp.Data.Metadata, "a-model") {
		t.Fatalf("preview discovery should not persist supported models: %+v", getResp.Data.Metadata)
	}
}

func TestAdminAccountModelDiscoveryPersistsGeminiCompatible(t *testing.T) {
	var upstreamSeenKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		upstreamSeenKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-1.5-pro"},{"name":"models/gemini-1.5-flash"}]}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-gemini-provider","display_name":"Discovery Gemini","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovery-gemini-account","runtime_class":"api_key","credential":{"api_key":"gemini-discovery-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamSeenKey != "gemini-discovery-secret" {
		t.Fatalf("expected Gemini API key query, got %q", upstreamSeenKey)
	}
	if strings.Contains(rec.Body.String(), "gemini-discovery-secret") {
		t.Fatalf("discovery response leaked credential: %s", rec.Body.String())
	}
	var resp apiopenapi.AccountModelDiscoveryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if resp.Data.Source != apiopenapi.AccountModelDiscoverySourceGeminiCompatible || !resp.Data.Persisted || !stringSliceContains(resp.Data.ModelIds, "gemini-1.5-pro") || !stringSliceContains(resp.Data.ModelIds, "gemini-1.5-flash") {
		t.Fatalf("unexpected Gemini discovery response: %+v", resp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata == nil || !jsonObjectContainsString(*getResp.Data.Metadata, "gemini-1.5-pro") || (*getResp.Data.Metadata)["model_discovery_source"] != "gemini-compatible" || (*getResp.Data.Metadata)["model_discovery_last_seen_at"] == "" {
		t.Fatalf("expected persisted discovery metadata, got %+v", getResp.Data.Metadata)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	if strings.Contains(auditRec.Body.String(), "gemini-discovery-secret") {
		t.Fatalf("audit log leaked discovery credential: %s", auditRec.Body.String())
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	discoveryAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.discover_models")
	if discoveryAudit.After["model_count"] != float64(2) && discoveryAudit.After["model_count"] != 2 {
		t.Fatalf("expected model_count audit evidence, got %+v", discoveryAudit.After)
	}
}

func TestAdminAccountModelDiscoveryPreviewAntigravityReverseProxy(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Method        string
		Authorization string
		Cookie        string
		RequestID     string
		UserAgent     string
		Project       string
	}
	var call upstreamCall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		var payload struct {
			Project string `json:"project"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode antigravity discovery body: %v", err)
		}
		call = upstreamCall{
			Path:          r.URL.Path,
			Method:        r.Method,
			Authorization: r.Header.Get("Authorization"),
			Cookie:        r.Header.Get("Cookie"),
			RequestID:     r.Header.Get("X-Request-ID"),
			UserAgent:     r.Header.Get("User-Agent"),
			Project:       payload.Project,
		}
		if r.Header.Get("X-SRapi-Test") != "" || r.Header.Get("X-Gateway-Test") != "" {
			t.Fatalf("unexpected SRapi/gateway header leakage: %+v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"z-antigravity":{"displayName":"Z"},"a-antigravity":{"displayName":"A"},"chat_20706":{"displayName":"internal"},"gemini-2.5-pro":{"displayName":"internal"}}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-discovery-provider","display_name":"Antigravity Discovery","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-discovery-account","runtime_class":"desktop_client_token","upstream_client":"antigravity_desktop","credential":{"access_token":"antigravity-discovery-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"project-1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"limit":10}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer caller-token")
	req.Header.Set("Cookie", "caller_cookie=1")
	req.Header.Set("X-Request-ID", "caller-request")
	req.Header.Set("X-SRapi-Test", "leak")
	req.Header.Set("X-Gateway-Test", "leak")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if call.Method != http.MethodPost || call.Authorization != "Bearer antigravity-discovery-token" || call.Cookie != "" || call.RequestID != "" || call.UserAgent != "Antigravity/1.0" || call.Project != "project-1" {
		t.Fatalf("unexpected antigravity discovery upstream call: %+v", call)
	}
	if strings.Contains(rec.Body.String(), "antigravity-discovery-token") || strings.Contains(rec.Body.String(), "caller-token") {
		t.Fatalf("discovery response leaked credential: %s", rec.Body.String())
	}
	var resp apiopenapi.AccountModelDiscoveryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if resp.Data.Source != apiopenapi.AccountModelDiscoverySourceReverseProxyAntigravity || resp.Data.Persisted || len(resp.Data.ModelIds) != 2 || resp.Data.ModelIds[0] != "a-antigravity" || resp.Data.ModelIds[1] != "z-antigravity" {
		t.Fatalf("unexpected antigravity discovery response: %+v", resp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata != nil && jsonObjectContainsString(*getResp.Data.Metadata, "a-antigravity") {
		t.Fatalf("preview discovery should not persist supported models: %+v", getResp.Data.Metadata)
	}
}

func TestAdminAccountModelDiscoveryPersistsAntigravityReverseProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer antigravity-persist-token" {
			t.Fatalf("expected selected account token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"gemini-3-pro-preview":{"maxTokens":1000000},"claude-sonnet-4-6":{"maxTokens":200000}}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-persist-provider","display_name":"Antigravity Persist","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-persist-account","runtime_class":"ide_plugin_token","credential":{"access_token":"antigravity-persist-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"persist-project"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AccountModelDiscoveryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if resp.Data.Source != apiopenapi.AccountModelDiscoverySourceReverseProxyAntigravity || !resp.Data.Persisted || !stringSliceContains(resp.Data.ModelIds, "gemini-3-pro-preview") || !stringSliceContains(resp.Data.ModelIds, "claude-sonnet-4-6") {
		t.Fatalf("unexpected antigravity persisted discovery response: %+v", resp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata == nil ||
		!jsonObjectContainsString(*getResp.Data.Metadata, "gemini-3-pro-preview") ||
		(*getResp.Data.Metadata)["model_discovery_source"] != "reverse-proxy-antigravity" ||
		(*getResp.Data.Metadata)["model_discovery_endpoint"] != upstream.URL+"/v1internal:fetchAvailableModels" ||
		(*getResp.Data.Metadata)["model_discovery_last_seen_at"] == "" {
		t.Fatalf("expected persisted antigravity discovery metadata, got %+v", getResp.Data.Metadata)
	}
}

func TestAdminAccountModelDiscoveryBootstrapsAntigravityProjectFromLoadCodeAssist(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		UserAgent     string
		Project       string
		IDEType       string
	}
	var calls []upstreamCall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode antigravity bootstrap body: %v", err)
		}
		call := upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
			Project:       strings.TrimSpace(fmt.Sprint(payload["project"])),
		}
		if metadata, ok := payload["metadata"].(map[string]any); ok {
			call.IDEType = strings.TrimSpace(fmt.Sprint(metadata["ideType"]))
		}
		calls = append(calls, call)
		switch r.URL.Path {
		case "/v1internal:loadCodeAssist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":{"id":"bootstrap-project"},"currentTier":{"id":"g1-pro-tier"}}`))
		case "/v1internal:fetchAvailableModels":
			if call.Project != "bootstrap-project" {
				t.Fatalf("expected bootstrapped project, got call %+v payload=%+v", call, payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":{"antigravity-bootstrap-model":{"displayName":"Bootstrap"}}}`))
		default:
			t.Fatalf("unexpected antigravity bootstrap path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-bootstrap-load-provider","display_name":"Antigravity Bootstrap Load","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-bootstrap-load-account","runtime_class":"desktop_client_token","upstream_client":"antigravity_desktop","credential":{"access_token":"antigravity-bootstrap-token"},"metadata":{"base_url":"`+upstream.URL+`","user_agent":"antigravity/1.23.2 windows/amd64"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"limit":10}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "caller-request")
	req.Header.Set("X-SRapi-Test", "leak")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(calls) != 2 {
		t.Fatalf("expected loadCodeAssist and discovery calls, got %+v", calls)
	}
	if calls[0].Path != "/v1internal:loadCodeAssist" || calls[0].Authorization != "Bearer antigravity-bootstrap-token" || calls[0].UserAgent != "antigravity/1.23.2 windows/amd64" || calls[0].IDEType != "ANTIGRAVITY" {
		t.Fatalf("unexpected loadCodeAssist call: %+v", calls[0])
	}
	if calls[1].Path != "/v1internal:fetchAvailableModels" || calls[1].Project != "bootstrap-project" || calls[1].Authorization != "Bearer antigravity-bootstrap-token" {
		t.Fatalf("unexpected discovery call: %+v", calls[1])
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata != nil && (*getResp.Data.Metadata)["project_id"] == "bootstrap-project" {
		t.Fatalf("preview discovery should not persist bootstrapped project: %+v", getResp.Data.Metadata)
	}
}

func TestAdminAccountModelDiscoveryPersistsAntigravityOnboardedProject(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		GoogAPIClient string
		Project       string
		TierID        string
	}
	var calls []upstreamCall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode antigravity onboard body: %v", err)
		}
		call := upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			GoogAPIClient: r.Header.Get("X-Goog-Api-Client"),
			Project:       strings.TrimSpace(fmt.Sprint(payload["project"])),
			TierID:        strings.TrimSpace(fmt.Sprint(payload["tier_id"])),
		}
		calls = append(calls, call)
		switch r.URL.Path {
		case "/v1internal:loadCodeAssist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"allowedTiers":[{"id":"free-tier","isDefault":false},{"id":"g1-pro-tier","isDefault":true}]}`))
		case "/v1internal:onboardUser":
			if call.TierID != "g1-pro-tier" || call.GoogAPIClient == "" {
				t.Fatalf("unexpected onboardUser call: %+v", call)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"done":true,"response":{"cloudaicompanionProject":{"id":"onboarded-project"}}}`))
		case "/v1internal:fetchAvailableModels":
			if call.Project != "onboarded-project" {
				t.Fatalf("expected onboarded project, got call %+v payload=%+v", call, payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":{"antigravity-onboard-model":{"displayName":"Onboard"}}}`))
		default:
			t.Fatalf("unexpected antigravity onboard path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-bootstrap-onboard-provider","display_name":"Antigravity Bootstrap Onboard","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-bootstrap-onboard-account","runtime_class":"ide_plugin_token","upstream_client":"antigravity_desktop","credential":{"access_token":"antigravity-onboard-token"},"metadata":{"base_url":"`+upstream.URL+`"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(calls) != 3 {
		t.Fatalf("expected loadCodeAssist, onboardUser, and discovery calls, got %+v", calls)
	}
	for _, call := range calls {
		if call.Authorization != "Bearer antigravity-onboard-token" {
			t.Fatalf("expected selected account token for %+v", call)
		}
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata == nil ||
		(*getResp.Data.Metadata)["project_id"] != "onboarded-project" ||
		(*getResp.Data.Metadata)["antigravity_project_id"] != "onboarded-project" ||
		(*getResp.Data.Metadata)["cloudaicompanion_project"] != "onboarded-project" ||
		(*getResp.Data.Metadata)["antigravity_project_bootstrapped_at"] == "" ||
		(*getResp.Data.Metadata)["antigravity_project_bootstrap_endpoint"] != upstream.URL+"/v1internal:onboardUser" ||
		!jsonObjectContainsString(*getResp.Data.Metadata, "antigravity-onboard-model") {
		t.Fatalf("expected persisted antigravity project and discovery metadata, got %+v", getResp.Data.Metadata)
	}
}

func TestAdminAccountModelDiscoveryRejectsAntigravityAPIKeyRuntime(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-api-key-discovery","display_name":"Antigravity API Key Discovery","adapter_type":"reverse-proxy-antigravity","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-api-key-account","runtime_class":"api_key","credential":{"api_key":"wrong-kind"},"metadata":{"base_url":"`+upstream.URL+`"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported discovery 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("antigravity api-key runtime should not call upstream")
	}
}

func TestAdminAccountModelDiscoveryRejectsUnsupportedRuntime(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-reverse-provider","display_name":"Discovery Reverse","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovery-reverse-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"oauth-access","refresh_token":"oauth-refresh"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported discovery 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("unsupported runtime should not call upstream")
	}
}

func TestGatewayUsesDiscoveredSupportedModelsForCandidateFiltering(t *testing.T) {
	var (
		mu        sync.Mutex
		modelAuth string
		chatAuth  string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/models":
			modelAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"discovered-upstream"}]}`))
		case "/v1/chat/completions":
			chatAuth = r.Header.Get("Authorization")
			if chatAuth != "Bearer discovered-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"message":"wrong account"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"routed through discovered account"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-routing-provider","display_name":"Discovery Routing Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"discovery-routing-model","display_name":"Discovery Routing Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"discovered-upstream","status":"active"}`)
	discoveredAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovered-account","runtime_class":"api_key","credential":{"api_key":"discovered-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.1},"status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"blocked-account","runtime_class":"api_key","credential":{"api_key":"blocked-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.99,"supported_models":["other-upstream"]},"status":"active"}`)

	discoverReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(discoveredAccount.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	discoverReq.Header.Set("Content-Type", "application/json")
	discoverReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	discoverReq.AddCookie(sessionCookie)
	discoverRec := httptest.NewRecorder()
	handler.ServeHTTP(discoverRec, discoverReq)
	if discoverRec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", discoverRec.Code, discoverRec.Body.String())
	}
	if modelAuth != "Bearer discovered-secret" {
		t.Fatalf("expected discovery upstream auth, got %q", modelAuth)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"discovery-routing-model","messages":[{"role":"user","content":"route with discovered catalog"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	if chatAuth != "Bearer discovered-secret" {
		t.Fatalf("expected discovered account to be selected, got auth %q", chatAuth)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=discovery-routing-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SelectedAccountId == nil || *decisionsResp.Data[0].SelectedAccountId != string(discoveredAccount.Data.Id) {
		t.Fatalf("expected discovered account to win scheduling, got %+v", decisionsResp.Data)
	}
}

type stubDependencyPinger struct {
	err error
}

func (s stubDependencyPinger) Ping(_ context.Context) error {
	return s.err
}

func TestAdminCatalogFlow(t *testing.T) {
	handler := New(config.Load(), nil)

	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"anthropic","display_name":"Anthropic","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}

	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	if providerResp.Data.Name != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", providerResp.Data.Name)
	}

	createModelBody := `{"canonical_name":"claude-sonnet-4","display_name":"Claude Sonnet 4","family":"claude","context_window":200000,"max_output_tokens":4096,"quality_tier":"premium","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`
	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(createModelBody))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}

	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(createModelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}
	if modelResp.Data.CanonicalName != "claude-sonnet-4" {
		t.Fatalf("expected model claude-sonnet-4, got %s", modelResp.Data.CanonicalName)
	}

	createAliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/aliases", strings.NewReader(`{"alias":"claude-sonnet","fallback_models":["gpt-4o-mini"],"strategy_hint":"balanced","status":"active"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasReq.AddCookie(sessionCookie)
	createAliasReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAliasRec := httptest.NewRecorder()
	handler.ServeHTTP(createAliasRec, createAliasReq)
	if createAliasRec.Code != http.StatusCreated {
		t.Fatalf("expected model alias create 201, got %d", createAliasRec.Code)
	}
	var aliasResp apiopenapi.ModelAliasResponse
	if err := json.NewDecoder(createAliasRec.Body).Decode(&aliasResp); err != nil {
		t.Fatalf("decode model alias response: %v", err)
	}
	if aliasResp.Data.Alias != "claude-sonnet" || aliasResp.Data.ModelId != modelResp.Data.Id {
		t.Fatalf("unexpected alias response: %+v", aliasResp.Data)
	}

	createMappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"2","upstream_model_name":"claude-3-7-sonnet","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}],"pricing_override":{"currency":"USD"}}`))
	createMappingReq.Header.Set("Content-Type", "application/json")
	createMappingReq.AddCookie(sessionCookie)
	createMappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createMappingRec := httptest.NewRecorder()
	handler.ServeHTTP(createMappingRec, createMappingReq)
	if createMappingRec.Code != http.StatusCreated {
		t.Fatalf("expected model mapping create 201, got %d", createMappingRec.Code)
	}
	var mappingResp apiopenapi.ModelProviderMappingResponse
	if err := json.NewDecoder(createMappingRec.Body).Decode(&mappingResp); err != nil {
		t.Fatalf("decode model mapping response: %v", err)
	}
	if mappingResp.Data.ModelId != modelResp.Data.Id || mappingResp.Data.ProviderId != providerResp.Data.Id {
		t.Fatalf("unexpected mapping response: %+v", mappingResp.Data)
	}

	createAccountBody := `{"provider_id":"2","name":"anthropic-main","runtime_class":"api_key","credential":{"api_key":"secret-value"},"status":"active","priority":10,"weight":1.5}`
	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(createAccountBody))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.AddCookie(sessionCookie)
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", createAccountRec.Code)
	}

	var accountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(createAccountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if accountResp.Data.Name != "anthropic-main" {
		t.Fatalf("expected account anthropic-main, got %s", accountResp.Data.Name)
	}

	assertAdminListContains(t, handler, sessionCookie, loginResp.Data.CsrfToken, "/api/v1/admin/providers", func(payload []byte) int {
		var resp apiopenapi.ProviderListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode providers list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertAdminListContains(t, handler, sessionCookie, loginResp.Data.CsrfToken, "/api/v1/admin/models", func(payload []byte) int {
		var resp apiopenapi.ModelListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode models list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertAdminListContains(t, handler, sessionCookie, loginResp.Data.CsrfToken, "/api/v1/admin/accounts", func(payload []byte) int {
		var resp apiopenapi.ProviderAccountListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode accounts list: %v", err)
		}
		return len(resp.Data)
	}, 2)

	capabilitiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/capabilities", nil)
	capabilitiesReq.AddCookie(sessionCookie)
	capabilitiesRec := httptest.NewRecorder()
	handler.ServeHTTP(capabilitiesRec, capabilitiesReq)
	if capabilitiesRec.Code != http.StatusOK {
		t.Fatalf("expected capabilities 200, got %d", capabilitiesRec.Code)
	}
	var capabilitiesResp apiopenapi.CapabilityListResponse
	if err := json.NewDecoder(capabilitiesRec.Body).Decode(&capabilitiesResp); err != nil {
		t.Fatalf("decode capabilities response: %v", err)
	}
	if len(capabilitiesResp.Data) < 4 {
		t.Fatalf("expected seeded capabilities, got %d", len(capabilitiesResp.Data))
	}

	strategiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/strategies", nil)
	strategiesReq.AddCookie(sessionCookie)
	strategiesRec := httptest.NewRecorder()
	handler.ServeHTTP(strategiesRec, strategiesReq)
	if strategiesRec.Code != http.StatusOK {
		t.Fatalf("expected strategies 200, got %d", strategiesRec.Code)
	}
	var strategiesResp apiopenapi.SchedulerStrategyListResponse
	if err := json.NewDecoder(strategiesRec.Body).Decode(&strategiesResp); err != nil {
		t.Fatalf("decode strategies response: %v", err)
	}
	if len(strategiesResp.Data) != 7 {
		t.Fatalf("expected 7 strategies, got %d", len(strategiesResp.Data))
	}
	for _, strategy := range strategiesResp.Data {
		if strategy.Version == "" || !strings.HasPrefix(strategy.ConfigHash, "sha256:") || len(strategy.Config) == 0 {
			t.Fatalf("expected seeded strategy registry descriptor, got %+v", strategy)
		}
	}

	apiKeyResp, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-sonnet","messages":[{"role":"user","content":"say hello"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d", chatRec.Code)
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat completion response: %v", err)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("expected 1 chat choice, got %d", len(chatResp.Choices))
	}
	if chatText := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); !strings.Contains(chatText, "say hello") {
		t.Fatalf("expected chat response to echo prompt, got %q", chatText)
	}

	chatStreamReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-sonnet","messages":[{"role":"user","content":"stream hello"}],"stream":true}`))
	chatStreamReq.Header.Set("Content-Type", "application/json")
	chatStreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatStreamRec := httptest.NewRecorder()
	handler.ServeHTTP(chatStreamRec, chatStreamReq)
	if body := chatStreamRec.Body.String(); !strings.Contains(body, "data:") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("expected chat stream SSE payload, got %q", body)
	}

	responsesReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-sonnet","input":"say responses"}`))
	responsesReq.Header.Set("Content-Type", "application/json")
	responsesReq.Header.Set("Authorization", "Bearer "+apiKey)
	responsesRec := httptest.NewRecorder()
	handler.ServeHTTP(responsesRec, responsesReq)
	if responsesRec.Code != http.StatusOK {
		t.Fatalf("expected responses 200, got %d", responsesRec.Code)
	}
	var responsesResp apiopenapi.ResponsesResponse
	if err := json.NewDecoder(responsesRec.Body).Decode(&responsesResp); err != nil {
		t.Fatalf("decode responses response: %v", err)
	}
	if len(responsesResp.Output) != 1 || responsesResp.Output[0].Content == nil || len(*responsesResp.Output[0].Content) == 0 {
		t.Fatal("expected responses output content")
	}
	if text := (*responsesResp.Output[0].Content)[0].Text; text == nil || !strings.Contains(*text, "say responses") {
		t.Fatalf("expected responses prompt echo, got %v", text)
	}

	responsesStreamReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-sonnet","input":"stream responses","stream":true}`))
	responsesStreamReq.Header.Set("Content-Type", "application/json")
	responsesStreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	responsesStreamRec := httptest.NewRecorder()
	handler.ServeHTTP(responsesStreamRec, responsesStreamReq)
	if body := responsesStreamRec.Body.String(); !strings.Contains(body, "event: response.output_text.delta") || !strings.Contains(body, "event: response.completed") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected responses stream SSE payload, got %q", body)
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet","max_tokens":16,"messages":[{"role":"user","content":"say messages"}]}`))
	messageReq.Header.Set("Content-Type", "application/json")
	messageReq.Header.Set("Authorization", "Bearer "+apiKey)
	messageRec := httptest.NewRecorder()
	handler.ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusOK {
		t.Fatalf("expected messages 200, got %d", messageRec.Code)
	}
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || !strings.Contains(*messageResp.Content[0].Text, "say messages") {
		t.Fatalf("expected messages prompt echo, got %v", messageResp.Content)
	}

	messageStreamReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet","max_tokens":16,"messages":[{"role":"user","content":"stream messages"}],"stream":true}`))
	messageStreamReq.Header.Set("Content-Type", "application/json")
	messageStreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	messageStreamRec := httptest.NewRecorder()
	handler.ServeHTTP(messageStreamRec, messageStreamReq)
	if body := messageStreamRec.Body.String(); !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: content_block_delta") || !strings.Contains(body, "event: message_stop") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected messages stream SSE payload, got %q", body)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) < 6 {
		t.Fatalf("expected at least 6 scheduler decisions, got %d", len(decisionsResp.Data))
	}
	latestDecision := decisionsResp.Data[len(decisionsResp.Data)-1]
	if latestDecision.SelectedAccountId == nil || latestDecision.CandidateCount == 0 || len(latestDecision.Scores) == 0 {
		t.Fatalf("expected selected account and score payload, got %+v", latestDecision)
	}
	if latestDecision.StrategyVersion == "" || !strings.HasPrefix(latestDecision.StrategyConfigHash, "sha256:") || len(latestDecision.StrategyWeights) == 0 {
		t.Fatalf("expected strategy version/hash/weights snapshot, got %+v", latestDecision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) < 6 {
		t.Fatalf("expected at least 6 usage logs, got %d", len(usageResp.Data))
	}
	for _, item := range usageResp.Data {
		if !item.Success {
			t.Fatalf("expected successful usage log, got %+v", item)
		}
		if !item.UsageEstimated {
			t.Fatalf("expected estimated usage log, got %+v", item)
		}
		if item.TotalTokens == 0 {
			t.Fatalf("expected token usage, got %+v", item)
		}
	}

	meUsageReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/usage", nil)
	meUsageReq.AddCookie(sessionCookie)
	meUsageRec := httptest.NewRecorder()
	handler.ServeHTTP(meUsageRec, meUsageReq)
	if meUsageRec.Code != http.StatusOK {
		t.Fatalf("expected current user usage 200, got %d", meUsageRec.Code)
	}
	var meUsageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(meUsageRec.Body).Decode(&meUsageResp); err != nil {
		t.Fatalf("decode current user usage: %v", err)
	}
	if len(meUsageResp.Data) != len(usageResp.Data) {
		t.Fatalf("expected current user usage count %d, got %d", len(usageResp.Data), len(meUsageResp.Data))
	}

	adminOverviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/overview", nil)
	adminOverviewReq.AddCookie(sessionCookie)
	adminOverviewRec := httptest.NewRecorder()
	handler.ServeHTTP(adminOverviewRec, adminOverviewReq)
	if adminOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected admin overview 200, got %d", adminOverviewRec.Code)
	}
	var adminOverviewResp apiopenapi.AdminOverviewResponse
	if err := json.NewDecoder(adminOverviewRec.Body).Decode(&adminOverviewResp); err != nil {
		t.Fatalf("decode admin overview: %v", err)
	}
	if adminOverviewResp.Data.ProviderCount != 2 || adminOverviewResp.Data.ModelCount != 2 || adminOverviewResp.Data.ActiveAccountCount != 2 {
		t.Fatalf("unexpected admin overview: %+v", adminOverviewResp.Data)
	}
	if adminOverviewResp.Data.UsageLogCount < 6 || adminOverviewResp.Data.SchedulerDecisionCount < 6 {
		t.Fatalf("expected usage and decision counts in overview, got %+v", adminOverviewResp.Data)
	}

	schedulerOverviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/overview", nil)
	schedulerOverviewReq.AddCookie(sessionCookie)
	schedulerOverviewRec := httptest.NewRecorder()
	handler.ServeHTTP(schedulerOverviewRec, schedulerOverviewReq)
	if schedulerOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler overview 200, got %d", schedulerOverviewRec.Code)
	}
	var schedulerOverviewResp apiopenapi.SchedulerOverviewResponse
	if err := json.NewDecoder(schedulerOverviewRec.Body).Decode(&schedulerOverviewResp); err != nil {
		t.Fatalf("decode scheduler overview: %v", err)
	}
	if schedulerOverviewResp.Data.TotalDecisions < 6 || schedulerOverviewResp.Data.SelectedDecisions < 6 || schedulerOverviewResp.Data.SuccessRate != 1 {
		t.Fatalf("unexpected scheduler overview: %+v", schedulerOverviewResp.Data)
	}

	accountHealthReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/health", nil)
	accountHealthReq.AddCookie(sessionCookie)
	accountHealthRec := httptest.NewRecorder()
	handler.ServeHTTP(accountHealthRec, accountHealthReq)
	if accountHealthRec.Code != http.StatusOK {
		t.Fatalf("expected account health 200, got %d", accountHealthRec.Code)
	}
	var accountHealthResp apiopenapi.AccountHealthResponse
	if err := json.NewDecoder(accountHealthRec.Body).Decode(&accountHealthResp); err != nil {
		t.Fatalf("decode account health: %v", err)
	}
	if accountHealthResp.Data.Status != "healthy" || accountHealthResp.Data.SuccessRate != 1 {
		t.Fatalf("unexpected account health: %+v", accountHealthResp.Data)
	}

	accountQuotaReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/quota", nil)
	accountQuotaReq.AddCookie(sessionCookie)
	accountQuotaRec := httptest.NewRecorder()
	handler.ServeHTTP(accountQuotaRec, accountQuotaReq)
	if accountQuotaRec.Code != http.StatusOK {
		t.Fatalf("expected account quota 200, got %d", accountQuotaRec.Code)
	}
	var accountQuotaResp apiopenapi.AccountQuotaListResponse
	if err := json.NewDecoder(accountQuotaRec.Body).Decode(&accountQuotaResp); err != nil {
		t.Fatalf("decode account quota: %v", err)
	}
	if len(accountQuotaResp.Data) == 0 || accountQuotaResp.Data[0].QuotaType != "monthly_tokens" || accountQuotaResp.Data[0].Used == "0" {
		t.Fatalf("unexpected account quota: %+v", accountQuotaResp.Data)
	}

	providerTestReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/"+string(providerResp.Data.Id)+"/test", nil)
	providerTestReq.AddCookie(sessionCookie)
	providerTestReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerTestRec := httptest.NewRecorder()
	handler.ServeHTTP(providerTestRec, providerTestReq)
	if providerTestRec.Code != http.StatusOK {
		t.Fatalf("expected provider test 200, got %d body=%s", providerTestRec.Code, providerTestRec.Body.String())
	}
	var providerTestResp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(providerTestRec.Body).Decode(&providerTestResp); err != nil {
		t.Fatalf("decode provider test: %v", err)
	}
	if !providerTestResp.Data.Ok || providerTestResp.Data.ProviderId == nil || *providerTestResp.Data.ProviderId != providerResp.Data.Id {
		t.Fatalf("unexpected provider test result: %+v", providerTestResp.Data)
	}

	accountTestReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", nil)
	accountTestReq.AddCookie(sessionCookie)
	accountTestReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountTestRec := httptest.NewRecorder()
	handler.ServeHTTP(accountTestRec, accountTestReq)
	if accountTestRec.Code != http.StatusOK {
		t.Fatalf("expected account test 200, got %d body=%s", accountTestRec.Code, accountTestRec.Body.String())
	}
	var accountTestResp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(accountTestRec.Body).Decode(&accountTestResp); err != nil {
		t.Fatalf("decode account test: %v", err)
	}
	if !accountTestResp.Data.Ok || accountTestResp.Data.AccountId == nil || *accountTestResp.Data.AccountId != accountResp.Data.Id {
		t.Fatalf("unexpected account test result: %+v", accountTestResp.Data)
	}

	updateProviderReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/providers/"+string(providerResp.Data.Id), strings.NewReader(`{"display_name":"Anthropic Updated","status":"disabled"}`))
	updateProviderReq.Header.Set("Content-Type", "application/json")
	updateProviderReq.AddCookie(sessionCookie)
	updateProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(updateProviderRec, updateProviderReq)
	if updateProviderRec.Code != http.StatusOK {
		t.Fatalf("expected provider update 200, got %d", updateProviderRec.Code)
	}
	var updateProviderResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(updateProviderRec.Body).Decode(&updateProviderResp); err != nil {
		t.Fatalf("decode provider update: %v", err)
	}
	if updateProviderResp.Data.DisplayName != "Anthropic Updated" || updateProviderResp.Data.Status != apiopenapi.ResourceStatusDisabled {
		t.Fatalf("unexpected provider update response: %+v", updateProviderResp.Data)
	}

	updateModelReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/models/"+string(modelResp.Data.Id), strings.NewReader(`{"display_name":"Claude Sonnet Updated","status":"disabled","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"json_mode","level":"optional","status":"stable","version":"v1"}]}`))
	updateModelReq.Header.Set("Content-Type", "application/json")
	updateModelReq.AddCookie(sessionCookie)
	updateModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateModelRec := httptest.NewRecorder()
	handler.ServeHTTP(updateModelRec, updateModelReq)
	if updateModelRec.Code != http.StatusOK {
		t.Fatalf("expected model update 200, got %d", updateModelRec.Code)
	}
	var updateModelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(updateModelRec.Body).Decode(&updateModelResp); err != nil {
		t.Fatalf("decode model update: %v", err)
	}
	if updateModelResp.Data.DisplayName != "Claude Sonnet Updated" || updateModelResp.Data.Status != apiopenapi.ResourceStatusDisabled || len(updateModelResp.Data.Capabilities) != 2 {
		t.Fatalf("unexpected model update response: %+v", updateModelResp.Data)
	}

	updateAccountReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"name":"anthropic-main-updated","priority":20,"weight":2,"metadata":{"base_url":"https://example.invalid/v1"}}`))
	updateAccountReq.Header.Set("Content-Type", "application/json")
	updateAccountReq.AddCookie(sessionCookie)
	updateAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(updateAccountRec, updateAccountReq)
	if updateAccountRec.Code != http.StatusOK {
		t.Fatalf("expected account update 200, got %d", updateAccountRec.Code)
	}
	var updateAccountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(updateAccountRec.Body).Decode(&updateAccountResp); err != nil {
		t.Fatalf("decode account update: %v", err)
	}
	if updateAccountResp.Data.Name != "anthropic-main-updated" || updateAccountResp.Data.Priority != 20 {
		t.Fatalf("unexpected account update response: %+v", updateAccountResp.Data)
	}

	disableAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/disable", nil)
	disableAccountReq.AddCookie(sessionCookie)
	disableAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	disableAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(disableAccountRec, disableAccountReq)
	if disableAccountRec.Code != http.StatusOK {
		t.Fatalf("expected account disable 200, got %d", disableAccountRec.Code)
	}
	var disableAccountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(disableAccountRec.Body).Decode(&disableAccountResp); err != nil {
		t.Fatalf("decode account disable: %v", err)
	}
	if disableAccountResp.Data.Status != apiopenapi.ProviderAccountStatusDisabled {
		t.Fatalf("expected disabled account, got %+v", disableAccountResp.Data)
	}

	enableAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/enable", nil)
	enableAccountReq.AddCookie(sessionCookie)
	enableAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	enableAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(enableAccountRec, enableAccountReq)
	if enableAccountRec.Code != http.StatusOK {
		t.Fatalf("expected account enable 200, got %d", enableAccountRec.Code)
	}
	var enableAccountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(enableAccountRec.Body).Decode(&enableAccountResp); err != nil {
		t.Fatalf("decode account enable: %v", err)
	}
	if enableAccountResp.Data.Status != apiopenapi.ProviderAccountStatusActive {
		t.Fatalf("expected active account, got %+v", enableAccountResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"api_key.create", "provider.create", "model.create", "model_alias.create", "model_provider_mapping.create", "provider_account.create", "provider.test", "provider_account.test", "provider.update", "model.update", "provider_account.update", "provider_account.disable", "provider_account.enable"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
	if auditResp.Data[0].TraceId == "" || auditResp.Data[0].ActorUserId == nil {
		t.Fatalf("expected audit trace and actor, got %+v", auditResp.Data[0])
	}
	providerUpdateAudit := mustFindAuditLog(t, auditResp.Data, "provider.update")
	if providerUpdateAudit.Before["status"] == providerUpdateAudit.After["status"] || providerUpdateAudit.After["status"] != "disabled" {
		t.Fatalf("expected provider audit before/after status change, got before=%+v after=%+v", providerUpdateAudit.Before, providerUpdateAudit.After)
	}
	accountUpdateAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.update")
	if _, ok := accountUpdateAudit.After["credential_ciphertext"]; ok {
		t.Fatalf("account audit must not expose credential ciphertext: %+v", accountUpdateAudit.After)
	}

	billingReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/billing-ledger?reference_type=usage_log", nil)
	billingReq.AddCookie(sessionCookie)
	billingRec := httptest.NewRecorder()
	handler.ServeHTTP(billingRec, billingReq)
	if billingRec.Code != http.StatusOK {
		t.Fatalf("expected billing ledger 200, got %d", billingRec.Code)
	}
	var billingResp apiopenapi.BillingLedgerListResponse
	if err := json.NewDecoder(billingRec.Body).Decode(&billingResp); err != nil {
		t.Fatalf("decode billing ledger: %v", err)
	}
	if len(billingResp.Data) != 0 {
		t.Fatalf("expected usage charges to wait for balance worker, got %+v", billingResp.Data)
	}

	outboxReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/events/outbox?event_type=GatewayRequestCompleted", nil)
	outboxReq.AddCookie(sessionCookie)
	outboxRec := httptest.NewRecorder()
	handler.ServeHTTP(outboxRec, outboxReq)
	if outboxRec.Code != http.StatusOK {
		t.Fatalf("expected outbox 200, got %d", outboxRec.Code)
	}
	var outboxResp apiopenapi.DomainEventOutboxListResponse
	if err := json.NewDecoder(outboxRec.Body).Decode(&outboxResp); err != nil {
		t.Fatalf("decode outbox events: %v", err)
	}
	if len(outboxResp.Data) < len(usageResp.Data) {
		t.Fatalf("expected outbox events for usage logs, got %d events for %d usage logs", len(outboxResp.Data), len(usageResp.Data))
	}
	for _, event := range outboxResp.Data {
		if event.EventType != "GatewayRequestCompleted" || event.ProducerModule != "gateway" || event.Status != apiopenapi.DomainEventOutboxStatusPending {
			t.Fatalf("unexpected outbox event: %+v", event)
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatalf("marshal outbox payload: %v", err)
		}
		if strings.Contains(string(payload), "say hello") || strings.Contains(string(payload), "secret-value") {
			t.Fatalf("outbox payload leaked request or credential material: %s", string(payload))
		}
	}

	_ = apiKeyResp
}

func TestGatewayRejectsModelWithoutProviderMapping(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"unmapped-provider","display_name":"Unmapped Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}

	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"unmapped-model","display_name":"Unmapped Model","status":"active"}`))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}

	apiKeyResp, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"unmapped-model","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unmapped model 503, got %d", rec.Code)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=unmapped-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected 1 unmapped decision, got %d", len(decisionsResp.Data))
	}
	if decisionsResp.Data[0].SelectedAccountId != nil || decisionsResp.Data[0].CandidateCount != 0 {
		t.Fatalf("expected no selected account and no candidates, got %+v", decisionsResp.Data[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=unmapped-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].Success || usageResp.Data[0].ErrorClass == nil || *usageResp.Data[0].ErrorClass != "no_available_account" {
		t.Fatalf("expected failed no_available_account usage log, got %+v", usageResp.Data)
	}

	outboxReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/events/outbox?event_type=GatewayRequestCompleted", nil)
	outboxReq.AddCookie(sessionCookie)
	outboxRec := httptest.NewRecorder()
	handler.ServeHTTP(outboxRec, outboxReq)
	if outboxRec.Code != http.StatusOK {
		t.Fatalf("expected outbox 200, got %d", outboxRec.Code)
	}
	var outboxResp apiopenapi.DomainEventOutboxListResponse
	if err := json.NewDecoder(outboxRec.Body).Decode(&outboxResp); err != nil {
		t.Fatalf("decode outbox events: %v", err)
	}
	if len(outboxResp.Data) != 1 {
		t.Fatalf("expected one failed gateway outbox event, got %d", len(outboxResp.Data))
	}
	event := outboxResp.Data[0]
	if event.Payload["success"] != false || event.Payload["error_class"] != "no_available_account" || event.AggregateType != "usage_log" {
		t.Fatalf("unexpected failed gateway outbox event: %+v", event)
	}
	_ = apiKeyResp
}

func TestGatewaySubscriptionEntitlementRejectsBeforeSchedulerConsumesAccount(t *testing.T) {
	schedulerStore := schedulermemory.New()
	subscriptionStore := subscriptionmemory.New()
	subscriptionSvc, err := subscriptionservice.New(subscriptionStore, nil)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	handler := New(config.Load(), nil,
		WithSchedulerStore(schedulerStore),
		WithSubscriptionStore(subscriptionStore),
	)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"entitlement-provider","display_name":"Entitlement Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"blocked-model","display_name":"Blocked Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"blocked-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"entitlement-account","runtime_class":"api_key","credential":{"api_key":"entitlement-secret"},"status":"active"}`)

	userID, err := strconv.Atoi(string(loginResp.Data.User.Id))
	if err != nil {
		t.Fatalf("parse login user id: %v", err)
	}
	plan, err := subscriptionSvc.CreatePlan(t.Context(), subscriptioncontract.CreatePlanRequest{
		Name:         "locked-plan",
		Price:        "0",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"allowed_models": []any{"allowed-model"},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := subscriptionSvc.CreateUserSubscription(t.Context(), subscriptioncontract.CreateSubscriptionRequest{
		UserID: userID,
		PlanID: plan.ID,
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"blocked-model","messages":[{"role":"user","content":"should be rejected"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected entitlement rejection 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	var gatewayErr apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&gatewayErr); err != nil {
		t.Fatalf("decode gateway error: %v", err)
	}
	if gatewayErr.Error.Code == nil || *gatewayErr.Error.Code != "entitlement_model_not_allowed" {
		t.Fatalf("expected entitlement error code, got %+v", gatewayErr.Error)
	}

	decisions, err := schedulerStore.ListDecisions(t.Context())
	if err != nil {
		t.Fatalf("list scheduler decisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("entitlement rejection must happen before scheduler decision, got %+v", decisions)
	}
	leases, err := schedulerStore.ListLeases(t.Context())
	if err != nil {
		t.Fatalf("list scheduler leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("entitlement rejection must not acquire leases, got %+v", leases)
	}
	feedbacks, err := schedulerStore.ListFeedbacks(t.Context())
	if err != nil {
		t.Fatalf("list scheduler feedbacks: %v", err)
	}
	if len(feedbacks) != 0 {
		t.Fatalf("entitlement rejection must not record scheduler feedback, got %+v", feedbacks)
	}
}

func TestGatewayInvokesOpenAICompatibleProviderAdapter(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-secret" {
			t.Fatalf("unexpected upstream authorization %q", got)
		}
		if r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "upstream-model" || len(payload.Messages) != 1 || !strings.Contains(payload.Messages[0].Content, "call upstream") {
			t.Fatalf("unexpected upstream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"upstream adapter response"}}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"upstream-openai","display_name":"Upstream OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.AddCookie(sessionCookie)
	providerReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", providerRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	modelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"adapter-model","display_name":"Adapter Model","status":"active"}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.AddCookie(sessionCookie)
	modelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", modelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	mappingBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"upstream-model","status":"active"}`
	mappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(mappingBody))
	mappingReq.Header.Set("Content-Type", "application/json")
	mappingReq.AddCookie(sessionCookie)
	mappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	mappingRec := httptest.NewRecorder()
	handler.ServeHTTP(mappingRec, mappingReq)
	if mappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", mappingRec.Code)
	}

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"upstream-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1"},"status":"active"}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")
	accountReq.AddCookie(sessionCookie)
	accountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", accountRec.Code)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"adapter-model","messages":[{"role":"user","content":"call upstream"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d", chatRec.Code)
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if text := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); text != "upstream adapter response" {
		t.Fatalf("expected upstream response text, got %q", text)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=adapter-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].UsageEstimated || usageResp.Data[0].TotalTokens != 12 {
		t.Fatalf("expected parsed upstream usage, got %+v", usageResp.Data)
	}
}

func TestGatewaySameProtocolRawPassthroughPreservesRequestAndResponse(t *testing.T) {
	decodeObject := func(t *testing.T, raw []byte) map[string]any {
		t.Helper()
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode json object: %v body=%s", err, string(raw))
		}
		return payload
	}

	t.Run("openai chat", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected OpenAI upstream path %s", r.URL.Path)
			}
			payload := decodeObject(t, mustReadAll(t, r.Body))
			streamOptions, _ := payload["stream_options"].(map[string]any)
			if payload["model"] != "raw-openai-upstream" ||
				payload["parallel_tool_calls"] != true ||
				payload["user"] != "raw-user" ||
				payload["stream"] != false ||
				streamOptions["include_usage"] != false {
				t.Fatalf("expected raw OpenAI request fields to be preserved, got %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl_raw","object":"chat.completion","created":123,"model":"raw-openai-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"raw openai"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3},"raw_only_marker":"openai-upstream"}`))
		}))
		defer upstream.Close()

		handler := New(config.Load(), nil)
		loginResp, sessionCookie := mustLoginAdmin(t, handler)
		providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"raw-openai-provider","display_name":"Raw OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
		modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"raw-openai-model","display_name":"Raw OpenAI Model","status":"active"}`)
		mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"raw-openai-upstream","status":"active"}`)
		mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"raw-openai-account","runtime_class":"api_key","credential":{"api_key":"raw-openai-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
		_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

		rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"raw-openai-model","messages":[{"role":"user","content":"raw chat"}],"parallel_tool_calls":true,"stream_options":{"include_usage":false},"user":"raw-user"}`)
		response := decodeObject(t, rec.Body.Bytes())
		if response["id"] != "chatcmpl_raw" || response["raw_only_marker"] != "openai-upstream" {
			t.Fatalf("expected raw OpenAI response to be replayed, got %+v", response)
		}
	})

	t.Run("anthropic messages", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/messages" {
				t.Fatalf("unexpected Anthropic upstream path %s", r.URL.Path)
			}
			payload := decodeObject(t, mustReadAll(t, r.Body))
			container, _ := payload["container"].(map[string]any)
			if payload["model"] != "raw-claude-upstream" ||
				payload["service_tier"] != "auto" ||
				payload["stream"] != false ||
				container["id"] != "container-raw" {
				t.Fatalf("expected raw Anthropic request fields to be preserved, got %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg_raw","type":"message","role":"assistant","model":"raw-claude-upstream","content":[{"type":"text","text":"raw anthropic"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1},"raw_only_marker":"anthropic-upstream"}`))
		}))
		defer upstream.Close()

		handler := New(config.Load(), nil)
		loginResp, sessionCookie := mustLoginAdmin(t, handler)
		providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"raw-anthropic-provider","display_name":"Raw Anthropic","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
		modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"raw-anthropic-model","display_name":"Raw Anthropic Model","status":"active"}`)
		mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"raw-claude-upstream","status":"active"}`)
		mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"raw-anthropic-account","runtime_class":"api_key","credential":{"api_key":"raw-anthropic-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
		_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

		rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"raw-anthropic-model","max_tokens":32,"messages":[{"role":"user","content":"raw messages"}],"service_tier":"auto","container":{"id":"container-raw"}}`)
		response := decodeObject(t, rec.Body.Bytes())
		if response["id"] != "msg_raw" || response["raw_only_marker"] != "anthropic-upstream" {
			t.Fatalf("expected raw Anthropic response to be replayed, got %+v", response)
		}
	})

	t.Run("gemini generate content", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1beta/models/raw-gemini-upstream:generateContent" {
				t.Fatalf("unexpected Gemini upstream path %s", r.URL.Path)
			}
			payload := decodeObject(t, mustReadAll(t, r.Body))
			if payload["cachedContent"] != "cachedContents/raw" || payload["model"] != nil || payload["stream"] != nil {
				t.Fatalf("expected raw Gemini request fields to be preserved, got %+v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"raw gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1},"rawOnlyMarker":"gemini-upstream"}`))
		}))
		defer upstream.Close()

		handler := New(config.Load(), nil)
		loginResp, sessionCookie := mustLoginAdmin(t, handler)
		providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"raw-gemini-provider","display_name":"Raw Gemini","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
		modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"raw-gemini-model","display_name":"Raw Gemini Model","status":"active"}`)
		mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"raw-gemini-upstream","status":"active"}`)
		mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"raw-gemini-account","runtime_class":"api_key","credential":{"api_key":"raw-gemini-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta"},"status":"active"}`)
		_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

		rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/raw-gemini-model:generateContent", `{"contents":[{"role":"user","parts":[{"text":"raw gemini"}]}],"cachedContent":"cachedContents/raw"}`)
		response := decodeObject(t, rec.Body.Bytes())
		if response["rawOnlyMarker"] != "gemini-upstream" {
			t.Fatalf("expected raw Gemini response to be replayed, got %+v", response)
		}
	})
}

func TestGatewayInvokesGenericReverseProxyProviderAdapter(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.URL.Path != "/v2/chat" {
			t.Fatalf("unexpected generic upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "generic-secret" {
			t.Fatalf("unexpected generic upstream auth %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("generic adapter must not send default authorization header, got %q", got)
		}
		var payload struct {
			UpstreamModel string `json:"upstream_model"`
			PromptItems   []struct {
				Content string `json:"content"`
			} `json:"prompt_items"`
			Route string `json:"route"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode generic upstream request: %v", err)
		}
		if payload.UpstreamModel != "generic-upstream" || len(payload.PromptItems) != 1 || payload.PromptItems[0].Content != "call generic upstream" || payload.Route != "gateway-test" {
			t.Fatalf("unexpected generic upstream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"text":"generic gateway response"},"metering":{"input_tokens":6,"output_tokens":7,"cached_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"generic-http-provider","display_name":"Generic HTTP Provider","adapter_type":"generic-reverse-proxy","protocol":"openai-compatible","status":"active","config_schema":{"base_url":"`+upstream.URL+`","chat_path":"/v2/chat","auth_header_template":"X-Api-Key: {{api_key}}","body_mapping_rules":{"model_field":"upstream_model","messages_field":"prompt_items","extra":{"route":"gateway-test"}},"response_path_rules":{"text_path":"output.text","usage_path":"metering"}}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"generic-http-model","display_name":"Generic HTTP Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"generic-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"generic-http-account","runtime_class":"api_key","credential":{"api_key":"generic-secret"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"generic-http-model","messages":[{"role":"user","content":"call generic upstream"}]}`)
	if upstreamCalls != 1 {
		t.Fatalf("expected one generic upstream call, got %d", upstreamCalls)
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode generic chat response: %v", err)
	}
	if text := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); text != "generic gateway response" {
		t.Fatalf("expected generic response text, got %q", text)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=generic-http-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode generic usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].UsageEstimated || usageResp.Data[0].InputTokens != 6 || usageResp.Data[0].OutputTokens != 7 || usageResp.Data[0].CachedTokens != 2 || usageResp.Data[0].TotalTokens != 15 {
		t.Fatalf("expected parsed generic usage, got %+v", usageResp.Data)
	}
	if usageResp.Data[0].TargetProtocol == nil || *usageResp.Data[0].TargetProtocol != "openai-compatible" {
		t.Fatalf("expected generic provider to preserve target protocol evidence, got %+v", usageResp.Data[0])
	}
}

func TestGatewayUsageLogPersistsPricingRuleCost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"priced adapter response"}}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"priced-openai","display_name":"Priced OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"priced-model","display_name":"Priced Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"priced-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"priced-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	pricingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules", strings.NewReader(`{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"1000","output_price_per_million_tokens":"2000","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"USD"}`))
	pricingReq.Header.Set("Content-Type", "application/json")
	pricingReq.AddCookie(sessionCookie)
	pricingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	pricingRec := httptest.NewRecorder()
	handler.ServeHTTP(pricingRec, pricingReq)
	if pricingRec.Code != http.StatusCreated {
		t.Fatalf("expected pricing rule create 201, got %d body=%s", pricingRec.Code, pricingRec.Body.String())
	}
	var pricingResp apiopenapi.PricingRuleResponse
	if err := json.NewDecoder(pricingRec.Body).Decode(&pricingResp); err != nil {
		t.Fatalf("decode pricing rule: %v", err)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"priced-model","messages":[{"role":"user","content":"call priced upstream"}]}`)

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=priced-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage log, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if usage.Cost != "0.01900000" || usage.Currency != "USD" || usage.TotalTokens != 12 {
		t.Fatalf("expected priced usage cost, got %+v", usage)
	}

	billingReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/billing-ledger?reference_type=usage_log", nil)
	billingReq.AddCookie(sessionCookie)
	billingRec := httptest.NewRecorder()
	handler.ServeHTTP(billingRec, billingReq)
	if billingRec.Code != http.StatusOK {
		t.Fatalf("expected billing ledger 200, got %d body=%s", billingRec.Code, billingRec.Body.String())
	}
	var billingResp apiopenapi.BillingLedgerListResponse
	if err := json.NewDecoder(billingRec.Body).Decode(&billingResp); err != nil {
		t.Fatalf("decode billing ledger: %v", err)
	}
	pricingRuleID, err := strconv.Atoi(string(pricingResp.Data.Id))
	if err != nil {
		t.Fatalf("parse pricing rule id: %v", err)
	}
	if len(billingResp.Data) != 0 {
		t.Fatalf("expected usage charges to wait for balance worker, got %+v", billingResp.Data)
	}
	if pricingRuleID <= 0 {
		t.Fatalf("expected pricing rule id, got %d", pricingRuleID)
	}
}

func TestGatewayUsageReturnsCurrentAPIKeySnapshot(t *testing.T) {
	usageStore := usagememory.New()
	handler := New(config.Load(), nil, WithUsageStore(usageStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"usage-key","scopes":["gateway:invoke"],"allowed_models":["usage-model"],"rpm_limit":60,"tpm_limit":1000,"concurrency_limit":2}`)
	otherKeyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"other-usage-key","scopes":["gateway:invoke"]}`)

	apiKeyID, err := strconv.Atoi(string(keyResp.Data.ApiKey.Id))
	if err != nil {
		t.Fatalf("parse api key id: %v", err)
	}
	otherAPIKeyID, err := strconv.Atoi(string(otherKeyResp.Data.ApiKey.Id))
	if err != nil {
		t.Fatalf("parse other api key id: %v", err)
	}
	createdAt := time.Now().UTC().Add(-time.Hour)
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_gateway_usage_key",
		UserID:         1,
		APIKeyID:       apiKeyID,
		SourceEndpoint: "/v1/chat/completions",
		Model:          "usage-model",
		InputTokens:    3,
		OutputTokens:   4,
		CachedTokens:   1,
		TotalTokens:    8,
		Success:        true,
		Cost:           "0.10000000",
		CreatedAt:      createdAt,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_gateway_usage_other_key",
		UserID:         1,
		APIKeyID:       otherAPIKeyID,
		SourceEndpoint: "/v1/chat/completions",
		Model:          "usage-model",
		InputTokens:    100,
		OutputTokens:   100,
		TotalTokens:    200,
		Success:        true,
		Cost:           "9.00000000",
		CreatedAt:      createdAt,
	})

	usageReq := httptest.NewRequest(http.MethodGet, "/v1/usage?days=30", nil)
	usageReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected gateway usage 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.GatewayUsageResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode gateway usage: %v", err)
	}
	if usageResp.Object != apiopenapi.Usage || usageResp.ApiKeyId != keyResp.Data.ApiKey.Id || usageResp.Mode != apiopenapi.QuotaLimited || !usageResp.IsValid {
		t.Fatalf("unexpected gateway usage identity fields: %+v", usageResp)
	}
	if usageResp.Usage.Requests != 1 || usageResp.Usage.TotalTokens != 8 || usageResp.Usage.Cost != "0.10000000" {
		t.Fatalf("expected key-scoped usage totals, got %+v", usageResp.Usage)
	}
	if usageResp.Limits == nil || (*usageResp.Limits)["rpm"] != float64(60) || (*usageResp.Limits)["tpm"] != float64(1000) || (*usageResp.Limits)["concurrency"] != float64(2) {
		t.Fatalf("expected key limits in usage response, got %+v", usageResp.Limits)
	}
	if usageResp.AllowedModels == nil || len(*usageResp.AllowedModels) != 1 || (*usageResp.AllowedModels)[0] != "usage-model" {
		t.Fatalf("expected allowed model policy, got %+v", usageResp.AllowedModels)
	}
	if len(usageResp.RecentRequests) != 1 || usageResp.RecentRequests[0].RequestId != "req_gateway_usage_key" {
		t.Fatalf("expected scoped recent request, got %+v", usageResp.RecentRequests)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/v1/usage?days=0", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid days 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestGatewayGeminiGenerateContentUsesCanonicalPipeline(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []upstreamGeminiCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model     string            `json:"model"`
			Messages  []upstreamMessage `json:"messages"`
			MaxTokens int               `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamGeminiCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Messages:      payload.Messages,
			MaxTokens:     payload.MaxTokens,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"gemini upstream response"}}],"usage":{"prompt_tokens":6,"completion_tokens":8,"total_tokens":14}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp, modelResp, accountResp := mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"systemInstruction":{"parts":[{"text":"be brief"}]},"contents":[{"role":"user","parts":[{"text":"gemini route prompt"}]}],"generationConfig":{"maxOutputTokens":32,"temperature":0.2,"topP":0.8}}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/gemini-route-model:generateContent", body)
	var resp apiopenapi.GeminiGenerateContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gemini response: %v", err)
	}
	if len(resp.Candidates) != 1 || len(resp.Candidates[0].Content.Parts) != 1 || resp.Candidates[0].Content.Parts[0].Text == nil || *resp.Candidates[0].Content.Parts[0].Text != "gemini upstream response" {
		t.Fatalf("unexpected gemini response: %+v", resp)
	}
	if resp.UsageMetadata == nil || resp.UsageMetadata.TotalTokenCount == nil || *resp.UsageMetadata.TotalTokenCount != 14 {
		t.Fatalf("expected parsed gemini usage metadata, got %+v", resp.UsageMetadata)
	}

	mu.Lock()
	gotCalls := append([]upstreamGeminiCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/chat/completions" || call.Authorization != "Bearer gemini-route-upstream-secret" || call.Model != "gemini-route-upstream" || call.MaxTokens != 32 {
		t.Fatalf("unexpected upstream call: %+v", call)
	}
	if len(call.Messages) != 2 || call.Messages[0].Role != "system" || call.Messages[0].Content != "be brief" || call.Messages[1].Role != "user" || !strings.Contains(call.Messages[1].Content, "gemini route prompt") {
		t.Fatalf("expected canonical messages forwarded upstream, got %+v", call.Messages)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=gemini-route-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceProtocol != "gemini-compatible" || usage.SourceEndpoint != "/v1beta/models/gemini-route-model:generateContent" || usage.TargetProtocol == nil || *usage.TargetProtocol != "openai-compatible" || usage.TotalTokens != 14 || usage.UsageEstimated {
		t.Fatalf("unexpected gemini usage record: %+v", usage)
	}
	if usage.ProviderId == nil || *usage.ProviderId != providerResp.Data.Id || usage.AccountId == nil || *usage.AccountId != accountResp.Data.Id || usage.Model != modelResp.Data.CanonicalName {
		t.Fatalf("expected provider/account/model evidence, got %+v", usage)
	}
}

func TestGatewayGeminiGenerateContentSchedulesGeminiCompatibleUpstream(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []upstreamNativeGeminiCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			SystemInstruction *struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"systemInstruction"`
			GenerationConfig *struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode gemini upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamNativeGeminiCall{
			Path:       r.URL.Path,
			APIKey:     r.URL.Query().Get("key"),
			Contents:   payload.Contents,
			SystemText: geminiSystemInstructionText(payload.SystemInstruction),
			MaxTokens:  payload.GenerationConfig.MaxOutputTokens,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"native gemini upstream response"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp, modelResp, accountResp := mustCreateNativeGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"systemInstruction":{"parts":[{"text":"answer tersely"}]},"contents":[{"role":"user","parts":[{"text":"native gemini prompt"}]}],"generationConfig":{"maxOutputTokens":48}}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/native-gemini-route-model:generateContent", body)
	var resp apiopenapi.GeminiGenerateContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gemini response: %v", err)
	}
	if len(resp.Candidates) != 1 || len(resp.Candidates[0].Content.Parts) != 1 || resp.Candidates[0].Content.Parts[0].Text == nil || *resp.Candidates[0].Content.Parts[0].Text != "native gemini upstream response" {
		t.Fatalf("unexpected native gemini response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]upstreamNativeGeminiCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one native gemini upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1beta/models/gemini-native-upstream:generateContent" || call.APIKey != "native-gemini-secret" || call.MaxTokens != 48 {
		t.Fatalf("unexpected native gemini call: %+v", call)
	}
	if call.SystemText != "answer tersely" || len(call.Contents) != 1 || call.Contents[0].Role != "user" || call.Contents[0].Parts[0].Text != "native gemini prompt" {
		t.Fatalf("expected canonical Gemini payload, got %+v", call)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=native-gemini-route-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.TargetProtocol == nil || *usage.TargetProtocol != "gemini-compatible" || usage.TotalTokens != 15 || usage.UsageEstimated {
		t.Fatalf("unexpected native gemini usage record: %+v", usage)
	}
	if usage.ProviderId == nil || *usage.ProviderId != providerResp.Data.Id || usage.AccountId == nil || *usage.AccountId != accountResp.Data.Id || usage.Model != modelResp.Data.CanonicalName {
		t.Fatalf("expected provider/account/model evidence, got %+v", usage)
	}
}

func TestGatewayGeminiCountTokensSchedulesGeminiCompatibleUpstream(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []upstreamNativeGeminiCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode gemini count request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamNativeGeminiCall{
			Path:     r.URL.Path,
			APIKey:   r.URL.Query().Get("key"),
			Contents: payload.Contents,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalTokens":19,"cachedContentTokenCount":3,"promptTokensDetails":[{"modality":"TEXT","tokenCount":16}]}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp, modelResp, accountResp := mustCreateNativeGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"contents":[{"role":"user","parts":[{"text":"count native gemini prompt"}]}]}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/native-gemini-route-model:countTokens", body)
	var resp apiopenapi.GeminiCountTokensResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gemini count response: %v", err)
	}
	if resp.TotalTokens != 19 || resp.CachedContentTokenCount == nil || *resp.CachedContentTokenCount != 3 || resp.PromptTokensDetails == nil || len(*resp.PromptTokensDetails) != 1 {
		t.Fatalf("unexpected gemini count response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]upstreamNativeGeminiCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one native gemini count call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1beta/models/gemini-native-upstream:countTokens" || call.APIKey != "native-gemini-secret" {
		t.Fatalf("unexpected native gemini count call: %+v", call)
	}
	if len(call.Contents) != 1 || call.Contents[0].Role != "user" || call.Contents[0].Parts[0].Text != "count native gemini prompt" {
		t.Fatalf("expected countTokens payload forwarded, got %+v", call)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=native-gemini-route-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.TotalTokens != 0 || usage.Cost != "0.00000000" || usage.SourceEndpoint != "/v1beta/models/native-gemini-route-model:countTokens" {
		t.Fatalf("unexpected token count usage record: %+v", usage)
	}
	if usage.ProviderId == nil || *usage.ProviderId != providerResp.Data.Id || usage.AccountId == nil || *usage.AccountId != accountResp.Data.Id || usage.Model != modelResp.Data.CanonicalName {
		t.Fatalf("expected provider/account/model evidence, got %+v", usage)
	}
}

func TestGatewayAnthropicCountTokensSchedulesAnthropicCompatibleUpstream(t *testing.T) {
	type anthropicCountCall struct {
		Path    string
		APIKey  string
		Version string
		Model   string
		System  string
		Message string
	}
	var (
		mu    sync.Mutex
		calls []anthropicCountCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string `json:"model"`
			System   string `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode Anthropic count request: %v", err)
		}
		message := ""
		if len(payload.Messages) > 0 {
			message = payload.Messages[0].Content
		}
		mu.Lock()
		calls = append(calls, anthropicCountCall{
			Path:    r.URL.Path,
			APIKey:  r.Header.Get("x-api-key"),
			Version: r.Header.Get("anthropic-version"),
			Model:   payload.Model,
			System:  payload.System,
			Message: message,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":23,"cache_creation_input_tokens":2}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"anthropic-count-provider","display_name":"Anthropic Count Provider","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active","capabilities":{"token_counting":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"anthropic-count-model","display_name":"Anthropic Count Model","status":"active","capabilities":[{"key":"token_counting","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-count-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"anthropic-count-account","runtime_class":"api_key","credential":{"api_key":"anthropic-count-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"model":"anthropic-count-model","system":"count only","messages":[{"role":"user","content":"count this anthropic prompt"}]}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages/count_tokens", body)
	var resp apiopenapi.AnthropicCountTokensResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode Anthropic count response: %v", err)
	}
	if resp.InputTokens != 23 || resp.AdditionalProperties["cache_creation_input_tokens"] == nil {
		t.Fatalf("unexpected Anthropic count response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]anthropicCountCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one Anthropic count call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/messages/count_tokens" || call.APIKey != "anthropic-count-secret" || call.Version != "2023-06-01" || call.Model != "claude-count-upstream" {
		t.Fatalf("unexpected Anthropic count call: %+v", call)
	}
	if call.System != "count only" || call.Message != "count this anthropic prompt" {
		t.Fatalf("expected count_tokens body shape preserved with mapped model, got %+v", call)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=anthropic-count-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.TotalTokens != 0 || usage.Cost != "0.00000000" || usage.SourceEndpoint != "/v1/messages/count_tokens" || usage.TargetProtocol == nil || *usage.TargetProtocol != "anthropic-compatible" {
		t.Fatalf("unexpected Anthropic count usage record: %+v", usage)
	}
	if usage.ProviderId == nil || *usage.ProviderId != providerResp.Data.Id || usage.AccountId == nil || *usage.AccountId != accountResp.Data.Id || usage.Model != modelResp.Data.CanonicalName {
		t.Fatalf("expected provider/account/model evidence, got %+v", usage)
	}
}

func TestGatewayAnthropicCountTokensRequiresProviderScopedCapability(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"anthropic-count-missing-cap-provider","display_name":"Anthropic Missing Count Capability","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"anthropic-count-missing-cap-model","display_name":"Anthropic Missing Count Capability Model","status":"active","capabilities":[{"key":"token_counting","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-count-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"anthropic-count-missing-cap-account","runtime_class":"api_key","credential":{"api_key":"anthropic-count-secret"},"metadata":{"base_url":"https://api.anthropic.com/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"anthropic-count-missing-cap-model","messages":[{"role":"user","content":"count"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected no available account when provider lacks token_counting, got %d body=%s", rec.Code, rec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=anthropic-count-missing-cap-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SelectedAccountId != nil || !rejectionReasonsContain(decisionsResp.Data[0].RejectReasons, "capability_mismatch") {
		t.Fatalf("expected capability_mismatch decision, got %+v", decisionsResp.Data)
	}
}

func TestGatewayResponsesCompactRequiresProviderScopedCapability(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"compact-missing-cap-provider","display_name":"Compact Missing Capability","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"responses":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"compact-missing-cap-model","display_name":"Compact Missing Capability Model","status":"active","capabilities":[{"key":"responses_compact","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"compact-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"compact-missing-cap-account","runtime_class":"api_key","credential":{"api_key":"compact-secret"},"metadata":{"base_url":"https://api.openai.com/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(`{"model":"compact-missing-cap-model","input":"compact"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected no available account when provider lacks responses_compact, got %d body=%s", rec.Code, rec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=compact-missing-cap-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SelectedAccountId != nil || !rejectionReasonsContain(decisionsResp.Data[0].RejectReasons, "capability_mismatch") {
		t.Fatalf("expected compact capability_mismatch decision, got %+v", decisionsResp.Data)
	}
}

func TestGatewayGeminiStreamGenerateContentEmitsGeminiSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"gemini stream\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" response\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/gemini-route-model:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"stream gemini"}]}]}`)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected event stream content type, got %q", got)
	}
	body := rec.Body.String()
	for _, expected := range []string{"data:", `"text":"gemini stream"`, `"text":" response"`, "usageMetadata", "data: [DONE]"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected SSE body to contain %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, "gemini stream response") {
		t.Fatalf("expected split Gemini stream deltas, got aggregated stream body: %s", body)
	}
}

func TestGatewayGeminiProviderErrorsUseGoogleStyleEnvelope(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit","message":"slow down"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-route-model:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"rate limit me"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected Gemini rate limit 429, got %d body=%s", rec.Code, rec.Body.String())
	}
	var errResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode gemini error: %v", err)
	}
	if errResp.Error.Code != http.StatusTooManyRequests || errResp.Error.Status != "RESOURCE_EXHAUSTED" || !strings.Contains(errResp.Error.Message, "rate limit") {
		t.Fatalf("unexpected gemini error envelope: %+v", errResp)
	}
}

func TestGatewayProviderErrorMessagePassthroughMetadata(t *testing.T) {
	cases := []struct {
		name           string
		slug           string
		metadataSuffix string
		wantMessage    string
	}{
		{
			name:        "default generic message",
			slug:        "default",
			wantMessage: "provider rejected request",
		},
		{
			name:           "metadata exposes sanitized upstream message",
			slug:           "exposed",
			metadataSuffix: `,"expose_provider_error_messages":true,"provider_error_passthrough_status_codes":[400],"provider_error_passthrough_classes":["invalid_request"],"provider_error_passthrough_keywords":["schema"]`,
			wantMessage:    "invalid schema from upstream",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Fatalf("unexpected upstream path %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"invalid schema from upstream","type":"invalid_request_error"}}`))
			}))
			defer upstream.Close()

			handler := New(config.Load(), nil)
			loginResp, sessionCookie := mustLoginAdmin(t, handler)
			providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"provider-error-message-`+tc.slug+`","display_name":"Provider Error Message `+tc.slug+`","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
			modelName := "provider-error-message-model-" + tc.slug
			modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"`+modelName+`","display_name":"Provider Error Message Model `+tc.slug+`","status":"active"}`)
			mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-error-message-upstream","status":"active"}`)
			metadata := `"base_url":` + strconv.Quote(upstream.URL+"/v1") + tc.metadataSuffix
			mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"provider-error-message-`+tc.slug+`-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{`+metadata+`},"status":"active"}`)
			_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+modelName+`","messages":[{"role":"user","content":"trigger upstream validation"}]}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+apiKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected provider rejection 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			var errResp apiopenapi.GatewayErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
				t.Fatalf("decode gateway error: %v", err)
			}
			if errResp.Error.Message != tc.wantMessage {
				t.Fatalf("expected gateway message %q, got %+v", tc.wantMessage, errResp.Error)
			}
			if errResp.Error.Code == nil || *errResp.Error.Code != "invalid_request" {
				t.Fatalf("expected invalid_request code, got %+v", errResp.Error)
			}
		})
	}
}

func TestGatewayGeminiListModels(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	activeModel := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-list-visible","display_name":"Gemini List Visible","family":"gemini","context_window":131072,"max_output_tokens":8192,"status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"token_counting","level":"required","status":"stable","version":"v1"}]}`)
	secondModel := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-list-second","display_name":"Gemini List Second","quality_tier":"flash","context_window":32768,"max_output_tokens":2048,"status":"active"}`)
	_ = mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-list-disabled","display_name":"Gemini List Disabled","status":"disabled"}`)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gemini-list-key","scopes":["gateway:invoke"],"allowed_models":["`+string(activeModel.Data.CanonicalName)+`","`+string(secondModel.Data.CanonicalName)+`"]}`)

	firstReq := httptest.NewRequest(http.MethodGet, "/v1beta/models?pageSize=1", nil)
	firstReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected 200 Gemini models first page, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}
	var firstResp apiopenapi.GeminiModelList
	if err := json.NewDecoder(firstRec.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first Gemini models page: %v", err)
	}
	if len(firstResp.Models) != 1 || firstResp.Models[0].Name != "models/gemini-list-visible" || firstResp.Models[0].BaseModelId != "gemini-list-visible" || firstResp.Models[0].Version != "gemini" || firstResp.Models[0].InputTokenLimit != 131072 || firstResp.Models[0].OutputTokenLimit != 8192 {
		t.Fatalf("unexpected first Gemini model page: %+v", firstResp)
	}
	if !geminiModelMethodsContain(firstResp.Models[0].SupportedGenerationMethods, apiopenapi.GenerateContent) || !geminiModelMethodsContain(firstResp.Models[0].SupportedGenerationMethods, apiopenapi.StreamGenerateContent) || !geminiModelMethodsContain(firstResp.Models[0].SupportedGenerationMethods, apiopenapi.CountTokens) {
		t.Fatalf("expected generateContent, streamGenerateContent, and countTokens methods, got %+v", firstResp.Models[0].SupportedGenerationMethods)
	}
	if firstResp.NextPageToken == nil || *firstResp.NextPageToken != "1" {
		t.Fatalf("expected next page token 1, got %+v", firstResp.NextPageToken)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1beta/models?pageToken="+*firstResp.NextPageToken, nil)
	secondReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected 200 Gemini models second page, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var secondResp apiopenapi.GeminiModelList
	if err := json.NewDecoder(secondRec.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second Gemini models page: %v", err)
	}
	if len(secondResp.Models) != 1 || secondResp.Models[0].Name != "models/gemini-list-second" || secondResp.Models[0].Version != "flash" || secondResp.NextPageToken != nil {
		t.Fatalf("unexpected second Gemini model page: %+v", secondResp)
	}
	if geminiModelMethodsContain(secondResp.Models[0].SupportedGenerationMethods, apiopenapi.StreamGenerateContent) || geminiModelMethodsContain(secondResp.Models[0].SupportedGenerationMethods, apiopenapi.CountTokens) {
		t.Fatalf("model without streaming/token_counting should not advertise those methods: %+v", secondResp.Models[0].SupportedGenerationMethods)
	}
}

func TestGatewayGeminiGetModel(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	activeModel := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-get-visible","display_name":"Gemini Get Visible","family":"gemini","context_window":131072,"max_output_tokens":8192,"status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"token_counting","level":"required","status":"stable","version":"v1"}]}`)
	hiddenModel := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-get-hidden","display_name":"Gemini Get Hidden","status":"active"}`)
	_ = hiddenModel
	_ = mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-get-disabled","display_name":"Gemini Get Disabled","status":"disabled"}`)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gemini-get-key","scopes":["gateway:invoke"],"allowed_models":["`+string(activeModel.Data.CanonicalName)+`"]}`)

	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-get-visible", nil)
	req.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 Gemini model get, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.GeminiModelInfo
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode Gemini model get: %v", err)
	}
	if resp.Name != "models/gemini-get-visible" || resp.BaseModelId != "gemini-get-visible" || resp.Version != "gemini" || resp.InputTokenLimit != 131072 || resp.OutputTokenLimit != 8192 {
		t.Fatalf("unexpected Gemini model get response: %+v", resp)
	}
	if !geminiModelMethodsContain(resp.SupportedGenerationMethods, apiopenapi.GenerateContent) || !geminiModelMethodsContain(resp.SupportedGenerationMethods, apiopenapi.StreamGenerateContent) || !geminiModelMethodsContain(resp.SupportedGenerationMethods, apiopenapi.CountTokens) {
		t.Fatalf("expected generateContent, streamGenerateContent, and countTokens methods, got %+v", resp.SupportedGenerationMethods)
	}

	prefixedReq := httptest.NewRequest(http.MethodGet, "/v1beta/models/models%2Fgemini-get-visible", nil)
	prefixedReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	prefixedRec := httptest.NewRecorder()
	handler.ServeHTTP(prefixedRec, prefixedReq)
	if prefixedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 Gemini prefixed model get, got %d body=%s", prefixedRec.Code, prefixedRec.Body.String())
	}

	hiddenReq := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-get-hidden", nil)
	hiddenReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	hiddenRec := httptest.NewRecorder()
	handler.ServeHTTP(hiddenRec, hiddenReq)
	if hiddenRec.Code != http.StatusForbidden {
		t.Fatalf("expected hidden Gemini model get 403, got %d body=%s", hiddenRec.Code, hiddenRec.Body.String())
	}
	var hiddenResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(hiddenRec.Body).Decode(&hiddenResp); err != nil {
		t.Fatalf("decode hidden Gemini model get response: %v", err)
	}
	if hiddenResp.Error.Status != "PERMISSION_DENIED" {
		t.Fatalf("expected Gemini PERMISSION_DENIED, got %+v", hiddenResp.Error)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-get-missing", nil)
	missingReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected missing Gemini model get 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
	var missingResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(missingRec.Body).Decode(&missingResp); err != nil {
		t.Fatalf("decode missing Gemini model get response: %v", err)
	}
	if missingResp.Error.Status != "NOT_FOUND" {
		t.Fatalf("expected Gemini NOT_FOUND, got %+v", missingResp.Error)
	}
}

func TestGatewayGeminiListModelsRejectsInvalidPaginationAndDisabledKey(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gemini-list-invalid-key","scopes":["gateway:invoke"]}`)

	invalidReq := httptest.NewRequest(http.MethodGet, "/v1beta/models?pageSize=0", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid pagination 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
	var invalidResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(invalidRec.Body).Decode(&invalidResp); err != nil {
		t.Fatalf("decode invalid pagination response: %v", err)
	}
	if invalidResp.Error.Status != "INVALID_ARGUMENT" {
		t.Fatalf("expected Gemini INVALID_ARGUMENT, got %+v", invalidResp.Error)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+string(keyResp.Data.ApiKey.Id), strings.NewReader(`{"status":"disabled"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	patchReq.AddCookie(sessionCookie)
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected disable key 200, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	disabledReq := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	disabledReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	disabledRec := httptest.NewRecorder()
	handler.ServeHTTP(disabledRec, disabledReq)
	if disabledRec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled key 403, got %d body=%s", disabledRec.Code, disabledRec.Body.String())
	}
	var disabledResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(disabledRec.Body).Decode(&disabledResp); err != nil {
		t.Fatalf("decode disabled key response: %v", err)
	}
	if disabledResp.Error.Status != "PERMISSION_DENIED" {
		t.Fatalf("expected Gemini PERMISSION_DENIED, got %+v", disabledResp.Error)
	}
}

func TestGatewayGeminiAuthAcceptsGoogleAPIKeyForms(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"gemini-auth-visible","display_name":"Gemini Auth Visible","status":"active"}`)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"gemini-auth-key","scopes":["gateway:invoke"],"allowed_models":["`+string(modelResp.Data.CanonicalName)+`"]}`)
	apiKey := keyResp.Data.PlaintextKey

	tests := []struct {
		name      string
		path      string
		headerKey string
		headerVal string
	}{
		{name: "x goog header", path: "/v1beta/models/gemini-auth-visible", headerKey: "x-goog-api-key", headerVal: apiKey},
		{name: "x api key header", path: "/v1beta/models/gemini-auth-visible", headerKey: "x-api-key", headerVal: apiKey},
		{name: "authorization bearer fallback", path: "/v1beta/models/gemini-auth-visible", headerKey: "Authorization", headerVal: "Bearer " + apiKey},
		{name: "query key", path: "/v1beta/models/gemini-auth-visible?key=" + apiKey},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.headerKey != "" {
				req.Header.Set(tc.headerKey, tc.headerVal)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected Gemini auth form %q to return 200, got %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
			var resp apiopenapi.GeminiModelInfo
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode Gemini model response: %v", err)
			}
			if resp.Name != "models/gemini-auth-visible" {
				t.Fatalf("unexpected Gemini model response: %+v", resp)
			}
		})
	}
}

func TestGatewayGeminiAuthRejectsDeprecatedAPIKeyQuery(t *testing.T) {
	handler := New(config.Load(), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models?api_key=deprecated", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected deprecated api_key query to return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode Gemini error response: %v", err)
	}
	if resp.Error.Status != "INVALID_ARGUMENT" || !strings.Contains(resp.Error.Message, "api_key is deprecated") {
		t.Fatalf("unexpected deprecated api_key error: %+v", resp.Error)
	}
}

func TestGatewayOpenAIAuthRejectsGoogleAPIKeyHeader(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"openai-auth-boundary-key","scopes":["gateway:invoke"]}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("x-goog-api-key", keyResp.Data.PlaintextKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected OpenAI models route to reject Google key header, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode OpenAI auth error response: %v", err)
	}
	if resp.Error.Code == nil || *resp.Error.Code != "invalid_api_key" {
		t.Fatalf("expected invalid_api_key OpenAI auth error, got %+v", resp.Error)
	}
}

func TestGatewayRequestRecordsSchedulerFeedback(t *testing.T) {
	schedulerStore := schedulermemory.New()
	handler := New(config.Load(), nil, WithSchedulerStore(schedulerStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"feedback-provider","display_name":"Feedback Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"feedback-model","display_name":"Feedback Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"feedback-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"feedback-account","runtime_class":"api_key","credential":{"api_key":"feedback-secret"},"status":"active"}`)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"feedback-model","messages":[{"role":"user","content":"record feedback"}]}`)

	feedbacks, err := schedulerStore.ListFeedbacks(t.Context())
	if err != nil {
		t.Fatalf("list scheduler feedbacks: %v", err)
	}
	if len(feedbacks) != 1 {
		t.Fatalf("expected one scheduler feedback, got %+v", feedbacks)
	}
	accountID, err := strconv.Atoi(string(accountResp.Data.Id))
	if err != nil {
		t.Fatalf("parse account id: %v", err)
	}
	providerID, err := strconv.Atoi(string(providerResp.Data.Id))
	if err != nil {
		t.Fatalf("parse provider id: %v", err)
	}
	feedback := feedbacks[0]
	if !feedback.Success || feedback.DecisionID <= 0 || feedback.AccountID != accountID || feedback.ProviderID != providerID {
		t.Fatalf("unexpected scheduler feedback identity: %+v", feedback)
	}
	if feedback.Model != "feedback-model" || feedback.InputTokens == 0 || feedback.OutputTokens == 0 || feedback.ActualCost != "0.00000000" || feedback.Currency != "USD" {
		t.Fatalf("unexpected scheduler feedback evidence: %+v", feedback)
	}
}

func TestGatewayCompatibilityEndpointsTargetSameOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		Model         string
		Messages      []upstreamMessage
	}

	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string            `json:"model"`
			Messages []upstreamMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Messages:      payload.Messages,
		})
		mu.Unlock()

		content := "upstream compatibility response"
		if len(payload.Messages) > 0 {
			content = "upstream echo: " + payload.Messages[len(payload.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(content) + `}}],"usage":{"prompt_tokens":4,"completion_tokens":5,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp080-openai","display_name":"WP080 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp080-model","display_name":"WP080 Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"tool_calling","level":"optional","status":"stable","version":"v1"},{"key":"structured_output","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"wp080-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp080-upstream-account","runtime_class":"api_key","credential":{"api_key":"wp080-upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"wp080-model","messages":[{"role":"user","content":"chat same upstream"}]}`)
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if text := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); !strings.Contains(text, "chat same upstream") {
		t.Fatalf("expected chat response from upstream, got %q", text)
	}

	responsesRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"wp080-model","instructions":"respond briefly","input":"responses same upstream","max_output_tokens":32}`)
	var responsesResp apiopenapi.ResponsesResponse
	if err := json.NewDecoder(responsesRec.Body).Decode(&responsesResp); err != nil {
		t.Fatalf("decode responses response: %v", err)
	}
	if len(responsesResp.Output) != 1 || responsesResp.Output[0].Content == nil || len(*responsesResp.Output[0].Content) == 0 {
		t.Fatal("expected responses output content")
	}
	if text := (*responsesResp.Output[0].Content)[0].Text; text == nil || !strings.Contains(*text, "responses same upstream") {
		t.Fatalf("expected responses response from upstream, got %v", text)
	}

	messageRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"wp080-model","system":"respond briefly","max_tokens":32,"messages":[{"role":"user","content":"messages same upstream"}]}`)
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || !strings.Contains(*messageResp.Content[0].Text, "messages same upstream") {
		t.Fatalf("expected messages response from upstream, got %v", messageResp.Content)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 3 {
		t.Fatalf("expected three upstream calls, got %d: %+v", len(gotCalls), gotCalls)
	}
	expectedPrompts := []string{"chat same upstream", "responses same upstream", "messages same upstream"}
	for idx, call := range gotCalls {
		if call.Path != "/v1/chat/completions" {
			t.Fatalf("expected upstream chat completions path for call %d, got %s", idx, call.Path)
		}
		if call.Authorization != "Bearer wp080-upstream-secret" {
			t.Fatalf("expected upstream credential for call %d, got %q", idx, call.Authorization)
		}
		if call.Model != "wp080-upstream" {
			t.Fatalf("expected upstream model for call %d, got %q", idx, call.Model)
		}
		if len(call.Messages) == 0 || !strings.Contains(call.Messages[len(call.Messages)-1].Content, expectedPrompts[idx]) {
			t.Fatalf("expected upstream prompt %q for call %d, got %+v", expectedPrompts[idx], idx, call.Messages)
		}
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp080-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 3 {
		t.Fatalf("expected three usage records, got %+v", usageResp.Data)
	}
	for _, item := range usageResp.Data {
		if !item.Success || item.AccountId == nil || *item.AccountId != accountResp.Data.Id || item.ProviderId == nil || *item.ProviderId != providerResp.Data.Id {
			t.Fatalf("expected successful usage on same provider account, got %+v", item)
		}
	}
}

func TestGatewayResponsesRejectsFunctionCallOutputWithoutCallID(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"missing-call-id-model","input":[{"type":"function_call_output","output":"{}"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "function_call_output input item requires call_id") {
		t.Fatalf("expected missing call_id error, got %s", rec.Body.String())
	}
}

func TestGatewayResponsesRejectsFunctionCallOutputWithoutContinuationContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"missing-tool-context-model","input":[{"type":"function_call_output","call_id":"call_1","output":"{}"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "function_call_output input item requires matching function_call, item_reference, or previous_response_id") {
		t.Fatalf("expected missing continuation context error, got %s", rec.Body.String())
	}
}

func TestGatewayResponsesRejectsMessagePreviousResponseID(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"message-previous-response-id-model","previous_response_id":"msg_123456","input":"continue"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "previous_response_id must reference a response id") {
		t.Fatalf("expected previous_response_id message-id error, got %s", rec.Body.String())
	}
}

func TestGatewayProviderAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")

	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"fallback-provider","display_name":"Fallback Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"alias-model","display_name":"Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-upstream","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	req := httptest.NewRequest(http.MethodPost, "/api/provider/openai-compatible/v1/chat/completions", strings.NewReader(`{"model":"alias-model","messages":[{"role":"user","content":"alias route"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider alias chat 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias decision, got %d", len(decisionsResp.Data))
	}
	if decisionsResp.Data[0].SelectedProviderId == nil || *decisionsResp.Data[0].SelectedProviderId != string(openaiProvider.Id) {
		t.Fatalf("expected alias to force openai-compatible provider, got %+v", decisionsResp.Data[0])
	}
	if decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("expected alias to filter candidates to one provider, got %+v", decisionsResp.Data[0])
	}
	if decisionsResp.Data[0].SourceEndpoint != "/api/provider/openai-compatible/v1/chat/completions" {
		t.Fatalf("expected alias source endpoint in decision, got %q", decisionsResp.Data[0].SourceEndpoint)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=alias-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].SourceEndpoint != "/api/provider/openai-compatible/v1/chat/completions" {
		t.Fatalf("expected alias source endpoint in usage log, got %+v", usageResp.Data)
	}
}

func TestGatewayRootLegacyOpenAIAliasForcesProviderContext(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer root-openai-secret" {
			t.Fatalf("unexpected upstream authorization %q", got)
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		if payload.Model != "root-openai-upstream" {
			t.Fatalf("expected mapped upstream model, got %q", payload.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-root-alias","object":"chat.completion","model":"root-openai-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"root alias ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"openai","display_name":"OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"root-openai-fallback","display_name":"Root OpenAI Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"root-openai-alias-model","display_name":"Root OpenAI Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"root-openai-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Data.Id)+`","upstream_model_name":"root-openai-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(openaiProvider.Data.Id)+`","name":"root-openai-account","runtime_class":"api_key","credential":{"api_key":"root-openai-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":10}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/openai/v1/chat/completions", `{"model":"root-openai-alias-model","messages":[{"role":"user","content":"root alias"}]}`)
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode root alias chat response: %v", err)
	}
	if len(chatResp.Choices) != 1 || decodeChatMessageText(t, chatResp.Choices[0].Message.Content) != "root alias ok" {
		t.Fatalf("unexpected root alias chat response: %+v", chatResp)
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one root alias upstream call, got %d", upstreamCalls)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=root-openai-alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one root alias decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Data.Id) || decision.CandidateCount != 1 || decision.SourceEndpoint != "/openai/v1/chat/completions" {
		t.Fatalf("expected root alias to force openai provider and record source endpoint, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=root-openai-alias-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 ||
		!usageResp.Data[0].Success ||
		usageResp.Data[0].ProviderId == nil ||
		*usageResp.Data[0].ProviderId != string(openaiProvider.Data.Id) ||
		usageResp.Data[0].AccountId == nil ||
		*usageResp.Data[0].AccountId != string(accountResp.Data.Id) ||
		usageResp.Data[0].SourceEndpoint != "/openai/v1/chat/completions" ||
		usageResp.Data[0].TargetProtocol == nil ||
		*usageResp.Data[0].TargetProtocol != "openai-compatible" ||
		usageResp.Data[0].TotalTokens != 7 {
		t.Fatalf("expected root alias usage evidence, got %+v", usageResp.Data)
	}
}

func TestGatewayResponsesInputItemsAliasReplaysRawUpstreamJSON(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/responses/resp_input_items/input_items" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer input-items-secret" {
			t.Fatalf("unexpected upstream authorization %q", got)
		}
		if r.URL.Query().Get("model") != "" || r.URL.Query().Get("before") != "" {
			t.Fatalf("internal or unsupported query params leaked upstream: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("after") != "item_1" || r.URL.Query().Get("limit") != "2" || r.URL.Query().Get("order") != "asc" {
			t.Fatalf("expected input_items pagination query, got %s", r.URL.RawQuery)
		}
		if got := r.URL.Query()["include"]; len(got) != 2 || got[0] != "file_search_call.results" || got[1] != "reasoning.encrypted_content" {
			t.Fatalf("expected repeated include query params, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"item_1","type":"message","role":"user","content":[{"type":"input_text","text":"kept raw"}]}],"first_id":"item_1","last_id":"item_1","has_more":false,"raw_marker":"input-items-upstream"}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"input-items-alias-model","display_name":"Input Items Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"input-items-upstream-model","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(openaiProvider.Id)+`","name":"input-items-alias-account","runtime_class":"api_key","credential":{"api_key":"input-items-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/api/provider/openai-compatible/v1/responses/resp_input_items/input_items?model=input-items-alias-model&after=item_1&limit=2&order=asc&include=file_search_call.results&include=reasoning.encrypted_content&before=drop", "")
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream input_items call, got %d", upstreamCalls)
	}
	response := map[string]any{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode raw input_items response: %v", err)
	}
	if response["object"] != "list" || response["raw_marker"] != "input-items-upstream" {
		t.Fatalf("expected raw input_items JSON replay, got %+v", response)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=input-items-alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 {
		t.Fatalf("expected input_items alias scheduler evidence, got %+v", decisionsResp.Data)
	}
	firstDecision := decisionsResp.Data[0]
	secondDecision := decisionsResp.Data[1]
	if firstDecision.AttemptNo != 1 ||
		firstDecision.SelectedAccountId == nil ||
		*firstDecision.SelectedAccountId == string(accountResp.Data.Id) ||
		firstDecision.SourceEndpoint != "/api/provider/openai-compatible/v1/responses/resp_input_items/input_items" {
		t.Fatalf("unexpected first input_items scheduler decision: %+v", firstDecision)
	}
	if secondDecision.AttemptNo != 2 ||
		secondDecision.SelectedAccountId == nil ||
		*secondDecision.SelectedAccountId != string(accountResp.Data.Id) ||
		secondDecision.FallbackFromDecisionId == nil ||
		*secondDecision.FallbackFromDecisionId != string(firstDecision.Id) ||
		secondDecision.SourceEndpoint != "/api/provider/openai-compatible/v1/responses/resp_input_items/input_items" {
		t.Fatalf("unexpected input_items failover scheduler decision: first=%+v second=%+v", firstDecision, secondDecision)
	}
	if got := secondDecision.RejectReasons["account_"+*firstDecision.SelectedAccountId]; got != "fallback_excluded" {
		t.Fatalf("expected input_items failover exclusion evidence, got %+v", secondDecision.RejectReasons)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=input-items-alias-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 2 {
		t.Fatalf("expected zero-usage input_items evidence, got %+v", usageResp.Data)
	}
	firstUsage := usageResp.Data[0]
	secondUsage := usageResp.Data[1]
	if firstUsage.AttemptNo != 1 ||
		firstUsage.Success ||
		firstUsage.ErrorClass == nil ||
		*firstUsage.ErrorClass != "configuration_error" ||
		firstUsage.SourceEndpoint != "/api/provider/openai-compatible/v1/responses/resp_input_items/input_items" {
		t.Fatalf("unexpected first input_items usage attempt: %+v", firstUsage)
	}
	if secondUsage.AttemptNo != 2 ||
		!secondUsage.Success ||
		secondUsage.AccountId == nil ||
		*secondUsage.AccountId != string(accountResp.Data.Id) ||
		secondUsage.SourceEndpoint != "/api/provider/openai-compatible/v1/responses/resp_input_items/input_items" ||
		secondUsage.TotalTokens != 0 ||
		secondUsage.Cost != "0.00000000" {
		t.Fatalf("unexpected successful input_items usage attempt: %+v", secondUsage)
	}
	if firstUsage.RequestId != secondUsage.RequestId {
		t.Fatalf("expected input_items failover attempts to share request id, got %q and %q", firstUsage.RequestId, secondUsage.RequestId)
	}
}

func TestGatewayChatCompletionFailoverRecordsAttemptEvidence(t *testing.T) {
	var (
		mu             sync.Mutex
		primaryCalls   int
		secondaryCalls int
	)
	primaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected primary upstream path %s", r.URL.Path)
		}
		mu.Lock()
		primaryCalls++
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer failover-primary-secret" {
			t.Fatalf("expected primary upstream auth, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"primary unavailable","type":"server_error"}}`))
	}))
	defer primaryUpstream.Close()

	secondaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected secondary upstream path %s", r.URL.Path)
		}
		mu.Lock()
		secondaryCalls++
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer failover-secondary-secret" {
			t.Fatalf("expected secondary upstream auth, got %q", got)
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode secondary upstream request: %v", err)
		}
		if payload.Model != "failover-secondary-upstream" {
			t.Fatalf("expected fallback upstream model, got %q", payload.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"failover ok"}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`))
	}))
	defer secondaryUpstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"failover-attempt-model","display_name":"Failover Attempt Model","status":"active"}`)
	primaryProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"failover-primary-provider","display_name":"Failover Primary","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	secondaryProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"failover-secondary-provider","display_name":"Failover Secondary","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(primaryProvider.Data.Id)+`","upstream_model_name":"failover-primary-upstream","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(secondaryProvider.Data.Id)+`","upstream_model_name":"failover-secondary-upstream","status":"active"}`)
	primaryAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(primaryProvider.Data.Id)+`","name":"failover-primary-account","runtime_class":"api_key","credential":{"api_key":"failover-primary-secret"},"metadata":{"base_url":"`+primaryUpstream.URL+`/v1","health_score":0.99,"latency_p95_ms":50,"quality_score":0.99},"status":"active"}`)
	secondaryAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(secondaryProvider.Data.Id)+`","name":"failover-secondary-account","runtime_class":"api_key","credential":{"api_key":"failover-secondary-secret"},"metadata":{"base_url":"`+secondaryUpstream.URL+`/v1","health_score":0.80,"latency_p95_ms":1000,"quality_score":0.50},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"failover-attempt-model","messages":[{"role":"user","content":"exercise failover"}]}`)
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode failover chat response: %v", err)
	}
	if len(chatResp.Choices) != 1 || decodeChatMessageText(t, chatResp.Choices[0].Message.Content) != "failover ok" {
		t.Fatalf("expected fallback response, got %+v", chatResp)
	}

	mu.Lock()
	gotPrimaryCalls := primaryCalls
	gotSecondaryCalls := secondaryCalls
	mu.Unlock()
	if gotPrimaryCalls != 1 || gotSecondaryCalls != 1 {
		t.Fatalf("expected one call to each upstream, got primary=%d secondary=%d", gotPrimaryCalls, gotSecondaryCalls)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=failover-attempt-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 2 {
		t.Fatalf("expected failed and successful usage attempts, got %+v", usageResp.Data)
	}
	firstUsage := usageResp.Data[0]
	secondUsage := usageResp.Data[1]
	if firstUsage.AttemptNo != 1 || firstUsage.Success || firstUsage.ProviderId == nil || *firstUsage.ProviderId != string(primaryProvider.Data.Id) || firstUsage.AccountId == nil || *firstUsage.AccountId != string(primaryAccount.Data.Id) || firstUsage.ErrorClass == nil {
		t.Fatalf("unexpected first usage attempt: %+v", firstUsage)
	}
	if secondUsage.AttemptNo != 2 || !secondUsage.Success || secondUsage.ProviderId == nil || *secondUsage.ProviderId != string(secondaryProvider.Data.Id) || secondUsage.AccountId == nil || *secondUsage.AccountId != string(secondaryAccount.Data.Id) || secondUsage.TotalTokens != 10 {
		t.Fatalf("unexpected second usage attempt: %+v", secondUsage)
	}
	if firstUsage.RequestId != secondUsage.RequestId {
		t.Fatalf("expected attempts to share request id, got %q and %q", firstUsage.RequestId, secondUsage.RequestId)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?request_id="+string(firstUsage.RequestId), nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 {
		t.Fatalf("expected two scheduler decisions, got %+v", decisionsResp.Data)
	}
	firstDecision := decisionsResp.Data[0]
	secondDecision := decisionsResp.Data[1]
	if firstDecision.AttemptNo != 1 || firstDecision.SelectedAccountId == nil || *firstDecision.SelectedAccountId != string(primaryAccount.Data.Id) || firstDecision.FallbackFromDecisionId != nil {
		t.Fatalf("unexpected first decision: %+v", firstDecision)
	}
	if secondDecision.AttemptNo != 2 || secondDecision.SelectedAccountId == nil || *secondDecision.SelectedAccountId != string(secondaryAccount.Data.Id) || secondDecision.FallbackFromDecisionId == nil || *secondDecision.FallbackFromDecisionId != string(firstDecision.Id) {
		t.Fatalf("unexpected fallback decision: first=%+v second=%+v", firstDecision, secondDecision)
	}
	if got := secondDecision.RejectReasons["account_"+string(primaryAccount.Data.Id)]; got != "fallback_excluded" {
		t.Fatalf("expected fallback exclusion evidence for primary account, got %+v", secondDecision.RejectReasons)
	}
	metrics := metricsBody(t, handler)
	if !strings.Contains(metrics, `srapi_gateway_failover_total{endpoint_family="chat_completions",model="failover-attempt-model",provider_protocol="openai-compatible",result="success"} 1`) {
		t.Fatalf("expected failover metric, got:\n%s", metrics)
	}
}

func TestGatewayChatCompletionPoolModeRetriesSameCandidateBeforeFailover(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		upstreamCalls++
		if upstreamCalls == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"try again","type":"rate_limit"}}`))
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "pool-retry-upstream" {
			t.Fatalf("expected mapped upstream model, got %q", payload.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"retry ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"pool-retry-provider","display_name":"Pool Retry Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"pool-retry-model","display_name":"Pool Retry Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"pool-retry-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"pool-retry-account","runtime_class":"api_key","credential":{"api_key":"pool-retry-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","pool_mode":true,"pool_mode_retry_count":1,"pool_mode_retry_base_delay_ms":0,"pool_mode_retry_max_delay_ms":0},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"pool-retry-model","messages":[{"role":"user","content":"retry"}]}`)
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode retry chat response: %v", err)
	}
	if len(chatResp.Choices) != 1 || decodeChatMessageText(t, chatResp.Choices[0].Message.Content) != "retry ok" {
		t.Fatalf("expected retry response, got %+v", chatResp)
	}
	if upstreamCalls != 2 {
		t.Fatalf("expected same-candidate retry to call upstream twice, got %d", upstreamCalls)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=pool-retry-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one successful usage row for same-candidate retry, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if usage.AttemptNo != 1 || !usage.Success || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 6 {
		t.Fatalf("unexpected retry usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?request_id="+string(usage.RequestId), nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].AttemptNo != 1 || decisionsResp.Data[0].FallbackFromDecisionId != nil {
		t.Fatalf("expected one scheduler decision for same-candidate retry, got %+v", decisionsResp.Data)
	}
}

func TestGatewayChatCompletionStreamFailoverBeforeDownstreamWrite(t *testing.T) {
	var (
		mu             sync.Mutex
		primaryCalls   int
		secondaryCalls int
	)
	primaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected primary upstream path %s", r.URL.Path)
		}
		mu.Lock()
		primaryCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"primary stream unavailable","type":"server_error"}}`))
	}))
	defer primaryUpstream.Close()

	secondaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected secondary upstream path %s", r.URL.Path)
		}
		mu.Lock()
		secondaryCalls++
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer failover-stream-secondary-secret" {
			t.Fatalf("expected secondary upstream auth, got %q", got)
		}
		var payload struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode secondary stream upstream request: %v", err)
		}
		if payload.Model != "failover-stream-secondary-upstream" || !payload.Stream {
			t.Fatalf("unexpected secondary stream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"failover stream\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4,\"total_tokens\":12}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer secondaryUpstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"failover-stream-model","display_name":"Failover Stream Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	primaryProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"failover-stream-primary-provider","display_name":"Failover Stream Primary","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	secondaryProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"failover-stream-secondary-provider","display_name":"Failover Stream Secondary","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(primaryProvider.Data.Id)+`","upstream_model_name":"failover-stream-primary-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(secondaryProvider.Data.Id)+`","upstream_model_name":"failover-stream-secondary-upstream","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	primaryAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(primaryProvider.Data.Id)+`","name":"failover-stream-primary-account","runtime_class":"api_key","credential":{"api_key":"failover-stream-primary-secret"},"metadata":{"base_url":"`+primaryUpstream.URL+`/v1","health_score":0.99,"latency_p95_ms":50,"quality_score":0.99},"status":"active"}`)
	secondaryAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(secondaryProvider.Data.Id)+`","name":"failover-stream-secondary-account","runtime_class":"api_key","credential":{"api_key":"failover-stream-secondary-secret"},"metadata":{"base_url":"`+secondaryUpstream.URL+`/v1","health_score":0.80,"latency_p95_ms":1000,"quality_score":0.50},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"failover-stream-model","stream":true,"messages":[{"role":"user","content":"exercise stream failover"}]}`)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected event stream content type, got %q", got)
	}
	body := rec.Body.String()
	for _, expected := range []string{
		`data: {"choices":[{"delta":{"content":"failover stream"}}]}`,
		`data: {"choices":[{"delta":{"content":" ok"}}]}`,
		`"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}`,
		"data: [DONE]",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected stream body to contain %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, "failover stream ok") {
		t.Fatalf("expected raw upstream chunks, got aggregated synthetic stream: %s", body)
	}
	if strings.Contains(body, "primary stream unavailable") {
		t.Fatalf("primary upstream error leaked into downstream stream: %s", body)
	}

	mu.Lock()
	gotPrimaryCalls := primaryCalls
	gotSecondaryCalls := secondaryCalls
	mu.Unlock()
	if gotPrimaryCalls != 1 || gotSecondaryCalls != 1 {
		t.Fatalf("expected one call to each stream upstream, got primary=%d secondary=%d", gotPrimaryCalls, gotSecondaryCalls)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=failover-stream-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 2 {
		t.Fatalf("expected failed and successful stream usage attempts, got %+v", usageResp.Data)
	}
	firstUsage := usageResp.Data[0]
	secondUsage := usageResp.Data[1]
	if firstUsage.AttemptNo != 1 || firstUsage.Success || firstUsage.ProviderId == nil || *firstUsage.ProviderId != string(primaryProvider.Data.Id) || firstUsage.AccountId == nil || *firstUsage.AccountId != string(primaryAccount.Data.Id) {
		t.Fatalf("unexpected first stream usage attempt: %+v", firstUsage)
	}
	if secondUsage.AttemptNo != 2 || !secondUsage.Success || secondUsage.ProviderId == nil || *secondUsage.ProviderId != string(secondaryProvider.Data.Id) || secondUsage.AccountId == nil || *secondUsage.AccountId != string(secondaryAccount.Data.Id) || secondUsage.TotalTokens != 12 {
		t.Fatalf("unexpected second stream usage attempt: %+v", secondUsage)
	}
	if firstUsage.RequestId != secondUsage.RequestId {
		t.Fatalf("expected stream attempts to share request id, got %q and %q", firstUsage.RequestId, secondUsage.RequestId)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?request_id="+string(firstUsage.RequestId), nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 {
		t.Fatalf("expected two stream scheduler decisions, got %+v", decisionsResp.Data)
	}
	firstDecision := decisionsResp.Data[0]
	secondDecision := decisionsResp.Data[1]
	if firstDecision.AttemptNo != 1 || firstDecision.SelectedAccountId == nil || *firstDecision.SelectedAccountId != string(primaryAccount.Data.Id) || firstDecision.FallbackFromDecisionId != nil {
		t.Fatalf("unexpected first stream decision: %+v", firstDecision)
	}
	if secondDecision.AttemptNo != 2 || secondDecision.SelectedAccountId == nil || *secondDecision.SelectedAccountId != string(secondaryAccount.Data.Id) || secondDecision.FallbackFromDecisionId == nil || *secondDecision.FallbackFromDecisionId != string(firstDecision.Id) {
		t.Fatalf("unexpected stream fallback decision: first=%+v second=%+v", firstDecision, secondDecision)
	}
	if got := secondDecision.RejectReasons["account_"+string(primaryAccount.Data.Id)]; got != "fallback_excluded" {
		t.Fatalf("expected stream fallback exclusion evidence for primary account, got %+v", secondDecision.RejectReasons)
	}
}

func TestGatewayProviderAliasUsesPresetProviderKey(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	deepseekProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"deepseek","display_name":"DeepSeek","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"deepseek-fallback-provider","display_name":"DeepSeek Fallback Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"deepseek-alias-model","display_name":"DeepSeek Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"deepseek-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(deepseekProvider.Data.Id)+`","upstream_model_name":"deepseek-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(deepseekProvider.Data.Id)+`","name":"deepseek-account","runtime_class":"api_key","credential":{"api_key":"deepseek-secret"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/deepseek/v1/chat/completions", `{"model":"deepseek-alias-model","messages":[{"role":"user","content":"alias route"}]}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=deepseek-alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one deepseek alias decision, got %d", len(decisionsResp.Data))
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(deepseekProvider.Data.Id) {
		t.Fatalf("expected deepseek alias to force deepseek provider, got %+v", decision)
	}
	if decision.CandidateCount != 1 {
		t.Fatalf("expected deepseek alias to filter candidates to one provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/deepseek/v1/chat/completions" {
		t.Fatalf("expected deepseek alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestAdminInstallProviderPresetsIsIdempotent(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	first := mustInstallProviderPresets(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	if first.Data.Succeeded == 0 {
		t.Fatalf("expected provider presets to install missing providers, got %+v", first.Data)
	}
	if first.Data.Failed != 0 {
		t.Fatalf("expected provider preset install without failures, got %+v", first.Data)
	}

	providersReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
	providersReq.AddCookie(sessionCookie)
	providersRec := httptest.NewRecorder()
	handler.ServeHTTP(providersRec, providersReq)
	if providersRec.Code != http.StatusOK {
		t.Fatalf("expected provider list 200, got %d body=%s", providersRec.Code, providersRec.Body.String())
	}
	var providersResp apiopenapi.ProviderListResponse
	if err := json.NewDecoder(providersRec.Body).Decode(&providersResp); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	providersByName := map[string]apiopenapi.Provider{}
	for _, provider := range providersResp.Data {
		providersByName[provider.Name] = provider
	}
	for _, name := range []string{"deepseek", "kimi", "qwen", "zhipu", "grok", "mistral", "groq", "together"} {
		provider, ok := providersByName[name]
		if !ok {
			t.Fatalf("expected installed provider preset %s in %+v", name, providersByName)
		}
		if provider.Status != apiopenapi.ResourceStatusDisabled {
			t.Fatalf("expected installed provider %s to default disabled, got %s", name, provider.Status)
		}
	}
	deepseekSchema := providersByName["deepseek"].ConfigSchema
	if deepseekSchema == nil || (*deepseekSchema)["default_base_url"] != "https://api.deepseek.com" {
		t.Fatalf("expected deepseek preset default base url, got %+v", deepseekSchema)
	}
	togetherSchema := providersByName["together"].ConfigSchema
	if togetherSchema == nil || (*togetherSchema)["default_base_url"] != "https://api.together.ai/v1" {
		t.Fatalf("expected together preset default base url, got %+v", togetherSchema)
	}

	for _, preset := range []struct {
		name           string
		defaultBaseURL string
	}{
		{name: "deepseek", defaultBaseURL: "https://api.deepseek.com"},
		{name: "qwen", defaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{name: "together", defaultBaseURL: "https://api.together.ai/v1"},
	} {
		provider := providersByName[preset.name]
		mustUpdateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(provider.Id), `{"status":"active"}`)
		mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(provider.Id)+`","name":"`+preset.name+`-preset-test-account","runtime_class":"api_key","credential":{"api_key":"`+preset.name+`-secret"},"status":"active"}`)

		testResp := mustTestProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(provider.Id))
		if !testResp.Data.Ok || testResp.Data.ProviderId == nil || *testResp.Data.ProviderId != provider.Id {
			t.Fatalf("expected provider preset %s to test ok, got %+v", preset.name, testResp.Data)
		}
		if testResp.Data.Checks == nil {
			t.Fatalf("expected provider preset %s test checks", preset.name)
		}
		checks := *testResp.Data.Checks
		if checks["provider_key"] != preset.name || checks["default_base_url"] != preset.defaultBaseURL || checks["platform_family"] != "openai_compatible" {
			t.Fatalf("unexpected provider preset %s test checks: %+v", preset.name, checks)
		}
	}

	second := mustInstallProviderPresets(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	if second.Data.Succeeded != 0 || second.Data.Failed != 0 {
		t.Fatalf("expected second install to be idempotent, got %+v", second.Data)
	}
	if second.Data.Requested != first.Data.Requested {
		t.Fatalf("expected stable preset count, first=%+v second=%+v", first.Data, second.Data)
	}
}

func TestGatewayAnthropicProviderAliasTargetsMessagesUpstream(t *testing.T) {
	type upstreamCall struct {
		Path      string
		APIKey    string
		Version   string
		Model     string
		System    string
		MaxTokens int
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model     string `json:"model"`
			System    string `json:"system"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:      r.URL.Path,
			APIKey:    r.Header.Get("x-api-key"),
			Version:   r.Header.Get("anthropic-version"),
			Model:     payload.Model,
			System:    payload.System,
			MaxTokens: payload.MaxTokens,
			Messages:  payload.Messages,
		})
		mu.Unlock()
		content := "anthropic alias response"
		if len(payload.Messages) > 0 {
			content = "anthropic echo: " + payload.Messages[len(payload.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":` + strconv.Quote(content) + `}],"usage":{"input_tokens":4,"output_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	anthropicProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"anthropic-compatible","display_name":"Anthropic Compatible","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"anthropic-fallback-provider","display_name":"Anthropic Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"anthropic-alias-model","display_name":"Anthropic Alias Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"anthropic-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(anthropicProvider.Data.Id)+`","upstream_model_name":"claude-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(anthropicProvider.Data.Id)+`","name":"anthropic-alias-account","runtime_class":"api_key","credential":{"api_key":"anthropic-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	messageRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/anthropic-compatible/v1/messages", `{"model":"anthropic-alias-model","system":"respond briefly","max_tokens":32,"messages":[{"role":"user","content":"alias messages"}]}`)
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || !strings.Contains(*messageResp.Content[0].Text, "alias messages") {
		t.Fatalf("expected messages response from Anthropic upstream, got %v", messageResp.Content)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/messages" || call.APIKey != "anthropic-secret" || call.Version == "" || call.Model != "claude-upstream" || call.System != "respond briefly" || call.MaxTokens != 32 {
		t.Fatalf("unexpected Anthropic upstream call: %+v", call)
	}
	if len(call.Messages) != 1 || call.Messages[0].Role != "user" || call.Messages[0].Content != "alias messages" {
		t.Fatalf("unexpected Anthropic upstream messages: %+v", call.Messages)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=anthropic-alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one Anthropic alias decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(anthropicProvider.Data.Id) || decision.TargetProtocol != "anthropic-compatible" || decision.SourceEndpoint != "/api/provider/anthropic-compatible/v1/messages" {
		t.Fatalf("expected Anthropic alias scheduler evidence, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=anthropic-alias-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].TargetProtocol == nil || *usageResp.Data[0].TargetProtocol != "anthropic-compatible" || usageResp.Data[0].TotalTokens != 9 {
		t.Fatalf("expected Anthropic usage evidence, got %+v", usageResp.Data)
	}
}

func TestGatewayClaudeCodeReverseProxyUsesOfficialClientMessagesShape(t *testing.T) {
	type upstreamCall struct {
		Path             string
		RawQuery         string
		Authorization    string
		UserAgent        string
		Version          string
		Beta             string
		App              string
		StainlessRuntime string
		StainlessLang    string
		SessionID        string
		ClientRequestID  string
		Model            string
		System           []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		MaxTokens int
		Message   string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model  string `json:"model"`
			System []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"system"`
			MaxTokens int `json:"max_tokens"`
			Messages  []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		call := upstreamCall{
			Path:             r.URL.Path,
			RawQuery:         r.URL.RawQuery,
			Authorization:    r.Header.Get("Authorization"),
			UserAgent:        r.Header.Get("User-Agent"),
			Version:          r.Header.Get("anthropic-version"),
			Beta:             r.Header.Get("anthropic-beta"),
			App:              r.Header.Get("x-app"),
			StainlessRuntime: r.Header.Get("x-stainless-runtime"),
			StainlessLang:    r.Header.Get("x-stainless-lang"),
			SessionID:        r.Header.Get("x-claude-code-session-id"),
			ClientRequestID:  r.Header.Get("x-client-request-id"),
			Model:            payload.Model,
			System:           payload.System,
			MaxTokens:        payload.MaxTokens,
		}
		if len(payload.Messages) > 0 {
			call.Message = payload.Messages[0].Content
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"claude code 2api ok"}],"usage":{"input_tokens":7,"output_tokens":8}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp420-claude-code","display_name":"WP420 Claude Code","adapter_type":"reverse-proxy-claude-code-cli","protocol":"anthropic-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp420-claude-code-model","display_name":"WP420 Claude Code Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"claude-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp420-claude-code-account","runtime_class":"cli_client_token","upstream_client":"claude_code_cli","credential":{"cli_client_token":"claude-code-token"},"metadata":{"base_url":"`+upstream.URL+`/v1","user_agent":"claude-cli/2.1.63 (external, cli)","claude_code_session_id":"session-gateway-123","claude_client_request_id":"client-req-gateway-123","claude_code_version":"2.1.63","claude_code_build":"abc123"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	messageRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"wp420-claude-code-model","system":"be direct","max_tokens":64,"messages":[{"role":"user","content":"hello claude code"}]}`)
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || *messageResp.Content[0].Text != "claude code 2api ok" {
		t.Fatalf("unexpected Claude Code response: %+v", messageResp.Content)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/messages" || call.RawQuery != "beta=true" || call.Authorization != "Bearer claude-code-token" || call.UserAgent != "claude-cli/2.1.63 (external, cli)" {
		t.Fatalf("unexpected Claude Code upstream route/auth: %+v", call)
	}
	if call.Version != "2023-06-01" ||
		!strings.Contains(call.Beta, "claude-code-20250219") ||
		!strings.Contains(call.Beta, "oauth-2025-04-20") ||
		call.App != "cli" ||
		call.StainlessRuntime != "node" ||
		call.StainlessLang != "js" ||
		call.SessionID != "session-gateway-123" ||
		call.ClientRequestID != "client-req-gateway-123" {
		t.Fatalf("unexpected Claude Code upstream headers: %+v", call)
	}
	if call.Model != "claude-upstream" || call.MaxTokens != 64 || call.Message != "hello claude code" {
		t.Fatalf("unexpected Claude Code upstream payload: %+v", call)
	}
	if len(call.System) < 3 ||
		!strings.HasPrefix(call.System[0].Text, "x-anthropic-billing-header: cc_version=2.1.63.abc123; cc_entrypoint=cli; cch=") ||
		call.System[1].Text != "You are Claude Code, Anthropic's official CLI for Claude." ||
		call.System[2].Text != "be direct" {
		t.Fatalf("unexpected Claude Code system blocks: %+v", call.System)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp420-claude-code-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one Claude Code decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(providerResp.Data.Id) || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(accountResp.Data.Id) || decision.TargetProtocol != "anthropic-compatible" {
		t.Fatalf("expected Claude Code scheduler evidence, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp420-claude-code-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].TargetProtocol == nil || *usageResp.Data[0].TargetProtocol != "anthropic-compatible" || usageResp.Data[0].TotalTokens != 15 {
		t.Fatalf("expected Claude Code usage evidence, got %+v", usageResp.Data)
	}
}

func TestGatewayEmbeddingsRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		Model         string
		Input         []string
		Dimensions    *int
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions *int     `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Input:         append([]string(nil), payload.Input...),
			Dimensions:    payload.Dimensions,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.11,0.22,0.33],"index":0},{"object":"embedding","embedding":[0.44,0.55,0.66],"index":1}],"model":"embedding-upstream","usage":{"prompt_tokens":9,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp270-openai","display_name":"WP270 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"embeddings":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp270-embedding-model","display_name":"WP270 Embedding Model","status":"active","capabilities":[{"key":"embeddings","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"embedding-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp270-embedding-account","runtime_class":"api_key","credential":{"api_key":"embedding-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/embeddings", `{"model":"wp270-embedding-model","input":["first embedding","second embedding"],"dimensions":3}`)
	var embeddingResp apiopenapi.EmbeddingResponse
	if err := json.NewDecoder(rec.Body).Decode(&embeddingResp); err != nil {
		t.Fatalf("decode embeddings response: %v", err)
	}
	if embeddingResp.Object != apiopenapi.EmbeddingResponseObjectList || embeddingResp.Model != "wp270-embedding-model" || len(embeddingResp.Data) != 2 {
		t.Fatalf("unexpected embeddings response: %+v", embeddingResp)
	}
	vector, err := embeddingResp.Data[0].Embedding.AsEmbeddingVector0()
	if err != nil || len(vector) != 3 {
		t.Fatalf("expected float embedding vector, got vector=%v err=%v", vector, err)
	}
	if embeddingResp.Usage.PromptTokens == nil || *embeddingResp.Usage.PromptTokens != 9 || embeddingResp.Usage.CompletionTokens == nil || *embeddingResp.Usage.CompletionTokens != 0 {
		t.Fatalf("expected upstream embedding usage, got %+v", embeddingResp.Usage)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/embeddings" || gotCalls[0].Authorization != "Bearer embedding-secret" || gotCalls[0].Model != "embedding-upstream" {
		t.Fatalf("unexpected upstream call: %+v", gotCalls[0])
	}
	if gotCalls[0].Dimensions == nil || *gotCalls[0].Dimensions != 3 || len(gotCalls[0].Input) != 2 {
		t.Fatalf("unexpected upstream input details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp270-embedding-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/embeddings" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) {
		t.Fatalf("unexpected embedding usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp270-embedding-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/embeddings" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected embedding decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayEmbeddingAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"embedding-fallback-provider","display_name":"Embedding Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"embeddings":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp270-alias-embedding-model","display_name":"WP270 Alias Embedding Model","status":"active","capabilities":[{"key":"embeddings","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-embedding","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"embedding-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-embedding","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/openai-compatible/v1/embeddings", `{"model":"wp270-alias-embedding-model","input":"alias embedding"}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp270-alias-embedding-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias embedding decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected embedding alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/embeddings" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayImageGenerationRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path           string
		Authorization  string
		Model          string
		Prompt         string
		Count          int
		Size           string
		Quality        string
		Style          string
		ResponseFormat string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model          string `json:"model"`
			Prompt         string `json:"prompt"`
			N              int    `json:"n"`
			Size           string `json:"size"`
			Quality        string `json:"quality"`
			Style          string `json:"style"`
			ResponseFormat string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:           r.URL.Path,
			Authorization:  r.Header.Get("Authorization"),
			Model:          payload.Model,
			Prompt:         payload.Prompt,
			Count:          payload.N,
			Size:           payload.Size,
			Quality:        payload.Quality,
			Style:          payload.Style,
			ResponseFormat: payload.ResponseFormat,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000001,"data":[{"url":"https://example.test/wp290-image.png","revised_prompt":"polished image prompt"}],"model":"image-upstream","usage":{"prompt_tokens":12,"completion_tokens":1,"total_tokens":13}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp290-openai","display_name":"WP290 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp290-image-model","display_name":"WP290 Image Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp290-image-account","runtime_class":"api_key","credential":{"api_key":"image-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/images/generations", `{"model":"wp290-image-model","prompt":"make a clean test image","n":1,"size":"1024x1024","quality":"high","style":"vivid","response_format":"url"}`)
	var imageResp apiopenapi.ImageGenerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&imageResp); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if imageResp.Created != 1710000001 || len(imageResp.Data) != 1 || imageResp.Data[0].Url == nil || *imageResp.Data[0].Url != "https://example.test/wp290-image.png" {
		t.Fatalf("unexpected image response: %+v", imageResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/images/generations" || gotCalls[0].Authorization != "Bearer image-secret" || gotCalls[0].Model != "image-upstream" {
		t.Fatalf("unexpected upstream image call: %+v", gotCalls[0])
	}
	if gotCalls[0].Prompt != "make a clean test image" || gotCalls[0].Count != 1 || gotCalls[0].Size != "1024x1024" || gotCalls[0].Quality != "high" || gotCalls[0].Style != "vivid" || gotCalls[0].ResponseFormat != "url" {
		t.Fatalf("unexpected upstream image details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp290-image-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one image usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/images/generations" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 13 {
		t.Fatalf("unexpected image usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp290-image-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/images/generations" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected image decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayImageGenerationPoolModeRetriesThenFailsOver(t *testing.T) {
	var (
		mu             sync.Mutex
		primaryCalls   int
		secondaryCalls int
	)
	primaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected primary upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-primary-secret" {
			t.Fatalf("expected primary upstream auth, got %q", got)
		}
		mu.Lock()
		primaryCalls++
		callNo := primaryCalls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if callNo == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"try again","type":"rate_limit"}}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"primary unavailable","type":"server_error"}}`))
	}))
	defer primaryUpstream.Close()

	secondaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected secondary upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-secondary-secret" {
			t.Fatalf("expected secondary upstream auth, got %q", got)
		}
		var payload struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode secondary upstream request: %v", err)
		}
		if payload.Model != "image-failover-secondary-upstream" || payload.Prompt != "retry then fallback" {
			t.Fatalf("unexpected secondary payload: %+v", payload)
		}
		mu.Lock()
		secondaryCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000002,"data":[{"url":"https://example.test/fallback-image.png"}],"model":"image-failover-secondary-upstream","usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`))
	}))
	defer secondaryUpstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"image-direct-failover-model","display_name":"Image Direct Failover Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	primaryProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"image-direct-primary-provider","display_name":"Image Direct Primary","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	secondaryProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"image-direct-secondary-provider","display_name":"Image Direct Secondary","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(primaryProvider.Data.Id)+`","upstream_model_name":"image-failover-primary-upstream","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(secondaryProvider.Data.Id)+`","upstream_model_name":"image-failover-secondary-upstream","status":"active"}`)
	primaryAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(primaryProvider.Data.Id)+`","name":"image-direct-primary-account","runtime_class":"api_key","credential":{"api_key":"image-primary-secret"},"metadata":{"base_url":"`+primaryUpstream.URL+`/v1","pool_mode":true,"pool_mode_retry_count":1,"pool_mode_retry_base_delay_ms":0,"pool_mode_retry_max_delay_ms":0,"health_score":0.99,"latency_p95_ms":50,"quality_score":0.99},"status":"active"}`)
	secondaryAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(secondaryProvider.Data.Id)+`","name":"image-direct-secondary-account","runtime_class":"api_key","credential":{"api_key":"image-secondary-secret"},"metadata":{"base_url":"`+secondaryUpstream.URL+`/v1","health_score":0.80,"latency_p95_ms":1000,"quality_score":0.50},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/images/generations", `{"model":"image-direct-failover-model","prompt":"retry then fallback","n":1}`)
	var imageResp apiopenapi.ImageGenerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&imageResp); err != nil {
		t.Fatalf("decode image failover response: %v", err)
	}
	if imageResp.Created != 1710000002 || len(imageResp.Data) != 1 || imageResp.Data[0].Url == nil || *imageResp.Data[0].Url != "https://example.test/fallback-image.png" {
		t.Fatalf("unexpected image failover response: %+v", imageResp)
	}

	mu.Lock()
	gotPrimaryCalls := primaryCalls
	gotSecondaryCalls := secondaryCalls
	mu.Unlock()
	if gotPrimaryCalls != 2 || gotSecondaryCalls != 1 {
		t.Fatalf("expected primary retry then fallback, got primary=%d secondary=%d", gotPrimaryCalls, gotSecondaryCalls)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=image-direct-failover-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode image failover usage logs: %v", err)
	}
	if len(usageResp.Data) != 2 {
		t.Fatalf("expected failed and successful image usage attempts, got %+v", usageResp.Data)
	}
	firstUsage := usageResp.Data[0]
	secondUsage := usageResp.Data[1]
	if firstUsage.AttemptNo != 1 || firstUsage.Success || firstUsage.AccountId == nil || *firstUsage.AccountId != string(primaryAccount.Data.Id) || firstUsage.ErrorClass == nil || *firstUsage.ErrorClass != "provider_5xx" {
		t.Fatalf("unexpected first image usage attempt: %+v", firstUsage)
	}
	if secondUsage.AttemptNo != 2 || !secondUsage.Success || secondUsage.AccountId == nil || *secondUsage.AccountId != string(secondaryAccount.Data.Id) || secondUsage.TotalTokens != 7 {
		t.Fatalf("unexpected second image usage attempt: %+v", secondUsage)
	}
	if firstUsage.RequestId != secondUsage.RequestId {
		t.Fatalf("expected image failover attempts to share request id, got %q and %q", firstUsage.RequestId, secondUsage.RequestId)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?request_id="+string(firstUsage.RequestId), nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode image failover decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 {
		t.Fatalf("expected two image scheduler decisions, got %+v", decisionsResp.Data)
	}
	firstDecision := decisionsResp.Data[0]
	secondDecision := decisionsResp.Data[1]
	if firstDecision.AttemptNo != 1 || firstDecision.SelectedAccountId == nil || *firstDecision.SelectedAccountId != string(primaryAccount.Data.Id) {
		t.Fatalf("unexpected first image decision: %+v", firstDecision)
	}
	if secondDecision.AttemptNo != 2 || secondDecision.SelectedAccountId == nil || *secondDecision.SelectedAccountId != string(secondaryAccount.Data.Id) || secondDecision.FallbackFromDecisionId == nil || *secondDecision.FallbackFromDecisionId != string(firstDecision.Id) {
		t.Fatalf("unexpected second image decision: first=%+v second=%+v", firstDecision, secondDecision)
	}
	if got := secondDecision.RejectReasons["account_"+string(primaryAccount.Data.Id)]; got != "fallback_excluded" {
		t.Fatalf("expected fallback exclusion evidence for primary account, got %+v", secondDecision.RejectReasons)
	}
}

func TestGatewayImageGenerationAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"image-fallback-provider","display_name":"Image Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp290-alias-image-model","display_name":"WP290 Alias Image Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-image","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"image-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-image","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/openai-compatible/v1/images/generations", `{"model":"wp290-alias-image-model","prompt":"alias image prompt"}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp290-alias-image-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias image decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected image alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/images/generations" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayImageEditRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path           string
		Authorization  string
		Model          string
		Prompt         string
		Count          string
		Size           string
		Quality        string
		ResponseFormat string
		Filename       string
		ContentType    string
		Image          string
		MaskFilename   string
		Mask           string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		imageFile, imageHeader, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("expected upstream image: %v", err)
		}
		defer imageFile.Close()
		imageBytes, err := io.ReadAll(imageFile)
		if err != nil {
			t.Fatalf("read upstream image: %v", err)
		}
		maskFile, maskHeader, err := r.FormFile("mask")
		if err != nil {
			t.Fatalf("expected upstream mask: %v", err)
		}
		defer maskFile.Close()
		maskBytes, err := io.ReadAll(maskFile)
		if err != nil {
			t.Fatalf("read upstream mask: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:           r.URL.Path,
			Authorization:  r.Header.Get("Authorization"),
			Model:          r.FormValue("model"),
			Prompt:         r.FormValue("prompt"),
			Count:          r.FormValue("n"),
			Size:           r.FormValue("size"),
			Quality:        r.FormValue("quality"),
			ResponseFormat: r.FormValue("response_format"),
			Filename:       imageHeader.Filename,
			ContentType:    imageHeader.Header.Get("Content-Type"),
			Image:          string(imageBytes),
			MaskFilename:   maskHeader.Filename,
			Mask:           string(maskBytes),
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000200,"data":[{"url":"https://example.test/wp480-edit.png","revised_prompt":"edited prompt"}],"model":"image-edit-upstream","usage":{"input_tokens":20,"output_tokens":4,"total_tokens":24}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp480-openai","display_name":"WP480 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp480-image-edit-model","display_name":"WP480 Image Edit Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-edit-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp480-image-edit-account","runtime_class":"api_key","credential":{"api_key":"image-edit-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayImageEditRequest(t, handler, apiKey, "/v1/images/edits", map[string]string{
		"model":           "wp480-image-edit-model",
		"prompt":          "replace the background",
		"n":               "1",
		"size":            "1024x1024",
		"quality":         "high",
		"response_format": "url",
	}, "source.png", "image/png", []byte("PNG-source"), "mask.png", "image/png", []byte("PNG-mask"))
	var imageResp apiopenapi.ImageGenerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&imageResp); err != nil {
		t.Fatalf("decode image edit response: %v", err)
	}
	if imageResp.Created != 1710000200 || len(imageResp.Data) != 1 || imageResp.Data[0].Url == nil || *imageResp.Data[0].Url != "https://example.test/wp480-edit.png" {
		t.Fatalf("unexpected image edit response: %+v", imageResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/images/edits" || gotCalls[0].Authorization != "Bearer image-edit-secret" || gotCalls[0].Model != "image-edit-upstream" {
		t.Fatalf("unexpected upstream image edit call: %+v", gotCalls[0])
	}
	if gotCalls[0].Prompt != "replace the background" || gotCalls[0].Count != "1" || gotCalls[0].Size != "1024x1024" || gotCalls[0].Quality != "high" || gotCalls[0].ResponseFormat != "url" || gotCalls[0].Filename != "source.png" || gotCalls[0].ContentType != "image/png" || gotCalls[0].Image != "PNG-source" || gotCalls[0].MaskFilename != "mask.png" || gotCalls[0].Mask != "PNG-mask" {
		t.Fatalf("unexpected upstream image edit details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp480-image-edit-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one image edit usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/images/edits" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 24 {
		t.Fatalf("unexpected image edit usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp480-image-edit-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/images/edits" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected image edit decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayImageEditAcceptsJSONImageReferences(t *testing.T) {
	type upstreamCall struct {
		Path              string
		Authorization     string
		Model             string
		Prompt            string
		Count             string
		Size              string
		Quality           string
		ResponseFormat    string
		OutputFormat      string
		OutputCompression string
		Background        string
		Filenames         []string
		ContentTypes      []string
		Images            []string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		files := append([]*multipart.FileHeader(nil), r.MultipartForm.File["image"]...)
		files = append(files, r.MultipartForm.File["image[]"]...)
		call := upstreamCall{
			Path:              r.URL.Path,
			Authorization:     r.Header.Get("Authorization"),
			Model:             r.FormValue("model"),
			Prompt:            r.FormValue("prompt"),
			Count:             r.FormValue("n"),
			Size:              r.FormValue("size"),
			Quality:           r.FormValue("quality"),
			ResponseFormat:    r.FormValue("response_format"),
			OutputFormat:      r.FormValue("output_format"),
			OutputCompression: r.FormValue("output_compression"),
			Background:        r.FormValue("background"),
		}
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				t.Fatalf("open upstream image: %v", err)
			}
			imageBytes, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				t.Fatalf("read upstream image: %v", err)
			}
			call.Filenames = append(call.Filenames, header.Filename)
			call.ContentTypes = append(call.ContentTypes, header.Header.Get("Content-Type"))
			call.Images = append(call.Images, string(imageBytes))
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000400,"data":[{"b64_json":"aW1hZ2UtanNvbi1lZGl0"}],"model":"image-edit-json-upstream","usage":{"input_tokens":21,"output_tokens":5,"total_tokens":26}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp510-openai","display_name":"WP510 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp510-image-edit-json-model","display_name":"WP510 Image Edit JSON Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-edit-json-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp510-image-edit-json-account","runtime_class":"api_key","credential":{"api_key":"image-edit-json-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	jsonBody := `{"model":"wp510-image-edit-json-model","prompt":"combine references","n":1,"size":"1024x1536","quality":"high","response_format":"b64_json","output_format":"webp","output_compression":88,"background":"transparent","images":[{"image_url":"data:image/png;base64,UE5HLWpzb24="},{"b64_json":"SlBFRy1qc29u","mime_type":"image/jpeg","filename":"two.jpg"}]}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/images/edits", jsonBody)
	var imageResp apiopenapi.ImageGenerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&imageResp); err != nil {
		t.Fatalf("decode image edit json response: %v", err)
	}
	if imageResp.Created != 1710000400 || len(imageResp.Data) != 1 || imageResp.Data[0].B64Json == nil || *imageResp.Data[0].B64Json == "" {
		t.Fatalf("unexpected image edit json response: %+v", imageResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	got := gotCalls[0]
	if got.Path != "/v1/images/edits" || got.Authorization != "Bearer image-edit-json-secret" || got.Model != "image-edit-json-upstream" || got.Prompt != "combine references" {
		t.Fatalf("unexpected upstream image edit json call: %+v", got)
	}
	if got.Count != "1" || got.Size != "1024x1536" || got.Quality != "high" || got.ResponseFormat != "b64_json" || got.OutputFormat != "webp" || got.OutputCompression != "88" || got.Background != "transparent" {
		t.Fatalf("unexpected upstream image edit json fields: %+v", got)
	}
	if len(got.Images) != 2 || got.Images[0] != "PNG-json" || got.Images[1] != "JPEG-json" || got.Filenames[0] != "image_1.png" || got.Filenames[1] != "two.jpg" || got.ContentTypes[0] != "image/png" || got.ContentTypes[1] != "image/jpeg" {
		t.Fatalf("unexpected upstream image edit json images: %+v", got)
	}
}

func TestGatewayImageEditStreamReturnsSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		if got := r.FormValue("stream"); got != "" {
			t.Fatalf("expected stream to stay local, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000500,"data":[{"b64_json":"c3RyZWFtLWVkaXQ=","revised_prompt":"stream edit"}],"model":"image-edit-stream-upstream","usage":{"input_tokens":22,"output_tokens":6,"total_tokens":28}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp520-openai","display_name":"WP520 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp520-image-edit-stream-model","display_name":"WP520 Image Edit Stream Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-edit-stream-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp520-image-edit-stream-account","runtime_class":"api_key","credential":{"api_key":"image-edit-stream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayImageEditRequest(t, handler, apiKey, "/v1/images/edits", map[string]string{
		"model":          "wp520-image-edit-stream-model",
		"prompt":         "stream the edit",
		"stream":         "true",
		"partial_images": "2",
		"n":              "1",
	}, "source.png", "image/png", []byte("PNG-stream-source"), "", "", nil)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected event stream content type, got %q", got)
	}
	body := rec.Body.String()
	for _, expected := range []string{"data:", "image.generation.result", "stream edit", "data: [DONE]"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected SSE body to contain %q, got %s", expected, body)
		}
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp520-image-edit-stream-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one stream image edit usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/images/edits" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 28 {
		t.Fatalf("unexpected stream image edit usage evidence: %+v", usage)
	}
}

func TestGatewayImageEditJSONStreamReturnsSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		if got := r.FormValue("stream"); got != "" {
			t.Fatalf("expected stream to stay local, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000600,"data":[{"url":"https://example.test/wp520-json-stream.png","revised_prompt":"json stream edit"}],"model":"image-edit-json-stream-upstream","usage":{"input_tokens":23,"output_tokens":7,"total_tokens":30}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp520-json-openai","display_name":"WP520 JSON OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp520-image-edit-json-stream-model","display_name":"WP520 Image Edit JSON Stream Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-edit-json-stream-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp520-image-edit-json-stream-account","runtime_class":"api_key","credential":{"api_key":"image-edit-json-stream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	body := `{"model":"wp520-image-edit-json-stream-model","prompt":"json stream the edit","stream":true,"images":[{"image_url":"data:image/png;base64,UE5HLWpzb24tc3RyZWFt"}]}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/images/edits", body)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected event stream content type, got %q", got)
	}
	if got := rec.Body.String(); !strings.Contains(got, "image.generation.result") || !strings.Contains(got, "json stream edit") || !strings.Contains(got, "data: [DONE]") {
		t.Fatalf("unexpected SSE body: %s", got)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp520-image-edit-json-stream-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one json stream usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/images/edits" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 30 {
		t.Fatalf("unexpected json stream image edit usage evidence: %+v", usage)
	}
}

func TestGatewayImageEditRejectsUnsupportedJSONReferences(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	for name, body := range map[string]string{
		"remote_url": `{"model":"gpt-image-1","prompt":"edit","images":[{"image_url":"https://example.com/image.png"}]}`,
		"file_id":    `{"model":"gpt-image-1","prompt":"edit","images":[{"file_id":"file-abc123"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+apiKey)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if name == "remote_url" && !strings.Contains(rec.Body.String(), "remote image URLs are not supported") {
				t.Fatalf("expected remote URL rejection, got %s", rec.Body.String())
			}
			if name == "file_id" && !strings.Contains(rec.Body.String(), "file_id image references are not supported") {
				t.Fatalf("expected file_id rejection, got %s", rec.Body.String())
			}
		})
	}
}

func TestGatewayImageEditAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"image-edit-fallback-provider","display_name":"Image Edit Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp480-alias-image-edit-model","display_name":"WP480 Alias Image Edit Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-image-edit","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"image-edit-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-image-edit","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayImageEditRequest(t, handler, apiKey, "/api/provider/openai-compatible/v1/images/edits", map[string]string{"model": "wp480-alias-image-edit-model", "prompt": "alias image edit prompt"}, "alias.png", "image/png", []byte("PNG-alias"), "", "", nil)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp480-alias-image-edit-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias image edit decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected image edit alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/images/edits" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayImageVariationRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path           string
		Authorization  string
		Model          string
		Count          string
		Size           string
		ResponseFormat string
		Filename       string
		ContentType    string
		Image          string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		imageFile, imageHeader, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("expected upstream image: %v", err)
		}
		defer imageFile.Close()
		imageBytes, err := io.ReadAll(imageFile)
		if err != nil {
			t.Fatalf("read upstream image: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:           r.URL.Path,
			Authorization:  r.Header.Get("Authorization"),
			Model:          r.FormValue("model"),
			Count:          r.FormValue("n"),
			Size:           r.FormValue("size"),
			ResponseFormat: r.FormValue("response_format"),
			Filename:       imageHeader.Filename,
			ContentType:    imageHeader.Header.Get("Content-Type"),
			Image:          string(imageBytes),
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000300,"data":[{"url":"https://example.test/wp490-variation.png"}],"model":"image-variation-upstream","usage":{"input_tokens":18,"output_tokens":2,"total_tokens":20}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp490-openai","display_name":"WP490 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp490-image-variation-model","display_name":"WP490 Image Variation Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-variation-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp490-image-variation-account","runtime_class":"api_key","credential":{"api_key":"image-variation-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayImageVariationRequest(t, handler, apiKey, "/v1/images/variations", map[string]string{
		"model":           "wp490-image-variation-model",
		"n":               "2",
		"size":            "1024x1024",
		"response_format": "url",
	}, "source.png", "image/png", []byte("PNG-source"))
	var imageResp apiopenapi.ImageGenerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&imageResp); err != nil {
		t.Fatalf("decode image variation response: %v", err)
	}
	if imageResp.Created != 1710000300 || len(imageResp.Data) != 1 || imageResp.Data[0].Url == nil || *imageResp.Data[0].Url != "https://example.test/wp490-variation.png" {
		t.Fatalf("unexpected image variation response: %+v", imageResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/images/variations" || gotCalls[0].Authorization != "Bearer image-variation-secret" || gotCalls[0].Model != "image-variation-upstream" {
		t.Fatalf("unexpected upstream image variation call: %+v", gotCalls[0])
	}
	if gotCalls[0].Count != "2" || gotCalls[0].Size != "1024x1024" || gotCalls[0].ResponseFormat != "url" || gotCalls[0].Filename != "source.png" || gotCalls[0].ContentType != "image/png" || gotCalls[0].Image != "PNG-source" {
		t.Fatalf("unexpected upstream image variation details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp490-image-variation-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one image variation usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/images/variations" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 20 {
		t.Fatalf("unexpected image variation usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp490-image-variation-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/images/variations" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected image variation decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayImageVariationAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"image-variation-fallback-provider","display_name":"Image Variation Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp490-alias-image-variation-model","display_name":"WP490 Alias Image Variation Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-image-variation","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"image-variation-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-image-variation","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayImageVariationRequest(t, handler, apiKey, "/api/provider/openai-compatible/v1/images/variations", map[string]string{"model": "wp490-alias-image-variation-model"}, "alias.png", "image/png", []byte("PNG-alias"))

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp490-alias-image-variation-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias image variation decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected image variation alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/images/variations" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayModerationRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		Model         string
		Input         []string
		User          string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
			User  string   `json:"user"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Input:         append([]string(nil), payload.Input...),
			User:          payload.User,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"modr_wp310","model":"moderation-upstream","results":[{"flagged":false,"categories":{"violence":false,"self-harm":false},"category_scores":{"violence":0.01,"self-harm":0.02},"category_applied_input_types":{"violence":["text"]}}],"usage":{"prompt_tokens":7,"total_tokens":7}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp310-openai","display_name":"WP310 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"moderations":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp310-moderation-model","display_name":"WP310 Moderation Model","status":"active","capabilities":[{"key":"moderations","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"moderation-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp310-moderation-account","runtime_class":"api_key","credential":{"api_key":"moderation-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/moderations", `{"model":"wp310-moderation-model","input":["first safe input","second safe input"],"user":"user-123"}`)
	var moderationResp apiopenapi.ModerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&moderationResp); err != nil {
		t.Fatalf("decode moderation response: %v", err)
	}
	if moderationResp.Id != "modr_wp310" || moderationResp.Model != "wp310-moderation-model" || len(moderationResp.Results) != 1 || moderationResp.Results[0].Flagged || moderationResp.Results[0].Categories["violence"] {
		t.Fatalf("unexpected moderation response: %+v", moderationResp)
	}
	if moderationResp.Results[0].CategoryScores["self-harm"] <= 0 || moderationResp.Results[0].CategoryAppliedInputTypes == nil || len((*moderationResp.Results[0].CategoryAppliedInputTypes)["violence"]) != 1 {
		t.Fatalf("expected moderation category details, got %+v", moderationResp.Results[0])
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/moderations" || gotCalls[0].Authorization != "Bearer moderation-secret" || gotCalls[0].Model != "moderation-upstream" {
		t.Fatalf("unexpected upstream moderation call: %+v", gotCalls[0])
	}
	if len(gotCalls[0].Input) != 2 || gotCalls[0].Input[0] != "first safe input" || gotCalls[0].User != "user-123" {
		t.Fatalf("unexpected upstream moderation details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp310-moderation-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one moderation usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/moderations" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 7 {
		t.Fatalf("unexpected moderation usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp310-moderation-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/moderations" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected moderation decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayModerationAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"moderation-fallback-provider","display_name":"Moderation Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"moderations":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp310-alias-moderation-model","display_name":"WP310 Alias Moderation Model","status":"active","capabilities":[{"key":"moderations","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-moderation","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"moderation-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-moderation","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/openai-compatible/v1/moderations", `{"model":"wp310-alias-moderation-model","input":"alias moderation input"}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp310-alias-moderation-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias moderation decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected moderation alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/moderations" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayRerankRouteTargetsRerankCompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path            string
		Authorization   string
		Model           string
		Query           string
		Documents       []any
		TopN            *int
		ReturnDocuments bool
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model           string `json:"model"`
			Query           string `json:"query"`
			Documents       []any  `json:"documents"`
			TopN            *int   `json:"top_n"`
			ReturnDocuments bool   `json:"return_documents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:            r.URL.Path,
			Authorization:   r.Header.Get("Authorization"),
			Model:           payload.Model,
			Query:           payload.Query,
			Documents:       append([]any(nil), payload.Documents...),
			TopN:            payload.TopN,
			ReturnDocuments: payload.ReturnDocuments,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"rerank_wp320","model":"rerank-upstream","results":[{"index":1,"relevance_score":0.93,"document":{"text":"SRapi is a self-hosted AI API gateway.","source":"docs"}}],"usage":{"prompt_tokens":11,"total_tokens":11}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp320-rerank","display_name":"WP320 Rerank","adapter_type":"rerank-compatible","protocol":"rerank-compatible","status":"active","capabilities":{"rerank":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp320-rerank-model","display_name":"WP320 Rerank Model","status":"active","capabilities":[{"key":"rerank","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"rerank-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp320-rerank-account","runtime_class":"api_key","credential":{"api_key":"rerank-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/rerank", `{"model":"wp320-rerank-model","query":"what is srapi","documents":["Payment processors settle card orders.",{"text":"SRapi is a self-hosted AI API gateway.","source":"docs"}],"top_n":1,"return_documents":true}`)
	var rerankResp apiopenapi.RerankResponse
	if err := json.NewDecoder(rec.Body).Decode(&rerankResp); err != nil {
		t.Fatalf("decode rerank response: %v", err)
	}
	if rerankResp.Id != "rerank_wp320" || rerankResp.Model != "wp320-rerank-model" || len(rerankResp.Results) != 1 || rerankResp.Results[0].Index != 1 || rerankResp.Results[0].RelevanceScore <= 0.9 || rerankResp.Results[0].Document == nil {
		t.Fatalf("unexpected rerank response: %+v", rerankResp)
	}
	if (*rerankResp.Results[0].Document)["source"] != "docs" || rerankResp.Usage == nil || rerankResp.Usage.PromptTokens == nil || *rerankResp.Usage.PromptTokens != 11 {
		t.Fatalf("expected rerank document and usage details, got %+v", rerankResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/rerank" || gotCalls[0].Authorization != "Bearer rerank-secret" || gotCalls[0].Model != "rerank-upstream" || gotCalls[0].Query != "what is srapi" {
		t.Fatalf("unexpected upstream rerank call: %+v", gotCalls[0])
	}
	if gotCalls[0].TopN == nil || *gotCalls[0].TopN != 1 || !gotCalls[0].ReturnDocuments || len(gotCalls[0].Documents) != 2 {
		t.Fatalf("unexpected upstream rerank details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp320-rerank-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one rerank usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/rerank" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 11 {
		t.Fatalf("unexpected rerank usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp320-rerank-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/rerank" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected rerank decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayAudioTranscriptionRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path           string
		Authorization  string
		Model          string
		Language       string
		Prompt         string
		ResponseFormat string
		Temperature    string
		User           string
		Filename       string
		ContentType    string
		Audio          string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse upstream multipart: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected upstream file: %v", err)
		}
		defer file.Close()
		audio, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upstream file: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:           r.URL.Path,
			Authorization:  r.Header.Get("Authorization"),
			Model:          r.FormValue("model"),
			Language:       r.FormValue("language"),
			Prompt:         r.FormValue("prompt"),
			ResponseFormat: r.FormValue("response_format"),
			Temperature:    r.FormValue("temperature"),
			User:           r.FormValue("user"),
			Filename:       header.Filename,
			ContentType:    header.Header.Get("Content-Type"),
			Audio:          string(audio),
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"gateway transcribed audio","task":"transcribe","language":"en","duration":2.5,"segments":[{"id":0,"start":0,"end":2.5,"text":"gateway transcribed audio","tokens":[1,2,3]}],"usage":{"prompt_tokens":14,"total_tokens":14}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp330-openai","display_name":"WP330 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"audio_transcriptions":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp330-audio-model","display_name":"WP330 Audio Model","status":"active","capabilities":[{"key":"audio_transcriptions","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"audio-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp330-audio-account","runtime_class":"api_key","credential":{"api_key":"audio-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayMultipartRequest(t, handler, apiKey, "/v1/audio/transcriptions", map[string]string{
		"model":           "wp330-audio-model",
		"language":        "en",
		"prompt":          "meeting notes",
		"response_format": "verbose_json",
		"temperature":     "0.2",
		"user":            "user-123",
	}, "sample.wav", "audio/wav", []byte("RIFF-gateway-audio"))
	var audioResp apiopenapi.AudioTranscriptionResponse
	if err := json.NewDecoder(rec.Body).Decode(&audioResp); err != nil {
		t.Fatalf("decode audio transcription response: %v", err)
	}
	if audioResp.Text != "gateway transcribed audio" || audioResp.Language == nil || *audioResp.Language != "en" || audioResp.Duration == nil || *audioResp.Duration != 2.5 || audioResp.Segments == nil || len(*audioResp.Segments) != 1 {
		t.Fatalf("unexpected audio transcription response: %+v", audioResp)
	}
	if audioResp.Usage == nil || audioResp.Usage.PromptTokens == nil || *audioResp.Usage.PromptTokens != 14 {
		t.Fatalf("expected audio usage details, got %+v", audioResp.Usage)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/audio/transcriptions" || gotCalls[0].Authorization != "Bearer audio-secret" || gotCalls[0].Model != "audio-upstream" || gotCalls[0].Filename != "sample.wav" || gotCalls[0].ContentType != "audio/wav" || gotCalls[0].Audio != "RIFF-gateway-audio" {
		t.Fatalf("unexpected upstream audio call: %+v", gotCalls[0])
	}
	if gotCalls[0].Language != "en" || gotCalls[0].Prompt != "meeting notes" || gotCalls[0].ResponseFormat != "verbose_json" || gotCalls[0].Temperature != "0.2" || gotCalls[0].User != "user-123" {
		t.Fatalf("unexpected upstream audio details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp330-audio-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one audio usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/audio/transcriptions" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 14 {
		t.Fatalf("unexpected audio usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp330-audio-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/audio/transcriptions" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected audio decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayAudioTranscriptionAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"audio-fallback-provider","display_name":"Audio Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"audio_transcriptions":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp330-alias-audio-model","display_name":"WP330 Alias Audio Model","status":"active","capabilities":[{"key":"audio_transcriptions","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-audio","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"audio-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-audio","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayMultipartRequest(t, handler, apiKey, "/api/provider/openai-compatible/v1/audio/transcriptions", map[string]string{"model": "wp330-alias-audio-model", "response_format": "json"}, "alias.wav", "audio/wav", []byte("RIFF-alias-audio"))

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp330-alias-audio-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias audio decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected audio alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/audio/transcriptions" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayAudioTranscriptionTextResponseFormatReturnsPlainText(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp330-text-audio-model","display_name":"WP330 Text Audio Model","status":"active","capabilities":[{"key":"audio_transcriptions","level":"required","status":"stable","version":"v1"}]}`)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-audio","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayMultipartRequest(t, handler, apiKey, "/v1/audio/transcriptions", map[string]string{"model": "wp330-text-audio-model", "response_format": "text"}, "plain.wav", "audio/wav", []byte("RIFF-plain-audio"))
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", contentType)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "SRapi local transcription for plain.wav" {
		t.Fatalf("unexpected plain text transcription body %q", body)
	}
}

func TestGatewayAudioSpeechRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path           string
		Authorization  string
		Model          string
		Input          string
		Voice          string
		ResponseFormat string
		Speed          *float32
		Instructions   string
		User           string
		Accent         string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model          string   `json:"model"`
			Input          string   `json:"input"`
			Voice          string   `json:"voice"`
			ResponseFormat string   `json:"response_format"`
			Speed          *float32 `json:"speed"`
			Instructions   string   `json:"instructions"`
			User           string   `json:"user"`
			Accent         string   `json:"accent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream speech request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:           r.URL.Path,
			Authorization:  r.Header.Get("Authorization"),
			Model:          payload.Model,
			Input:          payload.Input,
			Voice:          payload.Voice,
			ResponseFormat: payload.ResponseFormat,
			Speed:          payload.Speed,
			Instructions:   payload.Instructions,
			User:           payload.User,
			Accent:         payload.Accent,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF-gateway-speech"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp340-openai","display_name":"WP340 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"audio_speech":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp340-speech-model","display_name":"WP340 Speech Model","status":"active","capabilities":[{"key":"audio_speech","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"speech-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp340-speech-account","runtime_class":"api_key","credential":{"api_key":"speech-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/audio/speech", `{"model":"wp340-speech-model","input":"speak gateway audio","voice":"alloy","response_format":"wav","speed":1.2,"instructions":"warm voice","user":"user-123","accent":"neutral"}`)
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "audio/wav") {
		t.Fatalf("expected audio/wav content type, got %q", contentType)
	}
	if body := rec.Body.String(); body != "RIFF-gateway-speech" {
		t.Fatalf("unexpected audio speech body %q", body)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream speech call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/audio/speech" || gotCalls[0].Authorization != "Bearer speech-secret" || gotCalls[0].Model != "speech-upstream" || gotCalls[0].Input != "speak gateway audio" || gotCalls[0].Voice != "alloy" || gotCalls[0].ResponseFormat != "wav" {
		t.Fatalf("unexpected upstream speech call: %+v", gotCalls[0])
	}
	if gotCalls[0].Speed == nil || *gotCalls[0].Speed < 1.19 || *gotCalls[0].Speed > 1.21 || gotCalls[0].Instructions != "warm voice" || gotCalls[0].User != "user-123" || gotCalls[0].Accent != "neutral" {
		t.Fatalf("unexpected upstream speech details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp340-speech-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one audio speech usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/audio/speech" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens <= 0 {
		t.Fatalf("unexpected audio speech usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp340-speech-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/audio/speech" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected audio speech decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayAudioSpeechAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"speech-fallback-provider","display_name":"Speech Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"audio_speech":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp340-alias-speech-model","display_name":"WP340 Alias Speech Model","status":"active","capabilities":[{"key":"audio_speech","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-speech","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"speech-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-speech","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/openai-compatible/v1/audio/speech", `{"model":"wp340-alias-speech-model","input":"alias speech","voice":"alloy","response_format":"mp3"}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp340-alias-speech-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias audio speech decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected audio speech alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/audio/speech" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayRerankAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	rerankProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"rerank-compatible","display_name":"Rerank Compatible","adapter_type":"rerank-compatible","protocol":"rerank-compatible","status":"active","capabilities":{"rerank":true}}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"rerank-fallback-provider","display_name":"Rerank Fallback","adapter_type":"rerank-compatible","protocol":"rerank-compatible","status":"active","capabilities":{"rerank":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp320-alias-rerank-model","display_name":"WP320 Alias Rerank Model","status":"active","capabilities":[{"key":"rerank","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-rerank","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"rerank-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(rerankProvider.Data.Id)+`","upstream_model_name":"alias-rerank","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(rerankProvider.Data.Id)+`","name":"rerank-alias-account","runtime_class":"api_key","credential":{"api_key":"alias-secret"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/rerank-compatible/v1/rerank", `{"model":"wp320-alias-rerank-model","query":"alias rerank","documents":["first document","second document"],"top_n":1}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp320-alias-rerank-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias rerank decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(rerankProvider.Data.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected rerank alias to force rerank-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/rerank-compatible/v1/rerank" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayModelAliasAndSessionAffinityFeedScheduler(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp140-provider","display_name":"WP140 Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp140-canonical","display_name":"WP140 Canonical","status":"active"}`)
	createAliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/aliases", strings.NewReader(`{"alias":"wp140-cli-alias","fallback_models":["wp140-fallback"],"strategy_hint":"cost_saver","status":"active"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasReq.AddCookie(sessionCookie)
	createAliasReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAliasRec := httptest.NewRecorder()
	handler.ServeHTTP(createAliasRec, createAliasReq)
	if createAliasRec.Code != http.StatusCreated {
		t.Fatalf("expected model alias create 201, got %d body=%s", createAliasRec.Code, createAliasRec.Body.String())
	}

	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"wp140-upstream","status":"active"}`)
	firstAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp140-first-account","runtime_class":"api_key","credential":{"api_key":"first-secret"},"metadata":{"health_score":0.9},"status":"active"}`)
	stickyAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp140-sticky-account","runtime_class":"api_key","credential":{"api_key":"sticky-secret"},"metadata":{"health_score":0.9,"session_affinity_key":"thread-wp140"},"status":"active"}`)

	keyReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"gateway-alias","scopes":["gateway:invoke"],"allowed_models":["wp140-cli-alias"]}`))
	keyReq.Header.Set("Content-Type", "application/json")
	keyReq.AddCookie(sessionCookie)
	keyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	keyRec := httptest.NewRecorder()
	handler.ServeHTTP(keyRec, keyReq)
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("expected api key create 201, got %d body=%s", keyRec.Code, keyRec.Body.String())
	}
	var keyResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(keyRec.Body).Decode(&keyResp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"wp140-cli-alias","messages":[{"role":"user","content":"route with affinity"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	chatReq.Header.Set("X-SRapi-Session-Affinity-Key", "thread-wp140")
	chatReq.Header.Set("X-SRapi-Sticky-Strength", "hard")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected alias affinity chat 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp140-canonical", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one wp140 decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.Strategy != apiopenapi.SchedulerDecisionStrategyCostSaver {
		t.Fatalf("expected alias strategy cost_saver, got %+v", decision)
	}
	if decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(stickyAccount.Data.Id) {
		t.Fatalf("expected sticky account %s selected, got %+v", stickyAccount.Data.Id, decision.SelectedAccountId)
	}
	if !decision.StickyHit || !jsonObjectContainsString(decision.RejectReasons, "hard_sticky_mismatch") {
		t.Fatalf("expected hard sticky evidence, got decision=%+v first=%+v", decision, firstAccount.Data.Id)
	}
	hints, ok := decision.Scores["routing_hints"].(map[string]any)
	if !ok {
		t.Fatalf("expected routing hints, got %+v", decision.Scores)
	}
	if hints["model_alias"] != "wp140-cli-alias" || hints["sticky_strength"] != "hard" || hints["session_affinity_source"] != "header:x-srapi-session-affinity-key" {
		t.Fatalf("unexpected routing hints: %+v", hints)
	}
	if keyHash, ok := hints["session_affinity_key_hash"].(string); !ok || !strings.HasPrefix(keyHash, "sha256:") || strings.Contains(keyHash, "thread-wp140") {
		t.Fatalf("expected hashed affinity key, got %+v", hints)
	}
}

func TestGatewayAdminSettingsApplySchedulerStrategyRollout(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"rollout response"}}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	healthyProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"rollout-healthy-provider","display_name":"Rollout Healthy Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	cheapProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"rollout-cheap-provider","display_name":"Rollout Cheap Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"rollout-model","display_name":"Rollout Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(healthyProvider.Data.Id)+`","upstream_model_name":"rollout-healthy-upstream","status":"active","pricing_override":{"relative_cost":"0.9"}}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(cheapProvider.Data.Id)+`","upstream_model_name":"rollout-cheap-upstream","status":"active","pricing_override":{"relative_cost":"0.1"}}`)
	healthyAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(healthyProvider.Data.Id)+`","name":"rollout-healthy","runtime_class":"api_key","credential":{"api_key":"healthy-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.95},"status":"active"}`)
	cheapAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(cheapProvider.Data.Id)+`","name":"rollout-cheap","runtime_class":"api_key","credential":{"api_key":"cheap-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.60,"quality_score":0.6},"status":"active"}`)

	keyResp, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	settingsBody := `{"general":{"site_name":"SRapi","logo_url":"","version_label":"","custom_menus":[]},"agreement":{"user_agreement":"","privacy_policy":""},"features":{"enabled_channels":[],"channel_monitoring_enabled":true,"invitation_rebate_enabled":false,"payments_enabled":false},"security":{"admin_api_key":{"configured":false},"registration_enabled":true,"oauth_enabled":false,"oauth_providers":[]},"users":{"default_balance":"0","default_group":"default","user_self_delete_enabled":false,"rpm_limit_default":0},"gateway":{"overload_cooldown_seconds":30,"rate_limit_cooldown_seconds":30,"stream_timeout_seconds":600,"request_shaper_enabled":true,"beta_strategy":"allow_configured","scheduler_strategy_rollout_enabled":true,"scheduler_strategy_shadow_strategy":"cost_saver","scheduler_strategy_rollout_percent":100,"scheduler_strategy_rollout_models":["rollout-model"],"scheduler_strategy_rollout_api_key_hashes":[]},"payment":{"enabled":false,"providers":[],"subscription_plans_enabled":false},"email":{"smtp_configured":false,"templates":{}},"backup":{"enabled":false,"retention_days":30}}`
	settingsReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", strings.NewReader(settingsBody))
	settingsReq.Header.Set("Content-Type", "application/json")
	settingsReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	settingsReq.AddCookie(sessionCookie)
	settingsRec := httptest.NewRecorder()
	handler.ServeHTTP(settingsRec, settingsReq)
	if settingsRec.Code != http.StatusOK {
		t.Fatalf("expected settings update 200, got %d body=%s", settingsRec.Code, settingsRec.Body.String())
	}

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"rollout-model","messages":[{"role":"user","content":"rollout request"}]}`)
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=rollout-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one rollout decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.Strategy != apiopenapi.SchedulerDecisionStrategyCostSaver || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(cheapAccount.Data.Id) {
		t.Fatalf("expected rollout cost_saver to select cheap account %s, got %+v healthy=%s", cheapAccount.Data.Id, decision, healthyAccount.Data.Id)
	}
	if !stringSliceContains(decision.CompatibilityWarnings, "strategy_rollout_shadow_selected") {
		t.Fatalf("expected rollout warning evidence, got %+v", decision.CompatibilityWarnings)
	}
	hints, ok := decision.Scores["routing_hints"].(map[string]any)
	if !ok {
		t.Fatalf("expected routing hints, got %+v", decision.Scores)
	}
	rollout, ok := hints["strategy_rollout"].(map[string]any)
	if !ok || rollout["shadow_strategy"] != "cost_saver" || rollout["shadow_selected"] != true {
		t.Fatalf("expected rollout hints, got %+v", hints)
	}
	hash, ok := rollout["rollout_key_hash"].(string)
	if !ok || !strings.HasPrefix(hash, "sha256:") || strings.Contains(hash, keyResp.Data.PlaintextKey) {
		t.Fatalf("expected hashed rollout key only, got %+v", rollout)
	}
}

func TestGatewayRateLimitFeedbackAppliesAccountCooldown(t *testing.T) {
	var upstreamCalls int
	resetAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"usage_limit_reached","message":"slow down","resets_at":` + strconv.FormatInt(resetAt.Unix(), 10) + `}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"rate-limit-provider","display_name":"Rate Limit Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.AddCookie(sessionCookie)
	providerReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", providerRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	modelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"rate-limit-model","display_name":"Rate Limit Model","status":"active"}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.AddCookie(sessionCookie)
	modelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", modelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	mappingBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"rate-limit-upstream","status":"active"}`
	mappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(mappingBody))
	mappingReq.Header.Set("Content-Type", "application/json")
	mappingReq.AddCookie(sessionCookie)
	mappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	mappingRec := httptest.NewRecorder()
	handler.ServeHTTP(mappingRec, mappingReq)
	if mappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", mappingRec.Code)
	}

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"rate-limit-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1"},"status":"active"}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")
	accountReq.AddCookie(sessionCookie)
	accountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", accountRec.Code)
	}
	var accountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(accountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"rate-limit-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected first rate limit 429, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	cooldownAccount := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if cooldownAccount == nil || cooldownAccount.Metadata == nil || (*cooldownAccount.Metadata)["cooldown_active"] != true || (*cooldownAccount.Metadata)["cooldown_reason"] != "rate_limit" {
		t.Fatalf("expected account cooldown metadata, got %+v", cooldownAccount)
	}
	cooldownUntil, err := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint((*cooldownAccount.Metadata)["cooldown_until"])))
	if err != nil {
		t.Fatalf("expected RFC3339 cooldown_until, got %+v", (*cooldownAccount.Metadata)["cooldown_until"])
	}
	if !cooldownUntil.Equal(resetAt) {
		t.Fatalf("expected cooldown_until from upstream reset %v, got %v", resetAt, cooldownUntil)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/health", nil)
	healthReq.AddCookie(sessionCookie)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected account health 200, got %d body=%s", healthRec.Code, healthRec.Body.String())
	}
	var healthResp apiopenapi.AccountHealthResponse
	if err := json.NewDecoder(healthRec.Body).Decode(&healthResp); err != nil {
		t.Fatalf("decode account health: %v", err)
	}
	if healthResp.Data.RuntimeClass != apiopenapi.RuntimeClassApiKey ||
		healthResp.Data.ErrorClass == nil ||
		*healthResp.Data.ErrorClass != "rate_limit" ||
		healthResp.Data.CooldownReason == nil ||
		*healthResp.Data.CooldownReason != "rate_limit" ||
		healthResp.Data.CooldownUntil == nil ||
		healthResp.Data.QuotaExhausted ||
		healthResp.Data.QuotaRemainingRatio != 1 {
		t.Fatalf("unexpected account health diagnostics: %+v", healthResp.Data)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"rate-limit-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second request blocked by cooldown 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected cooldown to prevent second upstream call, got %d upstream calls", upstreamCalls)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=rate-limit-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 || !jsonObjectContainsString(decisionsResp.Data[1].RejectReasons, "cooldown_active") {
		t.Fatalf("expected second decision cooldown_active, got %+v", decisionsResp.Data)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=rate-limit-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 2 || usageResp.Data[0].ErrorClass == nil || *usageResp.Data[0].ErrorClass != "rate_limit" {
		t.Fatalf("expected rate_limit usage followed by cooldown failure, got %+v", usageResp.Data)
	}
}

func TestGatewayOverloadedFeedbackAppliesAccountCooldown(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"error":{"type":"overloaded_error","message":"overloaded"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"overload-provider","display_name":"Overload Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"overload-model","display_name":"Overload Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, modelResp.Data.Id, `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"overload-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"overload-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"overload-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected first overloaded response 503, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	cooldownAccount := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if cooldownAccount == nil || cooldownAccount.Metadata == nil || (*cooldownAccount.Metadata)["cooldown_active"] != true || (*cooldownAccount.Metadata)["cooldown_reason"] != "overloaded" || (*cooldownAccount.Metadata)["last_error_class"] != "overloaded" {
		t.Fatalf("expected overloaded account cooldown metadata, got %+v", cooldownAccount)
	}
	cooldownUntil, err := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint((*cooldownAccount.Metadata)["cooldown_until"])))
	if err != nil {
		t.Fatalf("expected RFC3339 cooldown_until, got %+v", (*cooldownAccount.Metadata)["cooldown_until"])
	}
	if cooldownUntil.Before(time.Now().UTC().Add(9 * time.Minute)) {
		t.Fatalf("expected overloaded cooldown near 10 minutes, got %v", cooldownUntil)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"overload-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second request blocked by overload cooldown 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected overloaded cooldown to prevent second upstream call, got %d upstream calls", upstreamCalls)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=overload-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 || !jsonObjectContainsString(decisionsResp.Data[1].RejectReasons, "cooldown_active") {
		t.Fatalf("expected second decision cooldown_active, got %+v", decisionsResp.Data)
	}
}

func TestGatewayAuthFailureFeedbackAppliesAccountCooldown(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"type":"permission_error","message":"account forbidden"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"auth-fail-provider","display_name":"Auth Fail Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"auth-fail-model","display_name":"Auth Fail Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, modelResp.Data.Id, `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"auth-fail-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"auth-fail-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"auth-fail-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway {
		t.Fatalf("expected first auth failure response 502, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	cooldownAccount := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if cooldownAccount == nil || cooldownAccount.Metadata == nil || (*cooldownAccount.Metadata)["cooldown_active"] != true || (*cooldownAccount.Metadata)["cooldown_reason"] != "auth_failed" || (*cooldownAccount.Metadata)["last_error_class"] != "auth_failed" || cooldownAccount.Status != apiopenapi.ProviderAccountStatusActive {
		t.Fatalf("expected active account with auth_failed cooldown metadata, got %+v", cooldownAccount)
	}
	cooldownUntil, err := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint((*cooldownAccount.Metadata)["cooldown_until"])))
	if err != nil {
		t.Fatalf("expected RFC3339 cooldown_until, got %+v", (*cooldownAccount.Metadata)["cooldown_until"])
	}
	if cooldownUntil.Before(time.Now().UTC().Add(9 * time.Minute)) {
		t.Fatalf("expected auth failure cooldown near 10 minutes, got %v", cooldownUntil)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"auth-fail-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second request blocked by auth cooldown 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected auth cooldown to prevent second upstream call, got %d upstream calls", upstreamCalls)
	}
}

func TestGatewayConfiguredErrorCooldownRuleAppliesAccountCooldown(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"type":"capacity_unavailable","message":"capacity unavailable"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"configured-cooldown-provider","display_name":"Configured Cooldown Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"configured-cooldown-model","display_name":"Configured Cooldown Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, modelResp.Data.Id, `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"configured-cooldown-upstream","status":"active"}`)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"configured-cooldown-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1","error_cooldown_rules":[{"status_code":503,"error_class":"provider_5xx","keywords":["capacity"],"cooldown_seconds":120,"reason":"provider_capacity"}]},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"configured-cooldown-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway {
		t.Fatalf("expected first configured cooldown response 502, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	cooldownAccount := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if cooldownAccount == nil || cooldownAccount.Metadata == nil || (*cooldownAccount.Metadata)["cooldown_active"] != true || (*cooldownAccount.Metadata)["cooldown_reason"] != "provider_capacity" || (*cooldownAccount.Metadata)["last_error_class"] != "provider_5xx" {
		t.Fatalf("expected configured account cooldown metadata, got %+v", cooldownAccount)
	}
	cooldownUntil, err := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint((*cooldownAccount.Metadata)["cooldown_until"])))
	if err != nil {
		t.Fatalf("expected RFC3339 cooldown_until, got %+v", (*cooldownAccount.Metadata)["cooldown_until"])
	}
	if cooldownUntil.Before(time.Now().UTC().Add(110 * time.Second)) {
		t.Fatalf("expected configured cooldown near 120 seconds, got %v", cooldownUntil)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"configured-cooldown-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second request blocked by configured cooldown 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected configured cooldown to prevent second upstream call, got %d upstream calls", upstreamCalls)
	}
}

func TestGatewayHandledErrorStatusCodesSkipConfiguredCooldown(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"type":"capacity_unavailable","message":"capacity unavailable"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"handled-error-provider","display_name":"Handled Error Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"handled-error-model","display_name":"Handled Error Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, modelResp.Data.Id, `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"handled-error-upstream","status":"active"}`)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"handled-error-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1","handled_error_status_codes":[429],"error_cooldown_rules":[{"status_code":503,"error_class":"provider_5xx","keywords":["capacity"],"cooldown_seconds":120,"reason":"provider_capacity"}]},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"handled-error-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway {
		t.Fatalf("expected first configured cooldown response 502, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	account := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if account == nil || account.Metadata == nil {
		t.Fatalf("expected account metadata, got %+v", account)
	}
	if (*account.Metadata)["cooldown_active"] == true || (*account.Metadata)["cooldown_reason"] != nil {
		t.Fatalf("expected handled status gate to skip cooldown metadata, got %+v", *account.Metadata)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"handled-error-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusBadGateway {
		t.Fatalf("expected second configured cooldown response 502, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 2 {
		t.Fatalf("expected skipped cooldown to allow second upstream call, got %d upstream calls", upstreamCalls)
	}
}

func TestGatewayPoolModeSkipsAccountCooldownWithoutCustomCodes(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit","message":"rate limited"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"pool-mode-provider","display_name":"Pool Mode Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"pool-mode-model","display_name":"Pool Mode Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, modelResp.Data.Id, `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"pool-mode-upstream","status":"active"}`)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"pool-mode-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1","pool_mode":true,"pool_mode_retry_count":0},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"pool-mode-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected first pool-mode response 429, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	account := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if account == nil || account.Metadata == nil {
		t.Fatalf("expected account metadata, got %+v", account)
	}
	if (*account.Metadata)["cooldown_active"] == true || (*account.Metadata)["cooldown_reason"] != nil {
		t.Fatalf("expected pool mode to skip cooldown metadata, got %+v", *account.Metadata)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"pool-mode-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second pool-mode response 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 2 {
		t.Fatalf("expected skipped pool-mode cooldown to allow second upstream call, got %d upstream calls", upstreamCalls)
	}
}

func TestCreateAccountNormalizesLegacyErrorPolicyCredentialMetadata(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"legacy-policy-provider","display_name":"Legacy Policy Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)

	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"legacy-policy-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret","pool_mode":true,"custom_error_codes_enabled":true,"custom_error_codes":[401,429]},"status":"active"}`)
	if accountResp.Data.Metadata == nil {
		t.Fatalf("expected normalized metadata, got nil")
	}
	metadata := *accountResp.Data.Metadata
	if metadata["pool_mode"] != true || metadata["custom_error_codes_enabled"] != true {
		t.Fatalf("expected legacy policy booleans normalized into metadata, got %+v", metadata)
	}
	codes, ok := metadata["custom_error_codes"].([]any)
	if !ok || len(codes) != 2 {
		t.Fatalf("expected custom_error_codes normalized into metadata, got %+v", metadata["custom_error_codes"])
	}
}

func TestUpdateAccountNormalizesLegacyErrorPolicyCredentialMetadata(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"legacy-update-policy-provider","display_name":"Legacy Update Policy Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"legacy-update-policy-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"https://example.invalid/v1"},"status":"active"}`)

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"credential":{"api_key":"upstream-secret-2","pool_mode":true}}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(sessionCookie)
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected account update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updateResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode account update: %v", err)
	}
	if updateResp.Data.Metadata == nil {
		t.Fatalf("expected normalized metadata, got nil")
	}
	metadata := *updateResp.Data.Metadata
	if metadata["base_url"] != "https://example.invalid/v1" || metadata["pool_mode"] != true {
		t.Fatalf("expected merged normalized metadata, got %+v", metadata)
	}
}

func TestGatewaySchedulerRejectsSaturatedAccount(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"limited-provider","display_name":"Limited Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"limited-model","display_name":"Limited Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(createModelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	createMappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"limited-upstream","status":"active"}`))
	createMappingReq.Header.Set("Content-Type", "application/json")
	createMappingReq.AddCookie(sessionCookie)
	createMappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createMappingRec := httptest.NewRecorder()
	handler.ServeHTTP(createMappingRec, createMappingReq)
	if createMappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", createMappingRec.Code)
	}

	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","name":"saturated-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active","metadata":{"max_concurrency":1,"current_concurrency":1}}`))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.AddCookie(sessionCookie)
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", createAccountRec.Code)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"limited-model","messages":[{"role":"user","content":"hello"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected saturated account 503, got %d", chatRec.Code)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=limited-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one limited-model decision, got %d", len(decisionsResp.Data))
	}
	reasons := decisionsResp.Data[0].RejectReasons
	found := false
	for _, reason := range reasons {
		if reason == "concurrency_full" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected concurrency_full reject reason, got %+v", reasons)
	}
}

func TestGatewayUpdatesAccountRuntimeQuotaMetadataForScheduler(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"quota ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-runtime-quota-provider","display_name":"Account Runtime Quota","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"account-runtime-quota-model","display_name":"Account Runtime Quota Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"account-runtime-quota-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"account-runtime-quota-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","rpm_limit":1},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-runtime-quota-model","messages":[{"role":"user","content":"first quota request"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d body=%s", accountsRec.Code, accountsRec.Body.String())
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	account := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if account == nil || account.Metadata == nil {
		t.Fatalf("expected updated account metadata, got %+v", account)
	}
	if got := intFromJSONValue((*account.Metadata)["rpm_used"]); got != 1 {
		t.Fatalf("expected account rpm_used metadata 1, got %v in %+v", got, account.Metadata)
	}
	if got := intFromJSONValue((*account.Metadata)["tpm_used"]); got != 5 {
		t.Fatalf("expected account tpm_used metadata 5, got %v in %+v", got, account.Metadata)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-runtime-quota-model","messages":[{"role":"user","content":"second quota request"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected account runtime quota rejection 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=account-runtime-quota-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 || !jsonObjectContainsString(decisionsResp.Data[1].RejectReasons, "rpm_limit_exceeded") {
		t.Fatalf("expected second decision rpm_limit_exceeded, got %+v", decisionsResp.Data)
	}
}

func TestGatewayEnforcesAccountRPMWithRedisCounterWhenMetadataIsStale(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		upstreamHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"quota ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-redis-quota-provider","display_name":"Account Redis Quota","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"account-redis-quota-model","display_name":"Account Redis Quota Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"account-redis-quota-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"account-redis-quota-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","rpm_limit":1},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-redis-quota-model","messages":[{"role":"user","content":"first redis quota request"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"metadata":{"base_url":"`+upstream.URL+`/v1","rpm_limit":1,"rpm_used":0,"tpm_used":0}}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected account metadata reset 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"account-redis-quota-model","messages":[{"role":"user","content":"second redis quota request"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected account Redis quota rejection 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(secondRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode account Redis quota response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != "rpm_limit_exceeded" || errResp.Error.Type != apiopenapi.RateLimitError {
		t.Fatalf("unexpected account Redis quota response: %+v", errResp)
	}
	if upstreamHits != 1 {
		t.Fatalf("expected account Redis quota to block before second upstream dispatch, upstream hits=%d", upstreamHits)
	}
}

func TestGatewayEnforcesAccountRPMOnDirectDispatchRouteWithRedisCounter(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		upstreamHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000001,"data":[{"url":"https://example.test/account-redis-image.png"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-redis-image-provider","display_name":"Account Redis Image","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"account-redis-image-model","display_name":"Account Redis Image Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"account-redis-image-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"account-redis-image-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","rpm_limit":1},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/images/generations", `{"model":"account-redis-image-model","prompt":"first direct dispatch image","n":1}`)

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"metadata":{"base_url":"`+upstream.URL+`/v1","rpm_limit":1,"rpm_used":0,"tpm_used":0}}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected account metadata reset 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"account-redis-image-model","prompt":"second direct dispatch image","n":1}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected account Redis quota rejection 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(secondRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode account Redis quota response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != "rpm_limit_exceeded" || errResp.Error.Type != apiopenapi.RateLimitError {
		t.Fatalf("unexpected account Redis quota response: %+v", errResp)
	}
	if upstreamHits != 1 {
		t.Fatalf("expected account Redis quota to block before second image upstream dispatch, upstream hits=%d", upstreamHits)
	}
}

func TestGatewayVisionRequestUsesCapabilityTaxonomy(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"text-only-provider","display_name":"Text Only Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"text-only-model","display_name":"Text Only Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(createModelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	createMappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"text-only-upstream","status":"active"}`))
	createMappingReq.Header.Set("Content-Type", "application/json")
	createMappingReq.AddCookie(sessionCookie)
	createMappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createMappingRec := httptest.NewRecorder()
	handler.ServeHTTP(createMappingRec, createMappingReq)
	if createMappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", createMappingRec.Code)
	}

	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","name":"text-only-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.AddCookie(sessionCookie)
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", createAccountRec.Code)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"text-only-model","messages":[{"role":"user","content":[{"type":"text","text":"describe this"},{"type":"image_url","image_url":{"url":"https://example.invalid/image.png"}}]}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected vision capability mismatch 503, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=text-only-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one text-only-model decision, got %d", len(decisionsResp.Data))
	}
	if !jsonObjectContainsString(decisionsResp.Data[0].RejectReasons, "capability_mismatch") {
		t.Fatalf("expected capability_mismatch reject reason, got %+v", decisionsResp.Data[0].RejectReasons)
	}
	if stringSliceContains(decisionsResp.Data[0].CompatibilityWarnings, "vision_ignored") {
		t.Fatalf("did not expect preserved vision input to be marked ignored, got %+v", decisionsResp.Data[0].CompatibilityWarnings)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=text-only-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].Success || usageResp.Data[0].ErrorClass == nil || *usageResp.Data[0].ErrorClass != "no_available_account" {
		t.Fatalf("expected failed no_available_account usage log, got %+v", usageResp.Data)
	}
}

func TestGatewayReverseProxyAccountAutoProtectsOnSessionInvalid(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation" {
			t.Fatalf("expected chatgpt web conversation path, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Fatalf("expected reverse proxy bearer token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token") != "requirements-token" {
			t.Fatalf("expected chatgpt requirements token, got %+v", r.Header)
		}
		if r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"session_invalid","message":"session expired"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"reverse-provider","display_name":"Reverse Provider","adapter_type":"reverse-proxy-chatgpt-web","protocol":"openai-compatible","status":"active"}`))
	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.AddCookie(sessionCookie)
	providerReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", providerRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	modelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"reverse-model","display_name":"Reverse Model","status":"active"}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.AddCookie(sessionCookie)
	modelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", modelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	mappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"reverse-upstream","status":"active"}`))
	mappingReq.Header.Set("Content-Type", "application/json")
	mappingReq.AddCookie(sessionCookie)
	mappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	mappingRec := httptest.NewRecorder()
	handler.ServeHTTP(mappingRec, mappingReq)
	if mappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", mappingRec.Code)
	}

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"reverse-account","runtime_class":"oauth_refresh","upstream_client":"chatgpt_web","credential":{"access_token":"oauth-access","refresh_token":"refresh-token"},"metadata":{"base_url":"` + upstream.URL + `","user_agent":"ChatGPT/1.0","chatgpt_requirements_token":"requirements-token"},"status":"active"}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")
	accountReq.AddCookie(sessionCookie)
	accountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", accountRec.Code)
	}
	var accountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(accountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"reverse-model","messages":[{"role":"user","content":"call reverse"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatReq.Header.Set("X-Request-ID", "req_reverse_gateway")
	chatReq.Header.Set("X-SRapi-Test", "must-not-forward")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusBadGateway {
		t.Fatalf("expected reverse proxy gateway failure 502, got %d", chatRec.Code)
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?status=needs_reauth", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}
	if len(accountsResp.Data) != 1 || accountsResp.Data[0].Id != accountResp.Data.Id {
		t.Fatalf("expected reverse account needs_reauth, got %+v", accountsResp.Data)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d", metricsRec.Code)
	}
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_request_error_total{error_class="session_invalid"} 1`) {
		t.Fatalf("expected reverse proxy session_invalid metric, got:\n%s", metricsRec.Body.String())
	}
}

func TestGatewayReverseProxyBanSignalDisablesAccountAndStopsScheduling(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.URL.Path != "/backend-api/conversation" {
			t.Fatalf("expected chatgpt web conversation path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"account_banned","message":"account banned"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"banned-provider","display_name":"Banned Provider","adapter_type":"reverse-proxy-chatgpt-web","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"banned-model","display_name":"Banned Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"banned-upstream","status":"active"}`)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"banned-account","runtime_class":"oauth_refresh","upstream_client":"chatgpt_web","credential":{"access_token":"oauth-access","refresh_token":"refresh-token"},"metadata":{"base_url":"` + upstream.URL + `","user_agent":"ChatGPT/1.0","chatgpt_requirements_token":"requirements-token"},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"banned-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway {
		t.Fatalf("expected first banned request 502, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?status=disabled", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d body=%s", accountsRec.Code, accountsRec.Body.String())
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}
	if len(accountsResp.Data) != 1 || accountsResp.Data[0].Id != accountResp.Data.Id {
		t.Fatalf("expected banned account disabled, got %+v", accountsResp.Data)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"banned-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second banned request 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected disabled account to stop further upstream calls, got %d", upstreamCalls)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_account_banned_total 1`) {
		t.Fatalf("expected reverse proxy banned metric, got:\n%s", metricsRec.Body.String())
	}
}

func TestGatewayChatGPTWebReverseProxyUsesConversationOfficialClientShape(t *testing.T) {
	type upstreamCall struct {
		Path              string
		Authorization     string
		UserAgent         string
		Accept            string
		TargetPath        string
		RequirementsToken string
		DeviceID          string
		SessionID         string
		Model             string
		Action            string
		ForceUseSSE       bool
		Message           string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			mu.Lock()
			calls = append(calls, upstreamCall{Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), UserAgent: r.Header.Get("User-Agent"), Accept: r.Header.Get("Accept")})
			mu.Unlock()
			_, _ = w.Write([]byte(`<html data-build="build-123"><script src="/assets/c/test/_build.js"></script></html>`))
			return
		}
		if r.URL.Path == "/backend-api/sentinel/chat-requirements" {
			if r.Header.Get("Authorization") != "Bearer chatgpt-web-token" ||
				r.Header.Get("X-OpenAI-Target-Path") != "/backend-api/sentinel/chat-requirements" {
				t.Fatalf("unexpected requirements request headers: %+v", r.Header)
			}
			var body struct {
				P string `json:"p"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode requirements request: %v", err)
			}
			if !strings.HasPrefix(body.P, "gAAAAAC") {
				t.Fatalf("expected generated legacy requirements token, got %q", body.P)
			}
			mu.Lock()
			calls = append(calls, upstreamCall{
				Path:              r.URL.Path,
				Authorization:     r.Header.Get("Authorization"),
				UserAgent:         r.Header.Get("User-Agent"),
				Accept:            r.Header.Get("Accept"),
				TargetPath:        r.Header.Get("X-OpenAI-Target-Path"),
				RequirementsToken: body.P,
			})
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"requirements-token-auto","so_token":"so-token"}`))
			return
		}
		var payload struct {
			Action      string `json:"action"`
			Model       string `json:"model"`
			ForceUseSSE bool   `json:"force_use_sse"`
			Messages    []struct {
				Content struct {
					Parts []string `json:"parts"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		call := upstreamCall{
			Path:              r.URL.Path,
			Authorization:     r.Header.Get("Authorization"),
			UserAgent:         r.Header.Get("User-Agent"),
			Accept:            r.Header.Get("Accept"),
			TargetPath:        r.Header.Get("X-OpenAI-Target-Path"),
			RequirementsToken: r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token"),
			DeviceID:          r.Header.Get("OAI-Device-Id"),
			SessionID:         r.Header.Get("OAI-Session-Id"),
			Model:             payload.Model,
			Action:            payload.Action,
			ForceUseSSE:       payload.ForceUseSSE,
		}
		if len(payload.Messages) > 0 && len(payload.Messages[0].Content.Parts) > 0 {
			call.Message = payload.Messages[0].Content.Parts[0]
		}
		if r.Header.Get("X-Request-ID") != "" || r.Header.Get("X-SRapi-Test") != "" || strings.Contains(call.UserAgent, "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"chatgpt web gateway ok\"]}}}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp430-chatgpt-web","display_name":"WP430 ChatGPT Web","adapter_type":"reverse-proxy-chatgpt-web","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp430-chatgpt-web-model","display_name":"WP430 ChatGPT Web Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gpt-5-chat-web","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp430-chatgpt-web-account","runtime_class":"oauth_refresh","upstream_client":"chatgpt_web","credential":{"access_token":"chatgpt-web-token","refresh_token":"refresh-token"},"metadata":{"base_url":"`+upstream.URL+`","user_agent":"Mozilla/5.0 ChatGPTWeb/1.0","oai_device_id":"device-gateway-123","oai_session_id":"session-gateway-123","chatgpt_client_version":"client-version-gateway","chatgpt_client_build_number":"build-gateway"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"wp430-chatgpt-web-model","messages":[{"role":"user","content":"hello chatgpt web gateway"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatReq.Header.Set("X-Request-ID", "req_chatgpt_web_gateway")
	chatReq.Header.Set("X-SRapi-Test", "must-not-forward")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected ChatGPT Web gateway success 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(chatResp.Choices) != 1 || decodeChatMessageText(t, chatResp.Choices[0].Message.Content) != "chatgpt web gateway ok" {
		t.Fatalf("unexpected ChatGPT Web chat response: %+v", chatResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 3 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/" || gotCalls[1].Path != "/backend-api/sentinel/chat-requirements" {
		t.Fatalf("expected bootstrap and requirements before conversation, got %+v", gotCalls)
	}
	call := gotCalls[2]
	if call.Path != "/backend-api/conversation" ||
		call.Authorization != "Bearer chatgpt-web-token" ||
		call.UserAgent != "Mozilla/5.0 ChatGPTWeb/1.0" ||
		call.Accept != "text/event-stream" ||
		call.TargetPath != "/backend-api/conversation" ||
		call.RequirementsToken != "requirements-token-auto" ||
		call.DeviceID != "device-gateway-123" ||
		call.SessionID != "session-gateway-123" {
		t.Fatalf("unexpected ChatGPT Web upstream route/auth/headers: %+v", call)
	}
	if call.Model != "gpt-5-chat-web" || call.Action != "next" || !call.ForceUseSSE || call.Message != "hello chatgpt web gateway" {
		t.Fatalf("unexpected ChatGPT Web upstream payload: %+v", call)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp430-chatgpt-web-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one ChatGPT Web decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(providerResp.Data.Id) || decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(accountResp.Data.Id) || decision.TargetProtocol != "openai-compatible" {
		t.Fatalf("expected ChatGPT Web scheduler evidence, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp430-chatgpt-web-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].TargetProtocol == nil || *usageResp.Data[0].TargetProtocol != "openai-compatible" || !usageResp.Data[0].UsageEstimated {
		t.Fatalf("expected ChatGPT Web usage evidence, got %+v", usageResp.Data)
	}
}

func TestGatewayAntigravityReverseProxyUsesDesktopRuntimeIdentity(t *testing.T) {
	var upstreamPath string
	var upstreamAuthorization string
	var upstreamUserAgent string
	var upstreamModel string
	var upstreamPrompt string
	var upstreamProject string
	var upstreamRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamAuthorization = r.Header.Get("Authorization")
		upstreamUserAgent = r.Header.Get("User-Agent")
		if r.Header.Get("X-Request-ID") != "" || r.Header.Get("X-SRapi-Test") != "" || strings.Contains(upstreamUserAgent, "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		var payload struct {
			Project   string `json:"project"`
			RequestID string `json:"requestId"`
			UserAgent string `json:"userAgent"`
			Model     string `json:"model"`
			Request   struct {
				Contents []struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"contents"`
			} `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.UserAgent != "antigravity" {
			t.Fatalf("expected antigravity v1internal body userAgent, got %+v", payload)
		}
		upstreamProject = payload.Project
		upstreamRequestID = payload.RequestID
		upstreamModel = payload.Model
		if len(payload.Request.Contents) > 0 && len(payload.Request.Contents[0].Parts) > 0 {
			upstreamPrompt = payload.Request.Contents[0].Parts[0].Text
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"antigravity ok"}]}}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":7}}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"antigravity-provider","display_name":"Antigravity Provider","adapter_type":"reverse-proxy-antigravity","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"antigravity-model","display_name":"Antigravity Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"antigravity-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"antigravity-account","runtime_class":"desktop_client_token","upstream_client":"antigravity_desktop","credential":{"access_token":"desktop-token"},"metadata":{"base_url":"`+upstream.URL+`","project_id":"project-1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"antigravity-model","messages":[{"role":"user","content":"call antigravity"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatReq.Header.Set("X-Request-ID", "req_antigravity_gateway")
	chatReq.Header.Set("X-SRapi-Test", "must-not-forward")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected antigravity gateway success 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	if upstreamPath != "/v1internal:generateContent" || upstreamAuthorization != "Bearer desktop-token" || upstreamUserAgent != "Antigravity/1.0" {
		t.Fatalf("unexpected antigravity upstream request path=%q auth=%q ua=%q", upstreamPath, upstreamAuthorization, upstreamUserAgent)
	}
	if upstreamProject != "project-1" || !strings.HasPrefix(upstreamRequestID, "agent-") || upstreamModel != "antigravity-upstream" || upstreamPrompt != "call antigravity" {
		t.Fatalf("unexpected antigravity upstream payload project=%q request_id=%q model=%q prompt=%q", upstreamProject, upstreamRequestID, upstreamModel, upstreamPrompt)
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(chatResp.Choices) != 1 || decodeChatMessageText(t, chatResp.Choices[0].Message.Content) != "antigravity ok" {
		t.Fatalf("unexpected antigravity chat response: %+v", chatResp)
	}
}

func TestGatewayReverseProxyOAuthRefreshPersistsCredentialAndAudits(t *testing.T) {
	var gotAuthorization string
	var gotPath string
	var gotRefreshForm url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse refresh form: %v", err)
			}
			gotRefreshForm = cloneURLValues(r.PostForm)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-access","refresh_token":"fresh-token-rotated","expires_in":3600}`))
		case "/backend-api/conversation":
			gotAuthorization = r.Header.Get("Authorization")
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"conversation.delta\",\"delta\":\"refreshed ok\"}\n\ndata: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"refresh-provider","display_name":"Refresh Provider","adapter_type":"reverse-proxy-chatgpt-web","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"refresh-model","display_name":"Refresh Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"refresh-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"refresh-account","runtime_class":"oauth_refresh","upstream_client":"chatgpt_web","credential":{"access_token":"expired-token","refresh_token":"fresh-token","expires_at":"2000-01-01T00:00:00Z"},"metadata":{"base_url":"` + upstream.URL + `","oauth_token_url":"` + upstream.URL + `/oauth/token","oauth_client_id":"chatgpt-client","user_agent":"ChatGPT/1.0","chatgpt_requirements_token":"requirements-token"},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"refresh-model","messages":[{"role":"user","content":"call refresh"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected refreshed gateway success 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	if gotRefreshForm.Get("grant_type") != "refresh_token" || gotRefreshForm.Get("refresh_token") != "fresh-token" || gotRefreshForm.Get("client_id") != "chatgpt-client" {
		t.Fatalf("unexpected refresh form: %v", gotRefreshForm)
	}
	if gotAuthorization != "Bearer fresh-access" {
		t.Fatalf("expected refreshed bearer token, got %q", gotAuthorization)
	}
	if gotPath != "/backend-api/conversation" {
		t.Fatalf("expected ChatGPT Web conversation path, got %q", gotPath)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	refreshAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.oauth_refresh")
	if refreshAudit.ResourceId != string(accountResp.Data.Id) || refreshAudit.After["refresh_status"] != "success" {
		t.Fatalf("unexpected refresh audit: %+v", refreshAudit)
	}
	if _, ok := refreshAudit.After["access_token"]; ok {
		t.Fatalf("refresh audit leaked access token: %+v", refreshAudit.After)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_oauth_refresh_total{status="success"} 1`) {
		t.Fatalf("expected reverse proxy refresh success metric, got:\n%s", metricsRec.Body.String())
	}
}

func TestGatewayReverseProxyOAuthRefreshFailureDoesNotPersistCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when refresh credential is missing")
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"refresh-fail-provider","display_name":"Refresh Fail Provider","adapter_type":"reverse-proxy-chatgpt-web","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"refresh-fail-model","display_name":"Refresh Fail Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"refresh-fail-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"refresh-fail-account","runtime_class":"oauth_refresh","upstream_client":"chatgpt_web","credential":{"access_token":"expired-token","expires_at":"2000-01-01T00:00:00Z"},"metadata":{"base_url":"` + upstream.URL + `/v1","user_agent":"ChatGPT/1.0"},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"refresh-fail-model","messages":[{"role":"user","content":"call refresh"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusBadGateway {
		t.Fatalf("expected refresh failure 502, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	failedAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.oauth_refresh_failed")
	if failedAudit.ResourceId != string(accountResp.Data.Id) || failedAudit.After["refresh_status"] != "failed" {
		t.Fatalf("unexpected refresh failure audit: %+v", failedAudit)
	}
	if auditLogHasAction(auditResp.Data, "provider_account.oauth_refresh") {
		t.Fatalf("did not expect successful refresh audit in %+v", auditResp.Data)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_oauth_refresh_total{status="credential_missing"} 1`) {
		t.Fatalf("expected reverse proxy refresh missing credential metric, got:\n%s", metricsRec.Body.String())
	}
}

func mustLoginAdmin(t *testing.T, handler http.Handler) (apiopenapi.LoginResponse, *http.Cookie) {
	t.Helper()
	loginBody := `{"email":"admin@srapi.local","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d", loginRec.Code)
	}
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	return loginResp, cookies[0]
}

func mustCreateGatewayAPIKey(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string) (apiopenapi.CreateApiKeyResponse, string) {
	t.Helper()
	resp := mustCreateAPIKey(t, handler, sessionCookie, csrfToken, `{"name":"gateway","scopes":["gateway:invoke"]}`)
	return resp, resp.Data.PlaintextKey
}

func mustCreateAPIKey(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.CreateApiKeyResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected api key create 201, got %d", rec.Code)
	}
	var resp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}
	return resp
}

func mustCreatePaymentProvider(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.PaymentProviderInstanceResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/payments/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected payment provider create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.PaymentProviderInstanceResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode payment provider response: %v", err)
	}
	return resp
}

func mustTestPaymentProvider(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, providerID apiopenapi.Id) apiopenapi.AdminTestResultResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/payments/providers/"+string(providerID)+"/test", nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected payment provider test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode payment provider test response: %v", err)
	}
	return resp
}

type alipayWebhookHTTPTestKeys struct {
	merchantPrivateKey string
	alipayPrivateKey   string
	alipayPublicKey    string
}

func newAlipayWebhookHTTPTestKeys(t *testing.T) alipayWebhookHTTPTestKeys {
	t.Helper()
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate merchant key: %v", err)
	}
	alipayKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate alipay key: %v", err)
	}
	return alipayWebhookHTTPTestKeys{
		merchantPrivateKey: encodeRSAPrivateKeyForHTTPTest(merchantKey),
		alipayPrivateKey:   encodeRSAPrivateKeyForHTTPTest(alipayKey),
		alipayPublicKey:    encodeRSAPublicKeyForHTTPTest(t, &alipayKey.PublicKey),
	}
}

func signedAlipayHTTPNotification(t *testing.T, keys alipayWebhookHTTPTestKeys, fields map[string]string) map[string]any {
	t.Helper()
	client, err := alipaysdk.New("app_test_123", keys.alipayPrivateKey, false)
	if err != nil {
		t.Fatalf("new alipay signer: %v", err)
	}
	values := url.Values{}
	for key, value := range fields {
		values.Set(key, value)
	}
	signature, err := client.SignValues(values)
	if err != nil {
		t.Fatalf("sign alipay notification: %v", err)
	}
	values.Set("sign_type", "RSA2")
	values.Set("sign", base64.StdEncoding.EncodeToString(signature))
	payload := make(map[string]any, len(values))
	for key, values := range values {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	return payload
}

type wechatWebhookHTTPTestKeys struct {
	apiV3Key            string
	merchantPrivateKey  string
	platformPrivateKey  *rsa.PrivateKey
	platformPublicKey   string
	platformPublicKeyID string
}

func newWechatWebhookHTTPTestKeys(t *testing.T) wechatWebhookHTTPTestKeys {
	t.Helper()
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wechat merchant key: %v", err)
	}
	platformKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wechat platform key: %v", err)
	}
	return wechatWebhookHTTPTestKeys{
		apiV3Key:            "0123456789abcdef0123456789abcdef",
		merchantPrivateKey:  encodeRSAPrivateKeyForHTTPTest(merchantKey),
		platformPrivateKey:  platformKey,
		platformPublicKey:   encodeRSAPublicKeyForHTTPTest(t, &platformKey.PublicKey),
		platformPublicKeyID: "PUB_KEY_ID_HTTP_TEST",
	}
}

func signedWechatHTTPNotification(t *testing.T, keys wechatWebhookHTTPTestKeys, transaction map[string]any) (string, map[string]string) {
	t.Helper()
	plaintext, err := json.Marshal(transaction)
	if err != nil {
		t.Fatalf("marshal wechat transaction: %v", err)
	}
	block, err := aes.NewCipher([]byte(keys.apiV3Key))
	if err != nil {
		t.Fatalf("new wechat aes cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new wechat gcm: %v", err)
	}
	associatedData := "transaction"
	resourceNonce := "notify123456"
	ciphertext := aead.Seal(nil, []byte(resourceNonce), plaintext, []byte(associatedData))
	body := map[string]any{
		"id":            "evt_wechat_http_paid",
		"create_time":   time.Now().UTC().Format(time.RFC3339),
		"event_type":    "TRANSACTION.SUCCESS",
		"resource_type": "encrypt-resource",
		"summary":       "transaction success",
		"resource": map[string]any{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      base64.StdEncoding.EncodeToString(ciphertext),
			"associated_data": associatedData,
			"nonce":           resourceNonce,
			"original_type":   "transaction",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal wechat notification: %v", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signNonce := "signnonce123"
	message := timestamp + "\n" + signNonce + "\n" + string(raw) + "\n"
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, keys.platformPrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign wechat notification: %v", err)
	}
	return string(raw), map[string]string{
		"Wechatpay-Nonce":     signNonce,
		"Wechatpay-Serial":    keys.platformPublicKeyID,
		"Wechatpay-Signature": base64.StdEncoding.EncodeToString(signature),
		"Wechatpay-Timestamp": timestamp,
	}
}

func encodeRSAPrivateKeyForHTTPTest(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func encodeRSAPublicKeyForHTTPTest(t *testing.T, key *rsa.PublicKey) string {
	t.Helper()
	raw, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: raw}))
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

func geminiModelMethodsContain(methods []apiopenapi.GeminiModelInfoSupportedGenerationMethods, expected apiopenapi.GeminiModelInfoSupportedGenerationMethods) bool {
	for _, method := range methods {
		if method == expected {
			return true
		}
	}
	return false
}

func mustGatewayRequest(t *testing.T, handler http.Handler, apiKey, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s %s 200, got %d body=%s", method, path, rec.Code, rec.Body.String())
	}
	return rec
}

func mustReadAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}

func mustGatewayMultipartRequest(t *testing.T, handler http.Handler, apiKey, path string, fields map[string]string, filename string, contentType string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected POST %s 200, got %d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func mustGatewayImageEditRequest(t *testing.T, handler http.Handler, apiKey, path string, fields map[string]string, imageFilename string, imageContentType string, imagePayload []byte, maskFilename string, maskContentType string, maskPayload []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	writeMultipartTestFile(t, writer, "image", imageFilename, imageContentType, imagePayload)
	if len(maskPayload) > 0 {
		writeMultipartTestFile(t, writer, "mask", maskFilename, maskContentType, maskPayload)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected POST %s 200, got %d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func mustGatewayImageVariationRequest(t *testing.T, handler http.Handler, apiKey, path string, fields map[string]string, imageFilename string, imageContentType string, imagePayload []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	writeMultipartTestFile(t, writer, "image", imageFilename, imageContentType, imagePayload)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected POST %s 200, got %d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func writeMultipartTestFile(t *testing.T, writer *multipart.Writer, fieldName string, filename string, contentType string, payload []byte) {
	t.Helper()
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
}

func seedUsageLog(t *testing.T, store usagecontract.Store, input usagecontract.UsageLog) {
	t.Helper()
	if input.UserID == 0 {
		input.UserID = 1
	}
	if input.APIKeyID == 0 {
		input.APIKeyID = 1
	}
	if input.SourceProtocol == "" {
		input.SourceProtocol = "openai-compatible"
	}
	if input.TargetProtocol == "" {
		input.TargetProtocol = "openai-compatible"
	}
	if input.Cost == "" {
		input.Cost = "0.00000000"
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if _, err := store.Create(t.Context(), input); err != nil {
		t.Fatalf("seed usage log: %v", err)
	}
}

func mustCreateProvider(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ProviderResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	return resp
}

func mustUpdateProvider(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, providerID, body string) apiopenapi.ProviderResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/providers/"+providerID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider update response: %v", err)
	}
	return resp
}

func mustTestProvider(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, providerID string) apiopenapi.AdminTestResultResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/"+providerID+"/test", nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider test response: %v", err)
	}
	return resp
}

func mustFindProviderByName(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, name string) apiopenapi.Provider {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	for _, provider := range resp.Data {
		if provider.Name == name {
			return provider
		}
	}
	t.Fatalf("provider %s not found in %+v", name, resp.Data)
	return apiopenapi.Provider{}
}

func mustInstallProviderPresets(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string) apiopenapi.BatchOperationResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/preset/install", nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider preset install 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.BatchOperationResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider preset install response: %v", err)
	}
	return resp
}

func mustCreateModel(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ModelResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}
	return resp
}

func mustCreateMapping(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, modelID, body string) apiopenapi.ModelProviderMappingResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+modelID+"/mappings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelProviderMappingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode mapping response: %v", err)
	}
	return resp
}

func mustCreateAccount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ProviderAccountResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	return resp
}

type upstreamGeminiCall struct {
	Path          string
	Authorization string
	Model         string
	Messages      []upstreamMessage
	MaxTokens     int
}

type upstreamNativeGeminiCall struct {
	Path     string
	APIKey   string
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	SystemText string
	MaxTokens  int
}

func geminiSystemInstructionText(value *struct {
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}) string {
	if value == nil || len(value.Parts) == 0 {
		return ""
	}
	return value.Parts[0].Text
}

func mustCreateGeminiGatewayTarget(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, upstreamURL string, unique bool) (apiopenapi.ProviderResponse, apiopenapi.ModelResponse, apiopenapi.ProviderAccountResponse) {
	t.Helper()
	suffix := ""
	if unique {
		suffix = "-error"
	}
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"gemini-route-provider`+suffix+`","display_name":"Gemini Route Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"gemini-route-model`+suffix+`","display_name":"Gemini Route Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-route-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"gemini-route-account`+suffix+`","runtime_class":"api_key","credential":{"api_key":"gemini-route-upstream-secret"},"metadata":{"base_url":"`+upstreamURL+`/v1"},"status":"active"}`)
	return providerResp, modelResp, accountResp
}

func mustCreateNativeGeminiGatewayTarget(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, upstreamURL string) (apiopenapi.ProviderResponse, apiopenapi.ModelResponse, apiopenapi.ProviderAccountResponse) {
	t.Helper()
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"native-gemini-route-provider","display_name":"Native Gemini Route Provider","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active","capabilities":{"token_counting":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"native-gemini-route-model","display_name":"Native Gemini Route Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"token_counting","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-native-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"native-gemini-route-account","runtime_class":"api_key","credential":{"api_key":"native-gemini-secret"},"metadata":{"base_url":"`+upstreamURL+`/v1beta"},"status":"active"}`)
	return providerResp, modelResp, accountResp
}

func mustCreateAccountGroup(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.AccountGroupResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account group create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AccountGroupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account group response: %v", err)
	}
	return resp
}

func decodeChatMessageText(t *testing.T, content apiopenapi.ChatMessage_Content) string {
	t.Helper()
	text, err := content.AsChatMessageContent0()
	if err == nil {
		return text
	}
	blocks, err := content.AsChatMessageContent1()
	if err != nil || len(blocks) == 0 || blocks[0].Text == nil {
		t.Fatalf("unexpected chat message content: %v %v", blocks, err)
	}
	return *blocks[0].Text
}

func assertAdminListContains(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, path string, countFn func([]byte) int, expected int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s 200, got %d", path, rec.Code)
	}
	if got := countFn(rec.Body.Bytes()); got != expected {
		t.Fatalf("expected %s count %d, got %d", path, expected, got)
	}
	_ = csrfToken
}

func auditLogHasAction(items []apiopenapi.AuditLog, action string) bool {
	for _, item := range items {
		if item.Action == action {
			return true
		}
	}
	return false
}

func pricingRuleListHasID(items []apiopenapi.PricingRule, id apiopenapi.Id) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

func auditContractLogHasAction(items []auditcontract.Log, action string) bool {
	for _, item := range items {
		if item.Action == action {
			return true
		}
	}
	return false
}

func mustFindAuditLog(t *testing.T, items []apiopenapi.AuditLog, action string) apiopenapi.AuditLog {
	t.Helper()
	for _, item := range items {
		if item.Action == action {
			return item
		}
	}
	t.Fatalf("audit action %s not found in %+v", action, items)
	return apiopenapi.AuditLog{}
}

func findProviderAccountByID(items []apiopenapi.ProviderAccount, id apiopenapi.Id) *apiopenapi.ProviderAccount {
	for i := range items {
		if items[i].Id == id {
			return &items[i]
		}
	}
	return nil
}

func findProviderAccountExportByName(items []apiopenapi.ProviderAccountExportItem, name string) *apiopenapi.ProviderAccountExportItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func jsonObjectContainsString(value apiopenapi.JsonObject, target string) bool {
	for _, item := range value {
		switch item := item.(type) {
		case string:
			if item == target {
				return true
			}
		case []any:
			for _, nested := range item {
				if nested == target {
					return true
				}
			}
		case []string:
			for _, nested := range item {
				if nested == target {
					return true
				}
			}
		}
	}
	return false
}

func intFromJSONValue(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func rejectionReasonsContain(value apiopenapi.JsonObject, target string) bool {
	for _, item := range value {
		if item == target {
			return true
		}
	}
	return false
}
