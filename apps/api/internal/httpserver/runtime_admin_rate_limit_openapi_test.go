package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

func TestAdminRateLimitsUseOpenAPIWireTypesAndFeedGateway(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"rate ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"openapi-rate-provider","display_name":"OpenAPI Rate Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"openapi-rate-model","display_name":"OpenAPI Rate Model","status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, csrfToken, `{"name":"openapi-rate-group","description":"OpenAPI Rate Group","status":"active"}`)
	modelID := mustParseOpenAPIID(t, modelResp.Data.Id)
	groupID := mustParseOpenAPIID(t, groupResp.Data.Id)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"openapi-rate-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"openapi-rate-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	accountID := mustParseOpenAPIID(t, accountResp.Data.Id)
	addAccountGroupMember(t, handler, sessionCookie, csrfToken, groupID, accountID)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrfToken)

	modelLimit := upsertModelRateLimit(t, handler, sessionCookie, csrfToken, `{"model_id":`+strconv.FormatInt(modelID, 10)+`,"rpm_limit":1,"tpm_limit":0,"max_concurrency":0,"enabled":true}`)
	if modelLimit.Data.ModelId != modelID || modelLimit.Data.RpmLimit != 1 || !modelLimit.Data.Enabled {
		t.Fatalf("unexpected model rate limit response: %+v", modelLimit.Data)
	}
	groupLimit := upsertGroupRateLimit(t, handler, sessionCookie, csrfToken, `{"account_group_id":`+strconv.FormatInt(groupID, 10)+`,"rpm_limit":1,"tpm_limit":0,"max_concurrency":0,"enabled":true}`)
	if groupLimit.Data.AccountGroupId != groupID || groupLimit.Data.RpmLimit != 1 || !groupLimit.Data.Enabled {
		t.Fatalf("unexpected group rate limit response: %+v", groupLimit.Data)
	}

	assertModelRateLimitListContains(t, handler, sessionCookie, modelID)
	assertGroupRateLimitListContains(t, handler, sessionCookie, groupID)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"openapi-rate-model","messages":[{"role":"user","content":"first"}]}`)
	second := gatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"openapi-rate-model","messages":[{"role":"user","content":"second"}]}`)
	assertGatewayRateLimitError(t, second, "rpm_limit_exceeded")
	if upstreamHits != 1 {
		t.Fatalf("expected model RPM limit to block before second upstream dispatch, upstream hits=%d", upstreamHits)
	}

	deleteModelRateLimit(t, handler, sessionCookie, csrfToken, modelID)
	redisServer.FlushAll()
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"openapi-rate-model","messages":[{"role":"user","content":"third"}]}`)
	groupLimited := gatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"openapi-rate-model","messages":[{"role":"user","content":"fourth"}]}`)
	assertGatewayRateLimitError(t, groupLimited, "rpm_limit_exceeded")

	deleteGroupRateLimit(t, handler, sessionCookie, csrfToken, groupID)
	redisServer.FlushAll()
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"openapi-rate-model","messages":[{"role":"user","content":"fifth"}]}`)
}

func TestGatewayRateLimitReasonsKeepModelAndGroupDimensionsActionable(t *testing.T) {
	for _, name := range []string{"rpm", "model_rpm", "group_rpm"} {
		if got := gatewayRateLimitReason(name); got != "rpm_limit_exceeded" {
			t.Fatalf("gatewayRateLimitReason(%q) = %q, want rpm_limit_exceeded", name, got)
		}
	}
	for _, name := range []string{"tpm", "model_tpm", "group_tpm"} {
		if got := gatewayRateLimitReason(name); got != "tpm_limit_exceeded" {
			t.Fatalf("gatewayRateLimitReason(%q) = %q, want tpm_limit_exceeded", name, got)
		}
	}
	for _, name := range []string{"account_rpm", "group_rpm"} {
		if got := gatewayAccountQuotaErrorClass(name); got != "rpm_limit_exceeded" {
			t.Fatalf("gatewayAccountQuotaErrorClass(%q) = %q, want rpm_limit_exceeded", name, got)
		}
	}
	for _, name := range []string{"account_tpm", "group_tpm"} {
		if got := gatewayAccountQuotaErrorClass(name); got != "tpm_limit_exceeded" {
			t.Fatalf("gatewayAccountQuotaErrorClass(%q) = %q, want tpm_limit_exceeded", name, got)
		}
	}
}

func TestAdminRateLimitConcurrencyFeedsGateway(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	modelFixture := newRateLimitConcurrencyFixture(t, limiter, "model")
	upsertModelRateLimit(t, modelFixture.handler, modelFixture.sessionCookie, modelFixture.csrfToken, `{"model_id":`+strconv.FormatInt(modelFixture.modelID, 10)+`,"max_concurrency":1,"enabled":true}`)
	modelFixture.startHoldingRequest(t)
	assertConcurrentGatewayRequestLimited(t, modelFixture.handler, modelFixture.apiKey, modelFixture.modelName)
	modelFixture.finishHoldingRequest(t)
	deleteModelRateLimit(t, modelFixture.handler, modelFixture.sessionCookie, modelFixture.csrfToken, modelFixture.modelID)

	redisServer.FlushAll()
	groupFixture := newRateLimitConcurrencyFixture(t, limiter, "group")
	upsertGroupRateLimit(t, groupFixture.handler, groupFixture.sessionCookie, groupFixture.csrfToken, `{"account_group_id":`+strconv.FormatInt(groupFixture.groupID, 10)+`,"max_concurrency":1,"enabled":true}`)
	groupFixture.startHoldingRequest(t)
	assertConcurrentGatewayRequestLimited(t, groupFixture.handler, groupFixture.apiKey, groupFixture.modelName)
	groupFixture.finishHoldingRequest(t)
	deleteGroupRateLimit(t, groupFixture.handler, groupFixture.sessionCookie, groupFixture.csrfToken, groupFixture.groupID)
}

type rateLimitConcurrencyFixture struct {
	handler         http.Handler
	apiKey          string
	sessionCookie   *http.Cookie
	csrfToken       string
	modelID         int64
	groupID         int64
	modelName       string
	upstreamStarted chan struct{}
	releaseUpstream chan struct{}
	firstDone       chan *httptest.ResponseRecorder
	releaseOnce     sync.Once
}

func newRateLimitConcurrencyFixture(t *testing.T, limiter *ratelimit.Limiter, suffix string) *rateLimitConcurrencyFixture {
	t.Helper()
	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var startOnce sync.Once
	fixture := &rateLimitConcurrencyFixture{
		upstreamStarted: upstreamStarted,
		releaseUpstream: releaseUpstream,
		firstDone:       make(chan *httptest.ResponseRecorder, 1),
	}
	t.Cleanup(func() {
		fixture.releaseOnce.Do(func() { close(releaseUpstream) })
	})
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
	t.Cleanup(upstream.Close)

	cfg := config.Load()
	cfg.Gateway.RequestTimeout = time.Minute
	handler := New(cfg, nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrfToken := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"openapi-concurrency-provider-`+suffix+`","display_name":"OpenAPI Concurrency Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelName := "openapi-concurrency-model-" + suffix
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"`+modelName+`","display_name":"OpenAPI Concurrency Model","status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, csrfToken, `{"name":"openapi-concurrency-group-`+suffix+`","description":"OpenAPI Concurrency Group","status":"active"}`)
	modelID := mustParseOpenAPIID(t, modelResp.Data.Id)
	groupID := mustParseOpenAPIID(t, groupResp.Data.Id)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"openapi-concurrency-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"openapi-concurrency-account-`+suffix+`","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	accountID := mustParseOpenAPIID(t, accountResp.Data.Id)
	addAccountGroupMember(t, handler, sessionCookie, csrfToken, groupID, accountID)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrfToken)

	fixture.handler = handler
	fixture.apiKey = apiKey
	fixture.sessionCookie = sessionCookie
	fixture.csrfToken = csrfToken
	fixture.modelID = modelID
	fixture.groupID = groupID
	fixture.modelName = modelName
	return fixture
}

func (f *rateLimitConcurrencyFixture) startHoldingRequest(t *testing.T) {
	t.Helper()
	go func() {
		f.firstDone <- gatewayRequest(t, f.handler, f.apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"`+f.modelName+`","messages":[{"role":"user","content":"first"}]}`)
	}()
	select {
	case <-f.upstreamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first gateway request to reach upstream")
	}
}

