package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestGatewayCodexClientModelsQueryUsesCodexCatalogShape(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken
	_ = mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"codex-client-custom-model","display_name":"Codex Client Custom","context_window":123456,"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1/models?client_version=0.124.0", "")
	if jsonBody := rec.Body.String(); jsonContainsKey(t, jsonBody, "object") || jsonContainsKey(t, jsonBody, "data") {
		t.Fatalf("codex client model catalog should not use OpenAI object/data shape: %s", jsonBody)
	}
	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode codex client model catalog: %v", err)
	}
	custom := findCodexClientModel(t, payload.Models, "codex-client-custom-model")
	if custom["display_name"] != "Codex Client Custom" ||
		custom["description"] != "Codex Client Custom" ||
		custom["prefer_websockets"] != false ||
		custom["priority"] != float64(100) ||
		custom["context_window"] != float64(123456) ||
		custom["max_context_window"] != float64(123456) {
		t.Fatalf("unexpected custom codex client model metadata: %+v", custom)
	}
	if _, ok := custom["apply_patch_tool_type"]; ok {
		t.Fatalf("custom codex client model must not advertise apply_patch_tool_type: %+v", custom)
	}
	if _, ok := custom["availability_nux"]; ok {
		t.Fatalf("custom codex client model must not advertise availability_nux: %+v", custom)
	}
	if custom["base_instructions"] == "" {
		t.Fatalf("expected custom codex client model to include base instructions: %+v", custom)
	}
	if plans, ok := custom["available_in_plans"].([]any); !ok || len(plans) == 0 {
		t.Fatalf("expected custom codex client model plans: %+v", custom)
	}

	openAIRec := mustGatewayRequest(t, handler, apiKey, http.MethodGet, "/v1/models", "")
	var openAIList apiopenapi.OpenAIModelList
	if err := json.NewDecoder(openAIRec.Body).Decode(&openAIList); err != nil {
		t.Fatalf("decode openai model list: %v", err)
	}
	if openAIList.Object != apiopenapi.OpenAIModelListObjectList || len(openAIList.Data) == 0 {
		t.Fatalf("plain /v1/models should keep OpenAI list shape: %+v", openAIList)
	}
}

func TestCodexClientModelCatalogTemplatesAndHiddenMediaModels(t *testing.T) {
	models := buildCodexClientModels([]map[string]any{
		{"id": "gpt-5.5"},
		{"id": "grok-imagine-video"},
	})
	gpt55 := findCodexClientModel(t, models, "gpt-5.5")
	if gpt55["minimal_client_version"] != "0.124.0" ||
		gpt55["prefer_websockets"] != true ||
		gpt55["apply_patch_tool_type"] != "freeform" ||
		gpt55["default_reasoning_level"] != "medium" {
		t.Fatalf("unexpected gpt-5.5 codex client template: %+v", gpt55)
	}
	if tiers, ok := gpt55["service_tiers"].([]map[string]any); ok {
		if len(tiers) != 1 || tiers[0]["id"] != "priority" {
			t.Fatalf("unexpected gpt-5.5 service tiers: %+v", tiers)
		}
	} else if tiers, ok := gpt55["service_tiers"].([]any); !ok || len(tiers) != 1 {
		t.Fatalf("unexpected gpt-5.5 service tiers: %+v", gpt55["service_tiers"])
	}
	video := findCodexClientModel(t, models, "grok-imagine-video")
	if video["visibility"] != "hide" || video["prefer_websockets"] != false {
		t.Fatalf("expected media model hidden with custom-model defaults: %+v", video)
	}
}

func findCodexClientModel(t *testing.T, models []map[string]any, slug string) map[string]any {
	t.Helper()
	for _, model := range models {
		if model["slug"] == slug {
			return model
		}
	}
	t.Fatalf("missing codex client model %q in %+v", slug, models)
	return nil
}

func jsonContainsKey(t *testing.T, raw string, key string) bool {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	_, ok := doc[key]
	return ok
}
