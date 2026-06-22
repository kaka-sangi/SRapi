package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// maxStreamMeterBytes bounds how much of a streamed response is retained for
// post-stream usage parsing. Real model responses are far smaller; beyond this
// the meter stops growing and usage falls back to the admission estimate, so a
// pathological upstream cannot exhaust memory.
const maxStreamMeterBytes = 16 << 20

// anthropicStreamUsageAccumulator incrementally recovers token usage from an
// Anthropic SSE stream as bytes flow past, independent of the bounded meter.
// The terminal usage figures live on the message_delta event, which can fall
// beyond the 16MB meter cap on a pathologically large response; this fallback
// keeps a running tally so usage is still recovered in that case. It mirrors
// sub2api's processEvent usage handling (gateway_forward_as_responses.go):
// message_start carries the prompt-side counts (input/cache tokens) and
// message_delta carries the cumulative output count, merged with non-zero-wins
// semantics so a later non-zero value supersedes an earlier one.
type anthropicStreamUsageAccumulator struct {
	pending []byte // bytes of a partial trailing SSE line carried across chunks
	usage   anthropicAccumulatedUsage
	seen    bool // whether any usage field was ever observed
}

// anthropicAccumulatedUsage holds the running Anthropic token tally. Fields use
// Anthropic's wire names; cache_read_input_tokens maps to gateway CachedTokens
// and cache_creation_input_tokens to gateway CacheCreationTokens.
type anthropicAccumulatedUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// anthropicStreamUsageEvent is the minimal shape needed to read usage off an
// Anthropic SSE event payload. message_start nests usage under message.usage;
// message_delta carries usage at the top level.
type anthropicStreamUsageEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage anthropicAccumulatedUsage `json:"usage"`
	} `json:"message,omitempty"`
	Usage *anthropicAccumulatedUsage `json:"usage,omitempty"`
}

// mergeAnthropicAccumulatedUsage applies non-zero-wins semantics: a non-zero
// source field overwrites the destination, while a zero source field leaves the
// destination untouched. This is a faithful port of sub2api's
// mergeAnthropicUsage (gateway_forward_as_responses.go).
func mergeAnthropicAccumulatedUsage(dst *anthropicAccumulatedUsage, src anthropicAccumulatedUsage) {
	if dst == nil {
		return
	}
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
}

// write feeds a streamed chunk into the accumulator. Chunks may split SSE lines
// at arbitrary byte boundaries, so a partial trailing line is buffered until the
// next chunk completes it.
func (a *anthropicStreamUsageAccumulator) write(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	data := chunk
	if len(a.pending) > 0 {
		data = append(a.pending, chunk...)
		a.pending = nil
	}
	for {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			// No newline yet: retain the remainder for the next chunk. Copy so we
			// do not alias the caller's reusable read buffer.
			if len(data) > 0 {
				a.pending = append(a.pending[:0:0], data...)
			}
			return
		}
		a.processLine(data[:nl])
		data = data[nl+1:]
	}
}

// processLine inspects a single SSE line for an Anthropic usage-bearing event.
// Only "data: " lines are parsed; the event-type line is not required because
// the payload itself carries the discriminating "type" field.
func (a *anthropicStreamUsageAccumulator) processLine(line []byte) {
	line = bytes.TrimRight(line, "\r")
	const prefix = "data: "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return
	}
	payload := line[len(prefix):]
	if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		return
	}
	var event anthropicStreamUsageEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return
	}
	// message_delta carries the cumulative output usage.
	if event.Type == "message_delta" && event.Usage != nil {
		mergeAnthropicAccumulatedUsage(&a.usage, *event.Usage)
		a.seen = true
	}
	// message_start carries the prompt-side (input/cache) usage.
	if event.Type == "message_start" && event.Message != nil {
		mergeAnthropicAccumulatedUsage(&a.usage, event.Message.Usage)
		a.seen = true
	}
}

// streamLeaseCloser wraps a streamed upstream body so that closing it also
// releases the request's concurrency lease, exactly once, after the caller has
// finished streaming to the client.
type streamLeaseCloser struct {
	body    io.ReadCloser
	release func()
	once    sync.Once
}

func newStreamLeaseCloser(body io.ReadCloser, release func()) io.ReadCloser {
	if body == nil {
		body = io.NopCloser(bytes.NewReader(nil))
	}
	return &streamLeaseCloser{body: body, release: release}
}

