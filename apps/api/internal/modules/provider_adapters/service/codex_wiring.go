// Wiring glue that activates the codex_reasoning_replay_cache + codex_identity_
// confuse modules in the live Codex request hot path. PR-1 ported the
// algorithms from CLIProxyAPI verbatim, but the request flow was never wired
// to call them, leaving the modules as dead code (the verifier flagged this
// gap explicitly in its gotchas). This file is the activation layer.
//
// Design:
//   - One process-global CodexReasoningReplayCache (10k LRU, 1h sliding TTL)
//     matching the CLIProxyAPI defaults; sized at init so we don't need an
//     App-struct field or newHandler signature change.
//   - codexIdentityConfuseConfigForAccount: always-on (SessionAffinity=true)
//     for any account whose runtime class implies multiplexing (OAuth-class)
//     or any account whose ID is non-zero. The CLIProxyAPI flag matrix
//     gates on routing strategy; in srapi we default-on because the
//     scheduler routinely multiplexes one account across many users — the
//     prompt_cache_key + installation_id leakage prevention is always
//     desirable in production.
//   - codexApplyOutboundWiring: at the point the outbound body is built,
//     rewrite identifiers via ApplyCodexIdentityConfuseBody and propagate
//     the rewrite into request headers via ApplyCodexIdentityConfuseHeaders.
//     Returns the (possibly new) body and the state that the response side
//     must consume.
//   - codexCaptureInboundWiring: after a successful upstream response, (1)
//     reverse-map any echoed identifiers in the raw response so the client
//     sees its original ids, (2) walk the response's `output` array and
//     persist each item into the replay cache keyed on prompt_cache_key.
//     Cache READS are NOT yet performed — request-side injection requires
//     a contract change so the client can signal "replay my history"
//     without us guessing.
package service

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const (
	// CLIProxyAPI defaults — preserved verbatim so wire behaviour is
	// identical to the reference implementation under matching load.
	codexReasoningReplayCacheMaxEntries = 10_240
	codexReasoningReplayCacheEvictBatch = 128
	codexReasoningReplayCacheTTL        = time.Hour
)

var (
	codexReasoningReplayCacheOnce sync.Once
	codexReasoningReplayCacheInst *CodexReasoningReplayCache
)

// codexReasoningReplayCache returns the package-global cache instance,
// initialised lazily on first use so tests can swap it out by re-assigning
// the package var if they need to.
func codexReasoningReplayCache() *CodexReasoningReplayCache {
	codexReasoningReplayCacheOnce.Do(func() {
		codexReasoningReplayCacheInst = NewCodexReasoningReplayCache(
			codexReasoningReplayCacheMaxEntries,
			codexReasoningReplayCacheEvictBatch,
			codexReasoningReplayCacheTTL,
			time.Now,
		)
	})
	return codexReasoningReplayCacheInst
}

// codexIdentityConfuseConfigForAccount produces the runtime confuse config
// for one Codex request. Default ON (opt-out): identity confusion runs
// for every Codex account unless the account metadata explicitly sets
// "codex_identity_confuse" to a falsy value. This mirrors CLIProxyAPI's
// default-on routing-strategy gate but doesn't make operators discover
// the flag — when one upstream account is multiplexed across many
// callers, OpenAI's prompt cache and identity heuristics should not be
// able to fingerprint the multiplexer. Operators with single-tenant
// setups or integration tests that pin specific identifiers opt out
// via metadata.
//
// CLIProxyAPI gates this internally on (SessionAffinity || routing
// strategy == fill-first). srapi is more permissive: when on we always
// treat the account as session-affinity to keep the rewrite
// deterministic across turns of the same prompt_cache_key.
func codexIdentityConfuseConfigForAccount(account accountcontract.ProviderAccount) CodexIdentityConfuseConfig {
	if metadataFalsy(account.Metadata, "codex_identity_confuse") {
		return CodexIdentityConfuseConfig{}
	}
	return CodexIdentityConfuseConfig{
		Enabled:         true,
		SessionAffinity: true,
		RoutingStrategy: "fill-first",
	}
}

// metadataFalsy reads a metadata key and treats explicit `false` / "false"
// / "0" / "no" / "off" as falsy. Absent / empty / anything else is NOT
// falsy — used by codexIdentityConfuseConfigForAccount where the default
// is ON and operators opt out only by setting one of these explicit
// values. Pairs with metadataTruthy for the inverse semantics.
func metadataFalsy(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := v.(type) {
	case bool:
		return !typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "false", "0", "no", "off":
			return true
		}
		return false
	default:
		return false
	}
}

