package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	payloadrulescontract "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/contract"
	payloadrulesmemory "github.com/srapi/srapi/apps/api/internal/modules/payload_rules/store/memory"
)

// TestGatewayAppliesPayloadOverrideRule proves the end-to-end path: an operator
// "override" rule mutates the marshaled upstream request body before dispatch.
func TestGatewayAppliesPayloadOverrideRule(t *testing.T) {
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

	// Seed an override rule that forces reasoning.effort=high on every
	// openai-compatible upstream request.
	payloadStore := payloadrulesmemory.New()
	if _, err := payloadStore.CreateRule(context.Background(), payloadrulescontract.CreateRule{
		Name:          "force-high-effort",
		Enabled:       true,
		Action:        payloadrulescontract.ActionOverride,
		MatchModel:    "*",
		MatchProtocol: "openai-compatible",
		Params:        map[string]any{"reasoning.effort": "high"},
	}); err != nil {
		t.Fatalf("seed payload rule: %v", err)
	}

	handler := New(config.Load(), nil, WithPayloadRulesStore(payloadStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"transform-provider","display_name":"Transform Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"transform-model","display_name":"Transform Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"transform-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"transform-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"transform-model","messages":[{"role":"user","content":"hello"}]}`)
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
	reasoning, _ := doc["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("expected payload override reasoning.effort=high in upstream body, got %s", sent)
	}
}
