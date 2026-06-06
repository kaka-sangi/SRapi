package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	sessionaffinitycontract "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
)

// Session affinity ("会话粘度") keeps a multi-turn conversation pinned to the
// upstream account that served its earlier turns. Re-using one account
// maximizes provider-side prompt-cache hits and keeps session-scoped upstream
// state consistent. Unlike the explicit X-SRapi-* sticky headers, derivation
// here works for off-the-shelf Anthropic/OpenAI clients that never send SRapi's
// proprietary headers.
const (
	// gatewaySessionAffinityTTL is how long a derived/explicit conversation
	// stays pinned to its account. Active conversations refresh the TTL on each
	// turn; idle ones release the account. Mirrors sub2api's 1h stickySessionTTL.
	gatewaySessionAffinityTTL = time.Hour
	// gatewaySessionDigestMaxSegments caps the leading turns that feed a digest
	// chain, bounding key size for very long conversations while preserving the
	// longest-prefix match across turns.
	gatewaySessionDigestMaxSegments = 24
	// gatewaySessionDigestHashLen is the hex length of each per-turn content
	// hash in a digest chain.
	gatewaySessionDigestHashLen = 16
)

// legacyAnthropicUserIDRegex matches Claude Code's legacy metadata.user_id
// format user_<device>_account_<uuid>_session_<uuid>; group 3 is the session id.
var legacyAnthropicUserIDRegex = regexp.MustCompile(`^user_([a-fA-F0-9]{64})_account_([a-fA-F0-9-]*)_session_([a-fA-F0-9-]{36})$`)

// gatewaySessionScope isolates one API key's sessions from another's, so two
// keys that happen to send the same session identifier never share a binding.
func gatewaySessionScope(apiKeyID int) string {
	return strconv.Itoa(apiKeyID)
}

// deriveGatewaySessionAffinity derives a stable session key for the request when
// the client did not send an explicit X-SRapi sticky header. It returns the key
// and a source label, or "" when no session identity can be derived (e.g.
// non-conversational endpoints). The priority cascade mirrors sub2api:
//  1. Anthropic metadata.user_id session id (Claude Code),
//  2. OpenAI/Codex prompt_cache_key,
//  3. explicit session/conversation id headers,
//  4. a content digest chain over system + messages (longest-prefix matched).
func deriveGatewaySessionAffinity(r *http.Request, canonical gatewaycontract.CanonicalRequest) (string, string) {
	if key, source := explicitSessionIdentity(r, canonical); key != "" {
		return key, source
	}
	if chain := buildGatewayDigestChain(canonical); chain != "" {
		return chain, "derived:content_digest"
	}
	return "", ""
}

func explicitSessionIdentity(r *http.Request, canonical gatewaycontract.CanonicalRequest) (string, string) {
	var probe struct {
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
		PromptCacheKey string `json:"prompt_cache_key"`
	}
	if len(canonical.RawBody) > 0 {
		_ = json.Unmarshal(canonical.RawBody, &probe)
	}
	if seed := anthropicSessionSeed(probe.Metadata.UserID); seed != "" {
		return "sid:auid:" + shortDigest(seed), "derived:anthropic_metadata"
	}
	if pck := strings.TrimSpace(probe.PromptCacheKey); pck != "" {
		return "sid:pck:" + shortDigest(pck), "derived:prompt_cache_key"
	}
	if r != nil {
		for _, header := range []string{"Session-Id", "X-Session-Id", "Conversation-Id", "X-Conversation-Id"} {
			if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
				return "sid:hdr:" + shortDigest(value), "derived:session_header"
			}
		}
	}
	return "", ""
}

// anthropicSessionSeed extracts a session-stable seed from an Anthropic
// metadata.user_id, supporting both the JSON {device_id,account_uuid,session_id}
// form and the legacy user_..._session_<uuid> form. It returns "" for
// unrecognized values so we never pin every conversation of a user that sends a
// per-user (not per-session) identifier; those fall through to the digest chain.
func anthropicSessionSeed(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if strings.HasPrefix(userID, "{") {
		var parsed struct {
			DeviceID    string `json:"device_id"`
			AccountUUID string `json:"account_uuid"`
			SessionID   string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(userID), &parsed); err != nil {
			return ""
		}
		if strings.TrimSpace(parsed.SessionID) == "" {
			return ""
		}
		return strings.Join([]string{parsed.DeviceID, parsed.AccountUUID, parsed.SessionID}, "|")
	}
	if matches := legacyAnthropicUserIDRegex.FindStringSubmatch(userID); matches != nil {
		return strings.Join([]string{matches[1], matches[2], matches[3]}, "|")
	}
	return ""
}

