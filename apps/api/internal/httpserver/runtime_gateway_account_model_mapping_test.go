package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

// TestAccountModelOverride covers the per-account model_mapping resolver guards:
// only a valid, non-blank string override for the requested model applies.
func TestAccountModelOverride(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		model    string
		want     string
	}{
		{name: "nil metadata", metadata: nil, model: "gpt-4o", want: ""},
		{name: "no mapping key", metadata: map[string]any{"base_url": "x"}, model: "gpt-4o", want: ""},
		{name: "mapping wrong type", metadata: map[string]any{"model_mapping": "nope"}, model: "gpt-4o", want: ""},
		{name: "model not mapped", metadata: map[string]any{"model_mapping": map[string]any{"claude": "c"}}, model: "gpt-4o", want: ""},
		{name: "override non-string", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": 5}}, model: "gpt-4o", want: ""},
		{name: "override blank", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "  "}}, model: "gpt-4o", want: ""},
		{name: "blank model", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "up"}}, model: "  ", want: ""},
		{name: "match trims", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "  gpt-4o-2024-11-20 "}}, model: "gpt-4o", want: "gpt-4o-2024-11-20"},
		{name: "map string string form", metadata: map[string]any{"model_mapping": map[string]string{"gpt-4o": "gpt-4o-upstream"}}, model: "gpt-4o", want: "gpt-4o-upstream"},
		{name: "case insensitive exact match", metadata: map[string]any{"model_mapping": map[string]any{"GPT-4O": "upstream-case"}}, model: "gpt-4o", want: "upstream-case"},
		{name: "suffix preserved for exact match", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "upstream-case"}}, model: "gpt-4o(high)", want: "upstream-case(high)"},
		{name: "exact suffix key wins", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "base-upstream", "gpt-4o(high)": "explicit-upstream"}}, model: "gpt-4o(high)", want: "explicit-upstream"},
		{name: "target suffix wins for exact match", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "upstream-case(medium)"}}, model: "gpt-4o(high)", want: "upstream-case(medium)"},
		{name: "empty suffix ignored", metadata: map[string]any{"model_mapping": map[string]any{"gpt-4o": "upstream-case"}}, model: "gpt-4o()", want: ""},
		{name: "wildcard match", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": "claude-default"}}, model: "claude-sonnet-4-5", want: "claude-default"},
		{name: "suffix preserved for wildcard match", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": "claude-default"}}, model: "claude-sonnet-4-5(8192)", want: "claude-default(8192)"},
		{name: "wildcard is case insensitive", metadata: map[string]any{"model_mapping": map[string]any{"CLAUDE-*": "claude-default"}}, model: "claude-opus-4-5", want: "claude-default"},
		{name: "longest wildcard wins", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": "claude-default", "claude-sonnet-*": "claude-sonnet", "claude-sonnet-4*": "claude-sonnet-4"}}, model: "claude-sonnet-4-5", want: "claude-sonnet-4"},
		{name: "exact beats wildcard", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": "claude-default", "claude-sonnet-4-5": "claude-exact"}}, model: "claude-sonnet-4-5", want: "claude-exact"},
		{name: "blank wildcard override skipped", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": "  ", "gemini-*": "gemini-up"}}, model: "claude-sonnet-4-5", want: ""},
		{name: "non-string wildcard override skipped", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": 12}}, model: "claude-sonnet-4-5", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accountModelOverride(accountcontract.ProviderAccount{Metadata: tc.metadata}, tc.model, "/v1/chat/completions")
			if got != tc.want {
				t.Fatalf("accountModelOverride = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAccountCompactModelOverride(t *testing.T) {
	account := accountcontract.ProviderAccount{Metadata: map[string]any{
		"model_mapping": map[string]any{
			"gpt-*": "gpt-normal",
		},
		"compact_model_mapping": map[string]any{
			"gpt-5.*":      "gpt-5-compact",
			"gpt-5.4-mini": "gpt-5.4-mini-compact",
		},
	}}

	if got := accountModelOverride(account, "gpt-5.4-mini", "/v1/responses/compact"); got != "gpt-5.4-mini-compact" {
		t.Fatalf("compact exact override = %q, want %q", got, "gpt-5.4-mini-compact")
	}
	if got := accountModelOverride(account, "gpt-5.4", "/v1/responses/compact"); got != "gpt-5-compact" {
		t.Fatalf("compact wildcard override = %q, want %q", got, "gpt-5-compact")
	}
	if got := accountModelOverride(account, "gpt-5.4", "/v1/responses"); got != "gpt-normal" {
		t.Fatalf("normal responses should use model_mapping, got %q", got)
	}
	if got := accountModelOverride(account, "gpt-5.4", "/v1/chat/completions"); got != "gpt-normal" {
		t.Fatalf("chat should use model_mapping, got %q", got)
	}

	stringMapAccount := accountcontract.ProviderAccount{Metadata: map[string]any{
		"compact_model_mapping": map[string]string{"gpt-*": "gpt-string-map-compact"},
	}}
	if got := accountModelOverride(stringMapAccount, "gpt-5.4", "/v1/responses/compact"); got != "gpt-string-map-compact" {
		t.Fatalf("compact map[string]string override = %q, want %q", got, "gpt-string-map-compact")
	}
}

func TestAccountSupportsUpstreamModelWildcard(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		model    string
		want     bool
	}{
		{name: "no allowlist allows all", metadata: nil, model: "gpt-4o", want: true},
		{name: "empty allowlist rejects", metadata: map[string]any{"supported_models": []any{}}, model: "gpt-4o", want: false},
		{name: "exact still matches", metadata: map[string]any{"supported_models": []any{"gpt-4o"}}, model: "models/gpt-4o", want: true},
		{name: "wildcard matches normalized model", metadata: map[string]any{"supported_models": []any{"claude-*"}}, model: "models/claude-sonnet-4-5", want: true},
		{name: "wildcard is case insensitive", metadata: map[string]any{"supported_models": []any{"GEMINI-3.*"}}, model: "gemini-3.1-pro-high", want: true},
		{name: "wildcard miss rejects", metadata: map[string]any{"supported_models": []any{"claude-*"}}, model: "gemini-3.1-pro-high", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accountSupportsUpstreamModel(tc.metadata, tc.model)
			if got != tc.want {
				t.Fatalf("accountSupportsUpstreamModel = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProviderSupportsUpstreamModelWildcard(t *testing.T) {
	cases := []struct {
		name         string
		configSchema map[string]any
		model        string
		want         bool
	}{
		{name: "no allowlist allows all", configSchema: nil, model: "gpt-4o", want: true},
		{name: "empty allowlist rejects", configSchema: map[string]any{"supported_models": []any{}}, model: "gpt-4o", want: false},
		{name: "exact still matches", configSchema: map[string]any{"supported_models": []any{"gpt-4o"}}, model: "models/gpt-4o", want: true},
		{name: "wildcard matches normalized model", configSchema: map[string]any{"supported_models": []any{"claude-*"}}, model: "models/claude-sonnet-4-5", want: true},
		{name: "hyphen key matches", configSchema: map[string]any{"supported-models": []any{"o3-*"}}, model: "o3-mini", want: true},
		{name: "wildcard miss rejects", configSchema: map[string]any{"supported_models": []any{"claude-*"}}, model: "gemini-3.1-pro-high", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := providerSupportsUpstreamModel(tc.configSchema, tc.model)
			if got != tc.want {
				t.Fatalf("providerSupportsUpstreamModel = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAccountRoutableForModelCodexBypassesAllowlist proves the fix for free
// Codex accounts: a Codex CLI provider forwards regardless of the discovery
// derived supported_models allowlist (mirroring sub2api — the upstream decides),
// while every other adapter type keeps enforcing the allowlist.
func TestAccountRoutableForModelCodexBypassesAllowlist(t *testing.T) {
	codex := providercontract.Provider{AdapterType: "reverse-proxy-codex-cli"}
	codexProviderAllowlist := providercontract.Provider{
		AdapterType:  "reverse-proxy-codex-cli",
		ConfigSchema: map[string]any{"supported_models": []any{"gpt-5.4*"}},
	}
	codexCased := providercontract.Provider{AdapterType: "  Reverse-Proxy-Codex-CLI "}
	antigravity := providercontract.Provider{AdapterType: "reverse-proxy-antigravity"}
	plain := providercontract.Provider{AdapterType: "openai-compatible"}

	// supported_models discovered for a free account: gpt-5.5 is NOT advertised.
	freeAllowlist := map[string]any{"supported_models": []any{"gpt-5.4", "gpt-5.4-mini"}}
	emptyAllowlist := map[string]any{"supported_models": []any{}}

	cases := []struct {
		name     string
		provider providercontract.Provider
		metadata map[string]any
		model    string
		want     bool
	}{
		{name: "codex bypasses discovery allowlist for gpt-5.5", provider: codex, metadata: freeAllowlist, model: "gpt-5.5", want: true},
		{name: "codex bypass is case/space insensitive", provider: codexCased, metadata: freeAllowlist, model: "gpt-5.5", want: true},
		{name: "codex bypasses even an empty allowlist", provider: codex, metadata: emptyAllowlist, model: "gpt-5.5", want: true},
		{name: "codex still honors provider allowlist miss", provider: codexProviderAllowlist, metadata: freeAllowlist, model: "gpt-5.5", want: false},
		{name: "codex honors provider allowlist hit", provider: codexProviderAllowlist, metadata: freeAllowlist, model: "gpt-5.4-mini", want: true},
		{name: "antigravity still enforces allowlist (miss)", provider: antigravity, metadata: freeAllowlist, model: "gpt-5.5", want: false},
		{name: "antigravity still enforces allowlist (hit)", provider: antigravity, metadata: freeAllowlist, model: "gpt-5.4", want: true},
		{name: "plain adapter still enforces allowlist", provider: plain, metadata: freeAllowlist, model: "gpt-5.5", want: false},
		{name: "non-codex with no allowlist allows all", provider: plain, metadata: nil, model: "gpt-5.5", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accountRoutableForModel(tc.provider, accountcontract.ProviderAccount{Metadata: tc.metadata}, tc.model)
			if got != tc.want {
				t.Fatalf("accountRoutableForModel = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestGatewayAppliesAccountModelMapping proves the wire effect: an account whose
// metadata.model_mapping remaps the catalog model overrides the channel's
// default upstream_model_name in the actual upstream request body.
func TestGatewayAppliesAccountModelMapping(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"map-provider","display_name":"Map Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"map-model","display_name":"Map Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	// Channel default maps the catalog model to "provider-default-upstream"...
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-default-upstream","status":"active"}`)
	// ...but this account only supports the mapped upstream model. Candidate
	// filtering must evaluate supported_models after model_mapping is applied.
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"map-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","supported_models":["account-override-upstream"],"model_mapping":{"map-model":"account-override-upstream"}},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"map-model","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	if doc["model"] != "account-override-upstream" {
		t.Fatalf("expected per-account model override in upstream body, got model=%v body=%s", doc["model"], sent)
	}
}

func TestGatewaySupportedModelsWildcardUsesMappedUpstreamModel(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wildcard-supported-provider","display_name":"Wildcard Supported Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wildcard-map-model","display_name":"Wildcard Map Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-default-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wildcard-supported-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","supported_models":["account-override-*"],"model_mapping":{"wildcard-map-model":"account-override-2026"}},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"wildcard-map-model","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	if doc["model"] != "account-override-2026" {
		t.Fatalf("expected mapped upstream model through wildcard allowlist, got model=%v body=%s", doc["model"], sent)
	}
}

func TestGatewayDisabledModelMappingIsNotScheduled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("disabled mapping should not reach upstream path=%s", r.URL.Path)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"disabled-map-provider","display_name":"Disabled Map Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"disabled-map-model","display_name":"Disabled Map Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"disabled-map-upstream","status":"disabled"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"disabled-map-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"disabled-map-model","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected disabled mapping to produce no_available_account 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGatewayResponsesCompactAppliesCompactModelMapping proves compact-only
// parity with sub2api: account metadata.compact_model_mapping affects
// /v1/responses/compact but does not require changing the catalog mapping.
func TestGatewayResponsesCompactAppliesCompactModelMapping(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses/compact" {
			t.Fatalf("expected compact upstream path, got %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmp_1","object":"response.compaction","input_tokens":9,"output_tokens":2}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"compact-map-provider","display_name":"Compact Map Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"responses_compact":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"compact-map-model","display_name":"Compact Map Model","status":"active","capabilities":[{"key":"responses_compact","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"provider-default-compact","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"compact-map-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","model_mapping":{"compact-map-model":"normal-account-upstream"},"compact_model_mapping":{"compact-*":"compact-account-upstream"}},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses/compact", `{"model":"compact-map-model","input":"compact me","stream":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a compact request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream compact body %q: %v", sent, err)
	}
	if doc["model"] != "compact-account-upstream" {
		t.Fatalf("expected compact model override in upstream body, got model=%v body=%s", doc["model"], sent)
	}
}

func TestGatewayCodexNormalizesUpstreamModelBeforeAccountFiltering(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"codex alias ok\"}]}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"codex-alias-provider","display_name":"Codex Alias Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-alias-model","display_name":"Codex Alias Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"openai/gpt5.4mini-openai-compact","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"codex-alias-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","supported_models":["gpt-5.4-mini"]},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"codex-alias-model","input":"hello"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sent []byte
	select {
	case sent = <-bodyCh:
	default:
		t.Fatal("upstream did not receive a request body")
	}
	var doc map[string]any
	if err := json.Unmarshal(sent, &doc); err != nil {
		t.Fatalf("decode upstream body %q: %v", sent, err)
	}
	if doc["model"] != "gpt-5.4-mini" {
		t.Fatalf("expected normalized codex upstream model, got model=%v body=%s", doc["model"], sent)
	}
}
