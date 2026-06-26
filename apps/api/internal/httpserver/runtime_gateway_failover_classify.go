package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// UpstreamFailoverDecision is the structured verdict for an upstream failure.
//
// Class names mirror the directive's four buckets ("transient", "account_bad",
// "client_bad", "server_bad"). The existing srapi error_class taxonomy
// (rate_limit / quota_exhausted / auth_failed / etc.) is still used by
// gatewayShouldFailover for finer-grained per-candidate retry decisions — this
// classifier sits on top to give callers a single boolean failover/blacklist
// answer that mirrors sub2api's transport-error + status-code policy.
type UpstreamFailoverDecision struct {
	// Class is one of: "transient", "account_bad", "client_bad", "server_bad".
	Class string
	// ShouldFailover requests the caller to retry against a different candidate.
	ShouldFailover bool
	// ShouldBlacklist requests the caller to mark the credential bad and stop
	// scheduling it (auth revoked / forbidden). Maps to applyProviderAccountProtection.
	ShouldBlacklist bool
	// RetryAfterMs is parsed from the Retry-After header (seconds or HTTP-date)
	// when the upstream returned 429/503 with one. 0 when absent/invalid.
	RetryAfterMs int
}

// authErrorCooldownMs is the cooldown duration in milliseconds applied to
// accounts that receive a 401 Unauthorized. Instead of permanently blacklisting
// (NeedsReauth), the account is temporarily cooled down for 30 minutes so
// transient credential issues can self-heal (token rotation, eventual
// consistency, etc.). Matches CLIProxyAPI's 2-30 minute cooldown policy.
const authErrorCooldownMs = 30 * 60 * 1000 // 30 minutes

// upstreamFailoverTransientNetworkMarkers are case-insensitive substrings that
// indicate a transient network-layer blip — match sub2api's
// classifyOpenAITransportError "transient" inverse set (timeouts, EOFs, resets).
var upstreamFailoverTransientNetworkMarkers = []string{
	"i/o timeout",
	"deadline exceeded",
	"connection reset by peer",
	"unexpected eof",
	"broken pipe",
	"timeout exceeded while awaiting headers",
}

// upstreamFailoverPersistentNetworkMarkers mirrors sub2api's
// openAIPersistentTransportErrorMarkers — string markers for proxy/DNS faults
// that are operator-actionable rather than retry-worthy on the same credential.
var upstreamFailoverPersistentNetworkMarkers = []string{
	"authentication failed",
	"proxy authentication required",
	"connection refused",
	"no route to host",
	"network is unreachable",
	"no such host",
}

// ClassifyUpstreamError returns a UpstreamFailoverDecision for an upstream
// failure. Inputs:
//   - statusCode: HTTP status from the upstream (0 when network failed before headers).
//   - errBody:    response body (used to parse Retry-After when only headers absent;
//     currently a placeholder for body-based rate-limit hints).
//   - networkErr: the transport error returned by Do() (nil when HTTP status received).
//
// Decision policy (matches the user directive verbatim):
//   - 401             -> "account_bad",  Failover=true,   30-min cooldown (no blacklist)
//   - 403             -> "account_bad",  Blacklist=true,  Failover=true
//   - 429             -> "transient",    Failover=true,   RetryAfterMs parsed
//   - 5xx             -> "server_bad",   Failover=true,   RetryAfterMs parsed for 503
//   - 408 / EOF / net timeout / context.DeadlineExceeded -> "transient", Failover=true
//   - other 4xx       -> "client_bad",   no failover (caller's request is wrong)
//   - context.Canceled -> "transient" with Failover=false (client gone — don't burn another credential)
//   - persistent network markers (proxy/DNS down) -> "account_bad", Blacklist=true, Failover=true
//   - other network errors -> "transient", Failover=true
func ClassifyUpstreamError(statusCode int, errBody []byte, networkErr error) UpstreamFailoverDecision {
	return classifyUpstreamErrorWithHeader(statusCode, nil, errBody, networkErr)
}