// buildGatewayDigestChain builds a longest-prefix-matchable conversation digest:
// s:<hash>-u:<hash>-a:<hash>-...  A later turn's chain is a prefix extension of
// the earlier turn's chain, so the binding store resolves it back to the same
// account. The leading-segment cap bounds key size for very long conversations.
func buildGatewayDigestChain(canonical gatewaycontract.CanonicalRequest) string {
	parts := make([]string, 0, gatewaySessionDigestMaxSegments)
	if system := strings.TrimSpace(canonical.Instructions); system != "" {
		parts = append(parts, "s:"+shortDigest(system))
	}
	turns := canonical.Messages
	for _, message := range turns {
		if len(parts) >= gatewaySessionDigestMaxSegments {
			break
		}
		content := digestMessageContent(message.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, digestRolePrefix(message.Role)+":"+shortDigest(content))
	}
	// Responses-style requests carry input items rather than chat messages.
	if len(canonical.Messages) == 0 {
		for _, block := range canonical.InputItems {
			if len(parts) >= gatewaySessionDigestMaxSegments {
				break
			}
			content := digestBlockContent(block)
			if strings.TrimSpace(content) == "" {
				continue
			}
			parts = append(parts, digestRolePrefix(block.Role)+":"+shortDigest(content))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return sessionaffinitycontract.ChainMarker + strings.Join(parts, "-")
}

func digestMessageContent(blocks []gatewaycontract.ContentBlock) string {
	var builder strings.Builder
	for _, block := range blocks {
		builder.WriteString(digestBlockContent(block))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func digestBlockContent(block gatewaycontract.ContentBlock) string {
	switch {
	case strings.TrimSpace(block.Text) != "":
		return block.Text
	case strings.TrimSpace(block.ToolArgumentsJSON) != "":
		return "tool_call:" + block.ToolName + ":" + block.ToolArgumentsJSON
	case strings.TrimSpace(block.ToolResultForID) != "":
		return "tool_result:" + block.ToolResultForID
	case strings.TrimSpace(block.FileID) != "":
		return "file:" + block.FileID
	case strings.TrimSpace(block.MediaURL) != "":
		return "media:" + block.MediaURL
	case len(block.MediaBase64) > 0:
		return "media_b64:" + block.MIMEType + ":" + strconv.Itoa(len(block.MediaBase64))
	default:
		return ""
	}
}

func digestRolePrefix(role string) string {
	if strings.EqualFold(strings.TrimSpace(role), "assistant") {
		return "a"
	}
	return "u"
}

func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if len(encoded) > gatewaySessionDigestHashLen {
		return encoded[:gatewaySessionDigestHashLen]
	}
	return encoded
}

// lookupGatewaySessionAffinity returns the account previously bound to the
// session, refreshing the binding TTL on a hit. It is best-effort: any store
// error degrades to "no binding" so a transient cache outage never fails a
// request.
func (rt *runtimeState) lookupGatewaySessionAffinity(ctx context.Context, apiKeyID int, sessionKey string) (int, bool) {
	if rt == nil || rt.sessionAffinity == nil {
		return 0, false
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || apiKeyID <= 0 {
		return 0, false
	}
	binding, err := rt.sessionAffinity.Lookup(ctx, gatewaySessionScope(apiKeyID), sessionKey, gatewaySessionAffinityTTL)
	if err != nil {
		if rt.logger != nil {
			rt.logger.Warn("session affinity lookup failed", "error", err)
		}
		return 0, false
	}
	if !binding.Found() {
		return 0, false
	}
	return binding.AccountID, true
}

// bindGatewaySessionAffinity records that accountID served this session so the
// next turn reuses it. Best-effort; binding failures never fail the request.
func (rt *runtimeState) bindGatewaySessionAffinity(ctx context.Context, apiKeyID int, sessionKey string, accountID int) {
	if rt == nil || rt.sessionAffinity == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || apiKeyID <= 0 || accountID <= 0 {
		return
	}
	if err := rt.sessionAffinity.Bind(ctx, gatewaySessionScope(apiKeyID), sessionKey, accountID, gatewaySessionAffinityTTL); err != nil {
		if rt.logger != nil {
			rt.logger.Warn("session affinity bind failed", "error", err)
		}
	}
}

func candidatesContainAccount(candidates []schedulercontract.Candidate, accountID int) bool {
	for _, candidate := range candidates {
		if candidate.Account.ID == accountID {
			return true
		}
	}
	return false
}
