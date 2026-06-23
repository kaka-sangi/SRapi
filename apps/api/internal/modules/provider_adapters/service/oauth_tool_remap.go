package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// oauthToolNameSanitizeRawPayload rewrites non-standard tool names in the
// marshaled request body for OAuth accounts. Anthropic (and some other
// providers) use tool names to fingerprint third-party clients; non-official
// tool names on OAuth-authenticated requests can trigger higher billing rates
// or abuse detection. This mirrors CLIProxyAPI's tool name remapping.
//
// API-key accounts are unaffected (the provider already knows it's a
// third-party integration). Only applies when the account is OAuth-class.
func oauthToolNameSanitizeRawPayload(req contract.ConversationRequest, raw []byte) []byte {
	if !isOAuthAccount(req.Account.RuntimeClass) {
		return raw
	}
	if len(req.Tools) == 0 {
		return raw
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return raw
	}
	toolsRaw, ok := body["tools"]
	if !ok || len(toolsRaw) == 0 {
		return raw
	}
	var tools []map[string]any
	if err := json.Unmarshal(toolsRaw, &tools); err != nil {
		return raw
	}
	changed := false
	for i, tool := range tools {
		name, _ := tool["name"].(string)
		if name == "" {
			continue
		}
		sanitized := sanitizeToolName(name)
		if sanitized != name {
			tools[i]["name"] = sanitized
			changed = true
		}
	}
	if !changed {
		return raw
	}
	newTools, err := json.Marshal(tools)
	if err != nil {
		return raw
	}
	body["tools"] = newTools
	out, err := json.Marshal(body)
	if err != nil {
		return raw
	}
	return out
}

// officialToolNames is the set of tool names that Anthropic's official clients
// (Claude Code, Claude Desktop, etc.) use. These are safe to send as-is on
// OAuth accounts. Non-listed names are hashed to a stable prefix to avoid
// leaking the client's identity while preserving deterministic behavior.
var officialToolNames = map[string]bool{
	"Bash":         true,
	"Read":         true,
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"WebSearch":    true,
	"WebFetch":     true,
	"TodoRead":     true,
	"TodoWrite":    true,
	"Grep":         true,
	"Glob":         true,
	"LS":           true,
	"NotebookRead": true,
	"NotebookEdit": true,
}

func sanitizeToolName(name string) string {
	if officialToolNames[name] {
		return name
	}
	if strings.HasPrefix(name, "mcp__") || strings.HasPrefix(name, "computer_") {
		return name
	}
	h := sha256.Sum256([]byte(name))
	return "tool_" + hex.EncodeToString(h[:])[:12]
}

func isOAuthAccount(rc accountcontract.RuntimeClass) bool {
	return rc == accountcontract.RuntimeClassOauthRefresh ||
		rc == accountcontract.RuntimeClassOauthDeviceCode
}
