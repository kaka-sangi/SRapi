package service

import "strings"

// thinkingProtocolFamily classifies the upstream's contract for replaying
// reasoning ("thinking") blocks across turns.
//
// Different Claude /v1/messages-compatible upstreams hold OPPOSITE contracts:
//
//   - Anthropic official: a `thinking` block in the request history must carry
//     a valid signature. A missing or forged signature produces
//     400 "thinking.signature: Field required". The defensive move is to STRIP
//     unsigned blocks before forwarding.
//
//   - DeepSeek /anthropic, Kimi /coding, GLM, MiniMax-M, Qwen *-thinking, etc:
//     thinking blocks must round-trip byte-identical. The upstream rejects the
//     request with 400 if we modify them: "The content[].thinking in the
//     thinking mode must be passed back to the API". The defensive move is to
//     forward UNCHANGED.
//
// The strip pass in claudeThinkingSanitizeRawPayload was previously
// unconditional, breaking the second family. This classifier gates that
// pass by the upstream model id (post per-account model mapping; see
// `req.Mapping.UpstreamModelName` at the call site).
//
// Ported, with attribution, from sub2api's
// backend/internal/service/thinking_protocol.go after the 2026-06 upstream pull.
type thinkingProtocolFamily int

const (
	// thinkingFamilyUnknown — unrecognised upstream. Conservatively do not
	// strip; thinking signature validation is an Anthropic-strict feature
	// and any rewrite on an unknown upstream could break a contract we do
	// not understand.
	thinkingFamilyUnknown thinkingProtocolFamily = iota

	// thinkingFamilyAnthropicStrict — Claude family. Unsigned thinking blocks
	// must be stripped before forwarding.
	thinkingFamilyAnthropicStrict

	// thinkingFamilyPassbackRequired — third-party Claude-compatible upstream
	// that requires byte-identical thinking-block round-trip.
	thinkingFamilyPassbackRequired
)

// resolveThinkingProtocolFamily returns the family for the given upstream model
// id. The id is the one the request will carry to the upstream, i.e. the
// per-account-mapped model name.
func resolveThinkingProtocolFamily(model string) thinkingProtocolFamily {
	id := strings.ToLower(strings.TrimSpace(model))
	if id == "" {
		return thinkingFamilyUnknown
	}

	// Passback-required matches FIRST so a third-party upstream prefixed with
	// "claude" (e.g. an Anthropic-shaped Kimi relay called "claude-kimi-x")
	// would still be caught by the explicit per-vendor prefix list below.
	switch {
	case strings.HasPrefix(id, "deepseek-"),
		strings.HasPrefix(id, "kimi-"),
		strings.HasPrefix(id, "moonshot-"),
		strings.HasPrefix(id, "glm-"):
		return thinkingFamilyPassbackRequired
	}
	// MiniMax M-series (MiniMax-M2, M2.5, M2.7-highspeed, ...) routes through
	// https://api.minimax.io/anthropic and documents interleaved-thinking
	// passback. ToLower normalises to "minimax-".
	if strings.HasPrefix(id, "minimax-m") {
		return thinkingFamilyPassbackRequired
	}
	// Qwen *-thinking variants (qwen-/qwen2-/qwen3-/qwen4-*-thinking).
	if (strings.HasPrefix(id, "qwen-") ||
		strings.HasPrefix(id, "qwen2-") ||
		strings.HasPrefix(id, "qwen3-") ||
		strings.HasPrefix(id, "qwen4-")) && strings.Contains(id, "-thinking") {
		return thinkingFamilyPassbackRequired
	}

	switch {
	case strings.HasPrefix(id, "claude-"),
		strings.HasPrefix(id, "opus-"),
		strings.HasPrefix(id, "sonnet-"),
		strings.HasPrefix(id, "haiku-"):
		return thinkingFamilyAnthropicStrict
	}

	return thinkingFamilyUnknown
}

// shouldStripClaudeThinkingForModel returns true only for the Anthropic-strict
// family. For passback-required and unknown families the strip pass is a
// no-op (passback would break the upstream contract; unknown is conservative).
func shouldStripClaudeThinkingForModel(model string) bool {
	return resolveThinkingProtocolFamily(model) == thinkingFamilyAnthropicStrict
}
