package service

import (
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/pkg/signature"
)

func TestResolveThinkingProtocolFamily_TableCovers(t *testing.T) {
	cases := []struct {
		model string
		want  thinkingProtocolFamily
	}{
		// Anthropic-strict — Claude vendor prefixes.
		{"claude-sonnet-4-6", thinkingFamilyAnthropicStrict},
		{"claude-opus-4-8", thinkingFamilyAnthropicStrict},
		{"opus-4", thinkingFamilyAnthropicStrict},
		{"sonnet-4", thinkingFamilyAnthropicStrict},
		{"haiku-4-5", thinkingFamilyAnthropicStrict},
		{"  Claude-3-7  ", thinkingFamilyAnthropicStrict}, // trimmed + lowered

		// Passback-required — third-party Anthropic-compatible relays.
		{"deepseek-v4-pro", thinkingFamilyPassbackRequired},
		{"kimi-coding-latest", thinkingFamilyPassbackRequired},
		{"moonshot-v2", thinkingFamilyPassbackRequired},
		{"glm-4.6", thinkingFamilyPassbackRequired},
		{"minimax-m2", thinkingFamilyPassbackRequired},
		{"MiniMax-M2.7-highspeed", thinkingFamilyPassbackRequired},
		{"qwen3-235b-a22b-thinking-2507", thinkingFamilyPassbackRequired},
		{"qwen2-72b-thinking", thinkingFamilyPassbackRequired},

		// Unknown — anything that doesn't fit a documented contract.
		{"", thinkingFamilyUnknown},
		{"gpt-4o", thinkingFamilyUnknown},
		{"gemini-2.5-pro", thinkingFamilyUnknown},
		{"qwen-3-72b-chat", thinkingFamilyUnknown},                       // qwen without -thinking
		{"minimax-text-01", thinkingFamilyUnknown},                       // minimax not -m series
		{"claudette-fork", thinkingFamilyUnknown},                        // "claudette-" does NOT have prefix "claude-"
		{"claude-foreign-relay-x", thinkingFamilyAnthropicStrict},        // documented limitation — anything starting with "claude-" is strict
		{"deepseek-but-not-real", thinkingFamilyPassbackRequired},
	}
	for _, tc := range cases {
		if got := resolveThinkingProtocolFamily(tc.model); got != tc.want {
			t.Errorf("resolveThinkingProtocolFamily(%q) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

// TestClaudeThinkingSanitizePassbackFamilyRoundTripsBytes asserts the
// sanitizer is a pass-through for passback-required upstreams even when
// the payload contains thinking blocks that would otherwise be stripped
// (no signature, no metadata). The wire bytes must be byte-identical so
// the upstream's round-trip contract is preserved.
func TestClaudeThinkingSanitizePassbackFamilyRoundTripsBytes(t *testing.T) {
	for _, model := range []string{"deepseek-v4-pro", "kimi-coding-latest", "glm-4.6", "minimax-m2", "qwen3-72b-thinking"} {
		body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","signature":"NOT-A-SIG"}]}]}`)
		out := claudeThinkingSanitizeRawPayload(model, body)
		if string(out) != string(body) {
			t.Errorf("model %q: sanitizer altered passback-required payload (would break upstream contract).\nwant: %s\ngot:  %s", model, string(body), string(out))
		}
	}
}

// TestClaudeThinkingSanitizeUnknownFamilyRoundTripsBytes mirrors the
// passback test for unknown families — we must be conservative and not
// touch payloads going to upstreams whose contract we don't know.
func TestClaudeThinkingSanitizeUnknownFamilyRoundTripsBytes(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x","signature":"NOT-A-SIG"}]}]}`)
	out := claudeThinkingSanitizeRawPayload("gpt-4o", body)
	if string(out) != string(body) {
		t.Fatalf("sanitizer altered unknown-family payload.\nwant: %s\ngot:  %s", string(body), string(out))
	}
}

// TestClaudeThinkingSanitizeStrictFamilyStillStrips guards the previous
// strict-family behavior — Claude/Opus/Sonnet/Haiku still get the
// forged-signature strip. Existing tests cover the deeper marshal
// path; this one just pins the public function level.
func TestClaudeThinkingSanitizeStrictFamilyStillStrips(t *testing.T) {
	// Clear so the cache state is hermetic.
	signature.DefaultThinkingCache.Clear("claude-strict-test")
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"forged","signature":"NOT-A-SIG"}]}]}`)
	out := claudeThinkingSanitizeRawPayload("claude-strict-test", body)
	if strings.Contains(string(out), "NOT-A-SIG") {
		t.Fatalf("strict-family path lost its strip behavior:\n%s", string(out))
	}
}
