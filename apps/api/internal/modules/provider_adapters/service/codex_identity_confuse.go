// Ported from CLIProxyAPI internal/runtime/executor/codex_executor.go (the
// applyCodexIdentityConfuseBody / applyCodexIdentityConfuseHeaders /
// applyCodexIdentityConfuseResponsePayload family) and
// codex_executor_signature_test.go. The reference behaviour:
//
//   - For each upstream Codex auth (account) and each rewritable identifier
//     (prompt_cache_key, x-codex-installation-id, turn_id), derive a stable
//     UUID via UUIDv5 over uuid.NameSpaceOID, namespace string
//     "cli-proxy-api:codex:identity-confuse:<kind>:<authID>:<original>".
//   - Rewrite those identifiers in the outbound JSON body and outbound
//     HTTP headers (X-Codex-Turn-Metadata, Session_id, Conversation_id,
//     X-Client-Request-Id, Thread-Id, X-Codex-Window-Id).
//   - On the streamed response payload, reverse-map any echoed rewritten
//     identifier back to the caller's original value so the client never
//     sees the rewrite.
//
// The deterministic rewrite preserves the upstream's prompt-cache hit rate
// for repeated turns of the same caller while making the multiplexed
// behaviour unreproducible across different caller identities.
//
// Deviations from the reference (called out per the port directive):
//
//  1. CLIProxyAPI uses tidwall/sjson to splice the JSON body and reads via
//     gjson. We use encoding/json with map[string]any and re-marshal once
//     per rewrite. The on-the-wire content is equivalent for the documented
//     payload shape.
//  2. The reference depends on an opt-in config flag (cfg.Codex.IdentityConfuse
//     plus routing strategy). The behaviour is preserved verbatim: the
//     IsEnabled helper takes a config snapshot; call sites must check it
//     before calling Apply.
package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// CodexIdentityConfuseConfig captures the two routing flags the reference
// uses to decide whether identity confusion runs.
type CodexIdentityConfuseConfig struct {
	Enabled         bool
	SessionAffinity bool
	RoutingStrategy string
}

// CodexIdentityConfuseEnabled mirrors codexIdentityConfuseEnabled.
func CodexIdentityConfuseEnabled(cfg CodexIdentityConfuseConfig) bool {
	if !cfg.Enabled {
		return false
	}
	strategy := strings.ToLower(strings.TrimSpace(cfg.RoutingStrategy))
	return cfg.SessionAffinity || strategy == "fill-first" || strategy == "fillfirst" || strategy == "ff"
}

// CodexIdentityReplacement records a single (original, rewritten) pair so
// the response stream can be reverse-mapped.
type CodexIdentityReplacement struct {
	Original string
	Confused string
}

// CodexIdentityConfuseState holds the per-request rewrite state. Callers
// must keep this instance for the lifetime of the request so the response
// stream can be reverse-mapped at the end.
type CodexIdentityConfuseState struct {
	Enabled                bool
	AuthID                 string
	OriginalPromptCacheKey string
	PromptCacheKey         string
	TurnIDs                []CodexIdentityReplacement
}

// ApplyCodexIdentityConfuseBody rewrites the outbound Codex request body.
// userPayload is the caller's pre-rewrite body (used to read the original
// identifiers); rawJSON is the body that will actually be sent.
//
// Returns the (possibly new) outbound body and the rewrite state. When
// identity confusion is disabled, returns rawJSON and a zero state.
func ApplyCodexIdentityConfuseBody(cfg CodexIdentityConfuseConfig, authID string, userPayload, rawJSON []byte) ([]byte, CodexIdentityConfuseState) {
	authID = strings.TrimSpace(authID)
	if !CodexIdentityConfuseEnabled(cfg) || authID == "" || len(rawJSON) == 0 {
		return rawJSON, CodexIdentityConfuseState{}
	}
	state := CodexIdentityConfuseState{Enabled: true, AuthID: authID}

	var userObj map[string]any
	if len(userPayload) > 0 {
		_ = json.Unmarshal(bytes.TrimSpace(userPayload), &userObj)
	}
	var raw map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(rawJSON), &raw); err != nil {
		// Body is not JSON we can rewrite; keep it as-is.
		return rawJSON, state
	}

	if userObj != nil {
		if key := strings.TrimSpace(stringValueOrEmpty(userObj["prompt_cache_key"])); key != "" {
			state.OriginalPromptCacheKey = key
			state.PromptCacheKey = codexIdentityConfuseUUID(authID, "prompt-cache", key)
			raw["prompt_cache_key"] = state.PromptCacheKey
		}
		if metadata, ok := userObj["client_metadata"].(map[string]any); ok {
			if installationID := strings.TrimSpace(stringValueOrEmpty(metadata["x-codex-installation-id"])); installationID != "" {
				if rawMetadata, ok := raw["client_metadata"].(map[string]any); ok {
					rawMetadata["x-codex-installation-id"] = codexIdentityConfuseUUID(authID, "installation", installationID)
				}
			}
		}
	}

	if metadata, ok := raw["client_metadata"].(map[string]any); ok {
		if turnMetadata := strings.TrimSpace(stringValueOrEmpty(metadata["x-codex-turn-metadata"])); turnMetadata != "" {
			metadata["x-codex-turn-metadata"] = applyCodexTurnMetadataIdentityConfuse(turnMetadata, &state)
		}
		if state.PromptCacheKey != "" {
			if windowID := strings.TrimSpace(stringValueOrEmpty(metadata["x-codex-window-id"])); windowID != "" {
				metadata["x-codex-window-id"] = state.PromptCacheKey + ":0"
			}
		}
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return rawJSON, state
	}
	return encoded, state
}

