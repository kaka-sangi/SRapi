package translators

import (
	"encoding/json"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// modelAliasRewriter is the load-bearing helper — the translator + the
// per-request constructor both rely on its behaviour. These tests pin it
// directly so a refactor of the rewriter can't silently change semantics.

func TestModelAliasRewriterRewritesMatchedModel(t *testing.T) {
	aliases := map[string]string{"gpt-5-codex": "gpt-5.1-codex-internal"}
	in := []byte(`{"model":"gpt-5-codex","temperature":0.2}`)
	out := modelAliasRewriter("gpt-5-codex", in, aliases)

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("rewriter output not valid JSON: %v", err)
	}
	if got := parsed["model"]; got != "gpt-5.1-codex-internal" {
		t.Errorf("model = %v, want gpt-5.1-codex-internal", got)
	}
	if got := parsed["temperature"]; got != 0.2 {
		t.Errorf("temperature was lost in rewrite: %v", got)
	}
}

func TestModelAliasRewriterFallsThroughOnNoMatch(t *testing.T) {
	aliases := map[string]string{"gpt-5-codex": "gpt-5.1-codex-internal"}
	in := []byte(`{"model":"gpt-other"}`)
	out := modelAliasRewriter("gpt-other", in, aliases)
	if string(out) != string(in) {
		t.Errorf("expected fallthrough on no-match, got %q", out)
	}
}

func TestModelAliasRewriterFallsThroughOnEmptyAliases(t *testing.T) {
	in := []byte(`{"model":"gpt-5-codex"}`)
	if got := modelAliasRewriter("gpt-5-codex", in, nil); string(got) != string(in) {
		t.Errorf("expected fallthrough on nil aliases, got %q", got)
	}
	if got := modelAliasRewriter("gpt-5-codex", in, map[string]string{}); string(got) != string(in) {
		t.Errorf("expected fallthrough on empty alias map, got %q", got)
	}
}

func TestModelAliasRewriterFallsThroughOnMalformedJSON(t *testing.T) {
	aliases := map[string]string{"gpt-5-codex": "alt"}
	in := []byte(`not-json`)
	if got := modelAliasRewriter("gpt-5-codex", in, aliases); string(got) != string(in) {
		t.Errorf("expected fallthrough on bad JSON, got %q", got)
	}
}

func TestModelAliasRewriterUsesModelNameWhenBodyHasNoModelField(t *testing.T) {
	aliases := map[string]string{"gpt-5-codex": "alt"}
	in := []byte(`{"messages":[]}`)
	out := modelAliasRewriter("gpt-5-codex", in, aliases)
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if got := parsed["model"]; got != "alt" {
		t.Errorf("expected rewriter to use modelName arg when body had no model field, got %v", got)
	}
}

// Per-request constructor: registers a translator that uses the supplied
// alias map. The package-init identity registration is replaced for this
// registry instance; downstream call sites get the rewrite.
func TestRegisterOpenAIResponsesToCodexWithAliasesIsActive(t *testing.T) {
	r := translator.NewRegistry()
	RegisterOpenAIResponsesToCodexWithAliases(r, map[string]string{"gpt-5-codex": "alt"})

	in := []byte(`{"model":"gpt-5-codex"}`)
	out := r.TranslateRequest(
		translator.FormatOpenAIResponses,
		translator.FormatCodex,
		"gpt-5-codex", in, false,
	)
	var parsed map[string]any
	_ = json.Unmarshal(out, &parsed)
	if got := parsed["model"]; got != "alt" {
		t.Errorf("registry-mediated translate did not rewrite model, got %v", got)
	}
}

// The package init() registration on Default() registers an identity
// translator for the pair — proves the registry is consulted on the
// hot path even when no per-request alias map is supplied. The output
// must equal the input byte-for-byte.
func TestDefaultRegistryHasIdentityForOpenAIResponsesToCodex(t *testing.T) {
	r := translator.Default()
	in := []byte(`{"model":"gpt-5-codex","temperature":0.2}`)
	out := r.TranslateRequest(
		translator.FormatOpenAIResponses,
		translator.FormatCodex,
		"gpt-5-codex", in, false,
	)
	if string(out) != string(in) {
		t.Errorf("identity registration changed output: %q != %q", out, in)
	}
}
