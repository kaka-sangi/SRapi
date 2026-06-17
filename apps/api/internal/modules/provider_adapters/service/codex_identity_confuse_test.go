package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCodexIdentityConfuseDisabledNoOp(t *testing.T) {
	body := []byte(`{"prompt_cache_key":"sess_orig"}`)
	out, state := ApplyCodexIdentityConfuseBody(CodexIdentityConfuseConfig{Enabled: false}, "auth-123", body, body)
	if state.Enabled {
		t.Fatal("state should be disabled")
	}
	if string(out) != string(body) {
		t.Fatalf("disabled body changed: %s", string(out))
	}
}

func TestCodexIdentityConfuseRewritesPromptCacheKeyDeterministically(t *testing.T) {
	cfg := CodexIdentityConfuseConfig{Enabled: true, SessionAffinity: true}
	body := []byte(`{"prompt_cache_key":"sess_orig","client_metadata":{"x-codex-installation-id":"install_orig","x-codex-window-id":"window_orig","x-codex-turn-metadata":"{\"prompt_cache_key\":\"sess_orig\",\"turn_id\":\"turn_orig\",\"window_id\":\"window_orig\"}"}}`)

	out1, state1 := ApplyCodexIdentityConfuseBody(cfg, "auth-A", body, body)
	out2, state2 := ApplyCodexIdentityConfuseBody(cfg, "auth-A", body, body)

	if state1.PromptCacheKey == "" || state1.PromptCacheKey == "sess_orig" {
		t.Fatalf("PromptCacheKey not rewritten: %q", state1.PromptCacheKey)
	}
	if state1.PromptCacheKey != state2.PromptCacheKey {
		t.Fatalf("rewrite not deterministic: %q vs %q", state1.PromptCacheKey, state2.PromptCacheKey)
	}
	if string(out1) != string(out2) {
		t.Fatalf("body rewrite not deterministic")
	}

	var rewritten map[string]any
	if err := json.Unmarshal(out1, &rewritten); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rewritten["prompt_cache_key"] != state1.PromptCacheKey {
		t.Fatalf("prompt_cache_key not in rewritten body: %+v", rewritten)
	}
	metadata := rewritten["client_metadata"].(map[string]any)
	if metadata["x-codex-installation-id"] == "install_orig" {
		t.Fatalf("installation_id not rewritten: %+v", metadata)
	}
	if metadata["x-codex-window-id"] != state1.PromptCacheKey+":0" {
		t.Fatalf("window_id not rewritten: %v", metadata["x-codex-window-id"])
	}
}

func TestCodexIdentityConfuseIsolatedAcrossAuths(t *testing.T) {
	cfg := CodexIdentityConfuseConfig{Enabled: true, SessionAffinity: true}
	body := []byte(`{"prompt_cache_key":"sess_orig"}`)
	_, stateA := ApplyCodexIdentityConfuseBody(cfg, "auth-A", body, body)
	_, stateB := ApplyCodexIdentityConfuseBody(cfg, "auth-B", body, body)
	if stateA.PromptCacheKey == stateB.PromptCacheKey {
		t.Fatal("rewrites must be isolated per auth")
	}
}

func TestCodexIdentityConfuseExposeRestoresClientIdentifier(t *testing.T) {
	state := CodexIdentityConfuseState{
		Enabled:                true,
		AuthID:                 "auth-A",
		OriginalPromptCacheKey: "sess_orig",
		PromptCacheKey:         "sess_rewritten",
		TurnIDs:                []CodexIdentityReplacement{{Original: "turn_orig", Confused: "turn_rewritten"}},
	}
	upstream := []byte(`{"prompt_cache_key":"sess_rewritten","turn_id":"turn_rewritten"}`)
	exposed := ApplyCodexIdentityExposeResponsePayload(upstream, state)
	if !strings.Contains(string(exposed), "sess_orig") {
		t.Fatalf("expose did not restore prompt_cache_key: %s", string(exposed))
	}
	if !strings.Contains(string(exposed), "turn_orig") {
		t.Fatalf("expose did not restore turn_id: %s", string(exposed))
	}
}

func TestCodexIdentityConfuseHeadersRewrite(t *testing.T) {
	state := &CodexIdentityConfuseState{
		Enabled:        true,
		AuthID:         "auth-A",
		PromptCacheKey: "sess_rewritten",
	}
	headers := http.Header{}
	headers.Set("X-Codex-Turn-Metadata", `{"prompt_cache_key":"sess_orig","turn_id":"turn_orig"}`)
	headers.Set("Session_id", "sess_orig")
	headers.Set("Conversation_id", "sess_orig")
	ApplyCodexIdentityConfuseHeaders(headers, state)
	if headers.Get("X-Client-Request-Id") != "sess_rewritten" {
		t.Fatalf("X-Client-Request-Id not set: %v", headers.Get("X-Client-Request-Id"))
	}
	if headers.Get("X-Codex-Window-Id") != "sess_rewritten:0" {
		t.Fatalf("X-Codex-Window-Id not set: %v", headers.Get("X-Codex-Window-Id"))
	}
	if !strings.Contains(headers.Get("X-Codex-Turn-Metadata"), "sess_rewritten") {
		t.Fatalf("turn metadata not rewritten: %v", headers.Get("X-Codex-Turn-Metadata"))
	}
}

func TestCodexIdentityConfuseTurnIDStable(t *testing.T) {
	state := &CodexIdentityConfuseState{Enabled: true, AuthID: "auth-A"}
	got1 := state.ConfuseTurnID("turn_orig")
	got2 := state.ConfuseTurnID("turn_orig")
	if got1 != got2 {
		t.Fatalf("turn rewrite not stable: %q vs %q", got1, got2)
	}
	if got1 == "turn_orig" {
		t.Fatalf("turn id not rewritten: %q", got1)
	}
	// Idempotent on already-rewritten value.
	if state.ConfuseTurnID(got1) != got1 {
		t.Fatal("rewrite not idempotent on rewritten value")
	}
}

func TestCodexIdentityConfuseEnabled(t *testing.T) {
	if CodexIdentityConfuseEnabled(CodexIdentityConfuseConfig{Enabled: false}) {
		t.Fatal("disabled config must not enable")
	}
	if !CodexIdentityConfuseEnabled(CodexIdentityConfuseConfig{Enabled: true, SessionAffinity: true}) {
		t.Fatal("session affinity should enable")
	}
	if !CodexIdentityConfuseEnabled(CodexIdentityConfuseConfig{Enabled: true, RoutingStrategy: "fill-first"}) {
		t.Fatal("fill-first should enable")
	}
	if !CodexIdentityConfuseEnabled(CodexIdentityConfuseConfig{Enabled: true, RoutingStrategy: "ff"}) {
		t.Fatal("ff should enable")
	}
	if CodexIdentityConfuseEnabled(CodexIdentityConfuseConfig{Enabled: true, RoutingStrategy: "round-robin"}) {
		t.Fatal("round-robin should NOT enable")
	}
}
