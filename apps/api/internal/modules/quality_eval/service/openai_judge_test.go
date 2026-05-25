package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
)

func TestOpenAIJudgeUsesJSONModeAndParsesRubric(t *testing.T) {
	var request chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected judge path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer judge-key" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode judge request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"{\"correctness\":5,\"coherence\":4,\"safety\":3,\"rationale\":\"usable\"}"}}]}`))
	}))
	defer server.Close()

	judge, err := NewOpenAIJudge(OpenAIJudgeConfig{APIKey: "judge-key", BaseURL: server.URL, Model: "gpt-4o-mini", Client: server.Client()})
	if err != nil {
		t.Fatalf("new judge: %v", err)
	}
	result, err := judge.Evaluate(t.Context(), qualitycontract.EvaluationSample{
		RequestID:       "req_judge",
		SanitizedPrompt: "prompt",
		SanitizedOutput: "output",
	})
	if err != nil {
		t.Fatalf("evaluate sample: %v", err)
	}
	if request.Model != "gpt-4o-mini" || request.ResponseFormat["type"] != "json_object" {
		t.Fatalf("unexpected judge request: %+v", request)
	}
	if len(request.Messages) != 2 || request.Messages[0].Role != "system" {
		t.Fatalf("expected system and user judge messages, got %+v", request.Messages)
	}
	if result.JudgeModel != "gpt-4o-mini" || result.Score != 0.8 || result.Correctness != 5 || result.Coherence != 4 || result.Safety != 3 {
		t.Fatalf("unexpected judge result: %+v", result)
	}
}
