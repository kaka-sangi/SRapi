package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func mustProviderContractForExclusionTest(id int, configSchema map[string]any) providercontract.Provider {
	return providercontract.Provider{ID: id, ConfigSchema: configSchema}
}

func mustAccountContractForExclusionTest(id int, metadata map[string]any) accountcontract.ProviderAccount {
	return accountcontract.ProviderAccount{ID: id, ProviderID: id, Metadata: metadata}
}

// TestAccountExcludesModel covers the per-account excluded_models wildcard
// resolver: exact, prefix, suffix, and substring patterns, plus the guards that
// keep an empty/blank list from excluding everything. Names are matched after
// stripping the discovery "models/" prefix.
func TestAccountExcludesModel(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		models   []string
		want     bool
	}{
		{name: "nil metadata", metadata: nil, models: []string{"gpt-4o"}, want: false},
		{name: "no key", metadata: map[string]any{"base_url": "x"}, models: []string{"gpt-4o"}, want: false},
		{name: "empty list", metadata: map[string]any{"excluded_models": []any{}}, models: []string{"gpt-4o"}, want: false},
		{name: "blank patterns ignored", metadata: map[string]any{"excluded_models": []any{"  ", ""}}, models: []string{"gpt-4o"}, want: false},
		{name: "exact match", metadata: map[string]any{"excluded_models": []any{"gpt-4o"}}, models: []string{"gpt-4o"}, want: true},
		{name: "exact no match", metadata: map[string]any{"excluded_models": []any{"gpt-4o"}}, models: []string{"claude-3"}, want: false},
		{name: "prefix wildcard", metadata: map[string]any{"excluded_models": []any{"gpt-4*"}}, models: []string{"gpt-4o-mini"}, want: true},
		{name: "prefix anchored miss", metadata: map[string]any{"excluded_models": []any{"gpt-4*"}}, models: []string{"x-gpt-4o"}, want: false},
		{name: "suffix wildcard", metadata: map[string]any{"excluded_models": []any{"*-preview"}}, models: []string{"o1-preview"}, want: true},
		{name: "suffix anchored miss", metadata: map[string]any{"excluded_models": []any{"*-preview"}}, models: []string{"o1-preview-2"}, want: false},
		{name: "substring wildcard", metadata: map[string]any{"excluded_models": []any{"*vision*"}}, models: []string{"gpt-4-vision-preview"}, want: true},
		{name: "case insensitive", metadata: map[string]any{"excluded_models": []any{"GPT-4O"}}, models: []string{"gpt-4o"}, want: true},
		{name: "models/ prefix stripped", metadata: map[string]any{"excluded_models": []any{"gemini-pro"}}, models: []string{"models/gemini-pro"}, want: true},
		{name: "comma string form", metadata: map[string]any{"excluded_models": "gpt-4o, claude-3"}, models: []string{"claude-3"}, want: true},
		{name: "hyphen key form", metadata: map[string]any{"excluded-models": []any{"claude-*"}}, models: []string{"claude-sonnet-4"}, want: true},
		{name: "second name matches", metadata: map[string]any{"excluded_models": []any{"upstream-x"}}, models: []string{"catalog-model", "upstream-x"}, want: true},
		{name: "blank name skipped", metadata: map[string]any{"excluded_models": []any{"*"}}, models: []string{"  "}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accountExcludesModel(tc.metadata, tc.models...)
			if got != tc.want {
				t.Fatalf("accountExcludesModel(%v, %v) = %v, want %v", tc.metadata, tc.models, got, tc.want)
			}
		})
	}
}

func TestProviderAccountExcludesModel(t *testing.T) {
	provider := mustProviderContractForExclusionTest(1, map[string]any{
		"excluded_models": []any{"provider-secret", "provider-upstream-*"},
	})
	account := mustAccountContractForExclusionTest(1, map[string]any{
		"excluded_models": []any{"account-secret"},
	})
	if !providerAccountExcludesModel(provider, account, "provider-secret") {
		t.Fatal("expected provider-level canonical exclusion to apply")
	}
	if !providerAccountExcludesModel(provider, account, "catalog-name", "provider-upstream-1") {
		t.Fatal("expected provider-level upstream exclusion to apply")
	}
	if !providerAccountExcludesModel(provider, account, "account-secret") {
		t.Fatal("expected account-level exclusion to remain active")
	}
	if providerAccountExcludesModel(provider, account, "visible-model", "visible-upstream") {
		t.Fatal("did not expect unrelated model to be excluded")
	}
}

