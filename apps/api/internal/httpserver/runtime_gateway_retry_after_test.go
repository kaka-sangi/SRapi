package httpserver

import (
	"errors"
	"net/http"
	"testing"
	"time"

	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// Anthropic's 5h / 7d unified windows do not surface via the generic
// `Retry-After` header — the upstream returns dedicated headers
// (`anthropic-ratelimit-unified-5h-reset`) the adapter classifier turns
// into ProviderError.RetryAfter. The failover layer must honor the parsed
// value so the cooldown matches the real multi-hour reset instead of
// falling back to the rate-limit module's minute-scale default.
func TestProviderGatewayRetryAfterHonorsParsedAnthropicWindow(t *testing.T) {
	now := time.Now().UTC()
	resetIn5h := now.Add(5 * time.Hour)
	providerErr := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		Message:    "anthropic 5h window exhausted",
		RetryAfter: &resetIn5h,
	}
	cooldown, ok := providerGatewayRetryAfter(providerErr, now)
	if !ok {
		t.Fatalf("expected ok=true for future anthropic reset")
	}
	if cooldown < 4*time.Hour+59*time.Minute || cooldown > 5*time.Hour+time.Second {
		t.Fatalf("expected cooldown ~5h, got %s", cooldown)
	}
}

func TestProviderGatewayRetryAfterRejectsPastResetAt(t *testing.T) {
	now := time.Now().UTC()
	resetInPast := now.Add(-time.Minute)
	providerErr := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: &resetInPast,
	}
	if _, ok := providerGatewayRetryAfter(providerErr, now); ok {
		t.Fatalf("expected ok=false for past reset")
	}
}

func TestProviderGatewayRetryAfterIgnoresMissingRetryAfter(t *testing.T) {
	providerErr := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		// No RetryAfter — the caller must fall through to the generic
		// ClassifyUpstreamError + rate-limit module default cooldown.
	}
	if _, ok := providerGatewayRetryAfter(providerErr, time.Now()); ok {
		t.Fatalf("expected ok=false when RetryAfter is nil")
	}
}

func TestProviderGatewayRetryAfterIgnoresNonProviderError(t *testing.T) {
	if _, ok := providerGatewayRetryAfter(errors.New("network error"), time.Now()); ok {
		t.Fatalf("expected ok=false for non-ProviderError")
	}
	if _, ok := providerGatewayRetryAfter(nil, time.Now()); ok {
		t.Fatalf("expected ok=false for nil err")
	}
}

// A wrapped provider error (e.g. through a context-augmenting transport
// layer) must still surface the parsed RetryAfter — errors.As unwraps
// through the chain.
func TestProviderGatewayRetryAfterUnwrapsWrappedProviderError(t *testing.T) {
	now := time.Now().UTC()
	resetIn1h := now.Add(time.Hour)
	wrapped := wrappedErr{
		inner: provideradaptercontract.ProviderError{
			Class:      "rate_limit",
			StatusCode: http.StatusTooManyRequests,
			RetryAfter: &resetIn1h,
		},
	}
	cooldown, ok := providerGatewayRetryAfter(wrapped, now)
	if !ok {
		t.Fatalf("expected ok=true through wrapped error")
	}
	if cooldown < 59*time.Minute || cooldown > time.Hour+time.Second {
		t.Fatalf("expected cooldown ~1h, got %s", cooldown)
	}
}

type wrappedErr struct{ inner error }

func (w wrappedErr) Error() string { return w.inner.Error() }
func (w wrappedErr) Unwrap() error { return w.inner }
