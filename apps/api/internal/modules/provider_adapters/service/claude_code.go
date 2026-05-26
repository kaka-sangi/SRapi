package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const (
	claudeCodeDefaultBeta = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28"
	claudeCodeAgentText   = "You are Claude Code, Anthropic's official CLI for Claude."
)

func isClaudeCodeReverseProxy(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-claude-code-cli")
}

func isClaudeCodeTokenCountReverseProxy(req contract.TokenCountRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Provider.AdapterType), "reverse-proxy-claude-code-cli")
}

func claudeCodeReverseProxyRuntimeIsAPIKey(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func claudeCodeTokenCountReverseProxyRuntimeIsAPIKey(req contract.TokenCountRequest) bool {
	return strings.EqualFold(strings.TrimSpace(string(req.Account.RuntimeClass)), "api_key")
}

func claudeCodeMessagesEndpoint(baseURL string) string {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(strings.TrimRight(endpoint, "/"), "/messages") {
		endpoint += "/messages"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		return endpoint + separator + "beta=true"
	}
	query := parsed.Query()
	if query.Get("beta") == "" {
		query.Set("beta", "true")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func claudeCodeCountTokensEndpoint(baseURL string) string {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(endpoint, "/messages/count_tokens"):
	case strings.HasSuffix(endpoint, "/messages"):
		endpoint = strings.TrimSuffix(endpoint, "/messages") + "/messages/count_tokens"
	default:
		endpoint += "/messages/count_tokens"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		return endpoint + separator + "beta=true"
	}
	query := parsed.Query()
	if query.Get("beta") == "" {
		query.Set("beta", "true")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func claudeCodeMessagesHeaders(req contract.ConversationRequest) http.Header {
	headers := http.Header{
		"Content-Type": {"application/json"},
	}
	beta := requestSetting(req, "anthropic_beta", "anthropic-beta", "claude_code_beta")
	if beta == "" {
		beta = claudeCodeDefaultBeta
	} else {
		beta = mergeCommaList(beta, "claude-code-20250219", "oauth-2025-04-20", "interleaved-thinking-2025-05-14")
	}
	if req.SourceEndpoint == "/v1/messages/count_tokens" {
		beta = mergeCommaList(beta, "token-counting-2024-11-01")
	}
	headers.Set("Anthropic-Beta", beta)
	headers.Set("Anthropic-Version", defaultRequestSetting(req, "2023-06-01", "anthropic_version", "anthropic-version"))
	headers.Set("X-App", defaultRequestSetting(req, "cli", "x_app", "x-app"))
	headers.Set("X-Stainless-Retry-Count", defaultRequestSetting(req, "0", "x_stainless_retry_count", "x-stainless-retry-count"))
	headers.Set("X-Stainless-Runtime", defaultRequestSetting(req, "node", "x_stainless_runtime", "x-stainless-runtime"))
	headers.Set("X-Stainless-Lang", defaultRequestSetting(req, "js", "x_stainless_lang", "x-stainless-lang"))
	headers.Set("X-Stainless-Timeout", defaultRequestSetting(req, "600", "x_stainless_timeout", "x-stainless-timeout"))
	if sessionID := defaultRequestSetting(req, req.RequestID, "claude_code_session_id", "x_claude_code_session_id", "X-Claude-Code-Session-Id", "session_id"); sessionID != "" {
		headers.Set("X-Claude-Code-Session-Id", sessionID)
	}
	if clientRequestID := defaultRequestSetting(req, req.RequestID, "claude_client_request_id", "x_client_request_id", "X-Client-Request-Id", "x-client-request-id"); clientRequestID != "" {
		headers.Set("x-client-request-id", clientRequestID)
	}
	if req.Stream {
		headers.Set("Accept", "text/event-stream")
		headers.Set("Accept-Encoding", "identity")
	} else {
		headers.Set("Accept", "application/json")
	}
	return headers
}

func claudeCodeTokenCountHeaders(req contract.TokenCountRequest) http.Header {
	textReq := tokenCountTextRequest(req)
	textReq.SourceEndpoint = "/v1/messages/count_tokens"
	textReq.Stream = false
	return claudeCodeMessagesHeaders(textReq)
}

func claudeCodeMessagesPayload(req contract.ConversationRequest, raw []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if firstSystemTextHasPrefix(payload["system"], "x-anthropic-billing-header:") {
		return raw, nil
	}
	payload["system"] = claudeCodeSystemBlocks(req, raw, payload["system"])
	return json.Marshal(payload)
}

func claudeCodeTokenCountPayload(req contract.TokenCountRequest, raw []byte) ([]byte, error) {
	return claudeCodeMessagesPayload(tokenCountTextRequest(req), raw)
}

func tokenCountTextRequest(req contract.TokenCountRequest) contract.ConversationRequest {
	return contract.ConversationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: req.SourceProtocol,
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.Model,
		Provider:       req.Provider,
		Account:        req.Account,
		Mapping:        req.Mapping,
		Credential:     req.Credential,
	}
}

func claudeCodeSystemBlocks(req contract.ConversationRequest, raw []byte, original any) []map[string]any {
	blocks := []map[string]any{
		{"type": "text", "text": claudeCodeBillingHeader(req, raw, original)},
		{"type": "text", "text": claudeCodeAgentText},
	}
	blocks = append(blocks, claudeCodeOriginalSystemBlocks(original)...)
	return blocks
}

func claudeCodeOriginalSystemBlocks(value any) []map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []map[string]any{{"type": "text", "text": text}}
		}
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(block["text"]))
			if text == "" || text == "<nil>" || strings.HasPrefix(text, "x-anthropic-billing-header:") || text == claudeCodeAgentText {
				continue
			}
			next := cloneMap(block)
			next["type"] = "text"
			next["text"] = text
			out = append(out, next)
		}
		return out
	case map[string]any:
		text := strings.TrimSpace(fmt.Sprint(typed["text"]))
		if text != "" && text != "<nil>" {
			next := cloneMap(typed)
			next["type"] = "text"
			next["text"] = text
			return []map[string]any{next}
		}
	}
	return nil
}

