package service

import (
	"encoding/json"
	"testing"

	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestApplyPayloadTransforms(t *testing.T) {
	base := []byte(`{"reasoning":{"effort":"low"},"max_tokens":100,"temperature":0.7}`)
	transforms := []contract.PayloadTransform{
		{Action: "override", Path: "reasoning.effort", Value: "high"},                      // overwrite nested
		{Action: "default", Path: "top_p", Value: 0.9},                                     // absent -> set
		{Action: "default", Path: "max_tokens", Value: 999},                                // present -> keep
		{Action: "override", Path: "generationConfig.thinkingConfig.budget", Value: 32768}, // create deep
		{Action: "filter", Path: "temperature"},                                            // remove
	}
	out, err := applyPayloadTransforms(base, transforms)
	if err != nil {
		t.Fatalf("applyPayloadTransforms: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	reasoning, _ := doc["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("override failed: reasoning.effort = %v", reasoning["effort"])
	}
	if doc["top_p"] != 0.9 {
		t.Fatalf("default(absent) failed: top_p = %v", doc["top_p"])
	}
	if doc["max_tokens"] != float64(100) {
		t.Fatalf("default(present) clobbered: max_tokens = %v", doc["max_tokens"])
	}
	gc, _ := doc["generationConfig"].(map[string]any)
	tc, _ := gc["thinkingConfig"].(map[string]any)
	if tc["budget"] != float64(32768) {
		t.Fatalf("deep override failed: budget = %v", tc["budget"])
	}
	if _, ok := doc["temperature"]; ok {
		t.Fatalf("filter failed: temperature still present")
	}
}

func TestRawSameProtocolPayloadAppliesCanonicalReasoning(t *testing.T) {
	req := contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Stream:         true,
		RawBody:        []byte(`{"model":"caller-model","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"low"}`),
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: "upstream-model"},
		Reasoning:      map[string]any{"effort": "high"},
	}

	payload, ok, err := rawSameProtocolPayload(req, rawEndpointOpenAIChatCompletions)
	if err != nil {
		t.Fatalf("rawSameProtocolPayload: %v", err)
	}
	if !ok {
		t.Fatal("expected raw payload to be used")
	}
	if payload["model"] != "upstream-model" || payload["stream"] != true || payload["reasoning_effort"] != "high" {
		t.Fatalf("expected upstream model and canonical reasoning, got %+v", payload)
	}
}

func TestApplyPayloadTransformsNoOpAndNonObject(t *testing.T) {
	base := []byte(`{"a":1}`)
	// No transforms -> identical bytes returned.
	out, err := applyPayloadTransforms(base, nil)
	if err != nil || string(out) != string(base) {
		t.Fatalf("expected no-op passthrough, got %s err=%v", out, err)
	}
	// Non-object body -> left untouched.
	arr := []byte(`[1,2,3]`)
	out, err = applyPayloadTransforms(arr, []contract.PayloadTransform{{Action: "override", Path: "x", Value: 1}})
	if err != nil || string(out) != string(arr) {
		t.Fatalf("expected non-object passthrough, got %s err=%v", out, err)
	}
}
