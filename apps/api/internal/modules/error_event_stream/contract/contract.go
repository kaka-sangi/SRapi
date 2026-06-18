// Package contract defines the public surface of the error_event_stream module:
// an in-memory ring-buffer + fan-out pub/sub for real-time admin error event
// streaming. Mirrors CLIProxyAPI's internal/redisqueue/queue.go SubscribeErrors
// behaviour (256-entry subscriber buffer, drop-when-full policy, multi-subscriber
// fan-out) so an admin SSE consumer can watch upstream failures arrive live
// without polling the persistent ops_error_logs table.
//
// The module is process-local — there is no Redis dependency; the publisher
// keeps a bounded ring of the most recent events and broadcasts every Publish
// to all active subscribers. Subscribers that lag (full channel) are dropped
// with a one-shot warn log, matching the CPA policy.
package contract

import (
	"context"
	"errors"
)

// ErrTooManySubscribers is returned by Publisher.Subscribe when the configured
// max subscriber cap is reached. The admin SSE handler maps this to a 503.
var ErrTooManySubscribers = errors.New("error_event_stream: too many subscribers")

// ErrClosed is returned when a Subscriber method is called after Close.
var ErrClosed = errors.New("error_event_stream: subscriber closed")

// Event is the payload broadcast to subscribers. It carries enough context for
// an operator to triage a failing request live: which user/account/model, the
// classified error phase + class, the upstream status code, and a short body
// excerpt for the message snippet. The struct shape is the SSE wire payload
// (encoded as JSON in the data: frame).
type Event struct {
	// AtUnixMs is the millisecond unix timestamp the event was published.
	AtUnixMs int64 `json:"at_unix_ms"`
	// RequestID is the canonical gateway request id (matches usage_log and
	// ops_error_logs.request_id so operators can cross-reference).
	RequestID string `json:"request_id"`
	// UserID is the authenticated user id, when known.
	UserID *int `json:"user_id,omitempty"`
	// AccountID is the provider account that handled (and failed) the attempt,
	// when known.
	AccountID *int `json:"account_id,omitempty"`
	// ProviderID is the provider that owns the failing account, when known.
	ProviderID *int `json:"provider_id,omitempty"`
	// AccountName is the operator-facing account name. It is non-secret and
	// lets admins triage a live event without resolving ids in another panel.
	AccountName string `json:"account_name,omitempty"`
	// ProviderName is the stable provider key/name associated with the failed
	// attempt.
	ProviderName string `json:"provider_name,omitempty"`
	// Model is the canonical model name the request targeted.
	Model string `json:"model,omitempty"`
	// RequestedModel is the inbound model value after gateway normalization.
	RequestedModel string `json:"requested_model,omitempty"`
	// UpstreamModel is the provider-side model value sent upstream.
	UpstreamModel string `json:"upstream_model,omitempty"`
	// SourceEndpoint is the gateway endpoint family, e.g. /v1/chat/completions.
	SourceEndpoint string `json:"source_endpoint,omitempty"`
	// SourceProtocol is the inbound compatibility protocol family.
	SourceProtocol string `json:"source_protocol,omitempty"`
	// TargetProtocol is the selected provider protocol family.
	TargetProtocol string `json:"target_protocol,omitempty"`
	// AttemptNo is the cross-candidate attempt number for the failing request.
	AttemptNo int `json:"attempt_no,omitempty"`
	// StatusCode is the upstream HTTP status (0 for transport-level failures).
	StatusCode int `json:"status_code"`
	// UpstreamRequestID is the provider request id extracted from response
	// headers when the upstream returned one.
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	// ErrorClass is the gateway classification (e.g. server_bad, transient,
	// network_error, client_bad, no_available_account).
	ErrorClass string `json:"error_class,omitempty"`
	// ErrorPhase is the request lifecycle phase (e.g. upstream, scheduling).
	ErrorPhase string `json:"error_phase,omitempty"`
	// ErrorOwner classifies who likely owns remediation: client, provider,
	// scheduler, reverse_proxy, internal, etc.
	ErrorOwner string `json:"error_owner,omitempty"`
	// ErrorSource names the signal source, e.g. upstream_http or gateway.
	ErrorSource string `json:"error_source,omitempty"`
	// Message is the short upstream error message (provider-supplied).
	Message string `json:"message,omitempty"`
	// BodyExcerpt is a truncated, redacted snapshot of the upstream error body.
	BodyExcerpt string `json:"body_excerpt,omitempty"`
}

// SubscribeOptions filters the broadcast stream for one subscriber. Empty
// fields disable that filter. The publisher checks Match before pushing to the
// subscriber's channel so a heavily-filtered subscriber doesn't waste its
// 256-entry buffer on irrelevant events.
type SubscribeOptions struct {
	// AccountID, when non-nil, drops events whose AccountID differs.
	AccountID *int
	// ErrorClass, when non-empty, drops events whose ErrorClass differs.
	ErrorClass string
	// MinStatusCode, when > 0, drops events whose StatusCode < MinStatusCode.
	MinStatusCode int
	// MaxStatusCode, when > 0, drops events whose StatusCode > MaxStatusCode.
	MaxStatusCode int
	// SinceUnixMs, when > 0, drops events older than the cutoff. Used both as a
	// replay floor for the ring buffer (events older than SinceUnixMs are not
	// replayed) and as a steady-state filter (stale events that somehow leak
	// through are dropped).
	SinceUnixMs int64
}

// Match reports whether an event passes the configured filters.
func (o SubscribeOptions) Match(ev Event) bool {
	if o.AccountID != nil {
		if ev.AccountID == nil || *ev.AccountID != *o.AccountID {
			return false
		}
	}
	if o.ErrorClass != "" && ev.ErrorClass != o.ErrorClass {
		return false
	}
	if o.MinStatusCode > 0 && ev.StatusCode < o.MinStatusCode {
		return false
	}
	if o.MaxStatusCode > 0 && ev.StatusCode > o.MaxStatusCode {
		return false
	}
	if o.SinceUnixMs > 0 && ev.AtUnixMs < o.SinceUnixMs {
		return false
	}
	return true
}

// Subscriber is the per-consumer handle returned by Publisher.Subscribe.
// Receive returns a channel that yields filtered events; Close releases the
// slot and any internal goroutines.
type Subscriber interface {
	// Receive returns the read-only event channel. The channel is closed
	// when Close is called or the publisher is shut down.
	Receive() <-chan Event
	// Close releases the subscriber. Safe to call multiple times.
	Close() error
}

// Publisher is the fan-out surface. Publish is best-effort (never blocks);
// Subscribe returns a Subscriber bounded by the configured max cap.
type Publisher interface {
	// Publish broadcasts an event to every matching subscriber. It never
	// blocks the caller: subscribers whose channel is full are dropped
	// (their channel is closed) and a one-shot warning is logged.
	Publish(ctx context.Context, ev Event) error
	// Subscribe registers a new consumer and returns its Subscriber handle.
	// When the SinceUnixMs filter is set the publisher replays matching ring
	// buffer entries before the steady-state stream begins.
	Subscribe(ctx context.Context, opts SubscribeOptions) (Subscriber, error)
}