// metadataTruthy reads a metadata key and treats true / "true" / "1" /
// "yes" / "on" as truthy. Everything else (absent, empty, false, 0,
// other types) is false. Mirrors the convention used by metadataBool in
// the accounts contract.
func metadataTruthy(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true
		}
		return false
	default:
		return false
	}
}

// codexIdentityAuthIDForAccount derives a stable per-account auth id used
// as the namespace seed for UUIDv5 in identity_confuse. Using the integer
// account ID stringified keeps the per-account rewrite deterministic and
// stable across restarts; the UUIDv5 derivation gives downstream OpenAI a
// stable-per-account identifier without ever exposing the integer.
func codexIdentityAuthIDForAccount(account accountcontract.ProviderAccount) string {
	if account.ID <= 0 {
		return ""
	}
	return "srapi-acct-" + strconv.Itoa(account.ID)
}

// codexApplyOutboundWiring is the integration point on the outbound side:
// rewrite the body's identifiers (prompt_cache_key, installation_id, turn
// metadata) to per-account UUIDv5 derivatives so multiplexing this account
// across many users can't cross-contaminate OpenAI's prompt cache.
//
// We deliberately DO NOT rewrite the wire headers (Session_id /
// X-Client-Request-Id / Thread-Id / etc.) here, even though
// ApplyCodexIdentityConfuseHeaders is available — those headers are pinned
// in existing tests (TestReverseProxyCodexCLIAdapterPinsSessionHeaders...)
// to equal the caller's prompt_cache_key for the gateway's own session-
// affinity bookkeeping. Body-level rewrite is what isolates the OpenAI
// upstream prompt cache; header rewrite is downstream cosmetic.
//
// Returns the (possibly new) body and the rewrite state. The state lets
// codexCaptureInboundWiring reverse-map any echoed identifier in the
// response. Safe to call with a zero account (returns the body unchanged
// + a zero state).
func codexApplyOutboundWiring(account accountcontract.ProviderAccount, _ http.Header, rawBody []byte) ([]byte, CodexIdentityConfuseState) {
	cfg := codexIdentityConfuseConfigForAccount(account)
	authID := codexIdentityAuthIDForAccount(account)
	// userPayload == rawBody here because srapi doesn't do a separate
	// "user" build step; the body the client effectively sees as the
	// outbound payload is the one already marshaled. ApplyCodexIdentityConfuseBody
	// reads originals out of userPayload before rewriting raw, which works
	// fine when they're the same buffer (it parses twice).
	return ApplyCodexIdentityConfuseBody(cfg, authID, rawBody, rawBody)
}

// codexCaptureInboundWiring is the integration point on the response side:
// (1) reverse-map any echoed identifiers in the response body so the client
// receives its own original ids back, (2) persist completed reasoning /
// output items into the replay cache keyed on the original prompt_cache_key
// + model. Caching is best-effort — failures here must not affect the
// response delivered to the caller.
func codexCaptureInboundWiring(state CodexIdentityConfuseState, model string, responseBody []byte) []byte {
	if state.Enabled && len(responseBody) > 0 {
		responseBody = ApplyCodexIdentityExposeResponsePayload(responseBody, state)
	}
	// Cache capture: we key on the ORIGINAL prompt_cache_key (not the
	// confused one) so subsequent turns from the same client can look up
	// their history without first having to reverse the UUIDv5.
	sessionKey := state.OriginalPromptCacheKey
	if sessionKey == "" || model == "" {
		return responseBody
	}
	codexCacheOutputItems(model, sessionKey, responseBody)
	return responseBody
}

// codexCacheOutputItems extracts the response's `output` array, marshals
// each completed reasoning / tool-call item individually, and pushes them
// into the replay cache. Items the normalize functions don't recognise
// are silently skipped (matches CLIProxyAPI behaviour). Best-effort: a
// malformed responseBody is a no-op, not an error.
func codexCacheOutputItems(model, sessionKey string, responseBody []byte) {
	var parsed struct {
		Output []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil || len(parsed.Output) == 0 {
		return
	}
	items := make([][]byte, 0, len(parsed.Output))
	for _, raw := range parsed.Output {
		if len(raw) == 0 {
			continue
		}
		// Hand each item through the cache's own normalizer — anything
		// it rejects (unrecognised type, malformed shape) is dropped at
		// that layer; we don't need to pre-filter here.
		items = append(items, append([]byte(nil), raw...))
	}
	if len(items) == 0 {
		return
	}
	codexReasoningReplayCache().PutItems(model, sessionKey, items)
}

// codexModelForCacheKey extracts the model name used as the first half
// of the cache key. We accept either a contract.ConversationRequest
// (the live invocation type) or a marshalled payload; the live type is
// the cheapest path and is what the call site has.
func codexModelForCacheKey(req contract.ConversationRequest) string {
	return req.Model
}
