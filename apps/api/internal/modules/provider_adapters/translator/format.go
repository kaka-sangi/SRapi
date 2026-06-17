// Package translator is the cross-format payload normalisation layer for the
// provider adapters. It mirrors CLIProxyAPI's internal/translator/ shape: a
// registry keyed on (from, to) format pairs, with Request and Response
// (stream + non-stream) translation functions registered per pair.
//
// The package is deliberately minimal here. The real per-pair logic lives in
// translators/ subpackages and is registered at process init via package
// init() blocks. Call sites in service/ (codex.go, claude.go, etc.) consult
// this registry instead of branching on format inline.
//
// Migration is incremental: a translator may not yet be registered for a
// given pair. The Default() registry's Request/Response calls fall through
// to the caller's input unchanged when no translator matches — same
// behavioural envelope as CLIProxyAPI's NeedConvert false branch. New
// translators land one at a time without disturbing the existing inline
// transforms.
package translator

import "strings"

// Format is the canonical identifier for an API payload shape. Keep these as
// lowercase snake_case strings so they survive YAML/JSON config round-trips
// without re-encoding.
type Format string

const (
	// Upstream Codex CLI's /v1/responses payload shape (OpenAI native +
	// codex-specific fields like prompt_cache_key / store / reasoning).
	FormatCodex Format = "codex"

	// OpenAI's documented /v1/responses API — the shape srapi exposes to
	// inbound clients on the /v1/responses route.
	FormatOpenAIResponses Format = "openai_responses"

	// Claude / Anthropic /v1/messages native shape.
	FormatClaudeMessages Format = "claude_messages"

	// The compatibility envelope srapi exposes for clients that send
	// Anthropic-shaped requests but want to hit non-Anthropic upstreams.
	FormatAnthropicCompatible Format = "anthropic_compatible"

	// Gemini CLI request payload shape (camelCase fields, separate
	// safety_settings + system_instruction blocks).
	FormatGeminiRequest Format = "gemini_request"

	// Antigravity desktop reverse-proxy payload shape.
	FormatAntigravity Format = "antigravity"

	// ChatGPT web reverse-proxy bridge payload shape (PR-3 — conversation
	// envelope with file_service:// asset pointers).
	FormatChatGPTWeb Format = "chatgpt_web"
)

// FormatFromString resolves a string to a known Format. Unknown strings
// round-trip unchanged so operator-supplied custom formats (e.g. future
// ollama) work without a code change here — the registry simply won't have
// a translator for the unknown pair and the caller's input falls through.
func FormatFromString(s string) Format {
	return Format(strings.TrimSpace(strings.ToLower(s)))
}

// String returns the canonical lowercase identifier — useful for logging,
// metrics labels, and audit records.
func (f Format) String() string { return string(f) }

// Empty reports whether the format identifier is blank. Used by the registry
// to short-circuit invalid Register calls and surface empty pairs as a
// no-translator outcome rather than a panic.
func (f Format) Empty() bool { return strings.TrimSpace(string(f)) == "" }