func (c *streamLeaseCloser) Read(p []byte) (int, error) { return c.body.Read(p) }

func (c *streamLeaseCloser) Close() error {
	err := c.body.Close()
	c.once.Do(func() {
		if c.release != nil {
			c.release()
		}
	})
	return err
}

func newStreamTimer(interval time.Duration) (*time.Timer, <-chan time.Time) {
	if interval <= 0 {
		return nil, nil
	}
	timer := time.NewTimer(interval)
	return timer, timer.C
}

func resetStreamTimer(timer *time.Timer, interval time.Duration) {
	if timer == nil || interval <= 0 {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(interval)
}

func stopStreamTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func writeSSEKeepalive(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := io.WriteString(w, ":\n\n"); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// writeConversationStreamPassthrough streams a same-protocol upstream response
// to the client incrementally (flushing each chunk for low time-to-first-byte),
// tees a bounded copy to recover usage via the same parser the buffered path
// uses, and records usage after the stream completes. It is only reached when
// invokeProviderConversation produced a live StreamBody (eligible reverse-proxy
// passthrough); every other case keeps the buffered path unchanged.
func (s *Server) writeConversationStreamPassthrough(
	w http.ResponseWriter,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	result schedulercontract.ScheduleResult,
	providerResp provideradaptercontract.ConversationResponse,
	admission gatewayAdmission,
	modelID int,
	startedAt time.Time,
) {
	defer func() { _ = providerResp.StreamBody.Close() }()

	setSSEResponseHeaders(w)
	forwardUpstreamResponseHeaders(w, providerResp.Headers, s.gatewayPassthroughHeaderConfig(r.Context()))
	// Re-assert after forwarding upstream headers so an upstream that omits (or
	// contradicts) it can't re-enable proxy buffering on our hop.
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	// Idle-timeout enforcement: read the upstream in a goroutine so the main loop
	// can react to a stall independently of a blocked Read. If no chunk arrives
	// within the configured window, stop streaming and close the body — a hung
	// upstream must not hold the client connection open indefinitely.
	idle := s.cfg.Gateway.StreamIdleTimeout
	idleTimedOut := false
	idleTimer, idleCh := newStreamTimer(idle)
	defer stopStreamTimer(idleTimer)
	keepalive := s.cfg.Gateway.StreamKeepaliveInterval
	keepaliveTimer, keepaliveCh := newStreamTimer(keepalive)
	defer stopStreamTimer(keepaliveTimer)

	type streamChunk struct {
		data []byte
		err  error
	}
	chunkCh := make(chan streamChunk)
	readerDone := make(chan struct{})
	defer close(readerDone)
	go func() {
		for {
			b := make([]byte, 32*1024)
			n, err := providerResp.StreamBody.Read(b)
			if n > 0 {
				select {
				case chunkCh <- streamChunk{data: b[:n]}:
				case <-readerDone:
					return
				}
			}
			if err != nil {
				select {
				case chunkCh <- streamChunk{err: err}:
				case <-readerDone:
				}
				return
			}
		}
	}()

	var meter bytes.Buffer
	// usageAcc is a fallback Anthropic-SSE usage accumulator that runs over the
	// full stream (not just the bounded meter), so usage is still recovered when
	// the terminal message_delta usage event falls beyond the 16MB meter cap.
	var usageAcc anthropicStreamUsageAccumulator
	interrupted := false
readLoop:
	for {
		select {
		case sc := <-chunkCh:
			resetStreamTimer(idleTimer, idle)
			if len(sc.data) > 0 {
				if remaining := maxStreamMeterBytes - meter.Len(); remaining > 0 {
					if len(sc.data) <= remaining {
						meter.Write(sc.data)
					} else {
						meter.Write(sc.data[:remaining])
					}
				}
				usageAcc.write(sc.data)
				if _, writeErr := w.Write(sc.data); writeErr != nil {
					interrupted = true
					break readLoop
				}
				if flusher != nil {
					flusher.Flush()
				}
				resetStreamTimer(keepaliveTimer, keepalive)
			}
			if sc.err != nil {
				if sc.err != io.EOF {
					interrupted = true
					// A non-EOF mid-stream read error must surface to the client in
					// the source protocol, not as a silent truncation that looks like
					// a clean but incomplete response.
					_ = writeConversationStreamError(w, flusher, canonical, "upstream stream error", "upstream_stream_error", http.StatusBadGateway)
				}
				break readLoop
			}
		case <-idleCh:
			idleTimedOut = true
			interrupted = true
			_ = providerResp.StreamBody.Close()
			// Tell the client the stream stalled instead of leaving it to hang on a
			// connection that simply stops producing data.
			_ = writeConversationStreamError(w, flusher, canonical, "upstream stream idle timeout", "stream_idle_timeout", http.StatusGatewayTimeout)
			break readLoop
		case <-keepaliveCh:
			if err := writeSSEKeepalive(w, flusher); err != nil {
				interrupted = true
				break readLoop
			}
			resetStreamTimer(keepaliveTimer, keepalive)
		}
	}

	// Recover usage from the streamed bytes using the same parser as the
	// buffered path; fall back to the admission estimate if parsing is
	// unavailable or the response was too large to fully meter.
	usage := admission.EstimatedUsage
	usageEstimated := true
	quotaSignals := providerResp.QuotaSignals
	if providerResp.StreamParse != nil && meter.Len() > 0 {
		if parsed, parseErr := providerResp.StreamParse(meter.Bytes(), statusOrOK(providerResp.StatusCode)); parseErr == nil {
			parsedUsage := gatewayUsageFromProvider(parsed)
			usage = parsedUsage
			usageEstimated = parsedUsage.Estimated
			quotaSignals = parsed.QuotaSignals
			if !interrupted {
				s.runtime.bindGatewayPreviousResponseAffinity(r.Context(), authed.Key.ID, parsed.ID, result.Candidate.Account.ID)
			}
		}
	}

	// Fallback: if the primary meter parse produced no concrete usage (e.g. the
	// terminal usage event fell beyond the 16MB meter cap, leaving only the
	// admission estimate), but the incremental Anthropic accumulator captured
	// real token counts, adopt them. The primary StreamParse-on-meter path stays
	// authoritative whenever it yielded usage.
	if usageEstimated && usageAcc.seen && (usageAcc.usage.InputTokens > 0 || usageAcc.usage.OutputTokens > 0) {
		usage = gatewaycontract.Usage{
			InputTokens:         usageAcc.usage.InputTokens,
			OutputTokens:        usageAcc.usage.OutputTokens,
			CachedTokens:        usageAcc.usage.CacheReadInputTokens,
			CacheCreationTokens: usageAcc.usage.CacheCreationInputTokens,
			Observed:            true,
			Estimated:           false,
		}
		usageEstimated = false
	}

	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(modelID, result.Candidate, canonical, usage), ptrInt(result.Candidate.Account.ID), usage.Estimated)

	record := gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		ProviderID:            ptrInt(result.Candidate.Provider.ID),
		AccountID:             ptrInt(result.Candidate.Account.ID),
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		TargetProtocol:        result.Candidate.Provider.Protocol,
		Model:                 canonical.CanonicalModel,
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               !interrupted,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CachedTokens:          usage.CachedTokens,
		CacheCreationTokens:   usage.CacheCreationTokens,
		UsageEstimated:        usageEstimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
		ProviderQuotaSignals:  quotaSignals,
	}
	if interrupted {
		if idleTimedOut {
			record.ErrorClass = ptrStringValue("stream_idle_timeout")
			record.StreamCompletionState = "idle_timeout"
		} else {
			record.ErrorClass = ptrStringValue("stream_interrupted")
			record.StreamCompletionState = "interrupted"
		}
		record.ErrorPhase = "stream"
		record.ErrorOwner = "provider"
		record.ErrorSource = "upstream_stream"
		record.ProviderErrorMessage = *record.ErrorClass
	} else {
		record.StreamCompletionState = "completed"
	}
	s.recordOpsErrorLog(r.Context(), record)
	s.runtime.recordGatewayUsage(r.Context(), record)
}

