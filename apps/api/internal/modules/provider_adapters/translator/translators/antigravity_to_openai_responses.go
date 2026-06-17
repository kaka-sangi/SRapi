package translators

import (
	"encoding/json"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// Pair: antigravity → openai_responses.
//
// CLIProxyAPI's internal/translator/ directory does not carry an
// antigravity → openai_responses pair directly; antigravity is a
// Google reverse-proxy variant that wraps gemini_request inside an
// outer envelope (project, requestId, userAgent, requestType, model,
// inner generateText request). srapi's antigravityPayload (in
// apps/api/internal/modules/provider_adapters/service/antigravity.go)
// owns that envelope construction inline.
//
// What the registered translator does:
//
//   - Identity pass-through. The inline antigravityPayload already
//     emits the canonical antigravity-shaped bytes (including the
//     image_gen / agent requestType discrimination, the credit-type
//     gating, the session-id derivation). Re-shaping here would
//     either duplicate that logic or silently drift if the inline
//     code changes. The identity registration mirrors the
//     openai_responses_to_codex package-init identity: presence
//     proves the registry is consulted on the hot path; the no-op
//     output proves byte-for-byte parity with the prior inline
//     path — antigravity_test.go and the per-payload contract tests
//     all keep passing unchanged.
//
// Future fuller extraction seam: when a future PR moves the inner
// gemini_request synthesis into this translator file, the inner
// payload helper should be hooked through the
// gemini_request_to_openai_responses translator (also identity for
// now) so both pairs stay symmetrical.
//
// nil-safe + non-panicking on caller input.
func antigravityToOpenAIResponsesRewriter(_ string, rawJSON []byte, _ bool) []byte {
	if len(rawJSON) == 0 {
		return rawJSON
	}
	// Defensive parse probe: if the body isn't JSON we still pass it
	// through unchanged. The inline transforms exhibited the same
	// fallthrough behaviour.
	var probe map[string]any
	if err := json.Unmarshal(rawJSON, &probe); err != nil {
		return rawJSON
	}
	return rawJSON
}

// init registers the identity translator on the Default() registry.
func init() {
	translator.Default().Register(
		translator.FormatAntigravity,
		translator.FormatOpenAIResponses,
		antigravityToOpenAIResponsesRewriter,
		nil,
	)
}
