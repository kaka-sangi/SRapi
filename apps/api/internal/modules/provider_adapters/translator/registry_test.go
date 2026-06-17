package translator

import (
	"context"
	"testing"
)

// Foundation layer for the cross-format payload registry. Per-pair
// translators live in translators/ subpackages and are tested there; this
// file pins the registry's own contract — Register / Lookup / fallthrough
// semantics / HasResponseTransformer / identity short-circuit. Any future
// change to the registry's behavioural envelope (e.g. tightening fallthrough
// to error on missing pairs) must update these tests deliberately.

func TestFormatFromStringNormalisesCase(t *testing.T) {
	cases := map[string]Format{
		"codex":            FormatCodex,
		"CODEX":            FormatCodex,
		" codex ":          FormatCodex,
		"openai_responses": FormatOpenAIResponses,
		"":                 Format(""),
		"future_ollama":    Format("future_ollama"),
	}
	for in, want := range cases {
		if got := FormatFromString(in); got != want {
			t.Errorf("FormatFromString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPairValidAndIdentity(t *testing.T) {
	cases := []struct {
		name     string
		pair     Pair
		valid    bool
		identity bool
	}{
		{"normal", Pair{FormatCodex, FormatOpenAIResponses}, true, false},
		{"identity", Pair{FormatCodex, FormatCodex}, true, true},
		{"empty_from", Pair{Format(""), FormatCodex}, false, false},
		{"empty_to", Pair{FormatCodex, Format("")}, false, false},
		{"both_empty", Pair{Format(""), Format("")}, false, true}, // identity over empty is true; that's fine — Valid() gates the real cases.
	}
	for _, tc := range cases {
		if tc.pair.Valid() != tc.valid {
			t.Errorf("%s: Valid() = %v, want %v", tc.name, tc.pair.Valid(), tc.valid)
		}
		if tc.pair.Identity() != tc.identity {
			t.Errorf("%s: Identity() = %v, want %v", tc.name, tc.pair.Identity(), tc.identity)
		}
	}
}

func TestPairStringIsStableForMetrics(t *testing.T) {
	p := Pair{FormatCodex, FormatOpenAIResponses}
	if p.String() != "codex->openai_responses" {
		t.Errorf("Pair.String() = %q, want %q", p.String(), "codex->openai_responses")
	}
}

// Register installs a translator under a pair; Lookup returns it.
// Re-registering overwrites — same as CLIProxyAPI's sdk registry contract.
func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(FormatCodex, FormatOpenAIResponses,
		func(_ string, raw []byte, _ bool) []byte { return append([]byte("A:"), raw...) },
		nil,
	)
	t1, ok := r.Lookup(FormatCodex, FormatOpenAIResponses)
	if !ok {
		t.Fatal("expected lookup hit after Register")
	}
	if got := t1.Request("m", []byte("x"), false); string(got) != "A:x" {
		t.Errorf("first-registration Request output = %q, want %q", got, "A:x")
	}

	// Overwrite — second Register replaces.
	r.Register(FormatCodex, FormatOpenAIResponses,
		func(_ string, raw []byte, _ bool) []byte { return append([]byte("B:"), raw...) },
		nil,
	)
	t2, _ := r.Lookup(FormatCodex, FormatOpenAIResponses)
	if got := t2.Request("m", []byte("x"), false); string(got) != "B:x" {
		t.Errorf("re-registration Request output = %q, want %q", got, "B:x")
	}
}

// Calls with an empty Format are silently dropped — defends against a
// future translator init() block fed an empty config value.
func TestRegistryRegisterIgnoresInvalidPair(t *testing.T) {
	r := NewRegistry()
	r.Register(Format(""), FormatOpenAIResponses, func(string, []byte, bool) []byte { return nil }, nil)
	if _, ok := r.Lookup(Format(""), FormatOpenAIResponses); ok {
		t.Error("expected invalid pair to be silently rejected")
	}
}

// Missing translator: TranslateRequest returns the input unchanged.
// Identity pair: same fallthrough. nil/empty body: unchanged.
func TestRegistryTranslateRequestFallsThroughOnMiss(t *testing.T) {
	r := NewRegistry()
	in := []byte(`{"hello":"world"}`)
	out := r.TranslateRequest(FormatCodex, FormatOpenAIResponses, "m", in, false)
	if string(out) != string(in) {
		t.Errorf("expected fallthrough on missing translator, got %q", out)
	}
	out = r.TranslateRequest(FormatCodex, FormatCodex, "m", in, false)
	if string(out) != string(in) {
		t.Errorf("expected identity fallthrough, got %q", out)
	}
	if got := r.TranslateRequest(FormatCodex, FormatOpenAIResponses, "m", nil, false); got != nil {
		t.Errorf("expected nil input fallthrough, got %v", got)
	}
}

// Response side: streaming returns the input as a single-chunk slice on
// fallthrough; non-stream returns the input bytes. Identity short-circuits.
func TestRegistryTranslateResponseFallsThroughOnMiss(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()
	in := []byte(`{"a":1}`)
	stream := r.TranslateResponseStream(ctx, FormatCodex, FormatOpenAIResponses, "m", nil, nil, in, nil)
	if len(stream) != 1 || string(stream[0]) != string(in) {
		t.Errorf("expected single-chunk fallthrough, got %v", stream)
	}
	nonstream := r.TranslateResponseNonStream(ctx, FormatCodex, FormatOpenAIResponses, "m", nil, nil, in, nil)
	if string(nonstream) != string(in) {
		t.Errorf("expected non-stream fallthrough, got %q", nonstream)
	}
}

// HasResponseTransformer is false on identity AND on missing AND on
// request-only translators — operators use this to decide whether to
// buffer the upstream stream.
func TestRegistryHasResponseTransformerSemantics(t *testing.T) {
	r := NewRegistry()
	// Request-only translator: HasResponse should still be false.
	r.Register(FormatCodex, FormatOpenAIResponses,
		func(string, []byte, bool) []byte { return nil }, nil,
	)
	if r.HasResponseTransformer(FormatCodex, FormatOpenAIResponses) {
		t.Error("request-only translator should not report HasResponseTransformer")
	}
	// Register a response translator on a different pair.
	r.Register(FormatClaudeMessages, FormatOpenAIResponses, nil,
		func(_ context.Context, _ string, _, _, raw []byte, _ *any) [][]byte { return [][]byte{raw} },
	)
	if !r.HasResponseTransformer(FormatClaudeMessages, FormatOpenAIResponses) {
		t.Error("response translator should report HasResponseTransformer")
	}
	// Identity must always be false even if a translator was somehow
	// registered (it would never be invoked).
	if r.HasResponseTransformer(FormatCodex, FormatCodex) {
		t.Error("identity pair must never report HasResponseTransformer")
	}
}

// Default() is the process-wide singleton — same instance across calls.
func TestDefaultRegistryIsStable(t *testing.T) {
	a := Default()
	b := Default()
	if a != b {
		t.Error("Default() should return the same singleton across calls")
	}
}