func (f *rateLimitConcurrencyFixture) finishHoldingRequest(t *testing.T) {
	t.Helper()
	f.releaseOnce.Do(func() { close(f.releaseUpstream) })
	select {
	case firstRec := <-f.firstDone:
		if firstRec.Code != http.StatusOK {
			t.Fatalf("expected first gateway request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first gateway request to finish")
	}
}

func assertConcurrentGatewayRequestLimited(t *testing.T, handler http.Handler, apiKey, model string) {
	t.Helper()
	rec := gatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"second"}]}`)
	assertGatewayRateLimitError(t, rec, "concurrency_limit_exceeded")
}

func addAccountGroupMember(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, groupID int64, accountID int64) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-groups/"+strconv.FormatInt(groupID, 10)+"/accounts/"+strconv.FormatInt(accountID, 10), nil)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account group member add 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AccountGroupMemberResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account group member response: %v", err)
	}
	if resp.Data.AccountId != apiopenapi.Id(strconv.FormatInt(accountID, 10)) ||
		resp.Data.AccountGroupId != apiopenapi.Id(strconv.FormatInt(groupID, 10)) {
		t.Fatalf("unexpected account group member response: %+v", resp.Data)
	}
}

func assertModelRateLimitListContains(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, modelID int64) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/model-rate-limits?page=1&page_size=10", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected model rate limit list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelRateLimitListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model rate limit list response: %v", err)
	}
	if !modelRateLimitListHasModelID(resp.Data, modelID) {
		t.Fatalf("expected model rate limit list to contain %d, got %+v", modelID, resp.Data)
	}
}

func assertGroupRateLimitListContains(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, groupID int64) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/group-rate-limits?page=1&page_size=10", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected group rate limit list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.GroupRateLimitListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode group rate limit list response: %v", err)
	}
	if !groupRateLimitListHasGroupID(resp.Data, groupID) {
		t.Fatalf("expected group rate limit list to contain %d, got %+v", groupID, resp.Data)
	}
}

func deleteModelRateLimit(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, modelID int64) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/model-rate-limits/"+strconv.FormatInt(modelID, 10), nil)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assertDeleteResponse(t, rec, "model rate limit")
}

func deleteGroupRateLimit(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, groupID int64) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/group-rate-limits/"+strconv.FormatInt(groupID, 10), nil)
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assertDeleteResponse(t, rec, "group rate limit")
}

func assertDeleteResponse(t *testing.T, rec *httptest.ResponseRecorder, name string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s delete 200, got %d body=%s", name, rec.Code, rec.Body.String())
	}
	var resp apiopenapi.DeleteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode %s delete response: %v", name, err)
	}
	if !resp.Data.Deleted {
		t.Fatalf("unexpected %s delete response: %+v", name, resp.Data)
	}
}

func gatewayRequest(t *testing.T, handler http.Handler, apiKey, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertGatewayRateLimitError(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected gateway 429, got %d body=%s", rec.Code, rec.Body.String())
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode gateway rate limit response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != code || errResp.Error.Type != apiopenapi.RateLimitError {
		t.Fatalf("unexpected gateway rate limit response: %+v", errResp)
	}
}
