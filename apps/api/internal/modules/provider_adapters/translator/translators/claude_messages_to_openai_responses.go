package translators

import (
	"encoding/json"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// Pair: claude_messages → openai_responses.
//
// CLIProxyAPI's internal/translator/claude_messages_to_openai_responses/
// directory is the reference for this pair. In srapi the corresponding
// inline body construction lives in
// apps/api/internal/modules/provider_adapters/service/conversation_protocols.go
// (anthropicCompatibleRequestBody + anthropicCompatiblePayload). That
// function still owns the per-call payload synthesis; this translator
// runs ON TOP of the synthesised bytes as a normalisation pass so the
// registry sits on the hot path observably without changing the
// emitted payload byte-for-byte.
//
// What the registered translator does:
//
//   - Identity pass-through. The inline anthropicCompatibleRequestBody
//     already emits a fully canonical Anthropic /v1/messages-shaped
//     payload, including the PR-X security strip
//     (claudeThinkingSanitizeRawPayload). The signature strip is the
//     only logical concern of this pair the translator could re-apply;
//     re-running it here would either double-apply (no-op but wasted
//     CPU) or silently drift if the strip implementation changes —
//     neither is desirable. So the translator stays an explicit
//     identity, mirroring the openai_responses_to_codex package-init
//     identity registration: presence in the registry proves the
//     call-site is wired; the no-op output proves byte-for-byte
//     parity with the pre-refactor inline path.
//
// Migration plan for fuller extraction: when a future PR moves the
// content_block synthesis (tool block translation, system prompt
// rewriting, content_block parts → output_text parts) out of
// conversation_protocols.go and into this file, the rewriter helper
// below is the seam. It already accepts a raw payload + a per-call
// model name + the stream flag, matching the inline signature.
//
// Concurrency: the rewriter is pure-functional; the registry's
// RWMutex guards the map. Safe under fan-in from multiple goroutines.
//
// nil-safe: empty rawJSON, unparseable JSON, missing "messages" field —
// all fall through to the input unchanged.
func claudeMessagesToOpenAIResponsesRewriter(_ string, rawJSON []byte, _ bool) []byte {
	if len(rawJSON) == 0 {
		return rawJSON
	}
	// Defensive parse-then-marshal pass: if the body is JSON, the
	// round-trip canonicalises field order which the upstream is
	// permissive about. If it isn't JSON, we fall through to the
	// caller's bytes — the inline transforms exhibited the same
	// fallthrough behaviour.
	var probe map[string]any
	if err := json.Unmarshal(rawJSON, &probe); err != nil {
		return rawJSON
	}
	// IDENTITY: do not re-marshal. The caller already produced the
	// canonical bytes; re-marshalling here would re-order fields and
	// break byte-for-byte comparisons in existing tests
	// (claude_signature_wiring_test, conversation_protocols_openai
	// tests etc.). Return the input unchanged.
	return rawJSON
}

// init registers the identity translator on the Default() registry.
// The presence proves the registry is consulted on the hot path; the
// no-op output proves byte-for-byte parity with the inline path
// (which the wider claude/v1 messages tests pin).
func init() {
	translator.Default().Register(
		translator.FormatClaudeMessages,
		translator.FormatOpenAIResponses,
		claudeMessagesToOpenAIResponsesRewriter,
		nil,
	)
}
