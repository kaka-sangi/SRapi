package translators

import (
	"encoding/json"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// Pair: gemini_request → openai_responses.
//
// CLIProxyAPI's internal/translator/gemini/ tree splits gemini → other
// formats fairly aggressively (gemini-cli, gemini-web, etc.). In srapi
// the gemini adapter is upstream-native: gemini_request payloads are
// constructed inline by geminiCompatibleRequestBody +
// geminiCompatiblePayload (apps/api/internal/modules/provider_adapters/
// service/gemini_conversation_request.go) and forwarded to the
// upstream without any cross-format rewriting. Antigravity rides on
// top of the same per-call gemini synthesis (see antigravity.go's
// antigravityInnerRequest).
//
// Because srapi's gemini handling is upstream-native (no openai_responses
// envelope adaptation needed at the request side), the registered
// translator is a deliberate identity — the same pattern as the
// openai_responses_to_codex package-init identity. Presence in the
// registry proves the call-site is wired; the no-op output means
// byte-for-byte parity with the prior inline path.
//
// TODO(future PR): if a future srapi feature requires cross-format
// rewriting (e.g. translating Gemini-shaped tool_config to OpenAI-style
// tool_choice on the upstream-bound payload), the rewriter helper
// below is the seam. Until then identity is correct.
//
// nil-safe across the board: nil rawJSON falls through, unparseable
// JSON falls through, the function never panics on caller input.
func geminiRequestToOpenAIResponsesRewriter(_ string, rawJSON []byte, _ bool) []byte {
	if len(rawJSON) == 0 {
		return rawJSON
	}
	// Defensive parse probe: if the body isn't JSON we still pass it
	// through unchanged so the upstream sees the caller's original
	// bytes. The inline transforms exhibited the same fallthrough
	// behaviour.
	var probe map[string]any
	if err := json.Unmarshal(rawJSON, &probe); err != nil {
		return rawJSON
	}
	return rawJSON
}

// init registers the identity translator on the Default() registry.
func init() {
	translator.Default().Register(
		translator.FormatGeminiRequest,
		translator.FormatOpenAIResponses,
		geminiRequestToOpenAIResponsesRewriter,
		nil,
	)
}