func (s *Server) writeImageGenerationStreamPassthrough(
	w http.ResponseWriter,
	r *http.Request,
	authed apikeycontract.AuthResult,
	canonical gatewaycontract.CanonicalRequest,
	result schedulercontract.ScheduleResult,
	providerResp provideradaptercontract.ImageGenerationResponse,
	admission gatewayAdmission,
	modelID int,
	startedAt time.Time,
) {
	defer func() { _ = providerResp.StreamBody.Close() }()

	setSSEResponseHeaders(w)
	forwardUpstreamResponseHeaders(w, providerResp.Headers, s.gatewayPassthroughHeaderConfig(r.Context()))
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	idle := s.cfg.Gateway.ImageStreamIdleTimeout
	idleTimedOut := false
	idleTimer, idleCh := newStreamTimer(idle)
	defer stopStreamTimer(idleTimer)
	keepalive := s.cfg.Gateway.ImageStreamKeepaliveInterval
	keepaliveTimer, keepaliveCh := newStreamTimer(keepalive)
	defer stopStreamTimer(keepaliveTimer)
	type streamChunk struct {
		data []byte
		err  error
	}
	chunkCh := make(chan streamChunk)
	readerDone := make(chan struct{})
	defer close(readerDone)
	go func() {
		for {
			b := make([]byte, 32*1024)
			n, err := providerResp.StreamBody.Read(b)
			if n > 0 {
				select {
				case chunkCh <- streamChunk{data: b[:n]}:
				case <-readerDone:
					return
				}
			}
			if err != nil {
				select {
				case chunkCh <- streamChunk{err: err}:
				case <-readerDone:
				}
				return
			}
		}
	}()

	var meter bytes.Buffer
	interrupted := false
readLoop:
	for {
		select {
		case sc := <-chunkCh:
			resetStreamTimer(idleTimer, idle)
			if len(sc.data) > 0 {
				if remaining := maxStreamMeterBytes - meter.Len(); remaining > 0 {
					if len(sc.data) <= remaining {
						meter.Write(sc.data)
					} else {
						meter.Write(sc.data[:remaining])
					}
				}
				if _, writeErr := w.Write(sc.data); writeErr != nil {
					interrupted = true
					break readLoop
				}
				if flusher != nil {
					flusher.Flush()
				}
				resetStreamTimer(keepaliveTimer, keepalive)
			}
			if sc.err != nil {
				if sc.err != io.EOF {
					interrupted = true
					// Inject an in-band error event so the client knows the
					// stream was interrupted rather than seeing a silent close.
					_ = writeSSEStreamError(w, flusher, "upstream image stream interrupted", "upstream_stream_error", http.StatusBadGateway)
				}
				break readLoop
			}
		case <-idleCh:
			idleTimedOut = true
			interrupted = true
			_ = providerResp.StreamBody.Close()
			_ = writeSSEStreamError(w, flusher, "upstream image stream idle timeout", "stream_idle_timeout", http.StatusGatewayTimeout)
			break readLoop
		case <-keepaliveCh:
			if err := writeSSEKeepalive(w, flusher); err != nil {
				interrupted = true
				break readLoop
			}
			resetStreamTimer(keepaliveTimer, keepalive)
		}
	}

	usage := admission.EstimatedUsage
	usageEstimated := true
	quotaSignals := providerResp.QuotaSignals
	if providerResp.StreamParse != nil && meter.Len() > 0 {
		if parsed, parseErr := providerResp.StreamParse(meter.Bytes(), statusOrOK(providerResp.StatusCode)); parseErr == nil {
			parsedUsage := gatewayUsageFromImageProvider(parsed)
			usage = parsedUsage
			usageEstimated = parsedUsage.Estimated
			quotaSignals = parsed.QuotaSignals
		}
	}

	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequestForCanonical(modelID, result.Candidate, canonical, usage), ptrInt(result.Candidate.Account.ID), usage.Estimated)
	record := gatewayUsageRecord{
		RequestID:             canonical.RequestID,
		Authed:                authed,
		DecisionID:            result.Decision.ID,
		AttemptNo:             result.Decision.AttemptNo,
		ProviderID:            ptrInt(result.Candidate.Provider.ID),
		AccountID:             ptrInt(result.Candidate.Account.ID),
		SourceProtocol:        string(canonical.SourceProtocol),
		SourceEndpoint:        canonical.SourceEndpoint,
		TargetProtocol:        result.Candidate.Provider.Protocol,
		Model:                 canonical.CanonicalModel,
		RequestedModel:        gatewayUsageRequestedSnapshot(canonical, result.Candidate),
		UpstreamModel:         gatewayUsageUpstreamSnapshot(canonical, result.Candidate),
		Success:               !interrupted,
		StatusCode:            ptrInt(http.StatusOK),
		LatencyMS:             elapsedMillis(startedAt),
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CachedTokens:          usage.CachedTokens,
		UsageEstimated:        usageEstimated,
		Pricing:               pricing,
		CompatibilityWarnings: canonical.CompatibilityWarnings,
		ProviderQuotaSignals:  quotaSignals,
	}
	if interrupted {
		if idleTimedOut {
			record.ErrorClass = ptrStringValue("stream_idle_timeout")
			record.StreamCompletionState = "idle_timeout"
		} else {
			record.ErrorClass = ptrStringValue("stream_interrupted")
			record.StreamCompletionState = "interrupted"
		}
		record.ErrorPhase = "stream"
		record.ErrorOwner = "provider"
		record.ErrorSource = "upstream_stream"
		record.ProviderErrorMessage = *record.ErrorClass
	} else {
		record.StreamCompletionState = "completed"
	}
	s.recordOpsErrorLog(r.Context(), record)
	s.runtime.recordGatewayUsage(r.Context(), record)
}