// TestGatewayExcludedModelDeniesScheduling proves the wire effect: the only
// account serving a catalog model excludes it via a wildcard, so the gateway
// returns 503 no_available_account rather than routing upstream.
func TestGatewayExcludedModelDeniesScheduling(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"excl-provider","display_name":"Excl Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"excl-model","display_name":"Excl Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"excl-model","status":"active"}`)
	// The only account serving this provider excludes the model by wildcard.
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"excl-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","excluded_models":["excl-*"]},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	// mustGatewayRequest asserts 200, so issue the deny request directly.
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"excl-model","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no available account) for excluded model, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGatewayProviderExcludedModelDeniesScheduling(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"provider-excl","display_name":"Provider Excl","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"excluded_models":["provider-excl-*"]}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"provider-excl-model","display_name":"Provider Excl Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-excl-model","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"provider-excl-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"provider-excl-model","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no available account) for provider-excluded model, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGatewayProviderSupportedModelsDeniesScheduling(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"provider-supported","display_name":"Provider Supported","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"supported_models":["provider-visible"]}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"provider-blocked","display_name":"Provider Blocked","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-blocked","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"provider-supported-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"provider-blocked","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no available account) for provider allowlist miss, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGatewayExcludedModelHiddenFromListing proves /v1/models hides a catalog
// model whose every serving account excludes it, while leaving a sibling model
// (not excluded) visible.
func TestGatewayExcludedModelHiddenFromListing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"hide-provider","display_name":"Hide Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	hiddenModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"hide-secret","display_name":"Hide Secret","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	visibleModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"hide-visible","display_name":"Hide Visible","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	createAliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(hiddenModel.Data.Id)+"/aliases", strings.NewReader(`{"alias":"hide-secret-alias","status":"active"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasReq.AddCookie(sessionCookie)
	createAliasReq.Header.Set("X-CSRF-Token", csrf)
	createAliasRec := httptest.NewRecorder()
	handler.ServeHTTP(createAliasRec, createAliasReq)
	if createAliasRec.Code != http.StatusCreated {
		t.Fatalf("expected hidden model alias create 201, got %d body=%s", createAliasRec.Code, createAliasRec.Body.String())
	}
	mustCreateMapping(t, handler, sessionCookie, csrf, string(hiddenModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"hide-secret","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(visibleModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"hide-visible","status":"active"}`)
	// One account: excludes "hide-secret" but serves "hide-visible".
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"hide-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","excluded_models":["hide-secret"]},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /v1/models, got %d body=%s", rec.Code, rec.Body.String())
	}
	var list apiopenapi.OpenAIModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode model list: %v body=%s", err, rec.Body.String())
	}
	ids := map[string]bool{}
	for _, m := range list.Data {
		ids[m.Id] = true
	}
	if ids["hide-secret"] {
		t.Fatalf("expected excluded model hidden from /v1/models, got list=%s", rec.Body.String())
	}
	if ids["hide-secret-alias"] {
		t.Fatalf("expected excluded model alias hidden from /v1/models, got list=%s", rec.Body.String())
	}
	if !ids["hide-visible"] {
		t.Fatalf("expected non-excluded sibling model visible in /v1/models, got list=%s", rec.Body.String())
	}
}

