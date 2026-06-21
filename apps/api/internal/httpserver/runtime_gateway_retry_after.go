package httpserver

import (
	"errors"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// providerGatewayRetryAfter extracts the upstream-parsed Retry-After timestamp
// from a provider error. Adapter-level classifiers populate this from the
// real upstream signal (Anthropic 5h/7d unified-window resets, Codex quota
// window resets, Gemini retryDelay, generic `Retry-After`). Returning the
// parsed *time.Time lets the failover handler set a cooldown that matches
// the upstream's actual reset window instead of falling back to the
// rate-limit module's minutes-scale default.
//
// Returns (0, false) when no provider error is present, when no RetryAfter
// was parsed, or when the parsed time is not in the future relative to
// `now`. The bool sentinel lets the caller skip the parsed path entirely
// without conflating "no signal" with "signal of zero duration".
func providerGatewayRetryAfter(err error, now time.Time) (time.Duration, bool) {
	var providerErr provideradaptercontract.ProviderError
	if !errors.As(err, &providerErr) {
		return 0, false
	}
	if providerErr.RetryAfter == nil {
		return 0, false
	}
	resetAt := providerErr.RetryAfter.UTC()
	cutoff := now.UTC()
	if !resetAt.After(cutoff) {
		return 0, false
	}
	return resetAt.Sub(cutoff), true
}

// recordGatewayCooldownForUpstreamFailure picks the right cooldown duration
// for an upstream failure and records it against the candidate account.
// Decision order (first match wins):
//
//   - Adapter-parsed RetryAfter on ProviderError — the real upstream window
//     for Anthropic 5h/7d, Codex quota, Gemini retryDelay, generic
//     `Retry-After`.
//   - Generic `Retry-After` parsed by ClassifyUpstreamError — kept for
//     non-provider errors and for the rare provider error that didn't
//     populate RetryAfter.
//   - Account-targeted upstream cooldown statuses (429 / 408 / 5xx) — the
//     rate-limit module clamps to its own default minimum.
//
// Network-only failures (errorClass == "" and upstreamStatus == 0) are
// deliberately not cooled down: the account isn't the cause, and the
// runtime's per-candidate retry policy is the right control there. The
// caller must check that gate before invoking this helper.
func (s *Server) recordGatewayCooldownForUpstreamFailure(account accountcontract.ProviderAccount, canonicalModel string, err error) {
	errorClass, upstreamStatus, _ := providerGatewayError(err)
	if errorClass == "" && upstreamStatus == 0 {
		return
	}
	if cooldown, ok := providerGatewayRetryAfter(err, time.Now()); ok {
		s.runtime.recordGatewayAccountRateLimitCooldown(account, canonicalModel, cooldown)
		return
	}
	decision := ClassifyUpstreamError(upstreamStatus, nil, err)
	switch {
	case decision.RetryAfterMs > 0:
		s.runtime.recordGatewayAccountRateLimitCooldown(account, canonicalModel, time.Duration(decision.RetryAfterMs)*time.Millisecond)
	case isAccountTargetedUpstreamCooldownStatus(upstreamStatus):
		s.runtime.recordGatewayAccountRateLimitCooldown(account, canonicalModel, 0)
	}
}
