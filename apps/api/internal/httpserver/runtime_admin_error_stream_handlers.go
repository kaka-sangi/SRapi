// Package httpserver — admin SSE endpoint for the in-memory error event
// stream module. Port of CLIProxyAPI's SubscribeErrors fan-out: the gateway
// publishes one contract.Event per recorded provider attempt failure;
// connected admins receive them live as text/event-stream frames.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	erroreventcontract "github.com/srapi/srapi/apps/api/internal/modules/error_event_stream/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// errorStreamHeartbeat is the keepalive interval. Browsers + reverse proxies
// occasionally drop idle EventSource connections after ~30s; an SSE comment
// every 15s keeps the channel warm without polluting the event stream.
const errorStreamHeartbeat = 15 * time.Second

// publishErrorEvent assembles a contract.Event from the gateway failover
// recorder inputs and pushes it into the in-process publisher. Best-effort:
// nil publisher (test runtime with no error stream wired), full subscriber
// channels, and Publish errors are all swallowed so the gateway hot path
// never blocks on telemetry.
func (s *Server) publishErrorEvent(ctx context.Context, authed apikeycontract.AuthResult, canonical gatewaycontract.CanonicalRequest, result schedulercontract.ScheduleResult, providerErr error, errorClass string, upstreamStatus int) {
	if s == nil || s.runtime == nil || s.runtime.errorEventStream == nil {
		return
	}
	headers := providerHeadersFromError(providerErr)
	phase := classifyErrorPhase(errorClass, upstreamStatus)
	ev := erroreventcontract.Event{
		AtUnixMs:          time.Now().UTC().UnixMilli(),
		RequestID:         canonical.RequestID,
		ProviderName:      strings.TrimSpace(result.Candidate.Provider.Name),
		AccountName:       strings.TrimSpace(result.Candidate.Account.Name),
		Model:             canonical.CanonicalModel,
		RequestedModel:    gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:     gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		SourceEndpoint:    canonical.SourceEndpoint,
		SourceProtocol:    string(canonical.SourceProtocol),
		TargetProtocol:    result.Candidate.Provider.Protocol,
		AttemptNo:         result.Decision.AttemptNo,
		StatusCode:        upstreamStatus,
		UpstreamRequestID: upstreamRequestIDFromHeaders(headers),
		ErrorClass:        errorClass,
		ErrorPhase:        phase,
		ErrorOwner:        classifyErrorOwner(phase),
		ErrorSource:       classifyErrorSource(phase),
		Message:           providerErrorMessage(providerErr),
		BodyExcerpt:       providerErrorBodyExcerpt(providerErr),
	}
	if authed.UserID > 0 {
		uid := authed.UserID
		ev.UserID = &uid
	}
	if result.Candidate.Account.ID > 0 {
		aid := result.Candidate.Account.ID
		ev.AccountID = &aid
	}
	if result.Candidate.Provider.ID > 0 {
		pid := result.Candidate.Provider.ID
		ev.ProviderID = &pid
	}
	if err := s.runtime.errorEventStream.Publish(ctx, ev); err != nil && s.runtime.logger != nil {
		s.runtime.logger.Warn("error_event_stream publish failed",
			"request_id", canonical.RequestID,
			"error", err,
		)
	}
}

// handleAdminErrorStream serves GET /api/v1/admin/error-stream as an SSE
// stream of contract.Event objects. Optional filters via query params:
//
//	account_id     — restrict to one provider account (int)
//	error_class    — exact match (e.g. server_bad, transient)
//	min_status     — drop events below this upstream status code
//	max_status     — drop events above this upstream status code
//	since          — unix-ms cursor; the ring buffer is replayed from here
//
// The handler returns 503 when the in-process publisher is missing or the
// subscriber cap is reached.
func (s *Server) handleAdminErrorStream(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeJSONString(w, http.StatusForbidden, `{"error":{"code":"FORBIDDEN","message":"admin access required"},"request_id":"`+requestID+`"}`)
		return
	}
	if s.runtime == nil || s.runtime.errorEventStream == nil {
		writeJSONString(w, http.StatusServiceUnavailable, `{"error":{"code":"UNAVAILABLE","message":"error event stream unavailable"},"request_id":"`+requestID+`"}`)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"streaming unsupported"},"request_id":"`+requestID+`"}`)
		return
	}

	opts, err := parseAdminErrorStreamOptions(r)
	if err != nil {
		writeJSONString(w, http.StatusBadRequest, `{"error":{"code":"VALIDATION_FAILED","message":"`+err.Error()+`"},"request_id":"`+requestID+`"}`)
		return
	}
	sub, err := s.runtime.errorEventStream.Subscribe(r.Context(), opts)
	if err != nil {
		if errors.Is(err, erroreventcontract.ErrTooManySubscribers) {
			writeJSONString(w, http.StatusServiceUnavailable, `{"error":{"code":"TOO_MANY_SUBSCRIBERS","message":"error stream is at capacity"},"request_id":"`+requestID+`"}`)
			return
		}
		writeJSONString(w, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"subscribe failed"},"request_id":"`+requestID+`"}`)
		return
	}
	defer func() { _ = sub.Close() }()

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Emit a single bootstrap comment so EventSource resolves onopen immediately.
	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(errorStreamHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-sub.Receive():
			if !ok {
				// Publisher closed the subscriber (eviction or shutdown).
				return
			}
			if err := writeAdminErrorStreamEvent(w, ev); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// parseAdminErrorStreamOptions extracts the optional SSE filters from the
// request query string. Invalid filters fail closed so an operator never thinks
// they are watching a narrow stream while the backend opened a broad one.
func parseAdminErrorStreamOptions(r *http.Request) (erroreventcontract.SubscribeOptions, error) {
	q := r.URL.Query()
	opts := erroreventcontract.SubscribeOptions{}
	if v := strings.TrimSpace(q.Get("account_id")); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			return erroreventcontract.SubscribeOptions{}, errors.New("invalid account_id")
		}
		opts.AccountID = &id
	}
	if v := strings.TrimSpace(q.Get("error_class")); v != "" {
		opts.ErrorClass = v
	}
	if v := strings.TrimSpace(q.Get("min_status")); v != "" {
		code, err := strconv.Atoi(v)
		if err != nil || code <= 0 {
			return erroreventcontract.SubscribeOptions{}, errors.New("invalid min_status")
		}
		opts.MinStatusCode = code
	}
	if v := strings.TrimSpace(q.Get("max_status")); v != "" {
		code, err := strconv.Atoi(v)
		if err != nil || code <= 0 {
			return erroreventcontract.SubscribeOptions{}, errors.New("invalid max_status")
		}
		opts.MaxStatusCode = code
	}
	if opts.MinStatusCode > 0 && opts.MaxStatusCode > 0 && opts.MinStatusCode > opts.MaxStatusCode {
		return erroreventcontract.SubscribeOptions{}, errors.New("min_status must be <= max_status")
	}
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		since, err := strconv.ParseInt(v, 10, 64)
		if err != nil || since <= 0 {
			return erroreventcontract.SubscribeOptions{}, errors.New("invalid since")
		}
		opts.SinceUnixMs = since
	}
	return opts, nil
}

// writeAdminErrorStreamEvent encodes ev as a single SSE frame:
//
//	event: gateway_error
//	data: {<json>}
//
// followed by the blank-line terminator. Returns the underlying Write error
// when the client disconnected mid-frame so the caller can stop streaming.
func writeAdminErrorStreamEvent(w http.ResponseWriter, ev erroreventcontract.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: gateway_error\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	return nil
}
