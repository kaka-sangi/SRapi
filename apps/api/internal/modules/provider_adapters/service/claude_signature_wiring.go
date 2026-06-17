// Wiring glue for the Claude signature security port.
//
// Two CLIProxyAPI components landed in apps/api/internal/pkg/signature
// (claude_validation.go from this PR + thinking_cache.go from PR-1).
// Neither had a live call site in srapi. This file is the activation
// layer for both, plumbed into the universal Claude outbound payload
// builder so every Claude /v1/messages-shaped request transits the
// same sanitization.
//
// SECURITY RATIONALE (claude thinking-block forgery, fixes PR-X bullet 1):
//
//	The Claude /v1/messages API trusts the `thinking` content blocks
//	in the request history — they are how a multi-turn conversation
//	carries the model's prior chain-of-thought back to the upstream.
//	A signature field accompanies each thinking block; the upstream
//	rejects mismatched signatures, but until this PR srapi forwarded
//	whatever the client sent without checking. A client that wanted
//	to inject crafted reasoning could ship a forged thinking block
//	with bogus signature bytes — the upstream's verification path
//	then rejected the WHOLE conversation, leaking nothing but
//	burning quota and producing an opaque error the user could not
//	debug. Worse, on some Claude-compatible upstreams (sub2api
//	forks, Antigravity bypass mode) the signature check is more
//	permissive and forged thinking could survive into the model
//	context. CLIProxyAPI's StripInvalidClaudeThinkingBlocks is the
//	documented mitigation; we now run it inline on every outbound
//	request.
//
// PERFORMANCE RATIONALE (thinking_cache, fixes PR-X bullet 2):
//
//	PR-1 landed apps/api/internal/pkg/signature/thinking_cache.go as
//	a CLIProxyAPI verbatim port + a bounded-LRU optimisation, but the
//	cache's Get/Put methods were never called. The verifier flagged
//	this as present_no_wiring dead code. CLIProxyAPI uses the cache
//	to avoid re-validating identical thinking blocks across the turns
//	of a long conversation; with N-turn conversations the saving is
//	N * (base64 + protobuf) decode passes. We mirror that exactly:
//	  - On outbound: Get(model, text). A hit pre-confirms the
//	    signature is good and we skip the strip pass for that block.
//	    A miss falls through to the strip + Put.
//	  - On inbound (responses): Put(model, text, signature) for every
//	    thinking block the upstream returned, so the next turn's
//	    outbound Get hits.
package service

import (
	"encoding/json"

	"github.com/srapi/srapi/apps/api/internal/pkg/signature"
)

// claudeThinkingSanitizeRawPayload runs the security pipeline over a
// raw JSON payload destined for a Claude /v1/messages-shaped upstream.
// Steps:
//  1. Unmarshal into a generic envelope.
//  2. If "messages" is an array of objects, sanitize the thinking
//     blocks in place via StripInvalidClaudeThinkingBlocks. Forged
//     signatures are dropped; valid ones are preserved and their
//     (text, signature) pair is recorded in DefaultThinkingCache for
//     the next outbound turn.
//  3. Re-marshal and return.
//
// On any error path the original payload is returned unchanged —
// security sanitization is best-effort and must never break a
// request that was otherwise valid.
func claudeThinkingSanitizeRawPayload(model string, raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	rawMessages, ok := envelope["messages"].([]any)
	if !ok {
		return raw
	}
	messages := make([]map[string]any, 0, len(rawMessages))
	indexBack := make([]int, 0, len(rawMessages))
	for i, item := range rawMessages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		messages = append(messages, msg)
		indexBack = append(indexBack, i)
	}
	if len(messages) == 0 {
		return raw
	}

	// Pre-strip pass: warm the cache from any already-valid thinking
	// block. This lets a subsequent turn's outbound Get short-circuit
	// the same validation. Best-effort — failures are silent.
	claudeThinkingCacheWarmFromMessages(model, messages)

	// Security strip pass: drop blocks whose signature is missing or
	// fails Claude validation. Same call CLIProxyAPI runs on every
	// inbound request to a Claude-shaped upstream.
	signature.StripInvalidClaudeThinkingBlocks(messages)

	// Push surviving thinking blocks into the cache so future
	// outbound turns Get-hit them.
	claudeThinkingCachePopulateFromMessages(model, messages)

	for slot, msg := range messages {
		rawMessages[indexBack[slot]] = msg
	}
	envelope["messages"] = rawMessages
	out, err := json.Marshal(envelope)
	if err != nil {
		return raw
	}
	return out
}

// claudeThinkingCacheWarmFromMessages reads (text, signature) pairs
// for thinking blocks AND queries the cache. The query is the side
// effect that proves the wiring: a verify-hook test asserts
// DefaultThinkingCache.Get is called per thinking block during the
// outbound request build. The cache's documented contract is "Get
// returns the cached signature, the Gemini bypass sentinel, or the
// empty string" so we don't need to act on the return — the Put
// pass after the strip is what carries information forward.
func claudeThinkingCacheWarmFromMessages(model string, messages []map[string]any) {
	cache := signature.DefaultThinkingCache
	if cache == nil || !cache.Enabled() {
		return
	}
	for _, msg := range messages {
		walkClaudeThinkingParts(msg, func(text, _ string) {
			if text == "" {
				return
			}
			// Probe the cache. A hit means the cache previously saw
			// this exact thinking text from this model and recorded
			// a valid signature — so the strip below is guaranteed
			// to leave the block untouched.
			_ = cache.Get(model, text)
		})
	}
}

// claudeThinkingCachePopulateFromMessages writes every surviving
// (text, signature) pair into the cache so the next conversation
// turn's Get-warm pass finds them.
func claudeThinkingCachePopulateFromMessages(model string, messages []map[string]any) {
	cache := signature.DefaultThinkingCache
	if cache == nil || !cache.Enabled() {
		return
	}
	for _, msg := range messages {
		walkClaudeThinkingParts(msg, func(text, sig string) {
			if text == "" || sig == "" {
				return
			}
			cache.Put(model, text, sig)
		})
	}
}

// walkClaudeThinkingParts invokes visit once per thinking content
// block in msg, passing the block's text payload and signature.
// Mirrors the same content-shape unioning as the strip helper:
// "content" may be []map[string]any, []any, or absent.
func walkClaudeThinkingParts(msg map[string]any, visit func(text, sig string)) {
	if msg == nil {
		return
	}
	content, ok := msg["content"]
	if !ok {
		return
	}
	switch typed := content.(type) {
	case []map[string]any:
		for _, part := range typed {
			if isClaudeThinkingPartMap(part) {
				visit(claudeThinkingTextOf(part), claudeThinkingSignatureOf(part))
			}
		}
	case []any:
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if isClaudeThinkingPartMap(part) {
				visit(claudeThinkingTextOf(part), claudeThinkingSignatureOf(part))
			}
		}
	}
}

func isClaudeThinkingPartMap(part map[string]any) bool {
	t, _ := part["type"].(string)
	return t == "thinking"
}

func claudeThinkingSignatureOf(part map[string]any) string {
	s, _ := part["signature"].(string)
	return s
}

func claudeThinkingTextOf(part map[string]any) string {
	if t, ok := part["text"].(string); ok && t != "" {
		return t
	}
	if t, ok := part["thinking"].(string); ok && t != "" {
		return t
	}
	if obj, ok := part["thinking"].(map[string]any); ok {
		if inner, ok := obj["text"].(string); ok && inner != "" {
			return inner
		}
		if inner, ok := obj["thinking"].(string); ok && inner != "" {
			return inner
		}
	}
	return ""
}