// classifyUpstreamErrorWithHeader is the header-aware variant used internally
// when the caller has access to the upstream response headers (preferred over
// scraping errBody for Retry-After). Kept separate so the public API stays
// minimal until the header path is wired through.
func classifyUpstreamErrorWithHeader(statusCode int, headers http.Header, errBody []byte, networkErr error) UpstreamFailoverDecision {
	// Network failure preempts status code — no HTTP round-trip completed.
	if statusCode == 0 && networkErr != nil {
		return classifyNetworkError(networkErr)
	}

	// Parse upstream error body for provider-specific error codes (OpenAI,
	// Anthropic, etc.) that refine the HTTP-status-only classification.
	bodyClass := classifyUpstreamErrorBody(errBody)

	switch {
	case statusCode == http.StatusUnauthorized:
		return UpstreamFailoverDecision{
			Class:          "account_bad",
			ShouldFailover: true,
			RetryAfterMs:   authErrorCooldownMs,
		}
	case statusCode == http.StatusForbidden:
		// Distinguish quota exhaustion from permanent bans. OpenAI returns
		// 403 for both "insufficient_quota" and "account_deactivated".
		if bodyClass == "quota_exhausted" || bodyClass == "rate_limit" {
			return UpstreamFailoverDecision{
				Class:          "transient",
				ShouldFailover: true,
				RetryAfterMs:   authErrorCooldownMs,
			}
		}
		if bodyClass == "content_policy" {
			return UpstreamFailoverDecision{
				Class:          "client_bad",
				ShouldFailover: false,
			}
		}
		return UpstreamFailoverDecision{
			Class:           "account_bad",
			ShouldFailover:  true,
			ShouldBlacklist: true,
		}
	case statusCode == http.StatusTooManyRequests:
		return UpstreamFailoverDecision{
			Class:          "transient",
			ShouldFailover: true,
			RetryAfterMs:   parseRetryAfterMillis(headers),
		}
	case statusCode == http.StatusRequestTimeout:
		return UpstreamFailoverDecision{
			Class:          "transient",
			ShouldFailover: true,
		}
	case statusCode >= http.StatusInternalServerError && statusCode <= 599:
		return UpstreamFailoverDecision{
			Class:          "server_bad",
			ShouldFailover: true,
			RetryAfterMs:   parseRetryAfterMillis(headers),
		}
	case statusCode >= 400 && statusCode < 500:
		return UpstreamFailoverDecision{
			Class:          "client_bad",
			ShouldFailover: false,
		}
	}

	// 1xx/2xx/3xx — not a failure. Caller should not be invoking the classifier;
	// return a transient no-op so we don't accidentally mark traffic as failed.
	return UpstreamFailoverDecision{Class: "transient"}
}

