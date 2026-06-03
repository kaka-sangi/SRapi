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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accountModelOverride(accountcontract.ProviderAccount{Metadata: tc.metadata}, tc.model)
			if got != tc.want {
				t.Fatalf("accountModelOverride = %q, want %q", got, tc.want)
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
	// ...but this account overrides it to "account-override-upstream".
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"map-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","model_mapping":{"map-model":"account-override-upstream"}},"status":"active"}`)
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