func TestGatewayProviderExcludedModelHiddenFromListing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"provider-hide","display_name":"Provider Hide","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"excluded_models":["provider-hidden"]}}`)
	hiddenModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"provider-hidden","display_name":"Provider Hidden","status":"active"}`)
	visibleModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"provider-visible","display_name":"Provider Visible","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(hiddenModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-hidden","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(visibleModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-visible","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"provider-hide-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /v1/models, got %d body=%s", rec.Code, rec.Body.String())
	}
	var list apiopenapi.OpenAIModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode model list: %v body=%s", err, rec.Body.String())
	}
	ids := map[string]bool{}
	for _, m := range list.Data {
		ids[m.Id] = true
	}
	if ids["provider-hidden"] {
		t.Fatalf("expected provider-excluded model hidden from /v1/models, got list=%s", rec.Body.String())
	}
	if !ids["provider-visible"] {
		t.Fatalf("expected visible sibling model in /v1/models, got list=%s", rec.Body.String())
	}
}

func TestGatewayProviderSupportedModelHiddenFromListing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"provider-supported-list","display_name":"Provider Supported List","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"supported_models":["provider-visible"]}}`)
	hiddenModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"provider-hidden","display_name":"Provider Hidden","status":"active"}`)
	visibleModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"provider-visible","display_name":"Provider Visible","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(hiddenModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-hidden","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(visibleModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-visible","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"provider-supported-list-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /v1/models, got %d body=%s", rec.Code, rec.Body.String())
	}
	var list apiopenapi.OpenAIModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode model list: %v body=%s", err, rec.Body.String())
	}
	ids := map[string]bool{}
	for _, m := range list.Data {
		ids[m.Id] = true
	}
	if ids["provider-hidden"] {
		t.Fatalf("expected provider-unsupported model hidden from /v1/models, got list=%s", rec.Body.String())
	}
	if !ids["provider-visible"] {
		t.Fatalf("expected provider-supported sibling model in /v1/models, got list=%s", rec.Body.String())
	}
}

// TestGatewayUnsupportedModelHiddenFromListing proves model-list visibility
// uses the same per-account supported_models allowlist as gateway scheduling:
// a catalog model whose only active serving account cannot route its mapped
// upstream model is hidden, including active aliases for that canonical model.
func TestGatewayUnsupportedModelHiddenFromListing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"supported-list-provider","display_name":"Supported List Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	hiddenModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"supported-hidden","display_name":"Supported Hidden","status":"active"}`)
	visibleModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"supported-visible","display_name":"Supported Visible","status":"active"}`)
	createAliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(hiddenModel.Data.Id)+"/aliases", strings.NewReader(`{"alias":"supported-hidden-alias","status":"active"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasReq.AddCookie(sessionCookie)
	createAliasReq.Header.Set("X-CSRF-Token", csrf)
	createAliasRec := httptest.NewRecorder()
	handler.ServeHTTP(createAliasRec, createAliasReq)
	if createAliasRec.Code != http.StatusCreated {
		t.Fatalf("expected hidden model alias create 201, got %d body=%s", createAliasRec.Code, createAliasRec.Body.String())
	}
	mustCreateMapping(t, handler, sessionCookie, csrf, string(hiddenModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"unsupported-upstream","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(visibleModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"supported-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"supported-list-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","supported_models":["supported-upstream"]},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /v1/models, got %d body=%s", rec.Code, rec.Body.String())
	}
	var list apiopenapi.OpenAIModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode model list: %v body=%s", err, rec.Body.String())
	}
	ids := map[string]bool{}
	for _, m := range list.Data {
		ids[m.Id] = true
	}
	if ids["supported-hidden"] {
		t.Fatalf("expected unsupported model hidden from /v1/models, got list=%s", rec.Body.String())
	}
	if ids["supported-hidden-alias"] {
		t.Fatalf("expected unsupported model alias hidden from /v1/models, got list=%s", rec.Body.String())
	}
	if !ids["supported-visible"] {
		t.Fatalf("expected supported sibling model visible in /v1/models, got list=%s", rec.Body.String())
	}
}

// TestGatewayUnsupportedGeminiModelHiddenFromListing keeps the native Gemini
// models.list surface aligned with the same account availability rules used by
// OpenAI-compatible listings and scheduler candidates.
func TestGatewayUnsupportedGeminiModelHiddenFromListing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"supported-gemini-provider","display_name":"Supported Gemini Provider","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	hiddenModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"supported-gemini-hidden","display_name":"Supported Gemini Hidden","status":"active"}`)
	visibleModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"supported-gemini-visible","display_name":"Supported Gemini Visible","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(hiddenModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-hidden-upstream","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(visibleModel.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-visible-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"supported-gemini-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta","supported_models":["gemini-visible-upstream"]},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1beta/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /v1beta/models, got %d body=%s", rec.Code, rec.Body.String())
	}
	var list apiopenapi.GeminiModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode gemini model list: %v body=%s", err, rec.Body.String())
	}
	names := map[string]bool{}
	for _, model := range list.Models {
		names[model.Name] = true
	}
	if names["models/supported-gemini-hidden"] {
		t.Fatalf("expected unsupported Gemini model hidden from /v1beta/models, got list=%s", rec.Body.String())
	}
	if !names["models/supported-gemini-visible"] {
		t.Fatalf("expected supported Gemini sibling model visible in /v1beta/models, got list=%s", rec.Body.String())
	}

	getHidden := gatewayRequest(t, handler, apiKey, http.MethodGet, "/v1beta/models/supported-gemini-hidden", "")
	if getHidden.Code != http.StatusNotFound {
		t.Fatalf("expected unsupported Gemini model get 404, got %d body=%s", getHidden.Code, getHidden.Body.String())
	}
	getVisible := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1beta/models/supported-gemini-visible", "")
	if getVisible.Code != http.StatusOK {
		t.Fatalf("expected supported Gemini model get 200, got %d body=%s", getVisible.Code, getVisible.Body.String())
	}
}
