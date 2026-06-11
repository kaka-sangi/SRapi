package service

import (
	"testing"

	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestCodexResponsesPayloadStripsUnsupportedCompatibilityFields(t *testing.T) {
	payload, stream, err := codexResponsesPayload(contract.ConversationRequest{
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/responses",
		RawBody: []byte(`{
			"model":"codex-local",
			"input":"hello",
			"context_management":{"strategy":"auto"},
			"truncation":"auto",
			"max_output_tokens":64,
			"temperature":0.2,
			"stream":false
		}`),
		Mapping: modelcontract.ModelProviderMapping{UpstreamModelName: "codex-upstream"},
	})
	if err != nil {
		t.Fatalf("build codex responses payload: %v", err)
	}
	if !stream {
		t.Fatal("codex responses payload should stream by default")
	}
	if payload["model"] != "codex-upstream" {
		t.Fatalf("model = %v, want codex-upstream", payload["model"])
	}
	for _, removed := range []string{"context_management", "truncation", "max_output_tokens", "temperature"} {
		if _, ok := payload[removed]; ok {
			t.Fatalf("expected %s to be removed, got %+v", removed, payload)
		}
	}
}