func firstSystemTextHasPrefix(value any, prefix string) bool {
	switch typed := value.(type) {
	case string:
		return strings.HasPrefix(strings.TrimSpace(typed), prefix)
	case []any:
		if len(typed) == 0 {
			return false
		}
		if block, ok := typed[0].(map[string]any); ok {
			return strings.HasPrefix(strings.TrimSpace(fmt.Sprint(block["text"])), prefix)
		}
	case map[string]any:
		return strings.HasPrefix(strings.TrimSpace(fmt.Sprint(typed["text"])), prefix)
	}
	return false
}

func claudeCodeBillingHeader(req contract.ConversationRequest, raw []byte, original any) string {
	version := defaultRequestSetting(req, "2.1.63", "claude_code_version", "cc_version")
	build := requestSetting(req, "claude_code_build", "claude_code_build_hash", "cc_build")
	if build == "" {
		build = claudeCodeBuildFingerprint(version, original)
	}
	entrypoint := defaultRequestSetting(req, "cli", "claude_code_entrypoint", "cc_entrypoint")
	cch := requestSetting(req, "claude_code_cch", "cc_cch")
	if cch == "" {
		cch = sha256HexPrefix(raw, 5)
	}
	header := fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=%s; cch=%s;", version, build, entrypoint, cch)
	if workload := requestSetting(req, "claude_code_workload", "cc_workload"); workload != "" {
		header += " cc_workload=" + workload + ";"
	}
	return header
}

func claudeCodeBuildFingerprint(version string, original any) string {
	input := strings.TrimSpace(fmt.Sprint(original)) + strings.TrimSpace(version)
	if input == "" {
		input = "claude-code"
	}
	return sha256HexPrefix([]byte(input), 3)
}

func sha256HexPrefix(raw []byte, n int) string {
	sum := sha256.Sum256(raw)
	encoded := hex.EncodeToString(sum[:])
	if n <= 0 || n > len(encoded) {
		return encoded
	}
	return encoded[:n]
}

func defaultRequestSetting(req contract.ConversationRequest, fallback string, keys ...string) string {
	if value := requestSetting(req, keys...); value != "" {
		return value
	}
	return fallback
}

func mergeCommaList(base string, required ...string) string {
	seen := map[string]bool{}
	parts := make([]string, 0, len(required)+len(strings.Split(base, ",")))
	for _, value := range strings.Split(base, ",") {
		token := strings.TrimSpace(value)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		parts = append(parts, token)
	}
	for _, value := range required {
		token := strings.TrimSpace(value)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		parts = append(parts, token)
	}
	return strings.Join(parts, ",")
}
