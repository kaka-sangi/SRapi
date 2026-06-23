package service

import (
	"net/http"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// Warmup interception ("拦截预热请求"): when an account opts in via metadata
// intercept_warmup_requests, low-value priming requests (title generation, topic
// extraction) are answered with a canned response instead of being forwarded
// upstream — saving tokens and avoiding needless load/abuse signal on the
// account. Mirrors sub2api's intercept_warmup_requests.

// accountInterceptWarmupEnabled reports whether the account opts into warmup
// interception. Strictly a boolean true (matching sub2api), so a stray string
// never silently enables it.
func accountInterceptWarmupEnabled(metadata map[string]any) bool {
	enabled, _ := metadata["intercept_warmup_requests"].(bool)
	return enabled
}

// warmupMarkers are the specific priming-prompt phrases (lowercased) that
// identify a warmup/title request. They are intentionally precise to avoid ever
// short-circuiting a genuine request.
var warmupMarkers = []string{
	"please write a 5-10 word title for the following conversation",
	"analyze if this message indicates a new conversation topic",
	"extract a 2-3 word title",
}

// isWarmupRequest detects a priming/title request from the request body.
// Two detection modes (mirrors sub2api):
//  1. Phrase-based: known title-generation/topic-analysis prompts.
//  2. Token-budget: max_tokens=1 on a small model (haiku) — clients use this
//     to probe connectivity without burning real tokens.
func isWarmupRequest(req contract.ConversationRequest) bool {
	if isMaxTokensOneSmallModel(req) {
		return true
	}
	body := strings.ToLower(string(req.RawBody))
	if strings.TrimSpace(body) == "" {
		body = strings.ToLower(req.Instructions)
		for _, message := range req.Messages {
			for _, part := range message.Parts {
				body += "\n" + strings.ToLower(part.Text)
			}
		}
	}
	for _, marker := range warmupMarkers {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

// isMaxTokensOneSmallModel detects max_tokens=1 on a small/cheap model — a
// common connectivity probe pattern used by Claude Code and similar clients.
func isMaxTokensOneSmallModel(req contract.ConversationRequest) bool {
	if req.MaxOutputTokens == nil || *req.MaxOutputTokens != 1 {
		return false
	}
	model := strings.ToLower(req.Model)
	for _, prefix := range warmupModelPrefixes {
		if strings.Contains(model, prefix) {
			return true
		}
	}
	return false
}

var warmupModelPrefixes = []string{
	"haiku",
	"gpt-4o-mini",
	"gemini-2.0-flash",
}

// warmupMockResponse is the canned, zero-cost response returned for an
// intercepted warmup request.
func warmupMockResponse(req contract.ConversationRequest) contract.ConversationResponse {
	return contract.ConversationResponse{
		ID:         "warmup_" + strings.TrimSpace(req.RequestID),
		Parts:      []contract.ContentPart{{Kind: contract.ContentPartText, Text: "OK"}},
		StopReason: contract.StopReasonEndTurn,
		StatusCode: http.StatusOK,
		Usage:      contract.Usage{},
	}
}
