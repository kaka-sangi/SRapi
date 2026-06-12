package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

func TestApplyGatewayModelSuffixStripsUnknownSuffixFromRawModel(t *testing.T) {
	canonical := gatewaycontract.CanonicalRequest{
		SourceProtocol: gatewaycontract.ProtocolOpenAICompatible,
		SourceEndpoint: "/v1/chat/completions",
		RawBody:        []byte(`{"model":"suffix-model(custom)","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"low"}`),
	}

	applyGatewayModelSuffix(&canonical, gatewayModelSuffixFromModel("suffix-model(custom)"))

	var doc map[string]any
	if err := json.Unmarshal(canonical.RawBody, &doc); err != nil {
		t.Fatalf("decode raw body: %v", err)
	}
	if doc["model"] != "suffix-model" {
		t.Fatalf("expected raw model suffix to be stripped, got body=%s", canonical.RawBody)
	}
	if doc["reasoning_effort"] != "low" {
		t.Fatalf("expected unknown suffix not to rewrite reasoning, got body=%s", canonical.RawBody)
	}
	if len(canonical.Reasoning) != 0 || len(canonical.RequestCapabilities) != 0 {
		t.Fatalf("expected unknown suffix not to add reasoning metadata, got reasoning=%v capabilities=%v", canonical.Reasoning, canonical.RequestCapabilities)
	}
}

func TestGatewayChatCompletionModelSuffixAppliesReasoning(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_suffix","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"suffix-chat-provider","display_name":"Suffix Chat Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"suffix-chat-model","display_name":"Suffix Chat Model","status":"active","capabilities":[{"key":"reasoning_control","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"suffix-upstream-model","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"suffix-chat-account","runtime_class":"api_key","credential":{"api_key":"suffix-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"suffix-chat-model(high)","messages":[{"role":"user","content":"hello"}]}`)
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
	if doc["model"] != "suffix-upstream-model" {
		t.Fatalf("expected upstream model without suffix, got model=%v body=%s", doc["model"], sent)
	}
	if doc["reasoning_effort"] != "high" {
		t.Fatalf("expected suffix to set reasoning_effort, got body=%s", sent)
	}
}

func TestGatewayResponsesModelSuffixAppliesReasoning(t *testing.T) {
	bodyCh := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		select {
		case bodyCh <- raw:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_suffix","object":"response","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"suffix-responses-provider","display_name":"Suffix Responses Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"responses":true,"reasoning_control":true},"config_schema":{"responses_passthrough":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"suffix-responses-model","display_name":"Suffix Responses Model","status":"active","capabilities":[{"key":"responses","level":"required","status":"stable","version":"v1"},{"key":"reasoning_control","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"suffix-responses-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"suffix-responses-account","runtime_class":"api_key","credential":{"api_key":"suffix-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"suffix-responses-model(8192)","input":"hello","stream":false}`)
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
	if doc["model"] != "suffix-responses-upstream" {
		t.Fatalf("expected upstream model without suffix, got model=%v body=%s", doc["model"], sent)
	}
	reasoning, _ := doc["reasoning"].(map[string]any)
	if reasoning["budget_tokens"] != float64(8192) || reasoning["type"] != "enabled" {
		t.Fatalf("expected suffix to set reasoning budget, got body=%s", sent)
	}
}
