package translators

import (
	"encoding/json"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// Pair: chatgpt_web → openai_responses.
//
// CLIProxyAPI's chatgpt2api reference covers this pair as the bridge
// between the chatgpt-web reverse-proxy envelope (action, messages,
// conversation_mode, sentinel header overlay) and the OpenAI /v1/
// responses shape. In srapi the inline bridge body construction lives
// in chatgpt_web.go's chatGPTWebBuildConversationBody +
// chatGPTWebConversationPayload + chatGPTWebMultimodalPayloadBytes.
//
// What the registered translator does:
//
//   - Identity pass-through. The inline chatGPTWebConversationPayload
//     (text-only) and chatGPTWebMultimodalPayloadBytes (with
//     file_service:// asset pointers from the PR-3 file uploader)
//     already emit the canonical bytes the upstream expects, including
//     the websocket_request_id and the parent_message_id stable-id
//     synthesis. Re-shaping here would duplicate that logic.
//
//     The identity registration mirrors the openai_responses_to_codex
//     package-init identity: presence proves the registry is consulted
//     on the hot path; the no-op output proves byte-for-byte parity
//     with the prior inline path. The PR-3 wiring (FlareSolverr
//     clearance, file upload, image slot, WS fallback) all stay
//     OUTSIDE this translator — they're transport-layer concerns
//     (cookies, headers, concurrency caps), not payload transforms.
//
// Future fuller extraction seam: when a future PR moves the message
// shape synthesis (chatGPTWebMessages + chatGPTWebMultimodalPrompt)
// out of chatgpt_web.go and into this file, this rewriter becomes the
// home for the cross-format translation. Until then the inline
// helpers own the shape and this translator is identity.
//
// nil-safe + non-panicking on caller input.
func chatGPTWebToOpenAIResponsesRewriter(_ string, rawJSON []byte, _ bool) []byte {
	if len(rawJSON) == 0 {
		return rawJSON
	}
	// Defensive parse probe: if the body isn't JSON the upstream sees
	// the caller's original bytes. The inline transforms exhibited the
	// same fallthrough.
	var probe map[string]any
	if err := json.Unmarshal(rawJSON, &probe); err != nil {
		return rawJSON
	}
	return rawJSON
}

// init registers the identity translator on the Default() registry.
func init() {
	translator.Default().Register(
		translator.FormatChatGPTWeb,
		translator.FormatOpenAIResponses,
		chatGPTWebToOpenAIResponsesRewriter,
		nil,
	)
}