type responsesStreamFailedEvent struct {
	Type     string                        `json:"type"`
	Response responsesStreamFailedResponse `json:"response"`
}

type responsesStreamFailedResponse struct {
	ID     string                     `json:"id"`
	Object string                     `json:"object"`
	Model  string                     `json:"model,omitempty"`
	Status string                     `json:"status"`
	Output []any                      `json:"output"`
	Error  responsesStreamFailedError `json:"error"`
}

type responsesStreamFailedError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeConversationStreamError(w http.ResponseWriter, flusher http.Flusher, canonical gatewaycontract.CanonicalRequest, message string, code string, statusCode int) error {
	if gatewaySourceEndpointIsResponses(canonical.SourceEndpoint) {
		return writeResponsesStreamFailed(w, flusher, canonical, message, code)
	}
	return writeSSEStreamError(w, flusher, message, code, statusCode)
}

func gatewaySourceEndpointIsResponses(sourceEndpoint string) bool {
	sourceEndpoint = strings.ToLower(strings.TrimSpace(sourceEndpoint))
	return strings.HasSuffix(sourceEndpoint, "/responses") || gatewaySourceEndpointIsResponsesCompact(sourceEndpoint)
}

func writeResponsesStreamFailed(w http.ResponseWriter, flusher http.Flusher, canonical gatewaycontract.CanonicalRequest, message string, code string) error {
	model := strings.TrimSpace(canonical.CanonicalModel)
	if model == "" {
		model = strings.TrimSpace(canonical.Model)
	}
	payload := responsesStreamFailedEvent{
		Type: "response.failed",
		Response: responsesStreamFailedResponse{
			ID:     responsesStreamFailureID(canonical.RequestID),
			Object: "response",
			Model:  model,
			Status: "failed",
			Output: []any{},
			Error: responsesStreamFailedError{
				Code:    strings.TrimSpace(code),
				Message: strings.TrimSpace(message),
			},
		},
	}
	if payload.Response.Error.Code == "" {
		payload.Response.Error.Code = "stream_error"
	}
	if payload.Response.Error.Message == "" {
		payload.Response.Error.Message = "upstream stream error"
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: response.failed\ndata: ")); err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func responsesStreamFailureID(requestID string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(requestID) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
		if b.Len() >= 96 {
			break
		}
	}
	part := b.String()
	if part == "" {
		part = "stream_error"
	}
	if strings.HasPrefix(part, "resp_") {
		return part
	}
	return "resp_" + part
}

// writeSSEStreamError emits an in-band `event: error` SSE frame so a client sees
// a real error instead of a silently truncated stream. The frame shape (type:
// error + nested error object) is compatible with chat and Anthropic-style SSE
// clients that do not define a richer terminal failure event.
func writeSSEStreamError(w http.ResponseWriter, flusher http.Flusher, message string, code string, statusCode int) error {
	payload := map[string]any{
		"type":        "error",
		"status_code": statusCode,
		"error": map[string]any{
			"message": message,
			"type":    code,
			"code":    code,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: error\ndata: ")); err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func statusOrOK(status int) int {
	if status <= 0 {
		return http.StatusOK
	}
	return status
}
