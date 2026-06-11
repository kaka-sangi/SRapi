package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
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
		{name: "wildcard match", metadata: map[string]any{"model_mapping": map[string]any{"claude-*": "claude-default"}}, model: "claude-sonnet-4-5", want: "claude-default"},
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