// ApplyCodexIdentityConfuseHeaders rewrites outbound HTTP headers in place.
// Matches applyCodexIdentityConfuseHeaders verbatim.
func ApplyCodexIdentityConfuseHeaders(headers http.Header, state *CodexIdentityConfuseState) {
	if headers == nil || state == nil || !state.Enabled {
		return
	}
	if rawTurnMetadata := strings.TrimSpace(headers.Get("X-Codex-Turn-Metadata")); rawTurnMetadata != "" {
		headers.Set("X-Codex-Turn-Metadata", applyCodexTurnMetadataIdentityConfuse(rawTurnMetadata, state))
	}
	if state.PromptCacheKey == "" {
		return
	}
	// CLIProxyAPI preserves the original header casing for Session_id /
	// Conversation_id; net/http will canonicalize Session_id, but the
	// behaviour we need is "if the caller provided it, replace its value".
	for _, name := range []string{"Session_id", "session_id", "Session-Id"} {
		if headers.Get(name) != "" {
			headers.Set(name, state.PromptCacheKey)
		}
	}
	for _, name := range []string{"Conversation_id", "conversation_id", "Conversation-Id"} {
		if headers.Get(name) != "" {
			headers.Set(name, state.PromptCacheKey)
		}
	}
	headers.Set("X-Client-Request-Id", state.PromptCacheKey)
	headers.Set("Thread-Id", state.PromptCacheKey)
	headers.Set("X-Codex-Window-Id", state.PromptCacheKey+":0")
}

// ApplyCodexIdentityConfuseResponsePayload rewrites outbound stream bytes from
// the caller's original identifier to the rewritten one (used when echoing
// stored requests). Matches applyCodexIdentityConfuseResponsePayload.
func ApplyCodexIdentityConfuseResponsePayload(payload []byte, state CodexIdentityConfuseState) []byte {
	payload = replaceCodexIdentityResponsePayload(payload, state.OriginalPromptCacheKey, state.PromptCacheKey)
	for _, turnID := range state.TurnIDs {
		payload = replaceCodexIdentityResponsePayload(payload, turnID.Original, turnID.Confused)
	}
	return payload
}

// ApplyCodexIdentityExposeResponsePayload rewrites the upstream response so
// the client sees its original identifiers. Matches
// applyCodexIdentityExposeResponsePayload.
func ApplyCodexIdentityExposeResponsePayload(payload []byte, state CodexIdentityConfuseState) []byte {
	payload = replaceCodexIdentityResponsePayload(payload, state.PromptCacheKey, state.OriginalPromptCacheKey)
	for _, turnID := range state.TurnIDs {
		payload = replaceCodexIdentityResponsePayload(payload, turnID.Confused, turnID.Original)
	}
	return payload
}

// ConfuseTurnID returns the rewritten turn id, memoizing the (original,
// confused) pair so the response stream can reverse it. Matches
// state.confuseTurnID.
func (s *CodexIdentityConfuseState) ConfuseTurnID(turnID string) string {
	turnID = strings.TrimSpace(turnID)
	if s == nil || !s.Enabled || strings.TrimSpace(s.AuthID) == "" || turnID == "" {
		return turnID
	}
	for _, replacement := range s.TurnIDs {
		if replacement.Original == turnID || replacement.Confused == turnID {
			return replacement.Confused
		}
	}
	confused := codexIdentityConfuseUUID(s.AuthID, "turn", turnID)
	s.TurnIDs = append(s.TurnIDs, CodexIdentityReplacement{Original: turnID, Confused: confused})
	return confused
}

func applyCodexTurnMetadataIdentityConfuse(rawTurnMetadata string, state *CodexIdentityConfuseState) string {
	if state == nil || !state.Enabled {
		return rawTurnMetadata
	}
	updated := rawTurnMetadata
	var meta map[string]any
	if err := json.Unmarshal([]byte(updated), &meta); err == nil {
		if state.PromptCacheKey != "" {
			if _, ok := meta["prompt_cache_key"]; ok {
				meta["prompt_cache_key"] = state.PromptCacheKey
			}
			if _, ok := meta["window_id"]; ok {
				meta["window_id"] = state.PromptCacheKey + ":0"
			}
		}
		if turnID := strings.TrimSpace(stringValueOrEmpty(meta["turn_id"])); turnID != "" {
			meta["turn_id"] = state.ConfuseTurnID(turnID)
		}
		if encoded, err := json.Marshal(meta); err == nil {
			updated = string(encoded)
		}
	} else if state.PromptCacheKey != "" && state.OriginalPromptCacheKey != "" {
		updated = strings.ReplaceAll(updated, state.OriginalPromptCacheKey, state.PromptCacheKey)
	}
	return updated
}

func replaceCodexIdentityResponsePayload(payload []byte, from, to string) []byte {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if len(payload) == 0 || from == "" || to == "" || from == to || !bytes.Contains(payload, []byte(from)) {
		return payload
	}
	return bytes.ReplaceAll(payload, []byte(from), []byte(to))
}

func codexIdentityConfuseUUID(authID, kind, value string) string {
	name := strings.Join([]string{"cli-proxy-api", "codex", "identity-confuse", kind, strings.TrimSpace(authID), strings.TrimSpace(value)}, ":")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}
