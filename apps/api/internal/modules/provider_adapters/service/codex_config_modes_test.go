package service

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestResolveCodexModelAlias_NilSafeAndEmptyInputs(t *testing.T) {
	if got := ResolveCodexModelAlias(nil, "openai", "gpt-5-codex"); got != "gpt-5-codex" {
		t.Fatalf("nil cfg should return canonical unchanged, got %q", got)
	}
	cfg := &config.Config{}
	if got := ResolveCodexModelAlias(cfg, "openai", "gpt-5-codex"); got != "gpt-5-codex" {
		t.Fatalf("empty cfg should return canonical unchanged, got %q", got)
	}
	if got := ResolveCodexModelAlias(cfg, "", "gpt-5-codex"); got != "gpt-5-codex" {
		t.Fatalf("empty channel should return canonical unchanged, got %q", got)
	}
	if got := ResolveCodexModelAlias(cfg, "openai", ""); got != "" {
		t.Fatalf("empty canonical should return empty unchanged, got %q", got)
	}
}

func TestResolveCodexModelAlias_ExactMatch(t *testing.T) {
	cfg := &config.Config{
		Codex: config.CodexConfig{
			ModelAlias: map[string]map[string]string{
				"openai": {"gpt-5-codex": "gpt-5.1-codex-internal"},
			},
		},
	}
	if got := ResolveCodexModelAlias(cfg, "openai", "gpt-5-codex"); got != "gpt-5.1-codex-internal" {
		t.Fatalf("exact match should resolve alias, got %q", got)
	}
}

func TestResolveCodexModelAlias_ChannelLowerCased(t *testing.T) {
	cfg := &config.Config{
		Codex: config.CodexConfig{
			ModelAlias: map[string]map[string]string{
				"openai": {"gpt-5-codex": "gpt-5.1-codex-internal"},
			},
		},
	}
	// Channel key is normalized to lower case on lookup so an "OpenAI"
	// argument from a header still hits the lower-cased map entry.
	if got := ResolveCodexModelAlias(cfg, "OpenAI", "gpt-5-codex"); got != "gpt-5.1-codex-internal" {
		t.Fatalf("channel case-folding failed: got %q", got)
	}
}

func TestResolveCodexModelAlias_CanonicalCaseSensitive(t *testing.T) {
	cfg := &config.Config{
		Codex: config.CodexConfig{
			ModelAlias: map[string]map[string]string{
				"openai": {"gpt-5-codex": "gpt-5.1-codex-internal"},
			},
		},
	}
	// Upstream model IDs are case-sensitive — a mismatched case must not
	// resolve. The function returns the canonical unchanged.
	if got := ResolveCodexModelAlias(cfg, "openai", "GPT-5-CODEX"); got != "GPT-5-CODEX" {
		t.Fatalf("canonical lookup must be case-sensitive, got %q", got)
	}
}

func TestResolveCodexModelAlias_NoGlobMatching(t *testing.T) {
	cfg := &config.Config{
		Codex: config.CodexConfig{
			ModelAlias: map[string]map[string]string{
				"openai": {"gpt-5-*": "gpt-5.1-codex-internal"},
			},
		},
	}
	// CLIProxyAPI parity: exact match only, glob patterns must NOT match.
	if got := ResolveCodexModelAlias(cfg, "openai", "gpt-5-codex"); got != "gpt-5-codex" {
		t.Fatalf("glob lookup must not match, got %q", got)
	}
}

func TestResolveCodexModelAlias_EmptyAliasFallsBack(t *testing.T) {
	cfg := &config.Config{
		Codex: config.CodexConfig{
			ModelAlias: map[string]map[string]string{
				"openai": {"gpt-5-codex": "   "},
			},
		},
	}
	if got := ResolveCodexModelAlias(cfg, "openai", "gpt-5-codex"); got != "gpt-5-codex" {
		t.Fatalf("blank alias must fall back to canonical, got %q", got)
	}
}

func TestShouldDisableCodexImageGeneration_Never(t *testing.T) {
	if ShouldDisableCodexImageGeneration(nil, "anything") {
		t.Fatalf("nil cfg must default to allow")
	}
	cfg := &config.Config{}
	if ShouldDisableCodexImageGeneration(cfg, "anything") {
		t.Fatalf("empty mode must default to allow")
	}
	cfg.Codex.DisableImageGeneration = "never"
	if ShouldDisableCodexImageGeneration(cfg, "codex_cli_rs/0.125.0") {
		t.Fatalf("never must always return false")
	}
	cfg.Codex.DisableImageGeneration = "bogus"
	if ShouldDisableCodexImageGeneration(cfg, "codex_cli_rs/0.125.0") {
		t.Fatalf("unknown mode must fail safe to allow")
	}
}

func TestShouldDisableCodexImageGeneration_Always(t *testing.T) {
	cfg := &config.Config{Codex: config.CodexConfig{DisableImageGeneration: "always"}}
	if !ShouldDisableCodexImageGeneration(cfg, "codex_cli_rs/0.125.0") {
		t.Fatalf("always must block matched UAs")
	}
	if !ShouldDisableCodexImageGeneration(cfg, "") {
		t.Fatalf("always must block empty UAs")
	}
	if !ShouldDisableCodexImageGeneration(cfg, "totally-unrelated-client/1.0") {
		t.Fatalf("always must block unmatched UAs")
	}
}

func TestShouldDisableCodexImageGeneration_Auto(t *testing.T) {
	cfg := &config.Config{Codex: config.CodexConfig{DisableImageGeneration: "auto"}}

	// Matches the Codex CLI family — block.
	if !ShouldDisableCodexImageGeneration(cfg, "codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color") {
		t.Fatalf("auto must block matching Codex CLI UA")
	}
	if !ShouldDisableCodexImageGeneration(cfg, "Codex_CLI_RS/0.999.99") {
		t.Fatalf("auto regex must be case-insensitive")
	}

	// Does NOT match — pass through.
	if ShouldDisableCodexImageGeneration(cfg, "") {
		t.Fatalf("auto must NOT block an empty UA (safe default)")
	}
	if ShouldDisableCodexImageGeneration(cfg, "OpenAI/Python 1.40") {
		t.Fatalf("auto must NOT block unrelated clients")
	}
}