func classifyNetworkError(err error) UpstreamFailoverDecision {
	// Client disconnect — don't burn another credential.
	if errors.Is(err, context.Canceled) {
		return UpstreamFailoverDecision{Class: "transient", ShouldFailover: false}
	}
	// Typed checks first — portable and unambiguous (mirrors
	// sub2api/openai_upstream_transport_error.go).
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return UpstreamFailoverDecision{
			Class:           "account_bad",
			ShouldFailover:  true,
			ShouldBlacklist: true,
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return UpstreamFailoverDecision{
			Class:           "account_bad",
			ShouldFailover:  true,
			ShouldBlacklist: true,
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return UpstreamFailoverDecision{Class: "transient", ShouldFailover: true}
	}

	msg := strings.ToLower(err.Error())
	for _, marker := range upstreamFailoverPersistentNetworkMarkers {
		if strings.Contains(msg, marker) {
			return UpstreamFailoverDecision{
				Class:           "account_bad",
				ShouldFailover:  true,
				ShouldBlacklist: true,
			}
		}
	}
	for _, marker := range upstreamFailoverTransientNetworkMarkers {
		if strings.Contains(msg, marker) {
			return UpstreamFailoverDecision{Class: "transient", ShouldFailover: true}
		}
	}

	// Unknown — treat as transient and let the same-candidate retry policy +
	// gatewayShouldFailover decide whether to actually retry.
	return UpstreamFailoverDecision{Class: "transient", ShouldFailover: true}
}

// classifyErrorPhase maps an errorClass + upstream HTTP status onto one of the
// six sub2api error-phase buckets:
//   - "auth"     — 401/403 or auth_failed/permission_denied/credential_error
//   - "routing"  — no_available_account (the scheduler rejected every candidate)
//   - "upstream" — any 4xx/5xx response received from the upstream
//   - "network"  — transport-level failure (no HTTP response, e.g. DNS/TCP)
//   - "request"  — client-side validation rejected (invalid_request, bad payload)
//   - "internal" — gateway internal error (panic / unexpected nil)
//
// Falls back to "upstream" when the inputs are ambiguous but a status was
// received, and to "internal" otherwise so unknown failures don't get
// silently bucketed as routing.
func classifyErrorPhase(errorClass string, statusCode int) string {
	switch strings.ToLower(strings.TrimSpace(errorClass)) {
	case "no_available_account":
		return "routing"
	case "auth_failed", "auth_error", "permission_denied", "credential_error", "forbidden":
		return "auth"
	case "network_error", "transport_error":
		return "network"
	case "invalid_request", "validation_required", "bad_request":
		return "request"
	case "internal_error", "panic":
		return "internal"
	}
	if statusCode == 401 || statusCode == 403 {
		return "auth"
	}
	if statusCode >= 400 && statusCode < 500 {
		// 4xx that wasn't already auth: treat as upstream (the upstream rejected).
		if statusCode == http.StatusBadRequest {
			return "request"
		}
		return "upstream"
	}
	if statusCode >= 500 && statusCode < 600 {
		return "upstream"
	}
	if statusCode == 0 && strings.TrimSpace(errorClass) == "" {
		return "internal"
	}
	if statusCode == 0 {
		return "network"
	}
	return "upstream"
}

// classifyErrorOwner returns the responsibility bucket: client | provider |
// platform. Used by the admin panel to colour-code rows by who needs to act.
func classifyErrorOwner(phase string) string {
	switch phase {
	case "request":
		return "client"
	case "auth", "upstream", "network":
		return "provider"
	case "routing", "internal":
		return "platform"
	default:
		return "platform"
	}
}

// classifyErrorSource returns where the error originated, mirroring sub2api's
// source taxonomy: client_request | upstream_http | gateway.
func classifyErrorSource(phase string) string {
	switch phase {
	case "request":
		return "client_request"
	case "auth", "upstream":
		return "upstream_http"
	case "network", "routing", "internal":
		return "gateway"
	default:
		return "gateway"
	}
}

// upstreamRequestIDFromHeaders extracts an upstream provider's request id from
// the failing response headers. Checks the OpenAI/Anthropic/Codex conventions
// in order: x-request-id, openai-request-id, x-codex-request-id, anthropic-request-id.
func upstreamRequestIDFromHeaders(headers http.Header) string {
	if headers == nil {
		return ""
	}
	for _, key := range []string{
		"X-Request-Id",
		"Openai-Request-Id",
		"X-Codex-Request-Id",
		"Anthropic-Request-Id",
		"Request-Id",
	} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

// maxRetryAfterMillis caps any parsed Retry-After value so a malicious
// upstream returning an absurd delay (e.g. Retry-After: 31536000) cannot
// stall the account indefinitely. 5 minutes matches axonhub's
// MaxRetryAfterDuration.
const maxRetryAfterMillis = 5 * 60 * 1000

// minRetryAfterMillis is the floor for a positive Retry-After. Values
// below 1 second are too aggressive for a retry and likely erroneous.
const minRetryAfterMillis = 1000

// parseRetryAfterMillis parses the Retry-After header (RFC 7231 §7.1.3): either
// a delta-seconds integer or an HTTP-date. Returns 0 when absent/invalid so the
// caller falls back to its own backoff schedule. The result is clamped to
// [minRetryAfterMillis, maxRetryAfterMillis] when positive.
func parseRetryAfterMillis(headers http.Header) int {
	if headers == nil {
		return 0
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	var ms int
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0
		}
		ms = secs * 1000
	} else if t, err := http.ParseTime(raw); err == nil {
		delta := time.Until(t)
		if delta <= 0 {
			return 0
		}
		ms = int(delta / time.Millisecond)
	} else {
		return 0
	}
	if ms < minRetryAfterMillis {
		ms = minRetryAfterMillis
	}
	if ms > maxRetryAfterMillis {
		ms = maxRetryAfterMillis
	}
	return ms
}

// classifyUpstreamErrorBody parses the JSON error body returned by upstream
// providers (OpenAI, Anthropic, etc.) and returns a refined error class.
// Returns "" when the body is empty, unparseable, or contains no recognized
// error code. This lets the status-code-level classifier refine its decision.
func classifyUpstreamErrorBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	code := strings.ToLower(strings.TrimSpace(envelope.Error.Code))
	typ := strings.ToLower(strings.TrimSpace(envelope.Error.Type))
	msg := strings.ToLower(strings.TrimSpace(envelope.Error.Message))

	switch {
	case code == "insufficient_quota" || strings.Contains(msg, "insufficient_quota") || strings.Contains(msg, "billing_hard_limit"):
		return "quota_exhausted"
	case code == "rate_limit_exceeded" || code == "rate_limit" || strings.Contains(code, "usage_limit"):
		return "rate_limit"
	case code == "content_policy_violation" || strings.Contains(code, "content_policy") || strings.Contains(code, "content_moderation"):
		return "content_policy"
	case code == "account_deactivated" || strings.Contains(msg, "account deactivated") || strings.Contains(msg, "account has been disabled"):
		return "account_deactivated"
	case typ == "overloaded_error" || strings.Contains(msg, "overloaded"):
		return "overloaded"
	case strings.Contains(msg, "safety") || strings.Contains(msg, "safety_system"):
		return "content_policy"
	}
	return ""
}
