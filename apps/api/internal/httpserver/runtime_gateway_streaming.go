package httpserver

import (
	"bytes"
	"io"
	"net/http"
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	forwardUpstreamResponseHeaders(w, providerResp.Headers, s.gatewayPassthroughHeaderConfig(r.Context()))
	flusher, _ := w.(http.Flusher)

	// Idle-timeout enforcement: read the upstream in a goroutine so the main loop
	// can react to a stall independently of a blocked Read. If no chunk arrives
	// within the configured window, stop streaming and close the body — a hung
	// upstream must not hold the client connection open indefinitely.
	idle := s.cfg.Gateway.StreamIdleTimeout
	idleTimedOut := false

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
		var sc streamChunk
		if idle > 0 {
			timer := time.NewTimer(idle)
			select {
			case sc = <-chunkCh:
				timer.Stop()
			case <-timer.C:
				idleTimedOut = true
				interrupted = true
				_ = providerResp.StreamBody.Close()
				break readLoop
			}
		} else {
			sc = <-chunkCh
		}
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
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if sc.err != nil {
			if sc.err != io.EOF {
				interrupted = true
			}
			break
		}
	}

	// Recover usage from the streamed bytes using the same parser as the
	// buffered path; fall back to the admission estimate if parsing is
	// unavailable or the response was too large to fully meter.
	usage := admission.EstimatedUsage
	usageEstimated := true
	var quotaSignals []provideradaptercontract.QuotaSignal
	if providerResp.StreamParse != nil && meter.Len() > 0 {
		if parsed, parseErr := providerResp.StreamParse(meter.Bytes(), statusOrOK(providerResp.StatusCode)); parseErr == nil {
			parsedUsage := gatewayUsageFromProvider(parsed)
			usage = parsedUsage
			usageEstimated = parsedUsage.Estimated
			quotaSignals = parsed.QuotaSignals
		}
	}

	pricing := s.runtime.gatewayPricing(r.Context(), gatewayPricingRequest(modelID, result.Candidate, usage), usage.Estimated)

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
		} else {
			record.ErrorClass = ptrStringValue("stream_interrupted")
		}
	}
	s.runtime.recordGatewayUsage(r.Context(), record)
}

func statusOrOK(status int) int {
	if status <= 0 {
		return http.StatusOK
	}
	return status
}
